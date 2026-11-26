package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type FollowUpCheck struct {
	At           string `json:"at"`
	DueDate      string `json:"due_date"` // Commitment date evaluated at this contact.
	AgreedAction string `json:"agreed_action"`
	Owner        string `json:"owner"`
	Delivery     string `json:"delivery"` // Whether the agreed action was carried out.
	Outcome      string `json:"outcome"`  // done means the participant confirmed that the action helped.
	Note         string `json:"note"`
}

// FollowUpReschedule preserves an abandoned deadline, including changes before the first check.
type FollowUpReschedule struct {
	At          string `json:"at"`
	DueDate     string `json:"due_date"`
	NextDueDate string `json:"next_due_date"`
}

type FollowUp struct {
	SignalID     string               `json:"signal_id"`
	Episode      int                  `json:"episode"`
	Pair         string               `json:"pair"`
	Title        string               `json:"title"`
	Owner        string               `json:"owner"`
	AgreedAction string               `json:"agreed_action"`
	DueDate      string               `json:"due_date"`
	Status       string               `json:"status"` // closed marks the end of follow-up, regardless of the outcome.
	Revision     int                  `json:"revision"`
	Checks       []FollowUpCheck      `json:"checks"`
	Reschedules  []FollowUpReschedule `json:"reschedules"`
}

type storedFollowUp struct {
	FollowUp
	LastCommand string `json:"last_command"`
}

type FollowUpView struct {
	FollowUp
	Timing string `json:"timing"`
}

type FollowUpQueue struct {
	Today string         `json:"today"`
	Items []FollowUpView `json:"items"`
}

type FollowUpCommand struct {
	Operation    string `json:"operation"`
	Revision     *int   `json:"revision"`
	Owner        string `json:"owner"`
	AgreedAction string `json:"agreed_action"`
	DueDate      string `json:"due_date"`
	Delivery     string `json:"delivery"`
	Outcome      string `json:"outcome"`
	Note         string `json:"note"`
	NextDueDate  string `json:"next_due_date"`
}

var (
	ErrFollowUpNotFound = errors.New("сигнал или контрольный контакт не найден")
	ErrFollowUpConflict = errors.New("контрольный контакт уже изменён. Обновите очередь")
	ErrFollowUpInvalid  = errors.New("некорректный контрольный контакт")
	moscow              = time.FixedZone("МСК", 3*60*60)
)

func validDay(value string) bool {
	date, err := time.Parse(time.DateOnly, value)
	return err == nil && date.Year() > 0 && date.Format(time.DateOnly) == value
}

func (c *FollowUpCommand) normalize() error {
	c.Owner = strings.TrimSpace(c.Owner)
	c.AgreedAction = strings.TrimSpace(c.AgreedAction)
	c.Note = strings.TrimSpace(c.Note)
	if c.Revision == nil || *c.Revision < 0 {
		return fmt.Errorf("%w: укажите версию записи", ErrFollowUpInvalid)
	}
	switch c.Operation {
	case "plan":
		if c.Owner == "" {
			c.Owner = "HR"
		}
		if c.AgreedAction == "" || utf8.RuneCountInString(c.AgreedAction) > 500 || utf8.RuneCountInString(c.Owner) > 100 || !validDay(c.DueDate) {
			return fmt.Errorf("%w: нужны действие до 500 символов, ответственный до 100 символов и календарная дата", ErrFollowUpInvalid)
		}
		if c.Outcome != "" || c.Note != "" || c.NextDueDate != "" || c.Delivery != "" {
			return ErrFollowUpInvalid
		}
	case "check":
		if c.Delivery == "" {
			c.Delivery = "unknown"
		}
		switch c.Delivery {
		case "done", "partial", "not_done", "unknown":
		default:
			return fmt.Errorf("%w: неизвестный статус выполнения действия", ErrFollowUpInvalid)
		}
		switch c.Outcome {
		case "done", "partial", "no_change", "unknown":
		default:
			return fmt.Errorf("%w: укажите результат проверки", ErrFollowUpInvalid)
		}
		if c.Owner != "" || c.AgreedAction != "" || c.DueDate != "" || utf8.RuneCountInString(c.Note) > 500 {
			return ErrFollowUpInvalid
		}
		if c.NextDueDate != "" && (!validDay(c.NextDueDate) || c.Outcome == "done") {
			return fmt.Errorf("%w: укажите следующий срок для незавершённого случая", ErrFollowUpInvalid)
		}
	default:
		return ErrFollowUpInvalid
	}
	return nil
}

func (f FollowUp) view(today string) FollowUpView {
	// Snapshots returned to readers never alias a mutable store slice.
	f.Checks = append([]FollowUpCheck{}, f.Checks...)
	f.Reschedules = append([]FollowUpReschedule{}, f.Reschedules...)
	timing := "upcoming"
	switch {
	case f.Status == "closed":
		timing = "closed"
	case f.DueDate < today:
		timing = "overdue"
	case f.DueDate == today:
		timing = "today"
	}
	return FollowUpView{FollowUp: f, Timing: timing}
}

func (s *Store) FollowUpQueue() FollowUpQueue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := FollowUpQueue{Today: s.now().In(moscow).Format(time.DateOnly), Items: []FollowUpView{}}
	for _, f := range s.st.FollowUps {
		q.Items = append(q.Items, f.view(q.Today))
	}
	sort.Slice(q.Items, func(i, j int) bool {
		a, b := q.Items[i], q.Items[j]
		if a.Status != b.Status {
			return a.Status == "open"
		}
		if a.DueDate != b.DueDate {
			return a.DueDate < b.DueDate
		}
		if a.SignalID != b.SignalID {
			return a.SignalID < b.SignalID
		}
		return a.Episode < b.Episode
	})
	return q
}

// UpdateFollowUp compares revisions and persists the operation atomically. A retried last command
// returns its existing result. New records require a pair and title resolved by the server.
func (s *Store) UpdateFollowUp(signal string, episode int, pair, title string, c FollowUpCommand) (FollowUpView, bool, error) {
	if err := c.normalize(); err != nil {
		return FollowUpView{}, false, err
	}
	b, _ := json.Marshal(c)
	fingerprint := sha256.Sum256(b)
	hash := hex.EncodeToString(fingerprint[:])
	key := fmt.Sprintf("%s/%d", signal, episode)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	today := now.In(moscow).Format(time.DateOnly)
	old, found := s.st.FollowUps[key]
	if found && old.LastCommand == hash {
		return old.view(today), false, nil
	}
	if !found && (c.Operation != "plan" || pair == "" || title == "") {
		return FollowUpView{}, false, ErrFollowUpNotFound
	}
	if old.Revision != *c.Revision || old.Status == "closed" {
		return FollowUpView{}, false, ErrFollowUpConflict
	}
	task := old.FollowUp
	if !found {
		task = FollowUp{SignalID: signal, Episode: episode, Pair: pair, Title: title, Status: "open", Checks: []FollowUpCheck{}}
	}
	if c.Operation == "plan" {
		if found && task.DueDate != c.DueDate {
			move := FollowUpReschedule{At: now.UTC().Format(time.RFC3339), DueDate: task.DueDate, NextDueDate: c.DueDate}
			task.Reschedules = append(task.Reschedules[:len(task.Reschedules):len(task.Reschedules)], move)
		}
		task.Owner, task.AgreedAction, task.DueDate = c.Owner, c.AgreedAction, c.DueDate
	} else {
		if c.NextDueDate != "" && c.NextDueDate < today {
			return FollowUpView{}, false, fmt.Errorf("%w: следующий контакт должен быть сегодня или позже", ErrFollowUpInvalid)
		}
		check := FollowUpCheck{At: now.UTC().Format(time.RFC3339), DueDate: task.DueDate, Owner: task.Owner, AgreedAction: task.AgreedAction, Delivery: c.Delivery, Outcome: c.Outcome, Note: c.Note}
		task.Checks = append(task.Checks[:len(task.Checks):len(task.Checks)], check)
		if c.NextDueDate == "" {
			task.Status = "closed"
		} else {
			task.DueDate = c.NextDueDate
		}
	}
	task.Revision++
	if s.st.FollowUps == nil {
		s.st.FollowUps = map[string]storedFollowUp{}
	}
	s.st.FollowUps[key] = storedFollowUp{FollowUp: task, LastCommand: hash}
	if err := s.save(); err != nil {
		if found {
			s.st.FollowUps[key] = old
		} else {
			delete(s.st.FollowUps, key)
		}
		return FollowUpView{}, false, err
	}
	return task.view(today), true, nil
}

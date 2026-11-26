// Package store — состояние прототипа: ответы, разбор комментариев, вердикты HR, пороги и ссылки на анкеты.
// Хранится в памяти и сохраняется атомарно в JSON-файл (STATE_FILE), чтобы переживать перезапуск.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ElzaraA/mgts-ai/internal/signals"
	"github.com/ElzaraA/mgts-ai/internal/survey"
	"github.com/ElzaraA/mgts-ai/internal/textai"
)

// Event — запись журнала «откуда пришли данные» (для экрана «Источники данных»).
type Event struct {
	At     string `json:"at"`
	Source string `json:"source"`
	Text   string `json:"text"`
}

type state struct {
	FormWindow *FormWindow                  `json:"form_window,omitempty"`
	Survey     survey.Survey                `json:"survey"`
	Responses  map[string]survey.Response   `json:"responses"`
	Previous   map[string][]survey.Response `json:"previous_responses,omitempty"` // Superseded versions, oldest first.
	Analyses   map[string]textai.Analysis   `json:"analyses"`
	Verdicts   map[string]signals.Verdict   `json:"verdicts"`
	Thresholds signals.Thresholds           `json:"thresholds"`
	Tokens     map[string]string            `json:"tokens"` // токен ссылки на анкету → ID участника
	Events     []Event                      `json:"events"`
	Deliveries []string                     `json:"deliveries,omitempty"` // последние X-Delivery-Id вебхука (FIFO)
}

// MaxDeliveries — сколько последних ID доставки вебхука помнить для отсева повторов.
const MaxDeliveries = 500

type Store struct {
	mu   sync.RWMutex
	st   state
	file string
	now  func() time.Time
}

func New(sv survey.Survey, file string) *Store {
	s := &Store{file: file, now: time.Now}
	s.st = fresh(sv)
	s.ensureTokens()
	return s
}

func fresh(sv survey.Survey) state {
	return state{Survey: sv, Responses: map[string]survey.Response{}, Analyses: map[string]textai.Analysis{},
		Verdicts: map[string]signals.Verdict{}, Thresholds: signals.Default, Tokens: map[string]string{}, Events: []Event{}}
}

// Load читает снимок; ok=false, если файла нет. Повреждённый снимок не роняет сервер: он переименовывается
// в <файл>.corrupt-<время> (чтобы его можно было разобрать вручную), и сервер стартует как с пустым состоянием.
func (s *Store) Load() (bool, error) {
	if s.file == "" {
		return false, nil
	}
	b, err := os.ReadFile(s.file)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var st state
	if err := json.Unmarshal(b, &st); err != nil {
		bad := fmt.Sprintf("%s.corrupt-%s", s.file, s.now().UTC().Format("20060102T150405Z"))
		if rerr := os.Rename(s.file, bad); rerr != nil {
			return false, fmt.Errorf("state file corrupt (%v) and not renamed: %w", err, rerr)
		}
		slog.Warn("state file corrupt, starting fresh", "file", s.file, "moved_to", bad, "err", err)
		return false, nil
	}
	if st.Responses == nil {
		st.Responses = map[string]survey.Response{}
	}
	if st.Analyses == nil {
		st.Analyses = map[string]textai.Analysis{}
	}
	refreshOfflineAnalyses(&st)
	if st.Verdicts == nil {
		st.Verdicts = map[string]signals.Verdict{}
	}
	if st.Tokens == nil {
		st.Tokens = map[string]string{}
	}
	s.mu.Lock()
	s.st = st
	s.mu.Unlock()
	s.ensureTokens()
	return true, nil
}

// MergeSurvey применяет структуру анкет из файла (SURVEY_FILE) поверх снимка: вопросы, заголовок, число месяцев
// и служебные колонки берутся из файла, пары — объединение файла и снимка (при совпадении ID или участника
// главнее файл). Так правка data/survey.json на финале действует и при сохранённом состоянии.
func (s *Store) MergeSurvey(file survey.Survey) error {
	s.mu.Lock()
	old := s.st.Survey
	merged := file
	// Existing responses use this origin; a restart must not change their calendar.
	if old.StartMonth != "" {
		merged.StartMonth = old.StartMonth
	}
	mergeAliases(&merged, old)
	merged.Participant = append([]survey.Pair(nil), file.Participant...)
	ids, people := map[string]bool{}, map[string]bool{}
	for _, p := range file.Participant {
		ids[p.ID], people[p.Intern], people[p.Mentor] = true, true, true
	}
	for _, p := range old.Participant {
		if ids[p.ID] || people[p.Intern] || people[p.Mentor] {
			continue
		}
		merged.Participant = append(merged.Participant, p)
		ids[p.ID], people[p.Intern], people[p.Mentor] = true, true, true
	}
	s.st.Survey = merged
	s.ensureTokensLocked()
	if err := s.save(); err != nil {
		s.st.Survey = old
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	return nil
}

// mergeAliases добавляет к структуре из файла варианты заголовков, выученные при импорте (remember) и лежащие
// в снимке: правка файла главнее, но выученное не теряется при перезапуске. Срезы file не меняются на месте.
func mergeAliases(file *survey.Survey, old survey.Survey) {
	union := func(dst, src []string) []string {
		out := append([]string(nil), dst...)
		seen := map[string]bool{}
		for _, a := range dst {
			seen[strings.ToLower(strings.TrimSpace(a))] = true
		}
		for _, a := range src {
			if k := strings.ToLower(strings.TrimSpace(a)); k != "" && !seen[k] {
				seen[k] = true
				out = append(out, a)
			}
		}
		return out
	}
	file.Questions = append([]survey.Question(nil), file.Questions...)
	for i := range file.Questions {
		if q, ok := old.Q(file.Questions[i].Code); ok {
			file.Questions[i].Aliases = union(file.Questions[i].Aliases, q.Aliases)
		}
	}
	file.IDColumns.Intern = union(file.IDColumns.Intern, old.IDColumns.Intern)
	file.IDColumns.Mentor = union(file.IDColumns.Mentor, old.IDColumns.Mentor)
	file.IDColumns.Period = union(file.IDColumns.Period, old.IDColumns.Period)
}

// save — атомарная запись (tmp + rename); вызывается под записью мьютекса.
func (s *Store) save() error {
	if s.file == "" {
		return nil
	}
	b, err := json.Marshal(s.st)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.file), 0o755); err != nil {
		return err
	}
	tmp := s.file + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.file)
}

func (s *Store) stamp() string { return s.now().UTC().Format(time.RFC3339) }

// Reset — начальное состояние (структура анкет + опционально данные).
func (s *Store) Reset(sv survey.Survey, rs []survey.Response, source string) error {
	s.mu.Lock()
	s.st = fresh(sv)
	s.mu.Unlock()
	s.ensureTokens()
	_, err := s.Upsert(rs, source, "Загружен синтетический датасет (данные не настоящие)")
	return err
}

// Upsert добавляет или заменяет ответы (повторная отправка за тот же месяц — новая версия ответа).
// Возвращает ID изменившихся ответов. Идемпотентно: тот же файл второй раз ничего не меняет.
// Если снимок не удалось записать, изменения в памяти откатываются: ответ либо принят целиком, либо нет.
func (s *Store) Upsert(rs []survey.Response, source, note string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upsertLocked(rs, source, note)
}

// ApplyImport атомарно применяет импорт: новую структуру (если появились пары; nil — без изменений) и ответы.
func (s *Store) ApplyImport(sv *survey.Survey, rs []survey.Response, note string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sv == nil {
		return s.upsertLocked(rs, "import", note)
	}
	oldSv, oldTokens := s.st.Survey, copyMap(s.st.Tokens)
	s.st.Survey = *sv
	s.ensureTokensLocked()
	changed, err := s.upsertLocked(rs, "import", note)
	if err != nil {
		s.st.Survey, s.st.Tokens = oldSv, oldTokens
	}
	return changed, err
}

func copyMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *Store) upsertLocked(rs []survey.Response, source, note string) ([]string, error) {
	type prev struct {
		r  survey.Response
		ok bool
	}
	backup := map[string]prev{}
	previous := s.st.Previous
	if len(rs) > 0 {
		s.st.Previous = copyMap(previous)
	}
	events := s.st.Events
	var changed []string
	for _, r := range rs {
		if r.At == "" {
			r.At = s.stamp()
		}
		if old, ok := s.st.Responses[r.ID]; ok {
			// импорт дополняет ответ: колонки, которых нет в файле, не затирают уже известные значения;
			// ответ, пришедший напрямую из анкеты или вебхука, свежее выгрузки — импорт его не трогает
			if source == "import" {
				if old.Source == "form" || old.Source == "webhook" {
					continue
				}
				if r.Source != "yandex-forms" {
					r = merge(old, r)
				}
			}
			if same(old, r) && (r.Source != "yandex-forms" || old.Source == r.Source) {
				continue
			}
			versions := s.st.Previous[r.ID]
			s.st.Previous[r.ID] = append(versions[:len(versions):len(versions)], old)
		}
		if _, seen := backup[r.ID]; !seen {
			old, ok := s.st.Responses[r.ID]
			backup[r.ID] = prev{old, ok}
		}
		s.st.Responses[r.ID] = r
		changed = append(changed, r.ID)
	}
	if note != "" || len(changed) > 0 {
		if note == "" {
			note = "Принято ответов"
		}
		s.logLocked(source, note, len(changed))
	}
	if err := s.save(); err != nil {
		for id, p := range backup {
			if p.ok {
				s.st.Responses[id] = p.r
			} else {
				delete(s.st.Responses, id)
			}
		}
		s.st.Events = events
		s.st.Previous = previous
		return nil, err
	}
	return changed, nil
}

// ResponseVersions returns a pair's responses in arrival order. The last element
// of each list is the sole version used in calculations and participation counts.
func (s *Store) ResponseVersions(pair string) map[string][]survey.Response {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string][]survey.Response{}
	for id, current := range s.st.Responses {
		if current.PairID == pair {
			out[id] = append(slices.Clone(s.st.Previous[id]), current)
		}
	}
	return out
}

func merge(old, upd survey.Response) survey.Response {
	out := upd
	out.Scores, out.Comments = map[string]float64{}, map[string]string{}
	for k, v := range old.Scores {
		out.Scores[k] = v
	}
	for k, v := range old.Comments {
		out.Comments[k] = v
	}
	for k, v := range upd.Scores {
		out.Scores[k] = v
	}
	for k, v := range upd.Comments {
		out.Comments[k] = v
	}
	if old.Source != "" && same(old, out) {
		out.Source, out.At = old.Source, old.At
	}
	return out
}

func same(a, b survey.Response) bool {
	if a.Role != b.Role || a.Person != b.Person || a.Period != b.Period || len(a.Scores) != len(b.Scores) || len(a.Comments) != len(b.Comments) {
		return false
	}
	for k, v := range a.Scores {
		if b.Scores[k] != v {
			return false
		}
	}
	for k, v := range a.Comments {
		if b.Comments[k] != v {
			return false
		}
	}
	return true
}

func (s *Store) logLocked(source, note string, n int) {
	text := note
	if n >= 0 {
		text = note + ": " + itoa(n)
	}
	// новый срез: старый остаётся нетронутым для отката при ошибке записи
	s.st.Events = append([]Event{{At: s.stamp(), Source: source, Text: text}}, s.st.Events...)
	if len(s.st.Events) > 50 {
		s.st.Events = s.st.Events[:50]
	}
}

// Log — запись в журнал без изменения данных.
func (s *Store) Log(source, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.Events = append([]Event{{At: s.stamp(), Source: source, Text: text}}, s.st.Events...)
	if len(s.st.Events) > 50 {
		s.st.Events = s.st.Events[:50]
	}
	s.saveOrLog("log")
}

// saveOrLog — для записей, где откат не нужен (журнал, разбор текста): ошибка не теряется, а попадает в лог.
func (s *Store) saveOrLog(op string) {
	if err := s.save(); err != nil {
		slog.Error("state not saved", "op", op, "file", s.file, "err", err)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// SetAnalysis сохраняет разбор комментария.
func (s *Store) SetAnalysis(key string, a textai.Analysis) error {
	return s.SetAnalyses(map[string]textai.Analysis{key: a})
}

// SetAnalyses — пакетная запись разборов одним сохранением снимка (импорт на сотни комментариев
// не переписывает файл сотни раз). Разбор при ошибке записи остаётся в памяти: его можно повторить.
func (s *Store) SetAnalyses(m map[string]textai.Analysis) error {
	if len(m) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, a := range m {
		s.st.Analyses[k] = a
	}
	return s.save()
}

// PendingItem — комментарий, ожидающий разбора моделью.
type PendingItem struct {
	Key, Text, Role string
	Period          int
}

// Pending — комментарии, у которых разбор помечен заметкой note (например, «ожидает разбора моделью»).
func (s *Store) Pending(note string) []PendingItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []PendingItem
	for _, r := range s.st.Responses {
		for code, text := range r.Comments {
			key := r.ID + "/" + code
			if a, ok := s.st.Analyses[key]; ok && a.Note == note {
				out = append(out, PendingItem{Key: key, Text: text, Role: string(r.Role), Period: r.Period})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Analysis — разбор комментария, если есть.
func (s *Store) Analysis(key string) (textai.Analysis, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.st.Analyses[key]
	return a, ok
}

// Input — снимок для расчёта сигналов.
func (s *Store) Input() signals.Input {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rs := make([]survey.Response, 0, len(s.st.Responses))
	hasFormResponse := false
	for _, r := range s.st.Responses {
		rs = append(rs, r)
		hasFormResponse = hasFormResponse || r.Source == "form" || r.Source == "webhook"
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].ID < rs[j].ID })
	an := make(map[string]textai.Analysis, len(s.st.Analyses))
	for k, v := range s.st.Analyses {
		an[k] = v
	}
	sv := s.st.Survey
	sv.Participant = append([]survey.Pair(nil), sv.Participant...)
	in := signals.Input{Survey: sv, Responses: rs, Analyses: an, T: s.st.Thresholds}
	if s.st.FormWindow != nil || hasFormResponse {
		window := s.formWindowLocked()
		in.WindowPeriod, in.WindowOpen = window.Period, !window.Closed
	}
	return in
}

func (s *Store) Verdicts() map[string]signals.Verdict {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]signals.Verdict, len(s.st.Verdicts))
	for k, v := range s.st.Verdicts {
		out[k] = v
	}
	return out
}

// SetVerdict — решение HR по сигналу; пустой статус снимает вердикт.
func (s *Store) SetVerdict(id, status, comment string) (signals.Verdict, error) {
	return s.Decide(id, status, comment, "", 0)
}

// Decide — решение HR с действием («встреча 1:1 проведена»…) и месяцем решения (для петли «действие → эффект»).
func (s *Store) Decide(id, status, comment, action string, period int) (signals.Verdict, error) {
	return s.DecideInEpisode(id, status, comment, action, period, 0)
}

func (s *Store) DecideInEpisode(id, status, comment, action string, period, episode int) (signals.Verdict, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, had := s.st.Verdicts[id]
	undo := func() {
		if had {
			s.st.Verdicts[id] = old
		} else {
			delete(s.st.Verdicts, id)
		}
	}
	if status == "" {
		delete(s.st.Verdicts, id)
		if err := s.save(); err != nil {
			undo()
			return signals.Verdict{}, err
		}
		return signals.Verdict{}, nil
	}
	v := signals.Verdict{Status: status, Comment: comment, At: s.stamp(), Action: action, Period: period}
	v.Episode = episode
	if !had || (episode > 0 && old.Period > 0 && old.Period < episode) {
		v.FirstAt = v.At
	} else if old.Episode == episode {
		v.FirstAt = old.FirstAt
	}
	s.st.Verdicts[id] = v
	if err := s.save(); err != nil {
		undo()
		return signals.Verdict{}, err
	}
	return v, nil
}

func (s *Store) Survey() survey.Survey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sv := s.st.Survey
	sv.Participant = append([]survey.Pair(nil), sv.Participant...)
	return sv
}

// SetSurvey заменяет структуру (например, после импорта появились новые пары).
func (s *Store) SetSurvey(sv survey.Survey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	oldSv, oldTokens := s.st.Survey, copyMap(s.st.Tokens)
	s.st.Survey = sv
	s.ensureTokensLocked()
	if err := s.save(); err != nil {
		s.st.Survey, s.st.Tokens = oldSv, oldTokens
		return err
	}
	return nil
}

func (s *Store) Thresholds() signals.Thresholds {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.st.Thresholds
}

func (s *Store) SetThresholds(t signals.Thresholds) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, events := s.st.Thresholds, s.st.Events
	s.st.Thresholds = t
	s.logLocked("hr", "Пороги сигналов изменены", -1)
	if err := s.save(); err != nil {
		s.st.Thresholds, s.st.Events = old, events
		return err
	}
	return nil
}

func (s *Store) Events() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Event{}, s.st.Events...)
}

// ensureTokens выдаёт каждому участнику постоянную случайную ссылку на анкету.
func (s *Store) ensureTokens() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureTokensLocked()
}

func (s *Store) ensureTokensLocked() {
	have := map[string]bool{}
	for _, p := range s.st.Tokens {
		have[p] = true
	}
	for _, p := range s.st.Survey.Participant {
		for _, person := range []string{p.Intern, p.Mentor} {
			if !have[person] {
				s.st.Tokens[newToken()] = person
				have[person] = true
			}
		}
	}
}

func newToken() string {
	b := make([]byte, 9)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Store) HasDelivery(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Contains(s.st.Deliveries, id)
}

func (s *Store) recordDeliveryLocked(id string) {
	next := append(slices.Clone(s.st.Deliveries), id)
	if over := len(next) - MaxDeliveries; over > 0 {
		next = next[over:]
	}
	s.st.Deliveries = next
}

// ClaimDelivery атомарно отмечает ID доставки вебхука. false — такой ID уже был (повтор).
func (s *Store) ClaimDelivery(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slices.Contains(s.st.Deliveries, id) {
		return false, nil
	}
	old := s.st.Deliveries
	s.recordDeliveryLocked(id)
	if err := s.save(); err != nil {
		// не помним ID, который не удалось сохранить: повтор от Яндекс Форм должен пройти
		s.st.Deliveries = old
		return false, err
	}
	return true, nil
}

// ReleaseDelivery снимает отметку, если ответ не удалось принять: повтор от Яндекс Форм должен пройти.
func (s *Store) ReleaseDelivery(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, d := range s.st.Deliveries {
		if d == id {
			s.st.Deliveries = append(s.st.Deliveries[:i:i], s.st.Deliveries[i+1:]...)
			s.saveOrLog("release_delivery")
			return
		}
	}
}

// Person — участник по токену.
func (s *Store) Person(token string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.st.Tokens[token]
	return p, ok
}

// Tokens — ID участника → токен.
func (s *Store) Tokens() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]string{}
	for t, p := range s.st.Tokens {
		out[p] = t
	}
	return out
}

// Responses участника, по месяцам.
func (s *Store) Responses() []survey.Response { return s.Input().Responses }

// Open создаёт хранилище и читает снимок. fromFile — структура sv успешно прочитана из SURVEY_FILE:
// тогда она главнее структуры из снимка (см. MergeSurvey). loaded=false — снимка нет или он был повреждён.
func Open(sv survey.Survey, fromFile bool, file string) (*Store, bool, error) {
	s := New(sv, file)
	loaded, err := s.Load()
	if err != nil {
		return nil, false, err
	}
	if loaded && fromFile {
		if err := s.MergeSurvey(sv); err != nil {
			return nil, false, err
		}
	}
	return s, loaded, nil
}

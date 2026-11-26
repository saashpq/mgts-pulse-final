package store

import (
	"errors"
	"slices"

	"github.com/ElzaraA/mgts-ai/internal/survey"
)

type FormWindow struct {
	Period int  `json:"period"`
	Closed bool `json:"closed"`
}

var ErrWindowChanged = errors.New("приём за этот месяц закрыт. Обновите анкету, чтобы увидеть текущий месяц")
var ErrWindowPeriod = errors.New("выберите месяц в пределах программы стажировки")
var ErrDuplicateDelivery = errors.New("повторная доставка")

func (s *Store) formWindowLocked() FormWindow {
	if s.st.FormWindow != nil {
		w := *s.st.FormWindow
		if w.Period > s.st.Survey.Periods {
			return FormWindow{Period: s.st.Survey.Periods, Closed: true}
		}
		return w
	}
	// Старые снимки и первая загрузка Excel: по умолчанию продолжаем последний месяц с данными.
	period := 1
	for _, r := range s.st.Responses {
		if r.Period > period && r.Period <= s.st.Survey.Periods {
			period = r.Period
		}
	}
	return FormWindow{Period: period}
}

func (s *Store) FormWindow() FormWindow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.formWindowLocked()
}

func (s *Store) SetFormWindow(w FormWindow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if w.Period < 1 || w.Period > s.st.Survey.Periods {
		return ErrWindowPeriod
	}
	old := s.st.FormWindow
	s.st.FormWindow = &w
	if err := s.save(); err != nil {
		s.st.FormWindow = old
		return err
	}
	return nil
}

// SaveSubmission проверяет окно под той же блокировкой, что и запись: отправка из уже открытой
// вкладки не попадёт в закрытый месяц. До явного выбора HR авторизованный вебхук также может
// доставлять исторические ответы, как при восстановлении старых снимков.
func (s *Store) SaveSubmission(r survey.Response, note, delivery string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if delivery != "" && slices.Contains(s.st.Deliveries, delivery) {
		return nil, ErrDuplicateDelivery
	}
	if r.Source == "form" || s.st.FormWindow != nil {
		w := s.formWindowLocked()
		if w.Closed || r.Period != w.Period {
			return nil, ErrWindowChanged
		}
	}
	previous := s.st.Deliveries
	if delivery != "" {
		s.recordDeliveryLocked(delivery)
	}
	changed, err := s.upsertLocked([]survey.Response{r}, r.Source, note)
	if err != nil {
		s.st.Deliveries = previous
	}
	return changed, err
}

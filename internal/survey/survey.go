// Package survey описывает структуру анкет стажёра и наставника и единицу данных — ответ за месяц.
// Структура вынесена в data/survey.json: на финале её правят под реальный датасет МГТС, код не меняется.
package survey

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type Role string

const (
	Intern Role = "intern"
	Mentor Role = "mentor"
)

// Question — вопрос анкеты. Kind: "scale" (1..Max) или "text".
type Question struct {
	Code      string   `json:"code"`
	Role      Role     `json:"role"`
	Text      string   `json:"text"`
	Kind      string   `json:"kind"`
	Max       float64  `json:"max,omitempty"`
	Dimension string   `json:"dimension,omitempty"` // satisfaction, interest, clarity, support, performance, speed, autonomy
	Mirror    string   `json:"mirror,omitempty"`    // код парного вопроса у другой стороны (одно и то же явление глазами двоих)
	Aliases   []string `json:"aliases,omitempty"`   // варианты заголовка колонки в Excel
}

// Survey — обе анкеты и названия служебных колонок.
type Survey struct {
	FractionalHistory bool       `json:"fractional_history,omitempty"` // Legacy synthetic qualification workbook used fractional scores.
	Title             string     `json:"title"`
	Periods           int        `json:"periods"`
	StartMonth        string     `json:"start_month,omitempty"`
	Questions         []Question `json:"questions"`
	IDColumns         Columns    `json:"id_columns"`
	Participant       []Pair     `json:"pairs"`
}

// Columns — варианты заголовков служебных колонок.
type Columns struct {
	Intern []string `json:"intern"`
	Mentor []string `json:"mentor"`
	Period []string `json:"period"`
}

// Pair — пара «стажёр — наставник» (связь 1:1). Имена не храним: только ID.
type Pair struct {
	ID     string `json:"id"`
	Intern string `json:"intern"`
	Mentor string `json:"mentor"`
	Track  string `json:"track,omitempty"` // направление стажировки, если известно
	Demo   bool   `json:"demo,omitempty"`
}

// WithoutDemoPairs retains the configured real roster on a clean start.
func (s Survey) WithoutDemoPairs() Survey {
	pairs := make([]Pair, 0, len(s.Participant))
	for _, p := range s.Participant {
		if !p.Demo {
			pairs = append(pairs, p)
		}
	}
	s.Participant = pairs
	return s
}

// Response — ответ одного участника за один месяц.
type Response struct {
	ID       string             `json:"id"`
	Role     Role               `json:"role"`
	Person   string             `json:"person"`
	PairID   string             `json:"pair"`
	Period   int                `json:"period"`
	Scores   map[string]float64 `json:"scores"`
	Comments map[string]string  `json:"comments"`
	Source   string             `json:"source"` // synthetic | import | form | webhook
	At       string             `json:"at"`
}

// Load читает структуру анкет из файла; при ошибке возвращает Default.
func Load(path string) (Survey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Default(), err
	}
	var s Survey
	if err := json.Unmarshal(b, &s); err != nil {
		return Default(), fmt.Errorf("survey %s: %w", path, err)
	}
	if err := s.Validate(); err != nil {
		return Default(), err
	}
	return s, nil
}

// Validate проверяет согласованность: коды уникальны, зеркальные вопросы существуют и у другой роли.
func (s Survey) Validate() error {
	if s.StartMonth != "" {
		if _, err := time.Parse("2006-01", s.StartMonth); err != nil {
			return fmt.Errorf("survey: start_month must be YYYY-MM")
		}
	}
	if s.Periods < 1 {
		return fmt.Errorf("survey: periods must be >= 1")
	}
	seen := map[string]Question{}
	for _, q := range s.Questions {
		if q.Code == "" || (q.Role != Intern && q.Role != Mentor) {
			return fmt.Errorf("survey: bad question %q", q.Code)
		}
		if _, dup := seen[q.Code]; dup {
			return fmt.Errorf("survey: duplicate code %q", q.Code)
		}
		if q.Kind != "scale" && q.Kind != "text" {
			return fmt.Errorf("survey: %s: kind must be scale|text", q.Code)
		}
		if q.Kind == "scale" && q.Max <= 1 {
			return fmt.Errorf("survey: %s: max must be > 1", q.Code)
		}
		seen[q.Code] = q
	}
	for _, q := range s.Questions {
		if q.Mirror == "" {
			continue
		}
		m, ok := seen[q.Mirror]
		if !ok || m.Role == q.Role {
			return fmt.Errorf("survey: %s: mirror %q must be a question of the other role", q.Code, q.Mirror)
		}
	}
	pairs := map[string]bool{}
	for _, p := range s.Participant {
		if p.ID == "" || p.Intern == "" || p.Mentor == "" || pairs[p.ID] {
			return fmt.Errorf("survey: bad pair %q", p.ID)
		}
		pairs[p.ID] = true
	}
	return nil
}

// ByRole — вопросы одной роли в порядке анкеты.
func (s Survey) ByRole(r Role) []Question {
	var out []Question
	for _, q := range s.Questions {
		if q.Role == r {
			out = append(out, q)
		}
	}
	return out
}

// Q — вопрос по коду.
func (s Survey) Q(code string) (Question, bool) {
	for _, q := range s.Questions {
		if q.Code == code {
			return q, true
		}
	}
	return Question{}, false
}

// PairOf находит пару по ID участника любой роли.
func (s Survey) PairOf(person string) (Pair, Role, bool) {
	for _, p := range s.Participant {
		if strings.EqualFold(p.Intern, person) {
			return p, Intern, true
		}
		if strings.EqualFold(p.Mentor, person) {
			return p, Mentor, true
		}
	}
	return Pair{}, "", false
}

// Norm приводит оценку к шкале 1..5, чтобы пороги работали при любых шкалах анкеты (например, 1..10).
func (q Question) Norm(v float64) float64 {
	if q.Max == 5 || q.Max <= 1 {
		return v
	}
	return 1 + (v-1)*4/(q.Max-1)
}

// Default — рабочая гипотеза структуры анкет по вебинару МГТС (00:41:44): стажёра спрашивают, как проходит
// стажировка, интересны и понятны ли задачи; наставника — как стажёр справляется, оперативность, самостоятельность.
func Default() Survey {
	return Survey{
		FractionalHistory: true,
		Title:             "Ежемесячный опрос программы стажировок МГТС",
		Periods:           6,
		Questions: []Question{
			{Code: "i_overall", Role: Intern, Kind: "scale", Max: 5, Dimension: "satisfaction", Mirror: "m_overall",
				Text: "Как в целом проходит стажировка?", Aliases: []string{"как проходит стажировка", "общая оценка", "удовлетворенность"}},
			{Code: "i_interest", Role: Intern, Kind: "scale", Max: 5, Dimension: "interest",
				Text: "Насколько вам интересны задачи?", Aliases: []string{"интерес", "интересны ли задачи"}},
			{Code: "i_clarity", Role: Intern, Kind: "scale", Max: 5, Dimension: "clarity", Mirror: "m_understanding",
				Text: "Насколько понятны поставленные задачи?", Aliases: []string{"понятность", "понятны ли задачи"}},
			{Code: "i_support", Role: Intern, Kind: "scale", Max: 5, Dimension: "support",
				Text: "Хватает ли вам поддержки наставника?", Aliases: []string{"поддержка", "поддержка наставника"}},
			{Code: "i_comment", Role: Intern, Kind: "text",
				Text: "Что получилось, что мешает, что хотели бы изменить?", Aliases: []string{"комментарий", "комментарий стажера"}},
			{Code: "m_overall", Role: Mentor, Kind: "scale", Max: 5, Dimension: "performance", Mirror: "i_overall",
				Text: "Как стажёр справляется с работой?", Aliases: []string{"как справляется", "общая оценка стажера"}},
			{Code: "m_speed", Role: Mentor, Kind: "scale", Max: 5, Dimension: "speed",
				Text: "Насколько оперативно стажёр выполняет задачи?", Aliases: []string{"оперативность"}},
			{Code: "m_autonomy", Role: Mentor, Kind: "scale", Max: 5, Dimension: "autonomy",
				Text: "Насколько стажёр самостоятелен?", Aliases: []string{"самостоятельность"}},
			{Code: "m_understanding", Role: Mentor, Kind: "scale", Max: 5, Dimension: "clarity", Mirror: "i_clarity",
				Text: "Насколько стажёр понимает поставленные задачи?", Aliases: []string{"понимание задач"}},
			{Code: "m_comment", Role: Mentor, Kind: "text",
				Text: "Комментарий наставника о работе стажёра", Aliases: []string{"комментарий", "комментарий наставника"}},
		},
		IDColumns: Columns{
			Intern: []string{"intern_id", "id стажера", "стажер", "стажер id"},
			Mentor: []string{"mentor_id", "id наставника", "наставник", "наставник id"},
			Period: []string{"period", "месяц", "период", "month", "номер месяца"},
		},
		Participant: []Pair{
			{ID: "P1", Intern: "I-01", Mentor: "M-01", Track: "Аналитика", Demo: true},
			{ID: "P2", Intern: "I-02", Mentor: "M-02", Track: "Автоматизация процессов", Demo: true},
			{ID: "P3", Intern: "I-03", Mentor: "M-03", Track: "Разработка", Demo: true},
			{ID: "P4", Intern: "I-04", Mentor: "M-04", Track: "ПТО", Demo: true},
			{ID: "P5", Intern: "I-05", Mentor: "M-05", Track: "Клиентский сервис", Demo: true},
		},
	}
}

package signals

import (
	"context"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/survey"
	"github.com/ElzaraA/mgts-ai/internal/textai"
)

func TestCriticalSignalUsesLatestRoleAndMatchingQuestion(t *testing.T) {
	in := synthInput()
	in.Survey.Questions = append(in.Survey.Questions, survey.Question{Role: survey.Mentor, Code: "m_other", Kind: "text"})
	for _, r := range in.Responses {
		if r.PairID == "P3" && r.Period == 6 && r.Role == survey.Mentor {
			r.Comments["m_other"] = "Отличный проект."
			in.Analyses[r.ID+"/m_other"] = textai.Offline{}.Analyze(context.Background(), r.Comments["m_other"], "mentor")
		}
	}
	for _, s := range pairOf(Analyze(in, nil), "P3").Signals {
		if s.Type != "S10" {
			continue
		}
		var quoted, numeric bool
		for _, e := range s.Evidence {
			quoted = quoted || e.Role == "mentor" && e.Period == 6 && e.Question == "m_comment" && e.Quote == "Не вижу его в команде после стажировки."
			numeric = numeric || e.ResponseID != "" && e.Value > 0 && e.Period >= 5
		}
		if s.LastPeriod != 6 || s.Title != "Сигнал о возможном конфликте" || !quoted || !numeric {
			t.Fatalf("wrong evidence for current risk: %+v", s)
		}
		return
	}
	t.Fatal("missed the current mentor report")
}

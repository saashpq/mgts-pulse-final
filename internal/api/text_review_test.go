package api

import (
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/signals"
	"github.com/ElzaraA/mgts-ai/internal/survey"
	"github.com/ElzaraA/mgts-ai/internal/textai"
)

func TestTextReviewCountsEachUnclassifiedComment(t *testing.T) {
	in := signals.Input{
		Responses: []survey.Response{
			{ID: "a", PairID: "P1", Role: survey.Intern, Period: 1, Comments: map[string]string{"q1": "Комментарий", "q2": "  "}},
			{ID: "b", PairID: "P2", Role: survey.Mentor, Period: 2, Comments: map[string]string{"q1": "Первый", "q2": "Второй", "q3": "Третий"}},
		},
		Analyses: map[string]textai.Analysis{"b/q1": {Themes: []string{"growth"}}},
	}
	got := textReview(in, signals.ProgramReport{Current: 2})
	if got.Total != 4 || got.Unclassified != 3 || len(got.Rows) != 2 || got.Rows[0] != (reviewRow{"P2", 2, survey.Mentor, 2}) {
		t.Fatalf("%+v", got)
	}
}

func TestCriticalCommentHasSeparateManualReview(t *testing.T) {
	in := signals.Input{Responses: []survey.Response{
		{ID: "r1", PairID: "P1", Role: survey.Intern, Period: 6, Comments: map[string]string{"note": "Хочу прекратить стажировку."}},
		{ID: "r2", PairID: "P1", Role: survey.Mentor, Period: 4, Comments: map[string]string{"note": "Коллеги унижали меня."}},
		{ID: "r3", PairID: "P2", Role: survey.Mentor, Period: 6, Comments: map[string]string{"note": "Коллеги унижали меня."}},
	}, Analyses: map[string]textai.Analysis{
		"r1/note": {Themes: []string{"attrition"}, Quote: "Хочу прекратить стажировку."},
		"r2/note": {Themes: []string{"conflict"}, Quote: "Коллеги унижали меня."},
		"r3/note": {Themes: []string{"conflict"}, Quote: "Коллеги унижали меня."},
	}}
	report := signals.ProgramReport{Current: 6, Pairs: []signals.PairReport{
		{PairID: "P1"}, {PairID: "P2", Signals: []signals.SignalView{{Signal: signals.Signal{Type: "S10"}}}},
	}}
	got := textReview(in, report)
	want := criticalComment{"P1", 6, survey.Intern, "note"}
	if len(got.Critical) != 1 || got.Critical[0] != want || got.Unclassified != 0 {
		t.Fatalf("%+v", got)
	}
}

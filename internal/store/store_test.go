package store

import (
	"path/filepath"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/signals"
	"github.com/ElzaraA/mgts-ai/internal/survey"
	"github.com/ElzaraA/mgts-ai/internal/synth"
	"github.com/ElzaraA/mgts-ai/internal/textai"
)

func TestPersistAndIdempotency(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	sv := survey.Default()
	s := New(sv, file)
	rs := synth.Responses(sv)
	if err := s.Reset(sv, rs, "synthetic"); err != nil {
		t.Fatal(err)
	}
	if ch, _ := s.Upsert(rs, "import", ""); len(ch) != 0 {
		t.Fatalf("same data twice must change nothing: %d", len(ch))
	}
	// импорт без части колонок не затирает известные значения
	partial := rs[0]
	partial.Scores = map[string]float64{"i_overall": 1}
	partial.Comments = nil
	ch, _ := s.Upsert([]survey.Response{partial}, "import", "")
	if len(ch) != 1 {
		t.Fatal("changed value must be saved")
	}
	got := s.Responses()[0]
	if got.Scores["i_overall"] != 1 || got.Scores["i_interest"] == 0 || len(got.Comments) == 0 {
		t.Fatalf("merge: %+v", got)
	}
	s.SetAnalysis("k", textai.Analysis{Sentiment: -1})
	_, _ = s.SetVerdict("P2-S7", "confirmed", "ok")
	th := signals.Default
	th.GapMin = 1.2
	_ = s.SetThresholds(th)
	tok := s.Tokens()["I-01"]

	s2 := New(sv, file)
	if ok, err := s2.Load(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if len(s2.Responses()) != len(rs) || s2.Verdicts()["P2-S7"].Status != "confirmed" || s2.Thresholds().GapMin != 1.2 {
		t.Fatal("state not restored")
	}
	if p, ok := s2.Person(tok); !ok || p != "I-01" {
		t.Fatal("form links must survive restart")
	}
	if a, ok := s2.Analysis("k"); !ok || a.Sentiment != -1 {
		t.Fatal("analysis")
	}
	if _, _ = s2.SetVerdict("P2-S7", "", ""); len(s2.Verdicts()) != 0 {
		t.Fatal("verdict removal")
	}
	if len(s2.Events()) == 0 {
		t.Fatal("journal")
	}
}

func TestMemoryOnly(t *testing.T) {
	s := New(survey.Default(), "")
	if ok, err := s.Load(); ok || err != nil {
		t.Fatal("no file — nothing to load")
	}
	if len(s.Tokens()) != 10 {
		t.Fatalf("each participant gets a link: %d", len(s.Tokens()))
	}
}

func TestDeliveries(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	s := New(survey.Default(), file)
	steps := []struct {
		op, id string
		fresh  bool
	}{
		{"claim", "d1", true},
		{"claim", "d1", false},
		{"claim", "d2", true},
		{"release", "d1", false},
		{"claim", "d1", true}, // после неудачной обработки повтор принимается
		{"release", "nope", false},
	}
	for i, st := range steps {
		switch st.op {
		case "claim":
			if got, err := s.ClaimDelivery(st.id); err != nil || got != st.fresh {
				t.Fatalf("step %d: %v %v", i, got, err)
			}
		case "release":
			s.ReleaseDelivery(st.id)
		}
	}
	// переживает перезапуск
	s2 := New(survey.Default(), file)
	if ok, err := s2.Load(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if fresh, _ := s2.ClaimDelivery("d2"); fresh {
		t.Fatal("delivery id must persist")
	}
	// хранится не больше MaxDeliveries: самые старые вытесняются
	for i := 0; i < MaxDeliveries; i++ {
		if _, err := s2.ClaimDelivery("x" + itoa(i)); err != nil {
			t.Fatal(err)
		}
	}
	if fresh, _ := s2.ClaimDelivery("d2"); !fresh {
		t.Fatal("oldest id must be evicted")
	}
	if fresh, _ := s2.ClaimDelivery("x499"); fresh {
		t.Fatal("recent id must be kept")
	}
	if n := len(s2.st.Deliveries); n != MaxDeliveries {
		t.Fatalf("kept %d", n)
	}
}

// Выгрузка не затирает более свежий ответ, пришедший из анкеты.
func TestImportKeepsFormAnswer(t *testing.T) {
	sv := survey.Default()
	s := New(sv, "")
	rs := synth.Responses(sv)
	if err := s.Reset(sv, rs, "synthetic"); err != nil {
		t.Fatal(err)
	}
	form := rs[0]
	form.Scores = map[string]float64{"i_overall": 1, "i_interest": 1, "i_clarity": 1, "i_support": 1}
	form.Source = "form"
	if ch, _ := s.Upsert([]survey.Response{form}, "form", ""); len(ch) != 1 {
		t.Fatal("form answer must be saved")
	}
	if ch, _ := s.Upsert(rs[:1], "import", ""); len(ch) != 0 {
		t.Fatal("import must not overwrite a form answer")
	}
	for _, r := range s.Responses() {
		if r.ID == form.ID && r.Scores["i_overall"] != 1 {
			t.Fatal("form answer lost")
		}
	}
}

// Выученные при импорте заголовки (remember) не теряются при перезапуске с survey.json, где их нет.
func TestMergeSurveyKeepsLearnedAliases(t *testing.T) {
	learned := survey.Default()
	learned.Questions = append([]survey.Question(nil), learned.Questions...)
	learned.Questions[2].Aliases = append(append([]string(nil), learned.Questions[2].Aliases...), "Ясность задач")
	learned.IDColumns.Intern = append(append([]string(nil), learned.IDColumns.Intern...), "Код практиканта")
	s := New(learned, "")
	file := survey.Default()
	if err := s.MergeSurvey(file); err != nil {
		t.Fatal(err)
	}
	got := s.Survey()
	if q, _ := got.Q("i_clarity"); q.Aliases[len(q.Aliases)-1] != "Ясность задач" || len(q.Aliases) != 3 {
		t.Fatalf("%v", q.Aliases)
	}
	if ids := got.IDColumns.Intern; ids[len(ids)-1] != "Код практиканта" || len(ids) != len(file.IDColumns.Intern)+1 {
		t.Fatalf("%v", ids)
	}
	if q, _ := file.Q("i_clarity"); len(q.Aliases) != 2 {
		t.Fatal("file survey must not change in place")
	}
}

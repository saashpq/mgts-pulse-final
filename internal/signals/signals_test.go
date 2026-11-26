package signals

import (
	"context"
	"math"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/survey"
	"github.com/ElzaraA/mgts-ai/internal/synth"
	"github.com/ElzaraA/mgts-ai/internal/textai"
)

func synthInput() Input {
	sv := survey.Default()
	rs := synth.Responses(sv)
	an := map[string]textai.Analysis{}
	for _, r := range rs {
		for c, txt := range r.Comments {
			an[r.ID+"/"+c] = textai.Offline{}.Analyze(context.Background(), txt, string(r.Role))
		}
	}
	return Input{Survey: sv, Responses: rs, Analyses: an, T: Default}
}

func pairOf(rep ProgramReport, id string) PairReport {
	for _, p := range rep.Pairs {
		if p.PairID == id {
			return p
		}
	}
	return PairReport{}
}

func has(p PairReport, typ string) bool {
	for _, s := range p.Signals {
		if s.Type == typ {
			return true
		}
	}
	return false
}

// Каждый сценарий синтетики ловится своим сигналом, а контрольная пара не поднимает тревогу.
func TestScenarios(t *testing.T) {
	rep := Analyze(synthInput(), nil)
	cases := []struct {
		pair, status string
		must         []string
	}{
		{"P1", "ok", nil},
		{"P2", "attention", []string{"S7", "S9", "S10", "S3"}},
		{"P3", "attention", []string{"S1", "S6", "S8", "S10"}},
		{"P4", "attention", []string{"S3", "S4", "S8"}},
		{"P5", "ok", []string{"S11", "S12"}},
	}
	for _, c := range cases {
		p := pairOf(rep, c.pair)
		if p.Status != c.status {
			t.Errorf("%s: status %s, want %s", c.pair, p.Status, c.status)
		}
		for _, typ := range c.must {
			if !has(p, typ) {
				t.Errorf("%s: no %s; got %+v", c.pair, typ, p.Signals)
			}
		}
	}
	if p := pairOf(rep, "P1"); len(p.Signals) != 0 || len(p.Resolved) != 0 {
		t.Errorf("control pair must stay quiet: %+v %+v", p.Signals, p.Resolved)
	}
	// провал P5 в M3 виден в истории и снят после восстановления
	p5 := pairOf(rep, "P5")
	if p5.History[2] != "high" || len(p5.Resolved) == 0 {
		t.Errorf("P5 dip must be in history and resolved: %v %v", p5.History, p5.Resolved)
	}
	if rep.Attention != 3 || rep.EarlierOrOnly < 3 {
		t.Errorf("attention %d, earlier %d", rep.Attention, rep.EarlierOrOnly)
	}
}

// Вывод ИИ всегда опирается на дословную цитату из ответа.
func TestEvidenceQuotesAreVerbatim(t *testing.T) {
	in := synthInput()
	text := map[string]string{}
	for _, r := range in.Responses {
		for c, s := range r.Comments {
			text[r.ID+"/"+c] = s
		}
	}
	for _, p := range Analyze(in, nil).Pairs {
		for _, s := range append(p.Signals, p.Resolved...) {
			for _, e := range s.Evidence {
				if e.Quote == "" {
					continue
				}
				if src := text[e.ResponseID+"/"+e.Question]; !contains(src, e.Quote) {
					t.Errorf("%s: quote %q not in %q", s.ID, e.Quote, src)
				}
			}
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Решение HR «ложный» снимает пару со статуса «требует вмешательства», если других сигналов нет.
func TestFalseVerdictSuppresses(t *testing.T) {
	in := synthInput()
	rep := Analyze(in, nil)
	v := map[string]Verdict{}
	for _, s := range pairOf(rep, "P4").Signals {
		v[s.ID] = Verdict{Status: "false"}
	}
	rep2 := Analyze(in, v)
	if p := pairOf(rep2, "P4"); p.Status != "ok" || rep2.FalsePositive == 0 {
		t.Fatalf("got %s, fp %d", p.Status, rep2.FalsePositive)
	}
}

// Пороги влияют на результат: при очень строгом пороге падения S1 у P2 пропадает.
func TestThresholdsApply(t *testing.T) {
	in := synthInput()
	in.T.DeclineSlope, in.T.ZDrop = -2, 5
	if p := pairOf(Analyze(in, nil), "P2"); has(p, "S1") {
		t.Fatal("S1 must disappear with strict thresholds")
	}
}

func TestSlopeAndRuns(t *testing.T) {
	if v, ok := Slope([]float64{5, 4, 3}, 3); !ok || math.Abs(v+1) > 1e-9 {
		t.Fatalf("slope %v", v)
	}
	if _, ok := Slope([]float64{5, NaN, 3}, 3); ok {
		t.Fatal("2 points must not give 3-point slope")
	}
	f, l, g, ok := runOf([]float64{4, 4, 5, 5, 5, 5}, []float64{4, 4, 3.5, 3.5, 3.4, 3.4}, 2, func(a, b float64) bool { return math.Abs(a-b) >= 1 })
	if !ok || f != 2 || l != 5 || g < 1.59 {
		t.Fatalf("run %d %d %v", f, l, g)
	}
	if _, _, _, ok := runOf([]float64{4, 5, 4}, []float64{4, 3, 4}, 2, func(a, b float64) bool { return math.Abs(a-b) >= 1 }); ok {
		t.Fatal("single-month gap is noise")
	}
}

func TestEmptyProgram(t *testing.T) {
	rep := Analyze(Input{Survey: survey.Default(), T: Default}, nil)
	if rep.Current != 0 || rep.Digest == "" {
		t.Fatalf("%+v", rep)
	}
}

package signals

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestTheilSen(t *testing.T) {
	cases := []struct {
		s    []float64
		n    int
		want float64
		ok   bool
	}{
		// медиана 6 наклонов {0, −0.5, −0.67, −1, −1, −1} = −0.83 (в таблице §8 methodology ошибочно −0.5)
		{[]float64{5, 5, 4, 3}, 4, -5.0 / 6, true},
		{[]float64{4, 4, 2, 4}, 4, 0, true}, // один провал не делает тренд
		{[]float64{5, NaN, 4, 3, 2}, 4, -0.875, true},
		{[]float64{5, NaN, 4, 3}, 4, 0, false},
		{[]float64{5}, 1, 0, false},
		{[]float64{6, 5, NaN, 3}, 3, -1, true}, // индексы месяцев учитывают пропуск
	}
	for _, c := range cases {
		got, ok := TheilSen(c.s, c.n)
		if ok != c.ok || (ok && !near(got, c.want, 1e-9)) {
			t.Errorf("TheilSen(%v, %d) = %v %v, want %v %v", c.s, c.n, got, ok, c.want, c.ok)
		}
	}
}

func TestBaselineAndZ(t *testing.T) {
	if b, n, ok := Baseline([]float64{4, NaN, 5, 3}, 3); !ok || b != 4.5 || n != 2 {
		t.Fatalf("baseline %v %d %v", b, n, ok)
	}
	if _, n, ok := Baseline([]float64{4, NaN, 5}, 2); ok || n != 1 {
		t.Fatal("one point is not a baseline")
	}
	if b, _, ok := Baseline([]float64{4, 5, 3}, 10); !ok || b != 4 {
		t.Fatalf("t beyond length: %v", b)
	}
	if !math.IsNaN(median(nil)) || median([]float64{1, 2, 3, 4}) != 2.5 {
		t.Fatal("median")
	}
	// без личного разброса σ̃² = 4σ̂²/(4+1) при n_p = 2
	z, st := WithinZ(3, 4, 0, 2, 0.5)
	if !near(st, math.Sqrt(0.2), 1e-9) || !near(z, -1/math.Sqrt(0.2), 1e-9) {
		t.Fatalf("z %v σ %v", z, st)
	}
	if _, st := WithinZ(3, 4, 0, 2, 0.1); st != MinSigma {
		t.Fatalf("floor: %v", st)
	}
	if _, st := WithinZ(3, 4, NaN, 0, 0.5); !near(st, 0.5, 1e-9) {
		t.Fatalf("no personal points → pooled σ: %v", st)
	}
	if sdOf([]float64{4}) != 0 || !near(sdOf([]float64{3, 5}), math.Sqrt2, 1e-9) {
		t.Fatal("sd")
	}
}

func TestNoiseSigma(t *testing.T) {
	flat := map[string]*PairView{"a": {InternIndex: []float64{4, 4, 4}, MentorIndex: []float64{5, 5, NaN}}}
	if NoiseSigma(flat) != MinSigma || NoiseSigma(map[string]*PairView{}) != MinSigma {
		t.Fatal("constant series → floor 0.25")
	}
	// разности 1 и −1: √((1+1)/(2·2)) = √0.5
	v := map[string]*PairView{"a": {InternIndex: []float64{4, 5, 4}, MentorIndex: []float64{NaN, NaN, NaN}}}
	if !near(NoiseSigma(v), math.Sqrt(0.5), 1e-9) {
		t.Fatalf("%v", NoiseSigma(v))
	}
}

func TestDeclineBranches(t *testing.T) {
	// резкий провал: база 4.5, σ̂ = 0.3 → z ≈ −4.7
	d, ok := Decline([]float64{4.5, 4.5, 3}, 0.3, Default)
	if !ok || !d.Drop || d.Trend || d.At != 2 {
		t.Fatalf("%+v", d)
	}
	// устойчивое снижение без резкого провала
	d, _ = Decline([]float64{4.75, 4.75, 4.25, 3.75}, 0.5, Default)
	if d.Drop || !d.Trend || d.From != 0 || !near(d.Slope, -5.0/12, 1e-9) {
		t.Fatalf("%+v", d)
	}
	if _, ok := Decline([]float64{4, NaN}, 0.3, Default); ok {
		t.Fatal("no baseline")
	}
	if _, ok := Decline([]float64{NaN, NaN}, 0.3, Default); ok {
		t.Fatal("empty")
	}
}

// Ожидания контракта на синтетике: P1 — ни одной S1/S2, P2 — S1 не позже M4, P5 — S1 в M3 и снят к M5.
func TestDeclineOnSynthetic(t *testing.T) {
	in := synthInput()
	rep := Analyze(in, nil)
	all := func(id string) []SignalView {
		p := pairOf(rep, id)
		return append(append([]SignalView{}, p.Signals...), p.Resolved...)
	}
	for _, s := range all("P1") {
		t.Errorf("P1 must be quiet: %s", s.ID)
	}
	found := map[string]SignalView{}
	for _, id := range []string{"P2", "P5"} {
		for _, s := range all(id) {
			found[s.ID] = s
		}
	}
	if s, ok := found["P2-S1"]; !ok || s.FirstSeen > 4 || !strings.Contains(s.Explanation, "устойчивое снижение") {
		t.Fatalf("P2-S1: %+v", s)
	}
	s, ok := found["P5-S1"]
	if !ok || s.FirstSeen != 3 || s.Severity != High || !strings.Contains(s.Explanation, "резкий провал: z = -") {
		t.Fatalf("P5-S1: %+v", s)
	}
	for _, a := range pairOf(rep, "P5").Signals {
		if a.Type == "S1" {
			t.Fatal("P5-S1 must be resolved")
		}
	}
	v := Build(in)["P5"]
	for _, s := range Detect(v, in, 5) {
		if s.Type == "S1" {
			t.Fatal("P5-S1 must be gone by M5")
		}
	}
}

func TestThresholdsBackCompat(t *testing.T) {
	var th Thresholds
	old := `{"decline_slope":-0.3,"decline_from_peak":0.8,"low_score":3,"gap_min":1.2}`
	if err := json.Unmarshal([]byte(old), &th); err != nil {
		t.Fatal(err)
	}
	if th.DeclineSlope != -0.3 || th.GapMin != 1.2 || th.ZDrop != 3 || th.DeclineFromBase != 0.5 || th.CostRatio != 10 || th.Persist != 2 {
		t.Fatalf("%+v", th)
	}
	if err := json.Unmarshal([]byte(`{"z_drop":"x"}`), &th); err == nil {
		t.Fatal("bad type must fail")
	}
	b, _ := json.Marshal(Default)
	if strings.Contains(string(b), "decline_from_peak") || !strings.Contains(string(b), `"cost_ratio":10`) {
		t.Fatal(string(b))
	}
}

func TestClopperPearson(t *testing.T) {
	cases := []struct {
		x, n   int
		lo, hi float64
	}{
		{4, 5, 0.284, 0.995}, {5, 5, 0.478, 1}, {0, 5, 0, 0.522}, {4, 4, 0.398, 1}, {9, 10, 0.555, 0.997}, {45, 50, 0.782, 0.967},
	}
	for _, c := range cases {
		lo, hi := ClopperPearson(c.x, c.n)
		if !near(lo, c.lo, 1e-3) || !near(hi, c.hi, 1e-3) {
			t.Errorf("%d/%d → [%.4f; %.4f], want [%.3f; %.3f]", c.x, c.n, lo, hi, c.lo, c.hi)
		}
	}
	if lo, hi := ClopperPearson(1, 0); lo != 0 || hi != 1 {
		t.Fatal("degenerate")
	}
	if binomCDF(-1, 5, 0.5) != 0 || binomCDF(5, 5, 0.5) != 1 || binomCDF(2, 5, 0) != 1 || binomCDF(2, 5, 1) != 0 {
		t.Fatal("cdf edges")
	}
	if r := NewRate(0, 0); r.Value != nil || r.Lo != nil {
		t.Fatal("empty rate")
	}
	if r := NewRate(4, 5); *r.Value != 0.8 || *r.Lo != 0.284 || *r.Hi != 0.995 {
		t.Fatalf("%+v", r)
	}
}

// Подсказка «загрубить» — и когда даже верхняя граница ДИ ниже 0.5 (0/5: hi = 0.52 — ещё нет; 0/6: 0.46 — да).
func TestHintByInterval(t *testing.T) {
	mk := func(typ string, good, bad int) []PairReport {
		var out []PairReport
		for i := 0; i < good+bad; i++ {
			st := "confirmed"
			if i >= good {
				st = "false"
			}
			out = append(out, PairReport{Signals: []SignalView{{Signal: Signal{ID: typ + string(rune('a'+i)), Type: typ}, Verdict: &Verdict{Status: st}}}})
		}
		return out
	}
	by := func(ps []Precision, typ string) Precision {
		for _, p := range ps {
			if p.Type == typ {
				return p
			}
		}
		return Precision{}
	}
	p := by(PrecisionByType(mk("S2", 3, 2), Default), "S2")
	if p.Hint != "" || p.Lo == nil || *p.Hi != 0.947 {
		t.Fatalf("3/5 is not a reason to coarsen: %+v", p)
	}
	p = by(PrecisionByType(mk("S2", 2, 3), Default), "S2")
	if p.Suggest == nil || p.Suggest.Field != "decline_from_base" || p.Suggest.To != 0.6 {
		t.Fatalf("%+v", p)
	}
	tot := totalPrecision(PrecisionByType(mk("S3", 4, 1), Default))
	if *tot.Lo != 0.284 || *tot.Hi != 0.995 {
		t.Fatalf("%+v", tot)
	}
}

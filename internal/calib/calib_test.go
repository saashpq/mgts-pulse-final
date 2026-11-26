package calib

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestFire(t *testing.T) {
	cases := []struct {
		r    Rule
		s    []float64
		want bool
	}{
		{Rule{Kind: "current", Slope: -0.25, FromBase: 0.8}, []float64{4, 4.5, 4}, false}, // slope₃ = 0, пик − последнее = 0.5
		{Rule{Kind: "current", Slope: -0.25, FromBase: 0.8}, []float64{4, 4, 4}, false},
		{Rule{Kind: "current", Slope: -0.25, FromBase: 0.8}, []float64{5, 4.5, 4.2}, true},
		{Rule{Kind: "naive"}, []float64{4, 2.75}, true},
		{Rule{Kind: "z", Z: 3}, []float64{4.5, 4.5, 3}, true},
		{Rule{Kind: "trend", Slope: -0.25, FromBase: 0.5}, []float64{4.5, 4.5, 3}, false},
		{Rule{Kind: "trend", Slope: -0.25, FromBase: 0.5}, []float64{4.75, 4.75, 4.25, 3.75}, true},
		{Rule{Kind: "new", Z: 3, Slope: -0.25, FromBase: 0.5}, []float64{4, 4, 4}, false},
	}
	for i, c := range cases {
		if got := c.r.Fire(c.s, 0.3); got != c.want {
			t.Errorf("%d %s %v: %v", i, c.r.Kind, c.s, got)
		}
	}
}

// Имитация воспроизводит порядок чисел methodology §3.3 и детерминирована при одном seed.
func TestRunMatchesMethodology(t *testing.T) {
	p := DefaultParams()
	p.N = 1500
	res := Run(p, Grid())
	by := func(kind string, z, sl float64) Row {
		for _, r := range res.Rows {
			if r.Rule.Kind == kind && r.Rule.Z == z && r.Rule.Slope == sl {
				return r
			}
		}
		t.Fatalf("no rule %s %v %v", kind, z, sl)
		return Row{}
	}
	cur, def, strict := by("current", 0, -0.25), by("new", 3, -0.25), by("new", 3, -0.375)
	if v := *cur.FPR.Value; v < 0.55 || v > 0.72 {
		t.Errorf("current FPR %v, methodology 0.63", v)
	}
	if v := *def.FPR.Value; v < 0.1 || v > 0.2 || !def.Rule.Default {
		t.Errorf("default FPR %v, methodology 0.16", v)
	}
	if v := *strict.FPR.Value; v > 0.07 {
		t.Errorf("strict FPR %v, methodology 0.04", v)
	}
	if *cur.Step.Value < 0.99 || *def.Grad.Value < 0.9 || def.StepMonth == nil || *def.StepMonth != 3 {
		t.Errorf("recall: %+v", def)
	}
	if res.Sigma < 0.28 || res.Sigma > 0.36 || len(res.Best) != 3 {
		t.Errorf("sigma %v best %v", res.Sigma, res.Best)
	}
	again := Run(p, Grid()[:1])
	if *again.Rows[0].FPR.Value != *cur.FPR.Value {
		t.Error("same seed must give the same numbers")
	}
	var csv, md bytes.Buffer
	if err := WriteCSV(&csv, res); err != nil || strings.Count(csv.String(), "\n") != len(res.Rows)+1 || !strings.Contains(csv.String(), "cost_R10") {
		t.Fatal(csv.String())
	}
	if err := WriteMarkdown(&md, res); err != nil || !strings.Contains(md.String(), "(умолчание)") || !strings.Contains(md.String(), "★") ||
		!strings.Contains(md.String(), "Сверка с таблицей methodology") {
		t.Fatal(md.String())
	}
	if month(nil) != "—" || fnum(3.0) != "3" || fnum(-0.375) != "-0.375" {
		t.Fatal("format")
	}
}

type failWriter struct{ after int }

func (f *failWriter) Write(p []byte) (int, error) {
	if f.after <= 0 {
		return 0, errors.New("disk full")
	}
	f.after--
	return len(p), nil
}

func TestWriteErrors(t *testing.T) {
	p := DefaultParams()
	p.N = 50
	res := Run(p, Grid()[:2])
	res.Rows[0].StepMonth = nil
	if WriteCSV(&failWriter{0}, res) == nil || WriteCSV(&failWriter{1}, res) == nil || WriteMarkdown(&failWriter{0}, res) == nil {
		t.Fatal("write errors must surface")
	}
	var b bytes.Buffer
	if err := WriteCSV(&b, res); err != nil {
		t.Fatal(err)
	}
}

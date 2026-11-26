package synth

import (
	"math"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/survey"
)

func TestResponsesAndOnset(t *testing.T) {
	rs := Responses(survey.Default())
	if len(rs) != 59 { // 5 пар × 6 месяцев × 2 роли, у P5 пропуск стажёра в M5
		t.Fatalf("%d", len(rs))
	}
	if len(Onset["P1"]) != 0 || len(Onset) != 5 || Onset["P5"][0].T0 != 3 {
		t.Fatalf("%v", Onset)
	}
}

func TestRandomDeterministicAndScenarios(t *testing.T) {
	a := Random(1, RandomParams{Kind: "stable", Pairs: 50})
	b := Random(1, RandomParams{Kind: "stable", Pairs: 50})
	for i := range a {
		for j := range a[i] {
			if a[i][j] != b[i][j] || a[i][j] < 1 || a[i][j] > 5 || math.Mod(a[i][j]*4, 1) != 0 {
				t.Fatalf("same seed, same series, step 0.25 on 1..5: %v %v", a[i], b[i])
			}
		}
	}
	p := RandomParams{Kind: "grad"}
	p.defaults()
	if p.Shift(2) != 0 || p.Shift(3) != -0.4 || math.Abs(p.Shift(6)+1.6) > 1e-9 {
		t.Fatal("grad")
	}
	p.Kind = "step"
	if p.Shift(5) != -1 || p.Shift(1) != 0 {
		t.Fatal("step")
	}
	// ступенька −1 с M3: среднее по 2000 рядам после t0 ниже на ≈ 1 (обрезка 1..5 чуть сжимает сдвиг)
	before, after := 0.0, 0.0
	for _, s := range Random(3, RandomParams{Kind: "step", Pairs: 2000}) {
		before += (s[0] + s[1]) / 2
		after += (s[3] + s[4]) / 2
	}
	if d := (before - after) / 2000; d < 0.85 || d > 1.05 {
		t.Fatalf("step size %v", d)
	}
}

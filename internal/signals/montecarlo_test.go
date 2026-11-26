package signals

import (
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/survey"
	"github.com/ElzaraA/mgts-ai/internal/synth"
)

// Монте-Карло: 1000 стабильных пар (шум пункта 0.6, seed фиксирован), доля пар с ложной S1 хотя бы раз за M3–M6.
// Контракт R4 требует ≤ 0.06, но при наклоне −0.25 (умолчание контракта) имитация даёт ≈ 0.13 — так же, как
// таблица methodology §3.3 (0.16). ≤ 0.06 достигается вариантом −0.375 (там же: 0.04). Умолчание −0.25 оставлено:
// при C_FN/C_FP = 10 оно дешевле по ожидаемой стоимости (docs/calibration.md). Тест фиксирует оба числа.
func TestMonteCarloFalseS1(t *testing.T) {
	const n = 1000
	interns := synth.Random(7, synth.RandomParams{Kind: "stable", Pairs: n})
	mentors := synth.Random(8, synth.RandomParams{Kind: "stable", Pairs: n})
	views := map[string]*PairView{}
	for i := 0; i < n; i++ {
		views[string(rune(i))] = &PairView{Pair: survey.Pair{}, InternIndex: interns[i], MentorIndex: mentors[i]}
	}
	sigma := NoiseSigma(views)
	if sigma < 0.25 || sigma > 0.4 {
		t.Fatalf("sigma %v: index SD with item noise 0.6 must be ≈ 0.3", sigma)
	}
	share := func(th Thresholds) float64 {
		falsePairs := 0
		for _, s := range interns {
			for cut := 3; cut <= 6; cut++ {
				if d, ok := Decline(s[:cut], sigma, th); ok && (d.Drop || d.Trend) {
					falsePairs++
					break
				}
			}
		}
		return float64(falsePairs) / n
	}
	strict := Default
	strict.DeclineSlope = -0.375
	def, str := share(Default), share(strict)
	t.Logf("sigma %.3f, false S1 share: default %.3f, slope −0.375 %.3f", sigma, def, str)
	if str > 0.06 {
		t.Fatalf("strict variant: false S1 share %.3f > 0.06", str)
	}
	if def < 0.08 || def > 0.2 {
		t.Fatalf("default: false S1 share %.3f, methodology §3.3 expects ≈ 0.16", def)
	}
}

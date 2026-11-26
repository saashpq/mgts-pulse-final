package signals

import (
	"math"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/survey"
)

func TestItemDropUsesThePreviousMonthAndResolvesOnRecovery(t *testing.T) {
	n := math.NaN()
	for _, tc := range []struct {
		values      []float64
		base, onset int
	}{
		{[]float64{5, 5, 4, 2}, 2, 3},
		{[]float64{5, 5, 4, 2, 3}, 2, 3},
		{[]float64{5, 5, 4, 2, 4}, 4, -1},
		{[]float64{4, 3, 2}, 0, 2},
		{[]float64{5, n, 2}, 2, -1},
	} {
		base, onset := itemDropEpisode(tc.values, 2)
		if base != tc.base || onset != tc.onset {
			t.Errorf("%v: got (%d,%d)", tc.values, base, onset)
		}
	}
}

func TestAgreementIgnoresUnmatchedAutonomyScore(t *testing.T) {
	spec := survey.Default()
	for i := range spec.Questions {
		if spec.Questions[i].Dimension != "clarity" {
			spec.Questions[i].Mirror = ""
		}
	}
	in := Input{Survey: spec, T: Default, Responses: []survey.Response{
		{ID: "one", PairID: "P1", Role: survey.Intern, Period: 1, Scores: map[string]float64{"i_clarity": 2, "i_interest": 5}},
		{ID: "two", PairID: "P1", Role: survey.Mentor, Period: 1, Scores: map[string]float64{"m_understanding": 4, "m_autonomy": 1}},
		{ID: "three", PairID: "P1", Role: survey.Mentor, Period: 2, Scores: map[string]float64{"m_autonomy": 5}},
	}}
	v := Build(in)["P1"]
	if v.Diff[0] != -2 || !math.IsNaN(v.Diff[1]) || v.AgreementCodes[0] != "i_clarity" || v.AgreementCodes[1] != "m_understanding" {
		t.Fatal(v.Diff, v.AgreementCodes)
	}
}

package store

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/survey"
)

func TestSubmissionVersionsSurviveReloadAndRepeatedDelivery(t *testing.T) {
	sv := survey.Default()
	path := filepath.Join(t.TempDir(), "state.json")
	st := New(sv, path)
	row := survey.Response{ID: "P1-intern-M1", PairID: "P1", Person: "I-01", Role: survey.Intern, Period: 1, Source: "webhook", Scores: map[string]float64{"i_overall": 3}}
	if _, err := st.SaveSubmission(row, "", "event1"); err != nil {
		t.Fatal(err)
	}
	row.Scores = map[string]float64{"i_overall": 5}
	if _, err := st.SaveSubmission(row, "", "event2"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveSubmission(row, "", "event1"); !errors.Is(err, ErrDuplicateDelivery) {
		t.Fatal(err)
	}
	if ids, err := st.SaveSubmission(row, "", "event3"); err != nil || len(ids) != 0 {
		t.Fatal(ids, err)
	}
	copy := New(sv, path)
	if ok, err := copy.Load(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	responses := copy.Input().Responses
	versions := copy.ResponseVersions("P1")[row.ID]
	if len(responses) != 1 || responses[0].Scores["i_overall"] != 5 || len(versions) != 2 || versions[0].Scores["i_overall"] != 3 || versions[1].Scores["i_overall"] != 5 {
		t.Fatalf("%v / %v", responses, versions)
	}
}

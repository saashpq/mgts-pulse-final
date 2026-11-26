package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/survey"
)

// state.json до R4: пороги с decline_from_peak и без новых полей, вердикты без action/period.
func TestLoadPreR4State(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	old := `{"responses":{},"analyses":{},"verdicts":{"P2-S7":{"status":"confirmed","comment":"ok","at":"2026-10-01T10:00:00Z"}},` +
		`"thresholds":{"decline_slope":-0.3,"decline_from_peak":0.8,"low_score":3,"gap_min":1.2,"gap_high":1.5,"persist":2},"tokens":{}}`
	if err := os.WriteFile(file, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(survey.Default(), file)
	if ok, err := s.Load(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	th := s.Thresholds()
	if th.DeclineSlope != -0.3 || th.GapMin != 1.2 || th.ZDrop != 3 || th.DeclineFromBase != 0.5 || th.CostRatio != 10 || th.MentorLow != 3 {
		t.Fatalf("%+v", th)
	}
	if v := s.Verdicts()["P2-S7"]; v.Status != "confirmed" || v.Action != "" || v.Period != 0 {
		t.Fatalf("%+v", v)
	}
	v, err := s.Decide("P2-S7", "resolved", "", "встреча 1:1 проведена", 4)
	if err != nil || v.Action != "встреча 1:1 проведена" || v.Period != 4 {
		t.Fatalf("%+v %v", v, err)
	}
	s2 := New(survey.Default(), file)
	if _, err := s2.Load(); err != nil || s2.Verdicts()["P2-S7"].Period != 4 {
		t.Fatal("action must persist")
	}
}

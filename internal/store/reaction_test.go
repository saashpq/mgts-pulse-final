package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ElzaraA/mgts-ai/internal/signals"
	"github.com/ElzaraA/mgts-ai/internal/survey"
)

func TestFirstDecisionTimeByEpisode(t *testing.T) {
	s := New(survey.Default(), filepath.Join(t.TempDir(), "state.json"))
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	decide := func(id string, month, episode int) signals.Verdict {
		v, err := s.DecideInEpisode(id, "confirmed", "", "", month, episode)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	a := decide("a", 1, 1)
	now = now.Add(time.Hour)
	b := decide("a", 2, 1)
	if a.FirstAt == "" || a.FirstAt != b.FirstAt || a.At == b.At {
		t.Fatalf("updated first time: %+v %+v", a, b)
	}
	c := decide("a", 5, 5)
	if c.FirstAt != c.At || c.FirstAt == a.FirstAt {
		t.Fatalf("stale episode: %+v", c)
	}
	s.st.Verdicts["legacy"] = signals.Verdict{At: a.At, Period: 1}
	if decide("legacy", 2, 1).FirstAt != "" {
		t.Fatal("fabricated first legacy decision")
	}
}

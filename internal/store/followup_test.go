package store

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ElzaraA/mgts-ai/internal/survey"
)

func followUpPlan(revision int, action, day string) FollowUpCommand {
	return FollowUpCommand{Operation: "plan", Revision: &revision, AgreedAction: action, DueDate: day}
}

func TestFollowUpTimingAndRepeatedEpisode(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	s := New(survey.Default(), file)
	// A snapshot from before the feature can be loaded and receive its first task.
	if err := os.WriteFile(file, []byte(`{"responses":{},"verdicts":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Load(); err != nil || !ok {
		t.Fatal(ok, err)
	}
	clock := time.Date(2026, 11, 25, 20, 59, 59, 0, time.UTC)
	s.now = func() time.Time { return clock }
	for _, episode := range []int{1, 3} {
		if _, changed, err := s.UpdateFollowUp("P1-S3", episode, "P1", "Расхождение", followUpPlan(0, "Поговорить со стажёром", "2026-11-26")); err != nil || !changed {
			t.Fatal(changed, err)
		}
	}
	for _, tc := range []struct {
		after       time.Duration
		day, timing string
	}{
		{0, "2026-11-25", "upcoming"}, {time.Second, "2026-11-26", "today"}, {24 * time.Hour, "2026-11-26", "today"}, {24*time.Hour + time.Second, "2026-11-27", "overdue"},
	} {
		clock = time.Date(2026, 11, 25, 20, 59, 59, 0, time.UTC).Add(tc.after)
		q := s.FollowUpQueue()
		if q.Today != tc.day || len(q.Items) != 2 || q.Items[0].Timing != tc.timing || q.Items[0].Episode != 1 || q.Items[1].Episode != 3 {
			t.Fatalf("%v: %+v", clock, q)
		}
	}
}

func TestConcurrentFollowUpEditsAndRollback(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	s := New(survey.Default(), file)
	if _, _, err := s.UpdateFollowUp("P1-S3", 1, "P1", "Темы", followUpPlan(0, "Договориться о встрече", "2026-10-01")); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, day := range []string{"2026-10-03", "2026-10-04"} {
		wg.Add(1)
		go func(day string) {
			defer wg.Done()
			_, _, err := s.UpdateFollowUp("P1-S3", 1, "", "", followUpPlan(1, "Перенести встречу", day))
			results <- err
		}(day)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrFollowUpConflict):
			conflict++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	prev := s.FollowUpQueue().Items[0]
	if len(prev.Reschedules) != 1 || prev.Reschedules[0].DueDate != "2026-10-01" || prev.Reschedules[0].NextDueDate != prev.DueDate {
		t.Fatalf("expected the former deadline to survive one concurrent edit: %+v", prev)
	}
	reopened := New(survey.Default(), file)
	if ok, err := reopened.Load(); err != nil || !ok || len(reopened.FollowUpQueue().Items[0].Reschedules) != 1 {
		t.Fatal("missing deadline history after restart", err)
	}
	// Replace the target snapshot path with a directory to provoke a real save failure.
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	_, _, err := s.UpdateFollowUp("P1-S3", 1, "", "", followUpPlan(2, "Ещё один шаг", "2026-10-06"))
	if err == nil {
		t.Fatal("expected persistence failure")
	}
	item := s.FollowUpQueue().Items[0]
	if item.Revision != prev.Revision || item.DueDate != prev.DueDate || item.AgreedAction != prev.AgreedAction || len(item.Reschedules) != 1 {
		t.Fatalf("failed write changed queue: %+v", item)
	}
}

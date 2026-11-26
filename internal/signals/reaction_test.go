package signals

import "testing"

func TestReactionDenominatorIncludesPendingAndUndated(t *testing.T) {
	s := SignalView{SeenAt: "2026-11-01T10:00:00Z", Verdict: &Verdict{FirstAt: "2026-11-01T13:30:00Z"}}
	old := SignalView{SeenAt: s.SeenAt, Verdict: &Verdict{At: s.Verdict.FirstAt}}
	reverse := SignalView{SeenAt: s.Verdict.FirstAt, Verdict: &Verdict{FirstAt: s.SeenAt}}
	r := reactionTime([]PairReport{{Signals: []SignalView{s, {}}, Resolved: []SignalView{old, reverse}}})
	if r.Total != 4 || r.Measured != 1 || r.Unknown != 2 || r.Pending != 1 || r.MedianHours == nil || *r.MedianHours != 3.5 {
		t.Fatalf("%+v", r)
	}
}

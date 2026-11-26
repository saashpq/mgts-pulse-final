package signals

import "testing"

func TestRecurringMissingRequiresNewVerdict(t *testing.T) {
	const id = "P1-S11-intern"
	for _, lateReply := range []bool{true, false} {
		in := synthInput()
		var rows = in.Responses[:0]
		for _, r := range in.Responses {
			if r.Period > 6 || (r.Person == "I-01" && (r.Period >= 5 || r.Period == 2 && !lateReply)) {
				continue
			}
			rows = append(rows, r)
		}
		in.Responses = rows
		for _, status := range []string{"false", "confirmed", "resolved"} {
			for _, period := range []int{0, 2, 5} {
				p := pairOf(Analyze(in, map[string]Verdict{id: {Status: status, Period: period}}), "P1")
				var current *SignalView
				for i := range p.Signals {
					if p.Signals[i].ID == id {
						current = &p.Signals[i]
					}
				}
				first := 2
				if lateReply {
					first = 5
				}
				if current == nil || current.FirstPeriod != 5 || current.FirstSeen != first || current.LastPeriod != 6 || current.Severity != Medium {
					t.Fatalf("backfilled=%v: unexpected current episode %+v", lateReply, current)
				}
				applies := period == 5 || (period == 0 && lateReply)
				if (current.Verdict != nil) != applies || (!applies && (p.Status != "watch" || p.History[5] != "medium")) {
					t.Errorf("backfilled=%v, verdict=%s at M%d: pair=%s, verdict=%+v history=%v", lateReply, status, period, p.Status, current.Verdict, p.History)
				}
			}
		}
	}
}

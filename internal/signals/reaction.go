package signals

import "time"

// Reaction относится к последнему эпизоду каждого известного сигнала, включая снятые.
type Reaction struct {
	Total       int      `json:"total"`
	Measured    int      `json:"measured"`
	Pending     int      `json:"pending"`
	Unknown     int      `json:"unknown"`
	MedianHours *float64 `json:"median_hours"`
}

func reactionTime(pairs []PairReport) Reaction {
	var result Reaction
	var hours []float64
	for _, p := range pairs {
		for _, group := range [][]SignalView{p.Signals, p.Resolved} {
			for _, s := range group {
				result.Total++
				if s.Verdict == nil {
					result.Pending++
					continue
				}
				at, e1 := time.Parse(time.RFC3339, s.SeenAt)
				decided, e2 := time.Parse(time.RFC3339, s.Verdict.FirstAt)
				if e1 != nil || e2 != nil || decided.Before(at) {
					result.Unknown++
					continue
				}
				hours = append(hours, decided.Sub(at).Hours())
			}
		}
	}
	result.Measured = len(hours)
	if result.Measured > 0 {
		h := r2(median(hours))
		result.MedianHours = &h
	}
	return result
}

package signals

import (
	"math"
	"strings"

	"github.com/ElzaraA/mgts-ai/internal/survey"
)

// itemDropEpisode follows one contiguous decline. A missing observation
// breaks the chain. Recovery to the starting value closes the episode.
func itemDropEpisode(values []float64, threshold float64) (base, onset int) {
	base, onset = -1, -1
	if threshold <= 0 {
		return
	}
	for i, value := range values {
		if math.IsNaN(value) {
			base, onset = -1, -1
			continue
		}
		if base < 0 || value >= values[base] || (onset < 0 && value > values[i-1]) {
			base, onset = i, -1
		} else if onset < 0 && values[i-1]-value >= threshold {
			base, onset = i-1, i
		} else if onset < 0 && values[base]-value >= threshold {
			onset = i
		}
	}
	return
}

func verifiedQuote(rows []survey.Response, e Evidence) bool {
	if e.Quote == "" {
		return false
	}
	for _, r := range rows {
		if r.ID == e.ResponseID && string(r.Role) == e.Role && r.Period == e.Period {
			return strings.Contains(r.Comments[e.Question], e.Quote)
		}
	}
	return false
}

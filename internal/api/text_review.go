package api

import (
	"slices"
	"sort"
	"strings"

	"github.com/ElzaraA/mgts-ai/internal/signals"
	"github.com/ElzaraA/mgts-ai/internal/survey"
)

type reviewRow struct {
	Pair   string      `json:"pair"`
	Period int         `json:"period"`
	Role   survey.Role `json:"role"`
	Count  int         `json:"count"`
}

type reviewSummary struct {
	Total        int               `json:"total"`
	Unclassified int               `json:"unclassified"`
	Rows         []reviewRow       `json:"rows"`
	Critical     []criticalComment `json:"critical,omitempty"`
}

type criticalComment struct {
	Pair     string      `json:"pair"`
	Period   int         `json:"period"`
	Role     survey.Role `json:"role"`
	Question string      `json:"question"`
}

// The overview contains counts and navigation only. Verbatim comments stay in the HR pair detail.
func textReview(in signals.Input, report signals.ProgramReport) reviewSummary {
	hybrid := map[string]bool{}
	for _, p := range report.Pairs {
		for _, s := range p.Signals {
			if s.Type == "S10" {
				hybrid[p.PairID] = true
				break
			}
		}
	}
	var critical []criticalComment
	rows := []reviewRow{}
	total, unclassified := 0, 0
	for _, r := range in.Responses {
		count := 0
		for code, comment := range r.Comments {
			if strings.TrimSpace(comment) == "" {
				continue
			}
			total++
			a := in.Analyses[r.ID+"/"+code]
			if len(a.Themes) == 0 {
				count++
			}
			if !hybrid[r.PairID] && r.Period >= report.Current-1 && r.Period <= report.Current &&
				a.Quote != "" && (slices.Contains(a.Themes, "conflict") || slices.Contains(a.Themes, "attrition")) {
				critical = append(critical, criticalComment{r.PairID, r.Period, r.Role, code})
			}
		}
		if count > 0 {
			unclassified += count
			rows = append(rows, reviewRow{Pair: r.PairID, Period: r.Period, Role: r.Role, Count: count})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Period != rows[j].Period {
			return rows[i].Period > rows[j].Period
		}
		if rows[i].Pair != rows[j].Pair {
			return rows[i].Pair < rows[j].Pair
		}
		return rows[i].Role < rows[j].Role
	})
	sort.Slice(critical, func(i, j int) bool {
		a, b := critical[i], critical[j]
		if a.Period != b.Period {
			return a.Period > b.Period
		}
		if a.Pair != b.Pair {
			return a.Pair < b.Pair
		}
		if a.Role != b.Role {
			return a.Role < b.Role
		}
		return a.Question < b.Question
	})
	return reviewSummary{Total: total, Unclassified: unclassified, Rows: rows, Critical: critical}
}

package signals

import (
	"sort"

	"github.com/ElzaraA/mgts-ai/internal/textai"
)

// ThemeTrend — как часто тема из закрытого списка встречалась в комментариях по месяцам.
type ThemeTrend struct {
	Code     string   `json:"code"`
	Label    string   `json:"label"`
	Negative bool     `json:"negative"`
	ByPeriod []int    `json:"by_period"` // число ответов с темой; индекс 0 = M1
	Total    int      `json:"total"`
	Pairs    []string `json:"pairs"` // пары, где тема встречалась
}

// ThemesOverTime — только темы, которые встретились хоть раз; негативные первыми, затем по убыванию суммы.
func ThemesOverTime(in Input) []ThemeTrend {
	n := in.Survey.Periods
	acc := map[string]*ThemeTrend{}
	pairs := map[string]map[string]bool{}
	for _, r := range in.Responses {
		if r.Period < 1 || r.Period > n {
			continue
		}
		seen := map[string]bool{}
		for code := range r.Comments {
			for _, th := range in.Analyses[r.ID+"/"+code].Themes {
				seen[th] = true
			}
		}
		for th := range seen {
			t, ok := textai.ThemeByCode(th)
			if !ok {
				continue
			}
			tr := acc[th]
			if tr == nil {
				tr = &ThemeTrend{Code: t.Code, Label: t.Label, Negative: t.Negative, ByPeriod: make([]int, n), Pairs: []string{}}
				acc[th] = tr
				pairs[th] = map[string]bool{}
			}
			tr.ByPeriod[r.Period-1]++
			tr.Total++
			pairs[th][r.PairID] = true
		}
	}
	out := make([]ThemeTrend, 0, len(acc))
	for th, tr := range acc {
		for p := range pairs[th] {
			tr.Pairs = append(tr.Pairs, p)
		}
		sort.Strings(tr.Pairs)
		out = append(out, *tr)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Negative != out[j].Negative {
			return out[i].Negative
		}
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].Code < out[j].Code
	})
	return out
}

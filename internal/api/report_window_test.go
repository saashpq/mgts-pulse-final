package api

import (
	"fmt"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/config"
	"github.com/ElzaraA/mgts-ai/internal/signals"
	"github.com/ElzaraA/mgts-ai/internal/store"
	"github.com/ElzaraA/mgts-ai/internal/survey"
)

func TestReportUsesWindowForCollectionAndMissingForms(t *testing.T) {
	sv := survey.Default()
	sv.Participant = sv.Participant[:1]
	st := store.New(sv, "")
	h := New(config.Config{}, st, sv, nil).Handler()
	form := "/api/v1/form/" + st.Tokens()[sv.Participant[0].Intern]
	set := func(month int, closed bool) {
		t.Helper()
		w := do(h, "PUT", "/api/v1/survey/window", fmt.Sprintf(`{"period":%d,"closed":%t}`, month, closed))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	check := func(month int, fill float64, missing int) {
		t.Helper()
		var result struct{ Report signals.ProgramReport }
		decode(t, do(h, "GET", "/api/v1/overview", ""), &result)
		got := 0
		for _, p := range result.Report.Pairs {
			for _, s := range p.Signals {
				if s.Type == "S11" && s.LastPeriod == month {
					got++
				}
			}
		}
		if r := result.Report; r.Current != month || r.FillRate != fill || got != missing {
			t.Fatalf("M%d fill=%v missing=%d, want M%d fill=%v missing=%d", r.Current, r.FillRate, got, month, fill, missing)
		}
	}
	check(0, 0, 0)
	set(1, false)
	check(1, 0, 0)
	set(1, true)
	check(1, 0, 2)
	set(1, false)
	check(1, 0, 0)
	if w := do(h, "POST", form, `{"period":1,"answers":{"i_overall":4,"i_interest":5,"i_clarity":3,"i_support":4}}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	check(1, .5, 0)
	set(1, true)
	check(1, .5, 1)
	set(2, false)
	check(2, 0, 0)
	set(2, true)
	check(2, 0, 2)
	set(2, false)
	if w := do(h, "POST", form, `{"period":2,"answers":{"i_overall":1,"i_interest":1,"i_clarity":1,"i_support":1}}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	set(1, false)
	check(1, .5, 0)
	var pair struct{ Report signals.PairReport }
	decode(t, do(h, "GET", "/api/v1/pairs/"+sv.Participant[0].ID, ""), &pair)
	if pair.Report.InternIndex[1] != nil {
		t.Fatal("the reopened M1 report contains a future score")
	}
	set(2, false)
	check(2, .5, 0)
}

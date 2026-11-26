package api

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/config"
	"github.com/ElzaraA/mgts-ai/internal/store"
	"github.com/ElzaraA/mgts-ai/internal/survey"
)

func TestCleanImportPersistsCalendarAndRoster(t *testing.T) {
	sv := survey.Default().WithoutDemoPairs()
	if len(sv.Participant) != 0 {
		t.Fatal("clean program contains demo pairs")
	}
	state := filepath.Join(t.TempDir(), "state.json")
	start := func() (*store.Store, http.Handler) {
		st, _, err := store.Open(sv, true, state)
		if err != nil {
			t.Fatal(err)
		}
		return st, New(config.Config{LLMMock: true}, st, sv, nil).Handler()
	}
	st, h := start()
	var sources struct{ Links, Events []any }
	decode(t, do(h, "GET", "/api/v1/sources", ""), &sources)
	if sources.Events == nil || sources.Links == nil || len(sources.Links) != 0 {
		t.Fatal("empty data sources must be JSON arrays", sources)
	}
	post := func(h http.Handler, row string, accepted, changed int) {
		t.Helper()
		var got struct {
			Changed int
			Report  struct{ Accepted int }
		}
		data := []byte("intern_id,mentor_id,period,i_overall\n" + row)
		decode(t, upload(h, "/api/v1/imports", "r.csv", data), &got)
		if got.Report.Accepted != accepted || got.Changed != changed {
			t.Fatalf("%q: %+v", row, got)
		}
	}
	post(h, "a,b,2026-09,4", 1, 1)
	st, h = start()
	post(h, "a,b,2026-11,5", 1, 1)
	post(h, "a,b,2026-11,5", 1, 0)
	post(h, "a,b,2026-08,5", 0, 0)
	post(h, "c,b,2026-11,3", 0, 0)
	post(h, "c,d,2026-11,NaN", 0, 0)
	if st.Survey().StartMonth != "2026-09" || len(st.Survey().Participant) != 1 || len(st.Responses()) != 2 {
		t.Fatalf("origin=%s pairs=%d responses=%d", st.Survey().StartMonth, len(st.Survey().Participant), len(st.Responses()))
	}
	want := map[int]float64{1: 4, 3: 5}
	for _, r := range st.Responses() {
		if want[r.Period] != r.Scores["i_overall"] {
			t.Fatalf("wrong month/score: %+v", r)
		}
		delete(want, r.Period)
	}
	if len(want) != 0 {
		t.Fatal("missing periods", want)
	}
}

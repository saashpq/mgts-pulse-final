package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/config"
	"github.com/ElzaraA/mgts-ai/internal/store"
	"github.com/ElzaraA/mgts-ai/internal/survey"
)

func TestFollowUpAPI(t *testing.T) {
	schema := survey.Default()
	path := filepath.Join(t.TempDir(), "state.json")
	a := New(config.Config{HRToken: "hr"}, store.New(schema, path), schema, nil)
	if err := a.Seed(); err != nil {
		t.Fatal(err)
	}
	s := a.report().Pairs[0]
	for _, p := range a.report().Pairs {
		if len(p.Signals) > 0 {
			s = p
			break
		}
	}
	sig := s.Signals[0]
	route := fmt.Sprintf("/api/v1/follow-ups/%s/%d", url.PathEscape(sig.ID), sig.Episode)
	h := a.Handler()
	plan := `{"operation":"plan","revision":0,"due_date":"2026-02-01","agreed_action":"Уточнить задачи с наставником"}`
	if w := do(h, "GET", "/api/v1/follow-ups", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := do(h, "PUT", route, plan); w.Code != 401 {
		t.Fatal(w.Code)
	}
	put := func(route, body string, status int) store.FollowUpView {
		w := do(h, "PUT", route, body, "Authorization", "Bearer hr")
		if w.Code != status {
			t.Fatalf("%d/%d: %s", w.Code, status, w.Body.String())
		}
		var f store.FollowUpView
		if status == 200 {
			decode(t, w, &f)
		}
		return f
	}
	put("/api/v1/follow-ups/missing/1", plan, 404)
	put(route, strings.Replace(plan, "02-01", "02-30", 1), 400)
	put(route, strings.Replace(plan, `"revision":0,`, "", 1), 400)
	f := put(route, plan, 200)
	if f.SignalID != sig.ID || f.Owner != "HR" || f.Revision != 1 || f.Checks == nil || f.Pair != sig.Pair {
		t.Fatalf("%+v", f)
	}
	if retry := put(route, plan, 200); retry.Revision != 1 {
		t.Fatalf("create retry: %+v", retry)
	}
	// Hide all current rule episodes; an existing follow-up remains editable.
	if err := a.Store.SetFormWindow(store.FormWindow{Period: 1}); err != nil {
		t.Fatal(err)
	}
	part := `{"operation":"check","revision":1,"delivery":"done","outcome":"partial","note":"Согласован новый контакт","next_due_date":"2099-04-22"}`
	f = put(route, part, 200)
	if f.Status != "open" || f.Revision != 2 || f.DueDate != "2099-04-22" || len(f.Checks) != 1 || f.Checks[0].DueDate != "2026-02-01" || f.Checks[0].Outcome != "partial" {
		t.Fatalf("%+v", f)
	}
	if f = put(route, part, 200); len(f.Checks) != 1 {
		t.Fatalf("duplicated check: %+v", f)
	}
	put(route, `{"operation":"check","revision":1,"outcome":"done"}`, 409)
	st := store.New(schema, path)
	if ok, err := st.Load(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	h = New(config.Config{HRToken: "hr"}, st, schema, nil).Handler()
	f = put(route, `{"operation":"check","revision":2,"outcome":"done"}`, 200)
	if f.Status != "closed" || f.Timing != "closed" || f.Revision != 3 || len(f.Checks) != 2 || f.Checks[1].Delivery != "unknown" {
		t.Fatalf("%+v", f)
	}
	w := do(h, "GET", "/api/v1/follow-ups", "", "Authorization", "Bearer hr")
	var q store.FollowUpQueue
	if err := json.Unmarshal(w.Body.Bytes(), &q); err != nil || w.Code != 200 || q.Today == "" || len(q.Items) != 1 || q.Items[0].Checks[1].Outcome != "done" {
		t.Fatal(err, w.Code, w.Body.String())
	}
}

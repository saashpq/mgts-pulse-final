package api

import (
	"path/filepath"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/config"
	"github.com/ElzaraA/mgts-ai/internal/store"
	"github.com/ElzaraA/mgts-ai/internal/survey"
)

func TestFormWindowCorrectionsAndClosure(t *testing.T) {
	sv := survey.Default()
	path := filepath.Join(t.TempDir(), "state.json")
	cfg := config.Config{WebhookSecret: "test", HRToken: "hr"}
	st := store.New(sv, path)
	h := New(cfg, st, sv, nil).Handler()
	form := "/api/v1/form/" + st.Tokens()["I-01"]
	check := func(method, target, body string, want int, headers ...string) {
		t.Helper()
		w := do(h, method, target, body, headers...)
		if w.Code != want {
			t.Fatalf("%s %s: status %d, want %d, %s", method, target, w.Code, want, w.Body.String())
		}
	}
	window := func(target string, wantPeriod int, wantClosed bool) {
		t.Helper()
		var out struct {
			Period int
			Closed bool
		}
		decode(t, do(h, "GET", target, ""), &out)
		if out.Period != wantPeriod || out.Closed != wantClosed {
			t.Fatalf("got %+v, want M%d closed=%v", out, wantPeriod, wantClosed)
		}
	}
	answers := `"answers":{"i_overall":4,"i_interest":3,"i_support":5,"i_clarity":2}`
	window(form, 1, false)
	check("POST", form, "{"+answers+"}", 200)
	window(form, 1, false)
	check("POST", form, "{"+answers+"}", 200)
	if n := len(st.Responses()); n != 1 {
		t.Fatalf("%d responses after correction", n)
	}
	window("/api/v1/form/"+st.Tokens()["I-02"], 1, false)

	admin := "/api/v1/survey/window"
	check("PUT", admin, `{"period":2,"closed":false}`, 401)
	check("PUT", admin, `{"period":2,"closed":false}`, 200, "Authorization", "Bearer hr")
	window(form, 2, false)
	check("POST", form, `{"period":1,`+answers+"}", 409)
	check("POST", form, `{"period":2,`+answers+"}", 200)
	check("PUT", admin, `{"period":2,"closed":true}`, 200, "Authorization", "Bearer hr")
	window(form, 2, true)
	check("POST", form, "{"+answers+"}", 409)
	check("POST", "/api/v1/integrations/yandex-forms", `{"participant":"I-01","period":2,`+answers+"}", 409, "X-Webhook-Secret", "test")
	reloaded, loaded, err := store.Open(sv, true, path)
	if err != nil || !loaded {
		t.Fatalf("reload: %v %v", loaded, err)
	}
	h = New(cfg, reloaded, sv, nil).Handler()
	window(form, 2, true)
	check("PUT", admin, `{"period":2,"closed":false}`, 200, "Authorization", "Bearer hr")
	check("POST", form, `{"period":2,`+answers+"}", 200)
}

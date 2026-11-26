package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ElzaraA/mgts-ai/internal/config"
	"github.com/ElzaraA/mgts-ai/internal/llm"
	"github.com/ElzaraA/mgts-ai/internal/store"
	"github.com/ElzaraA/mgts-ai/internal/survey"
)

func newApp(t *testing.T, client llm.Client) (*App, http.Handler) {
	t.Helper()
	cfg := config.Config{CORSOrigins: []string{"http://localhost:5173"}, PublicURL: "http://ui", WebhookSecret: "s3", DemoMode: true, LLMModel: "m"}
	sv := survey.Default()
	a := New(cfg, store.New(sv, ""), sv, client)
	a.Start(context.Background())
	t.Cleanup(a.Stop)
	if err := a.Seed(); err != nil {
		t.Fatal(err)
	}
	return a, a.Handler()
}

func do(h http.Handler, method, path, body string, hdr ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestOverviewAndPair(t *testing.T) {
	_, h := newApp(t, nil)
	var ov struct {
		Report struct {
			Attention int `json:"attention"`
			Current   int `json:"current"`
		} `json:"report"`
		Analyzer map[string]string `json:"analyzer"`
	}
	decode(t, do(h, "GET", "/api/v1/overview", ""), &ov)
	if ov.Report.Attention != 3 || ov.Report.Current != 6 || ov.Analyzer["mode"] != "offline" {
		t.Fatalf("%+v", ov)
	}
	var p struct {
		Comments  []map[string]any `json:"comments"`
		Questions []map[string]any `json:"questions"`
	}
	decode(t, do(h, "GET", "/api/v1/pairs/P2", ""), &p)
	if len(p.Comments) != 12 || len(p.Questions) != 8 {
		t.Fatalf("comments %d questions %d", len(p.Comments), len(p.Questions))
	}
	if w := do(h, "GET", "/api/v1/pairs/P9", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := do(h, "GET", "/nope", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := do(h, "GET", "/health", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	var sl struct {
		Signals []map[string]any `json:"signals"`
	}
	decode(t, do(h, "GET", "/api/v1/signals?state=active&severity=high", ""), &sl)
	if len(sl.Signals) == 0 || sl.Signals[0]["severity"] != "high" {
		t.Fatalf("%v", sl.Signals)
	}
	decode(t, do(h, "GET", "/api/v1/signals?state=past", ""), &sl)
	for _, s := range sl.Signals {
		if s["active"] == true {
			t.Fatal("past filter")
		}
	}
	if w := do(h, "GET", "/api/v1/survey", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
}

func TestPairWithoutCommentsReturnsEmptyList(t *testing.T) {
	sv := survey.Default()
	st := store.New(sv, "")
	if _, err := st.Upsert([]survey.Response{{PairID: "P1", Role: survey.Intern, Period: 1, Scores: map[string]float64{"i_overall": 4}}}, "import", ""); err != nil {
		t.Fatal(err)
	}
	h := New(config.Config{}, st, sv, nil).Handler()
	w := do(h, "GET", "/api/v1/pairs/P1", "")
	var p struct{ Comments []commentView }
	decode(t, w, &p)
	if w.Code != 200 || p.Comments == nil || len(p.Comments) != 0 {
		t.Fatalf("expected an empty comments array, got %s", w.Body.String())
	}
}

func TestBriefAndOverviewExtras(t *testing.T) {
	_, h := newApp(t, nil)
	cases := []struct {
		pair      string
		code      int
		signals   bool
		withBoth  bool
		mustTitle string
	}{
		{"P2", 200, true, true, "Потеря мотивации стажёра"},
		{"P1", 200, false, true, ""},
		{"P9", 404, false, false, ""},
	}
	for _, c := range cases {
		w := do(h, "GET", "/api/v1/pairs/"+c.pair+"/brief", "")
		if w.Code != c.code {
			t.Fatalf("%s: %d", c.pair, w.Code)
		}
		if c.code != 200 {
			continue
		}
		var b struct {
			Pair        string `json:"pair"`
			GeneratedAt string `json:"generated_at"`
			Signals     []struct {
				Title  string   `json:"title"`
				Quotes []string `json:"quotes"`
			} `json:"signals"`
			Questions []struct {
				With string `json:"with"`
				Text string `json:"text"`
			} `json:"questions"`
			TalkingPoints []string `json:"talking_points"`
			Disclaimer    string   `json:"disclaimer"`
		}
		decode(t, w, &b)
		if b.Pair != c.pair || b.GeneratedAt == "" || b.Disclaimer == "" || len(b.TalkingPoints) == 0 || (len(b.Signals) > 0) != c.signals {
			t.Fatalf("%s: %+v", c.pair, b)
		}
		with := map[string]bool{}
		for _, q := range b.Questions {
			with[q.With] = true
		}
		if len(b.Questions) < 3 || len(b.Questions) > 5 || with["intern"] != c.withBoth || with["mentor"] != c.withBoth {
			t.Fatalf("%s: questions %+v", c.pair, b.Questions)
		}
		if c.mustTitle != "" && !strings.Contains(w.Body.String(), c.mustTitle) {
			t.Fatalf("%s: no %q", c.pair, c.mustTitle)
		}
	}
	if routeOf("/api/v1/pairs/P2/brief") != "/api/v1/pairs/{id}/brief" {
		t.Fatal("brief route must not blow up metrics")
	}
	// вердикт «ложный» попадает в точность сигналов на обзоре
	for _, id := range []string{"P4-S3", "P2-S3"} {
		if w := do(h, "POST", "/api/v1/signals/"+id+"/verdict", `{"status":"false"}`); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	var ov struct {
		Report struct {
			Precision []struct {
				Type      string   `json:"type"`
				False     int      `json:"false"`
				Precision *float64 `json:"precision"`
				Hint      string   `json:"hint"`
				Decisions int      `json:"decisions"`
				Enough    bool     `json:"enough"`
			} `json:"precision"`
			Total struct {
				Decisions int      `json:"decisions"`
				Precision *float64 `json:"precision"`
				Enough    bool     `json:"enough"`
			} `json:"precision_total"`
		} `json:"report"`
		Themes []struct {
			Code     string   `json:"code"`
			Negative bool     `json:"negative"`
			ByPeriod []int    `json:"by_period"`
			Pairs    []string `json:"pairs"`
		} `json:"themes_over_time"`
	}
	decode(t, do(h, "GET", "/api/v1/overview", ""), &ov)
	found := false
	for _, p := range ov.Report.Precision {
		if p.Type == "S3" {
			// два решения — точность есть, но подсказки по порогу нет: мало данных
			found = p.False == 2 && p.Precision != nil && *p.Precision == 0 && p.Decisions == 2 && !p.Enough && p.Hint == ""
		}
	}
	if !found {
		t.Fatalf("precision: %+v", ov.Report.Precision)
	}
	if tot := ov.Report.Total; tot.Decisions != 2 || tot.Precision == nil || *tot.Precision != 0 || tot.Enough {
		t.Fatalf("precision_total: %+v", tot)
	}
	if len(ov.Themes) == 0 || !ov.Themes[0].Negative || len(ov.Themes[0].ByPeriod) != 6 || len(ov.Themes[0].Pairs) == 0 {
		t.Fatalf("themes_over_time: %+v", ov.Themes)
	}
}

func TestFormFlowUpdatesDashboard(t *testing.T) {
	a, h := newApp(t, nil)
	ch := a.Broker.Subscribe()
	defer a.Broker.Unsubscribe(ch)
	tok := a.Store.Tokens()["I-01"]
	var f struct {
		Role      string `json:"role"`
		Period    int    `json:"period"`
		Questions []any  `json:"questions"`
	}
	decode(t, do(h, "GET", "/api/v1/form/"+tok, ""), &f)
	if f.Role != "intern" || f.Period != 6 || len(f.Questions) != 5 {
		t.Fatalf("%+v", f)
	}
	body := `{"period":6,"answers":{"i_overall":2,"i_interest":2,"i_clarity":2,"i_support":1,"i_comment":"Наставник не отвечает, конфликт с наставником"}}`
	if w := do(h, "POST", "/api/v1/form/"+tok, body); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	select {
	case ev := <-ch:
		if ev.Event != "data.updated" {
			t.Fatal(ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no live update")
	}
	var p struct {
		Report struct {
			Status string `json:"status"`
		} `json:"report"`
	}
	decode(t, do(h, "GET", "/api/v1/pairs/P1", ""), &p)
	if p.Report.Status != "attention" {
		t.Fatalf("control pair must react to a bad answer: %s", p.Report.Status)
	}
	for name, b := range map[string]string{
		"out of scale":     `{"answers":{"i_overall":9,"i_interest":2,"i_clarity":2,"i_support":1}}`,
		"missing scale":    `{"answers":{"i_overall":3}}`,
		"foreign question": `{"answers":{"i_overall":3,"i_interest":2,"i_clarity":2,"i_support":1,"m_speed":2}}`,
		"bad period":       `{"period":9,"answers":{"i_overall":3,"i_interest":2,"i_clarity":2,"i_support":1}}`,
		"not json":         `{`,
	} {
		if w := do(h, "POST", "/api/v1/form/"+tok, b); w.Code != 400 {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	if w := do(h, "GET", "/api/v1/form/unknown", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := do(h, "POST", "/api/v1/form/"+tok, body, "Origin", "https://evil.example"); w.Code != 403 {
		t.Fatal("csrf", w.Code)
	}
}

func TestWebhook(t *testing.T) {
	a, h := newApp(t, nil)
	body := `{"participant":"M-05","period":6,"answers":{"m_overall":"2","m_speed":"2","m_autonomy":"2","m_understanding":"2"}}`
	if w := do(h, "POST", "/api/v1/integrations/yandex-forms", body, "X-Webhook-Secret", "bad"); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := do(h, "POST", "/api/v1/integrations/yandex-forms", body, "X-Webhook-Secret", "s3"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	tok := a.Store.Tokens()["I-05"]
	if w := do(h, "POST", "/api/v1/integrations/yandex-forms", `{"token":"`+tok+`","period":5,"answers":{"i_overall":"4","i_interest":"4","i_clarity":"4","i_support":"4"}}`, "X-Webhook-Secret", "s3"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := do(h, "POST", "/api/v1/integrations/yandex-forms", `{"token":"x"}`, "X-Webhook-Secret", "s3"); w.Code != 404 {
		t.Fatal(w.Code)
	}
	// повтор с тем же X-Delivery-Id подтверждается, но данные не меняются
	dup := `{"participant":"M-05","period":6,"answers":{"m_overall":"5","m_speed":"5","m_autonomy":"5","m_understanding":"5"}}`
	steps := []struct {
		name, body, id string
		code           int
		duplicate      bool
		overall        float64
	}{
		{"first delivery", body, "dlv-1", 200, false, 2},
		{"retry same id", dup, "dlv-1", 200, true, 2},
		{"failed delivery", `{"participant":"nobody","answers":{}}`, "dlv-2", 400, false, 2},
		{"retry after failure", dup, "dlv-2", 200, false, 5},
		{"too long id", dup, strings.Repeat("x", 201), 400, false, 5},
	}
	for _, st := range steps {
		w := do(h, "POST", "/api/v1/integrations/yandex-forms", st.body, "X-Webhook-Secret", "s3", "X-Delivery-Id", st.id)
		if w.Code != st.code {
			t.Fatalf("%s: %d %s", st.name, w.Code, w.Body.String())
		}
		var res struct {
			Duplicate bool `json:"duplicate"`
		}
		decode(t, w, &res)
		if res.Duplicate != st.duplicate {
			t.Fatalf("%s: duplicate=%v", st.name, res.Duplicate)
		}
		for _, r := range a.Store.Responses() {
			if r.ID == "P5-mentor-M6" && r.Scores["m_overall"] != st.overall {
				t.Fatalf("%s: m_overall %v", st.name, r.Scores["m_overall"])
			}
		}
	}
	a.Cfg.WebhookSecret = ""
	if w := do(a.Handler(), "POST", "/api/v1/integrations/yandex-forms", body); w.Code != 503 {
		t.Fatal("disabled webhook", w.Code)
	}
}

func TestImportIdempotent(t *testing.T) {
	_, h := newApp(t, nil)
	data, err := os.ReadFile("../../data/sample/ответы_стажеров.xlsx")
	if err != nil {
		t.Skip("sample not generated")
	}
	send := func(name string, data []byte) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", name)
		fw.Write(data)
		mw.Close()
		r := httptest.NewRequest("POST", "/api/v1/imports", &buf)
		r.Header.Set("Content-Type", mw.FormDataContentType())
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	var res struct {
		Changed, Unchanged int
	}
	decode(t, send("a.xlsx", data), &res)
	if res.Changed != 0 || res.Unchanged != 29 {
		t.Fatalf("re-import of the same data must change nothing: %+v", res)
	}
	if w := send("x.csv", []byte("a,b\n1,2\n")); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := do(h, "POST", "/api/v1/imports", "x", "Content-Type", "text/plain"); w.Code != 400 {
		t.Fatal(w.Code)
	}
}

func TestVerdictAndThresholds(t *testing.T) {
	_, h := newApp(t, nil)
	if w := do(h, "POST", "/api/v1/signals/P4-S3/verdict", `{"status":"false","comment":"проверили"}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := do(h, "POST", "/api/v1/signals/P4-S3/verdict", `{"status":"maybe"}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := do(h, "POST", "/api/v1/signals/P1-S1/verdict", `{"status":"confirmed"}`); w.Code != 404 {
		t.Fatal(w.Code)
	}
	var th struct {
		Thresholds map[string]float64 `json:"thresholds"`
	}
	decode(t, do(h, "GET", "/api/v1/thresholds", ""), &th)
	th.Thresholds["gap_min"] = 1.2
	b, _ := json.Marshal(th.Thresholds)
	if w := do(h, "PUT", "/api/v1/thresholds", string(b)); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	th.Thresholds["decline_slope"] = 1
	b, _ = json.Marshal(th.Thresholds)
	if w := do(h, "PUT", "/api/v1/thresholds", string(b)); w.Code != 400 {
		t.Fatal(w.Code)
	}
	var src struct {
		Links []any `json:"links"`
	}
	decode(t, do(h, "GET", "/api/v1/sources", ""), &src)
	if len(src.Links) != 10 {
		t.Fatal(len(src.Links))
	}
	if w := do(h, "POST", "/api/v1/demo/reset", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := do(h, "GET", "/metrics", ""); !strings.Contains(w.Body.String(), "pulse_http_requests_total") {
		t.Fatal("metrics")
	}
}

type fakeLLM struct{}

func (fakeLLM) Complete(_ context.Context, msgs []llm.Message) (string, error) {
	if strings.Contains(msgs[1].Content, "рутина") || strings.Contains(msgs[1].Content, "Рутина") {
		// R4: цитата ≥ 3 слов в пределах одного предложения
		return `{"sentiment":-0.9,"themes":["routine"],"quote":"Рутина, одно и то же"}`, nil
	}
	return `{"sentiment":0,"themes":[],"quote":""}`, nil
}

// С моделью: сначала мгновенный офлайн-разбор, затем фоновый разбор моделью с проверкой цитаты.
func TestLLMWorker(t *testing.T) {
	a, h := newApp(t, fakeLLM{})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if an, ok := a.Store.Analysis("P2-intern-M4/i_comment"); ok && an.Analyzer == "llm:m" {
			var ov struct {
				Analyzer map[string]string `json:"analyzer"`
			}
			decode(t, do(h, "GET", "/api/v1/overview", ""), &ov)
			if ov.Analyzer["mode"] != "llm" {
				t.Fatal(ov)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("model analysis not applied")
}

func TestRecoverAndRoute(t *testing.T) {
	h := Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	if w := do(h, "GET", "/", ""); w.Code != 500 || strings.Contains(w.Body.String(), "boom") {
		t.Fatal("panic must be hidden")
	}
	if routeOf("/api/v1/form/abc") != "/api/v1/form/{id}" || routeOf("/api/v1/signals/P1-S1/verdict") != "/api/v1/signals/{id}/verdict" {
		t.Fatal("routeOf")
	}
	if w := do(CORS([]string{"http://ui"})(http.NotFoundHandler()), "OPTIONS", "/", "", "Origin", "http://ui"); w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "http://ui" {
		t.Fatal("cors")
	}
}

func TestMetricsHideContactAndParticipantIdentifiers(t *testing.T) {
	_, h := newApp(t, nil)
	privatePath := "/api/v1/follow-ups/person-a-signal/427"
	do(h, "PUT", privatePath, "{}")
	do(h, "GET", "/form/private-participant-token", "")
	do(h, "GET", "/pairs/private-pair/months/2", "")
	w := do(h, "GET", "/metrics", "")
	body := w.Body.String()
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	for _, private := range []string{"person-a-signal", "/427", "private-participant-token", "private-pair"} {
		if strings.Contains(body, private) {
			t.Errorf("public metrics contain private value %q", private)
		}
	}
	for _, route := range []string{"/api/v1/follow-ups/{signal}/{episode}", "/form/{id}", "/pairs/{id}/months/2"} {
		if !strings.Contains(body, route) {
			t.Errorf("missing anonymized route %q", route)
		}
	}
}

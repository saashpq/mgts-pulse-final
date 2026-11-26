package api

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ElzaraA/mgts-ai/internal/config"
	"github.com/ElzaraA/mgts-ai/internal/llm"
	"github.com/ElzaraA/mgts-ai/internal/store"
	"github.com/ElzaraA/mgts-ai/internal/survey"
)

// fileApp — приложение со снимком на диске. breakSave/fixSave делают запись снимка невозможной и снова возможной:
// на месте файла появляется непустой каталог, и rename tmp → файл падает даже у root.
func fileApp(t *testing.T) (*App, func(), func()) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "state.json")
	cfg := config.Config{CORSOrigins: []string{"http://localhost:5173"}, PublicURL: "http://ui", WebhookSecret: "s3", DemoMode: true}
	sv := survey.Default()
	a := New(cfg, store.New(sv, file), sv, nil)
	a.Start(context.Background())
	t.Cleanup(a.Stop)
	if err := a.Seed(); err != nil {
		t.Fatal(err)
	}
	breakSave := func() {
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(file, "x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fixSave := func() {
		if err := os.RemoveAll(file); err != nil {
			t.Fatal(err)
		}
	}
	return a, breakSave, fixSave
}

func m5(score string) string {
	return `{"participant":"M-05","period":6,"answers":{"m_overall":"` + score + `","m_speed":"2","m_autonomy":"2","m_understanding":"2"}}`
}

func mentorOverall(a *App) float64 {
	for _, r := range a.Store.Responses() {
		if r.ID == "P5-mentor-M6" {
			return r.Scores["m_overall"]
		}
	}
	return 0
}

// Сбой записи снимка: 500, ни ответ, ни X-Delivery-Id не запоминаются; повтор после восстановления принимается.
func TestWebhookSaveFailureRollsBack(t *testing.T) {
	a, breakSave, fixSave := fileApp(t)
	h := a.Handler()
	before := mentorOverall(a)
	breakSave()
	w := do(h, "POST", "/api/v1/integrations/yandex-forms", m5("1"), "X-Webhook-Secret", "s3", "X-Delivery-Id", "d-1")
	if w.Code != 500 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if got := mentorOverall(a); got != before {
		t.Fatalf("response must be rolled back: %v", got)
	}
	// форма тоже получает 500, а не 400
	tok := a.Store.Tokens()["M-05"]
	if w := do(h, "POST", "/api/v1/form/"+tok, `{"period":6,"answers":{"m_overall":"1","m_speed":"2","m_autonomy":"2","m_understanding":"2"}}`); w.Code != 500 {
		t.Fatalf("form: %d", w.Code)
	}
	fixSave()
	w = do(h, "POST", "/api/v1/integrations/yandex-forms", m5("1"), "X-Webhook-Secret", "s3", "X-Delivery-Id", "d-1")
	if w.Code != 200 || strings.Contains(w.Body.String(), "duplicate") {
		t.Fatalf("retry after failure must be accepted: %d %s", w.Code, w.Body.String())
	}
	if mentorOverall(a) != 1 {
		t.Fatal("retry not applied")
	}
	if w := do(h, "POST", "/api/v1/integrations/yandex-forms", m5("3"), "X-Webhook-Secret", "s3", "X-Delivery-Id", "d-1"); !strings.Contains(w.Body.String(), "duplicate") {
		t.Fatal("after success the id must be remembered")
	}
}

func multipartFile(name string, data []byte) (*bytes.Buffer, string) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(data)
	mw.Close()
	return &buf, mw.FormDataContentType()
}

// Импорт: сбой записи — 500 и ни новых пар, ни ответов; одновременные импорты не дают одинаковых ID пар.
func TestImportSaveFailureAndConcurrency(t *testing.T) {
	a, breakSave, fixSave := fileApp(t)
	h := a.Handler()
	csv := func(intern, mentor string) []byte {
		return []byte("ID стажёра;ID наставника;Месяц;Как проходит стажировка\n" + intern + ";" + mentor + ";1;4\n")
	}
	post := func(data []byte) *httptest.ResponseRecorder {
		body, ct := multipartFile("x.csv", data)
		r := httptest.NewRequest("POST", "/api/v1/imports", body)
		r.Header.Set("Content-Type", ct)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	pairs := len(a.Store.Survey().Participant)
	breakSave()
	if w := post(csv("I-90", "M-90")); w.Code != 500 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if n := len(a.Store.Survey().Participant); n != pairs {
		t.Fatalf("new pair must be rolled back: %d", n)
	}
	fixSave()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune('A' + i))
			if w := post(csv("I-X"+id, "M-X"+id)); w.Code != 200 {
				t.Errorf("%d %s", w.Code, w.Body.String())
			}
		}(i)
	}
	wg.Wait()
	seen := map[string]bool{}
	for _, p := range a.Store.Survey().Participant {
		if seen[p.ID] {
			t.Fatalf("duplicate pair id %s", p.ID)
		}
		seen[p.ID] = true
	}
	if len(seen) != pairs+8 {
		t.Fatalf("pairs %d", len(seen))
	}
	// битый XLSX — 400 с понятной причиной
	w := post([]byte("PK\x03\x04garbage"))
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}

func TestHRToken(t *testing.T) {
	cfg := config.Config{CORSOrigins: []string{"http://ui"}, WebhookSecret: "s3", HRToken: "hr-secret"}
	sv := survey.Default()
	a := New(cfg, store.New(sv, ""), sv, nil)
	if err := a.Seed(); err != nil {
		t.Fatal(err)
	}
	h := a.Handler()
	tok := a.Store.Tokens()["I-01"]
	cases := []struct {
		method, path string
		hdr          []string
		code         int
	}{
		{"GET", "/api/v1/overview", nil, 401},
		{"GET", "/api/v1/overview", []string{"Authorization", "Bearer wrong"}, 401},
		{"GET", "/api/v1/overview?token=hr-secret", nil, 401}, // в query токен принимается только для EventSource
		{"GET", "/api/v1/overview", []string{"Authorization", "Bearer hr-secret"}, 200},
		{"GET", "/api/v1/overview", []string{"Authorization", "bearer hr-secret"}, 200},
		{"POST", "/api/v1/demo/reset", nil, 401},
		{"GET", "/api/v1/form/" + tok, nil, 200},
		{"POST", "/api/v1/integrations/yandex-forms", []string{"X-Webhook-Secret", "bad"}, 401},
		{"GET", "/health", nil, 200},
		{"GET", "/metrics", nil, 200},
		{"GET", "/api/v1/events", nil, 401},
	}
	for _, c := range cases {
		if w := do(h, c.method, c.path, "", c.hdr...); w.Code != c.code {
			t.Errorf("%s %s %v: %d want %d", c.method, c.path, c.hdr, w.Code, c.code)
		}
	}
	if w := do(h, "OPTIONS", "/api/v1/overview", "", "Origin", "http://ui"); w.Code != 204 || !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatal("preflight must pass without token")
	}
	// SSE с ?token= — поток открывается; закрытие брокера завершает его
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL + "/api/v1/events?token=hr-secret")
	if err != nil || resp.StatusCode != 200 {
		t.Fatal(err, resp)
	}
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 256)
		for {
			if _, err := resp.Body.Read(buf); err != nil {
				close(done)
				return
			}
		}
	}()
	time.Sleep(50 * time.Millisecond)
	a.Broker.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE stream must end after Broker.Close")
	}
	resp.Body.Close()
	// после закрытия новые подписчики сразу получают закрытый канал
	if _, ok := <-a.Broker.Subscribe(); ok {
		t.Fatal("closed broker")
	}
	a.Broker.Close() // повторно — без паники
}

type countLLM struct {
	mu sync.Mutex
	n  int
}

func (c *countLLM) Complete(context.Context, []llm.Message) (string, error) {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
	return `{"sentiment":0,"themes":[],"quote":""}`, nil
}

// Переполненная очередь не теряет комментарии: фоновый добор ставит оставшиеся, и все получают разбор модели.
func TestLLMQueueRefill(t *testing.T) {
	cfg := config.Config{LLMModel: "m"}
	sv := survey.Default()
	a := New(cfg, store.New(sv, ""), sv, &countLLM{})
	a.jobs = make(chan job, 4) // очередь заведомо меньше 60 комментариев синтетики
	a.RefillEvery = 10 * time.Millisecond
	if err := a.Seed(); err != nil { // до Start: воркер ещё не разбирает, очередь переполняется
		t.Fatal(err)
	}
	if len(a.Store.Pending(PendingNote)) <= 4 {
		t.Fatal("expected overflow")
	}
	a.Start(context.Background())
	defer a.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(a.Store.Pending(PendingNote)) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("still pending: %d", len(a.Store.Pending(PendingNote)))
}

// Повторная постановка того же текста не дублирует задачу; изменённый текст ставится заново,
// а разбор устаревшего текста не сохраняется.
func TestEnqueueDedup(t *testing.T) {
	cfg := config.Config{LLMModel: "m"}
	sv := survey.Default()
	a := New(cfg, store.New(sv, ""), sv, &countLLM{})
	if !a.enqueue(job{key: "k", text: "a"}) || !a.enqueue(job{key: "k", text: "a"}) || len(a.jobs) != 1 {
		t.Fatal("dedup", len(a.jobs))
	}
	if !a.enqueue(job{key: "k", text: "b"}) || len(a.jobs) != 2 {
		t.Fatal("changed text must be queued")
	}
	a.work(context.Background(), <-a.jobs) // берёт обе задачи: старый текст «a» отброшен
	if an, ok := a.Store.Analysis("k"); !ok {
		t.Fatalf("%+v", an)
	}
	if len(a.queued) != 0 {
		t.Fatal(a.queued)
	}
	if a.refill() != 0 {
		t.Fatal("nothing pending")
	}
}

func TestSignalsGroupedView(t *testing.T) {
	_, h := newApp(t, nil)
	type sig struct {
		ID             string `json:"id"`
		Pair           string `json:"pair"`
		Primary        bool   `json:"primary"`
		Active         bool   `json:"active"`
		Related        []string
		RelatedSignals []sig `json:"related_signals"`
	}
	var all, grouped, def struct {
		View    string `json:"view"`
		Signals []sig  `json:"signals"`
	}
	decode(t, do(h, "GET", "/api/v1/signals?view=all&state=active", ""), &all)
	decode(t, do(h, "GET", "/api/v1/signals?view=grouped&state=active", ""), &grouped)
	decode(t, do(h, "GET", "/api/v1/signals?state=active", ""), &def)
	if def.View != "grouped" || len(def.Signals) != len(grouped.Signals) || all.View != "all" {
		t.Fatal("grouped must be the default")
	}
	pairs := map[string]bool{}
	total := 0
	for _, s := range grouped.Signals {
		if !s.Primary || !s.Active || pairs[s.Pair] || s.RelatedSignals == nil || len(s.Related) != len(s.RelatedSignals) {
			t.Fatalf("%+v", s)
		}
		pairs[s.Pair] = true
		total += 1 + len(s.RelatedSignals)
		for _, r := range s.RelatedSignals {
			if r.Primary || r.Pair != s.Pair || r.ID == "" {
				t.Fatalf("related %+v", r)
			}
		}
	}
	if total != len(all.Signals) || len(grouped.Signals) >= len(all.Signals) {
		t.Fatalf("grouped %d (+related %d) vs all %d", len(grouped.Signals), total, len(all.Signals))
	}
	for _, s := range all.Signals {
		if s.RelatedSignals != nil {
			t.Fatal("flat view must not carry related_signals")
		}
	}
	if w := do(h, "GET", "/api/v1/signals?view=tree", ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	var high struct{ Signals []sig }
	decode(t, do(h, "GET", "/api/v1/signals?severity=high", ""), &high)
	if len(high.Signals) == 0 {
		t.Fatal("high")
	}
	var ov struct {
		Report struct {
			Effect struct {
				PairsFlaggedEarly int              `json:"pairs_flagged_early"`
				DetectionTable    []map[string]any `json:"detection_table"`
				VerdictCoverage   *float64         `json:"verdict_coverage"`
				Overdue           *int             `json:"overdue"`
			} `json:"effect"`
		} `json:"report"`
	}
	decode(t, do(h, "GET", "/api/v1/overview", ""), &ov)
	// ответы синтетики только что загружены: даты есть, срок реакции не истёк
	if e := ov.Report.Effect; e.PairsFlaggedEarly != 3 || len(e.DetectionTable) != 5 || e.VerdictCoverage == nil || e.Overdue == nil || *e.Overdue != 0 {
		t.Fatalf("%+v", e)
	}
}

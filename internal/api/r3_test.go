package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/config"
	"github.com/ElzaraA/mgts-ai/internal/llm"
	"github.com/ElzaraA/mgts-ai/internal/store"
	"github.com/ElzaraA/mgts-ai/internal/survey"
)

const foreignCSV = "Код практиканта;Номер месяца;Оценка стажировки;Интерес;Ясность задач;Поддержка;Отзыв\n" +
	"I-01;3;2;2;2;2;Задачи однотипные, думаю уйти\n"

// upload — multipart с файлом и дополнительными полями.
func upload(h http.Handler, path, name string, data []byte, fields ...string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(data)
	for i := 0; i+1 < len(fields); i += 2 {
		mw.WriteField(fields[i], fields[i+1])
	}
	mw.Close()
	r := httptest.NewRequest("POST", path, &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

type previewResp struct {
	Sheets []struct {
		Name    string `json:"name"`
		Role    string `json:"role"`
		Rows    int    `json:"rows"`
		Columns []struct {
			Index   int      `json:"index"`
			Title   string   `json:"title"`
			Target  *string  `json:"target"`
			Samples []string `json:"samples"`
		} `json:"columns"`
	} `json:"sheets"`
	Targets []struct{ Value, Label, Role string } `json:"targets"`
}

func TestImportPreviewDoesNotSave(t *testing.T) {
	a, h := newApp(t, nil)
	before := len(a.Store.Responses())
	data, err := os.ReadFile("../../data/sample/ответы_стажеров.xlsx")
	if err != nil {
		t.Skip("sample not generated")
	}
	var pv previewResp
	decode(t, upload(h, "/api/v1/imports/preview", "ответы_стажеров.xlsx", data), &pv)
	if len(pv.Sheets) != 1 || pv.Sheets[0].Role != "intern" || pv.Sheets[0].Rows == 0 || len(pv.Targets) != 14 {
		t.Fatalf("%+v", pv)
	}
	for _, c := range pv.Sheets[0].Columns {
		if c.Target == nil || len(c.Samples) == 0 {
			t.Fatalf("sample file must map fully: %+v", c)
		}
	}
	var raw map[string]any
	decode(t, upload(h, "/api/v1/imports/preview", "чужой.csv", []byte(foreignCSV)), &raw)
	cols := raw["sheets"].([]any)[0].(map[string]any)["columns"].([]any)
	if cols[0].(map[string]any)["target"] != nil {
		t.Fatalf("unknown column must have target null: %v", cols[0])
	}
	if len(a.Store.Responses()) != before {
		t.Fatal("preview must not save")
	}
	if w := upload(h, "/api/v1/imports/preview", "a.csv", []byte("a,b\n1,2\n"), "role", "boss"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := do(h, "POST", "/api/v1/imports/preview", "x", "Content-Type", "text/plain"); w.Code != 400 {
		t.Fatal(w.Code)
	}
}

func TestImportMappingRememberExport(t *testing.T) {
	a, h := newApp(t, nil)
	mapping := `{"чужой.csv":{"role":"intern","columns":{"0":"intern_id","1":"period","2":"i_overall","3":"i_interest","4":"i_clarity","5":"i_support","6":"i_comment"}}}`
	if w := upload(h, "/api/v1/imports", "чужой.csv", []byte(foreignCSV), "mapping", `{"чужой.csv":{"role":"intern","columns":{"0":"m_speed"}}}`); w.Code != 400 {
		t.Fatal("question of other role", w.Code)
	}
	var res struct {
		Report struct {
			Accepted int `json:"accepted"`
		} `json:"report"`
		Changed    int `json:"changed"`
		Remembered int `json:"remembered"`
	}
	decode(t, upload(h, "/api/v1/imports", "чужой.csv", []byte(foreignCSV), "mapping", mapping, "remember", "true"), &res)
	if res.Report.Accepted != 1 || res.Changed != 1 || res.Remembered != 4 {
		t.Fatalf("%+v", res)
	}
	for _, r := range a.Store.Responses() {
		if r.ID == "P1-intern-M3" && (r.Scores["i_clarity"] != 2 || !strings.Contains(r.Comments["i_comment"], "однотипные")) {
			t.Fatalf("%+v", r)
		}
	}
	// без mapping тот же файл теперь распознаётся сам
	decode(t, upload(h, "/api/v1/imports", "чужой.csv", []byte(foreignCSV)), &res)
	if res.Report.Accepted != 1 || res.Changed != 0 || res.Remembered != 0 {
		t.Fatalf("%+v", res)
	}
	w := do(h, "GET", "/api/v1/survey/export", "")
	if w.Code != 200 || w.Header().Get("Content-Disposition") != `attachment; filename="survey.json"` {
		t.Fatal(w.Code, w.Header())
	}
	var sv survey.Survey
	decode(t, w, &sv)
	if err := sv.Validate(); err != nil {
		t.Fatal(err)
	}
	if q, _ := sv.Q("i_clarity"); q.Aliases[len(q.Aliases)-1] != "Ясность задач" || sv.IDColumns.Intern[len(sv.IDColumns.Intern)-1] != "Код практиканта" {
		t.Fatalf("%+v", sv.IDColumns)
	}
}

func TestImportRememberRollbackOnSaveFailure(t *testing.T) {
	a, h := newApp(t, nil)
	dir := t.TempDir()
	a.Store = store.New(survey.Default(), filepath.Join(dir, "blocked", "state.json"))
	os.WriteFile(filepath.Join(dir, "blocked"), []byte("file, not dir"), 0o600)
	mapping := `{"чужой.csv":{"role":"intern","columns":{"0":"intern_id","1":"period","4":"i_clarity"}}}`
	if w := upload(h, "/api/v1/imports", "чужой.csv", []byte(foreignCSV), "mapping", mapping, "remember", "true"); w.Code != 500 {
		t.Fatal(w.Code, w.Body.String())
	}
	if q, _ := a.Store.Survey().Q("i_clarity"); len(q.Aliases) != 2 {
		t.Fatalf("aliases must roll back: %v", q.Aliases)
	}
}

type errLLM struct{}

func (errLLM) Complete(context.Context, []llm.Message) (string, error) { return "", errors.New("down") }

type checkLLM struct{}

func (checkLLM) Complete(_ context.Context, msgs []llm.Message) (string, error) {
	if strings.Contains(msgs[1].Content, CheckSample) {
		return "```json\n{\"sentiment\":-0.7,\"themes\":[\"routine\",\"no_support\",\"attrition\"],\"quote\":\"стоит ли продолжать\"}\n```", nil
	}
	return `{"sentiment":0,"themes":[],"quote":""}`, nil
}

type lieLLM struct{}

func (lieLLM) Complete(context.Context, []llm.Message) (string, error) {
	return `{"sentiment":-1,"themes":["routine"],"quote":"наставник груб"}`, nil
}

func TestLLMCheck(t *testing.T) {
	var out struct {
		Mode      string           `json:"mode"`
		Model     string           `json:"model"`
		OK        bool             `json:"ok"`
		LatencyMS *int64           `json:"latency_ms"`
		Sample    string           `json:"sample"`
		Analysis  *json.RawMessage `json:"analysis"`
		Error     *string          `json:"error"`
	}
	_, h := newApp(t, nil)
	decode(t, do(h, "POST", "/api/v1/llm/check", ""), &out)
	if out.Mode != "offline" || out.OK || out.LatencyMS != nil || out.Analysis != nil || out.Error == nil || *out.Error != OfflineCheckError || out.Sample != CheckSample {
		t.Fatalf("%+v", out)
	}
	for _, c := range []struct {
		client llm.Client
		ok     bool
		err    string
	}{{checkLLM{}, true, ""}, {lieLLM{}, false, "цитаты нет"}, {errLLM{}, false, "недоступна"}} {
		_, h := newApp(t, c.client)
		out.Error, out.Analysis = nil, nil
		decode(t, do(h, "POST", "/api/v1/llm/check", ""), &out)
		if out.Mode != "llm" || out.Model != "m" || out.OK != c.ok || out.LatencyMS == nil || out.Analysis == nil {
			t.Fatalf("%T: %+v", c.client, out)
		}
		if c.ok && (out.Error != nil || !strings.Contains(string(*out.Analysis), `"analyzer":"llm:m"`)) {
			t.Fatalf("%s", *out.Analysis)
		}
		if !c.ok && (out.Error == nil || !strings.Contains(*out.Error, c.err)) {
			t.Fatalf("%T: %v", c.client, out.Error)
		}
	}
}

func webApp(t *testing.T) (http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><div id=root>SPA</div>"), 0o644)
	os.WriteFile(filepath.Join(dir, "assets", "app-1a2b.js"), []byte("console.log(1)"), 0o644)
	os.WriteFile(filepath.Join(dir, "..", "secret.txt"), []byte("secret"), 0o644)
	cfg := config.Config{CORSOrigins: []string{"http://localhost:5173"}, DemoMode: true, WebDir: dir, HRToken: "tok"}
	sv := survey.Default()
	a := New(cfg, store.New(sv, ""), sv, nil)
	if err := a.Seed(); err != nil {
		t.Fatal(err)
	}
	return a.Handler(), dir
}

func TestStaticSPA(t *testing.T) {
	h, _ := webApp(t)
	for _, c := range []struct {
		path string
		code int
		body string
	}{
		{"/", 200, "SPA"},
		{"/pairs/P2", 200, "SPA"},
		{"/form/abc", 200, "SPA"},
		{"/index.html", 200, "SPA"},
		{"/assets/app-1a2b.js", 200, "console.log"},
		{"/assets/missing.js", 404, ""},
		{"/../secret.txt", 301, ""}, // ServeMux сам чистит путь редиректом
		{"/secret.txt", 404, ""},
		{"/..%5csecret.txt", 404, ""},
		{"/api/v1/nope", 404, `"not_found"`},
		{"/api", 404, `"not_found"`},
		{"/api/v1/overview", 401, "unauthorized"},
		{"/health", 200, "ok"},
	} {
		auth := "Bearer tok"
		if c.code == 401 {
			auth = ""
		}
		w := do(h, "GET", c.path, "", "Authorization", auth)
		statusOK := w.Code == c.code
		if c.code == http.StatusMovedPermanently {
			// Go versions use either 301 or 307 for the same canonical path.
			statusOK = (statusOK || w.Code == http.StatusTemporaryRedirect) && w.Header().Get("Location") == "/secret.txt"
		}
		if !statusOK || !strings.Contains(w.Body.String(), c.body) {
			t.Fatalf("%s: %d %s", c.path, w.Code, w.Body.String())
		}
		if c.code == 200 && strings.HasPrefix(c.path, "/assets/") && !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
			t.Fatal("assets cache")
		}
	}
	if w := do(h, "POST", "/pairs/P2", ""); w.Code != 404 {
		t.Fatal("static only for GET", w.Code)
	}
	if w := do(h, "GET", "/api/v1/overview", "", "Authorization", "Bearer tok"); w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "json") {
		t.Fatal("api must not be intercepted", w.Code)
	}
	// интерфейс с того же origin может писать: Origin совпадает с Host
	if w := do(h, "POST", "/api/v1/demo/reset", "", "Authorization", "Bearer tok", "Origin", "http://example.com"); w.Code != 200 {
		t.Fatal("same origin write", w.Code)
	}
	if w := do(h, "POST", "/api/v1/demo/reset", "", "Authorization", "Bearer tok", "Origin", "http://evil.test"); w.Code != 403 {
		t.Fatal("foreign origin write", w.Code)
	}
}

func TestStaticTraversalDirect(t *testing.T) {
	dir := t.TempDir()
	web := filepath.Join(dir, "web")
	os.MkdirAll(web, 0o755)
	os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("secret"), 0o644)
	a := &App{webDir: web}
	for _, p := range []string{"/../secret.txt", "/..\\secret.txt", "/C:/secret.txt"} {
		r := httptest.NewRequest("GET", "/", nil)
		r.URL.Path = p
		w := httptest.NewRecorder()
		a.static(w, r)
		if w.Code != 404 || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("%s: %d %s", p, w.Code, w.Body.String())
		}
	}
}

func TestStaticWithoutIndex(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{WebDir: dir}
	sv := survey.Default()
	h := New(cfg, store.New(sv, ""), sv, nil).Handler()
	if w := do(h, "GET", "/pairs/P1", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := do(h, "GET", "/", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
}

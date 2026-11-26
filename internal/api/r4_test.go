package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/config"
	"github.com/ElzaraA/mgts-ai/internal/signals"
	"github.com/ElzaraA/mgts-ai/internal/store"
	"github.com/ElzaraA/mgts-ai/internal/survey"
)

type qualityResp struct {
	Report struct {
		Current int `json:"current"`
		Quality struct {
			GroundTruth string          `json:"ground_truth"`
			Evaluation  json.RawMessage `json:"evaluation"`
			TextAI      struct {
				Analyzer   string `json:"analyzer"`
				Model      string `json:"model"`
				PromptHash string `json:"prompt_hash"`
			} `json:"text_ai"`
			Goldset      json.RawMessage `json:"goldset"`
			ActionEffect struct {
				WithAction int `json:"with_action"`
			} `json:"action_effect"`
			Caveat string `json:"caveat"`
		} `json:"quality"`
		Actions []signals.ActionRow `json:"actions"`
	} `json:"report"`
}

// GET /overview → report.quality: на синтетике истина заложена в данные, gold-set читается из DATA_DIR.
func TestOverviewQuality(t *testing.T) {
	a, h := newApp(t, nil)
	var ov qualityResp
	decode(t, do(h, "GET", "/api/v1/overview", ""), &ov)
	q := ov.Report.Quality
	if q.GroundTruth != "synthetic" || string(q.Evaluation) == "null" || q.TextAI.Analyzer != "offline" || q.TextAI.PromptHash == "" ||
		string(q.Goldset) != "null" || q.Caveat == "" {
		t.Fatalf("%+v", q)
	}
	dir := t.TempDir()
	a.Cfg.DataDir = dir
	if err := os.WriteFile(filepath.Join(dir, GoldsetFile), []byte(`{"date":"2026-11-19","macro_f1":0.8}`), 0o600); err != nil {
		t.Fatal(err)
	}
	a.LLM = a.Offline
	decode(t, do(h, "GET", "/api/v1/overview", ""), &ov)
	if string(ov.Report.Quality.Goldset) != `{"date":"2026-11-19","macro_f1":0.8}` || ov.Report.Quality.TextAI.Analyzer != "llm" {
		t.Fatalf("%s", ov.Report.Quality.Goldset)
	}
	// каталог вместо файла — ошибка чтения не роняет ответ
	_ = os.Remove(filepath.Join(dir, GoldsetFile))
	_ = os.Mkdir(filepath.Join(dir, GoldsetFile), 0o755)
	if w := do(h, "GET", "/api/v1/overview", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
}

// Вердикт с действием: месяц решения — текущий месяц программы, действие попадает в report.actions.
func TestVerdictAction(t *testing.T) {
	_, h := newApp(t, nil)
	if w := do(h, "POST", "/api/v1/signals/P4-S8/verdict", `{"status":"confirmed","action":"поговорили с наставником"}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	long := make([]rune, 201)
	for i := range long {
		long[i] = 'а'
	}
	if w := do(h, "POST", "/api/v1/signals/P4-S8/verdict", `{"status":"confirmed","action":"`+string(long)+`"}`); w.Code != 400 {
		t.Fatal("action too long")
	}
	var ov qualityResp
	decode(t, do(h, "GET", "/api/v1/overview", ""), &ov)
	if len(ov.Report.Actions) != 1 || ov.Report.Actions[0].Period != ov.Report.Current || ov.Report.Actions[0].Action != "поговорили с наставником" ||
		ov.Report.Quality.ActionEffect.WithAction != 1 {
		t.Fatalf("%+v", ov.Report.Actions)
	}
}

// Новые пороги проверяются на границы; старый фронтенд без новых полей получает значения по умолчанию.
func TestReportsUseConfiguredComparisonThreshold(t *testing.T) {
	_, h := newApp(t, nil)
	if response := do(h, "PUT", "/api/v1/thresholds", `{"low_score":3.5}`); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var overview struct{ Report signals.ProgramReport }
	decode(t, do(h, "GET", "/api/v1/overview", ""), &overview)
	var pair struct {
		LowScore float64 `json:"low_score"`
		Report   signals.PairReport
	}
	decode(t, do(h, "GET", "/api/v1/pairs/P2", ""), &pair)
	if overview.Report.LowScore != 3.5 || pair.LowScore != 3.5 || !strings.Contains(overview.Report.Digest, "ниже 3,5") {
		t.Fatalf("overview=%v, pair=%v, digest=%s", overview.Report.LowScore, pair.LowScore, overview.Report.Digest)
	}
	if pair.Report.Detection.Naive == 0 {
		t.Fatal("P2 should cross the adjusted comparison threshold")
	}
}

func TestThresholdsR4(t *testing.T) {
	_, h := newApp(t, nil)
	for _, c := range []struct {
		body string
		code int
	}{
		{`{"z_drop":0.5}`, 400}, {`{"z_drop":6}`, 400}, {`{"decline_from_base":0.05}`, 400}, {`{"decline_from_base":2.5}`, 400},
		{`{"cost_ratio":0.1}`, 400}, {`{"cost_ratio":200}`, 400}, {`{"cost_ratio":30,"z_drop":2.5,"decline_from_base":0.75}`, 200},
		{`{"gap_min":1.2,"decline_from_peak":0.8}`, 200}, // старый фронтенд: лишнее поле игнорируется, новые — по умолчанию
	} {
		if w := do(h, "PUT", "/api/v1/thresholds", c.body); w.Code != c.code {
			t.Errorf("%s → %d, want %d", c.body, w.Code, c.code)
		}
	}
	var th struct {
		Thresholds map[string]float64 `json:"thresholds"`
	}
	decode(t, do(h, "GET", "/api/v1/thresholds", ""), &th)
	if _, old := th.Thresholds["decline_from_peak"]; old || th.Thresholds["z_drop"] != 3 || th.Thresholds["gap_min"] != 1.2 {
		t.Fatalf("%v", th.Thresholds)
	}
}

// Загруженные данные (не синтетика): истина — решения HR; без решений оценки нет.
func TestQualityHRTruth(t *testing.T) {
	sv := survey.Default()
	a := New(config.Config{LLMModel: "m"}, store.New(sv, ""), sv, nil)
	if onsetFor(nil) != nil {
		t.Fatal("no data — no onset")
	}
	if err := a.Seed(); err != nil {
		t.Fatal(err)
	}
	rs := a.Store.Responses()
	rs[0].Source = "import"
	if onsetFor(rs) != nil || onsetFor(rs[1:]) == nil {
		t.Fatal("onset only for fully synthetic data")
	}
}

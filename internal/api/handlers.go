package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ElzaraA/mgts-ai/internal/ingest"
	"github.com/ElzaraA/mgts-ai/internal/signals"
	"github.com/ElzaraA/mgts-ai/internal/survey"
	"github.com/ElzaraA/mgts-ai/internal/synth"
	"github.com/ElzaraA/mgts-ai/internal/textai"
)

func (a *App) input() signals.Input {
	in := a.Store.Input()
	if in.WindowPeriod > 0 {
		// Когда HR вновь открывает прошлый месяц, ответы за более поздние месяцы
		// сохраняются в хранилище и возвращаются в отчёт после выбора их периода.
		in.Responses = slices.DeleteFunc(in.Responses, func(r survey.Response) bool { return r.Period > in.WindowPeriod })
	}
	in.Now = time.Now()
	in.Onset = onsetFor(in.Responses)
	return in
}

// onsetFor — истина для оценки системы: если все ответы синтетические, эпизоды заложены в данные (synth.Onset);
// иначе nil — истина берётся из решений HR.
func onsetFor(rs []survey.Response) map[string][]signals.Episode {
	if len(rs) == 0 {
		return nil
	}
	for _, r := range rs {
		if r.Source != "synthetic" {
			return nil
		}
	}
	return synth.Onset
}

// GoldsetFile — результат последнего прогона gold-set (cmd/textaieval) в DATA_DIR.
const GoldsetFile = "goldset_result.json"

// withQuality дополняет report.quality тем, что знает только сервер: режим анализатора и прогон gold-set.
func (a *App) withQuality(rep *signals.ProgramReport) {
	q := &rep.Quality.TextAI
	if a.LLM == nil {
		q.Analyzer, q.Model = "offline", ""
	} else {
		q.Analyzer, q.Model = "llm", a.Cfg.LLMModel
	}
	b, err := os.ReadFile(filepath.Join(a.Cfg.DataDir, GoldsetFile))
	if err == nil && json.Valid(b) {
		rep.Quality.Goldset = json.RawMessage(b)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("goldset result not read", "err", err)
	}
}

func (a *App) report() signals.ProgramReport {
	return signals.Analyze(a.input(), a.Store.Verdicts())
}

// overview — всё для главного экрана: KPI, динамика программы, пары с тепловой картой и сводка для руководства.
func (a *App) overview(w http.ResponseWriter, _ *http.Request) {
	in := a.input()
	rep := signals.Analyze(in, a.Store.Verdicts())
	a.withQuality(&rep)
	writeJSON(w, 200, map[string]any{"report": rep, "analyzer": a.analyzerInfo(), "data_origin": responseOrigin(in.Responses),
		"themes_over_time": signals.ThemesOverTime(in), "text_review": textReview(in, rep)})
}

func responseOrigin(rows []survey.Response) string {
	synthetic := 0
	for _, row := range rows {
		if row.Source == "synthetic" {
			synthetic++
		}
	}
	switch {
	case len(rows) == 0:
		return "empty"
	case synthetic == len(rows):
		return "synthetic"
	case synthetic == 0:
		return "submitted"
	default:
		return "mixed"
	}
}

// brief — материалы HR к встрече 1:1: активные сигналы с цитатами, вопросы стажёру и наставнику, тезисы.
func (a *App) brief(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for _, p := range a.report().Pairs {
		if p.PairID == id {
			writeJSON(w, 200, signals.MakeBrief(p, time.Now()))
			return
		}
	}
	writeError(w, 404, "not_found", "пара не найдена")
}

func (a *App) analyzerInfo() map[string]any {
	if a.LLM == nil {
		return map[string]any{"mode": "offline", "label": "Словарь тем (без модели)", "model": ""}
	}
	return map[string]any{"mode": "llm", "label": "Модель + проверка цитат", "model": a.Cfg.LLMModel}
}

type commentView struct {
	ResponseID string          `json:"response_id"`
	Role       string          `json:"role"`
	Period     int             `json:"period"`
	Question   string          `json:"question"`
	Text       string          `json:"text"`
	Analysis   textai.Analysis `json:"analysis"`
}

type questionSeries struct {
	Code      string     `json:"code"`
	Role      string     `json:"role"`
	Text      string     `json:"text"`
	Dimension string     `json:"dimension"`
	Mirror    string     `json:"mirror,omitempty"`
	Values    []*float64 `json:"values"`
}

// pair — карточка пары: динамика по вопросам, расхождения, сигналы, комментарии с разбором и исходные ответы.
func (a *App) pair(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	in := a.input()
	rep := signals.Analyze(in, a.Store.Verdicts())
	var pr *signals.PairReport
	for i := range rep.Pairs {
		if rep.Pairs[i].PairID == id {
			pr = &rep.Pairs[i]
		}
	}
	if pr == nil {
		writeError(w, 404, "not_found", "пара не найдена")
		return
	}
	views := signals.Build(in)
	v := views[id]
	qs := []questionSeries{}
	for _, q := range in.Survey.Questions {
		if q.Kind != "scale" {
			continue
		}
		qs = append(qs, questionSeries{Code: q.Code, Role: string(q.Role), Text: q.Text, Dimension: q.Dimension, Mirror: q.Mirror, Values: ptrs(v.Questions[q.Code])})
	}
	comments := []commentView{}
	responses := []survey.Response{}
	for _, resp := range in.Responses {
		if resp.PairID != id {
			continue
		}
		responses = append(responses, resp)
		for code, text := range resp.Comments {
			an := in.Analyses[resp.ID+"/"+code]
			an.ByTheme = nil
			if an.Themes == nil {
				an.Themes = []string{}
			}
			comments = append(comments, commentView{ResponseID: resp.ID, Role: string(resp.Role), Period: resp.Period, Question: code, Text: text, Analysis: an})
		}
	}
	sort.Slice(comments, func(i, j int) bool {
		if comments[i].Period != comments[j].Period {
			return comments[i].Period > comments[j].Period
		}
		return comments[i].Role < comments[j].Role
	})
	writeJSON(w, 200, map[string]any{"report": pr, "current": rep.Current, "periods": rep.Periods, "low_score": rep.LowScore, "questions": qs,
		"comments": comments, "responses": responses, "themes": themeLabels()})
}

func themeLabels() map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, t := range textai.Themes {
		out[t.Code] = map[string]any{"label": t.Label, "negative": t.Negative}
	}
	return out
}

func ptrs(s []float64) []*float64 {
	out := make([]*float64, len(s))
	for i, x := range s {
		if !math.IsNaN(x) {
			v := math.Round(x*100) / 100
			out[i] = &v
		}
	}
	return out
}

type flatSignal struct {
	signals.SignalView
	Intern string `json:"intern"`
	Mentor string `json:"mentor"`
	Track  string `json:"track,omitempty"`
	Active bool   `json:"active"`
}

// groupedSignal — главный сигнал пары и полные объекты связанных с ним (view=grouped).
type groupedSignal struct {
	flatSignal
	RelatedSignals []flatSignal `json:"related_signals"`
}

// signalList — лента сигналов (активные и снятые) для экрана «Сигналы».
// view=grouped (по умолчанию): по одному главному сигналу на пару и состояние, остальные — в related_signals.
// view=all: плоский список, как раньше. Фильтр severity применяется к главным сигналам.
func (a *App) signalList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sev, state, view := q.Get("severity"), q.Get("state"), q.Get("view")
	if view == "" {
		view = "grouped"
	}
	if view != "grouped" && view != "all" {
		writeError(w, 400, "bad_request", "view: grouped | all")
		return
	}
	rep := a.report()
	out := []groupedSignal{}
	for _, p := range rep.Pairs {
		for _, list := range []struct {
			items  []signals.SignalView
			active bool
		}{{p.Signals, true}, {p.Resolved, false}} {
			if state == "active" && !list.active || state == "past" && list.active {
				continue
			}
			flat := func(s signals.SignalView) flatSignal {
				return flatSignal{SignalView: s, Intern: p.Intern, Mentor: p.Mentor, Track: p.Track, Active: list.active}
			}
			for _, s := range list.items {
				if sev != "" && string(s.Severity) != sev || view == "grouped" && !s.Primary {
					continue
				}
				g := groupedSignal{flatSignal: flat(s)}
				if view == "grouped" {
					g.RelatedSignals = []flatSignal{}
					for _, o := range list.items {
						if !o.Primary {
							g.RelatedSignals = append(g.RelatedSignals, flat(o))
						}
					}
				}
				out = append(out, g)
			}
		}
	}
	rank := map[signals.Severity]int{signals.High: 3, signals.Medium: 2, signals.Low: 1}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Active != out[j].Active {
			return out[i].Active
		}
		if rank[out[i].Severity] != rank[out[j].Severity] {
			return rank[out[i].Severity] > rank[out[j].Severity]
		}
		return out[i].ID < out[j].ID
	})
	var list any = out
	if view == "all" {
		flat := make([]flatSignal, len(out))
		for i, g := range out {
			flat[i] = g.flatSignal
		}
		list = flat
	}
	writeJSON(w, 200, map[string]any{"signals": list, "view": view, "current": rep.Current, "catalog": signals.Types})
}

// verdict — решение HR: подтверждён / ложный / решён. Вердикт привязан к ID сигнала (пара + тип).
func (a *App) verdict(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Status  string `json:"status"`
		Comment string `json:"comment"`
		Action  string `json:"action"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil {
		badBody(w, err)
		return
	}
	switch body.Status {
	case "", "confirmed", "false", "resolved":
	default:
		writeError(w, 400, "bad_request", "status: confirmed | false | resolved")
		return
	}
	if len([]rune(body.Comment)) > 500 {
		writeError(w, 400, "bad_request", "комментарий длиннее 500 символов")
		return
	}
	if len([]rune(body.Action)) > 200 {
		writeError(w, 400, "bad_request", "действие длиннее 200 символов")
		return
	}
	known := false
	episode := 0
	rep := a.report()
	for _, p := range rep.Pairs {
		for _, s := range append(append([]signals.SignalView{}, p.Signals...), p.Resolved...) {
			if s.ID == id {
				known = true
				episode = s.Episode
			}
		}
	}
	if !known {
		writeError(w, 404, "not_found", "сигнал не найден")
		return
	}
	// месяц решения — текущий месяц программы: с ним сравнивается индекс в следующих месяцах
	v, err := a.Store.DecideInEpisode(id, body.Status, strings.TrimSpace(body.Comment), strings.TrimSpace(body.Action), rep.Current, episode)
	if err != nil {
		slog.Error("verdict not saved", "id", id, "err", err)
		writeError(w, 500, "internal", "не удалось сохранить")
		return
	}
	a.Metrics.Inc("pulse_verdicts_total", "Решения HR по сигналам", "status", body.Status)
	a.Broker.Publish(SSEEvent{Event: "data.updated", Data: `{"verdict":true}`})
	writeJSON(w, 200, v)
}

func (a *App) getThresholds(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"thresholds": a.Store.Thresholds(), "defaults": signals.Default})
}

// putThresholds — HR правит пороги в интерфейсе; значения проверяются на разумные границы.
func (a *App) putThresholds(w http.ResponseWriter, r *http.Request) {
	var t signals.Thresholds
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&t); err != nil {
		badBody(w, err)
		return
	}
	bad := t.DeclineSlope >= 0 || t.DeclineSlope < -2 || t.ZDrop < 1 || t.ZDrop > 5 ||
		t.DeclineFromBase < 0.1 || t.DeclineFromBase > 2 || t.CostRatio < 0.5 || t.CostRatio > 100 || t.LowScore < 1 || t.LowScore > 5 ||
		t.GapMin <= 0 || t.GapHigh < t.GapMin || t.GapHigh > 4 || t.Persist < 1 || t.Persist > 6 || t.BlindSelf < 1 || t.BlindSelf > 5 ||
		t.BlindOther < 1 || t.BlindOther > 5 || t.Diverge <= 0 || t.LowDimension < 1 || t.LowDimension > 5 || t.InterestDrop <= 0 ||
		t.NegSentiment < -1 || t.NegSentiment > 0 || t.MentorLow < 1 || t.MentorLow > 5 || t.ShortRatio <= 0 || t.ShortRatio >= 1
	if bad {
		writeError(w, 400, "bad_request", "порог вне допустимого диапазона")
		return
	}
	if err := a.Store.SetThresholds(t); err != nil {
		slog.Error("thresholds not saved", "err", err)
		writeError(w, 500, "internal", "не удалось сохранить")
		return
	}
	a.Broker.Publish(SSEEvent{Event: "data.updated", Data: `{"thresholds":true}`})
	writeJSON(w, 200, map[string]any{"thresholds": t})
}

func (a *App) surveyInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, a.Store.Survey())
}

// sources — экран «Источники данных»: ссылки на анкеты, вебхук, режим анализатора, журнал поступлений.
func (a *App) sources(w http.ResponseWriter, _ *http.Request) {
	sv := a.Store.Survey()
	tokens := a.Store.Tokens()
	type link struct {
		Pair   string `json:"pair"`
		Person string `json:"person"`
		Role   string `json:"role"`
		URL    string `json:"url"`
		Token  string `json:"token"`
	}
	links := []link{}
	for _, p := range sv.Participant {
		for _, x := range []struct{ person, role string }{{p.Intern, "intern"}, {p.Mentor, "mentor"}} {
			t := tokens[x.person]
			links = append(links, link{Pair: p.ID, Person: x.person, Role: x.role, Token: t, URL: a.Cfg.PublicURL + "/form/" + t})
		}
	}
	counts := map[string]int{}
	for _, r := range a.Store.Responses() {
		counts[r.Source]++
	}
	writeJSON(w, 200, map[string]any{
		"links": links, "events": a.Store.Events(), "counts": counts, "analyzer": a.analyzerInfo(),
		"webhook":     map[string]any{"path": "/api/v1/integrations/yandex-forms", "secret_set": a.Cfg.WebhookSecret != ""},
		"demo":        a.Cfg.DemoMode,
		"form_window": a.Store.FormWindow(), "periods": sv.Periods,
	})
}

// readUpload читает multipart-файл (поле file, до 10 МБ) и роль листа. false — ответ с ошибкой уже записан.
func readUpload(w http.ResponseWriter, r *http.Request) ([]ingest.Table, string, survey.Role, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, 400, "bad_request", "ожидается multipart/form-data с полем file (до 10 МБ)")
		return nil, "", "", false
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeError(w, 400, "bad_request", "нет файла")
		return nil, "", "", false
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		writeError(w, 400, "bad_request", "не удалось прочитать файл")
		return nil, "", "", false
	}
	tables, err := ingest.ReadFile(hdr.Filename, data)
	if errors.Is(err, ingest.ErrTooLarge) {
		writeError(w, 400, "bad_request", err.Error())
		return nil, "", "", false
	}
	if err != nil {
		writeError(w, 400, "bad_request", "файл не читается как CSV или XLSX: "+err.Error())
		return nil, "", "", false
	}
	role := survey.Role(r.FormValue("role"))
	if role != "" && role != survey.Intern && role != survey.Mentor {
		writeError(w, 400, "bad_request", "role: intern | mentor")
		return nil, "", "", false
	}
	return tables, hdr.Filename, role, true
}

// importPreview — как файл будет прочитан: листы, предложенное сопоставление колонок и примеры значений.
// Ничего не сохраняет: HR проверяет и при необходимости правит сопоставление до загрузки.
func (a *App) importPreview(w http.ResponseWriter, r *http.Request) {
	tables, _, role, ok := readUpload(w, r)
	if !ok {
		return
	}
	sv := a.Store.Survey()
	writeJSON(w, 200, map[string]any{"sheets": ingest.Preview(tables, sv, role), "targets": ingest.Targets(sv)})
}

// importFile — загрузка выгрузки (CSV или XLSX). Роль листа определяется по заголовкам или задаётся полем role.
// mapping (JSON) — ручное сопоставление колонок по листам; remember=true — запомнить заголовки как aliases.
func (a *App) importFile(w http.ResponseWriter, r *http.Request) {
	tables, filename, role, ok := readUpload(w, r)
	if !ok {
		return
	}
	remember := r.FormValue("remember") == "true"
	// импорт целиком под мьютексом: два одновременных файла не выдадут новым парам одинаковые ID
	a.importMu.Lock()
	defer a.importMu.Unlock()
	sv := a.Store.Survey()
	mapping, err := ingest.ParseMapping(r.FormValue("mapping"), sv)
	if err != nil {
		writeError(w, 400, "bad_request", err.Error())
		return
	}
	learned := 0
	if remember && len(mapping) > 0 {
		learned = ingest.Remember(tables, &sv, mapping)
	}
	previousStart := sv.StartMonth
	origin := "import"
	if r.FormValue("source") == "yandex-forms" {
		origin = "yandex-forms"
	}
	rep := ingest.ImportMapped(tables, &sv, role, mapping, origin)
	if origin == "yandex-forms" {
		if rep.Rejected > 0 {
			writeError(w, 400, "bad_request", "Есть отклонённые ответы Яндекс Форм")
			return
		}
		for _, row := range rep.Responses {
			for _, q := range sv.ByRole(row.Role) {
				if _, ok := row.Scores[q.Code]; q.Kind == "scale" && !ok {
					writeError(w, 400, "bad_request", "В ответах Яндекс Форм отсутствует обязательная оценка")
					return
				}
			}
		}
	}
	var newSv *survey.Survey
	if len(rep.NewPairs) > 0 || learned > 0 || sv.StartMonth != previousStart {
		newSv = &sv
	}
	changed, err := a.Store.ApplyImport(newSv, rep.Responses, "Импорт "+filename)
	if err != nil {
		slog.Error("import not saved", "file", filename, "err", err)
		writeError(w, 500, "internal", "не удалось сохранить импорт, данные не изменены")
		return
	}
	if learned > 0 {
		a.Store.Log("import", fmt.Sprintf("Запомнено вариантов заголовков: %d (выгрузить: GET /api/v1/survey/export)", learned))
	}
	a.analyze(changed)
	a.Metrics.Inc("pulse_imports_total", "Загрузки файлов")
	writeJSON(w, 200, map[string]any{"report": rep, "changed": len(changed), "unchanged": rep.Accepted - len(changed), "remembered": learned})
}

// surveyExport — текущая структура анкет с выученными aliases: файл кладут в data/survey.json,
// и сопоставление переживает сброс состояния.
func (a *App) surveyExport(w http.ResponseWriter, _ *http.Request) {
	b, err := json.MarshalIndent(a.Store.Survey(), "", "  ")
	if err != nil {
		writeError(w, 500, "internal", "не удалось выгрузить")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="survey.json"`)
	_, _ = w.Write(append(b, '\n'))
}

func (a *App) reset(w http.ResponseWriter, _ *http.Request) {
	if !a.Cfg.DemoMode {
		writeError(w, 403, "forbidden", "сброс доступен только в демо-режиме")
		return
	}
	if err := a.Seed(); err != nil {
		slog.Error("demo reset failed", "err", err)
		writeError(w, 500, "internal", "не удалось сбросить")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

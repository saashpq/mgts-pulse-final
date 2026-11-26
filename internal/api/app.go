// Package api — HTTP API «Пульса стажировки» для единого HR-интерфейса, анкеты и вебхука.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ElzaraA/mgts-ai/internal/config"
	"github.com/ElzaraA/mgts-ai/internal/llm"
	"github.com/ElzaraA/mgts-ai/internal/metrics"
	"github.com/ElzaraA/mgts-ai/internal/privacy"
	"github.com/ElzaraA/mgts-ai/internal/store"
	"github.com/ElzaraA/mgts-ai/internal/survey"
	"github.com/ElzaraA/mgts-ai/internal/synth"
	"github.com/ElzaraA/mgts-ai/internal/textai"
)

type App struct {
	Cfg      config.Config
	Store    *store.Store
	Offline  textai.Offline
	LLM      textai.Analyzer // nil — модель не подключена
	Broker   *Broker
	Metrics  *metrics.Registry
	jobs     chan job
	wg       sync.WaitGroup
	cancel   context.CancelFunc
	seedFile survey.Survey
	// RefillEvery — как часто фоновый «добор» ставит в очередь комментарии, не попавшие в неё при переполнении.
	RefillEvery time.Duration
	checkMu     sync.Mutex // проверка модели — по одной
	webDir      string     // каталог собранного фронтенда; пусто — статику не отдаём
	importMu    sync.Mutex // импорт целиком последовательный: два файла одновременно не создадут одинаковые пары
	queuedMu    sync.Mutex
	queued      map[string]string // ключ комментария в очереди модели → текст последней постановки
}

// PendingNote — заметка разбора, который ещё ждёт модель. По ней фоновый добор находит пропущенные комментарии.
const PendingNote = "ожидает разбора моделью"

// llmBatch — сколько готовых разборов модели сохраняются одной записью снимка.
const llmBatch = 16

type job struct {
	key, text, role string
}

// New собирает приложение. llmClient nil — только офлайн-анализатор.
func New(cfg config.Config, st *store.Store, base survey.Survey, llmClient llm.Client) *App {
	a := &App{Cfg: cfg, Store: st, Broker: NewBroker(), Metrics: metrics.New(), jobs: make(chan job, 512), seedFile: base,
		RefillEvery: 30 * time.Second, queued: map[string]string{}, webDir: cfg.WebDir}
	if llmClient != nil {
		extra, err := privacy.LoadNames(filepath.Join(cfg.DataDir, "names.json"))
		if err != nil {
			slog.Warn("names.json not loaded", "err", err)
		}
		a.LLM = textai.LLM{Client: llmClient, Model: cfg.LLMModel, Names: func() []string { return a.maskNames(extra) }}
	}
	return a
}

// maskNames — словарь для маскирования: ID всех участников пар и имена из data/names.json.
func (a *App) maskNames(extra []string) []string {
	out := append([]string(nil), extra...)
	for _, p := range a.Store.Survey().Participant {
		out = append(out, p.Intern, p.Mentor)
	}
	return out
}

// Start запускает фоновый разбор комментариев моделью (один воркер: модель в контуре — общий ресурс)
// и добор: раз в RefillEvery комментарии с заметкой PendingNote, не попавшие в переполненную очередь, ставятся снова.
func (a *App) Start(ctx context.Context) {
	ctx, a.cancel = context.WithCancel(ctx)
	if a.LLM == nil {
		return
	}
	a.wg.Add(2)
	go func() {
		defer a.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case j := <-a.jobs:
				a.work(ctx, j)
			}
		}
	}()
	go func() {
		defer a.wg.Done()
		tick := time.NewTicker(a.RefillEvery)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				a.refill()
			}
		}
	}()
}

// work разбирает задачу и всё, что уже ждёт в очереди (до llmBatch), и сохраняет результаты одной записью.
func (a *App) work(ctx context.Context, first job) {
	batch := []job{first}
drain:
	for len(batch) < llmBatch {
		select {
		case j := <-a.jobs:
			batch = append(batch, j)
		default:
			break drain
		}
	}
	// задача устарела, если комментарий успели переписать и поставить заново: её текст не разбираем и не сохраняем
	current := func(j job) bool {
		a.queuedMu.Lock()
		defer a.queuedMu.Unlock()
		return a.queued[j.key] == j.text
	}
	res := map[string]textai.Analysis{}
	for _, j := range batch {
		if ctx.Err() != nil {
			break
		}
		if !current(j) {
			continue
		}
		start := time.Now()
		an := a.LLM.Analyze(ctx, j.text, j.role)
		a.Metrics.Add("pulse_llm_seconds_sum", "Время разбора комментариев моделью", time.Since(start).Seconds())
		a.Metrics.Inc("pulse_llm_analyses_total", "Разборы комментариев моделью", "result", resultLabel(an))
		if current(j) {
			res[j.key] = an
		}
	}
	a.queuedMu.Lock()
	for _, j := range batch {
		if a.queued[j.key] == j.text {
			delete(a.queued, j.key)
		}
	}
	a.queuedMu.Unlock()
	keys := make([]string, 0, len(res))
	for k := range res {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if err := a.Store.SetAnalyses(res); err != nil {
		slog.Error("llm analyses not saved", "n", len(res), "err", err)
	}
	for _, k := range keys {
		a.Broker.Publish(SSEEvent{Event: "analysis.updated", Data: `{"key":"` + k + `"}`})
	}
}

// enqueue ставит комментарий в очередь модели без блокировки. false — очередь полна (подберёт добор).
func (a *App) enqueue(j job) bool {
	a.queuedMu.Lock()
	defer a.queuedMu.Unlock()
	if t, ok := a.queued[j.key]; ok && t == j.text {
		return true
	}
	select {
	case a.jobs <- j:
		a.queued[j.key] = j.text
		return true
	default:
		return false
	}
}

// refill — добор: комментарии, которые всё ещё ждут модель, но не стоят в очереди. Возвращает число поставленных.
func (a *App) refill() int {
	n := 0
	for _, p := range a.Store.Pending(PendingNote) {
		a.queuedMu.Lock()
		_, inQueue := a.queued[p.Key]
		a.queuedMu.Unlock()
		if inQueue {
			continue
		}
		if !a.enqueue(job{key: p.Key, text: p.Text, role: p.Role}) {
			break
		}
		n++
	}
	if n > 0 {
		slog.Info("llm queue refilled", "n", n)
	}
	return n
}

func resultLabel(a textai.Analysis) string {
	if strings.HasPrefix(a.Analyzer, "llm") {
		return "accepted"
	}
	return "fallback"
}

// Stop останавливает воркер и добор.
func (a *App) Stop() {
	if a.cancel != nil {
		a.cancel()
	}
	a.wg.Wait()
}

// Seed — синтетические данные, если состояние пустое.
func (a *App) Seed() error {
	if err := a.Store.Reset(a.seedFile, synth.Responses(a.seedFile), "synthetic"); err != nil {
		return err
	}
	a.analyze(a.allIDs())
	return nil
}

func (a *App) allIDs() []string {
	var ids []string
	for _, r := range a.Store.Responses() {
		ids = append(ids, r.ID)
	}
	return ids
}

// analyze: офлайн-разбор сразу (дашборд обновляется мгновенно), модель — в фоне.
func (a *App) analyze(ids []string) {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	offline := map[string]textai.Analysis{}
	var jobs []job
	for _, r := range a.Store.Responses() {
		if !want[r.ID] {
			continue
		}
		for code, text := range r.Comments {
			key := r.ID + "/" + code
			res := a.Offline.Analyze(context.Background(), text, string(r.Role))
			if a.LLM != nil {
				res.Note = PendingNote
				jobs = append(jobs, job{key: key, text: text, role: string(r.Role)})
			}
			offline[key] = res
		}
	}
	if err := a.Store.SetAnalyses(offline); err != nil {
		slog.Error("offline analyses not saved", "n", len(offline), "err", err)
	}
	full := 0
	for _, j := range jobs {
		if !a.enqueue(j) {
			full++
		}
	}
	if full > 0 {
		slog.Warn("llm queue full, will refill in background", "skipped", full)
	}
	if len(ids) > 0 {
		a.Broker.Publish(SSEEvent{Event: "data.updated", Data: `{"changed":` + itoa(len(ids)) + `}`})
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// Handler — все маршруты с защитными middleware.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		a.Metrics.Write(w)
	})
	mux.HandleFunc("GET /api/v1/overview", a.overview)
	mux.HandleFunc("GET /api/v1/pairs/{id}", a.pair)
	mux.HandleFunc("GET /api/v1/pairs/{id}/brief", a.brief)
	mux.HandleFunc("GET /api/v1/signals", a.signalList)
	mux.HandleFunc("POST /api/v1/signals/{id}/verdict", a.verdict)
	mux.HandleFunc("GET /api/v1/thresholds", a.getThresholds)
	mux.HandleFunc("PUT /api/v1/thresholds", a.putThresholds)
	mux.HandleFunc("GET /api/v1/survey", a.surveyInfo)
	mux.HandleFunc("PUT /api/v1/survey/window", a.setFormWindow)
	mux.HandleFunc("GET /api/v1/sources", a.sources)
	mux.HandleFunc("POST /api/v1/imports/preview", a.importPreview)
	mux.HandleFunc("POST /api/v1/imports", a.importFile)
	mux.HandleFunc("GET /api/v1/survey/export", a.surveyExport)
	mux.HandleFunc("POST /api/v1/llm/check", a.llmCheckHandler)
	mux.HandleFunc("GET /api/v1/form/{token}", a.formGet)
	mux.HandleFunc("POST /api/v1/form/{token}", a.formPost)
	mux.HandleFunc("POST /api/v1/integrations/yandex-forms", a.webhook)
	mux.HandleFunc("GET /api/v1/events", a.events)
	mux.HandleFunc("POST /api/v1/demo/reset", a.reset)
	mux.HandleFunc("/", a.static)
	return chain(mux, Recover, SecurityHeaders, CORS(a.Cfg.CORSOrigins), SameOriginWrites(a.Cfg.CORSOrigins), Observe(a.Metrics), HRAuth(a.Cfg.HRToken))
}

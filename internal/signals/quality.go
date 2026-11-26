package signals

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ElzaraA/mgts-ai/internal/textai"
)

// Источник истины для оценки системы.
const (
	TruthSynthetic = "synthetic"   // эпизоды заложены в синтетические данные (synth.Onset)
	TruthHR        = "hr_verdicts" // эпизоды — подтверждённые или решённые HR сигналы, t0 = first_seen
)

// CeilingThreshold — потолочный эффект, если максимум ставят больше 15% ответов (Terwee et al. 2007).
const CeilingThreshold = 0.15

// Quality — экран «Качество системы»: насколько можно доверять сигналам, шкалам и ИИ-разбору.
type Quality struct {
	GroundTruth  string          `json:"ground_truth"`
	Evaluation   *Evaluation     `json:"evaluation"` // null — эпизодов нет
	AlertLoad    AlertLoad       `json:"alert_load"`
	Scales       Scales          `json:"scales"`
	Agreement    *Agreement      `json:"agreement"` // null — меньше двух наблюдений с ответами обеих сторон
	ICCNote      string          `json:"icc_note"`
	TextAI       TextAIQuality   `json:"text_ai"`
	Goldset      json.RawMessage `json:"goldset"` // DATA_DIR/goldset_result.json; null — прогона не было
	ActionEffect ActionEffect    `json:"action_effect"`
	Caveat       string          `json:"caveat"`
}

// AlertLoad — нагрузка на HR.
type AlertLoad struct {
	ActiveCards    int      `json:"active_cards"`    // пар с открытым активным сигналом (одна карточка на пару)
	Per10Pairs     float64  `json:"per_10_pairs"`    // карточек на 10 пар в текущем месяце
	AttentionShare float64  `json:"attention_share"` // доля пар в статусе attention или watch
	Flapping       []string `json:"flapping"`        // сигналы, которые за горизонт появлялись ≥ 2 раз (исчезали и возвращались)
}

// CeilingRow — доля максимальных ответов по вопросу.
type CeilingRow struct {
	Question string  `json:"question"`
	Text     string  `json:"text"`
	Role     string  `json:"role"`
	Share    float64 `json:"share"`
	N        int     `json:"n"`
}

// Scales — свойства шкал 1–5.
type Scales struct {
	NoiseSigma       float64      `json:"noise_sigma"` // σ̂ шума индексов (MSSD), на ней построен z в S1/S2
	Ceiling          []CeilingRow `json:"ceiling"`
	CeilingThreshold float64      `json:"ceiling_threshold"`
	FlatShare        float64      `json:"flat_share"` // доля анкет, где все шкалы одинаковые
}

// ThemeKappa — согласие словаря и модели по теме на текущих данных.
type ThemeKappa struct {
	Theme       string   `json:"theme"`
	Label       string   `json:"label"`
	Kappa       *float64 `json:"kappa"` // null — тема не встретилась ни у одного анализатора (κ не определена)
	Both        int      `json:"both"`
	OnlyOffline int      `json:"only_offline"`
	OnlyLLM     int      `json:"only_llm"`
	N           int      `json:"n"`
}

// TextAIQuality — ИИ-разбор комментариев.
type TextAIQuality struct {
	Analyzer   string         `json:"analyzer"`
	Model      string         `json:"model"`
	PromptHash string         `json:"prompt_hash"`
	Total      int            `json:"total"`    // разобранных комментариев
	Rejected   int            `json:"rejected"` // ответ модели отклонён или модель недоступна
	Reasons    map[string]int `json:"reasons"`
	Agreement  []ThemeKappa   `json:"agreement"` // null — ответов модели нет
}

// ActionEffect — что происходит с индексом после действия HR. Без контрольной группы — наблюдение.
type ActionEffect struct {
	WithAction  int      `json:"with_action"`
	Measured    int      `json:"measured"`
	MedianDelta *float64 `json:"median_delta"`
	Note        string   `json:"note"`
}

// ActionRow — решение HR и индекс роли сигнала до и после него.
type ActionRow struct {
	Signal string   `json:"signal"`
	Pair   string   `json:"pair"`
	Type   string   `json:"type"`
	Title  string   `json:"title"`
	Status string   `json:"status"`
	Action string   `json:"action"`
	Period int      `json:"period"` // месяц решения
	Role   string   `json:"role"`   // чей индекс смотрим: intern | mentor (для сигналов обеих сторон — стажёра)
	Before *float64 `json:"before"` // индекс роли в месяце решения
	After  *float64 `json:"after"`  // последний месяц после решения; null — данных ещё нет
	Delta  *float64 `json:"delta"`
	Months int      `json:"months"` // сколько месяцев между решением и after
}

// ActionNote — обязательная оговорка к эффекту действий.
const ActionNote = "Без контрольной группы это наблюдение, а не доказательство: индекс мог измениться и без действия (в том числе из-за регрессии к среднему)."

// buildActions — строки «действие → эффект» по вердиктам с месяцем решения (старые вердикты без period пропускаются).
func buildActions(pairs []PairReport, views map[string]*PairView) []ActionRow {
	out := []ActionRow{}
	for _, p := range pairs {
		v := views[p.PairID]
		if v == nil {
			continue
		}
		for _, list := range [][]SignalView{p.Signals, p.Resolved} {
			for _, s := range list {
				if s.Verdict == nil || s.Verdict.Period <= 0 {
					continue
				}
				role := Side(s.Signal)
				series := v.InternIndex
				if role == "mentor" {
					series = v.MentorIndex
				} else {
					role = "intern"
				}
				row := ActionRow{Signal: s.ID, Pair: p.PairID, Type: s.Type, Title: s.Title, Status: s.Verdict.Status,
					Action: s.Verdict.Action, Period: s.Verdict.Period, Role: role}
				if i := s.Verdict.Period - 1; i < len(series) && !math.IsNaN(series[i]) {
					b := r2(series[i])
					row.Before = &b
				}
				if j := lastIndex(series); j >= s.Verdict.Period {
					a := r2(series[j])
					row.After, row.Months = &a, j+1-s.Verdict.Period
					if row.Before != nil {
						d := r2(a - *row.Before)
						row.Delta = &d
					}
				}
				out = append(out, row)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Period != out[j].Period {
			return out[i].Period < out[j].Period
		}
		return out[i].Signal < out[j].Signal
	})
	return out
}

func actionEffect(rows []ActionRow) ActionEffect {
	e := ActionEffect{Note: ActionNote}
	var deltas []float64
	for _, r := range rows {
		if strings.TrimSpace(r.Action) == "" {
			continue
		}
		e.WithAction++
		if r.Delta != nil {
			e.Measured++
			deltas = append(deltas, *r.Delta)
		}
	}
	e.MedianDelta = medianPtr(deltas)
	return e
}

// flapRuns — сколько раз сигнал появлялся (число отрезков присутствия по месяцам бэктеста).
func flapRuns(present []bool) int {
	runs := 0
	for i, p := range present {
		if p && (i == 0 || !present[i-1]) {
			runs++
		}
	}
	return runs
}

func scales(in Input, views map[string]*PairView, sigma float64) Scales {
	sc := Scales{NoiseSigma: r2(sigma), Ceiling: []CeilingRow{}, CeilingThreshold: CeilingThreshold}
	for _, q := range in.Survey.Questions {
		if q.Kind != "scale" {
			continue
		}
		row := CeilingRow{Question: q.Code, Text: q.Text, Role: string(q.Role)}
		top := 0
		for _, v := range views {
			for _, x := range v.Questions[q.Code] {
				if math.IsNaN(x) {
					continue
				}
				row.N++
				if x >= 5-1e-9 {
					top++
				}
			}
		}
		if row.N > 0 {
			row.Share = r2(float64(top) / float64(row.N))
		}
		sc.Ceiling = append(sc.Ceiling, row)
	}
	flat, total := 0, 0
	for _, v := range views {
		for _, role := range []string{"intern", "mentor"} {
			idx := v.InternIndex
			if role == "mentor" {
				idx = v.MentorIndex
			}
			for i, x := range idx {
				if !math.IsNaN(x) {
					total++
					if v.Flat[role][i] {
						flat++
					}
				}
			}
		}
	}
	if total > 0 {
		sc.FlatShare = r2(float64(flat) / float64(total))
	}
	return sc
}

// textAI — отказы модели по причинам и κ словаря и модели по темам (только по комментариям, где ответ модели принят).
func textAI(in Input) TextAIQuality {
	q := TextAIQuality{Analyzer: "offline", PromptHash: textai.PromptHash(), Reasons: map[string]int{}}
	type pairT struct{ off, llm map[string]bool }
	var both []pairT
	for _, r := range in.Responses {
		for code, text := range r.Comments {
			a, ok := in.Analyses[r.ID+"/"+code]
			if !ok {
				continue
			}
			q.Total++
			if why := textai.Reason(a.Note); why != "" {
				q.Rejected++
				q.Reasons[why]++
			}
			if !strings.HasPrefix(a.Analyzer, "llm") {
				continue
			}
			if q.Model == "" {
				q.Analyzer, q.Model = "llm", a.Model
			}
			off := textai.Offline{}.Analyze(context.Background(), text, string(r.Role))
			t := pairT{off: set(off.Themes), llm: set(a.Themes)}
			if a.Note == textai.NoteMerged {
				// объединённые темы — не мнение модели: для κ берём только темы, которые назвала модель
				t.llm = set(a.LLMThemes)
			}
			both = append(both, t)
		}
	}
	if len(both) == 0 {
		return q
	}
	for _, th := range textai.Themes {
		k := ThemeKappa{Theme: th.Code, Label: th.Label, N: len(both)}
		for _, t := range both {
			switch o, l := t.off[th.Code], t.llm[th.Code]; {
			case o && l:
				k.Both++
			case o:
				k.OnlyOffline++
			case l:
				k.OnlyLLM++
			}
		}
		k.Kappa = kappa(k.Both, k.OnlyOffline, k.OnlyLLM, k.N)
		q.Agreement = append(q.Agreement, k)
	}
	return q
}

func set(xs []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// kappa — κ Коэна для двух бинарных разметок: (p_o − p_e)/(1 − p_e); nil, если p_e = 1 (тема у всех одна и та же).
func kappa(both, onlyA, onlyB, n int) *float64 {
	if n == 0 {
		return nil
	}
	neither := n - both - onlyA - onlyB
	fn := float64(n)
	po := float64(both+neither) / fn
	pa, pb := float64(both+onlyA)/fn, float64(both+onlyB)/fn
	pe := pa*pb + (1-pa)*(1-pb)
	if pe >= 1-1e-12 {
		return nil
	}
	k := r2((po - pe) / (1 - pe))
	return &k
}

// Kappa — κ Коэна для бинарных разметок (экспорт для инструментов оценки).
func Kappa(both, onlyA, onlyB, n int) *float64 { return kappa(both, onlyA, onlyB, n) }

func buildQuality(in Input, rep *ProgramReport, views map[string]*PairView, sigma float64, flapping []string) {
	n := len(in.Survey.Participant)
	q := Quality{Scales: scales(in, views, sigma), TextAI: textAI(in), ActionEffect: actionEffect(rep.Actions)}
	q.Caveat = fmt.Sprintf("Метрики на %d парах; интервалы широкие. Сигнал — повод для разговора, решение принимает HR.", n)
	q.ICCNote = fmt.Sprintf("ICC не считается: для интерпретации нужно не меньше 30 пар (Koo & Li 2016), сейчас %d; "+
		"стажёр и наставник отвечают на разные вопросы, поэтому согласие показано смещением и пределами Бланда–Альтмана.", n)
	if flapping == nil {
		flapping = []string{}
	}
	q.AlertLoad.Flapping = flapping
	for _, p := range rep.Pairs {
		for _, s := range p.Signals {
			if isOpen(s) {
				q.AlertLoad.ActiveCards++
				break
			}
		}
	}
	if n > 0 {
		q.AlertLoad.Per10Pairs = r2(float64(q.AlertLoad.ActiveCards) * 10 / float64(n))
		q.AlertLoad.AttentionShare = r2(float64(rep.Attention+rep.Watch) / float64(n))
	}
	var diffs [][]float64
	ids := make([]string, 0, len(views))
	for id := range views {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		diffs = append(diffs, views[id].Diff)
	}
	q.Agreement = ProgramAgreement(diffs)
	if in.Onset != nil {
		q.GroundTruth = TruthSynthetic
		if ev := evaluate(*rep, in.Onset, DefaultWindow, false); ev.Episodes > 0 {
			q.Evaluation = &ev
		}
	} else {
		q.GroundTruth = TruthHR
		if ev := evaluate(*rep, hrEpisodes(rep.Pairs), DefaultWindow, true); ev.Episodes > 0 {
			q.Evaluation = &ev
		}
	}
	rep.Quality = q
}

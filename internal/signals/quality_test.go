package signals

import (
	"math"
	"sort"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/synth"
	"github.com/ElzaraA/mgts-ai/internal/textai"
)

func synthWithOnset() Input {
	in := synthInput()
	in.Onset = synth.Onset
	return in
}

// На синтетике истина известна: все 5 эпизодов найдены не позже t0, P1 — без тревог.
func TestEvaluateSynthetic(t *testing.T) {
	rep := Analyze(synthWithOnset(), nil)
	q := rep.Quality
	if q.GroundTruth != TruthSynthetic || q.Evaluation == nil {
		t.Fatalf("%+v", q)
	}
	ev := *q.Evaluation
	if ev.Episodes != 5 || ev.Found != 5 || *ev.Recall.Lo != 0.478 || len(ev.FalsePairs) != 0 || ev.Window != 2 {
		t.Fatalf("%+v", ev)
	}
	if ev.MedianDelay == nil || *ev.MedianDelay != 0 || ev.Alerts == 0 || ev.Alerts != ev.TrueAlerts {
		t.Fatalf("delay/alerts %+v", ev)
	}
	// «средняя < 3» не видит P2 в окне; P5 — в тот же месяц
	if ev.Naive.Found != 4 || ev.OnlyOurs != 1 || ev.Same != 1 || ev.Earlier != 3 || ev.Later+ev.OnlyNaive+ev.Neither != 0 {
		t.Fatalf("%+v", ev)
	}
	if Evaluate(rep, synth.Onset, 0).Window != DefaultWindow {
		t.Fatal("default window")
	}
}

// Ложные тревоги: у пары без эпизодов и тревога раньше t0 − 1; неизвестные пары пропускаются.
func TestEvaluateFalseAlarms(t *testing.T) {
	sv := func(id, typ string, first int) SignalView {
		return SignalView{Signal: Signal{ID: id, Type: typ}, FirstSeen: first}
	}
	rep := ProgramReport{Pairs: []PairReport{
		{PairID: "A", Signals: []SignalView{sv("A-S1", "S1", 2), sv("A-S11-intern", "S11", 1)}},
		{PairID: "B", Signals: []SignalView{sv("B-S3", "S3", 1)}, Resolved: []SignalView{sv("B-S1", "S1", 6)}, Detection: Detection{Naive: 2}},
		{PairID: "C", Signals: []SignalView{sv("C-S1", "S1", 5)}, Detection: Detection{Naive: 4}},
		{PairID: "D", Signals: []SignalView{sv("D-S1", "S1", 3)}, Detection: Detection{Naive: 3}},
		{PairID: "E", Detection: Detection{Naive: 3}},
		{PairID: "X", Signals: []SignalView{sv("X-S1", "S1", 1)}},
	}}
	onset := map[string][]Episode{"A": {}, "B": {{Role: "intern", T0: 3}}, "C": {{Role: "intern", T0: 2}}, "D": {{T0: 3}}, "E": {{T0: 3}}}
	ev := Evaluate(rep, onset, 2)
	// A-S1 ложная (нет эпизодов), B-S3 ложная (M1 < t0 − 1 = 2), B-S1 поздняя, но не ложная; S11 не считается
	if ev.Alerts != 5 || ev.TrueAlerts != 3 || len(ev.FalsePairs) != 1 || ev.FalsePairs[0] != "A" {
		t.Fatalf("%+v", ev)
	}
	out := map[string]string{}
	for _, r := range ev.Rows {
		out[r.Pair] = r.Outcome
	}
	// B: наша тревога вне окна, правило — раньше t0 (вне окна): никто; C: только правило; D: одновременно; E: только правило
	if out["B"] != "neither" || out["C"] != "only_naive" || out["D"] != "same" || out["E"] != "only_naive" {
		t.Fatalf("%v", out)
	}
	rep.Pairs[3].Signals[0].FirstSeen = 4
	if ev := Evaluate(rep, onset, 2); ev.Later != 1 {
		t.Fatalf("later: %+v", ev)
	}
}

// Без синтетики истина — подтверждённые HR сигналы; без решений оценки нет (null).
func TestEvaluateHRVerdicts(t *testing.T) {
	in := synthInput()
	if q := Analyze(in, nil).Quality; q.GroundTruth != TruthHR || q.Evaluation != nil {
		t.Fatalf("%+v", q.Evaluation)
	}
	v := map[string]Verdict{"P2-S7": {Status: "confirmed", Period: 6}, "P4-S3": {Status: "false", Period: 6}, "P5-S11-intern": {Status: "confirmed"}}
	q := Analyze(in, v).Quality
	ev := q.Evaluation
	if ev == nil || ev.Episodes != 1 || ev.Found != 1 || ev.Alerts != 2 || ev.TrueAlerts != 1 || ev.FalsePairs[0] != "P4" {
		t.Fatalf("%+v", ev)
	}
}

func TestAgreement(t *testing.T) {
	if PairAgreement([]float64{1, NaN}) != nil || ProgramAgreement([][]float64{{NaN}}) != nil {
		t.Fatal("need ≥ 2 observations")
	}
	a := PairAgreement([]float64{1, 2, 3, NaN})
	if a.Bias != 2 || a.SD != 1 || a.LoALo != 0.04 || a.LoAHi != 3.96 || a.N != 3 || a.Method != MethodSimple {
		t.Fatalf("%+v", a)
	}
	if p := ProgramAgreement([][]float64{{1, 2}}); p.Method != MethodSimple {
		t.Fatal("one pair → simple")
	}
	// две пары с противоположным смещением: межпарная компонента раздувает SD
	p := ProgramAgreement([][]float64{{2, 2.2, 1.8}, {-2, -2.2, -1.8}})
	if p.Method != MethodRepeated || p.Bias != 0 || p.SD < 2 || p.N != 6 {
		t.Fatalf("%+v", p)
	}
	// на синтетике: P4 — стажёр выше наставника (контракт: ≈ +1.75 по i_overall − m_overall; по индексам 1.69)
	rep := Analyze(synthInput(), nil)
	if a := pairOf(rep, "P4").Agreement; a == nil || math.Abs(a.Bias-1.75) > 0.1 || len(pairOf(rep, "P4").Diff) != 6 {
		t.Fatalf("%+v", a)
	}
	if a := pairOf(rep, "P2").Agreement; a.Bias >= 0 {
		t.Fatalf("P2: mentor rates higher: %+v", a)
	}
	if q := rep.Quality.Agreement; q == nil || q.Method != MethodRepeated || q.N != 29 {
		t.Fatalf("%+v", q)
	}
}

// Петля «действие → эффект»: индекс роли сигнала в месяце решения и в последнем месяце после него.
func TestActions(t *testing.T) {
	in := synthInput()
	v := map[string]Verdict{
		"P5-S1": {Status: "resolved", Action: "встреча 1:1 проведена", Period: 3},
		"P2-S2": {Status: "confirmed", Period: 6},
		"P4-S8": {Status: "confirmed", Action: "поговорили с наставником", Period: 6},
		"P3-S3": {Status: "confirmed"}, // вердикт до R4: без месяца — не строка эффекта
	}
	rep := Analyze(in, v)
	rows := map[string]ActionRow{}
	for _, r := range rep.Actions {
		rows[r.Signal] = r
	}
	r := rows["P5-S1"]
	if r.Role != "intern" || *r.Before != 2.75 || *r.After != 4 || *r.Delta != 1.25 || r.Months != 3 || r.Action == "" {
		t.Fatalf("%+v", r)
	}
	if r := rows["P4-S8"]; r.Role != "mentor" || r.After != nil || r.Delta != nil || *r.Before != 2.5 {
		t.Fatalf("no data after the decision yet: %+v", r)
	}
	if _, ok := rows["P3-S3"]; ok || len(pairOf(rep, "P5").Actions) != 1 {
		t.Fatal("old verdicts are skipped; pair rows are present")
	}
	e := rep.Quality.ActionEffect
	if e.WithAction != 2 || e.Measured != 1 || *e.MedianDelta != 1.25 || e.Note == "" {
		t.Fatalf("%+v", e)
	}
	if buildActions([]PairReport{{PairID: "nope"}}, nil); len(buildActions(nil, nil)) != 0 {
		t.Fatal("empty")
	}
}

func TestQualityLoadAndScales(t *testing.T) {
	rep := Analyze(synthInput(), nil)
	q := rep.Quality
	if q.AlertLoad.ActiveCards != 4 || q.AlertLoad.Per10Pairs != 8 || q.AlertLoad.AttentionShare != 0.6 || q.AlertLoad.Flapping == nil {
		t.Fatalf("%+v", q.AlertLoad)
	}
	if q.Scales.NoiseSigma < 0.25 || q.Scales.CeilingThreshold != 0.15 || q.Scales.FlatShare <= 0 {
		t.Fatalf("%+v", q.Scales)
	}
	for _, c := range q.Scales.Ceiling {
		if c.Question == "i_overall" && (c.Share != 0.34 || c.N != 29) {
			t.Fatalf("methodology §4.1: 34%% fives in i_overall, got %+v", c)
		}
	}
	if q.Caveat == "" || q.ICCNote == "" || q.Goldset != nil || q.TextAI.Total != 59 || q.TextAI.Agreement != nil || q.TextAI.PromptHash == "" {
		t.Fatalf("%+v", q)
	}
	empty := Analyze(Input{Survey: synthInput().Survey, T: Default}, nil)
	if empty.Quality.Caveat == "" || empty.Actions == nil || empty.Quality.Agreement != nil {
		t.Fatalf("%+v", empty.Quality)
	}
}

func TestFlapRuns(t *testing.T) {
	// ряд 4,3,4,3: сигнал появляется в M2 и M4 — «мигает»
	if flapRuns([]bool{false, true, false, true}) != 2 || flapRuns([]bool{true, true, false}) != 1 || flapRuns(nil) != 0 {
		t.Fatal("runs")
	}
}

// κ словаря и модели считается только по комментариям, где ответ модели принят; отказы — по причинам.
func TestTextAIQuality(t *testing.T) {
	in := synthInput()
	keys := make([]string, 0, len(in.Analyses))
	for k := range in.Analyses {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for n, k := range keys {
		a := in.Analyses[k]
		switch n % 4 {
		case 0:
			a.Analyzer, a.Model = "llm:m", "m"
		case 1:
			a.Note = textai.NoteRejectedPrefix + "не JSON"
		case 2:
			a.Analyzer, a.Model, a.Note, a.LLMThemes = "llm:m", "m", textai.NoteMerged, nil
		}
		in.Analyses[k] = a
	}
	q := textAI(in)
	if q.Analyzer != "llm" || q.Model != "m" || q.Rejected == 0 || q.Reasons["не JSON"] != q.Rejected || q.Agreement == nil {
		t.Fatalf("%+v", q)
	}
	for _, k := range q.Agreement {
		if k.Theme == "routine" && (k.Kappa == nil || k.Both+k.OnlyOffline == 0 || k.N == 0) {
			t.Fatalf("%+v", k)
		}
	}
	if kappa(0, 0, 0, 0) != nil || kappa(0, 0, 0, 5) != nil || *Kappa(2, 0, 0, 4) != 1 {
		t.Fatal("kappa")
	}
}

// У цитаты в основании сигнала указан анализатор: словарь или модель; критичная тема, добавленная словарём, — словарь.
func TestEvidenceAnalyzer(t *testing.T) {
	in := synthInput()
	for k, a := range in.Analyses {
		if hasAny(a.Themes, "attrition", "conflict") {
			a.Analyzer, a.LLMThemes, a.Note = "llm:m", []string{}, textai.NoteMerged
		} else if len(a.Themes) > 0 {
			a.Analyzer = "llm:m"
			a.ByTheme = nil
		}
		in.Analyses[k] = a
	}
	seen := map[string]bool{}
	for _, p := range Analyze(in, nil).Pairs {
		for _, s := range append(p.Signals, p.Resolved...) {
			for _, e := range s.Evidence {
				if e.Quote != "" {
					seen[e.Analyzer] = true
					if s.Type == "S10" && e.Analyzer != "offline" {
						t.Fatalf("%s: %+v", s.ID, e)
					}
				} else if e.Analyzer != "" {
					t.Fatalf("no quote — no analyzer: %+v", e)
				}
			}
		}
	}
	if !seen["offline"] || !seen["llm:m"] {
		t.Fatalf("%v", seen)
	}
}

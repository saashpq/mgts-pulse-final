package signals

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Verdict — решение HR по сигналу. Решает человек, ИИ только подсказывает.
type Verdict struct {
	FirstAt string `json:"first_at,omitempty"` // Первая регистрация решения в этой цепочке.
	Episode int    `json:"episode,omitempty"`
	Status  string `json:"status"` // confirmed | false | resolved
	Comment string `json:"comment,omitempty"`
	At      string `json:"at"`
	// Action — что сделано («встреча 1:1 проведена», «сменили задачи»…), необязательно.
	Action string `json:"action,omitempty"`
	// Period — месяц решения: текущий месяц программы на момент вердикта (0 — вердикт старше R4).
	Period int `json:"period,omitempty"`
}

// SignalView — сигнал + когда впервые замечен + вердикт HR.
type SignalView struct {
	Episode int `json:"episode,omitempty"` // Начало текущего непрерывного эпизода в месяцах.
	Signal
	FirstSeen int      `json:"first_seen"` // месяц, когда сигнал впервые появился бы у HR
	Verdict   *Verdict `json:"verdict,omitempty"`
	// Primary — главный сигнал пары (в своём списке: активные или снятые); Related — ID остальных сигналов этого списка,
	// у главного; у неглавных — пустой массив. Так HR видит одну карточку на пару, а не 4 сигнала об одном и том же.
	Primary bool     `json:"primary"`
	Related []string `json:"related"`
	// SeenAt — когда текущий эпизод стал виден HR: время последнего ответа пары в начале эпизода (RFC3339).
	// Пусто — время поступления неизвестно.
	SeenAt string `json:"seen_at,omitempty"`
	// Overdue — активный сигнал без решения HR, у которого с SeenAt прошло больше due_days дней.
	Overdue bool `json:"overdue"`
}

type PairReport struct {
	PairID        string       `json:"pair"`
	Intern        string       `json:"intern"`
	Mentor        string       `json:"mentor"`
	Track         string       `json:"track,omitempty"`
	Status        string       `json:"status"` // attention | watch | ok
	Headline      string       `json:"headline"`
	Signals       []SignalView `json:"signals"`        // активные на текущий месяц
	Resolved      []SignalView `json:"resolved"`       // были раньше, сейчас не срабатывают
	History       []string     `json:"history"`        // максимальная критичность по месяцам: high|medium|low|""
	ProblemAlerts []bool       `json:"problem_alerts"` // Содержательная тревога по данным, доступным к концу месяца, до решений HR.
	InternIndex   []*float64   `json:"intern_index"`
	MentorIndex   []*float64   `json:"mentor_index"`
	// Detection — когда проблему заметила система и когда её заметило бы простое правило «средняя оценка < 3».
	Detection Detection `json:"detection"`
	Summary   string    `json:"summary"`
	// Diff — знаковая разница индексов «стажёр − наставник» по месяцам; Agreement — смещение и пределы согласия
	// (null — меньше двух месяцев с ответами обеих сторон).
	Diff      []*float64 `json:"diff"`
	Agreement *Agreement `json:"agreement"`
	// Actions — решения HR по сигналам пары и индекс до и после них.
	Actions []ActionRow `json:"actions"`
}

type ProgramReport struct {
	Reaction      Reaction     `json:"reaction"`
	LowScore      float64      `json:"low_score"` // порог, с которым рассчитано простое правило сравнения
	Current       int          `json:"current"`
	Periods       int          `json:"periods"`
	Pairs         []PairReport `json:"pairs"`
	Attention     int          `json:"attention"`
	Watch         int          `json:"watch"`
	InternAvg     []*float64   `json:"intern_avg"`
	MentorAvg     []*float64   `json:"mentor_avg"`
	FillRate      float64      `json:"fill_rate"` // доля заполненных анкет за текущий месяц
	SignalsNow    int          `json:"signals_now"`
	Confirmed     int          `json:"confirmed"`
	FalsePositive int          `json:"false_positive"`
	// EarlierOrOnly — пар, где система заметила проблему раньше простого правила или где правило её не видит вовсе.
	EarlierOrOnly int      `json:"earlier_or_only"`
	AvgLeadTime   *float64 `json:"avg_lead_time,omitempty"`
	Digest        string   `json:"digest"` // черновик ежемесячной сводки для руководства
	// Precision — точность каждого типа сигнала по вердиктам HR и подсказка, какой порог загрубить.
	Precision []Precision `json:"precision"`
	// PrecisionTotal — точность по всем типам вместе.
	PrecisionTotal PrecisionTotal `json:"precision_total"`
	// Effect — эффект пилота только из реальных полей; где данных нет — null.
	Effect Effect `json:"effect"`
	// Actions — петля «действие → эффект» по всем решениям HR с месяцем решения.
	Actions []ActionRow `json:"actions"`
	// Quality — экран «Качество системы».
	Quality Quality `json:"quality"`
}

// Effect — измеримый эффект: раньше ли система замечает проблему и успевает ли HR реагировать.
type Effect struct {
	// PairsFlaggedEarly — пар, где система заметила проблему раньше правила «средняя оценка < порога» или где правило её не видит.
	PairsFlaggedEarly int            `json:"pairs_flagged_early"`
	DetectionTable    []DetectionRow `json:"detection_table"`
	VerdictCoverage   *float64       `json:"verdict_coverage"` // доля активных сигналов с решением HR; null — активных нет
	Overdue           *int           `json:"overdue"`          // активных без решения, срок реакции истёк; null — нет дат
	OverdueUnknown    int            `json:"overdue_unknown"`  // активных без решения, у которых дата появления неизвестна
	ActiveSignals     int            `json:"active_signals"`
	DecidedSignals    int            `json:"decided_signals"`
}

// DetectionRow — месяц первой проблемы по паре: системой и простым правилом; null — не замечено.
type DetectionRow struct {
	Pair  string `json:"pair"`
	Ours  *int   `json:"ours"`
	Naive *int   `json:"naive"`
}

// typePriority — какой сигнал пары главный при равной критичности: от самых срочных и содержательных к служебным.
var typePriority = map[string]int{"S10": 13, "S7": 12, "S1": 11, "S3": 10, "S4": 9, "S8": 8, "S6": 7, "S2": 6, "S5": 5,
	"S14": 7, "S15": 6, "S9": 4, "S12": 3, "S13": 2, "S11": 1}

func isOpen(s SignalView) bool {
	return s.Verdict == nil || (s.Verdict.Status != "false" && s.Verdict.Status != "resolved")
}

// group сортирует сигналы (главный первым) и проставляет Primary/Related. Главный: сначала сигналы без вердикта
// «ложный»/«решён», затем наивысшая критичность, затем приоритет типа, затем ID.
func group(list []SignalView) {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if isOpen(a) != isOpen(b) {
			return isOpen(a)
		}
		if a.Severity.rank() != b.Severity.rank() {
			return a.Severity.rank() > b.Severity.rank()
		}
		if typePriority[a.Type] != typePriority[b.Type] {
			return typePriority[a.Type] > typePriority[b.Type]
		}
		return a.ID < b.ID
	})
	for i := range list {
		list[i].Primary = i == 0
		list[i].Related = []string{}
	}
	if len(list) > 0 {
		for _, s := range list[1:] {
			list[0].Related = append(list[0].Related, s.ID)
		}
	}
}

func intPtr(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

// Analyze — полный отчёт программы.
func Analyze(in Input, verdicts map[string]Verdict) ProgramReport {
	views := Build(in)
	current := in.WindowPeriod
	if current == 0 {
		for _, r := range in.Responses {
			if r.Period > current {
				current = r.Period
			}
		}
	}
	rep := ProgramReport{Current: current, Periods: in.Survey.Periods, LowScore: in.T.LowScore, Pairs: []PairReport{}}
	if current == 0 {
		rep.Digest = "Данных пока нет: загрузите выгрузку анкет или дождитесь первых ответов."
		rep.Precision = PrecisionByType(nil, in.T)
		rep.PrecisionTotal = totalPrecision(rep.Precision)
		rep.Effect.DetectionTable = []DetectionRow{}
		rep.Actions = []ActionRow{}
		views := Build(in)
		buildQuality(in, &rep, views, NoiseSigma(views), nil)
		return rep
	}
	var leads []float64
	// когда ответ пары за месяц поступил: самый поздний at среди ответов пары за этот месяц
	arrived := map[string]time.Time{}
	for _, r := range in.Responses {
		at, err := time.Parse(time.RFC3339, r.At)
		if err != nil {
			continue
		}
		k := fmt.Sprintf("%s/%d", r.PairID, r.Period)
		if at.After(arrived[k]) {
			arrived[k] = at
		}
	}
	seenAt := func(sv *SignalView, episodeStart int) {
		sv.Episode = episodeStart
		if at, ok := arrived[fmt.Sprintf("%s/%d", sv.Pair, episodeStart)]; ok {
			sv.SeenAt = at.UTC().Format(time.RFC3339)
		}
	}
	rep.Effect.DetectionTable = []DetectionRow{}
	overdue, dated := 0, false
	var flapping []string
	// σ̂ шума на каждый месяц бэктеста — только по данным, которые были известны в тот месяц
	sigmas := make([]float64, current+1)
	for cut := 1; cut <= current; cut++ {
		cv := make(map[string]*PairView, len(views))
		for id, v := range views {
			cv[id] = v.Cut(cut)
		}
		sigmas[cut] = NoiseSigma(cv)
	}
	inAt := func(cut int) Input {
		x := in
		x.Sigma = sigmas[cut]
		return x
	}
	for _, p := range in.Survey.Participant {
		v := views[p.ID]
		pr := PairReport{PairID: p.ID, Intern: p.Intern, Mentor: p.Mentor, Track: p.Track, Status: "ok",
			History: make([]string, in.Survey.Periods), ProblemAlerts: make([]bool, current), InternIndex: ptrs(v.InternIndex), MentorIndex: ptrs(v.MentorIndex),
			Signals: []SignalView{}, Resolved: []SignalView{}}
		firstSeen := map[string]int{}
		episodeStart := map[string]int{}
		lastSeen := map[string]SignalView{}
		present := map[string][]bool{}
		verdictFor := func(id string) *Verdict {
			vd, ok := verdicts[id]
			// Старое решение без даты совместимо с первым эпизодом. Переносить
			// его через известный перерыв оснований нет.
			if !ok || vd.Period > current || (vd.Period == 0 && episodeStart[id] != firstSeen[id]) ||
				(vd.Period > 0 && vd.Period < episodeStart[id]) {
				return nil
			}
			return &vd
		}
		for cut := 1; cut <= current; cut++ {
			for _, s := range Detect(v, inAt(cut), cut) {
				if s.LastPeriod == cut && !serviceTypes[s.Type] && (s.Severity == High || s.Severity == Medium) {
					pr.ProblemAlerts[cut-1] = true
				}
				if present[s.ID] == nil {
					present[s.ID] = make([]bool, current)
				}
				present[s.ID][cut-1] = true
				if firstSeen[s.ID] == 0 {
					firstSeen[s.ID] = cut
				}
				previous := lastSeen[s.ID]
				if cut == 1 || !present[s.ID][cut-2] ||
					(s.Type == "S11" && previous.FirstPeriod != s.FirstPeriod) {
					episodeStart[s.ID] = cut
				}
				lastSeen[s.ID] = SignalView{Signal: s}
				if s.LastPeriod == cut && sevRank(s.Severity) > sevRank(Severity(pr.History[cut-1])) {
					if vd := verdictFor(s.ID); vd == nil || vd.Status != "false" || vd.Period > cut {
						pr.History[cut-1] = string(s.Severity)
					}
				}
			}
		}
		active := map[string]bool{}
		for _, s := range Detect(v, inAt(current), current) {
			active[s.ID] = true
			sv := SignalView{Signal: s, FirstSeen: firstSeen[s.ID], Verdict: verdictFor(s.ID)}
			seenAt(&sv, episodeStart[s.ID])
			rep.Effect.ActiveSignals++
			if sv.Verdict != nil {
				rep.Effect.DecidedSignals++
			} else if sv.SeenAt == "" || in.Now.IsZero() {
				rep.Effect.OverdueUnknown++
			} else {
				dated = true
				at, _ := time.Parse(time.RFC3339, sv.SeenAt)
				if in.Now.Sub(at) > time.Duration(sv.DueDays)*24*time.Hour {
					sv.Overdue = true
					overdue++
				}
			}
			pr.Signals = append(pr.Signals, sv)
		}
		for id, sv := range lastSeen {
			if !active[id] {
				sv.FirstSeen = firstSeen[id]
				sv.Verdict = verdictFor(id)
				seenAt(&sv, episodeStart[id])
				pr.Resolved = append(pr.Resolved, sv)
			}
		}
		for id, pres := range present {
			if flapRuns(pres) >= 2 {
				flapping = append(flapping, id)
			}
		}
		group(pr.Signals)
		group(pr.Resolved)
		pr.Diff, pr.Agreement = ptrs(v.Diff), PairAgreement(v.Diff)
		pr.Actions = buildActions([]PairReport{pr}, views)
		for _, s := range pr.Signals {
			if s.Verdict != nil && (s.Verdict.Status == "false" || s.Verdict.Status == "resolved") {
				continue
			}
			if s.Severity == High {
				pr.Status = "attention"
			} else if s.Severity == Medium && pr.Status == "ok" {
				pr.Status = "watch"
			}
		}
		pr.Detection = detect(v, pr.History, in.T)
		rep.Effect.DetectionTable = append(rep.Effect.DetectionTable, DetectionRow{Pair: p.ID, Ours: intPtr(pr.Detection.Ours), Naive: intPtr(pr.Detection.Naive)})
		if pr.Detection.Ours > 0 && (pr.Detection.Naive == 0 || pr.Detection.Ours < pr.Detection.Naive) {
			rep.EarlierOrOnly++
		}
		if pr.Detection.Ours > 0 && pr.Detection.Naive > 0 {
			leads = append(leads, float64(pr.Detection.Naive-pr.Detection.Ours))
		}
		pr.Headline, pr.Summary = summarize(pr, in)
		switch pr.Status {
		case "attention":
			rep.Attention++
		case "watch":
			rep.Watch++
		}
		for _, s := range pr.Signals {
			rep.SignalsNow++
			if s.Verdict != nil {
				switch s.Verdict.Status {
				case "confirmed", "resolved":
					rep.Confirmed++
				case "false":
					rep.FalsePositive++
				}
			}
		}
		rep.Pairs = append(rep.Pairs, pr)
	}
	sort.SliceStable(rep.Pairs, func(i, j int) bool {
		return statusRank(rep.Pairs[i].Status) > statusRank(rep.Pairs[j].Status)
	})
	n := in.Survey.Periods
	ia, ma := make([]float64, n), make([]float64, n)
	for i := 0; i < n; i++ {
		var a, b []float64
		for _, v := range views {
			a = append(a, v.InternIndex[i])
			b = append(b, v.MentorIndex[i])
		}
		ia[i], ma[i] = mean(a), mean(b)
	}
	rep.InternAvg, rep.MentorAvg = ptrs(ia), ptrs(ma)
	filled := 0
	for _, r := range in.Responses {
		if r.Period == current {
			filled++
		}
	}
	if len(in.Survey.Participant) > 0 {
		rep.FillRate = r2(float64(filled) / float64(2*len(in.Survey.Participant)))
	}
	if len(leads) > 0 {
		x := r2(mean(leads))
		rep.AvgLeadTime = &x
	}
	rep.Digest = digest(rep)
	rep.Reaction = reactionTime(rep.Pairs)
	rep.Precision = PrecisionByType(rep.Pairs, in.T)
	rep.PrecisionTotal = totalPrecision(rep.Precision)
	rep.Effect.PairsFlaggedEarly = rep.EarlierOrOnly
	if rep.Effect.ActiveSignals > 0 {
		x := r2(float64(rep.Effect.DecidedSignals) / float64(rep.Effect.ActiveSignals))
		rep.Effect.VerdictCoverage = &x
	}
	if dated {
		rep.Effect.Overdue = &overdue
	}
	rep.Actions = buildActions(rep.Pairs, views)
	sort.Strings(flapping)
	buildQuality(in, &rep, views, sigmas[current], flapping)
	return rep
}

// Detection — месяц первой проблемы (1-based, 0 — не замечено).
type Detection struct {
	Ours  int `json:"ours"`  // первый сигнал medium/high
	Naive int `json:"naive"` // первый месяц, когда средняя оценка стажёра или наставника < порога
}

func detect(v *PairView, history []string, t Thresholds) Detection {
	var d Detection
	for i := range v.InternIndex {
		if (!math.IsNaN(v.InternIndex[i]) && v.InternIndex[i] < t.LowScore) || (!math.IsNaN(v.MentorIndex[i]) && v.MentorIndex[i] < t.LowScore) {
			d.Naive = i + 1
			break
		}
	}
	for i, h := range history {
		if h == string(High) || h == string(Medium) {
			d.Ours = i + 1
			break
		}
	}
	return d
}

func summarize(pr PairReport, in Input) (string, string) {
	var open []SignalView
	for _, s := range pr.Signals {
		if s.Verdict == nil || (s.Verdict.Status != "false" && s.Verdict.Status != "resolved") {
			open = append(open, s)
		}
	}
	if len(open) == 0 {
		h := "Нет активных сигналов"
		if len(pr.Resolved) > 0 {
			h = "Прежние сигналы сейчас не срабатывают"
		}
		return h, fmt.Sprintf("Пара %s: активных сигналов нет. %s", pr.PairID, trendText(pr))
	}
	top := open[0]
	var b strings.Builder
	fmt.Fprintf(&b, "Пара %s (%s): %s. %s", pr.PairID, strings.ToLower(statusText(pr.Status)), strings.ToLower(top.Title), top.Explanation)
	for _, e := range top.Evidence {
		if e.Quote != "" {
			who := "стажёр"
			if e.Role == "mentor" {
				who = "наставник"
			}
			fmt.Fprintf(&b, ". Цитата (%s, M%d): «%s»", who, e.Period, e.Quote)
			break
		}
	}
	fmt.Fprintf(&b, ". Рекомендуемое действие: %s — в течение %d дн.", lowerFirst(top.Action), top.DueDays)
	if len(open) > 1 {
		fmt.Fprintf(&b, " Ещё сигналов: %d.", len(open)-1)
	}
	return top.Title, b.String()
}

func trendText(pr PairReport) string {
	last := -1
	for i, x := range pr.InternIndex {
		if x != nil {
			last = i
		}
	}
	if last < 0 {
		return ""
	}
	s := fmt.Sprintf("Индекс стажёра в M%d — %s", last+1, Num(*pr.InternIndex[last]))
	if last < len(pr.MentorIndex) && pr.MentorIndex[last] != nil {
		s += fmt.Sprintf(", оценка наставника — %s", Num(*pr.MentorIndex[last]))
	}
	return s + "."
}

func digest(rep ProgramReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Программа стажировок, месяц %d из %d. Пар: %d, требуют вмешательства: %d, под наблюдением: %d. ",
		rep.Current, rep.Periods, len(rep.Pairs), rep.Attention, rep.Watch)
	if c := rep.Current - 1; c >= 0 && rep.InternAvg[c] != nil {
		fmt.Fprintf(&b, "Средний индекс стажёров — %s", Num(*rep.InternAvg[c]))
		if c > 0 && rep.InternAvg[c-1] != nil {
			fmt.Fprintf(&b, " (%s к прошлому месяцу)", NumSigned(*rep.InternAvg[c]-*rep.InternAvg[c-1]))
		}
		b.WriteString(". ")
	}
	fmt.Fprintf(&b, "Заполненность анкет за месяц — %.0f%%. ", rep.FillRate*100)
	for _, p := range rep.Pairs {
		if p.Status == "attention" {
			fmt.Fprintf(&b, "%s: %s. ", p.PairID, strings.ToLower(p.Headline))
		}
	}
	if rep.EarlierOrOnly > 0 {
		fmt.Fprintf(&b, "В %d парах система заметила проблему раньше, чем средняя оценка упала ниже %s, или там, где средняя оценка её не показывает. ", rep.EarlierOrOnly, strings.ReplaceAll(fmt.Sprintf("%g", rep.LowScore), ".", ","))
	}
	b.WriteString("Выводы системы — подсказка для HR; решения принимает человек.")
	return b.String()
}

func statusText(s string) string {
	switch s {
	case "attention":
		return "Требует вмешательства"
	case "watch":
		return "Под наблюдением"
	}
	return "Без срочных сигналов"
}

func statusRank(s string) int {
	switch s {
	case "attention":
		return 2
	case "watch":
		return 1
	}
	return 0
}

func sevRank(s Severity) int { return s.rank() }

func ptrs(s []float64) []*float64 {
	out := make([]*float64, len(s))
	for i, x := range s {
		if !math.IsNaN(x) {
			v := r2(x)
			out[i] = &v
		}
	}
	return out
}

func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToLower(string(r[0])) + string(r[1:])
}

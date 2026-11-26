package signals

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ElzaraA/mgts-ai/internal/survey"
	"github.com/ElzaraA/mgts-ai/internal/textai"
)

type Severity string

const (
	Low    Severity = "low"
	Medium Severity = "medium"
	High   Severity = "high"
)

func (s Severity) rank() int {
	switch s {
	case High:
		return 3
	case Medium:
		return 2
	case Low:
		return 1
	}
	return 0
}

// Thresholds — пороги. Стартовые значения: при 5 парах × 6 месяцев их не обучить статистически,
// поэтому они задаются с экспертом и правятся HR (PUT /api/v1/thresholds). Обоснование S1/S2 и цены ошибок —
// docs/calibration.md (имитация: шум пункта 0.6, известный месяц начала проблемы).
type Thresholds struct {
	DeclineSlope    float64 `json:"decline_slope"`     // S1/S2: наклон Тейла–Сена по 4 месяцам, балл/мес
	ZDrop           float64 `json:"z_drop"`            // S1/S2: резкий провал — z ≤ −ZDrop к собственной базе
	DeclineFromBase float64 `json:"decline_from_base"` // S1/S2: устойчивое снижение — ниже собственной базы не меньше чем на
	CostRatio       float64 `json:"cost_ratio"`        // C_FN/C_FP: сколько лишних разговоров HR готов провести ради одного найденного
	LowScore        float64 `json:"low_score"`         // S1/S2: ниже — критично
	GapMin          float64 `json:"gap_min"`           // S3: расхождение на зеркальных вопросах
	GapHigh         float64 `json:"gap_high"`          // S3: критичное расхождение
	Persist         int     `json:"persist"`           // S3/S4/S8: сколько месяцев подряд
	BlindSelf       float64 `json:"blind_self"`        // S4: «я ≥»
	BlindOther      float64 `json:"blind_other"`       // S4: «он ≤»
	Diverge         float64 `json:"diverge"`           // S5: разница наклонов
	LowDimension    float64 `json:"low_dimension"`     // S6: поддержка/понятность ≤
	InterestDrop    float64 `json:"interest_drop"`     // S7: падение интереса от базы M1–M2
	NegSentiment    float64 `json:"neg_sentiment"`     // S7: тональность ≤
	MentorLow       float64 `json:"mentor_low"`        // S8: оценка наставника ≤
	ShortRatio      float64 `json:"short_ratio"`       // S12: длина комментария < доли от обычной
}

var Default = Thresholds{DeclineSlope: -0.25, ZDrop: 3.0, DeclineFromBase: 0.5, CostRatio: 10, LowScore: 3.0, GapMin: 1.0, GapHigh: 1.5,
	Persist: 2, BlindSelf: 4, BlindOther: 3, Diverge: 0.3, LowDimension: 2.5, InterestDrop: 1.0, NegSentiment: -0.3, MentorLow: 3.0,
	ShortRatio: 0.25}

// UnmarshalJSON — поля, которых нет в JSON, берутся из Default: старый state.json (с decline_from_peak и без
// z_drop/decline_from_base/cost_ratio) читается без потерь, лишние поля игнорируются.
func (t *Thresholds) UnmarshalJSON(b []byte) error {
	type plain Thresholds
	p := plain(Default)
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*t = Thresholds(p)
	return nil
}

// Evidence — ссылка на исходный ответ: число или дословная цитата.
type Evidence struct {
	ResponseID string  `json:"response_id"`
	Role       string  `json:"role"`
	Period     int     `json:"period"`
	Question   string  `json:"question,omitempty"`
	Text       string  `json:"text,omitempty"` // формулировка вопроса для показа HR
	Value      float64 `json:"value,omitempty"`
	Quote      string  `json:"quote,omitempty"`
	// Analyzer — кто нашёл цитату: offline (словарь) или llm:<модель>.
	Analyzer string `json:"analyzer,omitempty"`
}

// Signal — объяснимая находка: что, где, почему (числа и пороги), что делать и в какой срок.
type Signal struct {
	ID          string     `json:"id"` // пара + тип: стабилен между пересчётами, к нему привязан вердикт HR
	Pair        string     `json:"pair"`
	Type        string     `json:"type"`
	Title       string     `json:"title"`
	Severity    Severity   `json:"severity"`
	Source      string     `json:"source"` // rules | text | hybrid
	FirstPeriod int        `json:"first_period"`
	LastPeriod  int        `json:"last_period"`
	Explanation string     `json:"explanation"`
	Action      string     `json:"action"`
	DueDays     int        `json:"due_days"`
	Evidence    []Evidence `json:"evidence"`
}

// Catalog — тип сигнала: название, действие HR, срок реакции.
type Catalog struct {
	Title, Action string
	DueDays       int
}

var Types = map[string]Catalog{
	"S1":  {"Устойчивое снижение удовлетворённости стажёра", "Встреча HR 1:1 со стажёром: что изменилось и чем помочь", 3},
	"S2":  {"Снижение оценки стажёра наставником", "Разговор HR с наставником о целях и ожиданиях от стажёра", 5},
	"S3":  {"Расхождение восприятия в паре", "Совместная встреча стажёра, наставника и HR: сверка ожиданий", 5},
	"S4":  {"«Слепое пятно»: стороны видят ситуацию по-разному", "Наставник даёт стажёру конкретную обратную связь по критериям", 5},
	"S5":  {"Расходящиеся траектории", "Уточнить у наставника, замечает ли он изменения у стажёра", 5},
	"S6":  {"Не хватает поддержки или понятности задач", "Пересоберите задачи со стажёром и наставником, уточните ожидаемый результат и договоритесь о поддержке.", 5},
	"S7":  {"Потеря мотивации стажёра", "Пересмотреть задачи вместе с наставником: усложнить или дать ротацию", 5},
	"S8":  {"Наставник отмечает трудности в работе стажёра", "Обсудить с наставником план развития и конкретные ожидания", 5},
	"S9":  {"Повторяющаяся проблема в комментариях", "Уточните у автора, сохраняется ли проблема. Если да, передайте её руководителю программы и согласуйте ответственного за решение.", 7},
	"S10": {"Признаки конфликта или риска ухода", "Свяжитесь с автором комментария наедине, уточните обстоятельства и обсудите необходимую поддержку.", 2},
	"S11": {"Пропуск анкеты", "Напомнить участнику об анкете и уточнить причину", 2},
	"S12": {"Комментарии стали формальными", "Короткий чек-ин: всё ли в порядке", 7},
	"S13": {"Формальная анкета", "Уточнить, удобна ли анкета и есть ли что обсудить вне её", 7},
	"S14": {"Негативная обратная связь о второй стороне", "Проведите личную встречу HR 1:1 с автором комментария, уточните обстоятельства и нужную поддержку.", 3},
	"S15": {"Изменилась оценка задач", "Пересоберите задачи вместе со стажёром и наставником. Зафиксируйте, что сделает следующую задачу понятнее или интереснее.", 5},
}

// TypeOrder — типы сигналов в порядке каталога.
func TypeOrder() []string {
	out := make([]string, 0, len(Types))
	for k := range Types {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) < len(out[j])
		}
		return out[i] < out[j]
	})
	return out
}

// Input — всё, что нужно для расчёта.
type Input struct {
	Survey    survey.Survey
	Responses []survey.Response
	// Нулевой месяц означает историческую выгрузку без календаря приёма.
	WindowPeriod int
	WindowOpen   bool
	// Analyses — разбор комментариев: ключ response_id + "/" + код вопроса.
	Analyses map[string]textai.Analysis
	T        Thresholds
	// Now — «сейчас» для просрочки реакции HR (effect.overdue). Нулевое — просрочка не считается (null).
	Now time.Time
	// Sigma — σ̂ шума индексов программы (NoiseSigma). 0 — Detect оценит её по самой паре; Analyze считает её
	// по всей программе на каждый месяц бэктеста (только по данным, известным в тот месяц).
	Sigma float64
	// Onset — заложенные в синтетику эпизоды (истина для оценки системы). nil — истина берётся из вердиктов HR.
	Onset map[string][]Episode
}

// PairView — динамика пары: ряды по месяцам (индекс 0 = M1), NaN — нет ответа.
type PairView struct {
	Pair        survey.Pair           `json:"pair"`
	InternIndex []float64             `json:"intern_index"` // среднее шкал стажёра (1..5)
	MentorIndex []float64             `json:"mentor_index"`
	Questions   map[string][]float64  `json:"questions"`
	Gaps        map[string][]float64  `json:"gaps"` // код вопроса стажёра → |стажёр − наставник|
	Sentiment   map[string][]float64  `json:"sentiment"`
	Themes      map[string][][]string `json:"themes"`
	Words       map[string][]float64  `json:"-"`
	Flat        map[string][]bool     `json:"-"` // все шкалы ответа одинаковые
	AllMax      map[string][]bool     `json:"-"` // все шкалы ответа на максимуме (потолок)
	// Diff — знаковая разница индексов «стажёр − наставник» по месяцам (Bland–Altman); NaN — нет одной из сторон.
	Diff         []float64           `json:"diff"`
	ResponseIDs  map[string][]string `json:"-"`
	CommentCodes map[string][]string `json:"-"`
}

// Build строит ряды по всем парам.
func Build(in Input) map[string]*PairView {
	n := in.Survey.Periods
	nan := func() []float64 {
		s := make([]float64, n)
		for i := range s {
			s[i] = NaN
		}
		return s
	}
	views := map[string]*PairView{}
	for _, p := range in.Survey.Participant {
		v := &PairView{Pair: p, InternIndex: nan(), MentorIndex: nan(), Questions: map[string][]float64{},
			Gaps: map[string][]float64{}, Sentiment: map[string][]float64{"intern": nan(), "mentor": nan()},
			Themes: map[string][][]string{"intern": make([][]string, n), "mentor": make([][]string, n)},
			Words:  map[string][]float64{"intern": nan(), "mentor": nan()},
			Flat:   map[string][]bool{"intern": make([]bool, n), "mentor": make([]bool, n)},
			AllMax: map[string][]bool{"intern": make([]bool, n), "mentor": make([]bool, n)}, Diff: nan(),
			ResponseIDs: map[string][]string{"intern": make([]string, n), "mentor": make([]string, n)}, CommentCodes: map[string][]string{}}
		for _, q := range in.Survey.Questions {
			if q.Kind == "scale" {
				v.Questions[q.Code] = nan()
			} else {
				role := string(q.Role)
				v.CommentCodes[role] = append(v.CommentCodes[role], q.Code)
			}
		}
		views[p.ID] = v
	}
	for _, r := range in.Responses {
		v := views[r.PairID]
		if v == nil || r.Period < 1 || r.Period > n {
			continue
		}
		i, role := r.Period-1, string(r.Role)
		v.ResponseIDs[role][i] = r.ID
		var sum, first float64
		var cnt int
		flat := true
		for code, val := range r.Scores {
			q, ok := in.Survey.Q(code)
			if !ok || q.Kind != "scale" {
				continue
			}
			nv := q.Norm(val)
			v.Questions[code][i] = nv
			if cnt == 0 {
				first = nv
			} else if nv != first {
				flat = false
			}
			sum += nv
			cnt++
		}
		v.Flat[role][i] = flat && cnt >= 2
		v.AllMax[role][i] = v.Flat[role][i] && first == 5
		if cnt > 0 {
			if r.Role == survey.Intern {
				v.InternIndex[i] = sum / float64(cnt)
			} else {
				v.MentorIndex[i] = sum / float64(cnt)
			}
		}
		var sent []float64
		var words float64
		themes := []string{}
		for code, text := range r.Comments {
			words += float64(textai.Words(text))
			if a, ok := in.Analyses[r.ID+"/"+code]; ok {
				sent = append(sent, a.Sentiment)
				themes = append(themes, a.Themes...)
			}
		}
		if len(r.Comments) > 0 {
			v.Words[role][i] = words
		}
		if len(sent) > 0 {
			v.Sentiment[role][i] = mean(sent)
		}
		v.Themes[role][i] = themes
	}
	for _, v := range views {
		for i := range v.Diff {
			if !math.IsNaN(v.InternIndex[i]) && !math.IsNaN(v.MentorIndex[i]) {
				v.Diff[i] = v.InternIndex[i] - v.MentorIndex[i]
			}
		}
		for _, q := range in.Survey.ByRole(survey.Intern) {
			if q.Mirror == "" || q.Kind != "scale" {
				continue
			}
			a, b := v.Questions[q.Code], v.Questions[q.Mirror]
			g := nan()
			for i := range g {
				if !math.IsNaN(a[i]) && !math.IsNaN(b[i]) {
					g[i] = math.Abs(a[i] - b[i])
				}
			}
			v.Gaps[q.Code] = g
		}
	}
	return views
}

// Cut — ряды пары, обрезанные до месяца cutoff (то, что HR видел бы в том месяце).
func (v *PairView) Cut(cutoff int) *PairView {
	cut := func(s []float64) []float64 {
		out := make([]float64, len(s))
		for i := range s {
			if i < cutoff {
				out[i] = s[i]
			} else {
				out[i] = NaN
			}
		}
		return out
	}
	c := &PairView{Pair: v.Pair, InternIndex: cut(v.InternIndex), MentorIndex: cut(v.MentorIndex), Questions: map[string][]float64{},
		Gaps: map[string][]float64{}, Sentiment: map[string][]float64{}, Themes: map[string][][]string{}, Words: map[string][]float64{},
		Flat: map[string][]bool{}, AllMax: map[string][]bool{}, Diff: cut(v.Diff), ResponseIDs: v.ResponseIDs, CommentCodes: v.CommentCodes}
	for k, f := range v.Flat {
		out := make([]bool, len(f))
		copy(out, f[:min(cutoff, len(f))])
		c.Flat[k] = out
	}
	for k, f := range v.AllMax {
		out := make([]bool, len(f))
		copy(out, f[:min(cutoff, len(f))])
		c.AllMax[k] = out
	}
	for k, s := range v.Questions {
		c.Questions[k] = cut(s)
	}
	for k, s := range v.Gaps {
		c.Gaps[k] = cut(s)
	}
	for k, s := range v.Sentiment {
		c.Sentiment[k] = cut(s)
	}
	for k, s := range v.Words {
		c.Words[k] = cut(s)
	}
	for k, s := range v.Themes {
		t := make([][]string, len(s))
		copy(t, s[:min(cutoff, len(s))])
		c.Themes[k] = t
	}
	return c
}

// Detect — сигналы пары на месяц cutoff (1-based), current — последний месяц, по которому есть данные программы.
func Detect(v *PairView, in Input, cutoff int) []Signal {
	if cutoff < 1 || cutoff > len(v.InternIndex) {
		return nil
	}
	t := in.T
	c := v.Cut(cutoff)
	var out []Signal
	add := func(typ string, sev Severity, src string, first, last int, expl string, ev []Evidence) {
		cat := Types[typ]
		out = append(out, Signal{ID: v.Pair.ID + "-" + typ, Pair: v.Pair.ID, Type: typ, Title: cat.Title, Severity: sev, Source: src,
			FirstPeriod: first, LastPeriod: last, Explanation: expl, Action: cat.Action, DueDays: cat.DueDays, Evidence: ev})
	}
	ref := func(role string, i int, q string, val float64) Evidence {
		e := Evidence{ResponseID: c.ResponseIDs[role][i], Role: role, Period: i + 1, Question: q, Value: r2(val)}
		if qq, ok := in.Survey.Q(q); ok {
			e.Text = qq.Text
		} else if q == "" {
			e.Text = "среднее по вопросам с оценкой"
		}
		return e
	}
	// quote — цитата из комментария; если указаны темы — предложение именно про эту тему.
	quote := func(role string, i int, themes ...string) Evidence {
		e := Evidence{ResponseID: c.ResponseIDs[role][i], Role: role, Period: i + 1}
		fallback := e
		lowestSentiment := math.Inf(1)
		for _, code := range c.CommentCodes[role] {
			a, ok := in.Analyses[e.ResponseID+"/"+code]
			if fallback.Question == "" {
				fallback.Question = code
			}
			if !ok || a.Quote == "" {
				continue
			}
			if a.Sentiment < lowestSentiment {
				fallback.Question, fallback.Quote, fallback.Analyzer = code, a.Quote, a.Analyzer
				lowestSentiment = a.Sentiment
			}
			for _, th := range themes {
				if !hasAny(a.Themes, th) {
					continue
				}
				e.Question, e.Quote, e.Analyzer = code, a.Quote, a.Analyzer
				if q := a.ByTheme[th]; q != "" {
					e.Quote = q
					// В разборе модели ByTheme хранит цитаты добавленных словарём тем.
					if strings.HasPrefix(a.Analyzer, "llm") {
						e.Analyzer = "offline"
					}
				}
				return e
			}
		}
		return fallback
	}

	// S1/S2 — резкий провал к собственной базе ИЛИ устойчивое снижение (methodology §3.4)
	sigma := in.Sigma
	if sigma <= 0 {
		sigma = NoiseSigma(map[string]*PairView{v.Pair.ID: c})
	}
	for _, d := range []struct {
		typ, role, who string
		s              []float64
	}{{"S1", "intern", "удовлетворённость стажёра", c.InternIndex}, {"S2", "mentor", "оценка стажёра наставником", c.MentorIndex}} {
		dec, ok := Decline(d.s, sigma, t)
		if !ok || !(dec.Drop || dec.Trend) {
			continue
		}
		sev := Medium
		if (dec.Drop && dec.Trend) || dec.Last < t.LowScore {
			sev = High
		}
		why := []string{}
		if dec.Drop {
			why = append(why, fmt.Sprintf("резкий провал: z = %s к собственной базе %s (порог %s, σ = %s)",
				NumSigned(dec.Z), Num(dec.Base), Num(-t.ZDrop), Num(dec.SigmaP)))
		}
		if dec.Trend {
			why = append(why, fmt.Sprintf("устойчивое снижение: %s балла/мес за 4 месяца, на %s ниже базы %s (пороги %s и %s)",
				Num(-dec.Slope), Num(dec.Base-dec.Last), Num(dec.Base), Num(-t.DeclineSlope), Num(t.DeclineFromBase)))
		}
		first := dec.At
		if dec.Trend {
			first = dec.From
		}
		ev := []Evidence{ref(d.role, dec.At, "", dec.Last)}
		if first != dec.At {
			ev = []Evidence{ref(d.role, first, "", d.s[first]), ev[0]}
		}
		add(d.typ, sev, "rules", first+1, dec.At+1,
			fmt.Sprintf("%s в M%d — %s: %s", capitalize(d.who), dec.At+1, Num(dec.Last), strings.Join(why, "; ")), ev)
	}

	// S3 — устойчивое расхождение на зеркальных вопросах (берём самое сильное)
	var best *Signal
	bestGap := 0.0
	for _, q := range in.Survey.ByRole(survey.Intern) {
		if q.Mirror == "" || q.Kind != "scale" {
			continue
		}
		a, b := c.Questions[q.Code], c.Questions[q.Mirror]
		first, last, mg, ok := runOf(a, b, t.Persist, func(x, y float64) bool { return math.Abs(x-y) >= t.GapMin })
		if !ok || (best != nil && mg <= bestGap) {
			continue
		}
		sev := Medium
		if mg >= t.GapHigh {
			sev = High
		}
		mq, _ := in.Survey.Q(q.Mirror)
		s := Signal{Severity: sev, FirstPeriod: first + 1, LastPeriod: last + 1,
			Explanation: fmt.Sprintf("«%s» (стажёр) и «%s» (наставник) расходятся до %s балла с M%d по M%d (порог %s, не меньше %d мес. подряд)",
				q.Text, mq.Text, Num(mg), first+1, last+1, Num(t.GapMin), t.Persist),
			Evidence: []Evidence{ref("intern", last, q.Code, a[last]), ref("mentor", last, q.Mirror, b[last])}}
		best, bestGap = &s, mg
	}
	if best != nil {
		add("S3", best.Severity, "rules", best.FirstPeriod, best.LastPeriod, best.Explanation, best.Evidence)
	}

	// S4 — «слепое пятно» в обе стороны на общей оценке
	for _, q := range in.Survey.ByRole(survey.Intern) {
		if q.Mirror == "" || q.Dimension != "satisfaction" {
			continue
		}
		a, b := c.Questions[q.Code], c.Questions[q.Mirror]
		if f, l, _, ok := runOf(a, b, t.Persist, func(x, y float64) bool { return x >= t.BlindSelf && y <= t.BlindOther }); ok {
			add("S4", Medium, "rules", f+1, l+1,
				fmt.Sprintf("С M%d по M%d стажёр оценивает стажировку на %.0f+ балла, а наставник его работу — на %.0f и ниже: стажёр может не знать о претензиях", f+1, l+1, t.BlindSelf, t.BlindOther),
				[]Evidence{ref("intern", l, q.Code, a[l]), ref("mentor", l, q.Mirror, b[l])})
		} else if f, l, _, ok := runOf(b, a, t.Persist, func(x, y float64) bool { return x >= t.BlindSelf && y <= t.BlindOther }); ok {
			add("S4", Medium, "rules", f+1, l+1,
				fmt.Sprintf("С M%d по M%d наставник доволен работой (%.0f+), а стажёр оценивает стажировку на %.0f и ниже: наставник может не видеть проблему", f+1, l+1, t.BlindSelf, t.BlindOther),
				[]Evidence{ref("mentor", l, q.Mirror, b[l]), ref("intern", l, q.Code, a[l])})
		}
	}

	// S5 — разнонаправленные тренды
	si, ok1 := Slope(c.InternIndex, 3)
	sm, ok2 := Slope(c.MentorIndex, 3)
	// обе стороны должны заметно двигаться: колебания ±0.1 в стабильной паре — не сигнал
	if ok1 && ok2 && si*sm < 0 && math.Abs(si-sm) >= t.Diverge && math.Abs(si) >= 0.15 && math.Abs(sm) >= 0.15 {
		last := lastIndex(c.InternIndex)
		add("S5", Medium, "rules", max(last-1, 1), last+1,
			fmt.Sprintf("За последние 3 месяца индекс стажёра меняется на %s/мес, а оценка наставника — на %s/мес: стороны движутся в разные стороны", NumSigned(si), NumSigned(sm)),
			[]Evidence{ref("intern", last, "", c.InternIndex[last])})
	}

	// S6 — низкая поддержка или понятность в текущем месяце (берём самую низкую из двух)
	var low6 *survey.Question
	for _, q := range in.Survey.ByRole(survey.Intern) {
		if q.Dimension != "support" && q.Dimension != "clarity" {
			continue
		}
		s := c.Questions[q.Code]
		last := lastIndex(s)
		if last < 0 || s[last] > t.LowDimension {
			continue
		}
		if low6 == nil {
			q := q
			low6 = &q
			continue
		}
		if ls := c.Questions[low6.Code]; s[last] < ls[lastIndex(ls)] {
			q := q
			low6 = &q
		}
	}
	for _, q := range in.Survey.ByRole(survey.Intern) {
		if low6 == nil || q.Code != low6.Code {
			continue
		}
		s := c.Questions[q.Code]
		last := lastIndex(s)
		sev, first := Medium, last
		if last > 0 && !math.IsNaN(s[last-1]) && s[last-1] <= t.LowDimension {
			sev, first = High, last-1
		}
		add("S6", sev, "rules", first+1, last+1,
			fmt.Sprintf("«%s» — %s из 5 в M%d (порог %s)", q.Text, Num(s[last]), last+1, Num(t.LowDimension)),
			[]Evidence{ref("intern", last, q.Code, s[last])})
		break
	}

	// S7 — потеря мотивации: числа И текст
	for _, q := range in.Survey.ByRole(survey.Intern) {
		if q.Dimension != "interest" {
			continue
		}
		s := c.Questions[q.Code]
		// база интереса — медиана M1–M2 (одна первая точка шумная); если ответ только один — он сам
		f0, fi := firstValue(s)
		if fi >= 0 && fi < 2 {
			if b := median(s[:2]); !math.IsNaN(b) {
				f0 = b
			}
		}
		last := lastIndex(s)
		if fi < 0 || last <= fi || last < 2 || f0-s[last] < t.InterestDrop {
			continue
		}
		textHit, at, why := false, -1, ""
		for i := last; i >= max(0, last-1); i-- {
			if hasAny(c.Themes["intern"][i], "routine", "no_growth") {
				textHit, at, why = true, i, "в комментариях — рутина или отсутствие развития"
				break
			}
			if !math.IsNaN(c.Sentiment["intern"][i]) && c.Sentiment["intern"][i] <= t.NegSentiment {
				textHit, at, why = true, i, fmt.Sprintf("тональность комментария %s (порог %s)", Num(c.Sentiment["intern"][i]), Num(t.NegSentiment))
				break
			}
		}
		if textHit {
			add("S7", High, "hybrid", fi+1, last+1,
				fmt.Sprintf("Интерес к задачам упал с %s (база M1–M2) до %s (M%d), падение ≥ %s; %s", Num(f0), Num(s[last]), last+1, Num(t.InterestDrop), why),
				[]Evidence{ref("intern", last, q.Code, s[last]), quote("intern", at, "routine", "no_growth")})
		}
	}

	// Падение интереса или понятности видно и без комментария. При высокой
	// текущей оценке ждём второго последовательного снижения.
	for _, q := range in.Survey.ByRole(survey.Intern) {
		if q.Kind != "scale" || (q.Dimension != "clarity" && q.Dimension != "interest") || cutoff < 2 {
			continue
		}
		s := c.Questions[q.Code]
		now, prev := cutoff-1, cutoff-2
		if math.IsNaN(s[now]) || math.IsNaN(s[prev]) || s[prev]-s[now] < t.InterestDrop {
			continue
		}
		if s[now] > t.LowScore && (prev == 0 || math.IsNaN(s[prev-1]) || s[prev-1] <= s[prev]) {
			continue
		}
		covered := false
		for _, existing := range out {
			if (existing.Type == "S6" || existing.Type == "S7") && existing.LastPeriod == cutoff && existing.Evidence[0].Question == q.Code {
				covered = true
			}
		}
		if covered {
			continue
		}
		add("S15", Medium, "rules", prev+1, cutoff, fmt.Sprintf("«%s»: %s в M%d → %s в M%d.", q.Text, Num(s[prev]), prev+1, Num(s[now]), cutoff), []Evidence{ref("intern", prev, q.Code, s[prev]), ref("intern", now, q.Code, s[now])})
		out[len(out)-1].ID += "-" + q.Code
	}

	// Свежая адресная жалоба даёт повод для личного разговора и без падения оценок.
	for _, role := range []string{"intern", "mentor"} {
		now := cutoff - 1
		for _, theme := range c.Themes[role][now] {
			meta, ok := textai.ThemeByCode(theme)
			if !ok || !meta.AboutOther || !meta.Negative {
				continue
			}
			e := quote(role, now, theme)
			if !directedFeedback(role, e.Quote, meta) {
				continue
			}
			add("S14", Medium, "text", cutoff, cutoff, "В комментарии автора есть негативная тема и упоминание второй стороны. На личной встрече уточните, к чему относилась реплика.", []Evidence{e})
			out[len(out)-1].ID += "-" + role
			break
		}
	}

	// S8 — наставник недоволен устойчиво
	for _, q := range in.Survey.ByRole(survey.Mentor) {
		if q.Dimension != "performance" {
			continue
		}
		s := c.Questions[q.Code]
		if f, l, _, ok := runOf(s, s, t.Persist, func(x, _ float64) bool { return x <= t.MentorLow }); ok {
			sev := Medium
			if s[l] < t.MentorLow {
				sev = High
			}
			add("S8", sev, "rules", f+1, l+1,
				fmt.Sprintf("«%s» — %s и ниже %d мес. подряд (M%d–M%d)", q.Text, Num(t.MentorLow), l-f+1, f+1, l+1),
				[]Evidence{ref("mentor", l, q.Code, s[l])})
		}
	}

	// S9 — одна и та же негативная тема ≥2 месяцев у одной стороны
	for _, role := range []string{"intern", "mentor"} {
		count := map[string][]int{}
		for i, ts := range c.Themes[role] {
			for _, th := range uniq(ts) {
				if tt, ok := textai.ThemeByCode(th); ok && tt.Negative {
					count[th] = append(count[th], i)
				}
			}
		}
		bestTheme, bestN := "", 0
		for th, idx := range count {
			// Старая жалоба с большим числом упоминаний не должна скрыть
			// свежий повтор по другой теме.
			if idx[len(idx)-1]+1 < cutoff-1 {
				continue
			}
			if len(idx) > bestN || (len(idx) == bestN && th < bestTheme) {
				bestTheme, bestN = th, len(idx)
			}
		}
		if bestN >= 2 {
			idx := count[bestTheme]
			tt, _ := textai.ThemeByCode(bestTheme)
			sev := Low
			if bestN >= 3 {
				sev = Medium
			}
			who := "стажёра"
			if role == "mentor" {
				who = "наставника"
			}
			ev := []Evidence{}
			for _, i := range idx[max(0, len(idx)-2):] {
				ev = append(ev, quote(role, i, bestTheme))
			}
			out = append(out, Signal{ID: v.Pair.ID + "-S9-" + role, Pair: v.Pair.ID, Type: "S9", Title: Types["S9"].Title + " (" + who + ")",
				Severity: sev, Source: "text", FirstPeriod: idx[0] + 1, LastPeriod: idx[len(idx)-1] + 1,
				Explanation: fmt.Sprintf("Тема «%s» в комментариях %s повторяется %d мес.: %s", tt.Label, who, bestN, periods(idx)),
				Action:      Types["S9"].Action, DueDays: Types["S9"].DueDays, Evidence: ev})
		}
	}

	// S10 — конфликт или риск ухода: текст, подтверждённый числами
	var confirmation *Signal
	for i := range out {
		s := &out[i]
		if s.LastPeriod >= cutoff-1 && (s.Type == "S1" || s.Type == "S3" || s.Type == "S8" || s.Type == "S6") {
			if confirmation == nil || s.LastPeriod > confirmation.LastPeriod {
				confirmation = s
			}
		}
	}
	if confirmation != nil {
	conflict:
		for i := cutoff - 1; i >= max(0, cutoff-2); i-- {
			for _, role := range []string{"intern", "mentor"} {
				ts := c.Themes[role][i]
				if hasAny(ts, "conflict", "attrition") {
					theme := "conflict"
					what, who := "конфликта в паре", "стажёра"
					if !hasAny(ts, "conflict") {
						theme, what = "attrition", "риска ухода"
					}
					q := quote(role, i, theme)
					if q.Quote == "" {
						continue
					}
					if role == "mentor" {
						who = "наставника"
					}
					add("S10", High, "hybrid", i+1, i+1,
						fmt.Sprintf("В комментарии %s за M%d есть признаки %s. Отдельный сигнал по оценкам — «%s» (M%d).", who, i+1, what, confirmation.Title, confirmation.LastPeriod),
						append([]Evidence{q}, confirmation.Evidence...))
					s := &out[len(out)-1]
					s.Title = "Сигнал о возможном конфликте"
					if theme == "attrition" {
						s.Title = "Риск ухода со стажировки"
						s.Action = "Предложите стажёру личный разговор, уточните его планы и спросите, какая поддержка ему нужна."
					}
					break conflict
				}
			}
		}
	}

	// S11: пока приём открыт, участники ещё могут ответить за этот месяц.
	completed := cutoff
	if in.WindowOpen && cutoff == in.WindowPeriod {
		completed--
	}
	for _, role := range []string{"intern", "mentor"} {
		idx := c.InternIndex
		if role == "mentor" {
			idx = c.MentorIndex
		}
		var missing []int
		for i := 0; i < completed && i < len(idx); i++ {
			if math.IsNaN(idx[i]) && c.ResponseIDs[role][i] == "" {
				missing = append(missing, i)
			}
		}
		if len(missing) == 0 {
			continue
		}
		// Ответ между пропусками начинает новый эпизод. Напоминание показывает
		// последнюю непрерывную серию; предыдущие пропуски видны в истории.
		start := len(missing) - 1
		for start > 0 && missing[start]-missing[start-1] == 1 {
			start--
		}
		missing = missing[start:]
		sev := Low
		if len(missing) >= 2 {
			sev = Medium
		}
		who := "Стажёр"
		if role == "mentor" {
			who = "Наставник"
		}
		out = append(out, Signal{ID: v.Pair.ID + "-S11-" + role, Pair: v.Pair.ID, Type: "S11", Title: Types["S11"].Title, Severity: sev,
			Source: "rules", FirstPeriod: missing[0] + 1, LastPeriod: missing[len(missing)-1] + 1,
			Explanation: fmt.Sprintf("%s не заполнил анкету: %s", who, periods(missing)), Action: Types["S11"].Action, DueDays: Types["S11"].DueDays,
			Evidence: []Evidence{}})
		first := missing[0]
		if first >= 2 && missing[len(missing)-1] == completed-1 && idx[first-2]-idx[first-1] >= t.DeclineFromBase {
			sig := &out[len(out)-1]
			sig.Title = "Снижение оценки и пропуск анкеты"
			sig.Action = "Проведите личный чек-ин с участником и уточните, как идут дела и нужна ли поддержка."
			sig.Severity = Medium
			sig.Explanation += fmt.Sprintf(". Перед пропуском индекс снизился с %s до %s.", Num(idx[first-2]), Num(idx[first-1]))
			sig.Evidence = []Evidence{ref(role, first-2, "", idx[first-2]), ref(role, first-1, "", idx[first-1])}
		}
	}

	// S12 — комментарии стали формальными
	for _, role := range []string{"intern", "mentor"} {
		w := c.Words[role]
		last := lastIndex(w)
		if last < 2 {
			continue
		}
		if shortened(w, last, t) {
			avg := mean(w[:last])
			who := "стажёра"
			if role == "mentor" {
				who = "наставника"
			}
			out = append(out, Signal{ID: v.Pair.ID + "-S12-" + role, Pair: v.Pair.ID, Type: "S12", Title: Types["S12"].Title, Severity: Low,
				Source: "rules", FirstPeriod: last + 1, LastPeriod: last + 1,
				Explanation: fmt.Sprintf("Комментарий %s в M%d: слов — %.0f, обычно — %.0f", who, last+1, w[last], avg),
				Action:      Types["S12"].Action, DueDays: Types["S12"].DueDays, Evidence: []Evidence{shortQuote(in, quote(role, last))}})
		}
	}
	// S13 — формальная анкета: одинаковые шкалы И признак невовлечённости (пустой или 1–2-словный комментарий,
	// резкое сокращение комментария как в S12). Одинаковые ответы с содержательным комментарием — валидный
	// straight-lining (Reuning & Plutzer 2020), «все = max» с комментарием ≥ 5 слов — потолок шкалы, а не формальность.
	for _, role := range []string{"intern", "mentor"} {
		f, w := c.Flat[role], c.Words[role]
		last := -1
		for i := len(f) - 1; i >= 0; i-- {
			if f[i] {
				last = i
				break
			}
		}
		if last < 0 {
			continue
		}
		first := last
		for first > 0 && f[first-1] {
			first--
		}
		short := math.IsNaN(w[last]) || w[last] <= 2
		shrunk := shortened(w, last, t)
		if !short && !shrunk {
			continue
		}
		if c.AllMax[role][last] && !math.IsNaN(w[last]) && w[last] >= 5 {
			continue
		}
		who := "стажёра"
		if role == "mentor" {
			who = "наставника"
		}
		why := fmt.Sprintf("все оценки одинаковые %d мес. подряд (M%d–M%d)", last-first+1, first+1, last+1)
		if last == first {
			why = fmt.Sprintf("в M%d все оценки одинаковые", last+1)
		}
		if short {
			why += ", а комментарий пустой или из 1–2 слов"
		} else {
			why += ", а комментарий заметно короче обычного"
		}
		ev := []Evidence{ref(role, last, "", c.InternIndex[last])}
		if role == "mentor" {
			ev = []Evidence{ref(role, last, "", c.MentorIndex[last])}
		}
		out = append(out, Signal{ID: v.Pair.ID + "-S13-" + role, Pair: v.Pair.ID, Type: "S13", Title: Types["S13"].Title, Severity: Low,
			Source: "rules", FirstPeriod: first + 1, LastPeriod: last + 1,
			Explanation: fmt.Sprintf("Анкета %s выглядит заполненной формально: %s. Оценкам такой анкеты стоит доверять меньше", who, why),
			Action:      Types["S13"].Action, DueDays: Types["S13"].DueDays, Evidence: ev})
	}
	// активны только свежие находки: отрезок, закончившийся два и более месяца назад, — уже история
	fresh := out[:0]
	for _, s := range out {
		if s.LastPeriod >= cutoff-1 {
			fresh = append(fresh, s)
		}
	}
	out = fresh
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity.rank() != out[j].Severity.rank() {
			return out[i].Severity.rank() > out[j].Severity.rank()
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func hasAny(ts []string, codes ...string) bool {
	for _, t := range ts {
		for _, c := range codes {
			if t == c {
				return true
			}
		}
	}
	return false
}

func uniq(ts []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range ts {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

func periods(idx []int) string {
	parts := make([]string, len(idx))
	for i, x := range idx {
		parts[i] = fmt.Sprintf("M%d", x+1)
	}
	return strings.Join(parts, ", ")
}

func capitalize(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// shortened — комментарий в месяце i заметно короче обычного (правило S12): обычно ≥ 5 слов, сейчас < ShortRatio от обычного.
func shortened(w []float64, i int, t Thresholds) bool {
	if i < 2 || math.IsNaN(w[i]) {
		return false
	}
	avg := mean(w[:i])
	return avg >= 5 && w[i] < avg*t.ShortRatio
}

// shortQuote — для короткого комментария цитатой служит сам комментарий целиком (дословно).
func shortQuote(in Input, e Evidence) Evidence {
	if e.Quote != "" {
		return e
	}
	for _, r := range in.Responses {
		if r.ID == e.ResponseID {
			e.Quote = strings.TrimSpace(r.Comments[e.Question])
		}
	}
	return e
}

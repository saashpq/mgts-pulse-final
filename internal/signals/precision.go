package signals

import (
	"fmt"
	"math"
)

// Suggestion — какой порог загрубить и до какого значения.
type Suggestion struct {
	Field string  `json:"field"` // поле Thresholds (json-имя)
	From  float64 `json:"from"`
	To    float64 `json:"to"`
}

// Precision — точность типа сигнала по вердиктам HR: доля подтверждённых и решённых среди всех вердиктов.
type Precision struct {
	Type      string      `json:"type"`
	Title     string      `json:"title"`
	Confirmed int         `json:"confirmed"`
	False     int         `json:"false"`
	Resolved  int         `json:"resolved"`
	Precision *float64    `json:"precision"` // null — вердиктов ещё нет
	Lo        *float64    `json:"lo"`        // 95% ДИ Клоппера–Пирсона; null — вердиктов нет
	Hi        *float64    `json:"hi"`
	Decisions int         `json:"decisions"` // всего решений HR по типу
	Enough    bool        `json:"enough"`    // решений ≥ MinDecisions: точности можно доверять, подсказки включены
	Hint      string      `json:"hint,omitempty"`
	Suggest   *Suggestion `json:"suggest,omitempty"`
}

// MinDecisions — с какого числа решений HR точность считается достаточно надёжной для подсказок по порогам.
// На 1–4 вердиктах одна ошибка HR меняет точность на 20–100 п. п.
const MinDecisions = 5

// PrecisionTotal — точность по всем типам сигналов.
type PrecisionTotal struct {
	Decisions int      `json:"decisions"`
	Precision *float64 `json:"precision"` // null — решений нет
	Lo        *float64 `json:"lo"`        // 95% ДИ Клоппера–Пирсона; null — решений нет
	Hi        *float64 `json:"hi"`
	Enough    bool     `json:"enough"` // решений ≥ MinDecisions
}

// interval — 95% ДИ Клоппера–Пирсона для good/n (null при n = 0).
func interval(good, n int) (*float64, *float64) {
	if n == 0 {
		return nil, nil
	}
	r := NewRate(good, n)
	return r.Lo, r.Hi
}

func totalPrecision(ps []Precision) PrecisionTotal {
	var t PrecisionTotal
	good := 0
	for _, p := range ps {
		t.Decisions += p.Decisions
		good += p.Confirmed + p.Resolved
	}
	if t.Decisions > 0 {
		x := r2(float64(good) / float64(t.Decisions))
		t.Precision = &x
	}
	t.Lo, t.Hi = interval(good, t.Decisions)
	t.Enough = t.Decisions >= MinDecisions
	return t
}

// tune — какой порог отвечает за тип сигнала и в какую сторону его загрублять (шаг и допустимая граница).
type tune struct {
	field      string
	step, stop float64
	get        func(Thresholds) float64
}

var tuning = map[string]tune{
	"S1":  {"decline_slope", -0.1, -2, func(t Thresholds) float64 { return t.DeclineSlope }},
	"S2":  {"decline_from_base", 0.1, 2, func(t Thresholds) float64 { return t.DeclineFromBase }},
	"S3":  {"gap_min", 0.25, 4, func(t Thresholds) float64 { return t.GapMin }},
	"S4":  {"blind_self", 0.5, 5, func(t Thresholds) float64 { return t.BlindSelf }},
	"S5":  {"diverge", 0.1, 4, func(t Thresholds) float64 { return t.Diverge }},
	"S6":  {"low_dimension", -0.25, 1, func(t Thresholds) float64 { return t.LowDimension }},
	"S7":  {"interest_drop", 0.25, 4, func(t Thresholds) float64 { return t.InterestDrop }},
	"S8":  {"mentor_low", -0.25, 1, func(t Thresholds) float64 { return t.MentorLow }},
	"S12": {"short_ratio", -0.1, 0.05, func(t Thresholds) float64 { return t.ShortRatio }},
}

// PrecisionByType — точность по каждому типу каталога. Считаются вердикты и по активным, и по снятым сигналам.
func PrecisionByType(pairs []PairReport, t Thresholds) []Precision {
	acc := map[string]*Precision{}
	order := TypeOrder()
	for _, typ := range order {
		acc[typ] = &Precision{Type: typ, Title: Types[typ].Title}
	}
	for _, p := range pairs {
		for _, list := range [][]SignalView{p.Signals, p.Resolved} {
			for _, s := range list {
				pr := acc[s.Type]
				if pr == nil || s.Verdict == nil {
					continue
				}
				switch s.Verdict.Status {
				case "confirmed":
					pr.Confirmed++
				case "resolved":
					pr.Resolved++
				case "false":
					pr.False++
				}
			}
		}
	}
	out := make([]Precision, 0, len(order))
	for _, typ := range order {
		pr := acc[typ]
		good := pr.Confirmed + pr.Resolved
		pr.Decisions = good + pr.False
		if pr.Decisions > 0 {
			x := r2(float64(good) / float64(pr.Decisions))
			pr.Precision = &x
		}
		pr.Lo, pr.Hi = interval(good, pr.Decisions)
		pr.Enough = pr.Decisions >= MinDecisions
		// загрубить, если даже верхняя граница ДИ ниже 0.5 или точность < 0.5 при ≥ 2 ложных
		if pr.Enough && (*pr.Hi < 0.5 || (*pr.Precision < 0.5 && pr.False >= 2)) {
			pr.Hint, pr.Suggest = hint(typ, pr.False, t)
		}
		out = append(out, *pr)
	}
	return out
}

func hint(typ string, falses int, t Thresholds) (string, *Suggestion) {
	tn, ok := tuning[typ]
	if !ok {
		return fmt.Sprintf("Ложных срабатываний: %d. Числового порога у сигнала нет — разберите случаи с экспертом (словарь тем или правило)", falses), nil
	}
	from := tn.get(t)
	to := math.Round((from+tn.step)*100) / 100
	if (tn.step > 0 && to > tn.stop) || (tn.step < 0 && to < tn.stop) {
		to = tn.stop
	}
	if to == from {
		return fmt.Sprintf("Ложных срабатываний: %d, а порог %s уже на границе (%s) — пересмотрите правило с экспертом", falses, tn.field, Num(from)), nil
	}
	return fmt.Sprintf("Ложных срабатываний: %d — загрубить порог %s с %s до %s", falses, tn.field, Num(from), Num(to)),
		&Suggestion{Field: tn.field, From: from, To: to}
}

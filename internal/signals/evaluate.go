package signals

import (
	"sort"

	"github.com/ElzaraA/mgts-ai/internal/synth"
)

// Episode — эпизод проблемы с известным месяцем начала (см. synth.Episode).
type Episode = synth.Episode

// DefaultWindow — окно совпадения: тревога засчитывает эпизод, если first_seen ∈ [t0, t0 + W] (methodology §1.1).
const DefaultWindow = 2

// serviceTypes — сигналы качества данных (пропуск анкеты, короткий комментарий, формальная анкета): это повод
// уточнить данные, а не прогноз проблемы, поэтому в оценку раннего предупреждения они не входят.
var serviceTypes = map[string]bool{"S11": true, "S12": true, "S13": true}

// NaiveEval — то же для правила «средняя оценка стажёра или наставника < LowScore».
type NaiveEval struct {
	Found       int      `json:"found"`
	Recall      Rate     `json:"recall"`
	MedianDelay *float64 `json:"median_delay"`
}

// EvalRow — эпизод: когда его заметили система и простое правило (null — не в окне) и итог сравнения.
type EvalRow struct {
	Pair    string `json:"pair"`
	Role    string `json:"role"`
	Class   string `json:"class"`
	T0      int    `json:"t0"`
	Ours    *int   `json:"ours"`
	Naive   *int   `json:"naive"`
	Outcome string `json:"outcome"` // earlier | same | later | only_ours | only_naive | neither
}

// Evaluation — качество раннего предупреждения против истины (эпизодов с известным t0).
type Evaluation struct {
	Window      int       `json:"window"`
	Episodes    int       `json:"episodes"`
	Found       int       `json:"found"`
	Recall      Rate      `json:"recall"`
	Alerts      int       `json:"alerts"`
	TrueAlerts  int       `json:"true_alerts"`
	Precision   Rate      `json:"precision"`
	MedianDelay *float64  `json:"median_delay"` // медиана first_seen − t0 по найденным эпизодам, месяцы
	Naive       NaiveEval `json:"naive"`
	Earlier     int       `json:"earlier"`
	Same        int       `json:"same"`
	Later       int       `json:"later"`
	OnlyOurs    int       `json:"only_ours"`
	OnlyNaive   int       `json:"only_naive"`
	Neither     int       `json:"neither"`
	FalsePairs  []string  `json:"false_pairs"` // пары без эпизодов, по которым была тревога
	Rows        []EvalRow `json:"rows"`
}

// Evaluate сравнивает тревоги отчёта с эпизодами. Эпизод найден, если у пары есть тревога с first_seen ∈ [t0, t0+W];
// тревога ложная, если у пары нет эпизодов или first_seen < t0 − 1 (раньше самого раннего эпизода пары).
// Тревога — уникальный ID сигнала (активный или снятый), кроме сигналов качества данных S11–S13.
// window ≤ 0 — DefaultWindow. Пары без записи в onset в оценку не входят.
func Evaluate(rep ProgramReport, onset map[string][]Episode, window int) Evaluation {
	return evaluate(rep, onset, window, false)
}

func evaluate(rep ProgramReport, onset map[string][]Episode, window int, decidedOnly bool) Evaluation {
	if window <= 0 {
		window = DefaultWindow
	}
	ev := Evaluation{Window: window, FalsePairs: []string{}, Rows: []EvalRow{}}
	var delays, naiveDelays []float64
	pairs := append([]PairReport(nil), rep.Pairs...)
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].PairID < pairs[j].PairID })
	for _, p := range pairs {
		eps, known := onset[p.PairID]
		if !known {
			continue
		}
		var alerts []int
		for _, list := range [][]SignalView{p.Signals, p.Resolved} {
			for _, s := range list {
				if serviceTypes[s.Type] || s.FirstSeen == 0 || (decidedOnly && s.Verdict == nil) {
					continue
				}
				alerts = append(alerts, s.FirstSeen)
			}
		}
		minT0 := 0
		for _, e := range eps {
			if minT0 == 0 || e.T0 < minT0 {
				minT0 = e.T0
			}
		}
		falseHere := 0
		for _, a := range alerts {
			if len(eps) == 0 || a < minT0-1 {
				falseHere++
			}
		}
		ev.Alerts += len(alerts)
		ev.TrueAlerts += len(alerts) - falseHere
		if len(eps) == 0 && len(alerts) > 0 {
			ev.FalsePairs = append(ev.FalsePairs, p.PairID)
		}
		for _, e := range eps {
			ev.Episodes++
			row := EvalRow{Pair: p.PairID, Role: e.Role, Class: e.Class, T0: e.T0}
			for _, a := range alerts {
				if a >= e.T0 && a <= e.T0+window && (row.Ours == nil || a < *row.Ours) {
					a := a
					row.Ours = &a
				}
			}
			if n := p.Detection.Naive; n >= e.T0 && n <= e.T0+window {
				row.Naive = &n
			}
			switch {
			case row.Ours != nil && row.Naive != nil && *row.Ours < *row.Naive:
				row.Outcome, ev.Earlier = "earlier", ev.Earlier+1
			case row.Ours != nil && row.Naive != nil && *row.Ours == *row.Naive:
				row.Outcome, ev.Same = "same", ev.Same+1
			case row.Ours != nil && row.Naive != nil:
				row.Outcome, ev.Later = "later", ev.Later+1
			case row.Ours != nil:
				row.Outcome, ev.OnlyOurs = "only_ours", ev.OnlyOurs+1
			case row.Naive != nil:
				row.Outcome, ev.OnlyNaive = "only_naive", ev.OnlyNaive+1
			default:
				row.Outcome, ev.Neither = "neither", ev.Neither+1
			}
			if row.Ours != nil {
				ev.Found++
				delays = append(delays, float64(*row.Ours-e.T0))
			}
			if row.Naive != nil {
				ev.Naive.Found++
				naiveDelays = append(naiveDelays, float64(*row.Naive-e.T0))
			}
			ev.Rows = append(ev.Rows, row)
		}
	}
	ev.Recall = NewRate(ev.Found, ev.Episodes)
	ev.Precision = NewRate(ev.TrueAlerts, ev.Alerts)
	ev.Naive.Recall = NewRate(ev.Naive.Found, ev.Episodes)
	ev.MedianDelay = medianPtr(delays)
	ev.Naive.MedianDelay = medianPtr(naiveDelays)
	return ev
}

func medianPtr(xs []float64) *float64 {
	if len(xs) == 0 {
		return nil
	}
	m := r2(median(xs))
	return &m
}

// hrEpisodes — истина по решениям HR: подтверждённый или решённый сигнал = эпизод с t0 = first_seen.
func hrEpisodes(pairs []PairReport) map[string][]Episode {
	out := map[string][]Episode{}
	for _, p := range pairs {
		out[p.PairID] = []Episode{}
		for _, list := range [][]SignalView{p.Signals, p.Resolved} {
			for _, s := range list {
				if s.Verdict != nil && (s.Verdict.Status == "confirmed" || s.Verdict.Status == "resolved") && !serviceTypes[s.Type] {
					out[p.PairID] = append(out[p.PairID], Episode{Role: Side(s.Signal), Class: s.Type, T0: s.FirstSeen})
				}
			}
		}
	}
	return out
}

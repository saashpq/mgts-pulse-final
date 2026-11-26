package signals

import "math"

// Agreement — согласие сторон по Бланду–Альтману на знаковой разнице «индекс стажёра − индекс наставника».
// bias > 0 — стажёр оценивает выше, чем наставник. Это описательная статистика: стороны отвечают на разные
// вопросы (methodology §2.0), поэтому расхождение — разница восприятий, а не ошибка измерения.
type Agreement struct {
	Bias   float64 `json:"bias"`
	SD     float64 `json:"sd"`
	LoALo  float64 `json:"loa_lo"`
	LoAHi  float64 `json:"loa_hi"`
	N      int     `json:"n"`
	Method string  `json:"method"`
}

const (
	// MethodSimple — SD разностей одной пары (n − 1).
	MethodSimple = "simple"
	// MethodRepeated — Bland & Altman (2007): повторные измерения при меняющемся истинном значении,
	// SD² = σ²_w + σ²_b из однофакторного ANOVA по парам (D — поправка на неравное число наблюдений).
	MethodRepeated = "bland-altman-2007-repeated"
)

func finite(xs []float64) []float64 {
	var out []float64
	for _, x := range xs {
		if !math.IsNaN(x) {
			out = append(out, x)
		}
	}
	return out
}

func newAgreement(bias, sd float64, n int, method string) *Agreement {
	return &Agreement{Bias: r2(bias), SD: r2(sd), LoALo: r2(bias - 1.96*sd), LoAHi: r2(bias + 1.96*sd), N: n, Method: method}
}

// PairAgreement — смещение и пределы согласия по одной паре; nil, если месяцев с ответами обеих сторон меньше двух.
func PairAgreement(diff []float64) *Agreement {
	d := finite(diff)
	if len(d) < 2 {
		return nil
	}
	return newAgreement(mean(d), sdOf(d), len(d), MethodSimple)
}

// ProgramAgreement — по всей программе с поправкой на повторные измерения (Bland & Altman 2007):
// σ²_w = MS_within, σ²_b = max(0, (MS_between − MS_within)/D), D = [(Σm)² − Σm²]/[(n−1)Σm], LoA = bias ± 1.96·√(σ²_w + σ²_b).
// Если пар с ≥ 1 наблюдением меньше двух или нет повторов — простая SD по всем наблюдениям (method = simple).
// Автокорреляцию ежемесячных ответов метод не учитывает (оговорка авторов).
func ProgramAgreement(diffs [][]float64) *Agreement {
	var all []float64
	var groups [][]float64
	for _, d := range diffs {
		if f := finite(d); len(f) > 0 {
			groups = append(groups, f)
			all = append(all, f...)
		}
	}
	if len(all) < 2 {
		return nil
	}
	bias := mean(all)
	n, total := len(groups), len(all)
	if n < 2 || total <= n {
		return newAgreement(bias, sdOf(all), total, MethodSimple)
	}
	var ssw, ssb, sumM2 float64
	for _, g := range groups {
		gm := mean(g)
		for _, x := range g {
			ssw += (x - gm) * (x - gm)
		}
		ssb += float64(len(g)) * (gm - bias) * (gm - bias)
		sumM2 += float64(len(g) * len(g))
	}
	msw := ssw / float64(total-n)
	msb := ssb / float64(n-1)
	D := (float64(total*total) - sumM2) / (float64(n-1) * float64(total))
	sb2 := math.Max(0, (msb-msw)/D)
	return newAgreement(bias, math.Sqrt(msw+sb2), total, MethodRepeated)
}

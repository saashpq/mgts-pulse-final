// Package signals — детерминированная часть аналитики: динамика, расхождения и сигналы проблем.
// Основа (наклон, падение от пика, устойчивое расхождение, «слепое пятно») перенесена из seed-кода
// PR «Пульс стажировки» (bobifisher, mgts/pulse/internal/signals) и доведена до каталога S1–S13.
// Пропущенный месяц — NaN и никогда не считается нулём.
package signals

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// NaN — пропуск.
var NaN = math.NaN()

// Slope — наклон МНК по последним n непустым точкам ряда.
func Slope(series []float64, n int) (float64, bool) {
	var xs, ys []float64
	for i := len(series) - 1; i >= 0 && len(xs) < n; i-- {
		if !math.IsNaN(series[i]) {
			xs = append(xs, float64(i))
			ys = append(ys, series[i])
		}
	}
	if len(xs) < n || n < 2 {
		return 0, false
	}
	var mx, my float64
	for i := range xs {
		mx += xs[i]
		my += ys[i]
	}
	mx /= float64(len(xs))
	my /= float64(len(ys))
	var num, den float64
	for i := range xs {
		num += (xs[i] - mx) * (ys[i] - my)
		den += (xs[i] - mx) * (xs[i] - mx)
	}
	return num / den, true
}

func lastIndex(s []float64) int {
	for i := len(s) - 1; i >= 0; i-- {
		if !math.IsNaN(s[i]) {
			return i
		}
	}
	return -1
}

func firstValue(s []float64) (float64, int) {
	for i, v := range s {
		if !math.IsNaN(v) {
			return v, i
		}
	}
	return NaN, -1
}

// runOf — последний отрезок из ≥minLen подряд идущих месяцев, где cond истинно. Возвращает 0-based границы.
func runOf(a, b []float64, minLen int, cond func(a, b float64) bool) (first, last int, maxGap float64, ok bool) {
	start, mg := -1, 0.0
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i <= n; i++ {
		hit := i < n && !math.IsNaN(a[i]) && !math.IsNaN(b[i]) && cond(a[i], b[i])
		if hit {
			if start < 0 {
				start, mg = i, 0
			}
			mg = math.Max(mg, math.Abs(a[i]-b[i]))
			continue
		}
		if start >= 0 && i-start >= minLen {
			first, last, maxGap, ok = start, i-1, mg, true
		}
		start = -1
	}
	return
}

func mean(xs []float64) float64 {
	s, n := 0.0, 0
	for _, x := range xs {
		if !math.IsNaN(x) {
			s += x
			n++
		}
	}
	if n == 0 {
		return NaN
	}
	return s / float64(n)
}

func r2(f float64) float64 {
	if math.IsNaN(f) {
		return f
	}
	return math.Round(f*100) / 100
}

// Num — единственный формат чисел в текстах сигналов, брифа и сводки: r2 (округление половины от нуля)
// и ровно 2 знака. Прямой %.2f округляет половину к чётному по двоичному представлению (3.125 → «3.12»),
// а r2 в JSON дал бы 3.13 — HR увидел бы два разных числа.
func Num(f float64) string {
	x := r2(f)
	if x == 0 {
		x = 0 // без «-0.00»
	}
	return strconv.FormatFloat(x, 'f', 2, 64)
}

// NumSigned — Num со знаком: «+0.25», «-0.10».
func NumSigned(f float64) string {
	s := Num(f)
	if !strings.HasPrefix(s, "-") {
		s = "+" + s
	}
	return s
}

// median — медиана непустых значений; NaN, если их нет.
func median(xs []float64) float64 {
	var v []float64
	for _, x := range xs {
		if !math.IsNaN(x) {
			v = append(v, x)
		}
	}
	if len(v) == 0 {
		return NaN
	}
	sort.Float64s(v)
	m := len(v) / 2
	if len(v)%2 == 1 {
		return v[m]
	}
	return (v[m-1] + v[m]) / 2
}

// TheilSen — оценка наклона Тейла–Сена: медиана попарных наклонов (y_j − y_i)/(j − i) по последним n непустым
// точкам ряда (индексы — номера месяцев, пропуски учитываются). Устойчива к одному выбросу на 4 точках.
// ok=false, если непустых точек меньше n или n < 2.
func TheilSen(s []float64, n int) (float64, bool) {
	var xs, ys []float64
	for i := len(s) - 1; i >= 0 && len(xs) < n; i-- {
		if !math.IsNaN(s[i]) {
			xs = append(xs, float64(i))
			ys = append(ys, s[i])
		}
	}
	if n < 2 || len(xs) < n {
		return 0, false
	}
	var slopes []float64
	for i := range xs {
		for j := i + 1; j < len(xs); j++ {
			slopes = append(slopes, (ys[i]-ys[j])/(xs[i]-xs[j]))
		}
	}
	return median(slopes), true
}

// Baseline — собственная база человека на месяц t (индекс с нуля): медиана непустых точек s[0..t−1].
// Возвращает базу и число точек; ok=false, если точек меньше двух.
func Baseline(s []float64, t int) (base float64, n int, ok bool) {
	if t > len(s) {
		t = len(s)
	}
	var pts []float64
	for i := 0; i < t; i++ {
		if !math.IsNaN(s[i]) {
			pts = append(pts, s[i])
		}
	}
	if len(pts) < 2 {
		return NaN, len(pts), false
	}
	return median(pts), len(pts), true
}

// MinSigma — нижняя граница σ шума: у человека, который всегда ставит 5, разброс 0, и без границы z = −∞.
const MinSigma = 0.25

// NoiseSigma — σ̂ шума индексов программы по последовательным разностям (MSSD):
// √(Σ(x_t − x_{t−1})² / (2M)), M — число пар соседних непустых месяцев по всем парам и обеим ролям.
// Разности устойчивее к тренду, чем SD. Нижняя граница MinSigma.
func NoiseSigma(views map[string]*PairView) float64 {
	var ss float64
	var m int
	for _, v := range views {
		for _, s := range [][]float64{v.InternIndex, v.MentorIndex} {
			for i := 1; i < len(s); i++ {
				if !math.IsNaN(s[i]) && !math.IsNaN(s[i-1]) {
					d := s[i] - s[i-1]
					ss += d * d
					m++
				}
			}
		}
	}
	if m == 0 {
		return MinSigma
	}
	return math.Max(MinSigma, math.Sqrt(ss/float64(2*m)))
}

// ShrinkNu — сила сжатия личного разброса к пулу программы («≈ 4 точки априорной информации»).
const ShrinkNu = 4.0

// WithinZ — отклонение от собственной базы в единицах сжатой σ: z = (x − base)/σ̃,
// σ̃² = (ν₀σ̂² + (n_p − 1)s_p²)/(ν₀ + n_p − 1), ν₀ = 4, σ̃ ≥ MinSigma.
// s_p, n_p — SD и число точек базы человека, sigma — σ̂ программы. Возвращает z и σ̃.
func WithinZ(x, base, sp float64, np int, sigma float64) (float64, float64) {
	df := float64(np - 1)
	if df < 0 || math.IsNaN(sp) {
		df, sp = 0, 0
	}
	v := (ShrinkNu*sigma*sigma + df*sp*sp) / (ShrinkNu + df)
	st := math.Max(MinSigma, math.Sqrt(v))
	return (x - base) / st, st
}

// sdOf — выборочное SD непустых точек (n − 1); 0 при одной точке.
func sdOf(xs []float64) float64 {
	m := mean(xs)
	var ss float64
	n := 0
	for _, x := range xs {
		if !math.IsNaN(x) {
			ss += (x - m) * (x - m)
			n++
		}
	}
	if n < 2 {
		return 0
	}
	return math.Sqrt(ss / float64(n-1))
}

// DeclineCheck — разбор правила S1/S2 на последней точке ряда.
type DeclineCheck struct {
	At, From int     // индекс последней точки и первой точки окна Тейла–Сена (с нуля)
	Last     float64 // последнее значение
	Base     float64 // медиана собственных ответов до последней точки
	Z        float64 // отклонение от базы в единицах сжатой σ
	SigmaP   float64 // сжатая σ человека
	Slope    float64 // наклон Тейла–Сена по 4 точкам (0, если точек меньше)
	Drop     bool    // z ≤ −ZDrop
	Trend    bool    // наклон ≤ DeclineSlope и база − последнее ≥ DeclineFromBase
}

// Decline — правило S1/S2: z ≤ −ZDrop (резкий провал) ИЛИ (TS₄ ≤ DeclineSlope и base − last ≥ DeclineFromBase).
// ok=false — нет последней точки или базы из ≥ 2 точек.
func Decline(s []float64, sigma float64, t Thresholds) (DeclineCheck, bool) {
	last := lastIndex(s)
	if last < 0 {
		return DeclineCheck{}, false
	}
	base, n, ok := Baseline(s, last)
	if !ok {
		return DeclineCheck{}, false
	}
	d := DeclineCheck{At: last, From: last, Last: s[last], Base: base}
	d.Z, d.SigmaP = WithinZ(s[last], base, sdOf(s[:last]), n, sigma)
	d.Drop = d.Z <= -t.ZDrop
	if ts, ok := TheilSen(s[:last+1], 4); ok {
		d.Slope = ts
		cnt := 0
		for i := last; i >= 0 && cnt < 4; i-- {
			if !math.IsNaN(s[i]) {
				d.From = i
				cnt++
			}
		}
		d.Trend = ts <= t.DeclineSlope && base-s[last] >= t.DeclineFromBase
	}
	return d, true
}

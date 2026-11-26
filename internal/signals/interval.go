package signals

import "math"

// binomCDF — P(X ≤ k) для X ~ Bin(n, p), через логарифмы (math.Lgamma) — без переполнения при больших n.
func binomCDF(k, n int, p float64) float64 {
	if k < 0 {
		return 0
	}
	if k >= n {
		return 1
	}
	if p <= 0 {
		return 1
	}
	if p >= 1 {
		return 0
	}
	ln := func(x float64) float64 { v, _ := math.Lgamma(x); return v }
	lp, lq := math.Log(p), math.Log1p(-p)
	sum := 0.0
	for i := 0; i <= k; i++ {
		sum += math.Exp(ln(float64(n+1)) - ln(float64(i+1)) - ln(float64(n-i+1)) + float64(i)*lp + float64(n-i)*lq)
	}
	return math.Min(1, sum)
}

// bisect — корень монотонной f на [0, 1] (f(lo) и f(hi) разных знаков) с точностью 1e-10.
func bisect(f func(float64) float64) float64 {
	lo, hi := 0.0, 1.0
	flo := f(lo)
	for i := 0; i < 100 && hi-lo > 1e-10; i++ {
		mid := (lo + hi) / 2
		if fm := f(mid); (fm > 0) == (flo > 0) {
			lo, flo = mid, fm
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// ClopperPearson — точный 95%-интервал доли x/n: Lo = B⁻¹(0.025; x, n−x+1), Hi = B⁻¹(0.975; x+1, n−x).
// Квантили бета-распределения ищутся бисекцией по биномиальной CDF: P(X ≥ x | Lo) = α/2, P(X ≤ x | Hi) = α/2.
// При x = 0 Lo = 0, при x = n Hi = 1. Метод консервативный: фактическое покрытие ≥ 95%.
func ClopperPearson(x, n int) (lo, hi float64) {
	if n <= 0 || x < 0 || x > n {
		return 0, 1
	}
	const a2 = 0.025
	if x > 0 {
		lo = bisect(func(p float64) float64 { return (1 - binomCDF(x-1, n, p)) - a2 })
	}
	hi = 1
	if x < n {
		hi = bisect(func(p float64) float64 { return binomCDF(x, n, p) - a2 })
	}
	return lo, hi
}

// Rate — доля с 95%-интервалом Клоппера–Пирсона; value = null, если n = 0.
type Rate struct {
	Value *float64 `json:"value"`
	Lo    *float64 `json:"lo"`
	Hi    *float64 `json:"hi"`
	X     int      `json:"x"`
	N     int      `json:"n"`
}

// NewRate — x/n с интервалом, округлённым до 3 знаков.
func NewRate(x, n int) Rate {
	r := Rate{X: x, N: n}
	if n == 0 {
		return r
	}
	v := r3(float64(x) / float64(n))
	lo, hi := ClopperPearson(x, n)
	lo, hi = r3(lo), r3(hi)
	r.Value, r.Lo, r.Hi = &v, &lo, &hi
	return r
}

func r3(f float64) float64 { return math.Round(f*1000) / 1000 }

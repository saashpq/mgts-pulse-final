package synth

import (
	"math"
	"math/rand"
)

// RandomParams — генератор рядов индекса одной роли с известным началом проблемы (для калибровки и Монте-Карло).
// Допущения methodology.md §0: базовый уровень пары ~U(BaseLo; BaseHi), Items пунктов, шум пункта N(0; ItemSD),
// округление до целого и обрезка до 1…5; индекс — среднее пунктов. Это допущения, а не данные МГТС.
type RandomParams struct {
	Kind    string  // stable | step | grad
	Pairs   int     // число рядов
	Periods int     // месяцев (по умолчанию 6)
	Items   int     // пунктов в индексе (по умолчанию 4)
	ItemSD  float64 // шум пункта (по умолчанию 0.6)
	T0      int     // месяц начала проблемы, 1-based (по умолчанию 3)
	Step    float64 // сдвиг для step (по умолчанию −1.0)
	Grad    float64 // сдвиг в месяц для grad (по умолчанию −0.4)
	BaseLo  float64 // по умолчанию 3.6
	BaseHi  float64 // по умолчанию 4.6
}

func (p *RandomParams) defaults() {
	def := func(x *float64, d float64) {
		if *x == 0 {
			*x = d
		}
	}
	if p.Periods == 0 {
		p.Periods = 6
	}
	if p.Items == 0 {
		p.Items = 4
	}
	if p.T0 == 0 {
		p.T0 = 3
	}
	def(&p.ItemSD, 0.6)
	def(&p.Step, -1.0)
	def(&p.Grad, -0.4)
	def(&p.BaseLo, 3.6)
	def(&p.BaseHi, 4.6)
}

// Shift — сдвиг истинного уровня в месяце t (1-based) для сценария.
func (p RandomParams) Shift(t int) float64 {
	if t < p.T0 {
		return 0
	}
	switch p.Kind {
	case "step":
		return p.Step
	case "grad":
		return p.Grad * float64(t-p.T0+1)
	}
	return 0
}

// Random — Pairs рядов индекса; детерминирован при одном seed.
func Random(seed int64, p RandomParams) [][]float64 {
	p.defaults()
	rng := rand.New(rand.NewSource(seed))
	out := make([][]float64, p.Pairs)
	for i := range out {
		base := p.BaseLo + rng.Float64()*(p.BaseHi-p.BaseLo)
		s := make([]float64, p.Periods)
		for t := range s {
			mu := base + p.Shift(t+1)
			sum := 0.0
			for k := 0; k < p.Items; k++ {
				x := math.Round(mu + rng.NormFloat64()*p.ItemSD)
				sum += math.Max(1, math.Min(5, x))
			}
			s[t] = sum / float64(p.Items)
		}
		out[i] = s
	}
	return out
}

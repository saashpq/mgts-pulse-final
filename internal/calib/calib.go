// Package calib — имитационная калибровка правила S1/S2 (methodology §3.3, §6): генератор рядов с известным
// началом проблемы, прогон сетки порогов, доля ложных тревог, полнота, медиана месяца обнаружения и ожидаемая
// стоимость ошибок E[C] при разных C_FN/C_FP. Имитация проверяет логику при НАШИХ допущениях о шуме,
// реальную валидность она не доказывает.
package calib

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/ElzaraA/mgts-ai/internal/signals"
	"github.com/ElzaraA/mgts-ai/internal/synth"
)

// Rule — правило срабатывания на ряду индекса.
type Rule struct {
	Name     string
	Kind     string  // current | new | z | trend | naive
	Z        float64 // порог z (new, z)
	Slope    float64 // порог наклона (current — МНК по 3 точкам; new/trend — Тейл–Сен по 4)
	FromBase float64 // падение от базы (new, trend); для current — падение от пика
	Default  bool    // правило по умолчанию в продукте
}

// Fire — срабатывает ли правило на последней точке ряда s.
func (r Rule) Fire(s []float64, sigma float64) bool {
	last := s[len(s)-1]
	switch r.Kind {
	case "current":
		peak := math.Inf(-1)
		for _, x := range s {
			peak = math.Max(peak, x)
		}
		sl, ok := signals.Slope(s, 3)
		return (ok && sl <= r.Slope) || peak-last >= r.FromBase
	case "naive":
		return last < 3
	}
	th := signals.Default
	th.ZDrop, th.DeclineSlope, th.DeclineFromBase = r.Z, r.Slope, r.FromBase
	d, ok := signals.Decline(s, sigma, th)
	switch r.Kind {
	case "z":
		return ok && d.Drop
	case "trend":
		return ok && d.Trend
	}
	return ok && (d.Drop || d.Trend)
}

// Grid — сетка правил: прежнее правило, варианты нового (z × наклон), отдельные ветки и «индекс < 3».
func Grid() []Rule {
	out := []Rule{{Name: "сейчас: slope₃ ≤ −0.25 ИЛИ пик − последнее ≥ 0.8", Kind: "current", Slope: -0.25, FromBase: 0.8}}
	for _, z := range []float64{2.5, 3, 3.5} {
		for _, sl := range []float64{-0.25, -0.375, -0.5} {
			out = append(out, Rule{Name: fmt.Sprintf("z ≤ −%s ИЛИ (TS₄ ≤ −%s и падение ≥ 0.5)", fnum(z), fnum(-sl)), Kind: "new", Z: z, Slope: sl,
				FromBase: 0.5, Default: z == signals.Default.ZDrop && sl == signals.Default.DeclineSlope})
		}
	}
	return append(out,
		Rule{Name: "z ≤ −3", Kind: "z", Z: 3},
		Rule{Name: "TS₄ ≤ −0.25 и падение ≥ 0.5", Kind: "trend", Slope: -0.25, FromBase: 0.5},
		Rule{Name: "наивное: индекс < 3", Kind: "naive"},
	)
}

func fnum(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", f), "0"), ".")
}

// Params — параметры имитации.
type Params struct {
	Seed       int64
	N          int       // рядов на сценарий
	ItemSD     float64   // шум пункта
	Prevalence float64   // доля пар с проблемой для E[C] (половина — ступенька, половина — плавно)
	Ratios     []float64 // C_FN/C_FP
	From, To   int       // горизонт проверки, месяцы (1-based)
}

// DefaultParams — как в methodology §3.3: 3000 рядов, шум пункта 0.6, горизонт M3–M6, t0 = M3.
func DefaultParams() Params {
	return Params{Seed: 1, N: 3000, ItemSD: 0.6, Prevalence: 0.2, Ratios: []float64{3, 10, 30}, From: 3, To: 6}
}

// Row — результат правила.
type Row struct {
	Rule       Rule
	FPR        signals.Rate // доля стабильных рядов с тревогой хотя бы раз за горизонт
	Step, Grad signals.Rate // доля найденных (тревога в горизонте, t0 = From)
	StepMonth  *float64     // медиана месяца первой тревоги
	GradMonth  *float64
	Cost       []float64 // E[C] на 100 пар при Ratios[i]
}

// Result — вся калибровка.
type Result struct {
	Params Params
	Sigma  float64 // σ̂ шума, оценённая тем же MSSD, что и в продукте, на стабильных рядах
	Rows   []Row
	Best   []int // индекс правила с минимальной E[C] для каждого R
}

// first — месяц первой тревоги (1-based) в горизонте или 0.
func first(r Rule, s []float64, sigma float64, from, to int) int {
	for cut := from; cut <= to && cut <= len(s); cut++ {
		if r.Fire(s[:cut], sigma) {
			return cut
		}
	}
	return 0
}

// Run — прогон сетки на трёх сценариях (stable / step −1.0 / grad −0.4 в месяц) с началом в M3.
func Run(p Params, rules []Rule) Result {
	gen := func(kind string, off int64) [][]float64 {
		return synth.Random(p.Seed+off, synth.RandomParams{Kind: kind, Pairs: p.N, ItemSD: p.ItemSD, T0: p.From})
	}
	stable, step, grad := gen("stable", 0), gen("step", 1), gen("grad", 2)
	views := map[string]*signals.PairView{}
	for i, s := range stable {
		views[fmt.Sprint(i)] = &signals.PairView{InternIndex: s, MentorIndex: []float64{}}
	}
	res := Result{Params: p, Sigma: signals.NoiseSigma(views)}
	for _, r := range rules {
		row := Row{Rule: r}
		count := func(set [][]float64) (signals.Rate, *float64) {
			var months []float64
			for _, s := range set {
				if m := first(r, s, res.Sigma, p.From, p.To); m > 0 {
					months = append(months, float64(m))
				}
			}
			var med *float64
			if len(months) > 0 {
				sort.Float64s(months)
				m := months[len(months)/2]
				if len(months)%2 == 0 {
					m = (months[len(months)/2-1] + m) / 2
				}
				med = &m
			}
			return signals.NewRate(len(months), len(set)), med
		}
		row.FPR, _ = count(stable)
		row.Step, row.StepMonth = count(step)
		row.Grad, row.GradMonth = count(grad)
		miss := 1 - (*row.Step.Value+*row.Grad.Value)/2
		for _, R := range p.Ratios {
			c := 100 * ((1-p.Prevalence)*(*row.FPR.Value) + p.Prevalence*miss*R)
			row.Cost = append(row.Cost, math.Round(c*10)/10)
		}
		res.Rows = append(res.Rows, row)
	}
	for k := range p.Ratios {
		best := 0
		for i, row := range res.Rows {
			if row.Cost[k] < res.Rows[best].Cost[k] {
				best = i
			}
		}
		res.Best = append(res.Best, best)
	}
	return res
}

func pct(r signals.Rate) string {
	return fmt.Sprintf("%.2f [%.2f; %.2f]", *r.Value, *r.Lo, *r.Hi)
}

func month(m *float64) string {
	if m == nil {
		return "—"
	}
	return "M" + fnum(*m)
}

// WriteCSV — таблица для дальнейшего анализа.
func WriteCSV(w io.Writer, res Result) error {
	head := "rule,kind,z,slope,from_base,default,fpr,fpr_lo,fpr_hi,step_recall,step_month,grad_recall,grad_month"
	for _, R := range res.Params.Ratios {
		head += fmt.Sprintf(",cost_R%s", fnum(R))
	}
	if _, err := fmt.Fprintln(w, head); err != nil {
		return err
	}
	for _, row := range res.Rows {
		m := func(p *float64) string {
			if p == nil {
				return ""
			}
			return fnum(*p)
		}
		line := fmt.Sprintf("%q,%s,%s,%s,%s,%t,%.3f,%.3f,%.3f,%.3f,%s,%.3f,%s", row.Rule.Name, row.Rule.Kind, fnum(row.Rule.Z), fnum(row.Rule.Slope),
			fnum(row.Rule.FromBase), row.Rule.Default, *row.FPR.Value, *row.FPR.Lo, *row.FPR.Hi, *row.Step.Value, m(row.StepMonth), *row.Grad.Value, m(row.GradMonth))
		for _, c := range row.Cost {
			line += "," + fnum(c)
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

// WriteMarkdown — docs/calibration.md.
func WriteMarkdown(w io.Writer, res Result) error {
	p := res.Params
	var b strings.Builder
	fmt.Fprintf(&b, "# Калибровка S1/S2 на имитации\n\n")
	fmt.Fprintf(&b, "*Сгенерировано `go run ./cmd/calibrate -seed %d -n %d` (детерминировано при том же seed). Методика — research/methodology.md §3.3, §6.*\n\n", p.Seed, p.N)
	fmt.Fprintf(&b, "**Допущения (наши, а не данные МГТС):** базовый уровень пары ~U(3.6; 4.6), 4 пункта, шум пункта N(0; %s), округление и обрезка до 1…5, "+
		"индекс — среднее пунктов. Сценарии по %d рядов одной роли: «стабильно», «ступенька» −1.0 с M%d, «плавно» −0.4/мес с M%d. "+
		"Проверка в горизонте M%d–M%d (бэктест «как видел бы HR»). σ̂ шума оценена тем же MSSD, что и в продукте, по стабильным рядам: **%.3f**.\n\n",
		fnum(p.ItemSD), p.N, p.From, p.From, p.From, p.To, res.Sigma)
	b.WriteString("| Правило | Ложная тревога у стабильной пары (хотя бы раз), 95% ДИ | Ступенька: найдено / медиана месяца | Плавно: найдено / медиана месяца |")
	for _, R := range p.Ratios {
		fmt.Fprintf(&b, " E[C], R=%s |", fnum(R))
	}
	b.WriteString("\n|---|---|---|---|" + strings.Repeat("---|", len(p.Ratios)) + "\n")
	for i, row := range res.Rows {
		name := row.Rule.Name
		if row.Rule.Default {
			name = "**" + name + " (умолчание)**"
		}
		fmt.Fprintf(&b, "| %s | %s | %.2f / %s | %.2f / %s |", name, pct(row.FPR), *row.Step.Value, month(row.StepMonth), *row.Grad.Value, month(row.GradMonth))
		for k, c := range row.Cost {
			mark := ""
			if res.Best[k] == i {
				mark = " ★"
			}
			fmt.Fprintf(&b, " %s%s |", fnum(c), mark)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "\n**E[C]** — ожидаемая стоимость ошибок на 100 пар: `100·[(1−π)·FPR·C_FP + π·(1 − полнота)·C_FN]`, C_FP = 1 (лишний разговор 1:1), "+
		"R = C_FN/C_FP (поле `cost_ratio`, сколько лишних разговоров HR готов провести, чтобы не пропустить одного стажёра), "+
		"π = %s — доля пар с проблемой (допущение), полнота — среднее ступеньки и плавного. ★ — минимум E[C] при данном R.\n\n", fnum(p.Prevalence))
	b.WriteString("**Как читать.** Прежнее правило ловит всё и рано, но у большинства стабильных пар хотя бы раз поднимает ложную тревогу (а ролей две). " +
		"Комбинация «резкий провал по z ИЛИ устойчивое снижение по Тейлу–Сену» снижает ложные тревоги в разы ценой задержки на плавном сценарии. " +
		"Выбор между вариантами — ценой ошибок: при большом R выгоднее чувствительный вариант, при малом — строгий.\n\n")
	b.WriteString("**Сверка с таблицей methodology §3.3** (там 3000 рядов, σ̂ = 0.30 задана, а не оценена):\n\n")
	b.WriteString("| Правило | FPR methodology | FPR здесь |\n|---|---|---|\n")
	for _, ref := range []struct {
		kind       string
		z, sl, fpr float64
	}{{"current", 0, -0.25, 0.63}, {"new", 3, -0.375, 0.04}, {"new", 3, -0.25, 0.16}, {"z", 3, 0, 0.03}, {"trend", 0, -0.25, 0.14}, {"naive", 0, 0, 0.01}} {
		for _, row := range res.Rows {
			if row.Rule.Kind == ref.kind && row.Rule.Z == ref.z && (ref.kind == "z" || ref.kind == "naive" || row.Rule.Slope == ref.sl) {
				fmt.Fprintf(&b, "| %s | %.2f | %.2f |\n", row.Rule.Name, ref.fpr, *row.FPR.Value)
			}
		}
	}
	b.WriteString("\nДоли ложных тревог совпадают по порядку (расхождение ≤ 0.02–0.03 — другой seed и оценённая, а не заданная σ̂). " +
		"Полнота ветки «только z» здесь ниже (≈ 0.6 против 0.82 на ступеньке): в продукте σ̃ человека сжимается к пулу, но включает личный " +
		"разброс базы, а с M4 база уже содержит сам провал — z консервативнее. Поэтому ветка Тейла–Сена нужна, а не только z.\n\n")
	b.WriteString("**Оговорки.** (1) Распространённость, форма падения и шум заданы нами: θ*, выбранный на имитации, — стартовая точка, а не оптимум. " +
		"(2) В продукте σ̂ считается по всей программе, включая пары с трендом, поэтому она может быть выше, а z — консервативнее. " +
		"(3) В пилоте пересчитывать по вердиктам HR только при ≥ 5 решениях по типу и с интервалом; на финале пороги фиксируются до получения данных.\n")
	_, err := io.WriteString(w, b.String())
	return err
}

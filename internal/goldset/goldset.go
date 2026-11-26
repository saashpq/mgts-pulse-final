// Package goldset — оценка ИИ-разбора комментариев на размеченной вручную выборке (methodology §5, docs/GOLDSET.md):
// P/R/F1 по темам, macro-F1, полнота критичных тем с ДИ Клоппера–Пирсона, κ тональности, κ разметчиков
// и доля детерминированных повторов.
package goldset

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ElzaraA/mgts-ai/internal/signals"
	"github.com/ElzaraA/mgts-ai/internal/textai"
)

// Item — строка gold-set.
type Item struct {
	ID, Role, Text string
	Gold           []string // темы разметчика 1
	GoldSent       *int     // тональность −1 | 0 | 1; nil — строка не размечена
	Gold2          []string // темы разметчика 2
	HasGold2       bool     // разметчик 2 заполнил ячейку («-» — тем нет)
}

// Critical — темы, которые питают сигналы S7, S9, S10: по ним отдельно полнота с интервалом.
var Critical = []string{"routine", "no_growth", "overload", "unclear", "no_support", "conflict", "attrition"}

// ErrNoLabels — в файле нет размеченных строк (заполните gold_sentiment).
var ErrNoLabels = errors.New("нет размеченных строк: заполните gold_themes и gold_sentiment (docs/GOLDSET.md)")

func splitThemes(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return []string{}, nil
	}
	var out []string
	for _, t := range strings.Split(s, ";") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := textai.ThemeByCode(t); !ok {
			return nil, fmt.Errorf("тема %q не из словаря", t)
		}
		out = append(out, t)
	}
	return out, nil
}

func parseSent(s string) (*int, error) {
	var v int
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "":
		return nil, nil
	case "-1", "−", "-", "neg", "−1":
		v = -1
	case "0", "neu":
		v = 0
	case "1", "+1", "+", "pos":
		v = 1
	default:
		return nil, fmt.Errorf("тональность %q: ожидается -1, 0 или 1", s)
	}
	return &v, nil
}

// Read читает CSV с колонками id,role,text,gold_themes,gold_sentiment[,gold2_themes]; темы через «;».
func Read(r io.Reader) ([]Item, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("пустой файл")
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))] = i
	}
	for _, need := range []string{"id", "role", "text", "gold_themes", "gold_sentiment"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("нет колонки %s", need)
		}
	}
	get := func(row []string, name string) string {
		if i, ok := col[name]; ok && i < len(row) {
			return row[i]
		}
		return ""
	}
	var out []Item
	for n, row := range rows[1:] {
		it := Item{ID: get(row, "id"), Role: get(row, "role"), Text: get(row, "text")}
		var err error
		if it.Gold, err = splitThemes(get(row, "gold_themes")); err != nil {
			return nil, fmt.Errorf("строка %d: %w", n+2, err)
		}
		if it.GoldSent, err = parseSent(get(row, "gold_sentiment")); err != nil {
			return nil, fmt.Errorf("строка %d: %w", n+2, err)
		}
		if g2 := get(row, "gold2_themes"); strings.TrimSpace(g2) != "" {
			it.HasGold2 = true
			if it.Gold2, err = splitThemes(g2); err != nil {
				return nil, fmt.Errorf("строка %d: %w", n+2, err)
			}
		}
		out = append(out, it)
	}
	return out, nil
}

// ThemeScore — качество по теме.
type ThemeScore struct {
	Theme    string   `json:"theme"`
	Label    string   `json:"label"`
	Support  int      `json:"support"` // положительных в золотой разметке
	TP       int      `json:"tp"`
	FP       int      `json:"fp"`
	FN       int      `json:"fn"`
	P        *float64 `json:"precision"`
	R        *float64 `json:"recall"`
	F1       *float64 `json:"f1"`
	Critical bool     `json:"critical"`
	RecallLo *float64 `json:"recall_lo,omitempty"` // 95% ДИ Клоппера–Пирсона, только для критичных тем
	RecallHi *float64 `json:"recall_hi,omitempty"`
}

// AnalyzerResult — результат одного анализатора.
type AnalyzerResult struct {
	Name              string         `json:"name"`
	MacroF1           *float64       `json:"macro_f1"`
	MicroF1           *float64       `json:"micro_f1"`
	Themes            []ThemeScore   `json:"themes"`
	SentimentKappa    *float64       `json:"sentiment_kappa"`    // 3 класса, пороги −0.3 / +0.3
	SentimentAccuracy *float64       `json:"sentiment_accuracy"` // доля совпадений класса
	Deterministic     *float64       `json:"deterministic"`      // доля комментариев с одинаковым ответом во всех прогонах
	Runs              int            `json:"runs"`
	Rejected          int            `json:"rejected"` // ответ модели отклонён (первый прогон)
	Reasons           map[string]int `json:"reasons"`
}

// KappaRow — согласие двух разметчиков по теме.
type KappaRow struct {
	Theme string   `json:"theme"`
	Kappa *float64 `json:"kappa"`
	Agree float64  `json:"agree"` // доля совпадений
	Prev  float64  `json:"prevalence"`
	N     int      `json:"n"`
}

// Result — прогон gold-set; кладётся в DATA_DIR/goldset_result.json и показывается в quality.goldset.
type Result struct {
	Date           string           `json:"date"`
	Model          string           `json:"model"` // модель основного анализатора; offline — словарь
	PromptHash     string           `json:"prompt_hash"`
	N              int              `json:"n"`
	Labeled        int              `json:"labeled"`
	Primary        string           `json:"primary"` // чьи macro_f1 и deterministic наверху: модель, если была, иначе словарь
	MacroF1        *float64         `json:"macro_f1"`
	Deterministic  *float64         `json:"deterministic"`
	Analyzers      []AnalyzerResult `json:"analyzers"`
	AnnotatorKappa []KappaRow       `json:"annotator_kappa"` // null — второй разметки нет
}

func ratio(a, b int) *float64 {
	if b == 0 {
		return nil
	}
	v := round3(float64(a) / float64(b))
	return &v
}

func round3(f float64) float64 {
	return float64(int64(f*1000+0.5)) / 1000
}

func sentClass(x float64) int {
	switch {
	case x <= -0.3:
		return -1
	case x >= 0.3:
		return 1
	}
	return 0
}

func has(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func signature(a textai.Analysis) string {
	th := append([]string{}, a.Themes...)
	sort.Strings(th)
	return fmt.Sprintf("%s|%.2f|%s|%s", strings.Join(th, ","), a.Sentiment, a.Quote, a.Analyzer)
}

// Evaluate прогоняет анализаторы runs раз (≥ 1) по размеченным строкам. model — имя модели для отчёта.
func Evaluate(ctx context.Context, items []Item, analyzers []textai.Analyzer, runs int, now time.Time, model string) (Result, error) {
	var lab []Item
	for _, it := range items {
		if it.GoldSent != nil {
			lab = append(lab, it)
		}
	}
	if len(lab) == 0 {
		return Result{}, ErrNoLabels
	}
	if runs < 1 {
		runs = 1
	}
	res := Result{Date: now.UTC().Format("2006-01-02"), Model: model, PromptHash: textai.PromptHash(), N: len(items), Labeled: len(lab)}
	for _, an := range analyzers {
		ar := AnalyzerResult{Name: an.Name(), Runs: runs, Reasons: map[string]int{}}
		first := make([]textai.Analysis, len(lab))
		same := 0
		for i, it := range lab {
			sig := ""
			stable := true
			for r := 0; r < runs; r++ {
				a := an.Analyze(ctx, it.Text, it.Role)
				if r == 0 {
					first[i], sig = a, signature(a)
					if why := textai.Reason(a.Note); why != "" {
						ar.Rejected++
						ar.Reasons[why]++
					}
				} else if signature(a) != sig {
					stable = false
				}
			}
			if stable {
				same++
			}
		}
		ar.Deterministic = ratio(same, len(lab))
		var f1s []float64
		var tpS, fpS, fnS int
		for _, th := range textai.Themes {
			ts := ThemeScore{Theme: th.Code, Label: th.Label, Critical: has(Critical, th.Code)}
			for i, it := range lab {
				g, p := has(it.Gold, th.Code), has(first[i].Themes, th.Code)
				switch {
				case g && p:
					ts.TP++
				case p:
					ts.FP++
				case g:
					ts.FN++
				}
				if g {
					ts.Support++
				}
			}
			ts.P, ts.R = ratio(ts.TP, ts.TP+ts.FP), ratio(ts.TP, ts.TP+ts.FN)
			if ts.TP+ts.FP+ts.FN > 0 {
				f := round3(2 * float64(ts.TP) / float64(2*ts.TP+ts.FP+ts.FN))
				ts.F1 = &f
				f1s = append(f1s, f)
			}
			if ts.Critical && ts.Support > 0 {
				r := signals.NewRate(ts.TP, ts.Support)
				ts.RecallLo, ts.RecallHi = r.Lo, r.Hi
			}
			tpS, fpS, fnS = tpS+ts.TP, fpS+ts.FP, fnS+ts.FN
			ar.Themes = append(ar.Themes, ts)
		}
		if len(f1s) > 0 {
			sum := 0.0
			for _, f := range f1s {
				sum += f
			}
			m := round3(sum / float64(len(f1s)))
			ar.MacroF1 = &m
		}
		if tpS+fpS+fnS > 0 {
			m := round3(2 * float64(tpS) / float64(2*tpS+fpS+fnS))
			ar.MicroF1 = &m
		}
		var gold, pred []int
		for i, it := range lab {
			gold, pred = append(gold, *it.GoldSent), append(pred, sentClass(first[i].Sentiment))
		}
		ar.SentimentKappa, ar.SentimentAccuracy = multiKappa(gold, pred)
		res.Analyzers = append(res.Analyzers, ar)
	}
	if len(res.Analyzers) > 0 {
		p := res.Analyzers[len(res.Analyzers)-1]
		res.Primary, res.MacroF1, res.Deterministic = p.Name, p.MacroF1, p.Deterministic
		if !strings.HasPrefix(p.Name, "llm") {
			res.Model = "offline"
		}
	}
	res.AnnotatorKappa = annotators(lab)
	return res, nil
}

// multiKappa — κ Коэна для нескольких классов и доля совпадений.
func multiKappa(a, b []int) (*float64, *float64) {
	n := float64(len(a))
	if n == 0 {
		return nil, nil
	}
	ca, cb := map[int]float64{}, map[int]float64{}
	agree := 0.0
	for i := range a {
		ca[a[i]]++
		cb[b[i]]++
		if a[i] == b[i] {
			agree++
		}
	}
	po := agree / n
	pe := 0.0
	for k, v := range ca {
		pe += v / n * cb[k] / n
	}
	acc := round3(po)
	if pe >= 1-1e-12 {
		return nil, &acc
	}
	k := round3((po - pe) / (1 - pe))
	return &k, &acc
}

func annotators(lab []Item) []KappaRow {
	var both []Item
	for _, it := range lab {
		if it.HasGold2 {
			both = append(both, it)
		}
	}
	if len(both) == 0 {
		return nil
	}
	var out []KappaRow
	for _, th := range textai.Themes {
		var b, o1, o2, pos int
		for _, it := range both {
			g1, g2 := has(it.Gold, th.Code), has(it.Gold2, th.Code)
			switch {
			case g1 && g2:
				b++
			case g1:
				o1++
			case g2:
				o2++
			}
			if g1 {
				pos++
			}
		}
		n := len(both)
		out = append(out, KappaRow{Theme: th.Code, Kappa: signals.Kappa(b, o1, o2, n), Agree: round3(float64(n-o1-o2) / float64(n)),
			Prev: round3(float64(pos) / float64(n)), N: n})
	}
	return out
}

func fmtp(p *float64) string {
	if p == nil {
		return "—"
	}
	return strconv.FormatFloat(*p, 'f', 2, 64)
}

// WriteMarkdown — короткий отчёт прогона.
func WriteMarkdown(w io.Writer, r Result) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Gold-set: результат прогона %s\n\nРазмечено %d из %d комментариев; хэш промпта `%s`; основной анализатор — %s (модель: %s).\n\n",
		r.Date, r.Labeled, r.N, r.PromptHash, r.Primary, r.Model)
	for _, a := range r.Analyzers {
		fmt.Fprintf(&b, "## %s\n\nmacro-F1 %s, micro-F1 %s; тональность: κ %s, совпадение класса %s; одинаковый ответ в %d прогонах — %s; отклонено ответов модели — %d.\n\n",
			a.Name, fmtp(a.MacroF1), fmtp(a.MicroF1), fmtp(a.SentimentKappa), fmtp(a.SentimentAccuracy), a.Runs, fmtp(a.Deterministic), a.Rejected)
		b.WriteString("| Тема | Положит. | P | R | F1 | R 95% ДИ (критичные) |\n|---|---|---|---|---|---|\n")
		for _, t := range a.Themes {
			ci := ""
			if t.RecallLo != nil {
				ci = fmt.Sprintf("[%s; %s]", fmtp(t.RecallLo), fmtp(t.RecallHi))
			}
			fmt.Fprintf(&b, "| %s | %d | %s | %s | %s | %s |\n", t.Theme, t.Support, fmtp(t.P), fmtp(t.R), fmtp(t.F1), ci)
		}
		b.WriteString("\n")
	}
	if r.AnnotatorKappa != nil {
		b.WriteString("## Согласие разметчиков (κ по темам, порог ≥ 0.6)\n\n| Тема | κ | Совпадение | Распространённость | n |\n|---|---|---|---|---|\n")
		for _, k := range r.AnnotatorKappa {
			fmt.Fprintf(&b, "| %s | %s | %.2f | %.2f | %d |\n", k.Theme, fmtp(k.Kappa), k.Agree, k.Prev, k.N)
		}
	} else {
		b.WriteString("Второй разметки нет: κ разметчиков не посчитан.\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

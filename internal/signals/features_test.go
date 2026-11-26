package signals

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ElzaraA/mgts-ai/internal/survey"
	"github.com/ElzaraA/mgts-ai/internal/textai"
)

// onePair — программа из одной пары с заданными ответами стажёра (оценки и комментарий по месяцам).
func onePair(scores [][]float64, comments []string) Input {
	sv := survey.Default()
	sv.Participant = []survey.Pair{{ID: "PX", Intern: "I-X", Mentor: "M-X"}}
	var codes []string
	for _, q := range sv.ByRole(survey.Intern) {
		if q.Kind == "scale" {
			codes = append(codes, q.Code)
		}
	}
	in := Input{Survey: sv, Analyses: map[string]textai.Analysis{}, T: Default}
	for m, sc := range scores {
		if sc == nil {
			continue
		}
		r := survey.Response{ID: fmt.Sprintf("PX-intern-M%d", m+1), Role: survey.Intern, Person: "I-X", PairID: "PX", Period: m + 1,
			Scores: map[string]float64{}, Comments: map[string]string{}}
		for i, c := range codes {
			r.Scores[c] = sc[i%len(sc)]
		}
		if comments[m] != "" {
			r.Comments["i_comment"] = comments[m]
			in.Analyses[r.ID+"/i_comment"] = textai.Offline{}.Analyze(context.Background(), comments[m], "intern")
		}
		in.Responses = append(in.Responses, r)
	}
	return in
}

func TestFormalSurveyS13(t *testing.T) {
	long := "Разбираюсь с отчётами, наставник помогает и объясняет."
	cases := []struct {
		name     string
		scores   [][]float64
		comments []string
		want     bool
		first    int
	}{
		// R4 (B2): одинаковые шкалы с содержательным комментарием — не формальность, нужен признак невовлечённости
		{"flat two months with real comment", [][]float64{{4, 5, 4, 5}, {4}, {4}}, []string{long, long, long}, false, 0},
		{"flat two months with short comment", [][]float64{{4, 5, 4, 5}, {4}, {4}}, []string{long, long, "ок"}, true, 2},
		{"flat with shrunk comment", [][]float64{{4, 5, 4, 5}, {4, 5, 4, 4}, {4}}, []string{long + " " + long + " " + long, long + " " + long, "Всё нормально, работаю дальше."}, true, 3},
		{"all max with real comment is ceiling", [][]float64{{4, 5, 4, 5}, {5}, {5}}, []string{long, "Всё отлично.", long}, false, 0},
		{"flat once with short comment", [][]float64{{4, 5, 4, 5}, {4, 5, 4, 4}, {3}}, []string{long, long, "норм"}, true, 3},
		{"flat once without comment", [][]float64{{4, 5, 4, 5}, {4, 5, 4, 4}, {5}}, []string{long, long, ""}, true, 3},
		{"flat once with real comment", [][]float64{{4, 5, 4, 5}, {4, 5, 4, 4}, {5}}, []string{long, long, long}, false, 0},
		{"varied scales", [][]float64{{4, 5, 4, 5}, {4, 5, 4, 4}, {5, 4, 5, 5}}, []string{"ок", "ок", "ок"}, false, 0},
		{"old flat run is history", [][]float64{{4}, {4}, {4, 5, 4, 5}, {5, 4, 5, 5}}, []string{long, long, long, long}, false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := onePair(c.scores, c.comments)
			var got *Signal
			for _, s := range Detect(Build(in)["PX"], in, len(c.scores)) {
				if s.Type == "S13" {
					s := s
					got = &s
				}
			}
			if (got != nil) != c.want {
				t.Fatalf("S13 = %+v, want %v", got, c.want)
			}
			if got != nil && (got.FirstPeriod != c.first || got.Severity != Low || got.ID != "PX-S13-intern") {
				t.Fatalf("%+v", got)
			}
		})
	}
}

func TestPrecisionByType(t *testing.T) {
	in := synthInput()
	rep := Analyze(in, nil)
	ids := map[string][]string{} // тип → ID сигналов (активных и снятых)
	for _, p := range rep.Pairs {
		for _, s := range append(append([]SignalView{}, p.Signals...), p.Resolved...) {
			ids[s.Type] = append(ids[s.Type], s.ID)
		}
	}
	if len(ids["S3"]) < 2 || len(ids["S10"]) < 2 || len(ids["S8"]) < 2 {
		t.Fatalf("synthetic data must have S3, S8, S10 twice: %v", ids)
	}
	verdicts := map[string]Verdict{
		ids["S3"][0]: {Status: "false"}, ids["S3"][1]: {Status: "false"},
		ids["S10"][0]: {Status: "false"}, ids["S10"][1]: {Status: "false"},
		ids["S8"][0]: {Status: "confirmed"}, ids["S8"][1]: {Status: "false"},
		ids["S1"][0]: {Status: "resolved"},
	}
	byType := map[string]Precision{}
	for _, p := range Analyze(in, verdicts).Precision {
		byType[p.Type] = p
	}
	if len(byType) != len(Types) {
		t.Fatalf("every catalog type must be listed: %d", len(byType))
	}
	cases := []struct {
		typ       string
		precision float64 // -1 — null
		hint      string
		suggest   *Suggestion
	}{
		// 2 решения — меньше MinDecisions: точность видна, подсказки по порогу ещё нет
		{"S3", 0, "", nil},
		{"S10", 0, "", nil},
		{"S8", 0.5, "", nil}, // половина подтверждена — подсказки нет
		{"S1", 1, "", nil},
		{"S5", -1, "", nil},
	}
	for _, c := range cases {
		p := byType[c.typ]
		if c.precision < 0 {
			if p.Precision != nil {
				t.Errorf("%s: precision must be null without verdicts", c.typ)
			}
			continue
		}
		if p.Precision == nil || *p.Precision != c.precision {
			t.Errorf("%s: precision %v", c.typ, p.Precision)
		}
		if (c.hint == "") != (p.Hint == "") || !strings.Contains(p.Hint, c.hint) {
			t.Errorf("%s: hint %q", c.typ, p.Hint)
		}
		if (c.suggest == nil) != (p.Suggest == nil) || (c.suggest != nil && *c.suggest != *p.Suggest) {
			t.Errorf("%s: suggest %+v", c.typ, p.Suggest)
		}
	}
	if Analyze(Input{Survey: survey.Default(), T: Default}, nil).Precision == nil {
		t.Fatal("empty program must still list precision")
	}
	if p := byType["S3"]; p.Decisions != 2 || p.Enough {
		t.Fatalf("%+v", p)
	}
}

// Подсказка по порогу — только с MinDecisions решений по типу; итог по всем типам — precision_total.
func TestPrecisionEnough(t *testing.T) {
	mk := func(typ string, statuses ...string) []PairReport {
		var out []PairReport
		for i, st := range statuses {
			out = append(out, PairReport{Signals: []SignalView{{Signal: Signal{ID: fmt.Sprintf("P%d-%s", i, typ), Type: typ}, Verdict: &Verdict{Status: st}}}})
		}
		return out
	}
	cases := []struct {
		name    string
		pairs   []PairReport
		typ     string
		enough  bool
		hint    string
		suggest bool
	}{
		{"4 false — not enough", mk("S3", "false", "false", "false", "false"), "S3", false, "", false},
		{"5 decisions, mostly false", mk("S3", "false", "false", "false", "false", "confirmed"), "S3", true, "gap_min с 1.00 до 1.25", true},
		{"5 decisions, mostly right", mk("S3", "false", "false", "confirmed", "resolved", "confirmed"), "S3", true, "", false},
		{"no numeric threshold", mk("S10", "false", "false", "false", "false", "false"), "S10", true, "Числового порога у сигнала нет", false},
	}
	for _, c := range cases {
		ps := PrecisionByType(c.pairs, Default)
		var p Precision
		for _, x := range ps {
			if x.Type == c.typ {
				p = x
			}
		}
		if p.Enough != c.enough || (c.hint == "") != (p.Hint == "") || !strings.Contains(p.Hint, c.hint) || (p.Suggest != nil) != c.suggest {
			t.Errorf("%s: %+v", c.name, p)
		}
		tot := totalPrecision(ps)
		if tot.Decisions != len(c.pairs) || tot.Enough != (len(c.pairs) >= MinDecisions) || tot.Precision == nil {
			t.Errorf("%s: total %+v", c.name, tot)
		}
	}
	if tot := totalPrecision(PrecisionByType(nil, Default)); tot.Precision != nil || tot.Decisions != 0 || tot.Enough {
		t.Fatalf("%+v", tot)
	}
}

func TestHintBounds(t *testing.T) {
	cases := []struct {
		typ     string
		t       Thresholds
		to      float64
		suggest bool
	}{
		{"S1", Default, -0.35, true},
		{"S6", Default, 2.25, true},
		{"S12", Default, 0.15, true},
		{"S3", Thresholds{GapMin: 3.9}, 4, true},
		{"S8", Thresholds{MentorLow: 1}, 0, false}, // уже на границе
		{"S13", Default, 0, false},
	}
	for _, c := range cases {
		h, s := hint(c.typ, 3, c.t)
		if h == "" || (s != nil) != c.suggest || (s != nil && s.To != c.to) {
			t.Errorf("%s: %q %+v", c.typ, h, s)
		}
	}
}

func TestBrief(t *testing.T) {
	in := synthInput()
	rep := Analyze(in, nil)
	now := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		pair       string
		verdicts   map[string]Verdict
		minSignals int
		mustType   string
		noType     string
	}{
		{"P2", nil, 4, "S7", ""},
		{"P3", map[string]Verdict{"P3-S10": {Status: "false"}, "P3-S8": {Status: "resolved"}}, 2, "S1", "S10"},
		{"P1", nil, 0, "", ""},
	}
	for _, c := range cases {
		t.Run(c.pair, func(t *testing.T) {
			r := rep
			if c.verdicts != nil {
				r = Analyze(in, c.verdicts)
			}
			b := MakeBrief(pairOf(r, c.pair), now)
			if b.Pair != c.pair || b.GeneratedAt != "2026-11-18T10:00:00Z" || b.Disclaimer == "" || b.Intern == "" || b.Mentor == "" {
				t.Fatalf("%+v", b)
			}
			if len(b.Signals) < c.minSignals || (c.minSignals == 0 && len(b.Signals) != 0) {
				t.Fatalf("signals %d", len(b.Signals))
			}
			types := map[string]bool{}
			for _, s := range b.Signals {
				types[s.Type] = true
				if s.Quotes == nil {
					t.Fatal("quotes must be an array")
				}
			}
			if (c.mustType != "" && !types[c.mustType]) || (c.noType != "" && types[c.noType]) {
				t.Fatalf("types %v", types)
			}
			if len(b.Questions) < 3 || len(b.Questions) > 5 || !hasWith(b.Questions, "intern") || !hasWith(b.Questions, "mentor") {
				t.Fatalf("questions %+v", b.Questions)
			}
			seen := map[string]bool{}
			for _, q := range b.Questions {
				if seen[q.Text] || (q.With != "intern" && q.With != "mentor") {
					t.Fatalf("bad question %+v", q)
				}
				seen[q.Text] = true
			}
			if len(b.TalkingPoints) == 0 {
				t.Fatal("talking points")
			}
		})
	}
	// цитаты брифа — из оснований сигналов
	b := MakeBrief(pairOf(rep, "P2"), now)
	quotes := 0
	for _, s := range b.Signals {
		quotes += len(s.Quotes)
	}
	if quotes == 0 {
		t.Fatal("P2 brief must carry quotes")
	}
}

func TestPickQuestions(t *testing.T) {
	cases := []struct {
		types    []string
		min, max int
	}{
		{nil, 3, 3},
		{[]string{"S11"}, 3, 3},
		{[]string{"S2", "S8"}, 5, 5},
		{[]string{"S2"}, 3, 3},
		{[]string{"S10", "S1", "S3", "S7"}, 5, 5},
		{[]string{"unknown"}, 3, 3},
	}
	for _, c := range cases {
		qs := pickQuestions(c.types, 5)
		per := map[string]int{}
		for _, q := range qs {
			per[q.With]++
		}
		if per["intern"] > 3 || per["mentor"] > 3 {
			t.Errorf("%v: unbalanced %v", c.types, per)
		}
		if len(qs) < c.min || len(qs) > c.max || !hasWith(qs, "intern") || !hasWith(qs, "mentor") {
			t.Errorf("%v → %+v", c.types, qs)
		}
	}
	// у каждого шаблона есть вопросы обеим сторонам — бриф подходит для разговора и со стажёром, и с наставником
	for typ := range Types {
		qs := QuestionTemplates[typ]
		if !hasWith(qs, "intern") || !hasWith(qs, "mentor") {
			t.Errorf("%s: templates must cover intern and mentor", typ)
		}
	}
}

func TestThemesOverTime(t *testing.T) {
	in := synthInput()
	got := ThemesOverTime(in)
	byCode := map[string]ThemeTrend{}
	for _, tr := range got {
		byCode[tr.Code] = tr
		sum := 0
		for _, n := range tr.ByPeriod {
			sum += n
		}
		if sum == 0 || sum != tr.Total || len(tr.ByPeriod) != in.Survey.Periods || len(tr.Pairs) == 0 {
			t.Fatalf("%+v", tr)
		}
	}
	for i := 1; i < len(got); i++ {
		a, b := got[i-1], got[i]
		if (!a.Negative && b.Negative) || (a.Negative == b.Negative && a.Total < b.Total) {
			t.Fatalf("order: %s before %s", a.Code, b.Code)
		}
	}
	cases := []struct {
		code  string
		pairs string
		month int // месяц, где тема точно есть
	}{
		{"routine", "P2", 3},
		{"overload", "P5", 3},
		{"no_support", "P3", 2},
	}
	for _, c := range cases {
		tr, ok := byCode[c.code]
		if !ok || !strings.Contains(strings.Join(tr.Pairs, ","), c.pairs) || tr.ByPeriod[c.month-1] == 0 {
			t.Errorf("%s: %+v", c.code, tr)
		}
	}
	if len(ThemesOverTime(Input{Survey: survey.Default()})) != 0 {
		t.Fatal("no data — no themes")
	}
}

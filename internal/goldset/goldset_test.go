package goldset

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ElzaraA/mgts-ai/internal/textai"
)

const sample = "\ufeffid,role,text,gold_themes,gold_sentiment,gold2_themes\n" +
	"a,intern,\"Рутина, одно и то же, не вижу развития.\",routine;no_growth,-1,routine\n" +
	"b,intern,\"Конфликт с наставником, думаю уйти.\",conflict;attrition,neg,conflict;attrition\n" +
	"c,mentor,Сильный стажёр.,praise,1,-\n" +
	"d,intern,Наставник часто занят.,no_support,0,\n" +
	"e,intern,Без разметки,,,\n"

// flaky — анализатор, который на втором прогоне отвечает иначе (недетерминированная модель).
type flaky struct{ n *int }

func (f flaky) Name() string { return "llm:fake" }
func (f flaky) Analyze(_ context.Context, text, _ string) textai.Analysis {
	*f.n++
	a := textai.Analysis{Analyzer: "llm:fake", Themes: []string{"routine"}, Sentiment: -0.5}
	if strings.Contains(text, "Конфликт") && *f.n%2 == 0 {
		a.Themes = []string{"praise"}
	}
	if strings.Contains(text, "Сильный") {
		a.Note = textai.NoteRejectedPrefix + "не JSON"
	}
	return a
}

func TestReadAndEvaluate(t *testing.T) {
	items, err := Read(strings.NewReader(sample))
	if err != nil || len(items) != 5 || items[4].GoldSent != nil || !items[2].HasGold2 || len(items[2].Gold2) != 0 || items[3].HasGold2 {
		t.Fatalf("%+v %v", items, err)
	}
	n := 0
	res, err := Evaluate(context.Background(), items, []textai.Analyzer{textai.Offline{}, flaky{&n}}, 3, time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC), "fake")
	if err != nil {
		t.Fatal(err)
	}
	if res.Labeled != 4 || res.N != 5 || res.Date != "2026-11-19" || res.Primary != "llm:fake" || res.Model != "fake" || res.PromptHash == "" {
		t.Fatalf("%+v", res)
	}
	off, llm := res.Analyzers[0], res.Analyzers[1]
	if *off.Deterministic != 1 || off.MacroF1 == nil || *off.MacroF1 < 0.8 || off.Rejected != 0 {
		t.Fatalf("offline: %+v", off)
	}
	if *llm.Deterministic != 0.75 || llm.Rejected != 1 || llm.Reasons["не JSON"] != 1 || *res.Deterministic != 0.75 {
		t.Fatalf("llm: %+v", llm)
	}
	for _, th := range off.Themes {
		if th.Theme == "conflict" && (!th.Critical || th.RecallLo == nil || th.Support != 1) {
			t.Fatalf("%+v", th)
		}
		if th.Theme == "praise" && th.RecallLo != nil {
			t.Fatal("CI only for critical themes")
		}
	}
	if len(res.AnnotatorKappa) != len(textai.Themes) || res.AnnotatorKappa[0].N != 3 {
		t.Fatalf("%+v", res.AnnotatorKappa)
	}
	var md bytes.Buffer
	if err := WriteMarkdown(&md, res); err != nil || !strings.Contains(md.String(), "Согласие разметчиков") || !strings.Contains(md.String(), "llm:fake") {
		t.Fatal(md.String())
	}
	// только словарь, без второй разметки
	res2, _ := Evaluate(context.Background(), items[3:4], []textai.Analyzer{textai.Offline{}}, 0, time.Now(), "x")
	md.Reset()
	if res2.Model != "offline" || res2.AnnotatorKappa != nil || WriteMarkdown(&md, res2) != nil || !strings.Contains(md.String(), "Второй разметки нет") {
		t.Fatalf("%+v", res2)
	}
}

func TestReadErrors(t *testing.T) {
	for name, in := range map[string]string{
		"empty":      "",
		"no column":  "id,role,text\n",
		"bad theme":  "id,role,text,gold_themes,gold_sentiment\na,intern,x,burnout,0\n",
		"bad sent":   "id,role,text,gold_themes,gold_sentiment\na,intern,x,,очень\n",
		"bad gold2":  "id,role,text,gold_themes,gold_sentiment,gold2_themes\na,intern,x,,0,nope\n",
		"broken csv": "id,role\n\"a,b\n",
	} {
		if _, err := Read(strings.NewReader(in)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	items, _ := Read(strings.NewReader("id,role,text,gold_themes,gold_sentiment\na,intern,x,routine; ;,+\n"))
	if len(items[0].Gold) != 1 || *items[0].GoldSent != 1 {
		t.Fatalf("%+v", items)
	}
	if _, err := Evaluate(context.Background(), []Item{{Text: "x"}}, nil, 1, time.Now(), ""); !errors.Is(err, ErrNoLabels) {
		t.Fatal(err)
	}
}

// Шаблон: 60 синтетических комментариев, колонки разметки пустые.
func TestTemplate(t *testing.T) {
	f, err := os.Open("../../eval/goldset_template.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	items, err := Read(f)
	if err != nil || len(items) != 60 {
		t.Fatalf("%d %v", len(items), err)
	}
	for _, it := range items {
		if it.GoldSent != nil || len(it.Gold) != 0 || it.Text == "" {
			t.Fatalf("%+v", it)
		}
	}
}

func TestKappaHelpers(t *testing.T) {
	if k, acc := multiKappa(nil, nil); k != nil || acc != nil {
		t.Fatal("empty")
	}
	if k, acc := multiKappa([]int{1, 1}, []int{1, 1}); k != nil || *acc != 1 {
		t.Fatal("degenerate")
	}
	if k, _ := multiKappa([]int{1, 0, -1}, []int{1, 0, -1}); *k != 1 {
		t.Fatal("perfect")
	}
	if sentClass(-0.3) != -1 || sentClass(0.29) != 0 || sentClass(0.3) != 1 {
		t.Fatal("classes")
	}
	if fmtp(nil) != "—" {
		t.Fatal("fmt")
	}
}

type failW struct{}

func (failW) Write([]byte) (int, error) { return 0, errors.New("x") }

func TestWriteMarkdownError(t *testing.T) {
	if WriteMarkdown(failW{}, Result{}) == nil {
		t.Fatal("error must surface")
	}
}

package textai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/llm"
)

func TestOffline(t *testing.T) {
	cases := []struct {
		text  string
		theme string
		sign  int
	}{
		{"Рутина, одно и то же, не вижу развития.", "routine", -1},
		{"Наставник не отвечает в чате. Чувствую себя брошенным.", "no_support", -1},
		{"После разговора с HR нагрузку снизили, стало легче.", "praise", 1},
		{"Освоила Power Query, очень довольна.", "growth", 1},
	}
	for _, c := range cases {
		a := Offline{}.Analyze(context.Background(), c.text, "intern")
		if !contains(a.Themes, c.theme) || (c.sign < 0) != (a.Sentiment < 0) || !strings.Contains(c.text, a.Quote) || a.Quote == "" {
			t.Errorf("%q → %+v", c.text, a)
		}
	}
	if a := (Offline{}).Analyze(context.Background(), "После разговора с HR нагрузку снизили", "intern"); contains(a.Themes, "overload") {
		t.Error("relief must not be overload")
	}
	if a := (Offline{}).Analyze(context.Background(), "   ", "intern"); len(a.Themes) != 0 {
		t.Error("empty text")
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func TestValidate(t *testing.T) {
	text := "Задачи однотипные: каждый день одно и то же."
	ok := "```json\n{\"sentiment\": -0.7, \"themes\": [\"routine\"], \"quote\": \"каждый день одно и то же\"}\n```"
	if a, err := Validate(ok, text); err != nil || a.Themes[0] != "routine" {
		t.Fatalf("%+v %v", a, err)
	}
	bad := map[string]string{
		"not json":       "так себе",
		"unknown theme":  `{"sentiment":-1,"themes":["burnout"],"quote":"одно и то же"}`,
		"invented quote": `{"sentiment":-1,"themes":["routine"],"quote":"стажёр хочет уволиться"}`,
		"no quote":       `{"sentiment":-1,"themes":["routine"],"quote":""}`,
		"range":          `{"sentiment":-3,"themes":[],"quote":""}`,
	}
	for name, raw := range bad {
		if _, err := Validate(raw, text); err == nil {
			t.Errorf("%s: must be rejected", name)
		}
	}
}

type fake struct {
	out string
	err error
}

func (f fake) Complete(context.Context, []llm.Message) (string, error) { return f.out, f.err }

func TestLLMFallback(t *testing.T) {
	text := "Наставник не отвечает в чате."
	good := LLM{Client: fake{out: `{"sentiment":-0.8,"themes":["no_support"],"quote":"Наставник не отвечает в чате."}`}, Model: "m"}
	if a := good.Analyze(context.Background(), text, "intern"); a.Analyzer != "llm:m" || a.Sentiment != -0.8 {
		t.Fatalf("%+v", a)
	}
	hall := LLM{Client: fake{out: `{"sentiment":-1,"themes":["conflict"],"quote":"Наставник кричит"}`}, Model: "m"}
	if a := hall.Analyze(context.Background(), text, "intern"); a.Analyzer != "offline" || a.Note == "" {
		t.Fatalf("hallucination must fall back: %+v", a)
	}
	down := LLM{Client: fake{err: errors.New("timeout")}, Model: "m"}
	if a := down.Analyze(context.Background(), text, "mentor"); a.Analyzer != "offline" || !contains(a.Themes, "no_support") {
		t.Fatalf("outage must fall back: %+v", a)
	}
	if !strings.Contains(Prompt(), "no_support") || good.Name() != "llm:m" {
		t.Fatal("prompt must list themes")
	}
	if Words("Всё та же рутина, нового не учусь.") != 7 {
		t.Fatal("words")
	}
}

type capture struct {
	out  string
	sent *string
}

func (c capture) Complete(_ context.Context, msgs []llm.Message) (string, error) {
	*c.sent = msgs[1].Content
	return c.out, nil
}

// В модель уходит только маскированный текст; цитата с меткой заменяется офлайн-цитатой.
func TestLLMMasksPersonalData(t *testing.T) {
	text := "Наставник M-01 не отвечает. Пишу ему на test@example.invalid и звоню +7 900 000-00-00."
	names := func() []string { return []string{"I-01", "M-01"} }
	cases := []struct {
		name, out, quote, analyzer, note string
	}{
		{"phone tag replaced", `{"sentiment":-0.6,"themes":["no_support"],"quote":"звоню [телефон]"}`, "Наставник M-01 не отвечает.", "llm:m", ""},
		{"masked quote replaced", `{"sentiment":-0.6,"themes":["no_support"],"quote":"Наставник [ФИО] не отвечает."}`, "Наставник M-01 не отвечает.", "llm:m", ""},
		{"unrelated fallback quote", `{"sentiment":-0.6,"themes":["attrition"],"quote":"звоню [телефон]"}`, "Наставник M-01 не отвечает.", "offline", NoteMaskedQuote},
		// R4: цитата короче 3 слов отклоняется, поэтому дословный фрагмент — из трёх слов
		{"verbatim unmasked quote", `{"sentiment":-0.6,"themes":["no_support"],"quote":"Пишу ему на"}`, "Пишу ему на", "llm:m", ""},
		{"quote invented", `{"sentiment":-0.6,"themes":["no_support"],"quote":"M-01 не отвечает"}`, "Наставник M-01 не отвечает.", "offline", "ответ модели отклонён"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var sent string
			l := LLM{Client: capture{out: c.out, sent: &sent}, Model: "m", Names: names}
			a := l.Analyze(context.Background(), text, "intern")
			for _, leak := range []string{"M-01", "test@example.invalid", "900"} {
				if strings.Contains(sent, leak) {
					t.Fatalf("personal data %q sent to model: %q", leak, sent)
				}
			}
			if a.Quote != c.quote || a.Analyzer != c.analyzer || !strings.HasPrefix(a.Note, c.note) {
				t.Fatalf("%+v", a)
			}
			want := []string{"e-mail", "телефон", "ФИО"}
			if strings.Join(a.Masked, ",") != strings.Join(want, ",") {
				t.Fatalf("masked %v", a.Masked)
			}
		})
	}
	// офлайн-цитаты нет, а цитата модели с меткой — показываем словарь, а не скрытые данные
	var sent string
	l := LLM{Client: capture{out: `{"sentiment":0,"themes":[],"quote":"[e-mail]"}`, sent: &sent}, Model: "m"}
	if a := l.Analyze(context.Background(), "Мой адрес a@b.ru", "mentor"); a.Analyzer != "offline" || a.Quote != "" || a.Note == "" {
		t.Fatalf("%+v", a)
	}
}

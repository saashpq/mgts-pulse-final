package signals

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/textai"
)

func signalOfKind(list []Signal, kind string) *Signal {
	for i := range list {
		if list[i].Type == kind {
			return &list[i]
		}
	}
	return nil
}

func TestHRActionForDirectedFeedback(t *testing.T) {
	for _, tc := range []struct {
		role, text string
		want       bool
	}{
		{"intern", "Наставник не отвечает на вопросы.", true},
		{"mentor", "Стажёр срывает сроки сдачи.", true},
		{"intern", "Чат-бот не отвечает на вопросы.", false},
		{"intern", "Наставник помогает, когда чат-бот не отвечает на вопросы.", false},
		{"intern", "Когда чат-бот не отвечает на вопросы, наставник помогает.", false},
		{"intern", "Чат-бот помогает, но наставник не отвечает на вопросы.", true},
		{"intern", "Наставник отлично помогает.", false},
	} {
		in := synthInput()
		v := Build(in)["P1"]
		id := v.ResponseIDs[tc.role][0]
		code := v.CommentCodes[tc.role][0]
		a := (textai.Offline{}).Analyze(context.Background(), tc.text, tc.role)
		in.Analyses[id+"/"+code] = a
		v.Themes[tc.role][0] = a.Themes
		s := signalOfKind(Detect(v, in, 1), "S14")
		if (s != nil) != tc.want {
			t.Fatalf("%s: %v", tc.text, s)
		}
		if s != nil && (s.DueDays != 3 || Side(*s) != tc.role || s.Evidence[0].Quote != tc.text || s.LastPeriod != 1) {
			t.Fatal(s)
		}
	}
}

func TestHRTasksWithoutTextAndClosedMissingMonth(t *testing.T) {
	in := synthInput()
	for _, code := range []string{"i_interest", "i_clarity"} {
		v := Build(in)["P1"]
		v.Questions[code][0], v.Questions[code][1] = 5, 3
		s := signalOfKind(Detect(v, in, 2), "S15")
		if s == nil || s.LastPeriod != 2 || s.Evidence[0].Question != code || s.Evidence[1].Value != 3 {
			t.Fatalf("%s: %+v", code, s)
		}
	}
	v := Build(in)["P1"]
	v.InternIndex[0], v.InternIndex[1], v.InternIndex[2] = 4.5, 3.7, math.NaN()
	v.ResponseIDs["intern"][2] = ""
	in.WindowOpen = true
	in.WindowPeriod = 3
	if s := signalOfKind(Detect(v, in, 3), "S11"); s != nil {
		t.Fatal("premature missing response", s)
	}
	in.WindowOpen = false
	s := signalOfKind(Detect(v, in, 3), "S11")
	if s == nil || !strings.Contains(s.Action, "чек-ин") || s.LastPeriod != 3 || len(s.Evidence) != 2 {
		t.Fatal(s)
	}
}

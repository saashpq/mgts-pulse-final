package api

import (
	"context"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/llm"
)

type sampleReply string

func (reply sampleReply) Complete(context.Context, []llm.Message) (string, error) {
	return string(reply), nil
}

func TestModelMustRecognizeCheckSample(t *testing.T) {
	for _, tc := range []struct{ name, reply string }{
		{"empty", `{"sentiment":0,"themes":[],"quote":""}`},
		{"unrelated", `{"sentiment":-0.8,"themes":["deadlines"],"quote":"стоит ли продолжать"}`},
		{"dictionary supplied the missing risk", `{"sentiment":-0.8,"themes":["routine","no_support"],"quote":"наставник не отвечает в чате"}`},
		{"unrelated extra theme", `{"sentiment":-0.8,"themes":["attrition","routine","no_support","deadlines"],"quote":"стоит ли продолжать"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, h := newApp(t, sampleReply(tc.reply))
			var got llmCheck
			decode(t, do(h, "POST", "/api/v1/llm/check", ""), &got)
			if got.OK || got.Error == nil || got.Analysis == nil || got.Analysis.Analyzer != "llm:m" {
				t.Fatalf("expected an accepted response that failed the sample check, got %+v", got)
			}
		})
	}
}

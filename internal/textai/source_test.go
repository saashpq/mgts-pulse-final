package textai

import (
	"context"
	"strings"
	"testing"
)

func TestAnswerSourceAndFollowUp(t *testing.T) {
	source := Source{ResponseID: "P9-intern-M4", Question: "i_comment", Period: 4, Role: "intern"}
	ctx := WithSource(context.Background(), source)
	var sent string
	analysis := func(id string) Analysis {
		body := `{"sentiment":-0.8,"themes":["no_support"],"sentence_id":1,"source_id":"` + id + `","question_kind":"help"}`
		model := LLM{Client: capture{out: body, sent: &sent}, Model: "test"}
		return model.Analyze(ctx, "Наставник не отвечает в чате.", "intern")
	}
	ok := analysis(source.ResponseID)
	if ok.Source == nil || *ok.Source != source || ok.HRQuestion != followUps["help"] || ok.Quote == "" || !strings.Contains(sent, source.ResponseID) || !strings.Contains(sent, `"period":4`) {
		t.Fatalf("source or question lost: %+v", ok)
	}
	wrong := analysis("P8-intern-M4")
	if wrong.Analyzer != "offline" || !strings.Contains(wrong.Note, "другая анкета") {
		t.Fatalf("wrong source accepted: %+v", wrong)
	}
}

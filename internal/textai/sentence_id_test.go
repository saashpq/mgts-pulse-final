package textai

import "testing"

func TestIncompleteModelOutputIsNotANeutralVerdict(t *testing.T) {
	for _, raw := range []string{`{}`, `{"sentiment":0,"themes":null}`, `{"themes":[],"sentence_id":0}`} {
		if _, err := Validate(raw, "Спасибо за встречу."); err == nil {
			t.Errorf("accepted incomplete output %s", raw)
		}
	}
}

func TestLiteralEvidenceFromSentenceID(t *testing.T) {
	const input = "Задачи понятны. Я решил досрочно закончить стажировку."
	a, err := Validate(`{"sentiment":-0.5,"themes":["attrition"],"sentence_id":2}`, input)
	if err != nil || a.Quote != "Я решил досрочно закончить стажировку." {
		t.Fatalf("wrong evidence: %+v (%v)", a, err)
	}
	for _, raw := range []string{
		`{"sentiment":-0.5,"themes":["attrition"],"sentence_id":3}`,
		`{"sentiment":-0.5,"themes":["attrition"],"sentence_id":0}`,
		`{"sentiment":-0.5,"themes":["attrition"],"sentence_id":1,"quote":"досрочно закончить стажировку"}`,
	} {
		if _, err := Validate(raw, input); err == nil {
			t.Fatalf("accepted invalid evidence: %s", raw)
		}
	}
}

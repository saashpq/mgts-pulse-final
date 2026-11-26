package survey

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultAndFile(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatal(err)
	}
	// Профиль финала содержит 13 шкал и четыре открытых вопроса.
	if s, err := Load("../../data/survey.json"); err != nil || len(s.Questions) != 17 {
		t.Fatalf("data/survey.json: %v", err)
	}
	if _, err := Load("nope.json"); err == nil {
		t.Fatal("missing file")
	}
	bad := Default()
	bad.Questions[0].Mirror = "i_interest" // зеркало той же роли — ошибка
	b, _ := json.Marshal(bad)
	p := filepath.Join(t.TempDir(), "s.json")
	os.WriteFile(p, b, 0o644)
	if _, err := Load(p); err == nil {
		t.Fatal("bad mirror must fail")
	}
	q := Question{Max: 10}
	if v := q.Norm(10); v != 5 {
		t.Fatalf("1..10 → 1..5: %v", v)
	}
	if p, r, ok := Default().PairOf("m-03"); !ok || p.ID != "P3" || r != Mentor {
		t.Fatal("pair lookup")
	}
}

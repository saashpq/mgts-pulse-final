package store

import (
	"path/filepath"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/survey"
	"github.com/ElzaraA/mgts-ai/internal/synth"
	"github.com/ElzaraA/mgts-ai/internal/textai"
)

func TestLoadRefreshesOfflineAnalysis(t *testing.T) {
	sv := survey.Default()
	rs := synth.Responses(sv)[:2]
	for i := range rs {
		rs[i].Comments = map[string]string{"comment": "Нашёл конфликт зависимостей пакета."}
	}
	s := New(sv, filepath.Join(t.TempDir(), "state.json"))
	if err := s.Reset(sv, rs, "import"); err != nil {
		t.Fatal(err)
	}
	offlineKey, modelKey := rs[0].ID+"/comment", rs[1].ID+"/comment"
	old := textai.Analysis{Analyzer: "offline", OfflineVersion: 2, Themes: []string{"conflict"}, Note: "test fallback", Masked: []string{"телефон"}}
	model := textai.Analysis{Analyzer: "llm:m", Model: "m", Themes: []string{"quality"}}
	if err := s.SetAnalyses(map[string]textai.Analysis{offlineKey: old, modelKey: model}); err != nil {
		t.Fatal(err)
	}
	loaded := New(sv, s.file)
	if ok, err := loaded.Load(); err != nil || !ok {
		t.Fatalf("load: %t, %v", ok, err)
	}
	a, _ := loaded.Analysis(offlineKey)
	if len(a.Themes) != 0 || a.Note != old.Note || len(a.Masked) != 1 || a.OfflineVersion != textai.OfflineVersion {
		t.Fatalf("offline result after restart: %+v", a)
	}
	a, _ = loaded.Analysis(modelKey)
	if a.Analyzer != model.Analyzer || len(a.Themes) != 1 || a.Themes[0] != "quality" {
		t.Fatalf("model result changed: %+v", a)
	}
}

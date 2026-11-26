package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/signals"
	"github.com/ElzaraA/mgts-ai/internal/survey"
	"github.com/ElzaraA/mgts-ai/internal/synth"
	"github.com/ElzaraA/mgts-ai/internal/textai"
)

// Повреждённый снимок не роняет старт: файл переименован в .corrupt-<время>, состояние пустое.
func TestCorruptStateFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "state.json")
	if err := os.WriteFile(file, []byte(`{"responses": {`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, loaded, err := Open(survey.Default(), true, file)
	if err != nil || loaded || s == nil {
		t.Fatal(loaded, err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("corrupt file must be moved away")
	}
	matches, _ := filepath.Glob(file + ".corrupt-*")
	if len(matches) != 1 {
		t.Fatal(matches)
	}
	if b, _ := os.ReadFile(matches[0]); string(b) != `{"responses": {` {
		t.Fatal("content must be kept for manual inspection")
	}
}

// survey.json главнее снимка: вопросы, заголовок и число месяцев из файла, пары — объединение.
func TestSurveyFileWinsOverSnapshot(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	old := survey.Default()
	old.Participant = append(old.Participant, survey.Pair{ID: "P6", Intern: "I-06", Mentor: "M-06"}, // только в снимке — сохраняется
		survey.Pair{ID: "P7", Intern: "I-01", Mentor: "M-77"}) // участник уже есть в файле — файл главнее
	s := New(old, file)
	if err := s.Reset(old, synth.Responses(survey.Default()), "synthetic"); err != nil {
		t.Fatal(err)
	}
	tokenP6 := s.Tokens()["I-06"]

	fromFile := survey.Default()
	fromFile.Title = "Новая анкета"
	fromFile.Periods = 8
	fromFile.Questions[0].Text = "Изменённый вопрос"
	fromFile.Participant = append(fromFile.Participant[:2:2], survey.Pair{ID: "P9", Intern: "I-09", Mentor: "M-09"})
	s2, loaded, err := Open(fromFile, true, file)
	if err != nil || !loaded {
		t.Fatal(loaded, err)
	}
	sv := s2.Survey()
	if sv.Title != "Новая анкета" || sv.Periods != 8 || sv.Questions[0].Text != "Изменённый вопрос" {
		t.Fatalf("file must win: %+v", sv.Title)
	}
	ids := []string{}
	for _, p := range sv.Participant {
		ids = append(ids, p.ID)
	}
	if got := strings.Join(ids, ","); got != "P1,P2,P9,P3,P4,P5,P6" {
		t.Fatal(got)
	}
	if s2.Tokens()["I-06"] != tokenP6 || s2.Tokens()["I-09"] == "" {
		t.Fatal("links: old kept, new issued")
	}
	if len(s2.Responses()) != len(synth.Responses(survey.Default())) {
		t.Fatal("responses kept", len(s2.Responses()))
	}
	// survey.json не прочитан — снимок не трогаем
	s3, _, _ := Open(survey.Default(), false, file)
	if s3.Survey().Title != "Новая анкета" {
		t.Fatal("snapshot survey must be kept when file failed")
	}
	// снимка нет — просто структура из файла
	s4, loaded, err := Open(fromFile, true, filepath.Join(t.TempDir(), "none.json"))
	if err != nil || loaded || s4.Survey().Periods != 8 {
		t.Fatal(loaded, err)
	}
}

// Сбой записи откатывает изменения в памяти: вердикт, пороги, доставку, ответы, импорт и структуру.
func TestSaveFailureRollsBack(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	sv := survey.Default()
	s := New(sv, file)
	rs := synth.Responses(sv)
	if err := s.Reset(sv, rs, "synthetic"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetVerdict("P2-S7", "confirmed", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(file, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetVerdict("P2-S7", "false", ""); err == nil || s.Verdicts()["P2-S7"].Status != "confirmed" {
		t.Fatal("verdict change must be rolled back")
	}
	if _, err := s.SetVerdict("P2-S7", "", ""); err == nil || s.Verdicts()["P2-S7"].Status != "confirmed" {
		t.Fatal("verdict removal must be rolled back")
	}
	if _, err := s.SetVerdict("P3-S1", "false", ""); err == nil || len(s.Verdicts()) != 1 {
		t.Fatal("new verdict must be rolled back")
	}
	th := signals.Default
	th.GapMin = 2
	events := len(s.Events())
	if err := s.SetThresholds(th); err == nil || s.Thresholds().GapMin != signals.Default.GapMin || len(s.Events()) != events {
		t.Fatal("thresholds")
	}
	if fresh, err := s.ClaimDelivery("d"); err == nil || fresh {
		t.Fatal("claim")
	}
	if len(s.st.Deliveries) != 0 {
		t.Fatal("claim must be rolled back")
	}
	upd := rs[0]
	upd.Scores = map[string]float64{"i_overall": 1}
	added := survey.Response{ID: "P1-intern-M9", Role: survey.Intern, Person: "I-01", PairID: "P1", Period: 9}
	if _, err := s.Upsert([]survey.Response{upd, added}, "form", ""); err == nil {
		t.Fatal("upsert must fail")
	}
	if got := s.Responses(); len(got) != len(rs) || got[0].Scores["i_overall"] == 1 {
		t.Fatal("upsert must be rolled back")
	}
	nsv := s.Survey()
	nsv.Participant = append(nsv.Participant, survey.Pair{ID: "P6", Intern: "I-06", Mentor: "M-06"})
	if _, err := s.ApplyImport(&nsv, []survey.Response{added}, "x"); err == nil || len(s.Survey().Participant) != 5 || len(s.Tokens()) != 10 {
		t.Fatal("import must be rolled back")
	}
	if err := s.SetSurvey(nsv); err == nil || len(s.Survey().Participant) != 5 {
		t.Fatal("survey must be rolled back")
	}
	if err := s.MergeSurvey(nsv); err == nil || len(s.Survey().Participant) != 5 {
		t.Fatal("merge must be rolled back")
	}
	if err := s.SetAnalysis("k", textai.Analysis{}); err == nil {
		t.Fatal("analysis save error must be reported")
	}
	s.Log("hr", "x")             // ошибка только в лог, без паники
	s.ReleaseDelivery("missing") // нечего снимать
	if err := os.RemoveAll(file); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyImport(&nsv, []survey.Response{added}, "x"); err != nil || len(s.Survey().Participant) != 6 {
		t.Fatal(err)
	}
	if _, err := s.ApplyImport(nil, nil, ""); err != nil {
		t.Fatal(err)
	}
}

func TestAnalysesBatchAndPending(t *testing.T) {
	sv := survey.Default()
	s := New(sv, "")
	if err := s.Reset(sv, synth.Responses(sv), "synthetic"); err != nil {
		t.Fatal(err)
	}
	if len(s.Pending("wait")) != 0 {
		t.Fatal("no analyses yet")
	}
	if err := s.SetAnalyses(map[string]textai.Analysis{"P1-intern-M1/i_comment": {Note: "wait"}, "P1-mentor-M1/m_comment": {Note: "done"}}); err != nil {
		t.Fatal(err)
	}
	p := s.Pending("wait")
	if len(p) != 1 || p[0].Key != "P1-intern-M1/i_comment" || p[0].Role != "intern" || p[0].Text == "" {
		t.Fatalf("%+v", p)
	}
	if err := s.SetAnalyses(nil); err != nil {
		t.Fatal(err)
	}
}

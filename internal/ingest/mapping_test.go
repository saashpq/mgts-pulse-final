package ingest

import (
	"strings"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/survey"
)

// foreignCSV — выгрузка с «чужими» заголовками: автоматически находятся только часть вопросов и месяц.
const foreignCSV = "Код практиканта;Номер месяца;Оценка стажировки;Интерес;Ясность задач;Поддержка;Отзыв\n" +
	"I-01;1;4;5;4;5;Всё понятно, интересно\n" +
	"I-02;1;3;2;3;2;Задачи однотипные\n" +
	";;;;;;\n"

func targetOf(c ColumnPreview) string {
	if c.Target == nil {
		return ""
	}
	return *c.Target
}

func TestPreview(t *testing.T) {
	sv := survey.Default()
	tables, _ := ReadFile("чужой.csv", []byte(foreignCSV))
	pv := Preview(tables, sv, "")
	if len(pv) != 1 || pv[0].Name != "чужой.csv" || pv[0].Role != "intern" || pv[0].Rows != 2 || len(pv[0].Columns) != 7 {
		t.Fatalf("%+v", pv)
	}
	want := []string{"", "period", "i_overall", "i_interest", "", "i_support", ""}
	for i, c := range pv[0].Columns {
		if c.Index != i || targetOf(c) != want[i] {
			t.Fatalf("col %d: %q → %q, want %q", i, c.Title, targetOf(c), want[i])
		}
	}
	if s := pv[0].Columns[0].Samples; len(s) != 2 || s[0] != "I-01" {
		t.Fatalf("samples %v", s)
	}
	// длинные значения обрезаны, не больше трёх примеров, пустой лист и колонка без заголовка с данными
	long := strings.Repeat("я", 100)
	tables, _ = ReadFile("x.csv", []byte("ID стажёра,Месяц,\nI-01,1,"+long+"\nI-01,2,b\nI-01,3,c\nI-01,4,d\n"))
	pv = Preview(append(tables, Table{Name: "пусто"}), sv, survey.Mentor)
	if c := pv[0].Columns[2]; c.Title != "" || len(c.Samples) != 3 || len([]rune(c.Samples[0])) != 81 {
		t.Fatalf("%+v", c)
	}
	if pv[0].Role != "" || targetOf(pv[0].Columns[0]) != "intern_id" || pv[1].Rows != 0 || len(pv[1].Columns) != 0 {
		t.Fatalf("%+v", pv)
	}
	tg := Targets(sv)
	if len(tg) != len(sv.Questions)+4 || tg[len(tg)-1].Value != "skip" || tg[0].Role != "intern" || tg[len(tg)-2].Label != "Месяц" {
		t.Fatalf("%+v", tg)
	}
}

func TestMappingValidation(t *testing.T) {
	sv := survey.Default()
	bad := []string{
		`{`,
		`{"a":{"role":"boss","columns":{}}}`,
		`{"a":{"role":"intern","columns":{"x":"i_overall"}}}`,
		`{"a":{"role":"intern","columns":{"-1":"i_overall"}}}`,
		`{"a":{"role":"intern","columns":{"0":"nope"}}}`,
		`{"a":{"role":"intern","columns":{"0":"m_overall"}}}`,
		`{"a":{"role":"intern","columns":{"0":"period","1":"period"}}}`,
	}
	for _, raw := range bad {
		if _, err := ParseMapping(raw, sv); err == nil {
			t.Fatalf("must reject %s", raw)
		}
	}
	m, err := ParseMapping(`{"a":{"role":"mentor","columns":{"0":"skip","1":"skip","2":"","3":"m_speed","4":"mentor_id"}}}`, sv)
	if err != nil || m["a"].Role != survey.Mentor {
		t.Fatal(err)
	}
	if m, err := ParseMapping("  ", sv); m != nil || err != nil {
		t.Fatal("empty mapping")
	}
}

func TestImportWithMappingAndRemember(t *testing.T) {
	sv := survey.Default()
	tables, _ := ReadFile("чужой.csv", []byte(foreignCSV))
	if rep := Import(tables, &sv, "", "import"); rep.Accepted != 0 || !strings.Contains(rep.Sheets[0].Errors[0], "ID участника") {
		t.Fatalf("auto import must fail without ID column: %+v", rep)
	}
	raw := `{"чужой.csv":{"role":"intern","columns":{"0":"intern_id","1":"period","2":"i_overall","3":"i_interest","4":"i_clarity","5":"i_support","6":"i_comment"}},
		"другой лист":{"role":"mentor","columns":{}}}`
	m, err := ParseMapping(raw, sv)
	if err != nil {
		t.Fatal(err)
	}
	stored := survey.Default()
	sv = stored
	sv.Participant = append([]survey.Pair(nil), stored.Participant...)
	if n := Remember(tables, &sv, m); n != 4 {
		// Код практиканта, Ясность задач, Отзыв + «Оценка стажировки» (совпадала только по словам вопроса)
		t.Fatalf("remembered %d", n)
	}
	if n := Remember(tables, &sv, m); n != 0 {
		t.Fatalf("second remember must add nothing: %d", n)
	}
	if q, _ := stored.Q("i_clarity"); len(q.Aliases) != 2 {
		t.Fatalf("source survey must not change: %v", q.Aliases)
	}
	if q, _ := sv.Q("i_clarity"); q.Aliases[len(q.Aliases)-1] != "Ясность задач" {
		t.Fatalf("%v", q.Aliases)
	}
	rep := ImportMapped(tables, &sv, "", m, "import")
	if rep.Accepted != 2 || rep.Responses[1].Scores["i_clarity"] != 3 || rep.Responses[1].Comments["i_comment"] != "Задачи однотипные" {
		t.Fatalf("%+v", rep)
	}
	// после запоминания тот же файл сопоставляется автоматически
	sv2 := sv
	if rep := Import(tables, &sv2, "", "import"); rep.Accepted != 2 || len(rep.Sheets[0].Unmapped) != 0 {
		t.Fatalf("%+v", rep)
	}
	// служебные колонки наставника и skip; колонка вне заголовка не запоминается
	mt, _ := ReadFile("m.csv", []byte("Куратор;Период оценки;Скорость;Мусор\nM-02;2;4;x\n"))
	mm := Mapping{"m.csv": {Role: survey.Mentor, Columns: map[string]string{"0": "mentor_id", "1": "period", "2": "m_speed", "3": "skip", "9": "m_autonomy"}}}
	if n := Remember(append(mt, Table{Name: "m.csv"}), &sv, mm); n != 3 {
		t.Fatalf("remembered %d", n)
	}
	if rep := ImportMapped(mt, &sv, "", mm, "import"); rep.Accepted != 1 || rep.Responses[0].PairID != "P2" || len(rep.Sheets[0].Unmapped) != 1 {
		t.Fatalf("%+v", rep)
	}
	// ручное сопоставление без месяца — понятная ошибка листа
	nm := Mapping{"m.csv": {Role: survey.Mentor, Columns: map[string]string{"0": "mentor_id"}}}
	if rep := ImportMapped(mt, &sv, "", nm, "import"); rep.Accepted != 0 || len(rep.Sheets[0].Errors) != 1 {
		t.Fatalf("%+v", rep)
	}
}

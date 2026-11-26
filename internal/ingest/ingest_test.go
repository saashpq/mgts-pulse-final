package ingest

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/survey"
)

func TestCSVImport(t *testing.T) {
	csv := "\xef\xbb\xbfID стажёра;ID наставника;Месяц;Как в целом проходит стажировка? (1–5);Насколько интересны задачи?;Понятны ли задачи;Поддержка наставника;Комментарий стажёра;Лишняя колонка\n" +
		"I-01;M-01;Месяц 1;4;5;4;5;Всё нравится;x\n" +
		"I-01;M-01;Месяц 2;7;5;4;5;;x\n" +
		"I-09;M-09;3;4;4;4;4;Новая пара;x\n" +
		"I-01;;2;4;4;4;4;;x\n"
	sv := survey.Default()
	tables, err := ReadFile("a.csv", []byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	rep := Import(tables, &sv, "", "import")
	sh := rep.Sheets[0]
	if sh.Role != "intern" || len(sh.Mapped) != 5 || len(sh.Unmapped) != 1 {
		t.Fatalf("mapping: %+v", sh)
	}
	if rep.Accepted != 3 || rep.Rejected != 1 || !strings.Contains(sh.Errors[0], "вне шкалы") {
		t.Fatalf("rows: %+v", rep)
	}
	if len(rep.NewPairs) != 1 || len(sv.Participant) != 6 {
		t.Fatalf("new pair: %+v", rep.NewPairs)
	}
	if r := rep.Responses[0]; r.ID != "P1-intern-M1" || r.Scores["i_interest"] != 5 || r.Comments["i_comment"] != "Всё нравится" {
		t.Fatalf("%+v", r)
	}
}

func TestDatesAndRoleHint(t *testing.T) {
	csv := "mentor_id,period,m_overall,m_speed\nM-02,2026-03-01,4,4\nM-02,2026-02-15,5,5\n"
	sv := survey.Default()
	tables, _ := ReadFile("m.csv", []byte(csv))
	rep := Import(tables, &sv, survey.Mentor, "import")
	if rep.Accepted != 2 || rep.Responses[0].Period != 2 || rep.Responses[1].Period != 1 {
		t.Fatalf("%+v", rep.Responses)
	}
	if rep := Import(tables, &sv, "", "import"); rep.Sheets[0].Role != "mentor" {
		t.Fatal("role by headers")
	}
	bad, _ := ReadFile("x.csv", []byte("a,b\n1,2\n"))
	if rep := Import(bad, &sv, "", "import"); rep.Accepted != 0 || len(rep.Sheets[0].Errors) == 0 {
		t.Fatalf("%+v", rep)
	}
}

// minimalXLSX — книга с одним листом: общие строки, inline-строки и числа.
func minimalXLSX(rows [][]string) []byte {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w := func(name, body string) {
		f, _ := z.Create(name)
		f.Write([]byte(body))
	}
	w("xl/workbook.xml", `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Ответы" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	w("xl/_rels/workbook.xml.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`)
	var sst []string
	var sheet strings.Builder
	sheet.WriteString(`<worksheet><sheetData>`)
	for i, r := range rows {
		fmt.Fprintf(&sheet, `<row r="%d">`, i+1)
		for j, c := range r {
			ref := fmt.Sprintf("%c%d", 'A'+j, i+1)
			if i == 0 {
				sst = append(sst, c)
				fmt.Fprintf(&sheet, `<c r="%s" t="s"><v>%d</v></c>`, ref, len(sst)-1)
			} else if j < 2 {
				fmt.Fprintf(&sheet, `<c r="%s" t="inlineStr"><is><t>%s</t></is></c>`, ref, c)
			} else {
				fmt.Fprintf(&sheet, `<c r="%s"><v>%s</v></c>`, ref, c)
			}
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	w("xl/worksheets/sheet1.xml", sheet.String())
	var s strings.Builder
	s.WriteString(`<sst>`)
	for _, x := range sst {
		fmt.Fprintf(&s, `<si><t>%s</t></si>`, x)
	}
	s.WriteString(`</sst>`)
	w("xl/sharedStrings.xml", s.String())
	z.Close()
	return buf.Bytes()
}

func TestXLSX(t *testing.T) {
	data := minimalXLSX([][]string{
		{"ID стажёра", "ID наставника", "Месяц", "Оперативность", "Самостоятельность", "Как справляется стажёр"},
		{"I-03", "M-03", "1", "4", "3", "4"},
	})
	tables, err := ReadFile("m.xlsx", data)
	if err != nil || len(tables) != 1 || tables[0].Name != "Ответы" {
		t.Fatalf("%v %+v", err, tables)
	}
	sv := survey.Default()
	rep := Import(tables, &sv, "", "import")
	if rep.Accepted != 1 || rep.Responses[0].Role != survey.Mentor || rep.Responses[0].Scores["m_autonomy"] != 3 {
		t.Fatalf("%+v", rep)
	}
	if _, err := ReadFile("bad.xlsx", []byte("PKnot a zip")); err == nil {
		t.Fatal("broken xlsx")
	}
	if colIndex("AB7") != 27 {
		t.Fatal("colIndex")
	}
}

// rawXLSX — книга из готовых частей: имя файла в архиве → содержимое.
func rawXLSX(parts map[string]string) []byte {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, body := range parts {
		f, _ := z.Create(name)
		f.Write([]byte(body))
	}
	z.Close()
	return buf.Bytes()
}

const wbOne = `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="A" r:id="rId1"/></sheets></workbook>`
const relsOne = `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`

func TestXLSXLimits(t *testing.T) {
	sheet := func(body string) []byte {
		return rawXLSX(map[string]string{"xl/workbook.xml": wbOne, "xl/_rels/workbook.xml.rels": relsOne,
			"xl/worksheets/sheet1.xml": `<worksheet><sheetData>` + body + `</sheetData></worksheet>`})
	}
	var many strings.Builder
	many.WriteString(`<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>`)
	for i := 0; i < MaxSheets+1; i++ {
		fmt.Fprintf(&many, `<sheet name="S%d" r:id="r%d"/>`, i, i)
	}
	many.WriteString(`</sheets></workbook>`)
	var cells strings.Builder
	for i := 0; i < MaxCells/100+1; i++ {
		cells.WriteString(`<row>` + strings.Repeat(`<c><v>1</v></c>`, 100) + `</row>`)
	}
	cases := map[string][]byte{
		// 40 МБ пробелов сжимаются в десятки КБ: общий бюджет распаковки должен остановить чтение
		"bomb":           sheet(`<row><c><v>1</v></c></row>` + strings.Repeat(" ", 40<<20)),
		"sheets":         rawXLSX(map[string]string{"xl/workbook.xml": many.String()}),
		"cells":          sheet(cells.String()),
		"column":         sheet(`<row r="1"><c r="XFE1"><v>1</v></c></row>`),
		"column_letters": sheet(`<row r="1"><c r="AAAAAAAAAAAAAAAA1"><v>1</v></c></row>`),
		"row_number":     sheet(`<row r="99999999"><c><v>1</v></c></row>`),
		"cell_row":       sheet(`<row><c r="A0"><v>1</v></c></row>`),
		"broken_xml":     sheet(`<row><c><v>1</v></row>`),
		"broken_sst": rawXLSX(map[string]string{"xl/workbook.xml": wbOne, "xl/_rels/workbook.xml.rels": relsOne,
			"xl/sharedStrings.xml": `<sst><si><t>a</si>`, "xl/worksheets/sheet1.xml": `<worksheet><sheetData/></worksheet>`}),
		"broken_rels": rawXLSX(map[string]string{"xl/workbook.xml": wbOne, "xl/_rels/workbook.xml.rels": `<Relationships><`,
			"xl/worksheets/sheet1.xml": `<worksheet><sheetData/></worksheet>`}),
	}
	for name, data := range cases {
		if _, err := ReadFile("x.xlsx", data); !errors.Is(err, ErrTooLarge) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// без sharedStrings и rels книга читается: лист по умолчанию sheet1.xml
	ok := rawXLSX(map[string]string{"xl/workbook.xml": `<workbook><sheets><sheet name="A"/></sheets></workbook>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="b1" t="inlineStr"><is><t>x</t></is></c></row></sheetData></worksheet>`})
	tables, err := ReadFile("ok.xlsx", ok)
	if err != nil || len(tables) != 1 || len(tables[0].Rows[0]) != 2 || tables[0].Rows[0][1] != "x" {
		t.Fatalf("%v %+v", err, tables)
	}
	if _, err := ReadFile("x.xlsx", rawXLSX(map[string]string{"xl/workbook.xml": wbOne})); err == nil {
		t.Fatal("missing sheet")
	}
}

func TestColIndex(t *testing.T) {
	for ref, want := range map[string]int{"A1": 0, "a1": 0, "ab7": 27, "XFD1048576": 16383, "XFE1": 16384, "AAAA1": -1, "1": -1, "": -1, "A": 0, "A0": -1, "A1048577": -1, "A1x": -1} {
		if got := colIndex(ref); got != want {
			t.Errorf("%s: %d want %d", ref, got, want)
		}
	}
}

func TestExcelDateStyleAndEpoch(t *testing.T) {
	const sheet = `<worksheet><sheetData><row><c s="1"><v>46266.5</v></c><c s="2"><v>46266</v></c><c><v>46266</v></c></row></sheetData></worksheet>`
	const styles = `<styleSheet><numFmts><numFmt numFmtId="164" formatCode="yyyy-mm"/></numFmts><cellXfs><xf numFmtId="0"/><xf numFmtId="14"/><xf numFmtId="164"/></cellXfs></styleSheet>`
	for _, tc := range []struct{ flag, want string }{{"0", "2026-09-01"}, {"1", "2030-09-02"}} {
		workbook := strings.Replace(wbOne, "<sheets>", `<workbookPr date1904="`+tc.flag+`"/><sheets>`, 1)
		data := rawXLSX(map[string]string{"xl/workbook.xml": workbook, "xl/styles.xml": styles, "xl/worksheets/sheet1.xml": sheet})
		tables, err := ReadFile("date.xlsx", data)
		if err != nil {
			t.Fatal(err)
		}
		row := tables[0].Rows[0]
		if row[0] != tc.want || row[1] != tc.want || row[2] != "46266" {
			t.Fatalf("epoch %s: %v", tc.flag, row)
		}
	}
}

// ID новой пары — max(Pn)+1, а не len+1: при «дырявой» нумерации len+1 совпал бы с существующей парой.
func TestNextPairID(t *testing.T) {
	sv := survey.Default()
	sv.Participant = []survey.Pair{{ID: "P1"}, {ID: "P3"}, {ID: "p10"}, {ID: "Team"}, {ID: "P11x"}}
	if got := nextPairID(&sv); got != "P11" {
		t.Fatal(got)
	}
	sv.Participant = []survey.Pair{{ID: "P2"}, {ID: "P3"}}
	if got := nextPairID(&sv); got != "P4" {
		t.Fatal(got)
	}
	sv.Participant = nil
	if got := nextPairID(&sv); got != "P1" {
		t.Fatal(got)
	}
	// импорт с новой парой при занятом P3
	sv = survey.Default()
	sv.Participant = sv.Participant[2:] // P3, P4, P5
	tables, _ := ReadFile("x.csv", []byte("ID стажёра;ID наставника;Месяц;Как проходит стажировка\nI-77;M-77;1;4\n"))
	rep := Import(tables, &sv, "", "import")
	if len(rep.NewPairs) != 1 || rep.NewPairs[0] != "P6" {
		t.Fatalf("%+v", rep.NewPairs)
	}
}

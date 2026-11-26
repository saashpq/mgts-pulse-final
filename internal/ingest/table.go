// Package ingest читает выгрузки анкет (CSV и XLSX) и превращает строки «участник × месяц» в ответы.
// XLSX разбирается стандартной библиотекой (zip + xml): у проекта нет внешних зависимостей,
// он собирается на ноутбуке финала без доступа к интернету.
package ingest

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Table — лист таблицы: первая непустая строка — заголовки.
type Table struct {
	Name string
	Rows [][]string
}

// ReadFile определяет формат по имени и содержимому.
func ReadFile(name string, data []byte) ([]Table, error) {
	if bytes.HasPrefix(data, []byte("PK")) || strings.HasSuffix(strings.ToLower(name), ".xlsx") {
		return readXLSX(data)
	}
	t, err := readCSV(data)
	if err != nil {
		return nil, err
	}
	t.Name = name
	return []Table{t}, nil
}

func readCSV(data []byte) (Table, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	first := string(data)
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	// Excel в русской локали сохраняет CSV через «;»
	delim := ','
	if strings.Count(first, ";") > strings.Count(first, ",") {
		delim = ';'
	} else if strings.Count(first, "\t") > strings.Count(first, ",") {
		delim = '\t'
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = delim
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	rows, err := r.ReadAll()
	if err != nil {
		return Table{}, fmt.Errorf("csv: %w", err)
	}
	return Table{Rows: rows}, nil
}

type xlsxWorkbook struct {
	Properties struct {
		Date1904 bool `xml:"date1904,attr"`
	} `xml:"workbookPr"`
	Sheets []struct {
		Name string `xml:"name,attr"`
		RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
	} `xml:"sheets>sheet"`
}

type xlsxRels struct {
	Rels []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

type xlsxSST struct {
	SI []struct {
		T string `xml:"t"`
		R []struct {
			T string `xml:"t"`
		} `xml:"r"`
	} `xml:"si"`
}

type xlsxCell struct {
	R     string `xml:"r,attr"`
	Style int    `xml:"s,attr"`
	T     string `xml:"t,attr"`
	V     string `xml:"v"`
	IS    struct {
		T string `xml:"t"`
		R []struct {
			T string `xml:"t"`
		} `xml:"r"`
	} `xml:"is"`
}

type xlsxStyles struct {
	Formats []struct {
		ID   int    `xml:"numFmtId,attr"`
		Code string `xml:"formatCode,attr"`
	} `xml:"numFmts>numFmt"`
	Styles []struct {
		Format int `xml:"numFmtId,attr"`
	} `xml:"cellXfs>xf"`
}

var formatLiteral = regexp.MustCompile(`"[^"]*"|\[[^\]]*\]|\\.|_.|\*.`)

func (s xlsxStyles) calendarStyles() []bool {
	custom := map[int]bool{}
	for _, f := range s.Formats {
		code := strings.ToLower(formatLiteral.ReplaceAllString(f.Code, ""))
		custom[f.ID] = strings.ContainsAny(code, "yd")
	}
	out := make([]bool, len(s.Styles))
	for i, style := range s.Styles {
		id := style.Format
		out[i] = custom[id] || id >= 14 && id <= 17 || id == 22 || id >= 27 && id <= 31 || id >= 34 && id <= 36 || id >= 50 && id <= 58
	}
	return out
}

func excelDate(v string, date1904 bool) string {
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(n) || n < 0 || n >= 2958466 {
		return v
	}
	days := int(n)
	epoch := time.Date(1899, 12, 31, 0, 0, 0, 0, time.UTC)
	if date1904 {
		epoch = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	} else if days == 60 {
		return "29.02.1900" // Excel's fictitious leap day; rejected by date validation.
	} else if days > 60 {
		days--
	}
	return epoch.AddDate(0, 0, days).Format("2006-01-02")
}

type xlsxRow struct {
	R string     `xml:"r,attr"`
	C []xlsxCell `xml:"c"`
}

// Лимиты XLSX: файл до 10 МБ сжатым может распаковаться в гигабайты (zip-бомба) или объявить ячейку XFD1048576,
// поэтому всё, что читается из архива, проходит через общий бюджет на файл.
const (
	MaxUnpacked = 32 << 20  // распакованных байт на файл суммарно
	MaxSheets   = 10        // листов
	MaxCells    = 200_000   // ячеек на файл
	maxSlots    = 2_000_000 // ячеек с учётом пустых до последней колонки строки (память под строки)
	maxCol      = 16383     // XFD — последняя колонка Excel
	maxRow      = 1_048_576 // последняя строка Excel
)

// ErrTooLarge — файл превышает лимиты или устроен некорректно. API отвечает 400.
var ErrTooLarge = errors.New("файл слишком большой или повреждён")

// budget — общий на файл лимит распакованных байт.
type budget struct{ left int64 }

type budgetReader struct {
	r io.Reader
	b *budget
}

func (br budgetReader) Read(p []byte) (int, error) {
	if br.b.left <= 0 {
		return 0, ErrTooLarge
	}
	if int64(len(p)) > br.b.left {
		p = p[:br.b.left]
	}
	n, err := br.r.Read(p)
	br.b.left -= int64(n)
	return n, err
}

func readXLSX(data []byte) ([]Table, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	bud := &budget{left: MaxUnpacked}
	open := func(name string) (io.ReadCloser, io.Reader, error) {
		f, ok := files[name]
		if !ok {
			return nil, nil, fmt.Errorf("xlsx: %s not found: %w", name, os.ErrNotExist)
		}
		if f.UncompressedSize64 > MaxUnpacked {
			return nil, nil, ErrTooLarge
		}
		rc, err := f.Open()
		if err != nil {
			return nil, nil, fmt.Errorf("xlsx: %s: %w", name, err)
		}
		return rc, budgetReader{r: rc, b: bud}, nil
	}
	read := func(name string, v any) error {
		rc, r, err := open(name)
		if err != nil {
			return err
		}
		defer rc.Close()
		if err := xml.NewDecoder(r).Decode(v); err != nil {
			return wrapXML(name, err)
		}
		return nil
	}
	var wb xlsxWorkbook
	if err := read("xl/workbook.xml", &wb); err != nil {
		return nil, err
	}
	if len(wb.Sheets) > MaxSheets {
		return nil, fmt.Errorf("%w: листов %d, допустимо не больше %d", ErrTooLarge, len(wb.Sheets), MaxSheets)
	}
	var styles xlsxStyles
	if err := read("xl/styles.xml", &styles); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	calendar := styles.calendarStyles()
	var rels xlsxRels
	if err := read("xl/_rels/workbook.xml.rels", &rels); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	target := map[string]string{}
	for _, r := range rels.Rels {
		t := strings.TrimPrefix(r.Target, "/")
		if !strings.HasPrefix(t, "xl/") {
			t = path.Join("xl", t)
		}
		target[r.ID] = t
	}
	// sharedStrings необязателен, но если он есть и битый — это ошибка, а не пустые заголовки
	var sst xlsxSST
	if err := read("xl/sharedStrings.xml", &sst); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	shared := make([]string, len(sst.SI))
	for i, si := range sst.SI {
		s := si.T
		for _, r := range si.R {
			s += r.T
		}
		shared[i] = s
	}
	cells, slots := 0, 0
	var out []Table
	for i, sh := range wb.Sheets {
		name := target[sh.RID]
		if name == "" {
			name = fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1)
		}
		t := Table{Name: sh.Name}
		if err := readSheet(open, name, shared, calendar, wb.Properties.Date1904, &t, &cells, &slots); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// readSheet читает лист потоково, строка за строкой, и прерывается, как только превышен лимит ячеек.
func readSheet(open func(string) (io.ReadCloser, io.Reader, error), name string, shared []string, calendar []bool, date1904 bool, t *Table, cells, slots *int) error {
	rc, r, err := open(name)
	if err != nil {
		return err
	}
	defer rc.Close()
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return wrapXML(name, err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "row" {
			continue
		}
		var row xlsxRow
		if err := dec.DecodeElement(&row, &se); err != nil {
			return wrapXML(name, err)
		}
		if row.R != "" {
			if n, err := strconv.Atoi(row.R); err != nil || n < 1 || n > maxRow {
				return fmt.Errorf("%w: номер строки %q", ErrTooLarge, row.R)
			}
		}
		*cells += len(row.C)
		if *cells > MaxCells {
			return fmt.Errorf("%w: больше %d ячеек", ErrTooLarge, MaxCells)
		}
		vals := map[int]string{}
		maxC := -1
		for j, c := range row.C {
			col := j
			if c.R != "" {
				if col = colIndex(c.R); col < 0 {
					return fmt.Errorf("%w: адрес ячейки %q", ErrTooLarge, c.R)
				}
			}
			if col > maxCol {
				return fmt.Errorf("%w: колонка за пределами XFD", ErrTooLarge)
			}
			v := c.V
			switch c.T {
			case "", "n":
				if c.Style >= 0 && c.Style < len(calendar) && calendar[c.Style] {
					v = excelDate(c.V, date1904)
				}
			case "s":
				if n, err := strconv.Atoi(c.V); err == nil && n >= 0 && n < len(shared) {
					v = shared[n]
				}
			case "inlineStr":
				v = c.IS.T
				for _, r := range c.IS.R {
					v += r.T
				}
			}
			vals[col] = v
			if col > maxC {
				maxC = col
			}
		}
		*slots += maxC + 1
		if *slots > maxSlots {
			return fmt.Errorf("%w: слишком широкие строки", ErrTooLarge)
		}
		line := make([]string, maxC+1)
		for k, v := range vals {
			line[k] = v
		}
		t.Rows = append(t.Rows, line)
	}
}

func wrapXML(name string, err error) error {
	if errors.Is(err, ErrTooLarge) {
		return err
	}
	return fmt.Errorf("%w: %s: %v", ErrTooLarge, name, err)
}

// colIndex: "C12" → 2, "ab7" → 27. -1 — нет букв, больше трёх букв или номер строки вне 1..1048576.
func colIndex(ref string) int {
	n, letters, i := 0, 0, 0
	for ; i < len(ref); i++ {
		ch := ref[i]
		if ch >= 'a' && ch <= 'z' {
			ch -= 'a' - 'A'
		}
		if ch < 'A' || ch > 'Z' {
			break
		}
		letters++
		if letters > 3 {
			return -1
		}
		n = n*26 + int(ch-'A'+1)
	}
	if letters == 0 {
		return -1
	}
	if rest := ref[i:]; rest != "" {
		row, err := strconv.Atoi(rest)
		if err != nil || row < 1 || row > maxRow {
			return -1
		}
	}
	return n - 1
}

// headerRow — индекс первой строки, где заполнено хотя бы два поля.
func headerRow(rows [][]string) int {
	for i, r := range rows {
		filled := 0
		for _, c := range r {
			if strings.TrimSpace(c) != "" {
				filled++
			}
		}
		if filled >= 2 {
			return i
		}
	}
	return -1
}

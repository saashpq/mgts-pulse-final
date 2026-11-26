package ingest

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/ElzaraA/mgts-ai/internal/survey"
)

// Report — что произошло при импорте: HR видит его сразу после загрузки файла.
type Report struct {
	Sheets     []SheetReport     `json:"sheets"`
	Accepted   int               `json:"accepted"`
	Rejected   int               `json:"rejected"`
	NewPairs   []string          `json:"new_pairs,omitempty"`
	StartMonth string            `json:"start_month,omitempty"`
	Responses  []survey.Response `json:"-"`
}

type SheetReport struct {
	Name     string   `json:"name"`
	Role     string   `json:"role"`
	Mapped   []string `json:"mapped"`   // «заголовок → код»
	Unmapped []string `json:"unmapped"` // колонки, которые не удалось сопоставить
	Accepted int      `json:"accepted"`
	Errors   []string `json:"errors,omitempty"` // «строка N: причина», не больше 20
}

var nonWord = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// norm: «Интересны ли задачи?» → «интересны ли задачи».
func norm(s string) string {
	s = strings.ToLower(strings.ReplaceAll(s, "ё", "е"))
	s = strings.ReplaceAll(s, "Ё", "е")
	return strings.TrimSpace(nonWord.ReplaceAllString(s, " "))
}

// Import сопоставляет колонки листов с вопросами анкеты и собирает ответы.
// roleHint пустой — роль листа определяется по тому, вопросов какой роли в нём больше.
// Неизвестные пары (ID стажёра и наставника есть, а в survey их нет) добавляются в sv.Participant.
func Import(tables []Table, sv *survey.Survey, roleHint survey.Role, source string) Report {
	return ImportMapped(tables, sv, roleHint, nil, source)
}

// ImportMapped — Import с ручным сопоставлением: для листов из mapping колонки берутся из него,
// остальные листы сопоставляются автоматически. mapping должен быть проверен ValidateMapping.
func ImportMapped(tables []Table, sv *survey.Survey, roleHint survey.Role, mapping Mapping, source string) Report {
	rep := Report{Sheets: []SheetReport{}}
	firstRow := map[string]string{}
	type sheetPlan struct {
		t Table
		h int
		p plan
	}
	var sheets []sheetPlan
	for _, t := range tables {
		h := headerRow(t.Rows)
		if h < 0 {
			continue
		}
		head := t.Rows[h]
		var p plan
		if m, ok := mapping[t.Name]; ok {
			p = manualPlan(m)
		} else {
			p = autoPlan(head, sv, roleHint)
		}
		sheets = append(sheets, sheetPlan{t, h, p})
	}
	start, _ := time.Parse("2006-01", sv.StartMonth)
	if start.IsZero() {
		// Only rows with valid answers and resolvable identities may establish the calendar.
		probe := *sv
		probe.Participant = probe.Participant[:len(probe.Participant):len(probe.Participant)]
		var scratch Report
		for _, sheet := range sheets {
			p := sheet.p
			for _, row := range sheet.t.Rows[sheet.h+1:] {
				if p.role == "" || p.idP < 0 || p.idP >= len(row) {
					continue
				}
				date, index := parsePeriod(row[p.idP])
				if !date.IsZero() {
					index = 1
				}
				if _, err := buildResponse(row, p.cols, p.role, p.idI, p.idM, index, &probe, &scratch); err == nil && !date.IsZero() && (start.IsZero() || date.Before(start)) {
					start = date
				}
			}
		}
	}
	for _, sheet := range sheets {
		t, h, p := sheet.t, sheet.h, sheet.p
		head := t.Rows[h]
		sr := SheetReport{Name: t.Name, Role: string(p.role)}
		if p.role == "" {
			sr.Errors = append(sr.Errors, "не найдено ни одного вопроса анкеты в заголовках")
			rep.Sheets = append(rep.Sheets, sr)
			continue
		}
		for i, name := range head {
			if c, ok := p.cols[i]; ok {
				sr.Mapped = append(sr.Mapped, fmt.Sprintf("%s → %s", strings.TrimSpace(name), c))
			} else if i != p.idI && i != p.idM && i != p.idP && strings.TrimSpace(name) != "" {
				sr.Unmapped = append(sr.Unmapped, strings.TrimSpace(name))
			}
		}
		if p.idP < 0 || (p.idI < 0 && p.idM < 0) {
			sr.Errors = append(sr.Errors, "нет колонки месяца или ID участника")
			rep.Sheets = append(rep.Sheets, sr)
			continue
		}
		for ri, row := range t.Rows[h+1:] {
			line := h + ri + 2
			if empty(row) {
				continue
			}
			raw := ""
			if p.idP < len(row) {
				raw = row[p.idP]
			}
			date, index := parsePeriod(raw)
			if !date.IsZero() && !start.IsZero() {
				index = 1 + (date.Year()-start.Year())*12 + int(date.Month()-start.Month())
			}
			var resp survey.Response
			var err error
			if index < 1 || index > sv.Periods {
				err = fmt.Errorf("месяц %q вне диапазона 1..%d", raw, sv.Periods)
				if !date.IsZero() && !start.IsZero() {
					err = fmt.Errorf("%w (начало программы %s)", err, start.Format("2006-01"))
				}
			} else {
				resp, err = buildResponse(row, p.cols, p.role, p.idI, p.idM, index, sv, &rep)
			}
			if err == nil {
				if earlier, duplicate := firstRow[resp.ID]; duplicate {
					err = fmt.Errorf("повтор анкеты за тот же месяц и роль; исходная строка %s", earlier)
				} else {
					firstRow[resp.ID] = fmt.Sprintf("%s/%d", t.Name, line)
				}
			}
			if err != nil {
				rep.Rejected++
				if len(sr.Errors) < 20 {
					sr.Errors = append(sr.Errors, fmt.Sprintf("строка %d: %v", line, err))
				}
				continue
			}
			resp.Source = source
			if !date.IsZero() {
				sv.StartMonth = start.Format("2006-01")
				rep.StartMonth = sv.StartMonth
			}
			rep.Responses = append(rep.Responses, resp)
			sr.Accepted++
			rep.Accepted++
		}
		rep.Sheets = append(rep.Sheets, sr)
	}
	return rep
}

// plan — как читать лист: роль, колонки вопросов (индекс → код) и индексы служебных колонок (-1 — нет).
type plan struct {
	role          survey.Role
	cols          map[int]string
	idI, idM, idP int
}

// autoPlan — сопоставление по заголовкам: вопросы по тексту и aliases, служебные колонки по id_columns.
func autoPlan(head []string, sv *survey.Survey, hint survey.Role) plan {
	role, cols := mapColumns(head, sv, hint)
	return plan{role: role, cols: cols, idI: findCol(head, sv.IDColumns.Intern), idM: findCol(head, sv.IDColumns.Mentor), idP: findCol(head, sv.IDColumns.Period)}
}

func mapColumns(head []string, sv *survey.Survey, hint survey.Role) (survey.Role, map[int]string) {
	best := survey.Role("")
	var bestCols map[int]string
	for _, role := range []survey.Role{survey.Intern, survey.Mentor} {
		if hint != "" && role != hint {
			continue
		}
		cols := map[int]string{}
		used := map[string]bool{}
		for i, name := range head {
			n := norm(name)
			if n == "" {
				continue
			}
			for _, q := range sv.ByRole(role) {
				if used[q.Code] {
					continue
				}
				if matches(n, q) {
					cols[i] = q.Code
					used[q.Code] = true
					break
				}
			}
		}
		if len(cols) > len(bestCols) {
			best, bestCols = role, cols
		}
	}
	return best, bestCols
}

// matches — заголовок колонки соответствует вопросу: точное совпадение кода или текста,
// либо все значимые слова варианта есть в заголовке (сравнение по основам: «интересны» ~ «интерес»),
// либо значимые слова заголовка (не меньше двух) есть в тексте вопроса.
func matches(n string, q survey.Question) bool {
	if n == norm(q.Code) || n == norm(q.Text) {
		return true
	}
	head := stems(n)
	if len(head) == 0 {
		return false
	}
	for _, a := range append([]string{q.Text}, q.Aliases...) {
		if as := stems(norm(a)); len(as) > 0 && subset(as, head) {
			return true
		}
	}
	if len(head) >= 2 {
		pool := stems(norm(q.Text + " " + strings.Join(q.Aliases, " ")))
		if subset(head, pool) {
			return true
		}
	}
	return false
}

var stop = map[string]bool{"ли": true, "в": true, "на": true, "вам": true, "ваш": true, "ваша": true, "целом": true, "насколько": true,
	"как": true, "ли.": true, "и": true, "по": true, "из": true, "с": true, "от": true, "до": true, "за": true, "о": true, "the": true}

// stems — множество основ значимых слов (первые 5 букв), без чисел и служебных слов.
func stems(n string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(n) {
		if stop[w] || strings.IndexFunc(w, unicode.IsLetter) < 0 {
			continue
		}
		r := []rune(w)
		if len(r) > 5 {
			r = r[:5]
		}
		out[string(r)] = true
	}
	return out
}

func subset(a, b map[string]bool) bool {
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func findCol(head []string, names []string) int {
	for i, h := range head {
		n := norm(h)
		for _, want := range names {
			if n == norm(want) {
				return i
			}
		}
	}
	return -1
}

var ordinalPeriod = regexp.MustCompile(`(?i)^(?:месяц\s*|month\s*|[мm]\s*)?([0-9]+)$`)

// parsePeriod preserves actual calendar gaps and accepts dates only when the whole value is valid.
func parsePeriod(raw string) (time.Time, int) {
	v := strings.TrimSpace(raw)
	if m := ordinalPeriod.FindStringSubmatch(v); m != nil {
		n, _ := strconv.Atoi(m[1])
		return time.Time{}, n
	}
	for _, layout := range []string{"2006-1", "2006.1", "2006/1", "2006-1-2", "2006.1.2", "2006/1/2", "2.1.2006", "2/1/2006", time.RFC3339, "2006-01-02T15:04:05"} {
		if d, err := time.Parse(layout, v); err == nil {
			return time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC), 0
		}
	}
	return time.Time{}, 0
}

func buildResponse(row []string, cols map[int]string, role survey.Role, idI, idM, period int, sv *survey.Survey, rep *Report) (survey.Response, error) {
	cell := func(i int) string {
		if i < 0 || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	if period < 1 || period > sv.Periods {
		return survey.Response{}, fmt.Errorf("месяц вне диапазона 1..%d", sv.Periods)
	}
	internID, mentorID := cell(idI), cell(idM)
	if internID == "" && mentorID == "" {
		return survey.Response{}, fmt.Errorf("пустой ID участника")
	}
	pair, exists, err := findPair(sv, internID, mentorID)
	if err != nil {
		return survey.Response{}, err
	}
	r := survey.Response{Role: role, Period: period,
		Scores: map[string]float64{}, Comments: map[string]string{}}
	for i, code := range cols {
		q, _ := sv.Q(code)
		v := cell(i)
		if v == "" {
			continue
		}
		if q.Kind == "text" {
			if len([]rune(v)) > 2000 {
				return survey.Response{}, fmt.Errorf("%s: комментарий длиннее 2000 символов", code)
			}
			r.Comments[code] = cleanText(v)
			continue
		}
		f, err := strconv.ParseFloat(strings.Replace(v, ",", ".", 1), 64)
		if err != nil || math.IsNaN(f) || f < 1 || f > q.Max || (!sv.FractionalHistory && f != math.Trunc(f)) {
			return survey.Response{}, fmt.Errorf("%s: значение %q вне шкалы целых чисел 1..%g", code, v, q.Max)
		}
		r.Scores[code] = f
	}
	if len(r.Scores) == 0 && len(r.Comments) == 0 {
		return survey.Response{}, fmt.Errorf("нет ответов")
	}
	if !exists {
		pair = survey.Pair{ID: nextPairID(sv), Intern: internID, Mentor: mentorID}
		sv.Participant = append(sv.Participant, pair)
		rep.NewPairs = append(rep.NewPairs, pair.ID)
	}
	r.PairID = pair.ID
	r.Person = pair.Intern
	if role == survey.Mentor {
		r.Person = pair.Mentor
	}
	r.ID = fmt.Sprintf("%s-%s-M%d", pair.ID, role, period)
	return r, nil
}

// nextPairID — P<max+1> по уже существующим ID вида P<n>; len+1 дал бы дубль, если пары удаляли или
// нумерация в survey.json не подряд. Уникальность проверяется по всем ID.
func nextPairID(sv *survey.Survey) string {
	used := map[string]bool{}
	n := 0
	for _, p := range sv.Participant {
		used[strings.ToUpper(p.ID)] = true
		if len(p.ID) > 1 && (p.ID[0] == 'P' || p.ID[0] == 'p') {
			if k, err := strconv.Atoi(p.ID[1:]); err == nil && k > n && k < 1_000_000 {
				n = k
			}
		}
	}
	for {
		n++
		if id := fmt.Sprintf("P%d", n); !used[id] {
			return id
		}
	}
}

func findPair(sv *survey.Survey, internID, mentorID string) (survey.Pair, bool, error) {
	if internID != "" && strings.EqualFold(internID, mentorID) {
		return survey.Pair{}, false, fmt.Errorf("ID стажёра совпадает с ID наставника")
	}
	var found survey.Pair
	for _, p := range sv.Participant {
		if (internID != "" && strings.EqualFold(p.Mentor, internID)) || (mentorID != "" && strings.EqualFold(p.Intern, mentorID)) {
			return survey.Pair{}, false, fmt.Errorf("роль участника расходится со списком пар")
		}
		i := internID != "" && strings.EqualFold(p.Intern, internID)
		m := mentorID != "" && strings.EqualFold(p.Mentor, mentorID)
		if !i && !m {
			continue
		}
		if found.ID != "" || (internID != "" && !i) || (mentorID != "" && !m) {
			return survey.Pair{}, false, fmt.Errorf("связь стажёра и наставника расходится со списком пар")
		}
		found = p
	}
	if found.ID != "" {
		return found, true, nil
	}
	if internID == "" || mentorID == "" {
		return survey.Pair{}, false, fmt.Errorf("участник %q не найден в списке пар. Для новой пары добавьте оба ID или сначала загрузите файл с обоими ID, затем повторите эту загрузку", internID+mentorID)
	}
	return survey.Pair{}, false, nil
}
func empty(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// cleanText убирает управляющие символы и обрезает слишком длинные комментарии.
func cleanText(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' {
			return -1
		}
		return r
	}, s)
	if r := []rune(s); len(r) > 2000 {
		s = string(r[:2000])
	}
	return strings.TrimSpace(s)
}

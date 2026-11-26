package ingest

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ElzaraA/mgts-ai/internal/survey"
)

// Служебные цели сопоставления колонок (не коды вопросов).
const (
	TargetIntern = "intern_id"
	TargetMentor = "mentor_id"
	TargetPeriod = "period"
	TargetSkip   = "skip"
)

// Target — значение для выпадающего списка «куда загрузить колонку».
type Target struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Role  string `json:"role"` // intern | mentor | any
}

// Targets — все допустимые цели: вопросы обеих ролей, служебные колонки и «Не загружать».
func Targets(sv survey.Survey) []Target {
	var out []Target
	for _, q := range sv.Questions {
		out = append(out, Target{Value: q.Code, Label: q.Text, Role: string(q.Role)})
	}
	return append(out,
		Target{Value: TargetIntern, Label: "ID стажёра", Role: "any"},
		Target{Value: TargetMentor, Label: "ID наставника", Role: "any"},
		Target{Value: TargetPeriod, Label: "Месяц", Role: "any"},
		Target{Value: TargetSkip, Label: "Не загружать", Role: "any"},
	)
}

// ColumnPreview — колонка листа с предложенным сопоставлением. Target nil — не распознана.
type ColumnPreview struct {
	Index   int      `json:"index"`
	Title   string   `json:"title"`
	Target  *string  `json:"target"`
	Samples []string `json:"samples"`
}

// SheetPreview — лист файла: роль по заголовкам (” — не определена), число строк данных и колонки.
type SheetPreview struct {
	Name    string          `json:"name"`
	Role    string          `json:"role"`
	Rows    int             `json:"rows"`
	Columns []ColumnPreview `json:"columns"`
}

// Preview показывает, как файл будет прочитан автоматически, ничего не сохраняя.
func Preview(tables []Table, sv survey.Survey, roleHint survey.Role) []SheetPreview {
	out := []SheetPreview{}
	for _, t := range tables {
		sp := SheetPreview{Name: t.Name, Columns: []ColumnPreview{}}
		h := headerRow(t.Rows)
		if h < 0 {
			out = append(out, sp)
			continue
		}
		head := t.Rows[h]
		p := autoPlan(head, &sv, roleHint)
		sp.Role = string(p.role)
		data := t.Rows[h+1:]
		width := len(head)
		for _, r := range data {
			if empty(r) {
				continue
			}
			sp.Rows++
			if len(r) > width {
				width = len(r)
			}
		}
		for i := 0; i < width; i++ {
			title := ""
			if i < len(head) {
				title = strings.TrimSpace(head[i])
			}
			c := ColumnPreview{Index: i, Title: title, Samples: samples(data, i, 3)}
			if title == "" && len(c.Samples) == 0 {
				continue
			}
			target := ""
			switch {
			case p.role != "" && p.cols[i] != "":
				target = p.cols[i]
			case i == p.idI:
				target = TargetIntern
			case i == p.idM:
				target = TargetMentor
			case i == p.idP:
				target = TargetPeriod
			}
			if target != "" {
				c.Target = &target
			}
			sp.Columns = append(sp.Columns, c)
		}
		out = append(out, sp)
	}
	return out
}

// samples — до n непустых значений колонки (обрезаны до 80 символов: превью, а не данные).
func samples(rows [][]string, col, n int) []string {
	out := []string{}
	for _, r := range rows {
		if len(out) == n {
			break
		}
		if col >= len(r) {
			continue
		}
		v := strings.TrimSpace(r[col])
		if v == "" {
			continue
		}
		if rs := []rune(v); len(rs) > 80 {
			v = string(rs[:80]) + "…"
		}
		out = append(out, v)
	}
	return out
}

// SheetMapping — ручное сопоставление листа: роль и «индекс колонки → цель».
type SheetMapping struct {
	Role    survey.Role       `json:"role"`
	Columns map[string]string `json:"columns"`
}

// Mapping — по имени листа (для CSV — имя файла).
type Mapping map[string]SheetMapping

// ParseMapping разбирает JSON-строку поля mapping и проверяет её по структуре анкеты.
func ParseMapping(raw string, sv survey.Survey) (Mapping, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var m Mapping
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("mapping: некорректный JSON")
	}
	return m, ValidateMapping(m, sv)
}

// ValidateMapping: роль intern|mentor, индексы — неотрицательные числа, цели из Targets,
// вопрос — той же роли, что и лист, каждая цель (кроме skip) — не больше одной колонки.
func ValidateMapping(m Mapping, sv survey.Survey) error {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sm := m[name]
		if sm.Role != survey.Intern && sm.Role != survey.Mentor {
			return fmt.Errorf("mapping %q: role — intern | mentor", name)
		}
		used := map[string]string{}
		for idx, target := range sm.Columns {
			if n, err := strconv.Atoi(idx); err != nil || n < 0 || n > maxCol {
				return fmt.Errorf("mapping %q: индекс колонки %q", name, idx)
			}
			switch target {
			case TargetSkip, "":
				continue
			case TargetIntern, TargetMentor, TargetPeriod:
			default:
				q, ok := sv.Q(target)
				if !ok {
					return fmt.Errorf("mapping %q: неизвестная цель %q", name, target)
				}
				if q.Role != sm.Role {
					return fmt.Errorf("mapping %q: вопрос %s относится к другой роли", name, target)
				}
			}
			if prev, dup := used[target]; dup {
				return fmt.Errorf("mapping %q: колонки %s и %s сопоставлены с одной целью %s", name, prev, idx, target)
			}
			used[target] = idx
		}
	}
	return nil
}

// manualPlan — план чтения листа по проверенному ручному сопоставлению.
func manualPlan(sm SheetMapping) plan {
	p := plan{role: sm.Role, cols: map[int]string{}, idI: -1, idM: -1, idP: -1}
	for idx, target := range sm.Columns {
		i, _ := strconv.Atoi(idx)
		switch target {
		case TargetSkip, "":
		case TargetIntern:
			p.idI = i
		case TargetMentor:
			p.idM = i
		case TargetPeriod:
			p.idP = i
		default:
			p.cols[i] = target
		}
	}
	return p
}

// Remember добавляет заголовки сопоставленных вручную колонок в aliases вопросов и служебных колонок,
// чтобы следующий файл с теми же заголовками сопоставился автоматически. Возвращает число новых вариантов.
func Remember(tables []Table, sv *survey.Survey, m Mapping) int {
	// копия вопросов: срез делит память с survey в хранилище, менять его на месте нельзя
	sv.Questions = append([]survey.Question(nil), sv.Questions...)
	n := 0
	for _, t := range tables {
		sm, ok := m[t.Name]
		if !ok {
			continue
		}
		h := headerRow(t.Rows)
		if h < 0 {
			continue
		}
		head := t.Rows[h]
		idxs := make([]int, 0, len(sm.Columns))
		for idx := range sm.Columns {
			i, _ := strconv.Atoi(idx)
			idxs = append(idxs, i)
		}
		sort.Ints(idxs)
		for _, i := range idxs {
			if i >= len(head) {
				continue
			}
			title := strings.TrimSpace(head[i])
			if norm(title) == "" {
				continue
			}
			switch target := sm.Columns[strconv.Itoa(i)]; target {
			case TargetSkip, "":
			case TargetIntern:
				n += addAlias(&sv.IDColumns.Intern, title)
			case TargetMentor:
				n += addAlias(&sv.IDColumns.Mentor, title)
			case TargetPeriod:
				n += addAlias(&sv.IDColumns.Period, title)
			default:
				for qi := range sv.Questions {
					q := &sv.Questions[qi]
					if q.Code != target || norm(title) == norm(q.Code) || norm(title) == norm(q.Text) {
						continue
					}
					n += addAlias(&q.Aliases, title)
				}
			}
		}
	}
	return n
}

// addAlias добавляет вариант, если такого (после нормализации) ещё нет. Срез копируется.
func addAlias(list *[]string, title string) int {
	for _, a := range *list {
		if norm(a) == norm(title) {
			return 0
		}
	}
	*list = append(append([]string(nil), *list...), title)
	return 1
}

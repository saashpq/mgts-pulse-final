// Package privacy маскирует персональные данные в тексте перед отправкой в языковую модель:
// телефоны, e-mail, паспорт- и СНИЛС-подобные числа, ФИО и ID участников из словаря.
// В модель уходит только маскированный текст; исходный остаётся в контуре.
package privacy

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Метки масок. Модель видит их вместо данных: «[телефон]», «[ФИО]» и т. д.
const (
	Phone    = "телефон"
	Email    = "e-mail"
	Passport = "паспорт"
	SNILS    = "СНИЛС"
	Name     = "ФИО"
	Handle   = "ник"
)

// Tag — как метка выглядит в маскированном тексте.
func Tag(label string) string { return "[" + label + "]" }

// sep — разделители внутри номеров: пробелы (и неразрывный), точки, дефисы и их типографские варианты.
const sep = `[\s\x{00A0}\x{202F}\x{2010}-\x{2015}\x{2212}\-.]`

// dash — дефисы в ID и двойных фамилиях: обычный, неразрывный (U+2011), тире и минус.
const dash = `[\-\x{2010}-\x{2015}\x{2212}]`

var (
	reEmail = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	// +7 / 7 / 8, затем 10 цифр с пробелами, дефисами, точками и скобками в любых местах: «7 916 123 45 67», «8 (916) 123-45-67».
	rePhone = regexp.MustCompile(`(?:\+7|(?:^|[^\d+])[78])` + sep + `*\(?` + sep + `*\d{3}` + sep + `*\)?` + sep + `*\d{3}` + sep + `*\d{2}` + sep + `*\d{2}(?:\D|$)`)
	// международный формат: «+» и код страны, всего 8–15 цифр (E.164): «+44 20 7946 0958»
	reIntl     = regexp.MustCompile(`\+[1-9](?:(?:` + sep + `|[()])*\d){7,14}(?:\D|$)`)
	reSNILS    = regexp.MustCompile(`\d{3}-\d{3}-\d{3}[\s\-]\d{2}|(?:^|\D)\d{11}(?:\D|$)`)
	rePassport = regexp.MustCompile(`(?:^|\D)\d{2}\s?\d{2}\s?№?\s?\d{6}(?:\D|$)`)
	// ник в Telegram: @ + 5–32 латинских букв, цифр и «_» (не часть e-mail — e-mail маскируется раньше)
	reHandle = regexp.MustCompile(`(?:^|[^A-Za-z0-9_@.\]])(@[A-Za-z][A-Za-z0-9_]{4,31})(?:[^A-Za-z0-9_]|$)`)
	// ФИО по шаблону, без словаря: «Иванова А.П.», «Иванова А. П.», «А. П. Иванова», «Римский-Корсаков Н.А.»
	reInitials = regexp.MustCompile(`(?:^|[^\p{L}])([А-ЯЁ][а-яё]+(?:` + dash + `[А-ЯЁ][а-яё]+)?\s*[А-ЯЁ]\.\s*[А-ЯЁ]\.|[А-ЯЁ]\.\s*[А-ЯЁ]\.\s*[А-ЯЁ][а-яё]+(?:` + dash + `[А-ЯЁ][а-яё]+)?)`)
	reWord     = regexp.MustCompile(`[\p{L}\p{N}]+(?:` + dash + `[\p{L}\p{N}]+)*`)
)

// Mask заменяет персональные данные метками. found — типы найденного (без повторов, в порядке проверки).
// names — словарь: ID участников и, если есть, ФИО (каждое слово ищется с учётом падежных окончаний).
func Mask(text string, names []string) (string, []string) {
	var found []string
	note := func(label string) {
		for _, f := range found {
			if f == label {
				return
			}
		}
		found = append(found, label)
	}
	text = reEmail.ReplaceAllStringFunc(text, func(string) string { note(Email); return Tag(Email) })
	text = replaceCore(text, rePhone, Phone, note)
	text = replaceCore(text, reIntl, Phone, note)
	text = replaceCore(text, reSNILS, SNILS, note)
	text = replaceCore(text, rePassport, Passport, note)
	text = replaceGroup(text, reHandle, Handle, note)
	text = replaceGroup(text, reInitials, Name, note)
	if masked, hit := maskNames(text, names); hit {
		text = masked
		note(Name)
	}
	return text, found
}

// replaceCore заменяет совпадение, сохраняя граничные символы, захваченные шаблоном вместо \b
// (в RE2 \b не работает с кириллицей).
func replaceCore(text string, re *regexp.Regexp, label string, note func(string)) string {
	return re.ReplaceAllStringFunc(text, func(m string) string {
		start, end := 0, len(m)
		for start < end {
			r, size := utf8.DecodeRuneInString(m[start:])
			if r >= '0' && r <= '9' || r == '+' {
				break
			}
			start += size
		}
		for end > start {
			r, size := utf8.DecodeLastRuneInString(m[:end])
			if r >= '0' && r <= '9' {
				break
			}
			end -= size
		}
		note(label)
		return m[:start] + Tag(label) + m[end:]
	})
}

// replaceGroup заменяет первую группу совпадения (граничные символы вокруг неё остаются).
func replaceGroup(text string, re *regexp.Regexp, label string, note func(string)) string {
	var b strings.Builder
	prev := 0
	for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
		if m[2] < prev {
			continue
		}
		b.WriteString(text[prev:m[2]])
		b.WriteString(Tag(label))
		prev = m[3]
		note(label)
	}
	b.WriteString(text[prev:])
	return b.String()
}

type nameWord struct{ full, stem string }

// caseEndings — падежные окончания имён и фамилий. Словарное слово совпадает с токеном целиком или
// как «основа + окончание из списка»: «Вера» → «Веры», «Верой», но не «верно» и не «верх».
// Пустое окончание («Иванова» → «Иванов») — только для основ от 5 букв: «Мира» не должна задевать «мир».
var caseEndings = map[string]bool{"а": true, "я": true, "у": true, "ю": true, "е": true, "ы": true, "и": true,
	"ой": true, "ей": true, "ою": true, "ею": true, "ом": true, "ем": true, "ым": true, "им": true,
	"ого": true, "его": true, "ому": true, "ему": true, "ах": true, "ях": true, "ам": true, "ям": true,
	"ами": true, "ями": true, "ов": true, "ев": true, "ых": true, "их": true}

func dictionary(names []string) []nameWord {
	var out []nameWord
	for _, n := range names {
		for _, w := range reWord.FindAllString(norm(n), -1) {
			if utf8.RuneCountInString(w) < 2 {
				continue
			}
			nw := nameWord{full: w, stem: w}
			rs := []rune(w)
			if len(rs) >= 4 && strings.ContainsRune("аяйьоеиыу", rs[len(rs)-1]) {
				nw.stem = string(rs[:len(rs)-1])
			}
			out = append(out, nw)
		}
	}
	return out
}

func (d nameWord) match(tok string) bool {
	if tok == d.full {
		return true
	}
	// падежные формы: «Иванова» → «Ивановой», «Анна» → «Анне», «Пётр» → «Петру»; только окончания из caseEndings
	if utf8.RuneCountInString(d.stem) < 3 || !strings.HasPrefix(tok, d.stem) {
		return false
	}
	tail := strings.TrimPrefix(tok, d.stem)
	if tail == "" {
		return utf8.RuneCountInString(d.stem) >= 5
	}
	return caseEndings[tail]
}

// maskNames заменяет слова из словаря; подряд идущие слова ФИО сливаются в одну метку.
func maskNames(text string, names []string) (string, bool) {
	dict := dictionary(names)
	if len(dict) == 0 {
		return text, false
	}
	locs := reWord.FindAllStringIndex(text, -1)
	var b strings.Builder
	prev, hit, lastMaskEnd := 0, false, -1
	for _, l := range locs {
		tok := norm(text[l[0]:l[1]])
		matched := false
		for _, d := range dict {
			if d.match(tok) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		hit = true
		gap := text[prev:l[0]]
		if lastMaskEnd == prev && strings.TrimSpace(gap) == "" {
			// продолжение того же ФИО: пробелы между словами поглощаем
			prev, lastMaskEnd = l[1], l[1]
			continue
		}
		b.WriteString(gap)
		b.WriteString(Tag(Name))
		prev, lastMaskEnd = l[1], l[1]
	}
	b.WriteString(text[prev:])
	return b.String(), hit
}

// norm: нижний регистр, ё → е, любые дефисы → «-» (ID «M‑01» с неразрывным дефисом = «M-01»).
func norm(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == 'ё':
			return 'е'
		case r >= 0x2010 && r <= 0x2015, r == 0x2212:
			return '-'
		}
		return r
	}, strings.ToLower(s))
}

// HasMask — есть ли в тексте метка маскирования.
func HasMask(s string) bool {
	for _, l := range []string{Phone, Email, Passport, SNILS, Name, Handle} {
		if strings.Contains(s, Tag(l)) {
			return true
		}
	}
	return false
}

// LoadNames читает необязательный словарь имён: JSON-массив строк или {"names": [...]}. Нет файла — пустой список.
func LoadNames(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []string
	if err := json.Unmarshal(b, &list); err == nil {
		return list, nil
	}
	var obj struct {
		Names []string `json:"names"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return nil, err
	}
	return obj.Names, nil
}

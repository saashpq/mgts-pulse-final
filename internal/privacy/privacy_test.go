package privacy

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMask(t *testing.T) {
	names := []string{"I-01", "M-01", "Иванова Анна Петровна", "Пётр Сидоров"}
	cases := []struct {
		name, in, want string
		found          []string
	}{
		{"phone +7 spaces", "Звоните +7 916 123 45 67 после обеда", "Звоните [телефон] после обеда", []string{Phone}},
		{"phone 8 brackets", "мой номер 8 (916) 123-45-67.", "мой номер [телефон].", []string{Phone}},
		{"phone compact", "+79161234567", "[телефон]", []string{Phone}},
		{"phone dots", "тел.: 8.916.123.45.67", "тел.: [телефон]", []string{Phone}},
		{"email", "пишите на survey@example.invalid, отвечу", "пишите на [e-mail], отвечу", []string{Email}},
		{"snils dashes", "СНИЛС 112-233-445 95", "СНИЛС [СНИЛС]", []string{SNILS}},
		{"snils plain", "номер 11223344595 в анкете", "номер [СНИЛС] в анкете", []string{SNILS}},
		{"passport", "паспорт 4510 123456 выдан", "паспорт [паспорт] выдан", []string{Passport}},
		{"passport compact", "серия-номер 4510123456", "серия-номер [паспорт]", []string{Passport}},
		{"full name", "Иванова Анна Петровна помогла с отчётом", "[ФИО] помогла с отчётом", []string{Name}},
		{"inflected", "Спасибо Ивановой и Петру Сидорову", "Спасибо [ФИО] и [ФИО]", []string{Name}},
		{"participant id", "наставник M-01 часто занят", "наставник [ФИО] часто занят", []string{Name}},
		{"mixed", "Анна (anna@x.ru, +7-916-123-45-67)", "[ФИО] ([e-mail], [телефон])", []string{Email, Phone, Name}},
		{"clean", "Задачи однотипные: 3 отчёта за 2 дня.", "Задачи однотипные: 3 отчёта за 2 дня.", nil},
		{"no false name", "Иван сделал 5 задач, аннотация готова", "Иван сделал 5 задач, аннотация готова", nil},
		{"other gender form is masked too", "Иванов помог", "[ФИО] помог", []string{Name}},
		{"year is not phone", "в 2026 году 12 задач", "в 2026 году 12 задач", nil},
		{"phone 7 no plus", "звони 7 916 123 45 67", "звони [телефон]", []string{Phone}},
		{"phone 7 compact", "тел 79161234567", "тел [телефон]", []string{Phone}},
		{"phone nbsp dash", "8\u00a0916\u00a0123\u201145\u201167", "[телефон]", []string{Phone}},
		{"phone intl", "London: +44 20 7946 0958, ok", "London: [телефон], ok", []string{Phone}},
		{"phone intl brackets", "+1 (415) 555-26-71", "[телефон]", []string{Phone}},
		{"short plus number", "оценка +1 за задачу", "оценка +1 за задачу", nil},
		{"telegram", "пиши в @anna_ivanova или @mgts_hr.", "пиши в [ник] или [ник].", []string{Handle}},
		{"telegram start", "@ivan99 знает", "[ник] знает", []string{Handle}},
		{"short at is not handle", "встреча @10", "встреча @10", nil},
		{"email not handle", "a.b@example.invalid", "[e-mail]", []string{Email}},
		{"id nb hyphen", "наставник M\u201101 занят", "наставник [ФИО] занят", []string{Name}},
		{"id en dash", "стажёр I\u201301 молодец", "стажёр [ФИО] молодец", []string{Name}},
		{"initials after", "отчёт проверила Смирнова Е.В. вчера", "отчёт проверила [ФИО] вчера", []string{Name}},
		{"initials spaced", "Смирнова Е. В.", "[ФИО]", []string{Name}},
		{"initials before", "спасибо А. П. Чехову и Н.А. Римскому-Корсакову", "спасибо [ФИО] и [ФИО]", []string{Name}},
		{"no initials", "См. п. 3 и т. д.", "См. п. 3 и т. д.", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, found := Mask(c.in, names)
			if got != c.want || !reflect.DeepEqual(found, c.found) {
				t.Fatalf("Mask(%q) = %q %v, want %q %v", c.in, got, found, c.want, c.found)
			}
			if c.found != nil && !HasMask(got) {
				t.Fatal("HasMask must see the tag")
			}
		})
	}
	if got, _ := Mask("Анна", nil); got != "Анна" {
		t.Fatal("no dictionary — no name masking")
	}
	// словарные имена — целым словом с падежным окончанием, обычные слова не задеваются
	dict := []string{"Вера", "Мира", "Илья", "Мария"}
	for in, want := range map[string]string{
		"Вера сказала, что всё верно":      "[ФИО] сказала, что всё верно",
		"спросите Веру или Верой займётся": "спросите [ФИО] или [ФИО] займётся",
		"верх, вероятно, вернуть":          "верх, вероятно, вернуть",
		"мир и миру: Мира":                 "мир и [ФИО]: [ФИО]",
		"от Ильи и Марии, с Марией":        "от [ФИО] и [ФИО], с [ФИО]",
		"ильм, мариновать, верность, иль":  "ильм, мариновать, верность, иль",
	} {
		if got, _ := Mask(in, dict); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestLoadNames(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name, body string
		want       []string
		err        bool
	}{
		{"array", `["Иванова Анна"]`, []string{"Иванова Анна"}, false},
		{"object", `{"names":["Пётр Сидоров"]}`, []string{"Пётр Сидоров"}, false},
		{"broken", `{`, nil, true},
	}
	for _, c := range cases {
		p := filepath.Join(dir, c.name+".json")
		if err := os.WriteFile(p, []byte(c.body), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := LoadNames(p)
		if (err != nil) != c.err || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v %v", c.name, got, err)
		}
	}
	if got, err := LoadNames(filepath.Join(dir, "none.json")); err != nil || got != nil {
		t.Fatal("missing file must be empty, not an error")
	}
	if _, err := LoadNames(dir); err == nil || strings.Contains(err.Error(), "not exist") {
		t.Fatal("directory must be an error")
	}
}

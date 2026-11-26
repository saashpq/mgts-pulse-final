// Package textai разбирает текстовые комментарии анкет: тональность, темы из закрытого списка и цитата-основание.
// Числа считают правила (package signals); ИИ нужен только там, где смысл живёт в тексте.
//
// Два анализатора с одним контрактом:
//   - Offline — словарь тем на Go: работает без сети и GPU, детерминирован, служит запасным путём;
//   - LLM — OpenAI-совместимая модель (vLLM/Ollama в контуре МГТС, GigaChat через gpt2giga).
//     Ответ модели принимается, только если темы из закрытого списка, тональность в диапазоне
//     и цитата ДОСЛОВНО встречается в комментарии. Иначе — офлайн-результат с пометкой.
package textai

import (
	"context"
	"strings"
	"unicode"
)

// Theme — тема из закрытого списка.
type Theme struct {
	Code     string
	Label    string
	Negative bool
	// AboutOther — тема говорит о второй стороне пары (жалоба стажёра на наставника или наоборот).
	AboutOther bool
	markers    []string // основы слов и фразы в нижнем регистре, «ё» → «е»
	exclude    []string // если встречается — тема снимается («нагрузку снизили» — не перегрузка)
}

// Themes — закрытый словарь тем. Модель не может придумать новую тему: это защищает от галлюцинаций
// и делает сигналы сравнимыми между месяцами.
var Themes = []Theme{
	{Code: "routine", Label: "рутина, однотипные задачи", Negative: true,
		markers: []string{"рутин", "однотипн", "одно и то же", "скучн", "не меняются", "только перезапуск"}},
	{Code: "no_growth", Label: "нет развития", Negative: true,
		markers: []string{"не вижу развития", "нового не уч", "не расту", "нет развития", "некуда расти", "задачи посложнее"}},
	{Code: "overload", Label: "перегрузка", Negative: true,
		markers: []string{"перегруж", "перегрузк", "перерабатыва", "устал", "не успеваю", "выгоран", "по вечерам", "снизить нагрузку", "сократить нагрузку"},
		exclude: []string{"снизили", "перераспредел", "стало легче", "в порядке"}},
	{Code: "unclear", Label: "непонятные задачи", Negative: true,
		markers: []string{"непонятно", "без описания", "не понимаю", "что ждут", "не понимает требован"}},
	{Code: "no_support", Label: "не хватает поддержки наставника", Negative: true, AboutOther: true,
		markers: []string{"наставник часто занят", "наставник занят", "не отвечает", "с задержкой", "через несколько дней", "брошен", "торопится", "почти не общаемся", "не хватает регулярной обратной связи", "не хватает регулярной связи с наставником", "редкая обратная связь", "обратной связи все еще недостаточно", "не хватает возможности быстро задать вопрос наставнику"}},
	{Code: "conflict", Label: "конфликт в паре", Negative: true, AboutOther: true,
		markers: []string{"глупые вопросы", "жалуется вместо работы", "некогда вести", "не вижу его в команде"}},
	{Code: "attrition", Label: "риск ухода", Negative: true,
		markers: []string{"стоит ли продолжать", "не останусь", "не буду продолжать стажировку"}},
	{Code: "deadlines", Label: "срывы сроков, медленно", Negative: true, AboutOther: true,
		markers: []string{"сроки срываются", "срывает срок", "медленно", "медленнее", "не успевает"}},
	{Code: "quality", Label: "ошибки в работе", Negative: true, AboutOther: true,
		markers: []string{"ошибк", "переделывать", "качество не растет"}},
	{Code: "passivity", Label: "мало самостоятельности", Negative: true, AboutOther: true,
		markers: []string{"не пытается искать", "не ищет", "ждет разжеванных", "не тянет самостоятельн", "много спрашивает", "много вопросов"}},
	{Code: "praise", Label: "довольны работой", Negative: false,
		markers: []string{"отличн", "доволен", "довольна", "нравится", "понравилось", "рекомендую", "хороший отзыв", "сильный", "толковый", "уверенно", "интересн", "всё хорошо", "все хорошо", "стало легче"}},
	{Code: "growth", Label: "рост и самостоятельность", Negative: false,
		markers: []string{"освоил", "самостоятельнее", "сама предложила", "сам предложил", "раньше срока", "вижу, куда расти", "хочу остаться", "учится быстро", "быстро вникает"}},
}

// ThemeByCode — тема по коду.
func ThemeByCode(code string) (Theme, bool) {
	for _, t := range Themes {
		if t.Code == code {
			return t, true
		}
	}
	return Theme{}, false
}

// Analysis — результат разбора одного комментария.
type Analysis struct {
	Source     *Source  `json:"source,omitempty"`
	HRQuestion string   `json:"hr_question,omitempty"`
	Sentiment  float64  `json:"sentiment"` // −1..1
	Themes     []string `json:"themes"`
	Quote      string   `json:"quote"` // дословный фрагмент комментария, на котором основан вывод
	// ByTheme — цитата под каждую тему (офлайн-анализатор знает, какое предложение дало тему).
	ByTheme  map[string]string `json:"by_theme,omitempty"`
	Analyzer string            `json:"analyzer"`
	Note     string            `json:"note,omitempty"` // почему ответ модели отброшен
	// Masked — типы персональных данных, скрытых перед отправкой в модель («телефон», «e-mail», «ФИО»…).
	// Сам отправленный текст не хранится.
	Masked []string `json:"masked,omitempty"`
	// Model и PromptHash — какой моделью и какой версией инструкции получен разбор (только для ответа модели).
	Model      string `json:"model,omitempty"`
	PromptHash string `json:"prompt_hash,omitempty"`
	// LLMThemes — темы, которые назвала сама модель, если к ним добавлены критичные темы словаря (Note = NoteMerged).
	LLMThemes []string `json:"llm_themes,omitempty"`
	// OfflineVersion — версия правил для пересчёта сохранённых словарных разборов.
	OfflineVersion int `json:"offline_version,omitempty"`
}

// Analyzer — общий контракт офлайн- и LLM-анализатора.
type Analyzer interface {
	Analyze(ctx context.Context, text, role string) Analysis
	Name() string
}

// Offline — словарный анализатор.
type Offline struct{}

const OfflineVersion = 4

func (Offline) Name() string { return "offline" }

func normText(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "ё", "е")
}

// Analyze ищет маркеры тем; тональность — баланс положительных и отрицательных тем.
func (Offline) Analyze(_ context.Context, text, _ string) Analysis {
	a := Analysis{Analyzer: "offline", OfflineVersion: OfflineVersion, Themes: []string{}, ByTheme: map[string]string{}}
	sentences := splitSentences(text)
	var pos, neg int
	for _, t := range Themes {
		for _, sentence := range sentences {
			if !matchesTheme(t, normText(sentence)) {
				continue
			}
			a.Themes = append(a.Themes, t.Code)
			a.ByTheme[t.Code] = sentence
			if t.Negative {
				if neg == 0 {
					a.Quote = sentence
				}
				neg++
			} else {
				if a.Quote == "" {
					a.Quote = sentence
				}
				pos++
			}
			break
		}
	}
	switch {
	case neg+pos == 0:
		a.Sentiment = 0
	default:
		a.Sentiment = round2(float64(pos-neg) / float64(pos+neg))
	}
	return a
}

func containsAny(s string, subs []string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

func splitSentences(text string) []string {
	var out []string
	start := 0
	rs := []rune(text)
	for i, r := range rs {
		if r == '.' || r == '!' || r == '?' || r == '\n' {
			if s := strings.TrimSpace(string(rs[start : i+1])); s != "" {
				out = append(out, s)
			}
			start = i + 1
		}
	}
	if s := strings.TrimSpace(string(rs[start:])); s != "" {
		out = append(out, s)
	}
	return out
}

func round2(f float64) float64 {
	if f < 0 {
		return -float64(int(-f*100+0.5)) / 100
	}
	return float64(int(f*100+0.5)) / 100
}

// Words — число слов (для сигнала «комментарии стали короткими»).
func Words(s string) int {
	return len(strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }))
}

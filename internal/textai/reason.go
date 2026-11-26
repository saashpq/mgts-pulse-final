package textai

import "strings"

// Заметки разбора (Analysis.Note), которые не означают отказ модели.
const (
	// NoteMerged — модель не нашла критичную тему, которую нашёл словарь: темы объединены (приоритет полноты).
	NoteMerged = "расхождение анализаторов: критичная тема сохранена"
	// NoteUnavailable — модель не ответила.
	NoteUnavailable = "модель недоступна, использован словарь"
	// NoteMaskedQuote — цитата модели содержит скрытые ПД.
	NoteMaskedQuote = "цитата модели содержит скрытые персональные данные, использован словарь"
	// NoteRejectedPrefix — начало заметки об отклонённом ответе модели.
	NoteRejectedPrefix = "ответ модели отклонён: "
)

// Reason — причина отказа от ответа модели по заметке разбора (для экрана «Качество системы»):
// «модель недоступна», «не JSON», «тема вне списка», «цитаты нет в комментарии», «ПД в цитате»…
// Пусто — заметка не означает отказ (пусто, ожидание модели, объединение тем).
func Reason(note string) string {
	switch {
	case note == NoteUnavailable:
		return "модель недоступна"
	case note == NoteMaskedQuote:
		return "ПД в цитате"
	case strings.HasPrefix(note, NoteRejectedPrefix):
		r := strings.TrimPrefix(note, NoteRejectedPrefix)
		if strings.HasPrefix(r, "тема ") && strings.HasSuffix(r, "не из списка") {
			return "тема вне списка"
		}
		return r
	}
	return ""
}

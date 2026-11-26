package signals

import (
	"regexp"
	"strings"

	"github.com/ElzaraA/mgts-ai/internal/textai"
)

var feedbackBoundary = regexp.MustCompile(`[,;:!?\n]|\s+[—–]\s+|\s+(?:но|зато|однако|когда|потому что|хотя|если)\s+`)

// Проверяем, что цитата указывает на вторую сторону пары. Жалоба на нагрузку
// или обращение постороннего коллеги сама по себе не означает конфликт пары.
func directedFeedback(role, quote string, theme textai.Theme) bool {
	text := strings.ReplaceAll(strings.ToLower(quote), "ё", "е")
	parts := feedbackBoundary.Split(strings.TrimRight(text, ".!?"), -1)
	for _, part := range parts {
		other := strings.Contains(part, "стажер") || strings.Contains(part, "практикант")
		if role == "intern" {
			other = strings.Contains(part, "наставник") || strings.Contains(part, "наставниц") || strings.Contains(part, "ментор")
		}
		if other && (len(parts) == 1 || theme.MatchesClause(part)) {
			return true
		}
	}
	return false
}

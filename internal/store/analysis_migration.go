package store

import (
	"context"

	"github.com/ElzaraA/mgts-ai/internal/textai"
)

// Результаты словаря можно воспроизвести локально при открытии старого снимка.
// Сохраняем причину отката и сведения об отправке в модель для журнала качества.
func refreshOfflineAnalyses(st *state) {
	for _, r := range st.Responses {
		for code, comment := range r.Comments {
			key := r.ID + "/" + code
			old := st.Analyses[key]
			if old.Analyzer != "offline" || old.OfflineVersion >= textai.OfflineVersion {
				continue
			}
			fresh := textai.Offline{}.Analyze(context.Background(), comment, string(r.Role))
			fresh.Note, fresh.Masked, fresh.PromptHash = old.Note, old.Masked, old.PromptHash
			st.Analyses[key] = fresh
		}
	}
}

package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/ElzaraA/mgts-ai/internal/textai"
)

// CheckSample — тестовый комментарий для проверки модели: в нём три темы словаря (рутина, нет поддержки, риск ухода).
const CheckSample = "Задачи однотипные, наставник не отвечает в чате, думаю, стоит ли продолжать."

// OfflineCheckError — ответ проверки, когда модель не настроена.
const OfflineCheckError = "Модель не подключена: задайте LLM_BASE_URL, LLM_MODEL, LLM_API_KEY"

type llmCheck struct {
	Mode      string           `json:"mode"`
	Model     string           `json:"model"`
	OK        bool             `json:"ok"`
	LatencyMS *int64           `json:"latency_ms"`
	Sample    string           `json:"sample"`
	Analysis  *textai.Analysis `json:"analysis"`
	Error     *string          `json:"error"`
}

// llmCheckTimeout — сколько ждать модель при проверке (холодный старт локальной модели бывает долгим).
var llmCheckTimeout = 60 * time.Second

// llmCheckHandler — разбор тестового комментария тем же путём, что и в работе: маскирование, закрытый список тем,
// проверка цитаты. Для зелёной проверки модель должна сама распознать темы контрольной фразы.
func (a *App) llmCheckHandler(w http.ResponseWriter, r *http.Request) {
	out := llmCheck{Mode: "offline", Sample: CheckSample}
	if a.LLM == nil {
		msg := OfflineCheckError
		out.Error = &msg
		writeJSON(w, 200, out)
		return
	}
	// проверки по одной: модель в контуре — общий ресурс, двойной клик не должен ставить два запроса
	a.checkMu.Lock()
	defer a.checkMu.Unlock()
	out.Mode, out.Model = "llm", a.Cfg.LLMModel
	ctx, cancel := context.WithTimeout(r.Context(), llmCheckTimeout)
	defer cancel()
	start := time.Now()
	an := a.LLM.Analyze(ctx, CheckSample, "intern")
	ms := time.Since(start).Milliseconds()
	an.ByTheme = nil
	out.LatencyMS, out.Analysis = &ms, &an
	if strings.HasPrefix(an.Analyzer, "llm:") {
		themes := an.Themes
		if an.Note == textai.NoteMerged {
			themes = an.LLMThemes
		}
		expected := []string{"routine", "no_support", "attrition"}
		out.OK = len(themes) == len(expected) && an.Sentiment < 0
		for _, theme := range expected {
			out.OK = out.OK && slices.Contains(themes, theme)
		}
		if !out.OK {
			var names []string
			for _, code := range themes {
				if t, ok := textai.ThemeByCode(code); ok {
					names = append(names, t.Label)
				}
			}
			reported := strings.Join(names, ", ")
			if reported == "" {
				reported = "нет тем"
			}
			msg := "Темы в ответе самой модели: " + reported + ". Контрольный пример предполагает негативную тональность и три темы: рутина, нехватка поддержки и риск ухода."
			out.Error = &msg
		}
	} else {
		msg := an.Note
		if msg == "" {
			msg = "ответ модели не принят"
		}
		out.Error = &msg
	}
	a.Metrics.Inc("pulse_llm_checks_total", "Проверки модели", "ok", map[bool]string{true: "true", false: "false"}[out.OK])
	writeJSON(w, 200, out)
}

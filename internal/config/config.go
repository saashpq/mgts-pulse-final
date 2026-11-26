// Package config читает настройки из окружения (и .env через LoadDotEnv).
package config

import (
	"net"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Addr          string
	LLMBaseURL    string
	LLMAPIKey     string
	LLMModel      string
	LLMMock       bool // true — только офлайн-анализатор текста, без вызова модели
	DataDir       string
	SurveyFile    string // структура анкет; на финале правится под реальный датасет
	StateFile     string // снимок состояния; пусто — только в памяти
	SeedSynthetic bool   // загрузить синтетический датасет при пустом состоянии
	WebhookSecret string // общий секрет вебхука Яндекс Форм (заголовок X-Webhook-Secret)
	CORSOrigins   []string
	PublicURL     string // адрес интерфейса для ссылок на анкеты
	DemoMode      bool   // разрешает POST /api/v1/demo/reset; по умолчанию выключен
	HRToken       string // если задан — HR-эндпоинты /api/v1/* требуют Authorization: Bearer <HR_TOKEN>
	WebDir        string // собранный фронтенд, который сервер отдаёт сам; пусто — только API
}

func Load() Config {
	key := os.Getenv("LLM_API_KEY")
	data := getenv("DATA_DIR", "data")
	c := Config{
		// по умолчанию только локальный интерфейс: прототип без входа не должен быть виден в сети
		Addr:       getenv("ADDR", "127.0.0.1:8080"),
		LLMBaseURL: getenv("LLM_BASE_URL", "http://localhost:8000/v1"),
		LLMAPIKey:  key,
		LLMModel:   getenv("LLM_MODEL", "Qwen2.5-7B-Instruct"),
		// Без ключа модель не вызываем: демо обязано работать без сети.
		LLMMock:       os.Getenv("LLM_MOCK") == "true" || key == "",
		DataDir:       data,
		SurveyFile:    getenv("SURVEY_FILE", filepath.Join(data, "survey.json")),
		StateFile:     os.Getenv("STATE_FILE"),
		SeedSynthetic: os.Getenv("SEED_SYNTHETIC") != "false",
		WebhookSecret: os.Getenv("WEBHOOK_SECRET"),
		CORSOrigins:   splitList(getenv("CORS_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")),
		PublicURL:     strings.TrimRight(getenv("PUBLIC_URL", "http://localhost:5173"), "/"),
		DemoMode:      os.Getenv("DEMO_MODE") == "true",
		HRToken:       os.Getenv("HR_TOKEN"),
		WebDir:        ResolveWebDir(os.Getenv("WEB_DIR")),
	}
	// один процесс (фронтенд из WEB_DIR): ссылки на анкеты ведут на этот же сервер, если PUBLIC_URL не задан явно
	if c.WebDir != "" && os.Getenv("PUBLIC_URL") == "" {
		c.PublicURL = SelfURL(c.Addr)
	}
	return c
}

// SelfURL — адрес сервера для браузера: «:8080» и «0.0.0.0:8080» → http://localhost:8080.
func SelfURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://localhost:8080"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// ResolveWebDir — каталог статики: заданный WEB_DIR, иначе frontend/dist, затем web. Пусто — каталога нет.
func ResolveWebDir(explicit string) string {
	candidates := []string{filepath.Join("frontend", "dist"), "web"}
	if explicit != "" {
		candidates = []string{explicit}
	}
	for _, d := range candidates {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	return ""
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

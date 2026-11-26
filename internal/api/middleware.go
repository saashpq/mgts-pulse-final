package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ElzaraA/mgts-ai/internal/metrics"
)

// Перенесено из AI Buddy и упрощено до стандартной библиотеки (без chi): проект собирается офлайн.

func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]apiError{"error": {Code: code, Message: msg}})
}

func badBody(w http.ResponseWriter, err error) {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "слишком большой запрос")
		return
	}
	writeError(w, http.StatusBadRequest, "bad_request", "некорректный JSON")
}

// Recover — паника обработчика не роняет сервер и не раскрывает внутренности.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				slog.Error("panic", "route", routeOf(r.URL.Path), "err", v)
				writeError(w, 500, "internal", "внутренняя ошибка")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// CORS — только перечисленные origin интерфейса.
func CORS(origins []string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range origins {
		allowed[strings.TrimRight(o, "/")] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if o := r.Header.Get("Origin"); o != "" && (allowed[o] || allowed["*"]) {
				w.Header().Set("Access-Control-Allow-Origin", o)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Webhook-Secret")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// HRAuth — необязательный общий токен HR. Пустой token — проверки нет (локальный прототип, как раньше).
// Без токена открыты: анкета по персональной ссылке (/api/v1/form/*), вебхук со своим секретом
// (/api/v1/integrations/*), /health и /metrics. Поток /api/v1/events принимает токен и в ?token=,
// потому что EventSource в браузере не умеет передавать заголовки. Сравнение — за постоянное время.
func HRAuth(token string) func(http.Handler) http.Handler {
	want := []byte(token)
	return func(next http.Handler) http.Handler {
		if token == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := r.URL.Path
			if !strings.HasPrefix(p, "/api/v1/") || strings.HasPrefix(p, "/api/v1/form/") || strings.HasPrefix(p, "/api/v1/integrations/") {
				next.ServeHTTP(w, r)
				return
			}
			got := ""
			if h := r.Header.Get("Authorization"); len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
				got = strings.TrimSpace(h[7:])
			} else if p == "/api/v1/events" {
				got = r.URL.Query().Get("token")
			}
			if subtle.ConstantTimeCompare([]byte(got), want) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="pulse"`)
				writeError(w, http.StatusUnauthorized, "unauthorized", "нужен токен HR: Authorization: Bearer <HR_TOKEN>")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SameOriginWrites — защита от CSRF: запись с чужого Origin отклоняется. Свой origin (интерфейс отдаёт
// этот же сервер из WEB_DIR) разрешён всегда.
func SameOriginWrites(origins []string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range origins {
		allowed[strings.TrimRight(o, "/")] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				if o := r.Header.Get("Origin"); o != "" && !allowed[o] && !allowed["*"] && !sameOrigin(o, r) {
					writeError(w, http.StatusForbidden, "forbidden", "запрос с чужого сайта заблокирован")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(c int) { s.status = c; s.ResponseWriter.WriteHeader(c) }
func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Observe — лог запроса без тела (там ответы анкет) и метрики по шаблону маршрута.
func Observe(m *metrics.Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w, status: 200}
			start := time.Now()
			next.ServeHTTP(sw, r)
			route := routeOf(r.URL.Path)
			m.Inc("pulse_http_requests_total", "HTTP-запросы", "route", route, "status", strconv.Itoa(sw.status))
			if !strings.HasSuffix(route, "/events") {
				slog.Info("http", "method", r.Method, "route", route, "status", sw.status, "ms", time.Since(start).Milliseconds())
			}
		})
	}
}

// routeOf — путь без идентификаторов, чтобы метрики не разрастались по числу пар и токенов.
func routeOf(p string) string {
	if strings.HasPrefix(p, "/api/v1/follow-ups/") {
		return "/api/v1/follow-ups/{signal}/{episode}"
	}
	for _, prefix := range []string{"/api/v1/pairs/", "/api/v1/form/", "/api/v1/signals/", "/form/", "/pairs/"} {
		if strings.HasPrefix(p, prefix) {
			rest := strings.TrimPrefix(p, prefix)
			if i := strings.IndexByte(rest, '/'); i >= 0 {
				return prefix + "{id}" + rest[i:]
			}
			return prefix + "{id}"
		}
	}
	return p
}

// sameOrigin — Origin запроса указывает на этот же сервер (схема http/https, хост и порт из Host).
func sameOrigin(origin string, r *http.Request) bool {
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && strings.EqualFold(u.Host, r.Host)
}

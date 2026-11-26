package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ElzaraA/mgts-ai/internal/store"
	"github.com/ElzaraA/mgts-ai/internal/survey"
)

// formGet — анкета по персональной ссылке. Аналитика респонденту недоступна: только вопросы и номер месяца.
func (a *App) formGet(w http.ResponseWriter, r *http.Request) {
	person, ok := a.Store.Person(r.PathValue("token"))
	if !ok {
		writeError(w, 404, "not_found", "ссылка недействительна")
		return
	}
	sv := a.Store.Survey()
	_, role, ok := sv.PairOf(person)
	if !ok {
		writeError(w, 404, "not_found", "участник не найден")
		return
	}
	window := a.Store.FormWindow()
	writeJSON(w, 200, map[string]any{"title": sv.Title, "role": role, "person": person,
		"questions": sv.ByRole(role), "period": window.Period, "periods": sv.Periods, "closed": window.Closed})
}

func (a *App) setFormWindow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Period int   `json:"period"`
		Closed *bool `json:"closed"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		badBody(w, err)
		return
	}
	if req.Closed == nil {
		writeError(w, 400, "bad_request", "укажите статус приёма ответов")
		return
	}
	window := store.FormWindow{Period: req.Period, Closed: *req.Closed}
	if err := a.Store.SetFormWindow(window); err != nil {
		if errors.Is(err, store.ErrWindowPeriod) {
			writeError(w, 400, "bad_request", err.Error())
		} else {
			writeError(w, 500, "internal", "не удалось сохранить настройки приёма")
		}
		return
	}
	a.Broker.Publish(SSEEvent{Event: "data.updated", Data: "{}"})
	writeJSON(w, 200, window)
}

type answersBody struct {
	Period  int            `json:"period"`
	Answers map[string]any `json:"answers"`
}

func (a *App) formPost(w http.ResponseWriter, r *http.Request) {
	person, ok := a.Store.Person(r.PathValue("token"))
	if !ok {
		writeError(w, 404, "not_found", "ссылка недействительна")
		return
	}
	var body answersBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&body); err != nil {
		badBody(w, err)
		return
	}
	resp, err := a.accept(person, body, "form", "")
	if err != nil {
		acceptError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "period": resp.Period})
}

// webhook — приём ответа из Яндекс Форм (HTTP-интеграция формы). Тело собирается шаблоном в настройках формы:
// {"token": "...", "period": 4, "answers": {"i_overall": "4", "i_comment": "..."}}. Вместо token можно participant.
func (a *App) webhook(w http.ResponseWriter, r *http.Request) {
	if a.Cfg.WebhookSecret == "" {
		writeError(w, 503, "not_configured", "WEBHOOK_SECRET не задан — вебхук выключен")
		return
	}
	got := r.Header.Get("X-Webhook-Secret")
	if subtle.ConstantTimeCompare([]byte(got), []byte(a.Cfg.WebhookSecret)) != 1 {
		writeError(w, 401, "unauthorized", "неверный секрет")
		return
	}
	// Яндекс Формы повторяют запрос при таймауте или ошибке (до 7 раз за 30 минут) с тем же X-Delivery-Id:
	// повтор подтверждаем, но данные не трогаем.
	delivery := strings.TrimSpace(r.Header.Get("X-Delivery-Id"))
	if len(delivery) > 200 {
		writeError(w, 400, "bad_request", "X-Delivery-Id длиннее 200 символов")
		return
	}
	duplicate := func() {
		a.Metrics.Inc("pulse_webhook_duplicates_total", "Повторные доставки вебхука")
		writeJSON(w, 200, map[string]any{"ok": true, "duplicate": true})
	}
	if delivery != "" && a.Store.HasDelivery(delivery) {
		duplicate()
		return
	}
	var body struct {
		answersBody
		Token       string `json:"token"`
		Participant string `json:"participant"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&body); err != nil {
		badBody(w, err)
		return
	}
	person := body.Participant
	if body.Token != "" {
		p, ok := a.Store.Person(body.Token)
		if !ok {
			writeError(w, 404, "not_found", "неизвестный токен")
			return
		}
		person = p
	}
	resp, err := a.accept(person, body.answersBody, "webhook", delivery)
	if errors.Is(err, store.ErrDuplicateDelivery) {
		duplicate()
		return
	}
	if err != nil {
		acceptError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "id": resp.ID})
}

// errSave — ответ корректен, но снимок состояния не записался: клиенту 500, чтобы он повторил.
var errSave = errors.New("не удалось сохранить ответ, повторите позже")

func acceptError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrWindowChanged) {
		writeError(w, http.StatusConflict, "window_changed", err.Error())
		return
	}
	if errors.Is(err, errSave) {
		writeError(w, 500, "internal", err.Error())
		return
	}
	writeError(w, 400, "bad_request", err.Error())
}

// accept проверяет ответы по структуре анкеты и сохраняет их. Повторная отправка за тот же месяц заменяет ответ.
func (a *App) accept(person string, body answersBody, source, delivery string) (survey.Response, error) {
	sv := a.Store.Survey()
	pair, role, ok := sv.PairOf(person)
	if !ok {
		return survey.Response{}, fmt.Errorf("участник не найден")
	}
	period := body.Period
	if period == 0 {
		period = a.Store.FormWindow().Period
	}
	if period < 1 || period > sv.Periods {
		return survey.Response{}, fmt.Errorf("месяц вне диапазона 1..%d", sv.Periods)
	}
	resp := survey.Response{ID: fmt.Sprintf("%s-%s-M%d", pair.ID, role, period), Role: role, Person: person, PairID: pair.ID,
		Period: period, Scores: map[string]float64{}, Comments: map[string]string{}, Source: source}
	for _, q := range sv.ByRole(role) {
		raw, present := body.Answers[q.Code]
		if !present || raw == nil {
			if q.Kind == "scale" {
				return survey.Response{}, fmt.Errorf("нет ответа на вопрос «%s»", q.Text)
			}
			continue
		}
		s := strings.TrimSpace(fmt.Sprint(raw))
		if q.Kind == "text" {
			if utf8.RuneCountInString(s) > 2000 {
				return survey.Response{}, fmt.Errorf("комментарий длиннее 2000 символов")
			}
			if s != "" {
				resp.Comments[q.Code] = s
			}
			continue
		}
		v, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64)
		if err != nil || v < 1 || v > q.Max || v != float64(int(v)) {
			return survey.Response{}, fmt.Errorf("«%s»: целое от 1 до %g", q.Text, q.Max)
		}
		resp.Scores[q.Code] = v
	}
	for code := range body.Answers {
		if q, ok := sv.Q(code); !ok || q.Role != role {
			return survey.Response{}, fmt.Errorf("неизвестный вопрос %q", code)
		}
	}
	changed, err := a.Store.SaveSubmission(resp, "Ответ "+person+" за M"+strconv.Itoa(period), delivery)
	if errors.Is(err, store.ErrWindowChanged) || errors.Is(err, store.ErrDuplicateDelivery) {
		return survey.Response{}, err
	}
	if err != nil {
		slog.Error("response not saved", "source", source, "id", resp.ID, "err", err)
		return survey.Response{}, errSave
	}
	a.analyze(changed)
	a.Metrics.Inc("pulse_responses_total", "Принятые ответы анкет", "source", source)
	return resp, nil
}

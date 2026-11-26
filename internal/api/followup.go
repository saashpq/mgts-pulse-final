package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/ElzaraA/mgts-ai/internal/store"
)

func (a *App) followUpQueue(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.Store.FollowUpQueue())
}

func (a *App) putFollowUp(w http.ResponseWriter, r *http.Request) {
	signal := r.PathValue("signal")
	episode, err := strconv.Atoi(r.PathValue("episode"))
	if err != nil || episode < 1 {
		writeError(w, 400, "bad_request", "некорректный эпизод")
		return
	}
	var body store.FollowUpCommand
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		badBody(w, err)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, 400, "bad_request", "ожидается один JSON-объект")
		return
	}
	var pair, title string
	if body.Operation == "plan" && body.Revision != nil && *body.Revision == 0 {
		for _, p := range a.report().Pairs {
			for _, s := range append(p.Signals, p.Resolved...) {
				if s.ID == signal && s.Episode == episode {
					pair, title = s.Pair, s.Title
					break
				}
			}
			if pair != "" {
				break
			}
		}
	}
	task, changed, err := a.Store.UpdateFollowUp(signal, episode, pair, title, body)
	switch {
	case errors.Is(err, store.ErrFollowUpNotFound):
		writeError(w, 404, "not_found", err.Error())
	case errors.Is(err, store.ErrFollowUpConflict):
		writeError(w, 409, "conflict", err.Error())
	case errors.Is(err, store.ErrFollowUpInvalid):
		writeError(w, 400, "bad_request", err.Error())
	case err != nil:
		writeError(w, 500, "internal", "не удалось сохранить контрольный контакт")
	default:
		if changed {
			a.Broker.Publish(SSEEvent{Event: "data.updated", Data: `{"follow_up":true}`})
		}
		writeJSON(w, 200, task)
	}
}

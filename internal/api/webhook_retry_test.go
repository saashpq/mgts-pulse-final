package api

import (
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/config"
	"github.com/ElzaraA/mgts-ai/internal/store"
	"github.com/ElzaraA/mgts-ai/internal/survey"
)

type beforeRead struct {
	begin func()
	io.Reader
}

func (r *beforeRead) Read(p []byte) (int, error) {
	if r.begin != nil {
		r.begin()
		r.begin = nil
	}
	return r.Reader.Read(p)
}

func TestWebhookRetrySnapshotDuringUnfinishedDelivery(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	sv := survey.Default()
	cfg := config.Config{WebhookSecret: "test"}
	app := New(cfg, store.New(sv, file), sv, nil)
	const endpoint = "/api/v1/integrations/yandex-forms"
	body := m5("1")
	checkRestart := func(wantDuplicate bool) {
		reopened, _, err := store.Open(sv, false, file)
		if err != nil {
			t.Fatal(err)
		}
		restarted := New(cfg, reopened, sv, nil)
		w := do(restarted.Handler(), "POST", endpoint, body, "X-Webhook-Secret", "test", "X-Delivery-Id", "retry")
		if w.Code != 200 || mentorOverall(restarted) != 1 || strings.Contains(w.Body.String(), "duplicate") != wantDuplicate {
			t.Fatalf("retry after restart: %d %s, saved score %v", w.Code, w.Body.String(), mentorOverall(restarted))
		}
	}
	req := httptest.NewRequest("POST", endpoint, &beforeRead{func() { checkRestart(false) }, strings.NewReader(body)})
	req.Header.Set("X-Webhook-Secret", "test")
	req.Header.Set("X-Delivery-Id", "retry")
	app.Handler().ServeHTTP(httptest.NewRecorder(), req)
	checkRestart(true)
}

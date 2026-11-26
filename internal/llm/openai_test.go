package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAIComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hi"}}]}`))
	}))
	defer srv.Close()

	c := &OpenAIClient{BaseURL: srv.URL, APIKey: "k", Model: "m", HTTP: srv.Client()}
	out, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}})
	if err != nil || out != "hi" {
		t.Fatalf("got %q err %v", out, err)
	}
}

func TestOpenAIStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	c := &OpenAIClient{BaseURL: srv.URL, HTTP: srv.Client(), Backoff: time.Millisecond}
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatal("must error")
	}
}

func TestOpenAIEmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()
	c := &OpenAIClient{BaseURL: srv.URL, HTTP: srv.Client(), Backoff: time.Millisecond}
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}})
	if err == nil {
		t.Fatal("must error on empty choices")
	}
}

func TestOpenAIBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	c := &OpenAIClient{BaseURL: srv.URL, HTTP: srv.Client(), Backoff: time.Millisecond}
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}})
	if err == nil {
		t.Fatal("must error on bad json")
	}
}

// Детерминизм: temperature 0 и seed (по умолчанию 42) уходят в каждом запросе.
func TestOpenAISeedAndTemperature(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()
	c := &OpenAIClient{BaseURL: srv.URL, HTTP: srv.Client()}
	if _, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"seed":42`) || !strings.Contains(body, `"temperature":0`) {
		t.Fatal(body)
	}
	c.Seed = 7
	_, _ = c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}})
	if !strings.Contains(body, `"seed":7`) {
		t.Fatal(body)
	}
}

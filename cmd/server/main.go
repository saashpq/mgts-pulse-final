// Сервер «Пульса стажировки»: HR-дашборд программы стажировок МГТС с ИИ-разбором комментариев.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ElzaraA/mgts-ai/internal/api"
	"github.com/ElzaraA/mgts-ai/internal/config"
	"github.com/ElzaraA/mgts-ai/internal/llm"
	"github.com/ElzaraA/mgts-ai/internal/store"
	"github.com/ElzaraA/mgts-ai/internal/survey"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	config.LoadDotEnv(".env")
	cfg := config.Load()
	if err := run(cfg); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(cfg config.Config) error {
	sv, err := survey.Load(cfg.SurveyFile)
	if err != nil {
		slog.Warn("survey file not loaded, using built-in default", "file", cfg.SurveyFile, "err", err)
	}
	// прочитанный survey.json главнее структуры из снимка: правка анкеты на финале действует без сброса данных
	initial := sv
	if !cfg.SeedSynthetic {
		initial = sv.WithoutDemoPairs()
	}
	st, loaded, err := store.Open(initial, err == nil, cfg.StateFile)
	if err != nil {
		return err
	}
	var client llm.Client
	if !cfg.LLMMock {
		client = &llm.OpenAIClient{BaseURL: cfg.LLMBaseURL, APIKey: cfg.LLMAPIKey, Model: cfg.LLMModel, HTTP: &http.Client{Timeout: 30 * time.Second}}
	}
	app := api.New(cfg, st, sv, client)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app.Start(ctx)
	if !loaded && cfg.SeedSynthetic {
		if err := app.Seed(); err != nil {
			return err
		}
	}
	srv := &http.Server{Addr: cfg.Addr, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 120 * time.Second}
	// открытые SSE-потоки закрываются сразу, иначе Shutdown ждал бы их до таймаута
	srv.RegisterOnShutdown(app.Broker.Close)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	slog.Info("listening", "addr", cfg.Addr, "analyzer", map[bool]string{true: "offline", false: "llm:" + cfg.LLMModel}[cfg.LLMMock],
		"hr_auth", cfg.HRToken != "", "demo", cfg.DemoMode, "web_dir", cfg.WebDir)
	if cfg.WebDir != "" {
		slog.Info("интерфейс отдаётся этим же сервером", "url", config.SelfURL(cfg.Addr))
	}
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shut); err != nil {
			// штатная остановка по сигналу: недождавшиеся соединения — не повод выходить с ошибкой
			slog.Warn("shutdown incomplete", "err", err)
		}
		app.Stop()
		slog.Info("stopped")
		return nil
	}
}

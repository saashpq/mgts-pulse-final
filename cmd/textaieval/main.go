// textaieval — прогон gold-set: офлайн-словарь и (если задан LLM_BASE_URL) модель против ручной разметки.
// Пишет DATA_DIR/goldset_result.json (его показывает экран «Качество системы») и markdown-отчёт.
//
//	go run ./cmd/textaieval -in eval/goldset.csv [-runs 3] [-md DATA_DIR/goldset_result.md]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ElzaraA/mgts-ai/internal/config"
	"github.com/ElzaraA/mgts-ai/internal/goldset"
	"github.com/ElzaraA/mgts-ai/internal/llm"
	"github.com/ElzaraA/mgts-ai/internal/textai"
)

func main() {
	config.LoadDotEnv(".env")
	in := flag.String("in", "eval/goldset.csv", "размеченный CSV (шаблон — eval/goldset_template.csv)")
	runs := flag.Int("runs", 3, "прогонов для проверки детерминизма")
	data := flag.String("data", envOr("DATA_DIR", "data"), "куда положить goldset_result.json")
	md := flag.String("md", "", "markdown-отчёт (по умолчанию DATA_DIR/goldset_result.md)")
	flag.Parse()
	f, err := os.Open(*in)
	if err != nil {
		fail(err)
	}
	items, err := goldset.Read(f)
	f.Close()
	if err != nil {
		fail(err)
	}
	analyzers := []textai.Analyzer{textai.Offline{}}
	model := "offline"
	if base := os.Getenv("LLM_BASE_URL"); base != "" {
		model = envOr("LLM_MODEL", "Qwen2.5-7B-Instruct")
		client := &llm.OpenAIClient{BaseURL: base, APIKey: os.Getenv("LLM_API_KEY"), Model: model, HTTP: &http.Client{Timeout: 60 * time.Second}}
		analyzers = append(analyzers, textai.LLM{Client: client, Model: model, Timeout: 60 * time.Second})
	}
	res, err := goldset.Evaluate(context.Background(), items, analyzers, *runs, time.Now(), model)
	if err != nil {
		fail(err)
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	out := filepath.Join(*data, "goldset_result.json")
	if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
		fail(err)
	}
	if *md == "" {
		*md = filepath.Join(*data, "goldset_result.md")
	}
	mf, err := os.Create(*md)
	if err != nil {
		fail(err)
	}
	if err := goldset.WriteMarkdown(mf, res); err != nil {
		fail(err)
	}
	if err := mf.Close(); err != nil {
		fail(err)
	}
	fmt.Printf("размечено %d из %d; macro-F1 (%s) = %v; записано %s и %s\n", res.Labeled, res.N, res.Primary, deref(res.MacroF1), out, *md)
}

func deref(p *float64) any {
	if p == nil {
		return "—"
	}
	return *p
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "textaieval:", err)
	os.Exit(1)
}

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsAreSafe(t *testing.T) {
	for _, k := range []string{"ADDR", "DEMO_MODE", "HR_TOKEN", "LLM_API_KEY", "LLM_MOCK"} {
		t.Setenv(k, "")
	}
	c := Load()
	if c.Addr != "127.0.0.1:8080" || c.DemoMode || c.HRToken != "" || !c.LLMMock {
		t.Fatalf("%+v", c)
	}
	t.Setenv("ADDR", ":8080")
	t.Setenv("DEMO_MODE", "true")
	t.Setenv("HR_TOKEN", "x")
	c = Load()
	if c.Addr != ":8080" || !c.DemoMode || c.HRToken != "x" {
		t.Fatalf("%+v", c)
	}
}

func TestWebDirAndSelfURL(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
	if ResolveWebDir("") != "" || ResolveWebDir("nope") != "" {
		t.Fatal("no dirs")
	}
	os.Mkdir("web", 0o755)
	if ResolveWebDir("") != "web" {
		t.Fatal("web fallback")
	}
	os.MkdirAll(filepath.Join("frontend", "dist"), 0o755)
	if ResolveWebDir("") != filepath.Join("frontend", "dist") || ResolveWebDir("web") != "web" {
		t.Fatal("frontend/dist first, explicit wins")
	}
	t.Setenv("WEB_DIR", "")
	t.Setenv("PUBLIC_URL", "")
	t.Setenv("ADDR", "127.0.0.1:9090")
	if c := Load(); c.PublicURL != "http://127.0.0.1:9090" || c.SurveyFile != filepath.Join("data", "survey.json") {
		t.Fatalf("%+v", c)
	}
	t.Setenv("PUBLIC_URL", "http://ui/")
	if c := Load(); c.PublicURL != "http://ui" {
		t.Fatal(c.PublicURL)
	}
	for in, want := range map[string]string{":8080": "http://localhost:8080", "0.0.0.0:80": "http://localhost:80", "bad": "http://localhost:8080", "[::1]:8080": "http://[::1]:8080"} {
		if got := SelfURL(in); got != want {
			t.Fatalf("%s: %s", in, got)
		}
	}
}

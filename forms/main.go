// Independent survey service. Receives SurveyJS answers and delivers them to Pulse over HTTP.
package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed public/*
var assets embed.FS
var schema struct {
	Pairs     []struct{ Intern, Mentor, Track string }
	Questions []struct {
		Code, Role, Kind string
		Max              int
	}
	Periods int
}

type submission struct {
	ID, Role, Intern, Mentor, Track string
	Period                          int
	Answers                         map[string]any
}

var guard sync.Mutex
var allowedID = regexp.MustCompile("^[a-f0-9-]{36}$")
var secret string
var client = &http.Client{Timeout: 6 * time.Second}

func setting(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
func valid(s submission) bool {
	if !allowedID.MatchString(s.ID) || (s.Role != "intern" && s.Role != "mentor") || s.Period < 1 || s.Period > schema.Periods {
		return false
	}
	known := false
	for _, p := range schema.Pairs {
		if p.Intern == s.Intern && p.Track == s.Track && (s.Role == "intern" || p.Mentor == s.Mentor) {
			known = true
			break
		}
	}
	if !known {
		return false
	}
	count := 0
	for _, q := range schema.Questions {
		if q.Role != s.Role {
			continue
		}
		v, has := s.Answers[q.Code]
		if !has && q.Kind == "text" {
			continue
		}
		count++
		if q.Kind == "scale" {
			n, ok := v.(float64)
			if !ok || n != float64(int(n)) || n < 1 || n > float64(q.Max) {
				return false
			}
		} else {
			t, ok := v.(string)
			if !ok || len([]rune(t)) > 2000 {
				return false
			}
		}
	}
	return count == len(s.Answers)
}
func deliver(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var s submission
	if err = json.Unmarshal(data, &s); err != nil {
		return 0, err
	}
	who := s.Intern
	if s.Role == "mentor" {
		who = s.Mentor
	}
	body, _ := json.Marshal(map[string]any{"participant": who, "period": s.Period, "answers": s.Answers})
	req, err := http.NewRequest("POST", setting("PULSE_ENDPOINT", "http://127.0.0.1:8260")+"/api/v1/integrations/yandex-forms", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Secret", secret)
	req.Header.Set("X-Delivery-Id", "surveyjs-"+s.ID)
	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return res.StatusCode, fmt.Errorf("Pulse HTTP %d", res.StatusCode)
	}
	return 200, os.Rename(path, filepath.Join("forms-data", "sent", s.ID+".json"))
}
func receive(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST", 405)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
		http.Error(w, "Чужой адрес формы", 403)
		return
	}
	var s submission
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&s) != nil || !valid(s) {
		http.Error(w, "Проверьте пару, направление, период и ответы.", 400)
		return
	}
	data, _ := json.Marshal(s)
	guard.Lock()
	defer guard.Unlock()
	pending := filepath.Join("forms-data", "pending", s.ID+".json")
	sent := filepath.Join("forms-data", "sent", s.ID+".json")
	for _, p := range []string{sent, pending} {
		if old, err := os.ReadFile(p); err == nil {
			a, b := sha256.Sum256(old), sha256.Sum256(data)
			if subtle.ConstantTimeCompare(a[:], b[:]) != 1 {
				http.Error(w, "Этот номер уже использован.", 409)
				return
			}
			if p == sent {
				w.WriteHeader(200)
				return
			}
		}
	}
	if _, err := os.Stat(pending); os.IsNotExist(err) {
		tmp := pending + ".tmp"
		if err = os.WriteFile(tmp, data, 0600); err == nil {
			err = os.Rename(tmp, pending)
		}
		if err != nil {
			http.Error(w, "Ошибка сохранения", 500)
			return
		}
	}
	// Leave multiple queued answers to the ordered worker so an older correction
	// cannot overwrite a later one when Pulse comes back online.
	if files, _ := filepath.Glob("forms-data/pending/*.json"); len(files) > 1 {
		w.WriteHeader(202)
		return
	}
	code, err := deliver(pending)
	if err != nil {
		if code == 400 || code == 404 || code == 409 {
			os.Rename(pending, filepath.Join("forms-data", "rejected", s.ID+".json"))
			http.Error(w, "Дашборд отклонил анкету. Проверьте текущий период.", 409)
			return
		}
		w.WriteHeader(202)
		return
	}
	w.WriteHeader(200)
}
func main() {
	secret = os.Getenv("WEBHOOK_SECRET")
	if secret == "" {
		log.Fatal("Use start.bat to start both services.")
	}
	b, err := os.ReadFile("data/survey.json")
	if err != nil || json.Unmarshal(b, &schema) != nil {
		log.Fatal("Missing survey configuration")
	}
	for _, dir := range []string{"pending", "sent", "rejected"} {
		if err := os.MkdirAll(filepath.Join("forms-data", dir), 0700); err != nil {
			log.Fatal(err)
		}
	}
	go func() {
		for range time.Tick(8 * time.Second) {
			guard.Lock()
			files, _ := filepath.Glob("forms-data/pending/*.json")
			sort.Slice(files, func(i, j int) bool {
				a, e := os.Stat(files[i])
				b, f := os.Stat(files[j])
				return e == nil && f == nil && a.ModTime().Before(b.ModTime())
			})
			for _, p := range files {
				status, err := deliver(p)
				if err != nil {
					if status == 400 || status == 404 || status == 409 {
						os.Rename(p, filepath.Join("forms-data", "rejected", filepath.Base(p)))
						continue
					}
					break
				}
			}
			guard.Unlock()
		}
	}()
	http.HandleFunc("/submit", receive)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" || name == "intern" || name == "mentor" {
			name = "index.html"
		}
		if strings.Contains(name, "/") {
			http.NotFound(w, r)
			return
		}
		f, e := assets.Open("public/" + name)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		switch filepath.Ext(name) {
		case ".js":
			w.Header().Set("Content-Type", "application/javascript")
		case ".css":
			w.Header().Set("Content-Type", "text/css")
		case ".html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case ".json":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		io.Copy(w, f)
	})
	srv := http.Server{Addr: setting("FORMS_ADDR", "127.0.0.1:8261"), ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

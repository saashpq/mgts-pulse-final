package api

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// static отдаёт собранный фронтенд (WEB_DIR) с того же адреса, что и API: на финале достаточно одного процесса.
// Существующий файл — как есть; путь без расширения — index.html (маршруты SPA, например /pairs/P2 или /form/<token>);
// отсутствующий файл с расширением — 404, чтобы вместо скрипта не пришёл HTML. /api/* никогда не перехватывается.
func (a *App) static(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if a.webDir == "" || p == "/api" || strings.HasPrefix(p, "/api/") || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		writeError(w, 404, "not_found", "route not found")
		return
	}
	clean := path.Clean("/" + p)
	rel := strings.TrimPrefix(clean, "/")
	// на Windows «\» и «C:» — часть пути: такие запросы не должны выйти за пределы каталога
	if rel != "" && (strings.ContainsAny(rel, "\\:") || !filepath.IsLocal(filepath.FromSlash(rel))) {
		http.NotFound(w, r)
		return
	}
	if rel != "" && a.serveFile(w, r, filepath.Join(a.webDir, filepath.FromSlash(rel))) {
		return
	}
	if rel != "" && path.Ext(clean) != "" {
		http.NotFound(w, r)
		return
	}
	if !a.serveFile(w, r, filepath.Join(a.webDir, "index.html")) {
		http.NotFound(w, r)
	}
}

// serveFile отдаёт обычный файл; false — файла нет (или это каталог).
func (a *App) serveFile(w http.ResponseWriter, r *http.Request, name string) bool {
	f, err := os.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	// файлы сборки Vite в assets/ содержат хэш в имени — их можно кэшировать надолго; index.html — нет
	if strings.Contains(filepath.ToSlash(name), "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
	return true
}

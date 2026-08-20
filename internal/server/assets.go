package server

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
)

//go:embed all:web
var embedded embed.FS

// webFS returns the asset filesystem. In dev mode assets are read from disk on
// every request so the UI can be edited without rebuilding.
func webFS(dev bool) (fs.FS, error) {
	if dev {
		if _, err := os.Stat("internal/server/web/index.html"); err != nil {
			return nil, fmt.Errorf("-dev needs to run from the repository root: %w", err)
		}
		return os.DirFS("internal/server/web"), nil
	}
	sub, err := fs.Sub(embedded, "web")
	if err != nil {
		return nil, fmt.Errorf("cannot open embedded assets: %w", err)
	}
	return sub, nil
}

// handleIndex serves the shell page with this process's session token injected.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	page, err := fs.ReadFile(s.files, "index.html")
	if err != nil {
		http.Error(w, "index.html is missing", http.StatusInternalServerError)
		return
	}
	body := strings.ReplaceAll(string(page), "__SCAN_TOKEN__", s.token)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(body))
}

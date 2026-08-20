// Package server hosts the go-scan web UI: a local, single-user app for
// building configurations, previewing the data they produce and running scans.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/markcheno/go-scan/internal/engine"
)

// Server serves the web UI on a loopback address.
type Server struct {
	// Addr is the listen address. Non-loopback addresses are refused.
	Addr string
	// Cache backs quote fetches. Nil disables caching.
	Cache *engine.Cache
	// BaseConfig seeds the UI when it first loads.
	BaseConfig engine.Config
	// DevAssets serves web assets from disk instead of the embedded copy.
	DevAssets bool
	// OpenBrowser launches a browser once the server is listening.
	OpenBrowser bool
	// Concurrency bounds simultaneous fetches per run.
	Concurrency int

	token string
	jobs  *jobStore
	files fs.FS
}

// ListenAndServe starts the server and blocks until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	if s.Addr == "" {
		s.Addr = "127.0.0.1:8080"
	}
	if err := requireLoopback(s.Addr); err != nil {
		return err
	}

	assets, err := webFS(s.DevAssets)
	if err != nil {
		return err
	}
	s.files = assets
	s.jobs = newJobStore()
	s.token = randomToken()

	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", s.Addr, err)
	}

	url := "http://" + listener.Addr().String() + "/"
	httpSrv := &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("go-scan web UI listening on %s", url)
	if s.OpenBrowser {
		openBrowser(url)
	}

	errc := make(chan error, 1)
	go func() { errc <- httpSrv.Serve(listener) }()

	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		log.Print("shutting down")
		s.jobs.cancelAll()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(s.files)))

	mux.HandleFunc("GET /api/meta", s.handleMeta)
	mux.HandleFunc("POST /api/validate", s.handleValidate)
	mux.HandleFunc("POST /api/preview", s.handlePreview)
	mux.HandleFunc("POST /api/scan", s.handleScan)
	mux.HandleFunc("POST /api/run", s.handleRun)
	mux.HandleFunc("GET /api/jobs/{id}/events", s.handleJobEvents)
	mux.HandleFunc("POST /api/jobs/{id}/cancel", s.handleJobCancel)
	mux.HandleFunc("GET /api/config", s.handleConfigLoad)
	mux.HandleFunc("PUT /api/config", s.handleConfigSave)
	mux.HandleFunc("POST /api/config/yaml", s.handleConfigYAML)
	mux.HandleFunc("GET /api/files", s.handleFileDownload)
	mux.HandleFunc("POST /api/cache/clear", s.handleCacheClear)

	return s.guard(mux)
}

// guard rejects requests that did not originate from the page this process
// served. Even bound to loopback the server is reachable from any page the
// user's browser has open, so a Host and Origin check plus a per-process token
// keeps other sites from driving it.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostIsLoopback(r.Host) {
			http.Error(w, "go-scan only serves loopback hosts", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !originIsSelf(origin, r.Host) {
			http.Error(w, "cross-origin requests are not allowed", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && !s.tokenOK(r) {
			http.Error(w, "missing or invalid session token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) tokenOK(r *http.Request) bool {
	if got := r.Header.Get("X-Scan-Token"); got != "" {
		return got == s.token
	}
	// EventSource cannot set headers, so SSE passes the token as a query
	// parameter instead.
	return r.URL.Query().Get("token") == s.token
}

// requireLoopback refuses to expose the UI beyond the local machine. The server
// runs unauthenticated and writes files anywhere the user can.
func requireLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address %q: %w", addr, err)
	}
	if host == "" {
		return fmt.Errorf("refusing to listen on all interfaces; use an explicit loopback address such as 127.0.0.1:8080")
	}
	if isLoopbackHost(host) {
		return nil
	}
	return fmt.Errorf("refusing to listen on %s: the web UI is unauthenticated and must stay on loopback", host)
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func hostIsLoopback(hostHeader string) bool {
	host, _, err := net.SplitHostPort(hostHeader)
	if err != nil {
		host = hostHeader
	}
	return isLoopbackHost(host)
}

func originIsSelf(origin, host string) bool {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(origin, "http://"), "https://")
	return trimmed == host
}

func randomToken() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand does not fail in practice; a predictable token is worse
		// than not starting.
		panic("go-scan: cannot generate a session token: " + err.Error())
	}
	return hex.EncodeToString(buf)
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	if err := exec.Command(cmd, append(args, url)...).Start(); err != nil {
		log.Printf("could not open a browser: %v", err)
	}
}

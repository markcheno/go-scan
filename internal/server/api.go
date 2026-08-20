package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/markcheno/go-scan/internal/engine"
)

// previewTickers and previewBars bound how much work a live preview does.
const (
	previewTickers = 3
	previewBars    = 500
)

type metaResponse struct {
	Sources              []string         `json:"sources"`
	Markets              []string         `json:"markets"`
	Compressions         []string         `json:"compressions"`
	PartitionDateFormats []string         `json:"partition_date_formats"`
	Categories           []string         `json:"categories"`
	Functions            []engine.FuncDef `json:"functions"`
	BaseHeaders          []string         `json:"base_headers"`
	Defaults             engine.Config    `json:"defaults"`
	Config               engine.Config    `json:"config"`
	Cwd                  string           `json:"cwd"`
	CacheDir             string           `json:"cache_dir"`
	Version              string           `json:"version"`
	PreviewLimits        map[string]int   `json:"preview_limits"`
}

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	cwd, _ := os.Getwd()
	cacheDir := ""
	if s.Cache != nil {
		cacheDir = s.Cache.Dir()
	}
	writeJSON(w, http.StatusOK, metaResponse{
		Sources:              engine.Sources,
		Markets:              engine.Markets(),
		Compressions:         engine.Compressions,
		PartitionDateFormats: engine.PartitionDateFormats,
		Categories:           engine.CategoryOrder,
		Functions:            engine.Catalog(),
		BaseHeaders:          engine.BaseHeaders,
		Defaults:             engine.DefaultConfig(),
		Config:               s.BaseConfig,
		Cwd:                  cwd,
		CacheDir:             cacheDir,
		PreviewLimits:        map[string]int{"tickers": previewTickers, "bars": previewBars},
	})
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	cfg, err := decodeConfig(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	problems := engine.Validate(cfg)
	if problems == nil {
		problems = []engine.FieldError{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"problems": problems,
		"headers":  engine.ProjectedHeaders(cfg),
	})
}

type previewRequest struct {
	Config     engine.Config `json:"config"`
	MaxTickers int           `json:"max_tickers"`
	MaxBars    int           `json:"max_bars"`
}

type previewResponse struct {
	Headers  []string               `json:"headers"`
	Rows     [][]string             `json:"rows"`
	Verdicts []engine.TickerVerdict `json:"verdicts"`
	Errors   []engine.TickerError   `json:"errors"`
	Tickers  []string               `json:"tickers"`
	Universe int                    `json:"universe"`
	Elapsed  string                 `json:"elapsed"`
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	var req previewRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.MaxTickers <= 0 {
		req.MaxTickers = previewTickers
	}
	if req.MaxBars <= 0 {
		req.MaxBars = previewBars
	}

	// A preview must never write files, so the output settings cannot make it
	// fail validation either.
	cfg := req.Config
	relaxOutput(&cfg)

	result, err := s.run(r.Context(), &cfg, engine.Options{
		DryRun:     true,
		MaxTickers: req.MaxTickers,
		MaxBars:    req.MaxBars,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, previewResponse{
		Headers:  result.Headers,
		Rows:     nonNilRows(result.Rows),
		Verdicts: nonNilVerdicts(result.Verdicts),
		Errors:   nonNilErrors(result.Errors),
		Tickers:  result.Tickers,
		Universe: result.Universe,
		Elapsed:  result.Elapsed.Round(time.Millisecond).String(),
	})
}

type scanResponse struct {
	Headers  []string               `json:"headers"`
	Verdicts []engine.TickerVerdict `json:"verdicts"`
	Errors   []engine.TickerError   `json:"errors"`
	Passed   int                    `json:"passed"`
	Total    int                    `json:"total"`
	Elapsed  string                 `json:"elapsed"`
}

// handleScan evaluates the filter across the whole universe without producing
// the full table, which is what the screener view needs.
func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	cfg, err := decodeConfig(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	relaxOutput(cfg)

	result, err := s.run(r.Context(), cfg, engine.Options{DryRun: true, MaxBars: previewBars})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	passed := 0
	for _, v := range result.Verdicts {
		if v.Passed {
			passed++
		}
	}
	writeJSON(w, http.StatusOK, scanResponse{
		Headers:  engine.ProjectedHeaders(cfg),
		Verdicts: nonNilVerdicts(result.Verdicts),
		Errors:   nonNilErrors(result.Errors),
		Passed:   passed,
		Total:    result.Universe,
		Elapsed:  result.Elapsed.Round(time.Millisecond).String(),
	})
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	cfg, err := decodeConfig(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if problems := engine.Validate(cfg); engine.HasErrors(problems) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"problems": problems})
		return
	}

	// The run outlives this request, so it gets its own cancellable context.
	ctx, cancel := context.WithCancel(context.Background())
	job := s.jobs.create(cancel)

	go func() {
		defer cancel()
		result, err := s.run(ctx, cfg, engine.Options{
			Progress: func(p engine.Progress) {
				job.publish(Event{Type: "progress", Progress: &p})
			},
			Log: func(msg string) {
				job.publish(Event{Type: "log", Message: msg})
			},
		})
		if err != nil {
			job.publish(Event{Type: "error", Message: err.Error()})
			return
		}
		kept := 0
		for _, v := range result.Verdicts {
			if v.Passed {
				kept++
			}
		}
		job.publish(Event{Type: "done", Result: &runSummary{
			Files:    result.Files,
			Rows:     len(result.Rows),
			Kept:     kept,
			Total:    result.Universe,
			Errors:   nonNilErrors(result.Errors),
			Verdicts: nonNilVerdicts(result.Verdicts),
			Headers:  result.Headers,
			Elapsed:  result.Elapsed.Round(time.Millisecond).String(),
		}})
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
}

func (s *Server) handleJobEvents(w http.ResponseWriter, r *http.Request) {
	job, ok := s.jobs.get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("no such job"))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	backlog, live := job.subscribe()
	for _, ev := range backlog {
		writeSSE(w, ev)
	}
	flusher.Flush()
	if live == nil {
		return
	}
	defer job.unsubscribe(live)

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case ev, open := <-live:
			if !open {
				return
			}
			writeSSE(w, ev)
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request) {
	job, ok := s.jobs.get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("no such job"))
		return
	}
	job.cancel()
	job.publish(Event{Type: "error", Message: "cancelled"})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleConfigLoad(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("a path is required"))
		return
	}
	cfg := engine.DefaultConfig()
	if err := engine.LoadConfig(path, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": cfg, "path": path})
}

func (s *Server) handleConfigSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path   string        `json:"path"`
		Config engine.Config `json:"config"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("a path is required"))
		return
	}
	if dir := filepath.Dir(req.Path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	if err := engine.SaveConfig(req.Path, &req.Config); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	abs, _ := filepath.Abs(req.Path)
	writeJSON(w, http.StatusOK, map[string]string{"path": abs})
}

// handleConfigYAML renders the config with the same encoder that writes it to
// disk, so the preview pane cannot drift from the saved file.
func (s *Server) handleConfigYAML(w http.ResponseWriter, r *http.Request) {
	cfg, err := decodeConfig(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := engine.MarshalConfig(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(out)
}

func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("a path is required"))
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, fmt.Errorf("%s is a directory", path))
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(filepath.Base(path)))
	http.ServeFile(w, r, path)
}

func (s *Server) handleCacheClear(w http.ResponseWriter, r *http.Request) {
	if err := s.Cache.Clear(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// run applies the server's shared options to an engine run.
func (s *Server) run(ctx context.Context, cfg *engine.Config, opts engine.Options) (*engine.Result, error) {
	opts.Fetcher = engine.NewQuoteFetcher(s.Cache)
	if opts.Concurrency == 0 {
		opts.Concurrency = s.Concurrency
	}
	return engine.Run(ctx, cfg, opts)
}

// relaxOutput clears the output settings for runs that write nothing, so a
// half-finished outfile does not block a preview.
func relaxOutput(cfg *engine.Config) {
	cfg.Outfile = "preview.csv"
	cfg.OutputFormats = engine.NewStringList()
}

func decodeConfig(r *http.Request) (*engine.Config, error) {
	var req struct {
		Config *engine.Config `json:"config"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return nil, err
	}
	if req.Config == nil {
		return nil, fmt.Errorf("a config is required")
	}
	return req.Config, nil
}

func decodeJSON(r *http.Request, dst any) error {
	body := http.MaxBytesReader(nil, r.Body, 4<<20)
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("request body must contain a single JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeSSE(w io.Writer, ev Event) {
	payload, err := json.Marshal(ev)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", payload)
}

// The UI is simpler when list fields are always arrays rather than null.
func nonNilRows(rows [][]string) [][]string {
	if rows == nil {
		return [][]string{}
	}
	return rows
}

func nonNilVerdicts(v []engine.TickerVerdict) []engine.TickerVerdict {
	if v == nil {
		return []engine.TickerVerdict{}
	}
	return v
}

func nonNilErrors(e []engine.TickerError) []engine.TickerError {
	if e == nil {
		return []engine.TickerError{}
	}
	return e
}

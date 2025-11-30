package telemetry

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rjboer/GoSDR/internal/logging"
)

//go:embed static/*
var staticFiles embed.FS

// SDRBackend is the minimal interface needed for MockSDR control.
type SDRBackend interface {
	SetPhaseDelta(phaseDeltaDeg float64)
	GetPhaseDelta() float64
}

// WebServer exposes telemetry history and live updates over HTTP.
type WebServer struct {
	srv     *http.Server
	hub     *Hub
	backend SDRBackend
	log     logging.Logger
}

// NewWebServer builds an HTTP server serving the embedded UI, history and live endpoints.
func NewWebServer(addr string, hub *Hub, backend SDRBackend, logger logging.Logger) *WebServer {
	if logger == nil {
		logger = logging.Default()
	}
	ws := &WebServer{
		hub:     hub,
		backend: backend,
		log:     logger.With(logging.Field{Key: "subsystem", Value: "telemetry"}),
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(http.FS(staticFiles)))
	mux.HandleFunc("/api/history", hub.handleHistory)
	mux.HandleFunc("/api/live", hub.handleLive)
	mux.HandleFunc("/api/tracks", hub.handleTracks)
	mux.HandleFunc("/api/tracks/", hub.handleTrackHistory)
	mux.HandleFunc("/api/diagnostics", hub.handleDiagnostics)
	mux.HandleFunc("/api/diagnostics/metrics", hub.handleMetricsStream)
	mux.HandleFunc("/api/diagnostics/health", hub.handleHealth)
	mux.HandleFunc("/api/diagnostics/spectrum", hub.handleSpectrumSnapshot)
	mux.HandleFunc("/api/config", hub.handleGetConfig)
	mux.HandleFunc("/api/config/update", hub.handleSetConfig)
	mux.HandleFunc("/api/mock/angle", ws.handleMockAngle)
	mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, staticFiles, "static/settings.html")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, staticFiles, "static/index.html")
	})

	ws.srv = &http.Server{Addr: addr, Handler: mux}
	return ws
}

func (w *WebServer) handleMockAngle(rw http.ResponseWriter, r *http.Request) {
	if w.backend == nil {
		writeJSONError(rw, http.StatusServiceUnavailable, "SDR backend not available")
		return
	}

	switch r.Method {
	case http.MethodGet:
		phaseDelta := w.backend.GetPhaseDelta()
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(map[string]float64{"phaseDelta": phaseDelta})

	case http.MethodPost:
		var payload struct {
			PhaseDelta float64 `json:"phaseDelta"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSONError(rw, http.StatusBadRequest, fmt.Sprintf("invalid payload: %v", err))
			return
		}
		if payload.PhaseDelta < -90 || payload.PhaseDelta > 90 {
			writeJSONError(rw, http.StatusBadRequest, "phaseDelta must be between -90 and 90 degrees")
			return
		}
		w.backend.SetPhaseDelta(payload.PhaseDelta)
		w.log.Info("mock angle updated", logging.Field{Key: "phaseDelta", Value: payload.PhaseDelta})
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(map[string]float64{"phaseDelta": payload.PhaseDelta})

	default:
		writeJSONError(rw, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// Start begins listening and shuts down when the context is canceled.
func (w *WebServer) Start(ctx context.Context) {
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := w.srv.Shutdown(shutdownCtx); err != nil {
			w.log.Warn("web telemetry shutdown", logging.Field{Key: "error", Value: err})
		}
	}()

	if err := w.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		w.log.Error("web telemetry server error", logging.Field{Key: "error", Value: err})
	}
}

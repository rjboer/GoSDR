package telemetry

import (
	"context"
	"embed"
	"log"
	"net/http"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

// WebServer exposes telemetry history and live updates over HTTP.
type WebServer struct {
	srv *http.Server
	hub *Hub
}

// NewWebServer builds an HTTP server serving the embedded UI, history and live endpoints.
func NewWebServer(addr string, hub *Hub) *WebServer {
	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(http.FS(staticFiles)))
	mux.HandleFunc("/api/history", hub.handleHistory)
	mux.HandleFunc("/api/live", hub.handleLive)
	mux.HandleFunc("/api/config", hub.handleGetConfig)
	mux.HandleFunc("/api/config/update", hub.handleSetConfig)
	mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, staticFiles, "static/settings.html")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, staticFiles, "static/index.html")
	})

	return &WebServer{
		hub: hub,
		srv: &http.Server{Addr: addr, Handler: mux},
	}
}

// Start begins listening and shuts down when the context is canceled.
func (w *WebServer) Start(ctx context.Context) {
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := w.srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("web telemetry shutdown: %v", err)
		}
	}()

	if err := w.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("web telemetry server error: %v", err)
	}
}

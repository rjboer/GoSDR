package telemetry

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Sample captures a single telemetry point for visualization.
type Sample struct {
	Timestamp time.Time `json:"timestamp"`
	AngleDeg  float64   `json:"angleDeg"`
	Peak      float64   `json:"peak"`
}

// Hub collects history and fan-outs telemetry updates to subscribers.
type Hub struct {
	mu           sync.RWMutex
	history      []Sample
	historyLimit int
	subscribers  map[chan Sample]struct{}
}

// NewHub builds a telemetry hub with the provided history limit.
func NewHub(historyLimit int) *Hub {
	if historyLimit <= 0 {
		historyLimit = 500
	}
	return &Hub{
		historyLimit: historyLimit,
		subscribers:  make(map[chan Sample]struct{}),
	}
}

// Report implements Reporter and records a new telemetry sample.
func (h *Hub) Report(angleDeg float64, peak float64) {
	sample := Sample{Timestamp: time.Now(), AngleDeg: angleDeg, Peak: peak}

	h.mu.Lock()
	h.history = append(h.history, sample)
	if len(h.history) > h.historyLimit {
		h.history = h.history[len(h.history)-h.historyLimit:]
	}
	for ch := range h.subscribers {
		select {
		case ch <- sample:
		default:
		}
	}
	h.mu.Unlock()
}

// History returns a copy of stored telemetry samples.
func (h *Hub) History() []Sample {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Sample, len(h.history))
	copy(out, h.history)
	return out
}

// Subscribe registers a listener for live updates.
func (h *Hub) Subscribe() (chan Sample, func()) {
	ch := make(chan Sample, 16)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	cancel := func() {
		h.mu.Lock()
		delete(h.subscribers, ch)
		close(ch)
		h.mu.Unlock()
	}
	return ch, cancel
}

// MultiReporter fans out telemetry to multiple destinations.
type MultiReporter []Reporter

// Report forwards telemetry to each configured reporter.
func (m MultiReporter) Report(angleDeg float64, peak float64) {
	for _, r := range m {
		if r != nil {
			r.Report(angleDeg, peak)
		}
	}
}

func (h *Hub) handleHistory(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.History())
}

func (h *Hub) handleLive(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, cancel := h.Subscribe()
	defer cancel()

	// send existing history for immediate display
	for _, sample := range h.History() {
		payload, _ := json.Marshal(sample)
		w.Write([]byte("data: "))
		w.Write(payload)
		w.Write([]byte("\n\n"))
	}
	flusher.Flush()

	for {
		select {
		case sample, ok := <-ch:
			if !ok {
				return
			}
			payload, _ := json.Marshal(sample)
			w.Write([]byte("data: "))
			w.Write(payload)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

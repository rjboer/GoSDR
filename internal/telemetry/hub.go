package telemetry

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Config represents the runtime configuration exposed by the telemetry hub.
// It focuses on user-facing sampling and buffering values that must be guarded
// by the hub's RWMutex for thread-safe access.
type Config struct {
	SampleRateHz int `json:"sampleRateHz"`
	BufferSize   int `json:"bufferSize"`
	HistoryLimit int `json:"historyLimit"`
}

const (
	minSampleRateHz = 1_000
	maxSampleRateHz = 61_440_000
	minBufferSize   = 64
	maxBufferSize   = 1 << 20
	minHistoryLimit = 1
	maxHistoryLimit = 10_000
)

func defaultConfig() Config {
	return Config{
		SampleRateHz: 2_000_000,
		BufferSize:   4096,
		HistoryLimit: 500,
	}
}

func validateConfig(cfg Config, base Config) (Config, error) {
	if base.SampleRateHz == 0 || base.BufferSize == 0 || base.HistoryLimit == 0 {
		base = defaultConfig()
	}

	if cfg.SampleRateHz == 0 {
		cfg.SampleRateHz = base.SampleRateHz
	}
	if cfg.BufferSize == 0 {
		cfg.BufferSize = base.BufferSize
	}
	if cfg.HistoryLimit == 0 {
		cfg.HistoryLimit = base.HistoryLimit
	}

	if cfg.SampleRateHz < minSampleRateHz || cfg.SampleRateHz > maxSampleRateHz {
		return Config{}, fmt.Errorf("sample rate must be between %d and %d Hz", minSampleRateHz, maxSampleRateHz)
	}
	if cfg.BufferSize < minBufferSize || cfg.BufferSize > maxBufferSize {
		return Config{}, fmt.Errorf("buffer size must be between %d and %d", minBufferSize, maxBufferSize)
	}
	if cfg.BufferSize&(cfg.BufferSize-1) != 0 {
		return Config{}, errors.New("buffer size must be a power of two")
	}
	if cfg.HistoryLimit < minHistoryLimit || cfg.HistoryLimit > maxHistoryLimit {
		return Config{}, fmt.Errorf("history limit must be between %d and %d", minHistoryLimit, maxHistoryLimit)
	}

	return cfg, nil
}

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
	config       Config
}

// NewHub builds a telemetry hub with the provided history limit.
func NewHub(historyLimit int) *Hub {
	cfg := defaultConfig()
	if historyLimit > 0 {
		cfg.HistoryLimit = historyLimit
	}
	cfg, _ = validateConfig(cfg, defaultConfig())
	return &Hub{
		historyLimit: cfg.HistoryLimit,
		subscribers:  make(map[chan Sample]struct{}),
		config:       cfg,
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

// ConfigSnapshot returns the latest validated configuration.
func (h *Hub) ConfigSnapshot() Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.config
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

func (h *Hub) applyConfig(cfg Config) {
	h.config = cfg
	h.historyLimit = cfg.HistoryLimit
	if len(h.history) > h.historyLimit {
		h.history = h.history[len(h.history)-h.historyLimit:]
	}
}

func (h *Hub) handleHistory(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.History())
}

func (h *Hub) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.ConfigSnapshot())
}

func (h *Hub) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var incoming Config
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		http.Error(w, fmt.Sprintf("invalid config payload: %v", err), http.StatusBadRequest)
		return
	}

	h.mu.RLock()
	current := h.config
	h.mu.RUnlock()

	cfg, err := validateConfig(incoming, current)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	h.applyConfig(cfg)
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(cfg)
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

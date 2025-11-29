package telemetry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rjboer/GoSDR/internal/logging"
)

// Config represents the runtime configuration exposed by the telemetry hub.
// It focuses on user-facing sampling and buffering values that must be guarded
// by the hub's RWMutex for thread-safe access.
type Config struct {
	SampleRateHz      int     `json:"sampleRateHz"`
	RxLoHz            float64 `json:"rxLoHz"`
	ToneOffsetHz      float64 `json:"toneOffsetHz"`
	SpacingWavelength float64 `json:"spacingWavelength"`
	NumSamples        int     `json:"numSamples"`
	BufferSize        int     `json:"bufferSize"`
	HistoryLimit      int     `json:"historyLimit"`
	TrackingLength    int     `json:"trackingLength"`
	PhaseStepDeg      float64 `json:"phaseStepDeg"`
	ScanStepDeg       float64 `json:"scanStepDeg"`
	PhaseCalDeg       float64 `json:"phaseCalDeg"`
	PhaseDeltaDeg     float64 `json:"phaseDeltaDeg"`
	MockPhaseDelta    float64 `json:"mockPhaseDelta"`
	WarmupBuffers     int     `json:"warmupBuffers"`
	RxGain0           int     `json:"rxGain0"`
	RxGain1           int     `json:"rxGain1"`
	TxGain            int     `json:"txGain"`
	SDRBackend        string  `json:"sdrBackend"`
	SDRURI            string  `json:"sdrUri"`
}

const (
	minSampleRateHz = 1_000
	maxSampleRateHz = 61_440_000
	minBufferSize   = 64
	maxBufferSize   = 1 << 20
	minHistoryLimit = 1
	maxHistoryLimit = 10_000
	minNumSamples   = 64
	maxNumSamples   = 1 << 20
	minTracking     = 1
	maxTracking     = 10_000
	configFilePath  = "config.json"
)

type persistentConfig struct {
	SampleRate     float64 `json:"sample_rate"`
	RxLO           float64 `json:"rx_lo"`
	RxGain0        int     `json:"rx_gain0"`
	RxGain1        int     `json:"rx_gain1"`
	TxGain         int     `json:"tx_gain"`
	ToneOffset     float64 `json:"tone_offset"`
	NumSamples     int     `json:"num_samples"`
	TrackingLength int     `json:"tracking_length"`
	PhaseStep      float64 `json:"phase_step"`
	PhaseCal       float64 `json:"phase_cal"`
	ScanStep       float64 `json:"scan_step"`
	Spacing        float64 `json:"spacing_wavelength"`
	PhaseDelta     float64 `json:"phase_delta"`
	SDRBackend     string  `json:"sdr_backend"`
	SDRURI         string  `json:"sdr_uri"`
	WarmupBuffers  int     `json:"warmup_buffers"`
	HistoryLimit   int     `json:"history_limit"`
	WebAddr        string  `json:"web_addr"`
	LogLevel       string  `json:"log_level"`
	LogFormat      string  `json:"log_format"`
}

func defaultConfig() Config {
	return Config{
		SampleRateHz:      2_000_000,
		RxLoHz:            2_300_000_000,
		ToneOffsetHz:      200_000,
		SpacingWavelength: 0.5,
		NumSamples:        512,
		BufferSize:        4096,
		HistoryLimit:      500,
		TrackingLength:    50,
		PhaseStepDeg:      1,
		ScanStepDeg:       2,
		PhaseCalDeg:       0,
		PhaseDeltaDeg:     35,
		MockPhaseDelta:    30,
		WarmupBuffers:     3,
		RxGain0:           0,
		RxGain1:           0,
		TxGain:            -10,
		SDRBackend:        "mock",
		SDRURI:            "ip:192.168.2.1",
	}
}

func defaultPersistentConfig() persistentConfig {
	return persistentConfig{
		SampleRate:     2e6,
		RxLO:           2.3e9,
		RxGain0:        60,
		RxGain1:        60,
		TxGain:         -10,
		ToneOffset:     200e3,
		NumSamples:     1 << 12,
		TrackingLength: 100,
		PhaseStep:      1,
		PhaseCal:       0,
		ScanStep:       2,
		Spacing:        0.5,
		PhaseDelta:     30,
		SDRBackend:     "mock",
		SDRURI:         "",
		WarmupBuffers:  3,
		HistoryLimit:   500,
		WebAddr:        ":8080",
		LogLevel:       "info",
		LogFormat:      "text",
	}
}

func validateConfig(cfg Config, base Config) (Config, error) {
	if base.SampleRateHz == 0 || base.BufferSize == 0 || base.HistoryLimit == 0 {
		base = defaultConfig()
	}

	if cfg.SampleRateHz == 0 {
		cfg.SampleRateHz = base.SampleRateHz
	}
	if cfg.RxLoHz == 0 {
		cfg.RxLoHz = base.RxLoHz
	}
	if cfg.ToneOffsetHz == 0 {
		cfg.ToneOffsetHz = base.ToneOffsetHz
	}
	if cfg.SpacingWavelength == 0 {
		cfg.SpacingWavelength = base.SpacingWavelength
	}
	if cfg.NumSamples == 0 {
		cfg.NumSamples = base.NumSamples
	}
	if cfg.BufferSize == 0 {
		cfg.BufferSize = base.BufferSize
	}
	if cfg.HistoryLimit == 0 {
		cfg.HistoryLimit = base.HistoryLimit
	}
	if cfg.TrackingLength == 0 {
		cfg.TrackingLength = base.TrackingLength
	}
	if cfg.PhaseStepDeg == 0 {
		cfg.PhaseStepDeg = base.PhaseStepDeg
	}
	if cfg.ScanStepDeg == 0 {
		cfg.ScanStepDeg = base.ScanStepDeg
	}
	if cfg.WarmupBuffers == 0 {
		cfg.WarmupBuffers = base.WarmupBuffers
	}
	if cfg.MockPhaseDelta == 0 {
		cfg.MockPhaseDelta = base.MockPhaseDelta
	}

	cfg.SDRBackend = strings.ToLower(strings.TrimSpace(cfg.SDRBackend))
	cfg.SDRURI = strings.TrimSpace(cfg.SDRURI)

	if cfg.SDRBackend == "" {
		cfg.SDRBackend = base.SDRBackend
	}

	switch cfg.SDRBackend {
	case "mock":
		cfg.SDRURI = ""
	case "pluto":
		if cfg.SDRURI == "" {
			cfg.SDRURI = base.SDRURI
		}
		if cfg.SDRURI == "" {
			return Config{}, errors.New("sdr uri required for pluto backend")
		}
	default:
		return Config{}, fmt.Errorf("unsupported sdr backend %q", cfg.SDRBackend)
	}

	if cfg.SampleRateHz < minSampleRateHz || cfg.SampleRateHz > maxSampleRateHz {
		return Config{}, fmt.Errorf("sample rate must be between %d and %d Hz", minSampleRateHz, maxSampleRateHz)
	}
	if cfg.NumSamples < minNumSamples || cfg.NumSamples > maxNumSamples {
		return Config{}, fmt.Errorf("num samples must be between %d and %d", minNumSamples, maxNumSamples)
	}
	if cfg.NumSamples&(cfg.NumSamples-1) != 0 {
		return Config{}, errors.New("num samples must be a power of two")
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
	if cfg.TrackingLength < minTracking || cfg.TrackingLength > maxTracking {
		return Config{}, fmt.Errorf("tracking length must be between %d and %d", minTracking, maxTracking)
	}
	if cfg.PhaseStepDeg <= 0 {
		return Config{}, errors.New("phase step must be positive")
	}
	if cfg.ScanStepDeg <= 0 {
		return Config{}, errors.New("scan step must be positive")
	}
	if cfg.SpacingWavelength <= 0 {
		return Config{}, errors.New("spacing wavelength must be positive")
	}

	return cfg, nil
}

func loadPersistentConfig(path string) (persistentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return persistentConfig{}, err
	}

	var cfg persistentConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return persistentConfig{}, err
	}

	return cfg, nil
}

func savePersistentConfig(path string, cfg persistentConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (h *Hub) persistConfig(cfg Config) error {
	stored, err := loadPersistentConfig(configFilePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			stored = defaultPersistentConfig()
		} else {
			return err
		}
	}

	stored.SampleRate = float64(cfg.SampleRateHz)
	stored.RxLO = cfg.RxLoHz
	stored.RxGain0 = cfg.RxGain0
	stored.RxGain1 = cfg.RxGain1
	stored.TxGain = cfg.TxGain
	stored.ToneOffset = cfg.ToneOffsetHz
	stored.NumSamples = cfg.NumSamples
	stored.TrackingLength = cfg.TrackingLength
	stored.PhaseStep = cfg.PhaseStepDeg
	stored.PhaseCal = cfg.PhaseCalDeg
	stored.ScanStep = cfg.ScanStepDeg
	stored.Spacing = cfg.SpacingWavelength
	stored.PhaseDelta = cfg.MockPhaseDelta
	stored.SDRBackend = cfg.SDRBackend
	stored.SDRURI = cfg.SDRURI
	stored.WarmupBuffers = cfg.WarmupBuffers
	stored.HistoryLimit = cfg.HistoryLimit
	if stored.LogLevel == "" {
		stored.LogLevel = "info"
	}
	if stored.LogFormat == "" {
		stored.LogFormat = "text"
	}

	return savePersistentConfig(configFilePath, stored)
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
	logger       logging.Logger
}

// NewHub builds a telemetry hub with the provided history limit.
func NewHub(historyLimit int, logger logging.Logger) *Hub {
	cfg := defaultConfig()
	if historyLimit > 0 {
		cfg.HistoryLimit = historyLimit
	}
	cfg, _ = validateConfig(cfg, defaultConfig())
	if logger == nil {
		logger = logging.Default()
	}
	return &Hub{
		historyLimit: cfg.HistoryLimit,
		subscribers:  make(map[chan Sample]struct{}),
		config:       cfg,
		logger:       logger.With(logging.Field{Key: "subsystem", Value: "telemetry"}),
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

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
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
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var incoming Config
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid config payload: %v", err))
		return
	}

	h.mu.RLock()
	current := h.config
	h.mu.RUnlock()

	cfg, err := validateConfig(incoming, current)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.mu.Lock()
	h.applyConfig(cfg)
	h.mu.Unlock()

	if err := h.persistConfig(cfg); err != nil {
		h.logger.Warn("failed to persist config", logging.Field{Key: "error", Value: err})
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to save config: %v", err))
		return
	}

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

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/rjboer/GoSDR/internal/app"
	"github.com/rjboer/GoSDR/internal/logging"
	"github.com/rjboer/GoSDR/internal/sdr"
	"github.com/rjboer/GoSDR/internal/telemetry"
)

func main() {
	const configPath = "config.json"
	logger := logging.New(logging.Warn, logging.Text, os.Stdout).With(logging.Field{Key: "subsystem", Value: "cli"})
	logging.SetDefault(logger)

	persistentCfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		logger.Error("load config", logging.Field{Key: "error", Value: err})
		os.Exit(1)
	}

	cfg, err := parseConfig(os.Args[1:], os.LookupEnv, persistentCfg)
	if err != nil {
		logger.Error("parse config", logging.Field{Key: "error", Value: err})
		os.Exit(1)
	}
	if cfg.verbose {
		cfg.debugMode = true
	}

	levelStr := cfg.logLevel
	if cfg.verbose {
		levelStr = "debug"
	}
	level, err := logging.ParseLevel(levelStr)
	if err != nil {
		logger.Error("invalid log level", logging.Field{Key: "error", Value: err})
		os.Exit(1)
	}
	cfg.logLevel = levelStr
	format, err := logging.ParseFormat(cfg.logFormat)
	if err != nil {
		logger.Error("invalid log format", logging.Field{Key: "error", Value: err})
		os.Exit(1)
	}

	logger = logging.New(level, format, os.Stdout).With(logging.Field{Key: "subsystem", Value: "cli"})
	logging.SetDefault(logger)
	logStartupBanner(logger, cfg)

	if err := saveConfig(configPath, persistentFromCLI(cfg)); err != nil {
		logger.Error("save config", logging.Field{Key: "error", Value: err})
		os.Exit(1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	backend, err := selectBackend(cfg)
	if err != nil {
		logger.Error("select backend", logging.Field{Key: "error", Value: err})
		os.Exit(1)
	}
	// Only use web telemetry (no stdout spam)
	var reporters []telemetry.Reporter
	if cfg.webAddr != "" {
		hubLogger := logger.With(logging.Field{Key: "subsystem", Value: "telemetry"})
		hub := telemetry.NewHub(cfg.historyLimit, hubLogger)
		reporters = append(reporters, hub)
		go telemetry.NewWebServer(cfg.webAddr, hub, backend, hubLogger).Start(ctx)
		hubLogger.Info("web interface available", logging.Field{Key: "addr", Value: cfg.webAddr})
	} else {
		// Fallback to stdout if no web interface
		reporters = append(reporters, telemetry.NewStdoutReporter(logger.With(logging.Field{Key: "subsystem", Value: "telemetry"})))
	}

	trackerLogger := logger.With(logging.Field{Key: "subsystem", Value: "tracker"})
	tracker := app.NewTracker(backend, telemetry.MultiReporter(reporters), trackerLogger, app.Config{
		SampleRate:        cfg.sampleRate,
		RxLO:              cfg.rxLO,
		RxGain0:           cfg.rxGain0,
		RxGain1:           cfg.rxGain1,
		TxGain:            cfg.txGain,
		ToneOffset:        cfg.toneOffset,
		NumSamples:        cfg.numSamples,
		SpacingWavelength: cfg.spacing,
		TrackingLength:    cfg.trackingLength,
		PhaseStep:         cfg.phaseStep,
		PhaseCal:          cfg.phaseCal,
		ScanStep:          cfg.scanStep,
		PhaseDelta:        cfg.phaseDelta,
		WarmupBuffers:     cfg.warmupBuffers,
		HistoryLimit:      cfg.historyLimit,
		DebugMode:         cfg.debugMode,
		TrackingMode:      cfg.trackingMode,
		MaxTracks:         cfg.maxTracks,
		TrackTimeout:      cfg.trackTimeout,
		MinSNRThreshold:   cfg.minSNR,
	})

	if err := tracker.Init(ctx); err != nil {
		trackerLogger.Error("init tracker", logging.Field{Key: "error", Value: err})
		os.Exit(1)
	}

	// Run continuously (no timeout)
	trackerLogger.Info("starting tracker", logging.Field{Key: "note", Value: "Ctrl+C to stop"})
	if err := tracker.Run(ctx); err != nil && err != context.Canceled {
		trackerLogger.Error("run tracker", logging.Field{Key: "error", Value: err})
		os.Exit(1)
	}
}

type cliConfig struct {
	sampleRate     float64
	rxLO           float64
	rxGain0        int
	rxGain1        int
	txGain         int
	toneOffset     float64
	numSamples     int
	trackingLength int
	phaseStep      float64
	phaseCal       float64
	scanStep       float64
	spacing        float64
	phaseDelta     float64
	trackingMode   string
	maxTracks      int
	trackTimeout   time.Duration
	minSNR         float64
	sdrBackend     string
	sdrURI         string
	warmupBuffers  int
	historyLimit   int
	webAddr        string
	logLevel       string
	logFormat      string
	debugMode      bool
	verbose        bool
}

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
	TrackingMode   string  `json:"tracking_mode"`
	MaxTracks      int     `json:"max_tracks"`
	TrackTimeout   string  `json:"track_timeout"`
	MinSNR         float64 `json:"min_snr_threshold"`
	SDRBackend     string  `json:"sdr_backend"`
	SDRURI         string  `json:"sdr_uri"`
	WarmupBuffers  int     `json:"warmup_buffers"`
	HistoryLimit   int     `json:"history_limit"`
	WebAddr        string  `json:"web_addr"`
	LogLevel       string  `json:"log_level"`
	LogFormat      string  `json:"log_format"`
	DebugMode      bool    `json:"debug_mode"`
}

func logStartupBanner(logger logging.Logger, cfg cliConfig) {
	logger.Info("starting monopulse tracker", logging.Field{Key: "config", Value: map[string]any{
		"sample_rate":      cfg.sampleRate,
		"rx_lo":            cfg.rxLO,
		"rx_gain0":         cfg.rxGain0,
		"rx_gain1":         cfg.rxGain1,
		"tx_gain":          cfg.txGain,
		"tone_offset":      cfg.toneOffset,
		"spacing":          cfg.spacing,
		"phase_step":       cfg.phaseStep,
		"phase_cal":        cfg.phaseCal,
		"scan_step":        cfg.scanStep,
		"tracking_length":  cfg.trackingLength,
		"warmup_buffers":   cfg.warmupBuffers,
		"history_limit":    cfg.historyLimit,
		"tracking_mode":    cfg.trackingMode,
		"max_tracks":       cfg.maxTracks,
		"track_timeout":    cfg.trackTimeout,
		"min_snr":          cfg.minSNR,
		"sdr_backend":      cfg.sdrBackend,
		"sdr_uri":          cfg.sdrURI,
		"log_level":        cfg.logLevel,
		"log_format":       cfg.logFormat,
		"debug_mode":       cfg.debugMode,
		"verbose":          cfg.verbose,
		"web_addr":         cfg.webAddr,
		"mock_phase_delta": cfg.phaseDelta,
	}})
}

func parseConfig(args []string, lookup func(string) (string, bool), defaults persistentConfig) (cliConfig, error) {
	cfg := cliConfig{}
	fs := flag.NewFlagSet("monopulse", flag.ContinueOnError)
	fs.Float64Var(&cfg.sampleRate, "sample-rate", envFloat(lookup, "MONO_SAMPLE_RATE", defaults.SampleRate), "Sample rate in Hz")
	fs.Float64Var(&cfg.rxLO, "rx-lo", envFloat(lookup, "MONO_RX_LO", defaults.RxLO), "RX LO frequency in Hz")
	fs.IntVar(&cfg.rxGain0, "rx-gain0", envInt(lookup, "MONO_RX_GAIN0", defaults.RxGain0), "RX gain for channel 0 (dB)")
	fs.IntVar(&cfg.rxGain1, "rx-gain1", envInt(lookup, "MONO_RX_GAIN1", defaults.RxGain1), "RX gain for channel 1 (dB)")
	fs.IntVar(&cfg.txGain, "tx-gain", envInt(lookup, "MONO_TX_GAIN", defaults.TxGain), "TX gain (dB)")
	fs.Float64Var(&cfg.toneOffset, "tone-offset", envFloat(lookup, "MONO_TONE_OFFSET", defaults.ToneOffset), "Tone offset in Hz")
	fs.IntVar(&cfg.numSamples, "num-samples", envInt(lookup, "MONO_NUM_SAMPLES", defaults.NumSamples), "Number of samples per RX call")
	fs.IntVar(&cfg.trackingLength, "tracking-length", envInt(lookup, "MONO_TRACKING_LENGTH", defaults.TrackingLength), "Number of tracking iterations")
	fs.Float64Var(&cfg.phaseStep, "phase-step", envFloat(lookup, "MONO_PHASE_STEP", defaults.PhaseStep), "Phase step (degrees) for monopulse updates")
	fs.Float64Var(&cfg.phaseCal, "phase-cal", envFloat(lookup, "MONO_PHASE_CAL", defaults.PhaseCal), "Additional calibration phase (degrees)")
	fs.Float64Var(&cfg.scanStep, "scan-step", envFloat(lookup, "MONO_SCAN_STEP", defaults.ScanStep), "Scan step in degrees for coarse search")
	fs.Float64Var(&cfg.spacing, "spacing-wavelength", envFloat(lookup, "MONO_SPACING_WAVELENGTH", defaults.Spacing), "Antenna spacing as a fraction of wavelength")
	fs.Float64Var(&cfg.phaseDelta, "mock-phase-delta", envFloat(lookup, "MONO_MOCK_PHASE_DELTA", defaults.PhaseDelta), "Mock SDR phase delta in degrees")
	fs.StringVar(&cfg.trackingMode, "tracking-mode", envString(lookup, "MONO_TRACKING_MODE", defaults.TrackingMode), "Tracking mode (single|multi)")
	fs.IntVar(&cfg.maxTracks, "max-tracks", envInt(lookup, "MONO_MAX_TRACKS", defaults.MaxTracks), "Maximum number of simultaneous tracks")
	fs.DurationVar(&cfg.trackTimeout, "track-timeout", envDuration(lookup, "MONO_TRACK_TIMEOUT", durationFromString(defaults.TrackTimeout, 0)), "Duration after which inactive tracks are marked lost")
	fs.Float64Var(&cfg.minSNR, "min-snr-threshold", envFloat(lookup, "MONO_MIN_SNR_THRESHOLD", defaults.MinSNR), "Minimum SNR required to create or update a track")
	fs.StringVar(&cfg.sdrBackend, "sdr-backend", envString(lookup, "MONO_SDR_BACKEND", defaults.SDRBackend), "SDR backend (mock|pluto)")
	fs.StringVar(&cfg.sdrURI, "sdr-uri", envString(lookup, "MONO_SDR_URI", defaults.SDRURI), "SDR URI")
	fs.IntVar(&cfg.warmupBuffers, "warmup-buffers", envInt(lookup, "MONO_WARMUP_BUFFERS", defaults.WarmupBuffers), "Number of RX buffers to discard for warm-up")
	fs.IntVar(&cfg.historyLimit, "history-limit", envInt(lookup, "MONO_HISTORY_LIMIT", defaults.HistoryLimit), "Maximum samples to keep in telemetry history")
	fs.StringVar(&cfg.webAddr, "web-addr", envString(lookup, "MONO_WEB_ADDR", defaults.WebAddr), "Optional web telemetry listen address (e.g. :8080)")
	fs.StringVar(&cfg.logLevel, "log-level", envString(lookup, "MONO_LOG_LEVEL", defaults.LogLevel), "Log level (debug|info|warn|error)")
	fs.StringVar(&cfg.logFormat, "log-format", envString(lookup, "MONO_LOG_FORMAT", defaults.LogFormat), "Log format (text|json)")
	fs.BoolVar(&cfg.debugMode, "debug-mode", envBool(lookup, "MONO_DEBUG_MODE", defaults.DebugMode), "Include debug telemetry fields")
	fs.BoolVar(&cfg.verbose, "verbose", envBool(lookup, "MONO_VERBOSE", false), "Enable verbose logging and debug output")

	if err := fs.Parse(args); err != nil {
		return cliConfig{}, fmt.Errorf("parse flags: %w", err)
	}
	return cfg, nil
}

func persistentFromCLI(cfg cliConfig) persistentConfig {
	if cfg.logLevel == "" {
		cfg.logLevel = "warn"
	}
	if cfg.logFormat == "" {
		cfg.logFormat = "text"
	}
	return persistentConfig{
		SampleRate:     cfg.sampleRate,
		RxLO:           cfg.rxLO,
		RxGain0:        cfg.rxGain0,
		RxGain1:        cfg.rxGain1,
		TxGain:         cfg.txGain,
		ToneOffset:     cfg.toneOffset,
		NumSamples:     cfg.numSamples,
		TrackingLength: cfg.trackingLength,
		PhaseStep:      cfg.phaseStep,
		PhaseCal:       cfg.phaseCal,
		ScanStep:       cfg.scanStep,
		Spacing:        cfg.spacing,
		PhaseDelta:     cfg.phaseDelta,
		TrackingMode:   cfg.trackingMode,
		MaxTracks:      cfg.maxTracks,
		TrackTimeout:   cfg.trackTimeout.String(),
		MinSNR:         cfg.minSNR,
		SDRBackend:     cfg.sdrBackend,
		SDRURI:         cfg.sdrURI,
		WarmupBuffers:  cfg.warmupBuffers,
		HistoryLimit:   cfg.historyLimit,
		WebAddr:        cfg.webAddr,
		LogLevel:       cfg.logLevel,
		LogFormat:      cfg.logFormat,
		DebugMode:      cfg.debugMode,
	}
}

func loadOrCreateConfig(path string) (persistentConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := defaultPersistentConfig()
			if saveErr := saveConfig(path, cfg); saveErr != nil {
				return persistentConfig{}, fmt.Errorf("create default config: %w", saveErr)
			}
			return cfg, nil
		}
		return persistentConfig{}, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	var cfg persistentConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return persistentConfig{}, fmt.Errorf("decode config: %w", err)
	}
	return cfg, nil
}

func saveConfig(path string, cfg persistentConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
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
		TrackingMode:   "single",
		MaxTracks:      1,
		TrackTimeout:   "3s",
		MinSNR:         3,
		SDRBackend:     "mock",
		SDRURI:         "",
		WarmupBuffers:  3,
		HistoryLimit:   500,
		WebAddr:        ":8080",
		LogLevel:       "warn",
		LogFormat:      "text",
		DebugMode:      false,
	}
}

func envFloat(lookup func(string) (string, bool), key string, def float64) float64 {
	if val, ok := lookup(key); ok {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			return parsed
		}
	}
	return def
}

func envInt(lookup func(string) (string, bool), key string, def int) int {
	if val, ok := lookup(key); ok {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return def
}

func envString(lookup func(string) (string, bool), key, def string) string {
	if val, ok := lookup(key); ok {
		return val
	}
	return def
}

func envDuration(lookup func(string) (string, bool), key string, def time.Duration) time.Duration {
	if val, ok := lookup(key); ok {
		if parsed, err := time.ParseDuration(val); err == nil {
			return parsed
		}
	}
	return def
}

func envBool(lookup func(string) (string, bool), key string, def bool) bool {
	if val, ok := lookup(key); ok {
		if parsed, err := strconv.ParseBool(val); err == nil {
			return parsed
		}
	}
	return def
}

func durationFromString(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}
	if parsed, err := time.ParseDuration(value); err == nil {
		return parsed
	}
	return fallback
}

func selectBackend(cfg cliConfig) (sdr.SDR, error) {
	switch cfg.sdrBackend {
	case "mock":
		return sdr.NewMock(), nil
	case "pluto":
		return sdr.NewPluto(), nil
	default:
		return nil, fmt.Errorf("unknown backend %s", cfg.sdrBackend)
	}
}

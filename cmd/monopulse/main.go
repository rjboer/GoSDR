package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
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

	cfg, err := parseConfig(os.Args[1:], persistentCfg)
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

	logger.Info("selecting SDR backend", logging.Field{Key: "backend", Value: cfg.sdrBackend})
	backend, err := selectBackend(cfg)
	if err != nil {
		logger.Error("select backend", logging.Field{Key: "error", Value: err})
		os.Exit(1)
	}
	logger.Info("backend selected successfully", logging.Field{Key: "backend", Value: cfg.sdrBackend})

	// Only use web telemetry (no stdout spam)
	var reporters []telemetry.Reporter
	if cfg.webAddr != "" {
		logger.Info("initializing telemetry hub")
		hubLogger := logger.With(logging.Field{Key: "subsystem", Value: "telemetry"})
		hub := telemetry.NewHub(cfg.historyLimit, hubLogger)
		reporters = append(reporters, hub)

		// Wire up Pluto SDR event logger if using Pluto backend
		if pluto, ok := backend.(*sdr.PlutoSDR); ok {
			logger.Info("configuring Pluto SDR event logging")
			pluto.SetEventLogger(hub)
			pluto.SetDebugMode(cfg.debugMode)
		}

		logger.Info("starting web server", logging.Field{Key: "addr", Value: cfg.webAddr})
		go telemetry.NewWebServer(cfg.webAddr, hub, backend, hubLogger).Start(ctx)
		hubLogger.Info("web interface available", logging.Field{Key: "addr", Value: cfg.webAddr})
	} else {
		// Fallback to stdout if no web interface
		reporters = append(reporters, telemetry.NewStdoutReporter(logger.With(logging.Field{Key: "subsystem", Value: "telemetry"})))
	}

	logger.Info("creating tracker")
	trackerLogger := logger.With(logging.Field{Key: "subsystem", Value: "tracker"})
	tracker := app.NewTracker(backend, telemetry.MultiReporter(reporters), trackerLogger, app.Config{
		URI:               cfg.sdrURI,
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
		SSHHost:           cfg.sshHost,
		SSHUser:           cfg.sshUser,
		SSHPassword:       cfg.sshPassword,
		SSHKeyPath:        cfg.sshKeyPath,
		SSHPort:           cfg.sshPort,
		SysfsRoot:         cfg.sysfsRoot,
	})

	logger.Info("initializing tracker (this may take a few seconds)")
	if err := tracker.Init(ctx); err != nil {
		trackerLogger.Error("init tracker", logging.Field{Key: "error", Value: err})
		os.Exit(1)
	}
	logger.Info("tracker initialized successfully")

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
	sshHost        string
	sshUser        string
	sshPassword    string
	sshKeyPath     string
	sshPort        int
	sysfsRoot      string
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
	SSHHost        string  `json:"ssh_host"`
	SSHUser        string  `json:"ssh_user"`
	SSHPassword    string  `json:"ssh_password"`
	SSHKeyPath     string  `json:"ssh_key_path"`
	SSHPort        int     `json:"ssh_port"`
	SysfsRoot      string  `json:"sysfs_root"`
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
		"ssh_host":         cfg.sshHost,
		"ssh_user":         cfg.sshUser,
		"ssh_password":     cfg.sshPassword,
		"ssh_port":         cfg.sshPort,
		"sysfs_root":       cfg.sysfsRoot,
		"log_level":        cfg.logLevel,
		"log_format":       cfg.logFormat,
		"debug_mode":       cfg.debugMode,
		"verbose":          cfg.verbose,
		"web_addr":         cfg.webAddr,
		"mock_phase_delta": cfg.phaseDelta,
	}})
}

func parseConfig(args []string, defaults persistentConfig) (cliConfig, error) {
	cfg := cliConfig{}
	fs := flag.NewFlagSet("monopulse", flag.ContinueOnError)
	fs.Float64Var(&cfg.sampleRate, "sample-rate", defaults.SampleRate, "Sample rate in Hz")
	fs.Float64Var(&cfg.rxLO, "rx-lo", defaults.RxLO, "RX LO frequency in Hz")
	fs.IntVar(&cfg.rxGain0, "rx-gain0", defaults.RxGain0, "RX gain for channel 0 (dB)")
	fs.IntVar(&cfg.rxGain1, "rx-gain1", defaults.RxGain1, "RX gain for channel 1 (dB)")
	fs.IntVar(&cfg.txGain, "tx-gain", defaults.TxGain, "TX gain (dB)")
	fs.Float64Var(&cfg.toneOffset, "tone-offset", defaults.ToneOffset, "Tone offset in Hz")
	fs.IntVar(&cfg.numSamples, "num-samples", defaults.NumSamples, "Number of samples per RX call")
	fs.IntVar(&cfg.trackingLength, "tracking-length", defaults.TrackingLength, "Number of tracking iterations")
	fs.Float64Var(&cfg.phaseStep, "phase-step", defaults.PhaseStep, "Phase step (degrees) for monopulse updates")
	fs.Float64Var(&cfg.phaseCal, "phase-cal", defaults.PhaseCal, "Additional calibration phase (degrees)")
	fs.Float64Var(&cfg.scanStep, "scan-step", defaults.ScanStep, "Scan step in degrees for coarse search")
	fs.Float64Var(&cfg.spacing, "spacing-wavelength", defaults.Spacing, "Antenna spacing as a fraction of wavelength")
	fs.Float64Var(&cfg.phaseDelta, "mock-phase-delta", defaults.PhaseDelta, "Mock SDR phase delta in degrees")
	fs.StringVar(&cfg.trackingMode, "tracking-mode", defaults.TrackingMode, "Tracking mode (single|multi)")
	fs.IntVar(&cfg.maxTracks, "max-tracks", defaults.MaxTracks, "Maximum number of simultaneous tracks")
	fs.DurationVar(&cfg.trackTimeout, "track-timeout", durationFromString(defaults.TrackTimeout, 0), "Duration after which inactive tracks are marked lost")
	fs.Float64Var(&cfg.minSNR, "min-snr-threshold", defaults.MinSNR, "Minimum SNR required to create or update a track")
	fs.StringVar(&cfg.sdrBackend, "sdr-backend", defaults.SDRBackend, "SDR backend (mock|pluto)")
	fs.StringVar(&cfg.sdrURI, "sdr-uri", defaults.SDRURI, "SDR URI")
	fs.StringVar(&cfg.sshHost, "sdr-ssh-host", defaults.SSHHost, "SSH hostname/IP for sysfs fallback when IIOD writes are disabled")
	fs.StringVar(&cfg.sshUser, "sdr-ssh-user", defaults.SSHUser, "SSH username for sysfs fallback (default root)")
	fs.StringVar(&cfg.sshPassword, "sdr-ssh-password", defaults.SSHPassword, "SSH password for sysfs fallback")
	fs.StringVar(&cfg.sshKeyPath, "sdr-ssh-key", defaults.SSHKeyPath, "Path to private key for SSH sysfs fallback")
	fs.IntVar(&cfg.sshPort, "sdr-ssh-port", defaults.SSHPort, "SSH port for sysfs fallback (default 22)")
	fs.StringVar(&cfg.sysfsRoot, "sdr-sysfs-root", defaults.SysfsRoot, "Sysfs root on device (default /sys/bus/iio/devices)")
	fs.IntVar(&cfg.warmupBuffers, "warmup-buffers", defaults.WarmupBuffers, "Number of RX buffers to discard for warm-up")
	fs.IntVar(&cfg.historyLimit, "history-limit", defaults.HistoryLimit, "Maximum samples to keep in telemetry history")
	fs.StringVar(&cfg.webAddr, "web-addr", defaults.WebAddr, "Optional web telemetry listen address (e.g. :8080)")
	fs.StringVar(&cfg.logLevel, "log-level", defaults.LogLevel, "Log level (debug|info|warn|error)")
	fs.StringVar(&cfg.logFormat, "log-format", defaults.LogFormat, "Log format (text|json)")
	fs.BoolVar(&cfg.debugMode, "debug-mode", defaults.DebugMode, "Include debug telemetry fields")
	fs.BoolVar(&cfg.verbose, "verbose", false, "Enable verbose logging and debug output")

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
		SSHHost:        cfg.sshHost,
		SSHUser:        cfg.sshUser,
		SSHPassword:    cfg.sshPassword,
		SSHKeyPath:     cfg.sshKeyPath,
		SSHPort:        cfg.sshPort,
		SysfsRoot:      cfg.sysfsRoot,
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
		SSHPort:        22,
		SysfsRoot:      "/sys/bus/iio/devices",
	}
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

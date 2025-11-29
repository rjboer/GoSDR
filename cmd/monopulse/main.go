package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/rjboer/GoSDR/internal/app"
	"github.com/rjboer/GoSDR/internal/sdr"
	"github.com/rjboer/GoSDR/internal/telemetry"
)

func main() {
	const configPath = "config.json"

	persistentCfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	cfg, err := parseConfig(os.Args[1:], os.LookupEnv, persistentCfg)
	if err != nil {
		log.Fatalf("parse config: %v", err)
	}
	if err := saveConfig(configPath, persistentFromCLI(cfg)); err != nil {
		log.Fatalf("save config: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	backend, err := selectBackend(cfg)
	if err != nil {
		log.Fatalf("select backend: %v", err)
	}
	// Only use web telemetry (no stdout spam)
	var reporters []telemetry.Reporter
	if cfg.webAddr != "" {
		hub := telemetry.NewHub(cfg.historyLimit)
		reporters = append(reporters, hub)
		go telemetry.NewWebServer(cfg.webAddr, hub).Start(ctx)
		log.Printf("Web interface: http://localhost%s", cfg.webAddr)
	} else {
		// Fallback to stdout if no web interface
		reporters = append(reporters, telemetry.StdoutReporter{})
	}

	tracker := app.NewTracker(backend, telemetry.MultiReporter(reporters), app.Config{
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
	})

	if err := tracker.Init(ctx); err != nil {
		log.Fatalf("init tracker: %v", err)
	}

	// Run continuously (no timeout)
	log.Printf("Starting tracker (Ctrl+C to stop)...")
	if err := tracker.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("run tracker: %v", err)
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
	sdrBackend     string
	sdrURI         string
	warmupBuffers  int
	historyLimit   int
	webAddr        string
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
	SDRBackend     string  `json:"sdr_backend"`
	SDRURI         string  `json:"sdr_uri"`
	WarmupBuffers  int     `json:"warmup_buffers"`
	HistoryLimit   int     `json:"history_limit"`
	WebAddr        string  `json:"web_addr"`
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
	fs.StringVar(&cfg.sdrBackend, "sdr-backend", envString(lookup, "MONO_SDR_BACKEND", defaults.SDRBackend), "SDR backend (mock|pluto)")
	fs.StringVar(&cfg.sdrURI, "sdr-uri", envString(lookup, "MONO_SDR_URI", defaults.SDRURI), "SDR URI")
	fs.IntVar(&cfg.warmupBuffers, "warmup-buffers", envInt(lookup, "MONO_WARMUP_BUFFERS", defaults.WarmupBuffers), "Number of RX buffers to discard for warm-up")
	fs.IntVar(&cfg.historyLimit, "history-limit", envInt(lookup, "MONO_HISTORY_LIMIT", defaults.HistoryLimit), "Maximum samples to keep in telemetry history")
	fs.StringVar(&cfg.webAddr, "web-addr", envString(lookup, "MONO_WEB_ADDR", defaults.WebAddr), "Optional web telemetry listen address (e.g. :8080)")

	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}
	return cfg, nil
}

func persistentFromCLI(cfg cliConfig) persistentConfig {
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
		SDRBackend:     cfg.sdrBackend,
		SDRURI:         cfg.sdrURI,
		WarmupBuffers:  cfg.warmupBuffers,
		HistoryLimit:   cfg.historyLimit,
		WebAddr:        cfg.webAddr,
	}
}

func loadOrCreateConfig(path string) (persistentConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := defaultPersistentConfig()
			if saveErr := saveConfig(path, cfg); saveErr != nil {
				return persistentConfig{}, saveErr
			}
			return cfg, nil
		}
		return persistentConfig{}, err
	}
	defer f.Close()

	var cfg persistentConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return persistentConfig{}, err
	}
	return cfg, nil
}

func saveConfig(path string, cfg persistentConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
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

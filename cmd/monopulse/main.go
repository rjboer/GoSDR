package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/rjboer/GoSDR/internal/app"
	"github.com/rjboer/GoSDR/internal/sdr"
	"github.com/rjboer/GoSDR/internal/telemetry"
)

func main() {
	cfg, err := parseConfig(os.Args[1:], os.LookupEnv)
	if err != nil {
		log.Fatalf("parse config: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	backend, err := selectBackend(cfg)
	if err != nil {
		log.Fatalf("select backend: %v", err)
	}
	reporters := []telemetry.Reporter{telemetry.StdoutReporter{}}
	if cfg.webAddr != "" {
		hub := telemetry.NewHub(cfg.historyLimit)
		reporters = append(reporters, hub)
		go telemetry.NewWebServer(cfg.webAddr, hub).Start(ctx)
		log.Printf("web telemetry listening on %s", cfg.webAddr)
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

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.trackingLength)*50*time.Millisecond)
	defer cancel()
	if err := tracker.Run(runCtx); err != nil {
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

func parseConfig(args []string, lookup func(string) (string, bool)) (cliConfig, error) {
	cfg := cliConfig{}
	fs := flag.NewFlagSet("monopulse", flag.ContinueOnError)
	fs.Float64Var(&cfg.sampleRate, "sample-rate", envFloat(lookup, "MONO_SAMPLE_RATE", 2e6), "Sample rate in Hz")
	fs.Float64Var(&cfg.rxLO, "rx-lo", envFloat(lookup, "MONO_RX_LO", 2.3e9), "RX LO frequency in Hz")
	fs.IntVar(&cfg.rxGain0, "rx-gain0", envInt(lookup, "MONO_RX_GAIN0", 60), "RX gain for channel 0 (dB)")
	fs.IntVar(&cfg.rxGain1, "rx-gain1", envInt(lookup, "MONO_RX_GAIN1", 60), "RX gain for channel 1 (dB)")
	fs.IntVar(&cfg.txGain, "tx-gain", envInt(lookup, "MONO_TX_GAIN", -10), "TX gain (dB)")
	fs.Float64Var(&cfg.toneOffset, "tone-offset", envFloat(lookup, "MONO_TONE_OFFSET", 200e3), "Tone offset in Hz")
	fs.IntVar(&cfg.numSamples, "num-samples", envInt(lookup, "MONO_NUM_SAMPLES", 1<<12), "Number of samples per RX call")
	fs.IntVar(&cfg.trackingLength, "tracking-length", envInt(lookup, "MONO_TRACKING_LENGTH", 100), "Number of tracking iterations")
	fs.Float64Var(&cfg.phaseStep, "phase-step", envFloat(lookup, "MONO_PHASE_STEP", 1), "Phase step (degrees) for monopulse updates")
	fs.Float64Var(&cfg.phaseCal, "phase-cal", envFloat(lookup, "MONO_PHASE_CAL", 0), "Additional calibration phase (degrees)")
	fs.Float64Var(&cfg.scanStep, "scan-step", envFloat(lookup, "MONO_SCAN_STEP", 2), "Scan step in degrees for coarse search")
	fs.Float64Var(&cfg.spacing, "spacing-wavelength", envFloat(lookup, "MONO_SPACING_WAVELENGTH", 0.5), "Antenna spacing as a fraction of wavelength")
	fs.Float64Var(&cfg.phaseDelta, "mock-phase-delta", envFloat(lookup, "MONO_MOCK_PHASE_DELTA", 30), "Mock SDR phase delta in degrees")
	fs.StringVar(&cfg.sdrBackend, "sdr-backend", envString(lookup, "MONO_SDR_BACKEND", "mock"), "SDR backend (mock|pluto)")
	fs.StringVar(&cfg.sdrURI, "sdr-uri", envString(lookup, "MONO_SDR_URI", ""), "SDR URI")
	fs.IntVar(&cfg.warmupBuffers, "warmup-buffers", envInt(lookup, "MONO_WARMUP_BUFFERS", 3), "Number of RX buffers to discard for warm-up")
	fs.IntVar(&cfg.historyLimit, "history-limit", envInt(lookup, "MONO_HISTORY_LIMIT", 500), "Maximum samples to keep in telemetry history")
	fs.StringVar(&cfg.webAddr, "web-addr", envString(lookup, "MONO_WEB_ADDR", ""), "Optional web telemetry listen address (e.g. :8080)")

	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}
	return cfg, nil
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

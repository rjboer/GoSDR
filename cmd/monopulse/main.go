package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/rjboer/GoSDR/internal/app"
	"github.com/rjboer/GoSDR/internal/sdr"
	"github.com/rjboer/GoSDR/internal/telemetry"
)

func main() {
	cfg := parseFlags()
	ctx := context.Background()

	backend, err := selectBackend(cfg)
	if err != nil {
		log.Fatalf("select backend: %v", err)
	}
	reporter := telemetry.StdoutReporter{}
	tracker := app.NewTracker(backend, reporter, app.Config{
		SampleRate:        cfg.sampleRate,
		RxLO:              cfg.rxLO,
		ToneOffset:        cfg.toneOffset,
		NumSamples:        cfg.numSamples,
		SpacingWavelength: cfg.spacing,
		TrackingLength:    cfg.trackingLength,
		PhaseStep:         cfg.phaseStep,
		PhaseCal:          cfg.phaseCal,
		ScanStep:          cfg.scanStep,
		PhaseDelta:        cfg.phaseDelta,
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
}

func parseFlags() cliConfig {
	cfg := cliConfig{}
	flag.Float64Var(&cfg.sampleRate, "sample-rate", 2e6, "Sample rate in Hz")
	flag.Float64Var(&cfg.rxLO, "rx-lo", 2.3e9, "RX LO frequency in Hz")
	flag.Float64Var(&cfg.toneOffset, "tone-offset", 200e3, "Tone offset in Hz")
	flag.IntVar(&cfg.numSamples, "num-samples", 1<<12, "Number of samples per RX call")
	flag.IntVar(&cfg.trackingLength, "tracking-length", 100, "Number of tracking iterations")
	flag.Float64Var(&cfg.phaseStep, "phase-step", 1, "Phase step (degrees) for monopulse updates")
	flag.Float64Var(&cfg.phaseCal, "phase-cal", 0, "Additional calibration phase (degrees)")
	flag.Float64Var(&cfg.scanStep, "scan-step", 2, "Scan step in degrees for coarse search")
	flag.Float64Var(&cfg.spacing, "spacing-wavelength", 0.5, "Antenna spacing as a fraction of wavelength")
	flag.Float64Var(&cfg.phaseDelta, "mock-phase-delta", 30, "Mock SDR phase delta in degrees")
	flag.StringVar(&cfg.sdrBackend, "sdr-backend", "mock", "SDR backend (mock|pluto)")
	flag.StringVar(&cfg.sdrURI, "sdr-uri", "", "SDR URI")
	flag.Parse()
	return cfg
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

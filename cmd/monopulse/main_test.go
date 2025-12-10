package main

import (
	"reflect"
	"testing"
)

func TestParseConfigDefaults(t *testing.T) {
	defaults := defaultPersistentConfig()
	cfg, err := parseConfig([]string{}, defaults)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}
	if cfg.sampleRate != 2e6 || cfg.rxLO != 2.3e9 || cfg.numSamples != 1<<12 {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}

func TestParseConfigFlagOverrides(t *testing.T) {
	defaults := defaultPersistentConfig()
	cfg, err := parseConfig([]string{
		"--sample-rate", "1000000",
		"--rx-lo", "2300000001",
		"--sdr-backend", "pluto",
		"--num-samples", "2048",
		"--mock-phase-delta", "15",
		"--phase-step", "2",
	}, defaults)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}
	if cfg.sampleRate != 1e6 || cfg.rxLO != 2.300000001e9 || cfg.sdrBackend != "pluto" || cfg.numSamples != 2048 || cfg.phaseStep != 2 {
		t.Fatalf("flag overrides not applied: %#v", cfg)
	}
}

func TestSelectBackendError(t *testing.T) {
	if _, err := selectBackend(cliConfig{sdrBackend: "unknown"}); err == nil {
		t.Fatalf("expected error for unknown backend")
	}
}

func TestSelectBackendMock(t *testing.T) {
	backend, err := selectBackend(cliConfig{sdrBackend: "mock"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reflect.ValueOf(backend).IsNil() {
		t.Fatalf("backend should not be nil")
	}
}

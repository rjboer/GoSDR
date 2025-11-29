package main

import (
	"reflect"
	"testing"
)

func TestParseConfigDefaults(t *testing.T) {
	defaults := defaultPersistentConfig()
	cfg, err := parseConfig([]string{}, func(string) (string, bool) { return "", false }, defaults)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}
	if cfg.sampleRate != 2e6 || cfg.rxLO != 2.3e9 || cfg.numSamples != 1<<12 {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}

func TestParseConfigEnvOverrides(t *testing.T) {
	env := map[string]string{
		"MONO_SAMPLE_RATE":      "1000000",
		"MONO_RX_LO":            "2300000001",
		"MONO_SDR_BACKEND":      "pluto",
		"MONO_NUM_SAMPLES":      "2048",
		"MONO_MOCK_PHASE_DELTA": "15",
	}
	lookup := func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	}

	defaults := defaultPersistentConfig()
	cfg, err := parseConfig([]string{"--phase-step", "2"}, lookup, defaults)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}
	if cfg.sampleRate != 1e6 || cfg.rxLO != 2.300000001e9 || cfg.sdrBackend != "pluto" || cfg.numSamples != 2048 || cfg.phaseStep != 2 {
		t.Fatalf("env overrides not applied: %#v", cfg)
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

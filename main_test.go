package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/rjboer/GoSDR/iiod"
)

func TestRunParsesAddressFromFlagAndEnv(t *testing.T) {
	mockedDial := func(addr string) (*iiod.Client, error) {
		return nil, errors.New(addr)
	}
	prevDial := dial
	dial = mockedDial
	defer func() { dial = prevDial }()

	buf := &strings.Builder{}
	getenv := func(key string) string {
		if key == "IIOD_ADDR" {
			return "env:1234"
		}
		return ""
	}

	err := run([]string{"--iiod-addr", "flag:5678"}, buf, getenv)
	if err == nil || !strings.Contains(err.Error(), "flag:5678") {
		t.Fatalf("expected dial to receive flag address, got %v", err)
	}

	err = run(nil, buf, getenv)
	if err == nil || !strings.Contains(err.Error(), "env:1234") {
		t.Fatalf("expected dial to receive env address, got %v", err)
	}
}

func TestRunHandlesDialError(t *testing.T) {
	mockedDial := func(string) (*iiod.Client, error) {
		return nil, errors.New("dial failed")
	}
	prevDial := dial
	dial = mockedDial
	defer func() { dial = prevDial }()

	if err := run(nil, &strings.Builder{}, func(string) string { return "" }); err == nil || !strings.Contains(err.Error(), "dial failed") {
		t.Fatalf("expected dial error, got %v", err)
	}
}

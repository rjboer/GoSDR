package telemetry

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rjboer/GoSDR/internal/logging"
)

func newTestHub() *Hub {
	return NewHub(10, logging.New(logging.Debug, logging.Text, io.Discard))
}

func TestHandleDiagnosticsReturnsMetricsAndSpectrum(t *testing.T) {
	hub := newTestHub()
	hub.UpdateSpectrumSnapshot([]float64{1, 2, 3, 4}, "test-source")

	req := httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil)
	rr := httptest.NewRecorder()

	hub.handleDiagnostics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp Diagnostics
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Process.NumGoroutine == 0 {
		t.Fatal("expected goroutine count to be reported")
	}
	if resp.Process.Uptime <= 0 {
		t.Fatal("expected positive uptime")
	}
	if len(resp.Spectrum.Bins) != 4 {
		t.Fatalf("expected 4 spectrum bins, got %d", len(resp.Spectrum.Bins))
	}
	if resp.Spectrum.Source != "test-source" {
		t.Fatalf("expected spectrum source 'test-source', got %q", resp.Spectrum.Source)
	}
}

func TestHandleDiagnosticsMethodNotAllowed(t *testing.T) {
	hub := newTestHub()
	req := httptest.NewRequest(http.MethodPost, "/api/diagnostics", nil)
	rr := httptest.NewRecorder()

	hub.handleDiagnostics(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestHandleSpectrumSnapshot(t *testing.T) {
	hub := newTestHub()
	bins := []float64{-1, -2, -3}
	hub.UpdateSpectrumSnapshot(bins, "live")

	req := httptest.NewRequest(http.MethodGet, "/api/diagnostics/spectrum", nil)
	rr := httptest.NewRecorder()

	hub.handleSpectrumSnapshot(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp SpectrumSnapshot
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(resp.Bins) != len(bins) {
		t.Fatalf("expected %d bins, got %d", len(bins), len(resp.Bins))
	}
	if resp.Source != "live" {
		t.Fatalf("expected source 'live', got %q", resp.Source)
	}
}

func TestHandleSpectrumSnapshotMethodNotAllowed(t *testing.T) {
	hub := newTestHub()
	req := httptest.NewRequest(http.MethodPost, "/api/diagnostics/spectrum", nil)
	rr := httptest.NewRecorder()

	hub.handleSpectrumSnapshot(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestHandleHealthReportsMockAndLiveData(t *testing.T) {
	hub := newTestHub()

	mockReq := httptest.NewRequest(http.MethodGet, "/api/diagnostics/health", nil)
	mockRR := httptest.NewRecorder()
	hub.handleHealth(mockRR, mockReq)

	var mockResp HealthStatus
	if err := json.NewDecoder(mockRR.Body).Decode(&mockResp); err != nil {
		t.Fatalf("decode mock response: %v", err)
	}
	if mockResp.Status != "degraded" {
		t.Fatalf("expected degraded status for mock data, got %q", mockResp.Status)
	}
	if mockResp.Process.Uptime <= 0 {
		t.Fatal("expected uptime in mock health response")
	}

	hub.UpdateSpectrumSnapshot([]float64{0.1, 0.2}, "live")
	liveReq := httptest.NewRequest(http.MethodGet, "/api/diagnostics/health", nil)
	liveRR := httptest.NewRecorder()
	hub.handleHealth(liveRR, liveReq)

	var liveResp HealthStatus
	if err := json.NewDecoder(liveRR.Body).Decode(&liveResp); err != nil {
		t.Fatalf("decode live response: %v", err)
	}
	if liveResp.Status != "ok" {
		t.Fatalf("expected ok status for live data, got %q", liveResp.Status)
	}
	if liveResp.Process.NumGoroutine == 0 {
		t.Fatal("expected goroutine count in live health response")
	}
}

func TestHandleHealthMethodNotAllowed(t *testing.T) {
	hub := newTestHub()
	req := httptest.NewRequest(http.MethodPost, "/api/diagnostics/health", nil)
	rr := httptest.NewRecorder()

	hub.handleHealth(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

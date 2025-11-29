# GoSDR Debug Capabilities - Enhancement Guide

## Current Debug Limitations

**What's Missing:**
- ❌ No structured logging (just `log.Printf`)
- ❌ No debug/verbose modes
- ❌ Limited visibility into DSP internals
- ❌ No performance profiling built-in
- ❌ No diagnostic endpoints
- ❌ No signal quality metrics

---

## Recommended Debug Enhancements

### 1. Structured Logging with Levels

**Why:** Different verbosity levels for development vs. production

**Implementation:**

```go
// internal/logging/logger.go
package logging

import (
    "log/slog"
    "os"
)

type Level int

const (
    LevelDebug Level = iota
    LevelInfo
    LevelWarn
    LevelError
)

var logger *slog.Logger

func Init(level Level) {
    opts := &slog.HandlerOptions{
        Level: slog.Level(level),
    }
    logger = slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

func Debug(msg string, args ...any) {
    logger.Debug(msg, args...)
}

func Info(msg string, args ...any) {
    logger.Info(msg, args...)
}

func Warn(msg string, args ...any) {
    logger.Warn(msg, args...)
}

func Error(msg string, args ...any) {
    logger.Error(msg, args...)
}
```

**Usage:**

```go
// In tracker.go
logging.Debug("RX buffer received",
    "samples", len(rx0),
    "iteration", i,
    "phase_delay", t.lastDelay)

logging.Info("Coarse scan complete",
    "angle", theta,
    "peak", peak,
    "duration_ms", elapsed.Milliseconds())
```

**CLI Flag:**
```bash
go run main.go --log-level=debug
```

**Output:**
```json
{"time":"2025-11-29T13:30:00Z","level":"DEBUG","msg":"RX buffer received","samples":4096,"iteration":5,"phase_delay":30.5}
{"time":"2025-11-29T13:30:01Z","level":"INFO","msg":"Coarse scan complete","angle":30.2,"peak":-15.3,"duration_ms":45}
```

---

### 2. Debug Mode with Enhanced Telemetry

**Why:** Expose internal DSP state for troubleshooting

**Add to telemetry Sample:**

```go
// internal/telemetry/hub.go
type Sample struct {
    Timestamp time.Time `json:"timestamp"`
    AngleDeg  float64   `json:"angleDeg"`
    Peak      float64   `json:"peak"`
    
    // Debug fields (only populated in debug mode)
    Debug *DebugInfo `json:"debug,omitempty"`
}

type DebugInfo struct {
    PhaseDelay      float64   `json:"phaseDelay"`
    MonopulsePhase  float64   `json:"monopulsePhase"`
    SumPeak         float64   `json:"sumPeak"`
    DeltaPeak       float64   `json:"deltaPeak"`
    SignalBins      []int     `json:"signalBins"`
    SpectrumSnap    []float64 `json:"spectrumSnap,omitempty"` // Optional: full spectrum
    IterationTime   float64   `json:"iterationTimeMs"`
}
```

**Enable via config:**
```json
{
  "telemetry": {
    "debugMode": true,
    "captureSpectrum": false
  }
}
```

**Web UI Enhancement:**
Add debug panel that shows:
- Current phase delay
- Monopulse error signal
- Sum/Delta peak levels
- Processing time per iteration

---

### 3. Signal Quality Metrics

**Why:** Understand tracking confidence and signal conditions

**Add metrics:**

```go
type SignalQuality struct {
    SNR          float64 `json:"snr"`           // Estimated SNR
    Confidence   float64 `json:"confidence"`    // Tracking confidence (0-1)
    NoiseFloor   float64 `json:"noiseFloor"`    // Estimated noise floor
    SignalPower  float64 `json:"signalPower"`   // Signal power
    LockStatus   string  `json:"lockStatus"`    // "locked", "tracking", "searching"
}

// Calculate SNR from spectrum
func estimateSNR(spectrum []float64, signalBins []int) float64 {
    // Signal: peak in signal bins
    signal := -math.MaxFloat64
    for _, bin := range signalBins {
        if spectrum[bin] > signal {
            signal = spectrum[bin]
        }
    }
    
    // Noise: median of non-signal bins
    noise := estimateNoiseFloor(spectrum, signalBins)
    
    return signal - noise // dB difference
}
```

**Display in web UI:**
```
Signal Quality
├─ SNR: 18.5 dB
├─ Confidence: 95%
├─ Lock Status: LOCKED
└─ Noise Floor: -45.2 dBFS
```

---

### 4. Performance Profiling

**Why:** Identify bottlenecks and optimize hot paths

**Built-in pprof endpoints:**

```go
// cmd/monopulse/main.go
import (
    "net/http"
    _ "net/http/pprof"
)

func main() {
    // ... existing code ...
    
    if cfg.enablePprof {
        go func() {
            log.Println("pprof server listening on :6060")
            log.Println(http.ListenAndServe(":6060", nil))
        }()
    }
}
```

**Usage:**
```bash
# Run with profiling enabled
go run main.go --enable-pprof

# CPU profile (30 seconds)
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Memory profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutine profile
go tool pprof http://localhost:6060/debug/pprof/goroutine
```

**Analyze:**
```bash
# Interactive analysis
(pprof) top10
(pprof) list FFTAndDBFS
(pprof) web  # Opens browser visualization
```

---

### 5. Diagnostic Endpoints

**Why:** Runtime inspection without stopping the tracker

**Add to web server:**

```go
// internal/telemetry/webserver.go

// GET /api/diagnostics
func (h *Hub) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
    diag := Diagnostics{
        Uptime:          time.Since(h.startTime),
        TotalSamples:    h.totalSamples,
        DroppedSamples:  h.droppedSamples,
        AvgIterationMs:  h.avgIterationTime,
        MemoryUsageMB:   getMemoryUsage(),
        GoroutineCount:  runtime.NumGoroutine(),
        CPUCores:        runtime.NumCPU(),
        GoVersion:       runtime.Version(),
    }
    
    json.NewEncoder(w).Encode(diag)
}

// GET /api/diagnostics/spectrum
func (h *Hub) handleSpectrumSnapshot(w http.ResponseWriter, r *http.Request) {
    // Return latest FFT spectrum for analysis
    spectrum := h.getLatestSpectrum()
    json.NewEncoder(w).Encode(spectrum)
}

// GET /api/diagnostics/health
func (h *Hub) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
    health := HealthStatus{
        Status:    "healthy",
        Tracking:  h.isTracking,
        LastError: h.lastError,
        Checks: map[string]bool{
            "sdr_connected":    h.sdrConnected,
            "tracking_active":  h.trackingActive,
            "web_server_up":    true,
        },
    }
    
    if !health.Checks["sdr_connected"] {
        health.Status = "degraded"
    }
    
    json.NewEncoder(w).Encode(health)
}
```

**Access:**
```bash
# System diagnostics
curl http://localhost:8080/api/diagnostics

# Current spectrum
curl http://localhost:8080/api/diagnostics/spectrum

# Health check
curl http://localhost:8080/api/diagnostics/health
```

---

### 6. Trace Recording

**Why:** Capture detailed execution traces for analysis

**Implementation:**

```go
// internal/tracing/trace.go
package tracing

import (
    "encoding/json"
    "os"
    "sync"
    "time"
)

type Event struct {
    Timestamp time.Time              `json:"timestamp"`
    Type      string                 `json:"type"`
    Data      map[string]interface{} `json:"data"`
}

type Recorder struct {
    mu     sync.Mutex
    events []Event
    file   *os.File
}

func NewRecorder(path string) (*Recorder, error) {
    f, err := os.Create(path)
    if err != nil {
        return nil, err
    }
    return &Recorder{file: f}, nil
}

func (r *Recorder) Record(eventType string, data map[string]interface{}) {
    event := Event{
        Timestamp: time.Now(),
        Type:      eventType,
        Data:      data,
    }
    
    r.mu.Lock()
    defer r.mu.Unlock()
    
    json.NewEncoder(r.file).Encode(event)
}

func (r *Recorder) Close() error {
    return r.file.Close()
}
```

**Usage:**

```go
// In tracker
if cfg.enableTracing {
    tracer, _ := tracing.NewRecorder("trace.jsonl")
    defer tracer.Close()
    
    tracer.Record("coarse_scan_start", map[string]interface{}{
        "scan_step": cfg.ScanStep,
    })
    
    // ... do coarse scan ...
    
    tracer.Record("coarse_scan_complete", map[string]interface{}{
        "angle":      theta,
        "peak":       peak,
        "duration_ms": elapsed.Milliseconds(),
    })
}
```

**Analyze traces:**
```bash
# View trace file
cat trace.jsonl | jq .

# Filter specific events
cat trace.jsonl | jq 'select(.type == "coarse_scan_complete")'

# Calculate average duration
cat trace.jsonl | jq -s 'map(select(.type == "iteration")) | map(.data.duration_ms) | add / length'
```

---

### 7. Web-Based Debug Dashboard

**Why:** Visual debugging without command-line tools

**Add debug page:** `http://localhost:8080/debug`

**Features:**
- **Live Spectrum Viewer** - Real-time FFT waterfall display
- **Phase Tracking Plot** - Phase delay over time
- **Monopulse Error Plot** - Error signal visualization
- **Performance Metrics** - CPU, memory, iteration time
- **Event Log** - Scrolling log of recent events
- **Configuration Inspector** - Current settings display

**Example HTML:**

```html
<div class="debug-dashboard">
  <div class="spectrum-viewer">
    <canvas id="spectrumCanvas"></canvas>
    <div class="controls">
      <button id="pauseBtn">Pause</button>
      <button id="exportBtn">Export Data</button>
    </div>
  </div>
  
  <div class="metrics-panel">
    <h3>Performance</h3>
    <div class="metric">
      <span>Iteration Time:</span>
      <span id="iterTime">12.5 ms</span>
    </div>
    <div class="metric">
      <span>CPU Usage:</span>
      <span id="cpuUsage">35%</span>
    </div>
    <div class="metric">
      <span>Memory:</span>
      <span id="memUsage">45 MB</span>
    </div>
  </div>
  
  <div class="event-log">
    <h3>Event Log</h3>
    <div id="logEntries" class="log-scroll"></div>
  </div>
</div>
```

---

### 8. Automated Testing & Validation

**Why:** Catch regressions and validate behavior

**Add test modes:**

```go
// internal/testing/validation.go

type ValidationTest struct {
    Name     string
    Input    TestSignal
    Expected ExpectedResult
}

type TestSignal struct {
    Angle      float64
    SNR        float64
    Frequency  float64
    Duration   time.Duration
}

type ExpectedResult struct {
    AngleTolerance float64 // ±degrees
    MinSNR         float64
    MaxLockTime    time.Duration
}

func RunValidation(tracker *app.Tracker, test ValidationTest) ValidationResult {
    // Generate test signal
    mockSDR := sdr.NewMockWithSignal(test.Input)
    
    // Run tracker
    start := time.Now()
    tracker.Run(context.Background())
    lockTime := time.Since(start)
    
    // Validate results
    finalAngle := tracker.LastAngle()
    angleError := math.Abs(finalAngle - test.Input.Angle)
    
    return ValidationResult{
        Passed:     angleError <= test.Expected.AngleTolerance,
        AngleError: angleError,
        LockTime:   lockTime,
    }
}
```

**Run validation suite:**
```bash
go run main.go --run-validation

# Output:
# ✓ Test 1: 0° signal (SNR=20dB) - PASS (error: 0.3°, lock: 45ms)
# ✓ Test 2: 45° signal (SNR=15dB) - PASS (error: 1.2°, lock: 67ms)
# ✗ Test 3: -60° signal (SNR=10dB) - FAIL (error: 5.1°, lock: 120ms)
```

---

### 9. Configuration Validation

**Why:** Catch invalid configurations before they cause issues

```go
// internal/config/validator.go

type ValidationError struct {
    Field   string
    Message string
}

func ValidateConfig(cfg Config) []ValidationError {
    var errors []ValidationError
    
    // Sample rate checks
    if cfg.SampleRate < 1e3 || cfg.SampleRate > 61.44e6 {
        errors = append(errors, ValidationError{
            Field:   "sampleRate",
            Message: "must be between 1 kHz and 61.44 MHz",
        })
    }
    
    // FFT size must be power of 2
    if !isPowerOfTwo(cfg.NumSamples) {
        errors = append(errors, ValidationError{
            Field:   "numSamples",
            Message: "must be a power of 2",
        })
    }
    
    // Antenna spacing reasonable
    if cfg.SpacingWavelength < 0.1 || cfg.SpacingWavelength > 2.0 {
        errors = append(errors, ValidationError{
            Field:   "spacingWavelength",
            Message: "should be between 0.1 and 2.0 wavelengths",
        })
    }
    
    return errors
}
```

**Display in web UI:**
```
⚠️ Configuration Warnings:
• Sample rate (100 kHz) is very low - tracking may be slow
• Antenna spacing (2.5λ) is unusual - expect ambiguities
```

---

## Implementation Priority

### High Priority (Implement First)
1. **Structured Logging** - Foundation for all debugging
2. **Debug Mode** - Enhanced telemetry
3. **Diagnostic Endpoints** - Runtime inspection

### Medium Priority
4. **Signal Quality Metrics** - Tracking confidence
5. **Performance Profiling** - Built-in pprof
6. **Configuration Validation** - Prevent errors

### Low Priority (Nice to Have)
7. **Trace Recording** - Detailed analysis
8. **Debug Dashboard** - Visual debugging
9. **Validation Suite** - Automated testing

---

## Quick Wins

### 1. Add `--verbose` Flag (30 minutes)

```go
// cmd/monopulse/main.go
verbose := flag.Bool("verbose", false, "Enable verbose logging")

if *verbose {
    log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
}
```

### 2. Add Iteration Timing (15 minutes)

```go
// internal/app/tracker.go
start := time.Now()
// ... do tracking iteration ...
elapsed := time.Since(start)

if cfg.Verbose {
    log.Printf("[PERF] Iteration %d: %v", i, elapsed)
}
```

### 3. Add Error Context (20 minutes)

```go
// Wrap errors with context
if err := t.sdr.RX(ctx); err != nil {
    return fmt.Errorf("RX iteration %d: %w", i, err)
}
```

---

## Example: Complete Debug Session

```bash
# 1. Start with debug logging
go run main.go --log-level=debug --enable-pprof

# 2. Monitor in another terminal
curl http://localhost:8080/api/diagnostics | jq .

# 3. Check health
curl http://localhost:8080/api/diagnostics/health

# 4. Capture CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# 5. View spectrum
curl http://localhost:8080/api/diagnostics/spectrum > spectrum.json

# 6. Analyze logs
tail -f logs.jsonl | jq 'select(.level == "ERROR")'
```

---

## Summary

**Best Debug Improvements:**
1. ✅ Structured logging with levels
2. ✅ Debug mode with enhanced telemetry
3. ✅ Diagnostic API endpoints
4. ✅ Built-in profiling (pprof)
5. ✅ Signal quality metrics
6. ✅ Configuration validation

**Expected Benefits:**
- 🐛 Faster bug identification
- 📊 Better performance visibility
- 🔍 Easier troubleshooting
- ✅ Proactive error detection
- 📈 Production monitoring capability

Start with structured logging and diagnostic endpoints - they provide the most value with minimal effort!

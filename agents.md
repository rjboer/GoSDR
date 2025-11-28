# Agent: Go Monopulse DOA Tracker Port

## Project Overview

You are working on a Go port of an existing Python script that implements a real-time **monopulse direction-of-arrival (DOA) tracker** using an **AD9361-based SDR** (e.g. ADALM-Pluto / FMCOMMS).

The Python script:

- Configures an AD9361 SDR with **two RX channels** at **2.3 GHz**.
- Transmits a complex tone at an offset frequency `fc0` (≈200 kHz).
- Receives IQ samples from both antennas.
- Performs:
  - Sum (Σ) and difference (Δ) beamforming in complex baseband.
  - A coarse scan over synthetic phase delays to find the initial DOA.
  - A **monopulse tracking loop**, adjusting the phase delay based on the monopulse error sign.
- Visualizes **steering angle vs time** with pyqtgraph.

The goal is to reproduce this behaviour in **pure Go**, with a clean architecture, testable DSP logic, and a usable runtime interface (CLI + simple visualization).

---

## High-Level Goals

1. **Port core DSP logic** from Python to Go:
   - Angle/phase conversion.
   - FFT-based power estimation and dBFS scaling.
   - Coarse scan for initial DOA.
   - Monopulse tracking loop.

2. **Abstract SDR I/O** behind an interface:
   - Allow plugging in:
     - Real AD9361/Pluto hardware via libiio/iiod or similar.
     - A simulation / replay source for development and testing.

3. **Provide a minimal visualization / telemetry path**:
   - At minimum, print current steering angle and peak level to stdout.
   - Optionally, stream data to a simple HTTP/WS endpoint for plotting in a browser.

4. **Make the system configurable and repeatable**:
   - CLI flags or config file for LO frequency, tone offset, spacing, gains, sample rate, tracking parameters.

5. **Keep the code idiomatic and maintainable**:
   - Clear package boundaries, unit tests for pure DSP, and no unnecessary dependencies.

---

## Constraints and Preferences

- Language: **Go** (latest stable).
- Style: idiomatic Go, small interfaces, focus on clarity over cleverness.
- Concurrency: use goroutines/channels where it simplifies SDR read / tracking loop, but avoid over-engineering.
- External libs: keep dependencies minimal. Use well-maintained DSP libs where necessary (FFT), but keep the abstraction so they can be swapped.
- Platform targets: at least Linux/amd64; preferable to keep code portable to Windows/macOS where possible.

---

## Proposed Architecture

**Module layout:**

- `cmd/monopulse/`
  - `main.go` — CLI entry point, wiring, config, logging.
- `internal/sdr/`
  - `sdr.go` — `SDR` interface (Init, RX, TX, Close).
  - `pluto.go` — real hardware implementation (future / optional).
  - `mock.go` — simulated SDR (for local dev and tests).
- `internal/dsp/`
  - `window.go` — Hamming window and helpers.
  - `fft.go` — FFT wrapper and dBFS conversion.
  - `angle.go` — phase ↔ angle math (`calcTheta` equivalent).
  - `monopulse.go` — scan and tracking logic.
- `internal/app/`
  - `tracker.go` — orchestration (looping over RX, calling DSP, updating angle).
- `internal/telemetry/` (optional)
  - `stdout.go` — print updates to console.
  - `http.go` — simple HTTP/WebSocket server for live plotting.

You may adjust package names as needed, but **keep DSP logic separated from SDR and I/O**.

---

## Functional Requirements

### 1. Configuration

Expose configuration via CLI flags and/or environment variables such as:

- `--sample-rate` (Hz)
- `--rx-lo` (Hz)
- `--tone-offset` (Hz, e.g. `200000`)
- `--rx-gain0`, `--rx-gain1` (dB)
- `--tx-gain` (dB)
- `--spacing-wavelength` (fraction, default `0.5`)
- `--num-samples` per RX call (power of two)
- `--tracking-length` (buffer length for history)
- `--phase-step` (deg for monopulse update)
- `--sdr-uri` / `--sdr-backend` (e.g. `pluto`, `mock`)

### 2. SDR Interface

Define a small interface, e.g.:

```go
type SDR interface {
    Init(cfg Config) error
    TX(iq0, iq1 []complex64) error
    RX() (chan0, chan1 []complex64, err error)
    Close() error
}
```

Implement at least:

- **MockSDR**: returns synthetic IQ data (e.g. a tone at `fc0` with controllable phase offset between channels).
- **PlutoSDR** (optional/future): wraps `libiio` or calls `iiod` via a Go binding.

### 3. DSP Functions

Implement the following core DSP functions in `internal/dsp/`:

#### `window.go`

```go
// Hamming returns a Hamming window of length n
func Hamming(n int) []float64
```

#### `fft.go`

```go
// DBFS converts IQ samples to dBFS spectrum
// Returns: fftShift (complex spectrum), dbfs (magnitude in dBFS)
func DBFS(samples []complex64, window []float64) ([]complex128, []float64)
```

Use a Go FFT library (e.g., `gonum.org/v1/gonum/dsp/fourier` or `github.com/mjibson/go-dsp/fft`).

- Apply Hamming window
- Compute FFT
- Shift zero frequency to center
- Convert to dBFS: `20*log10(abs(fft_shift) / 2^11)` (Pluto is 12-bit signed ADC)

#### `angle.go`

```go
// CalcTheta converts phase delay (degrees) to steering angle (degrees)
// Formula: theta = arcsin(c * deg2rad(phase) / (2*pi*f*d))
func CalcTheta(phaseDeg float64, freqHz float64, distanceM float64) float64

// PhaseShift applies a phase delay to IQ samples
func PhaseShift(samples []complex64, phaseDeg float64) []complex64
```

#### `monopulse.go`

```go
// MonopulseAngle computes the monopulse error angle from sum and delta FFTs
// Correlates a slice of the FFT around the signal of interest
func MonopulseAngle(sumFFT, deltaFFT []complex128, startBin, endBin int) float64

// ScanForDOA performs a coarse phase scan to find initial direction of arrival
// Returns: peakDelay (deg), peakDBFS, steerAngle (deg)
func ScanForDOA(rx0, rx1 []complex64, cfg ScanConfig) (peakDelay, peakDBFS, steerAngle float64)

// Tracking performs one monopulse tracking iteration
// Returns: new phase delay (deg)
func Tracking(rx0, rx1 []complex64, lastDelay float64, cfg TrackConfig) float64
```

### 4. Main Application Loop

In `internal/app/tracker.go`:

```go
type Tracker struct {
    sdr    sdr.SDR
    cfg    Config
    delay  float64  // current phase delay in degrees
    angles []float64 // tracking history
}

func (t *Tracker) Initialize() error
func (t *Tracker) CoarseScan() error
func (t *Tracker) Track() error  // runs one tracking iteration
func (t *Tracker) Run(ctx context.Context) error  // main loop
```

**Flow:**

1. Initialize SDR and transmit tone
2. Warm up: discard first ~20 RX buffers
3. Run `CoarseScan()` to get initial DOA
4. Loop: call `Track()` repeatedly, updating `delay` and appending to `angles`
5. Optionally send telemetry updates

### 5. Telemetry / Visualization

Provide at least **stdout logging**:

```
[2025-11-28 19:54:00] Angle: +12.3°  Delay: +45.2°  Peak: -23.4 dBFS
```

**Optional HTTP/WebSocket server** (`internal/telemetry/http.go`):

- Serve a simple HTML page with a chart (e.g. using Chart.js or similar)
- WebSocket endpoint streams `{timestamp, angle, delay, peak}` JSON
- Client plots angle vs time in real-time

---

## Implementation Workflow

Follow these steps to build the system incrementally:

### Phase 1: DSP Core (No Hardware)

1. **Create project structure**:
   ```
   mkdir -p cmd/monopulse internal/{sdr,dsp,app,telemetry}
   go mod init github.com/yourusername/gosdr
   ```

2. **Implement DSP functions** in `internal/dsp/`:
   - `window.go` → Hamming window
   - `fft.go` → DBFS conversion
   - `angle.go` → CalcTheta, PhaseShift
   - `monopulse.go` → MonopulseAngle, ScanForDOA, Tracking

3. **Write unit tests** for each DSP function:
   - Test Hamming window shape
   - Test FFT with known tone frequency
   - Test CalcTheta with known phase/angle pairs
   - Test PhaseShift preserves magnitude
   - Test MonopulseAngle with synthetic sum/delta signals

4. **Create MockSDR** in `internal/sdr/mock.go`:
   - Generate two channels of IQ data
   - Channel 0: tone at `fc0`
   - Channel 1: same tone with configurable phase offset
   - Add optional noise for realism

### Phase 2: Integration & CLI

5. **Implement SDR interface** in `internal/sdr/sdr.go`

6. **Build Tracker** in `internal/app/tracker.go`:
   - Wire SDR + DSP functions
   - Implement Initialize, CoarseScan, Track, Run

7. **Create CLI** in `cmd/monopulse/main.go`:
   - Parse flags (sample rate, LO freq, gains, etc.)
   - Initialize Tracker with MockSDR
   - Run tracking loop
   - Print angles to stdout

8. **Test end-to-end** with MockSDR:
   - Set mock to return fixed phase offset
   - Verify coarse scan finds correct angle
   - Verify tracking loop converges

### Phase 3: Telemetry & Visualization (Optional)

9. **Add stdout logger** in `internal/telemetry/stdout.go`

10. **Add HTTP/WebSocket server** in `internal/telemetry/http.go`:
    - Serve static HTML with real-time chart
    - Stream tracking data via WebSocket

11. **Integrate telemetry** into Tracker.Run()

### Phase 4: Real Hardware (Future)

12. **Implement PlutoSDR** in `internal/sdr/pluto.go`:
    - Use Go bindings for libiio (e.g., via cgo or external process)
    - Implement Init, TX, RX, Close
    - Handle buffer management and calibration

13. **Test with real Pluto**:
    - Run with `--sdr-backend=pluto --sdr-uri=ip:192.168.2.1`
    - Verify tracking with real RF signals

---

## Testing Requirements

### Unit Tests

- **DSP functions**: Test with known inputs/outputs
  - Hamming window: verify shape and sum
  - DBFS: verify FFT of known tone gives expected peak
  - CalcTheta: verify angle calculation for known phase delays
  - PhaseShift: verify phase rotation is correct
  - MonopulseAngle: verify correlation and angle extraction

### Integration Tests

- **MockSDR + Tracker**: 
  - Test coarse scan finds correct initial angle
  - Test tracking loop converges to target angle
  - Test tracking follows changing angle (mock can vary phase over time)

### Manual Testing

- **With real hardware** (when available):
  - Verify SDR configuration (gains, LO, sample rate)
  - Verify TX tone is transmitted
  - Verify RX channels receive data
  - Verify tracking responds to physical antenna movement

---

## Reference Information

### Key Formulas

**Steering angle from phase delay:**

```
theta = arcsin(c * phase_rad / (2 * pi * f * d))
```

Where:
- `c` = speed of light (3e8 m/s)
- `phase_rad` = phase delay in radians
- `f` = RF carrier frequency (Hz)
- `d` = antenna spacing (meters)

**Antenna spacing:**

```
d = d_wavelength * (c / f)
```

Typically `d_wavelength = 0.5` (half wavelength spacing)

**dBFS conversion:**

```
dBFS = 20 * log10(abs(fft_value) / 2^11)
```

For a 12-bit signed ADC (Pluto AD9361)

**Monopulse error:**

```
error = angle(correlate(sum_fft[bins], delta_fft[bins]))
```

The sign of this error drives the tracking loop update.

### Python → Go Translation Notes

| Python | Go Equivalent |
|--------|---------------|
| `np.exp(1j * phase)` | `cmplx.Exp(complex(0, phase))` |
| `np.deg2rad(x)` | `x * math.Pi / 180` |
| `np.rad2deg(x)` | `x * 180 / math.Pi` |
| `np.fft.fft(y)` | Use `gonum` or `go-dsp/fft` |
| `np.fft.fftshift(s)` | Manual shift: `append(s[n/2:], s[:n/2]...)` |
| `np.hamming(n)` | Implement or use DSP library |
| `np.correlate(a, b, 'valid')` | Manual correlation or use DSP library |
| `np.angle(z)` | `cmplx.Phase(z)` |
| `np.sign(x)` | `math.Copysign(1, x)` |

### Configuration Example

```go
type Config struct {
    SampleRate       float64
    RxLO             float64
    TxLO             float64
    ToneOffset       float64  // fc0
    RxGain0          int
    RxGain1          int
    TxGain           int
    NumSamples       int
    SpacingWavelength float64
    TrackingLength   int
    PhaseStep        float64  // degrees per tracking update
    SDRBackend       string   // "mock" or "pluto"
    SDRURI           string   // e.g. "ip:192.168.2.1"
}
```

---

## Development Tips

1. **Start simple**: Get DSP working with unit tests before integrating SDR
2. **Use MockSDR extensively**: Much faster iteration than real hardware
3. **Log liberally**: Print intermediate values during development
4. **Visualize**: Even simple stdout plots help debug tracking behavior
5. **Keep interfaces small**: Easy to test, easy to swap implementations
6. **Avoid premature optimization**: Clarity first, performance later
7. **Document assumptions**: Note any differences from Python implementation

---

## Success Criteria

The port is successful when:

- [ ] All DSP functions have unit tests and pass
- [ ] MockSDR generates realistic two-channel IQ data
- [ ] Coarse scan finds correct DOA from mock data
- [ ] Tracking loop converges and follows angle changes
- [ ] CLI runs end-to-end with configurable parameters
- [ ] Telemetry shows angle vs time (stdout or web)
- [ ] Code is idiomatic Go with clear package boundaries
- [ ] (Optional) Real Pluto hardware integration works

---

## Next Steps

Once you have this working:

1. **Optimize performance**: Profile and optimize hot paths (FFT, correlation)
2. **Add more SDR backends**: Support other AD9361 platforms (FMCOMMS, etc.)
3. **Enhance visualization**: Add spectrum plots, sum/delta patterns
4. **Add calibration**: Phase calibration, gain mismatch correction
5. **Multi-target tracking**: Extend to track multiple signals
6. **Record/replay**: Save IQ data for offline analysis

---

## Questions?

If you encounter issues or need clarification:

- Check the Python reference implementation in `python.py`
- Review DSP theory: monopulse tracking, beamforming, FFT
- Consult AD9361 documentation for hardware details
- Test with MockSDR first to isolate DSP vs hardware issues

Good luck with the port! 🚀

## `README.md`

```markdown
# Go Monopulse DOA Tracker (AD9361 / Pluto)

A Go port of a Python script that implements a real-time **monopulse direction-of-arrival (DOA) tracker** using an **AD9361-based SDR** (e.g. ADALM-Pluto / FMCOMMS).

The application:

- Transmits a continuous complex tone at an offset from the LO.
- Receives IQ samples on **two RX channels** (two antennas) at a known spacing.
- Forms **sum** (Σ) and **difference** (Δ) channels.
- Runs a coarse **phase scan** to estimate the initial direction of arrival.
- Uses a **monopulse tracking loop** to follow angle changes over time.
- Logs the **steering angle vs time**, and can be extended to provide real-time visualization.

---

## Background

The original Python script:

- Uses `adi` (Analog Devices IIO bindings), `numpy`, and `pyqtgraph`.
- Configures an AD9361 at:
  - LO: `rx_lo = 2.3e9` Hz
  - Sample rate: `2e6` samples/s
  - Tone offset: `fc0 = 200e3` Hz
- Sets antenna spacing to half wavelength (`d = 0.5 * λ`).
- Computes the DOA from the phase difference between the two RX channels:
  - Coarse scan: sweep synthetic phase delay from −180° to +180°.
  - Tracking: monopulse error (phase of Σ/Δ correlation) drives a phase-delay update loop.

This Go project recreates that behaviour with an emphasis on:

- Clear DSP and geometry math.
- Testability (mock SDR input).
- Clean separation between hardware access and algorithms.

---






## Project Structure

```text
.
├── cmd/
│   └── monopulse/        # main entry point (CLI)
├── internal/
│   ├── sdr/              # SDR interfaces and implementations (mock, Pluto, etc.)
│   ├── dsp/              # windowing, FFT, dBFS, angle math, monopulse logic
│   ├── app/              # orchestration of SDR + DSP
│   └── telemetry/        # logging / optional HTTP+WS visualisation
├── agent.md              # instructions and roadmap for an AI/dev agent
└── README.md             # this file

```
Example of the webinterface:
<img width="1400" height="1272" alt="image" src="https://github.com/user-attachments/assets/e9f8e93d-71cf-4bdc-942d-6c853d70e085" />






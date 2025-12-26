## `README.md`

please dont use this yet.... fixing bugs.....


This started when I saw a script from Jon Kraft about beamforming and direction of arrival tracking using an Pluto SDR. 
I have a pluto+ SDR and I enjoyed this very much. 
However to set this up was a pain and to explain deployment to others was undoable.

So I decided to create a golang version of Jon's idea because it is more easily portable (executables with the IIOD in there!). 
While I was at it, i thought about also adding a configuration page in there and other features, memory optimizations and multi tracking...

I somewhat created a very nice SDR Radio direction finding suite! 
After some feedback I added better explainations and an introduction. 


My implementation of the IIOD has the additional:
- Timeout Configuration (Prevent hangs)
- Trigger Support (Synchronized sampling, but not multidevice sampling)
- Streaming API (async support)


```markdown
# Go Monopulse DOA Tracker (AD9361 / Pluto)

A Go port (with some twists) of a Python script that implements a real-time **monopulse direction-of-arrival (DOA) tracker** using an **AD9361-based SDR** (e.g. ADALM-Pluto / FMCOMMS).

I own a **Pluto+ SDR** myself, so I really enjoyed going through it.

That inspired me to build a **Go version** of Jon’s idea, since Go makes it easy to create portable executables, and it could do with some improvements.  
While I was at it, I also decided to add a **web-based configuration page** and some extra structure/functionalities around the DSP and telemetry.

---

## What this application does

The application:

- Transmits a continuous complex tone at an offset from the LO.
- Receives IQ samples on **two RX channels** (two antennas) at a known spacing.
- Forms **sum** (Σ) and **difference** (Δ) channels.
- Runs a coarse **phase scan** to estimate the initial direction of arrival.
- Uses a **monopulse tracking loop** to follow angle changes over time.
- Logs the **steering angle vs time**, and can be extended to provide real-time visualization.
- Exposes a **web UI** for configuration and monitoring (because tweaking parameters in a browser is just nicer).

---

## Background & Motivation

The original Python script:

- Uses `adi` (Analog Devices IIO bindings), `numpy`, and `pyqtgraph`.
- Configures an AD9361 at:
  - LO: `rx_lo = 2.3e9` Hz  
  - Sample rate: `2e6` samples/s  
  - Tone offset: `fc0 = 200e3` Hz
- Sets antenna spacing to half wavelength (`d = 0.5 * λ`).
- Computes the DOA from the phase difference between the two RX channels:
  - **Coarse scan**: sweep synthetic phase delay from −180° to +180°.  
  - **Tracking**: monopulse error (phase of Σ/Δ correlation) drives a phase-delay update loop.

I loved the concept and wanted:

1. A **single static binary** I can drop onto different machines (Linux, Windows, etc.) without wrestling with Python environments.
2. A codebase where the **DSP, geometry, and hardware access** are clearly separated and easy to play with.
3. A small **web interface** to configure and observe the tracker without touching config files or command-line flags every time.

This Go project recreates the original behaviour with an emphasis on:

- Clear DSP and geometry math.
- Testability (mock SDR input).
- Clean separation between hardware access, algorithms, and the UI.
- Easy portability via Go executables.

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

## IIOD write fallback (SSH sysfs)

- Pluto firmware shipping IIOD protocol v0.25 does **not** support attribute writes. When the IIOD client reports that writes are unsupported (protocol < v0.26), the Pluto backend logs a warning and switches to an SSH-based sysfs writer to mirror the same attributes under `/sys/bus/iio/devices`.
- Attribute paths follow standard IIO naming: device attributes are written directly under the device directory, while channel attributes expand to `in_<channel>_<attr>` (or `out_<channel>_<attr>` for `altvoltage*`).
- Configure SSH/sysfs access with CLI flags or environment variables:
  - `--sdr-ssh-host` / `MONO_SDR_SSH_HOST` (defaults to the host portion of `--sdr-uri`)
  - `--sdr-ssh-user` / `MONO_SDR_SSH_USER` (default `root`)
  - `--sdr-ssh-password` / `MONO_SDR_SSH_PASSWORD`
  - `--sdr-ssh-key` / `MONO_SDR_SSH_KEY` (private key path)
  - `--sdr-ssh-port` / `MONO_SDR_SSH_PORT` (default `22`)
  - `--sdr-sysfs-root` / `MONO_SDR_SYSFS_ROOT` (default `/sys/bus/iio/devices`)
- A clear log entry is emitted the first time the fallback is used, including the SSH target host. Subsequent sysfs writes are logged only on error.

Now with impoved explainations:
<img width="2045" height="1694" alt="image" src="https://github.com/user-attachments/assets/60baacd2-143f-4410-92cc-8084efa64705" />


Example of the webinterface:
<img width="1400" height="1272" alt="image" src="https://github.com/user-attachments/assets/e9f8e93d-71cf-4bdc-942d-6c853d70e085" />






DSP Agent
Source Files
internal/dsp/angle.go
internal/dsp/fft.go
internal/dsp/window.go
internal/dsp/monopulse.go

Mission
Provide all deterministic signal processing required by the tracker.
No IO. No hardware. No state beyond pure functions.

Responsibilities
Angle Conversions
Phase shift → steering angle
Wavelength-aware geometry
Windowing + FFT
Compute FFT with Hann/Hamming
Compute magnitude spectrum in dBFS
Provide zero-centered spectrum view
Monopulse Signal Processing
SUM and DELTA channel formation
Coarse scan DOA detection
Tracking loop computations
IQ Manipulation
Phase rotation
Complex interleave/deinterleave helpers
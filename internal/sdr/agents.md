DR Hardware Agent

Source Files
internal/sdr/pluto.go
internal/sdr/mock.go
internal/sdr/sdr.go

Mission
Provide hardware-level interaction abstracted behind: 
type SDR interface {
    Init(cfg Config) error
    RX() (chan0, chan1 \[]complex64, err error)
    TX(iq0, iq1 \[]complex64) error
    Close() error
}

Responsibilities
Pluto Hardware Control
Uses IIOD Client for configuration and streaming
Auto-detects device names, RX/TX indices, buffer alignment rules
Applies TX gain fallback strategies (hardwaregain vs attenuation)
Applies RX gain fallback strategies (slow\_attack → manual → AGC off)
Streaming Logic
RX buffers mapped to IIOD buffer API
Performs any necessary interleave/deinterleave operations
Supports runtime switching between text and binary transport
Mock SDR
Synthetic tone generator with adjustable phase, amplitude, noise
Deterministic and unit-testable
Essential for validating DSP logic without hardware
Telemetry Hooks
Inform caller about dropped frames, overruns, underflows


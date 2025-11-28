package dsp

import "math"

const speedOfLight = 3e8

// PhaseToTheta converts a phase delay (degrees) to a steering angle (degrees).
// freqHz is the carrier frequency, spacingWavelength is the antenna spacing expressed as a fraction of wavelength.
func PhaseToTheta(phaseDeg float64, freqHz float64, spacingWavelength float64) float64 {
	if freqHz == 0 {
		return 0
	}
	d := spacingWavelength * (speedOfLight / freqHz)
	phaseRad := phaseDeg * math.Pi / 180
	arg := phaseRad * speedOfLight / (2 * math.Pi * freqHz * d)
	if arg > 1 {
		arg = 1
	} else if arg < -1 {
		arg = -1
	}
	return math.Asin(arg) * 180 / math.Pi
}

// ThetaToPhase converts a steering angle (degrees) back to a phase delay (degrees).
func ThetaToPhase(thetaDeg float64, freqHz float64, spacingWavelength float64) float64 {
	if freqHz == 0 {
		return 0
	}
	thetaRad := thetaDeg * math.Pi / 180
	d := spacingWavelength * (speedOfLight / freqHz)
	phaseRad := math.Sin(thetaRad) * 2 * math.Pi * freqHz * d / speedOfLight
	return phaseRad * 180 / math.Pi
}

// SignalBinRange mirrors the Python helper that focused on the fc0 tone.
func SignalBinRange(numSamples int, sampleRate float64, toneOffset float64) (int, int) {
	if numSamples <= 0 || sampleRate == 0 {
		return 0, 0
	}
	start := int(float64(numSamples) * (sampleRate/2 + toneOffset/2) / sampleRate)
	end := int(float64(numSamples) * (sampleRate/2 + toneOffset*2) / sampleRate)
	if start < 0 {
		start = 0
	}
	if end > numSamples {
		end = numSamples
	}
	return start, end
}

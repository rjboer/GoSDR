package iiod

import (
	"encoding/binary"
	"errors"
	"math"
)

// DeinterleaveIQBytes converts a raw interleaved signed 16-bit IQ buffer into
// two float32 slices (I and Q). This matches the AD9361 16-bit LE IQ format.
func DeinterleaveIQBytes(buf []byte) ([]float32, []float32, error) {
	if len(buf)%4 != 0 {
		return nil, nil, errors.New("DeinterleaveIQ: buffer length not multiple of 4")
	}

	sampleCount := len(buf) / 4
	I := make([]float32, sampleCount)
	Q := make([]float32, sampleCount)

	for n := 0; n < sampleCount; n++ {
		iOff := n * 4
		i16 := int16(binary.LittleEndian.Uint16(buf[iOff+0 : iOff+2]))
		q16 := int16(binary.LittleEndian.Uint16(buf[iOff+2 : iOff+4]))

		// Normalize to float32 -1..+1
		I[n] = float32(i16) / float32(math.MaxInt16)
		Q[n] = float32(q16) / float32(math.MaxInt16)
	}

	return I, Q, nil
}

// InterleaveIQFloats converts I/Q float32 sequences into interleaved I16 LE format,
// suitable for TX buffer writes for AD9361 / Pluto.
func InterleaveIQFloats(I []float32, Q []float32) ([]byte, error) {
	if len(I) != len(Q) {
		return nil, errors.New("InterleaveIQ: I/Q length mismatch")
	}

	sampleCount := len(I)
	buf := make([]byte, sampleCount*4)

	for n := 0; n < sampleCount; n++ {
		i := int16(max(min(I[n], 1.0), -1.0) * math.MaxInt16)
		q := int16(max(min(Q[n], 1.0), -1.0) * math.MaxInt16)

		off := n * 4
		binary.LittleEndian.PutUint16(buf[off+0:off+2], uint16(i))
		binary.LittleEndian.PutUint16(buf[off+2:off+4], uint16(q))
	}

	return buf, nil
}

// DeinterleaveIQComplex converts raw bytes (interleaved I/Q) into []complex64.
// sampleBytes indicates the size of one I or Q sample in bytes (e.g. 2 for int16).
// Currently only supports 2-byte (S16) samples.
func DeinterleaveIQComplex(buf []byte, sampleBytes int) ([]complex64, error) {
	if sampleBytes != 2 {
		return nil, errors.New("DeinterleaveIQComplex: only 2-byte samples supported")
	}
	if len(buf)%4 != 0 {
		// 2 bytes I + 2 bytes Q = 4 bytes per complex sample
		return nil, errors.New("DeinterleaveIQComplex: buffer length not multiple of 4")
	}

	sampleCount := len(buf) / 4
	out := make([]complex64, sampleCount)

	for n := 0; n < sampleCount; n++ {
		off := n * 4
		i16 := int16(binary.LittleEndian.Uint16(buf[off : off+2]))
		q16 := int16(binary.LittleEndian.Uint16(buf[off+2 : off+4]))

		// Normalize 12-bit/16-bit signed to -1..1 range
		// Pluto (AD9361) is effectively 12-bit shifted to 16-bit, so full short range.
		out[n] = complex(float32(i16)/32768.0, float32(q16)/32768.0)
	}
	return out, nil
}

// InterleaveIQComplex converts []complex64 to raw bytes (S16 LE interleaved).
func InterleaveIQComplex(samples []complex64, sampleBytes int) ([]byte, error) {
	if sampleBytes != 2 {
		return nil, errors.New("InterleaveIQComplex: only 2-byte samples supported")
	}

	sampleCount := len(samples)
	buf := make([]byte, sampleCount*4)

	for n := 0; n < sampleCount; n++ {
		// Clamp and scale
		v := samples[n]
		i := float32(real(v))
		q := float32(imag(v))

		i16 := int16(max(min(i, 1.0), -1.0) * 32767.0)
		q16 := int16(max(min(q, 1.0), -1.0) * 32767.0)

		off := n * 4
		binary.LittleEndian.PutUint16(buf[off:off+2], uint16(i16))
		binary.LittleEndian.PutUint16(buf[off+2:off+4], uint16(q16))
	}
	return buf, nil
}

func min(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

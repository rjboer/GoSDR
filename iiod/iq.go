package iiod

import (
	"encoding/binary"
	"errors"
	"math"
)

// DeinterleaveIQ converts a raw interleaved signed 16-bit IQ buffer into
// two float32 slices (I and Q). This matches the AD9361 16-bit LE IQ format.
func DeinterleaveIQ(buf []byte) ([]float32, []float32, error) {
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

// InterleaveIQ converts I/Q float32 sequences into interleaved I16 LE format,
// suitable for TX buffer writes for AD9361 / Pluto.
func InterleaveIQ(I []float32, Q []float32) ([]byte, error) {
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

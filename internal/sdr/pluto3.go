package sdr

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/rjboer/GoSDR/iiod"
)

//
// PART 3: BUFFER MANAGEMENT + IQ HELPERS
//

// PlutoBuffer wraps an IIOD buffer handle.
type PlutoBuffer struct {
	dev       string
	channel   string
	length    int
	cyclic    bool
	iiobuffer *iiod.Buffer
}

//
// RX BUFFER CREATION
//

func (p *PlutoSDR) createRXBuffer(ctx context.Context, length int) (*PlutoBuffer, error) {
	if p.rxDev == "" {
		return nil, fmt.Errorf("RX device not assigned")
	}

	rxChs, err := p.findRXChannels(ctx)
	if err != nil {
		return nil, err
	}

	// For monopulse we read from voltage0 only.
	ch := rxChs[0]

	flags := 0 // non-cyclic RX
	buf, err := p.client.OpenBufferWithContext(ctx, p.rxDev, length, flags)
	if err != nil {
		return nil, fmt.Errorf("OpenBuffer RX failed: %w", err)
	}

	return &PlutoBuffer{
		dev:       p.rxDev,
		channel:   ch,
		length:    length,
		cyclic:    false,
		iiobuffer: buf,
	}, nil
}

//
// TX BUFFER CREATION
//

func (p *PlutoSDR) createTXBuffer(ctx context.Context, length int, cyclic bool) (*PlutoBuffer, error) {
	if p.txDev == "" {
		return nil, fmt.Errorf("TX device not assigned")
	}

	txChs, err := p.findTXChannels(ctx)
	if err != nil {
		return nil, err
	}

	// For monopulse we use voltage0 as TX.
	ch := txChs[0]

	flags := 0
	if cyclic {
		flags = iiod.BufferFlagCyclic
	}

	buf, err := p.client.OpenBufferWithContext(ctx, p.txDev, length, flags)
	if err != nil {
		return nil, fmt.Errorf("OpenBuffer TX failed: %w", err)
	}

	return &PlutoBuffer{
		dev:       p.txDev,
		channel:   ch,
		length:    length,
		cyclic:    cyclic,
		iiobuffer: buf,
	}, nil
}

//
// RX READ
//

func (p *PlutoSDR) readRX(ctx context.Context, buf *PlutoBuffer) ([]byte, error) {
	if buf == nil || buf.iiobuffer == nil {
		return nil, fmt.Errorf("nil RX buffer")
	}

	raw, err := p.client.ReadBufferWithContext(ctx, buf.iiobuffer)
	if err != nil {
		return nil, fmt.Errorf("ReadBuffer (RX) failed: %w", err)
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("RX returned empty buffer")
	}
	return raw, nil
}

//
// TX WRITE
//

func (p *PlutoSDR) writeTX(ctx context.Context, buf *PlutoBuffer, data []byte) error {
	if buf == nil || buf.iiobuffer == nil {
		return fmt.Errorf("nil TX buffer")
	}

	if len(data) == 0 {
		return fmt.Errorf("no data for TX buffer")
	}

	if err := p.client.WriteBufferWithContext(ctx, buf.iiobuffer, data); err != nil {
		return fmt.Errorf("WriteBuffer (TX) failed: %w", err)
	}
	return nil
}

//
// BUFFER CLOSE
//

func (p *PlutoSDR) closeBuffer(ctx context.Context, buf *PlutoBuffer) error {
	if buf == nil || buf.iiobuffer == nil {
		return nil
	}
	return p.client.CloseBufferWithContext(ctx, buf.iiobuffer)
}

//
// IQ HELPERS
//

// ReadIQ reads IQ samples and converts them to float32 complex pairs.
func (p *PlutoSDR) ReadIQ(ctx context.Context, buf *PlutoBuffer) ([]complex64, error) {
	raw, err := p.readRX(ctx, buf)
	if err != nil {
		return nil, err
	}

	samples, err := iiod.ParseInt16Samples(raw)
	if err != nil {
		return nil, fmt.Errorf("parse RX samples: %w", err)
	}

	i16, q16, err := iiod.DeinterleaveIQ(samples, 1, 0)
	if err != nil {
		return nil, fmt.Errorf("DeinterleaveIQ failed: %w", err)
	}

	if len(i16) != len(q16) {
		return nil, fmt.Errorf("IQ length mismatch: I=%d Q=%d", len(i16), len(q16))
	}

	out := make([]complex64, len(i16))
	for n := 0; n < len(i16); n++ {
		out[n] = complex(float32(i16[n])/32768.0, float32(q16[n])/32768.0)
	}
	return out, nil
}

// WriteIQ sends complex64 samples to TX.
func (p *PlutoSDR) WriteIQ(ctx context.Context, buf *PlutoBuffer, samples []complex64) error {
	if buf == nil {
		return fmt.Errorf("nil TX buffer")
	}
	if len(samples) == 0 {
		return fmt.Errorf("no IQ samples to send")
	}

	i16 := make([]int16, len(samples))
	q16 := make([]int16, len(samples))

	for n, v := range samples {
		i := int16(real(v) * 32767.0)
		q := int16(imag(v) * 32767.0)
		i16[n] = i
		q16[n] = q
	}

	interleaved, err := iiod.InterleaveIQ([][][]int16{{i16, q16}})
	if err != nil {
		return fmt.Errorf("InterleaveIQ failed: %w", err)
	}

	raw := make([]byte, len(interleaved)*2)
	for idx, v := range interleaved {
		binary.LittleEndian.PutUint16(raw[idx*2:idx*2+2], uint16(v))
	}

	return p.writeTX(ctx, buf, raw)
}

//
// HELPER FOR SHORT-TIMEOUT CONTEXT
//

func (p *PlutoSDR) ctxIO() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 1500*time.Millisecond)
}

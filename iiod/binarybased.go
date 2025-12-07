package iiod

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"strings"
)

// Binary-only VERSION (context info)
func (c *Client) getContextInfoWithContextBinary(ctx context.Context) (ContextInfo, error) {
	cmd := IIODCommand{
		ClientID: 0,
		Opcode:   opcodeVersion,
		Device:   0,
		Code:     0,
	}

	if err := c.sendCommand(ctx, cmd, nil); err != nil {
		return ContextInfo{}, err
	}

	status, err := c.readResponse(ctx)
	if err != nil {
		return ContextInfo{}, err
	}

	buf, err := c.readPayload(status)
	if err != nil {
		return ContextInfo{}, err
	}
	if len(buf) == 0 {
		return ContextInfo{}, fmt.Errorf("binary VERSION returned empty payload (status=%d)", status)
	}

	return parseContextInfo(string(buf))
}

// Binary-only PRINT (XML context)
func (c *Client) getXMLContextWithContextBinary(ctx context.Context) (string, error) {
	// If we already have XML cached, just ensure maps and return
	if c.xmlContext != "" {
		if c.deviceIndexMap == nil || c.attributeCodes == nil {
			if err := c.refreshMetadataMaps(c.xmlContext); err != nil {
				log.Printf("Failed to parse IIOD metadata maps from cached XML: %v", err)
			}
		}
		return c.xmlContext, nil
	}

	cmd := IIODCommand{
		ClientID: 0,
		Opcode:   opcodePrint,
		Device:   0,
		Code:     0,
	}

	log.Printf("[IIOD DEBUG] getXMLContextWithContextBinary: Sending PRINT opcode...")
	if err := c.sendCommand(ctx, cmd, nil); err != nil {
		return "", err
	}

	status, err := c.readResponse(ctx)
	if err != nil {
		return "", err
	}
	log.Printf("[IIOD DEBUG] getXMLContextWithContextBinary: status/length=%d", status)

	buf, err := c.readPayload(status)
	if err != nil {
		return "", err
	}

	resp := string(buf)
	if resp == "" {
		return "", fmt.Errorf("binary PRINT returned empty XML context (status=%d)", status)
	}

	c.cacheXMLMetadata(resp)
	return c.xmlContext, nil
}

// Binary-only LIST_DEVICES
func (c *Client) listDevicesWithContextBinary(ctx context.Context) ([]string, error) {
	cmd := IIODCommand{
		ClientID: 0,
		Opcode:   opcodeListDevices,
		Device:   0,
		Code:     0,
	}

	if err := c.sendCommand(ctx, cmd, nil); err != nil {
		return nil, err
	}

	status, err := c.readResponse(ctx)
	if err != nil {
		return nil, err
	}

	// If status==0, we can still fall back to XML parsing (no text command needed)
	if status == 0 {
		return c.ListDevicesFromXML(ctx)
	}

	buf, err := c.readPayload(status)
	if err != nil {
		return nil, err
	}
	if len(buf) == 0 {
		return nil, nil
	}

	return strings.Fields(string(buf)), nil
}

// Binary-only LIST_CHANNELS
func (c *Client) getChannelsWithContextBinary(ctx context.Context, device string) ([]string, error) {
	if strings.TrimSpace(device) == "" {
		return nil, fmt.Errorf("device name is required")
	}

	payload := []byte(device + "\n")

	cmd := IIODCommand{
		ClientID: 0,
		Opcode:   opcodeListChannels,
		Device:   0,
		Code:     0,
	}

	if err := c.sendCommand(ctx, cmd, payload); err != nil {
		return nil, err
	}

	status, err := c.readResponse(ctx)
	if err != nil {
		return nil, err
	}

	buf, err := c.readPayload(status)
	if err != nil {
		return nil, err
	}
	if len(buf) == 0 {
		return nil, nil
	}

	return strings.Fields(string(buf)), nil
}

// Binary-only OPEN buffer
func (c *Client) openBufferWithContextBinary(ctx context.Context, device string, samples int) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}
	if samples <= 0 {
		return fmt.Errorf("sample count must be positive")
	}

	buf := bytes.NewBufferString(device + "\n")
	if err := binary.Write(buf, binary.BigEndian, uint64(samples)); err != nil {
		return fmt.Errorf("encode sample count: %w", err)
	}

	cmd := IIODCommand{
		ClientID: 0,
		Opcode:   opcodeOpenBuffer,
		Device:   0,
		Code:     0,
	}

	if err := c.sendCommand(ctx, cmd, buf.Bytes()); err != nil {
		return err
	}

	status, err := c.readResponse(ctx)
	if err != nil {
		return err
	}

	// For binary mode, status==0 can be treated as success with no extra payload
	if status == 0 {
		return nil
	}

	_, err = c.readPayload(status)
	return err
}

// Binary-only READBUF
func (c *Client) readBufferWithContextBinary(ctx context.Context, device string, samples int) ([]byte, error) {
	if strings.TrimSpace(device) == "" {
		return nil, fmt.Errorf("device name is required")
	}
	if samples <= 0 {
		return nil, fmt.Errorf("sample count must be positive")
	}

	buf := bytes.NewBufferString(device + "\n")
	if err := binary.Write(buf, binary.BigEndian, uint64(samples)); err != nil {
		return nil, fmt.Errorf("encode sample count: %w", err)
	}

	cmd := IIODCommand{
		ClientID: 0,
		Opcode:   opcodeReadBuffer,
		Device:   0,
		Code:     0,
	}

	if err := c.sendCommand(ctx, cmd, buf.Bytes()); err != nil {
		return nil, err
	}

	status, err := c.readResponse(ctx)
	if err != nil {
		return nil, err
	}

	// Pure binary: no text fallback here
	return c.readPayload(status)
}

// Binary-only WRITEBUF
func (c *Client) writeBufferWithContextBinary(ctx context.Context, device string, data []byte) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}
	if len(data) == 0 {
		return fmt.Errorf("no data provided for buffer write")
	}

	buf := bytes.NewBufferString(device + "\n")
	if err := binary.Write(buf, binary.BigEndian, uint64(len(data))); err != nil {
		return fmt.Errorf("encode data length: %w", err)
	}
	buf.Write(data)

	cmd := IIODCommand{
		ClientID: 0,
		Opcode:   opcodeWriteBuffer,
		Device:   0,
		Code:     0,
	}

	if err := c.sendCommand(ctx, cmd, buf.Bytes()); err != nil {
		return err
	}

	status, err := c.readResponse(ctx)
	if err != nil {
		return err
	}

	// status==0 => success, no extra payload
	if status == 0 {
		return nil
	}

	_, err = c.readPayload(status)
	return err
}

// Binary-only CLOSE buffer
func (c *Client) closeBufferWithContextBinary(ctx context.Context, device string) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}

	payload := []byte(device + "\n")

	cmd := IIODCommand{
		ClientID: 0,
		Opcode:   opcodeCloseBuffer,
		Device:   0,
		Code:     0,
	}

	if err := c.sendCommand(ctx, cmd, payload); err != nil {
		return err
	}

	status, err := c.readResponse(ctx)
	if err != nil {
		return err
	}

	if status == 0 {
		return nil
	}

	_, err = c.readPayload(status)
	return err
}

// -----------------------------------------------------------------------------
//  IQ HELPER FUNCTIONS (binary mode compatible)
// -----------------------------------------------------------------------------
//  These helpers allow pluto.go to compile and perform IQ channel unpacking
//  even though Pluto does not use binary buffer mode. They operate directly
//  on []int16 samples returned by text-mode buffer reads.
//
//  DeinterleaveIQ splits interleaved [I,Q, I,Q, ...] sequences into
//  dedicated I[] and Q[] slices for the selected channel.
//
//  InterleaveIQ performs the opposite: it converts multiple channels of
//  {I[],Q[]} pairs into a single interleaved sample slice.
// -----------------------------------------------------------------------------

// DeinterleaveIQ extracts I/Q samples for one channel from a multi-channel stream.
//
// samples:  []int16   (full interleaved raw sample array)
// nchannels: int      (total channels, usually 2 for Pluto)
// chIndex:   int      (channel index to extract: 0 or 1)
//
// Returns:
//
//	I[]int16, Q[]int16, error
func DeinterleaveIQ(samples []int16, nchannels int, chIndex int) ([]int16, []int16, error) {
	if nchannels <= 0 {
		return nil, nil, fmt.Errorf("invalid channel count: %d", nchannels)
	}
	if chIndex < 0 || chIndex >= nchannels {
		return nil, nil, fmt.Errorf("channel %d out of range", chIndex)
	}
	if len(samples)%2 != 0 {
		return nil, nil, fmt.Errorf("sample count %d is not even (I/Q pairs required)", len(samples))
	}

	// Per-channel stride:
	// For 2 channels: I0,Q0, I1,Q1, I0,Q0, I1,Q1, ...
	stride := 2 * nchannels

	// IQ offset for this channel
	base := chIndex * 2

	// Number of complex samples in total
	totalComplex := len(samples) / 2

	// Samples per channel
	chSamples := totalComplex / nchannels

	I := make([]int16, chSamples)
	Q := make([]int16, chSamples)

	idx := 0
	for n := 0; n < chSamples; n++ {
		pos := n*stride + base
		if pos+1 >= len(samples) {
			return nil, nil, fmt.Errorf("out-of-range access during IQ split")
		}
		I[idx] = samples[pos]
		Q[idx] = samples[pos+1]
		idx++
	}

	return I, Q, nil
}

// InterleaveIQ combines multi-channel IQ slices into a single []int16
// Interleaved as: I0,Q0, I1,Q1, I0,Q0, I1,Q1, ...
//
// channels: [channel][pair][sampleIndex]
//
//	channels[c][0] = I samples for channel c
//	channels[c][1] = Q samples for channel c
//
// Example for two channels:
//
//	channels = {
//	   { I0[], Q0[] },
//	   { I1[], Q1[] },
//	}
//
// Output []int16 layout:
//
//	I0,Q0, I1,Q1, I0,Q0, I1,Q1, ...
func InterleaveIQ(channels [][][]int16) ([]int16, error) {
	if len(channels) == 0 {
		return nil, fmt.Errorf("no channels provided")
	}

	nchannels := len(channels)
	nsamples := len(channels[0][0])

	// Validate sizes
	for c := range channels {
		if len(channels[c]) != 2 {
			return nil, fmt.Errorf("channel %d must have {I[], Q[]}", c)
		}
		if len(channels[c][0]) != nsamples || len(channels[c][1]) != nsamples {
			return nil, fmt.Errorf("channel %d length mismatch", c)
		}
	}

	// Produce output buffer
	out := make([]int16, nsamples*nchannels*2) // I/Q pairs per channel

	idx := 0
	for n := 0; n < nsamples; n++ {
		for c := 0; c < nchannels; c++ {
			out[idx] = channels[c][0][n] // I
			idx++
			out[idx] = channels[c][1][n] // Q
			idx++
		}
	}

	return out, nil
}

package iiod

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"strings"
)

type Buffer struct {
	client       *Client
	device       string
	size         int
	channelMask  uint64
	isOpen       bool
	enabledChans []string
}

func (c *Client) CreateStreamBuffer(device string, samples int, channelMask uint64) (*Buffer, error) {
	if strings.TrimSpace(device) == "" {
		return nil, fmt.Errorf("device name is required")
	}
	if samples <= 0 {
		return nil, fmt.Errorf("sample count must be positive")
	}

	buf := &Buffer{
		client:       c,
		device:       device,
		size:         samples,
		channelMask:  channelMask,
		enabledChans: make([]string, 0),
	}

	channels, err := c.GetChannels(device)
	if err != nil {
		return nil, fmt.Errorf("failed to get channels: %w", err)
	}

	for i, ch := range channels {
		if i >= 64 {
			break
		}
		if (channelMask & (1 << uint(i))) != 0 {
			if err := buf.enableChannelInternal(ch, true); err != nil {
				return nil, fmt.Errorf("failed to enable channel %s: %w", ch, err)
			}
		}
	}

	if err := buf.open(); err != nil {
		return nil, fmt.Errorf("failed to open buffer: %w", err)
	}

	return buf, nil
}

func (b *Buffer) EnableChannel(channelID string, enable bool) error {
	if b.isOpen {
		return fmt.Errorf("cannot change channel configuration after buffer is opened")
	}
	return b.enableChannelInternal(channelID, enable)
}

func (b *Buffer) enableChannelInternal(channelID string, enable bool) error {
	if strings.TrimSpace(channelID) == "" {
		return fmt.Errorf("channel ID is required")
	}

	val := "0"
	if enable {
		val = "1"
	}

	// ==== NEW: correct WriteAttr call with context + fallback ====
	ctx := context.Background()

	if b.client.mode == ProtocolBinary {
		err := b.client.WriteAttrWithContext(ctx, b.device, channelID, "en", val)
		if err == nil {
			b.updateEnabledList(channelID, enable)
			return nil
		}

		log.Printf("[IIOD DEBUG] binary WriteAttr failed for %s/%s/en (%v), falling back to text mode",
			b.device, channelID, err)
	}

	// === TEXT WRITE fallback ===
	cmd := fmt.Sprintf("WRITE %s %s en %s", b.device, channelID, val)
	if _, err := b.client.sendCommandString(ctx, cmd); err != nil {
		return fmt.Errorf("failed to write channel enable via text WRITE: %w", err)
	}

	b.updateEnabledList(channelID, enable)
	return nil
}

func (b *Buffer) updateEnabledList(ch string, enable bool) {
	if enable {
		for _, c := range b.enabledChans {
			if c == ch {
				return
			}
		}
		b.enabledChans = append(b.enabledChans, ch)
	} else {
		for i, c := range b.enabledChans {
			if c == ch {
				b.enabledChans = append(b.enabledChans[:i], b.enabledChans[i+1:]...)
				return
			}
		}
	}
}

func (b *Buffer) open() error {
	if b.isOpen {
		return fmt.Errorf("buffer already open")
	}
	if err := b.client.OpenBuffer(b.device, b.size); err != nil {
		return err
	}
	b.isOpen = true
	return nil
}

func (b *Buffer) ReadSamples() ([]byte, error) {
	if !b.isOpen {
		return nil, fmt.Errorf("buffer not open")
	}
	data, err := b.client.ReadBuffer(b.device, b.size)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("no data returned")
	}
	return data, nil
}

func (b *Buffer) WriteSamples(data []byte) error {
	if !b.isOpen {
		return fmt.Errorf("buffer not open")
	}
	if len(data) == 0 {
		return fmt.Errorf("no data to write")
	}
	return b.client.WriteBuffer(b.device, data)
}

func (b *Buffer) Close() error {
	if !b.isOpen {
		return nil
	}
	err := b.client.CloseBuffer(b.device)
	b.isOpen = false
	return err
}

func ParseInt16Samples(data []byte) ([]int16, error) {
	if len(data)%2 != 0 {
		return nil, fmt.Errorf("invalid sample length")
	}
	samples := make([]int16, len(data)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return samples, nil
}

func FormatInt16Samples(s []int16) []byte {
	b := make([]byte, len(s)*2)
	for i, v := range s {
		binary.LittleEndian.PutUint16(b[i*2:], uint16(v))
	}
	return b
}

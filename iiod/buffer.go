package iiod

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// Buffer represents an IIO buffer for streaming data from a device.
// It manages channel configuration and provides methods for reading/writing
// sample data in the binary format used by the IIO protocol.
type Buffer struct {
	client       *Client
	device       string
	size         int
	channelMask  uint64
	isOpen       bool
	enabledChans []string
}

// CreateStreamBuffer allocates a buffer for streaming data from the specified device.
// The buffer will be configured to hold 'samples' number of samples.
// The channelMask parameter is a bitmask indicating which channels should be enabled.
//
// Example:
//
//	buf, err := client.CreateStreamBuffer("cf-ad9361-lpc", 1024, 0x0F) // Enable first 4 channels
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

	// Get list of channels for this device
	channels, err := c.GetChannels(device)
	if err != nil {
		return nil, fmt.Errorf("failed to get channels: %w", err)
	}

	// Enable channels based on mask
	for i, ch := range channels {
		if i >= 64 {
			break // channelMask is uint64, max 64 channels
		}
		if (channelMask & (1 << uint(i))) != 0 {
			if err := buf.enableChannelInternal(ch, true); err != nil {
				return nil, fmt.Errorf("failed to enable channel %s: %w", ch, err)
			}
		}
	}

	// Open the buffer on the remote device
	if err := buf.open(); err != nil {
		return nil, fmt.Errorf("failed to open buffer: %w", err)
	}

	return buf, nil
}

// EnableChannel enables or disables a specific channel in the buffer.
// This must be called before opening the buffer.
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

	// Use WRITE_ATTR to set the channel's 'en' attribute
	// Format: WRITE_ATTR device channel en 1/0
	value := "0"
	if enable {
		value = "1"
	}

	err := b.client.WriteAttr(b.device, channelID, "en", value)
	if err != nil {
		return fmt.Errorf("failed to set channel enable state: %w", err)
	}

	// Track enabled channels
	if enable {
		// Add to enabled list if not already present
		found := false
		for _, ch := range b.enabledChans {
			if ch == channelID {
				found = true
				break
			}
		}
		if !found {
			b.enabledChans = append(b.enabledChans, channelID)
		}
	} else {
		// Remove from enabled list
		for i, ch := range b.enabledChans {
			if ch == channelID {
				b.enabledChans = append(b.enabledChans[:i], b.enabledChans[i+1:]...)
				break
			}
		}
	}

	return nil
}

// open sends the OPEN command to create the buffer on the remote device.
func (b *Buffer) open() error {
	if b.isOpen {
		return fmt.Errorf("buffer is already open")
	}

	if err := b.client.OpenBuffer(b.device, b.size); err != nil {
		return err
	}

	b.isOpen = true
	return nil
}

// ReadSamples reads sample data from the buffer.
// Returns raw binary data in the format provided by the device.
// The data format is device-specific (e.g., AD9361 uses interleaved int16 I/Q samples).
//
// For AD9361 devices, the format is typically:
//
//	[I0_ch0, Q0_ch0, I0_ch1, Q0_ch1, I1_ch0, Q1_ch0, I1_ch1, Q1_ch1, ...]
//
// where each sample is a little-endian int16.
func (b *Buffer) ReadSamples() ([]byte, error) {
	if !b.isOpen {
		return nil, fmt.Errorf("buffer is not open")
	}

	data, err := b.client.ReadBuffer(b.device, b.size)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("no data returned from buffer read")
	}

	return data, nil
}

// WriteSamples writes sample data to the buffer for transmission.
// The data should be in the device-specific binary format.
//
// For AD9361 devices, the format is typically:
//
//	[I0_ch0, Q0_ch0, I0_ch1, Q0_ch1, I1_ch0, Q1_ch0, I1_ch1, Q1_ch1, ...]
//
// where each sample is a little-endian int16.
func (b *Buffer) WriteSamples(data []byte) error {
	if !b.isOpen {
		return fmt.Errorf("buffer is not open")
	}

	if len(data) == 0 {
		return fmt.Errorf("no data to write")
	}

	return b.client.WriteBuffer(b.device, data)
}

// Close destroys the buffer and releases resources on the remote device.
func (b *Buffer) Close() error {
	if !b.isOpen {
		return nil // Already closed
	}

	err := b.client.CloseBuffer(b.device)
	b.isOpen = false
	return err
}

// ParseInt16Samples parses raw binary data as little-endian int16 samples.
// This is a helper function for devices like AD9361 that use 16-bit samples.
//
// Returns a slice of int16 values in the order they appear in the data.
func ParseInt16Samples(data []byte) ([]int16, error) {
	if len(data)%2 != 0 {
		return nil, fmt.Errorf("data length must be even for int16 samples")
	}

	samples := make([]int16, len(data)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2 : i*2+2]))
	}

	return samples, nil
}

// FormatInt16Samples formats int16 samples as little-endian binary data.
// This is a helper function for devices like AD9361 that use 16-bit samples.
func FormatInt16Samples(samples []int16) []byte {
	data := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(data[i*2:i*2+2], uint16(sample))
	}
	return data
}

// DeinterleaveIQ deinterleaves I/Q samples for a specific channel from interleaved data.
// Assumes data format: [I0_ch0, Q0_ch0, I0_ch1, Q0_ch1, ...]
//
// Parameters:
//   - samples: Interleaved I/Q samples as int16
//   - numChannels: Total number of channels in the interleaved data
//   - channelIndex: Zero-based index of the channel to extract
//
// Returns separate I and Q slices for the specified channel.
func DeinterleaveIQ(samples []int16, numChannels, channelIndex int) ([]int16, []int16, error) {
	if numChannels <= 0 {
		return nil, nil, fmt.Errorf("numChannels must be positive")
	}
	if channelIndex < 0 || channelIndex >= numChannels {
		return nil, nil, fmt.Errorf("channelIndex out of range")
	}

	samplesPerChannel := len(samples) / (numChannels * 2) // 2 for I and Q
	if len(samples)%(numChannels*2) != 0 {
		return nil, nil, fmt.Errorf("sample count not divisible by number of channels")
	}

	iSamples := make([]int16, samplesPerChannel)
	qSamples := make([]int16, samplesPerChannel)

	for i := 0; i < samplesPerChannel; i++ {
		baseIdx := i * numChannels * 2
		chOffset := channelIndex * 2
		iSamples[i] = samples[baseIdx+chOffset]
		qSamples[i] = samples[baseIdx+chOffset+1]
	}

	return iSamples, qSamples, nil
}

// InterleaveIQ interleaves I/Q samples for multiple channels.
// Produces format: [I0_ch0, Q0_ch0, I0_ch1, Q0_ch1, ...]
//
// Parameters:
//   - channels: Slice of channel data, where each element is a pair of [I, Q] slices
//
// Returns interleaved samples ready for transmission.
func InterleaveIQ(channels [][][]int16) ([]int16, error) {
	if len(channels) == 0 {
		return nil, fmt.Errorf("no channels provided")
	}

	// Verify all channels have same length
	samplesPerChannel := len(channels[0][0]) // I samples of first channel
	for i, ch := range channels {
		if len(ch) != 2 {
			return nil, fmt.Errorf("channel %d must have exactly 2 slices (I and Q)", i)
		}
		if len(ch[0]) != samplesPerChannel || len(ch[1]) != samplesPerChannel {
			return nil, fmt.Errorf("channel %d has mismatched I/Q lengths", i)
		}
	}

	numChannels := len(channels)
	result := make([]int16, samplesPerChannel*numChannels*2)

	for sampleIdx := 0; sampleIdx < samplesPerChannel; sampleIdx++ {
		for chIdx := 0; chIdx < numChannels; chIdx++ {
			baseIdx := sampleIdx*numChannels*2 + chIdx*2
			result[baseIdx] = channels[chIdx][0][sampleIdx]   // I
			result[baseIdx+1] = channels[chIdx][1][sampleIdx] // Q
		}
	}

	return result, nil
}

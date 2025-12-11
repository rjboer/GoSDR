package iiod

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
)

// Buffer represents an open stream buffer on the device.
type Buffer struct {
	client *Client
	device string
	size   int
	isOpen bool

	enabledChannels int
	bytesPerSample  int
}

type BufferHandle struct {
	device string
	id     int
}

// // CreateStreamBuffer opens a streaming buffer after enabling selected channels.
// func (c *Client) CreateStreamBuffer(device string, size int, channelMask uint8) (*Buffer, error) {
// 	if device == "" {
// 		return nil, fmt.Errorf("device is required")
// 	}
// 	if size <= 0 {
// 		return nil, fmt.Errorf("size must be positive")
// 	}

// 	ctx := context.Background()

// 	channels, err := c.GetChannelsWithContext(ctx, device)
// 	if err != nil {
// 		return nil, err
// 	}

// 	log.Printf("[IIOD DEBUG] CreateStreamBuffer: device=%s samples=%d channelMask=0x%x availableChannels=%v", device, size, channelMask, channels)

// 	enabled := bits.OnesCount8(channelMask & ((1 << uint(len(channels))) - 1))
// 	if enabled == 0 {
// 		return nil, fmt.Errorf("no channels enabled (mask=0x%x)", channelMask)
// 	}
// 	bytesPerSample := enabled * 2 // int16 samples

// 	if c.IsLegacy() {
// 		log.Printf("[IIOD DEBUG] CreateStreamBuffer: legacy server detected; skipping channel enable writes")
// 	} else {
// 		for i, ch := range channels {
// 			if channelMask&(1<<uint(i)) == 0 {
// 				continue
// 			}
// 			log.Printf("[IIOD DEBUG] CreateStreamBuffer: enabling channel %s (index=%d)", ch, i)
// 			if err := c.WriteAttrWithContext(ctx, device, ch, "en", "1"); err != nil {
// 				log.Printf("[IIOD DEBUG] CreateStreamBuffer: failed to enable %s/%s: %v", device, ch, err)
// 				return nil, err
// 			}
// 		}
// 	}

// 	log.Printf("[IIOD DEBUG] CreateStreamBuffer: issuing BUFFER_OPEN for %s with %d samples (mode=%v)", device, size, c.mode)
// 	if err := c.OpenBufferWithContext(ctx, device, size); err != nil {
// 		log.Printf("[IIOD DEBUG] CreateStreamBuffer: BUFFER_OPEN failed for %s: %v", device, err)
// 		return nil, err
// 	}

// 	log.Printf("[IIOD DEBUG] CreateStreamBuffer: buffer opened for %s (size=%d)", device, size)

// 	return &Buffer{client: c, device: device, size: size, isOpen: true, enabledChannels: enabled, bytesPerSample: bytesPerSample}, nil
// }

// CreateStreamBuffer opens a streaming buffer after enabling selected channels.
// This version includes correct device-index handling for PlutoSDR (IIOD v0.25).
// func (c *Client) CreateStreamBuffer(device string, size int, channelMask uint8) (*Buffer, error) {
// 	if device == "" {
// 		return nil, fmt.Errorf("device is required")
// 	}
// 	if size <= 0 {
// 		return nil, fmt.Errorf("size must be positive")
// 	}

// 	ctx := context.Background()

// 	// List channels (Pluto returns: voltage0 voltage1)
// 	channels, err := c.GetChannelsWithContext(ctx, device)
// 	if err != nil {
// 		return nil, err
// 	}

// 	log.Printf("[IIOD DEBUG] CreateStreamBuffer: device=%s samples=%d channelMask=0x%x availableChannels=%v",
// 		device, size, channelMask, channels)

// 	enabled := bits.OnesCount8(channelMask & ((1 << uint(len(channels))) - 1))
// 	if enabled == 0 {
// 		return nil, fmt.Errorf("no channels enabled: mask=0x%x", channelMask)
// 	}
// 	bytesPerSample := enabled * 2 // int16 per I/Q element

// 	//
// 	// IMPORTANT PART: Pluto (legacy IIOD) does NOT accept channel enabling via 'WRITE <dev> <ch> en 1'
// 	// and its IIOD server only accepts BUFFER_OPEN using the DEVICE INDEX, not the DEVICE NAME.
// 	//
// 	if c.IsLegacy() {
// 		log.Printf("[IIOD DEBUG] Legacy IIOD detected (PlutoSDR). Skipping channel enable writes.")

// 		// Resolve device index for BUFFER_OPEN (Pluto only supports numeric index)
// 		idx, ok := c.deviceIndexMap[device]
// 		if !ok {
// 			// Fallback: search by name in device info
// 			for name, v := range c.deviceIndexMap {
// 				if name == device {
// 					idx = v
// 					ok = true
// 					break
// 				}
// 			}
// 		}
// 		if !ok {
// 			return nil, fmt.Errorf("cannot locate device index for %s", device)
// 		}

// 		log.Printf("[IIOD DEBUG] BUFFER_OPEN using device index %d (name=%s)", idx, device)

// 		if err := c.OpenBufferWithContext(ctx, fmt.Sprintf("%d", idx), size); err != nil {
// 			log.Printf("[IIOD DEBUG] CreateStreamBuffer: BUFFER_OPEN failed idx=%d: %v", idx, err)
// 			return nil, err
// 		}

// 		log.Printf("[IIOD DEBUG] Legacy buffer opened successfully: index=%d size=%d", idx, size)

// 		return &Buffer{
// 			client:          c,
// 			device:          fmt.Sprintf("%d", idx), // IMPORTANT: use index for future READ/WRITE
// 			size:            size,
// 			isOpen:          true,
// 			enabledChannels: enabled,
// 			bytesPerSample:  bytesPerSample,
// 		}, nil
// 	}

// 	// ---------- Modern IIOD path (not used for Pluto v0.25) ----------
// 	for i, ch := range channels {
// 		if channelMask&(1<<uint(i)) == 0 {
// 			continue
// 		}
// 		log.Printf("[IIOD DEBUG] enabling channel %s", ch)
// 		if err := c.WriteAttrWithContext(ctx, device, ch, "en", "1"); err != nil {
// 			log.Printf("[IIOD DEBUG] failed enabling %s/%s: %v", device, ch, err)
// 			return nil, err
// 		}
// 	}

// 	if err := c.OpenBufferWithContext(ctx, device, size); err != nil {
// 		log.Printf("[IIOD DEBUG] BUFFER_OPEN failed for %s: %v", device, err)
// 		return nil, err
// 	}

// 	return &Buffer{client: c, device: device, size: size, isOpen: true, enabledChannels: enabled, bytesPerSample: bytesPerSample}, nil
// }

// CreateStreamBuffer creates a streaming buffer on the given device.
// enabledMask is a bitmask selecting which channels to enable.
func (c *Client) CreateStreamBuffer(ctx context.Context, device string, size int, enabledMask uint8) (*Buffer, error) {
	if size <= 0 {
		return nil, fmt.Errorf("buffer size must be > 0")
	}
	deviceIDMap := make(map[string]string)
	deviceIDMap["iio:device1"] = "1"
	deviceIDMap["iio:device2"] = "2"
	deviceIDMap["iio:device3"] = "3"
	deviceIDMap["iio:device4"] = "4"
	deviceIDMap["iio:device5"] = "5"
	deviceIDMap["iio:device6"] = "6"
	deviceIDMap["iio:device7"] = "7"
	deviceIDMap["iio:device8"] = "8"
	deviceIDMap["iio:device9"] = "9"
	deviceIDMap["iio:device10"] = "10"

	// ---------- Legacy IIOD path (not used for Pluto v0.25) ----------
	origDevice := device

	// Pluto legacy IIOD: device must be numeric index, not name
	if c.IsLegacy() {
		if idx, ok := c.deviceIndexMap[device]; ok {
			log.Printf("[IIOD DEBUG] Legacy IIOD: mapping device %q -> index %d", device, idx)
			device = fmt.Sprintf("%d", idx)
		} else {
			// fallback: reverse map using deviceIDMap if present
			for name, id := range deviceIDMap {
				if id == device {
					if idx2, ok2 := c.deviceIndexMap[name]; ok2 {
						log.Printf("[IIOD DEBUG] Legacy IIOD: mapping ID %q to index %d (name=%s)", device, idx2, name)
						device = fmt.Sprintf("%d", idx2)
						break
					}
				}
			}
		}
	}

	log.Printf("[IIOD DEBUG] CreateStreamBuffer: requesting channel list for device=%s (orig=%s)", device, origDevice)

	channels, err := c.GetChannelsWithContext(ctx, device)
	if err != nil {
		log.Printf("[IIOD DEBUG] CreateStreamBuffer: GetChannels failed for device=%s (orig=%s): %v",
			device, origDevice, err)
		return nil, err
	}

	log.Printf("[IIOD DEBUG] CreateStreamBuffer: channels=%v", channels)

	enabled := make(map[string]bool)
	enabledCount := 0

	for i, ch := range channels {
		if enabledMask&(1<<uint(i)) != 0 {
			enabled[ch] = true
			enabledCount++
			log.Printf("[IIOD DEBUG] CreateStreamBuffer: enabling channel %s", ch)
		}
	}

	if enabledCount == 0 {
		return nil, fmt.Errorf("no enabled RX channels (mask=0x%x)", enabledMask)
	}

	// Each channel = complex16 = 4 bytes per sample
	bytesPerSample := enabledCount * 4

	log.Printf("[IIOD DEBUG] CreateStreamBuffer: calling BUFFER_OPEN on device=%s size=%d enabledChannels=%d",
		device, size, enabledCount)

	if err := c.OpenBufferWithContext(ctx, device, size); err != nil {
		log.Printf("[IIOD DEBUG] CreateStreamBuffer: BUFFER_OPEN failed: %v", err)
		return nil, err
	}

	log.Printf("[IIOD DEBUG] CreateStreamBuffer: buffer opened successfully on device=%s", device)

	return &Buffer{
		client:          c,
		device:          device,
		size:            size,
		isOpen:          true,
		enabledChannels: enabledCount,
		bytesPerSample:  bytesPerSample,
	}, nil
}

// Close closes the buffer on the device.
func (b *Buffer) Close() error {
	if b == nil || !b.isOpen {
		return nil
	}

	if err := b.client.CloseBufferWithContext(context.Background(), b.device); err != nil {
		return err
	}
	b.isOpen = false
	return nil
}

// ReadSamples reads raw bytes from the buffer.
func (b *Buffer) ReadSamples() ([]byte, error) {
	if b == nil || !b.isOpen {
		return nil, fmt.Errorf("buffer not open")
	}

	nBytes := b.size
	if b.bytesPerSample > 0 {
		nBytes = b.size * b.bytesPerSample
	}

	return b.client.ReadBufferWithContext(context.Background(), b.device, nBytes)
}

// WriteSamples writes raw bytes to the buffer.
func (b *Buffer) WriteSamples(data []byte) error {
	if b == nil || !b.isOpen {
		return fmt.Errorf("buffer not open")
	}
	return b.client.WriteBufferWithContext(context.Background(), b.device, data)
}

// Helper payload encoders
func encodeDeviceCountPayload(device string, count uint64) []byte {
	buf := make([]byte, len(device)+1+8)
	copy(buf, []byte(device))
	buf[len(device)] = '\n'
	binary.BigEndian.PutUint64(buf[len(device)+1:], count)
	return buf
}

func encodeWriteBufferPayload(device string, data []byte) []byte {
	buf := make([]byte, len(device)+1+8+len(data))
	copy(buf, []byte(device))
	buf[len(device)] = '\n'
	binary.BigEndian.PutUint64(buf[len(device)+1:], uint64(len(data)))
	copy(buf[len(device)+1+8:], data)
	return buf
}

func encodeWritePayload(target string, value []byte) []byte {
	buf := make([]byte, len(target)+1+8+len(value))
	copy(buf, target)
	buf[len(target)] = '\n'
	binary.BigEndian.PutUint64(buf[len(target)+1:], uint64(len(value)))
	copy(buf[len(target)+1+8:], value)
	return buf
}

// Sample parsing helpers used in tests
func ParseInt16Samples(data []byte) ([]int16, error) {
	if len(data)%2 != 0 {
		return nil, errors.New("data length must be even")
	}
	samples := make([]int16, len(data)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2 : i*2+2]))
	}
	return samples, nil
}

func DeinterleaveIQ(samples []int16, channels int, channelIndex int) ([]int16, []int16, error) {
	if channels <= 0 {
		return nil, nil, errors.New("channels must be positive")
	}
	if channelIndex >= channels {
		return nil, nil, errors.New("channel index out of range")
	}
	stride := channels * 2
	if len(samples)%stride != 0 {
		return nil, nil, errors.New("samples not divisible by channel layout")
	}
	frames := len(samples) / stride
	I := make([]int16, frames)
	Q := make([]int16, frames)
	for i := 0; i < frames; i++ {
		base := i*stride + channelIndex*2
		I[i] = samples[base]
		Q[i] = samples[base+1]
	}
	return I, Q, nil
}

// InterleaveIQ arranges per-channel I/Q samples into interleaved layout.
// channels is indexed as [channel][I/Q][samples].
func InterleaveIQ(channels [][][]int16) ([]int16, error) {
	if len(channels) == 0 {
		return nil, errors.New("no channels provided")
	}
	sampleCount := len(channels[0][0])
	for idx, ch := range channels {
		if len(ch) != 2 {
			return nil, fmt.Errorf("channel %d missing I/Q", idx)
		}
		if len(ch[0]) != len(ch[1]) {
			return nil, fmt.Errorf("channel %d I/Q length mismatch", idx)
		}
		if len(ch[0]) != sampleCount {
			return nil, fmt.Errorf("channel %d sample count mismatch", idx)
		}
	}

	out := make([]int16, sampleCount*len(channels)*2)
	for s := 0; s < sampleCount; s++ {
		for chIdx, ch := range channels {
			base := (s*len(channels) + chIdx) * 2
			out[base] = ch[0][s]
			out[base+1] = ch[1][s]
		}
	}
	return out, nil
}

// FormatInt16Samples converts int16 samples to raw bytes (Little Endian).
func FormatInt16Samples(samples []int16) []byte {
	buf := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(s))
	}
	return buf
}

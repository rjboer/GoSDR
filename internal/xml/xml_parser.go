package sdrxml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
)

// -----------------------------------------------------------------------------
// PUBLIC: ParseIIODXML
// The most important thing of a parser is that it can parse the XML file.
// -----------------------------------------------------------------------------
var scanFmtRe = regexp.MustCompile(`^(le|be):([sSuU])(\d+)/(\d+)(?:X(\d+))?>>(\d+)$`)

// Parse decodes the raw IIOD XML stream into the SDRContext receiver and builds
// a lookup index for fast access.
func (ctx *SDRContext) Parse(raw []byte) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return errors.New("empty XML data")
	}

	if err := xml.Unmarshal(raw, ctx); err != nil {
		return fmt.Errorf("IIOD XML parse error: %w", err)
	}

	// Build fast lookup index
	ctx.BuildIndex()

	return nil
}

// -----------------------------------------------------------------------------
// BuildIndex - construct lookup tables from IIODcontext
// -----------------------------------------------------------------------------

func (ctx *SDRContext) BuildIndex() {
	idx := &IIODIndex{
		DevicesByID:   make(map[string]*DeviceEntry),
		DevicesByName: make(map[string]*DeviceEntry),
		Channels:      make(map[string]map[string]*ChannelEntry),
		AttrFiles:     make(map[string]map[string]map[string]string),
	}

	for i := range ctx.Device {
		dev := &ctx.Device[i]

		// Record by ID and by name
		if dev.ID != "" {
			idx.DevicesByID[dev.ID] = dev
		}
		if dev.Name != "" {
			idx.DevicesByName[dev.Name] = dev
		}

		// Build channel lookup
		if _, ok := idx.Channels[dev.Name]; !ok {
			idx.Channels[dev.Name] = make(map[string]*ChannelEntry)
		}

		// Build attribute filename lookup
		if _, ok := idx.AttrFiles[dev.Name]; !ok {
			idx.AttrFiles[dev.Name] = make(map[string]map[string]string)
		}

		for ci := range dev.Channel {
			ch := &dev.Channel[ci]

			if ch.ScanElementRaw != nil {
				if err := ch.ParseScanFormat(); err != nil {
					log.Printf("ParseScanFormat failed for device %q channel %q: %v", dev.Name, ch.ID, err)
				}
			}
			if ch.ParsedFormat != nil {
				bits := ch.ParsedFormat.Length * ch.ParsedFormat.Repeat
				ch.SampleSize = uint32((bits + 7) / 8)
			}
			// Register channel by ID or name (ID always exists)
			chName := ch.ID
			if ch.Name != "" {
				chName = ch.Name
			}
			idx.Channels[dev.Name][chName] = ch

			// Attribute filename lookup table for this channel
			if _, ok := idx.AttrFiles[dev.Name][chName]; !ok {
				idx.AttrFiles[dev.Name][chName] = make(map[string]string)
			}

			for _, attr := range ch.Attribute {
				if attr.Name != "" && attr.Filename != "" {
					idx.AttrFiles[dev.Name][chName][attr.Name] = attr.Filename
				}
			}
		}
	}

	idx.NoDevices = len(idx.DevicesByID)
	//count the channels

	count := 0
	for _, devMap := range idx.Channels {
		count += len(devMap)
	}
	idx.NoChannels = count
	ctx.Index = idx
}

// -----------------------------------------------------------------------------
// Lookup Helpers
// -----------------------------------------------------------------------------

// LookupDevice returns a device by name or ID.
func (index *IIODIndex) LookupDevice(identifier string) (*DeviceEntry, error) {
	if d, ok := index.DevicesByName[identifier]; ok {
		return d, nil
	}
	if d, ok := index.DevicesByID[identifier]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("device not found in XML: %q", identifier)
}

// LookupChannel returns a channel by channel name or ID.
func (index *IIODIndex) LookupChannel(devName, chName string) (*ChannelEntry, error) {
	devMap, ok := index.Channels[devName]
	if !ok {
		return nil, fmt.Errorf("device not found: %q", devName)
	}

	if ch, ok := devMap[chName]; ok {
		return ch, nil
	}

	// Try resolving via ID for altvoltage0 etc.
	for _, ch := range devMap {
		if ch.ID == chName {
			return ch, nil
		}
	}

	return nil, fmt.Errorf("channel %q not found in device %q", chName, devName)
}

// LookupAttributeFile returns filename for an attribute.
func (index *IIODIndex) LookupAttributeFile(dev, ch, attr string) (string, error) {
	devMap, ok := index.AttrFiles[dev]
	if !ok {
		return "", fmt.Errorf("device %q not found", dev)
	}

	chMap, ok := devMap[ch]
	if !ok {
		return "", fmt.Errorf("channel %q not found in device %q", ch, dev)
	}

	if f, ok := chMap[attr]; ok {
		return f, nil
	}

	return "", fmt.Errorf("attribute %q not found in device %q channel %q", attr, dev, ch)
}

// -----------------------------------------------------------------------------
// Raw string dump (useful for debugging malformed XML)
// -----------------------------------------------------------------------------

func NormalizeXMLForDebug(raw []byte) string {
	s := string(raw)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

// -----------------------------------------------------------------------------
// ParseScanFormat
// -----------------------------------------------------------------------------
// ParseScanFormat parses an IIOD scan-element format string into a ScanFormat struct.
// scaleStr may be nil or a pointer to a string containing the "scale=" attribute.
func (ce *ChannelEntry) ParseScanFormat() error {

	format := strings.TrimSpace(ce.ScanElementRaw.Format)
	m := scanFmtRe.FindStringSubmatch(format)
	if m == nil {
		return fmt.Errorf("invalid scan format: %q", format)
	}

	index, err := strconv.ParseUint(ce.ScanElementRaw.Index, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid index value %q: %w", ce.ScanElementRaw.Index, err)
	}

	endianPart := m[1]
	signPart := m[2]
	bitsStr := m[3]
	lengthStr := m[4]
	repeatNum := m[5] // n
	shiftStr := m[6]

	// --- Endianness ---
	isBE := (endianPart == "be")

	// --- Signedness ---
	// lowercase s/u → normal signed/unsigned
	// uppercase S/U → "fully-defined" ABI
	isSigned := false
	fully := false

	switch signPart {
	case "s":
		isSigned = true
	case "u":
		isSigned = false
	case "S":
		isSigned = true
		fully = true
	case "U":
		isSigned = false
		fully = true
	default:
		return errors.New("invalid sign specifier")
	}

	// --- Numeric fields ---
	bits, err := strconv.ParseUint(bitsStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid bits value %q: %w", bitsStr, err)
	}

	length, err := strconv.ParseUint(lengthStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid length value %q: %w", lengthStr, err)
	}

	if bits == 0 || length == 0 {
		return errors.New("bits and length must be > 0")
	}
	if bits > length {
		return fmt.Errorf("bits (%d) cannot exceed length (%d)", bits, length)
	}

	// --- Repeat (optional) ---
	repeat := uint64(1)
	if repeatNum != "" {
		repeat, err = strconv.ParseUint(repeatNum, 10, 32)
		if err != nil {
			return fmt.Errorf("invalid repeat value %q: %w", repeatNum, err)
		}
		if repeat == 0 {
			return errors.New("repeat must be >= 1")
		}
	}

	// --- Shift ---
	shift, err := strconv.ParseUint(shiftStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid shift value %q: %w", shiftStr, err)
	}

	// --- Optional scale attribute ---
	var scale float64
	withScale := false
	if ce.ScanElementRaw != nil && ce.ScanElementRaw.Scale != "" {
		s := strings.TrimSpace(ce.ScanElementRaw.Scale)
		if s != "" {
			scale, err = strconv.ParseFloat(s, 64)
			if err != nil {
				return fmt.Errorf("invalid scale attribute %q: %w", s, err)
			}
			withScale = true
		}
	}

	ce.ParsedFormat = &ScanFormat{
		Index:        uint32(index),
		IsBE:         isBE,
		IsSigned:     isSigned,
		Bits:         uint32(bits),
		Length:       uint32(length),
		Repeat:       uint32(repeat),
		Shift:        uint32(shift),
		FullyDefined: fully,
		WithScale:    withScale,
		Scale:        scale,
	}
	return nil
}

func ComputeSampleSize(channels []*ChannelEntry) int {
	size := 0
	for _, ch := range channels {
		if ch.ParsedFormat == nil {
			continue
		}
		bits := ch.ParsedFormat.Length * ch.ParsedFormat.Repeat
		size += int((bits + 7) / 8) // round up to full bytes
	}
	return size
}

// ComputeDeviceSampleSize computes the total sample size for a device.
// This is the sum of the sample sizes of all enabled channels.
func (dev *DeviceEntry) ComputeDeviceSampleSize(enabled []*ChannelEntry) uint32 {
	total := uint32(0)
	for _, ch := range enabled {
		total += ch.SampleSize
	}
	dev.SampleSize = total
	return total
}

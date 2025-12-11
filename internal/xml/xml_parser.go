package sdrxml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
)

// IIODIndex provides fast lookup structures for devices,
// channels, attributes, and filenames.
// This is built from the parsed XML.
type IIODIndex struct {
	DevicesByID   map[string]*DeviceEntry
	DevicesByName map[string]*DeviceEntry
	Channels      map[string]map[string]*ChannelEntry     // devName → chName → entry
	AttrFiles     map[string]map[string]map[string]string // dev → ch → attr → filename
}

// -----------------------------------------------------------------------------
// PUBLIC: ParseIIODXML
// The most important thing of a parser is that it can parse the XML file.
// -----------------------------------------------------------------------------

// Parse decodes the raw IIOD XML stream into the SDRContext receiver and builds
// a lookup index for fast access.
func (ctx *SDRContext) Parse(raw []byte) (*IIODIndex, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("empty XML data")
	}

	if err := xml.Unmarshal(raw, ctx); err != nil {
		return nil, fmt.Errorf("IIOD XML parse error: %w", err)
	}

	// Build fast lookup index
	index, err := BuildIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("IIOD XML index build error: %w", err)
	}

	return index, nil
}

// ParseIIODXML is kept for compatibility and delegates to SDRContext.Parse.
func ParseIIODXML(raw []byte) (*SDRContext, *IIODIndex, error) {
	var ctx SDRContext
	index, err := ctx.Parse(raw)
	if err != nil {
		return nil, nil, err
	}
	return &ctx, index, nil
}

// -----------------------------------------------------------------------------
// BuildIndex - construct lookup tables from IIODcontext
// -----------------------------------------------------------------------------

func BuildIndex(ctx *SDRContext) (*IIODIndex, error) {
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

	return idx, nil
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

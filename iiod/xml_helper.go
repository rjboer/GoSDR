package iiod

import (
	"fmt"
	"sort"
	"strings"
)

// -----------------------------------------------------------------------------
// HIGH-LEVEL XML INSPECTION HELPERS
// These helpers operate on IIODcontext + IIODIndex.
// They are used by connect.go, textbased.go, and pluto.go.
// -----------------------------------------------------------------------------

// GetDeviceList returns a sorted list of device names from XML.
func GetDeviceList(index *IIODIndex) []string {
	if index == nil {
		return nil
	}
	out := make([]string, 0, len(index.DevicesByName))
	for name := range index.DevicesByName {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// GetDeviceIDs returns a sorted list of device IDs such as "iio:device0".
func GetDeviceIDs(index *IIODIndex) []string {
	if index == nil {
		return nil
	}
	out := make([]string, 0, len(index.DevicesByID))
	for id := range index.DevicesByID {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// GetChannelList returns sorted channel names for a device.
func GetChannelList(index *IIODIndex, devName string) ([]string, error) {
	if index == nil {
		return nil, fmt.Errorf("index is nil")
	}
	devMap, ok := index.Channels[devName]
	if !ok {
		return nil, fmt.Errorf("device %q not found", devName)
	}
	out := make([]string, 0, len(devMap))
	for ch := range devMap {
		out = append(out, ch)
	}
	sort.Strings(out)
	return out, nil
}

// GetAttributeList returns sorted attribute names for a device+channel.
func GetAttributeList(index *IIODIndex, devName, chName string) ([]string, error) {
	if index == nil {
		return nil, fmt.Errorf("index is nil")
	}
	devMap, ok := index.AttrFiles[devName]
	if !ok {
		return nil, fmt.Errorf("device %q not found", devName)
	}
	chMap, ok := devMap[chName]
	if !ok {
		return nil, fmt.Errorf("channel %q not found in device %q", chName, devName)
	}

	out := make([]string, 0, len(chMap))
	for attr := range chMap {
		out = append(out, attr)
	}
	sort.Strings(out)
	return out, nil
}

// FilenameForAttribute returns the sysfs filename for a given device/channel/attribute.
func FilenameForAttribute(index *IIODIndex, devName, chName, attr string) (string, error) {
	return index.LookupAttributeFile(devName, chName, attr)
}

// -----------------------------------------------------------------------------
// ATTRIBUTE LOOKUP WITH BEST-EFFORT FALLBACK
// -----------------------------------------------------------------------------

// TryResolveAttribute tries multiple fallback resolutions:
// 1. direct attribute name match
// 2. lowercased attribute
// 3. strip prefixes such as "in_" or "out_"
// 4. attempt fuzzy match
//
// This improves robustness with PlutoSDR variations.
func TryResolveAttribute(index *IIODIndex, dev, ch, attr string) (string, error) {
	// Attempt exact file lookup
	filename, err := index.LookupAttributeFile(dev, ch, attr)
	if err == nil {
		return filename, nil
	}

	// Lowercase retry
	lattr := strings.ToLower(attr)
	for k := range index.AttrFiles[dev][ch] {
		if strings.ToLower(k) == lattr {
			return index.AttrFiles[dev][ch][k], nil
		}
	}

	// Strip prefix
	stripPrefixes := []string{"in_", "out_", "voltage_", "altvoltage_"}
	for _, p := range stripPrefixes {
		if strings.HasPrefix(attr, p) {
			s2 := strings.TrimPrefix(attr, p)
			for k := range index.AttrFiles[dev][ch] {
				if strings.EqualFold(k, s2) {
					return index.AttrFiles[dev][ch][k], nil
				}
			}
		}
	}

	// Fuzzy match (rarely needed)
	for k := range index.AttrFiles[dev][ch] {
		if strings.Contains(strings.ToLower(k), lattr) {
			return index.AttrFiles[dev][ch][k], nil
		}
	}

	return "", fmt.Errorf("attribute %q could not be resolved for device=%q channel=%q", attr, dev, ch)
}

// -----------------------------------------------------------------------------
// DEVICE IDENTIFICATION HELPERS
// -----------------------------------------------------------------------------

// FindDeviceByLabel tries to locate a device using both:
//   - XML "name" field (common in PlutoSDR)
//   - XML "label" attribute (less common, but used in some builds)
func FindDeviceByLabel(index *IIODIndex, label string) (*DeviceEntry, error) {
	for _, dev := range index.DevicesByName {
		if dev.Label == label {
			return dev, nil
		}
	}
	return nil, fmt.Errorf("device with label=%q not found", label)
}

// FindDevicesWithPrefix returns a list of devices matching a prefix.
func FindDevicesWithPrefix(index *IIODIndex, prefix string) []string {
	var out []string
	for name := range index.DevicesByName {
		if strings.HasPrefix(name, prefix) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// -----------------------------------------------------------------------------
// CHANNEL UTILITIES
// -----------------------------------------------------------------------------

// FindRXChannels returns all "input" channels of a device.
func FindRXChannels(index *IIODIndex, devName string) ([]*ChannelEntry, error) {
	dev, ok := index.Channels[devName]
	if !ok {
		return nil, fmt.Errorf("device %q not found", devName)
	}
	var out []*ChannelEntry
	for _, ch := range dev {
		if ch.Type == "input" {
			out = append(out, ch)
		}
	}
	return out, nil
}

// FindTXChannels returns all "output" channels of a device.
func FindTXChannels(index *IIODIndex, devName string) ([]*ChannelEntry, error) {
	dev, ok := index.Channels[devName]
	if !ok {
		return nil, fmt.Errorf("device %q not found", devName)
	}
	var out []*ChannelEntry
	for _, ch := range dev {
		if ch.Type == "output" {
			out = append(out, ch)
		}
	}
	return out, nil
}

// -----------------------------------------------------------------------------
// ATTRIBUTE INSPECTION
// -----------------------------------------------------------------------------

// ListAttributes returns a map of attribute → filename for a channel.
func ListAttributes(index *IIODIndex, dev, ch string) (map[string]string, error) {
	devMap, ok := index.AttrFiles[dev]
	if !ok {
		return nil, fmt.Errorf("device %q not found", dev)
	}
	chMap, ok := devMap[ch]
	if !ok {
		return nil, fmt.Errorf("channel %q not found", ch)
	}
	return chMap, nil
}

// -----------------------------------------------------------------------------
// REVERSE LOOKUP FOR BINARY MODE (future support)
// -----------------------------------------------------------------------------

// ReverseLookupAttribute returns devID, channelIndex, attrIndex.
// This ties XML filenames to binary protocol opcodes.
//
// For now this is a stub that is safe for Pluto,
// which does NOT use binary attribute opcodes.
func ReverseLookupAttribute(index *IIODIndex, dev, ch, attr string) (int, int, int, error) {
	// Pluto does not support binary opcodes; return dummy values.
	return -1, -1, -1, fmt.Errorf("binary attribute opcodes not available for Pluto (text-only)")
}

// -----------------------------------------------------------------------------
// DEBUGGING UTILITIES
// -----------------------------------------------------------------------------

// DumpDeviceTree prints a human-readable view.
func DumpDeviceTree(index *IIODIndex) string {
	var sb strings.Builder
	for devName := range index.DevicesByName {
		sb.WriteString(fmt.Sprintf("Device: %s\n", devName))

		chList, _ := GetChannelList(index, devName)
		for _, ch := range chList {
			sb.WriteString(fmt.Sprintf("  Channel: %s\n", ch))

			attrs, _ := GetAttributeList(index, devName, ch)
			for _, a := range attrs {
				filename, _ := FilenameForAttribute(index, devName, ch, a)
				sb.WriteString(fmt.Sprintf("    Attr: %-25s File: %s\n", a, filename))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

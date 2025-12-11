package sdrxml

import "encoding/xml"

// SDRContext represents the full IIOD XML schema.
// This struct is derived from actual PlutoSDR firmware XML output (v0.25/v0.38)
// and is compatible with older & newer IIOD releases.
type SDRContext struct {
	XMLName          xml.Name           `xml:"context" json:"context"`
	Text             string             `xml:",chardata" json:"text,omitempty"`
	Name             string             `xml:"name,attr" json:"name"`
	VersionMajor     string             `xml:"version-major,attr" json:"version-major"`
	VersionMinor     string             `xml:"version-minor,attr" json:"version-minor"`
	VersionGit       string             `xml:"version-git,attr" json:"version-git"`
	Description      string             `xml:"description,attr" json:"description"`
	ContextAttribute []ContextAttribute `xml:"context-attribute" json:"context-attribute"`
	Device           []DeviceEntry      `xml:"device" json:"device"`
	Index            *IIODIndex         `json:"index,omitempty"`
	ScanFormat       []ScanFormat       `json:"scan-format,omitempty"`
}

// IIODIndex provides fast lookup structures for devices,
// channels, attributes, and filenames.
// This is built from the parsed XML.
type IIODIndex struct {
	DevicesByID   map[string]*DeviceEntry
	DevicesByName map[string]*DeviceEntry
	Channels      map[string]map[string]*ChannelEntry     // devName → chName → entry
	AttrFiles     map[string]map[string]map[string]string // dev → ch → attr → filename
	NoDevices     int
	NoChannels    int
}

// ScanFormat represents the parsed IIOD scan-element format.
// This mirrors libiio's internal struct iio_data_format.
type ScanFormat struct {
	Index        uint32  // index of the scan element
	IsBE         bool    // big-endian (true) or little-endian (false)
	IsSigned     bool    // signed (true) or unsigned (false)
	Bits         uint32  // number of meaningful bits
	Length       uint32  // total bits allocated for storage
	Repeat       uint32  // number of repeated values (X2 etc.)
	Shift        uint32  // right-shift applied to the raw storage
	FullyDefined bool    // S/U variants use "fully defined" ABI semantics
	WithScale    bool    // true if scale attribute was present
	Scale        float64 // optional scaling factor
}

// -----------------------------------------------------------------------------
// CONTEXT-LEVEL ATTRIBUTES
// -----------------------------------------------------------------------------

type ContextAttribute struct {
	Text  string `xml:",chardata" json:"text,omitempty"`
	Name  string `xml:"name,attr" json:"name"`
	Value string `xml:"value,attr" json:"value"`
}

// -----------------------------------------------------------------------------
// DEVICE
// -----------------------------------------------------------------------------

type DeviceEntry struct {
	Text  string `xml:",chardata" json:"text,omitempty"`
	ID    string `xml:"id,attr" json:"id"`
	Name  string `xml:"name,attr" json:"name"`
	Label string `xml:"label,attr" json:"label,omitempty"` // not always present

	Channel         []ChannelEntry    `xml:"channel" json:"channel"`
	Attribute       []DevAttribute    `xml:"attribute" json:"attribute"`
	DebugAttribute  []DebugAttribute  `xml:"debug-attribute" json:"debug-attribute"`
	BufferAttribute []BufferAttribute `xml:"buffer-attribute" json:"buffer-attribute"`
}

// -----------------------------------------------------------------------------
// CHANNEL
// -----------------------------------------------------------------------------

type ChannelEntry struct {
	Text string `xml:",chardata" json:"text,omitempty"`
	ID   string `xml:"id,attr" json:"id"`
	Name string `xml:"name,attr" json:"name,omitempty"`
	Type string `xml:"type,attr" json:"type"` // input | output

	Attribute      []ChannelAttr `xml:"attribute" json:"attribute"`
	ScanElementRaw *ScanElement  `xml:"scan-element" json:"scan-element,omitempty"`
	ParsedFormat   *ScanFormat   `json:"parsed-format,omitempty"`
}

// -----------------------------------------------------------------------------
// ATTRIBUTE TYPES
// -----------------------------------------------------------------------------

// Device-level attribute (no filename)
type DevAttribute struct {
	Text string `xml:",chardata" json:"text,omitempty"`
	Name string `xml:"name,attr" json:"name"`
}

// Debug attribute
type DebugAttribute struct {
	Text string `xml:",chardata" json:"text,omitempty"`
	Name string `xml:"name,attr" json:"name"`
}

// Buffer attribute
type BufferAttribute struct {
	Text string `xml:",chardata" json:"text,omitempty"`
	Name string `xml:"name,attr" json:"name"`
}

// Channel attribute (includes filename)
type ChannelAttr struct {
	Text     string `xml:",chardata" json:"text,omitempty"`
	Name     string `xml:"name,attr" json:"name"`
	Filename string `xml:"filename,attr" json:"filename,omitempty"`
}

// -----------------------------------------------------------------------------
// SCAN ELEMENT
// -----------------------------------------------------------------------------

type ScanElement struct {
	Text   string `xml:",chardata" json:"text,omitempty"`
	Index  string `xml:"index,attr" json:"index"`
	Format string `xml:"format,attr" json:"format"`
	Scale  string `xml:"scale,attr" json:"scale"`
}

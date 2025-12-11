package sdrxml

import (
	"os"
	"testing"
)

func TestParseIIODXMLBuildsIndex(t *testing.T) {
	raw, err := os.ReadFile("pluto.xml")
	if err != nil {
		t.Fatalf("failed to read example XML: %v", err)
	}

	var ctx SDRContext
	index, err := ctx.Parse(raw)
	if err != nil {
		t.Fatalf("expected XML to parse, got error: %v", err)
	}

	if index == nil {
		t.Fatalf("expected index to be non-nil")
	}

	if ctx.Name != "local" || ctx.VersionMajor != "0" || ctx.VersionMinor != "25" {
		t.Fatalf("unexpected context metadata: %+v", ctx)
	}

	if len(ctx.Device) != 4 {
		t.Fatalf("expected 4 devices in the example XML, got %d", len(ctx.Device))
	}

	devByName, err := index.LookupDevice("ad9361-phy")
	if err != nil {
		t.Fatalf("LookupDevice by name failed: %v", err)
	}

	devByID, err := index.LookupDevice("iio:device0")
	if err != nil {
		t.Fatalf("LookupDevice by ID failed: %v", err)
	}

	if devByName != devByID {
		t.Fatalf("device lookup by name and ID should reference the same entry")
	}

	channel, err := index.LookupChannel("ad9361-phy", "TX_LO")
	if err != nil {
		t.Fatalf("LookupChannel failed: %v", err)
	}

	if channel.ID != "altvoltage1" || channel.Type != "output" {
		t.Fatalf("unexpected channel attributes: %+v", channel)
	}

	filename, err := index.LookupAttributeFile("ad9361-phy", "TX_LO", "external")
	if err != nil {
		t.Fatalf("LookupAttributeFile failed: %v", err)
	}

	if filename != "out_altvoltage1_TX_LO_external" {
		t.Fatalf("unexpected filename for attribute: %s", filename)
	}
}

package sdrxml
import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseIIODXMLBuildsIndex(t *testing.T) {
	raw, err := os.ReadFile("pluto.xml")
// examplePath returns the absolute path to one of the example XML files
// that live alongside this test file.
func examplePath(name string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), name)
}

func loadExampleXML(t *testing.T, name string) []byte {
	t.Helper()

	raw, err := os.ReadFile(examplePath(name))
	if err != nil {
		t.Fatalf("failed to read example XML: %v", err)
		t.Fatalf("failed to read example XML %q: %v", name, err)
	}

	return raw
}

func TestParsePlutoXMLBuildsIndex(t *testing.T) {
	raw := loadExampleXML(t, "pluto.xml")

	var ctx SDRContext
	index, err := ctx.Parse(raw)
	if err != nil {
	if err := ctx.Parse(raw); err != nil {
		t.Fatalf("expected XML to parse, got error: %v", err)
	}

	if index == nil {
		t.Fatalf("expected index to be non-nil")
	if ctx.Index == nil {
		t.Fatalf("expected index to be built")
	}

	if ctx.Name != "local" || ctx.VersionMajor != "0" || ctx.VersionMinor != "25" {
@@ -29,12 +45,14 @@ func TestParseIIODXMLBuildsIndex(t *testing.T) {
		t.Fatalf("expected 4 devices in the example XML, got %d", len(ctx.Device))
	}

	devByName, err := index.LookupDevice("ad9361-phy")
	idx := ctx.Index

	devByName, err := idx.LookupDevice("ad9361-phy")
	if err != nil {
		t.Fatalf("LookupDevice by name failed: %v", err)
	}

	devByID, err := index.LookupDevice("iio:device0")
	devByID, err := idx.LookupDevice("iio:device0")
	if err != nil {
		t.Fatalf("LookupDevice by ID failed: %v", err)
	}
@@ -43,7 +61,7 @@ func TestParseIIODXMLBuildsIndex(t *testing.T) {
		t.Fatalf("device lookup by name and ID should reference the same entry")
	}

	channel, err := index.LookupChannel("ad9361-phy", "TX_LO")
	channel, err := idx.LookupChannel("ad9361-phy", "TX_LO")
	if err != nil {
		t.Fatalf("LookupChannel failed: %v", err)
	}
@@ -52,7 +70,7 @@ func TestParseIIODXMLBuildsIndex(t *testing.T) {
		t.Fatalf("unexpected channel attributes: %+v", channel)
	}

	filename, err := index.LookupAttributeFile("ad9361-phy", "TX_LO", "external")
	filename, err := idx.LookupAttributeFile("ad9361-phy", "TX_LO", "external")
	if err != nil {
		t.Fatalf("LookupAttributeFile failed: %v", err)
	}
@@ -61,3 +79,44 @@ func TestParseIIODXMLBuildsIndex(t *testing.T) {
		t.Fatalf("unexpected filename for attribute: %s", filename)
	}
}

func TestParseAllExampleXMLs(t *testing.T) {
	tests := map[string]int{
		"ad5541a.xml":   1,
		"ad5628-1.xml":  1,
		"ad7091r.xml":   1,
		"adis16488.xml": 1,
		"pluto.xml":     4,
	}

	for name, expectedDevices := range tests {
		name, expectedDevices := name, expectedDevices
		t.Run(name, func(t *testing.T) {
			var ctx SDRContext
			if err := ctx.Parse(loadExampleXML(t, name)); err != nil {
				t.Fatalf("Parse(%s) returned error: %v", name, err)
			}

			if ctx.Index == nil {
				t.Fatalf("expected index to be populated for %s", name)
			}

			if len(ctx.Device) != expectedDevices {
				t.Fatalf("expected %d devices in %s, got %d", expectedDevices, name, len(ctx.Device))
			}

			if ctx.Index.NoDevices != expectedDevices {
				t.Fatalf("expected index to report %d devices in %s, got %d", expectedDevices, name, ctx.Index.NoDevices)
			}

			indexedChannels := 0
			for _, devChannels := range ctx.Index.Channels {
				indexedChannels += len(devChannels)
			}

			if ctx.Index.NoChannels != indexedChannels {
				t.Fatalf("expected index to track %d channels in %s, got %d", indexedChannels, name, ctx.Index.NoChannels)
			}
		})
	}
}
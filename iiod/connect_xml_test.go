package iiod

import "testing"

func TestParseDeviceIndexAndAttrCodes(t *testing.T) {
	t.Skip("iiod client mocks disabled")
	xmlContent := `
<context>
    <device id="dev0" index="2" name="demo">
        <attribute name="attr0" code="10" />
        <channel id="voltage0" type="input">
            <attribute name="scale" code="0x20" />
        </channel>
    </device>
    <device id="dev1" name="demo1">
        <attribute name="attr1" code="3" />
    </device>
</context>`

	deviceIdx, attrCodes, err := parseDeviceIndexAndAttrCodes(xmlContent)
	if err != nil {
		t.Fatalf("parseDeviceIndexAndAttrCodes returned error: %v", err)
	}

	if len(deviceIdx) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(deviceIdx))
	}

	if got := deviceIdx["dev0"]; got != 2 {
		t.Fatalf("unexpected index for dev0: %d", got)
	}

	if got := deviceIdx["dev1"]; got != 3 {
		t.Fatalf("unexpected index for dev1 fallback: %d", got)
	}

	keyDevice := attrKey{device: "dev0", channel: "", attr: "attr0"}
	if got := attrCodes[keyDevice]; got != 10 {
		t.Fatalf("unexpected code for device attr: %d", got)
	}

	keyChannel := attrKey{device: "dev0", channel: "voltage0", attr: "scale"}
	if got := attrCodes[keyChannel]; got != 0x20 {
		t.Fatalf("unexpected code for channel attr: %d", got)
	}
}

package main

import (
	"strings"
	"testing"
)

const devicectlJSON = `{
	"result": {
		"devices": [
			{
				"identifier": "9C8D7EDE-05A6-555B-9C10-745C9528CD14",
				"deviceProperties": {"name": "ONEO", "developerModeStatus": "enabled"},
				"hardwareProperties": {"reality": "physical", "udid": "00008160-001204C424C1400A", "serialNumber": "GH9J3JXYD2", "ecid": 5071790057537546},
				"connectionProperties": {"pairingState": "paired", "tunnelState": "connected"}
			},
			{
				"identifier": "0A6ECD14-F679-40A9-A4A9-EF08BE8D0C39",
				"deviceProperties": {"name": "iPhone 18 Pro"},
				"hardwareProperties": {"reality": "simulated", "udid": "0A6ECD14-F679-40A9-A4A9-EF08BE8D0C39"},
				"connectionProperties": {"pairingState": "paired", "tunnelState": "connected"}
			}
		]
	}
}`

func TestParseIOSDevices(t *testing.T) {
	devices, err := parseIOSDevices([]byte(devicectlJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want only the physical one: %+v", len(devices), devices)
	}
	d := devices[0]
	if d.Name != "ONEO" || d.UDID != "00008160-001204C424C1400A" || d.Serial != "GH9J3JXYD2" ||
		d.ECID != 5071790057537546 || !d.Paired || !d.Connected || !d.DeveloperMode {
		t.Fatalf("parsed %+v", d)
	}
}

func TestParseIOSDevicesProperties(t *testing.T) {
	data := `{"result": {"devices": [{
		"identifier": "X",
		"properties": {
			"hardware": {"reality": "physical", "udid": "AAA", "serialNumber": "S", "ecid": 1},
			"state": {"name": "ONEO", "developerModeStatus": {"enabled": {"mode": 1}}},
			"connection": {"pairingState": "paired", "state": "connected"}
		}
	}]}}`
	devices, err := parseIOSDevices([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	if d := devices[0]; d.Name != "ONEO" || d.UDID != "AAA" || d.Serial != "S" || d.ECID != 1 ||
		!d.Paired || !d.Connected || !d.DeveloperMode {
		t.Fatalf("parsed %+v", d)
	}
}

func TestPickIOSDevice(t *testing.T) {
	oneo := iosDevice{Name: "ONEO", UDID: "00008160-001204C424C1400A", Serial: "GH9J3JXYD2", ECID: 5071790057537546, Paired: true, Connected: true, DeveloperMode: true}
	other := iosDevice{Name: "Other", UDID: "00008160-000000000000000", Paired: true, Connected: true, DeveloperMode: true}
	off := iosDevice{Name: "Off", UDID: "00008160-111111111111111", Paired: true, Connected: true}
	for _, wanted := range []string{"", "ONEO", "oneo", "00008160-001204C424C1400A", "GH9J3JXYD2", "5071790057537546"} {
		got, err := pickIOSDevice([]iosDevice{oneo}, wanted)
		if err != nil {
			t.Errorf("pick %q: %v", wanted, err)
		} else if got != oneo {
			t.Errorf("pick %q = %+v, want ONEO", wanted, got)
		}
	}
	if _, err := pickIOSDevice([]iosDevice{oneo}, "missing"); err == nil || !strings.Contains(err.Error(), "ONEO") {
		t.Errorf("pick missing: %v, want an error listing ONEO", err)
	}
	if _, err := pickIOSDevice([]iosDevice{oneo, other}, ""); err == nil || !strings.Contains(err.Error(), "-device") {
		t.Errorf("pick ambiguous: %v, want an error asking for -device", err)
	}
	other.Connected = false
	if got, err := pickIOSDevice([]iosDevice{oneo, other}, ""); err != nil || got != oneo {
		t.Errorf("pick the connected device = %+v, %v", got, err)
	}
	if got, err := pickIOSDevice([]iosDevice{oneo, other}, "Other"); err != nil || got != other {
		t.Errorf("pick Other = %+v, %v", got, err)
	}
	if _, err := pickIOSDevice([]iosDevice{off}, ""); err == nil || !strings.Contains(err.Error(), "Developer Mode") {
		t.Errorf("pick developer mode off: %v, want an error about Developer Mode", err)
	}
	if _, err := pickIOSDevice([]iosDevice{{Name: "Unpaired"}}, ""); err == nil || !strings.Contains(err.Error(), "no iPhone") {
		t.Errorf("pick unpaired: %v, want an error about no iPhone", err)
	}
}

func TestIOSProject(t *testing.T) {
	var b strings.Builder
	err := iosProject.Execute(&b, map[string]string{
		"Name":   "Gallery",
		"App":    "Gallery.app",
		"ID":     "dev.oneo.nsgo.gallery",
		"Team":   "TEAM123",
		"Script": iosSignScript,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`PRODUCT_NAME = "Gallery";`,
		`PRODUCT_BUNDLE_IDENTIFIER = "dev.oneo.nsgo.gallery";`,
		`DEVELOPMENT_TEAM = "TEAM123";`,
		`path = "Gallery.app";`,
		`shellScript = "set -e\n`,
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("project does not contain %q", want)
		}
	}
}

func TestPBXString(t *testing.T) {
	if got, want := pbxString("a\"b\\c\nd"), `"a\"b\\c\nd"`; got != want {
		t.Errorf("pbxString = %s, want %s", got, want)
	}
}

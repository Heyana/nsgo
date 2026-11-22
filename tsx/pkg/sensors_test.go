package pkg

import "testing"

func TestSensorCapabilitiesParseAndStayIndependent(t *testing.T) {
	c, ignored, err := ParseCapabilities([]byte(`{"location":true,"backgroundLocation":false,"motion":true}`), true)
	if err != nil || len(ignored) != 0 {
		t.Fatalf("capabilities: %v %v", ignored, err)
	}
	if !c.Allows("location", "", "") || !c.Allows("motion", "", "") || c.Allows("backgroundLocation", "", "") {
		t.Fatal("sensor capabilities were conflated")
	}
	if _, _, err := ParseCapabilities([]byte(`{"location":"yes"}`), true); err == nil {
		t.Fatal("invalid sensor capability accepted")
	}
	c, _, err = ParseCapabilities([]byte(`{"location":true,"backgroundLocation":true}`), true)
	if err != nil || !c.Allows("backgroundLocation", "", "") || c.Allows("motion", "", "") {
		t.Fatal("background capability granted motion")
	}
}

func TestBackgroundMotionRequiresItsOwnPackageCapability(t *testing.T) {
	for _, raw := range []string{`{}`, `{"motion":true}`, `{"backgroundMotion":false}`} {
		c, _, err := ParseCapabilities([]byte(raw), true)
		if err != nil {
			t.Fatal(err)
		}
		if c.Allows("backgroundMotion", "", "") {
			t.Fatalf("%s allowed background motion", raw)
		}
	}
	c, ignored, err := ParseCapabilities([]byte(`{"motion":true,"backgroundMotion":true}`), true)
	if err != nil || len(ignored) != 0 || !c.Allows("backgroundMotion", "", "") || !c.Allows("motion", "", "") {
		t.Fatalf("capabilities %+v %v %v", c, ignored, err)
	}
	if _, _, err := ParseCapabilities([]byte(`{"backgroundMotion":"yes"}`), true); err == nil {
		t.Fatal("invalid background grant accepted")
	}
}

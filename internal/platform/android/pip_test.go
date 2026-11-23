package android

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestPictureInPictureRuntimeEvents(t *testing.T) {
	for _, data := range []string{
		`{"state":{"supported":true,"possible":false,"error":"not ready"}}`,
		`{"state":{"supported":true,"possible":true,"active":true}}`,
		`{"action":"restore"}`,
	} {
		event, ok := readPictureInPictureEvent(data)
		if !ok {
			t.Fatalf("rejected runtime event %s", data)
		}
		if event.Action == "" && !event.State.Supported {
			t.Fatalf("lost runtime state: %+v", event)
		}
	}
	if _, ok := readPictureInPictureEvent("{broken"); ok {
		t.Fatal("accepted a malformed runtime event")
	}
	var command struct{ Command, Token uint32 }
	if err := json.Unmarshal([]byte(pictureInPictureCommandJSON(driver.PictureInPictureRestored, 42)), &command); err != nil {
		t.Fatal(err)
	}
	if command.Command != driver.PictureInPictureRestored || command.Token != 42 {
		t.Fatalf("restoration command = %+v", command)
	}
}

// The runtime advertises video PiP in the fourth capability byte, with a bit no other capability there uses, and Protocol.java agrees with protocol.go.
func TestPictureInPictureCapabilityBit(t *testing.T) {
	protocol, err := os.ReadFile("java/dev/oneo/nsgo/Protocol.java")
	if err != nil {
		t.Fatal(err)
	}
	if m := regexp.MustCompile(`static final int CAP_VIDEO_PIP = (\d+);`).FindSubmatch(protocol); m == nil || string(m[1]) != strconv.Itoa(capVideoPiP) {
		t.Fatalf("Protocol.java's CAP_VIDEO_PIP is %q, protocol.go's capVideoPiP %d", m, capVideoPiP)
	}
	if capVideoPiP&(capExtendedSensors|capAuthSession) != 0 {
		t.Fatal("capVideoPiP shares a fourth-byte bit with another capability")
	}
}

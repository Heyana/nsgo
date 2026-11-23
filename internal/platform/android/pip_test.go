package android

import (
	"encoding/json"
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

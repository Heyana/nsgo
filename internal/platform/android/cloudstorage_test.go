//go:build android || nsgoandroidhost

package android

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestCloudStorageUnavailableReply keeps the reply Go makes up for a runtime without extended messages in the shape package cloud waits for: a "reply" to the command's ID with the "unavailable" error, which CloudStorage.java's replyError writes too.
func TestCloudStorageUnavailableReply(t *testing.T) {
	var r struct {
		Type  string `json:"type"`
		ID    uint64 `json:"id"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(cloudStorageUnavailable([]byte(`{"op":"get","id":7,"key":"k"}`)), &r); err != nil {
		t.Fatal(err)
	}
	if r.Type != "reply" || r.ID != 7 || r.Error != "unavailable" {
		t.Fatalf("reply %+v, want an unavailable reply to 7", r)
	}
	java, err := os.ReadFile("java/dev/oneo/nsgo/CloudStorage.java")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(java), `r.put("type", "reply");`) {
		t.Fatal(`CloudStorage.java no longer replies with type "reply"`)
	}
	view, err := os.ReadFile("java/dev/oneo/nsgo/NsgoView.java")
	if err != nil {
		t.Fatal(err)
	}
	// The environment's second capability byte carries the probed sign-in in the bit readEnvironment reads as CloudStorage.
	if !strings.Contains(string(view), "(CloudStorage.signedIn ? 128 : 0)") {
		t.Fatal("NsgoView does not carry Play Games' sign-in in bit 128 of the second capability byte")
	}
}

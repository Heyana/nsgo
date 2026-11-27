//go:build android || nsgoandroidhost

package android

import (
	"encoding/json"
	"testing"
)

// TestGameServicesUnavailableReply keeps the reply Go synthesizes for an older runtime without extended messages in the JSON shape gameservices_android.go decodes.
func TestGameServicesUnavailableReply(t *testing.T) {
	var r struct {
		Type  string `json:"type"`
		ID    uint64 `json:"id"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(gameServicesUnavailable([]byte(`{"op":"authenticate","id":9}`)), &r); err != nil {
		t.Fatal(err)
	}
	if r.Type != "result" || r.ID != 9 || r.Error != "unavailable" {
		t.Fatalf("reply %+v, want an unavailable result for id 9", r)
	}
}

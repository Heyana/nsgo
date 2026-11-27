//go:build js && wasm

package harmonyos

import (
	"encoding/json"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestCanvasImagesCarryTheirTint checks that a tinted image op sends its tint where Index.ets reads it, even a transparent black tint, and that an untinted image sends none.
func TestCanvasImagesCarryTheirTint(t *testing.T) {
	img := driver.ImageSource{URI: "https://example.com/a.png"}
	var d driver.Drawing
	d.DrawImage(img, 0, 0)
	d.DrawTintedImage(img, driver.Color(0x80ff0000), 1, 2)
	d.DrawTintedImage(img, 0, 3, 4)
	var got []map[string]any
	if err := json.Unmarshal([]byte(canvasJSON(&d, d.Ops, true)), &got); err != nil {
		t.Fatal(err)
	}
	var images []map[string]any
	for _, o := range got {
		if o["k"] == float64(driver.DrawImage) {
			images = append(images, o)
		}
	}
	if len(images) != 3 {
		t.Fatalf("got %d image ops, want 3: %v", len(images), got)
	}
	if _, ok := images[0]["tint"]; ok {
		t.Errorf("untinted image sent a tint: %v", images[0])
	}
	if images[1]["tint"] != float64(0x80ff0000) {
		t.Errorf("tinted image sent tint %v, want %v", images[1]["tint"], float64(0x80ff0000))
	}
	if tint, ok := images[2]["tint"]; !ok || tint != float64(0) {
		t.Errorf("a zero tint must still be sent, got %v (present %v)", tint, ok)
	}
}

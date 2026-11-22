//go:build js && wasm

package harmonyos

import (
	"encoding/json"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

// TestCanvasImagesCarryTheirSampling checks that each image op of the sampling drawing tells the host whether to smooth it, through Save, Restore and a smoothing set and restored in between: the left draws smooth and the right nearest.
func TestCanvasImagesCarryTheirSampling(t *testing.T) {
	d := driver.NewDrawing(nil)
	canvastest.DrawSampling(d, driver.ImageSource{URI: "asset:checker.png"}, 64)
	var ops []canvasOp
	if err := json.Unmarshal([]byte(canvasJSON(d.Ops)), &ops); err != nil {
		t.Fatal(err)
	}
	var nearest []bool
	for _, o := range ops {
		if o.K == int(driver.DrawImage) {
			nearest = append(nearest, o.Nearest)
		}
	}
	want := []bool{false, true, true, false}
	if len(nearest) != len(want) {
		t.Fatalf("%d image ops, want %d", len(nearest), len(want))
	}
	for i := range want {
		if nearest[i] != want[i] {
			t.Fatalf("image ops' nearest sampling = %v, want %v", nearest, want)
		}
	}
}

// TestImageSendsItsSampling checks that an image view's operation ends with whether it samples nearest, which Index.ets reads as o[5].
func TestImageSendsItsSampling(t *testing.T) {
	for _, sampling := range []driver.ImageSampling{driver.ImageSamplingSmooth, driver.ImageSamplingNearest} {
		m := &mounter{views: map[driver.NodeID]*view{}}
		m.item(&driver.MountBatch{}, &driver.MountItem{Kind: driver.ItemImage, ID: 4, Image: &driver.ImageConfig{Source: driver.ImageSource{URI: "a.png"}, Sampling: sampling}})
		var got [][]any
		if err := json.Unmarshal(m.o.bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || len(got[0]) != 6 || got[0][0] != "im" {
			t.Fatalf("image operations %v, want one im with 5 arguments", got)
		}
		if want := sampling == driver.ImageSamplingNearest; got[0][5] != want {
			t.Fatalf("sampling %d sent nearest %v, want %v", sampling, got[0][5], want)
		}
	}
}

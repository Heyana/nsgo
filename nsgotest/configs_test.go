package nsgotest

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestMediaRecordsKeepSourcesAndSnapshots(t *testing.T) {
	source := core.ImageSource{Data: []byte{1, 2}, Variants: map[float32]string{2: "double"}}
	placeholder := core.ImageSource{URI: "placeholder", Variants: map[float32]string{2: "placeholder-double"}}
	image := imageConfig(driver.ImageConfig{Source: source, Placeholder: placeholder})
	if !image.Source.Same(source) || !image.Placeholder.Same(placeholder) {
		t.Fatal("recording the image changed its source identity")
	}
	source.Variants[2] = "changed"
	placeholder.Variants[2] = "changed"
	if image.Source.Variants[2] != "double" || image.Placeholder.Variants[2] != "placeholder-double" {
		t.Fatal("the image record shares its source variant maps")
	}
	changed := image
	changed.Sampling = core.ImageSamplingNearest
	if image.Same(changed) {
		t.Fatal("image comparison ignored the sampling change")
	}

	headers := driver.NewVideoHeaders(map[string]string{"X-Trace": "fixture"})
	video := videoConfig(driver.VideoConfig{Source: "movie", AudioSource: "sound", Headers: headers})
	if video.Source != "movie" || video.AudioSource != "sound" || video.Headers != headers {
		t.Fatal("the video record lost its source or immutable headers")
	}
}

func TestInputRecordKeepsItsEditingPolicies(t *testing.T) {
	selection := core.RGB(0x456789)
	input := inputConfig(driver.InputConfig{Multiline: true, SelectionColor: selection})
	if input.Submit() != core.SubmitBehaviorNewline || input.Highlight() != selection.Faded(0.3) || input.Caret() != selection {
		t.Fatal("the input record lost its return or selection policy")
	}
	input.Multiline = false
	input.CaretColor = core.RGB(0xabcdef)
	if input.Submit() != core.SubmitBehaviorBlurAndSubmit || input.Caret() != input.CaretColor {
		t.Fatal("the input policy ignored the recorded fields")
	}
}

func TestScrollRecordOwnsItsListsAndKeepsZoomPolicy(t *testing.T) {
	offsets := []core.Points{10, 20}
	bars := []core.NodeID{2, 3}
	scroll := scrollConfig(driver.ScrollConfig{
		Behavior:     core.ScrollBehavior{SnapToOffsets: offsets},
		MinZoomScale: 1,
		MaxZoomScale: 3,
		Edges:        [4]driver.ScrollEdge{{Bars: bars}},
	})
	offsets[0], bars[0] = 99, 99
	if scroll.Behavior.SnapToOffsets[0] != 10 || scroll.Edges[0].Bars[0] != 2 {
		t.Fatal("the scroll record shares mutable lists")
	}
	if !scroll.Zooms() || scroll.ClampZoom(0.5) != 1 || scroll.ClampZoom(4) != 3 {
		t.Fatal("the scroll record lost its zoom limits")
	}
	other := scroll
	if !scroll.Equal(&other) {
		t.Fatal("equivalent scroll records compare differently")
	}
	other.NoPinchZoom = true
	if scroll.Equal(&other) {
		t.Fatal("scroll comparison ignored the pinch policy")
	}
}

func TestStickyRecordsOwnTheirCells(t *testing.T) {
	cells := []driver.StickyCell{{Cell: 4, Start: 10, End: 30, Node: 3, Inset: 2}}
	records := stickyCells(cells)
	cells[0].Start = 50
	if records[0].StickyShift(25) != 15 || records[0].Node != 3 || records[0].Inset != 2 {
		t.Fatal("the sticky record lost its range or animation binding")
	}
	if stickyCells(nil) != nil || stickyCells([]driver.StickyCell{}) == nil {
		t.Fatal("recording sticky cells changed an empty slice's presence")
	}
}

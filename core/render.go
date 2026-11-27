package core

import (
	"errors"
	"image"
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// RenderDrawing plays d into an image of size points at scale pixels per point,
// off the screen, and calls done with it on the engine's goroutine.
// Env.CanRenderDrawings says whether the platform can; elsewhere done receives
// an error with CodeUnavailable without asking the platform.
//
// Size and scale must be finite and positive, with at most 16 Mi output pixels.
// The drawing must have resolved its view paints and animation slots; image
// access rules of cx's owner apply to any images d draws.
func (cx *Scope) RenderDrawing(d *Drawing, size Size, scale float64, done func(image.Image, error)) {
	t := cx.tree()
	if t == nil || done == nil {
		return
	}
	owner := cx.current()
	if !cx.Env().canRenderDrawings.GetUntracked() {
		owner.Post(func() { done(nil, Unavailable(driver.NoDrawingRenderer)) })
		return
	}
	if _, _, err := driver.RenderPixelSize(d, size, scale); err != nil {
		owner.Post(func() { done(nil, err) })
		return
	}
	snapshot := *d
	snapshot.Ops = slices.Clone(d.Ops)
	snapshot.Images = slices.Clone(d.Images)
	t.pickDrawingScales(&snapshot)
	if t.rt.restricted {
		t.refuseDrawingImagesOwner(owner, &snapshot)
	}
	prepared, _ := copyDrawingFrames(&snapshot, nil)
	t.enableCanvas()
	if prepared.Shaded() {
		t.enableCanvasShaders()
	}
	t.request(cx, driver.MountItem{
		Kind: driver.ItemRenderDrawing,
		RenderDrawing: &driver.RenderDrawingRequest{
			Drawing: prepared,
			Size:    size,
			Scale:   scale,
		},
	}, func(ev driver.Event) {
		switch ev.Text {
		case driver.NoDrawingRenderer:
			done(nil, Unavailable(ev.Text))
		case "":
			img, _ := ev.Data.(image.Image)
			if img == nil {
				done(nil, errors.New("core: render drawing returned no pixels"))
				return
			}
			done(img, nil)
		default:
			done(nil, errors.New(ev.Text))
		}
	})
}

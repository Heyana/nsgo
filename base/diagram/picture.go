package diagram

import (
	"maps"

	"github.com/firede/limu/scene"
	"github.com/firede/limu/style"
	"github.com/nsgo-dev/nsgo/core"
)

// pictureSizes asks the same platform service as Image for fitted pictures' natural sizes. A new scene drops old results; late replies cannot repaint its replacement.
func pictureSizes(cx *core.Scope, current func() *scene.Scene) func() map[string]core.Size {
	sizes := core.NewSignalFunc(cx, map[string]core.Size{}, maps.Equal)
	generation := 0
	cx.Effect(func() {
		sc := current()
		generation++
		this := generation
		sizes.Set(map[string]core.Size{})
		if sc == nil {
			return
		}
		requested := map[string]bool{}
		for _, it := range sc.Items {
			pic := it.Picture
			if pic == nil || pic.Image == "" || !pic.Fit || requested[pic.Image] {
				continue
			}
			uri := pic.Image
			requested[uri] = true
			cx.ImageSize(core.ImageSource{URI: uri}, func(size core.Size, err error) {
				if generation != this || err != nil || size.Width <= 0 || size.Height <= 0 {
					return
				}
				next := maps.Clone(sizes.GetUntracked())
				next[uri] = size
				sizes.Set(next)
			})
		}
	})
	return sizes.Get
}

func (p painter) icon(name string, box scene.Rect, s style.Resolved) {
	svg := p.icons[name]
	if svg == nil {
		return
	}
	p.d.Save().SetAlpha(float32(s.Opacity)).SetFillColor(color(s.Text, 1))
	p.d.DrawSVGIn(svg, float32(box.X), float32(box.Y), float32(box.W), float32(box.H)).Restore()
}

// picture draws between an item's outline and label, as limu's scene specifies.
func (p painter) picture(it *scene.Item, s style.Resolved) {
	pic := it.Picture
	if pic == nil || pic.Box.W <= 0 || pic.Box.H <= 0 {
		return
	}
	if pic.Image == "" {
		p.icon(pic.Icon, pic.Box, s)
		return
	}
	box := pic.Box
	if pic.Fit {
		size := p.images[pic.Image]
		if size.Width <= 0 || size.Height <= 0 {
			return
		}
		scale := min(box.W/float64(size.Width), box.H/float64(size.Height))
		w, h := float64(size.Width)*scale, float64(size.Height)*scale
		box.X += (box.W - w) / 2
		box.Y += (box.H - h) / 2
		box.W, box.H = w, h
	}
	p.d.Save().SetAlpha(float32(s.Opacity)).DrawImageIn(core.ImageSource{URI: pic.Image}, float32(box.X), float32(box.Y), float32(box.W), float32(box.H)).Restore()
}

//go:build js && wasm

package web

import (
	"bytes"
	"strings"
	"syscall/js"

	"github.com/oneo-me/nsgo/core"
)

// image is what an image view draws: its source over its placeholder.
// The browser loads and decodes both; an <img> outside the page loads
// each for its events and natural size, and a layer draws it as its
// background, or as the mask of the tint color. An <img> reports no
// progress, so EventProgress is never sent.
type image struct {
	config              core.ImageConfig
	tint                core.Color
	source, placeholder picture
}

// picture is one image of an image view.
type picture struct {
	layer  js.Value
	img    js.Value
	src    core.ImageSource
	url    string
	object bool // url is an object URL the picture revokes
	loaded bool
	pixels core.Size // the natural size, once loaded
}

func (s *surface) createImage(v *view) {
	im := &image{}
	v.image = im
	for _, p := range []*picture{&im.placeholder, &im.source} {
		p.layer = s.element("div")
		setStyle(p.layer, map[string]string{
			"width": "100%", "height": "100%", "border-radius": "inherit", "pointer-events": "none",
			"background-position": "center", "background-repeat": "no-repeat",
			"mask-position": "center", "mask-repeat": "no-repeat",
			"-webkit-mask-position": "center", "-webkit-mask-repeat": "no-repeat",
		})
		v.el.Call("appendChild", p.layer)
		p.img = s.doc.Call("createElement", "img")
	}
	ph, src := &im.placeholder, &im.source
	v.listeners.on(ph.img, "load", func(js.Value) {
		ph.loaded, ph.pixels = true, naturalSize(ph.img)
		im.draw(v.size)
	})
	v.listeners.on(src.img, "load", func(js.Value) {
		src.loaded, src.pixels = true, naturalSize(src.img)
		im.draw(v.size)
		if fade := int(im.config.FadeDuration.Milliseconds()); fade > 0 {
			animate(src.layer, map[string]any{"opacity": 0}, fade, "linear")
			// The placeholder stays under the source until it is opaque.
			ph.layer.Call("animate", []any{
				map[string]any{"visibility": "visible"}, map[string]any{"visibility": "visible"},
			}, fade)
		}
		s.send(v, core.Event{Kind: core.EventLoad, Size: src.pixels})
		s.send(v, core.Event{Kind: core.EventLoadEnd})
	})
	v.listeners.on(src.img, "error", func(js.Value) {
		s.send(v, core.Event{Kind: core.EventError, Text: "web: cannot load the image " + shorten(src.src.URI)})
		s.send(v, core.Event{Kind: core.EventLoadEnd})
	})
}

func naturalSize(img js.Value) core.Size {
	return core.Size{
		Width:  core.Points(img.Get("naturalWidth").Float()),
		Height: core.Points(img.Get("naturalHeight").Float()),
	}
}

// shorten keeps a data: URI out of messages.
func shorten(uri string) string {
	if len(uri) > 80 {
		return uri[:77] + "..."
	}
	return uri
}

func (s *surface) setImage(v *view, c *core.ImageConfig) {
	im := v.image
	if im == nil {
		return
	}
	im.config = *c
	im.placeholder.load(c.Placeholder)
	if im.source.load(c.Source) && im.source.url != "" {
		s.send(v, core.Event{Kind: core.EventLoadStart})
	}
	im.draw(v.size)
}

// load starts loading src unless the picture already has it, and
// reports whether it did.
func (p *picture) load(src core.ImageSource) bool {
	if sameSource(p.src, src) {
		return false
	}
	p.release()
	p.src, p.loaded = src, false
	p.url, p.object = imageURL(src)
	if p.url != "" {
		p.img.Set("src", p.url)
	}
	return true
}

func sameSource(a, b core.ImageSource) bool {
	return a.URI == b.URI && len(a.Data) == len(b.Data) && (len(a.Data) == 0 || &a.Data[0] == &b.Data[0])
}

// imageURL returns where the browser loads src from: an object URL for
// bytes, and "asset:" paths next to the page.
func imageURL(src core.ImageSource) (url string, object bool) {
	if len(src.Data) > 0 {
		g := js.Global()
		data := g.Get("Uint8Array").New(len(src.Data))
		js.CopyBytesToJS(data, src.Data)
		opts := map[string]any{}
		// Browsers sniff the other image types, but draw SVG only when
		// told.
		if head := src.Data[:min(len(src.Data), 512)]; bytes.Contains(head, []byte("<svg")) {
			opts["type"] = "image/svg+xml"
		}
		blob := g.Get("Blob").New([]any{data}, opts)
		return g.Get("URL").Call("createObjectURL", blob).String(), true
	}
	if path, ok := strings.CutPrefix(src.URI, "asset:"); ok {
		return "assets/" + strings.TrimPrefix(path, "/"), false
	}
	return src.URI, false
}

// release stops loading and revokes the object URL the picture made.
func (p *picture) release() {
	// Without a src attribute, the <img> stops and reports nothing.
	p.img.Call("removeAttribute", "src")
	if p.object {
		js.Global().Get("URL").Call("revokeObjectURL", p.url)
	}
	p.url, p.object = "", false
}

func (im *image) release() {
	im.placeholder.release()
	im.source.release()
}

// draw shows the source once loaded, else the placeholder, in a box of
// size.
func (im *image) draw(size core.Size) {
	src, ph := &im.source, &im.placeholder
	setStyle(ph.layer, ph.look(im, size))
	setStyle(src.layer, src.look(im, size))
	opacity, visibility := "0", "visible"
	if src.loaded {
		opacity, visibility = "1", "hidden"
	}
	src.layer.Get("style").Call("setProperty", "opacity", opacity)
	ph.layer.Get("style").Call("setProperty", "visibility", visibility)
}

// look is the CSS of a layer drawing p.
func (p *picture) look(im *image, box core.Size) map[string]string {
	style := map[string]string{
		"background-image": "none", "background-color": "transparent",
		"mask-image": "none", "-webkit-mask-image": "none", "filter": "none",
	}
	if !p.loaded || p.url == "" {
		return style
	}
	url := cssURL(p.url)
	fit := fitSize(im.config.ResizeMode, p.pixels, box)
	if im.tint != 0 {
		style["background-color"] = cssColor(im.tint)
		style["mask-image"], style["-webkit-mask-image"] = url, url
		style["mask-size"], style["-webkit-mask-size"] = fit, fit
	} else {
		style["background-image"] = url
		style["background-size"] = fit
	}
	if b := im.config.BlurRadius; b > 0 {
		style["filter"] = "blur(" + px(b) + ")"
	}
	return style
}

// fitSize is the CSS background or mask size that fits an image of
// pixels into box by mode.
func fitSize(mode core.ResizeMode, pixels, box core.Size) string {
	switch mode {
	case core.Contain:
		return "contain"
	case core.Stretch:
		return "100% 100%"
	case core.Center:
		if pixels.Width > box.Width || pixels.Height > box.Height {
			return "contain"
		}
		return "auto"
	}
	return "cover"
}

var cssEscape = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\a `)

func cssURL(url string) string { return `url("` + cssEscape.Replace(url) + `")` }

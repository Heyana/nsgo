//go:build js && wasm

package web

import (
	"bytes"
	"strconv"
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
// bytes, and "asset:" paths in the resources beside the page.
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
		return "resources/" + strings.TrimPrefix(path, "/"), false
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

// look is the CSS of a layer drawing p: at the rectangle core's fit gives
// its natural size in box, by the resize mode and position.
func (p *picture) look(im *image, box core.Size) map[string]string {
	style := map[string]string{
		"background-image": "none", "background-color": "transparent",
		"mask-image": "none", "-webkit-mask-image": "none", "filter": "none",
	}
	if !p.loaded || p.url == "" {
		return style
	}
	url := cssURL(p.url)
	b := im.config.ResizeMode.Fit(p.natural(), box, im.config.Position)
	size := px(b.Size.Width) + " " + px(b.Size.Height)
	at := px(b.Origin.X) + " " + px(b.Origin.Y)
	if im.tint != 0 {
		style["background-color"] = cssColor(im.tint)
		style["mask-image"], style["-webkit-mask-image"] = url, url
		style["mask-size"], style["-webkit-mask-size"] = size, size
		style["mask-position"], style["-webkit-mask-position"] = at, at
	} else {
		style["background-image"] = url
		style["background-size"] = size
		style["background-position"] = at
	}
	if b := im.config.BlurRadius; b > 0 {
		style["filter"] = "blur(" + px(b) + ")"
	}
	return style
}

// natural returns the size in points p shows at: the size its source
// declares, else the browser's natural size over the density a name's
// "@2x" declares (an SVG's is its own).
func (p *picture) natural() core.Size {
	if s := p.src.Size; s.Width > 0 && s.Height > 0 {
		return s
	}
	d := core.Points(density(p.src.URI))
	return core.Size{Width: p.pixels.Width / d, Height: p.pixels.Height / d}
}

// imageInfo answers an ItemImageInfo with an <img> outside the page: the
// browser loads and decodes the source, keeping it in its caches for the
// views that show it next, and reports its natural size.
func (s *surface) imageInfo(it *core.MountItem) {
	id := it.Count
	var src core.ImageSource
	if it.Image != nil {
		src = it.Image.Source
	}
	url, object := imageURL(src)
	answer := func(ev core.Event) {
		if object {
			js.Global().Get("URL").Call("revokeObjectURL", url)
		}
		ev.Kind, ev.Count = core.EventImageInfo, id
		s.engine.Dispatch(ev)
	}
	if url == "" {
		answer(core.Event{Text: "web: empty image source"})
		return
	}
	img := s.doc.Call("createElement", "img")
	img.Set("src", url)
	then(img.Call("decode"), func(js.Value) {
		answer(core.Event{Size: naturalSize(img)})
	}, func(js.Value) {
		answer(core.Event{Text: "web: cannot load the image " + shorten(src.URI)})
	})
}

// density returns the N of a bitmap's "name@Nx.png" URI, else 1.
func density(uri string) float64 {
	name := uri[strings.LastIndexAny(uri, "/\\:")+1:]
	if strings.HasSuffix(strings.ToLower(name), ".svg") {
		return 1
	}
	at := strings.LastIndexByte(name, '@')
	if at < 0 {
		return 1
	}
	n, _, ok := strings.Cut(name[at+1:], "x")
	f, err := strconv.ParseFloat(n, 64)
	if !ok || err != nil || f <= 0 {
		return 1
	}
	return f
}

var cssEscape = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\a `)

func cssURL(url string) string { return `url("` + cssEscape.Replace(url) + `")` }

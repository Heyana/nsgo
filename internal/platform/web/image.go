//go:build js && wasm

package web

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
)

// image is what an image view draws: its source over its placeholder.
// Encoded sources load in an <img> outside the page and draw as their layer's
// background or tint mask. Pixel sources draw in a canvas inside that layer.
// An <img> reports no progress, so EventProgress is never sent.
type image struct {
	config              driver.ImageConfig
	tint                driver.Color
	source, placeholder picture
}

// picture is one image of an image view.
type picture struct {
	layer     js.Value
	img       js.Value
	css       *cssBox
	bitmap    *pixelCanvas
	bitmapCSS *cssBox
	src       driver.ImageSource
	url       string
	object    bool // url is an object URL the picture revokes
	loaded    bool
	pixels    driver.Size // the natural size, once loaded
}

func (s *surface) createImage(v *view) {
	im := &image{}
	v.image = im
	for _, p := range []*picture{&im.placeholder, &im.source} {
		p.layer = s.element("div")
		p.css = newCSSBox(p.layer)
		p.css.set(map[string]string{
			"width": "100%", "height": "100%", "border-radius": "inherit", "pointer-events": "none", "overflow": "hidden",
			"background-position": "center", "background-repeat": "no-repeat",
			"mask-position": "center", "mask-repeat": "no-repeat",
			"-webkit-mask-position": "center", "-webkit-mask-repeat": "no-repeat",
		})
		v.el.Call("appendChild", p.layer)
		p.img = s.doc.Call("createElement", "img")
	}
	ph, src := &im.placeholder, &im.source
	v.listeners.on(ph.img, "load", func(js.Value) {
		if ph.url == "" {
			return
		}
		ph.loaded, ph.pixels = true, naturalSize(ph.img)
		im.draw(v.size)
	})
	v.listeners.on(src.img, "load", func(js.Value) {
		if src.url == "" {
			return
		}
		src.loaded, src.pixels = true, naturalSize(src.img)
		im.draw(v.size)
		if fade := int(im.config.FadeDuration.Milliseconds()); fade > 0 && src.src.Offscreen == nil {
			animate(src.layer, map[string]any{"opacity": 0}, fade, "linear")
			// The placeholder stays under the source until it is opaque.
			ph.layer.Call("animate", []any{
				map[string]any{"visibility": "visible"}, map[string]any{"visibility": "visible"},
			}, fade)
		}
		s.send(v, driver.Event{Kind: driver.EventLoad, Size: src.pixels})
		s.send(v, driver.Event{Kind: driver.EventLoadEnd})
	})
	v.listeners.on(src.img, "error", func(js.Value) {
		if src.url == "" {
			return
		}
		s.send(v, driver.Event{Kind: driver.EventError, Text: "web: cannot load the image " + shorten(src.src.URI)})
		s.send(v, driver.Event{Kind: driver.EventLoadEnd})
	})
}

func naturalSize(img js.Value) driver.Size {
	return driver.Size{
		Width:  driver.Points(img.Get("naturalWidth").Float()),
		Height: driver.Points(img.Get("naturalHeight").Float()),
	}
}

// shorten keeps a data: URI out of messages.
func shorten(uri string) string {
	if len(uri) > 80 {
		return uri[:77] + "..."
	}
	return uri
}

func (s *surface) setImage(v *view, c *driver.ImageConfig) {
	im := v.image
	if im == nil {
		return
	}
	if c.Source.Pixels != nil && im.source.src.Pixels == nil {
		// A frame replaces an encoded image immediately, including an old fade.
		for _, p := range []*picture{&im.source, &im.placeholder} {
			animations := p.layer.Call("getAnimations")
			for i := range animations.Length() {
				animations.Index(i).Call("cancel")
			}
		}
	}
	im.config = *c
	im.placeholder.load(s, v, c.Placeholder)
	if im.source.load(s, v, c.Source) && c.Source.Offscreen == nil {
		s.send(v, driver.Event{Kind: driver.EventLoadStart})
	}
	im.draw(v.size)
	if c.Source.Pixels != nil {
		s.requestGlassPaint()
	}
}

// load installs a pixel frame, renders an offscreen drawing, or starts loading an encoded source.
// It reports whether the browser began a load.
func (p *picture) load(s *surface, v *view, src driver.ImageSource) bool {
	if sameSource(p.src, src) {
		return false
	}
	p.src, p.loaded = src, src.Pixels != nil
	if src.Pixels != nil {
		b := src.Pixels.Rect
		p.pixels = driver.Size{Width: driver.Points(b.Dx()), Height: driver.Points(b.Dy())}
		if p.bitmap == nil {
			p.release()
			el := p.layer.Get("ownerDocument").Call("createElement", "canvas")
			el.Set("className", "nsgo-el")
			el.Call("setAttribute", "aria-hidden", "true")
			p.layer.Call("appendChild", el)
			p.bitmap = newPixelCanvas(el)
			p.bitmapCSS = newCSSBox(el)
		}
		return false
	}
	if p.bitmap != nil {
		p.bitmap.el.Call("remove")
		p.bitmap, p.bitmapCSS = nil, nil
	}
	p.release()
	if src.Offscreen != nil {
		if canvas, _, ok := s.offscreenCanvas(src.Offscreen, v); ok && canvas.Truthy() {
			p.url, p.object = canvas.Call("toDataURL", "image/png").String(), false
		}
	} else {
		p.url, p.object = imageURL(src)
	}
	if p.url == "" {
		return false
	}
	p.img.Set("src", p.url)
	return true
}

func sameSource(a, b driver.ImageSource) bool {
	return a.URI == b.URI && len(a.Data) == len(b.Data) && (len(a.Data) == 0 || &a.Data[0] == &b.Data[0]) &&
		a.Pixels == b.Pixels && a.Offscreen.Same(b.Offscreen) && a.Version == b.Version
}

// imageURL returns where the browser loads encoded src from: an object URL
// for bytes and "asset:" paths beside the page.
func imageURL(src driver.ImageSource) (url string, object bool) {
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
		return resourceURL(path), false
	}
	return src.URI, false
}

// resourceURL returns where the browser fetches the app's resource path:
// the page's resources, which a published site names by their content
// (globalThis.nsgoResources maps each path to its name) and a dev build
// serves under the paths themselves.
func resourceURL(path string) string {
	path = strings.TrimPrefix(path, "/")
	if name, ok := resourceNames[path]; ok {
		return resourceBase + name
	}
	return resourceBase + path
}

// resourceBase is where the app's files are from the page, and
// resourceNames the names a published site gave its resources. Each app
// on a page is a Go program of its own, so they are its: the page sets
// them in its environment (NSGO_BASE, NSGO_RESOURCES), or a page of the
// app's own in globalThis.nsgoResources.
var (
	resourceBase  string
	resourceNames map[string]string
)

// loadResourceNames reads where the app's files are.
func loadResourceNames() {
	resourceBase = os.Getenv("NSGO_BASE")
	if s := os.Getenv("NSGO_RESOURCES"); s != "" {
		json.Unmarshal([]byte(s), &resourceNames)
		return
	}
	if names := js.Global().Get("nsgoResources"); names.Type() == js.TypeObject {
		s := js.Global().Get("JSON").Call("stringify", names).String()
		json.Unmarshal([]byte(s), &resourceNames)
	}
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
func (im *image) draw(size driver.Size) {
	src, ph := &im.source, &im.placeholder
	ph.draw(im, size)
	src.draw(im, size)
	opacity, visibility := "0", "visible"
	if src.loaded {
		opacity, visibility = "1", "hidden"
	}
	src.css.set1("opacity", opacity)
	ph.css.set1("visibility", visibility)
}

// draw draws p at the rectangle core's fit gives its natural size in box.
func (p *picture) draw(im *image, box driver.Size) {
	style := map[string]string{
		"background-image": "none", "background-color": "transparent",
		"mask-image": "none", "-webkit-mask-image": "none", "filter": "none",
		"image-rendering": "auto",
	}
	if p.loaded {
		b := im.config.ResizeMode.Fit(p.natural(), box, im.config.Position)
		w, h := webstyle.Px(b.Size.Width), webstyle.Px(b.Size.Height)
		x, y := webstyle.Px(b.Origin.X), webstyle.Px(b.Origin.Y)
		switch {
		case p.bitmap != nil:
			p.bitmap.put(p.src.Pixels, im.tint)
			p.bitmapCSS.set1("width", w)
			p.bitmapCSS.set1("height", h)
			p.bitmapCSS.set1("left", x)
			p.bitmapCSS.set1("top", y)
		case im.tint != 0:
			url := cssURL(p.url)
			style["background-color"] = webstyle.Color(im.tint)
			style["mask-image"], style["-webkit-mask-image"] = url, url
			style["mask-size"], style["-webkit-mask-size"] = w+" "+h, w+" "+h
			style["mask-position"], style["-webkit-mask-position"] = x+" "+y, x+" "+y
		default:
			style["background-image"] = cssURL(p.url)
			style["background-size"] = w + " " + h
			style["background-position"] = x + " " + y
		}
		if b := im.config.BlurRadius; b > 0 {
			style["filter"] = "blur(" + webstyle.Px(b) + ")"
		}
		// image-rendering is inherited, so it also reaches a pixel buffer's canvas.
		if im.config.Sampling == driver.ImageSamplingNearest {
			style["image-rendering"] = "pixelated"
			if js.Global().Get("CSS").Call("supports", "image-rendering", "crisp-edges").Bool() {
				style["image-rendering"] = "crisp-edges"
			}
		}
	}
	p.css.set(style)
}

// natural returns the size in points p shows at: the size its source
// declares, else the browser's natural size over the density a name's
// "@2x" declares (an SVG's is its own).
func (p *picture) natural() driver.Size {
	if s := p.src.Size; s.Width > 0 && s.Height > 0 {
		return s
	}
	if p.src.Pixels != nil {
		return p.pixels
	}
	d := driver.Points(density(p.src.URI))
	return driver.Size{Width: p.pixels.Width / d, Height: p.pixels.Height / d}
}

// imageInfo answers an ItemImageInfo with an <img> outside the page: the
// browser loads and decodes the source, keeping it in its caches for the
// views that show it next, and reports its natural size.
func (s *surface) imageInfo(it *driver.MountItem) {
	id := it.Count
	var src driver.ImageSource
	if it.Image != nil {
		src = it.Image.Source
	}
	url, object := imageURL(src)
	answer := func(ev driver.Event) {
		if object {
			js.Global().Get("URL").Call("revokeObjectURL", url)
		}
		ev.Kind, ev.Count = driver.EventImageInfo, id
		s.engine.Dispatch(ev)
	}
	if url == "" {
		answer(driver.Event{Text: "web: empty image source"})
		return
	}
	img := s.doc.Call("createElement", "img")
	img.Set("src", url)
	then(img.Call("decode"), func(js.Value) {
		answer(driver.Event{Size: naturalSize(img)})
	}, func(js.Value) {
		answer(driver.Event{Text: "web: cannot load the image " + shorten(src.URI)})
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

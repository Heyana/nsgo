package core

import (
	"errors"
	"image"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type imageNode struct {
	config driver.ImageConfig
	sent   driver.ImageConfig // as resolved for the pixel ratio
	// refused and refusedPlaceholder are the URIs Access refused last.
	refused, refusedPlaceholder string
	// frame is the copy of the source's pixels the platform was sent last.
	frame pixelFrame
}

// pixelFrame is the copy the core made of an app's pixels at a version, which the platform owns once sent: the app may write its buffer again once the commit that copied it ran.
type pixelFrame struct {
	from    *image.RGBA
	version uint64
	copy    *image.RGBA
}

// of returns the copy of src's pixels to send: the one made last while src names the same buffer at the same version, so a platform keeps its frame when only the look changes, else a new one. A buffer with no pixels sends none, as the view then shows nothing.
func (f *pixelFrame) of(src ImageSource) *image.RGBA {
	if src.Pixels == nil || src.Pixels.Rect.Empty() {
		*f = pixelFrame{}
		return nil
	}
	if f.from != src.Pixels || f.version != src.Version || f.copy == nil {
		*f = pixelFrame{from: src.Pixels, version: src.Version, copy: driver.CopyPixels(src.Pixels)}
	}
	return f.copy
}

func (t *Tree) mountImage(cx *Scope, parent *node, v Image) {
	n := t.newNode(driver.KindImage, parent, cx)
	t.setClasses(n, v.Class)
	t.bindStyle(cx, n, v.Style, v.StyleFunc)
	t.setRef(n, v.Ref)
	n.h = handlers{press: v.OnPress, layout: v.OnLayout, loadStart: v.OnLoadStart, loadEnd: v.OnLoadEnd,
		progress: v.OnProgress, load: v.OnLoad, loadError: v.OnError}
	n.props = driver.ViewProps{Role: RoleImage, Label: v.Label, TestID: v.TestID, ElementsHidden: v.ElementsHidden}
	t.bindAccessibilityText(cx, n, &n.props.Label, v.LabelFunc)
	n.image = &imageNode{config: driver.ImageConfig{Source: v.Source, Placeholder: v.Placeholder,
		ResizeMode: v.ResizeMode, BlurRadius: v.BlurRadius, FadeDuration: v.FadeDuration, HitTestAlpha: v.hitTestAlpha()}}
	if v.SourceFunc != nil {
		cx.renderEffect(func() {
			src := v.SourceFunc()
			if !src.Same(n.image.config.Source) {
				if naturalOf(src) != naturalOf(n.image.config.Source) {
					t.markLayout(n)
				}
				n.image.config.Source = src
				t.mark(n)
			}
		})
	}
}

func (i Image) hitTestAlpha() float32 {
	if i.HitTestAlpha > 0 {
		return min(i.HitTestAlpha, 1)
	}
	return 0
}

// naturalSize is what an image measures without a style size.
func (im *imageNode) naturalSize() Size { return naturalOf(im.config.Source) }

// naturalOf is the size src declares, its offscreen size, or its pixels in points.
func naturalOf(src ImageSource) Size {
	if src.Size == (Size{}) && src.Offscreen != nil {
		return src.Offscreen.Size
	}
	if src.Size == (Size{}) && src.Pixels != nil {
		b := src.Pixels.Rect
		return Size{Width: Points(b.Dx()), Height: Points(b.Dy())}
	}
	return src.Size
}

// Positions along an axis.
var (
	PositionStart  = PositionAxis{Offset: Pct(0)}
	PositionCenter = PositionAxis{Offset: Pct(50)}
	PositionEnd    = PositionAxis{Offset: Pct(100)}
)

// ImageSize calls done with the size in pixels of the image src names,
// as Image's OnLoad reports it, or the error loading or reading it (NSH's
// Image.getSize). The platform loads it as an Image would, the variant of
// an "asset:" source with Scales included, and keeps its bytes for the
// views that show it next. A source with Pixels loads nothing: done is called at once with the buffer's size.
func (cx *Scope) ImageSize(src ImageSource, done func(pixels Size, err error)) {
	t := cx.tree()
	if t == nil {
		return
	}
	if off := src.Offscreen; off != nil {
		w, h := off.PixelSize()
		done(Size{Width: Points(w), Height: Points(h)}, nil)
		return
	}
	if p := src.Pixels; p != nil {
		done(Size{Width: Points(p.Rect.Dx()), Height: Points(p.Rect.Dy())}, nil)
		return
	}
	if len(src.Data) == 0 {
		if err := cx.gate(serviceImage, src.URI); err != nil {
			done(Size{}, err)
			return
		}
	}
	t.alertID++
	if t.imageInfos == nil {
		t.imageInfos = map[uint32]imageInfoWait{}
	}
	t.imageInfos[t.alertID] = imageInfoWait{cx.current(), done}
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemImageInfo, Count: t.alertID,
		Image: &driver.ImageConfig{Source: t.pickScale(src)}})
	t.changed = true
}

// PrefetchImage loads the image src names ahead of showing it, keeping
// its bytes for the views that show it next (NSH's Image.prefetch), and
// calls done, when not nil, with whether it loaded.
func (cx *Scope) PrefetchImage(src ImageSource, done func(ok bool)) {
	cx.ImageSize(src, func(_ Size, err error) {
		if done != nil {
			done(err == nil)
		}
	})
}

// KeepImage pins src in the platform's canvas image cache so it is not evicted
// while kept, until release is called or the current scope is disposed.
// Keeping the same source more than once requires each release before it may
// be evicted again. Releasing more than once is safe. Empty and Pixels sources
// are not cached by key and return a no-op release.
func (cx *Scope) KeepImage(src ImageSource) (release func()) {
	owner := cx.current()
	t := owner.tree()
	if t == nil || owner.disposed || src.Pixels != nil || src.Empty() {
		return func() {}
	}
	if len(src.Data) == 0 {
		if err := cx.gate(serviceImage, src.URI); err != nil {
			return func() {}
		}
	}
	key := t.pickScale(src).Key()
	if key == "" {
		return func() {}
	}
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemKeepImage, Value: key})
	t.changed = true
	var done atomic.Bool
	releaseNow := func() {
		if t := owner.tree(); t != nil {
			t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemReleaseImage, Value: key})
			t.changed = true
		}
	}
	// The posted release marks the pin done only as it runs: Post drops it when the owner is disposed first, and the cleanup then releases the pin instead.
	release = func() {
		owner.Post(func() {
			if done.CompareAndSwap(false, true) {
				releaseNow()
			}
		})
	}
	owner.addCleanup(func() {
		if done.CompareAndSwap(false, true) {
			releaseNow()
		}
	})
	return release
}

type imageInfoWait struct {
	owner *Scope
	done  func(Size, error)
}

func (t *Tree) answerImageInfo(ev driver.Event) {
	w, ok := t.imageInfos[ev.Count]
	if !ok {
		return
	}
	delete(t.imageInfos, ev.Count)
	owner := w.owner
	if owner.disposed {
		owner = t.root
	}
	var err error
	if ev.Text != "" {
		err = errors.New(ev.Text)
	}
	if w.done != nil {
		t.within(owner, func() { w.done(ev.Size, err) })
	}
}

// ScalesOf returns the set of scales, leaving out any that is not one of
// ImageScaleValues.
func ScalesOf(scales ...float32) ImageScales {
	var set ImageScales
	for _, s := range scales {
		if i := slices.Index(driver.ImageScaleValues[:], s); i >= 0 {
			set |= 1 << i
		}
	}
	return set
}

// pickScale returns s as the platform loads it at the screen's pixel
// ratio: an "asset:" image with scales names the variant they pick
// (asset:/a.png at 2 is asset:/a@2x.png); without scales, for the 1x pick
// or with a query or fragment, which no file name has, the URI stays.
func (t *Tree) pickScale(s ImageSource) ImageSource {
	scales := s.Scales
	s.Scales = 0
	if scales == 0 || s.Variants == nil && (!strings.HasPrefix(s.URI, "asset:") || strings.ContainsAny(s.URI, "?#")) {
		return s
	}
	pick := scales.Pick(t.root.Env().ratio.GetUntracked())
	if pick == 1 {
		return s
	}
	if uri, ok := s.Variants[pick]; ok {
		s.URI, s.Variants = uri, nil
		return s
	}
	name := strings.LastIndexByte(s.URI, '/') + 1
	if name == 0 {
		name = len("asset:")
	}
	dot := strings.LastIndexByte(s.URI[name:], '.')
	if dot > 0 {
		dot += name
	} else {
		dot = len(s.URI)
	}
	s.URI = s.URI[:dot] + "@" + strconv.FormatFloat(float64(pick), 'g', -1, 32) + "x" + s.URI[dot:]
	return s
}

// resolveMode is m, or def for the zero value.
func resolveMode(m, def ResizeMode) ResizeMode {
	if m == 0 {
		return def
	}
	return m
}

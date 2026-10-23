package core

import (
	"errors"
	"slices"
	"strconv"
	"strings"
)

type imageNode struct {
	config ImageConfig
	sent   ImageConfig // as resolved for the pixel ratio
}

func (t *Tree) mountImage(cx *Scope, parent *node, v Image) {
	n := t.newNode(KindImage, parent, cx)
	t.bindStyle(cx, n, v.Style, v.StyleFunc)
	t.setRef(n, v.Ref)
	n.h = handlers{press: v.OnPress, layout: v.OnLayout, loadStart: v.OnLoadStart, loadEnd: v.OnLoadEnd,
		progress: v.OnProgress, load: v.OnLoad, loadError: v.OnError}
	n.props = ViewProps{Role: RoleImage, Label: v.Label, TestID: v.TestID}
	n.image = &imageNode{config: ImageConfig{Source: v.Source, Placeholder: v.Placeholder,
		ResizeMode: resolveMode(v.ResizeMode, Cover), BlurRadius: v.BlurRadius, FadeDuration: v.FadeDuration}}
	if v.SourceFunc != nil {
		cx.renderEffect(func() {
			src := v.SourceFunc()
			if !src.same(n.image.config.Source) {
				if src.Size != n.image.config.Source.Size {
					n.markLayout()
				}
				n.image.config.Source = src
				t.changed = true
			}
		})
	}
}

// naturalSize is what an image measures without a style size.
func (im *imageNode) naturalSize() Size { return im.config.Source.Size }

func (a ImageConfig) same(b ImageConfig) bool {
	return a.Source.same(b.Source) && a.Placeholder.same(b.Placeholder) && a.ResizeMode == b.ResizeMode &&
		a.BlurRadius == b.BlurRadius && a.FadeDuration == b.FadeDuration && a.Position == b.Position
}

// ObjectPosition is where an image sits in its view once its ResizeMode
// has sized it, as CSS's object-position places it. The parts of a
// covering image that overflow are cropped. The zero value centers it.
type ObjectPosition struct {
	X, Y PositionAxis
}

// PositionAxis is an ObjectPosition along one axis: Offset from the start
// edge (left, top), or from the end edge (right, bottom) when FromEnd.
// A percentage is of the free space, the view's size less the image's, so
// Pct(0) is the start, Pct(50) the center and Pct(100) the end; an auto
// Offset, the zero value, is the center.
type PositionAxis struct {
	Offset  Length
	FromEnd bool
}

// Positions along an axis.
var (
	PositionStart  = PositionAxis{Offset: Pct(0)}
	PositionCenter = PositionAxis{Offset: Pct(50)}
	PositionEnd    = PositionAxis{Offset: Pct(100)}
)

// Resolve returns where content starts along the axis, from the start
// edge, given the free space: the box's size less the content's.
func (a PositionAxis) Resolve(free Points) Points {
	var d Points
	switch a.Offset.unit {
	case unitPoints:
		d = Points(a.Offset.value)
	case unitPercent:
		d = free * Points(a.Offset.value) / 100
	default:
		d = free / 2
	}
	if a.FromEnd {
		return free - d
	}
	return d
}

// Place returns the top left corner of content of the given size placed
// in a box of size.
func (p ObjectPosition) Place(box, content Size) Point {
	return Point{X: p.X.Resolve(box.Width - content.Width), Y: p.Y.Resolve(box.Height - content.Height)}
}

// Fit returns where an image of natural size, in points, goes in a view
// of size box: scaled as m scales it (Center keeps the natural size, as
// CSS's none; ScaleDown shrinks an image larger than the box, as CSS's
// scale-down), then placed at pos. The rectangle may reach outside the
// box, which crops it. An image without a size fills the box. Every
// backend fits images and videos with it, as NSH's ResizeMode::fit.
func (m ResizeMode) Fit(natural, box Size, pos ObjectPosition) Bounds {
	nw, nh := natural.Width, natural.Height
	if nw <= 0 || nh <= 0 || m == Stretch {
		return Bounds{Size: box}
	}
	contain := min(box.Width/nw, box.Height/nh)
	var scale Points
	switch m {
	case Contain:
		scale = contain
	case Center:
		scale = 1
	case ScaleDown:
		scale = min(contain, 1)
	default: // Cover
		scale = max(box.Width/nw, box.Height/nh)
	}
	content := Size{Width: nw * scale, Height: nh * scale}
	return Bounds{Origin: pos.Place(box, content), Size: content}
}

// ImageSize calls done with the size in pixels of the image src names,
// as Image's OnLoad reports it, or the error loading or reading it (NSH's
// Image.getSize). The platform loads it as an Image would, the variant of
// an "asset:" source with Scales included, and keeps its bytes for the
// views that show it next.
func (cx *Scope) ImageSize(src ImageSource, done func(pixels Size, err error)) {
	t := cx.rt.tree
	if t == nil {
		return
	}
	t.alertID++
	if t.imageInfos == nil {
		t.imageInfos = map[uint32]imageInfoWait{}
	}
	t.imageInfos[t.alertID] = imageInfoWait{cx.current(), done}
	t.commands = append(t.commands, MountItem{Kind: ItemImageInfo, Count: t.alertID,
		Image: &ImageConfig{Source: t.pickScale(src)}})
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

type imageInfoWait struct {
	owner *Scope
	done  func(Size, error)
}

func (t *Tree) answerImageInfo(ev Event) {
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

// ImageScaleValues are the pixel densities an image asset can ship in, as
// its file name's suffix says ("@1.5x" to "@4x"; the file without one is
// 1x).
var ImageScaleValues = [...]float32{1, 1.5, 2, 3, 4}

// ImageScales is a set of ImageScaleValues: the variants an image asset
// ships in.
type ImageScales uint8

// ScalesOf returns the set of scales, leaving out any that is not one of
// ImageScaleValues.
func ScalesOf(scales ...float32) ImageScales {
	var set ImageScales
	for _, s := range scales {
		if i := slices.Index(ImageScaleValues[:], s); i >= 0 {
			set |= 1 << i
		}
	}
	return set
}

// pick returns the scale to load at ratio, as React Native picks: the
// smallest at least the ratio, else the largest; 0 for an empty set.
func (set ImageScales) pick(ratio float32) float32 {
	var pick float32
	for i, s := range ImageScaleValues {
		if set&(1<<i) != 0 {
			pick = s
			if s >= ratio {
				break
			}
		}
	}
	return pick
}

// pickScale returns s as the platform loads it at the screen's pixel
// ratio: an "asset:" image with scales names the variant they pick
// (asset:/a.png at 2 is asset:/a@2x.png); without scales, for the 1x pick
// or with a query or fragment, which no file name has, the URI stays.
func (t *Tree) pickScale(s ImageSource) ImageSource {
	scales := s.Scales
	s.Scales = 0
	if scales == 0 || !strings.HasPrefix(s.URI, "asset:") || strings.ContainsAny(s.URI, "?#") {
		return s
	}
	pick := scales.pick(t.root.Env().ratio.GetUntracked())
	if pick == 1 {
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

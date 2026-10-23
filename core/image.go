package core

import (
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
		a.BlurRadius == b.BlurRadius && a.FadeDuration == b.FadeDuration
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

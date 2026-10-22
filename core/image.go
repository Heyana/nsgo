package core

import (
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
		ResizeMode: v.ResizeMode, BlurRadius: v.BlurRadius, FadeDuration: v.FadeDuration}}
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

// scales are the variants an "asset:" image may have, as @2x files.
var scales = [...]float32{1, 1.5, 2, 3, 4}

// pickScale picks the variant of an "asset:" image for the pixel ratio:
// the smallest scale at least the ratio, else the largest; a.png becomes
// a@2x.png.
func (t *Tree) pickScale(s ImageSource) ImageSource {
	if !strings.HasPrefix(s.URI, "asset:") || strings.ContainsAny(s.URI, "?#") {
		return s
	}
	ratio := t.root.Env().ratio.GetUntracked()
	pick := scales[len(scales)-1]
	for _, sc := range scales {
		if sc >= ratio {
			pick = sc
			break
		}
	}
	if pick == 1 {
		return s
	}
	dot := strings.LastIndexByte(s.URI, '.')
	if dot < 0 || strings.Contains(s.URI[dot:], "/") {
		dot = len(s.URI)
	}
	s.URI = s.URI[:dot] + "@" + strconv.FormatFloat(float64(pick), 'g', -1, 32) + "x" + s.URI[dot:]
	return s
}

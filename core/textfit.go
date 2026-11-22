package core

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/layout"
)

func (a *attachNode) shownSize() Size {
	if a.fitScale > 0 {
		return a.fitSize
	}
	return a.size
}

// fitAttachmentTexts finishes font-relative attachment geometry before canvases record their drawings. The parent's prepared text and layout caches keep the natural sizes; a wider box can therefore restore the original font and paths.
func (t *Tree) fitAttachmentTexts(n *node) {
	if !t.fullRound && !n.dirty && !n.dirtyKids {
		return
	}
	if tn := n.text; tn != nil && tn.attachments == 0 && len(tn.fitTrees) > 0 {
		tn.fitTrees, tn.fitText, tn.fitStale = nil, driver.AttributedText{}, false
	}
	if tn := n.text; tn != nil && tn.fit && tn.attachments > 0 && tn.prepared != nil {
		if tn.fitStale || tn.resent || tn.box != tn.sentBox || tn.pending != nil {
			tn.fitStale = false
			var trees []*fittedAttachmentNode
			measure := func(scale float32) []driver.TextAttachment {
				trees = nil
				at := append([]driver.TextAttachment(nil), tn.text.Attachments...)
				for _, span := range tn.spans {
					if span.attach == nil {
						continue
					}
					f := newFittedAttachmentNode(span.attach, scale)
					layout.Compute(f, layout.Size[layout.AvailableSpace]{Width: layout.Definite(attachmentSpace), Height: layout.Definite(attachmentSpace)})
					layout.Round(f, t.ratio)
					i := len(trees)
					trees = append(trees, f)
					at[i].Size = f.size()
					at[i].Descent = f.descent(at[i].Align)
				}
				return at
			}
			text, prepared := &tn.text, tn.prepared
			fitted, ok := fitText(t.text, text, tn.box.Size, tn.minScale, measure)
			if ok {
				text = &fitted
			} else {
				measure(1)
			}
			changed := !text.Equal(&tn.fitText) || tn.resent || tn.pending != nil || tn.box != tn.sentBox
			tn.fitText = *text
			tn.fitTrees = trees
			if changed {
				tn.dropTextLayout()
				if tn.fitted != nil {
					tn.fitted.Release()
					tn.fitted = nil
				}
				if ok {
					tn.fitted = t.text.Prepare(text)
					prepared = tn.fitted
				}
				box := tn.box
				if fraction := tn.alignV.fraction(); fraction > 0 {
					if height := tn.size(box.Size.Width).Height; height < box.Size.Height {
						box.Origin.Y += fraction * (box.Size.Height - height)
						box.Size.Height = height
					}
				}
				if tn.noWrap {
					box.Size.Width = max(box.Size.Width, ceil(prepared.Measure(Auto).Width, t.ratio))
				}
				tn.pending = makeTextContent(text, prepared, box)
			}
		}
		for _, f := range tn.fitTrees {
			f.n.attach.fitSize, f.n.attach.fitScale = f.size(), f.scale
			f.apply(t, f.n.frame.Origin)
		}
		if tn.pending != nil {
			tn.shownBox = tn.pending.Content
			t.placeAttachments(n, tn.pending.Layout)
		} else if tn.layout != nil {
			// A replacement attachment may have the same placeholder geometry. Its new view still needs the retained text layout's position.
			t.placeAttachments(n, tn.layout)
		}
	}
	for _, child := range n.nativeChildren() {
		t.fitAttachmentTexts(child)
	}
}

// fittedAttachmentNode is a candidate layout with its own state. Computing it never rewrites the tree's natural layout or an Em canvas's resolved font. Point-sized views keep their dimensions; em-sized illustrations follow the font.
type fittedAttachmentNode struct {
	n        *node
	scale    float32
	style    layout.Style
	state    layout.State
	children []*fittedAttachmentNode
	parent   *fittedAttachmentNode
}

func newFittedAttachmentNode(n *node, scale float32) *fittedAttachmentNode {
	f := &fittedAttachmentNode{n: n, scale: scale, style: n.layoutStyle()}
	for _, child := range n.flatChildren() {
		c := newFittedAttachmentNode(child, scale)
		c.parent = f
		f.children = append(f.children, c)
	}
	return f
}
func (f *fittedAttachmentNode) Style() *layout.Style       { return &f.style }
func (f *fittedAttachmentNode) LayoutState() *layout.State { return &f.state }
func (f *fittedAttachmentNode) ChildCount() int            { return len(f.children) }
func (f *fittedAttachmentNode) Child(i int) layout.Node    { return f.children[i] }
func (f *fittedAttachmentNode) size() Size {
	return Size{Width: Points(f.state.Rounded.Size.Width), Height: Points(f.state.Rounded.Size.Height)}
}
func (f *fittedAttachmentNode) Measure(known layout.Size[layout.Opt], available layout.Size[layout.AvailableSpace]) layout.Size[float32] {
	n := f.n
	var natural Size
	switch {
	case n.canvas != nil && n.canvas.em != nil:
		em := *n.canvas.em
		em.fontSize *= Points(f.scale)
		if em.measure != nil {
			// A width-aware canvas measures itself at the fitted font, as Node.Measure does at the natural one.
			width := Auto
			switch {
			case available.Width.Kind == layout.SpaceDefinite:
				width = Points(max(available.Width.Value, 0))
			case known.Width.Valid:
				width = Points(max(known.Width.Value, 0))
			case available.Width.Kind == layout.SpaceMinContent:
				width = 0
			}
			fs := em.fontSize
			if fs <= 0 {
				fs = DefaultFontSize
			}
			box := em.boxAt(width, fs)
			return layout.Size[float32]{Width: float32(Points(max(box.Width, 0)) * fs), Height: float32(Points(max(box.Ascent, 0)+max(box.Descent, 0)) * fs)}
		}
		natural = em.naturalSize()
	case n.icon != nil && !n.icon.documentSize:
		natural = n.icon.naturalSize()
		natural.Width *= Points(f.scale)
		natural.Height *= Points(f.scale)
	default:
		return n.Measure(known, available)
	}
	size := fitNatural(natural, known)
	return layout.Size[float32]{Width: known.Width.Or(float32(size.Width)), Height: known.Height.Or(float32(size.Height))}
}

func (f *fittedAttachmentNode) descent(align VerticalAlign) Points {
	if align != 0 && align != VerticalAlignBaseline {
		return 0
	}
	c := f
	if len(f.children) == 1 {
		c = f.children[0]
	}
	if c.n.canvas == nil || c.n.canvas.em == nil {
		if baseline, ok := f.firstBaseline(); ok {
			return max(f.size().Height-baseline, 0)
		}
		return 0
	}
	descent := c.n.canvas.em.descentAt(c.size())
	if c != f {
		return max(f.size().Height-Points(c.state.Rounded.Location.Y)-c.size().Height+descent, 0)
	}
	return descent
}
func (f *fittedAttachmentNode) firstBaseline() (Points, bool) {
	if f.n.style.Display == DisplayNone || f.n.style.Visibility == VisibilityHidden {
		return 0, false
	}
	if tn := f.n.text; tn != nil && tn.prepared != nil && tn.text.Text != "" {
		box := contentBox(&f.state.Rounded)
		if fraction := tn.alignV.fraction(); fraction > 0 {
			if height := tn.size(box.Size.Width).Height; height < box.Size.Height {
				box.Origin.Y += fraction * (box.Size.Height - height)
				box.Size.Height = height
			}
		}
		shown := tn.prepared.Layout(box.Size)
		defer shown.Release()
		baseline, ok := firstLineBaseline(shown)
		return box.Origin.Y + baseline, ok
	}
	for _, child := range f.children {
		if child.n.style.Position == Absolute {
			continue
		}
		if baseline, ok := child.firstBaseline(); ok {
			return Points(child.state.Rounded.Location.Y) + baseline, true
		}
	}
	return 0, false
}

func (f *fittedAttachmentNode) apply(t *Tree, origin Point) {
	n := f.n
	t.setFrame(n, Bounds{Origin: origin, Size: f.size()})
	if n.canvas != nil && n.canvas.em != nil {
		size := Points(0)
		if f.scale != 1 {
			size = n.canvas.em.fontSize * Points(f.scale)
		}
		if size != n.canvas.em.fitFontSize {
			n.canvas.em.fitFontSize = size
			n.canvas.effect.mark(dirty)
		}
	}
	if tn := n.text; tn != nil {
		box := contentBox(&f.state.Rounded)
		if box != tn.box {
			tn.box = box
			tn.dropTextLayout()
			t.mark(n)
		}
	}
	for _, child := range f.children {
		l := child.state.Rounded.Location
		at := Point{X: Points(l.X), Y: Points(l.Y)}
		if child.n.style.Position == Absolute {
			for p := f; p != nil && p.n.style.Position == Static && !p.n.isRoot; p = p.parent {
				at.X -= p.n.frame.Origin.X
				at.Y -= p.n.frame.Origin.Y
			}
		}
		child.apply(t, at)
	}
}

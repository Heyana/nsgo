package formula

import (
	"github.com/caelyreth/untie"
	"github.com/caelyreth/untie/identity"
	"github.com/caelyreth/untie/ir"

	"github.com/nsgo-dev/nsgo/core"
)

// systemRun retains one whole text run, so the platform shapes its scripts, combining marks and bidi together rather than drawing missing glyphs alone.
type systemRun struct {
	text    string
	font    core.Font
	metrics core.TextMetrics
	x, y    float32
	scale   float32
}

// withSystemText belongs to the mounted View, not the renderer's shared cache: two engines can have different system fonts, metrics and fallback coverage.
func (r *Renderer) withSystemText(l *layout, measure func(string, core.Font) core.TextMetrics, language string) *layout {
	if l.textExpr == nil {
		return l
	}
	root, err := l.textExpr.RootSnapshot()
	if err != nil {
		return l
	}
	var measured []textMeasure
	texts := rewriteSystemText(root, l.missing, language, func(text string, font core.Font) core.TextMetrics {
		m := measure(text, font)
		measured = append(measured, textMeasure{width: m.Width, ascent: m.Ascent, descent: m.Descent})
		return m
	})
	engine, err := r.engine()
	if err != nil {
		return l
	}
	box, err := engine.layoutSystemText(root, l.key, l.missing, language, measured)
	if err != nil || box == nil {
		return l
	}
	out := &layout{key: l.key, description: l.description, drawn: true}
	out.drawBox(box, texts)
	return out
}

// replaySystemText applies pre-measured text run geometry in the worker without
// requiring the worker file to import core.
func replaySystemText(root *ir.Formula, missingRuns []untie.MissingFallback, language string, measured []textMeasure) {
	idx := 0
	rewriteSystemText(root, missingRuns, language, func(string, core.Font) core.TextMetrics {
		if idx >= len(measured) {
			return core.TextMetrics{}
		}
		m := measured[idx]
		idx++
		return core.TextMetrics{Width: m.width, Ascent: m.ascent, Descent: m.descent}
	})
}

// rewriteSystemText replaces each IR text run overlapping missing with a filled
// positioning marker sized by measure, returning the native text runs keyed by
// their assigned marker identity. Both the main thread and the browser worker
// run the exact same deterministic walk so marker identities match.
func rewriteSystemText(root *ir.Formula, missingRuns []untie.MissingFallback, language string, measure func(string, core.Font) core.TextMetrics) map[identity.NodeID]systemRun {
	texts := map[identity.NodeID]systemRun{}
	// Parsed nodes have no assigned identities. Reserve our marker identities above any existing one so this also stays valid if the parser adds them.
	var next identity.NodeID
	_ = ir.WalkFormula(root, untie.Limits{}, func(n ir.Node) bool {
		next = max(next, n.NodeBase().NodeID)
		return true
	})
	var rewrite func(ir.Node, ir.Variant, ir.Paint) ir.Node
	rewrite = func(n ir.Node, variant ir.Variant, paint ir.Paint) ir.Node {
		if n == nil {
			return nil
		}
		base := n.NodeBase()
		if base.Variant != ir.VariantAuto {
			variant = base.Variant
		}
		if base.Paint.HasExplicit || base.Paint.Role != ir.PaintInk {
			paint = base.Paint
		}
		if c, ok := n.(*ir.Color); ok {
			paint = c.Paint
		}
		if text, ok := n.(*ir.Text); ok {
			missing := false
			for _, need := range missingRuns {
				if need.Source.Start < text.Span.End && need.Source.End > text.Span.Start {
					missing = true
					break
				}
			}
			if !missing {
				return n
			}
			font := core.Font{Size: referenceSize, Language: language}
			switch variant {
			case ir.VariantBold, ir.VariantBoldScript, ir.VariantBoldFraktur, ir.VariantBoldSans:
				font.Weight = core.FontWeightBold
			case ir.VariantItalic:
				font.Italic = true
			case ir.VariantBoldItalic:
				font.Weight, font.Italic = core.FontWeightBold, true
			case ir.VariantMonospace:
				font.Family = core.FontMonospace
			}
			m := measure(text.Text, font)
			if m.Width < 0 || m.Ascent+m.Descent <= 0 {
				return n
			}
			width, ew := ir.EmLength(float64(m.Width / referenceSize))
			ascent, ea := ir.EmLength(float64(m.Ascent / referenceSize))
			descent, ed := ir.EmLength(float64(m.Descent / referenceSize))
			if ew != nil || ea != nil || ed != nil {
				return n
			}
			box := ir.NewBox(text.Span, ir.NewSpace(text.Span, ir.SpaceExplicit, width))
			box.Origin, box.Class, box.Size, box.Cramped = text.Origin, text.Class, text.Size, text.Cramped
			box.Variant, box.Paint = text.Variant, text.Paint
			box.LeftSpacing, box.RightSpacing = text.LeftSpacing, text.RightSpacing
			box.IgnoreAsSpace = text.IgnoreAsSpace
			box.Padding[0], box.Padding[2] = ascent, descent
			box.HasFill, box.Fill = true, paint
			next++
			box.NodeID = next
			texts[next] = systemRun{text: text.Text, font: font, metrics: m}
			return box
		}
		child := func(n ir.Node) ir.Node { return rewrite(n, variant, paint) }
		switch n := n.(type) {
		case *ir.Row:
			for i := range n.Items {
				n.Items[i] = child(n.Items[i])
			}
		case *ir.Fraction:
			n.Numerator, n.Denominator = child(n.Numerator), child(n.Denominator)
		case *ir.Radical:
			n.Radicand, n.Index = child(n.Radicand), child(n.Index)
		case *ir.Scripts:
			n.BaseItem, n.Top, n.Bottom = child(n.BaseItem), child(n.Top), child(n.Bottom)
			n.TopLeft, n.BottomLeft = child(n.TopLeft), child(n.BottomLeft)
			n.TopRight, n.BottomRight = child(n.TopRight), child(n.BottomRight)
		case *ir.Fenced:
			// Delimiters are glyphs; only the body contains text runs.
			for i := range n.Body {
				n.Body[i] = child(n.Body[i])
			}
		case *ir.Accent:
			n.Body = child(n.Body)
		case *ir.Line:
			n.Body = child(n.Body)
		case *ir.Cancel:
			n.Body = child(n.Body)
		case *ir.Style:
			n.Body = child(n.Body)
		case *ir.Overlay:
			n.Body = child(n.Body)
		case *ir.Raise:
			n.Body = child(n.Body)
		case *ir.Color:
			n.Body = child(n.Body)
		case *ir.Box:
			n.Body = child(n.Body)
		case *ir.Phantom:
			n.Body = child(n.Body)
		case *ir.Table:
			for i := range n.Rows {
				for j := range n.Rows[i].Cells {
					n.Rows[i].Cells[j] = child(n.Rows[i].Cells[j])
				}
			}
		case *ir.Multiline:
			for i := range n.Rows {
				for j := range n.Rows[i] {
					n.Rows[i][j] = child(n.Rows[i][j])
				}
			}
		}
		return n
	}
	root.Root = rewrite(root.Root, ir.VariantNormal, ir.Paint{})
	for i := range root.Tags {
		root.Tags[i].Body = rewrite(root.Tags[i].Body, ir.VariantNormal, ir.Paint{})
	}
	return texts
}

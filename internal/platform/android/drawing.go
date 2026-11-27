package android

import (
	"encoding/binary"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// writeDrawing writes each op as its kind and the fields it uses, in the
// format CanvasNode plays, then where the arguments bound to animated
// nodes are in those bytes, which Java writes their values into, then
// the drawing's nodes. DrawTextAlign and DrawTextBaseline, which only
// the recorder reads, are left out, and DrawShader, as CanvasNode has no
// GPU path: an image op that shades draws plain.
func writeDrawing(w *writer, d *driver.Drawing) {
	type slot struct {
		at    int
		color bool
		node  uint32
	}
	var slots []slot
	at := len(w.buf)
	w.u32(0)
	start := len(w.buf)
	bound := map[[2]int]uint32{}
	if d != nil {
		for _, s := range d.Slots {
			bound[[2]int{s.Op, int(s.Arg)}] = s.Node
		}
	}
	var i int
	// arg writes an argument, noting where it is when a node drives it.
	arg := func(a driver.DrawArg, v float32) {
		if n, ok := bound[[2]int{i, int(a)}]; ok {
			slots = append(slots, slot{at: len(w.buf) - start, node: n})
		}
		w.float(v)
	}
	color := func(c driver.Color) {
		if n, ok := bound[[2]int{i, int(driver.ArgColor)}]; ok {
			slots = append(slots, slot{at: len(w.buf) - start, color: true, node: n})
		}
		w.u32(uint32(c))
	}
	var ops []driver.DrawOp
	if d != nil {
		ops = d.Ops
	}
	for i = range ops {
		op := &ops[i]
		if op.Kind == driver.DrawTextAlign || op.Kind == driver.DrawTextBaseline || op.Kind == driver.DrawShader {
			continue
		}
		if op.Kind == driver.DrawVertices {
			if draw, ok := op.ImageRect(); ok {
				op = &draw
			}
		}
		w.byte(byte(op.Kind))
		switch op.Kind {
		case driver.DrawTranslate, driver.DrawScale, driver.DrawMoveTo, driver.DrawLineTo:
			arg(driver.ArgX, op.X)
			arg(driver.ArgY, op.Y)
		case driver.DrawRotate, driver.DrawAlpha:
			arg(driver.ArgA, op.A)
		case driver.DrawQuadTo:
			arg(driver.ArgX1, op.X1)
			arg(driver.ArgY1, op.Y1)
			arg(driver.ArgX, op.X)
			arg(driver.ArgY, op.Y)
		case driver.DrawCubicTo:
			arg(driver.ArgX1, op.X1)
			arg(driver.ArgY1, op.Y1)
			arg(driver.ArgX2, op.X2)
			arg(driver.ArgY2, op.Y2)
			arg(driver.ArgX, op.X)
			arg(driver.ArgY, op.Y)
		case driver.DrawTransform, driver.DrawSetTransform:
			arg(driver.ArgX1, op.X1)
			arg(driver.ArgY1, op.Y1)
			arg(driver.ArgX2, op.X2)
			arg(driver.ArgY2, op.Y2)
			arg(driver.ArgX, op.X)
			arg(driver.ArgY, op.Y)
		case driver.DrawArc:
			arg(driver.ArgX, op.X)
			arg(driver.ArgY, op.Y)
			arg(driver.ArgR, op.R)
			arg(driver.ArgA0, op.A0)
			arg(driver.ArgA1, op.A1)
			w.flags(op.CCW)
		case driver.DrawRect, driver.DrawClear:
			arg(driver.ArgX, op.X)
			arg(driver.ArgY, op.Y)
			arg(driver.ArgW, op.W)
			arg(driver.ArgH, op.H)
		case driver.DrawRoundRect:
			arg(driver.ArgX, op.X)
			arg(driver.ArgY, op.Y)
			arg(driver.ArgW, op.W)
			arg(driver.ArgH, op.H)
			arg(driver.ArgR, op.R)
		case driver.DrawEllipse:
			arg(driver.ArgX, op.X)
			arg(driver.ArgY, op.Y)
			arg(driver.ArgR, op.R)
			arg(driver.ArgRY, op.RY)
		case driver.DrawFill, driver.DrawClip:
			w.flags(op.EvenOdd)
		case driver.DrawFillColor, driver.DrawStrokeColor:
			color(op.Color)
		case driver.DrawFillGradient, driver.DrawStrokeGradient:
			writeGradient(w, op.Gradient)
		case driver.DrawLineWidth, driver.DrawLineDashOffset, driver.DrawMiterLimit:
			arg(driver.ArgW, op.W)
		case driver.DrawLineCap:
			w.byte(byte(op.Cap))
		case driver.DrawLineJoin:
			w.byte(byte(op.Join))
		case driver.DrawComposite:
			w.byte(byte(op.Composite))
		case driver.DrawLineDash:
			var dash []float32
			if op.Dash != nil {
				dash = *op.Dash
			}
			w.u32(uint32(len(dash)))
			w.floats(dash...)
		case driver.DrawShadow:
			color(op.Color)
			arg(driver.ArgX, op.X)
			arg(driver.ArgY, op.Y)
			arg(driver.ArgR, op.R)
		case driver.DrawFillText, driver.DrawStrokeText:
			w.str(op.Text)
			// The point the recorder placed the text at.
			arg(driver.ArgX, op.X+op.X1)
			arg(driver.ArgY, op.Y+op.Y1)
			w.str(op.Font.Family)
			w.points(op.Font.Size)
			w.u16(uint16(op.Font.Weight))
			variations := driver.FontAndroidVariationSettings(op.Font)
			hasAxes := variations != ""
			hasFeatures := len(op.Font.Features) > 0
			w.flags(op.Font.Italic, hasAxes, hasFeatures)
			w.byte(byte(op.Font.Variant))
			if hasAxes {
				w.str(variations)
			}
			if hasFeatures {
				w.str(driver.FontAndroidFeatureSettings(op.Font))
			}
		case driver.DrawAtlas:
			writeAtlas(w, d, op)
		case driver.DrawImage:
			var key string
			var density float32 = 1
			if img := d.Image(op); img != nil {
				key, density = img.Key(), img.Density()
			}
			w.str(key)
			w.float(density)
			arg(driver.ArgX, op.X)
			arg(driver.ArgY, op.Y)
			arg(driver.ArgW, op.W)
			arg(driver.ArgH, op.H)
			arg(driver.ArgX1, op.X1)
			arg(driver.ArgY1, op.Y1)
			arg(driver.ArgX2, op.X2)
			arg(driver.ArgY2, op.Y2)
			w.flags(op.Sampling == driver.ImageSamplingNearest, op.Tinted)
			if op.Tinted {
				color(op.Color)
			}
		case driver.DrawVertices:
			var key string
			if img := d.Image(op); img != nil {
				key = img.Key()
			}
			w.str(key)
			w.flags(op.Sampling == driver.ImageSamplingNearest)
			var vertices []driver.CanvasVertex
			if op.Mesh != nil {
				vertices = op.Mesh.Vertices
			}
			w.u32(uint32(len(vertices)))
			for _, v := range vertices {
				w.points(v.Position.X)
				w.points(v.Position.Y)
				w.points(v.TexCoord.X)
				w.points(v.TexCoord.Y)
				w.u32(uint32(v.Color))
			}
		}
	}
	binary.BigEndian.PutUint32(w.buf[at:], uint32(len(w.buf)-start))
	w.u32(uint32(len(slots)))
	for _, s := range slots {
		w.u32(uint32(s.at))
		w.flags(s.color)
		w.u32(s.node)
	}
	var nodes []driver.CanvasNode
	if d != nil {
		nodes = d.Nodes
	}
	w.u32(uint32(len(nodes)))
	for _, n := range nodes {
		w.bounds(n.Bounds)
		w.byte(byte(n.Role))
		w.str(n.ID)
		w.str(n.Label)
		w.str(n.Hint)
		w.str(n.TestID)
		w.floats(n.Value, n.Min, n.Max, n.Step)
		w.str(n.ValueText)
		w.flags(n.Selected, n.Disabled, n.Busy)
		w.byte(byte(n.Expanded))
		w.byte(byte(n.Checked))
		w.byte(byte(n.Live))
		w.byte(byte(n.EffectiveActions()))
	}
	// The resolution scale and whether its pixels stretch to the nearest.
	w.float(float32(d.ResolutionScale()))
	w.flags(!d.Smooth())
}

// writeAtlas supplies one image key followed by the sprite records.
func writeAtlas(w *writer, d *driver.Drawing, op *driver.DrawOp) {
	img := d.Image(op)
	sprites := d.AtlasSprites(op)
	if img == nil || len(sprites) == 0 {
		w.str("")
		w.flags(false)
		w.u32(0)
		return
	}
	w.str(img.Key())
	w.flags(op.Sampling == driver.ImageSamplingNearest)
	w.u32(uint32(len(sprites)))
	for _, sprite := range sprites {
		b, t := sprite.Rect, sprite.Transform
		w.points(b.Origin.X)
		w.points(b.Origin.Y)
		w.points(b.Size.Width)
		w.points(b.Size.Height)
		w.floats(t.SCos, t.SSin, t.TX, t.TY, sprite.Alpha)
	}
}

// firstFamily is the family writeHeaderFont sends a native header's fontFamily list as: the first, the system's for a generic one, so that an empty family keeps the header's own typeface. Text and drawings send the whole list, which Text.typeface falls back through.
func firstFamily(list string) string {
	families := driver.FontFamilies(list)
	if len(families) == 0 {
		return list
	}
	switch f := families[0]; f.Kind {
	case driver.FamilyMonospace:
		return driver.FontMonospace
	case driver.FamilySerif:
		return "serif"
	case driver.FamilySystem, driver.FamilyRounded:
		return ""
	default:
		return f.Name
	}
}

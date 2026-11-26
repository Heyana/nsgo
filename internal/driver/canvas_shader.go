package driver

import "slices"

// ShaderPaint is a compiled program with the values of its uniforms and
// the images it reads, which a drawing shades with (DrawShader,
// SetShader). core's ShaderProgram.Paint makes one; the zero ShaderPaint
// shades nothing.
type ShaderPaint struct{ s *Shader }

// NewShaderPaint returns the paint of s, which the drawing never changes.
func NewShaderPaint(s *Shader) ShaderPaint { return ShaderPaint{s} }

// PaintShader returns what p shades with, nil for the zero paint: a
// function rather than a method, so the public API reaches no Shader.
func PaintShader(p ShaderPaint) *Shader { return p.s }

// DrawShader runs paint's program for the pixels the rectangle x, y, w,
// h covers, under the transform, the clip, the alpha and the composite
// operator, in the drawing's order. The program reads pos in points from
// the rectangle's top left corner, dst the same point in pixels, rectSize
// the rectangle's size and pixelScale the pixels a point of it covers.
// A platform with no GPU path for canvases draws nothing.
func (d *Drawing) DrawShader(paint ShaderPaint, x, y, w, h float32) *Drawing {
	if paint.s == nil || w <= 0 || h <= 0 {
		return d
	}
	return d.op(DrawOp{Kind: DrawShader, Shader: paint, X: x, Y: y, W: w, H: h})
}

// SetShader shades the images later DrawImage, DrawImageIn,
// DrawImageFrom, DrawAtlas and DrawVertices calls draw with paint, until
// ClearShader: the image drawn is the program's image0, the paint's own
// images image1 on, pos is the point of image0 in its points, dst the same
// point in its pixels, rectSize image0's size in points and pixelScale its
// pixels a point. The program's color, premultiplied, takes each sprite's
// alpha, a vertex's color and a tint as the plain image would. A
// platform with no GPU path for canvases draws the images unshaded. Save
// and Restore keep the setting.
func (d *Drawing) SetShader(paint ShaderPaint) *Drawing {
	d.shader = paint.s
	return d
}

// ClearShader draws later images unshaded.
func (d *Drawing) ClearShader() *Drawing {
	d.shader = nil
	return d
}

// bindLanes binds the lanes of the last op's shader that animated nodes
// drive.
func (d *Drawing) bindLanes(s *Shader) {
	d.shaded = true
	at := len(d.Ops) - 1
	for _, n := range s.Nodes {
		if n.Node != 0 && n.Lane >= 0 && n.Lane < len(s.Lanes) {
			d.Slots = append(d.Slots, DrawSlot{Op: at, Arg: ArgLane, lane: n.Lane, channel: n.Channel, Node: n.Node})
		}
	}
}

// Shaded reports whether an op of the drawing shades with a program.
func (d *Drawing) Shaded() bool { return d.shaded }

// setLane returns op with its shader's lane set to v, the shader copied.
func setLane(op DrawOp, lane int, v float64) DrawOp {
	if op.Shader.s == nil || lane < 0 || lane >= len(op.Shader.s.Lanes) {
		return op
	}
	s := *op.Shader.s
	s.Lanes = slices.Clone(s.Lanes)
	s.Lanes[lane] = v
	op.Shader.s = &s
	return op
}

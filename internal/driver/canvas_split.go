package driver

// CanvasSplit is where a drawing's static part ends, as NSH's
// first_animated and prefix_path_needed: what the ops before At paint is
// the same whatever its bound arguments hold. A backend may keep those
// pixels and, when the arguments change, play only the painting from At
// on over them; the ops before At still set the state.
type CanvasSplit struct {
	// At is the first op a slot binds, len(Ops) when none is bound.
	At int
	// Path says the current path the ops before At build is still used
	// from At on: by a clip before At, or by an op from At on before a
	// DrawBeginPath starts anew. A path begun anew just before At is
	// empty there and is not.
	Path bool
}

// SplitCanvas returns where d's static part ends.
func SplitCanvas(d *Drawing) CanvasSplit {
	at := len(d.Ops)
	for _, s := range d.Slots {
		if s.Op >= 0 && s.Op < at {
			at = s.Op
		}
	}
	s := CanvasSplit{At: at}
	for _, op := range d.Ops[:at] {
		if op.Kind == DrawClip {
			s.Path = true
			return s
		}
	}
	// The path at At, if any, is what the ops since the last
	// DrawBeginPath before it built.
	for i := at - 1; i >= 0 && d.Ops[i].Kind != DrawBeginPath; i-- {
		if d.Ops[i].Kind.BuildsPath() {
			s.Path = true
			break
		}
	}
	if !s.Path {
		return s
	}
	s.Path = false
	for _, op := range d.Ops[at:] {
		if op.Kind == DrawBeginPath {
			break
		}
		if op.Kind.UsesPath() {
			s.Path = true
			break
		}
	}
	return s
}

// Static reports whether d has a static part worth keeping: some ops
// before At paint, and some from At on do too.
func (s CanvasSplit) Static(d *Drawing) bool {
	if s.At <= 0 || s.At >= len(d.Ops) {
		return false
	}
	for _, op := range d.Ops[:s.At] {
		if op.Kind.Paints() {
			return true
		}
	}
	return false
}

// Skips reports whether a player drawing over the kept static part
// leaves out op i: one before At that paints, or that builds a path no
// op from At on uses.
func (s CanvasSplit) Skips(i int, op *DrawOp) bool {
	if i >= s.At {
		return false
	}
	return op.Kind.Paints() || (!s.Path && (op.Kind == DrawBeginPath || op.Kind.BuildsPath()))
}

// Paints reports whether an op of kind k changes pixels.
func (k DrawKind) Paints() bool {
	switch k {
	case DrawFill, DrawStroke, DrawFillText, DrawStrokeText, DrawClear, DrawImage:
		return true
	}
	return false
}

// BuildsPath reports whether an op of kind k adds to the current path or
// closes it.
func (k DrawKind) BuildsPath() bool {
	switch k {
	case DrawMoveTo, DrawLineTo, DrawQuadTo, DrawCubicTo, DrawArc, DrawRect, DrawRoundRect, DrawEllipse,
		DrawClosePath:
		return true
	}
	return false
}

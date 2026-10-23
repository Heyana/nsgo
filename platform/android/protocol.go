package android

import (
	"encoding/binary"
	"math"

	"github.com/oneo-me/nsgo/core"
)

// This file holds what builds on every system: the protocol's encoding and
// the encoders of animations, so tests can check them against the Java
// runtime on a machine with a JDK.

// Native components on Android depart from NSH's, whose components are
// Java objects the embedding app registers: the Go process cannot make
// Android views, so a component's View returns the fully qualified name
// of an android.view.View subclass, such as "android.widget.SeekBar".
// The runtime builds it by reflection with its (Context) constructor,
// sets each prop by calling its public one-argument set method (prop
// "rating" calls setRating) with the JSON value coerced to the parameter,
// runs a command by calling the public method of that name with the
// arguments, reports the view's measured size as EventNativeSize and
// reports each callback of the listeners its class declares (its own
// setOn…Listener methods, not View's) as an event named after the
// callback, its arguments in a JSON array. The Go component sees every
// prop and command too, and may implement PropMapper and EventMapper to
// translate them.

// ViewProp is one setter call on a component's view: set<Name>(Value),
// Name's first letter upper-cased.
type ViewProp struct {
	Name  string
	Value any
}

// PropMapper is implemented by an Android component that chooses the
// setter calls a prop makes, in order; without it each prop calls its own
// setter with its value.
type PropMapper interface {
	ViewProps(name string, value any) []ViewProp
}

// EventMapper is implemented by an Android component that receives the
// listener callbacks of its view itself, to emit what it likes; without
// it each callback is emitted as is.
type EventMapper interface {
	ViewEvent(name string, args []any)
}

// writeLayoutAnimation writes a batch's layout animation: whether there
// is one, then each phase as LayoutTransition.read reads it.
func writeLayoutAnimation(w *writer, a *core.LayoutAnimation) {
	if a == nil {
		w.byte(0)
		return
	}
	w.byte(1)
	for _, tr := range [...]*core.LayoutTransition{a.Create, a.Update, a.Delete} {
		if tr == nil {
			w.byte(0)
			continue
		}
		w.byte(1)
		w.byte(byte(tr.Curve))
		w.float(tr.Damping)
		w.byte(byte(tr.Property))
		w.u64(uint64(tr.Duration))
		w.u64(uint64(tr.Delay))
	}
}

// writeAnimation writes an ItemAnimation's ops as Animator.readOp reads
// them. Numbers stay float64, so Java computes what core.Animator would.
func writeAnimation(w *writer, ops []core.AnimationOp) {
	w.u32(uint32(len(ops)))
	for i := range ops {
		o := &ops[i]
		w.byte(byte(o.Kind))
		w.u32(o.Node)
		switch o.Kind {
		case core.AnimCreateValue, core.AnimSet, core.AnimSetOffset:
			w.f64(o.Value)
		case core.AnimCreateInterpolate:
			w.u32(o.Input)
			writeInterpolation(w, o.Interpolation)
		case core.AnimCreateMath:
			w.byte(byte(o.Math))
			w.u32(o.A.Node)
			w.f64(o.A.Const)
			w.u32(o.B.Node)
			w.f64(o.B.Const)
		case core.AnimCreateDiffClamp:
			w.u32(o.Input)
			w.f64(o.Min)
			w.f64(o.Max)
		case core.AnimRead, core.AnimStop:
			w.u32(o.ID)
		case core.AnimStart:
			w.u32(o.ID)
			w.flags(o.Animation != nil)
			if o.Animation != nil {
				writeSpec(w, o.Animation)
			}
		case core.AnimBind:
			w.u32(uint32(o.View))
			w.byte(byte(o.Prop))
		case core.AnimBindPan, core.AnimBindScroll:
			w.u32(uint32(o.View))
			w.u32(o.X)
			w.u32(o.Y)
		}
	}
}

func writeInterpolation(w *writer, r *core.Interpolation) {
	if r == nil {
		r = &core.Interpolation{}
	}
	w.u32(uint32(len(r.Input)))
	for _, x := range r.Input {
		w.f64(x)
	}
	w.u32(uint32(len(r.Output)))
	for _, x := range r.Output {
		w.f64(x)
	}
	w.u32(uint32(len(r.Colors)))
	for _, c := range r.Colors {
		w.u32(uint32(c))
	}
	w.byte(byte(r.Extrapolate))
	w.byte(byte(r.ExtrapolateLeft))
	w.byte(byte(r.ExtrapolateRight))
	writeEasing(w, r.Easing)
}

func writeEasing(w *writer, e core.Easing) {
	w.byte(byte(e.Kind))
	w.byte(byte(e.Mode))
	w.floats(e.Bezier.X1, e.Bezier.Y1, e.Bezier.X2, e.Bezier.Y2)
	w.f64(e.Param)
}

// writeSpec writes an animation tree as Animator.Spec.read reads it; a
// nil step is kind 0, which Java runs as an empty delay.
func writeSpec(w *writer, a *core.Animation) {
	if a == nil {
		a = &core.Animation{}
	}
	w.byte(byte(a.Kind))
	w.u32(a.Value)
	w.f64(a.To)
	w.u64(uint64(a.Timing.Duration))
	w.u64(uint64(a.Timing.Delay))
	writeEasing(w, a.Timing.Easing)
	sp, d := a.Spring, a.Decay
	w.f64(sp.Stiffness)
	w.f64(sp.Damping)
	w.f64(sp.Mass)
	w.f64(sp.Velocity)
	w.f64(d.Velocity)
	w.f64(d.Deceleration)
	w.flags(d.Clamp)
	w.f64(d.Min)
	w.f64(d.Max)
	w.f64(d.Bounce)
	w.u64(uint64(a.Delay))
	w.u32(uint32(int32(a.Iterations)))
	w.u32(uint32(len(a.Steps)))
	for _, st := range a.Steps {
		writeSpec(w, st)
	}
}

// writeGradient writes a fill or stroke gradient as CanvasNode reads it.
func writeGradient(w *writer, g *core.Gradient) {
	if g == nil {
		g = &core.Gradient{}
	}
	w.flags(g.Radial)
	w.floats(g.X1, g.Y1, g.R1, g.X2, g.Y2, g.R2)
	w.floats(g.Transform[:]...)
	w.byte(byte(g.Spread))
	w.u32(uint32(len(g.Stops)))
	for _, st := range g.Stops {
		w.float(st.Offset)
		w.u32(uint32(st.Color))
	}
}

// writer and reader are the protocol's encoding: big-endian, lengths
// as uint32, points as float32, strings as a length and UTF-8 bytes and
// booleans as the bits of one byte, the first lowest.
type writer struct{ buf []byte }

func (w *writer) byte(b byte)          { w.buf = append(w.buf, b) }
func (w *writer) u16(v uint16)         { w.buf = binary.BigEndian.AppendUint16(w.buf, v) }
func (w *writer) u32(v uint32)         { w.buf = binary.BigEndian.AppendUint32(w.buf, v) }
func (w *writer) u64(v uint64)         { w.buf = binary.BigEndian.AppendUint64(w.buf, v) }
func (w *writer) float(f float32)      { w.u32(math.Float32bits(f)) }
func (w *writer) f64(f float64)        { w.u64(math.Float64bits(f)) }
func (w *writer) points(p core.Points) { w.float(float32(p)) }
func (w *writer) str(s string)         { w.u32(uint32(len(s))); w.buf = append(w.buf, s...) }

func (w *writer) floats(fs ...float32) {
	for _, f := range fs {
		w.float(f)
	}
}

func (w *writer) bounds(b core.Bounds) {
	w.points(b.Origin.X)
	w.points(b.Origin.Y)
	w.points(b.Size.Width)
	w.points(b.Size.Height)
}

func (w *writer) flags(bits ...bool) {
	var b byte
	for i, on := range bits {
		if on {
			b |= 1 << i
		}
	}
	w.byte(b)
}

type reader struct{ buf []byte }

func (r *reader) byte() byte {
	if len(r.buf) < 1 {
		return 0
	}
	b := r.buf[0]
	r.buf = r.buf[1:]
	return b
}

func (r *reader) u32() uint32 {
	if len(r.buf) < 4 {
		return 0
	}
	v := binary.BigEndian.Uint32(r.buf)
	r.buf = r.buf[4:]
	return v
}

func (r *reader) u64() uint64 {
	if len(r.buf) < 8 {
		return 0
	}
	v := binary.BigEndian.Uint64(r.buf)
	r.buf = r.buf[8:]
	return v
}

func (r *reader) float() float32      { return math.Float32frombits(r.u32()) }
func (r *reader) points() core.Points { return core.Points(r.float()) }

func (r *reader) str() string {
	n := int(r.u32())
	if n > len(r.buf) {
		n = len(r.buf)
	}
	s := string(r.buf[:n])
	r.buf = r.buf[n:]
	return s
}

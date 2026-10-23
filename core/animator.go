package core

import (
	"cmp"
	"math"
	"time"
)

// Animator evaluates the animation graph on a platform's main thread, as
// NSH's does: backends feed it each ItemAnimation, step it once a frame
// while it runs, and apply the values it gives each bound view. It is
// pure Go and touches no view, so every backend shares it.
//
// Drivers are closed-form functions of the time since they started, so
// the result does not depend on the frame rate or dropped frames. Not safe
// for concurrent use.
type Animator struct {
	nodes    map[uint32]*anode
	runs     map[uint32]*arun
	views    map[NodeID]*aview
	pans     map[NodeID][2]uint32
	scrolls  map[NodeID][2]uint32
	report   func(Event)
	gen      uint64 // bumped when any value changes
	changed  map[NodeID]bool
	now      float64 // seconds of the last step
	finished []Event
}

// NewAnimator returns an animator that reports finished runs and reads
// through report, for the backend to dispatch to the engine.
func NewAnimator(report func(Event)) *Animator {
	return &Animator{nodes: map[uint32]*anode{}, runs: map[uint32]*arun{}, views: map[NodeID]*aview{},
		pans: map[NodeID][2]uint32{}, scrolls: map[NodeID][2]uint32{}, report: report, changed: map[NodeID]bool{}}
}

// anode is a node of the graph.
type anode struct {
	kind   AnimationOpKind // the op that made it
	value  float64         // a value's
	offset float64
	vel    float64 // units per second, from the driver moving it
	driver *aleaf  // the animation moving a value
	op     AnimationOp
	// cache is the node's output in generation gen.
	gen   uint64
	num   float64
	color Color
	// diff-clamp state
	last, lastIn float64
	primed       bool
}

type aview struct {
	nodes [propCount]uint32
	sent  AnimatedValues
}

// AnimatedValues are the bound properties of a view, as the animator
// last evaluated them.
type AnimatedValues struct {
	// Set holds a bit for each bound AnimatedProp.
	Set                    uint16
	Opacity                float32
	TranslateX, TranslateY Points
	Scale, ScaleX, ScaleY  float32
	Rotate                 float32
	Background             Color
	BorderColor            Color
	ShadowColor            Color
	ShadowRadius           Points
}

// Has reports whether p is bound.
func (v AnimatedValues) Has(p AnimatedProp) bool { return v.Set&(1<<p) != 0 }

// Apply returns vs with the bound properties in place of its own, for
// backends that draw from a ViewStyle.
func (v AnimatedValues) Apply(vs ViewStyle) ViewStyle {
	if v.Has(PropOpacity) {
		vs.Opacity = v.Opacity
	}
	t := &vs.Transform
	if v.Has(PropTranslateX) {
		t.TranslateX = v.TranslateX
	}
	if v.Has(PropTranslateY) {
		t.TranslateY = v.TranslateY
	}
	if v.Has(PropScale) {
		t.Scale = v.Scale
	}
	if v.Has(PropScaleX) {
		t.ScaleX = v.ScaleX
	}
	if v.Has(PropScaleY) {
		t.ScaleY = v.ScaleY
	}
	if v.Has(PropRotate) {
		t.Rotate = v.Rotate
	}
	if v.Has(PropBackground) {
		vs.Background = v.Background
	}
	if v.Has(PropBorderColor) {
		vs.BorderColor = v.BorderColor
		if vs.Borders != (Edges{}) {
			vs.BorderColors = AllColors(v.BorderColor)
		}
	}
	if v.Has(PropShadowColor) {
		vs.ShadowColor = v.ShadowColor
	}
	if v.Has(PropShadowRadius) {
		vs.ShadowRadius = v.ShadowRadius
	}
	return vs
}

// Apply applies a batch's changes to the graph.
func (a *Animator) Apply(ops []AnimationOp) {
	for i := range ops {
		a.apply(&ops[i])
	}
}

func (a *Animator) apply(o *AnimationOp) {
	switch o.Kind {
	case AnimCreateValue:
		a.nodes[o.Node] = &anode{kind: o.Kind, value: o.Value}
		a.touch()
	case AnimCreateInterpolate, AnimCreateMath, AnimCreateDiffClamp:
		a.nodes[o.Node] = &anode{kind: o.Kind, op: *o}
		a.touch()
	case AnimSet, AnimSetOffset, AnimFlattenOffset, AnimExtractOffset, AnimStopValue:
		n := a.value(o.Node)
		if n == nil {
			return
		}
		if o.Kind != AnimSetOffset {
			a.interrupt(n)
		}
		switch o.Kind {
		case AnimSet:
			n.value = o.Value
		case AnimSetOffset:
			n.offset = o.Value
		case AnimFlattenOffset:
			n.value += n.offset
			n.offset = 0
		case AnimExtractOffset:
			n.offset += n.value
			n.value = 0
		}
		a.touch()
	case AnimRead:
		v := 0.0
		if n := a.value(o.Node); n != nil {
			v = n.value + n.offset
		}
		a.report(Event{Kind: EventAnimation, Count: o.ID, Value: float32(v), Finished: true})
	case AnimStart:
		if o.Animation == nil {
			return
		}
		r := &arun{id: o.ID, root: a.build(o.Animation)}
		a.runs[o.ID] = r
		r.root.bind(r)
		r.root.start(a, -1)
	case AnimStop:
		if r := a.runs[o.ID]; r != nil {
			a.end(r, false)
		}
	case AnimBind:
		v := a.views[o.View]
		if v == nil {
			if o.Node == 0 {
				return
			}
			v = &aview{}
			a.views[o.View] = v
		}
		v.nodes[o.Prop] = o.Node
		a.changed[o.View] = true
		if v.nodes == ([propCount]uint32{}) {
			delete(a.views, o.View)
		}
	case AnimBindPan:
		a.pans[o.View] = [2]uint32{o.X, o.Y}
	case AnimBindScroll:
		a.scrolls[o.View] = [2]uint32{o.X, o.Y}
	case AnimRelease:
		if n := a.nodes[o.Node]; n != nil {
			a.interrupt(n)
			delete(a.nodes, o.Node)
			a.touch()
		}
	}
}

// touch notes that values changed, so bound views are evaluated again.
func (a *Animator) touch() {
	a.gen++
	for id := range a.views {
		a.changed[id] = true
	}
}

func (a *Animator) value(id uint32) *anode {
	if n := a.nodes[id]; n != nil && n.kind == AnimCreateValue {
		return n
	}
	return nil
}

// interrupt stops the animation moving n, ending its run unfinished.
func (a *Animator) interrupt(n *anode) {
	if l := n.driver; l != nil {
		n.driver = nil
		if r := l.run; r != nil && a.runs[r.id] == r {
			a.end(r, false)
		}
	}
}

// end finishes run r, reporting whether it ran to its end.
func (a *Animator) end(r *arun, finished bool) {
	delete(a.runs, r.id)
	r.root.stop(a)
	a.report(Event{Kind: EventAnimation, Count: r.id, Finished: finished})
}

// Running reports whether an animation runs, so the backend keeps its
// frame callback.
func (a *Animator) Running() bool { return len(a.runs) > 0 }

// Step advances the running animations to now, a monotonic time.
func (a *Animator) Step(now time.Duration) {
	a.now = now.Seconds()
	for _, r := range a.runs {
		if a.runs[r.id] != r {
			continue
		}
		if done, _ := r.root.step(a, a.now); done {
			a.end(r, true)
		}
	}
}

// Changed calls fn with each bound view whose values may have changed
// since the last call, and their values.
func (a *Animator) Changed(fn func(view NodeID, v AnimatedValues)) {
	for id := range a.changed {
		delete(a.changed, id)
		vw := a.views[id]
		var vals AnimatedValues
		if vw != nil {
			vals = a.evalView(vw)
			if vals == vw.sent {
				continue
			}
			vw.sent = vals
		}
		fn(id, vals)
	}
}

// Touch has Changed report view's values again, as a backend needs after
// it drew the view's own style over them.
func (a *Animator) Touch(view NodeID) {
	if vw := a.views[view]; vw != nil {
		vw.sent = AnimatedValues{}
		a.changed[view] = true
	}
}

// Values returns view's bound values, and false when it has none.
func (a *Animator) Values(view NodeID) (AnimatedValues, bool) {
	vw := a.views[view]
	if vw == nil {
		return AnimatedValues{}, false
	}
	return a.evalView(vw), true
}

func (a *Animator) evalView(vw *aview) AnimatedValues {
	var v AnimatedValues
	for p, id := range vw.nodes {
		if id == 0 {
			continue
		}
		n := a.nodes[id]
		if n == nil {
			continue
		}
		num, col := a.eval(n)
		v.Set |= 1 << p
		switch AnimatedProp(p) {
		case PropOpacity:
			v.Opacity = float32(min(max(num, 0), 1))
		case PropTranslateX:
			v.TranslateX = Points(num)
		case PropTranslateY:
			v.TranslateY = Points(num)
		case PropScale:
			v.Scale = float32(num)
		case PropScaleX:
			v.ScaleX = float32(num)
		case PropScaleY:
			v.ScaleY = float32(num)
		case PropRotate:
			v.Rotate = float32(num)
		case PropBackground:
			v.Background = col
		case PropBorderColor:
			v.BorderColor = col
		case PropShadowColor:
			v.ShadowColor = col
		case PropShadowRadius:
			v.ShadowRadius = Points(max(num, 0))
		}
	}
	return v
}

// Pan sets the values bound to drags on view to translation, and reports
// whether there are any.
func (a *Animator) Pan(view NodeID, translation Point) bool {
	return a.drive(a.pans, view, translation)
}

// Scroll sets the values bound to scroll view view to its offset.
func (a *Animator) Scroll(view NodeID, offset Point) bool {
	return a.drive(a.scrolls, view, offset)
}

// PanBound reports whether drags on view move values.
func (a *Animator) PanBound(view NodeID) bool { _, ok := a.pans[view]; return ok }

func (a *Animator) drive(m map[NodeID][2]uint32, view NodeID, p Point) bool {
	ids, ok := m[view]
	if !ok {
		return false
	}
	for i, v := range [2]Points{p.X, p.Y} {
		if n := a.value(ids[i]); n != nil {
			a.interrupt(n)
			n.value = float64(v)
		}
	}
	a.touch()
	return true
}

// Forget drops what is bound to a deleted view.
func (a *Animator) Forget(view NodeID) {
	delete(a.views, view)
	delete(a.pans, view)
	delete(a.scrolls, view)
	delete(a.changed, view)
}

// eval returns n's output in the current generation.
func (a *Animator) eval(n *anode) (float64, Color) {
	if n.gen == a.gen && n.gen != 0 {
		return n.num, n.color
	}
	n.gen = a.gen
	switch n.kind {
	case AnimCreateValue:
		n.num = n.value + n.offset
	case AnimCreateInterpolate:
		in := 0.0
		if src := a.nodes[n.op.Input]; src != nil {
			in, _ = a.eval(src)
		}
		n.num, n.color = n.op.Interpolation.At(in)
	case AnimCreateMath:
		x, y := a.operand(n.op.A), a.operand(n.op.B)
		switch n.op.Math {
		case MathAdd:
			n.num = x + y
		case MathSub:
			n.num = x - y
		case MathMul:
			n.num = x * y
		case MathDiv:
			n.num = 0
			if y != 0 {
				n.num = x / y
			}
		case MathMod:
			n.num = 0
			if y != 0 {
				n.num = math.Mod(math.Mod(x, y)+y, y)
			}
		}
	case AnimCreateDiffClamp:
		in := 0.0
		if src := a.nodes[n.op.Input]; src != nil {
			in, _ = a.eval(src)
		}
		if !n.primed {
			n.primed = true
			n.last = min(max(in, n.op.Min), n.op.Max)
		} else {
			n.last = min(max(n.last+in-n.lastIn, n.op.Min), n.op.Max)
		}
		n.lastIn = in
		n.num = n.last
	}
	return n.num, n.color
}

func (a *Animator) operand(o Operand) float64 {
	if o.Node == 0 {
		return o.Const
	}
	if n := a.nodes[o.Node]; n != nil {
		v, _ := a.eval(n)
		return v
	}
	return 0
}

// At maps x through the interpolation: a number, and a color when it has
// Colors.
func (r *Interpolation) At(x float64) (float64, Color) {
	in := r.Input
	k := len(in)
	if k == 0 {
		return x, 0
	}
	if k == 1 {
		return r.output(0, 0, 0)
	}
	i := 0
	for i < k-2 && x > in[i+1] {
		i++
	}
	lo, hi := in[i], in[i+1]
	var t float64
	if hi != lo {
		t = (x - lo) / (hi - lo)
	}
	switch {
	case x < in[0]:
		switch cmp.Or(r.ExtrapolateLeft, r.Extrapolate) {
		case ExtrapolateIdentity:
			return x, 0
		case 0, ExtrapolateClamp:
			t = 0
		}
	case x > in[k-1]:
		switch cmp.Or(r.ExtrapolateRight, r.Extrapolate) {
		case ExtrapolateIdentity:
			return x, 0
		case 0, ExtrapolateClamp:
			t = 1
		}
	}
	if t >= 0 && t <= 1 {
		t = r.Easing.At(t)
	}
	return r.output(i, i+1, t)
}

func (r *Interpolation) output(i, j int, t float64) (float64, Color) {
	if len(r.Colors) > j {
		return t, lerpColor(r.Colors[i], r.Colors[j], t)
	}
	if len(r.Output) > j {
		return r.Output[i] + (r.Output[j]-r.Output[i])*t, 0
	}
	return 0, 0
}

func lerpColor(a, b Color, t float64) Color {
	t = min(max(t, 0), 1)
	var out Color
	for shift := 0; shift < 32; shift += 8 {
		x, y := float64(a>>shift&0xff), float64(b>>shift&0xff)
		out |= Color(math.Round(x+(y-x)*t)) << shift
	}
	return out
}

// At returns the eased progress at time fraction x from 0 to 1.
func (e Easing) At(x float64) float64 {
	if e.Kind == EasingBezier {
		return e.Bezier.At(x)
	}
	switch e.Mode {
	case EaseModeOut:
		return 1 - e.in(1-x)
	case EaseModeInOut:
		if x < 0.5 {
			return e.in(x*2) / 2
		}
		return 1 - e.in((1-x)*2)/2
	}
	return e.in(x)
}

// in is the curve's "in" form, React Native's Easing functions.
func (e Easing) in(t float64) float64 {
	switch e.Kind {
	case EasingQuad:
		return t * t
	case EasingCubic:
		return t * t * t
	case EasingPoly:
		n := e.Param
		if n == 0 {
			n = 2
		}
		return math.Pow(t, n)
	case EasingSin:
		return 1 - math.Cos(t*math.Pi/2)
	case EasingCircle:
		return 1 - math.Sqrt(max(1-t*t, 0))
	case EasingExp:
		if t <= 0 {
			return 0
		}
		return math.Pow(2, 10*(t-1))
	case EasingElastic:
		b := e.Param
		if b == 0 {
			b = 1
		}
		p := b * math.Pi
		return 1 - math.Pow(math.Cos(t*math.Pi/2), 3)*math.Cos(t*p)
	case EasingBack:
		s := e.Param
		if s == 0 {
			s = 1.70158
		}
		return t * t * ((s+1)*t - s)
	case EasingBounce:
		return bounce(t)
	}
	return t
}

func bounce(t float64) float64 {
	switch {
	case t < 1/2.75:
		return 7.5625 * t * t
	case t < 2/2.75:
		t -= 1.5 / 2.75
		return 7.5625*t*t + 0.75
	case t < 2.5/2.75:
		t -= 2.25 / 2.75
		return 7.5625*t*t + 0.9375
	}
	t -= 2.625 / 2.75
	return 7.5625*t*t + 0.984375
}

// Runs and their steps.

type arun struct {
	id   uint32
	root astep
}

// astep is a node of a running animation's tree.
type astep interface {
	// start begins at time at, in seconds, or on the next step when at
	// is negative.
	start(a *Animator, at float64)
	// step advances to now and reports whether the step is done and when
	// it ended.
	step(a *Animator, now float64) (done bool, end float64)
	// stop releases the values it moves.
	stop(a *Animator)
	// bind gives the leaves their run, so an interruption ends it.
	bind(r *arun)
	// values lists the values the step moves, for loops to reset.
	values(out []uint32) []uint32
}

func (a *Animator) build(an *Animation) astep {
	switch an.Kind {
	case AnimationTiming, AnimationSpring, AnimationDecay:
		return &aleaf{spec: an}
	case AnimationDelay:
		return &adelay{d: an.Delay.Seconds()}
	case AnimationSequence:
		s := &aseq{}
		for _, c := range an.Steps {
			s.steps = append(s.steps, a.build(c))
		}
		return s
	case AnimationParallel, AnimationStagger:
		p := &apar{}
		for i, c := range an.Steps {
			var st astep = a.build(c)
			if an.Kind == AnimationStagger && i > 0 {
				st = &aseq{steps: []astep{&adelay{d: an.Delay.Seconds() * float64(i)}, st}}
			}
			p.steps = append(p.steps, st)
		}
		return p
	case AnimationLoop:
		l := &aloop{n: an.Iterations}
		if len(an.Steps) > 0 {
			l.body = a.build(an.Steps[0])
		}
		return l
	}
	return &adelay{}
}

// aleaf moves one value.
type aleaf struct {
	spec    *Animation
	run     *arun
	node    *anode
	from    float64
	vel     float64 // initial velocity, units per second
	t0      float64
	started bool // has its start time
	// decay segments: a bounce starts a new one at a bound.
	segFrom, segVel, segT0 float64
	done                   bool
}

func (l *aleaf) bind(r *arun) { l.run = r }

func (l *aleaf) values(out []uint32) []uint32 { return append(out, l.spec.Value) }

func (l *aleaf) start(a *Animator, at float64) {
	n := a.value(l.spec.Value)
	l.node, l.done = n, false
	if n == nil {
		l.done = true
		return
	}
	if n.driver != nil && n.driver != l {
		a.interrupt(n)
	}
	n.driver = l
	l.from = n.value
	l.vel = n.vel
	switch l.spec.Kind {
	case AnimationSpring:
		if v := l.spec.Spring.Velocity; v != 0 {
			l.vel = v
		}
	case AnimationDecay:
		if v := l.spec.Decay.Velocity; v != 0 {
			l.vel = v
		}
	}
	l.t0, l.started = at, at >= 0
	l.segFrom, l.segVel, l.segT0 = l.from, l.vel, at
}

func (l *aleaf) stop(a *Animator) {
	if l.node != nil && l.node.driver == l {
		l.node.driver = nil
		l.node.vel = 0
	}
}

func (l *aleaf) step(a *Animator, now float64) (bool, float64) {
	n := l.node
	if l.done || n == nil {
		return true, now
	}
	if n.driver != l {
		// Something else took the value: the run was ended already.
		return true, now
	}
	if !l.started {
		l.t0, l.segT0, l.started = now, now, true
	}
	var v, vel float64
	var done bool
	var end float64
	switch l.spec.Kind {
	case AnimationTiming:
		v, done, end = l.timing(now)
	case AnimationSpring:
		v, vel, done = l.spring(now)
		end = now
	case AnimationDecay:
		v, vel, done = l.decay(now)
		end = now
	}
	n.value, n.vel = v, vel
	a.touch()
	if done {
		l.done = true
		n.driver = nil
		n.vel = 0
	}
	return done, end
}

func (l *aleaf) timing(now float64) (float64, bool, float64) {
	c := &l.spec.Timing
	d, delay := c.Duration.Seconds(), c.Delay.Seconds()
	end := l.t0 + delay + d
	// Steps of a composite start at exact ends; a nanosecond's slack
	// keeps float error from costing a frame.
	if now >= end-1e-9 {
		return l.spec.To, true, end
	}
	x := 1.0
	if d > 0 {
		x = (now - l.t0 - delay) / d
	}
	if x <= 0 {
		return l.from, false, end
	}
	return l.from + (l.spec.To-l.from)*c.Easing.At(x), false, end
}

// spring is React Native's closed-form damped spring.
func (l *aleaf) spring(now float64) (pos, vel float64, done bool) {
	c := &l.spec.Spring
	k, damp, m := c.Stiffness, c.Damping, c.Mass
	to := l.spec.To
	t := now - l.t0
	v0 := -l.vel
	zeta := damp / (2 * math.Sqrt(k*m))
	w0 := math.Sqrt(k / m)
	x0 := to - l.from
	if zeta < 1 {
		w1 := w0 * math.Sqrt(1-zeta*zeta)
		env := math.Exp(-zeta * w0 * t)
		sin, cos := math.Sin(w1*t), math.Cos(w1*t)
		pos = to - env*((v0+zeta*w0*x0)/w1*sin+x0*cos)
		vel = zeta*w0*env*(sin*(v0+zeta*w0*x0)/w1+x0*cos) - env*(cos*(v0+zeta*w0*x0)-w1*x0*sin)
	} else {
		env := math.Exp(-w0 * t)
		pos = to - env*(x0+(v0+w0*x0)*t)
		vel = env * (v0*(t*w0-1) + t*x0*w0*w0)
	}
	const rest = 0.001
	if math.Abs(vel) <= rest && math.Abs(to-pos) <= rest || t > 60 {
		return to, 0, true
	}
	return pos, vel, false
}

// decay slows the value exponentially, bouncing off or stopping at its
// bounds.
func (l *aleaf) decay(now float64) (pos, vel float64, done bool) {
	c := &l.spec.Decay
	k := 1 - c.Deceleration  // per millisecond
	const restSpeed = 0.0005 // units per millisecond
	for range 16 {
		t := (now - l.segT0) * 1000
		v := l.segVel / 1000
		pos = l.segFrom + v/k*(1-math.Exp(-k*t))
		speed := v * math.Exp(-k*t)
		if c.Clamp && (pos < c.Min || pos > c.Max) {
			bound := c.Max
			if pos < c.Min {
				bound = c.Min
			}
			// When it got there: x(t) = bound.
			f := 1 - (bound-l.segFrom)*k/v
			if f <= 0 || f >= 1 || c.Bounce <= 0 {
				return bound, 0, true
			}
			tb := -math.Log(f) / k
			hit := v * math.Exp(-k*tb)
			l.segFrom, l.segVel, l.segT0 = bound, -hit*c.Bounce*1000, l.segT0+tb/1000
			continue
		}
		if math.Abs(speed) < restSpeed {
			return pos, 0, true
		}
		return pos, speed * 1000, false
	}
	return pos, 0, true
}

// adelay waits.
type adelay struct {
	d, t0   float64
	started bool
}

func (d *adelay) start(a *Animator, at float64) { d.t0, d.started = at, at >= 0 }
func (d *adelay) stop(*Animator)                {}
func (d *adelay) bind(*arun)                    {}
func (d *adelay) values(out []uint32) []uint32  { return out }
func (d *adelay) step(a *Animator, now float64) (bool, float64) {
	if !d.started {
		d.t0, d.started = now, true
	}
	end := d.t0 + d.d
	return now >= end-1e-9, end
}

// aseq runs its steps in turn, each from the exact end of the last.
type aseq struct {
	steps []astep
	i     int
}

func (s *aseq) start(a *Animator, at float64) {
	s.i = 0
	if len(s.steps) > 0 {
		s.steps[0].start(a, at)
	}
}

func (s *aseq) stop(a *Animator) {
	if s.i < len(s.steps) {
		s.steps[s.i].stop(a)
	}
}

func (s *aseq) bind(r *arun) {
	for _, st := range s.steps {
		st.bind(r)
	}
}

func (s *aseq) values(out []uint32) []uint32 {
	for _, st := range s.steps {
		out = st.values(out)
	}
	return out
}

func (s *aseq) step(a *Animator, now float64) (bool, float64) {
	end := now
	for s.i < len(s.steps) {
		done, e := s.steps[s.i].step(a, now)
		if !done {
			return false, now
		}
		end = e
		s.i++
		if s.i < len(s.steps) {
			s.steps[s.i].start(a, e)
		}
	}
	return true, end
}

// apar runs its steps at once.
type apar struct {
	steps []astep
	done  []bool
}

func (p *apar) start(a *Animator, at float64) {
	p.done = make([]bool, len(p.steps))
	for _, st := range p.steps {
		st.start(a, at)
	}
}

func (p *apar) stop(a *Animator) {
	for i, st := range p.steps {
		if !p.done[i] {
			st.stop(a)
		}
	}
}

func (p *apar) bind(r *arun) {
	for _, st := range p.steps {
		st.bind(r)
	}
}

func (p *apar) values(out []uint32) []uint32 {
	for _, st := range p.steps {
		out = st.values(out)
	}
	return out
}

func (p *apar) step(a *Animator, now float64) (bool, float64) {
	all, end := true, 0.0
	for i, st := range p.steps {
		if p.done[i] {
			continue
		}
		done, e := st.step(a, now)
		if done {
			p.done[i] = true
			end = max(end, e)
		} else {
			all = false
		}
	}
	if !all {
		return false, now
	}
	if end == 0 {
		end = now
	}
	return true, end
}

// aloop runs its body again, its values put back where they started.
type aloop struct {
	body    astep
	n, i    int
	initial []float64
	ids     []uint32
}

func (l *aloop) startAt(a *Animator, at float64) {
	for k, id := range l.ids {
		if n := a.value(id); n != nil {
			n.value = l.initial[k]
		}
	}
	a.touch()
	l.body.start(a, at)
}

func (l *aloop) start(a *Animator, at float64) {
	l.i = 0
	if l.body == nil {
		return
	}
	l.ids = l.body.values(l.ids[:0])
	l.initial = l.initial[:0]
	for _, id := range l.ids {
		v := 0.0
		if n := a.value(id); n != nil {
			v = n.value
		}
		l.initial = append(l.initial, v)
	}
	l.body.start(a, at)
}

func (l *aloop) stop(a *Animator) {
	if l.body != nil {
		l.body.stop(a)
	}
}

func (l *aloop) bind(r *arun) {
	if l.body != nil {
		l.body.bind(r)
	}
}

func (l *aloop) values(out []uint32) []uint32 {
	if l.body == nil {
		return out
	}
	return l.body.values(out)
}

func (l *aloop) step(a *Animator, now float64) (bool, float64) {
	if l.body == nil {
		return true, now
	}
	// A body that ends within this frame starts again at its end, up to
	// a bound so a zero-length body cannot spin.
	for range 64 {
		done, end := l.body.step(a, now)
		if !done {
			return false, now
		}
		l.i++
		if l.n > 0 && l.i >= l.n {
			return true, end
		}
		l.startAt(a, end)
		if end >= now {
			return false, now
		}
	}
	return false, now
}

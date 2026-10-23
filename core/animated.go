package core

import "time"

// Animated values follow NSH's native animation graph (React Native's
// Animated with its native driver): the app builds values, derived nodes
// and animations once, and the platform evaluates them every frame on its
// main thread, so a running animation costs the engine nothing per frame.
// The graph travels in ItemAnimation; each backend runs an Animator.

// AnimatedNode is a value of the animation graph: an AnimatedValue or a
// node derived from others.
type AnimatedNode interface {
	animated() *animNode
}

type animNode struct {
	t  *Tree
	id uint32
}

func (n *animNode) animated() *animNode { return n }

// AnimatedValue is a number the platform animates. Bind it to a view
// through Style.Animated, or derive other nodes from it.
type AnimatedValue struct{ animNode }

// AnimatedXY are the values a drag or a scroll view moves.
type AnimatedXY struct{ X, Y *AnimatedValue }

// NewAnimatedValue returns a value starting at v, released with the
// current owner.
func NewAnimatedValue(cx *Scope, v float64) *AnimatedValue {
	t := cx.rt.tree
	a := &AnimatedValue{animNode{t: t, id: t.anim.node()}}
	t.anim.add(AnimationOp{Kind: AnimCreateValue, Node: a.id, Value: v})
	t.anim.own(cx, &a.animNode)
	return a
}

// Set jumps the value to v, stopping its animation.
func (v *AnimatedValue) Set(x float64) { v.op(AnimationOp{Kind: AnimSet, Value: x}) }

// SetOffset sets what is added to the value, as a drag's start.
func (v *AnimatedValue) SetOffset(x float64) { v.op(AnimationOp{Kind: AnimSetOffset, Value: x}) }

// FlattenOffset adds the offset into the value and resets it to 0.
func (v *AnimatedValue) FlattenOffset() { v.op(AnimationOp{Kind: AnimFlattenOffset}) }

// ExtractOffset moves the value into the offset and resets it to 0.
func (v *AnimatedValue) ExtractOffset() { v.op(AnimationOp{Kind: AnimExtractOffset}) }

// Stop stops the value's animation where it is.
func (v *AnimatedValue) Stop() { v.op(AnimationOp{Kind: AnimStopValue}) }

// Read asks the platform for the value, with its offset, and passes it to
// fn under the current owner.
func (v *AnimatedValue) Read(cx *Scope, fn func(float64)) {
	id := v.t.anim.wait(cx, func(e Event) { fn(float64(e.Value)) })
	v.op(AnimationOp{Kind: AnimRead, ID: id})
}

func (v *AnimatedValue) op(o AnimationOp) {
	o.Node = v.id
	v.t.anim.add(o)
}

// Interpolation maps a node through piecewise-linear ranges.
type Interpolation struct {
	// Input is increasing; Output (numbers) or Colors has one entry for
	// each.
	Input  []float64
	Output []float64
	Colors []Color
	// ExtrapolateLeft and ExtrapolateRight say what happens outside
	// Input; Extrapolate sets both.
	Extrapolate, ExtrapolateLeft, ExtrapolateRight Extrapolate
	// Easing curves each segment.
	Easing Easing
}

// Extrapolate is what an interpolation does outside its input range; 0
// clamps.
type Extrapolate uint8

const (
	// ExtrapolateClamp keeps the first or last output.
	ExtrapolateClamp Extrapolate = iota + 1
	// ExtrapolateExtend continues the first or last segment.
	ExtrapolateExtend
	// ExtrapolateIdentity returns the input unchanged.
	ExtrapolateIdentity
)

// Interpolate returns a node that maps in through r. Its output is a
// color when r has Colors.
func Interpolate(in AnimatedNode, r Interpolation) AnimatedNode {
	src := in.animated()
	return src.t.anim.derive(src, AnimationOp{Kind: AnimCreateInterpolate, Input: src.id, Interpolation: &r})
}

// DiffClamp follows the changes of in but stays within min and max, as a
// header that hides as content scrolls up and shows as it scrolls down.
func DiffClamp(in AnimatedNode, min, max float64) AnimatedNode {
	src := in.animated()
	return src.t.anim.derive(src, AnimationOp{Kind: AnimCreateDiffClamp, Input: src.id, Min: min, Max: max})
}

// MathOp is an arithmetic node's operation.
type MathOp uint8

const (
	MathAdd MathOp = iota + 1
	MathSub
	MathMul
	// MathDiv is 0 when b is 0.
	MathDiv
	// MathMod is never negative for a positive b.
	MathMod
)

// Operand is an arithmetic node's operand: another node, or Const when
// Node is 0.
type Operand struct {
	Node  uint32
	Const float64
}

// Constant is a fixed operand for Add, Sub, Mul, Div and Mod.
type Constant float64

func (Constant) animated() *animNode { return nil }

func operand(n AnimatedNode) (Operand, *animNode) {
	if c, ok := n.(Constant); ok {
		return Operand{Const: float64(c)}, nil
	}
	a := n.animated()
	return Operand{Node: a.id}, a
}

func mathNode(op MathOp, a, b AnimatedNode) AnimatedNode {
	oa, na := operand(a)
	ob, nb := operand(b)
	src := na
	if src == nil {
		src = nb
	}
	if src == nil {
		panic("core: an animated math node needs a node operand")
	}
	return src.t.anim.derive(src, AnimationOp{Kind: AnimCreateMath, Math: op, A: oa, B: ob})
}

// Add returns a + b. One of them may be a Constant; so for Sub, Mul, Div
// and Mod.
func Add(a, b AnimatedNode) AnimatedNode { return mathNode(MathAdd, a, b) }
func Sub(a, b AnimatedNode) AnimatedNode { return mathNode(MathSub, a, b) }
func Mul(a, b AnimatedNode) AnimatedNode { return mathNode(MathMul, a, b) }
func Div(a, b AnimatedNode) AnimatedNode { return mathNode(MathDiv, a, b) }
func Mod(a, b AnimatedNode) AnimatedNode { return mathNode(MathMod, a, b) }

// AnimatedStyle binds a view's properties to animated nodes, over its
// style. The platform writes them every frame they change.
type AnimatedStyle struct {
	Opacity                AnimatedNode
	TranslateX, TranslateY AnimatedNode
	Scale, ScaleX, ScaleY  AnimatedNode
	// Rotate is in degrees, clockwise.
	Rotate AnimatedNode
	// Background, BorderColor and ShadowColor take color
	// interpolations.
	Background, BorderColor, ShadowColor AnimatedNode
	ShadowRadius                         AnimatedNode
}

// AnimatedProp is a view property an animated node drives.
type AnimatedProp uint8

const (
	PropOpacity AnimatedProp = iota
	PropTranslateX
	PropTranslateY
	PropScale
	PropScaleX
	PropScaleY
	PropRotate
	PropBackground
	PropBorderColor
	PropShadowColor
	PropShadowRadius
	propCount
)

func (s *AnimatedStyle) nodes() [propCount]uint32 {
	var out [propCount]uint32
	if s == nil {
		return out
	}
	for i, n := range [...]AnimatedNode{s.Opacity, s.TranslateX, s.TranslateY, s.Scale, s.ScaleX, s.ScaleY,
		s.Rotate, s.Background, s.BorderColor, s.ShadowColor, s.ShadowRadius} {
		if n != nil {
			if a := n.animated(); a != nil {
				out[i] = a.id
			}
		}
	}
	return out
}

// Easing is how a timing animation or an interpolation segment
// progresses: a cubic Bézier, or one of React Native's curves. Its zero
// value is linear.
type Easing struct {
	Kind EasingKind
	// Mode turns a curve around; Bézier curves ignore it.
	Mode EaseMode
	// Bezier is EasingBezier's curve.
	Bezier Curve
	// Param is Poly's exponent, Elastic's bounciness and Back's
	// overshoot; 0 takes React Native's default.
	Param float64
}

// EasingKind is an easing's shape.
type EasingKind uint8

const (
	EasingBezier EasingKind = iota
	EasingQuad
	EasingCubic
	EasingPoly
	EasingSin
	EasingCircle
	EasingExp
	EasingElastic
	EasingBack
	EasingBounce
)

// EaseMode applies a curve as is, mirrored, or both halves.
type EaseMode uint8

const (
	EaseModeIn EaseMode = iota
	EaseModeOut
	EaseModeInOut
)

// BezierEasing returns the easing of c.
func BezierEasing(c Curve) Easing { return Easing{Bezier: c} }

// Animation is a description of how values move: Timing, Spring and
// Decay move one value, and Sequence, Parallel, Stagger, Delay and Loop
// compose them. StartAnimation runs it on the platform.
type Animation struct {
	Kind AnimationKind
	// Value is the node a Timing, Spring or Decay moves; To where Timing
	// and Spring take it.
	Value  uint32
	To     float64
	Timing TimingConfig
	Spring SpringConfig
	Decay  DecayConfig
	// Steps are a composite's animations.
	Steps []*Animation
	// Delay is Delay's wait and Stagger's interval.
	Delay time.Duration
	// Iterations is Loop's count; 0 repeats forever.
	Iterations int
	t          *Tree
}

// AnimationKind is what an Animation does.
type AnimationKind uint8

const (
	AnimationTiming AnimationKind = iota + 1
	AnimationSpring
	AnimationDecay
	AnimationSequence
	AnimationParallel
	AnimationStagger
	AnimationDelay
	AnimationLoop
)

// TimingConfig is a timing animation's: Duration (500 ms when 0) along
// Easing, after Delay.
type TimingConfig struct {
	Duration time.Duration
	Easing   Easing
	Delay    time.Duration
}

// SpringConfig is a damped spring's: Stiffness 100, Damping 10 and Mass 1
// when 0. Velocity, in units per second, replaces the value's current
// velocity when it is not 0.
type SpringConfig struct {
	Stiffness, Damping, Mass float64
	Velocity                 float64
}

// DecayConfig slows a value from Velocity (units per second; the value's
// current velocity when 0) by Deceleration per millisecond (0.998 when
// 0). With Clamp it stays within Min and Max: it stops at a bound, or
// bounces off it keeping Bounce of its speed.
type DecayConfig struct {
	Velocity     float64
	Deceleration float64
	Clamp        bool
	Min, Max     float64
	Bounce       float64
}

// Timing moves v to to over c.Duration.
func Timing(v *AnimatedValue, to float64, c TimingConfig) *Animation {
	if c.Duration == 0 {
		c.Duration = 500 * time.Millisecond
	}
	return &Animation{Kind: AnimationTiming, Value: v.id, To: to, Timing: c, t: v.t}
}

// Spring moves v to to on a damped spring.
func Spring(v *AnimatedValue, to float64, c SpringConfig) *Animation {
	if c.Stiffness <= 0 {
		c.Stiffness = 100
	}
	if c.Damping <= 0 {
		c.Damping = 10
	}
	if c.Mass <= 0 {
		c.Mass = 1
	}
	return &Animation{Kind: AnimationSpring, Value: v.id, To: to, Spring: c, t: v.t}
}

// Decay slows v down from a velocity, as a fling.
func Decay(v *AnimatedValue, c DecayConfig) *Animation {
	if c.Deceleration <= 0 || c.Deceleration >= 1 {
		c.Deceleration = 0.998
	}
	return &Animation{Kind: AnimationDecay, Value: v.id, Decay: c, t: v.t}
}

// Sequence runs steps one after another, each starting when the one
// before ends.
func Sequence(steps ...*Animation) *Animation {
	return &Animation{Kind: AnimationSequence, Steps: steps, t: treeOf(steps)}
}

// Parallel runs steps together and ends when all have.
func Parallel(steps ...*Animation) *Animation {
	return &Animation{Kind: AnimationParallel, Steps: steps, t: treeOf(steps)}
}

// Stagger runs steps together, each starting interval after the one
// before.
func Stagger(interval time.Duration, steps ...*Animation) *Animation {
	return &Animation{Kind: AnimationStagger, Steps: steps, Delay: interval, t: treeOf(steps)}
}

// Delay waits for d, as a step of a composite.
func Delay(d time.Duration) *Animation { return &Animation{Kind: AnimationDelay, Delay: d} }

// Loop runs a again each time it ends, with its values back at their
// start, iterations times; forever when 0.
func Loop(a *Animation, iterations int) *Animation {
	return &Animation{Kind: AnimationLoop, Steps: []*Animation{a}, Iterations: iterations, t: a.t}
}

func treeOf(steps []*Animation) *Tree {
	for _, s := range steps {
		if s.t != nil {
			return s.t
		}
	}
	return nil
}

// StartAnimation runs a on the platform and calls done, if not nil,
// under the current owner with whether it ran to its end. Stopping it, a
// value's Set or Stop, or another animation of one of its values ends it
// early. The owner going stops it.
func (cx *Scope) StartAnimation(a *Animation, done func(finished bool)) (stop func()) {
	t := cx.rt.tree
	ended := false
	id := t.anim.wait(cx, func(e Event) {
		ended = true
		if done != nil {
			done(e.Finished)
		}
	})
	t.anim.add(AnimationOp{Kind: AnimStart, ID: id, Animation: a})
	stop = func() {
		if !ended {
			ended = true
			t.anim.add(AnimationOp{Kind: AnimStop, ID: id})
		}
	}
	cx.current().addCleanup(stop)
	return stop
}

// AnimationOpKind is a change to the animation graph.
type AnimationOpKind uint8

const (
	// AnimCreateValue makes value Node at Value.
	AnimCreateValue AnimationOpKind = iota + 1
	// AnimCreateInterpolate makes Node map Input through Interpolation.
	AnimCreateInterpolate
	// AnimCreateMath makes Node combine A and B with Math.
	AnimCreateMath
	// AnimCreateDiffClamp makes Node follow Input's changes within Min
	// and Max.
	AnimCreateDiffClamp
	// AnimSet, AnimSetOffset, AnimFlattenOffset, AnimExtractOffset and
	// AnimStopValue change value Node.
	AnimSet
	AnimSetOffset
	AnimFlattenOffset
	AnimExtractOffset
	AnimStopValue
	// AnimRead answers ID with value Node's value in an EventAnimation.
	AnimRead
	// AnimStart runs Animation as ID; its end is an EventAnimation.
	AnimStart
	// AnimStop ends run ID early.
	AnimStop
	// AnimBind makes Node drive Prop of View; Node 0 unbinds it.
	AnimBind
	// AnimBindPan has drags on View set values X and Y to their
	// translation, and AnimBindScroll has scroll view View set them to its
	// offset.
	AnimBindPan
	AnimBindScroll
	// AnimRelease drops Node.
	AnimRelease
)

// AnimationOp is one change to the animation graph.
type AnimationOp struct {
	Kind          AnimationOpKind
	Node          uint32
	Value         float64
	Input         uint32
	Interpolation *Interpolation
	Math          MathOp
	A, B          Operand
	Min, Max      float64
	// ID names a run or a read.
	ID        uint32
	Animation *Animation
	View      NodeID
	Prop      AnimatedProp
	X, Y      uint32
}

// animations is the engine's side of the graph: node and request IDs,
// the ops for the next batch and those waiting for an answer.
type animations struct {
	t               *Tree
	nodes, requests uint32
	ops             []AnimationOp
	waits           map[uint32]animWait
}

type animWait struct {
	owner *Scope
	fn    func(Event)
}

func (a *animations) node() uint32 { a.nodes++; return a.nodes }

func (a *animations) add(o AnimationOp) {
	a.ops = append(a.ops, o)
	a.t.changed = true
}

// own releases n when cx goes.
func (a *animations) own(cx *Scope, n *animNode) {
	cx.current().addCleanup(func() { a.add(AnimationOp{Kind: AnimRelease, Node: n.id}) })
}

// derive makes a node with op from src, owned by the running owner, else
// by the graph's root.
func (a *animations) derive(src *animNode, op AnimationOp) AnimatedNode {
	t := src.t
	n := &animNode{t: t, id: a.node()}
	op.Node = n.id
	a.add(op)
	owner := t.rt.owner
	if owner == nil {
		owner = t.root
	}
	a.own(owner, n)
	return n
}

// wait registers fn for the EventAnimation answering a new request.
func (a *animations) wait(cx *Scope, fn func(Event)) uint32 {
	a.requests++
	if a.waits == nil {
		a.waits = map[uint32]animWait{}
	}
	a.waits[a.requests] = animWait{owner: cx.current(), fn: fn}
	return a.requests
}

// animationEvent answers a request.
func (t *Tree) animationEvent(ev Event) {
	w, ok := t.anim.waits[ev.Count]
	if !ok {
		return
	}
	delete(t.anim.waits, ev.Count)
	if !w.owner.Disposed() {
		t.within(w.owner, func() { w.fn(ev) })
	}
}

// emitAnimation sends the views' bindings and the graph's changes.
func (t *Tree) emitAnimation() {
	if len(t.anim.ops) == 0 {
		return
	}
	t.items = append(t.items, MountItem{Kind: ItemAnimation, Animation: t.anim.ops})
	t.anim.ops = nil
}

// viewAnimation is what a view has bound.
type viewAnimation struct {
	sent [propCount]uint32
}

// bindAnimated sends the changes to n's bound properties.
func (t *Tree) bindAnimated(n *node) {
	want := n.style.Animated.nodes()
	if n.anim == nil {
		if want == ([propCount]uint32{}) {
			return
		}
		n.anim = &viewAnimation{}
	}
	for p, id := range want {
		if id != n.anim.sent[p] {
			n.anim.sent[p] = id
			t.anim.add(AnimationOp{Kind: AnimBind, View: n.id, Prop: AnimatedProp(p), Node: id})
		}
	}
}

func (t *Tree) bindPan(cx *Scope, n *node, xy AnimatedXY) {
	if xy.X == nil && xy.Y == nil {
		return
	}
	t.anim.add(AnimationOp{Kind: AnimBindPan, View: n.id, X: xy.idX(), Y: xy.idY()})
}

func (t *Tree) bindScroll(cx *Scope, n *node, xy AnimatedXY) {
	if xy.X == nil && xy.Y == nil {
		return
	}
	t.anim.add(AnimationOp{Kind: AnimBindScroll, View: n.id, X: xy.idX(), Y: xy.idY()})
}

func (xy AnimatedXY) idX() uint32 {
	if xy.X == nil {
		return 0
	}
	return xy.X.id
}

func (xy AnimatedXY) idY() uint32 {
	if xy.Y == nil {
		return 0
	}
	return xy.Y.id
}

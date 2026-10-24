package core

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func TestNativeSendsChangedPropsCommandsAndEvents(t *testing.T) {
	var ref Ref
	var zoom *Signal[float64]
	var events []string
	h := newHost(t, Size{Width: 300, Height: 400}, func(cx *Scope) Node {
		zoom = NewSignal(cx, 3.0)
		return Native{
			Component: "map",
			PropsFunc: func() map[string]any {
				p := map[string]any{"zoom": zoom.Get(), "city": "Oslo"}
				if zoom.Get() > 5 {
					delete(p, "city")
				}
				return p
			},
			Ref:     &ref,
			TestID:  "map",
			OnEvent: func(name string, data any) { events = append(events, name) },
		}
	})
	b := h.commit()
	var cfg *NativeConfig
	for _, it := range b.Items {
		if it.Kind == ItemNative {
			cfg = it.Native
		}
	}
	n := h.find("map")
	if n.kind != KindNative || cfg == nil || cfg.Component != "map" ||
		!reflect.DeepEqual(cfg.Props, map[string]any{"zoom": 3.0, "city": "Oslo"}) {
		t.Fatalf("kind %v, config %+v", n.kind, cfg)
	}
	if k := kinds(h.commit()); k[ItemNative] != 0 {
		t.Fatalf("resent unchanged props: %v", k)
	}
	zoom.Set(7)
	ref.Command("fly", []float64{1, 2})
	b = h.commit()
	var cmd *MountItem
	cfg = nil
	for i, it := range b.Items {
		switch it.Kind {
		case ItemNative:
			cfg = it.Native
		case ItemNativeCommand:
			cmd = &b.Items[i]
		}
	}
	if cfg == nil || !reflect.DeepEqual(cfg.Props, map[string]any{"zoom": 7.0, "city": nil}) {
		t.Fatalf("changes %+v", cfg)
	}
	if cmd == nil || cmd.Value != "fly" || !reflect.DeepEqual(cmd.Data, []float64{1, 2}) {
		t.Fatalf("command %+v", cmd)
	}

	h.tree.Dispatch(Event{Kind: EventNative, ID: n.id, Text: "moved"})
	h.tree.Dispatch(Event{Kind: EventNativeSize, ID: n.id, Size: Size{Width: 120, Height: 80}})
	h.commit()
	if len(events) != 1 || events[0] != "moved" {
		t.Fatalf("events %v", events)
	}
	// Stretched to the column's width, it keeps the ratio it asked for.
	if n.frame.Size != (Size{Width: 300, Height: 200}) {
		t.Fatalf("frame %v, want the ratio the component asked for", n.frame)
	}
}

func TestInputAccessoryViewIsADetachedRootNamedByInputs(t *testing.T) {
	h := newHost(t, Size{Width: 320, Height: 600}, func(cx *Scope) Node {
		return View{Children: []Node{
			Input{InputAccessoryViewID: "bar", TestID: "field"},
			InputAccessoryView{ID: "bar", TestID: "bar", Children: []Node{
				View{Style: Style{Height: Pt(44)}},
			}},
		}}
	})
	b := h.commit()
	bar := h.find("bar")
	var accessory NodeID
	created, inserted := -1, false
	for i, it := range b.Items {
		switch {
		case it.Kind == ItemCreate && it.ID == bar.id:
			created = i
		case it.Kind == ItemInsert && it.ID == bar.id:
			inserted = true
		case it.Kind == ItemInput:
			if created < 0 {
				t.Fatal("the input named its accessory before it was created")
			}
			accessory = it.Input.Accessory
		}
	}
	if accessory != bar.id || inserted {
		t.Fatalf("accessory %v (want %v), inserted %v", accessory, bar.id, inserted)
	}
	if bar.frame != (Bounds{Size: Size{Width: 320, Height: 44}}) {
		t.Fatalf("frame %v", bar.frame)
	}
	h.tree.Resize(Size{Width: 400, Height: 600})
	h.commit()
	if bar.frame.Size.Width != 400 {
		t.Fatalf("frame %v after a resize", bar.frame)
	}
}

func TestFileDropReachesHandlers(t *testing.T) {
	var got []string
	entered, left := 0, 0
	h := newHost(t, Size{Width: 300, Height: 300}, func(cx *Scope) Node {
		return View{TestID: "drop", OnFileDrop: func(uris []string) { got = uris },
			OnFileDragEnter: func() { entered++ }, OnFileDragLeave: func() { left++ }}
	})
	b := h.commit()
	n := h.find("drop")
	for _, it := range b.Items {
		if it.Kind == ItemEvents && it.ID == n.id {
			if !it.Events.Has(EventFileDrop) || !it.Events.Has(EventFileDragEnter) || !it.Events.Has(EventFileDragLeave) {
				t.Fatalf("mask %b", it.Events)
			}
		}
	}
	h.tree.Dispatch(Event{Kind: EventFileDragEnter, ID: n.id})
	h.tree.Dispatch(Event{Kind: EventFileDragLeave, ID: n.id})
	h.tree.Dispatch(Event{Kind: EventFileDrop, ID: n.id, URIs: []string{"file:///tmp/a%20b.txt"}})
	if entered != 1 || left != 1 || len(got) != 1 || got[0] != "file:///tmp/a%20b.txt" {
		t.Fatalf("enter %d leave %d drop %v", entered, left, got)
	}
}

func TestAnimatedValuesBindAndRunOnAnAnimator(t *testing.T) {
	var fade *AnimatedValue
	var finished []bool
	var cx0 *Scope
	h := newHost(t, Size{Width: 300, Height: 300}, func(cx *Scope) Node {
		cx0 = cx
		fade = NewAnimatedValue(cx, 0)
		slide := Interpolate(fade, Interpolation{Input: []float64{0, 1}, Output: []float64{100, 0}})
		color := Interpolate(fade, Interpolation{Input: []float64{0, 1}, Colors: []Color{RGB(0x000000), RGB(0xffffff)}})
		return View{TestID: "box", Style: Style{Width: Pt(10), Height: Pt(10),
			Animated: &AnimatedStyle{Opacity: fade, TranslateY: slide, Background: color}}}
	})
	var reports []Event
	a := NewAnimator(func(e Event) { reports = append(reports, e) })
	feed := func(b *MountBatch) {
		for _, it := range b.Items {
			if it.Kind == ItemAnimation {
				a.Apply(it.Animation)
			}
		}
	}
	feed(h.commit())
	box := h.find("box").id
	vals, ok := a.Values(box)
	if !ok || vals.Opacity != 0 || vals.TranslateY != 100 || vals.Background != RGB(0) {
		t.Fatalf("values %+v %v", vals, ok)
	}

	cx0.StartAnimation(Timing(fade, 1, TimingConfig{Duration: time.Second}), func(f bool) { finished = append(finished, f) })
	feed(h.commit())
	a.Step(10 * time.Second) // the first step starts it
	a.Step(10*time.Second + 500*time.Millisecond)
	vals, _ = a.Values(box)
	if math.Abs(float64(vals.Opacity)-0.5) > 1e-3 || math.Abs(float64(vals.TranslateY)-50) > 0.1 {
		t.Fatalf("halfway %+v", vals)
	}
	changed := 0
	a.Changed(func(id NodeID, v AnimatedValues) {
		if id == box {
			changed++
		}
	})
	if changed != 1 {
		t.Fatalf("changed %d", changed)
	}
	a.Step(11 * time.Second)
	if a.Running() || len(reports) != 1 || !reports[0].Finished {
		t.Fatalf("running %v reports %+v", a.Running(), reports)
	}
	for _, e := range reports {
		h.tree.Dispatch(e)
	}
	if len(finished) != 1 || !finished[0] {
		t.Fatalf("finished %v", finished)
	}
	vals, _ = a.Values(box)
	if vals.Opacity != 1 || vals.Background != RGB(0xffffff) {
		t.Fatalf("end %+v", vals)
	}
}

func TestAnimatorSpringDecaySequenceAndLoop(t *testing.T) {
	a := NewAnimator(func(Event) {})
	a.Apply([]AnimationOp{{Kind: AnimCreateValue, Node: 1}, {Kind: AnimCreateValue, Node: 2}})
	spring := &Animation{Kind: AnimationSpring, Value: 1, To: 10, Spring: SpringConfig{Stiffness: 100, Damping: 10, Mass: 1}}
	a.Apply([]AnimationOp{{Kind: AnimStart, ID: 1, Animation: spring}})
	over := false
	for i := range 400 {
		a.Step(time.Duration(i) * 16 * time.Millisecond)
		if a.nodes[1].value > 10.01 {
			over = true
		}
	}
	if !over || a.Running() || a.nodes[1].value != 10 {
		t.Fatalf("spring: overshot %v running %v value %v", over, a.Running(), a.nodes[1].value)
	}

	decay := &Animation{Kind: AnimationDecay, Value: 2, Decay: DecayConfig{Velocity: 1000, Deceleration: 0.998, Clamp: true, Min: 0, Max: 300}}
	a.Apply([]AnimationOp{{Kind: AnimStart, ID: 2, Animation: decay}})
	for i := range 200 {
		a.Step(time.Duration(i) * 16 * time.Millisecond)
	}
	if v := a.nodes[2].value; v != 300 || a.Running() {
		t.Fatalf("decay stopped at %v, want its bound", v)
	}

	// A sequence starts each step at the exact end of the one before, and
	// a loop puts its values back.
	a.Apply([]AnimationOp{{Kind: AnimSet, Node: 1, Value: 0}})
	seq := &Animation{Kind: AnimationLoop, Iterations: 2, Steps: []*Animation{{Kind: AnimationSequence, Steps: []*Animation{
		{Kind: AnimationTiming, Value: 1, To: 1, Timing: TimingConfig{Duration: 100 * time.Millisecond}},
		{Kind: AnimationTiming, Value: 1, To: 3, Timing: TimingConfig{Duration: 100 * time.Millisecond}},
	}}}}
	var done []Event
	a.report = func(e Event) { done = append(done, e) }
	a.Apply([]AnimationOp{{Kind: AnimStart, ID: 3, Animation: seq}})
	a.Step(0)
	a.Step(150 * time.Millisecond)
	if v := a.nodes[1].value; math.Abs(v-2) > 1e-9 {
		t.Fatalf("sequence at 150ms: %v, want 2", v)
	}
	a.Step(250 * time.Millisecond) // into the second iteration, from 0
	if v := a.nodes[1].value; math.Abs(v-0.5) > 1e-9 {
		t.Fatalf("loop at 250ms: %v, want 0.5", v)
	}
	a.Step(400 * time.Millisecond)
	if len(done) != 1 || !done[0].Finished || done[0].Count != 3 || a.nodes[1].value != 3 {
		t.Fatalf("loop ended %+v at %v", done, a.nodes[1].value)
	}

	// Setting a value ends its run unfinished.
	done = nil
	a.Apply([]AnimationOp{{Kind: AnimStart, ID: 4, Animation: &Animation{Kind: AnimationTiming, Value: 1, To: 0,
		Timing: TimingConfig{Duration: time.Second}}}})
	a.Step(time.Second)
	a.Apply([]AnimationOp{{Kind: AnimSet, Node: 1, Value: 5}})
	if len(done) != 1 || done[0].Finished || a.Running() {
		t.Fatalf("interrupted %+v", done)
	}
}

func TestInterpolationExtrapolates(t *testing.T) {
	r := Interpolation{Input: []float64{0, 1}, Output: []float64{0, 10}, Extrapolate: ExtrapolateExtend,
		ExtrapolateLeft: ExtrapolateClamp}
	if v, _ := r.At(-1); v != 0 {
		t.Fatalf("left %v, want clamped", v)
	}
	if v, _ := r.At(2); v != 20 {
		t.Fatalf("right %v, want extended", v)
	}
	if v, _ := (&Interpolation{Input: []float64{0, 1}, Output: []float64{0, 10}}).At(2); v != 10 {
		t.Fatalf("default %v, want clamped", v)
	}
}

func TestAnimatorPanAndMath(t *testing.T) {
	a := NewAnimator(func(Event) {})
	a.Apply([]AnimationOp{
		{Kind: AnimCreateValue, Node: 1},
		{Kind: AnimCreateMath, Node: 2, Math: MathMul, A: Operand{Node: 1}, B: Operand{Const: 2}},
		{Kind: AnimCreateDiffClamp, Node: 3, Input: 1, Min: 0, Max: 50},
		{Kind: AnimBindPan, View: 7, X: 1},
		{Kind: AnimBind, View: 7, Prop: PropTranslateX, Node: 2},
		{Kind: AnimBind, View: 7, Prop: PropTranslateY, Node: 3},
	})
	for i, x := range []Points{30, 100, 80} {
		phase := GestureChanged
		if i == 0 {
			phase = GestureBegan
		}
		if !a.Gesture(&Event{Kind: EventPan, ID: 7, Phase: phase, Point: Point{X: x}}) {
			t.Fatal("pan not bound")
		}
		a.Values(7)
	}
	v, _ := a.Values(7)
	if v.TranslateX != 160 || v.TranslateY != 30 {
		t.Fatalf("values %+v, want 160 and a diff clamp of 30", v)
	}
	vs := v.Apply(ViewStyle{Opacity: 1, Transform: Transform{Rotate: 9}})
	if vs.Transform.TranslateX != 160 || vs.Transform.Rotate != 9 || vs.Opacity != 1 {
		t.Fatalf("applied %+v", vs)
	}
}

func TestLayoutAnimationRidesTheNextBatch(t *testing.T) {
	var wide *Signal[bool]
	var cx0 *Scope
	h := newHost(t, Size{Width: 300, Height: 300}, func(cx *Scope) Node {
		cx0 = cx
		wide = NewSignal(cx, false)
		return View{TestID: "box", StyleFunc: func() Style {
			if wide.Get() {
				return Style{Width: Pt(200), Height: Pt(10)}
			}
			return Style{Width: Pt(100), Height: Pt(10)}
		}}
	})
	if b := h.commit(); b.LayoutAnimation != nil {
		t.Fatal("animated without asking")
	}
	cx0.AnimateLayout(LayoutSpringPreset)
	wide.Set(true)
	b := h.commit()
	if b.LayoutAnimation == nil || b.LayoutAnimation.Update.Curve != LayoutSpring {
		t.Fatalf("layout animation %+v", b.LayoutAnimation)
	}
	wide.Set(false)
	if b := h.commit(); b.LayoutAnimation != nil {
		t.Fatal("the animation applied twice")
	}
	tr := LayoutEaseInEaseOutPreset.Update
	if p, done := tr.At(150 * time.Millisecond); math.Abs(p-0.5) > 1e-3 || done {
		t.Fatalf("ease at half: %v %v", p, done)
	}
	over := false
	for i := range 70 {
		if p, _ := LayoutSpringPreset.Update.At(time.Duration(i) * 10 * time.Millisecond); p > 1 {
			over = true
		}
	}
	if p, done := LayoutSpringPreset.Update.At(700 * time.Millisecond); !over || p != 1 || !done {
		t.Fatalf("spring: overshot %v, end %v %v", over, p, done)
	}
}

func TestGesturesMoveTheirValuesFromWhereTheyWere(t *testing.T) {
	a := NewAnimator(func(Event) {})
	a.Apply([]AnimationOp{
		{Kind: AnimCreateValue, Node: 1, Value: 40},
		{Kind: AnimCreateValue, Node: 2, Value: 2},
		{Kind: AnimCreateValue, Node: 3, Value: 0.5},
		{Kind: AnimBindPan, View: 7, X: 1},
		{Kind: AnimBindPinch, View: 7, Node: 2},
		{Kind: AnimBindRotation, View: 7, Node: 3},
	})
	began := Event{Kind: EventPan, ID: 7, Phase: GestureBegan, Point: Point{X: 10}}
	a.Gesture(&began)
	if began.Values.X != 50 {
		t.Fatalf("pan start values = %v, want 40 grabbed plus 10", began.Values)
	}
	ended := Event{Kind: EventPan, ID: 7, Phase: GestureEnded, Point: Point{X: -15}}
	a.Gesture(&ended)
	if ended.Values.X != 25 {
		t.Fatalf("pan end values = %v, want 25", ended.Values)
	}
	a.Gesture(&Event{Kind: EventPinch, ID: 7, Phase: GestureBegan, Value: 1})
	a.Gesture(&Event{Kind: EventPinch, ID: 7, Phase: GestureChanged, Value: 1.5})
	a.Gesture(&Event{Kind: EventRotation, ID: 7, Phase: GestureBegan, Value: 0})
	a.Gesture(&Event{Kind: EventRotation, ID: 7, Phase: GestureChanged, Value: 0.25})
	if s, r := a.value(2).value, a.value(3).value; s != 3 || r != 0.75 {
		t.Fatalf("pinch value %v, rotation value %v; want 3 and 0.75", s, r)
	}
	if a.Gesture(&Event{Kind: EventPinch, ID: 8, Phase: GestureBegan}) {
		t.Fatal("a view with no value bound moved one")
	}
}

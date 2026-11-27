package core

import (
	"fmt"
	"reflect"
	goruntime "runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/layout"
)

func TestNodeFootprint(t *testing.T) {
	nodeSize := unsafe.Sizeof(node{})
	t.Logf("sizeof(node)              = %d B", nodeSize)
	t.Logf("  sizeof(Style)           = %d B", unsafe.Sizeof(Style{}))
	t.Logf("  sizeof(handlers)        = %d B", unsafe.Sizeof(handlers{}))
	t.Logf("  sizeof(driver.ViewProps)= %d B", unsafe.Sizeof(driver.ViewProps{}))
	t.Logf("  sizeof(driver.ViewStyle)= %d B", unsafe.Sizeof(driver.ViewStyle{}))
	t.Logf("  sizeof(driver.MountItem)= %d B", unsafe.Sizeof(driver.MountItem{}))
	t.Logf("  sizeof(sent)            = %d B", unsafe.Sizeof(sent{}))
	t.Logf("  sizeof(layout.State)    = %d B", unsafe.Sizeof(layout.State{}))
	t.Logf("  sizeof(layout.Style)    = %d B", unsafe.Sizeof(layout.Style{}))
	t.Logf("  sizeof(textFields)      = %d B", unsafe.Sizeof(textFields{}))
	t.Logf("  sizeof(PartStates)      = %d B", unsafe.Sizeof(PartStates{}))
	t.Logf("  sizeof(nodeClasses)     = %d B", unsafe.Sizeof(nodeClasses{}))
	t.Logf("  sizeof(nodeExtra)       = %d B", unsafe.Sizeof(nodeExtra{}))
	t.Logf("  sizeof(textNode)        = %d B", unsafe.Sizeof(textNode{}))

	// Regression guard: node used to be 6,488 B (two inline Styles + unpacked fields).
	const maxNodeBytes = 896
	if nodeSize > maxNodeBytes {
		t.Fatalf("sizeof(node) = %d B, want <= %d B", nodeSize, maxNodeBytes)
	}

	measureTree := func(name string, build func(cx *Scope, tick *Signal[int]) Node) float64 {
		t.Helper()
		goruntime.GC()
		goruntime.GC()
		var before goruntime.MemStats
		goruntime.ReadMemStats(&before)

		root := NewRoot(RootOptions{Post: func(func()) {}, Clock: NewManualClock(time.Unix(0, 0)), RequestFrame: func() {}})
		tree := NewTree(root, ApproxTextEngine{})
		tree.Resize(Size{Width: 400, Height: 800}, Geometry{})
		var tick *Signal[int]
		tree.Mount(func(cx *Scope) Node {
			tick = NewSignal(cx, 0)
			return build(cx, tick)
		})
		tree.Commit()
		// A tiny steady-state commit lets shrinkItems drop the first-commit peak buffer.
		tick.Set(1)
		tree.Commit()

		goruntime.GC()
		goruntime.GC()
		var after goruntime.MemStats
		goruntime.ReadMemStats(&after)

		nodeCount := 0
		for _, nd := range tree.nodes {
			if nd != nil && !nd.deleted {
				nodeCount++
			}
		}
		var delta int64
		if after.HeapAlloc >= before.HeapAlloc {
			delta = int64(after.HeapAlloc - before.HeapAlloc)
		}
		perNode := float64(delta) / float64(nodeCount)
		t.Logf("tree %-24s: %d nodes, items cap=%d, HeapAlloc delta = %d B (%.1f B/node)",
			name, nodeCount, cap(tree.items), delta, perNode)
		goruntime.KeepAlive(tree)
		root.Dispose()
		return perNode
	}

	mixedPerNode := measureTree("2000-mixed-rows", func(cx *Scope, tick *Signal[int]) Node {
		rowStyle := Style{Direction: Row, Padding: All(8), Gap: 8}
		boxStyle := Style{Width: Pt(20), Height: Pt(20), Background: RGB(0x3366ff), CornerRadius: 4}
		textStyle := Style{FontSize: 14}
		kids := make([]Node, 0, 668)
		kids = append(kids, Text{ContentFunc: func() string { return fmt.Sprintf("t=%d", tick.Get()) }})
		for i := range 666 {
			kids = append(kids, View{Style: rowStyle, Children: []Node{
				View{Style: boxStyle},
				Text{Content: fmt.Sprintf("row %d", i), Style: textStyle},
			}})
		}
		return View{Children: kids}
	})

	staticViewsPerNode := measureTree("2000-static-views", func(cx *Scope, tick *Signal[int]) Node {
		cardStyle := Style{
			Width: Pt(120), Height: Pt(44), Padding: All(8),
			Background: RGB(0x3366ff), CornerRadius: 8, BorderWidth: 1, BorderColor: RGB(0x2244aa),
		}
		kids := make([]Node, 0, 2001)
		kids = append(kids, Text{ContentFunc: func() string { return fmt.Sprintf("t=%d", tick.Get()) }})
		for range 2000 {
			kids = append(kids, View{Style: cardStyle})
		}
		return View{Children: kids}
	})

	emptyPerNode := measureTree("2000-empty-views", func(cx *Scope, tick *Signal[int]) Node {
		kids := make([]Node, 0, 2001)
		kids = append(kids, Text{ContentFunc: func() string { return fmt.Sprintf("t=%d", tick.Get()) }})
		for range 2000 {
			kids = append(kids, View{})
		}
		return View{Children: kids}
	})

	classedPerNode := measureTree("2000-classed-views", func(cx *Scope, tick *Signal[int]) Node {
		kids := make([]Node, 0, 2001)
		kids = append(kids, Text{ContentFunc: func() string { return fmt.Sprintf("t=%d", tick.Get()) }})
		for range 2000 {
			kids = append(kids, View{Class: "flex-row p-2 gap-2 bg-gray-100 rounded"})
		}
		return View{Children: kids}
	})

	reactivePerNode := measureTree("2000-reactive-views", func(cx *Scope, tick *Signal[int]) Node {
		sig := NewSignal(cx, 0)
		kids := make([]Node, 0, 2001)
		kids = append(kids, Text{ContentFunc: func() string { return fmt.Sprintf("t=%d", tick.Get()) }})
		for range 2000 {
			kids = append(kids, View{
				Style: Style{Width: Pt(20), Height: Pt(20)},
				StyleFunc: func() Style {
					return Style{Background: RGB(uint32(0x102030 + sig.Get()))}
				},
			})
		}
		return View{Children: kids}
	})

	// Regression guards well below the old ~6.5–15 KB/node footprint.
	if emptyPerNode > 1500 {
		t.Errorf("2000-empty-views = %.1f B/node, want <= 1500 B/node", emptyPerNode)
	}
	if staticViewsPerNode > 1500 {
		t.Errorf("2000-static-views = %.1f B/node, want <= 1500 B/node", staticViewsPerNode)
	}
	if mixedPerNode > 2600 {
		t.Errorf("2000-mixed-rows = %.1f B/node, want <= 2600 B/node", mixedPerNode)
	}
	if classedPerNode > 2500 {
		t.Errorf("2000-classed-views = %.1f B/node, want <= 2500 B/node", classedPerNode)
	}
	if reactivePerNode > 4500 {
		t.Errorf("2000-reactive-views = %.1f B/node, want <= 4500 B/node", reactivePerNode)
	}
}

func TestStaticStyleSharingAndIsolation(t *testing.T) {
	var disabledA *Signal[bool]
	var dynBG *Signal[Color]

	baseHover := &Style{Background: RGB(0x2244ff)}
	baseFocus := &Style{BorderColor: RGB(0x00cc88), BorderWidth: 2}
	baseActive := &Style{Opacity: Alpha(0.7)}
	baseDisabled := &Style{Opacity: Alpha(0.4)}
	shadows := []BoxShadow{{Blur: 4, Color: RGB(0x112233)}}

	makeBtnStyle := func() Style {
		return Style{
			Width:        Pt(100),
			Height:       Pt(40),
			Background:   RGB(0x111111),
			CornerRadius: 8,
			Shadows:      shadows,
			Hover:        baseHover,
			Focus:        baseFocus,
			Active:       baseActive,
			Disabled:     baseDisabled,
		}
	}

	h := newHost(t, Size{Width: 400, Height: 300}, func(cx *Scope) Node {
		disabledA = NewSignal(cx, false)
		dynBG = NewSignal(cx, RGB(0x111111))
		return View{Children: []Node{
			View{TestID: "a", Style: makeBtnStyle(), Disabled: disabledA.Get, OnPress: func(PressEvent) {}},
			View{TestID: "b", Style: makeBtnStyle(), OnPress: func(PressEvent) {}},
			View{TestID: "c1", Class: "p-4 bg-white dark:bg-black rounded"},
			View{TestID: "c2", Class: "p-4 bg-white dark:bg-black rounded"},
			View{TestID: "dyn", Style: Style{Width: Pt(100), Height: Pt(40)}, StyleFunc: func() Style {
				return Style{Background: dynBG.Get()}
			}},
			View{TestID: "plain1"},
			View{TestID: "plain2"},
		}}
	})
	h.commit()

	a, b := h.find("a"), h.find("b")
	c1, c2 := h.find("c1"), h.find("c2")
	dyn := h.find("dyn")
	p1, p2 := h.find("plain1"), h.find("plain2")

	// Plain views share the read-only zero sentinels.
	if p1.base != &zeroStyle || p1.style != &zeroStyle || p2.style != &zeroStyle {
		t.Fatalf("plain views did not share zeroStyle: p1=(%p,%p) p2=(%p,%p)", p1.base, p1.style, p2.base, p2.style)
	}
	if p1.h != &noHandlers || p2.h != &noHandlers {
		t.Fatal("plain views did not share noHandlers")
	}
	if p1.nodeExtra != &zeroExtra || p2.nodeExtra != &zeroExtra {
		t.Fatal("plain views did not share zeroExtra")
	}

	// Siblings with identical static styles share base and resolved style at rest.
	if a.base != b.base {
		t.Fatalf("siblings did not share interned base style: a=%p b=%p", a.base, b.base)
	}
	if a.style != a.base || b.style != b.base {
		t.Fatalf("resting siblings did not alias base style: a=(%p,%p) b=(%p,%p)", a.style, a.base, b.style, b.base)
	}
	if a.lstyle != b.lstyle {
		t.Fatalf("siblings did not share interned layout.Style: a=%p b=%p", a.lstyle, b.lstyle)
	}
	if c1.style != c2.style || c1.cls.normal != c2.cls.normal {
		t.Fatalf("classed siblings did not share resolved styles: c1=%p c2=%p", c1.style, c2.style)
	}

	// Mutating the caller's original slice/pointer after mount must not affect mounted nodes.
	shadows[0].Blur = 99
	baseHover.Background = RGB(0xff0000)
	if a.base.Shadows[0].Blur != 4 || a.base.Hover.Background != RGB(0x2244ff) {
		t.Fatalf("caller mutation leaked into interned style: blur=%v hover=%#x", a.base.Shadows[0].Blur, a.base.Hover.Background)
	}

	// Hovering A changes only A; B and A.base remain untouched.
	h.tree.Dispatch(driver.Event{Kind: driver.EventMouseEnter, ID: a.id})
	h.commit()
	if a.style.Background != RGB(0x2244ff) {
		t.Fatalf("hovered a background = %#x, want 0x2244ff", a.style.Background)
	}
	if a.base.Background != RGB(0x111111) || b.style.Background != RGB(0x111111) || b.style != b.base {
		t.Fatalf("hover on a leaked into base or sibling b: a.base=%#x b.style=%#x", a.base.Background, b.style.Background)
	}

	// Focusing and pressing A changes only A.
	h.tree.Dispatch(driver.Event{Kind: driver.EventFocus, ID: a.id})
	h.tree.Dispatch(driver.Event{Kind: driver.EventPressDown, ID: a.id})
	h.commit()
	if a.style.BorderWidth != 2 || a.style.Opacity.Value() != 0.7 {
		t.Fatalf("focused+pressed a: border=%v opacity=%v", a.style.BorderWidth, a.style.Opacity.Value())
	}
	if b.style.BorderWidth != 0 || b.style.Opacity.Value() != 1 {
		t.Fatalf("focus+press on a leaked into b: border=%v opacity=%v", b.style.BorderWidth, b.style.Opacity.Value())
	}

	// Disabling A applies disabled opacity without affecting B.
	disabledA.Set(true)
	h.commit()
	if a.style.Opacity.Value() != 0.4 || b.style.Opacity.Value() != 1 {
		t.Fatalf("disabled a=%v b=%v", a.style.Opacity.Value(), b.style.Opacity.Value())
	}

	// Clearing all states on A returns A to sharing base with B.
	disabledA.Set(false)
	h.tree.Dispatch(driver.Event{Kind: driver.EventPressUp, ID: a.id})
	h.tree.Dispatch(driver.Event{Kind: driver.EventBlur, ID: a.id})
	h.tree.Dispatch(driver.Event{Kind: driver.EventMouseLeave, ID: a.id})
	h.commit()
	if a.style != a.base || a.style != b.style {
		t.Fatalf("unhovered a did not return to sharing base: a.style=%p a.base=%p b.style=%p", a.style, a.base, b.style)
	}

	// Reactive StyleFunc updates dyn without affecting any other node.
	dynBG.Set(RGB(0xabcdef))
	h.commit()
	if dyn.style.Background != RGB(0xabcdef) {
		t.Fatalf("dyn background = %#x, want 0xabcdef", dyn.style.Background)
	}
	if a.style.Background != RGB(0x111111) || b.style.Background != RGB(0x111111) {
		t.Fatalf("reactive update leaked into static siblings")
	}

	// Theme dark mode updates classed views together while keeping them shared.
	h.tree.SetEnvironment(Environment{Dark: true})
	h.commit()
	if c1.style.Background != RGB(0x000000) || c2.style.Background != RGB(0x000000) {
		t.Fatalf("dark mode classed views: c1=%#x c2=%#x", c1.style.Background, c2.style.Background)
	}
	if c1.style != c2.style {
		t.Fatalf("dark mode classed siblings stopped sharing style: c1=%p c2=%p", c1.style, c2.style)
	}
}

func TestSameStyleAndHashCoverEveryField(t *testing.T) {
	rt := &runtime{}
	rtType := reflect.TypeOf(Style{})
	for i := range rtType.NumField() {
		f := rtType.Field(i)
		if !f.IsExported() {
			continue
		}
		var s Style
		v := reflect.ValueOf(&s).Elem().Field(i)
		setNonZero(t, v)

		if sameStyle(&zeroStyle, &s) {
			t.Errorf("sameStyle(&zeroStyle, &s) = true when only %s is set", f.Name)
		}
		if sameStyle(&s, &zeroStyle) {
			t.Errorf("sameStyle(&s, &zeroStyle) = true when only %s is set", f.Name)
		}
		if !sameStyle(&s, &s) {
			t.Errorf("sameStyle(&s, &s) = false for %s", f.Name)
		}
		if isZeroStyle(&s) {
			t.Errorf("isZeroStyle = true when %s is set", f.Name)
		}
		if interned := rt.internStyle(&s); interned == &zeroStyle {
			t.Errorf("internStyle returned &zeroStyle when %s is set", f.Name)
		} else if !sameStyle(interned, &s) {
			t.Errorf("internStyle returned unequal style for %s", f.Name)
		}
	}
}

func setNonZero(t *testing.T, v reflect.Value) {
	t.Helper()
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1)
	case reflect.String:
		v.SetString("x")
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		setNonZero(t, s.Index(0))
		v.Set(s)
	case reflect.Map:
		m := reflect.MakeMapWithSize(v.Type(), 1)
		k := reflect.New(v.Type().Key()).Elem()
		if k.Kind() == reflect.String {
			k.SetString("wdth")
		} else {
			setNonZero(t, k)
		}
		elem := reflect.New(v.Type().Elem()).Elem()
		setNonZero(t, elem)
		m.SetMapIndex(k, elem)
		v.Set(m)
	case reflect.Ptr:
		elem := reflect.New(v.Type().Elem())
		if v.Type().Elem() != reflect.TypeOf(AnimatedStyle{}) && elem.Elem().Kind() == reflect.Struct && elem.Elem().NumField() > 0 {
			for j := range elem.Elem().NumField() {
				if elem.Elem().Type().Field(j).IsExported() {
					setNonZero(t, elem.Elem().Field(j))
					break
				}
			}
		}
		v.Set(elem)
	case reflect.Struct:
		switch v.Type() {
		case reflect.TypeOf(Length{}):
			v.Set(reflect.ValueOf(Pt(1)))
		case reflect.TypeOf(Opacity{}):
			v.Set(reflect.ValueOf(Alpha(0.5)))
		case reflect.TypeOf(GridPlacement{}):
			v.Set(reflect.ValueOf(GridLine(1)))
		case reflect.TypeOf(Paint{}):
			v.Set(reflect.ValueOf(PaintColor(RGB(0x112233))))
		case reflect.TypeOf(CustomCursor{}):
			v.Set(reflect.ValueOf(CustomCursor{Fallback: CursorPointer}))
		default:
			for j := range v.NumField() {
				if v.Type().Field(j).IsExported() {
					setNonZero(t, v.Field(j))
					return
				}
			}
			t.Fatalf("no exported field in struct %s", v.Type())
		}
	default:
		t.Fatalf("unhandled field kind %s (%s)", v.Kind(), v.Type())
	}
}

func TestZeroSentinelsStayUnmutated(t *testing.T) {
	var ref Ref
	h := newHost(t, Size{Width: 400, Height: 400}, func(cx *Scope) Node {
		return View{Children: []Node{
			View{TestID: "v", Ref: &ref, Style: Style{Padding: All(4)}, MatchID: "m",
				Tooltip: Text{Content: "hint"}, OnPress: func(PressEvent) {},
				PartStates: func() PartStates { return PartStates{Focused: true, Data: []string{"x"}} }},
			Text{TestID: "t", Content: "hello"},
			Input{TestID: "i", DefaultValue: "abc"},
			Scroll{TestID: "s", Children: []Node{View{Style: Style{Height: Pt(500)}}}},
			Image{TestID: "img"},
			Overlay{Children: []Node{Text{Content: "ov"}}},
			Modal{Children: []Node{Text{Content: "mod"}}},
		}}
	})
	h.commit()
	_, _ = ref.ContentBox()
	_ = ref.PointerLocked()
	h.tree.Remount()
	h.commit()

	if !reflect.DeepEqual(zeroStyle, Style{}) {
		t.Fatal("zeroStyle was mutated")
	}
	if zeroViewProps != (driver.ViewProps{}) {
		t.Fatal("zeroViewProps was mutated")
	}
	if defaultTextProps != (driver.ViewProps{Role: RoleText}) {
		t.Fatal("defaultTextProps was mutated")
	}
	if defaultViewStyle != driver.DefaultViewStyle {
		t.Fatal("defaultViewStyle was mutated")
	}
	if !noHandlers.empty() {
		t.Fatal("noHandlers was mutated")
	}
	if !reflect.DeepEqual(zeroExtra, nodeExtra{}) {
		t.Fatal("zeroExtra was mutated")
	}
}

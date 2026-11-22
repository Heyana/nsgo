package keys_test

import (
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func mobileHardwareTarget(t *testing.T) {
	t.Helper()
	if !slices.Contains([]string{"ios", "ios_simulator", "android"}, nsgotest.Target()) {
		t.Skip("checks native iPhone/iPad and Android hardware key routing")
	}
}

func TestMobileHardwareKeysConsumeBeforeActions(t *testing.T) {
	mobileHardwareTarget(t)
	var ref core.Ref
	var bindings, presses atomic.Int32
	var mu sync.Mutex
	var downs, ups []core.KeyEvent
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		for _, key := range []string{"k", "x"} {
			cx.OnKeyWith(key, core.KeyOptions{When: core.KeyAlways}, func() { bindings.Add(1) })
		}
		return core.View{Children: []core.Node{
			core.View{TestID: "recorder", Ref: &ref, Focusable: func() bool { return true },
				OnPress: func(core.PressEvent) { presses.Add(1) },
				OnKeyDown: func(e core.KeyEvent) bool {
					mu.Lock()
					downs = append(downs, e)
					mu.Unlock()
					return e.Key == "k" || e.Key == "space"
				},
				OnKeyUp: func(e core.KeyEvent) { mu.Lock(); ups = append(ups, e); mu.Unlock() },
				Style:   core.Style{Width: core.Pt(220), Height: core.Pt(60)}},
			core.Text{TestID: "held", ContentFunc: func() string { return strings.Join(core.KeysDown(cx), " ") }},
		}}
	}, nsgotest.Size(300, 180))
	app.Post(ref.Focus)
	app.WaitUntil(5*time.Second, func() bool { return app.Find("id:recorder").Focused })
	app.Key("k")
	app.Key("x")
	app.Key("space")
	if bindings.Load() != 1 || presses.Load() != 0 {
		t.Fatalf("consumed downs reached actions: bindings=%d, presses=%d", bindings.Load(), presses.Load())
	}
	app.KeyDown("shift")
	app.KeyDown("a")
	if got := app.Find("id:held").Text; got != "a shift" {
		t.Fatalf("native held keys = %q", got)
	}
	app.KeyUp("a")
	app.KeyUp("shift")
	app.KeyDown("b")
	app.KeyDown("b")
	app.KeyUp("b")
	app.KeyDown("mod")
	modifier := "meta"
	if nsgotest.Target() == "android" {
		modifier = "ctrl"
	}
	if got := app.Find("id:held").Text; got != modifier {
		t.Fatalf("platform modifier = %q, want %q", got, modifier)
	}
	app.KeyUp("mod")
	if got := app.Find("id:held").Text; got != "" {
		t.Fatalf("released native keys remain held: %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ups) != 7 || ups[0].Key != "k" || ups[2].Key != "space" || named(ups[3]) != (core.KeyEvent{Key: "a", Modifiers: core.Modifiers{Shift: true}}) || named(ups[4]) != (core.KeyEvent{Key: "shift"}) || ups[6].Key != modifier {
		t.Fatalf("native ups (consumed keys included) = %+v", ups)
	}
	var repeated []core.KeyEvent
	for _, e := range downs {
		if e.Key == "b" {
			repeated = append(repeated, named(e))
		}
	}
	want := []core.KeyEvent{{Key: "b"}, {Key: "b", Repeat: true}}
	if nsgotest.Target() != "android" {
		want = want[:1]
	}
	if !slices.Equal(repeated, want) {
		t.Fatalf("native repeat sequence = %+v, want %+v", repeated, want)
	}
}

func TestMobileHardwareKeyUpKeepsItsOriginalView(t *testing.T) {
	mobileHardwareTarget(t)
	var first, second core.Ref
	var mu sync.Mutex
	var heard []string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		control := func(id string, ref *core.Ref) core.Node {
			return core.View{TestID: id, Ref: ref, Focusable: func() bool { return true },
				OnKeyDown: func(e core.KeyEvent) bool {
					mu.Lock()
					heard = append(heard, id+" down "+e.Key)
					mu.Unlock()
					return false
				},
				OnKeyUp: func(e core.KeyEvent) { mu.Lock(); heard = append(heard, id+" up "+e.Key); mu.Unlock() },
				Style:   core.Style{Width: core.Pt(220), Height: core.Pt(50)}}
		}
		return core.View{Children: []core.Node{control("first", &first), control("second", &second),
			core.Input{TestID: "field", Style: core.Style{Width: core.Pt(220), Height: core.Pt(40)}},
			core.Text{TestID: "held", ContentFunc: func() string { return strings.Join(core.KeysDown(cx), " ") }},
		}}
	}, nsgotest.Size(300, 240))
	app.Post(first.Focus)
	app.WaitUntil(5*time.Second, func() bool { return app.Find("id:first").Focused })
	app.KeyDown("b")
	app.Post(second.Focus)
	app.WaitUntil(5*time.Second, func() bool { return app.Find("id:second").Focused })
	app.KeyUp("b")
	app.Type("id:field", "z")
	app.KeyDown("a")
	if got := app.Find("id:held").Text; got != "" {
		t.Fatalf("native editor key entered held keys: %q", got)
	}
	app.KeyUp("a")
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(heard, []string{"first down b", "first up b"}) {
		t.Fatalf("release moved with focus or field keys leaked: %v", heard)
	}
}

func TestAndroidHardwareKeysReleaseOnWindowLoss(t *testing.T) {
	if nsgotest.Target() != "android" {
		t.Skip("checks Android's native window focus loss")
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Text{TestID: "held", ContentFunc: func() string { return strings.Join(core.KeysDown(cx), " ") }}
	})
	app.KeyDown("shift")
	app.KeyDown("c")
	if got := app.Find("id:held").Text; got != "c shift" {
		t.Fatalf("native keys = %q", got)
	}
	app.BlurWindow()
	app.WaitUntil(5*time.Second, func() bool { return app.Find("id:held").Text == "" })
}

func TestAndroidHardwareTabKeepsNativeTraversal(t *testing.T) {
	if nsgotest.Target() != "android" {
		t.Skip("checks asynchronous Android key continuation focus search")
	}
	var first core.Ref
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		control := func(id string, consume bool, ref *core.Ref) core.Node {
			return core.View{TestID: id, Ref: ref, Focusable: func() bool { return true },
				OnKeyDown: func(e core.KeyEvent) bool { return consume && e.Key == "tab" },
				Style:     core.Style{Width: core.Pt(220), Height: core.Pt(50)}}
		}
		return core.View{Children: []core.Node{control("first", false, &first), control("second", true, nil), control("third", false, nil)}}
	})
	app.Post(first.Focus)
	app.WaitUntil(5*time.Second, func() bool { return app.Find("id:first").Focused })
	app.Key("tab")
	app.WaitUntil(5*time.Second, func() bool { return app.Find("id:second").Focused })
	app.Key("tab")
	if !app.Find("id:second").Focused {
		t.Fatal("a consumed Tab reached native focus traversal")
	}
}

// named is e without its physical Code, which only some backends report, so a check compares what every target names.
func named(e core.KeyEvent) core.KeyEvent {
	e.Code = ""
	return e
}

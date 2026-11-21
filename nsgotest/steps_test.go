package nsgotest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestFocusedViewReceivesKeyDownBeforeTheKeymap holds Key to give the focused view's OnKeyDown each key first, with its modifiers, a modifier alone included: a key it consumes reaches neither a binding nor the view's own Return, and one it leaves goes on to them (#287).
func TestFocusedViewReceivesKeyDownBeforeTheKeymap(t *testing.T) {
	for _, system := range []core.OS{core.OSMacOS, core.OSWindows} {
		t.Run(string(system), func(t *testing.T) {
			var got []core.KeyEvent
			var bindings, presses int
			app := Start(t, func(cx *core.Scope) core.Node {
				for _, key := range []string{"ctrl+shift+k", "mod+s", "alt+space", "f5", "x"} {
					cx.OnKeyWith(key, core.KeyOptions{When: core.KeyAlways}, func() { bindings++ })
				}
				return core.View{Children: []core.Node{core.View{
					TestID: "recorder", Focusable: func() bool { return true },
					OnPress:   func(core.PressEvent) { presses++ },
					OnKeyDown: func(e core.KeyEvent) bool { got = append(got, e); return e.Key != "x" },
					Style:     core.Style{Width: core.Pt(100), Height: core.Pt(30)},
				}}}
			}, Environment(core.Environment{OS: system, Idiom: core.IdiomDesktop}))
			app.Focus("id:recorder")
			platform := core.Modifiers{Platform: true}
			if system != core.OSMacOS {
				platform = core.Modifiers{Control: true}
			}
			for _, c := range []struct {
				key      string
				want     core.KeyEvent
				shortcut string
			}{
				{"ctrl+shift+k", core.KeyEvent{Key: "k", Modifiers: core.Modifiers{Control: true, Shift: true}}, "ctrl+shift+k"},
				{"mod+s", core.KeyEvent{Key: "s", Modifiers: platform}, map[bool]string{true: "meta+s", false: "ctrl+s"}[system == core.OSMacOS]},
				{"alt+space", core.KeyEvent{Key: "space", Modifiers: core.Modifiers{Alt: true}}, "alt+space"},
				{"F5", core.KeyEvent{Key: "f5"}, "f5"},
				{"Return", core.KeyEvent{Key: "enter"}, "enter"},
				{"shift", core.KeyEvent{Key: "shift", Modifiers: core.Modifiers{Shift: true}}, ""},
			} {
				got = nil
				app.Key(c.key)
				if len(got) != 1 || got[0] != c.want {
					t.Fatalf("Key(%q) delivered %+v, want %+v", c.key, got, c.want)
				}
				if s := got[0].Shortcut(); s != c.shortcut {
					t.Errorf("Key(%q).Shortcut() = %q, want %q", c.key, s, c.shortcut)
				}
			}
			if bindings != 0 || presses != 0 {
				t.Fatalf("consumed keys reached the app: %d bindings, %d presses", bindings, presses)
			}
			got = nil
			app.Key("x")
			if len(got) != 1 || bindings != 1 {
				t.Fatalf("a key the view leaves: %d key events, %d bindings; want 1 and 1", len(got), bindings)
			}
		})
	}
}

// TestKeysGoDownAndComeUpSeparately holds KeyDown and KeyUp to hold a key and release it, the view its down went to hearing OnKeyUp with the modifiers still held, a modifier KeyDown holds joining the keys after it, Key to fire OnKeyUp as a full press does, and core.KeysDown to read what is held; a KeyUp of a key not held and a KeyDown of a chord fail (#1046).
func TestKeysGoDownAndComeUpSeparately(t *testing.T) {
	var downs, ups []core.KeyEvent
	var held []string
	var bindings int
	app := Start(t, func(cx *core.Scope) core.Node {
		cx.OnKeyWith("shift+a", core.KeyOptions{When: core.KeyAlways}, func() { bindings++ })
		return core.View{Children: []core.Node{
			core.View{
				TestID: "pad", Focusable: func() bool { return true },
				OnKeyDown: func(e core.KeyEvent) bool { downs = append(downs, e); return e.Key == "left" },
				OnKeyUp:   func(e core.KeyEvent) { ups = append(ups, e) },
				Style:     core.Style{Width: core.Pt(100), Height: core.Pt(30)},
			},
			core.Text{TestID: "held", ContentFunc: func() string { held = core.KeysDown(cx); return strings.Join(held, " ") }},
		}}
	}, Environment(core.Environment{OS: core.OSWindows, Idiom: core.IdiomDesktop}))
	app.Focus("id:pad")
	app.KeyDown("left")
	app.KeyDown("space")
	if !slices.Equal(held, []string{"left", "space"}) || len(ups) != 0 {
		t.Fatalf("held %v, ups %+v", held, ups)
	}
	app.KeyUp("left")
	app.KeyUp("space")
	if want := []core.KeyEvent{{Key: "left"}, {Key: "space"}}; !slices.Equal(ups, want) || len(held) != 0 {
		t.Fatalf("ups %+v, want %+v; held %v", ups, want, held)
	}
	downs, ups = nil, nil
	app.KeyDown("shift")
	app.Key("a")
	if bindings != 1 {
		t.Fatalf("a with shift held fired %d shift+a bindings, want 1", bindings)
	}
	if want := []core.KeyEvent{{Key: "shift", Modifiers: core.Modifiers{Shift: true}}, {Key: "a", Modifiers: core.Modifiers{Shift: true}}}; !slices.Equal(downs, want) {
		t.Fatalf("downs %+v, want %+v", downs, want)
	}
	app.KeyDown("a")
	app.KeyUp("shift")
	app.KeyUp("a")
	want := []core.KeyEvent{{Key: "a", Modifiers: core.Modifiers{Shift: true}}, {Key: "shift"}, {Key: "a"}}
	if !slices.Equal(ups, want) {
		t.Fatalf("ups %+v, want %+v", ups, want)
	}
	if msg := fails(t, func(tb testing.TB) {
		a := Start(tb, func(cx *core.Scope) core.Node { return core.View{} })
		a.KeyUp("a")
	}); !strings.Contains(msg, "not held") {
		t.Fatalf("a KeyUp of a key not held failed with %q", msg)
	}
	if msg := fails(t, func(tb testing.TB) {
		a := Start(tb, func(cx *core.Scope) core.Node { return core.View{} })
		a.KeyDown("shift+a")
	}); !strings.Contains(msg, "one key") {
		t.Fatalf("a KeyDown of a chord failed with %q", msg)
	}
}

// TestKeysDownHearsKeysNoViewHears holds the keys a frame loop reads to follow KeyDown and KeyUp with no view that hears keys focused, once a read asked for every key, and to leave out those typed into a field (#1046).
func TestKeysDownHearsKeysNoViewHears(t *testing.T) {
	var held string
	app := Start(t, func(cx *core.Scope) core.Node {
		text := core.NewSignal(cx, "")
		return core.View{Children: []core.Node{
			core.Text{TestID: "held", ContentFunc: func() string { held = fmt.Sprint(core.KeysDown(cx), core.KeyHeld(cx, "right")); return held }},
			core.Input{TestID: "field", Value: text.Get, OnChange: text.Set},
		}}
	})
	app.KeyDown("right")
	if held != "[right] true" {
		t.Fatalf("held %q with right down", held)
	}
	app.KeyUp("right")
	if held != "[] false" {
		t.Fatalf("held %q with right up", held)
	}
	app.Focus("id:field")
	app.KeyDown("b")
	if held != "[] false" || app.Find("id:field").Text != "b" {
		t.Fatalf("typing into a field: held %q, the field %q", held, app.Find("id:field").Text)
	}
	app.KeyUp("b")
}

// failing is a testing.TB whose fatal failures end only the goroutine
// running the test body, so a test can check that a step fails.
type failing struct {
	testing.TB
	mu       sync.Mutex
	failure  string
	cleanups []func()
}

func (f *failing) Helper() {}
func (f *failing) Fatalf(format string, args ...any) {
	f.mu.Lock()
	f.failure = fmt.Sprintf(format, args...)
	f.mu.Unlock()
	runtime.Goexit()
}
func (f *failing) Fatal(args ...any)                 { f.Fatalf("%s", fmt.Sprint(args...)) }
func (f *failing) Errorf(format string, args ...any) { f.Fatalf(format, args...) }
func (f *failing) Cleanup(fn func())                 { f.cleanups = append(f.cleanups, fn) }

// fails runs body and returns how it failed the test, or "".
func fails(t *testing.T, body func(tb testing.TB)) string {
	f := &failing{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			for i := len(f.cleanups) - 1; i >= 0; i-- {
				f.cleanups[i]()
			}
		}()
		body(f)
	}()
	<-done
	return f.failure
}

func TestSelectorsMatchBareTextsAndRegularExpressions(t *testing.T) {
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.View{TestID: "card", Label: "Card", Style: core.Style{Height: core.Pt(40)}, Children: []core.Node{
				core.Text{Content: "Save draft"},
				core.Text{Content: " Save "},
			}},
			core.Text{Content: "Buy milk", Role: core.RoleHeader},
			core.Input{Placeholder: "Search", TestID: "search"},
		}}
	})
	for selector, want := range map[string]int{
		"Save":                    1, // a whole text, spaces around it aside
		"card":                    1, // a test ID first
		"Card":                    1, // then a label
		"text:Save":               2, // text: looks for a part
		"text:/^buy/i":            1,
		"label:/^c/i":             1,
		"role:header /milk$/":     1,
		"text:Search":             1, // an empty field's placeholder
		"id:card >> text:/draft/": 1,
		"text:Save >> nth=1":      1,
		"text:Save >> nth=2":      0,
	} {
		if got := len(app.Query(selector)); got != want {
			t.Errorf("%q matches %d views, want %d", selector, got, want)
		}
	}
	id := app.Find("id:search").ID
	app.Find(fmt.Sprintf("view:%d", id))
	if msg := fails(t, func(tb testing.TB) {
		Start(tb, func(cx *core.Scope) core.Node { return core.View{} }).Find("text:/a/x")
	}); !strings.Contains(msg, "flag") {
		t.Errorf("a bad flag fails with %q", msg)
	}
}

// Views hidden, clipped away, outside the window or without an area are
// not seen; a scroll view's content is, and so is a modal over all.
func TestQueriesSeeWhatAUserSees(t *testing.T) {
	sized := func(id string, h core.Points) core.View {
		return core.View{TestID: id, Style: core.Style{Height: core.Pt(h), Width: core.Pt(50)}}
	}
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			sized("plain", 10),
			sized("flat", 0),
			core.View{Style: core.Style{Display: core.DisplayNone}, Children: []core.Node{sized("none", 10)}},
			core.View{Style: core.Style{Height: core.Pt(10), OverflowX: core.Hidden, OverflowY: core.Hidden}, Children: []core.Node{
				sized("inside", 10), sized("clipped", 10)}},
			core.Scroll{Style: core.Style{Height: core.Pt(20)}, Children: []core.Node{
				sized("row-1", 15), sized("row-2", 15), sized("row-3", 15)}},
			core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(900)}, Children: []core.Node{sized("below", 10)}},
		}}
	})
	for id, want := range map[string]bool{
		"plain": true, "flat": false, "none": false, "inside": true, "clipped": false,
		"row-3": true, "below": false,
	} {
		if got := app.Exists("id:" + id); got != want {
			t.Errorf("%s seen = %v, want %v", id, got, want)
		}
	}
}

func TestStepsFailOnManyDisabledOrMissingViews(t *testing.T) {
	root := func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.View{OnPress: func(core.PressEvent) {}, Style: core.Style{Height: core.Pt(20)}, Children: []core.Node{core.Text{Content: "Go"}}},
			core.View{OnPress: func(core.PressEvent) {}, Style: core.Style{Height: core.Pt(20)}, Children: []core.Node{core.Text{Content: "Go"}}},
			core.View{OnPress: func(core.PressEvent) {}, Disabled: func() bool { return true }, Style: core.Style{Height: core.Pt(20)},
				Children: []core.Node{core.Text{Content: "Off"}}},
			core.Input{TestID: "ro", Readonly: true},
		}}
	}
	for selector, want := range map[string]string{
		"text:Go":  "matches 2",
		"text:Off": "disabled",
		"text:No":  "no pressable view",
	} {
		if msg := fails(t, func(tb testing.TB) { Start(tb, root).Press(selector) }); !strings.Contains(msg, want) {
			t.Errorf("pressing %q failed with %q, want %q", selector, msg, want)
		}
	}
	if msg := fails(t, func(tb testing.TB) { Start(tb, root).Type("id:ro", "x") }); !strings.Contains(msg, "disabled") {
		t.Errorf("typing in a read-only field failed with %q", msg)
	}
	Start(t, root).Press("text:Go >> nth=1")
}

func TestPressesAndLongPressesSendWhatPlatformsSend(t *testing.T) {
	var log []string
	var at time.Duration
	var app *App
	note := func(s string) func(core.PressEvent) { return func(core.PressEvent) { log = append(log, s) } }
	app = Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.View{TestID: "b", Style: core.Style{Height: core.Pt(40)}, OnPress: note("press"), OnPressDown: note("down"),
				OnPressUp: note("up"), OnLongPress: func(core.PressEvent) {
					log = append(log, "long")
					at = app.Clock.Now().Sub(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
				}, DelayLongPress: 300 * time.Millisecond},
		}}
	})
	app.Press("id:b")
	if !slices.Equal(log, []string{"down", "up", "press"}) {
		t.Fatalf("a press sent %v", log)
	}
	log = nil
	app.LongPress("id:b")
	if !slices.Equal(log, []string{"down", "long", "up"}) || at != 300*time.Millisecond {
		t.Fatalf("a long press sent %v at %v", log, at)
	}
}

func TestPressesReachTouchOnlyViewsWithoutKeyboardStops(t *testing.T) {
	var touches []core.TouchPhase
	var log []string
	app := Start(t, func(*core.Scope) core.Node {
		row := core.Style{Width: core.Pt(100), Height: core.Pt(40)}
		return core.View{Children: []core.Node{
			core.Input{TestID: "first", Style: row},
			core.View{TestID: "touch", Style: row, OnTouch: func(e core.TouchEvent) { touches = append(touches, e.Phase) }},
			core.View{TestID: "responder", Style: row, Responder: &core.Responder{
				Claims:    []core.TouchClaim{{}},
				OnGrant:   func(core.ResponderEvent) { log = append(log, "grant") },
				OnRelease: func(core.ResponderEvent) { log = append(log, "release") },
			}},
			core.Input{TestID: "last", Style: row},
		}}
	}, Environment(driver.Environment{OS: driver.OSWeb}))
	app.Press("touch")
	app.PressAt("touch", core.Point{X: 10, Y: 10})
	if !slices.Equal(touches, []core.TouchPhase{core.TouchStart, core.TouchEnd, core.TouchStart, core.TouchEnd}) {
		t.Fatalf("touch phases = %v", touches)
	}
	app.Press("responder")
	app.PressAt("responder", core.Point{X: 10, Y: 10})
	if !slices.Equal(log, []string{"grant", "release", "grant", "release"}) {
		t.Fatalf("responder events = %v", log)
	}
	app.Key("Tab")
	app.Key("Tab")
	if !app.Find("last").Focused {
		t.Fatal("Tab stopped on a touch-only view")
	}
}

func TestPressOnTouchChildStillPressesItsButton(t *testing.T) {
	var presses, touches int
	app := Start(t, func(*core.Scope) core.Node {
		return core.View{OnPress: func(core.PressEvent) { presses++ }, Children: []core.Node{
			core.View{TestID: "child", Style: core.Style{Width: core.Pt(100), Height: core.Pt(40)},
				OnTouch: func(e core.TouchEvent) {
					if e.Phase == core.TouchStart {
						touches++
					}
				}},
		}}
	})
	app.Press("child")
	app.PressAt("child", core.Point{X: 10, Y: 10})
	if presses != 2 || touches != 2 {
		t.Fatalf("presses = %d, touches = %d, want 2 each", presses, touches)
	}
}

func TestDragAtStartsWhereItSays(t *testing.T) {
	var start core.Point
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Padding: core.All(20)}, Children: []core.Node{
			core.View{TestID: "pad", Style: core.Style{Height: core.Pt(100)}, OnTouch: func(e core.TouchEvent) {
				if e.Phase == core.TouchStart {
					start = e.Touches[0].Position
				}
			}},
		}}
	})
	app.DragAt("id:pad", core.Point{X: 10, Y: 30}, core.Point{X: 40})
	if start != (core.Point{X: 10, Y: 30}) {
		t.Fatalf("the finger went down at %v", start)
	}
}

// A drag from a point of a view goes down on the pressable view under that point, as PressAt presses it and as the web does, not on the view the selector names (#566).
func TestDragAtPressesThePressableUnderItsPoint(t *testing.T) {
	for _, c := range []struct {
		name  string
		pan   bool
		path  []core.Point
		press int // the child's presses: a drag that moves past the slop cancels its press
	}{
		{"tap", false, nil, 1},
		{"drag", false, []core.Point{{X: 40}, {X: 80, Y: 20}}, 0},
		{"pan", true, []core.Point{{X: 40}, {X: 80, Y: 20}}, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			var parentDown, pans, childDown, childPress int
			app := Start(t, func(cx *core.Scope) core.Node {
				v := core.View{TestID: "parent", Style: core.Style{Width: core.Pt(200), Height: core.Pt(200)},
					OnPressDown: func(core.PressEvent) { parentDown++ }, Children: []core.Node{
						core.View{TestID: "child", Style: core.Style{Width: core.Pt(200), Height: core.Pt(200)},
							OnPressDown: func(core.PressEvent) { childDown++ }, OnPress: func(core.PressEvent) { childPress++ }},
					}}
				if c.pan {
					v.OnPan = func(core.PanEvent) { pans++ }
				}
				return v
			}, Size(200, 200))
			app.DragAt("id:parent", core.Point{X: 50, Y: 50}, c.path...)
			if parentDown != 0 || childDown != 1 || childPress != c.press {
				t.Fatalf("parent down %d, child down %d, child press %d, want 0, 1 and %d", parentDown, childDown, childPress, c.press)
			}
			if c.pan && pans == 0 {
				t.Fatal("the parent's pan did not take the drag")
			}
			app.PressAt("id:parent", core.Point{X: 50, Y: 50})
			if parentDown != 0 || childDown != 2 || childPress != c.press+1 {
				t.Fatalf("after PressAt: parent down %d, child down %d, child press %d, want 0, 2 and %d", parentDown, childDown, childPress, c.press+1)
			}
		})
	}
}

func TestLongPressSelectsText(t *testing.T) {
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.Text{Content: "Select me", Selectable: core.SelectableOn},
			core.View{Selectable: true, Children: []core.Node{core.Text{Content: "In an area"}}},
		}}
	})
	app.LongPress("text:Select me")
	app.LongPress("text:In an area")
}

// SelectText reports the texts its box overlaps, whole, as a copy gives them, with the box; no Size clears both.
func TestSelectableTextReportsItsSelectedString(t *testing.T) {
	var got []core.TextSelection
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.View{TestID: "area", Selectable: true, OnTextSelection: func(s core.TextSelection) { got = append(got, s) }, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
				core.Text{Content: "One", Style: core.Style{Width: core.Pt(50), Height: core.Pt(20)}},
				core.Text{Content: "Two", Style: core.Style{Width: core.Pt(50), Height: core.Pt(20)}},
			}},
			core.Text{Content: "Three", Style: core.Style{Height: core.Pt(20)}},
			core.Text{Content: "Hidden", Selectable: core.SelectableOff, Style: core.Style{Height: core.Pt(20)}},
		}}
	})
	steps := []struct {
		box  core.Bounds
		want string
	}{
		{core.Bounds{Origin: core.Point{X: 4, Y: 4}, Size: core.Size{Width: 20, Height: 10}}, "One"},
		{core.Bounds{Origin: core.Point{X: 4, Y: 4}, Size: core.Size{Width: 80, Height: 10}}, "One\tTwo"},
		{core.Bounds{Origin: core.Point{X: 60, Y: 4}, Size: core.Size{Width: 10, Height: 70}}, "Two\nThree"},
		{core.Bounds{}, ""},
	}
	for _, s := range steps {
		app.SelectText("id:area", s.box)
		if last := got[len(got)-1]; last.Text != s.want || last.Bounds != s.box {
			t.Errorf("SelectText(%v) reported %+v, want %q", s.box, last, s.want)
		}
	}
}

// SelectText and SelectTextRange report each text's selected range with its Ref, so texts of the same content tell apart; SelectTextRange selects a part of them.
func TestSelectableTextReportsItsSelectedRanges(t *testing.T) {
	var first, second, third core.Ref
	var got []core.TextSelection
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.View{TestID: "area", Selectable: true, OnTextSelection: func(s core.TextSelection) { got = append(got, s) }, Children: []core.Node{
			core.Text{Content: "same", Ref: &first, Style: core.Style{Height: core.Pt(20)}},
			core.Text{Content: "same", Ref: &second, Style: core.Style{Height: core.Pt(20)}},
			core.Text{Content: "same", Ref: &third, Style: core.Style{Height: core.Pt(20)}},
		}}
	})
	app.SelectTextRange("id:area", "text:same >> nth=1", 1, "text:same >> nth=2", 3)
	last := got[len(got)-1]
	want := []core.SelectedText{{Ref: &second, Start: 1, End: 4}, {Ref: &third, Start: 0, End: 3}}
	same := func(a, b core.SelectedText) bool { return a == b }
	if last.Text != "ame\nsam" || !slices.EqualFunc(last.Ranges, want, same) || last.Bounds.Size == (core.Size{}) {
		t.Errorf("SelectTextRange reported %+v, want text %q and ranges %+v", last, "ame\nsam", want)
	}
	app.SelectText("id:area", core.Bounds{Origin: core.Point{X: 4, Y: 4}, Size: core.Size{Width: 20, Height: 10}})
	last = got[len(got)-1]
	want = []core.SelectedText{{Ref: &first, Start: 0, End: 4}}
	if last.Text != "same" || !slices.EqualFunc(last.Ranges, want, same) {
		t.Errorf("SelectText reported %+v, want text %q and ranges %+v", last, "same", want)
	}
	app.SelectText("id:area", core.Bounds{})
	if last := got[len(got)-1]; last.Text != "" || last.Ranges != nil || last.Bounds.Size != (core.Size{}) {
		t.Errorf("cleared reported %+v", last)
	}
}

// An app looks up what the user selected in the system's dictionary, at the selection's box in the surface; a platform without the panel shows none.
func TestLookUpTheSelection(t *testing.T) {
	for _, can := range []bool{true, false} {
		var area core.Ref
		var selected core.TextSelection
		var shown bool
		app := Start(t, func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Padding: core.Edges{Top: 40, Left: 10}}, Children: []core.Node{
				core.View{TestID: "area", Ref: &area, Selectable: true, OnTextSelection: func(s core.TextSelection) { selected = s }, Children: []core.Node{
					core.Text{Content: "word"},
				}},
				core.View{TestID: "look-up", Style: core.Style{Height: core.Pt(20)}, OnPress: func(core.PressEvent) { shown = desktop.LookUp(cx, selected.Text, &area, selected.Bounds) }},
			}}
		}, Environment(driver.Environment{OS: driver.OSMacOS, Idiom: driver.IdiomDesktop, CanLookUp: can}))
		app.SelectText("id:area", core.Bounds{Origin: core.Point{X: 2, Y: 1}, Size: core.Size{Width: 20, Height: 10}})
		app.Press("id:look-up")
		want := []LookUp{{Text: "word", At: core.Bounds{Origin: core.Point{X: 12, Y: 41}, Size: core.Size{Width: 20, Height: 10}}}}
		if !can {
			want = nil
		}
		if shown != can || !slices.Equal(app.LookUps, want) {
			t.Errorf("CanLookUp %v: LookUp answered %v and showed %+v, want %+v", can, shown, app.LookUps, want)
		}
	}
}

func TestTypingIsKeyByKey(t *testing.T) {
	var keys, changes []string
	var submitted string
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.Input{TestID: "f", MaxLength: 4, DefaultValue: "a",
			OnKeyPress: func(k string) { keys = append(keys, k) },
			OnChange:   func(s string) { changes = append(changes, s) },
			OnSubmit:   func(s string) { submitted = s }}
	})
	app.Type("id:f", "bcde\n")
	if !slices.Equal(keys, []string{"b", "c", "d", "e", "Enter"}) || !slices.Equal(changes, []string{"ab", "abc", "abcd"}) {
		t.Fatalf("keys %q, changes %q", keys, changes)
	}
	if submitted != "abcd" {
		t.Fatalf("submitted %q", submitted)
	}
	keys, changes = nil, nil
	app.Clear("id:f")
	if !slices.Equal(keys, []string{"Backspace"}) || !slices.Equal(changes, []string{""}) {
		t.Fatalf("clearing sent keys %q, changes %q", keys, changes)
	}
}

func TestKeyAutoYieldsToFocusedControls(t *testing.T) {
	var app *App
	var binding, control int
	app = Start(t, func(cx *core.Scope) core.Node {
		cx.OnKey("space", func() { binding++ })
		return core.Func(func(cx *core.Scope) core.Node {
			cx.OnKeyWith("right", core.KeyOptions{When: core.KeyNotEditing}, func() { control++ })
			return core.View{TestID: "slider", Role: core.RoleAdjustable, OnPress: func(core.PressEvent) {}, Style: core.Style{Width: core.Pt(80), Height: core.Pt(20)}}
		})
	}, Environment(core.Environment{OS: core.OSMacOS}))
	app.Focus("id:slider")
	app.Key("space")
	if binding != 0 {
		t.Fatalf("Space binding fired on a focused control: %d", binding)
	}
	app.Key("right")
	if control != 1 || binding != 0 {
		t.Fatalf("control key handling = control %d, binding %d", control, binding)
	}
}

// App.Key runs the binding a device matches, by the focus: in a focused field the KeyAlways binding of Down, not a newer KeyAuto one, which runs with no field focused (#204).
func TestKeyRunsTheBindingTheFocusMatches(t *testing.T) {
	for _, focused := range []bool{true, false} {
		var ran []string
		app := Start(t, func(cx *core.Scope) core.Node {
			cx.OnKeyWith("down", core.KeyOptions{When: core.KeyAlways}, func() { ran = append(ran, "palette") })
			cx.OnKey("down", func() { ran = append(ran, "volume") })
			return core.Input{TestID: "search"}
		})
		if focused {
			app.Focus("id:search")
		}
		app.Key("down")
		want := "volume"
		if focused {
			want = "palette"
		}
		if !slices.Equal(ran, []string{want}) {
			t.Errorf("field focused %v: ran %q, want %q", focused, ran, want)
		}
	}
}

func TestDragsGoToWhatAFingerMoves(t *testing.T) {
	var log []string
	var pans []core.PanEvent
	app := Start(t, func(cx *core.Scope) core.Node {
		rows := make([]core.Node, 20)
		for i := range rows {
			rows[i] = core.View{Style: core.Style{Height: core.Pt(50)}, OnPress: func(core.PressEvent) { log = append(log, "press") },
				OnPressUp: func(core.PressEvent) { log = append(log, "up") }, TestID: fmt.Sprint("row-", i)}
		}
		return core.View{Children: []core.Node{
			core.View{TestID: "card", Style: core.Style{Height: core.Pt(60)},
				OnPan: func(e core.PanEvent) {
					pans = append(pans, e)
					log = append(log, fmt.Sprintf("pan %v %v", e.Phase, e.Translation.X))
				}},
			core.Scroll{TestID: "list", Style: core.Style{Height: core.Pt(300)}, Children: rows},
		}}
	})
	app.Drag("id:card", core.Point{X: 40})
	if len(log) < 2 || log[0] != fmt.Sprintf("pan %v 10", core.GestureBegan) || log[len(log)-1] != fmt.Sprintf("pan %v 40", core.GestureEnded) {
		t.Fatalf("the pan heard %v", log)
	}
	if len(pans) < 2 || pans[len(pans)-1].Position.X-pans[0].Position.X != pans[len(pans)-1].Translation.X-pans[0].Translation.X || pans[len(pans)-1].PagePosition.X-pans[0].PagePosition.X != pans[len(pans)-1].Translation.X-pans[0].Translation.X {
		t.Fatalf("pan positions = %+v, want translation from begin to end", pans)
	}
	log = nil
	app.Drag("id:row-1", core.Point{Y: -120})
	if got := app.Find("id:list").Offset.Y; got < 100 {
		t.Fatalf("the list scrolled to %v", got)
	}
	if !slices.Equal(log, []string{"up"}) {
		t.Fatalf("the row under the finger heard %v, want its press cancelled", log)
	}
	log = nil
	app.Drag("id:row-4", core.Point{Y: 3})
	if !slices.Equal(log, []string{"up", "press"}) {
		t.Fatalf("a finger that stayed pressed: %v", log)
	}
	before := app.Clock.Now()
	app.Scroll("id:row-5", 100)
	if got := app.Find("id:list").Offset.Y; got < 200 {
		t.Fatalf("scrolled to %v", got)
	}
	if took := app.Clock.Now().Sub(before); took < 10*driver.FrameInterval {
		t.Fatalf("100 points took %v of the clock, want a frame each 10", took)
	}
}

func TestHoverMovesBetweenViews(t *testing.T) {
	var log []string
	hover := func(id string) core.View {
		return core.View{TestID: id, Style: core.Style{Height: core.Pt(20)},
			OnMouseEnter: func(core.MouseEvent) { log = append(log, "in "+id) }, OnMouseLeave: func(core.MouseEvent) { log = append(log, "out "+id) }}
	}
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{hover("a"), hover("b")}}
	})
	app.Hover("id:a")
	app.Hover("id:b")
	app.HoverOut()
	if want := []string{"in a", "out a", "in b", "out b"}; !slices.Equal(log, want) {
		t.Fatalf("hovering heard %v, want %v", log, want)
	}
}

func TestAlertsGetDefaultAnswersAndFastForward(t *testing.T) {
	var answered []string
	var fired int
	app := Start(t, func(cx *core.Scope) core.Node {
		cx.AfterFunc(time.Minute, func() { fired++ })
		cx.AfterFunc(2*time.Minute, func() { fired++ })
		return core.View{TestID: "b", Style: core.Style{Height: core.Pt(20)}, OnPress: func(core.PressEvent) {
			cx.ShowAlert(core.AlertConfig{Title: "Sure?", Actions: []core.AlertAction{{Label: "Yes"}, {Label: "No", Role: core.AlertCancel}}},
				func(i int) { answered = append(answered, fmt.Sprint(i)) })
		}}
	})
	app.Press("id:b")
	app.Press("id:b")
	if !slices.Equal(answered, []string{"1"}) {
		t.Fatalf("answered %v, want the first alert cancelled as the next step starts", answered)
	}
	app.OnAlert = func(driver.AlertConfig) int { return 0 }
	app.Press("id:b")
	if len(answered) != 3 || answered[2] != "0" {
		t.Fatalf("answered %v, want OnAlert's answers", answered)
	}
	app.FastForward(5 * time.Minute)
	if fired != 2 {
		t.Fatalf("fast forward fired %d timers", fired)
	}
}

func TestSpanStylesAreRead(t *testing.T) {
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.Text{Style: core.Style{FontSize: 12}, Spans: []core.Span{
			{Content: "plain "}, {Content: "bold", Style: core.Style{FontWeight: core.FontWeightBold, FontSize: 20}},
		}}
	})
	if s := app.SpanStyle("text:bold"); s.Font.Weight != core.FontWeightBold || s.Font.Size != 20 {
		t.Fatalf("bold's style %+v", s)
	}
	if s := app.SpanStyle("text:plain"); s.Font.Size != 12 {
		t.Fatalf("plain's style %+v", s)
	}
}

func TestModifiedPressKeepsItsSpanModifiers(t *testing.T) {
	var span core.Modifiers
	app := Start(t, func(*core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.Text{Spans: []core.Span{{Content: "plain "}, {Content: "link", OnPress: func(e core.PressEvent) { span = e.Modifiers }}}},
		}}
	})
	mods := core.Modifiers{Shift: true, Platform: true}
	app.PressWith("text:link", mods)
	if span != mods {
		t.Fatalf("span modifiers = %v; want %v", span, mods)
	}
	if app.stepName != "pressWith(text:link, Shift+Platform)" {
		t.Fatalf("step name = %q", app.stepName)
	}
}

func TestTabMovesTheFocusBetweenPressablesAndFields(t *testing.T) {
	var log []string
	button := func(name string, noFocus bool) core.Node {
		return core.View{TestID: name, NoFocus: noFocus, Style: core.Style{Height: core.Pt(20)},
			OnPress: func(p core.PressEvent) { log = append(log, fmt.Sprintf("press %s %d", name, p.Clicks)) },
			OnFocus: func() { log = append(log, "focus "+name) },
			OnBlur:  func() { log = append(log, "blur "+name) }}
	}
	a := Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			button("a", false),
			button("skipped", true),
			core.Input{TestID: "field", Style: core.Style{Height: core.Pt(20)}},
			button("b", false),
		}}
	}, Environment(driver.Environment{OS: driver.OSMacOS}))
	a.Key("Tab")
	a.Key("Tab")
	if f := a.Find("field"); !f.Focused {
		t.Fatalf("the second Tab should focus the field\n%s", a.Dump())
	}
	a.Key("Tab")
	a.Key("Space")
	a.Key("shift+Tab")
	a.Key("shift+Tab")
	a.Press("a")
	a.Press("a")
	want := []string{"focus a", "blur a", "focus b", "press b 1", "blur b", "focus a", "blur a", "press a 1", "press a 2"}
	if !slices.Equal(log, want) {
		t.Fatalf("log = %q\nwant  %q", log, want)
	}
}

func TestFocusTargetsAnyKeyboardStop(t *testing.T) {
	pressed := 0
	root := func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.View{TestID: "save", Style: core.Style{Height: core.Pt(20)}, OnPress: func(core.PressEvent) { pressed++ }},
			core.Input{TestID: "name", Style: core.Style{Height: core.Pt(20)}},
			core.View{TestID: "plain", Style: core.Style{Height: core.Pt(20)}},
		}}
	}
	a := Start(t, root, Environment(driver.Environment{OS: driver.OSMacOS}))
	a.Focus("id:save")
	if !a.Find("id:save").Focused {
		t.Fatalf("Focus should focus a pressable view\n%s", a.Dump())
	}
	a.Key("Return")
	if pressed != 1 {
		t.Fatalf("Return pressed the focused button %d times, want once", pressed)
	}
	a.Focus("id:name")
	if !a.Find("id:name").Focused {
		t.Fatalf("Focus should keep focusing text fields\n%s", a.Dump())
	}
	a.Type("id:name", "Ada")
	if got := a.Find("id:name").Text; got != "Ada" {
		t.Fatalf("typed %q, want Ada", got)
	}
	if msg := fails(t, func(tb testing.TB) { Start(tb, root).Focus("id:plain") }); !strings.Contains(msg, "id:plain") || !strings.Contains(msg, "focusable") {
		t.Fatalf("non-focusable Focus failed with %q", msg)
	}
}

func TestFocusTabsToAStopAsAUserDoes(t *testing.T) {
	var log []string
	button := func(name string) core.Node {
		return core.View{TestID: name, Style: core.Style{Height: core.Pt(20)},
			OnPress: func(core.PressEvent) {},
			OnFocus: func() { log = append(log, "focus "+name) },
			OnBlur:  func() { log = append(log, "blur "+name) }}
	}
	a := Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{button("a"), button("b"), button("c")}}
	}, Environment(driver.Environment{OS: driver.OSMacOS}))
	a.Focus("id:b")
	a.Focus("id:a")
	// The stops on the way take and lose the focus, as they do on a platform.
	want := []string{"focus a", "blur a", "focus b", "blur b", "focus c", "blur c", "focus a"}
	if !slices.Equal(log, want) {
		t.Fatalf("log = %q\nwant  %q", log, want)
	}
}

func TestOnBlurRunsAsTheFocusedViewLeaves(t *testing.T) {
	stops := map[string]func(focus, blur func()) core.Node{
		"input": func(focus, blur func()) core.Node {
			return core.Input{TestID: "note", Style: core.Style{Height: core.Pt(20)}, OnFocus: focus, OnBlur: blur}
		},
		"pressable": func(focus, blur func()) core.Node {
			return core.View{TestID: "note", Style: core.Style{Height: core.Pt(20)}, OnPress: func(core.PressEvent) {}, OnFocus: focus, OnBlur: blur}
		},
		"focusable": func(focus, blur func()) core.Node {
			return core.View{TestID: "note", Style: core.Style{Height: core.Pt(20)}, Focusable: func() bool { return true }, OnFocus: focus, OnBlur: blur}
		},
	}
	// A shortcut removes any of them while it keeps the focus; a click keeps a field's focus too, as the field the issue's app lost it with.
	cases := []struct{ stop, by string }{{"input", "key"}, {"pressable", "key"}, {"focusable", "key"}, {"input", "press"}}
	for _, c := range cases {
		t.Run(c.stop+"/"+c.by, func(t *testing.T) {
			blurs, outs := 0, 0
			a := Start(t, func(cx *core.Scope) core.Node {
				shown := core.NewSignal(cx, true)
				focused := core.NewSignal(cx, false)
				cx.OnKeyWith("mod+k", core.KeyOptions{When: core.KeyAlways}, func() { shown.Set(false) })
				return core.View{OnFocusOut: func() { outs++ }, Children: []core.Node{
					core.Show{When: shown.Get, Then: func(cx *core.Scope) core.Node {
						return stops[c.stop](func() { focused.Set(true) }, func() { blurs++; focused.Set(false) })
					}},
					core.View{TestID: "close", Style: core.Style{Height: core.Pt(20)}, OnPress: func(core.PressEvent) { shown.Set(false) },
						Children: []core.Node{core.Text{Content: "Close"}}},
					core.Show{When: focused.Get, Then: func(cx *core.Scope) core.Node { return core.Text{TestID: "focused", Content: "editing"} }},
				}}
			}, Environment(driver.Environment{OS: driver.OSMacOS}))
			a.Focus("id:note")
			if !a.Exists("id:focused") {
				t.Fatalf("OnFocus did not run\n%s", a.Dump())
			}
			if c.by == "key" {
				a.Key("mod+k")
			} else {
				a.Press("id:close")
			}
			if a.Exists("id:note") {
				t.Fatalf("the note is still there\n%s", a.Dump())
			}
			if a.Exists("id:focused") || blurs != 1 || outs != 1 {
				t.Fatalf("removing the focused note ran OnBlur %d times and the frame's OnFocusOut %d times, want once each\n%s", blurs, outs, a.Dump())
			}
		})
	}
}

func TestFocusResolvesFieldsFirstThenStopsAroundTheMatch(t *testing.T) {
	a := Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.View{TestID: "button", Style: core.Style{Height: core.Pt(20)}, OnPress: func(core.PressEvent) {}, Children: []core.Node{
				core.Text{Content: "Save"},
			}},
			core.View{TestID: "row", Children: []core.Node{
				core.View{TestID: "inner", Style: core.Style{Height: core.Pt(20)}, OnPress: func(core.PressEvent) {}},
			}},
			core.View{TestID: "card", OnPress: func(core.PressEvent) {}, Children: []core.Node{
				core.View{TestID: "form", Children: []core.Node{
					core.Input{TestID: "name", Style: core.Style{Height: core.Pt(20)}},
				}},
			}},
		}}
	}, Environment(driver.Environment{OS: driver.OSMacOS}))
	for _, c := range []struct{ selector, want string }{
		{"text:Save", "button"}, {"id:row", "inner"}, {"id:form", "name"}, {"id:card", "name"},
	} {
		a.Focus(c.selector)
		if !a.Find("id:" + c.want).Focused {
			t.Fatalf("Focus(%q) should focus %s\n%s", c.selector, c.want, a.Dump())
		}
	}
}

func TestFocusFailsOnStopsThatTakeNoFocus(t *testing.T) {
	no := func() bool { return true }
	root := func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.View{TestID: "row", Style: core.Style{Height: core.Pt(20)}, OnPress: func(core.PressEvent) {}, Children: []core.Node{
				core.Input{TestID: "readonly", Readonly: true, Style: core.Style{Height: core.Pt(20)}},
			}},
			core.Input{TestID: "disabledfield", Disabled: no, Style: core.Style{Height: core.Pt(20)}},
			core.View{TestID: "card", OnPress: func(core.PressEvent) {}, Children: []core.Node{
				core.View{TestID: "disabled", Disabled: no, Style: core.Style{Height: core.Pt(20)}, OnPress: func(core.PressEvent) {}},
			}},
			core.View{TestID: "nofocus", NoFocus: true, Style: core.Style{Height: core.Pt(20)}, OnPress: func(core.PressEvent) {}},
			core.View{TestID: "save", Style: core.Style{Height: core.Pt(20)}, OnPress: func(core.PressEvent) {}},
		}}
	}
	mac := Environment(driver.Environment{OS: driver.OSMacOS})
	for _, c := range []struct {
		selector string
		opts     []Option
		want     string
	}{
		{"id:readonly", []Option{mac}, "is disabled"},
		{"id:disabledfield", []Option{mac}, "is disabled"},
		{"id:disabled", []Option{mac}, "is disabled"},
		{"id:nofocus", []Option{mac}, "NoFocus"},
		{"id:save", nil, "desktop or web environment"},
		{"id:save", []Option{Environment(driver.Environment{OS: driver.OSAndroid})}, "desktop or web environment"},
	} {
		msg := fails(t, func(tb testing.TB) { Start(tb, root, c.opts...).Focus(c.selector) })
		if !strings.Contains(msg, c.selector) || !strings.Contains(msg, c.want) {
			t.Fatalf("Focus(%q) failed with %q, want %q", c.selector, msg, c.want)
		}
	}
}

func TestAnAppsRefreshIndicatorFollowsThePull(t *testing.T) {
	var refreshing *core.Signal[bool]
	var pull *core.AnimatedValue
	refreshes := 0
	a := Start(t, func(cx *core.Scope) core.Node {
		refreshing = core.NewSignal(cx, false)
		pull = core.NewAnimatedValue(cx, 0)
		return core.View{Style: core.Style{Height: core.Pt(600)}, Children: []core.Node{
			core.Scroll{TestID: "page", Style: core.Style{FlexGrow: 1},
				Behavior:   core.ScrollBehavior{RefreshIndicator: 72},
				Refreshing: refreshing.Get, PullValue: pull,
				OnRefresh: func() { refreshes++; refreshing.Set(true) },
				Children:  []core.Node{core.View{Style: core.Style{Height: core.Pt(2000)}}}},
			core.View{TestID: "indicator", Style: core.Style{Position: core.Absolute, Width: core.Pt(390), Height: core.Pt(72),
				Animated: &core.AnimatedStyle{TranslateY: pull}}},
		}}
	})
	at := func() driver.Points { return driver.Points(a.Find("indicator").Animated.TranslateY) }
	a.Drag("page", driver.Point{Y: 60})
	if refreshes != 0 {
		t.Fatal("a pull short of the indicator's height refreshed")
	}
	if got := at(); got != 0 {
		t.Fatalf("the pull is %v after it sprang back, want 0", got)
	}
	a.Drag("page", driver.Point{Y: 100})
	if refreshes != 1 || at() != 72 {
		t.Fatalf("refreshes %d, pull %v: want one refresh keeping the indicator's 72", refreshes, at())
	}
	refreshing.Set(false)
	a.Settle()
	if at() != 0 {
		t.Fatalf("the pull is %v once refreshed, want 0", at())
	}
}

func TestAwakeHoldsNameWhatEachReleaseEnds(t *testing.T) {
	app := Start(t, func(cx *core.Scope) core.Node {
		cx.KeepAwake("downloading", core.KeepAwakeOptions{})
		var release func()
		return core.View{Children: []core.Node{
			core.View{TestID: "play", Style: core.Style{Height: core.Pt(20)}, OnPress: func(core.PressEvent) {
				release = cx.KeepAwake("playing", core.KeepAwakeOptions{Display: true})
			}},
			core.View{TestID: "pause", Style: core.Style{Height: core.Pt(20)}, OnPress: func(core.PressEvent) {
				release()
			}},
		}}
	})
	app.Press("id:play")
	if len(app.Awake) != 2 || !app.Awake[1].Display || app.Awake[1].Reason != "playing" {
		t.Fatalf("holds %+v, want the download's and the player's", app.Awake)
	}
	playing := app.Awake[1].ID
	app.Press("id:pause")
	if len(app.Awake) != 1 || app.Awake[0].Reason != "downloading" || app.Awake[0].Display {
		t.Fatalf("holds %+v, want only the download's", app.Awake)
	}
	if !slices.Equal(app.AwakeReleases, []uint32{playing}) || len(app.AwakeHolds) != 2 {
		t.Fatalf("releases %v of holds %+v, want the player's", app.AwakeReleases, app.AwakeHolds)
	}
}

func TestKeepImagePreventsEvictionAndReleasesOnDispose(t *testing.T) {
	atlas := core.Src("asset:/atlas.png")
	var current *core.Signal[core.ImageSource]
	var showHolder *core.Signal[bool]
	var releaseMain func()
	app := Start(t, func(cx *core.Scope) core.Node {
		current = core.NewSignalFunc(cx, atlas, nil)
		showHolder = core.NewSignal(cx, true)
		releaseMain = cx.KeepImage(atlas)
		return core.View{Children: []core.Node{
			core.Show{When: showHolder.Get, Then: func(cx *core.Scope) core.Node {
				// Second scope pins the same atlas; disposing it leaves the root's pin intact.
				cx.KeepImage(atlas)
				return core.View{}
			}},
			core.Canvas{
				Style: core.Style{Width: core.Pt(64), Height: core.Pt(64)},
				Draw: func(d *core.Drawing, _ core.Size) {
					d.DrawImageIn(current.Get(), 0, 0, 64, 64)
				},
			},
		}}
	})
	if !app.KeptImage(atlas) || app.KeptImages[atlas.Key()] != 2 {
		t.Fatalf("kept = %v, want refcount 2", app.KeptImages)
	}
	if !app.CanvasImageCached(atlas) {
		t.Fatal("atlas was not cached after initial draw")
	}

	// Draw 100 other unique sources across separate commits.
	for i := range 100 {
		src := core.ImageSource{Data: []byte(fmt.Sprintf("frame-%d", i))}
		app.Post(func() { current.Set(src) })
		app.Settle()
	}
	if !app.KeptImage(atlas) || !app.CanvasImageCached(atlas) {
		t.Fatal("kept atlas was evicted after drawing 100 other images")
	}

	// Disposing the child scope drops refcount from 2 to 1; atlas stays pinned and cached.
	app.Post(func() { showHolder.Set(false) })
	app.Settle()
	if !app.KeptImage(atlas) || app.KeptImages[atlas.Key()] != 1 || !app.CanvasImageCached(atlas) {
		t.Fatalf("after child dispose: kept=%v cached=%v", app.KeptImages, app.CanvasImageCached(atlas))
	}

	// Calling release twice is safe and unpins + evicts the old atlas cleanly.
	releaseMain()
	releaseMain()
	app.Settle()
	if app.KeptImage(atlas) || len(app.KeptImages) != 0 {
		t.Fatalf("atlas still kept after release: %v", app.KeptImages)
	}
	if app.CanvasImageCached(atlas) {
		t.Fatal("released old atlas was not evicted from cache")
	}
}

// TestPicksRecordTheirHints checks that a test sees the title, button label, type name and starting folder an app asked its pickers for.
func TestPicksRecordTheirHints(t *testing.T) {
	var files, dirs int
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.View{TestID: "b", Style: core.Style{Height: core.Pt(20)}, OnPress: func(core.PressEvent) {
			cx.PickFiles(core.PickFilesOptions{Multiple: true, Types: []string{".mp4"}, TypesName: "视频", Title: "选择视频", Button: "打开", Start: "/videos"}, func([]core.File, error) { files++ })
			cx.PickDirectories(core.PickDirectoriesOptions{Title: "导入资料库", Button: "导入", Start: "/library"}, func([]core.Directory, error) { dirs++ })
		}}
	})
	app.Press("id:b")
	if files != 1 || dirs != 1 {
		t.Fatalf("answered %d file and %d folder picks, want one each", files, dirs)
	}
	if len(app.FilePicks) != 1 || len(app.DirectoryPicks) != 1 {
		t.Fatalf("recorded %d file and %d folder picks, want one each", len(app.FilePicks), len(app.DirectoryPicks))
	}
	if f := app.FilePicks[0]; f.Title != "选择视频" || f.Button != "打开" || f.TypesName != "视频" || f.Start != "/videos" || !slices.Equal(f.Types, []string{".mp4"}) {
		t.Errorf("file pick %+v lost its hints", f)
	}
	if d := app.DirectoryPicks[0]; d.Title != "导入资料库" || d.Button != "导入" || d.Start != "/library" || d.Limit != 1 {
		t.Errorf("folder pick %+v lost its hints", d)
	}
}

func TestFileSaveRecordsRequestBytes(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	data := []byte("original")
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.View{TestID: "save", Style: core.Style{Height: core.Pt(20)}, OnPress: func(core.PressEvent) {
			cx.SaveFile(core.SaveOptions{Name: "report.txt", Data: data}, nil)
		}}
	})
	path := filepath.Join(t.TempDir(), "report.txt")
	app.SaveFile = func(r FileSave) string {
		if r.Name != "report.txt" || r.MIME != "text/plain" || string(r.Data) != "original" {
			t.Fatalf("save request = %+v", r)
		}
		return path
	}
	app.Press("id:save")
	copy(data, "modified")
	if len(app.Saves) != 1 || string(app.Saves[0].Data) != "original" {
		t.Fatalf("recorded saves = %+v", app.Saves)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "original" {
		t.Fatalf("read saved file = %q, %v", got, err)
	}
}

func TestOrientationRequestsFollowTheirScopes(t *testing.T) {
	var outer, middle, inner *core.Scope
	app := Start(t, func(cx *core.Scope) core.Node {
		outer = cx.Child()
		outer.Run(func() { outer.RequestOrientation(core.OrientationPortrait) })
		middle = cx.Child()
		middle.Run(func() { middle.RequestOrientation(core.OrientationPortraitUpsideDown) })
		inner = cx.Child()
		inner.Run(func() { inner.RequestOrientation(core.OrientationLandscape) })
		return core.View{}
	})
	check := func(want core.Orientation) {
		t.Helper()
		if app.Orientation != want {
			t.Fatalf("orientation = %v, want %v", app.Orientation, want)
		}
	}
	check(core.OrientationLandscape)
	app.Post(middle.Dispose)
	app.Settle()
	check(core.OrientationLandscape)
	app.Post(inner.Dispose)
	app.Settle()
	check(core.OrientationPortrait)
	app.Post(outer.Dispose)
	app.Settle()
	check(0)
}

func TestKeyCommandsFollowTheirOwners(t *testing.T) {
	var hide, cover *core.Signal[bool]
	var french func()
	app := Start(t, func(cx *core.Scope) core.Node {
		hide, cover = core.NewSignal(cx, false), core.NewSignal(cx, false)
		cx.SetLocale("en")
		french = func() { cx.SetLocale("fr") }
		return core.View{Children: []core.Node{
			core.Func(func(cx *core.Scope) core.Node {
				cx.OnKeyWith("mod+1", core.KeyOptions{Title: func() string {
					if cx.Env().Locale() == "fr" {
						return "Ouvrir"
					}
					return "Open"
				}, Group: func() string { return "Document" }}, func() {})
				cx.OnKey("mod+2", func() {})
				return core.View{StyleFunc: func() core.Style {
					if hide.Get() {
						return core.Style{Display: core.DisplayNone}
					}
					return core.Style{}
				}}
			}),
			core.Show{When: cover.Get, Then: func(cx *core.Scope) core.Node { return core.Modal{} }},
		}}
	}, Environment(core.Environment{OS: core.OSIOS}))

	check := func(want ...KeyCommand) {
		t.Helper()
		if got := app.KeyCommands(); !slices.Equal(got, want) {
			t.Errorf("KeyCommands = %v, want %v", got, want)
		}
	}
	command := KeyCommand{Key: "mod+1", Title: "Open", Group: "Document"}
	check(command)
	app.Post(french)
	command.Title = "Ouvrir"
	check(command)
	app.Post(func() { hide.Set(true) })
	check()
	app.Post(func() { hide.Set(false) })
	check(command)
	app.Post(func() { cover.Set(true) })
	check()
	app.Post(func() { cover.Set(false) })
	check(command)
}

func TestMouseDownUpHeldButtonsAndBlurWindow(t *testing.T) {
	var downs, ups []core.MouseButtonEvent
	var held []core.MouseButton
	var moves []core.MouseEvent
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.View{
				TestID: "pad",
				Style:  core.Style{Width: core.Pt(100), Height: core.Pt(80)},
				OnMouseDown: func(e core.MouseButtonEvent) {
					downs = append(downs, e)
				},
				OnMouseUp: func(e core.MouseButtonEvent) {
					ups = append(ups, e)
				},
				OnMouseMove: func(e core.MouseEvent) {
					moves = append(moves, e)
				},
			},
			core.Text{TestID: "held", ContentFunc: func() string {
				held = core.MouseButtonsDown(cx)
				return fmt.Sprint(held, core.MouseButtonHeld(cx, core.MouseButtonRight))
			}},
		}}
	}, Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))

	buttons := []core.MouseButton{
		core.MouseButtonLeft,
		core.MouseButtonRight,
		core.MouseButtonMiddle,
		core.MouseButtonBack,
		core.MouseButtonForward,
	}
	for _, b := range buttons {
		app.MouseDownAtWith("id:pad", core.Point{X: 12, Y: 24}, b, core.Modifiers{Shift: true})
		if !slices.Equal(held, []core.MouseButton{b}) {
			t.Fatalf("after %s down: held=%v", b, held)
		}
		app.MouseUpAtWith("id:pad", core.Point{X: 14, Y: 26}, b, core.Modifiers{Shift: true})
		if len(held) != 0 {
			t.Fatalf("after %s up: held=%v", b, held)
		}
	}
	if len(downs) != 5 || len(ups) != 5 {
		t.Fatalf("downs=%d ups=%d, want 5 each", len(downs), len(ups))
	}
	for i, b := range buttons {
		if downs[i].Button != b || downs[i].Position != (core.Point{X: 12, Y: 24}) || !downs[i].Modifiers.Shift || downs[i].Clicks != 1 {
			t.Fatalf("down[%d] = %+v", i, downs[i])
		}
		if ups[i].Button != b || ups[i].Position != (core.Point{X: 14, Y: 26}) || !ups[i].Modifiers.Shift || ups[i].Clicks != 1 {
			t.Fatalf("up[%d] = %+v", i, ups[i])
		}
	}

	// Simultaneous buttons + hover sees held button + BlurWindow clears all held buttons and keys.
	downs, ups, moves = nil, nil, nil
	app.MouseDown("id:pad", core.MouseButtonRight)
	app.MouseDown("id:pad", core.MouseButtonLeft)
	if !slices.Equal(held, []core.MouseButton{core.MouseButtonLeft, core.MouseButtonRight}) {
		t.Fatalf("multi-hold: held=%v", held)
	}
	app.Hover("id:pad")
	if len(moves) == 0 || moves[len(moves)-1].PressedButton != core.MouseButtonLeft {
		t.Fatalf("hover while held: moves=%+v", moves)
	}
	app.BlurWindow()
	if len(held) != 0 {
		t.Fatalf("after BlurWindow: held=%v, want empty", held)
	}
	if len(ups) != 2 || ups[0].Button != core.MouseButtonLeft || ups[1].Button != core.MouseButtonRight || ups[0].Clicks != 0 || ups[1].Clicks != 0 {
		t.Fatalf("BlurWindow ups = %+v", ups)
	}

	// MouseUp of a button not held fails.
	if msg := fails(t, func(tb testing.TB) {
		a := Start(tb, func(cx *core.Scope) core.Node {
			return core.View{TestID: "v", Style: core.Style{Width: core.Pt(40), Height: core.Pt(40)}}
		})
		a.MouseUp("id:v", core.MouseButtonLeft)
	}); !strings.Contains(msg, "not held") {
		t.Fatalf("MouseUp not held failed with %q", msg)
	}
	// MouseButtonNone fails.
	if msg := fails(t, func(tb testing.TB) {
		a := Start(tb, func(cx *core.Scope) core.Node {
			return core.View{TestID: "v", Style: core.Style{Width: core.Pt(40), Height: core.Pt(40)}}
		})
		a.MouseDown("id:v", core.MouseButtonNone)
	}); !strings.Contains(msg, "mouse button must be one of") {
		t.Fatalf("MouseDown none failed with %q", msg)
	}
}

func TestMouseButtonsDownHearsButtonsWithoutViewHandlers(t *testing.T) {
	var held string
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.View{TestID: "bg", Style: core.Style{Width: core.Pt(100), Height: core.Pt(100)}, Children: []core.Node{
			core.Text{TestID: "held", ContentFunc: func() string {
				held = fmt.Sprint(core.MouseButtonsDown(cx), core.MouseButtonHeld(cx, core.MouseButtonMiddle))
				return held
			}},
		}}
	})
	app.MouseDown("id:bg", core.MouseButtonMiddle)
	if held != "[middle] true" {
		t.Fatalf("held=%q with middle down", held)
	}
	app.MouseUp("id:bg", core.MouseButtonMiddle)
	if held != "[] false" {
		t.Fatalf("held=%q with middle up", held)
	}
}

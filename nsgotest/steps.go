package nsgotest

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// defaultLongPress is how long a press is held to be a long one when the
// view's DelayLongPress does not say, as on every platform.
const defaultLongPress = 500 * time.Millisecond

func pressable(v *View) bool {
	return v.Events.Has(driver.EventPress) || v.Events.Has(driver.EventPressDown)
}

func primaryPressTarget(v *View) bool { return v.Kind == KindInput || pressable(v) }

// pointerPressable includes touch listeners and responders, which can take a tap without becoming keyboard stops.
func pointerPressable(v *View) bool {
	return pressable(v) || v.Events.Has(driver.EventPressUp) || v.Events.Has(driver.EventLongPress) || v.Events.Has(driver.EventTouch)
}

// focusable reports whether v is a keyboard focus stop. It follows the stops moveFocus uses on desktops and the web: editable fields, pressable views and views explicitly marked Focusable, except for read-only, disabled or NoFocus views.
func focusable(v *View) bool {
	if v.disabled(nil) || v.Props.NoFocus {
		return false
	}
	return v.Kind == KindInput && !v.Input.Readonly ||
		v.Kind != KindInput && v.Props.FocusStop(v.Events)
}

// systemSwitch returns the system switch (core.SystemSwitch) selector matches headless, where it shows with no size as the platform sizes it, or nil.
func (a *App) systemSwitch(selector string) *View {
	sel, err := parseSelector(selector)
	if a.target != nil || err != nil {
		return nil
	}
	if m := a.resolveWith(sel, true); len(m) == 1 && m[0].Kind == KindNative && m[0].Component == driver.SwitchComponent {
		return m[0]
	}
	return nil
}

// pressTarget is the view a press on selector goes to: the field it matches
// or its nearest pressable ancestor (a text inside a button); a canvas node's
// press goes to the view under its middle. at is where it lands, in
// surface points, and index the byte offset of the characters a text:
// selector names in a text with pressable spans.
func (a *App) pressTarget(selector string) (v *View, at driver.Point, index int) {
	a.t.Helper()
	if m := a.Query(selector); a.target != nil && len(m) == 1 && m[0].Kind == KindNative {
		return m[0], a.target.reveal(m[0]), 0
	}
	if v := a.systemSwitch(selector); v != nil {
		_, at = v.center()
		return v, at, 0
	}
	if m := a.Query(selector); len(m) == 1 && m[0].Node != nil {
		_, at = m[0].center()
		if a.target != nil {
			at = a.target.reveal(m[0])
		}
		v = a.pressableAt(at)
		if v == nil {
			a.t.Fatalf("nsgotest: nothing pressable under %q\n%s", selector, a.Dump())
		}
		if v.disabled(nil) {
			a.t.Fatalf("nsgotest: %q is disabled: a pressable view that takes no input\n%s", selector, a.Dump())
		}
		return v, at, 0
	}
	var match *View
	v = a.pick(selector, "pressable view", func(m *View) (*View, bool) {
		t := m.upTo(primaryPressTarget)
		if t == nil {
			t = m.upTo(pointerPressable)
		}
		if t == nil {
			return nil, false
		}
		match = m
		return t, !m.disabled(t)
	})
	// The press lands on the match, as a finger on a text in a button
	// does.
	_, at = match.center()
	if a.target != nil {
		at = a.target.reveal(match)
	}
	if v == match && v.Kind == KindText && len(v.Spans) > 0 {
		index = a.charsAt(selector, v)
	}
	return v, at, index
}

// charsAt returns the byte offset in text view v of the characters the
// last part of selector names, or 0.
func (a *App) charsAt(selector string, v *View) int {
	sel, err := parseSelector(selector)
	if err != nil {
		return 0
	}
	p := sel.parts[len(sel.parts)-1]
	switch {
	case p.kind != "text" && p.kind != "":
	case !p.text.matches(v.Text) && v.spanAt(p.text) >= 0:
		return v.spanAt(p.text)
	case p.text.re != nil:
		if loc := p.text.re.FindStringIndex(v.Text); loc != nil {
			return loc[0]
		}
	default:
		if i := strings.Index(v.Text, strings.TrimSpace(p.text.want)); i >= 0 {
			return i
		}
	}
	return 0
}

// Press presses the field, pressable or touch-listening view selector names or is inside, as
// a tap does: a finger goes down and lifts on its middle, which the views
// listening to touches hear and presses outside the overlays it is not in
// report, while the view hears PressDown, PressUp and Press. On a
// platform, a native component's view, such as the system's switch,
// takes the press itself. The step settles the keyboard focus the press moves.
func (a *App) Press(selector string) {
	a.t.Helper()
	a.PressWith(selector, driver.Modifiers{})
}

// PressWith presses as Press does, with the modifier keys held for this press only. A target sends them through its native input.
func (a *App) PressWith(selector string, mods driver.Modifiers) {
	a.t.Helper()
	if mods == (driver.Modifiers{}) {
		a.step("press(%s)", selector)
	} else {
		a.step("pressWith(%s, %s)", selector, modifierNames(mods))
	}
	v, at, index := a.pressTarget(selector)
	a.pressAt(v, at, index, mods)
}

// PressAt presses the view selector matches at a point in it, or what is
// pressable there, as Press does its middle: for what finds its own
// targets under the pointer, such as a canvas.
func (a *App) PressAt(selector string, at driver.Point) {
	a.t.Helper()
	a.PressAtWith(selector, at, driver.Modifiers{})
}

// PressAtWith presses as PressAt does, with the modifier keys held for this press only.
func (a *App) PressAtWith(selector string, at driver.Point, mods driver.Modifiers) {
	a.t.Helper()
	if mods == (driver.Modifiers{}) {
		a.step("pressAt(%s, %v, %v)", selector, at.X, at.Y)
	} else {
		a.step("pressAtWith(%s, %v, %v, %s)", selector, at.X, at.Y, modifierNames(mods))
	}
	m := a.pick(selector, "view", func(m *View) (*View, bool) { return m, true })
	page := m.pagePoint(at)
	v := a.pressableAt(page)
	if v == nil {
		a.t.Fatalf("nsgotest: nothing pressable at %v in %q\n%s", at, selector, a.Dump())
	}
	a.pressAt(v, page, 0, mods)
}

// modifierNames names the keys mods holds for a step's name, as "Shift+Platform".
func modifierNames(mods driver.Modifiers) string {
	var names []string
	for _, key := range []struct {
		name string
		held bool
	}{{"Control", mods.Control}, {"Alt", mods.Alt}, {"Shift", mods.Shift}, {"Platform", mods.Platform}} {
		if key.held {
			names = append(names, key.name)
		}
	}
	return strings.Join(names, "+")
}

// pressAt presses v at the surface point at.
func (a *App) pressAt(v *View, at driver.Point, index int, mods driver.Modifiers) {
	a.t.Helper()
	field := v.Kind == KindInput && focusable(v)
	if a.target != nil {
		if field {
			a.target.prepareKeyboard()
		}
		a.target.press(at, 0, mods)
		a.done()
		return
	}
	if v.Kind == KindNative && v.Component == driver.SwitchComponent {
		// The system switch takes the press itself, as on a target, and reports the value it flips to.
		if v.NativeProps["disabled"] == true || v.disabled(nil) {
			a.t.Fatalf("nsgotest: the system switch is disabled: a pressable view that takes no input\n%s", a.Dump())
		}
		checked, _ := v.NativeProps["checked"].(bool)
		a.dispatch(driver.Event{Kind: driver.EventNative, ID: v.ID, Text: "change", Data: !checked})
		a.done()
		return
	}
	a.countClick(v)
	a.inputBy(driver.ModalityPointer)
	a.touchAt(driver.GestureBegan, at, 0)
	a.pressOutside(v, at)
	// A pointer's press takes the keyboard focus off pressable views and moves it into an editable field it hits.
	for _, o := range a.views {
		if o.Focused && (o.Kind != KindInput && a.tabs() || field && o != v) {
			a.setFocus(o, false)
			a.Settle()
		}
	}
	if field {
		a.setFocus(v, true)
		a.Settle()
	}
	for _, k := range []driver.EventKind{driver.EventPressDown, driver.EventPressUp, driver.EventPress} {
		ev := a.pressEvent(v, at)
		ev.Kind, ev.Index, ev.Count = k, index, a.clicks
		ev.Modifiers = mods
		a.send(v, ev)
	}
	a.touchAt(driver.GestureEnded, at, 0)
	a.done()
}

// pressEvent is a press on v at the surface point at, one click with no
// modifiers.
func (a *App) pressEvent(v *View, at driver.Point) driver.Event {
	return driver.Event{ID: v.ID, Point: v.localPoint(at), Page: at, Count: 1}
}

// clickInterval is how soon a press follows the last one to count up.
const clickInterval = 500 * time.Millisecond

// countClick counts a press of v: a press of the view pressed last,
// within clickInterval of the clock, counts up, as platforms count double
// clicks.
func (a *App) countClick(v *View) {
	now := a.Clock.Now()
	if v.ID == a.lastPress && now.Sub(a.lastPressAt) <= clickInterval {
		a.clicks++
	} else {
		a.clicks = 1
	}
	a.lastPress, a.lastPressAt = v.ID, now
}

// pressOutside reports a press at at to the overlays that listen and do
// not hold v.
func (a *App) pressOutside(v *View, at driver.Point) {
	var outside []*View
	walk(a.views[a.rootID], func(o *View) {
		if o.Events.Has(driver.EventPressOutside) && !v.within(o) {
			outside = append(outside, o)
		}
	})
	for _, o := range outside {
		a.send(o, driver.Event{Kind: driver.EventPressOutside, Point: at})
	}
}

// touchAt reports finger id (0 without one) at the surface point at, as
// platforms report it while a view listens to touches; elapsed is the
// time since it went down.
func (a *App) touchAt(phase driver.GesturePhase, at driver.Point, elapsed time.Duration, id ...int) {
	if !a.listensToTouches() {
		return
	}
	a.dispatch(driver.Event{Kind: driver.EventTouch, Phase: phase, Time: elapsed,
		Touches: []driver.Touch{{ID: append(id, 0)[0], PagePosition: at, Changed: true}}})
}

func (a *App) listensToTouches() bool {
	found := false
	walk(a.views[a.rootID], func(v *View) {
		found = found || v.Events.Has(driver.EventTouch)
	})
	return found
}

// walk calls fn for v and its visible descendants.
func walk(v *View, fn func(*View)) {
	if v == nil || v.Style.Hidden {
		return
	}
	fn(v)
	for _, c := range v.Children {
		walk(c, fn)
	}
}

// pressableAt returns the topmost visible view at the surface point p
// that takes presses, following visible overflow until a view that clips
// stops it, as a pointer there finds it.
func (a *App) pressableAt(p driver.Point) *View {
	if v := a.viewAt(p, primaryPressTarget); v != nil {
		return v
	}
	return a.viewAt(p, pointerPressable)
}

// viewAt returns the topmost visible view at the surface point p that ok
// accepts, as a pointer there finds it.
func (a *App) viewAt(p driver.Point, ok func(*View) bool) *View {
	var visit func(*View, driver.Point) *View
	visit = func(v *View, at driver.Point) *View {
		s := v.Animated.Apply(v.Style)
		if s.Hidden || v.Node != nil {
			return nil
		}
		local, mapped := driver.IntoView(v.Frame, v.pointerMatrix(), at)
		if !mapped {
			return nil
		}
		props := v.Props.driverProps()
		self, children := driver.ReachView(v.Frame.Size, &props, driver.Bounds{}, !s.Clip && v.Kind != KindScroll, local)
		if children {
			inner := v.intoContent(local)
			for i := len(v.Children) - 1; i >= 0; i-- {
				if hit := visit(v.Children[i], inner); hit != nil {
					return hit
				}
			}
		}
		if self && ok(v) {
			return v
		}
		return nil
	}
	if r := a.views[a.rootID]; r != nil {
		return visit(r, p)
	}
	return nil
}

// PressDown starts a press on the view selector matches, or the pressable
// view around it, and holds it until PressUp.
func (a *App) PressDown(selector string) { a.t.Helper(); a.half(selector, driver.EventPressDown) }

// PressUp ends a press PressDown started.
func (a *App) PressUp(selector string) { a.t.Helper(); a.half(selector, driver.EventPressUp) }

func (a *App) half(selector string, kind driver.EventKind) {
	a.t.Helper()
	a.step("%s(%s)", map[driver.EventKind]string{driver.EventPressDown: "pressDown", driver.EventPressUp: "pressUp",
		driver.EventContextMenu: "rightClick"}[kind], selector)
	v := a.pick(selector, "view hearing it", func(m *View) (*View, bool) {
		t := m.upTo(func(v *View) bool { return v.Events.Has(kind) })
		return t, t != nil && (kind == driver.EventContextMenu || !m.disabled(t))
	})
	_, at := v.center()
	a.inputBy(driver.ModalityPointer)
	ev := a.pressEvent(v, at)
	ev.Kind = kind
	if kind == driver.EventContextMenu {
		ev.Count = 0
	}
	a.send(v, ev)
	a.done()
}

// LongPress holds a finger on what selector names, or the nearest view
// around it that hears long presses, presses or touches or selects text
// (a selection area, or a selectable text), for its
// DelayLongPress (500 ms by default) of the clock's time, as a long
// press does: PressDown, LongPress, then PressUp, and no Press.
func (a *App) LongPress(selector string) {
	a.t.Helper()
	a.step("longPress(%s)", selector)
	var match *View
	v := a.pick(selector, "pressable view", func(m *View) (*View, bool) {
		t := m.upTo(func(v *View) bool {
			return v.Events.Has(driver.EventLongPress) || pressable(v) || v.Events.Has(driver.EventTouch) ||
				v.Props.SelectionArea || v.Kind == KindText && v.Props.Selectable == driver.SelectableOn
		})
		match = m
		return t, t != nil && !m.disabled(t)
	})
	_, at := v.center()
	index := 0
	if v == match && v.Kind == KindText && len(v.Spans) > 0 {
		index = a.charsAt(selector, v)
	}
	if a.target != nil {
		at = a.target.reveal(v)
	}
	hold := v.Props.DelayLongPress
	if hold <= 0 {
		hold = defaultLongPress
	}
	if a.target != nil {
		// A finger rests as a person's does: past what a scroll view holds
		// its content's touches back, with the test's clock going for what
		// times the hold on the engine, as a context menu does.
		hold = max(hold, defaultLongPress) + 200*time.Millisecond
		a.target.pressWhile(at, hold, func() { a.target.advance(hold, true) })
		a.done()
		return
	}
	a.inputBy(driver.ModalityPointer)
	a.touchAt(driver.GestureBegan, at, 0)
	a.pressOutside(v, at)
	ev := a.pressEvent(v, at)
	ev.Kind, ev.Index = driver.EventPressDown, index
	a.send(v, ev)
	a.elapse(hold)
	ev.Kind = driver.EventLongPress
	a.send(v, ev)
	// A finger rests as long as a person's, past a shorter delay.
	a.elapse(defaultLongPress - hold)
	ev.Kind = driver.EventPressUp
	a.send(v, ev)
	a.touchAt(driver.GestureEnded, at, max(hold, defaultLongPress))
	a.done()
}

// LongPressAt holds a finger at a point in the view selector matches, in its coordinates, as LongPress does its middle.
func (a *App) LongPressAt(selector string, at driver.Point) {
	a.t.Helper()
	a.step("longPressAt(%s, %v, %v)", selector, at.X, at.Y)
	m := a.pick(selector, "view", func(m *View) (*View, bool) { return m, true })
	page := m.pagePoint(at)
	v := a.viewAt(page, func(v *View) bool {
		return v.Events.Has(driver.EventLongPress) || pressable(v) || v.Events.Has(driver.EventTouch) ||
			v.Props.SelectionArea || v.Kind == KindText && v.Props.Selectable == driver.SelectableOn
	})
	if v == nil {
		v = m.upTo(func(v *View) bool {
			return v.Events.Has(driver.EventLongPress) || pressable(v) || v.Events.Has(driver.EventTouch) ||
				v.Props.SelectionArea || v.Kind == KindText && v.Props.Selectable == driver.SelectableOn
		})
	}
	if v == nil || m.disabled(v) {
		a.t.Fatalf("nsgotest: nothing pressable at %v in %q\n%s", at, selector, a.Dump())
	}
	hold := v.Props.DelayLongPress
	if hold <= 0 {
		hold = defaultLongPress
	}
	if a.target != nil {
		hold = max(hold, defaultLongPress) + 200*time.Millisecond
		a.target.pressWhile(page, hold, func() { a.target.advance(hold, true) })
		a.done()
		return
	}
	a.inputBy(driver.ModalityPointer)
	a.touchAt(driver.GestureBegan, page, 0)
	a.pressOutside(v, page)
	ev := a.pressEvent(v, page)
	ev.Kind = driver.EventPressDown
	a.send(v, ev)
	a.elapse(hold)
	ev.Kind = driver.EventLongPress
	a.send(v, ev)
	a.elapse(defaultLongPress - hold)
	ev.Kind = driver.EventPressUp
	a.send(v, ev)
	a.touchAt(driver.GestureEnded, page, max(hold, defaultLongPress))
	a.done()
}

// RightClick sends a secondary click to the view selector matches, or the
// one around it that listens, as a desktop's mouse does.
func (a *App) RightClick(selector string) {
	a.t.Helper()
	if a.target != nil {
		a.step("rightClick(%s)", selector)
		v := a.pick(selector, "view hearing it", func(m *View) (*View, bool) {
			t := m.upTo(func(v *View) bool { return v.Events.Has(driver.EventContextMenu) })
			return t, t != nil
		})
		a.target.rightClick(a.target.reveal(v))
		a.done()
		return
	}
	a.half(selector, driver.EventContextMenu)
}

// inputStep starts a step on the field selector names, the match or the
// first field in it, and focuses it as a tap would; a field the user
// cannot edit fails the step.
func (a *App) inputStep(selector, step string) *View {
	a.t.Helper()
	a.step("%s(%s)", step, selector)
	isInput := func(v *View) bool { return v.Kind == KindInput }
	v := a.pick(selector, "text field", func(m *View) (*View, bool) {
		t := m.downTo(isInput)
		return t, t != nil && !t.disabled(nil) && !t.Input.Readonly
	})
	if !v.Focused {
		if a.target != nil {
			a.target.focusField(v)
		} else {
			a.inputBy(driver.ModalityPointer)
			for _, o := range a.views {
				if o != v {
					a.setFocus(o, false)
				}
			}
			a.setFocus(v, true)
			a.Settle()
		}
	}
	return v
}

// Focus puts the keyboard focus on the selector's focus stop: the text field the match is or holds, focused as a tap would, else the pressable or Focusable view the match is or is in, else the first one inside it, reached by pressing Tab from where the focus is, so a following Return or Space presses it. Desktops and the web give views other than fields the keyboard focus; elsewhere such a view fails the step, as a disabled or read-only match and a NoFocus view do.
func (a *App) Focus(selector string) {
	a.t.Helper()
	a.step("focus(%s)", selector)
	isInput := func(v *View) bool { return v.Kind == KindInput }
	isStop := func(v *View) bool { return v.Kind != KindInput && (v.Events.Pressable() || v.Props.Focusable) }
	v := a.pick(selector, "focusable view", func(m *View) (*View, bool) {
		// A field in the match comes first, as Focus took fields only before it took other stops.
		t := m.downTo(isInput)
		if t == nil {
			t = m.upTo(isStop)
		}
		if t == nil {
			t = m.downTo(isStop)
		}
		return t, t != nil && !t.disabled(nil) && (t.Kind != KindInput || !t.Input.Readonly)
	})
	if v.Kind != KindInput {
		switch {
		case v.Props.NoFocus:
			a.t.Fatalf("nsgotest: %q is NoFocus: it never takes the keyboard focus\n%s", selector, a.Dump())
		case a.target != nil && !slices.Contains([]string{"macos", "windows", "linux", "tui", "web"}, Target()):
			a.t.Fatalf("nsgotest: %q is not a text field, and on %s only text fields take the keyboard focus; pressable views do on desktops and the web\n%s", selector, Target(), a.Dump())
		case a.target == nil && !a.tabs():
			a.t.Fatalf("nsgotest: %q is not a text field, and only text fields take the keyboard focus outside a desktop or web environment: give the test one with Environment\n%s", selector, a.Dump())
		}
	}
	if !v.Focused {
		switch {
		case v.Kind != KindInput:
			a.tabTo(selector, v)
		case a.target != nil:
			a.target.focusField(v)
		default:
			a.inputBy(driver.ModalityPointer)
			for _, o := range a.views {
				if o != v {
					a.setFocus(o, false)
				}
			}
			a.setFocus(v, true)
			a.Settle()
		}
	} else if a.target != nil {
		// The model still names the focused view after the app lost its activation: with the run's opt-in, an explicit focus step takes the keyboard back.
		a.target.prepareKeyboard()
	}
	a.done()
}

// tabTo presses Tab until v has the keyboard focus, as a user reaches a view no tap focuses, so the stops on the way take and lose the focus on a platform and headless alike. It fails the test once a Tab leaves the focus where it was, as when the app binds Tab, or brings it back around to where the first Tab put it without reaching v.
func (a *App) tabTo(selector string, v *View) {
	a.t.Helper()
	// The views with the focus, in a fixed order to compare.
	focused := func() []driver.NodeID {
		var ids []driver.NodeID
		for id, o := range a.views {
			if o.Focused {
				ids = append(ids, id)
			}
		}
		slices.Sort(ids)
		return ids
	}
	var first []driver.NodeID
	// Going around comes back to the first stop within a Tab a view; the bound only keeps a platform whose focus never repeats from holding the test.
	for range len(a.views) + 1 {
		before := focused()
		if a.target != nil {
			a.target.keys(nil, "tab")
			a.target.settle()
		} else {
			a.inputBy(driver.ModalityKeyboard)
			a.moveFocus(false)
			a.Settle()
		}
		if v.Focused {
			return
		}
		now := focused()
		switch {
		case slices.Equal(now, before) && a.target != nil:
			a.t.Fatalf("nsgotest: %q could not take the keyboard focus: Tab left the focus where it was, as it does when the app binds Tab\n%s", selector, a.Dump())
		case slices.Equal(now, before):
			a.t.Fatalf("nsgotest: %q could not take the keyboard focus: Tab left the focus where it was, as it does with no stop in sight\n%s", selector, a.Dump())
		case first == nil && len(now) > 0:
			first = now
		case first != nil && slices.Equal(now, first):
			a.t.Fatalf("nsgotest: %q could not take the keyboard focus: Tab went around every stop without reaching it\n%s", selector, a.Dump())
		}
	}
	a.t.Fatalf("nsgotest: %q could not take the keyboard focus\n%s", selector, a.Dump())
}

// Type types text at the end of the field selector names (the match or
// the first field in it), as a keyboard does: the field takes the focus,
// then each character is a key (KeyPress) and an edit (ChangeText); a
// line break in a single-line field is the return key, as Submit. A
// character past MaxLength is dropped.
func (a *App) Type(selector, text string) {
	a.t.Helper()
	v := a.inputStep(selector, "type")
	if a.target != nil {
		a.target.typeText(v, text)
		a.done()
		return
	}
	for _, c := range text {
		if c == '\n' && !v.Input.Multiline {
			a.returnKey(v)
		} else {
			a.key(v, c)
		}
	}
	a.done()
}

// Clear empties the field selector names, as selecting all and deleting
// does: one KeyPress of Backspace and an edit to nothing.
func (a *App) Clear(selector string) {
	a.t.Helper()
	v := a.inputStep(selector, "clear")
	if a.target != nil {
		a.target.clear(v)
		a.done()
		return
	}
	a.send(v, driver.Event{Kind: driver.EventKeyPress, Text: "Backspace"})
	if v.Text != "" {
		a.edit(v, "")
	}
	a.done()
}

// Submit presses Return in the field selector names, as its
// SubmitBehavior says: SubmitEditing, after which the focus leaves a
// single-line field by default, or a line break in a multiline field.
func (a *App) Submit(selector string) {
	a.t.Helper()
	v := a.inputStep(selector, "submit")
	if a.target != nil {
		a.target.keys(v, "Enter")
		a.done()
		return
	}
	a.returnKey(v)
	a.done()
}

// KeyPress types key in the field selector names, as a key the user
// presses: its text, "Backspace" or "Enter". It reports the key, then
// edits the end of the text as the key would; Enter is the return key.
func (a *App) KeyPress(selector, key string) {
	a.t.Helper()
	v := a.inputStep(selector, "keyPress")
	if a.target != nil {
		a.target.keys(v, key)
		a.done()
		return
	}
	a.typeKey(v, key)
	a.done()
}

// typeKey is key in field v: "Enter", "Backspace" or what it enters.
func (a *App) typeKey(v *View, key string) {
	switch key {
	case "Enter":
		a.returnKey(v)
	case "Backspace":
		a.send(v, driver.Event{Kind: driver.EventKeyPress, Text: key})
		if r := []rune(v.Text); len(r) > 0 {
			a.edit(v, string(r[:len(r)-1]))
		}
	default:
		a.send(v, driver.Event{Kind: driver.EventKeyPress, Text: key})
		if add, ok := driver.LimitEdit(v.Text, 0, key, v.Input.MaxLength); ok && add != "" {
			a.edit(v, v.Text+add)
		}
	}
}

// key types c at the end of field v: a key, then the edit, within
// MaxLength.
func (a *App) key(v *View, c rune) {
	name := string(c)
	if c == '\n' {
		name = "Enter"
	}
	a.send(v, driver.Event{Kind: driver.EventKeyPress, Text: name})
	if max := v.Input.MaxLength; max > 0 && driver.UTF16Len(v.Text)+utf16Len(c) > max {
		return
	}
	a.edit(v, v.Text+string(c))
}

func utf16Len(c rune) int {
	if c >= 0x10000 {
		return 2
	}
	return 1
}

// returnKey is Return in field v, as its SubmitBehavior resolves.
func (a *App) returnKey(v *View) {
	switch v.Input.Submit() {
	case driver.SubmitBehaviorNewline:
		a.key(v, '\n')
		return
	case driver.SubmitBehaviorBlurAndSubmit:
		a.send(v, driver.Event{Kind: driver.EventKeyPress, Text: "Enter"})
		a.send(v, driver.Event{Kind: driver.EventSubmitEditing, Text: v.Text})
		a.setFocus(v, false)
		a.Settle()
		return
	}
	a.send(v, driver.Event{Kind: driver.EventKeyPress, Text: "Enter"})
	a.send(v, driver.Event{Kind: driver.EventSubmitEditing, Text: v.Text})
}

// edit sets field v's text as a user's edit, the caret at its end.
func (a *App) edit(v *View, text string) {
	a.edits[v.ID]++
	v.Text = text
	a.syncInputContent(v)
	a.dispatch(driver.Event{Kind: driver.EventChangeText, ID: v.ID, Text: text, Count: a.edits[v.ID]})
	end := driver.UTF16Len(text)
	a.moveSelection(v, driver.TextRange{Start: end, End: end})
	if v.Events.Has(driver.EventCaretMove) && !a.noCaretMoves {
		at, size := v.caret()
		a.dispatch(driver.Event{Kind: driver.EventCaretMove, ID: v.ID, Point: at, Size: size})
	}
}

// caret is where field v's caret is at the end of its text, in the
// field's view, as the approximate text engine lays it out.
func (v *View) caret() (driver.Point, driver.Size) {
	size := v.Input.Style.Font.Size
	if size == 0 {
		size = 14
	}
	line := v.Input.Style.LineHeight
	if line == 0 {
		line = size * 1.2
	}
	lines := strings.Split(v.Text, "\n")
	last := lines[len(lines)-1]
	c := v.Input.Content
	return driver.Point{X: c.Origin.X + driver.Points(utf8.RuneCountInString(last))*size*0.6,
		Y: c.Origin.Y + driver.Points(len(lines)-1)*line}, driver.Size{Width: 1, Height: line}
}

// Paste pastes into the field selector names as the user would: the
// field hears it with OnPaste, then, unless its PasteBehavior is
// PasteReport, the text goes in at its end.
func (a *App) Paste(selector string, p driver.PasteEvent) {
	a.t.Helper()
	v := a.inputStep(selector, "paste")
	if v.Events.Has(driver.EventPaste) {
		a.dispatch(driver.Event{Kind: driver.EventPaste, ID: v.ID, Data: &p})
	}
	if p.Text != "" && v.Input.PasteBehavior == driver.PasteInsert {
		text := p.Text
		if !v.Input.Multiline {
			text = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(text)
		}
		a.edit(v, driver.ClipUTF16(v.Text+text, v.Input.MaxLength))
	}
	a.done()
}

// Select moves the selection of the field selector names, as the user
// would, in UTF-16 units.
func (a *App) Select(selector string, start, end int) {
	a.t.Helper()
	v := a.inputStep(selector, "select")
	if a.target != nil {
		a.target.selectRange(v, start, end)
		a.done()
		return
	}
	a.moveSelection(v, driver.TextRange{Start: start, End: end})
	a.done()
}

// SelectTab chooses the zero-based index in the mounted mobile.TabBar
// selector names. It uses the native tab bar on iOS and Android targets
// and delivers the same change event headless.
func (a *App) SelectTab(selector string, index int) {
	a.t.Helper()
	a.step("selectTab(%s, %d)", selector, index)
	var native testhost.TabBarSurface
	if a.target != nil {
		var ok bool
		native, ok = a.target.surface.(testhost.TabBarSurface)
		if !ok {
			a.t.Skip("nsgotest: the platform has no native tab bar")
		}
	}
	v := a.FindMounted(selector)
	if v.Component != driver.TabBarComponent || v.Kind != KindNative {
		a.t.Fatalf("nsgotest: %s: %q is not a mobile.TabBar", a.stepName, selector)
	}
	if v.disabled(nil) || v.upTo(func(p *View) bool { return p.Style.Hidden }) != nil {
		a.t.Fatalf("nsgotest: %s: %q is hidden or disabled", a.stepName, selector)
	}
	items, _ := v.NativeProps["items"].([]any)
	if index < 0 || index >= len(items) {
		a.t.Fatalf("nsgotest: %s: index %d is outside the bar's %d tabs", a.stepName, index, len(items))
	}
	if native != nil {
		a.target.check(native.SelectTab(v.ID, index))
	} else {
		a.dispatch(driver.Event{Kind: driver.EventNative, ID: v.ID, Text: "change", Data: float64(index)})
	}
	a.done()
}

func (a *App) moveSelection(v *View, r driver.TextRange) {
	if r == v.Selection {
		return
	}
	v.Selection = r
	if v.Events.Has(driver.EventSelectionChange) {
		a.dispatch(driver.Event{Kind: driver.EventSelectionChange, ID: v.ID, Index: r.Start, Count: uint32(r.End)})
	}
}

// Compose has an input method compose text in the field selector names;
// "" ends the composition, after the Type of what it commits, or Commit
// commits it. While it lasts the field holds the values the app sets, as
// platforms do. Under nsgo test -target it goes through the platform's
// text input client, as an input method's does.
func (a *App) Compose(selector, text string) {
	a.t.Helper()
	if a.target != nil {
		a.step("compose(%s, %q)", selector, text)
		a.Find(selector)
		a.target.compose(text)
		a.done()
		return
	}
	v := a.Find(selector)
	if text != "" {
		v.Composing = text
		if v.Events.Has(driver.EventCompositionChange) {
			a.dispatch(driver.Event{Kind: driver.EventCompositionChange, ID: v.ID, Text: text})
		}
		return
	}
	v.Composing = ""
	if v.Events.Has(driver.EventCompositionChange) {
		a.dispatch(driver.Event{Kind: driver.EventCompositionChange, ID: v.ID, Finished: true})
	}
	if it := v.held; it != nil {
		v.held = nil
		if a.edits[v.ID] <= it.Count {
			v.Text = it.Value
			a.syncInputContent(v)
		}
	}
}

// Commit has the input method composing in the field selector names
// commit text, ending its composition: the Type of text, then the
// composition's end, or the platform's commit under nsgo test -target.
func (a *App) Commit(selector, text string) {
	a.t.Helper()
	if a.target != nil {
		a.step("commit(%s, %q)", selector, text)
		a.Find(selector)
		a.target.commit(text)
		a.done()
		return
	}
	a.Type(selector, text)
	a.Compose(selector, "")
}

// KeyCommand describes a titled keyboard binding.
type KeyCommand struct {
	Key, Title, Group string
}

// KeyCommands returns the titled bindings that would fire with the
// current focus and visible views, in binding order.
func (a *App) KeyCommands() []KeyCommand {
	a.t.Helper()
	a.Settle()
	var out []KeyCommand
	for _, b := range driver.KeyCommands(a.keyBindings, a.focusKeyUse(), a.keyNames) {
		out = append(out, KeyCommand{Key: b.Key, Title: b.Title, Group: b.Group})
	}
	return out
}

// Key presses a key on a hardware keyboard, as the app's keymap reads it ("mod+s", "Escape", "a", or a modifier alone, "shift"), and lets it come up: the focused view's OnKeyDown hears it first, and a key it consumes goes no further; else a binding of the keymap fires, unless the focused view uses the key itself as on a device (core.KeyAuto), a field its editing keys, a pressable view Return and Space, a slider or a tab bar its arrows; else, in a desktop or web environment, Tab and Shift+Tab move the keyboard focus between fields and pressable views, and Return or Space press the pressable view it is on with the modifiers held, except with Alt on Windows, where they go to the window menu; else the focused field takes a character, Backspace or Enter, mod+a selects all its text, and mod+c, mod+x and mod+v copy, cut and paste with Clipboard when no application-menu item has the key as its shortcut; else, in a macOS, Windows or Linux environment, the enabled application-menu item with the key as its shortcut is chosen, unless it is NotInText and a field or web view has the focus; else Escape goes back. A key nothing takes does nothing, as on a device. Then the key comes up, which OnKeyUp of the view that heard it go down hears, with the key's modifiers still held. The modifiers KeyDown holds join the key, so KeyDown("shift") then Key("a") presses shift+a.
func (a *App) Key(key string) {
	a.t.Helper()
	a.step("key(%s)", key)
	if a.target != nil {
		a.target.keys(nil, key)
		a.done()
		return
	}
	parts := strings.Split(driver.NormalizeKey(key), "+")
	a.press(key, "")
	a.release(a.keyName(parts[len(parts)-1]), a.keyModifiers(parts[:len(parts)-1]))
	a.done()
}

// KeyDown presses one key and holds it, as the keymap names it ("a", "left", "space", "shift", "mod"), never a chord, which fails the step: what Key does as a key goes down, OnKeyDown, the keymap, Tab, the focused field and the menu, without its coming up. A modifier it holds joins every later KeyDown, KeyUp and Key until KeyUp releases it, so KeyDown("shift") then Key("a") presses shift+a. A key already held goes down again as its auto-repeat, except on the iOS and tvOS simulators, where UIKit reports a held key's down once. A key still held as the test ends is released then, under nsgo test -target through the platform, whose OnKeyUp the view that heard it go down hears, so none stays held into the next test. Under nsgo test -target, a platform that cannot hold a key skips the test: Android, HarmonyOS and watchOS.
func (a *App) KeyDown(key string) {
	a.t.Helper()
	a.step("keyDown(%s)", key)
	a.oneKey(key)
	if a.target != nil {
		a.target.hold(key, true)
		a.done()
		return
	}
	a.press(key, "")
	a.done()
}

// KeyUp releases one key KeyDown holds, which OnKeyUp of the view that heard it go down hears, wherever the focus has gone since, with the modifiers still held; a key not held fails the step. Under nsgo test -target, a platform that cannot hold a key skips the test, as for KeyDown.
func (a *App) KeyUp(key string) {
	a.t.Helper()
	a.step("keyUp(%s)", key)
	a.oneKey(key)
	if a.target != nil {
		a.target.hold(key, false)
		a.done()
		return
	}
	name := a.keyName(driver.NormalizeKey(key))
	if _, ok := a.held[name]; !ok {
		a.t.Fatalf("nsgotest: %s: %q is not held; KeyDown holds a key", a.stepName, key)
	}
	a.release(name, driver.Modifiers{})
	a.done()
}

// KeyCode presses and releases a physical position, such as KeyW, in a headless test. It uses the test environment's KeyCodeNames or the US layout.
func (a *App) KeyCode(code string) {
	a.t.Helper()
	a.step("keyCode(%s)", code)
	a.press(a.physicalKeyName(code), code)
	a.release(code, driver.Modifiers{})
	a.done()
}

// KeyCodeDown presses a physical position and holds it until KeyCodeUp.
func (a *App) KeyCodeDown(code string) {
	a.t.Helper()
	a.step("keyCodeDown(%s)", code)
	a.press(a.physicalKeyName(code), code)
	a.done()
}

// KeyCodeUp releases a physical position held by KeyCodeDown.
func (a *App) KeyCodeUp(code string) {
	a.t.Helper()
	a.step("keyCodeUp(%s)", code)
	a.physicalKeyName(code)
	if _, ok := a.held[code]; !ok {
		a.t.Fatalf("nsgotest: %s: %q is not held. KeyCodeDown holds a position", a.stepName, code)
	}
	a.release(code, driver.Modifiers{})
	a.done()
}

func (a *App) physicalKeyName(code string) string {
	a.t.Helper()
	if a.target != nil {
		a.t.Skip("nsgotest: physical key positions are supported in headless tests")
	}
	if name := a.env.KeyCodeNames[code]; name != "" {
		return name
	}
	name := driver.USKeyName(code)
	if name == "" {
		a.t.Fatalf("nsgotest: %s: unknown physical key code %q", a.stepName, code)
	}
	return name
}

// oneKey fails the step for a chord, which KeyDown and KeyUp do not take.
func (a *App) oneKey(key string) {
	a.t.Helper()
	if strings.Contains(driver.NormalizeKey(key), "+") {
		a.t.Fatalf("nsgotest: %s: KeyDown and KeyUp take one key, not the chord %q; hold each key", a.stepName, key)
	}
}

// press does what Key does as key goes down, the modifiers KeyDown holds joining it.
func (a *App) press(key, code string) {
	name := key
	key = a.withHeld(key)
	k := driver.NormalizeKey(key)
	a.inputBy(driver.ModalityKeyboard)
	if (k == "escape" || strings.HasSuffix(k, "+escape")) && a.unlockPointer() {
		a.done()
		return
	}
	names := a.keyNames(k)
	parts := strings.Split(k, "+")
	identity := code
	if identity == "" {
		name = a.keyName(parts[len(parts)-1])
		identity = name
	}
	_, repeat := a.held[identity]
	if a.keyDown(k, name, code, identity, repeat) {
		return
	}
	if id, consumed := driver.FindKey(a.keyBindings, names, a.focusKeyUse(), repeat); consumed {
		if id != 0 {
			a.dispatch(driver.Event{Kind: driver.EventKey, Count: id})
		}
		return
	}
	if a.tabs() {
		if k == "tab" || k == "shift+tab" {
			a.moveFocus(k == "shift+tab")
			return
		}
	}
	if a.env != nil && a.env.FocusesViews() {
		name := parts[len(parts)-1]
		// The keymap had the key first, including modified activations. Windows sends keys held with Alt as system keys, which go to the window menu rather than the focused view.
		mods := a.keyModifiers(parts[:len(parts)-1])
		if (name == "enter" || name == "space") && !(mods.Alt && a.env.OS == driver.OSWindows) {
			// Return or Space press the pressable the keyboard focus is on, sending the down, up and press events it handles and taking the key even when it handles only a long press, as the platforms do.
			for _, v := range a.shown() {
				if v.Focused && v.Kind != KindInput && a.canFocus(v) && v.Events.Pressable() {
					for _, kind := range []driver.EventKind{driver.EventPressDown, driver.EventPressUp, driver.EventPress} {
						a.send(v, driver.Event{Kind: kind, Modifiers: mods})
					}
					return
				}
			}
		}
	}
	// The focused field takes the keys that edit it, a character shifted
	// as the US layout shifts it.
	name, shift := k, false
	if rest, ok := strings.CutPrefix(k, "shift+"); ok {
		name, shift = rest, true
	}
	for _, v := range a.views {
		if !v.Focused || v.Kind != KindInput || v.Input.Readonly || v.Input.Disabled {
			continue
		}
		if slices.Contains(names, "mod+a") {
			if _, taken := a.menuShortcut(names); taken {
				break
			}
			a.moveSelection(v, driver.TextRange{Start: 0, End: driver.UTF16Len(v.Text)})
			return
		}
		// The field's own clipboard keys, which the platforms' fields take before the application menu's editing roles; an item the app gave one of them as its shortcut takes it first, as the platforms' menus do (chooseMenuEntry still hands an editing role to the field).
		role := driver.MenuRoleNone
		switch {
		case slices.Contains(names, "mod+c"):
			role = driver.MenuCopy
		case slices.Contains(names, "mod+x"):
			role = driver.MenuCut
		case slices.Contains(names, "mod+v"):
			role = driver.MenuPaste
		}
		if role != driver.MenuRoleNone {
			if _, taken := a.menuShortcut(names); taken {
				break
			}
			a.editField(v, role)
			return
		}
		if strings.Contains(name, "+") {
			continue
		}
		typed := key[len(key)-len(name):]
		switch {
		case name == "enter" && !shift:
			typed = "Enter"
		case name == "backspace" && !shift:
			typed = "Backspace"
		case name == "space":
			typed = " "
		case utf8.RuneCountInString(typed) != 1:
			continue
		case shift:
			typed = shifted(typed)
		}
		a.typeKey(v, typed)
		return
	}
	if entry, consumed := a.menuShortcut(names); consumed {
		a.chooseMenuEntry(entry)
		return
	}
	if a.selectedArea != 0 {
		switch {
		case slices.Contains(names, "mod+c") && a.editSelectionArea(driver.MenuCopy):
			return
		case slices.Contains(names, "mod+a") && a.editSelectionArea(driver.MenuSelectAll):
			return
		case k == "escape" && a.selectedText != "":
			if v := a.views[a.selectedArea]; v != nil {
				a.selectedArea, a.selectedText = 0, ""
				a.send(v, driver.Event{Kind: driver.EventSelectionChange})
				return
			}
			a.selectedArea, a.selectedText = 0, ""
		}
	}
	if k == "escape" {
		a.dispatch(driver.Event{Kind: driver.EventBack, ID: a.rootID})
	}
}

// menuShortcut finds the item of the application menu a key that the keymap, the keyboard focus and the focused field left goes to, as macOS's key equivalents and Windows' and Linux's menu shortcuts take it (driver.MenuShortcut): the item, and whether one takes the key. Only macOS, Windows and Linux show the menu.
func (a *App) menuShortcut(names []string) (driver.MenuEntry, bool) {
	if a.env == nil || a.env.OS != driver.OSMacOS && a.env.OS != driver.OSWindows && a.env.OS != driver.OSLinux {
		return driver.MenuEntry{}, false
	}
	editing := false
	for _, v := range a.views {
		if v.Focused && (v.Kind == KindInput || v.Kind == KindWeb) {
			editing = true
			break
		}
	}
	e, ok := driver.MenuShortcut(a.menu, names, a.env.OS, editing, a.env.Speak)
	return e, ok
}

// chooseMenuEntry carries out the application-menu item e, chosen by its shortcut or by ChooseMenu, as the desktop platforms do: an editing role goes to the focused field first, and the item's OnSelect runs when the field does not take it; another item with OnSelect runs it; one without carries out its role (menuRole).
func (a *App) chooseMenuEntry(e driver.MenuEntry) {
	if (e.Role.EditsText() || e.Role == driver.MenuPasteAndMatchStyle) && a.menuEdit(e.Role) {
		return
	}
	if e.ID != 0 {
		a.dispatchOn(a.menuTree, driver.Event{Kind: driver.EventMenu, Count: e.ID})
		return
	}
	a.menuRole(e.Role)
}

// menuRole carries out a standard role of an item without OnSelect: Close closes the main window as its close button does, running its OnClosing handlers; Quit asks the app's OnQuitRequest handlers and an approved request counts in Quits; Minimize, Zoom and Toggle Full Screen change the main window's state its environment reports, Minimize putting it in the background as the desktops do, the other windows staying as they are; an editing role with no focused field to take it does nothing, as on the platforms. A role the model cannot carry out fails the step, so that its key is not swallowed unnoticed.
func (a *App) menuRole(role driver.MenuRole) {
	a.t.Helper()
	switch role {
	case driver.MenuClose:
		a.CloseWindow()
	case driver.MenuQuit:
		a.requestQuit(false)
	case driver.MenuMinimize:
		a.changeWindowState(role, func(env *driver.Environment) { env.Minimized, env.Lifecycle = true, driver.LifecycleBackground })
	case driver.MenuZoom:
		a.changeWindowState(role, func(env *driver.Environment) { env.Maximized = !env.Maximized })
	case driver.MenuToggleFullScreen:
		a.changeWindowState(role, func(env *driver.Environment) { env.FullScreen = !env.FullScreen })
	case driver.MenuUndo, driver.MenuRedo, driver.MenuCut, driver.MenuCopy, driver.MenuPaste, driver.MenuPasteAndMatchStyle, driver.MenuDelete, driver.MenuSelectAll:
	default:
		a.t.Fatalf("nsgotest: the menu role %q is not modelled headless; give the item an OnSelect or run the test on a target", role.Label())
	}
}

// changeWindowState changes the main window's state that its environment reports, as the window's own Minimize, Zoom and Toggle Full Screen do, leaving the app's other windows as they are.
func (a *App) changeWindowState(role driver.MenuRole, change func(*driver.Environment)) {
	a.t.Helper()
	if a.env == nil || a.appTree == nil || a.tree == a.appTree {
		a.t.Fatalf("nsgotest: the menu role %q needs a main window open: a macOS environment, or StartApp with a Windows or Linux one", role.Label())
	}
	a.SetWindowEnvironment("", change)
}

// menuEdit carries out editing role in the focused field, as its own keys do, and reports whether a field took it. A focused web view's page is not modelled, so a role it would take fails the step.
func (a *App) menuEdit(role driver.MenuRole) bool {
	a.t.Helper()
	for _, v := range a.views {
		if !v.Focused {
			continue
		}
		switch {
		case v.Kind == KindWeb:
			a.t.Fatalf("nsgotest: the menu role %q in a web view's page is not modelled headless", role.Label())
		case v.Kind == KindInput && !v.Input.Readonly && !v.Input.Disabled:
			a.editField(v, role)
			return true
		}
	}
	return a.editSelectionArea(role)
}

// editSelectionArea carries out Copy or SelectAll on the active selection area set by SelectText or SelectTextRange, reporting whether it acted.
func (a *App) editSelectionArea(role driver.MenuRole) bool {
	if a.selectedArea == 0 {
		return false
	}
	v := a.views[a.selectedArea]
	if v == nil {
		a.selectedArea, a.selectedText = 0, ""
		return false
	}
	switch role {
	case driver.MenuCopy:
		if a.selectedText == "" {
			return false
		}
		a.Clipboard, a.ClipboardImage, a.ClipboardFiles = a.selectedText, nil, nil
		return true
	case driver.MenuSelectAll:
		texts := areaTextsOf(v)
		var m driver.SelectionModel
		m.SelectAll(texts)
		text := m.Text(texts)
		var ranges []driver.SelectedRange
		for _, r := range m.Ranges(texts) {
			ranges = append(ranges, driver.SelectedRange{ID: texts[r.Text].v.ID, Start: r.Start, End: r.End})
		}
		b := m.Extent(texts)
		a.selectedText = text
		a.send(v, driver.Event{Kind: driver.EventSelectionChange, Point: b.Origin, Size: b.Size, Text: text, Ranges: ranges})
		return true
	}
	return false
}

// editField carries out editing role in field v as the platforms' fields do with the clipboard (Clipboard): Copy and Cut take a non-empty selection, which a masked field keeps to itself; Paste replaces the selection with the clipboard's text, a single-line field's line breaks as spaces, within MaxLength, and a field that reports pastes (PasteReport) only reports it; Delete removes the selection. Undo and Redo are not modelled and fail the step.
func (a *App) editField(v *View, role driver.MenuRole) {
	a.t.Helper()
	start := max(v.Selection.Start, 0)
	end := min(max(v.Selection.End, start), driver.UTF16Len(v.Text))
	start = min(start, end)
	left, right := driver.UTF16Offset(v.Text, start), driver.UTF16Offset(v.Text, end)
	switch role {
	case driver.MenuSelectAll:
		a.moveSelection(v, driver.TextRange{Start: 0, End: driver.UTF16Len(v.Text)})
	case driver.MenuCopy, driver.MenuCut:
		if left == right || v.Input.Masked {
			return
		}
		a.Clipboard = v.Text[left:right]
		if role == driver.MenuCut {
			a.edit(v, v.Text[:left]+v.Text[right:])
		}
	case driver.MenuPaste, driver.MenuPasteAndMatchStyle:
		if a.Clipboard == "" {
			return
		}
		a.send(v, driver.Event{Kind: driver.EventPaste, Data: &driver.PasteEvent{Text: a.Clipboard}})
		if v.Input.PasteBehavior != driver.PasteInsert {
			return
		}
		text := a.Clipboard
		if !v.Input.Multiline {
			text = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(text)
		}
		if insert, ok := driver.LimitEdit(v.Text, end-start, text, v.Input.MaxLength); ok && insert != "" {
			a.edit(v, v.Text[:left]+insert+v.Text[right:])
		}
	case driver.MenuDelete:
		if left != right {
			a.edit(v, v.Text[:left]+v.Text[right:])
		}
	default:
		a.t.Fatalf("nsgotest: the menu role %q is not modelled headless in a field", role.Label())
	}
}

// heldKey is a key KeyDown holds: tree and id are the surface and the view its down went to, id 0 for none, and reported whether the platform would report its up, which it would not after a down while a field or web view had the focus, or with no view hearing keys while the surface did not ask for every key (driver.MountItem.HeldKeys).
type heldKey struct {
	tree       *core.Tree
	id         driver.NodeID
	name, code string
	reported   bool
}

// keyDown offers key k, normalized, to the focused view that hears keys go down (View.OnKeyDown), as the platforms do before the keymap, and reports whether it consumed the key. A modifier alone ("shift", "mod") is reported with its own modifier held and never consumed. With no such view, the surface that asks for every key hears it with no view, unless a field or web view has the focus. A key not held yet is held from here, with where its up goes; repeat is whether it was held already, which neither changes.
func (a *App) keyDown(k, name, code, identity string, repeat bool) bool {
	var focused *View
	editing := false
	for _, v := range a.shown() {
		if !v.Focused {
			continue
		}
		if v.Kind == KindInput || v.Kind == KindWeb {
			editing = true
		} else if v.Events.Has(driver.EventKeyDown) && focused == nil {
			focused = v
		}
	}
	parts := strings.Split(k, "+")
	held := parts[:len(parts)-1]
	modifier := modifierKey(name)
	if modifier {
		held = append(held, name)
	}
	ev := driver.Event{Kind: driver.EventKeyDown, Text: name, KeyCode: code, Modifiers: a.keyModifiers(held), Repeat: repeat}
	entry, consumed := heldKey{}, false
	switch {
	case editing:
	case focused != nil:
		ev.ID = focused.ID
		entry = heldKey{tree: a.tree, id: focused.ID, reported: true}
		consumed = a.tree.KeyDown(ev)
	case a.heldKeys:
		entry = heldKey{tree: a.tree, reported: true}
		a.tree.Dispatch(ev)
	}
	if !repeat {
		if a.held == nil {
			a.held = map[string]heldKey{}
		}
		entry.name, entry.code = name, code
		a.held[identity] = entry
	}
	a.Settle()
	return consumed && !modifier
}

// release lets key name, held, come up: the view its down went to hears it (EventKeyUp) with the modifiers still held and extra, those Key pressed with it.
func (a *App) release(identity string, extra driver.Modifiers) {
	h, ok := a.held[identity]
	if !ok {
		return
	}
	delete(a.held, identity)
	if !h.reported {
		return
	}
	mods := a.heldModifiers()
	mods.Control, mods.Alt, mods.Shift, mods.Platform = mods.Control || extra.Control, mods.Alt || extra.Alt, mods.Shift || extra.Shift, mods.Platform || extra.Platform
	h.tree.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: h.id, Text: h.name, KeyCode: h.code, Modifiers: mods})
	a.Settle()
}

// withHeld returns key with the modifiers KeyDown holds before it, its own aside.
func (a *App) withHeld(key string) string {
	parts := strings.Split(driver.NormalizeKey(key), "+")
	own := a.keyName(parts[len(parts)-1])
	held := a.heldModifiers()
	var mods []string
	for _, m := range []struct {
		name string
		on   bool
	}{{"ctrl", held.Control}, {"alt", held.Alt}, {"shift", held.Shift}, {"meta", held.Platform}} {
		if m.on && m.name != own {
			mods = append(mods, m.name)
		}
	}
	if len(mods) == 0 {
		return key
	}
	return strings.Join(append(mods, key), "+")
}

// heldModifiers are the modifiers KeyDown holds.
func (a *App) heldModifiers() driver.Modifiers {
	var names []string
	for _, key := range a.held {
		if modifierKey(key.name) {
			names = append(names, key.name)
		}
	}
	return a.keyModifiers(names)
}

// keyName names a key, normalized and without modifiers, as the platforms report it (core.KeyEvent.Key): a modifier by its own name, "mod" as "meta" on Apple platforms and "ctrl" elsewhere.
func (a *App) keyName(name string) string {
	switch name {
	case "control":
		return "ctrl"
	case "option":
		return "alt"
	case "cmd", "command", "super", "win":
		return "meta"
	case "mod":
		if a.keyModifiers([]string{"mod"}).Platform {
			return "meta"
		}
		return "ctrl"
	}
	return name
}

// modifierKey reports whether the key keyName names is a modifier.
func modifierKey(name string) bool {
	switch name {
	case "shift", "ctrl", "alt", "meta":
		return true
	}
	return false
}

// keyModifiers reads normalized key prefixes; mod is Command on Apple platforms and Control elsewhere, while meta is Command or the system key.
func (a *App) keyModifiers(names []string) driver.Modifiers {
	var mods driver.Modifiers
	apple := a.env != nil && (a.env.OS == driver.OSMacOS || a.env.OS == driver.OSIOS)
	for _, name := range names {
		switch name {
		case "ctrl":
			mods.Control = true
		case "alt":
			mods.Alt = true
		case "shift":
			mods.Shift = true
		case "meta":
			mods.Platform = true
		case "mod":
			if apple {
				mods.Platform = true
			} else {
				mods.Control = true
			}
		}
	}
	return mods
}

// focusKeyUse returns the keys the view with the keyboard focus uses itself (driver.FocusKeyUse), which the keymap's bindings that yield them leave it, as on a device.
func (a *App) focusKeyUse() driver.KeyUse {
	var use driver.KeyUse
	for _, v := range a.views {
		if v.Focused {
			use |= driver.FocusKeyUse(v.Kind.driverKind(), v.Props.Role, v.Events.Pressable())
		}
	}
	return use
}

// shifted is what the key typing c types with Shift on the US layout.
func shifted(c string) string {
	const plain, shift = "`1234567890-=[]\\;',./", "~!@#$%^&*()_+{}|:\"<>?"
	if i := strings.Index(plain, c); i >= 0 && len(c) == 1 {
		return shift[i : i+1]
	}
	return strings.ToUpper(c)
}

// keyNames are the names a press of normalized key k matches bindings by, as a platform's: "mod" is Command on Apple platforms and Control elsewhere, whichever of the two the key names. Off Apple platforms the "mod" spelling comes first, as Windows and Linux try a press's spellings in that order against their menu shortcuts (driver.MenuShortcut).
func (a *App) keyNames(k string) []string {
	cmd := "ctrl"
	if a.env != nil && (a.env.OS == driver.OSMacOS || a.env.OS == driver.OSIOS) {
		cmd = "meta"
	}
	parts := strings.Split(k, "+")
	mods, name := parts[:len(parts)-1], parts[len(parts)-1]
	swap := func(from, to string) string {
		out := []string{}
		for _, m := range mods {
			if m == from {
				m = to
			}
			out = append(out, m)
		}
		return driver.NormalizeKey(strings.Join(append(out, name), "+"))
	}
	switch {
	case slices.Contains(mods, "mod"):
		return []string{k, swap("mod", cmd)}
	case cmd == "ctrl" && slices.Contains(mods, "ctrl"):
		return []string{swap("ctrl", "mod"), k}
	case slices.Contains(mods, cmd):
		return []string{k, swap(cmd, "mod")}
	}
	return []string{k}
}

// tabs says whether Tab moves the keyboard focus between pressable views and fields, as on desktops and the web.
func (a *App) tabs() bool {
	return a.env != nil && a.env.FocusesViews() && a.env.OS != driver.OSAndroid &&
		(a.env.OS != driver.OSIOS || a.env.Idiom == driver.IdiomTV)
}

// moveFocus moves the keyboard focus to the next Tab stop, or the
// previous one, around: editable fields and the pressable views not
// NoFocus, disabled or out of sight and the Focusable ones, in tree
// order.
func (a *App) moveFocus(back bool) {
	var stops []*View
	at := -1
	for _, v := range a.tabOrder(a.shown()) {
		if !focusable(v) {
			continue
		}
		if v.Focused {
			at = len(stops)
		}
		stops = append(stops, v)
	}
	if len(stops) == 0 {
		return
	}
	next := 0
	switch {
	case at >= 0 && back:
		next = (at + len(stops) - 1) % len(stops)
	case at >= 0:
		next = (at + 1) % len(stops)
	case back:
		next = len(stops) - 1
	}
	for _, v := range a.views {
		if v != stops[next] {
			a.setFocus(v, false)
		}
	}
	a.setFocus(stops[next], true)
}

// tabOrder puts views, in tree order, in the Tab order: each dialog
// overlay's views right after the view its TabAfter names, before that
// view's children, as a popover's content follows its trigger.
func (a *App) tabOrder(views []*View) []*View {
	moved := false
	for _, v := range views {
		if v.Props.TabAfter != 0 {
			moved = true
			break
		}
	}
	if !moved {
		return views
	}
	// The views inside each overlay, by the overlay.
	inside := map[*View][]*View{}
	var rest []*View
	for _, v := range views {
		o := v.upTo(func(u *View) bool { return u.Props.TabAfter != 0 })
		if o == nil {
			rest = append(rest, v)
			continue
		}
		inside[o] = append(inside[o], v)
	}
	placed := map[driver.NodeID]bool{}
	for _, v := range rest {
		placed[v.ID] = true
	}
	after := map[driver.NodeID][]*View{}
	for _, v := range views {
		if o := v; v.Props.TabAfter != 0 {
			if placed[o.Props.TabAfter] {
				after[o.Props.TabAfter] = append(after[o.Props.TabAfter], o)
			} else {
				rest = append(rest, inside[o]...)
			}
		}
	}
	var out []*View
	var put func(v *View)
	put = func(v *View) {
		out = append(out, v)
		for _, o := range after[v.ID] {
			for _, w := range inside[o] {
				put(w)
			}
		}
	}
	for _, v := range rest {
		put(v)
	}
	return out
}

// Back goes back as Escape, the back button or the back gesture does.
func (a *App) Back() {
	a.t.Helper()
	a.step("back")
	if a.target != nil {
		a.target.back()
		a.done()
		return
	}
	a.dispatch(driver.Event{Kind: driver.EventBack, ID: a.rootID})
	a.done()
}

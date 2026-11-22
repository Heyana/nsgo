//go:build linux && !android

package linux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// TestApplicationMenuBuildsAndAnswersItsShortcut shows the app menu as a
// GTK menu bar and runs the item a native key press matches.
func TestApplicationMenuBuildsAndAnswersItsShortcut(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-menu-" + strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	app := func(cx *core.Scope) core.Node {
		bumped := core.NewSignal(cx, 0)
		cx.SetApplicationMenu([]core.MenuItem{
			{Label: "File", Items: []core.MenuItem{
				{Label: "Bump", Shortcut: "mod+b", OnSelect: func() { bumped.Update(func(n int) int { return n + 1 }) }},
				{Separator: true},
				{Label: "Quit", Role: driver.MenuQuit},
			}},
		})
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%d", title, bumped.Get())) })
		return core.View{Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(100)}}
	}
	w := startTestWindow(t, engine.App(app), Options{Title: title, Width: 200, Height: 100})
	waitForTitle(t, w, title+"|0")
	w.onLoop(t, func() {
		if w.menuBar == 0 {
			t.Error("the application menu has no GTK menu bar")
		}
		if gtkWidgetGetParent(w.menuBar) != w.box {
			t.Error("the menu bar is not above the surface")
		}
	})
	w.onLoop(t, func() { keyPressedCallback(0, 'b', 0, gdkControl, w.handle) })
	waitForTitle(t, w, title+"|1")

	// An app rebuilds its menu whenever a checkable item changes. The
	// rebuilds must not run out of purego callbacks or GTK actions.
	w.onLoop(t, func() {
		entries := w.chrome.menu
		for range 250 {
			w.setMenu(entries)
		}
		if w.menuBar == 0 {
			t.Error("the menu bar went away after rebuilding")
		}
	})
}

func TestMenuYieldFollowsNativeTextFocus(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	const title = "nsgo-menu-yield"
	app := func(cx *core.Scope) core.Node {
		menu, binding, focus := core.NewSignal(cx, 0), core.NewSignal(cx, 0), core.NewSignal(cx, false)
		cx.SetApplicationMenu([]core.MenuItem{{Label: "Actions", Items: []core.MenuItem{
			{Label: "Refresh", Shortcut: "mod+r", When: core.MenuNotEditing, OnSelect: func() { menu.Update(func(n int) int { return n + 1 }) }},
		}}})
		cx.OnKeyWith("mod+r", core.KeyOptions{YieldsToMenu: true}, func() { binding.Update(func(n int) int { return n + 1 }) })
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%d|%d|%v", title, menu.Get(), binding.Get(), focus.Get())) })
		return core.Input{OnFocus: func() { focus.Set(true) }, OnBlur: func() { focus.Set(false) },
			Style: core.Style{Width: driver.Pt(180), Height: driver.Pt(30)}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 240, Height: 120})
	waitForTitle(t, w, title+"|0|0|false")
	press := func() {
		w.onLoop(t, func() {
			keyPressedCallback(0, 'r', 27, gdkControl, w.handle)
			keyReleasedCallback(0, 'r', 27, gdkControl, w.handle)
		})
	}
	press()
	waitForTitle(t, w, title+"|1|0|false")
	w.onLoop(t, func() {
		for _, v := range w.entries {
			if v.kind == driver.KindInput {
				w.focusEntry(v)
				break
			}
		}
	})
	waitForFocusOrSkip(t, w, "the input focused", func() bool { return w.focusedEntry() != nil })
	waitForTitle(t, w, title+"|1|0|true")
	press()
	waitForTitle(t, w, title+"|1|1|true")
	w.onLoop(t, func() { gtkWindowSetFocus(w.win, 0) })
	waitForTitle(t, w, title+"|1|1|false")
	press()
	waitForTitle(t, w, title+"|2|1|false")
}

// TestMenuYieldReachesTheBindingTheMenuLeaves presses keys the GTK menu never takes from a focused field, a MenuNotEditing item's Control+R with a later item of the same shortcut, and an item's unmodified F5, against bindings that yield to the menu: with the field focused each binding runs, as the core keeps it in the keymap, and with no field focused the first item runs (#710).
func TestMenuYieldReachesTheBindingTheMenuLeaves(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	const title = "nsgo-menu-left"
	app := func(cx *core.Scope) core.Node {
		refresh, run, reload, bindingR, bindingF5, focus := core.NewSignal(cx, 0), core.NewSignal(cx, 0), core.NewSignal(cx, 0), core.NewSignal(cx, 0), core.NewSignal(cx, 0), core.NewSignal(cx, false)
		bump := func(s *core.Signal[int]) func() { return func() { s.Update(func(n int) int { return n + 1 }) } }
		cx.SetApplicationMenu([]core.MenuItem{{Label: "Actions", Items: []core.MenuItem{
			{Label: "Refresh", Shortcut: "mod+r", When: core.MenuNotEditing, OnSelect: bump(refresh)},
			{Label: "Run", Shortcut: "mod+r", OnSelect: bump(run)},
			{Label: "Reload", Shortcut: "f5", OnSelect: bump(reload)},
		}}})
		cx.OnKeyWith("mod+r", core.KeyOptions{YieldsToMenu: true}, bump(bindingR))
		cx.OnKeyWith("f5", core.KeyOptions{YieldsToMenu: true}, bump(bindingF5))
		cx.Effect(func() {
			cx.SetTitle(fmt.Sprintf("%s|%d%d%d|%d%d|%v", title, refresh.Get(), run.Get(), reload.Get(), bindingR.Get(), bindingF5.Get(), focus.Get()))
		})
		return core.Input{OnFocus: func() { focus.Set(true) }, OnBlur: func() { focus.Set(false) },
			Style: core.Style{Width: driver.Pt(180), Height: driver.Pt(30)}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 240, Height: 120})
	waitForTitle(t, w, title+"|000|00|false")
	press := func(keyval, keycode, state uint32) {
		w.onLoop(t, func() {
			keyPressedCallback(0, keyval, keycode, state, w.handle)
			keyReleasedCallback(0, keyval, keycode, state, w.handle)
		})
	}
	const f5 = gdkF1 + 4
	w.onLoop(t, func() {
		for _, v := range w.entries {
			if v.kind == driver.KindInput {
				w.focusEntry(v)
				break
			}
		}
	})
	waitForFocusOrSkip(t, w, "the input focused", func() bool { return w.focusedEntry() != nil })
	waitForTitle(t, w, title+"|000|00|true")
	press('r', 27, gdkControl)
	waitForTitle(t, w, title+"|000|10|true")
	press(f5, 71, 0)
	waitForTitle(t, w, title+"|000|11|true")
	w.onLoop(t, func() { gtkWindowSetFocus(w.win, 0) })
	waitForTitle(t, w, title+"|000|11|false")
	press('r', 27, gdkControl)
	waitForTitle(t, w, title+"|100|11|false")
	press(f5, 71, 0)
	waitForTitle(t, w, title+"|101|11|false")
}

func TestMenuYieldFollowsNativeWebFocus(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	// Give WebKit its own process and profile, as the other native WebKit
	// tests do, so the desktop session's web data cannot affect startup.
	if os.Getenv("NSGO_TEST_MENU_WEB") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		dir := t.TempDir()
		cmd.Env = append(os.Environ(), "NSGO_TEST_MENU_WEB="+t.Name(),
			"XDG_DATA_HOME="+filepath.Join(dir, "data"), "XDG_CACHE_HOME="+filepath.Join(dir, "cache"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			// WebKit traps its process when bubblewrap cannot launch its
			// D-Bus proxy. An Xvfb container can deny user namespaces even
			// when the host is not using the AppArmor restriction.
			log := string(out)
			if strings.Contains(log, "bwrap: Creating new namespace failed: Operation not permitted") &&
				strings.Contains(log, "Failed to fully launch dbus-proxy") {
				t.Skip("WebKitGTK could not start its sandbox because bubblewrap was denied a user namespace")
			}
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP: "+t.Name()+" ") {
			t.Skipf("%s", out)
		}
		return
	}
	if err := openWebKit(); err != nil {
		t.Skip(err)
	}
	const title = "nsgo-menu-web-yield"
	app := func(cx *core.Scope) core.Node {
		menu, binding, loaded := core.NewSignal(cx, 0), core.NewSignal(cx, 0), core.NewSignal(cx, false)
		cx.SetApplicationMenu([]core.MenuItem{{Label: "Actions", Items: []core.MenuItem{
			{Label: "Refresh", Shortcut: "mod+r", When: core.MenuNotEditing, OnSelect: func() { menu.Update(func(n int) int { return n + 1 }) }},
		}}})
		cx.OnKeyWith("mod+r", core.KeyOptions{YieldsToMenu: true}, func() { binding.Update(func(n int) int { return n + 1 }) })
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%d|%d|%v", title, menu.Get(), binding.Get(), loaded.Get())) })
		return core.WebView{Source: core.WebHTML("<!doctype html><input>", ""),
			OnLoad: func(core.WebNavigation) { loaded.Set(true) }, Style: core.Style{Width: driver.Pt(240), Height: driver.Pt(120)}}
	}
	w := startTestWindow(t, app, Options{ID: "nsgo-test-web-menu-yield", Title: title, Width: 280, Height: 160})
	waitForTitle(t, w, title+"|0|0|true")
	setFocus := func(focused bool) {
		t.Helper()
		w.onLoop(t, func() {
			if !focused {
				gtkWindowSetFocus(w.win, 0)
				return
			}
			for _, v := range w.views {
				if v != nil && v.web != nil && v.web.widget != 0 {
					gtkWidgetGrabFocus(v.web.widget)
					return
				}
			}
			t.Error("no web view")
		})
		waitForFocusOrSkip(t, w, fmt.Sprintf("keymap following web focus %v", focused), func() bool {
			bound := slices.ContainsFunc(w.keys, func(k driver.KeyBinding) bool { return k.Key == "mod+r" })
			return bound == focused && (w.focusedWeb() != nil) == focused
		})
	}
	press := func() {
		w.onLoop(t, func() {
			keyPressedCallback(0, 'r', 27, gdkControl, w.handle)
			keyReleasedCallback(0, 'r', 27, gdkControl, w.handle)
		})
	}
	setFocus(false)
	press()
	waitForTitle(t, w, title+"|1|0|true")
	setFocus(true)
	press()
	waitForTitle(t, w, title+"|1|1|true")
	setFocus(false)
	press()
	waitForTitle(t, w, title+"|2|1|true")
}

// waitForFocusOrSkip waits for ready, which reads the window on its loop, to hold. When it does not in time the test skips if the window manager never activated the window, as GNOME may refuse a window opened without a recent user interaction where Xvfb has no window manager to refuse it, and fails if the window was active.
func waitForFocusOrSkip(t *testing.T, w *window, what string, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		ok, active := false, false
		w.onLoop(t, func() { ok, active = ready(), gtkWindowIsActive(w.win) != 0 })
		if ok {
			return
		}
		if time.Now().After(deadline) {
			if !active {
				t.Skipf("the window manager did not activate the window, so %s cannot be tested", what)
			}
			t.Fatalf("want %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestRadioMenuItemsActivate builds radio items, whose actions GTK activates with the item's target, so each takes a string parameter and is string-stated, the checked one's state its target; choosing the other runs it. The roles only macOS has, and the Help holder with no items, are left out.
func TestRadioMenuItemsActivate(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	// The Speech item stays out of the menu, whose actions are the radios'.
	speechAs(t, false)
	var (
		activate  func(group uintptr, name *byte, parameter uintptr)
		getName   func(action uintptr) string
		paramType func(action uintptr) uintptr
		getState  func(action uintptr) uintptr
		getString func(v uintptr, length uintptr) string
		unref     func(v uintptr)
	)
	title := "nsgo-menu-" + strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	app := func(cx *core.Scope) core.Node {
		library := core.NewSignal(cx, "Movies")
		cx.Effect(func() {
			lib := library.Get()
			radio := func(name string) core.MenuItem {
				return core.MenuItem{Label: name, Radio: true, Checked: lib == name, OnSelect: func() { library.Set(name) }}
			}
			cx.SetApplicationMenu([]core.MenuItem{
				{Label: "Library", Items: []core.MenuItem{radio("Movies"), radio("Courses"), {Separator: true},
					{Role: core.MenuServices}, {Role: core.MenuStartSpeaking}, {Role: core.MenuBringAllToFront}}},
				{Label: "Help", Role: core.MenuHelpMenu},
			})
			cx.SetTitle(title + "|" + lib)
		})
		return core.View{Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(100)}}
	}
	w := startTestWindow(t, engine.App(app), Options{Title: title, Width: 200, Height: 100})
	waitForTitle(t, w, title+"|Movies")
	// The libraries are loaded once the window is.
	purego.RegisterLibFunc(&activate, gioLib, "g_action_group_activate_action")
	purego.RegisterLibFunc(&getName, gioLib, "g_action_get_name")
	purego.RegisterLibFunc(&paramType, gioLib, "g_action_get_parameter_type")
	purego.RegisterLibFunc(&getState, gioLib, "g_action_get_state")
	purego.RegisterLibFunc(&getString, glibLib, "g_variant_get_string")
	purego.RegisterLibFunc(&unref, glibLib, "g_variant_unref")
	var courses string
	w.onLoop(t, func() {
		if len(w.actionRefs) != 2 {
			t.Errorf("the menu has %d actions, want the two radios'", len(w.actionRefs))
		}
		for action := range w.actionRefs {
			name := getName(action)
			if paramType(action) == 0 {
				t.Errorf("%s takes no parameter, so GTK cannot activate it with its target", name)
			}
			state := getState(action)
			if state == 0 {
				t.Errorf("%s has no state", name)
				continue
			}
			checked := getString(state, 0) == radioTarget
			unref(state)
			if !checked {
				courses = name
			}
		}
	})
	if courses == "" {
		t.Fatal("no radio is unchecked")
	}
	w.onLoop(t, func() { activate(w.actionGroup, cstr(courses), gVariantNewString(cstr(radioTarget))) })
	waitForTitle(t, w, title+"|Courses")
}

// TestStartSpeakingReadsTheFocusedField chooses the Speech items with a field focused: Start Speaking reads its selection, or its whole text with none, nothing of a password field, and Stop Speaking stops what it said.
func TestStartSpeakingReadsTheFocusedField(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	speechAs(t, true)
	type said struct {
		id uint32
		r  driver.SpeakRequest
	}
	heard := make(chan said, 8)
	was := menuSpeak
	menuSpeak = func(id uint32, r driver.SpeakRequest, _ func(driver.Event)) { heard <- said{id, r} }
	t.Cleanup(func() { menuSpeak = was })
	const title = "nsgo-menu-speech"
	app := func(cx *core.Scope) core.Node {
		cx.SetApplicationMenu([]core.MenuItem{{Label: "Edit", Items: []core.MenuItem{{Label: "Speech", Items: []core.MenuItem{
			{Role: core.MenuStartSpeaking}, {Role: core.MenuStopSpeaking}}}}}})
		cx.SetTitle(title)
		return core.View{Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(100)}, Children: []core.Node{
			core.Input{TestID: "field", DefaultValue: "日本語のテキスト", Style: core.Style{Width: driver.Pt(180), Height: driver.Pt(30)}},
			core.Input{TestID: "password", DefaultValue: "secret", Masked: true, Style: core.Style{Width: driver.Pt(180), Height: driver.Pt(30)}},
		}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 240, Height: 120})
	waitForTitle(t, w, title)
	field := func(id string) *view {
		for _, v := range w.entries {
			if v.props.TestID == id {
				return v
			}
		}
		t.Fatalf("no field %s", id)
		return nil
	}
	choose := func(role driver.MenuRole) {
		w.onLoop(t, func() {
			if !w.menuChosen(menuRef{role: role}) {
				t.Errorf("%v was not run", role)
			}
		})
	}
	next := func() (said, bool) {
		select {
		case s := <-heard:
			return s, true
		case <-time.After(500 * time.Millisecond):
			return said{}, false
		}
	}
	choose(driver.MenuStartSpeaking)
	if s, ok := next(); ok {
		t.Errorf("with no field focused, Start Speaking said %+v", s)
	}
	w.onLoop(t, func() {
		v := field("field")
		w.focusEntry(v)
		gtkEditableSelectRegion(v.entry, 4, 8)
	})
	choose(driver.MenuStartSpeaking)
	if s, ok := next(); !ok || s.id != menuUtterance || s.r.Command != driver.SpeakStart || s.r.Text != "テキスト" || s.r.Lang == "" {
		t.Errorf("the selection was said as %+v (%v)", s, ok)
	}
	w.onLoop(t, func() { gtkEditableSelectRegion(field("field").entry, 2, 2) })
	choose(driver.MenuStartSpeaking)
	if s, ok := next(); !ok || s.r.Text != "日本語のテキスト" {
		t.Errorf("the whole text was said as %+v (%v)", s, ok)
	}
	choose(driver.MenuStopSpeaking)
	if s, ok := next(); !ok || s.id != menuUtterance || s.r.Command != driver.SpeakStop {
		t.Errorf("Stop Speaking sent %+v (%v)", s, ok)
	}
	w.onLoop(t, func() {
		v := field("password")
		w.focusEntry(v)
		gtkEditableSelectRegion(v.entry, 0, -1)
	})
	choose(driver.MenuStartSpeaking)
	if s, ok := next(); ok {
		t.Errorf("a password field was said as %+v", s)
	}
}

// TestStartSpeakingReadsTheFocusedPage runs Start Speaking with a WebKitGTK page focused: it reads the selection in the page's text area through WebKit's evaluation of a script, a second Start or a Stop drops a read still pending, and the page's own messages keep reaching the app throughout. It runs in a process of its own, as the other native WebKit tests do.
func TestStartSpeakingReadsTheFocusedPage(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("NSGO_TEST_MENU_WEB") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		dir := t.TempDir()
		cmd.Env = append(os.Environ(), "NSGO_TEST_MENU_WEB="+t.Name(),
			"XDG_DATA_HOME="+filepath.Join(dir, "data"), "XDG_CACHE_HOME="+filepath.Join(dir, "cache"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			log := string(out)
			if strings.Contains(log, "bwrap: Creating new namespace failed: Operation not permitted") &&
				strings.Contains(log, "Failed to fully launch dbus-proxy") {
				t.Skip("WebKitGTK could not start its sandbox because bubblewrap was denied a user namespace")
			}
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP: "+t.Name()+" ") {
			t.Skipf("%s", out)
		}
		return
	}
	if err := openWebKit(); err != nil {
		t.Skip(err)
	}
	speechAs(t, true)
	heard := make(chan driver.SpeakRequest, 8)
	was := menuSpeak
	menuSpeak = func(_ uint32, r driver.SpeakRequest, _ func(driver.Event)) { heard <- r }
	t.Cleanup(func() { menuSpeak = was })
	const title = "nsgo-menu-web-speech"
	// The page selects "page" in its text area and posts a count every 50ms.
	const page = `<!doctype html><textarea id=f>hello page</textarea><script>
const f = document.getElementById('f');
f.focus();
f.setSelectionRange(6, 10);
let n = 0;
setInterval(() => nsgo.postMessage('tick:' + (++n)), 50);
</script>`
	app := func(cx *core.Scope) core.Node {
		last := core.NewSignal(cx, "")
		cx.Effect(func() { cx.SetTitle(title + "|" + last.Get()) })
		return core.WebView{Source: core.WebHTML(page, ""), OnMessage: func(data string) { last.Set(data) },
			Style: core.Style{Width: driver.Pt(240), Height: driver.Pt(120)}}
	}
	w := startTestWindow(t, app, Options{ID: "nsgo-test-web-speech", Title: title, Width: 280, Height: 160})
	ticking := func() {
		t.Helper()
		before := w.titleOf(t)
		deadline := time.Now().Add(10 * time.Second)
		for {
			got := w.titleOf(t)
			if strings.HasPrefix(got, title+"|tick:") && got != before {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("the page's messages stopped reaching the app: title %q", got)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	ticking()
	w.onLoop(t, func() {
		for _, v := range w.views {
			if v != nil && v.web != nil && v.web.widget != 0 {
				gtkWidgetGrabFocus(v.web.widget)
				return
			}
		}
		t.Error("no web view")
	})
	waitForFocusOrSkip(t, w, "the page to take the focus", func() bool { return w.focusedWeb() != nil })
	next := func() (driver.SpeakRequest, bool) {
		select {
		case r := <-heard:
			return r, true
		case <-time.After(2 * time.Second):
			return driver.SpeakRequest{}, false
		}
	}
	w.onLoop(t, func() { w.speakMenu(true) })
	if r, ok := next(); !ok || r.Command != driver.SpeakStart || r.Text != "page" || r.Lang == "" {
		t.Fatalf("the page's selection was said as %+v (%v)", r, ok)
	}
	ticking()
	// A second Start Speaking supersedes the first: the page is read and said once.
	w.onLoop(t, func() { w.speakMenu(true); w.speakMenu(true) })
	if r, ok := next(); !ok || r.Text != "page" {
		t.Fatalf("the second Start Speaking said %+v (%v)", r, ok)
	}
	if r, ok := next(); ok {
		t.Errorf("the superseded read was said too: %+v", r)
	}
	// Stop Speaking drops the read still pending.
	w.onLoop(t, func() { w.speakMenu(true); w.speakMenu(false) })
	if r, ok := next(); !ok || r.Command != driver.SpeakStop {
		t.Fatalf("Stop Speaking sent %+v (%v)", r, ok)
	}
	if r, ok := next(); ok {
		t.Errorf("a read Stop Speaking dropped was said: %+v", r)
	}
	ticking()
}

// TestEditingRolesFallBackToOnSelect chooses Edit menu role items that carry an OnSelect: the focused field and the text selection take Select All and Copy, and with neither the items' OnSelect runs.
func TestEditingRolesFallBackToOnSelect(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	const title = "nsgo-menu-fallback"
	app := func(cx *core.Scope) core.Node {
		selected, copied, undone := core.NewSignal(cx, 0), core.NewSignal(cx, 0), core.NewSignal(cx, 0)
		count := func(s *core.Signal[int]) func() { return func() { s.Update(func(n int) int { return n + 1 }) } }
		cx.SetApplicationMenu([]core.MenuItem{{Label: "Edit", Items: []core.MenuItem{
			{Role: core.MenuUndo, OnSelect: count(undone)},
			{Role: core.MenuCopy, OnSelect: count(copied)},
			{Role: core.MenuSelectAll, OnSelect: count(selected)},
		}}})
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%d|%d|%d", title, selected.Get(), copied.Get(), undone.Get())) })
		return core.View{Style: core.Style{Width: driver.Pt(240), Height: driver.Pt(120)}, Children: []core.Node{
			core.Input{TestID: "field", DefaultValue: "hello", Style: core.Style{Width: driver.Pt(180), Height: driver.Pt(30)}},
			core.View{Selectable: true, Children: []core.Node{core.Text{Content: "Hello world", Style: core.Style{FontSize: 16}}}},
		}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 240, Height: 120})
	waitForTitle(t, w, title+"|0|0|0")
	choose := func(role driver.MenuRole) {
		w.onLoop(t, func() {
			for _, e := range w.chrome.menu[0].Items {
				if e.Role == role {
					if !w.menuChosen(menuRef{id: e.ID, role: e.Role}) {
						t.Errorf("%s was not taken", role.Label())
					}
					return
				}
			}
			t.Errorf("no %s item", role.Label())
		})
	}
	choose(driver.MenuSelectAll)
	choose(driver.MenuCopy)
	waitForTitle(t, w, title+"|1|1|0")

	var field *view
	w.onLoop(t, func() {
		for _, v := range w.entries {
			if v.props.TestID == "field" {
				field = v
				w.focusEntry(v)
			}
		}
	})
	choose(driver.MenuSelectAll)
	w.onLoop(t, func() {
		if start, end := w.selection(field); start != 0 || end != len("hello") {
			t.Errorf("Select All selected %d-%d of the field, want all of it", start, end)
		}
		gtkWindowSetFocus(w.win, 0)
	})
	// Undo falls back with the field blurred, after the menu's reports above, so the counts are final once it shows.
	choose(driver.MenuUndo)
	waitForTitle(t, w, title+"|1|1|1")

	w.onLoop(t, func() {
		for _, v := range w.views {
			if v != nil && v.kind == driver.KindText && v.plain == "Hello world" {
				b, _ := w.surfaceBounds(v)
				at := driver.Point{X: b.Origin.X + 10, Y: b.Origin.Y + b.Size.Height/2}
				w.pointerDown(at, 1, 0, 0, 2)
				w.pointerUp(at)
			}
		}
		if w.selecting == nil || w.selecting.Model.Text(w.selecting) != "Hello" {
			t.Error("double click selected no word")
		}
	})
	choose(driver.MenuSelectAll)
	choose(driver.MenuCopy)
	choose(driver.MenuUndo)
	waitForTitle(t, w, title+"|1|1|2")
	w.onLoop(t, func() {
		if got := w.selecting.Model.Text(w.selecting); got != "Hello world" {
			t.Errorf("Select All selected %q of the text, want all of it", got)
		}
	})
}

// TestMenuBarSyncsInPlace checks that setting the application menu again keeps the GtkPopoverMenuBar, GMenu models, GSimpleActionGroup and unchanged-kind GSimpleActions while updating their state, enabled flag and labels in place.
func TestMenuBarSyncsInPlace(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	var (
		getEnabled func(action uintptr) int32
		getState   func(action uintptr) uintptr
		getBool    func(v uintptr) int32
		unref      func(v uintptr)
	)
	const title = "nsgo-menu-sync-in-place"
	var toggle func()
	app := func(cx *core.Scope) core.Node {
		checked := core.NewSignal(cx, true)
		toggle = func() { cx.Post(func() { checked.Set(!checked.Get()) }) }
		cx.Effect(func() {
			on := checked.Get()
			items := []core.MenuItem{
				{Label: "Sidebar", Checkable: true, Checked: on, OnSelect: func() {}},
				{Separator: true},
				{Label: "Button", Disabled: !on, OnSelect: func() {}},
			}
			if !on {
				items = append(items, core.MenuItem{Label: "Link", OnSelect: func() {}})
			}
			menus := []core.MenuItem{{Label: "View", Items: items}}
			if !on {
				menus = append(menus, core.MenuItem{Label: "Help", Items: []core.MenuItem{{Label: "Docs", OnSelect: func() {}}}})
			}
			cx.SetApplicationMenu(menus)
			cx.SetTitle(fmt.Sprintf("%s|%v", title, on))
		})
		return core.View{Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(100)}}
	}
	w := startTestWindow(t, engine.App(app), Options{Title: title, Width: 240, Height: 120})
	waitForTitle(t, w, title+"|true")
	purego.RegisterLibFunc(&getEnabled, gioLib, "g_action_get_enabled")
	purego.RegisterLibFunc(&getState, gioLib, "g_action_get_state")
	purego.RegisterLibFunc(&getBool, glibLib, "g_variant_get_boolean")
	purego.RegisterLibFunc(&unref, glibLib, "g_variant_unref")

	var bar1, model1, group1, viewMenu1, sidebarAct1, buttonAct1 uintptr
	w.onLoop(t, func() {
		bar1, model1, group1 = w.menuBar, w.menuModel, w.actionGroup
		if bar1 == 0 || model1 == 0 || group1 == 0 {
			t.Fatalf("menu bar not initialized: bar=%#x model=%#x group=%#x", bar1, model1, group1)
		}
		viewMenu1 = w.menuTree[0].menu
		sidebarAct1 = w.menuTree[0].sections[0].items[0].action
		buttonAct1 = w.menuTree[0].sections[1].items[0].action
		st := getState(sidebarAct1)
		if st == 0 || getBool(st) == 0 {
			t.Error("Sidebar should start checked")
		}
		if st != 0 {
			unref(st)
		}
		if getEnabled(buttonAct1) == 0 {
			t.Error("Button should start enabled")
		}
	})

	toggle()
	waitForTitle(t, w, title+"|false")

	w.onLoop(t, func() {
		if w.menuBar != bar1 || w.menuModel != model1 || w.actionGroup != group1 {
			t.Fatalf("menu bar rebuilt: bar %#x->%#x model %#x->%#x group %#x->%#x",
				bar1, w.menuBar, model1, w.menuModel, group1, w.actionGroup)
		}
		if len(w.menuTree) != 2 || w.menuTree[0].menu != viewMenu1 {
			t.Fatalf("View submenu GMenu changed or Help not added: len=%d view=%#x", len(w.menuTree), w.menuTree[0].menu)
		}
		if got := w.menuTree[0].sections[0].items[0].action; got != sidebarAct1 {
			t.Fatalf("Sidebar action replaced: %#x -> %#x", sidebarAct1, got)
		}
		if got := w.menuTree[0].sections[1].items[0].action; got != buttonAct1 {
			t.Fatalf("Button action replaced: %#x -> %#x", buttonAct1, got)
		}
		st := getState(sidebarAct1)
		if st == 0 || getBool(st) != 0 {
			t.Error("Sidebar stayed checked after in-place update")
		}
		if st != 0 {
			unref(st)
		}
		if getEnabled(buttonAct1) != 0 {
			t.Error("Button was not disabled in place")
		}
		if len(w.actionRefs) != 4 {
			t.Errorf("actionRefs count = %d, want 4 (Sidebar, Button, Link, Docs)", len(w.actionRefs))
		}
	})
}

// TestWindowShowsWithFirstBatch checks that a non-hidden window stays unshown until its first mount batch applies and that a Hidden window stays hidden.
func TestWindowShowsWithFirstBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	t.Run("visible", func(t *testing.T) {
		gate := make(chan struct{})
		entered := make(chan struct{}, 1)
		defer func() {
			select {
			case <-gate:
			default:
				close(gate)
			}
		}()
		const title = "nsgo-first-batch-show"
		w := startTestWindow(t, func(cx *core.Scope) core.Node {
			select {
			case entered <- struct{}{}:
			default:
			}
			<-gate
			cx.SetTitle(title + "|mounted")
			return core.View{Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(100)}}
		}, Options{Title: title, Width: 240, Height: 120})

		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("app function did not start")
		}
		w.onLoop(t, func() {
			if gtkWidgetGetVisible(w.win) != 0 {
				t.Error("window showed before its first batch was applied")
			}
			if !w.showFirst.pending {
				t.Error("showFirst was not pending before the first batch")
			}
		})
		close(gate)
		waitForTitle(t, w, title+"|mounted")
		w.onLoop(t, func() {
			if gtkWidgetGetVisible(w.win) == 0 {
				t.Error("window did not show after its first batch was applied")
			}
			if w.showFirst.pending {
				t.Error("showFirst stayed pending after the first batch")
			}
		})
	})

	t.Run("hidden", func(t *testing.T) {
		const hiddenTitle = "nsgo-first-batch-hidden"
		hidden := startTestWindow(t, func(cx *core.Scope) core.Node {
			cx.SetTitle(hiddenTitle + "|mounted")
			return core.View{Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(100)}}
		}, Options{Title: hiddenTitle, Width: 240, Height: 120, Hidden: true})
		defer hidden.onLoop(t, hidden.close)
		waitForTitle(t, hidden, hiddenTitle+"|mounted")
		hidden.onLoop(t, func() {
			if gtkWidgetGetVisible(hidden.win) != 0 {
				t.Error("hidden window showed after its first batch")
			}
		})
	})
}

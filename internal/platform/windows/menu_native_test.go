//go:build windows

package windows

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestHiddenTitleBarMenuPopsUp opens the application menu of a window with a hidden title bar, which has no menu bar: Alt alone and F10 open it as a popup whose items do what the bar's do, Alt with a top-level item's first letter opens its menu or chooses it, and Alt with any other key, or after a click, stays Windows'. With the system title bar the bar takes the keys as before.
func TestHiddenTitleBarMenuPopsUp(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	chosen := make(chan string, 4)
	scopes := make(chan *core.Scope, 1)
	app := func(cx *core.Scope) core.Node {
		cx.SetApplicationMenu([]core.MenuItem{
			{Label: "File", Items: []core.MenuItem{{Label: "New", OnSelect: func() { chosen <- "New" }}}},
			{Label: "Sync", OnSelect: func() { chosen <- "Sync" }},
		})
		scopes <- cx
		return core.View{Style: core.Style{Background: driver.RGB(0xffffff)}}
	}
	hwnd := startApp(t, "nsgo-hidden-title-menu", app, 360, 240)
	var cx *core.Scope
	select {
	case cx = <-scopes:
	case <-time.After(5 * time.Second):
		t.Fatal("the app did not start")
	}
	menuBar := func() uintptr {
		var m uintptr
		onMain(t, hwnd, func(*window) { m, _, _ = getMenu.Call(hwnd) })
		return m
	}
	// waitFor waits until cond holds on the main thread.
	waitFor := func(what string, cond func(w *window) bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			ok := false
			onMain(t, hwnd, func(w *window) { ok = cond(w) })
			if ok {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal(what)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	// The system title bar shows the bar, which takes Alt into its menu mode.
	waitFor("no menu bar", func(w *window) bool { return w.chrome.bar != 0 })
	onMain(t, hwnd, func(w *window) {
		w.menuBarKey(wmSysKeyDown, vkMenu, 1<<29)
		if w.menuBarKey(wmSysKeyUp, vkMenu, 1<<29|1<<30|1<<31) || w.menuChar('f') || len(w.popups) > 0 {
			t.Error("Alt was taken from a shown menu bar")
		}
	})

	cx.Post(func() { cx.SetTitleBar(core.TitleBarHidden, 40) })
	waitFor("the title bar did not hide", func(w *window) bool { return w.chrome.titleBar == driver.TitleBarHidden })
	if menuBar() != 0 {
		t.Error("a window with a hidden title bar shows a menu bar")
	}
	// popup sends keys and waits for the menu they opened, returning its entries.
	popup := func(keys string, send func()) []driver.MenuEntry {
		t.Helper()
		send()
		var shown []driver.MenuEntry
		waitFor(keys+" opened no menu", func(w *window) bool {
			if w.popupTracking {
				shown = w.popupShown
			}
			return shown != nil
		})
		return shown
	}
	// end ends the menu's tracking, choosing item id unless it is zero.
	end := func(id uint32) {
		t.Helper()
		onMain(t, hwnd, func(w *window) {
			if id != 0 {
				w.popupChoice = &id
			}
			endMenu.Call()
		})
		waitFor("the menu did not close", func(w *window) bool { return !w.popupTracking && len(w.popups) == 0 })
	}
	wantChosen := func(keys, want string) {
		t.Helper()
		select {
		case got := <-chosen:
			if got != want {
				t.Errorf("%s chose %s, want %s", keys, got, want)
			}
		case <-time.After(3 * time.Second):
			t.Errorf("%s did not choose %s", keys, want)
		}
	}

	shown := popup("Alt", func() {
		postMessageW.Call(hwnd, wmSysKeyDown, vkMenu, 1<<29)
		postMessageW.Call(hwnd, wmSysKeyUp, vkMenu, 1<<29|1<<30|1<<31)
	})
	if len(shown) != 2 || shown[0].Label != "File" || len(shown[0].Items) != 1 || shown[1].Label != "Sync" {
		t.Fatalf("Alt opened %+v, want the application menu", shown)
	}
	end(shown[0].Items[0].ID)
	wantChosen("Alt", "New")

	if shown := popup("F10", func() { postMessageW.Call(hwnd, wmSysKeyDown, vkF10, 0) }); len(shown) != 2 {
		t.Errorf("F10 opened %+v, want the application menu", shown)
	}
	end(0)

	shown = popup("Alt+F", func() { postMessageW.Call(hwnd, wmSysChar, 'F', 1<<29) })
	if len(shown) != 1 || shown[0].Label != "New" {
		t.Fatalf("Alt+F opened %+v, want the File menu", shown)
	}
	end(shown[0].ID)
	wantChosen("Alt+F", "New")

	postMessageW.Call(hwnd, wmSysChar, 's', 1<<29)
	wantChosen("Alt+S", "Sync")

	onMain(t, hwnd, func(w *window) {
		// Alt+F4's key is no mnemonic's, and DefWindowProc closes the window with it.
		if w.menuBarKey(wmSysKeyDown, 0x73, 1<<29) || w.menuChar('x') || len(w.popups) > 0 || w.popupTracking {
			t.Error("Alt with a key no menu has opened a menu")
		}
		// Alt+click: a press in the window makes Alt no longer alone.
		w.menuBarKey(wmSysKeyDown, vkMenu, 1<<29)
		w.hideMenuBar()
		if w.menuBarKey(wmSysKeyUp, vkMenu, 1<<29|1<<30|1<<31) || len(w.popups) > 0 {
			t.Error("releasing Alt after a click opened the menu")
		}
	})
}

// TestConsumedAltChordKeepsMenusClosed sends both kinds of consumed keys
// through Run's message loop. Alt alone still opens either menu.
func TestConsumedAltChordKeepsMenusClosed(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	const altDown, altUp = 1<<29 | 1, 1<<31 | 1<<30 | 1<<29 | 1
	getState, setState := user32.NewProc("GetKeyboardState"), user32.NewProc("SetKeyboardState")
	for _, tc := range []struct {
		name  string
		title driver.TitleBarStyle
	}{
		{"bar", driver.TitleBarDefault},
		{"popup", driver.TitleBarHidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const title = "nsgo-consumed-alt"
			handled := make(chan string, 2)
			hwnd := startApp(t, title, func(cx *core.Scope) core.Node {
				cx.SetMenuBar(core.MenuBarAutoHide)
				cx.SetTitleBar(tc.title, 40)
				cx.SetApplicationMenu([]core.MenuItem{{Label: "Actions", Items: []core.MenuItem{
					{Label: "Example", OnSelect: func() {}},
				}}})
				cx.OnKey("alt+s", func() { handled <- "shortcut" })
				return core.View{TestID: "recorder", Focusable: func() bool { return true },
					OnKeyDown: func(e core.KeyEvent) bool {
						if e.Shortcut() != "alt+a" {
							return false
						}
						handled <- "view"
						return true
					}}
			}, 300, 200)
			onMain(t, hwnd, func(w *window) {
				w.applyBatches()
				for _, v := range w.views {
					if v != nil && v.props.TestID == "recorder" {
						w.focusView(v)
					}
				}
			})
			for _, key := range []struct {
				vk   uintptr
				want string
			}{{'A', "view"}, {'S', "shortcut"}} {
				var saved [256]byte
				onMain(t, hwnd, func(*window) {
					getState.Call(uintptr(unsafe.Pointer(&saved)))
					held := saved
					held[vkMenu] |= 0x80
					setState.Call(uintptr(unsafe.Pointer(&held)))
					postMessageW.Call(hwnd, wmSysKeyDown, vkMenu, altDown)
					postMessageW.Call(hwnd, wmSysKeyDown, key.vk, altDown)
				})
				select {
				case got := <-handled:
					if got != key.want {
						t.Errorf("Alt+%c reached %s, want %s", key.vk, got, key.want)
					}
				case <-time.After(3 * time.Second):
					t.Errorf("Alt+%c did not reach the %s", key.vk, key.want)
				}
				onMain(t, hwnd, func(w *window) {
					setState.Call(uintptr(unsafe.Pointer(&saved)))
					if w.menuBarKey(wmSysKeyUp, vkMenu, altUp) || w.chrome.revealed || len(w.popups) != 0 {
						t.Errorf("releasing Alt after Alt+%c opened the %s", key.vk, tc.name)
						w.hideMenuBar()
						w.popups = nil
					}
				})
			}
			onMain(t, hwnd, func(w *window) {
				w.menuBarKey(wmSysKeyDown, vkMenu, altDown)
				if !w.menuBarKey(wmSysKeyUp, vkMenu, altUp) || !w.chrome.revealed && len(w.popups) == 0 {
					t.Errorf("Alt alone did not open the %s", tc.name)
				}
				w.popups = nil
			})
		})
	}
}

// TestMenuLeavesOutMacRolesAndDrawsRadios builds a menu with the roles only macOS has, which leave no separator at either end of their group nor an empty submenu, the Speech items, which show only where Windows speaks, a Help holder with no items, which is left out, and radio items, which take MFT_RADIOCHECK.
func TestMenuLeavesOutMacRolesAndDrawsRadios(t *testing.T) {
	getItem := user32.NewProc("GetMenuItemInfoW")
	getString := user32.NewProc("GetMenuStringW")
	count := user32.NewProc("GetMenuItemCount")
	type menuItemInfo struct {
		cbSize, fMask, fType, fState, wID    uint32
		hSubMenu, hbmpChecked, hbmpUnchecked uintptr
		dwItemData, dwTypeData               uintptr
		cch                                  uint32
		hbmpItem                             uintptr
	}
	// items returns menu's items as labels, "-" for a separator, with "(o)" for a radio and "*" when checked, and their submenus.
	var items func(menu uintptr) []string
	items = func(menu uintptr) []string {
		var out []string
		n, _, _ := count.Call(menu)
		for i := range n {
			mi := menuItemInfo{fMask: 0x100 | 0x1 | 0x4} // MIIM_FTYPE, MIIM_STATE, MIIM_SUBMENU
			mi.cbSize = uint32(unsafe.Sizeof(mi))
			getItem.Call(menu, i, 1, uintptr(unsafe.Pointer(&mi)))
			if mi.fType&mfSeparator != 0 {
				out = append(out, "-")
				continue
			}
			buf := make([]uint16, 128)
			getString.Call(menu, i, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0x400)
			s := syscall.UTF16ToString(buf)
			if mi.fType&mfRadioCheck != 0 {
				s += "(o)"
			}
			if mi.fState&mfChecked != 0 {
				s += "*"
			}
			out = append(out, s)
			if mi.hSubMenu != 0 {
				out = append(out, "["+strings.Join(items(mi.hSubMenu), " ")+"]")
			}
		}
		return out
	}
	entries := []driver.MenuEntry{
		{Label: "App", Items: []driver.MenuEntry{{Role: driver.MenuAbout}, {Separator: true}, {Role: driver.MenuServices, Items: []driver.MenuEntry{{Label: "Mine", ID: 1}}},
			{Separator: true}, {Role: driver.MenuQuit, Label: "Quit"}}},
		{Label: "Edit", Items: []driver.MenuEntry{{Role: driver.MenuDelete, Label: "Delete"}, {Separator: true},
			{Label: "Speech", Items: []driver.MenuEntry{{Role: driver.MenuStartSpeaking}, {Role: driver.MenuStopSpeaking}}}}},
		{Label: "Library", Items: []driver.MenuEntry{{Label: "Movies", ID: 2, Radio: true, Checkable: true, Checked: true}, {Label: "Courses", ID: 3, Radio: true, Checkable: true}}},
		{Label: "Window", Role: driver.MenuWindowMenu, Items: []driver.MenuEntry{{Role: driver.MenuMinimize, Label: "Minimize"}, {Separator: true}, {Role: driver.MenuBringAllToFront}}},
		{Label: "Help", Role: driver.MenuHelpMenu},
	}
	for _, c := range []struct {
		speech bool
		want   string
	}{
		{false, "App [Quit\tAlt+F4] Edit [Delete] Library [Movies(o)* Courses(o)] Window [Minimize]"},
		{true, "App [Quit\tAlt+F4] Edit [Delete - Speech [Start Speaking Stop Speaking]] Library [Movies(o)* Courses(o)] Window [Minimize]"},
	} {
		speechAs(t, c.speech)
		menu, _, _ := createPopupMenu.Call()
		var refs []menuRef
		var bitmaps []uintptr
		(&window{}).appendMenuTo(menu, entries, &refs, &bitmaps)
		if got := strings.Join(items(menu), " "); got != c.want {
			t.Errorf("with speech %v, the menu is %q, want %q", c.speech, got, c.want)
		}
		destroyMenu.Call(menu)
		freeBitmaps(bitmaps)
	}
}

// TestMenuYieldFollowsNativeFocus presses Control+R with a MenuNotEditing item and a binding that yields to it: the item takes the key outside any field, the binding in a field and in a page, whose focus WebView2's focus events report, and the item again once the focus leaves.
func TestMenuYieldFollowsNativeFocus(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if _, err := findWebView2(); err != nil {
		t.Skip(err)
	}
	// WebView2 can leave the test process in E_UNEXPECTED after its composition controller closes. Keep its browser environment and COM apartment out of the process that runs the other native tests, with a profile of its own under temporary APPDATA and LOCALAPPDATA.
	if os.Getenv("NSGO_TEST_WEBVIEW2_MENU_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		dir, err := os.MkdirTemp("", "nsgo-menu-webview2-")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			deadline := time.Now().Add(5 * time.Second)
			for {
				if err := os.RemoveAll(dir); err == nil {
					return
				} else if time.Now().After(deadline) {
					t.Logf("WebView2 profile cleanup: %v", err)
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
		}()
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEBVIEW2_MENU_PROCESS="+t.Name(), "APPDATA="+dir, "LOCALAPPDATA="+dir)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP") {
			t.Skipf("%s", out)
		}
		return
	}
	const title = "nsgo-menu-yield"
	app := func(cx *core.Scope) core.Node {
		menu, binding, loaded := core.NewSignal(cx, 0), core.NewSignal(cx, 0), core.NewSignal(cx, false)
		cx.SetApplicationMenu([]core.MenuItem{{Label: "Actions", Items: []core.MenuItem{
			{Label: "Refresh", Shortcut: "mod+r", When: core.MenuNotEditing, OnSelect: func() { menu.Update(func(n int) int { return n + 1 }) }},
		}}})
		cx.OnKeyWith("mod+r", core.KeyOptions{YieldsToMenu: true}, func() { binding.Update(func(n int) int { return n + 1 }) })
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%d|%d|%v|", title, menu.Get(), binding.Get(), loaded.Get())) })
		return core.View{Style: core.Style{Padding: driver.All(10), Background: driver.RGB(0xffffff)}, Children: []core.Node{
			core.Input{Style: core.Style{Width: driver.Pt(240), Height: driver.Pt(30)}},
			core.WebView{Source: core.WebHTML("<!doctype html><input>", ""), OnLoad: func(core.WebNavigation) { loaded.Set(true) },
				Style: core.Style{Width: driver.Pt(240), Height: driver.Pt(100)}},
		}}
	}
	hwnd := startApp(t, title, app, 300, 220)
	// The first start of the browser process can take a while.
	waitForTitle(t, hwnd, title+"|0|0|true|", 30*time.Second)
	getState, setState := user32.NewProc("GetKeyboardState"), user32.NewProc("SetKeyboardState")
	press := func(want string) {
		t.Helper()
		onMain(t, hwnd, func(w *window) {
			var saved [256]byte
			getState.Call(uintptr(unsafe.Pointer(&saved)))
			held := saved
			held[vkControl] |= 0x80
			setState.Call(uintptr(unsafe.Pointer(&held)))
			defer setState.Call(uintptr(unsafe.Pointer(&saved)))
			// What WebView2's accelerator event does with Control+R while the page has the focus, else the message loop.
			took := false
			for wv := range w.webs {
				if wv.focused {
					took = wv.acceleratorKey(0, 'R', 1)
					break
				}
			}
			if !took {
				took = w.shortcut('R', 1)
			}
			if !took {
				t.Error("nothing took Control+R")
			}
		})
		waitForTitle(t, hwnd, title+"|"+want+"|true|", 5*time.Second)
	}
	// waitFor waits until the keymap holds the binding as bound says and cond holds, on the main thread.
	waitFor := func(what string, bound bool, cond func(w *window) bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			ok := false
			onMain(t, hwnd, func(w *window) {
				ok = cond(w) && slices.ContainsFunc(w.keys, func(k driver.KeyBinding) bool { return k.Key == "mod+r" }) == bound
			})
			if ok {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal(what)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	page := func(w *window) *webView {
		for wv := range w.webs {
			return wv
		}
		return nil
	}
	press("1|0")
	onMain(t, hwnd, func(w *window) {
		for _, v := range w.views {
			if v != nil && v.field != nil {
				w.setFocus(v.field)
			}
		}
	})
	waitFor("the keymap did not take the binding back in the field", true, func(w *window) bool { return w.focusedField() != nil })
	press("1|1")
	onMain(t, hwnd, func(w *window) { w.blurAll() })
	waitFor("the keymap kept the binding outside the field", false, func(w *window) bool { return w.focusedField() == nil })
	press("2|1")
	onMain(t, hwnd, func(w *window) { page(w).takeFocus() })
	waitFor("the keymap did not take the binding back in the page", true, func(w *window) bool { return page(w).focused })
	press("2|2")
	onMain(t, hwnd, func(w *window) { page(w).focusWindow() })
	waitFor("the keymap kept the binding outside the page", false, func(w *window) bool { return !page(w).focused })
	press("3|2")
}

// TestMenuYieldReachesTheBindingTheMenuLeaves presses Control+R with a MenuNotEditing item followed by another item of the same shortcut, and F5 with an item of its own, against bindings that yield to the menu: with a field focused the menu keeps only the first item of a shortcut and leaves Control+R, which the binding takes, while it takes F5 itself; with no field focused the first item of each takes the key (#710).
func TestMenuYieldReachesTheBindingTheMenuLeaves(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	const title = "nsgo-menu-left"
	app := func(cx *core.Scope) core.Node {
		refresh, run, reload, bindingR, bindingF5 := core.NewSignal(cx, 0), core.NewSignal(cx, 0), core.NewSignal(cx, 0), core.NewSignal(cx, 0), core.NewSignal(cx, 0)
		bump := func(s *core.Signal[int]) func() { return func() { s.Update(func(n int) int { return n + 1 }) } }
		cx.SetApplicationMenu([]core.MenuItem{{Label: "Actions", Items: []core.MenuItem{
			{Label: "Refresh", Shortcut: "mod+r", When: core.MenuNotEditing, OnSelect: bump(refresh)},
			{Label: "Run", Shortcut: "mod+r", OnSelect: bump(run)},
			{Label: "Reload", Shortcut: "f5", OnSelect: bump(reload)},
		}}})
		cx.OnKeyWith("mod+r", core.KeyOptions{YieldsToMenu: true}, bump(bindingR))
		cx.OnKeyWith("f5", core.KeyOptions{YieldsToMenu: true}, bump(bindingF5))
		cx.Effect(func() {
			cx.SetTitle(fmt.Sprintf("%s|%d%d%d|%d%d|", title, refresh.Get(), run.Get(), reload.Get(), bindingR.Get(), bindingF5.Get()))
		})
		return core.Input{Style: core.Style{Width: driver.Pt(240), Height: driver.Pt(30)}}
	}
	hwnd := startApp(t, title, app, 300, 120)
	waitForTitle(t, hwnd, title+"|000|00|", 10*time.Second)
	getState, setState := user32.NewProc("GetKeyboardState"), user32.NewProc("SetKeyboardState")
	const vkF5 = 0x74
	press := func(vk uintptr, control bool, want string) {
		t.Helper()
		onMain(t, hwnd, func(w *window) {
			var saved [256]byte
			getState.Call(uintptr(unsafe.Pointer(&saved)))
			held := saved
			if control {
				held[vkControl] |= 0x80
			} else {
				held[vkControl] &^= 0x80
			}
			setState.Call(uintptr(unsafe.Pointer(&held)))
			defer setState.Call(uintptr(unsafe.Pointer(&saved)))
			if !w.shortcut(vk, 1) {
				t.Errorf("nothing took key %#x", vk)
			}
		})
		waitForTitle(t, hwnd, title+"|"+want+"|", 5*time.Second)
	}
	// bound waits until the keymap holds the Control+R binding as want says.
	bound := func(want bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			ok := false
			onMain(t, hwnd, func(w *window) {
				ok = slices.ContainsFunc(w.keys, func(k driver.KeyBinding) bool { return k.Key == "mod+r" }) == want
			})
			if ok {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("the keymap does not hold the binding %v", want)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	onMain(t, hwnd, func(w *window) {
		for _, v := range w.views {
			if v != nil && v.field != nil {
				w.setFocus(v.field)
			}
		}
	})
	bound(true)
	press('R', true, "000|10")
	press(vkF5, false, "001|10")
	onMain(t, hwnd, func(w *window) { w.blurAll() })
	bound(false)
	press('R', true, "101|10")
	press(vkF5, false, "102|10")
}

// TestFocusedPageKeysReachShortcutsFirst types into a focused WebView2 page through the system's input queue, as the user's keyboard does. Control+J, which a KeyAlways binding takes, and Control+K, which a MenuAlways item takes, run the app's handlers and never reach the page; Control+A, whose KeyAuto binding yields to text, Control+L, whose MenuNotEditing item yields too, Control+B, which nothing binds, and a plain letter all reach the page.
func TestFocusedPageKeysReachShortcutsFirst(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if _, err := findWebView2(); err != nil {
		t.Skip(err)
	}
	const title = "nsgo-page-keys"
	app := func(cx *core.Scope) core.Node {
		bound, chosen, yielded, notEditing := core.NewSignal(cx, 0), core.NewSignal(cx, 0), core.NewSignal(cx, 0), core.NewSignal(cx, 0)
		page := core.NewSignal(cx, "")
		cx.SetApplicationMenu([]core.MenuItem{{Label: "Actions", Items: []core.MenuItem{
			{Label: "Palette", Shortcut: "mod+k", When: core.MenuAlways, OnSelect: func() { chosen.Update(func(n int) int { return n + 1 }) }},
			{Label: "Locate", Shortcut: "mod+l", When: core.MenuNotEditing, OnSelect: func() { notEditing.Update(func(n int) int { return n + 1 }) }},
		}}})
		cx.OnKeyWith("mod+j", core.KeyOptions{When: core.KeyAlways}, func() { bound.Update(func(n int) int { return n + 1 }) })
		cx.OnKey("mod+a", func() { yielded.Update(func(n int) int { return n + 1 }) })
		cx.Effect(func() {
			cx.SetTitle(fmt.Sprintf("%s|%d|%d|%d|%d|%s", title, bound.Get(), chosen.Get(), yielded.Get(), notEditing.Get(), page.Get()))
		})
		// The page reports each key that comes down with Control held, the field's value as it changes, and "ready" once it has the focus and its field has it within.
		const html = `<!doctype html><input id=f><script>
const f = document.getElementById('f');
document.addEventListener('keydown', e => { if (e.ctrlKey && e.key !== 'Control') nsgo.postMessage('^' + e.key.toLowerCase()); }, true);
f.addEventListener('input', () => nsgo.postMessage('=' + f.value));
nsgo.postMessage('loaded');
const ready = setInterval(() => {
  f.focus();
  if (document.hasFocus() && document.activeElement === f) { clearInterval(ready); nsgo.postMessage('ready'); }
}, 50);
</script>`
		return core.View{Style: core.Style{Padding: driver.All(10), Background: driver.RGB(0xffffff)}, Children: []core.Node{
			core.WebView{Source: core.WebHTML(html, ""), OnMessage: func(data string) { page.Update(func(s string) string { return s + data + " " }) },
				Style: core.Style{Width: driver.Pt(260), Height: driver.Pt(120)}},
		}}
	}
	hwnd := startApp(t, title, app, 300, 200)
	// The first start of the browser process can take a while.
	waitForTitle(t, hwnd, title+"|0|0|0|0|", 30*time.Second)
	if !takeForeground(hwnd) {
		t.Skip("cannot bring the window to the front for the system's input")
	}
	got := waitForTitle(t, hwnd, title+"|0|0|0|0|", 10*time.Second)
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(got, "ready") {
		if time.Now().After(deadline) {
			fg, _, _ := getForegroundWindow.Call()
			t.Fatalf("the page's field did not take the focus (in front: %v): %q", fg == hwnd, got)
		}
		onMain(t, hwnd, func(w *window) {
			for wv := range w.webs {
				if !wv.focused {
					wv.takeFocus()
				}
			}
		})
		time.Sleep(100 * time.Millisecond)
		got = windowText(hwnd)
	}
	onMain(t, hwnd, func(w *window) {
		focused := false
		for wv := range w.webs {
			focused = focused || wv.focused
		}
		if !focused {
			t.Fatal("the window does not know the page has the focus")
		}
	})
	sendKeys(t, hwnd, vkControl, 'J')
	waitForTitle(t, hwnd, title+"|1|0|0|0|", 5*time.Second)
	sendKeys(t, hwnd, vkControl, 'K')
	waitForTitle(t, hwnd, title+"|1|1|0|0|", 5*time.Second)
	sendKeys(t, hwnd, vkControl, 'A')
	sendKeys(t, hwnd, vkControl, 'L')
	sendKeys(t, hwnd, vkControl, 'B')
	sendKeys(t, hwnd, 'X')
	// The page posts in the order the keys came, so its field's value comes after every key before it.
	got = waitForTitle(t, hwnd, title+"|1|1|0|0|", 5*time.Second)
	deadline = time.Now().Add(5 * time.Second)
	for !strings.Contains(got, "=x") {
		if time.Now().After(deadline) {
			t.Fatalf("the page did not type x: %q", got)
		}
		time.Sleep(20 * time.Millisecond)
		got = windowText(hwnd)
	}
	if !strings.HasPrefix(got, title+"|1|1|0|0|") {
		t.Errorf("after the keys the counts are %q, want the binding and the Palette item once and the yielding binding and the Locate item never", got)
	}
	log := got[strings.LastIndex(got, "|")+1:]
	for _, key := range []string{"^j", "^k"} {
		if strings.Contains(log, key) {
			t.Errorf("the page also got %s: %q", key, log)
		}
	}
	for _, key := range []string{"^a", "^l", "^b"} {
		if !strings.Contains(log, key) {
			t.Errorf("the page did not get %s: %q", key, log)
		}
	}
}

// TestEditMenuEditsTheFocusedPage chooses the Edit menu's roles, as the menu bar sends them, while a WebView2 page's text area has the focus: each runs in the page what its shortcut runs, Copy and Cut writing the clipboard and Paste reading it, and Delete removes the selection alone, nothing with a caret.
func TestEditMenuEditsTheFocusedPage(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if _, err := findWebView2(); err != nil {
		t.Skip(err)
	}
	const title = "nsgo-page-edit"
	roles := []core.MenuRole{core.MenuUndo, core.MenuRedo, core.MenuCut, core.MenuCopy, core.MenuPaste, core.MenuDelete, core.MenuSelectAll}
	app := func(cx *core.Scope) core.Node {
		page := core.NewSignal(cx, "")
		var items []core.MenuItem
		for _, r := range roles {
			items = append(items, core.MenuItem{Role: r})
		}
		cx.SetApplicationMenu([]core.MenuItem{{Label: "Edit", Items: items}})
		cx.Effect(func() { cx.SetTitle(title + "|" + page.Get()) })
		// The page reports whether its text area has the focus, the area's value and its selection, as they change.
		const html = `<!doctype html><textarea id=f>hello page</textarea><script>
const f = document.getElementById('f');
f.focus();
f.setSelectionRange(10, 10);
let last = '';
setInterval(() => {
  if (document.activeElement !== f) f.focus();
  const s = (document.hasFocus() && document.activeElement === f ? 'ready' : 'wait') + ':' + f.value + ':' + f.selectionStart + '-' + f.selectionEnd;
  if (s !== last) { last = s; nsgo.postMessage(s); }
}, 30);
</script>`
		return core.View{Style: core.Style{Padding: driver.All(10), Background: driver.RGB(0xffffff)}, Children: []core.Node{
			core.WebView{Source: core.WebHTML(html, ""), OnMessage: func(data string) { page.Set(data) },
				Style: core.Style{Width: driver.Pt(260), Height: driver.Pt(120)}},
		}}
	}
	hwnd := startApp(t, title, app, 300, 200)
	// The first start of the browser process can take a while.
	waitForTitle(t, hwnd, title+"|", 30*time.Second)
	if !takeForeground(hwnd) {
		t.Skip("cannot bring the window to the front")
	}
	deadline := time.Now().Add(30 * time.Second)
	for !strings.HasPrefix(windowText(hwnd), title+"|ready:hello page:10-10") {
		if time.Now().After(deadline) {
			t.Fatalf("the page's text area did not take the focus: %q", windowText(hwnd))
		}
		onMain(t, hwnd, func(w *window) {
			for wv := range w.webs {
				if !wv.focused {
					wv.takeFocus()
				}
			}
		})
		time.Sleep(100 * time.Millisecond)
	}
	onMain(t, hwnd, func(w *window) {
		if w.focusedWeb() == nil {
			t.Fatal("the window does not know the page has the focus")
		}
	})
	choose := func(r core.MenuRole) {
		t.Helper()
		onMain(t, hwnd, func(w *window) {
			i := slices.IndexFunc(w.chrome.entries, func(ref menuRef) bool { return ref.role == r })
			if i < 0 {
				t.Fatalf("the menu has no item of role %d", r)
			}
			postMessageW.Call(hwnd, wmCommand, uintptr(i+1), 0)
		})
	}
	clipboard := func(want string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			got := ""
			onMain(t, hwnd, func(w *window) { got = w.clipboard() })
			if got == want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("the clipboard holds %q, want %q", got, want)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	setClipboard := func(s string) { onMain(t, hwnd, func(w *window) { w.setClipboard(s) }) }
	page := func(want string) {
		t.Helper()
		waitForTitle(t, hwnd, title+"|ready:"+want, 5*time.Second)
	}
	// With the caret at the end Delete removes nothing, which Select All then shows.
	choose(core.MenuDelete)
	choose(core.MenuSelectAll)
	page("hello page:0-10")
	setClipboard("before")
	choose(core.MenuCopy)
	clipboard("hello page")
	choose(core.MenuDelete)
	page(":0-0")
	choose(core.MenuPaste)
	page("hello page:10-10")
	choose(core.MenuUndo)
	page(":")
	choose(core.MenuRedo)
	page("hello page:")
	choose(core.MenuSelectAll)
	page("hello page:0-10")
	setClipboard("before")
	choose(core.MenuCut)
	page(":0-0")
	clipboard("hello page")
}

// TestStartSpeakingReadsTheFocusedPage runs Start Speaking while a WebView2 page's text area has the focus: it reads the area's selection from ExecuteScript's answer, a second Start or a Stop drops a read still pending, and the page's own messages keep reaching the app throughout. Where the window cannot come to the front, as in a disconnected session, the page cannot take the keyboard focus, and the test reads it as speakMenu reads a focused page.
func TestStartSpeakingReadsTheFocusedPage(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if _, err := findWebView2(); err != nil {
		t.Skip(err)
	}
	heard := make(chan string, 8)
	was := menuSay
	menuSay = func(_ *window, text string) { heard <- text }
	t.Cleanup(func() { menuSay = was })
	const title = "nsgo-page-speech"
	app := func(cx *core.Scope) core.Node {
		page := core.NewSignal(cx, "")
		cx.Effect(func() { cx.SetTitle(title + "|" + page.Get()) })
		// The page selects "page" in its text area and posts whether the area is the active element, with a count, every 50ms.
		const html = `<!doctype html><textarea id=f>hello page</textarea><script>
const f = document.getElementById('f');
f.focus();
f.setSelectionRange(6, 10);
let n = 0;
setInterval(() => {
  if (document.activeElement !== f) { f.focus(); f.setSelectionRange(6, 10); }
  nsgo.postMessage((document.activeElement === f ? 'ready' : 'wait') + ':' + (++n));
}, 50);
</script>`
		return core.View{Style: core.Style{Padding: driver.All(10), Background: driver.RGB(0xffffff)}, Children: []core.Node{
			core.WebView{Source: core.WebHTML(html, ""), OnMessage: func(data string) { page.Set(data) },
				Style: core.Style{Width: driver.Pt(260), Height: driver.Pt(120)}},
		}}
	}
	hwnd := startApp(t, title, app, 300, 200)
	// The first start of the browser process can take a while.
	waitForTitle(t, hwnd, title+"|ready:", 30*time.Second)
	focused := false
	if takeForeground(hwnd) {
		deadline := time.Now().Add(30 * time.Second)
		for !focused {
			onMain(t, hwnd, func(w *window) {
				focused = w.focusedWeb() != nil
				for wv := range w.webs {
					if !wv.focused {
						wv.takeFocus()
					}
				}
			})
			if time.Now().After(deadline) {
				t.Fatal("the page did not take the focus")
			}
			time.Sleep(100 * time.Millisecond)
		}
	} else {
		t.Log("the window cannot come to the front: the page is read as speakMenu reads a focused one")
	}
	speak := func(w *window, start bool) {
		if focused {
			w.speakMenu(start)
			return
		}
		n := menuSpeech.Add(1)
		for wv := range w.webs {
			if start {
				wv.speakSelection(n)
			}
		}
	}
	ticking := func() {
		t.Helper()
		before := windowText(hwnd)
		deadline := time.Now().Add(5 * time.Second)
		for {
			got := windowText(hwnd)
			if strings.HasPrefix(got, title+"|ready:") && got != before {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("the page's messages stopped reaching the app: title %q", got)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	next := func() (string, bool) {
		select {
		case text := <-heard:
			return text, true
		case <-time.After(2 * time.Second):
			return "", false
		}
	}
	onMain(t, hwnd, func(w *window) { speak(w, true) })
	if text, ok := next(); !ok || text != "page" {
		t.Fatalf("the page's selection was said as %q (%v)", text, ok)
	}
	ticking()
	// A second Start Speaking supersedes the first: the page is read and said once.
	onMain(t, hwnd, func(w *window) { speak(w, true); speak(w, true) })
	if text, ok := next(); !ok || text != "page" {
		t.Fatalf("the second Start Speaking said %q (%v)", text, ok)
	}
	if text, ok := next(); ok {
		t.Errorf("the superseded read was said too: %q", text)
	}
	// Stop Speaking drops the read still pending.
	onMain(t, hwnd, func(w *window) { speak(w, true); speak(w, false) })
	if text, ok := next(); ok {
		t.Errorf("a read Stop Speaking dropped was said: %q", text)
	}
	ticking()
}

var (
	keybdEvent        = user32.NewProc("keybd_event")
	attachThreadInput = user32.NewProc("AttachThreadInput")
	windowThread      = user32.NewProc("GetWindowThreadProcessId")
	currentThread     = kernel32.NewProc("GetCurrentThreadId")
)

// takeForeground makes hwnd the foreground window, joining the input of the window in front for the moment Windows' foreground lock asks it to, and reports whether it is.
func takeForeground(hwnd uintptr) bool {
	for range 20 {
		if fg, _, _ := getForegroundWindow.Call(); fg == hwnd {
			return true
		}
		fg, _, _ := getForegroundWindow.Call()
		own, _, _ := currentThread.Call()
		other, _, _ := windowThread.Call(fg, 0)
		if other != 0 && other != own {
			attachThreadInput.Call(own, other, 1)
		}
		setForegroundWindow.Call(hwnd)
		if other != 0 && other != own {
			attachThreadInput.Call(own, other, 0)
		}
		time.Sleep(100 * time.Millisecond)
	}
	fg, _, _ := getForegroundWindow.Call()
	return fg == hwnd
}

// sendKeys presses keys in order and lets them go in reverse through the system's input queue, after checking hwnd is still in front so the keys go nowhere else.
func sendKeys(t *testing.T, hwnd uintptr, keys ...byte) {
	t.Helper()
	if fg, _, _ := getForegroundWindow.Call(); fg != hwnd {
		t.Fatal("the window left the front before the keys were sent")
	}
	const keyUp = 0x2
	for _, k := range keys {
		scan, _, _ := mapVirtualKeyW.Call(uintptr(k), 0)
		keybdEvent.Call(uintptr(k), scan, 0, 0)
	}
	for i := len(keys) - 1; i >= 0; i-- {
		scan, _, _ := mapVirtualKeyW.Call(uintptr(keys[i]), 0)
		keybdEvent.Call(uintptr(keys[i]), scan, keyUp, 0)
	}
	time.Sleep(150 * time.Millisecond)
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
		return core.View{Style: core.Style{Width: driver.Pt(240), Height: driver.Pt(120), Background: driver.RGB(0xffffff)}, Children: []core.Node{
			core.Input{TestID: "field", DefaultValue: "hello", Style: core.Style{Width: driver.Pt(180), Height: driver.Pt(30)}},
			core.View{Selectable: true, Children: []core.Node{core.Text{Content: "Hello world", Style: core.Style{FontSize: 16}}}},
		}}
	}
	hwnd := startApp(t, title, app, 240, 120)
	waitForTitle(t, hwnd, title+"|0|0|0", 5*time.Second)
	choose := func(r core.MenuRole) {
		t.Helper()
		onMain(t, hwnd, func(w *window) {
			i := slices.IndexFunc(w.chrome.entries, func(ref menuRef) bool { return ref.role == r })
			if i < 0 {
				t.Fatalf("the menu has no item of role %d", r)
			}
			w.chooseMenu(w.chrome.entries[i])
		})
	}
	choose(core.MenuSelectAll)
	choose(core.MenuCopy)
	waitForTitle(t, hwnd, title+"|1|1|0", 5*time.Second)

	onMain(t, hwnd, func(w *window) {
		for _, v := range w.views {
			if v != nil && v.field != nil {
				w.setFocus(v.field)
			}
		}
	})
	choose(core.MenuSelectAll)
	onMain(t, hwnd, func(w *window) {
		in := w.focusedField()
		if in == nil {
			t.Fatal("the field did not take the focus")
		}
		if got := in.model.selected(); got != "hello" {
			t.Errorf("Select All selected %q of the field, want all of it", got)
		}
		w.blurAll()
	})
	// Undo falls back with the field blurred, after the menu's reports above, so the counts are final once it shows.
	choose(core.MenuUndo)
	waitForTitle(t, hwnd, title+"|1|1|1", 5*time.Second)

	onMain(t, hwnd, func(w *window) {
		for _, v := range w.views {
			if v != nil && v.kind == driver.KindText && v.plain == "Hello world" {
				b, _ := w.surfaceBounds(v)
				at := driver.Point{X: b.Origin.X + 10, Y: b.Origin.Y + b.Size.Height/2}
				w.selectPress(at, 2, false)
				w.selectUp()
			}
		}
		if w.textSelection == nil || w.textSelection.Model.Text(w.textSelection) != "Hello" {
			t.Error("double click selected no word")
		}
	})
	choose(core.MenuSelectAll)
	choose(core.MenuCopy)
	choose(core.MenuUndo)
	waitForTitle(t, hwnd, title+"|1|1|2", 5*time.Second)
	onMain(t, hwnd, func(w *window) {
		if got := w.textSelection.Model.Text(w.textSelection); got != "Hello world" {
			t.Errorf("Select All selected %q of the text, want all of it", got)
		}
	})
}

// TestMenuBarSyncsInPlace checks that setting the application menu again keeps the existing HMENU bar and submenu handles while updating item state, labels and commands in place.
func TestMenuBarSyncsInPlace(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	getItem := user32.NewProc("GetMenuItemInfoW")
	getCount := user32.NewProc("GetMenuItemCount")
	getString := user32.NewProc("GetMenuStringW")
	const title = "nsgo-menu-sync-in-place"
	var toggle func()
	chosen := make(chan string, 4)
	app := func(cx *core.Scope) core.Node {
		checked := core.NewSignal(cx, true)
		toggle = func() { cx.Post(func() { checked.Set(!checked.Get()) }) }
		cx.Effect(func() {
			on := checked.Get()
			items := []core.MenuItem{
				{Label: "Sidebar", Checkable: true, Checked: on, OnSelect: func() { chosen <- "Sidebar" }},
				{Separator: true},
				{Label: "Button", Disabled: !on, OnSelect: func() { chosen <- "Button" }},
			}
			if !on {
				items = append(items, core.MenuItem{Label: "Link", OnSelect: func() { chosen <- "Link" }})
			}
			menus := []core.MenuItem{{Label: "View", Items: items}}
			if !on {
				menus = append(menus, core.MenuItem{Label: "Help", Items: []core.MenuItem{{Label: "Docs", OnSelect: func() { chosen <- "Docs" }}}})
			}
			cx.SetApplicationMenu(menus)
			cx.SetTitle(fmt.Sprintf("%s|%v", title, on))
		})
		return core.View{Style: core.Style{Background: driver.RGB(0xffffff)}}
	}
	hwnd := startApp(t, title, app, 320, 200)
	waitForTitle(t, hwnd, title+"|true", 5*time.Second)

	readItem := func(menu uintptr, pos uint32) (menuItemInfo, string) {
		mi := menuItemInfo{fMask: miimFType | miimState | miimID | miimSubMenu}
		mi.cbSize = uint32(unsafe.Sizeof(mi))
		getItem.Call(menu, uintptr(pos), 1, uintptr(unsafe.Pointer(&mi)))
		buf := make([]uint16, 128)
		getString.Call(menu, uintptr(pos), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), mfByPosition)
		return mi, syscall.UTF16ToString(buf)
	}

	var bar1, viewSub1 uintptr
	onMain(t, hwnd, func(w *window) {
		bar1, _, _ = getMenu.Call(hwnd)
		if bar1 == 0 || bar1 != w.chrome.bar {
			t.Fatalf("bar1 = %#x, w.chrome.bar = %#x", bar1, w.chrome.bar)
		}
		mi, _ := readItem(bar1, 0)
		viewSub1 = mi.hSubMenu
		if viewSub1 == 0 {
			t.Fatal("View submenu handle is 0")
		}
		sidebar, _ := readItem(viewSub1, 0)
		if sidebar.fState&mfChecked == 0 {
			t.Error("Sidebar should start checked")
		}
	})

	toggle()
	waitForTitle(t, hwnd, title+"|false", 5*time.Second)

	var docsCmd uint16
	onMain(t, hwnd, func(w *window) {
		bar2, _, _ := getMenu.Call(hwnd)
		if bar2 != bar1 || w.chrome.bar != bar1 {
			t.Fatalf("menu bar HMENU changed from %#x to %#x", bar1, bar2)
		}
		if n, _, _ := getCount.Call(bar2); n != 2 {
			t.Fatalf("top-level menu count = %d, want 2", n)
		}
		viewMI, _ := readItem(bar2, 0)
		if viewMI.hSubMenu != viewSub1 {
			t.Fatalf("View submenu HMENU changed from %#x to %#x", viewSub1, viewMI.hSubMenu)
		}
		sidebar, _ := readItem(viewSub1, 0)
		if sidebar.fState&mfChecked != 0 {
			t.Error("Sidebar stayed checked after in-place update")
		}
		button, _ := readItem(viewSub1, 2)
		if button.fState&mfGrayed == 0 {
			t.Error("Button was not disabled in place")
		}
		if _, label := readItem(viewSub1, 3); label != "Link" {
			t.Errorf("added View[3] = %q, want Link", label)
		}
		helpMI, helpLabel := readItem(bar2, 1)
		if helpLabel != "Help" || helpMI.hSubMenu == 0 {
			t.Fatalf("added Help menu = %q (%#x)", helpLabel, helpMI.hSubMenu)
		}
		docsMI, docsLabel := readItem(helpMI.hSubMenu, 0)
		if docsLabel != "Docs" {
			t.Errorf("Help[0] = %q, want Docs", docsLabel)
		}
		docsCmd = uint16(docsMI.wID)
		w.menuCommand(docsCmd)
	})
	select {
	case got := <-chosen:
		if got != "Docs" {
			t.Errorf("menuCommand(%d) chose %q, want Docs", docsCmd, got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Docs command did not run")
	}
}

// TestWindowShowsWithFirstBatch checks that a visible window stays hidden until its first mount batch applies and that a Hidden window stays hidden.
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
		hwnd := startAppWith(t, func(cx *core.Scope) core.Node {
			select {
			case entered <- struct{}{}:
			default:
			}
			<-gate
			cx.SetTitle(title + "|mounted")
			return core.View{Style: core.Style{Background: driver.RGB(0xffffff)}}
		}, Options{Title: title, Width: 240, Height: 160})

		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("app function did not start")
		}
		onMain(t, hwnd, func(w *window) {
			if visible, _, _ := isWindowVisible.Call(hwnd); visible != 0 {
				t.Error("window showed before its first batch was applied")
			}
			if !w.showFirst.pending {
				t.Error("showFirst was not pending before the first batch")
			}
		})
		close(gate)
		waitForTitle(t, hwnd, title+"|mounted", 5*time.Second)
		// The title can change before the batch that mounts the view is applied.
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
			var shown, pending bool
			onMain(t, hwnd, func(w *window) {
				visible, _, _ := isWindowVisible.Call(hwnd)
				shown, pending = visible != 0, w.showFirst.pending
			})
			if shown && !pending {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("window did not show after its first batch was applied (visible %v, pending %v)", shown, pending)
			}
		}
	})

	t.Run("hidden", func(t *testing.T) {
		const hiddenTitle = "nsgo-first-batch-hidden"
		hiddenHWND := startAppWith(t, func(cx *core.Scope) core.Node {
			cx.SetTitle(hiddenTitle + "|mounted")
			return core.View{Style: core.Style{Background: driver.RGB(0xffffff)}}
		}, Options{Title: hiddenTitle, Width: 240, Height: 160, Hidden: true})
		waitForTitle(t, hiddenHWND, hiddenTitle+"|mounted", 5*time.Second)
		onMain(t, hiddenHWND, func(w *window) {
			if visible, _, _ := isWindowVisible.Call(hiddenHWND); visible != 0 {
				t.Error("hidden window showed after its first batch")
			}
		})
	})
}

//go:build windows

package windows

import (
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/scrollbar"
)

// TestDirectionMirrorsNativeControls runs a live Win32 window with an
// application menu bar (and nested submenu), single-line and multiline inputs,
// a vertical scroll view and an alert, toggles between LTR ("en"), RTL
// ("ar-XB") and back to LTR, and checks Win32 menu item flags, popup flags,
// DirectWrite input/alert layouts, alert button order and arrow navigation, and
// scroll bar placement (#1615).
func TestDirectionMirrorsNativeControls(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	scopes := make(chan *core.Scope, 1)
	app := func(cx *core.Scope) core.Node {
		cx.SetApplicationMenu([]core.MenuItem{
			{Label: "File", Items: []core.MenuItem{
				{Label: "Open", OnSelect: func() {}},
				{Label: "Recent", Items: []core.MenuItem{{Label: "Doc", OnSelect: func() {}}}},
			}},
			{Label: "Edit", Items: []core.MenuItem{
				{Label: "Copy", OnSelect: func() {}},
			}},
		})
		scopes <- cx
		return core.View{
			Style: core.Style{Width: core.Pt(360), Height: core.Pt(260), Direction: core.Column, Gap: 8, Background: driver.RGB(0xffffff)},
			Children: []core.Node{
				core.Input{
					TestID:       "single",
					DefaultValue: "abc",
					Placeholder:  "Name",
					Style:        core.Style{Width: core.Pt(240), Height: core.Pt(32), Color: driver.RGB(0x111111)},
				},
				core.Input{
					TestID:       "multi",
					Multiline:    true,
					DefaultValue: "line1\nline2\nline3\nline4\nline5",
					Placeholder:  "Notes",
					Style:        core.Style{Width: core.Pt(240), Height: core.Pt(48), Color: driver.RGB(0x111111)},
				},
				core.Scroll{
					TestID: "scroller",
					Style:  core.Style{Width: core.Pt(200), Height: core.Pt(60)},
					Children: []core.Node{
						core.View{Style: core.Style{Width: core.Pt(200), Height: core.Pt(240)}},
					},
				},
			},
		}
	}

	hwnd := startApp(t, "nsgo-direction-windows", app, 360, 260)
	var cx *core.Scope
	select {
	case cx = <-scopes:
	case <-time.After(5 * time.Second):
		t.Fatal("the app did not start")
	}

	getItem := user32.NewProc("GetMenuItemInfoW")
	itemCount := user32.NewProc("GetMenuItemCount")

	waitFor := func(what string, cond func(w *window) bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
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

	findView := func(w *window, testID string) *view {
		for _, v := range w.views {
			if v != nil && !v.deleted && v.props.TestID == testID {
				return v
			}
		}
		return nil
	}

	checkMenuTree := func(menu uintptr, wantRTL bool) {
		t.Helper()
		var walk func(uintptr)
		walk = func(m uintptr) {
			n, _, _ := itemCount.Call(m)
			if n == 0 {
				t.Errorf("empty menu %#x", m)
				return
			}
			for i := range n {
				mi := winMenuItemInfo{
					cbSize: uint32(unsafe.Sizeof(winMenuItemInfo{})),
					fMask:  miimFType | miimSubMenu,
				}
				getItem.Call(m, i, 1, uintptr(unsafe.Pointer(&mi)))
				if mi.fType&mfSeparator != 0 {
					continue
				}
				gotRTL := mi.fType&mfRightOrder != 0
				if gotRTL != wantRTL {
					t.Errorf("menu %#x item %d MFT_RIGHTORDER=%v (fType=%#x), want %v", m, i, gotRTL, mi.fType, wantRTL)
				}
				if mi.hSubMenu != 0 {
					walk(mi.hSubMenu)
				}
			}
		}
		walk(menu)
	}

	verifyPhase := func(phase string, rtl bool) {
		t.Helper()
		waitFor(phase+": direction and inputs", func(w *window) bool {
			if w.rtl != rtl || w.chrome.bar == 0 {
				return false
			}
			s := findView(w, "single")
			m := findView(w, "multi")
			sc := findView(w, "scroller")
			return s != nil && s.field != nil && s.field.config.Style.RTL == rtl &&
				m != nil && m.field != nil && m.field.config.Style.RTL == rtl &&
				sc != nil && sc.frame.Size.Width > 0
		})
		onMain(t, hwnd, func(w *window) {
			// 1. Win32 menu bar and nested submenus carry MFT_RIGHTORDER in RTL only.
			bar, _, _ := getMenu.Call(hwnd)
			if bar == 0 {
				t.Fatalf("%s: GetMenu returned 0", phase)
			}
			checkMenuTree(bar, rtl)

			// 2. Context menu popup flags carry TPM_LAYOUTRTL | TPM_RIGHTALIGN in RTL only.
			pf := w.popupFlags()
			if rtl && pf&(tpmLayoutRTL|tpmRightAlign) != tpmLayoutRTL|tpmRightAlign {
				t.Errorf("%s: RTL popupFlags=%#x missing TPM_LAYOUTRTL|TPM_RIGHTALIGN", phase, pf)
			}
			if !rtl && pf&(tpmLayoutRTL|tpmRightAlign) != 0 {
				t.Errorf("%s: LTR popupFlags=%#x unexpectedly has RTL flags", phase, pf)
			}

			// 3. Single-line input aligns right in RTL and left in LTR.
			sv := findView(w, "single")
			sv.field.setSize(float32(sv.field.config.Content.Size.Width), float32(sv.field.config.Content.Size.Height))
			sl := sv.field.textLayout()
			if sl == nil {
				t.Fatalf("%s: single-line textLayout is nil", phase)
			}
			ox, _ := sv.field.origin(sl)
			if rtl && ox <= sv.field.width/2 {
				t.Errorf("%s: RTL single-line origin x=%v, want > %v", phase, ox, sv.field.width/2)
			}
			if !rtl && ox != 0 {
				t.Errorf("%s: LTR single-line origin x=%v, want 0", phase, ox)
			}

			// 4. Multiline input places caret on right edge in RTL and left edge in LTR, and mirrors its vertical scroll bar.
			mv := findView(w, "multi")
			mv.field.setSize(float32(mv.field.config.Content.Size.Width), float32(mv.field.config.Content.Size.Height))
			ml := mv.field.textLayout()
			if ml == nil {
				t.Fatalf("%s: multiline textLayout is nil", phase)
			}
			mx, _, _ := caretAt(ml, 0)
			if rtl && mx <= mv.field.width/2 {
				t.Errorf("%s: RTL multiline caret(0) x=%v, want > %v", phase, mx, mv.field.width/2)
			}
			if !rtl && mx > 1 {
				t.Errorf("%s: LTR multiline caret(0) x=%v, want ~0", phase, mx)
			}
			mv.field.origin(ml)
			if ind, _, ok := mv.field.bar().Indicator(); ok {
				if rtl && ind.Origin.X != scrollbar.Inset {
					t.Errorf("%s: RTL multiline scrollbar X=%v, want %v", phase, ind.Origin.X, scrollbar.Inset)
				}
				if !rtl && float32(ind.Origin.X) <= mv.field.width/2 {
					t.Errorf("%s: LTR multiline scrollbar X=%v, want on right", phase, ind.Origin.X)
				}
			}

			// 5. Scroll view vertical bar mirrors to left edge in RTL and right edge in LTR.
			sc := findView(w, "scroller")
			thumb, _, ok := sc.bar().Indicator()
			if !ok {
				t.Fatalf("%s: scroller has no indicator", phase)
			}
			if rtl && thumb.Origin.X != scrollbar.Inset {
				t.Errorf("%s: RTL scroll indicator X=%v, want %v", phase, thumb.Origin.X, scrollbar.Inset)
			}
			if !rtl {
				wantX := sc.frame.Size.Width - scrollbar.Inset - scrollbar.Thickness
				if thumb.Origin.X != wantX {
					t.Errorf("%s: LTR scroll indicator X=%v, want %v", phase, thumb.Origin.X, wantX)
				}
			}

			// 6. Alert mirrors horizontal button layout and arrow key order in RTL.
			al := &shownAlert{
				config: driver.AlertConfig{
					Title:   "Delete item?",
					Message: "This cannot be undone.",
					Actions: []driver.AlertAction{
						{Label: "Delete", Role: driver.AlertDefault},
						{Label: "Cancel", Role: driver.AlertCancel},
					},
				},
				labels: []string{"Delete", "Cancel"},
				focus:  0,
				cancel: 1,
			}
			defer al.resetLayouts()
			al.layout(driver.Size{Width: 480, Height: 320}, w.rtl)
			if len(al.buttons) != 2 {
				t.Fatalf("%s: alert buttons = %d, want 2", phase, len(al.buttons))
			}
			if rtl && !(al.buttons[0].left > al.buttons[1].left) {
				t.Errorf("%s: RTL alert button[0]=%v not right of button[1]=%v", phase, al.buttons[0], al.buttons[1])
			}
			if !rtl && !(al.buttons[0].left < al.buttons[1].left) {
				t.Errorf("%s: LTR alert button[0]=%v not left of button[1]=%v", phase, al.buttons[0], al.buttons[1])
			}
			prevAlert := w.alert
			w.alert = al
			w.alertKey(vkLeft)
			gotFocus := al.focus
			w.alert = prevAlert
			if rtl && gotFocus != 1 {
				t.Errorf("%s: RTL vkLeft focus = %d, want 1", phase, gotFocus)
			}
			if !rtl && gotFocus != 0 {
				t.Errorf("%s: LTR vkLeft focus = %d, want 0", phase, gotFocus)
			}
		})
	}

	cx.Post(func() { cx.SetLocale("en") })
	verifyPhase("LTR initial", false)

	cx.Post(func() { cx.SetLocale("ar-XB") })
	verifyPhase("RTL", true)

	cx.Post(func() { cx.SetLocale("en") })
	verifyPhase("LTR restored", false)
}

//go:build darwin && !ios

package macos

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/scrollbar"
)

// TestDirectionMirrorsNativeControlsInLiveWindow runs a live macOS window with
// AppKit controls (single-line and multiline inputs, a masked input, NSSwitch,
// a vertical scroll view, application/dock/popup menus and an alert), switches
// the app between RTL ("ar-XB") and LTR ("en"), and verifies every native view,
// menu, alert, scroll bar and newly swapped editor mirrors (#1615).
func TestDirectionMirrorsNativeControlsInLiveWindow(t *testing.T) {
	if os.Getenv("NSGO_TEST_DIRECTION_PROCESS") != t.Name() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_DIRECTION_PROCESS="+t.Name(), "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	const title = "nsgo-direction-live"
	scopes := make(chan *core.Scope, 1)
	var setShowLate func(bool)
	var step *core.Signal[int]

	app := func(cx *core.Scope) core.Node {
		scopes <- cx
		showLate := core.NewSignal(cx, false)
		checked := core.NewSignal(cx, true)
		step = core.NewSignal(cx, 0)
		setShowLate = showLate.Set
		cx.SetApplicationMenu([]core.MenuItem{
			{Label: "File", Items: []core.MenuItem{
				{Label: "Open", OnSelect: func() {}},
				{Label: "Recent", Items: []core.MenuItem{{Label: "Doc", OnSelect: func() {}}}},
			}},
		})
		cx.SetDockMenu([]core.MenuItem{{Label: "New Window", OnSelect: func() {}}})
		cx.Effect(func() {
			cx.SetTitle(fmt.Sprintf("%s|%v|%d", title, cx.Env().RTL(), step.Get()))
		})
		return core.View{
			Style: core.Style{Width: core.Pt(360), Height: core.Pt(280), Direction: core.Column, Gap: 8},
			Children: []core.Node{
				core.Input{
					TestID:      "swapping",
					Style:       core.Style{Width: core.Pt(240), Height: core.Pt(36)},
					Placeholder: "Type here",
				},
				core.Input{
					TestID:      "area",
					Multiline:   true,
					Placeholder: "Notes",
					Style:       core.Style{Width: core.Pt(240), Height: core.Pt(60)},
				},
				core.SystemSwitch{
					TestID:  "toggle",
					Checked: checked.Get,
					OnChange: func(v bool) {
						checked.Set(v)
					},
				},
				core.Scroll{
					TestID: "scroller",
					Style:  core.Style{Width: core.Pt(200), Height: core.Pt(60)},
					Children: []core.Node{
						core.View{Style: core.Style{Width: core.Pt(200), Height: core.Pt(240)}},
					},
				},
				core.Show{
					When: showLate.Get,
					Then: func(*core.Scope) core.Node {
						return core.SystemSwitch{TestID: "late", Checked: func() bool { return false }}
					},
				},
			},
		}
	}

	done := make(chan error, 1)
	go func() {
		cx := <-scopes
		var live *surface
		runOnMain := func(fn func()) {
			ch := make(chan struct{})
			apple.OnMain(func() {
				fn()
				close(ch)
			})
			<-ch
		}
		waitTitle := func(want string) error {
			deadline := time.Now().Add(5 * time.Second)
			var last string
			for time.Now().Before(deadline) {
				var got string
				runOnMain(func() {
					for s := range surfaces {
						if s.window != 0 && !s.closed {
							live = s
							got = apple.GoString(s.window.Send(sel("title")))
							break
						}
					}
				})
				last = got
				if got == want {
					return nil
				}
				time.Sleep(15 * time.Millisecond)
			}
			return fmt.Errorf("window title = %q, want %q", last, want)
		}
		checkTree := func(v objc.ID, want int) error {
			var walk func(objc.ID) error
			walk = func(node objc.ID) error {
				if got := objc.Send[int](node, sel("userInterfaceLayoutDirection")); got != want {
					return fmt.Errorf("view %s userInterfaceLayoutDirection = %d, want %d",
						apple.GoString(node.Send(sel("className"))), got, want)
				}
				subs := node.Send(sel("subviews"))
				n := objc.Send[uint](subs, sel("count"))
				for i := uint(0); i < n; i++ {
					if err := walk(subs.Send(sel("objectAtIndex:"), i)); err != nil {
						return err
					}
				}
				return nil
			}
			return walk(v)
		}
		checkMenu := func(m objc.ID, want int) error {
			var walk func(objc.ID) error
			walk = func(menu objc.ID) error {
				if menu == 0 {
					return nil
				}
				if got := objc.Send[int](menu, sel("userInterfaceLayoutDirection")); got != want {
					return fmt.Errorf("menu %q userInterfaceLayoutDirection = %d, want %d",
						apple.GoString(menu.Send(sel("title"))), got, want)
				}
				items := menu.Send(sel("itemArray"))
				n := objc.Send[uint](items, sel("count"))
				for i := uint(0); i < n; i++ {
					if err := walk(items.Send(sel("objectAtIndex:"), i).Send(sel("submenu"))); err != nil {
						return err
					}
				}
				return nil
			}
			return walk(m)
		}
		findView := func(testID string) *view {
			for _, v := range live.views {
				if v != nil && v.props.TestID == testID {
					return v
				}
			}
			return nil
		}
		verifyPhase := func(rtl bool) error {
			wantDir := layoutLTR
			wantAlign := nsAlignLeft
			wantWriting := writingLTR
			if rtl {
				wantDir = layoutRTL
				wantAlign = nsAlignRight
				wantWriting = writingRTL
			}
			var err error
			runOnMain(func() {
				if e := checkTree(live.native, wantDir); e != nil {
					err = e
					return
				}
				if e := checkMenu(live.menu, wantDir); e != nil {
					err = fmt.Errorf("app menu: %w", e)
					return
				}
				if e := checkMenu(dockMenu.menu, wantDir); e != nil {
					err = fmt.Errorf("dock menu: %w", e)
					return
				}
				// Verify a newly constructed popup menu receives the surface direction.
				pop := live.submenu("", []driver.MenuEntry{
					{Label: "Copy", ID: 1},
					{Label: "More", Items: []driver.MenuEntry{{Label: "Paste", ID: 2}}},
				})
				live.directMenu(pop)
				if e := checkMenu(pop, wantDir); e != nil {
					pop.Send(selRelease)
					err = fmt.Errorf("popup menu: %w", e)
					return
				}
				pop.Send(selRelease)

				// Verify single-line / swapped input alignment & baseWritingDirection.
				sw := findView("swapping")
				if sw == nil || sw.input == nil {
					err = fmt.Errorf("missing swapping input")
					return
				}
				if got := objc.Send[int](sw.input.field, sel("alignment")); got != wantAlign {
					err = fmt.Errorf("swapping input alignment = %d, want %d (rtl=%v)", got, wantAlign, rtl)
					return
				}
				if sw.input.multiline {
					para := sw.input.field.Send(sel("defaultParagraphStyle"))
					if got := objc.Send[int](para, sel("baseWritingDirection")); got != wantWriting {
						err = fmt.Errorf("swapping multiline defaultParagraphStyle baseWritingDirection = %d, want %d", got, wantWriting)
						return
					}
				} else {
					if got := objc.Send[int](sw.input.field.Send(sel("cell")), sel("baseWritingDirection")); got != wantWriting {
						err = fmt.Errorf("swapping field cell baseWritingDirection = %d, want %d", got, wantWriting)
						return
					}
				}

				// Verify multiline input text view & placeholder label.
				area := findView("area")
				if area == nil || area.input == nil || !area.input.multiline {
					err = fmt.Errorf("missing multiline input")
					return
				}
				if got := objc.Send[int](area.input.field, sel("alignment")); got != wantAlign {
					err = fmt.Errorf("multiline alignment = %d, want %d", got, wantAlign)
					return
				}
				if got := objc.Send[int](area.input.label, sel("alignment")); got != wantAlign {
					err = fmt.Errorf("multiline placeholder alignment = %d, want %d", got, wantAlign)
					return
				}
				para := area.input.field.Send(sel("defaultParagraphStyle"))
				if got := objc.Send[int](para, sel("baseWritingDirection")); got != wantWriting {
					err = fmt.Errorf("multiline defaultParagraphStyle baseWritingDirection = %d, want %d", got, wantWriting)
					return
				}

				// Verify NSSwitch native control direction.
				tog := findView("toggle")
				if tog == nil || tog.componentView == 0 {
					err = fmt.Errorf("missing NSSwitch")
					return
				}
				if got := objc.Send[int](tog.componentView, sel("userInterfaceLayoutDirection")); got != wantDir {
					err = fmt.Errorf("NSSwitch userInterfaceLayoutDirection = %d, want %d", got, wantDir)
					return
				}

				// Verify vertical scroll bar indicator edge & thumb grab hit test.
				sc := findView("scroller")
				if sc == nil || sc.sc == nil {
					err = fmt.Errorf("missing scroll view")
					return
				}
				thumb, _, ok := sc.thumbRect()
				if !ok {
					err = fmt.Errorf("scroll view has no vertical thumb")
					return
				}
				if rtl {
					if thumb.Origin.X != scrollbar.Inset {
						err = fmt.Errorf("RTL scroll thumb X = %v, want %v", thumb.Origin.X, scrollbar.Inset)
						return
					}
					if !sc.grabsThumb(driver.Point{X: 4, Y: 10}) || sc.grabsThumb(driver.Point{X: sc.frame.Size.Width - 4, Y: 10}) {
						err = fmt.Errorf("RTL grabsThumb did not mirror to left edge")
						return
					}
				} else {
					wantX := sc.frame.Size.Width - scrollbar.Inset - scrollbar.Thickness
					if thumb.Origin.X != wantX {
						err = fmt.Errorf("LTR scroll thumb X = %v, want %v", thumb.Origin.X, wantX)
						return
					}
					if !sc.grabsThumb(driver.Point{X: sc.frame.Size.Width - 4, Y: 10}) || sc.grabsThumb(driver.Point{X: 4, Y: 10}) {
						err = fmt.Errorf("LTR grabsThumb did not stay on right edge")
						return
					}
				}

				// Verify NSAlert window contentView mirrors.
				alert := objc.ID(apple.Class("NSAlert")).Send(selNew)
				alert.Send(sel("setMessageText:"), apple.NSString("Title"))
				alert.Send(sel("addButtonWithTitle:"), apple.NSString("OK"))
				alert.Send(sel("addButtonWithTitle:"), apple.NSString("Cancel"))
				alert.Send(sel("layout"))
				live.directTree(alert.Send(sel("window")).Send(sel("contentView")))
				if e := checkTree(alert.Send(sel("window")).Send(sel("contentView")), wantDir); e != nil {
					alert.Send(selRelease)
					err = fmt.Errorf("NSAlert: %w", e)
					return
				}
				alert.Send(selRelease)
			})
			return err
		}

		// Phase 1: Start in LTR ("en").
		cx.Post(func() {
			cx.SetLocale("en")
			step.Set(1)
		})
		if err := waitTitle(title + "|false|1"); err != nil {
			done <- err
			cx.Post(cx.Quit)
			return
		}
		if err := verifyPhase(false); err != nil {
			done <- fmt.Errorf("LTR initial: %w", err)
			cx.Post(cx.Quit)
			return
		}

		// Phase 2: Switch dynamically to RTL ("ar-XB"), mount a late switch, and swap editor to masked.
		cx.Post(func() {
			cx.SetLocale("ar-XB")
			setShowLate(true)
			step.Set(2)
		})
		if err := waitTitle(title + "|true|2"); err != nil {
			done <- err
			cx.Post(cx.Quit)
			return
		}
		runOnMain(func() {
			sw := findView("swapping")
			cfg := sw.input.fs.Config
			cfg.Masked = true
			sw.configure(&cfg)
		})
		if err := verifyPhase(true); err != nil {
			done <- fmt.Errorf("RTL masked+late: %w", err)
			cx.Post(cx.Quit)
			return
		}

		// Phase 3: While RTL, swap single-line masked input to multiline NSScrollView+NSTextView.
		runOnMain(func() {
			sw := findView("swapping")
			cfg := sw.input.fs.Config
			cfg.Multiline = true
			cfg.Masked = false
			sw.configure(&cfg)
		})
		if err := verifyPhase(true); err != nil {
			done <- fmt.Errorf("RTL swapped multiline: %w", err)
			cx.Post(cx.Quit)
			return
		}

		// Phase 4: Switch dynamically back to LTR ("en") and swap back to single-line.
		runOnMain(func() {
			sw := findView("swapping")
			cfg := sw.input.fs.Config
			cfg.Multiline = false
			cfg.Masked = false
			sw.configure(&cfg)
		})
		cx.Post(func() {
			cx.SetLocale("en")
			step.Set(4)
		})
		if err := waitTitle(title + "|false|4"); err != nil {
			done <- err
			cx.Post(cx.Quit)
			return
		}
		if err := verifyPhase(false); err != nil {
			done <- fmt.Errorf("LTR restored: %w", err)
			cx.Post(cx.Quit)
			return
		}

		done <- nil
		cx.Post(cx.Quit)
	}()

	onMain(func() {
		Run(core.AppRoot(func(cx *core.Scope) { cx.SetMainView(app) }),
			Options{Title: title, Width: 360, Height: 280})
	})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// TestEmbeddedDirectionLeavesHostAlone verifies that an embedded surface mirrors
// its own AppKit subviews without altering sibling host views or the host window.
func TestEmbeddedDirectionLeavesHostAlone(t *testing.T) {
	onMain(func() {
		loadAppKit()
		win := objc.ID(apple.Class("NSWindow")).Send(selAlloc).Send(
			sel("initWithContentRect:styleMask:backing:defer:"),
			apple.Rect(0, 0, 320, 200), uint(0), uint(2), false)
		win.Send(sel("setReleasedWhenClosed:"), false)
		defer win.Send(selRelease)
		content := win.Send(sel("contentView"))
		content.Send(selSetLayoutDirection, layoutLTR)

		hostSibling := objc.ID(apple.Class("NSTextField")).Send(selNew)
		hostSibling.Send(selSetLayoutDirection, layoutLTR)
		content.Send(selAddSubview, hostSibling)
		defer hostSibling.Send(selRelease)

		embedded := NewView(func(*core.Scope) core.Node { return core.View{} }, Options{Width: 160, Height: 120})
		defer embedded.Close()
		content.Send(selAddSubview, objc.ID(embedded.Handle()))

		// Mount an input inside the embedded surface, then apply RTL direction.
		embedded.s.apply(&driver.MountBatch{
			Root: 1,
			Items: []driver.MountItem{
				{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
				{Kind: driver.ItemCreate, ID: 2, View: driver.KindInput},
				{Kind: driver.ItemInsert, ID: 2, Parent: 1},
				{Kind: driver.ItemDirection, RTL: true, Language: "ar-XB"},
			},
		})
		if got := objc.Send[int](embedded.s.native, sel("userInterfaceLayoutDirection")); got != layoutRTL {
			t.Errorf("embedded surface direction = %d, want %d", got, layoutRTL)
		}
		if got := objc.Send[int](embedded.s.views[2].input.field, sel("userInterfaceLayoutDirection")); got != layoutRTL {
			t.Errorf("embedded input direction = %d, want %d", got, layoutRTL)
		}
		if got := objc.Send[int](hostSibling, sel("userInterfaceLayoutDirection")); got != layoutLTR {
			t.Errorf("host sibling direction changed to %d, want %d", got, layoutLTR)
		}
	})
}

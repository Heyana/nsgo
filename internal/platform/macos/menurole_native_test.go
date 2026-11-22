//go:build darwin && !ios

package macos

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func menuRoleFallbackApp(title string) func(*core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		var button, field core.Ref
		focused := core.NewSignal(cx, "ready")
		selected, undone, cut := core.NewSignal(cx, 0), core.NewSignal(cx, 0), core.NewSignal(cx, 0)
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%s|%d|%d", title, focused.Get(), selected.Get(), undone.Get())) })
		cx.SetApplicationMenu([]core.MenuItem{{Label: "Edit", Items: []core.MenuItem{
			{Role: core.MenuRedo, OnSelect: func() { undone.Update(func(n int) int { return n + 1 }) }},
			{Role: core.MenuUndo, OnSelect: func() { undone.Update(func(n int) int { return n + 1 }) }},
			{Role: core.MenuSelectAll, OnSelect: func() { selected.Update(func(n int) int { return n + 1 }) }},
			{Role: core.MenuCut, OnSelect: func() { cut.Update(func(n int) int { return n + 1 }) }},
		}}})
		cx.OnKeyWith("mod+f", core.KeyOptions{When: core.KeyAlways}, field.Focus)
		cx.OnKeyWith("mod+b", core.KeyOptions{When: core.KeyAlways}, button.Focus)
		cx.OnKeyWith("mod+m", core.KeyOptions{When: core.KeyAlways}, func() { focused.Set(focused.Get() + "-checked") })
		cx.OnKeyWith("mod+q", core.KeyOptions{When: core.KeyAlways}, cx.Quit)
		return core.View{Style: core.Style{Width: core.Pt(200), Height: core.Pt(200)}, Children: []core.Node{
			core.Input{TestID: "field", Ref: &field, DefaultValue: "hello", Style: core.Style{Height: core.Pt(30)},
				OnFocus: func() { focused.Set("field") }},
			core.View{TestID: "button", Ref: &button, Style: core.Style{Width: core.Pt(100), Height: core.Pt(40)},
				OnFocus: func() { focused.Set("button") }, OnPress: func(core.PressEvent) {}},
		}}
	}
}

// TestMenuRoleFallback chooses Edit menu role items that carry an OnSelect: the focused field takes Select All, Undo and Redo, and a view leaves them to OnSelect. Undo, Redo and Cut follow the field's history and selection even while AppKit has no key window. Run owns AppKit's process lifetime, so the test gets a process of its own.
func TestMenuRoleFallback(t *testing.T) {
	if os.Getenv("NSGO_TEST_MENU_ROLE_PROCESS") != t.Name() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_MENU_ROLE_PROCESS="+t.Name(), "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	const title = "nsgo-menu-role"
	done := make(chan error, 1)
	go func() {
		var live *surface
		// main runs fn on the main queue and waits for it.
		main := func(fn func()) {
			barrier := make(chan struct{})
			apple.OnMain(func() {
				defer close(barrier)
				fn()
			})
			<-barrier
		}
		wait := func(want string) error {
			deadline := time.Now().Add(5 * time.Second)
			last := ""
			for time.Now().Before(deadline) {
				main(func() {
					for s := range surfaces {
						if s.window != 0 && strings.HasPrefix(apple.GoString(s.window.Send(sel("title"))), title) {
							live = s
						}
					}
					if live != nil {
						last = apple.GoString(live.window.Send(sel("title")))
					}
				})
				if last == want {
					return nil
				}
				time.Sleep(10 * time.Millisecond)
			}
			return fmt.Errorf("window did not reach %q: it shows %q", want, last)
		}
		key := func(name string) {
			apple.OnMain(func() {
				if live == nil {
					finish()
					return
				}
				// The platform reports a key binding by its ID, as it does a matched press.
				for _, b := range live.keys {
					if b.Key == name {
						live.engine.Dispatch(driver.Event{Kind: driver.EventKey, Count: b.ID})
					}
				}
			})
		}
		// choose picks the Edit menu's item of role, as a click on it does; its label is in the system's language.
		choose := func(role driver.MenuRole) (err error) {
			main(func() {
				edit := nsApp().Send(sel("mainMenu")).Send(selItemAtIndex, 0).Send(sel("submenu"))
				for i := range objc.Send[int](edit, sel("numberOfItems")) {
					if it := edit.Send(selItemAtIndex, i); live.roleFallback[uint32(objc.Send[int](it, sel("tag")))] == role {
						edit.Send(sel("performActionForItemAtIndex:"), i)
						return
					}
				}
				err = fmt.Errorf("the Edit menu has no %s item", role.Label())
			})
			return err
		}
		menuEnabled := func(role driver.MenuRole) (enabled bool, err error) {
			main(func() {
				edit := nsApp().Send(sel("mainMenu")).Send(selItemAtIndex, 0).Send(sel("submenu"))
				for i := range objc.Send[int](edit, sel("numberOfItems")) {
					if it := edit.Send(selItemAtIndex, i); live.roleFallback[uint32(objc.Send[int](it, sel("tag")))] == role {
						enabled = validateMenu(live.menuTarget, objc.SEL(0), it)
						return
					}
				}
				err = fmt.Errorf("the Edit menu has no %s item", role.Label())
			})
			return enabled, err
		}
		check := func() error {
			if err := wait(title + "|ready|0|0"); err != nil {
				return err
			}
			for _, role := range []driver.MenuRole{driver.MenuUndo, driver.MenuRedo, driver.MenuCut} {
				if enabled, err := menuEnabled(role); err != nil || !enabled {
					return fmt.Errorf("with nothing focused, %s enabled = %v: %v", role.Label(), enabled, err)
				}
			}
			key("mod+b")
			if err := wait(title + "|button|0|0"); err != nil {
				return err
			}
			if enabled, err := menuEnabled(driver.MenuUndo); err != nil || !enabled {
				return fmt.Errorf("with no field focused, Undo enabled = %v: %v", enabled, err)
			}
			for _, role := range []driver.MenuRole{driver.MenuSelectAll, driver.MenuUndo, driver.MenuRedo} {
				if err := choose(role); err != nil {
					return err
				}
			}
			if err := wait(title + "|button|1|2"); err != nil {
				return fmt.Errorf("with no field focused: %w", err)
			}
			key("mod+f")
			if err := wait(title + "|field|1|2"); err != nil {
				return err
			}
			if enabled, err := menuEnabled(driver.MenuUndo); err != nil || enabled {
				return fmt.Errorf("with an empty undo stack, Undo enabled = %v: %v", enabled, err)
			}
			if enabled, err := menuEnabled(driver.MenuRedo); err != nil || enabled {
				return fmt.Errorf("with an empty redo stack, Redo enabled = %v: %v", enabled, err)
			}
			if enabled, err := menuEnabled(driver.MenuCut); err != nil || enabled {
				return fmt.Errorf("with no selection, Cut enabled = %v: %v", enabled, err)
			}
			var typed string
			main(func() {
				first := live.window.Send(sel("firstResponder"))
				first.Send(sel("insertText:"), apple.NSString("x"))
				typed = apple.GoString(first.Send(sel("string")))
			})
			if typed == "hello" || len(typed) != len("hellox") {
				return fmt.Errorf("typing left %q, want one added character", typed)
			}
			if enabled, err := menuEnabled(driver.MenuUndo); err != nil || !enabled {
				return fmt.Errorf("after typing, Undo enabled = %v: %v", enabled, err)
			}
			if enabled, err := menuEnabled(driver.MenuRedo); err != nil || enabled {
				return fmt.Errorf("after typing, Redo enabled = %v: %v", enabled, err)
			}
			main(func() {
				first := live.window.Send(sel("firstResponder"))
				first.Send(sel("selectAll:"), objc.ID(0))
			})
			if enabled, err := menuEnabled(driver.MenuCut); err != nil || !enabled {
				return fmt.Errorf("after selecting text, Cut enabled = %v: %v", enabled, err)
			}
			for _, role := range []driver.MenuRole{driver.MenuUndo, driver.MenuSelectAll} {
				if err := choose(role); err != nil {
					return err
				}
			}
			if enabled, err := menuEnabled(driver.MenuUndo); err != nil || enabled {
				return fmt.Errorf("after undo, Undo enabled = %v: %v", enabled, err)
			}
			if enabled, err := menuEnabled(driver.MenuRedo); err != nil || !enabled {
				return fmt.Errorf("after undo, Redo enabled = %v: %v", enabled, err)
			}
			if err := choose(driver.MenuRedo); err != nil {
				return err
			}
			var redone string
			main(func() { redone = apple.GoString(live.window.Send(sel("firstResponder")).Send(sel("string"))) })
			if redone != typed {
				return fmt.Errorf("redo left %q, want %q", redone, typed)
			}
			if enabled, err := menuEnabled(driver.MenuUndo); err != nil || !enabled {
				return fmt.Errorf("after redo, Undo enabled = %v: %v", enabled, err)
			}
			if enabled, err := menuEnabled(driver.MenuRedo); err != nil || enabled {
				return fmt.Errorf("after redo, Redo enabled = %v: %v", enabled, err)
			}
			for _, role := range []driver.MenuRole{driver.MenuUndo, driver.MenuSelectAll} {
				if err := choose(role); err != nil {
					return err
				}
			}
			// The marker's key comes after the menu's reports, so the counts are final once it shows.
			key("mod+m")
			if err := wait(title + "|field-checked|1|2"); err != nil {
				return fmt.Errorf("with a field focused: %w", err)
			}
			var got apple.NSRange
			main(func() {
				if e := live.window.Send(sel("firstResponder")); objc.Send[bool](e, sel("isKindOfClass:"), apple.Class("NSText")) {
					got = objc.Send[apple.NSRange](e, selSelectedRange)
				}
			})
			if got.Location != 0 || got.Length != uint(len("hello")) {
				return fmt.Errorf("Select All selected %+v of the field, want all of %q", got, "hello")
			}
			key("mod+b")
			if err := wait(title + "|button|1|2"); err != nil {
				return err
			}
			for _, role := range []driver.MenuRole{driver.MenuUndo, driver.MenuRedo, driver.MenuCut} {
				if enabled, err := menuEnabled(role); err != nil || !enabled {
					return fmt.Errorf("after leaving the field, %s enabled = %v: %v", role.Label(), enabled, err)
				}
			}
			for _, role := range []driver.MenuRole{driver.MenuUndo, driver.MenuRedo} {
				if err := choose(role); err != nil {
					return err
				}
			}
			key("mod+m")
			return wait(title + "|button-checked|1|4")
		}
		err := check()
		done <- err
		key("mod+q")
	}()
	onMain(func() {
		Run(core.AppRoot(func(cx *core.Scope) { cx.SetMainView(menuRoleFallbackApp(title)) }),
			Options{Title: title, Width: 200, Height: 200})
	})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

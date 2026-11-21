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

// TestKeysComeUpAsTheWindowResignsKey holds a key on a focused view that hears it, for an app that reads the keys held: a down that goes down again without repeating reports its lost up first, the key held comes up as the window stops being key (NSWindowDidResignKeyNotification) and leaves core.KeysDown empty, and its up AppKit sends later is not heard again (#1046). Run owns AppKit's process lifetime, so the test gets a process of its own.
func TestKeysComeUpAsTheWindowResignsKey(t *testing.T) {
	if os.Getenv("NSGO_TEST_KEYS_PROCESS") != t.Name() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_KEYS_PROCESS="+t.Name(), "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	const title = "nsgo-keys-resign"
	app := func(cx *core.Scope) core.Node {
		var pad core.Ref
		log := core.NewSignal(cx, "")
		mark := func(s string) {
			log.Update(func(l string) string {
				if l != "" {
					l += ","
				}
				return l + s
			})
		}
		key := func(prefix string, e core.KeyEvent) {
			s := prefix + e.Key + "/" + e.Code
			if e.Repeat {
				s += "!"
			}
			mark(s)
		}
		cx.Effect(func() {
			cx.SetTitle(fmt.Sprintf("%s|%s|%s|%s|%s", title, log.Get(), strings.Join(core.KeysDown(cx), " "), strings.Join(core.KeyCodesDown(cx), " "), core.KeyCodeLabel(cx, "Enter")))
		})
		cx.OnKeyWith("mod+p", core.KeyOptions{When: core.KeyAlways}, pad.Focus)
		cx.OnKeyWith("mod+q", core.KeyOptions{When: core.KeyAlways}, cx.Quit)
		return core.View{Style: core.Style{Width: core.Pt(200), Height: core.Pt(200)}, Children: []core.Node{
			core.View{TestID: "pad", Ref: &pad, Focusable: func() bool { return true }, Style: core.Style{Width: core.Pt(100), Height: core.Pt(40)},
				OnFocus:   func() { mark("f") },
				OnKeyDown: func(e core.KeyEvent) bool { key("d:", e); return false },
				OnKeyUp:   func(e core.KeyEvent) { key("u:", e) }},
		}}
	}
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
			return fmt.Errorf("window did not reach %q, it shows %q", want, last)
		}
		binding := func(name string) {
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
		// key sends the B key's event of kind through the window's filter, as AppKit hands the app its keys.
		key := func(kind uint, repeat bool) {
			main(func() {
				live.filter(apple.KeyEvent(kind, 0, 0, objc.Send[int](live.window, sel("windowNumber")), "b", "b", repeat, 11))
			})
		}
		check := func() error {
			if err := wait(title + "||||↩"); err != nil {
				return err
			}
			binding("mod+p")
			if err := wait(title + "|f|||↩"); err != nil {
				return err
			}
			key(evKeyDown, false)
			key(evKeyDown, true)
			if err := wait(title + "|f,d:b/KeyB,d:b/KeyB!|b|KeyB|↩"); err != nil {
				return err
			}
			// Its up never came: the key going down again without repeating reports it first.
			key(evKeyDown, false)
			if err := wait(title + "|f,d:b/KeyB,d:b/KeyB!,u:b/KeyB,d:b/KeyB|b|KeyB|↩"); err != nil {
				return err
			}
			var held int
			main(func() {
				center := objc.ID(apple.Class("NSNotificationCenter")).Send(sel("defaultCenter"))
				center.Send(sel("postNotificationName:object:"), apple.Global(appKitPath, "NSWindowDidResignKeyNotification"), live.window)
			})
			if err := wait(title + "|f,d:b/KeyB,d:b/KeyB!,u:b/KeyB,d:b/KeyB,u:b/KeyB|||↩"); err != nil {
				return err
			}
			main(func() { held = len(live.keysHeld) })
			if held != 0 {
				return fmt.Errorf("%d keys still held after the window stopped being key", held)
			}
			// AppKit's up of the key released then is not heard again, and the key presses anew after.
			key(evKeyUp, false)
			key(evKeyDown, false)
			key(evKeyUp, false)
			return wait(title + "|f,d:b/KeyB,d:b/KeyB!,u:b/KeyB,d:b/KeyB,u:b/KeyB,d:b/KeyB,u:b/KeyB|||↩")
		}
		err := check()
		done <- err
		binding("mod+q")
	}()
	onMain(func() {
		Run(core.AppRoot(func(cx *core.Scope) { cx.SetMainView(app) }), Options{Title: title, Width: 200, Height: 200})
	})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

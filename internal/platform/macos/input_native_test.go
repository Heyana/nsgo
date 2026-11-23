//go:build darwin && !ios

package macos

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"slices"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// TestInputMethodCommitAndSinkCandidates runs a live AppKit window with real single-line, multiline and point-wide sink core.Inputs and verifies that:
//   - marked text still composed on blur (or moving focus to another input) is committed and reported with OnCompositionChange{Composing:false} before OnBlur, leaving hasMarkedText false;
//   - firstRectForCharacterRange:actualRange: on a 1pt-wide editor sink opens candidates beside the sink's caret in screen coordinates rather than at (0, 0) (#1562).
func TestInputMethodCommitAndSinkCandidates(t *testing.T) {
	if os.Getenv("NSGO_TEST_IME_PROCESS") != t.Name() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_IME_PROCESS="+t.Name(), "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}

	const title = "nsgo-macos-ime"
	var (
		mu   sync.Mutex
		log  []string
		note = func(s string) {
			mu.Lock()
			log = append(log, s)
			mu.Unlock()
		}
		snapshot = func() []string {
			mu.Lock()
			defer mu.Unlock()
			return append([]string(nil), log...)
		}
		clearLog = func() {
			mu.Lock()
			log = nil
			mu.Unlock()
		}
	)

	var singleRef, multiRef, otherRef, sinkRef core.Ref
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetMainView(func(cx *core.Scope) core.Node {
			cx.SetTitle(title)
			cx.OnKeyWith("mod+1", core.KeyOptions{When: core.KeyAlways}, singleRef.Focus)
			cx.OnKeyWith("mod+2", core.KeyOptions{When: core.KeyAlways}, multiRef.Focus)
			cx.OnKeyWith("mod+3", core.KeyOptions{When: core.KeyAlways}, otherRef.Focus)
			cx.OnKeyWith("mod+4", core.KeyOptions{When: core.KeyAlways}, sinkRef.Focus)
			cx.OnKeyWith("mod+0", core.KeyOptions{When: core.KeyAlways}, func() {
				singleRef.Blur()
				multiRef.Blur()
				sinkRef.Blur()
			})
			cx.OnKeyWith("mod+q", core.KeyOptions{When: core.KeyAlways}, cx.Quit)
			clear := core.RGBA(0xffffff, 0)
			return core.View{Style: core.Style{Width: core.Pt(400), Height: core.Pt(300)}, Children: []core.Node{
				core.Input{
					TestID: "single", Ref: &singleRef,
					Style:               core.Style{Width: core.Pt(240), Height: core.Pt(30)},
					OnChange:            func(s string) { note("single:change:" + s) },
					OnCompositionChange: func(c core.Composition) { note(fmt.Sprintf("single:comp:%v:%s", c.Composing, c.Text)) },
					OnBlur:              func() { note("single:blur") },
				},
				core.Input{
					TestID: "multi", Ref: &multiRef, Multiline: true,
					Style:               core.Style{Width: core.Pt(240), Height: core.Pt(60)},
					OnChange:            func(s string) { note("multi:change:" + s) },
					OnCompositionChange: func(c core.Composition) { note(fmt.Sprintf("multi:comp:%v:%s", c.Composing, c.Text)) },
					OnBlur:              func() { note("multi:blur") },
				},
				core.Input{
					TestID: "other", Ref: &otherRef,
					Style: core.Style{Width: core.Pt(240), Height: core.Pt(30)},
				},
				core.Input{
					TestID: "sink", Ref: &sinkRef, HideCaret: true,
					Style: core.Style{
						Position: core.Absolute, Left: core.Pt(140), Top: core.Pt(180),
						Width: core.Pt(1), Height: core.Pt(20),
						Color: clear, Background: clear, SelectionColor: clear,
						FontSize: 14, Padding: core.Edges{}, BorderWidth: 0,
					},
				},
			}}
		})
	})

	done := make(chan error, 1)
	go func() {
		var live *surface
		syncMain := func(fn func()) {
			barrier := make(chan struct{})
			apple.OnMain(func() {
				defer close(barrier)
				fn()
			})
			<-barrier
		}
		waitSurface := func() error {
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				syncMain(func() {
					for s := range surfaces {
						if s.window != 0 && apple.GoString(s.window.Send(sel("title"))) == title {
							live = s
							break
						}
					}
				})
				if live != nil {
					return nil
				}
				time.Sleep(10 * time.Millisecond)
			}
			return fmt.Errorf("window %q did not appear", title)
		}
		// key presses the binding name once the window's keymap holds it: the window shows before its first keymap is mounted, and a key sent before then reaches no binding.
		key := func(name string) {
			for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
				sent := false
				syncMain(func() {
					if live == nil {
						finish()
						sent = true
						return
					}
					for _, b := range live.keys {
						if b.Key == name {
							live.engine.Dispatch(driver.Event{Kind: driver.EventKey, Count: b.ID})
							sent = true
						}
					}
				})
				if sent || time.Now().After(deadline) {
					return
				}
			}
		}
		waitFor := func(desc string, cond func() bool) error {
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if cond() {
					return nil
				}
				time.Sleep(10 * time.Millisecond)
			}
			return fmt.Errorf("timed out waiting for %s (log=%v)", desc, snapshot())
		}
		viewByTestID := func(id string) *view {
			for _, v := range live.views {
				if v != nil && v.props.TestID == id {
					return v
				}
			}
			return nil
		}

		check := func() error {
			if err := waitSurface(); err != nil {
				return err
			}
			const message = "今日はいい天気"
			n := uint(driver.UTF16Len(message))

			// 1. Single-line input composing live conversion, then moving focus to another input.
			clearLog()
			if err := waitFor("test window active and key", func() bool {
				var ok bool
				syncMain(func() {
					ok = objc.Send[bool](nsApp(), sel("isActive")) &&
						objc.Send[bool](live.window, sel("isKeyWindow"))
				})
				return ok
			}); err != nil {
				return err
			}
			key("mod+1")
			if err := waitFor("single focused", func() bool {
				var ok bool
				syncMain(func() {
					if v := viewByTestID("single"); v != nil && v.input != nil && v.input.focused && v.input.editor() != 0 {
						ok = true
					}
				})
				return ok
			}); err != nil {
				return err
			}
			syncMain(func() {
				e := viewByTestID("single").input.editor()
				e.Send(sel("setMarkedText:selectedRange:replacementRange:"),
					apple.NSString(message), apple.NSRange{Location: n}, apple.NSRange{Location: nsNotFound})
			})
			if err := waitFor("single composing", func() bool {
				return slices.Contains(snapshot(), "single:comp:true:"+message)
			}); err != nil {
				return err
			}
			key("mod+3")
			if err := waitFor("single blur", func() bool {
				return slices.Contains(snapshot(), "single:blur")
			}); err != nil {
				return err
			}
			events := snapshot()
			changeAt := slices.Index(events, "single:change:"+message)
			endAt := slices.Index(events, "single:comp:false:")
			blurAt := slices.Index(events, "single:blur")
			if changeAt < 0 || endAt < 0 || blurAt < 0 || !(changeAt < endAt && endAt < blurAt) {
				return fmt.Errorf("single-line order wrong: change=%d end=%d blur=%d in %v", changeAt, endAt, blurAt, events)
			}
			var singleText string
			syncMain(func() { singleText = viewByTestID("single").input.text() })
			if singleText != message {
				return fmt.Errorf("single-line text after blur = %q, want %q", singleText, message)
			}

			// 2. Multiline input composing live conversion, then blurring programmatically.
			clearLog()
			key("mod+2")
			if err := waitFor("multi focused", func() bool {
				var ok bool
				syncMain(func() {
					if v := viewByTestID("multi"); v != nil && v.input != nil && v.input.focused {
						ok = true
					}
				})
				return ok
			}); err != nil {
				return err
			}
			syncMain(func() {
				e := viewByTestID("multi").input.editor()
				e.Send(sel("setMarkedText:selectedRange:replacementRange:"),
					apple.NSString(message), apple.NSRange{Location: n}, apple.NSRange{Location: nsNotFound})
			})
			if err := waitFor("multi composing", func() bool {
				return slices.Contains(snapshot(), "multi:comp:true:"+message)
			}); err != nil {
				return err
			}
			key("mod+0")
			if err := waitFor("multi blur", func() bool {
				return slices.Contains(snapshot(), "multi:blur")
			}); err != nil {
				return err
			}
			events = snapshot()
			changeAt = slices.Index(events, "multi:change:"+message)
			endAt = slices.Index(events, "multi:comp:false:")
			blurAt = slices.Index(events, "multi:blur")
			if changeAt < 0 || endAt < 0 || blurAt < 0 || !(changeAt < endAt && endAt < blurAt) {
				return fmt.Errorf("multiline order wrong: change=%d end=%d blur=%d in %v", changeAt, endAt, blurAt, events)
			}
			var multiText string
			var stillMarked bool
			syncMain(func() {
				v := viewByTestID("multi")
				multiText = v.input.text()
				stillMarked = objc.Send[bool](v.input.editor(), selHasMarkedText)
			})
			if multiText != message || stillMarked {
				return fmt.Errorf("multiline after blur: text=%q marked=%v, want %q and unmarked", multiText, stillMarked, message)
			}

			// 3. Point-wide editor sink candidate rect opens beside the sink, not at (0, 0).
			key("mod+4")
			if err := waitFor("sink focused", func() bool {
				var ok bool
				syncMain(func() {
					if v := viewByTestID("sink"); v != nil && v.input != nil && v.input.focused && v.input.editor() != 0 {
						ok = true
					}
				})
				return ok
			}); err != nil {
				return err
			}
			var rectErr error
			syncMain(func() {
				v := viewByTestID("sink")
				e := v.input.editor()
				e.Send(sel("setMarkedText:selectedRange:replacementRange:"),
					apple.NSString(message), apple.NSRange{Location: n}, apple.NSRange{Location: nsNotFound})
				e.Send(sel("scrollRangeToVisible:"), apple.NSRange{Location: n})
				winBox := objc.Send[apple.CGRect](v.native, sel("convertRect:toView:"),
					objc.Send[apple.CGRect](v.native, selBounds), objc.ID(0))
				want := objc.Send[apple.CGRect](live.window, sel("convertRectToScreen:"), winBox)
				for _, r := range []apple.NSRange{
					{Location: 0, Length: 0},
					{Location: 0, Length: n},
					{Location: 3, Length: n - 3},
					{Location: n, Length: 0},
				} {
					var actual apple.NSRange
					got := objc.Send[apple.CGRect](e, selFirstRectForRange, r, unsafe.Pointer(&actual))
					if got.Size.Height <= 0 || math.Abs(got.Origin.X-want.Origin.X) > 4 || math.Abs(got.Origin.Y-want.Origin.Y) > 6 {
						rectErr = fmt.Errorf("sink range %+v: candidate rect %+v, want beside %+v", r, got, want)
						return
					}
				}
			})
			if rectErr != nil {
				return rectErr
			}
			return nil
		}

		done <- check()
		key("mod+q")
	}()

	onMain(func() {
		Run(app, Options{Title: title, Width: 400, Height: 300})
	})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

//go:build darwin && !ios

package macos

import (
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func TestInputReportsDroppedComposition(t *testing.T) {
	for _, mode := range []struct {
		name      string
		multiline bool
	}{{"single-line", false}, {"multiline", true}} {
		t.Run(mode.name, func(t *testing.T) {
			onMain(func() {
				loadAppKit()
				window := objc.ID(apple.Class("NSWindow")).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
					apple.Rect(0, 0, 300, 120), uint(0), uint(2), false)
				window.Send(sel("setReleasedWhenClosed:"), false)
				defer window.Send(selRelease)
				s := &surface{window: window, views: make([]*view, 2), animator: driver.NewAnimator(func(driver.Event) {})}
				v := s.create(driver.KindInput, 1)
				s.views[v.id] = v
				defer s.delete(v, false)
				window.Send(sel("contentView")).Send(selAddSubview, v.native)
				v.native.Send(selSetFrame, apple.Rect(0, 0, 300, 120))
				v.configure(&driver.InputConfig{Multiline: mode.multiline, Content: driver.Bounds{Size: driver.Size{Width: 300, Height: 120}}})
				in := v.input
				var events []driver.Event
				in.fs.Events = 1<<driver.EventChangeText | 1<<driver.EventCompositionChange
				in.fs.Dispatch = func(ev driver.Event) { events = append(events, ev) }
				in.value("ab", 0)
				window.Send(sel("makeFirstResponder:"), in.field)
				editor := in.editor()
				if editor == 0 {
					t.Fatal("input has no editor")
				}
				editor.Send(selSetSelRange, apple.NSRange{Location: 2})
				mark := sel("setMarkedText:selectedRange:replacementRange:")
				replacement := apple.NSRange{Location: nsNotFound}
				editor.Send(mark, apple.NSString("r"), apple.NSRange{Location: 1}, replacement)
				if !in.fs.Composing() || in.fs.Known() != "abr" {
					t.Fatalf("reported %q, composing %v; want abr and an active composition", in.fs.Known(), in.fs.Composing())
				}
				events = nil
				editor.Send(mark, apple.NSString(""), apple.NSRange{}, replacement)
				if got := in.text(); got != "ab" {
					t.Fatalf("editor holds %q after dropping marked text, want ab", got)
				}
				if len(events) != 2 || events[0].Kind != driver.EventChangeText || events[0].Text != "ab" ||
					events[1].Kind != driver.EventCompositionChange || !events[1].Finished {
					t.Errorf("after dropping marked text: got %v, want a text change to ab followed by composition end", events)
				}
			})
		})
	}
}

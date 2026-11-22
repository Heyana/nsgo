//go:build darwin && !ios

package macos

import (
	"strings"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

type selectionHarness struct {
	window   objc.ID
	surface  *surface
	area     *view
	textView *view
	layout   *apple.Layout
	cleanup  func()
}

func newSelectionHarness(t *testing.T, text string) *selectionHarness {
	t.Helper()
	loadAppKit()
	_ = objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
	window := objc.ID(classWindow).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
		apple.Rect(0, 0, 400, 200), uint(0), uint(2), false)
	window.Send(sel("setReleasedWhenClosed:"), false)
	s := newSurface(Options{Width: 400, Height: 200}, false)
	s.window = window
	window.Send(sel("setContentView:"), s.native)

	style := driver.TextStyle{Font: driver.Font{Size: 18}, Color: driver.RGB(0)}
	prepared := apple.TextEngine{}.Prepare(&driver.AttributedText{Text: text, Style: style})
	size := prepared.Measure(driver.Auto)
	layout := prepared.Layout(size).(*apple.Layout)
	prepared.Release()

	areaNative := objc.ID(classView).Send(selNew)
	areaNative.Send(selSetFrame, apple.Rect(0, 0, 400, 200))
	s.native.Send(selAddSubview, areaNative)
	area := &view{
		surface: s,
		id:      1,
		kind:    driver.KindView,
		native:  areaNative,
		content: areaNative,
		style:   driver.DefaultViewStyle,
		area:    true,
	}
	views[areaNative] = area

	textNative := objc.ID(classView).Send(selNew)
	textNative.Send(selSetFrame, apple.Rect(20, 20, float64(size.Width), float64(size.Height)))
	areaNative.Send(selAddSubview, textNative)
	tv := &view{
		surface:    s,
		id:         2,
		kind:       driver.KindText,
		native:     textNative,
		content:    textNative,
		up:         area,
		style:      driver.DefaultViewStyle,
		layout:     layout,
		box:        driver.Bounds{Size: size},
		plain:      text,
		selectable: driver.SelectableOn,
	}
	views[textNative] = tv
	s.views = []*view{nil, area, tv}

	return &selectionHarness{
		window:   window,
		surface:  s,
		area:     area,
		textView: tv,
		layout:   layout,
		cleanup: func() {
			s.clearSelection()
			s.close()
			delete(views, s.native)
			s.native.Send(selRelease)
			window.Send(sel("close"))
			window.Send(selRelease)
		},
	}
}

func (h *selectionHarness) selectAll() {
	sl := &selection{area: h.area}
	h.surface.selecting = sl
	sl.list()
	sl.model.SelectAll(sl)
	h.window.Send(selMakeFirstResponer, h.surface.native)
}

func (h *selectionHarness) rightClickInSurface(p apple.CGPoint) objc.ID {
	loc := objc.Send[apple.CGPoint](h.surface.native, sel("convertPoint:toView:"), p, objc.ID(0))
	return objc.ID(apple.Class("NSEvent")).Send(
		sel("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
		evRightDown, loc, uint(0), 0.0, objc.Send[int](h.window, sel("windowNumber")), objc.ID(0), 0, 1, float32(1))
}

func TestSelectionMenuOffersTextServices(t *testing.T) {
	onMain(func() {
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))

		h := newSelectionHarness(t, "hello world")
		defer h.cleanup()
		h.selectAll()

		// Right-click over the selected text.
		ev := h.rightClickInSurface(apple.CGPoint{X: 30, Y: 28})
		menu := h.textView.native.Send(sel("menuForEvent:"), ev)
		if menu == 0 {
			t.Fatal("menuForEvent: over an active selection returned nil")
		}
		if !objc.Send[bool](menu, sel("allowsContextMenuPlugIns")) {
			t.Error("selection menu disabled context menu plug-ins (macOS Services)")
		}

		var hasLookUp, hasTranslate, hasCopy, hasShare bool
		var prevSep bool
		count := objc.Send[int](menu, sel("numberOfItems"))
		if count == 0 {
			t.Fatal("selection menu is empty")
		}
		for i := range count {
			item := menu.Send(sel("itemAtIndex:"), i)
			if objc.Send[bool](item, sel("isSeparatorItem")) {
				if i == 0 || i == count-1 || prevSep {
					t.Errorf("unexpected separator at index %d (count %d)", i, count)
				}
				prevSep = true
				continue
			}
			prevSep = false
			title := apple.GoString(item.Send(sel("title")))
			action := objc.Send[objc.SEL](item, sel("action"))
			switch action {
			case sel("_rvMenuItemAction"):
				cls := apple.GoString(item.Send(sel("className")))
				if strings.Contains(cls, "Translate") || strings.Contains(title, "Translate") {
					hasTranslate = true
				} else {
					hasLookUp = true
				}
			case sel("copy:"):
				hasCopy = true
				if got := item.Send(sel("target")); got != h.surface.native {
					t.Errorf("Copy target = %v, want surface %v", got, h.surface.native)
				}
			case sel("_performStandardShareMenuItem:"):
				hasShare = true
			case sel("cut:"), sel("paste:"), sel("pasteAsPlainText:"), sel("showWritingTools:"):
				t.Errorf("editing item %q (%v) leaked into read-only selection menu", title, action)
			}
		}
		if !hasLookUp {
			t.Error("selection menu lacks Look Up item")
		}
		if !hasTranslate {
			t.Error("selection menu lacks Translate item")
		}
		if !hasCopy {
			t.Error("selection menu lacks Copy item")
		}
		if !hasShare {
			t.Error("selection menu lacks Share item")
		}

		// The hidden helper must not intercept hit tests, focus or VoiceOver.
		if h.surface.selHelper == 0 {
			t.Fatal("selection helper was not created")
		}
		if hit := h.surface.selHelper.Send(sel("hitTest:"), apple.CGPoint{X: 1, Y: 1}); hit != 0 {
			t.Errorf("selection helper took hitTest: %v", hit)
		}
		if objc.Send[bool](h.surface.selHelper, sel("acceptsFirstResponder")) {
			t.Error("selection helper accepts first responder")
		}
		if objc.Send[bool](h.surface.selHelper, sel("isAccessibilityElement")) {
			t.Error("selection helper exposes an accessibility element")
		}

		// Right-click outside the selection highlight returns no selection menu.
		offEv := h.rightClickInSurface(apple.CGPoint{X: 350, Y: 180})
		if off := h.textView.native.Send(sel("menuForEvent:"), offEv); off != 0 {
			t.Errorf("menuForEvent: outside selection returned %v, want nil", off)
		}
		if !h.surface.selectionMenuAt(ev) || h.surface.selectionMenuAt(offEv) {
			t.Error("selectionMenuAt did not distinguish points on and off the selection")
		}

		// An app's own OnContextMenu takes precedence over the selection menu.
		h.textView.events |= 1 << driver.EventContextMenu
		if appOwned := h.textView.native.Send(sel("menuForEvent:"), ev); appOwned != 0 {
			t.Errorf("menuForEvent: with EventContextMenu returned %v, want nil", appOwned)
		}
		if h.surface.selectionMenuAt(ev) {
			t.Error("selectionMenuAt returned true when EventContextMenu is present")
		}
		h.textView.events &^= 1 << driver.EventContextMenu

		// Clearing the selection removes the selection menu.
		h.surface.clearSelection()
		if cleared := h.textView.native.Send(sel("menuForEvent:"), ev); cleared != 0 {
			t.Errorf("menuForEvent: with empty selection returned %v, want nil", cleared)
		}
	})
}

func TestSelectionMenuWhitespaceFallback(t *testing.T) {
	onMain(func() {
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))

		h := newSelectionHarness(t, "   ")
		defer h.cleanup()
		h.selectAll()

		ev := h.rightClickInSurface(apple.CGPoint{X: 25, Y: 28})
		menu := h.textView.native.Send(sel("menuForEvent:"), ev)
		if menu == 0 {
			t.Fatal("menuForEvent: over whitespace selection returned nil")
		}
		hasCopy := false
		count := objc.Send[int](menu, sel("numberOfItems"))
		for i := range count {
			item := menu.Send(sel("itemAtIndex:"), i)
			if objc.Send[bool](item, sel("isSeparatorItem")) {
				if i == 0 || i == count-1 {
					t.Errorf("edge separator at index %d of %d", i, count)
				}
				continue
			}
			if objc.Send[objc.SEL](item, sel("action")) == sel("copy:") {
				hasCopy = true
			}
		}
		if !hasCopy {
			t.Error("whitespace selection menu lacks Copy")
		}
	})
}

func TestSelectionServicesRequestorAndPasteboard(t *testing.T) {
	onMain(func() {
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))

		h := newSelectionHarness(t, "Bonjour le monde")
		defer h.cleanup()

		// Without a selection, no requestor is offered and Copy is disabled.
		if req := h.surface.native.Send(sel("validRequestorForSendType:returnType:"), apple.NSString(pasteboardText), objc.ID(0)); req != 0 {
			t.Fatalf("validRequestor without selection = %v, want nil", req)
		}
		copyItem := objc.ID(apple.Class("NSMenuItem")).Send(selAlloc).Send(
			sel("initWithTitle:action:keyEquivalent:"), apple.NSString("Copy"), sel("copy:"), apple.NSString(""))
		defer copyItem.Send(selRelease)
		if objc.Send[bool](h.surface.native, sel("validateMenuItem:"), copyItem) {
			t.Error("Copy validated true with no selection")
		}

		h.selectAll()
		if !objc.Send[bool](h.surface.native, sel("validateMenuItem:"), copyItem) {
			t.Error("Copy validated false with active selection")
		}

		// Both modern and legacy string pasteboard types resolve to the surface,
		// including when queried from a descendant view up the responder chain.
		for _, sendType := range []string{pasteboardText, pasteboardLegacyText} {
			if req := h.textView.native.Send(sel("validRequestorForSendType:returnType:"), apple.NSString(sendType), objc.ID(0)); req != h.surface.native {
				t.Errorf("validRequestor(%q, nil) from child = %v, want surface %v", sendType, req, h.surface.native)
			}
			// A return type implies editing/replacing text, which a read-only selection refuses.
			if req := h.surface.native.Send(sel("validRequestorForSendType:returnType:"), apple.NSString(sendType), apple.NSString(sendType)); req != 0 {
				t.Errorf("validRequestor(%q, %q) = %v, want nil", sendType, sendType, req)
			}
		}
		if req := h.surface.native.Send(sel("validRequestorForSendType:returnType:"), apple.NSString("public.png"), objc.ID(0)); req != 0 {
			t.Errorf("validRequestor(public.png, nil) = %v, want nil", req)
		}

		// writeSelectionToPasteboard:types: writes the selection for both string types.
		for _, requested := range []string{pasteboardText, pasteboardLegacyText} {
			pb := objc.ID(apple.Class("NSPasteboard")).Send(sel("pasteboardWithUniqueName"))
			types := objc.ID(apple.Class("NSArray")).Send(sel("arrayWithObject:"), apple.NSString(requested))
			if !objc.Send[bool](h.surface.native, sel("writeSelectionToPasteboard:types:"), pb, types) {
				t.Errorf("writeSelectionToPasteboard for %q returned false", requested)
			}
			if got := apple.GoString(pb.Send(sel("stringForType:"), apple.NSString(pasteboardText))); got != "Bonjour le monde" {
				t.Errorf("pasteboard (%q) UTF-8 string = %q, want %q", requested, got, "Bonjour le monde")
			}
			if got := apple.GoString(pb.Send(sel("stringForType:"), apple.NSString(pasteboardLegacyText))); got != "Bonjour le monde" {
				t.Errorf("pasteboard (%q) legacy string = %q, want %q", requested, got, "Bonjour le monde")
			}
			pb.Send(sel("releaseGlobally"))
		}

		// Non-text types are rejected.
		pb := objc.ID(apple.Class("NSPasteboard")).Send(sel("pasteboardWithUniqueName"))
		pngTypes := objc.ID(apple.Class("NSArray")).Send(sel("arrayWithObject:"), apple.NSString("public.png"))
		if objc.Send[bool](h.surface.native, sel("writeSelectionToPasteboard:types:"), pb, pngTypes) {
			t.Error("writeSelectionToPasteboard accepted public.png")
		}
		pb.Send(sel("releaseGlobally"))
	})
}

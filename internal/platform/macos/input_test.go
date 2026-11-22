//go:build darwin && !ios

package macos

import (
	"math"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// TestInputLayoutAfterUnmount lays out an input's native view, single-line and multiline, after the backend has removed it, as AppKit can for a view an outgoing transition still holds.
func TestInputLayoutAfterUnmount(t *testing.T) {
	onMain(func() {
		loadAppKit()
		s := &surface{views: make([]*view, 2), animator: driver.NewAnimator(func(driver.Event) {})}
		for _, multiline := range []bool{false, true} {
			v := s.create(driver.KindInput, 1)
			s.views[v.id] = v
			v.configure(&driver.InputConfig{Multiline: multiline, Content: driver.Bounds{Size: driver.Size{Width: 200, Height: 60}}})
			native := v.native.Send(sel("retain"))
			s.delete(v, false)
			if views[native] != nil {
				t.Error("the removed input is still registered")
			}
			native.Send(sel("layout"))
			native.Send(selRelease)
		}
	})
}

// TestMultilineEditorScrollsUnderPadding checks that a multiline input's
// scroll view fills the padding box and its text keeps the padding, uneven
// sides included (#701).
func TestMultilineEditorScrollsUnderPadding(t *testing.T) {
	onMain(func() {
		loadAppKit()
		s := &surface{views: make([]*view, 2), animator: driver.NewAnimator(func(driver.Event) {})}
		v := s.create(driver.KindInput, 1)
		s.views[v.id] = v
		v.native.Send(selSetFrame, apple.Rect(0, 0, 300, 200))
		pad := driver.Edges{Top: 18, Right: 20, Bottom: 20, Left: 12}
		v.configure(&driver.InputConfig{Multiline: true, Padding: pad,
			Content: driver.Bounds{Origin: driver.Point{X: 13, Y: 19}, Size: driver.Size{Width: 266, Height: 160}}})
		v.native.Send(sel("layout"))
		in := v.input
		if got, want := objc.Send[apple.CGRect](in.host, sel("frame")), apple.Rect(1, 1, 298, 198); got != want {
			t.Errorf("scroll view at %v, want the padding box %v", got, want)
		}
		if got := objc.Send[apple.CGPoint](in.field, sel("textContainerOrigin")); got != (apple.CGPoint{X: 12, Y: 18}) {
			t.Errorf("text starts at %v, want after the top and left padding", got)
		}
		in.host.Send(sel("tile"))
		visible := objc.Send[apple.CGSize](in.host, sel("contentSize")).Width
		wraps := objc.Send[apple.CGSize](in.field.Send(sel("textContainer")), sel("size")).Width
		if wraps != visible-32 {
			t.Errorf("text wraps at %v pt in a %v pt viewport, want the left and right padding off", wraps, visible)
		}
		s.delete(v, false)
	})
}

// TestMultilineEditorWrapsInItsViewport checks a multiline input's text
// view, its container and the text it lays out against the width the
// scroll view shows, as the input narrows and widens, with overlay and
// legacy scrollers (#42).
func TestMultilineEditorWrapsInItsViewport(t *testing.T) {
	onMain(func() { multilineEditorWraps(t) })
}

func multilineEditorWraps(t *testing.T) {
	loadAppKit()
	text := strings.Repeat("多行输入在可见区域内换行。", 20) + " https://example.com/" + strings.Repeat("a", 300)
	for _, style := range []int{1, 0} { // overlay, legacy
		window := objc.ID(apple.Class("NSWindow")).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
			apple.Rect(0, 0, 500, 200), uint(0), uint(2), false)
		window.Send(sel("setReleasedWhenClosed:"), false)
		scroll, tv := newMultilineEditor()
		window.Send(sel("contentView")).Send(selAddSubview, scroll)
		scroll.Send(sel("setScrollerStyle:"), style)
		tv.Send(sel("setString:"), apple.NSString(text))
		container := tv.Send(sel("textContainer"))
		layout := tv.Send(sel("layoutManager"))
		for _, width := range []float64{320, 185, 445, 60} {
			scroll.Send(sel("setFrame:"), apple.Rect(0, 0, width, 90))
			scroll.Send(sel("tile"))
			layout.Send(sel("ensureLayoutForTextContainer:"), container)
			visible := objc.Send[apple.CGSize](scroll, sel("contentSize")).Width
			editor := objc.Send[apple.CGRect](tv, sel("frame")).Size.Width
			wraps := objc.Send[apple.CGSize](container, sel("size")).Width
			used := objc.Send[apple.CGRect](layout, sel("usedRectForTextContainer:"), container)
			if editor != visible || wraps != visible || used.Origin.X+used.Size.Width > visible {
				t.Errorf("scroller style %d, %v pt wide: shows %v pt, editor %v pt, container %v pt, text to %v pt",
					style, width, visible, editor, wraps, used.Origin.X+used.Size.Width)
			}
		}
		scroll.Send(selRelease)
		window.Send(selRelease)
	}
}

// TestMultilineInputTakesTheLineHeight checks that a multiline input draws its lines at the style's line height, their baselines where a core.Text with the same style puts them, for a value set before and after the style and for its placeholder; that what is typed next takes it; and that dropping the line height keeps the alignment (#310).
func TestMultilineInputTakesTheLineHeight(t *testing.T) {
	onMain(func() {
		loadAppKit()
		s := &surface{views: make([]*view, 2), animator: driver.NewAnimator(func(driver.Event) {})}
		v := s.create(driver.KindInput, 1)
		s.views[v.id] = v
		defer s.delete(v, false)
		v.native.Send(selSetFrame, apple.Rect(0, 0, 300, 200))
		in := v.input
		const text = "HHHH\nHHHH\nHHHH"
		paragraph := textAttrs().paragraph
		lineHeights := func(id objc.ID) (lo, hi float64, align int) {
			if id == 0 {
				return 0, 0, -1
			}
			spacing := objc.Send[float64](id, sel("lineSpacing"))
			return objc.Send[float64](id, sel("minimumLineHeight")) + spacing,
				objc.Send[float64](id, sel("maximumLineHeight")) + spacing, objc.Send[int](id, sel("alignment"))
		}
		at := func(i int) objc.ID {
			return in.field.Send(selTextStorage).Send(sel("attribute:atIndex:effectiveRange:"), paragraph, uint(i), uintptr(0))
		}
		for _, lineHeight := range []driver.Points{25.5, 40, 15} {
			style := driver.TextStyle{Font: driver.Font{Size: 16, Weight: 400}, Color: driver.RGB(0), LineHeight: lineHeight, Align: driver.TextAlignCenter}
			config := driver.InputConfig{Multiline: true, Style: style, Placeholder: "HHHH", PlaceholderColor: driver.RGB(0),
				Content: driver.Bounds{Size: driver.Size{Width: 300, Height: 200}}}
			// Where core.Text puts the baselines of the same lines, from its top.
			p := apple.TextEngine{}.Prepare(&driver.AttributedText{Text: text, Style: style})
			l := p.Layout(p.Measure(300)).(*apple.Layout)
			var want []float64
			for _, line := range l.TextLines() {
				b := line.Bounds
				want = append(want, float64(b.Origin.Y+line.Ascender+(b.Size.Height-line.Ascender-line.Descender)/2))
			}
			l.Release()
			p.Release()
			check := func(when string, lines int) {
				v.native.Send(sel("layoutSubtreeIfNeeded"))
				got := baselines(v.native, 300, 200)
				if len(got) != lines {
					t.Errorf("%v pt lines, %s: baselines %v, want %d lines", lineHeight, when, got, lines)
					return
				}
				for k, b := range got {
					if math.Abs(b-want[k]) > 1 {
						t.Errorf("%v pt lines, %s: baselines %v, want core.Text's %v", lineHeight, when, got, want[:lines])
						break
					}
				}
			}
			in.replaceText(text)
			v.configure(&config)
			check("styled after the value", 3)
			in.replaceText("")
			check("the placeholder", 1)
			in.replaceText(text)
			check("value set after the style", 3)
			for _, i := range []int{0, 5, 10} {
				if lo, hi, align := lineHeights(at(i)); lo != float64(lineHeight) || hi != float64(lineHeight) || align != nsAlignCenter {
					t.Errorf("character %d's lines are %v to %v pt, aligned %d; want %v and %d", i, lo, hi, align, lineHeight, nsAlignCenter)
				}
			}
			if lo, hi, _ := lineHeights(in.field.Send(sel("typingAttributes")).Send(sel("objectForKey:"), paragraph)); lo != float64(lineHeight) || hi != float64(lineHeight) {
				t.Errorf("typed text gets lines %v to %v pt, want %v", lo, hi, lineHeight)
			}
			config.Style.LineHeight = 0
			v.configure(&config)
			if lo, _, align := lineHeights(at(0)); lo != 0 || align != nsAlignCenter {
				t.Errorf("without a line height the text's lines are at least %v pt, aligned %d; want 0 and centered", lo, align)
			}
			if in.lift != 0 {
				t.Errorf("without a line height the lines are lifted %v pt", in.lift)
			}
		}
	})
}

var multilineGeometryWindow = sync.OnceValue(func() objc.Class {
	return register("NsgoMultilineGeometryWindow", apple.Class("NSWindow"), nil, []objc.MethodDef{
		{Cmd: sel("isKeyWindow"), Fn: func(objc.ID, objc.SEL) bool { return true }},
	})
})

func TestMultilineInputEditingGeometryFitsTheFont(t *testing.T) {
	onMain(func() {
		loadAppKit()
		window := objc.ID(multilineGeometryWindow()).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
			apple.Rect(0, 0, 300, 120), uint(0), uint(2), false)
		window.Send(sel("setReleasedWhenClosed:"), false)
		defer window.Send(selRelease)
		s := &surface{views: make([]*view, 2), animator: driver.NewAnimator(func(driver.Event) {})}
		v := s.create(driver.KindInput, 1)
		s.views[v.id] = v
		defer s.delete(v, false)
		v.configure(&driver.InputConfig{Multiline: true})
		editor := v.input.field
		manager := sel("textLayoutManager")
		if !objc.Send[bool](editor, sel("respondsToSelector:"), manager) || editor.Send(manager) == 0 {
			t.Skip("requires a TextKit 2 editor")
		}
		window.Send(sel("contentView")).Send(selAddSubview, v.native)
		v.native.Send(selSetFrame, apple.Rect(0, 0, 300, 120))
		indicator := apple.Class("NSTextInsertionIndicator") // macOS 14
		for _, height := range []driver.Points{0, 22, 40} {
			v.configure(&driver.InputConfig{
				Multiline: true,
				Style:     driver.TextStyle{Font: driver.Font{Size: 14.5, Weight: 400}, LineHeight: height, Color: driver.RGB(0)},
				Padding:   driver.Edges{Top: 8},
				Content:   driver.Bounds{Origin: driver.Point{Y: 8}, Size: driver.Size{Width: 300, Height: 112}},
			})
			v.input.replaceText("HHHH\nHHHH\nHHHH")
			window.Send(sel("makeFirstResponder:"), editor)
			font := editor.Send(sel("font"))
			want := objc.Send[float64](font, sel("ascender")) - objc.Send[float64](font, sel("descender"))
			check := func(kind string, i uint, got float64) {
				t.Helper()
				if math.Abs(got-want) > 1 {
					t.Errorf("%v pt lines at character %d: %s is %.2f pt tall, want the font's %.2f pt", height, i, kind, got, want)
				}
			}
			for _, i := range []uint{1, 6, 11} {
				editor.Send(selSetSelRange, apple.NSRange{Location: i})
				v.native.Send(sel("layoutSubtreeIfNeeded"))
				ime := objc.Send[apple.CGRect](editor, sel("firstRectForCharacterRange:actualRange:"), apple.NSRange{Location: i, Length: 1}, uintptr(0))
				check("candidate anchor", i, ime.Size.Height)
				if indicator == 0 {
					continue
				}
				found := false
				for _, sub := range apple.Subviews(editor) {
					if objc.Send[bool](sub, sel("isKindOfClass:"), indicator) {
						check("insertion indicator", i, objc.Send[apple.CGRect](sub, sel("frame")).Size.Height)
						found = true
						break
					}
				}
				if !found {
					t.Errorf("no insertion indicator at character %d", i)
				}
			}
		}
	})
}

// baselines returns the baselines of the lines of dark ink v draws in a w by h box, from its top: the bottom of each band of rows with ink, which is where capitals stand.
func baselines(v objc.ID, w, h float64) []float64 {
	rep := v.Send(sel("bitmapImageRepForCachingDisplayInRect:"), apple.Rect(0, 0, w, h))
	v.Send(sel("cacheDisplayInRect:toBitmapImageRep:"), apple.Rect(0, 0, w, h), rep)
	pw, ph := objc.Send[int](rep, sel("pixelsWide")), objc.Send[int](rep, sel("pixelsHigh"))
	bpr, spp := objc.Send[int](rep, sel("bytesPerRow")), objc.Send[int](rep, sel("samplesPerPixel"))
	data := unsafe.Slice(objc.Send[*byte](rep, sel("bitmapData")), bpr*ph)
	inked := func(y int) bool {
		for x := range pw {
			if p := data[y*bpr+x*spp:]; p[3] > 128 && p[0] < 100 {
				return true
			}
		}
		return false
	}
	var out []float64
	for y := range ph {
		if inked(y) && (y+1 == ph || !inked(y+1)) {
			out = append(out, float64(y+1)*h/float64(ph))
		}
	}
	return out
}

// TestReadonlyInputSelects checks that an input turning read-only while it is edited stops taking edits at once, its field editor included, and that its text still selects, single-line and multiline.
func TestReadonlyInputSelects(t *testing.T) {
	onMain(func() {
		loadAppKit()
		window := objc.ID(apple.Class("NSWindow")).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
			apple.Rect(0, 0, 400, 100), uint(0), uint(2), false)
		window.Send(sel("setReleasedWhenClosed:"), false)
		defer window.Send(selRelease)
		for _, multiline := range []bool{false, true} {
			s := &surface{views: make([]*view, 2), animator: driver.NewAnimator(func(driver.Event) {})}
			v := s.create(driver.KindInput, 1)
			s.views[v.id] = v
			window.Send(sel("contentView")).Send(selAddSubview, v.native)
			v.native.Send(selSetFrame, apple.Rect(0, 0, 300, 60))
			c := driver.InputConfig{Multiline: multiline, Content: driver.Bounds{Size: driver.Size{Width: 300, Height: 60}}}
			v.configure(&c)
			v.input.focus()
			if v.input.editor() == 0 {
				t.Fatalf("multiline %v: no editor while focused", multiline)
			}
			c.Readonly = true
			v.configure(&c)
			e := v.input.editor()
			if objc.Send[bool](e, sel("isEditable")) || objc.Send[bool](v.input.field, sel("isEditable")) {
				t.Errorf("multiline %v: the input being edited still edits once read-only", multiline)
			}
			if !objc.Send[bool](e, sel("isSelectable")) || !objc.Send[bool](v.input.field, sel("isSelectable")) {
				t.Errorf("multiline %v: a read-only input's text does not select", multiline)
			}
			c.Readonly = false
			v.configure(&c)
			if e := v.input.editor(); e != 0 && !objc.Send[bool](e, sel("isEditable")) {
				t.Errorf("multiline %v: the input does not edit again once no longer read-only", multiline)
			}
			window.Send(sel("makeFirstResponder:"), objc.ID(0))
			s.delete(v, false)
		}
	})
}

// Replacing an editor after clipping was installed keeps both the editor and a multiline placeholder inside the holder.
func TestInputReplacementKeepsItsClipHolder(t *testing.T) {
	onMain(func() {
		loadAppKit()
		s := &surface{views: make([]*view, 2), animator: driver.NewAnimator(func(driver.Event) {})}
		v := s.create(driver.KindInput, 1)
		s.views[v.id] = v
		defer s.delete(v, false)
		v.native.Send(selSetFrame, apple.Rect(0, 0, 160, 80))
		v.style = driver.ViewStyle{Clip: true, CornerRadius: 24, ShadowColor: driver.RGB(0xff0000)}
		v.styleLayer(&v.style)
		holder := v.content
		if holder == v.native {
			t.Fatal("decorated input has no clip holder")
		}
		for _, mode := range [][2]bool{{true, false}, {false, false}, {false, true}, {true, false}} {
			v.setEditor(mode[0], mode[1])
			if got := v.input.host.Send(sel("superview")); got != holder {
				t.Errorf("editor multiline=%v masked=%v has parent %v, want holder %v", mode[0], mode[1], got, holder)
			}
			if label := v.input.label; label != 0 && label.Send(sel("superview")) != holder {
				t.Errorf("multiline placeholder escaped its clip holder")
			}
		}
	})
}

// TestInputCommitsMarkedTextOnFocusLoss checks that marked text an input method
// still composes (such as a whole message under live conversion) is unmarked,
// kept and reported before composition end when the input loses focus, hands
// off the shared field editor to another field, turns read-only or disabled,
// or its window resigns key, without firing spurious key presses (#1562).
func TestInputCommitsMarkedTextOnFocusLoss(t *testing.T) {
	onMain(func() {
		loadAppKit()
		window := objc.ID(apple.Class("NSWindow")).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
			apple.Rect(120, 140, 500, 240), uint(0), uint(2), false)
		window.Send(sel("setReleasedWhenClosed:"), false)
		defer window.Send(selRelease)
		const message = "今日はいい天気"
		markLen := uint(driver.UTF16Len(message))

		for _, multiline := range []bool{false, true} {
			for _, loss := range []string{"blur", "focus-other-input", "readonly", "disabled", "resign-key"} {
				s := &surface{window: window, views: make([]*view, 3), animator: driver.NewAnimator(func(driver.Event) {})}
				v1 := s.create(driver.KindInput, 1)
				s.views[v1.id] = v1
				v2 := s.create(driver.KindInput, 2)
				s.views[v2.id] = v2
				var evs []driver.Event
				v1.input.fs.Dispatch = func(ev driver.Event) { evs = append(evs, ev) }
				v1.input.fs.Events = driver.EventMask(^uint64(0))
				v2.input.fs.Dispatch = func(driver.Event) {}
				window.Send(sel("contentView")).Send(selAddSubview, v1.native)
				window.Send(sel("contentView")).Send(selAddSubview, v2.native)
				v1.native.Send(selSetFrame, apple.Rect(10, 10, 260, 40))
				v2.native.Send(selSetFrame, apple.Rect(10, 70, 260, 40))
				cfg := driver.InputConfig{Multiline: multiline, Content: driver.Bounds{Size: driver.Size{Width: 260, Height: 40}}}
				v1.configure(&cfg)
				v2.configure(&driver.InputConfig{Content: driver.Bounds{Size: driver.Size{Width: 260, Height: 40}}})
				v1.native.Send(sel("layoutSubtreeIfNeeded"))
				v2.native.Send(sel("layoutSubtreeIfNeeded"))
				v1.input.focus()
				e := v1.input.editor()
				if e == 0 {
					t.Fatalf("multiline=%v %s: no editor while focused", multiline, loss)
				}

				// Mark a full message with a mid-clause selection, as live conversion does.
				e.Send(sel("setMarkedText:selectedRange:replacementRange:"),
					apple.NSString(message), apple.NSRange{Location: 0, Length: 3}, apple.NSRange{Location: nsNotFound})
				if !objc.Send[bool](e, selHasMarkedText) {
					t.Fatalf("multiline=%v %s: editor has no marked text after setMarkedText", multiline, loss)
				}
				// Reset events and simulate a stale engine value so commitMarked must
				// dispatch the final committed EventChangeText before ending composition.
				v1.input.fs.Value("きょう", v1.input.fs.Count)
				v1.input.fs.Edited("きょう")
				evs = nil

				switch loss {
				case "blur":
					window.Send(sel("makeFirstResponder:"), objc.ID(0))
				case "focus-other-input":
					v2.input.focus()
				case "readonly":
					cfg.Readonly = true
					v1.configure(&cfg)
				case "disabled":
					cfg.Disabled = true
					v1.configure(&cfg)
				case "resign-key":
					s.commitMarked()
				}

				if got := v1.input.text(); got != message {
					t.Errorf("multiline=%v %s: text = %q, want %q", multiline, loss, got, message)
				}
				if !multiline {
					if got := apple.GoString(v1.input.field.Send(selStringValue)); got != message {
						t.Errorf("multiline=%v %s: field stringValue = %q, want %q", multiline, loss, got, message)
					}
				}
				if cur := v1.input.editor(); cur != 0 && objc.Send[bool](cur, selHasMarkedText) {
					t.Errorf("multiline=%v %s: marked text was not cleared", multiline, loss)
				}
				if loss == "resign-key" || loss == "readonly" {
					if got := objc.Send[apple.NSRange](e, selSelectedRange); got != (apple.NSRange{Location: markLen}) {
						t.Errorf("multiline=%v %s: selection after commit = %+v, want caret at end %d", multiline, loss, got, markLen)
					}
				}

				changeIdx, endIdx := -1, -1
				for i, ev := range evs {
					switch ev.Kind {
					case driver.EventChangeText:
						if ev.Text == message {
							changeIdx = i
						}
					case driver.EventCompositionChange:
						if ev.Finished {
							endIdx = i
						}
					case driver.EventKeyPress:
						t.Errorf("multiline=%v %s: spurious key press %q during commit", multiline, loss, ev.Text)
					}
				}
				if changeIdx < 0 {
					t.Errorf("multiline=%v %s: committed text %q was not reported (events: %+v)", multiline, loss, message, evs)
				}
				if endIdx < 0 {
					t.Errorf("multiline=%v %s: composition end was not reported (events: %+v)", multiline, loss, evs)
				}
				if changeIdx >= 0 && endIdx >= 0 && changeIdx >= endIdx {
					t.Errorf("multiline=%v %s: EventChangeText (%d) did not precede composition end (%d)", multiline, loss, changeIdx, endIdx)
				}

				window.Send(sel("makeFirstResponder:"), objc.ID(0))
				s.delete(v2, false)
				s.delete(v1, false)
			}
		}
	})
}

// TestPointWideSinkCandidateRect checks that firstRectForCharacterRange:actualRange:
// places macOS input-method candidates beside the caret of a point-wide editor sink
// (1pt or 0pt wide, as code, rich text and terminal editors use), including after
// scrolling or moving the sink, instead of returning (0, 0) at the screen's corner,
// while normal wide inputs keep tracking character positions (#1562).
func TestPointWideSinkCandidateRect(t *testing.T) {
	onMain(func() {
		loadAppKit()
		window := objc.ID(apple.Class("NSWindow")).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
			apple.Rect(220, 310, 600, 400), uint(0), uint(2), false)
		window.Send(sel("setReleasedWhenClosed:"), false)
		defer window.Send(selRelease)
		// Mount inside a flipped root view, as NSGo surfaces do.
		root := objc.ID(classView).Send(selAlloc).Send(sel("initWithFrame:"), apple.Rect(0, 0, 600, 400))
		window.Send(sel("setContentView:"), root)
		defer root.Send(selRelease)

		for _, tc := range []struct {
			name      string
			multiline bool
			width     driver.Points
		}{
			{"single-1pt", false, 1},
			{"single-0pt", false, 0},
			{"multi-1pt", true, 1},
			{"multi-0pt", true, 0},
		} {
			s := &surface{window: window, native: root, views: make([]*view, 2), animator: driver.NewAnimator(func(driver.Event) {})}
			v := s.create(driver.KindInput, 1)
			s.views[v.id] = v
			v.input.fs.Dispatch = func(driver.Event) {}
			root.Send(selAddSubview, v.native)
			v.native.Send(selSetFrame, apple.Rect(110, 90, float64(tc.width), 20))
			cfg := driver.InputConfig{
				Multiline: tc.multiline,
				Style:     driver.TextStyle{Font: driver.Font{Size: 14, Weight: 400}, Color: driver.RGB(0)},
				Content:   driver.Bounds{Size: driver.Size{Width: tc.width, Height: 20}},
			}
			v.configure(&cfg)
			v.native.Send(sel("layoutSubtreeIfNeeded"))
			v.input.focus()
			e := v.input.editor()
			if e == 0 {
				t.Fatalf("%s: no editor while focused", tc.name)
			}

			e.Send(sel("setMarkedText:selectedRange:replacementRange:"),
				apple.NSString("今日はいい天気ですね"), apple.NSRange{Location: 10, Length: 0}, apple.NSRange{Location: nsNotFound})
			e.Send(sel("scrollRangeToVisible:"), apple.NSRange{Location: 10, Length: 0})

			checkAt := func(when string, caretX, caretY float64) {
				v.native.Send(selSetFrame, apple.Rect(caretX, caretY, float64(tc.width), 20))
				v.native.Send(sel("layoutSubtreeIfNeeded"))
				winBox := objc.Send[apple.CGRect](v.native, sel("convertRect:toView:"),
					objc.Send[apple.CGRect](v.native, selBounds), objc.ID(0))
				wantScreen := objc.Send[apple.CGRect](window, sel("convertRectToScreen:"), winBox)
				for _, r := range []apple.NSRange{
					{Location: 0, Length: 0},
					{Location: 0, Length: 10},
					{Location: 5, Length: 5},
					{Location: 10, Length: 0},
				} {
					var actual apple.NSRange
					got := objc.Send[apple.CGRect](e, selFirstRectForRange, r, unsafe.Pointer(&actual))
					if got.Size.Height <= 0 {
						t.Errorf("%s %s range %+v: empty screen rect %+v (candidates would open at screen corner)", tc.name, when, r, got)
						continue
					}
					if math.Abs(got.Origin.X-wantScreen.Origin.X) > 4 || math.Abs(got.Origin.Y-wantScreen.Origin.Y) > 6 {
						t.Errorf("%s %s range %+v: screen rect %+v, want beside sink %+v", tc.name, when, r, got, wantScreen)
					}
					// Also verify nil actualRange works safely.
					gotNil := objc.Send[apple.CGRect](e, selFirstRectForRange, r, unsafe.Pointer(nil))
					if gotNil != got {
						t.Errorf("%s %s range %+v: nil actualRange gave %+v, want %+v", tc.name, when, r, gotNil, got)
					}
				}
			}

			checkAt("initial caret", 110, 90)
			checkAt("moved caret", 280, 210)

			window.Send(sel("makeFirstResponder:"), objc.ID(0))
			s.delete(v, false)
		}

		// Normal wide inputs keep tracking character positions across the line.
		for _, multiline := range []bool{false, true} {
			s := &surface{window: window, native: root, views: make([]*view, 2), animator: driver.NewAnimator(func(driver.Event) {})}
			v := s.create(driver.KindInput, 1)
			s.views[v.id] = v
			v.input.fs.Dispatch = func(driver.Event) {}
			root.Send(selAddSubview, v.native)
			v.native.Send(selSetFrame, apple.Rect(40, 40, 260, 40))
			v.configure(&driver.InputConfig{
				Multiline: multiline,
				Style:     driver.TextStyle{Font: driver.Font{Size: 14, Weight: 400}, Color: driver.RGB(0)},
				Content:   driver.Bounds{Size: driver.Size{Width: 260, Height: 40}},
			})
			v.native.Send(sel("layoutSubtreeIfNeeded"))
			v.input.focus()
			e := v.input.editor()
			e.Send(sel("setMarkedText:selectedRange:replacementRange:"),
				apple.NSString("今日はいい天気"), apple.NSRange{Location: 7, Length: 0}, apple.NSRange{Location: nsNotFound})
			start := objc.Send[apple.CGRect](e, selFirstRectForRange, apple.NSRange{Location: 0, Length: 0}, unsafe.Pointer(nil))
			end := objc.Send[apple.CGRect](e, selFirstRectForRange, apple.NSRange{Location: 7, Length: 0}, unsafe.Pointer(nil))
			if start.Size.Height <= 0 || end.Size.Height <= 0 || end.Origin.X <= start.Origin.X+40 {
				t.Errorf("wide multiline=%v: start=%+v end=%+v, want end.Origin.X well past start.Origin.X", multiline, start, end)
			}
			window.Send(sel("makeFirstResponder:"), objc.ID(0))
			s.delete(v, false)
		}
	})
}

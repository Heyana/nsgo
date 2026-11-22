//go:build linux && !android

package linux

import (
	"testing"
	"time"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/scrollbar"
)

var (
	gtkSwitchNew                func() uintptr
	gtkTextViewGetJustification func(uintptr) int32
	gtkLabelGetXalign           func(uintptr) float32
)

func init() {
	if openLibs() == nil {
		purego.RegisterLibFunc(&gtkSwitchNew, gtkLib, "gtk_switch_new")
		purego.RegisterLibFunc(&gtkTextViewGetJustification, gtkLib, "gtk_text_view_get_justification")
		purego.RegisterLibFunc(&gtkLabelGetXalign, gtkLib, "gtk_label_get_xalign")
	}
}

type directionSwitchComponent struct {
	widget uintptr
}

func (c *directionSwitchComponent) View() any         { return c.widget }
func (*directionSwitchComponent) SetProp(string, any) {}
func (*directionSwitchComponent) Command(string, any) {}
func (*directionSwitchComponent) Close()              {}

// TestDirectionMirrorsGTKControls opens a real GTK4 window with a menu bar,
// single-line GtkEntry, multiline GtkTextView with placeholder label, hosted
// GtkSwitch, vertical scroll view and popup menu, toggles between LTR ("en"),
// RTL ("ar-XB") and LTR, and verifies GTK widget directions, justification,
// alignment and scroll bar placement (#1615).
func TestDirectionMirrorsGTKControls(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	scopes := make(chan *core.Scope, 1)
	var sw *directionSwitchComponent
	app := func(cx *core.Scope) core.Node {
		scopes <- cx
		cx.SetApplicationMenu([]core.MenuItem{
			{Label: "File", Items: []core.MenuItem{{Label: "Open", OnSelect: func() {}}}},
			{Label: "Edit", Items: []core.MenuItem{{Label: "Copy", OnSelect: func() {}}}},
		})
		return core.View{
			Style: core.Style{Width: core.Pt(360), Height: core.Pt(260), Direction: core.Column, Gap: 8, Background: driver.RGB(0xffffff)},
			Children: []core.Node{
				core.Input{
					TestID:      "single",
					Placeholder: "Name",
					Style:       core.Style{Width: core.Pt(240), Height: core.Pt(32)},
				},
				core.Input{
					TestID:      "multi",
					Multiline:   true,
					Placeholder: "Notes",
					Style:       core.Style{Width: core.Pt(240), Height: core.Pt(60)},
				},
				core.Native{
					TestID:    "toggle",
					Component: "dirswitch",
					Style:     core.Style{Width: core.Pt(50), Height: core.Pt(30)},
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

	w := startTestWindow(t, app, Options{
		Title:  "nsgo-direction-linux",
		Width:  360,
		Height: 260,
		Components: driver.NativeComponents{
			"dirswitch": func(*driver.NativeContext) driver.NativeComponent {
				sw = &directionSwitchComponent{widget: gtkSwitchNew()}
				return sw
			},
		},
	})
	var origDefault int32
	w.onLoop(t, func() { origDefault = gtkWidgetGetDefaultDirection() })
	defer w.onLoop(t, func() { gtkWidgetSetDefaultDirection(origDefault) })

	cx := <-scopes
	wait := func(what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			var done bool
			w.onLoop(t, func() { done = ok() })
			if done {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", what)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	findView := func(testID string) *view {
		for _, v := range w.views {
			if v != nil && !v.deleted && v.props.TestID == testID {
				return v
			}
		}
		return nil
	}

	verifyPhase := func(phase string, rtl bool) {
		t.Helper()
		wantDir := gtkTextDir(rtl)
		wantJustify := int32(0)
		wantXalign := float32(0)
		if rtl {
			wantJustify = 1
			wantXalign = 1
		}
		wait(phase+" layout and direction", func() bool {
			if w.dir != wantDir || w.menuBar == 0 || sw == nil {
				return false
			}
			sv, mv, sc := findView("single"), findView("multi"), findView("scroller")
			return sv != nil && sv.entry != 0 && sv.input.Style.RTL == rtl &&
				mv != nil && mv.textView != 0 && mv.hint != 0 && mv.input.Style.RTL == rtl &&
				sc != nil && sc.frame.Size.Width > 0
		})
		w.onLoop(t, func() {
			if got := gtkWidgetGetDefaultDirection(); got != wantDir {
				t.Errorf("%s: gtk_widget_get_default_direction = %d, want %d", phase, got, wantDir)
			}
			if got := gtkWidgetGetDirection(w.surface); got != wantDir {
				t.Errorf("%s: surface direction = %d, want %d", phase, got, wantDir)
			}
			if got := gtkWidgetGetDirection(w.menuBar); got != wantDir {
				t.Errorf("%s: menuBar direction = %d, want %d", phase, got, wantDir)
			}
			if got := gtkWidgetGetDirection(sw.widget); got != wantDir {
				t.Errorf("%s: GtkSwitch direction = %d, want %d", phase, got, wantDir)
			}
			sv := findView("single")
			if got := gtkWidgetGetDirection(sv.entry); got != wantDir {
				t.Errorf("%s: GtkEntry direction = %d, want %d", phase, got, wantDir)
			}
			if del := gtkEditableGetDelegate(sv.entry); del != 0 {
				if got := gtkWidgetGetDirection(del); got != wantDir {
					t.Errorf("%s: GtkEntry delegate direction = %d, want %d", phase, got, wantDir)
				}
			}
			mv := findView("multi")
			if got := gtkWidgetGetDirection(mv.entry); got != wantDir {
				t.Errorf("%s: GtkScrolledWindow direction = %d, want %d", phase, got, wantDir)
			}
			if got := gtkWidgetGetDirection(mv.textView); got != wantDir {
				t.Errorf("%s: GtkTextView direction = %d, want %d", phase, got, wantDir)
			}
			if got := gtkWidgetGetDirection(mv.hint); got != wantDir {
				t.Errorf("%s: placeholder GtkLabel direction = %d, want %d", phase, got, wantDir)
			}
			if got := gtkTextViewGetJustification(mv.textView); got != wantJustify {
				t.Errorf("%s: GtkTextView justification = %d, want %d", phase, got, wantJustify)
			}
			if got := gtkLabelGetXalign(mv.hint); got != wantXalign {
				t.Errorf("%s: placeholder GtkLabel xalign = %v, want %v", phase, got, wantXalign)
			}
			sc := findView("scroller")
			thumb, ok := sc.bar().Thumb()
			if !ok {
				t.Errorf("%s: scroller has no vertical thumb", phase)
			} else if rtl && thumb.Origin.X != scrollbar.Inset {
				t.Errorf("%s: RTL scroll thumb X = %v, want %v", phase, thumb.Origin.X, scrollbar.Inset)
			} else if !rtl {
				wantX := sc.frame.Size.Width - scrollbar.Inset - scrollbar.Thickness
				if thumb.Origin.X != wantX {
					t.Errorf("%s: LTR scroll thumb X = %v, want %v", phase, thumb.Origin.X, wantX)
				}
			}
		})

		// Verify context menu popover receives the direction too.
		cx.Post(func() {
			cx.PopupMenu([]core.MenuItem{{Label: "Copy", OnSelect: func() {}}}, core.Point{X: 20, Y: 20}, nil)
		})
		wait(phase+" popover menu", func() bool { return w.popover != 0 })
		w.onLoop(t, func() {
			if got := gtkWidgetGetDirection(w.popover); got != wantDir {
				t.Errorf("%s: GtkPopoverMenu direction = %d, want %d", phase, got, wantDir)
			}
			gtkPopoverPopdown(w.popover)
		})
		wait(phase+" popover close", func() bool { return w.popover == 0 })
	}

	cx.Post(func() { cx.SetLocale("en") })
	verifyPhase("LTR initial", false)

	cx.Post(func() { cx.SetLocale("ar-XB") })
	verifyPhase("RTL", true)

	cx.Post(func() { cx.SetLocale("en") })
	verifyPhase("LTR restored", false)
}

// TestEmbeddedDirectionMirrorsOwnWidgetsOnly verifies that an embedded GTK4
// surface mirrors its own descendant widgets (entry, text view, hosted switch)
// to RTL without modifying GTK's default direction or the host's widgets (#1615).
func TestEmbeddedDirectionMirrorsOwnWidgetsOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	host := startTestWindow(t, func(*core.Scope) core.Node { return core.View{} }, Options{Width: 300, Height: 240})
	var origDefault int32
	var hostSibling uintptr
	host.onLoop(t, func() {
		origDefault = gtkWidgetGetDefaultDirection()
		gtkWidgetSetDefaultDirection(gtkTextDirLTR)
		hostSibling = gtkSwitchNew()
		gObjectRefSink(hostSibling)
	})
	defer host.onLoop(t, func() {
		if hostSibling != 0 {
			gObjectUnref(hostSibling)
		}
		gtkWidgetSetDefaultDirection(origDefault)
	})

	embScopes := make(chan *core.Scope, 1)
	var embedded *View
	var embSwitch *directionSwitchComponent
	host.onLoop(t, func() {
		var err error
		embedded, err = NewView(func(cx *core.Scope) core.Node {
			embScopes <- cx
			return core.View{
				Style: core.Style{Width: core.Pt(240), Height: core.Pt(180), Direction: core.Column},
				Children: []core.Node{
					core.Input{TestID: "emb-field", Placeholder: "Field", Style: core.Style{Width: core.Pt(200), Height: core.Pt(32)}},
					core.Input{TestID: "emb-area", Multiline: true, Placeholder: "Area", Style: core.Style{Width: core.Pt(200), Height: core.Pt(50)}},
					core.Native{TestID: "emb-switch", Component: "embswitch", Style: core.Style{Width: core.Pt(50), Height: core.Pt(30)}},
				},
			}
		}, Options{
			Components: driver.NativeComponents{
				"embswitch": func(*driver.NativeContext) driver.NativeComponent {
					embSwitch = &directionSwitchComponent{widget: gtkSwitchNew()}
					return embSwitch
				},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		gtkBoxAppend(host.box, embedded.Handle())
	})
	defer host.onLoop(t, func() {
		if embedded != nil {
			gtkBoxRemove(host.box, embedded.Handle())
			embedded.Close()
		}
	})

	embCX := <-embScopes
	embCX.Post(func() { embCX.SetLocale("ar-XB") })

	deadline := time.Now().Add(5 * time.Second)
	for {
		var ready bool
		host.onLoop(t, func() {
			ew := embedded.w
			if ew.dir != gtkTextDirRTL || embSwitch == nil {
				return
			}
			var field, area *view
			for _, v := range ew.views {
				if v != nil && !v.deleted {
					switch v.props.TestID {
					case "emb-field":
						field = v
					case "emb-area":
						area = v
					}
				}
			}
			ready = field != nil && field.entry != 0 && field.input.Style.RTL &&
				area != nil && area.textView != 0 && area.input.Style.RTL
		})
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for embedded RTL controls")
		}
		time.Sleep(20 * time.Millisecond)
	}

	host.onLoop(t, func() {
		if got := gtkWidgetGetDefaultDirection(); got != gtkTextDirLTR {
			t.Errorf("embedded RTL changed GTK default direction to %d, want %d", got, gtkTextDirLTR)
		}
		if got := gtkWidgetGetDirection(hostSibling); got != gtkTextDirLTR {
			t.Errorf("embedded RTL changed host sibling direction to %d, want %d", got, gtkTextDirLTR)
		}
		ew := embedded.w
		if got := gtkWidgetGetDirection(ew.surface); got != gtkTextDirRTL {
			t.Errorf("embedded surface direction = %d, want %d", got, gtkTextDirRTL)
		}
		if got := gtkWidgetGetDirection(embSwitch.widget); got != gtkTextDirRTL {
			t.Errorf("embedded GtkSwitch direction = %d, want %d", got, gtkTextDirRTL)
		}
		for _, v := range ew.views {
			if v == nil || v.deleted {
				continue
			}
			if v.props.TestID == "emb-field" {
				if got := gtkWidgetGetDirection(v.entry); got != gtkTextDirRTL {
					t.Errorf("embedded GtkEntry direction = %d, want %d", got, gtkTextDirRTL)
				}
				if del := gtkEditableGetDelegate(v.entry); del != 0 {
					if got := gtkWidgetGetDirection(del); got != gtkTextDirRTL {
						t.Errorf("embedded GtkEntry delegate direction = %d, want %d", got, gtkTextDirRTL)
					}
				}
			}
			if v.props.TestID == "emb-area" {
				if got := gtkWidgetGetDirection(v.textView); got != gtkTextDirRTL {
					t.Errorf("embedded GtkTextView direction = %d, want %d", got, gtkTextDirRTL)
				}
				if got := gtkWidgetGetDirection(v.hint); got != gtkTextDirRTL {
					t.Errorf("embedded placeholder direction = %d, want %d", got, gtkTextDirRTL)
				}
			}
		}
	})
}

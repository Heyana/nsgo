//go:build linux && !android

package linux

import (
	"testing"
	"time"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestTransparentWindowLook opens a window transparent and off the task list: it is undecorated without the theme's background, and its environment says transparency shows where the display composites with alpha and the task list hint exists where GDK draws on X11. It reports WindowTransparent where the display composites. Made framed again it has both back and reports it is not transparent.
func TestTransparentWindowLook(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	var (
		hasCSSClass  func(widget uintptr, class *byte) int32
		getDecorated func(window uintptr) int32
	)
	purego.RegisterLibFunc(&hasCSSClass, gtkLib, "gtk_widget_has_css_class")
	purego.RegisterLibFunc(&getDecorated, gtkLib, "gtk_window_get_decorated")
	app := func(cx *core.Scope) core.Node { return core.View{} }
	w := startTestWindow(t, app, Options{Title: "nsgo-transparent", Width: 160, Height: 100, Transparent: true, SkipTaskbar: true})
	w.onLoop(t, func() {
		if hasCSSClass(w.win, cstr("nsgo-transparent")) == 0 || hasCSSClass(w.win, cstr("background")) != 0 || getDecorated(w.win) != 0 {
			t.Error("a transparent window keeps its decorations or its theme's background")
		}
		env := w.environment()
		if env.CanMakeWindowsTransparent != displayComposites() || env.CanSkipTaskbar != skipTaskbarHint() {
			t.Errorf("the environment reports transparency %v and the task list %v; the display composites %v and has the hint %v",
				env.CanMakeWindowsTransparent, env.CanSkipTaskbar, displayComposites(), skipTaskbarHint())
		}
		if env.WindowTransparent != displayComposites() {
			t.Errorf("a transparent window reports WindowTransparent %v where the display composites %v", env.WindowTransparent, displayComposites())
		}
		w.setTransparent(false)
		if w.environment().WindowTransparent {
			t.Error("a window made framed again reports WindowTransparent")
		}
		if hasCSSClass(w.win, cstr("nsgo-transparent")) != 0 || hasCSSClass(w.win, cstr("background")) == 0 || getDecorated(w.win) == 0 {
			t.Error("a window made framed again lacks its decorations or its theme's background")
		}
	})
}

// TestTransparentWindowNativePixels renders the complete GtkWindow, theme and decorations included, and checks that it keeps the clear area around an antialiased view instead of painting a rectangular box.
func TestTransparentWindowNativePixels(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	w := startTestWindow(t, func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(160), Height: core.Pt(100)}, Children: []core.Node{
			core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(20), Top: core.Pt(20),
				Width: core.Pt(60), Height: core.Pt(60), CornerRadius: 30, Background: core.RGB(0xff0000)}},
		}}
	}, Options{Title: "nsgo-transparent-pixels", Width: 160, Height: 100, Transparent: true})
	var composited bool
	w.onLoop(t, func() { composited = displayComposites() })
	if !composited {
		t.Skip("the display has no composited alpha channel")
	}
	var clear, edge, red int
	for deadline := time.Now().Add(5 * time.Second); red == 0; time.Sleep(10 * time.Millisecond) {
		w.onLoop(t, func() {
			width, height := gtkWidgetGetWidth(w.win), gtkWidgetGetHeight(w.win)
			if width <= 0 || height <= 0 {
				return
			}
			paintable := gtkWidgetPaintableNew(w.win)
			defer gObjectUnref(paintable)
			snapshot := gtkSnapshotNew()
			gdkPaintableSnapshot(paintable, snapshot, float64(width), float64(height))
			node := gtkSnapshotFreeToNode(snapshot)
			if node == 0 {
				return
			}
			defer gskRenderNodeUnref(node)
			surface := cairoImageSurfaceCreate(0, width, height)
			defer cairoSurfaceDestroy(surface)
			cr := cairoCreate(surface)
			defer cairoDestroy(cr)
			gskRenderNodeDraw(node, cr)
			cairoSurfaceFlush(surface)
			data, stride := cairoImageSurfaceGetData(surface), int(cairoImageSurfaceGetStride(surface))
			clear, edge, red = 0, 0, 0
			for y := range int(height) {
				for x := range int(width) {
					p := pixelAt(data, stride, x, y)
					switch {
					case p[3] == 0:
						clear++
					case p[3] < 255 && p[2] > 0 && p[1] == 0 && p[0] == 0:
						edge++
					case p[2] == 255 && p[1] == 0 && p[0] == 0:
						red++
					}
				}
			}
		})
		if time.Now().After(deadline) {
			t.Fatal("the native window did not paint the view")
		}
	}
	if clear < 500 || edge == 0 || red < 500 {
		t.Errorf("native window pixels: %d clear, %d antialiased, %d red", clear, edge, red)
	}
}

// TestWindowShadowKeepsServerDecorations checks that NoShadow only clears the shadow GTK draws: a window the window manager decorates keeps its frame, getting no GTK title bar and staying shown with its editor's focus, opened shadowless or changed.
func TestWindowShadowKeepsServerDecorations(t *testing.T) {
	if testing.Short() {
		t.Skip("opens windows")
	}
	for _, initial := range []bool{false, true} {
		name := "changed"
		if initial {
			name = "opened"
		}
		t.Run(name, func(t *testing.T) {
			app := func(*core.Scope) core.Node {
				return core.Input{DefaultValue: "retained", AutoFocus: true,
					Style: core.Style{Width: core.Pt(140), Height: core.Pt(30)}}
			}
			w := startTestWindow(t, app, Options{Title: "nsgo-shadow-" + name, Width: 180, Height: 100, NoShadow: initial})
			var focus uintptr
			for deadline := time.Now().Add(5 * time.Second); focus == 0; time.Sleep(10 * time.Millisecond) {
				w.onLoop(t, func() { focus = gtkWindowGetFocus(w.win) })
				if time.Now().After(deadline) {
					t.Fatal("the editor did not get focus")
				}
			}
			w.onLoop(t, func() {
				var hasClass func(uintptr, *byte) int32
				var getTitlebar func(uintptr) uintptr
				purego.RegisterLibFunc(&hasClass, gtkLib, "gtk_widget_has_css_class")
				purego.RegisterLibFunc(&getTitlebar, gtkLib, "gtk_window_get_titlebar")
				csd := hasClass(w.win, cstr("csd")) != 0 || hasClass(w.win, cstr("solid-csd")) != 0
				header := getTitlebar(w.win)
				if !initial {
					w.windowCommand(&driver.WindowCommand{Action: driver.WindowSetShadow, On: false})
				}
				if hasClass(w.win, cstr("nsgo-no-shadow")) == 0 {
					t.Error("a shadowless window has no rule clearing GTK's shadow")
				}
				if getTitlebar(w.win) != header || (hasClass(w.win, cstr("csd")) != 0 || hasClass(w.win, cstr("solid-csd")) != 0) != csd {
					t.Errorf("NoShadow changed the window's decorations (client-side before: %v)", csd)
				}
				if gtkWidgetGetVisible(w.win) == 0 || gtkWindowGetFocus(w.win) != focus {
					t.Error("changing the shadow hid the window or moved the editor focus")
				}
				w.windowCommand(&driver.WindowCommand{Action: driver.WindowSetShadow, On: true})
				if hasClass(w.win, cstr("nsgo-no-shadow")) != 0 || getTitlebar(w.win) != header {
					t.Error("restoring the shadow kept the shadow rule or changed the title bar")
				}
			})
		})
	}
}

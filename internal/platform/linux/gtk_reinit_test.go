//go:build linux && !android

package linux

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/core"
)

// Each scenario needs a fresh native process: GTK deliberately keeps its initialization state for the process's lifetime, including after failure.
func TestGTKInitFailureDoesNotPoisonLaterStarts(t *testing.T) {
	if scenario := os.Getenv("NSGO_GTK_REINIT_CHILD"); scenario != "" {
		os.Unsetenv("DISPLAY")
		os.Unsetenv("WAYLAND_DISPLAY")
		os.Setenv("GDK_BACKEND", "x11")
		for _, kind := range strings.Split(scenario, ",") {
			var err error
			app := func(*core.Scope) core.Node { return core.View{} }
			switch kind {
			case "start":
				var w *window
				w, err = start(app, Options{Title: "no-display"})
				if w != nil {
					t.Fatal("start returned a window without a display")
				}
			case "view":
				var view *View
				view, err = NewView(app, Options{})
				if view != nil {
					t.Fatal("NewView returned a widget without a display")
				}
			default:
				t.Fatalf("unknown scenario %q", kind)
			}
			if err == nil || !strings.Contains(err.Error(), "no display") {
				t.Fatalf("%s error=%v, want no display", kind, err)
			}
		}
		return
	}
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	for _, scenario := range []string{"start,start", "view,view", "start,view", "view,start"} {
		t.Run(scenario, func(t *testing.T) {
			runGTKInitChild(t, "TestGTKInitFailureDoesNotPoisonLaterStarts", "NSGO_GTK_REINIT_CHILD="+scenario)
		})
	}
}

func runGTKInitChild(t *testing.T, name, env string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+name+"$", "-test.v")
	cmd.Env = append(os.Environ(), env)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native GTK child failed: %v\n%s", err, out)
	}
}

func TestGTKDisplayCanBeSuppliedAfterInitializationFailure(t *testing.T) {
	if os.Getenv("NSGO_GTK_REINIT_DISPLAY_CHILD") == "1" {
		displayName := os.Getenv("DISPLAY")
		os.Unsetenv("DISPLAY")
		os.Unsetenv("WAYLAND_DISPLAY")
		os.Setenv("GDK_BACKEND", "x11")
		app := func(*core.Scope) core.Node { return core.View{} }
		if _, err := NewView(app, Options{}); err == nil || !strings.Contains(err.Error(), "no display") {
			t.Fatalf("initial NewView error=%v, want no display", err)
		}
		// An embedding host owns GTK's display. Supplying one explicitly after its initial failure must not be rejected by a cached error.
		var openDisplay func(*byte) uintptr
		var manager func() uintptr
		var setDefault func(uintptr, uintptr)
		purego.RegisterLibFunc(&openDisplay, gtkLib, "gdk_display_open")
		purego.RegisterLibFunc(&manager, gtkLib, "gdk_display_manager_get")
		purego.RegisterLibFunc(&setDefault, gtkLib, "gdk_display_manager_set_default_display")
		display := openDisplay(cstr(displayName))
		if display == 0 {
			t.Fatalf("host could not open %q", displayName)
		}
		setDefault(manager(), display)
		view, err := NewView(app, Options{})
		if err != nil {
			t.Fatalf("NewView with the host's display: %v", err)
		}
		if view.Handle() == 0 {
			t.Fatal("NewView made no native widget")
		}
		view.Close()
		return
	}
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	if os.Getenv("DISPLAY") == "" {
		t.Skip("needs an X11 display for host recovery")
	}
	runGTKInitChild(t, "TestGTKDisplayCanBeSuppliedAfterInitializationFailure", "NSGO_GTK_REINIT_DISPLAY_CHILD=1")
}

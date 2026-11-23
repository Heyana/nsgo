package keys_test

import (
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// positionRecorder records the keys a focused view hears go down and come up, with their positions, and shows core.KeyCodesDown, core.KeysDown, core.KeyCodeHeld of ShiftRight and core.KeyCodeLabel of KeyW, KeyY and KeyZ.
type positionRecorder struct {
	mu         sync.Mutex
	downs, ups []core.KeyEvent
}

func (r *positionRecorder) take() (downs, ups []core.KeyEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	downs, ups, r.downs, r.ups = r.downs, r.ups, nil, nil
	return downs, ups
}

func startPositions(t *testing.T) (*nsgotest.App, *positionRecorder) {
	t.Helper()
	r := new(positionRecorder)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Padding: core.All(20)}, Children: []core.Node{
			core.View{
				TestID: "recorder", Focusable: func() bool { return true },
				OnKeyDown: func(e core.KeyEvent) bool { r.mu.Lock(); r.downs = append(r.downs, e); r.mu.Unlock(); return false },
				OnKeyUp:   func(e core.KeyEvent) { r.mu.Lock(); r.ups = append(r.ups, e); r.mu.Unlock() },
				Style:     core.Style{Width: core.Pt(200), Height: core.Pt(40)},
			},
			core.Text{TestID: "codes", ContentFunc: func() string { return strings.Join(core.KeyCodesDown(cx), " ") }},
			core.Text{TestID: "keys", ContentFunc: func() string { return strings.Join(core.KeysDown(cx), " ") }},
			core.Text{TestID: "shift", ContentFunc: func() string { return strconv.FormatBool(core.KeyCodeHeld(cx, "ShiftRight")) }},
			core.Text{TestID: "labels", ContentFunc: func() string {
				return core.KeyCodeLabel(cx, "KeyW") + " " + core.KeyCodeLabel(cx, "KeyY") + " " + core.KeyCodeLabel(cx, "KeyZ")
			}},
		}}
	}, nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}), nsgotest.Size(300, 160))
	app.Focus("id:recorder")
	return app, r
}

func shows(t *testing.T, app *nsgotest.App, id, want string) {
	t.Helper()
	if got := app.Find("id:" + id).Text; got != want {
		t.Fatalf("%s reads %q, want %q", id, got, want)
	}
}

// A key pressed by its physical position reports the position beside the name the keyboard layout gives it, and core.KeyCodesDown, core.KeyCodeHeld and core.KeyCodeLabel follow it, a modifier's side included (#1242).
func TestKeysReportTheirPhysicalPosition(t *testing.T) {
	switch nsgotest.Target() {
	case "", "macos", "windows", "linux", "web":
	default:
		t.Skip("no focused view hears keys go down here")
	}
	app, r := startPositions(t)
	app.KeyCodeDown("KeyW")
	downs, _ := r.take()
	if len(downs) != 1 || downs[0].Code != "KeyW" || downs[0].Key == "" || downs[0].Modifiers != (core.Modifiers{}) {
		t.Fatalf("KeyW went down as %+v, want one down at KeyW with the layout's name", downs)
	}
	w := downs[0].Key
	shows(t, app, "codes", "KeyW")
	shows(t, app, "keys", w)
	if label := strings.Fields(app.Find("id:labels").Text); len(label) == 0 || utf8.RuneCountInString(w) == 1 && label[0] != strings.ToUpper(w) {
		t.Fatalf("core.KeyCodeLabel of KeyW reads %q, want %q", label, strings.ToUpper(w))
	}
	app.KeyCodeDown("ShiftRight")
	shows(t, app, "codes", "KeyW ShiftRight")
	shows(t, app, "shift", "true")
	app.KeyCodeUp("ShiftRight")
	shows(t, app, "shift", "false")
	app.KeyCodeUp("KeyW")
	shows(t, app, "codes", "")
	downs, ups := r.take()
	if want := []core.KeyEvent{{Key: "shift", Code: "ShiftRight", Modifiers: core.Modifiers{Shift: true}}}; !slices.Equal(downs, want) {
		t.Fatalf("ShiftRight went down as %+v, want %+v", downs, want)
	}
	if want := []core.KeyEvent{{Key: "shift", Code: "ShiftRight"}, {Key: w, Code: "KeyW"}}; !slices.Equal(ups, want) {
		t.Fatalf("the keys came up as %+v, want %+v", ups, want)
	}
	app.KeyCode("ArrowLeft")
	downs, ups = r.take()
	if want := []core.KeyEvent{{Key: "left", Code: "ArrowLeft"}}; !slices.Equal(downs, want) || !slices.Equal(ups, want) {
		t.Fatalf("ArrowLeft went down as %+v and came up as %+v, want %+v", downs, ups, want)
	}
}

// On iOS, where no view hears keys go down, a key pressed by its physical position still reaches core.KeyCodesDown, core.KeyCodeHeld and core.KeyCodeLabel, a modifier's side included (#1242): the simulator's HID keyboard presses the position, and a device's host reports it with the key.
func TestKeyCodesFollowPositionsOnIOS(t *testing.T) {
	if target := nsgotest.Target(); target != "ios" && target != "ios_simulator" {
		t.Skip("checks UIKit's hardware keys")
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Padding: core.All(20)}, Children: []core.Node{
			core.Text{TestID: "codes", ContentFunc: func() string { return strings.Join(core.KeyCodesDown(cx), " ") }},
			core.Text{TestID: "keys", ContentFunc: func() string { return strings.Join(core.KeysDown(cx), " ") }},
			core.Text{TestID: "shift", ContentFunc: func() string { return strconv.FormatBool(core.KeyCodeHeld(cx, "ShiftRight")) }},
			core.Text{TestID: "labels", ContentFunc: func() string { return core.KeyCodeLabel(cx, "KeyW") }},
		}}
	}, nsgotest.Size(300, 160))
	app.KeyCodeDown("KeyW")
	shows(t, app, "codes", "KeyW")
	w := app.Find("id:keys").Text
	if w == "" {
		t.Fatal("core.KeysDown reads nothing with KeyW held, want the layout's name of the key")
	}
	if utf8.RuneCountInString(w) == 1 {
		shows(t, app, "labels", strings.ToUpper(w))
	}
	app.KeyCodeDown("ShiftRight")
	shows(t, app, "codes", "KeyW ShiftRight")
	shows(t, app, "shift", "true")
	app.KeyCodeUp("ShiftRight")
	shows(t, app, "shift", "false")
	app.KeyCodeUp("KeyW")
	shows(t, app, "codes", "")
}

// A modifier held both by its name and by a position stays held until both let it go, whichever comes up first (#1242).
func TestModifierHeldByNameAndByPosition(t *testing.T) {
	switch nsgotest.Target() {
	case "", "macos", "windows", "linux", "web":
	default:
		t.Skip("no focused view hears keys go down here")
	}
	app, r := startPositions(t)
	shifted := func(when string, want bool) {
		t.Helper()
		r.take()
		app.KeyCode("ArrowLeft")
		downs, _ := r.take()
		if len(downs) != 1 || downs[0].Code != "ArrowLeft" || downs[0].Modifiers.Shift != want {
			t.Fatalf("%s, ArrowLeft went down as %+v, want Shift %v", when, downs, want)
		}
	}
	app.KeyDown("shift")
	app.KeyCodeDown("ShiftRight")
	app.KeyCodeUp("ShiftRight")
	shifted("with shift held and ShiftRight up", true)
	app.KeyCodeDown("ShiftRight")
	app.KeyUp("shift")
	shifted("with ShiftRight held and shift up", true)
	app.KeyCodeUp("ShiftRight")
	shifted("with both up", false)
}

// On an AZERTY and a QWERTZ layout, the key in QWERTY's W position reports KeyW while its name and label follow the layout (#1242). It sets the X server's layout with setxkbmap, so it runs only where NSGO_TEST_XKB_LAYOUTS opts in, as on CI's own Xvfb display, never on a desktop someone types on.
func TestKeyPositionsFollowTheXKBLayout(t *testing.T) {
	if nsgotest.Target() != "linux" || os.Getenv("NSGO_TEST_XKB_LAYOUTS") == "" {
		t.Skip("sets the X server's keyboard layout: set NSGO_TEST_XKB_LAYOUTS under nsgo test -target linux on a display of the test's own")
	}
	if _, err := exec.LookPath("setxkbmap"); err != nil {
		t.Skip("setxkbmap is not installed")
	}
	t.Cleanup(func() { setLayout(t, "us") })
	// GDK follows the X server's keymap changes only once it has read the keymap, so an app of the test's own presses a key before the first change.
	t.Run("us", func(t *testing.T) {
		probe, _ := startPositions(t)
		probe.KeyCode("KeyW")
	})
	for _, c := range []struct {
		layout string
		down   []string
		want   []string
		labels string
	}{
		{"fr", []string{"KeyW", "KeyQ"}, []string{"z", "a"}, "Z Y W"},
		{"de", []string{"KeyY", "KeyZ", "KeyW"}, []string{"z", "y", "w"}, "W Z Y"},
	} {
		t.Run(c.layout, func(t *testing.T) {
			setLayout(t, c.layout)
			// GDK reads the X server's new keymap when the window's loop handles its notice: an app of the test's own waits for its key to follow the layout before the one checked starts, and reads its names.
			if !t.Run("keymap", func(t *testing.T) {
				probe, r := startPositions(t)
				probe.WaitUntil(10*time.Second, func() bool {
					probe.KeyCode(c.down[0])
					downs, _ := r.take()
					return len(downs) == 1 && downs[0].Key == c.want[0]
				})
			}) {
				t.FailNow()
			}
			app, r := startPositions(t)
			for i, code := range c.down {
				app.KeyCode(code)
				downs, ups := r.take()
				want := []core.KeyEvent{{Key: c.want[i], Code: code}}
				if !slices.Equal(downs, want) || !slices.Equal(ups, want) {
					t.Fatalf("%s on %s went down as %+v and came up as %+v, want %+v", code, c.layout, downs, ups, want)
				}
			}
			app.KeyCodeDown(c.down[0])
			shows(t, app, "codes", c.down[0])
			shows(t, app, "keys", c.want[0])
			shows(t, app, "labels", c.labels)
			app.KeyCodeUp(c.down[0])
		})
	}
}

// setLayout has the X server use an XKB layout.
func setLayout(t *testing.T, layout string) {
	t.Helper()
	if out, err := exec.Command("setxkbmap", "-layout", layout).CombinedOutput(); err != nil {
		t.Fatalf("setxkbmap -layout %s: %v: %s", layout, err, out)
	}
}

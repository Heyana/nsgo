package nsgotest

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// closingDialog is a dialog with an exit transition and a button outside it that closes it.
func closingDialog(cx *core.Scope) core.Node {
	open := core.NewSignal(cx, true)
	return base.Dialog{
		Open: open.Get, Animation: core.ModalNone, TestID: "dialog",
		PopupStyle: core.Style{Width: core.Pt(100), Height: core.Pt(100),
			Transition: core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionOpacity}},
		EndingStyle: core.Style{Opacity: core.Alpha(0)},
		After: []core.Node{core.View{Role: core.RoleButton, Label: "close", TestID: "close",
			OnPress: func(core.PressEvent) { open.Set(false) }, Style: core.Style{Width: core.Pt(40), Height: core.Pt(40)}}},
	}
}

// A step that closes a dialog returns once its exit transition is over and it has left the tree.
func TestStepsWaitForEndingStyleTransition(t *testing.T) {
	app := Start(t, closingDialog)
	app.Press("id:close")
	if app.Exists("id:dialog") {
		t.Fatal("Press returned while the dialog's ending transition still runs")
	}
}

// ManualFrames leaves the exit transition to the test's Advance.
func TestManualFramesKeepsEndingStyleTransitionPending(t *testing.T) {
	app := Start(t, closingDialog, ManualFrames())
	app.Press("id:close")
	if !app.Exists("id:dialog") {
		t.Fatal("ManualFrames advanced the ending transition")
	}
	app.Advance(200 * time.Millisecond)
	if app.Exists("id:dialog") {
		t.Fatal("ending transition did not finish after Advance")
	}
}

func TestWaitForGoneFinishesSheetClosedByPost(t *testing.T) {
	if onTarget() {
		t.Skip("checks the headless clock")
	}
	reply := make(chan struct{})
	app := Start(t, func(cx *core.Scope) core.Node {
		open := core.NewSignal(cx, true)
		return base.Sheet{
			Open: open.Get, TestID: "sheet",
			PopupStyle: core.Style{Width: core.Pt(160), Height: core.Pt(100)},
			Children: []core.Node{core.View{
				Role: core.RoleButton, TestID: "close",
				Style: core.Style{Width: core.Pt(60), Height: core.Pt(40)},
				OnPress: func(core.PressEvent) {
					go func() {
						select {
						case <-t.Context().Done():
						case <-reply:
							cx.Post(func() { open.Set(false) })
						}
					}()
				},
			}},
		}
	})
	app.Press("id:close")
	app.Find("id:sheet")
	close(reply)
	app.WaitForGone("id:sheet", time.Second)
}

func TestManualFramesKeepsTheClockStillDuringWaits(t *testing.T) {
	app := Start(t, func(*core.Scope) core.Node { return core.View{} }, ManualFrames())
	before := app.Clock.Now()
	checks := 0
	app.WaitUntil(time.Second, func() bool {
		checks++
		return checks == 2
	})
	if after := app.Clock.Now(); after != before {
		t.Fatalf("WaitUntil advanced the manual clock by %v", after.Sub(before))
	}
}

// fading is a dynamic style whose opacity follows on, with a transition of delay and duration.
func fading(on *core.Signal[bool], delay, duration time.Duration) func() core.Style {
	return func() core.Style {
		opacity := core.Alpha(1)
		if on.Get() {
			opacity = core.Alpha(0.5)
		}
		return core.Style{Width: core.Pt(40), Height: core.Pt(40), Opacity: opacity,
			Transition: core.Transition{Delay: delay, Duration: duration, Properties: core.TransitionOpacity}}
	}
}

// A step that restyles a view runs its OnTransitionEnd once the transition's delay and duration are over, and returns then.
func TestStepsRunOnTransitionEnd(t *testing.T) {
	var ends []time.Duration
	var began time.Time
	app := Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, false)
		return core.View{Children: []core.Node{
			core.View{TestID: "faded", OnTransitionEnd: func() { ends = append(ends, cx.Now().Sub(began)) },
				StyleFunc: fading(on, 50*time.Millisecond, 250*time.Millisecond)},
			core.View{Role: core.RoleButton, Label: "fade", TestID: "fade", OnPress: func(core.PressEvent) { on.Set(true) },
				Style: core.Style{Width: core.Pt(40), Height: core.Pt(40)}},
		}}
	})
	ends = nil
	began = app.Clock.Now()
	app.Press("id:fade")
	if len(ends) != 1 {
		t.Fatalf("OnTransitionEnd ran %d times in the step, want once", len(ends))
	}
	if want := 300 * time.Millisecond; ends[0] < want || ends[0] > want+driver.FrameInterval {
		t.Fatalf("OnTransitionEnd ran %v after the press, want %v", ends[0], want)
	}
	if took := app.Clock.Now().Sub(began); took > 300*time.Millisecond+driver.FrameInterval {
		t.Fatalf("the step took %v of the clock, past the transition's end", took)
	}
}

// Answering an alert is a step: what the answer starts, as a sheet's exit, runs its frames before Answer returns, and so do the answers OnAlert gives at the end of a step.
func TestAnswerRunsOnTransitionEnd(t *testing.T) {
	ends := 0
	app := Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, false)
		return core.View{Children: []core.Node{
			core.View{TestID: "faded", OnTransitionEnd: func() { ends++ }, StyleFunc: fading(on, 0, 250*time.Millisecond)},
			core.View{Role: core.RoleButton, Label: "ask", TestID: "ask", Style: core.Style{Width: core.Pt(40), Height: core.Pt(40)}, OnPress: func(core.PressEvent) {
				cx.ShowAlert(core.AlertConfig{Title: "Fade?", Actions: []core.AlertAction{{Label: "No", Role: core.AlertCancel}, {Label: "Yes"}}}, func(i int) {
					if i == 1 {
						on.Set(!on.Get())
					}
				})
			}},
		}}
	}, PendingAlerts())
	ends = 0
	app.Press("id:ask")
	app.Answer(1)
	if ends != 1 {
		t.Fatalf("OnTransitionEnd ran %d times by the time Answer returned, want once", ends)
	}
	app.OnAlert = func(driver.AlertConfig) int { return 1 }
	app.Press("id:ask")
	if ends != 2 {
		t.Fatalf("OnTransitionEnd ran %d times after OnAlert answered, want twice", ends)
	}
}

// An OnAlert answer that brings its alert back each time fails the step, naming it, rather than holding the test for good.
func TestOnAlertThatNeverEndsFailsTheStep(t *testing.T) {
	msg := fails(t, func(tb testing.TB) {
		app := Start(tb, func(cx *core.Scope) core.Node {
			var ask func()
			ask = func() {
				cx.ShowAlert(core.AlertConfig{Title: "Retry?", Actions: []core.AlertAction{{Label: "Retry"}, {Label: "Cancel", Role: core.AlertCancel}}}, func(i int) {
					if i == 0 {
						ask()
					}
				})
			}
			return core.View{Role: core.RoleButton, Label: "load", TestID: "load", Style: core.Style{Width: core.Pt(40), Height: core.Pt(40)}, OnPress: func(core.PressEvent) { ask() }}
		})
		app.OnAlert = func(driver.AlertConfig) int { return 0 }
		app.Press("id:load")
	})
	if !strings.Contains(msg, "press(id:load)") || !strings.Contains(msg, `"Retry?"`) {
		t.Fatalf("an answer that brings its alert back failed with %q, want the step and the alert named", msg)
	}
}

// A view whose OnTransitionEnd restyles it again holds a step for settleLimit at most.
func TestEndlessTransitionsHoldAStepForTheLimit(t *testing.T) {
	app := Start(t, func(cx *core.Scope) core.Node {
		flip := core.NewSignal(cx, false)
		toggle := func() { flip.Set(!flip.GetUntracked()) }
		return core.View{TestID: "pulse", Role: core.RoleButton, Label: "pulse",
			OnPress: func(core.PressEvent) { toggle() }, OnTransitionEnd: toggle,
			StyleFunc: fading(flip, 0, 100*time.Millisecond)}
	})
	began := app.Clock.Now()
	app.Press("id:pulse")
	if took := app.Clock.Now().Sub(began); took < settleLimit || took > settleLimit+2*driver.FrameInterval {
		t.Fatalf("the step took %v of the clock, want the %v limit", took, settleLimit)
	}
}

// A view removed while its transition runs holds the step no longer.
func TestARemovedViewsTransitionHoldsNoStep(t *testing.T) {
	app := Start(t, func(cx *core.Scope) core.Node {
		shown, faded := core.NewSignal(cx, true), core.NewSignal(cx, false)
		return core.View{Children: []core.Node{
			core.View{Role: core.RoleButton, Label: "fade", TestID: "fade",
				OnPress: func(core.PressEvent) {
					faded.Set(true)
					cx.AfterFunc(50*time.Millisecond, func() { shown.Set(false) })
				}, Style: core.Style{Width: core.Pt(40), Height: core.Pt(40)}},
			core.Show{When: shown.Get, Then: func(*core.Scope) core.Node {
				return core.View{TestID: "faded", OnTransitionEnd: func() { t.Error("OnTransitionEnd ran for a removed view") },
					StyleFunc: fading(faded, 0, time.Second)}
			}},
		}}
	})
	began := app.Clock.Now()
	app.Press("id:fade")
	if app.Exists("id:faded") {
		t.Fatal("the view was not removed")
	}
	if took := app.Clock.Now().Sub(began); took > 50*time.Millisecond+2*driver.FrameInterval {
		t.Fatalf("the step took %v of the clock for a view removed after 50ms", took)
	}
}

// Idiom changes the idiom alone: the rest of the environment stays the default, or the Environment option's in either order, also when one Environment option starts more than one app.
func TestIdiomOptionChangesOnlyTheIdiom(t *testing.T) {
	root := func(cx *core.Scope) core.Node {
		return core.Text{TestID: "env", ContentFunc: func() string {
			e := cx.Env()
			return fmt.Sprintf("desktop=%v os=%q dark=%v scale=%v lifecycle=%v locale=%q", e.Desktop(), e.OS(), e.Dark(), e.FontScale(), e.Lifecycle(), e.Locale())
		}}
	}
	read := func(opts ...Option) string { return Start(t, root, opts...).Find("id:env").Text }
	if got, want := read(), `desktop=false os="" dark=false scale=1 lifecycle=active locale=""`; got != want {
		t.Fatalf("default environment = %s, want %s", got, want)
	}
	if got, want := read(Idiom(core.IdiomDesktop)), `desktop=true os="" dark=false scale=1 lifecycle=active locale=""`; got != want {
		t.Fatalf("with Idiom = %s, want %s", got, want)
	}
	env := Environment(core.Environment{OS: core.OSWindows, Dark: true, FontScale: 1.5, Locale: "fr-FR"})
	want := `desktop=true os="windows" dark=true scale=1.5 lifecycle=active locale="fr-FR"`
	if got := read(env, Idiom(core.IdiomDesktop)); got != want {
		t.Fatalf("Environment then Idiom = %s, want %s", got, want)
	}
	if got := read(Idiom(core.IdiomDesktop), env); got != want {
		t.Fatalf("Idiom then Environment = %s, want %s", got, want)
	}
	if got, want := read(env), `desktop=false os="windows" dark=true scale=1.5 lifecycle=active locale="fr-FR"`; got != want {
		t.Fatalf("Environment reused after Idiom = %s, want %s", got, want)
	}
}

// A desktop idiom is all a headless test needs to hover a view and find its tooltip, which phones never show.
func TestIdiomOptionShowsHoverTooltips(t *testing.T) {
	root := func(cx *core.Scope) core.Node {
		return core.View{TestID: "target", Tooltip: core.Text{Content: "the tip"}, Children: []core.Node{core.Text{Content: "hover me"}}}
	}
	for _, c := range []struct {
		name string
		opts []Option
		want bool
	}{{"phone", nil, false}, {"desktop", []Option{Idiom(core.IdiomDesktop)}, true}} {
		t.Run(c.name, func(t *testing.T) {
			app := Start(t, root, c.opts...)
			app.Hover("id:target")
			app.Advance(time.Second)
			if got := app.Exists("text:the tip"); got != c.want {
				t.Fatalf("tooltip shown = %v, want %v", got, c.want)
			}
		})
	}
}

// Frames follow the manual clock: Advance runs the frame an app waits for
// at each display interval, with the frame's time from that clock.
func TestFramesFollowTheClock(t *testing.T) {
	var times []time.Time
	app := Start(t, func(cx *core.Scope) core.Node {
		var loop func(at time.Time)
		loop = func(at time.Time) {
			times = append(times, at)
			cx.RequestAnimationFrame(loop)
		}
		cx.RequestAnimationFrame(loop)
		return core.View{}
	})
	app.Advance(3 * driver.FrameInterval)
	if len(times) != 3 {
		t.Fatalf("the callback ran %d times in three frames, want 3", len(times))
	}
	for i := 1; i < len(times); i++ {
		if got := times[i].Sub(times[i-1]); got != driver.FrameInterval {
			t.Fatalf("frame %d came %v after the one before, want %v", i, got, driver.FrameInterval)
		}
	}
	if !times[2].Equal(app.Clock.Now()) {
		t.Fatalf("the last frame's time = %v, want the clock's %v", times[2], app.Clock.Now())
	}
}

func TestWallTimeOptionSetsTheFirstFrame(t *testing.T) {
	want := time.Date(2031, 4, 5, 6, 7, 8, 0, time.FixedZone("test", 9*60*60))
	var got time.Time
	Start(t, func(cx *core.Scope) core.Node {
		got = cx.WallTime()
		return core.View{}
	}, WallTime(want))
	if !got.Equal(want) || got.Location() != want.Location() {
		t.Fatalf("first wall time = %v (%v), want %v (%v)", got, got.Location(), want, want.Location())
	}
}

// Without an option the first render reads the clock's documented start, in UTC.
func TestWallTimeStartsAtTheClocksStart(t *testing.T) {
	var got time.Time
	app := Start(t, func(cx *core.Scope) core.Node {
		got = cx.WallTime()
		return core.View{}
	})
	if want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC); !got.Equal(want) || !app.Clock.Now().Equal(want) {
		t.Fatalf("first wall time = %v, clock = %v, want both %v", got, app.Clock.Now(), want)
	}
}

func TestClockOptionRunsAppTimers(t *testing.T) {
	clock := core.NewManualClock(time.Date(2032, 1, 1, 0, 0, 0, 0, time.UTC))
	var fired int
	app := Start(t, func(cx *core.Scope) core.Node {
		cx.AfterFunc(time.Hour, func() { fired++ })
		return core.View{}
	}, Clock(clock))
	if app.Clock != clock {
		t.Fatal("Clock option did not install the supplied clock")
	}
	clock.Advance(time.Hour - time.Millisecond)
	app.Settle()
	if fired != 0 {
		t.Fatalf("timer fired %d times before its hour, want 0", fired)
	}
	clock.Advance(time.Millisecond)
	app.Settle()
	if fired != 1 {
		t.Fatalf("timer fired %d times, want 1", fired)
	}
}

// WallTime applies over the Clock option's clock whichever comes first, and moves none of its timers.
func TestWallTimeAppliesToTheClockOption(t *testing.T) {
	want := time.Date(2040, 6, 1, 12, 0, 0, 0, time.UTC)
	for _, order := range []string{"wall first", "clock first"} {
		t.Run(order, func(t *testing.T) {
			clock := core.NewManualClock(time.Date(2032, 1, 1, 0, 0, 0, 0, time.UTC))
			opts := []Option{WallTime(want), Clock(clock)}
			if order == "clock first" {
				opts = []Option{Clock(clock), WallTime(want)}
			}
			var got time.Time
			var fired int
			app := Start(t, func(cx *core.Scope) core.Node {
				got = cx.WallTime()
				cx.AfterFunc(time.Hour, func() { fired++ })
				return core.View{}
			}, opts...)
			if !got.Equal(want) {
				t.Fatalf("first wall time = %v, want %v", got, want)
			}
			if !app.Clock.Now().Equal(time.Date(2032, 1, 1, 0, 0, 0, 0, time.UTC)) {
				t.Fatalf("clock = %v, want the clock's own start", app.Clock.Now())
			}
			app.Settle()
			if fired != 0 {
				t.Fatalf("timer fired %d times, want 0", fired)
			}
		})
	}
}

// The app's directories are the test's own, made before the app starts, so a test never writes to the user's data, cache or logs; in a browser they are empty, as the web backend's are.
func TestAppDirectoriesAreTemporary(t *testing.T) {
	if Target() == "web" {
		t.Skip("the web has no app directories")
	}
	var dirs []string
	Start(t, func(cx *core.Scope) core.Node {
		dirs = []string{cx.DataDir(), cx.CacheDir(), cx.LogsDir()}
		return core.View{}
	})
	if runtime.GOOS == "js" {
		for _, dir := range dirs {
			if dir != "" {
				t.Fatalf("browser app directories = %q, want empty", dirs)
			}
		}
		return
	}
	tmp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, d := range dirs {
		real, err := filepath.EvalSymlinks(d)
		if err != nil {
			t.Fatalf("%q: %v", d, err)
		}
		if fi, err := os.Stat(real); err != nil || !fi.IsDir() || !strings.HasPrefix(real, tmp+string(filepath.Separator)) || seen[real] {
			t.Fatalf("app directories %q: %q is not a directory of its own in %s", dirs, d, tmp)
		}
		seen[real] = true
	}
}

func TestEnvironmentReportsMinimized(t *testing.T) {
	var states []bool
	app := Start(t, func(cx *core.Scope) core.Node {
		cx.Effect(func() { states = append(states, desktop.Minimized(cx)) })
		return core.View{}
	})
	app.SetEnvironment(core.Environment{Minimized: true})
	if len(states) != 2 || states[0] || !states[1] {
		t.Fatalf("minimized states %v", states)
	}
}

// TestHotKeyRegistrationAnswers: the headless platform registers an app's global shortcuts, answers a combination the app holds already as taken, and those HotKeyTaken and HotKeyUnsupported list as they say, so a test sees what the app shows for each.
func TestHotKeyRegistrationAnswers(t *testing.T) {
	if Target() != "" {
		t.Skip("a platform answers the shortcuts itself")
	}
	recorder := func(cx *core.Scope) core.Node {
		first := cx.RegisterHotKey("Ctrl+Alt+K", nil)
		again := cx.RegisterHotKey("ctrl+alt+k", nil)
		other := cx.RegisterHotKey("Alt+Space", nil)
		return core.View{Children: []core.Node{
			core.Text{ContentFunc: func() string { return "first " + first.State().String() }},
			core.Text{ContentFunc: func() string { return "again " + again.State().String() }},
			core.Text{ContentFunc: func() string { return "other " + other.State().String() }},
		}}
	}
	app := Start(t, recorder)
	app.Find("text:first registered")
	app.Find("text:again taken")
	app.Find("text:other registered")

	app = Start(t, recorder, HotKeyTaken("ctrl+alt+k"), HotKeyUnsupported("alt + space"))
	app.Find("text:first taken")
	app.Find("text:again taken")
	app.Find("text:other unsupported")
}

// Headless there are no native pixels: Capture and CapturePNG skip the test, and Screenshot only logs, but still fails on a name that leaves the screenshots directory.
func TestCaptureSkipsHeadlessAndScreenshotNamesStayInside(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless behavior")
	}
	root := func(cx *core.Scope) core.Node { return core.View{} }
	for name, capture := range map[string]func(*App){
		"Capture":    func(a *App) { a.Capture() },
		"CapturePNG": func(a *App) { a.CapturePNG() },
	} {
		var sub *testing.T
		t.Run(name, func(t *testing.T) {
			sub = t
			capture(Start(t, root))
			t.Error("captured headless")
		})
		if !sub.Skipped() {
			t.Errorf("%s did not skip the test headless", name)
		}
	}
	for _, name := range []string{"", ".", "./", "../outside", "en/../../outside", filepath.Join(os.TempDir(), "outside")} {
		if msg := fails(t, func(tb testing.TB) { Start(tb, root).Screenshot(name) }); !strings.Contains(msg, "relative file path") {
			t.Errorf("Screenshot(%q) failed with %q, want a relative file path asked for", name, msg)
		}
	}
	if msg := fails(t, func(tb testing.TB) { Start(tb, root).Screenshot("en/my library") }); msg != "" {
		t.Errorf("Screenshot of a relative name failed headless: %s", msg)
	}
}

// A drop on a view inside a drop zone reaches the zone, as a platform's hit test walks up to it; one with no zone around it, a disabled zone, or a zone outside the modal the view is in fails the step.
func TestDropFilesUsesTheNearestEnabledDropAncestor(t *testing.T) {
	var got []string
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.View{TestID: "zone", Style: core.Style{Width: core.Pt(100), Height: core.Pt(40)},
			OnFileDrop: func(uris []string) { got = append([]string(nil), uris...) },
			Children:   []core.Node{core.Text{Content: "drop here", TestID: "label"}}}
	})
	app.DropFiles("id:label", "file:///video.mp4")
	if len(got) != 1 || got[0] != "file:///video.mp4" {
		t.Fatalf("drop reached %q, want the ancestor", got)
	}
	if msg := fails(t, func(tb testing.TB) {
		Start(tb, func(cx *core.Scope) core.Node { return core.Text{Content: "elsewhere", TestID: "outside"} }).DropFiles("id:outside", "file:///a.mp4")
	}); !strings.Contains(msg, "no view hearing file drops matches") {
		t.Errorf("a drop with no zone failed with %q", msg)
	}
	if msg := fails(t, func(tb testing.TB) {
		app := Start(tb, func(cx *core.Scope) core.Node {
			return core.View{Disabled: func() bool { return true }, OnFileDrop: func([]string) { t.Error("a disabled zone took the drop") },
				Children: []core.Node{core.Text{Content: "drop here", TestID: "label"}}}
		})
		app.DropFiles("id:label", "file:///a.mp4")
	}); !strings.Contains(msg, "is disabled") {
		t.Errorf("a drop on a disabled zone failed with %q", msg)
	}
	if msg := fails(t, func(tb testing.TB) {
		app := Start(tb, func(cx *core.Scope) core.Node {
			return core.View{OnFileDrop: func([]string) { t.Error("a zone outside the modal took the drop") }, Children: []core.Node{
				core.Modal{Children: []core.Node{core.Text{Content: "in the modal", TestID: "inmodal"}}}}}
		})
		app.DropFiles("id:inmodal", "file:///a.mp4")
	}); !strings.Contains(msg, "no view hearing file drops matches") {
		t.Errorf("a drop in a modal failed with %q", msg)
	}
}

// Bounds is the frame Press aims at, in surface points: its ancestors' origins less the scroll offsets of the scroll views around it.
func TestViewBoundsUsesSurfaceCoordinates(t *testing.T) {
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Padding: core.All(10)}, Children: []core.Node{
			core.View{TestID: "child", Style: core.Style{Width: core.Pt(40), Height: core.Pt(20)}},
			core.Scroll{TestID: "scroll", Style: core.Style{Height: core.Pt(100)}, Children: []core.Node{
				core.View{Style: core.Style{Height: core.Pt(300)}},
				core.View{TestID: "row", Style: core.Style{Height: core.Pt(30)}}}}}}
	})
	b := app.Find("id:child").Bounds()
	if b.Origin != (core.Point{X: 10, Y: 10}) || b.Size != (core.Size{Width: 40, Height: 20}) {
		t.Fatalf("bounds = %+v, want 10,10 40x20", b)
	}
	app.ScrollTo("id:scroll", core.Point{Y: 250})
	if b := app.Find("id:row").Bounds(); b.Origin.Y != 10+20+300-250 {
		t.Fatalf("scrolled row bounds = %+v, want y %v", b, 10+20+300-250)
	}
}

// Headless, the system switch takes a press itself, as on a target: it reports the value it flips to. A disabled one fails the press, and a hidden one matches nothing.
func TestPressTogglesSystemSwitchHeadless(t *testing.T) {
	for _, os := range []core.OS{core.OSMacOS, core.OSIOS} {
		var changes []bool
		checked := false
		disabled := false
		app := Start(t, func(cx *core.Scope) core.Node {
			on := core.NewSignal(cx, false)
			return core.SystemSwitch{Checked: func() bool { checked = on.Get(); return checked }, Disabled: func() bool { return disabled },
				OnChange: func(v bool) { changes = append(changes, v); on.Set(v) }, TestID: "switch"}
		}, Environment(core.Environment{OS: os}))
		app.Press("id:switch")
		app.Press("id:switch")
		if len(changes) != 2 || !changes[0] || changes[1] || checked {
			t.Fatalf("%v: presses reported %v, checked %v", os, changes, checked)
		}
	}
	if msg := fails(t, func(tb testing.TB) {
		Start(tb, func(cx *core.Scope) core.Node {
			return core.SystemSwitch{Disabled: func() bool { return true }, OnChange: func(bool) { t.Error("a disabled switch changed") }, TestID: "switch"}
		}, Environment(core.Environment{OS: core.OSMacOS})).Press("id:switch")
	}); !strings.Contains(msg, "disabled") {
		t.Errorf("a press on a disabled switch failed with %q", msg)
	}
	if msg := fails(t, func(tb testing.TB) {
		Start(tb, func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Display: core.DisplayNone}, Children: []core.Node{
				core.SystemSwitch{OnChange: func(bool) { t.Error("a hidden switch changed") }, TestID: "switch"}}}
		}, Environment(core.Environment{OS: core.OSMacOS})).Press("id:switch")
	}); msg == "" {
		t.Error("a press on a hidden switch passed")
	}
}

// A web view that is mounted but hidden goes on loading and posting messages, as a platform's does.
func TestNavigateReportsToHiddenMountedWebView(t *testing.T) {
	var loaded []string
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Display: core.DisplayNone}, Children: []core.Node{
			core.WebView{TestID: "web", Source: core.WebSrc("https://example.com"),
				OnLoadStart: func(n core.WebNavigation) { loaded = append(loaded, "start:"+n.URL) },
				OnLoad:      func(n core.WebNavigation) { loaded = append(loaded, "load:"+n.URL) },
				OnMessage:   func(d string) { loaded = append(loaded, "message:"+d) }}}}
	})
	app.Navigate("id:web", core.WebNavigation{URL: "https://example.com/next"})
	app.PostMessage("id:web", "hi")
	if want := "start:https://example.com/next load:https://example.com/next message:hi"; strings.Join(loaded, " ") != want {
		t.Fatalf("web view events = %v, want %s", loaded, want)
	}
}

// The model records reveals and trashes, answers them as the test says, and passes core.ErrNoTrash on as itself.
func TestRevealAndTrashAnswerAsTheTestSays(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("revealing a file needs one on a filesystem, which a browser lacks")
	}
	var cx *core.Scope
	app := Start(t, func(c *core.Scope) core.Node { cx = c; return core.View{} })
	path := filepath.Join(t.TempDir(), "film.mp4")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var revealed error = errors.New("no answer")
	cx.RevealFile(path, func(err error) { revealed = err })
	app.Settle()
	if revealed != nil || len(app.RevealedFiles) != 1 || app.RevealedFiles[0] != path {
		t.Fatalf("reveal answered %v, recorded %q", revealed, app.RevealedFiles)
	}
	var at string
	var trashed error
	trash := func() {
		at, trashed = "unanswered", nil
		cx.TrashFile(path, func(a string, err error) { at, trashed = a, err })
		app.Settle()
	}
	trash()
	if at != "" || trashed != nil {
		t.Fatalf("default trash answered %q, %v", at, trashed)
	}
	app.TrashFile = func(string) (string, error) { return "/Trash/film.mp4", nil }
	trash()
	if at != "/Trash/film.mp4" || trashed != nil {
		t.Fatalf("trash answered %q, %v", at, trashed)
	}
	app.TrashFile = func(string) (string, error) { return "", core.ErrNoTrash }
	trash()
	if !errors.Is(trashed, core.ErrNoTrash) {
		t.Fatalf("no trash answered %v", trashed)
	}
	if len(app.TrashedFiles) != 3 {
		t.Fatalf("recorded %q", app.TrashedFiles)
	}
}

// With a desktop environment the main view shows in the main window, whose close CloseWindow asks for: its handlers run newest first, and it closes unless one cancels.
func TestCloseWindowRunsTheMainWindowsHandlers(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	var calls []string
	var w *desktop.Window
	cancel := true
	app := Start(t, func(cx *core.Scope) core.Node {
		w = desktop.CurrentWindow(cx)
		if w == nil {
			t.Fatal("the main view is in no window")
		}
		w.OnClosing(cx, func(c *desktop.WindowClosing) {
			calls = append(calls, "old")
			if cancel {
				c.Cancel()
			}
		})
		w.OnClosing(cx, func(c *desktop.WindowClosing) { calls = append(calls, "new") })
		return core.Text{Content: "main", TestID: "main"}
	}, Environment(core.Environment{OS: core.OSMacOS}))
	if app.root.MainWindow() != w {
		t.Fatal("the main view's window is not the main window")
	}
	if app.CloseWindow() {
		t.Fatal("a canceled close closed the window")
	}
	if got, want := strings.Join(calls, ","), "new,old"; got != want {
		t.Fatalf("closing handlers ran %q, want %q", got, want)
	}
	app.Find("id:main")
	cancel, calls = false, nil
	if !app.CloseWindow() || app.root.MainWindow() != nil {
		t.Fatal("an accepted close left the window open")
	}
	if len(app.Query("id:main")) != 0 {
		t.Fatal("steps still reach the closed window's views")
	}
	if msg := fails(t, func(tb testing.TB) { Start(tb, func(cx *core.Scope) core.Node { return core.View{} }).CloseWindow() }); !strings.Contains(msg, "no window to close") {
		t.Errorf("CloseWindow without a desktop environment failed with %q", msg)
	}
}

// Application-menu items with a standard role and no OnSelect do headless what the desktop platforms do with them, by their shortcut and by ChooseMenu, and a role the model does not carry out fails the step rather than swallow its key (#711).
func TestApplicationMenuRolesHeadless(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless menu role path")
	}
	mac := Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop})
	start := func(tb testing.TB, menu []desktop.MenuItem, view func(cx *core.Scope) core.Node, opts ...Option) *App {
		return StartApp(tb, func(cx *core.Scope) {
			desktop.SetApplicationMenu(cx, menu)
			cx.SetMainView(view)
		}, opts...)
	}
	text := func(cx *core.Scope) core.Node { return core.Text{Content: "main"} }

	// Close closes the main window as its close button does, its OnClosing handlers deciding.
	cancel, closings := true, 0
	app := start(t, []desktop.MenuItem{{Label: "File", Items: []desktop.MenuItem{{Role: core.MenuClose}}}}, func(cx *core.Scope) core.Node {
		desktop.CurrentWindow(cx).OnClosing(cx, func(c *desktop.WindowClosing) {
			closings++
			if cancel {
				c.Cancel()
			}
		})
		return core.Text{Content: "main"}
	}, mac)
	app.Key("mod+w")
	if closings != 1 || app.root.MainWindow() == nil {
		t.Fatalf("a canceled close role ran %d handlers and closed the window: %v", closings, app.root.MainWindow() == nil)
	}
	cancel = false
	app.ChooseMenu("File", "Close")
	if closings != 2 || app.root.MainWindow() != nil {
		t.Fatalf("the close role ran %d handlers and left the window open: %v", closings, app.root.MainWindow() != nil)
	}

	// On macOS Quit asks the app, which may cancel; elsewhere it quits without asking, as the platforms do.
	quitMenu := []desktop.MenuItem{{Label: "App", Items: []desktop.MenuItem{{Role: core.MenuQuit}}}}
	for _, os := range []core.OS{core.OSMacOS, core.OSWindows, core.OSLinux} {
		requests, keep := 0, true
		app = StartApp(t, func(cx *core.Scope) {
			desktop.SetApplicationMenu(cx, quitMenu)
			desktop.OnQuitRequest(cx, func(q *desktop.QuitRequest) {
				requests++
				if keep {
					q.Cancel()
				}
			})
			cx.SetMainView(text)
		}, Environment(core.Environment{OS: os, Idiom: core.IdiomDesktop}))
		app.ChooseMenu("App", "Quit")
		keep = false
		if os == core.OSMacOS {
			if requests != 1 || app.Quits != 0 {
				t.Fatalf("macOS: a canceled quit role asked %d times and quit %d times", requests, app.Quits)
			}
			app.Key("meta+q")
			if requests != 2 || app.Quits != 1 {
				t.Fatalf("macOS: an approved quit role asked %d times and quit %d times", requests, app.Quits)
			}
			continue
		}
		app.Key("alt+f4")
		if requests != 0 || app.Quits != 2 {
			t.Fatalf("%v: the quit role asked %d times and quit %d times", os, requests, app.Quits)
		}
	}

	// Minimize, Zoom and Toggle Full Screen change the window state the environment reports.
	var env *core.Env
	app = start(t, []desktop.MenuItem{{Label: "Window", Items: []desktop.MenuItem{{Role: core.MenuMinimize}, {Role: core.MenuZoom}, {Role: core.MenuToggleFullScreen}}}}, func(cx *core.Scope) core.Node {
		env = cx.Env()
		return core.Text{Content: "main"}
	}, mac)
	app.ChooseMenu("Window", "Zoom")
	app.Key("ctrl+meta+f")
	if !env.Maximized() || !env.FullScreen() || env.Minimized() {
		t.Fatalf("zoom and full screen reported maximized %v, full screen %v, minimized %v", env.Maximized(), env.FullScreen(), env.Minimized())
	}
	app.Key("meta+m")
	app.ChooseMenu("Window", "Zoom")
	if env.Maximized() || !env.FullScreen() || !env.Minimized() {
		t.Fatalf("minimize and a second zoom reported maximized %v, full screen %v, minimized %v", env.Maximized(), env.FullScreen(), env.Minimized())
	}

	// A non-editing role with OnSelect is an app action on every desktop; it does not also minimize or quit.
	selected := 0
	app = start(t, []desktop.MenuItem{{Label: "App", Items: []desktop.MenuItem{
		{Role: core.MenuQuit, OnSelect: func() { selected++ }},
		{Role: core.MenuMinimize, OnSelect: func() { selected++ }},
	}}}, func(cx *core.Scope) core.Node {
		env = cx.Env()
		return core.Text{Content: "main"}
	}, mac)
	app.ChooseMenu("App", "Quit")
	app.ChooseMenu("App", "Minimize")
	if selected != 2 || app.Quits != 0 || env.Minimized() {
		t.Fatalf("role callbacks ran %d times, quit %d times, minimized %v", selected, app.Quits, env.Minimized())
	}

	// The editing roles act on the focused field; one with OnSelect runs it only when no field takes it.
	var pastes []string
	selected = 0
	app = start(t, []desktop.MenuItem{{Label: "Edit", Items: []desktop.MenuItem{
		{Role: core.MenuCopy}, {Role: core.MenuPaste}, {Role: core.MenuPasteAndMatchStyle}, {Role: core.MenuDelete},
		{Role: core.MenuSelectAll, OnSelect: func() { selected++ }}, {Role: core.MenuUndo},
	}}}, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.Input{TestID: "field", DefaultValue: "hello world", MaxLength: 14},
			core.Input{TestID: "report", PasteBehavior: core.PasteReport, OnPaste: func(p core.PasteEvent) { pastes = append(pastes, p.Text) }},
		}}
	}, mac)
	app.ChooseMenu("Edit", "Select All")
	if selected != 1 {
		t.Fatalf("select all with no field focused ran OnSelect %d times", selected)
	}
	app.Focus("id:field")
	app.Select("id:field", 0, 5)
	app.ChooseMenu("Edit", "Copy")
	app.Select("id:field", 6, 11)
	app.ChooseMenu("Edit", "Delete")
	if got := app.Find("id:field").Text; app.Clipboard != "hello" || got != "hello " {
		t.Fatalf("copy and delete left %q in the clipboard and %q in the field", app.Clipboard, got)
	}
	app.Clipboard = "big\nworld"
	app.Key("alt+shift+meta+v")
	if got := app.Find("id:field").Text; got != "hello big worl" {
		t.Fatalf("paste and match style produced %q", got)
	}
	app.Key("meta+a")
	app.ChooseMenu("Edit", "Select All")
	if s := app.Find("id:field").Selection; selected != 1 || s.Start != 0 || s.End != 14 {
		t.Fatalf("select all in the field ran OnSelect %d times and selected %+v", selected, s)
	}
	app.Focus("id:report")
	app.ChooseMenu("Edit", "Paste")
	if got := app.Find("id:report").Text; len(pastes) != 1 || pastes[0] != "big\nworld" || got != "" {
		t.Fatalf("a reporting field heard %q and holds %q", pastes, got)
	}

	// Roles the model does not carry out fail the step, naming the role.
	for _, c := range []struct {
		name string
		run  func(*App)
		want string
	}{
		{"about", func(app *App) { app.ChooseMenu("App", "About") }, `menu role "About" is not modelled headless`},
		{"hide", func(app *App) { app.Key("meta+h") }, `menu role "Hide" is not modelled headless`},
		{"undo", func(app *App) { app.Focus("id:field"); app.Key("meta+z") }, `menu role "Undo" is not modelled headless in a field`},
		{"holder", func(app *App) { app.ChooseMenu("App", "Window") }, `does nothing`},
	} {
		msg := fails(t, func(tb testing.TB) {
			app := start(tb, []desktop.MenuItem{{Label: "App", Items: []desktop.MenuItem{
				{Role: core.MenuAbout}, {Role: core.MenuHide}, {Role: core.MenuUndo}, {Role: core.MenuWindowMenu},
			}}}, func(cx *core.Scope) core.Node { return core.Input{TestID: "field"} }, mac)
			c.run(app)
		})
		if !strings.Contains(msg, c.want) {
			t.Errorf("%s failed with %q, want %q", c.name, msg, c.want)
		}
	}
}

// A focused field takes its own clipboard keys, with or without an Edit menu, as the platforms' fields do: copy and cut take a selection a masked field keeps to itself, and paste replaces the selection.
func TestFieldClipboardKeys(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless field model")
	}
	for _, os := range []core.OS{core.OSWindows, core.OSAndroid} {
		app := Start(t, func(cx *core.Scope) core.Node {
			return core.View{Children: []core.Node{
				core.Input{TestID: "field", DefaultValue: "abc"},
				core.Input{TestID: "secret", DefaultValue: "pw", Masked: true},
			}}
		}, Environment(core.Environment{OS: os}))
		app.Focus("id:field")
		app.Key("ctrl+c")
		if app.Clipboard != "" {
			t.Fatalf("%v: copy with no selection put %q in the clipboard", os, app.Clipboard)
		}
		app.Select("id:field", 0, 3)
		app.Key("ctrl+c")
		app.Select("id:field", 1, 2)
		app.Key("ctrl+v")
		if got := app.Find("id:field").Text; app.Clipboard != "abc" || got != "aabcc" {
			t.Fatalf("%v: copy and paste left %q in the clipboard and %q in the field", os, app.Clipboard, got)
		}
		app.Select("id:field", 0, 2)
		app.Key("ctrl+x")
		if got := app.Find("id:field").Text; app.Clipboard != "aa" || got != "bcc" {
			t.Fatalf("%v: cut left %q in the clipboard and %q in the field", os, app.Clipboard, got)
		}
		app.Focus("id:secret")
		app.Select("id:secret", 0, 2)
		app.Key("ctrl+x")
		if got := app.Find("id:secret").Text; app.Clipboard != "aa" || got != "pw" {
			t.Fatalf("%v: cutting a masked field left %q in the clipboard and %q in the field", os, app.Clipboard, got)
		}
	}

	// An application-menu item the app gave a clipboard key as its shortcut takes it before the field, as the desktops' menus do.
	for _, os := range []core.OS{core.OSMacOS, core.OSWindows, core.OSLinux} {
		chosen := 0
		app := StartApp(t, func(cx *core.Scope) {
			desktop.SetApplicationMenu(cx, []desktop.MenuItem{{Label: "Edit", Items: []desktop.MenuItem{{Label: "Copy Link", Shortcut: "mod+c", OnSelect: func() { chosen++ }}}}})
			cx.SetMainView(func(cx *core.Scope) core.Node { return core.Input{TestID: "field", DefaultValue: "abc"} })
		}, Environment(core.Environment{OS: os, Idiom: core.IdiomDesktop}))
		app.Focus("id:field")
		app.Select("id:field", 0, 3)
		app.Key("mod+c")
		if chosen != 1 || app.Clipboard != "" {
			t.Fatalf("%v: the item with mod+c ran %d times and the field copied %q", os, chosen, app.Clipboard)
		}
	}
}

// A focused field selects all with mod+a unless an application-menu item has that shortcut, in which case the item wins as it does on each desktop.
func TestFieldSelectAllKeyYieldsToApplicationMenu(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless field model")
	}
	for _, system := range []core.OS{core.OSMacOS, core.OSWindows, core.OSLinux} {
		t.Run(string(system), func(t *testing.T) {
			app := Start(t, func(cx *core.Scope) core.Node {
				return core.Input{TestID: "field", DefaultValue: "hello"}
			}, Environment(core.Environment{OS: system, Idiom: core.IdiomDesktop}))
			app.Focus("id:field")
			app.Key("mod+a")
			if s := app.Find("id:field").Selection; s.Start != 0 || s.End != 5 {
				t.Fatalf("without a menu item, mod+a selected %+v", s)
			}

			chosen := 0
			app = StartApp(t, func(cx *core.Scope) {
				desktop.SetApplicationMenu(cx, []desktop.MenuItem{{Label: "Edit", Items: []desktop.MenuItem{
					{Label: "Select Layer", Shortcut: "mod+a", OnSelect: func() { chosen++ }},
				}}})
				cx.SetMainView(func(cx *core.Scope) core.Node {
					return core.Input{TestID: "field", DefaultValue: "hello"}
				})
			}, Environment(core.Environment{OS: system, Idiom: core.IdiomDesktop}))
			app.Focus("id:field")
			app.Key("mod+a")
			if chosen != 1 {
				t.Fatalf("the menu item ran %d times, want once", chosen)
			}
			if s := app.Find("id:field").Selection; s.Start != 0 || s.End != 0 {
				t.Fatalf("the menu item left field selection %+v", s)
			}
		})
	}
}

// Another window the app opens runs beside the main one without disturbing the views steps reach, and each window's alerts are answered to the window that showed them, though both number them alike.
func TestOtherWindowsLeaveTheMainOnesViews(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	var answers []string
	app := Start(t, func(cx *core.Scope) core.Node {
		desktop.OpenWindow(cx, desktop.WindowOptions{Width: 200, Height: 100, Title: "Other"}, func(cx *core.Scope) core.Node {
			cx.ShowAlert(core.AlertConfig{Title: "Other?", Actions: []core.AlertAction{{Label: "Yes"}}}, func(i int) { answers = append(answers, "other") })
			return core.View{TestID: "other", Children: []core.Node{core.Text{Content: "other window"}}}
		})
		cx.ShowAlert(core.AlertConfig{Title: "Main?", Actions: []core.AlertAction{{Label: "Yes"}}}, func(i int) { answers = append(answers, "main") })
		return core.View{TestID: "main", Children: []core.Node{core.Text{Content: "main window"}}}
	}, Environment(core.Environment{OS: core.OSMacOS}), PendingAlerts())
	app.Find("id:main")
	app.Find("text:main window")
	if len(app.Query("id:other")) != 0 {
		t.Fatal("steps reach another window's views")
	}
	if len(app.Alerts()) != 2 {
		t.Fatalf("alerts %v, want both windows'", app.Alerts())
	}
	first := app.Alerts()[0].Title
	app.Answer(0)
	app.Answer(0)
	want := "main other"
	if first == "Other?" {
		want = "other main"
	}
	if got := strings.Join(answers, " "); got != want {
		t.Fatalf("answers reached %q, want %q", got, want)
	}
}

// StartApp runs the app's function once in the app's own scope, as nsgo.Run does: what it registers lives as long as the app, and with a desktop environment the view it sets shows in the main window, whose steps reach it, while its application menu chooses in that scope.
func TestStartAppKeepsTheAppsScope(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	for _, os := range []core.OS{core.OSMacOS, core.OSWindows, core.OSLinux} {
		var disposed bool
		var chosen, quits int
		var w *desktop.Window
		app := StartApp(t, func(cx *core.Scope) {
			cx.Cleanup(func() { disposed = true })
			desktop.SetApplicationMenu(cx, []desktop.MenuItem{{Label: "File", Items: []desktop.MenuItem{{Label: "New", OnSelect: func() { chosen++ }}}}})
			desktop.OnQuitRequest(cx, func(*desktop.QuitRequest) { quits++ })
			cx.SetMainView(func(cx *core.Scope) core.Node {
				w = desktop.CurrentWindow(cx)
				return core.Text{Content: "main", TestID: "main"}
			})
		}, Environment(core.Environment{OS: os}))
		if disposed {
			t.Fatalf("%v: the app's scope was disposed as it started", os)
		}
		if w == nil || app.root.MainWindow() != w {
			t.Fatalf("%v: the main view is not in the main window", os)
		}
		app.Find("id:main")
		app.ChooseMenu("File", "New")
		if chosen != 1 {
			t.Fatalf("%v: the menu item ran %d times", os, chosen)
		}
		app.Report(driver.Event{Kind: driver.EventQuitRequest, Count: 1})
		if quits != 1 {
			t.Fatalf("%v: the quit handler ran %d times", os, quits)
		}
	}
	// Elsewhere the main view fills the app's one surface.
	app := StartApp(t, func(cx *core.Scope) {
		cx.SetMainView(func(cx *core.Scope) core.Node {
			if desktop.CurrentWindow(cx) != nil {
				t.Error("a phone opened a window")
			}
			return core.Text{Content: "main", TestID: "main"}
		})
	}, Environment(core.Environment{OS: core.OSIOS}))
	app.Find("id:main")
}

// A key nothing else takes chooses the enabled application-menu item it is the shortcut of, in a submenu too and a role's own on a Mac; a disabled item takes nothing, and on a Mac, as AppKit stops at the first item with the key, leaves nothing to a later item with it, which Windows chooses; a NotInText one leaves the key to a focused field and keeps it from a later item with it on every desktop, and Linux offers the menu no key pressed without Control, Alt or the logo key while a field has the focus (#710).
func TestApplicationMenuShortcuts(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless menu shortcut path")
	}
	var chosen []string
	choose := func(name string) func() { return func() { chosen = append(chosen, name) } }
	menu := []desktop.MenuItem{{Label: "File", Items: []desktop.MenuItem{
		{Label: "Reload", Shortcut: "mod+r", OnSelect: choose("reload")},
		{Label: "Disabled", Shortcut: "mod+d", Disabled: true, OnSelect: choose("disabled")},
		{Label: "Refresh", Shortcut: "mod+i", When: core.MenuNotEditing, OnSelect: choose("refresh")},
		{Label: "Shadow", Shortcut: "mod+i", OnSelect: choose("shadow")},
		{Label: "Stop", Shortcut: "f5", OnSelect: choose("stop")},
		{Label: "Off", Shortcut: "mod+e", Disabled: true, OnSelect: choose("off")},
		{Label: "Export", Items: []desktop.MenuItem{
			{Label: "Again", Shortcut: "mod+e", OnSelect: choose("again")},
			{Label: "Grayed", Disabled: true, Items: []desktop.MenuItem{{Label: "Inside", Shortcut: "mod+g", OnSelect: choose("inside")}}},
		}},
		{Role: core.MenuCopy, OnSelect: choose("copy")},
	}}}
	start := func(os core.OS) *App {
		chosen = nil
		return StartApp(t, func(cx *core.Scope) {
			desktop.SetApplicationMenu(cx, menu)
			cx.SetMainView(func(cx *core.Scope) core.Node {
				return core.Input{TestID: "field", Style: core.Style{Width: core.Pt(100), Height: core.Pt(20)}}
			})
		}, Environment(core.Environment{OS: os, Idiom: core.IdiomDesktop}))
	}
	app := start(core.OSMacOS)
	for _, key := range []string{"mod+r", "meta+r", "mod+d", "mod+i", "mod+e", "mod+g", "mod+c"} {
		app.Key(key)
	}
	if want := []string{"reload", "reload", "refresh", "copy"}; !slices.Equal(chosen, want) {
		t.Errorf("on macOS: chose %q, want %q", chosen, want)
	}
	chosen = nil
	app.Focus("id:field")
	app.Key("mod+i")
	app.Key("mod+r")
	app.Key("f5")
	if want := []string{"reload", "stop"}; !slices.Equal(chosen, want) {
		t.Errorf("with the field focused: chose %q, want %q", chosen, want)
	}

	// Elsewhere the focused field handles the editing roles' keys, which the menu leaves alone, and a disabled item leaves its key to a later one.
	app = start(core.OSWindows)
	app.Key("ctrl+c")
	app.Key("ctrl+r")
	app.Key("ctrl+e")
	if want := []string{"reload", "again"}; !slices.Equal(chosen, want) {
		t.Errorf("on Windows: chose %q, want %q", chosen, want)
	}
	for _, os := range []core.OS{core.OSWindows, core.OSLinux} {
		app = start(os)
		app.Key("ctrl+i")
		app.Key("f5")
		app.Focus("id:field")
		app.Key("ctrl+i")
		app.Key("f5")
		want := []string{"refresh", "stop", "stop"}
		if os == core.OSLinux {
			want = []string{"refresh", "stop"}
		}
		if !slices.Equal(chosen, want) {
			t.Errorf("on %s: chose %q, want %q", os, chosen, want)
		}
	}

	// Phones show no application menu.
	app = start(core.OSAndroid)
	app.Key("ctrl+r")
	if len(chosen) != 0 {
		t.Errorf("on Android: chose %q", chosen)
	}
}

// A root view's menu takes its shortcut as an app's does (#399).
func TestApplicationMenuShortcutOfAView(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless menu shortcut path")
	}
	app := Start(t, func(cx *core.Scope) core.Node {
		n := core.NewSignal(cx, 0)
		desktop.SetApplicationMenu(cx, []desktop.MenuItem{{Label: "View", Items: []desktop.MenuItem{
			{Label: "Reload", Shortcut: "mod+r", OnSelect: func() { n.Set(n.Get() + 1) }},
		}}})
		return core.Text{ContentFunc: func() string { return fmt.Sprint(n.Get()) }}
	}, Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	app.Key("mod+r")
	app.WaitFor("text:1", time.Second)
}

// Given a root view, Windows and Linux open no window, as nsgo.Run does there, and macOS shows it in the main window.
func TestStartOpensWindowsWhereRunDoes(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	for os, want := range map[core.OS]bool{core.OSMacOS: true, core.OSWindows: false, core.OSLinux: false} {
		var w *desktop.Window
		app := Start(t, func(cx *core.Scope) core.Node {
			w = desktop.CurrentWindow(cx)
			return core.Text{Content: "main", TestID: "main"}
		}, Environment(core.Environment{OS: os}))
		app.Find("id:main")
		if (w != nil) != want {
			t.Errorf("%v: the root view's window is %v, want one %v", os, w, want)
		}
	}
}

// A window or a panel opened Transparent reports it, as on a compositing desktop, also after the test reports a new environment, and a window follows SetWindowTransparent; another panel does not report it, and a panel keeps what it opened with.
func TestTransparentSurfacesReportIt(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	var clear, plain, pet, main bool
	var card, mainScope *core.Scope
	app := StartApp(t, func(cx *core.Scope) {
		desktop.OpenPanel(cx, desktop.PanelOptions{Width: 300, Height: 400, FitContent: true, Transparent: true}, func(cx *core.Scope) core.Node {
			card = cx
			cx.Effect(func() { clear = desktop.Transparent(cx) })
			return core.Text{Content: "card"}
		})
		desktop.OpenPanel(cx, desktop.PanelOptions{}, func(cx *core.Scope) core.Node {
			cx.Effect(func() { plain = desktop.Transparent(cx) })
			return core.Text{Content: "plain"}
		})
		desktop.OpenWindow(cx, desktop.WindowOptions{Width: 120, Height: 90, Transparent: true}, func(cx *core.Scope) core.Node {
			cx.Effect(func() { pet = desktop.Transparent(cx) })
			return core.View{}
		})
		cx.SetMainView(func(cx *core.Scope) core.Node {
			mainScope = cx
			cx.Effect(func() { main = desktop.Transparent(cx) })
			return core.Text{Content: "main", TestID: "main"}
		})
	}, Environment(core.Environment{OS: core.OSMacOS}))
	app.Find("id:main")
	if !clear || plain || !pet || main {
		t.Fatalf("the transparent panel reports %v, the other %v, the transparent window %v and the main window %v", clear, plain, pet, main)
	}
	desktop.SetWindowTransparent(mainScope, true)
	desktop.SetWindowTransparent(card, false)
	app.Settle()
	if !main || !clear {
		t.Fatalf("the main window made transparent reports %v, the panel asked to change %v", main, clear)
	}
	app.SetEnvironment(core.Environment{OS: core.OSMacOS, Dark: true})
	if !clear || plain || !pet || !main {
		t.Fatalf("after a new environment the transparent panel reports %v, the other %v, the transparent window %v and the main window %v", clear, plain, pet, main)
	}
	desktop.SetWindowTransparent(mainScope, false)
	app.Settle()
	if main {
		t.Fatal("the main window framed again reports it is transparent")
	}
}

// Steps act on the window SetMainView opens, not on one the app opened before it, and on the next one once it closes and the app shows its view again.
func TestStepsFollowTheMainWindow(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	var show func()
	app := StartApp(t, func(cx *core.Scope) {
		desktop.OpenWindow(cx, desktop.WindowOptions{Width: 200, Height: 100, Title: "Settings"}, func(cx *core.Scope) core.Node {
			return core.Text{Content: "settings", TestID: "settings"}
		})
		show = func() {
			cx.SetMainView(func(cx *core.Scope) core.Node { return core.Text{Content: "main", TestID: "main"} })
		}
		show()
	}, Environment(core.Environment{OS: core.OSMacOS}))
	app.Find("id:main")
	if len(app.Query("id:settings")) != 0 {
		t.Fatal("steps reach the window the app opened first")
	}
	if !app.CloseWindow() {
		t.Fatal("the main window did not close")
	}
	app.Post(show)
	app.Settle()
	app.Find("id:main")
}

// SetWindowEnvironment changes one window's environment, the main window's for "" or one the app opened by its title, while the app's own surface and its other windows keep the app's; the main window's Minimize changes only it; SetEnvironment sets every window's again.
func TestOneWindowsEnvironment(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	var app, main, settings *core.Env
	a := StartApp(t, func(cx *core.Scope) {
		app = cx.Env()
		desktop.SetApplicationMenu(cx, []desktop.MenuItem{{Label: "Window", Items: []desktop.MenuItem{{Role: core.MenuMinimize}}}})
		desktop.OpenWindow(cx, desktop.WindowOptions{Width: 200, Height: 100, Title: "Settings"}, func(cx *core.Scope) core.Node {
			settings = cx.Env()
			return core.Text{Content: "settings"}
		})
		cx.SetMainView(func(cx *core.Scope) core.Node {
			main = cx.Env()
			return core.Text{Content: "main", TestID: "main"}
		})
	}, Environment(core.Environment{OS: core.OSMacOS}))
	a.Find("id:main")

	a.SetWindowEnvironment("", func(env *core.Environment) { env.Lifecycle, env.Minimized = core.LifecycleBackground, true })
	if main.Lifecycle() != core.LifecycleBackground || !main.Minimized() {
		t.Fatalf("the main window is %v, minimized %v; want it in the background", main.Lifecycle(), main.Minimized())
	}
	if settings.Lifecycle() != core.LifecycleActive || settings.Minimized() || app.Lifecycle() != core.LifecycleActive {
		t.Fatalf("settings %v, minimized %v, app %v; want them active while only the main window is away", settings.Lifecycle(), settings.Minimized(), app.Lifecycle())
	}

	a.SetWindowEnvironment("Settings", func(env *core.Environment) { env.Dark = true })
	if !settings.Dark() || main.Dark() || main.Lifecycle() != core.LifecycleBackground {
		t.Fatalf("settings dark %v, main dark %v, main %v; want the change on Settings alone, the main window's kept", settings.Dark(), main.Dark(), main.Lifecycle())
	}

	a.SetEnvironment(core.Environment{OS: core.OSMacOS})
	if main.Lifecycle() != core.LifecycleActive || main.Minimized() || settings.Dark() {
		t.Fatalf("after SetEnvironment main %v, minimized %v, settings dark %v; want every window's the app's", main.Lifecycle(), main.Minimized(), settings.Dark())
	}

	a.ChooseMenu("Window", "Minimize")
	if !main.Minimized() || main.Lifecycle() != core.LifecycleBackground || settings.Minimized() || settings.Lifecycle() != core.LifecycleActive {
		t.Fatalf("Minimize left the main window minimized %v and %v, Settings minimized %v and %v; want the main window alone in the background", main.Minimized(), main.Lifecycle(), settings.Minimized(), settings.Lifecycle())
	}
}

// A window the app opens transparent, shadowless and off the taskbar is recorded so, and the main window's changes to them reach Window.
func TestTransparentWindowsAreRecorded(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	var main *core.Scope
	app := StartApp(t, func(cx *core.Scope) {
		desktop.OpenWindow(cx, desktop.WindowOptions{Width: 120, Height: 90, Transparent: true, NoShadow: true, SkipTaskbar: true},
			func(cx *core.Scope) core.Node { return core.View{} })
		cx.SetMainView(func(cx *core.Scope) core.Node {
			main = cx
			return core.Text{Content: "main", TestID: "main"}
		})
	}, Environment(core.Environment{OS: core.OSWindows}))
	if len(app.Opened) != 2 {
		t.Fatalf("opened %+v", app.Opened)
	}
	if pet := app.Opened[0]; !pet.Transparent || !pet.NoShadow || !pet.SkipTaskbar || pet.Width != 120 {
		t.Fatalf("the pet's window opened as %+v", pet)
	}
	if w := app.Opened[1]; w.Transparent || w.NoShadow || w.SkipTaskbar {
		t.Fatalf("the main window opened as %+v", w)
	}
	desktop.SetWindowTransparent(main, true)
	desktop.SetWindowShadow(main, false)
	desktop.SetSkipTaskbar(main, true)
	app.Settle()
	want := []driver.WindowCommand{{Action: driver.WindowSetTransparent, On: true}, {Action: driver.WindowSetShadow},
		{Action: driver.WindowSetSkipTaskbar, On: true}}
	if !slices.Equal(app.Window, want) {
		t.Fatalf("the main window was sent %+v; want %+v", app.Window, want)
	}
}

// A window tells whether it can be transparent and skip the taskbar as the environment says: neither on Wayland without a compositor's alpha, both on Windows.
func TestTransparentWindowCapabilities(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	app := StartApp(t, func(cx *core.Scope) {
		cx.SetMainView(func(cx *core.Scope) core.Node {
			text := func(s string) func(*core.Scope) core.Node {
				return func(*core.Scope) core.Node { return core.Text{Content: s} }
			}
			return core.View{Children: []core.Node{
				core.Show{When: func() bool { return desktop.CanMakeWindowsTransparent(cx) }, Then: text("see-through"), Else: text("opaque")},
				core.Show{When: func() bool { return desktop.CanSkipTaskbar(cx) }, Then: text("off the taskbar"), Else: text("on the taskbar")},
			}}
		})
	}, Environment(core.Environment{OS: core.OSLinux, Idiom: core.IdiomDesktop}))
	app.Find("text:opaque")
	app.Find("text:on the taskbar")
	app.SetEnvironment(core.Environment{OS: core.OSWindows, Idiom: core.IdiomDesktop, CanMakeWindowsTransparent: true, CanSkipTaskbar: true})
	app.Find("text:see-through")
	app.Find("text:off the taskbar")
}

func TestMenuShortcutSpellingMatchesDesktop(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	var got string
	app := StartApp(t, func(cx *core.Scope) {
		desktop.SetApplicationMenu(cx, []desktop.MenuItem{{Label: "View", Items: []desktop.MenuItem{
			{Label: "Ctrl", Shortcut: "ctrl+r", OnSelect: func() { got = "ctrl" }},
			{Label: "Mod", Shortcut: "mod+r", OnSelect: func() { got = "mod" }},
		}}})
		cx.SetMainView(func(*core.Scope) core.Node { return core.Text{Content: "main"} })
	}, Environment(core.Environment{OS: core.OSWindows, Idiom: core.IdiomDesktop}))
	app.Key("ctrl+r")
	if got != "mod" {
		t.Fatalf("ctrl+r chose %q, want mod spelling", got)
	}
}

// Another window's shortcuts and menu bar leave the main window's, which Key and ChooseMenu reach, and a context menu any window shows chooses on that window.
func TestOtherWindowsKeepTheirShortcutsAndMenus(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	var got []string
	app := StartApp(t, func(cx *core.Scope) {
		cx.SetMainView(func(cx *core.Scope) core.Node {
			cx.OnKey("mod+s", func() { got = append(got, "save") })
			desktop.SetApplicationMenu(cx, []desktop.MenuItem{{Label: "File", Items: []desktop.MenuItem{{Label: "New", OnSelect: func() { got = append(got, "new") }}}}})
			return core.Text{Content: "main", TestID: "main"}
		})
		desktop.OpenWindow(cx, desktop.WindowOptions{Width: 200, Height: 100, Title: "Other"}, func(cx *core.Scope) core.Node {
			cx.OnKey("escape", func() { got = append(got, "escape") })
			desktop.SetApplicationMenu(cx, []desktop.MenuItem{{Label: "Other"}})
			desktop.PopupMenu(cx, []desktop.MenuItem{{Label: "Copy", OnSelect: func() { got = append(got, "copy") }}}, core.Point{}, func() { got = append(got, "closed") })
			return core.Text{Content: "other"}
		})
	}, Environment(core.Environment{OS: core.OSWindows}))
	if strings.Join(app.Keymap, " ") != "mod+s" {
		t.Fatalf("keymap %v, want the main window's", app.Keymap)
	}
	app.Key("mod+s")
	app.ChooseMenu("File", "New")
	app.ChoosePopup("Copy")
	if want := "save new copy closed"; strings.Join(got, " ") != want {
		t.Fatalf("got %q, want %q", strings.Join(got, " "), want)
	}
}

// On macOS the application menu is the one the main window or the app's own surface sent last, in either order, as the core reads it for YieldsToMenu bindings: Key gives the binding a key only an earlier menu has, and once the last surface restores the platform's menu, none of the earlier one's items take a key.
func TestMacOSApplicationMenuFollowsTheLastSurface(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless macOS menu model")
	}
	var got []string
	choose := func(label, key string) []desktop.MenuItem {
		return []desktop.MenuItem{{Label: label, Shortcut: key, OnSelect: func() { got = append(got, label) }}}
	}
	mac := Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop})
	app := StartApp(t, func(cx *core.Scope) {
		desktop.SetApplicationMenu(cx, choose("app", "mod+r"))
		cx.SetMainView(func(cx *core.Scope) core.Node {
			desktop.SetApplicationMenu(cx, choose("main", "mod+m"))
			cx.OnKeyWith("mod+r", core.KeyOptions{YieldsToMenu: true}, func() { got = append(got, "binding") })
			return core.Text{Content: "main"}
		})
	}, mac)
	app.ChooseMenu("main")
	app.Key("mod+r")
	if want := "main binding"; strings.Join(got, " ") != want {
		t.Fatalf("main window last: got %q, want %q", strings.Join(got, " "), want)
	}

	got = nil
	var appScope *core.Scope
	app = StartApp(t, func(cx *core.Scope) {
		appScope = cx
		cx.SetMainView(func(cx *core.Scope) core.Node {
			desktop.SetApplicationMenu(cx, choose("main", "mod+r"))
			cx.OnKeyWith("mod+r", core.KeyOptions{YieldsToMenu: true}, func() { got = append(got, "binding") })
			return core.Text{Content: "main"}
		})
	}, mac)
	app.Post(func() { desktop.SetApplicationMenu(appScope, choose("app", "mod+a")) })
	app.Settle()
	app.ChooseMenu("app")
	app.Key("mod+r")
	if want := "app binding"; strings.Join(got, " ") != want {
		t.Fatalf("app surface last: got %q, want %q", strings.Join(got, " "), want)
	}

	got = nil
	app.Post(func() { desktop.SetApplicationMenu(appScope, nil) })
	app.Settle()
	if app.Menu != nil {
		t.Fatalf("after the app surface restored the platform's menu, Menu is %v", app.Menu)
	}
	app.Key("mod+r")
	if want := "binding"; strings.Join(got, " ") != want {
		t.Fatalf("platform's menu last: got %q, want %q", strings.Join(got, " "), want)
	}
}

// The app icon's menu is the app's, the one a surface set last: ChooseDockMenu chooses on that surface, though the surfaces number their items alike, and the menu goes with the window that set it.
func TestDockMenuIsTheApps(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	var got []string
	var other *desktop.Window
	var open func()
	app := StartApp(t, func(cx *core.Scope) {
		desktop.SetDockMenu(cx, []desktop.MenuItem{{Label: "New Window", OnSelect: func() { got = append(got, "app") }}})
		cx.SetMainView(func(cx *core.Scope) core.Node {
			desktop.SetApplicationMenu(cx, []desktop.MenuItem{{Label: "File", Items: []desktop.MenuItem{{Label: "New", OnSelect: func() { got = append(got, "menu") }}}}})
			return core.Text{Content: "main", TestID: "main"}
		})
		open = func() {
			other = desktop.OpenWindow(cx, desktop.WindowOptions{Width: 200, Height: 100, Title: "Library"}, func(cx *core.Scope) core.Node {
				desktop.SetDockMenu(cx, []desktop.MenuItem{{Label: "Next Library", OnSelect: func() { got = append(got, "library") }}})
				return core.Text{Content: "library"}
			})
		}
	}, Environment(core.Environment{OS: core.OSMacOS}))
	app.Find("id:main")
	app.ChooseDockMenu("New Window")
	app.Post(open)
	app.Settle()
	if len(app.DockMenu) != 1 || app.DockMenu[0].Label != "Next Library" {
		t.Fatalf("dock menu %+v, want the library window's", app.DockMenu)
	}
	app.ChooseDockMenu("Next Library")
	app.Post(other.Close)
	app.Settle()
	if app.DockMenu != nil {
		t.Fatalf("dock menu %+v after its window closed", app.DockMenu)
	}
	if want := "app library"; strings.Join(got, " ") != want {
		t.Fatalf("got %q, want %q", strings.Join(got, " "), want)
	}
}

// Answering an alert by its place, as the automation does, answers it to the surface that showed it, though the surfaces number their alerts alike.
func TestAnswerAtReachesTheAlertsSurface(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	var answers []string
	app := StartApp(t, func(cx *core.Scope) {
		cx.SetMainView(func(cx *core.Scope) core.Node {
			cx.ShowAlert(core.AlertConfig{Title: "Main?", Actions: []core.AlertAction{{Label: "Yes"}}}, func(int) { answers = append(answers, "main") })
			return core.Text{Content: "main"}
		})
		desktop.OpenWindow(cx, desktop.WindowOptions{Width: 200, Height: 100}, func(cx *core.Scope) core.Node {
			cx.ShowAlert(core.AlertConfig{Title: "Other?", Actions: []core.AlertAction{{Label: "Yes"}}}, func(int) { answers = append(answers, "other") })
			return core.Text{Content: "other"}
		})
	}, Environment(core.Environment{OS: core.OSLinux}), PendingAlerts())
	alerts := app.Alerts()
	if len(alerts) != 2 || alerts[0].ID != alerts[1].ID {
		t.Fatalf("alerts %+v, want two numbered alike", alerts)
	}
	want := map[string]string{"Main?": "main", "Other?": "other"}[alerts[1].Title]
	app.answerAt(1, 0)
	if len(answers) != 1 || answers[0] != want {
		t.Fatalf("answering %q reached %v", alerts[1].Title, answers)
	}
}

// The window commands a test reads hold where the app lets the pointer through its window.
func TestWindowPointerEventsReachTheWindow(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.View{TestID: "pass", Style: core.Style{Width: core.Pt(100), Height: core.Pt(100)}, OnPress: func(core.PressEvent) { desktop.SetWindowPointerEvents(cx, desktop.WindowPointerPainted) }}
	}, Environment(core.Environment{OS: core.OSMacOS}))
	app.Press("id:pass")
	if n := len(app.Window); n == 0 || app.Window[n-1].Action != driver.WindowSetPointerEvents ||
		app.Window[n-1].PointerEvents != desktop.WindowPointerPainted {
		t.Fatalf("the window was sent %+v", app.Window)
	}
}

// OpenLinks delivers links as the system does: a handler hears them, one that registers later hears those that came before it, and the app routes them where it chooses.
func TestOpenLinksReachTheApp(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless model")
	}
	app := Start(t, func(cx *core.Scope) core.Node {
		shown := core.NewSignal(cx, "home")
		desktop.OnOpenLinks(cx, func(links []string) {
			if u, err := url.Parse(links[0]); err == nil && u.Scheme == "umapp" {
				shown.Set(u.Host)
			}
		})
		return core.Text{ContentFunc: shown.Get, TestID: "page"}
	}, Environment(core.Environment{OS: core.OSWindows}))
	app.OpenLinks("umapp://settings/desktop")
	if got := app.Find("id:page").Text; got != "settings" {
		t.Fatalf("page %q after a link, want settings", got)
	}
	app.OpenLinks("https://example.com")
	if got := app.Find("id:page").Text; got != "settings" {
		t.Fatalf("page %q after another scheme's link, want settings", got)
	}
}

func TestDiskSpace(t *testing.T) {
	space := core.DiskSpace{Total: 1 << 41, Available: 1 << 35, Opportunistic: 1 << 33}
	for _, tc := range []struct {
		name string
		opts []Option
		want string
	}{
		{"pinned", []Option{DiskSpace(space, nil)}, fmt.Sprint(space)},
		{"undeclared", []Option{DiskSpace(core.DiskSpace{}, core.ErrNotDeclared)}, core.ErrNotDeclared.Error()},
		{"unavailable", nil, core.ErrDiskSpaceUnavailable.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := Start(t, func(cx *core.Scope) core.Node {
				status := core.NewSignal(cx, "waiting")
				cx.DiskSpace(func(space core.DiskSpace, err error) {
					if err != nil {
						status.Set(err.Error())
					} else {
						status.Set(fmt.Sprint(space))
					}
				})
				if status.Get() != "waiting" {
					panic("DiskSpace answered inline")
				}
				return core.Text{ContentFunc: status.Get}
			}, tc.opts...)
			app.Find("text:" + tc.want)
		})
	}
}

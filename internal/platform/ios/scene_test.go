//go:build ios && nsgotest

package ios_test

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/ios"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestSceneReconnectKeepsTheEngine disconnects and reconnects the app's one scene, as UIKit may with a scene in the background, and checks that the running engine shows in the new window with its state, rather than a second engine starting the app over it (#1030).
func TestSceneReconnectKeepsTheEngine(t *testing.T) {
	const wait = 5 * time.Second
	var built, presses atomic.Int32
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		built.Add(1)
		count := core.NewSignal(cx, 0)
		return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
			core.View{TestID: "add", Style: core.Style{Width: core.Pt(160), Height: core.Pt(60), Background: core.RGB(0x3366cc)},
				OnPress: func(core.PressEvent) {
					count.Update(func(n int) int { return n + 1 })
					presses.Store(int32(count.Get()))
				}},
			core.Text{TestID: "count", ContentFunc: func() string { return fmt.Sprint(count.Get()) }},
		}}
	})
	app.WaitFor("id:add", wait)
	app.Press("id:add")
	app.WaitUntil(wait, func() bool { return presses.Load() == 1 })
	for round := range 2 {
		kept, shown := ios.SceneReconnectForTest()
		if !kept {
			t.Fatalf("round %d: the reconnected scene started another engine", round)
		}
		if !shown {
			t.Fatalf("round %d: the surface does not show in the reconnected scene's key window", round)
		}
		if n := built.Load(); n != 1 {
			t.Fatalf("round %d: the app's root was built %d times, want once", round, n)
		}
		// The press reaches the app through the new window, with the count it had.
		app.Press("id:add")
		app.WaitUntil(wait, func() bool { return presses.Load() == int32(round+2) })
	}
}

// sceneCounter is an app whose root builds once a scene, each build counting its own presses of "add".
type sceneCounter struct {
	built   atomic.Int32
	presses [4]atomic.Int32
	// look, when set, gives build n more of the scene's own settings.
	look func(cx *core.Scope, n int32) []core.Node
}

func (a *sceneCounter) root(cx *core.Scope) core.Node {
	n := a.built.Add(1)
	count := core.NewSignal(cx, 0)
	children := []core.Node{
		core.View{TestID: "add", Style: core.Style{Width: core.Pt(160), Height: core.Pt(60), Background: core.RGB(0x3366cc)},
			OnPress: func(core.PressEvent) {
				count.Update(func(c int) int { return c + 1 })
				if int(n) < len(a.presses) {
					a.presses[n].Store(int32(count.Get()))
				}
			}},
		core.Text{TestID: "count", ContentFunc: func() string { return fmt.Sprint(count.Get()) }},
	}
	if a.look != nil {
		children = append(children, a.look(cx, n)...)
	}
	return core.View{Style: core.Style{FlexGrow: 1, Padding: core.Edges{Top: 80}}, Children: children}
}

// counts returns how many times each build's "add" was pressed, the first build's first.
func (a *sceneCounter) counts() [3]int32 {
	return [3]int32{a.presses[1].Load(), a.presses[2].Load(), a.presses[3].Load()}
}

const sceneWait = 15 * time.Second

// openSecondScene starts a, opens a second scene beside the first, and returns both, the first's first; it skips where UIKit shows one scene at a time. The scenes it opens close as the test ends, so iPadOS connects no leftover session as the next test launches.
func openSecondScene(t *testing.T, a *sceneCounter) (*nsgotest.App, ios.SceneForTest, ios.SceneForTest) {
	t.Helper()
	if !ios.MultipleScenesForTest() {
		t.Skip("UIKit shows the app in one scene at a time here: multiple scenes need an iPad")
	}
	app := nsgotest.Start(t, a.root)
	app.WaitFor("id:add", sceneWait)
	if err := ios.CloseOtherScenesForTest(sceneWait); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := ios.CloseOtherScenesForTest(sceneWait); err != nil {
			t.Error(err)
		}
	})
	second, err := ios.OpenSceneForTest(sceneWait)
	if err != nil {
		t.Fatal(err)
	}
	scenes := ios.ScenesForTest()
	if len(scenes) != 2 {
		t.Fatalf("%d scenes connected, want 2: %+v", len(scenes), scenes)
	}
	return app, scenes[0], second
}

// TestScenesShowTreesOfTheirOwn opens a second scene, as the user opening another window of the app on an iPad does, and checks that each scene shows a surface of its own, in its own window, running a tree of its own on the one engine, and that a press in each reaches its own tree.
func TestScenesShowTreesOfTheirOwn(t *testing.T) {
	a := &sceneCounter{}
	app, first, second := openSecondScene(t, a)
	if first.Scene == second.Scene || first.Window == 0 || second.Window == 0 || first.Window == second.Window {
		t.Fatalf("the scenes share a window: %+v, %+v", first, second)
	}
	if !first.Surface || !second.Surface || first.View == second.View {
		t.Fatalf("the scenes do not each show a surface of their own: %+v, %+v", first, second)
	}
	if first.Tree != 0 || second.Tree == 0 || !second.Attached {
		t.Fatalf("the second scene runs no tree of its own: %+v, %+v", first, second)
	}
	if n := a.built.Load(); n != 2 {
		t.Fatalf("the root was built %d times, want once a scene", n)
	}
	if err := ios.PressInSceneForTest(second, "add"); err != nil {
		t.Fatal(err)
	}
	app.WaitUntil(sceneWait, func() bool { return a.counts() == [3]int32{0, 1, 0} })
	if err := ios.ActivateSceneForTest(first, sceneWait); err != nil {
		t.Fatal(err)
	}
	app.Press("id:add")
	app.WaitUntil(sceneWait, func() bool { return a.counts() == [3]int32{1, 1, 0} })
	if err := ios.ActivateSceneForTest(second, sceneWait); err != nil {
		t.Fatal(err)
	}
	if err := ios.PressInSceneForTest(second.Refresh(), "add"); err != nil {
		t.Fatal(err)
	}
	app.WaitUntil(sceneWait, func() bool { return a.counts() == [3]int32{1, 2, 0} })
}

// TestSceneDisconnectsWhileTheOtherRuns disconnects the second scene, as UIKit releases a scene in the background, while the first keeps running, connects it again with its tree and state kept, and then closes it, as the user closing that window does, which ends its tree alone.
func TestSceneDisconnectsWhileTheOtherRuns(t *testing.T) {
	a := &sceneCounter{}
	app, first, second := openSecondScene(t, a)
	if err := ios.PressInSceneForTest(second, "add"); err != nil {
		t.Fatal(err)
	}
	app.WaitUntil(sceneWait, func() bool { return a.counts() == [3]int32{0, 1, 0} })
	reconnect := ios.DisconnectSceneForTest(second)
	if got := second.Refresh(); got.Window != 0 && got.View == second.View {
		t.Fatalf("the disconnected scene still shows its surface: %+v", got)
	}
	if err := ios.ActivateSceneForTest(first, sceneWait); err != nil {
		t.Fatal(err)
	}
	if got := first.Refresh(); got.View != first.View || got.Tree != 0 || !got.Attached {
		t.Fatalf("the first scene changed as the second disconnected: %+v, was %+v", got, first)
	}
	app.Press("id:add")
	app.WaitUntil(sceneWait, func() bool { return a.counts() == [3]int32{1, 1, 0} })

	reconnect()
	again := second.Refresh()
	if again.View != second.View || again.Tree != second.Tree || !again.Attached || again.Window == 0 {
		t.Fatalf("the scene reconnected without its surface and tree: %+v, was %+v", again, second)
	}
	if n := a.built.Load(); n != 2 {
		t.Fatalf("the root was built %d times, want once a scene", n)
	}
	if err := ios.ActivateSceneForTest(again, sceneWait); err != nil {
		t.Fatal(err)
	}
	if err := ios.PressInSceneForTest(again, "add"); err != nil {
		t.Fatal(err)
	}
	// The press counts on from the state the scene's tree kept.
	app.WaitUntil(sceneWait, func() bool { return a.counts() == [3]int32{1, 2, 0} })

	if err := ios.CloseSceneForTest(again, sceneWait); err != nil {
		t.Fatal(err)
	}
	if scenes := ios.ScenesForTest(); len(scenes) != 1 || scenes[0].View != first.View {
		t.Fatalf("closing the second scene left %+v", scenes)
	}
	if err := ios.ActivateSceneForTest(first, sceneWait); err != nil {
		t.Fatal(err)
	}
	app.Press("id:add")
	app.WaitUntil(sceneWait, func() bool { return a.counts() == [3]int32{2, 2, 0} })
	// A window the user opens next shows a new tree.
	third, err := ios.OpenSceneForTest(sceneWait)
	if err != nil {
		t.Fatal(err)
	}
	if third.Tree == 0 || third.Tree == second.Tree || a.built.Load() != 3 {
		t.Fatalf("the next scene shows %+v after %d builds", third, a.built.Load())
	}
}

// TestSceneStatusBarAndOrientationsArePerScene gives each scene's tree its own status bar, and the second a modal that keeps to portrait, and checks that each scene's root view controller answers UIKit with its own.
func TestSceneStatusBarAndOrientationsArePerScene(t *testing.T) {
	a := &sceneCounter{look: func(cx *core.Scope, n int32) []core.Node {
		if n == 1 {
			cx.SetStatusBar(core.StatusBarSetting{Foreground: core.StatusBarDark})
			return nil
		}
		cx.SetStatusBar(core.StatusBarSetting{Foreground: core.StatusBarLight})
		return []core.Node{core.Modal{TestID: "portrait", Presentation: core.ModalPageSheet,
			IOS: core.IOSModalProps{SupportedOrientations: core.OrientationPortrait}}}
	}}
	app, first, second := openSecondScene(t, a)
	const darkContent, lightContent, portrait = 3, 1, 1 << 1
	app.WaitUntil(sceneWait, func() bool {
		f, s := first.Refresh(), second.Refresh()
		return f.StatusBar == darkContent && s.StatusBar == lightContent && s.Orientations == portrait && f.Orientations != portrait
	})
}

// TestSceneKeyboardInsetFollowsFocus focuses fields in both scenes and checks the software keyboard's inset in each tree.
func TestSceneKeyboardInsetFollowsFocus(t *testing.T) {
	var heights [2]atomic.Int32
	a := &sceneCounter{look: func(cx *core.Scope, n int32) []core.Node {
		if n <= int32(len(heights)) {
			cx.Effect(func() { heights[n-1].Store(int32(cx.Env().KeyboardHeight())) })
		}
		return []core.Node{core.Input{TestID: "keyboard", Style: core.Style{Width: core.Pt(200), Height: core.Pt(44)}}}
	}}
	_, first, second := openSecondScene(t, a)
	// The keys an earlier test pressed leave the simulator with a hardware keyboard, which keeps the software one hidden.
	if err := ios.DetachHardwareKeyboardForTest(); err != nil {
		t.Fatal(err)
	}
	for i, sc := range []ios.SceneForTest{second, first, second} {
		if err := ios.ActivateSceneForTest(sc, sceneWait); err != nil {
			t.Fatal(err)
		}
		if err := ios.PressInSceneForTest(sc.Refresh(), "keyboard"); err != nil {
			t.Fatal(err)
		}
		owner := 1 - i%2
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(25 * time.Millisecond) {
			if heights[owner].Load() > 0 && heights[1-owner].Load() == 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("focus in scene %d gave heights [%d %d]: %+v", owner, heights[0].Load(), heights[1].Load(), ios.ScenesForTest())
			}
		}
	}
}

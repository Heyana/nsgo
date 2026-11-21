//go:build ios && nsgotest

package ios

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

// SceneReconnectForTest disconnects the app's scene and connects it again through the scene delegate's methods, as UIKit does with a scene it releases in the background and brings back as the user returns. It reports whether the surface kept its engine and whether its view shows in a new key window of the scene.
func SceneReconnectForTest() (kept, shown bool) {
	onMainSync(func() {
		old, e := runner.window.Send(sel("retain")), runner.engine
		defer old.Send(selRelease)
		scene := old.Send(sel("windowScene"))
		delegate := scene.Send(sel("delegate"))
		if objc.Send[bool](delegate, sel("respondsToSelector:"), sel("sceneDidDisconnect:")) {
			delegate.Send(sel("sceneDidDisconnect:"), scene)
		}
		delegate.Send(sel("scene:willConnectToSession:options:"), scene, scene.Send(sel("session")), objc.ID(0))
		kept = runner.engine == e
		shown = runner.window != 0 && runner.window != old && runner.uiWindow() == runner.window && objc.Send[bool](runner.window, sel("isKeyWindow"))
	})
	return kept, shown
}

// SystemGestureEdgesForTest asks UIKit's root controller for its preference.
func SystemGestureEdgesForTest() uint {
	var edges uint
	onMainSync(func() {
		edges = objc.Send[uint](runner.controller, sel("preferredScreenEdgesDeferringSystemGestures"))
	})
	return edges
}

// SceneForTest is one of the scenes UIKit connected the app's, as UIKit and the engine have it.
type SceneForTest struct {
	// Scene is the UIWindowScene, Window its key window and View the view its root view controller shows, 0 for none.
	Scene, Window, View objc.ID
	// Surface says the view is a surface of the app's, Tree is the engine's ID of the tree it shows, 0 for the first, and Attached whether an engine runs that tree.
	Surface, Attached bool
	Tree              uint32
	// StatusBar and Orientations are what the root view controller answers UIKit: its UIStatusBarStyle and its UIInterfaceOrientationMask.
	StatusBar    int
	Orientations uint
	// Foreground says the scene is in the foreground, active or not.
	Foreground bool
}

// MultipleScenesForTest reports whether UIKit can show the app in several scenes at once: an iPad's, when the app declares them.
func MultipleScenesForTest() bool {
	var ok bool
	onMainSync(func() {
		const pad = 1
		ok = multipleScenes() && objc.Send[int](objc.ID(apple.Class("UIDevice")).Send(sel("currentDevice")), sel("userInterfaceIdiom")) == pad
	})
	return ok
}

// ScenesForTest returns the scenes UIKit connected, the one with the engine's first tree first and the rest by their trees.
func ScenesForTest() []SceneForTest {
	var out []SceneForTest
	onMainSync(func() {
		app := objc.ID(apple.Class("UIApplication")).Send(sel("sharedApplication"))
		apple.ForEach(app.Send(sel("connectedScenes")), func(scene objc.ID) { out = append(out, sceneState(scene)) })
	})
	slices.SortFunc(out, func(a, b SceneForTest) int {
		if a.Surface != b.Surface {
			if a.Surface {
				return -1
			}
			return 1
		}
		return int(a.Tree) - int(b.Tree)
	})
	return out
}

// Refresh returns sc as UIKit and the engine have it now.
func (sc SceneForTest) Refresh() SceneForTest {
	var out SceneForTest
	onMainSync(func() { out = sceneState(sc.Scene) })
	return out
}

func sceneState(scene objc.ID) SceneForTest {
	sc := SceneForTest{Scene: scene}
	state := objc.Send[int](scene, sel("activationState"))
	sc.Foreground = state == 0 || state == 1
	apple.ForEach(scene.Send(sel("windows")), func(w objc.ID) {
		if objc.Send[bool](w, sel("isKeyWindow")) || sc.Window == 0 && !objc.Send[bool](w, sel("isHidden")) {
			sc.Window = w
		}
	})
	if sc.Window == 0 {
		return sc
	}
	ctrl := sc.Window.Send(sel("rootViewController"))
	if ctrl == 0 {
		return sc
	}
	sc.View = ctrl.Send(sel("viewIfLoaded"))
	sc.StatusBar = objc.Send[int](ctrl, sel("preferredStatusBarStyle"))
	sc.Orientations = objc.Send[uint](ctrl, sel("supportedInterfaceOrientations"))
	v := views[sc.View]
	if v == nil || v.surface.native != sc.View {
		return sc
	}
	s := v.surface
	sc.Surface, sc.Tree, sc.Attached = true, s.tree, s.engine != nil
	return sc
}

// waitScenes waits up to wait for done to hold of the connected scenes.
func waitScenes(wait time.Duration, what string, done func([]SceneForTest) bool) error {
	deadline := time.Now().Add(wait)
	for {
		if done(ScenesForTest()) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s within %v: %+v", what, wait, ScenesForTest())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// requestScene asks UIKit to activate session, a new one when it is 0, as the user opening a window of the app, or bringing one to the front, does.
func requestScene(session objc.ID) error {
	failed := make(chan string, 1)
	onMainSync(func() {
		handler := objcblock.New(func(_ objc.Block, err objc.ID) {
			failed <- apple.GoString(err.Send(sel("localizedDescription")))
		})
		app := objc.ID(apple.Class("UIApplication")).Send(sel("sharedApplication"))
		app.Send(sel("requestSceneSessionActivation:userActivity:options:errorHandler:"), session, objc.ID(0), objc.ID(0), handler)
		handler.Release()
	})
	select {
	case msg := <-failed:
		return errors.New(msg)
	case <-time.After(300 * time.Millisecond):
		return nil
	}
}

// OpenSceneForTest asks UIKit for a new scene session, as the user opening another window of the app does, and waits up to wait for it to show a tree of its own, attached to the engine, in the foreground. It returns that scene.
func OpenSceneForTest(wait time.Duration) (SceneForTest, error) {
	before := ScenesForTest()
	if err := requestScene(0); err != nil {
		return SceneForTest{}, err
	}
	var opened SceneForTest
	err := waitScenes(wait, "no new scene showed a tree", func(scs []SceneForTest) bool {
		for _, sc := range scs {
			if !slices.ContainsFunc(before, func(b SceneForTest) bool { return b.Scene == sc.Scene }) && sc.Attached && sc.Foreground {
				opened = sc
				return true
			}
		}
		return false
	})
	return opened, err
}

// ActivateSceneForTest brings sc to the front and waits up to wait for it to be active.
func ActivateSceneForTest(sc SceneForTest, wait time.Duration) error {
	var session objc.ID
	onMainSync(func() { session = sc.Scene.Send(sel("session")) })
	if err := requestScene(session); err != nil {
		return err
	}
	return waitScenes(wait, "the scene did not come to the front", func([]SceneForTest) bool {
		var active bool
		onMainSync(func() { active = objc.Send[int](sc.Scene, sel("activationState")) == 0 })
		return active
	})
}

// CloseSceneForTest asks UIKit to destroy sc's session, as the user closing one of the app's windows does, and waits up to wait for UIKit to disconnect it and the backend to let its surface go.
func CloseSceneForTest(sc SceneForTest, wait time.Duration) error {
	var surface *surface
	onMainSync(func() {
		if v := views[sc.View]; v != nil {
			surface = v.surface
		}
		app := objc.ID(apple.Class("UIApplication")).Send(sel("sharedApplication"))
		handler := objcblock.New(func(_ objc.Block, err objc.ID) {
			fmt.Printf("closing a scene failed: %s\n", apple.GoString(err.Send(sel("localizedDescription"))))
		})
		app.Send(sel("requestSceneSessionDestruction:options:errorHandler:"), sc.Scene.Send(sel("session")), objc.ID(0), handler)
		handler.Release()
	})
	return waitScenes(wait, "the scene stayed", func(scs []SceneForTest) bool {
		if slices.ContainsFunc(scs, func(o SceneForTest) bool { return o.Scene == sc.Scene }) {
			return false
		}
		var gone bool
		onMainSync(func() { gone = surface == nil || surface == runner || !slices.Contains(scenes, surface) })
		return gone
	})
}

// CloseOtherScenesForTest closes every scene but the one the engine's first tree shows, as a test leaves the app for the next, which iPadOS would otherwise connect the sessions of again.
func CloseOtherScenesForTest(wait time.Duration) error {
	first := runnerScene()
	for _, sc := range ScenesForTest() {
		if sc.Scene == first {
			continue
		}
		if err := CloseSceneForTest(sc, wait); err != nil {
			return err
		}
	}
	return nil
}

// runnerScene is the scene the engine's first tree shows in, 0 while in none.
func runnerScene() objc.ID {
	var scene objc.ID
	onMainSync(func() {
		if runner.window != 0 {
			scene = runner.window.Send(sel("windowScene"))
		}
	})
	return scene
}

// DisconnectSceneForTest calls the scene delegate's sceneDidDisconnect: for sc, as UIKit does as it releases a scene in the background, and returns what connects it again through scene:willConnectToSession:options:, as UIKit does as the user returns to it.
func DisconnectSceneForTest(sc SceneForTest) (reconnect func()) {
	onMainSync(func() { sc.Scene.Send(sel("delegate")).Send(sel("sceneDidDisconnect:"), sc.Scene) })
	return func() {
		onMainSync(func() {
			sc.Scene.Send(sel("delegate")).Send(sel("scene:willConnectToSession:options:"), sc.Scene, sc.Scene.Send(sel("session")), objc.ID(0))
		})
	}
}

// PressInSceneForTest presses the middle of the view with testID in the tree sc shows, through sc's window.
func PressInSceneForTest(sc SceneForTest, testID string) error {
	var s *surface
	var at driver.Point
	var err error
	onMainSync(func() {
		v := views[sc.View]
		if v == nil {
			err = errors.New("the scene shows no surface")
			return
		}
		s = v.surface
		i := slices.IndexFunc(s.views, func(v *view) bool { return v != nil && v.props.TestID == testID })
		if i < 0 {
			err = fmt.Errorf("the scene shows no %q", testID)
			return
		}
		n := s.views[i].native
		b := objc.Send[apple.CGRect](n, selBounds)
		p := objc.Send[apple.CGPoint](n, sel("convertPoint:toView:"), apple.CGPoint{X: b.Size.Width / 2, Y: b.Size.Height / 2}, objc.ID(0))
		at = driver.Point{X: driver.Points(p.X), Y: driver.Points(p.Y)}
	})
	if err != nil {
		return err
	}
	return (&testSurface{s: s}).Press(at, 0, driver.Modifiers{})
}

//go:build js && wasm && nsgotest

package harmonyos

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall/js"
	"time"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// The test host: a test's app runs in the page, with the ArkTS render layer
// drawing what it mounts, and the host opens it, takes the root view's
// picture, and sends the test's input into the engine. A press goes to the
// view the engine's frames put under the point, as ArkUI's hit test names
// the view a page's press went to, and a field takes the focus the page
// reports.
//
// Screenshots come back through the bridge: the page snapshots the root
// FrameNode, packs a PNG and calls nsgoSnapshotReply.

func init() { testhost.Register(host{}) }

type host struct{}

func (host) Main(run func() int) {
	go func() { os.Exit(run()) }()
	select {}
}

func (host) Several() bool { return false }

func (host) Open(app func(cx *core.Scope) core.Node, o testhost.Options) (testhost.Surface, error) {
	if js.Global().Get("nsgoHarmony").Type() != js.TypeObject {
		return nil, errors.New("the page has no nsgoHarmony binding: run the tests with nsgo test -target harmonyos")
	}
	installSnapshots()
	s := newSurface(Options{})
	env := testhost.Pin(s.env, o.Environment)
	if o.Size.Width > 0 && o.Size.Height > 0 {
		s.size0 = o.Size
	}
	engineOpts := engine.Options{
		ReadAsset:    readAsset,
		CheckCommits: o.CheckCommits,
		Size:         s.size0, Environment: env, Clock: o.Clock, Storage: o.Storage,
		Observe: o.Observe, Event: o.Event, TransitionEnds: o.TransitionEnds,
	}
	if dir, err := os.MkdirTemp("", "nsgo-test-"); err == nil {
		engineOpts.DataDir = filepath.Join(dir, "data")
		engineOpts.CacheDir = filepath.Join(dir, "cache")
		engineOpts.LogsDir = filepath.Join(dir, "logs")
	}
	s.engine = engine.Start(s, app, engineOpts)
	s.install()
	s.bridge.Call("ready")
	return &testSurface{s: s, epoch: time.Now()}, nil
}

// testSurface is the app's surface for a test: points are the surface's,
// as a finger's are.
type testSurface struct {
	s     *surface
	epoch time.Time
}

// touch reports one finger at at.
func (t *testSurface) touch(phase driver.GesturePhase, at driver.Point, mods driver.Modifiers) {
	ev := driver.Event{Kind: driver.EventTouch, Phase: phase, Modifiers: mods, Time: time.Since(t.epoch),
		Touches: []driver.Touch{{ID: 0, Position: at, PagePosition: at, Changed: true}}}
	t.s.m.captureTouches(&ev)
	t.s.dispatch(ev)
}

func (t *testSurface) Press(at driver.Point, hold time.Duration, mods driver.Modifiers) error {
	// A press goes where the page's own does: to the view under the point,
	// which a page learns from ArkUI and a test from the frames.
	if id := t.s.m.pressTarget(at); id != 0 {
		t.s.m.press(id, driver.EventPress, at, mods)
		// A field takes the keyboard focus as it does when a finger taps
		// it, which the page reports from ArkUI's own focus.
		if v := t.s.m.views[id]; v != nil && (v.kind == driver.KindInput || v.kind == driver.KindWeb) && !v.disabled {
			t.s.m.reportFocus(id)
			t.s.m.o.reset()
			t.s.m.o.op("fo", id)
			t.s.m.flush()
		}
		return nil
	}
	t.touch(driver.GestureBegan, at, mods)
	// A tap keeps the finger down for a moment, as one does.
	if hold <= 0 {
		hold = 30 * time.Millisecond
	}
	time.Sleep(hold)
	t.touch(driver.GestureEnded, at, mods)
	return nil
}

func (t *testSurface) RightClick(driver.Point) error { return testhost.ErrNoMenu }

func (t *testSurface) Drag(path []driver.Point) error {
	if len(path) == 0 {
		return errors.New("a drag needs a path")
	}
	t.touch(driver.GestureBegan, path[0], driver.Modifiers{})
	for _, p := range path[1:] {
		time.Sleep(10 * time.Millisecond)
		t.touch(driver.GestureChanged, p, driver.Modifiers{})
	}
	time.Sleep(20 * time.Millisecond)
	t.touch(driver.GestureEnded, path[len(path)-1], driver.Modifiers{})
	return nil
}

// Scroll drags the content by delta and rests before lifting, as a finger
// does when it scrolls and lets go.
func (t *testSurface) Scroll(at driver.Point, delta driver.Point) error {
	// ArkUI scrolls its own view, so a test drives the page's Scroller
	// rather than dragging: the page then reports the offset to the app.
	if id := t.s.m.scrollTarget(at); id != 0 {
		t.s.bridge.Call("scrollBy", int(id), float64(delta.X), float64(delta.Y))
		return nil
	}
	from := at
	to := driver.Point{X: at.X - delta.X, Y: at.Y - delta.Y}
	steps := 6
	t.touch(driver.GestureBegan, from, driver.Modifiers{})
	for i := 1; i <= steps; i++ {
		f := float32(i) / float32(steps)
		p := driver.Point{X: from.X + (to.X-from.X)*driver.Points(f), Y: from.Y + (to.Y-from.Y)*driver.Points(f)}
		time.Sleep(10 * time.Millisecond)
		t.touch(driver.GestureChanged, p, driver.Modifiers{})
	}
	time.Sleep(60 * time.Millisecond)
	t.touch(driver.GestureEnded, to, driver.Modifiers{})
	return nil
}

func (t *testSurface) Hover(driver.Point) error { return nil }

func (t *testSurface) focusedInput() (driver.NodeID, *view) {
	id := t.s.m.nativeFocused
	if id == 0 {
		id = t.s.m.focused
	}
	v := t.s.m.views[id]
	if v == nil || v.kind != driver.KindInput {
		return 0, nil
	}
	return id, v
}

func inputSelection(v *view) (driver.TextRange, int, int) {
	n := driver.UTF16Len(v.value)
	sel := v.selection
	if !v.selected {
		sel = driver.TextRange{Start: n, End: n}
	}
	sel.Start = min(max(sel.Start, 0), n)
	sel.End = min(max(sel.End, 0), n)
	if sel.Start > sel.End {
		sel.Start, sel.End = sel.End, sel.Start
	}
	return sel, driver.UTF16Offset(v.value, sel.Start), driver.UTF16Offset(v.value, sel.End)
}

func (t *testSurface) replaceInput(id driver.NodeID, v *view, left, right int, insert string) error {
	value := v.value[:left] + insert + v.value[right:]
	t.s.m.edited(id, value)
	// Type and clipboard editing keys are test-host shortcuts for keyboard input. The engine receives the edit above, but ArkUI's TextInput has not emitted onChange, so update the native field through the page bridge as a real keyboard would show it.
	t.s.bridge.Call("typeInput", int(id), value)
	caret := driver.UTF16Len(value[:left+len(insert)])
	_, err := t.SelectRange(driver.TextRange{Start: caret, End: caret})
	return err
}

func (t *testSurface) setClipboard(text string) {
	t.s.m.clipboard = text
	if t.s.bridge.Type() == js.TypeObject && t.s.bridge.Get("clipset").Type() == js.TypeFunction {
		t.s.bridge.Call("clipset", text)
	}
}

// Type replaces the native field's last reported selection, or appends before
// the first selection callback, and updates the rendered field through the page.
func (t *testSurface) Type(text string) error {
	id, v := t.focusedInput()
	if v == nil {
		return errors.New("no text field has the keyboard focus")
	}
	if v.disabled || v.readonly {
		return nil
	}
	sel, left, right := inputSelection(v)
	insert, ok := driver.LimitEdit(v.value, sel.End-sel.Start, text, v.maxLength)
	if !ok {
		return nil
	}
	return t.replaceInput(id, v, left, right, insert)
}

// SelectRange uses the renderer's existing TextInputController selection path
// and waits for ArkUI's own onTextSelectionChange to report the resulting range.
func (t *testSurface) SelectRange(selection driver.TextRange) (driver.TextRange, error) {
	id, v := t.focusedInput()
	if v == nil {
		return driver.TextRange{}, errors.New("no text field has the keyboard focus")
	}
	t.s.m.mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemSelection, ID: id, Selection: selection}}})
	until := time.Now().Add(2 * time.Second)
	for time.Now().Before(until) {
		if v.selected && v.selection == selection {
			return v.selection, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return v.selection, errors.New("ArkUI did not report the requested native selection")
}

func (t *testSurface) Compose(string) error { return testhost.ErrNoInputMethod }
func (t *testSurface) Commit(string) error  { return testhost.ErrNoInputMethod }

func (t *testSurface) KeyDown(key string) error { return t.holdKey(key, 0) }
func (t *testSurface) KeyUp(key string) error   { return t.holdKey(key, 1) }

// KeyCodeDown implements testhost.KeyCodeSurface: the physical position code goes down and stays held, reported with InputKit's KeyCode for the position and name, the US layout's key there, as its text.
func (t *testSurface) KeyCodeDown(code, name string) error { return t.holdCode(code, name, 0) }

// KeyCodeUp implements testhost.KeyCodeSurface: the position code, which KeyCodeDown held, comes up.
func (t *testSurface) KeyCodeUp(code, name string) error { return t.holdCode(code, name, 1) }

func (t *testSurface) holdCode(code, name string, action int) error {
	for k, position := range ohosKeyPositions {
		if position == code {
			t.s.m.key(k, action, t.s.m.keyModifiers(0), name)
			return nil
		}
	}
	return errors.New("InputKit has no KeyCode for " + code)
}

func (t *testSurface) holdKey(key string, action int) error {
	key = driver.NormalizeKey(key)
	if key == "mod" {
		key = "ctrl"
	}
	code, ok := keyCode[key]
	if !ok {
		return errors.New("unknown key " + key)
	}
	t.s.m.key(code, action, t.s.m.keyModifiers(0), key)
	return nil
}

// Key presses a key the keymap names, as a keyboard does, and drives the
// focused native field's clipboard, select-all and Backspace edits when no
// keymap binding takes them.
func (t *testSurface) Key(key string) error {
	norm := driver.NormalizeKey(key)
	mods := 0
	for {
		switch {
		case strings.HasPrefix(norm, "mod+"):
			mods |= 1
			norm = strings.TrimPrefix(norm, "mod+")
		case strings.HasPrefix(norm, "ctrl+"):
			mods |= 1
			norm = strings.TrimPrefix(norm, "ctrl+")
		case strings.HasPrefix(norm, "alt+"):
			mods |= 2
			norm = strings.TrimPrefix(norm, "alt+")
		case strings.HasPrefix(norm, "shift+"):
			mods |= 4
			norm = strings.TrimPrefix(norm, "shift+")
		case strings.HasPrefix(norm, "meta+"):
			mods |= 8
			norm = strings.TrimPrefix(norm, "meta+")
		default:
			if handled, err := t.editFieldKey(norm, mods); handled {
				return err
			}
			code, ok := keyCode[norm]
			if !ok && len(norm) > 0 {
				code = driver.NodeID(norm[0])
			}
			t.s.m.key(code, 0, mods, norm)
			t.s.m.key(code, 1, 0, "")
			return nil
		}
	}
}

// editFieldKey handles clipboard, select-all and Backspace keys on the focused
// native field after offering them to the surface's keymap.
func (t *testSurface) editFieldKey(name string, mods int) (bool, error) {
	id, v := t.focusedInput()
	if v == nil {
		return false, nil
	}
	for t.s.m.keyRunning {
		time.Sleep(time.Millisecond)
	}
	effective := t.s.m.keyModifiers(mods)
	isEdit := (effective == 1 && (name == "a" || name == "c" || name == "x" || name == "v")) ||
		(effective == 0 && name == "backspace")
	if !isEdit {
		return false, nil
	}
	if t.s.m.matchKey(name, effective, false, true) {
		return true, nil
	}
	if v.disabled {
		return true, nil
	}
	switch name {
	case "a":
		_, err := t.SelectRange(driver.TextRange{Start: 0, End: driver.UTF16Len(v.value)})
		return true, err
	case "c":
		if v.masked {
			return true, nil
		}
		_, left, right := inputSelection(v)
		if left < right {
			t.setClipboard(v.value[left:right])
		}
		return true, nil
	case "x":
		if v.readonly || v.masked {
			return true, nil
		}
		_, left, right := inputSelection(v)
		if left == right {
			return true, nil
		}
		t.setClipboard(v.value[left:right])
		return true, t.replaceInput(id, v, left, right, "")
	case "v":
		if v.readonly {
			return true, nil
		}
		text := t.s.m.clipboard
		if text == "" {
			return true, nil
		}
		t.s.dispatch(driver.Event{Kind: driver.EventPaste, ID: id, Data: &driver.PasteEvent{Text: text}})
		if v.pasteBehavior != driver.PasteInsert {
			return true, nil
		}
		if !v.multiline {
			text = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(text)
		}
		sel, left, right := inputSelection(v)
		insert, ok := driver.LimitEdit(v.value, sel.End-sel.Start, text, v.maxLength)
		if !ok || insert == "" {
			return true, nil
		}
		return true, t.replaceInput(id, v, left, right, insert)
	case "backspace":
		if v.readonly {
			return true, nil
		}
		_, left, right := inputSelection(v)
		if left == right {
			if left == 0 {
				return true, nil
			}
			_, size := utf8.DecodeLastRuneInString(v.value[:left])
			left -= size
		}
		return true, t.replaceInput(id, v, left, right, "")
	}
	return false, nil
}

// keyCode names the Linux input codes a key goes by, as the emulator's
// uinput reports them.
var keyCode = func() map[string]driver.NodeID {
	out := make(map[string]driver.NodeID, len(linuxKeys))
	for code, name := range linuxKeys {
		out[name] = code
	}
	return out
}()

func (t *testSurface) Back() error {
	t.s.dispatch(driver.Event{Kind: driver.EventBack})
	return nil
}

func (t *testSurface) Answer(id uint32, index int) error {
	t.s.dispatch(driver.Event{Kind: driver.EventAlert, Count: id, Index: index})
	return nil
}

func (t *testSurface) ChooseDockMenu(uint32) error { return testhost.ErrNoMenu }
func (t *testSurface) ChooseMenu(uint32) error     { return testhost.ErrNoMenu }
func (t *testSurface) ChoosePopup(uint32) error    { return testhost.ErrNoMenu }

var _ testhost.TabBarSurface = (*testSurface)(nil)

// SelectTab selects the native tab bar's item at index, as tapping it would.
func (t *testSurface) SelectTab(id driver.NodeID, index int) error {
	v := t.s.m.views[id]
	if v == nil || v.kind != driver.KindNative || v.tabBar == nil {
		return fmt.Errorf("view %d is not a native tab bar", id)
	}
	if index < 0 || index >= len(v.tabBar.spec.Items) {
		return fmt.Errorf("tab index %d out of range for %d tabs", index, len(v.tabBar.spec.Items))
	}
	t.s.m.tabSelected(id, index)
	return nil
}

// screenshot requests answers a test's picture: the page snapshots the
// root view and calls nsgoSnapshotReply with the PNG's base64.
var (
	snapshotMu   sync.Mutex
	snapshotNext int
	snapshotWait = map[int]chan []byte{}
)

func installSnapshots() {
	js.Global().Set("nsgoSnapshotReply", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 2 {
			return nil
		}
		id := args[0].Int()
		snapshotMu.Lock()
		ch := snapshotWait[id]
		delete(snapshotWait, id)
		snapshotMu.Unlock()
		if ch == nil {
			return nil
		}
		data, err := base64.StdEncoding.DecodeString(args[1].String())
		if err != nil {
			data = nil
		}
		ch <- data
		return nil
	}))
}

func (t *testSurface) Screenshot() ([]byte, error) {
	snapshotMu.Lock()
	snapshotNext++
	id := snapshotNext
	ch := make(chan []byte, 1)
	snapshotWait[id] = ch
	snapshotMu.Unlock()
	t.s.bridge.Call("snapshot", id)
	select {
	case data := <-ch:
		if len(data) == 0 {
			return nil, errors.New("the page took no picture")
		}
		return data, nil
	case <-time.After(5 * time.Second):
		snapshotMu.Lock()
		delete(snapshotWait, id)
		snapshotMu.Unlock()
		return nil, errors.New("the page did not answer the snapshot")
	}
}

func (t *testSurface) Animating() bool { return t.s.anim.Running() }

func (t *testSurface) Resize(size driver.Size) error {
	t.s.size0 = size
	t.s.engine.Resize(size, driver.Geometry{})
	return nil
}

func (t *testSurface) SetEnvironment(env driver.Environment) error {
	t.s.env = env
	t.s.engine.SetEnvironment(env)
	return nil
}

func (t *testSurface) Sync() {
	for t.s.m.keyRunning {
		time.Sleep(time.Millisecond)
	}
	t.s.engine.Sync()
}

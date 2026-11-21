//go:build android && (nsgotest || nsgoautomation)

package android

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// testSurface acts on the Android runtime's primary view through TestAgent.java
// over the pipe, for the test host (testhost.go) and for nsgo dev -automation's
// live surface (automation.go), where test is nil.
type testSurface struct {
	s    *surface
	test *testOpen

	mu        sync.Mutex
	next      uint32
	replies   map[uint32]chan agentReply
	closed    chan struct{}
	signature string
}

type agentReply struct{ err, data string }

// newTestSurface wires s's msgTestReply handler to a new testSurface.
func newTestSurface(s *surface, test *testOpen) *testSurface {
	t := &testSurface{
		s:       s,
		test:    test,
		replies: map[uint32]chan agentReply{},
		closed:  make(chan struct{}),
	}
	s.testReply = t.reply
	return t
}

// close unblocks any in-flight ask once the pipe to the runtime has closed.
func (t *testSurface) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	select {
	case <-t.closed:
	default:
		close(t.closed)
	}
}

// reply takes the agent's answer, on the pipe's reader.
func (t *testSurface) reply(r *reader) {
	id, err, data := r.u32(), r.str(), r.str()
	t.mu.Lock()
	ch := t.replies[id]
	delete(t.replies, id)
	t.mu.Unlock()
	if ch != nil {
		ch <- agentReply{err, data}
	}
}

// ask has the agent take op and returns its data.
func (t *testSurface) ask(op map[string]any) (string, error) {
	t.mu.Lock()
	select {
	case <-t.closed:
		t.mu.Unlock()
		return "", errors.New("the runtime closed")
	default:
	}
	t.next++
	id := t.next
	ch := make(chan agentReply, 1)
	t.replies[id] = ch
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.replies, id)
		t.mu.Unlock()
	}()
	data, _ := json.Marshal(op)
	t.s.send(msgTest, func(w *writer) {
		w.u32(id)
		w.str(string(data))
	})
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case r := <-ch:
		if r.err != "" {
			return "", errors.New(r.err)
		}
		return r.data, nil
	case <-t.closed:
		return "", errors.New("the runtime closed")
	case <-timer.C:
		return "", fmt.Errorf("the test agent did not answer %v", op["op"])
	}
}

func (t *testSurface) do(op map[string]any) error {
	_, err := t.ask(op)
	return err
}

func (t *testSurface) Press(at driver.Point, hold time.Duration, mods driver.Modifiers) error {
	return t.do(map[string]any{"op": "press", "x": at.X, "y": at.Y, "hold": hold.Milliseconds(), "modifiers": mods})
}

func (t *testSurface) RightClick(driver.Point) error {
	return errors.New("a phone has no secondary click: use LongPress")
}

func (t *testSurface) Drag(path []driver.Point) error {
	return t.do(map[string]any{"op": "drag", "path": path})
}

func (t *testSurface) Scroll(at driver.Point, delta driver.Point) error {
	return t.do(map[string]any{"op": "scroll", "x": at.X, "y": at.Y, "dx": delta.X, "dy": delta.Y})
}

func (t *testSurface) Hover(driver.Point) error { return nil }

func (t *testSurface) Type(text string) error {
	return t.do(map[string]any{"op": "type", "text": text})
}

func (t *testSurface) Key(key string) error { return t.do(map[string]any{"op": "key", "key": key}) }

func (t *testSurface) SelectRange(selection driver.TextRange) (driver.TextRange, error) {
	data, err := t.ask(map[string]any{"op": "select-range", "start": selection.Start, "end": selection.End})
	if err != nil {
		return driver.TextRange{}, err
	}
	var actual driver.TextRange
	if err := json.Unmarshal([]byte(data), &actual); err != nil {
		return driver.TextRange{}, fmt.Errorf("native selection: %w", err)
	}
	return actual, nil
}

// Compose and Commit go through the focused field's InputConnection, as
// an input method's composition does.
func (t *testSurface) Compose(text string) error {
	return t.do(map[string]any{"op": "compose", "text": text})
}

func (t *testSurface) Commit(text string) error {
	return t.do(map[string]any{"op": "commit", "text": text})
}

func (t *testSurface) Back() error { return t.do(map[string]any{"op": "back"}) }

func (t *testSurface) SelectTab(id driver.NodeID, index int) error {
	return t.do(map[string]any{"op": "select-tab", "id": id, "index": index})
}

func (t *testSurface) Answer(id uint32, index int) error {
	return t.do(map[string]any{"op": "answer", "id": id, "index": index})
}

func (t *testSurface) Screenshot() ([]byte, error) {
	data, err := t.ask(map[string]any{"op": "screenshot"})
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(data)
}

func (t *testSurface) Sync() {
	if t.s.engine != nil {
		t.s.engine.Sync()
	}
}

// Animating compares where every view is, how far it scrolled and how
// opaque it is with the last look: anything that moved animates.
func (t *testSurface) Animating() bool {
	sig, err := t.ask(map[string]any{"op": "signature"})
	if err != nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	moved := sig != t.signature
	t.signature = sig
	return moved
}

// Resize does nothing: a phone's screen is the size it is.
func (t *testSurface) Resize(driver.Size) error { return nil }

func (t *testSurface) SetEnvironment(env driver.Environment) error {
	if t.test == nil {
		return errors.New("a running app's window follows the system's settings")
	}
	t.test.pin.Set(&env)
	t.s.engine.Repin()
	return nil
}

// ChooseMenu implements testhost.Surface: no native menu bar shows here.
func (t *testSurface) ChooseMenu(uint32) error { return testhost.ErrNoMenu }

// OpenLinks delivers the URLs supplied by the test to the app's engine.
func (t *testSurface) OpenLinks(links []string) {
	t.s.engine.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: links})
}

func (t *testSurface) ReceiveShare(items []driver.SharedItem) {
	t.s.engine.Dispatch(driver.Event{Kind: driver.EventShared, Data: items})
}

// ChooseDockMenu implements testhost.Surface: Android has no app icon menu.
func (t *testSurface) ChooseDockMenu(uint32) error { return testhost.ErrNoMenu }

// ChoosePopup implements testhost.Surface: the core shows no native
// context menus here.
func (t *testSurface) ChoosePopup(uint32) error { return testhost.ErrNoMenu }

package windows

import (
	"slices"
	"sync"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	embeddedMsgSize        uint32 = 0x0005 // WM_SIZE
	embeddedMsgActivate    uint32 = 0x0006 // WM_ACTIVATE
	embeddedMsgShowWindow  uint32 = 0x0018 // WM_SHOWWINDOW
	embeddedMsgActivateApp uint32 = 0x001C // WM_ACTIVATEAPP
	embeddedMsgNCDestroy   uint32 = 0x0082 // WM_NCDESTROY

	embeddedSizeRestored  uintptr = 0 // SIZE_RESTORED
	embeddedSizeMinimized uintptr = 1 // SIZE_MINIMIZED
	embeddedSizeMaximized uintptr = 2 // SIZE_MAXIMIZED

	embeddedWAInactive uintptr = 0 // WA_INACTIVE
)

// embeddedRootState holds the lifecycle-relevant state of an embedded view's ancestor root window (GA_ROOT).
type embeddedRootState struct {
	visible   bool
	minimized bool
	active    bool
}

// initialEmbeddedRootState builds the root window's initial state on attach from IsWindowVisible(root), IsIconic(root) and GetForegroundWindow() == root.
func initialEmbeddedRootState(visible, iconic, foreground bool) embeddedRootState {
	return embeddedRootState{
		visible:   visible,
		minimized: iconic,
		active:    foreground,
	}
}

// lifecycle reports the driver.Lifecycle corresponding to the root window's visibility, minimized and activation state.
func (s embeddedRootState) lifecycle() driver.Lifecycle {
	if !s.visible || s.minimized {
		return driver.LifecycleBackground
	}
	if s.active {
		return driver.LifecycleActive
	}
	return driver.LifecycleInactive
}

// applyMessage updates s for a Win32 message observed on the root window and reports whether the state changed.
func (s embeddedRootState) applyMessage(message uint32, wParam, _ uintptr) (embeddedRootState, bool) {
	next := s
	switch message {
	case embeddedMsgSize:
		switch wParam {
		case embeddedSizeMinimized:
			next.minimized = true
		case embeddedSizeRestored, embeddedSizeMaximized:
			next.minimized = false
		default:
			return s, false
		}
	case embeddedMsgShowWindow:
		next.visible = wParam != 0
	case embeddedMsgActivate:
		// LOWORD(wParam) is WA_INACTIVE / WA_ACTIVE / WA_CLICKACTIVE; HIWORD(wParam) is nonzero when the window is minimized.
		next.active = wParam&0xffff != embeddedWAInactive
	case embeddedMsgActivateApp:
		next.active = wParam != 0
	default:
		return s, false
	}
	return next, next != s
}

// embeddedRootWatcher receives lifecycle updates for an embedded view's ancestor root window.
type embeddedRootWatcher interface {
	embeddedRootChanged(root uintptr, state embeddedRootState)
	embeddedRootDestroyed(root uintptr)
}

// embeddedRootHooks abstracts Win32 subclass installation and root state reads so embeddedRootManager can be tested in pure Go on every OS.
type embeddedRootHooks struct {
	install   func(root uintptr) bool
	remove    func(root uintptr)
	readState func(root uintptr) embeddedRootState
}

type embeddedRootEntry struct {
	state      embeddedRootState
	subclassed bool
	views      []embeddedRootWatcher
}

// embeddedRootManager ref-counts root-window subclasses across embedded views sharing the same ancestor root HWND (GA_ROOT). Removing or reparenting one embedded view leaves siblings observing the same root untouched, and both view destruction and root WM_NCDESTROY clean up the subclass.
type embeddedRootManager struct {
	hooks  embeddedRootHooks
	mu     sync.Mutex
	roots  map[uintptr]*embeddedRootEntry
	byView map[embeddedRootWatcher]uintptr
}

func newEmbeddedRootManager(hooks embeddedRootHooks) *embeddedRootManager {
	return &embeddedRootManager{
		hooks:  hooks,
		roots:  map[uintptr]*embeddedRootEntry{},
		byView: map[embeddedRootWatcher]uintptr{},
	}
}

// attach registers w to observe root, installing a subclass on root if no other embedded view is observing it yet.
func (m *embeddedRootManager) attach(w embeddedRootWatcher, root uintptr) (embeddedRootState, bool) {
	if w == nil || root == 0 {
		return embeddedRootState{}, false
	}

	var removeOld uintptr
	m.mu.Lock()
	if cur, ok := m.byView[w]; ok {
		if cur == root {
			if entry := m.roots[root]; entry != nil {
				state, sub := entry.state, entry.subclassed
				m.mu.Unlock()
				return state, sub
			}
		}
		removeOld = m.detachLocked(w)
	}
	if entry := m.roots[root]; entry != nil {
		entry.views = append(entry.views, w)
		m.byView[w] = root
		state, sub := entry.state, entry.subclassed
		m.mu.Unlock()
		if removeOld != 0 && m.hooks.remove != nil {
			m.hooks.remove(removeOld)
		}
		return state, sub
	}
	m.mu.Unlock()

	if removeOld != 0 && m.hooks.remove != nil {
		m.hooks.remove(removeOld)
	}

	var state embeddedRootState
	if m.hooks.readState != nil {
		state = m.hooks.readState(root)
	}
	if m.hooks.install == nil || !m.hooks.install(root) {
		return state, false
	}

	m.mu.Lock()
	entry := m.roots[root]
	if entry == nil {
		entry = &embeddedRootEntry{state: state, subclassed: true}
		m.roots[root] = entry
	}
	entry.views = append(entry.views, w)
	m.byView[w] = root
	out, sub := entry.state, entry.subclassed
	m.mu.Unlock()
	return out, sub
}

// detach removes w from its observed root, removing the root's subclass once no embedded views remain on it.
func (m *embeddedRootManager) detach(w embeddedRootWatcher) {
	if w == nil {
		return
	}
	m.mu.Lock()
	removeRoot := m.detachLocked(w)
	m.mu.Unlock()
	if removeRoot != 0 && m.hooks.remove != nil {
		m.hooks.remove(removeRoot)
	}
}

func (m *embeddedRootManager) detachLocked(w embeddedRootWatcher) uintptr {
	root, ok := m.byView[w]
	if !ok {
		return 0
	}
	delete(m.byView, w)
	entry := m.roots[root]
	if entry == nil {
		return 0
	}
	if i := slices.Index(entry.views, w); i >= 0 {
		entry.views = slices.Delete(entry.views, i, i+1)
	}
	if len(entry.views) > 0 {
		return 0
	}
	delete(m.roots, root)
	if entry.subclassed {
		return root
	}
	return 0
}

// sync updates w's observed root after a potential reparenting. It reports whether w's root changed and whether w is currently attached to newRoot.
func (m *embeddedRootManager) sync(w embeddedRootWatcher, newRoot uintptr) (state embeddedRootState, changed bool, attached bool) {
	if w == nil {
		return embeddedRootState{}, false, false
	}
	m.mu.Lock()
	oldRoot, hadOld := m.byView[w]
	if hadOld && oldRoot == newRoot {
		if entry := m.roots[newRoot]; entry != nil {
			state, attached = entry.state, entry.subclassed
			m.mu.Unlock()
			return state, false, attached
		}
	}
	if !hadOld && newRoot == 0 {
		m.mu.Unlock()
		return embeddedRootState{}, false, false
	}
	m.mu.Unlock()

	if newRoot == 0 {
		m.detach(w)
		return embeddedRootState{}, true, false
	}
	state, attached = m.attach(w, newRoot)
	return state, true, attached
}

// stateOf returns the tracked state for w's observed root, if attached.
func (m *embeddedRootManager) stateOf(w embeddedRootWatcher) (embeddedRootState, bool) {
	if w == nil {
		return embeddedRootState{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	root, ok := m.byView[w]
	if !ok {
		return embeddedRootState{}, false
	}
	entry := m.roots[root]
	if entry == nil {
		return embeddedRootState{}, false
	}
	return entry.state, true
}

// watchersOf returns a snapshot of the embedded views currently observing root.
func (m *embeddedRootManager) watchersOf(root uintptr) []embeddedRootWatcher {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.roots[root]
	if entry == nil || len(entry.views) == 0 {
		return nil
	}
	return slices.Clone(entry.views)
}

// handleMessage processes a Win32 message observed on root and notifies every embedded view attached to root when its lifecycle state changes or when root receives WM_NCDESTROY.
func (m *embeddedRootManager) handleMessage(root uintptr, message uint32, wParam, lParam uintptr) (stateChanged bool, destroyed bool) {
	m.mu.Lock()
	entry := m.roots[root]
	if entry == nil {
		m.mu.Unlock()
		return false, false
	}
	if message == embeddedMsgNCDestroy {
		views := slices.Clone(entry.views)
		subclassed := entry.subclassed
		delete(m.roots, root)
		for _, v := range views {
			if m.byView[v] == root {
				delete(m.byView, v)
			}
		}
		m.mu.Unlock()
		if subclassed && m.hooks.remove != nil {
			m.hooks.remove(root)
		}
		for _, v := range views {
			v.embeddedRootDestroyed(root)
		}
		return false, true
	}

	next, changed := entry.state.applyMessage(message, wParam, lParam)
	if !changed {
		m.mu.Unlock()
		return false, false
	}
	entry.state = next
	views := slices.Clone(entry.views)
	m.mu.Unlock()

	for _, v := range views {
		m.mu.Lock()
		still := m.byView[v] == root
		m.mu.Unlock()
		if still {
			v.embeddedRootChanged(root, next)
		}
	}
	return true, false
}

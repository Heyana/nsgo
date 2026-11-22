//go:build windows

package windows

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	comctl32             = syscall.NewLazyDLL("comctl32.dll")
	setWindowSubclass    = comctl32.NewProc("SetWindowSubclass")
	removeWindowSubclass = comctl32.NewProc("RemoveWindowSubclass")
	defSubclassProc      = comctl32.NewProc("DefSubclassProc")
)

const (
	// embeddedRootSubclassID identifies NSGo's comctl32 subclass on an embedded view's ancestor root window (GA_ROOT).
	embeddedRootSubclassID uintptr = 0x4E53474F // "NSGO"

	wmWindowPosChanged = 0x0047
	swpHideWindow      = 0x0080
)

type windowPos struct {
	Hwnd            uintptr
	HwndInsertAfter uintptr
	X, Y, CX, CY    int32
	Flags           uint32
}

var (
	rootSubclassOnce sync.Once
	rootSubclassPtr  uintptr
	embeddedRoots    *embeddedRootManager
)

func init() {
	embeddedRoots = newEmbeddedRootManager(embeddedRootHooks{
		install:   installEmbeddedRootSubclass,
		remove:    removeEmbeddedRootSubclass,
		readState: readEmbeddedRootState,
	})
}

func rootSubclassCallback() uintptr {
	rootSubclassOnce.Do(func() {
		rootSubclassPtr = syscall.NewCallback(rootSubclassProc)
	})
	return rootSubclassPtr
}

func installEmbeddedRootSubclass(root uintptr) bool {
	if root == 0 {
		return false
	}
	if setWindowSubclass.Find() != nil || removeWindowSubclass.Find() != nil || defSubclassProc.Find() != nil {
		return false
	}
	r, _, _ := setWindowSubclass.Call(root, rootSubclassCallback(), embeddedRootSubclassID, 0)
	return r != 0
}

func removeEmbeddedRootSubclass(root uintptr) {
	if root == 0 || removeWindowSubclass.Find() != nil {
		return
	}
	removeWindowSubclass.Call(root, rootSubclassCallback(), embeddedRootSubclassID)
}

func readEmbeddedRootState(root uintptr) embeddedRootState {
	if root == 0 {
		return embeddedRootState{}
	}
	visible, _, _ := isWindowVisible.Call(root)
	fg, _, _ := getForegroundWindow.Call()
	return initialEmbeddedRootState(visible != 0, isIconicWindow(root), fg == root)
}

func rootSubclassProc(hwnd, message, wParam, lParam, _, _ uintptr) uintptr {
	msg := uint32(message)
	if msg == embeddedMsgNCDestroy {
		// Remove the subclass before DefSubclassProc tears down subclass data.
		embeddedRoots.handleMessage(hwnd, msg, wParam, lParam)
		r, _, _ := defSubclassProc.Call(hwnd, message, wParam, lParam)
		return r
	}

	r, _, _ := defSubclassProc.Call(hwnd, message, wParam, lParam)
	switch msg {
	case embeddedMsgSize, embeddedMsgShowWindow:
		embeddedRoots.handleMessage(hwnd, msg, wParam, lParam)
	case embeddedMsgActivate:
		changed, _ := embeddedRoots.handleMessage(hwnd, msg, wParam, lParam)
		if !changed {
			for _, watcher := range embeddedRoots.watchersOf(hwnd) {
				if w, ok := watcher.(*window); ok && w.engine != nil && w.hwnd != 0 {
					w.post(w.reportFrameWindow)
				}
			}
		}
	case embeddedMsgActivateApp:
		*appActive() = wParam != 0
		embeddedRoots.handleMessage(hwnd, msg, wParam, lParam)
	case wmWindowPosChanged:
		if lParam != 0 {
			wp := *(**windowPos)(unsafe.Pointer(&lParam))
			switch {
			case wp.Flags&swpHideWindow != 0:
				embeddedRoots.handleMessage(hwnd, embeddedMsgShowWindow, 0, 0)
			case wp.Flags&swpShowWindow != 0:
				embeddedRoots.handleMessage(hwnd, embeddedMsgShowWindow, 1, 0)
			}
		}
	}
	return r
}

// embeddedAncestorRoot returns the ancestor root window (GA_ROOT) of an embedded child view, or 0 if it has no distinct root window.
func (w *window) embeddedAncestorRoot() uintptr {
	if !w.embedded || w.hwnd == 0 {
		return 0
	}
	root := w.owner()
	if root == w.hwnd {
		return 0
	}
	return root
}

// observeEmbeddedRoot attaches w to its current ancestor root window's subclass so root minimize, restore, show/hide and activation reach w.
func (w *window) observeEmbeddedRoot() {
	if !w.embedded || w.hwnd == 0 {
		return
	}
	root := w.embeddedAncestorRoot()
	w.observedRoot = root
	embeddedRoots.attach(w, root)
}

// syncEmbeddedRoot checks whether an embedded view was reparented under a different ancestor root window and moves its root observation accordingly.
func (w *window) syncEmbeddedRoot() {
	if !w.embedded || w.hwnd == 0 {
		return
	}
	root := w.embeddedAncestorRoot()
	if root == w.observedRoot {
		return
	}
	w.observedRoot = root
	_, changed, _ := embeddedRoots.sync(w, root)
	if changed {
		w.embeddedRootChanged(root, embeddedRootState{})
	}
}

// unobserveEmbeddedRoot detaches w from its observed root window, removing the root subclass when no other embedded view observes that root.
func (w *window) unobserveEmbeddedRoot() {
	if !w.embedded {
		return
	}
	w.observedRoot = 0
	embeddedRoots.detach(w)
}

func (w *window) embeddedRootChanged(_ uintptr, _ embeddedRootState) {
	if w.hwnd == 0 || w.destroying {
		return
	}
	if w.engine == nil {
		w.env = w.environment()
		return
	}
	w.updateEnvironment()
}

func (w *window) embeddedRootDestroyed(root uintptr) {
	if w.observedRoot == root {
		w.observedRoot = 0
	}
	if w.hwnd == 0 || w.destroying {
		return
	}
	if w.engine == nil {
		w.env = w.environment()
		return
	}
	w.updateEnvironment()
}

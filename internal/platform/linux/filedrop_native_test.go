//go:build linux && !android

package linux

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

type x11EventBuf [24]uintptr

// x11SelectionRequest is Xlib's XSelectionRequestEvent.
type x11SelectionRequest struct {
	typ       int32
	serial    uintptr
	sendEvent int32
	display   uintptr
	owner     uintptr
	requestor uintptr
	selection uintptr
	target    uintptr
	property  uintptr
	time      uintptr
}

// x11SelectionEvent is Xlib's XSelectionEvent padded to XEvent's size.
type x11SelectionEvent struct {
	typ       int32
	serial    uintptr
	sendEvent int32
	display   uintptr
	requestor uintptr
	selection uintptr
	target    uintptr
	property  uintptr
	time      uintptr
	_         [15]uintptr
}

// x11PropertyEvent is Xlib's XPropertyEvent.
type x11PropertyEvent struct {
	typ       int32
	serial    uintptr
	sendEvent int32
	display   uintptr
	window    uintptr
	atom      uintptr
	time      uintptr
	state     int32
}

const (
	xdndPropertyNotify     = 28
	xdndSelectionRequestEv = 30
	xdndSelectionNotifyEv  = 31
	xdndPropertyChangeMask = 1 << 22
	xdndXAAtom             = 4
	xdndXAString           = 31
	xdndPropModeReplace    = 0
)

// xdndXlib holds the Xlib symbols used to speak XDND v5 as another X11 client.
type xdndXlib struct {
	openDisplay       func(name *byte) uintptr
	closeDisplay      func(display uintptr) int32
	rootWindow        func(display uintptr) uintptr
	createWindow      func(display, parent uintptr, x, y int32, width, height, border uint32, borderPixel, background uintptr) uintptr
	destroyWindow     func(display, window uintptr) int32
	internAtom        func(display uintptr, name *byte, onlyIfExists int32) uintptr
	selectInput       func(display, window uintptr, mask int) int32
	changeProperty    func(display, window, property, typ uintptr, format, mode int32, data *byte, n int32) int32
	getProperty       func(display, window, property uintptr, offset, length int, del int32, reqType uintptr, actualType *uintptr, actualFormat *int32, n, after *uint, data **byte) int32
	free              func(data *byte) int32
	setSelectionOwner func(display, selection, owner, timestamp uintptr) int32
	getSelectionOwner func(display, selection uintptr) uintptr
	sendEvent         func(display, window uintptr, propagate int32, mask int, event unsafe.Pointer) int32
	translateCoords   func(display, src, dst uintptr, srcX, srcY int32, dstX, dstY *int32, child *uintptr) int32
	flush             func(display uintptr) int32
	xSync             func(display uintptr, discard int32) int32
	pending           func(display uintptr) int32
	nextEvent         func(display uintptr, event *x11EventBuf) int32
}

func loadXdndXlib() (*xdndXlib, bool) {
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		if lib, err = purego.Dlopen("libX11.so", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			return nil, false
		}
	}
	var x xdndXlib
	purego.RegisterLibFunc(&x.openDisplay, lib, "XOpenDisplay")
	purego.RegisterLibFunc(&x.closeDisplay, lib, "XCloseDisplay")
	purego.RegisterLibFunc(&x.rootWindow, lib, "XDefaultRootWindow")
	purego.RegisterLibFunc(&x.createWindow, lib, "XCreateSimpleWindow")
	purego.RegisterLibFunc(&x.destroyWindow, lib, "XDestroyWindow")
	purego.RegisterLibFunc(&x.internAtom, lib, "XInternAtom")
	purego.RegisterLibFunc(&x.selectInput, lib, "XSelectInput")
	purego.RegisterLibFunc(&x.changeProperty, lib, "XChangeProperty")
	purego.RegisterLibFunc(&x.getProperty, lib, "XGetWindowProperty")
	purego.RegisterLibFunc(&x.free, lib, "XFree")
	purego.RegisterLibFunc(&x.setSelectionOwner, lib, "XSetSelectionOwner")
	purego.RegisterLibFunc(&x.getSelectionOwner, lib, "XGetSelectionOwner")
	purego.RegisterLibFunc(&x.sendEvent, lib, "XSendEvent")
	purego.RegisterLibFunc(&x.translateCoords, lib, "XTranslateCoordinates")
	purego.RegisterLibFunc(&x.flush, lib, "XFlush")
	purego.RegisterLibFunc(&x.xSync, lib, "XSync")
	purego.RegisterLibFunc(&x.pending, lib, "XPending")
	purego.RegisterLibFunc(&x.nextEvent, lib, "XNextEvent")
	return &x, true
}

// TestFileDropFromX11FileManagerReachesView drives a real GTK 4 window through
// the X11 XDND v5 protocol from a second X11 display connection, just as a
// Linux file manager does: advertising text/uri-list, negotiating XdndStatus
// over nested, disabled and modal views, serving XdndSelection on drop, and
// opening every dropped file with core.FileFromURL (#1601).
func TestFileDropFromX11FileManagerReachesView(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	x, ok := loadXdndXlib()
	if !ok {
		t.Skip("no libX11")
	}

	dir := t.TempDir()
	files := []struct {
		name    string
		content string
		isDir   bool
	}{
		{name: "a b.txt", content: "hello space"},
		{name: "é.txt", content: "accent"},
		{name: "100%#?.txt", content: "special"},
		{name: "folder", isDir: true},
	}
	var wantURIs []string
	var uriLines []string
	uriLines = append(uriLines, "# comment from a file manager")
	for _, f := range files {
		p := filepath.Join(dir, f.name)
		if f.isDir {
			if err := os.Mkdir(p, 0o755); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.WriteFile(p, []byte(f.content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		u := fileURL(p)
		wantURIs = append(wantURIs, u)
		uriLines = append(uriLines, u)
	}
	// Remote URIs in text/uri-list must be ignored without blocking local files.
	uriLines = append(uriLines, "https://example.com/ignored.txt")
	uriListPayload := []byte(strings.Join(uriLines, "\r\n") + "\r\n")

	var (
		mu       sync.Mutex
		events   []string
		lastDrop []string
		modalSig *core.Signal[bool]
		scopes   = make(chan *core.Scope, 1)
	)
	record := func(s string) {
		mu.Lock()
		events = append(events, s)
		mu.Unlock()
	}
	snapshot := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), events...)
	}
	clearEvents := func() {
		mu.Lock()
		events = nil
		lastDrop = nil
		mu.Unlock()
	}
	waitEvents := func(want []string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			got := snapshot()
			if slices.Equal(got, want) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("events = %v, want %v", got, want)
			}
			time.Sleep(15 * time.Millisecond)
		}
	}

	title := "nsgo-xdnd-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		showModal := core.NewSignal(cx, false)
		modalSig = showModal
		scopes <- cx
		zone := func(name string, style core.Style, disabled bool, children ...core.Node) core.Node {
			return core.View{
				TestID: name, Style: style, Children: children,
				Disabled:        func() bool { return disabled },
				OnFileDragEnter: func() { record("enter " + name) },
				OnFileDragLeave: func() { record("leave " + name) },
				OnFileDrop: func(uris []string) {
					mu.Lock()
					lastDrop = append([]string(nil), uris...)
					mu.Unlock()
					record("drop " + name + " " + strings.Join(uris, " "))
				},
			}
		}
		return core.View{
			Style: core.Style{Width: driver.Pt(360), Height: driver.Pt(240), Background: driver.RGB(0xffffff)},
			Children: []core.Node{
				zone("outer", abs(0, 0, 160, 120, driver.RGB(0xeeeeee)), false,
					core.View{TestID: "inner", Style: abs(10, 10, 40, 40, driver.RGB(0xcccccc))},
					zone("nestedOff", abs(90, 10, 50, 50, driver.RGB(0xbbbbbb)), true),
				),
				zone("off", abs(200, 0, 120, 100, driver.RGB(0xffdddd)), true),
				core.Show{
					When: showModal.Get,
					Then: func(cx *core.Scope) core.Node {
						return core.Modal{
							Children: []core.Node{
								core.View{
									Style: core.Style{Width: driver.Pt(360), Height: driver.Pt(240), PointerEvents: core.PointerEventsBoxNone},
									Children: []core.Node{
										zone("modalZone", abs(180, 120, 140, 80, driver.RGB(0xddeeff)), false),
									},
								},
							},
						}
					},
				},
			},
		}
	}

	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 240})
	cx := <-scopes

	var xid uintptr
	deadline := time.Now().Add(5 * time.Second)
	for {
		var ready bool
		w.onLoop(t, func() {
			if !drawsOnX11() {
				ready = true
				return
			}
			_, xid, _ = w.x11Window()
			var hasOuter bool
			for _, v := range w.views {
				if v != nil && !v.deleted && v.props.TestID == "outer" && v.frame.Size.Width > 0 {
					hasOuter = true
					break
				}
			}
			ready = xid != 0 && hasOuter && w.drop.target != 0 && gtkWidgetGetMapped(w.surface) != 0 && gtkWidgetGetWidth(w.surface) > 0
		})
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("window never mapped with a drop target")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if xid == 0 {
		t.Skip("GDK is not drawing on X11")
	}

	display := x.openDisplay(nil)
	if display == 0 {
		t.Skip("cannot open the X11 display")
	}
	defer x.closeDisplay(display)
	root := x.rootWindow(display)
	atom := func(name string) uintptr { return x.internAtom(display, cstr(name), 0) }

	xdndAware := atom("XdndAware")
	xdndEnter := atom("XdndEnter")
	xdndPosition := atom("XdndPosition")
	xdndStatus := atom("XdndStatus")
	xdndLeave := atom("XdndLeave")
	xdndDrop := atom("XdndDrop")
	xdndFinished := atom("XdndFinished")
	xdndSelection := atom("XdndSelection")
	xdndActionCopy := atom("XdndActionCopy")
	targetsAtom := atom("TARGETS")
	uriListAtom := atom("text/uri-list")
	textPlainAtom := atom("text/plain")

	// Verify GTK 4 advertised XdndAware >= 5 on the target window.
	var actualType uintptr
	var actualFormat int32
	var nItems, bytesAfter uint
	var propData *byte
	x.getProperty(display, xid, xdndAware, 0, 1, 0, 0, &actualType, &actualFormat, &nItems, &bytesAfter, &propData)
	var xdndVersion uintptr
	if nItems > 0 && propData != nil {
		xdndVersion = *(*uintptr)(unsafe.Pointer(propData))
	}
	if propData != nil {
		x.free(propData)
	}
	if xdndVersion < 5 {
		t.Fatalf("XdndAware version = %d, want >= 5", xdndVersion)
	}

	// Create an X11 source window (standing in for a file manager) and obtain a real server timestamp.
	source := x.createWindow(display, root, 0, 0, 10, 10, 0, 0, 0)
	defer x.destroyWindow(display, source)
	x.selectInput(display, source, xdndPropertyChangeMask)
	stampProp := atom("NSGO_XDND_TIMESTAMP")
	var zeroByte byte
	x.changeProperty(display, source, stampProp, xdndXAString, 8, xdndPropModeReplace, &zeroByte, 0)
	x.flush(display)

	var serverTime uintptr
	stampDeadline := time.Now().Add(5 * time.Second)
	for serverTime == 0 {
		if x.pending(display) == 0 {
			if time.Now().After(stampDeadline) {
				t.Fatal("timed out waiting for X11 server timestamp")
			}
			time.Sleep(5 * time.Millisecond)
			continue
		}
		var ev x11EventBuf
		x.nextEvent(display, &ev)
		if *(*int32)(unsafe.Pointer(&ev)) == xdndPropertyNotify {
			pe := (*x11PropertyEvent)(unsafe.Pointer(&ev))
			if pe.window == source && pe.atom == stampProp {
				serverTime = pe.time
			}
		}
	}

	x.setSelectionOwner(display, xdndSelection, source, serverTime)
	x.xSync(display, 0)
	if owner := x.getSelectionOwner(display, xdndSelection); owner != source {
		t.Fatalf("XdndSelection owner = %#x, want %#x", owner, source)
	}

	// surfaceToRoot converts surface-local points to X11 root-window pixels.
	surfaceToRoot := func(sxPt, syPt float64) (int32, int32) {
		var winX, winY int32
		w.onLoop(t, func() {
			var tx, ty float64
			gtkNativeGetSurfaceTransform(w.win, &tx, &ty)
			var pt graphenePoint
			gtkWidgetComputePoint(w.surface, w.win, &graphenePoint{float32(sxPt), float32(syPt)}, &pt)
			scale := gtkWidgetGetScaleFactor(w.surface)
			if scale < 1 {
				scale = 1
			}
			winX = int32(math.Round((float64(pt.x) + tx) * float64(scale)))
			winY = int32(math.Round((float64(pt.y) + ty) * float64(scale)))
		})
		var rootX, rootY int32
		var child uintptr
		x.translateCoords(display, xid, root, winX, winY, &rootX, &rootY, &child)
		return rootX, rootY
	}

	sendMsg := func(msgType uintptr, data [5]int) {
		ev := x11ClientMessage{
			typ:         x11ClientMessageType,
			display:     display,
			window:      xid,
			messageType: msgType,
			format:      32,
			data:        data,
		}
		x.sendEvent(display, xid, 0, 0, unsafe.Pointer(&ev))
		x.flush(display)
	}

	answerSelection := func(req *x11SelectionRequest) {
		prop := req.property
		if prop == 0 {
			prop = req.target
		}
		replyProp := prop
		switch req.target {
		case targetsAtom:
			offered := []uintptr{targetsAtom, uriListAtom}
			x.changeProperty(display, req.requestor, prop, xdndXAAtom, 32, xdndPropModeReplace,
				(*byte)(unsafe.Pointer(&offered[0])), int32(len(offered)))
		case uriListAtom:
			x.changeProperty(display, req.requestor, prop, uriListAtom, 8, xdndPropModeReplace,
				&uriListPayload[0], int32(len(uriListPayload)))
		default:
			replyProp = 0
		}
		notify := x11SelectionEvent{
			typ:       xdndSelectionNotifyEv,
			display:   display,
			requestor: req.requestor,
			selection: req.selection,
			target:    req.target,
			property:  replyProp,
			time:      req.time,
		}
		x.sendEvent(display, req.requestor, 0, 0, unsafe.Pointer(&notify))
		x.flush(display)
	}

	// nextReply returns the next wantMsg sent to the source before deadline, answering selection requests meanwhile.
	nextReply := func(wantMsg uintptr, deadline time.Time) ([5]int, bool) {
		for time.Now().Before(deadline) {
			if x.pending(display) == 0 {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			var ev x11EventBuf
			x.nextEvent(display, &ev)
			switch *(*int32)(unsafe.Pointer(&ev)) {
			case xdndSelectionRequestEv:
				answerSelection((*x11SelectionRequest)(unsafe.Pointer(&ev)))
			case x11ClientMessageType:
				cm := (*x11ClientMessage)(unsafe.Pointer(&ev))
				if cm.window == source && cm.messageType == wantMsg {
					return cm.data, true
				}
			}
		}
		return [5]int{}, false
	}

	waitReply := func(wantMsg uintptr, what string) [5]int {
		t.Helper()
		reply, ok := nextReply(wantMsg, time.Now().Add(5*time.Second))
		if !ok {
			t.Fatalf("timed out waiting for %s", what)
		}
		return reply
	}

	// A position that moves the drag onto another GTK widget is answered twice: GTK 4 sends a refusing XdndStatus for the drop crossing it synthesizes (gtk_main_do_event ends that crossing with gtk_drop_end_event, and a drop target already holding the drop does not answer it), then the one the motion handler decides. A source acts on the latest status, so this one does too: it reads statuses until no more arrive.
	sendPosition := func(sxPt, syPt float64) (accepted bool, action uintptr) {
		rx, ry := surfaceToRoot(sxPt, syPt)
		packed := int((uint32(rx) << 16) | (uint32(ry) & 0xffff))
		sendMsg(xdndPosition, [5]int{int(source), 0, packed, int(serverTime), int(xdndActionCopy)})
		reply := waitReply(xdndStatus, fmt.Sprintf("XdndStatus at (%v, %v)", sxPt, syPt))
		for {
			later, ok := nextReply(xdndStatus, time.Now().Add(250*time.Millisecond))
			if !ok {
				break
			}
			reply = later
		}
		return reply[1]&1 != 0, uintptr(reply[4])
	}

	// 1. Non-file drag (text/plain) is refused by GTK 4's GtkDropTarget.
	sendMsg(xdndEnter, [5]int{int(source), 5 << 24, int(textPlainAtom), 0, 0})
	if accepted, action := sendPosition(25, 25); accepted || action != 0 {
		t.Fatalf("text/plain drag status = (%v, %d), want refused", accepted, action)
	}
	sendMsg(xdndLeave, [5]int{int(source), 0, 0, 0, 0})
	if got := snapshot(); len(got) != 0 {
		t.Fatalf("text/plain drag emitted events %v", got)
	}

	// 2. Real text/uri-list drag across inner child, nested disabled zone, standalone disabled zone, empty area, then drop.
	sendMsg(xdndEnter, [5]int{int(source), 5 << 24, int(uriListAtom), 0, 0})
	if accepted, action := sendPosition(25, 25); !accepted || action != xdndActionCopy {
		t.Fatalf("over inner child: status = (%v, %#x), want (true, Copy=%#x)", accepted, action, xdndActionCopy)
	}
	if accepted, action := sendPosition(110, 30); accepted || action != 0 {
		t.Fatalf("over nested disabled zone: status = (%v, %#x), want refused", accepted, action)
	}
	if accepted, action := sendPosition(240, 40); accepted || action != 0 {
		t.Fatalf("over standalone disabled zone: status = (%v, %#x), want refused", accepted, action)
	}
	if accepted, action := sendPosition(240, 180); accepted || action != 0 {
		t.Fatalf("over empty area: status = (%v, %#x), want refused", accepted, action)
	}
	if accepted, action := sendPosition(25, 25); !accepted || action != xdndActionCopy {
		t.Fatalf("re-enter inner child: status = (%v, %#x), want (true, Copy=%#x)", accepted, action, xdndActionCopy)
	}

	sendMsg(xdndDrop, [5]int{int(source), 0, int(serverTime), 0, 0})
	fin := waitReply(xdndFinished, "XdndFinished")
	if fin[1]&1 == 0 || uintptr(fin[2]) != xdndActionCopy {
		t.Fatalf("XdndFinished = %v, want accepted with Copy (%#x)", fin, xdndActionCopy)
	}
	wantDrop := "drop outer " + strings.Join(wantURIs, " ")
	waitEvents([]string{"enter outer", "leave outer", "enter outer", wantDrop, "leave outer"})

	// Verify every dropped file URL opens cleanly through core.FileFromURL.
	mu.Lock()
	dropped := append([]string(nil), lastDrop...)
	mu.Unlock()
	if !slices.Equal(dropped, wantURIs) {
		t.Fatalf("dropped URIs = %v, want %v", dropped, wantURIs)
	}
	for i, f := range files {
		cf, err := core.FileFromURL(dropped[i])
		if f.isDir {
			if err == nil {
				t.Fatalf("FileFromURL(%q) opened a folder", dropped[i])
			}
			continue
		}
		if err != nil {
			t.Fatalf("FileFromURL(%q): %v", dropped[i], err)
		}
		rc, err := cf.Open()
		if err != nil {
			t.Fatalf("Open(%q): %v", dropped[i], err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("Read(%q): %v", dropped[i], err)
		}
		if string(data) != f.content {
			t.Fatalf("content of %q = %q, want %q", f.name, string(data), f.content)
		}
	}

	// 3. XdndLeave delivers OnFileDragLeave.
	clearEvents()
	sendMsg(xdndEnter, [5]int{int(source), 5 << 24, int(uriListAtom), 0, 0})
	if accepted, action := sendPosition(25, 25); !accepted || action != xdndActionCopy {
		t.Fatalf("enter before leave: status = (%v, %#x), want accepted", accepted, action)
	}
	sendMsg(xdndLeave, [5]int{int(source), 0, 0, 0, 0})
	waitEvents([]string{"enter outer", "leave outer"})

	// 4. A modal bounds XDND drops: background zones refuse, modalZone accepts and receives the files.
	clearEvents()
	cx.Post(func() { modalSig.Set(true) })
	modalDeadline := time.Now().Add(5 * time.Second)
	for {
		var hasModal bool
		w.onLoop(t, func() {
			if root := w.view(w.root); root != nil && topModal(root) != root {
				for _, v := range w.views {
					if v != nil && !v.deleted && v.props.TestID == "modalZone" && v.frame.Size.Width > 0 {
						hasModal = true
						break
					}
				}
			}
		})
		if hasModal {
			break
		}
		if time.Now().After(modalDeadline) {
			t.Fatal("modal never mounted")
		}
		time.Sleep(15 * time.Millisecond)
	}

	sendMsg(xdndEnter, [5]int{int(source), 5 << 24, int(uriListAtom), 0, 0})
	if accepted, action := sendPosition(25, 25); accepted || action != 0 {
		t.Fatalf("over background behind modal: status = (%v, %#x), want refused", accepted, action)
	}
	if accepted, action := sendPosition(240, 160); !accepted || action != xdndActionCopy {
		t.Fatalf("over modalZone: status = (%v, %#x), want (true, Copy=%#x)", accepted, action, xdndActionCopy)
	}
	sendMsg(xdndDrop, [5]int{int(source), 0, int(serverTime), 0, 0})
	fin = waitReply(xdndFinished, "XdndFinished on modalZone")
	if fin[1]&1 == 0 || uintptr(fin[2]) != xdndActionCopy {
		t.Fatalf("modal XdndFinished = %v, want accepted with Copy (%#x)", fin, xdndActionCopy)
	}
	wantModal := "drop modalZone " + strings.Join(wantURIs, " ")
	waitEvents([]string{"enter modalZone", wantModal, "leave modalZone"})
}

// TestFileURIsReadsSingleGFile checks that fileURIs also reads a GValue holding
// a single GFile (from a GTK 4 drag source offering G_TYPE_FILE), and skips
// non-local URIs.
func TestFileURIsReadsSingleGFile(t *testing.T) {
	if testing.Short() {
		t.Skip("uses GTK")
	}
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	path := filepath.Join(t.TempDir(), "single file.txt")
	if err := os.WriteFile(path, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	gfile := gFileNewForPath(cstr(path))
	value := new([3]uint64)
	gValueInit(uintptr(unsafe.Pointer(value)), gFileGetType())
	gValueTakeObject(uintptr(unsafe.Pointer(value)), gfile)
	defer gValueUnset(uintptr(unsafe.Pointer(value)))

	got := fileURIs(uintptr(unsafe.Pointer(value)))
	want := []string{fileURL(path)}
	if !slices.Equal(got, want) {
		t.Fatalf("fileURIs(GFile) = %v, want %v", got, want)
	}
}

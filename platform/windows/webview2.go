//go:build windows

package windows

// WebView2 through COM over syscall, and the loader's job done in Go.
// WebView2Loader.dll only finds the installed Evergreen runtime and calls
// its EmbeddedBrowserWebView.dll, so NSGo does the same and ships no DLL:
// the runtime's version is in EdgeUpdate's registry keys, and the client
// DLL's CreateWebViewEnvironmentWithOptionsInternal makes the environment.
// The vtable indexes below follow WebView2.idl (SDK 1.0.2903.40); each
// interface's methods come after IUnknown's three.

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var (
	ole32          = syscall.NewLazyDLL("ole32.dll")
	coInitializeEx = ole32.NewProc("CoInitializeEx")
	coTaskMemFree  = ole32.NewProc("CoTaskMemFree")
)

// guid is a COM interface ID.
type guid struct {
	a    uint32
	b, c uint16
	d    [8]byte
}

// parseGUID parses "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"; it is only
// given the constants below.
func parseGUID(s string) guid {
	hex := strings.ReplaceAll(s, "-", "")
	word := func(from, to int) uint64 {
		n, err := strconv.ParseUint(hex[from:to], 16, 64)
		if err != nil {
			panic("bad GUID " + s)
		}
		return n
	}
	g := guid{a: uint32(word(0, 8)), b: uint16(word(8, 12)), c: uint16(word(12, 16))}
	for i := range g.d {
		g.d[i] = byte(word(16+2*i, 18+2*i))
	}
	return g
}

var (
	iidUnknown                = parseGUID("00000000-0000-0000-c000-000000000046")
	iidEnvironment            = parseGUID("b96d755e-0319-4e92-a296-23436f46a1fc")
	iidController2            = parseGUID("c979903e-d4ca-4228-92eb-47ee3fa96eab")
	iidSettings2              = parseGUID("ee9a0f68-f46c-4e32-ac23-ef8cac224d2a")
	iidEnvironmentCompleted   = parseGUID("4e8a3389-c9d8-4bd2-b6b5-124fee6cc14d")
	iidControllerCompleted    = parseGUID("6c4819f3-c9b7-4260-8127-c9f5bde7f68c")
	iidNavigationStarting     = parseGUID("9adbe429-f36d-432b-9ddc-f8881fbd76e3")
	iidNavigationCompleted    = parseGUID("d33a35bf-1c49-4f98-93ab-006e0533fe1c")
	iidWebMessageReceived     = parseGUID("57213f19-00e6-49fa-8e07-898ea01ecbd2")
	iidDocumentTitleChanged   = parseGUID("f5f2b923-953e-4042-9f95-f3a118e1afd4")
	iidHistoryChanged         = parseGUID("c79a420c-efd9-4058-9295-3e8b4bcab645")
	iidExecuteScriptCompleted = parseGUID("49511172-cc67-4bca-9923-137112f4c4cc")
	iidAddScriptCompleted     = parseGUID("b99369f3-9b11-47b5-bc6f-8e7895fcea17")
)

// Vtable indexes.
const (
	comQueryInterface = 0
	comAddRef         = 1
	comRelease        = 2

	// ICoreWebView2Environment
	envCreateController = 3

	// ICoreWebView2Controller, then ICoreWebView2Controller2
	controllerPutIsVisible                = 4
	controllerPutBounds                   = 6
	controllerNotifyParentPositionChanged = 23
	controllerClose                       = 24
	controllerGetCoreWebView2             = 25
	controllerPutDefaultBackgroundColor   = 27

	// ICoreWebView2
	webGetSettings                   = 3
	webGetSource                     = 4
	webNavigate                      = 5
	webNavigateToString              = 6
	webAddNavigationStarting         = 7
	webAddHistoryChanged             = 13
	webAddNavigationCompleted        = 15
	webAddScriptOnDocumentCreated    = 27
	webRemoveScriptOnDocumentCreated = 28
	webExecuteScript                 = 29
	webReload                        = 31
	webAddWebMessageReceived         = 34
	webGetCanGoBack                  = 38
	webGetCanGoForward               = 39
	webGoBack                        = 40
	webGoForward                     = 41
	webStop                          = 43
	webAddDocumentTitleChanged       = 46
	webGetDocumentTitle              = 48

	// ICoreWebView2Settings, then ICoreWebView2Settings2
	settingsPutIsScriptEnabled = 4
	settingsGetUserAgent       = 21
	settingsPutUserAgent       = 22

	// ICoreWebView2NavigationCompletedEventArgs
	navigationGetIsSuccess      = 3
	navigationGetWebErrorStatus = 4

	// ICoreWebView2WebMessageReceivedEventArgs
	messageTryGetString = 5
)

const (
	sOK             = 0
	eNoInterface    = 0x80004002
	rpcEChangedMode = 0x80010106
)

func failed(hr uintptr) bool { return int32(hr) < 0 }

// comObject is what a COM interface pointer points to: its vtable first.
type comObject struct{ vtbl *[64]uintptr }

// call calls method on o. Its arguments may be Go pointers converted to
// uintptr, which the directive keeps on the heap during the call.
//
//go:uintptrescapes
func (o *comObject) call(method int, args ...uintptr) uintptr {
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, uintptr(unsafe.Pointer(o)))
	r, _, _ := syscall.SyscallN(o.vtbl[method], append(all, args...)...)
	return r
}

func (o *comObject) release() {
	if o != nil {
		o.call(comRelease)
	}
}

// query returns o's interface iid, nil if it has none.
func (o *comObject) query(iid *guid) *comObject {
	var out *comObject
	if failed(o.call(comQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))) {
		return nil
	}
	return out
}

// getString calls a getter returning a string the caller frees.
func (o *comObject) getString(method int) string {
	var p *uint16
	if failed(o.call(method, uintptr(unsafe.Pointer(&p)))) {
		return ""
	}
	return takeString(p)
}

func (o *comObject) getBool(method int) bool {
	var b int32
	o.call(method, uintptr(unsafe.Pointer(&b)))
	return b != 0
}

// takeString copies and frees a string COM allocated.
func takeString(p *uint16) string {
	if p == nil {
		return ""
	}
	s := wideString(p)
	coTaskMemFree.Call(uintptr(unsafe.Pointer(p)))
	return s
}

// wideString copies a NUL-terminated UTF-16 string.
func wideString(p *uint16) string {
	if p == nil {
		return ""
	}
	n := 0
	for at := unsafe.Pointer(p); *(*uint16)(at) != 0; at = unsafe.Add(at, 2) {
		n++
	}
	return syscall.UTF16ToString(unsafe.Slice(p, n))
}

// comHandler is a WebView2 callback object: IUnknown and Invoke, whose
// two arguments are an HRESULT or sender and a result or event's
// arguments. Go keeps it alive while COM holds a reference.
type comHandler struct {
	vtbl   *[4]uintptr
	refs   int32
	iid    *guid
	invoke func(a uintptr, b unsafe.Pointer)
}

var (
	handlerVtbl = [4]uintptr{
		syscall.NewCallback(handlerQueryInterface),
		syscall.NewCallback(handlerAddRef),
		syscall.NewCallback(handlerRelease),
		syscall.NewCallback(handlerInvoke),
	}
	handlersMu sync.Mutex
	handlers   = map[*comHandler]struct{}{}
)

// newHandler returns a handler of interface iid holding one reference,
// the caller's, which it gives up with release.
func newHandler(iid *guid, invoke func(a uintptr, b unsafe.Pointer)) *comHandler {
	h := &comHandler{vtbl: &handlerVtbl, refs: 1, iid: iid, invoke: invoke}
	handlersMu.Lock()
	handlers[h] = struct{}{}
	handlersMu.Unlock()
	return h
}

func (h *comHandler) ptr() uintptr { return uintptr(unsafe.Pointer(h)) }

func (h *comHandler) release() { handlerRelease(h) }

func handlerQueryInterface(h *comHandler, iid *guid, out *unsafe.Pointer) uintptr {
	if *iid == iidUnknown || *iid == *h.iid {
		*out = unsafe.Pointer(h)
		handlerAddRef(h)
		return sOK
	}
	*out = nil
	return eNoInterface
}

func handlerAddRef(h *comHandler) uintptr {
	handlersMu.Lock()
	defer handlersMu.Unlock()
	h.refs++
	return uintptr(h.refs)
}

func handlerRelease(h *comHandler) uintptr {
	handlersMu.Lock()
	defer handlersMu.Unlock()
	if h.refs--; h.refs == 0 {
		delete(handlers, h)
	}
	return uintptr(h.refs)
}

func handlerInvoke(h *comHandler, a uintptr, b unsafe.Pointer) uintptr {
	if h.invoke != nil {
		h.invoke(a, b)
	}
	return sOK
}

// eventToken is an EventRegistrationToken.
type eventToken struct{ value int64 }

// subscribe registers handler h with add method, giving up the caller's
// reference: WebView2 keeps its own until the web view closes.
func (o *comObject) subscribe(method int, h *comHandler) {
	var token eventToken
	o.call(method, h.ptr(), uintptr(unsafe.Pointer(&token)))
	h.release()
}

var errNoWebView2 = errors.New("the WebView2 runtime is not installed")

// evergreenClient is the WebView2 Evergreen runtime's EdgeUpdate client.
const evergreenClient = `{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

// findWebView2 returns the path of the installed Evergreen runtime's
// EmbeddedBrowserWebView.dll, looking where WebView2Loader does: the
// machine's install (32-bit registry view first, where EdgeUpdate
// writes), then the user's.
func findWebView2() (string, error) {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64", "386": "x86"}[runtime.GOARCH]
	if arch == "" {
		return "", errNoWebView2
	}
	roots := []struct {
		key  syscall.Handle
		view uint32
	}{
		{syscall.HKEY_LOCAL_MACHINE, syscall.KEY_WOW64_32KEY},
		{syscall.HKEY_LOCAL_MACHINE, syscall.KEY_WOW64_64KEY},
		{syscall.HKEY_CURRENT_USER, 0},
	}
	for _, root := range roots {
		var dirs []string
		client := `SOFTWARE\Microsoft\EdgeUpdate\Clients\` + evergreenClient
		pv := regString(root.key, root.view, client, "pv")
		if pv == "" || pv == "0.0.0.0" {
			continue
		}
		if location := regString(root.key, root.view, client, "location"); location != "" {
			dirs = append(dirs, filepath.Join(location, pv))
		}
		state := `SOFTWARE\Microsoft\EdgeUpdate\ClientState\` + evergreenClient
		if dir := regString(root.key, root.view, state, "EBWebView"); dir != "" {
			dirs = append(dirs, dir)
		}
		for _, dir := range dirs {
			dll := filepath.Join(dir, "EBWebView", arch, "EmbeddedBrowserWebView.dll")
			if _, err := os.Stat(dll); err == nil {
				return dll, nil
			}
		}
	}
	return "", errNoWebView2
}

// regString reads a string value, "" when there is none.
func regString(root syscall.Handle, view uint32, path, name string) string {
	var key syscall.Handle
	if syscall.RegOpenKeyEx(root, wide(path), 0, syscall.KEY_READ|view, &key) != nil {
		return ""
	}
	defer syscall.RegCloseKey(key)
	var kind, size uint32
	if syscall.RegQueryValueEx(key, wide(name), nil, &kind, nil, &size) != nil ||
		(kind != syscall.REG_SZ && kind != syscall.REG_EXPAND_SZ) || size < 2 {
		return ""
	}
	buf := make([]uint16, size/2)
	if syscall.RegQueryValueEx(key, wide(name), nil, &kind, (*byte)(unsafe.Pointer(&buf[0])), &size) != nil {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// webEnvironment is a window's WebView2 environment: one browser process
// for all its web views, made on the window's thread, which it belongs
// to.
type webEnvironment struct {
	env     *comObject
	err     error
	done    bool
	waiting []func(*comObject, error)
}

// withWebEnvironment calls fn on the main thread once the environment is
// made, or cannot be.
func (w *window) withWebEnvironment(fn func(*comObject, error)) {
	e := w.webEnv
	if e == nil {
		e = &webEnvironment{}
		w.webEnv = e
		e.waiting = append(e.waiting, fn)
		if err := w.startWebEnvironment(); err != nil {
			// Report from a task of its own: the view's events may come
			// later in the batch being applied.
			w.post(func() { e.finish(nil, err) })
		}
		return
	}
	if e.done {
		fn(e.env, e.err)
		return
	}
	e.waiting = append(e.waiting, fn)
}

func (e *webEnvironment) finish(env *comObject, err error) {
	e.env, e.err, e.done = env, err, true
	waiting := e.waiting
	e.waiting = nil
	for _, fn := range waiting {
		fn(env, err)
	}
}

func (w *window) startWebEnvironment() error {
	const coinitApartmentThreaded = 2
	if hr, _, _ := coInitializeEx.Call(0, coinitApartmentThreaded); hr == rpcEChangedMode {
		return errors.New("WebView2 needs a single-threaded COM apartment on the UI thread")
	}
	path, err := findWebView2()
	if err != nil {
		return err
	}
	dll, err := syscall.LoadDLL(path)
	if err != nil {
		return err
	}
	create, err := dll.FindProc("CreateWebViewEnvironmentWithOptionsInternal")
	if err != nil {
		return err
	}
	var folder *uint16
	if w.cacheDir != "" {
		folder = wide(filepath.Join(w.cacheDir, "WebView2"))
	}
	e := w.webEnv
	var h *comHandler
	h = newHandler(&iidEnvironmentCompleted, func(hr uintptr, result unsafe.Pointer) {
		defer h.release()
		env := (*comObject)(result)
		if failed(hr) || env == nil {
			e.finish(nil, hresultError("WebView2 could not start", hr))
			return
		}
		// As WebView2Loader does, keep the environment through its own
		// interface.
		if env = env.query(&iidEnvironment); env == nil {
			e.finish(nil, errors.New("WebView2 gave no environment"))
			return
		}
		e.finish(env, nil)
	})
	const checkRunningInstance, installedRuntime = 1, 0
	hr, _, _ := create.Call(checkRunningInstance, installedRuntime, uintptr(unsafe.Pointer(folder)), 0, h.ptr())
	if failed(hr) {
		h.release()
		return hresultError("WebView2 could not start", hr)
	}
	return nil
}

func hresultError(what string, hr uintptr) error {
	return errors.New(what + ": HRESULT 0x" + strconv.FormatUint(uint64(uint32(hr)), 16))
}

//go:build linux && !android

package linux

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Sharing on Linux tries the XDG desktop portal's OpenURI chooser (with
// ask=true) for a single file, staged plain text, or a web/mailto URI, and
// falls back to org.freedesktop.FileManager1.ShowItems for multiple files
// or when the portal is unavailable. When neither service exists on the
// session bus, Share answers driver.NoShareSheet.

const (
	openURIIface     = "org.freedesktop.portal.OpenURI"
	fileManagerName  = "org.freedesktop.FileManager1"
	fileManagerPath  = "/org/freedesktop/FileManager1"
	fileManagerIface = "org.freedesktop.FileManager1"
)

var (
	gVariantNewHandle                     func(v int32) uintptr
	gUnixFDListNew                        func() uintptr
	gUnixFDListAppend                     func(list uintptr, fd int32, err *uintptr) int32
	gDBusConnectionCallWithUnixFDListSync func(conn uintptr, name, path, iface, method *byte,
		params, replyType uintptr, flags uint32, timeout int32,
		fdList uintptr, outFDList *uintptr, cancellable uintptr, err *uintptr) uintptr

	bindShare = sync.OnceFunc(func() {
		bindGlobalHotKeys()
		purego.RegisterLibFunc(&gVariantNewHandle, glibLib, "g_variant_new_handle")
		purego.RegisterLibFunc(&gUnixFDListNew, gioLib, "g_unix_fd_list_new")
		purego.RegisterLibFunc(&gUnixFDListAppend, gioLib, "g_unix_fd_list_append")
		purego.RegisterLibFunc(&gDBusConnectionCallWithUnixFDListSync, gioLib, "g_dbus_connection_call_with_unix_fd_list_sync")
	})

	cbSharePortalResponse = sync.OnceValue(func() uintptr { return purego.NewCallback(sharePortalResponse) })
)

type linuxShareKind uint8

const (
	linuxShareURI linuxShareKind = iota
	linuxShareSingleFile
	linuxShareMultiFiles
)

type linuxSharePlan struct {
	kind  linuxShareKind
	uri   string
	paths []string
}

type linuxShareRequest struct {
	key     uintptr
	w       *window
	id      uint32
	bus     uintptr
	request string
	sub     uint32
	paths   []string
}

var (
	linuxShareSeq atomic.Uintptr
	linuxSharesMu sync.Mutex
	linuxShares   = map[uintptr]*linuxShareRequest{}
)

// share carries out ItemShare on Linux via the desktop portal or file manager.
func (w *window) share(r driver.ShareRequest, id uint32) {
	if w.closing {
		return
	}
	bindShare()
	appID := w.appID
	parent := w.portalParentWindow()
	req := r
	portalQueue() <- func() {
		plan, err := prepareLinuxShare(req, filepath.Join(os.TempDir(), "nsgo-share"))
		if err != nil {
			w.dispatchShareEvent(driver.Event{Kind: driver.EventFiles, Count: id, Text: err.Error()})
			return
		}
		if plan.kind == linuxShareMultiFiles {
			w.shareViaFileManager(id, plan.paths)
			return
		}
		bus := portalBus(appID)
		if bus == 0 {
			if len(plan.paths) > 0 {
				w.shareViaFileManager(id, plan.paths)
				return
			}
			w.dispatchShareEvent(driver.Event{Kind: driver.EventFiles, Count: id, Text: driver.NoShareSheet})
			return
		}
		token := portalToken("nsgo_share")
		expected := portalRequestPath(bus, token)
		if expected == "" {
			if len(plan.paths) > 0 {
				w.shareViaFileManager(id, plan.paths)
				return
			}
			w.dispatchShareEvent(driver.Event{Kind: driver.EventFiles, Count: id, Text: driver.NoShareSheet})
			return
		}
		w.post(func() {
			if w.closing {
				return
			}
			pending := registerLinuxShare(w, id, bus, expected, plan.paths)
			if pending == nil {
				portalQueue() <- func() {
					if len(plan.paths) > 0 {
						w.shareViaFileManager(id, plan.paths)
					} else {
						w.dispatchShareEvent(driver.Event{Kind: driver.EventFiles, Count: id, Text: driver.NoShareSheet})
					}
				}
				return
			}
			portalQueue() <- func() {
				var reply uintptr
				if plan.kind == linuxShareURI {
					reply = portalCall(bus, globalShortcutPath, openURIIface, "OpenURI",
						gVariantNewString(cstr(parent)),
						gVariantNewString(cstr(plan.uri)),
						portalShareOptions(token))
				} else {
					reply = portalOpenFile(bus, parent, plan.paths[0], token)
				}
				if reply == 0 {
					w.post(func() { cancelLinuxShare(pending.key) })
					if len(plan.paths) > 0 {
						w.shareViaFileManager(id, plan.paths)
					} else {
						w.dispatchShareEvent(driver.Event{Kind: driver.EventFiles, Count: id, Text: driver.NoShareSheet})
					}
					return
				}
				actual := portalTupleText(reply, 0)
				gVariantUnref(reply)
				if actual != "" && actual != expected {
					w.post(func() { rebindLinuxShare(pending.key, actual) })
				}
			}
		})
	}
}

func (w *window) shareViaFileManager(id uint32, paths []string) {
	if len(paths) == 0 {
		w.dispatchShareEvent(driver.Event{Kind: driver.EventFiles, Count: id, Text: driver.NoShareSheet})
		return
	}
	bus := sessionBus()
	if bus == 0 || !fileManagerShowItems(bus, paths) {
		w.dispatchShareEvent(driver.Event{Kind: driver.EventFiles, Count: id, Text: driver.NoShareSheet})
		return
	}
	w.dispatchShareEvent(driver.Event{Kind: driver.EventFiles, Count: id, Finished: true})
}

func (w *window) dispatchShareEvent(ev driver.Event) {
	w.post(func() {
		if !w.closing && w.engine != nil {
			w.engine.Dispatch(ev)
		}
	})
}

func (w *window) closeShares() {
	linuxSharesMu.Lock()
	var owned []*linuxShareRequest
	for k, req := range linuxShares {
		if req.w == w {
			owned = append(owned, req)
			delete(linuxShares, k)
		}
	}
	linuxSharesMu.Unlock()
	for _, req := range owned {
		if req.sub != 0 {
			gDBusConnectionSignalUnsubscribe(req.bus, req.sub)
			req.sub = 0
		}
	}
}

func registerLinuxShare(w *window, id uint32, bus uintptr, requestPath string, paths []string) *linuxShareRequest {
	key := linuxShareSeq.Add(1)
	req := &linuxShareRequest{key: key, w: w, id: id, bus: bus, request: requestPath, paths: paths}
	linuxSharesMu.Lock()
	linuxShares[key] = req
	linuxSharesMu.Unlock()
	sub := gDBusConnectionSignalSubscribe(bus, cstr(globalShortcutName), cstr(globalRequestIface), cstr("Response"),
		cstr(requestPath), nil, 0, cbSharePortalResponse(), key, 0)
	if sub == 0 {
		linuxSharesMu.Lock()
		delete(linuxShares, key)
		linuxSharesMu.Unlock()
		return nil
	}
	req.sub = sub
	return req
}

func cancelLinuxShare(key uintptr) {
	linuxSharesMu.Lock()
	req := linuxShares[key]
	delete(linuxShares, key)
	linuxSharesMu.Unlock()
	if req != nil && req.sub != 0 {
		gDBusConnectionSignalUnsubscribe(req.bus, req.sub)
		req.sub = 0
	}
}

func rebindLinuxShare(key uintptr, actual string) {
	linuxSharesMu.Lock()
	req := linuxShares[key]
	linuxSharesMu.Unlock()
	if req == nil || req.w.closing {
		return
	}
	if req.sub != 0 {
		gDBusConnectionSignalUnsubscribe(req.bus, req.sub)
	}
	req.request = actual
	req.sub = gDBusConnectionSignalSubscribe(req.bus, cstr(globalShortcutName), cstr(globalRequestIface), cstr("Response"),
		cstr(actual), nil, 0, cbSharePortalResponse(), key, 0)
}

func sharePortalResponse(conn, sender, path, iface, signal, params, data uintptr) {
	linuxSharesMu.Lock()
	req := linuxShares[data]
	delete(linuxShares, data)
	linuxSharesMu.Unlock()
	if req == nil {
		return
	}
	if req.sub != 0 {
		gDBusConnectionSignalUnsubscribe(req.bus, req.sub)
		req.sub = 0
	}
	if req.w.closing || req.w.engine == nil {
		return
	}
	code := portalTupleUint32(params, 0)
	switch outcome := linuxSharePortalOutcome(code, len(req.paths) > 0); outcome {
	case shareOutcomeShown:
		req.w.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: req.id, Finished: true})
	case shareOutcomeCancelled:
		req.w.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: req.id})
	case shareOutcomeFallback:
		paths := req.paths
		w, id := req.w, req.id
		portalQueue() <- func() { w.shareViaFileManager(id, paths) }
	default:
		req.w.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: req.id, Text: driver.NoShareSheet})
	}
}

type linuxShareOutcome uint8

const (
	shareOutcomeShown linuxShareOutcome = iota
	shareOutcomeCancelled
	shareOutcomeFallback
	shareOutcomeUnavailable
)

func linuxSharePortalOutcome(code uint32, hasFiles bool) linuxShareOutcome {
	switch code {
	case 0:
		return shareOutcomeShown
	case 1:
		return shareOutcomeCancelled
	default:
		if hasFiles {
			return shareOutcomeFallback
		}
		return shareOutcomeUnavailable
	}
}

func portalShareOptions(token string) uintptr {
	return portalVariant("a{sv}", "{'handle_token': <"+portalQuote(token)+">, 'ask': <true>}")
}

func portalOpenFile(bus uintptr, parent, path, token string) uintptr {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	fdList := gUnixFDListNew()
	if fdList == 0 {
		return 0
	}
	defer gObjectUnref(fdList)

	var gerr uintptr
	idx := gUnixFDListAppend(fdList, int32(f.Fd()), &gerr)
	if gerr != 0 {
		gErrorFree(gerr)
		return 0
	}
	if idx < 0 {
		return 0
	}

	args := []uintptr{
		gVariantNewString(cstr(parent)),
		gVariantNewHandle(idx),
		portalShareOptions(token),
	}
	ok := true
	for i, arg := range args {
		if arg == 0 {
			ok = false
			continue
		}
		args[i] = gVariantTakeRef(arg)
	}
	defer func() {
		for _, arg := range args {
			gVariantUnrefNonZero(arg)
		}
	}()
	if !ok {
		return 0
	}
	params := gVariantNewTuple(&args[0], uintptr(len(args)))
	reply := gDBusConnectionCallWithUnixFDListSync(bus, cstr(globalShortcutName), cstr(globalShortcutPath),
		cstr(openURIIface), cstr("OpenFile"), params, 0, 0, -1, fdList, nil, 0, &gerr)
	if gerr != 0 {
		gErrorFree(gerr)
		return 0
	}
	return reply
}

func fileManagerShowItems(bus uintptr, paths []string) bool {
	if bus == 0 || len(paths) == 0 {
		return false
	}
	uris := make([]uintptr, len(paths))
	for i, p := range paths {
		uris[i] = gVariantNewString(cstr(linuxFileURI(p)))
	}
	strType := gVariantTypeNew(cstr("s"))
	defer gVariantTypeFree(strType)
	arr := gVariantNewArray(strType, &uris[0], uintptr(len(uris)))
	startup := gVariantNewString(cstr(""))
	args := []uintptr{gVariantTakeRef(arr), gVariantTakeRef(startup)}
	defer func() {
		for _, arg := range args {
			gVariantUnrefNonZero(arg)
		}
	}()
	params := gVariantNewTuple(&args[0], uintptr(len(args)))
	var gerr uintptr
	reply := gDBusConnectionCallSync(bus, cstr(fileManagerName), cstr(fileManagerPath),
		cstr(fileManagerIface), cstr("ShowItems"), params, 0, 0, -1, 0, &gerr)
	if gerr != 0 {
		gErrorFree(gerr)
		return false
	}
	gVariantUnrefNonZero(reply)
	return true
}

// prepareLinuxShare stages files (or plain text when no files are given) and
// picks the portal or file manager route for r.
func prepareLinuxShare(r driver.ShareRequest, stageDir string) (linuxSharePlan, error) {
	if len(r.Files) == 0 {
		if u := linuxShareURIForText(r.Text); u != "" {
			return linuxSharePlan{kind: linuxShareURI, uri: u}, nil
		}
		p, err := (driver.SharedFile{Name: "share.txt", Data: []byte(r.Text), MIME: "text/plain"}).Stage(stageDir)
		if err != nil {
			return linuxSharePlan{}, err
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return linuxSharePlan{}, err
		}
		return linuxSharePlan{kind: linuxShareSingleFile, paths: []string{abs}}, nil
	}
	paths := make([]string, 0, len(r.Files))
	for _, f := range r.Files {
		p, err := f.Stage(stageDir)
		if err != nil {
			return linuxSharePlan{}, err
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return linuxSharePlan{}, err
		}
		paths = append(paths, abs)
	}
	if len(paths) == 1 {
		return linuxSharePlan{kind: linuxShareSingleFile, paths: paths}, nil
	}
	return linuxSharePlan{kind: linuxShareMultiFiles, paths: paths}, nil
}

// linuxShareURIForText returns s trimmed when it is a standalone http, https
// or mailto URI suitable for OpenURI.OpenURI.
func linuxShareURIForText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, " \t\r\n") {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return ""
		}
		return s
	case "mailto":
		if u.Opaque == "" {
			return ""
		}
		return s
	default:
		return ""
	}
}

// linuxFileURI formats an absolute path as a file:/// URI for FileManager1.
func linuxFileURI(path string) string {
	return "file://" + (&url.URL{Path: path}).EscapedPath()
}

//go:build windows

package windows

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWebEnvironmentOptionsRegisterSchemes(t *testing.T) {
	o := newWebEnvironmentOptions([]string{"app", "gallery"})
	opts := (*comObject)(unsafe.Pointer(o))
	if v := opts.getString(7); v != webTargetCompatibleVersion { // get_TargetCompatibleBrowserVersion
		t.Errorf("target version %q", v)
	}
	if options5 := parseGUID("0ae35d64-c47f-4464-814e-259c345d1501"); opts.query(&options5) != nil {
		t.Error("options claim ICoreWebView2EnvironmentOptions5")
	}
	opts4 := opts.query(&iidEnvironmentOptions4)
	if opts4 == nil {
		t.Fatal("no ICoreWebView2EnvironmentOptions4")
	}
	var count uint32
	var list *[2]*comObject
	if hr := opts4.call(3, uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&list))); failed(hr) || count != 2 {
		t.Fatalf("GetCustomSchemeRegistrations = %#x, %d", hr, count)
	}
	for i, want := range []string{"app", "gallery"} {
		r := list[i]
		if name := r.getString(3); name != want {
			t.Errorf("scheme %d = %q, want %q", i, name, want)
		}
		if !r.getBool(4) || !r.getBool(8) {
			t.Errorf("scheme %q is not secure with an authority", want)
		}
		r.release()
	}
	coTaskMemFree.Call(uintptr(unsafe.Pointer(list)))
	opts4.release()
	o.release()
	webEnvironmentObjectsMu.Lock()
	defer webEnvironmentObjectsMu.Unlock()
	if len(webEnvironmentObjects) != 0 {
		t.Errorf("%d objects still held", len(webEnvironmentObjects))
	}
}

// TestWebReasonPhraseIsNeverEmpty holds every status a scheme handler can answer to a reason phrase, as CreateWebResourceResponse needs one.
func TestWebReasonPhraseIsNeverEmpty(t *testing.T) {
	for status := int64(100); status < 600; status++ {
		if webReasonPhrase(status) == "" {
			t.Errorf("status %d has no reason phrase", status)
		}
	}
	if got := webReasonPhrase(405); got != "Method Not Allowed" {
		t.Errorf("405 = %q", got)
	}
}

func TestWebRequestHeadersAreLines(t *testing.T) {
	got := webRequestHeaders(map[string]string{"X-B": "2", "X-A": "1", "Bad\r\nName": "x", "X-C": "a\r\nInjected: y", "": "z"})
	if want := "X-A: 1\r\nX-B: 2"; got != want {
		t.Errorf("webRequestHeaders = %q, want %q", got, want)
	}
	if got := webRequestHeaders(nil); got != "" {
		t.Errorf("webRequestHeaders(nil) = %q", got)
	}
}

func TestWebDownloadFilenameStaysInDownloadDirectory(t *testing.T) {
	for _, test := range []struct {
		path, want string
	}{
		{`C:\Users\me\Downloads\report.pdf`, "report.pdf"},
		{`..\..\secret.txt`, "secret.txt"},
		{`..`, "download"},
		{`.`, "download"},
		{``, "download"},
		{`C:\`, "download"},
		{`C:`, "download"},
	} {
		if got := webDownloadFilename(test.path); got != test.want {
			t.Errorf("webDownloadFilename(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}

type fakeDownloadOp struct {
	vtbl         *[19]uintptr
	refs         int32
	uri          string
	state        int32
	bytes        int64
	bytesFail    bool
	interrupt    int32
	cancelled    bool
	stateHandler *comObject
}

type fakeDownloadArgs struct {
	vtbl           *[10]uintptr
	op             *fakeDownloadOp
	defaultPath    string
	resultPath     string
	putPathFail    bool
	putHandledFail bool
	cancelled      bool
	handled        bool
}

var (
	fakeDownloadOpVtbl = [19]uintptr{
		comQueryInterface:       syscall.NewCallback(func(*fakeDownloadOp, *guid, *unsafe.Pointer) uintptr { return eNoInterface }),
		comAddRef:               syscall.NewCallback(fakeDownloadOpAddRef),
		comRelease:              syscall.NewCallback(fakeDownloadOpRelease),
		downloadAddStateChanged: syscall.NewCallback(fakeDownloadOpAddStateChanged),
		downloadGetURI:          syscall.NewCallback(fakeDownloadOpGetURI),
		downloadGetBytes:        syscall.NewCallback(fakeDownloadOpGetBytes),
		downloadGetState:        syscall.NewCallback(fakeDownloadOpGetState),
		downloadGetInterrupt:    syscall.NewCallback(fakeDownloadOpGetInterrupt),
		downloadCancel:          syscall.NewCallback(fakeDownloadOpCancel),
	}
	fakeDownloadArgsVtbl = [10]uintptr{
		comQueryInterface:         syscall.NewCallback(func(*fakeDownloadArgs, *guid, *unsafe.Pointer) uintptr { return eNoInterface }),
		comAddRef:                 syscall.NewCallback(func(*fakeDownloadArgs) uintptr { return 1 }),
		comRelease:                syscall.NewCallback(func(*fakeDownloadArgs) uintptr { return 1 }),
		downloadArgsGetOperation:  syscall.NewCallback(fakeDownloadArgsGetOperation),
		downloadArgsPutCancel:     syscall.NewCallback(fakeDownloadArgsPutCancel),
		downloadArgsGetResultPath: syscall.NewCallback(fakeDownloadArgsGetResultPath),
		downloadArgsPutResultPath: syscall.NewCallback(fakeDownloadArgsPutResultPath),
		downloadArgsPutHandled:    syscall.NewCallback(fakeDownloadArgsPutHandled),
	}
)

func fakeDownloadOpAddRef(op *fakeDownloadOp) uintptr {
	op.refs++
	return uintptr(op.refs)
}

func fakeDownloadOpRelease(op *fakeDownloadOp) uintptr {
	op.refs--
	if op.refs == 0 && op.stateHandler != nil {
		op.stateHandler.release()
		op.stateHandler = nil
	}
	return uintptr(op.refs)
}

func fakeDownloadOpAddStateChanged(op *fakeDownloadOp, handler *comObject, token *eventToken) uintptr {
	if handler != nil {
		handler.addRef()
		op.stateHandler = handler
	}
	if token != nil {
		token.value = 1
	}
	return sOK
}

func fakeDownloadOpGetURI(op *fakeDownloadOp, out *uintptr) uintptr {
	return putString(out, op.uri)
}

func fakeDownloadOpGetBytes(op *fakeDownloadOp, out *int64) uintptr {
	if op.bytesFail {
		return eFail
	}
	*out = op.bytes
	return sOK
}

func fakeDownloadOpGetState(op *fakeDownloadOp, out *int32) uintptr {
	*out = op.state
	return sOK
}

func fakeDownloadOpGetInterrupt(op *fakeDownloadOp, out *int32) uintptr {
	*out = op.interrupt
	return sOK
}

func fakeDownloadOpCancel(op *fakeDownloadOp) uintptr {
	op.cancelled = true
	return sOK
}

func fakeDownloadArgsGetOperation(a *fakeDownloadArgs, out **comObject) uintptr {
	if a.op == nil {
		*out = nil
		return eFail
	}
	a.op.refs++
	*out = (*comObject)(unsafe.Pointer(a.op))
	return sOK
}

func fakeDownloadArgsPutCancel(a *fakeDownloadArgs, cancel int32) uintptr {
	a.cancelled = cancel != 0
	return sOK
}

func fakeDownloadArgsGetResultPath(a *fakeDownloadArgs, out *uintptr) uintptr {
	return putString(out, a.defaultPath)
}

func fakeDownloadArgsPutResultPath(a *fakeDownloadArgs, path *uint16) uintptr {
	if a.putPathFail {
		return eFail
	}
	a.resultPath = wideString(path)
	return sOK
}

func fakeDownloadArgsPutHandled(a *fakeDownloadArgs, handled int32) uintptr {
	if a.putHandledFail {
		return eFail
	}
	a.handled = handled != 0
	return sOK
}

func (op *fakeDownloadOp) fireStateChanged() {
	if op.stateHandler != nil {
		op.stateHandler.call(3, uintptr(unsafe.Pointer(op)), 0)
	}
}

func newTestDownloadWebView(downloadDir string, events *[]driver.WebDownload) *webView {
	w := &window{webs: map[*webView]bool{}}
	v := &view{id: 7, events: driver.EventMask(1 << driver.EventDownload)}
	wv := &webView{
		w:      w,
		v:      v,
		config: driver.WebConfig{DownloadDir: downloadDir},
		dispatch: func(e driver.Event) {
			if e.Kind == driver.EventDownload {
				if dl, ok := e.Data.(*driver.WebDownload); ok && dl != nil {
					*events = append(*events, *dl)
				}
			}
		},
	}
	w.webs[wv] = true
	return wv
}

func TestWebDownloadLifecycle(t *testing.T) {
	t.Run("cancels without DownloadDir", func(t *testing.T) {
		var events []driver.WebDownload
		wv := newTestDownloadWebView("", &events)
		op := &fakeDownloadOp{vtbl: &fakeDownloadOpVtbl, uri: "https://example.com/a.txt"}
		args := &fakeDownloadArgs{vtbl: &fakeDownloadArgsVtbl, op: op, defaultPath: `C:\Downloads\a.txt`}
		wv.downloadStarted((*comObject)(unsafe.Pointer(args)))
		if !args.cancelled || args.handled || len(events) != 0 || op.refs != 0 {
			t.Fatalf("cancelled=%v handled=%v events=%+v refs=%d", args.cancelled, args.handled, events, op.refs)
		}
	})

	t.Run("cancels when closed", func(t *testing.T) {
		var events []driver.WebDownload
		wv := newTestDownloadWebView(t.TempDir(), &events)
		wv.closed = true
		op := &fakeDownloadOp{vtbl: &fakeDownloadOpVtbl, uri: "https://example.com/a.txt"}
		args := &fakeDownloadArgs{vtbl: &fakeDownloadArgsVtbl, op: op, defaultPath: `C:\Downloads\a.txt`}
		wv.downloadStarted((*comObject)(unsafe.Pointer(args)))
		if !args.cancelled || len(events) != 0 {
			t.Fatalf("cancelled=%v events=%+v", args.cancelled, events)
		}
	})

	t.Run("completes and reports bytes", func(t *testing.T) {
		var events []driver.WebDownload
		dir := t.TempDir()
		wv := newTestDownloadWebView(dir, &events)
		wv.aborted = 42
		op := &fakeDownloadOp{vtbl: &fakeDownloadOpVtbl, uri: "https://example.com/report.pdf", bytes: 1234}
		args := &fakeDownloadArgs{vtbl: &fakeDownloadArgsVtbl, op: op, defaultPath: `C:\Users\me\Downloads\report.pdf`}
		wv.downloadStarted((*comObject)(unsafe.Pointer(args)))
		if wv.aborted != 0 {
			t.Errorf("aborted = %d, want 0", wv.aborted)
		}
		if args.cancelled || !args.handled {
			t.Fatalf("cancelled=%v handled=%v", args.cancelled, args.handled)
		}
		if len(events) != 1 || events[0].State != driver.WebDownloadStarted || events[0].Filename != "report.pdf" || events[0].URL != op.uri {
			t.Fatalf("started event = %+v", events)
		}
		if args.resultPath != events[0].Path || !strings.HasPrefix(args.resultPath, dir) {
			t.Fatalf("resultPath = %q, event Path = %q, dir = %q", args.resultPath, events[0].Path, dir)
		}
		if _, err := os.Stat(filepath.Dir(args.resultPath)); err != nil {
			t.Fatalf("download dir not created: %v", err)
		}
		op.state = 2 // COMPLETED
		op.fireStateChanged()
		if len(events) != 2 || events[1].State != driver.WebDownloadFinished || events[1].Bytes != 1234 || events[1].ID != events[0].ID {
			t.Fatalf("finished events = %+v", events)
		}
		if op.refs != 0 || len(wv.downloads) != 0 {
			t.Fatalf("refs=%d active=%d", op.refs, len(wv.downloads))
		}
	})

	t.Run("falls back to stat when get_BytesReceived fails", func(t *testing.T) {
		var events []driver.WebDownload
		dir := t.TempDir()
		wv := newTestDownloadWebView(dir, &events)
		op := &fakeDownloadOp{vtbl: &fakeDownloadOpVtbl, uri: "https://example.com/data.bin", bytesFail: true}
		args := &fakeDownloadArgs{vtbl: &fakeDownloadArgsVtbl, op: op, defaultPath: `C:\Downloads\data.bin`}
		wv.downloadStarted((*comObject)(unsafe.Pointer(args)))
		if err := os.WriteFile(args.resultPath, []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		op.state = 2
		op.fireStateChanged()
		if len(events) != 2 || events[1].State != driver.WebDownloadFinished || events[1].Bytes != 5 {
			t.Fatalf("events = %+v", events)
		}
	})

	t.Run("interrupted reports failure", func(t *testing.T) {
		var events []driver.WebDownload
		wv := newTestDownloadWebView(t.TempDir(), &events)
		op := &fakeDownloadOp{vtbl: &fakeDownloadOpVtbl, uri: "https://example.com/big.zip"}
		args := &fakeDownloadArgs{vtbl: &fakeDownloadArgsVtbl, op: op, defaultPath: `C:\Downloads\big.zip`}
		wv.downloadStarted((*comObject)(unsafe.Pointer(args)))
		op.state = 1 // INTERRUPTED
		op.interrupt = 14
		op.fireStateChanged()
		if len(events) != 2 || events[1].State != driver.WebDownloadFailed || !strings.Contains(events[1].Message, "Reason 14") {
			t.Fatalf("events = %+v", events)
		}
		if op.refs != 0 || len(wv.downloads) != 0 {
			t.Fatalf("refs=%d active=%d", op.refs, len(wv.downloads))
		}
	})

	t.Run("mkdir failure reports failure and cancels", func(t *testing.T) {
		var events []driver.WebDownload
		blocker := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		wv := newTestDownloadWebView(blocker, &events)
		op := &fakeDownloadOp{vtbl: &fakeDownloadOpVtbl, uri: "https://example.com/a.txt"}
		args := &fakeDownloadArgs{vtbl: &fakeDownloadArgsVtbl, op: op, defaultPath: `C:\Downloads\a.txt`}
		wv.downloadStarted((*comObject)(unsafe.Pointer(args)))
		if !args.cancelled || op.refs != 0 || len(wv.downloads) != 0 {
			t.Fatalf("cancelled=%v refs=%d active=%d", args.cancelled, op.refs, len(wv.downloads))
		}
		if len(events) != 1 || events[0].State != driver.WebDownloadFailed || events[0].Message == "" || events[0].URL != op.uri {
			t.Fatalf("events = %+v", events)
		}
	})

	t.Run("put_ResultFilePath failure reports failure and cancels", func(t *testing.T) {
		var events []driver.WebDownload
		wv := newTestDownloadWebView(t.TempDir(), &events)
		op := &fakeDownloadOp{vtbl: &fakeDownloadOpVtbl, uri: "https://example.com/a.txt"}
		args := &fakeDownloadArgs{vtbl: &fakeDownloadArgsVtbl, op: op, defaultPath: `C:\Downloads\a.txt`, putPathFail: true}
		wv.downloadStarted((*comObject)(unsafe.Pointer(args)))
		if !args.cancelled || op.refs != 0 || len(wv.downloads) != 0 {
			t.Fatalf("cancelled=%v refs=%d active=%d", args.cancelled, op.refs, len(wv.downloads))
		}
		if len(events) != 1 || events[0].State != driver.WebDownloadFailed || events[0].Message == "" {
			t.Fatalf("events = %+v", events)
		}
	})

	t.Run("close cancels active download", func(t *testing.T) {
		var events []driver.WebDownload
		wv := newTestDownloadWebView(t.TempDir(), &events)
		op := &fakeDownloadOp{vtbl: &fakeDownloadOpVtbl, uri: "https://example.com/stream.bin"}
		args := &fakeDownloadArgs{vtbl: &fakeDownloadArgsVtbl, op: op, defaultPath: `C:\Downloads\stream.bin`}
		wv.downloadStarted((*comObject)(unsafe.Pointer(args)))
		handler := op.stateHandler
		if handler == nil {
			t.Fatal("StateChanged handler was not subscribed")
		}
		handler.addRef()
		defer handler.release()
		wv.close()
		if !op.cancelled || op.refs != 0 || len(wv.downloads) != 0 {
			t.Fatalf("cancelled=%v refs=%d active=%d", op.cancelled, op.refs, len(wv.downloads))
		}
		// A late state notification after close must report nothing.
		op.state = 1
		handler.call(3, uintptr(unsafe.Pointer(op)), 0)
		if len(events) != 1 {
			t.Fatalf("events after close = %+v", events)
		}
	})
}

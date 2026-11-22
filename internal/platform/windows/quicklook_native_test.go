//go:build windows

package windows

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestRegisteredTextPreviewHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("uses the system's registered file preview handler")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if hr, _, _ := coInitializeEx.Call(0, 2); failed(hr) {
		t.Fatal(hresultError("initialize COM", hr))
	}
	defer ole32.NewProc("CoUninitialize").Call()
	file := filepath.Join(t.TempDir(), "NSGo preview 1372.txt")
	if err := os.WriteFile(file, []byte("NSGo native file preview 1372"), 0600); err != nil {
		t.Fatal(err)
	}
	frame := newPreviewFrame(0)
	defer previewFrameRelease(frame)
	handler, err := filePreviewHandler(file, frame)
	if err != nil {
		t.Fatal(err)
	}
	defer releasePreviewHandler(handler)
	instance, _, _ := getModuleHandleW.Call(0)
	hwnd, _, err := createWindowExW.Call(0, uintptr(unsafe.Pointer(wide("STATIC"))),
		uintptr(unsafe.Pointer(wide("NSGo preview 1372"))), 0x00cf0000,
		0, 0, 640, 480, 0, 0, instance, 0)
	if hwnd == 0 {
		t.Fatalf("create the preview's window: %v", err)
	}
	defer destroyWindow.Call(hwnd)
	defer handler.call(previewUnload)
	frame.hwnd.Store(hwnd)
	area := rect{Right: 640, Bottom: 480}
	if hr := handler.call(previewSetWindow, hwnd, uintptr(unsafe.Pointer(&area))); failed(hr) {
		t.Fatal(hresultError("set the preview's window", hr))
	}
	if hr := handler.call(previewRender); failed(hr) {
		t.Fatal(hresultError("render the preview", hr))
	}
	if hr := handler.call(previewSetRect, uintptr(unsafe.Pointer(&rect{Right: 320, Bottom: 240}))); failed(hr) {
		t.Fatal(hresultError("resize the preview", hr))
	}
	if hr := handler.call(previewUnload); failed(hr) {
		t.Fatal(hresultError("unload the preview", hr))
	}
}

// TestPreviewFrameCOM checks the actual Win32 table ABI and invokes the site's vtable, as a native handler does across COM's process boundary.
func TestPreviewFrameCOM(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	frame := newPreviewFrame(0)
	object := (*comObject)(unsafe.Pointer(frame))
	defer previewFrameRelease(frame)
	queried := object.query(&iidPreviewFrame)
	if queried == nil || queried != object {
		t.Fatal("the site does not expose IPreviewHandlerFrame")
	}
	queried.release()
	if unknown := object.query(&iidPreviewItem); unknown != nil {
		unknown.release()
		t.Fatal("the site exposed an unrelated interface")
	}
	runtime.GC()
	if _, ok := previewFrames.Load(frame.ptr()); !ok {
		t.Fatal("a live native site was unpinned")
	}
	var info previewFrameInfo
	if hr := object.call(3, uintptr(unsafe.Pointer(&info))); failed(hr) {
		t.Fatal(hresultError("read the native accelerator table", hr))
	}
	defer user32.NewProc("DestroyAcceleratorTable").Call(info.Accelerators)
	var table [6]previewAccelerator
	n, _, _ := user32.NewProc("CopyAcceleratorTableW").Call(info.Accelerators,
		uintptr(unsafe.Pointer(&table[0])), uintptr(len(table)))
	if n != 6 || info.Count != 6 {
		t.Fatalf("native accelerator counts = %d, %d", n, info.Count)
	}
	for i, key := range []uint16{0x1b, 0x20, 0x25, 0x26, 0x27, 0x28} {
		if table[i].Key != key || table[i].Flags&1 == 0 {
			t.Fatalf("native accelerator %d = %+v", i, table[i])
		}
	}
	if hr := object.call(4, uintptr(unsafe.Pointer(&msg{Message: wmKeyDown, WParam: 0x1b}))); hr != 1 {
		t.Fatalf("detached site's accelerator result = %#x", hr)
	}
}

// TestNativePreviewSelectionAndClose uses the registered text previewer, including its separate-process keyboard forwarding through our COM site.
func TestNativePreviewSelectionAndClose(t *testing.T) {
	if testing.Short() {
		t.Skip("uses the system's registered file preview handler")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if hr, _, _ := coInitializeEx.Call(0, 2); failed(hr) {
		t.Fatal(hresultError("initialize COM", hr))
	}
	defer ole32.NewProc("CoUninitialize").Call()
	paths := []string{filepath.Join(t.TempDir(), "first #日本.txt"), filepath.Join(t.TempDir(), "second.txt")}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte(filepath.Base(path)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	instance, _, _ := getModuleHandleW.Call(0)
	hwnd, _, err := createWindowExW.Call(0, uintptr(unsafe.Pointer(wide("STATIC"))), 0,
		0x00cf0000, 0, 0, 240, 120, 0, 0, instance, 0)
	if hwnd == 0 {
		t.Fatalf("create the owner window: %v", err)
	}
	defer destroyWindow.Call(hwnd)
	w := &window{hwnd: hwnd}
	defer w.closeQuickLook()
	w.quickLook(&driver.QuickLookRequest{Paths: paths})
	p := activePreview
	if p == nil || p.handler == nil {
		t.Fatal("the registered handler did not render the first file")
	}
	peek := user32.NewProc("PeekMessageW")
	pump := func(until func() bool) {
		t.Helper()
		for deadline := time.Now().Add(10 * time.Second); !until(); time.Sleep(time.Millisecond) {
			var m msg
			for {
				ok, _, _ := peek.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
				if ok == 0 {
					break
				}
				if !previewMessage(&m) {
					translateMessage.Call(uintptr(unsafe.Pointer(&m)))
					dispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
				}
			}
			if time.Now().After(deadline) {
				t.Fatal("the native handler did not forward the preview key")
			}
		}
	}
	focus := func() uintptr {
		t.Helper()
		var hwnd uintptr
		if hr := p.handler.call(8, uintptr(unsafe.Pointer(&hwnd))); failed(hr) || hwnd == 0 {
			t.Fatal(hresultError("find the native preview's focused control", hr))
		}
		return hwnd
	}
	text := func(index int) {
		t.Helper()
		var buf [1024]uint16
		var count uintptr
		// WM_GETTEXT is marshalled by Windows to the handler's native edit control, so this reads the rendered file, not our request.
		ok, _, err := user32.NewProc("SendMessageTimeoutW").Call(focus(), 0x0d, uintptr(len(buf)),
			uintptr(unsafe.Pointer(&buf[0])), 2, 2000, uintptr(unsafe.Pointer(&count)))
		if ok == 0 {
			t.Fatalf("read the native preview text: %v", err)
		}
		got := syscall.UTF16ToString(buf[:])
		if want := filepath.Base(paths[index]); !strings.Contains(got, want) {
			t.Fatalf("native preview text = %q, want %q", got, want)
		}
		t.Logf("native preview text: %q", got)
	}
	send := func(key uintptr) {
		t.Helper()
		if ok, _, err := postMessageW.Call(focus(), wmKeyDown, key, 1); ok == 0 {
			t.Fatalf("send the native preview key: %v", err)
		}
	}
	text(0)
	send(0x27)
	pump(func() bool { return p.request.Index == 1 || p.hwnd == 0 })
	if p.hwnd == 0 || p.handler == nil {
		t.Fatal("switching the file closed the panel")
	}
	text(1)
	w.quickLook(&driver.QuickLookRequest{Paths: paths})
	if p.request.Index != 0 || p.handler == nil {
		t.Fatal("an app update did not replace the preview file")
	}
	text(0)
	send(0x1b)
	pump(func() bool { return p.hwnd == 0 })
	if activePreview != nil || p.handler != nil || p.frame != nil {
		t.Fatal("closing the panel retained its native handler")
	}
	w.quickLook(&driver.QuickLookRequest{Paths: paths, Index: 1})
	p = activePreview
	if p == nil || p.handler == nil {
		t.Fatal("reopening the native preview failed")
	}
	text(1)
	w.closeQuickLook()
	if activePreview != nil || p.hwnd != 0 || p.handler != nil || p.frame != nil {
		t.Fatal("owner cleanup retained the preview window or its handler")
	}
}

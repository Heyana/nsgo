//go:build windows

package windows

import (
	"errors"
	"log"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Windows' registered preview handlers, hosted out of process by COM's
// preview surrogate, render into a native window the app supplies.
var (
	iidPreviewHandler = parseGUID("8895B1C6-B41F-4C1C-A562-0D564250836F")
	iidPreviewStream  = parseGUID("B824B49D-22AC-4161-AC8A-9916E8FA3F7F")
	iidPreviewItem    = parseGUID("7F73BE3F-FB79-493C-A6C7-7EE14E245841")
	iidPreviewFile    = parseGUID("B7D14566-0509-4CCE-A71F-0A554233BD9B")
	previewShlwapi    = syscall.NewLazyDLL("shlwapi.dll")
	previewAssoc      = previewShlwapi.NewProc("AssocQueryStringW")
	previewStream     = previewShlwapi.NewProc("SHCreateStreamOnFileEx")
	previewClassID    = ole32.NewProc("CLSIDFromString")
)

const (
	previewSetWindow = 3
	previewSetRect   = 4
	previewRender    = 5
	previewUnload    = 6
	previewSetFocus  = 7
	previewTranslate = 9
)

// filePreviewHandler creates and initializes the handler registered for
// path. Stream initialization is preferred, as Microsoft's host contract
// requires; item and file initializers support the other native handlers.
func filePreviewHandler(path string, frame *previewFrame) (*comObject, error) {
	const assocShellExtension = 16
	var class [128]uint16
	n := uint32(len(class))
	hr, _, _ := previewAssoc.Call(0, assocShellExtension,
		uintptr(unsafe.Pointer(wide(filepath.Ext(path)))),
		uintptr(unsafe.Pointer(wide("{8895B1C6-B41F-4C1C-A562-0D564250836F}"))),
		uintptr(unsafe.Pointer(&class[0])), uintptr(unsafe.Pointer(&n)))
	if failed(hr) {
		return nil, hresultError("find the file's preview handler", hr)
	}
	var clsid guid
	hr, _, _ = previewClassID.Call(uintptr(unsafe.Pointer(&class[0])), uintptr(unsafe.Pointer(&clsid)))
	if failed(hr) {
		return nil, hresultError("read the preview handler's class", hr)
	}
	var handler *comObject
	hr, _, _ = coCreateInstance.Call(uintptr(unsafe.Pointer(&clsid)), 0, clsctxLocalServer,
		uintptr(unsafe.Pointer(&iidPreviewHandler)), uintptr(unsafe.Pointer(&handler)))
	if failed(hr) {
		return nil, hresultError("create the preview handler", hr)
	}
	if site := handler.query(&iidObjectWithSite); site != nil {
		hr = site.call(3, frame.ptr())
		site.release()
		if failed(hr) {
			releasePreviewHandler(handler)
			return nil, hresultError("set the preview handler's site", hr)
		}
	} else {
		handler.release()
		return nil, errors.New("the preview handler has no host site")
	}
	if err := initializePreview(handler, path); err != nil {
		releasePreviewHandler(handler)
		return nil, err
	}
	return handler, nil
}

func initializePreview(handler *comObject, path string) error {
	if init := handler.query(&iidPreviewStream); init != nil {
		defer init.release()
		var stream *comObject
		const readShared, normalFile = 0x40, 0x80
		hr, _, _ := previewStream.Call(uintptr(unsafe.Pointer(wide(path))), readShared, normalFile,
			0, 0, uintptr(unsafe.Pointer(&stream)))
		if failed(hr) {
			return hresultError("open the preview's stream", hr)
		}
		defer stream.release()
		if hr = init.call(3, stream.ptr(), 0); failed(hr) {
			return hresultError("initialize the preview's stream", hr)
		}
		return nil
	}
	if init := handler.query(&iidPreviewItem); init != nil {
		defer init.release()
		var item *comObject
		hr, _, _ := shCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(wide(path))), 0,
			uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item)))
		if failed(hr) {
			return hresultError("open the preview's shell item", hr)
		}
		defer item.release()
		if hr = init.call(3, item.ptr(), 0); failed(hr) {
			return hresultError("initialize the preview's shell item", hr)
		}
		return nil
	}
	if init := handler.query(&iidPreviewFile); init != nil {
		defer init.release()
		if hr := init.call(3, uintptr(unsafe.Pointer(wide(path))), 0); failed(hr) {
			return hresultError("initialize the preview's file", hr)
		}
		return nil
	}
	return errors.New("the preview handler has no supported initializer")
}

// filePreview is the app's native preview window. Its content belongs to
// the registered handler; the host owns its size, file selection and life.
type filePreview struct {
	owner    *window
	hwnd     uintptr
	handler  *comObject
	frame    *previewFrame
	request  driver.QuickLookRequest
	revision uint64
}

var (
	activePreview *filePreview
	previewChild  = user32.NewProc("IsChild")
	previewClass  = sync.OnceValue(func() *uint16 {
		instance, _, _ := getModuleHandleW.Call(0)
		name := wide("NsgoFilePreview")
		cursor, _, _ := loadCursorW.Call(0, idcArrow)
		wc := wndClassEx{WndProc: syscall.NewCallback(previewWindowProc), Instance: instance,
			ClassName: name, Cursor: cursor}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if atom, _, _ := registerClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
			return nil
		}
		return name
	})
)

func (w *window) quickLook(request *driver.QuickLookRequest) {
	if request == nil || len(request.Paths) == 0 || !w.showsOwnWindow() {
		return
	}
	p := activePreview
	if p != nil && p.owner != w {
		p.close()
		p = nil
	}
	if p == nil {
		name := previewClass()
		if name == nil {
			return
		}
		instance, _, _ := getModuleHandleW.Call(0)
		dpi, _, _ := getDpiForWindow.Call(w.hwnd)
		if dpi == 0 {
			dpi = 96
		}
		const overlappedWindow, toolWindow, defaultPosition = 0x00cf0000, 0x80, 0x80000000
		hwnd, _, err := createWindowExW.Call(toolWindow, uintptr(unsafe.Pointer(name)), 0,
			overlappedWindow, defaultPosition, defaultPosition, 640*dpi/96, 480*dpi/96,
			w.hwnd, 0, instance, 0)
		if hwnd == 0 {
			log.Printf("nsgo: create the file preview window: %v", err)
			return
		}
		p = &filePreview{owner: w, hwnd: hwnd}
		activePreview = p
	}
	p.request = *request
	p.request.Paths = slices.Clone(request.Paths)
	p.show(min(max(request.Index, 0), len(request.Paths)-1))
}

func (p *filePreview) show(index int) {
	if index < 0 || index >= len(p.request.Paths) {
		return
	}
	p.revision++
	revision := p.revision
	path := p.request.Paths[index]
	frame := newPreviewFrame(p.hwnd)
	handler, err := filePreviewHandler(path, frame)
	if err != nil {
		frame.hwnd.Store(0)
		previewFrameRelease(frame)
		log.Printf("nsgo: file preview: %v", err)
		if p.hwnd != 0 && activePreview == p && p.revision == revision {
			p.close()
		}
		return
	}
	if p.hwnd == 0 || activePreview != p || p.revision != revision {
		frame.hwnd.Store(0)
		releasePreviewHandler(handler)
		previewFrameRelease(frame)
		return
	}
	p.unload()
	if p.hwnd == 0 || activePreview != p || p.revision != revision {
		frame.hwnd.Store(0)
		releasePreviewHandler(handler)
		previewFrameRelease(frame)
		return
	}
	p.handler, p.frame = handler, frame
	// A native RPC may pump messages that close or replace the preview.
	// Keep this call's reference until it returns and check its generation.
	handler.addRef()
	defer handler.release()
	current := func() bool { return p.hwnd != 0 && p.handler == handler && p.revision == revision }
	var area rect
	getClientRect.Call(p.hwnd, uintptr(unsafe.Pointer(&area)))
	hr := handler.call(previewSetWindow, p.hwnd, uintptr(unsafe.Pointer(&area)))
	if !current() {
		return
	}
	if failed(hr) {
		log.Printf("nsgo: file preview: %v", hresultError("set its window", hr))
		p.close()
		return
	}
	hr = handler.call(previewRender)
	if !current() {
		return
	}
	if failed(hr) {
		log.Printf("nsgo: file preview: %v", hresultError("render its file", hr))
		p.close()
		return
	}
	p.request.Index = index
	setWindowTextW.Call(p.hwnd, uintptr(unsafe.Pointer(wide(filepath.Base(path)))))
	showWindow.Call(p.hwnd, swShow)
	setForegroundWindow.Call(p.hwnd)
	handler.call(previewSetFocus)
}

func (p *filePreview) unload() {
	if handler := p.handler; handler != nil {
		frame := p.frame
		p.handler, p.frame = nil, nil
		frame.hwnd.Store(0)
		releasePreviewHandler(handler)
		previewFrameRelease(frame)
	}
}

func (p *filePreview) close() {
	p.revision++
	hwnd := p.hwnd
	p.hwnd = 0
	if activePreview == p {
		activePreview = nil
	}
	p.unload()
	if hwnd != 0 {
		destroyWindow.Call(hwnd)
	}
}

func (w *window) closeQuickLook() {
	if p := activePreview; p != nil && p.owner == w {
		p.close()
	}
}

// previewWindowProc never posts Quit: closing a preview leaves its app.
func previewWindowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	p := activePreview
	if p != nil && p.hwnd == hwnd {
		switch message {
		case wmPreviewKey:
			if p.frame != nil && p.frame.token == lParam {
				p.key(wParam)
			}
			return 0
		case wmSize:
			if p.handler != nil {
				var area rect
				getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&area)))
				p.handler.call(previewSetRect, uintptr(unsafe.Pointer(&area)))
			}
		case wmDestroy:
			p.revision++
			p.hwnd = 0
			activePreview = nil
			p.unload()
		}
	}
	r, _, _ := defWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}

// previewMessage handles the host's keys even when the handler's child
// has focus, then offers other keyboard messages to the native handler.
func previewMessage(m *msg) bool {
	p := activePreview
	if p == nil || m.Message < 0x100 || m.Message > 0x109 {
		return false
	}
	if m.Hwnd != p.hwnd {
		if child, _, _ := previewChild.Call(p.hwnd, m.Hwnd); child == 0 {
			return false
		}
	}
	if m.Message == wmKeyDown && p.key(m.WParam) {
		return true
	}
	if handler := p.handler; handler != nil {
		handler.addRef()
		defer handler.release()
		return handler.call(previewTranslate, uintptr(unsafe.Pointer(m))) == sOK
	}
	return false
}

func (p *filePreview) key(key uintptr) bool {
	switch key {
	case 0x1b, 0x20: // Escape, Space.
		p.close()
	case 0x25, 0x26: // Left, Up.
		p.show(p.request.Index - 1)
	case 0x27, 0x28: // Right, Down.
		p.show(p.request.Index + 1)
	default:
		return false
	}
	return true
}

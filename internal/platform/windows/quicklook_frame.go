//go:build windows

package windows

import (
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// IPreviewHandlerFrame receives keys from the handler's separate process. Its callbacks only post to the host: unloading a handler during its own TranslateAccelerator RPC would reenter the handler while it uses its site.
type previewFrame struct {
	vtbl  *[5]uintptr
	refs  atomic.Int32
	hwnd  atomic.Uintptr
	token uintptr
}

type previewFrameInfo struct {
	Accelerators uintptr
	Count        uint32
}

type previewAccelerator struct {
	Flags        byte
	Key, Command uint16
}

const wmPreviewKey = 0x8000 + 1 // Private to NsgoFilePreview's window class.

var (
	iidPreviewFrame   = parseGUID("FEC87AAF-35F9-447A-ADB7-20234491401A")
	iidObjectWithSite = parseGUID("FC4801A3-2BA9-11CF-A229-00AA003D7352")
	previewFrameID    atomic.Uintptr
	previewFrames     sync.Map // Pins sites until their last native COM reference.
	previewAccel      = user32.NewProc("CreateAcceleratorTableW")
	previewFrameTable = [5]uintptr{
		syscall.NewCallback(previewFrameQuery),
		syscall.NewCallback(previewFrameAddRef),
		syscall.NewCallback(previewFrameRelease),
		syscall.NewCallback(previewFrameContext),
		syscall.NewCallback(previewFrameTranslate),
	}
)

func newPreviewFrame(hwnd uintptr) *previewFrame {
	f := &previewFrame{vtbl: &previewFrameTable, token: previewFrameID.Add(1)}
	f.refs.Store(1)
	f.hwnd.Store(hwnd)
	previewFrames.Store(f.ptr(), f)
	return f
}

func (f *previewFrame) ptr() uintptr { return uintptr(unsafe.Pointer(f)) }

func previewFrameQuery(f *previewFrame, iid *guid, out *unsafe.Pointer) uintptr {
	if out == nil {
		return ePointer
	}
	*out = nil
	if iid == nil {
		return ePointer
	}
	if *iid != iidUnknown && *iid != iidPreviewFrame {
		return eNoInterface
	}
	*out = unsafe.Pointer(f)
	previewFrameAddRef(f)
	return sOK
}

func previewFrameAddRef(f *previewFrame) uintptr { return uintptr(f.refs.Add(1)) }

func previewFrameRelease(f *previewFrame) uintptr {
	n := f.refs.Add(-1)
	if n == 0 {
		previewFrames.Delete(f.ptr())
	}
	return uintptr(n)
}

func previewFrameContext(f *previewFrame, info *previewFrameInfo) uintptr {
	if info == nil {
		return ePointer
	}
	*info = previewFrameInfo{}
	// The handler owns this table and frees it when its site is replaced. Medium-integrity handlers may use it to decide which keys to forward.
	keys := [...]uint16{0x1b, 0x20, 0x25, 0x26, 0x27, 0x28}
	var table [len(keys)]previewAccelerator
	for i, key := range keys {
		table[i] = previewAccelerator{Flags: 1, Key: key}
	}
	h, _, _ := previewAccel.Call(uintptr(unsafe.Pointer(&table[0])), uintptr(len(table)))
	if h == 0 {
		return eFail
	}
	info.Accelerators, info.Count = h, uint32(len(table))
	return sOK
}

func previewKey(key uintptr) bool {
	switch key {
	case 0x1b, 0x20, 0x25, 0x26, 0x27, 0x28:
		return true
	}
	return false
}

func previewFrameTranslate(f *previewFrame, m *msg) uintptr {
	const sFalse = 1
	if m == nil {
		return ePointer
	}
	hwnd := f.hwnd.Load()
	if hwnd == 0 || m.Message != wmKeyDown || !previewKey(m.WParam) {
		return sFalse
	}
	// The site token rejects a queued key after another file has replaced the handler, even when the preview window itself is still the same.
	if ok, _, _ := postMessageW.Call(hwnd, wmPreviewKey, m.WParam, f.token); ok == 0 {
		return sFalse
	}
	return sOK
}

func releasePreviewHandler(handler *comObject) {
	handler.call(previewUnload)
	if site := handler.query(&iidObjectWithSite); site != nil {
		site.call(3, 0) // IObjectWithSite.SetSite(nil).
		site.release()
	}
	handler.release()
}

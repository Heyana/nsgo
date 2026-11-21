//go:build windows

package windows

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	enumClipboardFormats   = user32.NewProc("EnumClipboardFormats")
	getClipboardFormatName = user32.NewProc("GetClipboardFormatNameW")
	getClipboardSequence   = user32.NewProc("GetClipboardSequenceNumber")
	globalFlags            = kernel32.NewProc("GlobalFlags")
	setLastError           = kernel32.NewProc("SetLastError")
)

// Clipboard formats with byte payloads or native handles. Only the byte
// formats can be written back from a snapshot.
var standardClipboardFormats = map[uintptr]struct {
	name  string
	bytes bool
}{
	1:             {"CF_TEXT", true},
	2:             {"CF_BITMAP", false},
	3:             {"CF_METAFILEPICT", false},
	4:             {"CF_SYLK", true},
	5:             {"CF_DIF", true},
	6:             {"CF_TIFF", true},
	7:             {"CF_OEMTEXT", true},
	cfDIB:         {"CF_DIB", true},
	9:             {"CF_PALETTE", false},
	10:            {"CF_PENDATA", false},
	11:            {"CF_RIFF", true},
	12:            {"CF_WAVE", true},
	cfUnicodeText: {"CF_UNICODETEXT", true},
	14:            {"CF_ENHMETAFILE", false},
	cfHDrop:       {"CF_HDROP", true},
	16:            {"CF_LOCALE", true},
	17:            {"CF_DIBV5", true},
	0x80:          {"CF_OWNERDISPLAY", false},
	0x81:          {"CF_DSPTEXT", true},
	0x82:          {"CF_DSPBITMAP", false},
	0x83:          {"CF_DSPMETAFILEPICT", false},
	0x8e:          {"CF_DSPENHMETAFILE", false},
}

func clipboardFormatName(format uintptr) string {
	if standard, ok := standardClipboardFormats[format]; ok && standard.bytes {
		return standard.name
	}
	if format < 0xc000 || format > 0xffff {
		return ""
	}
	var name [256]uint16
	n, _, _ := getClipboardFormatName.Call(format, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	return syscall.UTF16ToString(name[:n])
}

func clipboardFormatID(name string) (uintptr, error) {
	for id, standard := range standardClipboardFormats {
		if standard.name != name {
			continue
		}
		if !standard.bytes {
			return 0, fmt.Errorf("clipboard format %q holds a native handle", name)
		}
		return id, nil
	}
	native, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	id, _, err := registerClipboardFormatW.Call(uintptr(unsafe.Pointer(native)))
	if id == 0 {
		return 0, fmt.Errorf("register clipboard format %q: %w", name, err)
	}
	return id, nil
}

func clipboardChangeCount() int64 {
	count, _, _ := getClipboardSequence.Call()
	if count == 0 {
		return -1
	}
	return int64(uint32(count))
}

func (w *window) readClipboardItems() ([]driver.ClipboardItem, error) {
	if ok, _, err := openClipboard.Call(w.hwnd); ok == 0 {
		return nil, fmt.Errorf("open clipboard: %w", err)
	}
	defer closeClipboard.Call()

	var formats []uintptr
	for format := uintptr(0); ; {
		setLastError.Call(0)
		next, _, err := enumClipboardFormats.Call(format)
		if next == 0 {
			if err != syscall.Errno(0) {
				return nil, fmt.Errorf("enumerate clipboard formats: %w", err)
			}
			break
		}
		format = next
		formats = append(formats, format)
	}

	var reps []driver.ClipboardRepresentation
	remaining := uintptr(driver.MaxClipboardImage)
	for _, format := range formats {
		name := clipboardFormatName(format)
		if name == "" {
			continue
		}
		h, _, _ := getClipboardData.Call(format)
		if h == 0 {
			continue // a delayed-rendered format its owner failed to render; keep the others
		}
		const gmemInvalidHandle = 0x8000
		if flags, _, _ := globalFlags.Call(h); flags == gmemInvalidHandle {
			continue // a registered format can be a native handle
		}
		n, _, _ := globalSize.Call(h)
		if n > remaining {
			return nil, driver.ErrClipboardQuota
		}
		remaining -= n
		data := globalBytes(h)
		if n > 0 && len(data) == 0 {
			return nil, fmt.Errorf("the clipboard's memory for %q cannot be read", name)
		}
		reps = append(reps, driver.ClipboardRepresentation{Type: name, Data: data})
	}
	if len(reps) == 0 {
		return nil, nil
	}
	return []driver.ClipboardItem{{Representations: reps}}, nil
}

func (w *window) writeClipboardItems(items []driver.ClipboardItem) error {
	type entry struct {
		format, handle uintptr
	}
	var entries []entry
	defer func() {
		for _, e := range entries {
			if e.handle != 0 {
				globalFree.Call(e.handle)
			}
		}
	}()

	// Prepare all formats and buffers before clearing the system clipboard.
	formats := map[uintptr]bool{}
	for _, item := range items {
		for _, rep := range item.Representations {
			format, err := clipboardFormatID(rep.Type)
			if err != nil {
				return err
			}
			if formats[format] {
				return fmt.Errorf("Windows clipboard items repeat format %q", rep.Type)
			}
			formats[format] = true
			const gmemZeroInit = 0x0040
			// GlobalAlloc(0) returns a discarded handle that
			// SetClipboardData rejects. Presence-only formats use a zero byte.
			size := max(1, len(rep.Data))
			h, _, err := globalAlloc.Call(gmemMoveable|gmemZeroInit, uintptr(size))
			if h == 0 {
				return fmt.Errorf("allocate clipboard format %q: %w", rep.Type, err)
			}
			entries = append(entries, entry{format, h})
			if len(rep.Data) > 0 {
				p, _, err := globalLock.Call(h)
				if p == 0 {
					return fmt.Errorf("lock clipboard format %q: %w", rep.Type, err)
				}
				rtlMoveMemory.Call(p, uintptr(unsafe.Pointer(&rep.Data[0])), uintptr(len(rep.Data)))
				globalUnlock.Call(h)
			}
		}
	}

	if ok, _, err := openClipboard.Call(w.hwnd); ok == 0 {
		return fmt.Errorf("open clipboard: %w", err)
	}
	defer closeClipboard.Call()
	if ok, _, err := emptyClipboard.Call(); ok == 0 {
		return fmt.Errorf("empty clipboard: %w", err)
	}
	for i := range entries {
		e := &entries[i]
		if ok, _, err := setClipboardData.Call(e.format, e.handle); ok == 0 {
			return fmt.Errorf("write clipboard format %q: %w", clipboardFormatName(e.format), err)
		}
		e.handle = 0 // Windows owns the buffer from here.
	}
	return nil
}

//go:build windows

package windows

import (
	"encoding/binary"
	"syscall"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The caret and pastes of text fields, from NSGo's own editor: the caret
// it draws, and the clipboard it pastes from, read as the user pastes.

var (
	globalSize               = kernel32.NewProc("GlobalSize")
	registerClipboardFormatW = user32.NewProc("RegisterClipboardFormatW")
)

const cfDIB = 8

// caretMoved reports where in's caret is, while it has the focus.
func (in *textInput) caretMoved() {
	if !in.focused || !in.fs.Events.Has(driver.EventCaretMove) {
		return
	}
	var ev driver.Event
	if x, y, h, ok := in.caretRect(); ok {
		c := in.config.Content.Origin
		ev.Point = driver.Point{X: c.X + driver.Points(x), Y: c.Y + driver.Points(y)}
		ev.Size = driver.Size{Width: driver.Points(caretWidth()), Height: driver.Points(h)}
	}
	ev.Kind, ev.ID = driver.EventCaretMove, in.v.id
	in.w.engine.Dispatch(ev)
}

// pasteEvent reports a paste into in, and whether its text goes in.
func (w *window) pasteEvent(in *textInput) bool {
	if !in.fs.Events.Has(driver.EventPaste) {
		return true
	}
	p := &driver.PasteEvent{Text: w.clipboard()}
	if r, _, _ := openClipboard.Call(w.hwnd); r != 0 {
		p.HTML = clipboardHTML()
		if h, _, _ := getClipboardData.Call(cfHDrop); h != 0 {
			p.URIs = hdropFiles(h)
		}
		if len(p.URIs) == 0 {
			p.Files = clipboardImage()
		}
		closeClipboard.Call()
	}
	w.engine.Dispatch(driver.Event{Kind: driver.EventPaste, ID: in.v.id, Data: p})
	return p.Text != "" && in.config.PasteBehavior == driver.PasteInsert
}

// cfHTMLFormat returns the registered Win32 clipboard format ID for CF_HTML ("HTML Format"), or 0 if registration fails.
func cfHTMLFormat() uintptr {
	name, _ := syscall.UTF16PtrFromString("HTML Format")
	id, _, _ := registerClipboardFormatW.Call(uintptr(unsafe.Pointer(name)))
	return id
}

// clipboardHTML returns the open clipboard's CF_HTML content, "" when it holds none.
func clipboardHTML() string {
	if cf := cfHTMLFormat(); cf != 0 {
		if h, _, _ := getClipboardData.Call(cf); h != 0 {
			return parseCFHTML(globalBytes(h))
		}
	}
	return ""
}

// clipboardImage returns the open clipboard's image: its PNG, else its
// device-independent bitmap as a BMP file.
func clipboardImage() []driver.PastedFile {
	name, _ := syscall.UTF16PtrFromString("PNG")
	if png, _, _ := registerClipboardFormatW.Call(uintptr(unsafe.Pointer(name))); png != 0 {
		if h, _, _ := getClipboardData.Call(png); h != 0 {
			if b := globalBytes(h); len(b) > 0 {
				return []driver.PastedFile{{MIME: "image/png", Data: b}}
			}
		}
	}
	h, _, _ := getClipboardData.Call(cfDIB)
	if bmp := bmpOfDIB(globalBytes(h)); bmp != nil {
		return []driver.PastedFile{{MIME: "image/bmp", Data: bmp}}
	}
	return nil
}

// bmpOfDIB wraps the clipboard's DIB — a header, its color table and its
// pixels — as a BMP file: a header that says where the pixels start. nil
// stands for data the BMP reader would not take.
func bmpOfDIB(dib []byte) []byte {
	if len(dib) < 40 {
		return nil
	}
	headerSize := binary.LittleEndian.Uint32(dib)
	if headerSize < 40 || int(headerSize) > len(dib) {
		return nil
	}
	bpp, used := binary.LittleEndian.Uint16(dib[14:]), binary.LittleEndian.Uint32(dib[32:])
	compression := binary.LittleEndian.Uint32(dib[16:])
	colors := used
	if colors == 0 && bpp <= 8 {
		colors = 1 << bpp
	}

	var body []byte
	bodyHeader := headerSize
	if headerSize == 40 {
		info := append(make([]byte, 0, 40), dib[:40]...)
		pixels := dib[40:]
		if compression == 3 {
			// A 40-byte BITMAPINFOHEADER puts the three BI_BITFIELDS masks
			// between the header and the pixels, which golang.org/x/image/bmp
			// refuses unless normalized to BI_RGB without the 12-byte gap.
			if len(dib) < 52 || bpp != 32 || !bgraMasks(dib[40:52]) {
				return nil
			}
			pixels = dib[52:]
			binary.LittleEndian.PutUint32(info[16:], 0)
		}
		body = append(info, pixels...)
	} else if compression == 3 {
		// V4/V5 headers keep the four masks inside bytes 40..56. Keep the
		// wider header intact when alpha is 0xff000000 so x/image/bmp
		// preserves transparency (allowAlpha = infoLen > 40); only rewrite
		// to a 40-byte BI_RGB header for alpha-mask-0 (BGRX), which x/image
		// refuses as BI_BITFIELDS.
		if headerSize < 56 || bpp != 32 || !bgraMasks(dib[40:headerSize]) {
			return nil
		}
		if binary.LittleEndian.Uint32(dib[52:56]) == 0 {
			info := append(make([]byte, 0, 40), dib[:40]...)
			binary.LittleEndian.PutUint32(info, 40)
			binary.LittleEndian.PutUint32(info[16:], 0)
			bodyHeader = 40
			body = append(info, dib[headerSize:]...)
		} else {
			body = dib
		}
	} else {
		// Keep wider V4/V5 BI_RGB headers intact so x/image/bmp retains alpha.
		body = dib
	}

	offset := 14 + int(bodyHeader) + int(colors)*4
	file := make([]byte, 14, 14+len(body))
	file[0], file[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(file[2:], uint32(14+len(body)))
	binary.LittleEndian.PutUint32(file[10:], uint32(offset))
	return append(file, body...)
}

// bgraMasks reports whether the masks are the plain blue, green, red and
// alpha ones 32-bit pixels are already laid out in.
func bgraMasks(masks []byte) bool {
	if len(masks) < 12 {
		return false
	}
	read := func(at int) uint32 { return binary.LittleEndian.Uint32(masks[at:]) }
	alpha := uint32(0)
	if len(masks) >= 16 {
		alpha = read(12)
	}
	return read(0) == 0x00ff0000 && read(4) == 0x0000ff00 && read(8) == 0x000000ff && (alpha == 0xff000000 || alpha == 0)
}

// globalBytes copies the memory of global handle h.
func globalBytes(h uintptr) []byte {
	if h == 0 {
		return nil
	}
	n, _, _ := globalSize.Call(h)
	p, _, _ := globalLock.Call(h)
	if p == 0 || n == 0 {
		return nil
	}
	defer globalUnlock.Call(h)
	b := make([]byte, n)
	rtlMoveMemory.Call(uintptr(unsafe.Pointer(&b[0])), p, n)
	return b
}

// hdropFiles reads an HDROP list as file: URLs.
func hdropFiles(h uintptr) []string {
	count, _, _ := dragQueryFileW.Call(h, 0xFFFFFFFF, 0, 0)
	var uris []string
	for i := range uint32(count) {
		n, _, _ := dragQueryFileW.Call(h, uintptr(i), 0, 0)
		if n == 0 {
			continue
		}
		buf := make([]uint16, n+1)
		if got, _, _ := dragQueryFileW.Call(h, uintptr(i), uintptr(unsafe.Pointer(&buf[0])), n+1); got != n {
			continue
		}
		if uri := fileURL(syscall.UTF16ToString(buf)); uri != "" {
			uris = append(uris, uri)
		}
	}
	return uris
}

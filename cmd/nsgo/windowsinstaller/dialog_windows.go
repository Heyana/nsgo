package windowsinstaller

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"sync/atomic"
	"syscall"
)

// TASKDIALOGCONFIG and TASKDIALOG_BUTTON are packed(1), including on arm64. Keep native text separately reachable while the packed pointer bytes are used. https://microsoft.github.io/windows-docs-rs/doc/windows/Win32/UI/Controls/struct.TASKDIALOGCONFIG.html
type taskDialog struct {
	data    [160]byte
	text    []*uint16
	buttons []byte
	pins    *runtime.Pinner
}

func (d *taskDialog) number(offset int, n uint32) { binary.LittleEndian.PutUint32(d.data[offset:], n) }
func (d *taskDialog) address(offset int, p any) {
	binary.LittleEndian.PutUint64(d.data[offset:], uint64(nativeWord(p)))
}
func (d *taskDialog) string(offset int, s string) {
	p := wide(s)
	d.keep(p)
	d.address(offset, ptr(p))
}
func (d *taskDialog) button(id uint32, text string) {
	b := make([]byte, 12)
	binary.LittleEndian.PutUint32(b, id)
	p := wide(text)
	d.keep(p)
	binary.LittleEndian.PutUint64(b[4:], uint64(nativeWord(ptr(p))))
	d.buttons = append(d.buttons, b...)
}
func (d *taskDialog) keep(p *uint16) {
	d.text = append(d.text, p)
	if d.pins != nil {
		d.pins.Pin(p)
	}
}
func (d *taskDialog) show() (int32, bool, error) {
	d.pins = new(runtime.Pinner)
	defer d.pins.Unpin()
	defer func() { d.pins = nil }()
	for _, text := range d.text {
		d.pins.Pin(text)
	}
	if len(d.buttons) > 0 {
		d.pins.Pin(&d.buttons[0])
	}
	d.number(0, 160)
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	d.address(12, instance)
	if resource, _, _ := kernel32.NewProc("FindResourceW").Call(instance, 1, 14); resource != 0 {
		d.address(36, 1)
	}
	if len(d.buttons) > 0 {
		d.number(60, uint32(len(d.buttons)/12))
		d.address(64, ptr(&d.buttons[0]))
	}
	var selected, checked int32
	hr, _, _ := loadLibrary("comctl32.dll").NewProc("TaskDialogIndirect").Call(ptr(&d.data[0]), ptr(&selected), 0, ptr(&checked))
	runtime.KeepAlive(d)
	return selected, checked != 0, hresult(hr)
}
func (d *taskDialog) callback(fn func(uintptr, uint32, uintptr, uintptr, uintptr) uintptr) {
	d.address(140, syscall.NewCallback(fn))
}
func dialog(title, instruction, content string, w dialogWords) *taskDialog {
	d := &taskDialog{}
	d.string(28, title)
	d.string(44, instruction)
	d.string(52, content)
	d.number(20, 8|w.layoutFlag())
	return d
}

func chooseDirectory(parent uintptr, current, title string) string {
	type browseInfo struct {
		Parent, Root   uintptr
		Display, Title *uint16
		Flags          uint32
		Callback, Data uintptr
		Image          int32
	}
	var display, path [260]uint16
	callback := syscall.NewCallback(func(hwnd uintptr, message uint32, _ uintptr, _ uintptr) uintptr {
		if message == 1 {
			user32.NewProc("SendMessageW").Call(hwnd, 1024+103, 1, ptr(wide(current)))
		}
		return 0
	})
	info := browseInfo{Parent: parent, Display: &display[0], Title: wide(title), Flags: 1 | 0x40, Callback: callback}
	var pins runtime.Pinner
	pins.Pin(info.Display)
	pins.Pin(info.Title)
	defer pins.Unpin()
	item, _, _ := shell32.NewProc("SHBrowseForFolderW").Call(ptr(&info))
	if item == 0 {
		return ""
	}
	defer ole32.NewProc("CoTaskMemFree").Call(item)
	if ok, _, _ := shell32.NewProc("SHGetPathFromIDListW").Call(item, ptr(&path[0])); ok != 0 {
		return syscall.UTF16ToString(path[:])
	}
	return ""
}
func confirmInstall(m Manifest, dir string, w dialogWords) (string, bool, bool, error) {
	d := dialog(m.Name, w.Install+" "+m.Name, dir, w)
	d.button(2, w.Cancel)
	d.string(92, w.Desktop)
	d.button(100, w.Install)
	d.button(101, w.Browse)
	d.number(72, 100)
	d.callback(func(hwnd uintptr, notification uint32, wparam, lparam, data uintptr) uintptr {
		if notification == 2 && wparam == 101 {
			if selected := chooseDirectory(hwnd, dir, w.Browse); selected != "" {
				dir = selected
				p := wide(dir)
				d.keep(p)
				user32.NewProc("SendMessageW").Call(hwnd, 1024+108, 0, ptr(p))
			}
			return 1
		}
		return 0
	})
	selected, desktop, err := d.show()
	return dir, desktop, selected == 100, err
}
func confirmUninstall(m Manifest, dir string, w dialogWords) (bool, error) {
	d := dialog(m.Name, w.Uninstall+" "+m.Name, dir, w)
	d.button(2, w.Cancel)
	d.button(100, w.Uninstall)
	selected, _, err := d.show()
	return selected == 100, err
}
func confirmRunning(m Manifest, w dialogWords) bool {
	result, _, _ := user32.NewProc("MessageBoxW").Call(0, ptr(wide(fmt.Sprintf(w.Running, m.Name))), ptr(wide(m.Name)), 0x31)
	return result == 1
}
func showFailure(name string, err error) {
	user32.NewProc("MessageBoxW").Call(0, ptr(wide(err.Error())), ptr(wide(name)), 0x10)
}
func showFinished(m Manifest, w dialogWords) (bool, error) {
	d := dialog(m.Name, m.Name, w.Done, w)
	d.button(8, w.Close)
	d.button(100, w.Open)
	selected, _, err := d.show()
	return selected == 100, err
}
func runProgress(m Manifest, w dialogWords, fn func(func(uint64, uint64)) error) error {
	d := dialog(m.Name, m.Name, w.Working, w)
	d.number(20, 0x200|0x800|0x8000|w.layoutFlag())
	d.button(1, w.Close)
	done := make(chan error, 1)
	var percent atomic.Uint32
	var result error
	finished := false
	d.callback(func(hwnd uintptr, notification uint32, wparam, lparam, data uintptr) uintptr {
		send := user32.NewProc("SendMessageW")
		switch notification {
		case 0:
			send.Call(hwnd, 1024+111, 1, 0)
			go func() {
				done <- withCOM(func() error {
					return fn(func(n, total uint64) {
						if total > 0 {
							percent.Store(uint32(n * 100 / total))
						}
					})
				})
			}()
		case 2:
			if !finished {
				return 1
			}
		case 4:
			send.Call(hwnd, 1024+106, uintptr(percent.Load()), 0)
			select {
			case result = <-done:
				finished = true
				send.Call(hwnd, 1024+111, 1, 1)
				send.Call(hwnd, 1024+102, 1, 0)
			default:
			}
		}
		return 0
	})
	_, _, err := d.show()
	if err != nil {
		return err
	}
	return result
}

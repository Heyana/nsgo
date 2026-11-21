//go:build windows

package windows

import (
	"path/filepath"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	iidOleWindow = parseGUID("00000114-0000-0000-C000-000000000046")
	messageBoxW  = user32.NewProc("MessageBoxW")
)

const (
	oleWindowGetWindow    = 3
	fileDialogGetFileName = 16
	fileDialogClose       = 23
)

// saveDialogPreflight keeps the dialog open while the engine checks the
// path. It closes the dialog for the approved path only if the edit still
// matches the one the user submitted.
type saveDialogPreflight struct {
	w               *window
	id, attempt     uint32
	pending, closed bool
	approved        string
	refusal         driver.Event
}

func (p *saveDialogPreflight) fileOK(dialog *comObject) uintptr {
	const sFalse = 1 // keep the dialog open
	path, err := saveDialogPath(dialog)
	if err != nil || path == "" || p.pending {
		return sFalse
	}
	hwnd := saveDialogWindow(dialog)
	if hwnd == 0 {
		hwnd = p.w.owner()
	}
	folder, name := saveDialogEdit(dialog)
	p.pending = true
	p.refusal = driver.Event{}
	p.attempt++
	attempt := p.attempt
	go func() {
		approved, reason := p.w.saves.Check(p.id, attempt, path, p.w.engine.Dispatch)
		p.w.post(func() {
			if p.closed {
				return
			}
			p.pending = false
			if !approved {
				p.refusal = driver.Event{Kind: driver.EventFiles, Count: p.id, Data: path, Text: reason}
				if reason == "" {
					reason = "The app refused the save destination."
				}
				const mbIconWarning = 0x30
				messageBoxW.Call(hwnd, uintptr(unsafe.Pointer(wide(reason))), uintptr(unsafe.Pointer(wide(filepath.Base(path)))), mbIconWarning)
				return
			}
			if currentFolder, currentName := saveDialogEdit(dialog); currentFolder == folder && currentName == name {
				p.approved = path
				if failed(dialog.call(fileDialogClose, sOK)) {
					p.approved = ""
				}
			}
		})
	}()
	return sFalse
}

func saveDialogWindow(dialog *comObject) uintptr {
	var hwnd uintptr
	if ole := dialog.query(&iidOleWindow); ole != nil {
		ole.call(oleWindowGetWindow, uintptr(unsafe.Pointer(&hwnd)))
		ole.release()
	}
	return hwnd
}

func saveDialogEdit(dialog *comObject) (folderPath, name string) {
	var folder *comObject
	if !failed(dialog.call(fileDialogGetFolder, uintptr(unsafe.Pointer(&folder)))) && folder != nil {
		folderPath, _ = shellItemPath(folder)
		folder.release()
	}
	var fileName *uint16
	if !failed(dialog.call(fileDialogGetFileName, uintptr(unsafe.Pointer(&fileName)))) {
		name = takeString(fileName)
	}
	return folderPath, name
}

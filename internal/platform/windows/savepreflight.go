//go:build windows

package windows

import (
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var iidFileDialogCustomize = parseGUID("E6FDD21A-163F-4975-9C8C-A69F1BA37034")

const (
	fileDialogGetFileName = 16
	fileDialogClose       = 23
	fileDialogAddText     = 11
	fileDialogSetLabel    = 12
	fileDialogSetState    = 14

	saveRefusalControl = 0x1000
	dialogControlShown = 3 // CDCS_ENABLED | CDCS_VISIBLE
)

// saveDialogPreflight keeps the dialog open while the engine checks the
// path. It closes the dialog for the approved path only if the edit still
// matches the one the user submitted.
type saveDialogPreflight struct {
	w               *window
	customize       *comObject
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
	folder, name := saveDialogEdit(dialog)
	p.pending = true
	p.refusal = driver.Event{}
	p.customize.call(fileDialogSetState, saveRefusalControl, 0)
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
				p.customize.call(fileDialogSetLabel, saveRefusalControl, uintptr(unsafe.Pointer(wide(reason))))
				p.customize.call(fileDialogSetState, saveRefusalControl, dialogControlShown)
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

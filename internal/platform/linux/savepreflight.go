//go:build linux && !android

package linux

import (
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	gDBusErrorGetRemoteError       func(uintptr) uintptr
	gDgettext                      func(domain, message *byte) *byte
	gtkFileChooserDialogNew        func(title *byte, parent uintptr, action int32, firstButton *byte) uintptr
	gtkFileChooserSetCurrentFolder func(chooser, folder, err uintptr) int32
	gtkFileChooserSetCurrentName   func(chooser uintptr, name *byte)
	gtkFileChooserGetFile          func(chooser uintptr) uintptr
	gtkDialogAddButton             func(dialog uintptr, label *byte, response int32) uintptr
	gtkDialogGetContentArea        func(dialog uintptr) uintptr
	gtkWindowSetModal              func(window uintptr, modal int32)
	cbSaveDialogResponse           uintptr

	bindSavePreflight = sync.OnceFunc(func() {
		bindInputSources()
		purego.RegisterLibFunc(&gDBusErrorGetRemoteError, gioLib, "g_dbus_error_get_remote_error")
		purego.RegisterLibFunc(&gDgettext, glibLib, "g_dgettext")
		purego.RegisterLibFunc(&gtkFileChooserDialogNew, gtkLib, "gtk_file_chooser_dialog_new")
		purego.RegisterLibFunc(&gtkFileChooserSetCurrentFolder, gtkLib, "gtk_file_chooser_set_current_folder")
		purego.RegisterLibFunc(&gtkFileChooserSetCurrentName, gtkLib, "gtk_file_chooser_set_current_name")
		purego.RegisterLibFunc(&gtkFileChooserGetFile, gtkLib, "gtk_file_chooser_get_file")
		purego.RegisterLibFunc(&gtkDialogAddButton, gtkLib, "gtk_dialog_add_button")
		purego.RegisterLibFunc(&gtkDialogGetContentArea, gtkLib, "gtk_dialog_get_content_area")
		purego.RegisterLibFunc(&gtkWindowSetModal, gtkLib, "gtk_window_set_modal")
		cbSaveDialogResponse = newCallback(saveDialogResponse)
	})
)

// fileChooserPortalAbsent checks on a worker. A sandbox or a portal
// explicitly requested by the user still uses GTK's native dialog.
func fileChooserPortalAbsent() bool {
	debug := strings.FieldsFunc(os.Getenv("GDK_DEBUG"), func(ch rune) bool { return strings.ContainsRune(" ,:;\t", ch) })
	if slices.Contains(debug, "no-portals") {
		return true
	}
	if _, err := os.Stat("/.flatpak-info"); err == nil || os.Getenv("SNAP") != "" || os.Getenv("GTK_USE_PORTAL") == "1" {
		return false
	}
	const busSession = 2
	var err uintptr
	bus := gBusGetSync(busSession, 0, &err)
	if bus == 0 {
		if err != 0 {
			gErrorFree(err)
		}
		return os.Getenv("DBUS_SESSION_BUS_ADDRESS") == ""
	}
	defer gObjectUnref(bus)
	args := [2]uintptr{
		gVariantNewString(cstr("org.freedesktop.portal.FileChooser")),
		gVariantNewString(cstr("version")),
	}
	reply := gDBusConnectionCallSync(bus, cstr(globalShortcutName), cstr(globalShortcutPath),
		cstr(dbusPropertiesIface), cstr("Get"), gVariantNewTuple(&args[0], 2), 0, 0, 2000, 0, &err)
	if reply != 0 {
		gVariantUnref(reply)
		return false
	}
	if err == 0 {
		return false
	}
	defer gErrorFree(err)
	name := gDBusErrorGetRemoteError(err)
	defer gFree(name)
	switch goString(name) {
	case "org.freedesktop.DBus.Error.ServiceUnknown", "org.freedesktop.DBus.Error.NameHasNoOwner",
		"org.freedesktop.DBus.Error.UnknownInterface", "org.freedesktop.DBus.Error.UnknownProperty",
		"org.freedesktop.DBus.Error.UnknownMethod":
		return true
	default:
		return false
	}
}

func (w *window) saveFile(r driver.SaveRequest, id uint32) {
	if !r.Preflight {
		w.showSaveFile(r, id)
		return
	}
	bindSavePreflight()
	go func() {
		local := fileChooserPortalAbsent()
		w.post(func() {
			if !w.closing {
				if local {
					w.showValidatingSaveFile(r, id)
				} else {
					w.showSaveFile(r, id)
				}
			}
		})
	}()
}

type saveDialogPreflight struct {
	r           driver.SaveRequest
	id, attempt uint32
	warning     uintptr
	pending     bool
	refusal     driver.Event
}

const (
	fileChooserSave = 1
	dialogAccept    = -3
	dialogCancel    = -6
)

// GtkFileDialog answers after it closes. When no portal is available,
// GTK's in-process chooser lets us keep the same dialog open on refusal.
func (w *window) showValidatingSaveFile(r driver.SaveRequest, id uint32) {
	domain := cstr("gtk40")
	dialog := gtkFileChooserDialogNew(gDgettext(domain, cstr("Save")), w.toplevel(), fileChooserSave, nil)
	gtkWindowSetModal(dialog, 1)
	gtkDialogAddButton(dialog, gDgettext(domain, cstr("_Cancel")), dialogCancel)
	gtkDialogAddButton(dialog, gDgettext(domain, cstr("_Save")), dialogAccept)
	if folder := dialogStartFolder(r.Start); folder != 0 {
		gtkFileChooserSetCurrentFolder(dialog, folder, 0)
		gObjectUnref(folder)
	}
	gtkFileChooserSetCurrentName(dialog, cstr(r.Name))
	warning := gtkLabelNew(nil)
	gtkLabelSetXalign(warning, 0)
	gtkLabelSetWrap(warning, 1)
	gtkWidgetSetVisible(warning, 0)
	gtkBoxAppend(gtkDialogGetContentArea(dialog), warning)
	if w.saveDialogs == nil {
		w.saveDialogs = make(map[uintptr]*saveDialogPreflight)
	}
	w.saveDialogs[dialog] = &saveDialogPreflight{r: r, id: id, warning: warning}
	gSignalConnect(dialog, "response", cbSaveDialogResponse, w.handle)
	gtkWindowPresent(dialog)
}

func saveDialogResponse(dialog uintptr, response int32, handle uintptr) {
	w := windowOf(handle)
	if w == nil || w.saveDialogs[dialog] == nil {
		return
	}
	p := w.saveDialogs[dialog]
	if response != dialogAccept {
		ev := p.refusal
		if ev.Data == nil {
			ev = driver.Event{Kind: driver.EventFiles, Count: p.id}
		}
		w.destroySaveDialog(dialog)
		w.engine.Dispatch(ev)
		return
	}
	if p.pending {
		return
	}
	path := saveDialogDestination(dialog)
	if path == "" {
		return
	}
	p.pending = true
	p.attempt++
	attempt := p.attempt
	gtkWidgetSetVisible(p.warning, 0)
	go func() {
		approved, reason := w.saves.Check(p.id, attempt, path, w.engine.Dispatch)
		w.post(func() {
			if w.saveDialogs[dialog] != p {
				return
			}
			p.pending = false
			if !approved {
				p.refusal = driver.Event{Kind: driver.EventFiles, Count: p.id, Data: path, Text: reason}
				gtkLabelSetText(p.warning, cstr(reason))
				gtkWidgetSetVisible(p.warning, 1)
				return
			}
			if saveDialogDestination(dialog) == path {
				w.destroySaveDialog(dialog)
				go func() { w.engine.Dispatch(driver.SaveResult(p.r, p.id, path)) }()
			}
		})
	}()
}

func saveDialogDestination(dialog uintptr) string {
	file := gtkFileChooserGetFile(dialog)
	if file == 0 {
		return ""
	}
	defer gObjectUnref(file)
	return filePath(file)
}

func (w *window) destroySaveDialog(dialog uintptr) {
	delete(w.saveDialogs, dialog)
	gtkWindowDestroy(dialog)
}

func (w *window) closeSaveDialogs() {
	for dialog := range w.saveDialogs {
		w.destroySaveDialog(dialog)
	}
}

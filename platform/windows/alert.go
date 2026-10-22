//go:build windows

package windows

import (
	"encoding/binary"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
)

var (
	messageBoxW         = user32.NewProc("MessageBoxW")
	getSystemDirectoryW = kernel32.NewProc("GetSystemDirectoryW")
	loadLibraryW        = kernel32.NewProc("LoadLibraryW")
	getProcAddress      = kernel32.NewProc("GetProcAddress")
	createActCtxW       = kernel32.NewProc("CreateActCtxW")
	activateActCtx      = kernel32.NewProc("ActivateActCtx")
	deactivateActCtx    = kernel32.NewProc("DeactivateActCtx")
	releaseActCtx       = kernel32.NewProc("ReleaseActCtx")
)

const (
	tdfAllowDialogCancellation   = 0x0008
	tdfPositionRelativeToWindow  = 0x1000
	tdcbfOKButton                = 0x0001
	tdWarningIcon                = 0xFFFF // MAKEINTRESOURCEW(-1)
	actCtxAssemblyDirectoryValid = 0x0004
	actCtxResourceNameValid      = 0x0008
	invalidHandle                = ^uintptr(0)

	mbOK          = 0x00
	mbOKCancel    = 0x01
	mbYesNoCancel = 0x03
	mbYesNo       = 0x04
	mbIconWarning = 0x30
	idOK          = 1
	idCancel      = 2
	idYes         = 6
	idNo          = 7

	// alertButton numbers an alert's buttons from their index.
	alertButton = 100
)

type actCtx struct {
	Size                  uint32
	Flags                 uint32
	Source                *uint16
	ProcessorArchitecture uint16
	LangID                uint16
	AssemblyDirectory     *uint16
	ResourceName          uintptr
	ApplicationName       *uint16
	Module                uintptr
}

// taskDialog is TaskDialogIndirect and the activation context it needs:
// it lives only in version 6 of comctl32, which a process without a
// manifest reaches through the manifest shell32.dll carries as resource
// 124. Both are 0 where that fails.
var taskDialog = sync.OnceValues(func() (proc, actx uintptr) {
	var dir [syscall.MAX_PATH]uint16
	if n, _, _ := getSystemDirectoryW.Call(uintptr(unsafe.Pointer(&dir[0])), uintptr(len(dir))); n == 0 || n >= uintptr(len(dir)) {
		return 0, 0
	}
	ac := actCtx{
		Flags:             actCtxAssemblyDirectoryValid | actCtxResourceNameValid,
		Source:            wide("shell32.dll"),
		AssemblyDirectory: &dir[0],
		ResourceName:      124,
	}
	ac.Size = uint32(unsafe.Sizeof(ac))
	actx, _, _ = createActCtxW.Call(uintptr(unsafe.Pointer(&ac)))
	if actx == invalidHandle {
		return 0, 0
	}
	var cookie uintptr
	if r, _, _ := activateActCtx.Call(actx, uintptr(unsafe.Pointer(&cookie))); r != 0 {
		module, _, _ := loadLibraryW.Call(uintptr(unsafe.Pointer(wide("comctl32.dll"))))
		deactivateActCtx.Call(0, cookie)
		name, _ := syscall.BytePtrFromString("TaskDialogIndirect")
		if module != 0 {
			proc, _, _ = getProcAddress.Call(module, uintptr(unsafe.Pointer(name)))
		}
	}
	if proc == 0 {
		releaseActCtx.Call(actx)
		return 0, 0
	}
	return proc, actx
})

// showAlert shows the first queued alert, unless one is showing, and
// answers it. The dialog runs a message loop of its own, so the window
// keeps applying batches meanwhile.
func (w *window) showAlert() {
	if w.alerting || len(w.alerts) == 0 {
		return
	}
	a := w.alerts[0]
	w.alerts = w.alerts[1:]
	w.alerting = true
	index := w.ask(&a)
	w.alerting = false
	w.engine.Dispatch(core.Event{Kind: core.EventAlert, Count: a.ID, Index: index})
	if len(w.alerts) > 0 {
		postMessageW.Call(w.hwnd, wmAlert, 0, 0)
	}
}

// ask shows a and returns the index of the action chosen, or -1.
func (w *window) ask(a *core.AlertConfig) int {
	cancel := -1
	destructive := false
	for i, act := range a.Actions {
		if act.Role == core.AlertCancel && cancel < 0 {
			cancel = i
		}
		destructive = destructive || act.Role == core.AlertDestructive
	}
	proc, actx := taskDialog()
	if proc == 0 {
		return w.messageBox(a, cancel, destructive)
	}

	// TASKDIALOG_BUTTON and TASKDIALOGCONFIG are packed to single bytes,
	// which Go structs cannot express.
	var buttons packed
	var labels []*uint16
	defaultButton := 0
	for i, act := range a.Actions {
		label := wide(act.Label)
		labels = append(labels, label)
		buttons.u32(uint32(alertButton + i))
		buttons.ptr(uintptr(unsafe.Pointer(label)))
		if defaultButton == 0 && act.Role == core.AlertDefault {
			defaultButton = alertButton + i
		}
	}
	var common uint32
	if len(a.Actions) == 0 {
		common = tdcbfOKButton
	}
	var icon uintptr
	if destructive {
		icon = tdWarningIcon
	}
	// Without a title the dialog shows the executable's name.
	title, instruction, content := optional(w.title), optional(a.Title), optional(a.Message)
	var c packed
	c.u32(0) // the size, set below
	c.ptr(w.hwnd)
	c.ptr(0) // instance
	c.u32(tdfAllowDialogCancellation | tdfPositionRelativeToWindow)
	c.u32(common)
	c.ptr(uintptr(unsafe.Pointer(title)))
	c.ptr(icon)
	c.ptr(uintptr(unsafe.Pointer(instruction)))
	c.ptr(uintptr(unsafe.Pointer(content)))
	c.u32(uint32(len(a.Actions)))
	c.ptr(buttons.addr())
	c.u32(uint32(defaultButton))
	c.u32(0) // radio buttons
	c.ptr(0)
	c.u32(0)
	for range 6 { // verification, expanded and collapsed texts, footer icon, footer
		c.ptr(0)
	}
	c.ptr(0) // callback
	c.ptr(0)
	c.u32(0) // width
	binary.LittleEndian.PutUint32(c.b, uint32(len(c.b)))

	var cookie uintptr
	active, _, _ := activateActCtx.Call(actx, uintptr(unsafe.Pointer(&cookie)))
	var chosen int32
	hr, _, _ := syscall.SyscallN(proc, c.addr(), uintptr(unsafe.Pointer(&chosen)), 0, 0)
	if active != 0 {
		deactivateActCtx.Call(0, cookie)
	}
	runtime.KeepAlive(labels)
	runtime.KeepAlive(buttons.b)
	runtime.KeepAlive(title)
	runtime.KeepAlive(instruction)
	runtime.KeepAlive(content)
	switch i := int(chosen) - alertButton; {
	case int32(hr) < 0:
		return w.messageBox(a, cancel, destructive)
	case i >= 0 && i < len(a.Actions):
		return i
	case chosen == idCancel:
		return cancel
	}
	return -1
}

// messageBox shows a with the button set closest to its actions: Yes,
// No and Cancel cannot carry their labels.
func (w *window) messageBox(a *core.AlertConfig, cancel int, destructive bool) int {
	var others []int
	for i := range a.Actions {
		if i != cancel {
			others = append(others, i)
		}
	}
	answers := map[uintptr]int{}
	var style uintptr
	switch {
	case len(others) == 0:
		style, answers[idOK] = mbOK, cancel
	case len(others) == 1 && cancel < 0:
		style, answers[idOK] = mbOK, others[0]
	case len(others) == 1:
		style, answers[idOK], answers[idCancel] = mbOKCancel, others[0], cancel
	case len(others) == 2 && cancel < 0:
		style, answers[idYes], answers[idNo] = mbYesNo, others[0], others[1]
	default:
		style, answers[idYes], answers[idNo], answers[idCancel] = mbYesNoCancel, others[0], others[1], cancel
		if cancel < 0 {
			answers[idCancel] = others[2]
		}
	}
	if destructive {
		style |= mbIconWarning
	}
	text := a.Title
	if a.Message != "" {
		if text != "" {
			text += "\n\n"
		}
		text += a.Message
	}
	r, _, _ := messageBoxW.Call(w.hwnd, uintptr(unsafe.Pointer(wide(text))), uintptr(unsafe.Pointer(wide(w.title))), style)
	if i, ok := answers[r]; ok {
		return i
	}
	return -1
}

// optional returns s as UTF-16, or nil when empty.
func optional(s string) *uint16 {
	if s == "" {
		return nil
	}
	return wide(s)
}

// packed lays fields out one after another, without padding.
type packed struct{ b []byte }

func (p *packed) u32(v uint32) { p.b = binary.LittleEndian.AppendUint32(p.b, v) }

func (p *packed) ptr(v uintptr) {
	if unsafe.Sizeof(v) == 8 {
		p.b = binary.LittleEndian.AppendUint64(p.b, uint64(v))
	} else {
		p.u32(uint32(v))
	}
}

func (p *packed) addr() uintptr {
	if len(p.b) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&p.b[0]))
}

//go:build windows

package windows

import (
	"bytes"
	"encoding/xml"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// System notifications as Windows' toasts through WinRT's ToastNotificationManager, and the badge as the taskbar button's overlay icon. An app outside an MSIX names itself with an AppUserModelID, its ID, which the current user's registry gives the app's name and icon, as toasts need; a press reaches the app while it runs, through the toast's Activated event, or starts the app through the COM activator. A scheduled toast is the notifier's, which shows it at its time whether the app runs or not.

var (
	combase                   = syscall.NewLazyDLL("combase.dll")
	roGetActivationFactory    = combase.NewProc("RoGetActivationFactory")
	roActivateInstance        = combase.NewProc("RoActivateInstance")
	windowsCreateString       = combase.NewProc("WindowsCreateString")
	windowsDeleteString       = combase.NewProc("WindowsDeleteString")
	windowsGetStringRawBuffer = combase.NewProc("WindowsGetStringRawBuffer")
	getCurrentPackageFullName = kernel32.NewProc("GetCurrentPackageFullName")
	advapi32                  = syscall.NewLazyDLL("advapi32.dll")
	regCreateKeyExW           = advapi32.NewProc("RegCreateKeyExW")
	regSetValueExW            = advapi32.NewProc("RegSetValueExW")

	createCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	deleteDC           = gdi32.NewProc("DeleteDC")
	selectObject       = gdi32.NewProc("SelectObject")
	getDIBits          = gdi32.NewProc("GetDIBits")
	getObjectW         = gdi32.NewProc("GetObjectW")
	createDIBSection   = gdi32.NewProc("CreateDIBSection")
	createBitmap       = gdi32.NewProc("CreateBitmap")
	createFontW        = gdi32.NewProc("CreateFontW")
	setBkMode          = gdi32.NewProc("SetBkMode")
	setTextColor       = gdi32.NewProc("SetTextColor")
	drawTextW          = user32.NewProc("DrawTextW")
	createIconIndirect = user32.NewProc("CreateIconIndirect")
	destroyIcon        = user32.NewProc("DestroyIcon")
	getIconInfo        = user32.NewProc("GetIconInfo")
	loadImageW         = user32.NewProc("LoadImageW")

	iidToastManagerStatics  = parseGUID("50ac103f-d235-4598-bbef-98fe4d1a3ad4")
	iidToastManagerStatics2 = parseGUID("7ab93c52-0e48-4750-ba9d-1a4113981847")
	iidToastFactory         = parseGUID("04124b20-82c6-4229-b109-fd9ed4662b53")
	iidToastNotification2   = parseGUID("9dfb9fd1-143a-490e-90bf-b9fba7132de7")
	iidXmlDocumentIO        = parseGUID("6cd0e74e-ee65-4489-9ebf-ca43e87ba637")
	iidXmlDocument          = parseGUID("f7f3a506-1e87-42d6-bcfb-b8c809fa5494")
	// IScheduledToastNotificationFactory and IScheduledToastNotification2.
	iidScheduledToasts = parseGUID("e7bed191-0bb9-4189-8394-31761b476fd7")
	iidScheduledToast2 = parseGUID("a66ea09c-31b4-43b0-b5dd-7a40e85363b1")
	// TypedEventHandler<ToastNotification, IInspectable>, Activated's.
	iidToastActivated = parseGUID("ab54de2d-97d9-5528-b6ad-105afe156530")
	clsidTaskbarList  = parseGUID("56fdf344-fd6d-11d0-958a-006097c9a090")
	iidTaskbarList3   = parseGUID("ea1afb91-9e28-4b86-90e9-9e9f8a5eefaf")
)

// The methods called, by their vtable slots: IInspectable's six come
// before an interface's own.
const (
	toastStaticsCreateNotifier       = 6
	toastStaticsCreateNotifierWithID = 7
	toastStatics2History             = 6
	toastNotifierShow                = 6
	toastNotifierSetting             = 8
	toastNotifierAddToSchedule       = 9
	toastNotifierRemoveFromSchedule  = 10
	toastNotifierGetScheduled        = 11
	scheduledToastFactoryCreate      = 6
	scheduledToast2PutTag            = 6
	scheduledToast2GetTag            = 7
	scheduledToast2GetGroup          = 9
	scheduledToast2PutGroup          = 8
	vectorViewGetAt                  = 6
	vectorViewSize                   = 7
	toastFactoryCreate               = 6
	toastAddActivated                = 11
	toast2PutTag                     = 6
	toast2PutGroup                   = 8
	toastHistoryRemoveGroup          = 6
	toastHistoryRemoveGroupID        = 7
	toastHistoryRemoveGroupedTagID   = 8
	toastHistoryRemoveGroupedTag     = 9
	xmlDocumentIOLoadXML             = 6
	taskbarHrInit                    = 3
	taskbarSetProgressValue          = 9
	taskbarSetProgressState          = 10
	taskbarSetOverlayIcon            = 18
)

// comApartment makes the UI thread a single-threaded apartment, once, as
// drops and WebView2 do.
var comApartment = sync.OnceFunc(func() { oleInitialize.Call(0) })

// toasts is the toast notifier and the toasts showing, by the app's ID,
// which the main thread keeps.
var toasts struct {
	once     sync.Once
	notifier *comObject
	aumid    string
	packaged bool
	shown    map[string]*comObject
}

// hstring makes a WinRT string of s, which deleteHString frees.
func hstring(s string) uintptr {
	u := utf16.Encode([]rune(s))
	var h uintptr
	if len(u) == 0 {
		return 0
	}
	windowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)), uintptr(unsafe.Pointer(&h)))
	return h
}

func deleteHString(h uintptr) {
	if h != 0 {
		windowsDeleteString.Call(h)
	}
}

// activationFactory returns WinRT class name's factory as interface iid,
// nil where Windows has none.
func activationFactory(name string, iid *guid) *comObject {
	h := hstring(name)
	defer deleteHString(h)
	var f *comObject
	if hr, _, _ := roGetActivationFactory.Call(h, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&f))); failed(hr) {
		return nil
	}
	return f
}

// toastNotifier returns the app's toast notifier, nil where it has none:
// an MSIX's own, else one named by the app's ID, which it registers.
func (w *window) toastNotifier() *comObject {
	toasts.once.Do(func() {
		toasts.shown = map[string]*comObject{}
		var n uint32
		// APPMODEL_ERROR_NO_PACKAGE: the app is no package's.
		r, _, _ := getCurrentPackageFullName.Call(uintptr(unsafe.Pointer(&n)), 0)
		toasts.packaged = r != 15700
		statics := activationFactory("Windows.UI.Notifications.ToastNotificationManager", &iidToastManagerStatics)
		if statics == nil {
			return
		}
		defer statics.release()
		var notifier *comObject
		if toasts.packaged {
			if failed(statics.call(toastStaticsCreateNotifier, uintptr(unsafe.Pointer(&notifier)))) {
				return
			}
		} else {
			toasts.aumid = w.opts.ID
			if toasts.aumid == "" {
				toasts.aumid = "dev.nsgo.app"
			}
			registerAUMID(toasts.aumid, w.opts.Title)
			id := hstring(toasts.aumid)
			hr := statics.call(toastStaticsCreateNotifierWithID, id, uintptr(unsafe.Pointer(&notifier)))
			deleteHString(id)
			if failed(hr) {
				return
			}
		}
		toasts.notifier = notifier
	})
	return toasts.notifier
}

// registerAUMID gives the current user's AppUserModelID id the app's
// name and icon, which its toasts show.
func registerAUMID(id, name string) {
	var key syscall.Handle
	path := `Software\Classes\AppUserModelId\` + id
	if r, _, _ := regCreateKeyExW.Call(uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(wide(path))), 0, 0, 0,
		syscall.KEY_WRITE, 0, uintptr(unsafe.Pointer(&key)), 0); r != 0 {
		return
	}
	defer syscall.RegCloseKey(key)
	set := func(name, value string) {
		v := syscall.StringToUTF16(value)
		regSetValueExW.Call(uintptr(key), uintptr(unsafe.Pointer(wide(name))), 0, syscall.REG_SZ,
			uintptr(unsafe.Pointer(&v[0])), uintptr(len(v)*2))
	}
	if name == "" {
		name = filepath.Base(os.Args[0])
	}
	set("DisplayName", name)
	if icon := appIconPNG(id); icon != "" {
		set("IconUri", icon)
	}
}

// appIconPNG writes the executable's icon as a PNG in the app's cache
// directory, which toasts read, and returns its path, "" without one.
func appIconPNG(id string) string {
	_, dir, _ := engine.AppDirs(id)
	if dir == "" {
		return ""
	}
	instance, _, _ := getModuleHandleW.Call(0)
	// LR_DEFAULTCOLOR at 256 by 256: the largest icon nsgo links.
	const imageIcon, appIcon = 1, 1
	icon, _, _ := loadImageW.Call(instance, appIcon, imageIcon, 256, 256, 0)
	if icon == 0 {
		return ""
	}
	defer destroyIcon.Call(icon)
	img := iconImage(icon)
	if img == nil {
		return ""
	}
	var buf bytes.Buffer
	if png.Encode(&buf, img) != nil {
		return ""
	}
	path := filepath.Join(dir, "notification-icon.png")
	if os.MkdirAll(dir, 0o755) != nil || os.WriteFile(path, buf.Bytes(), 0o644) != nil {
		return ""
	}
	return path
}

// bitmapInfo is BITMAPINFO with no color table.
type bitmapInfo struct {
	Size                         uint32
	Width, Height                int32
	Planes, BitCount             uint16
	Compression, SizeImage       uint32
	XPelsPerMeter, YPelsPerMeter int32
	ClrUsed, ClrImportant        uint32
	Colors                       [1]uint32
}

// iconInfo is ICONINFO.
type iconInfo struct {
	Icon        int32
	HotX, HotY  uint32
	Mask, Color uintptr
}

// gdiBitmap is BITMAP.
type gdiBitmap struct {
	Type, Width, Height, WidthBytes int32
	Planes, BitsPixel               uint16
	Bits                            uintptr
}

// iconImage reads an icon's color bitmap, nil for one without alpha.
func iconImage(icon uintptr) *image.NRGBA {
	var info iconInfo
	if r, _, _ := getIconInfo.Call(icon, uintptr(unsafe.Pointer(&info))); r == 0 {
		return nil
	}
	defer deleteObject.Call(info.Mask)
	defer deleteObject.Call(info.Color)
	var bm gdiBitmap
	if info.Color == 0 || func() bool {
		r, _, _ := getObjectW.Call(info.Color, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
		return r == 0
	}() {
		return nil
	}
	w, h := int(bm.Width), int(bm.Height)
	pix := make([]byte, w*h*4)
	bi := bitmapInfo{Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	bi.Size = uint32(unsafe.Sizeof(bi)) - 4
	dc, _, _ := createCompatibleDC.Call(0)
	defer deleteDC.Call(dc)
	if n, _, _ := getDIBits.Call(dc, info.Color, 0, uintptr(h), uintptr(unsafe.Pointer(&pix[0])),
		uintptr(unsafe.Pointer(&bi)), 0); n == 0 {
		return nil
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	alpha := false
	for i := 0; i < len(pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = pix[i+2], pix[i+1], pix[i], pix[i+3]
		alpha = alpha || pix[i+3] != 0
	}
	if !alpha {
		return nil
	}
	return img
}

// notificationSetting reads the toast notifier's setting on the main thread's STA.
func (w *window) notificationSetting() int {
	comApartment()
	if n := w.toastNotifier(); n != nil {
		// NotificationSetting: Enabled, then the ways it is turned off.
		var setting int32
		if !failed(n.call(toastNotifierSetting, uintptr(unsafe.Pointer(&setting)))) {
			if setting == 0 {
				return driver.NotificationsGranted
			}
			return driver.NotificationsDenied
		}
	}
	return driver.NotificationsUnavailable
}

// notificationPermissionStatus is notificationSetting mapped to a driver.Permission* status.
func (w *window) notificationPermissionStatus() int {
	switch w.notificationSetting() {
	case driver.NotificationsGranted:
		return driver.PermissionGranted
	case driver.NotificationsDenied:
		return driver.PermissionDenied
	}
	return driver.PermissionUnavailable
}

// notify carries out a notification's item on the main thread, in the
// single-threaded apartment WinRT and the shell's objects need there.
func (w *window) notify(it *driver.MountItem) {
	comApartment()
	e, id := w.engine, it.Count
	switch it.Kind {
	case driver.ItemNotificationPermission:
		e.Dispatch(driver.Event{Kind: driver.EventNotification, Count: id, Index: w.notificationSetting()})
	case driver.ItemShowNotification:
		e.Dispatch(driver.Event{Kind: driver.EventNotification, Count: id, Finished: w.showToast(*it.Notification)})
	case driver.ItemCloseNotification:
		w.closeToast(it.Value)
	case driver.ItemBadge:
		w.setOverlayBadge(it.Count)
	case driver.ItemTaskProgress:
		w.setTaskProgress(*it.Progress)
	}
}

const (
	tbpfNoProgress    = 0
	tbpfIndeterminate = 1
	tbpfNormal        = 2
	tbpfError         = 4
	tbpfPaused        = 8
)

// taskbarButtonCreated is the message the taskbar sends a window once its button is in place, and again after Explorer starts again. ITaskbarList3 must not be called for a window before it, so the window keeps its overlay badge and progress, shows them once the message comes and shows them again with each one after it.
var taskbarButtonCreated = func() uint32 {
	m, _, _ := registerWindowMessageW.Call(uintptr(unsafe.Pointer(wide("TaskbarButtonCreated"))))
	return uint32(m)
}()

var changeWindowMessageFilterEx = user32.NewProc("ChangeWindowMessageFilterEx")

// hearTaskbarButton lets TaskbarButtonCreated reach hwnd from Explorer while the app runs elevated, where UIPI blocks it otherwise.
func hearTaskbarButton(hwnd uintptr) {
	const msgfltAllow = 1
	if taskbarButtonCreated != 0 {
		changeWindowMessageFilterEx.Call(hwnd, uintptr(taskbarButtonCreated), msgfltAllow, 0)
	}
}

// taskbarButtonReady shows what the window keeps for its taskbar button, which is now in place (taskbarButtonCreated).
func (w *window) taskbarButtonReady() {
	w.buttonReady = true
	if w.badge != 0 {
		w.showOverlayBadge()
	}
	if w.shownProgress().State != driver.TaskProgressNone {
		w.showTaskProgress()
	}
}

// ownsTaskbarButton reports whether the window has a taskbar button of its own. The hidden root window of an app that opens its own windows has none, as it only ever shows as an alert's window; nor do panels and tray icons' views, which are tool windows, or a host's child window.
func (w *window) ownsTaskbarButton() bool {
	return !w.embedded && w.kind == driver.SurfaceWindow && !(w.primary && w.opts.Hidden)
}

// rootWindow is the backend's root window of the app w is a surface of.
func (w *window) rootWindow() *window {
	if w.hostWindow != nil {
		return w.hostWindow
	}
	return w
}

// setTaskProgress keeps progress for the window's taskbar button and shows it there once the button is in place. A window without a button of its own sets the app's progress instead, which every window of the app shows while it has none of its own, as the Dock tile and Linux launchers show one progress for the app; as several surfaces set it, a call older than the one it holds is a surface's that committed after a later call's, and changes nothing.
func (w *window) setTaskProgress(progress driver.TaskProgress) {
	if w.ownsTaskbarButton() {
		w.progress = progress
		w.showTaskProgress()
		return
	}
	root := w.rootWindow()
	if progress.Call < root.appProgress.Call {
		return
	}
	root.appProgress = progress
	for _, o := range windows {
		if o.rootWindow() == root && o.ownsTaskbarButton() && o.progress.State == driver.TaskProgressNone {
			o.showTaskProgress()
		}
	}
}

// shownProgress is the progress the window's taskbar button shows: its own, or the app's while it has none.
func (w *window) shownProgress() driver.TaskProgress {
	if w.progress.State != driver.TaskProgressNone {
		return w.progress
	}
	return w.rootWindow().appProgress
}

// showTaskProgress sets the taskbar button's progress state and value.
func (w *window) showTaskProgress() {
	if !w.buttonReady || !w.ownsTaskbarButton() {
		return
	}
	list := taskbar()
	if list == nil {
		return
	}
	state, completed := taskbarProgress(w.shownProgress())
	list.call(taskbarSetProgressState, w.hwnd, state)
	if completed >= 0 {
		// Both counts are ULONGLONGs, two words each on 386.
		args := append([]uintptr{w.hwnd}, u64Args(uint64(completed))...)
		list.call(taskbarSetProgressValue, append(args, u64Args(taskbarProgressTotal)...)...)
	}
}

// taskbarProgressTotal is the whole of a task as SetProgressValue is given it.
const taskbarProgressTotal = 1000

// taskbarProgress is progress as ITaskbarList3 takes it: its TBPF_ state, and the completed part of taskbarProgressTotal for the states that show a fraction, -1 for those that show none.
func taskbarProgress(p driver.TaskProgress) (state uintptr, completed int) {
	switch p.State {
	case driver.TaskProgressNormal:
		state = tbpfNormal
	case driver.TaskProgressPaused:
		state = tbpfPaused
	case driver.TaskProgressError:
		state = tbpfError
	case driver.TaskProgressIndeterminate:
		return tbpfIndeterminate, -1
	default:
		return tbpfNoProgress, -1
	}
	return state, int(math.Round(min(max(p.Value, 0), 1) * taskbarProgressTotal))
}

// toastXML is a toast's content: its launch arguments, its title, its body and, when silent, no sound.
func toastXML(n driver.Notification, launch string) string {
	esc := func(s string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	s := `<toast launch="` + esc(launch) + `"><visual><binding template="ToastGeneric"><text>` + esc(n.Title) + `</text>`
	if n.Body != "" {
		s += `<text>` + esc(n.Body) + `</text>`
	}
	s += `</binding></visual>`
	if n.Silent {
		s += `<audio silent="true"/>`
	}
	return s + `</toast>`
}

// toastDocument is n's toast content as an XmlDocument launching with launch, nil where Windows cannot load it.
func toastDocument(n driver.Notification, launch string) *comObject {
	var inspectable *comObject
	class := hstring("Windows.Data.Xml.Dom.XmlDocument")
	hr, _, _ := roActivateInstance.Call(class, uintptr(unsafe.Pointer(&inspectable)))
	deleteHString(class)
	if failed(hr) {
		return nil
	}
	defer inspectable.release()
	io := inspectable.query(&iidXmlDocumentIO)
	if io == nil {
		return nil
	}
	content := hstring(toastXML(n, launch))
	hr = io.call(xmlDocumentIOLoadXML, content)
	deleteHString(content)
	io.release()
	if failed(hr) {
		return nil
	}
	return inspectable.query(&iidXmlDocument)
}

// showToast shows n as a toast tagged with its ID, or schedules it for its At, replacing the one of that tag, shown or scheduled, and reports whether the notifier took it.
func (w *window) showToast(n driver.Notification) bool {
	notifier := w.toastNotifier()
	if notifier == nil {
		return false
	}
	unscheduleToast(notifier, n.ID)
	if !n.At.IsZero() {
		return scheduleToast(notifier, n)
	}
	launch := toastLaunch(n.ID)
	doc := toastDocument(n, launch)
	factory := activationFactory("Windows.UI.Notifications.ToastNotification", &iidToastFactory)
	if doc == nil || factory == nil {
		doc.release()
		factory.release()
		return false
	}
	var toast *comObject
	hr := factory.call(toastFactoryCreate, doc.ptr(), uintptr(unsafe.Pointer(&toast)))
	factory.release()
	doc.release()
	if failed(hr) {
		return false
	}
	if !setToastIdentity(toast, &iidToastNotification2, n.ID) {
		toast.release()
		return false
	}
	appID, e := n.ID, w.engine
	_, press := parseToastActivationArgs(launch)
	pressed := newHandler(&iidToastActivated, func(uintptr, unsafe.Pointer) {
		if toastPresses.first(press, time.Now()) {
			e.Dispatch(driver.Event{Kind: driver.EventNotificationPress, Text: appID})
		}
		w.post(func() {
			if old := toasts.shown[appID]; old != nil {
				old.release()
				delete(toasts.shown, appID)
			}
			showWindow.Call(w.hwnd, swRestore)
			setForegroundWindow.Call(w.hwnd)
		})
	})
	pressed.agile = true
	var token eventToken
	toast.call(toastAddActivated, pressed.ptr(), uintptr(unsafe.Pointer(&token)))
	pressed.release()
	if failed(notifier.call(toastNotifierShow, toast.ptr())) {
		toast.release()
		return false
	}
	removeToastHistory(n.ID, true)
	if old := toasts.shown[n.ID]; old != nil {
		old.release()
	}
	toasts.shown[n.ID] = toast
	return true
}

// scheduleToast has the notifier show n as a toast tagged with its ID at its At, with the app's process ended too, and reports whether it took it. Windows has no daily or weekly schedule, only a snooze's, so a repeated one is refused. A press of it reaches the app through the toast activator alone, as no toast the app holds hears its Activated.
func scheduleToast(notifier *comObject, n driver.Notification) bool {
	if n.Repeat != driver.NotificationOnce {
		return false
	}
	doc := toastDocument(n, toastLaunch(n.ID))
	factory := activationFactory("Windows.UI.Notifications.ScheduledToastNotification", &iidScheduledToasts)
	if doc == nil || factory == nil {
		doc.release()
		factory.release()
		return false
	}
	// A DateTime by value: 100-nanosecond intervals since 1601 in UTC; the notifier refuses one that passed.
	at := n.At
	if soon := time.Now().Add(time.Second); at.Before(soon) {
		at = soon
	}
	ticks := uint64(at.UnixNano()/100 + 116444736000000000)
	var toast *comObject
	hr := factory.call(scheduledToastFactoryCreate, append(append([]uintptr{doc.ptr()}, u64Args(ticks)...), uintptr(unsafe.Pointer(&toast)))...)
	factory.release()
	doc.release()
	if failed(hr) {
		return false
	}
	defer toast.release()
	if !setToastIdentity(toast, &iidScheduledToast2, n.ID) {
		return false
	}
	if failed(notifier.call(toastNotifierAddToSchedule, toast.ptr())) {
		return false
	}
	removeToastHistory(n.ID, true)
	return true
}

// Both toast identity interfaces have Tag and Group setters in these slots.
func setToastIdentity(toast *comObject, iid *guid, id string) bool {
	identity := toast.query(iid)
	if identity == nil {
		return false
	}
	defer identity.release()
	tagText, groupText := toastIdentity(id)
	tag, group := hstring(tagText), hstring(groupText)
	defer deleteHString(tag)
	defer deleteHString(group)
	return !failed(identity.call(toast2PutTag, tag)) && !failed(identity.call(toast2PutGroup, group))
}

// unscheduleToast takes the notifier's scheduled toasts tagged id off its schedule, and every one it holds, all the app's, when id is empty.
func unscheduleToast(notifier *comObject, id string) {
	var list *comObject
	if failed(notifier.call(toastNotifierGetScheduled, uintptr(unsafe.Pointer(&list)))) || list == nil {
		return
	}
	defer list.release()
	var size uint32
	if failed(list.call(vectorViewSize, uintptr(unsafe.Pointer(&size)))) {
		return
	}
	for i := range size {
		var toast *comObject
		if failed(list.call(vectorViewGetAt, uintptr(i), uintptr(unsafe.Pointer(&toast)))) || toast == nil {
			continue
		}
		if t2 := toast.query(&iidScheduledToast2); t2 != nil {
			var tag, group uintptr
			tagHR := t2.call(scheduledToast2GetTag, uintptr(unsafe.Pointer(&tag)))
			groupHR := t2.call(scheduledToast2GetGroup, uintptr(unsafe.Pointer(&group)))
			if !failed(tagHR) && !failed(groupHR) && toastMatches(id, hstringText(tag), hstringText(group)) {
				notifier.call(toastNotifierRemoveFromSchedule, toast.ptr())
			}
			deleteHString(tag)
			deleteHString(group)
			t2.release()
		}
		toast.release()
	}
}

// hstringText is the text of WinRT string h.
func hstringText(h uintptr) string {
	if h == 0 {
		return ""
	}
	var n uint32
	p, _, _ := windowsGetStringRawBuffer.Call(h, uintptr(unsafe.Pointer(&n)))
	if p == 0 || n == 0 {
		return ""
	}
	// The buffer is the string's own, which h keeps alive.
	return string(utf16.Decode(unsafe.Slice(*(**uint16)(unsafe.Pointer(&p)), n)))
}

// closeToast removes the toast tagged id from the screen, the action center and the notifier's schedule, and with an empty id every toast in the app's group, those of earlier runs included, which leaves other apps' toasts alone.
func (w *window) closeToast(id string) {
	if id == "" {
		for tag, old := range toasts.shown {
			old.release()
			delete(toasts.shown, tag)
		}
	} else if old := toasts.shown[id]; old != nil {
		old.release()
		delete(toasts.shown, id)
	}
	notifier := w.toastNotifier()
	if notifier == nil {
		return
	}
	unscheduleToast(notifier, id)
	removeToastHistory(id, false)
}

// removeToastHistory also retires raw long-ID tags written by older versions.
func removeToastHistory(id string, legacyOnly bool) {
	if _, group := toastIdentity(id); legacyOnly && group == toastGroup {
		return
	}
	statics := activationFactory("Windows.UI.Notifications.ToastNotificationManager", &iidToastManagerStatics2)
	if statics == nil {
		return
	}
	defer statics.release()
	var history *comObject
	if failed(statics.call(toastStatics2History, uintptr(unsafe.Pointer(&history)))) {
		return
	}
	defer history.release()
	remove := func(tagText, groupText string) {
		tag, group := hstring(tagText), hstring(groupText)
		defer deleteHString(tag)
		defer deleteHString(group)
		if toasts.packaged {
			if id == "" {
				history.call(toastHistoryRemoveGroup, group)
			} else {
				history.call(toastHistoryRemoveGroupedTag, tag, group)
			}
			return
		}
		aumid := hstring(toasts.aumid)
		defer deleteHString(aumid)
		if id == "" {
			history.call(toastHistoryRemoveGroupID, group, aumid)
		} else {
			history.call(toastHistoryRemoveGroupedTagID, tag, group, aumid)
		}
	}
	if legacyOnly {
		if _, group := toastIdentity(id); group != toastGroup {
			remove(id, toastGroup)
		}
		return
	}
	if id == "" {
		remove("", toastGroup)
		remove("", toastLongGroup)
	} else {
		tag, group := toastIdentity(id)
		remove(tag, group)
		if group != toastGroup {
			remove(id, toastGroup)
		}
	}
}

// taskbarList is the shell's taskbar list once made, and taskbarFailure the HRESULT the last attempt to make it failed with; the main thread keeps both.
var (
	taskbarList    *comObject
	taskbarFailure uintptr
)

// taskbar returns the shell's taskbar list, made on the main thread, or nil while it cannot be made. A failure is not kept, as its cause may pass, and is logged once for each HRESULT.
func taskbar() *comObject {
	if taskbarList != nil {
		return taskbarList
	}
	comApartment()
	var list *comObject
	hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidTaskbarList)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidTaskbarList3)), uintptr(unsafe.Pointer(&list)))
	if !failed(hr) {
		if hr = list.call(taskbarHrInit); !failed(hr) {
			taskbarList = list
			return list
		}
		list.release()
	}
	if hr != taskbarFailure {
		taskbarFailure = hr
		log.Print(hresultError("nsgo: the shell's taskbar list cannot be made for the app's taskbar buttons", hr))
	}
	return nil
}

// setOverlayBadge keeps count for the window's taskbar button and shows it there once the button is in place.
func (w *window) setOverlayBadge(count uint32) {
	w.badge = count
	w.showOverlayBadge()
}

// showOverlayBadge shows the window's badge count on its taskbar button, a red disc drawn at the small icon's size, or clears it for 0.
func (w *window) showOverlayBadge() {
	if !w.buttonReady {
		return
	}
	count := w.badge
	list := taskbar()
	if list == nil {
		return
	}
	if count == 0 {
		list.call(taskbarSetOverlayIcon, w.hwnd, 0, 0)
		return
	}
	const smCxSmIcon = 49
	size, _, _ := getSystemMetrics.Call(smCxSmIcon)
	label := strconv.FormatUint(uint64(count), 10)
	if count > 99 {
		label = "99+"
	}
	icon := badgeIcon(int(max(size, 16)), label)
	if icon == 0 {
		return
	}
	desc := wide(label)
	list.call(taskbarSetOverlayIcon, w.hwnd, icon, uintptr(unsafe.Pointer(desc)))
	// The taskbar keeps a copy.
	destroyIcon.Call(icon)
}

// badgeIcon draws label in white on a red disc size pixels across, as an
// icon.
func badgeIcon(size int, label string) uintptr {
	bi := bitmapInfo{Width: int32(size), Height: -int32(size), Planes: 1, BitCount: 32}
	bi.Size = uint32(unsafe.Sizeof(bi)) - 4
	dc, _, _ := createCompatibleDC.Call(0)
	defer deleteDC.Call(dc)
	var bits unsafe.Pointer
	dib, _, _ := createDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if dib == 0 {
		return 0
	}
	defer deleteObject.Call(dib)
	pix := unsafe.Slice((*byte)(bits), size*size*4)
	// The disc's coverage of each pixel, four samples a side.
	cover := make([]byte, size*size)
	r := float64(size) / 2
	for y := range size {
		for x := range size {
			in := 0
			for sy := range 4 {
				for sx := range 4 {
					dx, dy := float64(x)+(float64(sx)+0.5)/4-r, float64(y)+(float64(sy)+0.5)/4-r
					if math.Hypot(dx, dy) <= r {
						in++
					}
				}
			}
			cover[y*size+x] = byte(in * 255 / 16)
			i := (y*size + x) * 4
			// #e81123, Windows' badge red, in BGR.
			pix[i], pix[i+1], pix[i+2] = 0x23, 0x11, 0xe8
		}
	}
	old, _, _ := selectObject.Call(dc, dib)
	const fwBold, transparent, antialiased = 700, 1, 4
	height := -size * 3 / 4
	if len(label) > 1 {
		height = -size * 3 / 5
	}
	font, _, _ := createFontW.Call(uintptr(height), 0, 0, 0, fwBold, 0, 0, 0, 1, 0, 0, antialiased, 0,
		uintptr(unsafe.Pointer(wide("Segoe UI"))))
	oldFont, _, _ := selectObject.Call(dc, font)
	setBkMode.Call(dc, transparent)
	setTextColor.Call(dc, 0xffffff)
	text := syscall.StringToUTF16(label)
	box := rect{Right: int32(size), Bottom: int32(size)}
	const dtCenter, dtVCenter, dtSingleLine = 0x1, 0x4, 0x20
	drawTextW.Call(dc, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)-1), uintptr(unsafe.Pointer(&box)),
		dtCenter|dtVCenter|dtSingleLine)
	selectObject.Call(dc, oldFont)
	deleteObject.Call(font)
	selectObject.Call(dc, old)
	// GDI leaves its text's alpha at 0: the disc's coverage is every
	// pixel's, premultiplied.
	for i, a := range cover {
		p := pix[i*4 : i*4+4]
		p[0], p[1], p[2], p[3] = byte(int(p[0])*int(a)/255), byte(int(p[1])*int(a)/255), byte(int(p[2])*int(a)/255), a
	}
	mask, _, _ := createBitmap.Call(uintptr(size), uintptr(size), 1, 1, 0)
	defer deleteObject.Call(mask)
	info := iconInfo{Icon: 1, Mask: mask, Color: dib}
	icon, _, _ := createIconIndirect.Call(uintptr(unsafe.Pointer(&info)))
	return icon
}

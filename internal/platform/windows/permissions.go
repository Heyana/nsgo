//go:build windows

package windows

import (
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Windows keeps the microphone and the camera from apps in the capability access manager's consent store: a switch for the device (HKLM), one for the user, and one for the app, under its package family name for a packaged app and under NonPackaged for the others. It keeps nothing else from desktop apps, has no prompt an app can show without opening the device, and opens Settings at the microphone's or the camera's page.

const consentStore = `Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\`

var getCurrentPackageFamilyName = kernel32.NewProc("GetCurrentPackageFamilyName")

// windowsCapabilities are the consent store's capabilities of the permissions Windows keeps, and the pages of Settings that switch them.
var windowsCapabilities = map[string]struct{ capability, page string }{
	driver.PermissionMicrophone: {"microphone", "ms-settings:privacy-microphone"},
	driver.PermissionCamera:     {"webcam", "ms-settings:privacy-webcam"},
}

// packageFamilyName is the app's package family name, "" outside a package.
func packageFamilyName() string {
	if getCurrentPackageFamilyName.Find() != nil {
		return ""
	}
	n := uint32(0)
	getCurrentPackageFamilyName.Call(uintptr(unsafe.Pointer(&n)), 0)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n)
	if r, _, _ := getCurrentPackageFamilyName.Call(uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0]))); r != 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// consentStatus is the status of a capability from its switches' values: any off denies it; one not stored is on, but for a packaged app's own, which Windows asks as the app first opens the device.
func consentStatus(machine, user, app string, packaged bool) int {
	for _, v := range []string{machine, user, app} {
		if strings.EqualFold(v, "Deny") {
			return driver.PermissionDenied
		}
	}
	if packaged && app == "" {
		return driver.PermissionNotAsked
	}
	return driver.PermissionGranted
}

// permissionStatus is whether the app has the permission name, and whether the device's switch, which only an administrator changes, keeps it.
func permissionStatus(name string) (status int, byDevice bool) {
	c, ok := windowsCapabilities[name]
	if !ok {
		return driver.PermissionNotGated, false
	}
	path := consentStore + c.capability
	machine := regString(syscall.HKEY_LOCAL_MACHINE, 0, path, "Value")
	user := regString(syscall.HKEY_CURRENT_USER, 0, path, "Value")
	family := packageFamilyName()
	app := family
	if family == "" {
		app = "NonPackaged"
	}
	status = consentStatus(machine, user, regString(syscall.HKEY_CURRENT_USER, 0, path+`\`+app, "Value"), family != "")
	return status, strings.EqualFold(machine, "Deny")
}

const notificationsSettingsPage = "ms-settings:notifications"

// permissionPage is the Settings URI that switches the permission name, "" for none.
func permissionPage(name string) string {
	if name == driver.PermissionNotifications {
		return notificationsSettingsPage
	}
	if c, ok := windowsCapabilities[name]; ok {
		return c.page
	}
	return ""
}

// permission carries out an ItemPermission numbered id.
func (w *window) permission(id uint32, r driver.PermissionRequest) {
	answer := func(status int) {
		w.engine.Dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Index: status})
	}
	switch r.Command {
	case driver.PermissionCheck, driver.PermissionAsk:
		if r.Name == driver.PermissionNotifications {
			answer(w.notificationPermissionStatus())
			return
		}
		status, _ := permissionStatus(r.Name)
		answer(status)
	case driver.PermissionOpenSettings:
		page := permissionPage(r.Name)
		w.engine.Dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Finished: page != "" && w.shellOpenURL(page)})
	case driver.PermissionGuide:
		w.guidePermission(id, r.Name)
	case driver.PermissionStopGuide:
		guideMu.Lock()
		g := currentGuide
		guideMu.Unlock()
		if g != nil && g.w == w && g.id == id {
			g.end()
		}
	}
}

// The guide opens the permission's page of Settings and waits for the app to have it, for the user to close Settings, or for its stop. One runs at a time.
const (
	guidePoll         = 250 * time.Millisecond
	guideSettingsWait = 10 * time.Second
	settingsProcess   = "SystemSettings.exe"
)

type permissionGuide struct {
	w        *window
	id       uint32
	name     string
	stop     chan struct{}
	stopOnce sync.Once
	// quiet says it ends unanswered, as its window is gone.
	quiet bool
}

var (
	guideMu      sync.Mutex
	currentGuide *permissionGuide
)

func (g *permissionGuide) end() { g.stopOnce.Do(func() { close(g.stop) }) }

// guidePermission starts a guide numbered id of the permission name, answering at once what Settings cannot change: a permission it has no page of, one the app has, and one the device's switch keeps.
func (w *window) guidePermission(id uint32, name string) {
	var status int
	var byDevice bool
	if name == driver.PermissionNotifications {
		status = w.notificationPermissionStatus()
	} else {
		status, byDevice = permissionStatus(name)
	}
	page := permissionPage(name)
	if page == "" || status == driver.PermissionGranted || status == driver.PermissionUnavailable || byDevice || !w.shellOpenURL(page) {
		w.engine.Dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Index: status})
		return
	}
	g := &permissionGuide{w: w, id: id, name: name, stop: make(chan struct{})}
	guideMu.Lock()
	old := currentGuide
	currentGuide = g
	guideMu.Unlock()
	if old != nil {
		old.end()
	}
	go g.track()
}

// endWindowGuide ends window w's guide unanswered, as w goes.
func endWindowGuide(w *window) {
	guideMu.Lock()
	g := currentGuide
	if g != nil && g.w == w {
		g.quiet = true
	}
	guideMu.Unlock()
	if g != nil && g.w == w {
		g.end()
	}
}

// track waits for the guide's end, then answers it once.
func (g *permissionGuide) track() {
	ticker := time.NewTicker(guidePoll)
	defer ticker.Stop()
	started, seen := time.Now(), false
	for {
		if g.name != driver.PermissionNotifications {
			if status, _ := permissionStatus(g.name); status == driver.PermissionGranted {
				break
			}
		}
		running := processRunning(settingsProcess)
		if running {
			seen = true
		} else if seen || time.Since(started) > guideSettingsWait {
			break
		}
		select {
		case <-g.stop:
		case <-ticker.C:
			continue
		}
		break
	}
	g.end()
	guideMu.Lock()
	if currentGuide == g {
		currentGuide = nil
	}
	quiet := g.quiet
	guideMu.Unlock()
	if !quiet {
		if g.name == driver.PermissionNotifications {
			g.w.post(func() {
				g.w.engine.Dispatch(driver.Event{Kind: driver.EventPermission, Count: g.id, Index: g.w.notificationPermissionStatus()})
			})
			return
		}
		status, _ := permissionStatus(g.name)
		g.w.engine.Dispatch(driver.Event{Kind: driver.EventPermission, Count: g.id, Index: status})
	}
}

// processRunning reports whether a process of the executable name runs.
func processRunning(name string) bool {
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(snapshot)
	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = syscall.Process32First(snapshot, &entry); err == nil; err = syscall.Process32Next(snapshot, &entry) {
		if strings.EqualFold(syscall.UTF16ToString(entry.ExeFile[:]), name) {
			return true
		}
	}
	return false
}

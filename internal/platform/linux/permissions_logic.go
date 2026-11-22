package linux

import (
	"strings"
	"sync"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	portalDesktopName    = "org.freedesktop.portal.Desktop"
	portalDesktopPath    = "/org/freedesktop/portal/desktop"
	portalCameraIface    = "org.freedesktop.portal.Camera"
	portalDeviceIface    = "org.freedesktop.portal.Device"
	portalRequestIface   = "org.freedesktop.portal.Request"
	permissionStoreName  = "org.freedesktop.impl.portal.PermissionStore"
	permissionStorePath  = "/org/freedesktop/impl/portal/PermissionStore"
	permissionStoreIface = "org.freedesktop.impl.portal.PermissionStore"

	permissionTableDevices = "devices"
	permissionIDCamera     = "camera"
	permissionIDMicrophone = "microphone"
)

// linuxSandboxKind identifies the Linux application sandbox the process runs in.
type linuxSandboxKind uint8

const (
	linuxSandboxNone linuxSandboxKind = iota
	linuxSandboxFlatpak
	linuxSandboxSnap
)

// flatpakInfo holds the fields parsed from /.flatpak-info.
type flatpakInfo struct {
	appID       string
	sockets     []string
	devices     []string
	filesystems []string
}

// linuxSandbox describes the sandbox identity used when querying the XDG
// PermissionStore and desktop portals.
type linuxSandbox struct {
	kind    linuxSandboxKind
	appID   string
	flatpak flatpakInfo
}

// parseFlatpakInfo parses the key-file format of /.flatpak-info.
func parseFlatpakInfo(data []byte) flatpakInfo {
	var info flatpakInfo
	section := ""
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch section {
		case "Application":
			if key == "name" {
				info.appID = val
			}
		case "Context":
			switch key {
			case "sockets":
				info.sockets = splitFlatpakList(val)
			case "devices":
				info.devices = splitFlatpakList(val)
			case "filesystems":
				info.filesystems = splitFlatpakList(val)
			}
		}
	}
	return info
}

func splitFlatpakList(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ";") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// detectLinuxSandbox resolves the sandbox kind and portal app ID from
// /.flatpak-info, Snap environment variables, and the app's fallback ID.
func detectLinuxSandbox(flatpakData []byte, hasFlatpak bool, snap, snapName, fallbackAppID string) linuxSandbox {
	if hasFlatpak {
		info := parseFlatpakInfo(flatpakData)
		appID := info.appID
		if appID == "" {
			appID = fallbackAppID
		}
		return linuxSandbox{
			kind:    linuxSandboxFlatpak,
			appID:   appID,
			flatpak: info,
		}
	}
	if snap != "" {
		appID := fallbackAppID
		if snapName != "" {
			appID = "snap." + snapName
		}
		return linuxSandbox{
			kind:  linuxSandboxSnap,
			appID: appID,
		}
	}
	return linuxSandbox{
		kind:  linuxSandboxNone,
		appID: fallbackAppID,
	}
}

// permissionStoreResult represents a lookup in org.freedesktop.impl.portal.PermissionStore.
type permissionStoreResult uint8

const (
	// permissionStoreUnavailable means the PermissionStore could not be reached
	// (e.g. blocked by Flatpak's xdg-dbus-proxy or service absent).
	permissionStoreUnavailable permissionStoreResult = iota
	// permissionStoreNotFound means the store answered that no decision is saved
	// (or the entry is explicitly "ask").
	permissionStoreNotFound
	// permissionStoreAllowed means the store grants access ("yes").
	permissionStoreAllowed
	// permissionStoreDenied means the store denies access ("no").
	permissionStoreDenied
	// permissionStoreUnknown means the store returned an unrecognized value.
	permissionStoreUnknown
)

// parsePermissionStoreValues interprets the string slice stored in the XDG
// PermissionStore for devices/camera or devices/microphone.
func parsePermissionStoreValues(values []string) permissionStoreResult {
	if len(values) == 0 {
		return permissionStoreNotFound
	}
	switch strings.ToLower(strings.TrimSpace(values[0])) {
	case "":
		return permissionStoreNotFound
	case "yes":
		return permissionStoreAllowed
	case "no":
		return permissionStoreDenied
	case "ask":
		return permissionStoreNotFound
	default:
		return permissionStoreUnknown
	}
}

// cameraRemoteProbe represents a non-prompting probe of
// org.freedesktop.portal.Camera.OpenPipeWireRemote.
type cameraRemoteProbe uint8

const (
	cameraRemoteUnavailable cameraRemoteProbe = iota
	cameraRemoteGranted
	cameraRemoteNotAllowed
	cameraRemoteOtherError
)

func isPortalNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "org.freedesktop.portal.Error.NotFound")
}

func isPortalNotAllowedError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "org.freedesktop.portal.Error.NotAllowed") ||
		strings.Contains(msg, "org.freedesktop.DBus.Error.AccessDenied")
}

func isPortalUnknownMethodError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "org.freedesktop.DBus.Error.UnknownMethod")
}

func isPortalServiceUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "org.freedesktop.DBus.Error.ServiceUnknown") ||
		strings.Contains(msg, "org.freedesktop.DBus.Error.NameHasNoOwner") ||
		strings.Contains(msg, "org.freedesktop.DBus.Error.UnknownInterface") ||
		strings.Contains(msg, "org.freedesktop.DBus.Error.UnknownObject") ||
		strings.Contains(msg, "org.freedesktop.DBus.Error.UnknownMethod")
}

// classifyCameraRemoteError classifies a D-Bus error returned by
// org.freedesktop.portal.Camera.OpenPipeWireRemote.
func classifyCameraRemoteError(err error) cameraRemoteProbe {
	if err == nil {
		return cameraRemoteGranted
	}
	if isPortalNotAllowedError(err) {
		return cameraRemoteNotAllowed
	}
	if isPortalServiceUnavailableError(err) {
		return cameraRemoteUnavailable
	}
	return cameraRemoteOtherError
}

// linuxPermissionBackend abstracts D-Bus portal, PermissionStore, and PipeWire
// operations so sandbox permission logic is fully unit-testable on any host.
type linuxPermissionBackend interface {
	lookupPermissionStore(table, id, appID string) permissionStoreResult
	cameraPortalAvailable() (present bool, ok bool)
	cameraOpenPipeWireRemote() cameraRemoteProbe
	cameraAccess(done func(code uint32, err error)) (cancel func())
	devicePortalAvailable() bool
	deviceAccess(device string, done func(code uint32, err error)) (cancel func())
	probePipeWireMicrophone() pipewireMicProbe
}

// linuxPermissionController resolves permission checks and requests for a
// sandboxed Linux process. Outside Flatpak and Snap every permission is not
// gated and the backend is never called.
type linuxPermissionController struct {
	sandbox linuxSandbox
	backend linuxPermissionBackend

	mu     sync.Mutex
	denied map[string]bool
}

func newLinuxPermissionController(sandbox linuxSandbox, backend linuxPermissionBackend) *linuxPermissionController {
	return &linuxPermissionController{
		sandbox: sandbox,
		backend: backend,
		denied:  map[string]bool{},
	}
}

// needsSandboxQuery reports whether name is gated in the detected sandbox.
func (c *linuxPermissionController) needsSandboxQuery(name string) bool {
	return c.sandbox.kind != linuxSandboxNone &&
		(name == driver.PermissionCamera || name == driver.PermissionMicrophone)
}

func (c *linuxPermissionController) setDenied(name string, denied bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if denied {
		c.denied[name] = true
	} else {
		delete(c.denied, name)
	}
}

func (c *linuxPermissionController) isDenied(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.denied[name]
}

// check returns the current permission status for name without prompting.
func (c *linuxPermissionController) check(name string) int {
	if !c.needsSandboxQuery(name) {
		return driver.PermissionNotGated
	}
	if c.backend == nil {
		return driver.PermissionUnknown
	}
	switch name {
	case driver.PermissionCamera:
		return c.checkCamera()
	case driver.PermissionMicrophone:
		return c.checkMicrophone()
	default:
		return driver.PermissionNotGated
	}
}

func (c *linuxPermissionController) checkCamera() int {
	switch c.backend.lookupPermissionStore(permissionTableDevices, permissionIDCamera, c.sandbox.appID) {
	case permissionStoreAllowed:
		c.setDenied(driver.PermissionCamera, false)
		return driver.PermissionGranted
	case permissionStoreDenied:
		c.setDenied(driver.PermissionCamera, true)
		return driver.PermissionDenied
	case permissionStoreNotFound:
		if _, ok := c.backend.cameraPortalAvailable(); ok {
			return driver.PermissionNotAsked
		}
		return driver.PermissionUnknown
	case permissionStoreUnknown:
		return driver.PermissionUnknown
	}

	// PermissionStore is unreachable (for example, filtered by Flatpak's D-Bus
	// proxy). OpenPipeWireRemote checks whether camera access is already granted
	// without showing any prompt.
	switch c.backend.cameraOpenPipeWireRemote() {
	case cameraRemoteGranted:
		c.setDenied(driver.PermissionCamera, false)
		return driver.PermissionGranted
	case cameraRemoteNotAllowed:
		if c.isDenied(driver.PermissionCamera) {
			return driver.PermissionDenied
		}
		return driver.PermissionNotAsked
	default:
		return driver.PermissionUnknown
	}
}

func (c *linuxPermissionController) checkMicrophone() int {
	switch c.backend.lookupPermissionStore(permissionTableDevices, permissionIDMicrophone, c.sandbox.appID) {
	case permissionStoreAllowed:
		c.setDenied(driver.PermissionMicrophone, false)
		return driver.PermissionGranted
	case permissionStoreDenied:
		c.setDenied(driver.PermissionMicrophone, true)
		return driver.PermissionDenied
	case permissionStoreNotFound:
		switch c.backend.probePipeWireMicrophone() {
		case pipewireMicGranted:
			c.setDenied(driver.PermissionMicrophone, false)
			return driver.PermissionGranted
		case pipewireMicDenied:
			c.setDenied(driver.PermissionMicrophone, true)
			return driver.PermissionDenied
		case pipewireMicNoSources:
			return driver.PermissionNotAsked
		default:
			if c.backend.devicePortalAvailable() {
				return driver.PermissionNotAsked
			}
			return driver.PermissionUnknown
		}
	case permissionStoreUnknown:
		return driver.PermissionUnknown
	}

	// PermissionStore is unreachable: inspect PipeWire device visibility and the
	// Device portal.
	switch c.backend.probePipeWireMicrophone() {
	case pipewireMicGranted:
		c.setDenied(driver.PermissionMicrophone, false)
		return driver.PermissionGranted
	case pipewireMicDenied:
		c.setDenied(driver.PermissionMicrophone, true)
		return driver.PermissionDenied
	case pipewireMicNoSources:
		if c.isDenied(driver.PermissionMicrophone) {
			return driver.PermissionDenied
		}
		return driver.PermissionNotAsked
	default:
		if c.backend.devicePortalAvailable() {
			if c.isDenied(driver.PermissionMicrophone) {
				return driver.PermissionDenied
			}
			return driver.PermissionNotAsked
		}
		return driver.PermissionUnknown
	}
}

// shouldSkipMicrophonePrompt reports whether microphone access is already
// granted in the PermissionStore before attempting AccessDevice.
func (c *linuxPermissionController) shouldSkipMicrophonePrompt() (status int, skip bool) {
	if c.backend == nil {
		return driver.PermissionUnknown, true
	}
	if c.backend.lookupPermissionStore(permissionTableDevices, permissionIDMicrophone, c.sandbox.appID) == permissionStoreAllowed {
		c.setDenied(driver.PermissionMicrophone, false)
		return driver.PermissionGranted, true
	}
	return 0, false
}

// resolveCameraAskResponse maps an AccessCamera D-Bus response or call error
// into a driver permission status.
func (c *linuxPermissionController) resolveCameraAskResponse(code uint32, err error) int {
	if err != nil {
		if isPortalNotAllowedError(err) {
			c.setDenied(driver.PermissionCamera, true)
			return driver.PermissionDenied
		}
		return c.checkCamera()
	}
	switch code {
	case portalResponseSuccess:
		c.setDenied(driver.PermissionCamera, false)
		return driver.PermissionGranted
	case portalResponseCancelled:
		switch c.backend.lookupPermissionStore(permissionTableDevices, permissionIDCamera, c.sandbox.appID) {
		case permissionStoreAllowed:
			c.setDenied(driver.PermissionCamera, false)
			return driver.PermissionGranted
		case permissionStoreDenied:
			c.setDenied(driver.PermissionCamera, true)
			return driver.PermissionDenied
		case permissionStoreNotFound:
			c.setDenied(driver.PermissionCamera, false)
			return driver.PermissionNotAsked
		default:
			c.setDenied(driver.PermissionCamera, true)
			return driver.PermissionDenied
		}
	default:
		switch c.backend.lookupPermissionStore(permissionTableDevices, permissionIDCamera, c.sandbox.appID) {
		case permissionStoreAllowed:
			c.setDenied(driver.PermissionCamera, false)
			return driver.PermissionGranted
		case permissionStoreDenied:
			c.setDenied(driver.PermissionCamera, true)
			return driver.PermissionDenied
		default:
			return driver.PermissionUnknown
		}
	}
}

// resolveMicrophoneAskResponse maps an AccessDevice("microphone") D-Bus
// response or call error into a driver permission status, falling back to
// PermissionStore and PipeWire device access when AccessDevice is restricted
// or unavailable.
func (c *linuxPermissionController) resolveMicrophoneAskResponse(code uint32, err error) int {
	if err != nil {
		return c.resolveMicrophoneFallback()
	}
	switch code {
	case portalResponseSuccess:
		c.setDenied(driver.PermissionMicrophone, false)
		return driver.PermissionGranted
	case portalResponseCancelled:
		switch c.backend.lookupPermissionStore(permissionTableDevices, permissionIDMicrophone, c.sandbox.appID) {
		case permissionStoreAllowed:
			c.setDenied(driver.PermissionMicrophone, false)
			return driver.PermissionGranted
		case permissionStoreDenied:
			c.setDenied(driver.PermissionMicrophone, true)
			return driver.PermissionDenied
		case permissionStoreNotFound:
			c.setDenied(driver.PermissionMicrophone, false)
			return driver.PermissionNotAsked
		default:
			c.setDenied(driver.PermissionMicrophone, true)
			return driver.PermissionDenied
		}
	default:
		return c.resolveMicrophoneFallback()
	}
}

func (c *linuxPermissionController) resolveMicrophoneFallback() int {
	switch c.backend.lookupPermissionStore(permissionTableDevices, permissionIDMicrophone, c.sandbox.appID) {
	case permissionStoreAllowed:
		c.setDenied(driver.PermissionMicrophone, false)
		return driver.PermissionGranted
	case permissionStoreDenied:
		c.setDenied(driver.PermissionMicrophone, true)
		return driver.PermissionDenied
	}
	switch c.backend.probePipeWireMicrophone() {
	case pipewireMicGranted:
		c.setDenied(driver.PermissionMicrophone, false)
		return driver.PermissionGranted
	case pipewireMicDenied:
		c.setDenied(driver.PermissionMicrophone, true)
		return driver.PermissionDenied
	}
	if c.isDenied(driver.PermissionMicrophone) {
		return driver.PermissionDenied
	}
	return driver.PermissionUnknown
}

// ask requests permission for name and invokes done once with the resulting
// status. The returned cancel function suppresses any pending completion and
// cancels the underlying portal request if still in flight.
func (c *linuxPermissionController) ask(name string, done func(int)) (cancel func()) {
	if !c.needsSandboxQuery(name) {
		if done != nil {
			done(driver.PermissionNotGated)
		}
		return func() {}
	}
	if c.backend == nil {
		if done != nil {
			done(driver.PermissionUnknown)
		}
		return func() {}
	}
	var once sync.Once
	var mu sync.Mutex
	var cancelled bool
	var cancelReq func()

	finish := func(status int) {
		once.Do(func() {
			mu.Lock()
			wasCancelled := cancelled
			mu.Unlock()
			if !wasCancelled && done != nil {
				done(status)
			}
		})
	}

	switch name {
	case driver.PermissionCamera:
		reqCancel := c.backend.cameraAccess(func(code uint32, err error) {
			finish(c.resolveCameraAskResponse(code, err))
		})
		mu.Lock()
		cancelReq = reqCancel
		if cancelled && cancelReq != nil {
			cancelReq()
		}
		mu.Unlock()
	case driver.PermissionMicrophone:
		if status, skip := c.shouldSkipMicrophonePrompt(); skip {
			finish(status)
			return func() {}
		}
		reqCancel := c.backend.deviceAccess(permissionIDMicrophone, func(code uint32, err error) {
			finish(c.resolveMicrophoneAskResponse(code, err))
		})
		mu.Lock()
		cancelReq = reqCancel
		if cancelled && cancelReq != nil {
			cancelReq()
		}
		mu.Unlock()
	default:
		finish(driver.PermissionNotGated)
	}

	return func() {
		mu.Lock()
		if cancelled {
			mu.Unlock()
			return
		}
		cancelled = true
		fn := cancelReq
		mu.Unlock()
		if fn != nil {
			fn()
		}
	}
}

// handle executes an ItemPermission command through c and dispatches its
// EventPermission response.
func (c *linuxPermissionController) handle(id uint32, r driver.PermissionRequest, dispatch func(driver.Event)) (cancel func()) {
	switch r.Command {
	case driver.PermissionStopGuide:
		return func() {}
	case driver.PermissionOpenSettings:
		dispatch(driver.Event{Kind: driver.EventPermission, Count: id})
		return func() {}
	case driver.PermissionAsk:
		return c.ask(r.Name, func(status int) {
			dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Index: status})
		})
	default:
		// PermissionCheck and PermissionGuide (Linux has no settings page to guide to).
		dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Index: c.check(r.Name)})
		return func() {}
	}
}

//go:build darwin

package privacy

import (
	"log"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

// The frameworks that keep the permissions phones and Macs share, loaded as one is first asked about.
const (
	avFoundationPath  = "/System/Library/Frameworks/AVFoundation.framework/AVFoundation"
	contactsPath      = "/System/Library/Frameworks/Contacts.framework/Contacts"
	eventKitPath      = "/System/Library/Frameworks/EventKit.framework/EventKit"
	photosPath        = "/System/Library/Frameworks/Photos.framework/Photos"
	coreBluetoothPath = "/System/Library/Frameworks/CoreBluetooth.framework/CoreBluetooth"
	coreLocationPath  = "/System/Library/Frameworks/CoreLocation.framework/CoreLocation"
)

// The statuses of TCC as AVFoundation, Contacts, EventKit, Photos and CoreBluetooth number them; MediaPlayer swaps denied and restricted, which tell the same.
const (
	tccNotDetermined = 0
	tccAuthorized    = 3
	tccLimited       = 4

	cnEntityTypeContacts = 0

	ekEntityTypeEvent    = 0
	ekEntityTypeReminder = 1

	phAccessLevelReadWrite = 2
)

// pollInterval is how often a request the system answers with no callback looks at the status again, and pollLimit how long it waits for the user.
const (
	pollInterval = 250 * time.Millisecond
	pollLimit    = 10 * time.Minute
)

var (
	frameworks sync.Map // path → whether it loaded
	// contactStore, eventStore and centralManager are kept: a request dies with its store, and CoreBluetooth asks while its manager lives.
	requestMu      sync.Mutex
	contactStore   objc.ID
	eventStore     objc.ID
	centralManager objc.ID
)

func sel(name string) objc.SEL { return objc.RegisterName(name) }

func class(name string) objc.ID { return objc.ID(objc.GetClass(name)) }

// loaded loads the framework at path once, reporting whether it has one.
func loaded(path string) bool {
	if v, ok := frameworks.Load(path); ok {
		return v.(bool)
	}
	_, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	frameworks.Store(path, err == nil)
	return err == nil
}

// global reads the object the framework at path exports as name, 0 when it has none.
func global(path, name string) objc.ID {
	lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0
	}
	addr, err := purego.Dlsym(lib, name)
	if err != nil || addr == 0 {
		return 0
	}
	return **(**objc.ID)(unsafe.Pointer(&addr))
}

// withString passes fn an NSString of s, released once fn returns, so callers on goroutines without an autorelease pool leak nothing.
func withString(s string, fn func(objc.ID)) {
	b := append([]byte(s), 0)
	ns := class("NSString").Send(sel("alloc")).Send(sel("initWithUTF8String:"), &b[0])
	defer ns.Send(sel("release"))
	fn(ns)
}

func responds(o objc.ID, name string) bool {
	return o != 0 && objc.Send[bool](o, sel("respondsToSelector:"), sel(name))
}

// fromTCC is the status of TCC's raw value v: EventKit's 4, write only, is not the access asked for, and MediaPlayer's 1 and 2 are both refusals.
func fromTCC(v int) int {
	switch v {
	case tccNotDetermined:
		return driver.PermissionNotAsked
	case tccAuthorized:
		return driver.PermissionGranted
	case 1, 2, 4:
		return driver.PermissionDenied
	}
	return driver.PermissionUnknown
}

// fromTCCLimited is fromTCC where raw 4 is limited access (PHAuthorizationStatusLimited, CNAuthorizationStatusLimited).
func fromTCCLimited(v int) int {
	if v == tccLimited {
		return driver.PermissionLimited
	}
	return fromTCC(v)
}

func granted(ok bool) int {
	if ok {
		return driver.PermissionGranted
	}
	return driver.PermissionDenied
}

// Status is whether the app has the permission name (a driver.Permission* key), as a driver.Permission* status, without asking the user.
func Status(name string) int {
	switch name {
	case driver.PermissionMicrophone, driver.PermissionCamera:
		device, media := captureDevice(name)
		if device == 0 || media == 0 {
			return driver.PermissionUnavailable
		}
		return fromTCC(objc.Send[int](device, sel("authorizationStatusForMediaType:"), media))
	case driver.PermissionContacts:
		store := contactStoreClass()
		if store == 0 {
			return driver.PermissionUnavailable
		}
		return fromTCCLimited(objc.Send[int](store, sel("authorizationStatusForEntityType:"), cnEntityTypeContacts))
	case driver.PermissionCalendars, driver.PermissionReminders:
		store := eventStoreClass()
		if store == 0 {
			return driver.PermissionUnavailable
		}
		return fromTCC(objc.Send[int](store, sel("authorizationStatusForEntityType:"), entityType(name)))
	case driver.PermissionPhotos:
		lib := photoLibraryClass()
		if lib == 0 {
			return driver.PermissionUnavailable
		}
		if responds(lib, "authorizationStatusForAccessLevel:") {
			return fromTCCLimited(objc.Send[int](lib, sel("authorizationStatusForAccessLevel:"), phAccessLevelReadWrite))
		}
		if responds(lib, "authorizationStatus") {
			return fromTCCLimited(objc.Send[int](lib, sel("authorizationStatus")))
		}
		return driver.PermissionUnavailable
	case driver.PermissionBluetooth:
		if !loaded(coreBluetoothPath) {
			return driver.PermissionUnavailable
		}
		cb := class("CBManager")
		if !responds(cb, "authorization") {
			return driver.PermissionUnknown
		}
		return fromTCC(objc.Send[int](cb, sel("authorization")))
	case driver.PermissionLocation, driver.PermissionBackgroundLocation:
		return locationStatus(name == driver.PermissionBackgroundLocation)
	}
	return platformStatus(name)
}

// Request asks the user for the permission name with the system's prompt, where it has one and has not asked yet, and calls answer with its status then, on any goroutine. When the Info.plist the system reads lacks the permission's usage description, it logs the key and answers driver.PermissionUnavailable without asking, as the system would end the app as it asks.
func Request(name string, answer func(status int)) {
	if platformAsks(name) {
		answer(platformRequest(name))
		return
	}
	st := Status(name)
	if st != driver.PermissionNotAsked {
		answer(st)
		return
	}
	if key := usageKey(name); key != "" && !DeclaresUsage(key) {
		log.Printf("nsgo: requesting %s needs %s in the app's Info.plist (nsgo.yaml's apple.info)", name, key)
		answer(driver.PermissionUnavailable)
		return
	}
	switch name {
	case driver.PermissionMicrophone, driver.PermissionCamera:
		device, media := captureDevice(name)
		block := objcblock.New(func(objc.Block, bool) { answer(Status(name)) })
		device.Send(sel("requestAccessForMediaType:completionHandler:"), media, block)
		block.Release()
	case driver.PermissionContacts:
		requestMu.Lock()
		if contactStore == 0 {
			contactStore = contactStoreClass().Send(sel("alloc")).Send(sel("init"))
		}
		store := contactStore
		requestMu.Unlock()
		block := objcblock.New(func(objc.Block, bool, objc.ID) { answer(Status(name)) })
		store.Send(sel("requestAccessForEntityType:completionHandler:"), cnEntityTypeContacts, block)
		block.Release()
	case driver.PermissionCalendars, driver.PermissionReminders:
		requestMu.Lock()
		if eventStore == 0 {
			eventStore = eventStoreClass().Send(sel("alloc")).Send(sel("init"))
		}
		store := eventStore
		requestMu.Unlock()
		block := objcblock.New(func(objc.Block, bool, objc.ID) { answer(Status(name)) })
		switch {
		case fullAccess() && name == driver.PermissionCalendars:
			store.Send(sel("requestFullAccessToEventsWithCompletion:"), block)
		case fullAccess():
			store.Send(sel("requestFullAccessToRemindersWithCompletion:"), block)
		default:
			store.Send(sel("requestAccessToEntityType:completion:"), entityType(name), block)
		}
		block.Release()
	case driver.PermissionPhotos:
		lib := photoLibraryClass()
		block := objcblock.New(func(_ objc.Block, status int) { answer(fromTCCLimited(status)) })
		if responds(lib, "requestAuthorizationForAccessLevel:handler:") {
			lib.Send(sel("requestAuthorizationForAccessLevel:handler:"), phAccessLevelReadWrite, block)
		} else {
			lib.Send(sel("requestAuthorization:"), block)
		}
		block.Release()
	case driver.PermissionBluetooth:
		requestMu.Lock()
		if centralManager == 0 {
			centralManager = newCentralManager()
		}
		requestMu.Unlock()
		// CoreBluetooth calls nothing back: wait for the status to change.
		go func() {
			for start := time.Now(); time.Since(start) < pollLimit; {
				time.Sleep(pollInterval)
				if st := Status(name); st != driver.PermissionNotAsked {
					answer(st)
					return
				}
			}
			answer(Status(name))
		}()
	default:
		platformAskOnce(name, answer)
	}
}

// newCentralManager makes the CBCentralManager whose creation shows Bluetooth's prompt, with no delegate, as the prompt shows without one.
func newCentralManager() objc.ID {
	options := centralManagerOptions()
	defer options.Send(sel("release"))
	return class("CBCentralManager").Send(sel("alloc")).Send(sel("initWithDelegate:queue:options:"), objc.ID(0), objc.ID(0), options)
}

// centralManagerOptions are a new manager's options, for the caller to release: the power alert off, so asking for the permission while the radio is off does not also ask the user to turn it on.
func centralManagerOptions() objc.ID {
	options := class("NSMutableDictionary").Send(sel("alloc")).Send(sel("init"))
	if key := global(coreBluetoothPath, "CBCentralManagerOptionShowPowerAlertKey"); key != 0 {
		no := class("NSNumber").Send(sel("alloc")).Send(sel("initWithBool:"), false)
		options.Send(sel("setObject:forKey:"), no, key)
		no.Send(sel("release"))
	}
	return options
}

// usageKey is the Info.plist key that says why the app asks for the permission name.
func usageKey(name string) string {
	switch name {
	case driver.PermissionMicrophone:
		return "NSMicrophoneUsageDescription"
	case driver.PermissionCamera:
		return "NSCameraUsageDescription"
	case driver.PermissionContacts:
		return "NSContactsUsageDescription"
	case driver.PermissionCalendars:
		if fullAccess() {
			return "NSCalendarsFullAccessUsageDescription"
		}
		return "NSCalendarsUsageDescription"
	case driver.PermissionReminders:
		if fullAccess() {
			return "NSRemindersFullAccessUsageDescription"
		}
		return "NSRemindersUsageDescription"
	case driver.PermissionPhotos:
		return "NSPhotoLibraryUsageDescription"
	case driver.PermissionBluetooth:
		return "NSBluetoothAlwaysUsageDescription"
	case driver.PermissionMediaLibrary:
		return "NSAppleMusicUsageDescription"
	}
	if _, ok := driver.AutomationTarget(name); ok {
		return "NSAppleEventsUsageDescription"
	}
	return ""
}

// capture is AVFoundation's media types, read once.
var capture struct {
	once         sync.Once
	audio, video objc.ID
}

// captureDevice is AVCaptureDevice and the media type of the microphone or the camera, 0 where the system has none.
func captureDevice(name string) (device, media objc.ID) {
	capture.once.Do(func() {
		if loaded(avFoundationPath) {
			capture.audio, capture.video = global(avFoundationPath, "AVMediaTypeAudio"), global(avFoundationPath, "AVMediaTypeVideo")
		}
	})
	media = capture.audio
	if name == driver.PermissionCamera {
		media = capture.video
	}
	device = class("AVCaptureDevice")
	if media == 0 || !responds(device, "authorizationStatusForMediaType:") {
		return 0, 0
	}
	return device, media
}

// contactStoreClass is CNContactStore, 0 where the system has no Contacts framework, as tvOS has not.
func contactStoreClass() objc.ID {
	if !loaded(contactsPath) {
		return 0
	}
	store := class("CNContactStore")
	if !responds(store, "authorizationStatusForEntityType:") {
		return 0
	}
	return store
}

// photoLibraryClass is PHPhotoLibrary, 0 where the system has no Photos framework.
func photoLibraryClass() objc.ID {
	if !loaded(photosPath) {
		return 0
	}
	return class("PHPhotoLibrary")
}

// locationStatus reads CLLocationManager's authorization status without asking, matching apple/sensors.go.
func locationStatus(background bool) int {
	if !loaded(coreLocationPath) {
		return driver.PermissionUnavailable
	}
	cl := class("CLLocationManager")
	if cl == 0 {
		return driver.PermissionUnavailable
	}
	if responds(cl, "locationServicesEnabled") && !objc.Send[bool](cl, sel("locationServicesEnabled")) {
		return driver.PermissionUnavailable
	}
	if background && !backgroundLocationDeclared() {
		return driver.PermissionUnavailable
	}
	if !responds(cl, "authorizationStatus") {
		return driver.PermissionUnknown
	}
	switch objc.Send[int](cl, sel("authorizationStatus")) {
	case 0:
		return driver.PermissionNotAsked
	case 1, 2:
		return driver.PermissionDenied
	case 3, 4:
		return driver.PermissionGranted
	}
	return driver.PermissionUnknown
}

func backgroundLocationDeclared() bool {
	info := class("NSBundle").Send(sel("mainBundle")).Send(sel("infoDictionary"))
	if info == 0 {
		return false
	}
	declared := false
	withString("UIBackgroundModes", func(key objc.ID) {
		modes := info.Send(sel("objectForKey:"), key)
		if modes == 0 || !objc.Send[bool](modes, sel("isKindOfClass:"), class("NSArray")) {
			return
		}
		withString("location", func(loc objc.ID) {
			declared = objc.Send[bool](modes, sel("containsObject:"), loc)
		})
	})
	return declared
}

// eventStoreClass is EKEventStore, 0 where the system has no EventKit, as tvOS has not.
func eventStoreClass() objc.ID {
	if !loaded(eventKitPath) {
		return 0
	}
	return class("EKEventStore")
}

// fullAccess says EventKit asks for full access, as from macOS 14 and iOS 17, with keys of its own.
func fullAccess() bool {
	store := eventStoreClass()
	return store != 0 && objc.Send[bool](store, sel("instancesRespondToSelector:"), sel("requestFullAccessToEventsWithCompletion:"))
}

func entityType(name string) int {
	if name == driver.PermissionReminders {
		return ekEntityTypeReminder
	}
	return ekEntityTypeEvent
}

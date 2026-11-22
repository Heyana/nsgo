//go:build ios

package privacy

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

const (
	mediaPlayerPath = "/System/Library/Frameworks/MediaPlayer.framework/MediaPlayer"
	coreMotionPath  = "/System/Library/Frameworks/CoreMotion.framework/CoreMotion"
)

// mediaLibrary is MPMediaLibrary, 0 where the system has none, as tvOS has not.
func mediaLibrary() objc.ID {
	if !loaded(mediaPlayerPath) {
		return 0
	}
	if lib := class("MPMediaLibrary"); responds(lib, "authorizationStatus") {
		return lib
	}
	return 0
}

// platformStatus answers the media library, motion, and what only Macs keep, which iOS has not.
func platformStatus(name string) int {
	switch name {
	case driver.PermissionMediaLibrary:
		lib := mediaLibrary()
		if lib == 0 {
			return driver.PermissionUnavailable
		}
		return fromTCC(objc.Send[int](lib, sel("authorizationStatus")))
	case driver.PermissionMotion:
		if !loaded(coreMotionPath) {
			return driver.PermissionUnavailable
		}
		cm := class("CMMotionManager")
		if cm == 0 {
			return driver.PermissionUnavailable
		}
		if !responds(cm, "authorizationStatus") {
			return driver.PermissionGranted
		}
		return fromTCC(objc.Send[int](cm, sel("authorizationStatus")))
	}
	return driver.PermissionUnavailable
}

// platformAsks says Request answers name with platformRequest: what only Macs keep and what sensors handle.
func platformAsks(name string) bool {
	switch name {
	case driver.PermissionMicrophone, driver.PermissionCamera, driver.PermissionContacts,
		driver.PermissionCalendars, driver.PermissionReminders, driver.PermissionPhotos,
		driver.PermissionBluetooth, driver.PermissionMediaLibrary:
		return false
	}
	return true
}

func platformRequest(name string) int { return Status(name) }

// platformAskOnce asks for the media library.
func platformAskOnce(name string, answer func(int)) {
	block := objcblock.New(func(objc.Block, int) { answer(Status(name)) })
	mediaLibrary().Send(sel("requestAuthorization:"), block)
	block.Release()
}

// DeclaresUsage reports whether the app's Info.plist declares key, as iOS ends an app that asks for what it declares no reason for.
func DeclaresUsage(key string) bool {
	bundle := class("NSBundle").Send(sel("mainBundle"))
	declared := false
	withString(key, func(k objc.ID) { declared = bundle.Send(sel("objectForInfoDictionaryKey:"), k) != 0 })
	return declared
}

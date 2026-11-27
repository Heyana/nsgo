package miniprogram

import (
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// permissionScope maps an NSGo permission key to WeChat's scope name, which
// api.js normalizes for QQ, Douyin and Alipay.
func permissionScope(name string) (string, bool) {
	switch name {
	case driver.PermissionMicrophone:
		return "scope.record", true
	case driver.PermissionCamera:
		return "scope.camera", true
	case driver.PermissionBluetooth:
		return "scope.bluetooth", true
	}
	return "", false
}

// scopeStatus maps an authSetting entry to a driver permission status.
// present reports whether the scope key existed in authSetting, boolean whether
// its value was a JS boolean, and granted its boolean value.
func scopeStatus(present, boolean, granted bool) int {
	if !present {
		return driver.PermissionNotAsked
	}
	if !boolean {
		return driver.PermissionUnknown
	}
	if granted {
		return driver.PermissionGranted
	}
	return driver.PermissionDenied
}

// authorizeFailureStatus classifies an authorize failure message into a
// terminal permission status when the host reports refusal or an unsupported
// scope.
func authorizeFailureStatus(errMsg string) (int, bool) {
	lower := strings.ToLower(errMsg)
	switch {
	case strings.Contains(lower, "invalid scope"),
		strings.Contains(lower, "not support"),
		strings.Contains(lower, "unsupported"),
		strings.Contains(lower, "is missing"):
		return driver.PermissionUnavailable, true
	case strings.Contains(lower, "deny"),
		strings.Contains(lower, "denied"),
		strings.Contains(lower, "refuse"),
		strings.Contains(lower, "reject"):
		return driver.PermissionDenied, true
	}
	return 0, false
}

// permissionHost abstracts the normalized mini-program permission APIs
// (getSetting, authorize, openSetting) so host tests exercise the full
// asynchronous state machine without WebAssembly.
type permissionHost interface {
	hasGetSetting() bool
	getSetting(scope string, done func(status int, ok bool))
	hasAuthorize() bool
	authorize(scope string, done func(ok bool, errMsg string))
	hasOpenSetting() bool
	openSetting(done func(ok bool))
}

// handlePermission executes an ItemPermission against host and dispatches its
// EventPermission reply (except for PermissionStopGuide, which sends none).
func handlePermission(id uint32, r driver.PermissionRequest, host permissionHost, dispatch func(driver.Event)) {
	if r.Command == driver.PermissionStopGuide {
		return
	}
	scope, supported := permissionScope(r.Name)
	if r.Command == driver.PermissionOpenSettings {
		opened := func(ok bool) {
			dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Finished: ok})
		}
		if !supported || host == nil || !host.hasOpenSetting() {
			opened(false)
			return
		}
		host.openSetting(opened)
		return
	}
	answer := func(status int) {
		dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Index: status})
	}
	if !supported || host == nil {
		answer(driver.PermissionUnavailable)
		return
	}
	hasGet := host.hasGetSetting()
	hasAuth := host.hasAuthorize()
	if !hasGet && !hasAuth {
		answer(driver.PermissionUnavailable)
		return
	}
	if r.Command != driver.PermissionAsk {
		// PermissionCheck and PermissionGuide (mini programs have no settings guide).
		if !hasGet {
			answer(driver.PermissionUnknown)
			return
		}
		host.getSetting(scope, func(status int, ok bool) {
			if !ok {
				answer(driver.PermissionUnknown)
				return
			}
			answer(status)
		})
		return
	}
	ask := func() {
		if !hasAuth {
			answer(driver.PermissionUnavailable)
			return
		}
		host.authorize(scope, func(ok bool, errMsg string) {
			if ok {
				answer(driver.PermissionGranted)
				return
			}
			classifiedStatus, classified := authorizeFailureStatus(errMsg)
			if classified && classifiedStatus == driver.PermissionUnavailable {
				answer(driver.PermissionUnavailable)
				return
			}
			finish := func(fallback int) {
				if classified && fallback != driver.PermissionGranted && fallback != driver.PermissionDenied {
					answer(classifiedStatus)
					return
				}
				answer(fallback)
			}
			if !hasGet {
				finish(driver.PermissionUnknown)
				return
			}
			host.getSetting(scope, func(status int, ok bool) {
				if !ok {
					finish(driver.PermissionUnknown)
					return
				}
				finish(status)
			})
		})
	}
	if !hasGet {
		ask()
		return
	}
	host.getSetting(scope, func(status int, ok bool) {
		if ok && (status == driver.PermissionGranted || status == driver.PermissionDenied) {
			answer(status)
			return
		}
		if !hasAuth {
			if ok {
				answer(status)
				return
			}
			answer(driver.PermissionUnknown)
			return
		}
		ask()
	})
}

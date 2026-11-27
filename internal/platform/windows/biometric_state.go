package windows

import (
	"sync"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// WinRT Windows.Security.Credentials.UI.UserConsentVerifierAvailability values.
const (
	winHelloAvailAvailable            int32 = 0
	winHelloAvailDeviceNotPresent     int32 = 1
	winHelloAvailNotConfiguredForUser int32 = 2
	winHelloAvailDisabledByPolicy     int32 = 3
	winHelloAvailDeviceBusy           int32 = 4
)

// WinRT Windows.Security.Credentials.UI.UserConsentVerificationResult values.
const (
	winHelloVerifyVerified             int32 = 0
	winHelloVerifyDeviceNotPresent     int32 = 1
	winHelloVerifyNotConfiguredForUser int32 = 2
	winHelloVerifyDisabledByPolicy     int32 = 3
	winHelloVerifyDeviceBusy           int32 = 4
	winHelloVerifyRetriesExhausted     int32 = 5
	winHelloVerifyCanceled             int32 = 6
)

// WinRT Windows.Foundation.AsyncStatus values.
const (
	winHelloAsyncStarted   uintptr = 0
	winHelloAsyncCompleted uintptr = 1
	winHelloAsyncCanceled  uintptr = 2
	winHelloAsyncError     uintptr = 3
)

// HRESULT values mapped from WinRT UserConsentVerifier calls.
const (
	winHelloHRAbort             uint32 = 0x80004004 // E_ABORT
	winHelloHRNotImpl           uint32 = 0x80004001 // E_NOTIMPL
	winHelloHRNoInterface       uint32 = 0x80004002 // E_NOINTERFACE
	winHelloHRClassNotAvailable uint32 = 0x80040111 // CLASS_E_CLASSNOTAVAILABLE
	winHelloHRClassNotReg       uint32 = 0x80040154 // REGDB_E_CLASSNOTREG
	winHelloHRNotSupported      uint32 = 0x80070032 // HRESULT_FROM_WIN32(ERROR_NOT_SUPPORTED)
	winHelloHRNotFound          uint32 = 0x80070490 // HRESULT_FROM_WIN32(ERROR_NOT_FOUND)
	winHelloHRCancelled         uint32 = 0x800704c7 // HRESULT_FROM_WIN32(ERROR_CANCELLED)
)

// windowsHelloAvailabilityKind maps the completion of
// IUserConsentVerifierStatics::CheckAvailabilityAsync to driver's BiometricKind.
func windowsHelloAvailabilityKind(status uintptr, value int32, resultsOK bool) int {
	if status != winHelloAsyncCompleted || !resultsOK {
		return driver.BiometricNone
	}
	if value == winHelloAvailAvailable {
		return driver.BiometricWindowsHello
	}
	return driver.BiometricNone
}

// windowsHelloVerificationResult maps the completion of
// IUserConsentVerifierInterop::RequestVerificationForWindowAsync to driver's
// BiometricResult.
func windowsHelloVerificationResult(status uintptr, value int32, resultsOK bool, errHR uintptr) int {
	switch status {
	case winHelloAsyncCanceled:
		return driver.BiometricCancelled
	case winHelloAsyncCompleted:
		if !resultsOK {
			return driver.BiometricFailure
		}
		switch value {
		case winHelloVerifyVerified:
			return driver.BiometricSuccess
		case winHelloVerifyCanceled:
			return driver.BiometricCancelled
		case winHelloVerifyDeviceNotPresent, winHelloVerifyNotConfiguredForUser, winHelloVerifyDisabledByPolicy:
			return driver.BiometricUnavailable
		case winHelloVerifyRetriesExhausted, winHelloVerifyDeviceBusy:
			return driver.BiometricFailure
		default:
			return driver.BiometricFailure
		}
	case winHelloAsyncError:
		return windowsHelloErrorHRResult(errHR)
	default:
		return driver.BiometricFailure
	}
}

// windowsHelloStartErrorResult maps a synchronous failure starting
// RequestVerificationForWindowAsync (or a nil returned operation) to driver's
// BiometricResult.
func windowsHelloStartErrorResult(hr uintptr) int {
	if hr == 0 || isWindowsHelloUnavailableHR(hr) {
		return driver.BiometricUnavailable
	}
	return windowsHelloErrorHRResult(hr)
}

func windowsHelloErrorHRResult(hr uintptr) int {
	switch uint32(hr) {
	case winHelloHRCancelled, winHelloHRAbort:
		return driver.BiometricCancelled
	}
	if isWindowsHelloUnavailableHR(hr) {
		return driver.BiometricUnavailable
	}
	return driver.BiometricFailure
}

func isWindowsHelloUnavailableHR(hr uintptr) bool {
	switch uint32(hr) {
	case winHelloHRNotImpl, winHelloHRNoInterface, winHelloHRClassNotAvailable,
		winHelloHRClassNotReg, winHelloHRNotSupported, winHelloHRNotFound:
		return true
	}
	return false
}

// biometricRun tracks one in-flight Windows Hello async operation so either its
// WinRT completion callback or window destruction claims and releases it once.
type biometricRun struct {
	owner  any
	cancel func()
}

type biometricTracker struct {
	mu   sync.Mutex
	runs map[*biometricRun]struct{}
}

func (t *biometricTracker) add(r *biometricRun) {
	t.mu.Lock()
	if t.runs == nil {
		t.runs = map[*biometricRun]struct{}{}
	}
	t.runs[r] = struct{}{}
	t.mu.Unlock()
}

// claim removes r from the tracker and reports whether it was still active.
// Whoever claims a run owns its native resources and releases them.
func (t *biometricTracker) claim(r *biometricRun) bool {
	t.mu.Lock()
	_, ok := t.runs[r]
	delete(t.runs, r)
	t.mu.Unlock()
	return ok
}

// cancelOwner removes every active run belonging to owner under the lock and
// invokes its cancel hook outside the lock, because IAsyncInfo::Cancel may
// synchronously invoke the WinRT completion callback on the calling thread.
func (t *biometricTracker) cancelOwner(owner any) {
	t.mu.Lock()
	var owned []*biometricRun
	for r := range t.runs {
		if r.owner == owner {
			delete(t.runs, r)
			owned = append(owned, r)
		}
	}
	t.mu.Unlock()
	for _, r := range owned {
		if r.cancel != nil {
			r.cancel()
		}
	}
}

func (t *biometricTracker) count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.runs)
}

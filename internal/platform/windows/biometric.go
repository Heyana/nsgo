//go:build windows

package windows

import (
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Windows Hello user verification through WinRT's
// Windows.Security.Credentials.UI.UserConsentVerifier.
//
// Availability calls IUserConsentVerifierStatics::CheckAvailabilityAsync without
// showing any UI prompt. Verification calls
// IUserConsentVerifierInterop::RequestVerificationForWindowAsync parented to the
// window's top-level HWND so the Windows Hello dialog stays modal to that window
// while the message pump continues running.
//
// Both IUserConsentVerifierStatics and IUserConsentVerifierInterop inherit from
// IInspectable (vtable slots 0..5), so their first methods live at slot 6.

const (
	userConsentVerifierClass = "Windows.Security.Credentials.UI.UserConsentVerifier"

	ucvCheckAvailabilityAsync                   = 6  // IUserConsentVerifierStatics::CheckAvailabilityAsync
	ucvInteropRequestVerificationForWindowAsync = 6  // IUserConsentVerifierInterop::RequestVerificationForWindowAsync
	ucvAsyncCancel                              = 9  // IAsyncInfo::Cancel
	ucvAsyncClose                               = 10 // IAsyncInfo::Close
)

var (
	iidUserConsentVerifierStatics = parseGUID("af4f3f91-564c-4ddc-b8b5-973447627c65")
	iidUserConsentVerifierInterop = parseGUID("39e050c3-4e74-441a-8dc0-b81104df949c")
	// Parameterized WinRT interface IDs derived from their signatures (namespace
	// 11f47ad5-7b73-42c0-abae-878b1e16adee, SHA-1):
	//   AsyncOperationCompletedHandler<UserConsentVerifierAvailability>
	//   IAsyncOperation<UserConsentVerificationResult>
	//   AsyncOperationCompletedHandler<UserConsentVerificationResult>
	iidUCVAvailabilityCompleted = parseGUID("28988174-ace2-5c15-a0df-580a26d94294")
	iidUCVVerificationOp        = parseGUID("fd596ffd-2318-558f-9dbe-d21df43764a5")
	iidUCVVerificationCompleted = parseGUID("0cffc6c9-4c2b-5cd4-b38c-7b8df3ff5afb")

	windowsBiometricRuns biometricTracker

	windowsBiometricAPIAvailable = func() bool {
		return combase.Load() == nil &&
			roGetActivationFactory.Find() == nil &&
			windowsCreateString.Find() == nil &&
			windowsDeleteString.Find() == nil
	}

	biometricActivationFactory = func(name string, iid *guid) *comObject {
		if !windowsBiometricAPIAvailable() {
			return nil
		}
		return activationFactory(name, iid)
	}
)

func (w *window) biometricAnswer(id uint32, index int) {
	if !w.destroying && !w.biometricsClosed && w.engine != nil {
		w.engine.Dispatch(driver.Event{Kind: driver.EventBiometric, Count: id, Index: index})
	}
}

func (w *window) biometricPostAnswer(id uint32, index int) {
	w.post(func() {
		w.biometricAnswer(id, index)
	})
}

// endWindowBiometrics cancels and releases every pending Windows Hello async
// operation belonging to w as the window is destroyed.
func endWindowBiometrics(w *window) {
	w.biometricsClosed = true
	windowsBiometricRuns.cancelOwner(w)
}

func releaseBiometricAsync(op, asyncInfo *comObject, cancel bool) {
	if asyncInfo != nil {
		if cancel {
			asyncInfo.call(ucvAsyncCancel)
		}
		asyncInfo.call(ucvAsyncClose)
		asyncInfo.release()
	}
	op.release()
}

// biometricAvailability serves driver.ItemBiometricAvailability without showing
// any prompt.
func (w *window) biometricAvailability(id uint32) {
	if w.destroying || w.biometricsClosed {
		return
	}
	comApartment()
	statics := biometricActivationFactory(userConsentVerifierClass, &iidUserConsentVerifierStatics)
	if statics == nil {
		w.biometricAnswer(id, driver.BiometricNone)
		return
	}
	var op *comObject
	hr := statics.call(ucvCheckAvailabilityAsync, uintptr(unsafe.Pointer(&op)))
	statics.release()
	if failed(hr) || op == nil {
		w.biometricAnswer(id, driver.BiometricNone)
		return
	}
	asyncInfo := op.query(&iidAsyncInfo)
	run := &biometricRun{
		owner:  w,
		cancel: func() { releaseBiometricAsync(op, asyncInfo, true) },
	}
	windowsBiometricRuns.add(run)

	completed := newHandler(&iidUCVAvailabilityCompleted, func(_ uintptr, status unsafe.Pointer) {
		if !windowsBiometricRuns.claim(run) {
			return
		}
		st := uintptr(status)
		var avail int32
		resultsOK := st == winHelloAsyncCompleted &&
			!failed(op.call(asyncGetResults, uintptr(unsafe.Pointer(&avail))))
		releaseBiometricAsync(op, asyncInfo, false)
		w.biometricPostAnswer(id, windowsHelloAvailabilityKind(st, avail, resultsOK))
	})
	completed.agile = true
	hr = op.call(asyncPutCompleted, completed.ptr())
	completed.release()
	if failed(hr) && windowsBiometricRuns.claim(run) {
		releaseBiometricAsync(op, asyncInfo, true)
		w.biometricAnswer(id, driver.BiometricNone)
	}
}

// biometricAuthenticate serves driver.ItemBiometricAuthenticate via
// IUserConsentVerifierInterop::RequestVerificationForWindowAsync parented to
// w's top-level HWND.
func (w *window) biometricAuthenticate(id uint32, reason string) {
	if w.destroying || w.biometricsClosed {
		return
	}
	if reason == "" {
		w.biometricAnswer(id, driver.BiometricFailure)
		return
	}
	owner := w.owner()
	if owner == 0 {
		w.biometricAnswer(id, driver.BiometricUnavailable)
		return
	}
	comApartment()
	interop := biometricActivationFactory(userConsentVerifierClass, &iidUserConsentVerifierInterop)
	if interop == nil {
		w.biometricAnswer(id, driver.BiometricUnavailable)
		return
	}
	if !windowsBiometricAPIAvailable() {
		interop.release()
		w.biometricAnswer(id, driver.BiometricUnavailable)
		return
	}
	msg := hstring(reason)
	if msg == 0 {
		interop.release()
		w.biometricAnswer(id, driver.BiometricFailure)
		return
	}
	var op *comObject
	hr := interop.call(
		ucvInteropRequestVerificationForWindowAsync,
		owner,
		msg,
		uintptr(unsafe.Pointer(&iidUCVVerificationOp)),
		uintptr(unsafe.Pointer(&op)),
	)
	deleteHString(msg)
	interop.release()
	if failed(hr) || op == nil {
		w.biometricAnswer(id, windowsHelloStartErrorResult(hr))
		return
	}
	asyncInfo := op.query(&iidAsyncInfo)
	run := &biometricRun{
		owner:  w,
		cancel: func() { releaseBiometricAsync(op, asyncInfo, true) },
	}
	windowsBiometricRuns.add(run)

	completed := newHandler(&iidUCVVerificationCompleted, func(_ uintptr, status unsafe.Pointer) {
		if !windowsBiometricRuns.claim(run) {
			return
		}
		st := uintptr(status)
		var result int32
		var resultsOK bool
		var errHR uintptr
		switch st {
		case winHelloAsyncCompleted:
			resultsOK = !failed(op.call(asyncGetResults, uintptr(unsafe.Pointer(&result))))
		case winHelloAsyncError:
			errHR = asyncErrorCode(op)
		}
		releaseBiometricAsync(op, asyncInfo, false)
		w.biometricPostAnswer(id, windowsHelloVerificationResult(st, result, resultsOK, errHR))
	})
	completed.agile = true
	hr = op.call(asyncPutCompleted, completed.ptr())
	completed.release()
	if failed(hr) && windowsBiometricRuns.claim(run) {
		releaseBiometricAsync(op, asyncInfo, true)
		w.biometricAnswer(id, windowsHelloStartErrorResult(hr))
	}
}

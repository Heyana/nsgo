//go:build windows

package windows

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

func biometricTestWindow(t *testing.T, hwnd uintptr) (*window, func() []driver.Event) {
	t.Helper()
	var mu sync.Mutex
	var events []driver.Event
	eng := engine.Start(noopPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{
		Event: func(_ uint32, ev driver.Event) {
			if ev.Kind == driver.EventBiometric {
				mu.Lock()
				events = append(events, ev)
				mu.Unlock()
			}
		},
	})
	t.Cleanup(eng.Close)
	return &window{engine: eng, hwnd: hwnd}, func() []driver.Event {
		mu.Lock()
		defer mu.Unlock()
		return append([]driver.Event(nil), events...)
	}
}

type fakeUserConsentVerifier struct {
	factoryTable, opTable, infoTable [128]uintptr
	factory, op, info                comObject

	completion *comHandler

	// Configured responses.
	startHR   uintptr
	putHR     uintptr
	resultsHR uintptr
	resultVal int32
	errorCode uint32
	autoFire  bool
	autoStat  uintptr

	// Observed inputs & ref counts.
	gotHWND          uintptr
	gotMessage       string
	gotOpIID         guid
	factoryReleases  atomic.Int32
	opReleases       atomic.Int32
	infoReleases     atomic.Int32
	puts             atomic.Int32
	cancels          atomic.Int32
	closes           atomic.Int32
	cancelFiresAsync bool
}

func newFakeUserConsentVerifier() *fakeUserConsentVerifier {
	f := &fakeUserConsentVerifier{}
	f.factory.vtbl, f.op.vtbl, f.info.vtbl = &f.factoryTable, &f.opTable, &f.infoTable
	one := syscall.NewCallback(func(uintptr) uintptr { return 1 })

	f.infoTable[comAddRef] = one
	f.infoTable[comRelease] = syscall.NewCallback(func(uintptr) uintptr {
		f.infoReleases.Add(1)
		return 0
	})
	f.infoTable[asyncInfoErrorCodeSlot] = syscall.NewCallback(func(_ uintptr, out *int32) uintptr {
		*out = int32(f.errorCode)
		return sOK
	})
	f.infoTable[ucvAsyncCancel] = syscall.NewCallback(func(uintptr) uintptr {
		f.cancels.Add(1)
		if f.cancelFiresAsync && f.completion != nil {
			syscall.SyscallN(f.completion.vtbl[3], f.completion.ptr(), 0, winHelloAsyncCanceled)
		}
		return sOK
	})
	f.infoTable[ucvAsyncClose] = syscall.NewCallback(func(uintptr) uintptr {
		f.closes.Add(1)
		return sOK
	})

	f.opTable[comQueryInterface] = syscall.NewCallback(func(_ uintptr, iid *guid, out **comObject) uintptr {
		if *iid == iidAsyncInfo {
			*out = &f.info
			return sOK
		}
		*out = nil
		return eNoInterface
	})
	f.opTable[comAddRef] = one
	f.opTable[comRelease] = syscall.NewCallback(func(uintptr) uintptr {
		f.opReleases.Add(1)
		return 0
	})
	f.opTable[asyncPutCompleted] = syscall.NewCallback(func(_ uintptr, handler *comHandler) uintptr {
		f.puts.Add(1)
		if failed(f.putHR) {
			return f.putHR
		}
		f.completion = handler
		handlerAddRef(handler)
		if f.autoFire {
			syscall.SyscallN(handler.vtbl[3], handler.ptr(), 0, f.autoStat)
			handlerRelease(handler)
			f.completion = nil
		}
		return sOK
	})
	f.opTable[asyncGetResults] = syscall.NewCallback(func(_ uintptr, out *int32) uintptr {
		if failed(f.resultsHR) {
			return f.resultsHR
		}
		*out = f.resultVal
		return sOK
	})

	f.factoryTable[comAddRef] = one
	f.factoryTable[comRelease] = syscall.NewCallback(func(uintptr) uintptr {
		f.factoryReleases.Add(1)
		return 0
	})
	// Note: slot 6 is shared by IUserConsentVerifierStatics::CheckAvailabilityAsync
	// and IUserConsentVerifierInterop::RequestVerificationForWindowAsync; individual
	// tests bind slot 6 for whichever interface they simulate.
	return f
}

func (f *fakeUserConsentVerifier) asStatics() {
	f.factoryTable[ucvCheckAvailabilityAsync] = syscall.NewCallback(func(_ uintptr, out **comObject) uintptr {
		if failed(f.startHR) {
			*out = nil
			return f.startHR
		}
		*out = &f.op
		return sOK
	})
}

func (f *fakeUserConsentVerifier) asInterop() {
	f.factoryTable[ucvInteropRequestVerificationForWindowAsync] = syscall.NewCallback(func(_ uintptr, hwnd, msg uintptr, iid *guid, out **comObject) uintptr {
		f.gotHWND = hwnd
		f.gotMessage = goHString(msg)
		if iid != nil {
			f.gotOpIID = *iid
		}
		if failed(f.startHR) {
			*out = nil
			return f.startHR
		}
		*out = &f.op
		return sOK
	})
}

func (f *fakeUserConsentVerifier) complete(status uintptr) {
	if f.completion != nil {
		h := f.completion
		f.completion = nil
		syscall.SyscallN(h.vtbl[3], h.ptr(), 0, status)
		handlerRelease(h)
	}
}

func TestWindowsBiometricMissingWinRTAPIFallsBackCleanly(t *testing.T) {
	oldAvail := windowsBiometricAPIAvailable
	windowsBiometricAPIAvailable = func() bool { return false }
	defer func() { windowsBiometricAPIAvailable = oldAvail }()

	w, events := biometricTestWindow(t, 0x1234)
	w.biometricAvailability(10)
	w.biometricAuthenticate(11, "Unlock vault")

	got := events()
	if len(got) != 2 {
		t.Fatalf("events = %+v, want 2", got)
	}
	if got[0].Count != 10 || got[0].Index != driver.BiometricNone {
		t.Errorf("availability = %+v, want BiometricNone", got[0])
	}
	if got[1].Count != 11 || got[1].Index != driver.BiometricUnavailable {
		t.Errorf("authenticate = %+v, want BiometricUnavailable", got[1])
	}
}

func TestWindowsBiometricAvailabilityFlow(t *testing.T) {
	for _, tc := range []struct {
		name  string
		avail int32
		want  int
	}{
		{"available", winHelloAvailAvailable, driver.BiometricWindowsHello},
		{"not configured", winHelloAvailNotConfiguredForUser, driver.BiometricNone},
		{"disabled by policy", winHelloAvailDisabledByPolicy, driver.BiometricNone},
		{"device busy", winHelloAvailDeviceBusy, driver.BiometricNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeUserConsentVerifier()
			f.asStatics()
			f.resultVal = tc.avail

			oldFactory := biometricActivationFactory
			biometricActivationFactory = func(name string, iid *guid) *comObject {
				if name != userConsentVerifierClass || *iid != iidUserConsentVerifierStatics {
					t.Errorf("unexpected activationFactory(%q, %v)", name, iid)
					return nil
				}
				return &f.factory
			}
			defer func() { biometricActivationFactory = oldFactory }()

			w, events := biometricTestWindow(t, 0x100)
			w.biometricAvailability(7)
			if f.factoryReleases.Load() != 1 {
				t.Fatalf("factoryReleases = %d, want 1", f.factoryReleases.Load())
			}
			if f.puts.Load() != 1 {
				t.Fatalf("puts = %d, want 1", f.puts.Load())
			}

			f.complete(winHelloAsyncCompleted)
			w.runTasks()

			got := events()
			if len(got) != 1 || got[0].Count != 7 || got[0].Index != tc.want {
				t.Fatalf("events = %+v, want Count=7 Index=%d", got, tc.want)
			}
			if f.opReleases.Load() != 1 || f.infoReleases.Load() != 1 || f.closes.Load() != 1 {
				t.Fatalf("releases: op=%d info=%d closes=%d", f.opReleases.Load(), f.infoReleases.Load(), f.closes.Load())
			}
			if windowsBiometricRuns.count() != 0 {
				t.Fatalf("leaked active runs: %d", windowsBiometricRuns.count())
			}
		})
	}
}

func TestWindowsBiometricAuthenticateFlow(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    uintptr
		resultVal int32
		errorCode uint32
		startHR   uintptr
		want      int
	}{
		{"verified", winHelloAsyncCompleted, winHelloVerifyVerified, 0, 0, driver.BiometricSuccess},
		{"canceled", winHelloAsyncCompleted, winHelloVerifyCanceled, 0, 0, driver.BiometricCancelled},
		{"not configured", winHelloAsyncCompleted, winHelloVerifyNotConfiguredForUser, 0, 0, driver.BiometricUnavailable},
		{"disabled by policy", winHelloAsyncCompleted, winHelloVerifyDisabledByPolicy, 0, 0, driver.BiometricUnavailable},
		{"retries exhausted", winHelloAsyncCompleted, winHelloVerifyRetriesExhausted, 0, 0, driver.BiometricFailure},
		{"device busy", winHelloAsyncCompleted, winHelloVerifyDeviceBusy, 0, 0, driver.BiometricFailure},
		{"async error cancelled", winHelloAsyncError, 0, winHelloHRCancelled, 0, driver.BiometricCancelled},
		{"sync E_NOINTERFACE", 0, 0, 0, uintptr(winHelloHRNoInterface), driver.BiometricUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeUserConsentVerifier()
			f.asInterop()
			f.resultVal = tc.resultVal
			f.errorCode = tc.errorCode
			f.startHR = tc.startHR

			oldFactory := biometricActivationFactory
			biometricActivationFactory = func(name string, iid *guid) *comObject {
				if name != userConsentVerifierClass || *iid != iidUserConsentVerifierInterop {
					t.Errorf("unexpected activationFactory(%q, %v)", name, iid)
					return nil
				}
				return &f.factory
			}
			defer func() { biometricActivationFactory = oldFactory }()

			const fakeHWND uintptr = 0x5678
			w, events := biometricTestWindow(t, fakeHWND)
			w.biometricAuthenticate(42, "Unlock passwords")

			if f.gotHWND != fakeHWND {
				t.Errorf("gotHWND = 0x%x, want 0x%x", f.gotHWND, fakeHWND)
			}
			if f.gotMessage != "Unlock passwords" {
				t.Errorf("gotMessage = %q, want %q", f.gotMessage, "Unlock passwords")
			}
			if f.gotOpIID != iidUCVVerificationOp {
				t.Errorf("gotOpIID = %v, want %v", f.gotOpIID, iidUCVVerificationOp)
			}
			if f.factoryReleases.Load() != 1 {
				t.Fatalf("factoryReleases = %d, want 1", f.factoryReleases.Load())
			}

			if !failed(tc.startHR) {
				f.complete(tc.status)
				w.runTasks()
				if f.opReleases.Load() != 1 || f.infoReleases.Load() != 1 || f.closes.Load() != 1 {
					t.Fatalf("releases: op=%d info=%d closes=%d", f.opReleases.Load(), f.infoReleases.Load(), f.closes.Load())
				}
			}

			got := events()
			if len(got) != 1 || got[0].Count != 42 || got[0].Index != tc.want {
				t.Fatalf("events = %+v, want Count=42 Index=%d", got, tc.want)
			}
		})
	}
}

func TestWindowsBiometricAuthenticateGuardsEmptyReasonAndZeroHWND(t *testing.T) {
	oldFactory := biometricActivationFactory
	biometricActivationFactory = func(string, *guid) *comObject {
		t.Fatal("activated WinRT despite invalid inputs")
		return nil
	}
	defer func() { biometricActivationFactory = oldFactory }()

	w, events := biometricTestWindow(t, 0x1234)
	w.biometricAuthenticate(1, "")
	w.hwnd = 0
	w.biometricAuthenticate(2, "Confirm")

	got := events()
	if len(got) != 2 {
		t.Fatalf("events = %+v, want 2", got)
	}
	if got[0].Index != driver.BiometricFailure {
		t.Errorf("empty reason = %+v, want BiometricFailure", got[0])
	}
	if got[1].Index != driver.BiometricUnavailable {
		t.Errorf("zero hwnd = %+v, want BiometricUnavailable", got[1])
	}
}

func TestWindowsBiometricWindowCloseCancelsPendingPrompt(t *testing.T) {
	f := newFakeUserConsentVerifier()
	f.asInterop()
	f.cancelFiresAsync = true

	oldFactory := biometricActivationFactory
	biometricActivationFactory = func(string, *guid) *comObject { return &f.factory }
	defer func() { biometricActivationFactory = oldFactory }()

	w, events := biometricTestWindow(t, 0x9999)
	w.biometricAuthenticate(55, "Confirm payment")
	if f.completion == nil || windowsBiometricRuns.count() == 0 {
		t.Fatal("expected pending verification operation")
	}

	// Closing the window cancels and closes the operation, releases COM refs
	// once, and does not deadlock when IAsyncInfo::Cancel synchronously fires
	// the completion callback.
	endWindowBiometrics(w)

	if f.cancels.Load() != 1 || f.closes.Load() != 1 || f.opReleases.Load() != 1 || f.infoReleases.Load() != 1 {
		t.Fatalf("cancels=%d closes=%d opReleases=%d infoReleases=%d", f.cancels.Load(), f.closes.Load(), f.opReleases.Load(), f.infoReleases.Load())
	}

	// A late completion callback arriving after window close must be ignored and
	// must not double-release or dispatch to the closed window.
	f.complete(winHelloAsyncCompleted)
	w.runTasks()

	if f.opReleases.Load() != 1 || f.infoReleases.Load() != 1 {
		t.Fatalf("late completion released again: op=%d info=%d", f.opReleases.Load(), f.infoReleases.Load())
	}
	if got := events(); len(got) != 0 {
		t.Fatalf("closed window received late events: %+v", got)
	}
}

func TestWindowsBiometricAvailabilityAgainstTheSystem(t *testing.T) {
	if os.Getenv("NSGO_WINDOWS_BIOMETRIC_NATIVE") != "1" {
		t.Skip("set NSGO_WINDOWS_BIOMETRIC_NATIVE=1 to query Windows Hello availability on this machine")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	oleInitialize.Call(0)

	w, events := biometricTestWindow(t, 0)
	w.biometricAvailability(1)
	deadline := time.Now().Add(10 * time.Second)
	for len(events()) == 0 {
		w.runTasks()
		if len(events()) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("CheckAvailabilityAsync did not complete")
		}
		time.Sleep(25 * time.Millisecond)
	}
	got := events()
	t.Logf("Windows Hello availability event: %+v", got[0])
	if got[0].Index != driver.BiometricNone && got[0].Index != driver.BiometricWindowsHello {
		t.Fatalf("unexpected biometric kind: %d", got[0].Index)
	}
}

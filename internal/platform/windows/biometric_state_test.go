package windows

import (
	"sync/atomic"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWindowsHelloAvailabilityKind(t *testing.T) {
	tests := []struct {
		name      string
		status    uintptr
		value     int32
		resultsOK bool
		want      int
	}{
		{"available", winHelloAsyncCompleted, winHelloAvailAvailable, true, driver.BiometricWindowsHello},
		{"device not present", winHelloAsyncCompleted, winHelloAvailDeviceNotPresent, true, driver.BiometricNone},
		{"not configured for user", winHelloAsyncCompleted, winHelloAvailNotConfiguredForUser, true, driver.BiometricNone},
		{"disabled by policy", winHelloAsyncCompleted, winHelloAvailDisabledByPolicy, true, driver.BiometricNone},
		{"device busy", winHelloAsyncCompleted, winHelloAvailDeviceBusy, true, driver.BiometricNone},
		{"unknown enum value", winHelloAsyncCompleted, 99, true, driver.BiometricNone},
		{"get results failed", winHelloAsyncCompleted, winHelloAvailAvailable, false, driver.BiometricNone},
		{"async canceled", winHelloAsyncCanceled, winHelloAvailAvailable, true, driver.BiometricNone},
		{"async error", winHelloAsyncError, winHelloAvailAvailable, true, driver.BiometricNone},
		{"async started", winHelloAsyncStarted, winHelloAvailAvailable, true, driver.BiometricNone},
	}
	for _, tc := range tests {
		if got := windowsHelloAvailabilityKind(tc.status, tc.value, tc.resultsOK); got != tc.want {
			t.Errorf("%s: windowsHelloAvailabilityKind(%d, %d, %v) = %d, want %d", tc.name, tc.status, tc.value, tc.resultsOK, got, tc.want)
		}
	}
}

func TestWindowsHelloVerificationResult(t *testing.T) {
	tests := []struct {
		name      string
		status    uintptr
		value     int32
		resultsOK bool
		errHR     uintptr
		want      int
	}{
		{"verified", winHelloAsyncCompleted, winHelloVerifyVerified, true, 0, driver.BiometricSuccess},
		{"canceled", winHelloAsyncCompleted, winHelloVerifyCanceled, true, 0, driver.BiometricCancelled},
		{"device not present", winHelloAsyncCompleted, winHelloVerifyDeviceNotPresent, true, 0, driver.BiometricUnavailable},
		{"not configured", winHelloAsyncCompleted, winHelloVerifyNotConfiguredForUser, true, 0, driver.BiometricUnavailable},
		{"disabled by policy", winHelloAsyncCompleted, winHelloVerifyDisabledByPolicy, true, 0, driver.BiometricUnavailable},
		{"device busy", winHelloAsyncCompleted, winHelloVerifyDeviceBusy, true, 0, driver.BiometricFailure},
		{"retries exhausted", winHelloAsyncCompleted, winHelloVerifyRetriesExhausted, true, 0, driver.BiometricFailure},
		{"unknown result value", winHelloAsyncCompleted, 42, true, 0, driver.BiometricFailure},
		{"completed but GetResults failed", winHelloAsyncCompleted, winHelloVerifyVerified, false, 0, driver.BiometricFailure},
		{"async canceled", winHelloAsyncCanceled, 0, false, 0, driver.BiometricCancelled},
		{"async error ERROR_CANCELLED", winHelloAsyncError, 0, false, uintptr(winHelloHRCancelled), driver.BiometricCancelled},
		{"async error E_ABORT", winHelloAsyncError, 0, false, uintptr(winHelloHRAbort), driver.BiometricCancelled},
		{"async error E_NOINTERFACE", winHelloAsyncError, 0, false, uintptr(winHelloHRNoInterface), driver.BiometricUnavailable},
		{"async error REGDB_E_CLASSNOTREG", winHelloAsyncError, 0, false, uintptr(winHelloHRClassNotReg), driver.BiometricUnavailable},
		{"async error CLASS_E_CLASSNOTAVAILABLE", winHelloAsyncError, 0, false, uintptr(winHelloHRClassNotAvailable), driver.BiometricUnavailable},
		{"async error ERROR_NOT_SUPPORTED", winHelloAsyncError, 0, false, uintptr(winHelloHRNotSupported), driver.BiometricUnavailable},
		{"async error ERROR_NOT_FOUND", winHelloAsyncError, 0, false, uintptr(winHelloHRNotFound), driver.BiometricUnavailable},
		{"async error E_FAIL", winHelloAsyncError, 0, false, 0x80004005, driver.BiometricFailure},
		{"unexpected status", 99, 0, false, 0, driver.BiometricFailure},
	}
	for _, tc := range tests {
		if got := windowsHelloVerificationResult(tc.status, tc.value, tc.resultsOK, tc.errHR); got != tc.want {
			t.Errorf("%s: windowsHelloVerificationResult(%d, %d, %v, 0x%x) = %d, want %d", tc.name, tc.status, tc.value, tc.resultsOK, tc.errHR, got, tc.want)
		}
	}
}

func TestWindowsHelloStartErrorResult(t *testing.T) {
	tests := []struct {
		hr   uintptr
		want int
	}{
		{0, driver.BiometricUnavailable},
		{uintptr(winHelloHRNotImpl), driver.BiometricUnavailable},
		{uintptr(winHelloHRNoInterface), driver.BiometricUnavailable},
		{uintptr(winHelloHRClassNotAvailable), driver.BiometricUnavailable},
		{uintptr(winHelloHRClassNotReg), driver.BiometricUnavailable},
		{uintptr(winHelloHRNotSupported), driver.BiometricUnavailable},
		{uintptr(winHelloHRNotFound), driver.BiometricUnavailable},
		{uintptr(winHelloHRCancelled), driver.BiometricCancelled},
		{uintptr(winHelloHRAbort), driver.BiometricCancelled},
		{0x80004005, driver.BiometricFailure}, // E_FAIL
		{0x80070005, driver.BiometricFailure}, // E_ACCESSDENIED
	}
	for _, tc := range tests {
		if got := windowsHelloStartErrorResult(tc.hr); got != tc.want {
			t.Errorf("windowsHelloStartErrorResult(0x%x) = %d, want %d", tc.hr, got, tc.want)
		}
	}
}

func TestBiometricTrackerSingleClaimAndReentrantCancel(t *testing.T) {
	var tr biometricTracker
	ownerA := new(int)
	ownerB := new(int)

	var cancelsA, cancelsB atomic.Int32
	var reentrantClaimed atomic.Bool

	var runA *biometricRun
	runA = &biometricRun{
		owner: ownerA,
		cancel: func() {
			cancelsA.Add(1)
			// Simulate IAsyncInfo::Cancel synchronously invoking the completion
			// handler on the same thread: claiming must not deadlock and must
			// report false because cancelOwner already claimed ownership.
			if tr.claim(runA) {
				reentrantClaimed.Store(true)
			}
		},
	}
	runB := &biometricRun{
		owner:  ownerB,
		cancel: func() { cancelsB.Add(1) },
	}

	tr.add(runA)
	tr.add(runB)
	if tr.count() != 2 {
		t.Fatalf("count = %d, want 2", tr.count())
	}

	tr.cancelOwner(ownerA)
	if cancelsA.Load() != 1 || cancelsB.Load() != 0 {
		t.Fatalf("cancelsA=%d cancelsB=%d, want 1, 0", cancelsA.Load(), cancelsB.Load())
	}
	if reentrantClaimed.Load() {
		t.Fatal("re-entrant claim during cancelOwner succeeded")
	}
	if tr.claim(runA) {
		t.Fatal("late claim after cancelOwner succeeded")
	}
	if tr.count() != 1 {
		t.Fatalf("count after cancelOwner(ownerA) = %d, want 1", tr.count())
	}

	if !tr.claim(runB) {
		t.Fatal("first claim of active runB failed")
	}
	if tr.claim(runB) {
		t.Fatal("second claim of runB succeeded")
	}
	tr.cancelOwner(ownerB)
	if cancelsB.Load() != 0 {
		t.Fatalf("cancelOwner ran cancel on already-claimed runB (%d)", cancelsB.Load())
	}
}

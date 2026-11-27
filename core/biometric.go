package core

import "github.com/nsgo-dev/nsgo/internal/driver"

// BiometricKind says which enrolled biometric or user-verification method the
// system can use.
type BiometricKind uint8

const (
	BiometricNone BiometricKind = iota
	BiometricFace
	BiometricFingerprint
	// BiometricWindowsHello reports that Windows Hello user verification is
	// available. Windows does not distinguish which factor the user configured,
	// and AuthenticateBiometric accepts any configured Windows Hello method
	// (face, fingerprint, or Windows Hello PIN).
	BiometricWindowsHello
)

// BiometricResult is the result of AuthenticateBiometric.
type BiometricResult uint8

const (
	BiometricSuccess BiometricResult = iota + 1
	BiometricFailure
	BiometricCancelled
	BiometricUnavailable
)

// BiometricAvailability answers whether an enrolled biometric or Windows Hello
// user verification is available, and which kind it is. It does not show a
// system prompt.
func (cx *Scope) BiometricAvailability(done func(BiometricKind)) {
	if done == nil {
		return
	}
	if t := cx.tree(); t != nil && !cx.denied(serviceBiometrics, "") {
		t.request(cx, driver.MountItem{Kind: driver.ItemBiometricAvailability}, func(ev driver.Event) {
			done(BiometricKind(ev.Index))
		})
		return
	}
	done(BiometricNone)
}

// AuthenticateBiometric asks the user to authenticate with reason, which the
// prompt shows and must not be empty: an empty one answers BiometricFailure
// without a prompt. On macOS, iOS, Android and HarmonyOS it uses biometrics
// alone and never falls back to a device passcode; on Windows
// (BiometricWindowsHello), Windows Hello accepts whichever method the user
// configured (face, fingerprint, or Windows Hello PIN).
func (cx *Scope) AuthenticateBiometric(reason string, done func(BiometricResult)) {
	answer := func(result BiometricResult) {
		if done != nil {
			done(result)
		}
	}
	if reason == "" {
		// LocalAuthentication throws on an empty reason, which would end the app.
		answer(BiometricFailure)
		return
	}
	if t := cx.tree(); t != nil && !cx.denied(serviceBiometrics, "") {
		t.request(cx, driver.MountItem{Kind: driver.ItemBiometricAuthenticate, Value: reason}, func(ev driver.Event) {
			answer(BiometricResult(ev.Index))
		})
		return
	}
	answer(BiometricUnavailable)
}

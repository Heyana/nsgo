package ios

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestApplyCapabilitiesTVOSAndIOS(t *testing.T) {
	allProbed := capabilities{
		speech:        true,
		speak:         true,
		audio:         true,
		camera:        true,
		signInApple:   true,
		purchases:     true,
		notifications: true,
		gpu:           true,
	}

	var tv driver.Environment
	tv.OS = driver.OSIOS
	tv.Idiom = driver.IdiomTV
	applyCapabilities(&tv, driver.IdiomTV, allProbed)

	if tv.Speech {
		t.Error("tvOS: Speech = true, want false (tvOS has no Speech framework)")
	}
	if !tv.Speak {
		t.Error("tvOS: Speak = false, want true")
	}
	if tv.Audio {
		t.Error("tvOS: Audio = true, want false (tvOS audio is not enabled yet)")
	}
	if tv.Camera {
		t.Error("tvOS: Camera = true, want false")
	}
	if tv.ScanCode {
		t.Error("tvOS: ScanCode = true, want false")
	}
	if tv.HasWebView {
		t.Error("tvOS: HasWebView = true, want false (tvOS has no WebKit)")
	}
	if !tv.SignInWithApple {
		t.Error("tvOS: SignInWithApple = false, want true")
	}
	if !tv.Purchases {
		t.Error("tvOS: Purchases = false, want true")
	}
	if tv.ScheduledNotifications {
		t.Error("tvOS: ScheduledNotifications = true, want false")
	}
	if tv.GPU || tv.AudioPlaybackControls {
		t.Errorf("tvOS: GPU=%v AudioPlaybackControls=%v, want false, false", tv.GPU, tv.AudioPlaybackControls)
	}
	if tv.CustomCursor || tv.CanSendInput {
		t.Errorf("tvOS: CustomCursor=%v CanSendInput=%v, want false, false", tv.CustomCursor, tv.CanSendInput)
	}

	var phone driver.Environment
	phone.OS = driver.OSIOS
	phone.Idiom = driver.IdiomPhone
	applyCapabilities(&phone, driver.IdiomPhone, allProbed)

	if !phone.Speech || !phone.Speak || !phone.Audio || !phone.Camera || !phone.ScanCode ||
		!phone.HasWebView || !phone.SignInWithApple || !phone.Purchases || !phone.ScheduledNotifications ||
		!phone.GPU || !phone.AudioPlaybackControls || !phone.CanSendInput {
		t.Errorf("iOS phone capabilities = %+v, want all probed iOS capabilities true", phone)
	}
	if phone.CustomCursor {
		t.Error("iOS phone: CustomCursor = true, want false")
	}
}

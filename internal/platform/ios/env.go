package ios

import "github.com/nsgo-dev/nsgo/internal/driver"

// This file builds on every system so tests can verify the iOS and tvOS environment capability contract without the UIKit runtime.

// capabilities are the platform capabilities probed on the device before idiom-specific gates apply. tvOS shares the iOS backend but has no WebKit, Speech framework, camera, code scanner, scheduled alert notifications or Stage-1 audio playback.
type capabilities struct {
	speech        bool
	speak         bool
	audio         bool
	camera        bool
	signInApple   bool
	purchases     bool
	notifications bool
	gpu           bool
}

// applyCapabilities sets the capability flags on env for idiom, gating out features tvOS does not provide or has not enabled yet so core.Env reports them accurately.
func applyCapabilities(env *driver.Environment, idiom driver.Idiom, c capabilities) {
	tv := idiom == driver.IdiomTV
	env.DiscoverServices = !tv
	env.Speech = !tv && c.speech
	env.Speak = c.speak
	env.Audio = !tv && c.audio // none on tvOS in the first stages (docs/architecture/audio.md)
	env.AudioSchedule = !tv && c.audio
	env.AudioPlaybackControls = !tv && c.audio
	env.GPU = !tv && c.gpu // tvOS waits for its own verification (docs/architecture/gpu.md)
	env.Camera = !tv && c.camera
	env.ScanCode = !tv && c.camera
	env.CanSendInput = !tv
	env.HasWebView = !tv
	env.SignInWithApple = c.signInApple
	env.Purchases = c.purchases
	// tvOS's center shows badges only.
	env.ScheduledNotifications = !tv && c.notifications
	env.CustomCursor = false
}

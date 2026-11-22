//go:build darwin

package apple

import (
	"runtime"
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// rateReason reads AVFoundation's own reason constants from a notification like the player's.
func TestVideoRateReason(t *testing.T) {
	// A pool belongs to the thread that made it, so the test stays on it until the drain.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(Class("NSAutoreleasePool")).Send(sel("new"))
	defer pool.Send(sel("drain"))
	for name, want := range map[string]int{
		"AVPlayerRateDidChangeReasonAppBackgrounded":         videoPauseBackground,
		"AVPlayerRateDidChangeReasonAudioSessionInterrupted": driver.VideoPauseInterruption,
		"AVPlayerRateDidChangeReasonSetRateCalled":           driver.VideoPauseControls,
	} {
		info := objc.ID(Class("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"), avName(name), avName("AVPlayerRateDidChangeReasonKey"))
		note := objc.ID(Class("NSNotification")).Send(sel("notificationWithName:object:userInfo:"), avName("AVPlayerRateDidChangeNotification"), objc.ID(0), info)
		if got := rateReason(note); got != want {
			t.Errorf("rateReason(%s) = %d, want %d", name, got, want)
		}
	}
	if got := rateReason(objc.ID(Class("NSNotification")).Send(sel("notificationWithName:object:"), avName("AVPlayerRateDidChangeNotification"), objc.ID(0))); got != driver.VideoPauseControls {
		t.Errorf("rateReason without a reason = %d, want the controls", got)
	}
}

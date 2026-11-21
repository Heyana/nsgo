//go:build darwin

package apple

import (
	"math"
	"runtime"
	"time"

	"github.com/ebitengine/purego/objc"
)

// scheduledAudioTime translates the Go clock's remaining interval to AVAudioTime's hardware host clock. Nil plays at the next opportunity.
func scheduledAudioTime(start time.Time) objc.ID {
	if start.IsZero() {
		return 0
	}
	class := objc.ID(Class("AVAudioTime"))
	delay := time.Until(start)
	if delay <= 0 {
		return 0
	}
	now := machTime()
	offset := objc.Send[uint64](class, sel("hostTimeForSeconds:"), delay.Seconds())
	return class.Send(sel("timeWithHostTime:"), now+offset)
}

func (a *Audio) outputLatency() time.Duration {
	node := a.audioEngine().Send(sel("outputNode"))
	seconds := objc.Send[float64](node, sel("presentationLatency"))
	if seconds <= 0 && runtime.GOOS == "ios" {
		// The session can describe the route before this engine starts.
		session := objc.ID(Class("AVAudioSession")).Send(sel("sharedInstance"))
		seconds = objc.Send[float64](session, sel("outputLatency"))
	}
	if seconds > 0 && seconds < float64(math.MaxInt64)/float64(time.Second) {
		return time.Duration(seconds * float64(time.Second))
	}
	return 0
}

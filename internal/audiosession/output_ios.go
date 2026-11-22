//go:build ios

package audiosession

import (
	"sync"
	"time"

	"github.com/ebitengine/purego/objc"
)

type bufferPreference struct{ seconds float64 }

var buffers struct {
	before float64
	active map[*bufferPreference]bool
}

// HoldOutput keeps an ambient PCM output's session active. Positive hints are preferences for the shared I/O buffer. The shortest requested duration wins and the preference from before the first request returns when they all end.
func HoldOutput(hint time.Duration) (func(), error) {
	var release func()
	var err error
	withPool(func() { release, err = Hold(Ambient) })
	if err != nil {
		return release, err
	}
	if hint <= 0 {
		return func() { withPool(release) }, nil
	}
	p := &bufferPreference{seconds: hint.Seconds()}
	preferBuffer(p, true)
	var once sync.Once
	return func() {
		once.Do(func() {
			preferBuffer(p, false)
			withPool(release)
		})
	}, nil
}

func preferBuffer(p *bufferPreference, add bool) {
	holders.Lock()
	defer holders.Unlock()
	withPool(func() {
		shared := objc.ID(objc.GetClass("AVAudioSession")).Send(sel("sharedInstance"))
		if add {
			if len(buffers.active) == 0 {
				buffers.before = objc.Send[float64](shared, sel("preferredIOBufferDuration"))
				buffers.active = make(map[*bufferPreference]bool)
			}
			buffers.active[p] = true
		} else {
			delete(buffers.active, p)
		}
		var wanted float64
		for current := range buffers.active {
			if wanted == 0 || current.seconds < wanted {
				wanted = current.seconds
			}
		}
		if wanted == 0 {
			wanted = buffers.before
		}
		if objc.Send[float64](shared, sel("preferredIOBufferDuration")) != wanted {
			objc.Send[bool](shared, sel("setPreferredIOBufferDuration:error:"), wanted, objc.ID(0))
		}
	})
}

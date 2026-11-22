//go:build ios

package apple

import (
	"errors"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/audiosession"
)

var (
	errPCMInterrupted = errors.New("nsgo: audio: the PCM output was interrupted")
	errPCMRoute       = errors.New("nsgo: audio: the output route changed")
	errPCMDevice      = errors.New("nsgo: audio: the system's media services stopped")
)

func (o *audioOutput) watchRoute() func() {
	if !loadAudio() {
		return nil
	}
	var stopInterruption, stopRoute, stopDevice func()
	withPool(func() {
		center := objc.ID(Class("NSNotificationCenter")).Send(sel("defaultCenter"))
		stopInterruption = Observe(center, avName("AVAudioSessionInterruptionNotification"), func(note objc.ID) {
			if interruptionBegan(note) {
				go o.interrupt(errPCMInterrupted)
			}
		})
		stopRoute = Observe(center, avName("AVAudioSessionRouteChangeNotification"), func(note objc.ID) {
			info := note.Send(sel("userInfo"))
			value := info.Send(sel("objectForKey:"), avName("AVAudioSessionRouteChangeReasonKey"))
			switch value.Send(sel("unsignedIntegerValue")) {
			case 1, 2, 4, 6, 7, 8:
				go o.interrupt(errPCMRoute)
			}
		})
		stopDevice = Observe(center, avName("AVAudioSessionMediaServicesWereLostNotification"), func(objc.ID) {
			go o.interrupt(errPCMDevice)
		})
	})
	return func() {
		OnMain(func() {
			stopInterruption()
			stopRoute()
			stopDevice()
		})
	}
}

func (o *audioOutput) holdSession() (func(), error) {
	return audiosession.HoldOutput(o.conf.Latency)
}

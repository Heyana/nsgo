//go:build darwin && !ios

package apple

import (
	"errors"
	"sync"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/audiosession"
)

type pcmHardwareProperty struct {
	selector, scope, element uint32
}

var pcmDefaultDevice = pcmHardwareProperty{0x644f7574, 0x676c6f62, 0} // 'dOut', 'glob', main element

var pcmDevice struct {
	add, remove func(uint32, *pcmHardwareProperty, uintptr, uintptr) int32
	changed     uintptr
}

var loadPCMDevice = sync.OnceFunc(func() {
	lib := load("/System/Library/Frameworks/CoreAudio.framework/CoreAudio")
	purego.RegisterLibFunc(&pcmDevice.add, lib, "AudioObjectAddPropertyListener")
	purego.RegisterLibFunc(&pcmDevice.remove, lib, "AudioObjectRemovePropertyListener")
	pcmDevice.changed = purego.NewCallback(func(_, _ uint32, _ *pcmHardwareProperty, id uintptr) int32 {
		if stream, ok := pcmStreams.Load(id); ok {
			go stream.(*audioOutput).interrupt(errPCMRoute)
		}
		return 0
	})
})

var errPCMRoute = errors.New("nsgo: audio: the output device changed")

func (o *audioOutput) watchRoute() func() {
	loadPCMDevice()
	const systemObject = 1
	if pcmDevice.add(systemObject, &pcmDefaultDevice, pcmDevice.changed, o.id) != 0 {
		return nil
	}
	return func() { pcmDevice.remove(systemObject, &pcmDefaultDevice, pcmDevice.changed, o.id) }
}

func (o *audioOutput) holdSession() (func(), error) {
	// The HAL buffer size is a device-wide setting. Use what this device is using.
	return audiosession.Hold(audiosession.Ambient)
}

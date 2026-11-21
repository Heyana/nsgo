//go:build linux && !android

package linux

import (
	"fmt"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/pcmoutput"
)

// ALSA PCM constants from <alsa/pcm.h>. Linux targets are little-endian
// (amd64 and arm64), where SND_PCM_FORMAT_FLOAT is SND_PCM_FORMAT_FLOAT_LE.
const (
	sndPCMStreamPlayback      = 0
	sndPCMNonblock            = 0x00000001
	sndPCMFormatFloatLE       = 14
	sndPCMAccessRWInterleaved = 3
	sndPCMStateSuspended      = 7
)

var alsa struct {
	once sync.Once
	ok   bool

	open       func(pcm *uintptr, name *byte, stream, mode int32) int32
	close      func(pcm uintptr) int32
	setParams  func(pcm uintptr, format, access int32, channels, rate uint32, softResample int32, latency uint32) int32
	getParams  func(pcm uintptr, bufferSize, periodSize *uint) int32
	prepare    func(pcm uintptr) int32
	drop       func(pcm uintptr) int32
	writei     func(pcm uintptr, buffer unsafe.Pointer, size uint) int
	avail      func(pcm uintptr) int
	delay      func(pcm uintptr, delay *int) int32
	wait       func(pcm uintptr, timeout int32) int32
	strerror   func(errnum int32) uintptr
	availDelay func(pcm uintptr, avail, delay *int) int32
	pause      func(pcm uintptr, enable int32) int32
	resume     func(pcm uintptr) int32
	state      func(pcm uintptr) int32
}

func loadALSA() bool {
	alsa.once.Do(func() {
		// RTLD_GLOBAL exposes libasound symbols to ALSA's own dynamically
		// loaded plugins such as PipeWire-ALSA and PulseAudio-ALSA.
		var lib uintptr
		for _, name := range []string{"libasound.so.2", "libasound.so"} {
			h, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err == nil {
				lib = h
				break
			}
		}
		if lib == 0 {
			return
		}
		register := func(fn any, sym string) (ok bool) {
			defer func() {
				if recover() != nil {
					ok = false
				}
			}()
			purego.RegisterLibFunc(fn, lib, sym)
			return true
		}
		required := register(&alsa.open, "snd_pcm_open") &&
			register(&alsa.close, "snd_pcm_close") &&
			register(&alsa.setParams, "snd_pcm_set_params") &&
			register(&alsa.getParams, "snd_pcm_get_params") &&
			register(&alsa.prepare, "snd_pcm_prepare") &&
			register(&alsa.drop, "snd_pcm_drop") &&
			register(&alsa.writei, "snd_pcm_writei") &&
			register(&alsa.avail, "snd_pcm_avail") &&
			register(&alsa.delay, "snd_pcm_delay") &&
			register(&alsa.wait, "snd_pcm_wait") &&
			register(&alsa.strerror, "snd_strerror")
		if !required {
			return
		}
		register(&alsa.availDelay, "snd_pcm_avail_delay")
		register(&alsa.pause, "snd_pcm_pause")
		register(&alsa.resume, "snd_pcm_resume")
		register(&alsa.state, "snd_pcm_state")
		alsa.ok = true
	})
	return alsa.ok
}

func (*window) OpenAudioOutput(c driver.AudioOutputConfig) (driver.AudioOutput, error) {
	return openALSAOutput(c)
}

func openALSAOutput(c driver.AudioOutputConfig) (driver.AudioOutput, error) {
	if !loadALSA() {
		return nil, driver.ErrAudioOutputUnsupported
	}
	var pcm uintptr
	if rc := alsa.open(&pcm, cstr("default"), sndPCMStreamPlayback, sndPCMNonblock); rc < 0 || pcm == 0 {
		return nil, fmt.Errorf("nsgo: audio: open ALSA output: %s", alsaErrorText(int(rc)))
	}
	var lastRC int32
	for _, latencyUs := range pcmoutput.LatencyCandidates(c.Latency) {
		lastRC = alsa.setParams(
			pcm,
			sndPCMFormatFloatLE,
			sndPCMAccessRWInterleaved,
			uint32(c.Channels),
			uint32(c.SampleRate),
			1, // allow ALSA software resampling
			latencyUs,
		)
		if lastRC == 0 {
			break
		}
	}
	if lastRC < 0 {
		alsa.close(pcm)
		return nil, fmt.Errorf("nsgo: audio: configure ALSA output: %s", alsaErrorText(int(lastRC)))
	}
	var bufferFrames, periodFrames uint
	if alsa.getParams(pcm, &bufferFrames, &periodFrames) < 0 {
		bufferFrames, periodFrames = 0, 0
	}
	dev := &alsaDevice{
		pcm:          pcm,
		channels:     c.Channels,
		bufferFrames: int(bufferFrames),
		periodFrames: int(periodFrames),
	}
	return pcmoutput.New(dev, c), nil
}

type alsaDevice struct {
	pcm          uintptr
	channels     int
	bufferFrames int
	periodFrames int
}

func (d *alsaDevice) BufferFrames() int { return d.bufferFrames }
func (d *alsaDevice) PeriodFrames() int { return d.periodFrames }

func (d *alsaDevice) Prepare() error {
	if d.pcm == 0 {
		return pcmoutput.ErrDisconnected
	}
	if alsa.state != nil && alsa.resume != nil && alsa.state(d.pcm) == sndPCMStateSuspended {
		// snd_pcm_resume may return -ENOSYS when hardware cannot resume
		// directly; ALSA documents falling through to snd_pcm_prepare.
		_ = alsa.resume(d.pcm)
	}
	if rc := alsa.prepare(d.pcm); rc < 0 {
		return classifyALSAError("prepare ALSA output", int(rc))
	}
	return nil
}

func (d *alsaDevice) AvailDelay() (int, int64, bool, error) {
	if d.pcm == 0 {
		return 0, 0, false, pcmoutput.ErrDisconnected
	}
	if alsa.availDelay != nil {
		var avail, delay int
		if rc := alsa.availDelay(d.pcm, &avail, &delay); rc < 0 {
			return 0, 0, false, classifyALSAError("query ALSA output", int(rc))
		}
		return avail, int64(delay), true, nil
	}
	avail := alsa.avail(d.pcm)
	if avail < 0 {
		return 0, 0, false, classifyALSAError("query ALSA available frames", avail)
	}
	var delay int
	if rc := alsa.delay(d.pcm, &delay); rc < 0 {
		return avail, 0, false, nil
	}
	return avail, int64(delay), true, nil
}

func (d *alsaDevice) Wait(timeout time.Duration) (bool, error) {
	if d.pcm == 0 {
		return false, pcmoutput.ErrDisconnected
	}
	ms := int32(timeout / time.Millisecond)
	if ms < 1 {
		ms = 1
	}
	rc := alsa.wait(d.pcm, ms)
	if rc > 0 {
		return true, nil
	}
	if rc == 0 {
		return false, nil
	}
	return false, classifyALSAError("wait for ALSA output", int(rc))
}

func (d *alsaDevice) Write(samples []float32) (int, error) {
	if d.pcm == 0 {
		return 0, pcmoutput.ErrDisconnected
	}
	frames := len(samples) / d.channels
	if frames <= 0 {
		return 0, nil
	}
	n := alsa.writei(d.pcm, unsafe.Pointer(&samples[0]), uint(frames))
	if n >= 0 {
		return n, nil
	}
	return 0, classifyALSAError("write ALSA frames", n)
}

func (d *alsaDevice) Drop() {
	if d.pcm == 0 {
		return
	}
	alsa.drop(d.pcm)
}

func (d *alsaDevice) Close() {
	if d.pcm == 0 {
		return
	}
	alsa.close(d.pcm)
	d.pcm = 0
}

func classifyALSAError(op string, code int) error {
	errno := syscall.Errno(-code)
	switch errno {
	case syscall.EAGAIN, syscall.EINTR:
		return pcmoutput.ErrAgain
	case syscall.EPIPE:
		return pcmoutput.ErrXRun
	case syscall.ESTRPIPE:
		return fmt.Errorf("%w: %s", pcmoutput.ErrSuspended, alsaErrorText(code))
	case syscall.ENODEV, syscall.ENXIO, syscall.EIO, syscall.EBADFD:
		return fmt.Errorf("%w: %s", pcmoutput.ErrDisconnected, alsaErrorText(code))
	default:
		return fmt.Errorf("nsgo: audio: %s: %s", op, alsaErrorText(code))
	}
}

func alsaErrorText(code int) string {
	if alsa.strerror != nil {
		if s := goString(alsa.strerror(int32(code))); s != "" {
			return s
		}
	}
	return fmt.Sprintf("ALSA error %d", code)
}

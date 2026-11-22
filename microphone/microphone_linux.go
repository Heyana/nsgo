//go:build linux && !android

package microphone

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"runtime"
	"structs"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
)

const (
	paStreamRecord = 2
	paSampleS16LE  = 3

	sndPCMStreamCapture       = 1
	sndPCMFormatS16LE         = 2
	sndPCMAccessRWInterleaved = 3
)

// paSampleSpec matches C's struct pa_sample_spec (12 bytes on 64-bit Linux).
type paSampleSpec struct {
	_        structs.HostLayout
	format   int32
	rate     uint32
	channels uint8
	_        [3]byte
}

// paBufferAttr matches C's struct pa_buffer_attr (20 bytes).
type paBufferAttr struct {
	_         structs.HostLayout
	maxlength uint32
	tlength   uint32
	prebuf    uint32
	minreq    uint32
	fragsize  uint32
}

var pulseLib struct {
	once   sync.Once
	err    error
	newFn  func(server, name *byte, dir int32, dev, streamName *byte, ss *paSampleSpec, channelMap uintptr, attr *paBufferAttr, errCode *int32) uintptr
	readFn func(s uintptr, data unsafe.Pointer, bytes uintptr, errCode *int32) int32
	freeFn func(s uintptr)
}

func loadPulse() error {
	pulseLib.once.Do(func() {
		lib, err := purego.Dlopen("libpulse-simple.so.0", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			pulseLib.err = err
			return
		}
		purego.RegisterLibFunc(&pulseLib.newFn, lib, "pa_simple_new")
		purego.RegisterLibFunc(&pulseLib.readFn, lib, "pa_simple_read")
		purego.RegisterLibFunc(&pulseLib.freeFn, lib, "pa_simple_free")
	})
	return pulseLib.err
}

var alsaLib struct {
	once      sync.Once
	err       error
	openFn    func(pcmp *uintptr, name *byte, stream, mode int32) int32
	setParams func(pcm uintptr, format, access int32, channels, rate uint32, softResample int32, latency uint32) int32
	prepareFn func(pcm uintptr) int32
	readiFn   func(pcm uintptr, buf unsafe.Pointer, frames uint64) int64
	recoverFn func(pcm uintptr, errCode, silent int32) int32
	closeFn   func(pcm uintptr) int32
}

func loadALSA() error {
	alsaLib.once.Do(func() {
		lib, err := purego.Dlopen("libasound.so.2", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			alsaLib.err = err
			return
		}
		purego.RegisterLibFunc(&alsaLib.openFn, lib, "snd_pcm_open")
		purego.RegisterLibFunc(&alsaLib.setParams, lib, "snd_pcm_set_params")
		purego.RegisterLibFunc(&alsaLib.prepareFn, lib, "snd_pcm_prepare")
		purego.RegisterLibFunc(&alsaLib.readiFn, lib, "snd_pcm_readi")
		purego.RegisterLibFunc(&alsaLib.recoverFn, lib, "snd_pcm_recover")
		purego.RegisterLibFunc(&alsaLib.closeFn, lib, "snd_pcm_close")
	})
	return alsaLib.err
}

// Supported says this target can record (libpulse-simple or ALSA is present).
// While nsgotest's fake microphone is connected, it and Allowed and Authorize
// answer from the fake.
func Supported() bool {
	if reachable() {
		return recorderSupported()
	}
	return loadPulse() == nil || loadALSA() == nil
}

// Allowed says this process may record without asking.
func Allowed() bool {
	if reachable() {
		return recorderAllowed()
	}
	return loadPulse() == nil || loadALSA() == nil
}

// Authorize returns nil when an audio capture library is present on Linux, or
// ErrUnavailable if neither PulseAudio nor ALSA can be loaded.
func Authorize() error {
	if reachable() {
		return recorderAuthorize()
	}
	return authorizeSystem()
}

func authorizeSystem() error {
	if loadPulse() == nil || loadALSA() == nil {
		return nil
	}
	return fmt.Errorf("%w: neither libpulse-simple.so.0 nor libasound.so.2 is available", ErrUnavailable)
}

// Capture records 16-bit mono PCM at rate hertz via PulseAudio (or PipeWire's
// PulseAudio compatibility layer) with an ALSA fallback, passing samples to
// the callback until the Stream stops or the input stream ends.
func Capture(rate int, samples func([]int16)) (*Stream, error) {
	if rate <= 0 {
		return nil, errors.New("microphone: sample rate must be positive")
	}
	if err := authorizeSystem(); err != nil {
		return nil, err
	}
	ended := make(chan struct{})
	end := sync.OnceFunc(func() { close(ended) })
	sess, err := openLinuxCapture(rate, 1, false, samples, func(bool) { end() }, func(error) { end() })
	if err != nil {
		return nil, err
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			sess.stop()
			end()
		})
	}
	return &Stream{stop: stop, ended: ended}, nil
}

func openWAVCapture(rate, channels int, onSamples func([]int16), onInterrupt func(bool), onError func(error)) (*captureSession, error) {
	return openLinuxCapture(rate, channels, true, onSamples, onInterrupt, onError)
}

func openLinuxCapture(rate, channels int, allowFallback bool, onSamples func([]int16), onInterrupt func(bool), onError func(error)) (*captureSession, error) {
	channelList := []int{max(channels, 1)}
	if allowFallback && channels == 2 {
		channelList = []int{2, 1}
	}
	rateList := []int{rate}
	if rate <= 0 {
		rateList = []int{48000, 44100, 16000}
	} else if allowFallback {
		rateList = []int{rate, 48000, 44100, 16000}
	}

	var lastErr error
	if loadPulse() == nil {
		for _, ch := range channelList {
			for _, r := range rateList {
				if sess, err := openPulseCapture(r, ch, onSamples, onInterrupt, onError); err == nil {
					return sess, nil
				} else {
					lastErr = err
				}
			}
		}
	}
	if loadALSA() == nil {
		for _, ch := range channelList {
			for _, r := range rateList {
				if sess, err := openALSACapture(r, ch, onSamples, onInterrupt, onError); err == nil {
					return sess, nil
				} else {
					lastErr = err
				}
			}
		}
	}
	if lastErr == nil {
		lastErr = ErrUnavailable
	}
	return nil, lastErr
}

func openPulseCapture(rate, channels int, onSamples func([]int16), _ func(bool), onError func(error)) (*captureSession, error) {
	appName := []byte("nsgo\x00")
	streamName := []byte("microphone\x00")
	ss := paSampleSpec{
		format:   paSampleS16LE,
		rate:     uint32(rate),
		channels: uint8(channels),
	}
	chunkFrames := max(256, rate/50)
	chunkBytes := chunkFrames * channels * 2
	attr := paBufferAttr{
		maxlength: ^uint32(0),
		tlength:   ^uint32(0),
		prebuf:    ^uint32(0),
		minreq:    ^uint32(0),
		fragsize:  uint32(chunkBytes),
	}
	var paErr int32
	handle := pulseLib.newFn(nil, &appName[0], paStreamRecord, nil, &streamName[0], &ss, 0, &attr, &paErr)
	runtime.KeepAlive(appName)
	runtime.KeepAlive(streamName)
	if handle == 0 {
		return nil, fmt.Errorf("%w: pa_simple_new failed (code %d)", ErrUnavailable, paErr)
	}

	var stopped atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer pulseLib.freeFn(handle)
		raw := make([]byte, chunkBytes)
		for !stopped.Load() {
			var readErr int32
			if pulseLib.readFn(handle, unsafe.Pointer(&raw[0]), uintptr(len(raw)), &readErr) < 0 {
				if !stopped.Load() && onError != nil {
					onError(fmt.Errorf("microphone: pa_simple_read failed (code %d)", readErr))
				}
				return
			}
			if stopped.Load() {
				return
			}
			out := bytesToInt16(raw)
			onSamples(out)
		}
	}()

	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			stopped.Store(true)
			<-done
		})
	}
	return &captureSession{rate: rate, channels: channels, stop: stop}, nil
}

func openALSACapture(rate, channels int, onSamples func([]int16), _ func(bool), onError func(error)) (*captureSession, error) {
	devName := []byte("default\x00")
	var pcm uintptr
	if rc := alsaLib.openFn(&pcm, &devName[0], sndPCMStreamCapture, 0); rc < 0 || pcm == 0 {
		runtime.KeepAlive(devName)
		return nil, fmt.Errorf("%w: snd_pcm_open failed (%d)", ErrUnavailable, rc)
	}
	runtime.KeepAlive(devName)

	if rc := alsaLib.setParams(pcm, sndPCMFormatS16LE, sndPCMAccessRWInterleaved, uint32(channels), uint32(rate), 1, 40000); rc < 0 {
		alsaLib.closeFn(pcm)
		return nil, fmt.Errorf("%w: snd_pcm_set_params failed (%d)", ErrUnavailable, rc)
	}
	if rc := alsaLib.prepareFn(pcm); rc < 0 {
		alsaLib.closeFn(pcm)
		return nil, fmt.Errorf("%w: snd_pcm_prepare failed (%d)", ErrUnavailable, rc)
	}

	chunkFrames := max(256, rate/50)
	frameBytes := channels * 2
	var stopped atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer alsaLib.closeFn(pcm)
		raw := make([]byte, chunkFrames*frameBytes)
		for !stopped.Load() {
			n := alsaLib.readiFn(pcm, unsafe.Pointer(&raw[0]), uint64(chunkFrames))
			if n < 0 {
				if stopped.Load() {
					return
				}
				if alsaLib.recoverFn(pcm, int32(n), 1) < 0 {
					if onError != nil {
						onError(fmt.Errorf("microphone: snd_pcm_readi failed (%d)", n))
					}
					return
				}
				continue
			}
			if n == 0 || stopped.Load() {
				continue
			}
			out := bytesToInt16(raw[:int(n)*frameBytes])
			onSamples(out)
		}
	}()

	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			stopped.Store(true)
			<-done
		})
	}
	return &captureSession{rate: rate, channels: channels, stop: stop}, nil
}

func bytesToInt16(raw []byte) []int16 {
	n := len(raw) / 2
	out := make([]int16, n)
	for i := range n {
		out[i] = int16(binary.LittleEndian.Uint16(raw[2*i:]))
	}
	return out
}

// linuxVolumeStats matches the start of struct statvfs on 64-bit Linux ABIs.
type linuxVolumeStats struct {
	blockSize, fragmentSize, blocks, free, available uint64
	_                                                [9]uint64
}

var linuxStatvfs = sync.OnceValue(func() func(string, *linuxVolumeStats) int32 {
	sym, err := purego.Dlsym(purego.RTLD_DEFAULT, "statvfs")
	if err != nil {
		return nil
	}
	var fn func(string, *linuxVolumeStats) int32
	purego.RegisterFunc(&fn, sym)
	return fn
})

func availableDiskSpace(dir string) int64 {
	stat := linuxStatvfs()
	if dir == "" || stat == nil {
		return -1
	}
	var st linuxVolumeStats
	if stat(dir, &st) != 0 {
		return -1
	}
	if st.fragmentSize != 0 && st.available > math.MaxInt64/st.fragmentSize {
		return math.MaxInt64
	}
	return int64(st.available * st.fragmentSize)
}

func isNoSpaceError(err error) bool {
	return errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT)
}

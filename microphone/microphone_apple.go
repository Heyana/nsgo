//go:build darwin

package microphone

import (
	"errors"
	"fmt"
	"structs"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/privacy"
)

const (
	bufferSeconds = 0.1
	bufferCount   = 3

	kAudioFormatLinearPCM     = 0x6C70636D // 'lpcm'
	kLinearPCMFormatSignedInt = 1 << 2
	kLinearPCMFormatPacked    = 1 << 3
)

// streamDescription is AudioStreamBasicDescription.
type streamDescription struct {
	_                structs.HostLayout
	sampleRate       float64
	formatID         uint32
	formatFlags      uint32
	bytesPerPacket   uint32
	framesPerPacket  uint32
	bytesPerFrame    uint32
	channelsPerFrame uint32
	bitsPerChannel   uint32
	reserved         uint32
}

// queueBuffer is the start of AudioQueueBuffer.
type queueBuffer struct {
	_        structs.HostLayout
	capacity uint32
	data     unsafe.Pointer
	size     uint32
}

var system struct {
	once       sync.Once
	err        error
	newInput   func(format *streamDescription, callback, user, runLoop, mode uintptr, flags uint32, queue *uintptr) int32
	allocate   func(queue uintptr, size uint32, buffer **queueBuffer) int32
	enqueue    func(queue uintptr, buffer *queueBuffer, packets uint32, descriptions uintptr) int32
	start      func(queue, startTime uintptr) int32
	stop       func(queue uintptr, immediate bool) int32
	dispose    func(queue uintptr, immediate bool) int32
	callback   uintptr
	nextID     atomic.Uintptr
	recordings sync.Map // of *recording, by id
}

type recording struct {
	samples func([]int16)
	stopped atomic.Bool
}

func load() error {
	system.once.Do(func() {
		toolbox, err := purego.Dlopen("/System/Library/Frameworks/AudioToolbox.framework/AudioToolbox", purego.RTLD_NOW)
		if err != nil {
			system.err = err
			return
		}
		foundation, err := purego.Dlopen("/System/Library/Frameworks/AVFoundation.framework/AVFoundation", purego.RTLD_NOW)
		if err != nil {
			system.err = err
			return
		}
		purego.RegisterLibFunc(&system.newInput, toolbox, "AudioQueueNewInput")
		purego.RegisterLibFunc(&system.allocate, toolbox, "AudioQueueAllocateBuffer")
		purego.RegisterLibFunc(&system.enqueue, toolbox, "AudioQueueEnqueueBuffer")
		purego.RegisterLibFunc(&system.start, toolbox, "AudioQueueStart")
		purego.RegisterLibFunc(&system.stop, toolbox, "AudioQueueStop")
		purego.RegisterLibFunc(&system.dispose, toolbox, "AudioQueueDispose")
		if _, err := purego.Dlsym(foundation, "AVMediaTypeAudio"); err != nil {
			system.err = err
			return
		}
		// One callback serves every recording, as callbacks are never freed.
		system.callback = purego.NewCallback(func(user, queue uintptr, buffer *queueBuffer, _ uintptr, _ uint32, _ uintptr) {
			v, ok := system.recordings.Load(user)
			if !ok {
				return
			}
			r := v.(*recording)
			if r.stopped.Load() {
				return
			}
			if buffer.size > 0 {
				r.samples(append([]int16(nil), unsafe.Slice((*int16)(buffer.data), buffer.size/2)...))
			}
			system.enqueue(queue, buffer, 0, 0)
		})
	})
	return system.err
}

// Supported says this target can record. While nsgotest's fake microphone is connected, it and Allowed and Authorize answer from the fake.
func Supported() bool {
	if reachable() {
		return recorderSupported()
	}
	return load() == nil
}

// Allowed says the system lets this process record, without asking. It reads the same status as core.Scope.PermissionStatus of core.PermissionMicrophone, which apps with an engine use; this serves recording without one.
func Allowed() bool {
	if reachable() {
		return recorderAllowed()
	}
	return load() == nil && privacy.Status(driver.PermissionMicrophone) == driver.PermissionGranted
}

// Authorize asks the user the first time, waiting for the answer, so call it off the UI. It answers nil once this process may record, and ErrDenied when it may not. It asks as core.Scope.RequestPermission of core.PermissionMicrophone does, which apps with an engine use; this serves recording without one.
func Authorize() error {
	if reachable() {
		return recorderAuthorize()
	}
	return authorizeSystem()
}

// authorizeSystem is Authorize of the system's microphone, which Capture records.
func authorizeSystem() error {
	if err := load(); err != nil {
		return fmt.Errorf("microphone: %w", err)
	}
	switch privacy.Status(driver.PermissionMicrophone) {
	case driver.PermissionGranted:
		return nil
	case driver.PermissionNotAsked:
	default:
		return ErrDenied
	}
	if err := checkUsageDescription(); err != nil {
		return err
	}
	answer := make(chan int, 1)
	privacy.Request(driver.PermissionMicrophone, func(status int) { answer <- status })
	if <-answer != driver.PermissionGranted {
		return ErrDenied
	}
	return nil
}

// Capture records 16-bit mono at rate hertz, passing samples to the
// callback, on a thread of the system's, until the Stream stops or the
// system takes the microphone away, which Ended tells. It may wait for the user to answer the permission prompt, so call it off
// the UI.
func Capture(rate int, samples func([]int16)) (*Stream, error) {
	if rate <= 0 {
		return nil, errors.New("microphone: sample rate must be positive")
	}
	if err := authorizeSystem(); err != nil {
		return nil, err
	}
	ended := make(chan struct{})
	end := sync.OnceFunc(func() { close(ended) })
	stopQueue, _, err := openAudioQueue(rate, 1, bufferSeconds, bufferCount, samples, func(silenced bool) {
		if silenced {
			end()
		}
	})
	if err != nil {
		return nil, err
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			stopQueue()
			end()
		})
	}
	return &Stream{stop, ended}, nil
}

func openAudioQueue(rate, channels int, bufSec float64, bufCount int, samples func([]int16), onInterrupt func(bool)) (stop func(), resume func(), err error) {
	frameBytes := uint32(channels * 2)
	format := streamDescription{
		sampleRate:       float64(rate),
		formatID:         kAudioFormatLinearPCM,
		formatFlags:      kLinearPCMFormatSignedInt | kLinearPCMFormatPacked,
		bytesPerPacket:   frameBytes,
		framesPerPacket:  1,
		bytesPerFrame:    frameBytes,
		channelsPerFrame: uint32(channels),
		bitsPerChannel:   16,
	}
	leave, err := enterSession(onInterrupt)
	if err != nil {
		return nil, nil, err
	}
	r := &recording{samples: samples}
	id := system.nextID.Add(1)
	system.recordings.Store(id, r)
	var queue uintptr
	if status := system.newInput(&format, system.callback, id, 0, 0, 0, &queue); status != 0 {
		system.recordings.Delete(id)
		leave()
		return nil, nil, fmt.Errorf("microphone: AudioQueueNewInput: %d", status)
	}
	var stopOnce sync.Once
	stop = func() {
		stopOnce.Do(func() {
			r.stopped.Store(true)
			system.stop(queue, true)
			system.dispose(queue, true)
			system.recordings.Delete(id)
			leave()
		})
	}
	frames := max(256, int(float64(rate)*bufSec))
	size := uint32(frames) * frameBytes
	for range bufCount {
		var buffer *queueBuffer
		if status := system.allocate(queue, size, &buffer); status != 0 {
			stop()
			return nil, nil, fmt.Errorf("microphone: AudioQueueAllocateBuffer: %d", status)
		}
		system.enqueue(queue, buffer, 0, 0)
	}
	if status := system.start(queue, 0); status != 0 {
		stop()
		return nil, nil, fmt.Errorf("microphone: AudioQueueStart: %d", status)
	}
	resume = func() {
		if !r.stopped.Load() {
			system.start(queue, 0)
		}
	}
	return stop, resume, nil
}

func openWAVCapture(rate, channels int, onSamples func([]int16), onInterrupt func(bool), _ func(error)) (*captureSession, error) {
	channelList := []int{1}
	if channels == 2 {
		channelList = []int{2, 1}
	}
	rateList := []int{48000, 44100, 16000}
	if rate > 0 {
		rateList = []int{rate, 48000, 44100, 16000}
	}
	var lastErr error
	for _, ch := range channelList {
		for _, r := range rateList {
			stop, resume, err := openAudioQueue(r, ch, 0.02, 4, onSamples, onInterrupt)
			if err == nil {
				return &captureSession{rate: r, channels: ch, stop: stop, resume: resume}, nil
			}
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = ErrUnavailable
	}
	return nil, lastErr
}

// availableDiskSpace returns -1 on Apple targets so recording does not call
// statfs or NSURLVolumeAvailableCapacity* without a privacy-manifest reason;
// full disks are still reported as ErrNoSpace when a write returns ENOSPC.
func availableDiskSpace(string) int64 { return -1 }

func isNoSpaceError(err error) bool {
	return errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT)
}

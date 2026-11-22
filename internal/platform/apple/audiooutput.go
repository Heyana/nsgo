//go:build darwin

package apple

import (
	"errors"
	"fmt"
	"runtime"
	"structs"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	auOutput            = 0x61756f75 // 'auou'
	auDefaultOutput     = 0x64656620 // 'def '
	auRemoteIO          = 0x72696f63 // 'rioc'
	auApple             = 0x6170706c // 'appl'
	auLinearPCM         = 0x6c70636d // 'lpcm'
	auFloatPacked       = 1 | 8
	auScopeGlobal       = 0
	auScopeInput        = 1
	auStreamFormat      = 8
	auSetRenderCallback = 23
	auHostTimeValid     = 1 << 1
)

type componentDescription struct {
	_                                        structs.HostLayout
	kind, subtype, manufacturer, flags, mask uint32
}

type pcmFormat struct {
	_                                        structs.HostLayout
	rate                                     float64
	format, flags, bytesPacket, framesPacket uint32
	bytesFrame, channels, bits, reserved     uint32
}

type pcmRenderCallback struct {
	_        structs.HostLayout
	fun, ctx uintptr
}

// Only the prefix through flags is used. SMPTETime occupies 24 bytes.
type pcmTimestamp struct {
	_          structs.HostLayout
	sampleTime float64
	hostTime   uint64
	rateScalar float64
	wordClock  uint64
	smpte      [24]byte
	flags      uint32
}

// The AudioBuffer is 8-byte aligned in AudioBufferList on Apple targets.
type audioBufferList struct {
	_      structs.HostLayout
	count  uint32
	_      uint32
	buffer struct {
		channels, bytes uint32
		data            *float32
	}
}

var pcmUnit struct {
	find    func(uintptr, *componentDescription) uintptr
	create  func(uintptr, *uintptr) int32
	dispose func(uintptr) int32
	set     func(uintptr, uint32, uint32, uint32, unsafe.Pointer, uint32) int32
	init    func(uintptr) int32
	reset   func(uintptr, uint32, uint32) int32
	start   func(uintptr) int32
	stop    func(uintptr) int32
	now     func() uint64
	numer   uint32
	denom   uint32
	render  uintptr
}

var loadPCMUnit = sync.OnceValue(func() bool {
	lib, err := purego.Dlopen("/System/Library/Frameworks/AudioToolbox.framework/AudioToolbox", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return false
	}
	purego.RegisterLibFunc(&pcmUnit.find, lib, "AudioComponentFindNext")
	purego.RegisterLibFunc(&pcmUnit.create, lib, "AudioComponentInstanceNew")
	purego.RegisterLibFunc(&pcmUnit.dispose, lib, "AudioComponentInstanceDispose")
	purego.RegisterLibFunc(&pcmUnit.set, lib, "AudioUnitSetProperty")
	purego.RegisterLibFunc(&pcmUnit.init, lib, "AudioUnitInitialize")
	purego.RegisterLibFunc(&pcmUnit.reset, lib, "AudioUnitReset")
	purego.RegisterLibFunc(&pcmUnit.start, lib, "AudioOutputUnitStart")
	purego.RegisterLibFunc(&pcmUnit.stop, lib, "AudioOutputUnitStop")
	system := load("/usr/lib/libSystem.B.dylib")
	purego.RegisterLibFunc(&pcmUnit.now, system, "mach_absolute_time")
	var timebase func(*struct{ numer, denom uint32 }) int32
	purego.RegisterLibFunc(&timebase, system, "mach_timebase_info")
	var info struct{ numer, denom uint32 }
	if timebase(&info) != 0 || info.denom == 0 {
		return false
	}
	pcmUnit.numer, pcmUnit.denom = info.numer, info.denom
	pcmUnit.render = purego.NewCallback(renderPCM)
	return true
})

var (
	pcmStreams sync.Map
	pcmID      atomic.Uint64
)

// audioOutput controls one output unit. The control lock is never taken by its render callback. Only that callback writes scheduled buffers.
type audioOutput struct {
	id      uintptr
	unit    uintptr
	conf    driver.AudioOutputConfig
	mu      sync.Mutex
	closed  bool
	active  bool
	letGo   func()
	unwatch func()

	timeline     pcmTimeline
	base, frames int64
	at           time.Time
	latency      atomic.Int64
	underruns    atomic.Uint64
}

// OpenAudioOutput opens a unit without activating the device or holding the process's audio session.
func OpenAudioOutput(conf driver.AudioOutputConfig) (driver.AudioOutput, error) {
	if !loadPCMUnit() {
		return nil, driver.ErrAudioOutputUnsupported
	}
	kind := uint32(auDefaultOutput)
	if runtime.GOOS == "ios" {
		kind = auRemoteIO
	}
	component := pcmUnit.find(0, &componentDescription{kind: auOutput, subtype: kind, manufacturer: auApple})
	if component == 0 {
		return nil, driver.ErrAudioOutputUnsupported
	}
	o := &audioOutput{id: uintptr(pcmID.Add(1)), conf: conf}
	if err := pcmStatus("open output", pcmUnit.create(component, &o.unit)); err != nil {
		return nil, err
	}
	pcmStreams.Store(o.id, o)
	if err := o.configure(); err != nil {
		o.Close()
		return nil, err
	}
	o.unwatch = o.watchRoute()
	return o, nil
}

func (o *audioOutput) configure() error {
	stride := uint32(o.conf.Channels * 4)
	format := pcmFormat{
		rate: float64(o.conf.SampleRate), channels: uint32(o.conf.Channels), format: auLinearPCM,
		flags: auFloatPacked, bytesPacket: stride, framesPacket: 1, bytesFrame: stride, bits: 32,
	}
	status := pcmUnit.set(o.unit, auStreamFormat, auScopeInput, 0, unsafe.Pointer(&format), uint32(unsafe.Sizeof(format)))
	if err := pcmStatus("set PCM format", status); err != nil {
		return err
	}
	cb := pcmRenderCallback{fun: pcmUnit.render, ctx: o.id}
	status = pcmUnit.set(o.unit, auSetRenderCallback, auScopeInput, 0, unsafe.Pointer(&cb), uint32(unsafe.Sizeof(cb)))
	if err := pcmStatus("set PCM render callback", status); err != nil {
		return err
	}
	return pcmStatus("initialize PCM output", pcmUnit.init(o.unit))
}

func pcmStatus(op string, code int32) error {
	if code == 0 {
		return nil
	}
	return fmt.Errorf("nsgo: audio: %s: OSStatus %d", op, code)
}

func hostNanos(ticks uint64) int64 {
	return int64(ticks * uint64(pcmUnit.numer) / uint64(pcmUnit.denom))
}

func renderPCM(id uintptr, _ *uint32, stamp *pcmTimestamp, _, frames uint32, output *audioBufferList) int32 {
	stream, ok := pcmStreams.Load(id)
	if !ok || output == nil || output.count != 1 || output.buffer.data == nil {
		return -50
	}
	o := stream.(*audioOutput)
	samples := int(frames) * o.conf.Channels
	if samples > int(output.buffer.bytes)/4 {
		return -50
	}
	buf := unsafe.Slice(output.buffer.data, samples)
	produced := min(max(o.conf.Render(buf), 0), int(frames))
	if produced < int(frames) {
		clear(buf[produced*o.conf.Channels:])
		o.underruns.Add(1)
	}
	now := hostNanos(pcmUnit.now())
	at := now
	if stamp != nil && stamp.flags&auHostTimeValid != 0 && stamp.hostTime != 0 {
		at = hostNanos(stamp.hostTime)
	}
	o.timeline.push(at, int64(frames), int64(produced))
	buffer := pcmDuration(int64(frames), o.conf.SampleRate)
	o.latency.Store(int64(buffer) + max(at-now, 0))
	return 0
}

func (o *audioOutput) Start() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.start()
}

func (o *audioOutput) start() error {
	if o.closed {
		return errors.New("nsgo: audio: PCM output is closed")
	}
	if o.active {
		return nil
	}
	if o.letGo == nil {
		letGo, err := o.holdSession()
		if err != nil {
			return err
		}
		o.letGo = letGo
	}
	if err := pcmStatus("start PCM output", pcmUnit.start(o.unit)); err != nil {
		o.releaseSession()
		return err
	}
	o.active = true
	return nil
}

func (o *audioOutput) Pause() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stop()
	o.releaseSession()
}

// stop freezes the position before resetting the unit's queued frames.
func (o *audioOutput) stop() int {
	if !o.active {
		return 0
	}
	now, at := hostNanos(pcmUnit.now()), time.Now()
	pcmUnit.stop(o.unit)
	dropped := o.advance(now, at)
	pcmUnit.reset(o.unit, auScopeGlobal, 0)
	o.timeline.reset()
	o.base, o.active = o.frames, false
	return dropped
}

func (o *audioOutput) releaseSession() {
	if o.letGo != nil {
		o.letGo()
		o.letGo = nil
	}
}

func (o *audioOutput) Flush() int {
	o.mu.Lock()
	wasRunning := o.active
	dropped := o.stop()
	var err error
	if wasRunning {
		err = o.start()
	}
	o.mu.Unlock()
	if err != nil && o.conf.Interrupt != nil {
		o.conf.Interrupt(err)
	}
	return dropped
}

func (o *audioOutput) Close() {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return
	}
	o.closed = true
	o.stop()
	o.releaseSession()
	pcmUnit.dispose(o.unit)
	pcmStreams.Delete(o.id)
	unwatch := o.unwatch
	o.mu.Unlock()
	if unwatch != nil {
		unwatch()
	}
}

func (o *audioOutput) interrupt(err error) {
	o.mu.Lock()
	wasRunning := o.active
	o.stop()
	o.releaseSession()
	o.mu.Unlock()
	if wasRunning && o.conf.Interrupt != nil {
		o.conf.Interrupt(err)
	}
}

func (o *audioOutput) Position() (int64, time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.active {
		o.advance(hostNanos(pcmUnit.now()), time.Now())
	}
	return o.frames, o.at
}

func (o *audioOutput) advance(now int64, at time.Time) int {
	frames, heard, dropped := o.timeline.position(now, o.conf.SampleRate)
	if total := o.base + frames; total > o.frames {
		o.frames = total
		if when := at.Add(time.Duration(heard - now)); when.After(o.at) {
			o.at = when
		}
	}
	return dropped
}

func (o *audioOutput) Latency() time.Duration { return time.Duration(o.latency.Load()) }
func (o *audioOutput) Underruns() int         { return int(o.underruns.Load()) }

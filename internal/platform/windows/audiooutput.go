//go:build windows

package windows

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Shared-mode, event-driven WASAPI pull PCM output over syscall COM vtables.
//
// Each open stream owns one OS-thread-locked goroutine initialized in the multithreaded COM apartment (COINIT_MULTITHREADED). That goroutine owns all WASAPI COM interfaces (IMMDeviceEnumerator, IMMDevice, IAudioClient, IAudioRenderClient, IAudioClock), drives the WaitForMultipleObjects event loop, invokes the client's Render callback, samples IAudioClock + QPC for monotonic Position, and handles Start, Pause, Flush, Close and device-switch notifications without cross-thread COM calls or lock contention on the audio thread.

var (
	wasapiCoUninitialize            = ole32.NewProc("CoUninitialize")
	wasapiCreateEventW              = kernel32.NewProc("CreateEventW")
	wasapiSetEvent                  = kernel32.NewProc("SetEvent")
	wasapiResetEvent                = kernel32.NewProc("ResetEvent")
	wasapiWaitForMultipleObjects    = kernel32.NewProc("WaitForMultipleObjects")
	wasapiQueryPerformanceCounter   = kernel32.NewProc("QueryPerformanceCounter")
	wasapiQueryPerformanceFrequency = kernel32.NewProc("QueryPerformanceFrequency")

	avrtDLL                         = syscall.NewLazyDLL("avrt.dll")
	avSetMmThreadCharacteristicsW   = avrtDLL.NewProc("AvSetMmThreadCharacteristicsW")
	avRevertMmThreadCharacteristics = avrtDLL.NewProc("AvRevertMmThreadCharacteristics")
)

var (
	clsidMMDeviceEnumerator = parseGUID("bcde0395-e52f-467c-8e3d-c4579291692e")
	iidMMDeviceEnumerator   = parseGUID("a95664d2-9614-4f35-a746-de8db63617e6")
	iidMMNotificationClient = parseGUID("7991eec9-7e89-4d85-8390-6c703cec60c0")
	iidAudioClient          = parseGUID("1cb9ad4c-dbfa-4c32-b178-c2f568a703b2")
	iidAudioRenderClient    = parseGUID("f294acfc-3146-4483-a7bf-addca7c260e2")
	iidAudioClock           = parseGUID("cd63314f-3fba-4a1b-812c-ef96358728e7")
)

// WASAPI COM vtable indexes (mmdeviceapi.h and audioclient.h).
const (
	// IMMDeviceEnumerator
	mmEnumGetDefaultAudioEndpoint                = 4
	mmEnumRegisterEndpointNotificationCallback   = 6
	mmEnumUnregisterEndpointNotificationCallback = 7

	// IMMDevice
	mmDeviceActivate = 3
	mmDeviceGetID    = 5

	// IAudioClient
	acInitialize        = 3
	acGetBufferSize     = 4
	acGetStreamLatency  = 5
	acGetCurrentPadding = 6
	acStart             = 10
	acStop              = 11
	acReset             = 12
	acSetEventHandle    = 13
	acGetService        = 14

	// IAudioRenderClient
	arcGetBuffer     = 3
	arcReleaseBuffer = 4

	// IAudioClock
	clkGetFrequency = 3
	clkGetPosition  = 4
)

const (
	coinitMultithreaded = 0
	sFalse              = 1
	clsctxAll           = 0x17

	mmFlowRender        = 0 // eRender
	mmRoleConsole       = 0 // eConsole
	mmDeviceStateActive = 0x00000001

	audclntShareModeShared              = 0
	audclntStreamFlagsEventCallback     = 0x00040000
	audclntStreamFlagsSrcDefaultQuality = 0x08000000
	audclntStreamFlagsAutoConvertPCM    = 0x80000000
	audclntStreamFlagsPullPCM           = audclntStreamFlagsEventCallback | audclntStreamFlagsAutoConvertPCM | audclntStreamFlagsSrcDefaultQuality

	waitObject0  = 0x00000000
	waitTimeout  = 0x00000102
	waitInfinite = 0xFFFFFFFF
)

var (
	errWASAPIClosed        = errors.New("nsgo: audio: PCM output is closed")
	errWASAPIDeviceChanged = errors.New("nsgo: audio: the output device changed")
	errWASAPIWaitFailed    = errors.New("nsgo: audio: audio event wait failed")
)

// OpenAudioOutput implements driver.AudioOutputFactory for the Windows backend.
func (*window) OpenAudioOutput(c driver.AudioOutputConfig) (driver.AudioOutput, error) {
	return openWASAPIOutput(c)
}

type wasapiCmdKind int

const (
	wasapiCmdStart wasapiCmdKind = iota
	wasapiCmdPause
	wasapiCmdFlush
	wasapiCmdPosition
	wasapiCmdClose
)

type wasapiCmd struct {
	kind  wasapiCmdKind
	reply chan wasapiReply
}

type wasapiReply struct {
	err     error
	dropped int
	frames  int64
	at      time.Time
}

type wasapiOutput struct {
	conf driver.AudioOutputConfig

	wakeEvent   uintptr
	bufferEvent uintptr

	cmdMu  sync.Mutex
	cmdCh  chan wasapiCmd
	closed atomic.Bool
	done   chan struct{}

	deviceSwitched atomic.Bool

	posMu     sync.Mutex
	posFrames int64
	posAt     time.Time
	active    atomic.Bool
	latency   atomic.Int64
	underruns atomic.Uint64

	// Worker-goroutine-only fields:
	enumerator   *comObject
	notifier     *wasapiNotifier
	device       *comObject
	client       *comObject
	renderClient *comObject
	clock        *comObject
	clockFreq    uint64
	qpcFreq      uint64
	bufferFrames uint32
	streamLatHns int64
	needsReopen  bool
	state        wasapiState
}

func openWASAPIOutput(conf driver.AudioOutputConfig) (driver.AudioOutput, error) {
	wakeEvent, _, err := wasapiCreateEventW.Call(0, 0, 0, 0)
	if wakeEvent == 0 {
		return nil, errors.New("nsgo: audio: CreateEventW(wake): " + err.Error())
	}
	bufferEvent, _, err := wasapiCreateEventW.Call(0, 0, 0, 0)
	if bufferEvent == 0 {
		closeHandle.Call(wakeEvent)
		return nil, errors.New("nsgo: audio: CreateEventW(buffer): " + err.Error())
	}
	var qpcFreq uint64
	wasapiQueryPerformanceFrequency.Call(uintptr(unsafe.Pointer(&qpcFreq)))

	o := &wasapiOutput{
		conf:        conf,
		wakeEvent:   wakeEvent,
		bufferEvent: bufferEvent,
		cmdCh:       make(chan wasapiCmd, 1),
		done:        make(chan struct{}),
		qpcFreq:     qpcFreq,
		state:       newWASAPIState(conf.SampleRate, conf.Channels),
	}
	ready := make(chan error, 1)
	go o.worker(ready)
	if err := <-ready; err != nil {
		<-o.done
		return nil, err
	}
	return o, nil
}

func (o *wasapiOutput) worker(ready chan<- error) {
	defer close(o.done)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := coInitializeEx.Call(0, coinitMultithreaded)
	switch {
	case hr == sOK || hr == sFalse:
		defer wasapiCoUninitialize.Call()
	case uint32(hr) == rpcEChangedMode:
		// Thread is already in a COM apartment; continue without calling CoUninitialize.
	case failed(hr):
		o.closeEvents()
		ready <- hresultError("nsgo: audio: CoInitializeEx", hr)
		return
	}

	var mmcssHandle uintptr
	if avSetMmThreadCharacteristicsW.Find() == nil {
		var taskIndex uint32
		h, _, _ := avSetMmThreadCharacteristicsW.Call(
			uintptr(unsafe.Pointer(wide("Audio"))),
			uintptr(unsafe.Pointer(&taskIndex)),
		)
		mmcssHandle = h
	}
	if mmcssHandle != 0 && avRevertMmThreadCharacteristics.Find() == nil {
		defer avRevertMmThreadCharacteristics.Call(mmcssHandle)
	}

	if err := o.initCOM(); err != nil {
		o.cleanupCOM()
		o.closeEvents()
		ready <- err
		return
	}
	ready <- nil

	o.run()

	o.cleanupCOM()
	o.closeEvents()
}

func (o *wasapiOutput) initCOM() error {
	hr, _, _ := coCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)),
		0,
		clsctxAll,
		uintptr(unsafe.Pointer(&iidMMDeviceEnumerator)),
		uintptr(unsafe.Pointer(&o.enumerator)),
	)
	if failed(hr) {
		return hresultError("nsgo: audio: CoCreateInstance(MMDeviceEnumerator)", hr)
	}
	o.notifier = newWASAPINotifier(o)
	if hr := o.enumerator.call(mmEnumRegisterEndpointNotificationCallback, o.notifier.ptr()); failed(hr) {
		o.notifier.closeAndRelease()
		o.notifier = nil
	}
	return o.openDevice()
}

func (o *wasapiOutput) openDevice() error {
	o.closeDevice()

	var dev *comObject
	if hr := o.enumerator.call(mmEnumGetDefaultAudioEndpoint, mmFlowRender, mmRoleConsole, uintptr(unsafe.Pointer(&dev))); failed(hr) {
		return hresultError("nsgo: audio: GetDefaultAudioEndpoint", hr)
	}
	o.device = dev

	if o.notifier != nil {
		var idPtr *uint16
		if hr := o.device.call(mmDeviceGetID, uintptr(unsafe.Pointer(&idPtr))); !failed(hr) && idPtr != nil {
			o.notifier.setDeviceID(wideString(idPtr))
			coTaskMemFree.Call(uintptr(unsafe.Pointer(idPtr)))
		}
	}

	var client *comObject
	if hr := o.device.call(
		mmDeviceActivate,
		uintptr(unsafe.Pointer(&iidAudioClient)),
		clsctxAll,
		0,
		uintptr(unsafe.Pointer(&client)),
	); failed(hr) {
		o.closeDevice()
		return hresultError("nsgo: audio: IMMDevice.Activate(IAudioClient)", hr)
	}
	o.client = client

	format := wasapiWaveFormat(o.conf.SampleRate, o.conf.Channels)
	hnsBuf := wasapiBufferDuration100ns(o.conf.Latency)
	if hr := initializeAudioClient(o.client, audclntShareModeShared, audclntStreamFlagsPullPCM, hnsBuf, &format); failed(hr) {
		o.closeDevice()
		return hresultError("nsgo: audio: IAudioClient.Initialize", hr)
	}

	if hr := o.client.call(acSetEventHandle, o.bufferEvent); failed(hr) {
		o.closeDevice()
		return hresultError("nsgo: audio: IAudioClient.SetEventHandle", hr)
	}

	var bufferFrames uint32
	if hr := o.client.call(acGetBufferSize, uintptr(unsafe.Pointer(&bufferFrames))); failed(hr) || bufferFrames == 0 {
		o.closeDevice()
		if failed(hr) {
			return hresultError("nsgo: audio: IAudioClient.GetBufferSize", hr)
		}
		return errors.New("nsgo: audio: IAudioClient.GetBufferSize returned zero frames")
	}
	o.bufferFrames = bufferFrames

	var streamLat int64
	if hr := o.client.call(acGetStreamLatency, uintptr(unsafe.Pointer(&streamLat))); !failed(hr) {
		o.streamLatHns = streamLat
	}

	var renderClient *comObject
	if hr := o.client.call(
		acGetService,
		uintptr(unsafe.Pointer(&iidAudioRenderClient)),
		uintptr(unsafe.Pointer(&renderClient)),
	); failed(hr) {
		o.closeDevice()
		return hresultError("nsgo: audio: IAudioClient.GetService(IAudioRenderClient)", hr)
	}
	o.renderClient = renderClient

	var clock *comObject
	if hr := o.client.call(
		acGetService,
		uintptr(unsafe.Pointer(&iidAudioClock)),
		uintptr(unsafe.Pointer(&clock)),
	); failed(hr) {
		o.closeDevice()
		return hresultError("nsgo: audio: IAudioClient.GetService(IAudioClock)", hr)
	}
	o.clock = clock

	var freq uint64
	if hr := o.clock.call(clkGetFrequency, uintptr(unsafe.Pointer(&freq))); failed(hr) || freq == 0 {
		o.closeDevice()
		if failed(hr) {
			return hresultError("nsgo: audio: IAudioClock.GetFrequency", hr)
		}
		return errors.New("nsgo: audio: IAudioClock.GetFrequency returned zero")
	}
	o.clockFreq = freq
	o.needsReopen = false
	return nil
}

//go:uintptrescapes
func initializeAudioClient(client *comObject, shareMode, flags uintptr, hnsBuffer uint64, format *waveFormatExtensible) uintptr {
	buf := u64Args(hnsBuffer)
	per := u64Args(0)
	if len(buf) == 1 {
		return client.call(acInitialize, shareMode, flags, buf[0], per[0], uintptr(unsafe.Pointer(format)), 0)
	}
	return client.call(acInitialize, shareMode, flags, buf[0], buf[1], per[0], per[1], uintptr(unsafe.Pointer(format)), 0)
}

func (o *wasapiOutput) closeDevice() {
	releaseAll(&o.clock, &o.renderClient, &o.client, &o.device)
	o.clockFreq = 0
	o.bufferFrames = 0
	o.streamLatHns = 0
	if o.notifier != nil {
		o.notifier.setDeviceID("")
	}
}

func (o *wasapiOutput) cleanupCOM() {
	if o.active.Load() {
		o.stopAndReset()
	}
	if o.notifier != nil {
		o.notifier.close()
		if o.enumerator != nil {
			o.enumerator.call(mmEnumUnregisterEndpointNotificationCallback, o.notifier.ptr())
		}
		o.notifier.release()
		o.notifier = nil
	}
	o.closeDevice()
	releaseAll(&o.enumerator)
}

func (o *wasapiOutput) closeEvents() {
	if o.wakeEvent != 0 {
		closeHandle.Call(o.wakeEvent)
		o.wakeEvent = 0
	}
	if o.bufferEvent != 0 {
		closeHandle.Call(o.bufferEvent)
		o.bufferEvent = 0
	}
}

func (o *wasapiOutput) run() {
	handles := [2]uintptr{o.wakeEvent, o.bufferEvent}
	for {
		if o.deviceSwitched.Swap(false) {
			o.handleInterrupt(errWASAPIDeviceChanged)
		}
		select {
		case cmd := <-o.cmdCh:
			if o.handleCmd(cmd) {
				return
			}
			continue
		default:
		}

		count := uintptr(1)
		timeout := uintptr(waitInfinite)
		if o.active.Load() {
			count = 2
			timeout = 500
		}
		res, _, _ := wasapiWaitForMultipleObjects.Call(count, uintptr(unsafe.Pointer(&handles[0])), 0, timeout)
		switch res {
		case waitObject0:
			continue
		case waitObject0 + 1:
			if o.deviceSwitched.Load() || len(o.cmdCh) > 0 {
				continue
			}
			if o.active.Load() {
				if err := o.renderAvailable(); err != nil {
					o.handleInterrupt(err)
				}
			}
		case waitTimeout:
			if o.active.Load() {
				if err := o.renderAvailable(); err != nil {
					o.handleInterrupt(err)
				}
			}
		default:
			if o.active.Load() {
				o.handleInterrupt(errWASAPIWaitFailed)
			}
		}
	}
}

func (o *wasapiOutput) handleCmd(cmd wasapiCmd) (exit bool) {
	switch cmd.kind {
	case wasapiCmdStart:
		cmd.reply <- wasapiReply{err: o.startWorker()}
	case wasapiCmdPause:
		o.stopAndReset()
		cmd.reply <- wasapiReply{}
	case wasapiCmdFlush:
		dropped := o.flushWorker()
		cmd.reply <- wasapiReply{dropped: dropped}
	case wasapiCmdPosition:
		if o.active.Load() {
			o.sampleClock()
		}
		frames, at := o.state.position()
		cmd.reply <- wasapiReply{frames: frames, at: at}
	case wasapiCmdClose:
		o.stopAndReset()
		cmd.reply <- wasapiReply{}
		return true
	}
	return false
}

func (o *wasapiOutput) startWorker() error {
	if o.active.Load() {
		return nil
	}
	if o.needsReopen || o.client == nil {
		if err := o.openDevice(); err != nil {
			return err
		}
	}
	wasapiResetEvent.Call(o.bufferEvent)
	if err := o.renderAvailable(); err != nil {
		if isInvalidatedErr(err) {
			o.closeDevice()
			o.needsReopen = true
		}
		return err
	}
	if hr := o.client.call(acStart); failed(hr) {
		o.client.call(acReset)
		o.state.resetRun()
		if wasapiIsDeviceError(hr) {
			o.closeDevice()
			o.needsReopen = true
		}
		return hresultError("nsgo: audio: IAudioClient.Start", hr)
	}
	var streamLat int64
	if hr := o.client.call(acGetStreamLatency, uintptr(unsafe.Pointer(&streamLat))); !failed(hr) {
		o.streamLatHns = streamLat
		o.latency.Store(int64(wasapiLatency(o.bufferFrames, o.conf.SampleRate, o.streamLatHns)))
	}
	o.active.Store(true)
	return nil
}

func (o *wasapiOutput) stopAndReset() int {
	if !o.active.Load() {
		return 0
	}
	o.sampleClock()
	if o.client != nil {
		o.client.call(acStop)
		o.sampleClock()
		o.client.call(acReset)
	}
	dropped := o.state.resetRun()
	o.active.Store(false)
	return dropped
}

func (o *wasapiOutput) flushWorker() int {
	if !o.active.Load() {
		return 0
	}
	o.sampleClock()
	var resetHR uintptr
	if o.client != nil {
		o.client.call(acStop)
		o.sampleClock()
		resetHR = o.client.call(acReset)
	}
	dropped := o.state.resetRun()
	o.active.Store(false)
	if failed(resetHR) {
		o.closeDevice()
		o.needsReopen = true
		if o.conf.Interrupt != nil {
			go o.conf.Interrupt(hresultError("nsgo: audio: IAudioClient.Reset", resetHR))
		}
		return dropped
	}
	if err := o.startWorker(); err != nil {
		o.closeDevice()
		o.needsReopen = true
		if o.conf.Interrupt != nil {
			go o.conf.Interrupt(err)
		}
	}
	return dropped
}

func (o *wasapiOutput) handleInterrupt(err error) {
	wasRunning := o.active.Load()
	if wasRunning {
		o.stopAndReset()
	}
	o.closeDevice()
	o.needsReopen = true
	if wasRunning && o.conf.Interrupt != nil {
		go o.conf.Interrupt(err)
	}
}

type wasapiDeviceError struct{ hr uintptr }

func (e wasapiDeviceError) Error() string {
	return hresultError("nsgo: audio: WASAPI device error", e.hr).Error()
}

func isInvalidatedErr(err error) bool {
	var de wasapiDeviceError
	return errors.As(err, &de) && wasapiIsDeviceError(de.hr)
}

func (o *wasapiOutput) renderAvailable() error {
	if o.client == nil || o.renderClient == nil {
		return errWASAPIDeviceChanged
	}
	var padding uint32
	if hr := o.client.call(acGetCurrentPadding, uintptr(unsafe.Pointer(&padding))); failed(hr) {
		if wasapiIsDeviceError(hr) {
			return wasapiDeviceError{hr: hr}
		}
		return hresultError("nsgo: audio: IAudioClient.GetCurrentPadding", hr)
	}
	if padding >= o.bufferFrames {
		o.sampleClock()
		return nil
	}
	available := o.bufferFrames - padding
	var dataPtr *float32
	if hr := o.renderClient.call(arcGetBuffer, uintptr(available), uintptr(unsafe.Pointer(&dataPtr))); failed(hr) {
		if wasapiIsDeviceError(hr) {
			return wasapiDeviceError{hr: hr}
		}
		return hresultError("nsgo: audio: IAudioRenderClient.GetBuffer", hr)
	}
	if dataPtr == nil {
		o.renderClient.call(arcReleaseBuffer, 0, 0)
		return nil
	}
	buf := unsafe.Slice(dataPtr, int(available)*o.conf.Channels)
	o.state.fillAndRecord(buf, o.conf.Render)
	o.underruns.Store(o.state.underruns)

	if hr := o.renderClient.call(arcReleaseBuffer, uintptr(available), 0); failed(hr) {
		if wasapiIsDeviceError(hr) {
			return wasapiDeviceError{hr: hr}
		}
		return hresultError("nsgo: audio: IAudioRenderClient.ReleaseBuffer", hr)
	}
	o.latency.Store(int64(wasapiLatency(o.bufferFrames, o.conf.SampleRate, o.streamLatHns)))
	o.sampleClock()
	return nil
}

func (o *wasapiOutput) sampleClock() {
	if o.clock == nil || o.clockFreq == 0 {
		return
	}
	var devPos, qpcPos uint64
	if hr := o.clock.call(clkGetPosition, uintptr(unsafe.Pointer(&devPos)), uintptr(unsafe.Pointer(&qpcPos))); failed(hr) {
		return
	}
	var qpcNow uint64
	wasapiQueryPerformanceCounter.Call(uintptr(unsafe.Pointer(&qpcNow)))
	wallNow := time.Now()
	frames, at := o.state.sampleClock(devPos, o.clockFreq, qpcPos, qpcNow, o.qpcFreq, wallNow)
	o.posMu.Lock()
	o.posFrames, o.posAt = frames, at
	o.posMu.Unlock()
}

func (o *wasapiOutput) sendCmd(kind wasapiCmdKind) (wasapiReply, bool) {
	o.cmdMu.Lock()
	defer o.cmdMu.Unlock()
	if o.closed.Load() {
		return wasapiReply{}, false
	}
	reply := make(chan wasapiReply, 1)
	select {
	case o.cmdCh <- wasapiCmd{kind: kind, reply: reply}:
	case <-o.done:
		return wasapiReply{}, false
	}
	wasapiSetEvent.Call(o.wakeEvent)
	select {
	case r := <-reply:
		return r, true
	case <-o.done:
		return wasapiReply{}, false
	}
}

func (o *wasapiOutput) Start() error {
	r, ok := o.sendCmd(wasapiCmdStart)
	if !ok {
		return errWASAPIClosed
	}
	return r.err
}

func (o *wasapiOutput) Pause() {
	o.sendCmd(wasapiCmdPause)
}

func (o *wasapiOutput) Flush() int {
	r, ok := o.sendCmd(wasapiCmdFlush)
	if !ok {
		return 0
	}
	return r.dropped
}

func (o *wasapiOutput) Position() (int64, time.Time) {
	if o.active.Load() && !o.closed.Load() {
		if r, ok := o.sendCmd(wasapiCmdPosition); ok {
			return r.frames, r.at
		}
	}
	o.posMu.Lock()
	frames, at := o.posFrames, o.posAt
	o.posMu.Unlock()
	return frames, at
}

func (o *wasapiOutput) Latency() time.Duration {
	return time.Duration(o.latency.Load())
}

func (o *wasapiOutput) Underruns() int {
	return int(o.underruns.Load())
}

func (o *wasapiOutput) Close() {
	o.cmdMu.Lock()
	defer o.cmdMu.Unlock()
	if o.closed.Swap(true) {
		return
	}
	reply := make(chan wasapiReply, 1)
	select {
	case o.cmdCh <- wasapiCmd{kind: wasapiCmdClose, reply: reply}:
		wasapiSetEvent.Call(o.wakeEvent)
		select {
		case <-reply:
		case <-o.done:
		}
	case <-o.done:
	}
	<-o.done
}

// wasapiNotifier implements IMMNotificationClient so default playback device switches, removals and state changes wake the worker to pause and interrupt.
type wasapiNotifier struct {
	vtbl   *[8]uintptr
	refs   atomic.Int32
	closed atomic.Bool
	owner  *wasapiOutput

	mu       sync.Mutex
	deviceID string
}

var (
	wasapiNotifyVtblOnce sync.Once
	wasapiNotifyVtbl     [8]uintptr
	wasapiNotifiersMu    sync.Mutex
	wasapiNotifiers      = map[*wasapiNotifier]struct{}{}
)

func initWASAPINotifyVtbl() {
	wasapiNotifyVtblOnce.Do(func() {
		wasapiNotifyVtbl = [8]uintptr{
			syscall.NewCallback(wasapiNotifyQueryInterface),
			syscall.NewCallback(wasapiNotifyAddRef),
			syscall.NewCallback(wasapiNotifyRelease),
			syscall.NewCallback(wasapiNotifyDeviceStateChanged),
			syscall.NewCallback(wasapiNotifyDeviceAdded),
			syscall.NewCallback(wasapiNotifyDeviceRemoved),
			syscall.NewCallback(wasapiNotifyDefaultDeviceChanged),
			wasapiNotifyPropertyChangedSlot(),
		}
	})
}

func newWASAPINotifier(owner *wasapiOutput) *wasapiNotifier {
	initWASAPINotifyVtbl()
	n := &wasapiNotifier{vtbl: &wasapiNotifyVtbl, owner: owner}
	n.refs.Store(1)
	wasapiNotifiersMu.Lock()
	wasapiNotifiers[n] = struct{}{}
	wasapiNotifiersMu.Unlock()
	return n
}

func (n *wasapiNotifier) ptr() uintptr { return uintptr(unsafe.Pointer(n)) }

func (n *wasapiNotifier) setDeviceID(id string) {
	n.mu.Lock()
	n.deviceID = id
	n.mu.Unlock()
}

func (n *wasapiNotifier) getDeviceID() string {
	n.mu.Lock()
	id := n.deviceID
	n.mu.Unlock()
	return id
}

func (n *wasapiNotifier) close() {
	n.closed.Store(true)
}

func (n *wasapiNotifier) release() {
	wasapiNotifyRelease(n)
}

func (n *wasapiNotifier) closeAndRelease() {
	n.close()
	n.release()
}

func (n *wasapiNotifier) signalSwitch() {
	if n.closed.Load() || n.owner == nil {
		return
	}
	n.owner.deviceSwitched.Store(true)
	wasapiSetEvent.Call(n.owner.wakeEvent)
}

func wasapiNotifyQueryInterface(n *wasapiNotifier, iid *guid, out *unsafe.Pointer) uintptr {
	if iid == nil || out == nil {
		return eNoInterface
	}
	if *iid == iidUnknown || *iid == iidMMNotificationClient || *iid == iidAgileObject {
		*out = unsafe.Pointer(n)
		wasapiNotifyAddRef(n)
		return sOK
	}
	*out = nil
	return eNoInterface
}

func wasapiNotifyAddRef(n *wasapiNotifier) uintptr {
	return uintptr(n.refs.Add(1))
}

func wasapiNotifyRelease(n *wasapiNotifier) uintptr {
	refs := n.refs.Add(-1)
	if refs == 0 {
		wasapiNotifiersMu.Lock()
		delete(wasapiNotifiers, n)
		wasapiNotifiersMu.Unlock()
	}
	return uintptr(refs)
}

func wasapiNotifyDeviceStateChanged(n *wasapiNotifier, deviceID *uint16, newState uintptr) uintptr {
	if n.closed.Load() || uint32(newState) == mmDeviceStateActive || deviceID == nil {
		return sOK
	}
	if current := n.getDeviceID(); current != "" && wideString(deviceID) == current {
		n.signalSwitch()
	}
	return sOK
}

func wasapiNotifyDeviceAdded(_ *wasapiNotifier, _ *uint16) uintptr {
	return sOK
}

func wasapiNotifyDeviceRemoved(n *wasapiNotifier, deviceID *uint16) uintptr {
	if n.closed.Load() || deviceID == nil {
		return sOK
	}
	if current := n.getDeviceID(); current != "" && wideString(deviceID) == current {
		n.signalSwitch()
	}
	return sOK
}

func wasapiNotifyDefaultDeviceChanged(n *wasapiNotifier, flow, role uintptr, deviceID *uint16) uintptr {
	if n.closed.Load() || uint32(flow) != mmFlowRender || uint32(role) != mmRoleConsole {
		return sOK
	}
	var nextID string
	if deviceID != nil {
		nextID = wideString(deviceID)
	}
	if current := n.getDeviceID(); current == "" || nextID != current {
		n.signalSwitch()
	}
	return sOK
}

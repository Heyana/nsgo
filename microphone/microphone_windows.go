//go:build windows

package microphone

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	coinitMultithreaded = 0x0
	rpcEChangedMode     = 0x80010106
	clsctxAll           = 0x17

	eCapture = 1
	eConsole = 0

	audclntShareModeShared          = 0
	audclntStreamFlagsEventCallback = 0x00040000
	audclntBufferFlagsSilent        = 0x2

	// 100 ms in 100-nanosecond REFERENCE_TIME units.
	wasapiBufferDuration = 1_000_000

	hrAccessDenied      = 0x80070005
	hrNotFound          = 0x80070490
	hrDeviceInvalidated = 0x88890004
	hrDeviceInUse       = 0x8889000A
	hrServiceNotRunning = 0x88890010
	hrResourcesInvalid  = 0x88890026

	waitObject0 = 0x00000000

	errDiskFull       syscall.Errno = 112
	errHandleDiskFull syscall.Errno = 39

	consentStoreMic = `Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\microphone`
)

var (
	ole32DLL       = syscall.NewLazyDLL("ole32.dll")
	coInitializeEx = ole32DLL.NewProc("CoInitializeEx")
	coUninitialize = ole32DLL.NewProc("CoUninitialize")
	coCreateInst   = ole32DLL.NewProc("CoCreateInstance")
	coTaskMemFree  = ole32DLL.NewProc("CoTaskMemFree")

	kernel32DLL                 = syscall.NewLazyDLL("kernel32.dll")
	createEventW                = kernel32DLL.NewProc("CreateEventW")
	setEventProc                = kernel32DLL.NewProc("SetEvent")
	waitForMultipleObjects      = kernel32DLL.NewProc("WaitForMultipleObjects")
	getCurrentPackageFamilyName = kernel32DLL.NewProc("GetCurrentPackageFamilyName")
	getDiskFreeSpaceExW         = kernel32DLL.NewProc("GetDiskFreeSpaceExW")
)

type comGUID struct {
	a    uint32
	b, c uint16
	d    [8]byte
}

func parseCOMGUID(s string) comGUID {
	hex := strings.ReplaceAll(s, "-", "")
	word := func(from, to int) uint64 {
		n, err := strconv.ParseUint(hex[from:to], 16, 64)
		if err != nil {
			panic("bad GUID " + s)
		}
		return n
	}
	g := comGUID{a: uint32(word(0, 8)), b: uint16(word(8, 12)), c: uint16(word(12, 16))}
	for i := range g.d {
		g.d[i] = byte(word(16+2*i, 18+2*i))
	}
	return g
}

var (
	clsidMMDeviceEnumerator = parseCOMGUID("BCDE0395-E52F-467C-8E3D-C4579291692E")
	iidIMMDeviceEnumerator  = parseCOMGUID("A95664D2-9614-4F35-A746-DE8DB63617E6")
	iidIAudioClient         = parseCOMGUID("1CB9AD4C-DBFA-4C32-B178-C2F568A703B2")
	iidIAudioCaptureClient  = parseCOMGUID("C8ADBD64-E71E-48A0-A4DE-185C395CD317")
)

// wasapiObj represents a COM interface pointer whose first word is its vtable.
type wasapiObj struct{ vtbl *[32]uintptr }

//go:uintptrescapes
func (o *wasapiObj) call(method int, args ...uintptr) uintptr {
	var buf [10]uintptr
	all := append(buf[:0], uintptr(unsafe.Pointer(o)))
	r, _, _ := syscall.SyscallN(o.vtbl[method], append(all, args...)...)
	return r
}

func (o *wasapiObj) release() {
	if o != nil {
		o.call(2) // IUnknown::Release
	}
}

func failedHR(hr uintptr) bool { return int32(hr) < 0 }

func mapWASAPIError(what string, hr uintptr) error {
	code := uint32(hr)
	switch code {
	case hrAccessDenied:
		return ErrDenied
	case hrDeviceInUse:
		return ErrBusy
	case hrNotFound, hrServiceNotRunning:
		return fmt.Errorf("%w: %s (HRESULT 0x%08x)", ErrUnavailable, what, code)
	default:
		return fmt.Errorf("microphone: %s: HRESULT 0x%08x", what, code)
	}
}

// Supported says this target can record. While nsgotest's fake microphone is
// connected, it and Allowed and Authorize answer from the fake.
func Supported() bool {
	if reachable() {
		return recorderSupported()
	}
	return ole32DLL.Load() == nil
}

// Allowed says Windows' capability access manager lets this process record,
// without asking.
func Allowed() bool {
	if reachable() {
		return recorderAllowed()
	}
	return ole32DLL.Load() == nil && !windowsMicDenied()
}

// Authorize checks Windows' microphone permission, returning nil when the
// process may open the microphone or ErrDenied when Windows blocks it.
func Authorize() error {
	if reachable() {
		return recorderAuthorize()
	}
	return authorizeSystem()
}

func authorizeSystem() error {
	if err := ole32DLL.Load(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if windowsMicDenied() {
		return ErrDenied
	}
	return nil
}

func windowsMicDenied() bool {
	machine := regStringValue(syscall.HKEY_LOCAL_MACHINE, consentStoreMic, "Value")
	user := regStringValue(syscall.HKEY_CURRENT_USER, consentStoreMic, "Value")
	appKey := packageFamilyName()
	if appKey == "" {
		appKey = "NonPackaged"
	}
	app := regStringValue(syscall.HKEY_CURRENT_USER, consentStoreMic+`\`+appKey, "Value")
	return windowsConsentDenied(machine, user, app)
}

func windowsConsentDenied(machine, user, app string) bool {
	for _, v := range []string{machine, user, app} {
		if strings.EqualFold(v, "Deny") {
			return true
		}
	}
	return false
}

func packageFamilyName() string {
	if getCurrentPackageFamilyName.Find() != nil {
		return ""
	}
	var n uint32
	getCurrentPackageFamilyName.Call(uintptr(unsafe.Pointer(&n)), 0)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n)
	if r, _, _ := getCurrentPackageFamilyName.Call(uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0]))); r != 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func regStringValue(root syscall.Handle, path, name string) string {
	sub, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	var key syscall.Handle
	if err := syscall.RegOpenKeyEx(root, sub, 0, syscall.KEY_READ, &key); err != nil {
		return ""
	}
	defer syscall.RegCloseKey(key)
	valName, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return ""
	}
	var typ, n uint32
	if err := syscall.RegQueryValueEx(key, valName, nil, &typ, nil, &n); err != nil || n == 0 || (typ != syscall.REG_SZ && typ != syscall.REG_EXPAND_SZ) {
		return ""
	}
	buf := make([]uint16, n/2+1)
	if err := syscall.RegQueryValueEx(key, valName, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &n); err != nil {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// Capture records 16-bit mono PCM at rate hertz from the default Windows
// capture endpoint via WASAPI shared mode, passing samples to the callback
// until the Stream stops or the device disconnects.
func Capture(rate int, samples func([]int16)) (*Stream, error) {
	if rate <= 0 {
		return nil, errors.New("microphone: sample rate must be positive")
	}
	if err := authorizeSystem(); err != nil {
		return nil, err
	}
	ended := make(chan struct{})
	end := sync.OnceFunc(func() { close(ended) })
	sess, err := openWASAPICapture(rate, 1, true, samples, func(bool) { end() }, func(error) { end() })
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
	return openWASAPICapture(rate, channels, false, onSamples, onInterrupt, onError)
}

type wasapiStartResult struct {
	rate     int
	channels int
	err      error
}

func openWASAPICapture(requestedRate, requestedChannels int, exactChannels bool, onSamples func([]int16), onInterrupt func(bool), onError func(error)) (*captureSession, error) {
	stopEvent, _, err := createEventW.Call(0, 1 /*manual-reset*/, 0, 0)
	if stopEvent == 0 {
		return nil, fmt.Errorf("microphone: CreateEventW: %w", err)
	}
	audioEvent, _, err := createEventW.Call(0, 0 /*auto-reset*/, 0, 0)
	if audioEvent == 0 {
		syscall.CloseHandle(syscall.Handle(stopEvent))
		return nil, fmt.Errorf("microphone: CreateEventW: %w", err)
	}

	started := make(chan wasapiStartResult, 1)
	done := make(chan struct{})
	var stopped atomic.Bool

	go func() {
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		hr, _, _ := coInitializeEx.Call(0, coinitMultithreaded)
		comInit := !failedHR(hr)
		if !comInit && uint32(hr) != rpcEChangedMode {
			started <- wasapiStartResult{err: mapWASAPIError("CoInitializeEx", hr)}
			return
		}
		if comInit {
			defer coUninitialize.Call()
		}

		var enumerator *wasapiObj
		hr, _, _ = coCreateInst.Call(
			uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)),
			0,
			clsctxAll,
			uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)),
			uintptr(unsafe.Pointer(&enumerator)),
		)
		if failedHR(hr) || enumerator == nil {
			started <- wasapiStartResult{err: mapWASAPIError("CoCreateInstance(MMDeviceEnumerator)", hr)}
			return
		}
		defer enumerator.release()

		var device *wasapiObj
		hr = enumerator.call(4 /*GetDefaultAudioEndpoint*/, eCapture, eConsole, uintptr(unsafe.Pointer(&device)))
		if failedHR(hr) || device == nil {
			started <- wasapiStartResult{err: mapWASAPIError("GetDefaultAudioEndpoint", hr)}
			return
		}
		defer device.release()

		var audioClient *wasapiObj
		hr = device.call(3 /*Activate*/, uintptr(unsafe.Pointer(&iidIAudioClient)), clsctxAll, 0, uintptr(unsafe.Pointer(&audioClient)))
		if failedHR(hr) || audioClient == nil {
			started <- wasapiStartResult{err: mapWASAPIError("IMMDevice::Activate(IAudioClient)", hr)}
			return
		}
		defer audioClient.release()

		var mixFmtPtr unsafe.Pointer
		hr = audioClient.call(8 /*GetMixFormat*/, uintptr(unsafe.Pointer(&mixFmtPtr)))
		if failedHR(hr) || mixFmtPtr == nil {
			started <- wasapiStartResult{err: mapWASAPIError("IAudioClient::GetMixFormat", hr)}
			return
		}
		defer coTaskMemFree.Call(uintptr(mixFmtPtr))

		srcFmt, err := readWASAPIMixFormat(mixFmtPtr)
		if err != nil {
			started <- wasapiStartResult{err: err}
			return
		}

		dstRate := requestedRate
		if dstRate <= 0 {
			dstRate = srcFmt.sampleRate
		}
		dstChannels := requestedChannels
		if dstChannels < 1 {
			dstChannels = 1
		}
		if !exactChannels && dstChannels > srcFmt.channels {
			dstChannels = srcFmt.channels
		}
		conv := newPCMConverter(srcFmt, dstRate, dstChannels)

		hr = callAudioClientInitialize(audioClient, audclntShareModeShared, audclntStreamFlagsEventCallback, wasapiBufferDuration, 0, mixFmtPtr)
		if failedHR(hr) {
			started <- wasapiStartResult{err: mapWASAPIError("IAudioClient::Initialize", hr)}
			return
		}

		hr = audioClient.call(13 /*SetEventHandle*/, audioEvent)
		if failedHR(hr) {
			started <- wasapiStartResult{err: mapWASAPIError("IAudioClient::SetEventHandle", hr)}
			return
		}

		var captureClient *wasapiObj
		hr = audioClient.call(14 /*GetService*/, uintptr(unsafe.Pointer(&iidIAudioCaptureClient)), uintptr(unsafe.Pointer(&captureClient)))
		if failedHR(hr) || captureClient == nil {
			started <- wasapiStartResult{err: mapWASAPIError("IAudioClient::GetService(IAudioCaptureClient)", hr)}
			return
		}
		defer captureClient.release()

		hr = audioClient.call(10 /*Start*/)
		if failedHR(hr) {
			started <- wasapiStartResult{err: mapWASAPIError("IAudioClient::Start", hr)}
			return
		}
		defer audioClient.call(11 /*Stop*/)

		started <- wasapiStartResult{rate: dstRate, channels: dstChannels}

		handles := [2]uintptr{stopEvent, audioEvent}
		for !stopped.Load() {
			r, _, _ := waitForMultipleObjects.Call(2, uintptr(unsafe.Pointer(&handles[0])), 0, 200)
			if r == waitObject0 || stopped.Load() {
				break
			}
			if !drainWASAPIPackets(captureClient, conv, &stopped, onSamples, onInterrupt, onError) {
				break
			}
		}
	}()

	res := <-started
	if res.err != nil {
		<-done
		syscall.CloseHandle(syscall.Handle(audioEvent))
		syscall.CloseHandle(syscall.Handle(stopEvent))
		return nil, res.err
	}

	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			stopped.Store(true)
			setEventProc.Call(stopEvent)
			<-done
			syscall.CloseHandle(syscall.Handle(audioEvent))
			syscall.CloseHandle(syscall.Handle(stopEvent))
		})
	}
	return &captureSession{rate: res.rate, channels: res.channels, stop: stop}, nil
}

func readWASAPIMixFormat(ptr unsafe.Pointer) (waveFormat, error) {
	head := unsafe.Slice((*byte)(ptr), 18)
	tag := binary.LittleEndian.Uint16(head[0:2])
	cbSize := binary.LittleEndian.Uint16(head[16:18])
	if tag == waveFormatExtensible && cbSize >= 22 {
		return parseWaveFormat(unsafe.Slice((*byte)(ptr), 40))
	}
	return parseWaveFormat(head)
}

// callAudioClientInitialize invokes IAudioClient::Initialize (vtable index 3),
// handling 64-bit REFERENCE_TIME arguments on both 32-bit and 64-bit Windows.
func callAudioClientInitialize(client *wasapiObj, shareMode, streamFlags uint32, bufferDur, periodicity int64, format unsafe.Pointer) uintptr {
	if unsafe.Sizeof(uintptr(0)) == 4 {
		return client.call(3,
			uintptr(shareMode),
			uintptr(streamFlags),
			uintptr(uint32(bufferDur)), uintptr(uint32(bufferDur>>32)),
			uintptr(uint32(periodicity)), uintptr(uint32(periodicity>>32)),
			uintptr(format),
			0,
		)
	}
	return client.call(3,
		uintptr(shareMode),
		uintptr(streamFlags),
		uintptr(bufferDur),
		uintptr(periodicity),
		uintptr(format),
		0,
	)
}

func drainWASAPIPackets(captureClient *wasapiObj, conv *pcmConverter, stopped *atomic.Bool, onSamples func([]int16), onInterrupt func(bool), onError func(error)) bool {
	for !stopped.Load() {
		var packetFrames uint32
		hr := captureClient.call(5 /*GetNextPacketSize*/, uintptr(unsafe.Pointer(&packetFrames)))
		if failedHR(hr) {
			reportWASAPIRuntimeError(hr, onInterrupt, onError)
			return false
		}
		if packetFrames == 0 {
			return true
		}
		var dataPtr unsafe.Pointer
		var numFrames, flags uint32
		hr = captureClient.call(3, /*GetBuffer*/
			uintptr(unsafe.Pointer(&dataPtr)),
			uintptr(unsafe.Pointer(&numFrames)),
			uintptr(unsafe.Pointer(&flags)),
			0,
			0,
		)
		if failedHR(hr) {
			reportWASAPIRuntimeError(hr, onInterrupt, onError)
			return false
		}
		silent := flags&audclntBufferFlagsSilent != 0 || dataPtr == nil
		var raw []byte
		if !silent && numFrames > 0 {
			raw = unsafe.Slice((*byte)(dataPtr), int(numFrames)*conv.src.blockAlign)
		}
		out := conv.convert(raw, int(numFrames), silent)
		captureClient.call(4 /*ReleaseBuffer*/, uintptr(numFrames))
		if len(out) > 0 && !stopped.Load() {
			onSamples(out)
		}
	}
	return false
}

func reportWASAPIRuntimeError(hr uintptr, onInterrupt func(bool), onError func(error)) {
	switch uint32(hr) {
	case hrDeviceInvalidated, hrServiceNotRunning, hrResourcesInvalid:
		if onInterrupt != nil {
			onInterrupt(true)
			return
		}
	}
	if onError != nil {
		onError(mapWASAPIError("IAudioCaptureClient", hr))
	}
}

func availableDiskSpace(dir string) int64 {
	if dir == "" || getDiskFreeSpaceExW.Find() != nil {
		return -1
	}
	if filepath.VolumeName(dir) == dir {
		dir += string(filepath.Separator)
	}
	p, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return -1
	}
	var available uint64
	ok, _, _ := getDiskFreeSpaceExW.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&available)), 0, 0)
	if ok == 0 {
		return -1
	}
	return int64(min(available, math.MaxInt64))
}

func isNoSpaceError(err error) bool {
	return errors.Is(err, errDiskFull) || errors.Is(err, errHandleDiskFull)
}

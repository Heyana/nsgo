//go:build windows

package windows

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

var (
	mfCreateByteStream     = mfplat.NewProc("MFCreateMFByteStreamOnStream")
	mfCreateMediaType      = mfplat.NewProc("MFCreateMediaType")
	mfCreateSourceReader   = syscall.NewLazyDLL("mfreadwrite.dll").NewProc("MFCreateSourceReaderFromByteStream")
	iidMFAttributes        = parseGUID("2cd2d921-c447-44a7-a13c-4adabfc247e3")
	mfByteStreamOriginName = parseGUID("fc358288-3cb6-460c-a424-b6681260375a")
	mfMajorType            = parseGUID("48eba18e-f8c9-4687-bf11-0a74c9f96a8f")
	mfSubtype              = parseGUID("f7e34c9a-42e8-4714-b74b-cb29d72c35e5")
	mfAudioType            = parseGUID("73647561-0000-0010-8000-00aa00389b71")
	mfFloatFormat          = parseGUID("00000003-0000-0010-8000-00aa00389b71")
	mfAudioChannels        = parseGUID("37e48bf5-645e-4c5b-89de-ada9e29b696a")
	mfAudioRate            = parseGUID("5faeeae7-0290-4c31-9e8a-c534f68d9dba")
)

const (
	mfAllStreams          = 0xfffffffe
	mfFirstAudioStream    = 0xfffffffd
	mfReaderError         = 0x1
	mfReaderEnd           = 0x2
	mfReaderFormatChanged = 0x20

	// Vtable indexes from mfreadwrite.h and mfobjects.h.
	mfReaderSelectStream = 4
	mfReaderCurrentType  = 6
	mfReaderSetType      = 7
	mfReaderReadSample   = 9
	mfGetUINT32          = 7
	mfSetGUID            = 24
	mfSetString          = 25
	mfSampleBuffer       = 41
	mfBufferLock         = 3
	mfBufferUnlock       = 4
)

// Decode jobs share one MTA thread, started at the first sound load. The
// fetches can overlap and only goroutines hand the decoding thread work.
var soundDecoder = sync.OnceValue(func() chan func(error) {
	jobs := make(chan func(error))
	go func() {
		runtime.LockOSThread()
		const coinitMultithreaded = 0
		var err error
		if hr, _, _ := coInitializeEx.Call(0, coinitMultithreaded); failed(hr) {
			err = hresultError("cannot initialize the audio decoder", hr)
		} else {
			err = mfStarted()
		}
		for job := range jobs {
			job(err)
		}
	}()
	return jobs
})

func (a *windowAudio) load(id uint32, src driver.ImageSource, maxBytes int64, limit time.Duration) {
	if limit <= 0 {
		limit = driver.MaxSoundDuration
	}
	limit = min(limit, driver.MaxSoundDuration)
	if maxBytes <= 0 {
		maxBytes = driver.MaxSoundBytes
	}
	maxBytes = min(maxBytes, driver.MaxSoundBytes)
	ctx, cancel := context.WithCancel(context.Background())
	a.loading[id] = cancel
	go func() {
		complete := func(pcm decodedSound, ev driver.AudioEvent) {
			if ctx.Err() != nil {
				return
			}
			a.w.post(func() {
				if ctx.Err() != nil || a.w.audio != a {
					return
				}
				delete(a.loading, id)
				cancel()
				if ev.Phase == driver.AudioLoaded && !ev.Cut {
					p := coAlloc(uintptr(len(pcm.data)))
					if p == nil {
						ev = audioFailure(driver.AudioFailed, "cannot allocate the sound's playback buffer")
					} else {
						copy(unsafe.Slice((*byte)(p), len(pcm.data)), pcm.data)
						a.sounds[id] = &winSound{format: pcm.format, data: p, size: uint32(len(pcm.data))}
					}
				}
				a.report(id, ev)
			})
		}
		var oversized atomic.Bool
		fetchCtx, stopFetch := context.WithCancel(ctx)
		data, err := imageload.Load(fetchCtx, src, func(loaded, total int64) {
			if loaded > maxBytes || total > maxBytes {
				oversized.Store(true)
				stopFetch()
			}
		})
		stopFetch()
		if oversized.Load() || int64(len(data)) > maxBytes {
			complete(decodedSound{}, audioFailure(driver.AudioQuotaExceeded, "the sound has more than 8 MiB"))
			return
		}
		if err != nil {
			code := driver.AudioNotSupported
			if remoteAudio(src.URI) {
				code = driver.AudioNetwork
			}
			complete(decodedSound{}, audioFailure(code, err.Error()))
			return
		}
		job := func(startErr error) {
			if ctx.Err() != nil {
				return
			}
			if startErr != nil {
				complete(decodedSound{}, audioFailure(driver.AudioUnavailable, startErr.Error()))
				return
			}
			pcm, ev, err := decodeWindowsSound(data, src.URI, limit)
			if err != nil {
				ev = audioFailure(driver.AudioDecode, err.Error())
			}
			complete(pcm, ev)
		}
		select {
		case soundDecoder() <- job:
		case <-ctx.Done():
		}
	}()
}

type soundFormat struct {
	channels uint32
	rate     uint32
}

type decodedSound struct {
	format soundFormat
	data   []byte
}

func decodeWindowsSound(data []byte, name string, limit time.Duration) (decodedSound, driver.AudioEvent, error) {
	var pcm decodedSound
	var event driver.AudioEvent
	reader, err := soundReader(data, name)
	if err != nil {
		return pcm, event, err
	}
	defer reader.release()
	reader.call(mfReaderSelectStream, mfAllStreams, 0)
	if hr := reader.call(mfReaderSelectStream, mfFirstAudioStream, 1); failed(hr) {
		return pcm, event, hresultError("the sound has no audio", hr)
	}
	if err := setSoundFormat(reader); err != nil {
		return pcm, event, err
	}
	var format soundFormat
	var frameBytes, maxBytes int
	readFormat := true
	for {
		var flags uint32
		var sample *comObject
		hr := reader.call(mfReaderReadSample, mfFirstAudioStream, 0, 0, uintptr(unsafe.Pointer(&flags)), 0, uintptr(unsafe.Pointer(&sample)))
		if failed(hr) {
			sample.release()
			return decodedSound{}, event, hresultError("the sound cannot be decoded", hr)
		}
		if flags&mfReaderError != 0 {
			sample.release()
			return decodedSound{}, event, errors.New("the audio decoder could not read the next sample")
		}
		readFormat = readFormat || flags&mfReaderFormatChanged != 0
		if sample != nil {
			if readFormat {
				// Some decoders, including AAC, negotiate their actual
				// output when the reader asks for the first sample.
				actual, err := getSoundFormat(reader)
				if err != nil {
					sample.release()
					return decodedSound{}, event, err
				}
				if len(pcm.data) != 0 && actual != format {
					sample.release()
					return decodedSound{}, event, fmt.Errorf("the sound changed from %d channels at %d Hz to %d channels at %d Hz", format.channels, format.rate, actual.channels, actual.rate)
				}
				format = actual
				pcm.format = soundFormat{channels: min(actual.channels, 2), rate: actual.rate}
				frameBytes = int(pcm.format.channels) * 4
				maxBytes = int(limit*time.Duration(actual.rate)/time.Second) * frameBytes
				readFormat = false
			}
			cut, err := pcm.appendSample(sample, int(format.channels)*4, maxBytes)
			sample.release()
			if err != nil {
				return decodedSound{}, event, err
			}
			if cut {
				return decodedSound{}, driver.AudioEvent{Phase: driver.AudioLoaded, Duration: limit, Cut: true}, nil
			}
		}
		if flags&mfReaderEnd != 0 {
			break
		}
	}
	if len(pcm.data) == 0 {
		return pcm, event, errors.New("the sound has no samples")
	}
	duration := time.Duration(len(pcm.data)/frameBytes) * time.Second / time.Duration(format.rate)
	return pcm, driver.AudioEvent{Phase: driver.AudioLoaded, Duration: duration, Bytes: int64(len(pcm.data))}, nil
}

func soundReader(data []byte, name string) (*comObject, error) {
	bytes, err := mediaByteStream(data, "sound"+audioExtension(data, name))
	if err != nil {
		return nil, err
	}
	defer bytes.release()
	var reader *comObject
	if hr, _, _ := mfCreateSourceReader.Call(bytes.ptr(), 0, uintptr(unsafe.Pointer(&reader))); failed(hr) || reader == nil {
		return nil, hresultError("the sound cannot be decoded", hr)
	}
	return reader, nil
}

func mediaByteStream(data []byte, name string) (*comObject, error) {
	if len(data) == 0 {
		return nil, errors.New("the sound is empty")
	}
	if uint64(len(data)) > math.MaxUint32 {
		return nil, errors.New("the audio source is too large for a memory stream")
	}
	ptr, _, _ := shCreateMemStream.Call(uintptr(unsafe.Pointer(unsafe.SliceData(data))), uintptr(len(data)))
	stream := *(**comObject)(unsafe.Pointer(&ptr))
	if stream == nil {
		return nil, errors.New("cannot allocate the sound's input stream")
	}
	defer stream.release()
	var bytes *comObject
	if hr, _, _ := mfCreateByteStream.Call(stream.ptr(), uintptr(unsafe.Pointer(&bytes))); failed(hr) || bytes == nil {
		return nil, hresultError("cannot open the sound's input stream", hr)
	}
	if attrs := bytes.query(&iidMFAttributes); attrs != nil {
		attrs.call(mfSetString, uintptr(unsafe.Pointer(&mfByteStreamOriginName)), uintptr(unsafe.Pointer(wide(name))))
		attrs.release()
	}
	return bytes, nil
}

func setSoundFormat(reader *comObject) error {
	var wanted *comObject
	if hr, _, _ := mfCreateMediaType.Call(uintptr(unsafe.Pointer(&wanted))); failed(hr) || wanted == nil {
		return hresultError("cannot create the decoded audio format", hr)
	}
	defer wanted.release()
	if hr := wanted.call(mfSetGUID, uintptr(unsafe.Pointer(&mfMajorType)), uintptr(unsafe.Pointer(&mfAudioType))); failed(hr) {
		return hresultError("cannot select audio decoding", hr)
	}
	if hr := wanted.call(mfSetGUID, uintptr(unsafe.Pointer(&mfSubtype)), uintptr(unsafe.Pointer(&mfFloatFormat))); failed(hr) {
		return hresultError("cannot select floating-point audio", hr)
	}
	if hr := reader.call(mfReaderSetType, mfFirstAudioStream, 0, wanted.ptr()); failed(hr) {
		return hresultError("the sound cannot be decoded to PCM", hr)
	}
	return nil
}

func getSoundFormat(reader *comObject) (soundFormat, error) {
	var actual *comObject
	if hr := reader.call(mfReaderCurrentType, mfFirstAudioStream, uintptr(unsafe.Pointer(&actual))); failed(hr) || actual == nil {
		return soundFormat{}, hresultError("cannot read the decoded sound's format", hr)
	}
	defer actual.release()
	var format soundFormat
	actual.call(mfGetUINT32, uintptr(unsafe.Pointer(&mfAudioChannels)), uintptr(unsafe.Pointer(&format.channels)))
	actual.call(mfGetUINT32, uintptr(unsafe.Pointer(&mfAudioRate)), uintptr(unsafe.Pointer(&format.rate)))
	if format.channels == 0 || format.channels > 64 || format.rate < 1000 || format.rate > 200000 {
		return soundFormat{}, errors.New("the sound's channel count or sample rate is unsupported")
	}
	return format, nil
}

func (s *decodedSound) appendSample(sample *comObject, stride, capBytes int) (bool, error) {
	var buffer *comObject
	if hr := sample.call(mfSampleBuffer, uintptr(unsafe.Pointer(&buffer))); failed(hr) || buffer == nil {
		return false, hresultError("cannot read decoded audio", hr)
	}
	defer buffer.release()
	var data *byte
	var size uint32
	if hr := buffer.call(mfBufferLock, uintptr(unsafe.Pointer(&data)), 0, uintptr(unsafe.Pointer(&size))); failed(hr) {
		return false, hresultError("cannot lock decoded audio", hr)
	}
	defer buffer.call(mfBufferUnlock)
	frameBytes := int(s.format.channels) * 4
	frames := int(size) / stride
	count := min(frames, (capBytes-len(s.data))/frameBytes)
	if count > 0 {
		pcm := unsafe.Slice(data, count*stride)
		if stride == frameBytes {
			s.data = append(s.data, pcm...)
		} else {
			for at := 0; at < len(pcm); at += stride {
				s.data = append(s.data, pcm[at:at+frameBytes]...)
			}
		}
	}
	return count < frames, nil
}

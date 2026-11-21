//go:build windows

package windows

import (
	"encoding/binary"
	"errors"
	"slices"
	"syscall"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	xaudio              = syscall.NewLazyDLL("xaudio2_9.dll")
	xaCreate            = xaudio.NewProc("XAudio2Create")
	xaCreateWithVersion = xaudio.NewProc("XAudio2CreateWithVersionInfo")
)

// Vtable indexes and packed structures are from xaudio2.h. Voices lack
// the three IUnknown methods that start an IXAudio2 interface.
const (
	xaCreateSourceVoice = 5
	xaCreateMasterVoice = 7
	xaVoiceDetails      = 0
	xaVoiceVolume       = 12
	xaVoiceSetMatrix    = 16
	xaVoiceGetMatrix    = 17
	xaVoiceDestroy      = 18
	xaVoiceStart        = 19
	xaVoiceSubmit       = 21
	xaVoiceState        = 25
	xaVoiceRate         = 26
	xaEndOfStream       = 0x40
	xaNoSamplesPlayed   = 0x100
	xaGameEffects       = 6
)

type winSound struct {
	format soundFormat
	data   unsafe.Pointer // native PCM memory, freed after its source voices
	size   uint32
}

type soundOutput struct {
	engine      *comObject
	master      *comObject
	channels    uint32       // the mastering voice's
	voices      []soundVoice // oldest playback first
	initialized bool         // this output holds a COM reference to the thread's apartment
}

type soundVoice struct {
	voice  *comObject
	format soundFormat
	sound  uint32
	// matrix is XAudio2's default output matrix into a mastering voice of two or more channels, one row of format.channels levels per output channel, which Voice.Pan scales; nil for a mono output, which takes no pan.
	matrix []float32
}

// xaBuffer is XAUDIO2_BUFFER. The zeroed tail represents the optional
// playback and loop regions and the packed pContext pointer.
type xaBuffer struct {
	flags, size uint32
	data        unsafe.Pointer
	tail        [20 + unsafe.Sizeof(uintptr(0))]byte
}

func openSoundOutput() (_ *soundOutput, err error) {
	const apartmentThreaded = 2
	com, _, _ := coInitializeEx.Call(0, apartmentThreaded)
	initialized := !failed(com)
	if !initialized && uint32(com) != rpcEChangedMode {
		return nil, hresultError("cannot initialize COM for XAudio2", com)
	}
	defer func() {
		if err != nil && initialized {
			coUninitialize.Call()
		}
	}()
	var engine *comObject
	var hr uintptr
	if xaCreateWithVersion.Find() == nil {
		const ntddiWin10 = 0x0a000000
		hr, _, _ = xaCreateWithVersion.Call(uintptr(unsafe.Pointer(&engine)), 0, 0, ntddiWin10)
	} else if xaCreate.Find() == nil {
		const processor1 = 1
		hr, _, _ = xaCreate.Call(uintptr(unsafe.Pointer(&engine)), 0, processor1)
	} else {
		return nil, errors.New("XAudio2 is unavailable")
	}
	if failed(hr) || engine == nil {
		return nil, hresultError("cannot initialize XAudio2", hr)
	}
	var master *comObject
	if hr := engine.call(xaCreateMasterVoice, uintptr(unsafe.Pointer(&master)), 0, 0, 0, 0, 0, xaGameEffects); failed(hr) || master == nil {
		engine.release()
		return nil, hresultError("cannot open the default XAudio2 output", hr)
	}
	var details [4]uint32 // XAUDIO2_VOICE_DETAILS: CreationFlags, ActiveFlags, InputChannels, InputSampleRate
	master.call(xaVoiceDetails, uintptr(unsafe.Pointer(&details)))
	return &soundOutput{engine: engine, master: master, channels: details[2], initialized: initialized}, nil
}

func (s *soundOutput) newVoice(format soundFormat) soundVoice {
	// WAVEFORMATEX is packed. Media Foundation decoded interleaved float PCM.
	var wave [18]byte
	const waveFloat = 3
	align := uint16(format.channels * 4)
	binary.LittleEndian.PutUint16(wave[0:], waveFloat)
	binary.LittleEndian.PutUint16(wave[2:], uint16(format.channels))
	binary.LittleEndian.PutUint32(wave[4:], format.rate)
	binary.LittleEndian.PutUint32(wave[8:], format.rate*uint32(align))
	binary.LittleEndian.PutUint16(wave[12:], align)
	binary.LittleEndian.PutUint16(wave[14:], 32)
	var voice *comObject
	if hr := s.engine.callF(xaCreateSourceVoice, 1<<3, uintptr(unsafe.Pointer(&voice)), uintptr(unsafe.Pointer(&wave)), 0, f32(2), 0, 0, 0); failed(hr) || voice == nil {
		return soundVoice{}
	}
	v := soundVoice{voice: voice, format: format}
	if s.channels >= 2 {
		v.matrix = make([]float32, format.channels*s.channels)
		voice.call(xaVoiceGetMatrix, 0, uintptr(format.channels), uintptr(s.channels), uintptr(unsafe.Pointer(&v.matrix[0])))
		if !slices.ContainsFunc(v.matrix, func(level float32) bool { return level != 0 }) {
			v.matrix = nil // XAudio2 reported no default mix to scale; the voice plays centered.
		}
	}
	return v
}

// pan sets v's output matrix to its default with the front left output's row scaled by driver.AudioPanGains' left gain and the front right's by its right gain, the first two outputs in XAudio2's channel order, so a centered voice keeps XAudio2's own mix.
func (s *soundOutput) pan(v soundVoice, pan float32) uintptr {
	if v.matrix == nil {
		return 0
	}
	left, right := driver.AudioPanGains(pan)
	in := int(v.format.channels)
	m := slices.Clone(v.matrix)
	for i := range in {
		m[i] *= left
		m[in+i] *= right
	}
	return v.voice.call(xaVoiceSetMatrix, 0, uintptr(in), uintptr(s.channels), uintptr(unsafe.Pointer(&m[0])), 0)
}

func (v soundVoice) idle() bool {
	// XAUDIO2_VOICE_STATE is a pointer, a UINT32 count and a packed UINT64 sample count.
	const pointerBytes = unsafe.Sizeof(uintptr(0))
	var state [pointerBytes + 12]byte
	v.voice.call(xaVoiceState, uintptr(unsafe.Pointer(&state)), xaNoSamplesPlayed)
	return binary.LittleEndian.Uint32(state[pointerBytes:]) == 0
}

func (s *soundOutput) play(id uint32, sound *winSound, volume, rate, pan float32) bool {
	var v soundVoice
	for i, idle := range s.voices {
		if idle.format == sound.format && idle.idle() {
			v = idle
			s.voices = slices.Delete(s.voices, i, i+1)
			break
		}
	}
	if v.voice == nil {
		if len(s.voices) == driver.MaxSoundVoices {
			// Reclaim an idle voice before replacing the oldest playback.
			i := max(0, slices.IndexFunc(s.voices, soundVoice.idle))
			s.voices[i].voice.call(xaVoiceDestroy)
			s.voices = slices.Delete(s.voices, i, i+1)
		}
		v = s.newVoice(sound.format)
		if v.voice == nil {
			return false
		}
	}
	voice := v.voice
	buffer := xaBuffer{flags: xaEndOfStream, size: sound.size, data: sound.data}
	if failed(voice.callF(xaVoiceVolume, 1, f32(volume), 0)) || failed(voice.callF(xaVoiceRate, 1, f32(rate), 0)) || failed(s.pan(v, pan)) ||
		failed(voice.call(xaVoiceSubmit, uintptr(unsafe.Pointer(&buffer)), 0)) || failed(voice.call(xaVoiceStart, 0, 0)) {
		voice.call(xaVoiceDestroy)
		return false
	}
	v.sound = id
	s.voices = append(s.voices, v)
	return true
}

func (a *windowAudio) play(id uint32, volume, rate, pan float32) {
	sound := a.sounds[id]
	if sound == nil {
		return
	}
	if a.output == nil {
		a.output, _ = openSoundOutput()
	}
	if a.output != nil && !a.output.play(id, sound, volume, rate, pan) {
		// Device failures are silent. The next Play can reopen the default output.
		a.closeOutput()
	}
}

func (a *windowAudio) stop(id uint32) {
	if a.output != nil {
		a.output.voices = slices.DeleteFunc(a.output.voices, func(v soundVoice) bool {
			if v.sound != id {
				return false
			}
			v.voice.call(xaVoiceDestroy)
			return true
		})
	}
}

func (a *windowAudio) unload(id uint32) {
	if cancel := a.loading[id]; cancel != nil {
		delete(a.loading, id)
		cancel()
	}
	a.stop(id)
	if sound := a.sounds[id]; sound != nil {
		delete(a.sounds, id)
		coTaskMemFree.Call(uintptr(sound.data))
	}
}

func (a *windowAudio) closeOutput() {
	if a.output != nil {
		for _, v := range a.output.voices {
			v.voice.call(xaVoiceDestroy)
		}
		a.output.master.call(xaVoiceDestroy)
		a.output.engine.release()
		if a.output.initialized {
			coUninitialize.Call()
		}
		a.output = nil
	}
}

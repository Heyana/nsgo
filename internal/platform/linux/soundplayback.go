//go:build linux && !android

package linux

import (
	"slices"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func setSoundVolume(pad uintptr, volume float32) {
	var value gValue
	gValueInit(uintptr(unsafe.Pointer(&value)), gTypeDouble)
	gValueSetDouble(&value, float64(volume))
	gObjectSetProperty(pad, cstr("volume"), &value)
	gValueUnset(uintptr(unsafe.Pointer(&value)))
}

func (m *soundMixer) endPlayback(play soundPlayback) {
	if play.playback != 0 && m.ended != nil {
		m.ended(play.id, play.playback)
	}
}

func (m *soundMixer) finishVoice(voice soundVoice) {
	m.remove(voice)
	m.endPlayback(voice.play)
}

func (m *soundMixer) control(id uint32, req *driver.AudioRequest) {
	if req.Playback == 0 {
		return
	}
	for i := range m.pending {
		play := &m.pending[i]
		if play.id == id && play.playback == req.Playback {
			if req.Command == driver.AudioPlaybackStop {
				m.endPlayback(*play)
				m.pending = slices.Delete(m.pending, i, i+1)
			} else {
				play.volume, play.rate = req.Volume, req.Rate
			}
			return
		}
	}
	for i, voice := range m.voices {
		if voice.play.id == id && voice.play.playback == req.Playback {
			if req.Command == driver.AudioPlaybackStop {
				m.finishVoice(voice)
				m.voices = slices.Delete(m.voices, i, i+1)
			} else {
				m.updateVoice(i, req.Volume, req.Rate)
			}
			return
		}
	}
}

func (m *soundMixer) updateVoice(index int, volume, rate float32) {
	voice := &m.voices[index]
	// The mixer applies pad volume to each output block. An upstream
	// volume element would have processed the queued PCM already.
	setSoundVolume(voice.pad, volume)
	voice.play.volume = volume
	if voice.play.rate == rate {
		return
	}
	start := time.Now().Add(20 * time.Millisecond)
	if start.Before(voice.started) {
		start = voice.started
	}
	elapsed := max(time.Duration(0), start.Sub(voice.started))
	frames := int(elapsed.Seconds() * float64(voice.play.rate) * soundRate)
	total := len(voice.play.pcm) / soundFrameBytes
	play := voice.play
	if play.loop {
		// A looping voice resumes its copies from where the current one is.
		play.skip = (play.skip + frames) % total
	} else {
		if frames >= total {
			return // the source finished and only the output buffer remains
		}
		play.pcm = play.pcm[frames*soundFrameBytes:]
	}
	play.rate, play.startAt = rate, start
	// The source caps determine speed. Reconnect the remaining PCM at the new rate while preserving this playback's position and eviction order.
	m.remove(*voice)
	replacement, ok := m.prepareVoice(play)
	if !ok {
		m.voices = slices.Delete(m.voices, index, index+1)
		m.endPlayback(play)
		return
	}
	m.voices[index] = replacement
}

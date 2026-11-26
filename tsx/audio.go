package tsx

import (
	"errors"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx/internal/js"
)

// nsgo/audio exposes the audio package's sounds (Scope.LoadSound): decoded once and played many times, with a handle for each playback, held to the "audio" capability (core.AccessAudio). A package's sounds keep NSH's default limits whatever the Go app set with audio.SetSoundLimits, as core counts a restricted scope's loads apart; TSX has no setter.
func init() {
	moduleTable["nsgo/audio"] = []export{
		fn("supported", (*runtime).audioSupported),
		fn("loadSound", (*runtime).loadSound),
	}
}

func (r *runtime) audioSupported(_ js.FunctionCall) js.Value {
	if r.checkAccess(core.AccessRequest{Permission: core.AccessAudio}) != nil {
		return r.vm.ToValue(false)
	}
	return r.vm.ToValue(core.Untracked(r.cx, r.cx.Env().CanPlayAudio))
}

// loadSound resolves with a Sound of a source URI, as an image's: an "asset:" path, a data: URL or an http(s) URL. It rejects with the audio error's name as reason, and with core's stable code where there is one: ERR_ACCESS_DENIED for a refused capability, ERR_NSH_QUOTA_EXCEEDED past a limit and ERR_FEATURE_UNAVAILABLE_ON_PLATFORM where nothing plays audio. In a sandbox, each load counts against Limits.Sounds until it fails or its sound closes, as core's quotas count distinct sources while every load of one holds a handle until the view goes.
func (r *runtime) loadSound(call js.FunctionCall) js.Value {
	p, resolve, reject := r.newPromise()
	uri, ok := arg(call, 0).Export().(string)
	if !ok {
		_ = reject(r.goError(&JSError{Name: "TypeError", Message: "loadSound: the source must be a URI string"}))
		return r.vm.ToValue(p)
	}
	if r.sandbox != nil {
		if l := r.limits.Sounds; l > 0 && r.sandbox.sounds >= l {
			_ = reject(r.audioError(quota("sounds", l)))
			return r.vm.ToValue(p)
		}
		r.sandbox.sounds++
	}
	r.cx.LoadSound(core.AudioSource{URI: uri}, func(s *core.Sound, err error) {
		r.inJS(func() {
			if err != nil {
				r.soundClosed()
				_ = reject(r.audioError(err))
				return
			}
			_ = resolve(r.soundValue(s))
		})
	})
	return r.vm.ToValue(p)
}

// soundClosed counts a sandbox's load failed or its sound closed.
func (r *runtime) soundClosed() {
	if r.sandbox != nil {
		r.sandbox.sounds--
	}
}

// audioError is err as JS sees it: goError's, with reason the audio error's name and, for a limit or a platform without audio, core's stable code. The sandbox's own quota has reason quota-exceeded.
func (r *runtime) audioError(err error) *js.Object {
	o := r.goError(err)
	var reason core.AudioError
	if !errors.As(err, &reason) {
		if core.ErrorCode(err) == core.CodeQuotaExceeded {
			_ = o.Set("reason", string(core.AudioQuotaExceeded))
		}
		return o
	}
	_ = o.Set("reason", string(reason))
	if core.ErrorCode(err) == "" {
		switch reason {
		case core.AudioQuotaExceeded:
			_ = o.Set("code", core.CodeQuotaExceeded)
		case core.AudioUnavailable:
			_ = o.Set("code", core.CodeUnavailable)
		}
	}
	return o
}

func (r *runtime) soundValue(s *core.Sound) *js.Object {
	o := r.vm.NewObject()
	closed := false
	_ = o.DefineAccessorProperty("duration", r.function(func(js.FunctionCall) js.Value {
		return r.vm.ToValue(float64(s.Duration().Microseconds()) / 1000)
	}), nil, js.FLAG_FALSE, js.FLAG_TRUE)
	_ = o.Set("play", r.function(func(call js.FunctionCall) js.Value {
		return r.playbackValue(s.Play(soundVoice(r.options(arg(call, 0)))))
	}))
	_ = o.Set("stop", r.function(func(js.FunctionCall) js.Value {
		s.Stop()
		return js.Undefined()
	}))
	_ = o.Set("close", r.function(func(js.FunctionCall) js.Value {
		if !closed {
			closed = true
			r.soundClosed()
		}
		s.Close()
		return js.Undefined()
	}))
	return o
}

// soundVoice reads a play's options, which core clamps, NaN taking the default. A volume of 0 or less is silence, where Go's SoundVoice takes 0 as the default: the playback starts muted, and setVolume can raise it.
func soundVoice(o opts) core.SoundVoice {
	v := core.SoundVoice{Rate: float32(o.float("rate", 1)), Pan: float32(o.float("pan", 0)), Loop: o.bool("loop", false), Muted: o.bool("muted", false)}
	if volume := o.float("volume", 1); volume <= 0 {
		v.Muted = true
	} else {
		v.Volume = float32(volume)
	}
	return v
}

func (r *runtime) playbackValue(p *core.Playback) *js.Object {
	o := r.vm.NewObject()
	_ = o.DefineAccessorProperty("ended", r.function(func(js.FunctionCall) js.Value {
		return r.vm.ToValue(p.Ended())
	}), nil, js.FLAG_FALSE, js.FLAG_TRUE)
	_ = o.Set("setVolume", r.function(func(call js.FunctionCall) js.Value {
		p.SetVolume(float32(arg(call, 0).ToFloat()))
		return js.Undefined()
	}))
	_ = o.Set("setRate", r.function(func(call js.FunctionCall) js.Value {
		p.SetRate(float32(arg(call, 0).ToFloat()))
		return js.Undefined()
	}))
	_ = o.Set("setPan", r.function(func(call js.FunctionCall) js.Value {
		p.SetPan(float32(arg(call, 0).ToFloat()))
		return js.Undefined()
	}))
	_ = o.Set("stop", r.function(func(js.FunctionCall) js.Value {
		p.Stop()
		return js.Undefined()
	}))
	return o
}

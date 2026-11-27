//go:build js && wasm

package miniprogram

import (
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func (s *surface) speak(it *driver.MountItem) {
	if it.Speak == nil {
		return
	}
	handleMiniProgramSpeak(s.bridge, it.Count, *it.Speak, s.engine.Dispatch)
}

// handleMiniProgramSpeak bridges ItemSpeak to the runtime's speaker (speak.js):
// WeChat's opt-in WechatSI plugin + InnerAudioContext when available, and
// SpeakUnavailable / empty voices elsewhere.
func handleMiniProgramSpeak(bridge js.Value, id uint32, r driver.SpeakRequest, dispatch func(driver.Event)) {
	switch r.Command {
	case driver.SpeakStart:
		requestMiniProgramSpeak(bridge, id, r, dispatch)
	case driver.SpeakStop:
		stopMiniProgramSpeak(bridge, id)
	case driver.SpeakVoices:
		requestMiniProgramVoices(bridge, id, dispatch)
	}
}

func requestMiniProgramSpeak(bridge js.Value, id uint32, r driver.SpeakRequest, dispatch func(driver.Event)) {
	if bridge.Get("speak").Type() != js.TypeFunction {
		dispatch(driver.Event{Kind: driver.EventSpeak, Count: id, Data: &driver.SpeakEvent{Error: driver.SpeakUnavailable}})
		return
	}
	var complete js.Func
	done := false
	complete = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if done {
			return nil
		}
		done = true
		defer complete.Release()
		ev := driver.SpeakEvent{}
		if res := first(args); res.Type() == js.TypeObject && !res.IsNull() {
			ev.Stopped = res.Get("stopped").Truthy()
			if e := res.Get("error"); e.Type() == js.TypeString && e.String() != "" {
				ev.Error = e.String()
				ev.Stopped = false
			}
		}
		dispatch(driver.Event{Kind: driver.EventSpeak, Count: id, Data: &ev})
		return nil
	})
	bridge.Call("speak", map[string]any{
		"id":    int(id),
		"text":  r.Text,
		"lang":  r.Lang,
		"voice": r.Voice,
		"rate":  float64(r.Rate),
		"pitch": float64(r.Pitch),
	}, complete)
}

func stopMiniProgramSpeak(bridge js.Value, id uint32) {
	if bridge.Get("stopSpeaking").Type() == js.TypeFunction {
		bridge.Call("stopSpeaking", int(id))
	}
}

func requestMiniProgramVoices(bridge js.Value, id uint32, dispatch func(driver.Event)) {
	var voices []driver.Voice
	if bridge.Get("voices").Type() == js.TypeFunction {
		list := bridge.Call("voices")
		if list.Type() == js.TypeObject && !list.IsNull() {
			n := list.Length()
			voices = make([]driver.Voice, 0, n)
			for i := range n {
				item := list.Index(i)
				if item.Type() != js.TypeObject || item.IsNull() {
					continue
				}
				voices = append(voices, driver.Voice{
					ID:   item.Get("id").String(),
					Name: item.Get("name").String(),
					Lang: item.Get("lang").String(),
				})
			}
		}
	}
	dispatch(driver.Event{Kind: driver.EventSpeak, Count: id, Data: &driver.SpeakEvent{Voices: voices}})
}

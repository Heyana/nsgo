//go:build js && wasm

package miniprogram

import (
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestMiniProgramSpeakWasmBridge(t *testing.T) {
	for _, tc := range []struct {
		name        string
		stopped     bool
		errCode     string
		wantStopped bool
		wantErr     string
	}{
		{"finished", false, "", false, ""},
		{"stopped", true, "", true, ""},
		{"language-unavailable", false, driver.SpeakLanguageUnavailable, false, driver.SpeakLanguageUnavailable},
		{"error-overrides-stopped", true, driver.SpeakFailed, false, driver.SpeakFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bridge := js.Global().Get("Object").New()
			speakFn := js.FuncOf(func(_ js.Value, args []js.Value) any {
				req := args[0]
				if req.Get("id").Int() != 42 || req.Get("text").String() != "你好" ||
					req.Get("lang").String() != "zh-CN" || req.Get("voice").String() != "WechatSI:zh_CN" ||
					req.Get("rate").Float() != 1.5 {
					t.Error("unexpected speak request payload")
				}
				args[1].Invoke(map[string]any{"stopped": tc.stopped, "error": tc.errCode})
				// Duplicate completion must be ignored.
				args[1].Invoke(map[string]any{"stopped": false, "error": driver.SpeakFailed})
				return nil
			})
			defer speakFn.Release()
			bridge.Set("speak", speakFn)

			answers := make(chan driver.Event, 2)
			handleMiniProgramSpeak(bridge, 42, driver.SpeakRequest{
				Command: driver.SpeakStart,
				Text:    "你好",
				Lang:    "zh-CN",
				Voice:   "WechatSI:zh_CN",
				Rate:    1.5,
				Pitch:   1,
			}, func(ev driver.Event) { answers <- ev })

			select {
			case ev := <-answers:
				report, ok := ev.Data.(*driver.SpeakEvent)
				if ev.Kind != driver.EventSpeak || ev.Count != 42 || !ok {
					t.Fatalf("unexpected event: %+v", ev)
				}
				if report.Stopped != tc.wantStopped || report.Error != tc.wantErr {
					t.Errorf("report = %+v, want stopped=%v err=%q", report, tc.wantStopped, tc.wantErr)
				}
			case <-time.After(time.Second):
				t.Fatal("missing EventSpeak")
			}
			if len(answers) != 0 {
				t.Fatalf("duplicate callbacks delivered: %d", len(answers))
			}
		})
	}

	t.Run("stop-and-voices", func(t *testing.T) {
		bridge := js.Global().Get("Object").New()
		stoppedID := 0
		stopFn := js.FuncOf(func(_ js.Value, args []js.Value) any {
			stoppedID = args[0].Int()
			return nil
		})
		defer stopFn.Release()
		voicesFn := js.FuncOf(func(js.Value, []js.Value) any {
			return []any{
				map[string]any{"id": "WechatSI:zh_CN", "name": "WechatSI (zh-CN)", "lang": "zh-CN"},
				map[string]any{"id": "WechatSI:en_US", "name": "WechatSI (en-US)", "lang": "en-US"},
			}
		})
		defer voicesFn.Release()
		bridge.Set("stopSpeaking", stopFn)
		bridge.Set("voices", voicesFn)

		handleMiniProgramSpeak(bridge, 77, driver.SpeakRequest{Command: driver.SpeakStop}, func(driver.Event) {
			t.Error("SpeakStop should not dispatch synchronously")
		})
		if stoppedID != 77 {
			t.Fatalf("stoppedID = %d, want 77", stoppedID)
		}

		var got driver.Event
		handleMiniProgramSpeak(bridge, 78, driver.SpeakRequest{Command: driver.SpeakVoices}, func(ev driver.Event) {
			got = ev
		})
		report, ok := got.Data.(*driver.SpeakEvent)
		if !ok || got.Kind != driver.EventSpeak || got.Count != 78 || len(report.Voices) != 2 {
			t.Fatalf("voices event = %+v", got)
		}
		if report.Voices[0].ID != "WechatSI:zh_CN" || report.Voices[1].Lang != "en-US" {
			t.Fatalf("voices = %+v", report.Voices)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		bridge := js.Global().Get("Object").New()
		var got driver.Event
		handleMiniProgramSpeak(bridge, 90, driver.SpeakRequest{Command: driver.SpeakStart, Text: "Hi"}, func(ev driver.Event) {
			got = ev
		})
		report, ok := got.Data.(*driver.SpeakEvent)
		if !ok || report.Error != driver.SpeakUnavailable {
			t.Fatalf("unavailable start = %+v", got)
		}

		got = driver.Event{}
		handleMiniProgramSpeak(bridge, 91, driver.SpeakRequest{Command: driver.SpeakVoices}, func(ev driver.Event) {
			got = ev
		})
		report, ok = got.Data.(*driver.SpeakEvent)
		if !ok || len(report.Voices) != 0 {
			t.Fatalf("unavailable voices = %+v", got)
		}
	})
}

//go:build ios && nsgotest

package watchos_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestWatchOSSpeakCapabilityAndStop verifies watchOS advertises CanSpeak and handles Voices, Speak and StopSpeaking through AVSpeechSynthesizer.
func TestWatchOSSpeakCapabilityAndStop(t *testing.T) {
	var (
		canSpeak   atomic.Bool
		voicesDone atomic.Bool
		speakDone  atomic.Bool
		stopped    atomic.Bool
		speakErr   atomic.Value
	)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		canSpeak.Store(cx.Env().CanSpeak())
		cx.Voices("", func([]core.VoiceInfo) {
			voicesDone.Store(true)
		})
		return core.View{
			TestID: "speak",
			Style:  core.Style{Width: core.Pt(40), Height: core.Pt(40)},
			OnPress: func(core.PressEvent) {
				if err := cx.Speak("Hello from Apple Watch", core.Voice{Lang: "en-US"}, func(wasStopped bool, err error) {
					stopped.Store(wasStopped)
					if err != nil {
						speakErr.Store(err)
					}
					speakDone.Store(true)
				}); err != nil {
					speakErr.Store(err)
					speakDone.Store(true)
					return
				}
				cx.StopSpeaking()
			},
		}
	})
	app.WaitFor("id:speak", 5*time.Second)
	if !canSpeak.Load() {
		t.Fatal("Env.CanSpeak() = false on watchOS, want true")
	}
	app.WaitUntil(5*time.Second, voicesDone.Load)
	app.Press("id:speak")
	app.WaitUntil(5*time.Second, speakDone.Load)
	if err, _ := speakErr.Load().(error); err != nil {
		t.Fatalf("Speak failed: %v", err)
	}
	if !stopped.Load() {
		t.Fatal("Speak was not reported stopped after StopSpeaking")
	}
}

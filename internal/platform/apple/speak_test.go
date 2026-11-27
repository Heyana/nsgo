//go:build darwin && !ios

package apple

import (
	"errors"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The synthesizer loads, lists the voices installed and finds one by language, by a bare language and by ID, and names what it lacks.
func TestSpeakVoices(t *testing.T) {
	if !loadSpeak() {
		t.Fatal("AVSpeechSynthesizer does not load")
	}
	voices := speakVoices()
	i := slices.IndexFunc(voices, func(v driver.Voice) bool { return strings.HasPrefix(v.Lang, "en-") })
	if i < 0 {
		t.Fatalf("no English voice among %d", len(voices))
	}
	if voices[i].ID == "" || voices[i].Name == "" {
		t.Fatalf("voice %+v", voices[i])
	}
	for _, lang := range []string{voices[i].Lang, "en"} {
		if v, code := speakVoice(lang, ""); v == 0 || code != "" {
			t.Errorf("speakVoice(%q) = %v, %q", lang, v, code)
		}
	}
	if v, code := speakVoice("", voices[i].ID); v == 0 || GoString(v.Send(sel("identifier"))) != voices[i].ID || code != "" {
		t.Errorf("speakVoice of ID %q = %v, %q", voices[i].ID, v, code)
	}
	if v, code := speakVoice("", "dev.nsgo.no-such-voice"); v != 0 || code != driver.SpeakVoiceUnavailable {
		t.Errorf("an unknown ID gives %v, %q", v, code)
	}
	if _, code := speakVoice("x-none", ""); code != driver.SpeakLanguageUnavailable {
		t.Errorf("an unknown language gives %q", code)
	}
}

// TestHandleSpeakLifecycleAndAudioSession verifies voices lookup, error codes, utterance rate/pitch setup, replacement, stop, delegate finish/cancel and balanced audio session acquire/release.
func TestHandleSpeakLifecycleAndAudioSession(t *testing.T) {
	if !loadSpeak() {
		t.Fatal("AVSpeechSynthesizer does not load")
	}
	voices := speakVoices()
	i := slices.IndexFunc(voices, func(v driver.Voice) bool { return strings.HasPrefix(v.Lang, "en-") })
	if i < 0 {
		t.Fatalf("no English voice among %d", len(voices))
	}
	voice := voices[i]

	oldLoaded := ensureSpeakLoaded
	oldAcquire := acquireSpeakSession
	oldSpeak := synthSpeak
	oldStop := synthStop
	t.Cleanup(func() {
		stopSpeaking()
		ensureSpeakLoaded = oldLoaded
		acquireSpeakSession = oldAcquire
		synthSpeak = oldSpeak
		synthStop = oldStop
	})

	var (
		acquires     int
		releases     int
		activeHolds  int
		sessionErr   error
		spokenIDs    []objc.ID
		stopCalls    int
		events       []driver.Event
		dispatch     = func(ev driver.Event) { events = append(events, ev) }
		resetCapture = func() {
			events = nil
			spokenIDs = nil
			stopCalls = 0
		}
	)
	acquireSpeakSession = func() (func(), error) {
		acquires++
		if sessionErr != nil {
			return nil, sessionErr
		}
		activeHolds++
		var once sync.Once
		return func() {
			once.Do(func() {
				releases++
				activeHolds--
			})
		}, nil
	}
	synthSpeak = func(u objc.ID) { spokenIDs = append(spokenIDs, u) }
	synthStop = func() { stopCalls++ }

	// SpeakVoices answers without acquiring the audio session.
	HandleSpeak(1, driver.SpeakRequest{Command: driver.SpeakVoices}, dispatch)
	if acquires != 0 || activeHolds != 0 {
		t.Fatalf("SpeakVoices touched audio session: acquires=%d active=%d", acquires, activeHolds)
	}
	if len(events) != 1 || events[0].Kind != driver.EventSpeak || events[0].Count != 1 || len(events[0].Data.(*driver.SpeakEvent).Voices) == 0 {
		t.Fatalf("SpeakVoices events = %+v", events)
	}

	// Unknown voice and unknown language fail before acquiring the audio session.
	resetCapture()
	HandleSpeak(2, driver.SpeakRequest{Command: driver.SpeakStart, Text: "hi", Voice: "dev.nsgo.no-such-voice"}, dispatch)
	HandleSpeak(3, driver.SpeakRequest{Command: driver.SpeakStart, Text: "hi", Lang: "x-none"}, dispatch)
	if acquires != 0 || activeHolds != 0 || len(spokenIDs) != 0 {
		t.Fatalf("failed voice lookup acquired session or spoke: acquires=%d spoken=%d", acquires, len(spokenIDs))
	}
	if len(events) != 2 || events[0].Data.(*driver.SpeakEvent).Error != driver.SpeakVoiceUnavailable || events[1].Data.(*driver.SpeakEvent).Error != driver.SpeakLanguageUnavailable {
		t.Fatalf("voice/lang error events = %+v", events)
	}

	// Audio session failure reports SpeakAudioBusy without starting synthesis.
	resetCapture()
	sessionErr = errors.New("busy")
	HandleSpeak(4, driver.SpeakRequest{Command: driver.SpeakStart, Text: "hi", Lang: voice.Lang, Voice: voice.ID, Rate: 1, Pitch: 1}, dispatch)
	sessionErr = nil
	if activeHolds != 0 || len(spokenIDs) != 0 || speaking.run != nil {
		t.Fatalf("busy session leaked state: active=%d spoken=%d run=%v", activeHolds, len(spokenIDs), speaking.run)
	}
	if len(events) != 1 || events[0].Count != 4 || events[0].Data.(*driver.SpeakEvent).Error != driver.SpeakAudioBusy {
		t.Fatalf("busy session events = %+v", events)
	}

	// Start -> finish releases the audio session once and ignores duplicate delegate callbacks.
	resetCapture()
	acquires, releases = 0, 0
	HandleSpeak(5, driver.SpeakRequest{Command: driver.SpeakStart, Text: "hello", Lang: voice.Lang, Voice: voice.ID, Rate: 1.5, Pitch: 1.25}, dispatch)
	if acquires != 1 || activeHolds != 1 || len(spokenIDs) != 1 || speaking.run == nil {
		t.Fatalf("start state: acquires=%d active=%d spoken=%d run=%v", acquires, activeHolds, len(spokenIDs), speaking.run)
	}
	u := spokenIDs[0]
	wantRate := driver.SpeakScale(1.5, avSpeechDefaultRate/2, avSpeechDefaultRate, 1)
	if got := objc.Send[float32](u, sel("rate")); math.Abs(float64(got-wantRate)) > 1e-4 {
		t.Errorf("utterance rate = %v, want %v", got, wantRate)
	}
	if got := objc.Send[float32](u, sel("pitchMultiplier")); math.Abs(float64(got-1.25)) > 1e-4 {
		t.Errorf("utterance pitch = %v, want 1.25", got)
	}
	spoken(u, false)
	spoken(u, false) // stale duplicate must be ignored
	if releases != 1 || activeHolds != 0 || speaking.run != nil {
		t.Fatalf("after finish: releases=%d active=%d run=%v", releases, activeHolds, speaking.run)
	}
	if len(events) != 1 || events[0].Count != 5 || events[0].Data.(*driver.SpeakEvent).Stopped || events[0].Data.(*driver.SpeakEvent).Error != "" {
		t.Fatalf("finish events = %+v", events)
	}

	// Start -> wrong-ID stop is ignored -> matching-ID stop releases session and reports stopped.
	resetCapture()
	acquires, releases = 0, 0
	HandleSpeak(6, driver.SpeakRequest{Command: driver.SpeakStart, Text: "first", Lang: voice.Lang}, dispatch)
	HandleSpeak(999, driver.SpeakRequest{Command: driver.SpeakStop}, dispatch)
	if activeHolds != 1 || stopCalls != 0 || len(events) != 0 {
		t.Fatalf("mismatched stop interrupted utterance: active=%d stops=%d events=%+v", activeHolds, stopCalls, events)
	}
	HandleSpeak(6, driver.SpeakRequest{Command: driver.SpeakStop}, dispatch)
	if releases != 1 || activeHolds != 0 || stopCalls != 1 || speaking.run != nil {
		t.Fatalf("after stop: releases=%d active=%d stops=%d run=%v", releases, activeHolds, stopCalls, speaking.run)
	}
	if len(events) != 1 || events[0].Count != 6 || !events[0].Data.(*driver.SpeakEvent).Stopped {
		t.Fatalf("stop events = %+v", events)
	}

	// Replacement stops utterance #7 (releasing its hold) before acquiring #8, and delegate cancel releases #8.
	resetCapture()
	acquires, releases = 0, 0
	HandleSpeak(7, driver.SpeakRequest{Command: driver.SpeakStart, Text: "one", Lang: voice.Lang}, dispatch)
	HandleSpeak(8, driver.SpeakRequest{Command: driver.SpeakStart, Text: "two", Lang: voice.Lang}, dispatch)
	if acquires != 2 || releases != 1 || activeHolds != 1 || stopCalls != 1 || len(spokenIDs) != 2 {
		t.Fatalf("after replacement: acquires=%d releases=%d active=%d stops=%d spoken=%d", acquires, releases, activeHolds, stopCalls, len(spokenIDs))
	}
	if len(events) != 1 || events[0].Count != 7 || !events[0].Data.(*driver.SpeakEvent).Stopped {
		t.Fatalf("replacement stop event = %+v", events)
	}
	spoken(spokenIDs[1], true)
	if releases != 2 || activeHolds != 0 || speaking.run != nil {
		t.Fatalf("after cancel: releases=%d active=%d run=%v", releases, activeHolds, speaking.run)
	}
	if len(events) != 2 || events[1].Count != 8 || !events[1].Data.(*driver.SpeakEvent).Stopped {
		t.Fatalf("cancel events = %+v", events)
	}

	// Unavailable synthesizer fails start and voices without touching the session, and ignores stop.
	resetCapture()
	acquires, releases = 0, 0
	ensureSpeakLoaded = func() bool { return false }
	HandleSpeak(9, driver.SpeakRequest{Command: driver.SpeakStart, Text: "hi", Lang: voice.Lang}, dispatch)
	HandleSpeak(10, driver.SpeakRequest{Command: driver.SpeakVoices}, dispatch)
	HandleSpeak(11, driver.SpeakRequest{Command: driver.SpeakStop}, dispatch)
	if acquires != 0 || activeHolds != 0 {
		t.Fatalf("unavailable synth touched session: acquires=%d active=%d", acquires, activeHolds)
	}
	if len(events) != 2 || events[0].Data.(*driver.SpeakEvent).Error != driver.SpeakUnavailable || events[1].Data.(*driver.SpeakEvent).Error != driver.SpeakUnavailable {
		t.Fatalf("unavailable events = %+v", events)
	}
}

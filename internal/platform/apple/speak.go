//go:build darwin

package apple

import (
	"log"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Speaking speaks text for macOS, iOS, tvOS and watchOS with AVSpeechSynthesizer: one synthesizer in the process, as there is one audio output, which speaks one utterance at a time, a new one stopping the one before. Its state is the main thread's; the delegate's reports hop there.
var speaking struct {
	synthesizer objc.ID
	// run is the utterance being spoken, nil for none.
	run *speakRun
}

// speakRun is an utterance being spoken and where it reports.
type speakRun struct {
	id        uint32
	dispatch  func(driver.Event)
	utterance objc.ID
	leave     func()
}

const (
	// avSpeechDefaultRate is AVSpeechUtteranceDefaultSpeechRate, between AVSpeechUtteranceMinimumSpeechRate's 0 and Maximum's 1.
	avSpeechDefaultRate = 0.5
	avfaudioPath        = "/System/Library/Frameworks/AVFAudio.framework/AVFAudio"
)

// Test seams for verifying session acquisition and synthesizer lifecycle without playing audio on test runners.
var (
	ensureSpeakLoaded   = func() bool { return loadSpeak() }
	acquireSpeakSession = holdSpeakSession
	synthSpeak          = func(u objc.ID) { speaking.synthesizer.Send(sel("speakUtterance:"), u) }
	synthStop           = func() { speaking.synthesizer.Send(sel("stopSpeakingAtBoundary:"), 0) } // AVSpeechBoundaryImmediate
)

// SpeakAvailable is Environment.Speak: every macOS, iOS, tvOS and watchOS NSGo runs on has AVSpeechSynthesizer, which loads only once the app speaks.
func SpeakAvailable() bool { return true }

// loadSpeak loads AVFoundation/AVFAudio and makes the synthesizer and its delegate, reporting whether it could.
var loadSpeak = sync.OnceValue(func() bool {
	if _, err := purego.Dlopen(avPath, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		if _, err := purego.Dlopen(avfaudioPath, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			return false
		}
	}
	if Class("AVSpeechSynthesizer") == 0 || Class("AVSpeechUtterance") == 0 || Class("AVSpeechSynthesisVoice") == 0 {
		if _, err := purego.Dlopen(avfaudioPath, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			return false
		}
		if Class("AVSpeechSynthesizer") == 0 || Class("AVSpeechUtterance") == 0 || Class("AVSpeechSynthesisVoice") == 0 {
			return false
		}
	}
	var protocols []*objc.Protocol
	if p := objc.GetProtocol("AVSpeechSynthesizerDelegate"); p != nil {
		protocols = append(protocols, p)
	}
	ended := func(stopped bool) func(objc.ID, objc.SEL, objc.ID, objc.ID) {
		return func(_ objc.ID, _ objc.SEL, _, utterance objc.ID) {
			OnMain(func() { spoken(utterance, stopped) })
		}
	}
	delegate := objc.ID(RegisterClass("NsgoSpeechSynthesizerDelegate", Class("NSObject"), protocols, []objc.MethodDef{
		{Cmd: sel("speechSynthesizer:didFinishSpeechUtterance:"), Fn: ended(false)},
		{Cmd: sel("speechSynthesizer:didCancelSpeechUtterance:"), Fn: ended(true)},
	})).Send(sel("new"))
	speaking.synthesizer = objc.ID(Class("AVSpeechSynthesizer")).Send(sel("new"))
	if speaking.synthesizer == 0 {
		return false
	}
	if objc.Send[bool](speaking.synthesizer, sel("respondsToSelector:"), sel("setUsesApplicationAudioSession:")) {
		speaking.synthesizer.Send(sel("setUsesApplicationAudioSession:"), true)
	}
	speaking.synthesizer.Send(sel("setDelegate:"), delegate)
	return true
})

// HandleSpeak carries out an ItemSpeak of utterance or request id, on the main thread, reporting EventSpeak through dispatch.
func HandleSpeak(id uint32, r driver.SpeakRequest, dispatch func(driver.Event)) {
	report := func(e driver.SpeakEvent) { dispatch(driver.Event{Kind: driver.EventSpeak, Count: id, Data: &e}) }
	if !ensureSpeakLoaded() {
		if r.Command != driver.SpeakStop {
			// No synthesizer: an utterance fails, and there are no voices.
			report(driver.SpeakEvent{Error: driver.SpeakUnavailable})
		}
		return
	}
	switch r.Command {
	case driver.SpeakStart:
		stopSpeaking()
		voice, code := speakVoice(r.Lang, r.Voice)
		if voice == 0 {
			report(driver.SpeakEvent{Error: code})
			return
		}
		leave, err := acquireSpeakSession()
		if err != nil {
			report(driver.SpeakEvent{Error: driver.SpeakAudioBusy})
			return
		}
		u := objc.ID(Class("AVSpeechUtterance")).Send(sel("speechUtteranceWithString:"), NSString(r.Text)).Send(sel("retain"))
		if u == 0 {
			if leave != nil {
				leave()
			}
			report(driver.SpeakEvent{Error: driver.SpeakFailed})
			return
		}
		u.Send(sel("setVoice:"), voice)
		u.Send(sel("setRate:"), driver.SpeakScale(r.Rate, avSpeechDefaultRate/2, avSpeechDefaultRate, 1))
		u.Send(sel("setPitchMultiplier:"), driver.SpeakScale(r.Pitch, 0.5, 1, 2))
		speaking.run = &speakRun{id: id, dispatch: dispatch, utterance: u, leave: leave}
		synthSpeak(u)
	case driver.SpeakStop:
		if run := speaking.run; run != nil && run.id == id {
			stopSpeaking()
		}
	case driver.SpeakVoices:
		report(driver.SpeakEvent{Voices: speakVoices()})
	}
}

// stopSpeaking stops the utterance being spoken, which reports itself stopped.
func stopSpeaking() {
	run := speaking.run
	if run == nil {
		return
	}
	// The delegate hears the cancel later, of an utterance no longer the one running: the run ends here.
	speaking.run = nil
	synthStop()
	run.end(driver.SpeakEvent{Stopped: true})
}

// spoken is the delegate's report that utterance finished, or was stopped.
func spoken(utterance objc.ID, stopped bool) {
	run := speaking.run
	if run == nil || run.utterance != utterance {
		return
	}
	speaking.run = nil
	run.end(driver.SpeakEvent{Stopped: stopped})
}

// end reports the run's end and lets its utterance and audio session go.
func (run *speakRun) end(e driver.SpeakEvent) {
	run.utterance.Send(sel("release"))
	if run.leave != nil {
		run.leave()
	}
	run.dispatch(driver.Event{Kind: driver.EventSpeak, Count: run.id, Data: &e})
}

// speakVoice is the voice of id, or the default one of lang, else 0 and why.
func speakVoice(lang, id string) (objc.ID, string) {
	class := objc.ID(Class("AVSpeechSynthesisVoice"))
	if id != "" {
		// Some macOS versions return a default voice for an unknown ID; an explicit choice must resolve to that voice, not a fallback.
		if v := class.Send(sel("voiceWithIdentifier:"), NSString(id)); v != 0 && GoString(v.Send(sel("identifier"))) == id {
			return v, ""
		}
		log.Printf("nsgo: speak: no voice %q", id)
		return 0, driver.SpeakVoiceUnavailable
	}
	if v := class.Send(sel("voiceWithLanguage:"), NSString(lang)); v != 0 {
		return v, ""
	}
	// voiceWithLanguage: knows "en-US", not "en": the first voice of a tag under lang speaks it.
	all := class.Send(sel("speechVoices"))
	prefix := strings.ToLower(lang) + "-"
	for i := range objc.Send[uint](all, sel("count")) {
		v := all.Send(sel("objectAtIndex:"), i)
		if strings.HasPrefix(strings.ToLower(GoString(v.Send(sel("language")))), prefix) {
			return v, ""
		}
	}
	return 0, driver.SpeakLanguageUnavailable
}

// speakVoices lists the voices installed.
func speakVoices() []driver.Voice {
	all := objc.ID(Class("AVSpeechSynthesisVoice")).Send(sel("speechVoices"))
	n := objc.Send[uint](all, sel("count"))
	voices := make([]driver.Voice, 0, n)
	for i := range n {
		v := all.Send(sel("objectAtIndex:"), i)
		voices = append(voices, driver.Voice{ID: GoString(v.Send(sel("identifier"))), Name: GoString(v.Send(sel("name"))),
			Lang: GoString(v.Send(sel("language")))})
	}
	return voices
}

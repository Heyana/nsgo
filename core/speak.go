package core

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// SpeakError is why an utterance failed, named as the Web Speech API's SpeechSynthesisErrorEvent's errors.
type SpeakError string

const (
	// SpeakNotAllowed: the current owner's Access refuses AccessTextToSpeech, or the platform refused to speak, as a browser does before the user's first gesture.
	SpeakNotAllowed SpeakError = driver.SpeakNotAllowed
	// SpeakLanguageUnavailable: no voice installed speaks the language.
	SpeakLanguageUnavailable SpeakError = driver.SpeakLanguageUnavailable
	// SpeakVoiceUnavailable: the platform has no voice of Voice.ID.
	SpeakVoiceUnavailable SpeakError = driver.SpeakVoiceUnavailable
	// SpeakTextTooLong: the text is longer than the platform speaks at once, as Android's engines' limit, 4000 characters by default.
	SpeakTextTooLong SpeakError = driver.SpeakTextTooLong
	// SpeakAudioBusy: the platform's audio output could not be had, such as iOS's audio session.
	SpeakAudioBusy SpeakError = driver.SpeakAudioBusy
	// SpeakUnavailable: this platform speaks no text (Env.CanSpeak).
	SpeakUnavailable SpeakError = driver.SpeakUnavailable
	// SpeakFailed: the synthesizer failed otherwise.
	SpeakFailed SpeakError = driver.SpeakFailed
)

func (e SpeakError) Error() string { return "nsgo: speak: " + string(e) }

// Voice is how Speak speaks: in the language Lang, a BCP 47 tag ("en-US", "ja-JP", "zh-TW"), "" being the app's locale; in the voice ID names, one of Voices' IDs, "" being the platform's default voice for Lang; at Rate and Pitch, from 0.5 to 2 with 1 the voice's own, 0 standing for 1 and other values clamped.
type Voice struct {
	Lang, ID    string
	Rate, Pitch float32
}

// VoiceInfo is a voice installed on the platform: the ID Voice takes, the name the platform gives it and the BCP 47 tag of its language.
type VoiceInfo = driver.Voice

// utterance is text being spoken.
type utterance struct {
	tree  *Tree
	owner *Scope
	id    uint32
	done  func(stopped bool, err error)
	ended bool
}

// Speak speaks text aloud in voice, one utterance at a time in an engine: a new one stops the one speaking, and so do StopSpeaking and the end of the scope that spoke it. done, which may be nil, runs once, after Speak returns, when the utterance finished, with stopped when it was stopped first, or with a SpeakError when it failed. macOS, iOS, tvOS and watchOS speak with AVSpeechSynthesizer, Android with TextToSpeech, Windows with SpeechSynthesizer, Linux with speech-dispatcher and the web with speechSynthesis; elsewhere it fails with SpeakUnavailable (Env.CanSpeak). It returns an error only for a voice's Lang that is no language tag, without calling done.
func (cx *Scope) Speak(text string, voice Voice, done func(stopped bool, err error)) error {
	if voice.Lang != "" && !languageTag(voice.Lang) {
		return fmt.Errorf("nsgo: speak: %q is no language tag", voice.Lang)
	}
	rt, t := cx.rt, cx.tree()
	if rt.speaking != nil {
		rt.speaking.stop()
	}
	if t == nil {
		// No surface to speak through, as a scope of a window that closed.
		if done != nil {
			cx.Post(func() { done(false, SpeakUnavailable) })
		}
		return nil
	}
	t.alertID++
	u := &utterance{tree: t, owner: cx.current(), id: t.alertID, done: done}
	// An utterance that fails before it starts ends on its own, so the platform hears nothing of it.
	fail := func(err error) { cx.Post(func() { u.end(false, err) }) }
	switch {
	case cx.denied(serviceSpeak, ""):
		fail(SpeakNotAllowed)
		return nil
	case !Untracked(cx, cx.Env().CanSpeak):
		fail(SpeakUnavailable)
		return nil
	case strings.TrimSpace(text) == "":
		// Nothing to say: it finishes at once, as the web's does.
		fail(nil)
		return nil
	}
	rt.speaking = u
	u.owner.addCleanup(u.stop)
	lang := voice.Lang
	if lang == "" {
		lang = Untracked(cx, cx.Env().Locale)
	}
	u.send(driver.SpeakRequest{Command: driver.SpeakStart, Text: text, Lang: lang, Voice: voice.ID,
		Rate: speakLevel(voice.Rate), Pitch: speakLevel(voice.Pitch)})
	return nil
}

// StopSpeaking stops the utterance the engine speaks, whose done hears it stopped; it does nothing when none is spoken.
func (cx *Scope) StopSpeaking() {
	if u := cx.rt.speaking; u != nil {
		u.stop()
	}
}

// Voices answers with the voices installed that speak lang, a BCP 47 tag that matches itself and the tags under it ("en" matches "en-US" and "en-GB"), or with every voice when lang is "", sorted by language and name, so an app can tell the user to install a voice it lacks. Where Env.CanSpeak is false, or the current owner's Access refuses AccessTextToSpeech, it answers with none without asking the platform.
func (cx *Scope) Voices(lang string, done func([]VoiceInfo)) {
	if done == nil {
		return
	}
	t := cx.tree()
	if t == nil || cx.denied(serviceSpeak, "") || !Untracked(cx, cx.Env().CanSpeak) {
		done(nil)
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemSpeak, Speak: &driver.SpeakRequest{Command: driver.SpeakVoices}}, func(ev driver.Event) {
		var voices []VoiceInfo
		if report, ok := ev.Data.(*driver.SpeakEvent); ok {
			for _, v := range report.Voices {
				v.Lang = driver.LanguageTag(v.Lang)
				if lang == "" || speaksLanguage(v.Lang, lang) {
					voices = append(voices, v)
				}
			}
		}
		slices.SortStableFunc(voices, func(a, b VoiceInfo) int {
			return cmp.Or(cmp.Compare(strings.ToLower(a.Lang), strings.ToLower(b.Lang)), cmp.Compare(a.Name, b.Name))
		})
		done(voices)
	})
}

// speaksLanguage reports whether a voice of tag speaks lang: the same tag, or one under it, case and '_' aside.
func speaksLanguage(tag, lang string) bool {
	tag, lang = strings.ToLower(tag), strings.ToLower(driver.LanguageTag(lang))
	return tag == lang || strings.HasPrefix(tag, lang+"-")
}

// speakLevel is a rate or pitch the platform takes: 0 is 1, and the rest is clamped to 0.5 to 2.
func speakLevel(v float32) float32 {
	if v == 0 || v != v {
		return 1
	}
	return min(max(v, 0.5), 2)
}

func (u *utterance) send(r driver.SpeakRequest) {
	u.tree.commands = append(u.tree.commands, driver.MountItem{Kind: driver.ItemSpeak, Count: u.id, Speak: &r})
	u.tree.changed = true
}

// stop stops the utterance, telling the platform, and ends it stopped.
func (u *utterance) stop() {
	if u.ended {
		return
	}
	u.send(driver.SpeakRequest{Command: driver.SpeakStop})
	u.end(true, nil)
}

// end ends the utterance, once, running done under its owner, or the root once that is gone.
func (u *utterance) end(stopped bool, err error) {
	if u.ended {
		return
	}
	u.ended = true
	if rt := u.tree.rt; rt.speaking == u {
		rt.speaking = nil
	}
	if u.done == nil {
		return
	}
	owner := u.owner
	if owner.disposed {
		owner = u.tree.root
	}
	u.tree.within(owner, func() { u.done(stopped, err) })
}

// speakEvent hands a platform's report to the utterance it ends, or to the request for voices it answers.
func (t *Tree) speakEvent(ev driver.Event) {
	if u := t.rt.speaking; u != nil && u.tree == t && u.id == ev.Count {
		report, _ := ev.Data.(*driver.SpeakEvent)
		if report == nil {
			report = &driver.SpeakEvent{}
		}
		var err error
		if report.Error != "" {
			err = SpeakError(report.Error)
		}
		u.end(report.Stopped && err == nil, err)
		return
	}
	t.answer(ev)
}

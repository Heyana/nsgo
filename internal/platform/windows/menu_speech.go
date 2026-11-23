//go:build windows

package windows

import (
	"log"
	"strings"
	"sync/atomic"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// menuUtterance numbers what the Speech menu's Start Speaking says. Core numbers its utterances from 1, so the synthesizer's run of 0 is the menu's, which reports its end to no engine.
const menuUtterance = 0

// menuSpeech counts the Speech menu's actions in every window, so that a focused page's selection, which comes later, is said only for the last one (speakSelection).
var menuSpeech atomic.Uint64

// speakMenu carries out Start Speaking, which reads the focused field's selection, or its whole text without one, or a focused web view's selection, in a voice for the app's language or else Windows' default one, stopping whatever speaks, and Stop Speaking, which stops what Start Speaking said, from any window.
func (w *window) speakMenu(start bool) {
	n := menuSpeech.Add(1)
	if !start {
		if run := speaking.run; run != nil && run.id == menuUtterance {
			stopSpeaking()
		}
		return
	}
	if wv := w.focusedWeb(); wv != nil {
		wv.speakSelection(n)
		return
	}
	menuSay(w, w.focusedField().speechText())
}

// menuSay is what Start Speaking says through; tests hear it instead.
var menuSay = (*window).sayForMenu

// sayForMenu speaks text as the menu's utterance, stopping whatever speaks.
func (w *window) sayForMenu(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	stopSpeaking()
	// A voice for the app's language, else Windows' default voice, which reads the text whatever its language, as macOS's does.
	code := w.startSpeaking(menuUtterance, driver.SpeakRequest{Command: driver.SpeakStart, Lang: w.env.Locale, Text: text})
	if code == driver.SpeakLanguageUnavailable {
		code = w.startSpeaking(menuUtterance, driver.SpeakRequest{Command: driver.SpeakStart, Text: text})
	}
	if code != "" {
		log.Printf("nsgo: Start Speaking: %s", code)
	}
}

// speechText is what Start Speaking reads of the field: its selection, or its whole text without one, and nothing of a password field or of none.
func (in *textInput) speechText() string {
	if in == nil || in.masked() {
		return ""
	}
	if in.model.hasSelection() {
		return in.model.selected()
	}
	return in.model.text
}

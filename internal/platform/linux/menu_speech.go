//go:build linux && !android

package linux

import (
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/speechd"
)

// menuUtterance numbers what the Speech menu's Start Speaking says. Core numbers its utterances from 1, so speech-dispatcher's utterance 0 is the menu's, whose end no engine hears.
const menuUtterance = 0

// menuSpeak hands the menu's requests to speech-dispatcher; tests hear them instead.
var menuSpeak = speechd.Handle

// menuSpeech counts the Speech menu's actions, so that a focused page's selection, which comes later, is said only for the last one (speakWebSelection).
var menuSpeech uint64

// speakMenu carries out Start Speaking, which reads the focused field's selection, or its whole text without one, or a focused web view's selection, stopping whatever speaks, and Stop Speaking, which stops what Start Speaking said.
func (w *window) speakMenu(start bool) {
	menuSpeech++
	if !start {
		menuSpeak(menuUtterance, driver.SpeakRequest{Command: driver.SpeakStop}, func(driver.Event) {})
		return
	}
	if v := w.focusedWeb(); v != nil {
		w.speakWebSelection(v, menuSpeech)
		return
	}
	v := w.focusedEntry()
	if v == nil || v.input.Masked && !v.input.Multiline {
		return
	}
	s, e := w.selection(v)
	w.sayForMenu(speechText(w.editorText(v), s, e))
}

// sayForMenu speaks text as the menu's utterance, in a voice for the app's language.
func (w *window) sayForMenu(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	lang := w.env.Locale
	if lang == "" {
		lang = "en-US"
	}
	menuSpeak(menuUtterance, driver.SpeakRequest{Command: driver.SpeakStart, Lang: lang, Text: text}, func(driver.Event) {})
}

// speechText is what Start Speaking reads of a field's text: the characters from start to end, GTK's offsets, or the whole text when they select none.
func speechText(text string, start, end int) string {
	if start > end {
		start, end = end, start
	}
	if start == end || start < 0 {
		return text
	}
	return text[byteOfChar(text, start):byteOfChar(text, end)]
}

// byteOfChar is the byte offset of character n of text, its length past the end.
func byteOfChar(text string, n int) int {
	for b := range text {
		if n == 0 {
			return b
		}
		n--
	}
	return len(text)
}

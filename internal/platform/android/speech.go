package android

import "github.com/nsgo-dev/nsgo/internal/driver"

// writeSpeechRequest encodes an ItemSpeech's fields in the order NsgoView.apply reads them.
func writeSpeechRequest(w *writer, count uint32, r driver.SpeechRequest) {
	w.u32(count)
	w.byte(byte(r.Command))
	w.str(r.Lang)
	w.flags(r.Interim)
}

// readSpeech reads what Speech.java's EventSpeech reports: its phase as the index, its transcript as the text, whether it is final as Finished, and its error as data.
func readSpeech(ev driver.Event, data string) *driver.SpeechEvent {
	return &driver.SpeechEvent{
		Phase: driver.SpeechPhase(ev.Index),
		Text:  ev.Text,
		Final: ev.Finished,
		Error: data,
	}
}

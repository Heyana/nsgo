package watchos

import "github.com/nsgo-dev/nsgo/internal/driver"

// dispatchWatchSpeak routes an ItemSpeak command on watchOS to Apple's AVSpeechSynthesizer handler, answering unavailable when no handler is wired.
func dispatchWatchSpeak(it *driver.MountItem, handle func(uint32, driver.SpeakRequest, func(driver.Event)), dispatch func(driver.Event)) {
	if it == nil || it.Speak == nil {
		return
	}
	if handle == nil {
		if it.Speak.Command != driver.SpeakStop {
			dispatch(driver.Event{Kind: driver.EventSpeak, Count: it.Count, Data: &driver.SpeakEvent{Error: driver.SpeakUnavailable}})
		}
		return
	}
	handle(it.Count, *it.Speak, dispatch)
}

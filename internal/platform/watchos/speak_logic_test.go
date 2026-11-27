package watchos

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWatchSpeakDispatch(t *testing.T) {
	type call struct {
		id  uint32
		req driver.SpeakRequest
	}
	var calls []call
	var events []driver.Event
	dispatch := func(ev driver.Event) { events = append(events, ev) }

	handle := func(id uint32, r driver.SpeakRequest, report func(driver.Event)) {
		calls = append(calls, call{id: id, req: r})
		switch r.Command {
		case driver.SpeakStart:
			report(driver.Event{Kind: driver.EventSpeak, Count: id, Data: &driver.SpeakEvent{}})
		case driver.SpeakStop:
			report(driver.Event{Kind: driver.EventSpeak, Count: id, Data: &driver.SpeakEvent{Stopped: true}})
		case driver.SpeakVoices:
			report(driver.Event{Kind: driver.EventSpeak, Count: id, Data: &driver.SpeakEvent{
				Voices: []driver.Voice{{ID: "com.apple.voice.compact.en-US.Samantha", Name: "Samantha", Lang: "en-US"}},
			}})
		}
	}

	startReq := driver.SpeakRequest{Command: driver.SpeakStart, Text: "Hello watch", Lang: "en-US", Rate: 1.2, Pitch: 0.9}
	stopReq := driver.SpeakRequest{Command: driver.SpeakStop}
	voicesReq := driver.SpeakRequest{Command: driver.SpeakVoices}

	dispatchWatchSpeak(&driver.MountItem{Kind: driver.ItemSpeak, Count: 1, Speak: &startReq}, handle, dispatch)
	dispatchWatchSpeak(&driver.MountItem{Kind: driver.ItemSpeak, Count: 1, Speak: &stopReq}, handle, dispatch)
	dispatchWatchSpeak(&driver.MountItem{Kind: driver.ItemSpeak, Count: 2, Speak: &voicesReq}, handle, dispatch)
	// Nil item or nil request is ignored.
	dispatchWatchSpeak(nil, handle, dispatch)
	dispatchWatchSpeak(&driver.MountItem{Kind: driver.ItemSpeak, Count: 3}, handle, dispatch)

	if len(calls) != 3 {
		t.Fatalf("calls = %d, want 3", len(calls))
	}
	if calls[0].id != 1 || calls[0].req != startReq {
		t.Fatalf("call[0] = %+v, want id 1 req %+v", calls[0], startReq)
	}
	if calls[1].id != 1 || calls[1].req.Command != driver.SpeakStop {
		t.Fatalf("call[1] = %+v, want id 1 SpeakStop", calls[1])
	}
	if calls[2].id != 2 || calls[2].req.Command != driver.SpeakVoices {
		t.Fatalf("call[2] = %+v, want id 2 SpeakVoices", calls[2])
	}
	if len(events) != 3 || events[0].Count != 1 || !events[1].Data.(*driver.SpeakEvent).Stopped || len(events[2].Data.(*driver.SpeakEvent).Voices) != 1 {
		t.Fatalf("events = %+v", events)
	}

	// Nil handler answers unavailable for start and voices, and sends nothing for stop.
	events = nil
	dispatchWatchSpeak(&driver.MountItem{Kind: driver.ItemSpeak, Count: 4, Speak: &startReq}, nil, dispatch)
	dispatchWatchSpeak(&driver.MountItem{Kind: driver.ItemSpeak, Count: 5, Speak: &voicesReq}, nil, dispatch)
	dispatchWatchSpeak(&driver.MountItem{Kind: driver.ItemSpeak, Count: 6, Speak: &stopReq}, nil, dispatch)
	if len(events) != 2 {
		t.Fatalf("nil handler events = %d, want 2: %+v", len(events), events)
	}
	for i, wantID := range []uint32{4, 5} {
		rep, ok := events[i].Data.(*driver.SpeakEvent)
		if !ok || events[i].Kind != driver.EventSpeak || events[i].Count != wantID || rep.Error != driver.SpeakUnavailable {
			t.Fatalf("event[%d] = %+v, want EventSpeak(%d, SpeakUnavailable)", i, events[i], wantID)
		}
	}
}

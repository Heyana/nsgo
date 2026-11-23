package android

import (
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// withoutAudioLatencyReads answers each AudioReadLatency in b at once with an unknown latency, through dispatch, and returns b without them, a copy when it had any, as a test's observer may hold b. Android has no public API for the output route's latency (AudioTrack.getLatency is hidden), and Audio.java takes no such command: it would end the batch with an unknown audio command.
func withoutAudioLatencyReads(b *driver.MountBatch, dispatch func(driver.Event)) *driver.MountBatch {
	read := func(it driver.MountItem) bool {
		return it.Kind == driver.ItemAudio && it.Audio != nil && it.Audio.Command == driver.AudioReadLatency
	}
	if !slices.ContainsFunc(b.Items, read) {
		return b
	}
	for _, it := range b.Items {
		if read(it) {
			dispatch(driver.Event{Kind: driver.EventAudio, Count: it.Count, Data: &driver.AudioEvent{Phase: driver.AudioLatency}})
		}
	}
	out := *b
	out.Items = slices.DeleteFunc(slices.Clone(b.Items), read)
	return &out
}

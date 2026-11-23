package android

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestAudioLatencyReadsAnswerUnknownWithoutReachingJava(t *testing.T) {
	b := &driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemAudio, Count: 4, Audio: &driver.AudioRequest{Command: driver.AudioReadLatency}},
		{Kind: driver.ItemAudio, Count: 5, Audio: &driver.AudioRequest{Command: driver.AudioPlay}},
	}}
	var answers []driver.Event
	out := withoutAudioLatencyReads(b, func(ev driver.Event) { answers = append(answers, ev) })
	if len(out.Items) != 1 || out.Items[0].Count != 5 || len(b.Items) != 2 {
		t.Fatalf("batch after the latency read = %+v, given %+v", out.Items, b.Items)
	}
	if len(answers) != 1 || answers[0].Kind != driver.EventAudio || answers[0].Count != 4 {
		t.Fatalf("answers = %+v", answers)
	}
	if e, _ := answers[0].Data.(*driver.AudioEvent); e == nil || e.Phase != driver.AudioLatency || e.Latency != 0 {
		t.Fatalf("latency answer = %+v, want an unknown AudioLatency", answers[0].Data)
	}
	if got := withoutAudioLatencyReads(out, func(driver.Event) { t.Fatal("answered a batch without a latency read") }); got != out {
		t.Fatal("a batch without latency reads was copied")
	}
}

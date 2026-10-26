//go:build darwin && !ios

package macos

import (
	"testing"
	"time"
)

func TestALongScrollCommitsOnlyWhenItEnds(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	var rested time.Duration
	// Ten seconds of input or glide: never mid-scroll.
	for at := 1; at < 10_000; at += 16 {
		if commitDue(ms(at), ms(at)+commitDelay, false, true, &rested) {
			t.Fatalf("committed mid-scroll at %d ms", at)
		}
	}
	if commitDue(ms(10_000), ms(10_100), true, false, &rested) {
		t.Fatal("committed while the offset still moved")
	}
	if !commitDue(ms(10_200), ms(10_100), true, false, &rested) {
		t.Fatal("did not commit at rest")
	}
}

func TestARunningAnimationHoldsACommitOnlyBriefly(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	var rested time.Duration
	if commitDue(ms(1000), ms(900), true, true, &rested) {
		t.Fatal("committed while the frame clock runs")
	}
	if commitDue(ms(1000)+restWait-ms(32), ms(900), true, true, &rested) {
		t.Fatal("committed before the wait ran out")
	}
	if !commitDue(ms(1000)+restWait, ms(900), true, true, &rested) {
		t.Fatal("an endless animation held the commit back")
	}
	// Input again starts the wait over.
	rested = ms(1)
	if commitDue(ms(5000), ms(4900), false, true, &rested) || rested != 0 {
		t.Fatal("input did not start the wait over")
	}
}

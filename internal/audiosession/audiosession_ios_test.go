//go:build ios

package audiosession

import (
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
)

// sessionCategory reads the session's category, loading AVFAudio first.
func sessionCategory() string {
	constant("AVAudioSessionCategoryAmbient")
	shared := objc.ID(objc.GetClass("AVAudioSession")).Send(sel("sharedInstance"))
	return objc.Send[string](shared.Send(sel("category")), sel("UTF8String"))
}

// A Playback hold sets the session's Playback category, which the Ring/Silent switch does not silence, and releasing it gives back the category from before.
func TestPlaybackHoldSetsTheCategory(t *testing.T) {
	old := deactivateAfter
	deactivateAfter = time.Millisecond
	t.Cleanup(func() { deactivateAfter = old })
	before := sessionCategory()
	release, err := Hold(Playback)
	if err != nil {
		t.Fatal(err)
	}
	if got := sessionCategory(); got != "AVAudioSessionCategoryPlayback" {
		release()
		t.Fatalf("held at %q, want AVAudioSessionCategoryPlayback", got)
	}
	release()
	waitFor(t, func() bool { return holders.applied == 0 })
	if got := sessionCategory(); got != before {
		t.Fatalf("released to %q, want %q", got, before)
	}
}

package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestScriptContentAndPiPControls(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("headless support model")
	}
	app := nsgotest.Start(t, Root, nsgotest.Environment(core.Environment{OS: core.OSIOS}), nsgotest.PictureInPictureSupport(true, true))
	app.Press("id:start-video")
	if !app.Find("id:video").PiPState.Active {
		t.Fatal("video did not enter PiP")
	}
	app.Press("text:Next line")
	app.FindMounted("text:Line 2")
	app.Press("id:start-script")
	if app.Find("id:script-status").Text != "Picture in Picture is active" {
		t.Fatal("script did not enter PiP")
	}
	app.Play("id:caption-video", core.VideoInfo{Duration: 5 * time.Second, Size: core.Size{Width: 320, Height: 180}})
	if got := app.FindMounted("id:caption-pip").PictureInPicture.Duration; got != 5*time.Second {
		t.Fatalf("caption PiP duration = %v, want 5s", got)
	}
	app.PictureInPictureControl("id:caption-pip", "pause", 0)
	if !app.FindMounted("id:caption-video").Video.Paused {
		t.Fatal("pausing PiP did not pause its video")
	}
	app.PictureInPictureControl("id:caption-pip", "play", 0)
	if app.FindMounted("id:caption-video").Video.Paused {
		t.Fatal("playing PiP did not resume its video")
	}
	app.PictureInPictureControl("id:caption-pip", "skip", 2*time.Second)
	if seeks := app.FindMounted("id:caption-video").Seeks; len(seeks) != 1 || seeks[0] != 2*time.Second {
		t.Fatalf("video seeks after PiP skip: %v", seeks)
	}
	app.PictureInPictureControl("id:caption-pip", "skip", 10*time.Second)
	if seeks := app.FindMounted("id:caption-video").Seeks; len(seeks) != 2 || seeks[1] != 5*time.Second {
		t.Fatalf("video seeks after clamped PiP skip: %v", seeks)
	}
}

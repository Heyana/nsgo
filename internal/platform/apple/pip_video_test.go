//go:build darwin && !ios

package apple

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/videoalpha/clips"
)

func TestPictureInPictureCompositesDecodedVideoAndOverlay(t *testing.T) {
	t.Run("video", func(t *testing.T) { testPictureInPictureComposite(t, false) })
	t.Run("video and separate audio", func(t *testing.T) { testPictureInPictureComposite(t, true) })
}

func testPictureInPictureComposite(t *testing.T, separateAudio bool) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pip.mov")
	if err := os.WriteFile(path, clips.PiP, 0o600); err != nil {
		t.Fatal(err)
	}
	config := driver.VideoConfig{
		Source: path, Paused: true, Muted: true, Rate: 1, ResizeMode: driver.Stretch,
		ProgressInterval: 100 * time.Millisecond,
	}
	if separateAudio {
		config.AudioSource = filepath.Join(dir, "audio.wav")
		if err := os.WriteFile(config.AudioSource, wav(3*time.Second), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(holdMainLoop())
	var video *Video
	var holder, root objc.ID
	onMain(func() {
		loadAVFoundation()
		loadAVKit()
		root = objc.ID(Class("CALayer")).Send(sel("new"))
		root.Send(sel("setBounds:"), Rect(0, 0, 8, 8))
		holder = objc.ID(Class("NSView")).Send(sel("alloc")).Send(sel("initWithFrame:"), Rect(0, 0, 8, 8))
		holder.Send(sel("setWantsLayer:"), true)
		root.Send(sel("addSublayer:"), holder.Send(sel("layer")))
		video = NewVideo(holder)
		video.Dispatch = func(driver.Event) {}
		video.frame.owner = &PictureInPicture{}
		video.Configure(&config)
		// UIKit removes the inline player view inside the detached tree.
		video.View().Send(sel("removeFromSuperview"))
		video.Seek(250 * time.Millisecond)
		overlay := objc.ID(Class("CALayer")).Send(sel("new"))
		overlay.Send(sel("setFrame:"), Rect(2, 2, 4, 4))
		red := cgColorCreateSRGB(1, 0, 0, 1)
		overlay.Send(sel("setBackgroundColor:"), red)
		Release(red)
		root.Send(sel("addSublayer:"), overlay)
		overlay.Send(sel("release"))
	})
	defer onMain(func() {
		video.Close()
		holder.Send(sel("release"))
		root.Send(sel("release"))
	})
	cm := load("/System/Library/Frameworks/CoreMedia.framework/CoreMedia")
	var getPixels func(uintptr) uintptr
	purego.RegisterLibFunc(&getPixels, cm, "CMSampleBufferGetImageBuffer")

	capture := func() (videoPixel, overlayPixel [4]byte) {
		t.Helper()
		var sample uintptr
		var err error
		onMain(func() {
			video.UpdatePictureInPictureFrame(false)
			if video.frame.layer != 0 {
				sample, err = sampleOfLayer(root, 8, 8, 8, 8, 0)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		if sample == 0 {
			return
		}
		defer Release(sample)
		pixel := getPixels(sample)
		if cvPixelBufferLock(pixel, 0) != 0 {
			t.Fatal("cannot read composite frame")
		}
		defer cvPixelBufferUnlock(pixel, 0)
		address, row := cvPixelBufferBase(pixel), int(cvPixelBufferRow(pixel))
		bytes := unsafe.Slice((*byte)(*(*unsafe.Pointer)(unsafe.Pointer(&address))), row*8)
		return [4]byte(bytes[row+4 : row+8]), [4]byte(bytes[row*4+16 : row*4+20])
	}
	waitForChannel := func(channel int) {
		t.Helper()
		var frame, overlay [4]byte
		for deadline := time.Now().Add(6 * time.Second); time.Now().Before(deadline); {
			frame, overlay = capture()
			// AVFoundation color conversion may leave small contributions in the other channels. Blue and green must still be dominant.
			other := max(int(frame[1-channel]), int(frame[2]))
			if int(frame[channel])-other > 120 && frame[channel] > 200 && frame[3] == 255 {
				if overlay != [4]byte{0, 0, 255, 255} {
					t.Fatalf("overlay BGRA = %v, want red", overlay)
				}
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		var itemState int
		var position time.Duration
		var reason string
		onMain(func() {
			item := video.player.Send(sel("currentItem"))
			itemState = objc.Send[int](item, sel("status"))
			position = video.position()
			reason = GoString(item.Send(sel("error")).Send(sel("localizedDescription")))
		})
		t.Fatalf("video BGRA = %v, want an opaque frame high in channel %d (item status %d, position %v, error %q)",
			frame, channel, itemState, position, reason)
	}
	waitForChannel(0)
	onMain(func() {
		config.Paused = false
		video.Configure(&config)
	})
	waitForChannel(1)
	onMain(func() {
		config.Paused = true
		video.Configure(&config)
		video.Seek(250 * time.Millisecond)
	})
	waitForChannel(0)
}

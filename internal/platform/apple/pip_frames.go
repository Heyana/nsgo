//go:build darwin

package apple

import (
	"fmt"
	"math"
	"structs"
	"sync"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Sample frames are drawn directly into a locked CVPixelBuffer, with one
// pending dirty flag rather than a queue of retained images. AVKit keeps
// the last enqueued frame while the engine or app is suspended.
var sampleOnce sync.Once
var cmTimebaseCreate func(uintptr, uintptr, *uintptr) int32
var cmHostClock func() uintptr
var cmTimebaseSetTime func(uintptr, cmTime) int32
var cmTimebaseSetRate func(uintptr, float64) int32
var pixelSurfaceKey, pixelSpaceKey uintptr
var cvBufferSetAttachment func(uintptr, uintptr, uintptr, uint32)

var (
	cvPixelBufferCreate func(uintptr, int, int, uint32, uintptr, *uintptr) int32
	cmVideoDescription  func(uintptr, uintptr, *uintptr) int32
	cmSampleCreate      func(uintptr, uintptr, bool, uintptr, uintptr, uintptr, *sampleTiming, *uintptr) int32
	cmSampleAttachments func(uintptr, bool) uintptr
	cfDictionarySet     func(uintptr, uintptr, uintptr)
	immediateKey        uintptr
)

type sampleTiming struct {
	_                              structs.HostLayout
	Duration, Presentation, Decode cmTime
}

func loadSamples() {
	pixelBuffersLoaded.Do(loadPixelBuffers)
	cv := load("/System/Library/Frameworks/CoreVideo.framework/CoreVideo")
	cm := load("/System/Library/Frameworks/CoreMedia.framework/CoreMedia")
	cf := load("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
	reg := purego.RegisterLibFunc
	reg(&cmTimebaseCreate, cm, "CMTimebaseCreateWithSourceClock")
	reg(&cmHostClock, cm, "CMClockGetHostTimeClock")
	reg(&cmTimebaseSetTime, cm, "CMTimebaseSetTime")
	reg(&cmTimebaseSetRate, cm, "CMTimebaseSetRate")
	reg(&cvBufferSetAttachment, cv, "CVBufferSetAttachment")
	pixelSurfaceKey = constant(cv, "kCVPixelBufferIOSurfacePropertiesKey")
	pixelSpaceKey = constant(cv, "kCVImageBufferCGColorSpaceKey")
	reg(&cvPixelBufferCreate, cv, "CVPixelBufferCreate")
	reg(&cmVideoDescription, cm, "CMVideoFormatDescriptionCreateForImageBuffer")
	reg(&cmSampleCreate, cm, "CMSampleBufferCreateForImageBuffer")
	reg(&cmSampleAttachments, cm, "CMSampleBufferGetSampleAttachmentsArray")
	reg(&cfDictionarySet, cf, "CFDictionarySetValue")
	immediateKey = constant(cm, "kCMSampleAttachmentKey_DisplayImmediately")
	bitmapsLoaded.Do(loadBitmaps)
}

// Dirty coalesces tree changes until the next display frame.
func (p *PictureInPicture) Dirty() { p.framePending = true }

// DrawFrame draws a custom tree's layer only when AVKit can accept it.
// It returns true while a pending frame needs a display tick to retry.
func (p *PictureInPicture) DrawFrame(tree objc.ID, scale float64) bool {
	if p.closing || !p.custom || p.controller == 0 || !p.framePending {
		return false
	}
	p.update()
	if objc.Send[int](p.layer, sel("status")) == 2 { // AVQueuedSampleBufferRenderingStatusFailed
		p.layer.Send(sel("flush"))
	}
	if !objc.Send[bool](p.layer, sel("isReadyForMoreMediaData")) {
		return true
	}
	c := p.snapshot.Load()
	w := min(max(int(math.Ceil(float64(c.Size.Width)*scale)), 1), 2048)
	h := min(max(int(math.Ceil(float64(c.Size.Height)*scale)), 1), 2048)
	sample, err := sampleOfLayer(tree, w, h, float64(c.Size.Width), float64(c.Size.Height), clampPiPPosition(c.Position, c.Duration))
	p.framePending = false
	if err != nil {
		p.fail(err.Error())
		return false
	}
	p.layer.Send(sel("enqueueSampleBuffer:"), sample)
	Release(sample)
	if now := time.Now(); !p.lastFrame.IsZero() {
		p.state.FrameInterval = now.Sub(p.lastFrame)
		p.lastFrame = now
	} else {
		p.lastFrame = now
	}
	if p.state.Active && time.Since(p.lastStats) >= time.Second && p.state.FrameInterval > 0 {
		p.lastStats = time.Now()
		p.report()
	}
	p.update()
	return false
}

// sampleOfLayer pictures only this detached content tree. CoreVideo
// owns the pixels, and no pointer to Go memory survives the call.
func sampleOfLayer(layer objc.ID, w, h int, pointsW, pointsH float64, position time.Duration) (uintptr, error) {
	sampleOnce.Do(loadSamples)
	var pixels, format, sample uintptr
	empty := dictionary(nil, nil)
	defer Release(empty)
	attrs := dictionary([]uintptr{pixelSurfaceKey}, []uintptr{empty})
	defer Release(attrs)
	if status := cvPixelBufferCreate(0, w, h, pixelBGRA, attrs, &pixels); status != 0 {
		return 0, fmt.Errorf("PiP pixel buffer: %d", status)
	}
	defer Release(pixels)
	cvBufferSetAttachment(pixels, pixelSpaceKey, srgb, 1)
	if status := cvPixelBufferLock(pixels, 0); status != 0 {
		return 0, fmt.Errorf("PiP pixel lock: %d", status)
	}
	defer cvPixelBufferUnlock(pixels, 0)
	const premultipliedFirst, byteOrder32Little = 2, 2 << 12
	ctx := cgBitmapContextCreate(cvPixelBufferBase(pixels), uintptr(w), uintptr(h), 8, cvPixelBufferRow(pixels), srgb, premultipliedFirst|byteOrder32Little)
	if ctx == 0 {
		return 0, fmt.Errorf("PiP bitmap context could not be created")
	}
	cgContextClearRect(ctx, Rect(0, 0, float64(w), float64(h)))
	cgContextTranslateCTM(ctx, 0, float64(h))
	cgContextScaleCTM(ctx, float64(w)/pointsW, -float64(h)/pointsH)
	layer.Send(sel("renderInContext:"), ctx)
	Release(ctx)
	if status := cmVideoDescription(0, pixels, &format); status != 0 {
		return 0, fmt.Errorf("PiP video format: %d", status)
	}
	defer Release(format)
	timing := sampleTiming{Presentation: cmTimeOf(position)}
	if status := cmSampleCreate(0, pixels, true, 0, 0, format, &timing, &sample); status != 0 {
		return 0, fmt.Errorf("PiP video frame: %d", status)
	}
	if attachments := cmSampleAttachments(sample, true); attachments != 0 {
		cfDictionarySet(cfArrayGetValueAtIndex(attachments, 0), immediateKey, cfTrue)
	}
	return sample, nil
}

// clampPiPPosition keeps the display layer's timebase within [0, duration] for
// finite playback, as AVPictureInPictureSampleBufferPlaybackDelegate requires.
func clampPiPPosition(position, duration time.Duration) time.Duration {
	if position < 0 {
		position = 0
	}
	if duration > 0 && position > duration {
		return duration
	}
	return position
}

func (p *PictureInPicture) configureTimebase(c driver.PictureInPictureConfig) {
	sampleOnce.Do(loadSamples)
	if p.timebase == 0 {
		if cmTimebaseCreate(0, cmHostClock(), &p.timebase) != 0 {
			p.fail("PiP sample clock could not be created")
			return
		}
		p.layer.Send(sel("setControlTimebase:"), p.timebase)
	}
	cmTimebaseSetTime(p.timebase, cmTimeOf(clampPiPPosition(c.Position, c.Duration)))
	rate := float64(1)
	if c.Paused {
		rate = 0
	}
	cmTimebaseSetRate(p.timebase, rate)
}

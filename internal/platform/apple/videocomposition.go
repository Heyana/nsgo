//go:build darwin

package apple

import (
	"errors"
	"fmt"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

type assetTrackResult struct {
	track objc.ID // retained by the receiver
	err   error
}

// loadAssetTrack starts loading the asset's first track of kind, which is
// the AVFoundation media-type code. The caller retains the asset.
func loadAssetTrack(asset objc.ID, kind string) <-chan assetTrackResult {
	result := make(chan assetTrackResult, 1)
	keys := objc.ID(Class("NSArray")).Send(sel("arrayWithObject:"), NSString("tracks"))
	callback := objcblock.New(func(objc.Block) {
		var failure objc.ID
		const loaded = 2 // AVKeyValueStatusLoaded
		status := objc.Send[int](asset, sel("statusOfValueForKey:error:"), NSString("tracks"), &failure)
		if status != loaded {
			message := "cannot load the asset's tracks"
			if failure != 0 {
				message = GoString(failure.Send(sel("localizedDescription")))
			}
			result <- assetTrackResult{err: errors.New(message)}
			return
		}
		tracks := asset.Send(sel("tracksWithMediaType:"), NSString(kind))
		if objc.Send[uint](tracks, sel("count")) == 0 {
			result <- assetTrackResult{err: errors.New("no matching track")}
			return
		}
		result <- assetTrackResult{track: tracks.Send(sel("objectAtIndex:"), uint(0)).Send(sel("retain"))}
	})
	asset.Send(sel("loadValuesAsynchronouslyForKeys:completionHandler:"), keys, callback)
	callback.Release()
	return result
}

// composeVideoAndAudio starts both asset loads before returning their
// cancellation function. A worker assembles the tracks once they are ready.
func composeVideoAndAudio(video, audio objc.ID, done func(objc.ID, error)) (cancel func()) {
	video.Send(sel("retain"))
	audio.Send(sel("retain"))
	v, a := loadAssetTrack(video, "vide"), loadAssetTrack(audio, "soun")
	go func() {
		picture, sound := <-v, <-a
		withPool(func() {
			defer picture.track.Send(sel("release"))
			defer sound.track.Send(sel("release"))
			composition, err := combineTracks(picture, sound)
			OnMain(func() {
				defer video.Send(sel("release"))
				defer audio.Send(sel("release"))
				defer composition.Send(sel("release"))
				done(composition, err)
			})
		})
	}()
	return func() {
		video.Send(sel("cancelLoading"))
		audio.Send(sel("cancelLoading"))
	}
}

// combineTracks starts the two media tracks at zero and keeps the picture's
// duration. AVPlayer can seek them through its one composition timeline.
func combineTracks(video, audio assetTrackResult) (objc.ID, error) {
	if video.err != nil {
		return 0, fmt.Errorf("the video track: %w", video.err)
	}
	if audio.err != nil {
		return 0, fmt.Errorf("the separate audio track: %w", audio.err)
	}
	picture := objc.Send[cmTimeRange](video.track, sel("timeRange"))
	sound := objc.Send[cmTimeRange](audio.track, sel("timeRange"))
	if picture.Duration.duration() <= 0 || sound.Duration.duration() <= 0 {
		return 0, errors.New("separate audio needs tracks with finite durations")
	}
	if sound.Duration.duration() > picture.Duration.duration() {
		sound.Duration = picture.Duration
	}
	composition := objc.ID(Class("AVMutableComposition")).Send(sel("composition"))
	add := func(kind string, from objc.ID, r cmTimeRange) (objc.ID, error) {
		track := composition.Send(sel("addMutableTrackWithMediaType:preferredTrackID:"), NSString(kind), int32(0))
		var failure objc.ID
		ok := objc.Send[bool](track, sel("insertTimeRange:ofTrack:atTime:error:"), r, from, cmTimeOf(0), &failure)
		if !ok {
			if failure != 0 {
				return 0, errors.New(GoString(failure.Send(sel("localizedDescription"))))
			}
			return 0, errors.New("cannot insert the track")
		}
		return track, nil
	}
	videoTrack, err := add("vide", video.track, picture)
	if err != nil {
		return 0, fmt.Errorf("the video track: %w", err)
	}
	videoTrack.Send(sel("setPreferredTransform:"), objc.Send[affine](video.track, sel("preferredTransform")))
	if _, err := add("soun", audio.track, sound); err != nil {
		return 0, fmt.Errorf("the separate audio track: %w", err)
	}
	return composition.Send(sel("retain")), nil
}

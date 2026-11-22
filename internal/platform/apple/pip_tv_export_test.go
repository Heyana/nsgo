//go:build ios && nsgotest

package apple

import (
	"errors"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// CheckTVPictureInPictureForTest runs after UIKit has opened the TV app.
func CheckTVPictureInPictureForTest() error {
	if !television() {
		return errors.New("the native host is not an Apple TV app")
	}
	loadAVFoundation()
	player := objc.ID(Class("AVPlayer")).Send(sel("new"))
	defer player.Send(sel("release"))
	session := newTVNowPlaying(player)
	if session == 0 {
		return errors.New("the system did not create a Now Playing session")
	}
	defer session.Send(sel("release"))
	players := session.Send(sel("players"))
	if objc.Send[int](players, sel("count")) != 1 || players.Send(sel("objectAtIndex:"), 0) != player {
		return errors.New("Now Playing does not own the PiP player")
	}
	if !objc.Send[bool](session, sel("automaticallyPublishesNowPlayingInfo")) {
		return errors.New("Now Playing does not publish the player's metadata")
	}
	var configured bool
	cls := RegisterClass("NsgoTestTVPiPAutomaticController", Class("NSObject"), nil, []objc.MethodDef{
		{Cmd: sel("isPictureInPicturePossible"), Fn: func(objc.ID, objc.SEL) bool { return true }},
		{Cmd: sel("isPictureInPictureActive"), Fn: func(objc.ID, objc.SEL) bool { return false }},
		{Cmd: sel("setCanStartPictureInPictureAutomaticallyFromInline:"), Fn: func(objc.ID, objc.SEL, bool) { configured = true }},
	})
	p := &PictureInPicture{controller: objc.ID(cls).Send(sel("new"))}
	defer p.controller.Send(sel("release"))
	p.Configure(driver.PictureInPictureConfig{Automatically: true})
	if configured || p.ContinuesInBackground() {
		return errors.New("tvOS configured iOS-only automatic entry or kept inline background playback alive")
	}
	custom := NewPictureInPicture(0, nil)
	defer custom.Close(nil)
	if custom.state.Supported {
		return errors.New("unverified tvOS custom-content PiP was advertised")
	}
	return nil
}

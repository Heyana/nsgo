//go:build darwin

package apple

import (
	"runtime"
	"sync"

	"github.com/ebitengine/purego/objc"
)

func television() bool {
	if runtime.GOOS != "ios" {
		return false
	}
	device := objc.ID(Class("UIDevice")).Send(sel("currentDevice"))
	return objc.Send[int](device, sel("userInterfaceIdiom")) == 2
}

var tvNowPlayingClass = sync.OnceValue(func() objc.Class {
	load("/System/Library/Frameworks/MediaPlayer.framework/MediaPlayer")
	return Class("MPNowPlayingSession")
})

// A custom player layer on tvOS must belong to a Now Playing session. Keep
// that session with the player until its PiP generation finishes stopping.
func newTVNowPlaying(player objc.ID) objc.ID {
	if !television() || player == 0 {
		return 0
	}
	cls := tvNowPlayingClass()
	if cls == 0 {
		return 0
	}
	players := objc.ID(Class("NSArray")).Send(sel("arrayWithObject:"), player)
	session := objc.ID(cls).Send(sel("alloc")).Send(sel("initWithPlayers:"), players)
	if session != 0 {
		session.Send(sel("setAutomaticallyPublishesNowPlayingInfo:"), true)
		session.Send(sel("becomeActiveIfPossibleWithCompletion:"), uintptr(0))
	}
	return session
}

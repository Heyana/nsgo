//go:build darwin

package apple

import (
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

const playerPauseAtEnd = 1 // AVPlayerActionAtItemEndPause

func exactAudioTime(d time.Duration) cmTime {
	return cmTime{Value: d.Nanoseconds(), Timescale: int32(time.Second), Flags: 1}
}

// prepareNativeLoop appends prepared replicas after the first pass. Its item is already ready, so the looper can read the asset's duration.
func (p *audioPlayer) prepareNativeLoop() {
	if !p.loop || p.looper != 0 {
		return
	}
	duration := p.duration()
	class := objc.ID(Class("AVPlayerLooper"))
	init := sel("initWithPlayer:templateItem:timeRange:existingItemsOrdering:")
	if duration <= 0 || !objc.Send[bool](class, sel("instancesRespondToSelector:"), init) {
		return
	}
	start := driver.AudioLoopStart(p.loopStart, duration)
	loopRange := cmTimeRange{Start: exactAudioTime(start), Duration: exactAudioTime(duration - start)}
	const loopItemsFollow = 1 // AVPlayerLooperItemOrderingLoopingItemsFollowExistingItems
	p.loopBase = p.laps
	p.looper = class.Send(sel("alloc")).Send(init, p.player, p.item, loopRange, loopItemsFollow)
}

// countNativeLoops counts the move from the original item to its first replica as well as the transitions between replicas.
func (p *audioPlayer) countNativeLoops() {
	if p.looper != 0 {
		if item := p.player.Send(sel("currentItem")); item != 0 && item != p.intro {
			count := objc.Send[int](p.looper, sel("loopCount"))
			p.laps = max(p.laps, p.loopBase+1+uint32(max(count, 0)))
		}
	}
}

func (p *audioPlayer) setLoop(loop bool) {
	if p.loop == loop {
		return
	}
	p.loop = loop
	if !p.loaded {
		return
	}
	if loop {
		p.prepareNativeLoop()
	} else if p.looper != 0 {
		p.resetLoopQueue(p.now())
	}
}

func (p *audioPlayer) releaseNativeLoop() {
	if p.looper != 0 {
		looper := p.looper
		p.looper = 0
		looper.Send(sel("disableLooping"))
		looper.Send(sel("release"))
	}
}

// resetLoopQueue makes a fresh first pass when looping is disabled or a seek goes into the intro. The looper owns its replicas until released.
func (p *audioPlayer) resetLoopQueue(position time.Duration) {
	p.countNativeLoops()
	p.generation++
	p.seekSerial++
	p.seeking = true
	p.closeQueue()
	intro := objc.ID(Class("AVPlayerItem")).Send(sel("playerItemWithAsset:"), p.item.Send(sel("asset")))
	p.openQueue(intro)
	p.prepareNativeLoop()
	p.seekTime(position, true)
}

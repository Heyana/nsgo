package core

import "github.com/nsgo-dev/nsgo/internal/driver"

// Playback is the core's part of audio.Playback, which apps use to control one play of a Sound. Call its methods on the engine's goroutine.
type Playback struct {
	sound        *soundBuffer
	id           uint32
	ended        *Signal[bool]
	volume, rate float32
}

// Ended reports whether playback finished, was stopped or evicted, or could not start. Reading it tracks the state in the current reactive scope.
func (p *Playback) Ended() bool { return p == nil || p.ended == nil || p.ended.Get() }

// SetVolume changes this playback's volume from zero (silence) to one.
func (p *Playback) SetVolume(v float32) {
	if p == nil || p.sound == nil {
		return
	}
	if v != v {
		v = 1
	}
	p.set(min(max(v, 0), 1), p.rate)
}

// SetRate changes this playback's speed from 0.5 to 2. Zero uses 1.
func (p *Playback) SetRate(r float32) {
	if p != nil && p.sound != nil {
		p.set(p.volume, audioRate(r))
	}
}

func (p *Playback) set(volume, rate float32) {
	if volume == p.volume && rate == p.rate {
		return
	}
	p.volume, p.rate = volume, rate
	b := p.sound
	b.key.tree.sendAudio(b.id, driver.AudioRequest{
		Command: driver.AudioPlaybackSet, Playback: p.id, Volume: volume, Rate: rate,
	})
}

// Stop ends this playback and leaves other playbacks of its sound running.
func (p *Playback) Stop() {
	if p == nil || p.sound == nil {
		return
	}
	b := p.sound
	b.key.tree.sendAudio(b.id, driver.AudioRequest{Command: driver.AudioPlaybackStop, Playback: p.id})
	p.finish()
}

func (p *Playback) finish() {
	delete(p.sound.playbacks, p.id)
	p.sound = nil
	p.ended.Set(true)
}

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// This file is the core's part of the audio package, which apps use (docs/architecture/audio.md): sounds decoded once and played many times, overlapping, and players of long audio, both commands in the batch of the turn that calls them, whose state the core keeps from the platform's events.

// AudioSource is what a sound or a player plays, as an image's source says what it shows: the core's part of audio.Source, which apps use. URI is an "asset:" path in the app's resources, a file path or file: URL, a data: URL or an http(s) URL; Data, when set, is the bytes of a file, such as one embedded with go:embed, whose format their first bytes say.
type AudioSource struct {
	URI  string
	Data []byte
}

// SoundVoice is how a playback of a sound plays: the core's part of audio.Voice, which apps use. Volume runs from 0 to 1 and Rate from 0.5 to 2, with 0 as 1 for both, as Video's; Pan runs from -1 (full left) to 1 (full right), with 0 centered. Other values are clamped, and NaN takes the default.
type SoundVoice struct {
	Volume, Rate, Pan float32
	// StartAt schedules this playback on the clock returned by audio.Now. A zero or past time plays as soon as possible.
	StartAt time.Time
	// Loop repeats this playback until its Playback or the sound stops, or the sound closes.
	Loop bool
	// Muted starts the playback at zero volume, which its Playback's SetVolume can raise.
	Muted bool
}

// AudioError is why a sound or a player failed, named as NSH's codes, which follow the web's MediaError: the core's part of audio.Error, which apps use. A failure that says more is one wrapped with the platform's words, which errors.Is and errors.As find.
type AudioError string

const (
	// AudioNotAllowed: the current owner's Access refuses AccessAudio, or AccessNetwork for an http(s) source; or, on the web, the browser refused to play before a user gesture.
	AudioNotAllowed AudioError = driver.AudioNotAllowed
	// AudioNetwork: the source could not be fetched.
	AudioNetwork AudioError = driver.AudioNetwork
	// AudioDecode: the source is in a format the platform cannot read.
	AudioDecode AudioError = driver.AudioDecode
	// AudioNotSupported: the source is none the platform plays, such as one with neither a URI nor bytes.
	AudioNotSupported AudioError = driver.AudioNotSupported
	// AudioQuotaExceeded: past a limit of the engine's, which a sound's load meets: more than 8 MiB encoded or 5 seconds decoded, a 65th sound, or more than 16 MiB decoded among them.
	AudioQuotaExceeded AudioError = driver.AudioQuotaExceeded
	// AudioUnavailable: the platform plays no audio (Env.CanPlayAudio).
	AudioUnavailable AudioError = driver.AudioUnavailable
	// AudioFailed: the platform failed otherwise, such as a player whose output went away.
	AudioFailed AudioError = driver.AudioFailed
)

func (e AudioError) Error() string { return "nsgo: audio: " + string(e) }

// audioFailure is an AudioError with what happened in words, and the failure under it, such as Access's refusal, an *Error.
type audioFailure struct {
	code    AudioError
	message string
	cause   error
}

func (f *audioFailure) Error() string { return f.code.Error() + ": " + f.message }

func (f *audioFailure) Unwrap() []error {
	if f.cause == nil {
		return []error{f.code}
	}
	return []error{f.code, f.cause}
}

// audioFail is code with message, code alone when message is empty.
func audioFail(code AudioError, message string) error {
	if message == "" {
		return code
	}
	return &audioFailure{code: code, message: message}
}

// AudioPlayerState is where an AudioPlayer is, as the core follows it from the platform's events: the core's part of audio.PlayerState, which apps use.
type AudioPlayerState uint8

const (
	// AudioPlayerIdle: nothing asked of it yet.
	AudioPlayerIdle AudioPlayerState = iota
	// AudioPlayerLoading: loading its source, or waiting for data as it plays.
	AudioPlayerLoading
	// AudioPlayerPlaying: playing.
	AudioPlayerPlaying
	// AudioPlayerPaused: loaded, not playing; paused by the app or by the system, as for a call.
	AudioPlayerPaused
	// AudioPlayerEnded: played to its end; Play starts it over.
	AudioPlayerEnded
	// AudioPlayerFailed: it failed and plays nothing until Play or Preload loads it again.
	AudioPlayerFailed
)

// String returns the state's name: idle, loading, playing, paused, ended or failed.
func (s AudioPlayerState) String() string {
	switch s {
	case AudioPlayerLoading:
		return "loading"
	case AudioPlayerPlaying:
		return "playing"
	case AudioPlayerPaused:
		return "paused"
	case AudioPlayerEnded:
		return "ended"
	case AudioPlayerFailed:
		return "failed"
	}
	return "idle"
}

// AudioPlayerOptions are an AudioPlayer's: the core's part of audio.PlayerOptions, which apps use. Its callbacks run on the engine's goroutine, in a later turn than what caused them.
type AudioPlayerOptions struct {
	// LoopStart is where playback returns when looping is enabled. The first pass starts normally. Zero loops the full track.
	LoopStart time.Duration
	// OnLoad runs once the player knows its duration, 0 for a stream without one.
	OnLoad func(duration time.Duration)
	// OnProgress receives the position and the duration every ProgressInterval while the player plays, and once more as it pauses or ends. The position goes back only with a seek or a loop starting over.
	OnProgress func(position, duration time.Duration)
	// ProgressInterval is the time between OnProgress calls: 250 milliseconds when 0, and at least 16, as Video's.
	ProgressInterval time.Duration
	// OnBuffer receives whether playback waits for data.
	OnBuffer func(buffering bool)
	// OnEnd runs each time the player plays to its end; never while it loops.
	OnEnd func()
	// OnError receives why the player failed, an AudioError.
	OnError func(err error)
	// AudioSession is what the player does to other apps' audio while it plays: by default it mixes with them and, on iOS, the Ring/Silent switch silences it; with AudioSessionPlayback or AudioSessionMixWithOthers it plays in silent mode, the first pausing other apps' audio.
	AudioSession AudioSession
}

// audioIDs numbers sounds and players for the whole process, so a late event of one that closed finds no other.
var audioIDs atomic.Uint32

// audioState is an engine's sounds and players.
type audioState struct {
	// buffers are the decoded sounds and those decoding, by surface and source: the Sounds of one source share one.
	buffers map[soundKey]*soundBuffer
	// decoded is the decoded bytes the loaded buffers hold.
	decoded int64
	// sounds and players are what the platform may report of, by ID.
	sounds  map[uint32]*soundBuffer
	players map[uint32]*AudioPlayer
}

type soundKey struct {
	tree   *Tree
	source string
}

func (rt *runtime) audioState() *audioState {
	if rt.audio == nil {
		rt.audio = &audioState{buffers: map[soundKey]*soundBuffer{}, sounds: map[uint32]*soundBuffer{},
			players: map[uint32]*AudioPlayer{}}
	}
	return rt.audio
}

// audioEvent hands a platform's report to the sound or player it is of.
func (rt *runtime) audioEvent(ev driver.Event) {
	st := rt.audio
	report, _ := ev.Data.(*driver.AudioEvent)
	if st == nil || report == nil {
		return
	}
	if b := st.sounds[ev.Count]; b != nil {
		b.event(report)
	} else if p := st.players[ev.Count]; p != nil {
		p.event(report)
	}
}

// sendAudio queues r for sound or player id in t's batch.
func (t *Tree) sendAudio(id uint32, r driver.AudioRequest) {
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemAudio, Count: id, Audio: &r})
	t.changed = true
}

// AudioOutputLatency implements audio.OutputLatency. An unavailable route returns zero with known false.
func (cx *Scope) AudioOutputLatency(done func(latency time.Duration, known bool)) {
	if done == nil {
		return
	}
	owner := cx.current()
	t := cx.tree()
	if cx.denied(serviceAudio, "") || t == nil || !Untracked(cx, cx.Env().CanPlayAudio) {
		owner.Post(func() { done(0, false) })
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemAudio, Audio: &driver.AudioRequest{Command: driver.AudioReadLatency}}, func(ev driver.Event) {
		if !owner.Disposed() {
			if e, _ := ev.Data.(*driver.AudioEvent); e != nil {
				done(e.Latency, e.Latency > 0)
			}
		}
	})
}

// audioRefusal is AudioNotAllowed when the running owner's or cx's Access refuses audio, or src's URI when it is no bytes the app holds, wrapping the refusal.
func audioRefusal(cx *Scope, src AudioSource) error {
	var err error
	if cx.denied(serviceAudio, "") {
		err = AccessDenied(AccessAudio, "")
	} else if len(src.Data) == 0 {
		o := cx.current()
		if err = sourceRefusal(o, AccessNetwork, src.URI); err == nil && o != cx {
			err = sourceRefusal(cx, AccessNetwork, src.URI)
		}
	}
	if err == nil {
		return nil
	}
	return &audioFailure{code: AudioNotAllowed, message: err.Error(), cause: err}
}

// audioUnplayable is why cx cannot play src at all, before the platform is asked: refused, no audio on the platform, or no source.
func audioUnplayable(cx *Scope, src AudioSource) error {
	if err := audioRefusal(cx, src); err != nil {
		return err
	}
	if cx.tree() == nil || !Untracked(cx, cx.Env().CanPlayAudio) {
		return AudioUnavailable
	}
	if src.URI == "" && len(src.Data) == 0 {
		return audioFail(AudioNotSupported, "no source")
	}
	return nil
}

// soundBuffer is one sound the platform decoded, or decodes, which the Sounds over it share.
type soundBuffer struct {
	rt       *runtime
	id       uint32
	key      soundKey
	loaded   bool
	duration time.Duration
	bytes    int64
	// open counts the Sounds over it that are not closed.
	open int
	// waits are the loads waiting for it to decode.
	waits     []*soundWait
	playbacks map[uint32]*Playback
}

type soundWait struct {
	owner *Scope
	done  func(*Sound, error)
	// gone says its owner was disposed.
	gone bool
}

// Sound is a short sound decoded into memory, played any number of times, overlapping: the core's part of audio.Sound, which apps use. Its methods are called on the engine's goroutine.
type Sound struct {
	buffer *soundBuffer
	closed bool
}

// LoadSound has the platform read and decode src into memory once and calls done with the sound or why not, an AudioError: the core's part of audio.LoadSound, which apps use. A source this engine's surface already holds, the same URI or bytes of the same digest, shares its decoded sound. The sound belongs to cx itself rather than the running owner, so loading with the app's scope from a handler keeps it until cx goes or Close is called; done, which runs in a later turn, never runs once cx went.
func (cx *Scope) LoadSound(src AudioSource, done func(*Sound, error)) {
	owner := cx
	if owner.disposed {
		return
	}
	fail := func(err error) {
		if done != nil && cx.rt.post != nil {
			owner.Post(func() { done(nil, err) })
		}
	}
	if err := audioUnplayable(cx, src); err != nil {
		fail(err)
		return
	}
	if len(src.Data) > driver.MaxSoundBytes {
		fail(audioFail(AudioQuotaExceeded, "the sound's bytes are "+strconv.Itoa(len(src.Data))+": a sound has at most 8 MiB"))
		return
	}
	t, st := cx.tree(), cx.rt.audioState()
	key := soundKey{tree: t, source: "uri:" + src.URI}
	if len(src.Data) > 0 {
		sum := sha256.Sum256(src.Data)
		key.source = "sha256:" + hex.EncodeToString(sum[:])
	}
	b := st.buffers[key]
	if b != nil && b.loaded {
		s := b.newSound(owner)
		if done != nil {
			owner.Post(func() { done(s, nil) })
		}
		return
	}
	if b == nil {
		if len(st.buffers) >= driver.MaxSounds {
			fail(audioFail(AudioQuotaExceeded, "an engine holds at most 64 sounds"))
			return
		}
		b = &soundBuffer{rt: cx.rt, id: audioIDs.Add(1), key: key}
		st.buffers[key], st.sounds[b.id] = b, b
		t.sendAudio(b.id, driver.AudioRequest{Command: driver.AudioLoad, Source: driver.ImageSource{URI: src.URI, Data: src.Data},
			MaxBytes: driver.MaxSoundBytes, MaxDuration: driver.MaxSoundDuration})
	}
	w := &soundWait{owner: owner, done: done}
	b.waits = append(b.waits, w)
	owner.addCleanup(func() {
		w.gone = true
		b.abandoned()
	})
}

// abandoned drops a load whose loads all went before it decoded.
func (b *soundBuffer) abandoned() {
	if b.loaded {
		return
	}
	for _, w := range b.waits {
		if !w.gone {
			return
		}
	}
	b.drop()
}

// drop forgets the buffer and has the platform free it.
func (b *soundBuffer) drop() {
	st := b.rt.audioState()
	if st.buffers[b.key] == b {
		delete(st.buffers, b.key)
	}
	if st.sounds[b.id] != b {
		return
	}
	delete(st.sounds, b.id)
	if b.loaded {
		st.decoded -= b.bytes
	}
	b.waits = nil
	b.endPlaybacks()
	b.key.tree.sendAudio(b.id, driver.AudioRequest{Command: driver.AudioUnload})
}

// newSound is a Sound over b that owner closes as it goes.
func (b *soundBuffer) newSound(owner *Scope) *Sound {
	s := &Sound{buffer: b}
	b.open++
	owner.addCleanup(s.Close)
	return s
}

// event takes the platform's answer to the buffer's load.
func (b *soundBuffer) event(e *driver.AudioEvent) {
	if e.Phase == driver.AudioPlaybackEnded {
		if p := b.playbacks[e.Playback]; p != nil {
			p.finish()
		}
		return
	}
	if b.loaded {
		return
	}
	st := b.rt.audioState()
	var err error
	switch e.Phase {
	case driver.AudioLoaded:
		switch {
		case e.Cut || e.Duration > driver.MaxSoundDuration:
			err = audioFail(AudioQuotaExceeded, "the sound is longer than 5 seconds: play it with a player")
		case st.decoded+e.Bytes > driver.MaxDecodedBytes:
			err = audioFail(AudioQuotaExceeded, "the sounds would take more than 16 MiB decoded")
		}
	case driver.AudioFailure:
		err = audioFail(AudioError(e.Error), e.Message)
		if e.Error == "" {
			err = audioFail(AudioFailed, e.Message)
		}
	default:
		return
	}
	waits := b.waits
	if err != nil {
		b.drop()
		for _, w := range waits {
			if !w.gone && w.done != nil {
				b.key.tree.within(w.owner, func() { w.done(nil, err) })
			}
		}
		return
	}
	b.loaded, b.duration, b.bytes, b.waits = true, e.Duration, e.Bytes, nil
	st.decoded += e.Bytes
	for _, w := range waits {
		if w.gone {
			continue
		}
		s := b.newSound(w.owner)
		if w.done != nil {
			b.key.tree.within(w.owner, func() { w.done(s, nil) })
		}
	}
	if b.open == 0 {
		b.drop()
	}
}

// Play starts a playback of the sound, over those playing, within a frame: it is a command in the batch of the turn that calls it. It returns the playback, to change, stop or watch alone; one of a closed sound, or on a backend that cannot address playbacks, is already ended. A playback that cannot be heard, the device muted, is no error.
func (s *Sound) Play(v SoundVoice) *Playback {
	p := &Playback{}
	if s == nil || s.closed {
		return p
	}
	b := s.buffer
	volume, rate := audioVolume(v.Volume), audioRate(v.Rate)
	if v.Muted {
		volume = 0
	}
	if b.key.tree.reported.AudioPlaybackControls {
		*p = Playback{sound: b, id: audioIDs.Add(1), ended: NewSignal(b.rt.root, false), volume: volume, rate: rate}
		if b.playbacks == nil {
			b.playbacks = map[uint32]*Playback{}
		}
		b.playbacks[p.id] = p
	}
	b.key.tree.sendAudio(b.id, driver.AudioRequest{Command: driver.AudioPlay, Playback: p.id, Volume: volume, Rate: rate, Pan: audioPan(v.Pan), StartAt: v.StartAt, Loop: v.Loop})
	return p
}

// Stop stops every playback of the sound's source.
func (s *Sound) Stop() {
	if !s.closed {
		s.buffer.key.tree.sendAudio(s.buffer.id, driver.AudioRequest{Command: driver.AudioStop})
		s.buffer.endPlaybacks()
	}
}

func (b *soundBuffer) endPlaybacks() {
	for _, p := range b.playbacks {
		p.finish()
	}
}

// Duration returns how long the sound plays.
func (s *Sound) Duration() time.Duration { return s.buffer.duration }

// Close lets the sound go: the decoded audio goes with the last Sound over it. Calling it again does nothing.
func (s *Sound) Close() {
	if s.closed {
		return
	}
	s.closed = true
	if s.buffer.open--; s.buffer.open == 0 {
		s.buffer.drop()
	}
}

// audioVolume is v from 0 to 1, 1 for 0.
func audioVolume(v float32) float32 {
	if v == 0 || v != v {
		return 1
	}
	return min(max(v, 0), 1)
}

// audioRate is r from 0.5 to 2, 1 for 0.
func audioRate(r float32) float32 {
	if r == 0 || r != r {
		return 1
	}
	return min(max(r, 0.5), 2)
}

// audioPan is p from -1 (full left) to 1 (full right), 0 (center) for NaN or -0.
func audioPan(p float32) float32 {
	if p == 0 || p != p {
		return 0
	}
	return min(max(p, -1), 1)
}

// AudioPlayer plays long audio, streamed and decoded as it plays: the core's part of audio.Player, which apps use. Its methods are called on the engine's goroutine, and all but the getters are commands in the batch of the turn that calls them.
type AudioPlayer struct {
	rt    *runtime
	tree  *Tree
	owner *Scope
	id    uint32
	src   AudioSource
	opts  AudioPlayerOptions
	// refused is why the owner's Access refuses it, nil when it does not.
	refused error
	// opened says the platform has it; wanted that Play asked for it to play, with no Pause since; playing that the platform said it plays, and has not paused, ended or failed since.
	opened, wanted, playing, closed bool
	// sought says Seek moved it after it ended.
	sought             bool
	state              AudioPlayerState
	position, duration time.Duration
	volume, rate       float32
	loop, muted        bool
	// seeks counts the AudioSeeks sent; seen and laps are the Seeks and Laps of the report the position came from last.
	seeks, seen, laps uint32
	plays             []func(error)
}

// NewAudioPlayer makes a player of src, loading nothing until Play or Preload: the core's part of audio.NewPlayer, which apps use. The player belongs to cx itself rather than the running owner, so creating one with the app's scope from a button or row handler keeps it playing after that handler's scope goes, closing only when cx goes or Close is called.
func (cx *Scope) NewAudioPlayer(src AudioSource, opts AudioPlayerOptions) *AudioPlayer {
	owner := cx
	opts.ProgressInterval = videoInterval(opts.ProgressInterval)
	p := &AudioPlayer{rt: cx.rt, tree: cx.tree(), owner: owner, id: audioIDs.Add(1), src: src, opts: opts, volume: 1, rate: 1}
	p.refused = audioRefusal(cx, src)
	owner.addCleanup(p.Close)
	return p
}

// unplayable is why the player cannot play at all, before the platform is asked.
func (p *AudioPlayer) unplayable() error {
	if p.refused != nil {
		return p.refused
	}
	if p.tree == nil || !Untracked(p.owner, p.owner.Env().CanPlayAudio) {
		return AudioUnavailable
	}
	if p.src.URI == "" && len(p.src.Data) == 0 {
		return audioFail(AudioNotSupported, "no source")
	}
	return nil
}

// failNow fails the player with err, as the platform would, in a later turn.
func (p *AudioPlayer) failNow(err error, done func(error)) {
	p.state, p.wanted, p.playing = AudioPlayerFailed, false, false
	if p.rt.post == nil {
		return
	}
	p.owner.Post(func() {
		if p.closed {
			return
		}
		if p.opts.OnError != nil {
			p.opts.OnError(err)
		}
		if done != nil && !p.closed {
			done(err)
		}
	})
}

// open has the platform make the player, with what it was given so far.
func (p *AudioPlayer) open() {
	if p.opened {
		return
	}
	p.opened = true
	p.rt.audioState().players[p.id] = p
	p.tree.sendAudio(p.id, driver.AudioRequest{Command: driver.AudioOpen, Source: driver.ImageSource{URI: p.src.URI, Data: p.src.Data},
		Volume: p.effectiveVolume(), Rate: p.rate, Loop: p.loop, LoopStart: max(0, p.opts.LoopStart), Session: p.opts.AudioSession, ProgressInterval: p.opts.ProgressInterval,
		Position: p.position})
}

func (p *AudioPlayer) send(c driver.AudioCommand) {
	p.tree.sendAudio(p.id, driver.AudioRequest{Command: c})
}

// Play plays the player from where it is, loading it first if needed, and calls done, when not nil, once it plays or with why it cannot, an AudioError. Where the platform plays no audio it fails at once with AudioUnavailable, without asking it.
func (p *AudioPlayer) Play(done func(error)) {
	if p.closed {
		return
	}
	if err := p.unplayable(); err != nil {
		p.failNow(err, done)
		return
	}
	// Playing and not paused since, it plays on; after a Pause the platform has not answered yet, it is asked to play again.
	if p.state == AudioPlayerPlaying && p.wanted {
		if done != nil {
			p.owner.Post(func() {
				if !p.closed {
					done(nil)
				}
			})
		}
		return
	}
	if done != nil {
		p.plays = append(p.plays, done)
	}
	switch p.state {
	case AudioPlayerIdle, AudioPlayerFailed:
		p.state = AudioPlayerLoading
	case AudioPlayerEnded:
		if !p.sought {
			p.position = 0
		}
	}
	p.wanted, p.sought = true, false
	p.open()
	p.send(driver.AudioResume)
}

// Preload has the platform load the player without playing it, so that Play starts at once.
func (p *AudioPlayer) Preload() {
	if p.closed {
		return
	}
	if err := p.unplayable(); err != nil {
		p.failNow(err, nil)
		return
	}
	if p.state != AudioPlayerIdle && p.state != AudioPlayerFailed {
		return
	}
	p.state = AudioPlayerLoading
	p.open()
	p.send(driver.AudioPreload)
}

// Pause pauses the player.
func (p *AudioPlayer) Pause() {
	if p.closed {
		return
	}
	p.wanted = false
	if p.opened {
		p.send(driver.AudioPause)
	}
}

// Seek moves the player to position, clamped to its duration once it is known. Paused, or before it loaded, it applies once the platform can seek, and Play starts from there.
func (p *AudioPlayer) Seek(position time.Duration) {
	if p.closed {
		return
	}
	position = max(position, 0)
	if p.duration > 0 {
		position = min(position, p.duration)
	}
	p.position = position
	if p.state == AudioPlayerEnded {
		p.sought = true
	}
	if p.opened {
		p.seeks++
		p.tree.sendAudio(p.id, driver.AudioRequest{Command: driver.AudioSeek, Position: position})
	}
}

// SetVolume sets the volume, from 0 to 1; 0 is 1, as Video's, and other values are clamped.
func (p *AudioPlayer) SetVolume(v float32) { p.set(audioVolume(v), p.rate, p.loop, p.muted) }

// SetMuted silences or restores the player without changing its volume or playback state.
func (p *AudioPlayer) SetMuted(muted bool) { p.set(p.volume, p.rate, p.loop, muted) }

// SetRate sets the playback speed, from 0.5 to 2; 0 is 1, as Video's, and other values are clamped.
func (p *AudioPlayer) SetRate(r float32) { p.set(p.volume, audioRate(r), p.loop, p.muted) }

// SetLoop repeats playback from the configured LoopStart at the end, with no OnEnd.
func (p *AudioPlayer) SetLoop(loop bool) { p.set(p.volume, p.rate, loop, p.muted) }

func (p *AudioPlayer) effectiveVolume() float32 {
	if p.muted {
		return 0
	}
	return p.volume
}

func (p *AudioPlayer) set(volume, rate float32, loop, muted bool) {
	if p.closed || volume == p.volume && rate == p.rate && loop == p.loop && muted == p.muted {
		return
	}
	p.volume, p.rate, p.loop, p.muted = volume, rate, loop, muted
	if p.opened {
		p.tree.sendAudio(p.id, driver.AudioRequest{Command: driver.AudioSet, Volume: p.effectiveVolume(), Rate: rate, Loop: loop})
	}
}

// Position returns where the player is, as its last report or seek said.
func (p *AudioPlayer) Position() time.Duration { return p.position }

// Duration returns how long the player plays, 0 until it loaded and for a stream without one.
func (p *AudioPlayer) Duration() time.Duration { return p.duration }

// State returns where the player is.
func (p *AudioPlayer) State() AudioPlayerState { return p.state }

// Close stops the player and releases the platform's; its callbacks, Play's among them, never run after it. Calling it again does nothing.
func (p *AudioPlayer) Close() {
	if p.closed {
		return
	}
	p.closed, p.wanted, p.plays = true, false, nil
	if p.opened {
		delete(p.rt.audioState().players, p.id)
		p.send(driver.AudioClose)
	}
}

// call runs fn under the player's owner while it is open.
func (p *AudioPlayer) call(fn func()) {
	if !p.closed {
		p.tree.within(p.owner, fn)
	}
}

// take takes the position of a report, unless it goes back without a seek or a loop starting over since the last, and reports whether the report came after the last seek.
func (p *AudioPlayer) take(e *driver.AudioEvent) bool {
	if e.Seeks < p.seeks {
		return false
	}
	if e.Duration > 0 {
		p.duration = e.Duration
	}
	fresh := e.Seeks != p.seen || e.Laps != p.laps
	p.seen, p.laps = e.Seeks, e.Laps
	if fresh || e.Position > p.position {
		p.position = e.Position
	}
	if p.duration > 0 {
		p.position = min(p.position, p.duration)
	}
	return true
}

// progressed calls OnProgress with where the player is.
func (p *AudioPlayer) progressed() {
	if f := p.opts.OnProgress; f != nil {
		position, duration := p.position, p.duration
		p.call(func() { f(position, duration) })
	}
}

// event takes a platform's report of the player.
func (p *AudioPlayer) event(e *driver.AudioEvent) {
	if p.closed {
		return
	}
	switch e.Phase {
	case driver.AudioOpened:
		p.duration = e.Duration
		if p.duration > 0 {
			p.position = min(p.position, p.duration)
		}
		if p.state == AudioPlayerLoading && !p.wanted {
			p.state = AudioPlayerPaused
		}
		if f := p.opts.OnLoad; f != nil {
			p.call(func() { f(e.Duration) })
		}
	case driver.AudioPlaying:
		p.state, p.playing = AudioPlayerPlaying, true
		plays := p.plays
		p.plays = nil
		for _, done := range plays {
			p.call(func() { done(nil) })
		}
	case driver.AudioPaused:
		if p.state == AudioPlayerEnded || p.state == AudioPlayerFailed {
			return
		}
		p.state, p.wanted, p.playing = AudioPlayerPaused, false, false
		p.take(e)
		p.progressed()
	case driver.AudioProgress:
		if p.take(e) {
			p.progressed()
		}
	case driver.AudioBuffer:
		switch {
		case e.Buffering && p.state == AudioPlayerPlaying:
			p.state = AudioPlayerLoading
		case !e.Buffering && p.state == AudioPlayerLoading && p.playing:
			p.state = AudioPlayerPlaying
		}
		if f := p.opts.OnBuffer; f != nil {
			p.call(func() { f(e.Buffering) })
		}
	case driver.AudioEnded:
		p.state, p.wanted, p.playing, p.sought = AudioPlayerEnded, false, false, false
		if p.duration > 0 {
			p.position = p.duration
		}
		p.progressed()
		if f := p.opts.OnEnd; f != nil {
			p.call(f)
		}
	case driver.AudioFailure:
		p.state, p.wanted, p.playing = AudioPlayerFailed, false, false
		code := AudioError(e.Error)
		if code == "" {
			code = AudioFailed
		}
		err := audioFail(code, e.Message)
		plays := p.plays
		p.plays = nil
		if f := p.opts.OnError; f != nil {
			p.call(func() { f(err) })
		}
		for _, done := range plays {
			p.call(func() { done(err) })
		}
	}
}

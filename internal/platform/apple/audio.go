//go:build darwin

package apple

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/audiosession"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

// Audio is one engine's sounds and players, the audio package's on macOS and iOS (docs/architecture/audio.md), as NSH's speaker: a surface makes it with its first ItemAudio and closes it as it closes, so an app that makes no sound never opens the output. Its methods run on the main thread; sounds are read and decoded on goroutines of their own, whose results hop back there.
//
// Sounds decode once, at their load, into an AVAudioPCMBuffer, through an AVAudioFile over a temporary file of their bytes as AVAudioFile reads only files, and play from memory on the engine's one AVAudioEngine: each playback is a voice, an AVAudioPlayerNode through an AVAudioUnitVarispeed, which gives the rate, and a mixer of its own, which gives the volume and stereo pan, as an effect between a player and the main mixer keeps the player's own mixing properties from it. Voices are reused once their buffer played, by sounds of their format; at the playback limit the oldest eligible voice stops. The engine starts at a playback, again after the system stopped it, as for a device change, and pauses a moment after the last playback ends, so the output is not held open; one that cannot start is silence, never an error.
//
// Players are AVPlayers without a layer, as Video's, made at their first preload or play: their load is polled until the item is ready or fails, as a paused player reports no time, their position comes from a periodic time observer, their end from AVPlayerItemDidPlayToEndTimeNotification and a stop no command asked for from AVPlayerRateDidChangeNotification, besides the time observer. On iOS each holds the shared audio session (internal/audiosession) at the level its AudioSession asks for while it plays, and playing sounds hold it ambient; an interruption of the session pauses the players as it begins, and they stay paused as it ends.
type Audio struct {
	dispatch func(driver.Event)
	engine   objc.ID
	sounds   map[uint32]*decodedSound
	// loading are the sounds decoding: an unload before the decode finished takes the ID out, and the decoded sound is freed.
	loading map[uint32]bool
	voices  []*soundVoice
	players map[uint32]*audioPlayer
	// soundsHeld lets go of the session the sounds hold while they play, nil while they hold none; idle pauses the engine and lets it go once the last playback ended.
	soundsHeld    func()
	idle          *time.Timer
	idleAt        time.Time
	unwatchEngine func()
	// interruptions is the observer of AVAudioSessionInterruptionNotification, 0 until AVFoundation loaded.
	interruptions objc.ID
	closed        bool
}

// decodedSound is a sound decoded into memory, its buffer and format retained.
type decodedSound struct {
	buffer, format objc.ID
	duration       time.Duration
}

// soundVoice is a player node, its varispeed and its mixer, attached to the engine.
type soundVoice struct {
	player, varispeed, mixer objc.ID
	mainBus                  uint
	// format is the one it is connected with, retained.
	format objc.ID
	// sound is the one it plays, 0 for none, and playback the core's handle of it, 0 for an untracked one. generation distinguishes a queued native completion from the next play on this node. restricted is set for a restricted scope's playback.
	sound, playback uint32
	generation      uint64
	started         time.Time
	restricted      bool
}

// idleGrace is how long the engine keeps the output open after the last playback ends, so that the next sound starts at once.
const idleGrace = 2 * time.Second

// loadAudio loads AVFoundation, with AVFAudio, reporting whether it has what sounds and players need.
var loadAudio = sync.OnceValue(func() bool {
	if _, err := purego.Dlopen(avPath, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		return false
	}
	return Class("AVAudioEngine") != 0 && Class("AVPlayer") != 0
})

// AudioAvailable is Environment.Audio: every macOS and iOS NSGo runs on has AVFoundation, which loads only once a sound or a player does.
func AudioAvailable() bool { return true }

// NewAudio returns an engine's sounds and players, reporting EventAudio through dispatch.
func NewAudio(dispatch func(driver.Event)) *Audio {
	return &Audio{dispatch: dispatch, sounds: map[uint32]*decodedSound{}, loading: map[uint32]bool{}, players: map[uint32]*audioPlayer{}}
}

func (a *Audio) report(id uint32, e driver.AudioEvent) {
	a.dispatch(driver.Event{Kind: driver.EventAudio, Count: id, Data: &e})
}

func (a *Audio) fail(id uint32, code, message string) {
	a.report(id, driver.AudioEvent{Phase: driver.AudioFailure, Error: code, Message: message})
}

// Handle carries out an ItemAudio for sound or player id, on the main thread.
func (a *Audio) Handle(id uint32, r *driver.AudioRequest) {
	if a.closed || r == nil {
		return
	}
	if !loadAudio() {
		switch r.Command {
		case driver.AudioLoad, driver.AudioOpen:
			a.fail(id, driver.AudioUnavailable, "AVFoundation is not available")
		case driver.AudioReadLatency:
			a.report(id, driver.AudioEvent{Phase: driver.AudioLatency})
		case driver.AudioPlay:
			a.playbackEnded(id, r.Playback)
		}
		return
	}
	if r.Command == driver.AudioReadLatency {
		a.report(id, driver.AudioEvent{Phase: driver.AudioLatency, Latency: a.outputLatency()})
		return
	}
	a.watchInterruptions()
	switch r.Command {
	case driver.AudioLoad:
		a.load(id, *r)
	case driver.AudioPlay:
		a.play(id, r)
	case driver.AudioPlaybackSet, driver.AudioPlaybackStop:
		a.controlPlayback(id, r)
	case driver.AudioStop:
		a.stop(id)
	case driver.AudioUnload:
		a.unload(id)
	case driver.AudioOpen:
		if old := a.players[id]; old != nil {
			old.close()
		}
		a.players[id] = &audioPlayer{audio: a, id: id, src: r.Source, volume: r.Volume, rate: r.Rate, loop: r.Loop,
			session: r.Session, interval: max(r.ProgressInterval, 16*time.Millisecond), position: r.Position, loopStart: r.LoopStart}
	case driver.AudioClose:
		if p := a.players[id]; p != nil {
			delete(a.players, id)
			p.close()
		}
	default:
		if p := a.players[id]; p != nil {
			p.apply(r)
		}
	}
}

// watchInterruptions has an interruption of the audio session pause the players as it begins, once. Only iOS posts AVAudioSessionInterruptionNotification. The players stay paused as the interruption ends, even when its options say they may resume: the core has them paused by the system, as for a call, and the app's Play resumes them, as after the app left the foreground (docs/architecture/audio.md).
func (a *Audio) watchInterruptions() {
	if a.interruptions != 0 {
		return
	}
	center := objc.ID(Class("NSNotificationCenter")).Send(sel("defaultCenter"))
	queue := objc.ID(Class("NSOperationQueue")).Send(selMainQueue)
	block := objcblock.New(func(_ objc.Block, note objc.ID) {
		if !a.closed && interruptionBegan(note) {
			a.endSoundVoices()
			a.PausePlayers()
		}
	})
	// A nil object observes the shared AVAudioSession's.
	a.interruptions = center.Send(selAddObserver, avName("AVAudioSessionInterruptionNotification"), objc.ID(0), queue, block).Send(sel("retain"))
	block.Release()
}

// interruptionBegan reports whether note, an AVAudioSessionInterruptionNotification, says the interruption began rather than ended.
func interruptionBegan(note objc.ID) bool {
	const began = 1 // AVAudioSessionInterruptionTypeBegan
	info := note.Send(sel("userInfo"))
	if info == 0 {
		return false
	}
	kind := info.Send(selObjectForKey, avName("AVAudioSessionInterruptionTypeKey"))
	return kind != 0 && objc.Send[uint](kind, sel("unsignedIntegerValue")) == began
}

// PausePlayers pauses the players playing, as the system would: what an iOS app does as it leaves the foreground, as NSH's do, and as an interruption begins.
func (a *Audio) PausePlayers() {
	for _, p := range a.players {
		switch {
		case p.playing:
			p.pause()
		case p.wantsPlay:
			// A play waiting for the load waits no more.
			p.wantsPlay = false
			p.report(driver.AudioEvent{Phase: driver.AudioPaused, Position: p.position, Seeks: p.seeks, Laps: p.laps})
		}
	}
}

// Close stops every playback and player and frees the sounds; nothing is reported after it. Calling it again does nothing.
func (a *Audio) Close() {
	if a.closed {
		return
	}
	a.closed = true
	for id, p := range a.players {
		delete(a.players, id)
		p.close()
	}
	for id := range a.sounds {
		a.unload(id)
	}
	clear(a.loading)
	if a.idle != nil {
		a.idle.Stop()
	}
	if a.unwatchEngine != nil {
		a.unwatchEngine()
	}
	if a.engine != 0 {
		a.engine.Send(sel("stop"))
		for _, v := range a.voices {
			v.drop(a.engine)
		}
		a.voices = nil
		a.engine.Send(sel("release"))
		a.engine = 0
	}
	a.letSoundsGo()
	if a.interruptions != 0 {
		objc.ID(Class("NSNotificationCenter")).Send(sel("defaultCenter")).Send(sel("removeObserver:"), a.interruptions)
		a.interruptions.Send(sel("release"))
		a.interruptions = 0
	}
}

// --- Sounds ------------------------------------------------------------------

// unload stops sound id's playbacks and frees it, or forgets its load.
func (a *Audio) unload(id uint32) {
	a.stop(id)
	delete(a.loading, id)
	if s := a.sounds[id]; s != nil {
		delete(a.sounds, id)
		s.buffer.Send(sel("release"))
		s.format.Send(sel("release"))
	}
}

// load reads and decodes sound id on a goroutine of its own, then keeps it and reports its load on the main thread, unless it was unloaded, or the engine closed, in the meantime.
func (a *Audio) load(id uint32, r driver.AudioRequest) {
	a.loading[id] = true
	go func() {
		sound, e, code, message := readSound(r)
		OnMain(func() {
			if a.closed || !a.loading[id] {
				if sound != nil {
					sound.buffer.Send(sel("release"))
					sound.format.Send(sel("release"))
				}
				return
			}
			delete(a.loading, id)
			if code != "" {
				a.fail(id, code, message)
				return
			}
			if sound != nil {
				a.sounds[id] = sound
			}
			a.report(id, e)
		})
	}()
}

// readSound reads and decodes the source within the request's size and duration limits: the sound, nil when it is cut as the core frees a cut sound, and its AudioLoaded; or an error's code and message.
func readSound(r driver.AudioRequest) (*decodedSound, driver.AudioEvent, string, string) {
	data, code, message := readAudio(r.Source, r.MaxBytes)
	if code != "" {
		return nil, driver.AudioEvent{}, code, message
	}
	path, err := audioTempFile(data, r.Source.URI)
	if err != nil {
		return nil, driver.AudioEvent{}, driver.AudioFailed, "cannot write the sound for decoding: " + err.Error()
	}
	defer os.Remove(path)
	var sound *decodedSound
	var e driver.AudioEvent
	maxDecodedBytes := cmp.Or(r.MaxDecodedBytes, int64(driver.MaxDecodedBytes))
	withPool(func() { sound, e, code, message = decodeSound(path, r.MaxDuration, maxDecodedBytes) })
	return sound, e, code, message
}

// readAudio reads src's bytes as an image's are read, failing past maxBytes when it is above 0: the bytes, or an error's code and message.
func readAudio(src driver.ImageSource, maxBytes int64) ([]byte, string, string) {
	if maxBytes > 0 && int64(len(src.Data)) > maxBytes {
		return nil, driver.AudioQuotaExceeded, fmt.Sprintf("the sound exceeds its limit of %d encoded bytes", maxBytes)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var over atomic.Bool
	progress := func(loaded, total int64) {
		if maxBytes > 0 && (loaded > maxBytes || total > maxBytes) {
			over.Store(true)
			cancel()
		}
	}
	data, err := imageload.Load(ctx, src, progress)
	if over.Load() || maxBytes > 0 && int64(len(data)) > maxBytes {
		return nil, driver.AudioQuotaExceeded, fmt.Sprintf("the sound exceeds its limit of %d encoded bytes", maxBytes)
	}
	if err != nil {
		// A source that cannot be fetched is a network error; one that cannot be read here is none the platform plays, as NSH has them.
		code := driver.AudioNotSupported
		if remote(src.URI) {
			code = driver.AudioNetwork
		}
		return nil, code, "cannot read " + describeSource(src) + ": " + err.Error()
	}
	return data, "", ""
}

// remote reports whether uri is an http(s) URL.
func remote(uri string) bool {
	scheme, _, ok := strings.Cut(uri, "://")
	return ok && (strings.EqualFold(scheme, "http") || strings.EqualFold(scheme, "https"))
}

// describeSource names src in a message: its URI, without a data: URL's bytes.
func describeSource(src driver.ImageSource) string {
	switch {
	case src.URI == "":
		return "the bytes"
	case strings.HasPrefix(src.URI, "data:"):
		return "the data: URL"
	}
	return src.URI
}

// audioExtensions are the file extensions AudioToolbox names the formats driver.AudioType tells by.
var audioExtensions = map[string]string{"audio/wav": ".wav", "audio/aiff": ".aiff", "audio/x-caf": ".caf", "audio/flac": ".flac",
	"audio/ogg": ".ogg", "audio/mp4": ".m4a", "audio/mpeg": ".mp3", "audio/aac": ".aac"}

// audioExtension is the extension of a file holding data: the one its first bytes name, else that of uri's path when it is a plain one.
func audioExtension(data []byte, uri string) string {
	if ext := audioExtensions[driver.AudioType(data)]; ext != "" {
		return ext
	}
	if strings.HasPrefix(uri, "data:") {
		return ""
	}
	if u, err := url.Parse(uri); err == nil && u.Path != "" {
		uri = u.Path
	}
	ext := strings.ToLower(filepath.Ext(uri))
	if len(ext) < 2 || len(ext) > 5 {
		return ""
	}
	for _, c := range ext[1:] {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return ""
		}
	}
	return ext
}

// audioTempFile writes data to a temporary file named for its format, which AudioToolbox goes by for formats without a header, and returns its path.
func audioTempFile(data []byte, uri string) (string, error) {
	f, err := os.CreateTemp("", "nsgo-audio-*"+audioExtension(data, uri))
	if err != nil {
		return "", err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// decodeSound decodes the file at path into memory, at most max of it, inside an autorelease pool: the sound and its AudioLoaded, or why not. A sound longer than max is reported cut and not decoded, as the core frees it.
func decodeSound(path string, max time.Duration, maxDecodedBytes int64) (*decodedSound, driver.AudioEvent, string, string) {
	fileClass, bufferClass := Class("AVAudioFile"), Class("AVAudioPCMBuffer")
	if fileClass == 0 || bufferClass == 0 {
		return nil, driver.AudioEvent{}, driver.AudioDecode, "AVAudioFile is not available"
	}
	fileURL := audioFileURL(path)
	if fileURL == 0 {
		return nil, driver.AudioEvent{}, driver.AudioDecode, "cannot name the sound's file"
	}
	var nsErr objc.ID
	file := objc.ID(fileClass).Send(sel("alloc")).Send(sel("initForReading:error:"), fileURL, unsafe.Pointer(&nsErr))
	if file == 0 {
		return nil, driver.AudioEvent{}, driver.AudioDecode, describeError(nsErr, "the sound cannot be decoded")
	}
	defer file.Send(sel("release"))
	format := file.Send(sel("processingFormat"))
	rate := objc.Send[float64](format, sel("sampleRate"))
	channels := objc.Send[uint32](format, sel("channelCount"))
	length := objc.Send[int64](file, sel("length"))
	if rate <= 0 || length <= 0 || channels == 0 {
		return nil, driver.AudioEvent{}, driver.AudioDecode, "the sound is empty"
	}
	if float64(length) > math.Ceil(max.Seconds()*rate) {
		return nil, driver.AudioEvent{Phase: driver.AudioLoaded, Duration: max, Cut: true}, "", ""
	}
	bytesPerFrame := int64(channels) * 4 // the processing format is 32-bit float
	if length > math.MaxUint32 || length > maxDecodedBytes/bytesPerFrame {
		return nil, driver.AudioEvent{}, driver.AudioQuotaExceeded, "the sound exceeds its decoded size limit"
	}
	frames := uint32(length)
	buffer := objc.ID(bufferClass).Send(sel("alloc")).Send(sel("initWithPCMFormat:frameCapacity:"), format, frames)
	if buffer == 0 {
		return nil, driver.AudioEvent{}, driver.AudioDecode, "no memory for the sound"
	}
	if !objc.Send[bool](file, sel("readIntoBuffer:frameCount:error:"), buffer, frames, unsafe.Pointer(&nsErr)) {
		buffer.Send(sel("release"))
		return nil, driver.AudioEvent{}, driver.AudioDecode, describeError(nsErr, "the sound cannot be decoded")
	}
	decoded := objc.Send[uint32](buffer, sel("frameLength"))
	if decoded == 0 {
		buffer.Send(sel("release"))
		return nil, driver.AudioEvent{}, driver.AudioDecode, "the sound is empty"
	}
	duration := time.Duration(float64(decoded) / rate * float64(time.Second))
	return &decodedSound{buffer: buffer, format: format.Send(sel("retain")), duration: duration},
		driver.AudioEvent{Phase: driver.AudioLoaded, Duration: duration, Bytes: int64(decoded) * bytesPerFrame}, "", ""
}

// audioFileURL is the file NSURL of path, 0 for a path no NSURL can name: one that is not UTF-8, as Apple's file systems name files, or holds a NUL, for which NSString gives nothing and NSURL would raise.
func audioFileURL(path string) objc.ID {
	if path == "" || !utf8.ValidString(path) || strings.IndexByte(path, 0) >= 0 {
		return 0
	}
	name := NSString(path)
	if name == 0 {
		return 0
	}
	return objc.ID(Class("NSURL")).Send(sel("fileURLWithPath:"), name)
}

// describeError is fallback with the localized description of nsErr, an NSError or 0.
func describeError(nsErr objc.ID, fallback string) string {
	if nsErr == 0 {
		return fallback
	}
	if text := GoString(nsErr.Send(sel("localizedDescription"))); text != "" {
		return fallback + ": " + text
	}
	return fallback
}

// audioEngine creates the output graph without starting playback.
func (a *Audio) audioEngine() objc.ID {
	if a.engine == 0 {
		c := Class("AVAudioEngine")
		if c == 0 {
			return 0
		}
		a.engine = objc.ID(c).Send(sel("alloc")).Send(sel("init"))
		// The main mixer exists, connected to the output, once asked for.
		a.engine.Send(sel("mainMixerNode"))
		a.watchEngine()
	}
	return a.engine
}

// startEngine starts the graph if needed, including after a system stop.
func (a *Audio) startEngine() objc.ID {
	if a.audioEngine() == 0 {
		return 0
	}
	if !objc.Send[bool](a.engine, sel("isRunning")) {
		a.endSoundVoices()
		var nsErr objc.ID
		if !objc.Send[bool](a.engine, sel("startAndReturnError:"), unsafe.Pointer(&nsErr)) {
			log.Print("nsgo: " + describeError(nsErr, "the audio engine did not start"))
			return 0
		}
	}
	return a.engine
}

// watchEngine ends the sounds whose engine stopped during a route change.
func (a *Audio) watchEngine() {
	center := objc.ID(Class("NSNotificationCenter")).Send(sel("defaultCenter"))
	// A configuration notification may post on the audio engine's thread. It must return before main calls back into the engine.
	block := objcblock.New(func(objc.Block, objc.ID) {
		OnMain(func() {
			if !a.closed && !objc.Send[bool](a.engine, sel("isRunning")) {
				a.endSoundVoices()
			}
		})
	})
	name := avName("AVAudioEngineConfigurationChangeNotification")
	token := center.Send(selAddObserver, name, a.engine, objc.ID(0), block).Send(sel("retain"))
	block.Release()
	a.unwatchEngine = func() {
		center.Send(sel("removeObserver:"), token)
		token.Send(sel("release"))
	}
}

// newVoice attaches a mixer for the life of a voice.
func newVoice(engine, format objc.ID) *soundVoice {
	mixer := objc.ID(Class("AVAudioMixerNode")).Send(sel("alloc")).Send(sel("init"))
	if mixer == 0 {
		return nil
	}
	v := &soundVoice{mixer: mixer}
	if !v.connectSource(engine, format) {
		mixer.Send(sel("release"))
		return nil
	}
	return v
}

// connectSource connects new nodes upstream of the voice mixer. A varispeed
// node keeps its input format, and removing a mixer from a running
// AVAudioEngine can break the graph.
func (v *soundVoice) connectSource(engine, format objc.ID) bool {
	player := objc.ID(Class("AVAudioPlayerNode")).Send(sel("alloc")).Send(sel("init"))
	varispeed := objc.ID(Class("AVAudioUnitVarispeed")).Send(sel("alloc")).Send(sel("init"))
	if player == 0 || varispeed == 0 {
		player.Send(sel("release"))
		varispeed.Send(sel("release"))
		return false
	}
	main := engine.Send(sel("mainMixerNode"))
	if v.player == 0 {
		engine.Send(sel("attachNode:"), v.mixer)
		v.mainBus = objc.Send[uint](main, sel("nextAvailableInputBus"))
	} else {
		v.dropSource(engine)
	}
	engine.Send(sel("attachNode:"), player)
	engine.Send(sel("attachNode:"), varispeed)
	engine.Send(sel("connect:to:format:"), player, varispeed, format)
	engine.Send(sel("connect:to:format:"), varispeed, v.mixer, format)
	// Keep the existing main mixer bus and send it the source format so the
	// main mixer can adapt that audio to the current output device.
	engine.Send(sel("connect:to:fromBus:toBus:format:"), v.mixer, main, uint(0), v.mainBus, format)
	v.player, v.varispeed, v.format = player, varispeed, format.Send(sel("retain"))
	return true
}

// dropSource disconnects the input chain before releasing its nodes.
func (v *soundVoice) dropSource(engine objc.ID) {
	v.player.Send(sel("stop"))
	engine.Send(sel("disconnectNodeOutput:"), v.varispeed)
	for _, node := range []objc.ID{v.player, v.varispeed} {
		engine.Send(sel("detachNode:"), node)
		node.Send(sel("release"))
	}
	v.format.Send(sel("release"))
}

// drop releases the mixer too, after the engine stopped.
func (v *soundVoice) drop(engine objc.ID) {
	v.dropSource(engine)
	engine.Send(sel("detachNode:"), v.mixer)
	v.mixer.Send(sel("release"))
}

// voiceFor takes a free voice of the sound's format or creates one up to limit. Past limit playing voices, or past MaxSoundVoices restricted ones for a restricted playback, it takes the oldest of those, ending its playback; otherwise it reuses a free voice of another format, connecting the sound's format to it.
func (a *Audio) voiceFor(engine objc.ID, s *decodedSound, limit int, restricted bool) *soundVoice {
	same := func(v *soundVoice) bool { return objc.Send[bool](v.format, sel("isEqual:"), s.format) }
	var free, matching, oldest, oldestRestricted *soundVoice
	active, restrictedActive := 0, 0
	for _, v := range a.voices {
		if v.sound == 0 {
			if matching == nil && same(v) {
				matching = v
			}
			free = v
			continue
		}
		active++
		if oldest == nil || v.started.Before(oldest.started) {
			oldest = v
		}
		if v.restricted {
			restrictedActive++
			if oldestRestricted == nil || v.started.Before(oldestRestricted.started) {
				oldestRestricted = v
			}
		}
	}
	var v *soundVoice
	switch {
	case restricted && restrictedActive >= driver.MaxSoundVoices:
		v = oldestRestricted
		a.finishVoice(v)
	case active >= limit:
		v = oldest
		a.finishVoice(v)
	case matching != nil:
		return matching
	case len(a.voices) < limit:
		v = newVoice(engine, s.format)
		if v != nil {
			a.voices = append(a.voices, v)
		}
		return v
	default:
		v = free
	}
	if v == nil {
		return nil
	}
	if same(v) {
		v.player.Send(sel("stop"))
		return v
	}
	if !v.connectSource(engine, s.format) {
		return nil
	}
	return v
}

// play starts a playback of sound id at the request's volume, rate, pan and loop, on the voice voiceFor gives.
func (a *Audio) play(id uint32, r *driver.AudioRequest) {
	s := a.sounds[id]
	if s == nil {
		a.playbackEnded(id, r.Playback)
		return
	}
	a.holdForSounds()
	engine := a.startEngine()
	if engine == 0 {
		a.playbackEnded(id, r.Playback)
		a.idleLater()
		return
	}
	limit := max(1, cmp.Or(r.MaxPlaybacks, driver.MaxSoundVoices))
	v := a.voiceFor(engine, s, limit, r.Restricted)
	if v == nil {
		a.playbackEnded(id, r.Playback)
		a.idleLater()
		return
	}
	if a.idle != nil {
		a.idle.Stop()
	}
	v.generation++
	gen := v.generation
	v.sound, v.playback, v.started, v.restricted = id, r.Playback, time.Now(), r.Restricted
	v.varispeed.Send(sel("setRate:"), r.Rate)
	v.mixer.Send(sel("setOutputVolume:"), r.Volume)
	v.mixer.Send(sel("setPan:"), r.Pan)
	// The played-back completion accounts for device latency and live rate changes. Leave the native callback before resetting or reusing a node.
	completed := objcblock.New(func(_ objc.Block, _ int) {
		OnMain(func() {
			if !a.closed && v.generation == gen && v.sound != 0 {
				a.finishVoice(v)
			}
		})
	})
	// A looping buffer completes only as the node stops, after finishVoice has let its playback go.
	const loops, playedBack = 1, 2 // AVAudioPlayerNodeBufferLoops, AVAudioPlayerNodeCompletionDataPlayedBack
	options := uint(0)
	if r.Loop {
		options = loops
	}
	v.player.Send(sel("scheduleBuffer:atTime:options:completionCallbackType:completionHandler:"), s.buffer, objc.ID(0), options, playedBack, completed)
	completed.Release()
	if at := scheduledAudioTime(r.StartAt); at != 0 {
		v.player.Send(sel("playAtTime:"), at)
	} else {
		v.player.Send(sel("play"))
	}
}

func (a *Audio) controlPlayback(id uint32, r *driver.AudioRequest) {
	if r.Playback == 0 {
		return
	}
	for _, v := range a.voices {
		if v.sound == id && v.playback == r.Playback {
			if r.Command == driver.AudioPlaybackStop {
				a.finishVoice(v)
			} else {
				v.varispeed.Send(sel("setRate:"), r.Rate)
				v.mixer.Send(sel("setOutputVolume:"), r.Volume)
			}
			return
		}
	}
}

func (a *Audio) playbackEnded(sound, playback uint32) {
	if !a.closed && playback != 0 {
		a.report(sound, driver.AudioEvent{Phase: driver.AudioPlaybackEnded, Playback: playback})
	}
}

// finishVoice disconnects the playback's identity before stop can enqueue its native completion.
func (a *Audio) finishVoice(v *soundVoice) {
	sound, playback := v.sound, v.playback
	v.sound, v.playback = 0, 0
	v.player.Send(sel("stop"))
	a.playbackEnded(sound, playback)
	a.idleLater()
}

// stop stops every playback of sound id.
func (a *Audio) stop(id uint32) {
	for _, v := range a.voices {
		if v.sound == id {
			a.finishVoice(v)
		}
	}
}

func (a *Audio) endSoundVoices() {
	for _, v := range a.voices {
		if v.sound != 0 {
			a.finishVoice(v)
		}
	}
}

// holdForSounds has the sounds hold the session, ambient, while they play.
func (a *Audio) holdForSounds() {
	if a.soundsHeld != nil {
		return
	}
	release, err := audiosession.Hold(audiosession.Ambient)
	if err != nil {
		log.Print("nsgo: cannot set and activate the audio session for a sound")
		return
	}
	a.soundsHeld = release
}

func (a *Audio) letSoundsGo() {
	if a.soundsHeld != nil {
		a.soundsHeld()
		a.soundsHeld = nil
	}
}

// idleLater pauses the engine after the last playback and an idle grace period.
func (a *Audio) idleLater() {
	if a.closed || a.soundsPlaying() {
		return
	}
	a.idleAt = time.Now().Add(idleGrace)
	if a.idle == nil {
		a.idle = time.AfterFunc(idleGrace, func() { OnMain(a.idleNow) })
		return
	}
	a.idle.Reset(idleGrace)
}

func (a *Audio) soundsPlaying() bool {
	for _, v := range a.voices {
		if v.sound != 0 {
			return true
		}
	}
	return false
}

func (a *Audio) idleNow() {
	if a.closed || a.soundsPlaying() {
		return
	}
	if wait := time.Until(a.idleAt); wait > 0 {
		a.idle.Reset(wait)
		return
	}
	if a.engine != 0 && objc.Send[bool](a.engine, sel("isRunning")) {
		// Paused, the engine keeps its nodes and connections but no longer runs the output, so the session can be deactivated.
		a.engine.Send(sel("pause"))
	}
	a.letSoundsGo()
}

// --- Players -----------------------------------------------------------------

// audioPlayer is a player and what it reported, on the main thread.
type audioPlayer struct {
	audio        *Audio
	id           uint32
	src          driver.ImageSource
	volume, rate float32
	loop         bool
	session      driver.AudioSession
	interval     time.Duration
	player, item objc.ID
	timeToken    objc.ID
	stops        []func()
	// file is the temporary file holding bytes the player plays, removed with it.
	file string
	// position is where its next load starts it: where AudioOpen or a seek asked it to be before it could seek, then where it last reported it was, so a load after a failure plays on from where the core has it.
	position time.Duration
	// opening says the player is being made, its source read on a goroutine; generation counts the makings, so one that finishes after a close finds the player went.
	opening                                       bool
	generation                                    int
	loaded, failed, buffering, wantsPlay, playing bool
	// ended says it played to its end with no seek since, so a play starts it over.
	ended bool
	// remote says its source is an http(s) URL, whose failures are network errors.
	remote      bool
	seeks, laps uint32
	looper      objc.ID
	intro       objc.ID
	loopStart   time.Duration
	loopBase    uint32
	seekSerial  uint64
	seeking     bool
	// release lets go of the audio session it holds while it plays, nil while it holds none.
	release func()
}

func (p *audioPlayer) report(e driver.AudioEvent) {
	if e.Phase == driver.AudioProgress || e.Phase == driver.AudioPaused {
		p.position = e.Position
	}
	p.audio.report(p.id, e)
}

// fail reports the player failed, once, and stops it until a play or a preload loads it again.
func (p *audioPlayer) fail(code, message string) {
	if p.failed {
		return
	}
	p.failed, p.playing, p.wantsPlay = true, false, false
	if p.player != 0 {
		p.player.Send(sel("pause"))
	}
	p.letGo()
	p.audio.fail(p.id, code, message)
}

func (p *audioPlayer) apply(r *driver.AudioRequest) {
	switch r.Command {
	case driver.AudioPreload:
		p.retry()
		p.open()
	case driver.AudioResume:
		p.retry()
		p.open()
		if p.loaded {
			p.start()
		} else {
			p.wantsPlay = true
		}
	case driver.AudioPause:
		p.pause()
	case driver.AudioSeek:
		p.seeks++
		p.ended = false
		p.position = max(r.Position, 0)
		if p.loaded {
			position := p.position
			p.report(p.progress(position))
			p.seek(position)
		}
	case driver.AudioSet:
		volumeChanged, rateChanged := p.volume != r.Volume, p.rate != r.Rate
		p.volume, p.rate = r.Volume, r.Rate
		p.setLoop(r.Loop)
		if p.player != 0 {
			if volumeChanged {
				p.player.Send(sel("setVolume:"), p.volume)
			}
			if p.playing && rateChanged {
				p.player.Send(sel("setRate:"), p.rate)
			}
		}
	}
}

// retry lets a failed player go, so that the open after it, a preload's or a play's, loads it again at where it last reported it was, or from the start if it had ended.
func (p *audioPlayer) retry() {
	if !p.failed {
		return
	}
	p.close()
	p.failed, p.loaded = false, false
}

// progress is an AudioProgress at position.
func (p *audioPlayer) progress(position time.Duration) driver.AudioEvent {
	p.countNativeLoops()
	d := p.duration()
	if d > 0 {
		position = min(position, d)
	}
	return driver.AudioEvent{Phase: driver.AudioProgress, Position: position, Duration: d, Seeks: p.seeks, Laps: p.laps}
}

// open makes the AVPlayer once, reading a source of bytes into a temporary file first, on a goroutine.
func (p *audioPlayer) open() {
	if p.player != 0 || p.opening || p.failed {
		return
	}
	p.remote = len(p.src.Data) == 0 && remote(p.src.URI)
	if p.remote {
		p.make(p.src.URI, "")
		return
	}
	p.opening = true
	p.generation++
	generation, src := p.generation, p.src
	go func() {
		path, temp, code, message := resolvePlayerSource(src)
		OnMain(func() {
			if p.generation != generation || !p.opening || p.audio.closed || p.audio.players[p.id] != p {
				if temp {
					os.Remove(path)
				}
				return
			}
			p.opening = false
			if temp {
				p.file = path
			}
			if code != "" {
				p.fail(code, message)
				return
			}
			p.make("", path)
		})
	}()
}

// resolvePlayerSource is the file a player of src plays: an "asset:" path under the app's resources, a file path or file: URL, or a temporary file holding bytes, a data: URL's among them, temp saying so; or an error's code and message.
func resolvePlayerSource(src driver.ImageSource) (path string, temp bool, code, message string) {
	uri := src.URI
	if len(src.Data) > 0 || strings.HasPrefix(uri, "data:") {
		data, code, message := readAudio(src, 0)
		if code != "" {
			return "", false, code, message
		}
		path, err := audioTempFile(data, uri)
		if err != nil {
			return "", false, driver.AudioFailed, "cannot write the audio for playing: " + err.Error()
		}
		return path, true, "", ""
	}
	switch {
	case strings.HasPrefix(uri, "asset:"):
		_, path, _ = locate(uri, imageload.ResourceRoot)
	case strings.HasPrefix(strings.ToLower(uri), "file:"):
		u, err := url.Parse(uri)
		if err != nil {
			return "", false, driver.AudioNotSupported, "cannot read " + uri + ": " + err.Error()
		}
		path = u.Path
	case strings.Contains(uri, "://"):
		return "", false, driver.AudioNotSupported, uri + ": a scheme the platform does not play"
	default:
		path = uri
	}
	if !utf8.ValidString(path) || strings.IndexByte(path, 0) >= 0 {
		return "", false, driver.AudioNotSupported, "cannot read " + uri + ": the path is not UTF-8 text"
	}
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		err = errors.New("a directory")
	}
	if err != nil {
		return "", false, driver.AudioNotSupported, "cannot read " + uri + ": " + err.Error()
	}
	return path, false, "", ""
}

// make opens the first item of the queue and watches its load, time and end.
func (p *audioPlayer) make(rawURL, path string) {
	var u objc.ID
	options := objc.ID(0)
	if rawURL != "" {
		if utf8.ValidString(rawURL) && strings.IndexByte(rawURL, 0) < 0 {
			u = objc.ID(Class("NSURL")).Send(sel("URLWithString:"), NSString(rawURL))
		}
	} else {
		u = audioFileURL(path)
		// As for videos: AVFoundation otherwise estimates a variable-bitrate file's timing, and a seek lands elsewhere than it says.
		options = objc.ID(Class("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"),
			objc.ID(Class("NSNumber")).Send(sel("numberWithBool:"), true), NSString("AVURLAssetPreferPreciseDurationAndTimingKey"))
	}
	if u == 0 {
		p.fail(driver.AudioNotSupported, "cannot name "+describeSource(p.src)+" as a URL")
		return
	}
	asset := objc.ID(Class("AVURLAsset")).Send(sel("URLAssetWithURL:options:"), u, options)
	p.item = objc.ID(Class("AVPlayerItem")).Send(sel("playerItemWithAsset:"), asset).Send(sel("retain"))
	p.openQueue(p.item)
	p.poll(p.generation, 0)
}

func (p *audioPlayer) openQueue(intro objc.ID) {
	p.intro = intro.Send(sel("retain"))
	items := objc.ID(Class("NSArray")).Send(sel("arrayWithObject:"), intro)
	p.player = objc.ID(Class("AVQueuePlayer")).Send(sel("queuePlayerWithItems:"), items).Send(sel("retain"))
	p.player.Send(sel("setActionAtItemEnd:"), playerPauseAtEnd)
	p.player.Send(sel("setVolume:"), p.volume)
	p.observe()
}

// observe watches the player's time every interval, as it starts and stops and as its time jumps, its rate, and its item's end and failure.
func (p *audioPlayer) observe() {
	player, generation := p.player, p.generation
	// The block's CMTime argument is left out: the player's current time is the same, and purego callbacks take no structs.
	tick := objcblock.New(func(objc.Block) {
		if p.generation == generation && p.player == player {
			p.tick()
		}
	})
	// A nil queue is the main queue.
	p.timeToken = p.player.Send(sel("addPeriodicTimeObserverForInterval:queue:usingBlock:"), cmTimeOf(p.interval), objc.ID(0), tick).Send(sel("retain"))
	tick.Release()
	center := objc.ID(Class("NSNotificationCenter")).Send(sel("defaultCenter"))
	queue := objc.ID(Class("NSOperationQueue")).Send(selMainQueue)
	watch := func(name string, object objc.ID, fn func(note objc.ID)) {
		block := objcblock.New(func(_ objc.Block, note objc.ID) {
			if p.generation == generation && p.player == player {
				fn(note)
			}
		})
		// A nil name would observe every notification of the object.
		token := center.Send(selAddObserver, avName(name), object, queue, block).Send(sel("retain"))
		block.Release()
		p.stops = append(p.stops, func() {
			center.Send(sel("removeObserver:"), token)
			token.Send(sel("release"))
		})
	}
	watch("AVPlayerItemDidPlayToEndTimeNotification", 0, func(note objc.ID) {
		if note.Send(sel("object")) == p.intro {
			p.reachedEnd()
		}
	})
	watch("AVPlayerItemFailedToPlayToEndTimeNotification", 0, func(note objc.ID) {
		if note.Send(sel("object")) != p.player.Send(sel("currentItem")) {
			return
		}
		var nsErr objc.ID
		if info := note.Send(sel("userInfo")); info != 0 {
			nsErr = info.Send(selObjectForKey, avName("AVPlayerItemFailedToPlayToEndTimeErrorKey"))
		}
		p.fail(p.failure(), describeError(nsErr, "the audio stopped playing"))
	})
	// The time observer's call as the player stops can come while its rate is not yet 0, with none after, as for an interruption on iOS: the rate's change reports the stop, from macOS 12 and iOS 15, which post it.
	watch("AVPlayerRateDidChangeNotification", p.player, func(objc.ID) {
		if p.playing && !p.failed && objc.Send[float32](p.player, sel("rate")) == 0 {
			p.stopped()
		}
	})
}

// failure is the code of a failure while loading or playing: a network error for an http(s) source, else one decoding.
func (p *audioPlayer) failure() string {
	if p.remote {
		return driver.AudioNetwork
	}
	return driver.AudioDecode
}

// poll checks the player every 100 ms until it loaded or failed, for at most a minute: a paused player reports no time to the periodic observer.
func (p *audioPlayer) poll(generation, attempt int) {
	if p.generation != generation || p.player == 0 {
		return
	}
	p.check()
	if p.loaded || p.failed {
		return
	}
	if attempt >= 600 {
		p.fail(p.failure(), "the audio did not load within a minute")
		return
	}
	time.AfterFunc(100*time.Millisecond, func() { OnMain(func() { p.poll(generation, attempt+1) }) })
}

func (p *audioPlayer) duration() time.Duration {
	if p.item == 0 {
		return 0
	}
	return objc.Send[cmTime](p.item, sel("duration")).duration()
}

func (p *audioPlayer) now() time.Duration {
	return max(objc.Send[cmTime](p.player, sel("currentTime")).duration(), 0)
}

// check reports the load, once, applying the seek asked for before it and starting a play that waited for it; a failure, once; and buffering changes.
func (p *audioPlayer) check() {
	const ready, failed = 1, 2 // AVPlayerItemStatus
	item := p.item
	if p.loaded {
		if current := p.player.Send(sel("currentItem")); current != 0 {
			item = current
		}
	}
	switch objc.Send[int](item, sel("status")) {
	case failed:
		p.fail(p.failure(), describeError(item.Send(sel("error")), "the audio failed to load"))
		return
	case ready:
		if p.loaded {
			break
		}
		p.loaded = true
		d := p.duration()
		p.prepareNativeLoop()
		if p.position > 0 {
			if d > 0 {
				p.position = min(p.position, d)
			}
			p.seek(p.position)
		}
		p.report(driver.AudioEvent{Phase: driver.AudioOpened, Duration: d})
		if p.wantsPlay {
			p.start()
		}
	}
	if p.looper != 0 && objc.Send[int](p.looper, sel("status")) == failed {
		p.fail(p.failure(), describeError(p.looper.Send(sel("error")), "the audio failed to loop"))
		return
	}
	const waitingToPlay = 1 // AVPlayerTimeControlStatusWaitingToPlayAtSpecifiedRate
	waiting := p.playing && objc.Send[int](p.player, sel("timeControlStatus")) == waitingToPlay
	if p.buffering != waiting {
		p.buffering = waiting
		p.report(driver.AudioEvent{Phase: driver.AudioBuffer, Buffering: waiting})
	}
}

// seek moves the player to position exactly, in nanoseconds rather than cmTimeOf's 600ths of a second, which could land a load after a failure before the position the core has.
func (p *audioPlayer) seek(position time.Duration) {
	if p.looper != 0 && p.player.Send(sel("currentItem")) != p.intro && position < driver.AudioLoopStart(p.loopStart, p.duration()) {
		p.resetLoopQueue(position)
		return
	}
	p.seekTime(position, false)
}

func (p *audioPlayer) seekTime(position time.Duration, resume bool) {
	p.seekSerial++
	serial, player := p.seekSerial, p.player
	p.seeking = true
	done := objcblock.New(func(_ objc.Block, _ bool) {
		OnMain(func() {
			if p.player == player && p.seekSerial == serial {
				p.seeking = false
				if resume && p.playing && !p.failed {
					player.Send(sel("setRate:"), p.rate)
				}
			}
		})
	})
	zero := cmTimeOf(0)
	player.Send(sel("seekToTime:toleranceBefore:toleranceAfter:completionHandler:"), exactAudioTime(position), zero, zero, done)
	done.Release()
}

// start plays the loaded player from where it is, from the start after it ended, holding the session while it plays.
func (p *audioPlayer) start() {
	p.wantsPlay = false
	if p.ended {
		p.ended = false
		p.seek(0)
	}
	p.hold()
	p.player.Send(sel("setRate:"), p.rate)
	p.playing = true
	p.report(driver.AudioEvent{Phase: driver.AudioPlaying})
}

// pause pauses the player, reporting where it stopped.
func (p *audioPlayer) pause() {
	p.wantsPlay = false
	if p.player != 0 {
		p.player.Send(sel("pause"))
	}
	if p.playing {
		p.paused()
	}
}

// paused reports that the player stopped playing, the app's pause or the system's.
func (p *audioPlayer) paused() {
	p.playing = false
	p.letGo()
	e := p.progress(p.now())
	e.Phase = driver.AudioPaused
	p.report(e)
}

// tick reports where the playing player is, or that the system paused it: an interruption, a lost route, an output gone.
func (p *audioPlayer) tick() {
	p.check()
	if !p.playing || p.failed || p.seeking {
		return
	}
	if objc.Send[float32](p.player, sel("rate")) == 0 {
		p.stopped()
		return
	}
	p.report(p.progress(p.now()))
}

// stopped reports that the playing player stopped with no command asking, a system pause, unless it is at its end, which its notification reports.
func (p *audioPlayer) stopped() {
	const waitingToPlay = 1 // AVPlayerTimeControlStatusWaitingToPlayAtSpecifiedRate
	if p.seeking || objc.Send[int](p.player, sel("timeControlStatus")) == waitingToPlay {
		return
	}
	at, d := p.now(), p.duration()
	if d > 0 && at >= d-50*time.Millisecond {
		// Should no notification come, it paused just before its end.
		generation := p.generation
		time.AfterFunc(500*time.Millisecond, func() {
			OnMain(func() {
				if p.generation == generation && p.playing && !p.ended && objc.Send[float32](p.player, sel("rate")) == 0 {
					p.paused()
				}
			})
		})
		return
	}
	p.paused()
}

// reachedEnd reports the end, with a seek for streams or systems where the native queue could not be prepared.
func (p *audioPlayer) reachedEnd() {
	if !p.playing || p.looper != 0 || p.seeking {
		return
	}
	if p.loop {
		start := driver.AudioLoopStart(p.loopStart, p.duration())
		p.laps++
		p.seek(start)
		p.player.Send(sel("setRate:"), p.rate)
		p.report(p.progress(start))
		return
	}
	p.playing, p.ended = false, true
	p.letGo()
	p.report(driver.AudioEvent{Phase: driver.AudioEnded})
}

// hold has the player hold the session at its level while it plays.
func (p *audioPlayer) hold() {
	if p.release != nil {
		return
	}
	level := audiosession.Ambient
	switch p.session {
	case driver.AudioSessionPlayback:
		level = audiosession.Playback
	case driver.AudioSessionMixWithOthers:
		level = audiosession.Mixing
	}
	release, err := audiosession.Hold(level)
	if err != nil {
		log.Print("nsgo: cannot set and activate the audio session for a player")
		return
	}
	p.release = release
}

func (p *audioPlayer) letGo() {
	if p.release != nil {
		p.release()
		p.release = nil
	}
}

// close stops the player and stops observing it, removing the file it played; it may be made again.
func (p *audioPlayer) close() {
	p.generation++
	p.seekSerial++
	p.opening, p.playing, p.wantsPlay, p.buffering, p.seeking = false, false, false, false, false
	p.letGo()
	p.closeQueue()
	if p.item != 0 {
		p.item.Send(sel("release"))
		p.item = 0
	}
	if p.file != "" {
		os.Remove(p.file)
		p.file = ""
	}
}

// closeQueue releases playback and notifications while retaining the asset item and file for a rebuild, seek or final close.
func (p *audioPlayer) closeQueue() {
	player := p.player
	p.player = 0
	if player != 0 && p.timeToken != 0 {
		player.Send(sel("removeTimeObserver:"), p.timeToken)
		p.timeToken.Send(sel("release"))
	}
	p.timeToken = 0
	for _, stop := range p.stops {
		stop()
	}
	p.stops = nil
	if player != 0 {
		player.Send(sel("pause"))
		p.releaseNativeLoop()
		player.Send(sel("release"))
	}
	if p.intro != 0 {
		p.intro.Send(sel("release"))
		p.intro = 0
	}
}

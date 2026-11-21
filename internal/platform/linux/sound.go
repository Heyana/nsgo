//go:build linux && !android

package linux

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"math"
	"slices"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	soundRate       = 48000
	soundFrameBytes = 2 * 4
	soundCaps       = "audio/x-raw,format=F32LE,layout=interleaved,channels=2,rate=48000"
	gstStateNull    = 1
	gstStatePlaying = 4
	gstMessageError = 1 << 1
	gstClockLost    = 1 << 10
	gstLatency      = 1 << 19
)

var (
	gstInitCheck             func(uintptr, uintptr, *uintptr) int32
	gstParseLaunch           func(*byte, *uintptr) uintptr
	gstParseBin              func(*byte, int32, *uintptr) uintptr
	gstObjectUnref           func(uintptr)
	gstMiniObjectUnref       func(uintptr)
	gstElementFactoryFind    func(*byte) uintptr
	gstElementSetState       func(uintptr, int32) int32
	gstElementGetState       func(uintptr, *int32, *int32, uint64) int32
	gstElementRunningTime    func(uintptr) uint64
	gstElementSyncState      func(uintptr) int32
	gstElementGetBus         func(uintptr) uintptr
	gstElementGetStaticPad   func(uintptr, *byte) uintptr
	gstElementGetRequestPad  func(uintptr, *byte) uintptr
	gstElementReleasePad     func(uintptr, uintptr)
	gstPadLink               func(uintptr, uintptr) int32
	gstPadUnlink             func(uintptr, uintptr) int32
	gstPadSetOffset          func(uintptr, int64)
	gstBinGetByName          func(uintptr, *byte) uintptr
	gstBinAdd                func(uintptr, uintptr) int32
	gstBinRemove             func(uintptr, uintptr) int32
	gstBinRecalculateLatency func(uintptr) int32
	gstBusPopFiltered        func(uintptr, uint32) uintptr
	gstMessageParseError     func(uintptr, *uintptr, uintptr)
	gstBufferNewAllocate     func(uintptr, uintptr, uintptr) uintptr
	gstBufferFill            func(uintptr, uintptr, unsafe.Pointer, uintptr) uintptr
	gstBufferGetSize         func(uintptr) uintptr
	gstBufferExtract         func(uintptr, uintptr, unsafe.Pointer, uintptr) uintptr
	gstSampleGetBuffer       func(uintptr) uintptr
	gstAppSrcPushBuffer      func(uintptr, uintptr) int32
	gstAppSrcEndOfStream     func(uintptr) int32
	gstAppSinkTryPullSample  func(uintptr, uint64) uintptr
	gstAppSinkIsEOS          func(uintptr) int32
	gstQueryNewLatency       func() uintptr
	gstElementQuery          func(uintptr, uintptr) int32
	gstQueryParseLatency     func(uintptr, *int32, *uint64, *uint64)
)

// loadSound initializes GStreamer once, without starting any output.
var loadSound = sync.OnceValue(func() bool {
	gst, err := purego.Dlopen("libgstreamer-1.0.so.0", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return false
	}
	app, err := purego.Dlopen("libgstapp-1.0.so.0", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		purego.Dlclose(gst)
		return false
	}
	bind := func(fn any, name string) { purego.RegisterLibFunc(fn, gst, name) }
	bind(&gstInitCheck, "gst_init_check")
	bind(&gstParseLaunch, "gst_parse_launch")
	bind(&gstParseBin, "gst_parse_bin_from_description")
	bind(&gstObjectUnref, "gst_object_unref")
	bind(&gstMiniObjectUnref, "gst_mini_object_unref")
	bind(&gstElementFactoryFind, "gst_element_factory_find")
	bind(&gstElementSetState, "gst_element_set_state")
	bind(&gstElementGetState, "gst_element_get_state")
	bind(&gstElementRunningTime, "gst_element_get_current_running_time")
	bind(&gstElementSyncState, "gst_element_sync_state_with_parent")
	bind(&gstElementGetBus, "gst_element_get_bus")
	bind(&gstElementGetStaticPad, "gst_element_get_static_pad")
	bind(&gstElementGetRequestPad, "gst_element_get_request_pad")
	bind(&gstElementReleasePad, "gst_element_release_request_pad")
	bind(&gstPadLink, "gst_pad_link")
	bind(&gstPadUnlink, "gst_pad_unlink")
	bind(&gstPadSetOffset, "gst_pad_set_offset")
	bind(&gstBinGetByName, "gst_bin_get_by_name")
	bind(&gstBinAdd, "gst_bin_add")
	bind(&gstBinRemove, "gst_bin_remove")
	bind(&gstBinRecalculateLatency, "gst_bin_recalculate_latency")
	bind(&gstBusPopFiltered, "gst_bus_pop_filtered")
	bind(&gstMessageParseError, "gst_message_parse_error")
	bind(&gstBufferNewAllocate, "gst_buffer_new_allocate")
	bind(&gstBufferFill, "gst_buffer_fill")
	bind(&gstBufferGetSize, "gst_buffer_get_size")
	bind(&gstBufferExtract, "gst_buffer_extract")
	bind(&gstSampleGetBuffer, "gst_sample_get_buffer")
	bind(&gstQueryNewLatency, "gst_query_new_latency")
	bind(&gstElementQuery, "gst_element_query")
	bind(&gstQueryParseLatency, "gst_query_parse_latency")
	purego.RegisterLibFunc(&gstAppSrcPushBuffer, app, "gst_app_src_push_buffer")
	purego.RegisterLibFunc(&gstAppSrcEndOfStream, app, "gst_app_src_end_of_stream")
	purego.RegisterLibFunc(&gstAppSinkTryPullSample, app, "gst_app_sink_try_pull_sample")
	purego.RegisterLibFunc(&gstAppSinkIsEOS, app, "gst_app_sink_is_eos")
	var gerr uintptr
	ok := gstInitCheck(0, 0, &gerr) != 0
	if gerr != 0 {
		gErrorFree(gerr)
	}
	return ok
})

func soundError(gerr uintptr) string {
	if gerr == 0 {
		return ""
	}
	defer gErrorFree(gerr)
	return mediaError(goString((*gError)(cPointer(gerr)).message))
}

func soundPipeline(description string) (uintptr, string) {
	var gerr uintptr
	pipeline := gstParseLaunch(cstr(description), &gerr)
	if gerr != 0 {
		if pipeline != 0 {
			gstObjectUnref(pipeline)
		}
		return 0, soundError(gerr)
	}
	if pipeline == 0 {
		return 0, "GStreamer did not make the audio pipeline"
	}
	return pipeline, ""
}

func soundBusError(bus uintptr) string {
	message := gstBusPopFiltered(bus, gstMessageError)
	if message == 0 {
		return ""
	}
	defer gstMiniObjectUnref(message)
	return gstErrorText(message)
}

func gstErrorText(message uintptr) string {
	var gerr uintptr
	gstMessageParseError(message, &gerr, 0)
	return soundError(gerr)
}

// gstSoundBuffer is the timing prefix of GstBuffer. The public ABI has a
// GstMiniObject (64 bytes on NSGo's Linux targets) and a pool pointer first.
type gstSoundBuffer struct {
	_                  [72]byte
	pts, dts, duration uint64
}

type gstMessageHeader struct {
	_    [64]byte
	kind uint32
}

// pushSound copies b into a native buffer that appsrc takes ownership of.
// A duration of zero leaves encoded bytes unstamped for the decoder.
func pushSound(src uintptr, b []byte, start uint64, duration time.Duration) bool {
	return pushSoundBuffer(src, b, start, duration) && gstAppSrcEndOfStream(src) == 0
}

// pushSoundBuffer queues b on src, stamped from start for duration when duration is not 0, and leaves the stream open.
func pushSoundBuffer(src uintptr, b []byte, start uint64, duration time.Duration) bool {
	size := uintptr(len(b))
	buffer := gstBufferNewAllocate(0, size, 0)
	if buffer == 0 {
		return false
	}
	if duration > 0 {
		timing := (*gstSoundBuffer)(cPointer(buffer))
		timing.pts = start
		timing.duration = uint64(duration)
	}
	if gstBufferFill(buffer, 0, unsafe.Pointer(&b[0]), size) != size {
		gstMiniObjectUnref(buffer)
		return false
	}
	return gstAppSrcPushBuffer(src, buffer) == 0
}

// decodeSound uses a temporary pipeline on a worker, retaining at most limit of the decoded stereo PCM. The app's mixer sees only decoded bytes. A sound whose PCM within limit would exceed maxDecoded bytes reports large and keeps none.
func decodeSound(ctx context.Context, b []byte, limit time.Duration, maxDecoded int64) (pcm []byte, cut, large bool, message string) {
	if len(b) == 0 {
		return nil, false, false, "the source has no audio bytes"
	}
	desc := "appsrc name=encoded format=bytes ! decodebin ! audio/x-raw ! audioconvert ! audioresample ! " +
		soundCaps + " ! appsink name=decoded sync=false max-buffers=4"
	pipeline, message := soundPipeline(desc)
	if pipeline == 0 {
		return nil, false, false, message
	}
	defer gstObjectUnref(pipeline)
	defer gstElementSetState(pipeline, gstStateNull)
	src, sink := gstBinGetByName(pipeline, cstr("encoded")), gstBinGetByName(pipeline, cstr("decoded"))
	defer gstObjectUnref(src)
	defer gstObjectUnref(sink)
	bus := gstElementGetBus(pipeline)
	defer gstObjectUnref(bus)
	if !pushSound(src, b, 0, 0) || gstElementSetState(pipeline, gstStatePlaying) == 0 {
		return nil, false, false, "GStreamer could not start decoding the sound"
	}
	maxBytes := int(math.Ceil(limit.Seconds()*soundRate)) * soundFrameBytes
	deadline := time.Now().Add(10 * time.Second)
	for ctx.Err() == nil && time.Now().Before(deadline) {
		sample := gstAppSinkTryPullSample(sink, uint64(50*time.Millisecond))
		if sample != 0 {
			var size int
			if buffer := gstSampleGetBuffer(sample); buffer != 0 {
				size = int(gstBufferGetSize(buffer))
				n := min(size, maxBytes-len(pcm))
				if int64(len(pcm))+int64(n) > maxDecoded {
					gstMiniObjectUnref(sample)
					return nil, false, true, ""
				}
				if n > 0 {
					start := len(pcm)
					pcm = slices.Grow(pcm, n)[:start+n]
					gstBufferExtract(buffer, 0, unsafe.Pointer(&pcm[start]), uintptr(n))
				}
				cut = n < size
			}
			gstMiniObjectUnref(sample)
			if cut {
				return pcm, true, false, ""
			}
			continue
		}
		if message := soundBusError(bus); message != "" {
			return nil, false, false, message
		}
		if gstAppSinkIsEOS(sink) != 0 {
			if len(pcm) == 0 {
				return nil, false, false, "the source has no decodable audio"
			}
			return pcm, false, false, ""
		}
	}
	return nil, false, false, "the sound did not decode in time"
}

func soundDuration(pcm []byte) time.Duration {
	return time.Duration(len(pcm)/soundFrameBytes) * time.Second / soundRate
}

type soundVoice struct {
	play           soundPlayback
	bin, src, pad  uintptr
	started, until time.Time
	// A looping voice keeps its appsrc and pushes copies of play.pcm, counted in copies, from started on; it never ends on its own.
	appsrc    uintptr
	inputRate int
	copies    int64
}

type soundPlayback struct {
	id, playback      uint32
	pcm               []byte
	volume, rate, pan float32
	// startAt is the Go clock time the voice should start at; zero, or a time sooner than the mixer can reach, starts it at the next mixer block.
	startAt time.Time
	loop    bool
	// skip is how many frames of pcm the first copy of a looping playback leaves out, so that a rate change resumes the loop where it was.
	skip int
	// limit is the engine's playback limit, driver.MaxSoundVoices when 0.
	limit      int
	restricted bool
}

// soundLoopAhead is how much of a looping voice stays queued past now.
const soundLoopAhead = 500 * time.Millisecond

// copyEnd is where copy n of a looping voice's sound ends, counted in its input frames so that copies stay sample-contiguous. The first copy is short by play.skip frames.
func (v *soundVoice) copyEnd(n int64) time.Duration {
	frames := max(0, n*int64(len(v.play.pcm)/soundFrameBytes)-int64(v.play.skip))
	return time.Duration(frames * int64(time.Second) / int64(v.inputRate))
}

// fill queues copies of a looping voice's sound until soundLoopAhead past now is queued.
func (v *soundVoice) fill(now time.Time) bool {
	for v.copyEnd(v.copies) < now.Sub(v.started)+soundLoopAhead {
		start, pcm := v.copyEnd(v.copies), v.play.pcm
		if v.copies == 0 {
			pcm = pcm[v.play.skip*soundFrameBytes:]
		}
		if !pushSoundBuffer(v.appsrc, pcm, uint64(start), v.copyEnd(v.copies+1)-start) {
			return false
		}
		v.copies++
	}
	return true
}

// feed keeps every looping voice queued ahead of its playback.
func (m *soundMixer) feed(now time.Time) {
	for i := range m.voices {
		if m.voices[i].play.loop {
			m.voices[i].fill(now)
		}
	}
}

func (p soundPlayback) voices() int { return max(1, cmp.Or(p.limit, driver.MaxSoundVoices)) }

// soundMixer keeps one live pipeline per engine. A silent input leaves
// the mixer's clock running between plays and on versions without force-live.
type soundMixer struct {
	pipeline, mixer, bus uintptr
	voices               []soundVoice
	pending              []soundPlayback
	// knownLatency is the last live latency the pipeline answered.
	knownLatency time.Duration
	ended        func(uint32, uint32)
}

func newSoundMixer(ended func(uint32, uint32)) *soundMixer {
	var output string
	for _, name := range []string{"autoaudiosink", "pipewiresink", "pulsesink", "alsasink"} {
		if factory := gstElementFactoryFind(cstr(name)); factory != 0 {
			gstObjectUnref(factory)
			output = name
			break
		}
	}
	if output == "" {
		return nil
	}
	m := soundMixerTo(output)
	if m != nil {
		m.ended = ended
	}
	return m
}

func soundMixerTo(output string) *soundMixer {
	desc := "audiotestsrc is-live=true wave=silence samplesperbuffer=480 ! " + soundCaps +
		" ! audiomixer name=mixer ignore-inactive-pads=true ! audioconvert ! audioresample ! " + output
	pipeline, message := soundPipeline(desc)
	if pipeline == 0 {
		log.Print("nsgo: sound output: ", message)
		return nil
	}
	m := &soundMixer{pipeline: pipeline, mixer: gstBinGetByName(pipeline, cstr("mixer")), bus: gstElementGetBus(pipeline)}
	if gstElementSetState(pipeline, gstStatePlaying) == 0 {
		m.close()
		return nil
	}
	return m
}

func (m *soundMixer) play(play soundPlayback) {
	for len(m.pending) >= play.voices() {
		m.endPlayback(m.pending[0])
		m.pending[0] = soundPlayback{}
		m.pending = m.pending[1:]
	}
	m.pending = append(m.pending, play)
	m.startPending()
}

// playing reports whether the output pipeline reached PLAYING, which starts its clock and configures its latency.
func (m *soundMixer) playing() bool {
	var state int32
	gstElementGetState(m.pipeline, &state, nil, 0)
	return state == gstStatePlaying
}

// latency is the pipeline's live latency: how long after a voice's running time its first sample reaches the output device. Zero when GStreamer has not reported one. While a voice's bin is linked the pipeline answers the query as not live, so the mixer keeps the answer it read before its first voice started.
func (m *soundMixer) latency() time.Duration {
	if d := m.queryLatency(); d > 0 {
		m.knownLatency = d
	}
	return m.knownLatency
}

func (m *soundMixer) queryLatency() time.Duration {
	query := gstQueryNewLatency()
	if query == 0 {
		return 0
	}
	defer gstMiniObjectUnref(query)
	var live int32
	var least, most uint64
	if gstElementQuery(m.pipeline, query) == 0 {
		return 0
	}
	gstQueryParseLatency(query, &live, &least, &most)
	if live == 0 || least == math.MaxUint64 || least > math.MaxInt64 {
		return 0
	}
	return time.Duration(least)
}

func (m *soundMixer) startPending() {
	if len(m.pending) == 0 || !m.playing() {
		return
	}
	if m.knownLatency == 0 {
		m.latency()
	}
	for _, voice := range m.pending {
		m.startVoice(voice)
	}
	clear(m.pending)
	m.pending = m.pending[:0]
}

func (m *soundMixer) checkOutput() string {
	latencyChanged := false
	for {
		message := gstBusPopFiltered(m.bus, gstMessageError|gstClockLost|gstLatency)
		if message == 0 {
			break
		}
		kind := (*gstMessageHeader)(cPointer(message)).kind
		var failure string
		switch kind {
		case gstMessageError:
			failure = gstErrorText(message)
		case gstClockLost:
			failure = "the output lost its audio clock"
		case gstLatency:
			latencyChanged = true
		}
		gstMiniObjectUnref(message)
		if failure != "" {
			return failure
		}
	}
	if latencyChanged {
		gstBinRecalculateLatency(m.pipeline)
	}
	return ""
}

func (m *soundMixer) startVoice(play soundPlayback) {
	m.sweep(time.Now(), 0)
	if play.restricted {
		oldest, n := -1, 0
		for i, v := range m.voices {
			if v.play.restricted {
				if n == 0 {
					oldest = i
				}
				n++
			}
		}
		if n >= driver.MaxSoundVoices {
			m.finishVoice(m.voices[oldest])
			m.voices = slices.Delete(m.voices, oldest, oldest+1)
		}
	}
	// A limit lowered while voices play stops the oldest until the new playback fits.
	for len(m.voices) >= play.voices() {
		m.finishVoice(m.voices[0])
		m.voices = m.voices[1:]
	}
	voice, ok := m.prepareVoice(play)
	if !ok {
		m.endPlayback(play)
		return
	}
	m.voices = append(m.voices, voice)
}

func (m *soundMixer) prepareVoice(play soundPlayback) (soundVoice, bool) {
	// Changing the input caps' sample rate gives the same pitch shift as
	// Apple's varispeed, and GStreamer resamples it back to the mixer rate.
	inputRate := int(math.Round(float64(play.rate) * soundRate))
	input := fmt.Sprintf("audio/x-raw,format=F32LE,layout=interleaved,channels=2,rate=%d", inputRate)
	desc := "appsrc name=voice is-live=true format=time min-latency=0 max-latency=0 caps=\"" + input +
		"\" ! audioresample ! capsfilter caps=\"" + soundCaps + "\""
	if play.pan != 0 {
		// The simple method is a balance: it attenuates the side away from the pan and leaves the other untouched, as driver.AudioPanGains does. audiopanorama is in gst-plugins-good, so a centered voice does not need it.
		desc += fmt.Sprintf(" ! audiopanorama method=simple panorama=%g", play.pan)
	}
	var gerr uintptr
	bin := gstParseBin(cstr(desc), 1, &gerr)
	if gerr != 0 || bin == 0 {
		if bin != 0 {
			gstObjectUnref(bin)
		}
		log.Print("nsgo: sound voice: ", soundError(gerr))
		return soundVoice{}, false
	}
	if gstBinAdd(m.pipeline, bin) == 0 {
		gstObjectUnref(bin)
		return soundVoice{}, false
	}
	voice := soundVoice{play: play, bin: bin, src: gstElementGetStaticPad(bin, cstr("src")),
		pad: gstElementGetRequestPad(m.mixer, cstr("sink_%u"))}
	start := gstElementRunningTime(m.pipeline)
	now := time.Now()
	// The voice's segment begins at zero. Its pad shifts that segment ahead of the mixer's block already being aggregated, or to the running time that matches startAt on the Go clock; the pipeline clock and the Go clock advance together over a voice's delay.
	lead := 20 * time.Millisecond
	if delay := play.startAt.Sub(now); delay > lead {
		lead = delay
	}
	if voice.src == 0 || voice.pad == 0 || start == math.MaxUint64 {
		m.remove(voice)
		return soundVoice{}, false
	}
	setSoundVolume(voice.pad, play.volume)
	gstPadSetOffset(voice.src, int64(start)+int64(lead))
	if gstPadLink(voice.src, voice.pad) != 0 || gstElementSyncState(bin) == 0 {
		m.remove(voice)
		return soundVoice{}, false
	}
	src := gstBinGetByName(bin, cstr("voice"))
	if play.loop {
		voice.appsrc, voice.inputRate, voice.started = src, inputRate, now.Add(lead)
		if !voice.fill(voice.started) {
			m.remove(voice)
			return soundVoice{}, false
		}
		return voice, true
	}
	defer gstObjectUnref(src)
	duration := time.Duration(float64(soundDuration(play.pcm)) / float64(play.rate))
	if !pushSound(src, play.pcm, 0, duration) {
		m.remove(voice)
		return soundVoice{}, false
	}
	// The mixer's reported latency includes the device's queued output.
	latency := m.knownLatency
	if latency <= 0 {
		latency = 500 * time.Millisecond
	}
	voice.started = now.Add(lead)
	voice.until = voice.started.Add(duration + latency)
	return voice, true
}

func (m *soundMixer) sweep(now time.Time, stop uint32) {
	if stop != 0 {
		m.pending = slices.DeleteFunc(m.pending, func(play soundPlayback) bool {
			if play.id != stop {
				return false
			}
			m.endPlayback(play)
			return true
		})
	}
	m.voices = slices.DeleteFunc(m.voices, func(voice soundVoice) bool {
		if voice.play.id != stop && (voice.play.loop || voice.until.After(now)) {
			return false
		}
		m.finishVoice(voice)
		return true
	})
}

func (m *soundMixer) remove(voice soundVoice) {
	gstElementSetState(voice.bin, gstStateNull)
	if voice.src != 0 && voice.pad != 0 {
		gstPadUnlink(voice.src, voice.pad)
	}
	if voice.src != 0 {
		gstObjectUnref(voice.src)
	}
	if voice.pad != 0 {
		gstElementReleasePad(m.mixer, voice.pad)
		gstObjectUnref(voice.pad)
	}
	if voice.appsrc != 0 {
		gstObjectUnref(voice.appsrc)
	}
	gstBinRemove(m.pipeline, voice.bin)
}

func (m *soundMixer) close() {
	gstElementSetState(m.pipeline, gstStateNull)
	for _, voice := range m.voices {
		m.finishVoice(voice)
	}
	for _, play := range m.pending {
		m.endPlayback(play)
	}
	m.voices, m.pending = nil, nil
	gstObjectUnref(m.bus)
	gstObjectUnref(m.mixer)
	gstObjectUnref(m.pipeline)
}

//go:build linux && !android

package linux

import (
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
	return gstAppSrcPushBuffer(src, buffer) == 0 && gstAppSrcEndOfStream(src) == 0
}

// decodeSound uses a temporary pipeline on a worker, retaining at most
// limit of the decoded stereo PCM. The app's mixer sees only decoded bytes.
func decodeSound(ctx context.Context, b []byte, limit time.Duration) (pcm []byte, cut bool, message string) {
	if len(b) == 0 {
		return nil, false, "the source has no audio bytes"
	}
	desc := "appsrc name=encoded format=bytes ! decodebin ! audio/x-raw ! audioconvert ! audioresample ! " +
		soundCaps + " ! appsink name=decoded sync=false max-buffers=4"
	pipeline, message := soundPipeline(desc)
	if pipeline == 0 {
		return nil, false, message
	}
	defer gstObjectUnref(pipeline)
	defer gstElementSetState(pipeline, gstStateNull)
	src, sink := gstBinGetByName(pipeline, cstr("encoded")), gstBinGetByName(pipeline, cstr("decoded"))
	defer gstObjectUnref(src)
	defer gstObjectUnref(sink)
	bus := gstElementGetBus(pipeline)
	defer gstObjectUnref(bus)
	if !pushSound(src, b, 0, 0) || gstElementSetState(pipeline, gstStatePlaying) == 0 {
		return nil, false, "GStreamer could not start decoding the sound"
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
				if n > 0 {
					start := len(pcm)
					pcm = slices.Grow(pcm, n)[:start+n]
					gstBufferExtract(buffer, 0, unsafe.Pointer(&pcm[start]), uintptr(n))
				}
				cut = n < size
			}
			gstMiniObjectUnref(sample)
			if cut {
				return pcm, true, ""
			}
			continue
		}
		if message := soundBusError(bus); message != "" {
			return nil, false, message
		}
		if gstAppSinkIsEOS(sink) != 0 {
			if len(pcm) == 0 {
				return nil, false, "the source has no decodable audio"
			}
			return pcm, false, ""
		}
	}
	return nil, false, "the sound did not decode in time"
}

func soundDuration(pcm []byte) time.Duration {
	return time.Duration(len(pcm)/soundFrameBytes) * time.Second / soundRate
}

type soundVoice struct {
	sound         uint32
	bin, src, pad uintptr
	until         time.Time
}

type soundPlayback struct {
	id                uint32
	pcm               []byte
	volume, rate, pan float32
}

// soundMixer keeps one live pipeline per engine. A silent input leaves
// the mixer's clock running between plays and on versions without force-live.
type soundMixer struct {
	pipeline, mixer, bus uintptr
	voices               []soundVoice
	pending              []soundPlayback
}

func newSoundMixer() *soundMixer {
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
	return soundMixerTo(output)
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

func (m *soundMixer) play(id uint32, pcm []byte, volume, rate, pan float32) {
	if len(m.pending) == driver.MaxSoundVoices {
		m.pending[0] = soundPlayback{}
		m.pending = m.pending[1:]
	}
	m.pending = append(m.pending, soundPlayback{id, pcm, volume, rate, pan})
	m.startPending()
}

func (m *soundMixer) startPending() {
	if len(m.pending) == 0 {
		return
	}
	var state int32
	gstElementGetState(m.pipeline, &state, nil, 0)
	if state != gstStatePlaying {
		return
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
	if len(m.voices) == driver.MaxSoundVoices {
		m.remove(m.voices[0])
		m.voices = m.voices[1:]
	}

	// Changing the input caps' sample rate gives the same pitch shift as
	// Apple's varispeed, and GStreamer resamples it back to the mixer rate.
	input := fmt.Sprintf("audio/x-raw,format=F32LE,layout=interleaved,channels=2,rate=%d", int(math.Round(float64(play.rate)*soundRate)))
	desc := "appsrc name=voice is-live=true format=time min-latency=0 max-latency=0 caps=\"" + input +
		"\" ! audioresample ! " + soundCaps + fmt.Sprintf(" ! volume volume=%g", play.volume)
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
		return
	}
	if gstBinAdd(m.pipeline, bin) == 0 {
		gstObjectUnref(bin)
		return
	}
	voice := soundVoice{sound: play.id, bin: bin, src: gstElementGetStaticPad(bin, cstr("src")),
		pad: gstElementGetRequestPad(m.mixer, cstr("sink_%u"))}
	start := gstElementRunningTime(m.pipeline)
	if voice.src == 0 || voice.pad == 0 || start == math.MaxUint64 {
		m.remove(voice)
		return
	}
	// The voice's segment begins at zero. Its pad shifts that segment
	// ahead of the mixer's block already being aggregated.
	gstPadSetOffset(voice.src, int64(start)+int64(20*time.Millisecond))
	if gstPadLink(voice.src, voice.pad) != 0 || gstElementSyncState(bin) == 0 {
		m.remove(voice)
		return
	}
	src := gstBinGetByName(bin, cstr("voice"))
	defer gstObjectUnref(src)
	duration := time.Duration(float64(soundDuration(play.pcm)) / float64(play.rate))
	if !pushSound(src, play.pcm, 0, duration) {
		m.remove(voice)
		return
	}
	// Leave time for the mixer and the device's output buffer to drain.
	voice.until = time.Now().Add(duration + 500*time.Millisecond)
	m.voices = append(m.voices, voice)
}

func (m *soundMixer) sweep(now time.Time, stop uint32) {
	if stop != 0 {
		m.pending = slices.DeleteFunc(m.pending, func(play soundPlayback) bool { return play.id == stop })
	}
	m.voices = slices.DeleteFunc(m.voices, func(voice soundVoice) bool {
		if voice.sound != stop && voice.until.After(now) {
			return false
		}
		m.remove(voice)
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
	gstBinRemove(m.pipeline, voice.bin)
}

func (m *soundMixer) close() {
	gstElementSetState(m.pipeline, gstStateNull)
	for _, voice := range m.voices {
		m.remove(voice)
	}
	gstObjectUnref(m.bus)
	gstObjectUnref(m.mixer)
	gstObjectUnref(m.pipeline)
}

//go:build linux && !android

package linux

// A looping player's track over driver.MaxGaplessLoopBytes or driver.MaxGaplessLoopDuration streams through a decoding pipeline of its own instead of decoding whole: the pipeline decodes ahead into an appsink, which the track voice reads from on the audio tick, and at the end of the track a flushing seek sends it back to the loop start while the mixer still plays the half second the voice queued, so the output has no gap at a repeat.

import (
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

const (
	gstFormatTime      = 3
	gstSeekFlush       = 1 << 0
	gstSeekAccurate    = 1 << 1
	gstStateChangeFail = 0
	gstMessageAny      = ^uint32(0)
)

var (
	gstElementSeekSimple    func(uintptr, int32, int32, int64) int32
	gstElementQueryDuration func(uintptr, int32, *int64) int32
)

// trackFrames gives a track voice the frames of a player's track.
type trackFrames interface {
	// frames is up to n frames from frame from on, or none when none are ready yet; end says from is the end of the track, and !ok that the track cannot be read.
	frames(from, n int) (pcm []byte, end, ok bool)
	// length is the track's duration, or 0 while it is unknown.
	length() time.Duration
	close()
}

// pcmTrack is a track decoded whole.
type pcmTrack []byte

func (t pcmTrack) frames(from, n int) ([]byte, bool, bool) {
	total := len(t) / soundFrameBytes
	if from >= total {
		return nil, true, true
	}
	return t[from*soundFrameBytes : min(from+n, total)*soundFrameBytes], false, true
}

func (t pcmTrack) length() time.Duration { return soundDuration(t) }

func (pcmTrack) close() {}

// trackDecoder streams a track through a decoding pipeline, on its own streaming thread, into an appsink that holds what it decoded ahead.
type trackDecoder struct {
	pipeline, sink, bus uintptr
	duration            time.Duration
	// at is the frame pending starts at; trim says the next buffer follows a seek to at and may start before it.
	at      int
	pending []byte
	trim    bool
	failed  bool
}

// openTrackDecoder starts decoding uri, waiting on the calling worker until the pipeline prerolls.
func openTrackDecoder(uri string) (*trackDecoder, string) {
	desc := "uridecodebin name=source ! audioconvert ! audioresample ! " + soundCaps + " ! appsink name=decoded sync=false max-buffers=64"
	pipeline, message := soundPipeline(desc)
	if pipeline == 0 {
		return nil, message
	}
	d := &trackDecoder{pipeline: pipeline, bus: gstElementGetBus(pipeline), sink: gstBinGetByName(pipeline, cstr("decoded"))}
	source := gstBinGetByName(pipeline, cstr("source"))
	if source == 0 || d.sink == 0 {
		if source != 0 {
			gstObjectUnref(source)
		}
		d.close()
		return nil, "GStreamer did not make the track's decoder"
	}
	var value gValue
	gValueInit(uintptr(unsafe.Pointer(&value)), gTypeString)
	gValueSetString(&value, cstr(uri))
	gObjectSetProperty(source, cstr("uri"), &value)
	gValueUnset(uintptr(unsafe.Pointer(&value)))
	gstObjectUnref(source)
	var state int32
	if gstElementSetState(pipeline, gstStatePlaying) == gstStateChangeFail ||
		gstElementGetState(pipeline, &state, nil, uint64(10*time.Second)) == gstStateChangeFail || state != gstStatePlaying {
		message := soundBusError(d.bus)
		if message == "" {
			message = "GStreamer could not start decoding the track"
		}
		d.close()
		return nil, message
	}
	if message := soundBusError(d.bus); message != "" {
		d.close()
		return nil, message
	}
	var duration int64
	if gstElementQueryDuration(pipeline, gstFormatTime, &duration) != 0 && duration > 0 {
		d.duration = time.Duration(duration)
	}
	return d, ""
}

func (d *trackDecoder) frames(from, n int) ([]byte, bool, bool) {
	if d.failed || d.busFailed() {
		d.failed = true
		return nil, false, false
	}
	if from != d.at {
		d.pending = d.pending[:0]
		d.at, d.trim = from, true
		if gstElementSeekSimple(d.pipeline, gstFormatTime, gstSeekFlush|gstSeekAccurate, int64(trackTime(int64(from)))) == 0 {
			d.failed = true
			return nil, false, false
		}
	}
	want := n * soundFrameBytes
	for len(d.pending) < want {
		sample := gstAppSinkTryPullSample(d.sink, 0)
		if sample == 0 {
			if d.busFailed() {
				d.failed = true
				return nil, false, false
			}
			if len(d.pending) == 0 && gstAppSinkIsEOS(d.sink) != 0 {
				if d.duration == 0 {
					// The end of the track is its exact length, which GStreamer could not tell before it.
					d.duration = trackTime(int64(d.at))
				}
				return nil, true, true
			}
			break
		}
		d.take(sample)
	}
	k := min(want, len(d.pending)) / soundFrameBytes * soundFrameBytes
	if k == 0 {
		return nil, false, true
	}
	pcm := make([]byte, k)
	copy(pcm, d.pending)
	d.pending = d.pending[:copy(d.pending, d.pending[k:])]
	d.at += k / soundFrameBytes
	return pcm, false, true
}

// take appends a pulled sample's frames, less any before the frame a seek asked for.
func (d *trackDecoder) take(sample uintptr) {
	defer gstMiniObjectUnref(sample)
	buffer := gstSampleGetBuffer(sample)
	if buffer == 0 {
		return
	}
	size := int(gstBufferGetSize(buffer))
	skip := 0
	if d.trim {
		d.trim = false
		if pts := (*gstSoundBuffer)(cPointer(buffer)).pts; pts != ^uint64(0) {
			if first := int(int64(pts) * soundRate / int64(time.Second)); first < d.at {
				skip = min(size, (d.at-first)*soundFrameBytes)
			}
		}
	}
	if n := size - skip; n > 0 {
		start := len(d.pending)
		d.pending = append(d.pending, make([]byte, n)...)
		gstBufferExtract(buffer, uintptr(skip), unsafe.Pointer(&d.pending[start]), uintptr(n))
	}
}

// busFailed drains the decoder's bus and says whether it held an error. Nothing else reads the bus, and each seek back to the loop start posts state, latency and tag messages, which would pile up for as long as the loop plays.
func (d *trackDecoder) busFailed() bool {
	failed := false
	for {
		message := gstBusPopFiltered(d.bus, gstMessageAny)
		if message == 0 {
			return failed
		}
		if (*gstMessageHeader)(cPointer(message)).kind == gstMessageError {
			failed = true
		}
		gstMiniObjectUnref(message)
	}
}

// length is the duration GStreamer reported, asked again while it is unknown, as for a source without a length or a format that tells it only once decoded further; the end of the track sets it exactly.
func (d *trackDecoder) length() time.Duration {
	if d.duration == 0 && d.pipeline != 0 {
		var duration int64
		if gstElementQueryDuration(d.pipeline, gstFormatTime, &duration) != 0 && duration > 0 {
			d.duration = time.Duration(duration)
		}
	}
	return d.duration
}

func (d *trackDecoder) close() {
	if d.pipeline == 0 {
		return
	}
	gstElementSetState(d.pipeline, gstStateNull)
	for _, o := range []uintptr{d.sink, d.bus, d.pipeline} {
		if o != 0 {
			gstObjectUnref(o)
		}
	}
	d.pipeline, d.sink, d.bus = 0, 0, 0
}

// trackURI is the URI GStreamer reads a player's source from, as videoFile opens it for GtkMediaFile: an asset under the resource root, a URI as it is, or a file path.
func trackURI(source string) string {
	if path, ok := strings.CutPrefix(source, "asset:"); ok {
		source = filepath.Join(imageload.ResourceRoot, filepath.FromSlash(strings.TrimLeft(path, "/")))
	} else if strings.HasPrefix(source, "file:") || strings.Contains(source, "://") {
		return source
	}
	if abs, err := filepath.Abs(source); err == nil {
		source = abs
	}
	return (&url.URL{Scheme: "file", Path: source}).String()
}

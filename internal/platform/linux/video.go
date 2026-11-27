//go:build linux && !android

package linux

import (
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
	"github.com/nsgo-dev/nsgo/internal/platform/videoalpha"
	"github.com/nsgo-dev/nsgo/internal/platform/videoplay"
)

// video is a video view's player: a GtkMediaStream shown by a GtkPicture, or by a GtkVideo with GTK's playback controls when Controls is set, hosted as web views are. The stream is the backend's own NsgoMediaStream over GstPlay when GstPlay and scaletempo load, which applies Rate at the current position for seekable sources; otherwise it is GTK's GtkMediaFile, which plays at 1× (warned once) and reports RateChangeable false. A looping stream starts over without ending, so EventEnd comes only for videos that do not repeat. A stream GStreamer cannot play reports EventError, with a hint when a plug-in is missing, and never EventLoad. A file that declares alpha keeps it, as GTK composites the picture over what is behind it, where GStreamer decodes it (VP8 and VP9 with alpha, with the codecalpha plug-in), and reports EventError where it cannot rather than show the picture opaque.
type video struct {
	hosted
	stream uintptr // GtkMediaStream, 0 without a source
	player *gstVideo
	config driver.VideoConfig
	made   bool
	// What the stream reported so far.
	loaded, failed, buffering bool
	// playing was asked for, applied again once the stream is prepared.
	playing  bool
	playback videoplay.State
	// timer reports EventProgress; timerData finds the view.
	timer     uint32
	timerData uintptr
	controls  bool // the widget is a GtkVideo
}

// rateWarned is the warning, once, that Rate does not apply.
var rateWarned sync.Once

// configureVideo applies a video's configuration, opening its source
// again when that changed.
func (w *window) configureVideo(v *view, c *driver.VideoConfig) {
	if v.video == nil {
		v.video = &video{}
	}
	vd := v.video
	old, first := vd.config, !vd.made
	vd.config, vd.made = *c, true
	reopened := first || c.Source != old.Source || c.Headers != old.Headers
	if reopened {
		w.openVideo(v)
	}
	if reopened || c.Controls != old.Controls {
		w.showVideo(v)
	}
	if vd.player == nil && c.Rate != 1 && vd.stream != 0 {
		rateWarned.Do(func() { log.Print("nsgo: Video Rate is not supported on Linux: it plays at 1×") })
	}
	if s := vd.stream; s != 0 {
		gtkMediaStreamSetMuted(s, boolInt(c.Muted))
		gtkMediaStreamSetVolume(s, float64(min(max(c.Volume, 0), 1)))
		gtkMediaStreamSetLoop(s, boolInt(c.Repeat))
		if vd.playback.Configure(reopened || c.Paused != old.Paused) {
			vd.playing = !c.Paused
			if vd.playing && vd.playback.Completed {
				vd.playback.Completed = false
				vd.applyRate()
				gtkMediaStreamSeek(s, 0)
			}
			vd.playback.Applied(vd.playing && !vd.failed)
			gtkMediaStreamSetPlaying(s, boolInt(vd.playing && !vd.failed))
			w.videoBuffer(v, vd.playing && !vd.loaded && !vd.failed)
		}
		vd.applyRate()
	}
	if vd.child != 0 {
		if vd.controls {
			gtkVideoSetLoop(vd.child, boolInt(c.Repeat))
		} else {
			gtkPictureSetContentFit(vd.child, contentFit(c.ResizeMode))
		}
	}
	if reopened || c.ProgressInterval != old.ProgressInterval {
		w.observeVideo(v)
	}
}

func (vd *video) applyRate() {
	p := vd.player
	if p == nil || vd.playback.Completed || p.rate == vd.config.Rate || gtkMediaStreamIsSeekable(vd.stream) == 0 {
		return
	}
	p.rate = vd.config.Rate
	videoGST.rate(p.play, float64(p.rate))
}

// contentFit is GTK's fit for a resize mode.
func contentFit(m driver.ResizeMode) int32 {
	switch m {
	case driver.Cover:
		return gtkContentFitCover
	case driver.Stretch:
		return gtkContentFitFill
	// GtkPicture has no natural-size fit: Center shrinks too.
	case driver.Center, driver.ScaleDown:
		return gtkContentFitScaleDown
	}
	return gtkContentFitContain
}

// videoFile resolves a media source to a GFile: "asset:" paths in the app's
// resources, URIs and file paths.
func videoFile(source string) uintptr {
	if path, ok := strings.CutPrefix(source, "asset:"); ok {
		path = filepath.Join(imageload.ResourceRoot, filepath.FromSlash(strings.TrimLeft(path, "/")))
		return gFileNewForPath(cstr(path))
	}
	if strings.HasPrefix(source, "file:") || strings.Contains(source, "://") {
		return gFileNewForURI(cstr(source))
	}
	return gFileNewForPath(cstr(source))
}

// openVideo opens v's source, or nothing.
func (w *window) openVideo(v *view) {
	vd := v.video
	w.closeStream(v)
	vd.loaded, vd.failed, vd.buffering, vd.playing = false, false, false, false
	vd.playback = videoplay.State{}
	source := vd.config.Source
	if source == "" {
		return
	}
	if msg := alphaRefused(videoalpha.Source(source, imageload.ResourceRoot)); msg != "" {
		vd.failed = true
		log.Printf("nsgo: video %s: %s", source, msg)
		w.report(v, driver.Event{Kind: driver.EventError, ID: v.id, Text: source + ": " + msg, Index: driver.VideoErrorAlphaUnsupported})
		return
	}
	w.report(v, driver.Event{Kind: driver.EventLoadStart, ID: v.id})
	file := videoFile(source)
	defer gObjectUnref(file)
	var headers *driver.VideoHeaders
	if videoHTTPSource(source) {
		headers = vd.config.Headers
	}
	vd.player = openGSTVideo(w, v, file, headers)
	if vd.player != nil {
		vd.stream = vd.player.stream
	} else if headers.Len() > 0 {
		vd.failed = true
		w.report(v, driver.Event{Kind: driver.EventError, ID: v.id, Text: "Video.Headers requires GstPlay and the GStreamer scaletempo filter on Linux."})
		return
	} else {
		vd.stream = gtkMediaFileNewForFile(file)
	}
	w.connect(v, vd.stream, "notify::prepared", cbVideoPrepared)
	// The picture's size is known with the first frame, which may come
	// after it is prepared.
	w.connect(v, vd.stream, "invalidate-size", cbVideoSize)
	w.connect(v, vd.stream, "notify::playing", cbVideoPlaying)
	w.connect(v, vd.stream, "notify::ended", cbVideoEnded)
	w.connect(v, vd.stream, "notify::error", cbVideoError)
}

// closeStream stops and drops v's stream.
func (w *window) closeStream(v *view) {
	vd := v.video
	if vd.stream == 0 {
		return
	}
	h := &vd.hosted
	kept := h.signals[:0]
	for _, s := range h.signals {
		if s.instance == vd.stream {
			gSignalHandlerDisconnect(s.instance, s.id)
		} else {
			kept = append(kept, s)
		}
	}
	h.signals = kept
	delete(w.natives, vd.stream)
	gtkMediaStreamSetPlaying(vd.stream, 0)
	if vd.player != nil {
		vd.player.close()
		vd.player = nil
	} else {
		gtkMediaFileClear(vd.stream)
	}
	if vd.child != 0 {
		w.setStreamOf(vd.child, vd.controls, 0)
	}
	gObjectUnref(vd.stream)
	vd.stream = 0
}

// showVideo shows the stream with GTK's controls or without.
func (w *window) showVideo(v *view) {
	vd := v.video
	controls := vd.config.Controls
	var child uintptr
	if controls {
		child = gtkVideoNew()
		bindLook()
		addLookCSS()
		gtkWidgetAddCSSClass(child, cstr("nsgo-video"))
		// The app drives playback (Paused), not the widget.
		gtkVideoSetAutoplay(child, 0)
	} else {
		child = gtkPictureNew()
		gtkPictureSetCanShrink(child, 1)
	}
	w.setStreamOf(child, controls, vd.stream)
	w.host(v, &vd.hosted, child, controls)
	vd.controls = controls
}

func (w *window) setStreamOf(child uintptr, controls bool, stream uintptr) {
	if controls {
		gtkVideoSetMediaStream(child, stream)
	} else {
		gtkPictureSetPaintable(child, stream)
	}
}

// observeVideo reports EventProgress every ProgressInterval while the
// video plays.
func (w *window) observeVideo(v *view) {
	vd := v.video
	w.stopProgress(vd)
	if vd.stream == 0 {
		return
	}
	vd.timerData = registerVideo(w, v)
	vd.timer = gTimeoutAdd(uint32(max(vd.config.ProgressInterval, 16*time.Millisecond).Milliseconds()),
		cbVideoProgress, vd.timerData)
}

func (w *window) stopProgress(vd *video) {
	if vd.timer != 0 {
		gSourceRemove(vd.timer)
		vd.timer = 0
	}
	if vd.timerData != 0 {
		videoTimers.Delete(vd.timerData)
		vd.timerData = 0
	}
}

// seek moves the video to t if its stream can seek.
func (vd *video) seek(t time.Duration) {
	if vd.stream != 0 && gtkMediaStreamIsSeekable(vd.stream) != 0 {
		t = max(t, 0)
		replay := vd.playback.Completed && t < micros(gtkMediaStreamGetDuration(vd.stream))
		if replay {
			vd.playback.Completed, vd.playing = false, !vd.config.Paused
			vd.applyRate()
		}
		gtkMediaStreamSeek(vd.stream, t.Microseconds())
		if replay {
			vd.playback.Applied(vd.playing)
			gtkMediaStreamSetPlaying(vd.stream, boolInt(vd.playing))
		}
	}
}

// closeVideo stops v's video and lets its widget go.
func (w *window) closeVideo(v *view) {
	vd := v.video
	w.stopProgress(vd)
	w.closeStream(v)
	w.unhost(v, &vd.hosted)
	v.video = nil
}

// videoBuffer reports whether the video waits for data when that changed.
func (w *window) videoBuffer(v *view, on bool) {
	if v.video.buffering == on {
		return
	}
	v.video.buffering = on
	w.report(v, driver.Event{Kind: driver.EventBuffer, ID: v.id, Buffering: on})
}

// videoLoaded reports the load once the stream is prepared and its size
// is known.
func (w *window) videoLoaded(v *view) {
	vd := v.video
	s := vd.stream
	// A stream that failed (GStreamer prepares one with no streams when it
	// has no plug-in for them) never loads.
	if vd.loaded || vd.failed || gtkMediaStreamIsPrepared(s) == 0 {
		return
	}
	hasVideo := gtkMediaStreamHasVideo(s) != 0
	if !hasVideo && gtkMediaStreamHasAudio(s) == 0 {
		return
	}
	width, height := gdkPaintableGetIntrinsicWidth(s), gdkPaintableGetIntrinsicHeight(s)
	if hasVideo && width <= 0 {
		return
	}
	vd.loaded = true
	vd.applyRate()
	// Playing asked for before the pipeline was ready.
	if vd.playing && gtkMediaStreamGetPlaying(s) == 0 {
		gtkMediaStreamSetPlaying(s, 1)
	}
	w.videoBuffer(v, false)
	w.report(v, driver.Event{Kind: driver.EventLoad, ID: v.id, Duration: micros(gtkMediaStreamGetDuration(s)),
		Size:           driver.Size{Width: driver.Points(width), Height: driver.Points(height)},
		RateChangeable: vd.player != nil && gtkMediaStreamIsSeekable(s) != 0})
}

// micros turns GTK's microseconds into a duration.
func micros(us int64) time.Duration { return time.Duration(us) * time.Microsecond }

// videoOf returns the live video stream p belongs to.
func videoOf(data, p uintptr) (*window, *view) {
	w, v := nativeOf(data, p)
	if v == nil || v.video == nil || v.video.stream != p {
		return nil, nil
	}
	return w, v
}

func videoPreparedCallback(stream, pspec uintptr, data uintptr) {
	if w, v := videoOf(data, stream); v != nil {
		w.videoLoaded(v)
	}
}

func videoPlayingCallback(stream, pspec, data uintptr) {
	w, v := videoOf(data, stream)
	if v == nil || v.video.failed {
		return
	}
	vd := v.video
	if gtkMediaStreamGetEnded(stream) != 0 {
		vd.playback.Ended(false, micros(gtkMediaStreamGetTimestamp(stream)))
		vd.playing = false
		return
	}
	vd.playing = gtkMediaStreamGetPlaying(stream) != 0
	if ev, ok := vd.playback.Changed(vd.playing, vd.config.Paused, driver.VideoPauseControls); ok {
		ev.ID = v.id
		w.report(v, ev)
	}
}

func videoEndedCallback(stream, pspec uintptr, data uintptr) {
	w, v := videoOf(data, stream)
	if v == nil {
		return
	}
	if gtkMediaStreamGetEnded(stream) == 0 {
		// The stream left its end: a seek, or GtkVideo's controls playing it again, which a later configuration must not stop.
		v.video.playback.Completed = false
		return
	}
	v.video.playback.Ended(false, micros(gtkMediaStreamGetTimestamp(stream)))
	v.video.playing = false
	w.report(v, driver.Event{Kind: driver.EventEnd, ID: v.id})
}

func videoErrorCallback(stream, pspec uintptr, data uintptr) {
	w, v := videoOf(data, stream)
	if v == nil {
		return
	}
	vd := v.video
	e := gtkMediaStreamGetError(stream)
	if e == 0 || vd.failed {
		return
	}
	vd.failed = true
	// Nothing plays: no progress, and the app hears it stopped.
	gtkMediaStreamSetPlaying(stream, 0)
	msg := mediaError(goString((*gError)(cPointer(e)).message))
	log.Printf("nsgo: video %s: %s", vd.config.Source, msg)
	w.videoBuffer(v, false)
	w.report(v, driver.Event{Kind: driver.EventError, ID: v.id, Text: msg})
}

func videoSizeCallback(stream uintptr, data uintptr) {
	if w, v := videoOf(data, stream); v != nil {
		w.videoLoaded(v)
	}
}

// videoTimers find a progress timer's view by its data.
var (
	videoTimers sync.Map // uintptr -> videoTimer
	nextTimer   uintptr
)

type videoTimer struct {
	w *window
	v *view
}

func registerVideo(w *window, v *view) uintptr {
	nextTimer++
	videoTimers.Store(nextTimer, videoTimer{w, v})
	return nextTimer
}

func videoProgressCallback(data uintptr) int32 {
	t, ok := videoTimers.Load(data)
	if !ok {
		return 0 // G_SOURCE_REMOVE
	}
	w, v := t.(videoTimer).w, t.(videoTimer).v
	if w.closing || v.deleted || v.video == nil {
		return 0
	}
	if s := v.video.stream; s != 0 && gtkMediaStreamGetPlaying(s) != 0 && gtkMediaStreamIsPrepared(s) != 0 {
		position := micros(gtkMediaStreamGetTimestamp(s))
		if p := v.video.player; p != nil {
			if nanos := videoGST.position(p.play); nanos != ^uint64(0) {
				position = time.Duration(nanos)
			}
		}
		w.report(v, driver.Event{Kind: driver.EventProgress, ID: v.id,
			Time: position, Duration: micros(gtkMediaStreamGetDuration(s))})
	}
	return 1 // G_SOURCE_CONTINUE
}

// alphaRefused says why a video whose file declares alpha c cannot keep it, or "" when it can or declares none: GStreamer's codecalpha plug-in (gst-plugins-bad) decodes VP8 and VP9 with alpha, and nothing decodes HEVC's alpha layer. Played without it, the picture would show opaque.
func alphaRefused(c videoalpha.Codec) string {
	var decoder string
	switch c {
	case videoalpha.None:
		return ""
	case videoalpha.VP8:
		decoder = "vp8alphadecodebin"
	case videoalpha.VP9:
		decoder = "vp9alphadecodebin"
	default:
		return "GStreamer cannot decode the alpha channel of " + c.String() + "; use VP9 with alpha in WebM on Linux."
	}
	if gstFeature(decoder) {
		return ""
	}
	return c.String() + " needs GStreamer's codecalpha plug-in (" + decoder + ", in gst-plugins-bad) to keep its alpha channel."
}

// mediaError is what GStreamer's error says, for apps: its first line
// without the element path (GstPlay puts the debug details on further
// lines), and a hint when a plug-in is missing.
func mediaError(message string) string {
	line, _, _ := strings.Cut(message, "\n")
	line = strings.TrimSpace(line)
	if rest, ok := strings.CutPrefix(line, "Error from element "); ok {
		line = rest
		if _, m, ok := strings.Cut(rest, ": "); ok {
			line = m
		}
	}
	if strings.Contains(message, "missing a plug-in") || strings.Contains(message, "No suitable plugins") {
		return line + " No installed GStreamer plug-in can play this format" +
			" (H.264 / AAC need gst-libav or gst-plugins-ugly / -bad)."
	}
	return line
}

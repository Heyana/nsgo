//go:build linux && !android

package linux

import (
	"sync"
	"sync/atomic"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// videoGST binds GstPlay, which supplies the pipeline for Linux videos that can change rate. Its NsgoMediaStream keeps the same paintable and GTK control APIs as the fallback GtkMediaFile.
var videoGST struct {
	streamType                                                    uintptr
	classInit, paintableInit                                      uintptr
	playStream, pauseStream, seekStream, audioStream              uintptr
	snapshot, width, height                                       uintptr
	contentsChanged, sizeChanged, sampleReady, drawSample         uintptr
	progress, mediaInfo, state, seekDone, end, buffer, mediaError uintptr

	newPlay, pipeline, newAdapter       func(uintptr) uintptr
	play, pause, stop                   func(uintptr)
	uri                                 func(uintptr, *byte)
	rate, volume                        func(uintptr, float64)
	mute                                func(uintptr, int32)
	seek                                func(uintptr, uint64)
	position, mediaDuration             func(uintptr) uint64
	currentInfo                         func(uintptr) uintptr
	audioStreams, videoStreams          func(uintptr) uint32
	seekable                            func(uintptr) int32
	element                             func(*byte, *byte) uintptr
	caps                                func(*byte) uintptr
	setCaps                             func(uintptr, uintptr)
	miniUnref                           func(uintptr)
	pullSample, pullPreroll             func(uintptr, uint64) uintptr
	sampleCaps, sampleBuffer, videoMeta func(uintptr) uintptr
	capsStructure                       func(uintptr, uint32) uintptr
	structureInt                        func(uintptr, *byte, *int32) int32
	extract                             func(uintptr, uintptr, uintptr, *uintptr, *uintptr)

	setObject                                                 func(uintptr, *byte, ...any)
	refSink                                                   func(uintptr) uintptr
	fileURI                                                   func(uintptr) uintptr
	takeBytes                                                 func(uintptr, uintptr) uintptr
	copyError                                                 func(uintptr) uintptr
	gtkPrepared                                               func(uintptr, int32, int32, int32, int64)
	gtkUpdate                                                 func(uintptr, int64)
	gtkEnded, gtkSeekDone, invalidateContents, invalidateSize func(uintptr)
	gtkSeekPending, gtkLoop                                   func(uintptr) int32
	gtkError                                                  func(uintptr, uintptr)
}

var bindVideoGST = sync.OnceValue(func() bool {
	if !gstFeature("scaletempo") {
		return false
	}
	libraries := []struct {
		name    string
		symbols map[string]any
	}{
		{"libgstplay-1.0.so.0", map[string]any{
			"gst_play_new": &videoGST.newPlay, "gst_play_get_pipeline": &videoGST.pipeline,
			"gst_play_signal_adapter_new": &videoGST.newAdapter,
			"gst_play_play":               &videoGST.play, "gst_play_pause": &videoGST.pause, "gst_play_stop": &videoGST.stop,
			"gst_play_set_uri": &videoGST.uri, "gst_play_set_rate": &videoGST.rate,
			"gst_play_set_mute": &videoGST.mute, "gst_play_set_volume": &videoGST.volume, "gst_play_seek": &videoGST.seek,
			"gst_play_get_position": &videoGST.position, "gst_play_get_media_info": &videoGST.currentInfo,
			"gst_play_media_info_get_duration":                &videoGST.mediaDuration,
			"gst_play_media_info_get_number_of_audio_streams": &videoGST.audioStreams,
			"gst_play_media_info_get_number_of_video_streams": &videoGST.videoStreams,
			"gst_play_media_info_is_seekable":                 &videoGST.seekable,
		}},
		{"libgstreamer-1.0.so.0", map[string]any{
			"gst_element_factory_make": &videoGST.element, "gst_caps_from_string": &videoGST.caps,
			"gst_mini_object_unref": &videoGST.miniUnref,
			"gst_sample_get_caps":   &videoGST.sampleCaps, "gst_sample_get_buffer": &videoGST.sampleBuffer,
			"gst_caps_get_structure": &videoGST.capsStructure, "gst_structure_get_int": &videoGST.structureInt,
			"gst_buffer_extract_dup": &videoGST.extract,
		}},
		{"libgstapp-1.0.so.0", map[string]any{
			"gst_app_sink_set_caps": &videoGST.setCaps, "gst_app_sink_try_pull_sample": &videoGST.pullSample,
			"gst_app_sink_try_pull_preroll": &videoGST.pullPreroll,
		}},
		{"libgstvideo-1.0.so.0", map[string]any{"gst_buffer_get_video_meta": &videoGST.videoMeta}},
	}
	for _, dll := range libraries {
		h, err := purego.Dlopen(dll.name, purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			return false
		}
		if dll.name == "libgstreamer-1.0.so.0" {
			bindVideoHTTP(h)
		}
		for name, fn := range dll.symbols {
			symbol, err := purego.Dlsym(h, name)
			if err != nil {
				return false
			}
			purego.RegisterFunc(fn, symbol)
		}
	}
	var streamType, paintableType func() uintptr
	for name, fn := range map[string]any{
		"gtk_media_stream_get_type": &streamType, "gdk_paintable_get_type": &paintableType,
		"gtk_media_stream_stream_prepared": &videoGST.gtkPrepared,
		"gtk_media_stream_update":          &videoGST.gtkUpdate, "gtk_media_stream_stream_ended": &videoGST.gtkEnded,
		"gtk_media_stream_is_seeking": &videoGST.gtkSeekPending, "gtk_media_stream_seek_success": &videoGST.gtkSeekDone,
		"gtk_media_stream_gerror": &videoGST.gtkError, "gtk_media_stream_get_loop": &videoGST.gtkLoop,
		"gdk_paintable_invalidate_contents": &videoGST.invalidateContents, "gdk_paintable_invalidate_size": &videoGST.invalidateSize,
	} {
		purego.RegisterLibFunc(fn, gtkLib, name)
	}
	purego.RegisterLibFunc(&videoGST.setObject, gobjectLib, "g_object_set")
	purego.RegisterLibFunc(&videoGST.refSink, gobjectLib, "g_object_ref_sink")
	purego.RegisterLibFunc(&videoGST.copyError, glibLib, "g_error_copy")
	purego.RegisterLibFunc(&videoGST.takeBytes, glibLib, "g_bytes_new_take")
	purego.RegisterLibFunc(&videoGST.fileURI, gioLib, "g_file_get_uri")

	videoGST.classInit = newCallback(videoClassInit)
	videoGST.paintableInit = newCallback(videoPaintableInit)
	videoGST.playStream = newCallback(videoStreamPlay)
	videoGST.pauseStream = newCallback(videoStreamPause)
	videoGST.seekStream = newCallback(videoStreamSeek)
	videoGST.audioStream = newCallback(videoStreamAudio)
	videoGST.snapshot = newCallback(videoSnapshot)
	videoGST.width = newCallback(videoWidth)
	videoGST.height = newCallback(videoHeight)
	videoGST.contentsChanged = newCallback(videoContentsChanged)
	videoGST.sizeChanged = newCallback(videoSizeChanged)
	videoGST.sampleReady = newCallback(videoSampleReady)
	videoGST.drawSample = newCallback(videoDrawSample)
	videoGST.progress = newCallback(videoTimeChanged)
	videoGST.mediaInfo = newCallback(videoMediaInfo)
	videoGST.state = newCallback(videoStateChanged)
	videoGST.seekDone = newCallback(videoSeekDone)
	videoGST.end = newCallback(videoStreamEnd)
	videoGST.buffer = newCallback(videoStreamBuffer)
	videoGST.mediaError = newCallback(videoStreamError)

	base := streamType()
	var layout gTypeQuery
	gTypeQueryFn(base, &layout)
	// GtkMediaStreamClass has six callbacks and eight reserved slots following GObjectClass.
	if layout.classSize != objectClassSize+14*8 {
		return false
	}
	videoGST.streamType = gTypeRegisterStaticSimple(base, cstr("NsgoMediaStream"), layout.classSize, videoGST.classInit, layout.instanceSize, 0, 0)
	if videoGST.streamType == 0 {
		return false
	}
	gTypeAddInterfaceStatic(videoGST.streamType, paintableType(), &gInterfaceInfo{init: videoGST.paintableInit})
	return true
})

func videoClassInit(class, _ uintptr) {
	setSlot(class, objectClassSize, videoGST.playStream)
	setSlot(class, objectClassSize+8, videoGST.pauseStream)
	setSlot(class, objectClassSize+16, videoGST.seekStream)
	setSlot(class, objectClassSize+24, videoGST.audioStream)
}

func videoPaintableInit(iface, _ uintptr) {
	// The paintable slots follow the two GTypes of GTypeInterface.
	setSlot(iface, 2*8, videoGST.snapshot)
	setSlot(iface, 5*8, videoGST.width)
	setSlot(iface, 6*8, videoGST.height)
}

// Players and their paintables are owned by GTK's main thread. The streaming threads can only queue a request to pull the latest frame.
var (
	gstVideos       = map[uintptr]*gstVideo{}
	gstVideoSamples sync.Map
	nextGSTVideo    uintptr
)

type gstVideo struct {
	w                                  *window
	v                                  *view
	stream, play, adapter, sink, image uintptr
	paintableSignals                   [2]uint64
	headerKey                          uintptr
	key                                uintptr
	rate                               float32
	appSink                            bool
	framePending                       atomic.Bool
}

func openGSTVideo(w *window, v *view, file uintptr, headers *driver.VideoHeaders) *gstVideo {
	if !bindVideoGST() {
		return nil
	}
	p := &gstVideo{w: w, v: v, rate: 1}
	p.play = videoGST.newPlay(0)
	pipeline := videoGST.pipeline(p.play)
	defer gObjectUnref(pipeline)
	filter := videoGST.element(cstr("scaletempo"), nil)
	if filter == 0 {
		gObjectUnref(p.play)
		return nil
	}
	videoGST.refSink(filter)
	videoGST.setObject(pipeline, cstr("audio-filter"), filter, uintptr(0))
	gObjectUnref(filter)

	if p.sink = videoGST.element(cstr("gtk4paintablesink"), nil); p.sink != 0 {
		gObjectGet(p.sink, cstr("paintable"), &p.image, uintptr(0))
		if p.image == 0 {
			gObjectUnref(p.sink)
			p.sink = 0
		}
	}
	if p.sink == 0 {
		p.sink = videoGST.element(cstr("appsink"), nil)
		p.appSink = true
	}
	if p.sink == 0 {
		gObjectUnref(p.play)
		return nil
	}
	videoGST.refSink(p.sink)
	videoGST.setObject(pipeline, cstr("video-sink"), p.sink, uintptr(0))
	p.stream = gObjectNew(videoGST.streamType, nil)
	gstVideos[p.stream] = p
	// GtkMediaStream and the play signal adapter both use the GTK main context. Register before setting the URI or prerolling.
	p.adapter = videoGST.newAdapter(p.play)
	gSignalConnect(p.adapter, "media-info-updated", videoGST.mediaInfo, p.stream)
	gSignalConnect(p.adapter, "state-changed", videoGST.state, p.stream)
	gSignalConnect(p.adapter, "position-updated", videoGST.progress, p.stream)
	gSignalConnect(p.adapter, "seek-done", videoGST.seekDone, p.stream)
	gSignalConnect(p.adapter, "end-of-stream", videoGST.end, p.stream)
	gSignalConnect(p.adapter, "buffering", videoGST.buffer, p.stream)
	gSignalConnect(p.adapter, "error", videoGST.mediaError, p.stream)
	if p.appSink {
		p.prepareAppSink()
	} else {
		p.paintableSignals = [2]uint64{
			gSignalConnect(p.image, "invalidate-contents", videoGST.contentsChanged, p.stream),
			gSignalConnect(p.image, "invalidate-size", videoGST.sizeChanged, p.stream),
		}
	}
	uri := videoGST.fileURI(file)
	if headers.Len() > 0 {
		nextGSTVideo++
		p.headerKey = nextGSTVideo
		videoHTTPRequests.Store(p.headerKey, videoHTTPRequest{uri: goString(uri), headers: headers})
		gSignalConnect(pipeline, "source-setup", videoHTTP.setup, p.headerKey)
	}
	videoGST.uri(p.play, (*byte)(cPointer(uri)))
	gFree(uri)
	// Also load metadata and the first frame for a video opened paused.
	videoGST.pause(p.play)
	return p
}

func (p *gstVideo) prepareAppSink() {
	caps := videoGST.caps(cstr("video/x-raw,format=RGBA"))
	videoGST.setCaps(p.sink, caps)
	videoGST.miniUnref(caps)
	videoGST.setObject(p.sink, cstr("emit-signals"), int32(1), cstr("max-buffers"), uint32(1),
		cstr("drop"), int32(1), cstr("enable-last-sample"), int32(0), uintptr(0))
	nextGSTVideo++
	p.key = nextGSTVideo
	gstVideoSamples.Store(p.key, p)
	gSignalConnect(p.sink, "new-sample", videoGST.sampleReady, p.key)
	gSignalConnect(p.sink, "new-preroll", videoGST.sampleReady, p.key)
}

func (p *gstVideo) close() {
	delete(gstVideos, p.stream)
	if p.headerKey != 0 {
		videoHTTPRequests.Delete(p.headerKey)
	}
	if p.appSink {
		gstVideoSamples.Delete(p.key)
	}
	for _, signal := range p.paintableSignals {
		if signal != 0 {
			gSignalHandlerDisconnect(p.image, signal)
		}
	}
	gObjectUnref(p.adapter)
	videoGST.stop(p.play)
	gObjectUnref(p.play)
	gObjectUnref(p.sink)
	if p.image != 0 {
		gObjectUnref(p.image)
	}
}

func videoSnapshot(stream, snap uintptr, width, height float64) {
	if p := gstVideos[stream]; p != nil && p.image != 0 {
		gdkPaintableSnapshot(p.image, snap, width, height)
	}
}

func videoWidth(stream uintptr) int32 {
	if p := gstVideos[stream]; p != nil && p.image != 0 {
		return gdkPaintableGetIntrinsicWidth(p.image)
	}
	return 0
}

func videoHeight(stream uintptr) int32 {
	if p := gstVideos[stream]; p != nil && p.image != 0 {
		return gdkPaintableGetIntrinsicHeight(p.image)
	}
	return 0
}

func videoContentsChanged(_, stream uintptr) { videoGST.invalidateContents(stream) }
func videoSizeChanged(_, stream uintptr)     { videoGST.invalidateSize(stream) }

func videoSampleReady(_, key uintptr) int32 {
	if value, ok := gstVideoSamples.Load(key); ok && value.(*gstVideo).framePending.CompareAndSwap(false, true) {
		gIdleAdd(videoGST.drawSample, key)
	}
	return 0 // GST_FLOW_OK
}

func videoDrawSample(key uintptr) int32 {
	value, ok := gstVideoSamples.Load(key)
	if !ok {
		return 0
	}
	p := value.(*gstVideo)
	p.framePending.Store(false)
	sample := videoGST.pullSample(p.sink, 0)
	if sample == 0 {
		sample = videoGST.pullPreroll(p.sink, 0)
	}
	if sample != 0 {
		p.setFrame(sample)
		videoGST.miniUnref(sample)
	}
	return 0
}

// GstVideoMeta's prefix through the first plane's stride (Linux has 64-bit targets). Buffers without this metadata use RGBA's packed rows.
type gstVideoPlane struct {
	metaFlags                                uint32
	metaInfo, buffer                         uintptr
	flags, format, id, width, height, planes uint32
	offset                                   [4]uintptr
	stride                                   [4]int32
}

func (p *gstVideo) setFrame(sample uintptr) {
	caps, buf := videoGST.sampleCaps(sample), videoGST.sampleBuffer(sample)
	if caps == 0 || buf == 0 {
		return
	}
	s := videoGST.capsStructure(caps, 0)
	var width, height int32
	if videoGST.structureInt(s, cstr("width"), &width) == 0 || videoGST.structureInt(s, cstr("height"), &height) == 0 ||
		width <= 0 || height <= 0 {
		return
	}
	offset, stride := uintptr(0), uintptr(width)*4
	if meta := videoGST.videoMeta(buf); meta != 0 {
		plane := (*gstVideoPlane)(cPointer(meta))
		if int64(plane.stride[0]) < int64(width)*4 {
			return
		}
		offset, stride = plane.offset[0], uintptr(plane.stride[0])
	}
	size := stride*uintptr(height-1) + uintptr(width)*4
	var pixels, copied uintptr
	videoGST.extract(buf, offset, size, &pixels, &copied)
	if copied != size || pixels == 0 {
		gFree(pixels)
		return
	}
	bytes := videoGST.takeBytes(pixels, copied)
	const rgba = 5 // GDK_MEMORY_R8G8B8A8
	texture := gdkMemoryTextureNew(width, height, rgba, bytes, stride)
	gBytesUnref(bytes)
	sizeChanged := videoWidth(p.stream) != width || videoHeight(p.stream) != height
	if p.image != 0 {
		gObjectUnref(p.image)
	}
	p.image = texture
	if sizeChanged {
		videoGST.invalidateSize(p.stream)
	}
	videoGST.invalidateContents(p.stream)
}

func videoStreamPlay(stream uintptr) int32 {
	if p := gstVideos[stream]; p != nil {
		videoGST.play(p.play)
		return 1
	}
	return 0
}

func videoStreamPause(stream uintptr) {
	if p := gstVideos[stream]; p != nil {
		videoGST.pause(p.play)
	}
}

func videoStreamSeek(stream uintptr, micros int64) {
	if p := gstVideos[stream]; p != nil {
		videoGST.seek(p.play, uint64(micros)*1000)
	}
}

func videoStreamAudio(stream uintptr, muted int32, volume float64) {
	if p := gstVideos[stream]; p != nil {
		videoGST.mute(p.play, muted)
		videoGST.volume(p.play, volume*volume*volume)
	}
}

func videoTimeChanged(_ uintptr, nanos uint64, stream uintptr) {
	if gtkMediaStreamIsPrepared(stream) == 0 {
		videoPrepareCurrent(stream)
	}
	if nanos != ^uint64(0) {
		videoGST.gtkUpdate(stream, int64(nanos/1000))
	}
}

func videoMediaInfo(_, info, stream uintptr) {
	videoPrepare(stream, info, false)
}

func videoStateChanged(_ uintptr, state int32, stream uintptr) {
	const paused = 2 // GST_PLAY_STATE_PAUSED
	if state >= paused && gtkMediaStreamIsPrepared(stream) == 0 {
		videoPrepareCurrent(stream)
	}
}

func videoPrepareCurrent(stream uintptr) {
	if p := gstVideos[stream]; p != nil {
		if info := videoGST.currentInfo(p.play); info != 0 {
			videoPrepare(stream, info, true)
			gObjectUnref(info)
		}
	}
}

func videoPrepare(stream, info uintptr, allowLive bool) {
	if info == 0 || gtkMediaStreamIsPrepared(stream) != 0 {
		return
	}
	nanos := videoGST.mediaDuration(info)
	seekable := videoGST.seekable(info)
	if nanos == 0 || nanos == ^uint64(0) {
		if !allowLive || seekable != 0 {
			// GstPlay first describes some finite streams before it has queried their duration. Wait for the actual metadata.
			return
		}
		nanos = 0
	}
	audio := videoGST.audioStreams(info) > 0
	video := videoGST.videoStreams(info) > 0
	if audio || video {
		videoGST.gtkPrepared(stream, boolInt(audio), boolInt(video), seekable, int64(nanos/1000))
	}
}

func videoSeekDone(adapter uintptr, nanos uint64, stream uintptr) {
	if videoGST.gtkSeekPending(stream) != 0 {
		videoGST.gtkSeekDone(stream)
	}
	videoTimeChanged(adapter, nanos, stream)
}

func videoStreamEnd(_, stream uintptr) {
	if videoGST.gtkLoop(stream) != 0 {
		videoStreamSeek(stream, 0)
		return
	}
	videoGST.gtkEnded(stream)
}

func videoStreamBuffer(_ uintptr, percent int32, stream uintptr) {
	if p := gstVideos[stream]; p != nil && !p.v.video.failed {
		p.w.videoBuffer(p.v, percent < 100)
	}
}

func videoStreamError(_, err, _, stream uintptr) {
	if err != 0 && gtkMediaStreamGetError(stream) == 0 {
		videoGST.gtkError(stream, videoGST.copyError(err))
	}
}

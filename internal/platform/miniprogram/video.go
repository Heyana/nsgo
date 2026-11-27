package miniprogram

import (
	"strconv"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/videoplay"
)

// videoState tracks a <video> view's configuration, playback ownership and
// queued VideoContext commands.
type videoState struct {
	src          string
	paused       bool
	made         bool
	repeat       bool
	rate         float32
	interval     time.Duration
	loaded       bool
	buffering    bool
	currentTime  time.Duration
	duration     time.Duration
	lastProgress time.Duration
	hasProgress  bool
	playback     videoplay.State
	configJSON   string
	cmdSeq       int
	pending      []videoCmd
}

type videoCmd struct {
	name string
	arg  float64
	has  bool
}

func videoObjectFit(mode driver.ResizeMode) string {
	switch mode {
	case driver.Cover:
		return "cover"
	case driver.Stretch:
		return "fill"
	default:
		return "contain"
	}
}

// setVideo applies ItemVideo to a KindVideo view, emitting "vd" when the
// <video> element attributes change and queueing VideoContext commands for
// playback and rate changes.
func (m *mounter) setVideo(v *view, c *driver.VideoConfig) {
	if v.video == nil {
		v.video = &videoState{}
	}
	vd := v.video
	url := ""
	if c.Source != "" {
		url = m.h.imageURL(driver.ImageSource{URI: c.Source})
	}
	sourceChanged := url != vd.src
	playChanged := !vd.made || sourceChanged || c.Paused != vd.paused
	wasMade := vd.made
	vd.paused, vd.made, vd.repeat = c.Paused, true, c.Repeat
	vd.interval = c.ProgressInterval
	if vd.interval <= 0 {
		vd.interval = 250 * time.Millisecond
	}
	if sourceChanged {
		vd.playback = videoplay.State{}
		vd.src = url
		vd.loaded = false
		vd.buffering = false
		vd.currentTime = 0
		vd.duration = 0
		vd.hasProgress = false
		if url != "" {
			m.send(v, driver.Event{Kind: driver.EventLoadStart})
		}
	}

	muted := c.Muted || c.Volume <= 0
	var cfg object
	cfg.str("src", url).
		boolean("ap", !c.Paused && url != "").
		boolean("loop", c.Repeat).
		boolean("muted", muted).
		boolean("ctrl", c.Controls).
		str("fit", videoObjectFit(c.ResizeMode))
	raw := cfg.bytes()
	if s := string(raw); s != vd.configJSON {
		vd.configJSON = s
		m.o.op("vd", v.id).raw(raw)
	}

	rate := c.Rate
	if rate <= 0 {
		rate = 1
	}
	if url != "" && (rate != vd.rate && (vd.rate != 0 || rate != 1) || sourceChanged && rate != 1) {
		vd.pending = append(vd.pending, videoCmd{name: "rate", arg: float64(rate), has: true})
	}
	vd.rate = rate

	if !vd.playback.Configure(playChanged) {
		return
	}
	if c.Paused || url == "" {
		vd.playback.Applied(false)
		if wasMade || sourceChanged {
			vd.pending = append(vd.pending, videoCmd{name: "pause"})
		}
		return
	}
	if vd.playback.Rewinds(vd.currentTime) {
		vd.currentTime = 0
		vd.pending = append(vd.pending, videoCmd{name: "seek", arg: 0, has: true})
	}
	vd.playback.Applied(true)
	if wasMade || !sourceChanged {
		vd.pending = append(vd.pending, videoCmd{name: "play"})
	}
}

// seekVideo queues a VideoContext.seek command and resumes playback if seeking
// back before the end of a non-repeating unpaused video.
func (m *mounter) seekVideo(v *view, t time.Duration) {
	if v.video == nil {
		v.video = &videoState{}
	}
	vd := v.video
	t = max(t, 0)
	replay := vd.playback.Completed && (vd.duration == 0 || t < vd.duration)
	vd.currentTime = t
	vd.pending = append(vd.pending, videoCmd{name: "seek", arg: t.Seconds(), has: true})
	if replay {
		vd.playback.Completed = false
		if !vd.paused {
			vd.playback.Applied(true)
			vd.pending = append(vd.pending, videoCmd{name: "play"})
		}
	}
}

// flushVideos emits queued VideoContext commands ("vc") as one ordered payload
// per video view so multiple commands in the same batch do not overwrite each
// other in setData.
func (m *mounter) flushVideos() {
	for _, v := range m.views {
		if v == nil || v.video == nil || len(v.video.pending) == 0 {
			continue
		}
		vd := v.video
		vd.cmdSeq++
		b := []byte{'['}
		b = strconv.AppendInt(b, int64(vd.cmdSeq), 10)
		b = append(b, ",["...)
		for i, cmd := range vd.pending {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, '[')
			b = appendString(b, cmd.name)
			if cmd.has {
				b = append(b, ',')
				b = appendNum(b, cmd.arg)
			}
			b = append(b, ']')
		}
		b = append(b, "]]"...)
		vd.pending = vd.pending[:0]
		m.o.op("vc", v.id).raw(b)
	}
}

func secondsDuration(sec float64) time.Duration {
	if sec != sec || sec < 0 || sec > 1e12 {
		return 0
	}
	return time.Duration(sec * float64(time.Second))
}

// videoLoaded reports metadata from <video>'s bindloadedmetadata (or first
// timeupdate with duration on platforms without loadedmetadata).
func (m *mounter) videoLoaded(id driver.NodeID, durationSec, width, height float64) {
	v := m.view(id)
	if v == nil || v.video == nil {
		return
	}
	vd := v.video
	vd.loaded = true
	if d := secondsDuration(durationSec); d > 0 {
		vd.duration = d
	}
	m.send(v, driver.Event{
		Kind:           driver.EventLoad,
		Duration:       vd.duration,
		RateChangeable: true,
		Size:           driver.Size{Width: driver.Points(width), Height: driver.Points(height)},
	})
}

func (m *mounter) videoPlay(id driver.NodeID) {
	v := m.view(id)
	if v == nil || v.video == nil {
		return
	}
	vd := v.video
	vd.playback.Completed = false
	if vd.buffering {
		vd.buffering = false
		m.send(v, driver.Event{Kind: driver.EventBuffer, Buffering: false})
	}
	if ev, ok := vd.playback.Changed(true, vd.paused, driver.VideoPauseControls); ok {
		m.send(v, ev)
	}
}

func (m *mounter) videoPause(id driver.NodeID) {
	v := m.view(id)
	if v == nil || v.video == nil {
		return
	}
	vd := v.video
	if vd.playback.Completed {
		return
	}
	if ev, ok := vd.playback.Changed(false, vd.paused, driver.VideoPauseControls); ok {
		m.send(v, ev)
	}
}

func (m *mounter) videoEnded(id driver.NodeID) {
	v := m.view(id)
	if v == nil || v.video == nil {
		return
	}
	vd := v.video
	if vd.duration > 0 && vd.currentTime < vd.duration {
		vd.currentTime = vd.duration
	}
	vd.playback.Ended(vd.repeat, vd.currentTime)
	if vd.buffering {
		vd.buffering = false
		m.send(v, driver.Event{Kind: driver.EventBuffer, Buffering: false})
	}
	m.send(v, driver.Event{Kind: driver.EventProgress, Time: vd.currentTime, Duration: vd.duration})
	m.send(v, driver.Event{Kind: driver.EventEnd})
}

func (m *mounter) videoTimeUpdate(id driver.NodeID, currentSec, durationSec float64) {
	v := m.view(id)
	if v == nil || v.video == nil {
		return
	}
	vd := v.video
	cur := secondsDuration(currentSec)
	dur := secondsDuration(durationSec)
	vd.currentTime = cur
	if dur > 0 {
		vd.duration = dur
		if !vd.loaded {
			m.videoLoaded(id, durationSec, 0, 0)
		}
	}
	if vd.buffering {
		vd.buffering = false
		m.send(v, driver.Event{Kind: driver.EventBuffer, Buffering: false})
	}
	if vd.hasProgress && cur >= vd.lastProgress && cur-vd.lastProgress < vd.interval {
		return
	}
	vd.lastProgress = cur
	vd.hasProgress = true
	m.send(v, driver.Event{Kind: driver.EventProgress, Time: vd.currentTime, Duration: vd.duration})
}

func (m *mounter) videoWaiting(id driver.NodeID) {
	v := m.view(id)
	if v == nil || v.video == nil || v.video.buffering {
		return
	}
	v.video.buffering = true
	m.send(v, driver.Event{Kind: driver.EventBuffer, Buffering: true})
}

func (m *mounter) videoError(id driver.NodeID, msg string) {
	v := m.view(id)
	if v == nil || v.video == nil {
		return
	}
	if msg == "" {
		msg = "the video could not play"
	}
	m.send(v, driver.Event{Kind: driver.EventError, Text: msg, Index: driver.VideoErrorOther})
}

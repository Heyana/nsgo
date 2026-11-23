package core

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// PictureInPictureState is a video's or mobile.PictureInPicture's native
// session state. Supported=false means this platform cannot provide PiP;
// Possible becomes true when the content and system are ready to start.
type PictureInPictureState = driver.PictureInPictureState

// PictureInPicture is core's implementation of mobile.PictureInPicture,
// which apps use. Its content has a layout root of its own and occupies
// no space where it is declared. Deleting it stops its session.
type PictureInPicture struct {
	Size          Size
	Paused        bool
	PausedFunc    func() bool
	Position      time.Duration
	PositionFunc  func() time.Duration
	Duration      time.Duration
	Automatically bool
	Children      []Node
	Ref           *Ref
	TestID        string
	OnState       func(PictureInPictureState)
	OnRestore     func()
	OnPlay        func()
	OnPause       func()
	OnSkip        func(time.Duration)
}

func (p PictureInPicture) Render(*Scope) Node { return p }
func (p PictureInPicture) mountIn(t *Tree, cx *Scope, parent *node) {
	size := p.Size
	if size.Width <= 0 {
		size.Width = 320
	}
	if size.Height <= 0 {
		size.Height = 180
	}
	n := t.newNode(driver.KindView, parent, cx)
	n.pip = &pictureInPictureNode{config: driver.PictureInPictureConfig{Size: size, Paused: p.Paused,
		Position: p.Position, Duration: p.Duration, Automatically: p.Automatically}, state: p.OnState,
		restore: p.OnRestore, play: p.OnPlay, pause: p.OnPause, skip: p.OnSkip}
	n.props = driver.ViewProps{TestID: p.TestID}
	t.setRef(n, p.Ref)
	t.pips = append(t.pips, n)
	if n.parent != nil {
		t.childrenChanged(n.parent)
	}
	cx.renderEffect(func() {
		c := n.pip.config
		if p.PausedFunc != nil {
			c.Paused = p.PausedFunc()
		}
		if p.PositionFunc != nil {
			c.Position = max(p.PositionFunc(), 0)
		}
		if c != n.pip.config {
			n.pip.config = c
			t.touch(n)
		}
	})
	for _, c := range p.Children {
		t.mount(cx, n, c)
	}
	if !pictureInPicturePlatform(cx) && p.OnState != nil {
		t.reports = append(t.reports, report{owner: n, fn: func() {
			if !pictureInPicturePlatform(cx) {
				p.OnState(PictureInPictureState{})
			}
		}})
	}
}

type pictureInPictureNode struct {
	config, sent         driver.PictureInPictureConfig
	state                func(PictureInPictureState)
	restore, play, pause func()
	skip                 func(time.Duration)
}

func (t *Tree) layoutPictureInPictures() {
	live := t.pips[:0]
	for _, n := range t.pips {
		if n.deleted {
			continue
		}
		live = append(live, n)
		if n.layoutDirty {
			t.layoutRoot(n, n.pip.config.Size, true)
			t.setFrame(n, Bounds{Size: n.pip.config.Size})
			t.syncFrames(n)
		}
	}
	clear(t.pips[len(live):])
	t.pips = live
}

func (t *Tree) emitPictureInPictures() {
	for _, n := range t.pips {
		if !n.deleted {
			t.emit(n)
		}
	}
}

func (t *Tree) dispatchPictureInPicture(n *node, e driver.Event) bool {
	if e.Kind != driver.EventNative || e.Text != driver.PictureInPictureEventName {
		return false
	}
	p := n.pip
	if n.video != nil {
		p = n.video.pip
	}
	if p == nil {
		return false
	}
	data, ok := e.Data.(*driver.PictureInPictureEvent)
	if !ok || data == nil {
		return true
	}
	t.within(n.owner, func() {
		switch data.Action {
		case "restore":
			if p.restore != nil {
				p.restore()
			}
			t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemPictureInPictureCommand, ID: n.id, Count: driver.PictureInPictureRestored, Before: NodeID(data.Token)})
			t.changed = true
		case "play":
			if p.play != nil {
				p.play()
			}
		case "pause":
			if p.pause != nil {
				p.pause()
			}
		case "skip":
			if p.skip != nil {
				p.skip(data.Time)
			}
		default:
			if p.state != nil {
				p.state(data.State)
			}
		}
	})
	return true
}

// StartPictureInPicture starts the video or mobile.PictureInPicture r
// names. The native state callback reports success or a distinct failure.
// Calling it while active is idempotent. It must follow a user action.
func (r *Ref) StartPictureInPicture() { r.pictureInPictureCommand(driver.PictureInPictureStart) }

// StopPictureInPicture ends r's session, if it is active.
func (r *Ref) StopPictureInPicture() { r.pictureInPictureCommand(driver.PictureInPictureStop) }
func (r *Ref) pictureInPictureCommand(command uint32) {
	if r.n == nil || r.n.deleted {
		return
	}
	p := r.n.pip
	supported := pictureInPicturePlatform(r.n.owner)
	if r.n.video != nil && r.n.video.config.PictureInPicture {
		p = r.n.video.pip
		supported = videoPictureInPicturePlatform(r.n.owner)
	}
	if p == nil {
		return
	}
	if !supported {
		if p.state != nil {
			p.state(PictureInPictureState{})
		}
		return
	}
	r.t.commands = append(r.t.commands, driver.MountItem{Kind: driver.ItemPictureInPictureCommand, ID: r.n.id, Count: command})
	r.t.changed = true
}

func pictureInPicturePlatform(cx *Scope) bool {
	return cx.Env().OS() == OSIOS && cx.Env().Idiom() != IdiomWatch
}

// Video backends may offer PiP without custom-content sessions.
func videoPictureInPicturePlatform(cx *Scope) bool {
	switch cx.Env().OS() {
	case OSMacOS, OSWeb, OSAndroid:
		return true
	}
	return pictureInPicturePlatform(cx)
}

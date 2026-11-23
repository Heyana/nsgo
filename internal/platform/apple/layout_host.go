//go:build darwin

package apple

import (
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// LayoutBatch tracks views inserted or retained by one animated commit.
type LayoutBatch struct {
	Animation *driver.LayoutAnimation
	Created   map[driver.NodeID]bool
	Deleted   map[objc.ID]bool
}

// LayoutHost supplies a backend's native views and placement to the shared layout transition lifecycle. Now is the frame clock; Start is when a new frame motion on a transition starts, which UIKit moves back to when the keyboard started moving.
type LayoutHost[V comparable] struct {
	Motions map[V]*LayoutMotion
	Ghosts  *[]*LayoutGhost
	ID      func(V) driver.NodeID
	Native  func(V) objc.ID
	Framed  func(V) bool
	Frame   func(V) driver.Bounds
	Shift   func(V) driver.Point
	Place   func(V, driver.Bounds)
	Restyle func(V)
	Disable func(V)
	Now     func() time.Duration
	Start   func(*driver.LayoutTransition) time.Duration
	Tick    func()
}

func (host LayoutHost[V]) motion(v V) *LayoutMotion {
	if m := host.Motions[v]; m != nil {
		return m
	}
	m := &LayoutMotion{}
	host.Motions[v] = m
	return m
}

// SetFrame animates a changed frame from where it shows when the batch or the item asks for it, retargeting a running motion with its velocity.
func (host LayoutHost[V]) SetFrame(v V, item *driver.MountItem, batch *LayoutBatch) {
	frame, m := host.Frame(v), host.Motions[v]
	target := frame
	if m != nil && m.Frame != nil {
		target = m.Frame.Target()
	}
	if item.Frame == target && host.Framed(v) {
		return
	}
	transition := item.Transition
	if batch != nil && batch.Animation.Update != nil && !batch.Created[host.ID(v)] {
		transition = batch.Animation.Update
	}
	if transition != nil && host.Framed(v) {
		m = host.motion(v)
		if m.Frame != nil {
			m.Frame.Retarget(item.Frame, *transition, host.Now())
		} else {
			motion := driver.StartLayoutMotion(frame, item.Frame, *transition, host.Start(transition))
			m.Frame = &motion
		}
		host.Tick()
		return
	}
	if m != nil {
		m.Frame = nil
	}
	host.Place(v, item.Frame)
}

// Appear starts only the top inserted view of an appearing subtree.
func (host LayoutHost[V]) Appear(v V, batch *LayoutBatch, parent driver.NodeID) {
	if batch == nil || batch.Animation.Create == nil || !batch.Created[host.ID(v)] || batch.Created[parent] {
		return
	}
	m := host.motion(v)
	m.Appear, m.AppearStart = batch.Animation.Create, host.Now()
	host.Tick()
}

// Match moves a replacement from its predecessor's box in the new parent.
func (host LayoutHost[V]) Match(v, old V, batch *LayoutBatch) {
	var zero V
	if batch == nil || batch.Animation.Update == nil || old == zero || !host.Framed(old) || !host.Framed(v) {
		return
	}
	native, previous := host.Native(v), host.Native(old)
	parent := native.Send(layoutSuperview)
	if parent == 0 || previous.Send(layoutSuperview) == 0 {
		return
	}
	r := objc.Send[CGRect](parent, sel("convertRect:fromView:"), objc.Send[CGRect](previous, selBounds), previous)
	shift := host.Shift(v)
	from := driver.Bounds{
		Origin: driver.Point{X: driver.Points(r.Origin.X) - shift.X, Y: driver.Points(r.Origin.Y) - shift.Y},
		Size:   driver.Size{Width: driver.Points(r.Size.Width), Height: driver.Points(r.Size.Height)},
	}
	previous.Send(selSetHidden, true)
	m := host.motion(v)
	m.Appear = nil
	motion := driver.StartLayoutMotion(from, host.Frame(v), *batch.Animation.Update, host.Start(batch.Animation.Update))
	m.Frame = &motion
	host.Place(v, from)
	host.Restyle(v)
	host.Tick()
}

// Vanish retains one exit per deleted subtree and lets UIKit disable its input.
func (host LayoutHost[V]) Vanish(v V, batch *LayoutBatch) bool {
	if batch == nil || batch.Animation.Delete == nil || !host.Framed(v) {
		return false
	}
	native := host.Native(v)
	parent := native.Send(layoutSuperview)
	if parent == 0 {
		return false
	}
	for n := parent; n != 0; n = n.Send(layoutSuperview) {
		if batch.Deleted[n] {
			return false
		}
	}
	batch.Deleted[native] = true
	native.Send(sel("retain"))
	if host.Disable != nil {
		host.Disable(v)
	}
	*host.Ghosts = append(*host.Ghosts, NewLayoutGhost(native, batch.Animation.Delete, host.Now()))
	host.Tick()
	return true
}

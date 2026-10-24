package engine

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/core"
)

// recorder is a platform that keeps the batches it is handed.
type recorder struct {
	core.ApproxTextEngine
	mu      sync.Mutex
	batches []*core.MountBatch
	mounted chan struct{}
	// frames receives a value for each display frame the app asks for.
	frames chan struct{}
}

func (r *recorder) Mount(b *core.MountBatch) {
	r.mu.Lock()
	r.batches = append(r.batches, b)
	r.mu.Unlock()
	select {
	case r.mounted <- struct{}{}:
	default:
	}
}

// RequestFrame implements Platform: the app waits for a display frame.
func (r *recorder) RequestFrame() {
	if r.frames == nil {
		return
	}
	select {
	case r.frames <- struct{}{}:
	default:
	}
}

// texts returns the latest text of every text view.
func (r *recorder) texts() map[core.NodeID]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[core.NodeID]string{}
	for _, b := range r.batches {
		for _, it := range b.Items {
			if it.Kind == core.ItemText {
				out[it.ID] = it.Text.Text
			}
		}
	}
	return out
}

func TestDispatchNeverBlocksAndMerges(t *testing.T) {
	p := &recorder{mounted: make(chan struct{}, 1)}
	var presses int
	var button core.NodeID
	e := Start(p, func(cx *core.Scope) core.Node {
		count := core.NewSignal(cx, 0)
		return core.View{Children: []core.Node{
			core.View{OnPress: func(core.PressEvent) { presses++; count.Set(presses) }, TestID: "button"},
			core.Text{ContentFunc: func() string { return strconv.Itoa(count.Get()) }},
		}}
	}, Options{Size: core.Size{Width: 100, Height: 100}})
	defer e.Close()
	<-p.mounted
	p.mu.Lock()
	for _, b := range p.batches {
		for _, it := range b.Items {
			if it.Kind == core.ItemViewProps && it.ViewProps.TestID == "button" {
				button = it.ID
			}
		}
	}
	p.mu.Unlock()

	// Far more than any fixed buffer, from a goroutine standing in for the
	// main thread: none of these may block.
	done := make(chan struct{})
	go func() {
		for range 5000 {
			e.Dispatch(core.Event{Kind: core.EventPress, ID: button})
			e.Resize(core.Size{Width: 100, Height: 100})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Dispatch blocked")
	}
	deadline := time.After(5 * time.Second)
	for {
		found := false
		for _, s := range p.texts() {
			found = found || s == "5000"
		}
		if found {
			return
		}
		select {
		case <-p.mounted:
		case <-deadline:
			t.Fatalf("texts = %v, want 5000 presses counted", p.texts())
		}
	}
}

func TestCloseDisposesTheApp(t *testing.T) {
	p := &recorder{mounted: make(chan struct{}, 1)}
	cleaned := make(chan struct{})
	e := Start(p, func(cx *core.Scope) core.Node {
		cx.Cleanup(func() { close(cleaned) })
		return core.View{}
	}, Options{})
	<-p.mounted
	e.Close()
	select {
	case <-cleaned:
	default:
		t.Fatal("Close did not dispose the app")
	}
}

// The app's frame callbacks run when the platform reports the frame it
// was asked for, and a callback that waits again asks for another.
func TestFramesFollowThePlatform(t *testing.T) {
	p := &recorder{mounted: make(chan struct{}, 1), frames: make(chan struct{}, 4)}
	seen := make(chan time.Time, 4)
	e := Start(p, func(cx *core.Scope) core.Node {
		var next func(at time.Time)
		next = func(at time.Time) {
			seen <- at
			cx.RequestAnimationFrame(next)
		}
		cx.RequestAnimationFrame(next)
		return core.View{}
	}, Options{})
	defer e.Close()
	<-p.mounted

	select {
	case <-p.frames:
	case <-time.After(5 * time.Second):
		t.Fatal("the app asked for no display frame")
	}
	e.Frame()
	select {
	case at := <-seen:
		if at.IsZero() {
			t.Fatal("the frame had no time")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the frame did not run the app's callback")
	}
	select {
	case <-p.frames:
	case <-time.After(5 * time.Second):
		t.Fatal("the callback waiting for the next frame did not ask for it")
	}
}

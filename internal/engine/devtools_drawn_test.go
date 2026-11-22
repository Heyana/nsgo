package engine

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestEngineAuditRequestsFramesDuringTransformTransition(t *testing.T) {
	p := &recorder{mounted: make(chan struct{}, 8), frames: make(chan struct{}, 8)}
	clock := core.NewManualClock(time.Unix(1000, 0))
	var tx *core.Signal[core.Points]
	var scope *core.Scope
	e := Start(p, func(cx *core.Scope) core.Node {
		scope = cx
		tx = core.NewSignal[core.Points](cx, 0)
		return core.Group{
			core.View{TestID: "box", StyleFunc: func() core.Style {
				return core.Style{
					Width: core.Pt(40), Height: core.Pt(20),
					Transform:  core.Transform{TranslateX: tx.Get()},
					Transition: core.Transition{Duration: 100 * time.Millisecond, Properties: core.TransitionTransform},
				}
			}},
			core.UpdateHighlights{NoUpdates: true, Layout: core.LayoutAudit{Bounds: true}},
		}
	}, Options{
		CheckCommits: true,
		Size:         driver.Size{Width: 200, Height: 200},
		Clock:        clock,
	})
	defer e.Close()
	e.Sync()
	select {
	case <-p.frames:
		t.Fatal("idle audit requested a display frame")
	default:
	}

	scope.Post(func() { tx.Set(100) })
	e.Sync()
	select {
	case <-p.frames:
	default:
		t.Fatal("transform transition did not request a display frame for the audit")
	}

	// Step halfway through the transition: audit redraws and requests another frame.
	p.mu.Lock()
	beforeBatches := len(p.batches)
	p.mu.Unlock()
	clock.Advance(50 * time.Millisecond)
	e.Frame()
	e.Sync()
	p.mu.Lock()
	afterBatches := len(p.batches)
	p.mu.Unlock()
	if afterBatches <= beforeBatches {
		t.Fatal("expected audit redraw batch at midpoint of transition")
	}
	select {
	case <-p.frames:
	default:
		t.Fatal("expected another frame request while transition is still active")
	}

	// Step to the end of the transition: audit redraws final box and stops requesting frames.
	clock.Advance(50 * time.Millisecond)
	e.Frame()
	e.Sync()
	select {
	case <-p.frames:
		t.Fatal("settled transition requested another frame")
	default:
	}
}

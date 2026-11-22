package engine

import (
	"strconv"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestRemountResetsBeforeTheFullBatchAndKeepsState(t *testing.T) {
	p := &recorder{}
	var count *core.Signal[int]
	renders := 0
	e := Start(p, func(cx *core.Scope) core.Node {
		renders++
		count = core.NewSignal(cx, 0)
		return core.Text{ContentFunc: func() string { return strconv.Itoa(count.Get()) }}
	}, Options{Size: driver.Size{Width: 100, Height: 100}})
	defer e.Close()
	e.Sync()
	e.push(message{fn: func() { count.Set(7) }})
	e.Sync()
	resets := 0
	e.Remount(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		// A native receiver drops its previous tree at this marker.
		p.batches = nil
		resets++
	})
	e.Sync()
	p.mu.Lock()
	defer p.mu.Unlock()
	if resets != 1 || renders != 1 || len(p.batches) != 1 {
		t.Fatalf("reset=%d renders=%d batches=%d", resets, renders, len(p.batches))
	}
	var created, current bool
	for _, it := range p.batches[0].Items {
		if it.Kind == driver.ItemCreate {
			created = true
		}
		if it.Kind == driver.ItemText && it.Text.Text == "7" {
			current = true
		}
	}
	if !created || !current {
		t.Fatalf("reset was not followed by a complete current tree: %+v", p.batches[0])
	}
}

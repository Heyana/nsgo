package engine

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestAsyncKeyDownQueuesBeforeItsRelease(t *testing.T) {
	p := &recorder{}
	entered, release := make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	var heard []string
	e := Start(p, func(*core.Scope) core.Node {
		return core.View{OnKeyDown: func(k core.KeyEvent) bool {
			close(entered)
			<-release
			mu.Lock()
			heard = append(heard, "down")
			mu.Unlock()
			return true
		}, OnKeyUp: func(core.KeyEvent) {
			mu.Lock()
			heard = append(heard, "up")
			mu.Unlock()
		}}
	}, Options{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }); e.Close() })
	e.Sync()
	var id driver.NodeID
	p.mu.Lock()
	for _, b := range p.batches {
		for _, it := range b.Items {
			if it.Kind == driver.ItemEvents && it.Events.Has(driver.EventKeyDown) {
				id = it.ID
			}
		}
	}
	p.mu.Unlock()
	answer := e.DispatchKeyDownAsync(driver.Event{Kind: driver.EventKeyDown, ID: id, Text: "k"})
	e.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: id, Text: "k"})
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("asynchronous down never reached the handler")
	}
	select {
	case <-answer:
		t.Fatal("consume answered before the handler finished")
	default:
	}
	once.Do(func() { close(release) })
	if !<-answer {
		t.Fatal("asynchronous consume answer was lost")
	}
	e.Sync()
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(heard, []string{"down", "up"}) {
		t.Fatalf("transport release overtook its pending down: %v", heard)
	}
}

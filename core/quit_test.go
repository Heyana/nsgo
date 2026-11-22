package core

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestOnQuitRunsLivingHandlersOldestFirstOnce(t *testing.T) {
	var ran []string
	var gone *Signal[bool]
	var deadline time.Time
	h := newHost(t, Size{Width: 390, Height: 844}, func(cx *Scope) Node {
		gone = NewSignal(cx, false)
		cx.OnQuit(func(ctx context.Context) {
			deadline, _ = ctx.Deadline()
			ran = append(ran, "app")
		})
		return View{Children: []Node{
			Show{When: func() bool { return !gone.Get() }, Then: func(cx *Scope) Node {
				cx.OnQuit(func(context.Context) { ran = append(ran, "gone") })
				return View{}
			}},
			Func(func(cx *Scope) Node {
				cx.OnQuit(func(context.Context) { panic("broken") })
				cx.OnQuit(func(context.Context) { ran = append(ran, "after a panic") })
				return View{}
			}),
		}}
	})
	h.commit()
	gone.Set(true)
	h.commit()
	end := time.Now().Add(QuitTimeout)
	if !h.root.Quitting() {
		t.Fatal("Quitting found no handlers")
	}
	if !slices.Equal(ran, []string{"app", "after a panic"}) {
		t.Errorf("ran %v", ran)
	}
	if len(h.errs) != 1 {
		t.Errorf("the panic was reported as %v", h.errs)
	}
	h.errs = nil
	if d := deadline.Sub(end); d < 0 || d > time.Second {
		t.Errorf("the handlers' time ends %v past QuitTimeout", d)
	}
	if h.root.Quitting() || len(ran) != 2 {
		t.Errorf("a second quit ran %v", ran)
	}
}

func TestQuitRequestRunsNewestFirstAndCanCancel(t *testing.T) {
	var ran []string
	h := newHost(t, Size{Width: 390, Height: 844}, func(cx *Scope) Node {
		cx.OnQuitRequest(func(*QuitRequest) { ran = append(ran, "old") })
		return Func(func(cx *Scope) Node {
			cx.OnQuitRequest(func(q *QuitRequest) { ran = append(ran, "new"); q.Cancel() })
			return View{}
		})
	})
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventQuitRequest, Count: 7})
	b := h.tree.Commit()
	if !slices.Equal(ran, []string{"new"}) {
		t.Fatalf("handlers ran %v", ran)
	}
	if b == nil || len(b.Items) != 1 || b.Items[0].Kind != driver.ItemQuitResponse || b.Items[0].Count != 7 || b.Items[0].On {
		t.Fatalf("cancel response %#v", b)
	}
}

func TestRequestRestartMarksTheProcessOnlyAfterApproval(t *testing.T) {
	restarting.Store(false)
	t.Cleanup(func() { restarting.Store(false) })
	var cx *Scope
	var answer func(bool)
	cancel := true
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node {
		cx = c
		cx.OnQuitRequest(func(q *QuitRequest) {
			if cancel {
				q.Cancel()
			} else {
				answer = q.Defer()
			}
		})
		return View{}
	})
	h.commit()
	cx.RequestRestart()
	if it := command(h.commit(), driver.ItemQuit); it != nil || Restarting() {
		t.Fatalf("canceled restart: command=%+v, restarting=%v", it, Restarting())
	}
	cancel = false
	cx.RequestRestart()
	if it := command(h.commit(), driver.ItemQuit); it != nil || Restarting() {
		t.Fatalf("deferred restart: command=%+v, restarting=%v", it, Restarting())
	}
	answer(true)
	answer(false)
	h.drain()
	if it := command(h.commit(), driver.ItemQuit); it == nil || !it.On || !Restarting() {
		t.Fatalf("approved restart: command=%+v, restarting=%v", it, Restarting())
	}
}

func TestQuitRequestCanceledReportsState(t *testing.T) {
	q := &QuitRequest{}
	if q.Canceled() {
		t.Fatal("a new request is canceled")
	}
	q.Cancel()
	if !q.Canceled() {
		t.Fatal("Cancel did not change Canceled")
	}
}

func TestQuitRequestApprovesAfterHandlersPass(t *testing.T) {
	var ran []string
	h := newHost(t, Size{Width: 390, Height: 844}, func(cx *Scope) Node {
		cx.OnQuitRequest(func(*QuitRequest) { ran = append(ran, "old") })
		return Func(func(cx *Scope) Node {
			cx.OnQuitRequest(func(*QuitRequest) { ran = append(ran, "new") })
			return View{}
		})
	})
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventQuitRequest, Count: 8})
	b := h.tree.Commit()
	if !slices.Equal(ran, []string{"new", "old"}) {
		t.Fatalf("handlers ran %v", ran)
	}
	if b == nil || len(b.Items) != 1 || b.Items[0].Kind != driver.ItemQuitResponse || b.Items[0].Count != 8 || !b.Items[0].On {
		t.Fatalf("approval response %#v", b)
	}
}

func TestQuitRequestCanDeferAndAnswersOnce(t *testing.T) {
	var answer func(bool)
	h := newHost(t, Size{Width: 390, Height: 844}, func(cx *Scope) Node {
		cx.OnQuitRequest(func(q *QuitRequest) { answer = q.Defer() })
		return View{}
	})
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventQuitRequest, Count: 9})
	if b := h.tree.Commit(); b != nil {
		for _, it := range b.Items {
			if it.Kind == driver.ItemQuitResponse {
				t.Fatal("deferred request answered before callback")
			}
		}
	}
	answer(true)
	answer(false)
	h.drain()
	b := h.tree.Commit()
	if b == nil || len(b.Items) != 1 || b.Items[0].Kind != driver.ItemQuitResponse || b.Items[0].Count != 9 || !b.Items[0].On {
		t.Fatalf("deferred response %#v", b)
	}
}

func TestQuitRequestCancelsForAHandlerThatPanicsAfterDefer(t *testing.T) {
	var answer func(bool)
	ran := false
	h := newHost(t, Size{Width: 390, Height: 844}, func(cx *Scope) Node {
		cx.OnQuitRequest(func(*QuitRequest) { ran = true })
		return Func(func(cx *Scope) Node {
			cx.OnQuitRequest(func(q *QuitRequest) {
				answer = q.Defer()
				panic("broken dialog")
			})
			return View{}
		})
	})
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventQuitRequest, Count: 12})
	b := h.tree.Commit()
	if len(h.errs) != 1 {
		t.Fatalf("the panic was reported as %v", h.errs)
	}
	h.errs = nil
	if ran {
		t.Error("a handler past the deferring one ran")
	}
	if b == nil || len(b.Items) != 1 || b.Items[0].Kind != driver.ItemQuitResponse || b.Items[0].Count != 12 || b.Items[0].On {
		t.Fatalf("response for the panicking handler %#v", b)
	}
	answer(true)
	h.drain()
	if b := h.tree.Commit(); b != nil && len(b.Items) > 0 {
		t.Fatalf("the lost answer answered again %#v", b)
	}
}

func TestQuitRequestKeepsAnAnswerGivenBeforeAPanic(t *testing.T) {
	h := newHost(t, Size{Width: 390, Height: 844}, func(cx *Scope) Node {
		cx.OnQuitRequest(func(q *QuitRequest) {
			q.Defer()(true)
			panic("after answering")
		})
		return View{}
	})
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventQuitRequest, Count: 13})
	h.drain()
	b := h.tree.Commit()
	h.errs = nil
	if b == nil || len(b.Items) != 1 || b.Items[0].Kind != driver.ItemQuitResponse || b.Items[0].Count != 13 || !b.Items[0].On {
		t.Fatalf("response %#v", b)
	}
}

func TestQuitRequestSurvivesAHandlerDisposingOthers(t *testing.T) {
	var ran []string
	h := newHost(t, Size{Width: 390, Height: 844}, func(cx *Scope) Node {
		cx.OnQuitRequest(func(*QuitRequest) { ran = append(ran, "root") })
		child := cx.Child()
		child.Run(func() {
			child.OnQuitRequest(func(*QuitRequest) { ran = append(ran, "older") })
			child.OnQuitRequest(func(*QuitRequest) {
				ran = append(ran, "newest")
				// As closing the window it lives in does.
				child.Dispose()
			})
		})
		return View{}
	})
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventQuitRequest, Count: 10})
	b := h.tree.Commit()
	if len(h.errs) > 0 {
		t.Fatalf("errors %v", h.errs)
	}
	if !slices.Equal(ran, []string{"newest", "root"}) {
		t.Fatalf("handlers ran %v", ran)
	}
	if b == nil || len(b.Items) != 1 || b.Items[0].Kind != driver.ItemQuitResponse || b.Items[0].Count != 10 || !b.Items[0].On {
		t.Fatalf("approval response %#v", b)
	}
}

func TestQuitRequestApprovesWithoutHandlers(t *testing.T) {
	h := newHost(t, Size{Width: 390, Height: 844}, func(*Scope) Node { return View{} })
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventQuitRequest, Count: 11})
	b := h.tree.Commit()
	if b == nil || len(b.Items) != 1 || b.Items[0].Kind != driver.ItemQuitResponse || !b.Items[0].On {
		t.Fatalf("default response %#v", b)
	}
}

func TestHidingKeepsTheHandlersForTheNextHide(t *testing.T) {
	runs := 0
	h := newHost(t, Size{Width: 390, Height: 844}, func(cx *Scope) Node {
		cx.OnQuit(func(context.Context) { runs++ })
		return View{}
	})
	h.commit()
	if !h.root.Hiding() || !h.root.Hiding() || runs != 2 {
		t.Errorf("two hides ran the handler %d times", runs)
	}
	if !h.root.Quitting() || h.root.Hiding() || runs != 3 {
		t.Errorf("a hide after quitting ran it %d times", runs)
	}
}

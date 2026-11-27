//go:build nsgodev

package core

import (
	"encoding/json"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/devtools"
)

func TestDevKeepSavesAndRestoresSignalAcrossRestarts(t *testing.T) {
	devtools.LoadState(nil)
	t.Cleanup(func() { devtools.LoadState(nil) })

	// First run: register a signal, mutate it, snapshot state as devServer would.
	root1 := NewRoot(RootOptions{Post: func(func()) {}})
	count1 := NewSignal(root1, 1)
	DevKeep(root1, "counter", count1)
	count1.Set(42)
	snap := devtools.SnapshotState()
	root1.Dispose()

	// Simulated restart: clear in-memory state, reload snapshot from dev server.
	devtools.LoadState(nil)
	devtools.LoadState(snap)

	var seenOnFirstRender int
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		count2 := NewSignal(cx, 1)
		DevKeep(cx, "counter", count2)
		seenOnFirstRender = count2.Get()
		return View{}
	})
	h.commit()
	if seenOnFirstRender != 42 {
		t.Fatalf("restored counter on first render = %d, want 42", seenOnFirstRender)
	}
}

func TestDevKeepFuncTypeMismatchAndUnserializable(t *testing.T) {
	devtools.LoadState(map[string]json.RawMessage{
		"num": json.RawMessage(`"not-an-int"`),
	})
	t.Cleanup(func() { devtools.LoadState(nil) })

	root := NewRoot(RootOptions{Post: func(func()) {}})
	defer root.Dispose()

	// Mismatched type must cleanly keep default value.
	num := NewSignal(root, 7)
	DevKeep(root, "num", num)
	if got := num.Get(); got != 7 {
		t.Fatalf("mismatched restore produced %d, want default 7", got)
	}

	// Unserializable values (func) must not panic and must not be stored.
	fn := NewSignalFunc(root, func() {}, nil)
	DevKeep(root, "bad-fn", fn)
	if _, ok := devtools.ReadState("bad-fn"); ok {
		t.Fatal("unserializable func value was unexpectedly stored")
	}

	// DevKeepFunc with custom struct round-trips cleanly.
	type filter struct {
		Query string
		Page  int
	}
	sig := NewSignal(root, filter{Query: "go", Page: 3})
	DevKeepFunc(root, "filter", sig.Get, sig.Set)
	snap := devtools.SnapshotState()

	devtools.LoadState(snap)
	root2 := NewRoot(RootOptions{Post: func(func()) {}})
	defer root2.Dispose()
	var restored filter
	sig2 := NewSignal(root2, filter{})
	DevKeepFunc(root2, "filter", sig2.Get, func(v filter) {
		restored = v
		sig2.Set(v)
	})
	if restored != (filter{Query: "go", Page: 3}) {
		t.Fatalf("restored filter = %+v, want {Query:go Page:3}", restored)
	}
}

func TestDevKeepStopsSavingAfterScopeDisposal(t *testing.T) {
	devtools.LoadState(nil)
	t.Cleanup(func() { devtools.LoadState(nil) })

	root := NewRoot(RootOptions{Post: func(func()) {}})
	defer root.Dispose()
	child := root.Child()
	sig := NewSignal(root, "before")
	DevKeep(child, "scoped", sig)
	sig.Set("saved")
	child.Dispose()
	sig.Set("after-dispose")

	raw, ok := devtools.ReadState("scoped")
	if !ok || string(raw) != `"saved"` {
		t.Fatalf("stored value after disposal = %q (%v), want %q", raw, ok, `"saved"`)
	}
}

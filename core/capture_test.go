package core

import (
	"bytes"
	"errors"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestCaptureRequest(t *testing.T) {
	var cx *Scope
	var ref Ref
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node {
		cx = c
		return View{Ref: &ref}
	})
	h.commit()
	for _, target := range []*Ref{nil, &ref} {
		var got []byte
		var calls int
		cx.Capture(target, func(data []byte, err error) {
			if err != nil || cx.current() != cx {
				t.Errorf("capture callback ran outside its owner or failed: %v", err)
			}
			got, calls = data, calls+1
		})
		// Capture must not block or invoke its callback synchronously before the platform answers.
		if calls != 0 {
			t.Fatalf("capture callback ran synchronously before commit (%d calls)", calls)
		}
		item := command(h.commit(), driver.ItemCapture)
		id := driver.NodeID(0)
		if target != nil {
			id = ref.n.id
		}
		if item == nil || item.ID != id || calls != 0 {
			t.Fatalf("capture request = %+v, callback count %d", item, calls)
		}
		data := []byte("png")
		answer := driver.CaptureResult(item.Count, data, nil)
		h.tree.Dispatch(answer)
		h.tree.Dispatch(answer)
		if !bytes.Equal(got, data) || calls != 1 {
			t.Errorf("capture returned %q after %d callbacks", got, calls)
		}
	}
}

func TestCaptureErrorsAndAccess(t *testing.T) {
	var root, held, confined, child *Scope
	var mounted, unmounted Ref
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		root = cx
		held = cx.Child()
		held.Run(func() { held.ProvideAccess(allows(func(AccessRequest) bool { return true })) })
		confined = cx.Child()
		confined.Run(func() { confined.Confine() })
		child = cx.Child()
		return View{Ref: &mounted}
	})
	h.commit()

	// Unmounted ref fails asynchronously on the engine goroutine without emitting a mount item.
	var unmountedErr error
	root.Capture(&unmounted, func(data []byte, err error) {
		if data != nil {
			t.Errorf("unmounted ref returned data %q", data)
		}
		unmountedErr = err
	})
	if unmountedErr != nil {
		t.Fatalf("unmounted ref callback ran synchronously: %v", unmountedErr)
	}
	if item := command(h.commit(), driver.ItemCapture); item != nil {
		t.Fatalf("unmounted ref emitted ItemCapture %+v", item)
	}
	h.drain()
	if unmountedErr == nil {
		t.Fatal("unmounted ref did not report an error")
	}

	// Access-held and confined scopes are refused outright with CodeAccessDenied.
	for name, scope := range map[string]*Scope{"access": held, "confined": confined} {
		var gotErr error
		scope.Capture(nil, func(_ []byte, err error) { gotErr = err })
		if gotErr != nil {
			t.Fatalf("%s scope callback ran synchronously", name)
		}
		h.drain()
		if ErrorCode(gotErr) != CodeAccessDenied {
			t.Errorf("%s scope capture err = %v, want CodeAccessDenied", name, gotErr)
		}
	}

	// NoCapture maps to CodeUnavailable; empty bytes and custom errors propagate as errors.
	cases := []struct {
		name     string
		answer   func(count uint32) driver.Event
		wantCode string
	}{
		{"unavailable", func(c uint32) driver.Event { return driver.Event{Kind: driver.EventFiles, Count: c, Text: driver.NoCapture} }, CodeUnavailable},
		{"empty", func(c uint32) driver.Event { return driver.CaptureResult(c, nil, nil) }, ""},
		{"failure", func(c uint32) driver.Event { return driver.CaptureResult(c, nil, errors.New("backend failed")) }, ""},
	}
	for _, tc := range cases {
		var gotErr error
		root.Capture(&mounted, func(_ []byte, err error) { gotErr = err })
		item := command(h.commit(), driver.ItemCapture)
		if item == nil {
			t.Fatalf("%s: no ItemCapture emitted", tc.name)
		}
		h.tree.Dispatch(tc.answer(item.Count))
		if gotErr == nil {
			t.Errorf("%s: expected error, got nil", tc.name)
		} else if tc.wantCode != "" && ErrorCode(gotErr) != tc.wantCode {
			t.Errorf("%s: ErrorCode(%v) = %q, want %q", tc.name, gotErr, ErrorCode(gotErr), tc.wantCode)
		}
	}

	// A disposed requesting owner still receives its answer under the root owner.
	var got []byte
	var ranOwner *Scope
	child.Capture(nil, func(data []byte, err error) {
		if err != nil {
			t.Errorf("disposed owner capture failed: %v", err)
		}
		got = data
		ranOwner = root.current()
	})
	item := command(h.commit(), driver.ItemCapture)
	if item == nil {
		t.Fatal("child capture emitted no ItemCapture")
	}
	child.Dispose()
	want := []byte("still-delivered")
	h.tree.Dispatch(driver.CaptureResult(item.Count, want, nil))
	if !bytes.Equal(got, want) || ranOwner != h.root {
		t.Errorf("disposed owner capture got %q under %v, want %q under tree root", got, ranOwner, want)
	}
}

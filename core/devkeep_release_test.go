//go:build !nsgodev

package core

import "testing"

func TestDevKeepIsNoOpInReleaseBuilds(t *testing.T) {
	root := NewRoot(RootOptions{Post: func(func()) {}})
	defer root.Dispose()

	sig := NewSignal(root, 10)
	DevKeep(root, "count", sig)
	if got := sig.Get(); got != 10 {
		t.Fatalf("DevKeep altered signal in release build: got %d, want 10", got)
	}

	called := false
	DevKeepFunc(root, "custom", func() int {
		called = true
		return 1
	}, func(int) {
		called = true
	})
	if called {
		t.Fatal("DevKeepFunc invoked callbacks in release build")
	}
}

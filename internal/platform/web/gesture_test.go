//go:build js && wasm

package web

import "testing"

func TestRemovePointerLetsGoOfTheRemoved(t *testing.T) {
	a, b, c := &pointer{id: 1}, &pointer{id: 2}, &pointer{id: 3}
	ps := []*pointer{a, b, c}
	got := removePointer(ps, b)
	if len(got) != 2 || got[0] != a || got[1] != c {
		t.Fatalf("removePointer left %v", got)
	}
	if ps[2] != nil {
		t.Error("the backing array keeps the removed pointer's place")
	}
}

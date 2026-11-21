//go:build android

package android

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestRequestReviewEncodesWithNoExtraPayload(t *testing.T) {
	var w writer
	encode(&w, &driver.MountBatch{
		Root:  1,
		Items: []driver.MountItem{{Kind: driver.ItemRequestReview}},
	}, false, nil, nil, nil, nil)
	r := reader{w.buf}
	if root := r.u32(); root != 1 {
		t.Fatalf("root = %d, want 1", root)
	}
	if anim := r.byte(); anim != 0 {
		t.Fatalf("layout animation kind = %d, want 0", anim)
	}
	if first := r.byte(); first != 0 {
		t.Fatalf("first flag = %d, want 0", first)
	}
	if n := r.u32(); n != 1 {
		t.Fatalf("item count = %d, want 1", n)
	}
	if kind := driver.ItemKind(r.byte()); kind != driver.ItemRequestReview {
		t.Fatalf("item kind = %v, want ItemRequestReview", kind)
	}
	if id := r.u32(); id != 0 {
		t.Fatalf("item id = %d, want 0", id)
	}
	if len(r.buf) != 0 {
		t.Fatalf("trailing payload = %d bytes, want 0", len(r.buf))
	}
}

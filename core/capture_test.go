package core

import (
	"bytes"
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

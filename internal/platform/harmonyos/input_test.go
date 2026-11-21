//go:build js && wasm

package harmonyos

import (
	"encoding/json"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestInputSendsEditability checks that a field's input operation carries its
// editability at o[13] and o[14] and its padding (top, right, bottom, left) at
// o[15]..o[18] (#1598).
func TestInputSendsEditability(t *testing.T) {
	pad := driver.Edges{Top: 18, Right: 20, Bottom: 22, Left: 12}
	for _, c := range []driver.InputConfig{{Readonly: true}, {Disabled: true}, {Multiline: true, Padding: pad}} {
		m := &mounter{views: map[driver.NodeID]*view{}}
		m.input(4, &c)
		var got [][]any
		if err := json.Unmarshal(m.o.bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || len(got[0]) != 19 || got[0][0] != "in" {
			t.Fatalf("input operations %v, want one in with 17 arguments", got)
		}
		o := got[0]
		if o[13] != c.Readonly || o[14] != c.Disabled {
			t.Errorf("readonly %v and disabled %v, want %v and %v", o[13], o[14], c.Readonly, c.Disabled)
		}
		if o[15] != float64(c.Padding.Top) || o[16] != float64(c.Padding.Right) ||
			o[17] != float64(c.Padding.Bottom) || o[18] != float64(c.Padding.Left) {
			t.Errorf("padding %v %v %v %v, want %+v", o[15], o[16], o[17], o[18], c.Padding)
		}
	}
}

//go:build js && wasm

package harmonyos

import (
	"encoding/json"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestViewPropsAndAccessibilityLabelOps verifies the vp and al argument layouts
// read by Index.ets: vp carries [id, testID, pointer, label, desc, disabled, level, group]
// and al carries [id, label].
func TestViewPropsAndAccessibilityLabelOps(t *testing.T) {
	m := &mounter{views: map[driver.NodeID]*view{}}
	b := &driver.MountBatch{}
	m.item(b, &driver.MountItem{
		Kind: driver.ItemViewProps,
		ID:   3,
		ViewProps: &driver.ViewProps{
			TestID:   "email-label",
			Label:    "Email",
			Hint:     "Required",
			LabelFor: 4,
			Disabled: true,
			Invalid:  true,
		},
	})
	m.item(b, &driver.MountItem{
		Kind:  driver.ItemAccessibilityLabel,
		ID:    3,
		Value: "Work email",
	})
	var got [][]any
	if err := json.Unmarshal(m.o.bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("ops = %v, want 2", got)
	}
	vp := got[0]
	if len(vp) != 9 || vp[0] != "vp" || vp[1] != float64(3) || vp[2] != "email-label" ||
		vp[4] != "Email" || vp[5] != "Required, invalid data" || vp[6] != true ||
		vp[7] != "yes" || vp[8] != true {
		t.Fatalf("vp op = %v", vp)
	}
	al := got[1]
	if len(al) != 3 || al[0] != "al" || al[1] != float64(3) || al[2] != "Work email" {
		t.Fatalf("al op = %v", al)
	}
}

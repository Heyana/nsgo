package keys

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The platform's own key events carry the physical position a test presses, so core.KeyCodesDown reads the position, a right-hand modifier and a numpad key apart from the left one and the digit row, while core.KeysDown reads the key the US layout puts there (#1242).
func TestPressedPositionsReachTheApp(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Padding: core.All(20)}, Children: []core.Node{
			core.Text{TestID: "codes", ContentFunc: func() string { return strings.Join(core.KeyCodesDown(cx), " ") }},
			core.Text{TestID: "keys", ContentFunc: func() string { return strings.Join(core.KeysDown(cx), " ") }},
		}}
	}, nsgotest.Size(300, 120))
	held := func(codes, keys string) {
		t.Helper()
		if got := app.Find("id:codes").Text; got != codes {
			t.Fatalf("core.KeyCodesDown reads %q, want %q", got, codes)
		}
		if got := app.Find("id:keys").Text; got != keys {
			t.Fatalf("core.KeysDown reads %q, want %q", got, keys)
		}
	}
	app.KeyCodeDown("KeyW")
	held("KeyW", "w")
	app.KeyCodeDown("ShiftRight")
	held("KeyW ShiftRight", "shift w")
	app.KeyCodeUp("KeyW")
	app.KeyCodeUp("ShiftRight")
	held("", "")
	app.KeyCodeDown("Numpad1")
	held("Numpad1", "1")
	app.KeyCodeUp("Numpad1")
	held("", "")
}

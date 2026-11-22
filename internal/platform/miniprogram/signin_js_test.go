//go:build js && wasm

package miniprogram

import (
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestMiniProgramSignInWasmBridge(t *testing.T) {
	for _, tc := range []struct {
		name, code, failure string
		prompt              bool
	}{
		{"success", "owned-code", "", true},
		{"cancelled", "", "cancelled", false},
		{"anonymous", "", "notSignedIn", false},
		{"malformed", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bridge := js.Global().Get("Object").New()
			host := js.FuncOf(func(_ js.Value, args []js.Value) any {
				if args[0].Get("prompt").Bool() != tc.prompt {
					t.Error("lost prompt option")
				}
				args[0].Get("complete").Invoke(map[string]any{"platform": "wechat", "code": tc.code, "error": tc.failure})
				return nil
			})
			defer host.Release()
			bridge.Set("signIn", host)
			answer := make(chan driver.Event, 1)
			requestMiniProgramSignIn(bridge, 0x80000010, tc.prompt, func(ev driver.Event) { answer <- ev })
			select {
			case ev := <-answer:
				if ev.Kind != driver.EventMiniProgramSignIn || ev.Count != 0x80000010 {
					t.Errorf("lost request identity")
				}
				if tc.code != "" {
					c, ok := ev.Data.(*driver.MiniProgramCredential)
					if ev.Text != "" || !ok || c.Code != tc.code || c.Platform != "wechat" {
						t.Error("lost host credential")
					}
				} else {
					want := tc.failure
					if want == "" {
						want = "failed"
					}
					if ev.Text != want || ev.Data != nil {
						t.Errorf("incorrect error %q", ev.Text)
					}
				}
			case <-time.After(time.Second):
				t.Fatal("no host answer")
			}
		})
	}
	bridge := js.Global().Get("Object").New()
	calls := 0
	requestMiniProgramSignIn(bridge, 16, false, func(ev driver.Event) {
		calls++
		if ev.Text != "unavailable" {
			t.Errorf("missing host error=%q", ev.Text)
		}
	})
	if calls != 1 {
		t.Errorf("missing host callbacks=%d", calls)
	}
}

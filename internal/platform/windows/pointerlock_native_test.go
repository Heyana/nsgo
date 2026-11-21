//go:build windows

package windows

import "testing"

func TestPointerLockKeepsRawMouseRegistration(t *testing.T) {
	previous, ok := previousMouseRegistration()
	if !ok {
		t.Fatal("could not read the process's initial Raw Input registration")
	}
	registered := rawInputDevice{UsagePage: rawMousePage, Usage: rawMouseUsage}
	if !setRawMouse(registered) {
		t.Fatal("could not register a raw mouse in the test process")
	}
	t.Cleanup(func() {
		if !setRawMouse(previous) {
			t.Error("could not restore the initial Raw Input registration")
		}
	})
	if got, ok := previousMouseRegistration(); !ok || got != registered {
		t.Fatalf("read registration (%+v, %t), want %+v", got, ok, registered)
	}
}

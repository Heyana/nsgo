package tsx

import (
	"testing"

	"github.com/nsgo-dev/nsgo/tsx/internal/js"
)

func TestStoppedRuntimeSkipsBoundCallbacks(t *testing.T) {
	r := decoding()
	t.Cleanup(r.vm.Close)
	r.sandbox = &sandboxed{}
	called := false
	fn := r.function(func(js.FunctionCall) js.Value {
		called = true
		return js.Undefined()
	})
	r.markStopped(ErrInterrupted)
	if got := r.call2(fn, js.Undefined()); called || !js.IsUndefined(got) {
		t.Fatalf("a stopped runtime called the bound callback, result = %v", got)
	}
}

//go:build js

package tsx

import "github.com/nsgo-dev/nsgo/tsx/internal/js"

// memory is nothing on the web, where Limits.Memory does not hold: the page's engine keeps its own heap.
type memory struct{}

// limitMemory does nothing on the web: the page's engine keeps its own heap, which Go cannot count.
func (r *runtime) limitMemory(int64) {}

func (r *runtime) encodeUTF8(v js.Value) js.Value {
	return r.vm.ToValue(r.vm.NewArrayBuffer([]byte(v.String())))
}

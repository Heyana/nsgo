//go:build js && wasm

package diagram

import (
	"syscall/js"
	"testing"
)

// The browser test runtime cannot listen on sockets; a real Blob uses its image decoder.
func pictureFixture(t *testing.T, data []byte) string {
	t.Helper()
	g := js.Global()
	bytes := g.Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(bytes, data)
	blob := g.Get("Blob").New([]any{bytes}, map[string]any{"type": "image/png"})
	uri := g.Get("URL").Call("createObjectURL", blob).String()
	t.Cleanup(func() { g.Get("URL").Call("revokeObjectURL", uri) })
	return uri
}

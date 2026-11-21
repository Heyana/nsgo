//go:build js && wasm

package imageload

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"syscall/js"
)

// FetchAsset reads the resource through the browser's fetch and TLS stack.
func FetchAsset(ctx context.Context, base, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	// Escape the already decoded path so literal percent signs, query
	// markers and fragments remain part of the file name.
	target := root.ResolveReference(&url.URL{Path: path})
	g := js.Global()
	abort := g.Get("AbortController").New()
	response, err := awaitAsset(ctx, abort, g.Call("fetch", target.String(), map[string]any{"signal": abort.Get("signal")}))
	if err != nil {
		return nil, err
	}
	status := response.Get("status").Int()
	if status == 404 || status == 410 {
		return nil, fs.ErrNotExist
	}
	if !response.Get("ok").Bool() {
		return nil, fmt.Errorf("reading bundled asset: HTTP %d", status)
	}
	buffer, err := awaitAsset(ctx, abort, response.Call("arrayBuffer"))
	if err != nil {
		return nil, err
	}
	source := g.Get("Uint8Array").New(buffer)
	data := make([]byte, source.Length())
	js.CopyBytesToGo(data, source)
	return data, nil
}

func awaitAsset(ctx context.Context, abort, promise js.Value) (js.Value, error) {
	type result struct {
		value js.Value
		err   error
	}
	done := make(chan result, 1)
	var resolved, rejected js.Func
	release := func() {
		resolved.Release()
		rejected.Release()
	}
	resolved = js.FuncOf(func(_ js.Value, args []js.Value) any {
		release()
		done <- result{value: args[0]}
		return nil
	})
	rejected = js.FuncOf(func(_ js.Value, args []js.Value) any {
		release()
		done <- result{err: fmt.Errorf("reading bundled asset: %s", js.Global().Get("String").Invoke(args[0]).String())}
		return nil
	})
	promise.Call("then", resolved, rejected)
	select {
	case r := <-done:
		return r.value, r.err
	case <-ctx.Done():
		abort.Call("abort")
		return js.Undefined(), ctx.Err()
	}
}

//go:build nsgodev && js && wasm && nsgominiprogram

package devtools

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"syscall/js"
)

// Fetch sends nsgo dev's server a request through the mini program's request API (nsgoMP.api.request), with the run's token (Token) and body as contentType when it has one, keeping net/http out of the 2 MB module.
func Fetch(method, url, contentType string, body io.Reader) (*Response, error) {
	return doMiniProgramRequest(miniProgramBridgeRequest, method, url, contentType, Token(), body)
}

func miniProgramBridgeRequest(req miniProgramRequest) (abort func(), err error) {
	bridge := js.Global().Get("nsgoMP")
	if !bridge.Truthy() {
		return nil, errors.New("devtools: mini program bridge unavailable")
	}
	api := bridge.Get("api")
	if !api.Truthy() || api.Get("request").Type() != js.TypeFunction {
		return nil, errors.New("devtools: mini program request API unavailable")
	}
	headers := make(map[string]any, len(req.Headers))
	for k, v := range req.Headers {
		headers[k] = v
	}
	opts := map[string]any{
		"url":          req.URL,
		"method":       req.Method,
		"header":       headers,
		"responseType": "arraybuffer",
	}
	if req.Body != "" {
		opts["data"] = req.Body
	}
	if req.EnableChunked {
		opts["enableChunked"] = true
	}

	var (
		mu    sync.Mutex
		funcs []js.Func
		done  bool
	)
	release := func() {
		mu.Lock()
		if done {
			mu.Unlock()
			return
		}
		done = true
		list := funcs
		funcs = nil
		mu.Unlock()
		for _, fn := range list {
			fn.Release()
		}
	}
	track := func(fn js.Func) js.Func {
		mu.Lock()
		if done {
			mu.Unlock()
			fn.Release()
			return fn
		}
		funcs = append(funcs, fn)
		mu.Unlock()
		return fn
	}

	success := track(js.FuncOf(func(_ js.Value, args []js.Value) any {
		release()
		var status int
		var data []byte
		if len(args) > 0 && args[0].Truthy() {
			res := args[0]
			status = jsStatusCode(res)
			data = jsBytes(res.Get("data"))
		}
		req.OnSuccess(status, data)
		return nil
	}))
	fail := track(js.FuncOf(func(_ js.Value, args []js.Value) any {
		release()
		msg := "request failed"
		if len(args) > 0 && args[0].Truthy() {
			if m := args[0].Get("errMsg"); m.Type() == js.TypeString && m.String() != "" {
				msg = m.String()
			} else if m := args[0].Get("message"); m.Type() == js.TypeString && m.String() != "" {
				msg = m.String()
			}
		}
		req.OnFail(errors.New(msg))
		return nil
	}))
	opts["success"] = success
	opts["fail"] = fail

	var task js.Value
	callErr := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("devtools: mini program request threw: %v", r)
			}
		}()
		task = api.Call("request", opts)
		return nil
	}()
	if callErr != nil {
		release()
		return nil, callErr
	}

	mu.Lock()
	finished := done
	mu.Unlock()
	if !finished && req.EnableChunked && task.Truthy() {
		if task.Get("onHeadersReceived").Type() == js.TypeFunction {
			onHeaders := track(js.FuncOf(func(_ js.Value, args []js.Value) any {
				if len(args) > 0 && args[0].Truthy() {
					if code := jsStatusCode(args[0]); code > 0 {
						req.OnHeaders(code)
					}
				}
				return nil
			}))
			task.Call("onHeadersReceived", onHeaders)
		}
		if task.Get("onChunkReceived").Type() == js.TypeFunction {
			onChunk := track(js.FuncOf(func(_ js.Value, args []js.Value) any {
				if len(args) > 0 && args[0].Truthy() {
					req.OnChunk(jsBytes(args[0].Get("data")))
				}
				return nil
			}))
			task.Call("onChunkReceived", onChunk)
		}
	}

	if task.Truthy() && task.Get("abort").Type() == js.TypeFunction {
		abort = func() {
			defer func() { recover() }()
			task.Call("abort")
		}
	}
	return abort, nil
}

func jsStatusCode(v js.Value) int {
	if s := v.Get("statusCode"); s.Type() == js.TypeNumber {
		return s.Int()
	}
	if s := v.Get("status"); s.Type() == js.TypeNumber {
		return s.Int()
	}
	return 0
}

func jsBytes(v js.Value) []byte {
	if !v.Truthy() {
		return nil
	}
	if v.Type() == js.TypeString {
		return []byte(v.String())
	}
	u8 := js.Global().Get("Uint8Array")
	if !v.InstanceOf(u8) {
		if buf := v.Get("buffer"); buf.Truthy() {
			v = u8.New(buf, v.Get("byteOffset").Int(), v.Get("byteLength").Int())
		} else {
			v = u8.New(v)
		}
	}
	n := v.Get("byteLength").Int()
	if n <= 0 {
		return nil
	}
	out := make([]byte, n)
	js.CopyBytesToGo(out, v)
	return out
}

//go:build js

package js

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	sjs "syscall/js"
)

// IsolatedFrameAvailable reports whether this browser environment supports an
// opaque-origin sandboxed iframe with MessageChannel. Non-DOM JS hosts (such
// as mini programs) report false.
func IsolatedFrameAvailable() bool {
	g := sjs.Global()
	return g.Get("document").Truthy() && g.Get("MessageChannel").Truthy()
}

// IsolatedChannel owns an opaque-origin <iframe sandbox="allow-scripts"> and
// the private MessagePort connected to its bootstrap runtime. All inbound
// messages are converted to plain Go values (nil, bool, float64, int64, string,
// []byte, []any, map[string]any) before crossing into package tsx.
type IsolatedChannel struct {
	frame       sjs.Value
	port        sjs.Value
	handshakeFn sjs.Func
	onMessageFn sjs.Func
	handshakeOn bool

	mu     sync.Mutex
	closed bool
	ready  chan struct{}
}

// OpenIsolatedFrame appends a hidden opaque-origin <iframe sandbox="allow-scripts">
// running srcdoc, completes the one-time MessagePort transfer authenticated by
// initToken, and invokes onMessage for every subsequent port message.
func OpenIsolatedFrame(srcdoc, initToken string, onMessage func(map[string]any)) *IsolatedChannel {
	g := sjs.Global()
	doc := g.Get("document")
	ch := g.Get("MessageChannel").New()
	port1 := ch.Get("port1")
	port2 := ch.Get("port2")

	frame := doc.Call("createElement", "iframe")
	// Opaque origin: allow-scripts ONLY (never allow-same-origin).
	frame.Call("setAttribute", "sandbox", "allow-scripts")
	frame.Call("setAttribute", "referrerpolicy", "no-referrer")
	frame.Call("setAttribute", "aria-hidden", "true")
	frame.Call("setAttribute", "tabindex", "-1")
	frame.Get("style").Set("display", "none")

	c := &IsolatedChannel{
		frame:       frame,
		port:        port1,
		handshakeOn: true,
		ready:       make(chan struct{}),
	}

	var readyOnce sync.Once
	c.onMessageFn = sjs.FuncOf(func(_ sjs.Value, args []sjs.Value) any {
		if len(args) == 0 {
			return nil
		}
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return nil
		}
		c.mu.Unlock()
		data := args[0].Get("data")
		if data.Type() != sjs.TypeObject {
			return nil
		}
		if t := data.Get("type"); t.Type() == sjs.TypeString && t.String() == "ready" {
			readyOnce.Do(func() { close(c.ready) })
			return nil
		}
		if m, ok := decodePortValue(data).(map[string]any); ok {
			onMessage(m)
		}
		return nil
	})
	port1.Set("onmessage", c.onMessageFn)
	port1.Call("start")

	c.handshakeFn = sjs.FuncOf(func(_ sjs.Value, args []sjs.Value) any {
		if len(args) == 0 {
			return nil
		}
		ev := args[0]
		cw := frame.Get("contentWindow")
		if !cw.Truthy() || !ev.Get("source").Equal(cw) {
			return nil
		}
		data := ev.Get("data")
		if data.Type() != sjs.TypeObject {
			return nil
		}
		hello := data.Get("hello")
		tok := data.Get("token")
		if hello.Type() != sjs.TypeBoolean || !hello.Bool() || tok.Type() != sjs.TypeString || tok.String() != initToken {
			return nil
		}
		c.removeHandshake()
		transfer := g.Get("Array").New(port2)
		cw.Call("postMessage", map[string]any{"token": initToken}, "*", transfer)
		return nil
	})
	g.Call("addEventListener", "message", c.handshakeFn)

	frame.Set("srcdoc", srcdoc)
	doc.Get("documentElement").Call("appendChild", frame)
	return c
}

// Ready is closed once the isolated iframe's bootstrap connects its MessagePort.
func (c *IsolatedChannel) Ready() <-chan struct{} { return c.ready }

// Post sends msg to the isolated iframe over the private MessagePort.
func (c *IsolatedChannel) Post(msg map[string]any) {
	c.mu.Lock()
	closed := c.closed
	port := c.port
	c.mu.Unlock()
	if closed || !port.Truthy() {
		return
	}
	port.Call("postMessage", encodePortValue(msg))
}

// Close closes the MessagePort, removes the iframe from the DOM, and releases
// all JS callbacks.
func (c *IsolatedChannel) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	port := c.port
	frame := c.frame
	c.port = sjs.Undefined()
	c.frame = sjs.Undefined()
	c.mu.Unlock()

	c.removeHandshake()
	if port.Truthy() {
		port.Set("onmessage", sjs.Null())
		port.Call("close")
		c.onMessageFn.Release()
	}
	if frame.Truthy() {
		frame.Call("remove")
	}
}

func (c *IsolatedChannel) removeHandshake() {
	c.mu.Lock()
	on := c.handshakeOn
	c.handshakeOn = false
	c.mu.Unlock()
	if on {
		sjs.Global().Call("removeEventListener", "message", c.handshakeFn)
		c.handshakeFn.Release()
	}
}

func encodePortValue(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case bool, int, int64, float64, string, sjs.Value:
		return x
	case float32:
		return float64(x)
	case int32:
		return int(x)
	case uint64:
		return float64(x)
	case []byte:
		arr := sjs.Global().Get("Uint8Array").New(len(x))
		if len(x) > 0 {
			sjs.CopyBytesToJS(arr, x)
		}
		return arr
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = encodePortValue(item)
		}
		return out
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = encodePortValue(val)
		}
		return out
	default:
		raw, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprint(x)
		}
		var parsed any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return string(raw)
		}
		return encodePortValue(parsed)
	}
}

func decodePortValue(v sjs.Value) any {
	switch v.Type() {
	case sjs.TypeUndefined, sjs.TypeNull:
		return nil
	case sjs.TypeBoolean:
		return v.Bool()
	case sjs.TypeNumber:
		f := v.Float()
		if f == math.Trunc(f) && math.Abs(f) < 1<<53 && !(f == 0 && math.Signbit(f)) {
			return int64(f)
		}
		return f
	case sjs.TypeString:
		return v.String()
	case sjs.TypeObject:
		g := sjs.Global()
		if v.InstanceOf(g.Get("Uint8Array")) {
			n := v.Get("byteLength").Int()
			buf := make([]byte, n)
			if n > 0 {
				sjs.CopyBytesToGo(buf, v)
			}
			return bytes.Clone(buf)
		}
		if v.InstanceOf(g.Get("ArrayBuffer")) {
			u8 := g.Get("Uint8Array").New(v)
			n := u8.Get("byteLength").Int()
			buf := make([]byte, n)
			if n > 0 {
				sjs.CopyBytesToGo(buf, u8)
			}
			return bytes.Clone(buf)
		}
		if g.Get("Array").Call("isArray", v).Bool() {
			n := v.Length()
			out := make([]any, n)
			for i := range n {
				out[i] = decodePortValue(v.Index(i))
			}
			return out
		}
		keys := g.Get("Object").Call("keys", v)
		n := keys.Length()
		out := make(map[string]any, n)
		for i := range n {
			k := keys.Index(i).String()
			if k == "__proto__" || k == "constructor" || k == "prototype" {
				continue
			}
			out[k] = decodePortValue(v.Get(k))
		}
		return out
	default:
		return nil
	}
}

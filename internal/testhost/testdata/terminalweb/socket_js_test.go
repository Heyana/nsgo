//go:build js

package terminalweb

import (
	"context"
	"errors"
	"io"
	"sync"
	"syscall/js"
)

// socket is the app's byte-stream adapter. A WebSocket message boundary has no meaning to a terminal; reads may split or join messages. Resize/control messages are the app/server's protocol, independent of terminal.Options.Conn.
type socket struct {
	ws        js.Value
	mu        sync.Mutex
	queue     [][]byte
	err       error
	wake      chan struct{}
	done      chan struct{}
	once      sync.Once
	listeners []js.Func
}

func dialSocket(ctx context.Context, url string) (*socket, error) {
	s := &socket{ws: js.Global().Get("WebSocket").New(url), wake: make(chan struct{}, 1), done: make(chan struct{})}
	s.ws.Set("binaryType", "arraybuffer")
	ready := make(chan error, 1)
	bind := func(name string, fn func([]js.Value)) {
		callback := js.FuncOf(func(_ js.Value, args []js.Value) any { fn(args); return nil })
		s.listeners = append(s.listeners, callback)
		s.ws.Set(name, callback)
	}
	bind("onopen", func([]js.Value) {
		select {
		case ready <- nil:
		default:
		}
	})
	bind("onmessage", func(args []js.Value) {
		data := args[0].Get("data")
		var bytes []byte
		if data.Type() == js.TypeString {
			bytes = []byte(data.String())
		} else {
			array := js.Global().Get("Uint8Array").New(data)
			bytes = make([]byte, array.Length())
			js.CopyBytesToGo(bytes, array)
		}
		s.mu.Lock()
		if s.err == nil && len(bytes) > 0 {
			s.queue = append(s.queue, bytes)
		}
		s.mu.Unlock()
		select {
		case s.wake <- struct{}{}:
		default:
		}
	})
	bind("onerror", func([]js.Value) {
		s.end(errors.New("WebSocket failed"))
		select {
		case ready <- errors.New("WebSocket failed"):
		default:
		}
	})
	bind("onclose", func([]js.Value) {
		s.end(io.EOF)
		select {
		case ready <- io.EOF:
		default:
		}
	})
	select {
	case err := <-ready:
		if err != nil {
			s.Close()
			return nil, err
		}
		return s, nil
	case <-ctx.Done():
		s.Close()
		return nil, ctx.Err()
	}
}

func (s *socket) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		s.mu.Lock()
		if len(s.queue) > 0 {
			n := copy(p, s.queue[0])
			s.queue[0] = s.queue[0][n:]
			if len(s.queue[0]) == 0 {
				s.queue = s.queue[1:]
			}
			s.mu.Unlock()
			return n, nil
		}
		err := s.err
		s.mu.Unlock()
		if err != nil {
			return 0, err
		}
		select {
		case <-s.wake:
		case <-s.done:
		}
	}
}

func (s *socket) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return 0, s.err
	}
	if s.ws.Get("readyState").Int() != 1 {
		return 0, io.ErrClosedPipe
	}
	array := js.Global().Get("Uint8Array").New(len(p))
	js.CopyBytesToJS(array, p)
	s.ws.Call("send", array)
	return len(p), nil
}

func (s *socket) end(err error) {
	s.once.Do(func() { s.mu.Lock(); s.err = err; s.mu.Unlock(); close(s.done) })
}

func (s *socket) Close() error {
	s.end(io.ErrClosedPipe)
	for _, name := range []string{"onopen", "onmessage", "onerror", "onclose"} {
		s.ws.Set(name, js.Null())
	}
	s.ws.Call("close")
	for _, callback := range s.listeners {
		callback.Release()
	}
	s.listeners = nil
	return nil
}

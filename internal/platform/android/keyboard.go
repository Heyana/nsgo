//go:build android || nsgoandroidhost

package android

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// consumeKey queues a hardware key down that Java waits on before reading another message, so the release that follows cannot overtake it, and answers whether the focused view consumed it with extKeyReply once the engine has run OnKeyDown. Waiting for the answer off the pipe's reader leaves it free for the native text requests the engine makes while it handles the key.
func (s *surface) consumeKey(target *engine.Engine, from, request uint32, ev driver.Event) {
	answer := target.DispatchKeyDownAsync(ev)
	go func() {
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case consumed := <-answer:
			s.answerKeyDown(from, request, consumed)
		case <-timer.C:
			// Java's watchdog has abandoned the native action and released the keys by now; the engine's buffered answer needs no waiter if it comes later.
		}
	}()
}

// answerKeyDown sends extKeyReply. A runtime without extended messages never asks, as it sends no request.
func (s *surface) answerKeyDown(surface, request uint32, consumed bool) {
	_ = s.sendExtended(extKeyReply, func(w *writer) {
		w.u32(surface)
		w.u32(request)
		w.flags(consumed)
	})
}

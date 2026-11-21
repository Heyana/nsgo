package android

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// The outer text protocol messages carry an operation and an independent
// request ID. Prepared-text and layout IDs still name their native objects.
const (
	textPrepare byte = iota + 1
	textMeasure
	textLayoutOp
	textLines
	textCarets
)

const textTimeout = time.Second

var (
	errTextTimeout = errors.New("text request timed out")
	errTextClosed  = errors.New("text pipe closed")
)

func textOperation(kind byte) string {
	switch kind {
	case textPrepare:
		return "prepare"
	case textMeasure:
		return "measure"
	case textLayoutOp:
		return "layout"
	case textLines:
		return "lines"
	case textCarets:
		return "carets"
	default:
		return fmt.Sprintf("operation %d", kind)
	}
}

type textReply struct {
	data []byte
	err  error
}

type textCall struct {
	id       uint64
	kind     byte
	payload  []byte
	deadline time.Time
	reply    chan textReply
}

// textRPC has one text writer per pipe. A write queued behind another message
// cannot keep the engine waiting past its deadline, and neither a late nor
// a duplicate reply can fill another request's reply slot.
type textRPC struct {
	ids     atomic.Uint64
	mu      sync.Mutex
	pending map[uint64]*textCall
	queue   chan *textCall
	closed  chan struct{}
}

func newTextRPC(write func([]byte, time.Time) error) *textRPC {
	rpc := &textRPC{
		pending: map[uint64]*textCall{},
		queue:   make(chan *textCall, 1),
		closed:  make(chan struct{}),
	}
	go rpc.write(write)
	return rpc
}

func (rpc *textRPC) write(write func([]byte, time.Time) error) {
	for {
		select {
		case <-rpc.closed:
			return
		case call := <-rpc.queue:
			if time.Now().Before(call.deadline) {
				if err := write(call.payload, call.deadline); err != nil {
					rpc.complete(call.id, call.kind, textReply{err: err})
				}
			}
		}
	}
}

func (rpc *textRPC) call(kind byte, timeout time.Duration, body func(*writer)) (uint64, textReply) {
	id := rpc.ids.Add(1)
	w := writer{}
	w.u64(id)
	w.byte(kind)
	body(&w)
	call := &textCall{id: id, kind: kind, payload: w.buf, deadline: time.Now().Add(timeout), reply: make(chan textReply, 1)}
	rpc.mu.Lock()
	rpc.pending[id] = call
	rpc.mu.Unlock()
	defer func() {
		rpc.mu.Lock()
		delete(rpc.pending, id)
		rpc.mu.Unlock()
	}()
	timer := time.NewTimer(time.Until(call.deadline))
	defer timer.Stop()
	select {
	case <-rpc.closed:
		return id, textReply{err: errTextClosed}
	case <-timer.C:
		return id, textReply{err: errTextTimeout}
	case rpc.queue <- call:
	}
	select {
	case <-rpc.closed:
		return id, textReply{err: errTextClosed}
	case <-timer.C:
		return id, textReply{err: errTextTimeout}
	case reply := <-call.reply:
		return id, reply
	}
}

// complete claims an exact pending request once. The pipe reader never
// waits for the engine goroutine to take a reply.
func (rpc *textRPC) complete(id uint64, kind byte, reply textReply) bool {
	rpc.mu.Lock()
	call, ok := rpc.pending[id]
	ok = ok && time.Now().Before(call.deadline)
	if ok {
		delete(rpc.pending, id)
	}
	rpc.mu.Unlock()
	if !ok {
		return false
	}
	if kind != call.kind {
		reply = textReply{err: fmt.Errorf("received a %s reply for %s", textOperation(kind), textOperation(call.kind))}
	}
	call.reply <- reply
	return true
}

func (rpc *textRPC) close() { close(rpc.closed) }

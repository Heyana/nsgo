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
	data      []byte
	err       error
	inline    [16]byte
	inlineLen uint8
}

func (r *textReply) bytes() []byte {
	if r.data != nil {
		return r.data
	}
	return r.inline[:r.inlineLen]
}

type textCall struct {
	kind     byte
	deadline time.Time
	reply    chan textReply
}

type textWork struct {
	id       uint64
	kind     byte
	buf      *[]byte
	deadline time.Time
}

// textRPC has one text writer per pipe. A write queued behind another message
// cannot keep the engine waiting past its deadline, and neither a late nor
// a duplicate reply can fill another request's reply slot.
type textRPC struct {
	ids       atomic.Uint64
	mu        sync.Mutex
	pending   map[uint64]*textCall
	queue     chan textWork
	closed    chan struct{}
	bufPool   sync.Pool
	callPool  sync.Pool
	timerPool sync.Pool
}

func newTextRPC(write func([]byte, time.Time) error) *textRPC {
	rpc := &textRPC{
		pending: map[uint64]*textCall{},
		queue:   make(chan textWork, 1),
		closed:  make(chan struct{}),
	}
	rpc.bufPool.New = func() any {
		b := make([]byte, 0, 256)
		return &b
	}
	rpc.callPool.New = func() any {
		return &textCall{reply: make(chan textReply, 1)}
	}
	go rpc.write(write)
	return rpc
}

func (rpc *textRPC) putBuf(bp *[]byte) {
	if cap(*bp) <= 64<<10 {
		*bp = (*bp)[:0]
		rpc.bufPool.Put(bp)
	}
}

func (rpc *textRPC) write(write func([]byte, time.Time) error) {
	for {
		select {
		case <-rpc.closed:
			return
		case work := <-rpc.queue:
			if time.Now().Before(work.deadline) {
				if err := write(*work.buf, work.deadline); err != nil {
					rpc.complete(work.id, work.kind, textReply{err: err})
				}
			}
			rpc.putBuf(work.buf)
		}
	}
}

func (rpc *textRPC) call(kind byte, timeout time.Duration, body func(*writer)) (uint64, textReply) {
	id := rpc.ids.Add(1)
	bp := rpc.bufPool.Get().(*[]byte)
	w := writer{(*bp)[:0]}
	w.u64(id)
	w.byte(kind)
	body(&w)
	*bp = w.buf
	deadline := time.Now().Add(timeout)
	call := rpc.callPool.Get().(*textCall)
	call.kind = kind
	call.deadline = deadline
	rpc.mu.Lock()
	rpc.pending[id] = call
	rpc.mu.Unlock()
	var timer *time.Timer
	if v := rpc.timerPool.Get(); v != nil {
		timer = v.(*time.Timer)
		timer.Reset(timeout)
	} else {
		timer = time.NewTimer(timeout)
	}
	completed := false
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		rpc.timerPool.Put(timer)
		if !completed {
			rpc.mu.Lock()
			delete(rpc.pending, id)
			rpc.mu.Unlock()
		}
	}()
	work := textWork{id: id, kind: kind, buf: bp, deadline: deadline}
	select {
	case <-rpc.closed:
		rpc.putBuf(bp)
		return id, textReply{err: errTextClosed}
	case <-timer.C:
		rpc.putBuf(bp)
		return id, textReply{err: errTextTimeout}
	case rpc.queue <- work:
	}
	select {
	case <-rpc.closed:
		return id, textReply{err: errTextClosed}
	case <-timer.C:
		return id, textReply{err: errTextTimeout}
	case reply := <-call.reply:
		completed = true
		// complete() already removed id from pending before sending on call.reply,
		// and rpc.write never holds a pointer to call, so call is exclusively owned here.
		rpc.callPool.Put(call)
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

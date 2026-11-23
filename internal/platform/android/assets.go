//go:build android || nsgoandroidhost

package android

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sync"
)

// errAssetsUnsupported is readAsset's error for a runtime built before extended messages, which cannot read bundled assets for Go.
var errAssetsUnsupported = fmt.Errorf("android: this runtime does not support reading bundled assets: %w", errors.ErrUnsupported)

// A reply carries either a chunk or the final result of the read.
const (
	assetChunk = iota
	assetDone
	assetMissing
	assetFailed
)

type assetReads struct {
	mu      sync.Mutex
	next    uint64
	pending map[uint64]*assetRead
	closed  bool
}

type assetRead struct {
	done   chan struct{}
	chunks [][]byte
	size   int
	err    error
}

// readAsset puts the APK read on Java's worker. The pipe reader collects
// its chunks without copying them. The caller joins them off the engine
// and pipe goroutines once the end arrives.
func (s *surface) readAsset(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a := &s.assets
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil, io.ErrClosedPipe
	}
	if a.pending == nil {
		a.pending = make(map[uint64]*assetRead)
	}
	a.next++
	id := a.next
	r := &assetRead{done: make(chan struct{})}
	a.pending[id] = r
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.pending, id)
		a.mu.Unlock()
	}()

	if err := s.sendExtended(extAssetRead, func(w *writer) {
		w.u64(id)
		w.str(path)
	}); errors.Is(err, errNoExtendedMessages) {
		return nil, errAssetsUnsupported
	} else if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.done:
		if r.err != nil {
			return nil, r.err
		}
		data := make([]byte, r.size)
		offset := 0
		for _, chunk := range r.chunks {
			offset += copy(data[offset:], chunk)
		}
		return data, nil
	}
}

func (a *assetReads) reply(reply *reader) {
	id, status := reply.u64(), reply.byte()
	a.mu.Lock()
	defer a.mu.Unlock()
	r := a.pending[id]
	if r == nil {
		return
	}
	if status == assetChunk {
		r.chunks = append(r.chunks, reply.buf)
		r.size += len(reply.buf)
		return
	}
	switch status {
	case assetDone:
	case assetMissing:
		r.err = fs.ErrNotExist
	case assetFailed:
		r.err = errors.New(reply.str())
	default:
		r.err = errors.New("android: invalid asset reply")
	}
	delete(a.pending, id)
	close(r.done)
}

func (a *assetReads) close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	for id, r := range a.pending {
		delete(a.pending, id)
		r.err = io.ErrClosedPipe
		close(r.done)
	}
}

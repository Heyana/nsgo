package android

import (
	"encoding/binary"
	"errors"
	"sync"
)

const (
	bulkChunk  = 32 << 10
	bulkHeader = 9 // original kind, total size, offset
)

// bulkWriter keeps one transfer in progress. A text write takes the
// exclusive lock, so once it waits, another fragment cannot take the pipe
// before it. Ordinary messages still use the pipe's own frame lock.
type bulkWriter struct {
	transfer sync.Mutex
	priority sync.RWMutex
}

func (b *bulkWriter) write(kind byte, head, body []byte, emit func([]byte) error) error {
	b.transfer.Lock()
	defer b.transfer.Unlock()
	total := len(head) + len(body)
	buf := make([]byte, bulkHeader+bulkChunk)
	buf[0] = kind
	binary.BigEndian.PutUint32(buf[1:], uint32(total))
	for off := 0; off < total; {
		binary.BigEndian.PutUint32(buf[5:], uint32(off))
		limit := min(bulkChunk, total-off)
		dst := buf[bulkHeader : bulkHeader+limit]
		n := copy(dst, head)
		head = head[n:]
		n = copy(dst[n:], body)
		body = body[n:]
		b.priority.RLock()
		err := emit(buf[:bulkHeader+limit])
		b.priority.RUnlock()
		if err != nil {
			return err
		}
		off += limit
	}
	return nil
}

// bulkReader assembles exactly one transfer. The pipe loop can dispatch
// other frames immediately while it waits for the rest of the payload.
type bulkReader struct {
	accept func(byte) bool
	kind   byte
	data   []byte
	offset int
}

var errBulkFrame = errors.New("invalid bulk pipe fragment")

func (b *bulkReader) add(fragment []byte) (byte, []byte, error) {
	if len(fragment) <= bulkHeader || len(fragment) > bulkHeader+bulkChunk {
		return 0, nil, errBulkFrame
	}
	kind := fragment[0]
	size := int(binary.BigEndian.Uint32(fragment[1:]))
	offset := int(binary.BigEndian.Uint32(fragment[5:]))
	part := fragment[bulkHeader:]
	// A Java byte array's length is a positive signed int.
	if !b.accept(kind) || size <= bulkChunk || size > 1<<31-1 || offset < 0 || offset > size-len(part) {
		return 0, nil, errBulkFrame
	}
	if b.data == nil {
		if offset != 0 {
			return 0, nil, errBulkFrame
		}
		b.kind, b.data = kind, make([]byte, size)
	}
	if kind != b.kind || size != len(b.data) || offset != b.offset {
		return 0, nil, errBulkFrame
	}
	b.offset += copy(b.data[offset:], part)
	if b.offset < size {
		return 0, nil, nil
	}
	data := b.data
	b.data, b.offset = nil, 0
	return kind, data, nil
}

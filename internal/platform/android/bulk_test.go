package android

import (
	"bytes"
	"encoding/binary"
	"errors"
	"runtime"
	"testing"
	"time"
)

// Hold the first fragment in the writer while text queues. The remaining
// fragments and the reply complete over that same writer.
func TestBulkWriteLetsTextPass(t *testing.T) {
	const image, text = 6, 26
	var wire bulkWriter
	var got [][]byte
	first, resume, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	head := []byte("image metadata")
	body := bytes.Repeat([]byte{0x42}, 2*bulkChunk+23)
	go func() {
		finished <- wire.write(image, head, body, func(part []byte) error {
			got = append(got, bytes.Clone(part))
			if len(got) == 1 {
				close(first)
				<-resume
			}
			return nil
		})
	}()
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("first fragment did not start")
	}
	go func() {
		wire.priority.Lock()
		got = append(got, []byte{text})
		wire.priority.Unlock()
	}()
	deadline := time.Now().Add(time.Second)
	for wire.priority.TryRLock() {
		wire.priority.RUnlock()
		if time.Now().After(deadline) {
			close(resume)
			t.Fatal("text writer did not queue")
		}
		runtime.Gosched()
	}
	close(resume)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("bulk writer did not finish")
	}
	if len(got) != 4 || !bytes.Equal(got[1], []byte{text}) {
		t.Fatalf("text did not pass between fragments: %d wire frames", len(got))
	}
	reader := bulkReader{accept: func(kind byte) bool { return kind == image }}
	want := append(bytes.Clone(head), body...)
	for i, chunk := range [][]byte{got[0], got[2], got[3]} {
		kind, data, err := reader.add(chunk)
		if err != nil {
			t.Fatalf("fragment %d: %v", i, err)
		}
		if i < 2 && data != nil {
			t.Fatal("incomplete image left the reader")
		}
		if i == 2 && (kind != image || !bytes.Equal(data, want)) {
			t.Fatal("reassembled image changed")
		}
	}
}

func TestBulkReaderRefusesNoncontiguousParts(t *testing.T) {
	const file = 13
	var chunks [][]byte
	var wire bulkWriter
	_ = wire.write(file, nil, make([]byte, bulkChunk+5), func(chunk []byte) error {
		chunks = append(chunks, bytes.Clone(chunk))
		return nil
	})
	for _, tt := range []struct {
		name string
		edit func([]byte)
	}{
		{"duplicate", func(b []byte) { binary.BigEndian.PutUint32(b[5:], 0) }},
		{"skipped byte", func(b []byte) { binary.BigEndian.PutUint32(b[5:], bulkChunk+1) }},
		{"size changed", func(b []byte) { binary.BigEndian.PutUint32(b[1:], bulkChunk+6) }},
		{"kind changed", func(b []byte) { b[0]++ }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := bulkReader{accept: func(kind byte) bool { return kind == file }}
			if _, _, err := r.add(chunks[0]); err != nil {
				t.Fatal(err)
			}
			part := bytes.Clone(chunks[1])
			tt.edit(part)
			if _, _, err := r.add(part); !errors.Is(err, errBulkFrame) {
				t.Fatalf("invalid continuation gave error %v", err)
			}
		})
	}
}

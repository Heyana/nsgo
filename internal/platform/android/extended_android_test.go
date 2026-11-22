//go:build android

package android

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"testing"
)

// TestSendExtendedWritesItsKindFirst checks the frame sendExtended writes: kind msgExtended, then the payload's length counting the sub-kind byte, the sub-kind and the body.
func TestSendExtendedWritesItsKindFirst(t *testing.T) {
	out, err := os.CreateTemp(t.TempDir(), "pipe")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	s := surface{out: out}
	if err := s.sendExtended(7, func(w *writer) { w.u32(42) }); err != nil {
		t.Fatal(err)
	}
	if _, err := out.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	kind, payload, err := readMessage(bufio.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{7, 0, 0, 0, 42}; kind != msgExtended || !bytes.Equal(payload, want) {
		t.Errorf("sendExtended wrote kind %d payload %v, want %d %v", kind, payload, msgExtended, want)
	}
}

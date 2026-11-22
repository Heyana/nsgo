//go:build android

package android

import (
	"bufio"
	"bytes"
	"errors"
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
	s.extendedMessages.Store(true)
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

// TestSendExtendedWaitsForTheRuntime checks that sendExtended writes nothing to a runtime whose environment has not said it reads msgExtended messages, as one built before them, which would refuse the message and end the pipe, and that the environment's capability bit lets them through.
func TestSendExtendedWaitsForTheRuntime(t *testing.T) {
	out, err := os.CreateTemp(t.TempDir(), "pipe")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	s := surface{out: out}
	wrote := false
	if err := s.sendExtended(7, func(w *writer) { wrote = true }); !errors.Is(err, errNoExtendedMessages) {
		t.Fatalf("sendExtended to a runtime without the capability: %v", err)
	}
	if info, err := out.Stat(); err != nil || info.Size() != 0 || wrote {
		t.Fatalf("sendExtended to a runtime without the capability wrote %v bytes (body ran: %v, %v)", info.Size(), wrote, err)
	}
	s.extendedMessages.Store(true)
	if err := s.sendExtended(7, func(w *writer) {}); err != nil {
		t.Fatalf("sendExtended to a runtime with the capability: %v", err)
	}
	if info, err := out.Stat(); err != nil || info.Size() != 6 {
		t.Fatalf("sendExtended to a runtime with the capability wrote %v bytes, want 6 (%v)", info.Size(), err)
	}
}

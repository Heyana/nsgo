package android

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBoxAndLookFastPathsOnJVM(t *testing.T) {
	javac, java := androidJDK(t)
	classes := t.TempDir()
	args := []string{
		"-d", classes,
		"java/dev/oneo/nsgo/Protocol.java",
		"java/dev/oneo/nsgo/Animator.java",
		"java/dev/oneo/nsgo/Shape.java",
		"java/dev/oneo/nsgo/Look.java",
		"java/dev/oneo/nsgo/Box.java",
	}
	if err := filepath.WalkDir("testdata/box", func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".java") {
			args = append(args, path)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.CommandContext(t.Context(), javac, args...).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-cp", classes, "dev.oneo.nsgo.BoxPerfCheck").CombinedOutput(); err != nil {
		t.Fatalf("java BoxPerfCheck: %v\n%s", err, out)
	}
}

func TestTextRPCPoolingAndInlineReplies(t *testing.T) {
	var rpc *textRPC
	rpc = newTextRPC(func(payload []byte, _ time.Time) error {
		r := reader{payload}
		id := r.u64()
		kind := r.byte()
		var reply textReply
		switch kind {
		case textPrepare:
			reply.inlineLen = 0
		case textMeasure:
			reply.inlineLen = 8
			binary.BigEndian.PutUint32(reply.inline[0:], 0x42c80000) // 100.0
			binary.BigEndian.PutUint32(reply.inline[4:], 0x41a00000) // 20.0
		case textLayoutOp:
			reply.inlineLen = 4
		case textLines:
			reply.data = make([]byte, 44)
			binary.BigEndian.PutUint32(reply.data[0:], 1)
		}
		rpc.complete(id, kind, reply)
		return nil
	})
	defer rpc.close()

	for i := range 200 {
		_, prep := rpc.call(textPrepare, time.Second, func(w *writer) {
			w.u32(uint32(i + 1))
			w.str("$123.45")
		})
		if prep.err != nil || len(prep.bytes()) != 0 {
			t.Fatalf("prepare %d: %+v", i, prep)
		}
		_, meas := rpc.call(textMeasure, time.Second, func(w *writer) {
			w.u32(uint32(i + 1))
			w.float(100)
			w.u32(0)
		})
		if meas.err != nil || len(meas.bytes()) != 8 {
			t.Fatalf("measure %d: %+v", i, meas)
		}
		_, lay := rpc.call(textLayoutOp, time.Second, func(w *writer) {
			w.u32(uint32(i + 1))
			w.u32(uint32(i + 1000))
			w.float(100)
			w.u32(0)
		})
		if lay.err != nil || len(lay.bytes()) != 4 {
			t.Fatalf("layout %d: %+v", i, lay)
		}
		_, lines := rpc.call(textLines, time.Second, func(w *writer) {
			w.u32(uint32(i + 1000))
			w.u32(0)
		})
		if lines.err != nil || len(lines.bytes()) != 44 {
			t.Fatalf("lines %d: %+v", i, lines)
		}
	}
}

func BenchmarkTextRPCCall(b *testing.B) {
	var rpc *textRPC
	rpc = newTextRPC(func(payload []byte, _ time.Time) error {
		r := reader{payload}
		id := r.u64()
		kind := r.byte()
		var reply textReply
		reply.inlineLen = 8
		rpc.complete(id, kind, reply)
		return nil
	})
	defer rpc.close()

	body := func(w *writer) {
		w.u32(42)
		w.float(120)
		w.u32(0)
	}
	b.ReportAllocs()
	for b.Loop() {
		_, reply := rpc.call(textMeasure, time.Second, body)
		if reply.err != nil || len(reply.bytes()) != 8 {
			b.Fatal(reply.err)
		}
	}
}

func BenchmarkReadFramePayload(b *testing.B) {
	var frame bytes.Buffer
	payload := make([]byte, 18)
	for range 256 {
		frame.Write(payload)
	}
	raw := frame.Bytes()

	b.Run("reusedScratch", func(b *testing.B) {
		var scratch []byte
		br := bufio.NewReader(bytes.NewReader(raw))
		b.ReportAllocs()
		for b.Loop() {
			p, err := readFramePayload(br, 18, &scratch, true)
			if err != nil {
				br.Reset(bytes.NewReader(raw))
				p, err = readFramePayload(br, 18, &scratch, true)
				if err != nil {
					b.Fatal(err)
				}
			}
			if len(p) != 18 {
				b.Fatal(len(p))
			}
		}
	})

	b.Run("freshAlloc", func(b *testing.B) {
		br := bufio.NewReader(bytes.NewReader(raw))
		b.ReportAllocs()
		for b.Loop() {
			p, err := readFramePayload(br, 18, nil, false)
			if err != nil {
				br.Reset(bytes.NewReader(raw))
				p, err = readFramePayload(br, 18, nil, false)
				if err != nil {
					b.Fatal(err)
				}
			}
			if len(p) != 18 {
				b.Fatal(len(p))
			}
		}
	})
}

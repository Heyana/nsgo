package main

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestZsyncControlVector(t *testing.T) {
	got, err := zsyncControl([]byte("hello\n"), "Hello World.AppImage", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	header, checksums, ok := bytes.Cut(got, []byte("\n\n"))
	want := "zsync: 0.6.2\nFilename: Hello World.AppImage\nMTime: Thu, 01 Jan 1970 00:00:00 +0000\nBlocksize: 2048\nLength: 6\nHash-Lengths: 1,4,16\nURL: Hello%20World.AppImage\nSHA-1: f572d396fae9206628714fb2ce00f72e94f2258f"
	if !ok || string(header) != want {
		t.Fatalf("control header = %q, want %q", header, want)
	}
	// The weak sum follows the upstream format; the padded block's MD4 was independently checked with OpenSSL's legacy provider.
	if got := hex.EncodeToString(checksums); got != "021eeb91b7abf2919564796696cfd71280dd3155" {
		t.Errorf("padded block record = %s", got)
	}
}

func TestZsyncControlBlocksAndNames(t *testing.T) {
	got, err := zsyncControl(bytes.Repeat([]byte{0xab}, 4097), "hello.AppImage", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	_, checksums, _ := bytes.Cut(got, []byte("\n\n"))
	if len(checksums) != 3*20 || !bytes.Equal(checksums[:20], checksums[20:40]) || bytes.Equal(checksums[:20], checksums[40:]) {
		t.Fatalf("three block records = %x", checksums)
	}
	for _, name := range []string{"", "../image", "image\nURL: other", "image\x00", "folder\\image"} {
		if _, err := zsyncControl([]byte("data"), name, time.Time{}); err == nil {
			t.Errorf("accepted control filename %q", name)
		}
	}
	if !strings.Contains(string(got), "Length: 4097\n") {
		t.Error("control length included last-block padding")
	}
}

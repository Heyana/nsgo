package vt

import "testing"

func TestMouseSyncKeepsMotionDedupUntilModesOrGeometryChange(t *testing.T) {
	loadLib(t)
	term, err := NewTerminal(80, 24, Effects{})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Free()
	encoder, err := NewMouseEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Free()
	term.Write([]byte("\x1b[?1003h\x1b[?1006h"))
	send := func(width int) string {
		encoder.Sync(term, width, 480, 10, 20, 0, 0, false)
		return string(encoder.Encode(MouseMotion, ButtonNone, 0, 25, 45))
	}
	if got := send(800); got != "\x1b[<35;3;3M" {
		t.Fatalf("first motion=%q", got)
	}
	if got := send(800); got != "" {
		t.Fatalf("unchanged context repeated motion=%q", got)
	}
	if got := send(900); got != "\x1b[<35;3;3M" {
		t.Fatalf("new geometry retained stale dedup=%q", got)
	}
	term.Write([]byte("\x1b[?1003l\x1b[?1000h"))
	if got := send(900); got != "" {
		t.Fatalf("normal mode reported hover=%q", got)
	}
	term.Write([]byte("\x1b[?1000l\x1b[?1003h"))
	if got := send(900); got != "\x1b[<35;3;3M" {
		t.Fatalf("new tracking mode retained stale dedup=%q", got)
	}
}

func TestMouseSyncReselectsAnAlreadyEnabledFormat(t *testing.T) {
	loadLib(t)
	term, err := NewTerminal(80, 24, Effects{})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Free()
	encoder, err := NewMouseEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Free()
	send := func() string {
		encoder.Sync(term, 800, 480, 10, 20, 0, 0, false)
		return string(encoder.Encode(MousePress, ButtonLeft, 0, 25, 45))
	}
	term.Write([]byte("\x1b[?1000h\x1b[?1005h\x1b[?1006h"))
	if got := send(); got != "\x1b[<0;3;3M" {
		t.Fatalf("SGR selected=%q", got)
	}
	// The flags stay set, but the last selected format changes.
	term.Write([]byte("\x1b[?1005h"))
	if got := send(); got != "\x1b[M ##" {
		t.Fatalf("reselected UTF-8=%q", got)
	}
	term.Write([]byte("\x1b[?1006h"))
	if got := send(); got != "\x1b[<0;3;3M" {
		t.Fatalf("reselected SGR=%q", got)
	}
}

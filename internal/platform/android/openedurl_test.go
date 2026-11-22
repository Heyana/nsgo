package android

import "testing"

func TestOpenedURLEnvironmentTail(t *testing.T) {
	for _, display := range []bool{false, true} {
		var w writer
		writeBaseEnvironment(&w, "en-US")
		w.byte(1)
		for range 4 {
			w.str("")
		}
		w.byte(capExtendedMessages)
		w.flags(display)
		if display {
			w.u32(0)
			w.floats(480, 800, 60, 120)
		}
		legacy := reader{buf: w.buf}
		if got := readEnvironment(&legacy); got.OpenedURL != "" || len(legacy.buf) != 0 {
			t.Fatalf("legacy display=%v: OpenedURL=%q, tail=%d", display, got.OpenedURL, len(legacy.buf))
		}
		const invocation = "https://example.com/orders/%E4%B8%AD?table=12&x=a%20b#detail"
		w.str(invocation)
		r := reader{buf: w.buf}
		got := readEnvironment(&r)
		if got.OpenedURL != invocation || len(r.buf) != 0 {
			t.Fatalf("display=%v: OpenedURL=%q, tail=%d", display, got.OpenedURL, len(r.buf))
		}
		if display && got.DisplayID != "0" {
			t.Fatal("invocation tail corrupted the display")
		}
	}
}

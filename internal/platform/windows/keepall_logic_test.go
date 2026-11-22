package windows

import (
	"slices"
	"testing"
	"unicode/utf16"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestEncodeKeepAllText(t *testing.T) {
	text := "안녕 세계"
	ranges := driver.KeepAllWordRanges(text)
	units, offsets, orig := encodeKeepAllText(text, ranges)
	want := utf16.Encode([]rune("안\u2060녕 세\u2060계"))
	if !slices.Equal(units, want) {
		t.Fatalf("units = %x, want %x", units, want)
	}
	if len(offsets) != len(units)+1 || offsets[len(units)] != len(text) {
		t.Fatalf("offsets len = %d, want %d", len(offsets), len(units)+1)
	}
	// Original UTF-16 indices round-trip: 안(0), WJ(1), 녕(1), ' '(2), 세(3), WJ(4), 계(4), end(5).
	wantOrig := []int{0, 1, 1, 2, 3, 4, 4, 5}
	if !slices.Equal(orig, wantOrig) {
		t.Fatalf("origUTF16 = %v, want %v", orig, wantOrig)
	}
	// Non-Korean text produces no word joiners and nil origUTF16.
	plainUnits, plainOffsets, plainOrig := encodeKeepAllText("hello world", nil)
	if plainOrig != nil || len(plainOffsets) != len(plainUnits)+1 {
		t.Fatalf("plain text had origUTF16=%v offsets=%d", plainOrig, len(plainOffsets))
	}
}

func TestEncodeKeepAllUTF16RoundTrip(t *testing.T) {
	text := "안녕하세요 반갑😀"
	units, toLayout, fromLayout := encodeKeepAllUTF16(text)
	raw := utf16.Encode([]rune(text))
	if len(toLayout) != len(raw)+1 {
		t.Fatalf("toLayout len = %d, want %d", len(toLayout), len(raw)+1)
	}
	if len(fromLayout) != len(units)+1 {
		t.Fatalf("fromLayout len = %d, want %d", len(fromLayout), len(units)+1)
	}
	for disp := uint32(0); int(disp) <= len(raw); disp++ {
		if got := fromLayout[toLayout[disp]]; got != disp {
			t.Errorf("fromLayout[toLayout[%d]] = %d, want %d", disp, got, disp)
		}
	}
	// Non-CJK text returns nil maps.
	_, toNil, fromNil := encodeKeepAllUTF16("hello world")
	if toNil != nil || fromNil != nil {
		t.Errorf("expected nil maps for ASCII, got %v / %v", toNil, fromNil)
	}
}

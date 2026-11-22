package ios

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestShouldPasteSelf(t *testing.T) {
	for _, c := range []struct {
		name string
		cfg  driver.InputConfig
		want bool
	}{
		{
			name: "multiline default without maxLength delegates to UIKit",
			cfg:  driver.InputConfig{Multiline: true},
			want: false,
		},
		{
			name: "multiline with maxLength pastes itself",
			cfg:  driver.InputConfig{Multiline: true, MaxLength: 10},
			want: true,
		},
		{
			name: "multiline submit without maxLength pastes itself",
			cfg:  driver.InputConfig{Multiline: true, SubmitBehavior: driver.SubmitBehaviorSubmit},
			want: true,
		},
		{
			name: "single-line without maxLength pastes itself",
			cfg:  driver.InputConfig{Multiline: false},
			want: true,
		},
		{
			name: "single-line with maxLength pastes itself",
			cfg:  driver.InputConfig{Multiline: false, MaxLength: 10},
			want: true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := shouldPasteSelf(c.cfg); got != c.want {
				t.Errorf("shouldPasteSelf(%+v) = %v, want %v", c.cfg, got, c.want)
			}
		})
	}
}

func TestPreparePasteText(t *testing.T) {
	for _, c := range []struct {
		name      string
		text      string
		multiline bool
		current   string
		sel       driver.TextRange
		maxLength int
		clip      func(string, int) string
		wantOut   string
		wantRepl  int
		wantOK    bool
	}{
		{
			name:      "clips past maxLength at end",
			text:      "ABCDEFGHIJ",
			current:   "12345",
			sel:       driver.TextRange{Start: 5, End: 5},
			maxLength: 10,
			wantOut:   "ABCDE",
			wantRepl:  0,
			wantOK:    true,
		},
		{
			name:      "refuses paste when full",
			text:      "KLM",
			current:   "12345ABCDE",
			sel:       driver.TextRange{Start: 10, End: 10},
			maxLength: 10,
			wantOut:   "",
			wantRepl:  0,
			wantOK:    false,
		},
		{
			name:      "replaces selection adding room",
			text:      "ABCDEFGHIJ",
			current:   "12345678",
			sel:       driver.TextRange{Start: 2, End: 6},
			maxLength: 10,
			wantOut:   "ABCDEF",
			wantRepl:  4,
			wantOK:    true,
		},
		{
			name:      "keeps emoji surrogate pair whole",
			text:      "ABCDEFGHI😀KLM",
			current:   "",
			sel:       driver.TextRange{},
			maxLength: 10,
			wantOut:   "ABCDEFGHI",
			wantRepl:  0,
			wantOK:    true,
		},
		{
			name:      "emoji alone does not fit in one UTF-16 unit",
			text:      "😀KLM",
			current:   "123456789",
			sel:       driver.TextRange{Start: 9, End: 9},
			maxLength: 10,
			wantOut:   "",
			wantRepl:  0,
			wantOK:    false,
		},
		{
			name:      "single-line normalizes CRLF before clipping",
			text:      "AB\r\nCD\nEFG",
			multiline: false,
			current:   "12345",
			sel:       driver.TextRange{Start: 5, End: 5},
			maxLength: 10,
			wantOut:   "AB CD",
			wantRepl:  0,
			wantOK:    true,
		},
		{
			name:      "multiline preserves newlines when clipping",
			text:      "AB\nCD\nEFG",
			multiline: true,
			current:   "12345",
			sel:       driver.TextRange{Start: 5, End: 5},
			maxLength: 10,
			wantOut:   "AB\nCD",
			wantRepl:  0,
			wantOK:    true,
		},
		{
			name:      "existing text past maxLength leaves zero room",
			text:      "XYZ",
			current:   "123456789012345",
			sel:       driver.TextRange{Start: 0, End: 2},
			maxLength: 10,
			wantOut:   "",
			wantRepl:  2,
			wantOK:    false,
		},
		{
			name:      "custom composed sequence clipper returning empty refuses",
			text:      "e\u0301x",
			current:   "123456789",
			sel:       driver.TextRange{Start: 9, End: 9},
			maxLength: 10,
			clip:      func(string, int) string { return "" },
			wantOut:   "",
			wantRepl:  0,
			wantOK:    false,
		},
		{
			name:      "empty paste returns false",
			text:      "",
			current:   "123",
			sel:       driver.TextRange{Start: 3, End: 3},
			maxLength: 10,
			wantOut:   "",
			wantRepl:  0,
			wantOK:    false,
		},
		{
			name:      "no maxLength keeps normalized single-line text",
			text:      "hello\nworld",
			multiline: false,
			current:   "a",
			sel:       driver.TextRange{Start: 1, End: 1},
			maxLength: 0,
			wantOut:   "hello world",
			wantRepl:  0,
			wantOK:    true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			gotOut, gotRepl, gotOK := preparePasteText(c.text, c.multiline, c.current, c.sel, c.maxLength, c.clip)
			if gotOut != c.wantOut || gotRepl != c.wantRepl || gotOK != c.wantOK {
				t.Errorf("preparePasteText() = (%q, %d, %v), want (%q, %d, %v)",
					gotOut, gotRepl, gotOK, c.wantOut, c.wantRepl, c.wantOK)
			}
		})
	}
}

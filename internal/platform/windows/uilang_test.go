package windows

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestWindowsUILanguageCandidates(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"en-XA", nil},
		{"ar-XB", nil},
		{"EN_xa", nil},
		{"ar_xb", nil},
		{"de", []string{"de"}},
		{"de-DE", []string{"de-DE", "de"}},
		{"zh_Hans", []string{"zh-Hans", "zh"}},
		{"zh-Hans-CN", []string{"zh-Hans-CN", "zh-Hans", "zh"}},
		{"en-US-u-hc-h23", []string{"en-US", "en"}},
		{"ja-JP-x-custom", []string{"ja-JP", "ja"}},
		{"de--DE", nil},
		{"-de", nil},
		{"de-", nil},
		{"de$DE", nil},
		{strings.Repeat("a", 85), nil},
	} {
		got := windowsUILanguageCandidates(tc.in)
		if !slices.Equal(got, tc.want) {
			t.Errorf("windowsUILanguageCandidates(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestUTF16MultiString(t *testing.T) {
	if got := utf16MultiString(); got != nil {
		t.Fatalf("empty tags = %v, want nil", got)
	}
	if got := utf16MultiString("", ""); got != nil {
		t.Fatalf("blank tags = %v, want nil", got)
	}
	got := utf16MultiString("zh-Hans")
	want := append(utf16.Encode([]rune("zh-Hans")), 0, 0)
	if !slices.Equal(got, want) {
		t.Fatalf("utf16MultiString(zh-Hans) = %v, want %v", got, want)
	}
	got2 := utf16MultiString("de-DE", "de")
	var want2 []uint16
	want2 = append(want2, utf16.Encode([]rune("de-DE"))...)
	want2 = append(want2, 0)
	want2 = append(want2, utf16.Encode([]rune("de"))...)
	want2 = append(want2, 0, 0)
	if !slices.Equal(got2, want2) {
		t.Fatalf("utf16MultiString(de-DE, de) = %v, want %v", got2, want2)
	}
}

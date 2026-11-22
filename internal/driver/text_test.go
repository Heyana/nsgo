package driver

import (
	"slices"
	"testing"
)

func TestTextBackgroundVerticalBounds(t *testing.T) {
	for _, tc := range []struct {
		name        string
		top, bottom Length
		wantTop     Points
		wantBottom  Points
	}{
		{name: "full", wantBottom: 20},
		{name: "mixed", top: Pct(60), bottom: Pt(2), wantTop: 12, wantBottom: 18},
		{name: "negative", top: Pt(-2), bottom: Pct(-10), wantBottom: 20},
		{name: "overlap", top: Pct(60), bottom: Pct(40), wantTop: 12, wantBottom: 12},
		{name: "outside", top: Pt(25), bottom: Pct(110), wantTop: 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bg := TextBackground{Top: tc.top, Bottom: tc.bottom}
			top, bottom := bg.VerticalBounds(20)
			if top != tc.wantTop || bottom != tc.wantBottom {
				t.Errorf("bounds = (%v, %v), want (%v, %v)", top, bottom, tc.wantTop, tc.wantBottom)
			}
		})
	}
}

func TestKeepAllLanguageAndWords(t *testing.T) {
	for _, tc := range []struct {
		lang string
		keep bool
		want string
	}{
		{"", false, ""},
		{"en-US", false, "en-US"},
		{"ja", false, "ja"},
		{"zh-Hans", false, "zh-Hans"},
		{"kok", false, "kok"},
		{"ko", true, "ko-u-lw-keepall"},
		{"KO", true, "KO-u-lw-keepall"},
		{"ko-KR", true, "ko-KR-u-lw-keepall"},
		{"ko_KR", true, "ko-KR-u-lw-keepall"},
		{"ko-Kore-KR", true, "ko-Kore-KR-u-lw-keepall"},
		{"ko-u-ca-gregory", true, "ko-u-ca-gregory-lw-keepall"},
		{"ko-u-lw-normal", true, "ko-u-lw-normal"},
		{"ko-x-custom", true, "ko-u-lw-keepall-x-custom"},
		{"ko-KR-t-en-x-custom", true, "ko-KR-t-en-u-lw-keepall-x-custom"},
	} {
		if got := KeepAllWords(tc.lang); got != tc.keep {
			t.Errorf("KeepAllWords(%q) = %v, want %v", tc.lang, got, tc.keep)
		}
		if got := KeepAllLanguage(tc.lang); got != tc.want {
			t.Errorf("KeepAllLanguage(%q) = %q, want %q", tc.lang, got, tc.want)
		}
	}
}

func TestKeepAllWordRanges(t *testing.T) {
	words := func(s string) []string {
		var out []string
		for _, r := range KeepAllWordRanges(s) {
			out = append(out, s[r[0]:r[1]])
		}
		return out
	}
	for _, tc := range []struct {
		text string
		want []string
	}{
		{"hello world", nil},
		{"가", nil},
		{"안녕하세요 반갑습니다 세계입니다", []string{"안녕하세요", "반갑습니다", "세계입니다"}},
		{"NSGo에서 100원입니다", []string{"NSGo에서", "100원입니다"}},
		{"안녕, 세계!", []string{"안녕", "세계"}},
		{"한글-테스트", []string{"한글", "테스트"}},
		{"한국어" + string(AttachmentRune) + "테스트", []string{"한국어", "테스트"}},
	} {
		if got := words(tc.text); !slices.Equal(got, tc.want) {
			t.Errorf("KeepAllWordRanges(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

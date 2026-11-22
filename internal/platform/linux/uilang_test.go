package linux

import (
	"slices"
	"testing"
)

func TestGNULanguageList(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{"en-XA", ""},
		{"ar-XB", ""},
		{"EN_xa", ""},
		{"ar_xb", ""},
		{"de", "de"},
		{"fr", "fr"},
		{"ja", "ja"},
		{"en", "en:C"},
		{"en-US", "en_US:en:C"},
		{"en-GB", "en_GB:en:C"},
		{"pt-BR", "pt_BR:pt"},
		{"de-DE-u-hc-h23", "de_DE:de"},
		{"es-419", "es"},
		{"zh", "zh_CN:zh"},
		{"zh-CN", "zh_CN:zh"},
		{"zh-TW", "zh_TW:zh_HK:zh"},
		{"zh-HK", "zh_HK:zh_TW:zh"},
		{"zh-MO", "zh_MO:zh_HK:zh_TW:zh"},
		{"zh-Hans", "zh_CN:zh"},
		{"zh_Hans", "zh_CN:zh"},
		{"zh-Hans-CN", "zh_CN:zh"},
		{"zh-Hans-SG", "zh_SG:zh_CN:zh"},
		{"zh-Hant", "zh_TW:zh_HK:zh"},
		{"zh-Hant-TW", "zh_TW:zh_HK:zh"},
		{"zh-Hant-HK", "zh_HK:zh_TW:zh"},
		{"zh-Hant-MO", "zh_MO:zh_HK:zh_TW:zh"},
		{"sr-Latn", "sr@latin:sr"},
		{"sr-Latn-RS", "sr_RS@latin:sr@latin:sr"},
		{"sr-Cyrl", "sr"},
		{"uz-Cyrl", "uz@cyrillic:uz"},
		{"uz-Cyrl-UZ", "uz_UZ@cyrillic:uz@cyrillic:uz"},
		{"ca-ES-valencia", "ca_ES@valencia:ca@valencia:ca"},
		{"-de", ""},
		{"de-", ""},
		{"de--DE", ""},
		{"123", ""},
		{"de:fr", ""},
	} {
		if got := gnuLanguageList(tc.in); got != tc.want {
			t.Errorf("gnuLanguageList(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLCMessagesCandidates(t *testing.T) {
	if got := lcMessagesCandidates("zh-Hans"); !slices.Equal(got, []string{"zh_CN.UTF-8", "zh_CN.utf8", "C.UTF-8", "C.utf8", "en_US.UTF-8"}) {
		t.Errorf("zh-Hans candidates = %v", got)
	}
	if got := lcMessagesCandidates("de"); !slices.Equal(got, []string{"C.UTF-8", "C.utf8", "en_US.UTF-8"}) {
		t.Errorf("de candidates = %v", got)
	}
	if got := lcMessagesCandidates("sr-Latn-RS"); !slices.Equal(got, []string{"sr_RS.UTF-8@latin", "sr_RS.utf8@latin", "C.UTF-8", "C.utf8", "en_US.UTF-8"}) {
		t.Errorf("sr-Latn-RS candidates = %v", got)
	}
}

func TestIsPOSIXLocale(t *testing.T) {
	for _, loc := range []string{"", "C", "POSIX"} {
		if !isPOSIXLocale(loc) {
			t.Errorf("isPOSIXLocale(%q) = false, want true", loc)
		}
	}
	for _, loc := range []string{"C.UTF-8", "en_US.UTF-8", "de_DE.UTF-8"} {
		if isPOSIXLocale(loc) {
			t.Errorf("isPOSIXLocale(%q) = true, want false", loc)
		}
	}
}

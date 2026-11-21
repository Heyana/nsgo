package emoji

import (
	"slices"
	"testing"
)

func TestCatalogCompleteness(t *testing.T) {
	base := Base()
	if len(base) < 1900 {
		t.Fatalf("Base() has %d emojis, want at least 1900", len(base))
	}
	counts := map[Category]int{}
	skinCount := 0
	for _, e := range base {
		if e.Emoji == "" {
			t.Fatalf("entry %d has empty Emoji", e.Index)
		}
		if e.Category > Flags {
			t.Fatalf("entry %d (%q) has invalid category %d", e.Index, e.Emoji, e.Category)
		}
		counts[e.Category]++
		if len(e.Skins) > 0 {
			if len(e.Skins) < 5 {
				t.Errorf("entry %d (%q) has %d skin variants, want >= 5", e.Index, e.Emoji, len(e.Skins))
			}
			skinCount++
		}
	}
	for c := Smileys; c <= Flags; c++ {
		if counts[c] == 0 {
			t.Errorf("category %d has no emojis", c)
		}
	}
	if skinCount < 300 {
		t.Errorf("skin-tone emoji count = %d, want >= 300", skinCount)
	}
	locs := SupportedLocales()
	for _, want := range []string{"en", "en-gb", "zh", "zh-hant", "ja", "ko", "de", "fr", "es", "es-mx"} {
		if !slices.Contains(locs, want) {
			t.Errorf("SupportedLocales() missing %q", want)
		}
	}
}

func TestResolveLocale(t *testing.T) {
	cases := map[string]string{
		"en":        "en",
		"en-US":     "en",
		"en_GB":     "en-gb",
		"en-XA":     "en",
		"ar-XB":     "en",
		"zh":        "zh",
		"zh-Hans":   "zh",
		"zh-CN":     "zh",
		"zh-SG":     "zh",
		"zh-Hant":   "zh-hant",
		"zh-TW":     "zh-hant",
		"zh-HK":     "zh-hant",
		"ja-JP":     "ja",
		"ko-KR":     "ko",
		"de-AT":     "de",
		"fr-CA":     "fr",
		"es-MX":     "es-mx",
		"es-419":    "es",
		"pt-BR":     "pt",
		"no":        "nb",
		"nb-NO":     "nb",
		"zz-Nobody": "",
	}
	for in, want := range cases {
		if got := ResolveLocale(in); got != want {
			t.Errorf("ResolveLocale(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMultiLanguageSearch(t *testing.T) {
	has := func(items []Item, want string) bool {
		for _, it := range items {
			if it.Emoji == want {
				return true
			}
		}
		return false
	}

	// English name, multi-word, and shortcode search.
	fireEN := Search("fire", []string{"en"})
	if len(fireEN) == 0 || fireEN[0].Emoji != "🔥" {
		t.Fatalf("Search(fire, en) first = %+v, want 🔥", fireEN[:min(len(fireEN), 3)])
	}
	if !has(Search(":thumbsup:", []string{"en"}), "👍") {
		t.Error("shortcode :thumbsup: did not match 👍")
	}
	if !has(Search("red heart", []string{"en"}), "❤️") {
		t.Error("multi-word 'red heart' did not match ❤️")
	}
	if !has(Search("grinning_face", []string{"en"}), "😀") {
		t.Error("underscore 'grinning_face' did not match 😀")
	}

	// Simplified Chinese search + English fallback.
	fireZH := Search("火", []string{"zh-Hans"})
	if len(fireZH) == 0 || fireZH[0].Emoji != "🔥" {
		t.Fatalf("Search(火, zh-Hans) first = %+v, want 🔥", fireZH[:min(len(fireZH), 3)])
	}
	if fireZH[0].Name == "" || fireZH[0].Name == "fire" {
		t.Errorf("zh-Hans name for 🔥 = %q, want localized Chinese name", fireZH[0].Name)
	}
	if !has(Search("fire", []string{"zh-Hans"}), "🔥") {
		t.Error("English fallback failed under zh-Hans")
	}

	// Traditional Chinese search.
	if !has(Search("火", []string{"zh-TW"}), "🔥") {
		t.Error("zh-TW search for 火 did not match 🔥")
	}

	// Japanese, Korean, German, French, Spanish searches.
	if !has(Search("炎", []string{"ja-JP"}), "🔥") {
		t.Error("ja-JP search for 炎 did not match 🔥")
	}
	if !has(Search("불", []string{"ko-KR"}), "🔥") {
		t.Error("ko-KR search for 불 did not match 🔥")
	}
	if !has(Search("feuer", []string{"de-DE"}), "🔥") {
		t.Error("de-DE search for feuer did not match 🔥")
	}
	if !has(Search("feu", []string{"fr-FR"}), "🔥") {
		t.Error("fr-FR search for feu did not match 🔥")
	}
	if !has(Search("fuego", []string{"es-ES"}), "🔥") {
		t.Error("es-ES search for fuego did not match 🔥")
	}

	// Direct emoji character query.
	direct := Search("🔥", []string{"en"})
	if len(direct) != 1 || direct[0].Emoji != "🔥" {
		t.Fatalf("direct search for 🔥 returned %+v", direct)
	}

	// Non-matching query returns empty slice.
	if got := Search("zzzznotanemoji123", []string{"en", "zh-Hans"}); len(got) != 0 {
		t.Errorf("non-matching query returned %d items", len(got))
	}
}

func TestByStringAndChildLocales(t *testing.T) {
	item, ok := ByString("👋🏽", []string{"en"})
	if !ok || item.Emoji != "👋🏽" || item.Name != "waving hand" {
		t.Fatalf("ByString(👋🏽) = %+v, %v", item, ok)
	}
	// Heart without VS16 resolves to ❤️.
	heart, ok := ByString("❤", []string{"en"})
	if !ok || heart.Emoji != "❤️" {
		t.Fatalf("ByString(❤) = %+v, %v", heart, ok)
	}
	// Child locale diff (en-gb inherits en and overrides British spellings).
	gb := LocaleTable("en-gb")
	if len(gb) != len(Base()) {
		t.Fatalf("en-gb length = %d, want %d", len(gb), len(Base()))
	}
}

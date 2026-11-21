package ui

import (
	"testing"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestEmojiPickerSearchClearAndSelect(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var picked []base.Emoji
		var ref base.EmojiPickerRef
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			return EmojiPicker{
				TestID:   "emoji",
				Ref:      &ref,
				OnSelect: func(em base.Emoji) { picked = append(picked, em) },
			}
		})

		if !app.Exists("role:textbox") {
			t.Fatal("styled emoji picker missing search field")
		}
		if !app.Exists("role:tablist") {
			t.Fatal("styled emoji picker missing category tabs")
		}
		if !app.Exists("role:radiogroup") {
			t.Fatal("styled emoji picker missing skin tone selector")
		}
		// Preview footer shows the initially highlighted emoji's name.
		if !app.Exists("text:grinning face") {
			t.Fatal("preview footer missing initially highlighted emoji name")
		}

		// Search, verify clear button appears, select via Enter.
		app.Type("role:textbox", "rocket")
		if app.Exists("text:grinning face") {
			t.Fatal("preview retained the previous result after searching for rocket")
		}
		if !app.Exists("label:Clear") {
			t.Fatal("clear button did not appear when search query is non-empty")
		}
		app.Key("Enter")
		if len(picked) != 1 || picked[0].Emoji != "🚀" {
			t.Fatalf("picked = %+v, want 🚀", picked)
		}

		// Pressing clear resets query back to the full catalog.
		app.Press("label:Clear")
		if ref.MatchedCount() < 1900 {
			t.Fatalf("MatchedCount after Clear = %d, want >= 1900", ref.MatchedCount())
		}
	})
}

func TestEmojiPickerCategoriesSkinToneAndLocales(t *testing.T) {
	i18n.SetApp(i18n.NewCatalog("en", "zh-Hans"))
	defer i18n.SetApp(nil)

	var picked []base.Emoji
	var cats []base.EmojiCategory
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return EmojiPicker{
			TestID:           "emoji",
			Recent:           func() []string { return []string{"🔥"} },
			OnSelect:         func(em base.Emoji) { picked = append(picked, em) },
			OnCategoryChange: func(c base.EmojiCategory) { cats = append(cats, c) },
		}
	}, nsgotest.Size(400, 440), nsgotest.Environment(core.Environment{Locale: "zh-CN"}))

	// Chinese UI strings routed through base/locales.
	if !app.Exists("role:tab 最近使用") || !app.Exists("role:tab 动物与自然") {
		t.Fatalf("expected localized Chinese category tabs\n%s", app.Dump())
	}

	// Switch category to Animals & Nature.
	app.Press("role:tab 动物与自然")
	if len(cats) == 0 || cats[len(cats)-1] != base.EmojiAnimals {
		t.Fatalf("OnCategoryChange = %v, want EmojiAnimals", cats)
	}

	// Choose Dark skin tone and search for waving hand in Chinese.
	app.Press("role:radio 深肤色")
	app.Type("role:textbox", "挥手")
	app.Key("Enter")
	if len(picked) != 1 || picked[0].Emoji != "👋🏿" {
		t.Fatalf("picked with dark skin tone = %+v, want 👋🏿", picked)
	}

	// Non-matching query displays Chinese empty text from base/locales.
	app.Type("role:textbox", "zzzznotanemoji")
	if !app.Exists("text:没有匹配的表情") {
		t.Fatal("missing Chinese empty message '没有匹配的表情'")
	}
}

func TestEmojiPickerMinimalOptions(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return EmojiPicker{
			TestID:       "emoji",
			NoSearch:     true,
			NoCategories: true,
			NoSkinTone:   true,
			NoPreview:    true,
			Style:        core.Style{Width: core.Pt(260), Height: core.Pt(220)},
		}
	}, nsgotest.Size(320, 320))

	if app.Exists("role:textbox") || app.Exists("role:tablist") || app.Exists("role:radiogroup") {
		t.Fatal("disabled parts should not render when NoSearch/NoCategories/NoSkinTone are set")
	}
	v := app.Find("id:emoji")
	if v.Frame.Size.Width != 260 || v.Frame.Size.Height != 220 {
		t.Fatalf("custom size = %+v, want 260x220", v.Frame.Size)
	}
}

package base_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/base/locales"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/internal/testhost"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"go.yaml.in/yaml/v3"
)

func TestEmojiSearchAndLookup(t *testing.T) {
	// English name, multi-word, and shortcode search.
	fire := base.SearchEmojis("fire", "en")
	if len(fire) == 0 || fire[0].Emoji != "🔥" || fire[0].Name != "fire" {
		t.Fatalf("SearchEmojis(fire, en) first = %+v", fire[:min(len(fire), 3)])
	}
	// Multi-language search: Chinese, Japanese, Korean, German, French, Spanish.
	for _, tc := range []struct {
		query, lang, want string
	}{
		{"火", "zh-Hans", "🔥"},
		{"火", "zh-TW", "🔥"},
		{"炎", "ja-JP", "🔥"},
		{"불", "ko-KR", "🔥"},
		{"feuer", "de-DE", "🔥"},
		{"feu", "fr-FR", "🔥"},
		{"fuego", "es-ES", "🔥"},
		{":thumbsup:", "en", "👍"},
		{"🔥", "en", "🔥"},
	} {
		got := base.SearchEmojis(tc.query, tc.lang)
		found := false
		for _, e := range got {
			if e.Emoji == tc.want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("SearchEmojis(%q, %q) did not include %s", tc.query, tc.lang, tc.want)
		}
	}

	// Category lookup covers all 9 standard Unicode categories.
	total := 0
	for _, cat := range base.EmojiCategories() {
		items := base.EmojisInCategory(cat, "en")
		if len(items) == 0 {
			t.Errorf("EmojisInCategory(%v) is empty", cat)
		}
		total += len(items)
	}
	if total < 1900 {
		t.Errorf("total emoji across categories = %d, want >= 1900", total)
	}

	// Skin tone application and EmojiByString lookup.
	wave, ok := base.EmojiByString("👋", "en")
	if !ok || !wave.HasSkinTones {
		t.Fatalf("EmojiByString(👋) = %+v, %v", wave, ok)
	}
	if got := wave.WithSkinTone(base.SkinToneMedium).Emoji; got != "👋🏽" {
		t.Errorf("WithSkinTone(Medium) = %q, want 👋🏽", got)
	}
	toned, ok := base.EmojiByString("👋🏿", "zh-Hans")
	if !ok || toned.Emoji != "👋🏿" || toned.Name == "" || toned.Name == "waving hand" {
		t.Fatalf("EmojiByString(👋🏿, zh-Hans) = %+v, %v", toned, ok)
	}
}

func TestEmojiSkinTonePairsAndReset(t *testing.T) {
	e, ok := base.EmojiByString("🧑‍🤝‍🧑", "en")
	if !ok {
		t.Fatal("people holding hands missing")
	}
	if got := e.WithSkinTone(base.SkinToneMedium).Emoji; got != "🧑🏽‍🤝‍🧑🏽" {
		t.Errorf("medium tone = %q, want both people medium", got)
	}
	toned, ok := base.EmojiByString("🧑🏻‍🤝‍🧑🏿", "en")
	if !ok {
		t.Fatal("mixed-tone people holding hands missing")
	}
	if got := toned.WithSkinTone(base.SkinToneDefault).Emoji; got != e.Emoji {
		t.Errorf("default tone = %q, want neutral %q", got, e.Emoji)
	}
}

func TestEmojiLookupMetadataIsIndependent(t *testing.T) {
	e, ok := base.EmojiByString("👋", "en")
	if !ok || len(e.Keywords) == 0 || len(e.Skins) == 0 {
		t.Fatal("waving hand metadata missing")
	}
	keyword, skin := e.Keywords[0], e.Skins[0]
	defer func() { e.Keywords[0], e.Skins[0] = keyword, skin }()
	e.Keywords[0], e.Skins[0] = "changed", "changed"
	next, _ := base.EmojiByString("👋", "en")
	if next.Keywords[0] != keyword || next.Skins[0] != skin {
		t.Fatal("mutating public metadata corrupted later catalog lookups")
	}
}

func TestEmojiPickerCategoryPropsMoveTheGrid(t *testing.T) {
	for _, controlled := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "controlled"}[controlled], func(t *testing.T) {
			var category *core.Signal[base.EmojiCategory]
			var picked []base.Emoji
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				category = core.NewSignal(cx, base.EmojiFood)
				p := base.EmojiPicker{
					DefaultCategory: base.EmojiFood,
					OnSelect:        func(e base.Emoji) { picked = append(picked, e) },
					Style:           core.Style{Width: core.Pt(320), Height: core.Pt(360)},
				}
				if controlled {
					p.Category = category.Get
				}
				return p
			}, nsgotest.Size(400, 400))
			first := base.EmojisInCategory(base.EmojiFood, "en")[0]
			if !app.Exists("label:" + first.Name) {
				t.Fatal("initial category was not scrolled into the virtual grid")
			}
			app.Focus("role:textbox")
			app.Key("enter")
			if len(picked) != 1 || picked[0].Category != base.EmojiFood {
				t.Fatalf("initial category selected %+v, want Food", picked)
			}
			if controlled {
				category.Set(base.EmojiAnimals)
				app.Settle()
				app.Key("enter")
				if len(picked) != 2 || picked[1].Category != base.EmojiAnimals {
					t.Fatalf("changed category selected %+v, want Animals", picked)
				}
				app.Type("role:textbox", "rocket")
				app.Key("escape")
				app.Key("enter")
				if len(picked) != 3 || picked[2].Category != base.EmojiAnimals {
					t.Fatalf("cleared query lost the controlled category: %+v", picked)
				}
			}
		})
	}
}

func TestEmojiPickerControlledCategoryDoesNotResetScrolling(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		category := core.NewSignal(cx, base.EmojiSmileys)
		return base.EmojiPicker{
			TestID: "picker", Category: category.Get, OnCategoryChange: category.Set,
			Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
			RowStyle: core.Style{Height: core.Pt(36)}, SectionStyle: core.Style{Height: core.Pt(36)},
		}
	}, nsgotest.Size(400, 400))
	scroll := app.Find("id:picker").Children[2]
	app.ScrollTo(fmt.Sprintf("view:%d", scroll.ID), core.Point{Y: 1801})
	app.Settle()
	if got := scroll.Offset.Y; got != 1801 {
		t.Fatalf("adopting the scrolled category jumped from 1801 to %v", got)
	}
}

func TestEmojiPickerHighlightFollowsReplacementResults(t *testing.T) {
	var preview *base.Emoji
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return base.EmojiPicker{
			Style:             core.Style{Width: core.Pt(320), Height: core.Pt(360)},
			OnHighlightChange: func(e *base.Emoji) { preview = e },
		}
	}, nsgotest.Size(400, 400))
	app.Type("role:textbox", "rocket")
	if preview == nil || preview.Emoji != "🚀" {
		t.Fatalf("preview after search = %+v, want rocket", preview)
	}
}

func TestEmojiPickerPreservesDirectEmojiSearch(t *testing.T) {
	var picked base.Emoji
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return base.EmojiPicker{
			DefaultSkinTone: base.SkinToneMedium,
			Style:           core.Style{Width: core.Pt(320), Height: core.Pt(360)},
			OnSelect:        func(e base.Emoji) { picked = e },
		}
	}, nsgotest.Size(400, 400))
	app.Type("role:textbox", "🧑🏻‍🤝‍🧑🏿")
	app.Key("enter")
	if picked.Emoji != "🧑🏻‍🤝‍🧑🏿" {
		t.Fatalf("direct emoji query selected %q instead of its exact sequence", picked.Emoji)
	}
}

func TestEmojiPickerQueryRefFollowsTheMountedPicker(t *testing.T) {
	var ref base.EmojiPickerRef
	var shown *core.Signal[bool]
	var changes []string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		shown = core.NewSignal(cx, true)
		return core.Show{When: shown.Get, Then: func(*core.Scope) core.Node {
			return base.EmojiPicker{
				Ref: &ref, DefaultQuery: "fire",
				OnQueryChange: func(q string) { changes = append(changes, q) },
				Style:         core.Style{Width: core.Pt(320), Height: core.Pt(360)},
			}
		}}
	}, nsgotest.Size(400, 400))
	if ref.Query() != "fire" {
		t.Fatalf("initial query = %q", ref.Query())
	}
	ref.SetQuery("rocket")
	app.Settle()
	if ref.Query() != "rocket" || len(changes) != 1 || changes[0] != "rocket" {
		t.Fatalf("requested query = %q, changes = %v", ref.Query(), changes)
	}
	shown.Set(false)
	app.Settle()
	ref.SetQuery("fire")
	if ref.Query() != "" || ref.MatchedCount() != 0 || len(changes) != 1 {
		t.Fatal("ref still addresses an unmounted picker")
	}
}

func TestEmojiPickerCategoryAndToneArrows(t *testing.T) {
	var category base.EmojiCategory
	var tone base.EmojiSkinTone
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return base.EmojiPicker{
			Style:            core.Style{Width: core.Pt(320), Height: core.Pt(360)},
			OnCategoryChange: func(c base.EmojiCategory) { category = c },
			OnSkinToneChange: func(v base.EmojiSkinTone) { tone = v },
		}
	}, nsgotest.Size(400, 400), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	app.Focus("role:tab Smileys & Emotion")
	app.Key("right")
	if category != base.EmojiPeople {
		t.Errorf("right selected category %v, want People", category)
	}
	app.Focus("role:radio Default skin tone")
	app.Key("right")
	if tone != base.SkinToneLight {
		t.Errorf("right selected tone %v, want Light", tone)
	}
}

// TestEmojiPickerLocalesCompleteness verifies that every locale catalog in
// base/locales/ defines the exact same key set and that every category, skin
// tone, and picker message is translated in English, Simplified Chinese, and
// transforms under the pseudo locale en-XA.
func TestEmojiPickerLocalesCompleteness(t *testing.T) {
	testhost.RequireSource(t)

	readKeys := func(name string) map[string]bool {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join("locales", name))
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parse locales/%s: %v", name, err)
		}
		keys := make(map[string]bool, len(doc))
		for k := range doc {
			keys[k] = true
		}
		return keys
	}
	enKeys := readKeys("en.yaml")
	zhKeys := readKeys("zh-Hans.yaml")
	for k := range enKeys {
		if !zhKeys[k] {
			t.Errorf("base/locales/zh-Hans.yaml is missing key %q", k)
		}
	}
	for k := range zhKeys {
		if !enKeys[k] {
			t.Errorf("base/locales/zh-Hans.yaml has extra key %q not in en.yaml", k)
		}
	}

	// Verify runtime resolution in en, zh-Hans, and pseudo locale en-XA.
	en := locales.In("en")
	zh := locales.In("zh-Hans")
	xa := locales.In(i18n.PseudoAccents)
	checkPair := func(name, enVal, zhVal, xaVal string) {
		t.Helper()
		if enVal == "" || zhVal == "" || xaVal == "" {
			t.Errorf("%s has empty translation: en=%q zh=%q xa=%q", name, enVal, zhVal, xaVal)
		}
		if enVal == zhVal {
			t.Errorf("%s not translated in zh-Hans: %q", name, zhVal)
		}
		if xaVal == enVal {
			t.Errorf("%s not pseudolocalized in en-XA: %q", name, xaVal)
		}
	}
	checkPair("EmojiPicker", en.EmojiPicker(), zh.EmojiPicker(), xa.EmojiPicker())
	checkPair("EmojiSearch", en.EmojiSearch(), zh.EmojiSearch(), xa.EmojiSearch())
	checkPair("EmojiSearchPlaceholder", en.EmojiSearchPlaceholder(), zh.EmojiSearchPlaceholder(), xa.EmojiSearchPlaceholder())
	checkPair("EmojiNoMatches", en.EmojiNoMatches(), zh.EmojiNoMatches(), xa.EmojiNoMatches())
	checkPair("EmojiCategories", en.EmojiCategories(), zh.EmojiCategories(), xa.EmojiCategories())
	checkPair("EmojiSkinTone", en.EmojiSkinTone(), zh.EmojiSkinTone(), xa.EmojiSkinTone())
	checkPair("EmojiCategoryRecent", en.EmojiCategoryRecent(), zh.EmojiCategoryRecent(), xa.EmojiCategoryRecent())
	checkPair("EmojiCategorySmileys", en.EmojiCategorySmileys(), zh.EmojiCategorySmileys(), xa.EmojiCategorySmileys())
	checkPair("EmojiCategoryPeople", en.EmojiCategoryPeople(), zh.EmojiCategoryPeople(), xa.EmojiCategoryPeople())
	checkPair("EmojiCategoryAnimals", en.EmojiCategoryAnimals(), zh.EmojiCategoryAnimals(), xa.EmojiCategoryAnimals())
	checkPair("EmojiCategoryFood", en.EmojiCategoryFood(), zh.EmojiCategoryFood(), xa.EmojiCategoryFood())
	checkPair("EmojiCategoryTravel", en.EmojiCategoryTravel(), zh.EmojiCategoryTravel(), xa.EmojiCategoryTravel())
	checkPair("EmojiCategoryActivities", en.EmojiCategoryActivities(), zh.EmojiCategoryActivities(), xa.EmojiCategoryActivities())
	checkPair("EmojiCategoryObjects", en.EmojiCategoryObjects(), zh.EmojiCategoryObjects(), xa.EmojiCategoryObjects())
	checkPair("EmojiCategorySymbols", en.EmojiCategorySymbols(), zh.EmojiCategorySymbols(), xa.EmojiCategorySymbols())
	checkPair("EmojiCategoryFlags", en.EmojiCategoryFlags(), zh.EmojiCategoryFlags(), xa.EmojiCategoryFlags())

	// Also verify Category.Label and SkinTone.Label through a Scope.
	i18n.SetApp(i18n.NewCatalog("en", "zh-Hans"))
	defer i18n.SetApp(nil)
	nsgotest.Start(t, func(cx *core.Scope) core.Node {
		seenCat := map[string]bool{}
		for _, c := range append([]base.EmojiCategory{base.EmojiRecent}, base.EmojiCategories()...) {
			lbl := c.Label(cx)
			if lbl == "" || seenCat[lbl] {
				t.Errorf("category %v label = %q (empty or duplicate)", c, lbl)
			}
			seenCat[lbl] = true
		}
		seenTone := map[string]bool{}
		for _, tone := range base.EmojiSkinTones() {
			lbl := tone.Label(cx)
			if lbl == "" || seenTone[lbl] {
				t.Errorf("skin tone %v label = %q (empty or duplicate)", tone, lbl)
			}
			seenTone[lbl] = true
		}
		return core.View{}
	}, nsgotest.Environment(core.Environment{Locale: "zh-CN"}))
}

func TestEmojiPickerSearchAndPressSelection(t *testing.T) {
	var picked []base.Emoji
	var cancelled int
	var ref base.EmojiPickerRef
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return base.EmojiPicker{
			TestID:   "picker",
			Ref:      &ref,
			Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
			OnSelect: func(e base.Emoji) { picked = append(picked, e) },
			OnCancel: func() { cancelled++ },
		}
	}, nsgotest.Size(400, 400))

	if ref.MatchedCount() < 1900 {
		t.Fatalf("initial MatchedCount() = %d, want >= 1900", ref.MatchedCount())
	}

	// Search by English name and select via press.
	app.Type("role:textbox", "fire")
	if ref.MatchedCount() == 0 || ref.MatchedCount() > 100 {
		t.Fatalf("MatchedCount after 'fire' = %d", ref.MatchedCount())
	}
	app.Press("label:fire")
	if len(picked) != 1 || picked[0].Emoji != "🔥" || picked[0].Name != "fire" {
		t.Fatalf("picked after pressing fire = %+v", picked)
	}

	// Non-matching query shows empty message from base/locales.
	app.Type("role:textbox", "zzzznotanemoji")
	if ref.MatchedCount() != 0 {
		t.Fatalf("MatchedCount for nonsense = %d, want 0", ref.MatchedCount())
	}
	if !app.Exists("text:No matching emoji") {
		t.Fatal("missing localized empty state message")
	}

	// First Escape clears query; second Escape invokes OnCancel.
	app.Key("Escape")
	if ref.MatchedCount() < 1900 {
		t.Fatalf("Escape did not clear search query; MatchedCount = %d", ref.MatchedCount())
	}
	if cancelled != 0 {
		t.Fatalf("OnCancel fired while clearing non-empty query")
	}
	app.Key("Escape")
	if cancelled != 1 {
		t.Fatalf("OnCancel count = %d, want 1", cancelled)
	}
}

func TestEmojiPickerMultiLanguageAndDynamicLocale(t *testing.T) {
	i18n.SetApp(i18n.NewCatalog("en", "zh-Hans"))
	defer i18n.SetApp(nil)

	var picked []base.Emoji
	var rootScope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		rootScope = cx
		return base.EmojiPicker{
			TestID:   "picker",
			Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
			OnSelect: func(e base.Emoji) { picked = append(picked, e) },
		}
	}, nsgotest.Size(400, 400), nsgotest.Environment(core.Environment{Locale: "zh-CN"}))

	// Category labels and search label come from base/locales in zh-Hans.
	if !app.Exists("role:tab 食物与饮料") {
		t.Fatalf("expected Chinese category tab '食物与饮料'\n%s", app.Dump())
	}
	if !app.Exists("label:搜索表情") {
		t.Fatalf("expected Chinese search field label '搜索表情'")
	}

	// Search in Chinese and press Enter to pick top result 🔥.
	app.Type("role:textbox", "火")
	app.Key("Enter")
	if len(picked) != 1 || picked[0].Emoji != "🔥" || picked[0].Name != "火焰" {
		t.Fatalf("Chinese search picked = %+v, want 🔥 (火焰)", picked)
	}

	// English fallback still works in Chinese UI.
	app.Key("Escape")
	app.Type("role:textbox", "rocket")
	app.Key("Enter")
	if len(picked) != 2 || picked[1].Emoji != "🚀" {
		t.Fatalf("English fallback picked = %+v, want 🚀", picked)
	}

	// Switching locale dynamically to English updates category labels and names.
	rootScope.SetLocale("en-US")
	app.Settle()
	if !app.Exists("role:tab Food & Drink") {
		t.Fatalf("category tab did not switch to English after SetLocale")
	}

	// Explicit Languages prop searches Japanese even in English UI.
	var jaPicked []base.Emoji
	jaApp := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return base.EmojiPicker{
			Languages: []string{"ja-JP"},
			Style:     core.Style{Width: core.Pt(320), Height: core.Pt(360)},
			OnSelect:  func(e base.Emoji) { jaPicked = append(jaPicked, e) },
		}
	}, nsgotest.Size(400, 400))
	jaApp.Type("role:textbox", "火")
	jaApp.Key("enter")
	if len(jaPicked) != 1 || jaPicked[0].Emoji != "🔥" {
		t.Fatalf("Japanese search picked = %+v, want 🔥", jaPicked)
	}
}

func TestEmojiPickerCategoriesAndRecent(t *testing.T) {
	var picked []base.Emoji
	var cats []base.EmojiCategory
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return base.EmojiPicker{
			TestID:           "picker",
			Recent:           func() []string { return []string{"🚀", "🎉"} },
			Style:            core.Style{Width: core.Pt(320), Height: core.Pt(360)},
			OnSelect:         func(e base.Emoji) { picked = append(picked, e) },
			OnCategoryChange: func(c base.EmojiCategory) { cats = append(cats, c) },
		}
	}, nsgotest.Size(400, 400))

	if !app.Exists("role:tab Recently used") {
		t.Fatal("expected Recently used tab when Recent is non-empty")
	}
	// First item is rocket from Recently used.
	app.Focus("role:textbox")
	app.Key("Enter")
	if len(picked) != 1 || picked[0].Emoji != "🚀" {
		t.Fatalf("first item in Recent = %+v, want 🚀", picked)
	}

	// Searching then clicking a category tab clears the query and jumps to that category.
	app.Type("role:textbox", "fire")
	app.Press("role:tab Food & Drink")
	if len(cats) == 0 || cats[len(cats)-1] != base.EmojiFood {
		t.Fatalf("OnCategoryChange = %v, want EmojiFood last", cats)
	}
	app.Focus("role:textbox")
	app.Key("Enter")
	if len(picked) != 2 || picked[1].Category != base.EmojiFood {
		t.Fatalf("selecting first item of Food & Drink got %+v", picked[len(picked)-1])
	}
}

func TestEmojiPickerKeyboardNavigationAndSkinTone(t *testing.T) {
	var picked []base.Emoji
	var tones []base.EmojiSkinTone
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return base.EmojiPicker{
			TestID:           "picker",
			Columns:          4,
			Style:            core.Style{Width: core.Pt(320), Height: core.Pt(360)},
			OnSelect:         func(e base.Emoji) { picked = append(picked, e) },
			OnSkinToneChange: func(tone base.EmojiSkinTone) { tones = append(tones, tone) },
		}
	}, nsgotest.Size(400, 400))

	smileys := base.EmojisInCategory(base.EmojiSmileys, "en")
	if len(smileys) < 6 {
		t.Fatal("too few smileys")
	}

	// Empty search field: right moves to index 1, down moves to index 5 (4 columns), left moves to index 4, up moves to index 0.
	app.Focus("role:textbox")
	app.Key("right")
	app.Key("enter")
	if len(picked) != 1 || picked[0].Emoji != smileys[1].Emoji {
		t.Fatalf("after right picked %+v, want %s", picked, smileys[1].Emoji)
	}
	app.Key("down")
	app.Key("enter")
	if len(picked) != 2 || picked[1].Emoji != smileys[5].Emoji {
		t.Fatalf("after down picked %+v, want %s", picked[1], smileys[5].Emoji)
	}
	app.Key("left")
	app.Key("up")
	app.Key("enter")
	if len(picked) != 3 || picked[2].Emoji != smileys[0].Emoji {
		t.Fatalf("after left+up picked %+v, want %s", picked[2], smileys[0].Emoji)
	}

	// Select Medium skin tone via radio button and pick waving hand.
	app.Press("role:radio Medium skin tone")
	if len(tones) != 1 || tones[0] != base.SkinToneMedium {
		t.Fatalf("OnSkinToneChange = %v, want [SkinToneMedium]", tones)
	}
	app.Type("role:textbox", "waving hand")
	app.Key("enter")
	if len(picked) != 4 || picked[3].Emoji != "👋🏽" {
		t.Fatalf("waving hand with Medium skin tone = %+v, want 👋🏽", picked[3])
	}
}

func TestEmojiPickerNoSearchAndDisabled(t *testing.T) {
	var picked []base.Emoji
	var disabled *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		disabled = core.NewSignal(cx, false)
		return base.EmojiPicker{
			TestID:   "picker",
			NoSearch: true,
			Disabled: disabled.Get,
			Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
			OnSelect: func(e base.Emoji) { picked = append(picked, e) },
		}
	}, nsgotest.Size(400, 400), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))

	if app.Exists("role:textbox") {
		t.Fatal("NoSearch picker rendered a search textbox")
	}
	smileys := base.EmojisInCategory(base.EmojiSmileys, "en")
	app.Focus("id:picker")
	app.Key("right")
	app.Key("space")
	if len(picked) != 1 || picked[0].Emoji != smileys[1].Emoji {
		t.Fatalf("NoSearch space picked %+v, want %s", picked, smileys[1].Emoji)
	}

	// Disabled suppresses keyboard and press selection.
	disabled.Set(true)
	app.Settle()
	app.Key("enter")
	if len(picked) != 1 {
		t.Fatalf("disabled picker allowed selection: %+v", picked)
	}
}

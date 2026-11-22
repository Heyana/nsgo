package ui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

func TestNumberInputFormatFollowsLocaleWithoutLosingDraft(t *testing.T) {
	headless(t)
	for _, controlled := range []bool{false, true} {
		t.Run(fmt.Sprint(controlled), func(t *testing.T) {
			var scope *core.Scope
			changes := 0
			app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
				scope = cx
				value := core.NewSignal(cx, 3.0)
				format := func(v float64) string {
					unit := "seconds"
					if cx.Env().Locale() == "fr" {
						unit = "secondes"
					}
					return fmt.Sprintf("%.1f %s", v, unit)
				}
				parse := func(text string) (float64, bool) {
					words := strings.Fields(text)
					if len(words) == 0 {
						return 0, false
					}
					v, err := strconv.ParseFloat(words[0], 64)
					return v, err == nil
				}
				if controlled {
					return NumberInput{Label: "Duration", Value: value.Get, OnChange: func(v float64) { changes++; value.Set(v) }, Format: format, Parse: parse}
				}
				initial := 3.0
				return base.NumberInput{Label: "Duration", DefaultValue: &initial, OnCommit: true, Format: format, Parse: parse, OnChange: func(*float64) { changes++ }}
			})
			before := app.Find("label:Duration")
			id := before.ID
			app.Post(func() { scope.SetLocale("fr") })
			app.Settle()
			if got := app.Find("label:Duration"); got.ID != id || got.Text != "3.0 secondes" {
				t.Fatalf("unfocused translation: id=%v text=%q", got.ID, got.Text)
			}
			app.Focus("label:Duration")
			app.Clear("label:Duration")
			app.Type("label:Duration", "4.")
			app.Post(func() { scope.SetLocale("en") })
			app.Settle()
			if got := app.Find("label:Duration"); got.ID != id || !got.Focused || got.Text != "4." {
				t.Fatalf("translation replaced the active draft: id=%v focus=%v text=%q", got.ID, got.Focused, got.Text)
			}
			app.Submit("label:Duration")
			if got := app.Find("label:Duration"); got.ID != id || got.Text != "4.0 seconds" {
				t.Fatalf("committed translation: id=%v text=%q", got.ID, got.Text)
			}
			if changes != 1 {
				t.Fatalf("translation reported a value change: %d changes, want 1 edit commit", changes)
			}
		})
	}
}

func TestRemainingControlLabelsFollowLocaleOnTheirOriginalViews(t *testing.T) {
	headless(t)
	var scope *core.Scope
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		scope = cx
		label := func(name string) func() string {
			return func() string {
				if cx.Env().Locale() == "fr" {
					return name + " français"
				}
				return name
			}
		}
		initial := 2.0
		return core.View{Children: []core.Node{
			base.NumberInput{LabelFunc: label("base number"), DefaultValue: &initial},
			NumberInput{LabelFunc: label("ui number")},
			base.OtpInput{LabelFunc: label("base code"), Length: 4, DefaultValue: "12"},
			OtpInput{LabelFunc: label("ui code"), Length: 4, DefaultValue: "12"},
			Spinner{LabelFunc: label("ring"), TestID: "ring"},
			Spinner{LabelFunc: label("spokes"), Variant: SpinnerSpokes, TestID: "spokes"},
		}}
	})
	for _, name := range []string{"base number", "ui number", "base code", "ui code"} {
		app.Focus("label:" + name)
		before := app.Find("label:" + name)
		app.Post(func() { scope.SetLocale("fr") })
		app.Settle()
		after := app.Find("label:" + name + " français")
		if after.ID != before.ID || !after.Focused || after.Text != before.Text {
			t.Fatalf("%s lost its view, focus or contents on translation", name)
		}
		for _, id := range []string{"ring", "spokes"} {
			if app.Find("id:"+id).Props.Label != id+" français" {
				t.Fatalf("%s spinner label did not update", id)
			}
		}
		app.Post(func() { scope.SetLocale("en") })
		app.Settle()
	}
}

func TestFormTextFunctionsKeepFocusedRowAndHandleEmptyParts(t *testing.T) {
	headless(t)
	var title, detail, footer, override *core.Signal[string]
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		title = core.NewSignal(cx, "")
		detail = core.NewSignal(cx, "")
		footer = core.NewSignal(cx, "")
		override = core.NewSignal(cx, "")
		return FormSection{TitleFunc: title.Get, FooterFunc: footer.Get, TestID: "section", Children: []core.Node{
			FormRow{TitleFunc: func() string {
				if title.Get() == "" {
					return "Account"
				}
				return title.Get()
			}, DetailFunc: detail.Get, LabelFunc: override.Get, TestID: "row", OnPress: func() {}},
		}}
	})
	app.Focus("row")
	before := app.Find("row")
	if before.Props.Label != "Account" || len(app.Find("section").Children) != 1 {
		t.Fatal("empty functions did not remove optional parts")
	}
	app.Post(func() { title.Set("Compte"); detail.Set("Personnel"); footer.Set("Sur cet appareil") })
	app.Settle()
	after := app.Find("row")
	if after.ID != before.ID || !after.Focused || after.Props.Label != "Compte" || !app.Exists("text:Personnel") || !app.Exists("text:Sur cet appareil") {
		t.Fatal("changing form text lost the row or its label/detail/footer")
	}
	header := app.Find("role:header")
	app.Post(func() { title.Set("Profil"); override.Set("Ouvrir le profil") })
	app.Settle()
	if app.Find("role:header").ID != header.ID || app.Find("row").Props.Label != "Ouvrir le profil" {
		t.Fatal("nonempty header update remounted its text or explicit label lost precedence")
	}
	app.Post(func() { title.Set(""); detail.Set(""); footer.Set(""); override.Set("") })
	app.Settle()
	after = app.Find("row")
	if after.ID != before.ID || !after.Focused || after.Props.Label != "Account" || len(app.Find("section").Children) != 1 || app.Exists("text:Personnel") {
		t.Fatal("clearing form words left optional parts or lost focused row")
	}
}

func TestSelectSearchTextFunctionsPreserveOpenQuery(t *testing.T) {
	each(t, func(t *testing.T, env core.Environment) {
		var placeholder, empty *core.Signal[string]
		app, _ := start(t, env, func(cx *core.Scope) core.Node {
			placeholder = core.NewSignal(cx, "Find")
			empty = core.NewSignal(cx, "Nothing")
			return Select[string]{TestID: "select", Label: "Options", Searchable: true, Options: fruitOptions(), SearchPlaceholderFunc: placeholder.Get, EmptyFunc: empty.Get}
		})
		app.Press("select")
		app.Type("role:textbox", "zzz")
		search := app.Find("role:textbox")
		message := app.Find("text:Nothing")
		app.Select("role:textbox", 1, 2)
		app.Post(func() { placeholder.Set("Rechercher"); empty.Set("Aucun résultat") })
		app.Settle()
		after := app.Find("role:textbox")
		if after.ID != search.ID || !after.Focused || after.Text != "zzz" || after.Input.Placeholder != "Rechercher" || after.Props.Label != "Rechercher" || after.Selection.Start != 1 || after.Selection.End != 2 {
			t.Fatalf("search was recreated or query/focus/selection lost: %+v", after)
		}
		if app.Find("text:Aucun résultat").ID != message.ID {
			t.Fatal("translating Empty remounted its text")
		}
		app.Post(func() { placeholder.Set(""); empty.Set("") })
		app.Settle()
		after = app.Find("role:textbox")
		if after.ID != search.ID || after.Input.Placeholder != "" || after.Props.Label == "" || app.Exists("text:Aucun résultat") {
			t.Fatal("empty text functions did not clear placeholder or restore catalog defaults")
		}
	})
}

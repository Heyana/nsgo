//go:build ios && nsgotest

package ios_test

import (
	"math"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/ios"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestInputLayoutAfterPoppingPage(t *testing.T) {
	const wait = 5 * time.Second
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		nav := base.NewNav(cx, "Home")
		return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, _ func() int) base.NavPage {
			if route == "Home" {
				return base.NavPage{Title: route, Children: []core.Node{
					base.Button{Label: "Open form", TestID: "push", Style: core.Style{Width: core.Pt(160), Height: core.Pt(44)},
						OnPress: func(core.PressEvent) { nav.Push("Form") }},
				}}
			}
			return base.NavPage{Title: route, TestID: "form", Children: []core.Node{
				core.Input{TestID: "field", Label: "Address", Style: core.Style{Height: core.Pt(44)}},
				core.Input{TestID: "notes", Label: "Notes", Multiline: true, Style: core.Style{Height: core.Pt(100)}},
			}}
		})
	})
	for range 3 {
		app.Press("id:push")
		app.WaitFor("id:field", wait)
		app.Type("id:field", "Address")
		app.Type("id:notes", "Notes")
		var callbacks []func() bool
		for _, id := range []string{"field", "notes"} {
			layout, release := ios.RetainInputForTest(id)
			if layout == nil {
				t.Fatalf("the native input %q did not mount", id)
			}
			defer release()
			callbacks = append(callbacks, layout)
		}
		app.Back()
		app.WaitForGone("id:form", wait)
		for _, layout := range callbacks {
			app.WaitUntil(wait, layout)
		}
		app.WaitFor("id:push", wait)
	}
}

// TestMultilineInputTakesTheLineHeight checks that a multiline input draws its lines, and its placeholder's, at the style's line height, their baselines where a core.Text with the same style puts them (#310).
func TestMultilineInputTakesTheLineHeight(t *testing.T) {
	const wait = 5 * time.Second
	style := core.Style{Width: core.Pt(300), Height: core.Pt(120), FontSize: 16, LineHeight: 25.5, Color: core.RGB(0x000000),
		PlaceholderColor: core.RGB(0x000000), Background: core.RGB(0xffffff)}
	padded := style
	padded.Height, padded.LineHeight, padded.Padding = core.Pt(130), 40, core.Symmetric(10, 12)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{FlexGrow: 1, Background: core.RGB(0xffffff)}, Children: []core.Node{
			core.Input{TestID: "value", Multiline: true, DefaultValue: "HHHH\nHHHH\nHHHH", Style: style},
			core.Input{TestID: "placeholder", Multiline: true, Placeholder: "HHHH", Style: style},
			core.Input{TestID: "padded", Multiline: true, DefaultValue: "HHHH\nHHHH", Style: padded},
		}}
	})
	for _, id := range []string{"value", "placeholder", "padded"} {
		app.WaitFor("id:"+id, wait)
		var got, want []float64
		app.WaitUntil(wait, func() bool {
			var ok bool
			got, want, ok = ios.InputBaselinesForTest(id)
			return ok && len(got) > 0
		})
		if len(got) != len(want) {
			t.Errorf("%s: baselines %v, want core.Text's %v", id, got, want)
			continue
		}
		for i := range got {
			if math.Abs(got[i]-want[i]) > 1 {
				t.Errorf("%s: baselines %v, want core.Text's %v", id, got, want)
				break
			}
		}
	}
}

// TestMultilinePlaceholderAccessibility checks that a multiline UITextView exposes its current placeholder as its accessibilityHint (updating, clearing, hiding while text is entered, and composing with an explicit AccessibilityDescription) while its drawn UILabel stays out of VoiceOver (#1595).
func TestMultilinePlaceholderAccessibility(t *testing.T) {
	const wait = 5 * time.Second
	var setPlaceholder, setDesc, setValue func(string)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		placeholder := core.NewSignal(cx, "Write a note")
		desc := core.NewSignal(cx, "")
		value := core.NewSignal(cx, "")
		setPlaceholder = placeholder.Set
		setDesc = desc.Set
		setValue = value.Set
		return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
			core.Input{
				TestID:                       "notes",
				Label:                        "Notes",
				Multiline:                    true,
				Value:                        value.Get,
				OnChange:                     value.Set,
				PlaceholderFunc:              placeholder.Get,
				AccessibilityDescriptionFunc: desc.Get,
				Style:                        core.Style{Width: core.Pt(240), Height: core.Pt(100)},
			},
			core.Input{
				TestID:      "single",
				Label:       "Title",
				Placeholder: "Enter a title",
				Style:       core.Style{Width: core.Pt(240), Height: core.Pt(44)},
			},
		}}
	})
	app.WaitFor("id:notes", wait)
	check := func(id, wantHint string, wantLabel bool) {
		t.Helper()
		var st ios.InputAccessibilityState
		app.WaitUntil(wait, func() bool {
			var ok bool
			st, ok = ios.InputAccessibilityForTest(id)
			return ok && st.EditorHint == wantHint
		})
		if st.ContainerIsElement {
			t.Errorf("%s: input container view is an accessibility element", id)
		}
		if st.HasPlaceholderLabel != wantLabel {
			t.Errorf("%s: HasPlaceholderLabel = %v, want %v", id, st.HasPlaceholderLabel, wantLabel)
		}
		if wantLabel && (st.LabelIsElement || !st.LabelElementsHidden) {
			t.Errorf("%s: drawn placeholder label exposed to VoiceOver (element=%v, hidden=%v)", id, st.LabelIsElement, st.LabelElementsHidden)
		}
	}
	check("single", "", false)
	check("notes", "Write a note", true)

	app.Post(func() { setDesc("Supports Markdown") })
	check("notes", "Write a note. Supports Markdown", true)

	app.Post(func() { setPlaceholder("Add details") })
	check("notes", "Add details. Supports Markdown", true)

	app.Post(func() { setValue("Draft") })
	check("notes", "Supports Markdown", true)

	app.Post(func() { setValue("") })
	check("notes", "Add details. Supports Markdown", true)

	app.Post(func() { setPlaceholder("") })
	check("notes", "Supports Markdown", true)

	app.Post(func() { setDesc("") })
	check("notes", "", true)
}

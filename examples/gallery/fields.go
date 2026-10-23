package main

import (
	"fmt"

	"github.com/oneo-me/nsgo/core"
)

// fieldsDemo shows Input's editing props: several lines, a length cap,
// return keys, the selection, keys and compositions, and the colors of
// the placeholder and the caret.
func fieldsDemo(cx *core.Scope, t *theme) core.Node {
	note := core.NewSignal(cx, "")
	height := core.NewSignal(cx, core.Points(0))
	code := core.NewSignal(cx, "")
	message := core.NewSignal(cx, "")
	sent := core.NewSignal(cx, 0)
	last := core.NewSignal(cx, "")
	words := core.NewSignal(cx, "Select some of these words")
	selection := core.NewSignal(cx, core.TextRange{})
	key := core.NewSignal(cx, "")
	composing := core.NewSignal(cx, "")
	upper := core.NewSignal(cx, "")
	ref := &core.Ref{}

	multiline := t.field()
	multiline.Height = core.Length{}
	multiline.MinHeight, multiline.MaxHeight = core.Pt(36), core.Pt(140)
	multiline.Padding = core.Symmetric(8, 10)
	return t.demo("Editing", "Multiline fields grow with their text; MaxLength, SubmitBehavior, selections, keys, compositions and the placeholder and caret colors.",
		core.Input{Value: note.Get, OnChange: note.Set, Multiline: true, Placeholder: "Several lines: Return breaks the line",
			Style: multiline, Label: "Note", TestID: "note",
			OnContentSizeChange: func(s core.Size) { height.Set(s.Height) }},
		t.live(func() string { return fmt.Sprintf("The text is %v points tall.", height.Get()) }),
		core.Input{Value: code.Get, OnChange: code.Set, MaxLength: 6, KeyboardType: core.KeyboardNumber,
			AutoComplete: core.AutoCompleteOneTimeCode, ReturnKeyType: core.ReturnKeyDone, Placeholder: "6-digit code",
			Style: t.field(), Label: "Code", TestID: "code"},
		t.live(func() string { return fmt.Sprintf("%d of 6 characters", core.UTF16Len(code.Get())) }),
		core.Input{Value: message.Get, OnChange: message.Set, Multiline: true, SubmitBehavior: core.SubmitBehaviorSubmit,
			ReturnKeyType: core.ReturnKeySend, Placeholder: "Message: Return sends, Shift-Return breaks the line",
			Style: multiline, Label: "Message", TestID: "message",
			OnSubmit: func(text string) {
				if text != "" {
					sent.Update(func(n int) int { return n + 1 })
					last.Set(text)
					message.Set("")
				}
			}},
		t.live(func() string {
			if n := sent.Get(); n > 0 {
				return fmt.Sprintf("Sent %d: %q", n, last.Get())
			}
			return "Nothing sent yet."
		}),
		core.Input{Value: words.Get, OnChange: words.Set, Ref: ref, Style: t.field(), Label: "Words", TestID: "words",
			OnSelectionChange: selection.Set,
			OnKeyPress:        key.Set,
			OnCompositionChange: func(c core.Composition) {
				composing.Set(c.Text)
			}},
		hstack(
			t.plain("Select all", func() { ref.Focus(); ref.SetSelection(0, core.UTF16Len(words.Get())) }),
			t.plain("Caret to start", func() { ref.Focus(); ref.SetSelection(0, 0) }),
		),
		t.live(func() string {
			r := selection.Get()
			s := fmt.Sprintf("Selection %d to %d, last key %q", r.Start, r.End, key.Get())
			if c := composing.Get(); c != "" {
				s += fmt.Sprintf(", composing %q", c)
			}
			return s
		}),
		core.Input{Value: upper.Get, OnChange: func(s string) { upper.Set(toUpper(s)) }, AutoCapitalize: core.AutoCapitalizeCharacters,
			NoAutoCorrect: true, Placeholder: "Uppercased as you type, even with an input method",
			Style: t.field().Merge(core.Style{PlaceholderColor: t.accent, SelectionColor: t.accent}), Label: "Uppercase", TestID: "upper"},
	)
}

// toUpper uppercases ASCII letters, leaving the rest as typed.
func toUpper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'a' <= c && c <= 'z' {
			b[i] = c - 'a' + 'A'
		}
	}
	return string(b)
}

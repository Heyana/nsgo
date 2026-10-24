package main

import (
	"fmt"
	"strings"

	"github.com/oneo-me/nsgo/core"
)

func always() bool { return true }

func inputPage(cx *core.Scope, t *theme) core.Node {
	plainName := core.NewSignal(cx, "")
	plainPassword := core.NewSignal(cx, "")
	labelled := func(label string, field core.Node) core.Node {
		return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{core.Text{Content: label, Style: t.caption()}, field}}
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Input", "The platform's field: controlled, masked, placeholders and keyboard types, styled by the app.",
			labelled("Name", core.Input{Value: plainName.Get, OnChange: plainName.Set, Placeholder: "Your name", Style: t.field(), Label: "Name", TestID: "name"}),
			labelled("Password", core.Input{Value: plainPassword.Get, OnChange: plainPassword.Set, Masked: true, Placeholder: "Masked", Style: t.field(), Label: "Password"}),
			core.View{Style: core.Style{Display: core.DisplayGrid, GridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))", Gap: 10},
				Children: []core.Node{
					labelled("Email", core.Input{KeyboardType: core.KeyboardEmail, Placeholder: "name@example.com", Style: t.field(), Label: "Email"}),
					labelled("Number", core.Input{KeyboardType: core.KeyboardNumber, Placeholder: "42", Style: t.field(), Label: "Number"}),
					labelled("Phone", core.Input{KeyboardType: core.KeyboardPhone, Placeholder: "+1 555 0100", Style: t.field(), Label: "Phone"}),
					labelled("URL", core.Input{KeyboardType: core.KeyboardURL, Placeholder: "https://", Style: t.field(), Label: "URL"}),
					labelled("Read-only", core.Input{DefaultValue: "Read-only text", Readonly: true, Style: t.field(), Label: "Read-only"}),
					labelled("Disabled", core.Input{DefaultValue: "Disabled", Disabled: always, Style: t.field(), Label: "Disabled"}),
				}},
			t.live(func() string {
				if plainName.Get() == "" {
					return "Type a name above."
				}
				return fmt.Sprintf("Hello, %s. Your password has %d characters.", plainName.Get(), len([]rune(plainPassword.Get())))
			}),
		),
		fieldsDemo(cx, t),
		caretDemo(cx, t),
		accessoryDemo(cx, t),
	}}
}

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

// caretDemo shows OnCaretMove and OnPaste: typing @ opens suggestions at
// the caret, and a paste of images or files is listed instead of typed.
func caretDemo(cx *core.Scope, t *theme) core.Node {
	text := core.NewSignal(cx, "")
	caret := core.NewSignal(cx, core.Bounds{})
	pasted := core.NewSignal(cx, "nothing pasted yet")
	mentioning := func() bool {
		s := text.Get()
		i := strings.LastIndex(s, "@")
		return i >= 0 && !strings.ContainsAny(s[i:], " \n")
	}
	mention := func(name string) core.Node {
		return core.View{Role: core.RoleButton, Label: name, TestID: "mention-" + name,
			OnPress: func(core.PressEvent) {
				s := text.GetUntracked()
				text.Set(s[:strings.LastIndex(s, "@")] + "@" + name + " ")
			},
			Style:    core.Style{Padding: core.Symmetric(6, 10), CornerRadius: 6, Hover: &core.Style{Background: t.sunken}},
			Children: []core.Node{core.Text{Content: "@" + name}}}
	}
	return t.demo("Caret and paste", "OnCaretMove reports the caret's bounds, which an Overlay takes as AnchorBounds: type @ for suggestions at the caret. OnPaste reports pasted text, images and files; PasteReport keeps the text out.",
		core.Input{TestID: "caret-field", Value: text.Get, OnChange: text.Set, Placeholder: "Write @ to mention someone",
			Style: t.field(), OnCaretMove: func(e core.CaretEvent) { caret.Set(e.PageBounds) },
			OnPaste: func(e core.PasteEvent) {
				var parts []string
				if e.Text != "" {
					parts = append(parts, fmt.Sprintf("%d characters", len([]rune(e.Text))))
				}
				for _, f := range e.Files {
					parts = append(parts, fmt.Sprintf("a %s of %d bytes", f.MIME, len(f.Data)))
				}
				if n := len(e.URIs); n > 0 {
					parts = append(parts, fmt.Sprintf("%d files", n))
				}
				pasted.Set("pasted " + strings.Join(parts, ", "))
			}},
		core.Show{When: mentioning, Then: func(*core.Scope) core.Node {
			return core.Overlay{AnchorBounds: caret.Get, Placement: core.PlaceBottom, Offset: 4, TestID: "mentions",
				Style: core.Style{Padding: core.All(4), Background: t.surface, CornerRadius: 8, BorderWidth: 1,
					BorderColor: t.line, ShadowColor: core.RGBA(0, 0.15), ShadowRadius: 8, ShadowOffset: core.Point{Y: 2}},
				Children: []core.Node{mention("ada"), mention("grace"), mention("linus")}}
		}},
		core.Text{TestID: "paste-status", Style: t.caption(), ContentFunc: pasted.Get},
	)
}

package core

import "testing"

func TestInputSubmitResolvesWithMultiline(t *testing.T) {
	for _, c := range []struct {
		set       SubmitBehavior
		multiline bool
		want      SubmitBehavior
	}{
		{SubmitBehaviorDefault, false, SubmitBehaviorBlurAndSubmit},
		{SubmitBehaviorDefault, true, SubmitBehaviorNewline},
		{SubmitBehaviorNewline, false, SubmitBehaviorBlurAndSubmit},
		{SubmitBehaviorNewline, true, SubmitBehaviorNewline},
		{SubmitBehaviorSubmit, true, SubmitBehaviorSubmit},
		{SubmitBehaviorBlurAndSubmit, true, SubmitBehaviorBlurAndSubmit},
	} {
		cfg := InputConfig{SubmitBehavior: c.set, Multiline: c.multiline}
		if got := cfg.Submit(); got != c.want {
			t.Errorf("Submit(%v, multiline %v) = %v, want %v", c.set, c.multiline, got, c.want)
		}
	}
}

func TestInputConfigCarriesProps(t *testing.T) {
	h := newHost(t, Size{200, 200}, func(cx *Scope) Node {
		return View{Style: Style{SelectionColor: RGB(0x00ff00)}, Children: []Node{
			Input{TestID: "in", Multiline: true, MaxLength: 5, ReturnKeyType: ReturnKeySend,
				SubmitBehavior: SubmitBehaviorSubmit, AutoCapitalize: AutoCapitalizeWords, NoAutoCorrect: true,
				AutoComplete: AutoCompleteEmail, KeyboardAppearance: KeyboardAppearanceDark, HideCaret: true,
				Style: Style{PlaceholderColor: RGB(0xff0000)}},
		}}
	})
	b := h.commit()
	var c *InputConfig
	for _, it := range b.Items {
		if it.Kind == ItemInput {
			c = it.Input
		}
	}
	if c == nil {
		t.Fatal("no ItemInput")
	}
	want := InputConfig{Multiline: true, MaxLength: 5, ReturnKey: ReturnKeySend, SubmitBehavior: SubmitBehaviorSubmit,
		AutoCapitalize: AutoCapitalizeWords, NoAutoCorrect: true, AutoComplete: AutoCompleteEmail,
		KeyboardAppearance: KeyboardAppearanceDark, HideCaret: true,
		PlaceholderColor: RGB(0xff0000), SelectionColor: RGB(0x00ff00)}
	got := *c
	got.Style, got.Content = InputConfig{}.Style, Bounds{}
	if got != want {
		t.Fatalf("config = %+v\nwant %+v", got, want)
	}
	if AutoCompleteNewPassword.String() != "new-password" || AutoCompleteBirthdate.String() != "bday" {
		t.Fatal("autocomplete names")
	}
}

func TestMultilineInputGrowsWithItsText(t *testing.T) {
	var text *Signal[string]
	var sizes []Size
	h := newHost(t, Size{200, 400}, func(cx *Scope) Node {
		text = NewSignal(cx, "")
		return View{Style: Style{AlignItems: AlignStart}, Children: []Node{
			Input{TestID: "in", Multiline: true, Value: text.Get, OnChange: text.Set,
				Style:               Style{Width: Pt(100), FontSize: 10},
				OnContentSizeChange: func(s Size) { sizes = append(sizes, s) }},
		}}
	})
	h.commit()
	in := h.find("in")
	if in.frame.Size.Height != 12 {
		t.Fatalf("empty height %v, want a line of 12", in.frame.Size.Height)
	}
	if len(sizes) != 1 || sizes[0] != (Size{0, 12}) {
		t.Fatalf("sizes %v, want an empty line", sizes)
	}
	// 20 characters of 6 points wrap at 100 points into two lines.
	h.tree.Dispatch(Event{Kind: EventChangeText, ID: in.id, Text: "aaaaaaaaa bbbbbbbbbb", Count: 1})
	h.commit()
	if in.frame.Size.Height != 24 {
		t.Fatalf("height %v, want two lines", in.frame.Size.Height)
	}
	if len(sizes) != 2 || sizes[1].Height != 24 {
		t.Fatalf("sizes %v", sizes)
	}
	// A trailing line break takes a line.
	h.tree.Dispatch(Event{Kind: EventChangeText, ID: in.id, Text: "a\n", Count: 2})
	h.commit()
	if in.frame.Size.Height != 24 {
		t.Fatalf("height %v with a trailing line break, want two lines", in.frame.Size.Height)
	}
}

func TestInputSelectionKeysAndComposition(t *testing.T) {
	var sel *Signal[TextRange]
	var got []TextRange
	var keys []string
	var comps []Composition
	ref := &Ref{}
	h := newHost(t, Size{200, 200}, func(cx *Scope) Node {
		sel = NewSignal(cx, TextRange{1, 2})
		return Input{TestID: "in", Ref: ref, DefaultValue: "abc", SelectedRange: sel.Get,
			OnSelectionChange:   func(r TextRange) { got = append(got, r) },
			OnKeyPress:          func(k string) { keys = append(keys, k) },
			OnCompositionChange: func(c Composition) { comps = append(comps, c) }}
	})
	b := h.commit()
	in := h.find("in")
	value, selected := -1, -1
	for i, it := range b.Items {
		switch it.Kind {
		case ItemInputValue:
			value = i
		case ItemSelection:
			selected = i
			if it.Selection != (TextRange{1, 2}) {
				t.Fatalf("selection %v", it.Selection)
			}
		}
	}
	if value < 0 || selected < value {
		t.Fatalf("value at %d, selection at %d: want the selection after the value", value, selected)
	}
	for _, k := range []EventKind{EventSelectionChange, EventKeyPress, EventCompositionChange} {
		if !in.events().Has(k) {
			t.Fatalf("mask lacks %v", k)
		}
	}
	sel.Set(TextRange{0, 3})
	if b := h.commit(); len(b.Items) != 1 || b.Items[0].Kind != ItemSelection || b.Items[0].Selection != (TextRange{0, 3}) {
		t.Fatalf("items %+v", b.Items)
	}
	ref.SetSelection(2, 2)
	if b := h.commit(); len(b.Items) != 1 || b.Items[0].Selection != (TextRange{2, 2}) {
		t.Fatalf("items %+v", b.Items)
	}
	h.tree.Dispatch(Event{Kind: EventSelectionChange, ID: in.id, Index: 1, Count: 3})
	h.tree.Dispatch(Event{Kind: EventKeyPress, ID: in.id, Text: "Backspace"})
	h.tree.Dispatch(Event{Kind: EventCompositionChange, ID: in.id, Text: "ni"})
	h.tree.Dispatch(Event{Kind: EventCompositionChange, ID: in.id, Text: "ni", Finished: true})
	if len(got) != 1 || got[0] != (TextRange{1, 3}) || len(keys) != 1 || keys[0] != "Backspace" {
		t.Fatalf("selection %v, keys %v", got, keys)
	}
	if len(comps) != 2 || comps[0] != (Composition{true, "ni"}) || comps[1] != (Composition{}) {
		t.Fatalf("compositions %v", comps)
	}
}

func TestLimitEditAndUTF16(t *testing.T) {
	if UTF16Len("a😀") != 3 || ClipUTF16("a😀b", 2) != "a" || ClipUTF16("a😀b", 3) != "a😀" {
		t.Fatal("utf-16 lengths")
	}
	if UTF16Offset("a😀b", 3) != 5 || UTF16Units("a😀b", 5) != 3 {
		t.Fatal("utf-16 offsets")
	}
	if kept, ok := LimitEdit("abc", 0, "de", 4); kept != "d" || !ok {
		t.Fatalf("kept %q %v", kept, ok)
	}
	if _, ok := LimitEdit("abcd", 0, "e", 4); ok {
		t.Fatal("a full field took a character")
	}
	if kept, ok := LimitEdit("abcd", 2, "xyz", 4); kept != "xy" || !ok {
		t.Fatalf("replacing kept %q %v", kept, ok)
	}
}

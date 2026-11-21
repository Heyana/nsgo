---
title: Select
description: One value from a list, as a popup or a sheet.
group: Components
order: 36
---

`ui.Select[T]` chooses one of a list of values of any comparable type. The list opens under the trigger on every device; `Presentation: base.PresentSheet` opens it as a sheet from the bottom instead, as a phone app may want.

```go preview=select/basic height=280
size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options: []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:   size.Get,
	OnChange: size.Set,
	Label:   "Size",
}
```

## Options and search

`Options` are `ui.Option[T]`s, each with a `Value`, a `Label`, and an optional `Group`, `Icon`, `Disabled` function or `Content` of the app's own. Adjacent options of one non-empty `Group` show under a header of it, in a `Select` or a `Combobox`, which the arrow keys and typing skip and screen readers read as a heading. Use `OptionsFunc` when the options follow a signal, so the open list updates in place and keeps the field's value and focus. `Placeholder` shows while no option is chosen. `Searchable` filters the list by typing, `OnClear` shows a clear button, and `Disabled` stops it.

```go preview=select/searchable height=400
ui.Select[string]{
	Options:  languages,
	Value:    language.Get,
	OnChange: language.Set,
	Searchable: true,
	SearchPlaceholder: "Search languages",
	OnClear:  func() { language.Set("") },
	Placeholder: "Choose a language",
}
```

`Style` lays the select out, 200 points wide unless it sets a width; its height, padding and type reach the field, as an [Input's](input.md) do, so a select lines up with a `SizeSmall` button:

```go
ui.Select[string]{Options: sizes, Value: size.Get, OnChange: size.Set, Label: "Size",
	Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
```

## Behavior

Up, Down, Return and Space open the list from the focused trigger; arrows move the highlight and typeahead jumps by the items' titles. Choosing an item closes the list and reports its value. Escape, back and a press outside close it without a change. `Empty` is the text shown while search finds nothing.

## Accessibility

`Label` names the field for assistive technology. Use `LabelFunc` to update that name in place as its signals change. An empty label uses `Placeholder`. The trigger and list carry the select's roles, and the selected item is reported. The keyboard reaches the trigger with Tab and opens it with Return or Space.

## Platform

On macOS, Windows and Linux the list is a popup beside the trigger. On iOS and Android it is a sheet, and the selected row stays on the field. The same options and values work on every target.

## Related

- [Forms](../forms.md): selects in a form.
- [Menu](menu.md): commands rather than a value.
- [Tabs](tabs.md): a few options shown at once.
- The gallery's Select and Combobox pages.
- [Unstyled components](../authoring/base.md#select): `base.Select`, with groups and a trigger of your own.

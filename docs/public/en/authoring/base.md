---
title: Unstyled components
description: NSGo's own controls without a look, to style part by part or to build a styled library on.
group: Writing components
order: 73
---

`base` holds every control `ui` has, with its behavior, state and accessibility but no look. Each part takes a `Class`, a `Style` and a `StyleFunc` ([Parts and states](parts.md)), so a library or an app with a design of its own uses them instead of `ui`, or wraps them as `ui` does ([Looks and themes](looks.md)). This page covers the controls of the [component reference](../ui.md); the gallery shows every one beside its `ui` form.

Controls such as `Button`, `Checkbox`, `Select`, `Slider` and `NumberInput` accept `Invalid: rejected.Get` to follow a validation signal. Style that state with `Class: "aria-invalid:border-red-500"` or `Style.AriaInvalid`. It updates accessibility and styles without disabling the control. Composite fields pass it to their real input and decorative parts; `ColorPicker` shares it with its controls.

For application values dragged between views, use [`base/dragdrop`](../drag-drop.md): an area connects sources and targets while the app supplies their content, preview and appearance.

## Button

`base.Button` takes its look from `Style` and its content from `Children`. `OnPress` runs on a completed press; `OnPressDown` and `OnPressUp` follow a press starting and ending, for feedback of your own.

```go
base.Button{
	Label:   "Save",
	OnPress: func(core.PressEvent) { save() },
	Style: core.Style{Background: core.RGB(0x007aff), Padding: core.Symmetric(8, 14), CornerRadius: 8, AlignSelf: core.AlignStart,
		Active: &core.Style{Opacity: core.Alpha(0.85)}},
	Children: []core.Node{core.Text{Content: "Save", Style: core.Style{Color: core.RGB(0xffffff)}}},
}
```

`Disabled`, `Busy`, `Selected` and `Expanded` report a state to the app, the style and assistive technology: an action still running uses `Busy` (styled with `aria-busy:`), a toggle or the current item of a list `Selected`, a disclosure `Expanded`. `NoFocus` takes a button out of the Tab stops when a row or a list reaches it another way.

## Checkbox

`base.Checkbox` takes a style for the box, one for the indicator and your mark inside it, shown from the value the app owns.

```go
base.Checkbox{
	Checked:        done.Get,
	OnChange:       done.Set,
	Label:          "Remember me",
	Style:          core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	IndicatorStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), BorderWidth: 1, CornerRadius: 5},
	Indicator:      core.Icon{SVG: lucide.Check(), Style: core.Style{FontSize: 14}},
	Children:       []core.Node{core.Text{Content: "Remember me"}},
}
```

The indicator takes its state from the box through `PartStates`, so `checked:` classes style it without making it a second stop.

For modifiers, set `OnChangeEvent func(checked bool, press core.PressEvent)`. It replaces `OnChange` and leaves the value with the app; `press.Modifiers.Shift` lets a list select a range. Space/Return activation on desktops, the web, Android and iOS/tvOS preserves modifiers unless an app shortcut handles the key first; on Windows, Alt with Space or Return goes to the window menu instead. In TSX, `onChangeEvent` receives `{ checked, press }`.

## Switch

`base.Switch` is a track, a thumb inside it, then its children as the label. A press reports the value it asks for, the opposite of `Checked`.

```go
base.Switch{
	Checked:    dark.Get,
	OnChange:   dark.Set,
	Label:      "Dark appearance",
	Style:      core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	TrackStyle: core.Style{Width: core.Pt(38), Height: core.Pt(22), CornerRadius: 11},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

The track and thumb take `StyleFunc`s of their own, and their state styles follow `Checked`.

## Slider

`base.Slider` takes a `base.SliderValue`: `base.SingleValue(v)` or `base.RangeValue(low, high)`. It styles the track, the indicator, the range and the thumbs apart.

```go
level := core.NewSignal(cx, 3.0)
base.Slider{
	Value:      func() base.SliderValue { return base.SingleValue(level.Get()) },
	OnChange:   func(v base.SliderValue) { level.Set(v.End) },
	Min:        func() float64 { return 0 },
	Max:        func() float64 { return 10 },
	Step:       func() float64 { return 1 },
	Label:      "Level",
	Style:      core.Style{Height: core.Pt(28)},
	TrackStyle: core.Style{Height: core.Pt(4), CornerRadius: 2},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

`Min`, `Max` and `Step` are reactive getters: changing their signals updates the mounted slider and its accessible range, including during a drag. `Axis` makes it vertical and `Inverted` puts its minimum at the far end, as `base.Progress`'s do; `Marks` places your own ticks on the indicator, and `OnChangeEnd` reports the value once a drag stops.

## Select

`base.Select` takes `base.SelectItem`s and a `*string` value, so “none” is `nil`. You style the trigger, the popup, its rows and its groups.

```go
choice := core.NewSignal[*string](cx, nil)
base.Select{
	Items: func() []base.SelectItem {
		return []base.SelectItem{{Value: "s", Title: "Small"}, {Value: "m", Title: "Medium"}}
	},
	Value:      choice.Get,
	OnChange:   choice.Set,
	Label:      "Size",
	Style:      core.Style{Height: core.Pt(28), Padding: core.Symmetric(4, 10), BorderWidth: 1, CornerRadius: 6},
	PopupStyle: core.Style{Padding: core.All(4), CornerRadius: 8},
	ItemStyle:  core.Style{Padding: core.Symmetric(4, 8), CornerRadius: 4},
}
```

Adjacent items with the same `Group` share a header, typeahead matches the items' titles, `SearchField` draws a search field of your own, and `Cleanable` shows a clear button. `OnCancel` hears Escape, back and a press outside.

## Tabs

`base.Tabs` holds `base.Tab`s and draws the indicator from `IndicatorStyle`; the app keeps which tab is selected. `base.Tab.OnPress` runs on every press, the selected one's included.

```go
base.Tabs{Label: "Sections", IndicatorStyle: core.Style{Height: core.Pt(2), Background: core.RGB(0x007aff)}, Children: []core.Node{
	base.Tab{Label: "Overview", Selected: func() bool { return section.Get() == "overview" },
		OnPress: func(core.PressEvent) { section.Set("overview") }, Children: []core.Node{core.Text{Content: "Overview"}}},
	base.Tab{Label: "Activity", Selected: func() bool { return section.Get() == "activity" },
		OnPress: func(core.PressEvent) { section.Set("activity") }, Children: []core.Node{core.Text{Content: "Activity"}}},
}}
```

## Dialog

`base.Dialog` takes its backdrop, layer and popup styles, an `Animation` or a `TransitionShader`, and your children. `Open`, `DefaultOpen` and `OnOpenChange` control it; `OnOpen`, `OnClose` and `OnCancel` report what happened. `OnConfirm` is the default action: Return runs it, unless the keyboard focus is on a button or a field that takes Return itself or another modal covers the dialog, then the dialog closes with `ReasonSelect`. `Role` and `AccessibilityDescription` are what screen readers announce, `RoleAlertDialog` for a confirmation. `NoCloseOnSwipe` keeps iOS's swipe from closing it, `Presentation` chooses the iOS presentation, and `StatusBar` changes the bar while it is open.

`base.Sheet` is a panel at any edge, with a backdrop and your content; with `CloseOnSwipe` a bottom sheet follows a downward drag and closes past a threshold. It takes `OnConfirm`, `Role` and `AccessibilityDescription` as `base.Dialog` does. `base.ShowAlertDialog` shows the system's alert or action sheet with buttons by role:

```go
base.ShowAlertDialog(cx, base.AlertDialog{
	Title: "Delete the note?",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(label string) {
	if label == "Delete" {
		remove()
	}
})
```

## Menu

`base.Menu` opens `base.MenuItem`s from a trigger of your own, and `base.ContextMenu` at the pointer. An item reports `OnSelect` and closes the menu unless `NoCloseOnSelect`; `Checked` makes it a checkable item, `Indicator` draws your mark before its content, and `Role` chooses what assistive technology hears: a menu item, a checkbox or a radio item. A `base.Submenu` holds more items.

```go
base.Menu{Trigger: core.Text{Content: "Actions"}, Children: []core.Node{
	base.MenuItem{Label: "Show sidebar", Checked: sidebar.Get, OnSelect: func() { sidebar.Set(!sidebar.Get()) }},
	base.Submenu{Label: "Share", Children: []core.Node{
		base.MenuItem{Label: "Copy link", OnSelect: copyLink},
		base.MenuItem{Label: "Email", OnSelect: emailLink},
	}},
}}
```

`Placement`, `Align`, `Offset` and `Margin` place the popup on desktops, and `PopupStyle` and `BackdropStyle` style it.

## EmojiPicker

`base.EmojiPicker` searches every Unicode emoji by its localized CLDR names, keywords and shortcodes in the app's languages (or explicit `Languages` / `LanguagesFunc`), with category tabs, optional `Recent` entries, skin tone selection, keyboard navigation and a virtualized grid. All default labels and category names come from `base/locales`, and the emoji dataset links only into apps that use `EmojiPicker` or its helpers (`base.SearchEmojis`, `base.EmojisInCategory`, `base.EmojiByString`).

```go
base.EmojiPicker{
	Recent:   recent.Get,
	OnSelect: func(e base.Emoji) { insert(e.Emoji) },
	Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
}
```

`Category` and `DefaultCategory` choose the browse group. Category tabs and skin tone choices are each one Tab stop with arrow navigation. `EmojiPickerRef` provides `Focus`, `Query`, `SetQuery`, `SetCategory` and `MatchedCount`; the styled picker uses the same query state. `LabelFunc`, `SearchLabelFunc` and `PlaceholderFunc` update words in place. `Emoji.WithSkinTone` applies the chosen tone to both people in a pair, and the default tone restores its neutral form; searching by an emoji preserves its exact sequence.

Use `ui.EmojiPicker` for the themed picker with Lucide category icons, a clearable search field, highlighted emoji preview and skin tone swatches.

---
title: Accessibility
description: Labels, roles, values, actions and focus for screen readers and keyboards.
group: Core concepts
order: 61
---

## What a screen reader reads

A view's `Label` is what assistive technology says, `AccessibilityDescription` more about it, read after the label, `Hint` what activating it does, and `Role` what it is: `core.RoleButton`, `RoleLink`, `RoleHeader`, `RoleImage`, `RoleCheckbox`, `RoleSwitch`, `RoleRadio`, `RoleTab`, `RoleTextField`, `RoleSearchField`, `RoleStatus`, `RoleMenu`, `RoleMenuItem`, `RoleDialog`, `RoleAlertDialog`, `RoleAlert`, `RoleAdjustable`, `RoleProgressBar`, `RoleTable` and the rest. A status is a polite live region by default; a search field is a text field announced as a search box. A dialog is read with its role and label as it opens, and its description after them, as `ui.Dialog`'s title and description are. A text without a label is read as its content. `LabelFunc`, `HintFunc` and `AccessibilityDescriptionFunc` bind changing accessibility text on the same view, so controls keep their focus and state through a language switch. Core elements with a label all accept `LabelFunc`, and `View` and `Text` accept `HintFunc`.

```go
core.View{
	Role:  core.RoleButton,
	Label: "Add to cart",
	Hint:  "Adds the item and opens the cart",
	OnPress: add,
	Children: []core.Node{core.Icon{SVG: lucide.ShoppingCart}},
}
```

`AccessibilityValue` gives a range or a textual value: a slider's minimum, maximum and current number, or a text such as "3 items". `Accessible` makes a view one element, so a card's name and price are read together instead of as two stops, and without a label the view reads its descendants' text. `AccessibilityElementsHidden` hides decoration, such as a chevron or a separator, while `ImportantForAccessibility` hides a whole subtree.

```go
core.View{Accessible: true, Label: "Apples, $1.20 a kilo", Children: []core.Node{
	core.Text{Content: "Apples"},
	core.Text{Content: "$1.20 a kilo"},
}}
```

## Visible labels

Give a `core.Text` or `core.View` the control's `Ref` as `LabelFor`. Pressing the label focuses the control and runs its press action, such as toggling a checkbox. Its current visible text names the control without repeating a `Label` string, and it follows the control's `Disabled` and `Invalid` states for accessibility and styles. A label gains no button role or Tab stop.

```go
field := new(core.Ref)
return core.View{Children: []core.Node{
	core.Text{Content: "Email", LabelFor: field},
	base.Input{Ref: field},
}}
```

`LabelledBy: []*core.Ref{label, requirement}` names a control from several visible elements in the given order, ahead of `Label`; it does not forward presses. Otherwise `Label` wins over implicit `LabelFor` names, whose order follows the tree. Text updates, moves, hiding, removal and remounting update the relation. Missing, repeated, cyclic and hidden references are ignored; associations stay inside one surface and confinement scope. A label cannot activate a control behind a modal.

`base` and `ui` buttons, checkboxes, radios, switches, selects and number inputs pass `Ref` and `LabelledBy` to their control; sliders expose `ThumbRef` and `StartThumbRef`. In TSX, use `labelFor={field()}` and `labelledBy={[label()]}` with node signals populated by `ref={node => field.set(node)}`, so a label before its field updates when that field mounts.

## Actions

`AccessibilityActions` lists what the screen reader offers on an element: the standard actions (`core.ActionActivate`, `ActionIncrement`, `ActionDecrement`, `ActionEscape`, `ActionMagicTap`) and the app's own, each with a `Name` and a `Label`. `OnAccessibilityAction` receives the name.

```go
core.View{
	Role: core.RoleAdjustable,
	Label: "Quantity",
	AccessibilityValue: func() core.AccessibilityValue {
		return core.AccessibilityValue{Max: 10, Now: float32(n.Get()), Text: fmt.Sprintf("%d items", n.Get())}
	},
	AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}},
	OnAccessibilityAction: func(name string) {
		if name == core.ActionIncrement {
			n.Update(func(v int) int { return v + 1 })
		} else {
			n.Update(func(v int) int { return v - 1 })
		}
	},
	Children: quantityControls,
}
```

`MultiSelectable` marks a grid, listbox or tree that permits more than one selected item. `base.DataTable` sets it for multi-row selection; the web, Windows and Linux expose the corresponding accessibility flag.

`Expanded` says whether a disclosure or a menu button is open, `Busy` that content is loading, `Invalid` that the app rejected a field's content or a control's value (styled with `aria-invalid:`), and `Checked`, `Selected` and `Pressed` report a control's state. A control's parts can take its state through `PartStates`, so an indicator follows its checkbox without being a stop of its own.

`SelectionItem` marks a view as an item in a selectable collection independently of its current `Selected` value. `base.DataTable` sets it on rows or cells when that table selects them, and `base.Tree` sets it on every row; Windows keeps the SelectionItem pattern and the web writes `aria-selected="true"` or `"false"`. A table without selection leaves rows without either marker.

## Live changes

`LiveRegion` has screen readers announce a view's label and the text inside it as they change: `core.LiveRegionPolite` waits for the user to be idle, `LiveRegionAssertive` interrupts. `cx.Announce` says one message once.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("%d items in the basket", count.Get()) },
	LiveRegion: core.LiveRegionPolite}
```

## Focus and the keyboard

Tooltips describe their trigger before it receives focus. `View.Tooltip`, `base.Tooltip` and `ui.Tooltip` use their current text as the control's accessibility description, unless it repeats the control's name, the app supplies `AccessibilityDescription` or the tooltip is `Disabled`. A wrapper passes the description to its controls. The bubble and its children are hidden from screen readers and never announce an alert; `ui.IconButton`'s default tooltip therefore does not repeat its label.

A `base.DataTable` exposes its named grid as the keyboard and accessibility focus stop; the pointer hit layer is hidden from assistive technology.

On desktops, the web and an Apple TV, views that take presses are Tab stops beside text fields; `NoFocus` keeps one out and `Focusable` makes a view without presses a stop. `Style.FocusVisible` draws the focus ring only when the keyboard moved the focus (`focus-visible:`), `Style.Focus` any focus and `Style.FocusWithin` the focus inside a view, `OnFocus` and `OnBlur` hear it, `OnBlur` also as the focused view leaves the tree, and `Ref.Focus()` and `Ref.SetAccessibilityFocus()` move it; `core.FocusedRef(cx)` is a ref to the view that has it, for what opens without a trigger, such as a find bar from a key, to give it back as it closes. Return and Space press the focused view, and arrow keys move through tab lists, radio groups and menus. A view's `Tooltip` shows at once when the keyboard moves the focus to it, until the focus leaves or Escape is pressed. A popup's `OnOpen` runs once its content is there, so a `Focus` there moves the focus in, as a non-modal dialog's should; a popover's content follows its trigger in the Tab order, and the focus goes back to the trigger as the popup closes.

`cx.Env().ScreenReader()` says whether one runs, `cx.Env().ReduceMotion()` whether the user asked for less motion, `cx.Env().IncreaseContrast()` whether the user asked for more contrast (Increase Contrast on macOS, iOS and watchOS, a Windows contrast theme, GNOME's High Contrast, GTK 4.20's contrast setting or a high-contrast GTK theme, Android 14's contrast setting above standard, `prefers-contrast: more` or `forced-colors: active` on the web), `cx.Env().FontScale()` the text size, and `cx.Env().RTL()` the layout direction. Texts follow the text size unless `NoFontScaling` is set, and a layout that is right to left mirrors its physical edges when it uses the logical ones.

## Next

- [Styles and themes](styles.md): focus and disabled states.
- [Forms](forms.md): labels and values for fields.
- [Canvas and shaders](canvas.md): exposing a drawing's parts.

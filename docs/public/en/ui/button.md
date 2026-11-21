---
title: Button
description: A push button, its variants and sizes, and the icon button.
group: Components
order: 31
---

`ui.Button` is a push button in `ui`'s look, with the variants and sizes a window needs. `OnPress` runs on a completed press.

```go preview=button/basic
ui.Button{Text: "Save", OnPress: save}
ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: cancel}
```

## Variants and sizes

`ui.Button.Variant` is `ButtonPrimary` (the default), `ButtonSecondary`, `ButtonOutline`, `ButtonGhost`, `ButtonTinted` or `ButtonDanger`. Tinted and danger use the theme’s accent or danger color on a soft fill. On desktop and TV idioms, `Size` is `SizeDefault` (28 points), `SizeSmall` (24) or `SizeLarge` (32). On touch idioms, the same sizes are 44, 40 and 48 points, so the button can be used without restyling it.

```go preview=button/variants
ui.Button{Text: "Delete", Variant: ui.ButtonOutline, OnPress: remove}
ui.Button{Text: "Continue", Size: ui.SizeLarge, OnPress: next}
ui.Button{Text: "Save", Icon: lucide.Check, Flex: 1, OnPress: save}
```

`Color` fills the button with a color of the app's instead of a variant's, and `Tint` its icon. `Flex` gives it a share of a row, while `Style` lays it out: it can make the button fill its container, set a size of the app's, add margins or round its corners; `TestID` names it for tests.

```go
ui.Button{Text: "Continue", Size: ui.SizeLarge,
    Style: core.Style{Width: core.Pct(100), Height: core.Pt(50)}, OnPress: next}
```

A button of an icon alone is `ui.IconButton`, with `Variant` `IconButtonGhost`, `IconButtonRaised` or `IconButtonFilled`, a flat circle in the theme's inset color or `Fill`, a `Size` in points and a `Label` for assistive technology. On desktops the label also shows as a tooltip once the pointer rests on the button; `Tooltip` says something else there, and one that returns `""` shows none. A `ui.Button` with `Icon` and no `Text` is a square too, and needs `Label`.

```go preview=button/icon
ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost, OnPress: remove}
```

## Content and states

`ui.Button` shows `Text`, an optional leading `Icon`, or both. Use `TextStyle` to customize the label’s font size, weight or other text styles.

```go
ui.Button{Text: "Saving…", Disabled: saving.Get, OnPress: save}
```

`Disabled` takes a function, so the button follows a signal: a disabled button takes neither presses nor the keyboard focus, and its look is the disabled state, not a color the app picks.

`Busy` is for an action still running. While it returns true, a spinner in the button's text color shows over the content, so it shows on a primary button's accent too, presses do nothing, and assistive technology hears that it is busy. By default the content keeps its room under the spinner. The button keeps keyboard focus. `BusyLabel` or `BusyLabelFunc` shows a message beside the spinner and reports it as the button’s accessible name while it is busy. The button keeps at least its resting width.

```go preview=button/busy
ui.Button{Text: "Save", Busy: saving.Get, BusyLabel: "Saving…", OnPress: save}
```

`ui.Spinner` and `ui.ProgressRing` take a `Tint` of their own for any other surface their accent would vanish on.

## Links

`ui.Link{Text: "Docs", Href: "/docs"}` opens a destination with the link role. Its text and hover underline use the accent unless `Style.Color` sets another color; `Style.Hover.Color` sets the hover color.

## Accessibility

The button reports `RoleButton` and its text as its name. `Label` replaces the text for assistive technology. An icon-only button without `Label` is unreadable, so name it.

Return and Space press a focused button on desktops, the web and Apple TV; a phone's screen reader presses it as a click.

## Platform

The button is a view drawn the same way everywhere, not a platform button; presses, focus and accessibility come from the platform. Touch idioms use larger layout targets as described above.

## Related

- [Forms](../forms.md): buttons in a form and validation.
- [Styles and themes](../styles.md): the theme a button takes its colors from.
- [Accessibility](../accessibility.md): roles, labels and focus.
- The gallery's Button page.
- [Unstyled components](../authoring/base.md#button): `base.Button`, for a button of a look of your own.

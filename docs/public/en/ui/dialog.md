---
title: Dialog
description: A modal surface, a sheet and an alert.
group: Components
order: 41
---

`ui.Dialog` is a modal surface over a dimmed app, a sheet from the bottom on touch screens. `ui.Sheet` is a panel from the bottom, and `ui.AlertDialog` asks the user to confirm.

```go preview=dialog/basic height=360
open := core.NewSignal(cx, false)
ui.Dialog{
	Open:         open.Get,
	OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
	Title:        "Rename the file",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { open.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

## Opening and closing

The app holds whether the dialog is open. `OnOpenChange` hears every change the dialog asks for, with a `base.Reason` that tells a press outside from Escape or back; `OnClose` runs once it has closed. `NoCloseOnEscape` and `NoCloseOnBackdropPress` keep Escape and a press outside from closing it.

`Title` and `Description` head the surface, `TitleFunc` and `DescriptionFunc` when they change while it is open, and `Footer` holds its buttons. `Bare` leaves the whole surface to `Children`, for a dialog of the app's own layout, which desktops clip inside the surface's rim to the corners `Style` and `StyleFunc` give it, and `Inset` sets its least distance from the window's edges on desktops.

On touch sheets, `Bare` now removes the default grabber and content padding so `Children` fill the panel. `NoGrabber` hides only the grabber, `ContentStyle` and `ContentStyleFunc` customize the content wrapper, and `TitleStyle` and `TitleStyleFunc` customize the title. `Leading` and `Trailing` put nodes, such as Cancel and Done buttons, at the ends of the title row, with the title centered between them. `StartingStyle` and `EndingStyle` customize the sheet transition; include a `Transition` in them when you need custom timing.

`Style` merges over the surface's look. `StyleFunc` merges after it and follows the signals it reads, including the appearance, while the dialog is open and when reopened. For example, `StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} }` follows the theme, where `colors := ui.ColorsOf(cx)`. Both fields also work on `ui.AlertDialog`. A zero color leaves the existing color in place; use a nonzero transparent color such as `core.RGBA(0xFFFFFF, 0)` to clear it.

## Sheets

`ui.Sheet` is a panel at the bottom over a dimmed backdrop; a press on the backdrop, Escape or back calls `OnClose`.

Set `Label` or `LabelFunc` for its accessible dialog name, and `TestID` to find the panel in tests. `LabelFunc` follows its signals without replacing the open content. `CloseOnSwipe: true` also calls `OnClose` after a downward swipe passes the closing threshold; update `Open` in that callback to close it.

Set `Style` or `StyleFunc` to customize the surface's background, corner radii and padding while keeping the grabber and safe-area handling. `StyleFunc` follows the signals it reads. A `CornerRadius` without `CornerRadii` rounds every corner in place of the default rounded top.

```go preview=dialog/sheet height=320
ui.Sheet{Open: filters.Get, OnClose: func() { filters.Set(false) }, Children: filterControls}
```

## Alerts

`ui.AlertDialog` asks the user to confirm or acknowledge something. Each action closes it, and a press outside does not.

```go preview=dialog/alert height=320
ui.AlertDialog{
	Open:         confirm.Get,
	OnOpenChange: func(v bool, _ base.Reason) { confirm.Set(v) },
	Title:        "Delete the note?",
	Description:  "This cannot be undone.",
	Actions: []ui.AlertAction{
		{Text: "Cancel", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
		{Text: "Delete", Color: colors.Danger, Role: core.AlertDestructive, Default: true, OnPress: remove},
	},
}
```

An action with `Role: core.AlertCancel` runs on Escape or back, followed by `OnOpenChange(false, base.ReasonCancel)`. `Default: true` gives an action keyboard focus when the alert opens on desktops, the web and Apple TV, and makes Return choose it; an action press runs `OnPress`, then `OnOpenChange(false, base.ReasonSelect)`. Return runs the default action while the keyboard focus is on no other action or field: with the focus moved to another action, Return presses that one, and a dialog over the alert keeps Return. Once an action has run the alert closes, whatever `OnOpenChange` does, and runs no other until `Open` turns false and true again. If several actions carry the same marker, the first wins. Without markers, Escape closes without running an action and Return has no default action. `Role: core.AlertDestructive` describes an action; `Variant` or `Color` chooses its look.

For the system's own alert or action sheet, use `cx.ShowAlert` ([Platform services](../services.md#alerts-and-the-clipboard)).

## Behavior

A modal blocks the app behind it, takes the keyboard focus and closes on Escape and back unless told not to. On touch screens a dialog is a sheet that follows a downward drag and closes past a threshold.

## Accessibility

`ui.Dialog` reports `RoleDialog`, or its explicit `Role`, and `ui.AlertDialog` reports `RoleAlertDialog` on desktops and touch screens. Both read their title, or `Label` when the title is not enough, followed by the description. Focus moves into a dialog when it opens and returns when it closes; the backdrop is not a stop. The footer's buttons keep their roles and labels.

## Platform

On Android and touch screens a dialog is a sheet from the bottom; on desktops it is a centered surface with a backdrop. The content is the same; only the presentation changes.

## Related

- [Navigation](../navigation.md): pages, tabs and modals together.
- [Menu](menu.md): anchored commands and context menus.
- [Accessibility](../accessibility.md): focus and dialog roles.
- The gallery's Dialog, Sheet and AlertDialog pages.
- [Unstyled components](../authoring/base.md#dialog): `base.Dialog` and `base.Sheet`, with iOS's presentations and motion of your own.

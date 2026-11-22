---
title: Terminal
description: Run a program or show a remote session in a terminal grid.
group: Core concepts
order: 63
---

`base/terminal` provides a session (`Terminal`) and its `View`. Desktops run local programs; iOS uses `Options.Conn`, an `io.ReadWriteCloser` connected to a remote program. The app owns the session and closes it when finished.

```go
t, err := terminal.New(terminal.Options{CursorBlink: true})
if err != nil {
	return core.Text{Content: err.Error()}
}
cx.Cleanup(func() { t.Close() })
return terminal.View{
	Terminal: t, AutoFocus: true,
	MinimumContrast: 4.5, PaddingColor: terminal.PaddingExtend,
	Style: core.Style{FlexGrow: 1},
}
```

`Options.Cursor` chooses the default block, bar or underline. `CursorBlink` enables its default blinking; zero remains steady. Programs override shape and blinking through DECSCUSR or DEC mode 12. An unfocused cursor is hollow and steady; composition and reduced motion also keep it steady. The platform's animation clock drives blinking without engine redraws of the terminal text.

`View.Font`, `Theme` and `DarkTheme` set the font and palette. The grid draws Unicode box-drawing and block characters and Powerline separators U+E0B0–U+E0B7 itself, aligning adjoining cells to device pixels. Other text uses platform fonts and their fallbacks.

`MinimumContrast` raises ordinary text's contrast against its actual cell background, including inverse colors, faint text and selection. Values 1–21 use WCAG's contrast ratio; zero preserves colors. Graphics and emoji keep their colors. With `Transparent`, contrast still uses the terminal's configured background.

`Padding` leaves room outside the grid, in points. `PaddingColor` chooses its background:

| Value | Behavior |
| --- | --- |
| `PaddingBackground` | Terminal background, including OSC 11 changes |
| `PaddingExtend` | Nearest cell background; on the primary screen, suppress top/bottom extension beside prompt rows, Powerline rows or rows containing a default-background cell |
| `PaddingExtendAlways` | Extend every edge without those exceptions |

TSX apps import `tsx/terminal` for its effect to register `Terminal` in `nsgo/base`. The props are `cursorBlink`, `minimumContrast` and `paddingColor`; the latter accepts `background`, `extend` and `extend-always`. Appearance props can change while the session continues.

## Actions and zoom

`View.Keybinds` overrides defaults with `Keybind{Key, Action, Amount}`. Keys use the same names as `Scope.OnKey`, such as `mod+=` or `ctrl+shift+c`. Later entries win. Bindings run while this terminal's input has the focus and no input method is composing.

The actions are `CopyToClipboard`, `PasteFromClipboard`, `SelectAll`, `ClearScreen`, `IncreaseFontSize`, `DecreaseFontSize`, `ResetFontSize`, `SetFontSize` and `Ignore`. Window and tab actions belong in the app's keymap. `Ignore` consumes a key without sending it to the program.

By default, `mod+=` and `mod+shift+=` increase the font by one point, `mod+-` decreases it, and `mod+0` resets to `View.Font.Size` (13 when unset). `Amount` sets the step for increase/decrease, one when zero, or the size for `SetFontSize`. Sizes clamp to 1–255 points; nonfinite requests are ignored. Zoom keeps the session and outer view, updates the grid's rows and columns, and resizes the program.

```go
ref := &terminal.Ref{}
view := terminal.View{
	Terminal: session, Ref: ref,
	Keybinds: []terminal.Keybind{
		{Key: "ctrl+shift+k", Action: terminal.ClearScreen},
		{Key: "mod+=", Action: terminal.IncreaseFontSize, Amount: 2},
	},
}
```

`Ref.FontSize()` is tracked for controls that display it; `Ref.SetFontSize(size)` and `Ref.Perform(action, amount)` run on the app's engine, as event handlers do. `Perform` reports whether an action is available. An unmounted ref returns zero size and refuses actions.

## Context menu

A secondary click opens an unstyled `base.ContextMenu` with localized Copy, Paste, Select All and Clear items. Copy requires a selection. Clear removes content above the prompt and is disabled on the alternate screen. Touch screens use the context menu's long-press presentation. Style the surface with `MenuClass`/`MenuStyle` and the items with `MenuItemClass`/`MenuItemStyle`.

Set `RightClickAction` to `RightClickPaste`, `RightClickCopy`, `RightClickCopyOrPaste` (copy a selection, otherwise paste), or `RightClickIgnore`. Ignore leaves the event for an enclosing app menu; use it when supplying a styled `ui.ContextMenu` that calls the ref's actions.

Import `tsx/terminal` to register `<Terminal>` in `nsgo/base`. Its reactive `keybinds` and `rightClickAction` props update the view while keeping the session. Action values use Ghostty names such as `increase_font_size` and `copy_to_clipboard`; right-click values are `context-menu`, `paste`, `copy`, `copy-or-paste` and `ignore`. The menu styling props use the Go names in lower camel case.

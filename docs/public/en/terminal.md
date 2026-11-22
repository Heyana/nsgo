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

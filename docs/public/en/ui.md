---
title: Component reference
description: The most-used components, their variants, states and accessibility.
group: Components
order: 30
---

These are `ui`'s components, in NSGo's look and following the theme. This reference starts with the components an app reaches for first; each page starts with the shortest example, then covers its parts, states and accessibility. The gallery covers every component, including those without a page here yet.

The [gallery](https://nsgo.dev/gallery/) runs a demo of every component in the browser.

## Actions

- [Button](ui/button.md): a push button, plain or in the accent color, and the icon button.

## Text and input

- [Text](ui/text.md): paragraphs, spans, links, fonts and document text.
- [Input](ui/input.md): the platform's text field, controlled or not.
- [Checkbox](ui/checkbox.md): a check box, checked, unchecked or indeterminate.
- [Switch](ui/switch.md): a setting that is on or off.
- [Select](ui/select.md): one value from a list, as a popup or a sheet.
- [Slider](ui/slider.md): one value or a range along a track.

## Collections

- [List](ui/list.md): rows of any number, built only in view, with selection and activation.
- [Scroll](ui/scroll.md): the platform's scroll view, with paging, snapping, zoom and refresh.

## Navigation

- [Tabs](ui/tabs.md): a tab list, a segmented control and the system tab bar.

## Overlays

- [Dialog](ui/dialog.md): a modal surface, a sheet and the system's alert.
- [Menu](ui/menu.md): a menu from a trigger, a context menu and their items.

## More

The gallery covers the rest: tables, charts, the code editor, rich text, the terminal, the dock, the command palette, color, date and emoji pickers (`ui.EmojiPicker`), toasts and more. Each has a demo there.

`base/terminal.View` opens existing local paths with Command+click (Control+click on Windows and Linux), as it opens URLs. Relative paths use the shell’s OSC 7 working directory; a local shell that has not reported one uses `terminal.Options.Dir`, or the home directory when unset. Quote filenames containing spaces. Remote directory authorities and connections without a local directory report do not resolve against local files. Files and folders use `Scope.OpenFile`; URL links continue through `Scope.OpenURL` and the embedding host’s URL handler.

`base/terminal.View` denies program clipboard reads and writes by default. Set `AllowClipboardRead` and `AllowClipboardWrite` only when the app permits them; the requesting view’s `AccessClipboard` still applies. The same props are available on TSX’s `Terminal` and can change without restarting its session. Apps provide consent UI. Explicit user paste independently offers a text/PNG snapshot to programs using the Kitty clipboard protocol (mode 5522); its one-use password can read only that snapshot. Programs without the mode receive ordinary bracketed text. Clipboard image availability follows the platform’s clipboard services; the X11 primary selection is not exposed.

## Copyable components

Top-level `ui` components are imported directly from `github.com/nsgo-dev/nsgo/ui`. `ui`'s eight subpackage components are declared in NSGo's `nsgo-module.yaml` and listed in `nsgo add -list`, so an app can either import them or copy them into its own tree to edit directly:

- `chat` (`ui/chat`): conversation view, composer, turn actions and find bar.
- `code-editor` (`ui/editor`): code editor in `ui`'s look, styled over `base/editor`.
- `commit-graph` (`ui/commitgraph`): commit list beside a revision graph, on `base/commitgraph`.
- `diff-view` (`ui/diff`): diff view in `ui`'s look, styled over `base/diff`.
- `file-picker` (`ui/filepicker`): modal file and folder picker with sidebar and breadcrumbs.
- `flow-diagram` (`ui/flow`): directed flow diagram and cards, styled over `base/flow`.
- `rich-editor` (`ui/richeditor`): rich text editor in `ui`'s look, styled over `base/richeditor`.
- `terminal` (`ui/terminal`): terminal emulator view in `ui`'s look, on `base/terminal`.

See [Copying components](copying-components.md) for how `nsgo add` copies them, rewrites imports, generates catalogs and records their source.

## Next

- [Copying components](copying-components.md): copy `ui`'s subpackage components or third-party components into your app.

- [Components and state](components.md): how a component returns elements and how signals update them.
- [Styles and themes](styles.md): the theme components take their colors from.
- [Unstyled components](authoring/base.md): the same controls without a look, for a look of your own.
- [Accessibility](accessibility.md): labels, roles and values for every control.

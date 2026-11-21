---
title: Components and state
description: Elements, components, signals and styles.
group: Core concepts
order: 50
---

## Elements

The screen is a tree of elements, which are Go struct values. `core` has the primitives: `View`, `Text`, `Image`, `Scroll`, `Input` and more. `ui` has the controls an app uses, such as `Button`, `Select` and `DatePicker`, in a look that follows the theme. (`base` holds the same controls without a look, for [component authors](authoring/index.md).)

```go
core.View{
	Style: core.Style{Padding: core.All(24), Gap: 12},
	Children: []core.Node{
		core.Text{Content: "Hello"},
	},
}
```

`core.WebView` shows web content; an empty `WebSource` loads `about:blank` and reports the usual load and navigation events. Set `SourceFunc` to an empty source or call `Ref.Load(core.WebSource{})` to clear the previous page. Set `InjectedJavaScriptBeforeContentFunc` when the script before the page's own scripts follows a signal: when it changes, the view loads its source again with the new script, in place of the page it shows, even one the user or `Ref.Load` navigated to. Set `UserAgentSuffix` (TSX `userAgentSuffix`) to add a token, such as `Version/26.0 Safari/605.1.15`, after the platform's own user agent, or after `UserAgent` when that is set; on macOS and iOS a suffix to the platform's own takes effect only as the view is first made, and HarmonyOS and the web keep their own user agent. Attach a `core.Ref` to call `Reload`, `GoBack` and `GoForward`. On native targets these commands keep the app's own HTML and data sources exempt from `core.AccessWebSchemes`; pages that create new navigations still follow the access policy.

On Linux, `OnNavigationStateChange` also reports same-document URL, history and title changes.

In TSX, a change of `injectedJavaScriptBeforeContentLoaded` loads the source again with the new script in the same web view, as `InjectedJavaScriptBeforeContentFunc` does; a change of `injectedJavaScript` makes a new web view, which loads the source anew, as a change of `userAgent`, `userAgentSuffix` or `mobile` does.

Set `Mobile` (TSX `mobile`) to show a page as a phone sees it on Windows, macOS and Linux, for sites such as YouTube Shorts that switch videos with vertical swipes only on phones: the page gets a phone's user agent unless `UserAgent` is set and a touch screen without hover, and a drag with the mouse, or a wheel or trackpad scroll over what does not scroll, reaches it as a finger's swipe. Windows emulates a phone as Chrome's device mode does. macOS and Linux do it with a script, so their pages' stylesheets still see a mouse, and the touch events are untrusted, which a page may ignore. iOS, Android, HarmonyOS, the web and mini programs show pages as they are.

`core.Video` plays a file or URL in the platform's player. A transparent clip shows what is behind it as HEVC with alpha in a `.mov` on macOS (iOS is not yet checked), and as VP9 with alpha in WebM on Linux with GStreamer's codecalpha plug-in (`gstreamer1.0-plugins-bad`, which a Linux package can recommend in `nsgo.yaml`). On macOS, iOS, Windows and Linux, a local clip whose alpha cannot be kept (WebM on Apple platforms, HEVC on Linux, and any on Windows until Media Foundation's alpha output has been checked on a Windows machine) makes `OnError` say so instead of the video showing in an opaque box: the error is a `*core.MediaError` whose `Reason` is `core.MediaErrorAlphaUnsupported`, so the app can decode that clip itself instead. Android and the web, and remote URLs on every platform, play such clips as their decoder outputs them, which may be opaque.

## Components

A component is a function that takes a scope and returns an element. It runs once; what changes later is driven by signals.

```go
func Greeting(cx *core.Scope) core.Node {
	return core.Text{Content: "Hello from NSGo"}
}
```

A component with props is a type with a `Render` method:

```go
type Badge struct{ Text string }

func (b Badge) Render(*core.Scope) core.Node {
	return core.Text{Content: b.Text}
}
```

`ui.ToastContent` can omit its close button with `NoClose`, add a `Trailing` node, and dismiss the toast through `OnPress`. `Toasts.NoClose` hides close buttons across the stack.

`ui.Toasts.Style` and `StyleFunc` customize every toast surface, including its background, border, corners and padding. The stack's style merges over the theme, followed by each `ToastContent.Style` and `StyleFunc`. Each `StyleFunc` runs after its corresponding `Style` and updates when its signals change. A `Color` in these styles colors the title, and the description at reduced opacity, so text stays legible on a dark or colored surface. The entry, exit and swipe phases still apply their motion styles. In TSX, set the `style` prop of `Toasts` and pass `style` or `styleFunc` to `show` for one toast.

## Signals

A signal holds a value. Reading it inside a function an element calls, such as `ContentFunc`, subscribes that part of the screen, and only that part updates when the value changes.

```go
count := core.NewSignal(cx, 0)

core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }}

ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }}
```

A signal of a whole slice updates every row that reads it. `core.NewCollection` holds items by key, each in a signal of its own, so a row follows its own item and changing one item runs one row:

```go
todos := core.NewCollection(cx, func(t Todo) int { return t.ID }, nil)

core.List{Count: todos.Len, Key: func(i int) any { return todos.KeyAt(i) },
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		return core.Text{ContentFunc: func() string { t, _ := todos.At(row.Index()); return t.Title }}
	}}

todos.Update(id, func(t Todo) Todo { t.Done = true; return t })
```

Other state is built on signals the same way:

- `core.NewSelector` tells whether a key is the selected one, and runs only the rows a selection leaves and reaches.
- `core.NewHistory` keeps a value with undo and redo, its `CanUndo` and `CanRedo` signals for an Edit menu.
- `core.NewPersisted` keeps a typed value in the app's storage as versioned JSON, migrating what older versions wrote.
- `core.From` follows a source outside the graph, such as a subscription that delivers on another goroutine.

## Styles

`core.Style` follows CSS: flexbox and grid, paddings and margins, corners, borders, shadows, gradients, transforms and states such as `Active`. Elements also take Tailwind classes through `Class`:

```go
core.Text{Content: "Done", Class: "self-start rounded-full bg-zinc-100 px-3 py-1 text-sm"}
```

Themes give `ui`'s components their look in light and dark.

## Desktop apps

`nsgo.Run(Root)` shows one root view. A desktop app can open what it shows itself instead: `nsgo.Run(App)` runs `func App(app *core.Scope)` once, and it opens windows, panels, tray icons and global shortcuts with `desktop`, which share its signals. This works on macOS, Windows and Linux; on macOS `examples/tray` keeps out of the Dock and lives in the menu bar. `nsgo.Run(App, nsgo.StartHidden())` opens the main window without showing it, so a tray app, or one the system launched at login (`desktop.LaunchedAtLogin`), does not flash it; the app shows it with `app.MainWindow().Show()`. Phones and the web show the app as they launch it.

`desktop.SetWindowLevel` keeps a window above the others, as a timer or a desktop pet does, or below them, and `desktop.SetWindowFrame` moves and sizes it. `desktop.CanSetWindowLevel` and `desktop.CanPlaceWindows` say whether they take effect in a window's scope: both do on macOS and Windows, only the level on Linux under X11, and neither under Wayland, whose compositor stacks and places windows itself.

A window opened with `desktop.WindowOptions{Transparent: true}`, or the main window with `nsgo.WindowTransparent()`, has no background or frame, so the desktop shows wherever the app draws nothing, as for a desktop pet or a widget; a view with `Desktop.WindowDrag` moves it. `NoShadow` drops the system's shadow and `SkipTaskbar` keeps the window off the Windows taskbar and Alt+Tab and the X11 task list, and `desktop.SetWindowTransparent`, `SetWindowShadow` and `SetSkipTaskbar` change them later. Windows changes transparency only in a window opened transparent, which has no shadow, and Wayland has no hint to keep a window off its task list. `desktop.CanMakeWindowsTransparent` and `desktop.CanSkipTaskbar` say in a window's scope whether these take effect: transparency needs DirectComposition on Windows and a compositor on Linux, as X11 without one has none, and on macOS `desktop.SetDockVisible` takes the app off the Dock.

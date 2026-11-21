# Writing reusable components

These are the rules NSGo's own `base` and `ui` follow, for components other apps import: a library, a design system or one complex view. An app's own screens may relax them. The [Writing components](https://nsgo.dev/docs/authoring/index.md) pages hold the full examples.

## Behavior and look

- Keep each control's behavior, state and accessibility in an unstyled form that takes every style from its user part by part, as `base.Switch` does, and put looks in a styled form that wraps it, as `ui.Switch` does. What the styled form needs that the unstyled one lacks goes into the unstyled one, never into a second implementation.
- Build on NSGo's `base` control when one exists instead of writing presses, focus and keyboard handling again; give it a look as `ui` does. `Ref.Focus` and a button's `AutoFocus` also focus pressables on Android and iOS/tvOS for a hardware keyboard. Space and Return preserve modifiers after the keymap; touches do not move that focus. See [Keyboard focus](https://nsgo.dev/docs/accessibility.md#focus-and-the-keyboard).
- Transfer app values between views with `base/dragdrop.Area`, `dragdrop.Source` and `dragdrop.Target`; supply the preview and keep a menu or accessible action for the same operation. `Data` is sampled once and may return a slice for a selection. Test with `App.DragTo` or TSX `app.dragTo`, with both views visible. [Drag and drop](https://nsgo.dev/docs/drag-drop.md)
- A component with no behavior worth sharing, such as a badge, can be styled alone.
- Keep components product-neutral: what only one app uses stays in that app.

## API

- A component is a type with a `Render` method whose fields' zero values are the usual case. Callbacks are optional: a nil `OnChange` shows a value without changing it.
- Controlled values come in as functions and leave through callbacks (`On func() bool`, `OnChange func(bool)`), read where they are shown so only that part updates.
- Ask for `core.PermissionMotion` from a press handler on mobile Safari before starting `WatchMotion`. See [Location and motion](https://nsgo.dev/docs/services.md#location-and-motion).
- Relational classes style native element trees on every target: `has-[>svg]:px-3` tests direct icon children, `[&_svg]:size-4 [&_svg]:shrink-0` styles descendant `core.Icon`/`SVGView` nodes, and conditional regions are transparent. `hover:[&_svg]` reads ancestor hover; `[&_svg]:hover` reads each icon. Inline styles win unless the class is important. Use the [styles reference](https://nsgo.dev/docs/styles.md) for the supported selector subset.
- Each part takes a `Class`, a `Style` and a `StyleFunc` named after it (`DotClass`, `DotStyle`, `DotStyleFunc`). Parts take the control's state through `PartStates`, so `checked:` classes apply while screen readers see one control.
- For sibling parts that follow a control, mark its view with `peer` and use `peer-checked:`, `peer-disabled:` or `peer-data-invalid:` on later siblings. Use `peer/field` with `peer-checked/field:` for named sources. A peer styles only the siblings after it under the same parent, never earlier siblings or the views inside it. A marker is not made a Tab stop, so `peer-active:` follows only a marker that takes presses. Likewise `active:`, `Style.Active`, `group` and `group-active:` never make a plain view pressable or a Tab stop: give the view or its group a press handler for an active look. Spans and `Screen.HeaderClass` have no siblings: put a span's peer variant on its `Text`.
- The root says what the control is: `Role`, `Label` when its text does not, `Hint`, and `Checked`, `Selected`, `Disabled` or `Expanded`. Pass a root view's `OnLayout`, `Tooltip` and `TestID` through.
- Pass `Ref` and `LabelledBy` to the actual control or input. Use `Text.LabelFor` or `View.LabelFor` for a visible label that focuses or activates it and follows its disabled/invalid states; `LabelledBy` names from refs in order, ahead of `Label`. In TSX, use node signals with callback refs when a label precedes its target ([labels](https://nsgo.dev/docs/accessibility.md#visible-labels)).
- Looks come from `Variant`, a type of its own (`ToggleVariant`) whose zero value is the default, never boolean fields or a shared color type. Keep one size; a component that needs more takes `Size`, a `ui.Size` whose zero value is the default, never `Small` or `Compact` booleans. One that is one shape at any scale, such as an avatar, takes `Size` in points.

## Look

- Apple texts, canvases and shaders keep their own shadows and outlines outside `Overflow: core.Hidden`; [Layout](https://nsgo.dev/docs/layout.md#position) lists the remaining native-content limits.
- Take every color from the theme inside a `StyleFunc` (`ui.ColorsOf(cx)`, `cx.Palette()`), so theme and appearance changes restyle in place. A color with no role goes in `Palette.Colors` under a name of yours, set for light and dark.
- Do nothing implicitly by device: no size, form or behavior that changes on its own on a phone, a desktop or a TV. Make it a field the app sets.
- On macOS and iOS, `Overflow: core.Hidden` keeps outer shadows and outlines on inputs, web views, videos, native components, images and scroll views, including differing corner radii; zooming scroll content keeps the viewport radius fixed. See [layout](https://nsgo.dev/docs/layout.md) for the remaining clip limits and inner-view workaround.
- A component shows the same on every platform. Imitate what one platform has natively, such as iOS's Liquid Glass, rather than dropping it elsewhere.
- Lists, trees, tables and sidebars have a default and a selected look, no hover or press; only rows of a popup that drops down, such as a menu or a select, follow the pointer.
- Leave the pointer's arrow alone; `core.Style.Cursor` is only for what the arrow cannot say, such as text fields and resize drags, and `core.Style.CursorImage` (`core.CustomCursor`) is for image-based cursors on desktops and the web (`core.Env.SupportsCustomCursor`).
- Keep a scroll bar against its container's edge: the scroll view fills the container and the padding goes on its content. Scroll through `core.Scroll` or `core.List`, never by stepping offsets. Put a footer inside its scroll view with `Position: core.Sticky, Bottom: core.Pt(0)` to pin it at the end while keeping short content in flow (`Right` horizontally); leave `Top` (`Left`) auto to use that end.

## Files and packages

- Ship data files under `resources/` and read them with `cx.ReadAsset` from Go. The callback runs on the engine when the bytes are ready. See [Bundled assets](https://nsgo.dev/docs/services.md#bundled-assets).
- One component to a file, named after it (`iconbutton.go`), with the types only it uses; what several share gets a file of its own.
- A complex component gets a package of its own whose API drops its name (`dock.Area`, not `dock.DockArea`).
- Keep unstyled and styled forms in separate packages when both exist, so apps with their own look import no styles.

On HarmonyOS, use Go’s standard file APIs under `cx.DataDir()`, `cx.CacheDir()` or the system’s temporary directory. See [Services](https://nsgo.dev/docs/services.md#storage).

## Words and tests

- Say the component's own words, such as an icon button's label, through the package's `locales/` catalogs in English and the languages you translate to, never string literals. Add `//go:generate go tool nsgo i18n` to the package and commit the generated `catalog_gen.go`, so apps build with Go alone.
- Test with `nsgotest` by role and text, in both appearances (`nsgotest.Environment`), look at it under `go tool nsgo dev -lang en-XA` for cut and untranslated words, and run `go tool nsgo check -target all` to vet every available Go/Xcode target. It lists unavailable Apple SDKs it skips; explicitly naming an Apple target keeps it required. In NSGo's repository target checks include the `nsgotest` hosts; name `harmonyos` explicitly to include its preview backend.
- Register Go components for TSX in an `init` function with `tsx.Register("example.com/chips", ...)`.

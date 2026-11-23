---
title: Styles and themes
description: Typed styles, Tailwind classes, state styles, and light and dark themes.
group: Core concepts
order: 52
---

## A style

`core.Style` is one Go value for a box's layout, look and text. Every field's zero value is unset, so styles compose: `Merge` replaces only what the later style sets, and a child inherits the text fields its parents set.

```go
card := core.Style{
	Background: core.RGB(0xffffff),
	CornerRadius: 12,
	Padding: core.All(16),
	Gap: 12,
	BorderWidth: 1,
	BorderColor: core.RGBA(0x000000, 0.1),
}
```

Styles take CSS's colors, corners, borders, outlines, shadows, gradients, filters, backdrop blurs, masks, blend modes, 3D transforms and transitions. `core.RGB`, `core.RGBA` and `core.ParseColor` make colors, `core.LinearGradient` and its kin make gradients, `core.Ring` makes Tailwind's ring, and `core.AllCorners` gives each corner its radius. `BorderCurveContinuous` rounds corners into their edges as iOS does. A box's shadows and outline stay outside its `Overflow` clip except where [Layout](layout.md#position) says. Backdrop filters change what is behind a view as CSS's `backdrop-filter` does: a `Blur` blurs it by its radius and the color filters change its colors. iOS draws the system material `View.IOS.BackdropMaterial` names for a blur without color filters; a blur naming a material other than `core.BackdropUltraThin` draws that material on iOS and AppKit's closest one on macOS, leaving color filters out. Linux and Android draw no backdrop filters. `Style.GlassEffect` (`core.GlassRegular` or `core.GlassClear`, `glassEffect` in TSX) draws Liquid Glass in place of a backdrop blur on macOS 26 and iOS 26 (and NSGo's glass shader on Windows; give a glass view a blur to fall back to elsewhere). Setting `Style.GlassInteractive` (`glassInteractive` in TSX) enables the system's interactive press response for Liquid Glass on macOS 27 (`NSGlassEffectView.effectIsInteractive`) and iOS 26 (`UIGlassEffect.isInteractive`) while keeping `OnPress` and pressable children working.

`Transform` moves the pointer target with the view: presses, hover and gestures begin where it is drawn, and reported pointer positions undo the transform. A transformed child outside its parent remains reachable unless a clip or scroll viewport hides it. Mini programs do not yet undo keyframes their render layer runs ([roadmap](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md#mini-programs)).

```go
core.View{Style: core.Style{
	CornerRadii: core.Corners{TopLeft: 28, BottomRight: 28},
	BorderCurve: core.BorderCurveContinuous,
	Shadows:     []core.BoxShadow{core.Ring(1, core.RGBA(0x000000, 0.08)), ui.Shadow(0, 8, 24, -6, ui.Black(0.18))},
}}
```

## States

A view's look can change with its state. `Hover`, `Focus`, `FocusWithin`, `Active`, `Selected`, `Checked`, `Disabled` and the `aria-*` states are `*Style` values that merge over the rest while the state holds. An app sets `Selected`, `Checked`, `Disabled` and `Expanded` through functions, and the platform reports presses and focus.

`Active` and `group-active:` follow a view while it is pressed, which needs a press handler or a `Responder` holding the touch. Styling a plain view with `Active`, `active:` or `group` does not make it pressable or a keyboard Tab stop; set `Focusable` when it should take focus itself.

```go
core.View{
	OnPress: func(core.PressEvent) { open() },
	Style: core.Style{
		Background: core.RGB(0x007aff),
		Active:     &core.Style{Opacity: core.Alpha(0.85)},
		Hover:      &core.Style{Background: core.RGB(0x006be0)},
	},
	Disabled: func() bool { return saving },
}
```

A part of a component can take the states of the control around it without reporting them itself: `PartStates` gives `Checked`, `Selected`, `Disabled` and the rest, so a checkbox's mark follows the checkbox for screen readers too.

## Classes

`Class` on `View`, `Text`, `Span`, `Input`, `Image`, `Scroll` and `Canvas` takes Tailwind CSS 4 classes, and every part of a `base` component has a class beside its style. Utilities, variants and arbitrary values mean what they mean in Tailwind, on its default theme and palette.

```go
core.View{Class: "flex items-center gap-3 rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"}
```

A class merges under the style, so the style's fields win. Classes can follow what changes with `ClassFunc`, and a state variant (`hover:bg-zinc-100`, `checked:opacity-100`) becomes the style's state. `group` and `group-hover:` style children from a parent's state, `first:` and `last:` follow a child's place, and `aria-checked:`, `data-open:` and `focus-within:` follow the states a control shows. `pointer-fine:` and `pointer-coarse:` style a subtree for a mouse or a touch screen, matching CSS's `pointer` media feature via `cx.Env().Pointer()` (the browser's `pointer` query on the web, fine on desktops and Macs, coarse on phones, tablets and watches, and neither on TVs), and compose with other variants such as `dark:` and `hover:` so the app chooses touch or pointer sizing explicitly rather than `core` changing it implicitly.

Mark a control with `peer` to style later siblings from its state, for example a checkbox with `Class: "peer"` followed by a label with `Class: "peer-checked:text-accent peer-disabled:opacity-50"`. A peer styles only the siblings after it under the same parent, `Show` and `Each` included, never earlier siblings or the views inside it. Use `peer/field` with `peer-checked/field:` to name one set, and `peer-data-invalid:` for a control's boolean `invalid` data state. Markers and states update reactively. Marking a view does not make it a Tab stop or take presses, so `peer-active:` follows only a peer that takes presses itself, such as a button.

`has-[>svg]:px-3` changes a parent's padding when it has a direct icon child; `[&_svg]:size-4 [&_svg]:shrink-0` sizes its descendant icons. `svg` selects `core.Icon` and `SVGView`, including through conditional regions. These selectors update when children, classes or states change. `hover:[&_svg]` follows the parent's hover; `[&_svg]:hover` follows each icon's. A selector's `active` follows only views that take presses, such as a button, as `peer-active:` does. Matching selectors compete by specificity before variant order; inline styles win unless the class is important.

Selectors match NSGo elements on every target. They support lists, child (`>`), descendant (`_`), adjacent (`+`) and following sibling (`~`) combinators, classes, `data-*` presence/equality, roles, supported boolean accessibility states and `:is`, `:where`, `:not`, `:has` and child positions. Types are `view`/`div`, `text`/`span` (whole text nodes), `scroll`, `input`, `textarea`, `image`/`img`, `canvas`, `svg`, `video`, `webview` and `native`. Use one top-level `&` in each arbitrary selector alternative; without it, the selector tests the element itself. Browser-only selectors, pseudo-elements, at-rules, escapes and explicit `&` inside a function are unknown; these selectors do not reach backend implementation views or individual text spans.

`peer-*` variants in `Span.Class` or `Screen.HeaderClass` have no siblings to follow. They are ignored and logged once per class and context; put a span's peer variant on its containing `Text` instead.

The theme's `Tailwind` field extends the default theme with colors, breakpoints and utilities, and a color the palette lacks names a `Palette` role (`bg-accent`, `text-text-muted/70`) or a theme color, read from the node's theme in the appearance it shows.

## Themes

A `core.Theme` is one look in both appearances: a `Palette` for light and one for dark, a corner radius and font families. The palette's roles are `Background`, `Surface`, `Text`, `TextMuted`, `Border`, `Accent`, `AccentText`, `Danger`, `Warning`, `Success`, `Selection`, `Focus` and `Scrollbar`; `Colors` holds the ones a library adds, such as `inset` and `raised`. `core.DefaultTheme()`, which an app shows until it sets another, is the neutral theme with a blue accent and `ui`'s roles, and `ui.WarmDuskTheme()` a warm palette.

```go
theme := core.DefaultTheme()
theme.Light.Accent, theme.Dark.Accent = core.RGB(0xbd481b), core.RGB(0xed986e)
theme.Radius = 10
cx.SetTheme(theme)
```

`cx.SetTheme` gives a theme to the scope and everything built under it, so a sidebar or a panel can take a theme of its own; a nil theme takes the one above. `core.ParseTheme` reads a YAML or JSON theme file, or a shadcn/ui theme file's CSS variables, with role colors, a radius and font families. `core.DefaultTheme`, the theme an app shows until it sets one, and `ui.WarmDuskTheme` return themes ready to modify.

`cx.SetAppearance` forces the whole app light or dark, `cx.SetSubtreeAppearance` does it for a subtree, and `core.AppearanceSystem` follows the system again. `cx.Dark()` says which appearance a subtree shows, and `cx.Palette()` gives its colors with the embedding host's overrides applied.

When no ancestor sets `Style.Color`, text and `currentColor` icons use the subtree's `Palette.Text`, following theme, appearance and host token changes in place. An explicit color, including `core.Transparent`, keeps inheriting.

## Colors in components

A component reads its colors where it is built and in its `StyleFunc`s, so a theme or appearance change follows in place, without building the component again. `ui.ColorsOf(cx)` names the roles `ui` uses, `ui.TintsOf(cx)` gives their translucent fills, and `ui.FinOf(cx)` gives `ui`'s layered finishes.

```go
colors := ui.ColorsOf(cx)
core.Text{Content: "Hello", StyleFunc: func() core.Style {
	return core.Style{Color: colors.Text(), FontSize: 15}
}}
```

`StyleFunc` runs again when what it read changes, so `cx.Dark()` and `cx.Palette()` inside it follow the theme. The same applies to `ClassFunc`, a span's `StyleFunc` and a component's `StyleFunc` fields.

## Cursors

`Style.Cursor` sets a named pointer cursor (`core.CursorText`, `core.CursorColResize`, and so on); leave pressables on `core.CursorAuto` so they keep the platform's arrow. For drawing tools, games or custom canvas pointers on desktops and the web, `Style.CursorImage` takes a `*core.CustomCursor` with a 1x `Image`, an optional HiDPI `Image2x` (picked automatically at `PixelRatio >= 1.5`), a logical-point `Hotspot` clamped to image bounds, and a `Fallback` named cursor. Check `cx.Env().SupportsCustomCursor()` to see whether the current target displays custom cursor images; touch platforms ignore them.

## Next

- [Layout](layout.md): where boxes go.
- [Text](text.md): fonts, spans and decorations.
- [Animation](animation.md): transitions between styles.

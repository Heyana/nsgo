---
title: Styles und Themes
description: Typisierte Styles, Tailwind-Klassen, Zustands-Styles sowie helle und dunkle Themes.
group: Kernkonzepte
order: 52
---

## Ein Style

`core.Style` ist ein einzelner Go-Wert für Layout, Aussehen und Text einer Box. Der Nullwert jedes Felds bedeutet „nicht gesetzt“, deshalb lassen sich Styles kombinieren: `Merge` ersetzt nur, was der spätere Style setzt, und ein Kind erbt die Textfelder, die seine Eltern setzen. Um eine Länge über einem Style, der eine vorgibt, auf nichts zu setzen, verwenden Sie `core.Zero` in `Padding`, `Margin`, `Borders`, `BorderWidth`, `CornerRadius` oder einem Abstand, so wie `core.Transparent` es für eine Farbe tut: `Padding: core.All(core.Zero)`.

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

Styles übernehmen die Farben, Ecken, Rahmen, Outlines, Schatten, Verläufe, Filter, Backdrop-Blurs, Masken, Blend-Modi, 3D-Transformationen und Transitions von CSS. `core.RGB`, `core.RGBA` und `core.ParseColor` erzeugen Farben, `core.LinearGradient` und seine Verwandten erzeugen Verläufe, `core.Ring` erzeugt den Ring von Tailwind, und `core.AllCorners` gibt jeder Ecke ihren eigenen Radius. `BorderCurveContinuous` rundet Ecken wie iOS fließend in ihre Kanten ein. Schatten und Outline einer Box bleiben außerhalb ihres `Overflow`-Clippings, außer wo [Layout](layout.md#position) es anders angibt. Backdrop-Filter verändern, was hinter einer View liegt, so wie `backdrop-filter` in CSS: Ein `Blur` zeichnet es mit seinem Radius weich, und die Farbfilter verändern seine Farben. iOS zeichnet für einen Blur ohne Farbfilter das Systemmaterial, das `View.IOS.BackdropMaterial` benennt; ein Blur, der ein anderes Material als `core.BackdropUltraThin` benennt, zeichnet unter iOS dieses Material und unter macOS das nächstliegende Material von AppKit und lässt Farbfilter weg. Linux und Android zeichnen keine Backdrop-Filter. `Style.GlassEffect` (`core.GlassRegular` oder `core.GlassClear`, in TSX `glassEffect`) zeichnet unter macOS 26 und iOS 26 Liquid Glass anstelle eines Backdrop-Blurs (und unter Windows den Glas-Shader von NSGo; geben Sie einer Glas-View einen Blur als Rückfallebene für andere Plattformen). Das Setzen von `Style.GlassInteractive` (in TSX `glassInteractive`) aktiviert unter macOS 27 (`NSGlassEffectView.effectIsInteractive`) und iOS 26 (`UIGlassEffect.isInteractive`) die interaktive Druckreaktion des Systems für Liquid Glass, während `OnPress` und drückbare Kinder weiterhin funktionieren.

`Transform` verschiebt das Zeigerziel mit der View: Drücke, Hover und Gesten beginnen dort, wo sie gezeichnet wird, und gemeldete Zeigerpositionen machen die Transformation rückgängig. Ein transformiertes Kind außerhalb seines Elternelements bleibt erreichbar, sofern kein Clipping oder Scroll-Viewport es verbirgt. Mini-Programme machen Keyframes, die ihre Render-Schicht ausführt, noch nicht rückgängig ([Roadmap](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md#mini-programs)).

```go
core.View{Style: core.Style{
	CornerRadii: core.Corners{TopLeft: 28, BottomRight: 28},
	BorderCurve: core.BorderCurveContinuous,
	Shadows:     []core.BoxShadow{core.Ring(1, core.RGBA(0x000000, 0.08)), ui.Shadow(0, 8, 24, -6, ui.Black(0.18))},
}}
```

## Zustände

Das Aussehen einer View kann sich mit ihrem Zustand ändern. `Hover`, `Focus`, `FocusWithin`, `Active`, `Selected`, `Checked`, `Disabled` und die `aria-*`-Zustände sind `*Style`-Werte, die über den Rest gemergt werden, solange der Zustand besteht. Eine App setzt `Selected`, `Checked`, `Disabled` und `Expanded` über Funktionen, und die Plattform meldet Drücke und Fokus.

`Active` und `group-active:` folgen einer View, solange sie gedrückt ist; dafür braucht sie einen Press-Handler oder einen `Responder`, der die Berührung hält. Eine einfache View mit `Active`, `active:` oder `group` zu stylen macht sie weder drückbar noch zu einem Tab-Stopp der Tastatur; setzen Sie `Focusable`, wenn sie selbst den Fokus erhalten soll.

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

Ein Teil einer Komponente kann die Zustände des umgebenden Steuerelements übernehmen, ohne sie selbst zu melden: `PartStates` liefert `Checked`, `Selected`, `Disabled` und die übrigen, sodass das Häkchen einer Checkbox auch für Screenreader der Checkbox folgt.

## Klassen

`Class` an `View`, `Text`, `Span`, `Input`, `Image`, `Scroll` und `Canvas` nimmt Tailwind-CSS-4-Klassen entgegen, und jeder Teil einer `base`-Komponente hat neben seinem Style eine Klasse. Utilities, Varianten und beliebige Werte bedeuten dasselbe wie in Tailwind, mit dessen Standard-Theme und -Palette.

```go
core.View{Class: "flex items-center gap-3 rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"}
```

Eine Klasse wird unter den Style gemergt, sodass die Felder des Styles Vorrang haben. Mit `ClassFunc` können Klassen Änderungen folgen, und eine Zustandsvariante (`hover:bg-zinc-100`, `checked:opacity-100`) wird zum Zustand des Styles. `group` und `group-hover:` stylen Kinder anhand des Zustands eines Elternelements, `first:` und `last:` folgen der Position eines Kindes, und `aria-checked:`, `data-open:` und `focus-within:` folgen den Zuständen, die ein Steuerelement anzeigt. `pointer-fine:` und `pointer-coarse:` stylen einen Teilbaum für eine Maus oder einen Touchscreen, entsprechend dem Media-Feature `pointer` von CSS über `cx.Env().Pointer()` (im Web die `pointer`-Abfrage des Browsers, fein auf Desktops und Macs, grob auf Telefonen, Tablets und Uhren und keines von beiden auf Fernsehern), und lassen sich mit anderen Varianten wie `dark:` und `hover:` kombinieren, sodass die App ausdrücklich zwischen Touch- und Zeigergrößen wählt, statt dass `core` dies implizit ändert.

Markieren Sie ein Steuerelement mit `peer`, um spätere Geschwister anhand seines Zustands zu stylen, zum Beispiel eine Checkbox mit `Class: "peer"`, gefolgt von einem Label mit `Class: "peer-checked:text-accent peer-disabled:opacity-50"`. Ein Peer stylt nur die Geschwister nach ihm unter demselben Elternelement, `Show` und `Each` eingeschlossen, niemals frühere Geschwister oder die Views in ihm. Verwenden Sie `peer/field` mit `peer-checked/field:`, um eine Gruppe zu benennen, und `peer-data-invalid:` für den booleschen Datenzustand `invalid` eines Steuerelements. Markierungen und Zustände werden reaktiv aktualisiert. Eine View zu markieren macht sie weder zu einem Tab-Stopp noch nimmt sie dadurch Drücke an, deshalb folgt `peer-active:` nur einem Peer, der selbst Drücke annimmt, etwa einem Button.

`has-[>svg]:px-3` ändert das Padding eines Elternelements, wenn es ein direktes Icon-Kind hat; `[&_svg]:size-4 [&_svg]:shrink-0` legt die Größe seiner untergeordneten Icons fest. `svg` wählt `core.Icon` und `SVGView` aus, auch über bedingte Bereiche hinweg. Diese Selektoren werden aktualisiert, wenn sich Kinder, Klassen oder Zustände ändern. `hover:[&_svg]` folgt dem Hover des Elternelements; `[&_svg]:hover` folgt dem jedes einzelnen Icons. Das `active` eines Selektors folgt nur Views, die Drücke annehmen, etwa einem Button, so wie `peer-active:`. Passende Selektoren konkurrieren zuerst über die Spezifität und dann über die Reihenfolge der Varianten; Inline-Styles haben Vorrang, sofern die Klasse nicht als wichtig markiert ist.

Selektoren passen auf jeder Zielplattform auf NSGo-Elemente. Sie unterstützen Listen, die Kombinatoren für Kind (`>`), Nachfahre (`_`), direkt folgendes (`+`) und nachfolgendes Geschwister (`~`), Klassen, Vorhandensein und Gleichheit von `data-*`, Rollen, unterstützte boolesche Barrierefreiheitszustände sowie `:is`, `:where`, `:not`, `:has` und Kindpositionen. Typen sind `view`/`div`, `text`/`span` (ganze Textknoten), `scroll`, `input`, `textarea`, `image`/`img`, `canvas`, `svg`, `video`, `webview` und `native`. Verwenden Sie in jeder Alternative eines beliebigen Selektors ein einzelnes `&` auf oberster Ebene; ohne es prüft der Selektor das Element selbst. Reine Browser-Selektoren, Pseudoelemente, At-Rules, Escapes und ein explizites `&` innerhalb einer Funktion sind unbekannt; diese Selektoren erreichen weder Implementierungs-Views des Backends noch einzelne Text-Spans.

`peer-*`-Varianten in `Span.Class` oder `Screen.HeaderClass` haben keine Geschwister, denen sie folgen könnten. Sie werden ignoriert und einmal pro Klasse und Kontext protokolliert; setzen Sie die Peer-Variante eines Spans stattdessen auf den umgebenden `Text`.

Das Feld `Tailwind` des Themes erweitert das Standard-Theme um Farben, Breakpoints und Utilities, und eine Farbe, die der Palette fehlt, benennt eine `Palette`-Rolle (`bg-accent`, `text-text-muted/70`) oder eine Theme-Farbe, die aus dem Theme des Knotens in dem Erscheinungsbild gelesen wird, das er zeigt.

## Themes

Ein `core.Theme` ist ein Aussehen in beiden Erscheinungsbildern: eine `Palette` für hell und eine für dunkel, ein Eckenradius und Schriftfamilien. Die Rollen der Palette sind `Background`, `Surface`, `Text`, `TextMuted`, `Border`, `Accent`, `AccentText`, `Danger`, `Warning`, `Success`, `Selection`, `Focus` und `Scrollbar`; `Colors` enthält die, die eine Bibliothek hinzufügt, etwa `inset` und `raised`. `core.DefaultTheme()`, das eine App zeigt, bis sie ein anderes setzt, ist das neutrale Theme mit blauem Akzent und den Rollen von `ui`, und `ui.WarmDuskTheme()` ist eine warme Palette.

```go
theme := core.DefaultTheme()
theme.Light.Accent, theme.Dark.Accent = core.RGB(0xbd481b), core.RGB(0xed986e)
theme.Radius = 10
cx.SetTheme(theme)
```

`cx.SetTheme` gibt dem Scope und allem, was darunter gebaut wird, ein Theme, sodass eine Seitenleiste oder ein Panel ein eigenes Theme erhalten kann; ein nil-Theme übernimmt das darüberliegende. `core.ParseTheme` liest eine Theme-Datei in YAML oder JSON oder die CSS-Variablen einer shadcn/ui-Theme-Datei, mit Rollenfarben, einem Radius und Schriftfamilien. `core.DefaultTheme`, das Theme, das eine App zeigt, bis sie eines setzt, und `ui.WarmDuskTheme` geben Themes zurück, die sich direkt anpassen lassen.

`cx.SetAppearance` erzwingt für die ganze App hell oder dunkel, `cx.SetSubtreeAppearance` tut dies für einen Teilbaum, und `core.AppearanceSystem` folgt wieder dem System. `cx.Dark()` gibt an, welches Erscheinungsbild ein Teilbaum zeigt, und `cx.Palette()` liefert seine Farben mit angewendeten Überschreibungen des einbettenden Hosts.

Wenn kein Vorfahre `Style.Color` setzt, verwenden Text und `currentColor`-Icons das `Palette.Text` des Teilbaums und folgen Änderungen an Theme, Erscheinungsbild und Host-Tokens direkt. Eine explizite Farbe, einschließlich `core.Transparent`, wird weiterhin vererbt.

## Farben in Komponenten

Eine Komponente liest ihre Farben dort, wo sie gebaut wird, und in ihren `StyleFunc`s, sodass sie einer Änderung des Themes oder Erscheinungsbilds direkt folgt, ohne die Komponente neu zu bauen. `ui.ColorsOf(cx)` benennt die Rollen, die `ui` verwendet, `ui.TintsOf(cx)` liefert ihre durchscheinenden Füllungen, und `ui.FinOf(cx)` liefert die geschichteten Oberflächen von `ui`.

```go
colors := ui.ColorsOf(cx)
core.Text{Content: "Hello", StyleFunc: func() core.Style {
	return core.Style{Color: colors.Text(), FontSize: 15}
}}
```

`StyleFunc` läuft erneut, wenn sich etwas ändert, das es gelesen hat, sodass `cx.Dark()` und `cx.Palette()` darin dem Theme folgen. Dasselbe gilt für `ClassFunc`, die `StyleFunc` eines Spans und die `StyleFunc`-Felder einer Komponente.

## Cursor

`Style.Cursor` setzt einen benannten Mauszeiger (`core.CursorText`, `core.CursorColResize` usw.); belassen Sie drückbare Elemente bei `core.CursorAuto`, damit sie den Pfeil der Plattform behalten. Für Zeichenwerkzeuge, Spiele oder eigene Canvas-Zeiger auf Desktops und im Web nimmt `Style.CursorImage` einen `*core.CustomCursor` mit einem 1x-`Image`, einem optionalen HiDPI-`Image2x` (automatisch ab `PixelRatio >= 1.5` gewählt), einem `Hotspot` in logischen Punkten, der auf die Bildgrenzen begrenzt wird, und einem benannten `Fallback`-Cursor. Prüfen Sie `cx.Env().SupportsCustomCursor()`, um festzustellen, ob die aktuelle Zielplattform eigene Cursorbilder anzeigt; Touch-Plattformen ignorieren sie.

## Weiter

- [Layout](layout.md): wo Boxen platziert werden.
- [Text](text.md): Schriften, Spans und Dekorationen.
- [Animation](animation.md): Übergänge zwischen Styles.

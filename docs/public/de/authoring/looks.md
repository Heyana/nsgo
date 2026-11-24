---
title: Aussehen und Themes
description: "Farben aus dem aktiven Theme beziehen und einer Komponente Varianten und Größen verleihen."
group: Komponenten schreiben
order: 72
---

Eine gestylte Komponente wählt ausschließlich Styles aus und sonst nichts: Sie umhüllt eine ungestylte Komponente – in einem eigenen Paket wie `chipsui` – und belässt deren Verhalten dort. Ihre Farben stammen aus dem Theme, sodass sie dem Theme der App, dem eigenen Theme eines Panels sowie dem hellen oder dunklen Erscheinungsbild folgt, ohne neu gebaut werden zu müssen.

## Farben aus dem Theme

Lesen Sie Farben innerhalb einer `StyleFunc`: Diese wird erneut ausgeführt, sobald sich das Gelesene ändert, sodass ein Wechsel des Themes oder Erscheinungsbilds die Komponente direkt neu stylt. `cx.Palette()` liefert die am Standort der Komponente angezeigte Palette, und `ui.ColorsOf(cx)` benennt die Rollen, die `ui` verwendet.

```go
// Toggle is chips.Toggle in the theme's colors.
type Toggle struct {
	On       func() bool
	OnChange func(bool)
	Label    string
}

func (t Toggle) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	on := func() bool { return t.On != nil && t.On() }
	return chips.Toggle{On: t.On, OnChange: t.OnChange, Label: t.Label,
		Class: "flex-row items-center gap-2 rounded-full px-3 py-1",
		StyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.Accent()}
			}
			return core.Style{Background: colors.Inset()}
		},
		DotClass: "size-2 rounded-full",
		DotStyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.OnAccent()}
			}
			return core.Style{Background: colors.Faint()}
		},
		Children: []core.Node{ui.Text(t.Label, 13, ui.Medium, colors.Text)},
	}
}
```

Hart codierte Farben funktionieren im Dunkelmodus und unter einem App-Theme nicht mehr; beziehen Sie jede Farbe über eine Rolle.

## Eigene Rollen

Eine Farbe, für die die Palette keine Rolle vorsieht, gehört unter einem eigenen Namen in `Palette.Colors` – gesetzt für beide Erscheinungsbilder. Apps überschreiben sie im Code oder in ihrer Theme-Datei unter `light` und `dark`:

```go
theme := core.DefaultTheme()
theme.Light.SetColor("chip", core.RGB(0xeef5ff))
theme.Dark.SetColor("chip", core.RGB(0x1b2a3d))
cx.SetTheme(theme)
```

Eine Komponente liest sie mit `cx.Palette().Color("chip")`, was zugleich meldet, ob das Theme sie definiert – so kann die Komponente andernfalls auf eine Standardrolle zurückfallen. Farbverläufe, Schatten und andere Effekte sind keine Farben: Belassen Sie diese im Code, ausgewählt über `cx.Dark()`.

## Varianten

Eine Komponente mit mehreren Darstellungsformen erhält `Variant`, einen eigenen Typ, dessen Nullwert das Standardaussehen ist. Er benennt ausschließlich Erscheinungsformen, die die Komponente selbst zeichnet – einschließlich ihrer Farbe –, statt boolescher Felder oder eines gemeinsamen Farbtyps:

```go
// ToggleVariant is a toggle's look.
type ToggleVariant uint8

const (
	// ToggleSolid fills the pill with the accent when on.
	ToggleSolid ToggleVariant = iota
	// ToggleOutline draws its edge in the accent instead.
	ToggleOutline
)
```

## Größen

Die meisten Komponenten benötigen nur eine Größe, wodurch Steuerelemente bündig ausgerichtet bleiben, ohne dass eine App wählen muss. Eine Komponente, die mehrere Größen braucht, erhält `Size` vom Typ `ui.Size` (Standard, klein oder groß), damit sie zu den 28, 24 und 32 Punkten von `ui.Button` passt; `Size.Height()` liefert die Höhe, an der sich andere Steuerelemente ausrichten lassen. Eine Komponente, die in jedem Maßstab dieselbe Form hat – wie ein Avatar –, nimmt stattdessen `Size` in Punkten entgegen.

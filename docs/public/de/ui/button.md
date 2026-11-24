---
title: Button
description: Ein Druckknopf, seine Varianten und Größen sowie der Icon-Button.
group: Komponenten
order: 31
---

`ui.Button` ist ein Druckknopf im Aussehen von `ui`, mit den Varianten und Größen, die ein Fenster braucht. `OnPress` wird bei einem vollständigen Druck ausgeführt.

```go preview=button/basic
ui.Button{Text: "Save", OnPress: save}
ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: cancel}
```

## Varianten und Größen

`ui.Button.Variant` ist `ButtonPrimary` (die Vorgabe), `ButtonSecondary`, `ButtonOutline`, `ButtonGhost`, `ButtonTinted` oder `ButtonDanger`. Tinted und Danger verwenden die Akzent- bzw. Gefahrenfarbe des Themes auf einer weichen Füllung. Bei Desktop- und TV-Idiomen ist `Size` `SizeDefault` (28 Punkt), `SizeSmall` (24) oder `SizeLarge` (32). Bei Touch-Idiomen betragen dieselben Größen 44, 40 und 48 Punkt, sodass sich der Button verwenden lässt, ohne ihn umzugestalten.

```go preview=button/variants
ui.Button{Text: "Delete", Variant: ui.ButtonOutline, OnPress: remove}
ui.Button{Text: "Continue", Size: ui.SizeLarge, OnPress: next}
ui.Button{Text: "Save", Icon: lucide.Check, Flex: 1, OnPress: save}
```

`Color` füllt den Button mit einer Farbe der App statt mit der einer Variante, und `Tint` färbt sein Icon. `Flex` gibt ihm einen Anteil an einer Zeile, während `Style` ihn anordnet: Damit kann der Button seinen Container ausfüllen, eine Größe der App erhalten, Außenabstände bekommen oder abgerundete Ecken haben; `TestID` benennt ihn für Tests.

```go
ui.Button{Text: "Continue", Size: ui.SizeLarge,
    Style: core.Style{Width: core.Pct(100), Height: core.Pt(50)}, OnPress: next}
```

Ein Button, der nur aus einem Icon besteht, ist `ui.IconButton`, mit `Variant` `IconButtonGhost`, `IconButtonRaised` oder `IconButtonFilled` – einem flachen Kreis in der Inset-Farbe des Themes oder in `Fill` –, einer `Size` in Punkt und einem `Label` für assistive Technologien. Auf Desktops erscheint das Label außerdem als Tooltip, sobald der Zeiger auf dem Button verweilt; `Tooltip` zeigt dort etwas anderes an, und ein Tooltip, das `""` zurückgibt, zeigt keines. Ein `ui.Button` mit `Icon` und ohne `Text` ist ebenfalls quadratisch und benötigt ein `Label`.

```go preview=button/icon
ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost, OnPress: remove}
```

## Inhalt und Zustände

`ui.Button` zeigt `Text`, ein optionales vorangestelltes `Icon` oder beides. Verwenden Sie `TextStyle`, um Schriftgröße, Schriftstärke oder andere Textstile des Labels anzupassen.

```go
ui.Button{Text: "Saving…", Disabled: saving.Get, OnPress: save}
```

`Disabled` nimmt eine Funktion entgegen, sodass der Button einem Signal folgt: Ein deaktivierter Button nimmt weder Drücke noch den Tastaturfokus an, und sein Aussehen ist der deaktivierte Zustand, keine Farbe, die die App wählt.

`Busy` ist für eine Aktion gedacht, die noch läuft. Solange es true zurückgibt, erscheint über dem Inhalt ein Spinner in der Textfarbe des Buttons, sodass er auch auf dem Akzent eines Primary-Buttons sichtbar ist; Drücke bewirken nichts, und assistive Technologien erfahren, dass er beschäftigt ist. Standardmäßig behält der Inhalt seinen Platz unter dem Spinner. Der Button behält den Tastaturfokus. `BusyLabel` oder `BusyLabelFunc` zeigt eine Meldung neben dem Spinner an und meldet sie, solange er beschäftigt ist, als barrierefreien Namen des Buttons. Der Button behält mindestens seine Breite im Ruhezustand.

```go preview=button/busy
ui.Button{Text: "Save", Busy: saving.Get, BusyLabel: "Saving…", OnPress: save}
```

`ui.Spinner` und `ui.ProgressRing` nehmen einen eigenen `Tint` für jede andere Fläche entgegen, auf der ihr Akzent verschwinden würde.

## Links

`ui.Link{Text: "Docs", Href: "/docs"}` öffnet ein Ziel mit der Rolle Link. Sein Text und die Unterstreichung beim Überfahren verwenden den Akzent, sofern `Style.Color` keine andere Farbe setzt; `Style.Hover.Color` legt die Farbe beim Überfahren fest.

## Barrierefreiheit

Der Button meldet `RoleButton` und seinen Text als Namen. `Label` ersetzt den Text für assistive Technologien. Ein Button nur mit Icon und ohne `Label` ist nicht lesbar, also benennen Sie ihn.

Die Eingabetaste und die Leertaste drücken einen fokussierten Button auf Desktops, im Web und auf Apple TV; der Screenreader eines Telefons drückt ihn als Klick.

## Plattform

Der Button ist ein View, der überall gleich gezeichnet wird, kein Button der Plattform; Drücke, Fokus und Barrierefreiheit stammen von der Plattform. Touch-Idiome verwenden größere Layoutziele, wie oben beschrieben.

## Verwandte Themen

- [Formulare](../forms.md): Buttons in einem Formular und Validierung.
- [Styles und Themes](../styles.md): das Theme, aus dem ein Button seine Farben bezieht.
- [Barrierefreiheit](../accessibility.md): Rollen, Labels und Fokus.
- Die Button-Seite der Galerie.
- [Komponenten ohne Design](../authoring/base.md#button): `base.Button`, für einen Button mit eigenem Aussehen.

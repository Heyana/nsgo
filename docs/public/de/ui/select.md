---
title: Select
description: Ein Wert aus einer Liste, als Popup oder Sheet.
group: Komponenten
order: 36
---

`ui.Select[T]` wählt einen Wert aus einer Liste von Werten eines beliebigen vergleichbaren Typs. Die Liste öffnet sich auf jedem Gerät unter dem Auslöser; `Presentation: base.PresentSheet` öffnet sie stattdessen als Sheet von unten, wie es eine Smartphone-App wünschen mag.

```go preview=select/basic height=280
size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options: []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:   size.Get,
	OnChange: size.Set,
	Label:   "Size",
}
```

## Optionen und Suche

`Options` sind `ui.Option[T]`s, jeweils mit einem `Value`, einem `Label` und optional einer `Group`, einem `Icon`, einer `Disabled`-Funktion oder eigenem `Content` der App. Benachbarte Optionen derselben nicht leeren `Group` erscheinen in einem `Select` oder einer `Combobox` unter einer Überschrift dieser Gruppe, die von den Pfeiltasten und der Tippsuche übersprungen und von Screenreadern als Überschrift vorgelesen wird. Verwenden Sie `OptionsFunc`, wenn die Optionen einem Signal folgen, damit sich die geöffnete Liste direkt aktualisiert und Wert und Fokus des Feldes erhalten bleiben. `Placeholder` wird angezeigt, solange keine Option gewählt ist. `Searchable` filtert die Liste beim Tippen, `OnClear` zeigt einen Button zum Leeren, und `Disabled` schaltet es ab.

```go preview=select/searchable height=400
ui.Select[string]{
	Options:  languages,
	Value:    language.Get,
	OnChange: language.Set,
	Searchable: true,
	SearchPlaceholder: "Search languages",
	OnClear:  func() { language.Set("") },
	Placeholder: "Choose a language",
}
```

`Style` bestimmt das Layout des Selects, 200 Punkt breit, sofern keine Breite gesetzt ist; Höhe, Innenabstand und Schrift wirken auf das Feld, wie bei einem [Input](input.md), sodass ein Select bündig mit einem `SizeSmall`-Button steht:

```go
ui.Select[string]{Options: sizes, Value: size.Get, OnChange: size.Set, Label: "Size",
	Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
```

## Verhalten

Pfeil hoch, Pfeil runter, Eingabetaste und Leertaste öffnen die Liste vom fokussierten Auslöser aus; die Pfeiltasten bewegen die Hervorhebung, und die Tippsuche springt anhand der Titel der Einträge. Das Wählen eines Eintrags schließt die Liste und meldet seinen Wert. Escape, Zurück und ein Drücken außerhalb schließen sie ohne Änderung. `Empty` ist der Text, der angezeigt wird, solange die Suche nichts findet.

## Barrierefreiheit

`Label` benennt das Feld für assistive Technologien. Verwenden Sie `LabelFunc`, um diesen Namen direkt zu aktualisieren, wenn sich seine Signals ändern. Ein leeres Label verwendet `Placeholder`. Auslöser und Liste tragen die Rollen des Selects, und der ausgewählte Eintrag wird gemeldet. Die Tastatur erreicht den Auslöser mit Tab und öffnet ihn mit Eingabetaste oder Leertaste.

## Plattform

Unter macOS, Windows und Linux ist die Liste ein Popup neben dem Auslöser. Unter iOS und Android ist sie ein Sheet, und die ausgewählte Zeile bleibt im Feld. Dieselben Optionen und Werte funktionieren auf jeder Zielplattform.

## Siehe auch

- [Formulare](../forms.md): Selects in einem Formular.
- [Menu](menu.md): Befehle statt eines Wertes.
- [Tabs](tabs.md): wenige gleichzeitig angezeigte Optionen.
- Die Seiten Select und Combobox der Galerie.
- [Komponenten ohne Design](../authoring/base.md#select): `base.Select`, mit Gruppen und eigenem Auslöser.

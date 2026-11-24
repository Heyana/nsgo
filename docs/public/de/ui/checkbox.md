---
title: Checkbox
description: Ein Kontrollkästchen – aktiviert, deaktiviert oder unbestimmt.
group: Komponenten
order: 34
---

`ui.Checkbox` ist ein Kontrollkästchen im Erscheinungsbild von `ui`, mit einer Beschriftung daneben. Es hält keinen eigenen Wert: Die App gibt an, ob es aktiviert ist, und aktualisiert dies in `OnChange`.

```go preview=checkbox/basic
done := core.NewSignal(cx, false)
ui.Checkbox{Checked: done.Get, OnChange: done.Set, Text: "Remember me"}
```

## Inhalt

`ui.Checkbox` zeigt `Text` oder die `Children` der App neben seinem Kästchen an. `Label` ersetzt beides für assistive Technologien, und `Hint` beschreibt, wofür es dient.

## Zustände

`Indeterminate` ist weder aktiviert noch deaktiviert – für ein „Alle auswählen“ über einer teilweisen Auswahl; ein Drücken fordert dann den aktivierten Zustand an. `Disabled` unterbindet Drücken und Fokus.

```go preview=checkbox/indeterminate height=220
ui.Checkbox{
	Checked:       func() bool { return chosen.Get() == len(files) },
	Indeterminate: func() bool { return chosen.Get() > 0 && chosen.Get() < len(files) },
	OnChange:      chooseAll,
	Text:          "Select all",
}
```

## Modifikatortasten beim Drücken

Verwenden Sie `OnChangeEvent` statt `OnChange`, wenn eine Liste eine Bereichsauswahl per Umschalt-Drücken benötigt:

```go
ui.Checkbox{Checked: done.Get, Text: "Remember me", OnChangeEvent: func(next bool, press core.PressEvent) {
	if press.Modifiers.Shift {
		chooseRange(next)
		return
	}
	done.Set(next)
}}
```

Ist dieser Callback gesetzt, ersetzt er `OnChange`; die App aktualisiert den Wert weiterhin selbst. Die Aktivierung per Tastatur auf Desktop und im Web behält die Modifikatoren nach den Tastenkürzeln der App bei; unter Windows gehen Alt mit Leertaste oder Eingabetaste stattdessen an das Fenstermenü. `onChangeEvent` in TSX erhält `{ checked, press }`.

## Barrierefreiheit

Die Checkbox meldet `RoleCheckbox` sowie ihren aktivierten oder unbestimmten Zustand. Ihr Name ist `Text`, `Label` oder der Text der Kinder. Die Leertaste schaltet sie um, wenn sie den Tastaturfokus hat, und ein Screenreader sagt den neuen Zustand an.

## Plattform

Das Kästchen wird von der App gezeichnet, nicht von einem Steuerelement der Plattform, und sieht daher überall gleich aus. Drücken, Fokusring und Screenreader stammen von der Plattform. Es wird auf jeder Zielplattform anhand von Rolle und Label getestet.

## Siehe auch

- [Switch](switch.md): eine Einstellung, die sofort wirkt.
- [Formulare](../forms.md): Auswahlmöglichkeiten in einem Formular.
- [List](list.md): Checkboxen in Zeilen.
- Die Checkbox-Seite der Galerie.
- [Komponenten ohne Design](../authoring/base.md#checkbox): `base.Checkbox` für ein Kästchen in eigenem Erscheinungsbild.

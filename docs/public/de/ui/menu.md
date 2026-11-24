---
title: Menu
description: Ein Menü über einen Auslöser, ein Kontextmenü und ihre Einträge.
group: Komponenten
order: 42
---

`ui.Menu` öffnet `ui.MenuItem`s über einen Button: daneben oder, mit `Presentation: base.PresentSheet`, als Sheet von unten – unabhängig vom Gerät. `ui.ContextMenu` öffnet dieselben Einträge am Zeiger.

```go preview=menu/basic height=280
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", Icon: lucide.Pencil, OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", Shortcut: "mod+d", OnSelect: duplicate},
	ui.MenuSeparator{},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

Das Menü öffnet und schließt sich selbst; mit `Open` und `OnOpenChange` kann die App den Zustand halten. `Trigger` bestimmt Aussehen und Inhalt des Buttons, und das Menü übernimmt dessen Drücken. Ein Popup ragt nie über den Platz neben seinem Auslöser hinaus: Es öffnet sich auf der Seite, auf der es vollständig passt, und seine Einträge scrollen, wenn sie diesen Platz oder `MaxHeight` überschreiten.

## Einträge

Ein `ui.MenuItem` zeigt `Text`, ein optionales `Icon` und ein `Shortcut` (`"mod+c"`, in einem Popup in der Schreibweise der Plattform) an und meldet `OnSelect`, wobei es das Menü schließt, sofern nicht `NoCloseOnSelect` gesetzt ist. `Disabled` schaltet es ab, `Checked` zeigt ein Häkchen daneben, und `Variant: ui.MenuItemDanger` kennzeichnet einen destruktiven Eintrag. `ui.MenuLabel` leitet eine Gruppe ein, und `ui.MenuSeparator` trennt Gruppen voneinander. Ein `ui.Submenu` enthält weitere Einträge und öffnet sich neben dem Eintrag oder, in einem Sheet, anstelle der umgebenden Einträge.

```go preview=menu/checked height=320
ui.Menu{Trigger: ui.Button{Text: "View", Variant: ui.ButtonGhost}, Children: []core.Node{
	ui.MenuLabel{Text: "Sort by"},
	ui.MenuItem{Text: "Name", Checked: func() bool { return sort.Get() == "name" }, OnSelect: func() { sort.Set("name") }},
	ui.MenuItem{Text: "Date", Checked: func() bool { return sort.Get() == "date" }, OnSelect: func() { sort.Set("date") }},
	ui.MenuSeparator{},
	ui.Submenu{Text: "Share", Children: []core.Node{
		ui.MenuItem{Text: "Copy link", OnSelect: copyLink},
		ui.MenuItem{Text: "Email", OnSelect: emailLink},
	}},
}}
```

## Kontextmenüs

`ui.ContextMenu` öffnet seine Einträge bei einem Sekundärklick – oder einem langen Drücken auf Touchscreens – am Zeiger über seinem `Trigger`.

```go preview=menu/context height=220
ui.ContextMenu{Trigger: fileRow, Children: []core.Node{
	ui.MenuItem{Text: "Open", OnSelect: open},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Verhalten

Escape, Zurück und ein Drücken außerhalb schließen ein Menü. Das Menü ist ein Popup, das durch `Placement` und `Align` platziert wird, oder ein Sheet von unten, wenn es so dargestellt wird. Auf einem Desktop bewegen Pfeil runter, Pfeil hoch, Pos1, Ende, Bild runter und Bild hoch durch die aktivierten Einträge, beginnend bei dem unter dem Zeiger; Pfeil rechts öffnet ein Untermenü und Pfeil links schließt es, und die Eingabetaste wählt aus. Ein mit der Tastatur geöffnetes Menü fokussiert seinen ersten Eintrag, und der Fokus kehrt zum Auslöser zurück, wenn sich das Menü schließt.

## Barrierefreiheit

Das Menü meldet `RoleMenu` und jeder Eintrag `RoleMenuItem`. `Label` benennt den Auslöser, wenn sein Inhalt das nicht tut. Ein Screenreader liest den Text eines Eintrags vor und ob er deaktiviert ist.

## Plattform

Auslöser und Einträge gehören der App; Sheet, Popup, Fokus, Escape und Zurück stammen von der Plattform. Die App wählt zwischen Popup und Sheet; NSGo wählt keines von beiden anhand des Geräts.

## Siehe auch

- [Select](select.md): ein Wert aus einer Liste.
- [Dialog](dialog.md): eine modale Oberfläche.
- [Tabs](tabs.md): gleichzeitig angezeigte Abschnitte.
- Die Seiten Menu, ContextMenu und Command der Galerie.
- [Komponenten ohne Design](../authoring/base.md#menu): `base.Menu` für ein Menü in eigenem Erscheinungsbild.

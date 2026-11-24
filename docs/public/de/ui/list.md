---
title: List
description: Beliebig viele Zeilen, nur im sichtbaren Bereich aufgebaut, mit Auswahl und Aktivierung.
group: Komponenten
order: 38
---

`ui.List` stellt Zeilen in einer nativen Scroll-View dar und baut nur die Zeilen in der Nähe des Bildschirms auf – mit Auswahl, Aktivierung und Tastatur.

```go preview=list/basic height=280
ui.List{
	Count:         func() int { return len(items) },
	Key:           func(i int) any { return items[i].ID },
	EstimatedSize: 44,
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		item := items[row.Index()]
		return ui.Text(item.Title, 13, ui.Regular, colors.Text)
	},
}
```

## Zeilen und Identität

`Count` gibt an, wie viele Zeilen es gibt, und `Row` baut eine auf. `Key` verleiht einer Zeile ihre Identität: Zeilen, die bestehen bleiben, behalten ihre Zellen und ihren Zustand, während darüber Zeilen hinzukommen und wegfallen. Verwenden Sie eine ID statt des Index, wenn Zeilen hinzugefügt, entfernt oder umsortiert werden können; der Index ist ein Signal, das der Zelle folgt, wenn sie eine andere Zeile anzeigt.

```go
ui.List{Count: count, Key: func(i int) any { return items[i].ID }, Row: func(cx *core.Scope, row core.ListRow) core.Node {
	return ui.Text(items[row.Index()].Title, 13, ui.Regular, colors.Text)
}}
```

`EstimatedSize` ist die erwartete Höhe einer Zeile, 50 Punkt bei 0. Zeilen dürfen unterschiedlich hoch sein; die Liste misst die aufgebauten und schätzt den Rest.

## Layout

`Header`, `Footer` und `Empty` enthalten Inhalte rund um die Zeilen beziehungsweise solange keine vorhanden sind. `Gap` trennt Zeilen, `Columns` ordnet sie nebeneinander an, und der Innenabstand der Liste gehört zum Inhalt, sodass die Scrollleiste am Rand bleibt. Geben Sie der Liste eine Höhe oder lassen Sie sie ihr Elternelement ausfüllen.

## Auswahl und Aktivierung

`ui.List` nimmt eine `Selection` entgegen, erstellt mit `ui.NewSelection(cx, multiple)`. Schlüssel, Labels, Aktivierung und Tastatur richten sich nach ihr: Pfeil hoch, Pfeil runter, Pos1 und Ende bewegen den Cursor, Umschalt erweitert, Mod+A wählt alles, die Leertaste schaltet in einer Mehrfachauswahl um, und die Eingabetaste oder ein Doppelklick aktiviert eine Zeile.

```go preview=list/selection height=320
selected := ui.NewSelection(cx, true)
ui.List{
	Count: count, Key: func(i int) any { return items[i].ID },
	Selection: selected,
	OnActivate: func(i int) { open(items[i]) },
	RowLabel:   func(i int) string { return items[i].Title },
	Row: func(cx *core.Scope, row core.ListRow) core.Node { return rowContent(cx, items[row.Index()]) },
}
```

`OnSelectionChange` läuft, nachdem ein Drücken, eine Taste oder die Tippsuche geändert hat, welche Zeilen gewählt sind, nicht aber, wenn die App die `Selection` selbst setzt – so kann ein Detailbereich der Wahl der Nutzerin oder des Nutzers folgen, während die App die Auswahl auf ein auf anderem Weg geöffnetes Element verschiebt.

`TapSelects` lässt ein Tippen auf einem Smartphone auswählen statt aktivieren. `RowLabel` ist das, was assistive Technologien vorlesen und womit die Tippsuche abgleicht.

## Laden und Scrollen

`OnLoadMore` läuft, wenn das Ende näher rückt, einmal pro Zeilenanzahl. `Ref` scrollt mit `ScrollToIndexAt`, `ScrollToEnd` und `ScrollToIndex`. `core.List`, die einfache Liste darunter, bietet zudem `OnLoadMoreAtStart`, `Inverted` für einen Chat, `StickyHeaderIndices`, `MaintainVisibleContentPosition`, `Viewport` und `ViewabilityConfig` für die Low-Level-Fälle ([Listen und Scrollen](../lists.md)).

## Barrierefreiheit

Die Liste ist eine einzige Liste oder Listbox, benannt durch `Label`, mit dem Text einer Zeile oder `RowLabel` als deren Name. Eine Liste mit Auswahl meldet die Zeile des Cursors und sagt sie an, wenn sich die Tastatur bewegt. Ein Screenreader, der sich aus dem sichtbaren Bereich hinausbewegt, fordert die Liste auf, dorthin zu scrollen, wodurch die erreichten Zeilen aufgebaut werden.

## Plattform

Die native Scroll-View und die Virtualisierung sind auf jeder Zielplattform gleich. Zeilen werden wiederverwendet, daher muss der Zustand einer Zeile ihrem `Key` folgen; Aufbauzustand in der App statt in einer Zeile zu halten, ist die übliche Regel.

## Siehe auch

- [Listen und Scrollen](../lists.md): der vollständige Leitfaden, einschließlich fixierter Header und Nachladen.
- [Scroll](scroll.md): eine einfache Scroll-View.
- [DataTable](../lists.md#tabellen): eine Tabelle mit vielen Zeilen und Spalten.
- Die Seiten List, Table und DataTable der Galerie.

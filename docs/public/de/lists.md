---
title: Listen und Scrollen
description: Scroll-Views mit nativen Gesten und Listen, die Zeilen für beliebig viele Einträge wiederverwenden.
group: Kernkonzepte
order: 55
---

## Eine Scroll-View

`core.Scroll` ist die eigene Scroll-View der Plattform, daher sind Wischen mit Schwung, Zurückfedern, die Scrollleiste, Einrasten und Touch-Scrollen nativ. Ihre Kinder werden in einer Spalte angeordnet – oder mit `Horizontal` in einer Zeile –, so lang, wie sie es benötigen.

```go
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

Die Scroll-View füllt ihren Container, und das Padding liegt auf ihrem Inhalt, sodass die Scrollleiste am Rand bleibt. Sie schrumpft, um in ihr Elternelement zu passen, sofern ihr Style nichts anderes vorgibt. Setzen Sie `NoShrink`, wenn sie ihre angeforderte Größe behalten muss, während ein Geschwisterelement schrumpft. `HideIndicator` blendet den Indikator der Plattform aus, und `Bar` zeichnet einen eigenen der App.

`Behavior` und `BehaviorFunc` legen fest, wie sie scrollt: `Paging`, `SnapToInterval` und `SnapToOffsets` mit `SnapToAlignment`, eine `DecelerationRate`, `NoBounces`, `ContentInset` und die Regeln für die Tastatur. `MinZoomScale` und `MaxZoomScale` ermöglichen Zoomen per Pinch-Geste, `OnRefresh` und `Refreshing` lassen sie per Herunterziehen aktualisieren, und `OffsetValues` folgt ihrem Offset auf dem Main-Thread für Effekte, die sich mit ihr bewegen.

```go
page := core.Scroll{
	OnScroll: func(e core.ScrollEvent) { y.Set(e.Offset.Y) },
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: []core.Node{...},
}
```

`OnScroll` meldet den Offset höchstens einmal pro Durchlauf oder einmal pro `ScrollEventThrottle`. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd` und `OnScrollEnd` umschließen die Gesten der Nutzerin oder des Nutzers. `Ref` scrollt aus dem Code mit `ScrollTo`, `ScrollToEnd`, `ZoomTo` und – bei einer Liste – `ScrollToIndex`.

## Eine Liste

`core.List` stellt lange Daten auf einer nativen Scroll-View dar und erstellt dabei nur die Zeilen in der Nähe des Bildschirms. `Count` gibt an, wie viele Zeilen es gibt, und `Row` baut eine davon; `Key` gibt jeder Zeile eine Identität, sodass Zeilen, die bestehen bleiben, ihre Zellen behalten, während darüber Zeilen hinzukommen und wegfallen.

```go
core.List{
	Count:         func() int { return len(items.Get()) },
	Key:           func(i int) any { return items.Get()[i].ID },
	EstimatedSize: 56,
	Row: func(cx *core.Scope, r core.ListRow) core.Node {
		return core.View{Style: core.Style{Padding: core.All(12)}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return items.Get()[r.Index()].Title }},
		}}
	},
}
```

`Row` läuft einmal pro wiederverwendeter Zelle und aktualisiert `r.Index()`, wenn diese Zelle für eine andere Zeile wiederverwendet wird. Lesen Sie `r.Index()` und Signals der Einträge daher innerhalb reaktiver Funktionen (`ContentFunc`, `StyleFunc`, `NewMemo`), statt beim Aufbau der Zeile einen einmaligen Wert festzuhalten. Ungenutzte Zellen werden geparkt, sodass ihre Berechnungen warten, bis die Zelle wieder an eine aktive Zeile gebunden ist.

`Header`, `Footer`, `Empty` und `Separator` enthalten den Inhalt um die Zeilen herum und zwischen ihnen. `NumColumns` setzt Zeilen nebeneinander, `Gap` und `ColumnGap` trennen sie, und `Horizontal` scrollt quer. `StickyHeaderIndices` heftet diese Zeilen an den Anfang des Viewports, bis die nächste sie hinausschiebt, und `StickyHeaderOffset` hält einen Header darüber frei.

`OnViewableItemsChange` meldet die sichtbaren Zeilen gemäß den Schwellenwerten von `ViewabilityConfig`, und `OnLoadMore` läuft, wenn sich der Viewport dem Ende nähert, einmal pro Zeilenanzahl. `OnLoadMoreAtStart` tut dasselbe am Anfang, und `InitialScrollIndex` öffnet die Liste bei einer bestimmten Zeile. `Alignment: core.ListAlignEnd` lässt eine Liste an ihrem Ende ruhen, wie es ein Chat mit seinen Zeilen in Lesereihenfolge tut: Sie öffnet sich bei ihrer neuesten Zeile, folgt neuen Zeilen, solange die Nutzerin oder der Nutzer am Ende ist, und hält eine kurze Unterhaltung unten. `Inverted` setzt stattdessen Zeile 0 ans Ende, und `MaintainVisibleContentPosition` folgt dem Ende einer wachsenden Liste, solange die Nutzerin oder der Nutzer in dessen Nähe ist.

```go
core.List{Ref: list, Count: count, EstimatedSize: 64, Row: row, OnLoadMore: loadMore,
	StickyHeaderIndices: headers(cx), Footer: core.View{Style: core.Style{Height: core.Pt(24)}}}
```

`list.ScrollToIndexAt(index, 0.5, 0, true)` scrollt eine Zeile in die Mitte des Viewports und `list.ScrollToEnd(true)` ans Ende. `list.ListPosition()` liest aus, wo der Viewport beginnt – eine Zeile und wie weit in sie hinein –, um eine Leseposition zu bewahren: `list.ScrollToIndexAt(p.Index, 0, -p.Offset, false)` bringt eine neu erstellte Liste dorthin zurück. Eine Liste kann auch zusammen mit anderem Inhalt auf einer Seite liegen: Setzen Sie `Viewport: core.ListViewportParent`, und die umgebende Scroll-View scrollt die Liste mit dem Rest.

## Tabellen

`base/datatable` zeigt eine Tabelle mit einer Million Zeilen an, mit virtualisierten Zeilen und Spalten, einem fixierten Header, links (`Fixed`) oder rechts (`FixedRight`) fixierten Spalten, unter denen die übrigen scrollen, Sortierung, Einzel- oder Mehrfachauswahl von Zeilen, Ändern der Größe und Verschieben von Spalten sowie eigenen Zellen der App. `MultiSelect` unterstützt das Umschalten mit Command oder Control, Bereiche mit Shift und Tastaturerweiterungen mit gedrückter Shift-Taste, während `SelectedRows` und `OnSelectedRowsChange` der App erlauben, die Zeilen zu steuern. `CellTooltip` gibt einer Zelle einen Tooltip, der die Zelle auch für Screenreader beschreibt und von `Styles.CellTooltip` gestaltet wird; was `RenderCell` zeichnet, empfängt keinen Zeiger, daher erscheint ein `ui.Tooltip` innerhalb einer Zelle nie. `base.Table` ist eine einfache Tabelle der Zeilen der App für kleinere Datenmengen, und `base/plot` zeichnet Diagramme.

Verwenden Sie `DataTable.LabelFunc` für einen Grid-Namen, der dem Gebietsschema oder dem Anwendungszustand folgt. `ColumnGroup.LabelFunc` aktualisiert eine gruppierte Spaltenüberschrift, solange die Gruppe sichtbar bleibt.

## Weiter

- [Animationen](animation.md): Effekte, die einer Scroll-View folgen.
- [Text](text.md): Textzeilen, die umbrechen.
- [Navigation](navigation.md): Bildschirme, deren Inhalt scrollt.

---
title: Scroll
description: Die Scroll-View der Plattform, mit Paging, Einrasten, Zoom und Aktualisieren.
group: Komponenten
order: 39
---

`core.Scroll` ist die plattformeigene Scroll-View. Ihre Kinder werden in einer Spalte angeordnet – oder mit `Horizontal` in einer Zeile –, und sie füllt ihren Container aus, während der Inhalt scrollt.

```go preview=scroll/basic height=240
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

Der Innenabstand gehört auf den Inhalt, nicht auf die Scroll-View, damit die Scrollleiste am Rand bleibt. Die Scroll-View schrumpft, um in ihr Elternelement zu passen, sofern ihr Style nichts anderes angibt. Setzen Sie `NoShrink`, wenn sie ihre angeforderte Größe behalten muss, während ein Geschwisterelement schrumpft.

## Verhalten

`Behavior` wählt, wie sie scrollt: `Paging`, `SnapToInterval` oder `SnapToOffsets` mit `SnapToAlignment`, eine `DecelerationRate`, `NoBounces`, `ContentInset` sowie die Regeln für die Tastatur.

```go preview=scroll/paging height=240
core.Scroll{
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: pages,
}
```

`HideIndicator` blendet die Scrollleiste der Plattform aus, und `Bar` zeichnet eine eigene der App. `MinZoomScale` und `MaxZoomScale` erlauben Zoomen per Pinch-Geste; `NoPinchZoom` ignoriert sie. `CenterContent` zentriert Inhalt, der kleiner als der Viewport ist.

## Aktualisieren und Ereignisse

`OnRefresh` macht die View aktualisierbar; setzen Sie `Refreshing`, während die Arbeit läuft. `PullValue` folgt, wie weit der Inhalt gezogen wird, für einen eigenen Indikator der App, und `Behavior.RefreshIndicator` legt dessen Höhe fest.

```go
refreshing := core.NewSignal(cx, false)
core.Scroll{
	Refreshing: refreshing.Get,
	OnRefresh:  func() { refreshing.Set(true); reload(cx) },
	Children:   content,
}
```

`OnScroll` meldet den Offset höchstens einmal pro Durchlauf oder einmal pro `ScrollEventThrottle`; `Stale` kennzeichnet eine Meldung aus der Zeit, bevor die Plattform einen Scrollvorgang aus dem Code angewendet hat. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd` und `OnScrollEnd` umschließen die Gesten der Nutzerin oder des Nutzers.

`OffsetValues` folgt dem Offset auf dem Haupt-Thread, für einen Header, der einklappt, ohne pro Frame einen Commit auszulösen. `Ref` scrollt aus dem Code mit `ScrollTo`, `ScrollToEnd` und `ZoomTo`, und der Ref einer beliebigen View darin holt diese mit `Reveal` in den sichtbaren Bereich, wobei jede umgebende Scroll-View gescrollt wird:

```go
row.Reveal(core.RevealOptions{Vertical: core.ScrollAlignCenter, Animated: true})
```

`RevealBox` holt stattdessen einen Kasten innerhalb der View in den sichtbaren Bereich, etwa den Kasten eines Wortes aus den `TextRangeBoxes` eines Textes, und `Margin` hält an den Rändern der äußersten Scroll-View Platz für eine darüberliegende Leiste frei.

Eine View, zu der die Tastatur den Fokus bewegt, scrollt von selbst in den sichtbaren Bereich.

## Layout

`Horizontal` scrollt quer. `Behavior.ContentInset` legt Platz um den Inhalt, der mit ihm scrollt. Für eine Seite, die den sicheren Bereich ausfüllt, verwenden Sie `PaddingEnv` auf dem Inhalt, nicht auf der Scroll-View.

## Barrierefreiheit

`Label` benennt den Scrollbereich, und `Role` gibt an, was er ist, etwa eine Listbox. Die Scrollgesten der Plattform, das Scrollen per Tastatur und das Scrollen per Screenreader bleiben erhalten; `Busy` meldet eine View, die noch lädt.

## Plattform

Jedes Backend verwendet die Scroll-View der Plattform, sodass Wischen mit Schwung, Zurückfedern, Scrollleisten und Touch-Scrollen nativ sind. Randeffekte unterscheiden sich: macOS und iOS federn elastisch zurück, Android dehnt, Windows und Linux stoppen, und das Web folgt dem Browser. `EdgeEffects` und `EdgeBar` fügen Leisten über dem Inhalt hinzu, wo die App sie haben möchte. Auf iOS- und Android-Smartphones und -Tablets zeigen überlaufende vertikale und horizontale Scroll-Views entlang der aktiven Achse einen per Touch ziehbaren Scroll-Griff, der durch den gesamten Inhaltsbereich fährt; kurze Scroll-Views, die ohne Überlauf passen, zoomende Views, `Behavior.NoScroll`, `HideIndicator` sowie Views mit einer eigenen `Bar` oder `base.SetScrollbarTheme` blenden ihn aus. In Tests meldet `nsgotest.App.ScrollHandle` seine Viewport-Grenzen, und `DragAt` oder `Touch` ziehen ihn direkt. Unter Windows erscheint der Overlay-Indikator bei Mausrad, Touch, Ziehen der Leiste und deren Schwung, nicht aber bei App-eigenem `ScrollTo`, `ScrollBy`, Reveals, Insets oder Animationen.

## Siehe auch

- [Listen und Scrollen](../lists.md): der vollständige Leitfaden zu Scrollen und Listen.
- [List](list.md): Zeilen, die wiederverwendet werden.
- [Layout](../layout.md): Innenabstände und der sichere Bereich.
- Die Seiten Scroll und Lists der Galerie.

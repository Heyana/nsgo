---
title: Tabs
description: Eine Tab-Liste, ein segmentiertes Steuerelement und die Tab-Leiste des Systems.
group: Komponenten
order: 40
---

`ui.Tabs[T]` wählt einen aus wenigen Werten, als segmentiertes Steuerelement oder als Tab-Liste; die App zeigt den Inhalt des gewählten Werts daneben an. Eine Tab-Liste ist ein einziger Tab-Stopp: Die Pfeiltasten bewegen sich durch die Tabs.

```go preview=tabs/basic
section := core.NewSignal(cx, "overview")
ui.Tabs[string]{
	Options: []ui.Option[string]{
		{Value: "overview", Label: "Overview"},
		{Value: "activity", Label: "Activity"},
		{Value: "settings", Label: "Settings"},
	},
	Value:    section.Get,
	OnChange: section.Set,
	Label:    "Sections",
}
```

## Varianten und Größen

`Variant` ist `TabsSegmented` (der Standard, ein erhabener Daumen in einer vertieften Spur) oder `TabsUnderline` (eine Akzentlinie unter dem ausgewählten Tab). Eine Unterstreichung kann `IndicatorColor` setzen und mit `IndicatorFit: TabsIndicatorContent` ihrem Label und Icon folgen statt dem ganzen Tab; in TSX heißen sie `indicatorColor` und `indicatorFit="content"`. `Size` ist `SizeDefault`, `SizeSmall` oder `SizeLarge`, und `Equal` gibt jedem Tab dieselbe Breite, statt ihn an sein Label anzupassen.

```go preview=tabs/underline
ui.Tabs[string]{Variant: ui.TabsUnderline, Equal: true, Size: ui.SizeSmall, Options: tabs, Value: section.Get, OnChange: section.Set}
```

Eine Option kann ein `Icon`, eine `Disabled`-Funktion, einen `Content`-Knoten und eine `TestID` tragen. `Content` ersetzt das Label und erbt den Textstil des Tabs.

Verwenden Sie `OptionsFunc`, wenn die Tabs einem Signal folgen. NSGo aktualisiert Zeilen anhand ihrer Werte an Ort und Stelle und behält den ausgewählten Tab und den Tastaturfokus bei, solange dieser Wert weiterhin angeboten wird; ein Wert, der verschwindet, bleibt unausgewählt, bis die App ihn ändert.

## Tab-Leisten

`ui.TabShell` hält Abschnitte der obersten Ebene bei jeder Fensterbreite neben einer Tab-Leiste – standardmäßig unten oder vertikal entlang der führenden oder nachfolgenden Kante, wenn `Edge` (oder `EdgeFunc`) `mobile.TabBarLeading`, `mobile.TabBarTrailing` oder `mobile.TabBarAutomatic` ist (das `cx.Env().VerticalBarEdge()` folgt). Sie verwendet die Leiste des Systems, wenn jeder Tab ein `SystemIcon` oder ein SVG-`Icon` liefert, unter iOS an jeder Kante und unter Android an der unteren Kante; ein `SystemIcon`, das unter iOS ein SF Symbol oder unter Android ein Drawable benennt, hat Vorrang, andernfalls wird das SVG angezeigt. Ansonsten zeichnet sie die Leiste mit den gelieferten SVG-Icons. Eine Leiste, die eigenen Platz einnimmt, hält selbst Abstand zum Safe Area ihrer Kante, sodass `core.EnvSafeArea` an dieser Kante eines Abschnitts daneben – einschließlich der Seite eines `base.Nav` – 0 ist; unter der schwebenden unteren Systemleiste von iOS halten Sie Inhalte mit `ui.TabBarInset` frei.

Setzen Sie `ShellTab.TextFunc` für Tab-Titel, die der Sprache oder dem Anwendungszustand folgen, und `TabShell.LabelFunc` für den barrierefreien Namen der Leiste. Das Ändern eines Titels behält den ausgewählten Abschnitt und den bestehenden Tab bei, solange sein Icon gleich bleibt.

`mobile.TabBar` ist die Tab-Leiste des Systems unter iOS und Android, mit `Items`, `Selected`, `OnChange` und optional `Edge` (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing` oder `mobile.TabBarAutomatic`) / `EdgeFunc`. Jedes `mobile.TabBarItem` akzeptiert einen `Title` und einen Icon-Namen der Plattform in `Icon`; `SVG: lucide.House()` wird überall dort angezeigt, wo `Icon` unter iOS kein SF Symbol oder unter Android kein Drawable benennt, eingepasst in 24 Punkte und für die Auswahl eingefärbt. In TSX akzeptiert `svg` ein SVG-Dokument und `edge` die Werte `"bottom"`, `"leading"`, `"trailing"` oder `"automatic"`. Anderswo nimmt `mobile.TabBar` keinen Platz ein, zeigen Sie dort also `ui.Tabs` an.

`ui.SidebarShell` hält das, was Seitenleiste, Fußzeile, Kopfzeilen, Inhalt und rechtes Panel enthalten, selbst vom linken und rechten Safe Area frei, wobei ihre Hintergründe bis an die Fensterränder reichen, sodass `core.EnvSafeArea` an diesen Seiten in ihnen 0 ist, und `ui.NavHeader` fügt den seitlichen Safe Area zu seinem Padding hinzu. Der Inhalt einer `ui.NavPage` oder eines `TabShell`-Abschnitts hält mit `PaddingEnv` selbst Abstand zum Safe Area, sodass das, was bis an die Ränder reichen soll, etwa ein randloses Bild, dies auch kann.

## Barrierefreiheit

Die Liste meldet `RoleTabList`, jeder Tab `RoleTab` und der ausgewählte Tab seinen Zustand. Die Liste ist ein einziger Tab-Stopp; Links und Oben, Rechts und Unten, Pos1 und Ende bewegen sich zwischen aktivierten Tabs, und der erreichte Tab meldet ein Drücken. `Label` benennt die Liste.

## Plattform

iOS und Android zeigen die Tab-Leiste des Systems mit eigenem Aussehen und eigenen Gesten; macOS, Windows, Linux und das Web zeigen `ui.Tabs`. Der Inhalt der App ist auf jeder Zielplattform derselbe.

## Siehe auch

- [Navigation](../navigation.md): Seiten und Tab-Leisten in einer App.
- [Menü](menu.md): Befehle statt Abschnitte.
- [Select](select.md): viele Optionen in einer Liste.
- Die Seiten Tabs und NavStack der Gallery.
- [Ungestylte Komponenten](../authoring/base.md#tabs): `base.Tabs`, für Tabs mit eigenem Aussehen.

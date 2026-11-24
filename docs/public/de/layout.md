---
title: Layout
description: Flexbox, Grid, Größen und Positionierung, mit der Safe Area und der Tastatur im Layout.
group: Kernkonzepte
order: 51
---

## Boxen

Jedes Element ist eine Box, die von der Engine angeordnet wird. Die Standardwerte stammen aus React Native: eine Flexbox in Spaltenrichtung, Kinder, die ihre Größe behalten, sofern sie nicht wachsen oder schrumpfen, Elemente, die entlang der Querachse gestreckt werden, und Größen, die Padding und Rahmen einschließen.

```go
core.View{
	Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.All(16), AlignItems: core.AlignCenter},
	Children: []core.Node{
		core.Text{Content: "One"},
		core.Text{Content: "Two"},
	},
}
```

`Column`, `Row`, `ColumnReverse` und `RowReverse` legen die Hauptachse fest; `Wrap` und `WrapReverse` lassen Kinder auf weitere Zeilen umbrechen. `JustifyContent` platziert sie entlang dieser Achse und `AlignItems` – oder `AlignSelf` an einem einzelnen Kind – quer dazu. `Gap`, `RowGap` und `ColumnGap` trennen sie voneinander, und Padding und Margin im `style` verwenden `core.All`, `core.Symmetric` oder `core.Edges`.

## Größen

`Width`, `Height`, `MinWidth`, `MaxWidth` und `FlexBasis` nehmen `core.Pt` für Punkte oder `core.Pct` für einen Prozentsatz des Elternelements. `FlexGrow` verteilt den freien Platz, und `FlexShrink` erlaubt einer Box, etwas davon abzugeben; eine Box ohne beides behält ihre Größe. `AspectRatio` ist die Breite geteilt durch die Höhe. `AutoMargin` nimmt den freien Platz auf den genannten Seiten ein, sodass eine Seite eine Box an die andere schiebt.

```go
core.View{Style: core.Style{Direction: core.Row, Gap: 16}, Children: []core.Node{
	core.View{Style: core.Style{Width: core.Pt(220), FlexShrink: 0}},
	core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}},
}}
```

Der Inhalt einer Box bestimmt ihre Größe, bis eine Breite oder Höhe etwas anderes vorgibt. `core.MinContent`, `core.MaxContent` und `core.FitContent` nehmen die Größen an, die CSS so benennt: Die Min-Content-Breite eines Textes ist sein breitestes Wort. Ein Text in einer Zeile schrumpft und bricht nur mit `FlexShrink: 1` um; fügen Sie `FlexGrow: 1` hinzu, wenn er den restlichen Platz einnehmen soll. Ein nicht gesetztes Minimum ist bereits 0, wie in React Native, daher ändert `MinWidth: core.Pt(0)` nichts, und ein schrumpfender Text bricht seine Wörter, wenn die Zeile schmaler als sein breitestes Wort ist. `MinWidth: core.AutoMinimum` hält ihn bei seinem breitesten Wort an, wie es das automatische Minimum von CSS tut.

Ein Bildschirm, der das Fenster füllt, erhält `FlexGrow: 1` und `FlexShrink: 1`, ebenso jede Box zwischen ihm und einer Liste oder Scroll-View, die scrollen soll. Mit `FlexGrow` allein wächst eine Box auf die Größe ihres Inhalts, sodass eine lange Liste sie über das Fenster hinaus dehnt, statt zu scrollen.

## Grid

`DisplayGrid` ordnet Kinder im CSS-Grid an. `GridTemplateColumns` und `GridTemplateRows` nehmen die CSS-Track-Syntax an, und `GridRow` und `GridColumn` platzieren ein Element nach Linie oder Spanne.

```go
core.View{Style: core.Style{
	Display: core.DisplayGrid,
	GridTemplateColumns: "repeat(3, 1fr)",
	GridAutoRows: "44px",
	Gap: 12,
}, Children: []core.Node{
	core.View{Style: core.Style{GridColumn: core.GridLines{Start: core.GridSpan(2)}}},
}}
```

`repeat(auto-fill, minmax(160px, 1fr))` erzeugt so viele Spalten, wie hineinpassen. `GridFlow` bestimmt, in welcher Reihenfolge sich das Grid füllt, und `JustifyItems`, `JustifySelf` und `AlignItems` platzieren Elemente in ihren Zellen.

## Position

`Relative` ist der Standard und hält eine Box im Fluss; `Top`, `Right`, `Bottom` und `Left` verschieben sie. `Absolute` nimmt sie aus dem Fluss und platziert sie in ihrem nächsten positionierten Vorfahren. `Fixed` platziert sie in der Root-View oder ihrem Modal, unabhängig davon, was scrollt, und `Sticky` heftet sie innerhalb ihres Elternelements in der nächsten Scroll-View an: `Top` von oben oder `Bottom` von unten, wenn `Top` auto ist (`Left` und `Right` horizontal). Eine Fußzeile mit `Position: core.Sticky, Bottom: core.Pt(0)` bleibt unten, während langer Inhalt scrollt, und folgt kurzem Inhalt direkt. `ZIndex` stapelt Boxen unter ihren Geschwistern.

```go
core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Right: core.Pt(8)}}
```

`Static` hält eine Box im Fluss und ignoriert ihre Insets, sodass absolut positionierte Kinder einen Vorfahren oberhalb davon finden. `Overflow` beschneidet Kinder auf eine Box, und `DisplayNone` nimmt eine Box aus dem Layout.

Die eigenen äußeren Schatten und die Kontur einer Box bleiben außerhalb ihres Zuschnitts, wie in CSS, auf Android, Windows, Linux, im Web und auf Apple-Plattformen, und zwar für jede Art von View, einschließlich Ecken mit unterschiedlichen Radien und gezoomtem Scroll-Inhalt. Bis dies auf einem physischen iPhone geprüft ist, legt `Bare` von `ui.Dialog` seinen Zuschnitt weiterhin auf eine innere View mit denselben Ecken, um den Rahmen eingerückt, was sie auf jeder Zielplattform erhält:

```go
core.View{Style: core.Style{CornerRadius: 16, Shadows: []core.BoxShadow{ui.Shadow(0, 8, 24, -6, ui.Black(0.18))}},
	Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, CornerRadius: 16, Overflow: core.Hidden}, Children: content}}}
```

## Richtung und Umgebung

`LayoutDirection` ordnet einen Teilbaum von links nach rechts oder von rechts nach links an, abweichend von der Richtung der App: `DirectionRTL` und `DirectionLTR` setzen sie, und der Standard erbt sie. Links und rechts in Margins, Paddings, Rahmen und Insets tauschen mit der Richtung, sodass sie wie in React Native Anfang und Ende sind. `PhysicalEdges` und `PhysicalBorders` belassen benannte Seiten an ihrem Platz. `WritingDirection` ordnet Abschnitte von Text mit gemischter Richtung, und `WritingAuto` übernimmt sie vom ersten starken Buchstaben.

`PaddingEnv` und `MarginEnv` fügen einer Seite die Umgebung hinzu. `core.EnvSafeArea` ist der Inset der Safe Area an dieser Kante und `core.EnvKeyboard` die Höhe der Bildschirmtastatur, sodass Inhalt oberhalb der Tastatur bleibt, während seine Bewegung mit ihr animiert wird.

```go
core.View{Style: core.Style{
	Padding: core.All(16),
	PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea},
}}
```

`core.EnvAll(core.EnvSafeArea)` benennt jede Kante, und `cx.Env().SafeArea()` und `cx.Env().KeyboardHeight()` lesen die Werte im Code aus.

Auf einem faltbaren oder Dual-Screen-Telefon listet `cx.Env().Geometry()` die Bereiche auf, in die ein Falz oder Scharnier das Fenster teilt, jeweils mit dem Teil der Safe Area, der darin liegt, sowie die Falze dazwischen; `State` und `Angle` geben an, wie weit das Gerät geöffnet ist (`core.HingeClosed`, `HingeHalfOpen` oder `HingeOpen`), auch wenn kein Falz das Fenster kreuzt, etwa auf einem Cover-Display oder einem flach aufgeklappten Bildschirm. Andere Geräte melden einen Bereich und `HingeUnknown`. Unter iOS 27.1 meldet `cx.Env().VerticalBarEdge()` die Kante, die das System für eine vertikale Leiste gewählt hat (`core.VerticalBarLeading`, `core.VerticalBarTrailing` oder `core.VerticalBarUnspecified`), der `mobile.TabBar` und `ui.TabShell` folgen, wenn `Edge` `mobile.TabBarAutomatic` ist. Nichts wird implizit nach der Geometrie angeordnet: Die App wählt ihre eigenen Bereiche.

## Responsive Klassen

`Class` nimmt Tailwind-CSS-Klassen an: `flex`, `flex-col`, `flex-wrap`, `items-center`, `justify-between`, `gap-3`, `p-4`, `w-full`, `grid`, `grid-cols-3`, `col-span-2` und die übrigen. Breakpoints folgen dem Fenster, und das Erscheinungsbild der Anzeige, die Richtung und reduzierte Bewegung folgen der Umgebung:

```go
core.View{Class: "flex flex-col gap-3 p-4 md:flex-row md:gap-6 dark:bg-zinc-900 rtl:flex-row-reverse"}
```

`sm:`, `md:`, `lg:`, `xl:` und `2xl:` gelten ab 40, 48, 64, 80 und 96 rem Breite; `max-md:` gilt unterhalb eines Breakpoints. `portrait:`, `dark:`, `rtl:`, `motion-reduce:`, `contrast-more:`, `pointer-fine:`, `pointer-coarse:`, `ios:`, `android:` und `web:` folgen dem, was die Umgebung angibt. Klassen werden unter einem `Style` zusammengeführt, und die Klasse eines Zustands (`hover:`) wird zu einem Zustands-Style, sodass sich beide mischen lassen.

## Weiter

- [Styles und Themes](styles.md): Farben, Ecken, Schatten und das Erscheinungsbild eines Teilbaums.
- [Text](text.md): wie Text seine Größe erhält und umbricht.
- [Listen und Scrollen](lists.md): Inhalt, der scrollt, und Zeilen, die wiederverwendet werden.

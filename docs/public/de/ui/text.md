---
title: Text
description: Absätze, Spans, Links, Schriften und Dokumente.
group: Komponenten
order: 32
---

`core.Text` ist ein Absatz, den die Text-Engine der Plattform setzt. `ui.Text` und `ui.Textf` sind Hilfsfunktionen für eine Zeile in der Typografie von `ui`; das Element bleibt ein `core.Text` und akzeptiert daher auch `Style` und `Spans`.

```go preview=text/basic
ui.Text("Settings", 20, ui.Bold, colors.Text)
ui.Textf(func() string { return fmt.Sprintf("%d files", n.Get()) }, 13, ui.Regular, colors.Muted)
```

## Inhalt

`Content` ist fest, und `ContentFunc` folgt einem Signal. `MaxLines` begrenzt die Zeilen, und `EllipsizeMode` kürzt den Rest am Anfang, in der Mitte oder am Ende; `Style.WhiteSpaceNoWrap` hält einen Absatz in einer Zeile. `TextAlign`, `TextTransform`, `LineHeight` und `LetterSpacing` gestalten den Absatz, und `OnTextLayout` meldet nach dem Layout seine Zeilen.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) },
	MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 15}}
```

## Spans und Links

`Spans` setzt einen Text aus Abschnitten zusammen, jeder mit eigenem Stil. Ein Span mit `OnPress` ist ein Link, und sein `Background` und `CornerRadius` werden in jeder Zeile hinter seinen Zeichen gezeichnet. `SpansFunc` liefert die Abschnitte erneut, wenn sich das ändert, was es liest – für hervorgehobenen Code oder einen sich ändernden Satz.

```go preview=text/spans
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "Read the "},
	{Content: "guide", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openGuide},
	{Content: " before you start."},
}}
```

`Attachment` setzt eine View anstelle des Span-Inhalts in eine Zeile: ein Icon, ein Bild oder einen kleinen Button. Sie wird in ihrer eigenen Größe angeordnet, bricht mit den Wörtern um und nimmt ihre eigenen Drücke entgegen.

## Schriften und Stil

`FontFamily` benennt eine Schriftfamilie oder eine CSS-Liste. `FontSize`, `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LetterSpacing`, `TextDecoration` und `TextShadowColor` samt Versatz und Radius gestalten den Text. `core.FontMonospace` ist die dicktengleiche Schriftfamilie der Plattform.

```go
core.Text{Content: "fn main() {}", Style: core.Style{FontFamily: core.FontMonospace, FontSize: 13}}
```

Registrieren Sie mitgelieferte Schriften mit `nsgo.Fonts` vor der ersten View, oder registrieren Sie einen Schriftschnitt zur Laufzeit der App mit `cx.RegisterFont`. Ein Text, der vorbereitet wurde, bevor eine Schrift eintrifft, wird erneut vorbereitet.

## Dokumente

`base/textview.TextView` zeigt Markdown oder HTML mit denselben Grundbausteinen an: Überschriften, Absätze, Listen, Tabellen, Code, Fußnoten, Hinweise, Bilder, Formeln und Links. `textview.Parse` liest Markdown und `textview.ParseHTML` HTML, und `Options.Streaming` repariert ein Dokument, das noch eintrifft.

```go preview=text/document height=240
textview.TextView{Markdown: "## Notes\n\nA **native** document.", Styles: ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) { open(e.URL) }}
```

`textview.NewSearch` durchsucht den Text, den eine `TextView` anzeigt, und hebt die Treffer hervor. Geben Sie mehreren Views dieselbe Suche mit `textview.ProvideSearch` und rufen Sie dann `Open`, `SetQuery`, `Next` und `Previous` auf; `Count` und `Selected` melden das Ergebnis. Die Teile der Suche werden mit `textview.PartSearch` und `textview.PartSearchSelected` gestaltet.

## Auswahl und Barrierefreiheit

`SelectableOn` lässt Nutzerinnen und Nutzer einen Text auswählen und kopieren. Eine `core.View` mit `Selectable` lässt ihre Texte gemeinsam in Dokumentreihenfolge auswählen. Eine `Role` wie `RoleHeader` teilt assistiven Technologien mit, was der Text ist, und `Label` ersetzt ihn, wenn die Wörter nicht das Label sind.

## Plattform

Die Text-Engine der Plattform setzt den Text, sodass Zeilenumbrüche, Schriften und Eingabemethoden ihr folgen. CJK-Zeilenumbruch, Emoji und Text von rechts nach links folgen dem System. Die App kann die Textgröße mit `cx.Env().FontScale()` auslesen, und ein Text folgt ihr, sofern `NoFontScaling` nicht gesetzt ist.

## Siehe auch

- [Text](../text.md): die vollständige Anleitung zu Text, Spans, Schriften und Dokumenten.
- [Bilder und Icons](../images.md): Inline-Bilder und Icons.
- [Barrierefreiheit](../accessibility.md): Überschriften, Labels und Auswahl.
- Die Seiten Text und TextView der Gallery.

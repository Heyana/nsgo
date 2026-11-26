---
title: Text
description: Text mit Spans, Schriften, Dekorationen, Auswahl und Dokumenten anzeigen.
group: Kernkonzepte
order: 53
---

## Ein Text

`core.Text` ist ein Absatz, der auf der Engine-Seite von der plattformeigenen Text-Engine gesetzt und vom Main-Thread gezeichnet wird. Seine Größe ergibt sich aus seiner Schrift und seiner Breite, und er bricht innerhalb seiner Box um.

```go
core.Text{Content: "Hello from NSGo", Style: core.Style{
	FontSize:   15,
	LineHeight: 22,
	FontWeight: core.FontWeightSemibold,
}}
```

`Content` ist fest, und `ContentFunc` folgt einem Signal, sodass ein veränderlicher Text es innerhalb der Funktion liest. `MaxLines` begrenzt die angezeigten Zeilen, `EllipsizeMode` kürzt den Rest am Anfang, in der Mitte oder am Ende, und `WhiteSpaceNoWrap` hält einen Absatz in einer Zeile. `AdjustsFontSizeToFit` verkleinert den Text, bis er in seine Box und sein Zeilenlimit passt, bis hinunter zu `MinimumFontScale` (mindestens 0,1). Formeln und andere schriftrelative Anhänge schrumpfen mit und kehren zu ihrer natürlichen Größe zurück, sobald Platz vorhanden ist; explizite Punktgrößen und Innenabstände behalten ihre festgelegten Werte.

Textfelder vererben sich wie in CSS: Ein Text innerhalb einer View mit `FontSize`, `Color` oder `LineHeight` übernimmt diese Werte, sofern er keine eigenen setzt. `TextAlign`, `TextTransform`, `LetterSpacing`, `TextDecoration`, `TextDecorationStyle`, `TextDecorationColor` sowie `TextShadowColor`, `TextShadowOffset` und `TextShadowRadius` gestalten die Zeilen und die Glyphen.

## Runs und Links

`Spans` setzt einen Text aus Runs zusammen, von denen jeder einen eigenen Style hat. Ein Span mit `OnPress` ist ein Link, und ein Druck auf einen anderen Span löst das eigene `OnPress` des Textes aus.

```go
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "By continuing you agree to the "},
	{Content: "terms", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openTerms},
	{Content: " and the "},
	{Content: "privacy policy", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openPrivacy},
	{Content: "."},
}}
```

`SpansFunc` liefert die Spans erneut, wenn sich das ändert, was es liest, und nur die abweichenden Teile werden neu vorbereitet, sodass die Zeilen eines Code-Editors neu eingefärbt werden, sobald das Highlighting eintrifft. `Background` und `CornerRadius` eines Spans zeichnen auf jeder Zeile eine Box hinter seine Zeichen, und sein `Cursor` legt das Aussehen des Zeigers über ihnen fest, mit Vorrang vor dem des Textes. `Visibility: core.VisibilityHidden` blendet einen Span an Ort und Stelle aus: Seine Wörter behalten ihren Platz, sodass die Zeilen stehen bleiben, wenn er erscheint, und Kopien sowie Screenreader lassen ihn aus – für Wörter, die nacheinander eingeblendet werden.

Setzen Sie `BackgroundInsetTop: core.Pct(62)` auf einen Span mit `Background`, um auf jeder Zeile die unteren 38 % seiner Box hervorzuheben. `BackgroundInsetBottom` beschneidet die untere Kante. Beide Einzüge nehmen Punkte oder Prozentwerte der Boxhöhe an, gemessen von der Oberlänge bis zur Unterlänge der Schrift. Sie lassen das Textlayout unverändert. Setzen Sie in TSX `backgroundInsetTop: "62%"` und `backgroundInsetBottom` im Style eines verschachtelten `<Text>`. Abgerundete Hintergründe und vertikale Einzüge werden auch unter HarmonyOS und auf den vier Mini-Program-Zielplattformen dargestellt.

`Attachment` setzt anstelle des Span-Inhalts eine View in einen Text. Sie wird in ihrer eigenen Größe angeordnet, bricht mit den umgebenden Wörtern um und nimmt ihre eigenen Drücke entgegen, sodass ein Icon, ein Bild oder ein kleiner Button in einer Zeile stehen kann. Eine Auswahl kopiert an ihrer Stelle den `Content` des Spans und lässt sie aus, wenn dieser leer ist. Standardmäßig richtet sich ihre erste Textzeile an der umgebenden Grundlinie aus, anhand der Metriken ihrer tatsächlichen Schrift. Ein `Em`-Canvas liefert seine eigene Grundlinie, und eine View ohne Grundlinie sitzt auf ihrer Unterkante. Setzen Sie `Style.VerticalAlign`, um eine andere Ausrichtung zu wählen.

```go
core.Text{Style: core.Style{FontSize: 17, LineHeight: 24}, Spans: []core.Span{
	{Content: "Search with "},
	{Attachment: core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 17}}},
	{Content: " NSGo"},
}}
```

## Schriften

`FontFamily` benennt eine Familie oder eine CSS-Liste (`Inter, "Noto Sans SC", sans-serif`), wobei jede Familie für die Zeichen verwendet wird, die den vorherigen fehlen. Jede Familie in der Liste übernimmt Gewicht und Stil. Unter macOS und iOS übernimmt eine Zeile Höhe und Grundlinie von den Schnitten, in denen ihre Zeichen gesetzt sind, sodass eine vollständig in einer späteren Familie gesetzte Zeile deren Werte übernimmt, während ein Browser die Werte der ersten Familie als Untergrenze beibehält. `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LineHeightMultiple`, `LetterSpacing` und `LetterSpacingEm` legen Schnitt und Abstände fest, und `core.FontMonospace` ist die dicktengleiche Familie der Plattform.

`Language` ist das BCP-47-Tag des Textes, das wie die obigen Felder auf Texte, Spans und Eingabefelder vererbt wird. Han-Zeichen werden in den Formen dieser Sprache gezeichnet, sodass `直` in `ja` seine japanische Form annimmt, selbst wenn das System Chinesisch bevorzugt, und Koreanisch (`ko`) bricht Zeilen zwischen Wörtern um, wie es `word-break: keep-all` in CSS tut. Texte und Eingabefelder, die keine Sprache setzen, verwenden die Sprache, in der die App angezeigt wird, und folgen ihr, wenn sie sich ändert. Setzen Sie sie auf eine View für einen Teilbaum oder auf einen `Span` für einen einzelnen Run:

```go
core.Text{Spans: []core.Span{
	{Content: "日本語: "},
	{Content: "直 骨 角", Style: core.Style{Language: "ja"}},
}}
```

Packen Sie die Schriften der App unter `resources/` und registrieren Sie sie vor der ersten View, oder registrieren Sie die Bytes einer Schrift, während die App läuft:

```go
func main() { nsgo.Run(Root, nsgo.Fonts("fonts/Inter-Regular.ttf", "fonts/Inter-Bold.ttf")) }

cx.RegisterFont(data) // a TrueType or OpenType face
```

Ein Text, der angeordnet wurde, bevor eine Schrift eintrifft, wird neu vorbereitet, und Canvases führen `Draw` erneut aus, damit ihre Glyphen und Textmessungen die geladene Schrift verwenden. `cx.Env().FontScale()` ist die Textgröße des Nutzers, der Texte folgen, sofern nicht `NoFontScaling` gesetzt ist; `MaxFontSizeMultiplier` begrenzt sie.

## Auswahl und Messung

`SelectableOn` erlaubt dem Nutzer, einen Text auszuwählen und zu kopieren, und eine `View` mit `Selectable` lässt ihre Texte gemeinsam in Dokumentreihenfolge auswählen, wie es bei einer Webseite der Fall ist. `SelectionColor` färbt die Hervorhebung ein. `OnTextSelection` empfängt eine `core.TextSelection`, während der Nutzer die Auswahl verschiebt: ihren reinen `Text`, `Markdown` und `HTML` (unter Beibehaltung der geparsten Inline- und Blockstruktur aller `TextView`s und gestalteten Texte im Bereich, einschließlich teilweiser Spans und Links), ihre `Ranges` pro Text und die `Bounds` der Texte, die sie berührt, in der View – zum Kopieren als Markdown oder HTML, für eine Wortliste oder eine Übersetzung der ausgewählten Wörter; `desktop.LookUp` zeigt sie im Wörterbuch des Systems an ([Dienste](services.md#links-und-dateien-öffnen)). `cx.MeasureText` misst eine Zeichenkette in einer Schrift, etwa für eine an ihren Text angepasste Spalte; reaktive Funktionen, die sie aufrufen, werden aktualisiert, wenn sich die verfügbaren Schriften ändern, und `OnTextLayout` meldet die Zeilen nach jedem Layout.

## Dokumente

`base/textview` zeigt Markdown oder HTML mit denselben Grundbausteinen an: Überschriften, Absätze, Listen, Tabellen, Code, Fußnoten, Hinweise, Bilder, Formeln und Links.

```go
textview.TextView{
	Markdown: "## NSGo\n\nNative views from **Go**.",
	Styles:   ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) {
		if strings.HasPrefix(e.URL, "#") {
			return
		}
		cx.OpenURL(e.URL, nil)
	},
}
```

Formeln (`$…$`, `$$…$$`) zeigen ihr TeX, bis die App `RenderMath: formula.RenderMath` aus `base/formula` übergibt, das sie in Größe und Farbe des umgebenden Textes setzt, sie in der Sprache der App vorlesen lässt und als TeX kopiert; es fügt einer Desktop-App etwa 6,5 MB und einer Web-App 3 MB (gzip-komprimiert) hinzu, die Schrift eingeschlossen. `formula.View` zeigt eine Formel an beliebiger anderer Stelle an, in TeX oder Typst; sein `OnHit`-Callback meldet für Formeleditoren den Untie-Quellteil, die nächstgelegene Einfügemarke und überlappende Diagnosen. Unter watchOS verwenden Formeln denselben Canvas-Pfad; ihr Einbinden fügt etwa 6,1 MB ausführbaren Code hinzu, einschließlich der 1,4 MB großen Schrift. Ein Lauf im Watch-Simulator maß etwa 3,9 MB gehaltenen Go-Heap für das Laden und Parsen dieser Schrift, einschließlich ihrer eigenen Byte-Kopie. Simulatormessungen sagen weder etwas über das Speicherlimit einer physischen Uhr noch über deren verfügbaren Spielraum aus; messen Sie diese auf der Uhr, auf der die App läuft. Text-Runs (`\text{}` oder Typst-Strings), die die konfigurierten Schriften nicht abdecken, verwenden die Systemschriften, einschließlich CJK und anderer Schriftsysteme; explizite `formula.Fonts.Fallbacks` haben Vorrang.

Für Quelltext, der stückweise eintrifft, übergeben Sie `SourceFunc` an `formula.View` und setzen `Streaming: true`, oder binden Sie `StreamingFunc` an den Empfangszustand der App. Dadurch werden behebbare Fehler- und Platzhalterdarstellungen ausgeblendet, während gültiger Inhalt weiter angeordnet wird. Setzen Sie das Flag zurück, wenn der Empfang abgeschlossen ist, um verbliebene Fehler anzuzeigen. Liefern Sie in einer `TextView` eine `RenderMath`-Funktion, die diese `formula.View` mit dem bereitgestellten `Math.TeX` und `Math.Display` zurückgibt; ihr `StreamingFunc` kann denselben Empfangszustand lesen.

Abgesetzte Formeln brechen an sicheren Relationen und Operatoren auf die Breite des Blocks innerhalb seiner Innenabstände und Rahmen um und behalten dabei die Schriftgröße des Textes bei. Eine Größenänderung oder eine geänderte Textgröße ordnet die Zeilen neu an. Unteilbare Teile scrollen weiterhin seitwärts; Inline-Formeln behalten ihre natürliche Größe.

Ein `mermaid`-Codeblock zeigt seinen Quelltext, bis die App `RenderDiagram: diagram.RenderDiagram` aus `base/diagram` übergibt, das ihn mit limu als natives Diagramm zeichnet: Flussdiagramme, Sequenz-, Klassen-, Zustands- und Entity-Relationship-Diagramme, Gantt-Diagramme, Kreisdiagramme, Mindmaps, Zeitleisten und mehr, in der Schrift der Plattform, hell oder dunkel, wie die App es anzeigt. Diagrammelemente mit Links öffnen diese beim Drücken über den URL-Dienst der App, und der Mermaid-Tooltip eines Elements erscheint, während der Zeiger darauf ruht. Es ordnet sich in der verfügbaren Breite des Textes an und lässt ein breites Flussdiagramm nach unten verlaufen, wobei die Größe seiner Beschriftungen erhalten bleibt; Diagramme, die nicht passen, scrollen weiterhin seitwärts. Eine Kopie übernimmt seinen Quelltext, und ein Block, der noch hereinströmt, wird als Code angezeigt, bis er vollständig ist. `diagram.View` zeigt ein Diagramm an beliebiger anderer Stelle an und folgt der Breite seines Containers, wenn sie sich ändert. Sein `Icons` ordnet Mermaid-Namen wie `"fa:fa-car"` `*core.SVG`-Dokumenten zu, einschließlich solcher aus `icons/lucide`; fehlende Namen hinterlassen keine Inline-Lücke. Bildknoten laden HTTP(S)-, Datei- oder `asset:`-Quellen über den gemeinsamen Canvas-Bildlader, eingepasst und zentriert, wenn `constraint: "on"` gesetzt ist, andernfalls gestreckt. Mermaid filtert Bildadressen vor dem Laden (und lehnt dabei unter anderem `data:`-URLs ab). Um in einer Text-View Icons bereitzustellen, setzen Sie `RenderDiagram` auf einen Callback, der `diagram.View{Source: code.Code, Icons: icons}` zurückgibt.

Um Diagrammen Ihre `ui`-Palette zu geben, geben Sie aus Ihrem `RenderDiagram`-Callback eine `diagram.View` mit `ThemeFunc` zurück. Die Funktion liest die Farben, während der Canvas zeichnet, sodass eine Änderung der Palette oder des Erscheinungsbilds eines Teilbaums die bestehende View neu einfärbt. Auf null belassene Farben behalten die Standardwerte des Diagramms.

```go
func drawDiagram(cx *core.Scope, code textview.CodeBlock) core.Node {
	colors := ui.ColorsOf(cx)
	return diagram.View{Source: code.Code, ThemeFunc: func() *diagram.Theme {
		return &diagram.Theme{
			Text: colors.Text(), Surface: colors.Surface(), Border: colors.Border(),
			Line: colors.Muted(), Accent: colors.Accent(), Critical: colors.Danger(),
		}
	}}
}
// ui.TextView{Markdown: source, RenderDiagram: drawDiagram}
```

`textview.Parse` liest Markdown und `textview.ParseHTML` liest HTML in Blöcke ein, die die App auch selbst erstellen kann. `Options.Streaming` repariert ein Dokument, das stückweise eintrifft: Es schließt, was sein Ende offen lässt, und hält eine letzte Zeile zurück, deren Bedeutung erst die nächsten Zeichen entscheiden, etwa ein alleinstehendes Listenzeichen, sodass eine Antwort nie Markierungen als Text aufblitzen lässt; `StreamingCaret` zeichnet an seinem Ende eine Einfügemarke, für eine Antwort, die während des Streamens geschrieben wird. `FadeIn` blendet jedes neue Zeichen des Dokuments nacheinander ein, nie weit hinter seinem Eintreffen, sodass eine schubweise eintreffende Antwort flüssig zu lesen ist.

Eine `textview.Search` findet Text in den Views, die sich ihr anschließen, über deren Feld `Search` oder über `textview.ProvideSearch` oberhalb von ihnen. `SetQuery`, `Open` und `Close` steuern sie aus einer Suchleiste, `Count` und `Selected` geben an, wo sie steht, und `Next` und `Previous` gehen ihre Treffer in Lesereihenfolge durch. Die Views markieren jeden Treffer mit `PartSearch` und den ausgewählten mit `PartSearchSelected`, damit die Styles der App sie einfärben können, scrollen ihn in den sichtbaren Bereich und lassen die Auswahl des Nutzers unangetastet.

```go
search := textview.NewSearch(cx)
textview.ProvideSearch(cx, search)
search.SetQuery("readme")
search.Open()
```

## Weiter

- [Bilder und Icons](images.md): Bilder und SVGs, auch innerhalb eines Textes.
- [Styles und Themes](styles.md): Farben und Textfelder, die sich vererben.
- [Layout](layout.md): wie ein Text seine Breite erhält.

## Rich-Text-Bearbeitung

`ui/richeditor.View` zeigt eine `base/richeditor.Session` im Erscheinungsbild des Themes an; `base/richeditor.View` liefert das Verhalten mit von der App bereitgestellten Styles. Unter iOS und Android wählt ein langer Druck ein Wort aus, und ein Doppeltippen bewirkt dasselbe; ziehen Sie die Auswahlgriffe, um den Bereich über Blöcke hinweg zu erweitern. Das native Bearbeitungsmenü bietet Kopieren und Alles auswählen sowie – solange die Session bearbeitbar ist – Ausschneiden und Einfügen an, jeweils abhängig von Auswahl und Zwischenablage. `Session.SetEditable` aktualisiert auch ein geöffnetes Menü. Gestalten Sie die Griffe mit `Styles.SelectionHandle` oder `Classes.SelectionHandle` auf der Basis-View.

Fokussieren Sie für ein eigenes Textsteuerelement einen `core.Input` und rufen Sie `Ref.OpenEditMenu(bounds, actions)` mit Grenzen in Oberflächenpunkten und einer Bitmaske aus `core.EditCut`, `EditCopy`, `EditPaste` und `EditSelectAll` auf. Unter iOS und Android empfängt `Input.OnEditAction` Ausschneiden, Kopieren und Alles auswählen; Einfügen verwendet `Input.OnPaste`. Rufen Sie `Ref.CloseEditMenu` auf, um es zu schließen.

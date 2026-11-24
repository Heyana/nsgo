---
title: Testo
description: Paragrafi, span, link, font e documenti.
group: Componenti
order: 32
---

`core.Text` è un paragrafo composto dal motore di testo della piattaforma. `ui.Text` e `ui.Textf` sono helper per una riga nella tipografia di `ui`; l'elemento resta un `core.Text`, quindi accetta anche `Style` e `Spans`.

```go preview=text/basic
ui.Text("Settings", 20, ui.Bold, colors.Text)
ui.Textf(func() string { return fmt.Sprintf("%d files", n.Get()) }, 13, ui.Regular, colors.Muted)
```

## Contenuto

`Content` è fisso e `ContentFunc` segue un signal. `MaxLines` limita le righe ed `EllipsizeMode` tronca il resto all'inizio, al centro o alla fine; `Style.WhiteSpaceNoWrap` mantiene un paragrafo su una sola riga. `TextAlign`, `TextTransform`, `LineHeight` e `LetterSpacing` danno stile al paragrafo, e `OnTextLayout` ne segnala le righe dopo il layout.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) },
	MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 15}}
```

## Span e link

`Spans` compone un testo di segmenti, ciascuno con uno stile proprio. Uno span con `OnPress` è un link, e i suoi `Background` e `CornerRadius` vengono disegnati dietro i suoi caratteri su ogni riga. `SpansFunc` fornisce di nuovo i segmenti quando ciò che legge cambia, per codice evidenziato o una frase che cambia.

```go preview=text/spans
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "Read the "},
	{Content: "guide", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openGuide},
	{Content: " before you start."},
}}
```

`Attachment` inserisce una vista in una riga, al posto del contenuto dello span: un'icona, un'immagine o un piccolo pulsante. Viene disposta alle proprie dimensioni, va a capo insieme alle parole e riceve le proprie pressioni.

## Font e stile

`FontFamily` indica una famiglia o un elenco CSS. `FontSize`, `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LetterSpacing`, `TextDecoration` e `TextShadowColor`, con il relativo offset e raggio, danno stile al testo. `core.FontMonospace` è la famiglia a spaziatura fissa della piattaforma.

```go
core.Text{Content: "fn main() {}", Style: core.Style{FontFamily: core.FontMonospace, FontSize: 13}}
```

Registra i font inclusi nel pacchetto con `nsgo.Fonts` prima della prima vista, oppure registra un font mentre l'app è in esecuzione con `cx.RegisterFont`. Un testo preparato prima dell'arrivo di un font viene preparato di nuovo.

## Documenti

`base/textview.TextView` mostra Markdown o HTML con le stesse primitive: intestazioni, paragrafi, elenchi, tabelle, codice, note a piè di pagina, avvisi, immagini, formule e link. `textview.Parse` legge il Markdown e `textview.ParseHTML` l'HTML, e `Options.Streaming` ripara un documento che sta ancora arrivando.

```go preview=text/document height=240
textview.TextView{Markdown: "## Notes\n\nA **native** document.", Styles: ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) { open(e.URL) }}
```

`textview.NewSearch` cerca nel testo mostrato da un `TextView` ed evidenzia le corrispondenze. Assegna la stessa ricerca a più viste con `textview.ProvideSearch`, poi chiama `Open`, `SetQuery`, `Next` e `Previous`; `Count` e `Selected` riportano il risultato. Le parti della ricerca si stilizzano con `textview.PartSearch` e `textview.PartSearchSelected`.

## Selezione e accessibilità

`SelectableOn` consente all'utente di selezionare e copiare un testo. Una `core.View` con `Selectable` fa sì che i suoi testi si selezionino insieme nell'ordine del documento. Un `Role` come `RoleHeader` indica alla tecnologia assistiva che cosa è il testo, e `Label` lo sostituisce quando le parole non costituiscono l'etichetta.

## Piattaforma

Il motore di testo della piattaforma dispone il testo, quindi interruzioni di riga, font e metodi di input lo seguono. L'andata a capo CJK, le emoji e il testo da destra a sinistra seguono il sistema. L'app può leggere la dimensione del testo con `cx.Env().FontScale()`, e un testo la segue a meno che non sia impostato `NoFontScaling`.

## Correlati

- [Testo](../text.md): la guida completa a testo, span, font e documenti.
- [Immagini e icone](../images.md): immagini e icone in linea.
- [Accessibilità](../accessibility.md): intestazioni, etichette e selezione.
- Le pagine Text e TextView della galleria.

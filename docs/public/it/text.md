---
title: Testo
description: Mostrare testo con span, font, decorazioni, selezione e documenti.
group: Concetti chiave
order: 53
---

## Un testo

`core.Text` è un paragrafo, impaginato dal motore di testo della piattaforma lato engine e disegnato dal thread principale. La sua dimensione dipende dal font e dalla larghezza, e va a capo all'interno del proprio riquadro.

```go
core.Text{Content: "Hello from NSGo", Style: core.Style{
	FontSize:   15,
	LineHeight: 22,
	FontWeight: core.FontWeightSemibold,
}}
```

`Content` è fisso, mentre `ContentFunc` segue un signal, quindi un testo che cambia lo legge all'interno della funzione. `MaxLines` limita le righe mostrate, `EllipsizeMode` tronca il resto all'inizio, al centro o alla fine, e `WhiteSpaceNoWrap` mantiene un paragrafo su una sola riga. `AdjustsFontSizeToFit` riduce il testo finché non rientra nel suo riquadro e nel suo limite di righe, fino a `MinimumFontScale` (almeno 0.1). Le formule e gli altri allegati relativi al font si riducono insieme al testo e tornano alla loro dimensione naturale quando lo spazio lo consente; le dimensioni in punti e i padding espliciti mantengono i valori impostati.

I campi del testo si ereditano come in CSS: un testo all'interno di una vista con `FontSize`, `Color` o `LineHeight` li adotta a meno che non imposti i propri. `TextAlign`, `TextTransform`, `LetterSpacing`, `TextDecoration`, `TextDecorationStyle`, `TextDecorationColor` e `TextShadowColor`, `TextShadowOffset` e `TextShadowRadius` danno stile alle righe e ai glifi.

## Segmenti e link

`Spans` compone un testo da segmenti, ciascuno con uno stile proprio. Uno span con `OnPress` è un link, mentre una pressione su un altro span attiva l'`OnPress` del testo stesso.

```go
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "By continuing you agree to the "},
	{Content: "terms", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openTerms},
	{Content: " and the "},
	{Content: "privacy policy", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openPrivacy},
	{Content: "."},
}}
```

`SpansFunc` fornisce di nuovo gli span quando ciò che legge cambia, e vengono preparate di nuovo solo le parti che differiscono, così le righe di un editor di codice si ricolorano man mano che arriva l'evidenziazione. `Background` e `CornerRadius` di uno span disegnano un riquadro dietro i suoi caratteri su ogni riga, e il suo `Cursor` indica l'aspetto del puntatore sopra di essi, con precedenza su quello del testo. `Visibility: core.VisibilityHidden` nasconde uno span sul posto: le sue parole mantengono il loro spazio, così le righe restano ferme quando appare, e le copie e gli screen reader lo escludono, per parole che compaiono in dissolvenza una alla volta.

Imposta `BackgroundInsetTop: core.Pct(62)` su uno span con un `Background` per evidenziare il 38% inferiore del suo riquadro su ogni riga. `BackgroundInsetBottom` riduce il bordo inferiore. Entrambi i rientri accettano punti o percentuali dell'altezza del riquadro, dall'ascendente al discendente del font. Non modificano l'impaginazione del testo. In TSX, imposta `backgroundInsetTop: "62%"` e `backgroundInsetBottom` nello stile di un `<Text>` annidato. Gli sfondi arrotondati e i rientri verticali vengono resi anche su HarmonyOS e sui quattro target mini-program.

`Attachment` inserisce una vista all'interno di un testo, al posto del contenuto dello span. Viene impaginata alla propria dimensione, va a capo con le parole che la circondano e riceve le proprie pressioni, così un'icona, un'immagine o un piccolo pulsante possono stare in una riga. Una selezione copia il `Content` dello span al suo posto, omettendolo se vuoto. Per impostazione predefinita la sua prima riga di testo si allinea alla linea di base circostante, usando le metriche del font effettivo. Un canvas `Em` fornisce una propria linea di base, mentre una vista senza linea di base poggia sul proprio bordo inferiore. Imposta `Style.VerticalAlign` per scegliere un altro allineamento.

```go
core.Text{Style: core.Style{FontSize: 17, LineHeight: 24}, Spans: []core.Span{
	{Content: "Search with "},
	{Attachment: core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 17}}},
	{Content: " NSGo"},
}}
```

## Font

`FontFamily` indica una famiglia o un elenco CSS (`Inter, "Noto Sans SC", sans-serif`), ciascuna usata per i caratteri che mancano a quelle che la precedono. Ogni famiglia dell'elenco adotta il peso e lo stile. Su macOS e iOS una riga prende l'altezza e la linea di base dai caratteri tipografici in cui sono composti i suoi caratteri, quindi una riga composta interamente in una famiglia successiva adotta quelle di tale famiglia, mentre un browser mantiene quelle della prima famiglia come valore minimo. `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LineHeightMultiple`, `LetterSpacing` e `LetterSpacingEm` impostano il carattere tipografico e la spaziatura, e `core.FontMonospace` è la famiglia a spaziatura fissa della piattaforma.

`Language` è il tag BCP 47 del testo, ereditato come i campi precedenti su testi, span e input. I caratteri Han vengono disegnati nelle sue forme, quindi `直` impostato in `ja` assume la forma giapponese anche dove il sistema preferisce il cinese, e il coreano (`ko`) va a capo tra le parole come fa `word-break: keep-all` in CSS. I testi e gli input che non ne impostano uno usano la lingua mostrata dall'app e la seguono quando cambia. Impostalo su una vista per un sottoalbero, oppure su uno `Span` per un singolo segmento:

```go
core.Text{Spans: []core.Span{
	{Content: "日本語: "},
	{Content: "直 骨 角", Style: core.Style{Language: "ja"}},
}}
```

Includi i font dell'app in `resources/` e registrali prima della prima vista, oppure registra i byte di un font mentre l'app è in esecuzione:

```go
func main() { nsgo.Run(Root, nsgo.Fonts("fonts/Inter-Regular.ttf", "fonts/Inter-Bold.ttf")) }

cx.RegisterFont(data) // a TrueType or OpenType face
```

Un testo impaginato prima dell'arrivo di un font viene preparato di nuovo, e i canvas eseguono di nuovo `Draw` affinché i loro glifi e le misurazioni del testo usino il font caricato. `cx.Env().FontScale()` è la dimensione del testo scelta dall'utente, che i testi seguono a meno che non sia impostato `NoFontScaling`; `MaxFontSizeMultiplier` ne fissa un limite massimo.

## Selezione e misurazione

`SelectableOn` consente all'utente di selezionare e copiare un testo, e una `View` con `Selectable` fa sì che i suoi testi vengano selezionati insieme, nell'ordine del documento, come avviene in una pagina web. `SelectionColor` colora l'evidenziazione. `OnTextSelection` riceve una `core.TextSelection` mentre l'utente sposta la selezione: il suo `Text` semplice, `Markdown` e `HTML` (che preservano la struttura inline e a blocchi analizzata di eventuali `TextView` e testi con stile nell'area, inclusi span parziali e link), i suoi `Ranges` per ciascun testo e i `Bounds` dei testi che tocca, nella vista, per copiare come Markdown o HTML, un elenco di parole o una traduzione delle parole selezionate; `desktop.LookUp` le mostra nel dizionario del sistema ([servizi](services.md#aprire-link-e-file)). `cx.MeasureText` misura una stringa in un font, per una colonna adattata al suo testo; le funzioni reattive che la chiamano si aggiornano quando cambiano i font disponibili, e `OnTextLayout` riporta le righe dopo ogni impaginazione.

## Documenti

`base/textview` mostra Markdown o HTML con le stesse primitive: titoli, paragrafi, elenchi, tabelle, codice, note a piè di pagina, avvisi, immagini, formule e link.

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

Le formule (`$…$`, `$$…$$`) mostrano il loro TeX finché l'app non passa `RenderMath: formula.RenderMath` da `base/formula`, che le compone nella dimensione e nel colore del testo circostante, le legge ad alta voce nella lingua dell'app e le copia come TeX; aggiunge circa 6.5 MB a un'app desktop e 3 MB compressi con gzip a un'app web, font incluso. `formula.View` mostra una formula in qualsiasi altro punto, in TeX o Typst; la sua callback `OnHit` riporta la parte di sorgente Untie, il cursore più vicino e le diagnostiche sovrapposte, per gli editor di formule. Su watchOS le formule usano lo stesso percorso del canvas; collegarle aggiunge circa 6.1 MB all'eseguibile, incluso il font da 1.4 MB. Un'esecuzione sul simulatore di watch ha misurato circa 3.9 MB di heap Go trattenuto per il caricamento e l'analisi di quel font, inclusa la copia dei byte di sua proprietà. Le misurazioni sul simulatore non stabiliscono né il limite di memoria né il margine disponibile di un watch fisico; misurali sul watch che esegue l'app. I segmenti di testo (`\text{}` o stringhe Typst) che i font configurati non coprono usano i font di sistema, inclusi CJK e altri sistemi di scrittura; i `formula.Fonts.Fallbacks` espliciti hanno la precedenza.

Per una sorgente che arriva a pezzi, passa `SourceFunc` a `formula.View` e imposta `Streaming: true`, oppure collega `StreamingFunc` allo stato di ricezione dell'app. In questo modo si nascondono gli errori recuperabili e i segnaposto, mentre il contenuto valido continua a essere impaginato. Disattiva il flag al termine della ricezione per mostrare gli eventuali errori rimasti. In una `TextView`, fornisci una funzione `RenderMath` che restituisca quella `formula.View` con i `Math.TeX` e `Math.Display` forniti; il suo `StreamingFunc` può leggere lo stesso stato di ricezione.

Le formule in display vanno a capo in corrispondenza di relazioni e operatori sicuri, alla larghezza del blocco all'interno di padding e bordi, mantenendo la dimensione del font del testo. Ridimensionare o cambiare la dimensione del testo ridistribuisce le righe. Le parti non divisibili scorrono comunque lateralmente; le formule inline mantengono la loro dimensione naturale.

Un blocco di codice `mermaid` mostra la sua sorgente finché l'app non passa `RenderDiagram: diagram.RenderDiagram` da `base/diagram`, che lo disegna come diagramma nativo, con limu: diagrammi di flusso, di sequenza, delle classi, degli stati ed entità-relazione, diagrammi di Gantt, grafici a torta, mappe mentali, linee temporali e altro ancora, nel font della piattaforma, chiaro o scuro come mostra l'app. Gli elementi del diagramma con link li aprono tramite il servizio URL dell'app quando vengono premuti, e il tooltip Mermaid di un elemento appare mentre il puntatore vi resta sopra. Si impagina alla larghezza disponibile del testo, disponendo verticalmente un diagramma di flusso largo pur mantenendo la dimensione delle etichette; i diagrammi che non possono rientrare scorrono comunque lateralmente. Una copia ne prende la sorgente, e un blocco ancora in arrivo viene mostrato come codice finché non è completo. `diagram.View` mostra un diagramma in qualsiasi altro punto e segue la larghezza del suo contenitore man mano che cambia. Il suo `Icons` associa nomi Mermaid come `"fa:fa-car"` a documenti `*core.SVG`, inclusi quelli di `icons/lucide`; i nomi assenti non lasciano spazi vuoti inline. I nodi immagine caricano sorgenti HTTP(S), file o `asset:` tramite il caricatore di immagini condiviso del canvas, adattandole e centrandole quando è impostato `constraint: "on"` e stirandole altrimenti. Mermaid filtra gli indirizzi delle immagini prima del caricamento (rifiutando anche gli URL `data:`). Per fornire icone in una text view, imposta `RenderDiagram` su una callback che restituisca `diagram.View{Source: code.Code, Icons: icons}`.

Per dare ai diagrammi la tua palette `ui`, restituisci una `diagram.View` con `ThemeFunc` dalla tua callback `RenderDiagram`. La funzione legge i colori mentre il canvas disegna, quindi un cambio di palette o di aspetto del sottoalbero ricolora la vista esistente. I colori lasciati a zero mantengono i valori predefiniti del diagramma.

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

`textview.Parse` legge Markdown e `textview.ParseHTML` legge HTML, producendo blocchi che l'app può anche costruire da sé. `Options.Streaming` ripara un documento che arriva a pezzi, chiudendo ciò che la sua fine lascia aperto e trattenendo un'ultima riga il cui significato dipende dai caratteri successivi, come un marcatore di elenco isolato, così una risposta non mostra mai per un istante i marcatori come testo, e `StreamingCaret` disegna un cursore alla sua fine, per una risposta scritta mentre arriva in streaming.

Una `textview.Search` trova il testo nelle viste che vi aderiscono, tramite il loro campo `Search` o `textview.ProvideSearch` posto sopra di esse. `SetQuery`, `Open` e `Close` la controllano da una barra di ricerca, `Count` e `Selected` indicano a che punto si trova, e `Next` e `Previous` scorrono le corrispondenze nell'ordine di lettura. Le viste contrassegnano ogni corrispondenza con `PartSearch` e quella selezionata con `PartSearchSelected`, affinché gli stili dell'app le colorino, la portano in vista con lo scorrimento e lasciano intatta la selezione dell'utente.

```go
search := textview.NewSearch(cx)
textview.ProvideSearch(cx, search)
search.SetQuery("readme")
search.Open()
```

## Prossimi passi

- [Immagini e icone](images.md): immagini e SVG, anche all'interno di un testo.
- [Stili e temi](styles.md): colori e campi del testo che si ereditano.
- [Layout](layout.md): come un testo ottiene la propria larghezza.

## Modifica del testo formattato

`ui/richeditor.View` mostra una `base/richeditor.Session` con l'aspetto del tema; `base/richeditor.View` fornisce il comportamento con stili forniti dall'app. Su iOS e Android, una pressione prolungata seleziona una parola e un doppio tocco fa lo stesso; trascina le maniglie di selezione per estendere l'intervallo tra più blocchi. Il menu di modifica nativo offre Copia e Seleziona tutto, e Taglia e Incolla mentre la sessione è modificabile, a seconda della selezione e degli appunti. `Session.SetEditable` aggiorna anche un menu aperto. Dai stile alle maniglie con `Styles.SelectionHandle` o `Classes.SelectionHandle` sulla vista base.

Per un controllo di testo personalizzato, porta il focus su un `core.Input` e chiama `Ref.OpenEditMenu(bounds, actions)` con i limiti in punti della superficie e una maschera di bit di `core.EditCut`, `EditCopy`, `EditPaste` ed `EditSelectAll`. Su iOS e Android, `Input.OnEditAction` riceve Taglia, Copia e Seleziona tutto; Incolla usa `Input.OnPaste`. Chiama `Ref.CloseEditMenu` per chiuderlo.

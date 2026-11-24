---
title: Layout
description: Flexbox, grid, dimensioni e posizionamento, con l'area sicura e la tastiera nel layout.
group: Concetti chiave
order: 51
---

## Riquadri

Ogni elemento è un riquadro disposto dal motore. I valori predefiniti sono quelli di React Native: un flexbox a colonna, figli che mantengono le proprie dimensioni a meno che non crescano o si restringano, elementi allungati lungo l'asse trasversale e dimensioni che includono padding e bordi.

```go
core.View{
	Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.All(16), AlignItems: core.AlignCenter},
	Children: []core.Node{
		core.Text{Content: "One"},
		core.Text{Content: "Two"},
	},
}
```

`Column`, `Row`, `ColumnReverse` e `RowReverse` impostano l'asse principale; `Wrap` e `WrapReverse` lasciano che i figli vadano su più righe. `JustifyContent` li posiziona lungo quell'asse e `AlignItems`, oppure `AlignSelf` su un singolo figlio, trasversalmente. `Gap`, `RowGap` e `ColumnGap` li separano, e padding e margin di `style` usano `core.All`, `core.Symmetric` o `core.Edges`.

## Dimensioni

`Width`, `Height`, `MinWidth`, `MaxWidth` e `FlexBasis` accettano `core.Pt` per i punti o `core.Pct` per una percentuale del genitore. `FlexGrow` ripartisce lo spazio libero e `FlexShrink` permette a un riquadro di cederne una parte; un riquadro senza nessuno dei due mantiene le proprie dimensioni. `AspectRatio` è la larghezza divisa per l'altezza. `AutoMargin` prende lo spazio libero sui lati che indica, così un lato spinge il riquadro verso l'altro.

```go
core.View{Style: core.Style{Direction: core.Row, Gap: 16}, Children: []core.Node{
	core.View{Style: core.Style{Width: core.Pt(220), FlexShrink: 0}},
	core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}},
}}
```

È il contenuto di un riquadro a determinarne le dimensioni, finché una larghezza o un'altezza non dice altrimenti. `core.MinContent`, `core.MaxContent` e `core.FitContent` corrispondono alle dimensioni definite dal CSS: la larghezza min-content di un testo è la sua parola più larga. Un testo all'interno di una riga si restringe e va a capo solo con `FlexShrink: 1`; aggiungi `FlexGrow: 1` quando deve occupare lo spazio rimanente. Un minimo non impostato vale già 0, come in React Native, quindi `MinWidth: core.Pt(0)` non cambia nulla e un testo che si restringe spezza le parole quando la riga è più stretta della sua parola più larga. `MinWidth: core.AutoMinimum` lo ferma alla sua parola più larga, come fa il minimo automatico del CSS.

Una schermata che riempie la finestra usa `FlexGrow: 1` e `FlexShrink: 1`, così come ogni riquadro tra essa e una lista o una vista a scorrimento che deve scorrere. Con il solo `FlexGrow`, un riquadro cresce fino al suo contenuto, per cui una lista lunga lo allunga oltre la finestra invece di scorrere.

## Grid

`DisplayGrid` dispone i figli secondo il CSS grid. `GridTemplateColumns` e `GridTemplateRows` accettano la sintassi delle tracce CSS, e `GridRow` e `GridColumn` posizionano un elemento per linea o per span.

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

`repeat(auto-fill, minmax(160px, 1fr))` crea tutte le colonne che ci stanno. `GridFlow` sceglie l'ordine in cui la griglia si riempie, e `JustifyItems`, `JustifySelf` e `AlignItems` posizionano gli elementi nelle loro celle.

## Posizione

`Relative` è il valore predefinito e mantiene un riquadro nel flusso; `Top`, `Right`, `Bottom` e `Left` lo spostano. `Absolute` lo toglie dal flusso e lo posiziona nel suo antenato posizionato più vicino. `Fixed` lo posiziona nella vista radice o nel suo modale, qualunque cosa scorra, e `Sticky` lo fissa all'interno del suo genitore nella vista a scorrimento più vicina: `Top` dall'alto, oppure `Bottom` dal basso quando `Top` è auto (`Left` e `Right` in orizzontale). Un piè di pagina con `Position: core.Sticky, Bottom: core.Pt(0)` resta in fondo mentre un contenuto lungo scorre, e segue un contenuto breve. `ZIndex` impila i riquadri rispetto ai loro fratelli.

```go
core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Right: core.Pt(8)}}
```

`Static` mantiene un riquadro nel flusso e ignora i suoi offset, così i figli con posizionamento assoluto trovano un antenato al di sopra di esso. `Overflow` ritaglia i figli entro un riquadro, e `DisplayNone` toglie un riquadro dal layout.

Le ombre esterne e il contorno del riquadro stesso restano fuori dal suo ritaglio, come nel CSS, su Android, Windows, Linux, il web e Apple, dove ciò vale per ogni tipo di vista, compresi angoli con raggi diversi e contenuti a scorrimento ingranditi. Finché ciò non sarà verificato su un iPhone fisico, il `Bare` di `ui.Dialog` mette ancora il proprio ritaglio su una vista interna con gli stessi angoli, rientrata della larghezza del bordo, che li mantiene su ogni target:

```go
core.View{Style: core.Style{CornerRadius: 16, Shadows: []core.BoxShadow{ui.Shadow(0, 8, 24, -6, ui.Black(0.18))}},
	Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, CornerRadius: 16, Overflow: core.Hidden}, Children: content}}}
```

## Direzione e ambiente

`LayoutDirection` dispone un sottoalbero da sinistra a destra o da destra a sinistra, al di sopra della direzione dell'app: `DirectionRTL` e `DirectionLTR` la impostano, e il valore predefinito la eredita. Sinistra e destra in margini, padding, bordi e offset si scambiano con la direzione, quindi sono inizio e fine come in React Native. `PhysicalEdges` e `PhysicalBorders` mantengono i lati indicati dove sono. `WritingDirection` ordina i tratti di testo a direzione mista, e `WritingAuto` la ricava dalla prima lettera forte.

`PaddingEnv` e `MarginEnv` aggiungono l'ambiente a un lato. `core.EnvSafeArea` è il rientro dell'area sicura su quel bordo e `core.EnvKeyboard` l'altezza della tastiera software, così il contenuto resta sopra la tastiera e si sposta con la sua animazione.

```go
core.View{Style: core.Style{
	Padding: core.All(16),
	PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea},
}}
```

`core.EnvAll(core.EnvSafeArea)` indica tutti i bordi, e `cx.Env().SafeArea()` e `cx.Env().KeyboardHeight()` leggono i valori nel codice.

Su un telefono pieghevole o a doppio schermo, `cx.Env().Geometry()` elenca le regioni in cui una piega o una cerniera divide la finestra, ciascuna con l'area sicura che vi ricade, e le pieghe tra di esse; i suoi `State` e `Angle` indicano quanto è aperto il dispositivo (`core.HingeClosed`, `HingeHalfOpen` o `HingeOpen`) anche quando nessuna piega attraversa la finestra, come su un display esterno o uno schermo aperto in piano. Gli altri dispositivi riportano una sola regione e `HingeUnknown`. Su iOS 27.1, `cx.Env().VerticalBarEdge()` riporta il bordo che il sistema ha scelto per una barra verticale (`core.VerticalBarLeading`, `core.VerticalBarTrailing` o `core.VerticalBarUnspecified`), che `mobile.TabBar` e `ui.TabShell` seguono quando `Edge` è `mobile.TabBarAutomatic`. Nulla viene disposto implicitamente in base alla geometria: è l'app a scegliere i propri pannelli.

## Classi responsive

`Class` accetta le classi di Tailwind CSS: `flex`, `flex-col`, `flex-wrap`, `items-center`, `justify-between`, `gap-3`, `p-4`, `w-full`, `grid`, `grid-cols-3`, `col-span-2` e le altre. I breakpoint seguono la finestra, mentre l'aspetto del display, la direzione e la riduzione del movimento seguono l'ambiente:

```go
core.View{Class: "flex flex-col gap-3 p-4 md:flex-row md:gap-6 dark:bg-zinc-900 rtl:flex-row-reverse"}
```

`sm:`, `md:`, `lg:`, `xl:` e `2xl:` si applicano da 40, 48, 64, 80 e 96 rem di larghezza; `max-md:` si applica al di sotto di una soglia. `portrait:`, `dark:`, `rtl:`, `motion-reduce:`, `contrast-more:`, `pointer-fine:`, `pointer-coarse:`, `ios:`, `android:` e `web:` seguono ciò che indica l'ambiente. Le classi si fondono sotto uno `Style`, e la classe di uno stato (`hover:`) diventa uno stile di stato, così i due possono essere combinati.

## Prossimi passi

- [Stili e temi](styles.md): colori, angoli, ombre e l'aspetto di un sottoalbero.
- [Testo](text.md): come il testo assume le proprie dimensioni e va a capo.
- [Liste e scorrimento](lists.md): contenuti che scorrono e righe che vengono riciclate.

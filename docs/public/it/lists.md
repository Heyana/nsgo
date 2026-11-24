---
title: Liste e scorrimento
description: Viste di scorrimento con gesti nativi e liste che riciclano le righe per qualsiasi numero di elementi.
group: Concetti chiave
order: 55
---

## Una vista di scorrimento

`core.Scroll` è la vista di scorrimento propria della piattaforma, quindi lanci, rimbalzi, barra di scorrimento, aggancio e scorrimento al tocco sono nativi. I suoi figli vengono disposti in una colonna, o in una riga con `Horizontal`, per tutta la lunghezza di cui hanno bisogno.

```go
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

La vista di scorrimento riempie il suo contenitore e il padding va applicato al suo contenuto, così la barra di scorrimento resta a ridosso del bordo. Si restringe per adattarsi al genitore, a meno che il suo stile non indichi altrimenti. Imposta `NoShrink` quando deve mantenere la dimensione richiesta mentre un elemento fratello si restringe. `HideIndicator` nasconde l'indicatore della piattaforma e `Bar` disegna quello dell'app.

`Behavior` e `BehaviorFunc` scelgono come scorre: `Paging`, `SnapToInterval` e `SnapToOffsets` con `SnapToAlignment`, un `DecelerationRate`, `NoBounces`, `ContentInset` e le regole della tastiera. `MinZoomScale` e `MaxZoomScale` consentono lo zoom con il pizzico, `OnRefresh` e `Refreshing` la aggiornano con un trascinamento verso il basso e `OffsetValues` segue il suo offset sul thread principale per gli effetti che si muovono insieme a essa.

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

`OnScroll` segnala l'offset al massimo una volta per turno, o una volta ogni `ScrollEventThrottle`. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd` e `OnScrollEnd` delimitano i gesti dell'utente. `Ref` scorre da codice con `ScrollTo`, `ScrollToEnd`, `ZoomTo` e, per una lista, `ScrollToIndex`.

## Una lista

`core.List` mostra dati lunghi su una vista di scorrimento nativa, creando solo le righe vicine allo schermo. `Count` indica quante righe ci sono e `Row` ne costruisce una; `Key` dà un'identità a ciascuna riga, così le righe che restano mantengono le proprie celle mentre altre righe compaiono e scompaiono sopra di esse.

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

`Row` viene eseguita una volta per ogni cella riciclata e aggiorna `r.Index()` quando quella cella viene riutilizzata per un'altra riga, quindi leggi `r.Index()` e i signal degli elementi all'interno di funzioni reattive (`ContentFunc`, `StyleFunc`, `NewMemo`) invece di catturare un valore una volta sola mentre costruisci la riga. Le celle inattive vengono parcheggiate, così le loro computazioni attendono finché la cella non viene di nuovo associata a una riga attiva.

`Header`, `Footer`, `Empty` e `Separator` contengono il contenuto attorno e tra le righe. `NumColumns` affianca le righe, `Gap` e `ColumnGap` le separano e `Horizontal` scorre in orizzontale. `StickyHeaderIndices` fissa quelle righe all'inizio del viewport finché la successiva non le spinge fuori, e `StickyHeaderOffset` lascia libero un header posto sopra di esse.

`OnViewableItemsChange` segnala le righe visibili secondo le soglie di `ViewabilityConfig`, e `OnLoadMore` viene eseguita quando il viewport si avvicina alla fine, una volta per ogni numero di righe. `OnLoadMoreAtStart` fa lo stesso all'inizio, e `InitialScrollIndex` apre la lista su una riga. `Alignment: core.ListAlignEnd` fa poggiare una lista sulla sua fine, come fa una chat con le righe in ordine di lettura: si apre sulla riga più recente, segue le nuove righe mentre l'utente si trova alla fine e tiene una conversazione breve in fondo. `Inverted` mette invece la riga 0 alla fine, e `MaintainVisibleContentPosition` segue la fine di una lista che cresce mentre l'utente si trova vicino a essa.

```go
core.List{Ref: list, Count: count, EstimatedSize: 64, Row: row, OnLoadMore: loadMore,
	StickyHeaderIndices: headers(cx), Footer: core.View{Style: core.Style{Height: core.Pt(24)}}}
```

`list.ScrollToIndexAt(index, 0.5, 0, true)` scorre una riga fino al centro del viewport, e `list.ScrollToEnd(true)` fino alla fine. `list.ListPosition()` legge dove inizia il viewport, cioè una riga e quanto all'interno di essa, per conservare una posizione di lettura: `list.ScrollToIndexAt(p.Index, 0, -p.Offset, false)` riporta lì una lista creata da capo. Una lista può anche stare in una pagina insieme ad altri contenuti: imposta `Viewport: core.ListViewportParent` e la vista di scorrimento che la circonda scorre la lista insieme al resto.

## Tabelle

`base/datatable` mostra una tabella di un milione di righe con righe e colonne virtualizzate, un header fisso, colonne bloccate a sinistra (`Fixed`) o a destra (`FixedRight`) mentre le altre scorrono sotto di esse, ordinamento, selezione di una o più righe, ridimensionamento e spostamento delle colonne e celle proprie dell'app. `MultiSelect` supporta l'attivazione e disattivazione con Command o Control, gli intervalli con Shift e le estensioni da tastiera con Shift, mentre `SelectedRows` e `OnSelectedRowsChange` permettono all'app di controllare le righe. `CellTooltip` assegna un tooltip a una cella, che descrive la cella anche agli screen reader, con lo stile definito da `Styles.CellTooltip`; ciò che `RenderCell` disegna non riceve il puntatore, quindi un `ui.Tooltip` all'interno di una cella non viene mai mostrato. `base.Table` è una semplice tabella delle righe dell'app per dati più piccoli, e `base/plot` disegna grafici.

Usa `DataTable.LabelFunc` per un nome della griglia che segue la lingua o lo stato dell'applicazione. `ColumnGroup.LabelFunc` aggiorna l'intestazione di una colonna raggruppata mentre il gruppo resta visibile.

## Prossimi passi

- [Animazioni](animation.md): effetti che seguono una vista di scorrimento.
- [Testo](text.md): righe di testo che vanno a capo.
- [Navigazione](navigation.md): schermate il cui contenuto scorre.

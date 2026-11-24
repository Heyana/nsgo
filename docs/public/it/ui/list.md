---
title: List
description: Righe in qualsiasi numero, costruite solo quando sono visibili, con selezione e attivazione.
group: Componenti
order: 38
---

`ui.List` mostra le righe su una vista di scorrimento nativa e costruisce solo quelle vicine allo schermo, con selezione, attivazione e tastiera.

```go preview=list/basic height=280
ui.List{
	Count:         func() int { return len(items) },
	Key:           func(i int) any { return items[i].ID },
	EstimatedSize: 44,
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		item := items[row.Index()]
		return ui.Text(item.Title, 13, ui.Regular, colors.Text)
	},
}
```

## Righe e identità

`Count` indica quante righe ci sono e `Row` ne costruisce una. `Key` dà a una riga la sua identità: le righe che restano mantengono le proprie celle e il proprio stato mentre altre righe compaiono e scompaiono sopra di loro. Usa un ID anziché l'indice quando le righe possono essere aggiunte, rimosse o riordinate; l'indice è un signal che segue la cella quando questa mostra un'altra riga.

```go
ui.List{Count: count, Key: func(i int) any { return items[i].ID }, Row: func(cx *core.Scope, row core.ListRow) core.Node {
	return ui.Text(items[row.Index()].Title, 13, ui.Regular, colors.Text)
}}
```

`EstimatedSize` è l'altezza prevista di una riga, 50 punti se è 0. Le righe possono avere altezze diverse; l'elenco misura quelle che costruisce e stima le altre.

## Layout

`Header`, `Footer` ed `Empty` contengono il contenuto attorno alle righe e quello mostrato quando non ce ne sono. `Gap` separa le righe, `Columns` le dispone affiancate e il padding dell'elenco è quello del contenuto, così la barra di scorrimento resta sul bordo. Assegna un'altezza all'elenco oppure lascia che riempia il contenitore padre.

## Selezione e attivazione

`ui.List` accetta una `Selection`, creata con `ui.NewSelection(cx, multiple)`. Chiavi, etichette, attivazione e tastiera la seguono: Su, Giù, Home e Fine spostano il cursore, Maiusc estende, Mod+A seleziona tutto, Spazio attiva o disattiva in una selezione multipla e Invio o un doppio clic attiva una riga.

```go preview=list/selection height=320
selected := ui.NewSelection(cx, true)
ui.List{
	Count: count, Key: func(i int) any { return items[i].ID },
	Selection: selected,
	OnActivate: func(i int) { open(items[i]) },
	RowLabel:   func(i int) string { return items[i].Title },
	Row: func(cx *core.Scope, row core.ListRow) core.Node { return rowContent(cx, items[row.Index()]) },
}
```

`OnSelectionChange` viene eseguito dopo che una pressione, un tasto o la ricerca durante la digitazione cambiano le righe scelte, e non quando l'app imposta la `Selection` da sé, così un riquadro di dettaglio può seguire la scelta dell'utente mentre l'app sposta la selezione su un elemento aperto in altro modo.

`TapSelects` fa sì che un tocco su un telefono selezioni anziché attivare. `RowLabel` è ciò che la tecnologia assistiva legge e ciò su cui si basa la ricerca durante la digitazione.

## Caricamento e scorrimento

`OnLoadMore` viene eseguito quando la fine si avvicina, una volta per ogni numero di righe. `Ref` scorre con `ScrollToIndexAt`, `ScrollToEnd` e `ScrollToIndex`. `core.List`, l'elenco semplice sottostante, offre anche `OnLoadMoreAtStart`, `Inverted` per una chat, `StickyHeaderIndices`, `MaintainVisibleContentPosition`, `Viewport` e `ViewabilityConfig` per i casi di basso livello ([Elenchi e scorrimento](../lists.md)).

## Accessibilità

L'elenco è un unico elenco o listbox, con il nome dato da `Label`, e il nome di una riga è il suo testo o `RowLabel`. Un elenco con selezione segnala la riga del cursore e la annuncia mentre la tastiera si sposta. Uno screen reader che si sposta fuori dalla vista chiede all'elenco di scorrere fin lì, il che costruisce le righe raggiunte.

## Piattaforma

La vista di scorrimento nativa e la virtualizzazione sono le stesse su ogni target. Le righe vengono riciclate, quindi lo stato di una riga deve seguire la sua `Key`; mantenere lo stato di costruzione nell'app anziché in una riga è la regola consueta.

## Correlati

- [Elenchi e scorrimento](../lists.md): la guida completa, inclusi gli header fissi e il caricamento progressivo.
- [Scroll](scroll.md): una semplice vista di scorrimento.
- [DataTable](../lists.md#tabelle): una tabella con molte righe e colonne.
- Le pagine List, Table e DataTable della galleria.

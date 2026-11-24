---
title: Select
description: Un valore da un elenco, come popup o come pannello inferiore.
group: Componenti
order: 36
---

`ui.Select[T]` sceglie uno tra un elenco di valori di qualsiasi tipo confrontabile. L'elenco si apre sotto il trigger su ogni dispositivo; `Presentation: base.PresentSheet` lo apre invece come pannello dal basso, come potrebbe desiderare un'app per telefono.

```go preview=select/basic height=280
size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options: []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:   size.Get,
	OnChange: size.Set,
	Label:   "Size",
}
```

## Opzioni e ricerca

Le `Options` sono delle `ui.Option[T]`, ciascuna con un `Value`, una `Label` e, facoltativamente, un `Group`, un'`Icon`, una funzione `Disabled` o un `Content` proprio dell'app. Le opzioni adiacenti appartenenti allo stesso `Group` non vuoto compaiono sotto un'intestazione del gruppo, in una `Select` o in una `Combobox`, che i tasti freccia e la digitazione saltano e che gli screen reader leggono come titolo. Usa `OptionsFunc` quando le opzioni seguono un signal, così l'elenco aperto si aggiorna sul posto e mantiene il valore e il focus del campo. `Placeholder` viene mostrato finché non è scelta alcuna opzione. `Searchable` filtra l'elenco digitando, `OnClear` mostra un pulsante per cancellare e `Disabled` la disattiva.

```go preview=select/searchable height=400
ui.Select[string]{
	Options:  languages,
	Value:    language.Get,
	OnChange: language.Set,
	Searchable: true,
	SearchPlaceholder: "Search languages",
	OnClear:  func() { language.Set("") },
	Placeholder: "Choose a language",
}
```

`Style` dispone la select, larga 200 punti a meno che non imposti una larghezza; la sua altezza, il padding e il carattere arrivano al campo, come fanno quelli di un [Input](input.md), così una select si allinea a un pulsante `SizeSmall`:

```go
ui.Select[string]{Options: sizes, Value: size.Get, OnChange: size.Set, Label: "Size",
	Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
```

## Comportamento

Su, Giù, Invio e Spazio aprono l'elenco dal trigger con il focus; le frecce spostano l'evidenziazione e la ricerca durante la digitazione salta in base ai titoli delle voci. Scegliere una voce chiude l'elenco e ne segnala il valore. Esc, il tasto indietro e una pressione all'esterno lo chiudono senza modifiche. `Empty` è il testo mostrato quando la ricerca non trova nulla.

## Accessibilità

`Label` dà un nome al campo per la tecnologia assistiva. Usa `LabelFunc` per aggiornare quel nome sul posto quando i suoi signal cambiano. Un'etichetta vuota usa `Placeholder`. Il trigger e l'elenco portano i ruoli della select e la voce selezionata viene segnalata. La tastiera raggiunge il trigger con Tab e lo apre con Invio o Spazio.

## Piattaforma

Su macOS, Windows e Linux l'elenco è un popup accanto al trigger. Su iOS e Android è un pannello dal basso e la riga selezionata resta sul campo. Le stesse opzioni e gli stessi valori funzionano su ogni target.

## Correlati

- [Moduli](../forms.md): select in un modulo.
- [Menu](menu.md): comandi anziché un valore.
- [Tabs](tabs.md): poche opzioni mostrate contemporaneamente.
- Le pagine Select e Combobox della galleria.
- [Componenti senza stile](../authoring/base.md#select): `base.Select`, con gruppi e un trigger tutto tuo.

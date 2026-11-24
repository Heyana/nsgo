---
title: Scroll
description: La vista di scorrimento della piattaforma, con paginazione, aggancio, zoom e aggiornamento.
group: Componenti
order: 39
---

`core.Scroll` è la vista di scorrimento propria della piattaforma. I suoi figli vengono disposti in una colonna, o in una riga con `Horizontal`, e la vista riempie il proprio contenitore mentre il contenuto scorre.

```go preview=scroll/basic height=240
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

Il padding va sul contenuto, non sulla vista di scorrimento, così la barra di scorrimento resta contro il bordo. La vista di scorrimento si restringe per adattarsi al contenitore padre, a meno che il suo stile non indichi diversamente. Imposta `NoShrink` quando deve mantenere la dimensione richiesta mentre un elemento fratello si restringe.

## Comportamento

`Behavior` sceglie come scorre: `Paging`, `SnapToInterval` o `SnapToOffsets` con `SnapToAlignment`, un `DecelerationRate`, `NoBounces`, `ContentInset` e le regole della tastiera.

```go preview=scroll/paging height=240
core.Scroll{
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: pages,
}
```

`HideIndicator` nasconde la barra di scorrimento della piattaforma e `Bar` disegna quella propria dell'app. `MinZoomScale` e `MaxZoomScale` consentono lo zoom con il pizzico; `NoPinchZoom` li ignora. `CenterContent` centra il contenuto più piccolo dell'area visibile.

## Aggiornamento ed eventi

`OnRefresh` rende la vista aggiornabile; imposta `Refreshing` mentre il lavoro è in corso. `PullValue` segue quanto il contenuto viene tirato, per un indicatore proprio dell'app, e `Behavior.RefreshIndicator` ne imposta l'altezza.

```go
refreshing := core.NewSignal(cx, false)
core.Scroll{
	Refreshing: refreshing.Get,
	OnRefresh:  func() { refreshing.Set(true); reload(cx) },
	Children:   content,
}
```

`OnScroll` segnala l'offset al massimo una volta per turno, o una volta per `ScrollEventThrottle`; `Stale` contrassegna un offset precedente all'applicazione, da parte della piattaforma, di uno scorrimento avviato dal codice. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd` e `OnScrollEnd` delimitano i gesti dell'utente.

`OffsetValues` segue l'offset sul thread principale, per un header che si comprime senza un commit per ogni frame. `Ref` scorre dal codice con `ScrollTo`, `ScrollToEnd` e `ZoomTo`, e il ref di qualsiasi vista al suo interno la porta in vista con `Reveal`, facendo scorrere ogni vista di scorrimento che la circonda:

```go
row.Reveal(core.RevealOptions{Vertical: core.ScrollAlignCenter, Animated: true})
```

`RevealBox` porta invece in vista un riquadro all'interno della vista, come il riquadro di una parola ottenuto da `TextRangeBoxes` di un testo, e `Margin` lascia spazio libero ai bordi della vista di scorrimento più esterna per una barra sovrapposta.

Una vista su cui la tastiera sposta il focus scorre in vista da sola.

## Layout

`Horizontal` scorre in orizzontale. `Behavior.ContentInset` aggiunge attorno al contenuto uno spazio che scorre con esso. Per una pagina che riempie l'area sicura, usa `PaddingEnv` sul contenuto, non sulla vista di scorrimento.

## Accessibilità

`Label` dà un nome all'area di scorrimento e `Role` dice di cosa si tratta, ad esempio una list box. I gesti di scorrimento della piattaforma, lo scorrimento da tastiera e quello dello screen reader vengono mantenuti; `Busy` segnala una vista che è ancora in caricamento.

## Piattaforma

Ogni backend usa la vista di scorrimento della piattaforma, quindi lanci, rimbalzi, barre di scorrimento e scorrimento al tocco sono nativi. Gli effetti ai bordi differiscono: macOS e iOS hanno l'effetto elastico, Android si allunga, Windows e Linux si fermano e il web segue il browser. `EdgeEffects` ed `EdgeBar` aggiungono barre sopra il contenuto dove l'app le desidera. Su telefoni e tablet iOS e Android, le viste di scorrimento verticali e orizzontali con contenuto in eccesso mostrano lungo l'asse attivo una maniglia di scorrimento trascinabile al tocco che percorre l'intera estensione del contenuto; le viste di scorrimento brevi che entrano senza eccedere, le viste con zoom, `Behavior.NoScroll`, `HideIndicator` e le viste che usano una `Bar` personalizzata o `base.SetScrollbarTheme` la nascondono. Nei test, `nsgotest.App.ScrollHandle` ne segnala i limiti nell'area visibile e `DragAt` o `Touch` la trascinano direttamente. Su Windows, l'indicatore sovrapposto compare per rotellina, tocco, trascinamento della barra e il relativo moto inerziale, non per `ScrollTo`, `ScrollBy`, reveal, inset o animazioni gestiti dall'app.

## Correlati

- [Elenchi e scorrimento](../lists.md): la guida completa allo scorrimento e agli elenchi.
- [List](list.md): righe che vengono riciclate.
- [Layout](../layout.md): padding e area sicura.
- Le pagine Scroll e Lists della galleria.

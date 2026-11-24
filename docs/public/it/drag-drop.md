---
title: Drag and drop
description: Trasferire valori applicativi tra viste all'interno di una finestra.
group: Concetti chiave
order: 57
---

`base/dragdrop` collega una sorgente e una destinazione nella stessa `Area`, all'interno di una finestra. L'app mantiene le proprie viste, stili, selezione e l'azione che gestisce il valore. Per impostazione predefinita una sorgente si avvia dopo quattro punti di movimento, lasciando intatto un clic o tocco ordinario.

```go
import "github.com/nsgo-dev/nsgo/base/dragdrop"

dragdrop.Area{Children: []core.Node{
	dragdrop.Source{
		View: core.View{
			Label: "Film", TestID: "poster",
			Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Poster"}},
		},
		Data: func() any { return filmID },
		Preview: func(any) core.Node { return core.Text{Content: "Film"} },
	},
	dragdrop.Target{
		View: core.View{
			TestID: "playlist",
			Style: core.Style{Width: core.Pt(160), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Playlist"}},
		},
		Accept: func(value any) bool { _, ok := value.(string); return ok },
		OnDrop: func(e dragdrop.Event) { addToPlaylist(e.Data.(string)) },
	},
}}
```

`Data` viene eseguita una volta all'inizio del trascinamento. Restituisci una slice per trasportare insieme più elementi selezionati; in quella funzione copia eventuali dati mutabili se devono restare un'istantanea. Una `Data` nil disabilita il trascinamento. `Threshold` imposta il movimento necessario. `Activation: dragdrop.AfterLongPress` attende prima `View.DelayLongPress` (500 ms per impostazione predefinita), poi il movimento; sceglila esplicitamente quando uno scorrimento rapido deve far scorrere la vista per prima cosa.

`View` conserva le sue proprietà ordinarie, inclusi i gestori di pressione e l'accessibilità. Una sorgente gestisce `View.Responder`; lascialo non impostato. I suoi figli possono includere un pulsante con la propria azione normale. `Preview` segue il puntatore senza ricevere input né rimontare la sorgente; in sua assenza viene mostrata l'etichetta della sorgente. Assegna sempre un'etichetta o un'anteprima a una sorgente. Aree annidate operano in modo indipendente.

Le destinazioni richiedono `OnDrop` per accettare un valore. La destinazione più interna ha la prima possibilità; quando `Accept` la rifiuta, un antenato che accetta può prenderla in carico. `OnEnter`, `OnMove` e `OnLeave` possono aggiornare l'aspetto della destinazione. Ogni evento trasporta `Data`, `Position` nella destinazione e `PagePosition` nella superficie. Disabilitare o rimuovere la destinazione attiva ne cancella lo stato di ingresso; al rilascio l'accettazione viene verificata nuovamente. Una destinazione dietro un'altra vista, ritagliata, disabilitata o sotto una finestra modale non può ricevere il rilascio.

`OnStart` e `OnEnd` descrivono la sessione della sorgente. `OnEnd` riceve `Result{Data, Dropped, Target}`. Escape, annullamento nativo del tocco, disabilitazione o rimozione della sorgente o rilascio al di fuori di una destinazione accettante terminano l'operazione senza drop. Il posizionamento della destinazione utilizza l'ultima geometria di cui è stato effettuato il commit; le animazioni native tra un commit e l'altro non vengono campionate. Questo meccanismo trasferisce valori all'interno della finestra; i file di sistema continuano a usare `OnFileDrop`.

Mantieni un menu, un comando da tastiera o un'azione di accessibilità che esegua la stessa operazione. Il trascinamento è una scorciatoia verso tale azione. I test usano `app.DragTo("id:poster", "id:playlist")`, oppure `DragTo` su una superficie selezionata; i test TSX usano `await app.dragTo("id:poster", "id:playlist")`. Entrambe le viste devono già rientrare nella finestra visibile; il passaggio invia una sequenza touch attraverso il responder senza sintetizzare un drop né scorrere durante il trascinamento. Vedi [Test](testing.md).

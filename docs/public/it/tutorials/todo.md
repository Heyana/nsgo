---
title: "Esempio: un'app di attività"
description: Costruisci passo dopo passo una piccola app di attività, con signal, una lista e un test.
group: Per iniziare
order: 11
---

Questo tutorial costruisce una piccola app di attività: aggiungere elementi, spuntarli ed eliminarli. Richiede circa dieci minuti e presuppone la [configurazione](../getting-started.md).

## Creare l'app

```sh
mkdir todo && cd todo
go mod init example.com/todo
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Costruire l'app di attività

L'app generata è un contatore. Sostituisci l'intero contenuto di `app.go` con questa app:

```go program height=480
package main

import (
	"fmt"
	"strings"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type todo struct {
	id   int
	text string
	done bool
}

func main() { nsgo.Run(Root) }

func Root(cx *core.Scope) core.Node {
	items := core.NewSignalFunc(cx, []todo{}, nil)
	draft := core.NewSignal(cx, "")
	nextID := core.NewSignal(cx, 1)
	colors := ui.ColorsOf(cx)

	add := func() {
		text := strings.TrimSpace(draft.Get())
		if text == "" {
			return
		}
		id := nextID.Get()
		nextID.Set(id + 1)
		items.Update(func(list []todo) []todo {
			return append(list, todo{id: id, text: text})
		})
		draft.Set("")
	}
	setDone := func(id int, done bool) {
		items.Update(func(list []todo) []todo {
			next := append([]todo(nil), list...)
			for i := range next {
				if next[i].id == id {
					next[i].done = done
				}
			}
			return next
		})
	}
	remove := func(id int) {
		items.Update(func(list []todo) []todo {
			out := list[:0:0]
			for _, it := range list {
				if it.id != id {
					out = append(out, it)
				}
			}
			return out
		})
	}
	left := func() int {
		n := 0
		for _, it := range items.Get() {
			if !it.done {
				n++
			}
		}
		return n
	}

	return core.View{
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.All(24), PaddingEnv: core.EnvAll(core.EnvSafeArea), Gap: 16},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Background()}
		},
		Children: []core.Node{
			ui.Text("Todos", 28, ui.Bold, colors.Text),
			core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
				ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "What needs doing?",
					OnSubmit: func(string) { add() }, TestID: "draft", Style: core.Style{FlexGrow: 1}},
				ui.Button{Text: "Add", Icon: lucide.Plus, OnPress: add, TestID: "add"},
			}},
			ui.Textf(func() string { return fmt.Sprintf("%d left", left()) }, 13, ui.Regular, colors.Muted),
			core.List{
				Count:         func() int { return len(items.Get()) },
				Key:           func(i int) any { return items.Get()[i].id },
				EstimatedSize: 44,
				Style:         core.Style{FlexGrow: 1},
				Empty:         ui.Text("Nothing yet. Add one above.", 13, ui.Regular, colors.Muted),
				Row: func(cx *core.Scope, row core.ListRow) core.Node {
					index := row.Index()
					it := items.Get()[index]
					return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(8, 0)},
						Children: []core.Node{
							ui.Checkbox{
								Checked: func() bool {
									for _, x := range items.Get() {
										if x.id == it.id {
											return x.done
										}
									}
									return false
								},
								OnChange: func(done bool) { setDone(it.id, done) },
								Text:     it.text,
								TestID:   fmt.Sprintf("todo-%d", it.id),
								Style:    core.Style{FlexGrow: 1, FlexShrink: 1},
							},
							ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost,
								TestID: fmt.Sprintf("delete-%d", it.id), OnPress: func() { remove(it.id) }},
						}}
				},
			},
		},
	}
}
```

Salva il file e l'app si riavvia. Scrivi un'attività, premi **Add** e spuntala.

## Come funziona

**Lo stato è fatto di signal.** `items`, `draft` e `nextID` sono signal. Un signal contiene un valore e tiene traccia di chi lo legge: quando `items` cambia, si aggiornano solo il testo e le righe che lo leggono. `core.NewSignal` confronta i valori con `==`, quindi per una slice si usa `core.NewSignalFunc` senza funzione di uguaglianza.

**Gli aggiornamenti sostituiscono la slice.** `items.Update` restituisce una nuova slice, quindi sono l'identità della lista e le chiavi delle righe a decidere cosa resta sullo schermo. `nextID` assegna un ID a ogni attività, e il `Key` di `core.List` lo usa; senza una chiave stabile, le righe riutilizzerebbero le proprie celle per altri elementi.

**Lo schermo è un albero di riquadri.** `Root` restituisce una `core.View` con padding, una spaziatura e i margini dell'area sicura. `FlexGrow: 1` e `FlexShrink: 1` le fanno riempire la finestra e niente di più, così una lista lunga scorre al suo interno invece di allungarla oltre il bordo della finestra. La riga dell'input imposta `Direction: core.Row`; tutto il resto è la colonna predefinita.

**L'input è controllato.** `draft.Get` è il valore del campo e `draft.Set` riceve ogni modifica. `OnSubmit` aggiunge un'attività quando si preme Invio, e `add` svuota la bozza, così il campo si svuota da solo.

**La lista costruisce solo ciò che è visibile.** `core.List` chiede `Count` righe e costruisce una riga solo quando si avvicina allo schermo. `Empty` viene mostrato finché non ci sono elementi, e la casella di spunta e il pulsante di eliminazione di ogni riga portano l'ID della propria attività, così una pressione trova ancora l'elemento giusto dopo che la lista è cambiata.

**L'aspetto è quello di `ui`.** `ui.Text`, `ui.Input`, `ui.Button`, `ui.Checkbox` e `ui.IconButton` prendono i propri colori dal tema e seguono automaticamente la modalità chiara e scura. `ui.ColorsOf(cx)` fornisce i colori per le parti che l'app disegna da sé, e uno `StyleFunc` segue sul posto un cambio di tema.

## Testarla

Il test generato preme il pulsante del contatore, quindi sostituisci `app_test.go` con un test dell'app di attività:

```go
package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestAddCompleteAndDelete(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Find("text:Nothing yet. Add one above.")
	app.Type("id:draft", "Buy milk")
	app.Press("id:add")
	app.WaitFor("text:Buy milk", time.Second)
	app.Press("id:todo-1")
	app.WaitFor("text:0 left", time.Second)
	app.Press("id:delete-1")
	app.WaitFor("text:Nothing yet. Add one above.", time.Second)
}
```

Il test avvia l'app in modalità headless, trova gli elementi in base al testo o al test ID e agisce come farebbe un utente. Eseguilo con:

```sh
go tool nsgo test
```

`TestMain` cede il thread principale alla piattaforma quando i test girano su un target, e `go tool nsgo test -target ios_simulator` esegue lo stesso test nel simulatore con il suo input nativo.

## Distribuirla

```sh
go tool nsgo build
```

`build` scrive un pacchetto di release per ogni target elencato in `nsgo.yaml`, in `dist/<name>-<version>-<target>`: un `.app`, un `.ipa`, un `.apk`, le directory di Windows e Linux oppure un sito web. Tutto ciò che va distribuito finisce anche in `dist/releases`: l'immagine disco, il programma di installazione, i pacchetti Debian e Arch, gli IPA, gli APK, il sito, i mini program e gli script che installano l'app dalle tue release GitHub. [Compilazione e distribuzione](../building.md) tratta installer, firma e notarizzazione, pubblicazione, icone, widget, App Clip e l'App Store.

## Prossimi passi

- [Esempio: un'app di note](notes.md) ed [Esempio: repository GitHub popolari](repos.md): altre due app, passo dopo passo.
- [Componenti e stato](../components.md): elementi, componenti, signal e regioni più in dettaglio.
- [Layout](../layout.md) e [Stili e temi](../styles.md): la struttura e l'aspetto dello schermo.
- [Liste e scorrimento](../lists.md): liste lunghe, intestazioni fisse e caricamento di altri elementi.
- [Test](../testing.md): trovare i controlli, premerli e catturare screenshot.
- [Integrazione](../embedding.md): inserire l'app all'interno di un'altra app.

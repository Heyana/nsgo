---
title: "Esempio: un'app di note"
description: Costruisci un'app di note con uno stack di navigazione, un editor multiriga e note conservate sul dispositivo.
group: Per iniziare
order: 12
---

Questo tutorial costruisce un'app di note: un elenco di note, una pagina che ne modifica una e note che sono ancora lì quando l'app si riavvia. Presuppone la [configurazione](../getting-started.md).

## Creare l'app

```sh
mkdir notes && cd notes
go mod init example.com/notes
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Scrivere l'app

Sostituisci l'intero contenuto di `app.go`:

```go program height=480
package main

import (
	"encoding/json"
	"strings"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type note struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

// title is a note's first line.
func (n note) title() string {
	line, _, _ := strings.Cut(strings.TrimSpace(n.Text), "\n")
	if line == "" {
		return "New note"
	}
	return line
}

func main() { nsgo.Run(Root) }

func Root(cx *core.Scope) core.Node {
	store := cx.Storage()
	var saved []note
	if data, ok := store.GetItem("notes"); ok {
		json.Unmarshal([]byte(data), &saved)
	}
	notes := core.NewSignalFunc(cx, saved, nil)
	save := func(list []note) {
		notes.Set(list)
		data, _ := json.Marshal(list)
		store.SetItem("notes", string(data))
	}

	// A route is the ID of the note it edits, and 0 the list.
	nav := base.NewNav(cx, 0)

	find := func(id int) note {
		for _, n := range notes.Get() {
			if n.ID == id {
				return n
			}
		}
		return note{ID: id}
	}
	add := func() {
		id := 1
		for _, n := range notes.Get() {
			id = max(id, n.ID+1)
		}
		save(append([]note{{ID: id}}, notes.Get()...))
		nav.Push(id)
	}
	edit := func(id int, text string) {
		list := append([]note(nil), notes.Get()...)
		for i := range list {
			if list[i].ID == id {
				list[i].Text = text
			}
		}
		save(list)
	}
	remove := func(id int) {
		var list []note
		for _, n := range notes.Get() {
			if n.ID != id {
				list = append(list, n)
			}
		}
		save(list)
		nav.Pop(1)
	}

	return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, id int, _ func() int) base.NavPage {
		if id == 0 {
			return listPage(cx, notes.Get, nav.Push, add)
		}
		return ui.NavPage(cx, base.NavPage{
			TitleFunc: func() string { return find(id).title() },
			Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.All(16)}, Children: []core.Node{
				ui.Input{
					Multiline:   true,
					Value:       func() string { return find(id).Text },
					OnChange:    func(text string) { edit(id, text) },
					Placeholder: "Write something",
					TestID:      "editor",
					Style:       core.Style{FlexGrow: 1, FlexShrink: 1},
				},
			}}},
		}, ui.NavHeader{
			OnBack:    func() { nav.Pop(1) },
			BackLabel: "Notes",
			Trailing: []core.Node{ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost,
				TestID: "delete", OnPress: func() { remove(id) }}},
		})
	})
}

func listPage(cx *core.Scope, notes func() []note, open func(int), add func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	return ui.NavPage(cx, base.NavPage{
		Title: "Notes",
		Children: []core.Node{core.List{
			Count:         func() int { return len(notes()) },
			Key:           func(i int) any { return notes()[i].ID },
			EstimatedSize: 48,
			Style:         core.Style{FlexGrow: 1},
			Empty: core.View{Style: core.Style{Padding: core.All(16)}, Children: []core.Node{
				ui.Text("No notes yet.", 13, ui.Regular, colors.Muted),
			}},
			Row: func(cx *core.Scope, row core.ListRow) core.Node {
				n := notes()[row.Index()]
				return core.View{
					Role:    core.RoleButton,
					OnPress: func(core.PressEvent) { open(n.ID) },
					Style:   core.Style{Padding: core.Symmetric(12, 16), Borders: core.Edges{Bottom: 1}},
					StyleFunc: func() core.Style {
						return core.Style{BorderColor: colors.Border()}
					},
					Children: []core.Node{ui.Text(n.title(), 15, ui.Regular, colors.Text)},
				}
			},
		}},
	}, ui.NavHeader{
		Trailing: []core.Node{ui.IconButton{Icon: lucide.Plus, Label: "New note", Variant: ui.IconButtonGhost,
			TestID: "new", OnPress: add}},
	})
}
```

Premi **+** per aggiungere una nota e scrivi; la sua prima riga diventa il titolo nell'elenco. Chiudi l'app e riavviala: le note sono ancora lì.

## Come funziona

**Uno stack di pagine.** `base.NewNav(cx, 0)` mantiene uno stack di route, qui gli ID delle note con `0` per l'elenco. `nav.Stack` costruisce una pagina per ogni route e mantiene montate le pagine sottostanti, così l'elenco conserva la propria posizione di scorrimento mentre una nota è aperta. `Push` apre una nota e `Pop` torna indietro.

**Pagine con un'intestazione.** `ui.NavPage` dà a una pagina lo sfondo di `ui` e un `ui.NavHeader` con il suo titolo, il pulsante Indietro e i pulsanti. Su iOS e Android è il sistema a disegnare l'intestazione, con il proprio gesto Indietro e la propria transizione; altrove la disegna `ui`.

**Archiviazione sul dispositivo.** `cx.Storage()` è l'archivio chiave-valore dell'app. `Root` legge le note una volta all'avvio e `save` le scrive in JSON ogni volta che cambiano. L'archivio contiene stringhe; i file che l'app scrive vanno in `cx.DataDir()`.

**Un'unica fonte di verità.** Ogni pagina legge lo stesso signal `notes`. Il `Value` dell'editor legge la nota, `OnChange` la salva, e il `TitleFunc` dell'intestazione e la riga dell'elenco seguono lo stesso signal, così si aggiornano mentre scrivi.

## Testarla

Sostituisci `app_test.go`. `nsgotest.Storage` avvia l'app con l'archivio già scritto, come se fosse già stata eseguita in precedenza:

```go
package main

import (
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestWriteANote(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Find("text:No notes yet.")
	app.Press("id:new")
	app.Type("id:editor", "Groceries\nMilk")
	app.WaitFor("role:header Groceries", time.Second)
	if data, _ := app.Storage().GetItem("notes"); !strings.Contains(data, "Milk") {
		t.Fatalf("saved %q", data)
	}
}

func TestSavedNotes(t *testing.T) {
	app := nsgotest.Start(t, Root, nsgotest.Storage(map[string]string{
		"notes": `[{"id":1,"text":"Ideas\nA notes app"}]`,
	}))
	app.Press("text:Ideas")
	app.Press("id:delete")
	app.WaitFor("text:No notes yet.", time.Second)
}
```

```sh
go tool nsgo test
```

## Prossimi passi

- [Esempio: repository GitHub popolari](repos.md): caricare dati dalla rete senza bloccare lo schermo.
- [Navigazione](../navigation.md): finestre modali, menu e barre a schede.
- [Servizi di piattaforma](../services.md): file, condivisione, appunti e altro ancora.

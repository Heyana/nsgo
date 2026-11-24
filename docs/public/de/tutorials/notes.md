---
title: "Beispiel: Eine Notizen-App"
description: Bauen Sie eine Notizen-App mit einem Navigationsstapel, einem mehrzeiligen Editor und Notizen, die auf dem Gerät gespeichert bleiben.
group: Erste Schritte
order: 12
---

Dieses Tutorial baut eine Notizen-App: eine Liste von Notizen, eine Seite zum Bearbeiten einer Notiz und Notizen, die beim nächsten Start der App noch da sind. Es setzt die [Einrichtung](../getting-started.md) voraus.

## Die App erstellen

```sh
mkdir notes && cd notes
go mod init example.com/notes
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Die App schreiben

Ersetzen Sie den gesamten Inhalt von `app.go`:

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

Drücken Sie **+**, um eine Notiz hinzuzufügen, und tippen Sie los; ihre erste Zeile wird zu ihrem Titel in der Liste. Beenden Sie die App und starten Sie sie erneut: Die Notizen sind noch da.

## So funktioniert es

**Ein Stapel von Seiten.** `base.NewNav(cx, 0)` verwaltet einen Stapel von Routen, hier Notiz-IDs, wobei `0` für die Liste steht. `nav.Stack` baut für jede Route eine Seite und lässt die darunterliegenden Seiten gemountet, sodass die Liste ihre Scrollposition behält, während eine Notiz geöffnet ist. `Push` öffnet eine Notiz und `Pop` geht zurück.

**Seiten mit Kopfzeile.** `ui.NavPage` gibt einer Seite den Hintergrund von `ui` und einen `ui.NavHeader` mit Titel, Zurück-Button und weiteren Buttons. Unter iOS und Android zeichnet das System die Kopfzeile, mit eigener Zurück-Geste und eigenem Übergang; anderswo zeichnet `ui` sie.

**Speicher auf dem Gerät.** `cx.Storage()` ist der Schlüssel-Wert-Speicher der App. `Root` liest die Notizen einmal beim Start, und `save` schreibt sie bei jeder Änderung als JSON. Der Speicher enthält Strings; Dateien, die die App schreibt, gehören in `cx.DataDir()`.

**Eine einzige Quelle der Wahrheit.** Jede Seite liest dasselbe Signal `notes`. Der `Value` des Editors liest die Notiz, `OnChange` speichert sie, und die `TitleFunc` der Kopfzeile sowie die Zeile der Liste folgen demselben Signal, sodass sie sich während des Tippens aktualisieren.

## Testen

Ersetzen Sie `app_test.go`. `nsgotest.Storage` startet die App mit bereits beschriebenem Speicher, als wäre sie schon einmal gelaufen:

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

## Wie es weitergeht

- [Beispiel: Beliebte GitHub-Repositories](repos.md): Daten aus dem Netzwerk laden, ohne den Bildschirm zu blockieren.
- [Navigation](../navigation.md): Modals, Menüs und Tab-Leisten.
- [Plattformdienste](../services.md): Dateien, Teilen, die Zwischenablage und mehr.

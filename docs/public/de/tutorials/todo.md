---
title: "Beispiel: Eine Todo-App"
description: Bauen Sie Schritt für Schritt eine kleine Todo-App mit Signalen, einer Liste und einem Test.
group: Erste Schritte
order: 11
---

Dieses Tutorial baut eine kleine Todo-App: Einträge hinzufügen, abhaken und löschen. Es dauert etwa zehn Minuten und setzt die [Einrichtung](../getting-started.md) voraus.

## Die App erstellen

```sh
mkdir todo && cd todo
go mod init example.com/todo
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Die Todo-App bauen

Die erzeugte App ist ein Zähler. Ersetzen Sie den gesamten Inhalt von `app.go` durch diese App:

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

Speichern Sie die Datei, und die App startet neu. Geben Sie eine Aufgabe ein, drücken Sie **Add** und haken Sie sie ab.

## So funktioniert es

**Zustand besteht aus Signalen.** `items`, `draft` und `nextID` sind Signale. Ein Signal enthält einen Wert und verfolgt, was ihn liest: Ändert sich `items`, werden nur die Texte und Zeilen aktualisiert, die es lesen. `core.NewSignal` vergleicht Werte mit `==`, daher verwendet ein Slice `core.NewSignalFunc` ohne Gleichheitsfunktion.

**Aktualisierungen ersetzen den Slice.** `items.Update` gibt einen neuen Slice zurück, daher entscheiden die Identität der Liste und die Schlüssel der Zeilen, was auf dem Bildschirm bleibt. `nextID` gibt jedem Todo eine ID, und `Key` von `core.List` verwendet sie; ohne stabilen Schlüssel würden Zeilen ihre Zellen für andere Einträge wiederverwenden.

**Der Bildschirm ist ein Baum aus Boxen.** `Root` gibt einen `core.View` mit Innenabstand, einem Abstand zwischen den Kindern und den Einzügen des sicheren Bereichs zurück. `FlexGrow: 1` und `FlexShrink: 1` lassen ihn das Fenster ausfüllen, aber nicht mehr, sodass eine lange Liste in ihm scrollt, statt ihn über den Fensterrand hinaus zu dehnen. Die Eingabezeile setzt `Direction: core.Row`; alles andere ist die Standardspalte.

**Die Eingabe ist kontrolliert.** `draft.Get` ist der Wert des Feldes, und `draft.Set` erhält jede Änderung. `OnSubmit` fügt eine Aufgabe hinzu, wenn die Eingabetaste gedrückt wird, und `add` leert den Entwurf, sodass sich das Feld selbst leert.

**Die Liste baut nur, was sichtbar ist.** `core.List` fragt nach `Count` Zeilen und baut eine Zeile erst, wenn sie sich dem Bildschirm nähert. `Empty` wird angezeigt, solange es keine Einträge gibt, und Checkbox und Lösch-Button jeder Zeile tragen die ID ihres Todos, sodass ein Druck auch nach einer Änderung der Liste den richtigen Eintrag findet.

**Das Aussehen stammt von `ui`.** `ui.Text`, `ui.Input`, `ui.Button`, `ui.Checkbox` und `ui.IconButton` beziehen ihre Farben aus dem Theme und folgen automatisch dem hellen und dunklen Modus. `ui.ColorsOf(cx)` liefert die Farben für die Teile, die die App selbst zeichnet, und eine `StyleFunc` folgt einem Theme-Wechsel an Ort und Stelle.

## Testen

Der erzeugte Test drückt den Button des Zählers, ersetzen Sie `app_test.go` daher durch einen Test der Todo-App:

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

Der Test startet die App ohne Oberfläche, findet Elemente über ihren Text oder ihre Test-ID und handelt so, wie es eine Nutzerin oder ein Nutzer tun würde. Führen Sie ihn so aus:

```sh
go tool nsgo test
```

`TestMain` überlässt der Plattform den Haupt-Thread, wenn die Tests auf einer Zielplattform laufen, und `go tool nsgo test -target ios_simulator` führt denselben Test im Simulator mit dessen nativer Eingabe aus.

## Ausliefern

```sh
go tool nsgo build
```

`build` schreibt für jede in `nsgo.yaml` aufgeführte Zielplattform ein Release-Paket nach `dist/<name>-<version>-<target>`: eine `.app`, eine `.ipa`, eine `.apk`, die Verzeichnisse für Windows und Linux oder eine Website. Alles, was verteilt werden soll, landet zusätzlich in `dist/releases`: das Disk-Image, das Setup-Programm, Debian- und Arch-Pakete, IPAs, APKs, die Website, Mini-Programme und Skripte, die die App aus Ihren GitHub-Releases installieren. [Bauen und Ausliefern](../building.md) behandelt Installer, Signierung und Notarisierung, Veröffentlichung, Icons, Widgets, App Clips und den App Store.

## Wie es weitergeht

- [Beispiel: Eine Notizen-App](notes.md) und [Beispiel: Beliebte GitHub-Repositories](repos.md): zwei weitere Apps, Schritt für Schritt.
- [Komponenten und Zustand](../components.md): Elemente, Komponenten, Signale und Regionen im Detail.
- [Layout](../layout.md) und [Styles und Themes](../styles.md): Struktur und Aussehen des Bildschirms.
- [Listen und Scrollen](../lists.md): lange Listen, haftende Kopfzeilen und Nachladen.
- [Testen](../testing.md): Steuerelemente finden, drücken und Screenshots aufnehmen.
- [Einbetten](../embedding.md): die App in eine andere App einbetten.

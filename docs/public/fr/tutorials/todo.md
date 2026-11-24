---
title: "Exemple : une application de tâches"
description: Construisez étape par étape une petite application de tâches, avec des signaux, une liste et un test.
group: Démarrer
order: 11
---

Ce tutoriel construit une petite application de tâches : ajouter des éléments, les cocher et les supprimer. Il dure une dizaine de minutes et suppose que l’[installation](../getting-started.md) est effectuée.

## Créer l’application

```sh
mkdir todo && cd todo
go mod init example.com/todo
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Construire l’application de tâches

L’application générée est un compteur. Remplacez tout le contenu de `app.go` par cette application :

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

Enregistrez le fichier et l’application redémarre. Saisissez une tâche, appuyez sur **Add** et cochez-la.

## Fonctionnement

**L’état repose sur des signaux.** `items`, `draft` et `nextID` sont des signaux. Un signal contient une valeur et suit ce qui la lit : lorsque `items` change, seuls le texte et les lignes qui le lisent sont mis à jour. `core.NewSignal` compare les valeurs avec `==`, donc une tranche utilise `core.NewSignalFunc` sans fonction d’égalité.

**Les mises à jour remplacent la tranche.** `items.Update` retourne une nouvelle tranche, de sorte que l’identité de la liste et les clés de ligne décident de ce qui reste à l’écran. `nextID` attribue un identifiant à chaque tâche, et `Key` de `core.List` l’utilise ; sans clé stable, les lignes réutiliseraient leurs cellules pour d’autres éléments.

**L’écran est un arbre de boîtes.** `Root` retourne une `core.View` avec du remplissage interne, un espacement et les marges de la zone de sécurité. `FlexGrow: 1` et `FlexShrink: 1` lui font remplir la fenêtre sans la dépasser, afin qu’une longue liste défile à l’intérieur au lieu de l’étirer au-delà du bord de la fenêtre. La ligne de saisie définit `Direction: core.Row` ; tout le reste utilise la disposition en colonne par défaut.

**Le champ de saisie est contrôlé.** `draft.Get` est la valeur du champ et `draft.Set` reçoit chaque modification. `OnSubmit` ajoute une tâche lorsque la touche Entrée est pressée, et `add` efface le brouillon, de sorte que le champ se vide de lui-même.

**La liste ne construit que ce qui est affiché.** `core.List` demande `Count` lignes et ne construit une ligne que lorsqu’elle approche de l’écran. `Empty` s’affiche tant qu’il n’y a aucun élément, et la case à cocher ainsi que le bouton de suppression de chaque ligne portent l’identifiant de leur tâche, de sorte qu’un appui retrouve toujours le bon élément après une modification de la liste.

**L’apparence est celle d’`ui`.** `ui.Text`, `ui.Input`, `ui.Button`, `ui.Checkbox` et `ui.IconButton` tirent leurs couleurs du thème et suivent automatiquement les modes clair et sombre. `ui.ColorsOf(cx)` fournit les couleurs pour les éléments que l’application dessine elle-même, et une `StyleFunc` suit sur place un changement de thème.

## La tester

Le test généré appuie sur le bouton du compteur ; remplacez donc `app_test.go` par un test de l’application de tâches :

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

Le test démarre l’application sans interface graphique, recherche les éléments par leur texte ou leur identifiant de test et agit comme le ferait un utilisateur. Exécutez-le avec :

```sh
go tool nsgo test
```

`TestMain` confie le thread principal à la plateforme lorsque les tests s’exécutent sur une cible, et `go tool nsgo test -target ios_simulator` exécute le même test dans le simulateur avec sa saisie native.

## La livrer

```sh
go tool nsgo build
```

`build` écrit un paquet de distribution pour chaque cible répertoriée dans `nsgo.yaml`, dans `dist/<name>-<version>-<target>` : un `.app`, un `.ipa`, un `.apk`, les répertoires Windows et Linux ou un site web. Tout ce qui est à distribuer atterrit également dans `dist/releases` : l’image disque, le programme d’installation, les paquets Debian et Arch, les IPA, les APK, le site, les mini-programmes et les scripts qui installent l’application depuis vos versions GitHub. [Compilation et distribution](../building.md) couvre les installateurs, la signature et la notarisation, la publication, les icônes, les widgets, les App Clips et l’App Store.

## Suite

- [Exemple : une application de notes](notes.md) et [Exemple : dépôts GitHub populaires](repos.md) : deux autres applications, étape par étape.
- [Composants et état](../components.md) : éléments, composants, signaux et régions plus en détail.
- [Mise en page](../layout.md) et [Styles et thèmes](../styles.md) : la structure et l’apparence de l’écran.
- [Listes et défilement](../lists.md) : longues listes, en-têtes figés et chargement supplémentaire.
- [Tests](../testing.md) : rechercher des contrôles, appuyer dessus et prendre des captures d’écran.
- [Intégration](../embedding.md) : placer l’application dans une autre application.

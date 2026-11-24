---
title: Mots, tests et publication
description: Traduire ce qu'un composant énonce, le tester comme un utilisateur et le proposer aux applications Go et TSX.
group: Écrire des composants
order: 74
---

## Mots

Un composant énonce ses propres textes, comme le nom d'un bouton pour les lecteurs d'écran, via les catalogues de son paquet plutôt que par des chaînes littérales. Placez un paquet `locales/` dans la bibliothèque avec le catalogue de la langue source et un fichier qui génère le code :

```yaml
# locales/en.yaml
clear:
  message: Clear
  description: What screen readers say of the button that empties a chip input.
```

```go
// Package locales says the chips' words in the language the app shows.
package locales

//go:generate go tool nsgo i18n
```

`go generate ./locales` écrit une fonction par message, que le composant appelle avec sa portée : `locales.Clear(cx)` l'énonce dans la langue affichée par l'application. Validez le fichier généré `catalog_gen.go` dans votre dépôt, afin que les applications compilent avec Go seul. Ajoutez un fichier par langue traduite (`zh-Hans.yaml`) ; une application fournit d'autres langues dans ses propres catalogues, sous une clé nommant le chemin d'import de votre paquet, comme pour les [textes propres à NSGo](../localization.md#nsgos-own-words).

## Tests

`nsgotest` exécute un composant comme l'utiliserait un utilisateur, en mode sans interface graphique (*headless*). Montez-le dans une petite racine et agissez dessus par rôle et par texte :

```go
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestToggle(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, false)
		return chips.Toggle{On: on.Get, OnChange: on.Set, Label: "Bold"}
	})
	app.Press("role:switch Bold")
	if !app.Find("role:switch Bold").Props.Checked {
		t.Fatal("the toggle did not turn on")
	}
}
```

Depuis la racine de la bibliothèque, `go tool nsgo test` teste chaque paquet, et `-target macos` ou une autre cible exécute les mêmes tests dans une fenêtre native. Testez les deux apparences ainsi qu'une pseudo-locale (`-lang en-XA`) pour repérer les couleurs et les textes codés en dur. [Tests](../testing.md) couvre le reste.

## Publication

Une bibliothèque de composants est un module Go : les applications l'installent avec `go get` et importent ses paquets. Conservez ses formes sans style et stylisée dans des paquets séparés lorsque les deux existent, afin qu'une application dotée de sa propre apparence n'importe aucun style dont elle n'a pas besoin.

La page [Composants](https://nsgo.dev/components/) du site répertorie les bibliothèques réutilisables ; ajoutez un document et une capture d'écran dans `docs/public/components/` pour proposer la vôtre.

Les applications TSX accèdent aux composants Go une fois ceux-ci enregistrés dans une fonction `init` :

```go
func init() {
	tsx.Register("example.com/chips", map[string]core.Node{"Toggle": Toggle{}})
}
```

`import { Toggle } from "example.com/chips"` accepte ensuite les champs comme props ([Applications TSX](../tsx.md#go-from-tsx)).

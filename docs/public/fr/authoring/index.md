---
title: Écrire des composants
description: Créer des composants que d'autres personnes intègrent à leurs applications, à l'image de base et ui dans NSGo.
group: Écrire des composants
order: 70
---

Ces pages s'adressent aux auteurs de composants : une bibliothèque de contrôles, un système de design ou une vue complexe que les applications importent en tant que paquet Go. Elles supposent la connaissance de [Composants et état](../components.md) et [Styles et thèmes](../styles.md).

## Un composant

Un composant est un type doté d'une méthode `Render`, comme dans une application. Ses champs correspondent à ce que l'application configure ; tout ce que l'application ne définit pas prend la valeur zéro, si bien que la valeur zéro de chaque champ doit représenter le cas habituel.

```go
package chips

import "github.com/nsgo-dev/nsgo/core"

// Chip is a short label in a pill.
type Chip struct {
	Text string
}

// Render implements core.Node.
func (c Chip) Render(*core.Scope) core.Node {
	return core.View{Class: "self-start rounded-full px-3 py-1", Children: []core.Node{
		core.Text{Content: c.Text, Class: "text-sm"},
	}}
}
```

Une application l'utilise comme n'importe quel élément : `chips.Chip{Text: "New"}`.

## Comportement et apparence

NSGo conserve chaque contrôle en deux versions. `base` contient son comportement, son état et son accessibilité sans aucune apparence, et reçoit chaque style de l'application partie par partie ; `ui` applique une apparence par-dessus la forme `base`, afin que le comportement vive en un seul endroit. Une bibliothèque peut suivre la même séparation :

- Un composant **sans style** (*unstyled*) gère les appuis, le clavier, le focus et ce qu'entendent les lecteurs d'écran, et expose le style de chaque partie, comme le fait `base.Switch`. Les applications et autres bibliothèques lui donnent leur propre apparence ([Parties et états](parts.md)).
- Un composant **stylisé** encapsule un composant sans style et se contente de choisir des styles, à partir du thème actif, comme le fait `ui.Switch` ([Apparences et thèmes](looks.md)).

Un composant sans comportement à partager, comme un badge, peut être uniquement stylisé.

## Un paquet

- Gardez un composant par fichier, nommé d'après lui (`chip.go`), avec les types qu'il est seul à utiliser ; ce que plusieurs partagent reçoit un fichier dédié.
- Donnez à un composant complexe un paquet dédié, dont l'API omet le nom du composant, comme `dock.Area` dans `base/dock`.
- Placez les textes énoncés par un composant dans le dossier `locales/` du paquet ([Mots, tests et publication](shipping.md)).
- Ne faites rien implicitement selon l'appareil : une taille ou une forme qui change sur un téléphone ou un ordinateur doit être un champ défini par l'application.

## Suite

- [Parties et états](parts.md) : valeurs contrôlées, parties stylisées et accessibilité.
- [Apparences et thèmes](looks.md) : couleurs issues du thème, variantes et tailles.
- [Composants sans style](base.md) : les contrôles propres à NSGo sans apparence, à styliser ou sur lesquels bâtir.
- [Mots, tests et publication](shipping.md) : traductions, tests et TSX.

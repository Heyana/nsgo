---
title: Sélecteur
description: Une valeur issue d’une liste, sous forme de fenêtre contextuelle ou de feuille.
group: Composants
order: 36
---

`ui.Select[T]` choisit une valeur parmi une liste de valeurs de tout type comparable. La liste s’ouvre sous le déclencheur sur chaque appareil ; `Presentation: base.PresentSheet` l’ouvre plutôt sous forme de feuille depuis le bas, comme le souhaiterait une application mobile.

```go preview=select/basic height=280
size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options: []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:   size.Get,
	OnChange: size.Set,
	Label:   "Size",
}
```

## Options et recherche

`Options` contient des `ui.Option[T]`, chacun doté d’une `Value`, d’un `Label`, et éventuellement d’un `Group`, d’une `Icon`, d’une fonction `Disabled` ou d’un `Content` propre à l’application. Des options adjacentes partageant un même `Group` non vide s’affichent sous un en-tête de groupe, dans un `Select` ou un `Combobox`, que les touches fléchées et la saisie ignorent et que les lecteurs d’écran lisent comme un titre. Utilisez `OptionsFunc` lorsque les options suivent un signal, afin que la liste ouverte se mette à jour sur place tout en conservant la valeur et le focus du champ. `Placeholder` s’affiche lorsqu’aucune option n’est choisie. `Searchable` filtre la liste au fil de la saisie, `OnClear` affiche un bouton d’effacement et `Disabled` désactive le sélecteur.

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

`Style` met en page le sélecteur, sur une largeur de 200 points sauf si une largeur est définie ; sa hauteur, son remplissage intérieur (*padding*) et sa typographie s’appliquent au champ, comme pour un [champ texte](input.md), afin qu’un sélecteur s’aligne avec un bouton `SizeSmall` :

```go
ui.Select[string]{Options: sizes, Value: size.Get, OnChange: size.Set, Label: "Size",
	Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
```

## Comportement

Haut, Bas, Entrée et Espace ouvrent la liste depuis le déclencheur ayant le focus ; les flèches déplacent la surbrillance et la saisie anticipée (*typeahead*) saute directement d’après les titres des éléments. Choisir un élément ferme la liste et signale sa valeur. Échap, le bouton retour et un appui à l’extérieur la ferment sans modification. `Empty` est le texte affiché lorsque la recherche ne donne aucun résultat.

## Accessibilité

`Label` nomme le champ pour les technologies d’assistance. Utilisez `LabelFunc` pour mettre à jour ce nom sur place lorsque ses signaux changent. Un libellé vide utilise `Placeholder`. Le déclencheur et la liste portent les rôles du sélecteur, et l’élément sélectionné est signalé. Le clavier atteint le déclencheur avec Tab et l’ouvre avec Entrée ou Espace.

## Plateformes

Sur macOS, Windows et Linux, la liste est une fenêtre contextuelle à côté du déclencheur. Sur iOS et Android, il s’agit d’une feuille, et la ligne sélectionnée reste affichée dans le champ. Les mêmes options et valeurs fonctionnent sur chaque cible.

## Voir aussi

- [Formulaires](../forms.md) : des sélecteurs dans un formulaire.
- [Menu](menu.md) : des commandes plutôt qu’une valeur.
- [Onglets](tabs.md) : quelques options affichées simultanément.
- Les pages Select et Combobox de la galerie.
- [Composants sans style](../authoring/base.md#select) : `base.Select`, avec des groupes et votre propre déclencheur.

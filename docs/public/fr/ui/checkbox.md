---
title: Case à cocher
description: Une case à cocher, cochée, décochée ou indéterminée.
group: Composants
order: 34
---

`ui.Checkbox` est une case à cocher avec l’apparence de `ui`, accompagnée d’un libellé à côté. Elle ne conserve aucune valeur : l’application indique si elle est cochée et met cet état à jour dans `OnChange`.

```go preview=checkbox/basic
done := core.NewSignal(cx, false)
ui.Checkbox{Checked: done.Get, OnChange: done.Set, Text: "Remember me"}
```

## Contenu

`ui.Checkbox` affiche `Text` ou les `Children` de l’application à côté de sa case. `Label` remplace les deux pour les technologies d’assistance, et `Hint` indique à quoi elle sert.

## États

`Indeterminate` n’est ni coché ni décoché, par exemple pour un « tout sélectionner » sur une sélection partielle ; un appui demande alors l’état coché. `Disabled` empêche les appuis et le focus.

```go preview=checkbox/indeterminate height=220
ui.Checkbox{
	Checked:       func() bool { return chosen.Get() == len(files) },
	Indeterminate: func() bool { return chosen.Get() > 0 && chosen.Get() < len(files) },
	OnChange:      chooseAll,
	Text:          "Select all",
}
```

## Modificateurs d’appui

Utilisez `OnChangeEvent` plutôt que `OnChange` lorsqu’une liste nécessite une sélection de plage par appui avec Maj :

```go
ui.Checkbox{Checked: done.Get, Text: "Remember me", OnChangeEvent: func(next bool, press core.PressEvent) {
	if press.Modifiers.Shift {
		chooseRange(next)
		return
	}
	done.Set(next)
}}
```

Lorsqu’il est défini, ce callback remplace `OnChange` ; l’application continue de mettre à jour la valeur. L’activation au clavier sur ordinateur et sur le web préserve les modificateurs après les raccourcis de l’application ; sous Windows, Alt combiné à Espace ou Entrée ouvre plutôt le menu de la fenêtre. En TSX, `onChangeEvent` reçoit `{ checked, press }`.

## Accessibilité

La case à cocher signale `RoleCheckbox` ainsi que son état coché ou indéterminé. Son nom est `Text`, `Label` ou le texte de ses enfants. Espace permet de la basculer lorsqu’elle a le focus clavier, et un lecteur d’écran annonce le nouvel état.

## Plateforme

La case est dessinée par l’application, et non par un contrôle de la plateforme, de sorte qu’elle présente le même aspect partout. L’appui, l’anneau de focus et le lecteur d’écran sont ceux de la plateforme. Elle est testée par rôle et par libellé sur chaque cible.

## Voir aussi

- [Interrupteur](switch.md) : un réglage qui prend effet immédiatement.
- [Formulaires](../forms.md) : les choix dans un formulaire.
- [Liste](list.md) : des cases à cocher dans des lignes.
- La page Checkbox de la galerie.
- [Composants sans style](../authoring/base.md#checkbox) : `base.Checkbox`, pour une case avec votre propre apparence.

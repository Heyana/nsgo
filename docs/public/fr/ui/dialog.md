---
title: Boîte de dialogue
description: Une surface modale, une feuille et une alerte.
group: Composants
order: 41
---

`ui.Dialog` est une surface modale par-dessus une application assombrie, et une feuille surgissant du bas sur les écrans tactiles. `ui.Sheet` est un panneau issu du bas, et `ui.AlertDialog` demande confirmation à l’utilisateur.

```go preview=dialog/basic height=360
open := core.NewSignal(cx, false)
ui.Dialog{
	Open:         open.Get,
	OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
	Title:        "Rename the file",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { open.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

## Ouverture et fermeture

L’application conserve l’état d’ouverture de la boîte de dialogue. `OnOpenChange` reçoit chaque changement demandé par la boîte de dialogue, avec un `base.Reason` qui distingue un appui à l’extérieur de la touche Échap ou du bouton retour ; `OnClose` s’exécute une fois qu’elle est fermée. `NoCloseOnEscape` et `NoCloseOnBackdropPress` empêchent Échap et un appui à l’extérieur de la fermer.

`Title` et `Description` figurent en tête de la surface, `TitleFunc` et `DescriptionFunc` lorsqu’ils changent pendant qu’elle est ouverte, et `Footer` contient ses boutons. `Bare` laisse toute la surface à `Children`, pour une boîte de dialogue utilisant la mise en page propre à l’application, que les ordinateurs de bureau rognent à l’intérieur du bord de la surface selon les coins donnés par `Style` et `StyleFunc`, et `Inset` fixe sa distance minimale par rapport aux bords de la fenêtre sur ordinateur.

Sur les feuilles tactiles, `Bare` supprime désormais la poignée et la marge interne par défaut du contenu afin que `Children` remplissent le panneau. `NoGrabber` masque uniquement la poignée, `ContentStyle` et `ContentStyleFunc` personnalisent le conteneur du contenu, et `TitleStyle` et `TitleStyleFunc` personnalisent le titre. `Leading` et `Trailing` placent des nœuds, tels que des boutons Annuler et Terminé, aux extrémités de la ligne de titre, avec le titre centré entre eux. `StartingStyle` et `EndingStyle` personnalisent la transition de la feuille ; incluez-y un `Transition` lorsque vous avez besoin d’un minutage personnalisé.

`Style` fusionne par-dessus l’apparence de la surface. `StyleFunc` fusionne après lui et suit les signaux qu’il lit, y compris l’apparence, pendant que la boîte de dialogue est ouverte et lorsqu’elle est rouverte. Par exemple, `StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} }` suit le thème, où `colors := ui.ColorsOf(cx)`. Ces deux champs fonctionnent également sur `ui.AlertDialog`. Une couleur nulle laisse la couleur existante en place ; utilisez une couleur transparente non nulle telle que `core.RGBA(0xFFFFFF, 0)` pour l’effacer.

## Feuilles

`ui.Sheet` est un panneau en bas par-dessus un arrière-plan assombri ; un appui sur l’arrière-plan, Échap ou le bouton retour appelle `OnClose`.

Définissez `Label` ou `LabelFunc` pour son nom accessible de boîte de dialogue, et `TestID` pour retrouver le panneau dans les tests. `LabelFunc` suit ses signaux sans remplacer le contenu ouvert. `CloseOnSwipe: true` appelle également `OnClose` lorsqu’un balayage vers le bas dépasse le seuil de fermeture ; mettez à jour `Open` dans ce callback pour la fermer.

Définissez `Style` ou `StyleFunc` pour personnaliser l’arrière-plan, les rayons des coins et les marges internes de la surface tout en conservant la poignée et la gestion de la zone de sécurité. `StyleFunc` suit les signaux qu’il lit. Un `CornerRadius` sans `CornerRadii` arrondit tous les coins au lieu du haut arrondi par défaut.

```go preview=dialog/sheet height=320
ui.Sheet{Open: filters.Get, OnClose: func() { filters.Set(false) }, Children: filterControls}
```

## Alertes

`ui.AlertDialog` demande à l’utilisateur de confirmer ou d’acquiescer quelque chose. Chaque action la ferme, contrairement à un appui à l’extérieur.

```go preview=dialog/alert height=320
ui.AlertDialog{
	Open:         confirm.Get,
	OnOpenChange: func(v bool, _ base.Reason) { confirm.Set(v) },
	Title:        "Delete the note?",
	Description:  "This cannot be undone.",
	Actions: []ui.AlertAction{
		{Text: "Cancel", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
		{Text: "Delete", Color: colors.Danger, Role: core.AlertDestructive, Default: true, OnPress: remove},
	},
}
```

Une action portant `Role: core.AlertCancel` s’exécute sur Échap ou retour, suivie par `OnOpenChange(false, base.ReasonCancel)`. `Default: true` donne le focus clavier à une action lorsque l’alerte s’ouvre sur ordinateur de bureau, le web et Apple TV, et fait en sorte qu’Entrée la choisisse ; un appui sur une action exécute `OnPress`, puis `OnOpenChange(false, base.ReasonSelect)`. Entrée exécute l’action par défaut tant que le focus clavier ne se trouve sur aucune autre action ni aucun champ : lorsque le focus est déplacé vers une autre action, Entrée appuie sur celle-ci, et une boîte de dialogue par-dessus l’alerte conserve Entrée. Une fois qu’une action s’est exécutée, l’alerte se ferme, quoi que fasse `OnOpenChange`, et n’en exécute aucune autre tant que `Open` ne repasse pas à false puis à true. Si plusieurs actions portent le même marqueur, la première l’emporte. Sans marqueurs, Échap ferme sans exécuter d’action et Entrée n’a pas d’action par défaut. `Role: core.AlertDestructive` décrit une action ; `Variant` ou `Color` choisit son apparence.

Pour l’alerte ou la feuille d’actions propre au système, utilisez `cx.ShowAlert` ([Services plateforme](../services.md#alerts-and-the-clipboard)).

## Comportement

Une fenêtre modale bloque l’application située derrière elle, prend le focus clavier et se ferme sur Échap et retour sauf indication contraire. Sur les écrans tactiles, une boîte de dialogue est une feuille qui suit un glissement vers le bas et se ferme au-delà d’un seuil.

## Accessibilité

`ui.Dialog` signale `RoleDialog`, ou son `Role` explicite, et `ui.AlertDialog` signale `RoleAlertDialog` sur ordinateur et sur écran tactile. Les deux lisent leur titre, ou `Label` lorsque le titre ne suffit pas, suivi de la description. Le focus entre dans une boîte de dialogue à son ouverture et revient à sa fermeture ; l’arrière-plan n’est pas un arrêt. Les boutons du pied conservent leurs rôles et leurs libellés.

## Plateforme

Sur Android et les écrans tactiles, une boîte de dialogue est une feuille surgissant du bas ; sur ordinateur de bureau, c’est une surface centrée avec un arrière-plan. Le contenu est le même ; seule la présentation change.

## Voir aussi

- [Navigation](../navigation.md) : pages, onglets et fenêtres modales ensemble.
- [Menu](menu.md) : commandes ancrées et menus contextuels.
- [Accessibilité](../accessibility.md) : focus et rôles des boîtes de dialogue.
- Les pages Dialog, Sheet et AlertDialog de la galerie.
- [Composants sans style](../authoring/base.md#dialog) : `base.Dialog` et `base.Sheet`, avec les présentations d’iOS et vos propres mouvements.

---
title: Menu
description: Un menu depuis un déclencheur, un menu contextuel et leurs éléments.
group: Composants
order: 42
---

`ui.Menu` ouvre des `ui.MenuItem` depuis un bouton : à côté de celui-ci, ou sous forme de feuille depuis le bas avec `Presentation: base.PresentSheet`, quel que soit l’appareil. `ui.ContextMenu` ouvre les mêmes éléments à la position du pointeur.

```go preview=menu/basic height=280
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", Icon: lucide.Pencil, OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", Shortcut: "mod+d", OnSelect: duplicate},
	ui.MenuSeparator{},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

Le menu s’ouvre et se ferme lui-même ; `Open` et `OnOpenChange` permettent à l’application d’en garder le contrôle. `Trigger` définit l’apparence et le contenu du bouton, et le menu intercepte ses appuis. Une fenêtre contextuelle ne dépasse jamais l’espace disponible à côté de son déclencheur : elle s’ouvre du côté où elle tient en entier et ses éléments défilent au-delà de cet espace, ou au-delà de `MaxHeight`.

## Éléments

Un `ui.MenuItem` affiche `Text`, une `Icon` facultative et un `Shortcut` (`"mod+c"`, formaté selon les conventions de la plateforme dans une fenêtre contextuelle), et signale `OnSelect` en fermant le menu sauf si `NoCloseOnSelect` est défini. `Disabled` le désactive, `Checked` affiche une coche à côté, et `Variant: ui.MenuItemDanger` indique une action destructive. `ui.MenuLabel` intitule un groupe et `ui.MenuSeparator` les sépare. Un `ui.Submenu` contient d’autres éléments et s’ouvre à côté de l’élément ou, dans une feuille, à la place des éléments qui l’entourent.

```go preview=menu/checked height=320
ui.Menu{Trigger: ui.Button{Text: "View", Variant: ui.ButtonGhost}, Children: []core.Node{
	ui.MenuLabel{Text: "Sort by"},
	ui.MenuItem{Text: "Name", Checked: func() bool { return sort.Get() == "name" }, OnSelect: func() { sort.Set("name") }},
	ui.MenuItem{Text: "Date", Checked: func() bool { return sort.Get() == "date" }, OnSelect: func() { sort.Set("date") }},
	ui.MenuSeparator{},
	ui.Submenu{Text: "Share", Children: []core.Node{
		ui.MenuItem{Text: "Copy link", OnSelect: copyLink},
		ui.MenuItem{Text: "Email", OnSelect: emailLink},
	}},
}}
```

## Menus contextuels

`ui.ContextMenu` ouvre ses éléments lors d’un clic secondaire, ou d’un appui long sur écran tactile, à l’emplacement du pointeur au-dessus de son `Trigger`.

```go preview=menu/context height=220
ui.ContextMenu{Trigger: fileRow, Children: []core.Node{
	ui.MenuItem{Text: "Open", OnSelect: open},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Comportement

Échap, le bouton retour et un appui à l’extérieur ferment un menu. Le menu est une fenêtre contextuelle positionnée selon `Placement` et `Align`, ou une feuille depuis le bas lorsqu’il est présenté ainsi. Sur ordinateur, Bas, Haut, Début, Fin, Page suivante et Page précédente parcourent les éléments actifs, en commençant par celui sous le pointeur, Droite ouvre un sous-menu et Gauche le ferme, et Entrée sélectionne. Un menu ouvert au clavier donne le focus à son premier élément, et le focus revient sur le déclencheur à la fermeture du menu.

## Accessibilité

Le menu signale `RoleMenu` et chaque élément `RoleMenuItem`. `Label` nomme le déclencheur lorsque son contenu ne le fait pas. Un lecteur d’écran lit le texte d’un élément et indique s’il est désactivé.

## Plateformes

Le déclencheur et les éléments appartiennent à l’application ; la feuille, la fenêtre contextuelle, le focus, Échap et le bouton retour relèvent de la plateforme. C’est l’application qui choisit la fenêtre contextuelle ou la feuille ; NSGo n’impose ni l’une ni l’autre selon l’appareil.

## Voir aussi

- [Sélecteur](select.md) : une valeur issue d’une liste.
- [Boîte de dialogue](dialog.md) : une surface modale.
- [Onglets](tabs.md) : des sections affichées simultanément.
- Les pages Menu, ContextMenu et Command de la galerie.
- [Composants sans style](../authoring/base.md#menu) : `base.Menu`, pour un menu avec votre propre apparence.

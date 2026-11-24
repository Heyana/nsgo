---
title: Onglets
description: Une liste d’onglets, un contrôle segmenté et la barre d’onglets système.
group: Composants
order: 40
---

`ui.Tabs[T]` permet de choisir une valeur parmi quelques-unes, sous forme de contrôle segmenté ou de liste d’onglets ; l’application affiche à côté le contenu correspondant à la valeur choisie. Une liste d’onglets constitue un seul arrêt de tabulation : les flèches déplacent la sélection d’un onglet à l’autre.

```go preview=tabs/basic
section := core.NewSignal(cx, "overview")
ui.Tabs[string]{
	Options: []ui.Option[string]{
		{Value: "overview", Label: "Overview"},
		{Value: "activity", Label: "Activity"},
		{Value: "settings", Label: "Settings"},
	},
	Value:    section.Get,
	OnChange: section.Set,
	Label:    "Sections",
}
```

## Variantes et tailles

`Variant` vaut `TabsSegmented` (par défaut, un curseur surélevé dans une piste en retrait) ou `TabsUnderline` (un trait d’accentuation sous l’onglet sélectionné). Une variante soulignée peut définir `IndicatorColor` et utiliser `IndicatorFit: TabsIndicatorContent` pour suivre son libellé et son icône plutôt que l’onglet entier ; en TSX, il s’agit de `indicatorColor` et `indicatorFit="content"`. `Size` vaut `SizeDefault`, `SizeSmall` ou `SizeLarge`, et `Equal` donne la même largeur à chaque onglet au lieu de l’adapter à son libellé.

```go preview=tabs/underline
ui.Tabs[string]{Variant: ui.TabsUnderline, Equal: true, Size: ui.SizeSmall, Options: tabs, Value: section.Get, OnChange: section.Set}
```

Une option peut porter une `Icon`, une fonction `Disabled`, un nœud `Content` et un `TestID`. `Content` remplace le libellé et hérite du style de texte de l’onglet.

Utilisez `OptionsFunc` lorsque les onglets suivent un signal. NSGo met à jour les éléments sur place selon leurs valeurs, en conservant l’onglet sélectionné et le focus clavier tant que cette valeur reste proposée ; une valeur qui disparaît reste non sélectionnée jusqu’à ce que l’application la modifie.

## Barres d’onglets

`ui.TabShell` maintient des sections de premier niveau à côté d’une barre d’onglets à toute largeur de fenêtre — en bas par défaut, ou verticalement le long du bord de début ou de fin lorsque `Edge` (ou `EdgeFunc`) vaut `mobile.TabBarLeading`, `mobile.TabBarTrailing` ou `mobile.TabBarAutomatic` (qui suit `cx.Env().VerticalBarEdge()`). Il utilise la barre système lorsque chaque onglet fournit `SystemIcon` ou une `Icon` SVG, sur n’importe quel bord sur iOS et sur le bord inférieur sur Android ; un `SystemIcon` qui désigne un SF Symbol sur iOS ou un drawable sur Android a priorité, et le SVG s’affiche dans le cas contraire. Sinon, il dessine la barre avec les icônes SVG fournies. Une barre qui occupe son propre espace reste d’elle-même à l’écart de la zone de sécurité de son bord, de sorte que `core.EnvSafeArea` sur ce bord d’une section voisine, y compris celui d’une page `base.Nav`, vaut 0 ; sous la barre système inférieure flottante d’iOS, gardez le contenu dégagé avec `ui.TabBarInset`.

Définissez `ShellTab.TextFunc` pour des titres d’onglets qui suivent la locale ou l’état de l’application, et `TabShell.LabelFunc` pour le nom accessible de la barre. Changer un titre préserve la section sélectionnée et l’onglet existant tant que son icône reste identique.

`mobile.TabBar` est la barre d’onglets système sur iOS et Android, avec `Items`, `Selected`, `OnChange`, et en option `Edge` (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing` ou `mobile.TabBarAutomatic`) / `EdgeFunc`. Chaque `mobile.TabBarItem` prend un `Title` et un nom d’icône de plateforme dans `Icon` ; `SVG: lucide.House()` s’affiche partout où `Icon` ne désigne aucun SF Symbol sur iOS ni aucun drawable sur Android, ajusté dans 24 points et teinté pour la sélection. En TSX, `svg` prend un document SVG et `edge` prend `"bottom"`, `"leading"`, `"trailing"` ou `"automatic"`. Ailleurs, `mobile.TabBar` n’occupe aucun espace, affichez donc `ui.Tabs` à la place.

`ui.SidebarShell` maintient d’elle-même le contenu de sa barre latérale, de son pied de page, de ses en-têtes, de son contenu et de son panneau droit à l’écart des zones de sécurité gauche et droite, leurs arrière-plans s’étendant jusqu’aux bords de la fenêtre, de sorte que `core.EnvSafeArea` sur ces côtés vaut 0 à l’intérieur, et `ui.NavHeader` ajoute la zone de sécurité latérale à sa marge interne. Le contenu d’une `ui.NavPage` ou d’une section de `TabShell` se tient lui-même à l’écart de la zone de sécurité, avec `PaddingEnv`, afin que ce qui doit s’étendre jusqu’aux bords, comme une image pleine largeur, puisse le faire.

## Accessibilité

La liste signale `RoleTabList`, chaque onglet `RoleTab`, et l’onglet sélectionné son état. La liste est un seul arrêt de tabulation ; Gauche et Haut, Droite et Bas, Début et Fin déplacent entre les onglets activés, et l’onglet atteint signale un appui. `Label` nomme la liste.

## Plateforme

iOS et Android affichent la barre d’onglets système avec leur propre apparence et leurs propres gestes ; macOS, Windows, Linux et le Web affichent `ui.Tabs`. Le contenu de l’application est identique sur chaque cible.

## Voir aussi

- [Navigation](../navigation.md) : pages et barres d’onglets dans une application.
- [Menu](menu.md) : des commandes plutôt que des sections.
- [Sélection](select.md) : de nombreuses options dans une liste.
- Les pages Tabs et NavStack de la galerie.
- [Composants sans style](../authoring/base.md#tabs) : `base.Tabs`, pour des onglets avec votre propre apparence.

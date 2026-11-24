---
title: Liste
description: Un nombre quelconque de lignes, construites uniquement à l’écran, avec sélection et activation.
group: Composants
order: 38
---

`ui.List` affiche des lignes dans une vue défilante native et ne construit que les lignes proches de l’écran, avec prise en charge de la sélection, de l’activation et du clavier.

```go preview=list/basic height=280
ui.List{
	Count:         func() int { return len(items) },
	Key:           func(i int) any { return items[i].ID },
	EstimatedSize: 44,
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		item := items[row.Index()]
		return ui.Text(item.Title, 13, ui.Regular, colors.Text)
	},
}
```

## Lignes et identité

`Count` indique le nombre de lignes et `Row` en construit une. `Key` donne son identité à une ligne : les lignes qui restent conservent leurs cellules et leur état lorsque des lignes apparaissent ou disparaissent au-dessus d’elles. Utilisez un identifiant plutôt que l’index lorsque des lignes peuvent être ajoutées, supprimées ou réordonnées ; l’index est un signal qui suit la cellule lorsqu’elle affiche une autre ligne.

```go
ui.List{Count: count, Key: func(i int) any { return items[i].ID }, Row: func(cx *core.Scope, row core.ListRow) core.Node {
	return ui.Text(items[row.Index()].Title, 13, ui.Regular, colors.Text)
}}
```

`EstimatedSize` est la hauteur attendue d’une ligne, soit 50 points lorsqu’elle vaut 0. Les lignes peuvent avoir des hauteurs différentes ; la liste mesure celles qu’elle construit et estime les autres.

## Mise en page

`Header`, `Footer` et `Empty` contiennent le contenu autour des lignes et lorsqu’il n’y en a aucune. `Gap` sépare les lignes, `Columns` les place côte à côte, et le remplissage intérieur (*padding*) de la liste s’applique à son contenu, afin que la barre de défilement reste contre le bord. Donnez une hauteur à la liste ou laissez-la remplir son parent.

## Sélection et activation

`ui.List` prend une `Selection`, créée avec `ui.NewSelection(cx, multiple)`. Les touches, les libellés, l’activation et le clavier la suivent : Haut, Bas, Début et Fin déplacent le curseur, Maj étend la sélection, Mod+A sélectionne tout, Espace bascule l’élément dans une sélection multiple, et Entrée ou un double-clic active une ligne.

```go preview=list/selection height=320
selected := ui.NewSelection(cx, true)
ui.List{
	Count: count, Key: func(i int) any { return items[i].ID },
	Selection: selected,
	OnActivate: func(i int) { open(items[i]) },
	RowLabel:   func(i int) string { return items[i].Title },
	Row: func(cx *core.Scope, row core.ListRow) core.Node { return rowContent(cx, items[row.Index()]) },
}
```

`OnSelectionChange` s’exécute après qu’un appui, une touche ou la saisie anticipée (*typeahead*) a modifié les lignes choisies, et non lorsque l’application définit elle-même `Selection`, afin qu’un panneau de détails puisse suivre le choix de l’utilisateur pendant que l’application déplace la sélection vers un élément ouvert d’une autre manière.

`TapSelects` fait qu’un appui sur téléphone sélectionne la ligne plutôt que de l’activer. `RowLabel` correspond à ce que lisent les technologies d’assistance et à ce sur quoi porte la saisie anticipée.

## Chargement et défilement

`OnLoadMore` s’exécute à l’approche de la fin, une fois par nombre de lignes. `Ref` permet de défiler avec `ScrollToIndexAt`, `ScrollToEnd` et `ScrollToIndex`. `core.List`, la liste de base sous-jacente, propose également `OnLoadMoreAtStart`, `Inverted` pour une messagerie, `StickyHeaderIndices`, `MaintainVisibleContentPosition`, `Viewport` et `ViewabilityConfig` pour les cas de bas niveau ([Listes et défilement](../lists.md)).

## Accessibilité

La liste est exposée comme une liste ou une zone de liste (*listbox*), nommée par `Label`, avec le texte d’une ligne ou `RowLabel` comme nom de ligne. Une liste dotée d’une sélection signale la ligne du curseur et l’annonce à mesure que le clavier se déplace. Lorsqu’un lecteur d’écran sort de la zone visible, il demande à la liste de défiler jusqu’à cet endroit, ce qui construit les lignes atteintes.

## Plateformes

La vue défilante native et la virtualisation sont identiques sur chaque cible. Les lignes sont recyclées, l’état d’une ligne doit donc suivre sa `Key` ; conserver l’état de construction dans l’application plutôt que dans une ligne est la règle habituelle.

## Voir aussi

- [Listes et défilement](../lists.md) : le guide complet, incluant les en-têtes figés et le chargement incrémental.
- [Défilement](scroll.md) : une vue défilante simple.
- [DataTable](../lists.md#tableaux) : un tableau comportant de nombreuses lignes et colonnes.
- Les pages List, Table et DataTable de la galerie.

---
title: Listes et défilement
description: Vues défilantes avec gestes natifs et listes recyclant les lignes pour n’importe quel nombre d’éléments.
group: Concepts clés
order: 55
---

## Une vue défilante

`core.Scroll` est la vue défilante propre à la plateforme ; ainsi, l’inertie, les rebonds, la barre de défilement, l’aimantation et le défilement tactile sont natifs. Ses enfants sont disposés en colonne, ou en ligne avec `Horizontal`, sur toute la longueur nécessaire.

```go
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

La vue défilante remplit son conteneur et le remplissage intérieur (*padding*) est appliqué à son contenu, afin que la barre de défilement reste contre le bord. Elle rétrécit pour s’adapter à son parent, sauf indication contraire dans son style. Définissez `NoShrink` lorsqu’elle doit conserver sa taille demandée pendant qu’un élément frère rétrécit. `HideIndicator` masque l’indicateur de la plateforme, et `Bar` dessine celui de l’application.

`Behavior` et `BehaviorFunc` choisissent son mode de défilement : `Paging`, `SnapToInterval` et `SnapToOffsets` avec `SnapToAlignment`, un `DecelerationRate`, `NoBounces`, `ContentInset` et les règles du clavier. `MinZoomScale` et `MaxZoomScale` permettent de zoomer par pincement, `OnRefresh` et `Refreshing` déclenchent l’actualisation par glissement vers le bas, et `OffsetValues` suit son décalage sur le fil principal pour les effets qui se déplacent avec lui.

```go
page := core.Scroll{
	OnScroll: func(e core.ScrollEvent) { y.Set(e.Offset.Y) },
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: []core.Node{...},
}
```

`OnScroll` signale le décalage au plus une fois par tour, ou une fois par `ScrollEventThrottle`. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd` et `OnScrollEnd` encadrent les gestes de l’utilisateur. `Ref` permet de faire défiler depuis le code avec `ScrollTo`, `ScrollToEnd`, `ZoomTo` et `ScrollToIndex` pour une liste.

## Une liste

`core.List` affiche de grandes quantités de données dans une vue défilante native tout en ne créant que les lignes proches de l’écran. `Count` indique le nombre de lignes et `Row` en construit une ; `Key` donne une identité à chaque ligne, afin que les lignes qui restent conservent leurs cellules lorsque des lignes apparaissent ou disparaissent au-dessus d’elles.

```go
core.List{
	Count:         func() int { return len(items.Get()) },
	Key:           func(i int) any { return items.Get()[i].ID },
	EstimatedSize: 56,
	Row: func(cx *core.Scope, r core.ListRow) core.Node {
		return core.View{Style: core.Style{Padding: core.All(12)}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return items.Get()[r.Index()].Title }},
		}}
	},
}
```

`Row` s’exécute une seule fois par cellule recyclée et met à jour `r.Index()` lorsque cette cellule est réutilisée pour une autre ligne ; lisez donc `r.Index()` et les signaux d’élément à l’intérieur de fonctions réactives (`ContentFunc`, `StyleFunc`, `NewMemo`) plutôt que de capturer une valeur ponctuelle lors de la construction de la ligne. Les cellules inactives sont mises en attente afin que leurs calculs patientent jusqu’à ce que la cellule soit de nouveau liée à une ligne active.

`Header`, `Footer`, `Empty` et `Separator` contiennent les éléments autour et entre les lignes. `NumColumns` place les lignes côte à côte, `Gap` et `ColumnGap` les séparent, et `Horizontal` fait défiler horizontalement. `StickyHeaderIndices` épingle ces lignes au début de la zone d’affichage jusqu’à ce que la suivante les pousse hors champ, et `StickyHeaderOffset` préserve un espace libre pour un en-tête au-dessus d’elles.

`OnViewableItemsChange` signale les lignes visibles selon les seuils de `ViewabilityConfig`, et `OnLoadMore` s’exécute lorsque la zone d’affichage approche de la fin, une fois par nombre de lignes. `OnLoadMoreAtStart` fait de même au début, et `InitialScrollIndex` ouvre la liste sur une ligne donnée. `Alignment: core.ListAlignEnd` cale une liste sur sa fin, comme une messagerie avec ses lignes dans l’ordre de lecture : elle s’ouvre sur sa ligne la plus récente, suit les nouvelles lignes tant que l’utilisateur se trouve à la fin et maintient une conversation courte en bas. `Inverted` place plutôt la ligne 0 à la fin, et `MaintainVisibleContentPosition` suit la fin d’une liste qui s’allonge lorsque l’utilisateur en est proche.

```go
core.List{Ref: list, Count: count, EstimatedSize: 64, Row: row, OnLoadMore: loadMore,
	StickyHeaderIndices: headers(cx), Footer: core.View{Style: core.Style{Height: core.Pt(24)}}}
```

`list.ScrollToIndexAt(index, 0.5, 0, true)` fait défiler une ligne jusqu’au milieu de la zone d’affichage, et `list.ScrollToEnd(true)` jusqu’à la fin. `list.ListPosition()` lit l’endroit où commence la zone d’affichage, une ligne et la progression dans celle-ci, pour conserver une position de lecture : `list.ScrollToIndexAt(p.Index, 0, -p.Offset, false)` ramène à cet endroit une liste recréée. Une liste peut également figurer sur une même page avec d’autres contenus : définissez `Viewport: core.ListViewportParent`, et la vue défilante qui l’entoure fait défiler la liste avec le reste.

## Tableaux

`base/datatable` affiche un tableau d’un million de lignes avec des lignes et des colonnes virtualisées, un en-tête fixe, des colonnes figées à gauche (`Fixed`) ou à droite (`FixedRight`) pendant que les autres défilent dessous, le tri, la sélection simple ou multiple de lignes, le redimensionnement et le déplacement des colonnes, ainsi que les cellules propres à l’application. `MultiSelect` prend en charge le basculement avec Commande ou Contrôle, les plages avec Maj et les extensions au clavier avec Maj, tandis que `SelectedRows` et `OnSelectedRowsChange` laissent l’application contrôler les lignes. `CellTooltip` dote une cellule d’une infobulle, qui décrit également la cellule aux lecteurs d’écran, stylisée par `Styles.CellTooltip` ; ce que dessine `RenderCell` ne reçoit pas de pointeur, si bien qu’un `ui.Tooltip` à l’intérieur d’une cellule ne s’affiche jamais. `base.Table` est un tableau simple des lignes de l’application pour des données plus modestes, et `base/plot` trace des graphiques.

Utilisez `DataTable.LabelFunc` pour un nom de grille qui suit la langue ou l’état de l’application. `ColumnGroup.LabelFunc` met à jour un en-tête de groupe de colonnes tant que le groupe reste visible.

## Suite

- [Animation](animation.md) : effets qui suivent une vue défilante.
- [Texte](text.md) : lignes de texte qui passent à la ligne.
- [Navigation](navigation.md) : écrans dont le contenu défile.

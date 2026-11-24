---
title: Mise en page
description: Flexbox, grille, dimensions et positionnement, avec la zone de sécurité et le clavier dans la mise en page.
group: Concepts clés
order: 51
---

## Boîtes

Chaque élément est une boîte disposée par le moteur. Les valeurs par défaut sont celles de React Native : une flexbox en colonne, des enfants qui conservent leur taille à moins de s’agrandir ou de rétrécir, des éléments étirés sur l’axe transversal et des dimensions qui incluent les marges internes et les bordures.

```go
core.View{
	Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.All(16), AlignItems: core.AlignCenter},
	Children: []core.Node{
		core.Text{Content: "One"},
		core.Text{Content: "Two"},
	},
}
```

`Column`, `Row`, `ColumnReverse` et `RowReverse` définissent l’axe principal ; `Wrap` et `WrapReverse` permettent aux enfants de passer à la ligne suivante. `JustifyContent` les place le long de cet axe et `AlignItems`, ou `AlignSelf` sur un seul enfant, perpendiculairement à celui-ci. `Gap`, `RowGap` et `ColumnGap` les séparent, et les marges internes et externes de `style` utilisent `core.All`, `core.Symmetric` ou `core.Edges`.

## Dimensions

`Width`, `Height`, `MinWidth`, `MaxWidth` et `FlexBasis` prennent `core.Pt` pour des points ou `core.Pct` pour un pourcentage du parent. `FlexGrow` répartit l’espace disponible et `FlexShrink` permet à une boîte d’en céder une partie ; une boîte sans aucun des deux conserve sa taille. `AspectRatio` correspond à la largeur divisée par la hauteur. `AutoMargin` absorbe l’espace disponible sur les côtés qu’il nomme, si bien qu’un côté pousse la boîte vers l’autre.

```go
core.View{Style: core.Style{Direction: core.Row, Gap: 16}, Children: []core.Node{
	core.View{Style: core.Style{Width: core.Pt(220), FlexShrink: 0}},
	core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}},
}}
```

Le contenu d’une boîte lui donne sa taille tant qu’une largeur ou une hauteur n’indique pas le contraire. `core.MinContent`, `core.MaxContent` et `core.FitContent` adoptent les dimensions nommées par CSS : la largeur min-content d’un texte est celle de son mot le plus large. Un texte à l’intérieur d’une ligne ne rétrécit et ne passe à la ligne qu’avec `FlexShrink: 1` ; ajoutez `FlexGrow: 1` lorsqu’il doit occuper l’espace restant. Un minimum non défini vaut déjà 0, comme dans React Native, donc `MinWidth: core.Pt(0)` ne change rien et un texte qui rétrécit coupe ses mots lorsque la ligne est plus étroite que son mot le plus large. `MinWidth: core.AutoMinimum` l’arrête à son mot le plus large, comme le fait le minimum automatique de CSS.

Un écran qui remplit la fenêtre prend `FlexGrow: 1` et `FlexShrink: 1`, tout comme chaque boîte située entre lui et une liste ou une vue défilante devant défiler. Avec `FlexGrow` seul, une boîte s’agrandit jusqu’à la taille de son contenu, de sorte qu’une longue liste l’étire au-delà de la fenêtre au lieu de défiler.

## Grille

`DisplayGrid` dispose les enfants selon CSS Grid. `GridTemplateColumns` et `GridTemplateRows` acceptent la syntaxe de pistes CSS, et `GridRow` et `GridColumn` placent un élément par ligne ou par étendue.

```go
core.View{Style: core.Style{
	Display: core.DisplayGrid,
	GridTemplateColumns: "repeat(3, 1fr)",
	GridAutoRows: "44px",
	Gap: 12,
}, Children: []core.Node{
	core.View{Style: core.Style{GridColumn: core.GridLines{Start: core.GridSpan(2)}}},
}}
```

`repeat(auto-fill, minmax(160px, 1fr))` crée autant de colonnes qu’il en tient dans l’espace disponible. `GridFlow` choisit l’ordre de remplissage de la grille, et `JustifyItems`, `JustifySelf` et `AlignItems` placent les éléments dans leurs cellules.

## Positionnement

`Relative` est la valeur par défaut et maintient une boîte dans le flux ; `Top`, `Right`, `Bottom` et `Left` la décalent. `Absolute` la sort du flux et la place par rapport à son plus proche ancêtre positionné. `Fixed` la place dans la vue racine ou sa fenêtre modale, indépendamment du défilement, et `Sticky` l’épingle à l’intérieur de son parent dans la plus proche vue défilante : `Top` depuis le haut, ou `Bottom` depuis le bas lorsque `Top` est auto (`Left` et `Right` horizontalement). Un pied de page avec `Position: core.Sticky, Bottom: core.Pt(0)` reste en bas pendant qu’un long contenu défile et suit directement un contenu court. `ZIndex` empile les boîtes parmi leurs sœurs.

```go
core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Right: core.Pt(8)}}
```

`Static` maintient une boîte dans le flux et ignore ses décalages, de sorte que les enfants positionnés en absolu trouvent un ancêtre au-dessus d’elle. `Overflow` découpe les enfants aux limites d’une boîte, et `DisplayNone` retire une boîte de la mise en page.

Les ombres extérieures et le contour propres à la boîte restent à l’extérieur de son découpage, comme en CSS, sur Android, Windows, Linux, le web et Apple, pour tout type de vue, y compris avec des coins de rayons différents et du contenu défilant zoomé. Tant que cela n’est pas vérifié sur un iPhone physique, le mode `Bare` d’`ui.Dialog` place toujours son découpage sur une vue interne dotée des mêmes coins, décalée vers l’intérieur par la bordure, ce qui les préserve sur chaque cible :

```go
core.View{Style: core.Style{CornerRadius: 16, Shadows: []core.BoxShadow{ui.Shadow(0, 8, 24, -6, ui.Black(0.18))}},
	Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, CornerRadius: 16, Overflow: core.Hidden}, Children: content}}}
```

## Direction et environnement

`LayoutDirection` dispose un sous-arbre de gauche à droite ou de droite à gauche, par-dessus la direction de l’application : `DirectionRTL` et `DirectionLTR` la définissent, et la valeur par défaut hérite. La gauche et la droite dans les marges externes, les marges internes, les bordures et les décalages s’inversent selon la direction, jouant ainsi le rôle de début et de fin comme dans React Native. `PhysicalEdges` et `PhysicalBorders` gardent les côtés nommés à leur place physique. `WritingDirection` ordonne les segments de texte à direction mixte, et `WritingAuto` la détermine à partir de la première lettre à direction forte.

`PaddingEnv` et `MarginEnv` ajoutent les valeurs de l’environnement à un côté. `core.EnvSafeArea` correspond à l’encart de la zone de sécurité sur ce bord et `core.EnvKeyboard` à la hauteur du clavier logiciel, afin que le contenu reste au-dessus du clavier pendant que son déplacement s’anime avec lui.

```go
core.View{Style: core.Style{
	Padding: core.All(16),
	PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea},
}}
```

`core.EnvAll(core.EnvSafeArea)` désigne chaque bord, et `cx.Env().SafeArea()` ainsi que `cx.Env().KeyboardHeight()` lisent ces valeurs dans le code.

Sur un téléphone pliable ou à double écran, `cx.Env().Geometry()` énumère les régions dans lesquelles un pli ou une charnière divise la fenêtre, chacune avec la zone de sécurité qui s’y trouve, ainsi que les plis entre elles ; ses champs `State` et `Angle` indiquent le degré d’ouverture de l’appareil (`core.HingeClosed`, `HingeHalfOpen` ou `HingeOpen`), même lorsqu’aucun pli ne traverse la fenêtre, comme sur un écran externe ou un écran ouvert à plat. Les autres appareils signalent une seule région et `HingeUnknown`. Sur iOS 27.1, `cx.Env().VerticalBarEdge()` indique le bord choisi par le système pour une barre verticale (`core.VerticalBarLeading`, `core.VerticalBarTrailing` ou `core.VerticalBarUnspecified`), que `mobile.TabBar` et `ui.TabShell` suivent lorsque `Edge` vaut `mobile.TabBarAutomatic`. Rien n’est disposé implicitement selon la géométrie : l’application choisit ses propres volets.

## Classes responsives

`Class` accepte les classes Tailwind CSS : `flex`, `flex-col`, `flex-wrap`, `items-center`, `justify-between`, `gap-3`, `p-4`, `w-full`, `grid`, `grid-cols-3`, `col-span-2` et les autres. Les points de rupture suivent la fenêtre, et l’apparence de l’affichage, la direction et la réduction des mouvements suivent l’environnement :

```go
core.View{Class: "flex flex-col gap-3 p-4 md:flex-row md:gap-6 dark:bg-zinc-900 rtl:flex-row-reverse"}
```

`sm:`, `md:`, `lg:`, `xl:` et `2xl:` s’appliquent à partir de 40, 48, 64, 80 et 96 rem de largeur ; `max-md:` s’applique en dessous d’un point de rupture. `portrait:`, `dark:`, `rtl:`, `motion-reduce:`, `contrast-more:`, `pointer-fine:`, `pointer-coarse:`, `ios:`, `android:` et `web:` suivent ce qu’indique l’environnement. Les classes sont fusionnées sous un `Style`, et une classe d’état (`hover:`) devient un style d’état, ce qui permet de combiner les deux.

## Suite

- [Styles et thèmes](styles.md) : couleurs, coins, ombres et apparence d’un sous-arbre.
- [Texte](text.md) : comment le texte prend sa taille et passe à la ligne.
- [Listes et défilement](lists.md) : contenu qui défile et lignes recyclées.

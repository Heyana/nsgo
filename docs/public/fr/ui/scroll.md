---
title: Défilement
description: La vue défilante propre à la plateforme, avec pagination, aimantation, zoom et actualisation.
group: Composants
order: 39
---

`core.Scroll` est la vue défilante propre à la plateforme. Ses enfants sont disposés en colonne, ou en ligne avec `Horizontal`, et elle remplit son conteneur tandis que le contenu défile.

```go preview=scroll/basic height=240
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

Le remplissage intérieur (*padding*) s’applique au contenu et non à la vue défilante, afin que la barre de défilement reste contre le bord. La vue défilante rétrécit pour s’adapter à son parent, sauf indication contraire dans son style. Définissez `NoShrink` lorsqu’elle doit conserver la taille demandée pendant qu’un élément frère rétrécit.

## Comportement

`Behavior` détermine son comportement de défilement : `Paging`, `SnapToInterval` ou `SnapToOffsets` avec `SnapToAlignment`, un `DecelerationRate`, `NoBounces`, `ContentInset` et les règles du clavier.

```go preview=scroll/paging height=240
core.Scroll{
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: pages,
}
```

`HideIndicator` masque la barre de défilement de la plateforme et `Bar` dessine celle de l’application. `MinZoomScale` et `MaxZoomScale` permettent de zoomer par pincement ; `NoPinchZoom` les ignore. `CenterContent` centre un contenu plus petit que la zone d’affichage.

## Actualisation et événements

`OnRefresh` rend la vue actualisable ; définissez `Refreshing` pendant l’exécution du traitement. `PullValue` suit la distance de traction du contenu, pour un indicateur propre à l’application, et `Behavior.RefreshIndicator` définit sa hauteur.

```go
refreshing := core.NewSignal(cx, false)
core.Scroll{
	Refreshing: refreshing.Get,
	OnRefresh:  func() { refreshing.Set(true); reload(cx) },
	Children:   content,
}
```

`OnScroll` signale le décalage au plus une fois par tour, ou une fois par `ScrollEventThrottle` ; `Stale` signale une mesure antérieure à l’application par la plateforme d’un défilement déclenché depuis le code. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd` et `OnScrollEnd` encadrent les gestes de l’utilisateur.

`OffsetValues` suit le décalage sur le fil principal, pour un en-tête qui se replie sans commit à chaque image. `Ref` permet de faire défiler depuis le code avec `ScrollTo`, `ScrollToEnd` et `ZoomTo`, et la référence de toute vue interne l’amène dans la zone visible avec `Reveal`, en faisant défiler toutes les vues défilantes qui l’entourent :

```go
row.Reveal(core.RevealOptions{Vertical: core.ScrollAlignCenter, Animated: true})
```

`RevealBox` amène plutôt dans la zone visible une boîte à l’intérieur de la vue, telle que la boîte d’un mot issue de `TextRangeBoxes` d’un texte, et `Margin` préserve un espace libre sur les bords de la vue défilante la plus externe pour une barre superposée.

Une vue vers laquelle le clavier déplace le focus défile d’elle-même dans la zone visible.

## Mise en page

`Horizontal` fait défiler horizontalement. `Behavior.ContentInset` ajoute autour du contenu un espace qui défile avec lui. Pour une page qui remplit la zone de sécurité, utilisez `PaddingEnv` sur le contenu et non sur la vue défilante.

## Accessibilité

`Label` nomme la zone de défilement et `Role` indique ce qu’elle est, comme une zone de liste. Les gestes de défilement de la plateforme, le défilement au clavier et le défilement au lecteur d’écran sont conservés ; `Busy` signale une vue encore en cours de chargement.

## Plateformes

Chaque backend utilise la vue défilante propre à la plateforme, de sorte que l’inertie, les rebonds, les barres de défilement et le défilement tactile sont natifs. Les effets de bord diffèrent : macOS et iOS effectuent un effet élastique, Android étire, Windows et Linux s’arrêtent net, et le web suit le navigateur. `EdgeEffects` et `EdgeBar` ajoutent des barres par-dessus le contenu là où l’application le souhaite. Sur les téléphones et tablettes iOS et Android, les vues défilantes verticales et horizontales qui débordent affichent une poignée de défilement déplaçable au toucher le long de l’axe actif, qui parcourt toute l’étendue du contenu ; les vues courtes qui tiennent sans déborder, les vues avec zoom, `Behavior.NoScroll`, `HideIndicator` ainsi que les vues utilisant un `Bar` personnalisé ou `base.SetScrollbarTheme` la masquent. Dans les tests, `nsgotest.App.ScrollHandle` renvoie ses limites dans la zone d’affichage et `DragAt` ou `Touch` la fait glisser directement. Sur Windows, l’indicateur superposé apparaît lors de l’utilisation de la molette, du tactile, du glissement de la barre et de leur inertie, mais pas pour `ScrollTo`, `ScrollBy`, les révélations, les marges internes (*insets*) ou les animations gérés par l’application.

## Voir aussi

- [Listes et défilement](../lists.md) : le guide complet du défilement et des listes.
- [Liste](list.md) : des lignes qui se recyclent.
- [Mise en page](../layout.md) : les marges internes et la zone de sécurité.
- Les pages Scroll et Lists de la galerie.

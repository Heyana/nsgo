---
title: Styles et thèmes
description: Styles typés, classes Tailwind, styles d’état et thèmes clair et sombre.
group: Concepts clés
order: 52
---

## Un style

`core.Style` est une valeur Go unique regroupant la mise en page, l’apparence et le texte d’une boîte. La valeur zéro de chaque champ signifie qu’il n’est pas défini, ce qui permet aux styles de se composer : `Merge` remplace uniquement ce que le style suivant définit, et un enfant hérite des champs de texte définis par ses parents. Pour ramener une longueur à zéro par-dessus un style qui en définit une, utilisez `core.Zero` dans `Padding`, `Margin`, `Borders`, `BorderWidth`, `CornerRadius` ou un espacement (*gap*), comme le fait `core.Transparent` pour une couleur : `Padding: core.All(core.Zero)`.

```go
card := core.Style{
	Background: core.RGB(0xffffff),
	CornerRadius: 12,
	Padding: core.All(16),
	Gap: 12,
	BorderWidth: 1,
	BorderColor: core.RGBA(0x000000, 0.1),
}
```

Les styles acceptent les couleurs, coins, bordures, contours, ombres, dégradés, filtres, flous d’arrière-plan, masques, modes de fusion, transformations 3D et transitions de CSS. `core.RGB`, `core.RGBA` et `core.ParseColor` créent des couleurs, `core.LinearGradient` et ses équivalents créent des dégradés, `core.Ring` crée l’anneau (*ring*) de Tailwind, et `core.AllCorners` attribue à chaque coin son rayon. `BorderCurveContinuous` arrondit les coins dans le prolongement de leurs bords comme le fait iOS. Les ombres et le contour d’une boîte restent à l’extérieur de son découpage `Overflow`, sauf dans les cas indiqués dans [Mise en page](layout.md#position). Les filtres d’arrière-plan modifient ce qui se trouve derrière une vue comme le fait `backdrop-filter` en CSS : un `Blur` floute l’arrière-plan selon son rayon et les filtres de couleur en modifient les couleurs. iOS dessine le matériau système nommé par `View.IOS.BackdropMaterial` pour un flou sans filtres de couleur ; un flou nommant un matériau autre que `core.BackdropUltraThin` dessine ce matériau sur iOS et celui qui en est le plus proche dans AppKit sur macOS, en omettant les filtres de couleur. Linux et Android ne dessinent aucun filtre d’arrière-plan. `Style.GlassEffect` (`core.GlassRegular` ou `core.GlassClear`, `glassEffect` en TSX) dessine Liquid Glass à la place d’un flou d’arrière-plan sur macOS 26 et iOS 26 (et le shader de verre de NSGo sur Windows ; donnez un flou à une vue en verre comme solution de repli ailleurs). Définir `Style.GlassInteractive` (`glassInteractive` en TSX) active la réponse interactive d’appui du système pour Liquid Glass sur macOS 27 (`NSGlassEffectView.effectIsInteractive`) et iOS 26 (`UIGlassEffect.isInteractive`) tout en conservant le fonctionnement de `OnPress` et des enfants pressables.

`Transform` déplace la cible du pointeur avec la vue : les appuis, le survol et les gestes commencent là où elle est dessinée, et les positions de pointeur signalées annulent la transformation. Un enfant transformé hors de son parent reste atteignable, à moins qu’un découpage ou une fenêtre d’affichage défilante ne le masque. Les mini-programmes n’annulent pas encore les images clés exécutées par leur couche de rendu ([feuille de route](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md#mini-programs)).

```go
core.View{Style: core.Style{
	CornerRadii: core.Corners{TopLeft: 28, BottomRight: 28},
	BorderCurve: core.BorderCurveContinuous,
	Shadows:     []core.BoxShadow{core.Ring(1, core.RGBA(0x000000, 0.08)), ui.Shadow(0, 8, 24, -6, ui.Black(0.18))},
}}
```

## États

L’apparence d’une vue peut changer selon son état. `Hover`, `Focus`, `FocusWithin`, `Active`, `Selected`, `Checked`, `Disabled` et les états `aria-*` sont des valeurs `*Style` qui se fusionnent par-dessus le reste tant que l’état est actif. Une application définit `Selected`, `Checked`, `Disabled` et `Expanded` via des fonctions, et la plateforme signale les appuis et le focus.

`Active` et `group-active:` suivent une vue pendant qu’elle est pressée, ce qui nécessite un gestionnaire d’appui ou un `Responder` maintenant le toucher. Appliquer `Active`, `active:` ou `group` à une simple vue ne la rend ni pressable ni accessible par la touche Tab du clavier ; définissez `Focusable` lorsqu’elle doit recevoir le focus elle-même.

```go
core.View{
	OnPress: func(core.PressEvent) { open() },
	Style: core.Style{
		Background: core.RGB(0x007aff),
		Active:     &core.Style{Opacity: core.Alpha(0.85)},
		Hover:      &core.Style{Background: core.RGB(0x006be0)},
	},
	Disabled: func() bool { return saving },
}
```

Une partie d’un composant peut reprendre les états du contrôle qui l’entoure sans les signaler elle-même : `PartStates` fournit `Checked`, `Selected`, `Disabled` et les autres états, de sorte que la coche d’une case à cocher suit la case pour les lecteurs d’écran également.

## Classes

`Class` sur `View`, `Text`, `Span`, `Input`, `Image`, `Scroll` et `Canvas` accepte des classes Tailwind CSS 4, et chaque partie d’un composant `base` dispose d’une classe en plus de son style. Les utilitaires, variantes et valeurs arbitraires conservent leur signification dans Tailwind, sur son thème et sa palette par défaut.

```go
core.View{Class: "flex items-center gap-3 rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"}
```

Une classe se fusionne sous le style, de sorte que les champs du style l’emportent. Les classes peuvent suivre les changements avec `ClassFunc`, et une variante d’état (`hover:bg-zinc-100`, `checked:opacity-100`) devient l’état du style. `group` et `group-hover:` stylent les enfants selon l’état d’un parent, `first:` et `last:` suivent la position d’un enfant, et `aria-checked:`, `data-open:` et `focus-within:` suivent les états exposés par un contrôle. `pointer-fine:` et `pointer-coarse:` stylent un sous-arbre pour une souris ou un écran tactile, en correspondant à la caractéristique média `pointer` de CSS via `cx.Env().Pointer()` (la requête `pointer` du navigateur sur le web, fin sur les ordinateurs de bureau et Mac, grossier sur les téléphones, tablettes et montres, et aucun des deux sur les téléviseurs), et se combinent avec d’autres variantes telles que `dark:` et `hover:` afin que l’application choisisse explicitement les dimensions tactiles ou au pointeur plutôt que `core` ne les modifie implicitement.

Marquez un contrôle avec `peer` pour styler ses frères suivants selon son état, par exemple une case à cocher avec `Class: "peer"` suivie d’une étiquette avec `Class: "peer-checked:text-accent peer-disabled:opacity-50"`. Un `peer` ne style que les frères qui le suivent sous le même parent, `Show` et `Each` inclus, jamais les frères qui le précèdent ni les vues qu’il contient. Utilisez `peer/field` avec `peer-checked/field:` pour nommer un groupe, et `peer-data-invalid:` pour l’état booléen de données `invalid` d’un contrôle. Les marqueurs et les états se mettent à jour de manière réactive. Marquer une vue n’en fait pas un arrêt de tabulation ni une cible d’appui ; `peer-active:` ne suit donc qu’un `peer` qui accepte lui-même les appuis, comme un bouton.

`has-[>svg]:px-3` modifie les marges internes d’un parent lorsqu’il possède une icône comme enfant direct ; `[&_svg]:size-4 [&_svg]:shrink-0` dimensionne ses icônes descendantes. `svg` sélectionne `core.Icon` et `SVGView`, y compris à travers les régions conditionnelles. Ces sélecteurs se mettent à jour lorsque les enfants, les classes ou les états changent. `hover:[&_svg]` suit le survol du parent ; `[&_svg]:hover` suit celui de chaque icône. L’état `active` d’un sélecteur ne suit que les vues qui acceptent les appuis, comme un bouton, tout comme le fait `peer-active:`. Les sélecteurs correspondants sont départagés par leur spécificité avant l’ordre des variantes ; les styles en ligne l’emportent à moins que la classe ne soit marquée comme importante.

Les sélecteurs correspondent aux éléments NSGo sur chaque cible. Ils prennent en charge les listes, les combinateurs d’enfant (`>`), de descendant (`_`), de frère adjacent (`+`) et de frères suivants (`~`), les classes, la présence ou l’égalité de `data-*`, les rôles, les états booléens d’accessibilité pris en charge ainsi que `:is`, `:where`, `:not`, `:has` et les positions d’enfant. Les types sont `view`/`div`, `text`/`span` (nœuds texte entiers), `scroll`, `input`, `textarea`, `image`/`img`, `canvas`, `svg`, `video`, `webview` et `native`. Utilisez un seul `&` de niveau supérieur dans chaque branche d’un sélecteur arbitraire ; sans lui, le sélecteur teste l’élément lui-même. Les sélecteurs propres aux navigateurs, les pseudo-éléments, les règles `@`, les séquences d’échappement et un `&` explicite à l’intérieur d’une fonction ne sont pas reconnus ; ces sélecteurs n’atteignent ni les vues d’implémentation du backend ni les segments de texte individuels.

Les variantes `peer-*` dans `Span.Class` ou `Screen.HeaderClass` n’ont aucun frère à suivre. Elles sont ignorées et journalisées une fois par classe et par contexte ; placez plutôt la variante `peer` d’un segment sur le `Text` qui le contient.

Le champ `Tailwind` du thème étend le thème par défaut avec des couleurs, des points de rupture et des utilitaires, et une couleur absente de la palette désigne un rôle de `Palette` (`bg-accent`, `text-text-muted/70`) ou une couleur du thème, lue depuis le thème du nœud dans l’apparence qu’il affiche.

## Thèmes

Un `core.Theme` représente une apparence complète dans les deux modes : une `Palette` pour le mode clair et une pour le mode sombre, un rayon de coin et des familles de polices. Les rôles de la palette sont `Background`, `Surface`, `Text`, `TextMuted`, `Border`, `Accent`, `AccentText`, `Danger`, `Warning`, `Success`, `Selection`, `Focus` et `Scrollbar` ; `Colors` contient ceux qu’une bibliothèque ajoute, comme `inset` et `raised`. `core.DefaultTheme()`, qu’une application affiche tant qu’elle n’en définit pas un autre, est le thème neutre avec un accent bleu et les rôles d’`ui`, et `ui.WarmDuskTheme()` une palette chaleureuse.

```go
theme := core.DefaultTheme()
theme.Light.Accent, theme.Dark.Accent = core.RGB(0xbd481b), core.RGB(0xed986e)
theme.Radius = 10
cx.SetTheme(theme)
```

`cx.SetTheme` attribue un thème à la portée et à tout ce qui est construit en dessous, de sorte qu’une barre latérale ou un panneau peut avoir son propre thème ; un thème `nil` reprend celui du niveau supérieur. `core.ParseTheme` lit un fichier de thème YAML ou JSON, ou les variables CSS d’un fichier de thème shadcn/ui, avec des couleurs de rôle, un rayon et des familles de polices. `core.DefaultTheme`, le thème qu’une application affiche tant qu’elle n’en définit pas un, et `ui.WarmDuskTheme` renvoient des thèmes prêts à être modifiés.

`cx.SetAppearance` force l’ensemble de l’application en mode clair ou sombre, `cx.SetSubtreeAppearance` le fait pour un sous-arbre, et `core.AppearanceSystem` suit de nouveau le système. `cx.Dark()` indique quelle apparence affiche un sous-arbre, et `cx.Palette()` fournit ses couleurs avec les surcharges de l’hôte d’intégration appliquées.

Lorsqu’aucun ancêtre ne définit `Style.Color`, le texte et les icônes `currentColor` utilisent `Palette.Text` du sous-arbre, suivant sur place les changements de thème, d’apparence et de jetons de l’hôte. Une couleur explicite, y compris `core.Transparent`, continue d’être héritée.

## Couleurs dans les composants

Un composant lit ses couleurs là où il est construit et dans ses `StyleFunc`, afin qu’un changement de thème ou d’apparence soit suivi sur place, sans reconstruire le composant. `ui.ColorsOf(cx)` nomme les rôles utilisés par `ui`, `ui.TintsOf(cx)` fournit leurs remplissages translucides, et `ui.FinOf(cx)` fournit les finitions en couches d’`ui`.

```go
colors := ui.ColorsOf(cx)
core.Text{Content: "Hello", StyleFunc: func() core.Style {
	return core.Style{Color: colors.Text(), FontSize: 15}
}}
```

`StyleFunc` s’exécute de nouveau lorsque ce qu’elle a lu change, de sorte que `cx.Dark()` et `cx.Palette()` à l’intérieur suivent le thème. Il en va de même pour `ClassFunc`, la `StyleFunc` d’un segment et les champs `StyleFunc` d’un composant.

## Curseurs

`Style.Cursor` définit un curseur de pointeur nommé (`core.CursorText`, `core.CursorColResize`, etc.) ; laissez les éléments pressables sur `core.CursorAuto` afin qu’ils conservent la flèche de la plateforme. Pour les outils de dessin, les jeux ou les pointeurs de canevas personnalisés sur ordinateur et sur le web, `Style.CursorImage` accepte un `*core.CustomCursor` avec une `Image` 1x, une `Image2x` HiDPI facultative (choisie automatiquement lorsque `PixelRatio >= 1.5`), un `Hotspot` en points logiques borné aux limites de l’image, et un curseur nommé `Fallback`. Vérifiez `cx.Env().SupportsCustomCursor()` pour savoir si la cible actuelle affiche des images de curseur personnalisées ; les plateformes tactiles les ignorent.

## Suite

- [Mise en page](layout.md) : où vont les boîtes.
- [Texte](text.md) : polices, segments et décorations.
- [Animation](animation.md) : transitions entre styles.

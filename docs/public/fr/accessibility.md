---
title: Accessibilité
description: Libellés, rôles, valeurs, actions et focus pour les lecteurs d'écran et les claviers.
group: Concepts clés
order: 61
---

## Ce que lit un lecteur d'écran

Le `Label` d'une vue est ce qu'énonce la technologie d'assistance, `AccessibilityDescription` ajoute des précisions lues après le libellé, `Hint` décrit l'effet de son activation, et `Role` indique sa nature : `core.RoleButton`, `RoleLink`, `RoleHeader`, `RoleImage`, `RoleCheckbox`, `RoleSwitch`, `RoleRadio`, `RoleTab`, `RoleTextField`, `RoleSearchField`, `RoleStatus`, `RoleMenu`, `RoleMenuItem`, `RoleDialog`, `RoleAlertDialog`, `RoleAlert`, `RoleAdjustable`, `RoleProgressBar`, `RoleTable` et les autres. Un statut est par défaut une zone dynamique polie (*polite live region*) ; un champ de recherche est un champ texte annoncé comme zone de recherche. Une boîte de dialogue est lue avec son rôle et son libellé dès son ouverture, suivis de sa description, comme le font le titre et la description d'`ui.Dialog`. Un texte sans libellé est lu d'après son contenu. `LabelFunc`, `HintFunc` et `AccessibilityDescriptionFunc` lient des textes d'accessibilité dynamiques sur la même vue, afin que les contrôles conservent leur focus et leur état lors d'un changement de langue. Tous les éléments de base dotés d'un libellé acceptent `LabelFunc`, et `View` comme `Text` acceptent `HintFunc`.

Utilisez `TextFunc` pour un texte dynamique sur `ui.Button`, `ui.Checkbox`, `ui.MenuItem` et `ui.AlertAction`. Leurs noms accessibles suivent ce texte. Les boutons et les cases à cocher acceptent également un `LabelFunc` distinct. Les curseurs, onglets, interrupteurs, barres de progression et déclencheurs de fenêtres contextuelles acceptent `LabelFunc` pour un nom accessible dynamique. `ui.Option.LabelFunc` fournit le texte de l'option dans les onglets, listes de sélection (*select*) et boîtes combinées (*combobox*). Les champs numériques et de code, les anneaux de progression et les deux variantes d'indicateur de chargement (*spinner*) acceptent aussi `LabelFunc`.

```go
core.View{
	Role:  core.RoleButton,
	Label: "Add to cart",
	Hint:  "Adds the item and opens the cart",
	OnPress: add,
	Children: []core.Node{core.Icon{SVG: lucide.ShoppingCart}},
}
```

`AccessibilityValue` fournit une plage ou une valeur textuelle : le minimum, le maximum et la valeur actuelle d'un curseur, ou un texte tel que « 3 articles ». `Accessible` fait d'une vue un élément unique, de sorte que le nom et le prix d'une carte soient lus ensemble plutôt qu'en deux arrêts, et sans libellé, la vue lit le texte de ses descendants. `AccessibilityElementsHidden` masque les éléments décoratifs, comme un chevron ou un séparateur, tandis que `ImportantForAccessibility` masque tout un sous-arbre.

```go
core.View{Accessible: true, Label: "Apples, $1.20 a kilo", Children: []core.Node{
	core.Text{Content: "Apples"},
	core.Text{Content: "$1.20 a kilo"},
}}
```

## Libellés visibles

Attribuez à un `core.Text` ou un `core.View` la `Ref` du contrôle via `LabelFor`. Appuyer sur le libellé place le focus sur le contrôle et déclenche son action d'appui, comme basculer une case à cocher. Son texte visible actuel nomme le contrôle sans répéter une chaîne `Label`, et il suit les états `Disabled` et `Invalid` du contrôle pour l'accessibilité et les styles. Un libellé n'acquiert ni rôle de bouton ni arrêt de tabulation.

```go
field := new(core.Ref)
return core.View{Children: []core.Node{
	core.Text{Content: "Email", LabelFor: field},
	base.Input{Ref: field},
}}
```

`LabelledBy: []*core.Ref{label, requirement}` nomme un contrôle à partir de plusieurs éléments visibles dans l'ordre donné, avant `Label` ; il ne transmet pas les appuis. Sinon, `Label` prévaut sur les noms implicites issus de `LabelFor`, dont l'ordre suit l'arbre. Les mises à jour de texte, déplacements, masquages, suppressions et remontages actualisent la relation. Les références manquantes, répétées, cycliques et masquées sont ignorées ; les associations restent à l'intérieur d'une même surface et d'une même portée de confinement. Un libellé ne peut pas activer un contrôle situé derrière une fenêtre modale.

Les boutons, cases à cocher, boutons radio, interrupteurs, listes de sélection et champs numériques de `base` et `ui` transmettent `Ref` et `LabelledBy` à leur contrôle ; les curseurs exposent `ThumbRef` et `StartThumbRef`. En TSX, utilisez `labelFor={field()}` et `labelledBy={[label()]}` avec des signaux de nœud renseignés par `ref={node => field.set(node)}`, afin qu'un libellé placé avant son champ se mette à jour lors du montage de ce champ.

## Actions

`AccessibilityActions` répertorie ce que le lecteur d'écran propose sur un élément : les actions standard (`core.ActionActivate`, `ActionIncrement`, `ActionDecrement`, `ActionEscape`, `ActionMagicTap`) ainsi que celles propres à l'application, chacune avec un `Name` et un `Label`. `OnAccessibilityAction` reçoit le nom. Utilisez `AccessibilityActionsFunc` sur `core.View` ou `core.Input` pour modifier les actions disponibles ou traduire leurs libellés tant que l'élément reste monté.

```go
core.View{
	Role: core.RoleAdjustable,
	Label: "Quantity",
	AccessibilityValue: func() core.AccessibilityValue {
		return core.AccessibilityValue{Max: 10, Now: float32(n.Get()), Text: fmt.Sprintf("%d items", n.Get())}
	},
	AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}},
	OnAccessibilityAction: func(name string) {
		if name == core.ActionIncrement {
			n.Update(func(v int) int { return v + 1 })
		} else {
			n.Update(func(v int) int { return v - 1 })
		}
	},
	Children: quantityControls,
}
```

`MultiSelectable` marque une grille, une zone de liste (*listbox*) ou une arborescence qui autorise plusieurs éléments sélectionnés. `base.DataTable` l'active pour la sélection multiligne ; le web, Windows et Linux exposent l'indicateur d'accessibilité correspondant.

`Expanded` indique si une section dépliable ou un bouton de menu est ouvert, `Busy` que du contenu est en cours de chargement, `Invalid` que l'application a rejeté le contenu d'un champ ou la valeur d'un contrôle (stylisé avec `aria-invalid:`), et `Checked`, `Selected` et `Pressed` signalent l'état d'un contrôle. Les parties d'un contrôle peuvent recevoir son état via `PartStates`, de sorte qu'un indicateur suive sa case à cocher sans constituer un arrêt distinct.

`SelectionItem` marque une vue comme élément d'une collection sélectionnable, indépendamment de sa valeur actuelle `Selected`. `base.DataTable` l'applique aux lignes ou aux cellules lorsque cette table les sélectionne, et `base.Tree` l'applique à chaque ligne ; Windows conserve le modèle SelectionItem et le web écrit `aria-selected="true"` ou `"false"`. Une table sans sélection laisse les lignes sans aucun de ces deux marqueurs.

## Changements dynamiques

`LiveRegion` fait annoncer par les lecteurs d'écran le libellé d'une vue et le texte qu'elle contient à mesure qu'ils changent : `core.LiveRegionPolite` attend que l'utilisateur soit inactif, `LiveRegionAssertive` interrompt la lecture. `cx.Announce` prononce un message une seule fois.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("%d items in the basket", count.Get()) },
	LiveRegion: core.LiveRegionPolite}
```

## Focus et clavier

Les infobulles (*tooltips*) décrivent leur déclencheur avant qu'il ne reçoive le focus. `View.Tooltip`, `base.Tooltip` et `ui.Tooltip` utilisent leur texte actuel comme description d'accessibilité du contrôle, sauf s'il répète le nom du contrôle, si l'application fournit `AccessibilityDescription` ou si l'infobulle est `Disabled`. Un conteneur enveloppant transmet la description à ses contrôles. La bulle et ses enfants sont masqués aux lecteurs d'écran et n'annoncent jamais d'alerte ; l'infobulle par défaut d'`ui.IconButton` ne répète donc pas son libellé.

Un `base.DataTable` expose sa grille nommée comme arrêt de focus clavier et d'accessibilité ; la couche de détection du pointeur est masquée aux technologies d'assistance.

Sur ordinateur, sur le web et sur Apple TV, les vues qui acceptent les appuis sont des arrêts de tabulation au même titre que les champs texte ; `NoFocus` en exclut une et `Focusable` transforme une vue sans appui en arrêt. `Style.FocusVisible` dessine l'anneau de focus uniquement lorsque le clavier a déplacé le focus (`focus-visible:`), `Style.Focus` pour tout focus et `Style.FocusWithin` lorsque le focus se trouve à l'intérieur d'une vue ; `OnFocus` et `OnBlur` l'écoutent, `OnBlur` également lorsque la vue focalisée quitte l'arbre, et `Ref.Focus()` et `Ref.SetAccessibilityFocus()` le déplacent ; `core.FocusedRef(cx)` est une référence vers la vue qui le détient, afin que ce qui s'ouvre sans déclencheur, comme une barre de recherche activée par un raccourci clavier, puisse le restituer à sa fermeture. Entrée et Espace appuient sur la vue focalisée, et les touches fléchées naviguent dans les listes d'onglets, groupes de boutons radio et menus. Le `Tooltip` d'une vue s'affiche aussitôt lorsque le clavier lui donne le focus, jusqu'à ce que le focus parte ou que la touche Échap soit pressée. Le `OnOpen` d'une fenêtre contextuelle s'exécute une fois son contenu présent, de sorte qu'un `Focus` à cet endroit déplace le focus à l'intérieur, comme doit le faire une boîte de dialogue non modale ; le contenu d'une fenêtre contextuelle (*popover*) suit son déclencheur dans l'ordre de tabulation, et le focus revient au déclencheur à la fermeture.

Sur Android, iPhone et iPad, `Ref.Focus()` et le paramètre `AutoFocus` d'un bouton peuvent donner le focus à une vue activable ou `Focusable` pour un clavier matériel. Toucher un contrôle ne déplace pas ce focus. Espace et Entrée activent le contrôle focalisé après la table de raccourcis (*keymap*), en préservant Contrôle, Alt, Maj et la touche plateforme dans chaque phase d'appui. `NoFocus`, les contrôles désactivés ou masqués et ceux situés derrière une fenêtre modale ne peuvent pas recevoir ce focus. Donner le focus à une vue n'ouvre aucun clavier logiciel ; les champs natifs conservent leur comportement d'édition. watchOS et les mini-programmes n'ont pas de chemin d'activation par clavier physique.

`cx.Env().ScreenReader()` indique si un lecteur d'écran est actif, `cx.Env().ReduceMotion()` si l'utilisateur a demandé moins d'animations, `cx.Env().IncreaseContrast()` s'il a demandé plus de contraste (Augmenter le contraste sur macOS, iOS et watchOS, un thème de contraste Windows, Contraste élevé de GNOME, le réglage de contraste de GTK 4.20 ou un thème GTK à fort contraste, le réglage de contraste au-dessus de standard sous Android 14, `prefers-contrast: more` ou `forced-colors: active` sur le web), `cx.Env().FontScale()` la taille du texte et `cx.Env().RTL()` le sens de mise en page. Les textes suivent la taille du texte sauf si `NoFontScaling` est défini, et une mise en page de droite à gauche met en miroir ses bords physiques lorsqu'elle utilise les bords logiques.

## Suite

- [Styles et thèmes](styles.md) : états focus et désactivé.
- [Formulaires](forms.md) : libellés et valeurs pour les champs.
- [Canevas et shaders](canvas.md) : exposer les parties d'un dessin.

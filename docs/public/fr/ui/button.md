---
title: Button
description: Un bouton-poussoir, ses variantes et tailles, ainsi que le bouton icône.
group: Composants
order: 31
---

`ui.Button` est un bouton-poussoir dans l’apparence d’`ui`, doté des variantes et des tailles dont une fenêtre a besoin. `OnPress` s’exécute lors d’un appui complet.

```go preview=button/basic
ui.Button{Text: "Save", OnPress: save}
ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: cancel}
```

## Variantes et tailles

`ui.Button.Variant` vaut `ButtonPrimary` (par défaut), `ButtonSecondary`, `ButtonOutline`, `ButtonGhost`, `ButtonTinted` ou `ButtonDanger`. Les variantes teintée et danger utilisent la couleur d’accentuation ou de danger du thème sur un remplissage doux. Sur les interfaces de bureau et de téléviseur, `Size` vaut `SizeDefault` (28 points), `SizeSmall` (24) ou `SizeLarge` (32). Sur les interfaces tactiles, ces mêmes tailles valent 44, 40 et 48 points, afin que le bouton puisse être utilisé sans avoir à le restyliser.

```go preview=button/variants
ui.Button{Text: "Delete", Variant: ui.ButtonOutline, OnPress: remove}
ui.Button{Text: "Continue", Size: ui.SizeLarge, OnPress: next}
ui.Button{Text: "Save", Icon: lucide.Check, Flex: 1, OnPress: save}
```

`Color` remplit le bouton avec une couleur définie par l’application plutôt que celle d’une variante, et `Tint` teinte son icône. `Flex` lui attribue une part d’une ligne, tandis que `Style` gère sa mise en page : il peut faire en sorte que le bouton remplisse son conteneur, définir une taille propre à l’application, ajouter des marges ou arrondir ses coins ; `TestID` le nomme pour les tests.

```go
ui.Button{Text: "Continue", Size: ui.SizeLarge,
    Style: core.Style{Width: core.Pct(100), Height: core.Pt(50)}, OnPress: next}
```

Un bouton composé uniquement d’une icône est `ui.IconButton`, avec `Variant` valant `IconButtonGhost`, `IconButtonRaised` ou `IconButtonFilled`, un cercle plat dans la couleur d’encastrement du thème ou `Fill`, une `Size` en points et un `Label` pour les technologies d’assistance. Sur ordinateur, le libellé s’affiche également en infobulle lorsque le pointeur reste sur le bouton ; `Tooltip` permet d’y afficher autre chose, et une fonction retournant `""` n’en affiche aucune. Un `ui.Button` doté d’`Icon` sans `Text` est également carré et nécessite un `Label`.

```go preview=button/icon
ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost, OnPress: remove}
```

## Contenu et états

`ui.Button` affiche `Text`, une `Icon` placée au début en option, ou les deux. Utilisez `TextStyle` pour personnaliser la taille de police, la graisse ou d’autres styles de texte du libellé.

```go
ui.Button{Text: "Saving…", Disabled: saving.Get, OnPress: save}
```

`Disabled` accepte une fonction, afin que le bouton suive un signal : un bouton désactivé ne reçoit ni les appuis ni le focus clavier, et son apparence correspond à l’état désactivé, et non à une couleur choisie par l’application.

`Busy` sert lorsqu’une action est encore en cours. Tant qu’il retourne vrai, un indicateur de chargement de la couleur du texte du bouton s’affiche par-dessus le contenu — il reste ainsi visible sur la couleur d’accentuation d’un bouton principal —, les appuis n’ont aucun effet et les technologies d’assistance savent que le bouton est occupé. Par défaut, le contenu conserve son espace sous l’indicateur. Le bouton garde le focus clavier. `BusyLabel` ou `BusyLabelFunc` affiche un message à côté de l’indicateur et le déclare comme nom accessible du bouton tant qu’il est occupé. Le bouton conserve au moins sa largeur au repos.

```go preview=button/busy
ui.Button{Text: "Save", Busy: saving.Get, BusyLabel: "Saving…", OnPress: save}
```

`ui.Spinner` et `ui.ProgressRing` acceptent chacun leur propre `Tint` pour toute autre surface sur laquelle leur couleur d’accentuation disparaîtrait.

## Liens

`ui.Link{Text: "Docs", Href: "/docs"}` ouvre une destination avec le rôle de lien. Son texte et le soulignement au survol utilisent la couleur d’accentuation, sauf si `Style.Color` définit une autre couleur ; `Style.Hover.Color` définit la couleur au survol.

## Accessibilité

Le bouton signale `RoleButton` et son texte comme nom. `Label` remplace le texte pour les technologies d’assistance. Un bouton composé uniquement d’une icône sans `Label` est illisible : nommez-le.

Entrée et Espace appuient sur un bouton qui a le focus sur ordinateur, sur le web et sur Apple TV ; le lecteur d’écran d’un téléphone l’active comme un clic.

## Plateforme

Le bouton est une vue dessinée de la même manière partout, et non un bouton natif de la plateforme ; les appuis, le focus et l’accessibilité proviennent de la plateforme. Les interfaces tactiles utilisent des cibles de mise en page plus grandes, comme décrit ci-dessus.

## Voir aussi

- [Formulaires](../forms.md) : les boutons dans un formulaire et la validation.
- [Styles et thèmes](../styles.md) : le thème dont un bouton tire ses couleurs.
- [Accessibilité](../accessibility.md) : rôles, libellés et focus.
- La page Button de la galerie.
- [Composants sans style](../authoring/base.md#button) : `base.Button`, pour un bouton avec votre propre apparence.

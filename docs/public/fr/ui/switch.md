---
title: Interrupteur
description: Un réglage activé ou désactivé, en trois tailles.
group: Composants
order: 35
---

`ui.Switch` est un réglage qui prend effet immédiatement. Il ne détient aucune valeur : l’application en est propriétaire et la met à jour dans `OnChange`.

```go preview=switch/basic
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}
```

## Tailles

`ui.Switch` accepte `Checked`, `OnChange`, `Label` et `Size` (`SizeDefault`, `SizeSmall` ou `SizeLarge`). Le bouton coulisse et la piste change de couleur au rythme de l’horloge d’affichage de la plateforme.

```go preview=switch/small
ui.Switch{Checked: sync.Get, OnChange: sync.Set, Label: "Sync automatically", Size: ui.SizeSmall}
```

Un interrupteur sert à un réglage qui s’applique immédiatement. Pour un choix dans un formulaire, utilisez une [case à cocher](checkbox.md) ; pour une option parmi plusieurs, utilisez des `ui.Radio` dans un `ui.RadioGroup` ou une [liste de sélection](select.md).

## Accessibilité

L’interrupteur signale `RoleSwitch` ainsi que son état activé ou désactivé. `Label` est ce que lisent les technologies d’assistance, et le texte à côté d’un interrupteur constitue généralement son libellé. La touche Espace ou un appui le fait basculer lorsqu’il a le focus clavier.

## Plateforme

Le contrôle par défaut est dessiné par l’application, de sorte qu’il ait la même apparence partout. `Variant: ui.SwitchSystem` demande le contrôle de la plateforme : NSSwitch sur macOS, UISwitch sur iOS et l’interrupteur Material 3 de 52 × 32 dp sur Android. Les autres cibles utilisent l’apparence par défaut. Les appuis, le focus et le lecteur d’écran relèvent de la plateforme. L’interrupteur par défaut d’un téléphone a volontairement la même taille que celui d’un ordinateur de bureau ; `SizeSmall` est destiné aux rangées denses, et non à un appareil particulier.

## Voir aussi

- [Case à cocher](checkbox.md) : un ou plusieurs choix.
- [Formulaires](../forms.md) : les réglages dans un formulaire.
- [Accessibilité](../accessibility.md) : libellés et focus.
- La page Switch de la galerie.
- [Composants sans style](../authoring/base.md#switch) : `base.Switch`, pour un interrupteur avec votre propre apparence.

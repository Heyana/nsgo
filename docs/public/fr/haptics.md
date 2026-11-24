---
title: Retour haptique
description: Déclencher un retour haptique prédéfini ou une séquence temporisée d’impulsions.
group: Fonctionnalités plateforme
order: 46
---

## Déclencher un retour

`mobile.PlayHaptic` déclenche un retour sous le doigt de l’utilisateur sur iOS et Android, et n’a aucun effet ailleurs ; associez-le donc à ce que l’utilisateur voit. `HapticSelection` signale un changement de valeur, le cran d’un sélecteur, d’un segment ou d’un curseur ; `HapticLight` à `HapticRigid` signalent la collision d’éléments, selon leur poids et leur élasticité ; et `HapticSuccess`, `HapticWarning` et `HapticError` marquent la fin d’une tâche selon son issue.

```go
mobile.PlayHaptic(cx, mobile.HapticSelection)
```

## Jouer un motif

`cx.Env().CanPlayHapticPattern()` indique si le retour haptique temporisé est disponible. Un motif contient des impulsions dotées d’un instant de départ `At` relatif au motif. Une `Duration` nulle produit une impulsion brève (*transient*), et une durée positive maintient l’impulsion jusqu’à 30 secondes. `Intensity` et `Sharpness` varient de 0 à 1. Un nouveau motif remplace le précédent. Passez un motif vide pour arrêter la lecture. Le motif s’arrête lorsque l’application passe en arrière-plan.

```go
mobile.PlayHapticPattern(cx, mobile.HapticPattern{
    {Intensity: 0.25, Sharpness: 0.4},
    {At: 100 * time.Millisecond, Intensity: 0.55, Sharpness: 0.6},
    {At: 200 * time.Millisecond, Intensity: 1, Sharpness: 0.8},
})
```

iOS restitue les événements via Core Haptics sur le matériel compatible. Android suit la temporisation et utilise l’intensité lorsque le vibreur prend en charge le contrôle d’amplitude. Les navigateurs Android équipés de `navigator.vibrate` suivent les temps d’activation et de pause après une interaction utilisateur. Android et le web jouent une impulsion brève comme une impulsion de 20 ms, et la netteté (*sharpness*) est disponible sur iOS. Sur une plateforme non prise en charge, l’appel est sans effet.

Dans les tests, définissez `HapticPatterns: true` via `nsgotest.Environment` et lisez `app.HapticPatterns`. L’historique inclut une entrée nil lorsque l’application annule la lecture.

## Quand l’utiliser

Déclenchez-le au moment même du geste, et non lorsque la réponse arrive : une sélection lorsqu’une valeur passe, un impact lorsqu’une vue se pose, un résultat lorsqu’une tâche se termine. Jouez-en un pour un changement provoqué par l’utilisateur, et aucun pour ce que l’application a fait d’elle-même.

## Suite

- [Notifications](notifications.md) : les notifications et pastilles qui véhiculent les mêmes informations.
- [Services plateforme](services.md) : le reste de ce que propose le système.
- [Accessibilité](accessibility.md) : l’annonce qu’entend un utilisateur qui ne ressent rien.

---
title: Introduction
description: Ce qu’est NSGo, sur quoi il s’exécute et comment il fonctionne.
order: 1
---

NSGo est un framework d’interface utilisateur native multiplateforme écrit en Go. Vous écrivez une application une seule fois, en Go ou en TSX, et elle s’exécute sur iOS et ses App Clips, tvOS, watchOS, Android, macOS, Windows, Linux et le web, dessinée par les propres vues de chaque plateforme. Les applications Go s’exécutent aussi dans les mini-programmes WeChat, QQ, Alipay et Douyin, à titre expérimental : ces plateformes limitent une application à 2 Mo, ce que les applications plus volumineuses dépassent. HarmonyOS exécute les applications Go en aperçu, dessinées par ArkUI.

## Ce que vous écrivez

Une application est un paquet main Go. Ses composants retournent des éléments : vues, texte, images, champs de saisie et contrôles de `ui`. L’état réside dans des signaux, et seul ce qui lit un signal est mis à jour lorsqu’il change.

```go
func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }},
	}}
}
```

La même application peut être écrite en TSX et stylisée avec des classes Tailwind ([Applications TSX](tsx.md)).

## Comment cela fonctionne

Un moteur Go gère l’application : le graphe réactif, les styles, la mise en page flexbox et grille, ainsi que la préparation du texte. Chaque mise à jour produit un lot unique de modifications, et un backend léger l’applique aux vues natives de la plateforme. Les animations s’exécutent sur l’horloge d’images de la plateforme, de sorte que rien n’est validé à chaque image.

| Plateforme | Rendu avec |
| --- | --- |
| iOS | UIKit et CoreText |
| Android | Android Views |
| macOS | AppKit et CoreText |
| Windows | Direct2D et DirectWrite |
| Linux | GTK 4, Pango et Cairo |
| Web | WebAssembly et éléments du DOM |

Chaque cible se compile sur Mac avec Go ; iOS nécessite également Xcode. L’outil `nsgo` crée, exécute, teste et empaquette les applications.

## Statut

NSGo est encore jeune : 0.1.0 est sa première version, son API n’est pas encore stable et ce qui manque figure dans la [feuille de route](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md). Il porte la conception de [Native Shell](https://github.com/longbridge/nsh) vers Go.

## Comment lire la documentation

Chaque page répond à une question. Un exemple de code est un fragment de composant, sauf indication contraire sur la page : `cx` est le `*core.Scope` que reçoit un composant. Les applications utilisent les composants de `ui`, qui disposent d’une apparence ; `base` contient les mêmes contrôles sans apparence, pour les auteurs de composants.

- **Démarrer** : [installation](getting-started.md), puis des exemples d’applications construites étape par étape, [une application de tâches](tutorials/todo.md), [une application de notes](tutorials/notes.md) et [les dépôts GitHub populaires](tutorials/repos.md).
- **Chaîne d’outils** : [exécution et débogage](debugging.md), [tests](testing.md), [compilation et distribution](building.md) et [intégration](embedding.md).
- **Composants** : [les composants les plus utilisés](ui.md), leurs variantes, leurs états et leur accessibilité.
- **Concepts clés** : [composants et état](components.md), [mise en page](layout.md), [styles](styles.md), [texte](text.md), [listes](lists.md), [navigation](navigation.md), [formulaires](forms.md), [localisation](localization.md), [TSX](tsx.md) et le reste.
- **Fonctionnalités plateforme** : [services de la plateforme](services.md), [notifications](notifications.md), [retour haptique](haptics.md), [widgets](widgets.md), [Live Activities](live-activities.md) et [autorisations](permissions.md).
- **Écrire des composants** : [composants pour les applications d’autres personnes](authoring/index.md), sans style et stylisés.
- **Contribuer** : [compiler NSGo depuis les sources](contributing.md), s’y retrouver dans le dépôt et proposer une modification.
- La [démo en ligne](https://nsgo.dev/gallery/) : chaque composant, dans votre navigateur.

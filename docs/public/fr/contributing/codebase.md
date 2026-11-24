---
title: La base de code
description: Où vit chaque partie de NSGo, et les documents qui expliquent pourquoi.
group: Contribuer
order: 81
---

NSGo est un seul module Go. Les applications importent ses paquets par le haut ; le backend de chaque plateforme se trouve tout en bas et reçoit ce qui a changé sous forme de vues natives à créer, mettre à jour et supprimer.

## Paquets

| Paquet | Ce qu'il contient |
| --- | --- |
| `nsgo` (la racine) | `nsgo.Run` et `nsgo.NewView`, choisissant un backend via des tags de compilation |
| `core` | L'API applicative : éléments, styles, classes, thèmes, signaux et services ; mise en page et préparation du texte |
| `base` | Composants sans style, un fichier chacun, et composants complexes organisés en paquets (`base/dock`, `base/editor`) |
| `ui` | Les composants de `base` dotés d'une apparence, et des paquets tels que `ui/chat` |
| `mobile`, `desktop` | Ce qui est propre aux téléphones ou propre aux ordinateurs de bureau |
| `tsx` | Applications TSX, leur bundler et compilateur, et les modules qu'elles importent |
| `i18n`, `*/locales` | Catalogues de messages et code généré à partir d'eux |
| `nsgotest` | Tests qui agissent comme un utilisateur, en mode headless ou sur une plateforme |
| `internal/engine`, `internal/driver`, `internal/layout` | La goroutine du moteur, le protocole de montage et la mise en page |
| `internal/platform/<target>` | Un backend : vues natives, texte et saisie pour une cible |
| `cmd/nsgo` | L'outil `nsgo` |
| `examples/` | La galerie et les autres applications d'exemple |

Les paquets n'importent que vers le bas ; `internal/moduletest/layering_test.go` vérifie le graphe sur chaque cible.

## Documents de conception

[ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) cartographie les paquets, les threads et les flux d'un commit jusqu'à l'écran, et liste les sujets dans `docs/architecture/` : un par composant majeur, tel que la mise en page, le texte, le défilement, les thèmes ou l'outil. Lisez le sujet correspondant à ce que vous modifiez avant d'intervenir ; il décrit comment la partie fonctionne et où elle s'écarte de [Native Shell](https://github.com/longbridge/nsh), dont NSGo porte la conception.

[AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) rassemble les règles que suit chaque modification, pour les humains comme pour les agents de codage. Quelques-unes structurent la plupart des changements :

- Chaque API a un emplacement unique : `core` pour les éléments et l'environnement, `base` pour les contrôles, `ui` pour leur apparence, `mobile` et `desktop` pour un type d'appareil donné.
- `base` n'a pas d'apparence ; un composant `ui` est sa version `base` dotée d'une apparence.
- Rien ne change implicitement selon l'appareil : pas de contrôles plus grands sur ordinateur ni plus petits sur téléphone dans `core` ou `base`.
- Des vues natives partout, identiques sur chaque plateforme : ce qu'une plateforme possède nativement, les autres l'imitent.
- Pas de C écrit à la main, et rien d'autre que macOS, Xcode et Go pour compiler toutes les cibles, à l'exception de l'émulateur Android x86_64.

Ce qui manque, plateforme par plateforme, figure dans la [feuille de route](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md).

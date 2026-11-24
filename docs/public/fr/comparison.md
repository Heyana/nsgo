---
title: Comparaison avec d'autres frameworks
description: "Où se situe NSGo par rapport à NSH, MyGo et d'autres frameworks d'interface utilisateur, et ce à quoi il renonce."
order: 2
---

La place de NSGo parmi les frameworks d'interface utilisateur repose sur trois choix : les applications sont écrites en Go (ou en TSX par-dessus), chaque plateforme effectue le rendu avec ses propres vues natives, et une seule base de code couvre les téléphones, les ordinateurs de bureau et le web. D'autres frameworks font d'autres choix, et chaque choix a un coût.

## Aperçu

| Framework | Langage | Rendu via | Cibles |
| --- | --- | --- | --- |
| NSGo | Go, TSX | Les vues de chaque plateforme | iOS, App Clips, tvOS, watchOS, Android, macOS, Windows, Linux, web ; mini-programmes, à titre expérimental |
| [NSH](https://nsh.dev) | Rust, TSX | Les vues de chaque plateforme | iOS, Android, macOS, Windows, Linux, web, applications GPUI, terminaux ; HarmonyOS et mini-programmes WeChat via des plateformes communautaires |
| [MyGo](https://github.com/egoist/mygo) | Go, pages web | Son propre moteur de rendu GPU, ou la webview du système | macOS, Windows, Linux, web |
| [Fyne](https://fyne.io), [Gio](https://gioui.org) | Go | Leur propre moteur de rendu | Ordinateurs de bureau, téléphones, web |
| [Wails](https://wails.io) | Go, pages web | La webview du système | Ordinateurs de bureau |
| [Flutter](https://flutter.dev) | Dart | Son propre moteur de rendu | Téléphones, ordinateurs de bureau, web |
| [Compose Multiplatform](https://www.jetbrains.com/compose-multiplatform/) | Kotlin | Son propre moteur de rendu ; Compose d'Android | Téléphones, ordinateurs de bureau, web |
| [React Native](https://reactnative.dev) | JavaScript, TSX | Les vues de chaque plateforme | iOS, Android ; autres via des plateformes communautaires |
| [.NET MAUI](https://dotnet.microsoft.com/apps/maui) | C# | Les contrôles de chaque plateforme | iOS, Android, macOS, Windows |
| [Avalonia](https://avaloniaui.net) | C# | Son propre moteur de rendu | Ordinateurs de bureau, téléphones, web |
| [Qt](https://www.qt.io) | C++, QML | Son propre moteur de rendu | Ordinateurs de bureau, téléphones, systèmes embarqués |
| [Electron](https://www.electronjs.org), [Tauri](https://tauri.app) | Pages web ; Rust pour Tauri | Un Chromium embarqué ; la webview du système pour Tauri | Ordinateurs de bureau ; téléphones pour Tauri |

## Points forts

- **Vues natives.** Le texte, les méthodes de saisie, la sélection, le défilement, l'accessibilité et les menus sont ceux de la plateforme elle-même : une application se comporte donc comme les utilisateurs l'attendent sur leur système, là où un framework qui dessine sa propre interface doit réimplémenter chacun de ces éléments. Ce qu'une seule plateforme possède, comme Liquid Glass sur iOS, les autres l'imitent de la même manière au lieu de l'ignorer.
- **Mises à jour à granularité fine.** Les composants ne s'exécutent qu'une seule fois et les signaux ne mettent à jour que ce qui les lit, sans DOM virtuel ni fonction de vue réévaluée à chaque image ; les animations s'exécutent sur l'horloge d'images de la plateforme.
- **Les paquets les plus légers et le démarrage le plus rapide.** Dans les [benchmarks](https://nsgo.dev/benchmarks/), la même application de compteur pèse environ 8 Mo une fois empaquetée sur macOS, Windows et Linux, soit moins que MyGo, GPUI et NSH (de 33 à 45 Mo pour NSH) ; elle démarre également le plus vite et consomme moins de mémoire que les frameworks qui effectuent le rendu sur le GPU, tels que MyGo et GPUI.
- **Go uniquement.** Le moteur, la mise en page et les backends sont en Go, sans C, sans cgo et sans moteur JavaScript pour les applications Go.
- **Toutes les plateformes depuis un seul Mac.** Un Mac doté de Go et de Xcode compile de manière croisée toutes les cibles : pas besoin de NDK, d'Android SDK, de JDK, de GTK ni de chaîne d'outils Windows.
- **Tous les fichiers de publication en une seule commande.** `nsgo build` génère une image disque signée et notariée pour macOS, un installateur Windows signé, des paquets deb et Arch ainsi qu'une AppImage, des paquets pour le Mac App Store, le Microsoft Store, Google Play et Flathub, et des scripts d'installation depuis les releases GitHub ([compilation et distribution](building.md)).
- **Mises à jour natives de chaque plateforme.** Sur macOS Sparkle, et sur Windows WinSparkle, téléchargent, vérifient et installent la version suivante ; sur Linux et Android, l'application détecte la nouvelle version et ouvre sa page de publication ; sur iOS, l'App Store s'en charge. Une application installée via un store reste gérée par ce store ([mises à jour](building.md#mises-à-jour)).
- **Toutes les plateformes.** Téléphones, tablettes, téléviseurs, montres, ordinateurs de bureau et le web partagent un seul moteur et une seule API, ainsi que les iOS App Clips, les widgets iPhone et Mac et, à titre expérimental, les mini-programmes WeChat, QQ, Alipay et Douyin.
- **Intégration.** Une application NSGo peut s'insérer en tant que vue dans une application iOS, Android, Go desktop ou web existante, pour migrer écran par écran ([intégration](embedding.md)).
- **Une boucle de développement rapide.** L'enregistrement d'un fichier Go recompile et redémarre l'application ; une modification TSX met à jour l'application en cours d'exécution sur place tout en conservant son état ; les erreurs de compilation et les panics lors du rendu s'affichent par-dessus l'application ([exécution et débogage](debugging.md)).
- **Une seule suite de tests partout.** Les tests trouvent et utilisent les contrôles par leur texte ou leur rôle comme le font les utilisateurs, rapidement sans fenêtre, ou à l'identique dans de vraies fenêtres sur chaque cible avec les entrées natives de la plateforme, en enregistrant des captures d'écran ([tests](testing.md)).
- **Localisation intégrée.** Les libellés d'une application et de ses composants vivent dans des catalogues, et les pseudo-locales détectent ce qui n'est pas traduit, ce qui est tronqué ou ce qui n'est pas prêt pour l'écriture de droite à gauche ([localisation](localization.md)).
- **Shaders en Go.** `core.Shader` exécute des shaders GPU écrits en syntaxe Go ou en WGSL, sur toutes les plateformes ([canevas](canvas.md#shaders)).
- **TSX à la demande.** Les écrans peuvent être écrits en TSX avec des classes Tailwind, sur le même moteur que les composants Go.

## Points faibles

- **Jeune.** 0.1.0 est la première version et l'API n'est pas encore stabilisée ; la [feuille de route](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md) liste ce qui manque, comme une limite de tas pour le TSX livré à l'exécution.
- **Un écosystème réduit.** Peu de composants tiers, d'exemples ou de réponses en ligne comparé à Flutter, React Native ou Qt.
- **Des différences entre plateformes.** Qui dit vues natives dit légères différences d'une plateforme à l'autre : une application doit donc être vérifiée sur chacune. Un framework qui dessine sa propre interface a exactement le même aspect partout.
- **Ses propres ponts.** Sur Android, le programme Go s'exécute à côté du runtime Java et communique avec lui via des pipes ; sur Linux, les utilisateurs ont besoin de GTK 4.14 ou plus récent.
- **Le TSX est interprété.** Le TSX s'exécute dans goja, un interpréteur JavaScript écrit en Go, plus lent que les moteurs utilisés par React Native ou Electron.

## NSH

NSGo porte la conception de NSH : l'architecture, les backends, les noms de l'API et les composants sans style de `base` proviennent de NSH ([état du projet](index.md#statut)).

- **Choisissez NSGo** pour écrire des applications en Go plutôt qu'en Rust ou en TSX, pour compiler toutes les cibles depuis un Mac avec Go et Xcode, ou pour ce que NSGo ajoute : des shaders GPU écrits en Go, des icônes vectorielles, un terminal, des applications ouvrant leurs propres fenêtres, panneaux et icônes de barre d'état, ainsi que les composants stylés de `ui`.
- **Choisissez NSH** pour un projet plus mature, du TSX exécuté par QuickJS avec bytecode et limites de tas, le backend GPUI ou la prise en charge d'Android 32 bits.

## MyGo

Tous deux proposent des interfaces utilisateur écrites en Go sans cgo, mais MyGo dessine lui-même son interface native sur le GPU et la reconstruit depuis une fonction de vue à chaque image, là où NSGo ne transmet aux vues natives que ce qui a changé.

- **Choisissez MyGo** pour des applications de bureau d'apparence identique sur chaque système, des fenêtres basées sur des pages web avec appels typés vers Go, ainsi que ses API et outils de distribution desktop : icônes de zone de notification, associations de fichiers, installateurs, notarisation et mises à jour différentielles signées.
- **Choisissez NSGo** pour cibler aussi les téléphones, utiliser la gestion du texte, la saisie et l'accessibilité propres à chaque plateforme, ou gérer l'état via des signaux plutôt que par redessin complet depuis les données de l'application.

## Autres toolkits Go

- **Fyne et Gio** dessinent tout eux-mêmes, comme le fait MyGo, et tous deux ciblent les téléphones et le web. Fyne offre un jeu complet de widgets avec sa propre apparence ; Gio est en mode immédiat, bas niveau et flexible. Tous deux ont des versions publiées et des utilisateurs ; aucun des deux n'a l'aspect ni le comportement d'une application native.
- **Wails** place un frontend web dans la webview du système avec Go derrière : n'importe quelle bibliothèque web fonctionne, mais l'interface reste une page web, uniquement sur ordinateur de bureau.

## Frameworks dans d'autres langages

- **Flutter et Compose Multiplatform** dessinent chaque pixel eux-mêmes : identiques sur toutes les plateformes et matures, avec de vastes écosystèmes, mais avec un texte, un défilement et une saisie réimplémentés plutôt que natifs, et du Dart ou du Kotlin à apprendre.
- **React Native** partage le modèle de NSGo d'un cœur réactif pilotant des vues natives, et dispose du plus grand écosystème de sa catégorie. Il se concentre sur iOS et Android et exécute la logique applicative en JavaScript ; NSGo l'exécute en Go et traite les ordinateurs de bureau et le web comme des cibles de premier plan.
- **.NET MAUI** encapsule les contrôles de chaque plateforme en C#, sans Linux ni le web. **Avalonia** dessine lui-même son interface, proche de WPF, sur tous les ordinateurs de bureau, les téléphones et le web.
- **Qt** est mature, rapide et atteint les systèmes embarqués, mais dessine ses propres widgets, exige du C++ ou du QML, et est distribué sous licence LGPL ou commerciale.
- **Electron et Tauri** affichent des pages web : toute la panoplie d'outils et de bibliothèques du web, au prix de la consommation mémoire d'un navigateur, surtout pour Electron, et d'une interface qui se ressent comme une page web.

## Choisir

Choisissez NSGo pour une application qui doit sembler native sur téléphones, ordinateurs de bureau et le web depuis une seule base de code Go, lorsqu'un projet encore jeune est acceptable. Pour une application qui doit être livrée rapidement sur une API stabilisée, optez pour un framework mature du tableau ci-dessus : Flutter ou React Native pour les téléphones, MyGo, Wails ou Tauri pour les ordinateurs de bureau uniquement.

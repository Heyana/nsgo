---
title: Installation
description: Installer ce dont NSGo a besoin, créer une application et l’exécuter sur chaque cible.
group: Démarrer
order: 10
---

## Prérequis

- [Go](https://go.dev/dl/) 1.27.3 ou plus récent.
- Pour compiler pour iOS, tvOS et watchOS, un Mac avec Xcode. Toutes les autres cibles se compilent depuis macOS avec Go seul ; celle de l’émulateur Android x86_64 nécessite également un Android NDK.
- Pour exécuter sur un iPhone, un identifiant Apple dans Accounts de Xcode et l’iPhone approuvé, avec le mode développeur activé.
- Pour exécuter sur Android, `adb` issu des platform-tools du SDK Android.
- Pour exécuter sur Windows, Go seul, qui compile également les paquets Windows et web.
- Sous Linux, GTK 4.14 ou plus récent, Pango et Cairo. Les images réseau utilisent libsoup 3, les vues web utilisent WebKitGTK 6.0, et l’audio et la vidéo utilisent les greffons GStreamer. Sur les systèmes appliquant des restrictions AppArmor sur les espaces de noms utilisateur, [WebKitGTK nécessite un profil bubblewrap](debugging.md#linux-web-views).

## Créer une application

Créez un module, ajoutez NSGo et son outil, puis laissez l’outil créer une application :

```sh
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
```

`go get -tool` ajoute `nsgo` au module une seule fois, afin que `go tool nsgo` l’exécute sans rien installer. `init` écrit :

| Fichier | Rôle |
| --- | --- |
| `app.go` | L’application, un compteur : un composant `Root` et `main`, qui appelle `nsgo.Run(Root)` |
| `app_test.go` | Un test qui appuie sur le bouton |
| `nsgo.yaml` | Le nom, l’identifiant, la version et les cibles de l’application |

`go tool nsgo init -template tsx` écrit à la place une application TSX, et `-template mixed` une application Go dont le point d’entrée TSX utilise ses propres composants Go.

### Modèles issus de modules Go

Passez le chemin du module du modèle avec une version facultative. Une version omise est résolue vers la version la plus récente trouvée par le proxy de modules Go.

```sh
go tool nsgo init -template example.com/my-template@v1.2.0 -module example.com/hello hello
```

Publiez le modèle de départ dans le répertoire `template/` du module. Incluez `nsgo.yaml` et `main.go`. L’outil effectue le rendu des deux, ainsi que de tout fichier se terminant par `.tmpl`, avec `text/template` de Go, et retire `.tmpl` des noms générés. Il copie les autres fichiers, y compris les ressources binaires, sans les modifier.

Les modèles reçoivent `.Name` (le nom d’affichage de l’application), `.ID` (l’identifiant généré de l’application), `.Slug` (le nom en minuscules avec tirets) et `.Module` (la valeur de `-module`, ou le slug). Fournissez un fichier `go.mod.tmpl` pour déclarer le module et les dépendances de l’application générée, car Go exclut les fichiers `go.mod` imbriqués lors du téléchargement de modules. À défaut, `-module` écrit le même `go.mod` minimal que pour un modèle intégré.

## L’exécuter

```sh
go tool nsgo dev
```

`dev` compile l’application pour cette machine et l’ouvre. À chaque enregistrement, il recompile et redémarre l’application, et une modification du TSX seul met à jour l’application en cours d’exécution sur place. Une compilation échouée s’affiche par-dessus l’application, avec des boutons pour copier l’erreur et relancer la compilation.

Exécutez-la ailleurs avec `-target` :

```sh
go tool nsgo dev -target web                # dans le navigateur
go tool nsgo dev -target ios_simulator      # dans le simulateur iOS
go tool nsgo dev -target tvos_simulator     # dans le simulateur Apple TV
go tool nsgo dev -target watchos_simulator  # dans le simulateur Apple Watch
go tool nsgo dev -target android            # sur un appareil connecté ou un émulateur
```

`go tool nsgo doctor` répertorie chaque cible en indiquant si cette machine peut la compiler et l’exécuter, et ce qui lui manque pour celles qu’elle ne peut pas traiter, comme Xcode, `adb`, un appareil connecté ou GTK.

[Exécution et débogage](debugging.md) couvre les appareils, le suivi des mises à jour, le moniteur de performances et les autres langues.

## Avec un agent de codage

La compétence (*skill*) de NSGo enseigne aux agents de codage, tels que Claude Code, Codex et Cursor, comment une application NSGo est écrite, testée et livrée. Ajoutez-la au répertoire de l’application, ou avec `-g` pour tous les projets :

```sh
bunx skills add nsgo-dev/nsgo
```

Les agents qui lisent eux-mêmes la documentation partent de <https://nsgo.dev/llms.txt>, qui répertorie chaque page en Markdown à l’adresse `https://nsgo.dev/docs/<page>.md` ; <https://nsgo.dev/llms-full.txt> les regroupe toutes. Un agent peut également utiliser l’application en cours d’exécution ([Exécution et débogage](debugging.md#agents)).

## Suite

- [Exemple : une application de tâches](tutorials/todo.md) : état, liste et test, en une dizaine de minutes.
- [Exemple : une application de notes](tutorials/notes.md) : une pile de navigation et des notes conservées sur l’appareil.
- [Exemple : dépôts GitHub populaires](tutorials/repos.md) : chargement depuis le réseau avec gestion des erreurs et nouvel essai.

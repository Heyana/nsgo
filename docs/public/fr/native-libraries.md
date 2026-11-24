---
title: Bibliothèques natives
description: Distribuez des bibliothèques partagées précompilées avec un paquet Go via les étapes habituelles d’empaquetage, de gestion des licences et de signature de NSGo.
group: Chaîne d’outils
order: 23
---

Un paquet Go peut distribuer les bibliothèques partagées qu’il charge via purego en plaçant un fichier `nsgo-natives.yaml` dans son répertoire de paquet. Lorsqu’une application importe le paquet, NSGo sélectionne ses déclarations pour la cible, vérifie les fichiers et les inclut avant la signature et la création des installateurs. L’application ne nécessite aucune configuration ni commande d’empaquetage supplémentaire.

## Déclarer des bibliothèques

```yaml
version: 1
libraries:
  macos_arm64:
    - name: libexample.dylib
      path: lib/macos-arm64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  macos_x64:
    - name: libexample.dylib
      url: https://downloads.example.org/v1.0/macos-x64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  windows_arm64: []
  windows_x64: []
  linux_arm64: []
  linux_x64: []
```

Les clés prises en charge sont `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64` et `linux_x64`. Déclarez chaque cible sur laquelle le paquet Go est sélectionné ; `[]` indique explicitement que cette cible n’a besoin d’aucune bibliothèque embarquée. Des balises de compilation Go peuvent autrement exclure le paquet des cibles qu’il ne prend pas en charge. `macos_universal` reprend les deux entrées macOS.

Chaque bibliothèque comprend :

- `name` : son nom de fichier installé, se terminant par `.dylib` sur macOS, `.dll` sur Windows, ou `.so` ou un `.so.*` versionné sur Linux.
- Exactement l’un des deux champs : `path`, un fichier relatif à ce répertoire de paquet, ou `url`, une URL HTTP(S) pointant directement vers la bibliothèque. Publiez les bibliothèques extraites plutôt que des URL d’archives. Les URL ne doivent pas contenir de noms d’utilisateur ni de mots de passe intégrés.
- `sha256` : l’empreinte SHA-256 du fichier d’origine, obligatoire pour l’une comme pour l’autre source. Par exemple, `shasum -a 256 lib/macos-arm64/libexample.dylib` l’affiche sur macOS.
- `licenses` : des chemins non vides vers les fichiers de licence et de mention d’origine, relatifs à ce répertoire de paquet. Ces textes sont livrés dans `THIRD_PARTY_NOTICES.txt` et accessibles via `core.ThirdPartyNotices()` dans les compilations de publication (*release*). La [politique de licences](building.md#mentions-de-licences) existante de l’application, y compris les exceptions exactes de modules, s’applique indépendamment aux licences natives.

Les chemins locaux et les liens symboliques doivent rester à l’intérieur du paquet déclarant. NSGo rejette les champs inconnus, les sommes de contrôle non concordantes, les fichiers destinés à un autre système d’exploitation ou à une autre architecture, ainsi que les conflits de noms installés. Deux paquets peuvent partager un fichier portant le même nom s’il possède la même somme de contrôle. La taille d’un fichier est limitée à 1 Gio.

## Empaqueter et charger

| Cible | Répertoire des bibliothèques |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows et Linux | À côté de l’exécutable |

Le paquet charge sa bibliothèque depuis ce répertoire, en utilisant l’emplacement de l’exécutable plutôt que le répertoire de travail. Précompilez les dépendances de sorte que leurs dépendances paires se chargent depuis le répertoire dans lequel elles sont livrées. Déclarez chaque fichier pair requis ainsi que chaque nom de fichier dont le chargeur a besoin.

NSGo vérifie de nouveau les téléchargements en cache avant toute réutilisation et coordonne les compilations concurrentes à l’aide d’un verrou de fichier. Le cache de contenu se trouve sous le répertoire de cache utilisateur du système d’exploitation, dans `nsgo/natives`. Les fichiers locaux sont vérifiés lors de leur copie.

Un paquet macOS universel exige les mêmes noms de bibliothèques pour les deux architectures. NSGo assemble les tranches avec `lipo` ; une bibliothèque déjà universelle fournie par l’éditeur peut servir pour l’une ou l’autre entrée. Les bibliothèques sont signées avant l’application, puis suivent les étapes habituelles d’image disque, de signature et de notarisation de l’application. La signature Windows inclut les DLL déclarées même lorsque `build.installers: false`. Les installateurs Linux incluent les mêmes fichiers et mentions que le répertoire de l’exécutable.

Les builds de développement et les tests sur cibles de bureau natives utilisent les mêmes étapes de sélection de paquets et de copie. Les déclarations sont sélectionnées en fonction de la cible de compilation, des balises et de la superposition Go générée, y compris les remplacements locaux et les paquets intégrés dans `vendor`. Les autres cibles n’empaquettent actuellement aucune bibliothèque à partir de ce fichier.

## Préserver les fichiers lors du vendoring

L’opération `vendor` de Go a besoin de données intégrées pour conserver des fichiers sous un répertoire de paquet. Un paquet peut marquer son manifeste et ses licences pour inclusion sans utiliser leurs octets à l’exécution :

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

Incluez également les chemins de bibliothèques locales dans cette directive `embed` lorsque vous utilisez `path`, par exemple `lib/*`. Les téléchargements par URL n’ont besoin que du manifeste et des fichiers de licence dans le paquet Go.

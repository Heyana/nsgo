---
title: Environnement de développement
description: Compiler NSGo depuis les sources, exécuter la galerie et tester une modification.
group: Contribuer
order: 80
---

## Prérequis

macOS avec Xcode et Go 1.27.3 ou plus récent compile toutes les cibles pour arm64, y compris l'APK Android. L'exécution sur appareil requiert ce que liste la page [Démarrer](getting-started.md#requirements), et la cible de l'émulateur Android x86_64, `android_x64`, nécessite un Android NDK, car Go ne lie android/amd64 qu'avec cgo. L'installateur Windows nécessite NSIS (`brew install makensis`) ; sans lui, `build` laisse son script généré.

```sh
git clone https://github.com/nsgo-dev/nsgo.git
cd nsgo
```

Lisez [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) et [AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) avant de modifier le code.

## La galerie

La galerie contient une démonstration de chaque élément, composant `base` et `ui`, ainsi que de chaque service. Une fenêtre large affiche les démonstrations sous forme d'onglets dans une barre latérale ; une fenêtre étroite affiche les onglets et les réglages dans une barre inférieure, les démonstrations étant empilées par-dessus. Lancez-la depuis la racine du dépôt :

```sh
go tool nsgo dev examples/gallery                   # nsgo.yaml's dev target, exiting with the app
go tool nsgo dev examples/gallery-tsx               # the gallery's components and apps, written in TSX
go tool nsgo dev -target web examples/gallery       # in the browser, serving until interrupted
go tool nsgo dev -target ios_simulator examples/gallery
go tool nsgo dev -target ios examples/gallery       # on the connected iPhone, signed by Xcode
go tool nsgo dev -target android examples/gallery   # on the connected device or emulator, over adb
go tool nsgo build -target all examples/gallery     # every platform's packages in examples/gallery/dist, what to hand out in dist/releases
```

## Vérifier et tester

```sh
go tool nsgo check                 # vet every package
go tool nsgo check -target all     # vet every available Go/Xcode target instead
go tool nsgo test                  # every package's tests
go tool nsgo test -bench . ./core  # core's benchmarks
(cd benchmark && go run ./runner)  # measure against other frameworks
# capacity curve for NSGo's 600-cell grid, without rebuilding it
(cd benchmark && go run ./runner -frameworks nsgo -cases grid -grid-cols 20 -grid-rows 15 -out /tmp/grid.json)
```

`-target` vérifie les cibles qu'il nomme au lieu de l'hôte et, dans le dépôt de NSGo, du client de développement, que la commande `check` simple vérifie. Pour vérifier toutes les plateformes concernées par une modification, nommez-les toutes en une seule commande, par exemple `check -target web,android -jobs 2`, plutôt que d'en lancer plusieurs : `-jobs n` vérifie jusqu'à n cibles simultanément, chaque `go vet` disposant de n workers de compilation, alors que par défaut une seule cible est vérifiée à la fois avec la valeur par défaut de Go ; une vérification lancée pendant qu'une autre tourne dans le même module attend celle-ci et l'indique. Le workflow `check.yml` de l'intégration continue vérifie toutes les cibles.

Lancée à la racine du dépôt ou dans `internal/dev`, la commande sans interface graphique teste `internal/dev` une seconde fois avec le tag de compilation `nsgodev`, dont les fichiers du client de développement ont besoin, afin que ses tests s'exécutent localement et en CI. Les deux exécutions acceptent les mêmes filtres et drapeaux et partagent un seul décompte et un seul rapport JUnit, où le paquet de la seconde exécution est nommé `internal/dev [nsgodev]`.

`check -target all` vérifie les cibles macOS, Windows, Linux, Android arm64 et web sur n'importe quel hôte. Elle vérifie également iOS, tvOS et watchOS lorsque leurs SDK Xcode sont disponibles, et liste ceux qu'elle ignore. Une cible explicitement nommée, telle que `check -target ios`, échoue si ses outils sont indisponibles, y compris lorsqu'elle est combinée avec `all`. L'échec d'une cible n'empêche pas la vérification des cibles restantes, et la commande échoue si l'une des vérifications requises échoue. `build -target all` requiert toujours macOS avec Xcode. Dans le propre dépôt de NSGo, chaque cible nommée vérifie aussi ses hôtes de test de plateforme avec `nsgotest` ; la CI nomme explicitement la cible en aperçu `harmonyos`, car elle ne fait pas partie de `all`.

Le job Linux de la CI exécute les tests `js/wasm` du backend Go HarmonyOS avec Node.js et le lanceur Wasm de Go. Exécutez les tests de la galerie localement avec un émulateur HarmonyOS connecté via `hdc`. La compilation des [modèles d'hôte HarmonyOS](building.md#harmonyos) requiert le SDK OpenHarmony API 26.

```sh
go tool nsgo test -env GOOS=js -env GOARCH=wasm ./internal/platform/harmonyos \
  -- "-exec=env -i \"PATH=$PATH\" \"$(go env GOROOT)/lib/wasm/go_js_wasm_exec\"" -count=1
go tool nsgo test -target harmonyos -run TestHarmonyOSPages -timeout 10m ./examples/gallery
```

Dans une session Wayland sur un bureau Linux, exécutez le contrôle d'acceptation réel du portail de raccourcis globaux avec `NSGO_TEST_GLOBAL_SHORTCUT_PORTAL=<installed-desktop-id> go tool nsgo test -run TestGlobalShortcutPortalWithPermission ./internal/platform/linux`. L'identifiant doit posséder une entrée `.desktop`. Accordez Ctrl+Alt+K dans l'invite du compositeur, puis appuyez dessus alors qu'une autre application a le focus ; le test attend à la fois l'enregistrement et l'activation. Il est ignoré si la variable d'environnement n'est pas définie.

Avec Docker disponible sur un hôte Linux, `go tool nsgo test -run '^TestLinuxPackagesNative$' -env NSGO_TEST_LINUX_PACKAGES=1 ./cmd/nsgo -- -timeout=20m` vérifie les installations réelles dans des conteneurs Debian, Ubuntu et Arch jetables. Il télécharge leurs images officielles, y installe les dépendances GTK, vérifie chaque base de paquets ainsi que les fichiers installés, puis supprime le paquet. La base de paquets de l'hôte n'est jamais utilisée.

## Publications

NSGo publie des versions à la demande lorsqu'un projet en aval a besoin d'une version taguée ou que les mainteneurs ont des modifications prêtes à être livrées. Lorsque vous demandez une publication pour une bibliothèque, mentionnez le travail dépendant et indiquez le commit contenant la dernière modification NSGo dont il a besoin. Le premier tag de version contenant ce commit est la version que la bibliothèque pourra requérir.

Pour tester une modification fusionnée avant qu'elle ne soit taguée, exécutez `go get github.com/nsgo-dev/nsgo@main` depuis le module dépendant. Vous pouvez utiliser un identifiant de commit à la place de `main` pour sélectionner la révision vérifiée. Go enregistre la pseudo-version résultante dans `go.mod`.

Un mainteneur attribue leur version et leur date aux changements dans `CHANGELOG.md`, laisse une nouvelle section Unreleased, puis tague le commit correspondant sur `main`. Le [workflow de publication](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/release.yml) vérifie le tag et le publie après la réussite du workflow de tests ; les notes publiées quittent alors `CHANGELOG.md`, qui ne conserve que les changements non publiés, pour apparaître dans [Releases](https://github.com/nsgo-dev/nsgo/releases).

## NSH

NSGo consulte [Native Shell](https://github.com/longbridge/nsh) comme référence de conception ; aucun de ses tests n'en dépend. Pour le consulter, clonez-le à côté du dépôt et basculez sur son dernier tag de version, jamais sur `main` :

```sh
git clone https://github.com/longbridge/nsh.git ../nsh
git -C ../nsh checkout "$(git -C ../nsh describe --tags --abbrev=0)"
```

## Suite

- [La base de code](contributing/codebase.md) : où vit chaque partie de NSGo.
- [Apporter une modification](contributing/changes.md) : ce qu'une pull request doit contenir.

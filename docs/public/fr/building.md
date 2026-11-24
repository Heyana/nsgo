---
title: Compilation et distribution
description: Empaqueter une application pour chaque plateforme, créer des installateurs, la signer et la faire notariser, la publier sur GitHub et l'intégrer dans d'autres applications.
group: Chaîne d’outils
order: 22
---

## Build

```sh
go tool nsgo build
```

`build` écrit un paquet de version allégé (*stripped release package*) pour chaque cible répertoriée dans `nsgo.yaml`, dans `dist/<name>-<version>-<target>` : un `.app`, un `.ipa`, un `.apk`, les répertoires Windows et Linux, ou un site web dont la page récupère chaque module TSX nommé par un `import()` lorsque l'import s'exécute. `-target` remplace la liste, et `all` désigne chaque cible qui ne nécessite que Go et Xcode ; ajoutez `android_x64` lorsqu'un NDK Android est installé, et `miniprogram`, ou `miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` ou `miniprogram_douyin` seul, pour les [mini-programmes](#mini-programs), ainsi que `harmonyos` pour un aperçu [HarmonyOS](#harmonyos). `-run` lance le paquet compilé pour cette machine, ou l'installe sur l'iPhone, le simulateur ou l'appareil Android choisi par `-device`.

Tout ce qui est destiné à être distribué atterrit dans `dist/releases`, le seul répertoire à consulter :

| Dans `dist/releases` | Contenu |
| --- | --- |
| `*.dmg`, `*-setup.exe`, `*.deb`, `*.pkg.tar.gz`, `*.AppImage` | [Installateurs](#installers) pour macOS, Windows et Linux |
| `*.AppImage.zsync` | Contrôles de mise à jour différentielle pour AppImageUpdate et les lanceurs compatibles |
| `*-ios.ipa`, `*-tvos.ipa`, `*.apk` | Les paquets iOS, tvOS et Android, pour les boutiques ou les appareils |
| `*.pkg`, `*.msix`, `*.aab`, `*.flatpak`, `*.tar.gz` et `flatpak/` | Les [paquets de boutique](#store-packages) d'une compilation `store` |
| `install.sh`, `install.ps1` | Scripts installant l'application depuis les [publications GitHub](#publish-on-github) |
| `appcast-<target>.xml` | Les flux de [mise à jour](#updates) lus par les applications |
| `homebrew/<name>.rb`, `winget/` | [Cask Homebrew et manifestes winget](#channels) de distribution directe |
| `*-web.zip` | Le site, à décompresser et servir tel quel |
| `miniprogram/<platform>/` | Le projet de chaque mini-programme, à téléverser depuis son outil de développement |

Chaque fichier est nommé `<name>-<version>-<system>-<arch>`, tel que `hello-1.2.0-windows-x64-setup.exe`, afin que les utilisateurs trouvent celui de leur système sur une page de publication, et possède son `.sha256` à côté. Une compilation d'une autre version vide d'abord le répertoire ; des compilations de la même version, y compris sur d'autres machines, s'y ajoutent. Les paquets des simulateurs et de watchOS restent dans leurs répertoires `dist/<name>-<version>-<target>`.

Le site web inclut `.wasm.br` et `.wasm.gz` lorsqu'ils sont plus petits que le module d'origine. Activez les fichiers précompressés sur les hébergeurs qui les prennent en charge. Servez-les à l'URL `.wasm` d'origine avec `Content-Type: application/wasm`, le `Content-Encoding` correspondant et `Vary: Accept-Encoding`. Le module d'origine fonctionne sur n'importe quel hébergeur statique. [Publier sur le web](web.md) couvre les titres sémantiques, les liens navigateur, les métadonnées par page et le chargement via CDN.

### Paquets web hors ligne

Définissez `web.offline: true` dans `nsgo.yaml` pour mettre en cache une version web dès son premier lancement en ligne.

```yaml
web:
  offline: true
```

Servez le répertoire publié en HTTPS, ou utilisez `go tool nsgo build -target web -run` sur `localhost`. Le *service worker* met en cache la page, WebAssembly, les scripts et les ressources empaquetées. Les visites en ligne chargent la version publiée. Une fois l'installation réussie et les onglets de la version précédente fermés, la dernière version peut démarrer depuis son cache. Les requêtes vers les services en ligne d'une application nécessitent toujours une connexion.

## Terminal

Exécutez l'application dans un terminal interactif avec `go tool nsgo dev -target tui`. `go tool nsgo test -target tui` y exécute ses tests via `nsgotest.Main`, à raison d'une application par processus de test. Le terminal contrôle la taille : une cellule représente 8 sur 16 points de mise en page. Choisissez explicitement les styles de votre application ; les composants conservent leurs formes et tailles ordinaires. `examples/tui` illustre les champs, boutons, raccourcis et le redimensionnement.

Compilez un exécutable console avec `go tool nsgo build -target tui_macos_arm64`, ou son équivalent `tui_macos_x64`, `tui_windows_arm64`, `tui_windows_x64`, `tui_linux_arm64` ou `tui_linux_x64`. Le groupe de compilation/vérification `tui` inclut les six, et `all` les inclut également. Chaque répertoire de paquet contient l'exécutable et les ressources ; il ne comporte pas d'installateur graphique. Les six compilent avec Go seul.

Le moteur prend en charge le texte des cellules, les arrière-plans, les bordures pleines, le découpage, le défilement, le focus et l'édition de texte. Il utilise la police et les couleurs du terminal ; Contrôle sert de `mod`, y compris sur Mac. Ctrl+C copie dans un éditeur et quitte lorsqu'aucun raccourci ni éditeur ne l'intercepte. Le collage entre crochets (*bracketed paste*) insère le texte sans déclencher de raccourcis, et le presse-papiers est local au processus. Le canevas, les images, les médias, les composants natifs hébergés, les menus/alertes système, les animations, le zoom, la mise en forme bidirectionnelle, l'ellipse en début/milieu et un pont vers le presse-papiers système ne sont pas encore implémentés. `base/terminal` reste le contrôle de terminal intégré dans une application graphique.

Tant que `nsgo dev` contrôle le terminal, les diagnostics de l'outil et de l'application vont dans `.nsgo/tui.log`. À la sortie, l'écran, le curseur et les modes du terminal sont restaurés avant de rendre la main au shell. Les captures d'écran des tests terminal rastérisent les cellules avec une police portable fixe plutôt que de capturer l'émulateur ; utilisez des assertions textuelles pour les glyphes Unicode absents de cette police.

## nsgo.yaml

```yaml
name: Hello
id: dev.example.hello   # the bundle ID; without it, dev.nsgo. and the name's letters and digits
version: 0.1.0
build_number: 1         # CFBundleVersion and Android's versionCode; else from version
# copyright: © 2026 Example  # the Windows executable's details and installer, and the macOS About panel
# appearance: dark      # light or dark whatever the system's
# theme: theme.json     # a TSX app's theme: NSGo's or a shadcn/ui one
# i18n: { dir: locales, source: en }  # the app's catalogs and the language they are written in
icon:
  apple: assets/AppIcon.icon  # macOS, iOS and watchOS, or an .icns for macOS
  windows: assets/IconWindows.png  # optional Windows artwork
  linux: assets/IconLinux.png      # optional Linux artwork
  png: assets/Icon.png        # every other target
  tvos: assets/AppIconTV.brandassets  # the Apple TV's layered icon and top shelf art, or a wide PNG
  watchos: assets/AppIconWatch.icon   # the Apple Watch's own, drawn in circles, or a square PNG
  android:
    png: assets/IconAndroid.png  # optional bitmap; valid without adaptive layers
    foreground: assets/IconForeground.png  # Android's adaptive icon: 432 px, the logo within the center 264 px
    background: "#1F1F1F"                 # a color or a square PNG
    monochrome: assets/IconMonochrome.png  # optional: the shape Android 13's themed icons tint
dev:
  target: native        # native, web, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android, harmonyos or miniprogram_<platform>
  hot_reload: true      # false builds once, watching no files
  # icon: { png: assets/Icon-dev.png }  # replaces icon's files in dev's builds; defaults to assets/*-dev.*
build:
  output: dist
  tags: []              # Go build tags every build, test and check of the app takes, such as timetzdata
  ldflags: []           # the app's own linker flags, such as -X main.releaseDate=2026-11-17
  open: false           # true opens the output directory after building
  installers: true      # false leaves out the disk image, the Windows installer and the Linux packages
  jobs: 0               # how many targets build at once; 0 is half the CPUs, 1 one after another
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android_arm64, web]
# apple: { sign_in_with_apple: true }  # the Sign in with Apple entitlement, for iOS, tvOS and Mac App Store builds
# apple: { purchases: true }  # StoreKit's library, which in-app purchases go through, in macOS, iOS and tvOS packages
# apple: { privacy: { accessed_apis: { NSPrivacyAccessedAPICategoryDiskSpace: [E174.1] }, tracking: false } }  # add required-reason APIs and tracking declarations to PrivacyInfo.xcprivacy
# apple: { requires_full_screen: false }  # let an iPad run the app in Split View, Slide Over and Stage Manager, in all four orientations
# apple: { orientations: [portrait, landscape_left] }  # the orientations a full-screen app takes on an iPad
# apple: { multiple_scenes: true }  # let an iPad open more than one window of the app, each showing the main view with its own state
# apple: { info: { UIBackgroundModes: [audio] } }  # background playback for iOS Picture in Picture
# android: { permissions: [REQUEST_INSTALL_PACKAGES] }  # manifest permissions beyond the runtime's
# android: { recording: true }  # the microphone service microphone.RecordWAV records in, with RECORD_AUDIO, the foreground-service permissions and POST_NOTIFICATIONS, which RecordWAV asks for so its notification shows from Android 13; a refusal still records
# android: { uses_cleartext_traffic: true }  # allow plain HTTP to any host in the APK and App Bundle
# android: { background_downloads: true }  # dataSync foreground service for cx.Download
# android: { background_motion: true }  # explicit finite motion shortService; streams also need a notification title
# android: { loopback_cleartext: true }  # allow plain HTTP to loopback in the APK and App Bundle
# commands: [{ name: hello-cli, package: ./cmd/hello-cli }]  # command-line programs the desktop packages ship beside the app: see Commands
# windows: { command_line: true }  # add <name>.com, a console companion that forwards arguments and streams to <name>.exe; not in store builds
# windows: { path: true }  # the installer puts the app's folder on the user's Path, so consoles find its commands
# windows: { publisher: Example Ltd }  # the company the Windows executable's details, its installer and the uninstall entry name; else windows.store.publisher_name
# linux: { maintainer: "Jane Doe <jane@example.com>" }  # the Linux packages' maintainer, else the app's name
# linux: { categories: [Office, Dictionary] }  # the freedesktop menu categories of the app's desktop entry, at least one a main category such as Office, Utility or Game, else menus file it under Other; your own start with X-
# linux: { share: packaging/share }  # files the .deb and Arch package install under /usr/share and the AppImage in its usr/share, such as metainfo/<id>.metainfo.xml or bash-completion/completions/<command>
# linux: { depends: { deb: [gstreamer1.0-plugins-good], arch: [gst-plugins-good] }, recommends: { deb: [gstreamer1.0-libav], arch: ["gst-libav: H.264 and AAC video"] } }  # packages the .deb and Arch package also depend on and recommend, after GTK 4 and WebKitGTK 6, each as dpkg or pacman writes it
# release: { github: me/hello }  # the GitHub repository the install scripts and updates install from, else the Git remote origin's; tag_prefix: v by default
# updates: { key: <public key> }  # how the app updates, by platform and channel: see Updates
# store: { dir: store }           # store listings directory (YAML and Markdown per language)
# miniprogram: { appid: { wechat: wx0123456789abcdef, alipay: "2021000000000000" }, custom_navigation: false }  # the mini programs'
```

Un nom de système d'exploitation nu dans `targets` — ou dans `nsgo build -target` et `nsgo check -target` — désigne ses architectures (`windows` correspond à `windows_arm64` et `windows_x64` ; `android` correspond uniquement à `android_arm64`, car la compilation x86_64 nécessite un NDK). `macos_universal`, que `macos` et `all` omettent, compile les deux architectures macOS et les réunit en une seule application, bibliothèques et widgets compris. Le canal Steam utilise le paquet universel d'application. Les compilations avec installateur l'enveloppent dans une seule image disque, avec un seul flux Sparkle sur le canal direct. `nsgo test -target` accepte également le nom de l'architecture pour laquelle s'exécutent ses tests : celle d'un système de bureau sur l'architecture de cette machine et celle d'Android sur `arm64`, de sorte que `windows_x64` exécute la cible `windows` sur une machine Windows x64, tandis qu'une architecture pour laquelle la cible ne s'exécute pas est refusée en indiquant le nom à utiliser.

Les drapeaux remplacent les valeurs qu'ils nomment, comme `-target`, `-version` ou `-output` ; `-h` les liste tous.

`build.ldflags` contient les drapeaux d'éditeur de liens propres à l'application, chacun écrit tel que `-ldflags` de `go build` l'accepte, comme `-X main.releaseDate=2026-11-17` ou `-X 'main.channel=beta builds'`. Chaque compilation qui lie l'application les utilise : `dev` et ses rechargements, `build` avec `-embed` aussi et `test` avec `-target` aussi, sur chaque cible. `-ldflags` passé à `build` et `test` s'y ajoute au lieu de les remplacer, en répétant l'option si besoin. Ils précèdent les drapeaux `-X` propres à l'outil, et l'éditeur de liens conserve le dernier pour une variable donnée, de sorte que le nom, l'ID et la version dans `core.App` restent ceux de `nsgo.yaml`. L'outil passe à Go son propre `-ldflags`, qui remplace celui présent dans `GOFLAGS`, les drapeaux de liaison ont donc leur place ici ; un `-ldflags` passé à un `test` sans interface graphique après le répertoire appartient à `go test` et les remplace tous.

Chaque commande `go` exécutée par l'outil pour l'application reprend `build.tags`, dans `dev` et ses rechargements, `build` avec `-embed` aussi, `test` avec `-target` aussi et `check`, sur chaque cible. Les tags propres à NSGo, comme `nsgodev` dans `dev` ou `nsgotest` dans les tests cibles, s'y ajoutent, tout comme les tags de `-tags` dans `GOFLAGS`, qui seraient autrement remplacés ; le reste de `GOFLAGS` s'applique tel quel.

`apple.info.LSMinimumSystemVersion` relève la version minimale du paquet macOS, et `apple.info.MinimumOSVersion` relève celle du paquet iOS, tvOS ou watchOS sélectionné. Écrivez des chaînes de version entre guillemets, comme `"14.2"` ou `"18.0"`, avec une à trois composantes numériques. Les planchers d'exécution sont macOS 13.0 (Go 1.27), iOS 15.0, tvOS 17.0 et watchOS 26.0 ; les valeurs inférieures ou mal formées échouent. La clé mobile doit satisfaire le plancher de chaque cible mobile sélectionnée. Les App Clips conservent leur propre `app_clip.minimum`. Les valeurs atteignent les métadonnées du paquet, la compilation des ressources, les projets de signature et l'éditeur de liens de l'application ; les frameworks iOS intégrés les utilisent aussi, avec Mac Catalyst au moins à 16.0 pour le runtime macOS 13 de Go.

`icon.windows` et `icon.linux` sélectionnent des PNG carrés pour leurs paquets d'application, visuels d'installateur et logos de boutique. Ils utilisent par défaut `assets/IconWindows.png` et `assets/IconLinux.png`, puis se rabattent sur `icon.png` ; `dev.icon` accepte les mêmes réglages et les fichiers par défaut `-dev`. `icon.android.png` sélectionne un bitmap Android indépendamment, y compris lorsqu'aucune couche adaptative n'est configurée. Les autres cibles conservent le repli partagé sur `icon.png`.

L'icône d'une Apple TV est une carte en couches, pas un carré : `tvos` prend le `.brandassets` de Xcode, ses piles `App Icon` et images de rayon supérieur (*top shelf*), ou une image large telle que 1280×768, que l'outil redimensionne pour remplir la carte sur sa couleur de bord et centre sur les images de rayon supérieur. Sans l'un ni l'autre, le PNG occupe les trois quarts de la hauteur de la carte. Une Apple Watch masque son icône en cercle ; `watchos` accepte donc un `.icon` dessiné pour les cercles de watchOS ou un PNG carré ; à défaut, l'application utilise `apple`, puis `png`. Les deux se rabattent sur `assets/AppIconTV.*` et `assets/AppIconWatch.*`, comme le font `apple` et `png`.

Sur Android, `icon.android.foreground` et `background` rendent l'icône du lanceur adaptative, de sorte que les lanceurs la découpent selon leur forme et la déplacent à leur guise au lieu de réduire `png` sur une plaque propre. Chaque couche fait 108 dp de côté, soit un PNG mis à l'échelle à 432 px ; le lanceur affiche les 72 dp centraux (288 px) et la masque selon une forme susceptible d'entamer tout ce qui sort du cercle central de 66 dp (264 px) ; gardez donc le logo dans ce cercle et laissez l'arrière-plan remplir tout le carré. `background` est un PNG ou une couleur telle que `"#1F1F1F"`. `monochrome`, une forme dont seul l'alpha compte, est ce qu'Android 13 et ultérieur teintent lorsque l'utilisateur active les icônes thématisées ; sans elle, l'icône y conserve ses couleurs. Le paquet garde `icon.android.png`, sinon `icon.png`, comme icône bitmap simple, qu'aucun lanceur d'Android 9 ou ultérieur — la plus ancienne version prise en charge par NSGo — ne préfère à l'icône adaptative ; sans `png`, l'outil la génère à partir du premier plan sur l'arrière-plan. Sans couches adaptatives, le lanceur affiche le bitmap Android ou le `png` partagé.

`android.uses_cleartext_traffic: true` autorise le HTTP en clair vers n'importe quel hôte dans un APK ou App Bundle, en définissant à la fois l'attribut du manifeste et sa politique de sécurité réseau. Sans cette option, `android.loopback_cleartext: true` autorise le HTTP en clair uniquement depuis `localhost`, `127.0.0.1` et `::1`, par exemple pour une vidéo servie par l'application elle-même. La politique limitée au bouclage interdit le trafic en clair vers d'autres hôtes ; l'option globale prévaut lorsque les deux réglages sont vrais. Pour un AAR intégré, configurez la politique de sécurité réseau Android de l'application hôte.

Chaque compilation accepte également les drapeaux propres à NSGo au démarrage, tels que `--nsgo-fps` ([Exécution et débogage](debugging.md#flags-of-every-build)). Dans `nsgo dev`, Commande/Contrôle+R recharge depuis n'importe quelle fenêtre ouverte et reste disponible après la fermeture de la vue principale lorsqu'une autre fenêtre maintient une fonction d'application active.

### Écran de lancement

Définissez `launch_screen` pour afficher un arrière-plan opaque, un PNG centré ou les deux sur iOS et Android 12 ou ultérieur. Les chemins d'images sont relatifs à `nsgo.yaml`. L'image s'inscrit proportionnellement dans 128 × 128 points ou dp. Un PNG transparent dont le côté le plus long mesure au moins 512 pixels reste net sur les écrans haute densité.

```yaml
launch_screen:
  background: "#F5F1EA"
  image: assets/Launch.png
  dark:
    background: "#201C26"
    image: assets/LaunchDark.png
```

Le système sélectionne l'apparence à moins que `appearance` ne la fixe. Omettez l'un ou l'autre champ `dark` pour réutiliser la valeur correspondante. Un arrière-plan ou une image omis conserve le réglage par défaut de la plateforme. Les applications intégrées reçoivent leur écran de lancement de l'hôte.

## Classes natives Android

Listez les classes Android précompilées dans le fichier `nsgo.yaml` de l'application pour utiliser une `View` personnalisée depuis `core.Native`.

```yaml
android:
  classes: [java/BadgeView.jar, java/Helpers.dex]
```

Les chemins sont relatifs à `nsgo.yaml` sauf s'ils sont absolus. Chaque chemin désigne un fichier `.dex` ou un JAR contenant le `classes.dex` racine, `classes2.dex` et les fragments suivants générés par D8. Préparez ces fichiers avec D8 en utilisant `--min-api 28`. Sur arm64, NSGo assemble l'APK ou l'App Bundle à partir de ces fichiers précompilés avec Go seul. Les sessions de développement Android recompilent lorsqu'un fichier listé change. Pour un AAR intégré, la compilation Android de l'hôte fournit ses dépendances Java. [Documentation D8](https://developer.android.com/tools/d8)

L'[exemple natif Android](https://github.com/nsgo-dev/nsgo/tree/main/examples/android-native) enregistre une classe dotée d'un constructeur public `Context` et la configure avec une prop Go.

## Jeux

Définissez `game: true` dans `nsgo.yaml` pour un jeu. Android classe l'application comme un jeu et utilise les optimisations du mode Jeu système. macOS et iOS activent le mode Jeu, macOS choisit par défaut la catégorie Jeux, et Linux ajoute `Game` à l'entrée desktop. Une clé explicite `macos.category` ou une clé Game Mode dans `apple.info` a priorité.

Définissez `gamepad: true` lorsque votre application gère une manette matérielle. Android déclare à la fois la manette et l'écran tactile comme facultatifs. macOS, iOS et tvOS déclarent le profil Extended Gamepad, et tvOS également le Micro Gamepad de la Siri Remote, les clés de contrôleur dans `apple.info` ayant priorité.

## Mentions de licences

`nsgo build` place `THIRD_PARTY_NOTICES.txt` dans les ressources du paquet autonome. Ce fichier inclut les textes de licence des modules Go utilisés par cette cible, du runtime Go et du runtime Java ou JavaScript embarqué par NSGo. Les compilations web nomment le fichier d'après son contenu, comme pour les autres ressources. Utilisez `core.ThirdPartyNotices()` pour afficher ce même texte dans un écran de crédits intégré à l'application. La fonction renvoie une chaîne vide en développement, dans les tests et dans les compilations de bibliothèques intégrées. Les [déclarations de bibliothèques natives](native-libraries.md) ajoutent leurs textes de licence d'origine aux mêmes mentions.

Par défaut, une licence tierce inconnue ou manquante fait échouer la compilation et une licence copyleft détectée émet un avertissement. Les modules répertoriés dans l'espace de travail Go de l'application (`go.work`) sont traités comme du code de première partie aux côtés du module propre de l'application : tous les fichiers de licence qu'ils contiennent sont inclus et ils ne déclenchent pas de vérifications tierces. Réglez chaque règle sur `error` ou `warn`. La liste `allow` exempte des chemins de module exacts de ces deux règles et conserve tout texte de licence trouvé par la compilation.

```yaml
licenses:
  unknown: error
  copyleft: warn
  allow: [example.org/reviewed-module]
```

## Bibliothèques issues de paquets Go

Les applications qui importent `base/terminal` embarquent également `libghostty-vt` : les paquets Windows et Linux placent la bibliothèque à côté de l'exécutable ; les paquets macOS et iOS la signent dans leurs frameworks. [Intégration](embedding.md) explique le XCFramework compagnon pour les hôtes iOS.

Une dépendance peut déclarer des bibliothèques partagées de bureau précompilées dans `nsgo-natives.yaml`. NSGo vérifie leur SHA-256, les copie dans l'application avant la signature et les inclut dans les installateurs ; l'auteur de l'application n'exécute aucune étape supplémentaire. [Bibliothèques natives](native-libraries.md) décrit le format destiné à l'auteur du paquet et les chemins de chargement.

## Commandes

Une application peut embarquer des programmes en ligne de commande à côté de son exécutable de bureau, comme une interface CLI agissant sur les données de l'application. Listez-les sous `commands`, chacun compilé à partir d'un paquet Go `main` :

```yaml
commands:
  - name: hello-cli             # the file's name, hello-cli.exe on Windows
    package: ./cmd/hello-cli    # a main package, built for each desktop target with CGO_ENABLED=0 and build.tags
    ldflags: [-X main.channel=beta]  # its own linker flags, after build.ldflags
windows:
  path: true                    # the installer puts the app's folder on the user's Path
```

Un nom ne comporte que lettres, chiffres, `-` et `_`, et ne peut être ni celui de l'exécutable de l'application, ni sa commande Linux, ni `Uninstall`, ni `AppRun`, ni celui d'une autre commande quelle que soit la casse. `macos_universal` fusionne les deux compilations macOS de chaque commande, comme elle fusionne celle de l'application.

Un paquet est lié avec `build.ldflags` et `-ldflags`, puis ses propres `ldflags` dont les valeurs `-X` l'emportent, puis l'identité de l'application, de sorte que `core.App()` fournisse à la commande le nom, l'ID, la version et le numéro de build de `nsgo.yaml`, comme à l'application. Une commande qui n'importe pas `core` reçoit la version via `-X`, par exemple `-ldflags "-X main.version=1.2.0"` passé à `build`, que l'application et ses commandes reçoivent toutes deux.

| Paquet | Commandes |
| --- | --- |
| macOS | Dans `Contents/MacOS`, signées avant l'application telle quelle, sous `<id>.<command>` : ad hoc, ou avec `sign.identity`, le runtime renforcé (*hardened runtime*) et un horodatage, afin que l'application soit [notarisée](#notarize), mais sans les droits (*entitlements*) de l'application. L'image disque ne place rien dans `PATH` ; le [cask Homebrew](#channels) lie chaque commande, et l'utilisateur peut lier `/Applications/<name>.app/Contents/MacOS/<command>` lui-même |
| Windows | `<command>.exe` à côté de `<name>.exe`, [signé](#sign-windows-packages) comme l'application, installé par l'installateur et supprimé par son désinstallateur ; avec `windows.path`, l'installateur ajoute aussi le dossier au `Path` de l'utilisateur, comme le fait `windows.command_line`, afin que les consoles ouvertes après installation trouvent les commandes. Une commande renommée ou retirée lors d'une mise à jour reste dans le dossier, comme tout fichier que l'application ne fournit plus, car le désinstallateur ne supprime que ce que sa version a installé |
| Linux | Dans `/opt/<slug>` avec un lien `/usr/bin/<command>` détenu par les paquets `.deb` et Arch, si bien que la suppression du paquet le retire, et que dpkg et pacman refusent de s'installer par-dessus celui d'un autre paquet ; l'AppImage les conserve à côté de l'application et ne place rien dans `PATH` |

Les installateurs, leurs sommes de contrôle et les flux appcast de [mise à jour](#updates) transportent les commandes comme les autres fichiers de l'application, de sorte qu'une mise à jour les remplace avec l'application. Les [paquets de boutique](#store-packages) les omettent, car le bac à sable du Mac App Store, le MSIX et le Flatpak n'exécutent que l'exécutable propre de l'application.

## Installateurs

Pour les cibles de bureau, `build` écrit ce à partir de quoi les utilisateurs installent l'application dans `dist/releases`, chacun accompagné d'une somme de contrôle `.sha256` :

| Cible | Installateur |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.dmg`, une image disque présentant l'application, une flèche et un lien vers Applications où la glisser, signée avec `sign.identity` et [notarisée](#notarize) |
| Windows | `<name>-<version>-windows-<arch>-setup.exe`, un installateur [NSIS](https://nsis.sourceforge.io) qui installe pour l'utilisateur, à nouveau dans le dossier choisi précédemment, en fermant l'application si elle tourne, avec un raccourci dans le menu Démarrer, un raccourci facultatif sur le Bureau et un désinstallateur ne supprimant que ce qu'il a installé, [signé avec `sign.certificate`](#sign-windows-packages) là où l'application signe les paquets Windows ; `windows.command_line` installe aussi le compagnon `.com` et ajoute le dossier de l'application au `Path` de l'utilisateur, afin que les consoles ouvertes après installation exécutent la commande de l'application, tandis que Win+R, qui trouve aussi le compagnon, démarre l'application et ferme la console ouverte ; le désinstallateur ne retire le dossier du `Path` que si un installateur l'a ajouté, jamais une entrée que vous aviez déjà |
| Linux | `<name>-<version>-linux-<arch>.deb` pour Debian et Ubuntu, et `<name>-<version>-linux-<arch>.pkg.tar.gz` pour Arch Linux, qui installent l'application dans `/opt/<slug>` avec une commande, des liens vers ses [commandes](#commands), son entrée desktop et son icône (le paquet Arch inclut `.MTREE` pour `pacman -Qkk`), ainsi que `<name>-<version>-linux-<arch>.AppImage`, qui s'exécute sur toute distribution sans installation |

L'installateur Windows nécessite le compilateur de NSIS, version 3.06 ou ultérieure — sur Mac, `brew install makensis` — si bien qu'un Mac construit tous les installateurs ; sans lui, `build` laisse le script `<name>.nsi` dans le paquet, qui se compile partout tel quel. L'installateur parle les langues des catalogues de l'application que NSIS traduit, chinois simplifié et traditionnel inclus, en choisissant celle du système. `-installers=false` ou `build.installers: false` compile uniquement les paquets.

Les installateurs reprennent vos propres images et icônes ; sinon, chacun utilise l'icône de l'application ou l'image intégrée :

```yaml
macos:
  dmg_background: assets/DMGBackground.png  # the disk image's window, which takes its size in points
  dmg_icon: assets/DMGIcon.png              # the disk image's icon: an .icns or a square PNG
windows:
  installer_icon: assets/Setup.png          # the setup program's icon: an .ico or a square PNG
  uninstaller_icon: assets/Uninstall.png    # the uninstaller's
```

Un PNG d'arrière-plan doté d'un jumeau deux fois plus grand, tel que `DMGBackground@2x.png`, reste net sur les écrans Retina. L'icône de l'application se place à un quart depuis la gauche, Applications à un quart depuis la droite, chacun avec son nom centré de haut en bas, là où l'image intégrée dessine sa flèche ; laissez-leur de la place dans un arrière-plan personnalisé. Les paquets Linux n'ont pas d'images propres ; ils installent l'icône de l'application.

Les paquets Linux installent aussi les métadonnées AppStream de l'application, `/usr/share/metainfo/<id>.metainfo.xml`, d'après lesquelles les logithèques la répertorient, rédigées à partir de `linux.summary`, `linux.license` (s'il est défini) et des noms de l'application, avec `Utility` comme catégorie lorsque `linux.categories` n'en nomme aucune ; l'AppImage les conserve dans le `usr/share` de son AppDir, à côté d'un lien vers son entrée desktop dans `usr/share/applications`, mentionnée par les métadonnées. Les fichiers que le système lit depuis `/usr/share`, tels que des métadonnées plus riches, d'autres entrées desktop, des complétions shell ou des pages de manuel, vont dans le répertoire nommé par `linux.share`, relatif à `nsgo.yaml` : les paquets `.deb` et Arch installent son arborescence sous `/usr/share`, et l'AppImage sous le `usr/share` de son AppDir. Son `metainfo/<id>.metainfo.xml` remplace celui écrit par NSGo, et un fichier situé à un chemin que le paquet génère lui-même, comme l'entrée desktop ou l'icône de l'application, fait échouer la compilation. Les fichiers de `resources/`, quel que soit leur nom, restent à côté de l'exécutable dans `/opt/<slug>`, et les Flatpak des compilations de boutique n'utilisent que les métadonnées de NSGo.

L'AppImage nécessite FUSE, présent sur la plupart des distributions, et utilise le GTK 4 du système, comme les paquets ; son environnement d'exécution est téléchargé une fois dans le cache utilisateur.

Les AppImage embarquent un localisateur `gh-releases-zsync` utilisant `release.github` ou le dépôt distant GitHub `origin`, en sélectionnant la dernière version stable et l'architecture de cette application. Téléversez chaque `.AppImage` avec son fichier `.AppImage.zsync` généré. Pour un autre hébergeur, définissez `linux.appimage_update` sur `zsync|https://example.com/latest.AppImage.zsync`, une URL stable servie avec prise en charge des plages d'octets HTTP ; il accepte également `gh-releases-zsync|owner|repo|latest-pre|name-*-linux-x64.AppImage.zsync` pour un canal de préversion. Une chaîne vide explicite désactive la découverte intégrée ; sans dépôt GitHub ni localisateur configuré, elle est également vide. Le fichier `.zsync` est tout de même généré, séparément du flux appcast signé.

## Paquets de boutique

`-channel store`, ou `updates.channel: store`, compile les cibles de bureau et Android pour leurs boutiques, qui se chargent des mises à jour, à la place des installateurs et scripts d'installation :

| Cible | Paquet de boutique |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.pkg` pour le Mac App Store : l'application dans le bac à sable (*App Sandbox*), signée avec l'identité du profil de provisionnement et empaquetée avec l'identité d'installateur de votre équipe |
| Windows | `<name>-<version>-windows-<arch>.msix` pour le Microsoft Store, qui le signe lors de la publication |
| Linux | `<name>-<version>-linux-<arch>.flatpak` installable, plus un `.tar.gz` contenant AppStream et `flatpak/<id>.yml` pour Flathub ou `flatpak-builder` sur le runtime de GNOME |
| Android | `<name>-<version>-android-<arch>.aab`, un App Bundle pour Google Play signé avec `sign.keystore`, la clé d'envoi (*upload key*) |

Le MSIX lit le nom d'affichage de chaque langue depuis `appName` dans les catalogues, en se repliant sur la langue source. Ses logos Démarrer, barre des tâches et Store incluent les variantes 100 %, 125 %, 150 %, 200 % et 400 %. L'outil génère leur index de ressources sous macOS ; aucun SDK Windows n'est requis.

`-target macos` sélectionne les deux architectures macOS sur le canal store et produit un unique `<name>-<version>-macos-universal.pkg`, tout comme `-target macos_universal`. Sélectionnez `-target android_arm64,android_x64` pour obtenir un unique `<name>-<version>-android-universal.aab` contenant `arm64-v8a` et `x86_64`. La tranche `x86_64` nécessite un NDK Android.

```yaml
macos:
  category: public.app-category.productivity  # LSApplicationCategoryType, which the Mac App Store requires
  # entitlements: { com.apple.security.files.downloads.read-write: true }  # beyond the network, the files the user picks and what apple.info explains
windows:
  store:                               # as Partner Center's Product identity lists them
    identity: 12345Example.Hello
    publisher: CN=00000000-0000-0000-0000-000000000000
    publisher_name: Example
  # device_capabilities: [microphone, webcam]  # the devices the app uses, which Windows asks the user for
linux:
  summary: Says hello                  # one line for software centers
  license: MIT                         # an SPDX expression; LicenseRef-proprietary by default
  # flatpak_runtime: "51"              # GNOME's runtime version
android:
  target_sdk: 36                       # the targetSdkVersion Google Play requires
sign:
  macos_profile: assets/MacAppStore.provisionprofile  # the Mac App Store's profile
  # installer: "3rd Party Mac Developer Installer: Example (ABCDE12345)"  # by default the keychain's of the profile's team
```

Le runtime cible l'API 36 lorsque `android.target_sdk` n'est pas défini. Une compilation d'Android App Bundle échoue en dessous de l'API 36, exigée par [Google Play](https://support.google.com/googleplay/android-developer/answer/11926878) pour les nouvelles applications mobiles et mises à jour à compter du 31 août 2026. Une compilation d'APK autonome émet un avertissement et continue.

Définissez `sign.macos_profile` pour signer le paquet Mac en vue de son téléversement. Sans widgets, le paquet non signé s'exécute tout de même dans le bac à sable, ce qui permet de tester l'accès au conteneur de l'application, au réseau et aux fichiers sélectionnés par l'utilisateur. Une compilation de boutique avec widgets exige à la fois `sign.macos_profile` pour `<id>` et un `sign.widgets_profile` macOS pour `<id>.widgets`, issus de la même équipe et accordant `apple.app_group` (`group.<id>` par défaut). Les deux paquets utilisent ce conteneur partagé ; aucune exception de répertoire personnel n'est ajoutée. `-installers=false` conserve le bac à sable et les signatures de profil tout en omettant le `.pkg` et son identité d'installateur. Téléversez le `.msix` dans Partner Center et le `.aab` dans Play Console. Soumettez `flatpak/<id>.yml` à Flathub une fois que la publication contient les archives qu'il nomme. `flatpak-builder` le compile aussi localement. Installez un paquet avec `flatpak install --user ./<name>-<version>-linux-<arch>.flatpak` ; le runtime GNOME correspondant doit être installé ou disponible depuis un dépôt configuré. Les compilations produisent le paquet en Go sans chaîne d'outils Linux. Les paquets de boutique utilisent une icône transparente en l'absence d'illustration et `Utility` lorsque `linux.categories` n'est pas défini. Pour une installation hors Store, définissez [`sign.certificate`](#sign-windows-packages) afin de signer le MSIX ; le sujet de son certificat doit correspondre à `windows.store.publisher`, et la machine destinataire doit faire confiance au certificat. Sans paramètres de signature, le Store signe le paquet téléversé.

Sur macOS et iOS, les effets de bord d'une vue défilante (`Scroll.EdgeEffects`), les flous d'arrière-plan macOS sans `IOS.BackdropMaterial` et les filtres de couleur d'arrière-plan (seuls, ou par-dessus un flou sans `IOS.BackdropMaterial`) dessinent via les classes privées `CABackdropLayer` et `CAFilter` de Core Animation, comme le font les matériaux et effets de bord propres à AppKit et UIKit, car aucune API publique ne floute selon un rayon, ne fait varier un flou sur une vue ou n'applique une matrice de couleurs en arrière-plan. L'examen de l'App Store peut rejeter les applications utilisant des API privées : NSGo ne les appelle que pour les vues défilantes avec barres de bord et ces filtres d'arrière-plan, mais chaque compilation macOS et iOS contient leurs noms.

Chaque paquet Apple produit par `build` — l'application, son extension de widgets, son App Clip, son application Apple Watch et un framework `-dynamic` `-embed` — embarque un manifeste de confidentialité, `PrivacyInfo.xcprivacy`, déclarant les API à motif requis utilisées par NSGo et le runtime Go : préférences utilisateur (`CA92.1`), horodatages de fichiers (`C617.1`) et heure de démarrage système (`35F9.1`). `apple.privacy` ajoute ceux propres à l'application : `accessed_apis`, les motifs de chaque catégorie, `tracking`, `tracking_domains` et `collected_data`, entrées de `NSPrivacyCollectedDataTypes` telles que nommées par Apple ; un fichier `PrivacyInfo.xcprivacy` dans `resources/` y est fusionné plutôt que remplacé.

Le `Info.plist` de chaque paquet iOS, tvOS et watchOS produit par `build` pour des appareils — celui de l'application, de son extension de widgets et de son App Clip — comporte les clés SDK et Xcode `DT` vérifiées par App Store Connect, lues depuis le Xcode qui compile, ainsi que `UIRequiredDeviceCapabilities: [arm64]` ; les paquets pour simulateur les omettent, et toute clé définie par `apple.info` conserve sa valeur. `apple.multiple_scenes: true` active `UIApplicationSupportsMultipleScenes` dans le manifeste de scène d'une application iOS, qui vaut `false` par défaut : sur un iPad, l'utilisateur peut alors ouvrir plusieurs fenêtres de l'application, chacune affichant de nouveau la vue principale avec son propre état ainsi que les signaux et le stockage partagés de l'application, jusqu'à ce que l'utilisateur la ferme.

### Publier sur Google Play

Activez l'API Google Play Developer et accordez à un compte de service l'autorisation de publier l'application sur le canal (*track*) choisi. Définissez `NSGO_PLAY_SERVICE_ACCOUNT` sur la clé JSON du compte ou sur le chemin de son fichier de clé. [Configuration de l'API Google Play](https://developers.google.com/android-publisher/getting_started)

```sh
go tool nsgo build -target android -channel store
go tool nsgo publish -target android -track internal
```

La commande trouve l'App Bundle correspondant à la version du projet dans `build.output/releases`. Passez un chemin `.aab` après les drapeaux pour sélectionner un fichier lorsqu'il existe plusieurs compilations. Utilisez `internal`, `production`, ou l'identifiant de canal d'un test fermé. La commande lit le nom du paquet et le code de version depuis le bundle, exige un code supérieur à toute version déjà sur ce canal, puis téléverse le bundle et valide la publication.

## Fiches de boutique

Les métadonnées des boutiques (titres, sous-titres, descriptions, mots-clés, notes de version, URL, captures d'écran et aperçus) résident dans le dépôt sous forme de fichiers texte YAML et Markdown dans `store/` (ou `store.dir` dans `nsgo.yaml`) :

```text
store/
  listing.yaml              # version: 1, default_locale, app_store and google_play settings
  en-US/
    listing.yaml            # version: 1, title, subtitle, short_description, keywords, URLs, media
    description.md          # full description (optional file in place of YAML description)
    release_notes.md        # what's new (optional file in place of YAML release_notes)
    promotional_text.md     # App Store promotional text (optional file)
    screenshots/
  zh-Hans/
    listing.yaml
    description.md
    release_notes.md
    screenshots/
```

Les deux fichiers `listing.yaml` exigent `version: 1` et refusent les clés inconnues. Les répertoires de locales canoniques tels que `zh-Hans` correspondent automatiquement au code de chaque boutique (`zh-Hans` sur App Store Connect, `zh-CN` sur Google Play), sauf surcharge par boutique avec `app_store.locale` ou `google_play.locale`.

```sh
go tool nsgo publish -store all -dry-run                  # validate lengths, locales and media without uploading
go tool nsgo publish -store appstore,play                 # validate and upload to App Store Connect and Google Play
go tool nsgo publish -store play -track production        # also update release notes on the named Play track
```

La validation s'exécute avant de lire les identifiants ou d'effectuer la moindre requête HTTP, en vérifiant les champs obligatoires, les codes de locale, les schémas d'URL, les fichiers et quantités de médias (jusqu'à 10 captures d'écran et 3 aperçus par type d'écran App Store ; jusqu'à 8 captures d'écran par type d'image Google Play), ainsi que les limites de caractères (App Store : titre 30, sous-titre 30, mots-clés 100, texte promotionnel 170, description 4000, notes de version 4000 ; Google Play : titre 30, description courte 80, description 4000, notes de version 500).

Les identifiants de téléversement proviennent uniquement de l'environnement : `NSGO_APP_STORE_KEY`, `NSGO_APP_STORE_KEY_ID` et `NSGO_APP_STORE_ISSUER` (avec repli sur la clé de notarisation `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` et `NSGO_NOTARY_ISSUER`) pour App Store Connect, et `NSGO_PLAY_SERVICE_ACCOUNT` (chemin de fichier JSON du compte de service ou texte JSON) pour Google Play.

### Conformité des boutiques

Utilisez `nsgo.yaml` pour suivre l'état de chaque questionnaire de publication.

```yaml
compliance:
  app_store: { age_rating: false }
  google_play: { data_safety: false }
  steam: { content_survey: false }
```

Passez une entrée à `true` après avoir rempli son questionnaire. Une compilation correspondante `-channel store` ou `-channel steam` vous rappelle les entrées réglées sur `false`. Omettez toute entrée que vous ne suivez pas dans le YAML.

- Remplissez le [questionnaire de classification par âge](https://developer.apple.com/help/app-store-connect/manage-app-information/set-an-app-age-rating) et les [informations de confidentialité de l'app](https://developer.apple.com/help/app-store-connect/manage-app-information/manage-app-privacy) dans App Store Connect.
- Pour le [questionnaire sur le chiffrement](https://developer.apple.com/help/app-store-connect/manage-app-information/determine-and-upload-app-encryption-documentation) d'Apple, définissez `apple.info.ITSAppUsesNonExemptEncryption` sur la réponse de votre application sous forme de booléen YAML. Si Apple envoie un code d'approbation, placez-le dans `apple.info.ITSEncryptionExportComplianceCode` sous forme de chaîne. Vous pouvez omettre ces clés et répondre lors du téléversement. Les compilations boutique Apple vérifient les types de ces clés ainsi que la structure du manifeste de confidentialité du paquet.
- Remplissez la section [Sécurité des données](https://support.google.com/googleplay/android-developer/answer/10787469) dans la Play Console. Les applications distribuées exclusivement sur le canal de test interne en sont exemptées.
- Remplissez l'[enquête sur le contenu](https://partner.steamgames.com/doc/gettingstarted/contentsurvey) dans Steamworks avant de soumettre le jeu pour examen.

## App Store Connect

Créez une application dans App Store Connect et signez un IPA pour appareil iOS avec un profil de distribution App Store ou un PKG de boutique macOS avec l'identité d'installateur de votre équipe. Sur un Mac avec Xcode, définissez `NSGO_APP_STORE_KEY` sur le fichier `.p8` de la clé d'équipe App Store Connect ou son contenu, `NSGO_APP_STORE_KEY_ID` sur son ID de clé et `NSGO_APP_STORE_ISSUER` sur son ID d'émetteur ; sans eux, la clé de notarisation (`NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID`, `NSGO_NOTARY_ISSUER`) est utilisée, comme pour les fiches de boutique. Accordez à cette clé l'accès à l'application et l'autorisation de téléverser des builds.

```sh
go tool nsgo publish ios path/to/Hello.ipa
go tool nsgo publish macos path/to/Hello.pkg
```

L'outil vérifie le numéro de build du paquet avant de le téléverser, puis rapporte le résultat du traitement d'Apple pour TestFlight. Incrémentez `build_number` pour l'envoi suivant. Les numéros de build macOS doivent croître sur l'ensemble des versions de l'application, tandis qu'iOS peut repartir de zéro pour une nouvelle version. Soumettez séparément un build traité pour l'examen de l'App Store dans App Store Connect. [Guide de distribution d'Apple](https://developer.apple.com/documentation/xcode/preparing-your-app-for-distribution)

## Packs d'actifs de boutique

Déclarez les répertoires d'actifs volumineux dans `mobile.asset_packs`. Les fichiers contenus dans ces répertoires sont livrés séparément de l'Android App Bundle de base et de l'application iOS. Les noms de packs commencent par une lettre minuscule et peuvent contenir des lettres minuscules, des chiffres et des tirets bas (`_`).

```yaml
mobile:
  asset_packs:
    - {name: levels, dir: resources/levels, delivery: install-time}
    - {name: voices, dir: large-assets/voices, delivery: fast-follow}
```

Compilez les packs Android avec `go tool nsgo build -channel store -target android_arm64`. Utilisez `bundletool build-apks --local-testing` et `bundletool install-apks` pour tester le bundle sur un appareil. NSGo ouvre les packs `install-time` installés. Les modules `fast-follow` sont inclus pour la livraison via Play, et la lecture de ces packs depuis le SDK Play Asset Delivery reste à venir.

Sur iOS, `go tool nsgo build -target ios` associe `install-time` à la politique `essential` d'Apple et `fast-follow` à `prefetch`, tant à la première installation que lors des mises à jour. Xcode 26 ou ultérieur compile le téléchargeur Background Assets. Utilisez un `apple.app_group` partagé ou `group.<id>` par défaut. Le `sign.profile` de l'application et le `sign.asset_packs_profile` de l'extension `<id>.assetdownloader` doivent tous deux disposer de ce groupe. Téléversez les fichiers `.aar` depuis `dist/releases` aux côtés de l'IPA à l'aide du flux d'envoi de packs d'actifs d'App Store Connect. [Création de packs d'actifs gérés](https://developer.apple.com/documentation/backgroundassets/creating-managed-asset-packs)

Depuis une goroutine, appelez `mobile.AssetPackReady(ctx, "levels")` pour vérifier la disponibilité et `mobile.OpenAssetPackFile("levels", "intro.map")` pour ouvrir un fichier relatif au répertoire déclaré. Fermez le lecteur retourné après utilisation. [Tester les packs d'actifs Apple localement](https://developer.apple.com/documentation/backgroundassets/testing-asset-packs-locally)

## Dépôts Steam

```sh
go tool nsgo build -channel steam -target macos_universal,windows_x64,linux_x64
```

Définissez `updates.channel: steam` pour utiliser ce canal par défaut. Sélectionnez les cibles macOS, Windows et Linux. Téléversez chaque répertoire `dist/<name>-<version>-<target>` vers son dépôt Steam, en utilisant le bundle d'application sous macOS, l'exécutable sous Windows et Linux arm64, et `steam-launch.sh` sous Linux x64 comme option de lancement. Steam gère les mises à jour. Les répertoires ne contiennent ni installateurs, ni programme de mise à jour automatique, ni `steam_appid.txt`.

Les dépôts Linux x64 incluent GTK 4 et ses dépendances pour Steam Linux Runtime 3.0 et 4.0. Le lanceur ne les utilise que lorsque le système ne dispose pas de GTK 4.14 ou plus récent. Conservez `nsgo-gtk/` à côté du lanceur : son dossier `licenses/` contient les licences des bibliothèques, les sources à partir desquelles elles ont été compilées et le script de compilation, ce qu'exige la LGPL pour GLib, Pango et GTK. Linux arm64 nécessite GTK 4 disponible dans son environnement d'exécution.

Pour tester localement hors du client Steam, définissez `steam: {app_id: 480}` dans `nsgo.yaml`, en remplaçant 480 par votre App ID Steamworks. `go tool nsgo dev` écrit l'ID dans `steam_appid.txt` au sein de son répertoire de compilation natif et lance l'application avec ce répertoire comme répertoire de travail. [API Steamworks](https://partner.steamgames.com/doc/api/steam_api), [SteamPipe](https://partner.steamgames.com/doc/sdk/uploading).

## Publier sur GitHub

Lorsque la compilation comporte une cible macOS, Windows ou Linux, `dist/releases` contient également `install.sh` et `install.ps1`, qui installent l'application depuis les publications du dépôt GitHub nommé par `release.github`, sinon celui sur lequel pointe le dépôt distant Git `origin` de l'application ; sans l'un ni l'autre, `build` n'en écrit aucun et le signale. Téléversez ses fichiers — ceux du premier niveau sans `miniprogram/` — vers la publication taguée `v<version>` (`release.tag_prefix` remplace `v` ; les scripts encodent les caractères d'URL réservés dans ce préfixe) :

```sh
gh release create v1.2.0 dist/releases/*.*
```

Les utilisateurs installent ou mettent ensuite l'application à jour en une ligne, affichée par la compilation :

```sh
curl -fsSL https://github.com/me/hello/releases/latest/download/install.sh | sh    # macOS and Linux
```

```powershell
irm https://github.com/me/hello/releases/latest/download/install.ps1 | iex         # Windows
```

Sous macOS, le script copie l'application depuis l'image disque vers `/Applications`, ou `~/Applications` s'il ne peut pas y écrire ; sous Linux, il installe le paquet Arch avec `pacman`, sinon le paquet Debian avec `apt-get` ou `dpkg`, via `sudo` ; sous Windows, il exécute l'installateur silencieusement, ce qui installe là où l'application se trouve déjà. Chacun vérifie d'abord le fichier par rapport à sa somme de contrôle. `<NAME>_VERSION`, le nom de l'application en majuscules, par exemple `HELLO_VERSION=1.1.0`, installe une autre version, et `<NAME>_DOWNLOAD_URL` télécharge depuis un miroir.

## Publier sur itch.io

Définissez `itch.project` sur le slug `owner/game` de votre jeu itch.io dans `nsgo.yaml`. Pour surcharger les noms de canaux, définissez `itch.channels` par cible de compilation. Les valeurs par défaut sont `html5` pour le web, `osx-<arch>` pour macOS, `windows-<arch>` pour Windows et `linux-<arch>` pour Linux.

```yaml
itch:
  project: example/my-game
  channels:
    macos_arm64: osx-arm64-beta
```

Avec `BUTLER_API_KEY` configuré comme secret en CI, compilez et téléversez les cibles.

```sh
go tool nsgo build -channel none -installers=false -target web,macos_arm64
go tool nsgo publish itch -target web,macos_arm64
```

Utilisez les mêmes options `-output` et `-version` sur les deux commandes si vous les surchargez. Le téléversement prend le répertoire de paquet de chaque cible et attribue la version de la compilation à sa version itch.io. NSGo utilise `butler` présent sur `PATH` ou télécharge une copie épinglée depuis itch.io. Après le premier envoi web, réglez le type de jeu itch.io sur HTML et marquez l'envoi comme jouable dans le navigateur. [Documentation Butler](https://itch.io/docs/butler/pushing.html)

## Mises à jour

`updates` permet à l'application de se maintenir à jour selon les modalités de sa plateforme et du canal par lequel les utilisateurs l'obtiennent :

```yaml
updates:
  key: zE0hx2FZwwHy6X7LbEP3GjTTE3KWfCAp5gsaBysC+vg=  # the public key nsgo keygen prints
  # channel: store                    # direct, store, steam or none; direct by default with a key, none without
  # feed: https://example.com/hello/  # where the appcasts and installers are; by default the latest GitHub release
  # automatic: false                  # check only when asked; true by default
  # automatic_downloads: true          # Sparkle downloads automatically; false by default, macOS only
```

`nsgo keygen` affiche une paire de clés sans écrire aucun fichier : placez la clé publique dans `updates.key` et validez-la dans le dépôt, et conservez la clé secrète hors du dépôt, dans un gestionnaire de mots de passe et dans `NSGO_UPDATES_KEY` là où sont compilées les versions, par exemple un secret GitHub Actions. Chaque compilation directe signe les installateurs avec la clé secrète et écrit `appcast-<target>.xml` à côté d'eux, un flux au format de [Sparkle](https://sparkle-project.org) que l'application lit sous `feed` : téléversez les fichiers appcast avec les installateurs. Sans la clé secrète, `build` n'écrit aucun appcast macOS ni Windows, car ceux-ci n'installent que des installateurs signés, et les applications Linux et Android lisant un appcast non signé ouvrent la page de la version au lieu d'installer son paquet.

`-release-notes notes.md` intègre un fichier Markdown contenant les notes de la version, tel que sa section de changelog, dans chaque appcast, rendu en HTML sans le HTML brut du fichier lui-même. Sparkle et WinSparkle l'affichent dans leurs fenêtres de mise à jour, et l'invite sous Linux et Android l'affiche en texte sous la version, jusqu'à 1 500 caractères ; les applications le lisent via `core.UpdateCheck.Notes`.

| Cible | Fonctionnement des mises à jour d'une compilation directe |
| --- | --- |
| macOS | Sparkle, que `build` intègre à l'application, vérifie chaque jour, propose la nouvelle version, télécharge l'image disque, vérifie sa signature, l'installe et redémarre l'application ; le menu d'application par défaut comporte *Rechercher des mises à jour…*, qu'une application définissant son propre menu ajoute avec `updater.Check` |
| Windows | WinSparkle, à côté de l'exécutable, fait de même dans la langue affichée par l'application au démarrage, lorsque WinSparkle en dispose, et lance silencieusement l'installateur, qui démarre la nouvelle version |
| Linux, Android | L'application lit son appcast et, dès que l'utilisateur choisit Mettre à jour, télécharge en flux et vérifie le paquet signé directement sur disque avec suivi de progression (`updater.Status` et l'indicateur de progression de tâche de la plateforme), puis l'installe via `pkexec dpkg`, `pkexec pacman` (`.pkg.tar.gz`/`.pkg.tar.zst`), remplacement atomique d'AppImage (avec redémarrage sous Linux), ou l'installateur de paquets d'Android ; les installations hors de ces formats ou un appcast non signé proposent plutôt la page de la version |
| iOS | L'application interroge l'App Store, qui la met à jour, et propose sa page sur la boutique |

Pour une application web autonome publiée, `updater.Check(cx)` compare la version en cours d'exécution avec un `nsgo.json` frais et propose de recharger après confirmation. Elle détecte aussi les fichiers de compilation modifiés sous une même étiquette de version. Elle ne nécessite aucune clé d'appcast ; les canaux explicites `none`/`steam` désactivent toujours les vérifications, et les hôtes intégrés gèrent leurs propres rechargements. Les mini-programmes utilisent le gestionnaire de mises à jour de leur plateforme et ne redémarrent qu'une fois la mise à jour téléchargée prête. Ces cibles vérifient lorsque l'application appelle l'outil de mise à jour ; la planification du téléchargement et de la vérification des mini-programmes appartient à l'hôte. Elles n'exposent aucun modificateur de préférence de vérification automatique. Un gestionnaire personnalisé `cx.CheckForUpdates` utilise `UpdateCheck.Restart` pour une mise à jour préparée dont l'étiquette de version peut être absente.

### Canaux

Le canal détermine qui met à jour l'application. `-channel` remplace `updates.channel` le temps d'une compilation, afin qu'un même projet compile pour chacun :

| Canal | Pour | Les paquets |
| --- | --- | --- |
| `direct` | GitHub, un site web, les casks Homebrew et winget, qui installent les propres fichiers de la version | Se mettent à jour eux-mêmes comme indiqué ci-dessus |
| `store` | L'App Store, le Mac App Store, le Microsoft Store, Google Play, Flathub et d'autres boutiques | Sont les [paquets de boutique](#store-packages) et n'embarquent aucun auto-updater ; iOS et macOS interrogent leur App Store sans clé, et une installation Google Play peut utiliser le flux de mise à jour natif de Play |
| `steam` | Steam | Produisent des [répertoires de dépôt](#steam-depots) mis à jour par Steam |
| `none` | Les distributions Linux et quiconque réempaquette l'application | Ne vérifient rien |

Les boutiques rejettent les applications qui se mettent à jour elles-mêmes ; téléversez donc une compilation `store`, jamais une compilation `direct`. Chaque publication directe comportant des installateurs macOS ou Windows génère aussi un cask Homebrew (`dist/releases/homebrew/<name>.rb`, déclarant `auto_updates true`, les sommes de contrôle et les URL de téléchargement des images disque compilées, ainsi qu'un lien `binary` vers chacune des [commandes](#commands) de l'application) et un manifeste winget multifichier (`dist/releases/winget/<id>.yaml`, `<id>.installer.yaml` et `<id>.locale.en-US.yaml` pour les installateurs NSIS compilés). Partout où le système indique qui a installé l'application, celle-ci le vérifie également : une application Android issue d'une autre boutique, un Flatpak ou Snap, et une compilation TestFlight laissent les mises à jour à cette boutique. Un appel manuel à `updater.Check` dans une installation Google Play demande à Play In-App Updates de proposer son flux immédiat lorsque c'est autorisé ; Play gère le consentement, le téléchargement, l'installation et le redémarrage. Les compilations de développement n'entrent jamais dans ce flux. Les paquets Android autonomes embarquent le SDK ; les [hôtes d'intégration](embedding.md#android) le fournissent via Gradle.

### Vérification depuis l'application

`updater.Check(cx)` vérifie comme le fait un élément de menu *Rechercher des mises à jour…*, et affiche ce qu'elle trouve ; `updater.CheckInBackground(cx)` n'affiche qu'une version plus récente, et `updater.CheckDaily(cx)` ne le fait qu'au plus une fois par jour. `updater.Status(cx)` lit la progression en direct du téléchargement et de l'installation sous Linux et Android. Tant que `automatic` est activé, l'application vérifie ainsi sous Linux, sur les compilations Android directes et sur les compilations Apple App Store au démarrage et chaque fois que l'utilisateur y revient ; une vérification qui échoue, par exemple hors ligne, est retentée une heure plus tard. Les compilations de développement ne se mettent jamais à jour. Dans les compilations Steam, ces trois fonctions ne font rien.

`updater.CapabilityOf(cx)` signale comment cette copie se met à jour sans requête réseau : `ManagedSelf`, `ManagedPage`, `ManagedDevelopment`, `ManagedChannel`, `ManagedStore` (avec son nom `Store`), ou `ManagedUnavailable`. Une compilation de développement, un canal désactivé ou une installation par boutique produit une explication spécifique à la raison lors de la vérification ; `core.UpdateCheck.Err` est réservé aux échecs tels qu'un flux invalide ou un programme de mise à jour embarqué impossible à charger.

Lisez `updater.Status(cx)` dans une liaison pour suivre `Capability`, `AutomaticChecks`, `AutomaticDownloads`, `Available`, `Notes` et `LastCheck`, et sous Linux et Android les champs `Phase`, `Loaded` et `Total` d'une mise à jour (`Fraction()` pour une barre de progression). `updater.SetAutomaticChecks(cx, on)` enregistre le choix de l'utilisateur et modifie le planificateur quotidien à l'exécution. Sparkle et WinSparkle gèrent leurs propres préférences ; Linux, Android et iOS conservent le choix dans une partition de stockage privée. `updater.SetAutomaticDownloads(cx, on)` n'est pris en charge que sous macOS, avec `updates.automatic_downloads` par défaut ; les autres plateformes renvoient `false` et ignorent le modificateur. Les compilations non prises en charge ou gérées en externe renvoient `false` pour les deux réglages. WinSparkle signale les métadonnées de mise à jour trouvée en lisant l'appcast après son rappel de découverte ; il garde toutefois le contrôle de l'installation. Linux résout l'appartenance à un paquet Debian ou Arch ainsi que l'identité AppImage au lancement ; ces copies signalent `ManagedSelf`, tandis que d'autres copies non empaquetées signalent `ManagedPage` avant toute vérification.

## Lancement à l'ouverture de session

`desktop.SetLaunchAtLogin(cx, on)` enregistre l'application auprès du mécanisme d'ouverture de session de la plateforme, et `desktop.LaunchAtLogin(cx, done)` le lit. Un MSIX compilé par l'outil actuel inclut automatiquement une tâche de démarrage désactivée ; son activation respecte l'interrupteur Applications au démarrage de l'utilisateur et la politique d'administrateur. Les installations Windows et Linux ordinaires conservent leur valeur Run et leur entrée XDG autostart. macOS utilise le service natif d'ouverture de session du paquet d'application.

Flatpak sollicite le portail Background, qui peut demander le consentement, sans accorder l'accès aux fichiers de configuration de l'hôte. Une lecture ultérieure attend la réponse native. Le portail ne dispose d'aucune requête de démarrage en lecture seule : les lectures Flatpak renvoient uniquement la dernière réponse confirmée dans cette exécution, `false` avant toute réponse, et ne peuvent pas détecter une modification hors de l'application ni retrouver le réglage lors de l'exécution suivante.

Un Snap empaqueté en externe déclare le nom du fichier de démarrage automatique sur son application :

```yaml
apps:
  app:
    command: bin/app
    autostart: app.desktop
```

NSGo écrit ce nom de fichier sous `$SNAP_USER_DATA/.config/autostart`, que snapd lance via l'enveloppe de l'application. `desktop.LaunchedAtLogin(cx)` distingue un lancement à la connexion avant la construction de l'application, y compris une activation `StartupTask` MSIX.

## Types de fichiers

`file_types` enregistre les documents ouverts par une application : depuis le Finder, le Dock et Ouvrir avec sous macOS, depuis Ouvrir avec de l'Explorateur via l'installateur NSIS ou le MSIX sous Windows, via l'entrée desktop des paquets Debian, Arch, AppImage et Flatpak sous Linux, et via Ouvrir dans sous iOS :

```yaml
file_types:
  - name: Flow File
    extensions: [flow]
    # id: dev.example.hello.flow   # <id>.<first extension> by default; another's ID imports its type
    # icon: assets/Flow.icns       # macOS document icon; or a square PNG
    # role: viewer                 # editor by default
    # conforms: [public.json]      # public.data by default
    # mime: [application/json]     # existing MIME types; otherwise inferred on Linux
```

L'installateur Windows ajoute l'application à Ouvrir avec sans remplacer le choix par défaut de l'utilisateur, et supprime ses enregistrements lors des mises à niveau et désinstallations. Linux nomme les types MIME des extensions courantes à partir d'une table fixe, et attribue à une extension inconnue du type propre de l'application `application/x-<type id>` avec une définition shared-mime-info. `mime` désigne à la place des types existants ; un type importé dont l'extension est absente de la table en a besoin. L'exécution d'une AppImage n'enregistre rien : un outil d'intégration au bureau installe son entrée et ses définitions.

L'application reçoit les fichiers via `desktop.OnOpenFiles`, y compris lors d'un lancement ultérieur sous Windows ou Linux pendant qu'elle tourne et lors d'ouvertures de documents par scène iOS. Les fichiers ayant lancé l'application attendent le premier gestionnaire :

```go
desktop.OnOpenFiles(cx, func(paths []string) { open(paths[0]) })
```

Sous iOS, les documents ordinaires sont importés dans des fichiers temporaires appartenant à l'application avec leurs noms d'origine. Les lectures depuis les fournisseurs de fichiers sont coordonnées hors du fil d'interface, et leurs portées de sécurité se terminent avant l'exécution du gestionnaire. Déplacez un import dans `cx.DataDir()` pour le conserver ; les modifications affectent la copie. iOS utilise l'icône de document de l'application ; `icon` s'applique sous macOS. Les App Clips et les paquets tvOS n'enregistrent aucun type de document. Les ouvertures de fichiers restent distinctes d'`OnOpenLinks`.

`desktop.AddRecentFile(cx, path)` enregistre un document ouvert par l'utilisateur : dans le menu du Dock et Fichier › Ouvrir un élément récent sous macOS, dans la catégorie Récents de la liste de raccourcis (*Jump List*) sous Windows, et dans les fichiers récemment utilisés du bureau sous Linux. Un choix atteint le même gestionnaire. `desktop.ClearRecentFiles` oublie les enregistrements de l'application, tout en conservant l'historique des autres applications pour les fichiers partagés sous Linux ; Linux respecte également le réglage d'historique de GTK.

`desktop.SetDockMenu(cx, items)` définit des actions sur l'icône de l'application : le menu du Dock de macOS, les tâches de la Jump List sous Windows et la *quicklist* sur le dock Ubuntu, Plasma et Dash to Dock sous Linux là où les quicklists sont prises en charge. Linux nécessite une entrée desktop installée correspondant à l'`id` de l'application, et conserve les sous-menus, séparateurs, éléments cochés et désactivés. Le menu appartient à la portée qui l'a défini, et le dernier appelant parmi les fenêtres de l'application le remplace.

## Liens

`url_schemes` dote l'application de ses propres liens, tels que `myapp://settings`, qui l'ouvrent depuis un navigateur, une autre application, `open`, `start` ou `xdg-open`. Les applications macOS, iOS et Android les enregistrent, tout comme l'installateur Windows et le paquet MSIX ainsi que l'entrée desktop Linux transportée par les paquets Debian, Arch, AppImage et Flatpak. La désinstallation les supprime.

```yaml
url_schemes: [myapp]
# app_links: [links.example.com]  # HTTPS hosts for the full app
```

L'application les reçoit via `desktop.OnOpenLinks`, y compris pendant son exécution : sous Windows et Linux, un second lancement transmet son lien à l'application en cours d'exécution et se termine. Un lien ayant lancé l'application attend le premier gestionnaire. N'importe qui peut rédiger un tel lien, et sous Windows et Linux le gestionnaire reçoit toute URL avec laquelle l'application est démarrée ; vérifiez donc le schéma et demandez confirmation à l'utilisateur avant qu'un lien ne modifie quoi que ce soit :

```go
desktop.OnOpenLinks(cx, func(links []string) { show(links[0]) })
```

Sous iOS et Android, `mobile.OnOpenLinks` reçoit les liens au lancement et pendant l'exécution de l'application. Ouvrez-en un avec `xcrun simctl openurl booted myapp://settings` sur le simulateur iOS ou `adb shell am start -a android.intent.action.VIEW -d myapp://settings` sous Android. `app.OpenLinks("myapp://settings")` en délivre un dans un test.

Pour les liens universels iOS, déclarez des hôtes HTTPS explicites avec `app_links: [links.example.com]`. Chaque hôte est un nom DNS sans schéma, chemin, port, adresse IP ni joker ; les noms sont mis en minuscules et les doublons sont rejetés. L'outil écrit `applinks:<host>` dans le droit Associated Domains de l'application complète, y compris pour la signature gérée par Xcode ; un profil IPA doit l'autoriser. Les App Clips conservent les déclarations `appclips:` distinctes issues de `app_clip.domains`.

Chaque hôte doit servir un fichier `apple-app-site-association` nommant l'application complète à l'adresse `https://<host>/.well-known/apple-app-site-association` ; les métadonnées du paquet seules ne peuvent pas autoriser un lien universel. Les [exigences d'association d'Apple](https://developer.apple.com/documentation/xcode/supporting-associated-domains) décrivent la configuration serveur. Les signatures ad hoc de simulateur omettent les droits développeur restreints, de sorte qu'un test de délégué n'établit pas l'association serveur.

Sous iOS, `mobile.OpenedURL(cx)` suit l'invocation d'App Clip, le lien universel ou l'URL de schéma personnalisé le plus récent, y compris la valeur initiale avant le montage de la racine. La livraison de schéma personnalisé continue d'atteindre `mobile.OnOpenLinks` ; la valeur d'invocation est à jour lorsque ce gestionnaire s'exécute. Les ouvertures de fichiers restent dans `OnOpenFiles` et ne remplacent pas l'URL d'invocation. La prise en charge des invocations Android/Web est suivie séparément dans #1475.

## Partages entrants sous Android

Choisissez les types MIME que l'application accepte depuis la feuille de partage Android :

```yaml
android:
  share_types: [text/plain, 'image/*', application/pdf]
```

Les compilations APK et App Bundle enregistrent ces types aussi bien pour un élément unique que pour plusieurs. Enregistrez [`mobile.OnShared`](services.md#opening-links-and-files) pour recevoir chaque partage.

Pour une application web utilisant des adresses fondées sur les chemins, définissez `web.base` sur son répertoire de déploiement (`/` ou `/my-app/`) et configurez votre serveur de production pour qu'il serve `index.html` pour les routes de l'application. Le serveur de développement fournit déjà ce repli pour les requêtes de navigation HTML. Les ressources restent relatives à cette base HTML fixe lors du rechargement d'un lien profond. Voir [Adresses d'application](addresses.md).

## Partages entrants sous iOS

Déclarez les Uniform Type Identifiers acceptés par l'extension de partage, en plaçant les types URL et texte avant les types de fichiers plus généraux :

```yaml
apple:
  share_types: [public.url, public.plain-text, public.image, com.adobe.pdf]
  # app_group: group.com.example.app  # defaults to group.<id>
sign:
  # share_profile: assets/Share.mobileprovision
```

`build` et `dev` empaquettent une extension de partage Go, `<id>.share`, avec l'application. Xcode la signe et enregistre son App Group pour les exécutions sur appareil. Pour un IPA, activez le même App Group pour `<id>` et `<id>.share` ; `sign.profile` et `sign.share_profile` doivent l'accorder. Les App Clips, tvOS et watchOS ne comportent pas d'extension de partage.

L'extension copie jusqu'à 128 éléments et 512 Mio de fichiers dans l'App Group, puis termine la feuille de partage. `mobile.OnShared` reçoit ces lots pendant que l'application est active ou lors de sa prochaine ouverture par l'utilisateur ; ils survivent à la fermeture de l'extension et de l'application avant leur livraison. iOS ne fournit aucune API prise en charge permettant à une extension de partage de mettre au premier plan ou de lancer son application conteneur. Ce transfert n'ouvre donc pas automatiquement l'application.

## Mini-programmes

```sh
go tool nsgo build -target miniprogram_wechat -run
go tool nsgo dev -target miniprogram_alipay
```

Chaque cible écrit un projet pour l'outil de développement de sa plateforme, également dans `dist/releases/miniprogram/<platform>`, que `-run` et `dev` ouvrent ; `dev` recompile à chaque sauvegarde et l'outil recharge :

| Cible | Plateforme | Outil |
| --- | --- | --- |
| `miniprogram_wechat` | WeChat | [WeChat DevTools](https://developers.weixin.qq.com/miniprogram/dev/devtools/download.html), avec son port de service activé (Paramètres, Sécurité) |
| `miniprogram_qq` | QQ | [Outil de développement QQ](https://q.qq.com/wiki/tools/devtool/), sous macOS |
| `miniprogram_alipay` | Alipay | [Mini Program Studio d'Alipay](https://opendocs.alipay.com/mini/ide/download), sous macOS |
| `miniprogram_douyin` | Douyin et autres applications ByteDance | [Outil de développement Douyin](https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/developer-instrument/download/developer-instrument-update-and-download), sous macOS |

`miniprogram.appid` indique l'AppID du mini-programme sur chaque plateforme (`wechat`, `qq`, `alipay`, `douyin`) ; sans cela, WeChat DevTools l'aperçoit avec son AppID touriste et les autres outils en demandent un. Sous WeChat, `-run` et chaque recompilation `dev` ferment puis rouvrent automatiquement le projet via la CLI, afin que DevTools compile les fichiers frais. Cela a été vérifié avec DevTools 2.01.2510290. La barre de navigation de la plateforme affiche le titre de l'application ; avec `custom_navigation`, l'application remplit la page et sa zone sûre la maintient à l'écart du bouton de menu. L'application lit le système du téléphone avec `Env.Simulated` à `true`, comme sur le web avec `--nsgo-os` : les pages d'une pile affichent leur propre `Header`, puisqu'aucun en-tête système ne le remplace. QQ, Alipay et Douyin n'ont pas encore été testés dans leurs outils, et Alipay ne transmet pas encore le retour à l'application, les images clés ni la hauteur du clavier.

Les mini-programmes sont expérimentaux. Un mini-programme n'exécute que les applications Go, pas TSX, et les plateformes limitent le module compressé de l'application à 2 Mo : une application de quelques écrans avec `base` tient dans cette limite, comme `examples/gallery-miniprogram`, ce qui n'est pas encore le cas d'une application utilisant les contrôles `ui`, et `build` signale les dépassements. Les shaders n'y dessinent rien, et les vues web, vidéos, composants natifs ainsi que les fichiers de polices de l'application ne sont pas encore affichés.

## HarmonyOS

```sh
go tool nsgo build -target harmonyos -run
```

`harmonyos` écrit `<name>-<version>-harmonyos.hap` dans `dist/releases`. Le moteur s'exécute en WebAssembly dans ArkWeb, et un hôte ArkTS dessine ce qu'il monte avec des vues ArkUI, de sorte que le texte, les champs, le défilement et les boîtes de dialogue soient ceux du système. Avec `hdc` dans `PATH`, `-run` installe le HAP sur l'appareil ou l'émulateur connecté et le démarre, et `nsgo test -target harmonyos` exécute les tests d'une application depuis un HAP non signé sur un émulateur sous une identité de paquet `.nsgotest` distincte, en supprimant ce HAP de test après l'exécution afin que l'application installée reste disponible.

Pour compiler l'hôte ArkTS à partir des modèles et utiliser l'`id` de l'application comme nom de paquet, placez `hvigorw` dans `PATH` et définissez `OHOS_BASE_SDK_HOME` sur un SDK OpenHarmony avec l'API 26 (`26.0.0`), la version SDK de compilation et de cible des modèles. `build.harmonyos_bundles` choisit `auto` (valeur par défaut), `exact` (qui requiert le SDK), ou `padded` (qui complète l'id à la longueur fixe de l'hôte validé dans le dépôt et ne requiert aucun SDK). Les tests cibles utilisent le même réglage et affichent l'hôte empaqueté. `harmonyos.permissions` liste les noms `ohos.permission` demandés par l'application, tels que `MICROPHONE` et `CAMERA`, et `widget.title` indique ce qu'affiche sa carte sur l'écran d'accueil.

HarmonyOS est un aperçu. Les émulateurs peuvent installer des HAP non signés. [Les notes d'architecture](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/harmonyos.md) dressent la liste des fonctionnalités ArkUI rendues par l'aperçu.

### HAP signés

Définissez `harmonyos.signing` pour signer un HAP avec vos éléments AppGallery Connect. Une installation directe sur un téléphone Huawei nécessite un certificat de débogage et un profil pour cet appareil. Utilisez les éléments de publication pour la distribution. [Guide de signature Huawei](https://developer.huawei.com/consumer/en/doc/harmonyos-guides-V2/command-line-building-app-hap-hvigor-0000001553061909-V2/)

```yaml
harmonyos:
  signing:
    keystore: signing/app.p12
    key_alias: app
    certificate: signing/app.cer
    profile: signing/app.p7b
```

Les chemins sont relatifs à `nsgo.yaml`. Définissez `NSGO_HARMONY_KEYSTORE_PASSWORD` et `NSGO_HARMONY_KEY_PASSWORD` dans l'environnement. La signature utilise `hap-sign-tool.jar` du SDK et Java depuis `JAVA_HOME` ou `PATH`. La compilation conserve l'`id` exact de l'application pour correspondre au profil ; la signature requiert donc le SDK et `hvigorw` même en mode `auto`. `go tool nsgo doctor`, exécuté dans le répertoire de l'application, vérifie la configuration de signature et le nom de paquet du profil sans mots de passe. `build -run` transmet à `hdc` le HAP signé.

## Signature

```yaml
sign:
  identity: "Apple Development: you@example.com (ABCDE12345)"  # signs the macOS app, else ad hoc
  profile: assets/AppStore.mobileprovision  # signs the IPA
  keystore: assets/release.p12              # signs APKs, with key_alias
  certificate: assets/codesign.pfx          # signs the Windows app and its installer
  # widgets_profile: assets/Widgets.mobileprovision  # the iOS extension's profile; use a macOS profile for a Mac App Store build
  # share_profile: assets/Share.mobileprovision      # signs the IPA's share extension
  # clip_profile: assets/Clip.mobileprovision        # signs the IPA's App Clip
```

Les mots de passe du keystore proviennent de `NSGO_KEYSTORE_PASSWORD` et `NSGO_KEY_PASSWORD`. `-team` signe le paquet iPhone. `-profile`, `-widgets-profile`, `-share-profile`, `-clip-profile` et `-macos-profile` remplacent respectivement en ligne de commande les profils de provisionnement de l'application, de l'extension de widgets, de l'extension de partage, de l'App Clip et du Mac App Store. `-installer` remplace l'identité Mac Installer Distribution.

### Signer les paquets Windows

Authenticode identifie l'éditeur d'un téléchargement Windows ; [SmartScreen vérifie sa réputation séparément](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation). `sign.certificate`, un fichier `.pfx`, signe l'exécutable, le compagnon en ligne de commande, les programmes et bibliothèques apportés par les ressources de l'application, l'installateur avec son désinstallateur, ainsi que le MSIX compilé pour le canal Store ; son mot de passe provient de `NSGO_CERTIFICATE_PASSWORD`. Les signatures sont effectuées en Go, comme les produit `signtool`, afin qu'un Mac les signe sans le SDK Windows :

```yaml
sign:
  certificate: assets/codesign.pfx
  timestamp_url: http://timestamp.digicert.com  # the RFC 3161 server that timestamps them
```

`sign.timestamp_url` utilise par défaut celui de DigiCert, et `none` laisse les signatures sans horodatage, ce qui entraîne leur expiration en même temps que le certificat. Un certificat stocké sur un jeton matériel ou dans un service cloud signe à la place via une commande dédiée, dans laquelle `%1` représente le fichier à signer et que la compilation exécute pour chacun des fichiers ci-dessus :

```yaml
sign:
  sign_command: "signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a %1"
```

`build` s'arrête si la commande réussit mais laisse l'exécutable de l'application, le compagnon de commande, une ressource PE ou l'exécutable d'installation sans signature Authenticode intégrée.

La signature du désinstallateur à l'intérieur de l'installateur nécessite NSIS 3.08 ou ultérieur, et un certificat qui n'est pas un `.pfx` ne peut pas le signer via `sign.certificate`. `-certificate`, `-sign-command` et `-timestamp-url` les définissent depuis la ligne de commande.

### Notariser

Les applications téléchargées sur Mac s'ouvrent sans l'avertissement de Gatekeeper une fois qu'Apple les a notarisées. Signez avec un Developer ID et indiquez le nom des identifiants conservés par `notarytool` dans votre trousseau :

```sh
xcrun notarytool store-credentials hello --apple-id you@example.com --team-id ABCDE12345
```

```yaml
sign:
  identity: "Developer ID Application: Example Inc (ABCDE12345)"
  notarize: hello   # the keychain profile above; or -notarize hello
```

`build` signe alors avec le runtime renforcé, soumet l'image disque, attend le verdict d'Apple — quelques minutes — et agrafe le ticket à l'image et à l'application ; un rejet affiche le journal d'Apple. En CI, `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` et `NSGO_NOTARY_ISSUER` — le fichier `.p8`, l'ID et l'émetteur d'une clé API App Store Connect — remplacent le profil. Le runtime renforcé interdit l'accès au microphone, à la caméra, aux calendriers et à la localisation à moins que l'application ne les déclare, ce que fait `build` lorsque `apple.info` contient `NSMicrophoneUsageDescription`, `NSCameraUsageDescription`, une description d'utilisation des calendriers ou rappels, ou une description d'utilisation de la localisation.

## Widgets et Activités en direct

`build` et `dev` compilent directement l'extension qui dessine les widgets et Activités en direct de l'application dans les paquets macOS et iOS à l'aide du compilateur Swift de Xcode, sans étape séparée. Sous iOS, l'application et ses widgets partagent un groupe d'applications (*app group*) : `-run` sur iPhone fait signer les deux par Xcode et enregistre le groupe ; pour un IPA, activez-le pour `<id>` et `<id>.widgets`, et signez l'extension avec `sign.widgets_profile`. Les APK et AAB Android autonomes incluent les fournisseurs et ressources App Widget issus de `widgets` ; le runtime Java précompilé dessine leurs chronologies sans qu'aucun SDK ni compilateur Java ne soit nécessaire lors des compilations d'applications. Android 16 affiche les Activités en direct sous forme de Live Updates, qui ne nécessitent aucune extension. [Widgets](widgets.md) et [Activités en direct](live-activities.md) couvrent leur configuration et leur API.

## Android App Links

`app_links` nomme des hôtes HTTPS explicites dont les liens ouvrent l'application Android complète :

```yaml
app_links: [example.com, shop.example.com]
```

L'APK et l'App Bundle déclarent un filtre `ACTION_VIEW` navigable avec `autoVerify` pour chaque hôte. Utilisez des hôtes DNS sans schéma, chemin, port, joker ni adresse IP. Servez `https://<host>/.well-known/assetlinks.json` avec l'identifiant de paquet de l'application et l'empreinte de son certificat de signature ; Android vérifie cette association avant d'acheminer les liens vers l'application. L'outil ne publie pas de fichiers d'association ; le [guide de vérification Android](https://developer.android.com/training/app-links/verify-applinks) décrit leur mise en place. Un hôte Android intégré déclare ses propres filtres d'intention et transmet les liens acceptés à sa `NsgoView`.

`mobile.OpenedURL(cx)` lit l'invocation acceptée avant le premier rendu de la racine et se met à jour lorsqu'un autre lien ouvre l'application en cours d'exécution. `mobile.OnOpenLinks` continue de recevoir chaque lien accepté. Une application web occupant toute la page lit l'adresse courante de sa page via `mobile.OpenedURL`, y compris les changements d'historique du navigateur et de fragment ; une application montée dans une autre page ne lit aucune adresse de la page englobante. Utilisez [`cx.AppURL`](addresses.md) lorsque l'application modifie également son adresse.

## App Clips

Un App Clip est une petite partie d'une application iOS qui s'exécute depuis un lien, un code ou une balise NFC sans installer l'application complète. `app_clip` en compile un dans le paquet iOS, à partir d'un paquet `main` dédié afin de ne lier que ce qu'il affiche :

```yaml
app_clip:
  main: ./clip              # the clip's main package, the app's own by default
  domains: [example.com]    # whose links open it
  # name: Hello Clip        # "<name> Clip" by default
  # id: dev.example.hello.Clip  # <id>.Clip by default
  # minimum: "17.0"         # 16.0 by default; 17.0 allows 100 MB when only links open it
  # notifications: true     # ask for notifications for a day
  # location: true          # let the clip confirm where it was opened
```

Le clip lit le lien qui l'a ouvert via `mobile.OpenedURL`, qui change lorsqu'un autre lien l'ouvre à nouveau, et `mobile.IsAppClip` indique si le code s'exécute en tant que tel. Le système télécharge un clip d'au plus 15 Mo, ou 100 Mo à partir d'iOS 17 lorsque seuls des liens l'ouvrent, et `build` refuse tout clip plus volumineux ; un clip liant `ui` pèse environ 11 Mo. `dev -target ios_simulator -clip https://example.com/order?table=12` exécute le clip seul dans le simulateur, ouvert depuis ce lien ; `-target ios` l'exécute sur un iPhone et affiche le code QR du lien : ajoutez le lien dans Réglages > Développeur > Expériences locales, puis scannez-le avec l'appareil photo. Utilisez `-release` si les symboles de développement font dépasser la limite de taille du code QR au clip. Apple ne signe les App Clips que pour les équipes membres de l'Apple Developer Program, pas pour les équipes personnelles.

`dev -target ios` et `build -run -target ios` conservent aussi le clip à l'intérieur de l'application complète : Xcode le signe comme une cible séparée et l'intègre avant de signer l'application. La signature automatique nécessite un compte d'équipe payant authentifié dans Xcode, avec provisionnement pour l'application et le clip. Pour l'App Store, activez les App Clips pour `<id>`, signez le clip avec `sign.clip_profile` et servez le fichier `apple-app-site-association` des domaines. L'exemple `examples/appclip` du dépôt est un café dont le clip permet de commander à une table.

Pour transmettre une commande ou d'autres données locales à l'application complète, définissez un groupe explicite aux côtés d'`app_clip` :

```yaml
apple:
  app_group: group.dev.example.hello
```

Le clip et l'application complète partagent alors `cx.DataDir()` et `cx.Storage()` dans le conteneur du groupe. Les fichiers sous le répertoire de données et les entrées clé-valeur écrits par le clip sont disponibles au démarrage de l'application complète ; `cx.LogsDir()` suit ce répertoire, tandis que chaque paquet conserve son propre cache. La signature Xcode revendique le groupe pour les deux cibles ; lors de l'utilisation de profils, activez le même groupe pour les deux App ID et accordez-le dans `sign.profile` et `sign.clip_profile`. Une autorisation manquante fait échouer la signature, et un groupe inaccessible empêche le démarrage. Le répertoire de données explicite d'un hôte conserve toutefois la priorité.

Sans `apple.app_group`, chaque paquet conserve son propre stockage en bac à sable. Activer ce réglage ne migre pas les données précédemment stockées dans ces bacs à sable, et les applications contenant uniquement des widgets conservent leur emplacement de stockage existant. Conservez les secrets partagés dans le trousseau plutôt que dans le stockage en clair. L'exemple du café enregistre la commande de son clip et l'affiche dans l'application complète.

Depuis une action explicite de l'utilisateur à l'intérieur du clip, appelez `mobile.ShowAppClipOverlay(cx, mobile.AppClipOverlayBottom)` pour demander à StoreKit de recommander son application complète correspondante. `AppClipOverlayBottomRaised` laisse de la place pour une barre inférieure. La fonction retournée annule cette requête ; le nettoyage de la portée l'annule également, et une requête plus ancienne ne peut pas annuler une requête plus récente. Les appels hors d'un App Clip iOS natif ne font rien. L'App Store décide s'il peut charger et afficher la recommandation ; l'appel ne signale donc ni l'affichage d'une offre ni une installation. Gardez la tâche du clip utilisable sans installer l'application complète. [Recommandations d'Apple pour la superposition](https://developer.apple.com/documentation/appclip/recommending-your-app-to-app-clip-users)

## Intégrer dans une autre application

`-embed` empaquette l'application pour une application iOS ou Android qui vous appartient : un XCFramework — composé de frameworks dynamiques avec `-dynamic` — ou un AAR dont `NSGoView` ou `NsgoView` l'affiche. Les programmes Go de bureau affichent des applications avec `nsgo.NewView`, et les pages web avec le fichier `nsgo.js` du site, dont l'hôte est celui de la page ou celui nommé par `web.host` dans `nsgo.yaml`. Les exemples `examples/embed-*` du dépôt illustrent chaque cas ; [Intégration](embedding.md) couvre le côté hôte.

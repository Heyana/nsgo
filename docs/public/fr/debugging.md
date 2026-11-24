---
title: Exécution et débogage
description: Exécuter une application sur n'importe quelle cible avec rechargement à chaud, et observer ses erreurs, mises à jour, mise en page, performances et autres langues.
group: Chaîne d’outils
order: 20
---

## Exécuter

```sh
go tool nsgo dev
```

`dev` compile l'application pour cette machine et l'ouvre, ou pour la cible nommée par `dev.target` dans `nsgo.yaml`. À chaque enregistrement d'un fichier Go, il recompile et redémarre l'application, en exécutant d'abord `go mod tidy` lorsque l'application importe un paquet dont le module est absent de `go.mod` ou `go.sum` ; une modification du TSX seul met à jour l'application en cours d'exécution sur place tout en conservant son état. L'exécution se termine lorsque l'application se ferme.

Sur macOS et Windows, une recompilation native en mode dev rouvre la fenêtre de la vue principale à sa dernière position et taille normales au sein de la session courante. Windows restaure également le plein écran. Sur Linux, GTK restaure sa taille normale et demande son dernier mode plein écran. Le gestionnaire de fenêtres choisit la position.

`-target` l'exécute ailleurs, et `-device` sélectionne l'iPhone, l'Apple TV, l'Apple Watch, le simulateur ou l'appareil Android par nom, UDID ou numéro de série :

```sh
go tool nsgo dev -target tui                # terminal cells; diagnostics in .nsgo/tui.log
go tool nsgo dev -target web                # in the browser, at the address it prints, on -port (8080 by default)
go tool nsgo dev -target ios_simulator      # in the iOS simulator
go tool nsgo dev -target ios                # on the connected iPhone, signed by Xcode
go tool nsgo dev -target tvos_simulator     # in the Apple TV simulator
go tool nsgo dev -target watchos_simulator  # in the Apple Watch simulator
go tool nsgo dev -target android            # on a connected device or emulator, over adb
go tool nsgo dev -target miniprogram_wechat # in WeChat DevTools (also _qq, _alipay, _douyin)
```

Le serveur de développement écoute sur toutes les interfaces par défaut. Utilisez `-host 127.0.0.1` pour le restreindre à la boucle locale, y compris lorsque Android se connecte via `adb reverse`, ou indiquez l'adresse IP d'une interface locale pour n'écouter que sur celle-ci. Utilisez une adresse joignable sur le réseau local pour les fonctionnalités de développement sur des appareils Apple physiques.

`-h` liste tous les drapeaux.

## Débogueur Go

Placez une version de [Delve](https://github.com/go-delve/delve/blob/master/Documentation/installation/README.md) compatible avec votre version de Go dans `PATH` sous le nom `dlv`. Démarrez une application desktop native avec ses informations de débogage et les optimisations Go désactivées.

```sh
go tool nsgo dev -target native -debug
```

Copiez le port depuis la ligne `API server listening at` de Delve. Dans VS Code avec l'extension Go, ajoutez une configuration d'attachement et remplacez `40000` par ce port.

```json
{
  "name": "Attach to NSGo",
  "type": "go",
  "request": "attach",
  "mode": "remote",
  "debugAdapter": "dlv-dap",
  "host": "127.0.0.1",
  "port": 40000
}
```

Dans GoLand, ajoutez une configuration d'exécution **Go Remote** avec l'hôte `127.0.0.1` et le même port. Posez un point d'arrêt dans le code Go de votre application, attachez le débogueur et déclenchez l'action qui l'atteint. Après une recompilation Go réussie, reconnectez le débogueur à la même adresse afin qu'il envoie les points d'arrêt au nouveau processus. Les modifications TSX utilisent toujours le rechargement à chaud.

`-debug` fonctionne sur macOS, Windows et Linux avec `-target native`. Utilisez une compilation dev pour déboguer et réservez `-release` pour mesurer une compilation de production.

## Aperçus de composants

Placez une ou plusieurs fonctions `PreviewXxx` à côté des composants dans un paquet Go importable. Enregistrez-les dans un fichier Go protégé par le tag de compilation preview.

```go
//go:build nsgopreview

package components

import "github.com/nsgo-dev/nsgo/core"

func PreviewEmpty(cx *core.Scope) core.Node {
    return core.Text{Content: "No items"}
}
```

```sh
go tool nsgo dev -preview ./components
go tool nsgo dev -target web -preview ./components
```

Cochez les fixtures que vous souhaitez afficher ensemble et choisissez une locale, un thème, une apparence claire ou sombre, une taille de zone d'affichage et un sens de mise en page. L'aperçu utilise le fichier `nsgo.yaml` de l'application la plus proche, ses catalogues de traduction, son `theme` configuré et ses ressources. Passez un répertoire d'application après les drapeaux pour sélectionner une autre configuration dans le même module. Dans ce cas, le chemin de l'aperçu est relatif au répertoire de l'application.

L'enregistrement d'un fichier recompile l'aperçu tout en conservant votre sélection et vos réglages d'affichage. Le tag `nsgopreview` exclut les fixtures des compilations release ordinaires. Utilisez la même fixture dans un fichier `_test.go` tagué avec `nsgotest.Start(t, PreviewEmpty)` et exécutez-le avec `GOFLAGS=-tags=nsgopreview go tool nsgo test -target macos ./components` pour prendre une capture d'écran native.

Après la première installation de l'application principale avec `-target ios_simulator`, une recompilation Go seule remplace l'exécutable installé et relance l'application. La modification de `nsgo.yaml`, des ressources empaquetées ou de l'icône de l'application déclenche une réinstallation.

Utilisez `go tool nsgo dev -target android -android-host` pour travailler sur l'interface Android avec une boucle de recompilation Go plus courte. L'outil installe le runtime natif une seule fois et exécute Go sur la machine de développement. Chaque recompilation Go se reconnecte à la même Activity via adb. Les modifications apportées aux ressources empaquetées, aux classes précompilées, aux icônes de lancement ou à `nsgo.yaml` réinstallent l'APK du runtime. Le processus Go utilise les contraintes de compilation, les fichiers et le réseau de l'hôte. `Scope.DiskSpace` signale le volume contenant le `DataDir` de l'hôte. Utilisez la commande dev Android standard pour tester les fichiers `_android.go` et le comportement Go sur appareil.

## Erreurs

Une compilation échouée s'affiche par-dessus l'application en cours d'exécution, avec des boutons pour copier l'erreur et relancer la compilation ; corrigez le fichier et l'enregistrement suivant le remplace. Un composant qui provoque un panic lors du rendu s'affiche de la même façon. Command+R dans l'application, Control+R hors des plateformes Apple, la recompile et la redémarre à tout moment, même par-dessus l'erreur, sauf si l'application lie cette touche elle-même avec `cx.OnKey` ou via un élément actif de son menu d'application ; dans le navigateur, la touche recharge la page comme d'habitude. La propre sortie de l'application, telle que `log.Print`, apparaît dans le terminal sous les lignes de l'outil.

Avant l'exécution, `go tool nsgo check` vérifie l'application et contrôle les types de son TSX ; `-target all` la vérifie pour chaque cible, de sorte que le code situé derrière les tags de compilation d'une autre plateforme est également vérifié depuis un Mac.

### Journaux des applications Apple empaquetées

Sur macOS, iOS, tvOS et watchOS, le logger Go par défaut écrit dans le journal unifié d'Apple, y compris `log.Print`, les diagnostics NSGo et la sortie console TSX. Dans Console, filtrez par l'identifiant de bundle de l'application en cours d'exécution comme sous-système (`subsystem`) et `nsgo` comme catégorie. Sur macOS, vous pouvez également interroger le magasin de journaux :

```sh
log show --last 10m --predicate 'subsystem == "dev.example.hello"'
```

Les enregistrements utilisent le type de journal par défaut et conservent leurs préfixes d'avertissement ou d'erreur. Leur texte est public : gardez donc les identifiants et autres secrets hors des journaux. Les messages longs donnent lieu à plusieurs entrées. La sortie terminal reste disponible pendant le développement ; un writer de log personnalisé ou un gestionnaire `slog` contrôle sa propre sortie.

### Vues web Linux

Ubuntu 24.04 et les versions ultérieures peuvent restreindre les espaces de noms utilisateur non privilégiés via AppArmor. Lorsque bubblewrap ne parvient pas à démarrer le bac à sable de WebKitGTK, NSGo journalise l'erreur et affiche un emplacement réservé. Certaines versions de WebKitGTK se terminent sinon avec `SIGTRAP` lors de l'ouverture de la première vue web. [Documentation AppArmor d'Ubuntu](https://documentation.ubuntu.com/security/security-features/privilege-restriction/apparmor/)

Utilisez le profil AppArmor pour bubblewrap de votre distribution s'il en existe un, tel que `bwrap-userns-restrict`. Sinon, un administrateur peut charger un profil accordant `userns` à `/usr/bin/bwrap` en partant du [profil issu de la CI Linux de NSGo](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/test.yml). L'administrateur gère ce profil car il s'applique à toutes les applications utilisant le bubblewrap du système.

`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1` désactive le bac à sable des sous-processus WebKit. Ne l'utilisez que pour le développement local avec du contenu de confiance.

## DevTools

```sh
go tool nsgo dev -devtools
```

`-devtools` ouvre les DevTools dans le navigateur, servis par `nsgo dev` pour chaque cible, y compris pour une application sur téléphone. Ils montrent l'application pendant son exécution :

- **Elements** : les vues sous forme d'arbre. Chacune affiche le composant qui l'a construite, un lien vers sa ligne dans votre éditeur, le sélecteur par lequel un test la trouve, ses classes, ses limites, sa marge externe, sa bordure et sa marge interne telles que mises en page, ainsi que le style qu'elle définit. Survoler une ligne dessine sa marge externe, sa marge interne et son contenu dans l'application, comme le font les DevTools d'un navigateur, et **Pick** vous permet d'appuyer sur une vue dans n'importe quelle fenêtre, panneau, feuille ou vue intégrée pour la localiser dans l'arbre.
- **State** : les signaux créés par votre code, regroupés sous les composants et calculs qui les possèdent, avec leurs valeurs, les valeurs des mémos et le nombre de calculs qui lisent chacun d'eux. Une valeur qu'aucun calcul ne lit ne modifie rien à l'écran. Les chaînes, booléens et nombres peuvent être édités, afin d'essayer un état sans modifier le code. Les valeurs créées par NSGo pour son propre usage se trouvent sous **NSGo's own**.
- **Profiler** : le temps de chaque commit, réparti entre votre code qui le précède, le commit avec sa mise en page, et son application sur le thread principal de la plateforme. Il indique également les lignes dont les écritures ont déclenché le commit, et s'ouvre sur les calculs exécutés, du plus long au plus court. Une seconde liste nomme les calculs qui se sont exécutés le plus souvent durant la dernière seconde, par la ligne qui les a créés. Le runtime d'Android applique les lots dans son propre code Java ; les commits Android n'ont donc pas de temps plateforme.
- **Recorder** : ce que vous appuyez, saisissez, effacez et faites défiler dans l'application, généré sous forme de fichier de test `nsgotest` prêt à copier dans un fichier `_test.go`. Dans Elements, **Assert shown** et **Assert text** ajoutent une vérification de la vue sélectionnée. Une vérification de texte conserve le texte dans la langue affichée par l'application : enregistrez donc dans la langue utilisée par vos tests.
- **Highlight updates** encadre chaque vue modifiée par un commit, comme le fait `-highlight`.
- **Layout** affiche et masque l'[audit de mise en page](#layout) sur l'appareil, comme le lance `-layout`.

L'état, les calculs et les écritures d'une application TSX sont nommés d'après leurs lignes TSX, grâce aux source maps des bundles, et renvoient directement aux fichiers de votre application.

Les DevTools lisent l'application via le serveur de `nsgo dev`, qui écoute sur votre réseau afin que les téléphones puissent le joindre. Chaque exécution génère un jeton que les adresses affichées et ouvertes par `nsgo dev` pour les DevTools et la cible web portent après `#`, et le serveur ne répond qu'aux requêtes qui le contiennent : ouvrez donc les DevTools et la cible web depuis ces adresses. Toute personne à qui vous communiquez l'une de ces adresses peut modifier l'état de l'application pendant son exécution et ajouter des étapes au test généré par Recorder ; relisez donc un test enregistré avant de l'exécuter. Les autres personnes sur votre réseau et les pages web d'autres sites que vous visitez n'ont accès à rien de tout cela.

## Mises à jour

```sh
go tool nsgo dev -highlight
```

`-highlight` encadre pendant une demi-seconde chaque vue modifiée par un commit, afin qu'une mise à jour qui redessine plus qu'elle ne le devrait apparaisse à l'écran pendant que vous utilisez l'application. Les contours s'affichent par-dessus chaque fenêtre, panneau, modale, feuille iOS et vue intégrée (`nsgo.NewView`). Les DevTools permettent de les activer et de les désactiver pendant l'exécution de l'application.

```sh
go tool nsgo dev -trace
```

`-trace` affiche chaque commit de l'application en cours d'exécution : son numéro, sa durée et les éléments de montage envoyés, afin de voir quelles vues une mise à jour a créées, modifiées ou supprimées. Au-dessus des éléments, il indique l'origine du commit : les lignes de votre code qui ont écrit dans des signaux depuis le dernier commit, en commençant par celle qui a écrit le plus souvent, ainsi que le nombre de liaisons, d'effets et de mémos exécutés.

```text
── commit 12: 40 items, commit 1.65 ms
   after app/market.go:473 ×4, app/market.go:487 ×4, app/market.go:384
   ran 118 bindings, 18 effects, 5 memos
```

Une ligne qui écrit de nombreuses fois, ou de nombreuses liaisons exécutées pour une seule écriture, indique l'endroit où regrouper les écritures (`cx.Batch`) ou restreindre ce qu'une liaison lit. `go tool nsgo test -trace` affiche la même chose sous chaque étape d'un test.

## Profils d'exécution

Capturez l'activité CPU Go, le tas ou une trace d'exécution d'une application en cours d'exécution avec `nsgo dev`. L'application envoie la capture au serveur de développement, qui l'enregistre sur la machine exécutant la commande.

```sh
go tool nsgo dev -target android -profile cpu -profile-duration 10s
go tool pprof -top .nsgo/profiles/cpu.pprof
```

L'exemple CPU fonctionne également sur ordinateur de bureau, iPhone ou simulateur iOS. Sur iPhone, autorisez l'accès au réseau local si le système le demande. Le profil de tas est un instantané et fonctionne aussi sur le web. Pour les événements de l'ordonnanceur et des goroutines, exécutez `go tool nsgo dev -profile trace` et ouvrez `.nsgo/profiles/execution.trace` avec `go tool trace`. Les captures CPU et d'exécution durent 10 secondes par défaut. `-profile-output` définit le chemin du fichier local. Pour les aperçus de composants, le chemin par défaut et les chemins relatifs utilisent le répertoire de l'application. Une session dev enregistre sa première capture complète, puis poursuit l'exécution de l'application.

```sh
go tool nsgo dev -target web -profile heap
go tool pprof -top .nsgo/profiles/heap.pprof
```

Le runtime navigateur de Go prend en charge les profils de tas et les traces d'exécution. Pour l'échantillonnage CPU dans le navigateur, utilisez les outils Performance du navigateur. Choisissez soit `-profile`, soit `-release`. La compilation de production avec `nsgo build` exclut le client de capture dev. [Diagnostics du runtime Go](https://go.dev/doc/diagnostics)

## Mise en page

```sh
go tool nsgo dev -layout
go tool nsgo dev -layout=bounds,grid=4
```

`-layout` démarre l'application avec l'audit de mise en page des DevTools par-dessus chaque fenêtre, panneau, feuille iOS et vue intégrée, qui trace le contour de chaque vue : sa boîte de bordure en bleu, le bord intérieur de sa marge interne en vert et le bord extérieur de sa marge externe en orange, chacun étant découpé là où une vue défilante ou une vue qui rogne son contenu le masque. `grid` dessine une grille de 8 points, et `grid=N` une grille de N points, au minimum 4. Indiquez la valeur après `=` : le drapeau seul signifie `bounds`.

Un indicateur dans le coin inférieur affiche la taille de la fenêtre en points, accompagné de boutons pour activer ou désactiver les boîtes, la grille et la règle. Sur Apple TV et d'autres appareils contrôlés uniquement à la télécommande, le focus atteint les boutons de l'indicateur afin que vous puissiez les basculer à la télécommande sans quitter l'application. Tant que la règle est activée, les appuis sélectionnent deux points au lieu d'atteindre l'application, et l'indicateur affiche la distance qui les sépare horizontalement et verticalement ; un troisième appui recommence. Sur Apple TV ou avec un clavier, les flèches directionnelles (ou un glissement sur la surface tactile de la Siri Remote) déplacent un réticule à l'écran et Sélection, Entrée ou Espace choisit les points, mettant à jour la distance en direct jusqu'à ce que le second point la verrouille.

| Touche | Effet |
| --- | --- |
| `Cmd+Option+Shift+L`, `Ctrl+Alt+Shift+L` | Affiche ou masque l'audit, dans une compilation avec `-layout` |
| Flèches, pavé directionnel / glissement de télécommande | Déplace le réticule de la règle du pas de la grille (8 pt par défaut) |
| `Shift` + Flèches | Déplace le réticule de la règle de 1 pt |
| Sélection, `Return`, `Space` | Choisit le point du réticule pendant la mesure |
| `Esc`, retour, Menu | Arrête la mesure |

Si votre application lie la même touche, la liaison de l'application l'emporte. Sans la touche, comme sur un téléphone, lancez également avec `-devtools` et affichez ou masquez l'audit avec **Layout** dans les DevTools ; masquez-le pour atteindre un contrôle situé sous l'indicateur. Les backends natifs et le web lisent les matrices de présentation afin que les limites, Pick et l'audit suivent les transformations affichées, les animations et le défilement ; watchOS suit son dessin affine. HarmonyOS et les mini-programmes conservent le suivi côté Go jusqu'à ce que leurs couches de rendu rapportent la géométrie. Les redessiner consomme du temps à chaque mise à jour : mesurez donc les performances sans eux.

## Vitesse

```sh
go tool nsgo dev -fps
go tool nsgo dev -release -fps
```

`-fps` affiche le moniteur de performances par-dessus l'application sur macOS, iOS, Windows, Linux et Android. Il mesure les images et les commits, et Android affiche également le temps d'aller-retour natif du texte. Les compilations dev embarquent le client de développement ; mesurez avec `-release`, qui compile comme le fait `nsgo build`.

## Langues et apparence

```sh
go tool nsgo dev -lang zh-Hans
go tool nsgo dev -lang en-XA   # accented and longer, to find untranslated and cut text
go tool nsgo dev -lang ar-XB   # right to left
```

`-lang` démarre l'application dans une locale, y compris une pseudo-locale ([Localisation](localization.md)).

## Agents

```sh
go tool nsgo dev -automation
go tool nsgo dev -target android -automation
go tool nsgo dev -target web -automation
```

`-automation` permet à un agent IA ou à un lanceur de tests de bout en bout d'utiliser l'application en cours d'exécution comme le fait une personne, sur macOS, Windows, Linux (`-target native`), Android (`-target android`), le web (`-target web`), les mini-programmes dans les simulateurs de leurs outils de développement (`-target miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` ou `miniprogram_douyin`) ainsi que les appareils et simulateurs Apple (`-target ios`, `tvos`, `watchos`, `ios_simulator`, `tvos_simulator` ou `watchos_simulator`) à ce jour : lister ses fenêtres, décrire ce que l'une affiche, appuyer, saisir, envoyer des touches, faire défiler, glisser, survoler, choisir des éléments de menu, répondre aux alertes, attendre ce qui apparaît et prendre des captures d'écran. Les agents y accèdent via MCP avec `go tool nsgo mcp`, qui utilise le `dev -automation` en cours d'exécution pour le répertoire, ou en démarre un avec les mêmes drapeaux (tels que `-target android`) et l'arrête lorsque le client se déconnecte :

```sh
claude mcp add nsgo -- go tool nsgo mcp
```

Les autres clients acceptent la même commande, `go` avec les arguments `tool nsgo mcp`, exécutée dans le répertoire de l'application. Les vues sont désignées par les sélecteurs des [tests](testing.md#a-test), ou par les références `view:` qu'un instantané affiche, lesquelles deviennent obsolètes dès qu'un enregistrement redémarre l'application.

L'agent agit sur les vraies données de l'application, telles que `dev` les exécute. Seules les compilations `dev -automation` l'incluent, et seuls les outils sur cette machine détenant le jeton dans `.nsgo/automation.yaml`, que vous seul pouvez lire, y ont accès.

### Outils MCP définis par l'application

Appelez `automation.Register` depuis une application Go ou une bibliothèque sur la goroutine du moteur. Par exemple, cette fonction fournit aux agents l'identifiant de la scène actuellement affichée.

```go
import (
    "context"
    "encoding/json"

    "github.com/nsgo-dev/nsgo/automation"
    "github.com/nsgo-dev/nsgo/core"
)

func registerSceneTool(cx *core.Scope, sceneID *core.Signal[string]) error {
    return automation.Register(cx, automation.Tool{
        Name:        "current_scene",
        Description: "Read the ID of the active scene",
        InputSchema: json.RawMessage(`{"type":"object"}`),
        Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
            return map[string]string{"id": sceneID.Get()}, nil
        },
    })
}
```

Le nom doit être unique sur le graphe de l'application et distinct des noms intégrés. `nsgo mcp` liste l'outil jusqu'à ce que le scope soit nettoyé. Le gestionnaire s'exécute sur la goroutine du moteur, ce qui lui permet de lire ou de modifier des signaux. Il reçoit un objet JSON, vérifie les arguments dont il a besoin et retourne une valeur sérialisable en JSON ou une erreur.

Dans un test, capturez le scope au démarrage de l'application, appelez `automation.Call(cx, ctx, name, args)` dans `app.Post`, puis utilisez `app.Settle()` avant de vérifier le résultat ou la vue. L'enregistrement est actif dans `dev -automation` et les tests, et sans effet dans les compilations release.

## Drapeaux de toute compilation

Toute compilation d'une application, paquets inclus, accepte les propres drapeaux de NSGo au démarrage, qui prennent le pas sur `nsgo.yaml` et le code de l'application et n'atteignent jamais les propres drapeaux de l'application :

| Drapeau | Effet |
| --- | --- |
| `--nsgo-fps` | Affiche le moniteur de performances sur macOS, iOS, Windows, Linux et Android |
| `--nsgo-lang=zh-Hans` | Démarre l'application dans une locale, y compris une pseudo-locale |
| `--nsgo-appearance=dark` | Démarre l'application en mode `light`, `dark` ou suivant `system` |
| `--nsgo-os=ios` | Exécute une compilation web comme `macos`, `windows`, `linux`, `ios` ou `android`, `web` étant lui-même |

```sh
./Hello --nsgo-fps                                   # Windows and Linux
open Hello.app --args --nsgo-fps                     # macOS
xcrun simctl launch booted dev.example.hello --nsgo-fps
adb shell am start -n dev.example.hello/dev.oneo.nsgo.NsgoActivity --ez nsgo-fps true --es nsgo-lang de
```

Sur le web, ils se placent dans la chaîne de requête de la page : `?nsgo-fps&nsgo-lang=de`.

`--nsgo-os` prévisualise la mise en page d'une autre plateforme dans un navigateur : l'application lit cet OS et son idiome, un téléphone pour iOS et Android et un ordinateur de bureau sinon, et `Env.Simulated` vaut true. Le navigateur ne dispose d'aucun des éléments natifs de cette plateforme : `NavHeader` et la barre d'onglets dessinée remplacent ceux du système, il n'y a pas de menu d'application, pas de menu contextuel natif (`desktop.PopupMenu`) ni de fonctions de fenêtre, et `mobile.IsAvailable` ainsi que `desktop.IsAvailable` valent false. Les menus dessinés tels que `ui.ContextMenu` et `ui.Menu` continuent de fonctionner. Les raccourcis suivent le clavier du visiteur. Les autres compilations ignorent ce drapeau et journalisent un avertissement.

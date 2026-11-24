---
title: Composants et état
description: Éléments, composants, signaux et styles.
group: Concepts clés
order: 50
---

## Éléments

L'écran est un arbre d'éléments, qui sont des valeurs de structures Go. `core` fournit les primitives : `View`, `Text`, `Image`, `Scroll`, `Input` et d'autres encore. `ui` propose les contrôles qu'une application utilise, tels que `Button`, `Select`, `DatePicker` et `EmojiPicker`, avec une apparence qui suit le thème. (`base` contient les mêmes contrôles sans apparence, pour les [auteurs de composants](authoring/index.md).)

```go
core.View{
	Style: core.Style{Padding: core.All(24), Gap: 12},
	Children: []core.Node{
		core.Text{Content: "Hello"},
	},
}
```

`core.WebView` affiche du contenu web ; une `WebSource` vide charge `about:blank` et signale les événements habituels de chargement et de navigation. Définissez `SourceFunc` sur une source vide ou appelez `Ref.Load(core.WebSource{})` pour effacer la page précédente. Définissez `InjectedJavaScriptBeforeContentFunc` lorsque le script exécuté avant les propres scripts de la page dépend d'un signal : lorsqu'il change, la vue recharge sa source avec le nouveau script à la place de la page affichée, même si l'utilisateur ou `Ref.Load` a navigué ailleurs. Définissez `UserAgentSuffix` (en TSX `userAgentSuffix`) pour ajouter un jeton, tel que `Version/26.0 Safari/605.1.15`, après l'agent utilisateur propre à la plateforme, ou après `UserAgent` lorsque celui-ci est défini ; sur macOS et iOS, un suffixe ajouté à celui de la plateforme ne prend effet qu'à la création initiale de la vue, et HarmonyOS ainsi que le web conservent leur propre agent utilisateur. Attachez une `core.Ref` pour appeler `Reload`, `GoBack` et `GoForward`. Sur les cibles natives, ces commandes maintiennent les propres sources HTML et de données de l'application exemptées de `core.AccessWebSchemes` ; les pages qui créent de nouvelles navigations restent soumises à la politique d'accès.

Sur tvOS, `cx.Env().HasWebView()` vaut false. Monter une `WebView` signale une erreur `*core.WebError` via `OnError` sans charger WebKit ; prévoyez un contenu applicatif spécifique pour cette cible.

Sur macOS, iOS, Windows et Linux, `OnNavigationStateChange` signale également les changements d'URL, d'historique et de titre au sein d'un même document.

Définissez `WebView.Media` sur `core.WebMicrophone`, `core.WebCamera` ou les deux pour autoriser les requêtes `getUserMedia` d'une page. Déclarez les descriptions d'utilisation dans `apple.info` et `RECORD_AUDIO` ou `CAMERA` dans `android.permissions` ; le système interroge tout de même l'utilisateur. Sur Android, une requête en attente est refusée si l'application retire l'un des périphériques demandés, si la page est quittée ou si la vue est fermée pendant que la boîte de dialogue système est ouverte. La page doit demander l'accès à nouveau après un tel changement.

`WebView.Muted` et `MutedFunc` (en TSX `muted`) coupent tout l'audio de la page sur Windows et Linux, y compris Web Audio et les cadres intégrés. macOS 12 et iOS 15 ou ultérieur suspendent la lecture multimédia tant que la vue est en sourdine. Les autres hôtes conservent leur lecture habituelle. Les mises à jour s'appliquent à la page courante et persistent d'une navigation à l'autre.

En TSX, une modification de `injectedJavaScriptBeforeContentLoaded` recharge la source avec le nouveau script dans la même vue web, comme le fait `InjectedJavaScriptBeforeContentFunc` ; une modification de `injectedJavaScript` crée une nouvelle vue web, qui recharge la source de zéro, tout comme un changement de `userAgent`, `userAgentSuffix` ou `mobile`. Sur HarmonyOS, une modification de l'un ou l'autre des scripts injectés crée une nouvelle vue web et démarre un nouvel historique natif. Sur Windows et HarmonyOS, chaque script injecté s'exécute dans le cadre principal à l'intérieur d'un bloc, de sorte que ses déclarations `let`, `const` et `class` de haut niveau restent locales ; affectez à `window` pour exposer une variable globale que la page doit voir.

Définissez `Mobile` (en TSX `mobile`) pour afficher une page comme la voit un téléphone sur Windows, macOS et Linux, pour des sites tels que YouTube Shorts qui ne changent de vidéo par balayage vertical que sur téléphone : la page reçoit l'agent utilisateur d'un téléphone sauf si `UserAgent` est défini, ainsi qu'un écran tactile sans survol, et un glissement à la souris ou un défilement à la molette ou au trackpad sur une zone qui ne défile pas lui parvient comme un balayage du doigt. Windows émule un téléphone comme le fait le mode appareil de Chrome. macOS et Linux le font par script, de sorte que les feuilles de style de leurs pages voient toujours une souris, et les événements tactiles sont marqués comme non fiables (`untrusted`), ce qu'une page peut ignorer. iOS, Android, HarmonyOS, le web et les mini-programmes affichent les pages telles quelles.

Sur macOS, iOS, Windows et Linux, `OnOpenWindow` reçoit les demandes d'ouverture de nouvelle fenêtre émises par une page web. La vue ne crée pas de popup. Utilisez les champs `URL` et `UserInitiated` de la requête pour décider s'il faut naviguer dans la même vue avec `Ref.Load`, ouvrir l'URL ailleurs ou l'ignorer. Une vue restreinte envoie les requêtes refusées à `OnError`.

Définissez `OnDownload` pour enregistrer les fichiers téléchargés par les pages d'une vue web sur macOS, iOS, Linux, Windows et Android. Les téléchargements sont enregistrés dans `downloads/<id>/<file name>` à l'intérieur du répertoire de données de l'application et signalent leur démarrage, leur achèvement avec le nombre d'octets, ou leur échec ; les vues sans `OnDownload`, ou les vues restreintes, annulent les téléchargements.

`core.Video` lit un fichier ou une URL dans le lecteur de la plateforme. Sur macOS, iOS et tvOS, définissez `Video.AudioSource` sur un fichier ou une URL distinct contenant le son d'un clip vidéo de durée finie. L'audio du lecteur provient alors de cette source. L'image et le son démarrent ensemble, et `Ref.Seek` déplace les deux sur la même ligne temporelle. La vidéo fixe la durée de lecture, et un audio plus court devient silencieux une fois terminé.

Un clip transparent laisse apparaître ce qui se trouve derrière lui en HEVC avec canal alpha dans un `.mov` sur macOS (iOS n'est pas encore vérifié), et en VP9 avec canal alpha en WebM sur Linux avec le greffon `codecalpha` de GStreamer (`gstreamer1.0-plugins-bad`, qu'un paquet Linux peut recommander dans `nsgo.yaml`). Sur macOS, iOS, Windows et Linux, un clip local dont le canal alpha ne peut pas être conservé (WebM sur les plateformes Apple, HEVC sur Linux, et tout format sur Windows tant que la sortie alpha de Media Foundation n'a pas été vérifiée sur une machine Windows) fait que `OnError` le signale plutôt que d'afficher la vidéo dans un rectangle opaque : l'erreur est une `*core.MediaError` dont `Reason` vaut `core.MediaErrorAlphaUnsupported`, afin que l'application puisse décoder ce clip elle-même à la place. Android et le web, ainsi que les URL distantes sur toutes les plateformes, lisent ces clips tels que leur décodeur les produit, ce qui peut être opaque.

`OnPause` et `OnPlay` suivent les contrôles intégrés sur Apple, Android, Windows, Linux et le web. `OnPause` passe `VideoPauseControls` pour une mise en pause issue d'un contrôle. `Paused` s'applique lorsqu'une source s'ouvre ou que sa valeur change. Les changements de volume, de vitesse et de mise en page conservent la pause ou la lecture native, y compris une lecture déclenchée par les contrôles alors que `Paused` reste true. Liez `PausedFunc` à votre état et mettez-le à jour dans ces callbacks lorsque votre application affiche son propre bouton de lecture.

`Rate` définit la vitesse de lecture et `RateFunc` permet de la modifier pendant la lecture. Utilisez `VideoInfo.RateChangeable` fourni par `OnLoad` pour afficher un contrôle de vitesse. Linux peut changer la vitesse des vidéos navigables lorsque GstPlay et le filtre `scaletempo` de GStreamer sont installés. Le lecteur de repli utilise 1×.

Définissez `Video.Headers` pour envoyer des en-têtes de requête HTTP tels que `Referer`, `User-Agent` ou `Cookie` sur Android ainsi que pour les fichiers HTTP directs tels que MP4 ou MOV sur Linux, macOS, iOS et tvOS.

```go
core.Video{
    Source:  "https://video.example/clip.mp4",
    Headers: map[string]string{"Referer": "https://video.example/player"},
}
```

La vidéo copie la map lors de son montage et utilise les mêmes en-têtes si `SourceFunc` fournit une nouvelle URL. Sur les plateformes Apple, utilisez une source disposant d'une taille en octets connue et prenant en charge les requêtes par plage (`range`) pour la recherche dans le flux. Le lecteur envoie les en-têtes configurés à travers les redirections de même origine. Les mêmes en-têtes s'appliquent également à un fichier HTTP direct `AudioSource` sur les plateformes Apple. Les URL de listes de lecture se terminant par `.m3u` ou `.m3u8` utilisent le chargeur natif.

Sur Linux, installez GstPlay ainsi que les greffons GStreamer `scaletempo` et `souphttpsrc`. Utilisez l'URL média finale si vous définissez `User-Agent` ou si votre `souphttpsrc` ne dispose pas de la propriété `location-trusted`. Avec cette propriété et l'agent utilisateur par défaut, GStreamer conserve les en-têtes de l'application sur les redirections de même origine. Les en-têtes de l'application s'appliquent au fichier source, tandis que les segments des flux adaptatifs utilisent leurs propres requêtes.

## Composants

Un composant est une fonction qui prend un scope et retourne un élément. Il s'exécute une seule fois ; ce qui change par la suite est piloté par des signaux.

```go
func Greeting(cx *core.Scope) core.Node {
	return core.Text{Content: "Hello from NSGo"}
}
```

Un composant avec des propriétés est un type doté d'une méthode `Render` :

```go
type Badge struct{ Text string }

func (b Badge) Render(*core.Scope) core.Node {
	return core.Text{Content: b.Text}
}
```

`ui.ToastContent` peut omettre son bouton de fermeture avec `NoClose`, ajouter un nœud `Trailing` et fermer le toast via `OnPress`. `Toasts.NoClose` masque les boutons de fermeture sur toute la pile.

`ui.Toasts.Style` et `StyleFunc` personnalisent chaque surface de toast, y compris son arrière-plan, sa bordure, ses coins et son remplissage interne (`padding`). Le style de la pile fusionne par-dessus le thème, suivi par `Style` et `StyleFunc` de chaque `ToastContent`. Chaque `StyleFunc` s'exécute après son `Style` correspondant et se met à jour lorsque ses signaux changent. Une propriété `Color` dans ces styles colore le titre, et la description à opacité réduite, afin que le texte reste lisible sur une surface sombre ou colorée. Les phases d'entrée, de sortie et de balayage continuent d'appliquer leurs styles de mouvement. En TSX, définissez la prop `style` de `Toasts` et passez `style` ou `styleFunc` à `show` pour un toast individuel.

## Signaux

Un signal contient une valeur. Le lire dans une fonction appelée par un élément, telle que `ContentFunc`, abonne cette partie de l'écran, et seule cette partie est mise à jour lorsque la valeur change.

```go
count := core.NewSignal(cx, 0)

core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }}

ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }}
```

Un signal portant une tranche complète met à jour chaque ligne qui le lit. `core.NewCollection` stocke les éléments par clé, chacun dans son propre signal, de sorte qu'une ligne suit son propre élément et que la modification d'un élément ne réexécute qu'une seule ligne :

```go
todos := core.NewCollection(cx, func(t Todo) int { return t.ID }, nil)

core.List{Count: todos.Len, Key: func(i int) any { return todos.KeyAt(i) },
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		return core.Text{ContentFunc: func() string { t, _ := todos.At(row.Index()); return t.Title }}
	}}

todos.Update(id, func(t Todo) Todo { t.Done = true; return t })
```

Les autres types d'état reposent de la même manière sur des signaux :

- `core.NewSelector` indique si une clé correspond à l'élément sélectionné, et ne réexécute que les lignes que la sélection quitte et atteint.
- `core.NewHistory` conserve une valeur avec annulation et rétablissement, ses signaux `CanUndo` et `CanRedo` servant à un menu Édition.
- `core.NewPersisted` conserve une valeur typée dans le stockage de l'application sous forme de JSON versionné, en migrant ce que des versions précédentes ont écrit.
- `core.From` suit une source extérieure au graphe, comme un abonnement qui émet depuis une autre goroutine.

## Styles

`core.Style` suit CSS : flexbox et grid, marges internes et externes, arrondis, bordures, ombres, dégradés, transformations et états tels que `Active`. Les éléments acceptent également des classes Tailwind via `Class` :

```go
core.Text{Content: "Done", Class: "self-start rounded-full bg-zinc-100 px-3 py-1 text-sm"}
```

Les thèmes donnent aux composants de `ui` leur apparence en mode clair et en mode sombre.

## Applications de bureau

`nsgo.Run(Root)` affiche une vue racine unique. Une application de bureau peut ouvrir elle-même ce qu'elle affiche à la place : `nsgo.Run(App)` exécute `func App(app *core.Scope)` une fois, et celle-ci ouvre des fenêtres, des panneaux, des icônes de zone de notification et des raccourcis globaux avec `desktop`, qui partagent ses signaux. Cela fonctionne sur macOS, Windows et Linux ; sur macOS, `examples/tray` reste hors du Dock et vit dans la barre de menus. `nsgo.Run(App, nsgo.StartHidden())` ouvre la fenêtre principale sans l'afficher, afin qu'une application de barre d'état, ou une application lancée par le système à l'ouverture de session (`desktop.LaunchedAtLogin`), ne clignote pas à l'écran ; l'application l'affiche via `app.MainWindow().Show()`. Les téléphones et le web affichent l'application dès son lancement.

`desktop.SetWindowLevel` maintient une fenêtre au-dessus des autres, comme le fait un minuteur ou un compagnon de bureau, ou en dessous d'elles, et `desktop.SetWindowFrame` la déplace et la redimensionne. `desktop.CanSetWindowLevel` et `desktop.CanPlaceWindows` indiquent si ces appels ont un effet dans le scope d'une fenêtre : les deux sont vrais sur macOS, Windows et Linux sous X11, et aucun des deux pour une fenêtre de premier niveau ordinaire sous Wayland, dont le compositeur gère l'empilement et le placement.

Une fenêtre ouverte avec `desktop.WindowOptions{Transparent: true}`, ou la fenêtre principale avec `nsgo.WindowTransparent()`, n'a ni arrière-plan ni cadre, de sorte que le bureau apparaît partout où l'application ne dessine rien, comme pour un compagnon de bureau ou un widget ; une vue avec `Desktop.WindowDrag` permet de la déplacer. `NoShadow` supprime l'ombre du système et `SkipTaskbar` exclut la fenêtre de la barre des tâches Windows, d'Alt+Tab et de la liste des tâches X11, tandis que `desktop.SetWindowTransparent`, `SetWindowShadow` et `SetSkipTaskbar` les modifient ultérieurement. Windows ne modifie la transparence que sur une fenêtre ouverte comme transparente, laquelle n'a pas d'ombre, et Wayland ne dispose d'aucun indicateur pour masquer une fenêtre de sa liste des tâches. `desktop.CanMakeWindowsTransparent` et `desktop.CanSkipTaskbar` indiquent dans le scope d'une fenêtre si ces options prennent effet : la transparence requiert DirectComposition sous Windows et un compositeur sous Linux, X11 sans compositeur n'en ayant pas, et sur macOS `desktop.SetDockVisible` retire l'application du Dock.

## Terminal

Importez `base/terminal` et créez une session avec `terminal.New(terminal.Options{Conn: conn})` pour un flux distant, ou définissez `Command` pour un programme local sur macOS, Linux ou Windows. Affichez `terminal.View{Terminal: session}` et appelez `Close` lorsque la session se termine. `Options.ImageLimit` borne le stockage graphique Kitty en octets : zéro utilise 64 Mio, une valeur négative désactive les images. Les programmes peuvent envoyer des images RGB, RGBA ou PNG ainsi que des placements par caractères génériques Unicode.

Appelez `Search(text)` pour surligner les correspondances à l'écran et dans l'historique de défilement, `SearchNext` et `SearchPrevious` pour les sélectionner, et `EndSearch` pour les effacer. `Options.OnSearch` signale `SearchStatus` ; `Theme.Search`, `SearchText`, `SearchSelected` et `SearchSelectedText` définissent les couleurs des correspondances. Les shells qui émettent des marqueurs OSC 133 prennent en charge Alt/Option+clic dans leur saisie de commande active pour déplacer le curseur. NSGo n'installe pas de scripts d'intégration shell.

---
title: Services de la plateforme
description: Alertes, presse-papiers, fichiers, photos, partage, stockage, secrets, reconnaissance vocale, synthèse vocale, réseau, connexion, achats intégrés et cycle de vie de l’application.
group: Fonctionnalités plateforme
order: 44
---

## Alertes et presse-papiers

`cx.ShowAlert` affiche l’alerte ou la feuille d’actions de la plateforme. Ses `Actions` reviennent par indice, et un rôle dessine les boutons d’annulation et de destruction à la manière de la plateforme. Sur macOS, les alertes et les sélecteurs de fichiers utilisent une feuille tant que leur fenêtre est visible. Une fenêtre masquée, réduite ou hors écran, ou une application de barre des menus sans fenêtre, ouvre plutôt la boîte de dialogue séparément. Fermer la fenêtre à l’origine de la demande annule ses boîtes de dialogue.

```go
cx.ShowAlert(core.AlertConfig{
	Title:   "Delete the file?",
	Message: "This cannot be undone.",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(index int) {
	if index == 1 {
		remove()
	}
})
```

`SetClipboard` écrit du texte et `SetClipboardImage` une image. `ReadClipboard`, `ReadClipboardImage` et `ReadClipboardFiles` répondent de manière asynchrone, et `HasClipboardText` et `HasClipboardImage` lisent le contenu du presse-papiers sans avis de collage. `core.FileFromURL` ouvre sous forme de `core.File` les URL que donnent `ReadClipboardFiles` et `View.OnFileDrop` : les URL `file:` et, sur le web, les URL `blob:`.

tvOS n’a pas de presse-papiers public. Les écritures dans le presse-papiers sont ignorées, les lectures ne renvoient aucun contenu et les tests de présence renvoient false.

```go
cx.ReadClipboard(func(text string) { pasted.Set(text) })
cx.SetClipboard("Hello")
```

Sur macOS et Windows, `ReadClipboardItems` donne un instantané de toutes les représentations en octets disponibles, jusqu’à 48 Mio. Chaque élément porte des valeurs `ClipboardRepresentation` ordonnées, avec un `Type` natif et ses `Data` brutes, y compris les formats de simple présence dont les données sont vides. `ClipboardFormats` liste les noms de formats natifs sans copier leur contenu. Pour restaurer un instantané, passez-le à `WriteClipboardItems` avec un rappel facultatif qui reçoit l’erreur d’écriture. Une tranche vide efface le presse-papiers. Comparez l’égalité des valeurs de `ClipboardChangeCount` pour détecter une nouvelle copie, y compris lorsque le texte copié est identique.

macOS conserve les limites des éléments et les noms de types du presse-papiers. Windows utilise un seul élément et préserve les noms Win32 adossés à des octets, tels que `CF_HDROP`, et les formats enregistrés, tels que `Rich Text Format`. À l’écriture, il combine les éléments de formats distincts et utilise un octet nul pour un contenu vide. Les handles GDI, les formats à affichage par le propriétaire et les identifiants numériques de formats privés sont omis. Sur les autres plateformes, les rappels de lecture et d’écriture reçoivent `CodeUnavailable` et le rappel du compteur reçoit `-1`.

## Ouvrir des liens et des fichiers

`cx.OpenURL` ouvre une URL dans le navigateur ou l’application du système, et `core.CheckURL` indique ce qu’il refuse : l’absence de schéma, plus de 16 Kio, les espaces, une adresse web sans hôte, ainsi que `javascript:`, `data:`, `blob:`, `file:`, `about:` et `asset:`.

```go
if err := core.CheckURL(url); err == nil {
	cx.OpenURL(url, func(opened bool) {
		if !opened {
			status.Set("No app opens that link.")
		}
	})
}
```

`cx.OpenFile` ouvre un fichier que possède l’application dans l’application associée à son type, et `cx.Share` affiche la feuille de partage du système pour des fichiers, du texte ou les deux sur macOS, iOS, Android, Windows, Linux (via le portail de bureau ou le gestionnaire de fichiers) et le web (`navigator.share`). Un `core.SharedFile` est un chemin sur le disque — ou, pour `Share` sur le web et partout où un fichier est tenu en mémoire, ses octets `Data` — avec le nom et le type que voit l’autre application. Sur les ordinateurs de bureau, `desktop.RevealFile` affiche un fichier ou un dossier sélectionné dans le Finder, l’Explorateur ou le gestionnaire de fichiers, et `desktop.TrashFile` le déplace vers la Corbeille ; il ne supprime jamais définitivement, et répond `core.ErrNoTrash` en laissant l’élément en place lorsque le volume n’a pas de corbeille ou que la corbeille ne le conserverait pas, comme un fichier plus grand que la Corbeille de Windows.

```go
cx.Share(core.ShareOptions{
	Files: []core.SharedFile{{Path: path, Name: "Notes.txt"}},
	Text:  "A note from NSGo",
}, func(shown bool, err error) {})
```

`mobile.OnShared(cx, func(items []mobile.SharedItem) { ... })` reçoit du texte et des fichiers pour [`android.share_types`](building.md#incoming-shares-on-android) ou [`apple.share_types`](building.md#incoming-shares-on-ios). Les partages Android peuvent démarrer l’application ; iOS les met en file d’attente dans un App Group jusqu’à ce que l’application soit active ou que l’utilisateur l’ouvre la prochaine fois. Les partages en attente attendent le premier gestionnaire. iOS conserve le lot de la boîte de réception jusqu’au retour du rappel ; la livraison a lieu au moins une fois, même à travers la fin d’un processus. Le texte arrive dans `Text`, et les fichiers arrivent sous forme de copies lisibles en cache avec `Path`, `Name` et `MIME`. Déplacez ou copiez un fichier vers un stockage persistant s’il doit survivre au cache.

`desktop.LookUp` affiche le panneau de dictionnaire du système pour un texte, comme le fait un toucher à trois doigts sur macOS, dessiné au-dessus d’un cadre dans une vue : le texte et le cadre d’une sélection, avec la `Ref` de sa vue `Selectable`. `desktop.CanLookUp` indique s’il le peut, dans une fenêtre sur macOS ; Windows, Linux et le web n’ont pas ce panneau. Dans les tests, `nsgotest.App.LookUps` liste les panneaux affichés lorsque `nsgotest.Environment` définit `CanLookUp`.

```go
var area core.Ref
var selected core.TextSelection
core.View{Ref: &area, Selectable: true, OnTextSelection: func(s core.TextSelection) { selected = s }, Children: subtitles}
// From a menu item or a shortcut:
if desktop.CanLookUp(cx) {
	desktop.LookUp(cx, selected.Text, &area, selected.Bounds)
}
```

`desktop.QuickLook` demande des aperçus de fichiers, à partir d’un indice, et met à jour les fichiers lorsqu’il est rappelé pendant que le panneau est ouvert. `desktop.CanQuickLook` indique la disponibilité dans une fenêtre propre à l’application sur macOS et Windows, et sur Linux lorsque la session expose le service D-Bus de GNOME Sushi, en cours d’exécution ou activable. Windows utilise le gestionnaire d’aperçu enregistré pour le type de fichier ; les formats pris en charge dépendent des gestionnaires ou de l’afficheur installés. Un résultat true signifie que la plateforme a été sollicitée, pas qu’elle a affiché le fichier.

Espace ou Échap ferme le panneau. Les touches fléchées sélectionnent les fichiers de la demande sur Windows et, sur X11, dans les versions de Sushi qui signalent la sélection et la fenêtre propriétaire. Sur Wayland, Linux ouvre actuellement un aperçu sans fenêtre parente ; la fermeture automatique avec la fenêtre propriétaire et la sélection par les flèches ne sont pas disponibles. Les afficheurs propres à KDE et le web ne sont pas pris en charge.

```go
if desktop.CanQuickLook(cx) {
	desktop.QuickLook(cx, []string{reportPath, sourcePath}, 0)
}
```

## Icônes des menus natifs

`desktop.MenuItem.Icon` affiche une icône à côté d’un élément dans les menus de l’application, du Dock, de la zone de notification et les menus contextuels sur macOS, Windows et Linux. Un `desktop.MenuIcon` prend une icône vectorielle analysée (`SVG`, comme `lucide.Folder()`), des octets d’image encodée (`Data`, PNG, JPEG, GIF, WebP ou SVG) ou une `core.ImageSource` locale, de ressource ou de pixels (`Source`). Définir `Template` teinte une image monochrome avec la couleur du texte du menu, en suivant l’apparence claire ou sombre (et l’état de surbrillance sur macOS) ; les icônes `SVG` monochromes sont traitées par défaut comme des modèles.

```go
desktop.SetApplicationMenu(cx, []desktop.MenuItem{{
	Label: "File",
	Items: []desktop.MenuItem{
		{Label: "Open Folder…", Icon: desktop.MenuIcon{SVG: lucide.Folder()}, OnSelect: openFolder},
		{Label: "Export", Icon: desktop.MenuIcon{Data: exportPNG, Template: true}, OnSelect: exportFile},
	},
}})
```

Les icônes se chargent de façon synchrone depuis des sources locales ; les URL distantes et les images invalides laissent l’élément sans icône. Les libellés de menu, les raccourcis et les indicateurs de case à cocher ou de bouton radio conservent leur comportement natif.

Dans les tests, `nsgotest.MenuItem.Icon` enregistre l’icône de chaque élément dans `app.Menu`, `app.DockMenu` et `app.PopupMenu`.

## Photos et fichiers

`cx.PickImages` affiche le sélecteur de photos du système et répond avec des `core.PickedFile`, dont l’application lit les octets, le nom, le type et la taille en pixels. `cx.TakePhoto` affiche l’appareil photo lorsque la plateforme en a un ; `cx.Env().CanTakePhoto` indique si c’est le cas. Sur macOS, Windows et Linux, il affiche l’aperçu caméra du lecteur avec un bouton Prendre une photo. `cx.ScanCode` ouvre un aperçu caméra et répond avec le premier code qu’il lit, un code QR sauf si `ScanOptions.Kinds` en demande d’autres, ou nil lorsque l’utilisateur le ferme ; `cx.Env().CanScanCode` indique si la plateforme dispose d’un lecteur, sur les appareils macOS, iOS et Android dotés d’une caméra, les machines Windows qui en ont une, les bureaux Linux avec le portail Camera et `pipewiresrc` de GStreamer, et les navigateurs avec `getUserMedia` et `BarcodeDetector`. macOS présente une feuille lorsque la fenêtre à l’origine de la demande est visible, Windows et Linux une fenêtre modale au-dessus de la fenêtre, et le web une surcouche au-dessus de la page. Mettez `NSCameraUsageDescription` dans `apple.info` pour macOS et iOS, et `CAMERA` dans `android.permissions` pour Android. Android, Windows et Linux lisent tous les types sauf PDF417. Traitez le texte du code comme une entrée non fiable.

Sur tvOS, `PickImages` répond sans aucune image et `TakePhoto` renvoie `core.ErrNoCamera` ; NSGo ne fournit pas le sélecteur de Caméra de continuité.

Dans les tests, `nsgotest.App.PickImages`, `nsgotest.App.TakePhoto` et `nsgotest.App.ScanCode` reçoivent des enregistrements `nsgotest.ImagePick` pour les sélections et les prises de vue, et `nsgotest.CodeScan` pour les lectures de codes, et répondent à la place de l’interface du système. Les points d’accroche fonctionnent sans interface et sous les hôtes `nsgo test -target` pris en charge. `nsgotest.Environment(core.Environment{Camera: true, ScanCode: true})` donne une caméra à une cible qui n’en a pas.

```go
cx.PickImages(core.PickOptions{Limit: 1, MaxDimension: 2048}, func(files []core.PickedFile) {
	if len(files) == 1 {
		avatar.Set(files[0].Data)
	}
})
```

`cx.PickFiles` sélectionne des fichiers de tout type, `cx.PickDirectories` sélectionne des dossiers sous forme de `fs.FS`, et `cx.SaveFile` enregistre un fichier ou des octets via la boîte de dialogue d’enregistrement du système. Les deux sélecteurs prennent un `Title`, un libellé `Button` et un dossier `Start`, et `PickFilesOptions.TypesName` nomme le menu des types, de sorte qu’une application puisse les afficher dans sa propre langue ; ce sont des indications, que macOS, Windows et Linux suivent, iOS seulement pour `Start`, et Android et le web pas du tout. `cx.PickScreenColor` prélève une couleur n’importe où sur l’écran là où `cx.Env().CanPickScreenColor()` est vrai : macOS, Windows, Linux avec le portail de bureau Screenshot, et les navigateurs dotés d’`EyeDropper`.

Définissez `core.SaveOptions.Start` sur un dossier absolu existant pour y ouvrir la boîte de dialogue d’enregistrement sur macOS, Windows, Linux et iOS. Avec une indication vide ou inutilisable, ou sur les autres cibles, le système choisit le dossier.

Définissez `core.SaveOptions.Accept` pour valider la destination choisie avant l’écriture. Il s’exécute sur la goroutine du moteur. Renvoyer une erreur laisse la destination intacte. Sur macOS, Windows et dans le sélecteur GTK intégré au processus sur Linux, la boîte de dialogue affiche la raison et permet à l’utilisateur de choisir un autre chemin. Annuler après un refus signale `saved == false`, le chemin refusé et l’erreur. Sur Android et dans le portail de bureau Linux, un refus termine l’enregistrement avec ce résultat. Android laisse l’utilisateur choisir un dossier et fournit un URI d’arborescence avec le nom de fichier demandé comme paramètre de requête `name`. Il crée le document après approbation et signale l’URI du nouveau document en cas de succès. La plateforme abandonne l’écriture si la vérification ne répond pas dans les cinq secondes. iOS et HarmonyOS signalent une erreur pour les enregistrements nécessitant une vérification. Le web n’a pas de chemin de destination et utilise son parcours d’enregistrement ordinaire.

Dans les tests sans interface, `App.PickFiles`, `App.PickDirectories` et `App.SaveFile` reçoivent des enregistrements `nsgotest.FilePick`, `nsgotest.DirectoryPick` et `nsgotest.FileSave`. Les mêmes enregistrements figurent dans `app.FilePicks`, `app.DirectoryPicks` et `app.Saves`.

### Ressources intégrées

Placez des fichiers dans le répertoire `resources/` de l’application et utilisez `cx.ReadAsset("asset:maps/level.bin", done)` pour les lire depuis Go. Le rappel reçoit `([]byte, error)` sur la goroutine du moteur après la lecture, et il est abandonné si son propriétaire disparaît ou s’exécute à nouveau. Utilisez `errors.Is(err, core.ErrAssetNotFound)` pour identifier les fichiers manquants. La lecture charge le fichier entier en mémoire.

## Audio PCM tiré

Utilisez `audio.NewOutput(cx, audio.OutputConfig{SampleRate: 48000, Channels: 2, Render: render})` pour lire du PCM Float32 entrelacé que votre application décode ou synthétise. `Start` commence à tirer des échantillons, `Pause` arrête les tirages et la portée propriétaire ferme la sortie en disparaissant. macOS, iOS, Android, Windows, Linux et le web prennent en charge ce flux. Les autres cibles renvoient `audio.ErrUnsupported`.

`Render` renvoie le nombre de trames qu’il a remplies. Il s’exécute sur le thread audio et doit éviter les blocages, les allocations et l’accès au graphe. Un remplissage incomplet joue du silence pour le reste et incrémente `Underruns`. `Position` renvoie le nombre monotone de trames et l’heure de l’hôte à cette trame. Utilisez `Flush` pour abandonner les trames en attente et `Latency` pour lire la latence du tampon. Les changements de sortie audio et les interruptions mettent la sortie en pause et appellent `OnInterrupt` sur le moteur. L’application peut la reprendre avec `Start`.

## Stockage

`cx.Storage()` est le stockage clé-valeur de l’application, conservé sur l’appareil et partagé par tous les graphes de l’application. Les lectures sont synchrones, et une lecture effectuée dans un calcul abonne celui-ci aux changements ; `SetItem` renvoie une `core.StorageFullError` au-delà du quota (`core.StorageQuota`, 5 Mio par défaut). C’est un simple fichier que n’importe quel programme de l’utilisateur peut lire : conservez donc plutôt les jetons et les mots de passe dans les [secrets](#secrets).

`cx.Store()` renvoie le magasin sous-jacent, pour un hôte qui crée une autre racine avec `core.RootOptions.Storage`, comme celui qui exécute un plugin sur une goroutine qui lui est propre : les deux racines partagent alors les entrées de l’application et ce qui est conservé sur l’appareil. C’est le magasin entier, et non la partition que `cx.ProvideStorage` donne au code que l’application n’a pas écrit.

Un hôte qui fournit sa propre persistance appelle `core.NewStore(backend)` avec un `core.StorageBackend` : `Load() (map[string]string, error)` charge une seule fois, et `Write(core.StorageChange)` reçoit les changements dans l’ordre. Faites un switch sur `change.Op` (`core.StorageSet`, `core.StorageRemove`, `core.StorageClear`) et utilisez `change.Key` et `change.Value`. Passez `nil` pour un stockage tenu uniquement en mémoire.

```go
store := cx.Storage()
if tab, ok := store.GetItem("tab"); ok {
	selected.Set(tab)
}
store.SetItem("tab", "library")
```

`store.Keys()` liste les clés, `RemoveItem` en supprime une et `Clear` vide le stockage. `cx.DataDir()` et `cx.CacheDir()` donnent les dossiers de l’application pour les fichiers trop volumineux pour le stockage clé-valeur ; `cx.LogsDir()` donne le répertoire de la plateforme pour les fichiers journaux, et un `Scope.ProvideStorage` donne à un sous-arbre une partition qui lui est propre.

Un App Clip iOS et son application complète partagent ces emplacements de données et de stockage lorsque `app_clip` et un `apple.app_group` explicite sont configurés ; leurs caches restent séparés. Activer le groupe ne migre pas les données existantes du bac à sable. [Le stockage des App Clips](building.md#app-clips) couvre la configuration et la signature.

Sur HarmonyOS, les opérations de fichiers Go telles que `os.ReadFile`, `os.WriteFile` et `os.Rename` fonctionnent dans ces répertoires et dans `os.TempDir()`. Les chemins relatifs commencent dans le répertoire des fichiers. Les verrous de fichiers consultatifs et les liens symboliques ne sont pas disponibles.

### Les dossiers de l’utilisateur

`desktop.UserDir(cx, desktop.DownloadsDir)` renvoie le chemin de l’un des dossiers de l’utilisateur : `DocumentsDir`, `DownloadsDir`, `DesktopDir`, `PicturesDir`, `MusicDir` ou `VideosDir`. Il interroge le système à chaque appel, si bien qu’un dossier déplacé par l’utilisateur est suivi : les dossiers du répertoire personnel sous macOS (`Movies` pour les vidéos), les dossiers connus sous Windows, y compris ceux de OneDrive, et les répertoires utilisateur XDG sous Linux. Le dossier peut ne pas encore exister. Les téléphones, le web et HarmonyOS n’ont pas de tels dossiers qu’une application atteint par chemin, pas plus que Linux pour un dossier que `user-dirs.dirs` omet : `UserDir` renvoie `core.ErrUserDirUnavailable`, et l’application demande alors à l’utilisateur avec `cx.PickDirectories` ou `cx.SaveFile`.

```go
dir, err := desktop.UserDir(cx, desktop.DownloadsDir)
if err != nil {
	dir = cx.DataDir()
}
```

Les tests n’obtiennent jamais les dossiers de l’utilisateur : chacun est un répertoire vide dans le répertoire temporaire du test, et `nsgotest.UserDirs(map[core.StandardDir]string{…})` les fixe, un dossier omis étant indisponible.

### Rapports de plantage

Les builds natives de production conservent les traces de plantage Go sous `cx.LogsDir()/crashes`. Apple enregistre les exceptions Objective-C non interceptées, Android les exceptions Java non interceptées, et Windows les exceptions SEH qui atteignent son filtre de premier niveau. Listez les rapports en attente au lancement suivant avec `cx.CrashReports()`. Chaque rapport identifie la version, la build et la plateforme qui l’ont produit. Appelez `Open()` pour lire le texte de diagnostic, fermez le lecteur, puis appelez `Acknowledge()` pour supprimer le rapport une fois que l’application l’a traité. Exécutez ces opérations sur les fichiers dans une goroutine.

## Espace disque

`cx.DiskSpace(func(space core.DiskSpace, err error) { … })` interroge de manière asynchrone le volume contenant `cx.DataDir()` et rappelle sur la goroutine du moteur. `Total` est la capacité signalée par le système, `Available` ce que l’application peut utiliser pour une opération demandée par l’utilisateur, et `Opportunistic` ce qu’elle peut utiliser pour les caches et le préchargement. Les trois sont en octets. `Available` inclut l’espace que le système récupère à la demande, là où il signale ce chiffre. Les plateformes qui ne donnent qu’un seul chiffre d’espace libre le renvoient dans les deux champs. Sur le web, `Total` est le quota de stockage estimé par le navigateur pour l’origine et `Available` la part inutilisée. Lorsque le service n’est pas disponible, il renvoie `core.ErrDiskSpaceUnavailable`.

Les builds Apple exigent que `apple.privacy.accessed_apis` dans `nsgo.yaml` déclare `NSPrivacyAccessedAPICategoryDiskSpace`. Déclarez `E174.1` pour vérifier l’espace avant d’écrire ou pour libérer des fichiers lorsque l’espace est faible, avec un changement que l’utilisateur peut observer. Déclarez `85F4.1` pour montrer l’espace disponible à l’utilisateur. L’application doit respecter les restrictions d’Apple sur l’utilisation et le partage des mesures. Une build sans la catégorie renvoie `core.ErrNotDeclared`. [Les raisons approuvées par Apple](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)

Définissez la réponse d’un test avec `nsgotest.DiskSpace(space, err)`.

## Stockage dans le cloud

Le paquet `cloud` synchronise entre appareils de petites entrées clé-valeur (`cloud.Get`, `cloud.Set`, `cloud.Delete`, `cloud.Keys`) et des fichiers ou instantanés de sauvegarde plus volumineux (`cloud.ReadFile`, `cloud.WriteFile`, `cloud.DeleteFile`, `cloud.ListFiles`) : le magasin clé-valeur iCloud et les conteneurs de documents ubiquitaires sur macOS et iOS, et les Saved Games de Google Play Games sur Android.

Chaque opération livre son rappel sur la goroutine du moteur et l’ignore si `cx` a été libéré. `cx.Env().CanUseCloudStorage()` indique si le stockage dans le cloud est disponible ; lorsque l’utilisateur est déconnecté, que les entitlements sont absents, ou que la plateforme n’a pas de backend cloud natif (Windows, Linux et le web), chaque appel renvoie `cloud.ErrUnavailable` et n’abandonne jamais d’écritures en silence.

```go
if cx.Env().CanUseCloudStorage() {
	cloud.WriteFile(cx, "saves/slot1.sav", saveBytes, func(err error) {
		if err != nil {
			status.Set(err.Error())
		}
	})
}

cloud.WatchChanges(cx, func(ev cloud.ChangeEvent) {
	reload(ev.Keys, ev.Files)
})

cloud.OnConflict(cx, func(c cloud.Conflict) {
	_ = c.Resolve(mergeSaves(c.Local, c.Remote))
})
```

`cloud.WatchChanges` et `cloud.OnConflict` livrent les changements externes venus d’autres appareils et les conflits de versions sur la goroutine du moteur, et se nettoient automatiquement lorsque `cx` est libéré. Lire un fichier en conflit renvoie une `*cloud.ConflictError` (qui correspond à `errors.Is(err, cloud.ErrConflict)`).

Pour activer les entitlements iCloud d’Apple dans `nsgo build`, ajoutez une section `cloud:` (ou `icloud:`) à `nsgo.yaml` :

```yaml
cloud:
  kv_store: true
  containers:
    - iCloud.com.example.myapp
```

Le profil de signature doit accorder les capacités iCloud que vous demandez. Sur macOS, seules les builds Mac App Store (`sign.macos_profile`) portent iCloud, car macOS ne l’accorde qu’avec un profil de provisionnement ; une build Developer ID l’omet avec un avertissement. Sur Android, le stockage dans le cloud nécessite le SDK Play Games Services v2 dans l’application et un joueur connecté.

Sur les boutiques pour ordinateur de bureau comme Steam, Steam Auto-Cloud synchronise `cx.DataDir()` directement, sans aucun code supplémentaire — continuez à y écrire les fichiers de sauvegarde locaux et laissez Steam synchroniser le répertoire.

Dans les tests, `nsgotest` donne à chaque application un magasin cloud isolé en mémoire : basculez la disponibilité avec `nsgotest.CloudAvailable` ou `app.SetCloudAvailable`, simulez des mises à jour externes venues d’autres appareils avec `app.SimulateCloudChange`, et testez la résolution des conflits avec `app.SimulateCloudConflict`.

## Secrets

`cx.Secrets()` est le magasin sécurisé de l’application, pour les jetons d’actualisation, les clés d’API et les mots de passe : `Get`, `Set` et `Delete` une chaîne par compte, que la plateforme protège. Un compte absent se lit comme `("", false, nil)`, et là où la plateforme n’a pas de magasin sécurisé, comme sur le web, chaque appel renvoie `core.ErrSecretsUnavailable` ; NSGo ne se rabat jamais sur un stockage en clair. Le système peut faire attendre un appel, comme le fait un trousseau qui interroge l’utilisateur : appelez-les donc hors de la goroutine du moteur et renvoyez le résultat avec `cx.Post`.

```go
secrets := cx.Secrets()
go func() {
	token, ok, err := secrets.Get("refresh-token")
	cx.Post(func() {
		if err == nil && ok {
			login(token)
		}
	})
}()
```

| Plateforme | Où un secret est conservé |
| --- | --- |
| macOS | Un mot de passe générique du trousseau de session : `kSecAttrService` est le service, `kSecAttrAccount` le compte |
| iOS, tvOS, watchOS | Un mot de passe générique du trousseau, de la même manière |
| Windows | Un identifiant générique du Gestionnaire d’identification dont la cible est `service/account`, 2560 octets au plus |
| Linux | Un élément du Secret Service (GNOME Keyring, KWallet) avec les attributs `service` et `account` ; indisponible lorsque la session n’en exécute aucun |
| Android | Chiffré avec une clé AES de l’Android Keystore, dans les fichiers privés de l’application, hors des sauvegardes ; les lectures se rabattent aussi sur les valeurs qu’`expo-secure-store` a chiffrées dans ses `SharedPreferences` `SecureStore` |
| Mini-programmes WeChat | Stockage chiffré par la plateforme (`wx.setStorage` / `wx.getStorage` avec `encrypt: true`, bibliothèque de base 2.21.3+) ; indisponible avec les bibliothèques de base plus anciennes |
| Web, autres mini-programmes | Indisponible (`core.ErrSecretsUnavailable`) : les navigateurs n’ont pas de magasin de secrets client protégé par le système d’exploitation (conservez les sessions web dans des cookies `HttpOnly` définis par le serveur), et les mini-programmes QQ, Alipay et Douyin n’ont pas de stockage chiffré |

Le service est par défaut l’identifiant de l’application (`core.App().ID`). Une application qui vient d’un autre framework garde ses utilisateurs connectés en nommant les éléments que ce framework a écrits : `Service` et, sur les plateformes Apple, `AccessGroup` et `Accessible`. Sur Android, définissez `Service` sur le `keychainService` d’`expo-secure-store` (`"key_v1"` par défaut) pour lire les valeurs qu’il a chiffrées dans les `SharedPreferences` ; écrire ou supprimer efface l’entrée migrée. Sur iOS, `core.SecretAfterFirstUnlock` permet à un travail en arrière-plan de lire un élément pendant que le téléphone est verrouillé, et une accessibilité `ThisDeviceOnly` le tient à l’écart des sauvegardes. Sur macOS, un `AccessGroup` déplace les éléments vers le trousseau à protection des données, ce qui nécessite une build signée par une équipe dont l’entitlement `keychain-access-groups` liste le groupe, configuré sous `macos.entitlements` dans `nsgo.yaml` pour les builds Mac App Store comme Developer ID.

```go
s := cx.Secrets()
s.Service = "app:no-auth" // the service an earlier build used
s.Accessible = core.SecretAfterFirstUnlockThisDeviceOnly
err := s.Set("refresh-token", token)
```

`nsgotest` conserve les secrets d’une application en mémoire, dans `App.Secrets`, et `nsgotest.Secrets` démarre un test avec certains d’entre eux, de sorte qu’un test ne touche jamais le trousseau de la machine. Un paquet TSX accède au magasin avec la capacité `secrets`, sous un service qui lui est propre.

## Réseau

`cx.Env().Network()` indique si le système dispose d’une route utilisable (`Online`) et s’il considère cette route comme facturée à l’usage ou coûteuse (`Expensive`). Les liaisons la suivent comme toute autre valeur d’environnement. `cx.OnNetworkChange(func(core.Network))` appelle le gestionnaire sur la goroutine du moteur à chaque changement, tant que sa portée existe. La route du système ne dit rien de la réponse d’un serveur particulier. Une cible qui ne peut pas signaler la connectivité est par défaut en ligne, et un coût inconnu vaut par défaut false.

Définissez `core.Environment{Network: &core.Network{Online: false}}` via `nsgotest.Environment` ou `App.SetEnvironment` pour tester le comportement hors ligne.

Le propre `net/http` de Go effectue les requêtes sur toutes les cibles, avec les réglages TLS et proxy du système. Les signaux appartiennent à la goroutine du moteur, de sorte qu’une réponse venue d’une autre goroutine les atteint via `cx.Post`.

```go
go func() {
	resp, err := http.Get("https://api.example.com/items")
	if err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	defer resp.Body.Close()
	var items []Item
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	cx.Post(func() { list.Set(items) })
}()
```

`cx.Post` exécute sa fonction sur la goroutine du graphe, où les signaux peuvent être définis. Sur Android, le manifeste demande l’autorisation internet, et les téléchargements d’images se mettent eux-mêmes en cache sur le disque.

`core.NewResource` fait de même pour une valeur chargée depuis une source. Sa fonction de récupération s’exécute sur une goroutine qui lui est propre et recharge lorsque la source change :

```go
items := core.NewResource(cx, func() (string, bool) { return query.Get(), query.Get() != "" },
	func(ctx context.Context, q string, _ core.ResourceInfo) ([]Item, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/items?q="+url.QueryEscape(q), nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var list []Item
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return nil, err
		}
		return list, nil
	})
```

`items.Get()`, `items.Err()` et `items.Loading()` sont des signaux que suivent les liaisons. Une nouvelle requête annule via `ctx` la requête encore en cours, de sorte qu’une réponse tardive ne remplace jamais une plus récente. `items.Refetch()` réessaie en conservant la dernière liste pendant le chargement, et `items.Mutate` définit immédiatement une valeur pour une mise à jour optimiste.

`core.Suspense` affiche un contenu de remplacement pendant que les ressources que lit son contenu chargent leur première valeur, de sorte qu’un écran n’a besoin d’aucun indicateur de chargement propre :

```go
core.Suspense{
	Content:  func(cx *core.Scope) core.Node { return itemList(cx, items) },
	Fallback: func(*core.Scope) core.Node { return ui.Spinner{Label: "Loading"} },
}
```

Le contenu reste monté mais masqué pendant qu’il attend, de sorte qu’il s’affiche immédiatement à l’arrivée des valeurs. Un rechargement le laisse affiché avec la dernière valeur.

### Découvrir les services locaux

Sur macOS, iOS, Android, Windows et Linux, `cx.AdvertiseService` publie un port d’écoute et des attributs TXT. `cx.BrowseServices` signale les services résolus avec leur hôte, leur port et leurs attributs TXT. `Host` peut être un nom d’hôte DNS ou une adresse IP. Lorsqu’un service disparaît, le rappel le reçoit à nouveau avec `Removed` défini. Vérifiez `cx.Env().CanDiscoverServices()` pour la prise en charge par la plateforme. Les deux appels renvoient une fonction d’arrêt et prennent fin lorsque leur portée est libérée. Une erreur de la plateforme est signalée de manière asynchrone sur la goroutine du moteur. Linux utilise Avahi et signale une erreur si le démon n’est pas disponible.

Pour Bonjour sur les plateformes Apple, déclarez les types de services et expliquez l’accès au réseau local dans `nsgo.yaml`. Le système gère la demande d’autorisation et mémorise le choix de l’utilisateur.

```yaml
apple:
  info:
    NSLocalNetworkUsageDescription: Finds nearby devices to sync with.
    NSBonjourServices:
      - _example._tcp
```

Android utilise `NsdManager`. Android 12 et versions antérieures, ainsi qu’Android 13 avant T Extensions 7, exigent `CHANGE_WIFI_MULTICAST_STATE` pour la découverte en Wi-Fi. Les builds autonomes déclarent cette autorisation. Si vous intégrez NSGo dans un hôte Android, ajoutez-la au manifeste de l’hôte pour ces appareils. Les valeurs TXT annoncées doivent être en UTF-8 valide sur les appareils dépourvus de l’API TXT brute d’Android (API 37 ou T Extensions 22). La découverte préserve les valeurs TXT brutes. Si un hôte Android cible l’API 37 ou supérieure, il doit déclarer et demander `ACCESS_LOCAL_NETWORK` sur Android 17 ou version ultérieure.

Utilisez `nsgotest.LocalServices` pour fournir les résultats de découverte dans un test sans interface.

## À l’écoute

Utilisez `cx.SetNowPlaying(core.NowPlaying{...})` pour afficher le média en cours de l’application dans le Centre de contrôle et la barre des menus de macOS. Fournissez `Title`, `Artist`, `Album` et, facultativement, `Artwork` sous forme de `*core.ImageSource`. Une `Duration` nulle signifie un flux en direct. Donnez la `Position` actuelle et le `Rate` de lecture (zéro en pause, un à vitesse normale). Publiez à nouveau lorsque la piste, la position ou la vitesse change. Le système fait avancer la position entre les appels.

Listez les commandes de transport prises en charge dans `Commands`, telles que `core.MediaPlay`, `core.MediaPause`, `core.MediaTogglePlayPause` et `core.MediaSeek`. Traitez les demandes avec `cx.OnMediaCommand`. Le rappel reçoit le `Kind` de la commande et, pour un déplacement, la `Position` demandée. Le gestionnaire actif le plus récent reçoit les commandes. Appelez `cx.ClearNowPlaying()` lorsque l’application a fini de lire. Tant que l’application publie manuellement, les vidéos contrôlées sur macOS suspendent leur propre publication. Vérifiez `cx.Env().CanShowNowPlaying()` pour la prise en charge par la plateforme.

## Téléchargements en arrière-plan

Utilisez `cx.Download` et `cx.OnDownloadUpdate` dans les applications iOS et macOS autonomes, et sur Android. Les vues Apple intégrées n’ont pas d’hôte de transfert. Les applications Android activent `android.background_downloads: true` dans `nsgo.yaml`. Démarrez un téléchargement à partir d’une action de l’utilisateur pendant que l’application est visible. Utilisez une destination absolue dont le répertoire parent existe et dans lequel l’application peut écrire.

```go
cx.OnDownloadUpdate(func(p core.DownloadProgress) {
    progress.Set(p)
})
task, err := cx.Download(core.DownloadRequest{URL: url}, path, core.DownloadOptions{
    CertificatePEM: serverCertificate,
})
```

Le transfert utilise GET et accepte des en-têtes de requête facultatifs. Il utilise le magasin de confiance HTTPS normal, sauf si `CertificatePEM` épingle le certificat feuille du serveur. L’épinglage exige aussi un nom d’hôte correspondant et un certificat valide. Utilisez l’URL finale, car le service exige une réponse réussie de cette URL.

Sur Apple, un certificat auto-signé sur un nom d’hôte public nécessite une exception App Transport Security pour cet hôte dans `apple.info`. Le transfert vérifie tout de même l’épinglage exact de son certificat, le nom d’hôte et la validité. Une connexion à un NAS local nécessite aussi une `NSLocalNetworkUsageDescription` expliquant l’accès, et les hôtes non qualifiés ou en `.local` utilisent `NSAllowsLocalNetworking`. [Les réglages ATS d’Apple](https://developer.apple.com/documentation/security/preventing-insecure-network-connections)

```yaml
apple:
  info:
    NSAppTransportSecurity:
      NSExceptionDomains:
        files.example.com:
          NSExceptionAllowsInsecureHTTPLoads: true
```

Le service de premier plan Android continue après que l’activité a quitté l’écran. Il publie une notification de progression, soumise à l’autorisation de notification d’Android. La progression utilise des nombres d’octets et définit `Total` à -1 tant qu’il est inconnu. Un succès remplace atomiquement la destination. `task.Pause()` conserve le fichier partiel, `task.Resume()` utilise une plage HTTP lorsque le serveur fournit un validateur, et `task.Cancel()` supprime le fichier partiel.

iOS utilise une URLSession d’arrière-plan du système qui poursuit les transferts pendant que l’application est suspendue. L’application reconnecte sa session au lancement et publie les fichiers avant d’accuser réception de la fin en arrière-plan. Sur macOS, les transferts s’exécutent dans le processus. Les plateformes Apple utilisent les données de reprise natives pour les téléchargements en pause et conservent les métadonnées des requêtes dans le stockage privé de l’application jusqu’à ce qu’un résultat final soit enregistré. Avec un serveur qui ne peut pas reprendre, le transfert recommence.

Enregistrez à nouveau `OnDownloadUpdate` au démarrage de l’application pour voir les tâches actives et les résultats enregistrés. Utilisez `p.Task.Forget()` après avoir traité un résultat final. Android peut arrêter le service, par exemple lorsque le temps de transfert au premier plan expire. L’application reçoit alors `DownloadInterrupted`. Sur Windows, Linux, tvOS, watchOS et le web, `Download` renvoie `ErrDownloadUnavailable`.

## Reconnaissance vocale et cycle de vie

`cx.StartSpeech` reconnaît la parole avec le moteur de reconnaissance du système, là où `cx.Env().CanRecognizeSpeech()` indique qu’il existe (macOS, iOS, Android, Windows et les navigateurs exposant `SpeechRecognition` ou `webkitSpeechRecognition`), et répond avec la transcription au fil de ses changements. Sur les plateformes Apple, déclarez `NSMicrophoneUsageDescription` et `NSSpeechRecognitionUsageDescription` dans `apple.info` ; sur Android, ajoutez `RECORD_AUDIO` à `android.permissions`. Dans un test sans interface, `app.Speech` contient une `nsgotest.SpeechSession` tant que la reconnaissance est active. Utilisez `app.HearSpeech` et `app.EndSpeech` pour livrer des résultats.

```go
session, err := cx.StartSpeech(core.SpeechOptions{
	Lang:    "en-US",
	Interim: true,
	OnResult: func(text string, final bool) {
		transcript.Set(text)
	},
})
if err == nil {
	defer session.Stop()
}
```

`cx.Env().Lifecycle()` indique si l’application est active, inactive ou en arrière-plan : sur les ordinateurs de bureau, la fenêtre dans laquelle se trouve `cx`, de sorte que le contenu d’une fenêtre réduite puisse se mettre en pause pendant qu’une autre fenêtre s’affiche, et dans la portée propre de l’application, toutes ses fenêtres ensemble. `desktop.Minimized(cx)` indique si cette fenêtre est réduite, et `desktop.FrontmostApp(cx)` quelle application est au premier plan sur le bureau, l’application elle-même comprise, et si sa fenêtre est en plein écran, comme la suit un animal de compagnie de bureau ou un outil de suivi du temps ; `desktop.CanReadFrontmostApp(cx)` indique si elle est suivie, sur macOS, Windows, X11 et les compositeurs Wayland pris en charge. `cx.OnQuit` fait le ménage à la fermeture, quelle qu’en soit la manière, avec 200 ms pour son travail. `desktop.OnQuitRequest` peut annuler ou différer un élément de menu Quitter sur ordinateur de bureau ou `desktop.RequestQuit(cx)`. Sur Windows et Linux, il s’exécute aussi après `OnClosing` lorsque la fermeture d’une fenêtre mettrait fin à l’application. `desktop.RequestRestart(cx)` interroge les mêmes gestionnaires avant de redémarrer ; `desktop.Restart(cx)` reste immédiat. Les relances par le programme de mise à jour sur Linux et Windows les interrogent aussi, de sorte qu’un enregistrement puisse les annuler ou les différer. Lorsque le système signale une fermeture de session, un redémarrage ou un arrêt, `q.Forced()` est vrai et l’application passe par `OnQuit`. Vérifiez `Forced()` avant de demander une décision à l’utilisateur. `cx.SetAppearance` force le mode clair ou sombre, `cx.SetLocale` choisit la langue, `cx.OnKey` lie un raccourci clavier, qui reste inactif tant que ce qu’affiche son propriétaire est masqué ou sous une boîte de dialogue, sauf si `core.KeyOptions{OverModals: true}` le maintient, qui prend sa touche avant le menu de l’application, sauf si `YieldsToMenu` la laisse à un élément de menu activé prenant le même raccourci là où se trouve le focus (tant qu’un champ texte ou une vue web a le focus, un élément `MenuNotEditing` laisse sa touche à la liaison et retient la touche des éléments suivants ayant le même raccourci, et le menu de Linux ne prend aucune touche pressée sans Ctrl, Alt ou une touche logo, de sorte que la liaison y conserve une telle touche), qui fonctionne dans la fenêtre où se trouve `cx`, sauf si `AllSurfaces` le lie dans toutes les fenêtres et tous les panneaux de l’application, y compris ceux ouverts plus tard, pour un raccourci valable dans toute l’application, qui reste alors inactif dans chacun d’eux tant que ce qu’affiche son propriétaire est masqué dans la fenêtre où il est lié, et qui laisse à un contrôle ayant le focus les touches qu’il utilise, à un bouton sa touche Entrée et sa barre d’espace, et à un curseur, une barre d’onglets ou un groupe de boutons radio ses flèches, sauf s’il est lié avec `core.KeyAlways` ou `core.KeyNotEditing` ; `cx.OnOpenFiles` reçoit les documents ouverts sur macOS, Windows, Linux et iOS. iOS importe les fichiers ordinaires dans un stockage temporaire appartenant à l’application ; déplacez-les vers `cx.DataDir()` pour les conserver. Sur macOS, fermer la fenêtre qui fournissait le menu de l’application restaure le menu de la fenêtre restante la plus récente, ou le menu par défaut lorsqu’il n’en reste aucune.

Sur Wayland, le suivi de l’application au premier plan exige la version 2 ou ultérieure du protocole wlr foreign-toplevel du compositeur ; les autres compositeurs signalent `CanReadFrontmostApp` à false. Son identifiant est l’identifiant d’application Wayland, son nom provient de l’entrée de bureau lorsqu’elle est disponible, et son PID vaut 0. Les applications Windows empaquetées utilisent leur AUMID au lieu d’un chemin d’hôte de cadre. macOS suit l’état de présentation plein écran d’AppKit plutôt que de le déduire de la taille de la fenêtre.

Un rôle d’édition dans le menu de l’application prend l’éditeur ayant le focus avant son `OnSelect` de repli. Sur macOS, Annuler et Rétablir suivent l’historique de cet éditeur, et Couper suit sa sélection, y compris lorsque l’application est inactive ; sans éditeur, le repli reste activé.

Sur iOS, donnez à `cx.OnKeyWith` une fonction `core.KeyOptions.Title` pour proposer le raccourci dans le menu clavier du système tant que la liaison peut se déclencher. `Group` nomme son menu sur iPadOS 26 et versions ultérieures. Les fonctions peuvent lire le catalogue de l’application, de sorte que les titres des raccourcis suivent la langue. Utilisez `app.KeyCommands()` dans les tests pour inspecter la liste actuelle.

```go
cx.OnKey("mod+k", func() { palette.Set(true) })
cx.SetLocale("de")
cx.OnQuit(func(ctx context.Context) { saveDraft(ctx) })
```

`cx.Env().DeviceName()` est le nom que l’utilisateur a donné à l’appareil, et `cx.Env().DeviceModel()` son modèle matériel, tel que `Mac15,7`, `iPhone15,2` ou `Google Pixel 8`, afin qu’une application puisse distinguer deux appareils appairés. macOS, Android, Windows et Linux donnent les deux ; iOS donne le modèle, et le nom seulement à une application dotée de l’entitlement d’Apple pour le nom d’appareil attribué par l’utilisateur. watchOS donne son identifiant matériel, HarmonyOS son nom commercial ou son modèle de produit, et les mini-programmes la marque et le modèle de la plateforme ; ces cibles laissent vide le nom attribué par l’utilisateur. Le web ne donne ni l’un ni l’autre. Les tests les définissent avec `nsgotest.Environment`.

`cx.Env().ThermalState()` signale `core.ThermalNominal`, `ThermalFair`, `ThermalSerious` ou `ThermalCritical` sur les plateformes Apple et Android API 29 ou ultérieure. `cx.Env().LowPowerMode()` signale `core.LowPowerOn` ou `LowPowerOff` sur les plateformes Apple, Android, Windows et les sessions Linux avec power-profiles-daemon. Là où le système ne donne aucune valeur, les lectures renvoient `ThermalUnknown` et `LowPowerUnknown`. Chaque lecture suit les changements indépendamment, de sorte qu’une application puisse ajuster sa charge de travail pendant qu’elle s’exécute. Dans les tests, définissez `ThermalState` et `LowPowerMode` sur `core.Environment` via `nsgotest.Environment` ou `app.SetEnvironment`.

`desktop.CursorPoint(cx, func(p core.Point, available bool) { ... })` lit la position du pointeur même en dehors des fenêtres de l’application sur macOS, Windows et X11. Il prend un échantillon par appel. Le point est dans les mêmes coordonnées d’écran que `desktop.Monitors(cx)` et `Window.ShowAt`. Sur Wayland et les autres cibles non prises en charge, ou si la portée n’a pas `core.AccessCursorPoint`, `available` vaut false.

`desktop.SetWindowConstraints` limite la taille du contenu et contrôle le redimensionnement, la réduction et l’agrandissement ; définissez `WindowOptions.Constraints` à l’ouverture d’une fenêtre ou utilisez `nsgo.WindowConstraints` pour la fenêtre principale. Linux applique les maximums non fixes sur X11 et retire les boutons de réduction et d’agrandissement interdits de la barre de titre native de GTK sur X11 et Wayland. Wayland ne prend actuellement en charge les tailles fixes que pour une borne supérieure.

`desktop.SetWindowLevel` fait flotter une fenêtre au-dessus des autres, et `desktop.SetWindowWorkspaces(cx, desktop.WindowWorkspacesAll)` ou `WindowOptions.Workspaces` la garde sur tous les espaces de travail, comme le sont un animal de compagnie de bureau, un affichage tête haute ou un minuteur : sur macOS sur chaque Space et au-dessus des espaces plein écran des autres applications, hors de Cmd-`, et sur Linux sur chaque bureau virtuel d’un gestionnaire de fenêtres X11. Windows et Wayland la laissent sur celui où elle s’est ouverte, et `desktop.CanSetWindowWorkspaces` indique si cela prend effet : vrai sur macOS et sous un gestionnaire de fenêtres X11 qui prend en charge `_NET_WM_DESKTOP`. Une fenêtre principale passe sur tous les espaces de travail en appelant `desktop.SetWindowWorkspaces` depuis son contenu. Dans les tests, `nsgotest.App.Window` liste les demandes, et `nsgotest.Environment` définit `CanSetWindowWorkspaces`.

`desktop.RegisterHotKey` lie un raccourci global, qui fonctionne quelle que soit l’application au premier plan, sur macOS, sur Linux et dans une application Windows exécutée avec `nsgo.Run(App)`. Le `State` de son handle indique si le système l’a pris : `HotKeyPending` jusqu’à sa réponse, puis `HotKeyRegistered`, `HotKeyTaken` lorsqu’un autre enregistrement détient la combinaison, ou `HotKeyUnsupported` pour une touche ou une plateforme sans raccourcis globaux, de sorte qu’un enregistreur de raccourcis puisse demander une autre combinaison. Sur macOS, une combinaison n’est considérée comme prise que lorsque l’application la détient déjà ou qu’une autre application l’a enregistrée de manière exclusive. Sur Wayland, le compositeur demande à l’utilisateur de lier le raccourci : il est enregistré une fois que l’utilisateur l’a fait, et pris lorsque l’utilisateur ou le compositeur l’écarte. Dans les tests, `nsgotest.HotKeyTaken` et `nsgotest.HotKeyUnsupported` font répondre ainsi les combinaisons.

Avec `desktop.TitleBarHidden`, Windows et Linux proposent le menu de l’application sous forme de menu contextuel lorsque Alt est pressé et relâché seul, ou avec F10. Alt avec la première lettre ou le premier chiffre d’un libellé de premier niveau ouvre ce sous-menu ou choisit cette commande, dans l’écriture du libellé. Les liaisons de l’application conservent leurs touches ; Maj+F10 reste la touche du menu contextuel, et les autres combinaisons avec Alt restent au système. Échap ferme le menu contextuel et restaure le focus clavier.

```go
shortcut := desktop.RegisterHotKey(cx, "alt+space", launcher.Toggle)
warning := core.Text{ContentFunc: func() string {
	if shortcut.State() == desktop.HotKeyTaken {
		return "Another app uses Alt+Space"
	}
	return ""
}}
```

Sur macOS, Windows et Linux, les redimensionnements sur ordinateur de bureau attendent jusqu’à 16 ms la nouvelle disposition avant de dessiner. Gardez les calculs réactifs et les gestionnaires d’événements rapides pour que la disposition puisse tenir ce délai ; effectuez les E/S lentes dans une goroutine et publiez les résultats via `cx.Post`. Une application occupée continue de se redimensionner et affiche sa disposition lorsqu’elle a rattrapé son retard.

`desktop.SetWindowFrame` déplace une fenêtre lorsque `WindowFrame.Move` est vrai, en utilisant `X` et `Y` en points depuis le coin supérieur gauche de l’écran principal. Les dimensions non définies conservent leur taille actuelle. Vérifiez `desktop.CanPlaceWindows` : macOS, Windows et Linux sur X11 prennent en charge le positionnement ; une fenêtre de premier niveau Wayland ordinaire conserve la position choisie par le compositeur. `desktop.Frame` suit la position de la fenêtre native sur ces ordinateurs de bureau.

Sur macOS et Windows, `WindowFrame{Animated: true}` amène en douceur un panneau visible vers son nouveau cadre, y compris la taille d’un panneau ancré. Les panneaux masqués et la réduction des animations appliquent immédiatement le nouveau cadre. Les fenêtres macOS ordinaires utilisent l’animation d’AppKit.

Définissez `desktop.WindowOptions.Parent` sur une fenêtre ouverte de l’application pour fermer les deux lorsque la fenêtre parente se ferme. Sur les ordinateurs de bureau, la fenêtre enfant reste au-dessus de sa parente et la suit lorsqu’elle se masque ou se réduit ; une fenêtre enfant masquée explicitement reste masquée lorsque la parente revient. Sur macOS, elle se centre aussi sur sa parente lorsque `Frame` n’est pas défini et la suit en plein écran.

`WindowOptions.NoShadow` et `desktop.SetWindowShadow(cx, false)` retirent aussi l’ombre d’une fenêtre Windows dont la barre de titre est masquée, qui dessine son propre habillage ; une fenêtre dotée de la barre de titre du système conserve son cadre et son ombre, tout comme une fenêtre Linux que décore son gestionnaire de fenêtres, où `NoShadow` n’efface que l’ombre des décorations propres à GTK.

`desktop.SetWindowPointerEvents` laisse passer le pointeur à travers une fenêtre vers les fenêtres situées dessous, de n’importe quelle application : `WindowPointerNone` partout, comme le fait un affichage tête haute, et `WindowPointerPainted` partout où aucune vue ne dessine quelque chose ni n’écoute le pointeur, comme autour d’un animal de compagnie de bureau qui reçoit encore les appuis, les glissements et les clics droits sur lui-même ; la barre de titre que dessine le système compte comme rien de dessiné. `WindowOptions.PointerEvents` et `PanelOptions.PointerEvents` ouvrent une fenêtre ainsi. Une région `Desktop.WindowDrag` fait toujours glisser la fenêtre à l’intérieur d’un ancêtre pressable ; un contrôle pressable ou d’édition plus proche conserve sa propre saisie au lieu de faire glisser la fenêtre. Une fenêtre qui ne reçoit pas le pointeur n’entend aucun appui : l’application le lui rend donc depuis un raccourci, une icône de la zone de notification ou une autre fenêtre. Le code que détient un `Access` a besoin de `core.AccessOverlay`.

```go
desktop.OpenPanel(cx, desktop.PanelOptions{Width: 600, Height: 400, PointerEvents: desktop.WindowPointerPainted}, Pet)
```

Sur macOS et Windows, une image dans une fenêtre en mode peint peut définir `Image.HitTestAlpha` (TSX `hitTestAlpha`) à une valeur supérieure à 0 et au plus égale à 1. Par exemple, 0.5 reçoit le pointeur sur les pixels de l’image opaques au moins à moitié. Le seuil s’applique aussi à l’`OnPress` de l’image. Un fond ou une bordure de vue compte toujours comme peint. Zéro utilise toute la vue de l’image, de même que les images sur Linux.

Dans une fenêtre macOS en mode peint, `Video.HitTestAlpha` (TSX `hitTestAlpha`) applique le même seuil aux trames vidéo décodées. Cela fonctionne avec `Controls` désactivé et une piste vidéo non transformée, comme du HEVC avec alpha pour un animal de compagnie de bureau. Zéro, une trame indisponible et les autres hôtes de bureau utilisent les limites de la vue vidéo.

`desktop.NewTray` peut dessiner une icône vivante avec `TrayOptions.View` sur macOS et Windows. Windows ajuste le rendu transparent de la vue à la taille d’icône de la zone de notification et le met à jour lorsque la vue change ou qu’une image finit de se charger ; utilisez `TrayOptions.OnClick`, `Panel` et `Menu` pour ses actions. Linux affiche actuellement des icônes PNG ; dessiner sa vue de zone de notification reste un objectif.

`desktop.OpenPanel` ouvre un panneau tel que ceux qu’affichent un tel raccourci ou une icône de la zone de notification : une fenêtre sans bordure au-dessus des autres. Comme `desktop.OpenWindow`, `desktop.NewTray`, `mobile.OpenOverlay`, `audio.NewPlayer`, `audio.LoadSound` et `core.NewHolder`, il appartient directement au `cx` que vous passez plutôt qu’au propriétaire du gestionnaire en cours d’exécution, de sorte que passer la portée de l’application depuis l’`OnPress` d’un bouton maintient la fenêtre, le panneau ou le lecteur en vie après la disparition de la portée de ce bouton ; les primitives réactives (`Effect`, `Cleanup`, `AfterFunc`, `OnKey`) appartiennent au contraire au propriétaire en cours, que `cx.Run` change. Une carte contextuelle se dessine elle-même sur un panneau avec `Transparent`, qui n’a ni fond, ni cadre, ni ombre propres, de sorte que seuls les coins arrondis et la `BoxShadow` de la carte s’affichent, avec de la place pour l’ombre à l’intérieur du panneau ; `FitContent` dimensionne le panneau d’après son contenu, jusqu’à `Width` et `Height`, avant son premier affichage et à mesure que le contenu change. `desktop.Transparent` indique si le panneau ne dessine désormais rien de lui-même, comme pour une fenêtre transparente, là où `desktop.CanMakeWindowsTransparent` indique seulement si une fenêtre pourrait l’être : là où c’est faux, comme sur Linux sans compositeur, donnez un fond au contenu.

Sur macOS, un panneau sans `Transparent` utilise des coins carrés lorsque l’une de ses dimensions est inférieure à 24 points et restaure ses coins arrondis aux tailles normales.

```go
lookup := desktop.OpenPanel(cx, desktop.PanelOptions{Width: 380, Height: 520, Transparent: true, FitContent: true, HideOnBlur: true}, Card)
desktop.RegisterHotKey(cx, "alt+space", lookup.Toggle)
```

`desktop.NewTray` affiche `TrayOptions.Title` à côté de son image ou de sa vue dessinée sur macOS. Pour un menu contextuel personnalisé, lisez `tray.ClickBounds()` dans `OnClick` ou `Menu` et passez le rectangle disponible à `panel.ShowAt(bounds, core.PlaceBottom)`. Il utilise les mêmes points d’écran que `Env.Displays` et inclut le titre. Les limites ne sont pas disponibles avant le premier clic, après la suppression, sur Linux, et lorsque le shell Windows ne peut pas les fournir. Chaque clic remplace les limites précédentes, de sorte qu’un résultat indisponible ne réutilise jamais un ancien rectangle. Définir `TrayOptions.Panel` conserve plutôt le comportement intégré d’affichage ou de masquage au clic.

`PanelOptions.Dock` maintient un panneau centré le long du bord utilisable d’un écran à travers `Show`, `Focus`, `Toggle` et les redimensionnements du contenu. Définissez `Edge` sur `core.PlaceTop`, `PlaceBottom`, `PlaceLeft` ou `PlaceRight`, `Display` sur un identifiant de `Env.Displays` (vide pour l’écran principal), et `Margin` sur la distance à ce bord en points. Un panneau ancré ne peut pas utiliser `ShowAt` ni `MoveTo` ; `SetWindowFrame` ne change que sa taille. Une icône de la zone de notification l’affiche à son point d’ancrage plutôt que sous l’icône.

```go
panel := desktop.OpenPanel(cx, desktop.PanelOptions{
	Width: 320, Height: 80, NoActivate: true,
	Dock: desktop.PanelDock{Edge: core.PlaceBottom, Margin: 12},
}, Candidates)
panel.OnPlacementError(cx, func(err error) { /* offer another presentation */ })
panel.Show()
```

`desktop.CanDockPanels(cx)` signale la prise en charge de l’ancrage séparément du placement arbitraire des fenêtres. macOS, Windows et X11 la prennent en charge ; Wayland a besoin de la bibliothèque d’exécution facultative `libgtk4-layer-shell.so.0` et d’un compositeur implémentant `zwlr_layer_shell_v1`. NSGo charge cette bibliothèque avant GTK ; un hôte d’intégration qui a déjà chargé Wayland doit précharger gtk4-layer-shell avant de démarrer l’hôte. Une bibliothèque, un protocole ou un écran sélectionné manquant laisse le panneau masqué et appelle `OnPlacementError`. Un ancrage Wayland utilise la couche supérieure sans réserver d’espace de travail : `NoActivate` laisse le clavier à la fenêtre actuelle, tandis qu’un panneau sans cette option prend le focus clavier exclusif jusqu’à ce qu’il soit masqué. Les fenêtres Wayland ordinaires conservent leur placement habituel par le compositeur.

Un enregistreur de raccourcis permet à l’utilisateur de choisir une telle touche. L’`OnKeyDown` d’une vue `Focusable` reçoit chaque touche qui s’enfonce pendant qu’elle a le focus, avant les liaisons de l’application, avec les modificateurs maintenus, y compris un modificateur pressé seul ; renvoyer `true` soustrait la touche aux liaisons, de sorte qu’enregistrer Commande+S n’enregistre pas le document. `KeyEvent.Shortcut` écrit la pression comme la prennent `OnKey` et `RegisterHotKey`, chaque modificateur par son nom ("ctrl+shift+k", "alt+space", "shift+meta+k" pour Commande+Maj+K), et "" pour un modificateur seul. Son `OnKeyUp` reçoit la remontée de chacune de ces touches, même après que `OnKeyDown` l’a consommée ou que le focus s’est déplacé, avec les modificateurs encore maintenus. Les deux fonctionnent sur macOS, Windows, Linux, le web, où le navigateur conserve les raccourcis qu’il ne cède à aucune page, Apple TV et dans `nsgotest` ; Android livre les événements du clavier physique aux vues ayant le focus de manière asynchrone, de sorte que la valeur de retour de leur `OnKeyDown` ne peut pas consommer la touche. L’iPhone et l’iPad prennent en charge l’interrogation des touches physiques. Un raccourci que l’application a enregistré globalement prend sa touche avant la vue : un enregistreur le désenregistre donc pendant qu’il enregistre.

```go
core.View{
	Focusable: func() bool { return true },
	OnKeyDown: func(e core.KeyEvent) bool {
		if s := e.Shortcut(); s != "" {
			shortcut.Set(s)
		}
		return true
	},
	OnKeyUp: func(core.KeyEvent) { recording.Set(false) },
	Children: []core.Node{core.Text{ContentFunc: shortcut.Get}},
}
```

Un jeu ou un canevas interroge plutôt les touches. `core.KeysDown(cx)` liste les touches maintenues sur la fenêtre et `core.KeyHeld(cx, "left")` indique si l’une l’est : des lectures suivies, que lit aussi une boucle `RequestAnimationFrame`. La première lecture fait signaler par la plateforme chaque touche à partir de ce moment, de sorte qu’une touche déjà maintenue reste inconnue jusqu’à ce qu’elle soit pressée de nouveau ; les touches saisies dans un champ ou une vue web, ou pendant qu’une méthode de saisie compose, ne sont jamais maintenues, et une touche encore maintenue lorsque la fenêtre perd le focus remonte à ce moment-là. Elles fonctionnent là où fonctionne `OnKeyUp`, ainsi que sur iPhone et iPad avec un clavier physique.

Pour les boutons de la souris, `View.OnMouseDown` et `View.OnMouseUp` livrent un `core.MouseButtonEvent` pour chaque bouton (`core.MouseButtonLeft`, `core.MouseButtonRight`, `core.MouseButtonMiddle`, `core.MouseButtonBack`, `core.MouseButtonForward`) sur macOS, Windows, Linux, le web et `nsgotest`, sans faire de la vue un arrêt de focus clavier. `core.MouseButtonsDown(cx)` et `core.MouseButtonHeld(cx, core.MouseButtonRight)` sont des lectures suivies des boutons maintenus sur la fenêtre, qui s’effacent automatiquement lorsque la fenêtre perd le focus.

Pour une commande liée à une position physique, lisez `KeyEvent.Code`, `core.KeyCodesDown(cx)` ou `core.KeyCodeHeld(cx, "KeyW")`. Le code utilise les noms de `KeyboardEvent.code` : sur AZERTY, la position W du QWERTY a `Code: "KeyW"` et `Key: "z"`. `core.KeyCodeLabel(cx, "KeyW")` donne le libellé d’affichage de la disposition active, pour une invite. Sur iOS, Android, HarmonyOS et les navigateurs sans table de disposition, il utilise le dernier nom de touche signalé à cette position et renvoie une chaîne vide tant qu’aucune touche n’a été vue. HarmonyOS nomme à la fois la position et la touche d’après le `KeyCode` d’InputKit, car ArkUI ne signale aucun code de balayage, de sorte que là, le nom de la touche ne suit pas encore une disposition autre que QWERTY.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if core.KeyHeld(cx, "left") {
		x.Update(func(x float32) float32 { return x - 4 })
	}
	if core.KeyHeld(cx, "right") {
		x.Update(func(x float32) float32 { return x + 4 })
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

## Garder l’écran allumé

`cx.KeepAwake("Playing video", core.KeepAwakeOptions{Display: true})` maintient l’écran allumé jusqu’à ce que la fonction de libération renvoyée s’exécute ou que la portée courante soit libérée. Sur Android, le maintien s’applique tant que l’application est visible. Sur le web, il fonctionne dans un contexte sécurisé prenant en charge Screen Wake Lock, et la page redemande le verrou lorsqu’elle redevient visible. Safari peut exiger un appui ou une touche avant d’accorder la première demande. Les maintiens portant uniquement sur l’ordinateur n’ont aucun effet sur ces deux plateformes.

## Verrouillage du pointeur

Dans une caméra de jeu ou un glissement qui doit continuer au bord de l’écran, appelez `ref.LockPointer()` depuis un appui sur la vue qui détient la ref. Le pointeur reste en place et est masqué pendant que la vue reçoit un mouvement relatif dans `OnPointerDelta(dx, dy)`. Les valeurs positives vont vers la droite et vers le bas.

Appelez la fonction renvoyée pour libérer le verrou. Échap, la perte du focus par une fenêtre, ou le masquage ou la suppression de la vue le libèrent aussi. Vérifiez `cx.Env().CanLockPointer()` pour savoir si la surface le prend en charge. Le web exige un geste de l’utilisateur, comme un appui. Linux sur Wayland exige un compositeur qui prend en charge les contraintes de pointeur et le mouvement relatif.

## Limites des sons

Utilisez `audio.SetSoundLimits(cx, audio.SoundLimits{MaxSounds: 200, MaxDuration: 20 * time.Second})` avant de charger des sons pour changer les limites du moteur. `MaxEncodedBytes`, `MaxDecodedBytes` et `MaxPlaybacks` peuvent aussi être définis. Les champs nuls utilisent les valeurs par défaut : 8 Mio encodés et 5 secondes par son, 64 sources, 16 Mio décodés pour l’ensemble des sons, et 32 lectures superposées par sortie. Les sons existants restent chargés et comptent dans les nouvelles limites. Les portées restreintes conservent les limites par défaut dans le budget du moteur : les sons `nsgo/audio` d’un paquet TSX signé les atteignent quoi qu’ait défini l’application Go, tandis que le TSX propre à l’application prend les limites de l’application Go. Le SoundPool d’Android lit au plus 32 sons à la fois et décode au plus 1 Mio de chacun, quelles que soient les limites.

## Plein écran

`desktop.SetFullScreen(cx, true, displayID)` remplit l’écran désigné par `cx.Env().Displays()` sur macOS, Windows et Linux. Omettez l’identifiant pour utiliser l’écran actuel de la fenêtre. Un identifiant inconnu laisse la fenêtre là où elle est. Répéter la même demande n’a aucun effet. `desktop.SetFullScreen(cx, false)` quitte le plein écran et restaure le cadre précédent de la fenêtre. Sur le web, entrez depuis un gestionnaire d’appui. Le navigateur choisit l’écran, et `cx.Env().FullScreen()` suit l’entrée et la sortie, y compris par Échap.

## Écrans

`cx.Env().Display()` renvoie la `core.DisplayInfo` actuelle de la fenêtre et met à jour les liaisons lorsque la fenêtre passe sur un autre écran. `cx.Env().Displays()` liste les écrans que signale la plateforme. `RefreshRate` et `MaxRefreshRate` sont en Hz, zéro signifiant une fréquence indisponible. iOS signale le maximum de la dalle. HarmonyOS signale l’écran de la fenêtre, sa fréquence actuelle et sa fréquence maximale prise en charge. Le web signale l’écran actuel et marque sa mesure par rappel de trame avec `RefreshRateEstimated`. Dans les tests, définissez `Displays` et `DisplayID` via `nsgotest.Environment`.

## Synthèse vocale

`cx.Speak` lit un texte à voix haute avec la voix du système pour une langue, là où `cx.Env().CanSpeak()` indique que la plateforme parle : macOS, iOS, tvOS, Android, Windows, le web et Linux avec speech-dispatcher. Un seul énoncé est prononcé à la fois : un autre `Speak` interrompt celui en cours, tout comme `cx.StopSpeaking` et la fin de la portée qui l’a prononcé. `done` apprend une seule fois comment il s’est terminé : achevé, arrêté, ou en échec avec une `core.SpeakError`, comme `core.SpeakLanguageUnavailable` lorsqu’aucune voix installée ne parle la langue.

```go
cx.Speak("Guten Morgen", core.Voice{Lang: "de-DE", Rate: 0.8}, func(stopped bool, err error) {
	speaking.Set(false)
})
```

`Rate` et `Pitch` vont de 0,5 à 2, 1 étant la valeur propre de la voix. `cx.Voices("ja", ...)` liste les voix installées pour une langue, afin que l’application puisse demander à l’utilisateur d’en installer une qui manque ou le laisser en choisir une, dont l’`ID` va dans `core.Voice`. Les navigateurs ne parlent qu’après que l’utilisateur a appuyé sur quelque chose dans la page, et échouent avec `core.SpeakNotAllowed` auparavant. Les composants Go d’un paquet TSX ne parlent qu’avec la capacité `text_to_speech`.

## Audio en boucle

Définissez `audio.Voice.Loop` pour répéter la lecture d’un son jusqu’à `Stop` ou `Close`. Pour la musique en streaming, définissez `audio.PlayerOptions.LoopStart` et appelez `SetLoop(true)`. Le premier passage inclut l’introduction. Les passages suivants commencent au décalage. Un décalage nul répète la piste entière.

Sur macOS 14 et iOS 17 ou versions ultérieures, une piste finie utilise la file d’attente native pour préparer chaque répétition avant la fin de la précédente. Les versions antérieures d’Apple se déplacent au début de la boucle à la fin du passage en cours, avec un court blanc. Sur Linux et le web, un lecteur en boucle décode en mémoire une piste d’au plus 32 Mio et 5 minutes, environ 23 Mo par minute, et la répète sans blanc. Sur Linux, une piste plus volumineuse ou plus longue est lue en streaming par son propre décodeur et se répète aussi sans blanc ; sur le web, elle revient en arrière à la fin, avec un court blanc. Sur Windows et Android, une boucle avec un début revient à celui-ci à la fin, avec un court blanc. Sur le web, un lecteur en boucle à une vitesse autre que 1 revient aussi en arrière à la fin, car la lecture sans blanc du navigateur décalerait la hauteur avec la vitesse. Sur le web, un lecteur en boucle, comme un son, ne joue qu’après que l’utilisateur a appuyé sur quelque chose dans la page, et échoue avec `NotAllowed` auparavant.

## Biométrie

`cx.BiometricAvailability` indique si Face ID, Touch ID ou une empreinte digitale de l’appareil est enregistré ; Android ne précise pas quel type est enregistré, de sorte qu’il y signale une empreinte digitale lorsqu’une est enregistrée, et sinon le visage lorsque l’appareil dispose du matériel de reconnaissance faciale. `cx.AuthenticateBiometric` demande au système de confirmer l’utilisateur avec une raison, qui ne doit pas être vide, et signale un succès, un échec, une annulation ou une indisponibilité. Il ne choisit pas de repli par code d’accès pour l’application.

```go
cx.BiometricAvailability(func(kind core.BiometricKind) {
	if kind != core.BiometricNone {
		cx.AuthenticateBiometric("Confirm deleting this account", func(result core.BiometricResult) {
			if result == core.BiometricSuccess {
				deleteAccount()
			}
		})
	}
})
```

Les applications utilisant Face ID déclarent `NSFaceIDUsageDescription` sous `apple.info` ; les applications Android listent `USE_BIOMETRIC` sous `android.permissions`.

## Connexion

`cx.AuthSession` connecte l’utilisateur auprès d’un fournisseur OAuth dans la session d’authentification du système plutôt que dans une vue web : `ASWebAuthenticationSession` sur macOS, iOS et watchOS, un Custom Tab ou le navigateur sur Android, le navigateur par défaut sur Windows, Linux et un Mac sans schéma, et une fenêtre contextuelle sur le web. Sur le web, la page de redirection envoie aussi son rappel sur un `BroadcastChannel` de même origine, de sorte qu’une connexion aboutit lorsque la `Cross-Origin-Opener-Policy` du fournisseur coupe la fenêtre contextuelle de la page. Une telle fenêtre contextuelle apparaît fermée à la page, de sorte que fermer une fenêtre contextuelle après qu’elle a affiché la page du fournisseur ne termine la session par une annulation qu’au bout de 5 minutes ; donnez à la session un bouton d’annulation. L’application conserve le protocole : elle crée le vérificateur PKCE et l’état, construit l’URL d’autorisation pour la redirection qui lui est donnée, et vérifie le rappel avant d’échanger son code. Annuler n’est pas échouer : `errors.Is(err, core.AuthCancelled)` signale que l’utilisateur a fermé la session, ou que l’`cancel` renvoyée l’a terminée.

```go
verifier, challenge, state := newPKCE() // the app's own
cx.AuthSession(func(redirect string) string {
	q := url.Values{"client_id": {clientID(redirect)}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid email"},
		"state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	return "https://accounts.example.com/authorize?" + q.Encode()
}, core.AuthSessionOptions{Scheme: "com.example.app", Path: "/oauth2redirect", Timeout: 5 * time.Minute}, func(callback string, err error) {
	if errors.Is(err, core.AuthCancelled) {
		return
	}
	if err != nil {
		showError(err)
		return
	}
	u, _ := url.Parse(callback)
	if u.Query().Get("state") == state {
		exchangeCode(u.Query().Get("code"), verifier)
	}
})
```

La redirection est `com.example.app:/oauth2redirect` lorsqu’un schéma est défini (Windows, Linux et Android en ont besoin dans les `url_schemes` de `nsgo.yaml` pour que le système d’exploitation remette le rappel à l’application en cours d’exécution ; la session système de macOS, d’iOS et de watchOS l’intercepte sans cette déclaration), et `http://127.0.0.1:<port>/oauth2redirect` sur la boucle locale des ordinateurs de bureau : enregistrez donc les deux auprès du fournisseur, souvent comme clients distincts. Laissez `Scheme` vide pour la boucle locale seule, ou définissez-le à `https` avec `Host` pour le domaine associé de l’application, qu’interceptent iOS 17.4, macOS 14.4 et watchOS 10.4. watchOS exige un schéma ou un domaine HTTPS associé ; il n’a pas de repli sur la boucle locale, et le minimum existant de NSGo, watchOS 26, s’applique toujours. Un onglet de navigateur fermé n’apprend rien à une session en boucle locale : donnez-lui donc un `Timeout` ou un bouton d’annulation. `Ephemeral` demande à macOS, iOS et watchOS de ne pas partager les cookies du navigateur, et demande un Custom Tab privé sur Android lorsque le navigateur en prend un en charge ; ne présumez pas d’une isolation des cookies sur Android. tvOS et les mini-programmes répondent `core.AuthUnavailable` pour l’instant. En TSX, `AuthSession.start(url, options)` a besoin de la capacité `auth_session` et donne une session dont le `result` se résout avec le rappel, ou null une fois annulée.

Android consomme un rappel correspondant avant les `OnOpenLinks` ordinaires, et un retour sans rappel termine la session par une annulation. Son rappel doit utiliser un schéma privé appartenant à l’application ; un schéma vide, un rappel HTTPS ou un schéma non déclaré renvoie `core.AuthUnavailable`, tout comme un runtime Android construit avant les sessions de connexion.

Une session de navigateur ne consomme que son lien de rappel correspondant ; les liens sans rapport et tardifs atteignent toujours `OnOpenLinks`. Démarrer une autre session de navigateur pour le même URI de rappel annule la précédente ; des URI distincts peuvent attendre dans des fenêtres différentes.

Utilisez exactement la redirection passée à la fonction de construction de l’URL d’autorisation. Les redirections web et en boucle locale sur ordinateur de bureau encodent les chemins Unicode et résolvent les segments de points avant que la fonction ne les reçoive ; un `Path` en boucle locale est du texte décodé, de sorte qu’un `%` littéral est lui aussi échappé.

Les mini-programmes utilisent plutôt `cx.SignInMiniProgram(core.MiniProgramSignInOptions{}, done)`. Le rappel reçoit un `core.MiniProgramCredential` avec sa `Platform` (`MiniProgramWeChat`, `MiniProgramQQ`, `MiniProgramDouyin` ou `MiniProgramAlipay`) et un `Code` non vide et de courte durée. Envoyez-le à votre serveur pour l’échange de code de l’hôte ; gardez-y le secret de l’application. Il s’agit de `wx.login`/`qq.login`/`tt.login`, ou de l’autorisation de base `auth_base` d’Alipay, sans demander d’informations de profil. Il n’ouvre aucune URL et n’exécute aucun constructeur d’URL OAuth.

`cx.Env().CanSignInMiniProgram()` indique la disponibilité de l’API de l’hôte. Les autres cibles répondent `core.MiniProgramSignInUnavailable` ; le code sans `core.AccessMiniProgramSignIn` reçoit `AccessDenied`. Douyin interroge silencieusement par défaut : un hôte non connecté renvoie `MiniProgramSignInNotSignedIn`, et `MiniProgramSignInOptions{Prompt: true}` autorise sa fenêtre de connexion au compte de l’hôte. Fermer cette fenêtre renvoie `MiniProgramSignInCancelled`. Un code d’appareil anonyme ne compte jamais comme un utilisateur connecté. Les tests définissent `nsgotest.Environment(core.Environment{MiniProgramSignIn: true})` et `app.MiniProgramSignIn` ; `app.MiniProgramSignIns` enregistre les demandes. Le TSX ne s’exécute pas dans les mini-programmes.

Une application dont les vues web doivent être connectées comme l’utilisateur l’est déjà ailleurs, par exemple dans son propre navigateur, écrit les cookies de cette connexion avec `cx.SetWebCookies`, y compris les cookies HttpOnly, que les scripts de page ne peuvent pas définir, puis charge les pages : done s’exécute une fois que le magasin les contient. Le magasin est le magasin persistant que partagent toutes les vues web de l’application, de sorte que les cookies dotés d’un `Expires` survivent à l’application ; les vues en navigation privée ne les voient pas, sauf sur Android, qui conserve un seul magasin par application. HarmonyOS, le web, tvOS et watchOS répondent `Unavailable`.

```go
cx.SetWebCookies([]core.WebCookie{
	{Name: "SID", Value: sid, Domain: ".example.com", Expires: expires, Secure: true, HTTPOnly: true},
}, func(err error) {
	if err == nil {
		page.Set(core.WebSrc("https://www.example.com/"))
	}
})
```

Pour supprimer un cookie, écrivez-le à nouveau avec le même nom, le même domaine et le même chemin et un `Expires` dans le passé ; une expiration nulle crée un cookie de session. Attendez le rappel avant de charger une page qui doit constater la suppression.

En sens inverse, `cx.WebCookies` liste les cookies du magasin, y compris les cookies HttpOnly ainsi que leur domaine, leur chemin, leur expiration et leurs attributs, que `document.cookie` ne peut pas lire, qu’une page du site soit affichée ou non : un `core.WebCookieFilter` sélectionne ceux qu’enverrait une requête vers une `URL`, ou ceux d’un `Domain` et de chaque hôte en dessous, et le filtre vide les sélectionne tous. Une application qui appelle le site depuis Go avec la connexion de l’utilisateur, ou qui montre où l’utilisateur est connecté, les lit ainsi. `cx.ClearWebsiteData` déconnecte l’utilisateur de tous les sites : il supprime les cookies du magasin, le stockage local et de session, IndexedDB, les service workers et les caches, et done s’exécute une fois qu’ils ont disparu. Le `CookieManager` d’Android ne répond que les noms et valeurs qu’enverrait une URL, de sorte que là, seul un filtre `URL` liste des cookies, en remplissant `Domain` avec son hôte et en laissant les autres champs vides, et qu’un filtre `Domain` ou le filtre vide répond `Unavailable` ; l’effacement y laisse le cache HTTP. HarmonyOS, le web, tvOS et watchOS répondent `Unavailable` aux deux.

```go
cx.WebCookies(core.WebCookieFilter{Domain: "example.com"}, func(cookies []core.WebCookie, err error) {
	signedIn.Set(err == nil && slices.ContainsFunc(cookies, func(c core.WebCookie) bool { return c.Name == "SID" }))
})
```

## Se connecter avec Apple

`cx.SignInWithApple` affiche la feuille système Se connecter avec Apple depuis le bouton propre à l’application sur macOS, iOS et tvOS, là où `cx.Env().CanSignInWithApple()` est vrai. Il répond une seule fois avec l’identifiant, ou avec une erreur : `core.AppleSignInCancelled` lorsque l’utilisateur ferme la feuille, ce qui n’est pas un échec, sinon `AppleSignInFailed` ou, sur les autres plateformes, `AppleSignInUnavailable`. Apple ne donne l’adresse e-mail et le nom qu’à la première connexion de l’utilisateur à l’application : conservez-les donc à ce moment-là. `Nonce` entre dans la requête sous forme de son SHA-256 en hexadécimal minuscule, que contient la revendication `nonce` du jeton d’identité : le serveur hache le nonce qu’il a donné à l’application et compare. `cx.AppleCredentialState` indique si l’utilisateur l’a révoqué depuis. Dans les tests sans interface, `app.AppleSignIn` reçoit une `nsgotest.AppleSignInRequest` et fournit l’identifiant ou l’erreur.

```go
nonce := rand.Text() // also sent to the server, which checks the token's claim against its SHA-256
cx.SignInWithApple(core.AppleSignInOptions{FullName: true, Email: true, Nonce: nonce}, func(c core.AppleCredential, err error) {
	switch {
	case errors.Is(err, core.AppleSignInCancelled):
	case err != nil:
		showError(err)
	default:
		signInOnServer(c.IdentityToken, c.AuthorizationCode, nonce, c.GivenName, c.FamilyName, c.Email)
	}
})
cx.AppleCredentialState(savedUser, func(s core.AppleCredentialState, err error) {
	if err == nil && s == core.AppleCredentialRevoked {
		signOut()
	}
})
```

L’application doit être signée avec l’entitlement Se connecter avec Apple, que `build` ajoute lorsque `nsgo.yaml` le demande ; sans lui, la feuille échoue :

```yaml
apple:
  sign_in_with_apple: true  # com.apple.developer.applesignin, in iOS and tvOS packages and Mac App Store builds
```

Xcode enregistre la capacité pour les builds sur appareil depuis `nsgo dev` ; un profil de provisionnement dans `sign.profile` ou `sign.macos_profile` doit provenir d’un App ID où Se connecter avec Apple est activé. macOS ne l’accorde qu’avec un profil, de sorte qu’une image disque ou une exécution `nsgo dev` sur un Mac ne peut pas se connecter, pas plus que le simulateur, dont les builds ne revendiquent aucun entitlement de développeur.

## Achats intégrés

`cx.Products`, `cx.Purchase` et les appels qui les accompagnent vendent via l’App Store sur macOS, iOS et tvOS, là où `cx.Env().CanPurchase()` est vrai, dans une build dont le `nsgo.yaml` demande StoreKit :

```yaml
apple:
  purchases: true  # StoreKit's library, in macOS, iOS and tvOS packages; no entitlement is needed
```

Affichez les prix que donne la boutique, `DisplayPrice`, jamais des prix propres à l’application : la boutique fixe elle-même les prix de chaque vitrine. Un achat répond une seule fois, avec la transaction ou une erreur : `core.PurchaseCancelled` lorsque l’utilisateur ferme la feuille, `core.PurchasePending` tant qu’il attend une approbation, comme avec Demander l’autorisation d’acheter, et sinon `core.PurchaseFailed`, avec les mots de la boutique, ou `PurchasesUnavailable` ailleurs. Envoyez le `Signed` de la transaction, le JWS de la boutique, au serveur de l’application pour vérification, livrez ce qui a été acheté, puis terminez-la ; la boutique remet une transaction non terminée à l’application à chaque lancement.

`cx.ManageSubscriptions` ouvre l’interface de gestion des abonnements d’Apple (la feuille système sur iOS, la page des abonnements de l’App Store sur macOS et tvOS). `cx.RequestRefund` lance une demande de remboursement pour une transaction et répond `core.RefundSuccess` ou `core.RefundCancelled`.

```go
cx.OnPurchaseUpdate(func(t core.Transaction) { // renewals, approved purchases, other devices', refunds, and unfinished ones at launch
	verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
})
cx.Products([]string{"com.example.pro.monthly"}, func(products []core.Product, err error) {
	if err == nil && len(products) > 0 {
		price.Set(products[0].DisplayPrice)
	}
})
cx.Purchase("com.example.pro.monthly", core.PurchaseOptions{AccountToken: userUUID}, func(t core.Transaction, err error) {
	switch {
	case errors.Is(err, core.PurchaseCancelled):
	case errors.Is(err, core.PurchasePending):
		showWaiting() // its transaction comes through OnPurchaseUpdate
	case err != nil:
		showError(err)
	default:
		verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
	}
})
```

`FinishTransaction` renvoie `PurchaseNotFound` pour une transaction déjà terminée, y compris celle que renvoie `CurrentEntitlements` ; un achat non consommable terminé reste possédé.

Enregistrez `OnPurchaseUpdate` au démarrage de l’application : les mises à jour qui arrivent avant tout gestionnaire attendent le premier. `cx.CurrentEntitlements` répond ce que l’utilisateur possède actuellement, pour le débloquer au lancement, et un bouton Restaurer les achats appelle `cx.SyncPurchases`, qui peut demander à l’utilisateur de se connecter. `AccountToken` est un UUID propre à l’application que portent la transaction et les notifications serveur de l’App Store. macOS ne vend que dans une build Mac App Store, et un App Clip ne vend rien ; Android, watchOS, Windows, Linux, le web et les mini-programmes répondent `PurchasesUnavailable` pour l’instant. Testez les achats avec `nsgotest`, dont `Products`, `PurchaseAnswer`, `UpdatePurchase` et `UnfinishedTransactions` remplacent la boutique. `app.PurchaseRequests` enregistre les opérations de la boutique sous forme de valeurs `nsgotest.PurchaseRequest`. En TSX, le module `Purchases` a besoin de la capacité `purchases` : `purchase` se résout avec `{ status: "purchased", transaction }`, `"pending"` ou `"cancelled"`, et `on("update", listener)` reçoit les mises à jour.

## Orientation de l’interface

Appelez `cx.RequestOrientation(core.OrientationLandscape)` dans la portée d’un lecteur plein écran pour demander le mode paysage à iOS, Android, HarmonyOS ou au navigateur. La dernière demande dont la portée est vivante prend effet. Lorsque cette portée se termine, la demande précédente reprend ou l’application revient à sa politique d’orientation habituelle.

Sur iOS 16 et versions ultérieures, la demande peut faire pivoter l’interface immédiatement. Sur les versions antérieures d’iOS, elle restreint la prochaine rotation du système. Les `IOS.SupportedOrientations` d’une fenêtre modale présentée ont la priorité. Sur Android et HarmonyOS, un mélange de directions portrait et paysage que le système ne peut pas représenter est réduit à une seule direction autorisée, en privilégiant celle déjà affichée. Android peut ignorer les demandes d’orientation en mode grand écran ou multifenêtre.

Dans une application web, appelez `desktop.ToggleFullScreen(cx)` depuis un gestionnaire d’appui qui ouvre la portée du lecteur. Le navigateur passe en plein écran à partir de l’appui, puis NSGo applique la demande d’orientation du lecteur. Quitter le plein écran libère le verrouillage. La console du navigateur signale les opérations non prises en charge et les refus.

`nsgotest.App.Orientation` enregistre le dernier masque demandé, zéro correspondant à la politique d’orientation habituelle.

## Avis sur les boutiques

`cx.RequestReview()` demande une invite de notation intégrée à l’application à l’App Store sur macOS et iOS, ou à Google Play sur Android. `cx.Env().CanRequestReview()` indique si le service est disponible. La boutique décide si et quand afficher l’invite et ne signale aucun résultat. Utilisez une build de développement pour tester l’invite d’Apple. Les builds TestFlight la suppriment.

Les paquets Android autonomes incluent le SDK Play Review, son activité de dialogue et la requête de visibilité du Play Store ; leur construction ne nécessite ni SDK Android ni JDK. Un appareil sans Play Store activé signale le service comme indisponible. Testez un paquet publié à l’aide du [partage interne d’applications](https://developer.android.com/guide/playcore/in-app-review/test).

Lorsque vous intégrez NSGo, ajoutez `com.google.android.play:review:2.0.2` à l’hôte Gradle. Dans le manifeste de l’hôte, ajoutez `<package android:name="com.android.vending" />` sous `<queries>` afin que l’application puisse voir le Play Store. Si l’hôte réduit son code, préservez les API Play Review et Tasks avec les règles ci-dessous, car NSGo les charge par leur nom.

```proguard
-keep class com.google.android.play.core.review.ReviewManagerFactory { *; }
-keep interface com.google.android.play.core.review.ReviewManager { *; }
-keep class com.google.android.play.core.review.ReviewInfo { *; }
-keep class com.google.android.gms.tasks.Task { *; }
-keep interface com.google.android.gms.tasks.OnCompleteListener { *; }
```

Dans les tests, définissez `CanRequestReview` dans `nsgotest.Environment` et vérifiez `nsgotest.App.ReviewRequests`.

## Gestes système sur les bords

Un jeu dont les commandes tactiles se trouvent au bord de l’écran peut appeler `mobile.DeferSystemGestures` pendant que les commandes sont actives. Utilisez la fonction renvoyée pour libérer la demande plus tôt. Elle prend aussi fin lorsque sa portée est nettoyée. Les demandes de plusieurs portées dans la même fenêtre se combinent.

```go
release := mobile.DeferSystemGestures(cx, mobile.SystemGestureOptions{
	IOSEdges: mobile.GestureEdgeBottom,
	AndroidRects: []core.Bounds{{
		Origin: core.Point{Y: 120},
		Size:   core.Size{Width: 24, Height: 96},
	}},
})
```

iOS utilise les bords physiques de l’écran. Android 10 et versions ultérieures utilisent des rectangles en points de surface mesurés depuis le coin supérieur gauche. Limitez les rectangles aux abords des commandes qui en ont besoin. Android réserve son geste d’accueil et plafonne la zone qu’une application peut exclure du geste Retour. `nsgotest.App.SystemGestures` donne aux tests la demande combinée actuelle sur leur surface principale. Utilisez `Surface.SystemGestures()` pour une autre fenêtre.

## Surcouches sur Android

`mobile.OpenOverlay` affiche une vue de l’application au-dessus des autres applications sur Android, un arbre qui lui est propre et qui partage les signaux de l’application, et un service de premier plan maintient l’application en marche pendant son affichage. L’application liste `SYSTEM_ALERT_WINDOW` sous `android.permissions`, et l’utilisateur l’accorde dans les réglages du système, qu’ouvre `mobile.RequestOverlayPermission` ; sans elle, la surcouche se termine aussitôt et `OnEnd` apprend pourquoi. Elle reçoit les touchers mais laisse les touches et les touchers alentour à l’application au premier plan, sauf si elle est `Focusable` ou `Passive`, et se déplace avec `desktop.SetWindowFrame` depuis son contenu. Les autres plateformes n’en ouvrent aucune.

```go
mobile.RequestOverlayPermission(cx, func(p core.OverlayPermission) {
	if p == mobile.OverlayGranted {
		mobile.OpenOverlay(cx, mobile.OverlayOptions{Width: 200, Height: 80}, Score)
	}
})
```

## Localisation et mouvement

`cx.Locate` obtient une position, `cx.WatchLocation` diffuse des positions et `cx.WatchMotion` diffuse des échantillons d’accéléromètre, de gyroscope, de magnétomètre et de mouvement fusionné de l’appareil. Chacun renvoie un `*core.SensorStream` dont `Stop` l’annule, comme le fait la fin de la portée qui l’a démarré ; les rappels s’exécutent sur la goroutine du moteur. Les autorisations sont `core.PermissionLocation`, `PermissionBackgroundLocation` et `PermissionMotion` ([autorisations](permissions.md)) : démarrer un flux ne demande jamais rien, et un flux sans son autorisation échoue avec `core.SensorNotAllowed`.

```go
cx.RequestPermission(core.PermissionLocation, func(s core.PermissionStatus) {
	if !s.Allowed() {
		return
	}
	cx.Locate(core.LocationOptions{}, func(f core.LocationFix, err error) {
		if err == nil {
			showPosition(f.Latitude, f.Longitude, f.Approximate)
		}
	})
})
```

`Locate` attend une nouvelle position, 30 secondes par défaut (`Timeout`) ; seul `MaxAge` lui permet d’en prendre une en cache, ce qu’elle indique avec `Cached` et `Age`. `Approximate` signale une position que l’utilisateur n’a autorisée qu’approximativement ; Android rafraîchit une telle position au plus toutes les dix minutes : donnez donc à `Locate` un `MaxAge` pour elle sur cette plateforme. Les coordonnées sont en WGS 84 ; une mesure que le fournisseur ne possède pas a `Valid` à false, et `Altitude` est ellipsoïdale là où `MSLAltitude` est au-dessus du niveau moyen de la mer. Le mouvement utilise les axes naturels de l’appareil, l’accélération en m/s² (+9,81 sur z, à plat face vers le haut), la rotation en rad/s et le champ magnétique en µT, toutes les 20 ms par défaut, borné entre 10 ms et 1 s. Sous charge, un flux conserve l’échantillon le plus récent par capteur et compte ceux qu’il a remplacés dans `Dropped`.

Les flux continuent pendant que l’application est inactive et se mettent en pause en arrière-plan, en marquant le premier échantillon suivant avec `Gap` ; un `Locate` en attente conserve le reste de son délai. `LocationOptions.Background` maintient un flux de localisation en arrière-plan sous `PermissionBackgroundLocation` : les applications Apple ajoutent le mode d’arrière-plan `location`, et sur Android un service de premier plan affiche une notification avec `NotificationTitle` et `NotificationBody` et maintient l’application en marche après la fin de son activité. Rien ne démarre un processus que le système a arrêté.

```yaml
apple:
  info:
    NSLocationWhenInUseUsageDescription: Shows where you are on the map.
    NSLocationUsageDescription: Shows where you are on the map.  # macOS
    NSMotionUsageDescription: Counts your reps.
    UIBackgroundModes: [location]                                # background streams
android:
  permissions: [ACCESS_COARSE_LOCATION, ACCESS_FINE_LOCATION, FOREGROUND_SERVICE, FOREGROUND_SERVICE_LOCATION]
```

iOS et watchOS fournissent la localisation et le mouvement, macOS la localisation, tvOS une position à la fois et Android les deux via les fournisseurs propres du framework. Linux obtient les positions et les flux de GeoClue 2 sur le bus système, dont l’agent interroge l’utilisateur au démarrage de la première demande de l’application ; comme GeoClue ne peut pas être interrogé sans afficher de demande, l’autorisation se lit comme non demandée tant que l’application ne l’a pas demandée au cours de cette exécution, et la localisation en arrière-plan n’est pas disponible. Windows les obtient du `Geolocator` de WinRT ; l’autorisation se lit comme refusée tant que la localisation est désactivée pour l’application, comme accordée une fois que l’application l’a demandée au cours de cette exécution et l’a obtenue, et sinon comme non demandée, et la localisation en arrière-plan n’y est pas disponible non plus. Un navigateur en contexte sécurisé fournit des positions de géolocalisation et, sur mobile, des mesures d’accéléromètre, de gyroscope et de mouvement fusionné. Appelez `cx.RequestPermission(core.PermissionMotion, done)` depuis un gestionnaire d’appui sur Safari. Le navigateur signale la localisation comme indisponible sans `navigator.geolocation`, et le mouvement comme indisponible sur les ordinateurs de bureau et pour les flux de magnétomètre. En TSX, `Sensors` dispose de `permission`, `requestPermission`, `locate`, `watchLocation` et `watchMotion`, sous les capacités de paquet `location`, `backgroundLocation` et `motion`, en millisecondes.

## Audio

Le paquet `audio` lit des sons courts décodés une fois en mémoire (`audio.LoadSound`) et de l’audio long en streaming (`audio.NewPlayer`) là où `cx.Env().CanPlayAudio()` est vrai ; ailleurs, les appels échouent avec `audio.Unavailable`.

```go
audio.LoadSound(cx, audio.Source{URI: "asset:sounds/click.wav"}, func(click *audio.Sound, err error) {
	if err != nil {
		return
	}
	click.Play(audio.Voice{})                                  // center, full volume, normal rate
	click.Play(audio.Voice{Volume: 0.5, Rate: 1.2, Pan: -0.5}) // overlaps, panned left
})
```

`audio.Voice` définit pour chaque lecture `Volume` (de 0 à 1, 0 valant 1), `Rate` (de 0,5 à 2, 0 valant 1) et `Pan` (de -1, entièrement à gauche, à 1, entièrement à droite, 0 étant centré) ; les valeurs hors de ces plages sont bornées et NaN prend la valeur par défaut. `Sound.Stop` arrête toutes les lectures du son et `Close` le libère, comme le fait sa portée propriétaire lorsqu’elle est libérée. `Player` prend en charge `Play`, `Preload`, `Pause`, `Seek`, `SetVolume`, `SetRate`, `SetLoop` et `Close`. macOS, iOS, Windows, Linux, Android et le web lisent de l’audio aujourd’hui. Les backends Apple, Windows et navigateur prennent en charge WAV, AAC et MP3. Linux utilise les codecs de GStreamer, et son lecteur en streaming lit à 1×. Android met un lecteur en pause lorsque sa vue quitte l’écran et limite chaque son décodé à 1 Mio. tvOS, watchOS, HarmonyOS et les mini-programmes signalent encore `CanPlayAudio` à false.

Les navigateurs exigent un geste de l’utilisateur pour une lecture audible. Chargez les sons courts à l’avance, puis appelez `Sound.Play` après un clic, un toucher ou l’appui sur une touche. Un son lu avant un geste est muet. Le rappel `Play` d’un lecteur reçoit `audio.NotAllowed` si le navigateur bloque la lecture. Réessayez depuis un geste de l’utilisateur.

`Sound.Play` renvoie une `audio.Playback`, qui, sur macOS, iOS, Windows, Linux et le web, peut définir son propre volume et sa propre vitesse ou s’arrêter indépendamment. `Ended()` est réactif et devient vrai lorsque cette lecture se termine, s’arrête, est évincée par la limite de voix ou ne peut pas démarrer. Les backends sans commandes individuelles renvoient un handle terminé.

Sur iOS, un moteur audio propre à une bibliothèque qui joue en dehors d’`audio` et de `Video`, comme une `AudioQueue`, détient l’unique session audio de l’application avec `audio.HoldSession(core.AudioSessionPlayback)` pendant qu’il joue, et appelle la fonction de libération renvoyée lorsqu’il s’arrête. Il prend les niveaux que prend l’`AudioSession` d’un lecteur, aux côtés des vidéos, lecteurs et enregistrements de l’application, de sorte qu’`AudioSessionPlayback` et `AudioSessionMixWithOthers` continuent de jouer lorsque le commutateur Sonnerie/Silence est sur silence. Ailleurs, il ne détient rien.

Utilisez `audio.Voice{Muted: true}` pour démarrer un effet en silence et `playback.SetVolume(0)` pour rendre muet un effet actif. Pour l’audio long, `audio.NewPlayer` diffuse une source et `player.SetMuted(true)` rend la sortie muette pendant que la lecture continue. Réactiver le son restaure le volume configuré du lecteur. `Voice.Volume` et `Player.SetVolume` utilisent zéro pour le plein volume par défaut.

En TSX, `nsgo/audio` expose `supported()` et `loadSound(uri)`, conditionnés par la capacité de paquet `audio`. `loadSound` se résout avec un son dont `play({ volume, rate, pan, loop, muted })` renvoie une lecture dotée de `setVolume`, `setRate`, `stop` et d’un `ended` réactif ; là, un volume de 0 est le silence. Un chargement refusé est rejeté avec une `Error` dont la `reason` est le nom de l’erreur audio, comme `quota-exceeded`, et dont le `code` est `ERR_ACCESS_DENIED` sans la capacité, `ERR_NSH_QUOTA_EXCEEDED` au-delà d’une limite et `ERR_FEATURE_UNAVAILABLE_ON_PLATFORM` là où rien ne lit d’audio. Un son n’est libéré que par `close()` ou lorsque sa vue disparaît ; dans un bac à sable, chaque chargement compte dans `Limits.Sounds` (256 par défaut) jusqu’à ce qu’il échoue ou que son son soit fermé.

## Manettes de jeu

Le paquet `gamepad` lit les manettes de jeu connectées selon la disposition W3C Standard Gamepad (de `ButtonSouth`/`ButtonA` à `ButtonHome`, `AxisLeftX`/`AxisLeftY`/`AxisRightX`/`AxisRightY`). `cx.Env().SupportsGamepads()` indique si la cible prend en charge les manettes (macOS, iOS, Windows, Linux, Android et les navigateurs dotés de l’API Gamepad).

`gamepad.Pads(cx)` est une lecture réactive suivie qui refait le rendu des portées lorsque des manettes se connectent ou se déconnectent, sans refaire le rendu lors des changements de boutons ou de sticks. Chaque `gamepad.Pad` expose sa `Family` (`FamilyXbox`, `FamilyPlayStation`, `FamilyNintendo` ou `FamilyGeneric`, également classée via `gamepad.DetectFamily`), son `VendorID` et son `ProductID` USB là où la plateforme les signale, et `CanRumble` pour la prise en charge des moteurs haptiques. Dans une boucle `RequestAnimationFrame`, `gamepad.State(cx, id)` renvoie les boutons de la manette (`[0, 1]`, avec `Pad.Pressed` vrai à `>= 0.5`) et ses axes (`[-1, 1]`, négatifs vers la gauche/le haut et positifs vers la droite/le bas), capturés une fois par trame d’affichage.

`gamepad.Rumble(cx, id, lowFreq, highFreq, duration)` pilote les moteurs de vibration basse fréquence (lourd/gauche) et haute fréquence (léger/droit) avec des intensités bornées à `[0, 1]`, en remplaçant toute vibration en cours sur cette manette et en s’arrêtant automatiquement après `duration`, lorsque l’application passe en arrière-plan ou perd le focus (`cx.Env().Lifecycle() != core.LifecycleActive`), lorsque `cx` est libéré, ou lorsque la manette se déconnecte. `gamepad.StopRumble(cx, id)` arrête la vibration plus tôt ; les manettes avec `CanRumble == false` ignorent proprement les appels de vibration.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if pad, ok := gamepad.State(cx, 0); ok {
		move(pad.Axes[gamepad.AxisLeftX], pad.Axes[gamepad.AxisLeftY], pad.Pressed(gamepad.ButtonA))
		if hit {
			gamepad.Rumble(cx, pad.ID, 0.8, 0.3, 180*time.Millisecond)
		}
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

Appelez `gamepad.EnableFocusNavigation(cx)` pour activer la navigation entre les arrêts de focus natifs avec une manette (la croix directionnelle et le stick gauche déplacent le focus, `ButtonSouth` active le contrôle ayant le focus et `ButtonEast` déclenche le retour). Les vues de jeu actives appellent `gamepad.CaptureInput(cx)` pour mettre en pause la navigation par focus tant qu’elles sont montées. En TSX, `nsgo/gamepad` expose `supported()`, `pads()`, `state(id)`, `Button` et `Axis`, conditionnés par la capacité de paquet `gamepad`.

## Synchronisation audio

Utilisez `audio.Now(cx)` comme horloge pour la lecture programmée. Définissez `audio.Voice.StartAt` à un instant futur lorsque `audio.CanSchedule(cx)` est vrai. Appelez `audio.OutputLatency(cx, func(latency time.Duration, known bool) { ... })` pour la sortie audio active. macOS, iOS, Linux et le web prennent en charge les démarrages programmés ; sur Windows et Android, `CanSchedule` vaut false et un `StartAt` joue immédiatement. La valeur `known` du rappel indique si la sortie a signalé une latence, ce que ne font pas Windows et Android.

## Services de jeu

Le paquet `gameservices` fournit une API indépendante du fournisseur pour la connexion des joueurs, les succès, les classements et les surcouches natives de Game Center / Play Games là où `cx.Env().CanUseGameServices()` est vrai. Chaque rappel s’exécute sur la goroutine du moteur, de sorte qu’il peut définir des signaux directement sans bloquer les trames.

```go
if cx.Env().CanUseGameServices() {
	gameservices.Authenticate(cx, func(p gameservices.Player, err error) {
		if err == nil && p.Authenticated {
			playerName.Set(p.DisplayName)
			gameservices.ReportAchievement(cx, "first-run", 100, nil)
			gameservices.SubmitScore(cx, "high-scores", 1250, nil)
		}
	})
}
```

`gameservices.ReportAchievement` borne la progression à `0..100` (`100` débloque le succès). `gameservices.LoadScores` lit des lignes classées `gameservices.ScoreEntry` pour `Global` ou `Friends` sur `AllTime`, `Week` ou `Today`, et `gameservices.ShowAchievements` et `gameservices.ShowLeaderboard` ouvrent les surcouches natives de la plateforme. macOS, iOS et tvOS atteignent GameKit via purego ; Android atteindra Play Games Services par un point d’accroche d’exécution Java qui n’est pas encore dans le runtime, de sorte qu’il signale `CanUseGameServices() == false` pour l’instant ; Windows, Linux, le web, watchOS et les mini-programmes signalent `CanUseGameServices() == false` et répondent `gameservices.ErrUnsupported`.

Dans les tests, activez les services de jeu avec `nsgotest.Environment(core.Environment{CanUseGameServices: true})` et pilotez le simulacre isolé de chaque application avec `App.SetGameServicesPlayer`, `App.SetGameServicesFriends`, `App.SeedLeaderboardScores`, `App.SetGameServicesError`, `App.GameServicesAchievements`, `App.GameServicesScores`, `App.GameServicesShownAchievements` et `App.GameServicesShownLeaderboards`.

## Pour aller plus loin

- [Notifications](notifications.md) : autorisation, badges et appuis.
- [Formulaires](forms.md) : champs texte et clavier.
- [Intégration](embedding.md) : les services qu’une application hôte fournit à l’application qu’elle intègre.
- [Compilation et distribution](building.md) : les autorisations et les fichiers dont un paquet a besoin.

### Activité physique, régions et mouvement borné

`cx.WatchSteps`, `cx.WatchActivity` et `cx.WatchPressure` renvoient des flux liés à une portée. Demandez `core.PermissionActivityRecognition` avant les pas et l’activité, et avant la pression sur Apple. Apple fournit les trois via Core Motion ; Android fournit les compteurs de pas et la pression, et signale la classification de l’activité comme indisponible. Les comptes appartiennent à la fenêtre `Since` de l’échantillon ; les fenêtres reprises portent `Gap`. La pression est en pascals, avec une altitude relative facultative en mètres. Les distances, étages et altitudes manquants ont `Valid` à false.

`cx.WatchGeofences` prend des `core.GeofenceOptions` avec de un à vingt cercles `GeofenceRegion` distincts (coordonnées WGS 84 et rayon en mètres). Il signale `GeofenceInside`, `GeofenceOutside` ou `GeofenceUnknown` pour chaque région. `Background` exige l’accès et les déclarations de localisation en arrière-plan ainsi que l’autorisation Toujours d’Apple ; Android a aussi besoin d’un titre de notification. L’arrêt ou la libération supprime ses enregistrements ; aucun abonnement ne survit à la mort du processus.

`cx.RequestPreciseLocation("route", done)` demande explicitement l’autorisation de précision et répond par un booléen, jamais par une position. Dans `apple.info`, déclarez `NSLocationTemporaryUsageDescriptionDictionary: {route: Explains why this feature needs precision.}`. Android a besoin des autorisations approximative et précise. Une réponse false peut laisser la localisation approximative autorisée.

`cx.WatchBackgroundMotion(core.BackgroundMotionOptions{Motion: core.MotionOptions{}, Duration: 20 * time.Second, NotificationTitle: "Recording motion"}, sample, failed)` demande un flux en direct de durée finie. Il a besoin d’`AccessMotion` et d’`AccessBackgroundMotion` ; les paquets TSX déclarent `motion` et `backgroundMotion`. iOS peut faire expirer plus tôt son bail d’exécution. Les applications Android définissent `android.background_motion: true` et déclarent `FOREGROUND_SERVICE` ; le service non persistant s’arrête dans les trois minutes même si la durée demandée est plus longue. L’expiration de la durée ou par le système d’exploitation signale `SensorTimedOut`. L’arrêt ou la libération met fin au fournisseur et au bail ; rien n’est enregistré après la terminaison. Les autres cibles signalent l’indisponibilité.

`Sensors` en TSX reproduit ces six méthodes et utilise des millisecondes. Les tests sans interface alimentent `FeedSteps`, `FeedActivity`, `FeedPressure` et `FeedGeofence`, et définissent `PreciseLocationAnswer` pour les demandes de précision. L’activité physique, le réveil par région et le comportement en arrière-plan suspendu nécessitent encore une validation sur appareil.

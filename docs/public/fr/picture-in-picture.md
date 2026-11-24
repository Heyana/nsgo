---
title: Image dans l’image (Picture in Picture)
description: Lire une vidéo en mode Image dans l’image (Picture in Picture) sur iOS, tvOS, macOS, Android et navigateur, ou afficher une arborescence personnalisée sur iOS.
group: Fonctionnalités plateforme
order: 45
---

Sur iOS, déclarez la lecture en arrière-plan avant d’utiliser le mode Image dans l’image (Picture in Picture, PiP) ; macOS ne nécessite aucun mode d’arrière-plan :

```yaml
apple:
  info:
    UIBackgroundModes: [audio]
```

Pour une vidéo, activez `PictureInPicture`, conservez une `core.Ref`, puis démarrez ou arrêtez la session depuis un contrôle de l’application. Suivez le rappel d’état pour activer ce contrôle lorsque `Possible` devient vrai :

```go
ref := &core.Ref{}
state := core.NewSignal(cx, core.PictureInPictureState{})
video := core.Video{
    Source: "asset:clip.mp4", Ref: ref,
    PictureInPicture: true, AutoPictureInPicture: true,
    OnPictureInPicture: state.Set,
}
start := ui.Button{
    Text: "Picture in Picture", OnPress: ref.StartPictureInPicture,
    Disabled: func() bool { return !state.Get().Possible },
}
```

`AutoPictureInPicture` active le passage automatique en PiP lorsqu’une application iOS passe en arrière-plan ; tvOS et macOS ignorent cette option. `Controls` affiche toujours les contrôles de lecture natifs ; le bouton PiP de l’application utilise la même session gérée pour une vidéo contrôlée. `OnPictureInPictureRestore` permet à l’application d’afficher sa page lecteur lorsque l’utilisateur revient. Gardez la vidéo montée tant que le mode PiP est actif : supprimer sa portée arrête la lecture et le mode PiP.

Sur Android, l’activité empaquetée déclare déjà la prise en charge du mode PiP. Une vidéo entre dans la fenêtre PiP de l’activité, tandis que le reste de l’interface de l’application est masqué, et conserve son lecteur et sa position à son retour. `StopPictureInPicture` agrandit cette même activité. Les contrôles lecture/pause de la fenêtre signalent `OnPlay` et `OnPause`, avec `VideoPauseControls` ; `Ref.Seek` utilise le même lecteur. Le passage automatique exige le réglage explicite ainsi qu’une vidéo préparée et en cours de lecture. Un démarrage peut tout de même échouer en raison de l’état actuel du système ; gérez `Error` même lorsque `Possible` valait vrai.

Un hôte Android intégré doit déclarer `android:supportsPictureInPicture="true"` et gérer les changements de configuration `orientation|screenSize|screenLayout|smallestScreenSize`. Transférez `onPictureInPictureModeChanged` et `onUserLeaveHint` de l’activité aux méthodes correspondantes de `NsgoView` ; sur Android 15+, transférez également `PictureInPictureUiState.isTransitioningToPip()` à `NsgoView.onPictureInPictureTransition`. Un hôte ou un appareil sans prise en charge signale `Supported: false`.

Sur le web, déclenchez le démarrage depuis une interaction utilisateur dans un navigateur proposant l’API standard PiP pour la vidéo. `Supported` reflète également la Permissions Policy de la page, et `Possible` exige que les métadonnées vidéo soient chargées. Les contrôles PiP propres au navigateur signalent le même état actif. Arrêter, remplacer ou effacer la source, ou supprimer la vidéo met fin à sa session ; un démarrage en attente ne peut pas rouvrir la source précédente. Le mode PiP web n’affiche que la vidéo, sans contenu NSGo personnalisé, sans passage automatique en arrière-plan ni `OnPictureInPictureRestore`. Les navigateurs non pris en charge signalent `Supported: false`.

Sur iOS, `mobile.PictureInPicture` accepte `Children` avec leur propre mise en page, de 320 × 180 points par défaut. Il n’occupe aucun espace dans l’application. Sa `Ref` dispose des mêmes méthodes de démarrage et d’arrêt, tandis que `OnState`, `OnRestore`, `OnPlay`, `OnPause` et `OnSkip` transmettent les actions du système. Liez `PausedFunc` et `PositionFunc` à l’état de lecture mis à jour par ces rappels ; `Duration: 0` décrit un contenu en direct. Une application suspendue conserve la dernière image rendue.

Placez un `core.Video` dans l’arborescence personnalisée pour composer ses images décodées avec des sous-titres dessinés par l’application ou d’autres superpositions. Démarrez la session à l’aide de la `Ref` externe. Partagez un signal de pause avec la vidéo, alimentez `PositionFunc` de la session personnalisée depuis `Video.OnProgress` et transmettez `OnSkip` à `Ref.Seek` de la vidéo. L’audio de la vidéo est lu par son propre lecteur.

L’arborescence personnalisée restitue aussi du texte, des vues, des images et du Canvas. WebView, les shaders Metal et les effets d’arrière-plan (*backdrop effects*) ne sont pas disponibles dans les images capturées. Les images animées et les transitions de calques natifs affichent l’état de leur modèle. Utilisez des valeurs animées explicites pour le mouvement. La validation App Store peut rejeter les usages sans lien avec la vidéo.

`Supported: false` distingue une plateforme indisponible d’une session prise en charge qui n’est pas prête ou dont `Error` explique un échec. La vidéo prend en charge iOS et tvOS lorsque AVKit indique que l’appareil supporte le mode PiP, ainsi que macOS, Android et les navigateurs web proposant l’API standard ; les arborescences personnalisées ne prennent en charge qu’iOS. Sur iOS et tvOS, la session audio partagée reste dans la catégorie de lecture tant que le mode PiP est activé, y compris en pause, en se mélangeant à l’audio des autres applications lorsque `AudioSession` de la vidéo vaut `AudioSessionMixWithOthers`.

Dans les tests headless, `nsgotest.PictureInPictureSupport(supported, possible)` définit ce que propose le système sous un `nsgotest.Environment` iOS, macOS ou Android ; `PiPState`, `PiPStarts` et `PiPStops` de la vue enregistrent la session ; `app.ReportPictureInPicture` signale un état tel que le système le ferait ; et `app.PictureInPictureControl` envoie les commandes `play`, `pause`, `skip` et `restore` de la fenêtre. La fenêtre PiP de la vidéo lit et met en pause le propre lecteur de la vidéo, ce que signalent `Video.OnPlay` et `Video.OnPause`, avec `VideoPauseControls` pour une pause ; `app.PlayVideo` et `app.PauseVideo` les signalent dans les tests.

Exécutez `go tool nsgo dev -target ios examples/pip` pour les deux approches, ou `-target macos` ou `-target android` pour le mode PiP vidéo. Les tests natifs Mac vérifient l’entrée, la sortie et les commandes répétées avec et sans contrôles natifs, ainsi que le remplacement d’une source contrôlée active. La validation sur iPhone, y compris le plein écran de vidéo contrôlée et l’entrée automatique, reste en attente ; l’implémentation dispose actuellement de tests natifs de délégué/d’image et d’une couverture applicative headless.

Pour la vidéo tvOS, exécutez `go tool nsgo dev -target tvos_simulator examples/pip` ; NSGo associe le lecteur à une session Now Playing native. Le simulateur tvOS 27 charge les vidéos avec et sans contrôles natifs, mais AVKit y signale que le mode PiP n’est pas pris en charge. L’entrée, l’arrêt et la restauration réels en PiP nécessitent encore une vérification sur une Apple TV physique.

Le test de régression de la cible web utilise un clip local enregistré et des appuis de confiance sur le bouton de l’application pour vérifier la véritable session PiP du navigateur à travers l’entrée, la sortie, le remplacement de source et la suppression dans Chromium. Les autres moteurs de navigateur nécessitent encore une validation.

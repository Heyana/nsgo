---
title: Autorisations
description: Vérifier et demander les autorisations du système, ouvrir leurs réglages et guider l’utilisateur vers Réglages Système sur macOS.
group: Fonctionnalités plateforme
order: 49
---

## Vérifier et demander

Le système réserve certaines capacités à l’accord explicite de l’utilisateur : le microphone, la caméra, les contacts, les calendriers, les rappels, les photos, le Bluetooth, la bibliothèque multimédia, la localisation et les notifications sur téléphones et Mac, les mouvements et la localisation en arrière-plan sur téléphones, ainsi que sur macOS l’Accessibilité, l’Enregistrement de l’écran, la Surveillance de l’entrée, l’Accès complet au disque, la Gestion des apps, les Outils de développement et l’Automatisation d’une autre application ([localisation et mouvements](services.md#location-and-motion), [notifications](notifications.md)). `core.Permissions()` les énumère sous forme de clés `core.Permission`. `cx.PermissionStatus` indique si l’application dispose d’une autorisation sans la demander, et `cx.RequestPermission` affiche l’invite du système lorsqu’elle existe et que la demande n’a pas encore été faite. Effectuez la demande lorsque l’utilisateur réalise une action qui nécessite l’autorisation. Face ID et Touch ID n’en font pas partie : `cx.BiometricAvailability` indique leur disponibilité et `cx.AuthenticateBiometric` les sollicite avec leur propre invite ([biométrie](services.md#biometrics)).

```go
cx.RequestPermission(core.PermissionCamera, func(s core.PermissionStatus) {
	if s.Allowed() {
		startScanner()
	}
})
```

Sur macOS, l’Automatisation est accordée par application cible : passez `core.PermissionAutomationOf("com.apple.finder")` pour vérifier ou demander l’autorisation pour une application cible en cours d’exécution via son identifiant de bundle, ou `core.PermissionAutomation` pour ouvrir le volet Automatisation dans Réglages Système. Une application cible qui n’est pas en cours d’exécution répond `PermissionUnknown`.

| Statut | `String()` | Signification |
| --- | --- | --- |
| `PermissionNotAsked` | `prompt` | Pas encore demandée : une requête affiche l’invite du système |
| `PermissionGranted` | `granted` | L’application dispose de l’autorisation |
| `PermissionLimited` | `limited` | Accès partiel, par exemple aux photos ou contacts sélectionnés |
| `PermissionDenied` | `denied` | Refusée ; seuls les réglages du système permettent de la modifier |
| `PermissionUnknown` | `unknown` | Contrôlée, mais le système ou la plateforme n’en communique pas l’état |
| `PermissionNotGated` | `not-gated` | Rien n’en restreint l’accès à l’application, comme sur Linux |
| `PermissionUnavailable` | `unavailable` | La plateforme ne propose pas cette autorisation |

`Allowed` vaut vrai pour les statuts accordé (*granted*), limité (*limited*) et non restreint (*not-gated*). Sur macOS, l’Accessibilité, l’Enregistrement de l’écran et la Surveillance de l’entrée répondent refusé tant que l’utilisateur ne les active pas, car la demande se contente d’ajouter l’application dans le volet correspondant, et l’autorisation d’Enregistrement de l’écran n’est signalée qu’au redémarrage suivant de l’application.

## Descriptions d’utilisation

Sur macOS et iOS, demander une autorisation exige que son motif figure dans le fichier `Info.plist` de l’application, alimenté par `apple.info` dans `nsgo.yaml` ; sans cela, la demande répond indisponible et journalise la clé, car le système mettrait fin à l’application. Un message portant la même clé dans les [catalogues](localization.md) de l’application, comme `NSCameraUsageDescription: Scans the codes you point it at.` dans `locales/en.yaml`, traduit le motif afin que le système formule la demande dans la langue de l’utilisateur ; une langue qui ne le traduit pas reprend le texte source.

```yaml
apple:
  info:
    NSMicrophoneUsageDescription: Records your voice notes.
    NSCameraUsageDescription: Scans the codes you point it at.
    NSContactsUsageDescription: Finds friends who already use the app.
    NSCalendarsFullAccessUsageDescription: Adds your bookings to your calendar.  # macOS 14 et iOS 17 ; NSCalendarsUsageDescription auparavant
    NSRemindersFullAccessUsageDescription: Turns tasks into reminders.            # NSRemindersUsageDescription auparavant
    NSPhotoLibraryUsageDescription: Imports photos from your library.
    NSBluetoothAlwaysUsageDescription: Finds your headphones.
    NSAppleMusicUsageDescription: Plays your library.                            # iOS
    NSAppleEventsUsageDescription: Reveals files in Finder.                      # Automatisation macOS
android:
  permissions:
    - RECORD_AUDIO
    - CAMERA
    - READ_CALENDAR
    - WRITE_CALENDAR
    - BLUETOOTH_CONNECT      # Android 12+ ; BLUETOOTH sur Android 9–11
    - READ_MEDIA_AUDIO       # Android 13+ ; READ_EXTERNAL_STORAGE sur Android 9–12
windows:
  device_capabilities: [microphone, webcam]  # ce que déclare le MSIX d’une compilation pour la boutique
```

Sur Android, chaque autorisation doit être répertoriée dans `android.permissions` de `nsgo.yaml` ; toute autorisation absente du manifeste répond `PermissionUnavailable`. Sous `nsgo dev`, macOS impute ce que fait l’application au terminal qui l’a lancée, de sorte que c’est le terminal qui reçoit la demande et figure dans la liste ; compilez l’application pour tester ses propres autorisations.

## Ouvrir les réglages

`cx.OpenPermissionSettings` ouvre la page qui permet d’accorder une autorisation et indique si une page a bien été ouverte : Confidentialité et sécurité sur le volet de l’autorisation (y compris Automatisation, Contacts, Photos et Service de localisation) ou Notifications sur macOS, la page de l’application dans Réglages (ou sa page Notifications à partir d’iOS 16) sur iOS, la page de l’application dans Paramètres sur Android, et la page du microphone, de la caméra ou des notifications sur Windows.

Pour `PermissionLocation` et `PermissionBackgroundLocation`, macOS ouvre le Service de localisation ; `PermissionMotion` ouvre Mouvements et forme physique, bien que macOS ne fournisse pas d’échantillons de mouvement. iOS et Android ouvrent la page de l’application pour ces trois autorisations. L’ouverture des réglages ne demande ni n’accorde aucune autorisation.

## Guider l’utilisateur

`desktop.GuidePermission` ouvre cette page et attend que l’utilisateur accorde l’autorisation. Sur macOS, un panneau suit la fenêtre de Réglages Système sans prendre le focus et, pour les volets dont la liste accepte un glisser-déposer d’application, met l’application à disposition pour que l’utilisateur puisse l’y déposer. Il se termine, et sa fonction reçoit le statut, dès que l’application obtient l’autorisation, que l’utilisateur ferme le panneau ou Réglages Système, que `stop` est appelé ou que le composant disparaît ; un nouveau guide met fin au précédent.

```go
stop := desktop.GuidePermission(cx, core.PermissionAccessibility, desktop.GuideOptions{}, func(s core.PermissionStatus) {
	trusted.Set(s == core.PermissionGranted)
})
```

L’application présentée est son bundle, ou à défaut son exécutable ; `GuideOptions.App` en désigne une autre et `Name` définit le nom affiché dans le panneau. Windows ouvre la page et attend la fermeture des Paramètres. Ailleurs, la fonction reçoit immédiatement le statut.

## Plateformes

| Plateforme | Ce qu’elle indique et demande |
| --- | --- |
| macOS | Toutes les autorisations (l’Automatisation par application cible en cours d’exécution via `PermissionAutomationOf`, ainsi que les notifications pour les applications empaquetées en bundle) ; l’Automatisation seule, la Gestion des apps, les Outils de développement et Multimédia et Apple Music sont inconnus ; le mouvement est indisponible |
| iOS | Le microphone, la caméra, les contacts, les calendriers, les rappels, les photos (avec `PermissionLimited`), le Bluetooth, la bibliothèque multimédia, la localisation, les mouvements et les notifications |
| Windows | Le microphone, la caméra et les notifications, pour lesquelles aucune invite n’est affichée ; le reste n’est pas restreint |
| Linux | Non restreint hors des bacs à sable ; les notifications reflètent la réponse éventuelle d’un serveur de notifications ; dans Flatpak et Snap, la caméra via le portail bureau (*desktop portal*) et le microphone via le portail et PipeWire |
| Web | Le microphone et la caméra (inconnus dans Firefox), les notifications, la localisation (mais pas la localisation en arrière-plan) et les mouvements sur les navigateurs mobiles |
| Android | Le microphone, la caméra, les calendriers, le Bluetooth et la bibliothèque multimédia, dès lors que `android.permissions` les déclare |
| Apple Watch, mini-programmes | Les notifications, la localisation et les mouvements via leurs services ; le reste réservé par le système est inconnu pour l’instant |

Les autorisations de localisation et de mouvement suivent les [capteurs de chaque plateforme](services.md#location-and-motion), et `PermissionNotifications` suit les [notifications](notifications.md). Le suivi d’activité physique utilise `PermissionActivityRecognition` (`ACTIVITY_RECOGNITION` sur Android, `NSMotionUsageDescription` sur Apple), y compris la pression barométrique Apple ; les capteurs de mouvement bruts sur Android restent distincts. Sur Safari mobile, demandez `PermissionMotion` depuis un gestionnaire d’appui.

## Tests

`nsgotest` répond à partir de son modèle : `app.Permissions` conserve chaque statut, initialement non demandé, et une requête portant sur une autorisation non demandée lui attribue la réponse définie dans `app.PermissionAnswers`, ou à défaut le statut accordé. `PermissionNotifications` reste synchronisé avec `app.NotificationPermission` et `app.NotificationAnswer`. `app.PermissionSettings` répertorie les réglages ouverts et `app.Guide` le guide affiché, auquel `app.GrantPermission` et `app.CloseGuide` mettent fin.

```go
app.PermissionAnswers[core.PermissionMicrophone] = core.PermissionDenied
app.Press("id:record")
app.Find("text:The microphone is off")
```

## Suite

- [Notifications](notifications.md) : leur autorisation propre, les badges et les appuis.
- [Compilation et distribution](building.md) : les droits (*entitlements*) et capacités déclarés par un paquet.

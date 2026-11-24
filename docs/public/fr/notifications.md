---
title: Notifications
description: Demander l’autorisation d’afficher des notifications, les afficher, les planifier et les fermer, les compter, afficher la progression d’une tâche sur l’icône de l’application et écouter les appuis.
group: Fonctionnalités plateforme
order: 45
---

## Autorisation

`cx.RequestNotificationPermission` demande à l’utilisateur d’autoriser l’application à afficher des notifications, avec l’invite du système la première fois, et répond par un `core.NotificationPermission` : `NotificationsGranted`, `NotificationsDenied` ou `NotificationsUnavailable` lorsque la plateforme ou la compilation n’en affiche aucune. Effectuez la demande lorsque l’utilisateur réalise une action qui nécessite des notifications : iOS et Android ne posent la question qu’une seule fois, et les navigateurs refusent une invite qui n’a pas été déclenchée par l’utilisateur. `cx.NotificationPermission` lit la réponse sans la demander.

```go
cx.RequestNotificationPermission(func(p core.NotificationPermission) {
	if p == core.NotificationsGranted {
		cx.ShowNotification(core.Notification{ID: "chat:42", Title: "Ada", Body: "Lunch?"}, nil)
	}
})
```

## Afficher une notification

`cx.ShowNotification` affiche une notification, y compris lorsque l’application est au premier plan, et remplace celle portant le même `ID` ; `Silent` désactive le son et la vibration du système. Elle indique à `done` si le système a accepté la notification (faux sans l’autorisation) et retourne `core.ErrNotification`, sans appeler `done`, lorsqu’il manque un identifiant ou un titre. `cx.CloseNotification` supprime la notification désignée par un `ID`.

```go
err := cx.ShowNotification(core.Notification{ID: "sync", Title: "Synced", Body: "12 notes"}, func(shown bool) {})
cx.CloseNotification("sync")
```

`cx.CloseNotifications` supprime toutes les notifications affichées par l’application et annule celles qu’elle a planifiées. Sur macOS, iOS, watchOS, Windows et Android, cela inclut celles que des exécutions précédentes ont laissées dans le centre de notifications, le centre d’actions ou le volet des notifications ; sur Linux et le web, cela ne concerne que celles affichées ou planifiées lors de cette exécution. Appelez-la lorsque l’application passe au premier plan et que sa fenêtre affiche déjà ce qu’elles annonçaient :

```go
cx.Effect(func() {
	if cx.Env().Lifecycle() == core.LifecycleActive {
		cx.CloseNotifications()
		cx.SetBadge(0)
	}
})
```

## Planifier une notification

Une notification dont `At` est postérieur à l’instant présent est planifiée : un nouvel appel à `cx.ShowNotification` avec le même `ID` la déplace, tandis que `cx.CloseNotification` l’annule avant son affichage. `Repeat` l’affiche chaque jour à l’heure de `At`, ou chaque semaine le même jour de la semaine à la même heure, à partir de la prochaine occurrence, dans le fuseau horaire de l’appareil. `cx.Env().CanScheduleNotifications()` indique si la plateforme permet la planification : les applications macOS, iOS et watchOS empaquetées en bundle, ainsi qu’Android et Windows, les planifient nativement que l’application soit lancée ou non (Windows enchaînant les répétitions quotidiennes et hebdomadaires pendant que l’application tourne et les réarmant au lancement), tandis que Linux et le web (là où `Notification` est disponible) les conservent tant que l’application ou la page reste ouverte. Ailleurs (tvOS, dont le centre n’affiche que des badges, et les mini-programmes), `done` répond faux.

```go
cx.ShowNotification(core.Notification{ID: "review", Title: "Cards to review", At: due}, nil)
cx.ShowNotification(core.Notification{ID: "habit", Title: "Practice",
	At: time.Date(2026, 1, 1, 9, 0, 0, 0, time.Local), Repeat: core.NotificationDaily}, nil)
```

Sous Android, une notification planifiée peut s’afficher quelques minutes après l’heure prévue lorsque le téléphone est en veille (*Doze*) ; une application qui exige une précision à la minute près, comme un réveil, inscrit `android.permission.SCHEDULE_EXACT_ALARM` dans `android.permissions` (que l’utilisateur accorde dans les réglages système à partir d’Android 12) ou `USE_EXACT_ALARM` (que Google Play réserve uniquement aux applications d’alarme et de calendrier). Les notifications d’une application Android forcée à s’arrêter attendent son prochain démarrage. Pour tester la remise après l’arrêt du processus, attendez que `ShowNotification` réponde vrai, puis tuez ses processus sans forcer l’arrêt du paquet.

## Appuis

`cx.OnNotificationPress` exécute sa fonction avec l’`ID` de la notification sur laquelle l’utilisateur a appuyé, tant que le composant qui l’a définie existe, et ramène l’application au premier plan. Un appui qui a lancé l’application atteint le premier gestionnaire enregistré. Conservez dans `ID` ce que l’appui doit ouvrir (`"chat:42"`), car un appui peut relancer l’application depuis zéro. Sous Windows, cela fonctionne pour une application placée dans le menu Démarrer par son installateur ; lancée autrement, une exécution n’entend les appuis que pendant qu’elle tourne.

```go
cx.OnNotificationPress(func(id string) { openChat(strings.TrimPrefix(id, "chat:")) })
```

Une application Android demandant l’autorisation à partir d’Android 13 inscrit `android.permission.POST_NOTIFICATIONS` dans `android.permissions` de `nsgo.yaml`.

## Badge

`cx.SetBadge` affiche un compteur sur l’icône de l’application : celle du Dock sur macOS, de l’écran d’accueil sur iOS (ce qui nécessite l’autorisation de notifications), du bouton de la barre des tâches sur Windows, des lanceurs Linux compatibles Unity et d’une application web installée. Une valeur inférieure ou égale à 0 l’efface. Une application macOS n’a besoin d’aucune autorisation pour le badge du Dock tant qu’elle ne vérifie ni ne demande l’autorisation de notifications ou n’affiche pas de notification ; à partir de là, pour cette exécution et les suivantes, le Dock n’affiche le badge que si l’utilisateur autorise les pastilles de l’application dans Réglages Système, ce que `cx.RequestNotificationPermission` demande en même temps que les alertes. Les lanceurs Linux sans interface de badge et Android n’ont pas de compteur à définir ; les lanceurs Android signalent simplement une application dont des notifications sont affichées.

```go
cx.SetBadge(len(unread.Get()))
```

## Progression de tâche

`cx.SetTaskProgress` indique l’avancement d’une tâche longue sur l’icône de l’application : une barre sur l’icône du Dock sous macOS, sur le bouton de la barre des tâches sous Windows et sur les lanceurs Linux compatibles Unity. Cela ne demande aucune autorisation et s’affiche à côté du badge. `TaskProgressNormal` affiche `Value`, une fraction comprise entre 0 et 1 ; `TaskProgressPaused` et `TaskProgressError` la colorent sous Windows et macOS ; `TaskProgressIndeterminate` s’anime sous Windows et prend la forme d’une barre fixe à moitié remplie sous macOS et Linux ; la valeur nulle de `TaskProgress` l’efface. Sous Windows, la barre apparaît sur le bouton de la fenêtre de la portée (*scope*), et une progression définie là où aucune fenêtre n’a de bouton propre, comme la portée de l’application elle-même ou un panneau de zone de notification, s’affiche sur chaque fenêtre de l’application qui n’en définit pas. iOS, Android et le web n’affichent rien.

```go
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressIndeterminate}) // aucune fraction pour l’instant
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressNormal, Value: .43})
cx.SetTaskProgress(core.TaskProgress{}) // terminé
```

## Suite

- [Autorisations](permissions.md) : les autres autorisations du système, comme le microphone et l’Accessibilité.
- [Retours haptiques](haptics.md) : le retour tactile émis par l’appareil sous le doigt de l’utilisateur.
- [Services plateforme](services.md) : alertes, presse-papiers, fichiers, stockage et réseau.
- [Activités en direct](live-activities.md) : ce que suit l’application, sur l’écran verrouillé et dans Dynamic Island.
- [Compilation et distribution](building.md) : les autorisations dont un paquet a besoin.

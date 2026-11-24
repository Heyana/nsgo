---
title: Activités en direct
description: Suivre un événement en temps réel sur l’écran verrouillé et dans la Dynamic Island, et sous forme de Live Updates sur Android.
group: Fonctionnalités plateforme
order: 48
---

## Activation

Une activité en direct suit un événement au fur et à mesure qu’il se déroule, comme une livraison ou un minuteur : sur l’écran verrouillé et dans la Dynamic Island des iPhone à partir d’iOS 16.1, et sous forme de Live Update sur Android 16, en haut du panneau de notifications et dans une pastille de la barre d’état ; les versions antérieures d’Android affichent une notification en cours. Activez-les dans `nsgo.yaml` :

```yaml
live_activities:
  enabled: true
  # frequent_updates: true   # iOS : plus de mises à jour que le quota du système, si l’utilisateur l’autorise
```

## Démarrer, mettre à jour et terminer

`widget.StartActivity` démarre une activité d’un type donné (*kind*), qui nomme ce qu’elle suit et s’écrit de la même manière que les types de widgets, en affichant un `widget.ActivityContent`. `Activity.Update` remplace ce qu’elle affiche, `Activity.Alert` fait de même tout en alertant l’utilisateur, et `Activity.End` y met fin, avec un contenu final lorsqu’il n’est pas nil, et quitte l’écran verrouillé selon les règles de `Dismissal`. `widget.Activities` répertorie les activités en cours de l’application, y compris celles d’exécutions précédentes de l’application, et `widget.ActivitiesEnabled` indique si l’utilisateur et la plateforme les autorisent.

```go
a, err := widget.StartActivity("delivery", widget.ActivityContent{
	Title: "Your order", Body: "On its way", Symbol: "box.truck.fill",
	Timer: arrival, ShowProgress: true, Progress: 0.4,
	Actions: []widget.ActivityAction{{ID: "pause", Title: "Pause", Symbol: "pause.fill"}},
})
if err != nil {
	return
}
a.Update(widget.ActivityContent{Title: "Your order", Body: "Arrived", Status: "Now"})
a.End(nil, widget.DismissAt(time.Now().Add(2*time.Hour)))
```

`StartActivity` signale `widget.ErrNoActivities` dans une application compilée sans prise en charge des activités en direct, `widget.ErrActivitiesUnsupported` ailleurs que sur iOS 16.1 et Android, et `widget.ErrActivitiesDisabled` lorsque l’utilisateur les a désactivées ; `Update` et `End` signalent `widget.ErrActivityEnded` une fois l’activité terminée. Le contenu d’une activité fait au maximum 4 Ko, selon la limite acceptée par ActivityKit et APNs.

## Ce qu’elle affiche

Les deux plateformes affichent le titre et le corps, le statut ou un minuteur décompté par le système, une barre de progression, jusqu’à trois actions et la couleur d’accentuation. iOS les dessine via l’extension de widget et accepte à leur place des vues de `widget` pour l’écran verrouillé et chaque zone de la Dynamic Island : `LockScreen`, `Leading`, `Trailing`, `Center` et `Bottom` en mode étendu, `CompactLeading` et `CompactTrailing` dans l’îlot compact, et `Minimal` lorsque plusieurs activités se le partagent. Android n’affiche que les champs, avec l’icône de l’application, car les Live Updates n’acceptent aucune vue de l’application.

```go
widget.ActivityContent{
	Title: "Order 412", Body: "On its way", Status: "8 min", Symbol: "box.truck.fill",
	Tint: widget.RGB(0x30d158), ShowProgress: true, Progress: 0.4,
	Trailing: widget.Text{Content: "8 min", Style: widget.Style{FontSize: 34, FontWeight: core.FontWeightBold}},
}
```

`URL` s’ouvre avec l’application lorsqu’on appuie sur l’activité, `Stale` indique à partir de quand iOS affiche le contenu comme périmé, et `Background` remplit l’arrière-plan de l’activité sur l’écran verrouillé.

## Appuis

`widget.OnAction` exécute le gestionnaire d’un appui sur une action ou sur un `widget.Button`, avec l’activité et l’identifiant de l’action (`Activity` vaut nil pour un [bouton de widget](widgets.md#buttons)). Définissez-le dans `main`, avant `nsgo.Run` : sur iOS, le système démarre l’application en arrière-plan lors d’un appui, sans ses vues, et retient l’activité jusqu’au retour du gestionnaire, afin que celui-ci puisse la mettre à jour à cet endroit ; sur Android, un appui parvient au processus Go en cours d’exécution via le récepteur d’exécution sans ouvrir son Activity. Les actions nécessitent que le moteur qui les a publiées reste actif ; une action périmée ne fait rien une fois ce moteur arrêté. La notification elle-même ouvre toujours l’application lorsqu’on appuie dessus. Le gestionnaire s’exécute sur une goroutine du système, et non sur celle des vues de l’application, qu’il atteint via la méthode `Post` d’un `core.Scope`.

```go
func main() {
	widget.OnAction(func(p widget.ActivityPress) {
		if p.Action == "pause" {
			p.Activity.Update(paused)
		}
	})
	nsgo.Run(Root)
}
```

Une vue de l’activité le permet également, sur iOS 17 et versions ultérieures : `widget.Button{Action: "pause"}` exécute le même gestionnaire sans ouvrir l’application, et affiche ses enfants seuls là où un appui ouvre simplement l’application.

## Mises à jour push

Une application en arrière-plan ne met à jour ses activités que tant qu’elle s’exécute ; ainsi, celle qui suit des événements se produisant ailleurs les démarre avec `widget.ActivityOptions{Push: true}`, envoie le jeton reçu par `widget.OnPushToken` à son serveur, et lui fait envoyer des mises à jour APNs `liveactivity` dont le `content-state` correspond au résultat de `widget.ActivityState` pour le contenu : aucune image ne passe dans un push. À partir d’iOS 17.2, l’application obtient également un jeton pour démarrer des activités par push, avec le type d’attributs `widget.ActivityAttributesType` et les attributs du type d’activité (*kind*) et de l’identifiant (*id*). Les notifications push nécessitent l’habilitation (*entitlement*) aux notifications push dans le profil de l’application.

```go
state, err := widget.ActivityState(content)  // le content-state du push
```

## Limites

Le système met fin à une activité au bout de huit heures. `widget.DismissDefault` laisse une activité terminée sur l’écran verrouillé jusqu’à quatre heures sur iOS et la retire immédiatement sur Android, `widget.DismissNow` la retire immédiatement et `widget.DismissAt` à un moment donné dans les quatre heures. Android demande lui-même `POST_NOTIFICATIONS` et `POST_PROMOTED_NOTIFICATIONS` : demandez d’abord l’autorisation à l’utilisateur avec `cx.RequestNotificationPermission` ([Notifications](notifications.md)). La démo Widgets de la galerie et `examples/widgets` en exécutent une.

## Suite

- [Widgets](widgets.md) : les chronologies de l’écran d’accueil et de l’écran verrouillé, dessinées par la même extension.
- [Notifications](notifications.md) : l’autorisation qu’Android demande en premier.
- [Compilation et distribution](building.md) : l’extension et les habilitations dont elle a besoin.

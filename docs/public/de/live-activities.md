---
title: Live-Aktivitäten
description: Etwas, während es geschieht, auf dem Sperrbildschirm und in der Dynamic Island verfolgen sowie als Live Updates unter Android.
group: Plattformfunktionen
order: 48
---

## Aktivieren

Eine Live-Aktivität verfolgt etwas, während es geschieht, etwa eine Lieferung oder einen Timer: auf dem Sperrbildschirm und in der Dynamic Island von iPhones ab iOS 16.1 sowie als Live Update unter Android 16, oben in der Benachrichtigungsleiste und in einem Chip der Statusleiste; ältere Android-Versionen zeigen eine laufende Benachrichtigung. Aktivieren Sie sie in `nsgo.yaml`:

```yaml
live_activities:
  enabled: true
  # frequent_updates: true   # iOS: more updates than the system's budget, as the user allows
```

## Starten, aktualisieren und beenden

`widget.StartActivity` startet eine Aktivität einer bestimmten Art, die benennt, was sie verfolgt, und so geschrieben wird wie die Arten von Widgets, und zeigt einen `widget.ActivityContent`. `Activity.Update` ersetzt, was sie zeigt, `Activity.Alert` tut dasselbe und benachrichtigt die Nutzerin oder den Nutzer, und `Activity.End` beendet sie – mit endgültigem Inhalt, sofern dieser nicht nil ist – und entfernt sie vom Sperrbildschirm, wie es `Dismissal` vorgibt. `widget.Activities` listet die laufenden Aktivitäten der App auf, einschließlich derer aus früheren Ausführungen der App, und `widget.ActivitiesEnabled` gibt an, ob die Nutzerin oder der Nutzer und die Plattform sie zulassen.

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

`StartActivity` meldet `widget.ErrNoActivities` in einer App, die ohne sie gebaut wurde, `widget.ErrActivitiesUnsupported` überall außer auf iOS 16.1 und Android sowie `widget.ErrActivitiesDisabled`, wenn die Nutzerin oder der Nutzer sie deaktiviert hat; `Update` und `End` melden `widget.ErrActivityEnded`, sobald sie beendet ist. Der Inhalt einer Aktivität umfasst höchstens 4 KB, wie es ActivityKit und APNs annehmen.

## Was sie zeigt

Beide Plattformen zeigen Titel und Text, den Status oder einen Timer, den das System weiterzählt, einen Fortschrittsbalken, bis zu drei Aktionen und die Akzentfarbe. iOS zeichnet sie mit der Widget-Erweiterung und nimmt an ihrer Stelle Views aus `widget` für den Sperrbildschirm und jeden Teil der Dynamic Island an: `LockScreen`, `Leading`, `Trailing`, `Center` und `Bottom` in der erweiterten Ansicht, `CompactLeading` und `CompactTrailing` in der kompakten Island und `Minimal`, wenn sich mehrere Aktivitäten die Island teilen. Android zeigt nur die Felder mit dem App-Symbol, da Live Updates keine Views der App annehmen.

```go
widget.ActivityContent{
	Title: "Order 412", Body: "On its way", Status: "8 min", Symbol: "box.truck.fill",
	Tint: widget.RGB(0x30d158), ShowProgress: true, Progress: 0.4,
	Trailing: widget.Text{Content: "8 min", Style: widget.Style{FontSize: 34, FontWeight: core.FontWeightBold}},
}
```

`URL` wird mit der App geöffnet, wenn auf die Aktivität getippt wird, `Stale` ist der Zeitpunkt, ab dem iOS den Inhalt als veraltet anzeigt, und `Background` füllt die Aktivität auf dem Sperrbildschirm.

## Antippen

`widget.OnAction` führt den Handler für das Antippen einer Aktion oder eines `widget.Button` aus, mit der Aktivität und der ID der Aktion (`Activity` ist nil bei einem [Widget-Button](widgets.md#buttons)). Setzen Sie ihn in `main`, vor `nsgo.Run`: Unter iOS startet das System die App für ein Antippen im Hintergrund, ohne ihre Views, und hält die Aktivität, bis der Handler zurückkehrt, sodass der Handler sie dort aktualisiert; unter Android erreicht ein Antippen den laufenden Go-Prozess über den Runtime-Receiver, ohne dessen Activity zu öffnen. Aktionen setzen voraus, dass die Engine, die sie gesendet hat, weiterläuft; eine veraltete Aktion bewirkt nichts mehr, nachdem diese Engine beendet ist. Die Benachrichtigung selbst öffnet beim Antippen weiterhin die App. Der Handler läuft auf einer Goroutine des Systems, nicht auf der der Views der App, die er über `Post` eines `core.Scope` erreicht.

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

Eine View der Aktivität kann dies ab iOS 17 ebenfalls: `widget.Button{Action: "pause"}` führt denselben Handler aus, ohne die App zu öffnen, und zeigt nur seine Kinder dort, wo ein Antippen lediglich die App öffnet.

## Push-Aktualisierungen

Eine App im Hintergrund aktualisiert ihre Aktivitäten nur, solange sie läuft. Eine App, die verfolgt, was anderswo geschieht, startet sie daher mit `widget.ActivityOptions{Push: true}`, sendet das Token, das `widget.OnPushToken` empfängt, an ihren Server und lässt diesen APNs-`liveactivity`-Aktualisierungen pushen, deren `content-state` die Antwort von `widget.ActivityState` für den Inhalt ist: Bilder gehen nicht in einen Push. Ab iOS 17.2 erhält die App auch ein Token, um Aktivitäten per Push zu starten, mit dem Attributtyp `widget.ActivityAttributesType` und Attributen aus Art und ID. Pushes benötigen das Push-Notifications-Entitlement im Profil der App.

```go
state, err := widget.ActivityState(content)  // the push's content-state
```

## Grenzen

Das System beendet eine Aktivität nach acht Stunden. `widget.DismissDefault` belässt eine beendete Aktivität unter iOS bis zu vier Stunden auf dem Sperrbildschirm und entfernt sie unter Android sofort, `widget.DismissNow` entfernt sie sofort und `widget.DismissAt` zu einem Zeitpunkt innerhalb von vier Stunden. Android fragt `POST_NOTIFICATIONS` und `POST_PROMOTED_NOTIFICATIONS` selbst an: Fragen Sie die Nutzerin oder den Nutzer zuerst mit `cx.RequestNotificationPermission` ([Benachrichtigungen](notifications.md)). Die Widgets-Demo der Gallery und `examples/widgets` führen eine aus.

## Weiter

- [Widgets](widgets.md): die Timelines des Home-Bildschirms und des Sperrbildschirms, gezeichnet von derselben Erweiterung.
- [Benachrichtigungen](notifications.md): die Berechtigung, die Android zuerst anfragt.
- [Bauen und Ausliefern](building.md): die Erweiterung und die Entitlements, die sie benötigt.

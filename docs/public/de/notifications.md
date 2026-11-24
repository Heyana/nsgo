---
title: Benachrichtigungen
description: "Um Erlaubnis für Benachrichtigungen bitten, sie anzeigen, planen und schließen, sie zählen, den Fortschritt einer Aufgabe auf dem App-Icon anzeigen und auf Drücke reagieren."
group: Plattformfunktionen
order: 45
---

## Berechtigung

`cx.RequestNotificationPermission` bittet den Nutzer, der App das Anzeigen von Benachrichtigungen zu erlauben, beim ersten Mal mit der Abfrage des Systems, und antwortet mit einer `core.NotificationPermission`: `NotificationsGranted`, `NotificationsDenied` oder `NotificationsUnavailable`, wenn die Plattform oder der Build keine anzeigt. Fragen Sie, während der Nutzer etwas tut, das Benachrichtigungen benötigt: iOS und Android fragen nur einmal, und Browser verweigern eine Abfrage, die der Nutzer nicht ausgelöst hat. `cx.NotificationPermission` liest die Antwort, ohne zu fragen.

```go
cx.RequestNotificationPermission(func(p core.NotificationPermission) {
	if p == core.NotificationsGranted {
		cx.ShowNotification(core.Notification{ID: "chat:42", Title: "Ada", Body: "Lunch?"}, nil)
	}
})
```

## Eine anzeigen

`cx.ShowNotification` zeigt eine Benachrichtigung an, auch während die App im Vordergrund ist, und ersetzt eine mit derselben `ID`; `Silent` lässt den Ton und die Vibration des Systems weg. Es antwortet `done` damit, ob das System die Benachrichtigung angenommen hat, ohne die Berechtigung mit false, und gibt für eine ohne ID oder Titel `core.ErrNotification` zurück, ohne `done` aufzurufen. `cx.CloseNotification` entfernt die durch eine ID benannte.

```go
err := cx.ShowNotification(core.Notification{ID: "sync", Title: "Synced", Body: "12 notes"}, func(shown bool) {})
cx.CloseNotification("sync")
```

`cx.CloseNotifications` entfernt jede Benachrichtigung, die die App angezeigt hat, und storniert die geplanten. Auf macOS, iOS, watchOS, Windows und Android schließt das diejenigen ein, die frühere Läufe in der Mitteilungszentrale, im Info-Center oder in der Benachrichtigungsleiste hinterlassen haben; auf Linux und im Web erreicht es nur die, die dieser Lauf angezeigt oder geplant hat. Rufen Sie es auf, wenn die App in den Vordergrund kommt und ihr Fenster zeigt, worüber sie informiert haben:

```go
cx.Effect(func() {
	if cx.Env().Lifecycle() == core.LifecycleActive {
		cx.CloseNotifications()
		cx.SetBadge(0)
	}
})
```

## Eine planen

Eine Benachrichtigung, deren `At` später als jetzt liegt, wird geplant; ein erneuter Aufruf von `cx.ShowNotification` mit ihrer `ID` verschiebt sie, während `cx.CloseNotification` sie storniert, bevor sie angezeigt wird. `Repeat` zeigt sie jeden Tag zur Uhrzeit von `At` an, oder zusätzlich jede Woche an dessen Wochentag, ab dem nächsten solchen Zeitpunkt, in der Zeitzone des Geräts. `cx.Env().CanScheduleNotifications()` gibt an, ob die Plattform sie plant: macOS-, iOS- und watchOS-Apps in einem Bundle, Android und Windows planen sie nativ, ob die App läuft oder nicht (wobei Windows tägliche und wöchentliche Wiederholungen verkettet, solange die App läuft, und sie beim Start neu einrichtet), während Linux und das Web (wo `Notification` verfügbar ist) sie behalten, solange die App oder Seite läuft. Anderswo (tvOS, dessen Mitteilungszentrale nur Badges anzeigt, und Mini-Programme) antwortet `done` mit false.

```go
cx.ShowNotification(core.Notification{ID: "review", Title: "Cards to review", At: due}, nil)
cx.ShowNotification(core.Notification{ID: "habit", Title: "Practice",
	At: time.Date(2026, 1, 1, 9, 0, 0, 0, time.Local), Repeat: core.NotificationDaily}, nil)
```

Android zeigt eine geplante Benachrichtigung möglicherweise einige Minuten nach ihrer Zeit an, während sich das Telefon im Doze-Modus befindet; eine App, die sie minutengenau braucht, etwa ein Wecker, führt `android.permission.SCHEDULE_EXACT_ALARM` in `android.permissions` auf, die der Nutzer ab Android 12 in den Systemeinstellungen gewährt, oder `USE_EXACT_ALARM`, das Google Play nur Wecker- und Kalender-Apps erlaubt. Die Benachrichtigungen einer zwangsweise beendeten Android-App warten, bis sie das nächste Mal startet. Um die Zustellung nach dem Ende des Prozesses zu testen, warten Sie, bis `ShowNotification` mit true antwortet, und beenden Sie dann ihre Prozesse, ohne das Paket zwangsweise zu stoppen.

## Drücke

`cx.OnNotificationPress` führt seine Funktion mit der `ID` einer Benachrichtigung aus, die der Nutzer gedrückt hat, solange die Komponente lebt, die sie gesetzt hat, und bringt die App in den Vordergrund. Ein Druck, der die App gestartet hat, erreicht den ersten registrierten Handler. Halten Sie in der `ID` fest, was der Druck öffnet (`"chat:42"`), da ein Druck die App neu starten kann. Unter Windows tut er das für eine App, die ihr Installer ins Startmenü eingetragen hat; ansonsten erfährt ein Lauf nur von Drücken, während er läuft.

```go
cx.OnNotificationPress(func(id string) { openChat(strings.TrimPrefix(id, "chat:")) })
```

Eine Android-App, die ab Android 13 fragt, führt `android.permission.POST_NOTIFICATIONS` in `android.permissions` von `nsgo.yaml` auf.

## Badge

`cx.SetBadge` setzt eine Zahl auf das Icon der App: auf das im Dock unter macOS, auf das auf dem Home-Bildschirm unter iOS, was die Benachrichtigungsberechtigung erfordert, auf die Taskleistenschaltfläche unter Windows, auf Unity-kompatible Linux-Launcher und auf das einer installierten Web-App. Eine Zahl von 0 oder weniger entfernt sie. Eine macOS-App braucht für ihr Dock-Badge keine Berechtigung, bis sie die Benachrichtigungsberechtigung prüft oder anfordert oder eine Benachrichtigung anzeigt; von da an, in diesem und späteren Läufen, zeigt das Dock das Badge nur, solange der Nutzer die Badges der App in den Systemeinstellungen erlaubt, worum `cx.RequestNotificationPermission` zusammen mit den Hinweisen bittet. Launcher ohne Linux-Badge-Schnittstelle und Android haben keine Zahl, die man setzen könnte; Android-Launcher markieren eine App, deren Benachrichtigungen angezeigt werden.

```go
cx.SetBadge(len(unread.Get()))
```

## Aufgabenfortschritt

`cx.SetTaskProgress` zeigt auf dem Icon der App an, wie weit eine lange Aufgabe fortgeschritten ist: als Balken über der Dock-Kachel unter macOS, auf der Taskleistenschaltfläche unter Windows und auf Unity-kompatiblen Linux-Launchern. Es braucht keine Berechtigung und wird neben dem Badge angezeigt. `TaskProgressNormal` zeigt `Value`, einen Bruchteil von 0 bis 1; `TaskProgressPaused` und `TaskProgressError` färben ihn unter Windows und macOS ein; `TaskProgressIndeterminate` bewegt sich unter Windows und ist unter macOS und Linux ein stillstehender, halb gefüllter Balken; der Nullwert von `TaskProgress` entfernt ihn. Unter Windows wird er auf der Schaltfläche des Fensters des Scopes angezeigt, und ein Fortschritt, der dort gesetzt wird, wo kein Fenster eine eigene Schaltfläche hat, etwa im eigenen Scope der App oder in einem Tray-Panel, wird auf jedem Fenster der App angezeigt, das keinen eigenen setzt. iOS, Android und das Web zeigen nichts an.

```go
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressIndeterminate}) // no fraction yet
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressNormal, Value: .43})
cx.SetTaskProgress(core.TaskProgress{}) // done
```

## Weiter

- [Berechtigungen](permissions.md): die anderen Berechtigungen des Systems, etwa das Mikrofon und die Bedienungshilfen.
- [Haptik](haptics.md): das Feedback, das das Gerät unter dem Finger des Nutzers abspielt.
- [Plattformdienste](services.md): Alerts, die Zwischenablage, Dateien, Speicher und Netzwerk.
- [Live-Aktivitäten](live-activities.md): was die App verfolgt, auf dem Sperrbildschirm und in der Dynamic Island.
- [Bauen und ausliefern](building.md): die Berechtigungen, die ein Paket braucht.

---
title: Notifiche
description: Chiedi il permesso di mostrare notifiche, mostrale, pianificale e chiudile, contale, mostra l'avanzamento di un'attività sull'icona dell'app e ascolta le pressioni.
group: Funzionalità di piattaforma
order: 45
---

## Permesso

`cx.RequestNotificationPermission` chiede all'utente di consentire all'app di mostrare notifiche, con la richiesta di sistema la prima volta, e risponde con un `core.NotificationPermission`: `NotificationsGranted`, `NotificationsDenied` oppure `NotificationsUnavailable` dove la piattaforma o la build non ne mostra. Chiedilo nel momento in cui l'utente fa qualcosa che richiede le notifiche: iOS e Android chiedono una sola volta e i browser rifiutano una richiesta che non sia stata causata dall'utente. `cx.NotificationPermission` legge la risposta senza chiedere.

```go
cx.RequestNotificationPermission(func(p core.NotificationPermission) {
	if p == core.NotificationsGranted {
		cx.ShowNotification(core.Notification{ID: "chat:42", Title: "Ada", Body: "Lunch?"}, nil)
	}
})
```

## Mostrarne una

`cx.ShowNotification` mostra una notifica, anche mentre l'app è in primo piano, e sostituisce quella con lo stesso `ID`; `Silent` omette il suono e la vibrazione di sistema. Risponde a `done` indicando se il sistema ha accettato la notifica (false senza il permesso) e restituisce `core.ErrNotification`, senza chiamare `done`, per una notifica priva di ID o di titolo. `cx.CloseNotification` rimuove quella indicata da un ID.

```go
err := cx.ShowNotification(core.Notification{ID: "sync", Title: "Synced", Body: "12 notes"}, func(shown bool) {})
cx.CloseNotification("sync")
```

`cx.CloseNotifications` rimuove tutte le notifiche mostrate dall'app e annulla quelle che ha pianificato. Su macOS, iOS, watchOS, Windows e Android questo include quelle lasciate da esecuzioni precedenti nel Centro Notifiche, nel Centro operativo o nella tendina; su Linux e sul web raggiunge solo quelle mostrate o pianificate da questa esecuzione. Chiamala quando l'app torna in primo piano e la sua finestra mostra ciò che le notifiche comunicavano:

```go
cx.Effect(func() {
	if cx.Env().Lifecycle() == core.LifecycleActive {
		cx.CloseNotifications()
		cx.SetBadge(0)
	}
})
```

## Pianificarne una

Una notifica il cui `At` è successivo al momento attuale viene pianificata; chiamare di nuovo `cx.ShowNotification` con il suo `ID` la sposta, mentre `cx.CloseNotification` la annulla prima che venga mostrata. `Repeat` la mostra ogni giorno all'ora del giorno di `At`, oppure ogni settimana anche nel suo giorno della settimana, a partire dalla prossima occorrenza, nel fuso orario del dispositivo. `cx.Env().CanScheduleNotifications()` indica se la piattaforma le pianifica: le app macOS, iOS e watchOS in un bundle, Android e Windows le pianificano in modo nativo che l'app sia in esecuzione o meno (con Windows che concatena le ripetizioni giornaliere e settimanali mentre l'app è in esecuzione e le riattiva all'avvio), mentre Linux e il web (dove `Notification` è disponibile) le conservano finché l'app o la pagina è in esecuzione. Altrove (tvOS, il cui centro mostra solo i badge, e i mini programmi) `done` risponde false.

```go
cx.ShowNotification(core.Notification{ID: "review", Title: "Cards to review", At: due}, nil)
cx.ShowNotification(core.Notification{ID: "habit", Title: "Practice",
	At: time.Date(2026, 1, 1, 9, 0, 0, 0, time.Local), Repeat: core.NotificationDaily}, nil)
```

Android può mostrare una notifica pianificata qualche minuto dopo il suo orario mentre il telefono è in sospensione (Doze); un'app che ne ha bisogno al minuto esatto, come una sveglia, elenca `android.permission.SCHEDULE_EXACT_ALARM` in `android.permissions`, che l'utente concede nelle impostazioni di sistema a partire da Android 12, oppure `USE_EXACT_ALARM`, che Google Play consente solo alle app di sveglie e calendari. Le notifiche di un'app Android chiusa forzatamente attendono il suo avvio successivo. Per verificare la consegna dopo la terminazione del processo, attendi che `ShowNotification` risponda true, quindi termina i suoi processi senza forzare l'arresto del pacchetto.

## Pressioni

`cx.OnNotificationPress` esegue la sua funzione con l'`ID` di una notifica premuta dall'utente, finché il componente che l'ha impostata è attivo, e porta l'app in primo piano. Una pressione che ha avviato l'app raggiunge il primo gestore registrato. Conserva nell'`ID` ciò che la pressione apre (`"chat:42"`), perché una pressione può avviare l'app da zero. Su Windows ciò avviene per un'app che il suo programma di installazione ha inserito nel menu Start; altrimenti un'esecuzione riceve le pressioni solo mentre è in esecuzione.

```go
cx.OnNotificationPress(func(id string) { openChat(strings.TrimPrefix(id, "chat:")) })
```

Un'app Android che chiede il permesso a partire da Android 13 elenca `android.permission.POST_NOTIFICATIONS` in `android.permissions` di `nsgo.yaml`.

## Badge

`cx.SetBadge` mette un conteggio sull'icona dell'app: quella del Dock su macOS, quella della schermata Home su iOS, che richiede il permesso per le notifiche, il pulsante della barra delle applicazioni su Windows, i launcher Linux compatibili con Unity e quella di un'app web installata. Un conteggio pari a 0 o inferiore lo cancella. Un'app macOS non ha bisogno di alcun permesso per il badge del Dock finché non controlla o chiede il permesso per le notifiche o mostra una notifica; da quel momento in poi, in quell'esecuzione e nelle successive, il Dock mostra il badge solo se l'utente consente i badge dell'app in Impostazioni di Sistema, che `cx.RequestNotificationPermission` chiede insieme agli avvisi. I launcher privi di un'interfaccia Linux per i badge e Android non hanno un conteggio da impostare; i launcher Android contrassegnano un'app le cui notifiche sono visibili.

```go
cx.SetBadge(len(unread.Get()))
```

## Avanzamento delle attività

`cx.SetTaskProgress` mostra a che punto è un'attività lunga sull'icona dell'app: una barra sopra il riquadro del Dock su macOS, sul pulsante della barra delle applicazioni su Windows e sui launcher Linux compatibili con Unity. Non richiede alcun permesso e compare accanto al badge. `TaskProgressNormal` mostra `Value`, una frazione da 0 a 1; `TaskProgressPaused` e `TaskProgressError` la colorano su Windows e macOS; `TaskProgressIndeterminate` è animata su Windows ed è una barra ferma, riempita a metà, su macOS e Linux; il valore zero di `TaskProgress` la cancella. Su Windows compare sul pulsante della finestra dell'ambito, e l'avanzamento impostato dove nessuna finestra ha un proprio pulsante, come l'ambito stesso dell'app o un pannello nell'area di notifica, compare su ogni finestra dell'app che non ne imposta uno. iOS, Android e il web non mostrano nulla.

```go
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressIndeterminate}) // no fraction yet
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressNormal, Value: .43})
cx.SetTaskProgress(core.TaskProgress{}) // done
```

## Prossimi passi

- [Permessi](permissions.md): gli altri permessi di sistema, come il microfono e l'Accessibilità.
- [Feedback aptico](haptics.md): il feedback che il dispositivo riproduce sotto il dito dell'utente.
- [Servizi di piattaforma](services.md): avvisi, appunti, file, archiviazione e rete.
- [Live Activities](live-activities.md): ciò che l'app segue, nella schermata di blocco e nella Dynamic Island.
- [Compilazione e distribuzione](building.md): i permessi di cui un pacchetto ha bisogno.

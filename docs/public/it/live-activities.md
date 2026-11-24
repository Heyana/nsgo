---
title: Live Activities
description: Segui qualcosa mentre accade sulla schermata di blocco e nella Dynamic Island, e come Live Update su Android.
group: Funzionalità di piattaforma
order: 48
---

## Attivarle

Una Live Activity segue qualcosa mentre accade, come una consegna o un timer: sulla schermata di blocco e nella Dynamic Island degli iPhone a partire da iOS 16.1, e come Live Update su Android 16, in cima all'area delle notifiche e in un chip della barra di stato; le versioni precedenti di Android mostrano una notifica continua. Attivale in `nsgo.yaml`:

```yaml
live_activities:
  enabled: true
  # frequent_updates: true   # iOS: more updates than the system's budget, as the user allows
```

## Avviare, aggiornare e terminare

`widget.StartActivity` ne avvia una di un certo tipo, che indica ciò che segue e si scrive come i tipi dei widget, mostrando un `widget.ActivityContent`. `Activity.Update` sostituisce ciò che mostra, `Activity.Alert` fa lo stesso e avvisa l'utente, e `Activity.End` la termina, con un contenuto finale se non è nil, e la toglie dalla schermata di blocco come indica `Dismissal`. `widget.Activities` elenca le attività in corso dell'app, incluse quelle di esecuzioni precedenti dell'app, e `widget.ActivitiesEnabled` indica se l'utente e la piattaforma le consentono.

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

`StartActivity` restituisce `widget.ErrNoActivities` in un'app compilata senza di esse, `widget.ErrActivitiesUnsupported` su piattaforme diverse da iOS 16.1 e Android, e `widget.ErrActivitiesDisabled` quando l'utente le ha disattivate; `Update` e `End` restituiscono `widget.ErrActivityEnded` una volta che l'attività è terminata. Il contenuto di un'attività è al massimo di 4 KB, come accettano ActivityKit e APNs.

## Cosa mostra

Entrambe le piattaforme mostrano il titolo e il corpo, lo stato o un timer che il sistema fa avanzare, una barra di avanzamento, fino a tre azioni e il colore d'accento. iOS li disegna con l'estensione dei widget e accetta al loro posto viste di `widget` per la schermata di blocco e per ciascuna parte della Dynamic Island: `LockScreen`, `Leading`, `Trailing`, `Center` e `Bottom` nella forma espansa, `CompactLeading` e `CompactTrailing` nell'isola compatta, e `Minimal` quando più attività la condividono. Android mostra soltanto i campi, con l'icona dell'app, poiché i Live Update non accettano viste dell'app.

```go
widget.ActivityContent{
	Title: "Order 412", Body: "On its way", Status: "8 min", Symbol: "box.truck.fill",
	Tint: widget.RGB(0x30d158), ShowProgress: true, Progress: 0.4,
	Trailing: widget.Text{Content: "8 min", Style: widget.Style{FontSize: 34, FontWeight: core.FontWeightBold}},
}
```

`URL` si apre con l'app quando si tocca l'attività, `Stale` indica quando iOS mostra il contenuto come non aggiornato, e `Background` riempie l'attività sulla schermata di blocco.

## Pressioni

`widget.OnAction` esegue il gestore della pressione di un'azione o di un `widget.Button`, con l'attività e l'ID dell'azione (`Activity` è nil per un [pulsante di widget](widgets.md#buttons)). Impostalo in `main`, prima di `nsgo.Run`: su iOS il sistema avvia l'app in background per una pressione, senza le sue viste, e trattiene l'attività finché il gestore non ritorna, quindi il gestore la aggiorna lì; su Android una pressione raggiunge il processo Go in esecuzione tramite il receiver del runtime senza aprirne l'Activity. Le azioni richiedono che il motore che le ha pubblicate resti in esecuzione; un'azione obsoleta non fa nulla dopo che quel motore è terminato. La notifica stessa apre comunque l'app quando viene toccata. Il gestore viene eseguito su una goroutine del sistema, non su quella delle viste dell'app, che raggiunge tramite il `Post` di un `core.Scope`.

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

Anche una vista dell'attività può farlo, su iOS 17 e versioni successive: `widget.Button{Action: "pause"}` esegue lo stesso gestore senza aprire l'app, e mostra solo i suoi figli dove una pressione si limita ad aprire l'app.

## Aggiornamenti push

Un'app in background aggiorna le proprie attività solo mentre è in esecuzione, quindi un'app che segue ciò che accade altrove le avvia con `widget.ActivityOptions{Push: true}`, invia al proprio server il token ricevuto da `widget.OnPushToken` e gli fa inviare aggiornamenti APNs `liveactivity` il cui `content-state` è la risposta di `widget.ActivityState` per il contenuto: in un push non vanno immagini. A partire da iOS 17.2 l'app riceve anche un token per avviare attività tramite push, con il tipo di attributi `widget.ActivityAttributesType` e gli attributi del tipo e dell'id. I push richiedono l'entitlement per le notifiche push nel profilo dell'app.

```go
state, err := widget.ActivityState(content)  // the push's content-state
```

## Limiti

Il sistema termina un'attività dopo otto ore. `widget.DismissDefault` lascia un'attività terminata sulla schermata di blocco per un massimo di quattro ore su iOS e la rimuove subito su Android, `widget.DismissNow` la rimuove subito e `widget.DismissAt` a un orario entro quattro ore. Android richiede da sé `POST_NOTIFICATIONS` e `POST_PROMOTED_NOTIFICATIONS`: chiedilo prima all'utente con `cx.RequestNotificationPermission` ([Notifiche](notifications.md)). La demo Widgets della galleria ed `examples/widgets` ne eseguono una.

## Prossimi passi

- [Widget](widgets.md): le timeline della schermata Home e della schermata di blocco, disegnate dalla stessa estensione.
- [Notifiche](notifications.md): il permesso che Android richiede per primo.
- [Compilazione e distribuzione](building.md): l'estensione e gli entitlement di cui ha bisogno.

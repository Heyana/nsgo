---
title: Permessi
description: Controlla e richiedi i permessi di sistema, apri le relative impostazioni e guida l'utente in Impostazioni di Sistema su macOS.
group: Funzionalità di piattaforma
order: 49
---

## Controllare e chiedere

Il sistema lascia all'utente la concessione di alcune delle cose che un'app può fare: il microfono, la fotocamera, i contatti, i calendari, i promemoria, le foto, il Bluetooth, la libreria multimediale, la posizione e le notifiche su telefoni e Mac, il movimento e la posizione in background sui telefoni e, su macOS, anche Accessibilità, Registrazione schermo, Monitoraggio input, Accesso completo al disco, Gestione app, Strumenti per sviluppatori e Automazione di un'altra app ([posizione e movimento](services.md#posizione-e-movimento), [notifiche](notifications.md)). `core.Permissions()` li elenca come chiavi `core.Permission`. `cx.PermissionStatus` indica se l'app ne dispone senza chiedere, e `cx.RequestPermission` chiede con la richiesta di sistema, dove ne esiste una e non è ancora stata mostrata. Chiedi nel momento in cui l'utente fa qualcosa che richiede il permesso. Face ID e Touch ID non sono tra questi: `cx.BiometricAvailability` ne indica la disponibilità e `cx.AuthenticateBiometric` chiede con una propria richiesta ([biometria](services.md#biometria)).

```go
cx.RequestPermission(core.PermissionCamera, func(s core.PermissionStatus) {
	if s.Allowed() {
		startScanner()
	}
})
```

Su macOS, l'Automazione viene concessa per singola app di destinazione: passa `core.PermissionAutomationOf("com.apple.finder")` per controllare o chiedere il permesso per un'app di destinazione in esecuzione tramite il suo bundle ID, oppure `core.PermissionAutomation` per aprire il pannello Automazione in Impostazioni di Sistema. Un'app di destinazione non in esecuzione risponde `PermissionUnknown`.

| Stato | `String()` | Significato |
| --- | --- | --- |
| `PermissionNotAsked` | `prompt` | Non ancora chiesto: una richiesta mostra il prompt di sistema |
| `PermissionGranted` | `granted` | L'app ne dispone |
| `PermissionLimited` | `limited` | Accesso parziale, come foto o contatti selezionati |
| `PermissionDenied` | `denied` | Rifiutato; solo le impostazioni di sistema possono cambiarlo |
| `PermissionUnknown` | `unknown` | Controllato, ma il sistema o la piattaforma non lo comunica |
| `PermissionNotGated` | `not-gated` | Nulla lo nega all'app, come su Linux |
| `PermissionUnavailable` | `unavailable` | La piattaforma non prevede nulla del genere |

`Allowed` è true per concesso, limitato e non controllato. macOS risponde negato per Accessibilità, Registrazione schermo e Monitoraggio input finché l'utente non li attiva, poiché chiedere si limita a elencare l'app nel relativo pannello, e comunica la concessione di Registrazione schermo solo dopo il riavvio dell'app.

## Descrizioni d'uso

Su macOS e iOS, per chiedere serve il motivo del permesso nell'Info.plist dell'app, contenuto in `apple.info` di `nsgo.yaml`; senza di esso la richiesta risponde non disponibile e registra nel log la chiave, perché il sistema terminerebbe l'app. Un messaggio dei [cataloghi](localization.md) dell'app con la stessa chiave, come `NSCameraUsageDescription: Scans the codes you point it at.` in `locales/en.yaml`, traduce il motivo, che il sistema mostra poi nella lingua dell'utente; una lingua che non lo traduce usa quello della lingua di origine.

```yaml
apple:
  info:
    NSMicrophoneUsageDescription: Records your voice notes.
    NSCameraUsageDescription: Scans the codes you point it at.
    NSContactsUsageDescription: Finds friends who already use the app.
    NSCalendarsFullAccessUsageDescription: Adds your bookings to your calendar.  # macOS 14 and iOS 17; NSCalendarsUsageDescription before
    NSRemindersFullAccessUsageDescription: Turns tasks into reminders.            # NSRemindersUsageDescription before
    NSPhotoLibraryUsageDescription: Imports photos from your library.
    NSBluetoothAlwaysUsageDescription: Finds your headphones.
    NSAppleMusicUsageDescription: Plays your library.                            # iOS
    NSAppleEventsUsageDescription: Reveals files in Finder.                      # macOS Automation
android:
  permissions:
    - RECORD_AUDIO
    - CAMERA
    - READ_CALENDAR
    - WRITE_CALENDAR
    - BLUETOOTH_CONNECT      # Android 12+; BLUETOOTH on Android 9–11
    - READ_MEDIA_AUDIO       # Android 13+; READ_EXTERNAL_STORAGE on Android 9–12
windows:
  device_capabilities: [microphone, webcam]  # what a store build's MSIX declares
```

Su Android, ogni permesso deve essere elencato in `android.permissions` di `nsgo.yaml`; uno omesso dal manifest risponde `PermissionUnavailable`. Con `nsgo dev`, macOS attribuisce ciò che fa l'app al terminale che l'ha avviata, quindi è il terminale l'app a cui viene chiesto e che viene elencata; compila l'app per testarne i permessi.

## Aprire le impostazioni

`cx.OpenPermissionSettings` apre la pagina che concede un permesso e indica se ne è stata aperta una: Privacy e sicurezza nel pannello del permesso (incluse Automazione, Contatti, Foto e Servizi di localizzazione) oppure Notifiche su macOS, la pagina dell'app in Impostazioni (o la sua pagina Notifiche a partire da iOS 16) su iOS, la pagina dell'app in Impostazioni su Android e la pagina del microfono, della fotocamera o delle notifiche su Windows.

Per `PermissionLocation` e `PermissionBackgroundLocation`, macOS apre Servizi di localizzazione; `PermissionMotion` apre Movimento e fitness, anche se macOS non fornisce campioni di movimento. iOS e Android aprono la pagina dell'app per tutti e tre. Aprire le impostazioni non richiede né concede alcun permesso.

## Guidare l'utente

`desktop.GuidePermission` apre quella pagina e attende che l'utente conceda il permesso. Su macOS un pannello segue la finestra di Impostazioni di Sistema senza prendere il focus e, per i pannelli il cui elenco accetta un'app trascinata al suo interno, mostra l'app che l'utente deve trascinare lì. Termina, e la sua funzione riceve lo stato, quando l'app ottiene il permesso, l'utente chiude il pannello o Impostazioni di Sistema, viene chiamato `stop` oppure il componente scompare; una nuova guida termina quella precedente.

```go
stop := desktop.GuidePermission(cx, core.PermissionAccessibility, desktop.GuideOptions{}, func(s core.PermissionStatus) {
	trusted.Set(s == core.PermissionGranted)
})
```

L'app mostrata è il suo bundle, altrimenti il suo eseguibile; `GuideOptions.App` ne indica un'altra e `Name` il nome con cui il pannello la chiama. Windows apre la pagina e attende la chiusura di Impostazioni. Altrove la funzione riceve subito lo stato.

## Piattaforme

| Piattaforma | Cosa comunica e chiede |
| --- | --- |
| macOS | Ogni permesso (Automazione per singola app di destinazione in esecuzione tramite `PermissionAutomationOf`, più le notifiche per le app in bundle); Automazione generica, Gestione app, Strumenti per sviluppatori e Contenuti multimediali e Apple Music sono sconosciuti; il movimento non è disponibile |
| iOS | Il microfono, la fotocamera, i contatti, i calendari, i promemoria, le foto (con `PermissionLimited`), il Bluetooth, la libreria multimediale, la posizione, il movimento e le notifiche |
| Windows | Il microfono, la fotocamera e le notifiche, per le quali non mostra mai una richiesta; il resto non è controllato |
| Linux | Non controllati fuori dalle sandbox; le notifiche indicano se risponde un server di notifiche; in Flatpak e Snap, la fotocamera tramite il portale desktop e il microfono tramite il portale e PipeWire |
| Web | Il microfono e la fotocamera (sconosciuti in Firefox), le notifiche, la posizione (non quella in background) e il movimento sui browser mobili |
| Android | Il microfono, la fotocamera, i calendari, il Bluetooth e la libreria multimediale, dove `android.permissions` li dichiara |
| Apple Watch, mini programmi | Notifiche, posizione e movimento tramite i rispettivi servizi; il resto controllato dal sistema è per ora sconosciuto |

I permessi di posizione e movimento seguono i [sensori di ciascuna piattaforma](services.md#posizione-e-movimento), e `PermissionNotifications` segue le [notifiche](notifications.md). Il fitness usa `PermissionActivityRecognition` (`ACTIVITY_RECOGNITION` su Android, `NSMotionUsageDescription` su Apple), inclusa la pressione su Apple; il movimento grezzo di Android resta separato. Su Safari mobile, chiedi `PermissionMotion` da un gestore di pressione.

## Test

`nsgotest` risponde in base al proprio modello: `app.Permissions` contiene ogni stato, inizialmente non chiesto, e una richiesta di un permesso non ancora chiesto lo imposta al valore di `app.PermissionAnswers` per quel permesso, altrimenti a concesso. `PermissionNotifications` resta sincronizzato con `app.NotificationPermission` e `app.NotificationAnswer`. `app.PermissionSettings` elenca le impostazioni aperte e `app.Guide` la guida visualizzata, che `app.GrantPermission` e `app.CloseGuide` terminano.

```go
app.PermissionAnswers[core.PermissionMicrophone] = core.PermissionDenied
app.Press("id:record")
app.Find("text:The microphone is off")
```

## Prossimi passi

- [Notifiche](notifications.md): il loro permesso, i badge e le pressioni.
- [Compilazione e distribuzione](building.md): gli entitlement e le capability che un pacchetto dichiara.

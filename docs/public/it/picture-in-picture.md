---
title: Picture in Picture
description: Riproduci video in Picture in Picture su iOS, tvOS, macOS, Android e nel browser, oppure mostra un albero personalizzato su iOS.
group: Funzionalità di piattaforma
order: 45
---

Su iOS, dichiara la riproduzione in background prima di usare Picture in Picture; macOS non richiede alcuna modalità in background:

```yaml
apple:
  info:
    UIBackgroundModes: [audio]
```

Per un video, abilita `PictureInPicture`, conserva un `core.Ref` e avvialo o fermalo da un controllo dell'app. Segui il callback di stato per abilitare quel controllo quando `Possible` diventa true:

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

`AutoPictureInPicture` attiva l'ingresso automatico quando un'app iOS passa in background; tvOS e macOS ignorano questa opzione. `Controls` mostra comunque i controlli di riproduzione nativi; il pulsante PiP dell'app usa la stessa sessione gestita per un video controllato. `OnPictureInPictureRestore` consente all'app di mostrare la pagina del player quando l'utente ritorna. Mantieni il video montato mentre il PiP è attivo: eliminare il suo ambito interrompe la riproduzione e il PiP.

Su Android, l'activity del pacchetto dichiara già il supporto al PiP. Un video entra nella finestra PiP dell'activity, con il resto dell'interfaccia dell'app nascosto, e mantiene il proprio player e la propria posizione al ritorno. `StopPictureInPicture` espande quella stessa activity. I controlli di riproduzione/pausa della finestra segnalano `OnPlay` e `OnPause`, con `VideoPauseControls`; `Ref.Seek` usa lo stesso player. L'ingresso automatico richiede l'impostazione esplicita e un video preparato e in riproduzione. Un avvio può comunque fallire a causa dello stato attuale del sistema; gestisci `Error` anche quando `Possible` era true.

Un host Android incorporato deve dichiarare `android:supportsPictureInPicture="true"` e gestire le modifiche di configurazione `orientation|screenSize|screenLayout|smallestScreenSize`. Inoltra `onPictureInPictureModeChanged` e `onUserLeaveHint` dell'activity agli stessi metodi di `NsgoView`; su Android 15+, inoltra anche `PictureInPictureUiState.isTransitioningToPip()` a `NsgoView.onPictureInPictureTransition`. Un host o un dispositivo senza supporto segnala `Supported: false`.

Sul web, avvia da una pressione dell'utente in un browser che offre l'API standard per il PiP dei video. `Supported` riflette anche la Permissions Policy della pagina, e `Possible` richiede che i metadati del video siano caricati. I controlli PiP del browser segnalano lo stesso stato attivo. Fermare, sostituire o cancellare la sorgente, oppure rimuovere il video, termina la sua sessione; un avvio in sospeso non può riaprire la sorgente precedente. Il PiP sul web mostra solo il video, senza contenuti NSGo personalizzati, ingresso automatico in background o `OnPictureInPictureRestore`. I browser non supportati segnalano `Supported: false`.

Su iOS, `mobile.PictureInPicture` accetta `Children` con un proprio layout, di 320 × 180 punti per impostazione predefinita. Non occupa spazio nell'app. Il suo `Ref` ha gli stessi metodi di avvio e arresto, mentre `OnState`, `OnRestore`, `OnPlay`, `OnPause` e `OnSkip` trasmettono le azioni di sistema. Associa `PausedFunc` e `PositionFunc` allo stato di riproduzione aggiornato da quei callback; `Duration: 0` descrive un contenuto in diretta. Un'app sospesa mantiene l'ultimo fotogramma renderizzato.

Inserisci un `core.Video` nell'albero personalizzato per comporre i suoi fotogrammi decodificati con sottotitoli disegnati dall'app o altri overlay. Avvia la sessione usando il `Ref` esterno. Condividi un segnale di pausa con il video, alimenta la `PositionFunc` della sessione personalizzata da `Video.OnProgress` e inoltra `OnSkip` al `Ref.Seek` del video. L'audio del video viene riprodotto tramite il suo player.

L'albero personalizzato renderizza anche testo, viste, immagini e Canvas. WebView, shader Metal ed effetti di sfondo non sono disponibili nei fotogrammi catturati. Le immagini animate e le transizioni dei layer nativi mostrano lo stato del loro modello. Usa valori animati espliciti per il movimento. La revisione dell'App Store può rifiutare usi non correlati al video.

`Supported: false` distingue una piattaforma non disponibile da una sessione supportata che non è pronta o il cui `Error` spiega un errore. Il video supporta iOS e tvOS quando AVKit segnala che il dispositivo supporta il PiP, macOS, Android e i browser web che offrono l'API standard; gli alberi personalizzati supportano solo iOS. Su iOS e tvOS, la sessione audio condivisa resta nella categoria di riproduzione mentre il PiP è abilitato, anche in pausa, mixandosi con l'audio delle altre app quando l'`AudioSession` del video è `AudioSessionMixWithOthers`.

Nei test headless, `nsgotest.PictureInPictureSupport(supported, possible)` imposta ciò che il sistema offre in un `nsgotest.Environment` iOS, macOS o Android, `PiPState`, `PiPStarts` e `PiPStops` della vista registrano la sessione, `app.ReportPictureInPicture` segnala uno stato come farebbe il sistema e `app.PictureInPictureControl` invia `play`, `pause`, `skip` e `restore` della finestra. La finestra PiP del video avvia e mette in pausa il player del video stesso, che `Video.OnPlay` e `Video.OnPause` segnalano, con `VideoPauseControls` per una pausa; nei test `app.PlayVideo` e `app.PauseVideo` li segnalano.

Esegui `go tool nsgo dev -target ios examples/pip` per entrambi i percorsi, oppure `-target macos` o `-target android` per il PiP dei video. I test nativi su Mac verificano l'ingresso, l'uscita e i comandi ripetuti con e senza controlli nativi, oltre alla sostituzione di una sorgente controllata attiva. L'accettazione su iPhone, inclusi lo schermo intero del video controllato e l'ingresso automatico, è ancora in sospeso; l'implementazione dispone attualmente di test nativi per delegate e fotogrammi e di copertura headless dell'app.

Per i video su tvOS, esegui `go tool nsgo dev -target tvos_simulator examples/pip`; NSGo associa il player a una sessione nativa In riproduzione. Il simulatore di tvOS 27 carica i video con e senza controlli nativi, ma lì AVKit segnala il PiP come non supportato. L'ingresso effettivo nel PiP, l'arresto e il ripristino richiedono ancora una verifica su una Apple TV fisica.

Il test di regressione del target web usa un clip locale registrato e pressioni attendibili sui pulsanti dell'app per verificare la sessione PiP effettiva del browser durante ingresso, uscita, sostituzione della sorgente ed eliminazione in Chromium. Gli altri motori di browser richiedono ancora un'accettazione.

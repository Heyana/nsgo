---
title: Feedback aptico
description: Riproduci un feedback aptico predefinito o una sequenza temporizzata di impulsi.
group: Funzionalità di piattaforma
order: 46
---

## Riprodurne uno

`mobile.PlayHaptic` riproduce un feedback sotto il dito dell'utente su iOS e Android e non fa nulla altrove, quindi abbinalo a ciò che l'utente vede. `HapticSelection` segnala il cambio di un valore, come il passo di un picker, di un segmento o di uno slider; da `HapticLight` a `HapticRigid` segnalano elementi che si scontrano, in base al loro peso e alla loro elasticità; `HapticSuccess`, `HapticWarning` e `HapticError` segnalano un'attività che termina con quell'esito.

```go
mobile.PlayHaptic(cx, mobile.HapticSelection)
```

## Riprodurre una sequenza

`cx.Env().CanPlayHapticPattern()` indica se il feedback aptico temporizzato è disponibile. Una sequenza contiene impulsi con un istante di inizio `At` relativo alla sequenza. Una `Duration` pari a zero produce un impulso transitorio, mentre una durata positiva prolunga l'impulso fino a 30 secondi. `Intensity` e `Sharpness` vanno da 0 a 1. Una nuova sequenza sostituisce la precedente. Passa una sequenza vuota per interrompere la riproduzione. La sequenza si interrompe quando l'app passa in background.

```go
mobile.PlayHapticPattern(cx, mobile.HapticPattern{
    {Intensity: 0.25, Sharpness: 0.4},
    {At: 100 * time.Millisecond, Intensity: 0.55, Sharpness: 0.6},
    {At: 200 * time.Millisecond, Intensity: 1, Sharpness: 0.8},
})
```

iOS riproduce gli eventi tramite Core Haptics sull'hardware supportato. Android segue la temporizzazione e usa l'intensità quando il vibratore supporta il controllo dell'ampiezza. I browser Android con `navigator.vibrate` seguono i tempi di attivazione e pausa dopo un'interazione dell'utente. Android e il web riproducono un impulso transitorio come un impulso di 20 ms, e la nitidezza è disponibile su iOS. Su una piattaforma non supportata, la chiamata non ha effetto.

Nei test, imposta `HapticPatterns: true` tramite `nsgotest.Environment` e leggi `app.HapticPatterns`. La cronologia include una voce nil quando l'app annulla la riproduzione.

## Quando

Riproducilo mentre il gesto avviene, non quando arriva la risposta: una selezione mentre un valore scorre, un impatto quando una vista si posa, un esito quando un'attività termina. Riproducine uno per un cambiamento causato dall'utente, e nessuno per ciò che l'app ha fatto da sola.

## Prossimi passi

- [Notifiche](notifications.md): le notifiche e i badge che portano la stessa notizia.
- [Servizi di piattaforma](services.md): il resto di ciò che offre il sistema.
- [Accessibilità](accessibility.md): l'annuncio che sente un utente che non percepisce nulla.

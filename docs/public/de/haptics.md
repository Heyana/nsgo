---
title: Haptik
description: Vordefiniertes haptisches Feedback oder eine zeitgesteuerte Abfolge von Impulsen abspielen.
group: Plattformfunktionen
order: 46
---

## Einzeln abspielen

`mobile.PlayHaptic` spielt unter iOS und Android Feedback unter dem Finger des Nutzers ab und tut anderswo nichts; kombinieren Sie es daher mit dem, was der Nutzer sieht. `HapticSelection` kennzeichnet eine Wertänderung, einen Schritt eines Pickers, eines Segments oder eines Schiebereglers; `HapticLight` bis `HapticRigid` kennzeichnen zusammenstoßende Elemente nach ihrem Gewicht und ihrer Nachgiebigkeit; und `HapticSuccess`, `HapticWarning` und `HapticError` kennzeichnen einen entsprechend endenden Vorgang.

```go
mobile.PlayHaptic(cx, mobile.HapticSelection)
```

## Ein Muster abspielen

`cx.Env().CanPlayHapticPattern()` meldet, ob zeitgesteuerte Haptik verfügbar ist. Ein Muster enthält Impulse mit einer Startzeit `At` relativ zum Muster. Eine `Duration` von null erzeugt einen kurzen Impuls (Transient), und eine positive Dauer hält den Impuls bis zu 30 Sekunden lang aufrecht. `Intensity` und `Sharpness` reichen von 0 bis 1. Ein neues Muster ersetzt das vorherige. Übergeben Sie ein leeres Muster, um die Wiedergabe zu stoppen. Das Muster stoppt, wenn die App in den Hintergrund wechselt.

```go
mobile.PlayHapticPattern(cx, mobile.HapticPattern{
    {Intensity: 0.25, Sharpness: 0.4},
    {At: 100 * time.Millisecond, Intensity: 0.55, Sharpness: 0.6},
    {At: 200 * time.Millisecond, Intensity: 1, Sharpness: 0.8},
})
```

iOS spielt die Ereignisse auf unterstützter Hardware über Core Haptics ab. Android folgt dem Timing und nutzt die Intensität, sofern der Vibrationsmotor Amplitudensteuerung unterstützt. Android-Browser mit `navigator.vibrate` folgen nach einer Nutzerinteraktion den Ein- und Aus-Zeiten. Android und das Web spielen einen Transienten als 20-ms-Impuls ab, und Schärfe (`Sharpness`) ist unter iOS verfügbar. Auf einer nicht unterstützten Plattform hat der Aufruf keine Wirkung.

Setzen Sie in Tests `HapticPatterns: true` über `nsgotest.Environment` und lesen Sie `app.HapticPatterns` aus. Der Verlauf enthält einen `nil`-Eintrag, wenn die App die Wiedergabe abbricht.

## Wann

Spielen Sie es ab, während die Geste stattfindet, nicht erst, wenn die Antwort eintrifft: eine Auswahl, während ein Wert vorbeizieht, ein Aufprall, wenn eine View landet, ein Ergebnis, wenn ein Vorgang endet. Spielen Sie ein Feedback für eine vom Nutzer ausgelöste Änderung ab und keines für etwas, das die App von sich aus getan hat.

## Weiter

- [Benachrichtigungen](notifications.md): die Benachrichtigungen und Badges, die dieselbe Nachricht überbringen.
- [Plattformdienste](services.md): der Rest dessen, was das System bietet.
- [Barrierefreiheit](accessibility.md): die Ansage, die ein Nutzer hört, der nichts spürt.

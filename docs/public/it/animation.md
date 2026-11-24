---
title: Animazioni
description: Transizioni, valori animati, animazioni di layout e preset di movimento eseguiti sul clock dei fotogrammi della piattaforma.
group: Concetti chiave
order: 58
---

## Valori animati

Un'animazione viene eseguita sul thread principale della piattaforma, scandita dal suo stesso clock dei fotogrammi, quindi non costa nulla al motore per ogni fotogramma. Un `AnimatedValue` è un numero che la piattaforma fa variare; `Style.Animated` associa a esso opacità, traslazione, scala, rotazione, sfondo, colore del bordo, ombra e colore del testo di una vista.

```go
fade := core.NewAnimatedValue(cx, 0)
cx.StartAnimation(core.Timing(fade, 1, core.TimingConfig{
	Duration: 300 * time.Millisecond,
	Easing:   core.EaseOut,
}), nil)

core.View{Style: core.Style{Animated: &core.AnimatedStyle{Opacity: fade}}}
```

`Timing`, `Spring` e `Decay` spostano un singolo valore, mentre `Sequence`, `Parallel`, `Stagger`, `Delay` e `Loop` li compongono. `cx.StartAnimation` restituisce un `AnimationRun` il cui `Stop` interrompe l'esecuzione nel punto in cui si trova e `Reset` ne ripristina i valori. Un'esecuzione parte dal valore corrente e una nuova animazione sullo stesso valore termina la precedente.

```go
scale := core.NewAnimatedValue(cx, 1)
ui.Spring(cx, scale, 1.1, core.SpringConfig{Stiffness: 220, Damping: 14})
```

I nodi derivati seguono un valore: `core.Interpolate` mappa numeri o colori con il proprio easing e la propria estrapolazione, mentre `core.Add`, `Sub`, `Mul`, `Div`, `Mod`, `DiffClamp` e `SnappingDiffClamp` calcolano altri valori a partire da esso. `ui.Lerp` è un'interpolazione breve.

```go
angle := core.Interpolate(scroll, core.Interpolation{
	Input:  []float64{0, 240},
	Output: []float64{0, 90},
	Easing: core.EaseOut,
})
```

`Scope.RequestAnimationFrame` esegue una funzione una volta al successivo fotogramma del display e `Scope.Animate` scandisce una durata su quel clock. Entrambi girano sulla goroutine del grafo, quindi possono impostare segnali. La funzione riceve il tempo del fotogramma — quello del display stesso su macOS, iOS, Android, web, Linux, Windows, HarmonyOS e watchOS (su HarmonyOS quando la macchina di build dispone dell'OpenHarmony SDK; un'app impacchettata senza di esso riceve il clock del motore) — così un ciclo che sposta gli elementi in base al tempo trascorso tra due fotogrammi segue il ritmo del display; un test `nsgotest` fornisce il tempo del clock di test.

Imposta la frequenza di callback di una finestra con `cx.SetPreferredFrameRate(core.FrameRateRange{Preferred: 30})`. Passa `Minimum` e `Maximum` per fornire al display link un intervallo, ad esempio `{Minimum: 80, Maximum: 120, Preferred: 120}` su uno schermo ProMotion. Chiamalo dallo scope della finestra stessa. La preferenza si applica a `RequestAnimationFrame` e `Animate` finché non cambia. Un intervallo nullo ripristina il ritmo normale del display. Il tempo passato al callback rimane il tempo effettivo del display. Il display e il sistema operativo scelgono la frequenza di aggiornamento disponibile e NSGo può saltare fotogrammi del display per cadenzare i callback.


`Scope.Tween` fa avanzare un numero su quel clock per ciò che un valore animato non può raggiungere, come una cifra che sale in un testo: chiama una funzione con ciascun numero da un estremo all'altro, come `Timing` sposterebbe un valore, e un'ultima volta con il valore finale. Un passo di `nsgotest` che lo avvia attende il valore finale, come per un'animazione, e con la riduzione del movimento attiva restituisce subito il valore finale. `Animated.tween` in TSX funziona allo stesso modo.

```go
freed := core.NewSignal(cx, 0.0)
cx.Tween(0, 18.7, core.TimingConfig{Duration: 600 * time.Millisecond, Easing: core.BezierEasing(core.EaseOut)}, freed.Set, nil)

core.Text{ContentFunc: func() string { return fmt.Sprintf("%.1f GB can be freed", freed.Get()) }}
```

## Transizioni

Una `Transition` anima lo stile o le classi di una vista quando cambiano, direttamente sulla piattaforma, senza lavoro per il motore a ogni fotogramma. `Properties` indica ciò che si muove; 0 significa ogni proprietà eccetto il riquadro.

```go
core.View{Style: core.Style{
	Background:  core.RGB(0xffffff),
	CornerRadius: 12,
	Transition:  core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionBackground | core.TransitionTransform},
	Transform:   core.Transform{Scale: 1},
	Hover:       &core.Style{Transform: core.Transform{Scale: 1.02}, Background: core.RGB(0xf5f5f5)},
}}
```

`TransitionLayout` sposta e ridimensiona una vista dalla posizione in cui appare quando il suo stile le assegna un nuovo riquadro. Un'animazione di layout anima invece un intero commit, come fa `LayoutAnimation` di React Native: `cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` anima gli spostamenti, gli inserimenti e le rimozioni del batch successivo, e i preset coprono easing, lineare e molla (`spring`). Una transizione `LayoutSpring` accetta `Response` (il periodo naturale non smorzato della molla) e `DampingRatio` (`< 1` sottosmorzata, `1` con smorzamento critico, `> 1` sovrasmorzata), viene eseguita fino a stabilizzarsi anziché per una durata fissa e conserva posizione e velocità verso un nuovo obiettivo se viene ricalibrata a metà corsa.

```go
cx.AnimateLayout(core.LayoutAnimation{
	Update: &core.LayoutTransition{
		Curve:        core.LayoutSpring,
		Response:     280 * time.Millisecond,
		DampingRatio: 0.85,
	},
})
open.Set(!open.Get())
```

`View.OnTransitionEnd` viene eseguito una volta che una modifica ha completato la propria transizione, indipendentemente dal fatto che qualcosa si sia mosso o meno. La riduzione del movimento disattiva le transizioni e le animazioni di layout, quindi rispetta l'impostazione della piattaforma mantenendo raggiungibile lo stato finale.

## Movimento

`core.Motion` muove insieme opacità, traslazione, scala e rotazione di una vista attraverso keyframe, a partire da una `MotionSpec` definita dall'app o da un `MotionPreset` pronto all'uso: ingressi, uscite, richiami d'attenzione e cicli. Associarne lo stile imposta la vista a riposo; `Play` esegue una specifica e `Exit` consente a una vista di uscire con il proprio movimento all'interno di un `core.Presence`.

```go
enter := core.SlideInUp.Spec()
motion := core.NewMotion(cx, &enter)
motion.Exit(core.FadeOut.Spec())

core.View{Style: core.Style{Animated: motion.Style()}, Children: content}
```

`core.Presence{When, Then, Else}` mantiene un ramo finché tutte le uscite registrate non sono terminate, così un pannello o un toast escono con la propria animazione qualunque cosa li rimuova. `core.PresenceOf` applica lo stesso comportamento a una chiave confrontabile e a una funzione di contenuto, mentre `MaxLeaving` limita il numero di vecchi rami mentre un valore cambia rapidamente. `MotionPresets()` elenca ogni preset e `Kind()` indica se entra, esce, attira l'attenzione o si ripete in ciclo.

## Keyframe

`KeyframeAnimation` ripete i keyframe su una piattaforma: opacità, scala, rotazione e traslazione verticale a ogni offset, con una durata, un ritardo, un easing e un numero di iterazioni. `KeyframesSpin`, `KeyframesPing`, `KeyframesPulse` e `KeyframesBounce` sono quelli di Tailwind.

```go
core.View{Style: core.Style{Animation: core.KeyframeAnimation{
	Keyframes: core.KeyframesPulse,
	Duration:  2 * time.Second,
	Iterations: 0, // forever
}}}
```

`ValueKeyframes` sposta un valore animato attraverso più tappe come fa Web Animations, con offset, un easing per segmento, un ritardo, iterazioni e una direzione. Anche i gesti possono guidare i valori: `View.PanValues`, `PinchValue` e `RotationValue` li spostano sul thread principale dal punto in cui si trovavano all'inizio del gesto.

`core.PanEvent` include `Position` nella vista e `PagePosition` nella pagina in ogni fase del pan, così gli strumenti di trascinamento non hanno bisogno di un evento di pressione separato per recuperare la posizione del dito.

## Prossimi passi

- [Stili e temi](styles.md): le proprietà che una transizione muove.
- [Elenchi e scorrimento](lists.md): valori che seguono una vista scorrevole.
- [Layout](layout.md): cosa sono i riquadri animati.

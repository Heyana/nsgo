---
title: Animationen
description: "Übergänge, animierte Werte, Layout-Animationen und Motion-Presets auf dem Frame-Takt der Plattform."
group: Kernkonzepte
order: 58
---

## Animierte Werte

Eine Animation läuft auf dem Hauptthread der Plattform und wird von deren eigenem Frame-Takt weitergeschaltet, sodass sie die Engine pro Frame nichts kostet. Ein `AnimatedValue` ist eine Zahl, die die Plattform bewegt; `Style.Animated` bindet Deckkraft, Verschiebung, Skalierung, Drehung, Hintergrund, Rahmenfarbe, Schatten und Textfarbe einer View daran.

```go
fade := core.NewAnimatedValue(cx, 0)
cx.StartAnimation(core.Timing(fade, 1, core.TimingConfig{
	Duration: 300 * time.Millisecond,
	Easing:   core.EaseOut,
}), nil)

core.View{Style: core.Style{Animated: &core.AnimatedStyle{Opacity: fade}}}
```

`Timing`, `Spring` und `Decay` bewegen einen einzelnen Wert, während `Sequence`, `Parallel`, `Stagger`, `Delay` und `Loop` sie kombinieren. `cx.StartAnimation` gibt einen `AnimationRun` zurück, dessen `Stop` einen Lauf an der aktuellen Stelle anhält und `Reset` die Werte zurücksetzt. Ein Lauf beginnt dort, wo sein Wert gerade steht, und eine neue Animation eines Werts beendet die vorherige.

```go
scale := core.NewAnimatedValue(cx, 1)
ui.Spring(cx, scale, 1.1, core.SpringConfig{Stiffness: 220, Damping: 14})
```

Abgeleitete Knoten folgen einem Wert: `core.Interpolate` bildet Zahlen oder Farben mit eigenem Easing und eigener Extrapolation ab, und `core.Add`, `Sub`, `Mul`, `Div`, `Mod`, `DiffClamp` sowie `SnappingDiffClamp` berechnen daraus weitere Werte. `ui.Lerp` ist eine Kurzform der Interpolation.

```go
angle := core.Interpolate(scroll, core.Interpolation{
	Input:  []float64{0, 240},
	Output: []float64{0, 90},
	Easing: core.EaseOut,
})
```

`Scope.RequestAnimationFrame` führt eine Funktion einmalig beim nächsten Display-Frame aus, und `Scope.Animate` schreitet eine Dauer auf diesem Takt ab. Beide laufen auf der Goroutine des Graphen und können daher Signals setzen. Die Funktion erhält die Frame-Zeit – die des Displays selbst auf macOS, iOS, Android, im Web, unter Linux, Windows, HarmonyOS und watchOS (auf HarmonyOS, wenn auf der Build-Maschine das OpenHarmony SDK installiert ist; eine ohne es paketierte App erhält den Takt der Engine), sodass eine Schleife, die Dinge anhand der Zeit zwischen zwei Frames bewegt, dem Tempo des Displays folgt; ein `nsgotest`-Test liefert die Zeit des Test-Takts.

Legen Sie die Callback-Rate eines Fensters mit `cx.SetPreferredFrameRate(core.FrameRateRange{Preferred: 30})` fest. Übergeben Sie `Minimum` und `Maximum`, um dem Display-Link einen Bereich vorzugeben, etwa `{Minimum: 80, Maximum: 120, Preferred: 120}` auf einem ProMotion-Bildschirm. Rufen Sie dies aus dem eigenen Scope des Fensters auf. Die Vorgabe gilt für `RequestAnimationFrame` und `Animate`, bis sie geändert wird. Ein Null-Bereich stellt das normale Tempo des Displays wieder her. Die Callback-Zeit bleibt weiterhin die tatsächliche Display-Zeit. Display und Betriebssystem wählen die verfügbare Bildwiederholrate, und NSGo kann Display-Frames überspringen, um Callbacks zu takten.


`Scope.Tween` schreitet eine Zahl auf diesem Takt für Werte ab, die ein animierter Wert nicht direkt erreichen kann, etwa eine hochzählende Zahl in einem Text: Es ruft eine Funktion mit jeder Zahl von einem Wert zum anderen auf – so wie `Timing` einen Wert bewegen würde – und ein letztes Mal mit dem Endwert. Ein `nsgotest`-Schritt, der es startet, wartet wie bei einer Animation auf den Endwert, und bei reduzierter Bewegung wird sofort der Endwert geliefert. `Animated.tween` in TSX verhält sich genauso.

```go
freed := core.NewSignal(cx, 0.0)
cx.Tween(0, 18.7, core.TimingConfig{Duration: 600 * time.Millisecond, Easing: core.BezierEasing(core.EaseOut)}, freed.Set, nil)

core.Text{ContentFunc: func() string { return fmt.Sprintf("%.1f GB can be freed", freed.Get()) }}
```

## Übergänge

Ein `Transition` animiert Style oder Klassen einer View, wenn sie sich ändern – direkt auf der Plattform und ohne Aufwand für die Engine pro Frame. `Properties` gibt an, was bewegt wird; 0 bedeutet jede Eigenschaft außer dem Frame.

```go
core.View{Style: core.Style{
	Background:  core.RGB(0xffffff),
	CornerRadius: 12,
	Transition:  core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionBackground | core.TransitionTransform},
	Transform:   core.Transform{Scale: 1},
	Hover:       &core.Style{Transform: core.Transform{Scale: 1.02}, Background: core.RGB(0xf5f5f5)},
}}
```

`TransitionLayout` bewegt und skaliert eine View von ihrer aktuellen Position aus, wenn ihr eigener Style ihr einen neuen Frame zuweist. Eine Layout-Animation animiert stattdessen einen gesamten Commit, ähnlich wie React Natives `LayoutAnimation`: `cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` animiert Verschiebungen, Einfügungen und Entfernungen des nächsten Batches, wobei die Presets Easing, Linear und Feder (`Spring`) abdecken. Ein `LayoutSpring`-Übergang nimmt `Response` (die ungedämpfte Eigenperiode der Feder) und `DampingRatio` (`< 1` unterdämpft, `1` kritisch gedämpft, `> 1` überdämpft), läuft bis zum Einschwingen statt über eine feste Dauer und übernimmt Position sowie Geschwindigkeit in ein neues Ziel, wenn während der Bewegung ein neues Ziel gesetzt wird.

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

`View.OnTransitionEnd` läuft, sobald eine Änderung ihren Übergang durchlaufen hat – unabhängig davon, ob sich etwas bewegt hat. Reduzierte Bewegung schaltet Übergänge und Layout-Animationen aus; berücksichtigen Sie diese Systemeinstellung daher, indem der Endzustand stets erreichbar bleibt.

## Motion

`core.Motion` bewegt Deckkraft, Verschiebung, Skalierung und Drehung einer View gemeinsam über Keyframes – entweder aus einem eigenen `MotionSpec` der App oder aus einem vorgefertigten `MotionPreset`: Eingänge (`entrances`), Ausgänge (`exits`), Aufmerksamkeits-Effekte (`attention seekers`) und Schleifen (`loops`). Das Binden seines Styles setzt die View in ihren Ruhezustand; `Play` startet eine Spezifikation, und `Exit` lässt eine View mit ihrer Motion innerhalb von `core.Presence` ausblenden.

```go
enter := core.SlideInUp.Spec()
motion := core.NewMotion(cx, &enter)
motion.Exit(core.FadeOut.Spec())

core.View{Style: core.Style{Animated: motion.Style()}, Children: content}
```

`core.Presence{When, Then, Else}` behält einen Zweig so lange im Baum, bis jeder darin registrierte Ausgang abgeschlossen ist, sodass ein Sheet oder Toast beim Entfernen stets mit seiner Motion verschwindet. `core.PresenceOf` wendet dasselbe Verhalten auf einen vergleichbaren Schlüssel und eine Inhaltsfunktion an, und `MaxLeaving` begrenzt alte Zweige, wenn sich ein Wert schnell ändert. `MotionPresets()` listet alle Presets auf, und `Kind()` gibt an, ob ein Preset einblendet, ausblendet, Aufmerksamkeit weckt oder in einer Schleife läuft.

## Keyframes

`KeyframeAnimation` wiederholt Keyframes auf der Plattform: Deckkraft, Skalierung, Drehung und vertikale Verschiebung an jedem Offset, mit Dauer, Verzögerung, Easing und Wiederholungen. `KeyframesSpin`, `KeyframesPing`, `KeyframesPulse` und `KeyframesBounce` entsprechen denen von Tailwind.

```go
core.View{Style: core.Style{Animation: core.KeyframeAnimation{
	Keyframes: core.KeyframesPulse,
	Duration:  2 * time.Second,
	Iterations: 0, // forever
}}}
```

`ValueKeyframes` bewegt einen animierten Wert wie bei Web Animations über Stützpunkte – mit Offsets, Easing pro Segment, Verzögerung, Wiederholungen und Richtung. Auch Gesten können Werte antreiben: `View.PanValues`, `PinchValue` und `RotationValue` bewegen sie auf dem Hauptthread ab dem Stand zu Beginn der Geste.

`core.PanEvent` enthält in jeder Pan-Phase `Position` innerhalb der View und `PagePosition` innerhalb der Seite, sodass Ziehwerkzeuge kein separates Press-Ereignis benötigen, um die Fingerposition zu ermitteln.

## Weiter

- [Styles und Themes](styles.md): Die Eigenschaften, die ein Übergang bewegt.
- [Listen und Scrollen](lists.md): Werte, die einer Scroll-View folgen.
- [Layout](layout.md): Was animierte Frames sind.

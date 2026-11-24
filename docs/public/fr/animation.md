---
title: Animation
description: Transitions, valeurs animées, animations de mise en page et préréglages de mouvement exécutés sur l'horloge d'affichage de la plateforme.
group: Concepts clés
order: 58
---

## Valeurs animées

Une animation s'exécute sur le fil principal de la plateforme, cadencée par sa propre horloge d'affichage, de sorte qu'elle ne coûte rien au moteur à chaque image. Une `AnimatedValue` est un nombre déplacé par la plateforme ; `Style.Animated` y lie l'opacité, la translation, l'échelle, la rotation, l'arrière-plan, la couleur de bordure, l'ombre et la couleur de texte d'une vue.

```go
fade := core.NewAnimatedValue(cx, 0)
cx.StartAnimation(core.Timing(fade, 1, core.TimingConfig{
	Duration: 300 * time.Millisecond,
	Easing:   core.EaseOut,
}), nil)

core.View{Style: core.Style{Animated: &core.AnimatedStyle{Opacity: fade}}}
```

`Timing`, `Spring` et `Decay` déplacent une valeur, et `Sequence`, `Parallel`, `Stagger`, `Delay` et `Loop` les composent. `cx.StartAnimation` renvoie un `AnimationRun` dont `Stop` arrête une exécution là où elle se trouve et `Reset` rétablit ses valeurs. Une exécution démarre depuis la position actuelle de sa valeur, et une nouvelle animation sur une valeur met fin à la précédente.

```go
scale := core.NewAnimatedValue(cx, 1)
ui.Spring(cx, scale, 1.1, core.SpringConfig{Stiffness: 220, Damping: 14})
```

Des nœuds dérivés suivent une valeur : `core.Interpolate` associe des nombres ou des couleurs avec sa propre courbe d'accélération et d'extrapolation, et `core.Add`, `Sub`, `Mul`, `Div`, `Mod`, `DiffClamp` et `SnappingDiffClamp` calculent d'autres valeurs à partir d'elle. `ui.Lerp` est une interpolation courte.

```go
angle := core.Interpolate(scroll, core.Interpolation{
	Input:  []float64{0, 240},
	Output: []float64{0, 90},
	Easing: core.EaseOut,
})
```

`Scope.RequestAnimationFrame` exécute une fonction une fois à la prochaine image d'affichage, et `Scope.Animate` fait avancer une durée sur cette horloge. Toutes deux s'exécutent sur la goroutine du graphe, ce qui leur permet de modifier des signaux. La fonction reçoit l'instant de l'image — celui de l'écran lui-même sur macOS, iOS, Android, le web, Linux, Windows, HarmonyOS et watchOS (sur HarmonyOS lorsque la machine de compilation dispose du SDK OpenHarmony ; une application empaquetée sans celui-ci reçoit l'horloge du moteur) — afin qu'une boucle qui déplace des éléments selon le temps écoulé entre deux images suive le rythme de l'écran ; un test `nsgotest` fournit celui de l'horloge de test.

Définissez la fréquence de rappel d'une fenêtre avec `cx.SetPreferredFrameRate(core.FrameRateRange{Preferred: 30})`. Passez `Minimum` et `Maximum` pour donner une plage au lien d'affichage, comme `{Minimum: 80, Maximum: 120, Preferred: 120}` sur un écran ProMotion. Appelez-le depuis la portée propre de la fenêtre. La préférence s'applique à `RequestAnimationFrame` et `Animate` jusqu'à ce qu'elle change. Une plage nulle rétablit le rythme normal de l'écran. L'instant transmis au rappel reste l'instant réel d'affichage. L'écran et le système d'exploitation choisissent le taux de rafraîchissement disponible, et NSGo peut sauter des images d'affichage pour cadencer les rappels.

`Scope.Tween` fait progresser un nombre sur cette horloge pour ce qu'une valeur animée ne peut pas atteindre, comme un compteur qui défile dans un texte : elle appelle une fonction avec chaque nombre d'un bout à l'autre, comme `Timing` déplacerait une valeur, et une dernière fois avec la valeur finale. Une étape `nsgotest` qui la démarre attend la dernière valeur, comme pour une animation, et lorsque la réduction des animations est activée, elle fournit immédiatement la valeur finale. `Animated.tween` en TSX fonctionne à l'identique.

```go
freed := core.NewSignal(cx, 0.0)
cx.Tween(0, 18.7, core.TimingConfig{Duration: 600 * time.Millisecond, Easing: core.BezierEasing(core.EaseOut)}, freed.Set, nil)

core.Text{ContentFunc: func() string { return fmt.Sprintf("%.1f GB can be freed", freed.Get()) }}
```

## Transitions

Une `Transition` anime le style ou les classes d'une vue lorsqu'ils changent, sur la plateforme, sans aucun travail du moteur par image. `Properties` indique ce qui bouge ; 0 signifie toutes les propriétés sauf le cadre (*frame*).

```go
core.View{Style: core.Style{
	Background:  core.RGB(0xffffff),
	CornerRadius: 12,
	Transition:  core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionBackground | core.TransitionTransform},
	Transform:   core.Transform{Scale: 1},
	Hover:       &core.Style{Transform: core.Transform{Scale: 1.02}, Background: core.RGB(0xf5f5f5)},
}}
```

`TransitionLayout` déplace et redimensionne une vue depuis sa position affichée lorsque son propre style lui donne un nouveau cadre. Une animation de mise en page anime plutôt tout un *commit*, comme le fait `LayoutAnimation` de React Native : `cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` anime les déplacements, insertions et suppressions du prochain lot, et les préréglages couvrent l'accélération/décélération, le linéaire et le ressort. Une transition `LayoutSpring` prend `Response` (la période naturelle non amortie du ressort) et `DampingRatio` (`< 1` sous-amorti, `1` amortissement critique, `> 1` sur-amorti), s'exécute jusqu'à stabilisation plutôt que pendant une durée fixe, et emporte sa position et sa vélocité vers une nouvelle cible en cas de reciblage en plein vol.

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

`View.OnTransitionEnd` s'exécute une fois qu'un changement a terminé sa transition, que quelque chose ait bougé ou non. La réduction des animations désactive les transitions et les animations de mise en page ; respectez donc le réglage de la plateforme en gardant l'état final atteignable.

## Mouvement

`core.Motion` anime ensemble l'opacité, la translation, l'échelle et la rotation d'une vue via des images clés (*keyframes*), à partir d'un `MotionSpec` propre à l'application ou d'un `MotionPreset` prêt à l'emploi : entrées, sorties, appels à l'attention et boucles. Lier son style place la vue au repos ; `Play` exécute une spécification, et `Exit` permet à une vue de disparaître avec son mouvement à l'intérieur d'un `core.Presence`.

```go
enter := core.SlideInUp.Spec()
motion := core.NewMotion(cx, &enter)
motion.Exit(core.FadeOut.Spec())

core.View{Style: core.Style{Animated: motion.Style()}, Children: content}
```

`core.Presence{When, Then, Else}` conserve une branche jusqu'à ce que toutes les sorties qu'elle a enregistrées soient terminées, afin qu'un panneau (*sheet*) ou une notification (*toast*) quitte l'écran avec son mouvement quel que soit ce qui le retire. `core.PresenceOf` applique le même comportement à une clé comparable et une fonction de contenu, et `MaxLeaving` borne les anciennes branches lorsqu'une valeur change rapidement. `MotionPresets()` liste tous les préréglages et `Kind()` indique s'ils servent à entrer, sortir, attirer l'attention ou boucler.

## Images clés

`KeyframeAnimation` répète des images clés sur une plateforme : opacité, échelle, rotation et translation verticale à chaque décalage, avec une durée, un délai, une courbe et un nombre d'itérations. `KeyframesSpin`, `KeyframesPing`, `KeyframesPulse` et `KeyframesBounce` reprennent ceux de Tailwind.

```go
core.View{Style: core.Style{Animation: core.KeyframeAnimation{
	Keyframes: core.KeyframesPulse,
	Duration:  2 * time.Second,
	Iterations: 0, // forever
}}}
```

`ValueKeyframes` déplace une valeur animée à travers des étapes comme le fait Web Animations, avec des décalages, une courbe par segment, un délai, des itérations et une direction. Les gestes peuvent également piloter des valeurs : `View.PanValues`, `PinchValue` et `RotationValue` les déplacent sur le fil principal à partir de l'endroit où elles se trouvaient au début du geste.

`core.PanEvent` inclut `Position` dans la vue et `PagePosition` dans la page à chaque phase du panoramique, afin que les outils de glissement n'aient pas besoin d'un événement d'appui séparé pour retrouver la position du doigt.

## Suite

- [Styles et thèmes](styles.md) : les propriétés qu'une transition anime.
- [Listes et défilement](lists.md) : les valeurs qui suivent une vue défilante.
- [Mise en page](layout.md) : ce que sont les cadres animés.

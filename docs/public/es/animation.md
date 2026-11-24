---
title: Animación
description: Transiciones, valores animados, animación de disposición y preajustes de movimiento que se ejecutan con el reloj de fotogramas de la plataforma.
group: Conceptos básicos
order: 58
---

## Valores animados

Una animación se ejecuta en el hilo principal de la plataforma, sincronizada con su propio reloj de fotogramas, por lo que no supone ningún coste por fotograma para el motor. Un `AnimatedValue` es un número que mueve la plataforma; `Style.Animated` vincula a él la opacidad, traslación, escala, rotación, fondo, color de borde, sombra y color de texto de una vista.

```go
fade := core.NewAnimatedValue(cx, 0)
cx.StartAnimation(core.Timing(fade, 1, core.TimingConfig{
	Duration: 300 * time.Millisecond,
	Easing:   core.EaseOut,
}), nil)

core.View{Style: core.Style{Animated: &core.AnimatedStyle{Opacity: fade}}}
```

`Timing`, `Spring` y `Decay` mueven un valor, y `Sequence`, `Parallel`, `Stagger`, `Delay` y `Loop` los componen. `cx.StartAnimation` devuelve un `AnimationRun` cuyo método `Stop` detiene la ejecución donde se encuentre y `Reset` restablece sus valores. Una ejecución comienza desde el valor actual, y una nueva animación sobre un valor detiene la anterior.

```go
scale := core.NewAnimatedValue(cx, 1)
ui.Spring(cx, scale, 1.1, core.SpringConfig{Stiffness: 220, Damping: 14})
```

Los nodos derivados siguen a un valor: `core.Interpolate` asigna números o colores con su propia curva de suavizado y extrapolación, y `core.Add`, `Sub`, `Mul`, `Div`, `Mod`, `DiffClamp` y `SnappingDiffClamp` calculan otros valores a partir de él. `ui.Lerp` es una interpolación breve.

```go
angle := core.Interpolate(scroll, core.Interpolation{
	Input:  []float64{0, 240},
	Output: []float64{0, 90},
	Easing: core.EaseOut,
})
```

`Scope.RequestAnimationFrame` ejecuta una función una vez en el siguiente fotograma de pantalla, y `Scope.Animate` avanza paso a paso durante una duración sobre ese reloj. Ambas se ejecutan en la goroutine del grafo, por lo que pueden modificar señales. La función recibe el instante del fotograma, el propio de la pantalla en macOS, iOS, Android, la web, Linux, Windows, HarmonyOS y watchOS (en HarmonyOS cuando la máquina de compilación cuenta con el SDK de OpenHarmony; una aplicación empaquetada sin él recibe el reloj del motor), de modo que un bucle que desplaza elementos según el tiempo transcurrido entre dos fotogramas sigue el ritmo de la pantalla; una prueba de `nsgotest` proporciona el tiempo del reloj de pruebas.

Establece la tasa de llamadas de retorno de una ventana con `cx.SetPreferredFrameRate(core.FrameRateRange{Preferred: 30})`. Pasa `Minimum` y `Maximum` para indicar un rango al enlace de pantalla, como `{Minimum: 80, Maximum: 120, Preferred: 120}` en una pantalla ProMotion. Llámalo desde el propio ámbito de la ventana. La preferencia se aplica a `RequestAnimationFrame` y `Animate` hasta que cambie. Un rango en cero restaura el ritmo normal de la pantalla. La marca de tiempo de la llamada de retorno sigue siendo la hora real de la pantalla. La pantalla y el sistema operativo eligen la frecuencia de actualización disponible, y NSGo puede omitir fotogramas de pantalla para espaciar las llamadas de retorno.


`Scope.Tween` hace avanzar un número sobre ese reloj para aquello que un valor animado no puede alcanzar, como una cifra que sube en un texto: llama a una función con cada número desde un valor hasta otro, igual que `Timing` movería un valor, y una vez más con el último. Un paso de `nsgotest` que lo inicia espera hasta el último valor, igual que con una animación, y con movimiento reducido entrega el último de inmediato. `Animated.tween` en TSX funciona igual.

```go
freed := core.NewSignal(cx, 0.0)
cx.Tween(0, 18.7, core.TimingConfig{Duration: 600 * time.Millisecond, Easing: core.BezierEasing(core.EaseOut)}, freed.Set, nil)

core.Text{ContentFunc: func() string { return fmt.Sprintf("%.1f GB can be freed", freed.Get()) }}
```

## Transiciones

Una `Transition` anima el estilo o las clases de una vista cuando cambian, en la plataforma, sin trabajo por fotograma para el motor. `Properties` indica qué se mueve; 0 significa todas las propiedades excepto el marco.

```go
core.View{Style: core.Style{
	Background:  core.RGB(0xffffff),
	CornerRadius: 12,
	Transition:  core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionBackground | core.TransitionTransform},
	Transform:   core.Transform{Scale: 1},
	Hover:       &core.Style{Transform: core.Transform{Scale: 1.02}, Background: core.RGB(0xf5f5f5)},
}}
```

`TransitionLayout` mueve y redimensiona una vista desde donde se muestra cuando su propio estilo le asigna un nuevo marco. En cambio, una animación de disposición anima toda una confirmación completa, como hace `LayoutAnimation` de React Native: `cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` anima los movimientos, inserciones y eliminaciones del siguiente lote, y los preajustes cubren curvas de suavizado, lineal y resorte. Una transición `LayoutSpring` recibe `Response` (el período natural sin amortiguación del resorte) y `DampingRatio` (`< 1` subamortiguado, `1` con amortiguación crítica, `> 1` sobreamortiguado), se ejecuta hasta estabilizarse en lugar de hacerlo durante una duración fija, y conserva su posición y velocidad hacia un nuevo objetivo si se redirige a mitad del trayecto.

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

`View.OnTransitionEnd` se ejecuta una vez que un cambio ha completado su transición, se haya movido algo o no. El movimiento reducido desactiva las transiciones y las animaciones de disposición, así que respeta la configuración de la plataforma manteniendo accesible el estado final.

## Movimiento

`core.Motion` mueve conjuntamente la opacidad, traslación, escala y rotación de una vista a través de fotogramas clave, a partir de un `MotionSpec` propio de la aplicación o de un `MotionPreset` predefinido: entradas, salidas, reclamos de atención y bucles. Vincular su estilo sitúa la vista en reposo; `Play` ejecuta una especificación, y `Exit` permite que una vista salga con su movimiento dentro de un `core.Presence`.

```go
enter := core.SlideInUp.Spec()
motion := core.NewMotion(cx, &enter)
motion.Exit(core.FadeOut.Spec())

core.View{Style: core.Style{Animated: motion.Style()}, Children: content}
```

`core.Presence{When, Then, Else}` conserva una rama hasta que finaliza cada salida que registró, de modo que una hoja o un aviso salgan con su movimiento independientemente de qué los elimine. `core.PresenceOf` aplica el mismo comportamiento a una clave comparable y una función de contenido, y `MaxLeaving` limita las ramas antiguas mientras un valor cambia rápidamente. `MotionPresets()` enumera todos los preajustes y `Kind()` indica si entra, sale, llama la atención o se repite en bucle.

## Fotogramas clave

`KeyframeAnimation` repite fotogramas clave en una plataforma: opacidad, escala, rotación y traslación vertical en cada desplazamiento, con duración, retardo, curva de suavizado e iteraciones. `KeyframesSpin`, `KeyframesPing`, `KeyframesPulse` y `KeyframesBounce` corresponden a los de Tailwind.

```go
core.View{Style: core.Style{Animation: core.KeyframeAnimation{
	Keyframes: core.KeyframesPulse,
	Duration:  2 * time.Second,
	Iterations: 0, // forever
}}}
```

`ValueKeyframes` mueve un valor animado a través de paradas como hace Web Animations, con desplazamientos, una curva de suavizado por segmento, retardo, iteraciones y dirección. Los gestos también pueden controlar valores: `View.PanValues`, `PinchValue` y `RotationValue` los mueven en el hilo principal desde donde estaban al comenzar el gesto.

`core.PanEvent` incluye `Position` en la vista y `PagePosition` en la página en cada fase del arrastre panorámico, por lo que las herramientas de arrastre no necesitan un evento de pulsación separado para recuperar la posición del dedo.

## Siguiente

- [Estilos y temas](styles.md): las propiedades que mueve una transición.
- [Listas y desplazamiento](lists.md): valores que siguen a una vista desplazable.
- [Disposición](layout.md): qué son los marcos animados.

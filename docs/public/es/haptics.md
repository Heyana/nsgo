---
title: Respuesta háptica
description: Reproduce respuesta háptica preestablecida o una secuencia temporizada de pulsos.
group: Funciones de plataforma
order: 46
---

## Reproducir una respuesta

`mobile.PlayHaptic` reproduce respuesta háptica bajo el dedo del usuario en iOS y Android, y no hace nada en otras plataformas, así que combínala con lo que el usuario ve. `HapticSelection` marca un cambio de valor, el paso de un selector, un segmento o un control deslizante; `HapticLight` a `HapticRigid` marcan elementos que colisionan, según su peso y elasticidad; y `HapticSuccess`, `HapticWarning` y `HapticError` marcan así el fin de una tarea.

```go
mobile.PlayHaptic(cx, mobile.HapticSelection)
```

## Reproducir un patrón

`cx.Env().CanPlayHapticPattern()` indica si la respuesta háptica temporizada está disponible. Un patrón contiene pulsos con un tiempo de inicio `At` relativo al patrón. Una `Duration` de cero crea un pulso transitorio, y una duración positiva sostiene el pulso hasta por 30 segundos. `Intensity` y `Sharpness` van de 0 a 1. Un nuevo patrón reemplaza al anterior. Pasa un patrón vacío para detener la reproducción. El patrón se detiene cuando la aplicación pasa a segundo plano.

```go
mobile.PlayHapticPattern(cx, mobile.HapticPattern{
    {Intensity: 0.25, Sharpness: 0.4},
    {At: 100 * time.Millisecond, Intensity: 0.55, Sharpness: 0.6},
    {At: 200 * time.Millisecond, Intensity: 1, Sharpness: 0.8},
})
```

iOS reproduce los eventos mediante Core Haptics en hardware compatible. Android sigue la temporización y usa la intensidad cuando el vibrador admite control de amplitud. Los navegadores en Android con `navigator.vibrate` siguen los tiempos de encendido y apagado después de una interacción del usuario. Android y la web reproducen un pulso transitorio como un pulso de 20 ms, y la nitidez (`Sharpness`) está disponible en iOS. En una plataforma no disponible, la llamada no tiene efecto.

En las pruebas, establece `HapticPatterns: true` mediante `nsgotest.Environment` y lee `app.HapticPatterns`. El historial incluye una entrada `nil` cuando la aplicación cancela la reproducción.

## Cuándo usarla

Reprodúcela en el momento en que ocurre el gesto, no cuando llega la respuesta: una selección al pasar por un valor, un impacto cuando una vista aterriza, un resultado cuando termina una tarea. Reproduce una para un cambio provocado por el usuario y ninguna para lo que la aplicación hizo por su cuenta.

## Siguiente

- [Notificaciones](notifications.md): las notificaciones e insignias que comunican las mismas novedades.
- [Servicios de plataforma](services.md): el resto de lo que ofrece el sistema.
- [Accesibilidad](accessibility.md): el anuncio que escucha un usuario que no percibe la vibración.

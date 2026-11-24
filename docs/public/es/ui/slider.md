---
title: Control deslizante
description: Un valor, o un rango, a lo largo de una pista con el teclado y el puntero.
group: Componentes
order: 37
---

`ui.Slider` selecciona un valor a lo largo de una pista, mediante arrastre, pasos y el teclado. Un valor corresponde a una sola perilla; `High` crea un rango de dos.

```go preview=slider/basic
volume := core.NewSignal(cx, 40.0)
ui.Slider{Value: volume.Get, OnChange: volume.Set, Min: 0, Max: 100, Step: 5, Label: "Volume"}
```

## Un rango

Establece `High` y `OnHighChange` para elegir dos valores. `Value` es el extremo inferior. `LowLabel` y `HighLabel` dan nombre a las perillas para la tecnología de asistencia.

```go preview=slider/range
price := core.NewSignal(cx, 20.0)
ceiling := core.NewSignal(cx, 80.0)
ui.Slider{
	Value: price.Get, OnChange: price.Set,
	High: ceiling.Get, OnHighChange: ceiling.Set,
	Min: 0, Max: 100, Step: 1,
	Label: "Price", LowLabel: "Minimum", HighLabel: "Maximum",
}
```

## Pasos y valores

`Step` ajusta el valor al paso; un paso de 0 admite cualquier valor. `ValueText` cambia lo que la tecnología de asistencia lee como valor, y `Disabled` lo desactiva.

```go preview=slider/format
ui.Slider{Value: speed.Get, OnChange: speed.Set, Min: 0.5, Max: 2, Step: 0.25, Label: "Speed",
	ValueText: func(v float64) string { return fmt.Sprintf("%g×", v) }}
```

Usa `MinFunc`, `MaxFunc` y `StepFunc` cuando el rango o el paso cambian, como cuando un vídeo informa su duración. Anulan sus campos estáticos y actualizan el control deslizante montado, conservando su foco y cualquier arrastre en curso. Un `MaxFunc` explícito que devuelve 0 permite un rango colapsado hasta que llega la duración; el valor de la aplicación se conserva y se muestra dentro de los límites actuales.

```go
ui.Slider{Value: position.Get, OnChange: position.Set, MaxFunc: duration.Get, Label: "Position"}
```

En TSX, `min={minimum()}`, `max={duration()}` y `step={step()}` siguen a sus señales en su lugar; `max={0}` colapsa el rango, tal como hace un `MaxFunc` que devuelve 0.

## Dirección

`Axis: base.Vertical` coloca el control deslizante en vertical, con su mínimo en la parte inferior; dale una altura en `Style`. `Inverted` coloca el mínimo en el extremo opuesto: la parte superior de un control deslizante vertical, el extremo final de uno horizontal. Las teclas de flecha a lo largo de la pista siguen la dirección. `ui.Progress` acepta los mismos `Axis` e `Inverted`.

```go
ui.Slider{Value: level.Get, OnChange: level.Set, Axis: base.Vertical, Label: "Level", Style: core.Style{Height: core.Pt(160)}}
```

## Comportamiento

Un arrastre sigue al puntero e informa a `OnChange` mientras se mueve. Las teclas de flecha mueven una perilla enfocada un paso, Inicio y Fin van a los extremos, y Re Pág y Av Pág avanzan más. `ui.Slider` marca los pasos debajo de la pista cuando `Step` es mayor que 0, hasta 60 marcas.

## Accesibilidad

El control deslizante informa `RoleAdjustable`, su `Label` y su valor; un rango tiene ambas perillas con sus propias etiquetas. `ValueText` se lee en lugar del número. El teclado y las acciones de incremento y decremento de un lector de pantalla lo modifican.

## Plataforma

La pista la dibuja la aplicación y tiene el mismo tamaño en todas partes. Las pulsaciones, el foco y el lector de pantalla corresponden a la plataforma; un arrastre táctil se comporta como un arrastre de puntero con el mismo paso.

## Relacionado

- [Formularios](../forms.md): controles deslizantes en un formulario.
- [Accesibilidad](../accessibility.md): valores y roles ajustables.
- Las páginas Slider, RangeSlider y Rating de la galería.
- [Componentes sin estilo](../authoring/base.md#slider): `base.Slider`, vertical y con tus propias marcas.

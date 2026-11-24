---
title: Interruptor
description: Un ajuste activado o desactivado, en tres tamaños.
group: Componentes
order: 35
---

`ui.Switch` es un ajuste que surte efecto de inmediato. No almacena ningún valor: la aplicación es su propietaria y lo actualiza en `OnChange`.

```go preview=switch/basic
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}
```

## Tamaños

`ui.Switch` recibe `Checked`, `OnChange`, `Label` y `Size` (`SizeDefault`, `SizeSmall` o `SizeLarge`). El botón deslizante se desliza y la pista cambia de color al ritmo del reloj de fotogramas de la plataforma.

```go preview=switch/small
ui.Switch{Checked: sync.Get, OnChange: sync.Set, Label: "Sync automatically", Size: ui.SizeSmall}
```

Un interruptor es para un ajuste que se aplica de inmediato. Para una elección dentro de un formulario, usa una [casilla de verificación](checkbox.md); para una de varias opciones, usa `ui.Radio` en un `ui.RadioGroup` o un [selector](select.md).

## Accesibilidad

El interruptor informa `RoleSwitch` y su estado activado o desactivado. `Label` es lo que lee la tecnología de asistencia, y el texto junto a un interruptor suele ser su etiqueta. La barra espaciadora o una pulsación lo alterna cuando tiene el foco del teclado.

## Plataforma

El control predeterminado lo dibuja la aplicación, por lo que se ve igual en todas partes. `Variant: ui.SwitchSystem` solicita el control de la plataforma: NSSwitch en macOS, UISwitch en iOS y el interruptor de 52 × 32 dp de Material 3 en Android. Otros destinos usan el aspecto predeterminado. Las pulsaciones, el foco y el lector de pantalla corresponden a la plataforma. El interruptor predeterminado de un teléfono tiene el mismo tamaño que el de un escritorio por diseño; `SizeSmall` es para filas densas, no para un dispositivo.

## Relacionado

- [Casilla de verificación](checkbox.md): una o más opciones.
- [Formularios](../forms.md): ajustes en un formulario.
- [Accesibilidad](../accessibility.md): etiquetas y foco.
- La página Switch de la galería.
- [Componentes sin estilo](../authoring/base.md#switch): `base.Switch`, para un interruptor con un aspecto propio.

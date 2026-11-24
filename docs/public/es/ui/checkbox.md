---
title: Casilla de verificación
description: Una casilla de verificación, marcada, desmarcada o indeterminada.
group: Componentes
order: 34
---

`ui.Checkbox` es una casilla de verificación con el aspecto de `ui` y una etiqueta a su lado. No guarda ningún valor: la aplicación indica si está marcada y lo actualiza en `OnChange`.

```go preview=checkbox/basic
done := core.NewSignal(cx, false)
ui.Checkbox{Checked: done.Get, OnChange: done.Set, Text: "Remember me"}
```

## Contenido

`ui.Checkbox` muestra `Text` o los `Children` de la aplicación junto a su casilla. `Label` sustituye a ambos para la tecnología de asistencia, y `Hint` indica para qué sirve.

## Estados

`Indeterminate` no está ni marcado ni desmarcado, útil para un «seleccionar todo» sobre una selección parcial; al pulsar, solicita quedar marcado. `Disabled` impide las pulsaciones y el foco.

```go preview=checkbox/indeterminate height=220
ui.Checkbox{
	Checked:       func() bool { return chosen.Get() == len(files) },
	Indeterminate: func() bool { return chosen.Get() > 0 && chosen.Get() < len(files) },
	OnChange:      chooseAll,
	Text:          "Select all",
}
```

## Modificadores de pulsación

Usa `OnChangeEvent` en lugar de `OnChange` cuando una lista necesite selección de rango con Shift al pulsar:

```go
ui.Checkbox{Checked: done.Get, Text: "Remember me", OnChangeEvent: func(next bool, press core.PressEvent) {
	if press.Modifiers.Shift {
		chooseRange(next)
		return
	}
	done.Set(next)
}}
```

Cuando está definida, esta devolución de llamada sustituye a `OnChange`; la aplicación sigue actualizando el valor. La activación por teclado en escritorio y en la web conserva los modificadores después de los atajos de la aplicación; en Windows, Alt con Espacio o Intro abre en su lugar el menú de la ventana. `onChangeEvent` de TSX recibe `{ checked, press }`.

## Accesibilidad

La casilla de verificación informa `RoleCheckbox` y su estado marcado o indeterminado. Su nombre es `Text`, `Label` o el texto de sus hijos. Espacio la alterna cuando tiene el foco del teclado, y un lector de pantalla anuncia el nuevo estado.

## Plataforma

La casilla la dibuja la aplicación, no un control de la plataforma, por lo que luce igual en todas partes. La pulsación, el anillo de foco y el lector de pantalla son los de la plataforma. Se prueba por rol y etiqueta en todos los destinos.

## Relacionado

- [Interruptor](switch.md): un ajuste que surte efecto de inmediato.
- [Formularios](../forms.md): opciones en un formulario.
- [Lista](list.md): casillas de verificación en filas.
- La página Checkbox de la galería.
- [Componentes sin estilo](../authoring/base.md#casilla-de-verificación): `base.Checkbox`, para una casilla con un aspecto propio.

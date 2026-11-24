---
title: Partes y estados
description: No almacenes estado que pertenezca a la aplicación, da estilo a cada parte e indica a la tecnología de asistencia qué es el componente.
group: Escribir componentes
order: 71
---

Un componente sin estilos decide cómo se comporta un control y deja su aspecto en manos de quien lo utiliza. Esta página construye `Toggle`, una píldora que está activada o desactivada, tal como `base` construye sus controles.

## Valores controlados

Un control no almacena ningún valor que pertenezca a la aplicación: esta lo pasa como una función y lo actualiza desde una llamada de retorno. El componente lo lee allí donde se muestra, de modo que solo esa parte se actualiza cuando cambia.

```go
// Toggle is a pill that is on or off. Its parts are the pill and a dot
// inside it, then its children, the label.
type Toggle struct {
	On       func() bool
	Disabled func() bool
	// OnChange receives the value a press asks for, the opposite of On's.
	OnChange func(on bool)

	Class        string
	Style        core.Style
	StyleFunc    func() core.Style
	DotClass     string
	DotStyle     core.Style
	DotStyleFunc func() core.Style
	Children     []core.Node
	Label        string
}
```

Una aplicación guarda el valor en una señal: `chips.Toggle{On: bold.Get, OnChange: bold.Set, Label: "Bold"}`.

## Partes

Cada parte que dibuja un componente admite `Class`, `Style` y `StyleFunc`, nombrados según la parte (`DotClass`, `DotStyle`, `DotStyleFunc`). `Style` es fijo, `StyleFunc` sigue las señales que lee y la clase se aplica debajo de ambos, de modo que la aplicación pueda usar lo que prefiera escribir.

Una parte toma el estado del control mediante `PartStates` en lugar de notificar el suyo propio, de forma que las clases `checked:` y los estilos `Checked` se apliquen a ella mientras la tecnología de asistencia ve un único control:

```go
func (t Toggle) Render(*core.Scope) core.Node {
	on := func() bool { return t.On != nil && t.On() }
	off := func() bool { return t.Disabled != nil && t.Disabled() }
	shows := func() core.PartStates { return core.PartStates{Checked: on(), Disabled: off()} }
	dot := core.View{Class: t.DotClass, Style: t.DotStyle, StyleFunc: t.DotStyleFunc, PartStates: shows}
	return core.View{
		Class: t.Class, Style: t.Style, StyleFunc: t.StyleFunc,
		Role: core.RoleSwitch, Label: t.Label, Checked: on, Disabled: t.Disabled,
		OnPress: func(core.PressEvent) {
			if t.OnChange != nil {
				t.OnChange(!on())
			}
		},
		Children: append([]core.Node{dot}, t.Children...),
	}
}
```

Una aplicación le da estilo solo con clases:

```go
chips.Toggle{
	On: bold.Get, OnChange: bold.Set, Label: "Bold",
	Class:    "flex-row items-center gap-2 rounded-full border border-zinc-300 px-3 py-1 checked:bg-blue-50",
	DotClass: "size-2 rounded-full bg-zinc-300 checked:bg-blue-500",
	Children: []core.Node{core.Text{Content: "Bold"}},
}
```

De forma análoga, `PartStates.Invalid` aplica clases `aria-invalid:` y `Style.AriaInvalid` a una parte decorativa, incluidos los estilos `group-aria-invalid:` dentro de ella, sin notificar otro control inválido. Coloca `Invalid` en el control real o en la entrada nativa para que la tecnología de asistencia reciba su estado de validación.

## Accesibilidad

La raíz de un control indica qué es: un `Role`, un `Label` cuando su texto no lo expresa, un `Hint` para lo que hace al activarlo y `Checked`, `Selected`, `Disabled`, `Invalid` o `Expanded` según corresponda. Sus partes permanecen en silencio. Un control que se mueve entre elementos con las flechas es una única parada de tabulación, como una lista de pestañas. [Accesibilidad](../accessibility.md) enumera los roles y las acciones.

## Qué reenviar

Un componente cuya raíz sea una vista propia recibe los campos `OnLayout`, `Tooltip` y `TestID` de esa vista y los reenvía, para que las aplicaciones puedan medirlo, explicarlo y probarlo como cualquier otra vista. Las llamadas de retorno son opcionales: un `OnChange` nulo crea un control que muestra un valor sin modificarlo.

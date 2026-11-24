---
title: Cuadro de diálogo
description: Una superficie modal, una hoja y una alerta.
group: Componentes
order: 41
---

`ui.Dialog` es una superficie modal sobre la aplicación atenuada, y una hoja desde la parte inferior en pantallas táctiles. `ui.Sheet` es un panel desde la parte inferior, y `ui.AlertDialog` pide confirmación al usuario.

```go preview=dialog/basic height=360
open := core.NewSignal(cx, false)
ui.Dialog{
	Open:         open.Get,
	OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
	Title:        "Rename the file",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { open.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

## Apertura y cierre

La aplicación guarda si el cuadro de diálogo está abierto. `OnOpenChange` recibe cada cambio que solicita el cuadro de diálogo, con un `base.Reason` que distingue una pulsación en el exterior de Escape o retroceso; `OnClose` se ejecuta una vez cerrado. `NoCloseOnEscape` y `NoCloseOnBackdropPress` impiden que Escape y una pulsación en el exterior lo cierren.

`Title` y `Description` encabezan la superficie, `TitleFunc` y `DescriptionFunc` cuando cambian mientras está abierto, y `Footer` contiene sus botones. `Bare` deja toda la superficie a `Children`, para un cuadro de diálogo con el diseño propio de la aplicación, que los escritorios recortan dentro del borde de la superficie según las esquinas que le dan `Style` y `StyleFunc`, e `Inset` establece su distancia mínima respecto a los bordes de la ventana en escritorios.

En hojas táctiles, `Bare` ahora elimina el tirador y el relleno de contenido predeterminados para que `Children` llenen el panel. `NoGrabber` oculta únicamente el tirador, `ContentStyle` y `ContentStyleFunc` personalizan el contenedor del contenido, y `TitleStyle` y `TitleStyleFunc` personalizan el título. `Leading` y `Trailing` colocan nodos, como los botones Cancelar y Listo, en los extremos de la fila del título, con el título centrado entre ellos. `StartingStyle` y `EndingStyle` personalizan la transición de la hoja; incluye un `Transition` en ellos cuando necesites una temporización personalizada.

`Style` se fusiona sobre el aspecto de la superficie. `StyleFunc` se fusiona después y sigue las señales que lee, incluida la apariencia, mientras el cuadro de diálogo está abierto y al volver a abrirse. Por ejemplo, `StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} }` sigue el tema, donde `colors := ui.ColorsOf(cx)`. Ambos campos también funcionan en `ui.AlertDialog`. Un color cero deja el color existente en su lugar; usa un color transparente distinto de cero como `core.RGBA(0xFFFFFF, 0)` para limpiarlo.

## Hojas

`ui.Sheet` es un panel en la parte inferior sobre un fondo atenuado; una pulsación sobre el fondo, Escape o retroceso llama a `OnClose`.

Establece `Label` o `LabelFunc` para su nombre accesible de cuadro de diálogo, y `TestID` para localizar el panel en las pruebas. `LabelFunc` sigue sus señales sin reemplazar el contenido abierto. `CloseOnSwipe: true` también llama a `OnClose` después de que un deslizamiento hacia abajo supere el umbral de cierre; actualiza `Open` en esa devolución de llamada para cerrarlo.

Establece `Style` o `StyleFunc` para personalizar el fondo, los radios de las esquinas y el relleno de la superficie manteniendo el tirador y el manejo del área segura. `StyleFunc` sigue las señales que lee. Un `CornerRadius` sin `CornerRadii` redondea todas las esquinas en lugar de la parte superior redondeada predeterminada.

```go preview=dialog/sheet height=320
ui.Sheet{Open: filters.Get, OnClose: func() { filters.Set(false) }, Children: filterControls}
```

## Alertas

`ui.AlertDialog` pide al usuario que confirme o reconozca algo. Cada acción lo cierra, y una pulsación en el exterior no lo hace.

```go preview=dialog/alert height=320
ui.AlertDialog{
	Open:         confirm.Get,
	OnOpenChange: func(v bool, _ base.Reason) { confirm.Set(v) },
	Title:        "Delete the note?",
	Description:  "This cannot be undone.",
	Actions: []ui.AlertAction{
		{Text: "Cancel", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
		{Text: "Delete", Color: colors.Danger, Role: core.AlertDestructive, Default: true, OnPress: remove},
	},
}
```

Una acción con `Role: core.AlertCancel` se ejecuta con Escape o retroceso, seguida de `OnOpenChange(false, base.ReasonCancel)`. `Default: true` otorga foco de teclado a una acción cuando se abre la alerta en escritorios, la web y Apple TV, y hace que Intro la elija; al pulsar una acción se ejecuta `OnPress` y luego `OnOpenChange(false, base.ReasonSelect)`. Intro ejecuta la acción predeterminada mientras el foco del teclado no esté en otra acción o campo: si el foco se mueve a otra acción, Intro pulsa esa otra, y un cuadro de diálogo sobre la alerta conserva Intro. Una vez que una acción se ha ejecutado, la alerta se cierra, haga lo que haga `OnOpenChange`, y no ejecuta ninguna otra hasta que `Open` vuelva a ser falso y luego verdadero. Si varias acciones llevan el mismo marcador, gana la primera. Sin marcadores, Escape cierra sin ejecutar ninguna acción e Intro no tiene acción predeterminada. `Role: core.AlertDestructive` describe una acción; `Variant` o `Color` elige su aspecto.

Para la alerta o la hoja de acciones propia del sistema, usa `cx.ShowAlert` ([Servicios de plataforma](../services.md#alerts-and-the-clipboard)).

## Comportamiento

Un modal bloquea la aplicación detrás de él, toma el foco del teclado y se cierra con Escape y retroceso salvo que se indique lo contrario. En pantallas táctiles un cuadro de diálogo es una hoja que sigue un arrastre hacia abajo y se cierra al superar un umbral.

## Accesibilidad

`ui.Dialog` informa `RoleDialog`, o su `Role` explícito, y `ui.AlertDialog` informa `RoleAlertDialog` en escritorios y pantallas táctiles. Ambos leen su título, o `Label` cuando el título no basta, seguido de la descripción. El foco entra en el cuadro de diálogo cuando se abre y regresa cuando se cierra; el fondo atenuado no es una parada. Los botones del pie conservan sus roles y etiquetas.

## Plataforma

En Android y en pantallas táctiles un cuadro de diálogo es una hoja desde la parte inferior; en escritorios es una superficie centrada con un fondo atenuado. El contenido es el mismo; solo cambia la presentación.

## Relacionado

- [Navegación](../navigation.md): páginas, pestañas y modales juntos.
- [Menú](menu.md): comandos anclados y menús contextuales.
- [Accesibilidad](../accessibility.md): foco y roles de cuadro de diálogo.
- Las páginas Dialog, Sheet y AlertDialog de la galería.
- [Componentes sin estilo](../authoring/base.md#dialog): `base.Dialog` y `base.Sheet`, con las presentaciones de iOS y animación propia.

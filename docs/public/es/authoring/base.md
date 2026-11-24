---
title: Componentes sin estilos
description: Los controles propios de NSGo sin apariencia visual, para darles estilo parte por parte o construir sobre ellos una biblioteca con estilos.
group: Escribir componentes
order: 73
---

`base` contiene todos los controles que tiene `ui`, con su comportamiento, estado y accesibilidad, pero sin apariencia visual. Cada parte admite `Class`, `Style` y `StyleFunc` ([Partes y estados](parts.md)), de modo que una biblioteca o una aplicación con diseño propio los utiliza en lugar de `ui`, o los envuelve tal como lo hace `ui` ([Apariencia y temas](looks.md)). Esta página cubre los controles de la [referencia de componentes](../ui.md); la galería muestra cada uno junto a su forma en `ui`.

Controles como `Button`, `Checkbox`, `Select`, `Slider` y `NumberInput` aceptan `Invalid: rejected.Get` para seguir una señal de validación. Da estilo a ese estado con `Class: "aria-invalid:border-red-500"` o `Style.AriaInvalid`. Actualiza la accesibilidad y los estilos sin deshabilitar el control. Los campos compuestos lo pasan a su entrada real y a sus partes decorativas; `ColorPicker` lo comparte con sus controles.

Para valores de la aplicación arrastrados entre vistas, usa [`base/dragdrop`](../drag-drop.md): un área conecta orígenes y destinos mientras la aplicación proporciona su contenido, vista previa y apariencia.

## Botón

`base.Button` toma su apariencia de `Style` y su contenido de `Children`. `OnPress` se ejecuta al completarse una pulsación; `OnPressDown` y `OnPressUp` siguen el inicio y el fin de una pulsación para ofrecer retroalimentación propia.

```go
base.Button{
	Label:   "Save",
	OnPress: func(core.PressEvent) { save() },
	Style: core.Style{Background: core.RGB(0x007aff), Padding: core.Symmetric(8, 14), CornerRadius: 8, AlignSelf: core.AlignStart,
		Active: &core.Style{Opacity: core.Alpha(0.85)}},
	Children: []core.Node{core.Text{Content: "Save", Style: core.Style{Color: core.RGB(0xffffff)}}},
}
```

`Disabled`, `Busy`, `Selected` y `Expanded` informan del estado a la aplicación, al estilo y a la tecnología de asistencia: una acción aún en curso usa `Busy` (estilizada con `aria-busy:`), un conmutador o el elemento actual de una lista usa `Selected`, y un panel desplegable usa `Expanded`. `NoFocus` saca un botón de las paradas de tabulación cuando una fila o una lista se alcanza por otra vía.

## Casilla de verificación

`base.Checkbox` recibe un estilo para la caja, otro para el indicador y la marca propia en su interior, mostrada a partir del valor que posee la aplicación.

```go
base.Checkbox{
	Checked:        done.Get,
	OnChange:       done.Set,
	Label:          "Remember me",
	Style:          core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	IndicatorStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), BorderWidth: 1, CornerRadius: 5},
	Indicator:      core.Icon{SVG: lucide.Check(), Style: core.Style{FontSize: 14}},
	Children:       []core.Node{core.Text{Content: "Remember me"}},
}
```

El indicador toma su estado de la caja mediante `PartStates`, por lo que las clases `checked:` le dan estilo sin convertirlo en una segunda parada.

Para recibir modificadores, define `OnChangeEvent func(checked bool, press core.PressEvent)`. Reemplaza a `OnChange` y deja el valor en manos de la aplicación; `press.Modifiers.Shift` permite que una lista seleccione un rango. La activación con Espacio/Enter en escritorio, la web, Android e iOS/tvOS conserva los modificadores salvo que un atajo de la aplicación gestione la tecla primero; en Windows, Alt junto con Espacio o Enter abre en su lugar el menú de la ventana. En TSX, `onChangeEvent` recibe `{ checked, press }`.

## Interruptor

`base.Switch` consta de una pista, un botón deslizante dentro de ella y luego sus hijos como etiqueta. Una pulsación informa del valor solicitado, el opuesto a `Checked`.

```go
base.Switch{
	Checked:    dark.Get,
	OnChange:   dark.Set,
	Label:      "Dark appearance",
	Style:      core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	TrackStyle: core.Style{Width: core.Pt(38), Height: core.Pt(22), CornerRadius: 11},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

La pista y el botón deslizante admiten sus propias funciones `StyleFunc`, y sus estilos de estado siguen a `Checked`.

## Control deslizante

`base.Slider` recibe un `base.SliderValue`: `base.SingleValue(v)` o `base.RangeValue(low, high)`. Permite estilizar por separado la pista, el indicador, el rango y los controles deslizadores.

```go
level := core.NewSignal(cx, 3.0)
base.Slider{
	Value:      func() base.SliderValue { return base.SingleValue(level.Get()) },
	OnChange:   func(v base.SliderValue) { level.Set(v.End) },
	Min:        func() float64 { return 0 },
	Max:        func() float64 { return 10 },
	Step:       func() float64 { return 1 },
	Label:      "Level",
	Style:      core.Style{Height: core.Pt(28)},
	TrackStyle: core.Style{Height: core.Pt(4), CornerRadius: 2},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

`Min`, `Max` y `Step` son funciones de lectura reactivas: cambiar sus señales actualiza el control deslizante montado y su rango accesible, incluso durante un arrastre. `Axis` lo hace vertical e `Inverted` coloca su mínimo en el extremo opuesto, igual que en `base.Progress`; `Marks` coloca tus propias marcas sobre el indicador, y `OnChangeEnd` informa del valor una vez que termina el arrastre.

## Selector

`base.Select` recibe valores `base.SelectItem` y un `*string`, de modo que «ninguno» es `nil`. Tú das estilo al activador, a la ventana emergente, a sus filas y a sus grupos.

```go
choice := core.NewSignal[*string](cx, nil)
base.Select{
	Items: func() []base.SelectItem {
		return []base.SelectItem{{Value: "s", Title: "Small"}, {Value: "m", Title: "Medium"}}
	},
	Value:      choice.Get,
	OnChange:   choice.Set,
	Label:      "Size",
	Style:      core.Style{Height: core.Pt(28), Padding: core.Symmetric(4, 10), BorderWidth: 1, CornerRadius: 6},
	PopupStyle: core.Style{Padding: core.All(4), CornerRadius: 8},
	ItemStyle:  core.Style{Padding: core.Symmetric(4, 8), CornerRadius: 4},
}
```

Los elementos adyacentes con el mismo `Group` comparten un encabezado, la búsqueda al teclear coincide con los títulos de los elementos, `SearchField` dibuja un campo de búsqueda propio y `Cleanable` muestra un botón para limpiar. `OnCancel` escucha Escape, retroceso y una pulsación en el exterior.

## Pestañas

`base.Tabs` contiene elementos `base.Tab` y dibuja el indicador a partir de `IndicatorStyle`; la aplicación conserva qué pestaña está seleccionada. `base.Tab.OnPress` se ejecuta en cada pulsación, incluida la de la pestaña ya seleccionada.

```go
base.Tabs{Label: "Sections", IndicatorStyle: core.Style{Height: core.Pt(2), Background: core.RGB(0x007aff)}, Children: []core.Node{
	base.Tab{Label: "Overview", Selected: func() bool { return section.Get() == "overview" },
		OnPress: func(core.PressEvent) { section.Set("overview") }, Children: []core.Node{core.Text{Content: "Overview"}}},
	base.Tab{Label: "Activity", Selected: func() bool { return section.Get() == "activity" },
		OnPress: func(core.PressEvent) { section.Set("activity") }, Children: []core.Node{core.Text{Content: "Activity"}}},
}}
```

## Cuadro de diálogo

`base.Dialog` recibe sus estilos de fondo, capa y ventana emergente, una `Animation` o un `TransitionShader`, y tus hijos. `Open`, `DefaultOpen` y `OnOpenChange` lo controlan; `OnOpen`, `OnClose` y `OnCancel` informan de lo ocurrido. `OnConfirm` es la acción predeterminada: Enter la ejecuta, salvo que el foco del teclado esté en un botón o en un campo que consuma Enter o que otro modal cubra el diálogo; entonces el diálogo se cierra con `ReasonSelect`. `Role` y `AccessibilityDescription` definen lo que anuncian los lectores de pantalla, con `RoleAlertDialog` para una confirmación. `NoCloseOnSwipe` impide que el deslizamiento de iOS lo cierre, `Presentation` elige la presentación de iOS y `StatusBar` modifica la barra mientras está abierto.

`base.Sheet` es un panel en cualquier borde, con un fondo y tu contenido; con `CloseOnSwipe`, una hoja inferior sigue un arrastre hacia abajo y se cierra al superar un umbral. Acepta `OnConfirm`, `Role` y `AccessibilityDescription` igual que `base.Dialog`. `base.ShowAlertDialog` muestra la alerta o la hoja de acciones del sistema con botones por rol:

```go
base.ShowAlertDialog(cx, base.AlertDialog{
	Title: "Delete the note?",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(label string) {
	if label == "Delete" {
		remove()
	}
})
```

## Menú

`base.Menu` abre elementos `base.MenuItem` desde un activador propio, y `base.ContextMenu` lo hace en la posición del puntero. Un elemento notifica `OnSelect` y cierra el menú salvo que tenga `NoCloseOnSelect`; `Checked` lo convierte en un elemento marcable, `Indicator` dibuja tu marca antes de su contenido y `Role` elige lo que oye la tecnología de asistencia: un elemento de menú, una casilla de verificación o una opción de radio. Un `base.Submenu` contiene más elementos.

```go
base.Menu{Trigger: core.Text{Content: "Actions"}, Children: []core.Node{
	base.MenuItem{Label: "Show sidebar", Checked: sidebar.Get, OnSelect: func() { sidebar.Set(!sidebar.Get()) }},
	base.Submenu{Label: "Share", Children: []core.Node{
		base.MenuItem{Label: "Copy link", OnSelect: copyLink},
		base.MenuItem{Label: "Email", OnSelect: emailLink},
	}},
}}
```

`Placement`, `Align`, `Offset` y `Margin` colocan la ventana emergente en escritorio, y `PopupStyle` y `BackdropStyle` le dan estilo.

## Selector de emojis

`base.EmojiPicker` busca cualquier emoji Unicode por sus nombres CLDR localizados, palabras clave y códigos abreviados en los idiomas de la aplicación (o mediante `Languages` / `LanguagesFunc` explícitos), con pestañas de categorías, entradas opcionales de `Recent`, selección de tono de piel, navegación por teclado y una cuadrícula virtualizada. Todas las etiquetas y nombres de categoría predeterminados proceden de `base/locales`, y el conjunto de datos de emojis solo se enlaza en aplicaciones que usen `EmojiPicker` o sus funciones auxiliares (`base.SearchEmojis`, `base.EmojisInCategory`, `base.EmojiByString`).

```go
base.EmojiPicker{
	Recent:   recent.Get,
	OnSelect: func(e base.Emoji) { insert(e.Emoji) },
	Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
}
```

`Category` y `DefaultCategory` eligen el grupo de exploración. Las pestañas de categoría y las opciones de tono de piel forman cada una una parada de tabulación con navegación por flechas. `EmojiPickerRef` ofrece `Focus`, `Query`, `SetQuery`, `SetCategory` y `MatchedCount`; el selector con estilos usa el mismo estado de consulta. `LabelFunc`, `SearchLabelFunc` y `PlaceholderFunc` actualizan los textos en su sitio. `Emoji.WithSkinTone` aplica el tono elegido a ambas personas en una pareja, y el tono predeterminado restaura su forma neutra; buscar por un emoji conserva su secuencia exacta.

Usa `ui.EmojiPicker` para el selector con tema aplicado, iconos de categoría de Lucide, un campo de búsqueda limpiable, vista previa del emoji resaltado y muestras de tono de piel.

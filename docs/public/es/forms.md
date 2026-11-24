---
title: Formularios
description: Campos de texto, foco, el teclado en pantalla, interruptores, selectores y validación.
group: Conceptos básicos
order: 57
---

## Campos de texto

`ui.Input` es el campo de texto de la plataforma con el aspecto de `ui` y los campos de `base.Input`. Con `Value` es controlado: la aplicación guarda el valor y lo actualiza en `OnChange`; con `DefaultValue` mantiene el suyo propio.

```go
name, email := core.NewSignal(cx, ""), &core.Ref{}

ui.Input{
	Value:         name.Get,
	OnChange:      name.Set,
	Placeholder:   "Your name",
	ReturnKeyType: core.ReturnKeyNext,
	OnSubmit:      func(string) { email.Focus() },
}
```

`Masked` oculta los caracteres, `Readonly` impide editar mientras el texto aún se selecciona y copia (en miniprogramas solo en un campo de una línea de WeChat: un campo multilínea de WeChat y los campos de Alipay se desactivan como hace `Disabled`, ya que esos hosts no pueden deshacer una edición), `Disabled` desactiva el campo y `Multiline` ajusta el texto y crece con él salvo que su estilo lo dimensione. Las líneas de un campo multilínea, y las de su texto indicativo, tienen la altura del `LineHeight` de su estilo, con la fuente centrada en cada una como en un `core.Text`; un campo de una línea mantiene su línea centrada en su altura. `KeyboardType` elige el teclado en pantalla (`core.KeyboardEmail`, `core.KeyboardNumber`, `core.KeyboardURL` y los demás), `AutoComplete` indica al sistema qué contiene el campo para el autocompletado y `MaxLength` limita las unidades UTF-16 que introduce el usuario. `AutoCapitalize` y `NoAutoCorrect` ajustan el comportamiento del teclado.

`OnSelectionChange` y `OnCaretMove` informan la selección y dónde está el cursor, para un menú que se abra en él. `Suggest`, un `core.Suggest`, sigue una consulta después de un disparador como `@`, y un `ui.SuggestList` junto al campo ofrece filas para ella en el cursor, entre las cuales Arriba, Abajo, Intro y una pulsación eligen, como hace `Suggest` de `chat.Composer` para menciones. `OnKeyPress` escucha cada tecla, `OnPaste` lo que pega el usuario y `OnCompositionChange` sigue un método de entrada.

En iOS y Android, `cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, handler)` toma una tecla Intro en pantalla antes de que el campo envíe, pierda el foco o inserte un salto de línea. `KeyAuto` y `KeyNotEditing` se lo dejan a `SubmitBehavior`; en iOS, un Intro que confirma texto marcado permanece en el método de entrada.

## Foco y teclado

`Ref.Focus()` enfoca un campo o, en escritorios, la web y Apple TV, una vista pulsable habilitada o `Focusable`. `Ref.Blur()` quita el foco únicamente de su propia vista, y `cx.DismissKeyboard()` cierra el teclado en pantalla. `OnFocus` y `OnBlur` siguen el foco, y `View.OnFocus`/`OnBlur` hacen lo mismo para cualquier vista enfocable. Una vista enfocada que sale del árbol, al cerrarse su `Show` o desaparecer su fila, recibe `OnBlur` una vez, después del commit que la eliminó o, cuando la plataforma informa primero el siguiente foco, antes del `OnFocus` de esa vista, y las vistas alrededor que permanecen reciben `OnFocusOut`.

Las solicitudes de foco, desenfoque, desplazamiento y zoom de un `Ref` no hacen nada mientras su elemento está en el contenido de un `Holder` que ningún lugar muestra, y las que siguen en cola cuando el elemento sale del árbol se descartan. Volver a mostrar el contenido no las repite: solicítelo de nuevo una vez colocado.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

Los teclados en pantalla cubren la parte inferior de un teléfono. `core.EnvKeyboard` como relleno o margen inferior mantiene el contenido por encima, y su movimiento se anima con el teclado:

```go
core.View{Style: core.Style{
	Padding:    core.All(16),
	PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard},
	Gap:        12,
}, Children: form}
```

Una vista de desplazamiento alrededor de un formulario puede cerrar el teclado al arrastrarse y decidir qué hace un toque mientras se muestra el teclado:

```go
core.Scroll{Behavior: core.ScrollBehavior{
	KeyboardDismissMode:       core.KeyboardDismissOnDrag,
	KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
}, Children: []core.Node{form}}
```

`cx.Env().KeyboardHeight()` y `cx.Env().KeyboardDuration()` leen el tamaño y la temporización del teclado en código, y `cx.Env().SafeArea()` los márgenes seguros alrededor de la pantalla.

## Opciones

`ui.Switch` es un ajuste que surte efecto de inmediato, `ui.Checkbox` está marcado, desmarcado o indeterminado, y `ui.Radio` forma parte de un `ui.RadioGroup`. `ui.Select[T]` abre una lista de opciones bajo su disparador, o como una hoja cuando su `Presentation` lo pide, y `ui.Tabs[T]` muestra unas pocas a la vez.

```go
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}

size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options:  []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:    size.Get,
	OnChange: size.Set,
	Label:    "Size",
}
```

`ui.NumberInput` y `base.NumberInput` aceptan `LabelFunc` para un nombre accesible cambiante. Un `Format` personalizado sigue las señales que lee mientras el campo no se está editando; un cambio de idioma preserva el borrador activo hasta que termina la edición. `ui.NumberInput` incrementa y ajusta un número por arrastre dentro de límites, `ui.DatePicker` y `ui.TimeField` editan fechas y horas, `ui.ColorPicker` edita un color y `ui.TagInput` edita una lista de etiquetas.

`ui.OtpInput` recibe un código de un solo uso en celdas. Tanto él como `base.OtpInput` aceptan `LabelFunc` sin reemplazar el campo enfocado ni su código. `Size` establece el alto y ancho predeterminados, 36 puntos en escritorios y televisores y 44 en dispositivos táctiles. `CellWidth` y `CellHeight` sobrescriben cada dimensión de forma independiente. `TextStyle` sobrescribe la tipografía de la celda, `CornerRadius` cambia sus esquinas y `CaretHeight` sobrescribe la altura derivada de la altura de línea.

```go
quantity := core.NewSignal(cx, 1.0)
ui.NumberInput{Value: quantity.Get, OnChange: quantity.Set, Min: 1, Max: 99, Step: 1, Label: "Quantity"}
```

## Filas agrupadas

`ui.FormSection` coloca sus filas en una tarjeta, con el título encima y el pie debajo. Establezca `CardStyle` para el fondo, borde o esquinas de la tarjeta. `CardStyleFunc` se fusiona sobre él y sigue las señales que lee. `DividerInset` recibe cada fila hija, incluidas las filas creadas por `core.Each`: devuelva un margen y `true` para sobrescribir su divisor, o `false` para mantener la alineación automática del título (12 puntos para otros nodos). Un margen de cero crea un divisor de ancho completo.

`ui.FormRow` coloca `Leading` o un mosaico de icono antes del título. Establezca `LeadingWidth` para reservar espacio para su nodo y alinear el divisor inferior con el título. El espacio predeterminado es de 24 puntos. Establezca `TileSize` para escalar juntos el mosaico de icono, el glifo y las esquinas. `TitleStyle` y `DetailStyle` sobrescriben la tipografía del título y del detalle. `TitleMaxLines` limita las líneas del título (cero es ilimitado), `MinHeight` sobrescribe la altura mínima de la fila en puntos y `Below` coloca un nodo después del título y el detalle en su columna de texto. Un `MinHeight` de cero mantiene el valor predeterminado: 40 puntos, o 48 en dispositivos táctiles.

Para un conjunto de selección única, envuelva la sección en `base.RadioGroup` y establezca en cada fila `Role: core.RoleRadio`, `Checked` y `OnPress`. La fila usa `base.Radio` para sus estados de accesibilidad marcado y seleccionado, una parada de Tab y selección con teclas de flecha; pulsar una fila ya marcada no hace nada. La aplicación posee el valor elegido.

`ui.FormSection.TitleFunc` y `FooterFunc`, y `ui.FormRow.TitleFunc`, `DetailFunc` y `LabelFunc`, actualizan sus textos sin reconstruir la fila. Un detalle, título de sección o pie vacío elimina ese texto opcional; una etiqueta de fila vacía usa su título actual. Un `ui.Select` con búsqueda enlaza `SearchPlaceholderFunc` y `EmptyFunc` mientras conserva su búsqueda abierta y su selección.

## Validación

La aplicación posee las reglas: una señal guarda el error, `OnBlur` o `OnSubmit` lo establece y un `core.Show` añade un mensaje bajo el campo mientras exista uno.

```go
address := core.NewSignal(cx, "")
err := core.NewSignal(cx, "")
colors := ui.ColorsOf(cx)
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{
		Value:    address.Get,
		OnChange: func(v string) { address.Set(v); err.Set("") },
		Label:    "Email",
		OnSubmit: func(string) {
			if !strings.Contains(address.Get(), "@") {
				err.Set("Enter an email address.")
			}
		},
	},
	core.Show{
		When: func() bool { return err.Get() != "" },
		Then: func(*core.Scope) core.Node { return ui.Textf(err.Get, 12, ui.Regular, colors.Danger) },
	},
}}
```

`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` anima un mensaje que aparece, y `OnSubmit` con `ReturnKeyNext` mueve el teclado al siguiente campo.

## Siguiente

- [Navegación](navigation.md): dónde vive un formulario.
- [Texto](text.md): cómo se aplica estilo al texto de un campo.
- [Accesibilidad](accessibility.md): etiquetas y valores que necesitan los lectores de pantalla.

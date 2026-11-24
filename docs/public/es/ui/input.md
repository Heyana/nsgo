---
title: Campo de entrada
description: El campo de texto de la plataforma, controlado o no, con foco y el teclado en pantalla.
group: Componentes
order: 33
---

`ui.Input` es el campo de texto de la plataforma con el aspecto de `ui`, de una línea salvo con `Multiline`. Tiene los campos de `base.Input`, como todo campo con estilo; `ui.Field` es el mismo campo sin tamaño propio, en un borde que no sigue ni al puntero ni al foco; `core.Transparent` en su `Background` y `core.Zero` en su `BorderWidth` desactivan la caja, para texto que se lee como texto plano pero se edita.

```go preview=input/basic
name := core.NewSignal(cx, "")
ui.Input{
	Value:       name.Get,
	OnChange:    name.Set,
	Placeholder: "Your name",
	TestID:      "name",
}
```

## Controlado y no controlado

Con `Value`, el campo es controlado: la aplicación guarda el valor y lo actualiza en `OnChange`. Con `DefaultValue`, el campo mantiene el suyo propio. Un campo controlado puede limpiarse estableciendo su valor en `""`.

```go preview=input/submit
draft := core.NewSignal(cx, "")
ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "New task",
	OnSubmit: func(string) { add(); draft.Set("") }}
```

## Campos

`Multiline` ajusta el texto, comienza con tres líneas de altura y crece con su contenido salvo que su estilo establezca `Height` o `MinHeight`. `Masked` oculta los caracteres, `Readonly` impide editar mientras el texto aún se selecciona y copia, `Disabled` desactiva el campo y `MaxLength` limita las unidades UTF-16 que introduce el usuario. `Placeholder` puede ser una función con `PlaceholderFunc`.

```go
ui.Input{Masked: true, Value: password.Get, OnChange: password.Set,
	AutoComplete: core.AutoCompletePassword}
```

`KeyboardType` elige un teclado en pantalla (`KeyboardEmail`, `KeyboardNumber`, `KeyboardURL` y los demás), `ReturnKeyType` etiqueta su tecla Intro y `SubmitBehavior` indica si Intro envía o inserta un salto de línea en multilínea. `AutoCapitalize`, `NoAutoCorrect` y `AutoComplete` ajustan la ayuda del sistema.

`InputSource` selecciona una fuente de entrada de teclado mientras el campo tiene el foco y restaura la del usuario al perderlo, salvo con `KeepInputSource`: `"com.apple.keylayout.ABC"` mantiene en inglés un campo para códigos o comandos en macOS. El identificador es el de la plataforma, un identificador de distribución de teclado como `"00000409"` en Windows y un método de entrada Fcitx 5 o IBus en Linux; una fuente que no esté habilitada, los teléfonos y la web no cambian nada.

```go
ui.Input{Placeholder: "Bundle ID", InputSource: "com.apple.keylayout.ABC"}
```

`Style` se fusiona sobre el aspecto del campo, y `StyleFunc` sobre ese resultado: lo que establecen, como la altura, el relleno, la fuente o un color, reemplaza lo de `ui`, y el resto permanece. El borde se oscurece bajo el puntero y toma el color de acento mientras tiene el foco siempre que ninguno establezca `BorderColor` ni `Hover`. Un `Padding` cero mantiene el de `ui`; `core.Zero`, una longitud establecida, lo elimina (`Padding: core.All(core.Zero)`).

```go
ui.Input{Placeholder: "Filter", Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
ui.Field{DefaultValue: "Untitled", Style: core.Style{FontSize: 24, FontWeight: ui.Bold,
	Background: core.Transparent, BorderWidth: core.Zero}}
```

## Foco y eventos

`Ref` enfoca y desenfoca el campo. `OnFocus` y `OnBlur` siguen el foco, `OnSelectionChange` y `OnCaretMove` la selección y el cursor, `OnKeyPress` cada tecla pulsada, `OnPaste` lo que se pega y `OnCompositionChange` un método de entrada en composición.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

El teclado en pantalla cubre la parte inferior de un teléfono. Coloca `core.EnvKeyboard` en el relleno o margen inferior del contenido para que permanezca por encima del teclado; el movimiento se anima con él. `cx.DismissKeyboard()` cierra el teclado y `cx.Env().KeyboardHeight()` lee su altura.

## Validación

La aplicación es dueña de las reglas. Una señal guarda el error, `OnBlur` o `OnSubmit` lo establece y un mensaje debajo del campo lo muestra ([Formularios](../forms.md#validation)).

```go preview=input/validation
err := core.NewSignal(cx, "")
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{Value: email.Get, OnChange: email.Set, Label: "Email",
		OnSubmit: func(string) {
			if !strings.Contains(email.Get(), "@") {
				err.Set("Enter an email address.")
			} else {
				err.Set("")
			}
		}},
	ui.Textf(err.Get, 12, ui.Regular, colors.Danger),
}}
```

## Accesibilidad

El campo informa `RoleTextField`, o `RoleSearchField` para un cuadro de búsqueda cuando su `Role` lo indica, y lee su `Label`; `Placeholder` no es una etiqueta, así que asigna un `Label` a cualquier campo sin etiqueta visible. `Readonly` y `Disabled` se informan, y `OnSubmit` es la acción del teclado. Los campos de una línea, enmascarados y multilínea exponen el `Placeholder` actual como el marcador o sugerencia de accesibilidad nativo en macOS, iOS y Linux, actualizándolo y limpiándolo sin una segunda parada de foco para la etiqueta de marcador dibujada de un campo multilínea.

## Plataforma

iOS, Android, Windows y Linux usan sus campos nativos; la web usa un `<input>` o `<textarea>`. El teclado en pantalla, el autocompletado, los métodos de entrada y la dirección del texto son los de la plataforma. El foco y la escritura se prueban de la misma manera en todos los destinos.

## Relacionado

- [Formularios](../forms.md): campos, validación, opciones y el teclado.
- [Accesibilidad](../accessibility.md): etiquetas y foco.
- [Diseño](../layout.md): mantener el contenido por encima del teclado.
- Las páginas Input y Form de la galería.

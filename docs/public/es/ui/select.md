---
title: Selector
description: Un valor de una lista, como ventana emergente o como hoja.
group: Componentes
order: 36
---

`ui.Select[T]` elige uno de una lista de valores de cualquier tipo comparable. La lista se abre debajo del disparador en cada dispositivo; `Presentation: base.PresentSheet` la abre en su lugar como una hoja desde la parte inferior, como puede querer una aplicación móvil.

```go preview=select/basic height=280
size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options: []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:   size.Get,
	OnChange: size.Set,
	Label:   "Size",
}
```

## Opciones y búsqueda

`Options` son elementos `ui.Option[T]`, cada uno con un `Value`, un `Label` y una función `Group`, `Icon`, `Disabled` o `Content` opcionales propios de la aplicación. Las opciones adyacentes de un mismo `Group` no vacío se muestran bajo un encabezado de ese grupo, en un `Select` o un `Combobox`, que las teclas de flecha y la escritura omiten y los lectores de pantalla leen como un encabezado. Usa `OptionsFunc` cuando las opciones sigan a una señal, de modo que la lista abierta se actualice en el sitio y conserve el valor y el foco del campo. `Placeholder` se muestra mientras no hay ninguna opción elegida. `Searchable` filtra la lista al escribir, `OnClear` muestra un botón para limpiar y `Disabled` lo detiene.

```go preview=select/searchable height=400
ui.Select[string]{
	Options:  languages,
	Value:    language.Get,
	OnChange: language.Set,
	Searchable: true,
	SearchPlaceholder: "Search languages",
	OnClear:  func() { language.Set("") },
	Placeholder: "Choose a language",
}
```

`Style` da disposición al selector, con 200 puntos de ancho salvo que establezca un ancho; su altura, relleno y tipografía llegan al campo, igual que en un [Input](input.md), de modo que un selector quede alineado con un botón `SizeSmall`:

```go
ui.Select[string]{Options: sizes, Value: size.Get, OnChange: size.Set, Label: "Size",
	Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
```

## Comportamiento

Arriba, Abajo, Intro y Espacio abren la lista desde el disparador enfocado; las flechas mueven el resaltado y la escritura anticipada salta según los títulos de los elementos. Elegir un elemento cierra la lista e informa su valor. Escape, atrás y una pulsación fuera la cierran sin cambios. `Empty` es el texto que se muestra cuando la búsqueda no encuentra nada.

## Accesibilidad

`Label` nombra el campo para la tecnología de asistencia. Usa `LabelFunc` para actualizar ese nombre en el sitio conforme cambian sus señales. Una etiqueta vacía usa `Placeholder`. El disparador y la lista llevan los roles del selector, y se informa el elemento seleccionado. El teclado llega al disparador con Tab y lo abre con Intro o Espacio.

## Plataforma

En macOS, Windows y Linux la lista es una ventana emergente junto al disparador. En iOS y Android es una hoja, y la fila seleccionada permanece en el campo. Las mismas opciones y valores funcionan en todos los destinos.

## Relacionado

- [Formularios](../forms.md): selectores en un formulario.
- [Menú](menu.md): comandos en lugar de un valor.
- [Pestañas](tabs.md): unas pocas opciones mostradas a la vez.
- Las páginas Select y Combobox de la galería.
- [Componentes sin estilos](../authoring/base.md#selector): `base.Select`, con grupos y un disparador propio.

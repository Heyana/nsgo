---
title: Botón
description: Un botón pulsador, sus variantes y tamaños, y el botón de ícono.
group: Componentes
order: 31
---

`ui.Button` es un botón pulsador con el aspecto de `ui`, con las variantes y tamaños que necesita una ventana. `OnPress` se ejecuta al completarse una pulsación.

```go preview=button/basic
ui.Button{Text: "Save", OnPress: save}
ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: cancel}
```

## Variantes y tamaños

`ui.Button.Variant` es `ButtonPrimary` (el valor predeterminado), `ButtonSecondary`, `ButtonOutline`, `ButtonGhost`, `ButtonTinted` o `ButtonDanger`. Las variantes tinted y danger usan el color de acento o de peligro del tema sobre un relleno suave. En idiomas de interfaz de escritorio y TV, `Size` es `SizeDefault` (28 puntos), `SizeSmall` (24) o `SizeLarge` (32). En idiomas táctiles, los mismos tamaños son 44, 40 y 48 puntos, de modo que el botón puede usarse sin cambiarle el estilo.

```go preview=button/variants
ui.Button{Text: "Delete", Variant: ui.ButtonOutline, OnPress: remove}
ui.Button{Text: "Continue", Size: ui.SizeLarge, OnPress: next}
ui.Button{Text: "Save", Icon: lucide.Check, Flex: 1, OnPress: save}
```

`Color` rellena el botón con un color de la aplicación en lugar del de una variante, y `Tint` colorea su ícono. `Flex` le asigna una proporción dentro de una fila, mientras que `Style` define su disposición: puede hacer que el botón llene su contenedor, establecer un tamaño propio de la aplicación, agregar márgenes o redondear sus esquinas; `TestID` lo nombra para las pruebas.

```go
ui.Button{Text: "Continue", Size: ui.SizeLarge,
    Style: core.Style{Width: core.Pct(100), Height: core.Pt(50)}, OnPress: next}
```

Un botón compuesto únicamente por un ícono es `ui.IconButton`, con `Variant` `IconButtonGhost`, `IconButtonRaised` o `IconButtonFilled`, un círculo plano con el color de inserción del tema o `Fill`, un `Size` en puntos y una `Label` para la tecnología de asistencia. En escritorios, la etiqueta también se muestra como descripción emergente cuando el puntero reposa sobre el botón; `Tooltip` muestra un texto distinto allí, y una que devuelva `""` no muestra ninguna. Un `ui.Button` con `Icon` y sin `Text` también es cuadrado y necesita `Label`.

```go preview=button/icon
ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost, OnPress: remove}
```

## Contenido y estados

`ui.Button` muestra `Text`, un `Icon` inicial opcional, o ambos. Usa `TextStyle` para personalizar el tamaño de fuente, el grosor u otros estilos de texto de la etiqueta.

```go
ui.Button{Text: "Saving…", Disabled: saving.Get, OnPress: save}
```

`Disabled` recibe una función, de modo que el botón sigue una señal: un botón deshabilitado no acepta pulsaciones ni foco del teclado, y su apariencia es la del estado deshabilitado, no un color elegido por la aplicación.

`Busy` es para una acción que aún está en curso. Mientras devuelve verdadero, se muestra sobre el contenido un indicador giratorio en el color de texto del botón, por lo que también es visible sobre el acento de un botón primario, las pulsaciones no hacen nada y la tecnología de asistencia escucha que está ocupado. Por defecto, el contenido conserva su espacio debajo del indicador giratorio. El botón conserva el foco del teclado. `BusyLabel` o `BusyLabelFunc` muestra un mensaje junto al indicador giratorio y lo informa como el nombre accesible del botón mientras está ocupado. El botón mantiene al menos su ancho en reposo.

```go preview=button/busy
ui.Button{Text: "Save", Busy: saving.Get, BusyLabel: "Saving…", OnPress: save}
```

`ui.Spinner` y `ui.ProgressRing` aceptan un `Tint` propio para cualquier otra superficie sobre la que su acento desaparecería.

## Enlaces

`ui.Link{Text: "Docs", Href: "/docs"}` abre un destino con el rol de enlace. Su texto y el subrayado al pasar el cursor usan el acento, a menos que `Style.Color` establezca otro color; `Style.Hover.Color` establece el color al pasar el cursor.

## Accesibilidad

El botón informa `RoleButton` y su texto como su nombre. `Label` reemplaza el texto para la tecnología de asistencia. Un botón de solo ícono sin `Label` resulta ilegible, así que nómbralo.

Intro y Espacio pulsan un botón enfocado en escritorios, la web y Apple TV; el lector de pantalla de un teléfono lo pulsa como un clic.

## Plataforma

El botón es una vista dibujada de la misma manera en todas partes, no un botón de la plataforma; las pulsaciones, el foco y la accesibilidad provienen de la plataforma. Los idiomas táctiles usan objetivos de diseño más grandes como se describió anteriormente.

## Relacionado

- [Formularios](../forms.md): botones en un formulario y validación.
- [Estilos y temas](../styles.md): el tema del que un botón toma sus colores.
- [Accesibilidad](../accessibility.md): roles, etiquetas y foco.
- La página de Botón en la galería.
- [Componentes sin estilos](../authoring/base.md#button): `base.Button`, para un botón con un aspecto propio.

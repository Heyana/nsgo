---
title: Texto
description: Párrafos, fragmentos, enlaces, fuentes y documentos.
group: Componentes
order: 32
---

`core.Text` es un párrafo compuesto por el motor de texto de la plataforma. `ui.Text` y `ui.Textf` son funciones auxiliares para una línea en la tipografía de `ui`; el elemento sigue siendo un `core.Text`, por lo que también admite `Style` y `Spans`.

```go preview=text/basic
ui.Text("Settings", 20, ui.Bold, colors.Text)
ui.Textf(func() string { return fmt.Sprintf("%d files", n.Get()) }, 13, ui.Regular, colors.Muted)
```

## Contenido

`Content` es fijo y `ContentFunc` sigue una señal. `MaxLines` limita las líneas y `EllipsizeMode` recorta el resto al inicio, en medio o al final; `Style.WhiteSpaceNoWrap` mantiene un párrafo en una sola línea. `TextAlign`, `TextTransform`, `LineHeight` y `LetterSpacing` dan estilo al párrafo, y `OnTextLayout` informa sus líneas después de la disposición.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) },
	MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 15}}
```

## Fragmentos y enlaces

`Spans` crea un texto compuesto por fragmentos, cada uno con su propio estilo. Un fragmento con `OnPress` es un enlace, y su `Background` y `CornerRadius` se dibujan detrás de sus caracteres en cada línea. `SpansFunc` vuelve a entregar los fragmentos cuando cambia lo que lee, para código resaltado o una oración que cambia.

```go preview=text/spans
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "Read the "},
	{Content: "guide", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openGuide},
	{Content: " before you start."},
}}
```

`Attachment` coloca una vista en una línea, en lugar del contenido del fragmento: un icono, una imagen o un botón pequeño. Se dispone con su propio tamaño, se ajusta con las palabras y recibe sus propias pulsaciones.

## Fuentes y estilo

`FontFamily` nombra una familia o una lista CSS. `FontSize`, `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LetterSpacing`, `TextDecoration` y `TextShadowColor`, junto con su desplazamiento y radio, dan estilo al texto. `core.FontMonospace` es la familia monoespaciada de la plataforma.

```go
core.Text{Content: "fn main() {}", Style: core.Style{FontFamily: core.FontMonospace, FontSize: 13}}
```

Registra fuentes empaquetadas con `nsgo.Fonts` antes de la primera vista, o registra una tipografía mientras la aplicación se ejecuta con `cx.RegisterFont`. Un texto preparado antes de que llegue una fuente se prepara de nuevo.

## Documentos

`base/textview.TextView` muestra Markdown o HTML con las mismas primitivas: encabezados, párrafos, listas, tablas, código, notas al pie, alertas, imágenes, fórmulas y enlaces. `textview.Parse` lee Markdown y `textview.ParseHTML` lee HTML, y `Options.Streaming` repara un documento que aún está llegando.

```go preview=text/document height=240
textview.TextView{Markdown: "## Notes\n\nA **native** document.", Styles: ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) { open(e.URL) }}
```

`textview.NewSearch` busca en el texto que muestra un `TextView` y resalta sus coincidencias. Proporciona la misma búsqueda a varias vistas con `textview.ProvideSearch`, luego llama a `Open`, `SetQuery`, `Next` y `Previous`; `Count` y `Selected` informan del resultado. Las partes de la búsqueda se estilizan con `textview.PartSearch` y `textview.PartSearchSelected`.

## Selección y accesibilidad

`SelectableOn` permite al usuario seleccionar y copiar un texto. Una `core.View` con `Selectable` hace que sus textos se seleccionen juntos en orden de documento. Un `Role` como `RoleHeader` indica a la tecnología de asistencia qué es el texto, y `Label` lo reemplaza cuando las palabras no son la etiqueta.

## Plataforma

El motor de texto de la plataforma realiza la disposición del texto, por lo que los saltos de línea, las fuentes y los métodos de entrada lo siguen. Los saltos de línea CJK, los emojis y el texto de derecha a izquierda siguen al sistema. La aplicación puede leer el tamaño del texto con `cx.Env().FontScale()`, y un texto lo sigue a menos que se establezca `NoFontScaling`.

## Relacionado

- [Texto](../text.md): la guía completa de texto, fragmentos, fuentes y documentos.
- [Imágenes e iconos](../images.md): imágenes e iconos en línea.
- [Accesibilidad](../accessibility.md): encabezados, etiquetas y selección.
- Las páginas Text y TextView de la galería.

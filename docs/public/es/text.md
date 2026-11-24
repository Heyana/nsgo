---
title: Texto
description: Muestra texto con fragmentos, fuentes, decoraciones, selección y documentos.
group: Conceptos básicos
order: 53
---

## Un texto

`core.Text` es un párrafo, compuesto por el propio motor de texto de la plataforma en el lado del motor y dibujado por el hilo principal. Su tamaño proviene de su fuente y su ancho, y se ajusta en líneas dentro de su cuadro.

```go
core.Text{Content: "Hello from NSGo", Style: core.Style{
	FontSize:   15,
	LineHeight: 22,
	FontWeight: core.FontWeightSemibold,
}}
```

`Content` es fijo y `ContentFunc` sigue una señal, de modo que un texto que cambia la lee dentro de la función. `MaxLines` limita las líneas mostradas, `EllipsizeMode` recorta el resto al inicio, en medio o al final, y `WhiteSpaceNoWrap` mantiene un párrafo en una sola línea. `AdjustsFontSizeToFit` encoge el texto hasta que quepa en su cuadro y en su límite de líneas, hasta `MinimumFontScale` (al menos 0.1). Las fórmulas y otros adjuntos relativos a la fuente se encogen con él y vuelven a su tamaño natural cuando el espacio lo permite; los tamaños explícitos en puntos y el relleno conservan los valores especificados.

Los campos de texto heredan como lo hace CSS: un texto dentro de una vista con `FontSize`, `Color` o `LineHeight` los adopta a menos que establezca los suyos propios. `TextAlign`, `TextTransform`, `LetterSpacing`, `TextDecoration`, `TextDecorationStyle`, `TextDecorationColor` y `TextShadowColor`, `TextShadowOffset` y `TextShadowRadius` dan estilo a las líneas y a los glifos.

## Fragmentos y enlaces

`Spans` crea un texto compuesto por fragmentos, cada uno con su propio estilo. Un fragmento con `OnPress` es un enlace, y una pulsación en otro fragmento corresponde al propio `OnPress` del texto.

```go
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "By continuing you agree to the "},
	{Content: "terms", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openTerms},
	{Content: " and the "},
	{Content: "privacy policy", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openPrivacy},
	{Content: "."},
}}
```

`SpansFunc` vuelve a entregar los fragmentos cuando cambia lo que lee, y solo se vuelven a preparar las partes que difieren, de modo que las filas de un editor de código cambian de color a medida que llega el resaltado. `Background` y `CornerRadius` de un fragmento dibujan un cuadro detrás de sus caracteres en cada línea, y su `Cursor` indica el aspecto del puntero sobre ellos por delante del propio texto. `Visibility: core.VisibilityHidden` oculta un fragmento en su lugar: sus palabras conservan su espacio, así que las líneas no se mueven cuando aparece, y las copias y los lectores de pantalla lo omiten, útil para palabras que aparecen gradualmente una por una.

Establece `BackgroundInsetTop: core.Pct(62)` en un fragmento con `Background` para resaltar el 38 % inferior de su cuadro en cada línea. `BackgroundInsetBottom` recorta el borde inferior. Ambos márgenes interiores aceptan puntos o porcentajes de la altura del cuadro, desde el ascenso hasta el descenso de la fuente. No alteran la disposición del texto. En TSX, establece `backgroundInsetTop: "62%"` y `backgroundInsetBottom` en el estilo de un `<Text>` anidado. Los fondos redondeados y los márgenes interiores verticales también se dibujan en HarmonyOS y en los cuatro destinos de miniprogramas.

`Attachment` coloca una vista dentro de un texto, en lugar del contenido del fragmento. Se dispone con su propio tamaño, fluye con las palabras a su alrededor y recibe sus propias pulsaciones, de modo que un icono, una imagen o un botón pequeño pueden ir en una línea. Una selección copia el `Content` del fragmento en su lugar, omitiéndolo cuando está vacío. De forma predeterminada, su primera línea de texto se alinea con la línea base circundante usando las métricas de su fuente real. Un lienzo `Em` proporciona su propia línea base, y una vista sin línea base se apoya sobre su borde inferior. Establece `Style.VerticalAlign` para elegir otra alineación.

```go
core.Text{Style: core.Style{FontSize: 17, LineHeight: 24}, Spans: []core.Span{
	{Content: "Search with "},
	{Attachment: core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 17}}},
	{Content: " NSGo"},
}}
```

## Fuentes

`FontFamily` nombra una familia o una lista CSS (`Inter, "Noto Sans SC", sans-serif`), donde cada una se usa para los caracteres que faltan en las anteriores. Cada familia de la lista toma el grosor y el estilo. En macOS e iOS una línea toma su altura y línea base de las tipografías en que se componen sus caracteres, por lo que una línea compuesta por completo en una familia posterior toma las de esa familia, mientras que un navegador mantiene las de la primera familia como mínimo. `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LineHeightMultiple`, `LetterSpacing` y `LetterSpacingEm` establecen la tipografía y el espaciado, y `core.FontMonospace` es la familia monoespaciada de la plataforma.

`Language` es la etiqueta BCP 47 del texto, heredada como los campos anteriores en textos, fragmentos y campos de entrada. Los caracteres Han se dibujan con sus formas correspondientes, de modo que `直` configurado en `ja` adopta su forma japonesa incluso cuando el sistema prefiere el chino, y el coreano (`ko`) corta las líneas entre palabras como hace `word-break: keep-all` en CSS. Los textos y campos de entrada que no establecen ninguna usan el idioma que muestra la aplicación y lo siguen cuando cambia. Establécelo en una vista para todo un subárbol, o en un `Span` para un solo fragmento:

```go
core.Text{Spans: []core.Span{
	{Content: "日本語: "},
	{Content: "直 骨 角", Style: core.Style{Language: "ja"}},
}}
```

Empaqueta las fuentes de la aplicación bajo `resources/` y regístralas antes de la primera vista, o registra los bytes de una fuente mientras la aplicación se ejecuta:

```go
func main() { nsgo.Run(Root, nsgo.Fonts("fonts/Inter-Regular.ttf", "fonts/Inter-Bold.ttf")) }

cx.RegisterFont(data) // una fuente TrueType u OpenType
```

Un texto dispuesto antes de que llegue una fuente se vuelve a preparar, y los lienzos vuelven a ejecutar `Draw` para que sus glifos y mediciones de texto usen la fuente cargada. `cx.Env().FontScale()` es el tamaño de texto del usuario, que los textos siguen a menos que se establezca `NoFontScaling`; `MaxFontSizeMultiplier` lo limita.

## Selección y medición

`SelectableOn` permite al usuario seleccionar y copiar un texto, y una `View` con `Selectable` hace que sus textos se seleccionen juntos, en orden de documento, como en una página web. `SelectionColor` colorea el resaltado. `OnTextSelection` recibe un `core.TextSelection` a medida que el usuario mueve la selección: su texto plano `Text`, `Markdown` y `HTML` (conservando la estructura analizada en línea y en bloque de cualquier `TextView` y texto con estilo en el área, incluidos fragmentos parciales y enlaces), sus `Ranges` por texto y los `Bounds` de los textos que abarca, en la vista, para copiar como Markdown o HTML, crear una lista de palabras o traducir las palabras seleccionadas; `desktop.LookUp` las muestra en el diccionario del sistema ([servicios](services.md#opening-links-and-files)). `cx.MeasureText` mide una cadena en una fuente, útil para ajustar una columna a su texto; las funciones reactivas que la llaman se actualizan cuando cambian las fuentes disponibles, y `OnTextLayout` informa las líneas después de cada disposición.

## Documentos

`base/textview` muestra Markdown o HTML con las mismas primitivas: encabezados, párrafos, listas, tablas, código, notas al pie, alertas, imágenes, fórmulas y enlaces.

```go
textview.TextView{
	Markdown: "## NSGo\n\nNative views from **Go**.",
	Styles:   ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) {
		if strings.HasPrefix(e.URL, "#") {
			return
		}
		cx.OpenURL(e.URL, nil)
	},
}
```

Las fórmulas (`$…$`, `$$…$$`) muestran su TeX hasta que la aplicación pasa `RenderMath: formula.RenderMath` de `base/formula`, que las compone con el tamaño y el color del texto que las rodea, se leen en voz alta en el idioma de la aplicación y se copian como TeX; añade unos 6,5 MB a una aplicación de escritorio y 3 MB comprimidos con gzip a una web, incluida su fuente. `formula.View` muestra una fórmula en cualquier otro lugar, en TeX o Typst; su callback `OnHit` informa la parte de origen de Untie, el cursor de texto más cercano y los diagnósticos superpuestos para editores de fórmulas. En watchOS, las fórmulas usan la misma ruta de lienzo; enlazarlas añade unos 6,1 MB de ejecutable, incluida la fuente de 1,4 MB. Una ejecución en el simulador de reloj midió unos 3,9 MB de montón de Go retenido al cargar y analizar esa fuente, incluida su copia propia de bytes. Las mediciones en el simulador no establecen ni el límite de memoria de un reloj físico ni su margen disponible; mídelos en el reloj que ejecuta la aplicación. Los tramos de texto (`\text{}` o cadenas de Typst) que sus fuentes configuradas no pueden cubrir usan las fuentes del sistema, incluidos CJK y otros sistemas de escritura; los `formula.Fonts.Fallbacks` explícitos tienen prioridad.

Cuando el código fuente llega por partes, pasa `SourceFunc` a `formula.View` y establece `Streaming: true`, o vincula `StreamingFunc` al estado de recepción de la aplicación. Esto oculta la tinta de errores recuperables y marcadores de posición mientras el contenido válido continúa disponiéndose. Desactiva el indicador cuando termine la recepción para mostrar los errores restantes. En un `TextView`, proporciona una función `RenderMath` que devuelva ese `formula.View` con los valores `Math.TeX` y `Math.Display` proporcionados; su `StreamingFunc` puede leer el mismo estado de recepción.

Las fórmulas en bloque se dividen en relaciones y operadores seguros al ancho del bloque dentro de su relleno y bordes, manteniendo el tamaño de fuente del texto. Redimensionar o cambiar el tamaño del texto vuelve a ajustar las líneas. Las partes indivisibles siguen desplazándose lateralmente; las fórmulas en línea mantienen su tamaño natural.

Un bloque de código `mermaid` muestra su código fuente hasta que la aplicación pasa `RenderDiagram: diagram.RenderDiagram` de `base/diagram`, que lo dibuja como un diagrama nativo, con limu: diagramas de flujo, de secuencia, de clases, de estado y de entidad-relación, diagramas de Gantt, gráficos circulares, mapas mentales, líneas de tiempo y más, con la fuente de la plataforma, en claro u oscuro según muestre la aplicación. Los elementos del diagrama con enlaces los abren mediante el servicio de URL de la aplicación al pulsarlos, y la información emergente de Mermaid de un elemento aparece mientras el puntero descansa sobre él. Se dispone al ancho disponible del texto, girando un diagrama de flujo ancho para que fluya hacia abajo mientras conserva el tamaño de sus etiquetas; los diagramas que no caben siguen desplazándose lateralmente. Al copiar se obtiene su código fuente, y un bloque que aún se está recibiendo en streaming se muestra como código hasta que se completa. `diagram.View` muestra un diagrama en cualquier otro lugar y sigue el ancho de su contenedor conforme cambia. Su campo `Icons` asigna nombres de Mermaid como `"fa:fa-car"` a documentos `*core.SVG`, incluidos los de `icons/lucide`; los nombres ausentes no dejan hueco en línea. Los nodos de imagen cargan fuentes HTTP(S), de archivo o `asset:` mediante el cargador de imágenes compartido del lienzo, ajustándose y centrándose cuando se establece `constraint: "on"` y estirándose en caso contrario. Mermaid filtra las direcciones de imagen antes de cargarlas (incluido el rechazo de URL `data:`). Para proporcionar iconos en una vista de texto, establece `RenderDiagram` en un callback que devuelva `diagram.View{Source: code.Code, Icons: icons}`.

Para dar a los diagramas tu paleta de `ui`, devuelve un `diagram.View` con `ThemeFunc` desde tu callback `RenderDiagram`. La función lee los colores mientras se dibuja el lienzo, por lo que un cambio en la paleta o en la apariencia del subárbol vuelve a colorear la vista existente. Los colores que se dejan en cero mantienen los valores predeterminados del diagrama.

```go
func drawDiagram(cx *core.Scope, code textview.CodeBlock) core.Node {
	colors := ui.ColorsOf(cx)
	return diagram.View{Source: code.Code, ThemeFunc: func() *diagram.Theme {
		return &diagram.Theme{
			Text: colors.Text(), Surface: colors.Surface(), Border: colors.Border(),
			Line: colors.Muted(), Accent: colors.Accent(), Critical: colors.Danger(),
		}
	}}
}
// ui.TextView{Markdown: source, RenderDiagram: drawDiagram}
```

`textview.Parse` lee Markdown y `textview.ParseHTML` lee HTML en bloques que la aplicación también puede construir. `Options.Streaming` repara un documento que llega por partes, cerrando lo que su final deja abierto y reteniendo una última línea cuyo significado deciden los siguientes caracteres, como un marcador de lista aislado, para que una respuesta nunca muestre marcadores fugazmente como texto, y `StreamingCaret` dibuja un cursor de texto al final, útil para una respuesta escrita mientras se transmite.

Un `textview.Search` busca texto en las vistas que se unen a él, mediante su campo `Search` o `textview.ProvideSearch` por encima de ellas. `SetQuery`, `Open` y `Close` lo controlan desde una barra de búsqueda, `Count` y `Selected` indican en qué punto está, y `Next` y `Previous` recorren sus coincidencias en orden de lectura. Las vistas marcan cada coincidencia con `PartSearch` y la seleccionada con `PartSearchSelected`, para que los estilos de la aplicación las coloreen, la desplacen hasta hacerla visible y no alteren la selección del usuario.

```go
search := textview.NewSearch(cx)
textview.ProvideSearch(cx, search)
search.SetQuery("readme")
search.Open()
```

## Siguiente

- [Imágenes e iconos](images.md): imágenes y SVG, también dentro de un texto.
- [Estilos y temas](styles.md): colores y campos de texto que se heredan.
- [Disposición](layout.md): cómo un texto toma su ancho.

## Edición de texto enriquecido

`ui/richeditor.View` muestra una `base/richeditor.Session` con el aspecto del tema; `base/richeditor.View` proporciona el comportamiento con estilos suministrados por la aplicación. En iOS y Android, una pulsación larga selecciona una palabra y un doble toque hace lo mismo; arrastra los controladores de selección para extender el rango entre bloques. El menú de edición nativo ofrece Copiar y Seleccionar todo, y Cortar y Pegar mientras la sesión es editable, según corresponda a la selección y al portapapeles. `Session.SetEditable` también actualiza un menú abierto. Da estilo a los tiradores con `Styles.SelectionHandle` o `Classes.SelectionHandle` en la vista base.

Para un control de texto personalizado, enfoca un `core.Input` y llama a `Ref.OpenEditMenu(bounds, actions)` con límites en puntos de superficie y una máscara de bits de `core.EditCut`, `EditCopy`, `EditPaste` y `EditSelectAll`. En iOS y Android, `Input.OnEditAction` recibe Cortar, Copiar y Seleccionar todo; Pegar usa `Input.OnPaste`. Llama a `Ref.CloseEditMenu` para cerrarlo.

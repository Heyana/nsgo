---
title: Disposición
description: Flexbox, cuadrícula, tamaños y posicionamiento, con el área segura y el teclado integrados en la disposición.
group: Conceptos básicos
order: 51
---

## Cuadros

Cada elemento es un cuadro dispuesto por el motor. Los valores predeterminados son los de React Native: un flexbox en columna, hijos que conservan su tamaño salvo que crezcan o se encojan, elementos estirados a lo ancho del eje transversal y tamaños que incluyen el relleno (`padding`) y los bordes.

```go
core.View{
	Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.All(16), AlignItems: core.AlignCenter},
	Children: []core.Node{
		core.Text{Content: "One"},
		core.Text{Content: "Two"},
	},
}
```

`Column`, `Row`, `ColumnReverse` y `RowReverse` establecen el eje principal; `Wrap` y `WrapReverse` permiten que los hijos fluyan a más líneas. `JustifyContent` los coloca a lo largo de ese eje y `AlignItems`, o `AlignSelf` en un hijo, en el eje transversal. `Gap`, `RowGap` y `ColumnGap` los separan, y el relleno (`Padding`) y el margen (`Margin`) del estilo usan `core.All`, `core.Symmetric` o `core.Edges`.

## Tamaños

`Width`, `Height`, `MinWidth`, `MaxWidth` y `FlexBasis` aceptan `core.Pt` para puntos o `core.Pct` para un porcentaje del padre. `FlexGrow` reparte el espacio libre y `FlexShrink` permite que un cuadro ceda parte del suyo; un cuadro sin ninguno de los dos conserva su tamaño. `AspectRatio` es el ancho dividido entre el alto. `AutoMargin` toma el espacio libre en los lados que nombra, de modo que un lado empuja el cuadro hacia el otro.

```go
core.View{Style: core.Style{Direction: core.Row, Gap: 16}, Children: []core.Node{
	core.View{Style: core.Style{Width: core.Pt(220), FlexShrink: 0}},
	core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}},
}}
```

El contenido de un cuadro le da su tamaño hasta que un ancho o un alto indiquen lo contrario. `core.MinContent`, `core.MaxContent` y `core.FitContent` adoptan los tamaños que CSS define con esos nombres: el ancho `min-content` de un texto es su palabra más ancha. Un texto dentro de una fila solo se encoge y hace salto de línea con `FlexShrink: 1`; agrega `FlexGrow: 1` cuando deba ocupar el espacio restante. Un mínimo sin configurar ya es 0, como en React Native, así que `MinWidth: core.Pt(0)` no cambia nada y un texto que se encoge parte sus palabras cuando la fila es más estrecha que su palabra más ancha. `MinWidth: core.AutoMinimum` lo detiene en su palabra más ancha, tal como hace el mínimo automático de CSS.

Una pantalla que llena la ventana lleva `FlexGrow: 1` y `FlexShrink: 1`, igual que cada cuadro entre ella y una lista o vista desplazable que deba desplazarse. Solo con `FlexGrow`, un cuadro crece hasta el tamaño de su contenido, de modo que una lista larga lo estira más allá de la ventana en lugar de desplazarse.

## Cuadrícula

`DisplayGrid` dispone los hijos en CSS Grid. `GridTemplateColumns` y `GridTemplateRows` aceptan la sintaxis de pistas de CSS, y `GridRow` y `GridColumn` ubican un elemento por línea o extensión (`span`).

```go
core.View{Style: core.Style{
	Display: core.DisplayGrid,
	GridTemplateColumns: "repeat(3, 1fr)",
	GridAutoRows: "44px",
	Gap: 12,
}, Children: []core.Node{
	core.View{Style: core.Style{GridColumn: core.GridLines{Start: core.GridSpan(2)}}},
}}
```

`repeat(auto-fill, minmax(160px, 1fr))` crea tantas columnas como quepan. `GridFlow` elige el orden en que se rellena la cuadrícula, y `JustifyItems`, `JustifySelf` y `AlignItems` ubican los elementos en sus celdas.

## Posición

`Relative` es el valor predeterminado y mantiene un cuadro en el flujo; `Top`, `Right`, `Bottom` y `Left` lo desplazan. `Absolute` lo saca del flujo y lo sitúa respecto a su ancestro posicionado más cercano. `Fixed` lo sitúa en la vista raíz o en su modal, sin importar lo que se desplace, y `Sticky` lo fija dentro de su padre en la vista desplazable más cercana: `Top` desde arriba, o `Bottom` desde abajo cuando `Top` es automático (`Left` y `Right` horizontalmente). Un pie de página con `Position: core.Sticky, Bottom: core.Pt(0)` permanece abajo mientras el contenido largo se desplaza y fluye después del contenido corto. `ZIndex` apila los cuadros entre sus hermanos.

```go
core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Right: core.Pt(8)}}
```

`Static` mantiene un cuadro en el flujo e ignora sus desplazamientos (`insets`), de modo que los hijos con posición absoluta buscan un ancestro por encima de él. `Overflow` recorta los hijos al cuadro y `DisplayNone` saca un cuadro de la disposición.

Las sombras exteriores y el contorno propios del cuadro permanecen fuera de su recorte, como en CSS, en Android, Windows, Linux, la web y Apple, en todo tipo de vista, incluidas esquinas con distintos radios y contenido desplazable con zoom. Hasta que eso se verifique en un iPhone físico, `Bare` de `ui.Dialog` sigue colocando su recorte en una vista interior con las mismas esquinas, desplazada hacia dentro por el grosor del borde, lo que las conserva en todos los destinos:

```go
core.View{Style: core.Style{CornerRadius: 16, Shadows: []core.BoxShadow{ui.Shadow(0, 8, 24, -6, ui.Black(0.18))}},
	Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, CornerRadius: 16, Overflow: core.Hidden}, Children: content}}}
```

## Dirección y el entorno

`LayoutDirection` dispone un subárbol de izquierda a derecha o de derecha a izquierda, por encima de la dirección de la aplicación: `DirectionRTL` y `DirectionLTR` la establecen, y por defecto hereda el valor. Izquierda y derecha en márgenes, rellenos, bordes e `insets` se intercambian con la dirección, de modo que funcionan como inicio y fin tal como en React Native. `PhysicalEdges` y `PhysicalBorders` mantienen los lados nombrados donde están. `WritingDirection` ordena fragmentos de texto de dirección mixta, y `WritingAuto` la toma de la primera letra fuerte.

`PaddingEnv` y `MarginEnv` suman el entorno a un lado. `core.EnvSafeArea` es el margen del área segura en ese borde y `core.EnvKeyboard` la altura del teclado en pantalla, de modo que el contenido se mantiene por encima del teclado mientras su desplazamiento se anima con él.

```go
core.View{Style: core.Style{
	Padding: core.All(16),
	PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea},
}}
```

`core.EnvAll(core.EnvSafeArea)` nombra todos los bordes, y `cx.Env().SafeArea()` y `cx.Env().KeyboardHeight()` leen los valores en código.

En un teléfono plegable o de doble pantalla, `cx.Env().Geometry()` lista las regiones en que un pliegue o bisagra divide la ventana, cada una con el área segura que cae en ella, y los pliegues entre ellas; su `State` y `Angle` indican qué tan abierto está el dispositivo (`core.HingeClosed`, `HingeHalfOpen` o `HingeOpen`) incluso cuando ningún pliegue cruza la ventana, como en una pantalla externa o una pantalla abierta por completo. Otros dispositivos reportan una sola región y `HingeUnknown`. En iOS 27.1, `cx.Env().VerticalBarEdge()` informa el borde elegido por el sistema para una barra vertical (`core.VerticalBarLeading`, `core.VerticalBarTrailing` o `core.VerticalBarUnspecified`), que `mobile.TabBar` y `ui.TabShell` siguen cuando `Edge` es `mobile.TabBarAutomatic`. Nada se dispone implícitamente según la geometría: la aplicación elige sus propios paneles.

## Clases adaptables

`Class` acepta clases de Tailwind CSS: `flex`, `flex-col`, `flex-wrap`, `items-center`, `justify-between`, `gap-3`, `p-4`, `w-full`, `grid`, `grid-cols-3`, `col-span-2` y el resto. Los puntos de quiebre siguen a la ventana, y la apariencia de la pantalla, la dirección y el movimiento reducido siguen al entorno:

```go
core.View{Class: "flex flex-col gap-3 p-4 md:flex-row md:gap-6 dark:bg-zinc-900 rtl:flex-row-reverse"}
```

`sm:`, `md:`, `lg:`, `xl:` y `2xl:` se aplican desde 40, 48, 64, 80 y 96 rem de ancho; `max-md:` se aplica por debajo de uno. `portrait:`, `dark:`, `rtl:`, `motion-reduce:`, `contrast-more:`, `pointer-fine:`, `pointer-coarse:`, `ios:`, `android:` y `web:` siguen lo que indica el entorno. Las clases se combinan debajo de un `Style`, y una clase de estado (`hover:`) pasa a ser un estilo de estado, de modo que ambos pueden mezclarse.

## Siguiente

- [Estilos y temas](styles.md): colores, esquinas, sombras y el aspecto de un subárbol.
- [Texto](text.md): cómo el texto toma su tamaño y ajusta sus líneas.
- [Listas y desplazamiento](lists.md): contenido que se desplaza y filas que se reciclan.

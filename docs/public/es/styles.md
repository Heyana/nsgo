---
title: Estilos y temas
description: Estilos tipados, clases de Tailwind, estilos de estado y temas claro y oscuro.
group: Conceptos básicos
order: 52
---

## Un estilo

`core.Style` es un único valor de Go para la disposición, el aspecto y el texto de un cuadro. El valor cero de cada campo equivale a no estar definido, por lo que los estilos se componen: `Merge` reemplaza solo lo que establece el estilo posterior, y un hijo hereda los campos de texto que establecen sus padres. Para establecer una longitud en nada sobre un estilo que indica una, usa `core.Zero` en `Padding`, `Margin`, `Borders`, `BorderWidth`, `CornerRadius` o una separación, tal como hace `core.Transparent` para un color: `Padding: core.All(core.Zero)`.

```go
card := core.Style{
	Background: core.RGB(0xffffff),
	CornerRadius: 12,
	Padding: core.All(16),
	Gap: 12,
	BorderWidth: 1,
	BorderColor: core.RGBA(0x000000, 0.1),
}
```

Los estilos admiten los colores, esquinas, bordes, contornos, sombras, degradados, filtros, desenfoques de fondo, máscaras, modos de fusión, transformaciones 3D y transiciones de CSS. `core.RGB`, `core.RGBA` y `core.ParseColor` crean colores, `core.LinearGradient` y sus afines crean degradados, `core.Ring` crea el anillo de Tailwind y `core.AllCorners` asigna un radio a cada esquina. `BorderCurveContinuous` redondea las esquinas de forma continua hacia sus bordes como lo hace iOS. Las sombras y el contorno de un cuadro permanecen fuera de su recorte `Overflow` salvo donde indica [Disposición](layout.md#posición). Los filtros de fondo cambian lo que está detrás de una vista tal como lo hace `backdrop-filter` de CSS: un `Blur` lo desenfoca según su radio y los filtros de color cambian sus colores. iOS dibuja el material del sistema que nombra `View.IOS.BackdropMaterial` para un desenfoque sin filtros de color; un desenfoque que nombra un material distinto de `core.BackdropUltraThin` dibuja ese material en iOS y el más cercano de AppKit en macOS, omitiendo los filtros de color. Linux y Android no dibujan filtros de fondo. `Style.GlassEffect` (`core.GlassRegular` o `core.GlassClear`, `glassEffect` en TSX) dibuja Liquid Glass en lugar de un desenfoque de fondo en macOS 26 e iOS 26 (y el shader de vidrio de NSGo en Windows; proporciona un desenfoque a la vista de vidrio como respaldo en los demás destinos). Establecer `Style.GlassInteractive` (`glassInteractive` en TSX) habilita la respuesta interactiva de pulsación del sistema para Liquid Glass en macOS 27 (`NSGlassEffectView.effectIsInteractive`) e iOS 26 (`UIGlassEffect.isInteractive`), manteniendo en funcionamiento `OnPress` y los hijos pulsables.

`Transform` mueve el destino del puntero junto con la vista: las pulsaciones, el desplazamiento del puntero y los gestos comienzan donde se dibuja, y las posiciones de puntero informadas deshacen la transformación. Un hijo transformado fuera de su padre sigue siendo alcanzable a menos que un recorte o un área de desplazamiento lo oculte. Los miniprogramas aún no deshacen los fotogramas clave que ejecuta su capa de renderizado ([hoja de ruta](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md#mini-programs)).

```go
core.View{Style: core.Style{
	CornerRadii: core.Corners{TopLeft: 28, BottomRight: 28},
	BorderCurve: core.BorderCurveContinuous,
	Shadows:     []core.BoxShadow{core.Ring(1, core.RGBA(0x000000, 0.08)), ui.Shadow(0, 8, 24, -6, ui.Black(0.18))},
}}
```

## Estados

El aspecto de una vista puede cambiar según su estado. `Hover`, `Focus`, `FocusWithin`, `Active`, `Selected`, `Checked`, `Disabled` y los estados `aria-*` son valores `*Style` que se combinan sobre el resto mientras el estado se mantiene. Una aplicación establece `Selected`, `Checked`, `Disabled` y `Expanded` mediante funciones, y la plataforma informa las pulsaciones y el foco.

`Active` y `group-active:` siguen a una vista mientras está pulsada, lo que requiere un manejador de pulsación o un `Responder` que retenga el toque. Dar estilo a una vista simple con `Active`, `active:` o `group` no la hace pulsable ni una parada de tabulación del teclado; establece `Focusable` cuando deba recibir el foco por sí misma.

```go
core.View{
	OnPress: func(core.PressEvent) { open() },
	Style: core.Style{
		Background: core.RGB(0x007aff),
		Active:     &core.Style{Opacity: core.Alpha(0.85)},
		Hover:      &core.Style{Background: core.RGB(0x006be0)},
	},
	Disabled: func() bool { return saving },
}
```

Una parte de un componente puede tomar los estados del control que la rodea sin informarlos por sí misma: `PartStates` proporciona `Checked`, `Selected`, `Disabled` y el resto, de modo que la marca de una casilla de verificación sigue a la casilla también para los lectores de pantalla.

## Clases

`Class` en `View`, `Text`, `Span`, `Input`, `Image`, `Scroll` y `Canvas` acepta clases de Tailwind CSS 4, y cada parte de un componente de `base` tiene una clase junto a su estilo. Las utilidades, variantes y valores arbitrarios significan lo mismo que en Tailwind, sobre su tema y paleta predeterminados.

```go
core.View{Class: "flex items-center gap-3 rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"}
```

Una clase se combina debajo del estilo, por lo que prevalecen los campos del estilo. Las clases pueden seguir lo que cambia con `ClassFunc`, y una variante de estado (`hover:bg-zinc-100`, `checked:opacity-100`) pasa a ser el estado del estilo. `group` y `group-hover:` aplican estilo a los hijos según el estado de un padre, `first:` y `last:` siguen la posición de un hijo, y `aria-checked:`, `data-open:` y `focus-within:` siguen los estados que muestra un control. `pointer-fine:` y `pointer-coarse:` aplican estilo a un subárbol para un ratón o una pantalla táctil, coincidiendo con la característica de medio `pointer` de CSS a través de `cx.Env().Pointer()` (la consulta `pointer` del navegador en la web, preciso en ordenadores de escritorio y Mac, impreciso en teléfonos, tabletas y relojes, y ninguno en televisores), y se componen con otras variantes como `dark:` y `hover:` para que la aplicación elija explícitamente el tamaño táctil o de puntero en lugar de que `core` lo cambie implícitamente.

Marca un control con `peer` para aplicar estilo a sus hermanos posteriores según su estado; por ejemplo, una casilla de verificación con `Class: "peer"` seguida de una etiqueta con `Class: "peer-checked:text-accent peer-disabled:opacity-50"`. Un `peer` aplica estilo solo a los hermanos posteriores a él bajo el mismo padre, incluidos `Show` y `Each`, nunca a hermanos anteriores ni a las vistas dentro de él. Usa `peer/field` con `peer-checked/field:` para nombrar un conjunto, y `peer-data-invalid:` para el estado de datos booleano `invalid` de un control. Los marcadores y los estados se actualizan de forma reactiva. Marcar una vista no la convierte en una parada de tabulación ni hace que reciba pulsaciones, por lo que `peer-active:` sigue únicamente a un `peer` que recibe pulsaciones por sí mismo, como un botón.

`has-[>svg]:px-3` cambia el relleno de un padre cuando tiene un hijo directo que es un icono; `[&_svg]:size-4 [&_svg]:shrink-0` dimensiona sus iconos descendientes. `svg` selecciona `core.Icon` y `SVGView`, incluso a través de regiones condicionales. Estos selectores se actualizan cuando cambian los hijos, las clases o los estados. `hover:[&_svg]` sigue el estado `hover` del padre; `[&_svg]:hover` sigue el de cada icono. El `active` de un selector sigue solo a vistas que reciben pulsaciones, como un botón, al igual que `peer-active:`. Los selectores coincidentes compiten por especificidad antes que por el orden de variantes; los estilos en línea prevalecen a menos que la clase sea importante.

Los selectores coinciden con los elementos de NSGo en todos los destinos. Admiten listas, combinadores de hijo (`>`), descendiente (`_`), hermano adyacente (`+`) y hermano siguiente (`~`), clases, presencia/igualdad de `data-*`, roles, estados booleanos de accesibilidad compatibles y `:is`, `:where`, `:not`, `:has` y posiciones de hijos. Los tipos son `view`/`div`, `text`/`span` (nodos de texto completos), `scroll`, `input`, `textarea`, `image`/`img`, `canvas`, `svg`, `video`, `webview` y `native`. Usa un `&` de nivel superior en cada alternativa de selector arbitrario; sin él, el selector evalúa al propio elemento. Los selectores exclusivos del navegador, los pseudoelementos, las reglas arroba (`at-rules`), los escapes y el `&` explícito dentro de una función son desconocidos; estos selectores no alcanzan las vistas de implementación del backend ni los fragmentos individuales de texto.

Las variantes `peer-*` en `Span.Class` o `Screen.HeaderClass` no tienen hermanos a los que seguir. Se ignoran y se registran una vez por clase y contexto; coloca en su lugar la variante `peer` de un fragmento en el `Text` que lo contiene.

El campo `Tailwind` del tema amplía el tema predeterminado con colores, puntos de quiebre y utilidades, y un color que no está en la paleta nombra un rol de `Palette` (`bg-accent`, `text-text-muted/70`) o un color del tema, leído desde el tema del nodo en la apariencia que muestra.

## Temas

Un `core.Theme` es un único aspecto en ambas apariencias: una `Palette` para claro y otra para oscuro, un radio de esquina y familias tipográficas. Los roles de la paleta son `Background`, `Surface`, `Text`, `TextMuted`, `Border`, `Accent`, `AccentText`, `Danger`, `Warning`, `Success`, `Selection`, `Focus` y `Scrollbar`; `Colors` contiene los que añade una biblioteca, como `inset` y `raised`. `core.DefaultTheme()`, que una aplicación muestra hasta que establece otro, es el tema neutro con un acento azul y los roles de `ui`, y `ui.WarmDuskTheme()` es una paleta cálida.

```go
theme := core.DefaultTheme()
theme.Light.Accent, theme.Dark.Accent = core.RGB(0xbd481b), core.RGB(0xed986e)
theme.Radius = 10
cx.SetTheme(theme)
```

`cx.SetTheme` asigna un tema al ámbito y a todo lo que se construye debajo de él, de modo que una barra lateral o un panel pueden tener su propio tema; un tema nulo (`nil`) toma el superior. `core.ParseTheme` lee un archivo de tema YAML o JSON, o las variables CSS de un archivo de tema de shadcn/ui, con colores de rol, un radio y familias tipográficas. `core.DefaultTheme`, el tema que muestra una aplicación hasta que establece uno, y `ui.WarmDuskTheme` devuelven temas listos para modificar.

`cx.SetAppearance` fuerza toda la aplicación a claro u oscuro, `cx.SetSubtreeAppearance` lo hace para un subárbol y `core.AppearanceSystem` vuelve a seguir al sistema. `cx.Dark()` indica qué apariencia muestra un subárbol y `cx.Palette()` proporciona sus colores con las anulaciones del host incrustante aplicadas.

Cuando ningún ancestro establece `Style.Color`, el texto y los iconos `currentColor` usan `Palette.Text` del subárbol, siguiendo en el acto los cambios de tema, apariencia y tokens del host. Un color explícito, incluido `core.Transparent`, sigue heredándose.

## Colores en componentes

Un componente lee sus colores donde se construye y en sus `StyleFunc`, de modo que un cambio de tema o apariencia se aplica en el acto, sin volver a construir el componente. `ui.ColorsOf(cx)` nombra los roles que usa `ui`, `ui.TintsOf(cx)` proporciona sus rellenos translúcidos y `ui.FinOf(cx)` proporciona los acabados por capas de `ui`.

```go
colors := ui.ColorsOf(cx)
core.Text{Content: "Hello", StyleFunc: func() core.Style {
	return core.Style{Color: colors.Text(), FontSize: 15}
}}
```

`StyleFunc` vuelve a ejecutarse cuando cambia lo que leyó, por lo que `cx.Dark()` y `cx.Palette()` dentro de ella siguen al tema. Lo mismo se aplica a `ClassFunc`, al `StyleFunc` de un fragmento (`span`) y a los campos `StyleFunc` de un componente.

## Cursores

`Style.Cursor` establece un cursor de puntero con nombre (`core.CursorText`, `core.CursorColResize`, etc.); deja los elementos pulsables en `core.CursorAuto` para que conserven la flecha de la plataforma. Para herramientas de dibujo, juegos o punteros personalizados de lienzo en escritorio y en la web, `Style.CursorImage` recibe un `*core.CustomCursor` con una `Image` 1x, una `Image2x` HiDPI opcional (elegida automáticamente cuando `PixelRatio >= 1.5`), un `Hotspot` en puntos lógicos limitado a los bordes de la imagen y un cursor con nombre `Fallback`. Comprueba `cx.Env().SupportsCustomCursor()` para saber si el destino actual muestra imágenes de cursor personalizadas; las plataformas táctiles las ignoran.

## Siguiente

- [Disposición](layout.md): adónde van los cuadros.
- [Texto](text.md): fuentes, fragmentos y decoraciones.
- [Animación](animation.md): transiciones entre estilos.

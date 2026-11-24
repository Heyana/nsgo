---
title: Pestañas
description: Una lista de pestañas, un control segmentado y la barra de pestañas del sistema.
group: Componentes
order: 40
---

`ui.Tabs[T]` elige uno de unos pocos valores, como un control segmentado o una lista de pestañas; la aplicación muestra junto a él el contenido del valor elegido. Una lista de pestañas es una sola parada de Tab: las flechas se desplazan por las pestañas.

```go preview=tabs/basic
section := core.NewSignal(cx, "overview")
ui.Tabs[string]{
	Options: []ui.Option[string]{
		{Value: "overview", Label: "Overview"},
		{Value: "activity", Label: "Activity"},
		{Value: "settings", Label: "Settings"},
	},
	Value:    section.Get,
	OnChange: section.Set,
	Label:    "Sections",
}
```

## Variantes y tamaños

`Variant` es `TabsSegmented` (el valor predeterminado, un control elevado en una pista hundida) o `TabsUnderline` (una línea de acento bajo la pestaña seleccionada). Un subrayado puede establecer `IndicatorColor` y usar `IndicatorFit: TabsIndicatorContent` para seguir a su etiqueta e icono en lugar de toda la pestaña; en TSX son `indicatorColor` e `indicatorFit="content"`. `Size` es `SizeDefault`, `SizeSmall` o `SizeLarge`, y `Equal` da a cada pestaña el mismo ancho en lugar de ajustarse a su etiqueta.

```go preview=tabs/underline
ui.Tabs[string]{Variant: ui.TabsUnderline, Equal: true, Size: ui.SizeSmall, Options: tabs, Value: section.Get, OnChange: section.Set}
```

Una opción puede llevar un `Icon`, una función `Disabled`, un nodo `Content` y un `TestID`. `Content` reemplaza la etiqueta y hereda el estilo de texto de la pestaña.

Usa `OptionsFunc` cuando las pestañas sigan una señal. NSGo actualiza las filas en su lugar según sus valores, manteniendo la pestaña seleccionada y el foco del teclado cuando ese valor sigue estando disponible; un valor que desaparece queda sin seleccionar hasta que la aplicación lo cambia.

## Barras de pestañas

`ui.TabShell` mantiene las secciones de nivel superior junto a una barra de pestañas en cualquier ancho de ventana: en la parte inferior de forma predeterminada, o verticalmente a lo largo del borde inicial o final cuando `Edge` (o `EdgeFunc`) es `mobile.TabBarLeading`, `mobile.TabBarTrailing` o `mobile.TabBarAutomatic` (que sigue a `cx.Env().VerticalBarEdge()`). Usa la barra del sistema cuando cada pestaña proporciona `SystemIcon` o un `Icon` SVG, en cualquier borde en iOS y en el borde inferior en Android; un `SystemIcon` que nombra un SF Symbol en iOS o un recurso drawable en Android tiene prioridad, y de lo contrario se muestra el SVG. En caso contrario, dibuja la barra con los iconos SVG proporcionados. Una barra que ocupa su propio espacio se mantiene por sí misma fuera del área segura de su borde, por lo que `core.EnvSafeArea` en ese borde de una sección contigua, incluida la de una página `base.Nav`, es 0; bajo la barra flotante inferior del sistema en iOS, mantén el contenido despejado con `ui.TabBarInset`.

Establece `ShellTab.TextFunc` para títulos de pestañas que sigan la configuración regional o el estado de la aplicación y `TabShell.LabelFunc` para el nombre accesible de la barra. Cambiar un título conserva la sección seleccionada y la pestaña existente siempre que su icono siga siendo el mismo.

`mobile.TabBar` es la barra de pestañas del sistema en iOS y Android, con `Items`, `Selected`, `OnChange` y el parámetro opcional `Edge` (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing` o `mobile.TabBarAutomatic`) / `EdgeFunc`. Cada `mobile.TabBarItem` acepta un `Title` y un nombre de icono de plataforma en `Icon`; `SVG: lucide.House()` se muestra siempre que `Icon` no nombre ningún SF Symbol en iOS ni ningún recurso drawable en Android, ajustado en 24 puntos y teñido para la selección. En TSX, `svg` acepta un documento SVG y `edge` acepta `"bottom"`, `"leading"`, `"trailing"` o `"automatic"`. En otros destinos `mobile.TabBar` no ocupa espacio, así que muestra `ui.Tabs` allí.

`ui.SidebarShell` mantiene lo que albergan su barra lateral, pie de página, encabezados, contenido y panel derecho fuera del área segura izquierda y derecha por sí mismo, con sus fondos extendiéndose hasta los bordes de la ventana, de modo que `core.EnvSafeArea` en esos lados es 0 en su interior, y `ui.NavHeader` añade el área segura lateral a su relleno. El contenido de una `ui.NavPage` o de una sección de `TabShell` se mantiene fuera del área segura por sí mismo, con `PaddingEnv`, de modo que lo que deba llegar hasta los bordes, como una imagen a sangre completa, pueda hacerlo.

## Accesibilidad

La lista informa `RoleTabList`, cada pestaña `RoleTab` y la pestaña seleccionada su estado. La lista es una sola parada de Tab; Izquierda y Arriba, Derecha y Abajo, Inicio y Fin se mueven entre las pestañas habilitadas, y la pestaña alcanzada informa una pulsación. `Label` da nombre a la lista.

## Plataforma

iOS y Android muestran la barra de pestañas del sistema con su propio aspecto y gestos; macOS, Windows, Linux y la web muestran `ui.Tabs`. El contenido de la aplicación es el mismo en todos los destinos.

## Relacionado

- [Navegación](../navigation.md): páginas y barras de pestañas en una aplicación.
- [Menú](menu.md): comandos en lugar de secciones.
- [Selector](select.md): muchas opciones en una lista.
- Las páginas Tabs y NavStack de la galería.
- [Componentes sin estilo](../authoring/base.md#tabs): `base.Tabs`, para pestañas con un aspecto propio.

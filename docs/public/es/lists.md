---
title: Listas y desplazamiento
description: Vistas desplazables con gestos nativos y listas que reciclan filas para cualquier número de elementos.
group: Conceptos básicos
order: 55
---

## Una vista desplazable

`core.Scroll` es la vista desplazable propia de la plataforma, por lo que la inercia, los rebotes, la barra de desplazamiento, el ajuste a puntos (`snapping`) y el desplazamiento táctil son nativos. Sus hijos se disponen en columna, o en fila con `Horizontal`, tan extensos como necesiten.

```go
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

La vista desplazable llena su contenedor y el relleno va en su contenido, para que la barra de desplazamiento quede pegada al borde. Se encoge para ajustarse a su padre a menos que su estilo indique lo contrario. Establece `NoShrink` cuando deba conservar su tamaño solicitado mientras un hermano se encoge. `HideIndicator` oculta el indicador de la plataforma, y `Bar` dibuja el propio de la aplicación.

`Behavior` y `BehaviorFunc` eligen cómo se desplaza: `Paging`, `SnapToInterval` y `SnapToOffsets` con `SnapToAlignment`, un `DecelerationRate`, `NoBounces`, `ContentInset` y las reglas del teclado. `MinZoomScale` y `MaxZoomScale` permiten hacer zoom con pellizco, `OnRefresh` y `Refreshing` habilitan tirar para actualizar, y `OffsetValues` sigue su desplazamiento en el hilo principal para efectos que se mueven con él.

```go
page := core.Scroll{
	OnScroll: func(e core.ScrollEvent) { y.Set(e.Offset.Y) },
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: []core.Node{...},
}
```

`OnScroll` informa el desplazamiento como máximo una vez por turno, o una vez cada `ScrollEventThrottle`. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd` y `OnScrollEnd` delimitan los gestos del usuario. `Ref` desplaza desde código con `ScrollTo`, `ScrollToEnd`, `ZoomTo` y `ScrollToIndex` para una lista.

## Una lista

`core.List` renderiza datos extensos en una vista desplazable nativa creando solo las filas cercanas a la pantalla. `Count` indica cuántas filas hay y `Row` construye una; `Key` da a cada fila una identidad, de modo que las filas que permanecen conservan sus celdas mientras otras filas aparecen y desaparecen por encima de ellas.

```go
core.List{
	Count:         func() int { return len(items.Get()) },
	Key:           func(i int) any { return items.Get()[i].ID },
	EstimatedSize: 56,
	Row: func(cx *core.Scope, r core.ListRow) core.Node {
		return core.View{Style: core.Style{Padding: core.All(12)}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return items.Get()[r.Index()].Title }},
		}}
	},
}
```

`Row` se ejecuta una vez por celda reciclada y actualiza `r.Index()` cuando esa celda se reutiliza para otra fila, así que lee `r.Index()` y las señales del elemento dentro de funciones reactivas (`ContentFunc`, `StyleFunc`, `NewMemo`) en lugar de capturar un valor único al construir la fila. Las celdas inactivas quedan estacionadas de modo que sus cálculos esperan hasta que la celda vuelva a vincularse a una fila activa.

`Header`, `Footer`, `Empty` y `Separator` contienen el contenido alrededor y entre las filas. `NumColumns` coloca filas lado a lado, `Gap` y `ColumnGap` las separan, y `Horizontal` desplaza horizontalmente. `StickyHeaderIndices` fija esas filas al inicio del área visible hasta que la siguiente las empuje fuera, y `StickyHeaderOffset` deja libre un encabezado por encima de ellas.

`OnViewableItemsChange` informa las filas visibles según los umbrales de `ViewabilityConfig`, y `OnLoadMore` se ejecuta cuando el área visible se acerca al final, una vez por cantidad de filas. `OnLoadMoreAtStart` hace lo mismo al principio, e `InitialScrollIndex` abre la lista en una fila determinada. `Alignment: core.ListAlignEnd` deja reposar la lista en su final, como hace un chat con sus filas en orden de lectura: se abre en su fila más reciente, sigue las nuevas filas mientras el usuario está al final y mantiene una conversación corta en la parte inferior. `Inverted` coloca en su lugar la fila 0 al final, y `MaintainVisibleContentPosition` sigue el final de una lista en crecimiento mientras el usuario está cerca de él.

```go
core.List{Ref: list, Count: count, EstimatedSize: 64, Row: row, OnLoadMore: loadMore,
	StickyHeaderIndices: headers(cx), Footer: core.View{Style: core.Style{Height: core.Pt(24)}}}
```

`list.ScrollToIndexAt(index, 0.5, 0, true)` desplaza una fila al centro del área visible, y `list.ScrollToEnd(true)` hasta el final. `list.ListPosition()` lee dónde empieza el área visible, una fila y cuánto avanza dentro de ella, para conservar una posición de lectura: `list.ScrollToIndexAt(p.Index, 0, -p.Offset, false)` devuelve una lista reconstruida a ese punto. Una lista también puede convivir en una página con otro contenido: establece `Viewport: core.ListViewportParent`, y la vista desplazable que la rodea desplaza la lista junto con el resto.

## Tablas

`base/datatable` muestra una tabla de un millón de filas con filas y columnas virtualizadas, un encabezado fijo, columnas fijadas a la izquierda (`Fixed`) o a la derecha (`FixedRight`) mientras las demás se desplazan debajo de ellas, ordenamiento, selección de una o varias filas, redimensionamiento y reordenamiento de columnas, y celdas propias de la aplicación. `MultiSelect` admite alternancias con Command o Control, rangos con Shift y extensiones con teclado usando Shift, mientras que `SelectedRows` y `OnSelectedRowsChange` permiten que la aplicación controle las filas. `CellTooltip` da a una celda una descripción emergente (`tooltip`), que también describe la celda a los lectores de pantalla, estilizada por `Styles.CellTooltip`; lo que dibuja `RenderCell` no recibe eventos de puntero, por lo que un `ui.Tooltip` dentro de una celda nunca se muestra. `base.Table` es una tabla sencilla de las filas de la aplicación para datos más pequeños, y `base/plot` dibuja gráficos.

Usa `DataTable.LabelFunc` para un nombre de cuadrícula que siga la configuración regional o el estado de la aplicación. `ColumnGroup.LabelFunc` actualiza el encabezado de un grupo de columnas mientras el grupo permanece visible.

## Siguiente

- [Animación](animation.md): efectos que siguen una vista desplazable.
- [Texto](text.md): filas de texto que ajustan líneas.
- [Navegación](navigation.md): pantallas cuyo contenido se desplaza.

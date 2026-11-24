---
title: Lista
description: Filas de cualquier cantidad, construidas solo cuando están a la vista, con selección y activación.
group: Componentes
order: 38
---

`ui.List` renderiza filas en una vista desplazable nativa y construye solo las filas cercanas a la pantalla, con selección, activación y teclado.

```go preview=list/basic height=280
ui.List{
	Count:         func() int { return len(items) },
	Key:           func(i int) any { return items[i].ID },
	EstimatedSize: 44,
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		item := items[row.Index()]
		return ui.Text(item.Title, 13, ui.Regular, colors.Text)
	},
}
```

## Filas e identidad

`Count` indica cuántas filas hay y `Row` construye una. `Key` da a cada fila su identidad: las filas que permanecen conservan sus celdas y su estado a medida que otras filas aparecen y desaparecen por encima de ellas. Usa un ID en lugar del índice cuando las filas puedan añadirse, eliminarse o reordenarse; el índice es una señal que sigue a la celda cuando esta muestra otra fila.

```go
ui.List{Count: count, Key: func(i int) any { return items[i].ID }, Row: func(cx *core.Scope, row core.ListRow) core.Node {
	return ui.Text(items[row.Index()].Title, 13, ui.Regular, colors.Text)
}}
```

`EstimatedSize` es la altura esperada de una fila, 50 puntos cuando es 0. Las filas pueden tener distintas alturas; la lista mide las que construye y estima el resto.

## Disposición

`Header`, `Footer` y `Empty` contienen el contenido alrededor de las filas y cuando no hay ninguna. `Gap` separa las filas, `Columns` las coloca lado a lado y el relleno de la lista es el del contenido, para que la barra de desplazamiento permanezca en el borde. Dale una altura a la lista o deja que llene a su padre.

## Selección y activación

`ui.List` recibe una `Selection`, creada con `ui.NewSelection(cx, multiple)`. Las teclas, las etiquetas, la activación y el teclado la siguen: Arriba, Abajo, Inicio y Fin mueven el cursor, Shift extiende la selección, Mod+A selecciona todo, Espacio alterna en una selección múltiple e Intro o un doble clic activan una fila.

```go preview=list/selection height=320
selected := ui.NewSelection(cx, true)
ui.List{
	Count: count, Key: func(i int) any { return items[i].ID },
	Selection: selected,
	OnActivate: func(i int) { open(items[i]) },
	RowLabel:   func(i int) string { return items[i].Title },
	Row: func(cx *core.Scope, row core.ListRow) core.Node { return rowContent(cx, items[row.Index()]) },
}
```

`OnSelectionChange` se ejecuta después de que una pulsación, una tecla o la escritura anticipada cambien qué filas están seleccionadas, y no cuando la aplicación establece la `Selection` por sí misma, de modo que un panel de detalle puede seguir la elección del usuario mientras la aplicación mueve la selección a un elemento abierto de otra manera.

`TapSelects` hace que un toque en un teléfono seleccione en lugar de activar. `RowLabel` es lo que lee la tecnología de asistencia y lo que coincide con la escritura anticipada.

## Carga y desplazamiento

`OnLoadMore` se ejecuta al acercarse el final, una vez por número de filas. `Ref` desplaza con `ScrollToIndexAt`, `ScrollToEnd` y `ScrollToIndex`. `core.List`, la lista simple que hay debajo, también tiene `OnLoadMoreAtStart`, `Inverted` para un chat, `StickyHeaderIndices`, `MaintainVisibleContentPosition`, `Viewport` y `ViewabilityConfig` para los casos de bajo nivel ([Listas y desplazamiento](../lists.md)).

## Accesibilidad

La lista es una lista o un cuadro de lista (`listbox`), nombrado por `Label`, con el texto de una fila o `RowLabel` como nombre. Una lista seleccionada informa la fila del cursor y la anuncia conforme se mueve el teclado. Un lector de pantalla que se mueve fuera de la vista pide a la lista que se desplace hasta allí, lo que construye las filas que alcanza.

## Plataforma

La vista desplazable nativa y la virtualización son las mismas en todos los destinos. Las filas se reciclan, por lo que el estado de una fila debe seguir a su `Key`; mantener el estado de construcción en la aplicación en lugar de hacerlo dentro de una fila es la regla habitual.

## Relacionado

- [Listas y desplazamiento](../lists.md): la guía completa, incluidos los encabezados fijos y la carga de más elementos.
- [Desplazamiento](scroll.md): una vista desplazable simple.
- [DataTable](../lists.md#tablas): una tabla de muchas filas y columnas.
- Las páginas List, Table y DataTable de la galería.

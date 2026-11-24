---
title: Desplazamiento
description: La vista desplazable de la plataforma, con paginación, ajuste a puntos, zoom y actualización.
group: Componentes
order: 39
---

`core.Scroll` es la vista desplazable propia de la plataforma. Sus hijos se disponen en columna, o en fila con `Horizontal`, y llena su contenedor mientras el contenido se desplaza.

```go preview=scroll/basic height=240
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

El relleno va en el contenido, no en la vista desplazable, para que la barra de desplazamiento permanezca pegada al borde. La vista desplazable se encoge para ajustarse a su padre a menos que su estilo indique lo contrario. Establece `NoShrink` cuando deba conservar su tamaño solicitado mientras un hermano se encoge.

## Comportamiento

`Behavior` elige cómo se desplaza: `Paging`, `SnapToInterval` o `SnapToOffsets` con `SnapToAlignment`, una `DecelerationRate`, `NoBounces`, `ContentInset` y las reglas del teclado.

```go preview=scroll/paging height=240
core.Scroll{
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: pages,
}
```

`HideIndicator` oculta la barra de desplazamiento de la plataforma y `Bar` dibuja la propia de la aplicación. `MinZoomScale` y `MaxZoomScale` permiten hacer zoom con pellizco; `NoPinchZoom` los ignora. `CenterContent` centra el contenido más pequeño que el área visible.

## Actualización y eventos

`OnRefresh` hace que la vista se pueda actualizar; establece `Refreshing` mientras el trabajo se ejecuta. `PullValue` sigue cuánto se tira del contenido, para un indicador propio de la aplicación, y `Behavior.RefreshIndicator` establece su altura.

```go
refreshing := core.NewSignal(cx, false)
core.Scroll{
	Refreshing: refreshing.Get,
	OnRefresh:  func() { refreshing.Set(true); reload(cx) },
	Children:   content,
}
```

`OnScroll` informa el desplazamiento como máximo una vez por turno, o una vez cada `ScrollEventThrottle`; `Stale` marca uno anterior a que la plataforma aplicara un desplazamiento desde código. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd` y `OnScrollEnd` delimitan los gestos del usuario.

`OffsetValues` sigue el desplazamiento en el hilo principal, para un encabezado que se colapsa sin un commit por fotograma. `Ref` desplaza desde código con `ScrollTo`, `ScrollToEnd` y `ZoomTo`, y el ref de cualquier vista interior la trae a la vista con `Reveal`, desplazando todas las vistas desplazables que la rodean:

```go
row.Reveal(core.RevealOptions{Vertical: core.ScrollAlignCenter, Animated: true})
```

`RevealBox` trae en su lugar un cuadro dentro de la vista al área visible, como el cuadro de una palabra obtenido de `TextRangeBoxes` de un texto, y `Margin` deja espacio libre en los bordes de la vista desplazable más externa para una barra situada encima.

Una vista a la que el teclado mueve el foco se desplaza por sí misma hasta quedar a la vista.

## Disposición

`Horizontal` desplaza horizontalmente. `Behavior.ContentInset` añade espacio alrededor del contenido que se desplaza con él. Para una página que llena el área segura, usa `PaddingEnv` en el contenido, no en la vista desplazable.

## Accesibilidad

`Label` nombra el área de desplazamiento y `Role` indica qué es, como un cuadro de lista. Se conservan los gestos de desplazamiento de la plataforma, el desplazamiento con teclado y el desplazamiento con lector de pantalla; `Busy` informa una vista que sigue cargando.

## Plataforma

Cada backend utiliza la vista desplazable de la plataforma, por lo que la inercia, los rebotes, las barras de desplazamiento y el desplazamiento táctil son nativos. Los efectos de borde difieren: macOS y iOS tienen efecto elástico, Android se estira, Windows y Linux se detienen y la web sigue al navegador. `EdgeEffects` y `EdgeBar` añaden barras sobre el contenido donde la aplicación las quiera. En teléfonos y tabletas iOS y Android, las vistas desplazables verticales y horizontales que desbordan muestran un controlador de desplazamiento arrastrable al tacto a lo largo del eje activo que recorre todo el rango del contenido; las vistas desplazables cortas que caben sin desbordar, las vistas con zoom, `Behavior.NoScroll`, `HideIndicator` y las vistas que usan una `Bar` personalizada o `base.SetScrollbarTheme` lo ocultan. En las pruebas, `nsgotest.App.ScrollHandle` informa sus límites en el área visible y `DragAt` o `Touch` lo arrastran directamente. En Windows, el indicador superpuesto aparece para rueda, toque, arrastre de barra y su inercia, no para `ScrollTo`, `ScrollBy`, revelaciones, márgenes internos ni animaciones propios de la aplicación.

## Relacionado

- [Listas y desplazamiento](../lists.md): la guía completa sobre desplazamiento y listas.
- [Lista](list.md): filas que se reciclan.
- [Disposición](../layout.md): rellenos y el área segura.
- Las páginas Scroll y Lists de la galería.

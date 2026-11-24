---
title: Arrastrar y soltar
description: Transfiera valores de la aplicación entre vistas dentro de una misma ventana.
group: Conceptos básicos
order: 57
---

`base/dragdrop` conecta un origen y un destino en la misma `Area`, dentro de una ventana. La aplicación conserva sus vistas, estilos, selección y la acción que gestiona el valor. Un origen se inicia tras cuatro puntos de movimiento de forma predeterminada, dejando intacta una pulsación ordinaria.

```go
import "github.com/nsgo-dev/nsgo/base/dragdrop"

dragdrop.Area{Children: []core.Node{
	dragdrop.Source{
		View: core.View{
			Label: "Film", TestID: "poster",
			Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Poster"}},
		},
		Data: func() any { return filmID },
		Preview: func(any) core.Node { return core.Text{Content: "Film"} },
	},
	dragdrop.Target{
		View: core.View{
			TestID: "playlist",
			Style: core.Style{Width: core.Pt(160), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Playlist"}},
		},
		Accept: func(value any) bool { _, ok := value.(string); return ok },
		OnDrop: func(e dragdrop.Event) { addToPlaylist(e.Data.(string)) },
	},
}}
```

`Data` se ejecuta una vez cuando comienza el arrastre. Devuelva un slice para llevar varios elementos seleccionados juntos; copie allí los datos mutables si deben permanecer como una instantánea. Un `Data` nil desactiva el arrastre. `Threshold` define el movimiento necesario. `Activation: dragdrop.AfterLongPress` espera primero a `View.DelayLongPress` (500 ms de forma predeterminada) y luego al movimiento; elíjalo explícitamente cuando un deslizamiento deba desplazar primero.

`View` conserva sus propiedades habituales, incluidos los controladores de pulsación y la accesibilidad. Un origen posee `View.Responder`; déjelo sin asignar. Sus hijos pueden incluir un botón con su acción normal. `Preview` sigue al puntero sin recibir entrada ni volver a montar el origen; sin ella, se muestra la etiqueta del origen. Dé al origen una etiqueta o una vista previa. Las áreas anidadas funcionan de forma independiente.

Los destinos necesitan `OnDrop` para aceptar un valor. El destino más interno tiene la primera oportunidad; cuando `Accept` lo rechaza, un ancestro que lo acepte puede tomarlo. `OnEnter`, `OnMove` y `OnLeave` pueden actualizar el aspecto del destino. Cada evento lleva `Data`, `Position` en el destino y `PagePosition` en la superficie. Desactivar o eliminar el destino activo limpia su estado de entrada; soltar vuelve a comprobar la aceptación. Un destino detrás de otra vista, recortado, desactivado o debajo de un modal no puede recibir la caída.

`OnStart` y `OnEnd` describen la sesión del origen. `OnEnd` recibe `Result{Data, Dropped, Target}`. Escape, la cancelación táctil nativa, desactivar o eliminar el origen, o soltar fuera de un destino que acepte terminan sin soltar el elemento. La ubicación del destino usa la última geometría confirmada; las animaciones nativas entre commits no se muestrean. Esto transfiere valores dentro de la ventana; los archivos del sistema siguen usando `OnFileDrop`.

Mantenga un menú, comando de teclado o acción accesible que realice la misma operación. Arrastrar es un atajo hacia esa acción. Las pruebas usan `app.DragTo("id:poster", "id:playlist")`, o `DragTo` de una superficie seleccionada; las pruebas TSX usan `await app.dragTo("id:poster", "id:playlist")`. Ambas vistas ya deben caber en la ventana visible; el paso envía una secuencia táctil a través del respondedor sin sintetizar una caída ni desplazar durante el arrastre. Consulte [Pruebas](testing.md).

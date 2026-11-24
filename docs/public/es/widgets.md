---
title: Widgets
description: Muestra las vistas de la aplicación en la pantalla de inicio y en la pantalla de bloqueo del iPhone y el iPad, y en el escritorio del Mac.
group: Funciones de plataforma
order: 47
---

## Un tipo de widget

Los widgets muestran un vistazo rápido de la aplicación esté o no en ejecución: en la pantalla de inicio y la pantalla de bloqueo del iPhone y del iPad, en el escritorio del Mac y en el Centro de notificaciones, y en el lanzador de aplicaciones de Android. Un tipo (`kind`) es un widget que ofrece la galería del sistema, con las familias en las que está disponible:

```yaml
widgets:
  - kind: counter
    name: Counter                 # shown in the system's gallery of widgets
    description: Shows the count.
    families: [small, medium, circular]  # also large, extra_large, rectangular and inline
# apple: { app_group: group.dev.example.hello }  # the shared App Group, group.<id> by default
```

En las plataformas de Apple, una extensión de widget lo dibuja, y `build` y `dev` la compilan con el compilador Swift de Xcode ([Compilación y distribución](building.md#widgets-y-live-activities)). En Android, el entorno de ejecución Java precompilado dibuja la misma línea temporal como App Widgets. Ambos siguen mostrando el contenido publicado cuando la aplicación se detiene.

Para una compilación en zona aislada del Mac App Store, configura `apple.app_group` y proporciona los perfiles de aprovisionamiento de macOS mediante `sign.macos_profile` y `sign.widgets_profile`. Tanto la aplicación como `<id>.widgets` deben pertenecer al mismo equipo y tener derechos (`entitlements`) sobre ese grupo. Las líneas temporales y las imágenes se guardan en su contenedor compartido; las compilaciones directas y de desarrollo para macOS conservan su ubicación existente en el directorio personal. [Firma para tiendas](building.md#paquetes-de-tienda) describe las opciones de compilación.

`name` y `description` pueden referenciar mensajes de los catálogos de la aplicación. Cuando lo hacen, la galería de widgets y el marcador de posición usan el idioma del usuario, recurriendo al texto del catálogo de origen si falta una traducción. Un valor que no sea una clave de catálogo se muestra literalmente.

## Una línea temporal

`widget.Update` escribe lo que muestran los widgets de un tipo a partir de este momento, como una línea temporal de entradas: el sistema muestra la entrada cuya fecha haya pasado más recientemente y luego cada una posterior en su fecha, sin la aplicación. Un `Date` cero equivale al momento actual. Sin entradas, los widgets muestran el nombre y la descripción del tipo. En las plataformas de Apple, una aplicación en primer plano actualiza sus widgets tantas veces como quiera, y una en segundo plano unas pocas decenas de veces al día.

```go
err := widget.Update("counter", widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Count", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Text{Content: strconv.Itoa(n), Style: widget.Style{FontSize: 44, FontWeight: core.FontWeightBold}},
	}},
})
```

`Entry.Families` muestra a una familia una vista propia en lugar de `Entry.View`, `Entry.Background` rellena el widget por detrás, más allá de los márgenes que mantiene el sistema, y `Entry.URL` se abre con la aplicación al tocar el widget. `widget.Update` devuelve `widget.ErrNoWidgets` en una aplicación compilada sin ellos y `widget.ErrUnsupported` fuera de macOS, iOS y Android, mientras que `widget.Available` indica cuál aplica antes de escribir. Puede ejecutarse en cualquier goroutine, también antes de `nsgo.Run`.

## Vistas

Las vistas de un widget no son elementos de `core`: se disponen como en SwiftUI, con una vista tan grande como su contenido salvo que `Style` le dé tamaño o la haga rellenar el espacio, y un contenedor que centra lo que contiene salvo que `AlignItems` y `Justify` indiquen lo contrario. `Text`, `Symbol` e `Image` (a partir de bytes o de un archivo que lee `Update`) muestran contenido; `Column`, `Row`, `Stack`, `Spacer` y `Divider` lo organizan; `Rectangle`, `Progress` y `Gauge` dibujan; y `Date` muestra una hora con los formatos del sistema, que `Relative`, `Offset` y `Timer` siguen contando por sí mismos, sin necesidad de una entrada para cada segundo.

```go
widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Next hour in", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Date{Time: hour, Format: widget.Timer, Style: widget.Style{TextStyle: widget.Title2, FontVariant: core.FontTabularNums}},
		widget.Progress{Value: 0.4, Style: widget.Style{Color: widget.Accent}},
	}},
}
```

`Style` acepta separación (`gap`), relleno (`padding`), tamaño o relleno de espacio, alineación, un fondo con esquinas redondeadas, los estilos de texto del sistema o un tamaño, grosor, diseño y variante de fuente, límites de líneas, un `Link` que se abre con la aplicación, y `Accented`, que incluye una vista entre aquellas que tiñe el sistema. `widget.Primary`, `Secondary` y `Accent` siguen la apariencia, y `RGB` y `RGBA` aceptan otro color para el modo oscuro con `Dark`.

## Botones

`widget.Button` ejecuta `widget.OnAction` desde iOS 17 y macOS 14, sin abrir la aplicación. Los sistemas anteriores muestran únicamente sus hijos. Registra el manejador en `main`, antes de `nsgo.Run`: el sistema puede iniciar la aplicación en segundo plano sin sus vistas. Una pulsación en un widget tiene `Activity == nil`; su `Action` es el valor del botón.

```go
widget.Button{Action: "refresh", Children: []widget.View{
    widget.Text{Content: "Refresh"},
}}
```

Gestiona la acción y llama a `widget.Update` antes de que el manejador retorne, de modo que la recarga automática de la línea temporal del sistema vea el resultado. El manejador se ejecuta fuera de la goroutine de las vistas; usa `core.Scope.Post` para cambiar vistas. Esto utiliza el mismo manejador e intent que [Actividades en vivo](live-activities.md), y solo necesita `widgets` en `nsgo.yaml`, sin `live_activities`.

## Android

Añade el widget desde el selector de widgets del lanzador y ajústale el tamaño allí. El renderizador selecciona una familia permitida a partir de sus dimensiones: `small` por debajo de 240 puntos en ambas dimensiones, `medium` a partir de 240 de ancho, `large` a partir de 240 de alto y `extra_large` a partir de 500 de ancho; si esa familia no está en la lista, usa la primera familia listada. `Entry.URL` y `Style.Link` abren la aplicación a través de un esquema que esta declare en `url_schemes`. Un enlace en una vista anidada tiene prioridad sobre el enlace de su contenedor.

Las entradas futuras de la línea temporal usan las alarmas inexactas de Android, que el sistema puede retrasar mientras la aplicación está en segundo plano. Las vistas de temporizador usan cronómetros nativos y avanzan sin la aplicación; actualmente respetan el tamaño y el color, mientras que el grosor de fuente personalizado, el diseño y el estilo cursivo siguen siendo objetivos pendientes. Las fechas relativas se actualizan según la programación del sistema. El renderizador genérico admite texto, imágenes, disposición, formas, barras, anillos y un conjunto de glifos de símbolos comunes; usa `Image` para recursos gráficos que deban coincidir en ambas plataformas. Los nombres arbitrarios de SF Symbols siguen siendo un objetivo abierto en Android. Los proveedores de widgets de Android se generan para APK y AAB independientes; los hosts incrustados no reciben ninguno.

## Actividades en vivo

La misma extensión dibuja las Actividades en vivo de la aplicación en la pantalla de bloqueo y en la Dynamic Island ([Actividades en vivo](live-activities.md)).

## Siguiente

- [Actividades en vivo](live-activities.md): lo que sigue la aplicación mientras sucede.
- [Notificaciones](notifications.md): las notificaciones e insignias junto a un widget.
- [Compilación y distribución](building.md): el grupo de aplicaciones que comparte la extensión.

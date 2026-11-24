---
title: Imágenes e iconos
description: Muestra una imagen desde una URL, los recursos de la aplicación, bytes o píxeles que dibuja la aplicación, y dibuja iconos de Lucide y SVG.
group: Conceptos básicos
order: 54
---

## Una imagen

`core.Image` muestra un mapa de bits o un SVG: una URL, un archivo, una ruta `asset:` en los recursos de la aplicación o bytes que mantiene la aplicación. La plataforma la carga, decodifica y almacena en caché fuera del hilo principal, y el hilo principal dibuja el resultado.

```go
core.Image{
	Source: core.Src("https://example.com/cover.jpg"),
	ResizeMode: core.Cover,
	Style: core.Style{Width: core.Pt(160), Height: core.Pt(100), CornerRadius: 12},
}
```

`core.Cover` rellena el cuadro y recorta lo que sobresale; `core.Contain` ajusta toda la imagen dentro, `core.Center` mantiene su tamaño natural y `core.ScaleDown` lo mantiene salvo que sea más grande que el cuadro. `BlurRadius` difumina la imagen en puntos, `TintColor` vuelve a colorear sus píxeles opacos y `ObjectPosition` la ubica dentro del cuadro.

Los mapas de bits escalados usan muestreo suave de forma predeterminada. Establece `Sampling: core.ImageSamplingNearest` en Go o `sampling="nearest"` en TSX para conservar los bordes nítidos del pixel art y de los búferes de píxeles. Las plataformas de Apple, Windows, Linux, Android y la web admiten esta configuración; HarmonyOS y los miniprogramas también la aplican, aunque su salida aún no se ha verificado en dispositivos. Para lograr bordes de píxel exactos en un navegador usando `pixelated` de CSS, escala a un número entero de píxeles de dispositivo por píxel de origen.

`Source` declara el tamaño que usa la disposición: una dimensión deriva la otra, y una imagen sin tamaño mide 0 por 0 hasta que su estilo le dé dimensiones. `core.ImageSource{Data: data, Size: core.Size{Width: 24, Height: 24}}` muestra bytes incrustados con `go:embed`, y `Size` da a un SVG su tamaño intrínseco.

Con [`desktop.WindowPointerPainted`](services.md), `HitTestAlpha: 0.5` deja pasar el puntero a través de píxeles con menos de la mitad de opacidad en macOS y Windows. TSX define el mismo umbral con `hitTestAlpha`.

## Cuadros que dibuja la aplicación

`Pixels` muestra un `*image.RGBA` que dibuja la aplicación, los cuadros de un reproductor de video o de un emulador, sin codificar, cargar ni decodificar nada. Escribe cada cuadro en el mismo búfer e incrementa `Version`: la vista lo muestra sin eventos de carga ni fundido, conservando su canal alfa, y en macOS, iOS, tvOS, watchOS, Android, Windows y Linux lo intercambia en el sitio sin un cuadro en blanco intermedio. Sin `Size`, mide sus píxeles en puntos.

```go
frame := image.NewRGBA(image.Rect(0, 0, 960, 540))
version := core.NewSignal(cx, uint64(0))
var tick func(time.Time)
tick = func(at time.Time) {
	if decoder.WriteDue(at, frame) { // writes the next frame into frame.Pix
		version.Set(version.Get() + 1)
	}
	cx.RequestAnimationFrame(tick)
}
cx.RequestAnimationFrame(tick)

core.Image{ResizeMode: core.Contain, SourceFunc: func() core.ImageSource {
	return core.ImageSource{Pixels: frame, Version: version.Get()}
}}
```

NSGo copia los píxeles cuando se ejecuta el commit que lleva una nueva `Version`, de modo que la aplicación puede escribir el siguiente cuadro desde su siguiente callback de `RequestAnimationFrame`; un decodificador en una goroutine propia escribe en un segundo búfer y solicita el intercambio. La misma `Version` no se vuelve a dibujar. `cx.ImageSize` responde de inmediato a una fuente de píxeles con el tamaño de su búfer. Un `core.Canvas` dibuja píxeles con `DrawImage`, copiados del mismo modo, en todas las plataformas; los miniprogramas requieren un lienzo 2D de WeChat. Los `Images` de un `core.Shader` aceptan píxeles de la misma manera, de modo que un shader puede filtrar un cuadro que escribe la aplicación, como la pantalla de un emulador, en toda plataforma que dibuja shaders.

Las plataformas de Apple, Android, Windows, Linux y la web actualizan las imágenes de píxeles directamente, y los miniprogramas de WeChat usan un lienzo 2D. HarmonyOS crea un `PixelMap` de cada versión, verificado en un dispositivo, y sus copias cruzan un puente de cadenas, por lo que los búferes grandes reescritos en cada cuadro funcionan muy por debajo de 60 fps allí ([búferes de píxeles](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/images.md#pixel-buffers)).

## Capturar una vista

Llama a `cx.Capture(&ref, done)` con el `core.Ref` de una vista montada, o a `cx.Capture(nil, done)` para la superficie actual. El callback recibe bytes PNG a la escala de píxeles de la superficie o un error en la goroutine del motor. Usa `core.ImageSource{Data: png}` para mostrar el resultado. Para compartirlo, escribe los bytes en un archivo `.png` en el directorio de caché de la aplicación y pasa su ruta en `core.ShareOptions.Files`. [Compartir archivos](services.md#abrir-enlaces-y-archivos)

La captura toma los píxeles visibles de la superficie dentro de los límites de la vista. Está disponible en macOS, iOS, tvOS, Windows, Linux, Android y la web. Android captura la ventana de la actividad. La captura web pinta el DOM de la aplicación en un lienzo, incluida la salida de shaders de NSGo, y omite marcos incrustados, imágenes que no puede leer y algunos efectos de CSS.

## Recursos y escalas

Los archivos bajo `resources/` se empaquetan con la aplicación. `core.Src("asset:images/cover.png")` carga uno, y `Scales: core.ScalesOf(1, 2, 3)` elige `cover.png`, `cover@2x.png` o `cover@3x.png` según la relación de píxeles de la pantalla.

```go
core.Image{Source: core.ImageSource{
	URI:    "asset:images/cover.png",
	Scales: core.ScalesOf(1, 2, 3),
}}
```

`Placeholder` muestra una fuente mientras se carga la imagen, y `FadeDuration` hace aparecer la imagen cargada sobre ella con un fundido; 0 deja elegir a la plataforma y un nanosegundo la muestra de inmediato. `OnLoadStart`, `OnProgress`, `OnLoad`, `OnError` y `OnLoadEnd` siguen la carga. `cx.ImageSize` lee el tamaño de una fuente en píxeles sin mostrarla, `cx.PrefetchImage` descarga una en la caché, `cx.SetImageCacheLimit` establece el presupuesto en memoria para imágenes codificadas en bytes (por defecto `core.DefaultImageCacheLimit`, 32 MiB; los valores negativos se limitan a 0), y `cx.KeepImage` fija un atlas o textura fuera de la expulsión de caché hasta que se ejecuta el `release` devuelto o se destruye su ámbito (`Scope`). El límite se aplica a cada caché, excluyendo elementos fijados, cargas en curso y la pasada de dibujo actual; no limita los píxeles decodificados, la memoria del proceso ni la caché del navegador. Cero desactiva la retención de elementos no fijados. Los backends nativos conservan las imágenes remotas en su propia caché en disco.

## Imágenes animadas y vectoriales

Las animaciones GIF y WebP se reproducen en la vista, cada cuadro durante su propio retardo, y se repiten según indique el archivo. Los documentos SVG se cargan como imágenes y se mantienen nítidos a cualquier tamaño; un SVG de un solo color puede teñirse con `TintColor`.

## Iconos

`core.Icon` dibuja un SVG en el color de texto que hereda, tan alto como el tamaño de fuente a menos que su estilo defina su tamaño. Su cuadro conserva la relación de aspecto del SVG, de modo que un cuadro cuadrado no lo deforma.

```go
core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 20}, Label: "Search"}
```

`icons/lucide` tiene un descriptor por cada icono de Lucide, nombrado en PascalCase (`lucide.ArrowUpRight`). Un icono es decorativo para la tecnología de asistencia a menos que `Label` lo nombre. `ui.Icon` es igual con los valores predeterminados de `ui`, y `core.SVG` y `core.SVGView` dibujan trazados que la propia aplicación construye.

## La imagen con estilo

`ui.Image` es una imagen en un contenedor redondeado, con un marcador de posición mientras se carga y un icono tenue de imagen cuando falla. Mantiene la relación de aspecto de la fuente, o 3:2 si no tiene una, a menos que su estilo fije ambos lados.

```go
ui.Image{
	Source:      core.Src("asset:images/cover.png"),
	Label:       "Cover",
	ResizeMode:  core.Cover,
	AspectRatio: 16.0 / 9.0,
	Placeholder: ui.Skeleton{Style: core.Style{Height: core.Pt(180)}},
}
```

## Siguiente

- [Texto](text.md): imágenes dentro de una línea, como fragmentos (`spans`).
- [Estilos y temas](styles.md): esquinas, sombras y tintes.
- [Listas y desplazamiento](lists.md): filas de imágenes que se reciclan.

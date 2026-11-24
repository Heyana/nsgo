---
title: Lienzo y shaders
description: Dibuja gráficos 2D en el motor y rellena una vista con un shader de fragmentos en la GPU.
group: Conceptos básicos
order: 59
---

## Un lienzo

`core.Canvas` graba dibujo 2D en el motor y hace que la plataforma lo reproduzca, igual que el `<canvas>` de una página web. `Draw` vuelve a ejecutarse cuando cambia una señal que leyó, el tamaño del lienzo o las fuentes disponibles, y la plataforma dibuja la grabación en el hilo principal. Una fuente cargada mediante `cx.RegisterFont` vuelve a ejecutar `Draw` en cuanto está disponible, actualizando tanto el texto como la geometría basada en `TextMetrics`.

Usa `Em` o `EmFunc` para un lienzo dimensionado en unidades `em` de su fuente heredada. Para contenido cuya altura depende del ancho, `MeasureEm(width, fontSize)` devuelve un `EmBox` para el ancho disponible en puntos (`core.Auto` cuando no está acotado) y el tamaño de fuente resuelto. Sus lecturas se rastrean y también se ejecuta durante la disposición, así que mantenlo libre de escrituras en señales. `Ref.ContentBox()` rastrea el cuadro de disposición de una vista montada dentro del relleno y los bordes, en sus propias coordenadas.

```go
core.Canvas{
	Style: core.Style{Height: core.Pt(220)},
	Draw: func(d *core.Drawing, size core.Size) {
		d.SetFillColor(core.RGB(0xf1f5f9)).FillRect(0, 0, float32(size.Width), float32(size.Height))
		d.SetStrokeColor(core.RGB(0x007aff)).SetLineWidth(2).SetLineCap(core.CapRound)
		d.BeginPath().MoveTo(24, 160).LineTo(90, 80).LineTo(160, 120).LineTo(230, 40).Stroke()
	},
}
```

La grabación sigue el modelo `CanvasRenderingContext2D` de la web: `Save` y `Restore`, transformaciones, trazados de líneas, curvas, rectángulos y arcos, rellenos y trazos con colores y degradados, recorte, guiones, sombras, texto e imágenes. `core.ParsePath` crea un `core.Path` a partir de datos de trazado SVG, y `FillFromView` y `StrokeFromView` toman los estilos `Fill` y `Stroke` heredados de la vista de lienzo, de modo que un gráfico siga el tema de la aplicación.

En la web, los lienzos dentro de un `core.Scroll` ampliado o de un `Style.Transform` / `AnimatedStyle` escalado se reproducen a la escala acumulada una vez que el movimiento se detiene, hasta cuatro veces la relación de píxeles de la pantalla. Un lienzo cuyo mapa de bits ampliado superaría los 4096 × 4096 píxeles dibuja solo su parte visible y se actualiza al desplazarse o hacer panorámica, manteniendo nítidos los trazos y glifos sin volver a ejecutar `Draw`. Apple, Android, Windows y Linux mantienen igualmente nítidos los lienzos y textos ampliados tanto con zoom de desplazamiento como con transformaciones de estilo.

```go
d.SetFillGradient(core.CanvasLinear(0, 0, float32(size.Width), 0,
	core.GradientStop{Offset: 0, Color: core.RGB(0x4da3ff)},
	core.GradientStop{Offset: 1, Color: core.RGB(0x007aff)},
)).FillRect(0, 0, float32(size.Width), 40)
```

## Texto, imágenes y partes

`MeasureText`, `TextMetrics` y `SetTextAlign` maquetan el texto en el motor de texto de la plataforma, de modo que las etiquetas usan las mismas fuentes que el resto de la aplicación. `DrawImage`, `DrawImageIn` y `DrawImageFrom` dibujan imágenes; `DrawSVG` y `DrawSVGIn` dibujan documentos `*core.SVG` analizados (`core.ParseSVG`) en su tamaño intrínseco o ajustados dentro de un rectángulo —con elementos `<text>` y `<tspan>` medidos y renderizados por el propio motor de texto de la plataforma y `currentColor` tomado del color de relleno del dibujo—; y `OnPress` puede comprobar el último dibujo con `IsPointInPath` e `IsPointInStroke`. Los backends nativos almacenan en caché las 64 fuentes de imagen de lienzo no retenidas más recientemente dibujadas; llama a `release := cx.KeepImage(src)` para fijar atlas de sprites o texturas de larga duración fuera de ese límite de expulsión hasta que se ejecute `release()` o se destruya `cx`.

```go
badge := core.MustParseSVG([]byte(`<svg viewBox="0 0 120 40">
	<rect width="120" height="40" rx="8" fill="#007aff"/>
	<text x="60" y="25" text-anchor="middle" font-size="14" fill="#fff">Live</text>
</svg>`))

d.DrawSVGIn(badge, 12, 12, 180, 60)
```

Los mapas de bits escalados usan muestreo suave de forma predeterminada. Para pixel art, llama a `d.SetImageSmoothing(false)` antes de `DrawImage`, `DrawImageFrom` o `DrawAtlas`, o establece `ctx.imageSmoothingEnabled = false` en TSX. `Save` y `Restore` conservan el ajuste junto con el resto del estado de dibujo. El muestreo por vecino más cercano está disponible en plataformas Apple, Windows, Linux, Android y la web; HarmonyOS y los miniprogramas también lo aplican, aunque su salida aún no se ha comprobado en dispositivos.

`d.DrawVertices(image, vertices)` dibuja una malla con textura. Cada tres valores `core.CanvasVertex` consecutivos forman un triángulo. Define `Position` en puntos del lienzo y `TexCoord` en puntos de la imagen, como en `DrawImageFrom`. El `Color` de cada vértice multiplica la textura y se interpola sobre el triángulo. Su valor cero deja la textura sin cambios. Actualiza los puntos y vuelve a grabar el dibujo para deformar la malla. La imagen puede ser cualquier `ImageSource`, incluido un búfer de píxeles o el `Source()` de una `OffscreenImage`; los lienzos de Android y los miniprogramas aún no dibujan imágenes fuera de pantalla. `SetImageSmoothing(false)` también se aplica a las mallas. La web requiere imágenes en línea o del mismo origen para mallas arbitrarias. Los miniprogramas dibujan mallas con el reproductor de la web, lo que aún no se ha comprobado en WeChat DevTools ni en un dispositivo. HarmonyOS recorta cada triángulo y dibuja la imagen mediante su transformación afín; al igual que con `DrawImage`, dibuja imágenes codificadas y búferes de píxeles pero no imágenes fuera de pantalla, y aplica el alfa medio de los vértices de cada triángulo pero sin tinte de color. Oculta únicamente las costuras entre triángulos opacos y aún no se ha comprobado en un dispositivo.

`DrawTintedImage`, `DrawTintedImageIn` y `DrawTintedImageFrom` multiplican los canales sRGB y alfa de una imagen por un color antes de componerla. Con el modo predeterminado source-over, los píxeles transparentes dejan visible el fondo. Usa `drawTintedImage(image, tint, ...args)` en TSX con las mismas formas de coordenadas que `drawImage`. Un origen cruzado en la web debe permitir solicitudes CORS anónimas para poder teñir sus píxeles. HarmonyOS tiñe imágenes procedentes de bytes, recursos de la aplicación y búferes de píxeles, aunque aún no se ha comprobado en un dispositivo; allí todavía no se dibujan imágenes fuera de pantalla.

Usa `d.DrawAtlas(source, rects, xforms, alphas)` para dibujar muchos sprites de una sola imagen en orden. Cada `core.Bounds` selecciona un mosaico en los puntos propios de la imagen. El `core.RSXform` correspondiente asigna la esquina superior izquierda del mosaico a `TX, TY`, con `SCos` y `SSin` iguales a la escala por el coseno y el seno de la rotación. Por ejemplo, `core.RSXform{SCos: 0.5, TX: 40, TY: 20}` dibuja un mosaico de 32 puntos a mitad de tamaño en (40, 20). Pasa `nil` para una opacidad completa de los sprites, o proporciona factores que multipliquen el alfa actual del dibujo.

`Drawing.Node` nombra una parte de un dibujo, como la barra de un gráfico o la región de un mapa, con límites, un rol, una etiqueta y un estado seleccionado. La tecnología de asistencia lee esas partes como hijos del lienzo, y las pruebas las encuentran por etiqueta, texto o ID de prueba; una pulsación sobre una de ellas equivale a pulsar en su centro.

```go
d.Node(core.CanvasNode{
	Bounds: core.Bounds{Origin: core.Point{X: x, Y: y}, Size: core.Size{Width: w, Height: h}},
	Role:   core.RoleButton,
	Label:  "Open the report",
	TestID: "report",
})
```

## Imágenes fuera de pantalla

`cx.NewOffscreenImage(size, scale, draw)` renderiza un `core.Drawing` una sola vez en una imagen fuera de pantalla retenida que los fotogramas posteriores dibujan con `DrawImage`, `DrawImageIn`, `DrawImageFrom`, `DrawAtlas` o `core.Image`. Pasar `scale <= 0` usa la relación de píxeles de pantalla actual. El dibujo solo vuelve a renderizarse cuando se llama a `Invalidate()` o `Resize(size, scale)` (o cuando se registran nuevas fuentes), y se libera automáticamente cuando se destruye `cx` o se llama a `Release()`.

```go
tiles := cx.NewOffscreenImage(core.Size{Width: 512, Height: 512}, 0, func(d *core.Drawing) {
	// Drawn once into the offscreen image until tiles.Invalidate() is called.
	for y := range 16 {
		for x := range 16 {
			d.SetFillColor(tileColor(x, y)).FillRect(float32(x*32), float32(y*32), 32, 32)
		}
	}
})

core.Canvas{
	Style: core.Style{Width: core.Pt(320), Height: core.Pt(240)},
	Draw: func(d *core.Drawing, _ core.Size) {
		// Reuses the cached surface in a single DrawImage operation per frame.
		d.DrawImage(tiles.Source(), -cameraX.Get(), -cameraY.Get())
	},
}
```

## Argumentos animados

`Drawing.Bind` vincula un argumento de la última operación a un nodo animado, y la plataforma vuelve a dibujar únicamente esa parte con su reloj de fotogramas, sin confirmación. `core.ArgX`, `ArgY`, `ArgR`, `ArgA` y `ArgColor` nombran los argumentos; macOS, iOS y Linux redibujan solo el área modificada, mientras que Windows vuelve a grabarla y Android y la web redibujan todo el lienzo.

```go
needle := core.NewAnimatedValue(cx, 0)
d.Save().Translate(cx, cy).Rotate(0).Bind(core.ArgA, needle).
	BeginPath().MoveTo(-14, -3).LineTo(80, 0).LineTo(-14, 3).ClosePath().Fill().Restore()
```

## Shaders

`core.Shader` rellena su caja con un shader de fragmentos que la GPU ejecuta para cada píxel. `core.CompileShader` toma un programa escrito con la sintaxis de Go que tenga una función `Fragment`, y `core.CompileWGSL` toma WGSL. `Uniforms` define las variables exportadas del programa e `Images` le proporciona hasta cuatro texturas.

```go
var program = core.MustCompileShader(`
package main

var Tint vec4

func Fragment(dst vec4, pos vec2) vec4 {
	v := 0.5 + 0.5*sin(pos.x/40)
	return Tint * vec4(v, v, v, 1)
}
`)

core.Shader{
	Program:  program,
	Uniforms: core.Uniforms{"Tint": core.RGB(0x007aff)},
	Style:    core.Style{Height: core.Pt(220), CornerRadius: 16},
}
```

Un shader de Go recibe el centro del píxel en `dst` y el mismo punto en puntos en `pos`, y dispone de las funciones integradas de GLSL; `rectSize`, `pixelScale`, `image0At` a `image3At` y sus tamaños son añadidos de NSGo. Un programa WGSL define `fn shade(uv: vec2f) -> vec4f` y lee `nsh.time` y `nsh.size`; `Paused` detiene su reloj. Una plataforma sin ruta de GPU para shaders no dibuja nada ahí.

`core.ShaderEffect` dibuja sus hijos a través de WGSL y `core.ScreenEffect` cubre la superficie. `core.ShaderTransition` y `core.ScreenTransition` muestrean las imágenes antes y después de que cambie `Value`. `core.TransitionShader` hace esto mismo para la navegación y los cuadros de diálogo. En Android, capturar vistas para efectos requiere Android 10 o posterior y OpenGL ES 3.0 con soporte para texturas externas. Tanto `core.Canvas` como `core.Shader` aceptan `Label` y sus hijos se dibujan encima de ellos.

## Siguiente

- [Imágenes e iconos](images.md): las imágenes que dibuja un lienzo.
- [Animación](animation.md): valores que vincula un lienzo.
- [Accesibilidad](accessibility.md): las partes que expone un lienzo.

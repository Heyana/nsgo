---
title: Canvas und Shader
description: "2D-Grafiken in der Engine zeichnen und eine View mit einem GPU-Fragment-Shader füllen."
group: Kernkonzepte
order: 59
---

## Ein Canvas

`core.Canvas` zeichnet 2D-Grafiken in der Engine auf und lässt die Plattform sie abspielen – genau wie `<canvas>` auf einer Webseite. `Draw` läuft erneut, sobald sich ein darin gelesenes Signal, die Größe des Canvas oder die verfügbaren Schriftarten ändern, und die Plattform zeichnet die Aufzeichnung auf dem Hauptthread. Eine über `cx.RegisterFont` geladene Schriftart führt `Draw` erneut aus, sobald sie bereitsteht, und aktualisiert sowohl Text als auch Geometrie auf Basis von `TextMetrics`.

Verwenden Sie `Em` oder `EmFunc` für einen Canvas, dessen Größe in `em` seiner geerbten Schriftart angegeben wird. Für Inhalte, deren Höhe von der Breite abhängt, liefert `MeasureEm(width, fontSize)` eine `EmBox` bei der verfügbaren Breite in Punkten (`core.Auto` bei unbegrenzter Breite) und der aufgelösten Schriftgröße. Seine Lesezugriffe werden verfolgt, und es läuft auch während des Layouts – schreiben Sie darin daher keine Signals. `Ref.ContentBox()` verfolgt den Layout-Bereich einer eingehängten View innerhalb von Innenabstand und Rahmen in ihren eigenen Koordinaten.

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

Die Aufzeichnung orientiert sich am `CanvasRenderingContext2D` des Webs: `Save` und `Restore`, Transformationen, Pfade aus Linien, Kurven, Rechtecken und Bögen, Füllungen und Konturen mit Farben und Farbverläufen, Clipping, Strichelungen, Schatten, Text und Bilder. `core.ParsePath` erstellt aus SVG-Pfaddaten einen `core.Path`, und `FillFromView` sowie `StrokeFromView` übernehmen die geerbten Styles `Fill` und `Stroke` der Canvas-View, sodass ein Diagramm dem App-Theme folgt.

Im Web werden Canvases innerhalb eines gezoomten `core.Scroll` oder eines skalierten `Style.Transform` / `AnimatedStyle` einmalig nach dem Stillstand mit der kumulierten Skalierung neu gerastert – bis zum Vierfachen des Pixelverhältnisses des Bildschirms. Ein Canvas, dessen gezoomtes Bitmap 4096 × 4096 Pixel überschreiten würde, zeichnet nur seinen sichtbaren Ausschnitt und aktualisiert sich beim Scrollen oder Verschieben, wodurch Linien und Glyphen scharf bleiben, ohne `Draw` erneut auszuführen. Apple, Android, Windows und Linux halten gezoomte Canvases und Text unter Scroll-Zoom wie auch Style-Transformationen ebenfalls scharf.

```go
d.SetFillGradient(core.CanvasLinear(0, 0, float32(size.Width), 0,
	core.GradientStop{Offset: 0, Color: core.RGB(0x4da3ff)},
	core.GradientStop{Offset: 1, Color: core.RGB(0x007aff)},
)).FillRect(0, 0, float32(size.Width), 40)
```

## Text, Bilder und Teile

`MeasureText`, `TextMetrics` und `SetTextAlign` setzen Text über die Text-Engine der Plattform, sodass Beschriftungen dieselben Schriftarten verwenden wie der Rest der App. `DrawImage`, `DrawImageIn` und `DrawImageFrom` zeichnen Bilder, `DrawSVG` und `DrawSVGIn` zeichnen geparste `*core.SVG`-Dokumente (`core.ParseSVG`) in ihrer intrinsischen Größe oder eingepasst in ein Rechteck – wobei `<text>`- und `<tspan>`-Elemente von der eigenen Text-Engine der Plattform vermessen und gerendert werden und `currentColor` aus der Füllfarbe der Zeichnung stammt –, und `OnPress` kann die letzte Zeichnung mit `IsPointInPath` und `IsPointInStroke` prüfen. Native Backends cachen die 64 zuletzt gezeichneten, nicht angepinnten Canvas-Bildquellen; rufen Sie `release := cx.KeepImage(src)` auf, um Sprite-Atlanten oder langlebige Texturen aus dieser Verdrängungsgrenze auszunehmen, bis `release()` läuft oder `cx` verworfen wird.

```go
badge := core.MustParseSVG([]byte(`<svg viewBox="0 0 120 40">
	<rect width="120" height="40" rx="8" fill="#007aff"/>
	<text x="60" y="25" text-anchor="middle" font-size="14" fill="#fff">Live</text>
</svg>`))

d.DrawSVGIn(badge, 12, 12, 180, 60)
```

Skalierte Bitmaps nutzen standardmäßig eine geglättete Abtastung. Für Pixel-Art rufen Sie `d.SetImageSmoothing(false)` vor `DrawImage`, `DrawImageFrom` oder `DrawAtlas` auf oder setzen in TSX `ctx.imageSmoothingEnabled = false`. `Save` und `Restore` sichern diese Einstellung zusammen mit dem übrigen Zeichenzustand. Nearest-Neighbor-Abtastung steht auf Apple-Plattformen, Windows, Linux, Android und im Web zur Verfügung; HarmonyOS und Mini-Programme wenden sie ebenfalls an, ihre Ausgabe ist jedoch noch nicht auf Geräten geprüft.

`d.DrawVertices(image, vertices)` zeichnet ein texturiertes Mesh. Jeweils drei aufeinanderfolgende `core.CanvasVertex`-Werte bilden ein Dreieck. Setzen Sie `Position` in Canvas-Punkten und `TexCoord` in Bildpunkten, wie bei `DrawImageFrom`. Die `Color` jedes Vertex multipliziert die Textur und wird über das Dreieck interpoliert. Ihr Nullwert belässt die Textur unverändert. Aktualisieren Sie die Punkte und zeichnen Sie die Aufzeichnung erneut, um das Mesh zu verformen. Das Bild kann eine beliebige `ImageSource` sein, einschließlich eines Pixel-Puffers oder `Source()` eines `OffscreenImage`; Android-Canvases und Mini-Programme zeichnen noch keine Offscreen-Bilder. `SetImageSmoothing(false)` gilt auch für Meshes. Im Web sind für beliebige Meshes Inline- oder Same-Origin-Bilder erforderlich. Mini-Programme zeichnen Meshes mit dem Web-Player, was in den WeChat DevTools oder auf einem Gerät noch nicht geprüft wurde. HarmonyOS schneidet jedes Dreieck zu und zeichnet das Bild über seine affine Transformation; wie bei `DrawImage` zeichnet es codierte Bilder und Pixel-Puffer, aber keine Offscreen-Bilder, und wendet den durchschnittlichen Vertex-Alpha-Wert jedes Dreiecks an, jedoch keine Farbtönung. Es verbirgt Nähte nur zwischen opaken Dreiecken und ist noch nicht auf einem Gerät geprüft.

`DrawTintedImage`, `DrawTintedImageIn` und `DrawTintedImageFrom` multiplizieren die sRGB- und Alpha-Kanäle eines Bilds vor dem Compositing mit einer Farbe. Im Standardmodus Source-Over lassen transparente Pixel den Hintergrund sichtbar. Verwenden Sie in TSX `drawTintedImage(image, tint, ...args)` mit denselben Koordinatenformen wie `drawImage`. Eine Cross-Origin-Quelle im Web muss anonyme CORS-Anfragen erlauben, damit ihre Pixel eingefärbt werden können. HarmonyOS tönt Bilder aus Bytes, den App-Ressourcen und Pixel-Puffern, was allerdings noch nicht auf einem Gerät geprüft ist; Offscreen-Bilder werden dort noch nicht gezeichnet.

Verwenden Sie `d.DrawAtlas(source, rects, xforms, alphas)`, um viele Sprites aus einem Bild der Reihe nach zu zeichnen. Jedes `core.Bounds` wählt eine Kachel in den eigenen Punkten des Bilds aus. Das zugehörige `core.RSXform` bildet die linke obere Ecke der Kachel auf `TX, TY` ab, wobei `SCos` und `SSin` auf den Skalierungsfaktor mal Kosinus bzw. Sinus der Drehung gesetzt werden. Beispielsweise zeichnet `core.RSXform{SCos: 0.5, TX: 40, TY: 20}` eine 32-Punkt-Kachel in halber Größe bei (40, 20). Übergeben Sie `nil` für volle Sprite-Deckkraft oder geben Sie Faktoren an, die den aktuellen Zeichen-Alpha-Wert multiplizieren.

`Drawing.Node` benennt einen Teil einer Zeichnung – etwa einen Balken eines Diagramms oder eine Region einer Karte – mit Begrenzung, Rolle, Label und Auswahlzustand. Assistive Technologien lesen diese Teile als Kinder des Canvas vor, und Tests finden sie anhand von Label, Text oder Test-ID; ein Druck auf ein solches Element entspricht einem Druck in dessen Mittelpunkt.

```go
d.Node(core.CanvasNode{
	Bounds: core.Bounds{Origin: core.Point{X: x, Y: y}, Size: core.Size{Width: w, Height: h}},
	Role:   core.RoleButton,
	Label:  "Open the report",
	TestID: "report",
})
```

## Offscreen-Bilder

`cx.NewOffscreenImage(size, scale, draw)` rendert ein `core.Drawing` einmalig in ein zwischengespeichertes Offscreen-Bild, das spätere Frames mit `DrawImage`, `DrawImageIn`, `DrawImageFrom`, `DrawAtlas` oder `core.Image` zeichnen. Ein Wert `scale <= 0` verwendet das aktuelle Pixelverhältnis des Displays. Die Zeichnung wird nur neu gerendert, wenn `Invalidate()` oder `Resize(size, scale)` aufgerufen wird (oder neue Schriftarten registriert werden), und beim Verwerfen von `cx` oder beim Aufruf von `Release()` automatisch freigegeben.

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

## Animierte Argumente

`Drawing.Bind` bindet ein Argument der letzten Operation an einen animierten Knoten, und die Plattform zeichnet nur diesen Teil auf ihrem Frame-Takt neu – ohne Commit. `core.ArgX`, `ArgY`, `ArgR`, `ArgA` und `ArgColor` benennen die Argumente; macOS, iOS und Linux zeichnen nur den geänderten Bereich neu, während Windows ihn erneut aufzeichnet und Android sowie das Web den gesamten Canvas neu zeichnen.

```go
needle := core.NewAnimatedValue(cx, 0)
d.Save().Translate(cx, cy).Rotate(0).Bind(core.ArgA, needle).
	BeginPath().MoveTo(-14, -3).LineTo(80, 0).LineTo(-14, 3).ClosePath().Fill().Restore()
```

## Shader

`core.Shader` füllt sein Feld mit einem Fragment-Shader, den die GPU für jedes Pixel ausführt. `core.CompileShader` nimmt ein Programm in Go-Syntax mit einer `Fragment`-Funktion entgegen, und `core.CompileWGSL` nimmt WGSL. `Uniforms` setzt die exportierten Variablen des Programms, und `Images` übergibt ihm bis zu vier Texturen.

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

Ein Go-Shader empfängt die Mitte des Pixels in `dst` sowie denselben Punkt in Punkten in `pos` und verfügt über die eingebauten Funktionen von GLSL; `rectSize`, `pixelScale`, `image0At` bis `image3At` und deren Größen sind Ergänzungen von NSGo. Ein WGSL-Programm definiert `fn shade(uv: vec2f) -> vec4f` und liest `nsh.time` sowie `nsh.size`; `Paused` hält seine Uhr an. Eine Plattform ohne GPU-Pfad für Shader zeichnet dort nichts.

`core.ShaderEffect` zeichnet seine Kinder durch WGSL und `core.ScreenEffect` deckt die gesamte Oberfläche ab. `core.ShaderTransition` und `core.ScreenTransition` tasten die Bilder vor und nach einer Änderung von `Value` ab. `core.TransitionShader` übernimmt dies für Navigation und Dialoge. Auf Android erfordert das Erfassen von Views für Effekte Android 10 oder neuer sowie OpenGL ES 3.0 mit Unterstützung für externe Texturen. Sowohl `core.Canvas` als auch `core.Shader` akzeptieren `Label`, und ihre Kinder werden über ihnen gezeichnet.

## Weiter

- [Bilder und Icons](images.md): Die Bilder, die ein Canvas zeichnet.
- [Animationen](animation.md): Werte, die ein Canvas bindet.
- [Barrierefreiheit](accessibility.md): Die Teile, die ein Canvas bereitstellt.

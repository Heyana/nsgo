---
title: Canvas e shader
description: Disegna grafica 2D sul motore e riempi una vista con un fragment shader sulla GPU.
group: Concetti chiave
order: 59
---

## Un canvas

`core.Canvas` registra il disegno 2D sul motore e lo fa eseguire alla piattaforma, come fa il `<canvas>` di una pagina web. `Draw` viene eseguito nuovamente quando cambia un segnale letto al suo interno, la dimensione del canvas o i font disponibili, e la piattaforma disegna la registrazione sul thread principale. Un font caricato tramite `cx.RegisterFont` riesegue `Draw` una volta disponibile, aggiornando sia il testo sia la geometria basata su `TextMetrics`.

Usa `Em` o `EmFunc` per un canvas dimensionato in em del suo font ereditato. Per i contenuti la cui altezza dipende dalla larghezza, `MeasureEm(width, fontSize)` restituisce un `EmBox` alla larghezza disponibile in punti (`core.Auto` quando illimitata) e alla dimensione del font risolta. Le sue letture sono tracciate e viene eseguito anche durante il layout, quindi mantienilo privo di scritture su segnali. `Ref.ContentBox()` tiene traccia del riquadro di layout di una vista montata all'interno di padding e bordi, nelle sue stesse coordinate.

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

La registrazione segue `CanvasRenderingContext2D` del web: `Save` e `Restore`, trasformazioni, tracciati di linee, curve, rettangoli e archi, riempimenti e tratti con colori e gradienti, ritaglio, tratteggi, ombre, testo e immagini. `core.ParsePath` crea un `core.Path` dai dati di un tracciato SVG, mentre `FillFromView` e `StrokeFromView` prendono gli stili `Fill` e `Stroke` ereditati dalla vista canvas, così un grafico segue il tema dell'app.

Sul web, i canvas all'interno di un `core.Scroll` ingrandito o di uno `Style.Transform` / `AnimatedStyle` scalato vengono rieseguiti alla scala cumulativa una volta terminato il movimento, fino a quattro volte il rapporto pixel dello schermo. Un canvas la cui bitmap ingrandita supererebbe 4096 × 4096 pixel disegna solo la parte visibile e si aggiorna durante lo scorrimento o il pan, mantenendo nitidi linee e glifi senza rieseguire `Draw`. Allo stesso modo, Apple, Android, Windows e Linux mantengono nitidi canvas e testi ingranditi sia sotto zoom dello scorrimento sia sotto trasformazioni di stile.

```go
d.SetFillGradient(core.CanvasLinear(0, 0, float32(size.Width), 0,
	core.GradientStop{Offset: 0, Color: core.RGB(0x4da3ff)},
	core.GradientStop{Offset: 1, Color: core.RGB(0x007aff)},
)).FillRect(0, 0, float32(size.Width), 40)
```

## Testo, immagini e parti

`MeasureText`, `TextMetrics` e `SetTextAlign` impaginano il testo sul motore tipografico della piattaforma, così le etichette usano gli stessi font del resto dell'app. `DrawImage`, `DrawImageIn` e `DrawImageFrom` disegnano immagini, `DrawSVG` e `DrawSVGIn` disegnano documenti `*core.SVG` analizzati (`core.ParseSVG`) alla loro dimensione intrinseca o adattati in un rettangolo — con gli elementi `<text>` e `<tspan>` misurati e renderizzati dal motore tipografico della piattaforma stessa e `currentColor` preso dal colore di riempimento del disegno — e `OnPress` può verificare l'ultimo disegno con `IsPointInPath` e `IsPointInStroke`. I backend nativi memorizzano nella cache le 64 sorgenti di immagini canvas non trattenute disegnate più di recente; chiama `release := cx.KeepImage(src)` per fissare atlanti di sprite o texture di lunga durata fuori da quel limite di rimozione finché `release()` non viene eseguito o `cx` non viene smaltito.

```go
badge := core.MustParseSVG([]byte(`<svg viewBox="0 0 120 40">
	<rect width="120" height="40" rx="8" fill="#007aff"/>
	<text x="60" y="25" text-anchor="middle" font-size="14" fill="#fff">Live</text>
</svg>`))

d.DrawSVGIn(badge, 12, 12, 180, 60)
```

Le bitmap scalate usano per impostazione predefinita un campionamento morbido. Per la pixel art, chiama `d.SetImageSmoothing(false)` prima di `DrawImage`, `DrawImageFrom` o `DrawAtlas`, oppure imposta `ctx.imageSmoothingEnabled = false` in TSX. `Save` e `Restore` conservano l'impostazione insieme al resto dello stato di disegno. Il campionamento nearest-neighbor è disponibile su piattaforme Apple, Windows, Linux, Android e web; anche HarmonyOS e i mini program lo applicano, sebbene il loro risultato non sia ancora stato verificato sui dispositivi.

`d.DrawVertices(image, vertices)` disegna una mesh texturizzata. Ogni tre valori `core.CanvasVertex` consecutivi formano un triangolo. Imposta `Position` in punti del canvas e `TexCoord` in punti dell'immagine, come in `DrawImageFrom`. Il `Color` di ciascun vertice moltiplica la texture e si interpola sul triangolo. Il suo valore zero lascia invariata la texture. Aggiorna i punti e registra nuovamente il disegno per deformare la mesh. L'immagine può essere qualsiasi `ImageSource`, incluso un buffer di pixel o `Source()` di una `OffscreenImage`; i canvas su Android e i mini program non disegnano ancora immagini offscreen. `SetImageSmoothing(false)` si applica anche alle mesh. Il web richiede immagini inline o della stessa origine per mesh arbitrarie. I mini program disegnano le mesh con il player web, che non è ancora stato verificato in WeChat DevTools né su un dispositivo. HarmonyOS ritaglia ciascun triangolo e disegna l'immagine attraverso la sua trasformazione affine; come con `DrawImage`, disegna immagini codificate e buffer di pixel ma non immagini offscreen, e applica l'alfa medio dei vertici di ogni triangolo ma nessuna tinta colore. Nasconde soltanto le giunture tra triangoli opachi e non è ancora stato verificato su un dispositivo.

`DrawTintedImage`, `DrawTintedImageIn` e `DrawTintedImageFrom` moltiplicano i canali sRGB e alfa di un'immagine per un colore prima della composizione. Con la modalità predefinita source-over, i pixel trasparenti lasciano visibile lo sfondo. Usa `drawTintedImage(image, tint, ...args)` in TSX con le stesse forme di coordinate di `drawImage`. Una sorgente cross-origin sul web deve consentire richieste CORS anonime affinché i suoi pixel possano essere colorati. HarmonyOS colora immagini provenienti da byte, risorse dell'app e buffer di pixel, sebbene non sia ancora stato verificato su un dispositivo; lì le immagini offscreen non vengono ancora disegnate.

Usa `d.DrawAtlas(source, rects, xforms, alphas)` per disegnare molti sprite da una singola immagine in ordine. Ogni `core.Bounds` seleziona una tessera nei punti propri dell'immagine. Il corrispondente `core.RSXform` mappa l'angolo superiore sinistro della tessera su `TX, TY`, con `SCos` e `SSin` impostati sulla scala moltiplicata rispettivamente per il coseno e il seno della rotazione. Ad esempio, `core.RSXform{SCos: 0.5, TX: 40, TY: 20}` disegna una tessera di 32 punti a metà dimensione alle coordinate (40, 20). Passa `nil` per la piena opacità degli sprite, oppure fornisci fattori che moltiplicano l'alfa corrente del disegno.

`Drawing.Node` assegna un nome a una parte di un disegno, come una barra di un grafico o una regione di una mappa, con riquadro, ruolo, etichetta e stato di selezione. La tecnologia assistiva legge quelle parti come figli del canvas e i test le trovano per etichetta, testo o ID di test; una pressione su una di esse corrisponde a una pressione al suo centro.

```go
d.Node(core.CanvasNode{
	Bounds: core.Bounds{Origin: core.Point{X: x, Y: y}, Size: core.Size{Width: w, Height: h}},
	Role:   core.RoleButton,
	Label:  "Open the report",
	TestID: "report",
})
```

## Immagini offscreen

`cx.NewOffscreenImage(size, scale, draw)` renderizza un `core.Drawing` una sola volta in un'immagine offscreen persistente che i fotogrammi successivi disegnano con `DrawImage`, `DrawImageIn`, `DrawImageFrom`, `DrawAtlas` o `core.Image`. Passare `scale <= 0` usa il rapporto pixel corrente del display. Il disegno viene nuovamente renderizzato solo quando viene chiamato `Invalidate()` o `Resize(size, scale)` (oppure quando vengono registrati nuovi font) e viene rilasciato automaticamente quando `cx` viene smaltito o viene chiamato `Release()`.

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

## Argomenti animati

`Drawing.Bind` associa un argomento dell'ultima operazione a un nodo animato, e la piattaforma ridisegna solo quella parte sul proprio clock dei fotogrammi, senza un commit. `core.ArgX`, `ArgY`, `ArgR`, `ArgA` e `ArgColor` nominano gli argomenti; macOS, iOS e Linux ridisegnano solo l'area modificata, mentre Windows la registra nuovamente e Android e il web ridisegnano l'intero canvas.

```go
needle := core.NewAnimatedValue(cx, 0)
d.Save().Translate(cx, cy).Rotate(0).Bind(core.ArgA, needle).
	BeginPath().MoveTo(-14, -3).LineTo(80, 0).LineTo(-14, 3).ClosePath().Fill().Restore()
```

## Shader

`core.Shader` riempie il proprio riquadro con un fragment shader eseguito dalla GPU per ogni pixel. `core.CompileShader` accetta un programma scritto nella sintassi di Go con una funzione `Fragment`, mentre `core.CompileWGSL` accetta WGSL. `Uniforms` imposta le variabili esportate del programma e `Images` gli fornisce fino a quattro texture.

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

Uno shader Go riceve il centro del pixel in `dst` e lo stesso punto in punti in `pos`, e dispone delle funzioni integrate di GLSL; `rectSize`, `pixelScale`, da `image0At` a `image3At` e le relative dimensioni sono aggiunte di NSGo. Un programma WGSL scrive `fn shade(uv: vec2f) -> vec4f` e legge `nsh.time` e `nsh.size`; `Paused` ne arresta il clock. Una piattaforma priva di percorso GPU per gli shader non disegna nulla in quel caso.

`core.ShaderEffect` disegna i propri figli attraverso WGSL e `core.ScreenEffect` copre la superficie. `core.ShaderTransition` e `core.ScreenTransition` campionano le immagini prima e dopo la modifica di `Value`. `core.TransitionShader` svolge questa funzione per navigazione e finestre di dialogo. Su Android, la cattura delle viste per gli effetti richiede Android 10 o successivo e OpenGL ES 3.0 con supporto per le texture esterne. Sia `core.Canvas` sia `core.Shader` accettano `Label` e i loro figli vengono disegnati sopra di essi.

## Prossimi passi

- [Immagini e icone](images.md): le immagini che un canvas disegna.
- [Animazioni](animation.md): i valori che un canvas associa.
- [Accessibilità](accessibility.md): le parti che un canvas espone.

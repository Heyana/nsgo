---
title: Canvas e shaders
description: Desenhe gráficos 2D na engine e preencha uma visualização com um shader de fragmento na GPU.
group: Conceitos principais
order: 59
---

## Um canvas

`core.Canvas` grava desenhos 2D na engine e faz a plataforma reproduzi-los, como faz o `<canvas>` de uma página web. `Draw` é executado novamente quando um sinal que ele leu, o tamanho do canvas ou as fontes disponíveis mudam, e a plataforma desenha a gravação na thread principal. Uma fonte carregada via `cx.RegisterFont` executa `Draw` novamente assim que fica disponível, atualizando tanto o texto quanto a geometria baseada em `TextMetrics`.

Use `Em` ou `EmFunc` para um canvas dimensionado em ems de sua fonte herdada. Para conteúdos cuja altura depende da largura, `MeasureEm(width, fontSize)` retorna um `EmBox` na largura disponível em pontos (`core.Auto` quando ilimitada) e no tamanho de fonte resolvido. Suas leituras são rastreadas e ele também é executado durante o layout, portanto mantenha-o livre de escritas em sinais. `Ref.ContentBox()` acompanha a caixa de layout de uma visualização montada dentro do preenchimento e das bordas, em suas próprias coordenadas.

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

A gravação segue a `CanvasRenderingContext2D` da web: `Save` e `Restore`, transformações, caminhos de linhas, curvas, retângulos e arcos, preenchimentos e traços com cores e gradientes, recortes, tracejados, sombras, texto e imagens. `core.ParsePath` cria um `core.Path` a partir de dados de caminho SVG, e `FillFromView` e `StrokeFromView` usam os estilos `Fill` e `Stroke` herdados da visualização do canvas, para que um gráfico acompanhe o tema do aplicativo.

Na web, canvases dentro de um `core.Scroll` com zoom ou de um `Style.Transform` / `AnimatedStyle` escalado são reproduzidos novamente na escala acumulada assim que o movimento para, até quatro vezes a razão de pixels da tela. Um canvas cujo bitmap com zoom excederia 4096 × 4096 pixels desenha apenas sua parte visível e atualiza conforme rola ou se desloca, mantendo linhas e glifos nítidos sem reexecutar `Draw`. Apple, Android, Windows e Linux também mantêm canvases e textos com zoom nítidos sob zoom de rolagem e transformações de estilo.

```go
d.SetFillGradient(core.CanvasLinear(0, 0, float32(size.Width), 0,
	core.GradientStop{Offset: 0, Color: core.RGB(0x4da3ff)},
	core.GradientStop{Offset: 1, Color: core.RGB(0x007aff)},
)).FillRect(0, 0, float32(size.Width), 40)
```

## Texto, imagens e partes

`MeasureText`, `TextMetrics` e `SetTextAlign` dispõem o texto na engine de texto da plataforma, para que os rótulos usem as mesmas fontes que o restante do aplicativo. `DrawImage`, `DrawImageIn` e `DrawImageFrom` desenham figuras; `DrawSVG` e `DrawSVGIn` desenham documentos `*core.SVG` analisados (`core.ParseSVG`) em seu tamanho intrínseco ou ajustados em um retângulo — com elementos `<text>` e `<tspan>` medidos e renderizados pela própria engine de texto da plataforma e `currentColor` obtido da cor de preenchimento do desenho — e `OnPress` pode testar o último desenho com `IsPointInPath` e `IsPointInStroke`. Os backends nativos armazenam em cache as 64 origens de imagem de canvas não fixadas mais recentemente desenhadas; chame `release := cx.KeepImage(src)` para fixar atlas de sprites ou texturas de longa duração fora desse limite de despejo até que `release()` seja executado ou `cx` seja descartado.

```go
badge := core.MustParseSVG([]byte(`<svg viewBox="0 0 120 40">
	<rect width="120" height="40" rx="8" fill="#007aff"/>
	<text x="60" y="25" text-anchor="middle" font-size="14" fill="#fff">Live</text>
</svg>`))

d.DrawSVGIn(badge, 12, 12, 180, 60)
```

Bitmaps redimensionados usam amostragem suave por padrão. Para pixel art, chame `d.SetImageSmoothing(false)` antes de `DrawImage`, `DrawImageFrom` ou `DrawAtlas`, ou defina `ctx.imageSmoothingEnabled = false` em TSX. `Save` e `Restore` preservam a configuração junto com o restante do estado de desenho. A amostragem por vizinho mais próximo está disponível nas plataformas Apple, Windows, Linux, Android e web; HarmonyOS e miniprogramas também a aplicam, embora sua saída ainda não tenha sido verificada em dispositivos.

`d.DrawVertices(image, vertices)` desenha uma malha texturizada. Cada três valores consecutivos de `core.CanvasVertex` formam um triângulo. Defina `Position` em pontos do canvas e `TexCoord` em pontos da imagem, como em `DrawImageFrom`. O `Color` de cada vértice multiplica a textura e é interpolado pelo triângulo. Seu valor zero deixa a textura inalterada. Atualize os pontos e grave o desenho novamente para deformar a malha. A imagem pode ser qualquer `ImageSource`, incluindo um buffer de pixels ou o `Source()` de uma `OffscreenImage`; os canvases do Android e os miniprogramas ainda não desenham imagens offscreen. `SetImageSmoothing(false)` também se aplica às malhas. A web requer imagens inline ou de mesma origem para malhas arbitrárias. Os miniprogramas desenham malhas com o player da web, o que ainda não foi verificado no WeChat DevTools ou em um dispositivo. O HarmonyOS recorta cada triângulo e desenha a imagem por meio de sua transformação afim; assim como em `DrawImage`, ele desenha imagens codificadas e buffers de pixels, mas não imagens offscreen, e aplica o alfa médio dos vértices de cada triângulo, mas nenhuma tonalidade de cor. Ele oculta as emendas apenas entre triângulos opacos e ainda não foi verificado em um dispositivo.

`DrawTintedImage`, `DrawTintedImageIn` e `DrawTintedImageFrom` multiplicam os canais sRGB e alfa de uma imagem por uma cor antes da composição. Com o modo padrão source-over, pixels transparentes mantêm o fundo visível. Use `drawTintedImage(image, tint, ...args)` em TSX com as mesmas formas de coordenadas que `drawImage`. Uma origem cross-origin na web deve permitir requisições CORS anônimas para que seus pixels sejam tingidos. O HarmonyOS aplica tonalidade a imagens a partir de bytes, recursos do aplicativo e buffers de pixels, embora ainda não tenha sido verificado em um dispositivo; imagens offscreen ainda não são desenhadas lá.

Use `d.DrawAtlas(source, rects, xforms, alphas)` para desenhar vários sprites de uma mesma imagem em ordem. Cada `core.Bounds` seleciona um bloco (tile) nos pontos próprios da imagem. O `core.RSXform` correspondente mapeia o canto superior esquerdo do bloco para `TX, TY`, com `SCos` e `SSin` definidos como a escala multiplicada pelo cosseno e pelo seno da rotação. Por exemplo, `core.RSXform{SCos: 0.5, TX: 40, TY: 20}` desenha um bloco de 32 pontos pela metade do tamanho em (40, 20). Passe `nil` para opacidade total dos sprites ou forneça fatores que multiplicam o alfa atual do desenho.

`Drawing.Node` nomeia uma parte de um desenho, como uma barra de um gráfico ou uma região de um mapa, com limites, um papel, um rótulo e um estado selecionado. A tecnologia assistiva lê essas partes como filhos do canvas, e os testes as encontram por rótulo, texto ou ID de teste; um pressionamento em uma delas é um pressionamento em seu centro.

```go
d.Node(core.CanvasNode{
	Bounds: core.Bounds{Origin: core.Point{X: x, Y: y}, Size: core.Size{Width: w, Height: h}},
	Role:   core.RoleButton,
	Label:  "Open the report",
	TestID: "report",
})
```

## Imagens offscreen

`cx.NewOffscreenImage(size, scale, draw)` renderiza um `core.Drawing` uma vez em uma imagem offscreen retida que quadros posteriores desenham com `DrawImage`, `DrawImageIn`, `DrawImageFrom`, `DrawAtlas` ou `core.Image`. Passar `scale <= 0` usa a razão de pixels da tela atual. O desenho só é renderizado novamente quando `Invalidate()` ou `Resize(size, scale)` é chamado (ou quando novas fontes são registradas), e é liberado automaticamente quando `cx` é descartado ou `Release()` é chamado.

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

`Drawing.Bind` vincula um argumento da última operação a um nó animado, e a plataforma redesenha apenas essa parte no seu relógio de quadros, sem um commit. `core.ArgX`, `ArgY`, `ArgR`, `ArgA` e `ArgColor` nomeiam os argumentos; macOS, iOS e Linux redesenham apenas a área alterada, enquanto o Windows grava novamente e o Android e a web redesenham o canvas inteiro.

```go
needle := core.NewAnimatedValue(cx, 0)
d.Save().Translate(cx, cy).Rotate(0).Bind(core.ArgA, needle).
	BeginPath().MoveTo(-14, -3).LineTo(80, 0).LineTo(-14, 3).ClosePath().Fill().Restore()
```

## Shaders

`core.Shader` preenche sua caixa com um shader de fragmento que a GPU executa para cada pixel. `core.CompileShader` recebe um programa escrito na sintaxe do Go com uma função `Fragment`, e `core.CompileWGSL` recebe WGSL. `Uniforms` define as variáveis exportadas do programa, e `Images` fornece a ele até quatro texturas.

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

Um shader Go recebe o centro do pixel em `dst` e o mesmo ponto em pontos em `pos`, e possui as funções embutidas do GLSL; `rectSize`, `pixelScale`, `image0At` a `image3At` e seus tamanhos são adições do NSGo. Um programa WGSL escreve `fn shade(uv: vec2f) -> vec4f` e lê `nsh.time` e `nsh.size`; `Paused` para o seu relógio. Uma plataforma sem caminho de GPU para shaders não desenha nada ali.

`core.ShaderEffect` desenha seus filhos por meio de WGSL e `core.ScreenEffect` cobre a superfície. `core.ShaderTransition` e `core.ScreenTransition` amostram as imagens antes e depois de `Value` mudar. `core.TransitionShader` faz isso para navegação e diálogos. No Android, capturar visualizações para efeitos requer Android 10 ou posterior e OpenGL ES 3.0 com suporte a texturas externas. Tanto `core.Canvas` quanto `core.Shader` aceitam `Label` e seus filhos são desenhados sobre eles.

## Próximos passos

- [Imagens e ícones](images.md): as figuras que um canvas desenha.
- [Animação](animation.md): valores aos quais um canvas se vincula.
- [Acessibilidade](accessibility.md): as partes que um canvas expõe.

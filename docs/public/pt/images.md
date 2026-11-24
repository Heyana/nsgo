---
title: Imagens e ícones
description: Exiba uma imagem a partir de uma URL, dos recursos do app, de bytes ou de pixels desenhados pelo app, além de ícones Lucide e SVGs.
group: Conceitos principais
order: 54
---

## Uma imagem

`core.Image` exibe um bitmap ou um SVG: uma URL, um arquivo, um caminho `asset:` nos recursos do app ou bytes mantidos pelo app. A plataforma carrega, decodifica e armazena em cache fora da thread principal, e a thread principal desenha o resultado.

```go
core.Image{
	Source: core.Src("https://example.com/cover.jpg"),
	ResizeMode: core.Cover,
	Style: core.Style{Width: core.Pt(160), Height: core.Pt(100), CornerRadius: 12},
}
```

`core.Cover` preenche a caixa e recorta o que transborda; `core.Contain` encaixa a imagem inteira, `core.Center` mantém seu tamanho natural e `core.ScaleDown` a mantém no tamanho natural, a menos que seja maior que a caixa. `BlurRadius` desfoca a imagem em pontos, `TintColor` recolore seus pixels opacos e `ObjectPosition` a posiciona dentro da caixa.

Bitmaps redimensionados usam amostragem suave por padrão. Defina `Sampling: core.ImageSamplingNearest` em Go ou `sampling="nearest"` em TSX para manter as bordas nítidas de pixel art e buffers de pixels. Plataformas Apple, Windows, Linux, Android e web suportam essa configuração; HarmonyOS e miniprogramas também a aplicam, embora o resultado ainda não tenha sido verificado em dispositivos físicos. Para bordas exatas de pixel em um navegador usando CSS `pixelated`, escale para um número inteiro de pixels do dispositivo por pixel de origem.

`Source` declara o tamanho usado pelo layout: uma dimensão deriva a outra, e uma imagem sem dimensões definidas fica com 0 por 0 até que seu estilo defina um tamanho. `core.ImageSource{Data: data, Size: core.Size{Width: 24, Height: 24}}` exibe bytes incorporados com `go:embed`, e `Size` dá a um SVG seu tamanho intrínseco.

Com [`desktop.WindowPointerPainted`](services.md), `HitTestAlpha: 0.5` permite que o ponteiro atravesse pixels com menos de metade de opacidade no macOS e no Windows. Em TSX, o mesmo limiar é definido com `hitTestAlpha`.

## Quadros desenhados pelo app

`Pixels` exibe um `*image.RGBA` desenhado pelo app, como quadros de um player de vídeo ou de um emulador, sem codificação, carregamento ou decodificação. Escreva cada quadro no mesmo buffer e incremente `Version`: a view o exibe sem eventos de carregamento e sem fade, preservando o canal alfa, e no macOS, iOS, tvOS, watchOS, Android, Windows e Linux faz a troca no local sem nenhum quadro em branco no intervalo. Sem `Size`, as dimensões são medidas em pontos a partir dos pixels.

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

O NSGo copia os pixels quando o commit que carrega uma nova `Version` é executado, então o app pode escrever o próximo quadro no próximo callback de `RequestAnimationFrame`; um decodificador em sua própria goroutine escreve em um segundo buffer e envia a troca. Uma mesma `Version` não é desenhada novamente. `cx.ImageSize` responde imediatamente para uma fonte de pixels com o tamanho de seu buffer. Um `core.Canvas` desenha pixels com `DrawImage`, copiados da mesma forma, em todas as plataformas; miniprogramas exigem um canvas 2D do WeChat. As `Images` de um `core.Shader` aceitam pixels da mesma maneira, permitindo que um shader filtre um quadro escrito pelo app, como a tela de um emulador, em toda plataforma que desenha shaders.

Plataformas Apple, Android, Windows, Linux e web atualizam imagens de pixels diretamente, e miniprogramas do WeChat usam um canvas 2D. O HarmonyOS cria um `PixelMap` para cada versão, verificado em dispositivo, e suas cópias atravessam uma ponte de strings, de modo que buffers grandes reescritos a cada quadro rodam bem abaixo de 60 fps por lá ([buffers de pixels](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/images.md#pixel-buffers)).

## Capturar uma view

Chame `cx.Capture(&ref, done)` com o `core.Ref` de uma view montada, ou `cx.Capture(nil, done)` para a superfície atual. O callback recebe os bytes PNG na escala de pixels da superfície ou um erro na goroutine do motor. Use `core.ImageSource{Data: png}` para exibir o resultado. Para compartilhar, grave os bytes em um arquivo `.png` no diretório de cache do app e passe o caminho em `core.ShareOptions.Files`. [Compartilhando arquivos](services.md#abrindo-links-e-arquivos)

A captura lê os pixels visíveis da superfície dentro dos limites da view. Está disponível no macOS, iOS, tvOS, Windows, Linux, Android e web. O Android captura a janela da activity. A captura na web desenha o DOM do app em um canvas, incluindo saída de shaders do NSGo, e omite frames incorporados, imagens que não pode ler e alguns efeitos CSS.

## Recursos e escalas

Arquivos em `resources/` são empacotados com o app. `core.Src("asset:images/cover.png")` carrega um deles, e `Scales: core.ScalesOf(1, 2, 3)` escolhe `cover.png`, `cover@2x.png` ou `cover@3x.png` de acordo com a proporção de pixels da tela.

```go
core.Image{Source: core.ImageSource{
	URI:    "asset:images/cover.png",
	Scales: core.ScalesOf(1, 2, 3),
}}
```

`Placeholder` exibe uma fonte enquanto a imagem carrega, e `FadeDuration` faz a transição da imagem carregada sobre ela; 0 deixa a plataforma escolher, e um nanossegundo exibe imediatamente. `OnLoadStart`, `OnProgress`, `OnLoad`, `OnError` e `OnLoadEnd` acompanham o carregamento. `cx.ImageSize` lê o tamanho em pixels de uma fonte sem exibi-la, `cx.PrefetchImage` baixa uma imagem para o cache, `cx.SetImageCacheLimit` define o orçamento em bytes do cache em memória de imagens codificadas (padrão `core.DefaultImageCacheLimit`, 32 MiB; valores negativos são limitados a 0) e `cx.KeepImage` fixa um atlas ou textura fora da remoção do cache até que a função `release` retornada seja executada ou seu escopo seja descartado. O limite se aplica a cada cache, excluindo itens fixados, carregamentos em andamento e o passo de desenho atual; ele não limita pixels decodificados, memória do processo ou cache do navegador. Zero desativa a retenção de itens não fixados. Backends nativos mantêm imagens remotas em um cache próprio em disco.

## Imagens animadas e vetoriais

Animações GIF e WebP são reproduzidas na view, cada quadro pelo seu próprio atraso, repetindo conforme especificado no arquivo. Documentos SVG são carregados como imagens e permanecem nítidos em qualquer tamanho; um SVG monocromático pode ser tingido com `TintColor`.

## Ícones

`core.Icon` desenha um SVG na cor de texto herdada, com a altura do tamanho da fonte, a menos que seu estilo defina outro tamanho. Sua caixa preserva a proporção do SVG, para que uma caixa quadrada não o distorça.

```go
core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 20}, Label: "Search"}
```

`icons/lucide` possui um acessor para cada ícone do Lucide, nomeado em PascalCase (`lucide.ArrowUpRight`). Um ícone é tratado como elemento decorativo para tecnologias assistivas, a menos que `Label` o nomeie. `ui.Icon` faz o mesmo com os padrões de `ui`, e `core.SVG` e `core.SVGView` desenham caminhos construídos pelo próprio app.

## A imagem estilizada

`ui.Image` é uma imagem em um recipiente arredondado, com um placeholder enquanto carrega e um ícone discreto de imagem quando falha. Ela mantém a proporção da fonte, ou 3:2 quando não houver uma, a menos que seu estilo defina os dois lados.

```go
ui.Image{
	Source:      core.Src("asset:images/cover.png"),
	Label:       "Cover",
	ResizeMode:  core.Cover,
	AspectRatio: 16.0 / 9.0,
	Placeholder: ui.Skeleton{Style: core.Style{Height: core.Pt(180)}},
}
```

## Próximos passos

- [Texto](text.md): imagens dentro de uma linha, como spans.
- [Estilos e temas](styles.md): cantos, sombras e colorações.
- [Listas e rolagem](lists.md): linhas de imagens com reciclagem.

---
title: Texto
description: Parágrafos, spans, links, fontes e documentos.
group: Componentes
order: 32
---

`core.Text` é um parágrafo composto pelo mecanismo de texto da plataforma. `ui.Text` e `ui.Textf` são auxiliares para uma linha na tipografia de `ui`; o elemento continua sendo um `core.Text`, então também aceita `Style` e `Spans`.

```go preview=text/basic
ui.Text("Settings", 20, ui.Bold, colors.Text)
ui.Textf(func() string { return fmt.Sprintf("%d files", n.Get()) }, 13, ui.Regular, colors.Muted)
```

## Conteúdo

`Content` é fixo e `ContentFunc` segue um signal. `MaxLines` limita as linhas e `EllipsizeMode` corta o restante no início, no meio ou no fim; `Style.WhiteSpaceNoWrap` mantém um parágrafo em uma única linha. `TextAlign`, `TextTransform`, `LineHeight` e `LetterSpacing` estilizam o parágrafo, e `OnTextLayout` relata suas linhas após o layout.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) },
	MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 15}}
```

## Spans e links

`Spans` cria um texto feito de trechos, cada um com um estilo próprio. Um span com `OnPress` é um link, e seu `Background` e `CornerRadius` são desenhados atrás de seus caracteres em cada linha. `SpansFunc` fornece os trechos novamente quando o que ele lê muda, para código destacado ou uma frase que muda.

```go preview=text/spans
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "Read the "},
	{Content: "guide", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openGuide},
	{Content: " before you start."},
}}
```

`Attachment` coloca uma view em uma linha, no lugar do conteúdo do span: um ícone, uma imagem ou um pequeno botão. Ela é disposta no seu próprio tamanho, quebra de linha junto com as palavras e recebe seus próprios toques.

## Fontes e estilo

`FontFamily` nomeia uma família ou uma lista CSS. `FontSize`, `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LetterSpacing`, `TextDecoration` e `TextShadowColor`, com seu deslocamento e raio, estilizam o texto. `core.FontMonospace` é a família de largura fixa da plataforma.

```go
core.Text{Content: "fn main() {}", Style: core.Style{FontFamily: core.FontMonospace, FontSize: 13}}
```

Registre fontes empacotadas com `nsgo.Fonts` antes da primeira view, ou registre uma face enquanto o aplicativo é executado com `cx.RegisterFont`. Um texto preparado antes de uma fonte chegar é preparado novamente.

## Documentos

`base/textview.TextView` mostra Markdown ou HTML com as mesmas primitivas: títulos, parágrafos, listas, tabelas, código, notas de rodapé, alertas, imagens, fórmulas e links. `textview.Parse` lê Markdown e `textview.ParseHTML` lê HTML, e `Options.Streaming` repara um documento que ainda está chegando.

```go preview=text/document height=240
textview.TextView{Markdown: "## Notes\n\nA **native** document.", Styles: ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) { open(e.URL) }}
```

`textview.NewSearch` pesquisa o texto que um `TextView` mostra e destaca suas correspondências. Dê a várias views a mesma pesquisa com `textview.ProvideSearch` e então chame `Open`, `SetQuery`, `Next` e `Previous`; `Count` e `Selected` relatam o resultado. As partes da pesquisa são estilizadas com `textview.PartSearch` e `textview.PartSearchSelected`.

## Seleção e acessibilidade

`SelectableOn` permite que o usuário selecione e copie um texto. Uma `core.View` com `Selectable` faz seus textos serem selecionados juntos na ordem do documento. Um `Role` como `RoleHeader` informa à tecnologia assistiva o que o texto é, e `Label` o substitui quando as palavras não são o rótulo.

## Plataforma

O mecanismo de texto da plataforma faz o layout do texto, então as quebras de linha, as fontes e os métodos de entrada o seguem. A quebra de linha CJK, os emojis e o texto da direita para a esquerda seguem o sistema. O aplicativo pode ler o tamanho do texto com `cx.Env().FontScale()`, e um texto o segue a menos que `NoFontScaling` esteja definido.

## Relacionados

- [Texto](../text.md): o guia completo de texto, spans, fontes e documentos.
- [Imagens e ícones](../images.md): imagens e ícones em linha.
- [Acessibilidade](../accessibility.md): títulos, rótulos e seleção.
- As páginas Text e TextView da galeria.

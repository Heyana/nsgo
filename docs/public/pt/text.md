---
title: Texto
description: Exiba texto com spans, fontes, decorações, seleção e documentos.
group: Conceitos principais
order: 53
---

## Um texto

`core.Text` é um parágrafo, composto pelo próprio mecanismo de texto da plataforma no lado do engine e desenhado pela thread principal. Seu tamanho vem da sua fonte e da sua largura, e ele quebra linhas dentro da sua caixa.

```go
core.Text{Content: "Hello from NSGo", Style: core.Style{
	FontSize:   15,
	LineHeight: 22,
	FontWeight: core.FontWeightSemibold,
}}
```

`Content` é fixo e `ContentFunc` acompanha um signal, de modo que um texto que muda o lê dentro da função. `MaxLines` limita as linhas exibidas, `EllipsizeMode` corta o restante no início, no meio ou no fim, e `WhiteSpaceNoWrap` mantém um parágrafo em uma única linha. `AdjustsFontSizeToFit` reduz o texto até que ele caiba na sua caixa e no seu limite de linhas, até `MinimumFontScale` (no mínimo 0.1). Fórmulas e outros anexos relativos à fonte encolhem junto e voltam ao tamanho natural quando há espaço; tamanhos explícitos em pontos e o padding mantêm os valores definidos pelo autor.

Os campos de texto são herdados como no CSS: um texto dentro de uma visualização com `FontSize`, `Color` ou `LineHeight` os assume, a menos que defina os seus próprios. `TextAlign`, `TextTransform`, `LetterSpacing`, `TextDecoration`, `TextDecorationStyle`, `TextDecorationColor` e `TextShadowColor`, `TextShadowOffset` e `TextShadowRadius` estilizam as linhas e os glifos.

## Trechos e links

`Spans` cria um texto feito de trechos, cada um com seu próprio estilo. Um span com `OnPress` é um link, e um toque em outro span aciona o próprio `OnPress` do texto.

```go
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "By continuing you agree to the "},
	{Content: "terms", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openTerms},
	{Content: " and the "},
	{Content: "privacy policy", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openPrivacy},
	{Content: "."},
}}
```

`SpansFunc` fornece os spans novamente quando o que ele lê muda, e apenas as partes que diferem são preparadas de novo, de modo que as linhas de um editor de código são recoloridas à medida que o realce chega. O `Background` e o `CornerRadius` de um span desenham uma caixa atrás dos seus caracteres em cada linha, e seu `Cursor` define a aparência do ponteiro sobre eles, com prioridade sobre o do texto. `Visibility: core.VisibilityHidden` oculta um span no lugar: suas palavras mantêm o espaço, então as linhas não se movem quando ele aparece, e cópias e leitores de tela o omitem, para palavras que surgem uma a uma.

Defina `BackgroundInsetTop: core.Pct(62)` em um span com `Background` para realçar os 38% inferiores da sua caixa em cada linha. `BackgroundInsetBottom` recorta a borda inferior. Ambos os recuos aceitam pontos ou porcentagens da altura da caixa, do ascendente ao descendente da fonte. Eles não alteram o layout do texto. Em TSX, defina `backgroundInsetTop: "62%"` e `backgroundInsetBottom` no estilo de um `<Text>` aninhado. Fundos arredondados e recuos verticais também são renderizados no HarmonyOS e nos quatro alvos de mini-programa.

`Attachment` coloca uma visualização dentro de um texto, no lugar do conteúdo do span. Ela é disposta no seu próprio tamanho, quebra linha junto com as palavras ao redor e recebe seus próprios toques, de modo que um ícone, uma imagem ou um pequeno botão pode ficar em uma linha. Uma seleção copia o `Content` do span no seu lugar, omitindo-o quando vazio. Por padrão, sua primeira linha de texto se alinha à linha de base ao redor, usando as métricas da sua fonte real. Um canvas `Em` fornece sua própria linha de base, e uma visualização sem linha de base se apoia na sua borda inferior. Defina `Style.VerticalAlign` para escolher outro alinhamento.

```go
core.Text{Style: core.Style{FontSize: 17, LineHeight: 24}, Spans: []core.Span{
	{Content: "Search with "},
	{Attachment: core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 17}}},
	{Content: " NSGo"},
}}
```

## Fontes

`FontFamily` nomeia uma família ou uma lista CSS (`Inter, "Noto Sans SC", sans-serif`), cada uma usada para os caracteres que faltam nas anteriores. Cada família da lista recebe o peso e o estilo. No macOS e no iOS, uma linha obtém sua altura e linha de base das faces em que seus caracteres são compostos, de modo que uma linha composta inteiramente em uma família posterior usa as dessa família, enquanto um navegador mantém as da primeira família como piso. `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LineHeightMultiple`, `LetterSpacing` e `LetterSpacingEm` definem a face e o espaçamento, e `core.FontMonospace` é a família de largura fixa da plataforma.

`Language` é a tag BCP 47 do texto, herdada como os campos acima em textos, spans e entradas. Os caracteres Han são desenhados nas suas formas, de modo que `直` definido em `ja` assume a forma japonesa mesmo quando o sistema prefere o chinês, e o coreano (`ko`) quebra linhas entre palavras como faz o `word-break: keep-all` do CSS. Textos e entradas que não definem nenhum estão no idioma exibido pelo app e o acompanham quando ele muda. Defina-o em uma visualização para uma subárvore, ou em um `Span` para um único trecho:

```go
core.Text{Spans: []core.Span{
	{Content: "日本語: "},
	{Content: "直 骨 角", Style: core.Style{Language: "ja"}},
}}
```

Empacote as fontes do app em `resources/` e registre-as antes da primeira visualização, ou registre os bytes de uma fonte enquanto o app é executado:

```go
func main() { nsgo.Run(Root, nsgo.Fonts("fonts/Inter-Regular.ttf", "fonts/Inter-Bold.ttf")) }

cx.RegisterFont(data) // a TrueType or OpenType face
```

Um texto disposto antes de uma fonte chegar é preparado novamente, e os canvases executam `Draw` de novo para que seus glifos e medições de texto usem a fonte carregada. `cx.Env().FontScale()` é o tamanho de texto do usuário, que os textos seguem a menos que `NoFontScaling` esteja definido; `MaxFontSizeMultiplier` o limita.

## Seleção e medição

`SelectableOn` permite que o usuário selecione e copie um texto, e uma `View` com `Selectable` faz seus textos serem selecionados juntos, em ordem de documento, como os de uma página web. `SelectionColor` colore o realce. `OnTextSelection` recebe um `core.TextSelection` conforme o usuário move a seleção: seu `Text` simples, `Markdown` e `HTML` (preservando a estrutura inline e de blocos analisada de quaisquer `TextView`s e textos estilizados na área, incluindo spans parciais e links), seus `Ranges` por texto e os `Bounds` dos textos que ela toca, na visualização, para copiar como Markdown ou HTML, uma lista de palavras ou uma tradução das palavras selecionadas; `desktop.LookUp` os mostra no dicionário do sistema ([serviços](services.md#abrindo-links-e-arquivos)). `cx.MeasureText` mede uma string em uma fonte, para uma coluna ajustada ao seu texto; funções reativas que a chamam são atualizadas quando as fontes disponíveis mudam, e `OnTextLayout` informa as linhas após cada layout.

## Documentos

`base/textview` exibe Markdown ou HTML com as mesmas primitivas: títulos, parágrafos, listas, tabelas, código, notas de rodapé, alertas, imagens, fórmulas e links.

```go
textview.TextView{
	Markdown: "## NSGo\n\nNative views from **Go**.",
	Styles:   ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) {
		if strings.HasPrefix(e.URL, "#") {
			return
		}
		cx.OpenURL(e.URL, nil)
	},
}
```

Fórmulas (`$…$`, `$$…$$`) exibem seu TeX até que o app passe `RenderMath: formula.RenderMath` de `base/formula`, que as compõe no tamanho e na cor do texto ao redor, lidas em voz alta no idioma do app e copiadas como TeX; isso adiciona cerca de 6,5 MB a um app desktop e 3 MB compactados com gzip a um app web, incluindo sua fonte. `formula.View` exibe uma fórmula em qualquer outro lugar, em TeX ou Typst; seu callback `OnHit` informa a parte da fonte Untie, o cursor mais próximo e os diagnósticos sobrepostos, para editores de fórmulas. No watchOS, as fórmulas usam o mesmo caminho de canvas; vinculá-las adiciona cerca de 6,1 MB de executável, incluindo a fonte de 1,4 MB. Uma execução no simulador de relógio mediu cerca de 3,9 MB de heap Go retido para carregar e analisar essa fonte, incluindo sua cópia própria dos bytes. Medições no simulador não estabelecem nem o limite de memória de um relógio físico nem a margem disponível; meça-os no relógio que executa o app. Trechos de texto (`\text{}` ou strings Typst) que suas fontes configuradas não conseguem cobrir usam as fontes do sistema, incluindo CJK e outras escritas; `formula.Fonts.Fallbacks` explícitos têm prioridade.

Para uma fonte que chega em partes, passe `SourceFunc` para `formula.View` e defina `Streaming: true`, ou vincule `StreamingFunc` ao estado de recebimento do app. Isso oculta a tinta de erros recuperáveis e de marcadores de posição enquanto o conteúdo válido continua sendo disposto. Desative a flag quando o recebimento terminar para mostrar os erros restantes. Em uma `TextView`, forneça uma função `RenderMath` que retorne esse `formula.View` com os `Math.TeX` e `Math.Display` fornecidos; seu `StreamingFunc` pode ler o mesmo estado de recebimento.

Fórmulas de exibição quebram linha em relações e operadores seguros na largura do bloco, dentro do seu padding e das suas bordas, mantendo o tamanho da fonte do texto. Redimensionar ou alterar o tamanho do texto refaz as linhas. Partes que não podem ser quebradas ainda rolam lateralmente; fórmulas inline mantêm seu tamanho natural.

Um bloco de código `mermaid` exibe sua fonte até que o app passe `RenderDiagram: diagram.RenderDiagram` de `base/diagram`, que o desenha como um diagrama nativo, com limu: fluxogramas, diagramas de sequência, de classes, de estados e de entidade-relacionamento, gráficos de Gantt, gráficos de pizza, mapas mentais, linhas do tempo e mais, na fonte da plataforma, claro ou escuro conforme o app exibe. Itens de diagrama com links os abrem pelo serviço de URL do app quando tocados, e a dica Mermaid de um item aparece enquanto o ponteiro repousa sobre ele. Ele é disposto conforme a largura disponível do texto, fazendo um fluxograma largo correr para baixo enquanto mantém o tamanho dos rótulos; diagramas que não cabem ainda rolam lateralmente. Uma cópia leva sua fonte, e um bloco que ainda está chegando aparece como código até estar completo. `diagram.View` exibe um diagrama em qualquer outro lugar e acompanha a largura do seu contêiner conforme ela muda. Seu `Icons` mapeia nomes Mermaid como `"fa:fa-car"` para documentos `*core.SVG`, incluindo os de `icons/lucide`; nomes ausentes não deixam espaço inline. Nós de imagem carregam fontes HTTP(S), de arquivo ou `asset:` pelo carregador de imagens de canvas compartilhado, ajustando e centralizando quando `constraint: "on"` está definido e esticando nos demais casos. O Mermaid filtra os endereços de imagem antes de carregar (incluindo rejeitar URLs `data:`). Para fornecer ícones em uma visualização de texto, defina `RenderDiagram` como um callback que retorne `diagram.View{Source: code.Code, Icons: icons}`.

Para dar aos diagramas a sua paleta `ui`, retorne um `diagram.View` com `ThemeFunc` do seu callback `RenderDiagram`. A função lê as cores enquanto o canvas desenha, de modo que uma mudança de paleta ou de aparência da subárvore recolore a visualização existente. Cores deixadas em zero mantêm os padrões do diagrama.

```go
func drawDiagram(cx *core.Scope, code textview.CodeBlock) core.Node {
	colors := ui.ColorsOf(cx)
	return diagram.View{Source: code.Code, ThemeFunc: func() *diagram.Theme {
		return &diagram.Theme{
			Text: colors.Text(), Surface: colors.Surface(), Border: colors.Border(),
			Line: colors.Muted(), Accent: colors.Accent(), Critical: colors.Danger(),
		}
	}}
}
// ui.TextView{Markdown: source, RenderDiagram: drawDiagram}
```

`textview.Parse` lê Markdown e `textview.ParseHTML` lê HTML em blocos que o app também pode construir. `Options.Streaming` repara um documento que chega em partes, fechando o que o seu final deixa aberto e retendo uma última linha cujo significado os próximos caracteres decidem, como um marcador de lista sozinho, para que uma resposta nunca exiba marcadores como texto por um instante, e `StreamingCaret` desenha um cursor no seu final, para uma resposta escrita enquanto chega.

Uma `textview.Search` encontra texto nas visualizações que se juntam a ela, pelo campo `Search` delas ou por `textview.ProvideSearch` acima delas. `SetQuery`, `Open` e `Close` a controlam a partir de uma barra de busca, `Count` e `Selected` dizem onde ela está, e `Next` e `Previous` percorrem suas correspondências em ordem de leitura. As visualizações marcam cada correspondência com `PartSearch` e a selecionada com `PartSearchSelected`, para que os estilos do app as colorem, rolam-na até ficar visível e deixam a seleção do usuário intacta.

```go
search := textview.NewSearch(cx)
textview.ProvideSearch(cx, search)
search.SetQuery("readme")
search.Open()
```

## Próximos passos

- [Imagens e ícones](images.md): imagens e SVGs, inclusive em um texto.
- [Estilos e temas](styles.md): cores e campos de texto que são herdados.
- [Layout](layout.md): como um texto obtém sua largura.

## Edição de texto rico

`ui/richeditor.View` exibe uma `base/richeditor.Session` com a aparência do tema; `base/richeditor.View` fornece o comportamento com estilos fornecidos pelo app. No iOS e no Android, um toque longo seleciona uma palavra e um toque duplo faz o mesmo; arraste as alças de seleção para estender o intervalo entre blocos. O menu de edição nativo oferece Copiar e Selecionar tudo, e Recortar e Colar enquanto a sessão é editável, conforme aplicável à seleção e à área de transferência. `Session.SetEditable` também atualiza um menu aberto. Estilize as alças com `Styles.SelectionHandle` ou `Classes.SelectionHandle` na visualização base.

Para um controle de texto personalizado, coloque o foco em um `core.Input` e chame `Ref.OpenEditMenu(bounds, actions)` com os limites em pontos da superfície e uma máscara de bits de `core.EditCut`, `EditCopy`, `EditPaste` e `EditSelectAll`. No iOS e no Android, `Input.OnEditAction` recebe Recortar, Copiar e Selecionar tudo; Colar usa `Input.OnPaste`. Chame `Ref.CloseEditMenu` para fechá-lo.

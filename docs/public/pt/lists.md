---
title: Listas e rolagem
description: Views de rolagem com gestos nativos e listas que reciclam linhas para qualquer quantidade de itens.
group: Conceitos principais
order: 55
---

## Uma view de rolagem

`core.Scroll` é a view de rolagem nativa da plataforma, portanto inércia, efeito elástico (bounce), barra de rolagem, encaixe (snapping) e rolagem por toque são nativos. Seus filhos são dispostos em uma coluna — ou em uma linha com `Horizontal` — tão longa quanto precisarem.

```go
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

A view de rolagem preenche seu contêiner e o padding vai em seu conteúdo, mantendo a barra de rolagem encostada na borda. Ela encolhe para caber no pai, a menos que seu estilo determine o contrário. Defina `NoShrink` quando ela precisar manter o tamanho solicitado enquanto um irmão encolhe. `HideIndicator` oculta o indicador da plataforma e `Bar` desenha a barra do próprio app.

`Behavior` e `BehaviorFunc` definem como ela rola: `Paging`, `SnapToInterval` e `SnapToOffsets` com `SnapToAlignment`, `DecelerationRate`, `NoBounces`, `ContentInset` e as regras do teclado. `MinZoomScale` e `MaxZoomScale` permitem zoom por pinça, `OnRefresh` e `Refreshing` criam o gesto de puxar para atualizar, e `OffsetValues` acompanha o deslocamento na thread principal para efeitos que se movem com a rolagem.

```go
page := core.Scroll{
	OnScroll: func(e core.ScrollEvent) { y.Set(e.Offset.Y) },
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: []core.Node{...},
}
```

`OnScroll` informa o deslocamento no máximo uma vez por turno, ou uma vez a cada `ScrollEventThrottle`. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd` e `OnScrollEnd` delimitam os gestos do usuário. `Ref` permite rolar por código com `ScrollTo`, `ScrollToEnd`, `ZoomTo` e `ScrollToIndex` em uma lista.

## Uma lista

`core.List` renderiza grandes volumes de dados em uma view de rolagem nativa criando apenas as linhas próximas à tela. `Count` indica quantas linhas existem e `Row` constrói uma linha; `Key` dá uma identidade a cada linha, para que linhas que permanecem mantenham suas células enquanto outras entram e saem acima delas.

```go
core.List{
	Count:         func() int { return len(items.Get()) },
	Key:           func(i int) any { return items.Get()[i].ID },
	EstimatedSize: 56,
	Row: func(cx *core.Scope, r core.ListRow) core.Node {
		return core.View{Style: core.Style{Padding: core.All(12)}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return items.Get()[r.Index()].Title }},
		}}
	},
}
```

`Row` executa uma vez por célula reciclada e atualiza `r.Index()` quando essa célula é reutilizada para outra linha; portanto, leia `r.Index()` e os sinais do item dentro de funções reativas (`ContentFunc`, `StyleFunc`, `NewMemo`) em vez de capturar um valor único ao construir a linha. Células ociosas ficam estacionadas para que seus cálculos aguardem até que a célula seja vinculada novamente a uma linha ativa.

`Header`, `Footer`, `Empty` e `Separator` armazenam o conteúdo ao redor e entre as linhas. `NumColumns` coloca linhas lado a lado, `Gap` e `ColumnGap` as separam e `Horizontal` rola horizontalmente. `StickyHeaderIndices` fixa essas linhas no início da área visível até que a próxima as empurre para fora, e `StickyHeaderOffset` mantém livre o espaço para um cabeçalho acima delas.

`OnViewableItemsChange` informa as linhas visíveis de acordo com os limites de `ViewabilityConfig`, e `OnLoadMore` executa quando a área visível se aproxima do fim, uma vez por quantidade de linhas. `OnLoadMoreAtStart` faz o mesmo no início, e `InitialScrollIndex` abre a lista em uma linha específica. `Alignment: core.ListAlignEnd` posiciona a lista no final, como um chat com as linhas em ordem de leitura: abre na linha mais recente, acompanha novas linhas enquanto o usuário está no final e mantém uma conversa curta na parte inferior. `Inverted` coloca a linha 0 no final, e `MaintainVisibleContentPosition` acompanha o final de uma lista crescente enquanto o usuário está próximo dele.

```go
core.List{Ref: list, Count: count, EstimatedSize: 64, Row: row, OnLoadMore: loadMore,
	StickyHeaderIndices: headers(cx), Footer: core.View{Style: core.Style{Height: core.Pt(24)}}}
```

`list.ScrollToIndexAt(index, 0.5, 0, true)` rola uma linha para o meio da área visível, e `list.ScrollToEnd(true)` rola para o final. `list.ListPosition()` lê onde a área visível começa — uma linha e o quanto já avançou nela — para preservar a posição de leitura: `list.ScrollToIndexAt(p.Index, 0, -p.Offset, false)` traz de volta uma lista recriada para aquele ponto. Uma lista também pode ficar em uma mesma página com outro conteúdo: defina `Viewport: core.ListViewportParent`, e a view de rolagem ao redor rolará a lista junto com o restante.

## Tabelas

`base/datatable` exibe uma tabela de um milhão de linhas com linhas e colunas virtualizadas, cabeçalho fixo, colunas fixadas à esquerda (`Fixed`) ou à direita (`FixedRight`) enquanto as outras rolam por baixo delas, ordenação, seleção de linha única ou múltipla, redimensionamento e movimentação de colunas e células próprias do app. `MultiSelect` suporta alternância com Command ou Control, intervalos com Shift e extensões de seleção por teclado com Shift, enquanto `SelectedRows` e `OnSelectedRowsChange` permitem que o app controle as linhas. `CellTooltip` dá uma dica de ferramenta a uma célula, que também a descreve para leitores de tela, estilizada por `Styles.CellTooltip`; o que `RenderCell` desenha não recebe eventos de ponteiro, então um `ui.Tooltip` dentro de uma célula nunca aparece. `base.Table` é uma tabela simples com as linhas do app para conjuntos menores de dados, e `base/plot` desenha gráficos.

Use `DataTable.LabelFunc` para um nome de grade que acompanhe a localidade ou o estado da aplicação. `ColumnGroup.LabelFunc` atualiza o título de um grupo de colunas enquanto o grupo permanece visível.

## Próximos passos

- [Animação](animation.md): efeitos que acompanham uma view de rolagem.
- [Texto](text.md): linhas de texto com quebra automática.
- [Navegação](navigation.md): telas cujo conteúdo rola.

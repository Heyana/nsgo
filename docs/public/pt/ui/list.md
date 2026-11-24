---
title: List
description: Linhas em qualquer quantidade, construídas apenas quando visíveis, com seleção e ativação.
group: Componentes
order: 38
---

`ui.List` renderiza linhas em uma view de rolagem nativa e constrói apenas as linhas próximas da tela, com seleção, ativação e o teclado.

```go preview=list/basic height=280
ui.List{
	Count:         func() int { return len(items) },
	Key:           func(i int) any { return items[i].ID },
	EstimatedSize: 44,
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		item := items[row.Index()]
		return ui.Text(item.Title, 13, ui.Regular, colors.Text)
	},
}
```

## Linhas e identidade

`Count` diz quantas linhas existem e `Row` constrói uma. `Key` dá identidade a uma linha: as linhas que permanecem mantêm suas células e seu estado enquanto linhas surgem e somem acima delas. Use um ID em vez do índice quando linhas puderem ser adicionadas, removidas ou reordenadas; o índice é um signal que segue a célula quando ela mostra outra linha.

```go
ui.List{Count: count, Key: func(i int) any { return items[i].ID }, Row: func(cx *core.Scope, row core.ListRow) core.Node {
	return ui.Text(items[row.Index()].Title, 13, ui.Regular, colors.Text)
}}
```

`EstimatedSize` é a altura esperada de uma linha, 50 pontos quando 0. As linhas podem ter alturas diferentes; a lista mede as que constrói e estima o resto.

## Layout

`Header`, `Footer` e `Empty` guardam conteúdo ao redor das linhas e enquanto não há nenhuma. `Gap` separa as linhas, `Columns` as coloca lado a lado, e o padding da lista é o do conteúdo, então a barra de rolagem fica na borda. Dê uma altura à lista ou deixe-a preencher seu pai.

## Seleção e ativação

`ui.List` recebe uma `Selection`, criada com `ui.NewSelection(cx, multiple)`. Chaves, rótulos, ativação e o teclado a seguem: Cima, Baixo, Home e End movem o cursor, Shift estende, Mod+A escolhe tudo, Espaço alterna em uma seleção múltipla, e Enter ou um clique duplo ativa uma linha.

```go preview=list/selection height=320
selected := ui.NewSelection(cx, true)
ui.List{
	Count: count, Key: func(i int) any { return items[i].ID },
	Selection: selected,
	OnActivate: func(i int) { open(items[i]) },
	RowLabel:   func(i int) string { return items[i].Title },
	Row: func(cx *core.Scope, row core.ListRow) core.Node { return rowContent(cx, items[row.Index()]) },
}
```

`OnSelectionChange` roda depois que um toque, uma tecla ou a digitação antecipada (typeahead) muda quais linhas estão escolhidas, e não quando o próprio aplicativo define a `Selection`, de modo que um painel de detalhes pode seguir a escolha do usuário enquanto o aplicativo move a seleção para um item aberto de outra forma.

`TapSelects` faz um toque em um celular selecionar em vez de ativar. `RowLabel` é o que a tecnologia assistiva lê e o que o typeahead compara.

## Carregamento e rolagem

`OnLoadMore` roda quando o fim se aproxima, uma vez por número de linhas. `Ref` rola com `ScrollToIndexAt`, `ScrollToEnd` e `ScrollToIndex`. `core.List`, a lista simples por baixo dela, também tem `OnLoadMoreAtStart`, `Inverted` para um chat, `StickyHeaderIndices`, `MaintainVisibleContentPosition`, `Viewport` e `ViewabilityConfig` para os casos de baixo nível ([Listas e rolagem](../lists.md)).

## Acessibilidade

A lista é uma única lista ou listbox, nomeada por `Label`, com o texto de uma linha ou `RowLabel` como nome dela. Uma lista com seleção relata a linha do cursor e a anuncia conforme o teclado se move. Um leitor de tela que se move para fora da área visível pede à lista que role até lá, o que constrói as linhas que ele alcança.

## Plataforma

A view de rolagem nativa e a virtualização são as mesmas em todos os alvos. As linhas são recicladas, então o estado de uma linha deve seguir sua `Key`; manter o estado de construção no aplicativo e não em uma linha é a regra usual.

## Relacionados

- [Listas e rolagem](../lists.md): o guia completo, incluindo cabeçalhos fixos e carregar mais.
- [Scroll](scroll.md): uma view de rolagem simples.
- [DataTable](../lists.md#tables): uma tabela com muitas linhas e colunas.
- As páginas List, Table e DataTable da galeria.

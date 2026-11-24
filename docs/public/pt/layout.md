---
title: Layout
description: Flexbox, grid, tamanhos e posicionamento, com a área segura e o teclado integrados ao layout.
group: Conceitos principais
order: 51
---

## Caixas

Todo elemento é uma caixa posicionada pelo motor. Os padrões são os mesmos do React Native: um flexbox em coluna, filhos que mantêm seu tamanho a menos que cresçam ou encolham, itens esticados ao longo do eixo transversal e tamanhos que incluem padding e bordas.

```go
core.View{
	Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.All(16), AlignItems: core.AlignCenter},
	Children: []core.Node{
		core.Text{Content: "One"},
		core.Text{Content: "Two"},
	},
}
```

`Column`, `Row`, `ColumnReverse` e `RowReverse` definem o eixo principal; `Wrap` e `WrapReverse` permitem que os filhos quebrem para novas linhas. `JustifyContent` os posiciona ao longo desse eixo e `AlignItems` — ou `AlignSelf` em um filho específico — no eixo transversal. `Gap`, `RowGap` e `ColumnGap` os separam, e o padding e a margem do `style` usam `core.All`, `core.Symmetric` ou `core.Edges`.

## Tamanhos

`Width`, `Height`, `MinWidth`, `MaxWidth` e `FlexBasis` aceitam `core.Pt` para pontos ou `core.Pct` para uma porcentagem do pai. `FlexGrow` distribui o espaço livre e `FlexShrink` permite que uma caixa ceda espaço; uma caixa sem nenhum dos dois mantém seu tamanho. `AspectRatio` é a largura dividida pela altura. `AutoMargin` absorve o espaço livre nos lados que especifica, de modo que um lado empurra a caixa para o outro.

```go
core.View{Style: core.Style{Direction: core.Row, Gap: 16}, Children: []core.Node{
	core.View{Style: core.Style{Width: core.Pt(220), FlexShrink: 0}},
	core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}},
}}
```

O conteúdo de uma caixa determina seu tamanho até que uma largura ou altura indique o contrário. `core.MinContent`, `core.MaxContent` e `core.FitContent` assumem os tamanhos homônimos do CSS: a largura `min-content` de um texto é sua palavra mais larga. Um texto dentro de uma linha só encolhe e quebra linha com `FlexShrink: 1`; adicione `FlexGrow: 1` quando ele precisar ocupar o espaço restante. Um mínimo não definido já é 0, como no React Native, então `MinWidth: core.Pt(0)` não muda nada e um texto que encolhe quebra suas palavras quando a linha fica mais estreita do que sua palavra mais larga. `MinWidth: core.AutoMinimum` interrompe o encolhimento na palavra mais larga, como faz o mínimo automático do CSS.

Uma tela que preenche a janela usa `FlexGrow: 1` e `FlexShrink: 1`, assim como cada caixa entre ela e uma lista ou scroll view que deva rolar. Apenas com `FlexGrow`, a caixa cresce até o tamanho do conteúdo, fazendo uma lista longa esticar além da janela em vez de rolar.

## Grid

`DisplayGrid` posiciona os filhos em CSS grid. `GridTemplateColumns` e `GridTemplateRows` aceitam a sintaxe de trilhas do CSS, e `GridRow` e `GridColumn` posicionam um item por linha ou extensão (`span`).

```go
core.View{Style: core.Style{
	Display: core.DisplayGrid,
	GridTemplateColumns: "repeat(3, 1fr)",
	GridAutoRows: "44px",
	Gap: 12,
}, Children: []core.Node{
	core.View{Style: core.Style{GridColumn: core.GridLines{Start: core.GridSpan(2)}}},
}}
```

`repeat(auto-fill, minmax(160px, 1fr))` cria tantas colunas quanto couberem. `GridFlow` define a ordem de preenchimento do grid, e `JustifyItems`, `JustifySelf` e `AlignItems` posicionam os itens em suas células.

## Posição

`Relative` é o padrão e mantém a caixa no fluxo; `Top`, `Right`, `Bottom` e `Left` a deslocam. `Absolute` a retira do fluxo e a posiciona em relação ao ancestral posicionado mais próximo. `Fixed` a posiciona na view raiz ou em seu modal, independentemente da rolagem, e `Sticky` a fixa dentro de seu pai na scroll view mais próxima: `Top` a partir do topo, ou `Bottom` a partir da base quando `Top` é auto (`Left` e `Right` na horizontal). Um rodapé com `Position: core.Sticky, Bottom: core.Pt(0)` permanece na parte inferior enquanto conteúdos longos rolam e segue logo após conteúdos curtos. `ZIndex` empilha caixas entre seus irmãos.

```go
core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Right: core.Pt(8)}}
```

`Static` mantém uma caixa no fluxo e ignora seus deslocamentos (`insets`), fazendo com que filhos com posicionamento absoluto encontrem um ancestral acima dela. `Overflow` recorta os filhos nos limites da caixa, e `DisplayNone` remove a caixa do layout.

As sombras externas e o contorno da própria caixa permanecem fora do recorte, como no CSS, no Android, Windows, Linux, web e Apple — nestas últimas, para qualquer tipo de view, incluindo cantos com raios diferentes e conteúdo rolável com zoom. Até que isso seja verificado em um iPhone físico, o `Bare` de `ui.Dialog` ainda coloca seu recorte em uma view interna com os mesmos cantos, recuada pela borda, o que os preserva em todos os alvos:

```go
core.View{Style: core.Style{CornerRadius: 16, Shadows: []core.BoxShadow{ui.Shadow(0, 8, 24, -6, ui.Black(0.18))}},
	Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, CornerRadius: 16, Overflow: core.Hidden}, Children: content}}}
```

## Direção e ambiente

`LayoutDirection` posiciona uma subárvore da esquerda para a direita ou da direita para a esquerda, sobrescrevendo a direção do app: `DirectionRTL` e `DirectionLTR` a definem, e o padrão herda do pai. Esquerda e direita em margens, paddings, bordas e deslocamentos invertem conforme a direção, funcionando como início e fim como no React Native. `PhysicalEdges` e `PhysicalBorders` mantêm os lados nomeados onde estão. `WritingDirection` ordena trechos de texto com direções mistas, e `WritingAuto` assume a direção a partir da primeira letra forte.

`PaddingEnv` e `MarginEnv` somam valores do ambiente a um lado. `core.EnvSafeArea` é o recuo da área segura naquela borda e `core.EnvKeyboard` é a altura do teclado virtual, mantendo o conteúdo acima do teclado enquanto seu movimento anima junto com ele.

```go
core.View{Style: core.Style{
	Padding: core.All(16),
	PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea},
}}
```

`core.EnvAll(core.EnvSafeArea)` aplica a todas as bordas, e `cx.Env().SafeArea()` e `cx.Env().KeyboardHeight()` leem os valores no código.

Em um telefone dobrável ou de tela dupla, `cx.Env().Geometry()` lista as regiões nas quais uma dobra ou dobradiça divide a janela, cada uma com a área segura correspondente e as dobras entre elas; seu `State` e `Angle` indicam o quanto o dispositivo está aberto (`core.HingeClosed`, `HingeHalfOpen` ou `HingeOpen`), mesmo quando nenhuma dobra cruza a janela, como em uma tela externa ou uma tela totalmente aberta. Outros dispositivos relatam uma região e `HingeUnknown`. No iOS 27.1, `cx.Env().VerticalBarEdge()` informa a borda escolhida pelo sistema para uma barra vertical (`core.VerticalBarLeading`, `core.VerticalBarTrailing` ou `core.VerticalBarUnspecified`), que `mobile.TabBar` e `ui.TabShell` seguem quando `Edge` é `mobile.TabBarAutomatic`. Nada é disposto pela geometria de forma implícita: o app escolhe seus próprios painéis.

## Classes responsivas

`Class` aceita classes do Tailwind CSS: `flex`, `flex-col`, `flex-wrap`, `items-center`, `justify-between`, `gap-3`, `p-4`, `w-full`, `grid`, `grid-cols-3`, `col-span-2` e as demais. Breakpoints acompanham a janela, enquanto a aparência da tela, a direção e a redução de movimento acompanham o ambiente:

```go
core.View{Class: "flex flex-col gap-3 p-4 md:flex-row md:gap-6 dark:bg-zinc-900 rtl:flex-row-reverse"}
```

`sm:`, `md:`, `lg:`, `xl:` e `2xl:` se aplicam a partir de 40, 48, 64, 80 e 96 rems de largura; `max-md:` se aplica abaixo de um breakpoint. `portrait:`, `dark:`, `rtl:`, `motion-reduce:`, `contrast-more:`, `pointer-fine:`, `pointer-coarse:`, `ios:`, `android:` e `web:` seguem o que o ambiente informa. As classes são mescladas abaixo de um `Style`, e a classe de um estado (`hover:`) torna-se um estilo de estado, permitindo combinar ambos.

## Próximos passos

- [Estilos e temas](styles.md): cores, cantos, sombras e o visual de uma subárvore.
- [Texto](text.md): como o texto assume seu tamanho e quebra linhas.
- [Listas e rolagem](lists.md): conteúdo rolável e linhas recicladas.

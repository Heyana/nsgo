---
title: Scroll
description: A view de rolagem da plataforma, com paginação, encaixe, zoom e atualização.
group: Componentes
order: 39
---

`core.Scroll` é a própria view de rolagem da plataforma. Seus filhos são dispostos em uma coluna, ou em uma linha com `Horizontal`, e ela preenche seu contêiner enquanto o conteúdo rola.

```go preview=scroll/basic height=240
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

O padding vai no conteúdo, não na view de rolagem, para que a barra de rolagem fique encostada na borda. A view de rolagem encolhe para caber no seu pai, a menos que seu estilo diga o contrário. Defina `NoShrink` quando ela precisar manter o tamanho solicitado enquanto um irmão encolhe.

## Comportamento

`Behavior` escolhe como ela rola: `Paging`, `SnapToInterval` ou `SnapToOffsets` com `SnapToAlignment`, uma `DecelerationRate`, `NoBounces`, `ContentInset` e as regras do teclado.

```go preview=scroll/paging height=240
core.Scroll{
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: pages,
}
```

`HideIndicator` oculta a barra de rolagem da plataforma e `Bar` desenha a do próprio aplicativo. `MinZoomScale` e `MaxZoomScale` permitem que pinças façam zoom; `NoPinchZoom` as ignora. `CenterContent` centraliza um conteúdo menor que a área visível.

## Atualização e eventos

`OnRefresh` torna a view atualizável; defina `Refreshing` enquanto o trabalho roda. `PullValue` segue o quanto o conteúdo é puxado, para um indicador próprio do aplicativo, e `Behavior.RefreshIndicator` define sua altura.

```go
refreshing := core.NewSignal(cx, false)
core.Scroll{
	Refreshing: refreshing.Get,
	OnRefresh:  func() { refreshing.Set(true); reload(cx) },
	Children:   content,
}
```

`OnScroll` relata o deslocamento no máximo uma vez por ciclo, ou uma vez por `ScrollEventThrottle`; `Stale` marca um de antes de a plataforma aplicar uma rolagem feita por código. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd` e `OnScrollEnd` delimitam os gestos do usuário.

`OffsetValues` segue o deslocamento na thread principal, para um cabeçalho que recolhe sem um commit por quadro. `Ref` rola por código com `ScrollTo`, `ScrollToEnd` e `ZoomTo`, e o ref de qualquer view interna a traz para a área visível com `Reveal`, rolando todas as views de rolagem ao redor dela:

```go
row.Reveal(core.RevealOptions{Vertical: core.ScrollAlignCenter, Animated: true})
```

`RevealBox` traz para a área visível uma caixa dentro da view, como a caixa de uma palavra obtida de `TextRangeBoxes` de um texto, e `Margin` mantém espaço livre nas bordas da view de rolagem mais externa para uma barra sobre ela.

Uma view para a qual o teclado move o foco rola sozinha para a área visível.

## Layout

`Horizontal` rola na horizontal. `Behavior.ContentInset` coloca, ao redor do conteúdo, um espaço que rola com ele. Para uma página que preenche a área segura, use `PaddingEnv` no conteúdo, não na view de rolagem.

## Acessibilidade

`Label` nomeia a área de rolagem e `Role` diz o que ela é, como uma list box. Os gestos de rolagem da plataforma, a rolagem pelo teclado e a rolagem do leitor de tela são mantidos; `Busy` relata uma view que ainda está carregando.

## Plataforma

Todo backend usa a view de rolagem da plataforma, então os arremessos, os ricochetes, as barras de rolagem e a rolagem por toque são nativos. Os efeitos de borda diferem: macOS e iOS fazem o efeito elástico, o Android estica, Windows e Linux param, e a web segue o navegador. `EdgeEffects` e `EdgeBar` adicionam barras sobre o conteúdo onde o aplicativo quiser. Em celulares e tablets iOS e Android, views de rolagem verticais e horizontais que transbordam mostram uma alça de rolagem arrastável por toque ao longo do eixo ativo, que percorre todo o intervalo do conteúdo; views de rolagem curtas que cabem sem transbordar, views com zoom, `Behavior.NoScroll`, `HideIndicator` e views que usam uma `Bar` personalizada ou `base.SetScrollbarTheme` a ocultam. Nos testes, `nsgotest.App.ScrollHandle` relata os limites da sua área visível e `DragAt` ou `Touch` a arrastam diretamente. No Windows, o indicador sobreposto aparece para a roda, o toque, o arrasto da barra e o momentum deles, não para `ScrollTo`, `ScrollBy`, revelações, insets ou animações controlados pelo aplicativo.

## Relacionados

- [Listas e rolagem](../lists.md): o guia completo de rolagem e listas.
- [List](list.md): linhas que são recicladas.
- [Layout](../layout.md): paddings e a área segura.
- As páginas Scroll e Lists da galeria.

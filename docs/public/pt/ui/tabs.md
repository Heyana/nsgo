---
title: Abas
description: Uma lista de abas, um controle segmentado e a barra de abas do sistema.
group: Componentes
order: 40
---

`ui.Tabs[T]` escolhe um entre alguns valores, como um controle segmentado ou uma lista de abas; o aplicativo mostra o conteúdo do valor escolhido ao lado. Uma lista de abas é uma única parada de Tab: as setas percorrem as abas.

```go preview=tabs/basic
section := core.NewSignal(cx, "overview")
ui.Tabs[string]{
	Options: []ui.Option[string]{
		{Value: "overview", Label: "Overview"},
		{Value: "activity", Label: "Activity"},
		{Value: "settings", Label: "Settings"},
	},
	Value:    section.Get,
	OnChange: section.Set,
	Label:    "Sections",
}
```

## Variantes e tamanhos

`Variant` é `TabsSegmented` (o padrão, um indicador elevado em uma trilha rebaixada) ou `TabsUnderline` (uma linha de destaque sob a aba selecionada). Um sublinhado pode definir `IndicatorColor` e usar `IndicatorFit: TabsIndicatorContent` para acompanhar seu rótulo e ícone em vez da aba inteira; em TSX, eles são `indicatorColor` e `indicatorFit="content"`. `Size` é `SizeDefault`, `SizeSmall` ou `SizeLarge`, e `Equal` dá a todas as abas a mesma largura em vez de ajustá-las ao rótulo.

```go preview=tabs/underline
ui.Tabs[string]{Variant: ui.TabsUnderline, Equal: true, Size: ui.SizeSmall, Options: tabs, Value: section.Get, OnChange: section.Set}
```

Uma opção pode ter um `Icon`, uma função `Disabled`, um nó `Content` e um `TestID`. `Content` substitui o rótulo e herda o estilo de texto da aba.

Use `OptionsFunc` quando as abas seguirem um signal. O NSGo atualiza as linhas no lugar pelos seus valores, mantendo a aba selecionada e o foco do teclado quando esse valor continua sendo oferecido; um valor que desaparece fica sem seleção até que o aplicativo o altere.

## Barras de abas

`ui.TabShell` mantém as seções de nível superior ao lado de uma barra de abas em qualquer largura de janela — embaixo por padrão, ou verticalmente ao longo da borda inicial ou final quando `Edge` (ou `EdgeFunc`) é `mobile.TabBarLeading`, `mobile.TabBarTrailing` ou `mobile.TabBarAutomatic` (que segue `cx.Env().VerticalBarEdge()`). Ele usa a barra do sistema quando todas as abas fornecem `SystemIcon` ou um `Icon` SVG, em qualquer borda no iOS e na borda inferior no Android; um `SystemIcon` que nomeia um SF Symbol no iOS ou um drawable no Android tem precedência, e caso contrário o SVG é exibido. Caso contrário, ele desenha a barra com os ícones SVG fornecidos. Uma barra que ocupa espaço próprio se mantém afastada da área segura da sua borda por conta própria, então `core.EnvSafeArea` nessa borda de uma seção ao lado dela, incluindo uma página de `base.Nav`, é 0; sob a barra de sistema flutuante inferior do iOS, mantenha o conteúdo afastado com `ui.TabBarInset`.

Defina `ShellTab.TextFunc` para títulos de abas que seguem o idioma ou o estado do aplicativo e `TabShell.LabelFunc` para o nome acessível da barra. Mudar um título preserva a seção selecionada e a aba existente, desde que seu ícone continue o mesmo.

`mobile.TabBar` é a barra de abas do sistema no iOS e no Android, com `Items`, `Selected`, `OnChange` e, opcionalmente, `Edge` (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing` ou `mobile.TabBarAutomatic`) / `EdgeFunc`. Cada `mobile.TabBarItem` aceita um `Title` e um nome de ícone da plataforma em `Icon`; `SVG: lucide.House()` é exibido sempre que `Icon` não nomeia um SF Symbol no iOS ou um drawable no Android, ajustado a 24 pontos e colorido conforme a seleção. Em TSX, `svg` aceita um documento SVG e `edge` aceita `"bottom"`, `"leading"`, `"trailing"` ou `"automatic"`. Em outros lugares, `mobile.TabBar` não ocupa espaço, então mostre `ui.Tabs` ali.

`ui.SidebarShell` mantém o que sua barra lateral, rodapé, cabeçalhos, conteúdo e painel direito contêm afastado das áreas seguras esquerda e direita por conta própria, com seus fundos se estendendo até as bordas da janela, então `core.EnvSafeArea` nesses lados é 0 dentro deles, e `ui.NavHeader` adiciona a área segura lateral ao seu padding. O conteúdo de uma `ui.NavPage` ou de uma seção de `TabShell` se mantém afastado da área segura por conta própria, com `PaddingEnv`, para que o que deve ir até as bordas, como uma imagem de sangria total, possa ir.

## Acessibilidade

A lista relata `RoleTabList`, cada aba `RoleTab`, e a aba selecionada seu estado. A lista é uma única parada de Tab; Esquerda e Cima, Direita e Baixo, Home e End movem entre as abas habilitadas, e a aba alcançada relata um toque. `Label` nomeia a lista.

## Plataforma

iOS e Android mostram a barra de abas do sistema com sua própria aparência e gestos; macOS, Windows, Linux e a web mostram `ui.Tabs`. O conteúdo do aplicativo é o mesmo em todos os alvos.

## Relacionados

- [Navegação](../navigation.md): páginas e barras de abas em um aplicativo.
- [Menu](menu.md): comandos em vez de seções.
- [Select](select.md): muitas opções em uma lista.
- As páginas Tabs e NavStack da galeria.
- [Componentes sem estilo](../authoring/base.md#abas): `base.Tabs`, para abas com uma aparência própria.

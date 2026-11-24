---
title: Navegação
description: Pilhas de navegação nativas, cabeçalhos do sistema, modais, menus e barras de abas.
group: Conceitos principais
order: 56
---

## Uma pilha

`base.NewNav` mantém uma pilha de rotas de um tipo: `Push` adiciona uma página, `Pop` remove a página do topo, `PopToRoot` volta à primeira e `Replace` troca a do topo. Seu `Stack` renderiza cada rota como uma página, mantendo as páginas de baixo montadas com seu estado enquanto a do topo aparece. `ui.NavPage` dá a cada página o fundo de `ui` e um `ui.NavHeader` com seu título, botão de voltar e botões.

O watchOS não tem barra de navegação, barra de abas nem interruptor do sistema, então lá `ui.NavPage` desenha seu `ui.NavHeader` acima da página, com o botão de voltar, `ui.TabShell` desenha sua própria barra de abas e `ui.SwitchSystem` é o interruptor próprio de `ui`.

```go
nav := base.NewNav(cx, "inbox")

return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, depth func() int) base.NavPage {
	if route == "inbox" {
		return ui.NavPage(cx, base.NavPage{Title: "Inbox", Children: []core.Node{
			ui.Button{Text: "Open a message", OnPress: func() { nav.Push("message") }},
		}}, ui.NavHeader{})
	}
	return ui.NavPage(cx, base.NavPage{Title: "Message"}, ui.NavHeader{
		OnBack: func() { nav.Pop(1) }, BackLabel: "Inbox",
		Trailing: []core.Node{ui.IconButton{Icon: lucide.Archive, Label: "Archive", Variant: ui.IconButtonGhost, OnPress: archive}},
	})
})
```

No iOS e no Android a plataforma desenha o cabeçalho do sistema a partir do título da página e dos botões do cabeçalho, com seu botão de voltar e sua transição; no macOS, Windows, Linux e na web, `ui` o desenha.

O Android usa uma barra de app superior do Material 3. Uma barra comum tem 64 dp com um título de 22 sp. `HeaderLargeTitle` dá uma barra grande de 152 dp com um título de 28 sp que se recolhe na barra comum à medida que a visualização de rolagem no topo da página rola seu conteúdo por baixo dela, e se expande quando o conteúdo volta ao topo.

Defina `HeaderTitleStyle` e `HeaderLargeTitleStyle` para estilizar as fontes nativas do título compacto e do expandido no iOS e no Android: `FontSize`, `FontWeight`, `FontFamily` e `FontStyle`. Campos não definidos mantêm os padrões de cada plataforma; `HeaderTitleStyleFunc` e `HeaderLargeTitleStyleFunc` acompanham as mudanças. `HeaderStyle` continua fornecendo o fundo, a cor do título e a tonalidade dos botões. Em TSX, esses são os objetos ou arrays de estilo `headerTitleStyle` e `headerLargeTitleStyle`.

Um `HeaderTitle` personalizado substitui apenas o título compacto no Android; `Title` continua sendo o título grande expandido. Defina `Android: core.AndroidScreenProps{HeaderRightWithLargeTitle: true}` para colocar `HeaderRight` ao lado desse título grande. Seus controles passam para a barra de ferramentas fixa à medida que o título se recolhe e continuam pressionáveis o tempo todo. TSX usa `android={{ headerRightWithLargeTitle: true }}`. A faixa grande cresce quando sua fonte precisa de mais espaço. Builds Android independentes ativam o layout RTL nativo; um host incorporador deve definir `android:supportsRtl="true"` em sua application para que os cabeçalhos nativos sigam a direção do aplicativo.

Dê a uma página seu próprio `Header` para substituir `ui.NavHeader`. No iOS e no Android ele se torna a visualização de título do cabeçalho do sistema, no lugar de `HeaderTitle`, ao lado de `HeaderLeft`, `HeaderRight` e do botão de voltar da página; ele preenche o espaço entre essas extremidades quando suas visualizações crescem ou ocupam uma largura percentual, como um campo de busca, e assume o tamanho do seu conteúdo nos demais casos. Em outros lugares, ele é desenhado acima do conteúdo. `ui.NavHeader.Scroll` continua conectando a rolagem de uma página do iOS ou do Android ao desfoque do seu cabeçalho.

`NoPopOnBack` mantém uma página quando voltar é pressionado, `OnBackPress` ouve o toque e `NoPopOnSwipe` desativa o gesto de deslizar para voltar do iOS. `cx.OnBackPress(func() bool)` dá ao aplicativo um handler antes da pilha, para alterações não salvas. `OnDismiss` informa que o usuário removeu a página, e a pilha a remove.

## Tela cheia imersiva

Chame `mobile.Immersive(cx, true)` para solicitar tela cheia para a janela e `mobile.Immersive(cx, false)` para voltar às suas configurações normais da barra do sistema. Para uma página ou um diálogo, use `StatusBar: core.StatusBarSetting{Immersive: core.ToggleOn}` para solicitá-la enquanto essa camada é exibida. Em TSX, use `statusBar={{ immersive: true }}` em uma página ou diálogo, ou `status_bar: { immersive: true }` em `nsgo.yaml` para o padrão da janela.

No Android, um deslize a partir da borda mostra brevemente as barras de status e de navegação. No HarmonyOS, a janela pede ao sistema que oculte a barra de status, a barra de navegação de três botões e o indicador de navegação. No iOS a barra de status se oculta e o UIKit decide quando ocultar ou revelar o indicador de início. `cx.Env().SafeArea()` continua informando as margens que o sistema fornece, incluindo os recortes da tela. No HarmonyOS com navegação de três botões em paisagem, o sistema informa a área dos botões ao longo da borda inferior enquanto os desenha ao longo de uma lateral, então a área segura pode deixar espaço embaixo enquanto os botões cobrem a lateral.

## Modais

`ui.Dialog` mostra uma superfície sobre o aplicativo escurecido, uma folha vinda de baixo em telas sensíveis ao toque, e `ui.Sheet` um painel vindo de baixo. Cada um fecha com Escape e com um toque no seu fundo, a menos que seja instruído a não fazê-lo.

```go
ui.Dialog{
	Open:         dialog.Get,
	OnOpenChange: func(open bool, _ base.Reason) { dialog.Set(open) },
	Title:        "Rename",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { dialog.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

`ui.AlertDialog` pede ao usuário que confirme, e `cx.ShowAlert` mostra o próprio alerta ou a própria folha de ações do sistema ([Dialog](ui/dialog.md)).

## Menus

`ui.Menu` abre `ui.MenuItem`s a partir de um botão: ao lado dele, ou como uma folha vinda de baixo quando seu `Presentation` pede, em qualquer dispositivo. `ui.ContextMenu` faz o mesmo para um clique secundário ou um toque longo, e `ui.Popover`, `ui.HoverCard` e `ui.Tooltip` mostram outros conteúdos ancorados.

```go
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", OnSelect: duplicate},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Abas

`ui.Tabs[T]` é um controle segmentado ou uma lista de abas, com um indicador que desliza até a opção escolhida; `TabStyle` é mesclado sobre o estilo de cada aba, seu `Selected` sobre o da aba escolhida, e `IndicatorStyle` sobre o do indicador. Abas sublinhadas podem definir `IndicatorColor` e ajustar a linha ao seu conteúdo com `IndicatorFit: TabsIndicatorContent`. `ui.Sidebar` guarda seções e linhas para uma barra lateral de desktop, e `ui.TabShell` mantém as seções de nível superior ao lado de uma barra de abas (embaixo por padrão, ou vertical na borda inicial ou final com `Edge` / `EdgeFunc`, incluindo `mobile.TabBarAutomatic` para seguir `cx.Env().VerticalBarEdge()`) em qualquer largura de janela. Ele usa a barra do sistema quando cada aba tem um `SystemIcon` ou um `Icon` em SVG, em qualquer borda no iOS e na borda inferior no Android, mostrando o SVG sempre que `SystemIcon` não nomeia nenhum SF Symbol no iOS nem drawable no Android; caso contrário, ele desenha a barra com os ícones SVG. Defina `BarHidden` enquanto uma página de detalhe empilhada estiver visível para ocultar qualquer uma das barras e restaurar a área segura da seção; `TabBarInset` passa a ser 0 enquanto ela está oculta ou posicionada na vertical.

`mobile.TabBar` é a barra de abas do sistema no iOS e no Android, com sua aparência Liquid Glass ou Material; ela recebe `Items` de `mobile.TabBarItem{Title, Icon, SVG}`, o índice selecionado, `OnChange` e um `Edge` opcional (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing` ou `mobile.TabBarAutomatic`, que segue `cx.Env().VerticalBarEdge()` no iOS 27.1 e recorre à borda inferior em outros lugares) ou um `EdgeFunc` reativo. Os papéis de paleta do Android `tabBarBackground`, `tabBarIdle`, `tabBarSelected` e `tabBarIndicator` definem o fundo da barra, os ícones e rótulos inativos, o ícone e o rótulo selecionados e a pílula selecionada. Paletas personalizadas recorrem a `Surface`, `TextMuted`, `Accent` e `accentContainer`, respectivamente, com uma pílula de 12% da cor de destaque quando `accentContainer` não está definido. A paleta padrão mantém o fundo e as cores inativas do Material, com a aba selecionada tingida pela cor de destaque. Em outros lugares ela não ocupa espaço, então um aplicativo mostra `ui.Tabs` ali. `mobile.IsAvailable(cx)` diz se a barra aparece. `Icon` nomeia um ícone da plataforma; `SVG`, como `lucide.House()`, aparece no iOS e no Android sempre que `Icon` não nomeia nenhum SF Symbol ou drawable, tingido pela barra como um ícone do sistema é.

Defina `TabBarItem.Badge` como `mobile.TabBadge{Text: "3"}` para texto ou `mobile.TabBadge{Dot: true}` para um ponto. Atualize-o por meio de `ItemsFunc` e retorne o valor zero para limpá-lo. `ui.ShellTab.Badge` recebe `func() mobile.TabBadge` tanto para a barra do sistema quanto para a barra desenhada. O texto tem precedência quando os dois campos estão definidos. O UIKit mostra um badge nativo em branco para um ponto. A tecnologia assistiva lê o texto do badge ou a palavra localizada para conteúdo novo. TSX recebe `badge: { text: "3" }` ou `badge: { dot: true }` em um item nativo, e uma função que retorna esse valor em um item de `TabShell`.

```go
mobile.TabBar{
	Items: []mobile.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}},
	Selected: func() int { return tab.Get() },
	OnChange: func(i int) { tab.Set(i) },
}
```

## Próximos passos

- [Formulários](forms.md): o que uma página de uma pilha contém.
- [Animação](animation.md): como as páginas e os modais se movem.
- [Layout](layout.md): a área segura em torno de um cabeçalho.

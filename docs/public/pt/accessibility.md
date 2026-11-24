---
title: Acessibilidade
description: Rótulos, papéis, valores, ações e foco para leitores de tela e teclados.
group: Conceitos principais
order: 61
---

## O que um leitor de tela lê

O `Label` de uma visualização é o que a tecnologia assistiva diz, `AccessibilityDescription` traz mais informações sobre ela, lida após o rótulo, `Hint` descreve o que a ativação faz e `Role` define o que ela é: `core.RoleButton`, `RoleLink`, `RoleHeader`, `RoleImage`, `RoleCheckbox`, `RoleSwitch`, `RoleRadio`, `RoleTab`, `RoleTextField`, `RoleSearchField`, `RoleStatus`, `RoleMenu`, `RoleMenuItem`, `RoleDialog`, `RoleAlertDialog`, `RoleAlert`, `RoleAdjustable`, `RoleProgressBar`, `RoleTable` e os demais. Um status é uma região ativa educada (`polite`) por padrão; um campo de busca é um campo de texto anunciado como caixa de busca. Um diálogo é lido com seu papel e rótulo ao abrir, e sua descrição em seguida, assim como o título e a descrição de `ui.Dialog`. Um texto sem rótulo é lido pelo seu conteúdo. `LabelFunc`, `HintFunc` e `AccessibilityDescriptionFunc` vinculam texto de acessibilidade dinâmico na mesma visualização, de modo que os controles mantêm o foco e o estado durante uma troca de idioma. Todos os elementos de `core` com um rótulo aceitam `LabelFunc`, e `View` e `Text` aceitam `HintFunc`.

Use `TextFunc` para texto dinâmico em `ui.Button`, `ui.Checkbox`, `ui.MenuItem` e `ui.AlertAction`. Seus nomes acessíveis seguem esse texto. Botões e caixas de seleção também aceitam um `LabelFunc` separado. Controles deslizantes, abas, interruptores, barras de progresso e gatilhos de pop-up aceitam `LabelFunc` para um nome acessível dinâmico. `ui.Option.LabelFunc` fornece o texto da opção em abas, selects e comboboxes. Entradas numéricas e de código, anéis de progresso e ambas as variantes de spinner também aceitam `LabelFunc`.

```go
core.View{
	Role:  core.RoleButton,
	Label: "Add to cart",
	Hint:  "Adds the item and opens the cart",
	OnPress: add,
	Children: []core.Node{core.Icon{SVG: lucide.ShoppingCart}},
}
```

`AccessibilityValue` fornece um intervalo ou um valor textual: o mínimo, o máximo e o número atual de um controle deslizante, ou um texto como "3 itens". `Accessible` torna uma visualização um único elemento, para que o nome e o preço de um cartão sejam lidos juntos em vez de em duas paradas, e sem um rótulo a visualização lê o texto de seus descendentes. `AccessibilityElementsHidden` oculta decorações, como uma seta ou um separador, enquanto `ImportantForAccessibility` oculta toda uma subárvore.

```go
core.View{Accessible: true, Label: "Apples, $1.20 a kilo", Children: []core.Node{
	core.Text{Content: "Apples"},
	core.Text{Content: "$1.20 a kilo"},
}}
```

## Rótulos visíveis

Passe o `Ref` do controle como `LabelFor` para um `core.Text` ou `core.View`. Pressionar o rótulo foca o controle e executa sua ação de pressionamento, como alternar uma caixa de seleção. Seu texto visível atual nomeia o controle sem repetir uma string `Label`, e ele acompanha os estados `Disabled` e `Invalid` do controle para acessibilidade e estilos. Um rótulo não ganha papel de botão nem parada de Tab.

```go
field := new(core.Ref)
return core.View{Children: []core.Node{
	core.Text{Content: "Email", LabelFor: field},
	base.Input{Ref: field},
}}
```

`LabelledBy: []*core.Ref{label, requirement}` nomeia um controle a partir de vários elementos visíveis na ordem fornecida, com prioridade sobre `Label`; ele não encaminha pressionamentos. Caso contrário, `Label` prevalece sobre nomes implícitos de `LabelFor`, cuja ordem segue a árvore. Atualizações de texto, movimentações, ocultação, remoção e remontagem atualizam a relação. Referências ausentes, repetidas, cíclicas e ocultas são ignoradas; associações permanecem dentro de uma mesma superfície e escopo de confinamento. Um rótulo não pode ativar um controle atrás de um modal.

Botões, caixas de seleção, botões de opção, interruptores, selects e entradas numéricas de `base` e `ui` repassam `Ref` e `LabelledBy` ao seu controle; controles deslizantes expõem `ThumbRef` e `StartThumbRef`. Em TSX, use `labelFor={field()}` e `labelledBy={[label()]}` com sinais de nó preenchidos por `ref={node => field.set(node)}`, para que um rótulo antes de seu campo seja atualizado quando esse campo for montado.

## Ações

`AccessibilityActions` lista o que o leitor de tela oferece em um elemento: as ações padrão (`core.ActionActivate`, `ActionIncrement`, `ActionDecrement`, `ActionEscape`, `ActionMagicTap`) e as do próprio aplicativo, cada uma com um `Name` e um `Label`. `OnAccessibilityAction` recebe o nome. Use `AccessibilityActionsFunc` em `core.View` ou `core.Input` para alterar as ações disponíveis ou traduzir seus rótulos enquanto o elemento permanece montado.

```go
core.View{
	Role: core.RoleAdjustable,
	Label: "Quantity",
	AccessibilityValue: func() core.AccessibilityValue {
		return core.AccessibilityValue{Max: 10, Now: float32(n.Get()), Text: fmt.Sprintf("%d items", n.Get())}
	},
	AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}},
	OnAccessibilityAction: func(name string) {
		if name == core.ActionIncrement {
			n.Update(func(v int) int { return v + 1 })
		} else {
			n.Update(func(v int) int { return v - 1 })
		}
	},
	Children: quantityControls,
}
```

`MultiSelectable` marca uma grade, caixa de listagem ou árvore que permite mais de um item selecionado. `base.DataTable` o define para seleção de múltiplas linhas; a web, o Windows e o Linux expõem a flag de acessibilidade correspondente.

`Expanded` indica se um painel expansível ou um botão de menu está aberto, `Busy` indica que o conteúdo está carregando, `Invalid` indica que o aplicativo rejeitou o conteúdo de um campo ou o valor de um controle (estilizado com `aria-invalid:`), e `Checked`, `Selected` e `Pressed` informam o estado de um controle. As partes de um controle podem assumir seu estado por meio de `PartStates`, para que um indicador acompanhe sua caixa de seleção sem ser uma parada própria.

`SelectionItem` marca uma visualização como um item em uma coleção selecionável, independentemente de seu valor atual de `Selected`. `base.DataTable` o define em linhas ou células quando essa tabela as seleciona, e `base.Tree` o define em todas as linhas; o Windows mantém o padrão SelectionItem e a web escreve `aria-selected="true"` ou `"false"`. Uma tabela sem seleção deixa as linhas sem nenhum dos marcadores.

## Alterações ao vivo

`LiveRegion` faz com que leitores de tela anunciem o rótulo de uma visualização e o texto dentro dela conforme mudam: `core.LiveRegionPolite` aguarda o usuário ficar ocioso, enquanto `LiveRegionAssertive` interrompe. `cx.Announce` pronuncia uma mensagem uma única vez.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("%d items in the basket", count.Get()) },
	LiveRegion: core.LiveRegionPolite}
```

## Foco e teclado

Tooltips descrevem seu gatilho antes que ele receba foco. `View.Tooltip`, `base.Tooltip` e `ui.Tooltip` usam seu texto atual como a descrição de acessibilidade do controle, a menos que ele repita o nome do controle, o aplicativo forneça `AccessibilityDescription` ou o tooltip esteja `Disabled`. Um wrapper repassa a descrição aos seus controles. A bolha e seus filhos ficam ocultos dos leitores de tela e nunca anunciam um alerta; o tooltip padrão de `ui.IconButton`, portanto, não repete seu rótulo.

Um `base.DataTable` expõe sua grade nomeada como a parada de foco do teclado e da acessibilidade; a camada de clique do ponteiro fica oculta da tecnologia assistiva.

Em desktops, na web e em uma Apple TV, visualizações que aceitam pressionamento são paradas de Tab ao lado de campos de texto; `NoFocus` mantém uma de fora e `Focusable` torna uma visualização sem pressionamento uma parada. `Style.FocusVisible` desenha o anel de foco apenas quando o teclado moveu o foco (`focus-visible:`), `Style.Focus` em qualquer foco e `Style.FocusWithin` quando o foco está dentro de uma visualização; `OnFocus` e `OnBlur` o escutam, `OnBlur` também quando a visualização focada deixa a árvore, e `Ref.Focus()` e `Ref.SetAccessibilityFocus()` o movem; `core.FocusedRef(cx)` é um ref para a visualização que possui o foco, útil para o que abre sem um gatilho — como uma barra de busca aberta por uma tecla — devolvê-lo ao fechar. Return e Space pressionam a visualização focada, e as teclas de seta navegam por listas de abas, grupos de botões de opção e menus. O `Tooltip` de uma visualização aparece imediatamente quando o teclado move o foco para ela, até que o foco saia ou Escape seja pressionado. O `OnOpen` de um pop-up é executado assim que seu conteúdo está presente, portanto um `Focus` ali move o foco para dentro, como deve acontecer em um diálogo não modal; o conteúdo de um popover segue seu gatilho na ordem do Tab, e o foco volta para o gatilho quando o pop-up fecha.

No Android, iPhone e iPad, `Ref.Focus()` e o `AutoFocus` de um botão podem focar uma visualização pressionável habilitada ou `Focusable` para um teclado físico. Tocar em um controle não move esse foco. Space e Return ativam o elemento pressionável focado após o mapa de teclas, preservando Control, Alt, Shift e a tecla de plataforma em cada fase do pressionamento. `NoFocus`, controles desabilitados ou ocultos e controles atrás de um modal não podem receber esse foco. Focar uma visualização não abre teclado virtual; campos nativos mantêm seu comportamento de edição. watchOS e miniprogramas não possuem caminho de ativação por teclado físico.

`cx.Env().ScreenReader()` informa se há um leitor de tela ativo, `cx.Env().ReduceMotion()` se o usuário solicitou menos movimento, `cx.Env().IncreaseContrast()` se o usuário solicitou mais contraste (Aumentar Contraste no macOS, iOS e watchOS, um tema de contraste do Windows, Alto Contraste do GNOME, a configuração de contraste do GTK 4.20 ou um tema GTK de alto contraste, a configuração de contraste acima do padrão do Android 14, `prefers-contrast: more` ou `forced-colors: active` na web), `cx.Env().FontScale()` informa o tamanho do texto e `cx.Env().RTL()` a direção do layout. Os textos seguem o tamanho do texto, a menos que `NoFontScaling` esteja definido, e um layout da direita para a esquerda espelha suas bordas físicas quando usa as bordas lógicas.

## Próximos passos

- [Estilos e temas](styles.md): estados de foco e desabilitado.
- [Formulários](forms.md): rótulos e valores para campos.
- [Canvas e shaders](canvas.md): expondo as partes de um desenho.

---
title: Componentes sem estilo
description: Os controles próprios do NSGo sem visual definido, para estilizar parte por parte ou servir de base para uma biblioteca estilizada.
group: Escrevendo componentes
order: 73
---

`base` contém todos os controles que `ui` possui, com seu comportamento, estado e acessibilidade, mas sem visual. Cada parte aceita um `Class`, um `Style` e um `StyleFunc` ([Partes e estados](parts.md)), de modo que uma biblioteca ou um aplicativo com design próprio os utiliza no lugar de `ui`, ou os envolve como `ui` faz ([Visuais e temas](looks.md)). Esta página aborda os controles da [referência de componentes](../ui.md); a galeria mostra cada um ao lado de sua forma em `ui`.

Controles como `Button`, `Checkbox`, `Select`, `Slider` e `NumberInput` aceitam `Invalid: rejected.Get` para acompanhar um sinal de validação. Estilize esse estado com `Class: "aria-invalid:border-red-500"` ou `Style.AriaInvalid`. Isso atualiza a acessibilidade e os estilos sem desabilitar o controle. Campos compostos repassam o estado para sua entrada real e partes decorativas; `ColorPicker` o compartilha com seus controles.

Para valores da aplicação arrastados entre visualizações, use [`base/dragdrop`](../drag-drop.md): uma área conecta origens e destinos enquanto o aplicativo fornece seu conteúdo, prévia e aparência.

## Botão

`base.Button` obtém seu visual a partir de `Style` e seu conteúdo a partir de `Children`. `OnPress` é executado em um pressionamento concluído; `OnPressDown` e `OnPressUp` acompanham o início e o fim de um pressionamento, para feedback próprio.

```go
base.Button{
	Label:   "Save",
	OnPress: func(core.PressEvent) { save() },
	Style: core.Style{Background: core.RGB(0x007aff), Padding: core.Symmetric(8, 14), CornerRadius: 8, AlignSelf: core.AlignStart,
		Active: &core.Style{Opacity: core.Alpha(0.85)}},
	Children: []core.Node{core.Text{Content: "Save", Style: core.Style{Color: core.RGB(0xffffff)}}},
}
```

`Disabled`, `Busy`, `Selected` e `Expanded` informam um estado ao aplicativo, ao estilo e à tecnologia assistiva: uma ação ainda em execução usa `Busy` (estilizado com `aria-busy:`), uma alternância ou o item atual de uma lista usa `Selected`, e um painel expansível usa `Expanded`. `NoFocus` remove um botão das paradas de Tab quando uma linha ou lista o alcança de outra forma.

## Caixa de seleção

`base.Checkbox` recebe um estilo para a caixa, um para o indicador e sua marca dentro dele, exibida a partir do valor que o aplicativo possui.

```go
base.Checkbox{
	Checked:        done.Get,
	OnChange:       done.Set,
	Label:          "Remember me",
	Style:          core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	IndicatorStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), BorderWidth: 1, CornerRadius: 5},
	Indicator:      core.Icon{SVG: lucide.Check(), Style: core.Style{FontSize: 14}},
	Children:       []core.Node{core.Text{Content: "Remember me"}},
}
```

O indicador assume seu estado a partir da caixa por meio de `PartStates`, portanto classes `checked:` o estilizam sem torná-lo uma segunda parada.

Para modificadores, defina `OnChangeEvent func(checked bool, press core.PressEvent)`. Ele substitui `OnChange` e deixa o valor com o aplicativo; `press.Modifiers.Shift` permite que uma lista selecione um intervalo. A ativação por Space/Return em desktops, na web, no Android e no iOS/tvOS preserva os modificadores, a menos que um atalho do aplicativo trate a tecla primeiro; no Windows, Alt com Space ou Return vai para o menu da janela. Em TSX, `onChangeEvent` recebe `{ checked, press }`.

## Interruptor

`base.Switch` é um trilho, um polegar (thumb) dentro dele e, em seguida, seus filhos como rótulo. Um pressionamento informa o valor solicitado, o oposto de `Checked`.

```go
base.Switch{
	Checked:    dark.Get,
	OnChange:   dark.Set,
	Label:      "Dark appearance",
	Style:      core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	TrackStyle: core.Style{Width: core.Pt(38), Height: core.Pt(22), CornerRadius: 11},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

O trilho e o polegar aceitam seus próprios `StyleFunc`, e seus estilos de estado acompanham `Checked`.

## Controle deslizante

`base.Slider` recebe um `base.SliderValue`: `base.SingleValue(v)` ou `base.RangeValue(low, high)`. Ele estiliza separadamente o trilho, o indicador, o intervalo e os polegares.

```go
level := core.NewSignal(cx, 3.0)
base.Slider{
	Value:      func() base.SliderValue { return base.SingleValue(level.Get()) },
	OnChange:   func(v base.SliderValue) { level.Set(v.End) },
	Min:        func() float64 { return 0 },
	Max:        func() float64 { return 10 },
	Step:       func() float64 { return 1 },
	Label:      "Level",
	Style:      core.Style{Height: core.Pt(28)},
	TrackStyle: core.Style{Height: core.Pt(4), CornerRadius: 2},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

`Min`, `Max` e `Step` são getters reativos: alterar seus sinais atualiza o controle deslizante montado e seu intervalo acessível, inclusive durante um arraste. `Axis` o torna vertical e `Inverted` coloca seu mínimo na extremidade oposta, como ocorre em `base.Progress`; `Marks` posiciona suas próprias marcas no indicador, e `OnChangeEnd` informa o valor assim que o arraste termina.

## Select

`base.Select` recebe `base.SelectItem`s e um valor `*string`, de modo que “nenhum” é `nil`. Você estiliza o gatilho, o pop-up, suas linhas e seus grupos.

```go
choice := core.NewSignal[*string](cx, nil)
base.Select{
	Items: func() []base.SelectItem {
		return []base.SelectItem{{Value: "s", Title: "Small"}, {Value: "m", Title: "Medium"}}
	},
	Value:      choice.Get,
	OnChange:   choice.Set,
	Label:      "Size",
	Style:      core.Style{Height: core.Pt(28), Padding: core.Symmetric(4, 10), BorderWidth: 1, CornerRadius: 6},
	PopupStyle: core.Style{Padding: core.All(4), CornerRadius: 8},
	ItemStyle:  core.Style{Padding: core.Symmetric(4, 8), CornerRadius: 4},
}
```

Itens adjacentes com o mesmo `Group` compartilham um cabeçalho, a digitação rápida (typeahead) faz correspondência com os títulos dos itens, `SearchField` desenha um campo de busca próprio e `Cleanable` exibe um botão para limpar. `OnCancel` escuta Escape, voltar e um pressionamento fora.

## Abas

`base.Tabs` contém `base.Tab`s e desenha o indicador a partir de `IndicatorStyle`; o aplicativo mantém qual aba está selecionada. `base.Tab.OnPress` é executado em todo pressionamento, inclusive no da aba selecionada.

```go
base.Tabs{Label: "Sections", IndicatorStyle: core.Style{Height: core.Pt(2), Background: core.RGB(0x007aff)}, Children: []core.Node{
	base.Tab{Label: "Overview", Selected: func() bool { return section.Get() == "overview" },
		OnPress: func(core.PressEvent) { section.Set("overview") }, Children: []core.Node{core.Text{Content: "Overview"}}},
	base.Tab{Label: "Activity", Selected: func() bool { return section.Get() == "activity" },
		OnPress: func(core.PressEvent) { section.Set("activity") }, Children: []core.Node{core.Text{Content: "Activity"}}},
}}
```

## Diálogo

`base.Dialog` recebe os estilos de seu fundo (backdrop), camada e pop-up, uma `Animation` ou um `TransitionShader`, e seus filhos. `Open`, `DefaultOpen` e `OnOpenChange` o controlam; `OnOpen`, `OnClose` e `OnCancel` informam o que aconteceu. `OnConfirm` é a ação padrão: Return a executa, a menos que o foco do teclado esteja em um botão ou campo que consome Return ou outro modal cubra o diálogo; então o diálogo fecha com `ReasonSelect`. `Role` e `AccessibilityDescription` são o que os leitores de tela anunciam, usando `RoleAlertDialog` para uma confirmação. `NoCloseOnSwipe` impede que o gesto de deslizar do iOS o feche, `Presentation` escolhe a apresentação no iOS e `StatusBar` altera a barra enquanto ele está aberto.

`base.Sheet` é um painel em qualquer borda, com um fundo e seu conteúdo; com `CloseOnSwipe`, um painel inferior acompanha o arraste para baixo e fecha ao passar de um limite. Ele aceita `OnConfirm`, `Role` e `AccessibilityDescription` como `base.Dialog`. `base.ShowAlertDialog` exibe o alerta ou folha de ações do sistema com botões por papel:

```go
base.ShowAlertDialog(cx, base.AlertDialog{
	Title: "Delete the note?",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(label string) {
	if label == "Delete" {
		remove()
	}
})
```

## Menu

`base.Menu` abre `base.MenuItem`s a partir de um gatilho próprio, e `base.ContextMenu` na posição do ponteiro. Um item informa `OnSelect` e fecha o menu, a menos que `NoCloseOnSelect` esteja definido; `Checked` o torna um item marcável, `Indicator` desenha sua marca antes do conteúdo e `Role` define o que a tecnologia assistiva ouve: um item de menu, uma caixa de seleção ou um item de opção. Um `base.Submenu` contém mais itens.

```go
base.Menu{Trigger: core.Text{Content: "Actions"}, Children: []core.Node{
	base.MenuItem{Label: "Show sidebar", Checked: sidebar.Get, OnSelect: func() { sidebar.Set(!sidebar.Get()) }},
	base.Submenu{Label: "Share", Children: []core.Node{
		base.MenuItem{Label: "Copy link", OnSelect: copyLink},
		base.MenuItem{Label: "Email", OnSelect: emailLink},
	}},
}}
```

`Placement`, `Align`, `Offset` e `Margin` posicionam o pop-up em desktops, e `PopupStyle` e `BackdropStyle` o estilizam.

## Seletor de emojis

`base.EmojiPicker` pesquisa todo emoji Unicode por seus nomes CLDR localizados, palavras-chave e códigos curtos nos idiomas do aplicativo (ou `Languages` / `LanguagesFunc` explícitos), com abas de categoria, entradas opcionais de `Recent`, seleção de tom de pele, navegação por teclado e uma grade virtualizada. Todos os rótulos padrão e nomes de categoria vêm de `base/locales`, e o conjunto de dados de emojis só é vinculado aos aplicativos que usam `EmojiPicker` ou seus auxiliares (`base.SearchEmojis`, `base.EmojisInCategory`, `base.EmojiByString`).

```go
base.EmojiPicker{
	Recent:   recent.Get,
	OnSelect: func(e base.Emoji) { insert(e.Emoji) },
	Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
}
```

`Category` e `DefaultCategory` escolhem o grupo de navegação. As abas de categoria e as escolhas de tom de pele são, cada uma, uma única parada de Tab com navegação pelas setas. `EmojiPickerRef` fornece `Focus`, `Query`, `SetQuery`, `SetCategory` e `MatchedCount`; o seletor estilizado usa o mesmo estado de busca. `LabelFunc`, `SearchLabelFunc` e `PlaceholderFunc` atualizam palavras no lugar. `Emoji.WithSkinTone` aplica o tom escolhido a ambas as pessoas em um par, e o tom padrão restaura sua forma neutra; buscar por um emoji preserva sua sequência exata.

Use `ui.EmojiPicker` para o seletor estilizado com tema, ícones de categoria Lucide, campo de busca limpável, prévia de emoji em destaque e amostras de tom de pele.

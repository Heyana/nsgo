---
title: Partes e estados
description: Não mantenha estados que pertencem ao aplicativo, estilize cada parte e informe à tecnologia assistiva o que o componente é.
group: Escrevendo componentes
order: 71
---

Um componente sem estilo decide como um controle se comporta e deixa a aparência para quem o usa. Esta página constrói `Toggle`, uma pílula que fica ligada ou desligada, da mesma maneira que `base` constrói seus controles.

## Valores controlados

Um controle não guarda valores que pertencem ao aplicativo: o aplicativo os passa como uma função e os define a partir de um callback. O componente lê o valor onde ele é exibido, para que apenas essa parte seja atualizada quando ele mudar.

```go
// Toggle is a pill that is on or off. Its parts are the pill and a dot
// inside it, then its children, the label.
type Toggle struct {
	On       func() bool
	Disabled func() bool
	// OnChange receives the value a press asks for, the opposite of On's.
	OnChange func(on bool)

	Class        string
	Style        core.Style
	StyleFunc    func() core.Style
	DotClass     string
	DotStyle     core.Style
	DotStyleFunc func() core.Style
	Children     []core.Node
	Label        string
}
```

Um aplicativo mantém o valor em um sinal: `chips.Toggle{On: bold.Get, OnChange: bold.Set, Label: "Bold"}`.

## Partes

Cada parte que um componente desenha recebe um `Class`, um `Style` e um `StyleFunc`, nomeados conforme a parte (`DotClass`, `DotStyle`, `DotStyleFunc`). `Style` é fixo, `StyleFunc` acompanha os sinais que lê e a classe fica abaixo de ambos, para que um aplicativo possa usar o que preferir escrever.

Uma parte assume o estado do controle por meio de `PartStates` em vez de informar o seu próprio, para que classes `checked:` e estilos `Checked` se apliquem a ela enquanto a tecnologia assistiva enxerga um único controle:

```go
func (t Toggle) Render(*core.Scope) core.Node {
	on := func() bool { return t.On != nil && t.On() }
	off := func() bool { return t.Disabled != nil && t.Disabled() }
	shows := func() core.PartStates { return core.PartStates{Checked: on(), Disabled: off()} }
	dot := core.View{Class: t.DotClass, Style: t.DotStyle, StyleFunc: t.DotStyleFunc, PartStates: shows}
	return core.View{
		Class: t.Class, Style: t.Style, StyleFunc: t.StyleFunc,
		Role: core.RoleSwitch, Label: t.Label, Checked: on, Disabled: t.Disabled,
		OnPress: func(core.PressEvent) {
			if t.OnChange != nil {
				t.OnChange(!on())
			}
		},
		Children: append([]core.Node{dot}, t.Children...),
	}
}
```

Um aplicativo o estiliza apenas com classes:

```go
chips.Toggle{
	On: bold.Get, OnChange: bold.Set, Label: "Bold",
	Class:    "flex-row items-center gap-2 rounded-full border border-zinc-300 px-3 py-1 checked:bg-blue-50",
	DotClass: "size-2 rounded-full bg-zinc-300 checked:bg-blue-500",
	Children: []core.Node{core.Text{Content: "Bold"}},
}
```

`PartStates.Invalid` aplica de forma semelhante classes `aria-invalid:` e `Style.AriaInvalid` a uma parte decorativa, incluindo estilos `group-aria-invalid:` dentro dela, sem informar outro controle inválido. Coloque `Invalid` no controle real ou na entrada nativa para que a tecnologia assistiva receba seu estado de validação.

## Acessibilidade

A raiz de um controle diz o que ele é: um `Role`, um `Label` quando seu texto não o diz, um `Hint` para o que a ativação faz, e `Checked`, `Selected`, `Disabled`, `Invalid` ou `Expanded` conforme aplicável. Suas partes permanecem silenciosas. Um controle que se move entre itens com as setas é uma única parada de Tab, como uma lista de abas. [Acessibilidade](../accessibility.md) lista os papéis e ações.

## O que repassar

Um componente cuja raiz é uma visualização própria recebe `OnLayout`, `Tooltip` e `TestID` dessa visualização e os repassa, para que os aplicativos possam medi-lo, explicá-lo e testá-lo como qualquer visualização. Callbacks são opcionais: um `OnChange` nil cria um controle que exibe um valor sem alterá-lo.

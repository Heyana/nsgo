---
title: Formulários
description: Campos de texto, foco, teclado virtual, seletores, alternadores e validação.
group: Conceitos principais
order: 57
---

## Campos de texto

`ui.Input` é o campo de texto da plataforma com a aparência de `ui`, possuindo os campos de `base.Input`. Com `Value` ele é controlado: o aplicativo guarda o valor e o atualiza em `OnChange`; com `DefaultValue` ele mantém o seu próprio valor.

```go
name, email := core.NewSignal(cx, ""), &core.Ref{}

ui.Input{
	Value:         name.Get,
	OnChange:      name.Set,
	Placeholder:   "Your name",
	ReturnKeyType: core.ReturnKeyNext,
	OnSubmit:      func(string) { email.Focus() },
}
```

`Masked` oculta os caracteres, `Readonly` impede edições enquanto o texto ainda pode ser selecionado e copiado (em miniprogramas, apenas em um campo de linha única do WeChat: um campo multilinha do WeChat e os campos do Alipay desativam como faz `Disabled`, já que esses hosts não conseguem desfazer uma edição), `Disabled` desativa o campo, e `Multiline` quebra o texto em linhas e cresce com ele a menos que seu estilo defina seu tamanho. As linhas de um campo multilinha, e as de seu texto de placeholder, têm a altura definida pelo `LineHeight` de seu estilo, com a fonte centralizada em cada uma como em um `core.Text`; um campo de linha única mantém sua linha centralizada em sua altura. `KeyboardType` escolhe o teclado virtual (`core.KeyboardEmail`, `core.KeyboardNumber`, `core.KeyboardURL` e os demais), `AutoComplete` informa ao sistema o que o campo contém para preenchimento automático, e `MaxLength` limita as unidades UTF-16 que o usuário digita. `AutoCapitalize` e `NoAutoCorrect` ajustam o comportamento do teclado.

`OnSelectionChange` e `OnCaretMove` relatam a seleção e onde está o cursor de texto, para um menu que abre nessa posição. `Suggest`, um `core.Suggest`, acompanha uma consulta após um gatilho como `@`, e uma `ui.SuggestList` ao lado do campo oferece linhas para ela no cursor, dentre as quais Seta para cima, Seta para baixo, Return e um toque escolhem, como `Suggest` de `chat.Composer` faz para menções. `OnKeyPress` escuta cada tecla, `OnPaste` o que o usuário cola e `OnCompositionChange` acompanha um método de entrada.

No iOS e Android, `cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, handler)` captura um Return virtual antes que o campo envie, perca foco ou insira uma quebra de linha. `KeyAuto` e `KeyNotEditing` deixam a decisão para `SubmitBehavior`; no iOS, um Return que confirma texto marcado permanece com o método de entrada.

## Foco e o teclado

`Ref.Focus()` foca um campo ou, em desktops, na web e na Apple TV, uma view habilitada que aceita toque ou `Focusable`. `Ref.Blur()` limpa apenas o foco da sua própria view, e `cx.DismissKeyboard()` fecha o teclado virtual. `OnFocus` e `OnBlur` acompanham o foco, e `View.OnFocus`/`OnBlur` fazem o mesmo para qualquer view focalizável. Uma view focada que sai da árvore, conforme seu `Show` fecha ou sua linha some, recebe `OnBlur` uma vez, após o commit que a removeu ou, quando a plataforma relata o próximo foco primeiro, antes do `OnFocus` dessa view, e as views ao redor que permanecem recebem `OnFocusOut`.

As solicitações de foco, desfoco, rolagem e zoom de um `Ref` não fazem nada enquanto seu elemento está no conteúdo de um `Holder` que nenhum lugar exibe, e aquelas ainda na fila quando o elemento sai da árvore são descartadas. Exibir o conteúdo novamente não as reproduz: solicite outra vez quando ele estiver posicionado.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

Os teclados virtuais cobrem a parte inferior de um celular. `core.EnvKeyboard` como preenchimento interno ou margem inferior mantém o conteúdo acima dele, e seu movimento é animado junto com o teclado:

```go
core.View{Style: core.Style{
	Padding:    core.All(16),
	PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard},
	Gap:        12,
}, Children: form}
```

Uma scroll view ao redor de um formulário pode dispensar o teclado ao ser arrastada e decidir o que um toque faz enquanto o teclado está visível:

```go
core.Scroll{Behavior: core.ScrollBehavior{
	KeyboardDismissMode:       core.KeyboardDismissOnDrag,
	KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
}, Children: []core.Node{form}}
```

`cx.Env().KeyboardHeight()` e `cx.Env().KeyboardDuration()` leem o tamanho e a duração da animação do teclado em código, e `cx.Env().SafeArea()` lê os recuos ao redor da tela.

## Escolhas

`ui.Switch` é uma configuração que entra em vigor imediatamente, `ui.Checkbox` fica marcado, desmarcado ou indeterminado, e `ui.Radio` é uma opção dentro de um `ui.RadioGroup`. `ui.Select[T]` abre uma lista de opções sob seu gatilho, ou como uma sheet quando seu `Presentation` solicita, e `ui.Tabs[T]` mostra algumas opções ao mesmo tempo.

```go
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}

size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options:  []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:    size.Get,
	OnChange: size.Set,
	Label:    "Size",
}
```

`ui.NumberInput` e `base.NumberInput` aceitam `LabelFunc` para um nome acessível variável. Um `Format` personalizado acompanha os signals que lê enquanto o campo não está sendo editado; uma troca de idioma preserva o rascunho ativo até que a edição termine. `ui.NumberInput` incrementa e ajusta um número por arrasto dentro de limites, `ui.DatePicker` e `ui.TimeField` editam datas e horários, `ui.ColorPicker` edita uma cor e `ui.TagInput` edita uma lista de tags.

`ui.OtpInput` recebe um código de uso único em células. Ele e `base.OtpInput` aceitam `LabelFunc` sem substituir o campo focado nem seu código. `Size` define a altura e a largura padrão, 36 pontos em desktops e TVs e 44 em dispositivos de toque. `CellWidth` e `CellHeight` substituem cada dimensão de forma independente. `TextStyle` substitui a tipografia das células, `CornerRadius` altera seus cantos e `CaretHeight` substitui a altura derivada da altura da linha.

```go
quantity := core.NewSignal(cx, 1.0)
ui.NumberInput{Value: quantity.Get, OnChange: quantity.Set, Min: 1, Max: 99, Step: 1, Label: "Quantity"}
```

## Linhas agrupadas

`ui.FormSection` coloca suas linhas em um cartão, com o título acima e o rodapé abaixo. Defina `CardStyle` para o fundo, borda ou cantos do cartão. `CardStyleFunc` é mesclado sobre ele e acompanha os signals que lê. `DividerInset` recebe cada linha filha, incluindo linhas criadas por `core.Each`: retorne um recuo e `true` para substituir seu divisor, ou `false` para manter o alinhamento automático do título (12 pontos para outros nós). Um recuo igual a zero cria um divisor de largura total.

`ui.FormRow` posiciona `Leading` ou um bloco de ícone antes do título. Defina `LeadingWidth` para reservar espaço para seu nó e alinhar o divisor abaixo dele com o título. O espaço padrão é de 24 pontos. Defina `TileSize` para escalar o bloco de ícone, o glifo e os cantos juntos. `TitleStyle` e `DetailStyle` substituem a tipografia do título e do detalhe. `TitleMaxLines` limita as linhas do título (zero é ilimitado), `MinHeight` substitui a altura mínima da linha em pontos e `Below` coloca um nó após o título e o detalhe na coluna de texto deles. `MinHeight` igual a zero mantém o padrão: 40 pontos, ou 48 em dispositivos de toque.

Para um conjunto de escolha única, envolva a seção em `base.RadioGroup` e defina em cada linha `Role: core.RoleRadio`, `Checked` e `OnPress`. A linha usa `base.Radio` para seus estados de acessibilidade marcado e selecionado, uma parada de Tab e seleção pelas setas; pressionar uma linha já marcada não faz nada. O aplicativo é dono do valor escolhido.

`ui.FormSection.TitleFunc` e `FooterFunc`, e `ui.FormRow.TitleFunc`, `DetailFunc` e `LabelFunc`, atualizam seus textos sem reconstruir a linha. Um detalhe, título de seção ou rodapé vazio remove esse texto opcional; um rótulo de linha vazio usa seu título atual. Um `ui.Select` pesquisável vincula `SearchPlaceholderFunc` e `EmptyFunc` mantendo sua busca aberta e sua seleção.

## Validação

O aplicativo é dono das regras: um signal guarda o erro, `OnBlur` ou `OnSubmit` o define e um `core.Show` adiciona uma mensagem sob o campo enquanto houver um erro.

```go
address := core.NewSignal(cx, "")
err := core.NewSignal(cx, "")
colors := ui.ColorsOf(cx)
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{
		Value:    address.Get,
		OnChange: func(v string) { address.Set(v); err.Set("") },
		Label:    "Email",
		OnSubmit: func(string) {
			if !strings.Contains(address.Get(), "@") {
				err.Set("Enter an email address.")
			}
		},
	},
	core.Show{
		When: func() bool { return err.Get() != "" },
		Then: func(*core.Scope) core.Node { return ui.Textf(err.Get, 12, ui.Regular, colors.Danger) },
	},
}}
```

`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` anima uma mensagem que aparece, e `OnSubmit` com `ReturnKeyNext` move o teclado para o próximo campo.

## Próximos passos

- [Navegação](navigation.md): onde um formulário vive.
- [Texto](text.md): como o texto de um campo é estilizado.
- [Acessibilidade](accessibility.md): rótulos e valores de que os leitores de tela precisam.

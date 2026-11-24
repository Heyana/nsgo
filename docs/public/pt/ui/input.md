---
title: Input
description: O campo de texto da plataforma, controlado ou não, com foco e o teclado virtual.
group: Componentes
order: 33
---

`ui.Input` é o campo de texto da plataforma com a aparência de `ui`, de uma linha a menos que seja `Multiline`. Ele tem os campos de `base.Input`, como todo campo estilizado; `ui.Field` é o mesmo campo sem tamanho próprio, em uma borda que não segue o ponteiro nem o foco; `core.Transparent` em seu `Background` e `core.Zero` em seu `BorderWidth` desligam a caixa, para um texto que se lê como texto simples mas é editável.

```go preview=input/basic
name := core.NewSignal(cx, "")
ui.Input{
	Value:       name.Get,
	OnChange:    name.Set,
	Placeholder: "Your name",
	TestID:      "name",
}
```

## Controlado e não controlado

Com `Value`, o campo é controlado: o aplicativo guarda o valor e o atualiza em `OnChange`. Com `DefaultValue`, o campo mantém o seu próprio. Um campo controlado pode ser limpo definindo seu valor como `""`.

```go preview=input/submit
draft := core.NewSignal(cx, "")
ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "New task",
	OnSubmit: func(string) { add(); draft.Set("") }}
```

## Campos

`Multiline` quebra as linhas, começa com três linhas de altura e cresce com seu texto, a menos que seu estilo defina `Height` ou `MinHeight`. `Masked` oculta os caracteres, `Readonly` impede edições enquanto o texto ainda pode ser selecionado e copiado, `Disabled` desliga o campo, e `MaxLength` limita as unidades UTF-16 que o usuário digita. `Placeholder` pode ser uma função com `PlaceholderFunc`.

```go
ui.Input{Masked: true, Value: password.Get, OnChange: password.Set,
	AutoComplete: core.AutoCompletePassword}
```

`KeyboardType` escolhe um teclado virtual (`KeyboardEmail`, `KeyboardNumber`, `KeyboardURL` e os demais), `ReturnKeyType` rotula sua tecla de retorno, e `SubmitBehavior` diz se Return envia ou quebra uma linha em um campo de várias linhas. `AutoCapitalize`, `NoAutoCorrect` e `AutoComplete` ajustam a ajuda do sistema.

`InputSource` seleciona uma fonte de entrada do teclado enquanto o campo tem o foco e restaura a do usuário quando ele o perde, a menos que `KeepInputSource`: `"com.apple.keylayout.ABC"` mantém em inglês, no macOS, um campo para códigos ou comandos. O ID é o da plataforma, um ID de layout de teclado como `"00000409"` no Windows e um método de entrada do Fcitx 5 ou IBus no Linux; uma fonte que não está ativada, celulares e a web não mudam nada.

```go
ui.Input{Placeholder: "Bundle ID", InputSource: "com.apple.keylayout.ABC"}
```

`Style` é mesclado sobre a aparência do campo, e `StyleFunc` sobre isso: o que eles definem, como a altura, o padding, a fonte ou uma cor, substitui o de `ui`, e o resto permanece. A borda escurece sob o ponteiro e assume a cor de destaque enquanto focada, desde que nenhum dos dois defina `BorderColor` ou `Hover`. Um `Padding` zero mantém o de `ui`; `core.Zero`, um comprimento que é definido, o remove (`Padding: core.All(core.Zero)`).

```go
ui.Input{Placeholder: "Filter", Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
ui.Field{DefaultValue: "Untitled", Style: core.Style{FontSize: 24, FontWeight: ui.Bold,
	Background: core.Transparent, BorderWidth: core.Zero}}
```

## Foco e eventos

`Ref` foca e desfoca o campo. `OnFocus` e `OnBlur` seguem o foco, `OnSelectionChange` e `OnCaretMove` a seleção e o cursor de texto, `OnKeyPress` cada tecla digitada, `OnPaste` o que é colado e `OnCompositionChange` a composição de um método de entrada.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

O teclado virtual cobre a parte de baixo de um celular. Coloque `core.EnvKeyboard` no padding ou na margem inferior do conteúdo para que ele fique acima do teclado; o movimento é animado junto com ele. `cx.DismissKeyboard()` fecha o teclado, e `cx.Env().KeyboardHeight()` lê sua altura.

## Validação

As regras são do aplicativo. Um signal guarda o erro, `OnBlur` ou `OnSubmit` o define, e uma mensagem abaixo do campo o mostra ([Formulários](../forms.md#validação)).

```go preview=input/validation
err := core.NewSignal(cx, "")
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{Value: email.Get, OnChange: email.Set, Label: "Email",
		OnSubmit: func(string) {
			if !strings.Contains(email.Get(), "@") {
				err.Set("Enter an email address.")
			} else {
				err.Set("")
			}
		}},
	ui.Textf(err.Get, 12, ui.Regular, colors.Danger),
}}
```

## Acessibilidade

O campo relata `RoleTextField`, ou `RoleSearchField` para uma caixa de busca quando seu `Role` assim indica, e lê seu `Label`; `Placeholder` não é um rótulo, então dê um `Label` a um campo sem rótulo visível. `Readonly` e `Disabled` são relatados, e `OnSubmit` é a ação do teclado. Campos de uma linha, mascarados e de várias linhas expõem o `Placeholder` atual como o placeholder ou a dica de acessibilidade nativa no macOS, iOS e Linux, atualizando e limpando sem uma segunda parada de foco para o rótulo de placeholder desenhado de um campo de várias linhas.

## Plataforma

iOS, Android, Windows e Linux usam seus campos nativos; a web usa um `<input>` ou `<textarea>`. O teclado virtual, o preenchimento automático, os métodos de entrada e a direção do texto são da plataforma. Foco e digitação são testados da mesma forma em todos os alvos.

## Relacionados

- [Formulários](../forms.md): campos, validação, escolhas e o teclado.
- [Acessibilidade](../accessibility.md): rótulos e foco.
- [Layout](../layout.md): manter o conteúdo acima do teclado.
- As páginas Input e Form da galeria.

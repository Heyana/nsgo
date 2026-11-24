---
title: Checkbox
description: Uma caixa de seleção, marcada, desmarcada ou indeterminada.
group: Componentes
order: 34
---

`ui.Checkbox` é uma caixa de seleção com a aparência de `ui`, com um rótulo ao lado. Ela não guarda nenhum valor: o aplicativo diz se ela está marcada e atualiza isso em `OnChange`.

```go preview=checkbox/basic
done := core.NewSignal(cx, false)
ui.Checkbox{Checked: done.Get, OnChange: done.Set, Text: "Remember me"}
```

## Conteúdo

`ui.Checkbox` mostra `Text` ou os `Children` do aplicativo ao lado da sua caixa. `Label` substitui ambos para a tecnologia assistiva, e `Hint` diz para que ela serve.

## Estados

`Indeterminate` não é nem marcado nem desmarcado, para um "selecionar tudo" sobre uma seleção parcial; um toque então pede o estado marcado. `Disabled` bloqueia os toques e o foco.

```go preview=checkbox/indeterminate height=220
ui.Checkbox{
	Checked:       func() bool { return chosen.Get() == len(files) },
	Indeterminate: func() bool { return chosen.Get() > 0 && chosen.Get() < len(files) },
	OnChange:      chooseAll,
	Text:          "Select all",
}
```

## Modificadores do toque

Use `OnChangeEvent` em vez de `OnChange` quando uma lista precisar de seleção de intervalo com Shift pressionado:

```go
ui.Checkbox{Checked: done.Get, Text: "Remember me", OnChangeEvent: func(next bool, press core.PressEvent) {
	if press.Modifiers.Shift {
		chooseRange(next)
		return
	}
	done.Set(next)
}}
```

Quando definido, esse callback substitui `OnChange`; o aplicativo ainda atualiza o valor. A ativação pelo teclado no desktop e na web preserva os modificadores após os atalhos do aplicativo; no Windows, Alt com Espaço ou Return vai para o menu da janela. O `onChangeEvent` do TSX recebe `{ checked, press }`.

## Acessibilidade

A caixa de seleção relata `RoleCheckbox` e seu estado marcado ou indeterminado. Seu nome é `Text`, `Label` ou o texto dos filhos. Espaço a alterna quando ela tem o foco do teclado, e um leitor de tela anuncia o novo estado.

## Plataforma

A caixa é desenhada pelo aplicativo, não por um controle da plataforma, então tem a mesma aparência em todo lugar. O toque, o anel de foco e o leitor de tela são da plataforma. Ela é testada por papel e rótulo em todos os alvos.

## Relacionados

- [Switch](switch.md): uma configuração que tem efeito imediato.
- [Formulários](../forms.md): escolhas em um formulário.
- [List](list.md): caixas de seleção em linhas.
- A página Checkbox da galeria.
- [Componentes sem estilo](../authoring/base.md#checkbox): `base.Checkbox`, para uma caixa com uma aparência própria.

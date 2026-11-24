---
title: Visuais e temas
description: Obtenha cores do tema em vigor e forneça variantes e tamanhos para um componente.
group: Escrevendo componentes
order: 72
---

Um componente estilizado escolhe estilos e nada mais: ele envolve um componente sem estilo, em um pacote próprio como `chipsui`, e mantém seu comportamento lá. Suas cores vêm do tema, para que acompanhe o tema do aplicativo, o tema próprio de um painel e a aparência clara ou escura sem precisar ser reconstruído.

## Cores do tema

Leia as cores dentro de uma `StyleFunc`: ela é executada novamente quando o que ela leu muda, portanto uma alteração de tema ou aparência reestiliza o componente no lugar. `cx.Palette()` fornece a paleta exibida onde o componente está, e `ui.ColorsOf(cx)` nomeia os papéis que `ui` usa.

```go
// Toggle is chips.Toggle in the theme's colors.
type Toggle struct {
	On       func() bool
	OnChange func(bool)
	Label    string
}

func (t Toggle) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	on := func() bool { return t.On != nil && t.On() }
	return chips.Toggle{On: t.On, OnChange: t.OnChange, Label: t.Label,
		Class: "flex-row items-center gap-2 rounded-full px-3 py-1",
		StyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.Accent()}
			}
			return core.Style{Background: colors.Inset()}
		},
		DotClass: "size-2 rounded-full",
		DotStyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.OnAccent()}
			}
			return core.Style{Background: colors.Faint()}
		},
		Children: []core.Node{ui.Text(t.Label, 13, ui.Medium, colors.Text)},
	}
}
```

Cores fixas no código quebram no modo escuro e sob o tema de um aplicativo; obtenha cada cor a partir de um papel.

## Papéis próprios

Uma cor para a qual a paleta não possui papel vai em `Palette.Colors` sob um nome seu, definida em ambas as aparências. Os aplicativos a sobrescrevem em código ou em seu arquivo de tema, sob `light` e `dark`:

```go
theme := core.DefaultTheme()
theme.Light.SetColor("chip", core.RGB(0xeef5ff))
theme.Dark.SetColor("chip", core.RGB(0x1b2a3d))
cx.SetTheme(theme)
```

Um componente a lê com `cx.Palette().Color("chip")`, que informa se o tema a possui, para que o componente possa recorrer a um papel quando ela não existir. Gradientes, sombras e outros acabamentos não são cores: mantenha-os em código, escolhidos por `cx.Dark()`.

## Variantes

Um componente com vários visuais aceita `Variant`, um tipo próprio cujo valor zero é o visual padrão. Ele nomeia apenas visuais que o componente desenha, incluindo sua cor, em vez de campos booleanos ou um tipo de cor compartilhado:

```go
// ToggleVariant is a toggle's look.
type ToggleVariant uint8

const (
	// ToggleSolid fills the pill with the accent when on.
	ToggleSolid ToggleVariant = iota
	// ToggleOutline draws its edge in the accent instead.
	ToggleOutline
)
```

## Tamanhos

A maioria dos componentes precisa de um único tamanho, o que mantém os controles alinhados sem que o aplicativo precise escolher. Um componente que precise de mais aceita `Size`, um `ui.Size` padrão, pequeno ou grande, para corresponder aos 28, 24 e 32 pontos de `ui.Button`; `Size.Height()` retorna a altura para alinhar com outros controles. Um componente que tem uma única forma em qualquer escala, como um avatar, recebe `Size` em pontos.

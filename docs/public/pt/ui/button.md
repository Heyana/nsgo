---
title: Button
description: Um botão de ação, suas variantes e tamanhos, e o botão de ícone.
group: Componentes
order: 31
---

`ui.Button` é um botão de ação com a aparência do `ui`, com as variantes e os tamanhos de que uma janela precisa. `OnPress` é executado ao fim de um pressionamento completo.

```go preview=button/basic
ui.Button{Text: "Save", OnPress: save}
ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: cancel}
```

## Variantes e tamanhos

`ui.Button.Variant` é `ButtonPrimary` (o padrão), `ButtonSecondary`, `ButtonOutline`, `ButtonGhost`, `ButtonTinted` ou `ButtonDanger`. Tinted e danger usam a cor de destaque ou de perigo do tema sobre um preenchimento suave. Nos idiomas de desktop e TV, `Size` é `SizeDefault` (28 pontos), `SizeSmall` (24) ou `SizeLarge` (32). Nos idiomas de toque, os mesmos tamanhos são 44, 40 e 48 pontos, de modo que o botão pode ser usado sem precisar mudar seu estilo.

```go preview=button/variants
ui.Button{Text: "Delete", Variant: ui.ButtonOutline, OnPress: remove}
ui.Button{Text: "Continue", Size: ui.SizeLarge, OnPress: next}
ui.Button{Text: "Save", Icon: lucide.Check, Flex: 1, OnPress: save}
```

`Color` preenche o botão com uma cor do app em vez da cor de uma variante, e `Tint` faz o mesmo com seu ícone. `Flex` dá a ele uma parte de uma linha, enquanto `Style` define seu layout: pode fazer o botão preencher seu contêiner, definir um tamanho do app, adicionar margens ou arredondar seus cantos; `TestID` o nomeia para testes.

```go
ui.Button{Text: "Continue", Size: ui.SizeLarge,
    Style: core.Style{Width: core.Pct(100), Height: core.Pt(50)}, OnPress: next}
```

Um botão apenas com ícone é `ui.IconButton`, com `Variant` `IconButtonGhost`, `IconButtonRaised` ou `IconButtonFilled`, um círculo plano na cor de inset do tema ou em `Fill`, um `Size` em pontos e um `Label` para a tecnologia assistiva. Em desktops, o rótulo também aparece como dica de ferramenta quando o ponteiro repousa sobre o botão; `Tooltip` mostra outra coisa ali, e um que retorna `""` não mostra nenhuma. Um `ui.Button` com `Icon` e sem `Text` também é quadrado e precisa de `Label`.

```go preview=button/icon
ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost, OnPress: remove}
```

## Conteúdo e estados

`ui.Button` mostra `Text`, um `Icon` opcional à frente, ou ambos. Use `TextStyle` para personalizar o tamanho da fonte, o peso ou outros estilos de texto do rótulo.

```go
ui.Button{Text: "Saving…", Disabled: saving.Get, OnPress: save}
```

`Disabled` recebe uma função, de modo que o botão acompanha um signal: um botão desativado não recebe pressionamentos nem o foco do teclado, e sua aparência é a do estado desativado, não uma cor escolhida pelo app.

`Busy` serve para uma ação ainda em andamento. Enquanto retorna true, um spinner na cor do texto do botão aparece sobre o conteúdo, de modo que ele também aparece sobre o destaque de um botão primário, os pressionamentos não fazem nada e a tecnologia assistiva ouve que ele está ocupado. Por padrão, o conteúdo mantém seu espaço sob o spinner. O botão mantém o foco do teclado. `BusyLabel` ou `BusyLabelFunc` mostra uma mensagem ao lado do spinner e a informa como nome acessível do botão enquanto ele está ocupado. O botão mantém pelo menos sua largura em repouso.

```go preview=button/busy
ui.Button{Text: "Save", Busy: saving.Get, BusyLabel: "Saving…", OnPress: save}
```

`ui.Spinner` e `ui.ProgressRing` aceitam um `Tint` próprio para qualquer outra superfície sobre a qual o destaque deles desapareceria.

## Links

`ui.Link{Text: "Docs", Href: "/docs"}` abre um destino com o papel de link. Seu texto e o sublinhado ao passar o ponteiro usam a cor de destaque, a menos que `Style.Color` defina outra cor; `Style.Hover.Color` define a cor ao passar o ponteiro.

## Acessibilidade

O botão informa `RoleButton` e seu texto como nome. `Label` substitui o texto para a tecnologia assistiva. Um botão apenas com ícone sem `Label` é ilegível, então dê um nome a ele.

Return e Espaço pressionam um botão focado em desktops, na web e na Apple TV; o leitor de tela de um celular o pressiona como um clique.

## Plataforma

O botão é uma view desenhada da mesma forma em todos os lugares, não um botão da plataforma; pressionamentos, foco e acessibilidade vêm da plataforma. Os idiomas de toque usam áreas de layout maiores, como descrito acima.

## Relacionados

- [Formulários](../forms.md): botões em um formulário e validação.
- [Estilos e temas](../styles.md): o tema do qual um botão obtém suas cores.
- [Acessibilidade](../accessibility.md): papéis, rótulos e foco.
- A página Button da galeria.
- [Componentes sem estilo](../authoring/base.md#botão): `base.Button`, para um botão com uma aparência própria.

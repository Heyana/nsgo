---
title: Select
description: Um valor de uma lista, como um popup ou uma sheet.
group: Componentes
order: 36
---

`ui.Select[T]` escolhe um entre uma lista de valores de qualquer tipo comparável. A lista abre sob o gatilho em todos os dispositivos; `Presentation: base.PresentSheet` a abre como uma sheet vinda de baixo, como um aplicativo de celular pode querer.

```go preview=select/basic height=280
size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options: []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:   size.Get,
	OnChange: size.Set,
	Label:   "Size",
}
```

## Opções e busca

`Options` são `ui.Option[T]`s, cada uma com um `Value`, um `Label` e, opcionalmente, um `Group`, um `Icon`, uma função `Disabled` ou um `Content` próprio do aplicativo. Opções adjacentes de um mesmo `Group` não vazio aparecem sob um cabeçalho dele, em um `Select` ou em um `Combobox`, que as setas e a digitação pulam e os leitores de tela leem como um título. Use `OptionsFunc` quando as opções seguirem um signal, para que a lista aberta se atualize no lugar e mantenha o valor e o foco do campo. `Placeholder` aparece enquanto nenhuma opção é escolhida. `Searchable` filtra a lista pela digitação, `OnClear` mostra um botão de limpar, e `Disabled` o desativa.

```go preview=select/searchable height=400
ui.Select[string]{
	Options:  languages,
	Value:    language.Get,
	OnChange: language.Set,
	Searchable: true,
	SearchPlaceholder: "Search languages",
	OnClear:  func() { language.Set("") },
	Placeholder: "Choose a language",
}
```

`Style` dispõe o select, com 200 pontos de largura a menos que defina uma largura; sua altura, padding e tipografia chegam ao campo, como os de um [Input](input.md), para que um select se alinhe com um botão `SizeSmall`:

```go
ui.Select[string]{Options: sizes, Value: size.Get, OnChange: size.Set, Label: "Size",
	Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
```

## Comportamento

Cima, Baixo, Return e Espaço abrem a lista a partir do gatilho focado; as setas movem o destaque e o typeahead salta pelos títulos dos itens. Escolher um item fecha a lista e relata seu valor. Escape, voltar e um toque fora a fecham sem mudança. `Empty` é o texto mostrado enquanto a busca não encontra nada.

## Acessibilidade

`Label` nomeia o campo para a tecnologia assistiva. Use `LabelFunc` para atualizar esse nome no lugar conforme seus signals mudam. Um rótulo vazio usa `Placeholder`. O gatilho e a lista carregam os papéis do select, e o item selecionado é relatado. O teclado alcança o gatilho com Tab e o abre com Return ou Espaço.

## Plataforma

No macOS, Windows e Linux, a lista é um popup ao lado do gatilho. No iOS e Android, é uma sheet, e a linha selecionada permanece no campo. As mesmas opções e valores funcionam em todos os alvos.

## Relacionados

- [Formulários](../forms.md): selects em um formulário.
- [Menu](menu.md): comandos em vez de um valor.
- [Tabs](tabs.md): poucas opções mostradas de uma vez.
- As páginas Select e Combobox da galeria.
- [Componentes sem estilo](../authoring/base.md#select): `base.Select`, com grupos e um gatilho próprio.

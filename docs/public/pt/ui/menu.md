---
title: Menu
description: Um menu a partir de um gatilho, um menu de contexto e seus itens.
group: Componentes
order: 42
---

`ui.Menu` abre `ui.MenuItem`s a partir de um botão: ao lado dele, ou como uma sheet vinda de baixo com `Presentation: base.PresentSheet`, qualquer que seja o dispositivo. `ui.ContextMenu` abre os mesmos itens no ponteiro.

```go preview=menu/basic height=280
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", Icon: lucide.Pencil, OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", Shortcut: "mod+d", OnSelect: duplicate},
	ui.MenuSeparator{},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

O menu abre e fecha sozinho; `Open` e `OnOpenChange` permitem que o aplicativo o controle. `Trigger` é a aparência e o conteúdo do botão, e o menu recebe seus toques. Um popup nunca ultrapassa o espaço ao lado do seu gatilho: ele abre do lado em que cabe inteiro, e seus itens rolam além desse espaço, ou além de `MaxHeight`.

## Itens

Um `ui.MenuItem` mostra `Text`, um `Icon` opcional e um `Shortcut` (`"mod+c"`, escrito do jeito da plataforma em um popup), e relata `OnSelect`, fechando o menu a menos que `NoCloseOnSelect`. `Disabled` o desliga, `Checked` mostra uma marca ao lado dele, e `Variant: ui.MenuItemDanger` marca um item destrutivo. `ui.MenuLabel` encabeça um grupo e `ui.MenuSeparator` os divide. Um `ui.Submenu` guarda mais itens e abre ao lado do item ou, em uma sheet, no lugar dos itens ao redor.

```go preview=menu/checked height=320
ui.Menu{Trigger: ui.Button{Text: "View", Variant: ui.ButtonGhost}, Children: []core.Node{
	ui.MenuLabel{Text: "Sort by"},
	ui.MenuItem{Text: "Name", Checked: func() bool { return sort.Get() == "name" }, OnSelect: func() { sort.Set("name") }},
	ui.MenuItem{Text: "Date", Checked: func() bool { return sort.Get() == "date" }, OnSelect: func() { sort.Set("date") }},
	ui.MenuSeparator{},
	ui.Submenu{Text: "Share", Children: []core.Node{
		ui.MenuItem{Text: "Copy link", OnSelect: copyLink},
		ui.MenuItem{Text: "Email", OnSelect: emailLink},
	}},
}}
```

## Menus de contexto

`ui.ContextMenu` abre seus itens com um clique secundário, ou um toque longo em telas sensíveis ao toque, no ponteiro sobre seu `Trigger`.

```go preview=menu/context height=220
ui.ContextMenu{Trigger: fileRow, Children: []core.Node{
	ui.MenuItem{Text: "Open", OnSelect: open},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Comportamento

Escape, voltar e um toque fora fecham um menu. O menu é um popup posicionado por `Placement` e `Align`, ou uma sheet vinda de baixo quando apresentado assim. Em um desktop, Baixo, Cima, Home, End, Page Down e Page Up percorrem os itens habilitados, começando pelo que está sob o ponteiro, Direita abre um submenu e Esquerda o fecha, e Return escolhe. Um menu aberto pelo teclado foca seu primeiro item, e o foco volta ao gatilho quando o menu fecha.

## Acessibilidade

O menu relata `RoleMenu` e cada item `RoleMenuItem`. `Label` nomeia o gatilho quando seu conteúdo não o faz. Um leitor de tela lê o texto de um item e se ele está desabilitado.

## Plataforma

O gatilho e os itens são do aplicativo; a sheet, o popup, o foco, Escape e voltar são da plataforma. O aplicativo escolhe o popup ou a sheet; o NSGo não escolhe nenhum dos dois pelo dispositivo.

## Relacionados

- [Select](select.md): um valor de uma lista.
- [Dialog](dialog.md): uma superfície modal.
- [Tabs](tabs.md): seções mostradas de uma vez.
- As páginas Menu, ContextMenu e Command da galeria.
- [Componentes sem estilo](../authoring/base.md#menu): `base.Menu`, para um menu com uma aparência própria.

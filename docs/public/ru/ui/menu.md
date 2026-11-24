---
title: Меню
description: Меню от триггера, контекстное меню и их пункты.
group: Компоненты
order: 42
---

`ui.Menu` открывает пункты `ui.MenuItem` от кнопки: рядом с ней или, с `Presentation: base.PresentSheet`, в виде панели снизу — на любом устройстве. `ui.ContextMenu` открывает те же пункты у указателя.

```go preview=menu/basic height=280
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", Icon: lucide.Pencil, OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", Shortcut: "mod+d", OnSelect: duplicate},
	ui.MenuSeparator{},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

Меню открывается и закрывается само; `Open` и `OnOpenChange` позволяют приложению управлять этим. `Trigger` — внешний вид и содержимое кнопки, а нажатия на неё принимает меню. Всплывающее меню никогда не выходит за пределы места рядом с триггером: оно открывается с той стороны, где помещается целиком, а его пункты прокручиваются, если не помещаются в это место или в `MaxHeight`.

## Пункты

`ui.MenuItem` показывает `Text`, необязательный `Icon` и `Shortcut` (`"mod+c"`, во всплывающем меню записывается так, как принято на платформе), и сообщает `OnSelect`, закрывая меню, если не задан `NoCloseOnSelect`. `Disabled` отключает пункт, `Checked` показывает рядом с ним галочку, а `Variant: ui.MenuItemDanger` помечает деструктивный пункт. `ui.MenuLabel` озаглавливает группу, а `ui.MenuSeparator` разделяет группы. `ui.Submenu` содержит дополнительные пункты и открывается рядом с пунктом или, в панели, на месте окружающих пунктов.

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

## Контекстные меню

`ui.ContextMenu` открывает свои пункты по вторичному щелчку или, на сенсорных экранах, по долгому нажатию — у указателя над своим `Trigger`.

```go preview=menu/context height=220
ui.ContextMenu{Trigger: fileRow, Children: []core.Node{
	ui.MenuItem{Text: "Open", OnSelect: open},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Поведение

Escape, «назад» и нажатие снаружи закрывают меню. Меню — это всплывающий элемент, расположенный согласно `Placement` и `Align`, или панель снизу, если оно так показано. На десктопе Down, Up, Home, End, Page Down и Page Up перемещают по доступным пунктам, начиная с того, над которым находится указатель, Right открывает подменю, а Left закрывает его, и Return выбирает пункт. Меню, открытое с клавиатуры, переводит фокус на первый пункт, а при закрытии меню фокус возвращается на триггер.

## Доступность

Меню сообщает роль `RoleMenu`, а каждый пункт — `RoleMenuItem`. `Label` задаёт имя триггера, если его содержимое этого не делает. Программа чтения с экрана зачитывает текст пункта и то, отключён ли он.

## Платформы

Триггер и пункты принадлежат приложению; панель, всплывающий элемент, фокус, Escape и «назад» — платформе. Всплывающий элемент или панель выбирает приложение; NSGo не выбирает ни то, ни другое в зависимости от устройства.

## См. также

- [Выпадающий список](select.md): значение из списка.
- [Диалог](dialog.md): модальная поверхность.
- [Tabs](tabs.md): разделы, показанные одновременно.
- Страницы Menu, ContextMenu и Command в галерее.
- [Нестилизованные компоненты](../authoring/base.md#menu): `base.Menu` — для меню с собственным оформлением.

---
title: Списки и прокрутка
description: Области прокрутки с нативными жестами и списки, которые переиспользуют строки для любого количества элементов.
group: Основные концепции
order: 55
---

## Область прокрутки

`core.Scroll` — это собственная область прокрутки платформы, поэтому инерционная прокрутка, отскок, полоса прокрутки, привязка и сенсорная прокрутка работают нативно. Её дочерние элементы располагаются в столбец, а с `Horizontal` — в строку, занимая столько места, сколько им нужно.

```go
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

Область прокрутки заполняет свой контейнер, а отступы задаются её содержимому, поэтому полоса прокрутки остаётся у края. Она сжимается под размер родителя, если её стиль не указывает иное. Задайте `NoShrink`, если она должна сохранять запрошенный размер, пока сжимается соседний элемент. `HideIndicator` скрывает индикатор платформы, а `Bar` рисует собственный индикатор приложения.

`Behavior` и `BehaviorFunc` определяют, как она прокручивается: `Paging`, `SnapToInterval` и `SnapToOffsets` вместе с `SnapToAlignment`, `DecelerationRate`, `NoBounces`, `ContentInset` и правила для клавиатуры. `MinZoomScale` и `MaxZoomScale` позволяют масштабировать щипком, `OnRefresh` и `Refreshing` обновляют содержимое жестом оттягивания, а `OffsetValues` отслеживает смещение в главном потоке для эффектов, движущихся вместе с прокруткой.

```go
page := core.Scroll{
	OnScroll: func(e core.ScrollEvent) { y.Set(e.Offset.Y) },
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: []core.Node{...},
}
```

`OnScroll` сообщает смещение не чаще одного раза за такт или одного раза за `ScrollEventThrottle`. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd` и `OnScrollEnd` обрамляют жесты пользователя. `Ref` прокручивает из кода с помощью `ScrollTo`, `ScrollToEnd`, `ZoomTo` и `ScrollToIndex` для списка.

## Список

`core.List` отображает длинные данные в нативной области прокрутки, создавая только строки рядом с экраном. `Count` сообщает количество строк, а `Row` строит одну строку; `Key` даёт каждой строке идентичность, поэтому оставшиеся строки сохраняют свои ячейки, когда строки выше появляются и исчезают.

```go
core.List{
	Count:         func() int { return len(items.Get()) },
	Key:           func(i int) any { return items.Get()[i].ID },
	EstimatedSize: 56,
	Row: func(cx *core.Scope, r core.ListRow) core.Node {
		return core.View{Style: core.Style{Padding: core.All(12)}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return items.Get()[r.Index()].Title }},
		}}
	},
}
```

`Row` выполняется один раз для каждой переиспользуемой ячейки и обновляет `r.Index()`, когда ячейка используется для другой строки, поэтому читайте `r.Index()` и сигналы элементов внутри реактивных функций (`ContentFunc`, `StyleFunc`, `NewMemo`), а не запоминайте однократное значение при построении строки. Простаивающие ячейки откладываются, и их вычисления ждут, пока ячейка снова не будет привязана к живой строке.

`Header`, `Footer`, `Empty` и `Separator` содержат содержимое вокруг строк и между ними. `NumColumns` располагает строки рядом, `Gap` и `ColumnGap` разделяют их, а `Horizontal` включает горизонтальную прокрутку. `StickyHeaderIndices` закрепляет эти строки в начале области просмотра, пока следующая не вытеснит их, а `StickyHeaderOffset` оставляет над ними место для заголовка.

`OnViewableItemsChange` сообщает о видимых строках согласно порогам `ViewabilityConfig`, а `OnLoadMore` срабатывает, когда область просмотра приближается к концу, — один раз на каждое количество строк. `OnLoadMoreAtStart` делает то же самое в начале, а `InitialScrollIndex` открывает список на заданной строке. `Alignment: core.ListAlignEnd` удерживает список у его конца, как чат со строками в порядке чтения: он открывается на самой новой строке, следует за новыми строками, пока пользователь находится в конце, и держит короткую переписку внизу. `Inverted` вместо этого помещает строку 0 в конец, а `MaintainVisibleContentPosition` следует за концом растущего списка, пока пользователь находится рядом с ним.

```go
core.List{Ref: list, Count: count, EstimatedSize: 64, Row: row, OnLoadMore: loadMore,
	StickyHeaderIndices: headers(cx), Footer: core.View{Style: core.Style{Height: core.Pt(24)}}}
```

`list.ScrollToIndexAt(index, 0.5, 0, true)` прокручивает строку в середину области просмотра, а `list.ScrollToEnd(true)` — к концу. `list.ListPosition()` считывает, где начинается область просмотра — строку и смещение внутри неё, — чтобы сохранить позицию чтения: `list.ScrollToIndexAt(p.Index, 0, -p.Offset, false)` возвращает туда заново созданный список. Список также может располагаться на одной странице с другим содержимым: задайте `Viewport: core.ListViewportParent`, и окружающая его область прокрутки будет прокручивать список вместе с остальным.

## Таблицы

`base/datatable` показывает таблицу из миллиона строк с виртуализированными строками и столбцами, закреплённым заголовком, столбцами, зафиксированными слева (`Fixed`) или справа (`FixedRight`), пока остальные прокручиваются под ними, сортировкой, выбором одной или нескольких строк, изменением размера и перемещением столбцов, а также собственными ячейками приложения. `MultiSelect` поддерживает переключение с Command или Control, диапазоны с Shift и расширение выделения с клавиатуры с Shift, а `SelectedRows` и `OnSelectedRowsChange` позволяют приложению управлять строками. `CellTooltip` даёт ячейке всплывающую подсказку, которая также описывает ячейку для программ чтения с экрана и оформляется через `Styles.CellTooltip`; то, что рисует `RenderCell`, не получает событий указателя, поэтому `ui.Tooltip` внутри ячейки никогда не показывается. `base.Table` — простая таблица строк приложения для небольших данных, а `base/plot` рисует диаграммы.

Используйте `DataTable.LabelFunc` для имени таблицы, которое следует за локалью или состоянием приложения. `ColumnGroup.LabelFunc` обновляет заголовок группы столбцов, пока группа остаётся в поле зрения.

## Далее

- [Анимация](animation.md): эффекты, следующие за областью прокрутки.
- [Текст](text.md): строки текста с переносом.
- [Навигация](navigation.md): экраны с прокручиваемым содержимым.

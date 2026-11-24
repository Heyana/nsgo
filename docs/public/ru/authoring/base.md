---
title: Нестилизованные компоненты
description: Собственные элементы управления NSGo без оформления — для покомпонентной стилизации или создания поверх них библиотек со стилями.
group: Написание компонентов
order: 73
---

Пакет `base` содержит каждый элемент управления из `ui` со всем его поведением, состоянием и поддержкой доступности, но без внешнего вида. Каждая часть принимает `Class`, `Style` и `StyleFunc` ([Части и состояния](parts.md)), поэтому библиотека компонентов или приложение с собственным дизайном используют `base` вместо `ui` или оборачивают его так же, как это делает `ui` ([Оформление и темы](looks.md)). На этой странице описаны элементы управления из [справочника компонентов](../ui.md); в галерее каждый из них показан рядом со своим вариантом из `ui`.

Такие элементы управления, как `Button`, `Checkbox`, `Select`, `Slider` и `NumberInput`, принимают `Invalid: rejected.Get`, чтобы следовать за сигналом валидации. Стилизовать это состояние можно через `Class: "aria-invalid:border-red-500"` или `Style.AriaInvalid`. Оно обновляет доступность и стили, не отключая сам элемент управления. Составные поля передают это состояние своему настоящему полю ввода и декоративным частям; `ColorPicker` передаёт его своим элементам управления.

Для перетаскивания значений приложения между представлениями используйте [`base/dragdrop`](../drag-drop.md): область связывает источники и цели, а приложение задаёт их содержимое, предпросмотр и внешний вид.

## Button

`base.Button` берёт внешний вид из `Style`, а содержимое — из `Children`. `OnPress` вызывается после завершённого нажатия; `OnPressDown` и `OnPressUp` отслеживают начало и завершение нажатия для собственной обратной связи.

```go
base.Button{
	Label:   "Save",
	OnPress: func(core.PressEvent) { save() },
	Style: core.Style{Background: core.RGB(0x007aff), Padding: core.Symmetric(8, 14), CornerRadius: 8, AlignSelf: core.AlignStart,
		Active: &core.Style{Opacity: core.Alpha(0.85)}},
	Children: []core.Node{core.Text{Content: "Save", Style: core.Style{Color: core.RGB(0xffffff)}}},
}
```

`Disabled`, `Busy`, `Selected` и `Expanded` передают состояние приложению, стилям и вспомогательным технологиям: для ещё выполняющегося действия используется `Busy` (стилизуется через `aria-busy:`), для переключателя или текущего элемента списка — `Selected`, для раскрывающегося элемента — `Expanded`. `NoFocus` убирает кнопку из остановок табуляции, если доступ к ней внутри строки или списка организован иначе.

## Checkbox

`base.Checkbox` принимает стиль для рамки флажка, стиль для индикатора и ваш знак внутри него, отображаемый по значению, которым владеет приложение.

```go
base.Checkbox{
	Checked:        done.Get,
	OnChange:       done.Set,
	Label:          "Remember me",
	Style:          core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	IndicatorStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), BorderWidth: 1, CornerRadius: 5},
	Indicator:      core.Icon{SVG: lucide.Check(), Style: core.Style{FontSize: 14}},
	Children:       []core.Node{core.Text{Content: "Remember me"}},
}
```

Индикатор получает состояние от рамки флажка через `PartStates`, поэтому классы `checked:` стилизуют его, не создавая второй остановки табуляции.

Чтобы учитывать клавиши-модификаторы, задайте `OnChangeEvent func(checked bool, press core.PressEvent)`. Оно заменяет `OnChange` и оставляет владение значением у приложения; `press.Modifiers.Shift` позволяет выбирать диапазон в списке. Активация клавишами Space и Return на настольных ОС, в вебе, на Android и iOS/tvOS сохраняет модификаторы, если сочетание клавиш приложения не перехватывает клавишу раньше; в Windows комбинация Alt со Space или Return вместо этого открывает меню окна. В TSX обработчик `onChangeEvent` получает `{ checked, press }`.

## Switch

`base.Switch` состоит из дорожки, ползунка внутри неё и дочерних элементов в роли подписи. Нажатие сообщает запрашиваемое значение — противоположное `Checked`.

```go
base.Switch{
	Checked:    dark.Get,
	OnChange:   dark.Set,
	Label:      "Dark appearance",
	Style:      core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	TrackStyle: core.Style{Width: core.Pt(38), Height: core.Pt(22), CornerRadius: 11},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

Дорожка и ползунок принимают собственные функции `StyleFunc`, а их стили состояний следуют за `Checked`.

## Slider

`base.Slider` принимает `base.SliderValue`: `base.SingleValue(v)` или `base.RangeValue(low, high)`. Дорожка, индикатор, диапазон и бегунки стилизуются раздельно.

```go
level := core.NewSignal(cx, 3.0)
base.Slider{
	Value:      func() base.SliderValue { return base.SingleValue(level.Get()) },
	OnChange:   func(v base.SliderValue) { level.Set(v.End) },
	Min:        func() float64 { return 0 },
	Max:        func() float64 { return 10 },
	Step:       func() float64 { return 1 },
	Label:      "Level",
	Style:      core.Style{Height: core.Pt(28)},
	TrackStyle: core.Style{Height: core.Pt(4), CornerRadius: 2},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

`Min`, `Max` и `Step` являются реактивными геттерами: изменение их сигналов обновляет смонтированный ползунок и его диапазон доступности, в том числе во время перетаскивания. `Axis` делает его вертикальным, а `Inverted` помещает минимум на дальний конец — так же, как у `base.Progress`; `Marks` размещает ваши собственные деления на индикаторе, а `OnChangeEnd` сообщает значение после завершения перетаскивания.

## Select

`base.Select` принимает элементы `base.SelectItem` и значение `*string`, так что «ничего не выбрано» соответствует `nil`. Вы сами стилизуете триггер, всплывающий список, его строки и группы.

```go
choice := core.NewSignal[*string](cx, nil)
base.Select{
	Items: func() []base.SelectItem {
		return []base.SelectItem{{Value: "s", Title: "Small"}, {Value: "m", Title: "Medium"}}
	},
	Value:      choice.Get,
	OnChange:   choice.Set,
	Label:      "Size",
	Style:      core.Style{Height: core.Pt(28), Padding: core.Symmetric(4, 10), BorderWidth: 1, CornerRadius: 6},
	PopupStyle: core.Style{Padding: core.All(4), CornerRadius: 8},
	ItemStyle:  core.Style{Padding: core.Symmetric(4, 8), CornerRadius: 4},
}
```

Соседние элементы с одинаковым полем `Group` получают общий заголовок, быстрый набор с клавиатуры ищет совпадения среди заголовков элементов, `SearchField` отрисовывает ваше собственное поле поиска, а `Cleanable` добавляет кнопку очистки. `OnCancel` получает нажатие Escape, действие «Назад» и нажатие вне списка.

## Tabs

`base.Tabs` содержит элементы `base.Tab` и рисует индикатор с помощью `IndicatorStyle`; выбранную вкладку хранит само приложение. `base.Tab.OnPress` срабатывает при каждом нажатии, включая нажатие на уже выбранную вкладку.

```go
base.Tabs{Label: "Sections", IndicatorStyle: core.Style{Height: core.Pt(2), Background: core.RGB(0x007aff)}, Children: []core.Node{
	base.Tab{Label: "Overview", Selected: func() bool { return section.Get() == "overview" },
		OnPress: func(core.PressEvent) { section.Set("overview") }, Children: []core.Node{core.Text{Content: "Overview"}}},
	base.Tab{Label: "Activity", Selected: func() bool { return section.Get() == "activity" },
		OnPress: func(core.PressEvent) { section.Set("activity") }, Children: []core.Node{core.Text{Content: "Activity"}}},
}}
```

## Dialog

`base.Dialog` принимает стили для подложки, слоя и всплывающего окна, `Animation` или `TransitionShader`, а также ваших дочерних элементов. Управление открытием выполняется через `Open`, `DefaultOpen` и `OnOpenChange`; события `OnOpen`, `OnClose` и `OnCancel` сообщают о произошедшем. `OnConfirm` — действие по умолчанию: клавиша Return запускает его (если только фокус клавиатуры не находится на кнопке или в поле, которое само обрабатывает Return, и диалог не перекрыт другим модальным окном), после чего диалог закрывается с причиной `ReasonSelect`. `Role` и `AccessibilityDescription` задают то, что объявляют программы чтения с экрана — для подтверждений используется `RoleAlertDialog`. `NoCloseOnSwipe` отключает закрытие смахиванием на iOS, `Presentation` выбирает способ показа на iOS, а `StatusBar` изменяет строку состояния на время, пока диалог открыт.

`base.Sheet` — это выдвижная панель у любого края экрана с подложкой и вашим содержимым; при включённом `CloseOnSwipe` нижняя панель следует за движением вниз и закрывается после преодоления порога. Как и `base.Dialog`, она принимает `OnConfirm`, `Role` и `AccessibilityDescription`. `base.ShowAlertDialog` показывает системное предупреждение или список действий с кнопками по ролям:

```go
base.ShowAlertDialog(cx, base.AlertDialog{
	Title: "Delete the note?",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(label string) {
	if label == "Delete" {
		remove()
	}
})
```

## Menu

`base.Menu` открывает пункты `base.MenuItem` от вашего собственного триггера, а `base.ContextMenu` — в позиции указателя. Пункт вызывает `OnSelect` и закрывает меню, если не задан флаг `NoCloseOnSelect`; `Checked` делает пункт отмечаемым, `Indicator` рисует вашу отметку перед содержимым, а `Role` определяет, как его воспринимают вспомогательные технологии: обычный пункт меню, флажок или переключатель. `base.Submenu` объединяет вложенные пункты.

```go
base.Menu{Trigger: core.Text{Content: "Actions"}, Children: []core.Node{
	base.MenuItem{Label: "Show sidebar", Checked: sidebar.Get, OnSelect: func() { sidebar.Set(!sidebar.Get()) }},
	base.Submenu{Label: "Share", Children: []core.Node{
		base.MenuItem{Label: "Copy link", OnSelect: copyLink},
		base.MenuItem{Label: "Email", OnSelect: emailLink},
	}},
}}
```

`Placement`, `Align`, `Offset` и `Margin` позиционируют всплывающее меню на настольных ОС, а `PopupStyle` и `BackdropStyle` задают его стили.

## EmojiPicker

`base.EmojiPicker` ищет среди всех эмодзи Unicode по локализованным названиям CLDR, ключевым словам и коротким кодам на языках приложения (либо по явно заданным `Languages` / `LanguagesFunc`), предоставляя вкладки категорий, необязательный раздел `Recent`, выбор оттенка кожи, навигацию с клавиатуры и виртуализированную сетку. Все стандартные подписи и названия категорий берутся из `base/locales`, а набор данных эмодзи включается в сборку только для тех приложений, которые используют `EmojiPicker` или его вспомогательные функции (`base.SearchEmojis`, `base.EmojisInCategory`, `base.EmojiByString`).

```go
base.EmojiPicker{
	Recent:   recent.Get,
	OnSelect: func(e base.Emoji) { insert(e.Emoji) },
	Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
}
```

`Category` и `DefaultCategory` задают группу просмотра. Вкладки категорий и варианты оттенка кожи представляют собой по одной остановке табуляции с навигацией клавишами-стрелками. `EmojiPickerRef` предоставляет `Focus`, `Query`, `SetQuery`, `SetCategory` и `MatchedCount`; стилизованный выбор эмодзи использует то же состояние поискового запроса. `LabelFunc`, `SearchLabelFunc` и `PlaceholderFunc` обновляют тексты на месте. `Emoji.WithSkinTone` применяет выбранный оттенок к обоим людям в паре, а оттенок по умолчанию восстанавливает нейтральную форму; поиск по самому эмодзи сохраняет его точную последовательность.

Используйте `ui.EmojiPicker` для получения стилизованного выбора эмодзи со значками категорий из Lucide, очищаемым полем поиска, предпросмотром подсвеченного эмодзи и образцами оттенков кожи.

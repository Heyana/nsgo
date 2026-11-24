---
title: Формы
description: Текстовые поля, фокус, экранная клавиатура, переключатели, селекторы и валидация.
group: Основные концепции
order: 57
---

## Текстовые поля

`ui.Input` — это платформенное текстовое поле в стиле `ui`, обладающее всеми полями `base.Input`. При задании `Value` оно работает как управляемое: приложение хранит значение и обновляет его в `OnChange`; с `DefaultValue` поле хранит собственное состояние.

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

`Masked` скрывает символы, `Readonly` запрещает редактирование, сохраняя возможность выделять и копировать текст (в мини-программах — только в однострочном поле WeChat: многострочное поле WeChat и поля Alipay отключаются так же, как при `Disabled`, поскольку эти хосты не могут отменить правку), `Disabled` отключает поле, а `Multiline` переносит текст и растёт вместе с ним, если размеры не заданы стилем. Строки многострочного поля и его заполнителя (placeholder) имеют высоту `LineHeight` из стиля, а шрифт в них отцентрирован так же, как в `core.Text`; однострочное поле центрирует строку по своей высоте. `KeyboardType` выбирает экранную клавиатуру (`core.KeyboardEmail`, `core.KeyboardNumber`, `core.KeyboardURL` и остальные), `AutoComplete` подсказывает системе назначение поля для автозаполнения, а `MaxLength` ограничивает число кодовых единиц UTF-16, вводимых пользователем. `AutoCapitalize` и `NoAutoCorrect` настраивают поведение клавиатуры.

`OnSelectionChange` и `OnCaretMove` сообщают о выделении и положении каретки — например, для меню, открывающегося рядом с ней. `Suggest` (типа `core.Suggest`) отслеживает запрос после триггера вроде `@`, а `ui.SuggestList` рядом с полем предлагает варианты у каретки, которые можно выбрать клавишами Up, Down, Return или нажатием — как `Suggest` у `chat.Composer` для упоминаний. `OnKeyPress` получает каждое нажатие клавиши, `OnPaste` — вставляемый пользователем текст, а `OnCompositionChange` отслеживает работу метода ввода (IME).

На iOS и Android `cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, handler)` перехватывает экранную клавишу Return до того, как поле выполнит отправку, снимет фокус или вставит перенос строки. `KeyAuto` и `KeyNotEditing` оставляют это поведение за `SubmitBehavior`; на iOS нажатие Return, подтверждающее вводимый через IME текст, остаётся у метода ввода.

## Фокус и клавиатура

`Ref.Focus()` устанавливает фокус на поле или — на десктопах, в вебе и на Apple TV — на активное нажимаемое или `Focusable` представление. `Ref.Blur()` снимает фокус только со своего представления, а `cx.DismissKeyboard()` закрывает экранную клавиатуру. `OnFocus` и `OnBlur` отслеживают фокус, и `View.OnFocus`/`OnBlur` работают так же для любого фокусируемого представления. Представление с фокусом, удаляемое из дерева (когда закрывается его `Show` или исчезает строка), получает `OnBlur` один раз — после коммита, удалившего его, или, если платформа сначала сообщает о следующем фокусе, перед `OnFocus` нового представления; остающиеся вокруг представления получают `OnFocusOut`.

Запросы `Ref` на фокус, снятие фокуса, прокрутку и масштабирование ничего не делают, пока его элемент находится в содержимом `Holder`, которое нигде не отображается, а запросы, всё ещё стоящие в очереди при удалении элемента из дерева, отбрасываются. Повторный показ содержимого их не воспроизводит: запросите действие заново после размещения.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

Экранные клавиатуры перекрывают нижнюю часть экрана телефона. `core.EnvKeyboard` в качестве нижнего внутреннего или внешнего отступа удерживает содержимое над клавиатурой, а его перемещение анимируется синхронно с клавиатурой:

```go
core.View{Style: core.Style{
	Padding:    core.All(16),
	PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard},
	Gap:        12,
}, Children: form}
```

Область прокрутки вокруг формы может скрывать клавиатуру при перетаскивании и определять поведение нажатий, пока клавиатура открыта:

```go
core.Scroll{Behavior: core.ScrollBehavior{
	KeyboardDismissMode:       core.KeyboardDismissOnDrag,
	KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
}, Children: []core.Node{form}}
```

`cx.Env().KeyboardHeight()` и `cx.Env().KeyboardDuration()` считывают размер и длительность анимации клавиатуры в коде, а `cx.Env().SafeArea()` — безопасные отступы по краям экрана.

## Элементы выбора

`ui.Switch` — переключатель настройки, применяемой сразу; `ui.Checkbox` поддерживает состояния отмечен, снят или неопределённое; `ui.Radio` — один из вариантов в `ui.RadioGroup`. `ui.Select[T]` открывает список вариантов под триггером или в виде листа, если этого требует `Presentation`, а `ui.Tabs[T]` показывает несколько вариантов одновременно.

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

`ui.NumberInput` и `base.NumberInput` принимают `LabelFunc` для динамического имени доступности. Пользовательский `Format` отслеживает сигналы, которые читает, пока поле не редактируется; переключение языка сохраняет текущий черновик до завершения редактирования. `ui.NumberInput` изменяет число с шагом или перетягиванием в заданных границах, `ui.DatePicker` и `ui.TimeField` редактируют даты и время, `ui.ColorPicker` выбирает цвет, а `ui.TagInput` редактирует список тегов.

`ui.OtpInput` принимает одноразовый код по ячейкам. И он, и `base.OtpInput` принимают `LabelFunc` без замены сфокусированного поля и его кода. `Size` задаёт высоту и ширину по умолчанию: 36 точек на десктопах и ТВ и 44 на сенсорных устройствах. `CellWidth` и `CellHeight` переопределяют каждое измерение независимо. `TextStyle` переопределяет типографику ячеек, `CornerRadius` изменяет их скругления, а `CaretHeight` переопределяет высоту каретки, вычисляемую из высоты строки.

```go
quantity := core.NewSignal(cx, 1.0)
ui.NumberInput{Value: quantity.Get, OnChange: quantity.Set, Min: 1, Max: 99, Step: 1, Label: "Quantity"}
```

## Сгруппированные строки

`ui.FormSection` размещает строки на карточке с заголовком сверху и подписью снизу. Задайте `CardStyle` для фона, рамки или скруглений карточки. `CardStyleFunc` накладывается поверх него и отслеживает читаемые сигналы. `DividerInset` вызывается для каждой дочерней строки, включая строки, созданные через `core.Each`: верните отступ и `true`, чтобы переопределить разделитель, или `false`, чтобы сохранить автоматическое выравнивание по заголовку (12 точек для остальных узлов). Нулевой отступ делает разделитель во всю ширину.

`ui.FormRow` размещает `Leading` или плитку с иконкой перед заголовком. Задайте `LeadingWidth`, чтобы зарезервировать место под свой узел и выровнять разделитель под ним по заголовку. По умолчанию слот имеет ширину 24 точки. Задайте `TileSize`, чтобы масштабировать плитку иконки, глиф и скругления вместе. `TitleStyle` и `DetailStyle` переопределяют типографику заголовка и подробностей. `TitleMaxLines` ограничивает число строк заголовка (ноль — без ограничений), `MinHeight` переопределяет минимальную высоту строки в точках, а `Below` размещает узел после заголовка и подробностей в их текстовом столбце. Нулевое значение `MinHeight` сохраняет значение по умолчанию: 40 точек или 48 на сенсорных устройствах.

Для группы с одиночным выбором оберните секцию в `base.RadioGroup` и задайте каждой строке `Role: core.RoleRadio`, `Checked` и `OnPress`. Строка использует `base.Radio` для состояний доступности «отмечено» и «выбрано», одной остановки Tab и выбора стрелками; нажатие уже отмеченной строки ничего не делает. Выбранное значение хранит приложение.

`ui.FormSection.TitleFunc` и `FooterFunc`, а также `ui.FormRow.TitleFunc`, `DetailFunc` и `LabelFunc` обновляют свои тексты без перестроения строки. Пустые подробности, заголовок или подпись секции удаляют этот необязательный текст; пустая метка строки использует её текущий заголовок. Доступный для поиска `ui.Select` привязывает `SearchPlaceholderFunc` и `EmptyFunc`, сохраняя открытый поиск и текущее выделение.

## Валидация

Правила принадлежат приложению: сигнал хранит ошибку, `OnBlur` или `OnSubmit` задают её, а `core.Show` отображает сообщение под полем, пока ошибка не пуста.

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

`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` анимирует появление сообщения, а `OnSubmit` с `ReturnKeyNext` переводит фокус и клавиатуру на следующее поле.

## Далее

- [Навигация](navigation.md): где размещается форма.
- [Текст](text.md): как стилизуется текст в поле.
- [Доступность](accessibility.md): метки и значения, нужные программам экранного доступа.

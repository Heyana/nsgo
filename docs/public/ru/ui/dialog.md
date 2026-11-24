---
title: Диалог
description: Модальная поверхность, нижняя панель и оповещение.
group: Компоненты
order: 41
---

`ui.Dialog` — модальная поверхность поверх затемнённого приложения, а на сенсорных экранах — панель, выезжающая снизу. `ui.Sheet` — панель снизу, а `ui.AlertDialog` просит пользователя подтвердить действие.

```go preview=dialog/basic height=360
open := core.NewSignal(cx, false)
ui.Dialog{
	Open:         open.Get,
	OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
	Title:        "Rename the file",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { open.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

## Открытие и закрытие

Приложение хранит, открыт ли диалог. `OnOpenChange` получает каждое изменение, которое запрашивает диалог, вместе с `base.Reason`, по которому нажатие снаружи отличается от Escape или «назад»; `OnClose` выполняется после закрытия. `NoCloseOnEscape` и `NoCloseOnBackdropPress` не дают Escape и нажатию снаружи закрыть диалог.

`Title` и `Description` находятся в верхней части поверхности (`TitleFunc` и `DescriptionFunc` — когда они меняются, пока диалог открыт), а `Footer` содержит его кнопки. `Bare` отдаёт всю поверхность `Children` — для диалога с собственной раскладкой приложения, которую на десктопах обрезает край поверхности по скруглениям углов, заданным `Style` и `StyleFunc`; `Inset` задаёт минимальное расстояние от краёв окна на десктопах.

На сенсорных панелях `Bare` теперь убирает стандартный захват и отступы содержимого, чтобы `Children` заполняли панель. `NoGrabber` скрывает только захват, `ContentStyle` и `ContentStyleFunc` настраивают обёртку содержимого, а `TitleStyle` и `TitleStyleFunc` — заголовок. `Leading` и `Trailing` размещают узлы, например кнопки Cancel и Done, по краям строки заголовка, а заголовок центрируется между ними. `StartingStyle` и `EndingStyle` настраивают переход панели; добавьте в них `Transition`, если нужна собственная анимация по времени.

`Style` накладывается поверх оформления поверхности. `StyleFunc` накладывается после него и следует за читаемыми сигналами, включая внешний вид, пока диалог открыт и при повторном открытии. Например, `StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} }` следует за темой, где `colors := ui.ColorsOf(cx)`. Оба поля работают и в `ui.AlertDialog`. Нулевой цвет оставляет существующий цвет; чтобы убрать его, используйте ненулевой прозрачный цвет, например `core.RGBA(0xFFFFFF, 0)`.

## Нижние панели

`ui.Sheet` — панель внизу поверх затемнённого фона; нажатие на фон, Escape или «назад» вызывают `OnClose`.

Задайте `Label` или `LabelFunc` для доступного имени диалога и `TestID`, чтобы находить панель в тестах. `LabelFunc` следует за своими сигналами, не заменяя открытое содержимое. `CloseOnSwipe: true` также вызывает `OnClose` после того, как свайп вниз проходит порог закрытия; чтобы закрыть панель, обновите `Open` в этом обработчике.

Задайте `Style` или `StyleFunc`, чтобы настроить фон поверхности, радиусы углов и отступы, сохранив захват и обработку безопасной области. `StyleFunc` следует за читаемыми сигналами. `CornerRadius` без `CornerRadii` скругляет все углы вместо стандартного скругления сверху.

```go preview=dialog/sheet height=320
ui.Sheet{Open: filters.Get, OnClose: func() { filters.Set(false) }, Children: filterControls}
```

## Оповещения

`ui.AlertDialog` просит пользователя что-то подтвердить или принять к сведению. Каждое действие закрывает его, а нажатие снаружи — нет.

```go preview=dialog/alert height=320
ui.AlertDialog{
	Open:         confirm.Get,
	OnOpenChange: func(v bool, _ base.Reason) { confirm.Set(v) },
	Title:        "Delete the note?",
	Description:  "This cannot be undone.",
	Actions: []ui.AlertAction{
		{Text: "Cancel", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
		{Text: "Delete", Color: colors.Danger, Role: core.AlertDestructive, Default: true, OnPress: remove},
	},
}
```

Действие с `Role: core.AlertCancel` выполняется по Escape или «назад», после чего вызывается `OnOpenChange(false, base.ReasonCancel)`. `Default: true` передаёт действию фокус клавиатуры при открытии оповещения на десктопах, в вебе и на Apple TV и делает так, что его выбирает Return; нажатие на действие выполняет `OnPress`, затем `OnOpenChange(false, base.ReasonSelect)`. Return выполняет действие по умолчанию, пока фокус клавиатуры не находится на другом действии или поле: если фокус перемещён на другое действие, Return нажимает его, а диалог поверх оповещения забирает Return себе. После выполнения действия оповещение закрывается, что бы ни делал `OnOpenChange`, и не выполняет других действий, пока `Open` не станет false и снова true. Если одну и ту же отметку несут несколько действий, побеждает первое. Без отметок Escape закрывает оповещение, не выполняя действия, а у Return нет действия по умолчанию. `Role: core.AlertDestructive` описывает действие; его внешний вид выбирают `Variant` или `Color`.

Для системного оповещения или листа действий используйте `cx.ShowAlert` ([Сервисы платформы](../services.md#alerts-and-the-clipboard)).

## Поведение

Модальное окно блокирует приложение за собой, забирает фокус клавиатуры и закрывается по Escape и «назад», если не указано иное. На сенсорных экранах диалог — это панель, которая следует за перетаскиванием вниз и закрывается после порога.

## Доступность

`ui.Dialog` сообщает роль `RoleDialog` или свою явную `Role`, а `ui.AlertDialog` сообщает `RoleAlertDialog` на десктопах и сенсорных экранах. Оба зачитывают свой заголовок — или `Label`, когда заголовка недостаточно, — а затем описание. При открытии фокус переходит в диалог, а при закрытии возвращается; фон не является точкой остановки. Кнопки нижней части сохраняют свои роли и метки.

## Платформы

На Android и сенсорных экранах диалог — это панель снизу; на десктопах — поверхность по центру с затемнённым фоном. Содержимое одинаково, меняется только способ показа.

## См. также

- [Навигация](../navigation.md): страницы, вкладки и модальные окна вместе.
- [Меню](menu.md): привязанные команды и контекстные меню.
- [Доступность](../accessibility.md): фокус и роли диалогов.
- Страницы Dialog, Sheet и AlertDialog в галерее.
- [Нестилизованные компоненты](../authoring/base.md#dialog): `base.Dialog` и `base.Sheet` — со способами показа iOS и собственной анимацией.

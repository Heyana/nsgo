---
title: Флажок
description: Флажок — установленный, снятый или в неопределённом состоянии.
group: Компоненты
order: 34
---

`ui.Checkbox` — флажок в оформлении `ui` с подписью рядом. Он не хранит значение: приложение сообщает, установлен ли он, и обновляет это в `OnChange`.

```go preview=checkbox/basic
done := core.NewSignal(cx, false)
ui.Checkbox{Checked: done.Get, OnChange: done.Set, Text: "Remember me"}
```

## Содержимое

`ui.Checkbox` показывает рядом со своим квадратом `Text` или `Children` приложения. `Label` заменяет и то и другое для вспомогательных технологий, а `Hint` сообщает, для чего он нужен.

## Состояния

`Indeterminate` — ни установлен, ни снят; это состояние для «выбрать все» при частичном выборе, и нажатие тогда запрашивает установленное состояние. `Disabled` блокирует нажатия и фокус.

```go preview=checkbox/indeterminate height=220
ui.Checkbox{
	Checked:       func() bool { return chosen.Get() == len(files) },
	Indeterminate: func() bool { return chosen.Get() > 0 && chosen.Get() < len(files) },
	OnChange:      chooseAll,
	Text:          "Select all",
}
```

## Модификаторы нажатия

Используйте `OnChangeEvent` вместо `OnChange`, когда списку нужен выбор диапазона нажатием с Shift:

```go
ui.Checkbox{Checked: done.Get, Text: "Remember me", OnChangeEvent: func(next bool, press core.PressEvent) {
	if press.Modifiers.Shift {
		chooseRange(next)
		return
	}
	done.Set(next)
}}
```

Если этот обработчик задан, он заменяет `OnChange`; значение по-прежнему обновляет приложение. Активация с клавиатуры на десктопе и в вебе сохраняет модификаторы после сочетаний клавиш приложения; в Windows Alt с пробелом или Return вместо этого открывает меню окна. `onChangeEvent` в TSX получает `{ checked, press }`.

## Доступность

Флажок сообщает роль `RoleCheckbox` и своё состояние — установлен или неопределён. Его имя — `Text`, `Label` или текст дочерних элементов. Когда флажок в фокусе клавиатуры, пробел переключает его, и программа чтения с экрана озвучивает новое состояние.

## Платформы

Квадрат рисует приложение, а не элемент управления платформы, поэтому он везде выглядит одинаково. Нажатие, кольцо фокуса и программа чтения с экрана — платформенные. Он тестируется по роли и метке на каждой целевой платформе.

## См. также

- [Switch](switch.md): настройка, которая применяется сразу.
- [Формы](../forms.md): выбор вариантов в форме.
- [Список](list.md): флажки в строках.
- Страница Checkbox в галерее.
- [Нестилизованные компоненты](../authoring/base.md#checkbox): `base.Checkbox` — для флажка с собственным оформлением.

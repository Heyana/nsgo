---
title: Поле ввода
description: Текстовое поле платформы, управляемое или нет, с фокусом и экранной клавиатурой.
group: Компоненты
order: 33
---

`ui.Input` — текстовое поле платформы в оформлении `ui`, однострочное, если не задан `Multiline`. У него есть поля `base.Input`, как у любого стилизованного поля; `ui.Field` — то же поле без собственного размера, в рамке, которая не реагирует на указатель и фокус; `core.Transparent` в его `Background` и `core.Zero` в его `BorderWidth` отключают рамку — для текста, который выглядит как обычный текст, но редактируется.

```go preview=input/basic
name := core.NewSignal(cx, "")
ui.Input{
	Value:       name.Get,
	OnChange:    name.Set,
	Placeholder: "Your name",
	TestID:      "name",
}
```

## Управляемые и неуправляемые

С `Value` поле управляемое: приложение хранит значение и обновляет его в `OnChange`. С `DefaultValue` поле хранит значение само. Управляемое поле можно очистить, установив его значение в `""`.

```go preview=input/submit
draft := core.NewSignal(cx, "")
ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "New task",
	OnSubmit: func(string) { add(); draft.Set("") }}
```

## Поля

`Multiline` переносит строки, начинается с высоты в три строки и растёт вместе с текстом, если в его стиле не заданы `Height` или `MinHeight`. `Masked` скрывает символы, `Readonly` запрещает правку, но текст по-прежнему можно выделять и копировать, `Disabled` отключает поле, а `MaxLength` ограничивает число единиц UTF-16, которые вводит пользователь. `Placeholder` может быть функцией — через `PlaceholderFunc`.

```go
ui.Input{Masked: true, Value: password.Get, OnChange: password.Set,
	AutoComplete: core.AutoCompletePassword}
```

`KeyboardType` выбирает экранную клавиатуру (`KeyboardEmail`, `KeyboardNumber`, `KeyboardURL` и остальные), `ReturnKeyType` задаёт подпись клавиши ввода, а `SubmitBehavior` определяет, отправляет ли Return значение или переносит строку в многострочном поле. `AutoCapitalize`, `NoAutoCorrect` и `AutoComplete` настраивают системную помощь.

`InputSource` выбирает источник ввода клавиатуры, пока поле в фокусе, и восстанавливает источник пользователя при потере фокуса, если не задан `KeepInputSource`: `"com.apple.keylayout.ABC"` удерживает поле для кодов или команд на английском в macOS. Идентификатор — платформенный: идентификатор раскладки клавиатуры, например `"00000409"`, в Windows и метод ввода Fcitx 5 или IBus в Linux; не включённый источник, телефоны и веб ничего не меняют.

```go
ui.Input{Placeholder: "Bundle ID", InputSource: "com.apple.keylayout.ABC"}
```

`Style` накладывается поверх оформления поля, а `StyleFunc` — поверх него: то, что они задают, например высота, отступы, шрифт или цвет, заменяет значения `ui`, а остальное сохраняется. Рамка темнеет под указателем и принимает акцентный цвет в фокусе, пока ни один из них не задаёт `BorderColor` или `Hover`. Нулевой `Padding` оставляет отступы `ui`; `core.Zero` — заданная длина — убирает их (`Padding: core.All(core.Zero)`).

```go
ui.Input{Placeholder: "Filter", Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
ui.Field{DefaultValue: "Untitled", Style: core.Style{FontSize: 24, FontWeight: ui.Bold,
	Background: core.Transparent, BorderWidth: core.Zero}}
```

## Фокус и события

`Ref` устанавливает и снимает фокус с поля. `OnFocus` и `OnBlur` следят за фокусом, `OnSelectionChange` и `OnCaretMove` — за выделением и кареткой, `OnKeyPress` — за каждой нажатой клавишей, `OnPaste` — за вставленным, а `OnCompositionChange` — за составлением текста методом ввода.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

Экранная клавиатура закрывает нижнюю часть телефона. Добавьте `core.EnvKeyboard` в нижний отступ (padding или margin) содержимого, чтобы оно оставалось над клавиатурой; перемещение анимируется вместе с ней. `cx.DismissKeyboard()` закрывает клавиатуру, а `cx.Env().KeyboardHeight()` возвращает её высоту.

## Валидация

Правила задаёт приложение. Сигнал хранит ошибку, `OnBlur` или `OnSubmit` устанавливает её, а сообщение под полем её показывает ([Формы](../forms.md#validation)).

```go preview=input/validation
err := core.NewSignal(cx, "")
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{Value: email.Get, OnChange: email.Set, Label: "Email",
		OnSubmit: func(string) {
			if !strings.Contains(email.Get(), "@") {
				err.Set("Enter an email address.")
			} else {
				err.Set("")
			}
		}},
	ui.Textf(err.Get, 12, ui.Regular, colors.Danger),
}}
```

## Доступность

Поле сообщает роль `RoleTextField` или `RoleSearchField` для поля поиска, если так указывает его `Role`, и зачитывает свой `Label`; `Placeholder` — не метка, поэтому полю без видимой подписи задайте `Label`. Состояния `Readonly` и `Disabled` сообщаются, а `OnSubmit` — действие клавиатуры. Однострочные, маскированные и многострочные поля в macOS, iOS и Linux предоставляют текущий `Placeholder` как нативный заполнитель или подсказку доступности, которые обновляются и очищаются без второй точки фокуса для рисуемой метки-заполнителя многострочного поля.

## Платформы

iOS, Android, Windows и Linux используют свои нативные поля; веб — `<input>` или `<textarea>`. Экранная клавиатура, автозаполнение, методы ввода и направление текста — платформенные. Фокус и ввод текста тестируются одинаково на каждой целевой платформе.

## См. также

- [Формы](../forms.md): поля, валидация, выбор вариантов и клавиатура.
- [Доступность](../accessibility.md): метки и фокус.
- [Раскладка](../layout.md): как держать содержимое над клавиатурой.
- Страницы Input и Form в галерее.

---
title: Части и состояния
description: Не храните состояние приложения, стилизуйте каждую часть и сообщайте вспомогательным технологиям о назначении компонента.
group: Написание компонентов
order: 71
---

Нестилизованный компонент определяет, как элемент управления ведёт себя, и оставляет внешний вид тому, кто его использует. На этой странице мы построим `Toggle` — переключатель в форме пилюли, который может быть включён или выключен — точно так же, как устроены элементы управления в `base`.

## Управляемые значения

Элемент управления не хранит значение, которым владеет приложение: приложение передаёт его в виде функции и обновляет из обратного вызова. Компонент считывает его там, где оно отображается, поэтому при изменении обновляется только эта часть.

```go
// Toggle is a pill that is on or off. Its parts are the pill and a dot
// inside it, then its children, the label.
type Toggle struct {
	On       func() bool
	Disabled func() bool
	// OnChange receives the value a press asks for, the opposite of On's.
	OnChange func(on bool)

	Class        string
	Style        core.Style
	StyleFunc    func() core.Style
	DotClass     string
	DotStyle     core.Style
	DotStyleFunc func() core.Style
	Children     []core.Node
	Label        string
}
```

Приложение хранит значение в сигнале: `chips.Toggle{On: bold.Get, OnChange: bold.Set, Label: "Bold"}`.

## Части

Каждая часть, которую отрисовывает компонент, принимает `Class`, `Style` и `StyleFunc`, названные по имени этой части (`DotClass`, `DotStyle`, `DotStyleFunc`). `Style` задаётся статически, `StyleFunc` отслеживает читаемые в ней сигналы, а класс применяется под обоими, чтобы приложение могло использовать любой удобный способ.

Часть получает состояние элемента управления через `PartStates`, вместо того чтобы объявлять собственное, благодаря чему классы `checked:` и стили `Checked` применяются к ней, а вспомогательные технологии видят единый элемент управления:

```go
func (t Toggle) Render(*core.Scope) core.Node {
	on := func() bool { return t.On != nil && t.On() }
	off := func() bool { return t.Disabled != nil && t.Disabled() }
	shows := func() core.PartStates { return core.PartStates{Checked: on(), Disabled: off()} }
	dot := core.View{Class: t.DotClass, Style: t.DotStyle, StyleFunc: t.DotStyleFunc, PartStates: shows}
	return core.View{
		Class: t.Class, Style: t.Style, StyleFunc: t.StyleFunc,
		Role: core.RoleSwitch, Label: t.Label, Checked: on, Disabled: t.Disabled,
		OnPress: func(core.PressEvent) {
			if t.OnChange != nil {
				t.OnChange(!on())
			}
		},
		Children: append([]core.Node{dot}, t.Children...),
	}
}
```

Приложение может стилизовать его одними лишь классами:

```go
chips.Toggle{
	On: bold.Get, OnChange: bold.Set, Label: "Bold",
	Class:    "flex-row items-center gap-2 rounded-full border border-zinc-300 px-3 py-1 checked:bg-blue-50",
	DotClass: "size-2 rounded-full bg-zinc-300 checked:bg-blue-500",
	Children: []core.Node{core.Text{Content: "Bold"}},
}
```

Аналогичным образом `PartStates.Invalid` применяет классы `aria-invalid:` и `Style.AriaInvalid` к декоративной части, включая стили `group-aria-invalid:` внутри неё, не объявляя при этом ещё один невалидный элемент управления. Устанавливайте `Invalid` на самом элементе управления или нативном поле ввода, чтобы вспомогательные технологии получали его состояние валидации.

## Доступность

Корневое представление элемента управления сообщает, чем он является: `Role`, `Label` (если его текст не раскрывает назначение), `Hint` для описания действия при активации и соответствующие флаги `Checked`, `Selected`, `Disabled`, `Invalid` или `Expanded`. Его внутренние части остаются безмолвными. Элемент управления с переходом между пунктами по клавишам-стрелкам представляет собой одну остановку табуляции, как список вкладок. На странице [Доступность](../accessibility.md) перечислены все роли и действия.

## Что пробрасывать дальше

Компонент, корнем которого является его собственное представление, должен принимать `OnLayout`, `Tooltip` и `TestID` этого представления и передавать их дальше, чтобы приложения могли измерять его, снабжать подсказкой и тестировать как любое обычное представление. Обратные вызовы необязательны: при `OnChange == nil` элемент управления просто отображает значение без его изменения.

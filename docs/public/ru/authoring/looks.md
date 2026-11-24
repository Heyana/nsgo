---
title: Оформление и темы
description: Берите цвета из действующей темы и добавляйте компоненту варианты и размеры.
group: Написание компонентов
order: 72
---

Стилизованный компонент выбирает только стили и ничего больше: он оборачивает нестилизованный компонент в отдельном пакете, например `chipsui`, и оставляет всё поведение там. Его цвета берутся из темы, поэтому он автоматически следует теме приложения, собственной теме панели и светлому или тёмному оформлению без пересоздания.

## Цвета из темы

Считывайте цвета внутри `StyleFunc`: эта функция запускается повторно при изменении всего, что она прочитала, поэтому смена темы или оформления перестилизует компонент на месте. `cx.Palette()` возвращает палитру, действующую там, где расположен компонент, а `ui.ColorsOf(cx)` именует цветовые роли, используемые пакетом `ui`.

```go
// Toggle is chips.Toggle in the theme's colors.
type Toggle struct {
	On       func() bool
	OnChange func(bool)
	Label    string
}

func (t Toggle) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	on := func() bool { return t.On != nil && t.On() }
	return chips.Toggle{On: t.On, OnChange: t.OnChange, Label: t.Label,
		Class: "flex-row items-center gap-2 rounded-full px-3 py-1",
		StyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.Accent()}
			}
			return core.Style{Background: colors.Inset()}
		},
		DotClass: "size-2 rounded-full",
		DotStyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.OnAccent()}
			}
			return core.Style{Background: colors.Faint()}
		},
		Children: []core.Node{ui.Text(t.Label, 13, ui.Medium, colors.Text)},
	}
}
```

Жёстко заданные цвета ломаются в тёмном режиме и при использовании темы приложения; берите каждый цвет из роли.

## Собственные роли

Цвет, для которого в палитре нет готовой роли, добавляется в `Palette.Colors` под вашим собственным именем и задаётся для обоих режимов оформления. Приложения могут переопределять его в коде или в файле темы в разделах `light` и `dark`:

```go
theme := core.DefaultTheme()
theme.Light.SetColor("chip", core.RGB(0xeef5ff))
theme.Dark.SetColor("chip", core.RGB(0x1b2a3d))
cx.SetTheme(theme)
```

Компонент считывает его через `cx.Palette().Color("chip")`, которая также сообщает, задан ли этот цвет в теме, чтобы компонент мог откатиться к стандартной роли, если цвет отсутствует. Градиенты, тени и прочие декоративные эффекты не являются цветами: храните их в коде, выбирая по `cx.Dark()`.

## Варианты

Компонент с несколькими вариантами внешнего вида принимает `Variant` — отдельный тип, нулевое значение которого соответствует оформлению по умолчанию. Он перечисляет только визуальные варианты компонента, включая цвет, вместо булевых полей или общего типа цвета:

```go
// ToggleVariant is a toggle's look.
type ToggleVariant uint8

const (
	// ToggleSolid fills the pill with the accent when on.
	ToggleSolid ToggleVariant = iota
	// ToggleOutline draws its edge in the accent instead.
	ToggleOutline
)
```

## Размеры

Большинству компонентов достаточно одного размера, благодаря чему элементы управления выстраиваются в ряд без необходимости выбора со стороны приложения. Если компоненту всё же нужны другие размеры, он принимает `Size` — значение `ui.Size` (обычный, малый или большой), чтобы соответствовать высотам `ui.Button` (28, 24 и 32 пункта); метод `Size.Height()` возвращает высоту для выравнивания с другими элементами управления. Компонент, сохраняющий одинаковую форму в любом масштабе, например аватар, вместо этого принимает `Size` в пунктах.

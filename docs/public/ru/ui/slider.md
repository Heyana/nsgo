---
title: Ползунок
description: Одно значение или диапазон на дорожке, с управлением клавиатурой и указателем.
group: Компоненты
order: 37
---

`ui.Slider` выбирает значение на дорожке перетаскиванием, с шагом и с клавиатуры. Одно значение — это один бегунок; `High` превращает его в диапазон из двух.

```go preview=slider/basic
volume := core.NewSignal(cx, 40.0)
ui.Slider{Value: volume.Get, OnChange: volume.Set, Min: 0, Max: 100, Step: 5, Label: "Volume"}
```

## Диапазон

Задайте `High` и `OnHighChange`, чтобы выбирать два значения. `Value` — нижняя граница. `LowLabel` и `HighLabel` называют бегунки для вспомогательных технологий.

```go preview=slider/range
price := core.NewSignal(cx, 20.0)
ceiling := core.NewSignal(cx, 80.0)
ui.Slider{
	Value: price.Get, OnChange: price.Set,
	High: ceiling.Get, OnHighChange: ceiling.Set,
	Min: 0, Max: 100, Step: 1,
	Label: "Price", LowLabel: "Minimum", HighLabel: "Maximum",
}
```

## Шаги и значения

`Step` привязывает значение к шагу; при шаге 0 допускается любое значение. `ValueText` меняет то, что вспомогательные технологии зачитывают как значение, а `Disabled` отключает ползунок.

```go preview=slider/format
ui.Slider{Value: speed.Get, OnChange: speed.Set, Min: 0.5, Max: 2, Step: 0.25, Label: "Speed",
	ValueText: func(v float64) string { return fmt.Sprintf("%g×", v) }}
```

Используйте `MinFunc`, `MaxFunc` и `StepFunc`, когда диапазон или шаг меняются — например, когда видео сообщает свою длительность. Они переопределяют соответствующие статические поля и обновляют смонтированный ползунок, сохраняя его фокус и текущее перетаскивание. Явный `MaxFunc`, возвращающий 0, допускает схлопнутый диапазон до получения длительности; значение приложения сохраняется и отображается в пределах текущих границ.

```go
ui.Slider{Value: position.Get, OnChange: position.Set, MaxFunc: duration.Get, Label: "Position"}
```

В TSX `min={minimum()}`, `max={duration()}` и `step={step()}` следуют за своими сигналами на месте; `max={0}` схлопывает диапазон так же, как `MaxFunc`, возвращающий 0.

## Направление

`Axis: base.Vertical` ставит ползунок вертикально, с минимумом внизу; задайте ему высоту в `Style`. `Inverted` переносит минимум на другой конец: наверх у вертикального ползунка и в конец у горизонтального. Клавиши со стрелками вдоль дорожки следуют направлению. `ui.Progress` принимает те же `Axis` и `Inverted`.

```go
ui.Slider{Value: level.Get, OnChange: level.Set, Axis: base.Vertical, Label: "Level", Style: core.Style{Height: core.Pt(160)}}
```

## Поведение

Перетаскивание следует за указателем и сообщает `OnChange` во время движения. Клавиши со стрелками сдвигают бегунок в фокусе на один шаг, Home и End переводят его к краям, а Page Up и Page Down сдвигают дальше. Когда `Step` больше 0, `ui.Slider` размечает шаги под дорожкой — не более 60 отметок.

## Доступность

Ползунок сообщает `RoleAdjustable`, свой `Label` и значение; у диапазона два бегунка, каждый со своей меткой. Вместо числа зачитывается `ValueText`. Значение меняется с клавиатуры и действиями увеличения и уменьшения программы чтения с экрана.

## Платформы

Дорожку рисует приложение, и она везде одного размера. Нажатия, фокус и программа чтения с экрана — платформенные; перетаскивание касанием ведёт себя как перетаскивание указателем с тем же шагом.

## См. также

- [Формы](../forms.md): ползунки в форме.
- [Доступность](../accessibility.md): значения и регулируемые роли.
- Страницы Slider, RangeSlider и Rating в галерее.
- [Нестилизованные компоненты](../authoring/base.md#slider): `base.Slider`, вертикальный и с собственными отметками.

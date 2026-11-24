---
title: Текст
description: Абзацы, фрагменты, ссылки, шрифты и документы.
group: Компоненты
order: 32
---

`core.Text` — это один абзац, набранный текстовым движком платформы. `ui.Text` и `ui.Textf` — вспомогательные функции для строки в типографике `ui`; элемент по-прежнему остаётся `core.Text`, поэтому он также принимает `Style` и `Spans`.

```go preview=text/basic
ui.Text("Settings", 20, ui.Bold, colors.Text)
ui.Textf(func() string { return fmt.Sprintf("%d files", n.Get()) }, 13, ui.Regular, colors.Muted)
```

## Содержимое

`Content` неизменен, а `ContentFunc` следует за сигналом. `MaxLines` ограничивает число строк, а `EllipsizeMode` обрезает остаток в начале, середине или конце; `Style.WhiteSpaceNoWrap` удерживает абзац в одной строке. `TextAlign`, `TextTransform`, `LineHeight` и `LetterSpacing` оформляют абзац, а `OnTextLayout` сообщает его строки после раскладки.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) },
	MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 15}}
```

## Фрагменты и ссылки

`Spans` составляет текст из фрагментов, у каждого из которых свой стиль. Фрагмент с `OnPress` является ссылкой, а его `Background` и `CornerRadius` рисуются за его символами на каждой строке. `SpansFunc` заново выдаёт фрагменты, когда меняется то, что он читает, — для подсвеченного кода или изменяющегося предложения.

```go preview=text/spans
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "Read the "},
	{Content: "guide", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openGuide},
	{Content: " before you start."},
}}
```

`Attachment` вставляет представление в строку вместо содержимого фрагмента: значок, изображение или небольшую кнопку. Оно раскладывается в собственном размере, переносится вместе со словами и само обрабатывает нажатия.

## Шрифты и стиль

`FontFamily` называет семейство или CSS-список. `FontSize`, `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LetterSpacing`, `TextDecoration`, а также `TextShadowColor` с его смещением и радиусом оформляют текст. `core.FontMonospace` — моноширинное семейство платформы.

```go
core.Text{Content: "fn main() {}", Style: core.Style{FontFamily: core.FontMonospace, FontSize: 13}}
```

Регистрируйте упакованные шрифты через `nsgo.Fonts` до первого представления или регистрируйте начертание во время работы приложения через `cx.RegisterFont`. Текст, подготовленный до появления шрифта, подготавливается заново.

## Документы

`base/textview.TextView` показывает Markdown или HTML с помощью тех же примитивов: заголовки, абзацы, списки, таблицы, код, сноски, предупреждения, изображения, формулы и ссылки. `textview.Parse` читает Markdown, а `textview.ParseHTML` — HTML; `Options.Streaming` исправляет документ, который ещё поступает.

```go preview=text/document height=240
textview.TextView{Markdown: "## Notes\n\nA **native** document.", Styles: ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) { open(e.URL) }}
```

`textview.NewSearch` ищет в тексте, который показывает `TextView`, и подсвечивает совпадения. Дайте нескольким представлениям общий поиск через `textview.ProvideSearch`, затем вызывайте `Open`, `SetQuery`, `Next` и `Previous`; `Count` и `Selected` сообщают результат. Части поиска оформляются через `textview.PartSearch` и `textview.PartSearchSelected`.

## Выделение и доступность

`SelectableOn` позволяет пользователю выделять и копировать текст. `core.View` с `Selectable` делает так, что его тексты выделяются вместе в порядке документа. `Role`, например `RoleHeader`, сообщает вспомогательным технологиям, чем является текст, а `Label` заменяет его, когда слова не являются меткой.

## Платформы

Текст раскладывает текстовый движок платформы, поэтому переносы строк, шрифты и методы ввода следуют ему. Перенос строк CJK, эмодзи и текст справа налево следуют системе. Приложение может узнать размер текста через `cx.Env().FontScale()`, и текст следует ему, если не задан `NoFontScaling`.

## См. также

- [Текст](../text.md): полное руководство по тексту, фрагментам, шрифтам и документам.
- [Изображения и значки](../images.md): встроенные изображения и значки.
- [Доступность](../accessibility.md): заголовки, метки и выделение.
- Страницы Text и TextView в галерее.

---
title: Раскладка
description: Flexbox, grid, размеры и позиционирование, с учётом безопасной области и клавиатуры в раскладке.
group: Основные концепции
order: 51
---

## Блоки

Каждый элемент — это блок, который раскладывает движок. Значения по умолчанию такие же, как в React Native: flexbox в виде колонки, дочерние элементы сохраняют свой размер, если они не растягиваются и не сжимаются, элементы растягиваются вдоль поперечной оси, а размеры включают внутренние отступы и рамки.

```go
core.View{
	Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.All(16), AlignItems: core.AlignCenter},
	Children: []core.Node{
		core.Text{Content: "One"},
		core.Text{Content: "Two"},
	},
}
```

`Column`, `Row`, `ColumnReverse` и `RowReverse` задают главную ось; `Wrap` и `WrapReverse` позволяют дочерним элементам переноситься на дополнительные строки. `JustifyContent` размещает их вдоль этой оси, а `AlignItems` (или `AlignSelf` для одного дочернего элемента) — поперёк неё. `Gap`, `RowGap` и `ColumnGap` разделяют их, а внутренние и внешние отступы в `style` используют `core.All`, `core.Symmetric` или `core.Edges`.

## Размеры

`Width`, `Height`, `MinWidth`, `MaxWidth` и `FlexBasis` принимают `core.Pt` для точек или `core.Pct` для процента от родителя. `FlexGrow` распределяет свободное пространство, а `FlexShrink` позволяет блоку отдать его часть; блок без обоих сохраняет свой размер. `AspectRatio` — это ширина, делённая на высоту. `AutoMargin` занимает свободное пространство с указанных сторон, так что одна сторона отталкивает блок к другой.

```go
core.View{Style: core.Style{Direction: core.Row, Gap: 16}, Children: []core.Node{
	core.View{Style: core.Style{Width: core.Pt(220), FlexShrink: 0}},
	core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}},
}}
```

Размер блока определяется его содержимым, пока ширина или высота не задают иное. `core.MinContent`, `core.MaxContent` и `core.FitContent` принимают размеры, названные так в CSS: ширина min-content у текста — это его самое широкое слово. Текст внутри строки сжимается и переносится только с `FlexShrink: 1`; добавьте `FlexGrow: 1`, если он должен занять оставшееся пространство. Незаданный минимум уже равен 0, как в React Native, поэтому `MinWidth: core.Pt(0)` ничего не меняет, а сжимающийся текст разбивает слова, когда строка уже его самого широкого слова. `MinWidth: core.AutoMinimum` останавливает его на самом широком слове, как это делает автоматический минимум в CSS.

Экран, заполняющий окно, получает `FlexGrow: 1` и `FlexShrink: 1`, как и каждый блок между ним и списком или прокручиваемым представлением, которое должно прокручиваться. Только с `FlexGrow` блок растёт до размера своего содержимого, поэтому длинный список растягивает его за пределы окна вместо прокрутки.

## Сетка

`DisplayGrid` раскладывает дочерние элементы в CSS grid. `GridTemplateColumns` и `GridTemplateRows` принимают синтаксис дорожек CSS, а `GridRow` и `GridColumn` размещают элемент по линии или по охвату.

```go
core.View{Style: core.Style{
	Display: core.DisplayGrid,
	GridTemplateColumns: "repeat(3, 1fr)",
	GridAutoRows: "44px",
	Gap: 12,
}, Children: []core.Node{
	core.View{Style: core.Style{GridColumn: core.GridLines{Start: core.GridSpan(2)}}},
}}
```

`repeat(auto-fill, minmax(160px, 1fr))` создаёт столько колонок, сколько помещается. `GridFlow` выбирает порядок заполнения сетки, а `JustifyItems`, `JustifySelf` и `AlignItems` размещают элементы в их ячейках.

## Позиционирование

`Relative` — значение по умолчанию, оно оставляет блок в потоке; `Top`, `Right`, `Bottom` и `Left` смещают его. `Absolute` извлекает блок из потока и размещает его в ближайшем позиционированном предке. `Fixed` размещает его в корневом представлении или в его модальном окне независимо от прокрутки, а `Sticky` закрепляет его внутри родителя в ближайшем прокручиваемом представлении: `Top` — от верхнего края, или `Bottom` — от нижнего, когда `Top` равен auto (`Left` и `Right` — по горизонтали). Нижний колонтитул с `Position: core.Sticky, Bottom: core.Pt(0)` остаётся внизу, пока прокручивается длинное содержимое, и следует за коротким содержимым. `ZIndex` упорядочивает блоки по глубине среди соседних элементов.

```go
core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Right: core.Pt(8)}}
```

`Static` оставляет блок в потоке и игнорирует его смещения, поэтому абсолютно позиционированные дочерние элементы находят предка выше него. `Overflow` обрезает дочерние элементы по границам блока, а `DisplayNone` исключает блок из раскладки.

Собственные внешние тени и обводка блока остаются вне его области обрезки, как в CSS, на Android, Windows, Linux, в вебе и на платформах Apple — там для всех видов представлений, включая углы с разными радиусами и масштабированное прокручиваемое содержимое. Пока это не проверено на физическом iPhone, `Bare` у `ui.Dialog` по-прежнему помещает обрезку во внутреннее представление с теми же углами, отступающее на ширину рамки, что сохраняет их на всех целевых платформах:

```go
core.View{Style: core.Style{CornerRadius: 16, Shadows: []core.BoxShadow{ui.Shadow(0, 8, 24, -6, ui.Black(0.18))}},
	Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, CornerRadius: 16, Overflow: core.Hidden}, Children: content}}}
```

## Направление и окружение

`LayoutDirection` раскладывает поддерево слева направо или справа налево поверх направления приложения: его задают `DirectionRTL` и `DirectionLTR`, а значение по умолчанию наследуется. Левая и правая стороны во внешних и внутренних отступах, рамках и смещениях меняются местами в зависимости от направления, поэтому они являются началом и концом, как в React Native. `PhysicalEdges` и `PhysicalBorders` оставляют названные стороны на месте. `WritingDirection` упорядочивает фрагменты текста со смешанным направлением, а `WritingAuto` берёт его по первой букве с сильной направленностью.

`PaddingEnv` и `MarginEnv` добавляют к стороне значение окружения. `core.EnvSafeArea` — это отступ безопасной области на этом краю, а `core.EnvKeyboard` — высота программной клавиатуры, так что содержимое остаётся над клавиатурой, а его перемещение анимируется вместе с ней.

```go
core.View{Style: core.Style{
	Padding: core.All(16),
	PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea},
}}
```

`core.EnvAll(core.EnvSafeArea)` задаёт все края, а `cx.Env().SafeArea()` и `cx.Env().KeyboardHeight()` читают значения в коде.

На складном телефоне или телефоне с двумя экранами `cx.Env().Geometry()` перечисляет области, на которые сгиб или шарнир делит окно, — каждую с попадающей в неё безопасной областью — и сгибы между ними; его `State` и `Angle` сообщают, насколько раскрыто устройство (`core.HingeClosed`, `HingeHalfOpen` или `HingeOpen`), даже если ни один сгиб не пересекает окно, как на внешнем дисплее или полностью раскрытом экране. Другие устройства сообщают об одной области и `HingeUnknown`. На iOS 27.1 `cx.Env().VerticalBarEdge()` сообщает край, который система выбрала для вертикальной панели (`core.VerticalBarLeading`, `core.VerticalBarTrailing` или `core.VerticalBarUnspecified`), и `mobile.TabBar` и `ui.TabShell` следуют ему, когда `Edge` равен `mobile.TabBarAutomatic`. Неявно по геометрии ничего не раскладывается: приложение само выбирает свои панели.

## Адаптивные классы

`Class` принимает классы Tailwind CSS: `flex`, `flex-col`, `flex-wrap`, `items-center`, `justify-between`, `gap-3`, `p-4`, `w-full`, `grid`, `grid-cols-3`, `col-span-2` и остальные. Контрольные точки следуют за окном, а оформление дисплея, направление и уменьшение движения следуют за окружением:

```go
core.View{Class: "flex flex-col gap-3 p-4 md:flex-row md:gap-6 dark:bg-zinc-900 rtl:flex-row-reverse"}
```

`sm:`, `md:`, `lg:`, `xl:` и `2xl:` применяются начиная с ширины 40, 48, 64, 80 и 96 rem; `max-md:` применяется ниже соответствующей ширины. `portrait:`, `dark:`, `rtl:`, `motion-reduce:`, `contrast-more:`, `pointer-fine:`, `pointer-coarse:`, `ios:`, `android:` и `web:` следуют тому, что сообщает окружение. Классы объединяются под `Style`, а класс состояния (`hover:`) становится стилем состояния, поэтому их можно сочетать.

## Далее

- [Стили и темы](styles.md): цвета, углы, тени и внешний вид поддерева.
- [Текст](text.md): как текст получает свой размер и переносится.
- [Списки и прокрутка](lists.md): прокручиваемое содержимое и переиспользуемые строки.

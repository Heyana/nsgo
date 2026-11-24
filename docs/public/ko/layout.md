---
title: 레이아웃
description: 안전 영역과 키보드를 레이아웃에 포함한 Flexbox, 그리드, 크기 및 위치 지정.
group: 핵심 개념
order: 51
---

## 박스

모든 엘리먼트는 엔진이 배치하는 박스입니다. 기본값은 React Native와 동일합니다. 열(column) 방향 flexbox, 늘어나거나(grow) 줄어들도록(shrink) 지정하지 않는 한 자신의 크기를 유지하는 자식, 교차축(cross axis)으로 늘어나는 아이템, 그리고 패딩과 테두리를 포함하는 크기 기준을 따릅니다.

```go
core.View{
	Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.All(16), AlignItems: core.AlignCenter},
	Children: []core.Node{
		core.Text{Content: "One"},
		core.Text{Content: "Two"},
	},
}
```

`Column`, `Row`, `ColumnReverse`, `RowReverse`는 주축(main axis)을 설정하고, `Wrap`과 `WrapReverse`는 자식이 여러 줄로 넘쳐 흐를 수 있게 합니다. `JustifyContent`는 주축을 따라 자식을 배치하며, `AlignItems` 또는 개별 자식의 `AlignSelf`는 교차축을 기준으로 배치합니다. `Gap`, `RowGap`, `ColumnGap`은 자식 사이의 간격을 지정하며, `style`의 패딩과 마진은 `core.All`, `core.Symmetric` 또는 `core.Edges`를 사용합니다.

## 크기

`Width`, `Height`, `MinWidth`, `MaxWidth`, `FlexBasis`는 포인트 단위의 `core.Pt` 또는 부모 대비 백분율인 `core.Pct`를 받습니다. `FlexGrow`는 남은 공간을 나누어 가지며 `FlexShrink`는 박스가 공간을 양보할 수 있게 합니다. 둘 다 없는 박스는 자신의 크기를 유지합니다. `AspectRatio`는 너비를 높이로 나눈 값입니다. `AutoMargin`은 지정한 방향의 남은 공간을 차지하므로, 한쪽에 지정하면 박스를 반대쪽으로 밀어냅니다.

```go
core.View{Style: core.Style{Direction: core.Row, Gap: 16}, Children: []core.Node{
	core.View{Style: core.Style{Width: core.Pt(220), FlexShrink: 0}},
	core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}},
}}
```

너비나 높이가 별도로 지정되지 않는 한 박스의 크기는 콘텐츠에 의해 결정됩니다. `core.MinContent`, `core.MaxContent`, `core.FitContent`은 CSS가 정의한 크기 값을 사용합니다(예: 텍스트의 min-content 너비는 가장 넓은 단어의 너비입니다). 행(row) 안의 텍스트는 `FlexShrink: 1`이 있어야만 줄어들고 줄바꿈되며, 남은 공간을 차지해야 할 때는 `FlexGrow: 1`을 추가합니다. React Native와 마찬가지로 설정되지 않은 최솟값은 이미 0이므로 `MinWidth: core.Pt(0)`은 아무것도 바꾸지 않으며, 행이 가장 넓은 단어보다 좁아지면 줄어드는 텍스트가 단어 중간에서 줄바꿈됩니다. `MinWidth: core.AutoMinimum`은 CSS의 자동 최솟값(automatic minimum)처럼 가장 넓은 단어 너비에서 멈추게 합니다.

창을 채우는 화면에는 `FlexGrow: 1`과 `FlexShrink: 1`을 지정하며, 그 화면과 스크롤되어야 하는 목록 또는 스크롤 뷰 사이에 있는 각 박스에도 동일하게 지정합니다. `FlexGrow`만 지정하면 박스가 콘텐츠 크기에 맞춰 커지므로 긴 목록이 스크롤되는 대신 창 밖으로 넘쳐 늘어나게 됩니다.

## 그리드

`DisplayGrid`는 자식을 CSS 그리드로 배치합니다. `GridTemplateColumns`와 `GridTemplateRows`는 CSS 트랙 문법을 받으며, `GridRow`와 `GridColumn`은 라인 번호나 스팬(span)으로 아이템을 배치합니다.

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

`repeat(auto-fill, minmax(160px, 1fr))`은 공간에 맞는 만큼 열을 생성합니다. `GridFlow`는 그리드가 채워지는 순서를 정하며, `JustifyItems`, `JustifySelf`, `AlignItems`는 각 셀 안에서 아이템을 배치합니다.

## 위치 지정

`Relative`는 기본값으로 박스를 레이아웃 흐름 안에 두며 `Top`, `Right`, `Bottom`, `Left`로 오프셋을 줍니다. `Absolute`는 박스를 흐름에서 빼내어 가장 가까운 위치 지정된(positioned) 조상 기준으로 배치합니다. `Fixed`는 스크롤 여부와 관계없이 루트 뷰 또는 해당 모달을 기준으로 배치하며, `Sticky`는 가장 가까운 스크롤 뷰 안에서 부모 내부에 고정합니다(`Top`은 위쪽 기준, `Top`이 auto일 때는 `Bottom`으로 아래쪽 기준, 가로 방향은 `Left`와 `Right` 기준). `Position: core.Sticky, Bottom: core.Pt(0)`이 지정된 푸터는 긴 콘텐츠가 스크롤되는 동안 하단에 고정되며, 짧은 콘텐츠에서는 콘텐츠 바로 뒤에 이어서 배치됩니다. `ZIndex`는 형제 박스 간의 쌓임 순서를 결정합니다.

```go
core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Right: core.Pt(8)}}
```

`Static`은 박스를 흐름 안에 유지하고 인셋(inset)을 무시하므로 절대 위치(`Absolute`)로 배치된 자식이 그 위쪽의 조상을 기준으로 삼게 됩니다. `Overflow`는 자식을 박스 영역에 맞춰 잘라내며(clip), `DisplayNone`은 박스를 레이아웃에서 제외합니다.

Android, Windows, Linux, 웹 및 Apple 플랫폼에서 박스의 바깥쪽 그림자와 외곽선(outline)은 CSS와 마찬가지로 클립 영역 밖에 그대로 남으며, 각기 다른 반지름을 가진 모서리나 확대된 스크롤 콘텐츠를 포함한 모든 종류의 뷰에 적용됩니다. 실물 iPhone에서 이를 검증하기 전까지 `ui.Dialog`의 `Bare`는 테두리만큼 안쪽으로 들어간 동일한 모서리 반지름을 가진 내부 뷰에 클립을 적용하여 모든 대상에서 그림자와 외곽선이 유지되도록 합니다.

```go
core.View{Style: core.Style{CornerRadius: 16, Shadows: []core.BoxShadow{ui.Shadow(0, 8, 24, -6, ui.Black(0.18))}},
	Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, CornerRadius: 16, Overflow: core.Hidden}, Children: content}}}
```

## 방향과 환경

`LayoutDirection`은 앱의 기본 방향을 덮어써서 서브트리를 왼쪽에서 오른쪽(LTR) 또는 오른쪽에서 왼쪽(RTL)으로 배치합니다. `DirectionRTL`과 `DirectionLTR`로 설정하며 기본값은 상속입니다. 마진, 패딩, 테두리, 인셋의 Left와 Right는 방향에 따라 서로 바뀌므로 React Native와 마찬가지로 start와 end 역할을 합니다. `PhysicalEdges`와 `PhysicalBorders`는 지정한 물리적 방향을 그대로 유지합니다. `WritingDirection`은 양방향 텍스트 구간의 순서를 정하며, `WritingAuto`는 첫 번째 강한 방향성 문자(strong letter)에서 방향을 결정합니다.

`PaddingEnv`와 `MarginEnv`는 지정한 면에 환경 인셋을 더합니다. `core.EnvSafeArea`는 해당 가장자리의 안전 영역 인셋이고, `core.EnvKeyboard`는 소프트웨어 키보드의 높이이므로 키보드가 애니메이션으로 움직이는 동안에도 콘텐츠가 키보드 위에 유지됩니다.

```go
core.View{Style: core.Style{
	Padding: core.All(16),
	PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea},
}}
```

`core.EnvAll(core.EnvSafeArea)`는 모든 가장자리를 지정하며, 코드에서는 `cx.Env().SafeArea()`와 `cx.Env().KeyboardHeight()`로 해당 값을 읽습니다.

폴더블 또는 듀얼 스크린 휴대폰에서 `cx.Env().Geometry()`는 접힘선(fold)이나 힌지(hinge)가 창을 나누는 영역들(각 영역에 해당하는 안전 영역 포함)과 영역 사이의 접힘선 정보를 나열합니다. `State`와 `Angle`은 커버 디스플레이나 완전히 평평하게 편 화면처럼 창을 가로지르는 접힘선이 없을 때에도 기기가 얼마나 열려 있는지(`core.HingeClosed`, `HingeHalfOpen` 또는 `HingeOpen`)를 알려줍니다. 그 외의 기기는 하나의 영역과 `HingeUnknown`을 보고합니다. iOS 27.1에서는 `cx.Env().VerticalBarEdge()`가 시스템이 세로 막대용으로 선택한 가장자리(`core.VerticalBarLeading`, `core.VerticalBarTrailing` 또는 `core.VerticalBarUnspecified`)를 보고하며, `Edge`가 `mobile.TabBarAutomatic`일 때 `mobile.TabBar`와 `ui.TabShell`이 이를 따릅니다. 지오메트리에 따라 암묵적으로 배치되는 것은 없으며, 앱이 직접 영역(pane) 구성을 선택합니다.

## 반응형 클래스

`Class`는 `flex`, `flex-col`, `flex-wrap`, `items-center`, `justify-between`, `gap-3`, `p-4`, `w-full`, `grid`, `grid-cols-3`, `col-span-2` 등의 Tailwind CSS 클래스를 받습니다. 중단점(breakpoint)은 창 크기를 따르며, 디스플레이 외관, 방향 및 동작 줄이기는 환경을 따릅니다.

```go
core.View{Class: "flex flex-col gap-3 p-4 md:flex-row md:gap-6 dark:bg-zinc-900 rtl:flex-row-reverse"}
```

`sm:`, `md:`, `lg:`, `xl:`, `2xl:`은 각각 너비 40, 48, 64, 80, 96 rem 이상에서 적용되며, `max-md:`는 해당 중단점 미만에서 적용됩니다. `portrait:`, `dark:`, `rtl:`, `motion-reduce:`, `contrast-more:`, `pointer-fine:`, `pointer-coarse:`, `ios:`, `android:`, `web:`은 환경 설정을 따릅니다. 클래스는 `Style` 아래로 병합되며 상태 클래스(`hover:`)는 상태 스타일이 되므로 둘을 함께 사용할 수 있습니다.

## 다음

- [스타일과 테마](styles.md): 색상, 모서리, 그림자 및 서브트리의 외관.
- [텍스트](text.md): 텍스트가 크기를 결정하고 줄바꿈되는 방식.
- [목록과 스크롤](lists.md): 스크롤되는 콘텐츠와 재사용되는 행.

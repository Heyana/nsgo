---
title: 스타일과 테마
description: 타입이 지정된 스타일, Tailwind 클래스, 상태 스타일 및 라이트/다크 테마.
group: 핵심 개념
order: 52
---

## 스타일

`core.Style`은 박스의 레이아웃, 외관, 텍스트를 담는 하나의 Go 값입니다. 모든 필드의 제로 값은 미설정 상태이므로 스타일끼리 합성할 수 있습니다. `Merge`는 뒤쪽 스타일이 설정한 항목만 덮어쓰며, 자식은 부모가 설정한 텍스트 필드를 상속합니다. 길이가 지정된 스타일 위에 길이를 없애려면, 색상에서 `core.Transparent`를 사용하는 것처럼 `Padding`, `Margin`, `Borders`, `BorderWidth`, `CornerRadius` 또는 간격(gap)에 `core.Zero`를 사용하세요. 예: `Padding: core.All(core.Zero)`.

```go
card := core.Style{
	Background: core.RGB(0xffffff),
	CornerRadius: 12,
	Padding: core.All(16),
	Gap: 12,
	BorderWidth: 1,
	BorderColor: core.RGBA(0x000000, 0.1),
}
```

스타일은 CSS의 색상, 모서리, 테두리, 외곽선(outline), 그림자, 그라데이션, 필터, 배경 흐림(backdrop blur), 마스크, 혼합 모드(blend mode), 3D 변환(transform) 및 트랜지션을 지원합니다. `core.RGB`, `core.RGBA`, `core.ParseColor`는 색상을 만들고, `core.LinearGradient`와 관련 함수들은 그라데이션을, `core.Ring`은 Tailwind의 링을, `core.AllCorners`는 각 모서리의 반경을 지정합니다. `BorderCurveContinuous`는 iOS처럼 모서리를 가장자리에 맞춰 부드럽게 둥글게 만듭니다. 박스의 그림자와 외곽선은 [레이아웃](layout.md#position)에 명시된 경우를 제외하면 `Overflow` 클리핑 영역 바깥에 그대로 그려집니다. 배경 필터(backdrop filter)는 CSS의 `backdrop-filter`처럼 뷰 뒤쪽에 있는 콘텐츠를 변형합니다. `Blur`는 지정된 반경만큼 뒤쪽을 흐리게 만들고, 색상 필터는 색상을 변경합니다. iOS에서는 색상 필터가 없는 흐림 효과에 `View.IOS.BackdropMaterial`이 지정한 시스템 머티리얼을 그립니다. `core.BackdropUltraThin` 외의 머티리얼을 지정한 흐림 효과는 색상 필터를 생략한 채 iOS에서는 해당 머티리얼을, macOS에서는 AppKit의 가장 가까운 머티리얼을 그립니다. Linux와 Android는 배경 필터를 그리지 않습니다. `Style.GlassEffect`(`core.GlassRegular` 또는 `core.GlassClear`, TSX에서는 `glassEffect`)는 macOS 26과 iOS 26에서 배경 흐림 대신 Liquid Glass를 그립니다(Windows에서는 NSGo의 글래스 셰이더를 사용하며, 그 외 플랫폼을 위해 글래스 뷰에는 폴백용 흐림 효과를 함께 지정해 두세요). `Style.GlassInteractive`(TSX에서는 `glassInteractive`)를 설정하면 `OnPress`와 누를 수 있는 자식 뷰가 계속 동작하는 상태에서 macOS 27(`NSGlassEffectView.effectIsInteractive`) 및 iOS 26(`UIGlassEffect.isInteractive`)의 Liquid Glass용 시스템 인터랙티브 누름 응답이 활성화됩니다.

`Transform`은 포인터 대상 영역도 뷰와 함께 이동시킵니다. 누르기, 호버, 제스처는 뷰가 실제로 그려진 위치에서 시작되며, 보고되는 포인터 좌표는 변환을 역적용해 계산됩니다. 변환되어 부모 영역 바깥으로 나간 자식 뷰도 클립이나 스크롤 뷰포트에 가려지지 않는 한 계속 상호작용할 수 있습니다. 미니 프로그램에서는 렌더 레이어가 실행하는 키프레임의 역변환을 아직 지원하지 않습니다([로드맵](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md#mini-programs)).

```go
core.View{Style: core.Style{
	CornerRadii: core.Corners{TopLeft: 28, BottomRight: 28},
	BorderCurve: core.BorderCurveContinuous,
	Shadows:     []core.BoxShadow{core.Ring(1, core.RGBA(0x000000, 0.08)), ui.Shadow(0, 8, 24, -6, ui.Black(0.18))},
}}
```

## 상태

뷰의 외관은 상태에 따라 바뀔 수 있습니다. `Hover`, `Focus`, `FocusWithin`, `Active`, `Selected`, `Checked`, `Disabled` 및 `aria-*` 상태들은 해당 상태가 유지되는 동안 나머지 스타일 위에 병합되는 `*Style` 값입니다. 앱은 함수를 통해 `Selected`, `Checked`, `Disabled`, `Expanded`를 설정하며, 플랫폼은 누름과 포커스를 보고합니다.

`Active`와 `group-active:`는 뷰가 눌려 있는 동안을 따르므로, 터치를 유지하는 누르기 핸들러나 `Responder`가 필요합니다. 일반 뷰에 `Active`, `active:` 또는 `group` 스타일을 지정한다고 해서 누를 수 있게 되거나 키보드 Tab 이동 대상이 되지는 않습니다. 뷰가 직접 포커스를 받아야 한다면 `Focusable`을 설정하세요.

```go
core.View{
	OnPress: func(core.PressEvent) { open() },
	Style: core.Style{
		Background: core.RGB(0x007aff),
		Active:     &core.Style{Opacity: core.Alpha(0.85)},
		Hover:      &core.Style{Background: core.RGB(0x006be0)},
	},
	Disabled: func() bool { return saving },
}
```

컴포넌트의 하위 파트는 상태를 직접 보고하지 않고도 자신을 감싼 컨트롤의 상태를 이어받을 수 있습니다. `PartStates`가 `Checked`, `Selected`, `Disabled` 등을 전달하므로 체크박스의 표시(mark)도 스크린 리더에서 체크박스의 상태를 함께 따르게 됩니다.

## 클래스

`View`, `Text`, `Span`, `Input`, `Image`, `Scroll`, `Canvas`의 `Class`는 Tailwind CSS 4 클래스를 받으며, `base` 컴포넌트의 모든 파트 역시 스타일과 함께 클래스를 가집니다. 유틸리티, 변형(variant), 임의 값(arbitrary value)은 Tailwind의 기본 테마와 팔레트에서와 같은 의미로 동작합니다.

```go
core.View{Class: "flex items-center gap-3 rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"}
```

클래스는 스타일 아래에 병합되므로 스타일 필드가 우선합니다. 클래스는 `ClassFunc`를 통해 변경 사항을 따라갈 수 있으며, 상태 변형(`hover:bg-zinc-100`, `checked:opacity-100`)은 해당 스타일의 상태로 변환됩니다. `group`과 `group-hover:`는 부모의 상태에 따라 자식의 스타일을 지정하고, `first:`와 `last:`는 자식의 위치를 따르며, `aria-checked:`, `data-open:`, `focus-within:`은 컨트롤이 나타내는 상태를 따릅니다. `pointer-fine:`과 `pointer-coarse:`는 `cx.Env().Pointer()`(웹에서는 브라우저의 `pointer` 쿼리, 데스크톱과 Mac에서는 fine, 휴대폰·태블릿·워치에서는 coarse, TV에서는 둘 다 아님)를 통해 CSS의 `pointer` 미디어 기능과 일치하는 방식으로 마우스나 터치스크린용 서브트리 스타일을 지정합니다. 이 변형들은 `dark:`나 `hover:` 같은 다른 변형들과 조합될 수 있어, `core`가 암묵적으로 크기를 바꾸는 대신 앱이 터치 또는 포인터 크기를 명시적으로 선택할 수 있게 합니다.

컨트롤에 `peer`를 표시하면 그 상태에 따라 뒤따르는 형제 뷰의 스타일을 지정할 수 있습니다. 예를 들어 `Class: "peer"`를 가진 체크박스 뒤에 `Class: "peer-checked:text-accent peer-disabled:opacity-50"`를 가진 레이블을 둘 수 있습니다. 피어(peer)는 `Show`와 `Each`를 포함해 같은 부모 아래에서 자신보다 뒤에 있는 형제 뷰에만 스타일을 적용하며, 앞선 형제나 자기 자신 내부의 뷰에는 적용하지 않습니다. 특정 피어 그룹에 이름을 붙이려면 `peer/field`와 `peer-checked/field:`를 사용하고, 컨트롤의 불리언 `invalid` 데이터 상태에는 `peer-data-invalid:`를 사용하세요. 마커와 상태는 반응형으로 업데이트됩니다. 뷰에 마커를 표시한다고 해서 Tab 이동 대상이 되거나 누르기를 받을 수 있게 되지는 않으므로, `peer-active:`는 버튼처럼 직접 누르기를 처리하는 피어만 따릅니다.

`has-[>svg]:px-3`은 부모가 직계 아이콘 자식을 가질 때 부모의 패딩을 변경하고, `[&_svg]:size-4 [&_svg]:shrink-0`은 하위 아이콘들의 크기를 지정합니다. `svg`는 조건부 영역 안을 포함해 `core.Icon`과 `SVGView`를 선택합니다. 이 선택자들은 자식, 클래스 또는 상태가 바뀔 때 업데이트됩니다. `hover:[&_svg]`는 부모의 호버를 따르고, `[&_svg]:hover`는 각 아이콘의 호버를 따릅니다. `peer-active:`와 마찬가지로 선택자의 `active`는 버튼처럼 누르기를 처리하는 뷰만 따릅니다. 일치하는 선택자들은 변형 순서보다 먼저 명시도(specificity)로 우선순위를 겨루며, 클래스가 important가 아닌 한 인라인 스타일이 우선합니다.

선택자는 모든 대상 플랫폼에서 NSGo 엘리먼트와 일치합니다. 목록, 자식(`>`), 자손(`_`), 인접 형제(`+`), 일반 형제(`~`) 결합자, 클래스, `data-*` 존재 여부/동등성, 역할(role), 지원되는 불리언 접근성 상태, 그리고 `:is`, `:where`, `:not`, `:has` 및 자식 위치 선택자를 지원합니다. 타입은 `view`/`div`, `text`/`span`(전체 텍스트 노드), `scroll`, `input`, `textarea`, `image`/`img`, `canvas`, `svg`, `video`, `webview`, `native`입니다. 각 임의 선택자 분기(alternative)에는 최상위 `&`를 하나 사용하세요. `&`가 없으면 선택자는 해당 엘리먼트 자체를 검사합니다. 브라우저 전용 선택자, 의사 요소(pseudo-element), at-규칙(at-rule), 이스케이프, 그리고 함수 내부에 명시된 `&`는 인식되지 않으며, 이 선택자들은 백엔드 구현 뷰나 개별 텍스트 스팬(span)에는 닿지 않습니다.

`Span.Class`나 `Screen.HeaderClass`의 `peer-*` 변형에는 따를 수 있는 형제 요소가 없습니다. 이들은 무시되며 클래스 및 컨텍스트별로 한 번씩 로그가 기록됩니다. 스팬의 피어 변형은 스팬을 감싸고 있는 `Text`에 대신 지정하세요.

테마의 `Tailwind` 필드는 기본 테마를 색상, 중단점(breakpoint), 유틸리티로 확장하며, 기본 팔레트에 없는 색상 이름은 현재 표시 중인 외관(appearance)의 노드 테마에서 읽어 온 `Palette` 역할(`bg-accent`, `text-text-muted/70`)이나 테마 색상을 가리킵니다.

## 테마

`core.Theme`은 라이트와 다크 두 외관을 모두 포함하는 하나의 스타일 테마로, 라이트용과 다크용 `Palette`, 모서리 반경, 글꼴 패밀리를 담고 있습니다. 팔레트의 역할은 `Background`, `Surface`, `Text`, `TextMuted`, `Border`, `Accent`, `AccentText`, `Danger`, `Warning`, `Success`, `Selection`, `Focus`, `Scrollbar`이며, `Colors`에는 `inset`이나 `raised`처럼 라이브러리가 추가한 역할들이 들어갑니다. 앱이 다른 테마를 설정하기 전까지 표시되는 `core.DefaultTheme()`은 파란색 강조 색상과 `ui`의 역할들을 갖춘 뉴트럴 테마이며, `ui.WarmDuskTheme()`은 따뜻한 톤의 팔레트입니다.

```go
theme := core.DefaultTheme()
theme.Light.Accent, theme.Dark.Accent = core.RGB(0xbd481b), core.RGB(0xed986e)
theme.Radius = 10
cx.SetTheme(theme)
```

`cx.SetTheme`은 해당 스코프와 그 아래에서 빌드되는 모든 요소에 테마를 지정하므로, 사이드바나 패널에 각기 다른 테마를 적용할 수 있습니다. nil 테마를 전달하면 상위 스코프의 테마를 따릅니다. `core.ParseTheme`은 역할 색상, 반경, 글꼴 패밀리가 포함된 YAML 또는 JSON 테마 파일이나 shadcn/ui 테마 파일의 CSS 변수를 읽어옵니다. 앱이 테마를 지정하기 전까지 사용되는 `core.DefaultTheme`과 `ui.WarmDuskTheme`은 수정 가능한 상태의 테마를 반환합니다.

`cx.SetAppearance`는 앱 전체를 라이트 또는 다크로 강제하고, `cx.SetSubtreeAppearance`는 서브트리에 대해 이를 지정하며, `core.AppearanceSystem`은 다시 시스템 설정을 따르게 합니다. `cx.Dark()`는 서브트리가 어떤 외관을 표시하는지 알려주고, `cx.Palette()`는 임베딩 호스트의 재정의(override)가 적용된 색상을 반환합니다.

어떤 조상 요소도 `Style.Color`를 설정하지 않은 경우, 텍스트와 `currentColor` 아이콘은 서브트리의 `Palette.Text`를 사용하며 테마, 외관 및 호스트 토큰 변경을 즉시 반영합니다. `core.Transparent`를 포함해 명시적으로 지정된 색상은 계속해서 상속됩니다.

## 컴포넌트의 색상

컴포넌트는 자신이 빌드되는 위치와 `StyleFunc` 안에서 색상을 읽어오므로, 컴포넌트를 다시 빌드하지 않고도 테마나 외관 변경이 그 자리에서 바로 반영됩니다. `ui.ColorsOf(cx)`는 `ui`가 사용하는 역할들을 제공하고, `ui.TintsOf(cx)`는 그 반투명 채우기를, `ui.FinOf(cx)`는 `ui`의 계층형 마감(layered finish)을 제공합니다.

```go
colors := ui.ColorsOf(cx)
core.Text{Content: "Hello", StyleFunc: func() core.Style {
	return core.Style{Color: colors.Text(), FontSize: 15}
}}
```

`StyleFunc`는 그 안에서 읽은 값이 변경될 때 다시 실행되므로, 내부의 `cx.Dark()`와 `cx.Palette()`는 테마를 따라갑니다. 이는 `ClassFunc`, 스팬의 `StyleFunc`, 그리고 컴포넌트의 `StyleFunc` 필드에도 동일하게 적용됩니다.

## 커서

`Style.Cursor`는 이름이 지정된 포인터 커서(`core.CursorText`, `core.CursorColResize` 등)를 설정합니다. 누를 수 있는 요소는 플랫폼의 기본 화살표 커서를 유지하도록 `core.CursorAuto`로 두세요. 데스크톱과 웹에서 그리기 도구, 게임 또는 커스텀 캔버스 포인터를 구현할 때는 `Style.CursorImage`에 `*core.CustomCursor`를 전달할 수 있으며, 여기에는 1x `Image`, 선택적인 HiDPI용 `Image2x`(`PixelRatio >= 1.5`일 때 자동 선택), 이미지 경계 내로 제한되는 논리 포인트 단위의 `Hotspot`, 그리고 `Fallback` 이름 지정 커서를 지정합니다. 현재 대상 플랫폼이 커스텀 커서 이미지를 표시하는지 확인하려면 `cx.Env().SupportsCustomCursor()`를 호출하세요. 터치 플랫폼에서는 커스텀 커서 이미지가 무시됩니다.

## 다음 단계

- [레이아웃](layout.md): 박스가 배치되는 방식.
- [텍스트](text.md): 글꼴, 스팬 및 텍스트 꾸밈.
- [애니메이션](animation.md): 스타일 간의 트랜지션.

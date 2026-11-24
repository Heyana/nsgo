---
title: 외형과 테마
description: 적용 중인 테마에서 색상을 가져오고 컴포넌트에 변형과 크기를 부여합니다.
group: 컴포넌트 작성
order: 72
---

스타일이 적용된 컴포넌트는 스타일 선택 외에 다른 일은 하지 않습니다. `chipsui`처럼 별도의 패키지 안에서 스타일 없는 컴포넌트를 감싸며, 동작 로직은 원래 컴포넌트에 그대로 둡니다. 색상은 테마에서 가져오므로 다시 빌드하지 않고도 앱의 테마, 패널 고유의 테마, 라이트/다크 모드를 자연스럽게 따릅니다.

## 테마의 색상

색상은 `StyleFunc` 내부에서 읽으세요. `StyleFunc`는 자신이 읽은 값이 바뀔 때마다 다시 실행되므로, 테마나 화면 모드가 바뀌면 컴포넌트가 그 자리에서 다시 스타일링됩니다. `cx.Palette()`는 컴포넌트가 위치한 곳의 팔레트를 반환하고, `ui.ColorsOf(cx)`는 `ui`가 사용하는 색상 역할(role)들을 제공합니다.

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

하드코딩된 색상은 다크 모드나 앱의 커스텀 테마에서 어색하게 깨집니다. 모든 색상은 역할에서 가져오세요.

## 커스텀 역할

팔레트에 적절한 역할이 없는 색상은 `Palette.Colors`에 직접 이름을 정해 넣고, 라이트와 다크 모드 양쪽에 모두 설정합니다. 앱은 코드에서 또는 테마 파일의 `light`와 `dark` 아래에서 이 값을 재정의할 수 있습니다:

```go
theme := core.DefaultTheme()
theme.Light.SetColor("chip", core.RGB(0xeef5ff))
theme.Dark.SetColor("chip", core.RGB(0x1b2a3d))
cx.SetTheme(theme)
```

컴포넌트는 `cx.Palette().Color("chip")`으로 이 색상을 읽으며, 테마에 해당 색상이 있는지 여부도 함께 반환되므로 없을 때 대체할 기본 역할로 폴백할 수 있습니다. 그라데이션, 그림자 및 기타 마감 효과는 색상이 아니므로 `cx.Dark()`로 분기해 코드에 두세요.

## 변형

여러 외형을 갖는 컴포넌트는 제로 값이 기본 외형인 전용 `Variant` 타입을 받습니다. 불리언 필드나 공용 색상 타입을 쓰는 대신, 컴포넌트가 실제로 그리는 외형(색상 포함)만 열거해 이름을 붙이세요:

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

## 크기

대부분의 컴포넌트에는 하나의 크기만 있으면 충분하며, 그래야 앱이 매번 고르지 않아도 컨트롤들이 나란히 정렬됩니다. 더 많은 크기가 필요한 컴포넌트는 기본, 작음, 큼을 나타내는 `ui.Size` 타입의 `Size` 필드를 받아 `ui.Button`의 28, 24, 32포인트와 높이를 맞춥니다. `Size.Height()`는 다른 컨트롤과 높이를 나란히 맞추는 데 필요한 값을 반환합니다. 아바타처럼 어떤 크기에서도 동일한 형태를 유지하는 컴포넌트는 대신 포인트 단위의 `Size`를 받습니다.

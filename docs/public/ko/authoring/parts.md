---
title: 부분과 상태
description: 앱이 소유한 상태를 보관하지 않고, 모든 부분에 스타일을 지정하며, 보조 기술에 컴포넌트의 정체를 알립니다.
group: 컴포넌트 작성
order: 71
---

스타일 없는 컴포넌트는 컨트롤이 어떻게 동작하는지만 결정하고, 어떻게 보일지는 사용하는 쪽에 맡깁니다. 이 페이지에서는 `base`가 컨트롤을 만드는 방식대로 켜고 끌 수 있는 알약 모양의 `Toggle`을 만들어 봅니다.

## 제어되는 값

컨트롤은 앱이 소유한 값을 스스로 보관하지 않습니다. 앱이 값을 함수로 전달하고 콜백으로 값을 변경합니다. 컴포넌트는 값이 표시되는 위치에서 함수를 읽으므로, 값이 바뀌어도 해당 부분만 업데이트됩니다.

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

앱은 이 값을 시그널에 보관합니다: `chips.Toggle{On: bold.Get, OnChange: bold.Set, Label: "Bold"}`.

## 부분

컴포넌트가 그리는 각 부분은 부분 이름을 붙인 `Class`, `Style`, `StyleFunc`를 받습니다(`DotClass`, `DotStyle`, `DotStyleFunc`). `Style`은 고정 스타일이고, `StyleFunc`는 내부에서 읽은 시그널을 추적하며, 클래스는 두 스타일 아래에 기본으로 깔리므로 앱이 원하는 어떤 방식으로든 스타일을 지정할 수 있습니다.

부분은 자체 상태를 보고하는 대신 `PartStates`를 통해 컨트롤의 상태를 전달받습니다. 덕분에 보조 기술에는 하나의 컨트롤로 보이면서도 `checked:` 클래스와 `Checked` 스타일이 각 부분에 적용됩니다:

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

앱은 클래스만으로도 스타일을 지정할 수 있습니다:

```go
chips.Toggle{
	On: bold.Get, OnChange: bold.Set, Label: "Bold",
	Class:    "flex-row items-center gap-2 rounded-full border border-zinc-300 px-3 py-1 checked:bg-blue-50",
	DotClass: "size-2 rounded-full bg-zinc-300 checked:bg-blue-500",
	Children: []core.Node{core.Text{Content: "Bold"}},
}
```

마찬가지로 `PartStates.Invalid`는 별도의 유효하지 않은 컨트롤을 보고하지 않으면서도 장식 부분에 `aria-invalid:` 클래스와 `Style.AriaInvalid`(내부 요소의 `group-aria-invalid:` 스타일 포함)를 적용합니다. 보조 기술이 유효성 검사 상태를 수신할 수 있도록 `Invalid`는 실제 컨트롤이나 네이티브 입력에 설정하세요.

## 접근성

컨트롤의 루트는 자신이 무엇인지 알립니다. 즉 `Role`, 텍스트만으로 설명되지 않을 때의 `Label`, 활성화 시 동작을 설명하는 `Hint`, 그리고 상황에 따라 `Checked`, `Selected`, `Disabled`, `Invalid`, `Expanded`를 설정합니다. 하위 부분들은 조용히 숨겨집니다. 탭 리스트처럼 화살표 키로 항목 사이를 이동하는 컨트롤은 하나의 Tab 정지점으로 만듭니다. 역할과 동작 목록은 [접근성](../accessibility.md)에 정리되어 있습니다.

## 그대로 전달할 속성

루트가 자체 뷰인 컴포넌트는 해당 뷰의 `OnLayout`, `Tooltip`, `TestID`를 받아 그대로 전달해야 앱이 일반 뷰처럼 크기를 측정하고 설명을 달고 테스트할 수 있습니다. 콜백은 선택 사항이어야 하며, `OnChange`가 `nil`이면 값을 바꾸지 않고 보여 주기만 하는 컨트롤이 됩니다.

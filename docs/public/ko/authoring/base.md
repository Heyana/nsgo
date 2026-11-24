---
title: 스타일 없는 컴포넌트
description: 부분별로 스타일을 지정하거나 스타일이 적용된 라이브러리를 구축할 수 있는, 외형이 없는 NSGo 자체 컨트롤.
group: 컴포넌트 작성
order: 73
---

`base`에는 `ui`가 가진 모든 컨트롤이 외형 없이 동작, 상태, 접근성만 갖춘 형태로 들어 있습니다. 각 부분은 `Class`, `Style`, `StyleFunc`를 받으므로([부분과 상태](parts.md)), 독자적인 디자인을 가진 라이브러리나 앱은 `ui` 대신 이를 직접 사용하거나 `ui`처럼 이를 감싸서 구현합니다([외형과 테마](looks.md)). 이 페이지는 [컴포넌트 레퍼런스](../ui.md)에 있는 컨트롤들을 다루며, 갤러리에서는 각 컨트롤을 `ui` 버전과 나란히 확인할 수 있습니다.

`Button`, `Checkbox`, `Select`, `Slider`, `NumberInput` 같은 컨트롤은 유효성 검사 시그널을 따르도록 `Invalid: rejected.Get`을 받습니다. 이 상태는 `Class: "aria-invalid:border-red-500"` 또는 `Style.AriaInvalid`로 스타일링하세요. 이는 컨트롤을 비활성화하지 않고도 접근성과 스타일을 업데이트합니다. 복합 필드는 이 상태를 실제 입력과 장식 부분으로 전달하며, `ColorPicker`는 내부 컨트롤들과 공유합니다.

뷰 사이에 애플리케이션 값을 드래그하려면 [`base/dragdrop`](../drag-drop.md)을 사용하세요. 드래그 영역이 소스와 타겟을 연결하고, 앱이 콘텐츠, 미리보기, 외형을 제공합니다.

## Button

`base.Button`은 `Style`에서 외형을, `Children`에서 콘텐츠를 가져옵니다. `OnPress`는 누르기가 완료되었을 때 실행되며, `OnPressDown`과 `OnPressUp`은 누르기 시작과 끝을 따라 자체 피드백을 줄 때 사용합니다.

```go
base.Button{
	Label:   "Save",
	OnPress: func(core.PressEvent) { save() },
	Style: core.Style{Background: core.RGB(0x007aff), Padding: core.Symmetric(8, 14), CornerRadius: 8, AlignSelf: core.AlignStart,
		Active: &core.Style{Opacity: core.Alpha(0.85)}},
	Children: []core.Node{core.Text{Content: "Save", Style: core.Style{Color: core.RGB(0xffffff)}}},
}
```

`Disabled`, `Busy`, `Selected`, `Expanded`는 앱, 스타일, 보조 기술에 상태를 보고합니다. 아직 실행 중인 작업에는 `Busy`(`aria-busy:`로 스타일링)를, 토글이나 리스트의 현재 항목에는 `Selected`를, 펼침 요소에는 `Expanded`를 사용합니다. `NoFocus`는 행이나 리스트가 다른 방식으로 버튼에 도달할 때 버튼을 Tab 정지점에서 제외합니다.

## Checkbox

`base.Checkbox`는 박스의 스타일, 인디케이터의 스타일, 그리고 그 안에 들어갈 마크를 받으며, 앱이 소유한 값에 따라 표시합니다.

```go
base.Checkbox{
	Checked:        done.Get,
	OnChange:       done.Set,
	Label:          "Remember me",
	Style:          core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	IndicatorStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), BorderWidth: 1, CornerRadius: 5},
	Indicator:      core.Icon{SVG: lucide.Check(), Style: core.Style{FontSize: 14}},
	Children:       []core.Node{core.Text{Content: "Remember me"}},
}
```

인디케이터는 `PartStates`를 통해 박스로부터 상태를 전달받으므로, 별도의 Tab 정지점을 만들지 않고도 `checked:` 클래스로 스타일링할 수 있습니다.

보조 키(modifier)가 필요하면 `OnChangeEvent func(checked bool, press core.PressEvent)`를 설정하세요. 이 콜백은 `OnChange`를 대체하고 값의 관리는 앱에 맡기며, `press.Modifiers.Shift`를 사용해 리스트에서 범위 선택을 구현할 수 있습니다. 데스크톱, 웹, Android, iOS/tvOS에서 Space/Return으로 활성화할 때는 앱 단축키가 먼저 키를 처리하지 않는 한 보조 키 상태가 유지됩니다. Windows에서는 Alt와 함께 Space나 Return을 누르면 창 메뉴로 이동합니다. TSX에서는 `onChangeEvent`가 `{ checked, press }`를 받습니다.

## Switch

`base.Switch`는 트랙, 그 안의 썸(thumb), 그리고 레이블 역할을 하는 자식 요소로 구성됩니다. 누르면 `Checked`의 반대 값으로 변경을 요청합니다.

```go
base.Switch{
	Checked:    dark.Get,
	OnChange:   dark.Set,
	Label:      "Dark appearance",
	Style:      core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	TrackStyle: core.Style{Width: core.Pt(38), Height: core.Pt(22), CornerRadius: 11},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

트랙과 썸은 각각 자체 `StyleFunc`를 받으며, 이들의 상태 스타일은 `Checked`를 따릅니다.

## Slider

`base.Slider`는 `base.SliderValue`(`base.SingleValue(v)` 또는 `base.RangeValue(low, high)`)를 받습니다. 트랙, 인디케이터, 범위, 썸의 스타일을 각각 따로 지정할 수 있습니다.

```go
level := core.NewSignal(cx, 3.0)
base.Slider{
	Value:      func() base.SliderValue { return base.SingleValue(level.Get()) },
	OnChange:   func(v base.SliderValue) { level.Set(v.End) },
	Min:        func() float64 { return 0 },
	Max:        func() float64 { return 10 },
	Step:       func() float64 { return 1 },
	Label:      "Level",
	Style:      core.Style{Height: core.Pt(28)},
	TrackStyle: core.Style{Height: core.Pt(4), CornerRadius: 2},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

`Min`, `Max`, `Step`은 반응형 게터(getter)이므로, 드래그 중을 포함해 시그널이 바뀌면 마운트된 슬라이더와 접근성 범위가 함께 업데이트됩니다. `Axis`는 슬라이더를 수직으로 만들고, `Inverted`는 `base.Progress`처럼 최솟값을 반대쪽 끝에 둡니다. `Marks`는 인디케이터 위에 직접 만든 눈금을 배치하고, `OnChangeEnd`는 드래그가 멈췄을 때 값을 보고합니다.

## Select

`base.Select`는 `base.SelectItem` 목록과 `*string` 값을 받으므로 "선택 안 됨"은 `nil`로 표현합니다. 트리거, 팝업, 행, 그룹의 스타일을 직접 지정합니다.

```go
choice := core.NewSignal[*string](cx, nil)
base.Select{
	Items: func() []base.SelectItem {
		return []base.SelectItem{{Value: "s", Title: "Small"}, {Value: "m", Title: "Medium"}}
	},
	Value:      choice.Get,
	OnChange:   choice.Set,
	Label:      "Size",
	Style:      core.Style{Height: core.Pt(28), Padding: core.Symmetric(4, 10), BorderWidth: 1, CornerRadius: 6},
	PopupStyle: core.Style{Padding: core.All(4), CornerRadius: 8},
	ItemStyle:  core.Style{Padding: core.Symmetric(4, 8), CornerRadius: 4},
}
```

인접한 항목들의 `Group`이 같으면 헤더를 공유하고, 타이프어헤드(typeahead)는 항목 제목을 매칭합니다. `SearchField`는 직접 만든 검색 필드를 그려 주고, `Cleanable`은 지우기 버튼을 표시합니다. `OnCancel`은 Escape, 뒤로 가기, 외부 클릭을 수신합니다.

## Tabs

`base.Tabs`는 `base.Tab`들을 담고 `IndicatorStyle`로 인디케이터를 그립니다. 어떤 탭이 선택되었는지는 앱이 관리합니다. `base.Tab.OnPress`는 이미 선택된 탭을 포함해 누를 때마다 실행됩니다.

```go
base.Tabs{Label: "Sections", IndicatorStyle: core.Style{Height: core.Pt(2), Background: core.RGB(0x007aff)}, Children: []core.Node{
	base.Tab{Label: "Overview", Selected: func() bool { return section.Get() == "overview" },
		OnPress: func(core.PressEvent) { section.Set("overview") }, Children: []core.Node{core.Text{Content: "Overview"}}},
	base.Tab{Label: "Activity", Selected: func() bool { return section.Get() == "activity" },
		OnPress: func(core.PressEvent) { section.Set("activity") }, Children: []core.Node{core.Text{Content: "Activity"}}},
}}
```

## Dialog

`base.Dialog`는 백드롭, 레이어, 팝업 스타일과 `Animation` 또는 `TransitionShader`, 그리고 자식 요소를 받습니다. `Open`, `DefaultOpen`, `OnOpenChange`로 열림 상태를 제어하고, `OnOpen`, `OnClose`, `OnCancel`로 일어난 일을 전달받습니다. `OnConfirm`은 기본 동작입니다. 키보드 포커스가 자체적으로 Return을 처리하는 버튼이나 필드에 있지 않고 다른 모달이 다이얼로그를 덮고 있지도 않을 때 Return을 누르면 `OnConfirm`이 실행되며, 다이얼로그는 `ReasonSelect`로 닫힙니다. `Role`과 `AccessibilityDescription`은 스크린 리더가 안내할 내용이며 확인창에는 `RoleAlertDialog`를 사용합니다. `NoCloseOnSwipe`는 iOS에서 스와이프로 닫히는 것을 막고, `Presentation`은 iOS 프레젠테이션 방식을 선택하며, `StatusBar`는 다이얼로그가 열린 동안 상태 표시줄을 변경합니다.

`base.Sheet`는 백드롭과 콘텐츠가 있는 어느 가장자리에나 붙는 패널입니다. `CloseOnSwipe`를 켜면 하단 시트가 아래로 드래그하는 동작을 따라가다 임계값을 넘으면 닫힙니다. `base.Dialog`와 마찬가지로 `OnConfirm`, `Role`, `AccessibilityDescription`을 받습니다. `base.ShowAlertDialog`는 역할별 버튼이 포함된 시스템 알림창이나 액션 시트를 띄웁니다:

```go
base.ShowAlertDialog(cx, base.AlertDialog{
	Title: "Delete the note?",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(label string) {
	if label == "Delete" {
		remove()
	}
})
```

## Menu

`base.Menu`는 직접 지정한 트리거에서 `base.MenuItem`들을 열고, `base.ContextMenu`는 포인터 위치에서 엽니다. 항목은 `OnSelect`를 보고하며 `NoCloseOnSelect`가 설정되지 않은 한 메뉴를 닫습니다. `Checked`는 체크 가능한 항목으로 만들고, `Indicator`는 콘텐츠 앞에 마크를 그리며, `Role`은 보조 기술이 이를 메뉴 항목, 체크박스 항목, 라디오 항목 중 무엇으로 인식할지 선택합니다. `base.Submenu`는 하위 항목들을 담습니다.

```go
base.Menu{Trigger: core.Text{Content: "Actions"}, Children: []core.Node{
	base.MenuItem{Label: "Show sidebar", Checked: sidebar.Get, OnSelect: func() { sidebar.Set(!sidebar.Get()) }},
	base.Submenu{Label: "Share", Children: []core.Node{
		base.MenuItem{Label: "Copy link", OnSelect: copyLink},
		base.MenuItem{Label: "Email", OnSelect: emailLink},
	}},
}}
```

데스크톱에서는 `Placement`, `Align`, `Offset`, `Margin`으로 팝업의 위치를 잡고, `PopupStyle`과 `BackdropStyle`로 스타일을 지정합니다.

## EmojiPicker

`base.EmojiPicker`는 앱의 언어(또는 명시적 `Languages` / `LanguagesFunc`)로 현지화된 CLDR 이름, 키워드, 쇼트코드를 기준으로 모든 유니코드 이모지를 검색하며, 카테고리 탭, 선택적 `Recent` 항목, 피부색 선택, 키보드 내비게이션, 가상화된 그리드를 제공합니다. 모든 기본 레이블과 카테고리 이름은 `base/locales`에서 가져오며, 이모지 데이터셋은 `EmojiPicker`나 그 헬퍼(`base.SearchEmojis`, `base.EmojisInCategory`, `base.EmojiByString`)를 사용하는 앱에만 링크됩니다.

```go
base.EmojiPicker{
	Recent:   recent.Get,
	OnSelect: func(e base.Emoji) { insert(e.Emoji) },
	Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
}
```

`Category`와 `DefaultCategory`는 탐색 그룹을 선택합니다. 카테고리 탭과 피부색 선택지는 각각 하나의 Tab 정지점이며 화살표 키로 이동합니다. `EmojiPickerRef`는 `Focus`, `Query`, `SetQuery`, `SetCategory`, `MatchedCount`를 제공하며, 스타일이 적용된 피커도 동일한 검색어 상태를 사용합니다. `LabelFunc`, `SearchLabelFunc`, `PlaceholderFunc`는 마운트된 상태에서 문구를 업데이트합니다. `Emoji.WithSkinTone`은 짝을 이루는 두 사람 모두에게 선택한 피부색을 적용하고 기본 피부색은 중립 형태로 되돌리며, 이모지 자체로 검색할 때는 정확한 시퀀스를 유지합니다.

Lucide 카테고리 아이콘, 지우기 버튼이 있는 검색 필드, 강조된 이모지 미리보기, 피부색 견본이 포함된 테마 적용 피커가 필요하면 `ui.EmojiPicker`를 사용하세요.

---
title: 폼
description: 텍스트 필드, 포커스, 소프트웨어 키보드, 스위치, 피커 및 유효성 검사.
group: 핵심 개념
order: 57
---

## 텍스트 필드

`ui.Input`은 `base.Input`의 필드를 가지며 `ui`의 외관을 따르는 플랫폼 네이티브 텍스트 필드입니다. `Value`를 지정하면 제어(controlled) 모드가 되어 앱이 값을 보관하고 `OnChange`에서 갱신하며, `DefaultValue`를 지정하면 자체적으로 값을 유지합니다.

```go
name, email := core.NewSignal(cx, ""), &core.Ref{}

ui.Input{
	Value:         name.Get,
	OnChange:      name.Set,
	Placeholder:   "Your name",
	ReturnKeyType: core.ReturnKeyNext,
	OnSubmit:      func(string) { email.Focus() },
}
```

`Masked`는 문자를 가리고, `Readonly`는 텍스트 선택과 복사는 허용하되 편집만 막습니다(미니 프로그램에서는 위챗 한 줄 필드에서만 동작하며, 위챗 여러 줄 필드와 알리페이 필드는 해당 호스트가 편집을 되돌릴 수 없어 `Disabled`처럼 비활성화됩니다). `Disabled`는 필드를 비활성화하며, `Multiline`은 텍스트를 줄바꿈하고 스타일로 크기를 고정하지 않는 한 내용에 맞춰 늘어납니다. 여러 줄 필드와 그 플레이스홀더의 줄 높이는 스타일의 `LineHeight`와 같고 폰트는 `core.Text`처럼 각 줄의 세로 중앙에 배치되며, 한 줄 필드는 줄을 전체 높이의 세로 중앙에 유지합니다. `KeyboardType`은 소프트웨어 키보드 종류(`core.KeyboardEmail`, `core.KeyboardNumber`, `core.KeyboardURL` 등)를 선택하고, `AutoComplete`는 시스템에 자동 완성을 위한 필드 용도를 알려 주며, `MaxLength`는 사용자가 입력할 수 있는 UTF-16 단위 길이를 제한합니다. `AutoCapitalize`와 `NoAutoCorrect`는 키보드 동작을 세부 조정합니다.

`OnSelectionChange`와 `OnCaretMove`는 캐럿 위치에서 메뉴를 열 수 있도록 선택 범위와 캐럿 좌표를 보고합니다. `core.Suggest` 타입의 `Suggest`는 `@` 같은 트리거 뒤의 검색어를 추적하며, 필드 옆의 `ui.SuggestList`는 캐럿 위치에 제안 목록 행을 띄워 위/아래 화살표, Return, 누르기로 선택할 수 있게 합니다(`chat.Composer`의 `Suggest`가 멘션에서 동작하는 방식과 같습니다). `OnKeyPress`는 각 키 입력을, `OnPaste`는 사용자가 붙여넣은 내용을, `OnCompositionChange`는 입력기(IME) 조합 상태를 추적합니다.

iOS와 Android에서는 `cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, handler)`를 사용하면 필드가 제출하거나 포커스를 잃거나 줄바꿈을 삽입하기 전에 소프트웨어 Return 키를 가로챕니다. `KeyAuto`와 `KeyNotEditing`은 `SubmitBehavior`에 맡기며, iOS에서 조합 중인 텍스트를 확정하는 Return 키는 입력기에 남습니다.

## 포커스와 키보드

`Ref.Focus()`는 필드에 포커스를 주거나, 데스크톱·웹·Apple TV에서는 활성화된 누를 수 있는 뷰 또는 `Focusable` 뷰에 포커스를 줍니다. `Ref.Blur()`는 해당 뷰 자신의 포커스만 해제하며, `cx.DismissKeyboard()`는 소프트웨어 키보드를 닫습니다. `OnFocus`와 `OnBlur`는 포커스 변화를 추적하며, `View.OnFocus`/`OnBlur`도 포커스 가능한 모든 뷰에서 동일하게 동작합니다. 포커스된 뷰가 `Show`가 닫히거나 행이 사라지면서 트리에서 제거되면, 이를 제거한 커밋 직후에(또는 플랫폼이 다음 포커스를 먼저 보고하는 경우 그 뷰의 `OnFocus` 직전에) `OnBlur`를 한 번 받고, 남아 있는 주변 뷰들은 `OnFocusOut`을 받습니다.

요소가 어떤 위치에도 표시되지 않는 `Holder`의 콘텐츠 안에 있는 동안에는 `Ref`의 포커스, 블러, 스크롤, 줌 요청이 아무 동작도 하지 않으며, 요소가 트리를 떠날 때 대기 중이던 요청은 버려집니다. 콘텐츠를 다시 표시해도 이전 요청이 재생되지 않으므로 배치된 후 다시 요청하세요.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

소프트웨어 키보드는 휴대폰 화면 하단을 덮습니다. 하단 패딩이나 마진에 `core.EnvKeyboard`를 지정하면 콘텐츠가 키보드 위에 유지되며 키보드와 함께 애니메이션으로 이동합니다.

```go
core.View{Style: core.Style{
	Padding:    core.All(16),
	PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard},
	Gap:        12,
}, Children: form}
```

폼을 감싸는 스크롤 뷰는 드래그할 때 키보드를 닫거나 키보드가 떠 있는 동안 탭이 어떻게 동작할지 지정할 수 있습니다.

```go
core.Scroll{Behavior: core.ScrollBehavior{
	KeyboardDismissMode:       core.KeyboardDismissOnDrag,
	KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
}, Children: []core.Node{form}}
```

코드에서는 `cx.Env().KeyboardHeight()`와 `cx.Env().KeyboardDuration()`으로 키보드 크기와 타이밍을 읽고, `cx.Env().SafeArea()`로 화면 주변 안전 영역 인셋을 읽을 수 있습니다.

## 선택 컨트롤

`ui.Switch`는 즉시 적용되는 설정이고, `ui.Checkbox`는 선택·미선택·불확정 상태를 가지며, `ui.Radio`는 `ui.RadioGroup` 안의 항목 하나를 나타냅니다. `ui.Select[T]`는 트리거 아래에 옵션 목록을 열거나 `Presentation` 지정에 따라 시트로 열며, `ui.Tabs[T]`는 여러 선택지를 한꺼번에 보여 줍니다.

```go
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}

size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options:  []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:    size.Get,
	OnChange: size.Set,
	Label:    "Size",
}
```

`ui.NumberInput`과 `base.NumberInput`은 변경되는 접근성 이름을 위해 `LabelFunc`를 지원합니다. 커스텀 `Format`은 필드가 편집 중이 아닐 때 읽는 시그널을 추적하며, 언어가 전환되어도 편집이 끝날 때까지 현재 입력 중인 초안을 유지합니다. `ui.NumberInput`은 범위 내에서 숫자를 단계별로 증감하거나 스크러빙하며, `ui.DatePicker`와 `ui.TimeField`는 날짜와 시간을, `ui.ColorPicker`는 색상을, `ui.TagInput`은 태그 목록을 편집합니다.

`ui.OtpInput`은 일회용 코드를 셀 단위로 입력받습니다. `ui.OtpInput`과 `base.OtpInput`은 포커스된 필드나 입력된 코드를 교체하지 않고 `LabelFunc`를 지원합니다. `Size`는 기본 높이와 너비를 설정하며 기본값은 데스크톱과 TV에서 36포인트, 터치 기기에서 44포인트입니다. `CellWidth`와 `CellHeight`로 가로·세로 크기를 독립적으로 재정의할 수 있습니다. `TextStyle`은 셀 타이포그래피를, `CornerRadius`는 모서리 둥글기를, `CaretHeight`는 줄 높이에서 파생된 캐럿 높이를 재정의합니다.

```go
quantity := core.NewSignal(cx, 1.0)
ui.NumberInput{Value: quantity.Get, OnChange: quantity.Set, Min: 1, Max: 99, Step: 1, Label: "Quantity"}
```

## 그룹화된 행

`ui.FormSection`은 위에 제목, 아래에 바닥글을 두고 행들을 카드 위에 배치합니다. 카드의 배경, 테두리, 모서리에는 `CardStyle`을 설정하세요. `CardStyleFunc`는 그 위에 병합되며 읽는 시그널을 추적합니다. `DividerInset`은 `core.Each`로 만든 행을 포함해 각 자식 행을 받습니다. 인셋 값과 `true`를 반환하면 구분선 인셋을 재정의하고, `false`를 반환하면 자동 제목 정렬(다른 노드는 12포인트)을 유지합니다. 인셋 0은 전체 너비 구분선을 만듭니다.

`ui.FormRow`는 제목 앞에 `Leading` 또는 아이콘 타일을 배치합니다. `LeadingWidth`를 설정하면 노드를 위한 공간을 확보하고 그 아래 구분선을 제목에 맞춥니다. 슬롯의 기본 너비는 24포인트입니다. `TileSize`는 아이콘 타일, 글리프, 모서리를 함께 스케일링합니다. `TitleStyle`과 `DetailStyle`은 제목과 세부 정보 타이포그래피를 재정의합니다. `TitleMaxLines`는 제목 줄 수를 제한하고(0은 무제한), `MinHeight`는 행의 최소 높이를 포인트 단위로 재정의하며, `Below`는 제목과 세부 정보의 텍스트 열 아래에 노드를 배치합니다. `MinHeight`가 0이면 기본값인 40포인트(터치 기기에서는 48포인트)를 유지합니다.

단일 선택 그룹을 만들려면 섹션을 `base.RadioGroup`으로 감싸고 각 행에 `Role: core.RoleRadio`, `Checked`, `OnPress`를 설정하세요. 행은 `base.Radio`를 사용해 checked 및 selected 접근성 상태, 단일 Tab 정지점, 화살표 키 선택을 제공하며 이미 선택된 행을 다시 눌러도 아무 일도 일어나지 않습니다. 선택된 값은 앱이 관리합니다.

`ui.FormSection.TitleFunc`와 `FooterFunc`, 그리고 `ui.FormRow.TitleFunc`, `DetailFunc`, `LabelFunc`는 행을 다시 빌드하지 않고 문구를 업데이트합니다. 세부 정보, 섹션 제목 또는 바닥글이 빈 문자열이면 해당 선택적 텍스트가 제거되며, 행 레이블이 비어 있으면 현재 제목을 사용합니다. 검색 가능한 `ui.Select`는 열려 있는 검색어와 선택 상태를 유지한 채 `SearchPlaceholderFunc`와 `EmptyFunc`를 바인딩합니다.

## 유효성 검사

규칙은 앱이 관리합니다. 시그널이 오류 메시지를 보관하고, `OnBlur`나 `OnSubmit`에서 오류를 설정하며, 오류가 있을 때 `core.Show`로 필드 아래에 메시지를 표시합니다.

```go
address := core.NewSignal(cx, "")
err := core.NewSignal(cx, "")
colors := ui.ColorsOf(cx)
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{
		Value:    address.Get,
		OnChange: func(v string) { address.Set(v); err.Set("") },
		Label:    "Email",
		OnSubmit: func(string) {
			if !strings.Contains(address.Get(), "@") {
				err.Set("Enter an email address.")
			}
		},
	},
	core.Show{
		When: func() bool { return err.Get() != "" },
		Then: func(*core.Scope) core.Node { return ui.Textf(err.Get, 12, ui.Regular, colors.Danger) },
	},
}}
```

`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)`는 나타나는 메시지에 애니메이션을 적용하고, `ReturnKeyNext`와 함께 `OnSubmit`을 사용하면 키보드를 다음 필드로 이동시킵니다.

## 다음 단계

- [내비게이션](navigation.md): 폼이 배치되는 곳.
- [텍스트](text.md): 필드의 텍스트 스타일링.
- [접근성](accessibility.md): 스크린 리더가 필요로 하는 레이블과 값.

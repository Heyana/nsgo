---
title: 접근성
description: 스크린 리더와 키보드를 위한 레이블, 역할, 값, 동작 및 포커스.
group: 핵심 개념
order: 61
---

## 스크린 리더가 읽는 내용

뷰의 `Label`은 보조 기술이 읽어 주는 이름이고, `AccessibilityDescription`은 레이블 뒤에 이어서 읽는 추가 설명이며, `Hint`는 활성화했을 때의 동작 설명, `Role`은 해당 요소의 역할입니다. 역할에는 `core.RoleButton`, `RoleLink`, `RoleHeader`, `RoleImage`, `RoleCheckbox`, `RoleSwitch`, `RoleRadio`, `RoleTab`, `RoleTextField`, `RoleSearchField`, `RoleStatus`, `RoleMenu`, `RoleMenuItem`, `RoleDialog`, `RoleAlertDialog`, `RoleAlert`, `RoleAdjustable`, `RoleProgressBar`, `RoleTable` 등이 있습니다. 상태(status)는 기본적으로 공손한 라이브 리전(polite live region)으로 동작하며, 검색 필드는 검색 상자로 안내되는 텍스트 필드입니다. 다이얼로그는 열릴 때 `ui.Dialog`의 제목 및 설명처럼 역할과 레이블을 먼저 읽고 그 뒤에 설명을 읽습니다. 레이블이 없는 텍스트는 그 콘텐츠 자체가 읽힙니다. `LabelFunc`, `HintFunc`, `AccessibilityDescriptionFunc`는 동일한 뷰에 동적으로 바뀌는 접근성 텍스트를 바인딩하므로, 언어가 전환되어도 컨트롤의 포커스와 상태가 유지됩니다. 레이블을 갖는 모든 코어 요소는 `LabelFunc`를 지원하며, `View`와 `Text`는 `HintFunc`를 지원합니다.

`ui.Button`, `ui.Checkbox`, `ui.MenuItem`, `ui.AlertAction`에서 변경되는 텍스트에는 `TextFunc`를 사용하세요. 이들의 접근성 이름은 해당 텍스트를 따릅니다. 버튼과 체크박스는 별도의 `LabelFunc`도 지원합니다. 슬라이더, 탭, 스위치, 프로그레스 바, 팝업 트리거는 변경되는 접근성 이름을 위해 `LabelFunc`를 받습니다. `ui.Option.LabelFunc`는 탭, 셀렉트, 콤보박스에서 옵션의 텍스트를 제공합니다. 숫자 및 코드 입력, 프로그레스 링, 두 가지 스피너 변형 역시 `LabelFunc`를 지원합니다.

```go
core.View{
	Role:  core.RoleButton,
	Label: "Add to cart",
	Hint:  "Adds the item and opens the cart",
	OnPress: add,
	Children: []core.Node{core.Icon{SVG: lucide.ShoppingCart}},
}
```

`AccessibilityValue`는 범위나 텍스트 형태의 값을 제공합니다. 예를 들어 슬라이더의 최솟값, 최댓값, 현재 숫자 또는 `"3 items"`와 같은 텍스트입니다. `Accessible`은 하나의 뷰를 단일 요소로 묶어 카드의 이름과 가격이 두 번에 나뉘어 읽히지 않고 한 번에 읽히도록 하며, 레이블이 없으면 뷰의 하위 텍스트들을 읽어 줍니다. `AccessibilityElementsHidden`은 갈매기표(chevron)나 구분선 같은 장식 요소를 숨기고, `ImportantForAccessibility`는 하위 트리 전체를 숨깁니다.

```go
core.View{Accessible: true, Label: "Apples, $1.20 a kilo", Children: []core.Node{
	core.Text{Content: "Apples"},
	core.Text{Content: "$1.20 a kilo"},
}}
```

## 보이는 레이블

`core.Text` 또는 `core.View`의 `LabelFor`에 컨트롤의 `Ref`를 지정합니다. 이 레이블을 누르면 해당 컨트롤에 포커스가 이동하고 체크박스 토글과 같은 누르기 동작이 실행됩니다. 현재 화면에 표시된 텍스트가 `Label` 문자열을 중복 지정하지 않고도 컨트롤의 이름이 되며, 접근성과 스타일 측면에서 컨트롤의 `Disabled` 및 `Invalid` 상태를 따릅니다. 레이블 자체에는 버튼 역할이나 Tab 정지점이 추가되지 않습니다.

```go
field := new(core.Ref)
return core.View{Children: []core.Node{
	core.Text{Content: "Email", LabelFor: field},
	base.Input{Ref: field},
}}
```

`LabelledBy: []*core.Ref{label, requirement}`는 여러 표시 요소를 지정된 순서대로 연결해 컨트롤의 이름으로 삼으며 `Label`보다 우선합니다. 다만 누르기 동작은 전달하지 않습니다. 그 외의 경우에는 `Label`이 트리의 순서를 따르는 암묵적 `LabelFor` 이름보다 우선합니다. 텍스트 변경, 이동, 숨김, 제거, 재마운트가 발생하면 관계도 함께 갱신됩니다. 누락되었거나 중복되었거나 순환하거나 숨겨진 참조는 무시되며, 연결은 하나의 서피스 및 격리 범위(confinement scope) 안에서만 유효합니다. 레이블은 모달 뒤에 있는 컨트롤을 활성화할 수 없습니다.

`base` 및 `ui`의 버튼, 체크박스, 라디오, 스위치, 셀렉트, 숫자 입력은 `Ref`와 `LabelledBy`를 내부 컨트롤로 전달하며, 슬라이더는 `ThumbRef`와 `StartThumbRef`를 노출합니다. TSX에서는 `ref={node => field.set(node)}`로 채운 노드 시그널과 함께 `labelFor={field()}` 및 `labelledBy={[label()]}`를 사용하여 필드보다 앞에 나온 레이블도 필드가 마운트될 때 갱신되도록 합니다.

## 동작

`AccessibilityActions`는 스크린 리더가 요소에서 제공하는 동작 목록을 정의합니다. 여기에는 표준 동작(`core.ActionActivate`, `ActionIncrement`, `ActionDecrement`, `ActionEscape`, `ActionMagicTap`)과 앱 고유의 동작이 포함되며, 각각 `Name`과 `Label`을 갖습니다. `OnAccessibilityAction`은 해당 `Name`을 전달받습니다. 요소가 마운트된 상태에서 사용 가능한 동작을 바꾸거나 레이블을 번역하려면 `core.View` 또는 `core.Input`에서 `AccessibilityActionsFunc`를 사용하세요.

```go
core.View{
	Role: core.RoleAdjustable,
	Label: "Quantity",
	AccessibilityValue: func() core.AccessibilityValue {
		return core.AccessibilityValue{Max: 10, Now: float32(n.Get()), Text: fmt.Sprintf("%d items", n.Get())}
	},
	AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}},
	OnAccessibilityAction: func(name string) {
		if name == core.ActionIncrement {
			n.Update(func(v int) int { return v + 1 })
		} else {
			n.Update(func(v int) int { return v - 1 })
		}
	},
	Children: quantityControls,
}
```

`MultiSelectable`은 둘 이상의 항목 선택을 허용하는 그리드, 리스트박스 또는 트리를 표시합니다. `base.DataTable`은 다중 행 선택 시 이를 설정하며, 웹, Windows, Linux는 이에 대응하는 접근성 플래그를 노출합니다.

`Expanded`는 펼침(disclosure) 요소나 메뉴 버튼이 열려 있는지를 나타내고, `Busy`는 콘텐츠가 로딩 중임을, `Invalid`는 앱이 필드의 내용이나 컨트롤의 값을 거부했음을(`aria-invalid:`로 스타일링) 나타내며, `Checked`, `Selected`, `Pressed`는 컨트롤의 상태를 보고합니다. 컨트롤의 각 부분은 `PartStates`를 통해 상태를 전달받을 수 있으므로, 인디케이터가 별도의 포커스 정지점이 되지 않고도 체크박스의 상태를 따를 수 있습니다.

`SelectionItem`은 현재 `Selected` 값과 무관하게 뷰가 선택 가능한 컬렉션의 항목임을 표시합니다. `base.DataTable`은 테이블이 행 또는 셀을 선택할 때 해당 행이나 셀에 이를 설정하고, `base.Tree`는 모든 행에 설정합니다. Windows는 SelectionItem 패턴을 유지하고, 웹은 `aria-selected="true"` 또는 `"false"`를 기록합니다. 선택 기능이 없는 테이블은 행에 두 표식 모두 남기지 않습니다.

## 실시간 변경

`LiveRegion`은 뷰의 레이블과 내부 텍스트가 바뀔 때 스크린 리더가 이를 안내하도록 합니다. `core.LiveRegionPolite`는 사용자가 유휴 상태가 될 때까지 기다리고, `LiveRegionAssertive`는 즉시 끼어들어 안내합니다. `cx.Announce`는 메시지 하나를 한 번 안내합니다.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("%d items in the basket", count.Get()) },
	LiveRegion: core.LiveRegionPolite}
```

## 포커스와 키보드

툴팁은 트리거가 포커스를 받기 전에 해당 트리거를 설명합니다. `View.Tooltip`, `base.Tooltip`, `ui.Tooltip`은 컨트롤의 이름을 반복하거나 앱이 `AccessibilityDescription`을 직접 제공하거나 툴팁이 `Disabled`인 경우를 제외하고, 현재 텍스트를 컨트롤의 접근성 설명으로 사용합니다. 래퍼는 이 설명을 내부 컨트롤에 전달합니다. 말풍선과 그 자식 요소는 스크린 리더에서 숨겨지며 알림(alert)을 발생시키지 않습니다. 따라서 `ui.IconButton`의 기본 툴팁은 레이블을 중복해서 읽지 않습니다.

`base.DataTable`은 이름이 지정된 그리드를 키보드 및 접근성 포커스 정지점으로 노출하며, 포인터 히트 레이어는 보조 기술로부터 숨깁니다.

데스크톱, 웹, Apple TV에서는 누르기 동작을 받는 뷰가 텍스트 필드와 함께 Tab 정지점이 됩니다. `NoFocus`는 정지점에서 제외하고, `Focusable`은 누르기 동작이 없는 뷰도 정지점으로 만듭니다. `Style.FocusVisible`은 키보드로 포커스가 이동했을 때만 포커스 링을 그리고(`focus-visible:`), `Style.Focus`는 모든 포커스를, `Style.FocusWithin`은 뷰 내부의 포커스를 스타일링합니다. `OnFocus`와 `OnBlur`는 포커스 이벤트를 수신하며, 포커스된 뷰가 트리에서 제거될 때도 `OnBlur`가 호출됩니다. `Ref.Focus()`와 `Ref.SetAccessibilityFocus()`는 포커스를 이동시킵니다. `core.FocusedRef(cx)`는 현재 포커스를 가진 뷰의 `Ref`를 반환하므로, 단축키로 여는 찾기 표시줄처럼 트리거 없이 열리는 요소가 닫힐 때 이전 포커스를 되돌려 줄 수 있습니다. Return과 Space는 포커스된 뷰를 누르고, 화살표 키는 탭 리스트, 라디오 그룹, 메뉴 사이를 이동합니다. 뷰의 `Tooltip`은 키보드로 포커스가 이동하면 즉시 표시되며, 포커스가 떠나거나 Escape를 누를 때까지 유지됩니다. 팝업의 `OnOpen`은 콘텐츠가 준비된 후 실행되므로 그 안에서 `Focus`를 호출해 비모달 다이얼로그처럼 포커스를 안으로 옮길 수 있습니다. 팝오버의 콘텐츠는 Tab 순서에서 트리거 바로 뒤를 따르며, 팝업이 닫히면 포커스가 트리거로 돌아갑니다.

Android, iPhone, iPad에서는 `Ref.Focus()`와 버튼의 `AutoFocus`가 하드웨어 키보드를 위해 활성화된 pressable 뷰나 `Focusable` 뷰에 포커스를 줄 수 있습니다. 컨트롤을 터치해도 이 포커스는 이동하지 않습니다. Space와 Return은 키맵 이후에 포커스된 pressable을 활성화하며, 모든 press 단계에서 Control, Alt, Shift 및 플랫폼 키 상태를 유지합니다. `NoFocus`가 지정되었거나 비활성화되었거나 숨겨졌거나 모달 뒤에 있는 컨트롤은 이 포커스를 받을 수 없습니다. 뷰에 포커스를 주어도 소프트웨어 키보드는 열리지 않으며, 네이티브 입력 필드는 기존 편집 동작을 유지합니다. watchOS와 미니 프로그램에는 물리 키보드 활성화 경로가 없습니다.

`cx.Env().ScreenReader()`는 스크린 리더 실행 여부를, `cx.Env().ReduceMotion()`은 동작 줄이기 설정 여부를, `cx.Env().IncreaseContrast()`는 고대비 요청 여부를 반환합니다(macOS, iOS, watchOS의 대비 증가, Windows 고대비 테마, GNOME 고대비, GTK 4.20 대비 설정 또는 고대비 GTK 테마, Android 14의 표준 초과 대비 설정, 웹의 `prefers-contrast: more` 또는 `forced-colors: active`). 또한 `cx.Env().FontScale()`은 텍스트 크기 배율을, `cx.Env().RTL()`은 레이아웃 방향을 반환합니다. 텍스트는 `NoFontScaling`이 설정되지 않는 한 텍스트 크기를 따르며, 오른쪽에서 왼쪽(RTL) 레이아웃은 논리적 가장자리를 사용할 때 물리적 가장자리를 좌우 반전합니다.

## 다음 단계

- [스타일과 테마](styles.md): 포커스 및 비활성화 상태.
- [폼](forms.md): 필드의 레이블과 값.
- [캔버스와 셰이더](canvas.md): 드로잉의 각 부분 노출하기.

---
title: 셀렉트
description: 팝업이나 시트로 목록에서 값 하나를 선택하는 컨트롤.
group: 컴포넌트
order: 36
---

`ui.Select[T]`는 비교 가능한 모든 타입의 값 목록에서 하나를 선택합니다. 목록은 모든 기기에서 트리거 아래에 열리며, 휴대폰 앱에서 자주 쓰는 방식처럼 `Presentation: base.PresentSheet`를 지정하면 대신 아래에서 올라오는 시트로 열립니다.

```go preview=select/basic height=280
size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options: []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:   size.Get,
	OnChange: size.Set,
	Label:   "Size",
}
```

## 옵션과 검색

`Options`는 `ui.Option[T]` 슬라이스로, 각 항목은 `Value`, `Label`, 그리고 선택적인 `Group`, `Icon`, `Disabled` 함수 또는 앱 고유의 `Content`를 가집니다. `Select`나 `Combobox`에서 비어 있지 않은 동일한 `Group`에 속해 인접한 옵션들은 해당 그룹 머리말 아래에 표시되며, 화살표 키와 타자 입력은 이 머리말을 건너뛰고 스크린 리더는 제목(heading)으로 읽어 줍니다. 옵션이 시그널을 따를 때는 `OptionsFunc`를 사용하세요. 그러면 열려 있는 목록이 필드의 값과 포커스를 유지한 채 제자리에서 업데이트됩니다. 선택된 옵션이 없을 때는 `Placeholder`가 표시됩니다. `Searchable`은 타이핑으로 목록을 필터링하고, `OnClear`는 지우기 버튼을 표시하며, `Disabled`는 컨트롤을 비활성화합니다.

```go preview=select/searchable height=400
ui.Select[string]{
	Options:  languages,
	Value:    language.Get,
	OnChange: language.Set,
	Searchable: true,
	SearchPlaceholder: "Search languages",
	OnClear:  func() { language.Set("") },
	Placeholder: "Choose a language",
}
```

`Style`은 셀렉트의 레이아웃을 지정하며 너비를 따로 설정하지 않으면 기본 너비는 200포인트입니다. [Input](input.md)과 마찬가지로 높이, 패딩 및 타이포그래피가 필드에 전달되므로 `SizeSmall` 버튼과 나란히 정렬할 수 있습니다:

```go
ui.Select[string]{Options: sizes, Value: size.Get, OnChange: size.Set, Label: "Size",
	Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
```

## 동작

포커스된 트리거에서 위 화살표, 아래 화살표, Return, Space 키를 누르면 목록이 열립니다. 화살표 키는 강조 표시를 이동시키고, 타자 입력 검색(typeahead)은 항목 제목에 맞춰 해당 위치로 건너뜁니다. 항목을 선택하면 목록이 닫히고 해당 값이 보고됩니다. Escape 키, 뒤로 가기 또는 바깥쪽을 누르면 변경 없이 닫힙니다. `Empty`는 검색 결과가 없을 때 표시되는 텍스트입니다.

## 접근성

`Label`은 보조 기술을 위해 필드의 이름을 지정합니다. 시그널이 바뀔 때 제자리에서 이름을 업데이트하려면 `LabelFunc`를 사용하세요. 레이블이 비어 있으면 `Placeholder`를 사용합니다. 트리거와 목록은 셀렉트의 역할을 가지며, 선택된 항목이 보고됩니다. 키보드로는 Tab 키로 트리거에 이동한 뒤 Return 또는 Space 키로 목록을 열 수 있습니다.

## 플랫폼

macOS, Windows, Linux에서는 목록이 트리거 옆에 팝업으로 열립니다. iOS와 Android에서는 시트로 열리며, 선택된 행이 필드에 표시됩니다. 모든 대상에서 동일한 옵션과 값이 동작합니다.

## 관련 문서

- [폼](../forms.md): 폼에서의 셀렉트.
- [Menu](menu.md): 값이 아니라 명령을 위한 메뉴.
- [Tabs](tabs.md): 몇 개의 옵션을 한꺼번에 표시.
- 갤러리의 Select 및 Combobox 페이지.
- [스타일이 없는 컴포넌트](../authoring/base.md#select): 그룹과 직접 만든 트리거를 지원하는 `base.Select`.

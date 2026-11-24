---
title: 탭
description: 탭 목록, 세그먼트 컨트롤 및 시스템 탭 바.
group: 컴포넌트
order: 40
---

`ui.Tabs[T]`는 세그먼트 컨트롤이나 탭 목록 형태로 몇 가지 값 중 하나를 선택하며, 앱은 선택된 값에 해당하는 콘텐츠를 그 옆에 표시합니다. 탭 목록은 하나의 Tab 정지 지점(stop)이며 방향키로 탭 사이를 이동합니다.

```go preview=tabs/basic
section := core.NewSignal(cx, "overview")
ui.Tabs[string]{
	Options: []ui.Option[string]{
		{Value: "overview", Label: "Overview"},
		{Value: "activity", Label: "Activity"},
		{Value: "settings", Label: "Settings"},
	},
	Value:    section.Get,
	OnChange: section.Set,
	Label:    "Sections",
}
```

## 변형과 크기

`Variant`는 `TabsSegmented`(기본값으로, 들어간 트랙 위에 올라온 썸 형태) 또는 `TabsUnderline`(선택된 탭 아래의 강조선)입니다. 밑줄 변형에서는 `IndicatorColor`를 설정하고 `IndicatorFit: TabsIndicatorContent`를 사용하여 전체 탭이 아닌 레이블과 아이콘 너비를 따르게 할 수 있습니다. TSX에서는 각각 `indicatorColor`와 `indicatorFit="content"`입니다. `Size`는 `SizeDefault`, `SizeSmall` 또는 `SizeLarge`이며, `Equal`을 설정하면 레이블 너비에 맞추지 않고 모든 탭에 동일한 너비를 부여합니다.

```go preview=tabs/underline
ui.Tabs[string]{Variant: ui.TabsUnderline, Equal: true, Size: ui.SizeSmall, Options: tabs, Value: section.Get, OnChange: section.Set}
```

각 옵션에는 `Icon`, `Disabled` 함수, `Content` 노드 및 `TestID`를 지정할 수 있습니다. `Content`는 레이블을 대체하며 탭의 텍스트 스타일을 상속받습니다.

탭이 시그널을 따라야 할 때는 `OptionsFunc`를 사용하세요. NSGo는 값을 기준으로 항목을 제자리에서 업데이트하므로 해당 값이 계속 제공되는 한 선택된 탭과 키보드 포커스가 유지되며, 값이 사라지면 앱이 값을 변경할 때까지 선택되지 않은 상태로 남습니다.

## 탭 바

`ui.TabShell`은 모든 창 너비에서 최상위 섹션을 탭 바 옆에 배치합니다. 기본적으로 하단에 위치하며, `Edge`(또는 `EdgeFunc`)가 `mobile.TabBarLeading`, `mobile.TabBarTrailing` 또는 `mobile.TabBarAutomatic`(`cx.Env().VerticalBarEdge()`를 따름)일 때는 시작(leading) 또는 끝(trailing) 가장자리를 따라 세로로 배치됩니다. 모든 탭이 `SystemIcon`이나 SVG `Icon`을 제공하면 iOS의 모든 가장자리와 Android의 하단 가장자리에서 시스템 탭 바를 사용합니다. iOS의 SF Symbol이나 Android의 drawable을 가리키는 `SystemIcon`이 우선 적용되며, 그렇지 않으면 SVG가 표시됩니다. 그 외의 경우에는 제공된 SVG 아이콘으로 탭 바를 직접 그립니다. 자체 공간을 차지하는 탭 바는 해당 가장자리의 안전 영역(safe area)을 스스로 비켜 서므로, 그 옆에 있는 섹션(`base.Nav` 페이지 포함)의 해당 가장자리 `core.EnvSafeArea`는 0이 됩니다. iOS의 플로팅 하단 시스템 바 아래에서는 `ui.TabBarInset`으로 콘텐츠가 겹치지 않도록 여백을 유지하세요.

로케일이나 애플리케이션 상태를 따르는 탭 제목에는 `ShellTab.TextFunc`를, 탭 바의 접근성 이름에는 `TabShell.LabelFunc`를 설정하세요. 아이콘이 동일하게 유지되는 한 제목을 변경해도 선택된 섹션과 기존 탭이 유지됩니다.

`mobile.TabBar`는 iOS와 Android의 시스템 탭 바로, `Items`, `Selected`, `OnChange`와 선택적 `Edge`(`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing` 또는 `mobile.TabBarAutomatic`) / `EdgeFunc`를 받습니다. 각 `mobile.TabBarItem`은 `Title`과 `Icon`에 플랫폼 아이콘 이름을 받으며, `Icon`이 iOS의 SF Symbol이나 Android의 drawable을 지정하지 않은 곳에서는 `SVG: lucide.House()`가 24포인트 크기에 맞춰지고 선택 상태에 따라 틴트가 적용되어 표시됩니다. TSX에서는 `svg`가 SVG 문서를 받고 `edge`가 `"bottom"`, `"leading"`, `"trailing"` 또는 `"automatic"`을 받습니다. 다른 플랫폼에서는 `mobile.TabBar`가 공간을 차지하지 않으므로 대신 `ui.Tabs`를 표시하세요.

`ui.SidebarShell`은 사이드바, 푸터, 헤더, 콘텐츠 및 오른쪽 패널의 내용을 좌우 안전 영역에서 스스로 비켜 서게 하고 배경은 창 가장자리까지 이어지도록 하므로, 그 내부에서 해당 측면의 `core.EnvSafeArea`는 0이 되며 `ui.NavHeader`는 패딩에 좌우 안전 영역을 더합니다. `ui.NavPage`나 `TabShell` 섹션의 콘텐츠는 `PaddingEnv`를 사용해 스스로 안전 영역을 피하므로, 전체 너비(full-bleed) 이미지처럼 가장자리까지 채워야 하는 요소는 그대로 끝까지 확장할 수 있습니다.

## 접근성

목록은 `RoleTabList`를, 각 탭은 `RoleTab`을, 선택된 탭은 선택 상태를 보고합니다. 목록은 하나의 Tab 정지 지점이며, Left와 Up, Right와 Down, Home과 End 키로 활성화된 탭 사이를 이동하고, 도달한 탭은 누르기 이벤트를 보고합니다. `Label`은 목록의 이름을 지정합니다.

## 플랫폼

iOS와 Android에서는 각 플랫폼 고유의 외관과 제스처를 가진 시스템 탭 바가 표시되며, macOS, Windows, Linux 및 웹에서는 `ui.Tabs`가 표시됩니다. 앱의 콘텐츠는 모든 타깃에서 동일합니다.

## 관련 문서

- [내비게이션](../navigation.md): 앱의 페이지와 탭 바.
- [메뉴](menu.md): 섹션이 아닌 명령 목록.
- [셀렉트](select.md): 목록 안의 많은 옵션.
- 갤러리의 Tabs 및 NavStack 페이지.
- [스타일이 없는 컴포넌트](../authoring/base.md#tabs): 나만의 외관을 가진 탭을 만들기 위한 `base.Tabs`.

---
title: 내비게이션
description: 네이티브 내비게이션 스택, 시스템 헤더, 모달, 메뉴 및 탭 바.
group: 핵심 개념
order: 56
---

## 스택

`base.NewNav`는 단일 타입의 라우트 스택을 유지합니다. `Push`는 페이지를 추가하고, `Pop`은 맨 위 페이지를 제거하며, `PopToRoot`는 첫 페이지로 돌아가고, `Replace`는 맨 위 페이지를 교체합니다. `Stack`은 모든 라우트를 페이지로 렌더링하며, 맨 위 페이지가 표시되는 동안 아래쪽 페이지들은 상태를 유지한 채 마운트된 상태로 둡니다. `ui.NavPage`는 각 페이지에 `ui`의 배경과 제목, 뒤로 가기 버튼, 액션 버튼들이 포함된 `ui.NavHeader`를 제공합니다.

watchOS에는 시스템 내비게이션 바, 탭 바, 스위치가 없으므로, 그곳에서는 `ui.NavPage`가 뒤로 가기 버튼을 포함한 `ui.NavHeader`를 페이지 위에 직접 그리고, `ui.TabShell`이 자체 탭 바를 그리며, `ui.SwitchSystem`은 `ui` 자체 스위치가 됩니다.

```go
nav := base.NewNav(cx, "inbox")

return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, depth func() int) base.NavPage {
	if route == "inbox" {
		return ui.NavPage(cx, base.NavPage{Title: "Inbox", Children: []core.Node{
			ui.Button{Text: "Open a message", OnPress: func() { nav.Push("message") }},
		}}, ui.NavHeader{})
	}
	return ui.NavPage(cx, base.NavPage{Title: "Message"}, ui.NavHeader{
		OnBack: func() { nav.Pop(1) }, BackLabel: "Inbox",
		Trailing: []core.Node{ui.IconButton{Icon: lucide.Archive, Label: "Archive", Variant: ui.IconButtonGhost, OnPress: archive}},
	})
})
```

iOS와 Android에서는 플랫폼이 페이지의 제목과 헤더의 버튼들로 뒤로 가기 버튼과 화면 전환이 포함된 시스템 헤더를 그리며, macOS, Windows, Linux, 웹에서는 `ui`가 이를 그립니다.

Android는 Material 3 상단 앱 바(top app bar)를 사용합니다. 일반 바는 64 dp 높이에 22 sp 제목을 가집니다. `HeaderLargeTitle`은 28 sp 제목을 가진 152 dp 크기의 큰 바를 제공하며, 페이지 상단의 스크롤 뷰가 콘텐츠를 그 아래로 스크롤함에 따라 일반 바로 축소되고, 콘텐츠가 맨 위로 돌아오면 다시 확장됩니다.

iOS와 Android에서 네이티브 축소 및 확장 제목 폰트의 스타일을 지정하려면 `HeaderTitleStyle`과 `HeaderLargeTitleStyle`에 `FontSize`, `FontWeight`, `FontFamily`, `FontStyle`을 설정하세요. 설정하지 않은 필드는 각 플랫폼의 기본값을 유지하며, `HeaderTitleStyleFunc`와 `HeaderLargeTitleStyleFunc`는 변경 사항을 추적합니다. `HeaderStyle`은 계속해서 배경, 제목 색상, 버튼 틴트를 제공합니다. TSX에서는 `headerTitleStyle`과 `headerLargeTitleStyle` 스타일 객체 또는 배열을 사용합니다.

커스텀 `HeaderTitle`은 Android에서 축소된 제목만 대체하며, `Title`은 확장된 큰 제목으로 유지됩니다. 큰 제목 옆에 `HeaderRight`를 배치하려면 `Android: core.AndroidScreenProps{HeaderRightWithLargeTitle: true}`를 설정하세요. 해당 컨트롤들은 제목이 축소될 때 고정된 툴바로 이동하며 그 과정 내내 누를 수 있습니다. TSX에서는 `android={{ headerRightWithLargeTitle: true }}`를 사용합니다. 폰트에 더 많은 공간이 필요하면 큰 제목 영역이 늘어납니다. 독립 실행형 Android 빌드는 네이티브 RTL 레이아웃을 활성화하며, 임베드 호스트에서는 네이티브 헤더가 앱의 방향을 따르도록 application에 `android:supportsRtl="true"`를 설정해야 합니다.

페이지에 자체 `Header`를 지정하면 `ui.NavHeader`를 대체합니다. iOS와 Android에서는 페이지의 `HeaderLeft`, `HeaderRight`, 뒤로 가기 버튼 옆에서 `HeaderTitle` 대신 시스템 헤더의 타이틀 뷰가 되며, 검색 필드처럼 내부 뷰가 늘어나거나 백분율 너비를 사용할 때는 양쪽 끝 사이의 공간을 채우고 그렇지 않으면 콘텐츠 크기를 따릅니다. 그 외 플랫폼에서는 콘텐츠 위에 그려집니다. `ui.NavHeader.Scroll`은 여전히 iOS 또는 Android 페이지의 스크롤을 헤더의 블러 효과와 연결합니다.

`NoPopOnBack`은 뒤로 가기를 눌렀을 때 페이지를 유지하고, `OnBackPress`는 해당 입력을 수신하며, `NoPopOnSwipe`는 iOS의 스와이프 뒤로 가기를 끕니다. `cx.OnBackPress(func() bool)`은 저장되지 않은 변경 사항 등을 처리할 수 있도록 스택보다 먼저 실행되는 앱 핸들러를 등록합니다. `OnDismiss`는 사용자가 페이지를 닫았음을 알리며, 스택은 해당 페이지를 제거합니다.

## 몰입형 전체 화면

`mobile.Immersive(cx, true)`를 호출하면 창에 전체 화면을 요청하고, `mobile.Immersive(cx, false)`를 호출하면 일반 시스템 바 설정으로 돌아갑니다. 페이지나 다이얼로그에서는 해당 레이어가 표시되는 동안 전체 화면을 요청하기 위해 `StatusBar: core.StatusBarSetting{Immersive: core.ToggleOn}`을 사용하세요. TSX에서는 페이지나 다이얼로그에 `statusBar={{ immersive: true }}`를 사용하거나, 창 기본값으로 `nsgo.yaml`에 `status_bar: { immersive: true }`를 설정합니다.

Android에서는 가장자리를 스와이프하면 상태 표시줄과 내비게이션 바가 잠시 나타납니다. HarmonyOS에서는 창이 시스템에 상태 표시줄, 3버튼 내비게이션 바, 내비게이션 인디케이터를 숨기도록 요청합니다. iOS에서는 상태 표시줄이 숨겨지고 UIKit이 홈 인디케이터를 언제 숨기거나 표시할지 결정합니다. `cx.Env().SafeArea()`는 디스플레이 컷아웃을 포함해 시스템이 제공하는 인셋을 계속 보고합니다. 가로 모드의 3버튼 내비게이션을 사용하는 HarmonyOS에서는 시스템이 버튼을 측면에 그리면서도 버튼 영역을 아래쪽 가장자리에 보고하므로, 안전 영역이 아래쪽에 공간을 남기고 버튼은 측면을 가릴 수 있습니다.

## 모달

`ui.Dialog`는 어두워진 앱 위에 표면을 표시하고 터치 스크린에서는 아래에서 올라오는 시트로 표시하며, `ui.Sheet`는 아래에서 올라오는 패널을 표시합니다. 별도로 막지 않는 한 둘 다 Escape 키를 누르거나 배경(backdrop)을 누르면 닫힙니다.

```go
ui.Dialog{
	Open:         dialog.Get,
	OnOpenChange: func(open bool, _ base.Reason) { dialog.Set(open) },
	Title:        "Rename",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { dialog.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

`ui.AlertDialog`는 사용자에게 확인을 요청하며, `cx.ShowAlert`는 시스템 자체 알림창 또는 액션 시트를 표시합니다([다이얼로그](ui/dialog.md)).

## 메뉴

`ui.Menu`는 버튼에서 `ui.MenuItem` 목록을 엽니다. 버튼 옆에 열거나, 어떤 기기에서든 `Presentation` 설정에 따라 아래에서 올라오는 시트로 엽니다. `ui.ContextMenu`는 보조 클릭이나 길게 누르기에 대해 동일하게 동작하며, `ui.Popover`, `ui.HoverCard`, `ui.Tooltip`은 그 밖의 기준 위치에 고정된(anchored) 콘텐츠를 표시합니다.

```go
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", OnSelect: duplicate},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## 탭

`ui.Tabs[T]`는 선택된 옵션으로 슬라이드하는 인디케이터가 있는 세그먼트 컨트롤 또는 탭 목록입니다. `TabStyle`은 각 탭의 스타일 위에, `Selected`는 선택된 탭의 스타일 위에, `IndicatorStyle`은 인디케이터의 스타일 위에 병합됩니다. 밑줄(underline) 탭은 `IndicatorColor`를 설정하고 `IndicatorFit: TabsIndicatorContent`로 선을 콘텐츠 너비에 맞출 수 있습니다. `ui.Sidebar`는 데스크톱 사이드바용 섹션과 행을 담으며, `ui.TabShell`은 모든 창 너비에서 최상위 섹션들을 탭 바(기본값은 하단이며, `Edge` / `EdgeFunc`로 `cx.Env().VerticalBarEdge()`를 따르는 `mobile.TabBarAutomatic`을 포함해 시작(leading) 또는 끝(trailing) 가장자리의 세로 배치 가능) 옆에 유지합니다. 모든 탭에 `SystemIcon`이나 SVG `Icon`이 있는 경우 iOS에서는 모든 가장자리에서, Android에서는 하단 가장자리에서 시스템 탭 바를 사용하며, iOS에서 `SystemIcon`이 SF Symbol 이름이 아니거나 Android에서 drawable 이름이 아닌 곳에는 SVG를 표시합니다. 그렇지 않은 경우에는 SVG 아이콘으로 바를 직접 그립니다. 푸시된 상세 페이지가 보이는 동안에는 `BarHidden`을 설정해 두 종류의 바를 모두 숨기고 섹션의 안전 영역을 복원할 수 있으며, 바가 숨겨지거나 세로로 배치된 동안에는 `TabBarInset`이 0이 됩니다.

`mobile.TabBar`는 Liquid Glass 또는 Material 외관을 갖춘 iOS 및 Android의 시스템 탭 바입니다. `mobile.TabBarItem{Title, Icon, SVG}`로 구성된 `Items`, 선택된 인덱스, `OnChange`, 그리고 선택적 `Edge`(`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing`, 또는 iOS 27.1에서는 `cx.Env().VerticalBarEdge()`를 따르고 그 외에서는 하단 가장자리로 폴백하는 `mobile.TabBarAutomatic`) 또는 반응형 `EdgeFunc`를 받습니다. Android의 팔레트 역할인 `tabBarBackground`, `tabBarIdle`, `tabBarSelected`, `tabBarIndicator`는 각각 바의 배경, 비선택 아이콘과 레이블, 선택된 아이콘과 레이블, 선택된 알약 모양(pill) 배경을 설정합니다. 커스텀 팔레트는 각각 `Surface`, `TextMuted`, `Accent`, `accentContainer`로 폴백하며, `accentContainer`가 설정되지 않은 경우 12% 투명도의 강조(accent) 알약 배경을 사용합니다. 기본 팔레트는 Material의 배경과 비선택 색상을 유지하면서 선택된 탭만 강조 색상으로 틴트합니다. 다른 플랫폼에서는 공간을 차지하지 않으므로 해당 환경에서는 `ui.Tabs`를 표시하면 됩니다. `mobile.IsAvailable(cx)`는 바가 표시되는지 여부를 반환합니다. `Icon`은 플랫폼 아이콘 이름을 지정하며, `lucide.House()` 같은 `SVG`는 iOS와 Android에서 `Icon`이 SF Symbol이나 drawable을 가리키지 않는 경우 시스템 아이콘과 동일하게 바의 틴트가 적용되어 표시됩니다.

텍스트 배지를 표시하려면 `TabBarItem.Badge`를 `mobile.TabBadge{Text: "3"}`으로, 점(dot) 배지를 표시하려면 `mobile.TabBadge{Dot: true}`로 설정하세요. `ItemsFunc`를 통해 업데이트하고, 제로 값을 반환해 배지를 지울 수 있습니다. `ui.ShellTab.Badge`는 시스템 바와 직접 그린 바 모두에 대해 `func() mobile.TabBadge`를 받습니다. 두 필드가 모두 설정된 경우 텍스트가 우선합니다. UIKit은 점 배지에 대해 빈 네이티브 배지를 표시합니다. 보조 기술은 배지 텍스트나 새 콘텐츠를 뜻하는 현지화된 단어를 읽습니다. TSX에서는 네이티브 항목에 `badge: { text: "3" }` 또는 `badge: { dot: true }`를 사용하고, `TabShell` 항목에는 해당 값을 반환하는 함수를 전달합니다.

```go
mobile.TabBar{
	Items: []mobile.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}},
	Selected: func() int { return tab.Get() },
	OnChange: func(i int) { tab.Set(i) },
}
```

## 다음 단계

- [폼](forms.md): 스택의 페이지 안에 들어가는 내용.
- [애니메이션](animation.md): 페이지와 모달이 움직이는 방식.
- [레이아웃](layout.md): 헤더 주변의 안전 영역.

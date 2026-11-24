---
title: 메뉴
description: 트리거에서 열리는 메뉴, 컨텍스트 메뉴 및 메뉴 항목.
group: 컴포넌트
order: 42
---

`ui.Menu`는 버튼에서 `ui.MenuItem` 목록을 엽니다. 기기에 관계없이 버튼 옆에 열거나, `Presentation: base.PresentSheet`를 설정해 아래에서 올라오는 시트로 엽니다. `ui.ContextMenu`는 포인터 위치에서 동일한 항목들을 엽니다.

```go preview=menu/basic height=280
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", Icon: lucide.Pencil, OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", Shortcut: "mod+d", OnSelect: duplicate},
	ui.MenuSeparator{},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

메뉴는 스스로 열리고 닫히며, `Open`과 `OnOpenChange`를 사용하면 앱이 열림 상태를 직접 제어할 수 있습니다. `Trigger`는 버튼의 외관과 콘텐츠를 담당하며 메뉴가 그 누르기 입력을 받습니다. 팝업은 트리거 옆의 여유 공간을 절대 벗어나지 않습니다. 전체가 들어맞는 쪽으로 열리며, 항목이 그 공간이나 `MaxHeight`를 넘어가면 스크롤됩니다.

## 항목

`ui.MenuItem`은 `Text`, 선택적인 `Icon`, 그리고 `Shortcut`(`"mod+c"`, 팝업에서는 해당 플랫폼 표기법으로 표시됨)을 보여 주며, 선택 시 `OnSelect`를 호출하고 `NoCloseOnSelect`가 아니면 메뉴를 닫습니다. `Disabled`는 항목을 비활성화하고, `Checked`는 옆에 체크 표시를 보여주며, `Variant: ui.MenuItemDanger`는 위험한 작업을 나타냅니다. `ui.MenuLabel`은 그룹의 머리말 역할을 하고 `ui.MenuSeparator`는 그룹을 나눕니다. `ui.Submenu`는 하위 항목들을 담으며 항목 옆에 열리거나, 시트에서는 주변 항목 자리를 대체해 열립니다.

```go preview=menu/checked height=320
ui.Menu{Trigger: ui.Button{Text: "View", Variant: ui.ButtonGhost}, Children: []core.Node{
	ui.MenuLabel{Text: "Sort by"},
	ui.MenuItem{Text: "Name", Checked: func() bool { return sort.Get() == "name" }, OnSelect: func() { sort.Set("name") }},
	ui.MenuItem{Text: "Date", Checked: func() bool { return sort.Get() == "date" }, OnSelect: func() { sort.Set("date") }},
	ui.MenuSeparator{},
	ui.Submenu{Text: "Share", Children: []core.Node{
		ui.MenuItem{Text: "Copy link", OnSelect: copyLink},
		ui.MenuItem{Text: "Email", OnSelect: emailLink},
	}},
}}
```

## 컨텍스트 메뉴

`ui.ContextMenu`는 `Trigger` 위에서 보조 클릭하거나 터치 스크린에서 길게 눌렀을 때 포인터 위치에 항목들을 엽니다.

```go preview=menu/context height=220
ui.ContextMenu{Trigger: fileRow, Children: []core.Node{
	ui.MenuItem{Text: "Open", OnSelect: open},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## 동작

Escape 키, 뒤로 가기 또는 바깥쪽을 누르면 메뉴가 닫힙니다. 메뉴는 `Placement`와 `Align`에 따라 배치되는 팝업이거나, 시트 프레젠테이션을 지정한 경우 아래에서 올라오는 시트입니다. 데스크톱에서는 아래 화살표, 위 화살표, Home, End, Page Down, Page Up 키로 포인터가 올라가 있는 항목부터 시작해 활성화된 항목들 사이를 이동하고, 오른쪽 화살표로 하위 메뉴를 열고 왼쪽 화살표로 닫으며, Return 키로 선택합니다. 키보드로 메뉴를 열면 첫 번째 항목에 포커스가 가며, 메뉴가 닫힐 때 포커스가 트리거로 돌아갑니다.

## 접근성

메뉴는 `RoleMenu`를, 각 항목은 `RoleMenuItem`을 보고합니다. 트리거의 콘텐츠에 이름이 없을 때는 `Label`이 트리거의 이름을 지정합니다. 스크린 리더는 항목의 텍스트와 비활성화 여부를 읽어 줍니다.

## 플랫폼

트리거와 항목은 앱이 구성하며, 시트, 팝업, 포커스, Escape 키 및 뒤로 가기는 플랫폼이 처리합니다. 팝업과 시트 중 무엇을 쓸지는 앱이 선택하며, NSGo가 기기에 따라 임의로 고르지 않습니다.

## 관련 문서

- [Select](select.md): 목록에서 값 하나 선택하기.
- [Dialog](dialog.md): 모달 서피스.
- [Tabs](tabs.md): 여러 섹션을 한꺼번에 표시.
- 갤러리의 Menu, ContextMenu, Command 페이지.
- [스타일이 없는 컴포넌트](../authoring/base.md#menu): 직접 원하는 외관으로 메뉴를 만들기 위한 `base.Menu`.

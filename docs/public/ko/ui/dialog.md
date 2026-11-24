---
title: 다이얼로그
description: 모달 서피스, 시트 및 알림창.
group: 컴포넌트
order: 41
---

`ui.Dialog`는 어두워진 앱 위에 표시되는 모달 서피스이며, 터치 스크린에서는 아래에서 올라오는 시트로 표시됩니다. `ui.Sheet`는 아래에서 올라오는 패널이고, `ui.AlertDialog`는 사용자에게 확인을 요청합니다.

```go preview=dialog/basic height=360
open := core.NewSignal(cx, false)
ui.Dialog{
	Open:         open.Get,
	OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
	Title:        "Rename the file",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { open.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

## 열기와 닫기

다이얼로그가 열려 있는지는 앱이 관리합니다. `OnOpenChange`는 다이얼로그가 요청하는 모든 변경 사항을 수신하며, 바깥 영역 누르기와 Escape 또는 뒤로 가기를 구분해 주는 `base.Reason`을 함께 전달받습니다. `OnClose`는 다이얼로그가 닫힌 뒤 실행됩니다. `NoCloseOnEscape`와 `NoCloseOnBackdropPress`는 Escape 키나 바깥 영역을 눌러도 닫히지 않도록 합니다.

`Title`과 `Description`은 서피스 상단에 표시되며(열려 있는 동안 변경되는 경우 `TitleFunc`와 `DescriptionFunc` 사용), `Footer`에는 버튼이 들어갑니다. `Bare`는 전체 서피스를 `Children`에 맡겨 앱 고유의 레이아웃으로 다이얼로그를 구성할 때 사용합니다. 데스크톱에서는 `Style`과 `StyleFunc`가 지정한 모서리에 맞춰 서피스 테두리 안쪽으로 클리핑되며, `Inset`은 데스크톱에서 창 가장자리로부터의 최소 거리를 설정합니다.

터치 시트에서 `Bare`는 기본 그래버(grabber)와 콘텐츠 패딩을 제거하여 `Children`이 패널을 채우도록 합니다. `NoGrabber`는 그래버만 숨기며, `ContentStyle`과 `ContentStyleFunc`는 콘텐츠 래퍼를, `TitleStyle`과 `TitleStyleFunc`는 제목을 커스터마이즈합니다. `Leading`과 `Trailing`은 취소 및 완료 버튼 같은 노드를 제목 행의 양 끝에 배치하고 그 사이에 제목을 가운데 정렬합니다. `StartingStyle`과 `EndingStyle`은 시트 전환 애니메이션을 커스터마이즈하며, 커스텀 타이밍이 필요하면 그 안에 `Transition`을 포함하세요.

`Style`은 서피스의 기본 외관 위에 병합됩니다. `StyleFunc`는 그 뒤에 병합되며 다이얼로그가 열려 있는 동안과 다시 열릴 때 화면 모드(appearance)를 포함해 자신이 읽는 시그널을 추적합니다. 예를 들어 `colors := ui.ColorsOf(cx)`일 때 `StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} }`는 테마를 따릅니다. 두 필드 모두 `ui.AlertDialog`에서도 동작합니다. 제로(zero) 색상은 기존 색상을 그대로 유지하므로, 색상을 지우려면 `core.RGBA(0xFFFFFF, 0)`과 같이 0이 아닌 투명 색상을 사용하세요.

## 시트

`ui.Sheet`는 어두워진 배경(backdrop) 위 하단에 표시되는 패널이며, 배경을 누르거나 Escape 또는 뒤로 가기를 누르면 `OnClose`가 호출됩니다.

접근성 다이얼로그 이름에는 `Label` 또는 `LabelFunc`를 설정하고, 테스트에서 패널을 찾으려면 `TestID`를 설정하세요. `LabelFunc`는 열려 있는 콘텐츠를 교체하지 않고 시그널을 따릅니다. `CloseOnSwipe: true`를 설정하면 아래로 스와이프하여 닫힘 임계값을 넘었을 때도 `OnClose`가 호출되므로, 해당 콜백에서 `Open`을 갱신하여 시트를 닫으세요.

그래버와 안전 영역 처리를 유지한 채 서피스의 배경, 모서리 둥글기, 패딩을 커스터마이즈하려면 `Style` 또는 `StyleFunc`를 설정하세요. `StyleFunc`는 읽는 시그널을 따릅니다. `CornerRadii` 없이 `CornerRadius`를 지정하면 기본 상단 모서리 둥글기 대신 네 모서리를 모두 둥글게 만듭니다.

```go preview=dialog/sheet height=320
ui.Sheet{Open: filters.Get, OnClose: func() { filters.Set(false) }, Children: filterControls}
```

## 알림창

`ui.AlertDialog`는 사용자에게 확인 또는 승인을 요청합니다. 각 액션을 누르면 알림창이 닫히며, 바깥 영역을 눌러도 닫히지 않습니다.

```go preview=dialog/alert height=320
ui.AlertDialog{
	Open:         confirm.Get,
	OnOpenChange: func(v bool, _ base.Reason) { confirm.Set(v) },
	Title:        "Delete the note?",
	Description:  "This cannot be undone.",
	Actions: []ui.AlertAction{
		{Text: "Cancel", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
		{Text: "Delete", Color: colors.Danger, Role: core.AlertDestructive, Default: true, OnPress: remove},
	},
}
```

`Role: core.AlertCancel`이 지정된 액션은 Escape 또는 뒤로 가기를 눌렀을 때 실행되며, 이어서 `OnOpenChange(false, base.ReasonCancel)`이 호출됩니다. `Default: true`는 데스크톱, 웹, Apple TV에서 알림창이 열릴 때 해당 액션에 키보드 포커스를 주고 Return 키로 선택할 수 있게 합니다. 액션을 누르면 `OnPress`가 실행된 뒤 `OnOpenChange(false, base.ReasonSelect)`가 호출됩니다. Return 키는 다른 액션이나 필드에 키보드 포커스가 없을 때 기본 액션을 실행하며, 포커스가 다른 액션으로 이동했다면 Return 키는 그 액션을 누르고, 알림창 위에 다른 다이얼로그가 열려 있으면 Return 키는 그 다이얼로그가 가져갑니다. 액션이 한 번 실행되면 `OnOpenChange`의 동작과 무관하게 알림창이 닫히며, `Open`이 false가 되었다가 다시 true가 되기 전까지는 다른 액션을 실행하지 않습니다. 여러 액션에 같은 표시가 있으면 첫 번째 액션이 우선합니다. 표시가 없으면 Escape는 액션을 실행하지 않고 창을 닫으며 Return 키에는 기본 액션이 없습니다. `Role: core.AlertDestructive`는 파괴적 액션임을 설명하며, 외관은 `Variant` 또는 `Color`로 선택합니다.

시스템 자체 알림창이나 액션 시트를 사용하려면 `cx.ShowAlert`를 사용하세요([플랫폼 서비스](../services.md#알림-대화상자와-클립보드)).

## 동작

모달은 뒤의 앱 상호작용을 차단하고 키보드 포커스를 가져가며, 막아 두지 않는 한 Escape 및 뒤로 가기를 누르면 닫힙니다. 터치 스크린에서 다이얼로그는 아래로 드래그하는 제스처를 따라 움직이다가 임계값을 넘으면 닫히는 시트로 동작합니다.

## 접근성

`ui.Dialog`는 데스크톱과 터치 스크린에서 `RoleDialog`(또는 명시한 `Role`)를 보고하고, `ui.AlertDialog`는 `RoleAlertDialog`를 보고합니다. 둘 다 제목(또는 제목만으로 충분하지 않을 때는 `Label`)을 읽은 뒤 설명을 읽습니다. 다이얼로그가 열리면 포커스가 안으로 이동하고 닫히면 원래 위치로 돌아가며, 배경(backdrop)은 포커스 정지점이 되지 않습니다. 바닥글의 버튼들은 각자의 역할과 레이블을 유지합니다.

## 플랫폼

Android와 터치 스크린에서는 다이얼로그가 아래에서 올라오는 시트로 표시되고, 데스크톱에서는 배경(backdrop) 위에 가운데 배치된 서피스로 표시됩니다. 콘텐츠는 동일하며 표시 방식만 달라집니다.

## 관련 문서

- [내비게이션](../navigation.md): 페이지, 탭, 모달을 함께 사용하기.
- [Menu](menu.md): 고정된 명령 메뉴와 컨텍스트 메뉴.
- [접근성](../accessibility.md): 포커스와 다이얼로그 역할.
- 갤러리의 Dialog, Sheet, AlertDialog 페이지.
- [스타일이 없는 컴포넌트](../authoring/base.md#dialog): iOS 프레젠테이션과 나만의 모션을 지원하는 `base.Dialog` 및 `base.Sheet`.

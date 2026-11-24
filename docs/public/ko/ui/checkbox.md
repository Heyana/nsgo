---
title: 체크박스
description: 선택됨, 선택 안 됨 또는 불확정 상태를 가지는 체크박스.
group: 컴포넌트
order: 34
---

`ui.Checkbox`는 옆에 레이블이 붙은 `ui` 외관의 체크박스입니다. 자체적으로 값을 보관하지 않으며, 앱이 선택 여부를 전달하고 `OnChange`에서 해당 값을 갱신합니다.

```go preview=checkbox/basic
done := core.NewSignal(cx, false)
ui.Checkbox{Checked: done.Get, OnChange: done.Set, Text: "Remember me"}
```

## 콘텐츠

`ui.Checkbox`는 박스 옆에 `Text` 또는 앱의 `Children`을 표시합니다. `Label`은 보조 기술에 전달되는 두 항목을 대체하며, `Hint`는 이 체크박스의 용도를 설명합니다.

## 상태

`Indeterminate`는 선택됨도 선택 안 됨도 아닌 상태로, 일부만 선택된 상태에서의 "전체 선택" 등에 사용됩니다. 이 상태에서 누르면 선택됨 상태로 전환을 요청합니다. `Disabled`는 누르기 동작과 포커스를 모두 막습니다.

```go preview=checkbox/indeterminate height=220
ui.Checkbox{
	Checked:       func() bool { return chosen.Get() == len(files) },
	Indeterminate: func() bool { return chosen.Get() > 0 && chosen.Get() < len(files) },
	OnChange:      chooseAll,
	Text:          "Select all",
}
```

## 누르기 보조 키

목록에서 Shift를 누른 채 클릭해 범위 선택을 해야 할 때는 `OnChange` 대신 `OnChangeEvent`를 사용하세요:

```go
ui.Checkbox{Checked: done.Get, Text: "Remember me", OnChangeEvent: func(next bool, press core.PressEvent) {
	if press.Modifiers.Shift {
		chooseRange(next)
		return
	}
	done.Set(next)
}}
```

이 콜백을 설정하면 `OnChange`를 대체하며, 값 갱신은 여전히 앱이 담당합니다. 데스크톱과 웹에서의 키보드 활성화는 앱 단축키 이후에도 보조 키 상태를 유지합니다. Windows에서는 Alt와 함께 Space 또는 Return을 누르면 창 메뉴로 이동합니다. TSX의 `onChangeEvent`는 `{ checked, press }`를 전달받습니다.

## 접근성

체크박스는 `RoleCheckbox`와 함께 선택됨 또는 불확정 상태를 보고합니다. 접근성 이름은 `Text`, `Label` 또는 자식의 텍스트가 됩니다. 키보드 포커스가 있을 때 Space 키로 전환할 수 있으며, 스크린 리더는 바뀐 상태를 안내합니다.

## 플랫폼

박스는 플랫폼 컨트롤이 아니라 앱에서 직접 그리므로 어디서나 동일하게 보입니다. 누르기 동작, 포커스 링, 스크린 리더는 플랫폼에서 제공합니다. 모든 타겟에서 역할과 레이블을 기준으로 테스트됩니다.

## 관련 문서

- [Switch](switch.md): 즉시 적용되는 설정.
- [폼](../forms.md): 폼의 선택 항목.
- [List](list.md): 행 안의 체크박스.
- 갤러리의 Checkbox 페이지.
- [스타일이 없는 컴포넌트](../authoring/base.md#checkbox): 나만의 외관을 가진 박스를 만들기 위한 `base.Checkbox`.

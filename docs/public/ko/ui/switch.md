---
title: 스위치
description: 세 가지 크기로 제공되는 켜기/끄기 설정.
group: 컴포넌트
order: 35
---

`ui.Switch`는 즉시 적용되는 설정 컨트롤입니다. 자체적으로 값을 보관하지 않으며, 앱이 값을 소유하고 `OnChange`에서 갱신합니다.

```go preview=switch/basic
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}
```

## 크기

`ui.Switch`는 `Checked`, `OnChange`, `Label` 및 `Size`(`SizeDefault`, `SizeSmall` 또는 `SizeLarge`)를 받습니다. 플랫폼의 프레임 클록에 맞춰 노브가 미끄러지고 트랙의 색상이 바뀝니다.

```go preview=switch/small
ui.Switch{Checked: sync.Get, OnChange: sync.Set, Label: "Sync automatically", Size: ui.SizeSmall}
```

스위치는 즉시 적용되는 설정에 사용합니다. 폼 내부의 선택에는 [체크박스](checkbox.md)를 사용하고, 여러 옵션 중 하나를 고를 때는 `ui.RadioGroup` 안의 `ui.Radio` 또는 [셀렉트](select.md)를 사용하세요.

## 접근성

스위치는 `RoleSwitch`와 켜짐/꺼짐 상태를 보고합니다. `Label`은 보조 기술이 읽어 주는 이름이며, 보통 스위치 옆에 있는 텍스트가 레이블이 됩니다. 키보드 포커스가 있을 때 Space 키를 누르거나 클릭하면 상태가 전환됩니다.

## 플랫폼

기본 컨트롤은 앱이 직접 그리므로 모든 곳에서 똑같이 보입니다. `Variant: ui.SwitchSystem`은 플랫폼 네이티브 컨트롤을 요청합니다(macOS에서는 NSSwitch, iOS에서는 UISwitch, Android에서는 Material 3의 52 × 32 dp 스위치). 다른 타깃에서는 기본 외관을 사용합니다. 누르기, 포커스, 스크린 리더는 플랫폼에서 제공됩니다. 휴대폰의 기본 스위치는 의도적으로 데스크톱과 같은 크기이며, `SizeSmall`은 특정 기기용이 아니라 밀집된 행을 위한 크기입니다.

## 관련 문서

- [체크박스](checkbox.md): 하나 이상의 선택.
- [폼](../forms.md): 폼에서의 설정.
- [접근성](../accessibility.md): 레이블과 포커스.
- 갤러리의 Switch 페이지.
- [스타일이 없는 컴포넌트](../authoring/base.md#switch): 나만의 외관을 가진 스위치를 만들기 위한 `base.Switch`.

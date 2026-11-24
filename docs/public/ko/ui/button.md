---
title: 버튼
description: 푸시 버튼, 변형(variant)과 크기, 그리고 아이콘 버튼.
group: 컴포넌트
order: 31
---

`ui.Button`은 `ui` 외관의 푸시 버튼으로, 창에 필요한 변형과 크기를 제공합니다. `OnPress`는 누르기가 완료되었을 때 실행됩니다.

```go preview=button/basic
ui.Button{Text: "Save", OnPress: save}
ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: cancel}
```

## 변형과 크기

`ui.Button.Variant`는 `ButtonPrimary`(기본값), `ButtonSecondary`, `ButtonOutline`, `ButtonGhost`, `ButtonTinted` 또는 `ButtonDanger`입니다. Tinted와 danger는 부드러운 채우기 배경 위에 테마의 강조 색상이나 위험 색상을 사용합니다. 데스크톱 및 TV 이디엄(idiom)에서 `Size`는 `SizeDefault`(28포인트), `SizeSmall`(24포인트) 또는 `SizeLarge`(32포인트)입니다. 터치 이디엄에서는 동일한 크기가 각각 44, 40, 48포인트이므로 스타일을 다시 지정하지 않고도 버튼을 사용할 수 있습니다.

```go preview=button/variants
ui.Button{Text: "Delete", Variant: ui.ButtonOutline, OnPress: remove}
ui.Button{Text: "Continue", Size: ui.SizeLarge, OnPress: next}
ui.Button{Text: "Save", Icon: lucide.Check, Flex: 1, OnPress: save}
```

`Color`는 변형의 색상 대신 앱에서 지정한 색상으로 버튼을 채우며, `Tint`는 아이콘 색상을 지정합니다. `Flex`는 행에서 버튼이 차지할 비율을 지정하고, `Style`은 버튼의 레이아웃을 배치합니다. 예를 들어 컨테이너를 가득 채우거나, 앱 고유의 크기를 설정하거나, 여백(margin)을 추가하거나, 모서리를 둥글게 만들 수 있습니다. `TestID`는 테스트용 이름을 지정합니다.

```go
ui.Button{Text: "Continue", Size: ui.SizeLarge,
    Style: core.Style{Width: core.Pct(100), Height: core.Pt(50)}, OnPress: next}
```

아이콘만으로 이루어진 버튼은 `ui.IconButton`입니다. `Variant`로 `IconButtonGhost`, `IconButtonRaised` 또는 `IconButtonFilled`를 가지며, 테마의 인셋 색상이나 `Fill`로 채워진 평면 원형 모양이고, 포인트 단위의 `Size`와 보조 기술을 위한 `Label`을 받습니다. 데스크톱에서는 포인터를 버튼 위에 올려두면 이 레이블이 툴팁으로도 표시됩니다. 다른 문구를 표시하려면 `Tooltip`을 지정하고, `""`을 반환하면 툴팁을 표시하지 않습니다. `Text` 없이 `Icon`만 있는 `ui.Button` 역시 정사각형 형태가 되며 `Label`이 필요합니다.

```go preview=button/icon
ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost, OnPress: remove}
```

## 콘텐츠와 상태

`ui.Button`은 `Text`, 선택적인 앞쪽 `Icon`, 또는 둘 다를 표시합니다. 레이블의 폰트 크기, 굵기 또는 기타 텍스트 스타일을 커스터마이즈하려면 `TextStyle`을 사용하세요.

```go
ui.Button{Text: "Saving…", Disabled: saving.Get, OnPress: save}
```

`Disabled`는 함수를 받으므로 버튼이 시그널을 따릅니다. 비활성화된 버튼은 누르기 동작과 키보드 포커스를 모두 받지 않으며, 앱이 임의로 고른 색상이 아니라 비활성화 상태의 외관을 따릅니다.

`Busy`는 아직 실행 중인 동작에 사용합니다. 이 함수가 true를 반환하는 동안 버튼의 텍스트 색상과 같은 스피너가 콘텐츠 위에 표시되므로 primary 버튼의 강조 색상 위에서도 잘 보이며, 누르기 동작은 아무 일도 하지 않고, 보조 기술은 버튼이 작업 중(busy)임을 전달받습니다. 기본적으로 스피너 아래에서 기존 콘텐츠의 공간은 유지되며, 버튼은 키보드 포커스를 유지합니다. `BusyLabel` 또는 `BusyLabelFunc`는 스피너 옆에 메시지를 표시하고 작업 중인 동안 이를 버튼의 접근성 이름으로 보고합니다. 버튼은 최소한 평상시 너비 이상을 유지합니다.

```go preview=button/busy
ui.Button{Text: "Save", Busy: saving.Get, BusyLabel: "Saving…", OnPress: save}
```

`ui.Spinner`와 `ui.ProgressRing`은 강조 색상이 묻힐 수 있는 다른 표면 위에서 사용할 수 있도록 자체 `Tint`를 받습니다.

## 링크

`ui.Link{Text: "Docs", Href: "/docs"}`는 링크 역할로 대상 경로를 엽니다. `Style.Color`로 다른 색상을 지정하지 않는 한 텍스트와 호버 밑줄에 강조 색상을 사용하며, `Style.Hover.Color`는 호버 시 색상을 설정합니다.

## 접근성

버튼은 `RoleButton`과 자신의 텍스트를 이름으로 보고합니다. `Label`은 보조 기술에 전달되는 텍스트를 대체합니다. 아이콘만 있는 버튼에 `Label`이 없으면 읽을 수 없으므로 반드시 이름을 지정하세요.

데스크톱, 웹, Apple TV에서는 Return과 Space 키로 포커스된 버튼을 누를 수 있으며, 휴대폰의 스크린 리더는 버튼을 클릭으로 누릅니다.

## 플랫폼

이 버튼은 플랫폼 네이티브 버튼이 아니라 어디서나 동일한 방식으로 그려지는 뷰이며, 누르기, 포커스, 접근성은 플랫폼에서 제공됩니다. 터치 이디엄에서는 위에서 설명한 것처럼 더 큰 레이아웃 타겟 크기를 사용합니다.

## 관련 문서

- [폼](../forms.md): 폼에서의 버튼과 유효성 검사.
- [스타일과 테마](../styles.md): 버튼이 색상을 가져오는 테마.
- [접근성](../accessibility.md): 역할, 레이블 및 포커스.
- 갤러리의 Button 페이지.
- [스타일이 없는 컴포넌트](../authoring/base.md#button): 나만의 외관을 가진 버튼을 만들기 위한 `base.Button`.

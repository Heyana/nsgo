---
title: 입력 필드
description: 포커스와 소프트웨어 키보드를 지원하는 제어 또는 비제어 방식의 플랫폼 텍스트 필드.
group: 컴포넌트
order: 33
---

`ui.Input`은 `ui` 외관을 따르는 플랫폼 텍스트 필드로, `Multiline`을 지정하지 않는 한 한 줄로 표시됩니다. 스타일이 적용된 모든 필드와 마찬가지로 `base.Input`의 필드를 가집니다. `ui.Field`는 자체 크기가 없고 포인터나 포커스를 따라 변하지 않는 테두리 안에 들어 있는 동일한 필드입니다. `Background`에 `core.Transparent`를, `BorderWidth`에 `core.Zero`를 지정하면 박스가 사라져 일반 텍스트처럼 보이지만 편집할 수 있는 필드가 됩니다.

```go preview=input/basic
name := core.NewSignal(cx, "")
ui.Input{
	Value:       name.Get,
	OnChange:    name.Set,
	Placeholder: "Your name",
	TestID:      "name",
}
```

## 제어와 비제어

`Value`를 지정하면 제어(controlled) 모드가 되어 앱이 값을 보관하고 `OnChange`에서 갱신합니다. `DefaultValue`를 지정하면 필드가 자체적으로 값을 유지합니다. 제어 필드는 값을 `""`로 설정하여 비울 수 있습니다.

```go preview=input/submit
draft := core.NewSignal(cx, "")
ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "New task",
	OnSubmit: func(string) { add(); draft.Set("") }}
```

## 필드

`Multiline`은 텍스트를 줄바꿈하며 세 줄 높이로 시작해 스타일에 `Height`나 `MinHeight`가 설정되지 않는 한 텍스트에 맞춰 늘어납니다. `Masked`는 문자를 가리고, `Readonly`는 텍스트 선택과 복사는 허용하되 편집만 막으며, `Disabled`는 필드를 비활성화하고, `MaxLength`는 사용자가 입력할 수 있는 UTF-16 단위 길이를 제한합니다. `Placeholder`는 `PlaceholderFunc`를 통해 함수로도 지정할 수 있습니다.

```go
ui.Input{Masked: true, Value: password.Get, OnChange: password.Set,
	AutoComplete: core.AutoCompletePassword}
```

`KeyboardType`은 소프트웨어 키보드 종류(`KeyboardEmail`, `KeyboardNumber`, `KeyboardURL` 등)를 선택하고, `ReturnKeyType`은 Return 키의 레이블을 지정하며, `SubmitBehavior`는 여러 줄 필드에서 Return 키가 제출할지 줄바꿈할지를 결정합니다. `AutoCapitalize`, `NoAutoCorrect`, `AutoComplete`는 시스템의 입력 보조 기능을 세부 조정합니다.

`InputSource`는 필드에 포커스가 있는 동안 키보드 입력 소스를 선택하고 `KeepInputSource`가 설정되지 않은 한 포커스를 잃을 때 사용자의 기존 입력 소스로 복원합니다. 예를 들어 `"com.apple.keylayout.ABC"`는 macOS에서 코드나 명령을 입력하는 필드를 영어로 유지합니다. 이 ID는 플랫폼 고유의 값으로, Windows에서는 `"00000409"` 같은 키보드 레이아웃 ID, Linux에서는 Fcitx 5 또는 IBus 입력기가 됩니다. 활성화되지 않은 입력 소스이거나 휴대폰 및 웹 환경에서는 아무런 변경도 일어나지 않습니다.

```go
ui.Input{Placeholder: "Bundle ID", InputSource: "com.apple.keylayout.ABC"}
```

`Style`은 필드의 기본 외관 위에 병합되고, `StyleFunc`는 그 위에 병합됩니다. 높이, 패딩, 폰트, 색상 등 이들이 설정한 속성은 `ui`의 기본값을 대체하며 나머지는 유지됩니다. 둘 다 `BorderColor`나 `Hover`를 설정하지 않는 한 테두리는 포인터가 올라가면 어두워지고 포커스를 받으면 강조 색상을 띱니다. 제로(zero) `Padding`은 `ui`의 패딩을 그대로 유지하며, 명시적으로 설정된 길이 값인 `core.Zero`를 지정하면 패딩이 제거됩니다(`Padding: core.All(core.Zero)`).

```go
ui.Input{Placeholder: "Filter", Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
ui.Field{DefaultValue: "Untitled", Style: core.Style{FontSize: 24, FontWeight: ui.Bold,
	Background: core.Transparent, BorderWidth: core.Zero}}
```

## 포커스와 이벤트

`Ref`는 필드에 포커스를 주거나 해제합니다. `OnFocus`와 `OnBlur`는 포커스 변화를, `OnSelectionChange`와 `OnCaretMove`는 선택 영역과 캐럿 위치를, `OnKeyPress`는 입력된 각 키를, `OnPaste`는 붙여넣은 내용을, `OnCompositionChange`는 입력기(IME) 조합 상태를 추적합니다.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

소프트웨어 키보드는 휴대폰 화면 하단을 덮습니다. 콘텐츠의 하단 패딩이나 마진에 `core.EnvKeyboard`를 지정하면 콘텐츠가 키보드 위에 유지되며 키보드와 함께 애니메이션으로 이동합니다. `cx.DismissKeyboard()`는 키보드를 닫고, `cx.Env().KeyboardHeight()`는 키보드 높이를 읽습니다.

## 유효성 검사

규칙은 앱이 관리합니다. 시그널이 오류를 보관하고 `OnBlur`나 `OnSubmit`에서 오류를 설정하며, 필드 아래의 메시지로 이를 표시합니다([폼](../forms.md#validation)).

```go preview=input/validation
err := core.NewSignal(cx, "")
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{Value: email.Get, OnChange: email.Set, Label: "Email",
		OnSubmit: func(string) {
			if !strings.Contains(email.Get(), "@") {
				err.Set("Enter an email address.")
			} else {
				err.Set("")
			}
		}},
	ui.Textf(err.Get, 12, ui.Regular, colors.Danger),
}}
```

## 접근성

필드는 `RoleTextField`를 보고하거나, 검색 상자용으로 `Role`이 지정된 경우 `RoleSearchField`를 보고하며 `Label`을 읽습니다. `Placeholder`는 레이블이 아니므로, 눈에 보이는 레이블이 없는 필드에는 `Label`을 지정하세요. `Readonly`와 `Disabled` 상태도 함께 보고되며, `OnSubmit`은 키보드의 실행 동작이 됩니다. 한 줄, 마스킹, 여러 줄 필드는 macOS, iOS, Linux에서 현재 `Placeholder`를 네이티브 접근성 플레이스홀더 또는 힌트로 노출하며, 여러 줄 필드의 그려진 플레이스홀더 레이블을 위한 별도의 포커스 정지점 없이도 갱신되고 지워집니다.

## 플랫폼

iOS, Android, Windows, Linux는 각 플랫폼의 네이티브 필드를 사용하고, 웹에서는 `<input>` 또는 `<textarea>`를 사용합니다. 소프트웨어 키보드, 자동 완성, 입력기(IME), 텍스트 방향은 플랫폼에서 제공합니다. 포커스와 타이핑은 모든 타겟에서 동일한 방식으로 테스트됩니다.

## 관련 문서

- [폼](../forms.md): 필드, 유효성 검사, 선택 컨트롤 및 키보드.
- [접근성](../accessibility.md): 레이블과 포커스.
- [레이아웃](../layout.md): 키보드 위에 콘텐츠 유지하기.
- 갤러리의 Input 및 Form 페이지.

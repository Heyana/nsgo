---
title: 터미널
description: 터미널 그리드에서 프로그램을 실행하거나 원격 세션을 표시합니다.
group: 핵심 개념
order: 63
---

`base/terminal`은 세션(`Terminal`)과 해당 `View`를 제공합니다. 데스크톱에서는 로컬 프로그램을 실행하며, iOS와 브라우저에서는 원격 프로그램에 연결된 `io.ReadWriteCloser`인 `Options.Conn`을 사용합니다. 앱이 세션을 소유하며, 사용이 끝나면 닫아야 합니다.

```go
t, err := terminal.New(terminal.Options{CursorBlink: true})
if err != nil {
	return core.Text{Content: err.Error()}
}
cx.Cleanup(func() { t.Close() })
return terminal.View{
	Terminal: t, AutoFocus: true,
	MinimumContrast: 4.5, PaddingColor: terminal.PaddingExtend,
	Style: core.Style{FlexGrow: 1},
}
```

`Options.Cursor`는 기본 블록(block), 세로 막대(bar) 또는 밑줄(underline) 모양을 선택합니다. `CursorBlink`는 기본 깜빡임을 활성화하며, 제로 값은 깜빡이지 않고 고정됩니다. 프로그램은 DECSCUSR 또는 DEC 모드 12를 통해 커서 모양과 깜빡임을 재정의할 수 있습니다. 포커스가 없는 커서는 속이 빈 고정된 모양으로 표시되며, 입력 조합 중이거나 동작 줄이기 설정이 켜져 있을 때도 고정 상태를 유지합니다. 플랫폼의 애니메이션 클록이 터미널 텍스트를 엔진에서 다시 그리지 않고도 깜빡임을 구동합니다.

`View.Font`, `Theme`, `DarkTheme`은 글꼴과 색상 팔레트를 설정합니다. 그리드는 인접한 셀을 디바이스 픽셀에 맞추어 유니코드 괘선(box-drawing) 및 블록 문자와 Powerline 구분 기호 U+E0B0–U+E0B7을 직접 그립니다. 그 외의 텍스트는 플랫폼 글꼴과 폴백 글꼴을 사용합니다.

`MinimumContrast`는 반전 색상, 흐린 텍스트(faint text), 선택 영역을 포함해 실제 셀 배경에 대한 일반 텍스트의 대비를 높입니다. 1–21 범위의 값은 WCAG 대비율을 사용하며, 0은 색상을 그대로 유지합니다. 그래픽과 이모지는 원래 색상을 유지합니다. `Transparent`를 사용하더라도 대비 계산은 여전히 터미널에 설정된 배경을 기준으로 합니다.

`Padding`은 그리드 바깥쪽에 포인트 단위의 여백을 둡니다. `PaddingColor`는 해당 여백의 배경을 선택합니다:

| 값 | 동작 |
| --- | --- |
| `PaddingBackground` | OSC 11 변경 사항을 포함한 터미널 배경 |
| `PaddingExtend` | 가장 가까운 셀의 배경; 기본 화면에서는 프롬프트 행, Powerline 행 또는 기본 배경 셀을 포함하는 행 옆의 위/아래 확장을 억제합니다 |
| `PaddingExtendAlways` | 이러한 예외 없이 모든 가장자리로 확장합니다 |

TSX 앱은 `nsgo/base`에 `Terminal`을 등록하는 부수 효과를 위해 `tsx/terminal`을 임포트합니다. prop은 `cursorBlink`, `minimumContrast`, `paddingColor`이며, `paddingColor`는 `background`, `extend`, `extend-always` 값을 받습니다. 외관 관련 prop은 세션이 계속 진행되는 동안에도 변경할 수 있습니다.

## 브라우저에서

앱은 고정된 버전의 libghostty-vt WebAssembly 모듈을 내장하고 동일한 터미널 뷰와 입력 경로를 실행합니다. 앱을 빌드할 때 Zig, 네이티브 라이브러리 설치 또는 런타임 다운로드가 필요하지 않습니다.

브라우저의 WebSocket을 `io.ReadWriteCloser`에 맞게 어댑팅하세요. 즉, 바이너리 바이트를 전송하고 들어오는 메시지를 스트림으로 합치거나 나눕니다. 인증, 재연결 및 크기 조정/제어 메시지는 앱과 서버가 담당하며, NSGo는 터미널 바이트만 처리합니다. [브라우저 인수 테스트 픽스처](https://github.com/nsgo-dev/nsgo/blob/main/internal/testhost/testdata/terminalweb/socket_js_test.go)에서 종료 처리가 포함된 완전한 바이트 스트림 어댑터를 확인할 수 있습니다.

## 동작과 확대/축소

`View.Keybinds`는 `Keybind{Key, Action, Amount}`로 기본 키 바인딩을 재정의합니다. 키는 `mod+=` 또는 `ctrl+shift+c`처럼 `Scope.OnKey`와 같은 이름을 사용합니다. 나중에 지정된 항목이 우선합니다. 바인딩은 이 터미널의 입력에 포커스가 있고 입력기(IME)가 조합 중이 아닐 때 실행됩니다.

동작에는 `CopyToClipboard`, `PasteFromClipboard`, `SelectAll`, `ClearScreen`, `IncreaseFontSize`, `DecreaseFontSize`, `ResetFontSize`, `SetFontSize`, `Ignore`가 있습니다. 창과 탭 동작은 앱의 키맵에서 처리합니다. `Ignore`는 키를 프로그램으로 보내지 않고 소비합니다.

기본적으로 `mod+=`와 `mod+shift+=`는 글꼴 크기를 1포인트 키우고, `mod+-`는 줄이며, `mod+0`은 `View.Font.Size`(설정되지 않은 경우 13)로 재설정합니다. `Amount`는 확대/축소의 단계(0일 때는 1) 또는 `SetFontSize`의 크기를 지정합니다. 크기는 1–255포인트로 제한되며, 유한하지 않은 요청은 무시됩니다. 확대/축소는 세션과 바깥쪽 뷰를 유지한 채 그리드의 행과 열을 갱신하고 프로그램 크기를 조정합니다.

```go
ref := &terminal.Ref{}
view := terminal.View{
	Terminal: session, Ref: ref,
	Keybinds: []terminal.Keybind{
		{Key: "ctrl+shift+k", Action: terminal.ClearScreen},
		{Key: "mod+=", Action: terminal.IncreaseFontSize, Amount: 2},
	},
}
```

`Ref.FontSize()`는 글꼴 크기를 표시하는 컨트롤을 위해 추적되며, `Ref.SetFontSize(size)`와 `Ref.Perform(action, amount)`는 이벤트 핸들러와 마찬가지로 앱의 엔진에서 실행됩니다. `Perform`은 해당 동작을 사용할 수 있는지 여부를 반환합니다. 마운트되지 않은 ref는 0의 크기를 반환하고 동작을 거부합니다.

## 컨텍스트 메뉴

보조 클릭(우클릭)을 하면 현지화된 복사(Copy), 붙여넣기(Paste), 모두 선택(Select All), 지우기(Clear) 항목이 있는 스타일이 지정되지 않은 `base.ContextMenu`가 열립니다. 복사는 선택 영역이 있어야 활성화됩니다. 지우기는 프롬프트 위의 내용을 제거하며 대체 화면(alternate screen)에서는 비활성화됩니다. 터치 스크린에서는 컨텍스트 메뉴의 길게 누르기(long-press) 표시 방식을 사용합니다. 메뉴 표면은 `MenuClass`/`MenuStyle`로, 항목은 `MenuItemClass`/`MenuItemStyle`로 스타일링하세요.

`RightClickAction`을 `RightClickPaste`, `RightClickCopy`, `RightClickCopyOrPaste`(선택 영역이 있으면 복사, 그렇지 않으면 붙여넣기) 또는 `RightClickIgnore`로 설정하세요. Ignore는 이벤트를 감싸고 있는 앱 메뉴에 넘기므로, ref의 동작을 호출하는 스타일이 적용된 `ui.ContextMenu`를 제공할 때 사용하세요.

`nsgo/base`에 `<Terminal>`을 등록하려면 `tsx/terminal`을 임포트하세요. 반응형 prop인 `keybinds`와 `rightClickAction`은 세션을 유지한 채 뷰를 업데이트합니다. 동작 값은 `increase_font_size`, `copy_to_clipboard` 같은 Ghostty 이름을 사용하며, 우클릭 값은 `context-menu`, `paste`, `copy`, `copy-or-paste`, `ignore`입니다. 메뉴 스타일링 prop은 Go 이름을 lower camelCase로 변환해 사용합니다.

## 마우스 리포트

마우스 추적을 활성화한 프로그램은 선택한 프로토콜로 libghostty-vt가 인코딩한 누르기, 떼기, 드래그, 호버 이벤트를 요청한 대로 전달받습니다. Shift 키를 누르면 선택을 위해 클릭과 드래그 캡처를 우회합니다. `View.MouseReporting`은 선택적인 반응형 `func() bool`이며 nil일 때는 true로 간주됩니다. 프로그램이 캡처를 요청하더라도 마우스 선택과 네이티브 스크롤백 스크롤을 유지하려면 false를 반환하세요. 리포트가 활성화된 동안에는 프로그램의 우클릭이 감싸고 있는 컨텍스트 메뉴보다 우선합니다.

대체 화면에서 대체 스크롤 모드가 활성화되어 있고 마우스 추적이 요청되지 않은 경우, 세로 휠 입력은 일반 또는 애플리케이션 커서 키 모드를 따라 커서 키 입력으로 변환됩니다. `MouseScrollMultiplier{Precision: 1, Discrete: 3}`은 터치패드와 휠의 기본 리포트 횟수를 설정하며, 0은 이 기본값을 사용하고 양수 값은 0.01–10,000으로 제한됩니다. 소수점 단위의 입력은 온전한 한 단계가 될 때까지 누적됩니다. 일반 스크롤백은 플랫폼의 네이티브 스크롤과 관성을 유지합니다.

`nsgo/base`에 `<Terminal>`을 등록하려면 `tsx/terminal`을 임포트하세요. `mouseReporting` prop의 기본값은 true이며, `mouseScrollPrecision`과 `mouseScrollDiscrete`로 배율을 설정합니다.

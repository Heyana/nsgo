---
title: 컴포넌트와 상태
description: 엘리먼트, 컴포넌트, 시그널 및 스타일.
group: 핵심 개념
order: 50
---

## 엘리먼트

화면은 Go 구조체 값인 엘리먼트들의 트리입니다. `core`에는 `View`, `Text`, `Image`, `Scroll`, `Input` 같은 기본 요소가 들어 있습니다. `ui`에는 테마의 외관을 따르는 `Button`, `Select`, `DatePicker`, `EmojiPicker` 등 앱에서 사용하는 컨트롤이 있습니다. (`base`는 [컴포넌트 작성자](authoring/index.md)를 위해 외관이 없는 동일한 컨트롤들을 제공합니다.)

```go
core.View{
	Style: core.Style{Padding: core.All(24), Gap: 12},
	Children: []core.Node{
		core.Text{Content: "Hello"},
	},
}
```

`core.WebView`는 웹 콘텐츠를 표시합니다. 빈 `WebSource`는 `about:blank`를 로드하고 일반적인 로드 및 탐색 이벤트를 보고합니다. 이전 페이지를 지우려면 `SourceFunc`를 빈 소스로 설정하거나 `Ref.Load(core.WebSource{})`를 호출하세요. 페이지 자체 스크립트보다 먼저 실행되는 스크립트가 시그널을 따라가도록 하려면 `InjectedJavaScriptBeforeContentFunc`를 설정하세요. 값이 변경되면 사용자나 `Ref.Load`가 이동한 페이지를 포함해 현재 표시 중인 페이지 대신 새 스크립트와 함께 소스를 다시 로드합니다. `UserAgentSuffix`(TSX에서는 `userAgentSuffix`)를 설정하면 플랫폼의 기본 사용자 에이전트 뒤나 `UserAgent`가 설정된 경우 그 뒤에 `Version/26.0 Safari/605.1.15` 같은 토큰을 추가합니다. macOS와 iOS에서는 플랫폼 기본값에 붙이는 접미사가 뷰가 처음 생성될 때만 적용되며, HarmonyOS와 웹은 자체 사용자 에이전트를 유지합니다. `Reload`, `GoBack`, `GoForward`를 호출하려면 `core.Ref`를 연결하세요. 네이티브 대상에서 이 명령들은 앱 자체의 HTML 및 데이터 소스를 `core.AccessWebSchemes` 제한에서 면제된 상태로 유지하지만, 새 탐색을 생성하는 페이지는 여전히 접근 정책을 따릅니다.

tvOS에서는 `cx.Env().HasWebView()`가 false입니다. `WebView`를 마운트하면 WebKit을 로드하지 않고 `OnError`를 통해 `*core.WebError`를 보고하므로, 해당 대상에 맞는 앱 콘텐츠를 제공하세요.

macOS, iOS, Windows, Linux에서는 `OnNavigationStateChange`가 동일 문서 내 URL, 히스토리, 제목 변경도 보고합니다.

페이지의 `getUserMedia` 요청을 허용하려면 `WebView.Media`를 `core.WebMicrophone`, `core.WebCamera` 또는 둘 다로 설정하세요. `apple.info`에 사용 설명을 선언하고 `android.permissions`에 `RECORD_AUDIO` 또는 `CAMERA`를 선언해야 하며, 시스템은 여전히 사용자에게 권한을 요청합니다. Android에서 시스템 프롬프트가 열려 있는 동안 앱이 요청된 장치 중 하나를 제거하거나 페이지가 이탈하거나 뷰가 닫히면 대기 중인 요청이 거부됩니다. 이러한 변경 이후에는 페이지가 접근 권한을 다시 요청해야 합니다.

`WebView.Muted`와 `MutedFunc`(TSX에서는 `muted`)는 Windows와 Linux에서 Web Audio와 임베디드 프레임을 포함한 모든 페이지 오디오를 음소거합니다. macOS 12 및 iOS 15 이상은 음소거 상태 동안 미디어 재생을 일시 중단합니다. 다른 호스트는 평소 재생 상태를 유지합니다. 업데이트는 현재 페이지에 즉시 적용되며 탐색 후에도 유지됩니다.

TSX에서 `injectedJavaScriptBeforeContentLoaded`가 변경되면 `InjectedJavaScriptBeforeContentFunc`와 마찬가지로 동일한 웹 뷰에서 새 스크립트와 함께 소스를 다시 로드합니다. `injectedJavaScript`가 변경되면 새 웹 뷰를 만들어 소스를 새로 로드하며, `userAgent`, `userAgentSuffix`, `mobile`이 변경될 때도 마찬가지입니다. HarmonyOS에서는 두 주입 스크립트 중 어느 것이든 변경되면 새 웹 뷰를 만들고 새 네이티브 히스토리를 시작합니다. Windows와 HarmonyOS에서는 각 주입 스크립트가 메인 프레임의 블록 안에서 실행되므로 최상위 `let`, `const`, `class` 선언은 해당 스크립트에 지역으로 남습니다. 페이지가 볼 수 있는 전역을 만들려면 `window`에 할당하세요.

YouTube Shorts처럼 휴대폰에서만 세로 스와이프로 동영상을 전환하는 사이트를 위해 Windows, macOS, Linux에서 페이지를 휴대폰 화면처럼 보이게 하려면 `Mobile`(TSX에서는 `mobile`)을 설정하세요. `UserAgent`가 설정되어 있지 않다면 페이지에 휴대폰 사용자 에이전트와 호버 없는 터치스크린이 제공되며, 마우스 드래그나 스크롤되지 않는 영역에서의 휠/트랙패드 스크롤이 손가락 스와이프로 전달됩니다. Windows는 Chrome의 기기 모드처럼 휴대폰을 에뮬레이션합니다. macOS와 Linux는 스크립트로 처리하므로 페이지의 스타일시트에는 여전히 마우스로 보이고 터치 이벤트가 신뢰할 수 없는(untrusted) 이벤트로 간주되어 페이지가 무시할 수도 있습니다. iOS, Android, HarmonyOS, 웹, 미니 프로그램은 페이지를 원래 모습 그대로 표시합니다.

macOS, iOS, Windows, Linux에서는 `OnOpenWindow`가 웹 페이지의 새 창 요청을 수신합니다. 뷰는 팝업을 생성하지 않습니다. 요청의 `URL`과 `UserInitiated`를 확인해 `Ref.Load`로 같은 뷰에서 이동할지, URL을 다른 곳에서 열지, 무시할지 결정하세요. 제한된 뷰는 거부된 요청을 `OnError`로 보냅니다.

macOS, iOS, Linux, Windows, Android에서 웹 뷰의 페이지가 다운로드하는 파일을 저장하려면 `OnDownload`를 설정하세요. 다운로드는 앱 데이터 디렉터리 내 `downloads/<id>/<file name>`에 저장되며 시작, 바이트 수와 함께 완료, 또는 실패를 보고합니다. `OnDownload`가 없는 뷰나 제한된 뷰는 다운로드를 취소합니다.

`core.Video`는 플랫폼의 플레이어로 파일이나 URL을 재생합니다. macOS, iOS, tvOS에서는 유한한 길이의 비디오 클립을 위해 사운드가 담긴 별도의 파일이나 URL을 `Video.AudioSource`에 설정할 수 있습니다. 플레이어의 오디오는 해당 소스에서 나옵니다. 영상과 소리는 함께 시작되며 `Ref.Seek`은 둘을 같은 타임라인에서 이동시킵니다. 재생 길이는 비디오가 결정하며, 더 짧은 오디오는 끝나면 무음이 됩니다.

투명 클립은 macOS에서는 알파가 포함된 HEVC `.mov`로(iOS는 아직 미확인), Linux에서는 GStreamer의 codecalpha 플러그인(`gstreamer1.0-plugins-bad`, Linux 패키지의 `nsgo.yaml`에서 권장 패키지로 지정 가능)을 사용한 알파 포함 WebM으로 뒤쪽 배경을 비춰 줍니다. macOS, iOS, Windows, Linux에서 알파를 유지할 수 없는 로컬 클립(Apple 플랫폼의 WebM, Linux의 HEVC, 그리고 Windows 기기에서 Media Foundation의 알파 출력이 검증되기 전까지의 모든 Windows 클립)은 동영상을 불투명한 상자로 표시하는 대신 `OnError`로 이를 알립니다. 이때 오류는 `Reason`이 `core.MediaErrorAlphaUnsupported`인 `*core.MediaError`이므로 앱이 해당 클립을 직접 디코딩할 수 있습니다. Android와 웹, 그리고 모든 플랫폼의 원격 URL은 디코더가 출력하는 대로 재생하므로 불투명할 수 있습니다.

`OnPause`와 `OnPlay`는 Apple, Android, Windows, Linux, 웹의 내장 컨트롤을 따릅니다. `OnPause`는 컨트롤에 의한 일시 정지일 때 `VideoPauseControls`를 전달합니다. `Paused`는 소스가 열리거나 값이 변경될 때 적용됩니다. 볼륨, 배속, 레이아웃 변경은 `Paused`가 true인 상태에서도 컨트롤로 재생하는 것을 포함해 네이티브 일시 정지 또는 재생 상태를 유지합니다. 앱이 자체 재생 버튼을 표시하는 경우 `PausedFunc`를 상태에 바인딩하고 이 콜백들에서 상태를 갱신하세요.

`Rate`는 재생 속도를 설정하며 `RateFunc`는 재생 중에 속도를 변경할 수 있습니다. 속도 컨트롤을 표시할지 결정하려면 `OnLoad`에서 받은 `VideoInfo.RateChangeable`을 사용하세요. Linux는 GstPlay와 GStreamer의 `scaletempo` 필터가 설치된 경우 탐색 가능한 동영상의 속도를 변경할 수 있습니다. 폴백 플레이어는 1×만 사용합니다.

Android 및 Linux, macOS, iOS, tvOS의 직접 HTTP 파일(MP4 또는 MOV 등)에서 `Referer`, `User-Agent`, `Cookie` 같은 HTTP 요청 헤더를 보내려면 `Video.Headers`를 설정하세요.

```go
core.Video{
    Source:  "https://video.example/clip.mp4",
    Headers: map[string]string{"Referer": "https://video.example/player"},
}
```

비디오는 마운트될 때 맵을 복사하며 `SourceFunc`가 새 URL을 제공해도 동일한 헤더를 사용합니다. Apple 플랫폼에서는 탐색(seeking)을 위해 바이트 길이가 알려져 있고 범위 요청(range request)을 지원하는 소스를 사용하세요. 플레이어는 동일 출처 리다이렉트를 거치면서도 설정된 헤더를 전송합니다. Apple 플랫폼에서는 직접 HTTP `AudioSource` 파일에도 동일한 헤더가 적용됩니다. `.m3u` 또는 `.m3u8`로 끝나는 재생 목록 URL은 네이티브 로더를 사용합니다.

Linux에서는 GstPlay와 GStreamer의 `scaletempo` 및 `souphttpsrc` 플러그인을 설치하세요. `User-Agent`를 설정하거나 `souphttpsrc`에 `location-trusted` 속성이 없다면 최종 미디어 URL을 사용하세요. 해당 속성이 있고 기본 사용자 에이전트를 사용하는 경우, GStreamer는 동일 출처 리다이렉트 시 앱 헤더를 유지합니다. 앱의 헤더는 소스 파일에 적용되며 적응형 미디어 세그먼트는 자체 요청을 사용합니다.

## 컴포넌트

컴포넌트는 스코프(`*core.Scope`)를 받아 엘리먼트를 반환하는 함수입니다. 한 번만 실행되며, 이후의 변경 사항은 시그널이 주도합니다.

```go
func Greeting(cx *core.Scope) core.Node {
	return core.Text{Content: "Hello from NSGo"}
}
```

props를 받는 컴포넌트는 `Render` 메서드를 가진 타입입니다.

```go
type Badge struct{ Text string }

func (b Badge) Render(*core.Scope) core.Node {
	return core.Text{Content: b.Text}
}
```

`ui.ToastContent`는 `NoClose`로 닫기 버튼을 생략하고, `Trailing` 노드를 추가하며, `OnPress`를 통해 토스트를 닫을 수 있습니다. `Toasts.NoClose`는 스택 전체의 닫기 버튼을 숨깁니다.

`ui.Toasts.Style`과 `StyleFunc`는 배경, 테두리, 모서리, 패딩을 포함한 모든 토스트 표면을 커스터마이즈합니다. 스택의 스타일이 테마 위에 병합되고, 그다음 각 `ToastContent.Style`과 `StyleFunc`가 적용됩니다. 각 `StyleFunc`는 해당하는 `Style` 뒤에 실행되며 읽는 시그널이 변경될 때 업데이트됩니다. 이 스타일들의 `Color`는 제목 색상을 지정하고 설명에는 투명도를 낮춰 적용하므로, 어둡거나 색상이 있는 표면에서도 텍스트 가독성이 유지됩니다. 진입, 퇴장, 스와이프 단계에서는 여전히 모션 스타일이 적용됩니다. TSX에서는 `Toasts`의 `style` prop을 설정하고 단일 토스트에 대해서는 `show`에 `style` 또는 `styleFunc`를 전달하세요.

## 시그널

시그널은 값을 보유합니다. `ContentFunc`처럼 엘리먼트가 호출하는 함수 안에서 시그널을 읽으면 화면의 해당 부분만 구독하게 되어 값이 바뀔 때 그 부분만 업데이트됩니다.

```go
count := core.NewSignal(cx, 0)

core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }}

ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }}
```

슬라이스 전체를 담은 시그널은 이를 읽는 모든 행을 업데이트합니다. `core.NewCollection`은 항목들을 키별로 각각의 시그널에 담아 보관하므로 각 행이 자신의 항목만 추적하며, 항목 하나를 변경하면 해당 행 하나만 실행됩니다.

```go
todos := core.NewCollection(cx, func(t Todo) int { return t.ID }, nil)

core.List{Count: todos.Len, Key: func(i int) any { return todos.KeyAt(i) },
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		return core.Text{ContentFunc: func() string { t, _ := todos.At(row.Index()); return t.Title }}
	}}

todos.Update(id, func(t Todo) Todo { t.Done = true; return t })
```

다른 상태들도 같은 방식으로 시그널 위에 구축됩니다.

- `core.NewSelector`는 특정 키가 선택된 키인지 알려 주며 선택이 해제되는 행과 새로 선택되는 행만 다시 실행합니다.
- `core.NewHistory`는 실행 취소(undo)와 다시 실행(redo)이 가능한 값을 보관하며, 편집 메뉴를 위한 `CanUndo`와 `CanRedo` 시그널을 제공합니다.
- `core.NewPersisted`는 타입이 지정된 값을 버전이 포함된 JSON 형태로 앱 저장소에 보관하며 이전 버전이 기록한 데이터를 마이그레이션합니다.
- `core.From`은 다른 고루틴에서 값을 전달하는 구독 등 그래프 외부의 소스를 추적합니다.

## 스타일

`core.Style`은 CSS를 따릅니다. Flexbox와 Grid, 패딩과 마진, 모서리, 테두리, 그림자, 그라데이션, 변환(transform), 그리고 `Active` 같은 상태를 지원합니다. 엘리먼트는 `Class`를 통해 Tailwind 클래스도 받을 수 있습니다.

```go
core.Text{Content: "Done", Class: "self-start rounded-full bg-zinc-100 px-3 py-1 text-sm"}
```

테마는 라이트 모드와 다크 모드에서 `ui` 컴포넌트의 외관을 결정합니다.

## 데스크톱 앱

`nsgo.Run(Root)`는 하나의 루트 뷰를 표시합니다. 데스크톱 앱은 대신 `nsgo.Run(App)`을 사용해 `func App(app *core.Scope)`를 한 번 실행하고, 그 안에서 `desktop`으로 시그널을 공유하는 창, 패널, 트레이 아이콘, 전역 단축키를 직접 열 수 있습니다. 이는 macOS, Windows, Linux에서 동작하며, macOS의 `examples/tray`는 Dock에 나타나지 않고 메뉴 막대에 상주합니다. `nsgo.Run(App, nsgo.StartHidden())`은 메인 창을 표시하지 않은 상태로 열어 두므로 트레이 앱이나 로그인 시 자동 실행된 앱(`desktop.LaunchedAtLogin`)이 창을 깜빡이지 않으며, 앱이 필요할 때 `app.MainWindow().Show()`로 창을 띄웁니다. 휴대폰과 웹은 실행 즉시 앱을 표시합니다.

`desktop.SetWindowLevel`은 타이머나 데스크톱 펫처럼 창을 다른 창 위에 계속 띄우거나 아래에 둘 수 있게 하며, `desktop.SetWindowFrame`은 창의 위치와 크기를 조정합니다. `desktop.CanSetWindowLevel`과 `desktop.CanPlaceWindows`는 창의 스코프에서 이 기능들이 적용되는지 알려 줍니다. macOS, Windows, X11 환경의 Linux에서는 둘 다 동작하며, 컴포지터가 스태킹과 배치를 결정하는 일반 Wayland 최상위 창에서는 둘 다 동작하지 않습니다.

`desktop.WindowOptions{Transparent: true}`로 연 창이나 `nsgo.WindowTransparent()`를 지정한 메인 창은 배경과 프레임이 없으므로 앱이 아무것도 그리지 않은 부분에 데스크톱이 그대로 보입니다(데스크톱 펫이나 위젯 등). `Desktop.WindowDrag`가 설정된 뷰를 드래그해 창을 이동할 수 있습니다. `NoShadow`는 시스템 그림자를 제거하고 `SkipTaskbar`는 Windows 작업 표시줄과 Alt+Tab 및 X11 작업 목록에서 창을 숨기며, `desktop.SetWindowTransparent`, `SetWindowShadow`, `SetSkipTaskbar`로 나중에 변경할 수 있습니다. Windows에서는 투명하게 연 창(그림자가 없음)에서만 투명도를 변경할 수 있고, Wayland에는 작업 목록에서 창을 숨기는 힌트가 없습니다. 창 스코프에서 `desktop.CanMakeWindowsTransparent`와 `desktop.CanSkipTaskbar`로 적용 가능 여부를 확인할 수 있습니다. 투명 창은 Windows에서 DirectComposition이 필요하고 Linux에서는 컴포지터가 필요하며(컴포지터 없는 X11에서는 미지원), macOS에서는 `desktop.SetDockVisible`로 Dock에서 앱을 숨길 수 있습니다.

## 터미널

`base/terminal`을 임포트하고 원격 스트림용으로는 `terminal.New(terminal.Options{Conn: conn})`으로 세션을 생성하거나, macOS, Linux, Windows의 로컬 프로그램용으로는 `Command`를 설정하세요. `terminal.View{Terminal: session}`을 렌더링하고 세션이 끝나면 `Close`를 호출하세요. `Options.ImageLimit`는 Kitty 그래픽 저장소 용량을 바이트 단위로 제한합니다. 0은 64 MiB를 사용하고, 음수 값은 이미지를 비활성화합니다. 프로그램은 RGB, RGBA 또는 PNG 이미지와 유니코드 플레이스홀더 배치를 전송할 수 있습니다.

화면과 스크롤백에서 일치 항목을 강조 표시하려면 `Search(text)`를 호출하고, 선택 이동은 `SearchNext`와 `SearchPrevious`, 강조 해제는 `EndSearch`를 호출하세요. `Options.OnSearch`는 `SearchStatus`를 보고하며, `Theme.Search`, `SearchText`, `SearchSelected`, `SearchSelectedText`는 일치 색상을 설정합니다. OSC 133 마커를 내보내는 셸은 활성 명령 입력 영역 안에서 Alt/Option-클릭으로 커서를 이동할 수 있습니다. NSGo는 셸 통합 스크립트를 설치하지 않습니다.

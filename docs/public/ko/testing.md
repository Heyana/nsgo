---
title: 테스트
description: 창 없이 헤드리스로, 또는 네이티브 창에서 실제 사용자처럼 앱을 테스트합니다.
group: 툴체인
order: 21
---

## 테스트

`nsgotest`는 창 없이 앱을 시작합니다. 테스트는 실제 사용자처럼 텍스트나 역할로 컨트롤을 찾아 조작합니다:

```go
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestPress(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Press("text:Press me")
	app.Find("text:Pressed 1 times")
}
```

`TestMain`은 테스트가 타겟에서 실행될 때 플랫폼에 메인 스레드를 넘겨줍니다.

패키지에 프로세스 전체 수준의 설정과 정리가 필요하면 `MainFunc` 안에서 `m.Run` 전후에 배치하세요. `defer`로 등록한 정리 코드는 테스트 프로세스가 종료되기 전에 실행됩니다:

```go
func TestMain(m *testing.M) {
	nsgotest.MainFunc(func() int {
		setup()
		defer teardown()
		return m.Run()
	})
}
```

`Main(m)`은 `MainFunc(m.Run)`의 한 줄 축약 형태입니다. 타겟은 각 테스트를 자체 프로세스에서 실행하므로 이 설정과 정리는 테스트 프로세스당 한 번씩 실행됩니다.


함수 형태로 실행되는 앱 `nsgo.Run(func(app *core.Scope) {...})`은 `nsgotest.StartApp(t, fn)`으로 테스트하며, 이 함수는 `Run`과 마찬가지로 앱 자체 스코프에서 한 번 실행되어 애플리케이션 메뉴와 종료 핸들러가 유지됩니다. 데스크톱 `nsgotest.Environment`를 사용하면 `nsgo.Run`이 메인 창을 여는 환경(macOS, 그리고 앱 함수의 경우 Windows와 Linux)에서 앱의 메인 뷰가 메인 창에 표시됩니다. 여기서는 `desktop.CurrentWindow`와 `OnClosing`이 동작합니다. `app.CloseWindow()`는 창의 닫기 버튼 요청을 보내고 창이 닫혔는지 여부를 보고하며, 핸들러 단독 테스트는 핸들러를 직접 호출한 뒤 `WindowClosing.Canceled()` 또는 `OnQuitRequest`용 `QuitRequest.Canceled()`를 읽어 수행합니다. `app.RequestQuit(false)`는 사용자 종료 요청을, `app.RequestQuit(true)`는 로그아웃 같은 강제 종료 요청을 보냅니다. 두 요청 모두 헤드리스 환경과 macOS, Windows, Linux에서 동작하며, 승인된 요청은 `app.Quits`에 집계됩니다. `app`에 대한 스텝은 메인 창에서 동작합니다. `app.Opened`는 앱이 연 창, 패널, 트레이 뷰를 요청한 그대로 나열하며 여기에는 `Transparent`와 `SkipTaskbar`도 포함되고, `app.Window`는 메인 창의 요청을 `nsgotest.WindowRequest` 값으로 기록합니다. 크기 제한을 확인하려면 `nsgotest.WindowSetConstraints`와 요청의 `Constraints`를 사용하세요. 헤드리스 환경에서는 `app.Shares`가 공유 요청을 파일, 텍스트, 앵커가 담긴 `nsgotest.Share` 값으로 기록합니다. `app.DropFiles`는 드롭 존 내부의 어떤 요소든 지정할 수 있으며, 주변에서 가장 가까운 `OnFileDrop`이 설정된 뷰가 파일을 받습니다. `app.OpenLinks("myapp://settings")`는 헤드리스 테스트와 iOS, Android 타겟에서 앱 자체 링크를 `desktop.OnOpenLinks` 또는 `mobile.OnOpenLinks`로 전달합니다. `app.ReceiveShare(mobile.SharedItem{Text: "https://example.com", MIME: "text/plain"})`는 헤드리스 테스트와 iOS, Android 타겟에서 공유 항목을 `mobile.OnShared`로 전달합니다. 뷰의 `Bounds()`는 `Ref.Bounds`가 보고하는 것과 같은 창 포인트 기준 프레임입니다. 이미지 뷰의 `Image.Source`는 뷰가 표시하는 소스이므로 `app.Find("id:video").Image.Source.Version`과 `Pixels.Rect`를 통해 어떤 픽셀 프레임이 어떤 크기로 도달했는지 확인할 수 있습니다. 헤드리스 환경에서 `app.Press`는 타겟에서와 마찬가지로 시스템 스위치(`ui.SwitchSystem`)를 전환하며, `app.Emit("id:switch", "change", true)`는 값을 직접 보고합니다. `app.Navigate`와 `app.PostMessage`는 마운트되었지만 숨겨진 채 계속 로드 중인 웹 뷰에도 도달합니다. `View.Web`은 웹 뷰의 소스, 설정, 내비게이션 정책에 대한 `nsgotest.WebConfig` 스냅샷을 보유합니다. `View.WebCommands`는 앱이 웹 뷰로 보낸 명령을 기록합니다. `Command`를 `nsgotest.WebLoad`나 `nsgotest.WebSendInput` 같은 값과 대조해 확인하세요.

마운트된 이미지, 입력, 모달, 화면, 스크롤, 비디오 및 Picture in Picture 설정에는 `nsgotest` 구성 레코드가 사용됩니다. `View.Sticky`는 `nsgotest.StickyCell` 값을 보유합니다. 이미지 변형 맵과 스크롤 목록은 모델이 배치를 수신할 때 복사됩니다.

`app.Surfaces()`를 사용하면 열린 창, 패널, 트레이 뷰를 메인 창이 먼저 오도록 열린 순서대로 나열할 수 있습니다. `app.Surface("title:Settings")`, `app.Surface("id:settings-root")`, `app.Surface("index:1")`은 현재 제목, 루트 뷰의 테스트 ID 또는 목록 내 위치로 표면을 선택합니다. 셀렉터는 정확히 하나의 열린 표면과 일치해야 합니다.

macOS, Windows, Linux 타겟에서는 `app.Surface("id:palette").WindowState()`가 안정화(settle) 후 UI 스레드에서 실제 네이티브 창의 `Frame`, `Visible`, `Focused`를 읽습니다. 패널과 원래 창을 비교해 `NoActivate` 패널을 표시할 때 키보드 포커스가 원래 위치에 유지되었는지 확인하세요. Wayland처럼 네이티브 위치를 알 수 없는 경우 `Frame.Move`는 false가 됩니다. 헤드리스 테스트와 다른 타겟은 이 스텝을 건너뛰며, 앱이 요청한 위치나 포커스를 대신 반환하지 않습니다.

표면은 `Exists`, `Texts`, `Enabled`, `Clear`, `Focus`, `Hover`, `RightClick`, `DropFiles`, `Resize`, `Settle` 등 `App`의 뷰 쿼리, 입력 및 제스처 스텝, 미디어 리포트, 캡처, 크기 조정 및 대기 기능을 그대로 갖습니다. 이들은 해당 창을 선택해 동일한 `App` 동작을 재사용하며, 대기와 스텝은 앱 전체를 안정화합니다. `Post`는 원래 앱을 통해 그래프 작업을 큐에 넣고 디스패치하기 전에 선택한 표면이 여전히 열려 있는지 확인합니다. 앱 전체 시계, 서비스 픽스처, 메뉴 및 알림 응답은 원래 `App`에서 관리하세요.

`Config`는 열기 옵션을 `nsgotest.SurfaceConfig`로 반환하며, 이는 `app.Opened`에도 기록됩니다. `Screenshot`, `CapturePNG`, `Capture`는 네이티브 타겟에서 해당 표면을 캡처합니다. `Close`는 `OnClosing`을 통해 해당 창에 닫기를 요청하고 닫혔는지 여부를 보고합니다. 닫힌 핸들에서는 쿼리, 스텝, 큐에 넣은 포스트가 실패하며 표면 목록에서도 제거됩니다. 부모 창을 닫으면 자식 창도 함께 닫힙니다. 각 스텝의 일반적인 타겟 지원 범위 내에서 동일한 스텝들이 헤드리스 환경과 macOS, Windows, Linux 테스트 호스트에서 모두 동작합니다. 단일 표면 타겟은 메인 표면 하나만 나열합니다.

`app.Menu`, `app.DockMenu`, `app.PopupMenu`는 레이블, 역할, 체크 상태, 중첩 항목을 담은 `nsgotest.MenuItem` 스냅샷을 노출합니다. 항목을 실행하려면 레이블 경로를 인자로 받는 해당 선택 스텝을 사용하세요.

`app.ClickTray("index:0", core.MouseButtonLeft)` 또는 `app.ClickTray("title:Tools", core.MouseButtonRight)`는 헤드리스 데스크톱 테스트에서 활성 트레이 아이콘 클릭을 보고합니다. `ClickTrayIndex`와 `ClickTrayTitle`은 편의용 함수입니다. 플랫폼이 보고하는 아이콘 사각형을 모델링해야 하는 테스트라면 버튼 뒤에 `core.Bounds` 하나를 전달하세요. 이를 전달하지 않으면 `Tray.ClickBounds`를 사용할 수 없습니다. 이후 `ChoosePopup`으로 트레이 메뉴를 선택할 수 있습니다.

`Drag`는 포인터가 눌린 위치에서 8포인트 이상 이동하면 눌림 상태를 취소합니다. 웹에도 동일한 규칙이 적용되므로 시작 뷰로 되돌아오는 드래그는 `OnPress`를 발생시키지 않습니다. `app.DragTo("id:source", "id:target")`는 일반 터치 리스폰더를 통해 두 뷰의 중심 사이를 드래그합니다. 선택한 표면에도 `DragTo`가 있으며, TSX 테스트에서는 `await app.dragTo("id:source", "id:target")`를 사용합니다. 두 뷰 모두 보이도록 유지하세요. 이 스텝은 드래그 중에 스크롤하거나 드롭 핸들러를 직접 호출하지 않습니다. [드래그 앤 드롭](drag-drop.md)을 참고하세요.

`app.KeyCommands()`는 현재 포커스와 보이는 뷰 상태에서 실행할 수 있는 제목 있는 키보드 단축키 목록을 반환합니다. 헤드리스 환경과 타겟 모두에서 키, 제목, 그룹을 보고합니다.

`app.PressWith("id:row", core.Modifiers{Shift: true})`는 한 번의 누르기에 보조 키를 함께 누르고, `PressAtWith(selector, core.Point{X: 20, Y: 10}, mods)`는 뷰 내 특정 지점을 누릅니다. `Shift`, `Control`, `Alt`, `Platform`(Apple에서는 Command, 그 외에서는 Windows/Super 키)은 헤드리스 환경과 네이티브 타겟 입력 모두에서 `OnPressDown`, `OnPressUp`, `OnPress`로 전달됩니다. 네이티브 플랫폼 관례도 그대로 적용되므로, macOS의 Chrome에서는 Control-클릭이 컨텍스트 메뉴로 동작합니다. 이후 호출하는 `Press`나 `PressAt`에는 보조 키가 적용되지 않습니다. watchOS에는 키보드 보조 키 입력이 없으므로 보조 키가 포함된 누르기를 거부합니다.

타겟 스텝은 중첩된 뷰포트와 스크롤 축을 존중하면서 뷰포트 가장자리를 가리는 네이티브 바 위로 스크롤된 컨트롤이 드러나도록 합니다. `PressAt`은 지정된 뷰의 뷰포트 기준 좌표를 사용합니다. 포인터 타게팅, 가시성, 스크롤 한도는 헤드리스 환경과 타겟 모델 모두에서 중첩되거나 소수점 배율을 포함한 스크롤 콘텐츠 확대/축소(zoom)를 반영합니다. 뷰의 `Bounds()`는 `Ref.Bounds`와 마찬가지로 변환 전 레이아웃 프레임을 유지합니다.

앱이 포인터를 잠근 후에는 `app.PointerDelta(dx, dy)`가 잠긴 뷰로 상대 이동량을 보냅니다. 헤드리스 테스트에서는 `nsgotest.Environment(core.Environment{CanLockPointer: true})`로 활성화합니다. Escape 키는 헤드리스 테스트와 타겟 테스트 모두에서 잠금을 해제합니다.

누르기 스텝에서 텍스트 필드를 직접 지정할 수 있습니다. 헤드리스 환경과 모든 타겟에서 누른 직후 `app.Find("id:field").Focused`를 즉시 읽을 수 있으며, 눌린 뷰의 핸들러가 해당 필드에 포커스를 주는 경우에도 마찬가지입니다.

`app.SelectTab("id:tabs", 1)`은 마운트된 `mobile.TabBar`의 두 번째 항목을 선택합니다. iOS 또는 Android `Environment`를 사용한 헤드리스 환경에서 동작하며, iOS 및 Android 타겟에서는 네이티브 탭 바를 사용합니다. 인덱스는 0부터 시작하며 기존 항목과 일치해야 합니다. 휴대폰 내비게이션에는 이 메서드를 사용하세요. `Emit`은 모델 이벤트를 보고하며 헤드리스에서만 실행됩니다.

`app.Focus(selector)`는 탭하는 것처럼 필드에 포커스를 맞추며, 데스크톱이나 웹 `Environment` 또는 해당 타겟에서는 Tab 키를 통해 누를 수 있거나 `Focusable`인 뷰에도 포커스를 맞춰 이어지는 `app.Key("Return")`이 해당 뷰를 누르도록 합니다. 앱이 호출하는 `Ref.Focus()`도 Android, iPhone, iPad에서 소프트웨어 키보드를 띄우지 않고 활성화된 누를 수 있는 뷰나 `Focusable` 뷰에 포커스를 줍니다. 네이티브 모바일 테스트에서는 `app.Post(ref.Focus)`를 사용하고 포커스가 맞춰질 때까지 기다리세요. 모바일 타겟에서는 포커스 단언문이 네이티브 포커스 보고를 따르므로 거부된 요청이 헤드리스 모델을 통해 통과할 수 없습니다. 데스크톱과 웹 타겟에서는 `app.Focus` 스텝이 일반 뷰에 대해 여전히 Tab을 사용합니다. 플랫폼에서와 마찬가지로 누를 수 있는 뷰는 `OnPress`, `OnPressDown`, `OnPressUp`, `OnLongPress` 중 하나 이상을 가지며, Return이나 Space를 누르면 해당 뷰가 처리하는 down, up, press 이벤트를 보냅니다. 따라서 `OnLongPress`만 있는 뷰는 포커스는 받지만 키로 눌러지지는 않습니다.

헤드리스 실행은 프로젝트 타겟과 무관하게 기본적으로 폰 이디엄(phone idiom)을 사용하므로 `cx.Env().Desktop()`은 false이고 호버 툴팁은 표시되지 않습니다. 데스크톱 화면 구성을 테스트하려면 `nsgotest.Idiom(core.IdiomDesktop)`을 전달하세요. `Environment` 옵션으로 설정한 항목을 포함해 나머지 환경 설정은 그대로 유지됩니다. 키보드 포커스는 이디엄이 아니라 환경의 OS를 따릅니다.

macOS 타겟 테스트에서 테스트 창은 AppKit의 화면 크기 제약을 무시하므로 `nsgotest.Size`와 `Resize`는 보이는 화면보다 높이가 크더라도 요청된 콘텐츠 크기를 그대로 유지합니다. iOS와 tvOS에서는 `app.Resize(width, height)`가 테스트 호스트의 `UIWindow` 크기를 조정하고 네이티브 레이아웃을 기다립니다. 각 테스트는 해당 씬의 크기로 시작합니다. 이는 다양한 뷰포트 크기에서 콘텐츠를 점검할 수는 있지만 Split View, Slide Over, Stage Manager 창을 배치하지는 않습니다. 라이브 자동화는 실행 중인 씬의 크기를 따르며 `Resize`를 거부합니다.

실제 타겟에서 `nsgotest.Environment`는 화면 모드(appearance), 로케일, 텍스트 크기, 동작 줄이기(reduced motion), 대비 증가(increased contrast), 타이밍 햅틱 지원, 발열 상태, 저전력 모드, 로그인 실행 사유, 전면 애플리케이션 여부 및 플랫폼의 추적 여부, 그리고 기기 이름과 모델을 고정합니다. 디스플레이 목록과 창의 현재 디스플레이(주사율 포함)를 고정하려면 `DisplayID`와 함께 `Displays`를 설정하세요. 나머지는 OS, 이디엄, 안전 영역, 시스템 헤더 높이 등 타겟 자체의 값을 유지합니다. 따라서 `core.OSWindows`가 설정된 `Environment`를 사용해도 `go tool nsgo test -target macos`가 앱을 Windows처럼 레이아웃하지는 않습니다. 해당 테스트는 전체 `Environment`가 적용되는 헤드리스로 실행하거나 `-target windows`로 실행하세요. `nsgotest.Idiom` 역시 헤드리스에서만 적용됩니다. `nsgotest.Size`는 macOS, Windows, Linux 및 웹에서 테스트 창의 크기를 지정하며, Android, iOS, tvOS, watchOS는 기기의 화면 크기를 유지합니다. `-target macos`에서는 `Environment` 없이도 앱이 `nsgo.Run`과 동일하게 창을 엽니다. 루트 뷰나 `SetMainView`로 설정한 뷰가 메인 창에 표시되어 `desktop.CurrentWindow`와 `OnClosing`이 동작하며, `nsgotest.Size`로 크기를 지정하지 않는 한 480×640 포인트로 열리고 헤드리스와 마찬가지로 스텝이 이 창에서 동작합니다. 닫기 버튼이나 Close 메뉴 항목의 단축키로 창을 닫으면 `OnClosing` 핸들러에 묻고, 창이 닫히면 더 이상 조작할 뷰가 남지 않으며 마지막 창이 닫힐 때 앱이 요청하는 종료는 테스트를 끝내는 대신 `app.Quits`에 집계됩니다.

바인딩이 키를 가져가지 않고 해당 단축키를 가진 애플리케이션 메뉴 항목도 없을 때, `app.Key("mod+a")`는 포커스된 필드의 텍스트를 전체 선택하고 `mod+c`, `mod+x`, `mod+v`는 `app.Clipboard`를 통해 복사, 잘라내기, 붙여넣기를 수행합니다. macOS, Windows, Linux `Environment`에서는 바인딩이나 포커스된 뷰 또는 필드가 가져가지 않은 키가 해당 키를 단축키로 가진 활성화된 애플리케이션 메뉴 항목을 선택합니다. 따라서 `app.Key("mod+r")`로 메뉴 단축키가 해당 항목에 도달하는지 테스트할 수 있습니다. 비활성화된 항목은 키를 가져가지 않으며, 필드에 포커스가 있을 때 `NotInText` 항목은 키를 가져가지 않고 뒤따르는 항목들이 그 키를 받지 못하게 막습니다. 또한 Linux에서는 Control, Alt 또는 로고 키 없이 누른 키를 어떤 메뉴 항목도 가져가지 않습니다. 헤드리스 환경에서 `OnSelect`가 없는 표준 역할의 항목을 `app.ChooseMenu`나 단축키로 선택하면 데스크톱과 동일하게 동작합니다. Close는 `OnClosing` 핸들러를 통해 메인 창을 닫고, Quit은 `OnQuitRequest` 핸들러에 물은 뒤 승인된 요청을 `app.Quits`에 집계하며, Minimize, Zoom, Toggle Full Screen은 환경이 보고하는 메인 창의 상태를 변경하되 앱의 다른 창들은 그대로 둡니다(`app.SetWindowEnvironment`로 특정 창 하나의 환경을 바꿀 수 있습니다). 편집 역할들은 포커스된 필드에서 동작합니다. 그 외의 다른 역할이나 필드에서의 Undo 또는 Redo는 키를 삼키지 않고 스텝을 실패 처리하며, `-target`에서는 이러한 항목이 테스트를 건너뜁니다. 헤드리스, 데스크톱 타겟, 웹에서는 필드가 동작하기 전에 `KeyAlways` 바인딩이 Return, Backspace, 전체 선택을 먼저 가져가며, `KeyAuto`와 `KeyNotEditing`은 이를 포커스된 필드에 맡깁니다. Android와 iOS 테스트 호스트는 Backspace와 전체 선택에 대해 `KeyAlways` 바인딩에 우선권을 주며, Android Control 단축키는 그다음에 네이티브 편집으로 전달되고 iOS는 바인딩되지 않은 키를 소프트웨어 키보드로 라우팅합니다. iOS의 소프트웨어 Return은 아직 키맵에 키를 먼저 전달하지 않습니다([#1351](https://github.com/nsgo-dev/nsgo/issues/1351)). 헤드리스와 데스크톱 및 웹 타겟에서는 포커스된 뷰의 `OnKeyDown`이 보조 키 단독(`app.Key("shift")`)을 포함해 `app.Key`를 가장 먼저 받으며, 여기서 소비된 키는 바인딩에 도달하지 않습니다. `app.Key`는 완전한 한 번의 키 입력이므로 이어서 뷰의 `OnKeyUp`이 키가 올라오는 이벤트를 받습니다. `app.KeyDown`과 `app.KeyUp`은 조합 키가 아닌 하나의 키를 따로 누르고 떼며, `KeyDown`으로 누르고 있는 보조 키는 `KeyUp`이 호출될 때까지 이후 모든 키에 결합됩니다. 따라서 `app.KeyDown("shift")` 후 `app.Key("a")`를 호출하면 Shift+A가 입력되고, 누르고 있지 않은 키에 대해 `KeyUp`을 호출하면 스텝이 실패합니다. 테스트가 끝날 때 아직 눌려 있는 키는 그때 해제되며(`-target`에서는 플랫폼을 통해 해제), 눌림을 받았던 뷰가 올라옴 이벤트도 받게 되어 다음 테스트까지 눌린 상태로 남지 않습니다. `core.KeysDown`과 `KeyHeld`는 현재 눌려 있는 키를 읽습니다. `-target`의 watchOS에서는 이를 사용하는 테스트를 건너뜁니다. `app.MouseDown`, `app.MouseDownAt`, `app.MouseDownWith`, `app.MouseDownAtWith` 및 대응하는 `MouseUp` 메서드는 5개 마우스 버튼(`core.MouseButtonLeft`부터 `core.MouseButtonForward`까지) 중 하나를 누르고 떼며 `core.MouseButtonsDown`과 `core.MouseButtonHeld`를 갱신합니다. `app.BlurWindow()`는 창 포커스 상실을 시뮬레이션하고 눌려 있는 모든 키와 마우스 버튼을 초기화합니다.

`KeyDown`과 `KeyUp`은 Android의 네이티브 하드웨어 키 경로와 HarmonyOS의 Go 키 브리지도 구동합니다. Android와 명시적으로 포커스가 맞춰진 iPhone/iPad 뷰에서는 `OnKeyDown`이 바인딩과 네이티브 활성화보다 먼저 실행됩니다. 소비된 키 누름도 여전히 `OnKeyUp`을 수신하며, 키 해제는 원래 뷰에 유지되고, `core.KeysDown`은 네이티브 에디터 바깥에서 눌린 키를 추적합니다. Android의 비동기 소비 응답은 UI를 응답 가능한 상태로 유지하며, 포커스 상실, 종료(teardown), 또는 2초 후 응답 누락 시 네이티브 동작을 다시 실행하지 않고 대기 중인 키를 비웁니다. HarmonyOS는 보고된 키를 추적하고 Go 바인딩을 억제할 수 있지만, 호스트가 아직 네이티브 일반 뷰 포커스를 지원하지 않으며 비동기 브리지에서 반환된 후 ArkUI 동작을 억제할 수 없습니다.

`app.KeyCode("KeyW")`는 물리적 키 위치를 누르며, `KeyCodeDown`과 `KeyCodeUp`은 이를 따로 누르고 뗍니다. 헤드리스 테스트에서는 다른 키보드 레이아웃을 테스트하기 위해 `map[string]string{"KeyW": "z"}`처럼 `core.Environment.KeyCodeNames`를 설정하세요. 지정하지 않은 위치는 US 레이아웃을 사용합니다. `-target`에서는 스캔 코드나 키 코드 같은 플랫폼 자체 키 이벤트가 위치 정보를 전달하므로, 오른쪽 보조 키나 키패드 숫자가 왼쪽 키나 상단 숫자 행과 구별되어 읽힙니다. macOS, Windows, Linux, 웹에서는 플랫폼의 현재 키보드 레이아웃이 키 이름을 결정하므로 AZERTY에서 `KeyW`는 `z`가 됩니다. iOS, Android, HarmonyOS는 해당 위치의 US 레이아웃 키를 보고합니다. `KeyDown`과 `KeyCodeDown` 양쪽 모두로 눌린 보조 키는 양쪽 모두에서 해제될 때까지 눌린 상태를 유지합니다. watchOS처럼 위치 기반 키 입력을 할 수 없는 타겟은 테스트를 건너뜁니다.

`-target`에서 `app.Select(selector, start, end)`는 네이티브 필드의 선택 영역을 이동합니다. 오프셋은 UTF-16 단위이며 텍스트 길이에 맞춰 잘리고, 시작과 끝이 뒤집힌 경우 순서가 정렬됩니다. iOS와 Android는 네이티브 텍스트 입력 API를 사용하고, HarmonyOS는 필드의 컨트롤러를 사용하며 선택 콜백을 기다립니다. `OnSelectionChange`는 정상적으로 실행되며, 테스트 모델은 해당 콜백이 없더라도 실제 범위를 읽습니다. macOS, Windows, Linux, 웹은 전체 선택과 편집 방향키를 사용하므로 해당 키를 가로채는 바인딩이나 애플리케이션 메뉴 항목이 있으면 이 스텝에서 키를 빼앗아 갑니다. 이후의 타이핑과 네이티브 클립보드 편집은 선택된 범위에 적용됩니다. HarmonyOS의 테스트 타이핑은 보고된 네이티브 범위를 교체하며, 해당 테스트 호스트는 아직 네이티브 클립보드 키를 구동하지 않습니다.

헤드리스 Linux 및 Android 테스트는 `app.Installs`에서 인스톨러에 전달된 패키지 경로를 확인할 수 있습니다. 시뮬레이션된 인스톨러에서 오류를 반환하게 하려면 `app.InstallPackage`를 설정하세요.

`app.DoublePress(selector)`는 플랫폼이 더블 클릭이나 더블 탭으로 인식할 수 있도록 한 스텝에서 두 번의 누르기를 보냅니다. `app.Pan`은 타겟의 드래그 입력을 사용하며, 각 오프셋은 시작점 기준입니다. `app.Pinch`는 헤드리스에서 동작하며 macOS, iOS 시뮬레이터, Android, 웹에서 플랫폼 제스처 경로를 구동합니다. iOS 시뮬레이터에서는 각 배율이 두 터치 사이의 거리를 설정하며, UIKit의 인식 임계값을 넘으면 보고가 시작됩니다. Android에서는 먼저 네이티브 인식기를 시작할 만큼 두 터치가 충분히 벌어진 다음 요청된 배율들을 거쳐 이동합니다. 핀치 입력이 없는 타겟에서는 `Pinch`에 도달하면 나머지 테스트를 건너뛰고 어떤 타겟이 지원하지 않는지 보고합니다.

`app.Orientation`은 활성화된 `Scope.RequestOrientation` 마스크를 보유합니다. 앱이 기본 방향 정책을 사용할 때는 0입니다.

`app.NowPlaying()`은 앱의 마지막 시스템 미디어 게시 정보를 반환하며, 지워진 후에는 nil을 반환합니다. `app.MediaCommand(core.MediaCommand{Kind: core.MediaSeek, Position: 30 * time.Second})`는 게시 정보를 통해 활성 핸들러로 명령을 보냅니다. 헤드리스에서는 `nsgotest.Environment(core.Environment{NowPlaying: true})`를 사용하거나 macOS 타겟에서 실행하세요.

## 창 없이 그리기

macOS에서는 `nsgotest.PlatformTextEngine()`이 헤드리스 테스트에 CoreText 메트릭을 제공합니다. 이를 `nsgotest.TextEngine`을 통해 전달하세요. 다른 환경에서는 nil을 반환하며 모델은 근사 메트릭을 유지합니다. 앱을 시작하기 전에 `nsgotest.RegisterFont`로 폰트 바이트를 등록하세요.

`nsgotest.Render(app.Find("id:score").Drawing, core.Size{Width: 600, Height: 800}, 2)`는 창을 열지 않고 기록된 캔버스를 소유된 `*image.RGBA`로 다시 그립니다. 이를 `image/png`로 인코딩하거나 픽셀을 비교하세요. 크기는 포인트 단위, 배율은 포인트당 픽셀 수이며 투명 출력은 최대 16 Mi 픽셀로 제한됩니다. 두 값 모두 유한한 양수여야 합니다. 이미지는 30초 제한 시간 내에 동기적으로 로드되며, 애니메이션 이미지는 첫 번째 프레임을 사용하고 실패 시 오류를 반환합니다. 렌더링 전에 애니메이션 슬롯과 뷰 페인트를 해석(resolve)하세요.

창 없는 렌더링과 폰트 등록은 현재 macOS만 지원합니다. 다른 타겟은 `nsgotest.ErrNoPlayer`를 반환하며, 네이티브 타겟의 `Screenshot`은 이와 별개로 계속 사용할 수 있습니다.

`View.Style`은 `nsgotest.ViewStyle`을 기록하고, `View.Animated`는 `nsgotest.AnimatedValues`를 기록합니다. `app.Animated(selector)`는 바인딩된 값을 기록된 기본 스타일에 적용합니다. 이펙트와 커스텀 커서는 앱 값의 정체성(identity)을 유지하며, `Radii`, `Rounded`, `Rim`, `Matrix`는 동일한 지오메트리 정책을 유지합니다.

`app.Find(selector).Kind`는 `nsgotest.ViewKind`입니다. 테스트에서 마운트된 뷰 형태를 검사해야 할 때는 `nsgotest.KindText`, `KindInput`, `KindCanvas`, `KindGPU` 같은 상수와 비교하세요.

헤드리스 GPU 테스트에서는 `app.Surface("title:Inspector").GPUReadPixels("id:viewport")`를 사용해 특정 창의 소프트웨어 GPU 픽셀을 읽을 수 있습니다. `Surface.TriggerGPUDeviceLost`와 `Surface.RestoreGPUDevice`는 해당 창의 픽셀 비율을 사용해 그 창에서의 디바이스 손실과 복구를 시뮬레이션합니다. 창마다 뷰 ID가 중복되더라도 디바이스와 리드백(readback) 콜백은 분리된 상태로 유지되며, `app.GPUFrames()`는 앱 전체의 제출 내역을 기록합니다. 이 헬퍼들은 헤드리스 소프트웨어 GPU 모델에서 동작합니다.

`nsgotest.UpdaterState(core.UpdaterState{...})`는 헤드리스와 네이티브 타겟 모두에서 첫 번째 렌더 전에 업데이터 기능과 기본 설정을 초기화합니다. 각 확인 결과를 제공하려면 `app.Update`를 설정하세요. 기본 설정 변경은 업데이트 피드에 접속하지 않고 모델링됩니다.

헤드리스 테스트는 `app.EditMenu`와 `app.EditMenuItems()`를 검사하고, `app.ChooseEditAction(core.EditCopy)`로 제공된 동작을 선택하며, `app.CloseEditMenu()`로 닫습니다. 붙여넣기는 테스트의 클립보드를 사용합니다. `app.LongPressAt(selector, point)`는 특정 지점을 길게 누르며, 헤드리스의 `app.Touch(selector, point).Hold(duration)`는 손가락을 떼기 전에 롱 프레스를 발생시켜 이후 `Move`로 선택 영역 드래그(sweep)를 테스트할 수 있게 합니다. 메뉴 레코드와 동작 선택은 동작을 모델링하므로 시스템 툴바를 검증하려면 네이티브 인수 테스트를 사용하세요.

`View.Props`는 식별자, 접근성 이름과 상태, 동작, 포커스 및 포인터 설정에 대한 `nsgotest.ViewProps` 스냅샷입니다. `HelpText`, `HasAction`, `ActionList`, `FocusStop`이 관련 정책을 노출하며, 레이블 ID, 동작, 리플 설정은 마운트된 값에서 복사됩니다.

## 위젯과 Live Activities

`nsgotest.Widgets(true)`는 앱이 `widget.Update`로 게시하는 내용을 기록합니다. `app.Widgets()`로 각 종류의 최신 타임라인을 읽으세요. 날짜가 없는 항목에는 앱의 월 타임(wall time)이 적용됩니다. `widget.Available()`을 false로 만들고 업데이트가 `widget.ErrNoWidgets`를 반환하게 하려면 `nsgotest.Widgets(false)`를 사용하세요.

`nsgotest.LiveActivities(true)`는 시작, 업데이트, 알림, 종료를 기록합니다. `app.LiveActivities()`는 콘텐츠 변경 내역 및 최종 콘텐츠와 함께 활동을 시작 순서대로 반환합니다. 시작 시 `widget.ErrActivitiesDisabled`를 반환하게 하려면 `nsgotest.LiveActivities(false)`를 사용하세요. 이 옵션들은 네이티브 타겟을 포함해 테스트 동안 프로세스 전체의 해당 서비스를 대체합니다. 이러한 서비스를 호출하는 테스트는 순차적으로 실행하세요.

헤드리스 테스트는 `app.ConnectGamepad(id, name)`, `app.SetGamepadInfo(id, family, vendorID, productID, canRumble)`, `app.DisconnectGamepad(id)`, `app.SetGamepadButton(id, btn, value)`, `app.SetGamepadAxis(id, axis, value)`로 게임 컨트롤러를 시뮬레이션합니다. 연결, 정보 업데이트, 연결 해제는 즉시 안정화되어 `gamepad.Pads`를 읽는 스코프를 다시 렌더링하며, 버튼과 축 업데이트는 `gamepad.State`를 위해 다음 프레임에 반영됩니다. `app.GamepadRumbles()`와 `app.LastGamepadRumble(id)`는 기록된 `gamepad.RumbleEvent` 시작 및 중지 명령을 검사합니다. `-target`에서는 이 테스트들을 건너뜁니다.

`app.FindMounted("id:tab-bar").TabBadge(index)`는 헤드리스와 타겟 테스트에서 네이티브 탭의 텍스트 또는 점(dot) 배지를 반환합니다. 직접 그려지는 `ui.TabShell` 탭에서는 `Props.Value.Text`가 배지로 안내되는 값입니다.

## 테스트 실행

```sh
go tool nsgo test
```

`test`는 테스트당 한 줄씩 출력합니다. `-run`은 `go test`처럼 `-run 'TestGallery/dark-home'` 같은 서브테스트를 포함해 테스트를 선택합니다. 타겟에서는 `-timeout`이 테스트당 제한 시간을 설정하며(기본값 3분, `0`은 비활성화), 해당 제한 시간에 도달하면 결과에 보고합니다. 시뮬레이터나 폰에서 `TestMain`이 `nsgotest.Main`을 호출하는 테스트는 테스트 호스트가 시작될 때 제한 시간이 시작되므로 앱 실행이 느리더라도 제한 시간에 포함되지 않습니다. 실행 자체에는 5분의 별도 제한이 있으며 이에 도달하면 실행 타임아웃으로 테스트가 실패합니다. 모든 서브테스트가 건너뛰어진 테스트는 건너뜀(skipped)으로 집계됩니다. `-junit report.xml`은 각 건너뜀 사유를 유지한 채 JUnit 리포트를 작성하고, `-trace`는 각 스텝의 마운트 항목과 커밋 시간을 출력하며, `-bench`는 대신 벤치마크를 결과당 한 줄씩 실행합니다. 헤드리스 실행은 `--` 뒤의 플래그를 `go test`로 전달하며, 타겟 실행은 아래의 런타임 플래그를 받습니다. 헤드리스 실행에서 `-- -c`로 테스트를 컴파일만 하는 경우에도 컴파일 오류는 콘솔과 JUnit 리포트에 표시됩니다.

라이브러리나 모듈 루트처럼 `nsgo.yaml`이 없는 디렉터리를 지정하면 `test`는 그 아래의 모든 패키지를 대상으로 삼습니다. Go의 패턴처럼 디렉터리 끝이 `/...`로 끝날 수도 있습니다. 디렉터리는 하나만 받을 수 있으며 두 번째 디렉터리가 오면 오류를 내고 멈추므로, 여러 디렉터리를 테스트하려면 상위 디렉터리를 지정하거나 각각 따로 실행하세요. `go tool nsgo check`도 같은 방식으로 패키지를 받아 검사합니다.

테스트는 반응형 그래프 소유권을 검사합니다. 시그널과 스코프에는 앱의 고루틴에서 접근하고, 워커에서는 `cx.Post`를 사용하세요. 앱이 유휴 상태일 때 다른 고루틴에서 호출이 들어오면 `Scope.Post` 진단 메시지와 함께 패닉이 발생하며, 나머지는 `-race`가 잡아냅니다. 내부 엔진 패닉도 테스트를 실패시킵니다. 배포된 앱에서는 영향을 받은 엔진만 중지하고 스택을 호스트에 보고합니다. 개발 빌드도 소유권을 검사하며 릴리스 빌드는 이 검사를 생략합니다. WebAssembly 개발 및 테스트 빌드는 브라우저 콜백이 브라우저가 관리하는 고루틴에서 재개될 수 있으므로 고루틴 ID 단언문을 생략하며, 다른 테스트 검사는 거기서도 계속 실행됩니다.

`app.Post(func() { state.Set(value) })`는 다음 스텝이나 대기가 안정화될 때 앱의 그래프 고루틴에서 실행될 작업을 큐에 넣습니다(헤드리스에서는 테스트 고루틴, 타겟에서는 엔진 고루틴). 테스트나 스텁 워커에서 이를 호출한 뒤 `app.Settle()`이나 대기를 사용해 결과 뷰를 읽으세요. 이 함수는 추가 작업을 다시 포스트할 수 있으며 같은 스텝에서 실행됩니다. 타겟에서는 테스트 고루틴 바깥에서 실행되므로 스텝을 실행하거나 `t.Fatal`을 호출하지 마세요.

헤드리스에서 `cx.ReadAsset`은 프로젝트의 `resources/` 디렉터리를 읽습니다. `-target`에서는 테스트용으로 패키징된 리소스를 읽습니다.

그런 다음 각 스텝은 최대 2초 동안 한 프레임씩 시계를 진행해 끝나는 애니메이션이 모두 종료되고 각 스타일 전환(transition)이 `OnTransitionEnd`를 실행할 때까지 기다립니다(타겟에서는 플랫폼이 자체 애니메이션을 실행하므로 전환에 대해서만 적용). 따라서 `EndingStyle`을 통해 닫히는 대화상자나 팝오버는 `app.Press`가 반환될 때 이미 사라져 있습니다. 진행 도중의 모습을 확인하려면 `nsgotest.ManualFrames()`로 시작하세요. 그러면 `app.Advance`가 시계를 움직일 때만 진행됩니다.

## 시간

헤드리스와 타겟 모두에서 앱은 `app.Clock` 위에서 실행됩니다. 이는 `2026-01-01 00:00 UTC`에서 시작해 테스트가 움직일 때만 진행되는 `core.ManualClock`입니다. `app.Advance(d)`는 시계를 `d`만큼 진행시켜 만기가 도래한 각 타이머를 실행하고, 앱이 타이머를 기다리는 동안 실제 디스플레이처럼 프레임 간격마다 디스플레이 프레임을 실행합니다. `app.FastForward(d)`는 노트북을 잠시 닫았다가 연 것처럼 한 번에 앞으로 건너뛰며, 그사이에 만기가 도래한 각 타이머를 마지막에 한 번 실행하고 디스플레이 프레임도 한 번 실행합니다. 헤드리스 테스트에서 시뮬레이션되는 디스플레이 주사율에서 콜백은 표면의 `SetPreferredFrameRate` 설정도 준수합니다.

앱은 이 시계를 `cx.Now()`로, 표시하는 날짜를 `cx.WallTime()`으로 읽으므로 제스처 속도나 더블 탭 시간 창처럼 앱이 직접 측정하는 시간에는 이 함수들을 사용하세요. `time.Now()`와 `time.Sleep`은 `Advance`가 움직이지 않는 시스템 시계를 따르므로, `time.Now()`로 시간을 재는 앱은 테스트가 진행한 시간을 놓치고 테스트 안의 `time.Sleep`은 테스트만 느리게 만듭니다. 응답 후 호출한 `cx.Post`처럼 다른 고루틴에서 도착하는 작업을 기다릴 때는 타임아웃이 실제 시간을 기준으로 동작하는 `app.WaitFor`, `app.WaitForGone` 또는 `app.WaitUntil`을 사용하세요.

헤드리스에서 이러한 대기 메서드는 확인 사이에 시계를 한 프레임씩 진행시킵니다. 대기 중에 시계를 멈춰 두려면 `nsgotest.ManualFrames()`로 시작하세요. 타겟에서는 플랫폼이 자체 애니메이션을 실행하는 동안 대기가 앱을 안정화하며, 스텝과 마찬가지로 스타일 전환이 `OnTransitionEnd`를 기다리는 동안에만 확인 사이에 시계를 움직입니다.

오늘 마감 항목처럼 날짜에 의존하는 화면은 `nsgotest.WallTime(t0)`으로 시작하세요. 첫 번째 렌더의 `cx.WallTime()`은 해당 시간대의 `t0`가 되며, 타이머는 계속 시계 자체의 시간으로 동작합니다. `nsgotest.Clock(c)`는 테스트가 직접 만든 시계 위에서 앱을 실행해 앱 외부 코드와 시계를 공유하게 하며, `c.Advance`가 시계를 움직일 때 앱의 타이머가 발동합니다. `nsgotest.Measure()`로 시작한 타겟 측정은 아래 설명처럼 대신 시스템 시계 위에서 앱을 실행합니다.

스텝이 제공하지 않는 헤드리스 플랫폼 보고를 위해서는 `app.Report(nsgotest.Event{Kind: nsgotest.EventPress, ID: view.ID})`를 호출하세요. 이 보고는 슬라이스, 내비게이션 상태, 선택적인 `nsgotest.TouchGeometry` 스냅샷을 복사하며, `Data`는 테스트의 네이티브 페이로드를 유지합니다.

## 플랫폼에서 실행

테스트는 플랫폼 자체의 입력을 사용하는 실제 창에서 실행됩니다. 테스트 탐색은 이식 가능한 패키지 안의 타겟 전용 테스트 파일을 포함해 타겟의 빌드 태그를 따릅니다:

```sh
go tool nsgo test -target macos
```

macOS에서는 테스트 창이 테스트 호스트 외부의 마우스, 키보드, 트랙패드 입력을 무시하므로 테스트 스위트가 실행되는 동안 머신을 사용해도 테스트 화면이 바뀌지 않습니다. 테스트 자체의 스텝은 계속 정상 동작합니다.

테스트 창은 앱을 활성화하지 않고도 테스트의 키를 받으므로 작업 중인 앱이 계속 전면에 유지됩니다. 다른 앱이 활성화된 상태에서는 AppKit이 필드 클릭을 거부할 수 있으며, `Focus`와 `Type`이 이러한 거부를 진단합니다. `go tool nsgo test -target macos -activate`를 사용하면 테스트 앱이 필드를 클릭하거나 키를 누르기 전에 활성화되어 키보드를 가져올 수 있습니다. 이 옵션을 사용하려면 잠금 해제된 데스크톱 세션과 `github.com/nsgo-dev/nsgo`를 임포트하고(빈 임포트 `_`로 충분) `TestMain`에서 `nsgotest.Main` 또는 `nsgotest.MainFunc`를 시작하는 테스트가 필요합니다.

정리(cleanup) 단계에서는 열려 남은 네이티브 컨텍스트 메뉴를 닫아 다음 서브테스트의 첫 클릭이나 키 입력을 가로채지 못하게 합니다. macOS에서 WindowServer 캡처는 창 경계에 맞춰 잘린 상태로 테스트 창의 열린 네이티브 팝업을 포함하며, 다른 창은 제외합니다.

네이티브 `WebView` 내부에서 신뢰할 수 있는(trusted) 이벤트를 보내려면 `Env.CanSendInput`을 확인하고 앱 그래프에서 입력을 보내세요: `app.Post(func() { page.SendInput(core.WebClick(12, 16)) })`. 클릭하기 전에 페이지가 준비 완료를 알릴 때까지 기다리고, `core.WebText`로 입력하기 전에 포커스를 맞추세요. Linux에는 X11과 XTest가 필요하며, 테스트 호스트는 물리적 입력은 계속 거부하면서 이 스크립트된 이벤트는 허용합니다.

타겟은 `macos`, `windows`, `linux`, `web`(Chrome, Chromium 또는 Edge), `ios`(연결된 iPhone에서 실행하며 여러 대일 경우 `-device`로 지정, Xcode가 마지막으로 사용한 팀 또는 `-team`으로 서명), `ios_simulator`, `tvos_simulator`(리모컨 방향키, 선택 버튼으로 Return, Menu로 Escape 사용), `watchos_simulator`, `android` 또는 `all`입니다. 각 테스트는 자체 프로세스에서 실행됩니다. `app.Capture()`는 현재 픽셀을 `image.Image`로 반환하고 `app.CapturePNG()`는 PNG 바이트를 반환하며, 둘 다 헤드리스에서는 테스트를 건너뜁니다. macOS에서는 Core Animation의 그림자, 마스크, 필터, 애니메이션, 캐럿까지 포함해 WindowServer가 합성한 그대로 창을 캡처합니다. 창이 최소화되었거나 다른 Space에 있는 등 창을 캡처할 수 없을 때는 대신 AppKit을 통해 뷰가 스스로를 그리므로 이러한 효과가 빠지며, 처음 발생할 때 이를 로그로 남깁니다. NSGo는 인코딩 전에 두 macOS 캡처 경로 모두를 sRGB로 변환합니다. iOS와 tvOS도 sRGB로 캡처하므로 이 타겟들에서의 픽셀 값은 디스플레이가 Display P3를 사용하는지 여부에 의존하지 않습니다. Android는 셰이더 서피스를 포함해 렌더링된 창을 캡처합니다. `app.Screenshot("en/library")`는 스크린샷을 `.nsgo/test/screenshots/<target>/en/library.png`(예: `macos`에서는 `.nsgo/test/screenshots/macos/en/library.png`)로 저장합니다. 이름에 공백을 포함할 수는 있지만 절대 경로나 `..`는 사용할 수 없습니다. `-screenshots dir`은 작업 디렉터리 기준 상대 경로인 `dir` 아래에 저장합니다. 기기 로그로 인해 일부가 누락되거나 잘린 스크린샷은 파일이 쓰이기 전에 테스트를 실패시키고, 이전 테스트가 이미 사용한 경로에 저장하려는 스크린샷도 테스트를 실패시킵니다. `-fps`는 `macos`, `windows`, `linux`, `ios_simulator`, `web`에서 매초 성능 모니터 측정값을 로그로 남기며, `android`에서는 엔진 측정값만 기록합니다. `-env KEY=VALUE`는 폰 테스트에서 다른 방법으로는 전달할 수 없는 테스트 환경 변수를 설정합니다. 시뮬레이터나 폰에서 `nsgo.yaml`이 없는 디렉터리의 테스트는 디렉터리 이름 뒤에 따라 앱으로 설치되며, `dev.nsgo.` 뒤에 이름의 영문자와 숫자만 붙인 ID를 가집니다(예: `base/formula`는 `dev.nsgo.formula`). `android`와 시뮬레이터에서는 테스트가 앱의 ID 뒤에 `.nsgotest`를 붙인 ID(App Clip도 동일)로 기존에 설치된 앱 빌드를 덮어쓰지 않고 나란히 설치되며, 실행이 끝나면 제거됩니다. `ios`에서는 프로비저닝 프로파일에 명시된 앱 자체 ID로 설치되어 iPhone에 설치된 기존 빌드와 그 데이터를 대체합니다.

실물 iPhone 타겟 테스트는 테스트 호스트가 시작된 후의 Go 패닉 진단을 포함해 앱의 stderr를 보존합니다.

`android`와 `ios`에서 테스트는 NSGo의 테스트 호스트 안에서만 실행됩니다. Android 런타임은 테스트 프로세스의 표준 출력에서 프로토콜을 읽으며, 도구는 호스트가 출력하는 표식을 통해 테스트의 출력과 종료 코드를 파악합니다. 이 타겟들에서 실행될 테스트가 있는 패키지는 테스트 파일에서 `github.com/nsgo-dev/nsgo`를 임포트해(빈 임포트로 충분) 호스트를 링크하고 `TestMain`에서 `nsgotest.Main`이나 `nsgotest.MainFunc`로 시작해야 합니다. 빌드하거나 설치하기 전에 `nsgo test`는 호스트가 링크되어 있고 `TestMain`이 선언되어 있는지 확인합니다. 해당 함수가 호스트를 우회하면 Android는 `nsgotest.Main` 힌트와 함께 잘못된 stdout 프로토콜 오류를 보고하므로, 호스트가 시작될 때까지 로그와 설정 출력은 stderr로 보내세요. `ui/chat` 같은 라이브러리 패키지는 테스트 파일에 이 두 가지를 추가하면 폰에서도 실행되며, 그대로 두어도 헤드리스와 다른 타겟에서는 실행됩니다.

플랫폼 백엔드를 링크하지 않는 패키지는 타겟의 런타임에서 헤드리스로 실행됩니다. `-target web`을 사용하면 라이브러리 컨트롤을 이런 방식으로 브라우저 안에서 테스트할 수 있습니다. 브라우저는 파일 시스템이 없으므로 저장소는 메모리에 유지되고 `DataDir`, `CacheDir`, `LogsDir`은 비어 있습니다. 파일 기반인 `nsgotest.MicrophoneInput` 페이크는 실제 WAV 파일이 필요하므로 여기서는 건너뜁니다.

결과를 보고하기 전에 패닉이 발생하거나 종료된 Android 테스트는 프로세스의 종료 상태와 패닉 스택을 포함한 캡처된 출력으로 실패 처리됩니다. 호스트를 시작하지 않고 종료된 `TestMain`도 종료 상태가 0이더라도 마찬가지로 실패합니다. Android 타겟 테스트가 제한 시간에 도달하면 호스트가 시작 표식을 출력했는지 여부와 러너가 지우기 전의 해당 테스트 `logcat` 출력 끝부분도 결과에 함께 포함됩니다.

`web`에서는 도구가 찾은 헤드리스 Chrome, Chromium 또는 Edge에서, 혹은 `NSGO_BROWSER`가 지정한 브라우저 실행 파일에서 테스트가 실행됩니다. macOS에서는 Chrome for Testing이 설치되어 있으면 이를 우선 사용합니다. 도구는 브라우저 시작을 최대 30초까지 기다리며, 명령이 취소되면 시작 대기를 중단하고 브라우저가 종료되거나 응답하지 않으면 브라우저의 오류 출력을 보고합니다.

각 스텝과 `app.Settle()`은 배치나 유한 애니메이션이 없는 100ms 구간을 세기 전에 엔진의 대기 중인 커밋을 기다립니다. 앱이 계속 변하더라도 전체 대기는 5초 후에 멈추므로, 느리게 열리는 첫 대화상자에도 테스트에 별도의 포스트나 슬립이 필요하지 않습니다.

Android에서는 빌드 전과 각 테스트 전에 도구가 잠든 화면을 깨우고 자격 증명이 없는 잠금 화면을 해제합니다. PIN, 비밀번호 또는 패턴으로 잠긴 기기는 잠금을 해제해야 실행을 계속할 수 있으며, 오류 메시지에 기기 이름이 표시됩니다. 일시적인 `adb` 상태 읽기 실패는 재시도하며, 계속 연결할 수 없는 기기는 남은 모든 테스트를 하나씩 실패시키는 대신 타겟 실행을 한 번에 중단합니다.

검증이 아니라 측정을 하려면 `nsgotest.Measure()`로 시작하세요. 앱이 시스템 시계에서 실행되며, `Advance`는 이동시킨 시간만큼 실제로 기다리고, 스텝은 애니메이션 중인 요소가 멈출 때까지만 기다립니다. `app.Mark("name")`은 그 지점부터 측정되는 구간에 이름을 붙이고, `app.Mark("-")`는 이를 종료합니다. 마크는 `-fps` 지정 여부와 관계없이 출력됩니다.

NSGo 플래그는 디렉터리 뒤에 두지 말고 `go tool nsgo test -target web ./app`처럼 디렉터리 앞에 두세요. 헤드리스 실행에서는 `--` 뒤의 플래그가 `go test`로 전달됩니다. 타겟 실행에서는 `--` 뒤에 `-short`, `-count`, `-cpu`, `-parallel`, `-failfast`, `-fullpath`를 지원하며(예: `go tool nsgo test -target web ./app -- -short -count=3`), `-run`이나 `-timeout`을 포함해 디렉터리 앞에 와야 하는 다른 Go 테스트 플래그는 명시적으로 거부합니다. `-count=0`은 테스트를 실행하지 않습니다. NSGo의 네이티브 호스트를 링크하는 테스트는 count와 CPU 설정마다 새 프로세스에서 반복 실행되며 결과에 `[run N]`, 스크린샷은 `run-N` 하위 디렉터리에 저장되고 호출마다 제한 시간이 적용됩니다. 라이브러리는 Go의 프로세스 내 반복 실행을 유지합니다.

## 미디어 리포트

`app.Play(selector, core.VideoInfo{Duration: time.Second, RateChangeable: true})`는 재생 길이, 원본 크기 및 재생 속도 변경 가능 여부와 함께 비디오 로드를 보고합니다. 속도 제어 기능을 사용할 수 없는 플레이어를 테스트하려면 `RateChangeable`을 false로 설정하세요.

`tui`에서 테스트를 실행하려면 대화형 터미널, 링크된 NSGo 백엔드, 그리고 `nsgotest.Main`을 호출하는 `TestMain`이 필요합니다. 각 프로세스는 하나의 애플리케이션을 열며, 스텝은 터미널 편집, 포커스, 마우스 및 키 라우팅을 테스트합니다. 테스트 출력은 터미널이 복원될 때까지 버퍼링됩니다. PNG 캡처는 포터블 고정 글꼴로 셀을 렌더링합니다. [터미널 타겟](building.md#터미널)

## TSX 테스트

`nsgo/test`로 작성된 TSX 앱의 `*.test.tsx` 파일은 Go 테스트와 함께 실행됩니다.

`open("App.tsx", { clock: {} })`는 수동 시계에서 앱을 실행하며 `app.clock`으로 이를 움직입니다. `app.clock.runFor(ms)`는 `app.wait(ms)`처럼 `Advance`와 같이 한 프레임씩 진행시키고, `app.clock.fastForward(ms)`는 `FastForward`처럼 앞으로 건너뜁니다. `clock: { time }`은 `Date.now()`와 `new Date()`가 읽는 시작 시각을 `Date` 또는 1970년 이후 밀리초인 `time`으로 설정하고, `app.clock.setSystemTime(time)`은 이후에 이를 설정하며, 둘 다 타이머를 진행시키지는 않습니다. `clock`을 지정하지 않으면 헤드리스 앱의 시간은 기기에서처럼 `waitFor`와 `waitUntil`이 기다리는 동안 한 프레임씩 흐릅니다.

`open("App.tsx", { camera: true })`는 테스트 환경에 카메라를 제공해 `Picker.canTakePhoto()`가 `true`로 resolve되도록 하고 `Picker.takePhoto` 호출이 `app.picks()`에(`camera: true`와 함께) 기록되며 `app.onPick`으로 응답할 수 있게 합니다. 촬영된 사진으로 `PickFile` 또는 `PickFile[]`를 반환하거나, 취소하려면 `null` 또는 `[]`를 반환하고, `Picker.takePhoto`를 거부(reject)하려면 오류를 throw하세요. `camera: true`가 없으면 `Picker.canTakePhoto()`는 `false`이고 `Picker.takePhoto()`는 거부됩니다.

테스트 파일 없이 TSX 앱을 스크립팅하려면 엔트리 파일을 전달하고 표준 입력으로 `*.test.tsx` 테스트 본문 형식으로 작성된 스텝(`app`이 앱 인스턴스)을 넘기세요:

```sh
go tool nsgo test App.tsx < steps.ts
```

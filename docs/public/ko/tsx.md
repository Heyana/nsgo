---
title: TSX 앱
description: Node 없이 사전 컴파일되는 Tailwind 클래스 기반 TSX로 앱을 작성합니다.
group: 핵심 개념
order: 63
---

## TSX 앱

TSX 앱은 소스를 임베드하고 진입점을 실행하는 Go 패키지입니다:

```go
//go:embed *.tsx nsgo.yaml
var src embed.FS

var Root = compiler.App(src, "App.tsx")

func main() { nsgo.Run(Root) }
```

```tsx
import { mount, signal, Text, View } from "nsgo";

function App() {
  const count = signal(0);
  return (
    <View class="gap-3 p-6" onPress={() => count.update((n) => n + 1)}>
      <Text class="text-lg font-bold">Pressed {count()} times</Text>
    </View>
  );
}

mount(App);
```

`go tool nsgo init -template tsx`를 실행하면 기본 프로젝트가 생성되며, `examples/tsx`도 또 다른 예시입니다. `nsgo dev`와 `nsgo build`는 TSX를 사전에 컴파일하므로 패키지에는 컴파일러가 포함되지 않으며, `go run`과 `go test`는 앱이 시작될 때 컴파일합니다. 어느 쪽이든 Node는 필요하지 않습니다.

## 컴포넌트와 모듈

TSX는 `nsgo`에서 기본 프리미티브를, `nsgo/base`에서 `base`의 컴포넌트를, `nsgo/gamepad`에서 게임 컨트롤러(서명된 패키지에서는 `gamepad` 권한 아래)를, `nsgo/audio`에서 사운드(`audio` 아래)를 임포트하며, 앱이 `tsx/ui`를 임포트하면 `nsgo/ui`에서 `ui`의 컴포넌트도 임포트할 수 있습니다. `examples/gallery-tsx`에서 이를 확인할 수 있습니다.

Mermaid 다이어그램을 사용하려면 Go 호스트에서 부수 효과를 위해 `tsx/textview` 또는 `tsx/ui`와 함께 `github.com/nsgo-dev/nsgo/tsx/diagram`을 임포트하세요. 이렇게 하면 다이어그램 파서를 사용하지 않는 앱에는 링크하지 않으면서도 `nsgo/base`에 `Diagram`이 추가됩니다. `Diagram`은 `source`, `fontSize`, 부분적인 `theme`, `accessibilityLabel` 및 일반 스타일링 prop을 받습니다. 직접 사용하거나 TextView 렌더러로 전달하세요:

```tsx
import { Diagram, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderDiagram={(code) => <Diagram source={code.code} errorClass="font-mono" />} />
```

동일한 `renderDiagram` prop이 `nsgo/ui`의 TextView에서도 동작합니다. Mermaid가 아닌 코드 블록은 기존 렌더러를 그대로 유지하며, 유효하지 않은 다이어그램은 소스를 그대로 표시합니다.

조판된 수식을 사용하려면 Go 호스트에서 부수 효과를 위해 `tsx/textview` 또는 `tsx/ui`와 함께 `github.com/nsgo-dev/nsgo/tsx/formula`를 임포트하세요. 이렇게 하면 `nsgo/base`에 `Formula`가 추가되며, 이 패키지를 생략한 앱의 바이너리에는 수식 엔진과 폰트가 포함되지 않습니다. `Formula`는 `source`, `dialect`(기본값은 `tex`, 또는 `typst`), `display`, `accessibilityLabel` 및 스타일링 prop을 받습니다. 폰트 크기와 색상은 주변 텍스트를 따릅니다. 직접 사용하거나 TextView 렌더러를 제공하세요:

```tsx
import { Formula, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderMath={(math) => <Formula source={math.tex} display={math.display} />} />
```

동일한 `renderMath`가 `nsgo/ui`의 TextView에서도 동작합니다. 수식을 복사할 때는 원래 TeX가 유지됩니다. Go의 `Renderer`는 여전히 호스트 설정으로 남습니다.

`UrlScheme.url()`은 추적된 애플리케이션 주소를 읽으며, `UrlScheme.setUrl(url, { history: "push" })`는 주소 항목을 추가하고 옵션을 생략하면 현재 항목을 교체합니다. `UrlScheme.on("change", fn)`은 브라우저 뒤로 가기, 앞으로 가기 및 해시 변경을 관찰하고 구독 해제 함수를 반환합니다. 라우팅, 동일 출처 검증 및 딥 링크 호스팅에 대해서는 [애플리케이션 주소](addresses.md)를 참고하세요.

## TSX에서 Go 호출하기

TSX에서 임포트할 수 있도록 Go 코드를 등록하세요:

```go
tsx.Register("app/native", map[string]core.Node{"Badge": Badge{}})
tsx.RegisterFunctions("app/native", map[string]any{
	"greet": func(name string) string { return "Hello, " + name },
})
```

그러면 `import { Badge, greet } from "app/native"`로 임포트해 `Badge`의 필드를 prop으로 전달하고 `greet`을 호출할 수 있습니다. `context.Context`를 인자로 받는 함수는 프로미스(Promise)가 되며, `tsx.RegisterClass`는 Go 타입을 TSX에서 `new`로 생성할 수 있는 클래스로 만듭니다.

`undefined`로 전달된 선택적 prop은 기본값을 사용합니다. 기본값이 해당 타입의 제로 값과 다른 게터(getter)를 가진 Go 컴포넌트를 등록할 때는 `tsx.Default("Max", float64(100))` 같은 옵션을 추가하세요. 반응형 prop이 undefined를 반환하는 동안에도 컴포넌트를 교체하지 않고 동일한 기본값이 적용됩니다.

문서화된 Go 기본값이 있는 데이터 필드도 생략할 수 있습니다. `Flow` 노드의 `width`와 `height`가 없으면 JSX 콘텐츠 크기를 측정해 사용하고, 엣지(edge)의 `width`가 생략되면 기본 선 두께를 사용하며, 할당량 행(quota row)의 `pace`가 생략되면 페이스 표시를 숨긴 상태로 둡니다.

Go 컴포넌트와 클래스를 등록하면 앱이 실행될 때 리플렉션으로 노출된(exported) 필드와 메서드를 찾습니다. 이 조회 때문에 Go 링커가 TSX에 등록된 것뿐만 아니라 앱 전체에서 아무것도 호출하지 않는 노출된 메서드를 제거하지 못하므로, `tsx`를 링크하면 심볼이 제거된(stripped) 데스크톱 바이너리 크기가 약 20 MB 늘어나며 앱이 링크하는 패키지가 많을수록 더 커집니다.

## 서명된 패키지

앱은 새로 배포된 자체 화면이나 플러그인처럼 다운로드한 TSX를 패키지 검증 후 실행할 수 있습니다. 패키지 코드가 접근할 수 있는 대상을 패키지의 `nsgo.yaml`에 선언하세요. 그 외의 모든 접근은 거부됩니다:

```yaml
capabilities:
  network:
    hosts: [api.example.com, "*.cdn.example.com"]
  open_url: true # or { schemes: [https] }
  host: [search, "player.*"] # the app's handlers it may call
  # webview: { hosts: [...], input: true, media: [microphone, camera] }, clipboard, picker, speech, text_to_speech, status_bar, notifications, biometrics, sign_in_with_apple, purchases, secrets, audio, gamepad, auth_session
```

루트 키는 오프라인으로 보관하고 그 키로 릴리스 키를 인증한 뒤, 해당 디렉터리에서 앱에 서명하세요. `go tool nsgo keygen`은 키 쌍을 출력하며 파일을 작성하지 않습니다. 루트 키용으로 한 번, 각 릴리스 키용으로 한 번씩 실행하세요. 공개 키는 플래그와 앱 코드에 넣고, 비밀 키는 저장소 밖에 두며 `NSGO_ROOT_KEY`와 `NSGO_RELEASE_KEY`를 통해서만 도구에 전달합니다:

```sh
go tool nsgo keygen # once for the root, once for the release key
NSGO_ROOT_KEY=<root secret> go tool nsgo certify -key <release public> -apps 'com.example.*'
NSGO_RELEASE_KEY=<release secret> go tool nsgo publish -cert release.cert plugin
go tool nsgo verify -root <root public> plugin/dist/Plugin-1.0.0.nsgopkg
```

앱은 코드에 작성된 루트 공개 키를 신뢰하고, 엔진 고루틴 밖에서 패키지를 검증한 뒤, 자체 뷰 옆에 샌드박스된 `tsx.View`로 표시합니다. 패키지는 뷰가 제공하는 호스트 채널을 통해서만 앱과 통신하며, 앱의 저장소와 시크릿 중 자체 할당된 영역만 사용합니다:

```go
trust, err := pkg.OpenTrust(filepath.Join(cx.DataDir(), "trust"))
trust.AddRoot(root)
p, err := trust.Verify(data) // refuses tampered, revoked and older builds
channel := core.NewHost()
channel.Handle("search", func(c *core.HostCall) { c.Resolve(results(c.Data)) })
view := tsx.View{Package: p, Sandbox: tsx.Sandbox{Host: channel}}
core.Catch{Try: view.Render, Fallback: failed}
trust.Accept(p) // once it ran, older builds are refused
```

`Sandbox.Grant`는 패키지가 선언한 것보다 적은 권한만 부여할 수 있고, `Sandbox.Limits`는 다른 시간 예산과 할당량을 지정하며, `Sandbox.Modules`는 NSGo 기본 제공 외의 Go 함수와 컴포넌트를 제공합니다. 패키지의 키보드 단축키, 뒤로 가기 핸들러, 내비게이션 스택은 키보드 포커스가 해당 뷰 안에 있을 때만 유효하며, 포커스를 직접 이동하는 동작(`focus()`, `autoFocus`, `Keyboard.dismiss`, `setAccessibilityFocus`)도 포커스가 이미 해당 뷰 안에 있거나 사용자가 뷰 안을 누른 것에 응답할 때만 동작합니다. 패키지는 부모가 허용한 공간만큼만 채우는 자체 뷰에 표시되며 그 이상을 넘지 않고 메뉴, 팝오버, 토스트를 포함해 영역 밖을 클리핑합니다. 스크롤 뷰 안에서는 별도의 뷰로 감싸야 콘텐츠 높이를 차지할 수 있습니다. 패키지의 다이얼로그, 알림(alert), 컨텍스트 메뉴 또는 접근성 안내(announcement)도 동일한 방식으로만 표시되며, 뒤로 가기 동작은 패키지 코드가 다이얼로그를 제거하지 않더라도 다이얼로그를 닫습니다. `View.Export`는 패키지의 진입 모듈이 내보내는(export) 컴포넌트 이름을 지정해 진입 모듈이 마운트하는 루트 대신 표시하며, 이때 진입 모듈의 `mount`는 컴포넌트를 렌더링하거나 실행하지 않습니다. 또한 각 뷰마다 복사되는 JSON 인코딩 가능 맵인 `View.Props`를 전달할 수 있어, 하나의 패키지가 동일한 권한과 제한 아래에서 각각 고유한 상태를 가진 여러 뷰를 제공할 수 있습니다. 내보낸 항목이 없으면 `tsx.ErrNoViewExport`로 패닉이 발생하고, 함수가 아니면 `tsx.ErrViewExportNotComponent`로 패닉이 발생합니다. 진입 모듈이 함수를 내보내는 패키지는 함수 플러그인입니다. `tsx.Load(cx, p, tsx.Sandbox{})`는 뷰 트리에 속한 `cx` 아래에서(없으면 `tsx.ErrNoTree`를 반환) 호출자의 고루틴에서 화면 표시 없이 동기적으로 이를 실행하거나 진입 모듈이 로드 중 던진 예외를 반환하며, `Plugin.Call`로 해당 함수들을 호출합니다. `go tool nsgo dev -limits`는 이러한 뷰와 동일한 제한 환경에서 앱을 실행하며, `go tool nsgo revoke`는 `Trust.UpdateRevocations`용 폐기된 릴리스 키 및 패키지 목록에 서명합니다. `examples/plugins`는 사용자에게 패키지의 권한 허용을 요청한 뒤 설치하고 실행합니다. 기본값이 64 MiB인 `Limits.Memory`는 배열, 문자열, 버퍼, 형식화 배열(typed array)에 대해 계측된 네이티브 저장소 예산을 제한합니다. 여기에는 `concat`, `flat`, `slice`, 배열 전개(spread), 문자열 연결, 템플릿 리터럴, `replace`, `replaceAll`, `TextEncoder`, `Blob` 복사가 포함됩니다. `-limits`의 `memory=<bytes>`로 예산을 변경할 수 있고 `memory=0`으로 끌 수 있습니다. 검사는 할당 전에 실행됩니다. Go가 회수 가능한 저장소를 확인한 후에도 예산을 초과하는 패키지는 `ERR_NSH_QUOTA_EXCEEDED`와 리소스 `memory` 오류로 중단됩니다. 일반 객체, 클로저, 스택, 작은 할당, 계측되지 않은 내장 객체 및 호스트 API도 메모리를 할당할 수 있습니다. 브라우저는 자체 힙을 유지합니다. `pkg.Build`를 사용하거나 `go tool nsgo publish -extra <file>`로 해당 파일의 바이트를 전달해 패키지를 직접 서명하는 앱은 `pkg.Contents.Extra`에 플러그인이 메뉴에 추가하는 항목 같은 자체 메타데이터를 넣을 수 있습니다. 이 데이터는 패키지와 함께 서명되며 `Verify`가 변경 없이 `p.Extra`로 반환하므로 앱이 패키지 코드를 실행하지 않고도 이를 파싱해 읽을 수 있습니다.

앱은 `nsgo keygen`이 출력하는 키를 사용해 `pkg.SignDocument`로 플러그인 카탈로그 같은 자체 JSON 문서에 서명하고 `Signed.Open`으로 검증할 수 있습니다. 각 문서 종류에는 `app:catalog-v1`처럼 버전이 포함된 고유한 `app:` 컨텍스트를 지정하세요. 문서는 다른 종류의 문서나 패키지, 인증서, 폐기 목록으로 통과될 수 없습니다. `Open`은 인증서나 폐기 목록 없이 전달받은 공개 키만 정확히 신뢰하며, 디코딩 대상 타입에 없는 본문 필드는 거부합니다. 봉투(envelope)에는 시간이나 시퀀스가 들어 있지 않으므로 오래된 문서도 최신 문서와 마찬가지로 검증됩니다. 따라서 본문에 버전이나 만료 시각을 넣어 직접 확인하세요.

네이티브 샌드박스에서는 `JSON.stringify`, reviver가 있는 `JSON.parse`, `Array.prototype.join`, `toString`, `toLocaleString`, `flat` 및 배열에 대한 `String()`이 1,000단계보다 깊어지면 catch 가능한 `RangeError`를 던집니다. 이때 배열의 `toString`과 그 안에서 호출되는 `join`이 각각 카운트되는 것처럼 서로 중첩된 호출이 모두 계산되며, 그보다 깊은 데이터는 호출을 멈추게 하거나 앱을 종료시킬 수 있습니다. 웹에서는 브라우저의 내장 구현을 사용합니다.

네이티브 타겟에서 `DefaultLimits.Stack`은 1,000프레임입니다. `Limits.Stack` 또는 `-limits stack=<frames>`를 설정해 최대 10,000프레임까지 다른 깊이를 선택할 수 있습니다. 0 이하의 Go 값이나 상한을 초과하는 입력에는 10,000이 적용됩니다. 웹에서는 브라우저의 스택 제한을 사용합니다.

네이티브 함수들이 서로를 호출하는 경우에도 JS 호출과 마찬가지로 `Limits.Stack`에 합산됩니다. 내장 함수인 게터, `toString` 또는 `valueOf`, 다른 메서드를 `exec`로 가지는 정규 표현식, 서로 내부에서 호출하는 `Reflect.apply`와 `Reflect.construct`, 바인딩된 함수나 프록시 체인은 제한을 넘기면 앱을 종료하는 대신 catch 가능한 `RangeError`를 던지며, 이보다 깊게 중첩된 플러그인 값은 Go로 변환하는 호출을 `RangeError`로 실패시킵니다. 이는 NSGo가 요구하는 goja 포크인 `github.com/nsgo-dev/goja`에서 제공되며, 앱의 `go.mod`를 변경하지 않아도 NSGo와 함께 자동으로 포함됩니다.

네이티브 타겟에서 `Limits.Call`은 타이머 콜백과 `await` 이후의 프로미스 후속 실행을 포함해 각 JavaScript 실행 조각(slice)을 제한합니다. 실행 조각이 중단되면 대기 중이던 `Plugin.Call`은 `tsx.ErrBudget`을 한 번 보고하고 남은 타이머를 중지하며 늦게 도착한 호스트 응답을 무시합니다. 중단된 조각에서 큐에 추가된 프로미스 리액션을 포함한 독립적인 호출들은 계속 실행됩니다. 다른 호출의 프로미스를 완료(settle)시키는 호출도 여전히 자체 결과나 오류를 보고합니다. 백트래킹 정규 표현식은 로드 예산과 호출 예산 중 더 짧은 값을 매칭 타임아웃으로 사용하며, 처리되지 않은 타임아웃 역시 `tsx.ErrBudget`이 됩니다.

`Limits.CallTotal`은 각 플러그인 호출과 샌드박스된 뷰 이벤트에 벽시계(wall-clock) 기준 마감 시간을 추가합니다. 여기에는 대기 시간이 포함되며, 호출이 완료된 후에도 해당 호출의 타이머, 인터벌, 프레임, 프로미스 후속 실행 및 호스트 응답을 유지합니다. 마감 시간에 도달하면 대기 중인 작업은 폐기되고 여전히 대기 중인 호출은 `tsx.ErrBudget`으로 실패합니다. 진입 모듈의 작업은 `Limits.Load`로 제한되며 `Load`가 0이면 `CallTotal`을 사용합니다. 단독 호스트 이벤트는 전달 시점부터 `CallTotal`을 적용받습니다. 대기 중인 호출의 마감 시간 핸들을 얻으려면 `Plugin.CallWithBudget`을 사용하세요. 호스트의 `Scope.Post` 안에서 마감 시간 전에 `budget.Extend(duration)`을 호출하면 해당 호출에만 추가 시간을 부여할 수 있습니다.

`Plugin.Interrupt`는 어느 고루틴에서나 런타임을 중지하고, 대기 중인 타이머를 폐기하며, 현재 및 이후 호출을 실패 처리합니다. `Plugin.Stopped()`와 `Plugin.Err()` 역시 어느 고루틴에서나 읽을 수 있습니다. `Err()`는 런타임이 실행 중일 때는 nil이며 중지되면 첫 번째 중지 사유를 반환합니다. 플러그인을 닫거나 소유자를 정상적으로 정리(dispose)하면 정리 작업을 실행한 뒤 `tsx.ErrPluginClosed`로 중지합니다. `tsx.Load`를 호출할 때 `Sandbox.OnStop`을 설정하면 대기 중인 호출이 실패한 후 엔진 고루틴에서 해당 사유를 한 번 전달받을 수 있습니다.

함수 플러그인을 로드할 때 `Sandbox.OnError`를 설정하면 대기 중인 `Plugin.Call` 밖에서 발생한 처리되지 않은 타이머, 애니메이션 프레임, `queueMicrotask` 및 `reportError` 실패를 전달받을 수 있습니다. 각 플러그인의 핸들러는 엔진 고루틴에서 실행되므로 호스트가 실패 원인을 식별하고 차단기(breaker)가 열릴 때 `Plugin.Interrupt`를 호출할 수 있습니다. 네이티브 실행 조각 예산 초과 중단도 호출이 이미 완료된 프로미스 작업을 포함해 `tsx.ErrBudget`을 보고합니다. 일반 콜백 오류는 플러그인을 계속 실행 상태로 둡니다. 로드 및 호출 결과 오류는 계속 호출자에게 반환되며, 뷰는 여전히 `core.Catch`를 사용합니다. 핸들러가 없으면 기존의 오류 경계(boundary)와 앱 리포트 동작이 그대로 유지됩니다.

진입 모듈이 실행되기 전에 엔진 고루틴에서 각 뷰의 `*tsx.ViewRuntime`을 받으려면 `Sandbox.OnViewRuntime`을 설정하세요. 어느 고루틴에서나 `Interrupt`를 호출해 작업을 중지하고 마지막으로 렌더링된 뷰를 그대로 남길 수 있습니다. `Stopped`와 `Err`는 종료 상태를 보고하며 정상 정리 시 `tsx.ErrViewClosed`를 반환합니다. `Sandbox.OnStop` 역시 뷰의 종료 사유를 한 번 보고합니다.

`Plugin.JSTime`과 `ViewRuntime.JSTime`은 현재 실행을 포함해 네이티브 런타임의 JavaScript 진입 구간에서 소비된 경과 시간을 보고합니다. 진입 구간 사이의 대기 시간과 등록된 동기 Go 함수 내부 시간은 제외됩니다. `ViewRuntime.EngineTime`에는 동기 Go 호출과 현재 실행 중인 작업을 포함해 해당 뷰가 소유한 반응형, 마운트, 커밋 및 레이아웃 작업 시간도 포함됩니다. 엔진 진입 사이의 대기 시간, 호스트나 다른 뷰에 속한 작업, 공유 커밋 부기(bookkeeping) 및 네이티브 그리기는 제외됩니다. 두 카운터 모두 어느 고루틴에서나 안전하게 읽을 수 있습니다. NSGo 외부의 브라우저 JavaScript 턴은 측정하거나 선점(preempt)할 수 없습니다. 웹에서는 두 카운터 모두 사용 불가로 보고되며 `Interrupt`는 호스트가 제어권을 되찾을 때 이후의 NSGo 콜백을 중지합니다.

웹에서는 아직 페이지를 패키지와 격리할 수 없습니다. 패키지 코드가 권한 범위를 넘어 신뢰 저장소를 포함한 페이지 자체의 네트워크, 클립보드, 저장소에 접근할 수 있기 때문입니다. 따라서 웹에서는 `tsx.View`와 `tsx.Load`가 `tsx.ErrUnisolated`로 패키지를 거부하며, `Sandbox.WebUnisolated`를 설정해 페이지와 동일한 권한으로 실행하는 경우에만 허용됩니다. 이 설정은 앱 자체의 권한을 맡길 수 있을 만큼 신뢰하는 패키지에서만 사용하세요.

## 에디터와 테스트

`.nsgo/tsconfig.json`을 확장(extend)하는 `tsconfig.json`을 두면 에디터에서 TSX 타입을 사용할 수 있으며, 이 파일은 `go tool nsgo dev`와 `go tool nsgo check`가 생성합니다. 앱 옆에 `nsgo/test`로 작성한 `*.test.tsx` 파일은 Go 테스트와 함께 실행됩니다([테스트](testing.md)).

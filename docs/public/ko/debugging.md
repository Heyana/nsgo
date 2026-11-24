---
title: 실행 및 디버깅
description: 핫 리로드로 모든 대상에서 앱을 실행하고 오류, 업데이트, 레이아웃, 속도 및 다른 언어를 확인합니다.
group: 툴체인
order: 20
---

## 실행

```sh
go tool nsgo dev
```

`dev`는 현재 머신 또는 `nsgo.yaml`의 `dev.target`에 지정된 대상을 위해 앱을 빌드하고 실행합니다. Go 파일을 저장할 때마다 앱을 다시 빌드해 재시작하며, 앱이 `go.mod`나 `go.sum`에 없는 모듈의 패키지를 임포트하면 먼저 `go mod tidy`를 실행합니다. TSX만 변경한 경우에는 상태를 유지한 채 실행 중인 앱을 제자리에서 업데이트합니다. 앱을 닫으면 실행이 종료됩니다.

macOS와 Windows에서는 네이티브 dev 재빌드 시 현재 세션 동안 메인 뷰의 창이 마지막 일반 위치와 크기로 다시 열립니다. Windows는 전체 화면 상태도 복원합니다. Linux에서는 GTK가 마지막 일반 크기를 복원하고 마지막 전체 화면 모드를 요청하며, 위치는 창 관리자가 결정합니다.

`-target`으로 다른 플랫폼에서 실행할 수 있고, `-device`로 이름, UDID 또는 시리얼을 지정해 iPhone, Apple TV, Apple Watch, 시뮬레이터 또는 Android 기기를 선택할 수 있습니다.

```sh
go tool nsgo dev -target tui                # terminal cells; diagnostics in .nsgo/tui.log
go tool nsgo dev -target web                # in the browser, at the address it prints, on -port (8080 by default)
go tool nsgo dev -target ios_simulator      # in the iOS simulator
go tool nsgo dev -target ios                # on the connected iPhone, signed by Xcode
go tool nsgo dev -target tvos_simulator     # in the Apple TV simulator
go tool nsgo dev -target watchos_simulator  # in the Apple Watch simulator
go tool nsgo dev -target android            # on a connected device or emulator, over adb
go tool nsgo dev -target miniprogram_wechat # in WeChat DevTools (also _qq, _alipay, _douyin)
```

dev 서버는 기본적으로 모든 인터페이스에서 수신 대기합니다. Android가 `adb reverse`를 통해 연결할 때를 포함해 루프백으로만 제한하려면 `-host 127.0.0.1`을 사용하고, 특정 로컬 인터페이스에서만 수신하려면 해당 인터페이스의 IP 주소를 지정하세요. 실제 Apple 기기에서 dev 기능을 사용하려면 LAN에서 접근 가능한 주소를 사용하세요.

`-h`를 실행하면 모든 플래그 목록을 볼 수 있습니다.

## Go 디버거

사용 중인 Go 버전과 호환되는 [Delve](https://github.com/go-delve/delve/blob/master/Documentation/installation/README.md)를 `PATH`에 `dlv`라는 이름으로 두세요. 디버깅 정보를 포함하고 Go 최적화를 끈 상태로 네이티브 데스크톱 앱을 시작합니다.

```sh
go tool nsgo dev -target native -debug
```

Delve가 출력하는 `API server listening at` 줄에서 포트 번호를 복사하세요. Go 확장이 설치된 VS Code에서는 attach 구성을 추가하고 `40000`을 해당 포트로 바꿉니다.

```json
{
  "name": "Attach to NSGo",
  "type": "go",
  "request": "attach",
  "mode": "remote",
  "debugAdapter": "dlv-dap",
  "host": "127.0.0.1",
  "port": 40000
}
```

GoLand에서는 호스트 `127.0.0.1`과 동일한 포트로 **Go Remote** 실행 구성을 추가하세요. 앱의 Go 코드에 중단점을 설정하고 연결한 뒤 해당 코드에 도달하는 동작을 실행합니다. Go 재빌드가 성공적으로 완료된 후에는 새 프로세스에 중단점이 전달되도록 같은 주소로 디버거를 다시 연결하세요. TSX 변경은 여전히 핫 리로드를 사용합니다.

`-debug`는 macOS, Windows, Linux에서 `-target native`와 함께 동작합니다. 디버깅에는 dev 빌드를 사용하고, `-release`는 릴리스 빌드 성능을 측정할 때만 사용하세요.

## 컴포넌트 프리뷰

임포트 가능한 Go 패키지 안에서 컴포넌트 옆에 하나 이상의 `PreviewXxx` 함수를 작성하세요. 프리뷰 빌드 태그로 보호된 Go 파일에 저장합니다.

```go
//go:build nsgopreview

package components

import "github.com/nsgo-dev/nsgo/core"

func PreviewEmpty(cx *core.Scope) core.Node {
    return core.Text{Content: "No items"}
}
```

```sh
go tool nsgo dev -preview ./components
go tool nsgo dev -target web -preview ./components
```

함께 표시할 픽스처를 체크하고 로캘, 테마, 라이트/다크 모양, 뷰포트 크기, 레이아웃 방향을 선택하세요. 프리뷰는 가장 가까운 앱의 `nsgo.yaml`, 번역 카탈로그, 설정된 `theme` 및 리소스를 사용합니다. 같은 모듈 내의 다른 설정을 선택하려면 플래그 뒤에 앱 디렉터리를 전달하세요. 이 경우 프리뷰 경로는 앱 디렉터리를 기준으로 합니다.

파일을 저장하면 선택한 항목과 표시 설정을 유지한 채 프리뷰가 다시 빌드됩니다. `nsgopreview` 태그는 일반 릴리스 빌드에서 픽스처를 제외합니다. 태그가 붙은 `_test.go` 파일에서 동일한 픽스처를 `nsgotest.Start(t, PreviewEmpty)`로 사용하고 `GOFLAGS=-tags=nsgopreview go tool nsgo test -target macos ./components`로 실행하면 네이티브 스크린샷을 찍을 수 있습니다.

`-target ios_simulator`로 메인 앱을 처음 설치한 후에는 Go만 변경해 재빌드할 때 설치된 실행 파일만 교체하고 앱을 다시 실행합니다. `nsgo.yaml`, 패키징된 리소스 또는 앱 아이콘을 편집하면 다시 설치가 수행됩니다.

Android UI 작업에서 Go 재빌드 주기를 더 짧게 하려면 `go tool nsgo dev -target android -android-host`를 사용하세요. 도구가 네이티브 런타임을 한 번 설치한 뒤 개발 머신에서 Go를 실행합니다. Go를 다시 빌드할 때마다 adb를 통해 동일한 Activity에 다시 연결됩니다. 패키징된 리소스, 사전 컴파일된 클래스, 런처 아이콘 또는 `nsgo.yaml`을 변경하면 런타임 APK를 다시 설치합니다. 이때 Go 프로세스는 호스트의 빌드 제약 조건, 파일, 네트워킹을 사용하며, `Scope.DiskSpace`는 호스트의 `DataDir`이 있는 볼륨의 용량을 보고합니다. `_android.go` 파일과 기기 내 Go 동작을 검증하려면 일반 Android dev 명령을 사용하세요.

## 오류

빌드가 실패하면 실행 중인 앱 위에 오류를 복사하는 버튼과 다시 빌드하는 버튼이 함께 표시됩니다. 파일을 수정해 다음에 저장하면 새 빌드로 교체됩니다. 렌더링 중 패닉이 발생한 컴포넌트도 같은 방식으로 표시됩니다. 앱에서 Command+R(Apple 이외 플랫폼에서는 Control+R)을 누르면 오류가 표시된 상태에서도 언제든 앱을 다시 빌드하고 시작합니다. 단, 앱이 `cx.OnKey`나 활성화된 애플리케이션 메뉴 항목으로 해당 키를 직접 바인딩한 경우는 제외되며, 브라우저에서는 평소처럼 페이지를 새로 고칩니다. `log.Print` 같은 앱 자체 출력은 도구 로그 아래의 터미널에 나타납니다.

실행하기 전에 `go tool nsgo check`를 실행하면 앱을 정적 검사(`vet`)하고 TSX 타입을 검사합니다. `-target all`을 주면 모든 대상을 검사하므로 다른 플랫폼의 빌드 태그 뒤에 있는 코드도 Mac에서 함께 확인할 수 있습니다.

### 패키징된 Apple 앱의 로그

macOS, iOS, tvOS, watchOS에서는 기본 Go 로거가 `log.Print`, NSGo 진단 로그, TSX 콘솔 출력을 포함해 Apple의 통합 로그에 기록합니다. Console 앱에서 실행 중인 앱의 번들 식별자를 subsystem으로, `nsgo`를 category로 필터링하세요. macOS에서는 저장소를 직접 조회할 수도 있습니다.

```sh
log show --last 10m --predicate 'subsystem == "dev.example.hello"'
```

레코드는 기본 로그 유형을 사용하며 경고 또는 오류 접두사를 유지합니다. 이 텍스트는 외부에 노출되므로 자격 증명과 기타 비밀 정보는 로그에 남기지 마세요. 긴 메시지는 여러 항목으로 나뉘어 기록됩니다. 개발 중에는 터미널 출력도 계속 제공되며, 커스텀 로그 라이터나 `slog` 핸들러는 자체 출력을 제어합니다.

### Linux 웹 뷰

Ubuntu 24.04 이상은 AppArmor를 통해 비특권 사용자 네임스페이스를 제한할 수 있습니다. bubblewrap이 WebKitGTK의 샌드박스를 시작하지 못하면 NSGo가 오류를 기록하고 플레이스홀더를 표시합니다. 일부 WebKitGTK 버전은 첫 번째 웹 뷰를 여는 동안 `SIGTRAP`으로 종료되기도 합니다. [Ubuntu AppArmor 문서](https://documentation.ubuntu.com/security/security-features/privilege-restriction/apparmor/)

배포판이 `bwrap-userns-restrict` 같은 bubblewrap AppArmor 프로필을 제공한다면 이를 사용하세요. 그렇지 않다면 관리자가 [NSGo의 Linux CI 프로필](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/test.yml)을 출발점으로 삼아 `/usr/bin/bwrap`에 `userns` 권한을 부여하는 프로필을 로드할 수 있습니다. 이 프로필은 시스템의 bubblewrap을 사용하는 모든 앱에 적용되므로 관리자가 관리합니다.

`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1`을 설정하면 WebKit의 서브프로세스 샌드박스가 꺼집니다. 신뢰할 수 있는 콘텐츠를 다루는 로컬 개발 환경에서만 사용하세요.

## DevTools

```sh
go tool nsgo dev -devtools
```

`-devtools`는 모든 대상(휴대폰 앱 포함)에 대해 `nsgo dev`가 제공하는 DevTools를 브라우저에서 엽니다. 실행 중인 앱의 상태를 실시간으로 보여 줍니다.

- **Elements**: 트리 형태의 뷰 목록입니다. 각 행에는 뷰를 만든 컴포넌트, 에디터 해당 줄로의 링크, 테스트가 뷰를 찾는 셀렉터, 클래스, 경계, 레이아웃된 마진·테두리·패딩, 설정된 스타일이 표시됩니다. 행에 마우스를 올리면 브라우저 DevTools처럼 앱 위에 마진, 패딩, 콘텐츠 영역을 그리며, **Pick**을 누르면 창, 패널, 시트 또는 임베디드 뷰의 어떤 뷰든 클릭해 트리에서 찾을 수 있습니다.
- **State**: 코드가 생성한 시그널들을 이를 소유한 컴포넌트 및 컴퓨테이션 아래에 배치해 현재 값, 메모 값, 각 시그널을 읽는 컴퓨테이션 수를 보여 줍니다. 어떤 컴퓨테이션도 읽지 않는 값은 화면을 바꾸지 않습니다. 문자열, 불리언, 숫자는 직접 편집할 수 있어 코드 수정 없이 다양한 상태를 시험해 볼 수 있습니다. NSGo가 내부적으로 만든 값들은 **NSGo's own** 아래에 숨겨져 있습니다.
- **Profiler**: 각 커밋의 소요 시간을 커밋 전 사용자 코드, 레이아웃을 포함한 커밋 자체, 플랫폼 메인 스레드 적용 시간으로 나누어 보여 줍니다. 또한 커밋을 유발한 쓰기가 일어난 코드 줄을 표시하며, 실행된 컴퓨테이션 목록을 가장 오래 걸린 순으로 열어 줍니다. 두 번째 목록은 최근 1초 동안 가장 많이 실행된 컴퓨테이션을 생성 위치 줄 번호별로 나열합니다. Android 런타임은 자체 Java 코드로 배치를 적용하므로 Android 커밋에는 플랫폼 시간이 표시되지 않습니다.
- **Recorder**: 앱에서 누르고, 입력하고, 지우고, 스크롤한 동작을 `_test.go` 파일에 바로 복사할 수 있는 `nsgotest` 테스트 파일로 기록합니다. Elements에서 **Assert shown**과 **Assert text**를 사용하면 선택한 뷰에 대한 검증 구문을 추가합니다. 텍스트 검증은 앱이 표시한 언어 그대로 기록되므로 테스트가 실행될 언어로 녹화하세요.
- **Highlight updates**: `-highlight`와 동일하게 커밋이 변경한 각 뷰에 윤곽선을 표시합니다.
- **Layout**: `-layout`으로 시작하는 것과 동일하게 기기에서 [레이아웃 검사](#layout)를 표시하거나 숨깁니다.

TSX 앱의 상태, 컴퓨테이션, 쓰기 위치는 번들의 소스 맵을 통해 TSX 파일의 줄 번호로 표시되며 앱의 파일로 연결됩니다.

DevTools는 휴대폰에서도 접근할 수 있도록 네트워크에서 수신 대기하는 `nsgo dev`의 서버를 통해 앱을 읽습니다. 실행할 때마다 토큰이 생성되어 `nsgo dev`가 출력하고 여는 DevTools 및 웹 대상 주소의 `#` 뒤에 붙으며, 서버는 이 토큰을 가진 요청에만 응답합니다. 따라서 반드시 출력된 주소로 DevTools와 웹 대상을 여세요. 이 주소를 다른 사람에게 공유하면 그 사람이 실행 중인 앱의 상태를 바꾸거나 Recorder가 작성하는 테스트에 단계를 추가할 수 있으므로, 기록된 테스트를 실행하기 전에 내용을 검토하세요. 네트워크의 다른 기기나 방문한 다른 웹 사이트에서는 이 정보에 전혀 접근할 수 없습니다.

## 업데이트 추적

```sh
go tool nsgo dev -highlight
```

`-highlight`는 커밋이 변경한 각 뷰에 0.5초 동안 윤곽선을 그려 앱을 사용하면서 의도보다 넓게 다시 그려지는 영역을 눈으로 확인할 수 있게 합니다. 윤곽선은 모든 창, 패널, 모달, iOS 시트, 임베디드 뷰(`nsgo.NewView`) 위에 그려집니다. 앱이 실행 중일 때 DevTools에서 켜고 끌 수 있습니다.

```sh
go tool nsgo dev -trace
```

`-trace`는 실행 중인 앱의 각 커밋마다 커밋 번호, 소요 시간, 전송된 마운트 항목을 출력하므로 업데이트가 어떤 뷰를 생성, 변경, 제거했는지 볼 수 있습니다. 항목 위에는 이전 커밋 이후 시그널을 기록한 코드 줄(가장 많이 기록한 순)과 실행된 바인딩, 이펙트, 메모 수가 표시됩니다.

```text
── commit 12: 40 items, commit 1.65 ms
   after app/market.go:473 ×4, app/market.go:487 ×4, app/market.go:384
   ran 118 bindings, 18 effects, 5 memos
```

한 줄이 여러 번 쓰기를 수행하거나 한 번의 쓰기로 수많은 바인딩이 실행된다면 `cx.Batch`로 쓰기를 묶거나 바인딩이 읽는 범위를 좁혀야 할 지점입니다. `go tool nsgo test -trace`를 실행하면 테스트의 각 단계 아래에 동일한 내용이 출력됩니다.

## 런타임 프로파일

`nsgo dev`를 사용해 실행 중인 앱의 Go CPU 활동, 힙 또는 실행 트레이스를 캡처할 수 있습니다. 앱은 캡처 데이터를 dev 서버로 전송하고, 서버는 명령을 실행한 머신에 파일을 저장합니다.

```sh
go tool nsgo dev -target android -profile cpu -profile-duration 10s
go tool pprof -top .nsgo/profiles/cpu.pprof
```

CPU 프로파일 예제는 데스크톱, iPhone, iOS 시뮬레이터에서도 동작합니다. iPhone에서는 시스템이 묻는 경우 로컬 네트워크 접근을 허용하세요. 힙은 스냅샷 방식이며 웹에서도 동작합니다. 스케줄러와 고루틴 이벤트를 보려면 `go tool nsgo dev -profile trace`를 실행한 뒤 `go tool trace`로 `.nsgo/profiles/execution.trace`를 여세요. CPU 및 실행 트레이스의 기본 시간은 10초입니다. `-profile-output`은 로컬 저장 경로를 지정합니다. 컴포넌트 프리뷰에서는 기본 경로와 상대 경로가 앱 디렉터리를 기준으로 합니다. dev 실행은 첫 번째 완료된 캡처를 저장한 뒤에도 앱을 계속 실행합니다.

```sh
go tool nsgo dev -target web -profile heap
go tool pprof -top .nsgo/profiles/heap.pprof
```

Go의 브라우저 런타임은 힙 프로파일과 실행 트레이스를 지원합니다. 브라우저 CPU 샘플링은 브라우저의 Performance 도구를 사용하세요. `-profile`과 `-release` 중 하나만 선택할 수 있습니다. `nsgo build`로 배포 빌드를 만들면 dev 캡처 클라이언트가 제외됩니다. [Go 런타임 진단](https://go.dev/doc/diagnostics)

## 레이아웃

```sh
go tool nsgo dev -layout
go tool nsgo dev -layout=bounds,grid=4
```

`-layout`은 모든 창, 패널, iOS 시트, 임베디드 뷰 위에 DevTools의 레이아웃 검사를 켠 상태로 앱을 시작합니다. 모든 뷰의 테두리 상자는 파란색, 패딩 안쪽 경계는 초록색, 마진 바깥 경계는 주황색 윤곽선으로 그리며, 스크롤 뷰나 클리핑 뷰에 가려진 부분은 잘라서 표시합니다. `grid`는 8포인트 격자를, `grid=N`은 최소 4 이상의 N포인트 격자를 그립니다. 값은 `=` 뒤에 지정하며 플래그만 단독으로 쓰면 `bounds`를 의미합니다.

하단 모서리의 판독기에는 창 크기가 포인트 단위로 표시되며 상자, 격자, 눈금자를 켜고 끄는 버튼이 있습니다. Apple TV 및 기타 리모컨 전용 기기에서는 포커스가 판독기 버튼으로 이동하므로 앱을 나가지 않고 리모컨으로 전환할 수 있습니다. 눈금자가 켜져 있는 동안에는 누르기 동작이 앱에 전달되지 않고 두 점을 선택하며, 판독기가 두 점 사이의 가로·세로 거리를 표시합니다. 세 번째 누르기는 측정을 새로 시작합니다. Apple TV나 키보드에서는 방향키(또는 Siri Remote 터치패드 스와이프)로 화면 위의 십자선을 움직이고 Select, Return 또는 Space로 점을 찍으면 두 번째 점이 고정될 때까지 거리가 실시간으로 갱신됩니다.

| 키 | 동작 |
| --- | --- |
| `Cmd+Option+Shift+L`, `Ctrl+Alt+Shift+L` | `-layout` 빌드에서 레이아웃 검사를 표시하거나 숨깁니다 |
| 방향키, 리모컨 D-pad / 스와이프 | 눈금자 십자선을 격자 간격(기본 8 pt)만큼 이동합니다 |
| `Shift` + 방향키 | 눈금자 십자선을 1 pt씩 이동합니다 |
| Select, `Return`, `Space` | 측정 중 현재 십자선 위치를 선택합니다 |
| `Esc`, 뒤로 가기, Menu | 측정을 종료합니다 |

앱이 같은 키를 바인딩했다면 앱의 바인딩이 우선합니다. 휴대폰처럼 키가 없는 환경에서는 `-devtools`도 함께 실행해 DevTools의 **Layout**으로 검사를 켜고 끄세요. 판독기 아래에 있는 컨트롤을 누르려면 검사를 잠시 숨기면 됩니다. 네이티브 백엔드와 웹은 프레젠테이션 행렬을 읽어 경계, Pick, 레이아웃 검사가 표시된 변환, 애니메이션, 스크롤을 그대로 따르며, watchOS는 아핀 드로잉을 따릅니다. HarmonyOS와 미니 프로그램은 렌더 레이어가 지오메트리를 보고할 때까지 Go 측 추적기를 유지합니다. 이를 다시 그리는 데 매 업데이트마다 시간이 들므로 속도를 측정할 때는 끄고 측정하세요.

## 속도

```sh
go tool nsgo dev -fps
go tool nsgo dev -release -fps
```

`-fps`는 macOS, iOS, Windows, Linux, Android에서 앱 위에 성능 모니터를 표시합니다. 프레임과 커밋을 측정하며 Android에서는 네이티브 텍스트 왕복 시간도 함께 보여 줍니다. dev 빌드에는 dev 클라이언트가 포함되므로 실제 성능을 측정할 때는 `nsgo build`와 동일하게 빌드하는 `-release`를 사용하세요.

## 언어 및 모양

```sh
go tool nsgo dev -lang zh-Hans
go tool nsgo dev -lang en-XA   # accented and longer, to find untranslated and cut text
go tool nsgo dev -lang ar-XB   # right to left
```

`-lang`은 지정한 로캘(의사 로캘 포함)로 앱을 시작합니다([지역화](localization.md)).

## 에이전트

```sh
go tool nsgo dev -automation
go tool nsgo dev -target android -automation
go tool nsgo dev -target web -automation
```

`-automation`은 현재 macOS, Windows, Linux(`-target native`), Android(`-target android`), 웹(`-target web`), 각 개발자 도구의 시뮬레이터 내 미니 프로그램(`-target miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay`, `miniprogram_douyin`), 그리고 Apple 기기 및 시뮬레이터(`-target ios`, `tvos`, `watchos`, `ios_simulator`, `tvos_simulator`, `watchos_simulator`)에서 AI 에이전트나 엔드 투 엔드 러너가 사람처럼 실행 중인 앱을 조작할 수 있게 합니다. 창 목록 조회, 화면 요소 구조 확인, 누르기, 입력, 키 전송, 스크롤, 드래그, 호버, 메뉴 항목 선택, 알림 응답, 화면 등장 대기, 스크린샷 촬영이 가능합니다. 에이전트는 `go tool nsgo mcp`를 통해 MCP로 연결하며, 이 명령은 해당 디렉터리에서 실행 중인 `dev -automation`을 사용하거나 동일한 플래그(예: `-target android`)로 새 세션을 시작했다가 클라이언트가 종료하면 함께 중단합니다.

```sh
claude mcp add nsgo -- go tool nsgo mcp
```

다른 클라이언트에서도 앱 디렉터리에서 `go` 명령에 `tool nsgo mcp` 인자를 넘겨 동일하게 연결할 수 있습니다. 뷰는 [테스트](testing.md#a-test)의 셀렉터나 스냅샷이 출력하는 `view:` 참조로 지정하며, 파일을 저장해 앱이 재시작되면 기존 참조는 무효가 됩니다.

에이전트는 `dev`가 실행하는 앱의 실제 데이터에 작용합니다. `dev -automation` 빌드에만 이 기능이 포함되며, 사용자 본인만 읽을 수 있는 `.nsgo/automation.yaml`의 토큰을 가진 현재 머신의 도구만 접근할 수 있습니다.

### 앱에서 정의하는 MCP 도구

Go 앱이나 라이브러리에서 엔진 고루틴 위에서 `automation.Register`를 호출하세요. 예를 들어 아래 함수는 현재 표시 중인 장면의 ID를 에이전트에 제공합니다.

```go
import (
    "context"
    "encoding/json"

    "github.com/nsgo-dev/nsgo/automation"
    "github.com/nsgo-dev/nsgo/core"
)

func registerSceneTool(cx *core.Scope, sceneID *core.Signal[string]) error {
    return automation.Register(cx, automation.Tool{
        Name:        "current_scene",
        Description: "Read the ID of the active scene",
        InputSchema: json.RawMessage(`{"type":"object"}`),
        Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
            return map[string]string{"id": sceneID.Get()}, nil
        },
    })
}
```

이름은 앱의 그래프 내에서 고유해야 하며 내장 이름과 겹치지 않아야 합니다. `nsgo mcp`는 스코프가 정리될 때까지 이 도구를 목록에 노출합니다. 핸들러는 엔진 고루틴에서 실행되므로 시그널을 읽거나 설정할 수 있습니다. JSON 객체를 받아 필요한 인자를 검증한 뒤 JSON으로 인코딩 가능한 값이나 오류를 반환합니다.

테스트에서는 앱이 시작될 때 스코프를 캡처해 `app.Post` 안에서 `automation.Call(cx, ctx, name, args)`를 호출하고, 결과나 뷰를 확인하기 전에 `app.Settle()`을 호출하세요. 등록 기능은 `dev -automation`과 테스트에서만 활성화되며 릴리스 빌드에서는 아무 동작도 하지 않습니다(no-op).

## 모든 빌드의 공통 플래그

패키징된 앱을 포함한 모든 빌드는 시작 시 NSGo 자체 플래그를 받습니다. 이 플래그들은 `nsgo.yaml`과 앱 코드를 재정의하며 앱 자체의 플래그로는 전달되지 않습니다.

| 플래그 | 동작 |
| --- | --- |
| `--nsgo-fps` | macOS, iOS, Windows, Linux, Android에서 성능 모니터를 표시합니다 |
| `--nsgo-lang=zh-Hans` | 의사 로캘을 포함해 지정한 로캘로 앱을 시작합니다 |
| `--nsgo-appearance=dark` | 앱을 `light`, `dark` 또는 `system` 설정에 따라 시작합니다 |
| `--nsgo-os=ios` | 웹 빌드를 `macos`, `windows`, `linux`, `ios`, `android` 또는 `web` 자체로 실행합니다 |

```sh
./Hello --nsgo-fps                                   # Windows and Linux
open Hello.app --args --nsgo-fps                     # macOS
xcrun simctl launch booted dev.example.hello --nsgo-fps
adb shell am start -n dev.example.hello/dev.oneo.nsgo.NsgoActivity --ez nsgo-fps true --es nsgo-lang de
```

웹에서는 페이지 쿼리에 넣습니다: `?nsgo-fps&nsgo-lang=de`.

`--nsgo-os`는 브라우저에서 다른 플랫폼의 레이아웃을 미리 봅니다. 앱은 해당 OS와 그 관용구(idiom, iOS와 Android는 휴대폰, 그 외에는 데스크톱)를 읽으며 `Env.Simulated`가 true를 반환합니다. 브라우저에는 해당 플랫폼의 네이티브 구성 요소가 없으므로 시스템 요소 대신 `NavHeader`와 직접 그린 탭 막대가 사용되고, 애플리케이션 메뉴, 네이티브 팝업 메뉴(`desktop.PopupMenu`), 창 함수는 없으며 `mobile.IsAvailable`과 `desktop.IsAvailable`은 false입니다. `ui.ContextMenu`와 `ui.Menu`처럼 직접 그린 메뉴는 그대로 동작합니다. 단축키는 방문자의 키보드를 따릅니다. 다른 빌드에서는 이 플래그를 무시하고 경고를 기록합니다.

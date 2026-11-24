---
title: 설치 및 준비
description: NSGo에 필요한 도구를 설치하고 앱을 생성해 모든 대상에서 실행합니다.
group: 시작하기
order: 10
---

## 요구 사항

- [Go](https://go.dev/dl/) 1.27.3 이상.
- iOS, tvOS, watchOS용으로 빌드하려면 Xcode가 설치된 Mac이 필요합니다. 다른 모든 대상은 macOS에서 Go만으로 빌드할 수 있으며, x86_64 Android 에뮬레이터용 빌드에는 Android NDK도 필요합니다.
- iPhone에서 실행하려면 Xcode의 Accounts에 등록된 Apple ID와 신뢰할 수 있는 기기로 설정된 iPhone이 필요하며, 개발자 모드(Developer Mode)가 켜져 있어야 합니다.
- Android에서 실행하려면 Android SDK의 platform-tools에 포함된 `adb`가 필요합니다.
- Windows에서 실행하려면 Go만 있으면 되며, Go만으로 Windows 및 웹 패키지도 빌드할 수 있습니다.
- Linux에서는 GTK 4.14 이상, Pango, Cairo가 필요합니다. 네트워크 이미지는 libsoup 3를, 웹 뷰는 WebKitGTK 6.0을, 오디오와 비디오는 GStreamer 플러그인을 사용합니다. AppArmor 사용자 네임스페이스 제한이 있는 시스템에서는 [WebKitGTK에 bubblewrap 프로필이 필요합니다](debugging.md#linux-web-views).

## 앱 생성하기

모듈을 만들고 NSGo와 해당 도구를 추가한 다음, 도구가 앱을 생성하도록 합니다:

```sh
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
```

`go get -tool`은 모듈에 `nsgo`를 한 번 추가하므로, 별도 설치 없이 `go tool nsgo`로 실행할 수 있습니다. `init`은 다음 파일들을 생성합니다:

| 파일 | 설명 |
| --- | --- |
| `app.go` | 카운터 앱: `Root` 컴포넌트와 `nsgo.Run(Root)`를 호출하는 `main` 함수 |
| `app_test.go` | 버튼을 누르는 테스트 |
| `nsgo.yaml` | 앱의 이름, ID, 버전 및 대상 |

`go tool nsgo init -template tsx`는 대신 TSX 앱을 생성하며, `-template mixed`는 TSX 진입점이 자체 Go 컴포넌트를 사용하는 Go 앱을 생성합니다.

### Go 모듈의 템플릿

선택적 버전과 함께 템플릿의 모듈 경로를 전달합니다. 버전을 생략하면 Go 모듈 프록시가 찾을 수 있는 최신 버전으로 결정됩니다.

```sh
go tool nsgo init -template example.com/my-template@v1.2.0 -module example.com/hello hello
```

모듈의 `template/` 디렉터리 안에 시작 템플릿을 게시하세요. `nsgo.yaml`과 `main.go`를 포함해야 합니다. 도구는 이 두 파일과 `.tmpl`로 끝나는 모든 파일을 Go의 `text/template`으로 렌더링하고, 출력 파일 이름에서 `.tmpl`을 제거합니다. 바이너리 에셋을 포함한 나머지 파일은 변경 없이 그대로 복사합니다.

템플릿은 `.Name`(앱의 표시 이름), `.ID`(생성된 앱 ID), `.Slug`(소문자 및 하이픈으로 연결된 이름), `.Module`(`-module` 값 또는 슬러그)을 전달받습니다. Go는 모듈 다운로드에서 중첩된 `go.mod` 파일을 제외하므로, 생성될 앱의 모듈과 의존성을 선언하려면 `go.mod.tmpl`을 함께 제공하세요. 이 파일이 없으면 `-module`은 내장 템플릿과 동일한 최소한의 `go.mod`를 작성합니다.

## 실행하기

```sh
go tool nsgo dev
```

`dev`는 현재 머신용으로 앱을 빌드하고 실행합니다. 저장할 때마다 앱을 다시 빌드하고 재시작하며, TSX만 변경된 경우에는 실행 중인 앱을 제자리에서(in place) 업데이트합니다. 빌드에 실패하면 앱 위에 오류가 표시되며, 오류를 복사하거나 다시 빌드하는 버튼이 제공됩니다.

`-target`으로 다른 환경에서 실행할 수 있습니다:

```sh
go tool nsgo dev -target web                # 브라우저에서 실행
go tool nsgo dev -target ios_simulator      # iOS 시뮬레이터에서 실행
go tool nsgo dev -target tvos_simulator     # Apple TV 시뮬레이터에서 실행
go tool nsgo dev -target watchos_simulator  # Apple Watch 시뮬레이터에서 실행
go tool nsgo dev -target android            # 연결된 기기 또는 에뮬레이터에서 실행
```

`go tool nsgo doctor`는 모든 대상에 대해 이 머신에서 빌드 및 실행이 가능한지 여부와, 불가능한 대상에 부족한 항목(Xcode, `adb`, 연결된 기기, GTK 등)을 나열합니다.

[실행 및 디버깅](debugging.md)에서는 기기 실행, 업데이트 추적, 성능 모니터 및 다른 언어 지원을 다룹니다.

## 코딩 에이전트와 함께 사용하기

NSGo의 스킬은 Claude Code, Codex, Cursor 같은 코딩 에이전트에게 NSGo 앱을 작성, 테스트 및 배포하는 방법을 알려줍니다. 앱 디렉터리에 추가하거나, `-g` 옵션으로 모든 프로젝트에 적용할 수 있습니다:

```sh
bunx skills add nsgo-dev/nsgo
```

문서를 직접 읽는 에이전트는 <https://nsgo.dev/llms.txt>에서 시작할 수 있으며, 여기에는 모든 페이지가 `https://nsgo.dev/docs/<page>.md` 형식의 Markdown으로 나열되어 있습니다. <https://nsgo.dev/llms-full.txt>에는 모든 문서가 하나로 모여 있습니다. 에이전트는 실행 중인 앱을 직접 사용할 수도 있습니다([실행 및 디버깅](debugging.md#agents)).

## 다음 단계

- [예제: 할 일 앱](tutorials/todo.md): 약 10분 만에 살펴보는 상태, 목록 및 테스트.
- [예제: 메모 앱](tutorials/notes.md): 내비게이션 스택과 기기에 저장되는 메모.
- [예제: 인기 GitHub 저장소](tutorials/repos.md): 오류 처리와 재시도를 포함한 네트워크 로딩.

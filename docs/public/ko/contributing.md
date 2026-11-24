---
title: 개발 환경 설정
description: 소스에서 NSGo를 빌드하고, 갤러리를 실행하며, 변경 사항을 테스트합니다.
group: 기여하기
order: 80
---

## 요구 사항

Xcode와 Go 1.27.3 이상이 설치된 macOS는 Android APK를 포함해 arm64용 모든 대상을 빌드할 수 있습니다. 실제 기기에서 실행하려면 [설치 및 준비](getting-started.md#requirements)에 나열된 항목이 필요하며, x86_64 Android 에뮬레이터 대상인 `android_x64`는 Go가 cgo를 통해서만 android/amd64를 링크하므로 Android NDK가 필요합니다. Windows 설치 프로그램을 만들려면 NSIS(`brew install makensis`)가 필요하며, 없으면 `build`가 스크립트 파일만 남깁니다.

```sh
git clone https://github.com/nsgo-dev/nsgo.git
cd nsgo
```

코드를 수정하기 전에 [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md)와 [AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md)를 먼저 읽으세요.

## 갤러리

갤러리에는 각 엘리먼트, `base` 및 `ui` 컴포넌트, 서비스의 데모가 들어 있습니다. 넓은 창에서는 사이드바의 탭 아래에 데모가 표시되고, 좁은 창에서는 탭과 설정이 하단 막대에 표시되며 데모가 그 위로 열립니다. 저장소 루트에서 실행하세요.

```sh
go tool nsgo dev examples/gallery                   # nsgo.yaml's dev target, exiting with the app
go tool nsgo dev examples/gallery-tsx               # the gallery's components and apps, written in TSX
go tool nsgo dev -target web examples/gallery       # in the browser, serving until interrupted
go tool nsgo dev -target ios_simulator examples/gallery
go tool nsgo dev -target ios examples/gallery       # on the connected iPhone, signed by Xcode
go tool nsgo dev -target android examples/gallery   # on the connected device or emulator, over adb
go tool nsgo build -target all examples/gallery     # every platform's packages in examples/gallery/dist, what to hand out in dist/releases
```

## 검사 및 테스트

```sh
go tool nsgo check                 # vet every package
go tool nsgo check -target all     # vet every available Go/Xcode target instead
go tool nsgo test                  # every package's tests
go tool nsgo test -bench . ./core  # core's benchmarks
(cd benchmark && go run ./runner)  # measure against other frameworks
# capacity curve for NSGo's 600-cell grid, without rebuilding it
(cd benchmark && go run ./runner -frameworks nsgo -cases grid -grid-cols 20 -grid-rows 15 -out /tmp/grid.json)
```

`-target`은 호스트 대신 지정한 대상들을 정적 검사(`vet`)하며, NSGo 저장소 안에서는 일반 `check`가 검사하는 dev 클라이언트도 함께 검사합니다. 변경 사항이 영향을 미치는 플랫폼을 검사하려면 여러 번 나누어 실행하지 말고 `check -target web,android -jobs 2`처럼 한 번에 모두 지정하세요. `-jobs n`은 최대 n개의 대상을 동시에 검사하며 각 `go vet`에 n개의 컴파일 워커를 할당합니다(기본값은 한 번에 하나의 대상에 Go 기본값 사용). 같은 모듈에서 다른 검사가 실행 중일 때 시작된 `check`는 앞선 검사가 끝날 때까지 기다린다는 안내를 출력하고 대기합니다. CI의 `check.yml`은 모든 대상을 검사합니다.

저장소 루트나 `internal/dev`에서 실행하면 헤드리스 명령이 dev 클라이언트 파일에 필요한 `nsgodev` 빌드 태그를 붙여 `internal/dev`를 한 번 더 테스트하므로, 로컬과 CI 모두에서 해당 테스트가 실행됩니다. 두 실행 모두 같은 필터와 플래그를 받으며 하나의 집계 및 JUnit 보고서를 공유하고, 두 번째 실행의 패키지 이름은 `internal/dev [nsgodev]`로 표시됩니다.

`check -target all`은 어떤 호스트에서든 macOS, Windows, Linux, Android arm64 및 웹 대상을 검사합니다. Xcode SDK를 사용할 수 있는 경우 iOS, tvOS, watchOS도 검사하며 건너뛴 대상 목록을 출력합니다. `check -target ios`처럼 명시적으로 지정한 대상은 도구를 사용할 수 없으면 `all`과 함께 지정한 경우라도 실패합니다. 한 대상의 실패가 나머지 대상 검사를 막지는 않으며, 필수 검사 중 하나라도 실패하면 명령이 실패합니다. `build -target all`은 여전히 Xcode가 설치된 macOS가 필요합니다. NSGo 자체 저장소에서는 지정된 각 대상의 플랫폼 테스트 호스트도 `nsgotest`로 함께 검사합니다. CI는 프리뷰 대상인 `harmonyos`가 `all` 밖에 있으므로 이를 명시적으로 지정합니다.

Linux CI 작업은 Node.js와 Go의 Wasm 런처를 사용해 HarmonyOS Go 백엔드의 `js/wasm` 테스트를 실행합니다. 로컬에서는 `hdc`로 연결된 HarmonyOS 에뮬레이터로 갤러리 테스트를 실행하세요. [HarmonyOS 호스트 템플릿](building.md#harmonyos)을 컴파일하려면 OpenHarmony SDK API 26이 필요합니다.

```sh
go tool nsgo test -env GOOS=js -env GOARCH=wasm ./internal/platform/harmonyos \
  -- "-exec=env -i \"PATH=$PATH\" \"$(go env GOROOT)/lib/wasm/go_js_wasm_exec\"" -count=1
go tool nsgo test -target harmonyos -run TestHarmonyOSPages -timeout 10m ./examples/gallery
```

Linux 데스크톱의 Wayland 세션에서는 `NSGO_TEST_GLOBAL_SHORTCUT_PORTAL=<installed-desktop-id> go tool nsgo test -run TestGlobalShortcutPortalWithPermission ./internal/platform/linux` 명령으로 실제 전역 단축키 포털 인수 검사를 실행하세요. 해당 ID에는 `.desktop` 항목이 있어야 합니다. 컴포지터 프롬프트에서 Ctrl+Alt+K 권한을 허용한 뒤 다른 애플리케이션에 포커스를 둔 상태에서 누르세요. 검사는 등록과 활성화가 모두 완료될 때까지 기다립니다. 환경 변수가 설정되어 있지 않으면 이 검사는 건너뜁니다.

Linux 호스트에서 Docker를 사용할 수 있다면 `go tool nsgo test -run '^TestLinuxPackagesNative$' -env NSGO_TEST_LINUX_PACKAGES=1 ./cmd/nsgo -- -timeout=20m`를 실행해 일회용 Debian, Ubuntu, Arch 컨테이너에서 실제 설치를 검증할 수 있습니다. 공식 이미지를 내려받아 그 안에 GTK 의존성을 설치하고, 각 패키지 데이터베이스와 설치된 파일을 검증한 뒤 패키지를 제거합니다. 호스트의 패키지 데이터베이스는 절대 사용하지 않습니다.

## 릴리스

NSGo는 하위 프로젝트가 태그된 버전을 필요로 하거나 메인테이너가 배포할 준비가 된 변경 사항이 있을 때 수시로 릴리스합니다. 라이브러리를 위해 릴리스를 요청할 때는 해당 의존 작업을 링크하고 필요한 마지막 NSGo 변경 사항이 포함된 커밋을 명시하세요. 그 커밋을 포함하는 첫 번째 릴리스 태그가 해당 라이브러리가 요구할 수 있는 버전이 됩니다.

태그되기 전에 병합된 변경 사항을 테스트하려면 의존 모듈에서 `go get github.com/nsgo-dev/nsgo@main`을 실행하세요. 검증할 리비전을 선택하려면 `main` 대신 커밋 ID를 사용할 수 있습니다. Go는 결과 의사 버전(pseudo-version)을 `go.mod`에 기록합니다.

메인테이너가 `CHANGELOG.md`의 변경 사항에 버전과 날짜를 부여하고 새 Unreleased 섹션을 남긴 뒤 `main`의 해당 커밋에 태그를 답니다. [릴리스 워크플로](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/release.yml)는 태그를 확인하고 테스트 워크플로가 통과한 후 릴리스를 게시합니다. 게시된 릴리스 노트는 미출시 변경 사항만 남기는 `CHANGELOG.md`를 떠나 [Releases](https://github.com/nsgo-dev/nsgo/releases)로 이동합니다.

## NSH

NSGo는 [Native Shell](https://github.com/longbridge/nsh)을 설계 참고 자료로 삼지만, 테스트에 NSH가 필요하지는 않습니다. 코드를 읽어 보려면 저장소 옆에 클론하고 `main`이 아닌 최신 릴리스 태그를 체크아웃하세요.

```sh
git clone https://github.com/longbridge/nsh.git ../nsh
git -C ../nsh checkout "$(git -C ../nsh describe --tags --abbrev=0)"
```

## 다음 단계

- [코드베이스](contributing/codebase.md): NSGo의 각 부분이 위치한 곳.
- [변경 사항 만들기](contributing/changes.md): 풀 리퀘스트에 필요한 사항.

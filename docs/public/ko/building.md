---
title: 빌드 및 배포
description: 모든 플랫폼용으로 앱을 패키징하고, 인스톨러를 만들고, 서명 및 공증하며, GitHub에 게시하고 다른 앱에 임베드합니다.
group: 툴체인
order: 22
---

## 빌드

```sh
go tool nsgo build
```

`build`는 `nsgo.yaml`에 나열된 각 타겟에 대해 디버그 심볼을 제거한 릴리스 패키지를 `dist/<name>-<version>-<target>`에 생성합니다. 결과물은 `.app`, `.ipa`, `.apk`, Windows 및 Linux 디렉터리, 또는 `import()`가 호출될 때 해당 TSX 모듈을 가져오는 웹 사이트입니다. `-target`을 지정하면 타겟 목록을 대체하며, `all`은 Go와 Xcode만으로 빌드할 수 있는 모든 타겟을 뜻합니다. Android NDK가 설치되어 있으면 `android_x64`를 추가할 수 있고, [미니 프로그램](#미니-프로그램)용으로는 `miniprogram` 또는 `miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay`, `miniprogram_douyin` 중 하나를, [HarmonyOS](#harmonyos) 프리뷰용으로는 `harmonyos`를 추가합니다. `-run`은 현재 머신용으로 빌드된 패키지를 실행하거나, `-device`로 지정한 iPhone, 시뮬레이터, Android 기기에 설치합니다.

배포할 모든 파일은 `dist/releases`에 모이므로 이 디렉터리만 확인하면 됩니다:

| `dist/releases` 내부 | 내용 |
| --- | --- |
| `*.dmg`, `*-setup.exe`, `*.deb`, `*.pkg.tar.gz`, `*.AppImage` | macOS, Windows, Linux용 [인스톨러](#인스톨러) |
| `*.AppImage.zsync` | AppImageUpdate 및 호환 런처용 델타 업데이트 제어 파일 |
| `*-ios.ipa`, `*-tvos.ipa`, `*.apk` | 스토어 또는 기기 배포용 iOS, tvOS, Android 패키지 |
| `*.pkg`, `*.msix`, `*.aab`, `*.flatpak`, `*.tar.gz` 및 `flatpak/` | `store` 빌드의 [스토어 패키지](#스토어-패키지) |
| `install.sh`, `install.ps1` | [GitHub 릴리스](#github에-게시)에서 앱을 설치하는 스크립트 |
| `appcast-<target>.xml` | 앱이 [업데이트](#업데이트)를 확인하는 피드 |
| `homebrew/<name>.rb`, `winget/` | 직접 배포(direct)용 [Homebrew cask 및 winget 매니페스트](#채널) |
| `*-web.zip` | 압축 해제 후 그대로 서빙할 수 있는 웹 사이트 |
| `miniprogram/<platform>/` | 각 플랫폼 개발자 도구에서 업로드할 미니 프로그램 프로젝트 |

각 파일은 `hello-1.2.0-windows-x64-setup.exe`처럼 `<name>-<version>-<system>-<arch>` 형식으로 이름이 지어져 릴리스 페이지에서 사용자가 자신의 시스템에 맞는 파일을 쉽게 찾을 수 있으며, 옆에 `.sha256` 파일이 함께 생성됩니다. 다른 버전을 빌드하면 먼저 디렉터리를 비우고, 다른 머신에서 같은 버전을 빌드할 때는 기존 디렉터리에 추가됩니다. 시뮬레이터와 watchOS 패키지는 각각의 `dist/<name>-<version>-<target>` 디렉터리에 남습니다.

웹 사이트에는 원본 모듈보다 크기가 작을 경우 `.wasm.br`과 `.wasm.gz` 파일도 포함됩니다. 사전 압축 파일을 지원하는 호스트에서는 이를 활성화하고, 원래 `.wasm` URL에서 `Content-Type: application/wasm`, 일치하는 `Content-Encoding`, `Vary: Accept-Encoding` 헤더와 함께 서빙하세요. 원본 모듈 자체는 어떤 정적 호스트에서도 동작합니다. [웹에 게시하기](web.md)에서는 시맨틱 헤딩, 브라우저 링크, 페이지별 메타데이터, CDN 로딩을 다룹니다.

### 오프라인 웹 패키지

`nsgo.yaml`에 `web.offline: true`를 설정하면 웹 릴리스를 첫 온라인 실행 시 캐싱합니다.

```yaml
web:
  offline: true
```

HTTPS로 게시된 디렉터리를 서빙하거나 로컬호스트에서 `go tool nsgo build -target web -run`을 사용하세요. 서비스 워커가 페이지, WebAssembly, 스크립트, 패키징된 리소스를 캐싱합니다. 온라인 방문 시에는 게시된 버전을 불러오며, 설치가 성공하고 이전 릴리스의 탭이 닫히면 최신 릴리스가 캐시에서 실행될 수 있습니다. 앱의 온라인 서비스 요청에는 여전히 네트워크 연결이 필요합니다.

## 터미널

`go tool nsgo dev -target tui`로 대화형 터미널에서 앱을 실행할 수 있습니다. `go tool nsgo test -target tui`는 `nsgotest.Main`을 통해 터미널에서 테스트를 실행하며, 테스트 프로세스당 하나의 애플리케이션이 실행됩니다. 크기는 터미널이 결정하며 셀 하나는 8×16 레이아웃 포인트에 해당합니다. 애플리케이션의 스타일을 명시적으로 선택하세요. 컴포넌트들은 일반적인 형태와 크기를 유지합니다. `examples/tui`에서 필드, 버튼, 단축키, 크기 조정을 확인할 수 있습니다.

콘솔 실행 파일을 빌드하려면 `go tool nsgo build -target tui_macos_arm64` 또는 `tui_macos_x64`, `tui_windows_arm64`, `tui_windows_x64`, `tui_linux_arm64`, `tui_linux_x64`를 사용하세요. `tui` 빌드/체크 그룹에는 이 6개가 모두 포함되며 `all`에도 포함됩니다. 각 패키지 디렉터리에는 실행 파일과 리소스가 들어가며 GUI 인스톨러는 생성되지 않습니다. 6개 모두 Go만으로 빌드됩니다.

이 백엔드는 셀 텍스트, 배경, 실선 테두리, 클리핑, 스크롤, 포커스, 텍스트 편집을 지원합니다. 터미널의 글꼴과 색상을 사용하며 Mac을 포함해 `mod`는 Control입니다. Ctrl+C는 에디터 안에서는 복사로 동작하고, 바인딩이나 에디터가 처리하지 않으면 종료합니다. Bracketed paste는 단축키를 실행하지 않고 텍스트를 삽입하며, 클립보드는 프로세스 내부 전용입니다. 캔버스, 이미지, 미디어, 네이티브 호스팅 컴포넌트, 시스템 메뉴/알림창, 애니메이션, 확대/축소, 양방향 셰이핑, 앞/중간 생략 부호, 시스템 클립보드 브리지는 아직 구현되지 않았습니다. `base/terminal`은 GUI 애플리케이션에 임베드되는 터미널 컨트롤을 가리킵니다.

`nsgo dev`가 터미널을 점유하는 동안 도구와 애플리케이션 진단은 `.nsgo/tui.log`에 기록됩니다. 종료 시에는 셸로 돌아가기 전에 화면, 커서, 터미널 모드를 복원합니다. 터미널 테스트 스크린샷은 에뮬레이터를 캡처하는 대신 고정된 포터블 글꼴로 셀을 래스터화하므로, 해당 글꼴에 없는 유니코드 글리프는 텍스트 단언문(assertion)으로 검증하세요.

## nsgo.yaml

```yaml
name: Hello
id: dev.example.hello   # the bundle ID; without it, dev.nsgo. and the name's letters and digits
version: 0.1.0
build_number: 1         # CFBundleVersion and Android's versionCode; else from version
# copyright: © 2026 Example  # the Windows executable's details and installer, and the macOS About panel
# appearance: dark      # light or dark whatever the system's
# theme: theme.json     # a TSX app's theme: NSGo's or a shadcn/ui one
# i18n: { dir: locales, source: en }  # the app's catalogs and the language they are written in
icon:
  apple: assets/AppIcon.icon  # macOS, iOS and watchOS, or an .icns for macOS
  windows: assets/IconWindows.png  # optional Windows artwork
  linux: assets/IconLinux.png      # optional Linux artwork
  png: assets/Icon.png        # every other target
  tvos: assets/AppIconTV.brandassets  # the Apple TV's layered icon and top shelf art, or a wide PNG
  watchos: assets/AppIconWatch.icon   # the Apple Watch's own, drawn in circles, or a square PNG
  android:
    png: assets/IconAndroid.png  # optional bitmap; valid without adaptive layers
    foreground: assets/IconForeground.png  # Android's adaptive icon: 432 px, the logo within the center 264 px
    background: "#1F1F1F"                 # a color or a square PNG
    monochrome: assets/IconMonochrome.png  # optional: the shape Android 13's themed icons tint
dev:
  target: native        # native, web, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android, harmonyos or miniprogram_<platform>
  hot_reload: true      # false builds once, watching no files
  # icon: { png: assets/Icon-dev.png }  # replaces icon's files in dev's builds; defaults to assets/*-dev.*
build:
  output: dist
  tags: []              # Go build tags every build, test and check of the app takes, such as timetzdata
  ldflags: []           # the app's own linker flags, such as -X main.releaseDate=2026-11-17
  open: false           # true opens the output directory after building
  installers: true      # false leaves out the disk image, the Windows installer and the Linux packages
  jobs: 0               # how many targets build at once; 0 is half the CPUs, 1 one after another
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android_arm64, web]
# apple: { sign_in_with_apple: true }  # the Sign in with Apple entitlement, for iOS, tvOS and Mac App Store builds
# apple: { purchases: true }  # StoreKit's library, which in-app purchases go through, in macOS, iOS and tvOS packages
# apple: { privacy: { accessed_apis: { NSPrivacyAccessedAPICategoryDiskSpace: [E174.1] }, tracking: false } }  # add required-reason APIs and tracking declarations to PrivacyInfo.xcprivacy
# apple: { requires_full_screen: false }  # let an iPad run the app in Split View, Slide Over and Stage Manager, in all four orientations
# apple: { orientations: [portrait, landscape_left] }  # the orientations a full-screen app takes on an iPad
# apple: { multiple_scenes: true }  # let an iPad open more than one window of the app, each showing the main view with its own state
# apple: { info: { UIBackgroundModes: [audio] } }  # background playback for iOS Picture in Picture
# android: { permissions: [REQUEST_INSTALL_PACKAGES] }  # manifest permissions beyond the runtime's
# android: { recording: true }  # the microphone service microphone.RecordWAV records in, with RECORD_AUDIO, the foreground-service permissions and POST_NOTIFICATIONS, which RecordWAV asks for so its notification shows from Android 13; a refusal still records
# android: { uses_cleartext_traffic: true }  # allow plain HTTP to any host in the APK and App Bundle
# android: { background_downloads: true }  # dataSync foreground service for cx.Download
# android: { background_motion: true }  # explicit finite motion shortService; streams also need a notification title
# android: { loopback_cleartext: true }  # allow plain HTTP to loopback in the APK and App Bundle
# commands: [{ name: hello-cli, package: ./cmd/hello-cli }]  # command-line programs the desktop packages ship beside the app: see Commands
# windows: { command_line: true }  # add <name>.com, a console companion that forwards arguments and streams to <name>.exe; not in store builds
# windows: { path: true }  # the installer puts the app's folder on the user's Path, so consoles find its commands
# windows: { publisher: Example Ltd }  # the company the Windows executable's details, its installer and the uninstall entry name; else windows.store.publisher_name
# linux: { maintainer: "Jane Doe <jane@example.com>" }  # the Linux packages' maintainer, else the app's name
# linux: { categories: [Office, Dictionary] }  # the freedesktop menu categories of the app's desktop entry, at least one a main category such as Office, Utility or Game, else menus file it under Other; your own start with X-
# linux: { share: packaging/share }  # files the .deb and Arch package install under /usr/share and the AppImage in its usr/share, such as metainfo/<id>.metainfo.xml or bash-completion/completions/<command>
# linux: { depends: { deb: [gstreamer1.0-plugins-good], arch: [gst-plugins-good] }, recommends: { deb: [gstreamer1.0-libav], arch: ["gst-libav: H.264 and AAC video"] } }  # packages the .deb and Arch package also depend on and recommend, after GTK 4 and WebKitGTK 6, each as dpkg or pacman writes it
# release: { github: me/hello }  # the GitHub repository the install scripts and updates install from, else the Git remote origin's; tag_prefix: v by default
# updates: { key: <public key> }  # how the app updates, by platform and channel: see Updates
# store: { dir: store }           # store listings directory (YAML and Markdown per language)
# miniprogram: { appid: { wechat: wx0123456789abcdef, alipay: "2021000000000000" }, custom_navigation: false }  # the mini programs'
```

`targets`나 `nsgo build -target`, `nsgo check -target`에 아키텍처 없는 OS 이름만 쓰면 해당 OS의 아키텍처들을 뜻합니다(`windows`는 `windows_arm64`와 `windows_x64`, `android`는 x86_64 빌드에 NDK가 필요하므로 `android_arm64`만 포함). `macos`와 `all`에서 제외되는 `macos_universal`은 두 macOS 아키텍처를 모두 빌드해 라이브러리와 위젯까지 하나의 앱으로 합칩니다. Steam 채널은 유니버설 앱 번들을 사용하며, 인스톨러 빌드는 이를 하나의 디스크 이미지로 묶고 직접(direct) 채널에서는 하나의 Sparkle 피드를 생성합니다. `nsgo test -target`에도 테스트를 실행할 아키텍처 이름을 지정할 수 있습니다. 데스크톱 OS는 현재 머신의 아키텍처에서, Android는 `arm64`에서 실행되므로 x64 Windows 머신에서는 `windows_x64`로 `windows` 타겟을 실행할 수 있으며, 실행할 수 없는 아키텍처를 지정하면 올바른 이름과 함께 거부됩니다.

`-target`, `-version`, `-output` 같은 플래그는 지정한 필드의 값을 대체합니다. 전체 목록은 `-h`로 확인할 수 있습니다.

`build.ldflags`에는 `-X main.releaseDate=2026-11-17`이나 `-X 'main.channel=beta builds'`처럼 go build의 `-ldflags` 형식으로 앱 고유의 링커 플래그를 적습니다. 이는 모든 타겟에서 `dev`와 리로드, `-embed`를 포함한 `build`, `-target`을 포함한 `test` 등 앱을 링크하는 모든 빌드에 적용됩니다. `build`와 `test`의 `-ldflags`는 기존 설정을 덮어쓰지 않고 뒤에 추가되며 여러 번 반복할 수 있습니다. 사용자 플래그는 도구 자체의 `-X` 플래그보다 앞에 오고 링커는 변수당 마지막 값을 유지하므로 `core.App`의 이름, ID, 버전은 `nsgo.yaml`의 값으로 유지됩니다. 도구가 go에 자체 `-ldflags`를 전달할 때 `GOFLAGS` 안의 값은 대체되므로 링커 플래그는 여기에 적어야 합니다. 반면 디렉터리 뒤에 오는 헤드리스 `test`의 `-ldflags`는 go test의 플래그로서 전체를 대체합니다.

앱을 위해 실행되는 모든 go 명령(`dev`와 리로드, `-embed` 포함 `build`, `-target` 포함 `test`, `check`)은 모든 타겟에서 `build.tags`를 사용합니다. `dev`의 `nsgodev`나 타겟 테스트의 `nsgotest` 같은 NSGo 자체 태그와 `GOFLAGS`의 `-tags` 값은 대체되지 않고 여기에 합쳐지며, 나머지 `GOFLAGS`도 그대로 적용됩니다.

`apple.info.LSMinimumSystemVersion`은 macOS 패키지의 최소 지원 버전을 높이고, `apple.info.MinimumOSVersion`은 선택된 iOS, tvOS, watchOS 패키지의 최소 버전을 높입니다. `"14.2"`나 `"18.0"`처럼 1~3개의 숫자 부분으로 이루어진 따옴표로 감싼 버전 문자열을 쓰세요. 런타임 하한은 macOS 13.0(Go 1.27), iOS 15.0, tvOS 17.0, watchOS 26.0이며 이보다 낮거나 형식이 잘못된 값은 실패합니다. 모바일 키는 선택된 모든 모바일 타겟의 하한을 충족해야 합니다. App Clip은 별도의 `app_clip.minimum`을 사용합니다. 이 값들은 패키지 메타데이터, 애셋 컴파일, 서명 프로젝트, 앱 링커에 전달되며, 임베드된 iOS 프레임워크에도 적용됩니다(Mac Catalyst는 Go의 macOS 13 런타임을 위해 최소 16.0).

`icon.windows`와 `icon.linux`는 각 앱 패키지, 인스톨러 아트워크, 스토어 로고에 쓰일 정사각형 PNG를 지정합니다. 기본값은 `assets/IconWindows.png`와 `assets/IconLinux.png`이고, 없으면 `icon.png`로 폴백합니다. `dev.icon`도 동일한 설정과 `-dev` 기본 파일을 받습니다. `icon.android.png`는 어댑티브 레이어가 설정되지 않았을 때도 독립적으로 Android 비트맵을 선택합니다. 그 외의 타겟은 공용 `icon.png` 폴백을 유지합니다.

Apple TV 아이콘은 정사각형이 아니라 레이어가 있는 카드입니다. `tvos`는 Xcode의 `.brandassets`(App Icon 스택과 탑 셸프 이미지 포함) 또는 1280×768 같은 가로가 넓은 이미지를 받으며, 넓은 이미지를 주면 도구가 가장자리 색상 위에 카드에 맞춰 그리고 탑 셸프 이미지 가운데에 배치합니다. 둘 다 없으면 카드 높이의 4분의 3 크기로 PNG를 배치합니다. Apple Watch는 아이콘을 원형으로 마스킹하므로 `watchos`는 watchOS의 원형에 맞춰 그린 `.icon`이나 정사각형 PNG를 받으며, 없으면 `apple` 다음 `png`를 사용합니다. 두 설정 모두 `apple`, `png`와 마찬가지로 `assets/AppIconTV.*` 및 `assets/AppIconWatch.*`로 폴백합니다.

Android에서는 `icon.android.foreground`와 `background`가 어댑티브 런처 아이콘을 만드므로, 런처가 `png`를 자체 배경판 위에 축소하는 대신 원하는 모양으로 마스킹하고 움직일 수 있습니다. 각 레이어는 108 dp 정사각형(432 px로 스케일된 PNG)이며, 런처는 중앙 72 dp(288 px)를 보여 주고 중앙 66 dp(264 px) 바깥을 잘라내는 모양으로 마스킹할 수 있으므로, 로고는 그 원 안에 두고 배경은 정사각형 전체를 채우세요. `background`는 PNG 또는 `"#1F1F1F"` 같은 색상입니다. `monochrome`은 알파 채널만 사용하는 도형으로, Android 13 이상에서 사용자가 테마 아이콘을 켤 때 색조가 입혀지며, 없으면 기존 색상을 유지합니다. 패키지는 NSGo가 지원하는 가장 오래된 버전인 Android 9 이상에서 어떤 런처도 어댑티브 아이콘보다 우선 선택하지 않는 일반 비트맵 아이콘으로 `icon.android.png`(없으면 `icon.png`)를 함께 보관하며, `png`가 없으면 도구가 배경 위에 전경을 합성해 생성합니다. 어댑티브 레이어가 없으면 런처는 Android 비트맵이나 공용 `png`를 표시합니다.

`android.uses_cleartext_traffic: true`는 APK와 App Bundle이 모든 호스트에 대해 평문 HTTP를 사용하도록 매니페스트 플래그와 네트워크 보안 정책을 설정합니다. 이 옵션을 켜지 않은 상태에서 `android.loopback_cleartext: true`를 설정하면 앱 자체가 서빙하는 비디오처럼 `localhost`, `127.0.0.1`, `::1`에 한해 평문 HTTP를 허용합니다. 루프백 전용 정책은 다른 호스트에 대한 평문 통신을 차단하며, 두 설정이 모두 true면 전체 호스트 허용이 우선합니다. 임베드되는 AAR의 경우에는 호스트 앱의 Android 네트워크 보안 정책을 구성하세요.

모든 빌드는 시작 시 `--nsgo-fps` 같은 NSGo 자체 플래그도 받습니다([실행 및 디버깅](debugging.md#모든-빌드의-공통-플래그)). `nsgo dev`에서는 열려 있는 모든 창에서 Command/Control+R로 리로드할 수 있으며, 다른 창에서 앱 함수가 계속 실행 중이라면 메인 뷰가 닫힌 뒤에도 동작합니다.

### 실행 화면

iOS와 Android 12 이상에서 불투명 배경, 가운데 정렬된 PNG 또는 둘 다를 표시하려면 `launch_screen`을 설정하세요. 이미지 경로는 `nsgo.yaml` 기준 상대 경로입니다. 이미지는 128×128 포인트 또는 dp 안에 비율을 유지해 맞춰집니다. 가장 긴 변이 최소 512픽셀인 투명 PNG를 사용하면 고밀도 화면에서도 선명하게 보입니다.

```yaml
launch_screen:
  background: "#F5F1EA"
  image: assets/Launch.png
  dark:
    background: "#201C26"
    image: assets/LaunchDark.png
```

`appearance`로 고정하지 않는 한 시스템이 화면 모드를 선택합니다. `dark` 아래의 필드를 생략하면 대응하는 기본값을 재사용하고, 배경이나 이미지를 생략하면 플랫폼 기본값을 유지합니다. 임베드된 앱은 호스트로부터 실행 화면을 상속받습니다.

## Android 네이티브 클래스

`core.Native`에서 커스텀 View를 사용하려면 앱의 `nsgo.yaml`에 미리 컴파일된 Android 클래스를 나열하세요.

```yaml
android:
  classes: [java/BadgeView.jar, java/Helpers.dex]
```

절대 경로가 아니면 `nsgo.yaml` 기준 상대 경로입니다. 각 경로는 `.dex` 파일이나 D8이 생성한 루트 `classes.dex`, `classes2.dex` 및 후속 샤드를 담은 JAR를 가리킵니다. 해당 파일들은 D8에서 `--min-api 28` 옵션으로 준비하세요. arm64에서는 NSGo가 Go만으로 이 사전 컴파일된 파일들로부터 APK나 App Bundle을 빌드합니다. Android 개발 세션은 나열된 파일이 변경되면 다시 빌드합니다. 임베드되는 AAR의 경우에는 호스트의 Android 빌드가 Java 의존성을 제공합니다. [D8 문서](https://developer.android.com/tools/d8)

[Android 네이티브 예제](https://github.com/nsgo-dev/nsgo/tree/main/examples/android-native)는 공개 `Context` 생성자를 가진 클래스를 등록하고 Go prop으로 설정하는 방법을 보여 줍니다.

## 게임

게임인 경우 `nsgo.yaml`에 `game: true`를 설정하세요. Android는 앱을 게임으로 분류하고 시스템의 Game Mode 최적화를 사용합니다. macOS와 iOS는 Game Mode를 활성화하며, macOS는 기본적으로 Games 카테고리를 사용하고, Linux는 데스크톱 엔트리에 `Game`을 추가합니다. 명시적인 `macos.category`나 `apple.info`의 Game Mode 키가 있으면 그쪽이 우선합니다.

앱이 하드웨어 게임패드를 처리한다면 `gamepad: true`를 설정하세요. Android는 게임패드와 터치스크린을 모두 선택 사항(optional)으로 선언합니다. macOS, iOS, tvOS는 Extended Gamepad 프로필을 선언하며 tvOS는 Siri Remote의 Micro Gamepad도 함께 선언합니다. `apple.info`에 컨트롤러 키를 직접 적으면 그 값이 우선합니다.

## 라이선스 고지

`nsgo build`는 독립 패키지 리소스에 `THIRD_PARTY_NOTICES.txt`를 포함합니다. 여기에는 해당 타겟이 사용한 Go 모듈, Go 런타임, NSGo에 번들된 Java 또는 JavaScript 런타임의 라이선스 전문이 들어갑니다. 웹 빌드는 다른 리소스와 마찬가지로 콘텐츠 해시로 파일 이름을 정합니다. 앱 내 크레딧 화면에 같은 텍스트를 표시하려면 `core.ThirdPartyNotices()`를 사용하세요. 이 함수는 개발 모드, 테스트, 임베드된 라이브러리 빌드에서는 빈 문자열을 반환합니다. [네이티브 라이브러리 선언](native-libraries.md)은 해당 라이브러리의 원래 라이선스 전문을 이 고지에 추가합니다.

기본적으로 알 수 없거나 누락된 서드파티 라이선스는 빌드를 실패시키고, 카피레프트(copyleft) 라이선스가 감지되면 경고를 출력합니다. 앱의 Go 워크스페이스(`go.work`)에 나열된 모듈은 앱 자체 모듈과 함께 퍼스트파티 코드로 취급되므로, 포함된 라이선스 파일이 수집되긴 하지만 서드파티 검사를 유발하지는 않습니다. 두 규칙 모두 `error` 또는 `warn`으로 설정할 수 있습니다. `allow` 목록에 정확한 모듈 경로를 적으면 두 규칙 모두에서 면제되며 빌드가 찾은 라이선스 텍스트는 그대로 유지됩니다.

```yaml
licenses:
  unknown: error
  copyleft: warn
  allow: [example.org/reviewed-module]
```

## 라이브러리 추가

라이브러리, 즉 Go 모듈은 먼저 계획을 보여 주는 `go get`인 `nsgo add`로 추가합니다:

```sh
nsgo add github.com/acme/chart           # 최신 릴리스, go.mod에 이미 있으면 그대로
nsgo add github.com/acme/chart@v1.3.0    # 이 버전, go.mod의 버전보다 낮아도
nsgo add -dry-run github.com/acme/chart  # 계획만 출력하고 아무것도 쓰지 않음
```

```text
github.com/acme/chart
  module     github.com/acme/chart v1.3.0, published 2026-11-20
  owner      github.com/acme, the first module from it in go.mod
  next       import "github.com/acme/chart"

example.com/hello
  go.mod     + require github.com/acme/chart v1.3.0 // indirect
             + require example.com/units v1.0.0 // indirect
             ~ require github.com/nsgo-dev/nsgo v0.4.0 => v0.4.2, a release
  go.sum     + 6 lines
  tools      go tool nsgo then runs v0.4.2, and so does an installed nsgo here
Apply these changes? [y/N]
```

계획은 모듈마다 버전과 게시일, 그리고 소유자를 보여 줍니다. 소유자는 호스트이며, github.com 같은 코드 호스팅 서비스에서는 계정까지 포함하고, `go.mod`에 아직 그 소유자의 모듈이 없으면 그렇다고 표시합니다. 해당하는 경우 철회(retract)와 지원 중단(deprecation) 알림, 릴리스 태그가 없는 버전, 모듈의 `replace`, 루트 패키지의 import 줄이 이어집니다. 그다음에는 `go`와 `toolchain`을 포함해 바뀌는 `go.mod`의 줄, 다른 버전을 실행하게 될 `go tool` 명령, 자체 호스트에서 가져오는 모듈, 그리고 체크섬 데이터베이스의 확인 없이 `go.sum`이 받아들이는 모듈이 나옵니다. `go get`은 아직 어떤 패키지도 import하지 않는 모듈을 `// indirect`로 표시하며, 패키지가 import하면 `go mod tidy`가 이 표시를 없앱니다.

터미널에서는 `add`가 쓰기 전에 묻고, `-y`는 묻지 않고 계획을 적용합니다. 터미널도 `-y`도 없으면 아무것도 쓰지 않고 상태 1로 종료합니다. Ctrl+C 등으로 중단하면 `go.mod`와 `go.sum`은 그대로 두고 상태 130으로 종료합니다. `go.mod`가 이미 요구하는 모듈은 버전을 지정하지 않는 한 그대로 두고 버전만 출력합니다.

계획 수립은 `go.mod`와 `go.sum`의 사본에서 go 명령만 실행합니다:

- 모듈이 이미 사용하는 툴체인과 `GOTOOLCHAIN=local`로 실행하므로 다른 툴체인을 내려받거나 실행하지 않습니다.
- `GOPROXY`의 첫 번째 `direct` 앞에 나열된 모듈 프록시를 거치며, `direct`는 `off`가 되므로 `GOPRIVATE`나 `GONOPROXY`가 표시한 모듈만 자체 호스트에서 가져옵니다. `GONOSUMDB`와 `GOAUTH`는 그대로 둡니다.
- `GOWORK=off`로 실행하므로 워크스페이스 안에서도 `GOWORK=off go get`이 쓰는 대로 그 모듈만을 위한 `go.mod`를 계획하고, 그렇다고 알려 줍니다.

`-y` 여부와 관계없이 `add`는 다음을 거부합니다:

- NSGo 요구 사항을 의사 버전(pseudo-version)이나 프리릴리스로 옮기는 것. NSGo의 커밋을 요구하는 라이브러리가 그렇게 만듭니다. 의도적으로 옮기려면 먼저 `go get github.com/nsgo-dev/nsgo@<버전>`을 실행합니다.
- 모듈의 툴체인보다 새로운 Go가 필요한 모듈. 먼저 `go get go@<버전>`으로 모듈의 Go 버전을 올립니다.
- `@latest` 같은 쿼리로 모듈 버전을 낮추는 것. 버전을 직접 입력하면 낮출 수 있으며, 계획은 이를 다운그레이드라고 부릅니다.

계획 수립 이후 `go.mod`와 `go.sum`이 둘 다 바뀌지 않았을 때만 각각 임시 파일에 쓴 뒤 이름을 바꿔 덮어쓰며, 오류나 중단이 생기면 두 파일을 모두 원래대로 되돌립니다. `vendor` 디렉터리가 있는 모듈에서는 계획이 다음 단계로 `go mod vendor`를 알려 줍니다.

`add`는 NSGo 0.3.4 이후에 추가되었습니다. 0.3.4 또는 그 이전 릴리스를 고정한 모듈에서는 설치된 `nsgo`가 이 명령을 위해 모듈의 `nsgo`를 실행하지 않고, `add`를 가져오는 `go get github.com/nsgo-dev/nsgo@<버전>`을 출력합니다. 그런 모듈에서 `go tool nsgo add`는 `add`를 모르는 고정된 `nsgo`를 실행합니다.

## Go 패키지의 라이브러리

`base/terminal`을 임포트하는 앱은 `libghostty-vt`도 함께 배포합니다. Windows와 Linux 패키지는 실행 파일 옆에 라이브러리를 배치하고, macOS와 iOS 번들은 프레임워크 내부에 서명해 넣습니다. [임베딩](embedding.md)에서 iOS 호스트용 동반 XCFramework를 설명합니다.

의존 패키지는 `nsgo-natives.yaml`에 미리 빌드된 데스크톱 공유 라이브러리를 선언할 수 있으며, 그 버전 2부터는 iOS와 tvOS용 프레임워크도 선언할 수 있습니다. NSGo는 SHA-256을 검증하고 서명 전에 앱에 복사하며 인스톨러에도 포함하므로 앱 작성자가 별도 단계를 거칠 필요가 없습니다. [네이티브 라이브러리](native-libraries.md)에서 패키지 작성자용 포맷과 로딩 경로를 설명합니다.

## 커맨드

앱은 데스크톱 실행 파일 옆에 앱 데이터를 다루는 CLI 같은 커맨드라인 프로그램을 함께 배포할 수 있습니다. 각각 Go `main` 패키지에서 빌드되도록 `commands` 아래에 나열하세요:

```yaml
commands:
  - name: hello-cli             # the file's name, hello-cli.exe on Windows
    package: ./cmd/hello-cli    # a main package, built for each desktop target with CGO_ENABLED=0 and build.tags
    ldflags: [-X main.channel=beta]  # its own linker flags, after build.ldflags
windows:
  path: true                    # the installer puts the app's folder on the user's Path
```

이름은 영문자, 숫자, `-`, `_`만 사용할 수 있으며, 대소문자 구분 없이 앱 실행 파일, Linux 커맨드, `Uninstall`, `AppRun` 또는 다른 커맨드와 같은 이름일 수 없습니다. `macos_universal`은 앱과 마찬가지로 각 커맨드의 두 macOS 빌드를 하나로 합칩니다.

패키지는 `build.ldflags`와 `-ldflags`, 이어서 `-X` 값이 우선하는 자체 `ldflags`, 마지막으로 앱의 식별 정보 순으로 링크되므로, 커맨드도 앱과 마찬가지로 `core.App()`에서 `nsgo.yaml`의 이름, ID, 버전, 빌드 번호를 얻을 수 있습니다. `core`를 임포트하지 않는 커맨드는 앱과 커맨드 모두에 적용되는 `build`의 `-ldflags "-X main.version=1.2.0"` 같은 `-X` 플래그로 버전을 받습니다.

| 패키지 | 커맨드 |
| --- | --- |
| macOS | `Contents/MacOS` 안에 앱과 동일하게(`<id>.<command>`) 앱보다 먼저 서명됩니다. 애드혹 또는 `sign.identity`로 강화된 런타임(hardened runtime)과 타임스탬프를 붙여 서명하므로 앱 [공증](#공증)이 가능하지만 앱의 entitlement는 받지 않습니다. 디스크 이미지는 `PATH`에 아무것도 추가하지 않으며, [Homebrew cask](#채널)가 각 커맨드를 링크하고 사용자가 직접 `/Applications/<name>.app/Contents/MacOS/<command>`를 링크할 수도 있습니다 |
| Windows | `<name>.exe` 옆에 `<command>.exe`로 위치하며 똑같이 [서명](#windows-패키지-서명)되고 인스톨러가 설치하며 언인스톨러가 제거합니다. `windows.path`를 설정하면 `windows.command_line`과 마찬가지로 인스톨러가 폴더를 사용자의 `Path`에 추가하므로 설치 후 연 콘솔에서 커맨드를 찾을 수 있습니다. 업데이트 시 이름이 바뀌거나 제외된 커맨드는 앱이 더 이상 배포하지 않는 파일과 마찬가지로 폴더에 남는데, 언인스톨러가 해당 버전이 설치한 것만 제거하기 때문입니다 |
| Linux | `/opt/<slug>`에 위치하며 `.deb`와 Arch 패키지가 소유하는 `/usr/bin/<command>` 링크를 만드므로 패키지를 제거하면 링크도 사라지고 dpkg와 pacman이 다른 패키지의 파일을 덮어쓰지 못하게 막습니다. AppImage는 앱 옆에 커맨드를 보관하며 `PATH`에는 아무것도 추가하지 않습니다 |

인스톨러, 체크섬, [업데이트](#업데이트) 앱캐스트는 앱의 다른 파일들과 함께 커맨드도 포함하므로 업데이트 시 앱과 함께 교체됩니다. [스토어 패키지](#스토어-패키지)는 Mac App Store 샌드박스, MSIX, Flatpak이 앱 자체 실행 파일만 실행하므로 커맨드를 제외합니다.

## 인스톨러

데스크톱 타겟에 대해 `build`는 사용자가 앱을 설치할 수 있는 파일을 `dist/releases`에 작성하며 각각 `.sha256` 체크섬을 함께 생성합니다:

| 타겟 | 인스톨러 |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.dmg`: 앱, 화살표, 드래그해 넣을 Applications 링크를 표시하는 디스크 이미지로, `sign.identity`로 서명하고 [공증](#공증)합니다 |
| Windows | `<name>-<version>-windows-<arch>-setup.exe`: 사용자 단위로 설치하고, 재설치 시 사용자가 이전에 고른 폴더에 설치하며, 실행 중인 앱을 종료하고, 시작 메뉴 바로가기와 선택적 바탕화면 바로가기, 그리고 자신이 설치한 파일만 지우는 언인스톨러를 포함하는 네이티브 Windows 인스톨러입니다. 앱이 Windows 패키지에 서명할 때 [`sign.certificate`로 함께 서명](#windows-패키지-서명)합니다. `windows.command_line`은 `.com` 동반 파일도 설치하고 앱 폴더를 사용자의 `Path`에 추가하므로 설치 후 연 콘솔에서 앱 커맨드를 실행할 수 있고, Win+R 역시 동반 파일을 찾아 앱을 시작하고 열었던 콘솔을 닫습니다. 언인스톨러는 인스톨러가 추가했던 경우에만 `Path`에서 폴더를 빼며 기존 항목은 건드리지 않습니다 |
| Linux | Debian/Ubuntu용 `<name>-<version>-linux-<arch>.deb`와 Arch Linux용 `<name>-<version>-linux-<arch>.pkg.tar.gz`는 `/opt/<slug>`에 앱, 커맨드, [커맨드](#커맨드) 링크, 데스크톱 엔트리, 아이콘을 설치하며(Arch 패키지는 `pacman -Qkk`용 `.MTREE` 포함), `<name>-<version>-linux-<arch>.AppImage`는 설치 없이 어떤 배포판에서도 실행됩니다 |

Windows 셋업과 언인스톨러는 어떤 시스템에서든 Go만으로 빌드되며 네이티브 Windows 대화 상자를 사용합니다. 앱 카탈로그의 언어 중 자체 번역이 있는 언어(간체/번체 중국어 포함)를 지원하며 시스템 UI 언어를 선택합니다. `/LANG=<LCID>`로 한 번의 실행에 사용할 언어를 고를 수 있으며, 예를 들어 간체 중국어는 `/LANG=2052`입니다. `/S`는 무인 설치 또는 제거를 수행하고, 마지막 인수로 지정한 `/D=<folder>`는 공백이 포함된 경로를 포함해 설치 폴더를 선택하며, `/RELAUNCH`는 업데이트 후 앱을 엽니다. `/D`가 없으면 업그레이드는 교체되는 기존 설치의 폴더에 설치됩니다. `-installers=false` 또는 `build.installers: false`는 패키지 파일만 빌드합니다.

인스톨러는 앱 고유의 이미지와 아이콘을 사용할 수 있으며, 지정하지 않으면 앱 아이콘이나 내장 이미지를 사용합니다:

```yaml
macos:
  dmg_background: assets/DMGBackground.png  # the disk image's window, which takes its size in points
  dmg_icon: assets/DMGIcon.png              # the disk image's icon: an .icns or a square PNG
windows:
  installer_icon: assets/Setup.png          # the setup program's icon: an .ico or a square PNG
  uninstaller_icon: assets/Uninstall.png    # the uninstaller's
```

배경 PNG와 함께 2배 크기 파일(예: `DMGBackground@2x.png`)을 두면 Retina 화면에서도 선명합니다. 앱 아이콘은 왼쪽에서 4분의 1 지점, Applications는 오른쪽에서 4분의 1 지점에 각각 이름이 위아래 가운데에 오도록 놓이며 내장 이미지는 그 사이에 화살표를 그립니다. 직접 만든 배경을 쓸 때는 그 자리를 비워 두세요. Linux 패키지에는 별도 이미지가 없고 앱 아이콘만 설치합니다.

Linux 패키지는 소프트웨어 센터가 앱을 나열할 때 읽는 AppStream 메타데이터(`/usr/share/metainfo/<id>.metainfo.xml`)도 설치합니다. 이는 `linux.summary`, 설정된 경우 `linux.license`, 앱 이름들로 작성되며 `linux.categories`가 비어 있으면 카테고리를 `Utility`로 둡니다. AppImage는 이를 AppDir의 `usr/share` 안에 두고, 메타데이터가 가리키는 `usr/share/applications`의 데스크톱 엔트리 링크 옆에 둡니다. 더 풍부한 메타데이터, 추가 데스크톱 엔트리, 셸 자동완성, 매뉴얼 페이지처럼 시스템이 `/usr/share`에서 읽는 파일들은 `nsgo.yaml` 기준 상대 경로로 `linux.share`가 가리키는 디렉터리에 넣으세요. `.deb`와 Arch 패키지는 그 트리를 `/usr/share` 아래에, AppImage는 AppDir의 `usr/share` 아래에 설치합니다. 그 안의 `metainfo/<id>.metainfo.xml`은 NSGo가 생성하는 파일을 대체하며, 앱의 데스크톱 엔트리나 아이콘처럼 패키지가 직접 작성하는 경로에 파일이 겹치면 빌드가 실패합니다. `resources/`의 파일은 이름과 무관하게 `/opt/<slug>`의 실행 파일 옆에 남고, 스토어 빌드의 Flatpak은 NSGo가 생성한 메타데이터만 사용합니다.

AppImage는 대부분의 배포판에 있는 FUSE가 필요하며 패키지들과 마찬가지로 시스템의 GTK 4를 사용합니다. 런타임은 사용자 캐시에 한 번만 다운로드됩니다.

AppImage는 `release.github` 또는 Git 리모트 `origin`을 사용해 최신 안정 릴리스와 현재 앱 아키텍처를 선택하는 `gh-releases-zsync` 로케이터를 임베드합니다. 각 `.AppImage`를 생성된 `.AppImage.zsync` 파일과 함께 업로드하세요. 다른 호스트를 쓰려면 `linux.appimage_update`를 HTTP range를 지원하는 안정 URL인 `zsync|https://example.com/latest.AppImage.zsync`로 설정하세요. 프리릴리스 채널용으로 `gh-releases-zsync|owner|repo|latest-pre|name-*-linux-x64.AppImage.zsync`도 지정할 수 있습니다. 명시적 빈 문자열을 주면 임베드된 탐색을 비활성화하며, GitHub 저장소나 설정된 로케이터가 없을 때도 비어 있습니다. `.zsync` 파일은 서명된 앱캐스트와 별개로 계속 생성됩니다.

## 스토어 패키지

`-channel store` 또는 `updates.channel: store`를 설정하면 인스톨러와 설치 스크립트 대신 자체적으로 업데이트를 제공하는 스토어용으로 데스크톱 및 Android 타겟을 빌드합니다:

| 타겟 | 스토어 패키지 |
| --- | --- |
| macOS | Mac App Store용 `<name>-<version>-macos-<arch>.pkg`: App Sandbox 내의 앱으로, 프로비저닝 프로필의 인증서로 서명하고 팀의 인스톨러 인증서로 패키징합니다 |
| Windows | Microsoft Store용 `<name>-<version>-windows-<arch>.msix`: 게시 시 스토어가 서명합니다 |
| Linux | 설치 가능한 `<name>-<version>-linux-<arch>.flatpak`과 함께, GNOME 런타임 기반 Flathub 또는 `flatpak-builder`용 AppStream이 포함된 `.tar.gz` 및 `flatpak/<id>.yml` |
| Android | `sign.keystore`(업로드 키)로 서명된 Google Play용 App Bundle `<name>-<version>-android-<arch>.aab` |

MSIX는 각 언어의 표시 이름을 카탈로그의 `appName`에서 읽고 없으면 소스 언어로 폴백합니다. 시작 메뉴, 작업 표시줄, 스토어 로고에는 100%, 125%, 150%, 200%, 400% 배율이 포함됩니다. 도구가 macOS에서 리소스 인덱스를 직접 작성하므로 Windows SDK가 필요하지 않습니다.

스토어 채널에서 `-target macos`를 지정하면 `-target macos_universal`과 마찬가지로 두 macOS 아키텍처를 모두 선택해 하나의 `<name>-<version>-macos-universal.pkg`를 생성합니다. `arm64-v8a`와 `x86_64`를 모두 담은 하나의 `<name>-<version>-android-universal.aab`를 만들려면 `-target android_arm64,android_x64`를 선택하세요. x86_64 슬라이스에는 Android NDK가 필요합니다.

```yaml
macos:
  category: public.app-category.productivity  # LSApplicationCategoryType, which the Mac App Store requires
  # entitlements: { com.apple.security.files.downloads.read-write: true }  # beyond the network, the files the user picks and what apple.info explains
windows:
  store:                               # as Partner Center's Product identity lists them
    identity: 12345Example.Hello
    publisher: CN=00000000-0000-0000-0000-000000000000
    publisher_name: Example
  # device_capabilities: [microphone, webcam]  # the devices the app uses, which Windows asks the user for
linux:
  summary: Says hello                  # one line for software centers
  license: MIT                         # an SPDX expression; LicenseRef-proprietary by default
  # flatpak_runtime: "51"              # GNOME's runtime version
android:
  target_sdk: 36                       # the targetSdkVersion Google Play requires
sign:
  macos_profile: assets/MacAppStore.provisionprofile  # the Mac App Store's profile
  # installer: "3rd Party Mac Developer Installer: Example (ABCDE12345)"  # by default the keychain's of the profile's team
```

`android.target_sdk`가 설정되지 않으면 런타임은 API 36을 타겟팅합니다. 2026년 8월 31일부터 [Google Play가 요구](https://support.google.com/googleplay/android-developer/answer/11926878)하는 신규 모바일 앱 및 업데이트 기준인 API 36 미만에서는 Android App Bundle 빌드가 실패합니다. 독립 실행형 APK 빌드는 경고만 출력하고 계속 진행합니다.

Mac 패키지를 업로드용으로 서명하려면 `sign.macos_profile`을 설정하세요. 위젯이 없다면 서명되지 않은 패키지도 샌드박스 안에서 실행되므로 앱 컨테이너, 네트워크, 사용자가 선택한 파일에 대한 접근을 테스트할 수 있습니다. 위젯이 포함된 스토어 빌드에서는 `<id>`용 `sign.macos_profile`과 같은 팀 소속이며 `apple.app_group`(기본값 `group.<id>`)을 부여하는 `<id>.widgets`용 macOS `sign.widgets_profile`이 모두 필요합니다. 두 번들 모두 해당 공유 컨테이너를 사용하며 홈 디렉터리 예외는 추가되지 않습니다. `-installers=false`를 주면 `.pkg`와 인스톨러 인증서는 생략하되 샌드박스와 프로필 서명은 유지합니다. `.msix`는 Partner Center에, `.aab`는 Play Console에 업로드하세요. 릴리스에 명시된 아카이브가 올라가면 `flatpak/<id>.yml`을 Flathub에 제출하세요. 로컬에서도 `flatpak-builder`로 빌드할 수 있으며, `flatpak install --user ./<name>-<version>-linux-<arch>.flatpak`으로 번들을 설치할 수 있습니다(일치하는 GNOME 런타임이 설치되어 있거나 설정된 리모트에서 받을 수 있어야 함). 빌드는 Linux 툴체인 없이 Go로 번들을 작성합니다. 스토어 패키지는 아트워크가 없을 때 투명 아이콘을 사용하고 `linux.categories`가 없으면 Utility를 사용합니다. 스토어 외부에서 설치하도록 MSIX에 서명하려면 [`sign.certificate`](#windows-패키지-서명)를 설정하세요. 인증서 주체가 `windows.store.publisher`와 일치해야 하며 설치 대상 머신이 인증서를 신뢰해야 합니다. 서명 설정을 비워 두면 업로드된 패키지에 스토어가 서명합니다.

macOS와 iOS에서 스크롤 뷰의 가장자리 효과(`Scroll.EdgeEffects`), `IOS.BackdropMaterial`이 없는 macOS 배경 블러, 그리고 배경 컬러 필터(단독 사용 또는 `IOS.BackdropMaterial`이 없는 블러 위에 사용)는 반경을 지정해 블러를 주거나 뷰 전체에서 블러를 변화시키거나 배경 컬러 행렬을 적용하는 공개 API가 없기 때문에 AppKit과 UIKit의 머티리얼·가장자리 효과와 마찬가지로 Core Animation의 비공개 `CABackdropLayer` 및 `CAFilter`를 통해 그립니다. App Store 심사에서 비공개 API 사용을 이유로 앱을 거부할 가능성이 있습니다. NSGo는 가장자리 바가 있는 스크롤 뷰와 해당 배경 필터에만 이를 호출하지만, 모든 macOS 및 iOS 빌드 바이너리에는 해당 심볼 이름이 포함됩니다.

`build`가 작성하는 모든 Apple 번들(앱, 위젯 확장, App Clip, Apple Watch 앱, `-dynamic` `-embed` 프레임워크)에는 개인정보 보호 매니페스트인 `PrivacyInfo.xcprivacy`가 포함되어 NSGo와 Go 런타임이 사용하는 필수 사유 API(user defaults `CA92.1`, file timestamps `C617.1`, system boot time `35F9.1`)를 선언합니다. `apple.privacy`에는 앱 자체의 `accessed_apis`(카테고리별 사유), `tracking`, `tracking_domains`, `collected_data`(Apple이 정한 `NSPrivacyCollectedDataTypes` 항목)를 추가할 수 있으며, `resources/`에 있는 `PrivacyInfo.xcprivacy`는 덮어쓰이지 않고 병합됩니다.

`build`가 기기용으로 작성하는 각 iOS, tvOS, watchOS 번들(앱, 위젯 확장, App Clip)의 `Info.plist`에는 빌드에 사용된 Xcode에서 읽어 온 SDK 및 Xcode `DT` 키(App Store Connect가 확인함)와 `UIRequiredDeviceCapabilities: [arm64]`가 들어갑니다. 시뮬레이터 번들에서는 제외되며 `apple.info`가 설정한 키는 기존 값을 유지합니다. `apple.multiple_scenes: true`는 iOS 앱의 씬 매니페스트에서 기본값이 false인 `UIApplicationSupportsMultipleScenes`를 활성화합니다. 그러면 사용자가 iPad에서 앱 창을 여러 개 열 수 있고, 각 창은 닫을 때까지 앱의 메인 뷰를 새로 표시하며 독립된 상태와 앱의 공유 시그널 및 스토리지를 사용합니다.

### Google Play에 게시

Google Play Developer API를 활성화하고 선택한 트랙에 앱을 릴리스할 권한을 서비스 계정에 부여하세요. 계정의 JSON 키 또는 키 파일 경로를 `NSGO_PLAY_SERVICE_ACCOUNT`에 설정합니다. [Google Play API 설정](https://developers.google.com/android-publisher/getting_started)

```sh
go tool nsgo build -target android -channel store
go tool nsgo publish -target android -track internal
```

이 명령은 `build.output/releases`에서 프로젝트 버전에 맞는 App Bundle을 찾습니다. 빌드가 여러 개일 때는 플래그 뒤에 `.aab` 경로를 넘겨 파일을 선택하세요. 트랙에는 `internal`, `production` 또는 비공개 테스트의 트랙 ID를 지정합니다. 명령은 번들에서 패키지와 버전 코드를 읽어 해당 트랙의 모든 기존 릴리스보다 높은 코드인지 확인한 뒤 번들을 업로드하고 릴리스를 커밋합니다.

## 스토어 등록정보

스토어 메타데이터(제목, 부제, 설명, 키워드, 릴리스 노트, URL, 스크린샷, 미리보기)는 저장소의 `store/`(또는 `nsgo.yaml`의 `store.dir`) 아래에 평문 YAML과 Markdown으로 저장됩니다:

```text
store/
  listing.yaml              # version: 1, default_locale, app_store and google_play settings
  en-US/
    listing.yaml            # version: 1, title, subtitle, short_description, keywords, URLs, media
    description.md          # full description (optional file in place of YAML description)
    release_notes.md        # what's new (optional file in place of YAML release_notes)
    promotional_text.md     # App Store promotional text (optional file)
    screenshots/
  zh-Hans/
    listing.yaml
    description.md
    release_notes.md
    screenshots/
```

두 `listing.yaml` 파일 모두 `version: 1`이 필수이며 알 수 없는 키를 거부합니다. `zh-Hans` 같은 표준 로케일 디렉터리는 `app_store.locale`이나 `google_play.locale`로 스토어별로 재정의하지 않는 한 각 스토어의 코드(App Store Connect에서는 `zh-Hans`, Google Play에서는 `zh-CN`)로 자동 매핑됩니다.

```sh
go tool nsgo publish -store all -dry-run                  # validate lengths, locales and media without uploading
go tool nsgo publish -store appstore,play                 # validate and upload to App Store Connect and Google Play
go tool nsgo publish -store play -track production        # also update release notes on the named Play track
```

유효성 검사는 자격 증명을 읽거나 HTTP 요청을 보내기 전에 먼저 실행되어 필수 필드, 로케일 코드, URL 스킴, 미디어 파일과 개수(App Store 디스플레이 유형당 스크린샷 최대 10개 및 미리보기 최대 3개, Google Play 이미지 유형당 스크린샷 최대 8개), 글자 수 제한(App Store: 제목 30, 부제 30, 키워드 100, 프로모션 텍스트 170, 설명 4000, 릴리스 노트 4000 / Google Play: 제목 30, 간단한 설명 80, 설명 4000, 릴리스 노트 500)을 확인합니다.

업로드 자격 증명은 오직 환경 변수에서만 읽어옵니다. App Store Connect에는 `NSGO_APP_STORE_KEY`, `NSGO_APP_STORE_KEY_ID`, `NSGO_APP_STORE_ISSUER`(없으면 공증 키 `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID`, `NSGO_NOTARY_ISSUER`로 폴백)를, Google Play에는 `NSGO_PLAY_SERVICE_ACCOUNT`(서비스 계정 JSON 파일 경로 또는 JSON 텍스트)를 사용합니다.

### 스토어 컴플라이언스

`nsgo.yaml`을 사용해 각 릴리스 설문지의 완료 상태를 추적할 수 있습니다.

```yaml
compliance:
  app_store: { age_rating: false }
  google_play: { data_safety: false }
  steam: { content_survey: false }
```

설문지를 완료한 뒤 해당 항목을 `true`로 설정하세요. 일치하는 `-channel store` 또는 `-channel steam` 빌드는 `false`로 남아 있는 항목에 대해 알림을 출력합니다. YAML에서 추적하지 않을 항목은 생략해도 됩니다.

- App Store Connect에서 [연령 등급 설문지](https://developer.apple.com/help/app-store-connect/manage-app-information/set-an-app-age-rating)와 [앱 개인정보 보호 세부 정보](https://developer.apple.com/help/app-store-connect/manage-app-information/manage-app-privacy)를 작성하세요.
- Apple의 [암호화 설문지](https://developer.apple.com/help/app-store-connect/manage-app-information/determine-and-upload-app-encryption-documentation)의 경우 `apple.info.ITSAppUsesNonExemptEncryption`을 YAML 불리언으로 설정하세요. Apple에서 승인 코드를 발급받았다면 `apple.info.ITSEncryptionExportComplianceCode`에 문자열로 넣으세요. 키를 생략하고 업로드 시 직접 답변할 수도 있습니다. Apple 스토어 빌드는 이 키들의 타입과 번들의 개인정보 보호 매니페스트 구조를 검사합니다.
- Play Console에서 [데이터 보안](https://support.google.com/googleplay/android-developer/answer/10787469) 섹션을 작성하세요. 내부 테스트 트랙으로만 배포되는 앱은 면제됩니다.
- 심사를 위해 게임을 제출하기 전에 Steamworks에서 [콘텐츠 설문조사](https://partner.steamgames.com/doc/gettingstarted/contentsurvey)를 완료하세요.

## App Store Connect

App Store Connect에서 앱을 생성하고, App Store 배포 프로필로 iOS 기기 IPA를 서명하거나 팀의 인스톨러 인증서로 macOS 스토어 PKG를 서명하세요. Xcode가 있는 Mac에서 `NSGO_APP_STORE_KEY`에 App Store Connect 팀 키 `.p8` 파일 경로 또는 내용을, `NSGO_APP_STORE_KEY_ID`에 키 ID를, `NSGO_APP_STORE_ISSUER`에 발급자 ID를 설정하세요. 이 값들이 없으면 스토어 등록정보와 마찬가지로 공증 키(`NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID`, `NSGO_NOTARY_ISSUER`)를 사용합니다. 해당 키에 앱 접근 권한과 빌드 업로드 권한을 부여하세요.

```sh
go tool nsgo publish ios path/to/Hello.ipa
go tool nsgo publish macos path/to/Hello.pkg
```

이 명령은 업로드 전 번들의 빌드 번호를 확인한 뒤 TestFlight에 대한 Apple의 처리 결과를 보고합니다. 다음 업로드를 위해서는 `build_number`를 올려야 합니다. macOS 빌드 번호는 모든 앱 버전에 걸쳐 계속 증가해야 하지만, iOS는 새 버전마다 번호를 다시 시작할 수 있습니다. 처리가 끝난 빌드는 App Store Connect에서 별도로 앱 심사에 제출하세요. [Apple의 배포 가이드](https://developer.apple.com/documentation/xcode/preparing-your-app-for-distribution)

## 스토어 애셋 팩

`mobile.asset_packs`에 대용량 애셋 디렉터리를 선언하세요. 해당 디렉터리 안의 파일은 기본 Android App Bundle 및 iOS 앱과 분리되어 전송됩니다. 팩 이름은 소문자로 시작해야 하며 소문자, 숫자, 밑줄을 포함할 수 있습니다.

```yaml
mobile:
  asset_packs:
    - {name: levels, dir: resources/levels, delivery: install-time}
    - {name: voices, dir: large-assets/voices, delivery: fast-follow}
```

`go tool nsgo build -channel store -target android_arm64`로 Android 팩을 빌드하세요. 기기에서 번들을 테스트하려면 `bundletool build-apks --local-testing`과 `bundletool install-apks`를 사용하세요. NSGo는 설치된 `install-time` 팩을 열 수 있습니다. `fast-follow` 모듈은 Play 배포용으로 포함되지만 Play Asset Delivery SDK에서 해당 팩을 읽는 기능은 아직 구현 중입니다.

iOS에서는 `go tool nsgo build -target ios`가 `install-time`을 Apple의 essential 정책으로, `fast-follow`를 prefetch로 매핑하여 첫 설치와 업데이트 모두에 적용합니다. Xcode 26 이상이 Background Assets 다운로더를 빌드합니다. 공유 `apple.app_group` 또는 기본 `group.<id>`를 사용하세요. 앱의 `sign.profile`과 `<id>.assetdownloader` 확장의 `sign.asset_packs_profile` 모두 해당 그룹이 필요합니다. `dist/releases`의 `.aar` 파일들을 App Store Connect의 애셋 팩 업로드 워크플로를 통해 IPA와 함께 업로드하세요. [관리형 애셋 팩 만들기](https://developer.apple.com/documentation/backgroundassets/creating-managed-asset-packs)

고루틴에서 `mobile.AssetPackReady(ctx, "levels")`를 호출해 준비 여부를 확인하고 `mobile.OpenAssetPackFile("levels", "intro.map")`으로 선언된 디렉터리 기준 상대 경로의 파일을 여세요. 사용 후 반환된 리더를 닫아야 합니다. [Apple 애셋 팩 로컬 테스트](https://developer.apple.com/documentation/backgroundassets/testing-asset-packs-locally)

## Steam 디포

```sh
go tool nsgo build -channel steam -target macos_universal,windows_x64,linux_x64
```

기본으로 이 채널을 사용하려면 `updates.channel: steam`을 설정하세요. macOS, Windows, Linux 타겟을 선택합니다. 각 `dist/<name>-<version>-<target>` 디렉터리를 해당하는 Steam 디포(depot)에 업로드하고, 실행 옵션으로 macOS에서는 앱 번들을, Windows와 Linux arm64에서는 실행 파일을, Linux x64에서는 `steam-launch.sh`를 지정하세요. 업데이트는 Steam이 처리합니다. 디렉터리에는 인스톨러, 자체 업데이터, `steam_appid.txt`가 포함되지 않습니다.

Linux x64 디포에는 Steam Linux Runtime 3.0 및 4.0용 GTK 4와 의존 라이브러리가 포함되며, 런처는 시스템에 GTK 4.14 이상이 없을 때만 이를 사용합니다. 런처 옆에 `nsgo-gtk/`을 그대로 두세요. 그 안의 `licenses/`에는 GLib, Pango, GTK에 대한 LGPL 요구사항인 라이브러리 라이선스, 빌드된 소스 코드, 빌드 스크립트가 들어 있습니다. Linux arm64는 런타임에 GTK 4가 설치되어 있어야 합니다.

Steam 클라이언트 외부에서 로컬로 테스트하려면 `nsgo.yaml`에 `steam: {app_id: 480}`을 설정하되 480 대신 여러분의 Steamworks App ID를 넣으세요. `go tool nsgo dev`는 네이티브 빌드 디렉터리의 `steam_appid.txt`에 이 ID를 기록하고 해당 디렉터리를 작업 디렉터리로 삼아 앱을 실행합니다. [Steamworks API](https://partner.steamgames.com/doc/api/steam_api), [SteamPipe](https://partner.steamgames.com/doc/sdk/uploading).

## GitHub에 게시

빌드에 macOS, Windows 또는 Linux 타겟이 포함되어 있으면 `dist/releases`에는 `install.sh`와 `install.ps1`도 들어갑니다. 이 스크립트들은 `release.github`가 지정한 GitHub 저장소(없으면 앱의 Git 리모트 `origin` 저장소)의 릴리스에서 앱을 설치하며, 둘 다 없으면 `build`는 스크립트를 생성하지 않고 그 사실을 알립니다. `miniprogram/`을 제외한 최상위 파일들을 `v<version>` 태그의 릴리스에 업로드하세요(`release.tag_prefix`로 `v`를 바꿀 수 있으며 스크립트는 접두사의 URL 예약 문자를 인코딩합니다):

```sh
gh release create v1.2.0 dist/releases/*.*
```

그러면 사용자는 빌드가 출력해 주는 한 줄 명령으로 앱을 설치하거나 업데이트할 수 있습니다:

```sh
curl -fsSL https://github.com/me/hello/releases/latest/download/install.sh | sh    # macOS and Linux
```

```powershell
irm https://github.com/me/hello/releases/latest/download/install.ps1 | iex         # Windows
```

macOS에서는 스크립트가 디스크 이미지에서 `/Applications`(쓰기 권한이 없으면 `~/Applications`)로 앱을 복사합니다. Linux에서는 `sudo`를 통해 `pacman`으로 Arch 패키지를, 없으면 `apt-get`이나 `dpkg`로 Debian 패키지를 설치합니다. Windows에서는 인스톨러를 무인(silent)으로 실행해 이미 앱이 설치된 위치에 설치합니다. 각 스크립트는 먼저 체크섬과 대조해 파일을 검증합니다. 앱 이름을 대문자로 바꾼 `<NAME>_VERSION`(예: `HELLO_VERSION=1.1.0`)으로 다른 버전을 설치할 수 있으며, `<NAME>_DOWNLOAD_URL`로 미러에서 다운로드할 수 있습니다.

## itch.io에 게시

`nsgo.yaml`의 `itch.project`에 itch.io 게임의 `owner/game` 슬러그를 설정하세요. 채널 이름을 재정의하려면 빌드 타겟별로 `itch.channels`를 설정합니다. 기본값은 웹 `html5`, macOS `osx-<arch>`, Windows `windows-<arch>`, Linux `linux-<arch>`입니다.

```yaml
itch:
  project: example/my-game
  channels:
    macos_arm64: osx-arm64-beta
```

CI 시크릿에 `BUTLER_API_KEY`를 설정한 뒤 타겟을 빌드하고 업로드하세요.

```sh
go tool nsgo build -channel none -installers=false -target web,macos_arm64
go tool nsgo publish itch -target web,macos_arm64
```

`-output`이나 `-version`을 재정의할 때는 두 명령에 같은 값을 전달하세요. 업로드는 각 타겟의 패키지 디렉터리를 가져가 itch.io 버전을 빌드 버전으로 설정합니다. NSGo는 `PATH`에 있는 `butler`를 사용하거나 itch.io에서 고정된 버전을 다운로드합니다. 첫 번째 웹 업로드 후에는 itch.io 게임 타입을 HTML로 설정하고 브라우저에서 플레이 가능하도록 표시하세요. [Butler 문서](https://itch.io/docs/butler/pushing.html)

## 업데이트

`updates`는 플랫폼과 사용자가 앱을 받은 채널의 방식에 따라 앱이 스스로 최신 상태를 유지하도록 합니다:

```yaml
updates:
  key: zE0hx2FZwwHy6X7LbEP3GjTTE3KWfCAp5gsaBysC+vg=  # the public key nsgo keygen prints
  # channel: store                    # direct, store, steam or none; direct by default with a key, none without
  # feed: https://example.com/hello/  # where the appcasts and installers are; by default the latest GitHub release
  # automatic: false                  # check only when asked; true by default
  # automatic_downloads: true          # Sparkle downloads automatically; false by default, macOS only
```

`nsgo keygen`은 파일을 쓰지 않고 키 쌍을 출력합니다. 공개 키는 `updates.key`에 넣어 커밋하고, 비밀 키는 저장소 밖에 두고 비밀번호 관리자 및 릴리스를 빌드하는 환경(예: GitHub Actions 시크릿)의 `NSGO_UPDATES_KEY`에 보관하세요. 각 direct 빌드는 비밀 키로 인스톨러에 서명하고 옆에 [Sparkle](https://sparkle-project.org) 포맷의 피드인 `appcast-<target>.xml`을 생성합니다. 앱은 이를 `feed` 아래에서 읽으므로 인스톨러와 함께 앱캐스트를 업로드하세요. 비밀 키가 없으면 macOS와 Windows는 서명된 인스톨러만 설치하므로 해당 앱캐스트를 작성하지 않으며, Linux와 Android 앱은 서명되지 않은 앱캐스트를 읽으면 패키지를 설치하는 대신 릴리스 페이지를 엽니다.

`-release-notes notes.md`를 지정하면 체인지로그의 한 섹션 같은 Markdown 릴리스 노트 파일을 각 앱캐스트에 넣으며, 파일 자체의 raw HTML은 제외한 채 HTML로 렌더링합니다. Sparkle과 WinSparkle은 업데이트 대화상자에 이를 표시하고, Linux와 Android의 프롬프트는 버전 아래에 최대 1,500자의 텍스트로 표시합니다. 앱에서는 `core.UpdateCheck.Notes`로 읽을 수 있습니다.

| 타겟 | direct 빌드의 업데이트 방식 |
| --- | --- |
| macOS | `build`가 앱에 포함하는 Sparkle이 매일 확인하고 새 버전을 안내하며 디스크 이미지를 다운로드해 서명을 검증한 뒤 설치하고 앱을 재시작합니다. 기본 앱 메뉴에는 Check for Updates…가 있으며 메뉴를 직접 구성하는 앱은 `updater.Check`로 추가합니다 |
| Windows | 실행 파일 옆의 WinSparkle이 동일한 작업을 수행하며, WinSparkle이 지원하는 언어일 경우 앱이 시작될 때 표시하는 언어를 사용하고, 인스톨러를 무인 실행해 새 버전을 시작합니다 |
| Linux, Android | 앱이 앱캐스트를 읽고 사용자가 업데이트를 선택하면 서명된 패키지를 디스크로 직접 스트리밍하며 검증하고 진행률을 보고한 뒤(`updater.Status` 및 플랫폼 작업 진행률 표시기), `pkexec dpkg`, `pkexec pacman`(`.pkg.tar.gz`/`.pkg.tar.zst`), 원자적 AppImage 교체(Linux에서는 재시작) 또는 Android 패키지 설치 프로그램으로 설치합니다. 이 외의 포맷이나 서명되지 않은 앱캐스트는 대신 릴리스 페이지를 엽니다 |
| iOS | 앱이 App Store에 업데이트를 확인하고 스토어의 앱 페이지를 엽니다 |

게시된 독립 실행형 웹 앱에서는 `updater.Check(cx)`가 실행 중인 릴리스를 새 `nsgo.json`과 비교해 확인 후 새로 고침을 제안하며, 같은 버전 레이블 아래에서 빌드 파일이 바뀐 것도 감지합니다. 앱캐스트 키는 필요하지 않으며 명시적 `none`/`steam` 채널은 여전히 확인을 비활성화하고, 임베드된 호스트는 자체적으로 리로드를 관리합니다. 미니 프로그램은 플랫폼의 업데이트 관리자를 사용하며 다운로드된 업데이트가 준비되었을 때만 재시작합니다. 이 타겟들은 앱이 업데이터를 호출할 때 확인하며, 미니 프로그램의 다운로드/확인 시점은 호스트에 속하고 자동 확인 설정 세터도 제공하지 않습니다. 커스텀 `cx.CheckForUpdates` 핸들러는 버전 레이블이 없을 수도 있는 준비된 업데이트에 대해 `UpdateCheck.Restart`를 사용합니다.

### 채널

채널은 누가 앱을 업데이트할지 결정합니다. `-channel`은 한 번의 빌드에 대해 `updates.channel`을 대체하므로 하나의 프로젝트로 채널마다 빌드할 수 있습니다:

| 채널 | 대상 | 패키지 |
| --- | --- | --- |
| `direct` | GitHub, 웹 사이트, Homebrew cask, winget(릴리스의 자체 파일을 설치함) | 위 표의 방식대로 자체 업데이트합니다 |
| `store` | App Store, Mac App Store, Microsoft Store, Google Play, Flathub 및 기타 스토어 | [스토어 패키지](#스토어-패키지)이며 자체 업데이터가 없습니다. iOS와 macOS는 키 없이 각 App Store에 문의하고, Google Play 설치본은 Play의 네이티브 업데이트 흐름을 사용할 수 있습니다 |
| `steam` | Steam | Steam이 업데이트하는 [디포 디렉터리](#steam-디포)를 생성합니다 |
| `none` | Linux 배포판 및 앱을 재패키징하는 모든 경우 | 아무것도 확인하지 않습니다 |

스토어는 자체 업데이트하는 앱을 거부하므로 반드시 direct가 아닌 `store` 빌드를 업로드하세요. macOS 또는 Windows 인스톨러가 포함된 각 direct 릴리스는 Homebrew cask(`dist/releases/homebrew/<name>.rb` — `auto_updates true`, 빌드된 디스크 이미지의 체크섬과 다운로드 URL, 각 앱 [커맨드](#커맨드)에 대한 `binary` 링크 선언)와 winget 다중 파일 매니페스트(`dist/releases/winget/<id>.yaml`, `<id>.installer.yaml`, 빌드된 NSIS 인스톨러용 `<id>.locale.en-US.yaml`)도 함께 작성합니다. 시스템이 앱을 누가 설치했는지 알려주는 곳에서는 앱도 이를 확인합니다. 다른 스토어에서 설치된 Android 앱, Flatpak이나 Snap, TestFlight 빌드는 업데이트를 해당 스토어에 맡깁니다. Google Play 설치본에서 수동으로 `updater.Check`를 호출하면 허용될 때 Play In-App Updates에 즉시 업데이트 흐름을 요청하며, Play가 동의, 다운로드, 설치, 재시작을 처리합니다. 개발 빌드는 이 흐름에 들어가지 않습니다. 독립 실행형 Android 패키지는 SDK를 번들하며, [임베딩 호스트](embedding.md#android)는 Gradle을 통해 제공합니다.

### 앱에서 확인하기

`updater.Check(cx)`는 *업데이트 확인…* 메뉴 항목처럼 확인하고 결과를 표시하며, `updater.CheckInBackground(cx)`는 새 버전이 있을 때만 표시하고, `updater.CheckDaily(cx)`는 하루에 최대 한 번만 그 작업을 수행합니다. `updater.Status(cx)`는 Linux와 Android에서 실시간 다운로드 및 설치 진행률을 읽습니다. `automatic`이 켜져 있으면 앱은 Linux, direct Android 빌드, Apple App Store 빌드에서 시작할 때와 사용자가 앱으로 돌아올 때마다 이 방식으로 확인하며, 오프라인 등으로 확인이 실패하면 한 시간 뒤에 다시 시도합니다. 개발 빌드는 절대 업데이트하지 않습니다. Steam 빌드에서는 이 세 함수가 아무것도 하지 않습니다.

`updater.CapabilityOf(cx)`는 네트워크 요청 없이 이 사본이 어떻게 업데이트되는지 보고합니다(`ManagedSelf`, `ManagedPage`, `ManagedDevelopment`, `ManagedChannel`, `Store` 이름을 포함한 `ManagedStore`, 또는 `ManagedUnavailable`). 개발 빌드, 비활성화된 채널, 스토어 설치는 확인 시 이유별 설명을 내놓으며, `core.UpdateCheck.Err`는 잘못된 피드나 로드할 수 없는 번들 업데이터 같은 실패에만 사용됩니다.

바인딩 안에서 `updater.Status(cx)`를 읽으면 `Capability`, `AutomaticChecks`, `AutomaticDownloads`, `Available`, `Notes`, `LastCheck`를, 그리고 Linux와 Android에서는 업데이트의 `Phase`, `Loaded`, `Total`(프로그레스 바용 `Fraction()`)을 추적할 수 있습니다. `updater.SetAutomaticChecks(cx, on)`은 사용자의 선택을 저장하고 런타임에 일일 스케줄러를 변경합니다. Sparkle과 WinSparkle은 자체 환경설정을 관리하며, Linux, Android, iOS는 비공개 스토리지 파티션에 선택을 저장합니다. `updater.SetAutomaticDownloads(cx, on)`은 macOS에서만 지원되며 기본값은 `updates.automatic_downloads`이고, 다른 플랫폼은 false를 반환하고 세터를 무시합니다. 지원되지 않거나 외부에서 관리되는 빌드는 두 설정 모두 false를 보고합니다. WinSparkle은 발견 콜백 이후에 앱캐스트를 읽어 발견된 업데이트 메타데이터를 보고하며, 설치는 여전히 WinSparkle이 제어합니다. Linux는 시작 시 Debian 또는 Arch 패키지 소유권과 AppImage 여부를 확인하며, 해당 사본들은 `ManagedSelf`를 보고하고 패키징되지 않은 기타 사본은 확인 전에 `ManagedPage`를 보고합니다.

## 로그인 시 실행

`desktop.SetLaunchAtLogin(cx, on)`은 플랫폼의 로그인 메커니즘에 앱을 등록하고, `desktop.LaunchAtLogin(cx, done)`은 이를 읽습니다. 현재 도구로 빌드한 MSIX에는 기본적으로 비활성화된 시작 작업(startup task)이 자동으로 포함되며, 활성화할 때는 사용자의 시작 앱 스위치와 관리자 정책을 따릅니다. 일반 Windows 및 Linux 설치본은 Run 레지스트리 값과 XDG 자동 시작 엔트리를 사용합니다. macOS는 번들 앱의 네이티브 로그인 서비스를 사용합니다.

Flatpak은 호스트 설정 파일 접근 권한을 주지 않고 동의 여부를 물을 수 있는 Background 포털을 요청합니다. 이후 읽기는 네이티브 응답을 기다립니다. 이 포털에는 읽기 전용 시작 쿼리가 없기 때문에 Flatpak 읽기는 현재 실행에서 마지막으로 확인된 응답만 반환하고(확인 전에는 false), 앱 외부에서의 변경을 감지하거나 다음 실행에서 설정을 복원할 수 없습니다.

외부에서 패키징된 Snap은 앱에 자동 시작 파일명을 선언합니다:

```yaml
apps:
  app:
    command: bin/app
    autostart: app.desktop
```

NSGo는 해당 파일명을 `$SNAP_USER_DATA/.config/autostart` 아래에 작성하며 snapd는 앱의 래퍼를 통해 이를 실행합니다. `desktop.LaunchedAtLogin(cx)`는 MSIX StartupTask 활성화를 포함해 앱이 빌드되기 전에 로그인 실행 여부를 구분합니다.

## 파일 형식

`file_types`는 앱이 여는 문서를 등록합니다. macOS의 Finder, Dock, '다음으로 열기', Windows의 Windows 인스톨러나 MSIX를 통한 탐색기 '연결 프로그램', Linux의 Debian, Arch, AppImage, Flatpak 패키지 데스크톱 엔트리, iOS의 '다음에서 열기'가 여기에 해당합니다:

```yaml
file_types:
  - name: Flow File
    extensions: [flow]
    # id: dev.example.hello.flow   # <id>.<first extension> by default; another's ID imports its type
    # icon: assets/Flow.icns       # macOS document icon; or a square PNG
    # role: viewer                 # editor by default
    # conforms: [public.json]      # public.data by default
    # mime: [application/json]     # existing MIME types; otherwise inferred on Linux
```

Windows 인스톨러는 사용자의 기본 연결 프로그램을 바꾸지 않고 '연결 프로그램' 목록에만 앱을 추가하며, 업그레이드와 제거 시 등록을 삭제합니다. Linux는 고정 테이블에서 흔한 확장자의 MIME 타입을 지정하고, 알 수 없는 앱 자체 타입의 확장자에는 공유 MIME-info 정의와 함께 `application/x-<type id>`를 부여합니다. `mime`은 기존 타입을 지정하며, 테이블에 없는 확장자를 가진 임포트된 타입에는 이를 설정해야 합니다. AppImage를 직접 실행하는 것만으로는 아무것도 등록되지 않으며 데스크톱 통합 도구가 엔트리와 정의를 설치합니다.

앱은 `desktop.OnOpenFiles`로 파일을 수신하며, 실행 중인 상태에서 Windows나 Linux로 새 실행이 들어오거나 iOS 씬 문서 열기가 발생할 때도 수신합니다. 앱을 실행시킨 파일들은 첫 번째 핸들러가 등록될 때까지 대기합니다:

```go
desktop.OnOpenFiles(cx, func(paths []string) { open(paths[0]) })
```

iOS에서는 일반 문서들이 원래 이름을 유지한 채 앱 소유의 임시 파일로 임포트됩니다. 파일 프로바이더 읽기는 UI 스레드 밖에서 조율되며 핸들러가 실행되기 전에 보안 스코프가 종료됩니다. 임포트된 파일을 보관하려면 `cx.DataDir()`로 옮기세요. 편집 내용은 복사본에 반영됩니다. iOS는 앱의 문서 아이콘을 사용하며 `icon`은 macOS에 적용됩니다. App Clip과 tvOS 패키지는 문서 타입을 등록하지 않습니다. 파일 열기는 `OnOpenLinks`와 분리되어 유지됩니다.

`desktop.AddRecentFile(cx, path)`는 사용자가 연 문서를 macOS의 Dock 메뉴와 파일 › 최근 항목 열기, Windows의 점프 목록 최근 항목 카테고리, Linux의 데스크톱 최근 사용 파일에 기록합니다. 목록에서 선택하면 같은 핸들러로 전달됩니다. `desktop.ClearRecentFiles`는 앱의 기록을 지우되 Linux에서 공유 파일에 대한 다른 앱의 기록은 유지하며, Linux는 GTK의 기록 설정도 따릅니다.

`desktop.SetDockMenu(cx, items)`는 앱 아이콘에 동작을 설정합니다. macOS의 Dock 메뉴, Windows의 점프 목록 작업, 그리고 퀵리스트가 지원되는 Linux의 Ubuntu dock, Plasma, Dash to Dock의 퀵리스트가 여기에 해당합니다. Linux에서는 앱의 `id`와 일치하는 설치된 데스크톱 엔트리가 필요하며 하위 메뉴, 구분선, 체크된 항목, 비활성화된 항목을 유지합니다. 메뉴는 이를 설정한 스코프에 속하며 앱의 창들 중 마지막으로 설정한 호출이 기존 메뉴를 대체합니다.

## 링크

`url_schemes`는 브라우저나 다른 앱, `open`, `start`, `xdg-open`에서 앱을 여는 `myapp://settings` 같은 앱 고유 링크를 등록합니다. macOS, iOS, Android 앱과 Windows 인스톨러 및 MSIX 패키지, 그리고 Debian, Arch, AppImage, Flatpak 패키지에 포함된 Linux 데스크톱 엔트리가 이를 등록하며 제거 시 삭제됩니다.

```yaml
url_schemes: [myapp]
# app_links: [links.example.com]  # HTTPS hosts for the full app
```

앱은 `desktop.OnOpenLinks`로 링크를 수신하며 실행 중일 때도 작동합니다. Windows와 Linux에서는 두 번째 실행이 실행 중인 앱에 링크를 넘기고 종료됩니다. 앱을 실행시킨 링크는 첫 번째 핸들러를 기다립니다. 누구나 이런 링크를 만들 수 있고 Windows와 Linux에서는 앱이 시작될 때 전달된 어떤 URL이든 핸들러로 들어오므로, 스킴을 검사하고 링크가 상태를 변경하기 전에 사용자에게 확인을 받으세요:

```go
desktop.OnOpenLinks(cx, func(links []string) { show(links[0]) })
```

iOS와 Android에서는 `mobile.OnOpenLinks`가 시작 시와 실행 중에 링크를 수신합니다. iOS 시뮬레이터에서는 `xcrun simctl openurl booted myapp://settings`로, Android에서는 `adb shell am start -a android.intent.action.VIEW -d myapp://settings`로 열 수 있습니다. 테스트에서는 `app.OpenLinks("myapp://settings")`로 전달합니다.

iOS 유니버설 링크를 사용하려면 `app_links: [links.example.com]`으로 명시적인 HTTPS 호스트를 선언하세요. 각 호스트는 스킴, 경로, 포트, IP 주소, 와일드카드 없는 DNS 이름이어야 하며, 소문자로 변환되고 중복은 거부됩니다. 도구는 Xcode 관리 서명을 포함해 전체 앱의 Associated Domains entitlement에 `applinks:<host>`를 기록하며, IPA 프로필도 이를 승인해야 합니다. App Clip은 `app_clip.domains`에서 온 별도의 `appclips:` 클레임을 유지합니다.

각 호스트는 `https://<host>/.well-known/apple-app-site-association`에서 전체 앱을 지정하는 `apple-app-site-association` 파일을 제공해야 합니다. 패키지 메타데이터만으로는 유니버설 링크가 승인되지 않습니다. [Apple의 연관 도메인 요구사항](https://developer.apple.com/documentation/xcode/supporting-associated-domains)에서 서버 설정을 설명합니다. 애드혹 시뮬레이터 서명은 제한된 개발자 entitlement를 생략하므로 델리게이트 테스트만으로 서버 연관이 증명되지는 않습니다.

iOS에서 `mobile.OpenedURL(cx)`는 첫 번째 루트가 마운트되기 전의 초기값을 포함해 가장 최근의 App Clip 호출, 유니버설 링크 또는 커스텀 스킴 URL을 추적합니다. 커스텀 스킴 전달은 여전히 `mobile.OnOpenLinks`에도 도달하며 해당 핸들러가 실행될 때 호출 값이 최신 상태가 됩니다. 파일 열기는 `OnOpenFiles`에 머무르며 호출 URL을 덮어쓰지 않습니다. Android/Web 호출 지원은 #1475에서 별도로 추적 중입니다.

## Android에서 공유 받기

Android 공유 시트에서 앱이 처리할 MIME 타입을 선택하세요:

```yaml
android:
  share_types: [text/plain, 'image/*', application/pdf]
```

APK와 App Bundle 빌드는 단일 항목과 다중 항목 모두에 대해 해당 타입을 등록합니다. 각 공유를 받으려면 [`mobile.OnShared`](services.md#링크와-파일-열기)를 등록하세요.

경로 기반 주소를 사용하는 웹 앱은 `web.base`를 배포 디렉터리(`/` 또는 `/my-app/`)로 설정하고 프로덕션 서버가 애플리케이션 라우트에 대해 `index.html`을 서빙하도록 구성하세요. 개발 서버는 이미 HTML 내비게이션 요청에 대해 이 폴백을 제공합니다. 딥 링크가 새로 고침될 때도 애셋은 고정된 HTML base를 기준으로 상대 경로를 유지합니다. [애플리케이션 주소](addresses.md)를 참고하세요.

## iOS에서 공유 받기

공유 확장(share extension)이 수락하는 Uniform Type Identifier를 URL과 텍스트 타입이 넓은 범위의 파일 타입보다 먼저 오도록 선언하세요:

```yaml
apple:
  share_types: [public.url, public.plain-text, public.image, com.adobe.pdf]
  # app_group: group.com.example.app  # defaults to group.<id>
sign:
  # share_profile: assets/Share.mobileprovision
```

`build`와 `dev`는 앱과 함께 Go 공유 확장인 `<id>.share`를 패키징합니다. Xcode는 기기 실행 시 이에 서명하고 App Group을 등록합니다. IPA의 경우 `<id>`와 `<id>.share` 모두에 같은 App Group을 활성화해야 하며 `sign.profile`과 `sign.share_profile`이 이를 부여해야 합니다. App Clip, tvOS, watchOS에는 공유 확장이 없습니다.

확장은 최대 128개 항목과 512 MiB의 파일을 App Group에 복사한 뒤 공유 시트를 완료합니다. `mobile.OnShared`는 앱이 활성화되어 있는 동안 또는 사용자가 다음에 앱을 열 때 이 배치를 수신하며, 전달되기 전에 확장이나 앱이 종료되어도 유지됩니다. iOS는 Share 확장이 자신을 포함한 앱을 포그라운드로 올리거나 실행할 수 있는 지원 API를 제공하지 않으므로 이 핸드오프가 자동으로 앱을 열지는 않습니다.

## 미니 프로그램

```sh
go tool nsgo build -target miniprogram_wechat -run
go tool nsgo dev -target miniprogram_alipay
```

각 타겟은 해당 플랫폼의 개발자 도구용 프로젝트를 `dist/releases/miniprogram/<platform>`에도 생성하며, `-run`과 `dev`가 이를 엽니다. `dev`는 저장할 때마다 다시 빌드하고 도구가 리로드합니다:

| 타겟 | 플랫폼 | 도구 |
| --- | --- | --- |
| `miniprogram_wechat` | WeChat | [WeChat DevTools](https://developers.weixin.qq.com/miniprogram/dev/devtools/download.html), 서비스 포트 켜기(Settings, Security) |
| `miniprogram_qq` | QQ | [QQ 개발자 도구](https://q.qq.com/wiki/tools/devtool/), macOS |
| `miniprogram_alipay` | Alipay | [Alipay Mini Program Studio](https://opendocs.alipay.com/mini/ide/download), macOS |
| `miniprogram_douyin` | Douyin 및 ByteDance의 기타 앱 | [Douyin 개발자 도구](https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/developer-instrument/download/developer-instrument-update-and-download), macOS |

`miniprogram.appid`는 각 플랫폼(`wechat`, `qq`, `alipay`, `douyin`)에서의 미니 프로그램 AppID를 지정합니다. 지정하지 않으면 WeChat DevTools는 테스트용(tourist) AppID로 미리보기를 열고 다른 도구들은 AppID를 묻습니다. WeChat에서는 `-run`과 매 `dev` 리빌드가 CLI를 통해 프로젝트를 닫고 자동으로 다시 열어 DevTools가 새 파일을 컴파일하도록 합니다. 이는 DevTools 2.01.2510290에서 확인되었습니다. 플랫폼 내비게이션 바는 앱의 제목을 표시하며, `custom_navigation`을 켜면 앱이 페이지를 가득 채우고 안전 영역(safe area)이 메뉴 버튼을 피하도록 해 줍니다. 앱은 웹의 `--nsgo-os`처럼 `Env.Simulated`가 true인 상태로 폰의 OS를 읽으며, 시스템 헤더가 대체하지 않으므로 스택 페이지들은 자체 `Header`를 표시합니다. QQ, Alipay, Douyin은 아직 해당 도구에서 테스트되지 않았으며, Alipay는 아직 앱을 통한 뒤로 가기, 키프레임, 키보드 높이를 지원하지 않습니다.

미니 프로그램은 실험적 기능입니다. 미니 프로그램은 TSX가 아닌 Go 앱만 실행하며, 플랫폼들은 압축된 앱 모듈 크기를 2 MB로 제한합니다. `base`를 사용한 몇 개 화면짜리 앱은 `examples/gallery-miniprogram`처럼 들어가지만 `ui` 컨트롤을 쓴 앱은 아직 용량에 맞지 않으며, 초과 시 `build`가 안내합니다. 셰이더는 아무것도 그리지 않으며 웹 뷰, 비디오, 네이티브 컴포넌트, 앱 폰트 파일은 아직 표시되지 않습니다.

## HarmonyOS

```sh
go tool nsgo build -target harmonyos -run
```

`harmonyos`는 `dist/releases`에 `<name>-<version>-harmonyos.hap`을 생성합니다. 엔진은 ArkWeb 안에서 WebAssembly로 실행되고, ArkTS 호스트가 마운트된 내용을 ArkUI 뷰로 그리므로 텍스트, 입력 필드, 스크롤, 다이얼로그가 시스템 자체 컴포넌트로 동작합니다. `PATH`에 `hdc`가 있으면 `-run`이 연결된 기기나 에뮬레이터에 HAP를 설치하고 실행하며, `nsgo test -target harmonyos`는 별도의 `.nsgotest` 번들 ID로 에뮬레이터에서 서명되지 않은 HAP로 앱 테스트를 실행한 뒤 설치된 앱을 유지하기 위해 해당 테스트 HAP를 삭제합니다.

템플릿에서 ArkTS 호스트를 컴파일하고 앱의 `id`를 번들 이름으로 사용하려면 `PATH`에 `hvigorw`를 추가하고 `OHOS_BASE_SDK_HOME`을 템플릿의 컴파일 및 타겟 SDK 버전인 API 26(`26.0.0`)의 OpenHarmony SDK로 설정하세요. `build.harmonyos_bundles`는 기본값인 `auto`, SDK가 필요한 `exact`, 또는 커밋된 호스트의 고정 길이에 맞춰 id를 패딩해 SDK 없이도 동작하는 `padded` 중 하나를 선택합니다. 타겟 테스트도 같은 설정을 사용하며 어떤 호스트를 패키징했는지 출력합니다. `harmonyos.permissions`는 `MICROPHONE`, `CAMERA`처럼 앱이 요청할 `ohos.permission` 이름을 나열하고, `widget.title`은 홈 화면 카드에 표시될 제목입니다.

HarmonyOS는 프리뷰 단계입니다. 에뮬레이터는 서명되지 않은 HAP를 설치할 수 있습니다. [아키텍처 노트](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/harmonyos.md)에 프리뷰가 그리는 ArkUI 기능 목록이 있습니다.

### 서명된 HAP

AppGallery Connect 자료로 HAP에 서명하려면 `harmonyos.signing`을 설정하세요. Huawei 휴대폰에 직접 설치하려면 해당 기기용 디버그 인증서와 프로필이 필요하며, 배포에는 릴리스 자료를 사용하세요. [Huawei 서명 가이드](https://developer.huawei.com/consumer/en/doc/harmonyos-guides-V2/command-line-building-app-hap-hvigor-0000001553061909-V2/)

```yaml
harmonyos:
  signing:
    keystore: signing/app.p12
    key_alias: app
    certificate: signing/app.cer
    profile: signing/app.p7b
```

경로는 `nsgo.yaml` 기준 상대 경로입니다. 환경 변수에 `NSGO_HARMONY_KEYSTORE_PASSWORD`와 `NSGO_HARMONY_KEY_PASSWORD`를 설정하세요. 서명에는 SDK의 `hap-sign-tool.jar`와 `JAVA_HOME` 또는 `PATH`의 Java를 사용합니다. 빌드는 프로필 번들 이름과 일치시키기 위해 앱의 정확한 `id`를 유지하므로, 서명 시에는 `auto` 모드에서도 SDK와 `hvigorw`가 필요합니다. 앱 디렉터리에서 `go tool nsgo doctor`를 실행하면 비밀번호 없이도 서명 구성과 프로필 번들 이름을 검사합니다. `build -run`은 서명된 HAP를 `hdc`에 전달합니다.

## 서명

```yaml
sign:
  identity: "Apple Development: you@example.com (ABCDE12345)"  # signs the macOS app, else ad hoc
  profile: assets/AppStore.mobileprovision  # signs the IPA
  keystore: assets/release.p12              # signs APKs, with key_alias
  certificate: assets/codesign.pfx          # signs the Windows app and its installer
  # widgets_profile: assets/Widgets.mobileprovision  # the iOS extension's profile; use a macOS profile for a Mac App Store build
  # share_profile: assets/Share.mobileprovision      # signs the IPA's share extension
  # clip_profile: assets/Clip.mobileprovision        # signs the IPA's App Clip
```

키스토어의 비밀번호는 `NSGO_KEYSTORE_PASSWORD`와 `NSGO_KEY_PASSWORD`에서 읽어옵니다. `-team`은 iPhone 패키지에 서명합니다. `-profile`, `-widgets-profile`, `-share-profile`, `-clip-profile`, `-macos-profile`은 커맨드라인에서 앱, 위젯 확장, 공유 확장, App Clip, Mac App Store 프로비저닝 프로필을 각각 대체합니다. `-installer`는 Mac Installer Distribution 인증서를 대체합니다.

### Windows 패키지 서명

Authenticode는 Windows 다운로드의 게시자를 식별합니다([SmartScreen 평판 검사는 별개입니다](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation)). `.pfx` 파일인 `sign.certificate`는 실행 파일, 커맨드 동반 파일, 앱 리소스에 포함된 프로그램과 라이브러리, 언인스톨러가 포함된 인스톨러, 스토어 채널용으로 빌드된 MSIX에 서명하며 비밀번호는 `NSGO_CERTIFICATE_PASSWORD`에서 가져옵니다. 서명은 `signtool`과 동일한 방식으로 Go에서 직접 생성되므로 Windows SDK 없이 Mac에서도 서명할 수 있습니다:

```yaml
sign:
  certificate: assets/codesign.pfx
  timestamp_url: http://timestamp.digicert.com  # the RFC 3161 server that timestamps them
```

`sign.timestamp_url`의 기본값은 DigiCert 서버이며, `none`을 주면 타임스탬프 없이 서명되어 인증서 만료 시 서명 효력도 만료됩니다. 하드웨어 토큰이나 클라우드 서비스에 있는 인증서는 대신 `%1`이 서명 대상 파일 경로를 뜻하는 자체 명령을 통해 서명할 수 있으며 빌드가 위의 모든 파일에 대해 이 명령을 실행합니다:

```yaml
sign:
  sign_command: "signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a %1"
```

명령이 성공하더라도 앱 실행 파일, 커맨드 동반 파일, PE 리소스 또는 셋업 실행 파일 중 하나라도 내장된 Authenticode 서명이 빠져 있으면 `build`는 중단됩니다.

인스톨러 내부의 언인스톨러는 내장되기 전에 앱 및 셋업과 같은 `sign.certificate` 또는 `sign.sign_command`로 서명됩니다. `-certificate`, `-sign-command`, `-timestamp-url`로 커맨드라인에서 이를 설정할 수 있습니다.

### 공증

Mac에서 다운로드한 앱은 Apple의 공증(notarization)을 받으면 Gatekeeper 경고 없이 열립니다. Developer ID로 서명하고 `notarytool`이 키체인에 저장한 자격 증명 이름을 지정하세요:

```sh
xcrun notarytool store-credentials hello --apple-id you@example.com --team-id ABCDE12345
```

```yaml
sign:
  identity: "Developer ID Application: Example Inc (ABCDE12345)"
  notarize: hello   # the keychain profile above; or -notarize hello
```

그러면 `build`는 강화된 런타임으로 서명하고 디스크 이미지를 제출한 뒤 몇 분간 Apple의 판정을 기다려 이미지와 앱에 티켓을 스테이플링하며, 거부되면 Apple의 로그를 출력합니다. CI에서는 App Store Connect API 키의 `.p8` 파일, ID, 발급자인 `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID`, `NSGO_NOTARY_ISSUER`가 키체인 프로필을 대신합니다. 강화된 런타임은 앱이 권한을 선언하지 않으면 마이크, 카메라, 캘린더, 위치 접근을 차단하는데, `apple.info`에 `NSMicrophoneUsageDescription`, `NSCameraUsageDescription`, 캘린더/미리알림 사용 설명, 위치 사용 설명이 들어 있으면 `build`가 해당 권한을 자동으로 추가합니다.

## 위젯과 실시간 현황

`build`와 `dev`는 별도 단계 없이 Xcode의 Swift 컴파일러로 macOS 및 iOS 패키지에 앱의 위젯과 실시간 현황(Live Activities)을 그리는 확장을 함께 컴파일합니다. iOS에서는 앱과 위젯이 앱 그룹을 공유하므로, iPhone에서 `-run`을 실행하면 Xcode가 둘 다 서명하고 그룹을 등록하며, IPA의 경우에는 `<id>`와 `<id>.widgets` 모두에 그룹을 활성화하고 `sign.widgets_profile`로 확장에 서명하세요. Android 독립 실행형 APK와 AAB에는 `widgets`의 App Widget 프로바이더와 리소스가 포함되며, 앱 빌드에 SDK나 Java 컴파일러 없이도 사전 빌드된 Java 런타임이 타임라인을 그립니다. Android 16에서는 별도 확장이 필요 없는 Live Updates로 실시간 현황을 표시합니다. 구성과 API는 [위젯](widgets.md) 및 [실시간 현황](live-activities.md)에서 다룹니다.

## Android App Links

`app_links`는 해당 링크가 전체 Android 앱을 열도록 할 명시적 HTTPS 호스트들을 지정합니다:

```yaml
app_links: [example.com, shop.example.com]
```

APK와 App Bundle은 각 호스트에 대해 `autoVerify`가 켜진 탐색 가능한(browsable) `ACTION_VIEW` 필터를 선언합니다. 스킴, 경로, 포트, 와일드카드, IP 주소가 없는 DNS 호스트만 사용하세요. 앱의 패키지 ID와 서명 인증서 지문이 담긴 `https://<host>/.well-known/assetlinks.json` 파일을 호스트에서 제공해야 Android가 링크를 앱으로 라우팅하기 전에 연관을 검증합니다. 도구는 연관 파일을 게시하지 않으며, 설정 방법은 [Android 검증 가이드](https://developer.android.com/training/app-links/verify-applinks)에 나와 있습니다. 임베드된 Android 호스트는 자체 인텐트 필터를 선언하고 수락된 링크를 `NsgoView`로 전달해야 합니다.

`mobile.OpenedURL(cx)`는 첫 번째 루트 렌더링 전에 수락된 호출 URL을 읽고, 실행 중인 앱을 다른 링크가 열면 변경됩니다. `mobile.OnOpenLinks` 역시 수락된 각 링크를 수신합니다. 전체 페이지 웹 앱은 브라우저 히스토리와 해시 변경을 포함해 현재 페이지 주소를 `mobile.OpenedURL`로 읽으며, 다른 페이지 안에 마운트된 앱은 상위 페이지의 주소를 읽지 않습니다. 앱이 주소를 직접 쓰기도 할 때는 [`cx.AppURL`](addresses.md)을 사용하세요.

## App Clip

App Clip은 앱을 설치하지 않고도 링크, 코드, NFC 태그를 통해 실행되는 iOS 앱의 작은 일부입니다. `app_clip`은 표시할 부분만 링크하도록 별도의 `main` 패키지에서 App Clip을 빌드해 iOS 패키지에 넣습니다:

```yaml
app_clip:
  main: ./clip              # the clip's main package, the app's own by default
  domains: [example.com]    # whose links open it
  # name: Hello Clip        # "<name> Clip" by default
  # id: dev.example.hello.Clip  # <id>.Clip by default
  # minimum: "17.0"         # 16.0 by default; 17.0 allows 100 MB when only links open it
  # notifications: true     # ask for notifications for a day
  # location: true          # let the clip confirm where it was opened
```

클립은 자신을 연 링크를 `mobile.OpenedURL`로 읽으며 다른 링크로 다시 열리면 값이 바뀝니다. `mobile.IsAppClip`은 코드가 App Clip으로 실행 중인지 여부를 반환합니다. 시스템은 최대 15 MB(링크로만 여는 경우 iOS 17부터 100 MB)까지의 클립을 다운로드하며 이를 초과하면 `build`가 거부합니다(`ui`를 링크한 클립은 약 11 MB입니다). `dev -target ios_simulator -clip https://example.com/order?table=12`는 시뮬레이터에서 해당 링크로 클립만 단독 실행하고, `-target ios`는 iPhone에서 실행하며 링크의 QR 코드를 출력합니다(설정 > 개발자 > Local Experiences에 링크를 추가한 뒤 카메라로 스캔하세요). 개발 심볼 때문에 클립이 QR 크기 제한을 넘으면 `-release`를 사용하세요. Apple은 개인 팀이 아닌 Apple Developer Program 유료 팀에 대해서만 App Clip 서명을 허용합니다.

`dev -target ios`와 `build -run -target ios`는 전체 앱 안에 클립을 함께 유지합니다. Xcode가 별도 타겟으로 클립에 서명하고 앱 서명 전에 임베드합니다. 자동 서명을 쓰려면 Xcode에 로그인된 유료 팀 계정과 앱·클립 양쪽의 프로비저닝이 필요합니다. App Store 배포 시에는 `<id>`에 App Clips를 활성화하고 `sign.clip_profile`로 클립에 서명하며 도메인의 `apple-app-site-association` 파일을 제공하세요. 저장소의 `examples/appclip`은 클립으로 테이블에서 주문하는 카페 예제입니다.

주문 정보나 기타 로컬 데이터를 전체 앱으로 넘기려면 `app_clip`과 함께 명시적인 그룹을 설정하세요:

```yaml
apple:
  app_group: group.dev.example.hello
```

그러면 클립과 전체 앱이 해당 그룹의 컨테이너에서 `cx.DataDir()`과 `cx.Storage()`를 공유합니다. 클립이 작성한 데이터 디렉터리 아래의 파일과 키-값 항목들은 전체 앱이 시작될 때 사용할 수 있습니다. `cx.LogsDir()`도 해당 디렉터리를 따르지만 각 번들은 자체 캐시를 유지합니다. Xcode 서명은 두 타겟 모두에 대해 그룹 권한을 요청하며, 프로필을 사용할 때는 두 App ID에 같은 그룹을 활성화하고 `sign.profile`과 `sign.clip_profile`에서 부여해야 합니다. 권한이 누락되면 서명이 거부되고, 접근할 수 없는 그룹은 시작을 중단시킵니다. 호스트의 명시적 데이터 디렉터리는 여전히 우선합니다.

`apple.app_group`이 없으면 각 번들은 자체 샌드박스 스토리지를 유지합니다. 이 설정을 켜더라도 이전에 각 샌드박스에 저장된 데이터가 마이그레이션되지는 않으며, 위젯만 있는 앱은 기존 스토리지 위치를 유지합니다. 공유 비밀 정보는 일반 스토리지 대신 키체인에 보관하세요. 카페 예제는 클립의 주문을 저장하고 전체 앱에서 표시합니다.

클립 내부에서 사용자의 명시적인 동작이 일어났을 때 `mobile.ShowAppClipOverlay(cx, mobile.AppClipOverlayBottom)`을 호출해 StoreKit이 대응하는 전체 앱을 추천하도록 요청할 수 있습니다. `AppClipOverlayBottomRaised`는 하단 바 공간을 남겨 둡니다. 반환된 함수는 해당 요청을 닫으며, 스코프 정리 시에도 닫히고 이전 요청은 더 새로운 요청을 닫을 수 없습니다. 네이티브 iOS App Clip 외부에서의 호출은 아무것도 하지 않습니다. 추천을 로드해 표시할 수 있는지 여부는 App Store가 결정하므로 이 호출은 표시 성공이나 설치 여부를 반환하지 않습니다. 전체 앱을 설치하지 않아도 클립의 작업을 완료할 수 있도록 유지하세요. [Apple 오버레이 가이드](https://developer.apple.com/documentation/appclip/recommending-your-app-to-app-clip-users)

## 다른 앱에 임베드

`-embed`는 직접 만든 iOS 또는 Android 앱에 넣을 수 있도록 앱을 패키징합니다. `-dynamic`을 주면 동적 프레임워크로 된 XCFramework를, Android용으로는 `NSGoView` 또는 `NsgoView`로 표시할 수 있는 AAR을 생성합니다. Go 데스크톱 프로그램은 `nsgo.NewView`로 앱을 표시하고, 웹 페이지는 사이트의 `nsgo.js`를 사용하며 이때 호스트는 해당 페이지 또는 `nsgo.yaml`의 `web.host`가 지정한 주소가 됩니다. 저장소의 `examples/embed-*`에서 각 예제를 볼 수 있으며, 호스트 측 구현은 [임베딩](embedding.md)에서 다룹니다.

---
title: 네이티브 라이브러리
description: NSGo의 일반 패키징, 라이선스 및 서명 단계를 거쳐 Go 패키지와 함께 사전 빌드된 공유 라이브러리와 Apple 프레임워크를 배포하고, 이들에 필요한 Linux 패키지를 지정합니다.
group: 툴체인
order: 23
---

Go 패키지는 패키지 디렉터리에 `nsgo-natives.yaml`을 두어 purego로 로드하는 공유 라이브러리를 함께 배포할 수 있으며, 버전 2부터는 iOS와 tvOS용 프레임워크도 배포하고 필요한 Linux 패키지도 지정할 수 있습니다. 앱이 해당 패키지를 임포트하면 NSGo는 타겟에 맞는 선언을 선택하고, 파일을 검증한 뒤 서명 및 인스톨러 생성 단계 전에 포함합니다. 앱에는 별도의 설정이나 패키징 명령이 필요하지 않습니다.

데스크톱 앱 빌드와 타겟 테스트는 앱 코드에 대해 cgo를 비활성화합니다. 네이티브 코드는 purego와 `nsgo-natives.yaml`에 선언한 라이브러리를 사용해야 합니다. NSGo는 cgo 파일이 들어 있는 앱 패키지를 빌드가 시작되기 전에 거부합니다.

## 라이브러리 선언

```yaml
version: 2
libraries:
  macos_arm64:
    - name: libexample.dylib
      path: lib/macos-arm64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  macos_x64:
    - name: libexample.dylib
      url: https://downloads.example.org/v1.0/macos-x64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  windows_arm64: []
  windows_x64: []
  linux_arm64: []
  linux_x64: []
  ios:
    - name: Example.framework
      url: https://downloads.example.org/v1.0/ios/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  ios_simulator:
    - name: Example.framework
      path: lib/ios-simulator/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  tvos: []
  tvos_simulator: []
```

버전 2가 지원하는 키는 `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64`, `linux_x64`, `ios`, `ios_simulator`, `tvos`, `tvos_simulator`입니다. 버전 1은 여섯 개의 데스크톱 키만 지원합니다. 파일은 해당 버전의 키에 대해서만 읽히므로, iOS와 tvOS 빌드는 버전 1 파일에서 아무것도 패키징하지 않습니다. Go 패키지가 선택되는 모든 타겟을 선언하세요. `[]`는 해당 타겟에 번들 라이브러리가 필요 없음을 명시합니다. 대신 Go 빌드 태그를 사용해 지원하지 않는 타겟에서 패키지를 제외할 수도 있습니다. `macos_universal`은 두 macOS 항목을 사용하고, 터미널 타겟은 해당 데스크톱의 항목을 사용합니다. 예를 들어 `tui_macos_arm64`는 `macos_arm64`를 사용합니다.

각 라이브러리에는 다음 필드가 있습니다:

- `name`: 설치될 파일 이름입니다. macOS에서는 `.dylib`, Windows에서는 `.dll`, Linux에서는 `.so` 또는 버전이 붙은 `.so.*`로 끝나야 합니다. iOS와 tvOS에서는 프레임워크 이름 `<Name>.framework`이며, `Name`은 ASCII 문자, 숫자, `.`, `-`로 이루어지고 문자나 숫자로 시작하고 끝나야 합니다.
- 이 패키지 디렉터리를 기준으로 한 상대 파일 경로인 `path` 또는 라이브러리 자체를 가리키는 HTTP(S) URL인 `url` 중 정확히 하나를 지정합니다. 버전 2는 https URL만 받으며, 리디렉션이 https를 벗어나면 다운로드가 실패합니다. 아카이브 URL이 아니라 압축이 풀린 라이브러리를 게시하고, 프레임워크는 바이너리만 게시하세요. URL에는 사용자 이름이나 비밀번호를 포함하지 마세요.
- `sha256`: 원본 파일의 SHA-256 해시값으로, 두 소스 방식 모두에서 필수입니다. 예를 들어 macOS에서는 `shasum -a 256 lib/macos-arm64/libexample.dylib` 명령으로 출력할 수 있습니다.
- `licenses`: 이 패키지 디렉터리를 기준으로 한 원본 라이선스 및 고지 파일의 비어 있지 않은 경로 목록입니다. 이 텍스트는 `THIRD_PARTY_NOTICES.txt`에 포함되어 배포되며, 릴리스 빌드에서 `core.ThirdPartyNotices()`를 통해 확인할 수 있습니다. 정확한 모듈 예외(exact module exception)를 포함한 앱의 기존 [라이선스 정책](building.md#라이선스-고지)은 네이티브 라이선스에도 독립적으로 적용됩니다.

로컬 경로와 심볼릭 링크는 선언하는 패키지 내부에 머물러야 합니다. NSGo는 알 수 없는 필드, 체크섬 불일치, 다른 운영체제 또는 아키텍처용 파일, 그리고 충돌하는 설치 파일 이름을 거부합니다. 두 패키지가 체크섬이 동일한 같은 이름의 파일을 공유할 수는 있습니다. 파일 크기는 1 GiB로 제한됩니다.

## 패키징 및 로드

| 타겟 | 라이브러리 디렉터리 |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows, Linux 및 터미널 타겟 | 실행 파일 옆 |
| iOS 및 tvOS | `<app>.app/Frameworks/<Name>.framework/<Name>` |

패키지는 작업 디렉터리가 아니라 실행 파일의 위치를 기준으로 해당 디렉터리에서 라이브러리를 로드합니다. 종속성들이 함께 배포된 디렉터리에서 서로(peer)를 로드하도록 사전 빌드하세요. 필요한 각 피어 파일과 로더가 요구하는 각 파일 이름을 모두 선언하세요.

NSGo는 캐시된 다운로드를 재사용하기 전에 다시 검증하며, 파일 잠금을 통해 동시 빌드를 조율합니다. 콘텐츠 캐시는 운영체제의 사용자 캐시 디렉터리 아래 `nsgo/natives`에 위치합니다. 로컬 파일은 복사되는 시점에 검증됩니다.

유니버설 macOS 패키지는 두 아키텍처 모두에 동일한 라이브러리 이름을 요구합니다. NSGo는 `lipo`로 아키텍처 슬라이스를 합치며, 게시자가 이미 유니버설로 만든 라이브러리를 양쪽 항목에 모두 사용할 수도 있습니다. 라이브러리는 앱보다 먼저 서명된 후 앱의 일반 디스크 이미지, 서명 및 공증 단계를 거칩니다. Windows 서명은 `build.installers: false`일 때도 선언된 DLL을 포함합니다. Linux 인스톨러에는 실행 파일 디렉터리와 동일한 파일 및 고지 사항이 포함됩니다. Mac에서는 터미널 타겟이 macOS 라이브러리에 애드혹 서명을 합니다. 다른 호스트에서는 배포된 그대로(유니버설 라이브러리는 통째로) 포함하며, 서명되지 않은 arm64 라이브러리는 거부합니다.

개발 빌드, `-run`, 타겟 테스트도 데스크톱과 iOS·tvOS 모두에서 동일한 패키지 선택 및 복사 단계를 사용합니다. 선언은 로컬 replace 및 벤더링된 패키지를 포함해 빌드의 타겟, 태그, 생성된 Go 오버레이를 기준으로 선택됩니다. Android, HarmonyOS, watchOS, 웹, 미니 프로그램은 이 파일에서 아무것도 패키징하지 않습니다.

## iOS 및 tvOS 프레임워크

NSGo는 각 프레임워크의 바이너리를 검사하며, 다음 조건을 만족하지 않으면 이유를 밝히고 빌드를 중단합니다:

- `LC_BUILD_VERSION`이 나타내는 대로 타겟의 플랫폼(기기 또는 시뮬레이터)용으로 빌드된 arm64 Mach-O 동적 라이브러리여야 합니다. 유니버설 파일은 arm64 슬라이스가 사용됩니다. arm64e는 거부됩니다.
- 최소 OS가 앱의 최소 버전, 즉 `apple.info.MinimumOSVersion`, 설정하지 않았다면 iOS 15.0 및 tvOS 17.0보다 새롭지 않아야 합니다([빌드 및 배포](building.md#nsgoyaml)).
- 설치 이름이 `@rpath/<Name>.framework/<Name>`이어야 합니다.
- `@rpath`를 통해 로드하는 프레임워크는 앱의 패키지가 같은 타겟에 선언한 것뿐이어야 합니다.

NSGo는 프레임워크의 `Info.plist`를 작성합니다. 그 `CFBundleIdentifier`는 앱 ID, 점 하나, 소문자로 쓴 `Name`으로 이루어지며, 플랫폼과 앱의 최소 OS도 함께 기록됩니다. 시뮬레이터 빌드는 프레임워크에 애드혹 서명을 합니다. 기기 빌드는 서명을 앱의 서명 절차에 맡깁니다. `sign.profile`은 앱보다 먼저 앱의 ID로 프레임워크에 서명하며, 기기에서 `nsgo build -run`과 `nsgo dev`를 실행할 때는 Xcode가 같은 방식으로 서명합니다. 앱의 실행 파일에는 실행 경로가 없으므로, 피어 프레임워크를 로드하는 프레임워크가 `Frameworks`에서 피어를 찾으려면 `@loader_path/..` 또는 `@executable_path/Frameworks` 실행 경로가 필요합니다. 또한 `/usr/lib/swift` 같은 시스템 실행 경로가 없으면 `@rpath`를 통해 그 밖의 것을 로드할 수 없습니다. 번들 ID가 대소문자 구분 없이 앱의 다른 번들과 겹치는 프레임워크, 예를 들어 NSGo의 `NSGoStoreKit.framework` 옆의 `StoreKit.framework`는 빌드를 중단시킵니다.

앱이 iOS 프레임워크를 선언하는 패키지를 링크하면 `nsgo build -embed`는 해당 패키지 이름을 알리며 중단합니다. 호스트 앱용 XCFramework는 아직 이 프레임워크들을 함께 담을 수 없습니다.

## Linux 패키지

버전 2부터 `linux`는 Linux 아키텍처별로, 해당 Go 패키지를 링크하는 앱의 Debian 패키지와 Arch 패키지가 의존하고 권장하는 시스템 패키지를 지정합니다.

```yaml
version: 2
libraries:
  # ...
linux:
  linux_x64:
    depends:
      deb: [libva2, libva-drm2, libpulse0]
      arch: [libva, libpulse]
    recommends:
      deb: [va-driver-all]
      arch: ["intel-media-driver: hardware decoding on Intel GPUs"]
  linux_arm64:
    depends:
      deb: [libavcodec61, libavformat61, libpulse0]
      arch: [ffmpeg, libpulse]
```

- `linux`는 선택 사항입니다. 키는 `linux_x64`와 `linux_arm64`이므로, 한 아키텍처에서는 라이브러리를 번들하고 다른 아키텍처에서는 시스템의 라이브러리를 로드하는 패키지도 각각에 필요한 것을 지정할 수 있습니다.
- `depends`와 `recommends`는 각각 `deb`와 `arch` 목록을 받으며, [nsgo.yaml](building.md#nsgoyaml)에 있는 앱의 `linux.depends` 및 `linux.recommends`와 형식이 같습니다. `deb` 항목은 `libsoup-3.0-0 | libsoup3`, `libavcodec61 (>= 7:7.1)`처럼 dpkg가 쓰는 형식으로, `arch` 항목은 `ffmpeg>=7`, `libpulse: audio output`처럼 pacman이 쓰는 형식으로 적습니다.
- `.deb`의 `Depends`와 `Recommends`, Arch 패키지의 `depend` 및 `optdepend` 줄에는 NSGo 자체의 패키지, 앱의 패키지, 빌드가 링크하는 각 Go 패키지의 항목이 차례로 들어가며, Go 패키지의 항목은 Go가 나열하는 순서를 따르고 이미 나열된 항목과 `depends` 항목이 이미 지정한 Go 패키지의 권장 항목은 제외됩니다. 타겟이나 빌드 태그로 제외된 Go 패키지는 아무것도 추가하지 않습니다.
- 빈 항목이나 줄바꿈이 포함된 항목이 있으면 해당 Go 패키지 이름을 알리며 빌드가 중단됩니다. 버전 1 파일의 `linux`나 이 두 키 이외의 키도 마찬가지입니다.
- AppImage와 스토어 빌드의 Flatpak은 `linux`에서 아무것도 가져오지 않습니다.

## 벤더링 시 파일 보존

Go의 vendor 작업은 패키지 디렉터리 아래의 파일을 유지하기 위해 임베드된 데이터를 필요로 합니다. 패키지는 런타임에 해당 바이트를 사용하지 않고도 매니페스트와 라이선스를 포함하도록 표시할 수 있습니다:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

`path`를 사용할 때는 `lib/*`처럼 로컬 라이브러리 경로도 해당 embed 지시문에 포함하세요. URL 다운로드 방식에서는 Go 패키지에 매니페스트와 라이선스 파일만 있으면 됩니다.

---
title: 네이티브 라이브러리
description: NSGo의 일반 패키징, 라이선스 및 서명 단계를 거쳐 Go 패키지와 함께 사전 빌드된 공유 라이브러리를 배포합니다.
group: 툴체인
order: 23
---

Go 패키지는 패키지 디렉터리에 `nsgo-natives.yaml`을 두어 purego로 로드하는 공유 라이브러리를 함께 배포할 수 있습니다. 앱이 해당 패키지를 임포트하면 NSGo는 타겟에 맞는 선언을 선택하고, 파일을 검증한 뒤 서명 및 인스톨러 생성 단계 전에 포함합니다. 앱에는 별도의 설정이나 패키징 명령이 필요하지 않습니다.

## 라이브러리 선언

```yaml
version: 1
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
```

지원되는 키는 `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64`, `linux_x64`입니다. Go 패키지가 선택되는 모든 타겟을 선언하세요. `[]`는 해당 타겟에 번들 라이브러리가 필요 없음을 명시합니다. 대신 Go 빌드 태그를 사용해 지원하지 않는 타겟에서 패키지를 제외할 수도 있습니다. `macos_universal`은 두 macOS 항목을 사용합니다.

각 라이브러리에는 다음 필드가 있습니다:

- `name`: 설치될 파일 이름입니다. macOS에서는 `.dylib`, Windows에서는 `.dll`, Linux에서는 `.so` 또는 버전이 붙은 `.so.*`로 끝나야 합니다.
- 이 패키지 디렉터리를 기준으로 한 상대 파일 경로인 `path` 또는 라이브러리 자체를 가리키는 HTTP(S) URL인 `url` 중 정확히 하나를 지정합니다. 아카이브 URL이 아니라 압축이 풀린 라이브러리를 게시하세요. URL에는 사용자 이름이나 비밀번호를 포함하지 마세요.
- `sha256`: 원본 파일의 SHA-256 해시값으로, 두 소스 방식 모두에서 필수입니다. 예를 들어 macOS에서는 `shasum -a 256 lib/macos-arm64/libexample.dylib` 명령으로 출력할 수 있습니다.
- `licenses`: 이 패키지 디렉터리를 기준으로 한 원본 라이선스 및 고지 파일의 비어 있지 않은 경로 목록입니다. 이 텍스트는 `THIRD_PARTY_NOTICES.txt`에 포함되어 배포되며, 릴리스 빌드에서 `core.ThirdPartyNotices()`를 통해 확인할 수 있습니다. 정확한 모듈 예외(exact module exception)를 포함한 앱의 기존 [라이선스 정책](building.md#라이선스-고지)은 네이티브 라이선스에도 독립적으로 적용됩니다.

로컬 경로와 심볼릭 링크는 선언하는 패키지 내부에 머물러야 합니다. NSGo는 알 수 없는 필드, 체크섬 불일치, 다른 운영체제 또는 아키텍처용 파일, 그리고 충돌하는 설치 파일 이름을 거부합니다. 두 패키지가 체크섬이 동일한 같은 이름의 파일을 공유할 수는 있습니다. 파일 크기는 1 GiB로 제한됩니다.

## 패키징 및 로드

| 타겟 | 라이브러리 디렉터리 |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows 및 Linux | 실행 파일 옆 |

패키지는 작업 디렉터리가 아니라 실행 파일의 위치를 기준으로 해당 디렉터리에서 라이브러리를 로드합니다. 종속성들이 함께 배포된 디렉터리에서 서로(peer)를 로드하도록 사전 빌드하세요. 필요한 각 피어 파일과 로더가 요구하는 각 파일 이름을 모두 선언하세요.

NSGo는 캐시된 다운로드를 재사용하기 전에 다시 검증하며, 파일 잠금을 통해 동시 빌드를 조율합니다. 콘텐츠 캐시는 운영체제의 사용자 캐시 디렉터리 아래 `nsgo/natives`에 위치합니다. 로컬 파일은 복사되는 시점에 검증됩니다.

유니버설 macOS 패키지는 두 아키텍처 모두에 동일한 라이브러리 이름을 요구합니다. NSGo는 `lipo`로 아키텍처 슬라이스를 합치며, 게시자가 이미 유니버설로 만든 라이브러리를 양쪽 항목에 모두 사용할 수도 있습니다. 라이브러리는 앱보다 먼저 서명된 후 앱의 일반 디스크 이미지, 서명 및 공증 단계를 거칩니다. Windows 서명은 `build.installers: false`일 때도 선언된 DLL을 포함합니다. Linux 인스톨러에는 실행 파일 디렉터리와 동일한 파일 및 고지 사항이 포함됩니다.

개발 빌드와 네이티브 데스크톱 타겟 테스트도 동일한 패키지 선택 및 복사 단계를 사용합니다. 선언은 로컬 replace 및 벤더링된 패키지를 포함해 빌드의 타겟, 태그, 생성된 Go 오버레이를 기준으로 선택됩니다. 다른 타겟들은 현재 이 파일에서 라이브러리를 패키징하지 않습니다.

## 벤더링 시 파일 보존

Go의 vendor 작업은 패키지 디렉터리 아래의 파일을 유지하기 위해 임베드된 데이터를 필요로 합니다. 패키지는 런타임에 해당 바이트를 사용하지 않고도 매니페스트와 라이선스를 포함하도록 표시할 수 있습니다:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

`path`를 사용할 때는 `lib/*`처럼 로컬 라이브러리 경로도 해당 embed 지시문에 포함하세요. URL 다운로드 방식에서는 Go 패키지에 매니페스트와 라이선스 파일만 있으면 됩니다.

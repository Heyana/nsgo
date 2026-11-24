---
title: 코드베이스
description: NSGo의 각 부분이 위치한 곳과 그 이유를 설명하는 문서.
group: 기여하기
order: 81
---

NSGo는 하나의 Go 모듈입니다. 앱은 상위 패키지들을 임포트하고, 각 플랫폼의 백엔드는 하단에 위치해 변경된 내용을 네이티브 뷰의 생성·업데이트·제거 명령으로 받아 처리합니다.

## 패키지

| 패키지 | 역할 |
| --- | --- |
| `nsgo` (루트) | `nsgo.Run` 및 `nsgo.NewView`, 빌드 태그에 따라 백엔드 선택 |
| `core` | 앱의 API: 엘리먼트, 스타일, 클래스, 테마, 시그널, 서비스; 레이아웃 및 텍스트 준비 |
| `base` | 스타일이 없는 컴포넌트(기능별 파일 하나씩)와 복잡한 컴포넌트 패키지(`base/dock`, `base/editor`) |
| `ui` | 스타일이 적용된 `base` 컴포넌트들과 `ui/chat` 같은 패키지 |
| `mobile`, `desktop` | 휴대폰 전용 또는 데스크톱 전용 기능 |
| `tsx` | TSX 앱, 번들러와 컴파일러, TSX에서 임포트하는 모듈 |
| `i18n`, `*/locales` | 메시지 카탈로그 및 이로부터 생성된 코드 |
| `nsgotest` | 헤드리스 또는 플랫폼 위에서 사용자처럼 동작하는 테스트 |
| `internal/engine`, `internal/driver`, `internal/layout` | 엔진 고루틴, 마운트 프로토콜 및 레이아웃 |
| `internal/platform/<target>` | 백엔드: 단일 대상의 네이티브 뷰, 텍스트 및 입력 처리 |
| `cmd/nsgo` | `nsgo` CLI 도구 |
| `examples/` | 갤러리 및 기타 예제 앱 |

패키지는 아래 방향으로만 임포트합니다. `internal/moduletest/layering_test.go`가 모든 대상에서 패키지 의존성 그래프를 검사합니다.

## 설계 문서

[ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md)는 패키지 구조, 스레드 구성, 커밋에서 화면까지의 흐름을 정리하고 `docs/architecture/`의 주제들을 나열합니다. 레이아웃, 텍스트, 스크롤, 테마, CLI 도구 등 각 부분마다 별도의 문서가 있습니다. 코드를 수정하기 전에 해당 부분의 문서를 먼저 읽으세요. 해당 부분이 어떻게 동작하는지, 그리고 NSGo가 설계를 이식한 [Native Shell](https://github.com/longbridge/nsh)과 어디에서 달라졌는지가 기록되어 있습니다.

[AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md)에는 사람과 코딩 에이전트 모두가 따라야 하는 규칙이 담겨 있습니다. 대부분의 변경에 영향을 주는 핵심 규칙은 다음과 같습니다.

- 각 API는 하나의 소속만 가집니다. 엘리먼트와 환경은 `core`, 컨트롤은 `base`, 외관은 `ui`, 특정 기기 종류 전용 기능은 `mobile`과 `desktop`에 둡니다.
- `base`에는 외관이 없으며, `ui` 컴포넌트는 `base` 형태에 외관을 입힌 것입니다.
- 기기에 따라 암묵적으로 달라지는 동작은 없습니다. `core`나 `base`에서 데스크톱이라고 컨트롤을 키우거나 휴대폰이라고 줄이지 않습니다.
- 어디서나 네이티브 뷰를 사용하며 모든 플랫폼에서 동일하게 동작합니다. 한 플랫폼에서 네이티브로 제공하는 기능은 다른 플랫폼이 모방합니다.
- 직접 작성한 C 코드는 사용하지 않으며, x86_64 Android 에뮬레이터 대상을 제외한 모든 대상을 빌드하는 데 macOS, Xcode, Go 외에 다른 도구가 필요하지 않습니다.

플랫폼별로 아직 구현되지 않은 항목은 [로드맵](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md)에 정리되어 있습니다.

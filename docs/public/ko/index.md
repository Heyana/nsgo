---
title: 소개
description: NSGo가 무엇인지, 어디에서 실행되며 어떻게 동작하는지 설명합니다.
order: 1
---

NSGo는 Go로 작성된 크로스 플랫폼 네이티브 UI 프레임워크입니다. Go나 TSX로 앱을 한 번 작성하면 iOS와 App Clips, tvOS, watchOS, Android, macOS, Windows, Linux 및 웹에서 실행되며 각 플랫폼 고유의 뷰로 그려집니다. Go 앱은 WeChat, QQ, Alipay 및 Douyin 미니 프로그램에서도 실험적으로 실행할 수 있지만, 해당 플랫폼들이 앱 크기를 2 MB로 제한하므로 더 큰 앱은 이를 초과합니다. HarmonyOS에서는 ArkUI로 그려지는 프리뷰 형태로 Go 앱이 실행됩니다.

## 작성하는 코드

앱은 하나의 Go main 패키지입니다. 앱의 컴포넌트는 뷰, 텍스트, 이미지, 입력 필드, 그리고 `ui`의 컨트롤 같은 엘리먼트를 반환합니다. 상태는 시그널에 저장되며, 시그널이 변경될 때 이를 읽는 부분만 업데이트됩니다.

```go
func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }},
	}}
}
```

동일한 앱을 TSX로 작성하고 Tailwind 클래스로 스타일을 지정할 수도 있습니다([TSX 앱](tsx.md)).

## 동작 방식

Go 엔진이 반응형 그래프, 스타일, flexbox 및 grid 레이아웃, 텍스트 준비 등 앱 전체를 소유합니다. 각 업데이트는 하나의 변경 배치를 생성하며, 얇은 백엔드가 이를 플랫폼의 네이티브 뷰에 적용합니다. 애니메이션은 플랫폼의 프레임 클록에서 실행되므로 프레임마다 커밋이 발생하지 않습니다.

| 플랫폼 | 렌더링 방식 |
| --- | --- |
| iOS | UIKit 및 CoreText |
| Android | Android Views |
| macOS | AppKit 및 CoreText |
| Windows | Direct2D 및 DirectWrite |
| Linux | GTK 4, Pango 및 Cairo |
| Web | WebAssembly 및 DOM 엘리먼트 |

모든 대상은 Go가 설치된 Mac에서 빌드되며, iOS에는 Xcode도 필요합니다. `nsgo` 도구는 앱을 생성, 실행, 테스트 및 패키징합니다.

## 현재 상태

NSGo는 아직 초기 단계입니다. 0.1.0이 첫 번째 릴리스이며 API가 아직 안정화되지 않았고, 구현되지 않은 내용은 [로드맵](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md)에 정리되어 있습니다. [Native Shell](https://github.com/longbridge/nsh)의 설계를 Go로 포팅한 프레임워크입니다.

## 문서 읽는 방법

각 페이지는 하나의 질문에 답합니다. 별도의 설명이 없는 한 코드 예제는 컴포넌트의 일부이며, `cx`는 컴포넌트가 전달받는 `*core.Scope`입니다. 앱에서는 외관이 적용된 `ui`의 컴포넌트를 사용하며, `base`에는 컴포넌트 작성자를 위해 외관이 없는 동일한 컨트롤들이 들어 있습니다.

- **시작하기**: [설정](getting-started.md), 그리고 단계별로 만들어 보는 예제 앱인 [할 일 앱](tutorials/todo.md), [메모 앱](tutorials/notes.md), [인기 GitHub 저장소](tutorials/repos.md).
- **툴체인**: [실행 및 디버깅](debugging.md), [테스트](testing.md), [빌드 및 배포](building.md), [임베딩](embedding.md).
- **컴포넌트**: [자주 사용하는 컴포넌트](ui.md), 변형(variants), 상태 및 접근성.
- **핵심 개념**: [컴포넌트와 상태](components.md), [레이아웃](layout.md), [스타일](styles.md), [텍스트](text.md), [목록](lists.md), [내비게이션](navigation.md), [폼](forms.md), [현지화](localization.md), [TSX](tsx.md) 등.
- **플랫폼 기능**: [플랫폼 서비스](services.md), [알림](notifications.md), [햅틱](haptics.md), [위젯](widgets.md), [Live Activities](live-activities.md), [권한](permissions.md).
- **컴포넌트 작성**: [다른 사람의 앱을 위한 컴포넌트](authoring/index.md) — 스타일 미적용 및 스타일 적용 컴포넌트.
- **기여하기**: [소스에서 NSGo 빌드하기](contributing.md), 코드 구조 파악 및 변경 사항 보내기.
- [라이브 데모](https://nsgo.dev/gallery/): 브라우저에서 직접 확인하는 모든 컴포넌트.

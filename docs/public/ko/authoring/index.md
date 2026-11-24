---
title: 컴포넌트 작성
description: NSGo 자체의 base 및 ui처럼 다른 사람들이 앱에 넣을 수 있는 컴포넌트를 만듭니다.
group: 컴포넌트 작성
order: 70
---

이 문서들은 컨트롤 라이브러리, 디자인 시스템 또는 앱이 Go 패키지로 임포트하는 복합 뷰를 만드는 컴포넌트 작성자를 위한 안내서입니다. [컴포넌트와 상태](../components.md) 및 [스타일과 테마](../styles.md)를 미리 이해하고 있다고 가정합니다.

## 컴포넌트

컴포넌트는 앱에서와 마찬가지로 `Render` 메서드를 가진 타입입니다. 필드는 앱이 설정하는 값들이며, 앱이 지정하지 않은 필드는 제로 값을 가지므로 각 필드의 제로 값이 가장 일반적인 기본 동작이 되도록 설계해야 합니다.

```go
package chips

import "github.com/nsgo-dev/nsgo/core"

// Chip is a short label in a pill.
type Chip struct {
	Text string
}

// Render implements core.Node.
func (c Chip) Render(*core.Scope) core.Node {
	return core.View{Class: "self-start rounded-full px-3 py-1", Children: []core.Node{
		core.Text{Content: c.Text, Class: "text-sm"},
	}}
}
```

앱에서는 이를 일반 요소처럼 사용합니다: `chips.Chip{Text: "New"}`.

## 동작과 외형

NSGo는 각 컨트롤을 두 형태로 분리해 유지합니다. `base`는 외형 없이 동작, 상태, 접근성만 담당하며 앱으로부터 부분별 스타일을 모두 전달받습니다. `ui`는 `base` 형태 위에 외형을 입히므로, 실제 동작 로직은 한 곳에만 존재합니다. 라이브러리도 이와 같은 분리를 따를 수 있습니다:

- **스타일 없는(unstyled)** 컴포넌트는 누르기, 키보드, 포커스, 스크린 리더가 읽는 내용을 소유하고 `base.Switch`처럼 각 부분의 스타일을 노출합니다. 앱과 다른 라이브러리가 여기에 자체 외형을 입힙니다([부분과 상태](parts.md)).
- **스타일 적용(styled)** 컴포넌트는 스타일 없는 컴포넌트를 감싸며 `ui.Switch`처럼 현재 적용된 테마에서 스타일을 선택하는 일만 합니다([외형과 테마](looks.md)).

배지처럼 공유할 만한 동작이 없는 컴포넌트는 스타일 적용 버전만 단독으로 작성해도 됩니다.

## 패키지 구성

- 한 파일에는 하나의 컴포넌트만 두고 컴포넌트 이름을 따서(`chip.go`) 파일명을 정하며, 그 컴포넌트만 사용하는 타입을 함께 둡니다. 여러 컴포넌트가 공유하는 요소는 별도 파일로 분리합니다.
- 복잡한 컴포넌트에는 `base/dock`의 `dock.Area`처럼 별도의 패키지를 두고 API 이름에서 컴포넌트명을 생략합니다.
- 컴포넌트가 출력하는 문구는 패키지의 `locales/`에 둡니다([단어, 테스트 및 배포](shipping.md)).
- 기기별로 암묵적으로 다르게 동작하지 않도록 합니다. 휴대폰이나 데스크톱에서 달라지는 크기나 형태는 앱이 설정하는 필드로 노출합니다.

## 다음 단계

- [부분과 상태](parts.md): 제어되는 값, 부분별 스타일링, 접근성.
- [외형과 테마](looks.md): 테마에서 색상 가져오기, 변형과 크기.
- [스타일 없는 컴포넌트](base.md): 스타일을 입히거나 기반으로 삼을 수 있는 NSGo 자체 컨트롤.
- [단어, 테스트 및 배포](shipping.md): 번역, 테스트, TSX 지원.

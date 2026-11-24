---
title: 단어, 테스트 및 배포
description: 컴포넌트의 문구를 번역하고, 사용자처럼 테스트하며, Go 및 TSX 앱에 제공합니다.
group: 컴포넌트 작성
order: 74
---

## 단어

스크린 리더용 버튼 이름처럼 컴포넌트가 직접 말하는 문구는 문자열 리터럴 대신 패키지의 카탈로그를 통해 제공하세요. 라이브러리에 원본 언어 카탈로그를 담은 `locales/` 패키지와 코드 생성을 위한 파일을 추가합니다:

```yaml
# locales/en.yaml
clear:
  message: Clear
  description: What screen readers say of the button that empties a chip input.
```

```go
// Package locales says the chips' words in the language the app shows.
package locales

//go:generate go tool nsgo i18n
```

`go generate ./locales`를 실행하면 메시지마다 함수가 생성되며, 컴포넌트는 스코프와 함께 이를 호출합니다. 예를 들어 `locales.Clear(cx)`는 앱이 표시 중인 언어로 해당 문구를 반환합니다. 앱이 Go 도구만으로 빌드할 수 있도록 생성된 `catalog_gen.go`도 커밋하세요. 번역할 언어마다 파일(`zh-Hans.yaml` 등)을 추가하며, 앱은 [NSGo 자체 문구](../localization.md#nsgos-own-words)와 마찬가지로 패키지 임포트 경로를 키로 삼아 자체 카탈로그에서 다른 언어를 제공할 수 있습니다.

## 테스트

`nsgotest`는 사용자가 사용하는 방식 그대로 컴포넌트를 헤드리스(headless)로 테스트합니다. 작은 루트에 컴포넌트를 마운트하고 역할과 텍스트로 상호작용하세요:

```go
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestToggle(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, false)
		return chips.Toggle{On: on.Get, OnChange: on.Set, Label: "Bold"}
	})
	app.Press("role:switch Bold")
	if !app.Find("role:switch Bold").Props.Checked {
		t.Fatal("the toggle did not turn on")
	}
}
```

라이브러리 루트에서 `go tool nsgo test`를 실행하면 모든 패키지를 테스트하며, `-target macos` 같은 타겟을 지정하면 네이티브 창에서 동일한 테스트를 실행합니다. 하드코딩된 색상이나 문구를 잡아내려면 라이트/다크 모드 양쪽과 의사 로케일(`-lang en-XA`)을 모두 테스트하세요. 자세한 내용은 [테스트](../testing.md)를 참고하세요.

## 배포

컴포넌트 라이브러리는 Go 모듈이므로 앱이 `go get`으로 받아 패키지를 임포트합니다. 스타일 없는 버전과 스타일이 적용된 버전이 모두 있을 때는 서로 다른 패키지로 분리해 두어, 독자적인 외형을 쓰는 앱이 사용하지 않을 스타일을 임포트하지 않도록 하세요.

사이트의 [컴포넌트](https://nsgo.dev/components/) 페이지는 다른 개발자들이 사용할 수 있는 라이브러리를 소개합니다. 여러분의 라이브러리를 올리려면 `docs/public/components/`에 문서와 스크린샷을 추가하세요.

TSX 앱은 `init` 함수에서 등록된 Go 컴포넌트를 사용할 수 있습니다:

```go
func init() {
	tsx.Register("example.com/chips", map[string]core.Node{"Toggle": Toggle{}})
}
```

이후 `import { Toggle } from "example.com/chips"`로 불러오면 구조체 필드를 props로 전달할 수 있습니다([TSX 앱](../tsx.md#go-from-tsx)).

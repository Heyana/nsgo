---
title: 웹에 게시하기
description: 문서 시맨틱, 페이지 메타데이터 및 CDN 에셋과 함께 NSGo 앱을 게시합니다.
group: 툴체인
order: 24
---

## 앱 빌드하기

```sh
go tool nsgo build -target web
```

생성된 디렉터리를 HTTP로 제공합니다. 릴리스 에셋에는 이름에 콘텐츠 해시가 포함되어 있으므로 immutable로 캐시하고, `index.html`, `nsgo.json`, `manifest.webmanifest`는 재검증(revalidate)하세요. 웹 앱은 NSGo의 레이아웃, 스크롤, 포커스, 포인터 동작을 그대로 유지합니다.

## 제목과 링크

`Role: core.RoleHeader`가 설정된 `core.View` 또는 `core.Text`는 `AccessibilityLevel`에 따라 `h1`부터 `h6`까지의 요소가 되며, 레벨이 지정되지 않으면 `h2`가 됩니다. `base/textview`의 Markdown 제목은 각자의 레벨을 유지합니다. 브라우저의 기본 제목 여백과 글꼴은 앱의 스타일을 변경하지 않습니다.

`base.Link`와 `ui.Link`는 `Href`를 앵커(anchor)로 게시합니다. 일반 클릭은 앱의 `OnOpen`과 `OnPress`를 실행하며, 가운데 버튼 클릭이나 보조 키를 누른 채 클릭하면 브라우저의 링크 동작을 사용합니다. 앱에서 `OnContextMenu`를 처리하지 않으면 브라우저가 기본 링크 컨텍스트 메뉴를 제공합니다. 비활성화된 링크에는 활성 주소가 없습니다. 커스텀 링크 역할 뷰의 경우 `core.View.Web`을 `core.WebViewProps{Href: "/notes"}`로 설정하거나 `HrefFunc`를 바인딩하세요. TSX에서는 `web={{href: "/notes"}}`를, 제목에는 `accessibilityLevel={1}`을 사용합니다. 안전하지 않은 실행 가능한 URL 스킴은 거부됩니다. 제한된(confined) 콘텐츠는 범위가 지정된 누르기 핸들러 안에서 탐색을 처리하며 브라우저 주소를 노출하지 않습니다.

## 페이지 메타데이터

`github.com/nsgo-dev/nsgo/web`을 임포트하고 페이지의 콘텐츠 옆에 `web.Head`를 배치합니다:

```go
core.Group{
	web.Head{Title: "Notes", Description: "Your saved notes"},
	pageContent,
}
```

`TitleFunc`과 `DescriptionFunc`는 시그널을 추적합니다. 나중에 표시되는 헤드는 자신이 설정한 필드를 덮어쓰고 나머지 필드는 상속받습니다. 페이지를 숨기거나 제거하면 이전 헤드가 복원되며, 모든 헤드를 제거하면 원래 문서가 복원됩니다. 빈 정적 필드는 상위 값을 상속받고, 빈 문자열을 반환하는 함수는 해당 값을 지웁니다. 헤드는 레이아웃 노드를 추가하지 않습니다. 네이티브 타겟, 임베디드 앱, 제한된 콘텐츠는 호스트 문서를 변경하지 않습니다.

TSX에서는 `nsgo/web`에서 `Head`를 임포트하고 `<Head title={title()} description="Your saved notes" />`처럼 작성하세요. 정적 HTML 미리 보기의 경우 사이트 배포 과정에서 각 URL을 사전 렌더링하세요. `Head`는 실행 중인 문서를 업데이트합니다.

## CDN에서 에셋 제공하기

로더는 모든 앱 파일을 `nsgo.json` 기준으로 해석합니다. 페이지에서는 다른 출처(origin)로부터 앱을 로드할 수도 있습니다:

```js
const url = new URL("https://cdn.example.com/notes/nsgo.json");
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url).href);
await mount(null, url.href, { config });
```

CORS를 통해 페이지의 출처가 CDN의 파일을 가져올 수 있도록 허용하고, WebAssembly 파일은 `application/wasm`으로 제공하세요. 앱이 사용하는 모든 딥 라우트(deep route)에 대해서는 앱 진입점 HTML을 제공해야 합니다. 특정 요소 안에 마운트하는 방법은 [임베딩](embedding.md#web)에서 다룹니다.

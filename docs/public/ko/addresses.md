---
title: 애플리케이션 주소
description: 브라우저 히스토리를 통해 웹 앱의 주소를 읽고 업데이트합니다.
group: 핵심 개념
order: 59
---

`cx.AppURL()`은 애플리케이션의 주소를 추적 가능한 값으로 읽습니다. 웹에서는 루트가 렌더링되기 전에 초기 페이지 URL을 사용할 수 있으므로, 새로 고침이나 공유 링크로 접속했을 때도 동일한 뷰를 선택할 수 있습니다. URL을 뷰에 어떻게 매핑할지는 앱이 결정합니다.

```go
func Root(cx *core.Scope) core.Node {
    return core.View{Children: []core.Node{
        core.Text{ContentFunc: cx.AppURL},
        base.Button{Label: "Open film", OnPress: func(core.PressEvent) {
            if err := cx.SetAppURL("/films/42", core.AppURLOptions{
                History: core.HistoryPush,
            }); err != nil {
                // Show the error in the app's usual way.
            }
        }},
    }}
}
```

`SetAppURL`은 새 페이지를 요청하거나 내비게이션 스택을 이동하지 않고 추적된 주소를 즉시 업데이트합니다. 기본적으로 현재 브라우저 히스토리 항목을 교체하며, `HistoryPush`를 지정하면 새 항목을 추가합니다. 같은 커밋 안에서 내비게이션 또는 모달 푸시가 함께 일어나면 그쪽이 히스토리 항목을 제공하므로, 하나의 동작이 두 개의 뒤로 가기 단계를 만들지 않습니다. 한 커밋 안에 여러 번 쓰기가 일어나면 마지막 주소가 유지되며 푸시 요청도 보존됩니다.

상대 주소는 현재 주소 또는 명시적 HTML base를 기준으로 해석됩니다. 브라우저 주소는 페이지의 동일한 출처(origin) 안에 머물러야 합니다. 유효하지 않은 주소, 공백, 제어 문자, 백슬래시, 16 KiB를 초과하는 주소는 값을 변경하지 않고 오류를 반환합니다.

`cx.OnAppURLChange(func(change core.AppURLChange) { ... })`는 브라우저 히스토리 이동과 해시 편집을 관찰합니다. `change.Reason`은 `"back"`, `"forward"` 또는 `"address"`이며, 반환된 함수로 구독을 해제할 수 있고 스코프가 정리되면 리스너도 제거됩니다. 초기 주소를 읽으려면 `AppURL`을 사용하세요. 앱 자체의 쓰기는 바인딩을 업데이트할 뿐 변경 알림을 보내지 않습니다. 이동 후에도 주소가 같다면 변경 알림이 발생하지 않습니다.

브라우저의 뒤로 가기는 여전히 모달을 닫거나 내비게이션 화면을 팝합니다. 앱이 뒤로 가기를 거부하면 기존 히스토리 항목과 주소가 복원됩니다. 닫기 핸들러는 대체할 주소를 직접 설정할 수 있습니다. 프로그래밍 방식의 팝은 비동기 히스토리 이동이 끝난 뒤에 주소를 업데이트하며, 이동 도중에 도착한 쓰기는 이동이 끝날 때까지 대기합니다. 이 플레이어가 만든 항목을 넘어 뒤로 이동하면 이전 로드에서 남아 있던 항목을 포함해 앱을 벗어나게 됩니다.

`/films/42` 같은 경로를 사용하려면 배포되는 앱 디렉터리를 설정하고 서버가 애플리케이션 라우트에 대해 `index.html`을 서빙하도록 구성하세요:

```yaml
web:
  base: /        # /my-app/ when deployed below that directory
```

`web.base`는 깊은 주소(deep link)를 직접 열었을 때도 스크립트, 스타일, Wasm 및 리소스가 정상적으로 로드되도록 HTML base를 기록합니다. 이를 설정하지 않으면 기존의 상대 애셋 경로가 유지되며, 쿼리나 해시 기반 주소는 설정 없이도 그대로 사용할 수 있습니다. 서버의 폴백 규칙은 실제 애셋 요청을 건드리지 않아야 합니다. 엄격한 콘텐츠 보안 정책(CSP)에서는 이 동일 출처 base(`base-uri 'self'`)를 허용해야 합니다.

다른 페이지에 임베드된 앱은 주소를 내부적으로만 유지하고 상위 페이지의 히스토리를 건드리지 않습니다. 동일 출처 iframe은 `data-nsgo-history`를 통해 명시적으로 히스토리 연동을 활성화할 수 있습니다. 격리된 하위 트리(confined subtree)는 애플리케이션의 주소를 읽거나 변경할 수 없습니다.

다른 타겟에서는 `AppURL`이 앱으로 열린 마지막 링크 또는 `SetAppURL`이 저장한 값을 유지합니다. 값을 설정해도 네이티브 창이나 시스템 브라우저는 바뀌지 않습니다. 미니 프로그램 역시 호스트의 라우트를 바꾸지 않고 이 값을 유지합니다.

TSX에서도 동일한 상태를 사용합니다:

```tsx
import { UrlScheme } from "nsgo";

UrlScheme.url();
UrlScheme.setUrl("/films/42", { history: "push" });
const stop = UrlScheme.on("change", ({ url, reason }) => {
  // Update the app's route from the browser's choice.
});
```

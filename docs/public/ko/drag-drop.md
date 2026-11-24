---
title: 드래그 앤 드롭
description: 하나의 창 안에서 뷰 간에 애플리케이션 값을 전달합니다.
group: 핵심 개념
order: 57
---

`base/dragdrop`은 한 창 안의 같은 `Area` 내에서 소스와 대상을 연결합니다. 앱은 자체 뷰, 스타일, 선택 상태, 그리고 값을 처리하는 동작을 그대로 유지합니다. 소스는 기본적으로 4포인트 이상 움직였을 때 드래그를 시작하므로 일반적인 누르기 동작은 영향을 받지 않습니다.

```go
import "github.com/nsgo-dev/nsgo/base/dragdrop"

dragdrop.Area{Children: []core.Node{
	dragdrop.Source{
		View: core.View{
			Label: "Film", TestID: "poster",
			Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Poster"}},
		},
		Data: func() any { return filmID },
		Preview: func(any) core.Node { return core.Text{Content: "Film"} },
	},
	dragdrop.Target{
		View: core.View{
			TestID: "playlist",
			Style: core.Style{Width: core.Pt(160), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Playlist"}},
		},
		Accept: func(value any) bool { _, ok := value.(string); return ok },
		OnDrop: func(e dragdrop.Event) { addToPlaylist(e.Data.(string)) },
	},
}}
```

`Data`는 드래그가 시작될 때 한 번 실행됩니다. 여러 선택 항목을 함께 전달하려면 슬라이스를 반환하고, 스냅샷으로 유지되어야 하는 가변 데이터라면 이 시점에 복사하세요. `Data`가 nil을 반환하면 드래그가 비활성화됩니다. `Threshold`는 드래그 시작에 필요한 이동 거리를 설정합니다. `Activation: dragdrop.AfterLongPress`는 먼저 `View.DelayLongPress`(기본값 500 ms) 동안 기다린 뒤 움직임을 감지하므로, 스와이프 시 스크롤이 먼저 동작해야 하는 곳에서 명시적으로 선택하세요.

`View`는 누르기 핸들러와 접근성을 포함한 일반 props를 그대로 유지합니다. 소스는 `View.Responder`를 소유하므로 이를 비워 두세요. 소스의 자식에는 일반적인 동작을 수행하는 버튼을 포함할 수 있습니다. `Preview`는 입력을 가로채거나 소스를 다시 마운트하지 않고 포인터를 따라다니며, 설정하지 않으면 소스의 레이블이 표시됩니다. 소스에는 레이블이나 프리뷰 중 하나를 반드시 지정하세요. 중첩된 `Area`는 서로 독립적으로 동작합니다.

대상이 값을 수락하려면 `OnDrop`이 있어야 합니다. 가장 안쪽 대상이 먼저 기회를 얻고, `Accept`가 거부하면 수락하는 조상 대상이 받을 수 있습니다. `OnEnter`, `OnMove`, `OnLeave`로 대상의 외관을 업데이트할 수 있습니다. 각 이벤트는 `Data`, 대상 내부 기준 좌표인 `Position`, 표면 기준 좌표인 `PagePosition`을 전달합니다. 활성 대상을 비활성화하거나 제거하면 진입 상태가 해제되며, 포인터를 놓을 때 수락 여부를 다시 확인합니다. 다른 뷰 뒤에 가려졌거나 클리핑되었거나 비활성화되었거나 모달 아래에 있는 대상은 드롭을 받을 수 없습니다.

`OnStart`와 `OnEnd`는 소스의 세션을 알립니다. `OnEnd`는 `Result{Data, Dropped, Target}`을 받습니다. Escape 키, 네이티브 터치 취소, 소스 비활성화 또는 제거, 수락하지 않는 대상 바깥에서 놓기는 드롭 없이 종료됩니다. 대상 배치는 마지막으로 커밋된 지오메트리를 사용하며 커밋 사이에 진행되는 네이티브 애니메이션은 샘플링하지 않습니다. 이 기능은 창 내부에서 값을 전달하기 위한 것이며, 시스템 파일 드롭에는 여전히 `OnFileDrop`을 사용합니다.

같은 작업을 수행할 수 있는 메뉴, 키보드 명령 또는 접근성 동작을 함께 제공하세요. 드래그는 그 작업에 대한 단축 수단입니다. Go 테스트에서는 `app.DragTo("id:poster", "id:playlist")` 또는 선택한 표면의 `DragTo`를, TSX 테스트에서는 `await app.dragTo("id:poster", "id:playlist")`를 사용합니다. 두 뷰 모두 이미 보이는 창 안에 들어 있어야 하며, 이 단계는 드롭을 합성하거나 드래그 중에 스크롤하지 않고 리스폰더를 통해 터치 시퀀스를 전달합니다. [테스트](testing.md)를 참고하세요.

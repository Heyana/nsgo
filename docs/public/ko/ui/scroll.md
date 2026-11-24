---
title: 스크롤
description: 페이징, 스냅, 줌, 새로고침을 지원하는 플랫폼 스크롤 뷰.
group: 컴포넌트
order: 39
---

`core.Scroll`은 플랫폼 고유의 스크롤 뷰입니다. 자식 요소는 세로 열로 배치되거나 `Horizontal`을 지정하면 가로 행으로 배치되며, 콘텐츠가 스크롤되는 동안 스크롤 뷰는 컨테이너를 가득 채웁니다.

```go preview=scroll/basic height=240
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

패딩은 스크롤 뷰가 아니라 콘텐츠에 적용하므로 스크롤 막대는 가장자리에 붙어 있게 됩니다. 스타일에서 달리 지정하지 않으면 스크롤 뷰는 부모에 맞춰 축소됩니다. 형제 요소가 축소되는 동안에도 요청한 크기를 유지해야 한다면 `NoShrink`를 설정하세요.

## 동작

`Behavior`는 스크롤 방식을 지정합니다. 여기에는 `Paging`, `SnapToAlignment`와 함께 사용하는 `SnapToInterval` 및 `SnapToOffsets`, `DecelerationRate`, `NoBounces`, `ContentInset`, 그리고 키보드 규칙이 포함됩니다.

```go preview=scroll/paging height=240
core.Scroll{
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: pages,
}
```

`HideIndicator`는 플랫폼의 스크롤 막대를 숨기고, `Bar`는 앱 자체의 스크롤 막대를 그립니다. `MinZoomScale`과 `MaxZoomScale`은 핀치 줌을 활성화하며, `NoPinchZoom`은 핀치 동작을 무시합니다. `CenterContent`는 뷰포트보다 작은 콘텐츠를 가운데 정렬합니다.

## 새로고침과 이벤트

`OnRefresh`를 지정하면 뷰를 새로고침할 수 있으며, 작업이 실행되는 동안 `Refreshing`을 설정하세요. `PullValue`는 앱 자체의 표시기(indicator)를 위해 콘텐츠가 얼마나 당겨졌는지를 추적하며, `Behavior.RefreshIndicator`는 그 높이를 설정합니다.

```go
refreshing := core.NewSignal(cx, false)
core.Scroll{
	Refreshing: refreshing.Get,
	OnRefresh:  func() { refreshing.Set(true); reload(cx) },
	Children:   content,
}
```

`OnScroll`은 턴마다 최대 한 번, 또는 `ScrollEventThrottle`마다 한 번 오프셋을 보고합니다. `Stale`은 코드로 지정한 스크롤을 플랫폼이 적용하기 전의 이벤트를 표시합니다. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd`, `OnScrollEnd`는 사용자의 제스처 구간을 앞뒤로 감쌉니다.

`OffsetValues`는 프레임마다 커밋 없이 접히는 헤더 등을 구현할 수 있도록 메인 스레드에서 오프셋을 추적합니다. `Ref`는 코드에서 `ScrollTo`, `ScrollToEnd`, `ZoomTo`로 스크롤하며, 내부에 있는 어떤 뷰의 ref든 `Reveal`을 호출하면 자신을 감싼 모든 스크롤 뷰를 스크롤하여 화면 안으로 들어오게 합니다:

```go
row.Reveal(core.RevealOptions{Vertical: core.ScrollAlignCenter, Animated: true})
```

`RevealBox`는 뷰 자체가 아니라 텍스트의 `TextRangeBoxes`에서 얻은 단어 영역 같은 뷰 내부의 특정 사각 영역을 화면 안으로 가져오며, `Margin`은 가장 바깥쪽 스크롤 뷰의 가장자리 위에 놓이는 바를 위해 여유 공간을 확보합니다.

키보드로 포커스가 이동한 뷰는 자동으로 스크롤되어 화면에 표시됩니다.

## 레이아웃

`Horizontal`은 가로로 스크롤합니다. `Behavior.ContentInset`은 콘텐츠와 함께 스크롤되는 여백을 콘텐츠 주변에 추가합니다. 안전 영역을 채우는 페이지에서는 스크롤 뷰가 아니라 콘텐츠에 `PaddingEnv`를 사용하세요.

## 접근성

`Label`은 스크롤 영역의 이름을 지정하고, `Role`은 목록 상자(list box)처럼 그 영역이 무엇인지 나타냅니다. 플랫폼의 스크롤 제스처, 키보드 스크롤, 스크린 리더 스크롤이 그대로 유지되며, `Busy`는 뷰가 아직 로딩 중임을 보고합니다.

## 플랫폼

모든 백엔드는 플랫폼의 스크롤 뷰를 사용하므로 플링(fling), 바운스, 스크롤 막대 및 터치 스크롤이 네이티브로 동작합니다. 가장자리 효과는 플랫폼마다 다릅니다. macOS와 iOS는 러버밴딩(rubber-band), Android는 늘어나기(stretch), Windows와 Linux는 정지, 웹은 브라우저 동작을 따릅니다. `EdgeEffects`와 `EdgeBar`는 앱이 원하는 위치의 콘텐츠 위에 바를 추가합니다. iOS와 Android 휴대폰 및 태블릿에서는 내용이 넘치는 세로·가로 스크롤 뷰의 활성 축을 따라 터치로 드래그할 수 있는 스크롤 핸들이 표시되어 전체 콘텐츠 범위를 빠르게 이동할 수 있습니다. 넘치지 않고 한 화면에 들어맞는 짧은 스크롤 뷰, 줌 중인 뷰, `Behavior.NoScroll`, `HideIndicator`, 그리고 커스텀 `Bar`나 `base.SetScrollbarTheme`을 사용하는 뷰에서는 이 핸들이 숨겨집니다. 테스트에서는 `nsgotest.App.ScrollHandle`이 핸들의 뷰포트 경계를 보고하며, `DragAt` 또는 `Touch`로 직접 드래그할 수 있습니다. Windows에서는 오버레이 표시기가 마우스 휠, 터치, 막대 드래그 및 그 관성(momentum)에 대해서만 나타나며, 앱이 호출한 `ScrollTo`, `ScrollBy`, reveal, 인셋 또는 애니메이션에는 나타나지 않습니다.

## 관련 문서

- [목록과 스크롤](../lists.md): 스크롤과 목록에 대한 전체 가이드.
- [List](list.md): 행을 재사용하는 목록.
- [레이아웃](../layout.md): 패딩과 안전 영역.
- 갤러리의 Scroll 및 Lists 페이지.

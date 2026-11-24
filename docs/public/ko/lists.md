---
title: 목록과 스크롤
description: 네이티브 제스처를 지원하는 스크롤 뷰와 항목 수에 관계없이 행을 재사용하는 목록.
group: 핵심 개념
order: 55
---

## 스크롤 뷰

`core.Scroll`은 플랫폼 고유의 스크롤 뷰이므로 플링(fling), 바운스, 스크롤 막대, 스냅, 터치 스크롤이 모두 네이티브로 동작합니다. 자식 요소는 필요한 길이만큼 세로 열로 배치되며, `Horizontal`을 지정하면 가로 행으로 배치됩니다.

```go
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

스크롤 뷰가 컨테이너를 가득 채우고 패딩은 콘텐츠에 들어가므로 스크롤 막대는 가장자리에 붙어 있게 됩니다. 스타일에서 달리 지정하지 않으면 스크롤 뷰는 부모에 맞춰 축소됩니다. 형제 요소가 축소되는 동안에도 요청한 크기를 유지해야 한다면 `NoShrink`를 설정하세요. `HideIndicator`는 플랫폼의 스크롤 표시기를 숨기고, `Bar`는 앱 자체의 스크롤 막대를 그립니다.

`Behavior`와 `BehaviorFunc`는 스크롤 방식을 결정합니다. 여기에는 `Paging`, `SnapToAlignment`와 함께 사용하는 `SnapToInterval` 및 `SnapToOffsets`, `DecelerationRate`, `NoBounces`, `ContentInset`, 그리고 키보드 규칙이 포함됩니다. `MinZoomScale`과 `MaxZoomScale`은 핀치 줌을 활성화하고, `OnRefresh`와 `Refreshing`은 당겨서 새로고침을 구현하며, `OffsetValues`는 스크롤과 함께 움직이는 효과를 위해 메인 스레드에서 오프셋을 추적합니다.

```go
page := core.Scroll{
	OnScroll: func(e core.ScrollEvent) { y.Set(e.Offset.Y) },
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: []core.Node{...},
}
```

`OnScroll`은 턴마다 최대 한 번, 또는 `ScrollEventThrottle`마다 한 번 오프셋을 보고합니다. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd`, `OnScrollEnd`는 사용자의 제스처 시작과 끝을 감쌉니다. `Ref`는 코드에서 `ScrollTo`, `ScrollToEnd`, `ZoomTo` 및 목록용 `ScrollToIndex`로 스크롤할 수 있게 합니다.

## 목록

`core.List`는 화면 근처의 행만 생성하면서 네이티브 스크롤 뷰 위에 긴 데이터를 렌더링합니다. `Count`는 전체 행 수를 지정하고 `Row`는 행 하나를 빌드합니다. `Key`는 각 행에 고유 식별자를 부여하여, 위쪽에 행이 추가되거나 제거되더라도 남아 있는 행은 기존 셀을 그대로 유지합니다.

```go
core.List{
	Count:         func() int { return len(items.Get()) },
	Key:           func(i int) any { return items.Get()[i].ID },
	EstimatedSize: 56,
	Row: func(cx *core.Scope, r core.ListRow) core.Node {
		return core.View{Style: core.Style{Padding: core.All(12)}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return items.Get()[r.Index()].Title }},
		}}
	},
}
```

`Row`는 재사용되는 셀마다 한 번만 실행되고 해당 셀이 다른 행에 재사용될 때 `r.Index()`를 업데이트하므로, 행을 빌드할 때 일회성 값을 캡처하지 말고 반응형 함수(`ContentFunc`, `StyleFunc`, `NewMemo`) 안에서 `r.Index()`와 항목 시그널을 읽으세요. 유휴 상태의 셀은 대기(parked) 상태가 되어, 셀이 다시 활성 행에 바인딩될 때까지 계산이 보류됩니다.

`Header`, `Footer`, `Empty`, `Separator`는 행의 앞뒤와 사이에 들어가는 콘텐츠를 담습니다. `NumColumns`는 행을 나란히 배치하고, `Gap`과 `ColumnGap`은 행 사이의 간격을 띄우며, `Horizontal`은 가로로 스크롤합니다. `StickyHeaderIndices`는 해당 행들을 다음 고정 헤더가 밀어낼 때까지 뷰포트 시작 부분에 고정하고, `StickyHeaderOffset`은 그 위의 헤더와 겹치지 않도록 여백을 둡니다.

`OnViewableItemsChange`는 `ViewabilityConfig`의 임계값에 따라 화면에 보이는 행들을 보고하며, `OnLoadMore`는 뷰포트가 끝에 가까워질 때 행 수당 한 번씩 실행됩니다. `OnLoadMoreAtStart`는 시작 부분에서 같은 역할을 수행하고, `InitialScrollIndex`는 목록을 지정된 행 위치에서 엽니다. `Alignment: core.ListAlignEnd`는 채팅처럼 행이 읽기 순서대로 배치된 목록의 끝부분에 위치를 맞춥니다. 즉, 가장 최신 행에서 열리고, 사용자가 끝에 머무는 동안 새 행을 따라가며, 짧은 대화도 아래쪽에 배치합니다. 반면 `Inverted`는 행 0을 끝에 배치하고, `MaintainVisibleContentPosition`은 사용자가 끝 근처에 있을 때 늘어나는 목록의 끝을 따라갑니다.

```go
core.List{Ref: list, Count: count, EstimatedSize: 64, Row: row, OnLoadMore: loadMore,
	StickyHeaderIndices: headers(cx), Footer: core.View{Style: core.Style{Height: core.Pt(24)}}}
```

`list.ScrollToIndexAt(index, 0.5, 0, true)`는 행을 뷰포트 중앙으로 스크롤하고, `list.ScrollToEnd(true)`는 끝으로 스크롤합니다. `list.ListPosition()`은 읽기 위치를 유지할 수 있도록 뷰포트가 시작하는 지점, 즉 해당 행과 그 안쪽으로 얼마나 스크롤되었는지를 읽어 오며, `list.ScrollToIndexAt(p.Index, 0, -p.Offset, false)`는 새로 만든 목록을 그 위치로 되돌립니다. 목록을 다른 콘텐츠와 함께 한 페이지에 배치할 수도 있습니다. `Viewport: core.ListViewportParent`를 설정하면 바깥쪽 스크롤 뷰가 나머지 콘텐츠와 함께 목록을 스크롤합니다.

## 테이블

`base/datatable`은 가상화된 행과 열, 고정 헤더, 다른 열이 아래로 스크롤되는 동안 왼쪽(`Fixed`) 또는 오른쪽(`FixedRight`)에 고정되는 열, 정렬, 단일 또는 다중 행 선택, 열 크기 조절 및 이동, 그리고 앱 고유의 셀을 갖춘 백만 행 규모의 테이블을 표시합니다. `MultiSelect`는 Command 또는 Control 토글, Shift 범위 선택, Shift를 누른 상태의 키보드 선택 확장을 지원하며, `SelectedRows`와 `OnSelectedRowsChange`를 통해 앱이 선택된 행을 제어할 수 있습니다. `CellTooltip`은 셀에 툴팁을 지정하며 이는 스크린 리더에도 셀을 설명해 주고 `Styles.CellTooltip`으로 스타일링됩니다. `RenderCell`이 그리는 요소는 포인터 이벤트를 받지 않으므로 셀 내부의 `ui.Tooltip`은 표시되지 않습니다. `base.Table`은 소규모 데이터용 일반 테이블이고, `base/plot`은 차트를 그립니다.

로케일이나 애플리케이션 상태를 따르는 그리드 이름에는 `DataTable.LabelFunc`를 사용하세요. `ColumnGroup.LabelFunc`는 그룹이 화면에 보이는 동안 그룹화된 열 머리글을 업데이트합니다.

## 다음 단계

- [애니메이션](animation.md): 스크롤 뷰를 따라가는 효과.
- [텍스트](text.md): 줄바꿈되는 텍스트 행.
- [내비게이션](navigation.md): 콘텐츠가 스크롤되는 화면.

---
title: 목록
description: 화면에 보이는 행만 빌드하며 선택과 활성화를 지원하는 임의 개수의 행 목록.
group: 컴포넌트
order: 38
---

`ui.List`는 네이티브 스크롤 뷰 위에 행을 렌더링하며, 선택, 활성화 및 키보드 조작을 지원하면서 화면 근처의 행만 빌드합니다.

```go preview=list/basic height=280
ui.List{
	Count:         func() int { return len(items) },
	Key:           func(i int) any { return items[i].ID },
	EstimatedSize: 44,
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		item := items[row.Index()]
		return ui.Text(item.Title, 13, ui.Regular, colors.Text)
	},
}
```

## 행과 식별자

`Count`는 행의 개수를 지정하고 `Row`는 각 행을 빌드합니다. `Key`는 행에 고유 식별자를 부여하여, 위쪽에 행이 추가되거나 제거되더라도 남아 있는 행은 기존 셀과 상태를 그대로 유지합니다. 행이 추가, 제거되거나 순서가 바뀔 수 있는 경우에는 인덱스 대신 ID를 사용하세요. 인덱스는 셀이 다른 행을 표시할 때 그 셀을 따라 바뀌는 시그널입니다.

```go
ui.List{Count: count, Key: func(i int) any { return items[i].ID }, Row: func(cx *core.Scope, row core.ListRow) core.Node {
	return ui.Text(items[row.Index()].Title, 13, ui.Regular, colors.Text)
}}
```

`EstimatedSize`는 행의 예상 높이로, 0이면 50포인트가 됩니다. 행마다 높이가 달라도 되며, 목록은 빌드한 행의 높이를 측정하고 나머지는 추정합니다.

## 레이아웃

`Header`, `Footer`, `Empty`는 행의 앞뒤와 행이 없을 때 표시할 콘텐츠를 담습니다. `Gap`은 행 사이의 간격을 두고, `Columns`는 행을 나란히 배치하며, 목록의 패딩은 콘텐츠에 적용되므로 스크롤 막대는 가장자리에 그대로 붙어 있습니다. 목록에 높이를 지정하거나 부모를 채우도록 설정하세요.

## 선택과 활성화

`ui.List`는 `ui.NewSelection(cx, multiple)`로 만든 `Selection`을 받습니다. 키, 레이블, 활성화 및 키보드 조작이 이를 따르며, 위/아래 화살표, Home, End 키로 커서를 이동하고, Shift 키로 선택 범위를 확장하며, Mod+A로 전체를 선택하고, 다중 선택에서는 Space 키로 토글하며, Enter 키나 더블 클릭으로 행을 활성화합니다.

```go preview=list/selection height=320
selected := ui.NewSelection(cx, true)
ui.List{
	Count: count, Key: func(i int) any { return items[i].ID },
	Selection: selected,
	OnActivate: func(i int) { open(items[i]) },
	RowLabel:   func(i int) string { return items[i].Title },
	Row: func(cx *core.Scope, row core.ListRow) core.Node { return rowContent(cx, items[row.Index()]) },
}
```

`OnSelectionChange`는 누르기, 키 입력 또는 타자 입력 검색(typeahead)으로 선택된 행이 바뀐 뒤에만 실행되며 앱이 `Selection`을 직접 설정할 때는 실행되지 않으므로, 앱이 다른 경로로 열린 항목으로 선택을 옮기는 동안에도 상세 창이 사용자의 선택을 따를 수 있습니다.

`TapSelects`는 휴대폰에서 탭했을 때 활성화 대신 선택이 이루어지게 합니다. `RowLabel`은 보조 기술이 읽어 주는 텍스트이자 타자 입력 검색(typeahead)이 일치시키는 기준입니다.

## 로딩과 스크롤

`OnLoadMore`는 끝에 가까워질 때 행 개수당 한 번씩 실행됩니다. `Ref`는 `ScrollToIndexAt`, `ScrollToEnd`, `ScrollToIndex`로 스크롤합니다. 그 아래의 기본 목록인 `core.List`에는 저수준 제어에 쓰는 `OnLoadMoreAtStart`, 채팅용 `Inverted`, `StickyHeaderIndices`, `MaintainVisibleContentPosition`, `Viewport`, `ViewabilityConfig`도 있습니다([목록과 스크롤](../lists.md)).

## 접근성

목록은 `Label`로 이름이 지정된 하나의 목록(list) 또는 목록 상자(listbox)이며, 각 행의 이름은 행의 텍스트나 `RowLabel`이 됩니다. 선택을 지원하는 목록은 커서가 위치한 행을 보고하고 키보드로 이동할 때 이를 읽어 줍니다. 스크린 리더가 화면 밖으로 이동하면 목록에 해당 위치로 스크롤하도록 요청하며, 이 과정에서 도달한 행들이 빌드됩니다.

## 플랫폼

네이티브 스크롤 뷰와 가상화 방식은 모든 대상에서 동일합니다. 행은 재사용되므로 행의 상태는 `Key`를 따라야 하며, 빌드 상태를 행 내부가 아니라 앱에 두는 것이 일반적인 규칙입니다.

## 관련 문서

- [목록과 스크롤](../lists.md): 고정 헤더와 추가 로딩을 포함한 전체 가이드.
- [Scroll](scroll.md): 기본 스크롤 뷰.
- [DataTable](../lists.md#테이블): 수많은 행과 열로 구성된 테이블.
- 갤러리의 List, Table, DataTable 페이지.

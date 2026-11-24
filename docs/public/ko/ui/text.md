---
title: 텍스트
description: 단락, 스팬, 링크, 글꼴 및 문서.
group: 컴포넌트
order: 32
---

`core.Text`는 플랫폼의 텍스트 엔진으로 조판되는 하나의 단락입니다. `ui.Text`와 `ui.Textf`는 `ui` 타이포그래피로 한 줄을 표시하기 위한 헬퍼이지만, 반환되는 요소는 여전히 `core.Text`이므로 `Style`과 `Spans`도 받을 수 있습니다.

```go preview=text/basic
ui.Text("Settings", 20, ui.Bold, colors.Text)
ui.Textf(func() string { return fmt.Sprintf("%d files", n.Get()) }, 13, ui.Regular, colors.Muted)
```

## 콘텐츠

`Content`는 고정 텍스트를, `ContentFunc`는 시그널을 따르는 텍스트를 지정합니다. `MaxLines`는 줄 수를 제한하고 `EllipsizeMode`는 나머지 텍스트를 앞(head), 중간(middle) 또는 끝(tail)에서 줄임표로 자릅니다. `Style.WhiteSpaceNoWrap`은 단락을 한 줄로 유지합니다. `TextAlign`, `TextTransform`, `LineHeight`, `LetterSpacing`은 단락 스타일을 지정하며, `OnTextLayout`은 레이아웃 후 줄 정보를 보고합니다.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) },
	MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 15}}
```

## 스팬과 링크

`Spans`는 각자 고유한 스타일을 가진 텍스트 런(run)들로 텍스트를 구성합니다. `OnPress`가 있는 스팬은 링크가 되며, `Background`와 `CornerRadius`는 모든 줄의 글자 뒤에 그려집니다. `SpansFunc`는 읽는 값이 바뀔 때 런 목록을 다시 반환하며, 구문 강조된 코드나 동적으로 바뀌는 문장에 사용합니다.

```go preview=text/spans
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "Read the "},
	{Content: "guide", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openGuide},
	{Content: " before you start."},
}}
```

`Attachment`는 스팬의 콘텐츠 자리에 아이콘, 이미지 또는 작은 버튼 같은 뷰를 줄 안에 배치합니다. 자체 크기로 레이아웃되며, 단어들과 함께 줄바꿈되고, 자체 누르기 이벤트를 받습니다.

## 글꼴과 스타일

`FontFamily`는 글꼴 패밀리 이름이나 CSS 목록을 지정합니다. `FontSize`, `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LetterSpacing`, `TextDecoration` 그리고 `TextShadowColor` 및 오프셋과 반경으로 텍스트를 스타일링합니다. `core.FontMonospace`는 플랫폼의 고정폭 글꼴 패밀리입니다.

```go
core.Text{Content: "fn main() {}", Style: core.Style{FontFamily: core.FontMonospace, FontSize: 13}}
```

패키징된 글꼴은 첫 번째 뷰가 생성되기 전에 `nsgo.Fonts`로 등록하거나, 앱 실행 중에 `cx.RegisterFont`로 서체를 등록하세요. 글꼴이 도착하기 전에 준비된 텍스트는 글꼴이 등록된 후 다시 준비됩니다.

## 문서

`base/textview.TextView`는 제목, 단락, 목록, 표, 코드, 각주, 알림(alert), 이미지, 수식, 링크 등 동일한 기본 요소로 Markdown이나 HTML을 표시합니다. `textview.Parse`는 Markdown을, `textview.ParseHTML`은 HTML을 읽으며, `Options.Streaming`은 아직 스트리밍으로 도착 중인 문서를 보정합니다.

```go preview=text/document height=240
textview.TextView{Markdown: "## Notes\n\nA **native** document.", Styles: ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) { open(e.URL) }}
```

`textview.NewSearch`는 `TextView`가 표시하는 텍스트를 검색하고 일치하는 부분을 강조 표시합니다. `textview.ProvideSearch`로 여러 뷰에 동일한 검색 객체를 제공한 뒤 `Open`, `SetQuery`, `Next`, `Previous`를 호출하세요. `Count`와 `Selected`가 검색 결과를 보고합니다. 검색 영역은 `textview.PartSearch`와 `textview.PartSearchSelected`로 스타일링합니다.

## 선택과 접근성

`SelectableOn`을 사용하면 사용자가 텍스트를 선택하고 복사할 수 있습니다. `Selectable`이 설정된 `core.View`는 내부의 텍스트들을 문서 순서대로 함께 선택할 수 있게 합니다. `RoleHeader` 같은 `Role`은 보조 기술에 텍스트의 역할을 알려 주며, 텍스트 내용 자체가 레이블이 아닐 때는 `Label`이 이를 대체합니다.

## 플랫폼

플랫폼의 텍스트 엔진이 텍스트 레이아웃을 처리하므로 줄바꿈, 글꼴 및 입력 방식이 플랫폼을 따릅니다. CJK 줄바꿈, 이모지 및 오른쪽에서 왼쪽으로 쓰는(RTL) 텍스트도 시스템을 따릅니다. 앱은 `cx.Env().FontScale()`로 텍스트 크기를 읽을 수 있으며, `NoFontScaling`이 설정되지 않는 한 텍스트는 이 배율을 따릅니다.

## 관련 문서

- [텍스트](../text.md): 텍스트, 스팬, 글꼴 및 문서에 대한 전체 가이드.
- [이미지와 아이콘](../images.md): 인라인 이미지와 아이콘.
- [접근성](../accessibility.md): 제목, 레이블 및 선택.
- 갤러리의 Text 및 TextView 페이지.

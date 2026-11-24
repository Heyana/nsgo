---
title: 텍스트
description: 스팬, 글꼴, 장식, 선택 및 문서와 함께 텍스트를 표시합니다.
group: 핵심 개념
order: 53
---

## 텍스트

`core.Text`는 하나의 단락으로, 엔진 측에서 플랫폼 자체 텍스트 엔진으로 조판되고 메인 스레드에서 그려집니다. 크기는 글꼴과 너비에서 결정되며 박스 안에서 줄바꿈됩니다.

```go
core.Text{Content: "Hello from NSGo", Style: core.Style{
	FontSize:   15,
	LineHeight: 22,
	FontWeight: core.FontWeightSemibold,
}}
```

`Content`는 고정된 값이고 `ContentFunc`는 시그널을 추적하므로, 변경되는 텍스트는 해당 함수 안에서 시그널을 읽습니다. `MaxLines`는 표시되는 줄 수를 제한하고, `EllipsizeMode`는 앞·중간·끝에서 나머지 부분을 줄임표로 자르며, `WhiteSpaceNoWrap`은 단락을 한 줄로 유지합니다. `AdjustsFontSizeToFit`은 `MinimumFontScale`(최소 0.1)까지 텍스트를 축소해 박스와 줄 수 제한 안에 맞춥니다. 수식과 기타 글꼴 상대적 첨부 요소도 함께 축소되었다가 공간이 허용되면 원래 크기로 돌아가며, 명시적인 포인트 크기와 패딩은 지정된 값을 유지합니다.

텍스트 필드는 CSS처럼 상속됩니다. `FontSize`, `Color` 또는 `LineHeight`가 설정된 뷰 안의 텍스트는 자체 값을 설정하지 않는 한 해당 값을 물려받습니다. `TextAlign`, `TextTransform`, `LetterSpacing`, `TextDecoration`, `TextDecorationStyle`, `TextDecorationColor`와 `TextShadowColor`, `TextShadowOffset`, `TextShadowRadius`는 줄과 글리프에 스타일을 지정합니다.

## 런(run)과 링크

`Spans`는 각기 고유한 스타일을 가진 런(run)들로 텍스트를 구성합니다. `OnPress`가 있는 스팬은 링크가 되며, 다른 스팬을 누르면 텍스트 자체의 `OnPress`가 동작합니다.

```go
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "By continuing you agree to the "},
	{Content: "terms", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openTerms},
	{Content: " and the "},
	{Content: "privacy policy", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openPrivacy},
	{Content: "."},
}}
```

`SpansFunc`는 읽고 있는 값이 바뀔 때 스팬을 다시 반환하며 달라진 부분만 다시 준비하므로, 구문 강조가 도착함에 따라 코드 에디터의 줄 색상이 갱신됩니다. 스팬의 `Background`와 `CornerRadius`는 모든 줄의 문자 뒤에 상자를 그리며, `Cursor`는 텍스트의 설정보다 우선해 그 위의 포인터 모양을 지정합니다. `Visibility: core.VisibilityHidden`은 스팬을 그 자리에서 숨깁니다. 단어가 차지하는 공간은 유지되므로 다시 표시될 때 줄 위치가 움직이지 않고 복사 및 스크린 리더에서는 제외되어, 단어가 하나씩 서서히 나타나는 효과에 쓸 수 있습니다.

`Background`가 있는 스팬에 `BackgroundInsetTop: core.Pct(62)`를 설정하면 각 줄에서 상자 아래쪽 38%를 강조 표시할 수 있습니다. `BackgroundInsetBottom`은 아래쪽 가장자리를 줄입니다. 두 인셋 모두 글꼴의 어센트(ascent)에서 디센트(descent)까지인 상자 높이에 대해 포인트 단위나 백분율을 받으며, 텍스트 레이아웃은 변경하지 않습니다. TSX에서는 중첩된 `<Text>`의 스타일에 `backgroundInsetTop: "62%"`와 `backgroundInsetBottom`을 설정하세요. 둥근 모서리 배경과 세로 인셋은 HarmonyOS 및 네 가지 미니 프로그램 타겟에서도 렌더링됩니다.

`Attachment`는 스팬의 콘텐츠 대신 텍스트 안에 뷰를 넣습니다. 자체 크기로 레이아웃되고 주변 단어들과 함께 줄바꿈되며 자체 누르기 이벤트를 받으므로 아이콘, 이미지 또는 작은 버튼을 줄 안에 배치할 수 있습니다. 선택 시 해당 위치에는 스팬의 `Content`가 복사되며, 비어 있으면 생략됩니다. 기본적으로 첫 번째 텍스트 줄은 실제 글꼴의 메트릭을 사용해 주변 기준선(baseline)에 맞춰 정렬됩니다. `Em` 캔버스는 자체 기준선을 제공하며, 기준선이 없는 뷰는 아래쪽 가장자리에 맞춰 놓입니다. 다른 정렬 방식을 선택하려면 `Style.VerticalAlign`을 설정하세요.

```go
core.Text{Style: core.Style{FontSize: 17, LineHeight: 24}, Spans: []core.Span{
	{Content: "Search with "},
	{Attachment: core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 17}}},
	{Content: " NSGo"},
}}
```

## 글꼴

`FontFamily`는 글꼴 패밀리나 CSS 목록(`Inter, "Noto Sans SC", sans-serif`)을 지정하며, 앞선 글꼴에 없는 문자를 뒷 글꼴이 순서대로 채웁니다. 목록의 각 패밀리는 지정된 굵기와 스타일을 따릅니다. macOS와 iOS에서는 줄의 높이와 기준선을 문자가 실제로 그려진 서체에서 가져오므로, 줄 전체가 뒤쪽 패밀리로 그려지면 브라우저는 첫 번째 패밀리를 최솟값으로 유지하는 반면 해당 패밀리의 높이와 기준선을 따릅니다. `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LineHeightMultiple`, `LetterSpacing`, `LetterSpacingEm`은 서체와 간격을 설정하며, `core.FontMonospace`는 플랫폼의 고정폭 글꼴 패밀리입니다.

`Language`는 텍스트의 BCP 47 태그로, 위 필드들과 마찬가지로 텍스트, 스팬, 입력 필드에서 상속됩니다. 한자는 이 태그의 자형으로 그려지므로 `ja`로 설정된 `直`은 시스템이 중국어를 우선하더라도 일본어 자형을 따르며, 한국어(`ko`)는 CSS의 `word-break: keep-all`처럼 어절 단위로 줄바꿈됩니다. 아무것도 설정하지 않은 텍스트와 입력 필드는 앱이 표시하는 언어를 따르며 언어가 바뀔 때 함께 변경됩니다. 하위 트리에 적용하려면 뷰에, 단일 런에 적용하려면 `Span`에 설정하세요.

```go
core.Text{Spans: []core.Span{
	{Content: "日本語: "},
	{Content: "直 骨 角", Style: core.Style{Language: "ja"}},
}}
```

앱의 글꼴을 `resources/` 아래에 패키징해 첫 번째 뷰 전에 등록하거나, 앱이 실행되는 동안 글꼴 바이트를 등록하세요.

```go
func main() { nsgo.Run(Root, nsgo.Fonts("fonts/Inter-Regular.ttf", "fonts/Inter-Bold.ttf")) }

cx.RegisterFont(data) // TrueType 또는 OpenType 서체
```

글꼴이 로드되기 전에 레이아웃된 텍스트는 다시 준비되며, 캔버스도 `Draw`를 다시 실행해 글리프와 텍스트 측정이 로드된 글꼴을 사용하도록 합니다. `cx.Env().FontScale()`은 사용자의 텍스트 크기 배율이며, `NoFontScaling`이 설정되지 않는 한 텍스트가 이를 따릅니다. `MaxFontSizeMultiplier`는 최대 배율을 제한합니다.

## 선택과 측정

`SelectableOn`은 사용자가 텍스트를 선택하고 복사할 수 있게 하며, `Selectable`이 설정된 `View`는 웹 페이지처럼 내부 텍스트들이 문서 순서대로 함께 선택되도록 합니다. `SelectionColor`는 선택 강조 색상을 지정합니다. `OnTextSelection`은 사용자가 선택 영역을 움직일 때 `core.TextSelection`을 받습니다. 여기에는 일반 `Text`, `Markdown` 및 `HTML`(영역 내 `TextView`와 스타일이 지정된 텍스트의 파싱된 인라인·블록 구조를 부분 스팬과 링크까지 포함해 보존), 텍스트별 `Ranges`, 그리고 뷰 좌표계에서 선택이 닿는 텍스트들의 `Bounds`가 담겨 있어 Markdown이나 HTML로 복사하거나 단어 목록 또는 선택된 단어 번역을 만드는 데 활용할 수 있습니다. `desktop.LookUp`은 시스템 사전에서 해당 단어를 보여 줍니다([서비스](services.md#링크와-파일-열기)). `cx.MeasureText`는 특정 글꼴로 문자열의 크기를 측정해 텍스트에 맞춘 열 등을 구성할 때 사용합니다. 이를 호출하는 반응형 함수는 사용 가능한 글꼴이 바뀔 때 업데이트되며, `OnTextLayout`은 각 레이아웃 이후의 줄 정보를 보고합니다.

## 문서

`base/textview`는 동일한 기본 요소로 Markdown이나 HTML을 표시합니다. 제목, 단락, 목록, 표, 코드, 각주, 알림(alert), 이미지, 수식, 링크를 지원합니다.

```go
textview.TextView{
	Markdown: "## NSGo\n\nNative views from **Go**.",
	Styles:   ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) {
		if strings.HasPrefix(e.URL, "#") {
			return
		}
		cx.OpenURL(e.URL, nil)
	},
}
```

수식(`$…$`, `$$…$$`)은 앱이 `base/formula`의 `RenderMath: formula.RenderMath`를 전달하기 전까지는 TeX 원문으로 표시됩니다. 이를 전달하면 주변 텍스트의 크기와 색상으로 조판되고 앱의 언어로 소리 내어 읽히며 TeX로 복사됩니다. 이 기능은 글꼴을 포함해 데스크톱 앱에 약 6.5 MB, 웹 앱에 gzip 기준 약 3 MB를 추가합니다. `formula.View`는 그 외의 모든 곳에서 TeX 또는 Typst 형식의 수식을 표시하며, `OnHit` 콜백은 수식 에디터를 위해 Untie 소스 파트, 가장 가까운 캐럿, 겹치는 진단을 보고합니다. watchOS에서도 수식은 동일한 캔버스 경로를 사용하며, 이를 링크하면 1.4 MB 글꼴을 포함해 실행 파일 크기가 약 6.1 MB 늘어납니다. 워치 시뮬레이터 실행에서는 소유된 바이트 복사본을 포함해 해당 글꼴을 로드하고 파싱하는 데 약 3.9 MB의 상주 Go 힙 메모리가 측정되었습니다. 시뮬레이터 측정치는 실제 워치의 메모리 한계나 여유 공간을 확정하지 않으므로, 앱을 실행할 실제 워치에서 직접 측정하세요. 설정된 글꼴이 지원하지 못하는 텍스트 런(`\text{}` 또는 Typst 문자열)은 CJK 및 기타 문자를 포함한 시스템 글꼴을 사용하며, 명시적인 `formula.Fonts.Fallbacks`가 우선순위를 가집니다.

소스가 조각 단위로 도착하는 경우 `formula.View`에 `SourceFunc`를 전달하고 `Streaming: true`를 설정하거나 `StreamingFunc`를 앱의 수신 상태에 바인딩하세요. 이렇게 하면 유효한 콘텐츠가 계속 레이아웃되는 동안 복구 가능한 오류와 플레이스홀더 표시가 숨겨집니다. 수신이 끝나면 플래그를 해제해 남은 오류를 표시하세요. `TextView`에서는 전달된 `Math.TeX`와 `Math.Display`를 사용해 해당 `formula.View`를 반환하는 `RenderMath` 함수를 제공하면 되며, 그 안의 `StreamingFunc`도 같은 수신 상태를 읽을 수 있습니다.

디스플레이 수식은 텍스트의 글꼴 크기를 유지하면서 패딩과 테두리 안쪽의 블록 너비에 맞춰 안전한 관계 기호와 연산자 위치에서 줄바꿈됩니다. 크기를 조절하거나 텍스트 크기를 변경하면 줄이 다시 배치됩니다. 나눌 수 없는 부분은 계속 가로로 스크롤되며, 인라인 수식은 본래 크기를 유지합니다.

`mermaid` 코드 블록은 앱이 `base/diagram`의 `RenderDiagram: diagram.RenderDiagram`을 전달하기 전까지는 소스를 그대로 표시합니다. 이를 전달하면 limu를 사용해 플로우차트, 시퀀스, 클래스, 상태, 개체-관계(ER) 다이어그램, 간트 차트, 파이 차트, 마인드맵, 타임라인 등을 앱의 화면 모드(라이트 또는 다크)와 플랫폼 글꼴에 맞춰 네이티브 다이어그램으로 그립니다. 링크가 있는 다이어그램 항목은 누르면 앱의 URL 서비스를 통해 열리며, 포인터를 올려두면 항목의 Mermaid 툴팁이 표시됩니다. 텍스트의 가용 너비에 맞춰 레이아웃되므로 넓은 플로우차트는 레이블 크기를 유지한 채 세로 방향으로 전환되며, 여전히 들어맞지 않는 다이어그램은 가로로 스크롤됩니다. 복사 시에는 원본 소스가 복사되고, 스트리밍 중인 블록은 완성될 때까지 코드로 표시됩니다. `diagram.View`는 다른 어느 곳에서나 다이어그램을 표시하며 컨테이너 너비가 바뀔 때 그에 맞춰 따라갑니다. `Icons` 필드는 `"fa:fa-car"` 같은 Mermaid 이름을 `icons/lucide`를 포함한 `*core.SVG` 문서에 매핑하며, 없는 이름은 인라인 여백을 남기지 않습니다. 이미지 노드는 공유 캔버스 이미지 로더를 통해 HTTP(S), 파일 또는 `asset:` 소스를 불러오며 `constraint: "on"`이 설정된 경우 비율에 맞춰 가운데 정렬하고 그렇지 않으면 늘려 채웁니다. Mermaid는 로드 전에 이미지 주소를 필터링합니다(`data:` URL 거부 포함). 텍스트 뷰에서 아이콘을 제공하려면 `RenderDiagram`에 `diagram.View{Source: code.Code, Icons: icons}`를 반환하는 콜백을 설정하세요.

다이어그램에 `ui` 팔레트를 적용하려면 `RenderDiagram` 콜백에서 `ThemeFunc`를 설정한 `diagram.View`를 반환하세요. 이 함수는 캔버스가 그릴 때 색상을 읽으므로, 팔레트나 하위 트리 외관 모드가 바뀌면 기존 뷰의 색상이 다시 입혀집니다. 제로 값으로 남겨 둔 색상은 다이어그램의 기본값을 유지합니다.

```go
func drawDiagram(cx *core.Scope, code textview.CodeBlock) core.Node {
	colors := ui.ColorsOf(cx)
	return diagram.View{Source: code.Code, ThemeFunc: func() *diagram.Theme {
		return &diagram.Theme{
			Text: colors.Text(), Surface: colors.Surface(), Border: colors.Border(),
			Line: colors.Muted(), Accent: colors.Accent(), Critical: colors.Danger(),
		}
	}}
}
// ui.TextView{Markdown: source, RenderDiagram: drawDiagram}
```

`textview.Parse`는 Markdown을, `textview.ParseHTML`은 HTML을 앱에서도 직접 구성할 수 있는 블록들로 파싱합니다. `Options.Streaming`은 조각 단위로 도착하는 문서를 복구해 끝부분에서 열린 채 남은 구문을 닫고, 단독 목록 기호처럼 다음 문자가 와야 의미가 결정되는 마지막 줄을 보류하므로 응답이 스트리밍되는 동안 기호가 일반 텍스트로 깜빡이지 않으며, `StreamingCaret`은 스트리밍으로 작성되는 응답의 끝에 캐럿을 그려 줍니다.

`textview.Search`는 `Search` 필드나 상위 트리의 `textview.ProvideSearch`를 통해 연결된 뷰들 전체에서 텍스트를 찾습니다. `SetQuery`, `Open`, `Close`로 찾기 막대에서 검색을 제어하고, `Count`와 `Selected`로 현재 위치를 확인하며, `Next`와 `Previous`로 읽기 순서대로 일치 항목을 이동합니다. 각 뷰는 일치 항목을 `PartSearch`로, 선택된 항목을 `PartSearchSelected`로 표시해 앱의 스타일로 색상을 입히고 화면 안으로 스크롤하며 사용자의 텍스트 선택 영역은 그대로 유지합니다.

```go
search := textview.NewSearch(cx)
textview.ProvideSearch(cx, search)
search.SetQuery("readme")
search.Open()
```

## 다음

- [이미지와 아이콘](images.md): 사진과 SVG를 텍스트 안에서도 사용할 수 있습니다.
- [스타일과 테마](styles.md): 상속되는 색상과 텍스트 필드.
- [레이아웃](layout.md): 텍스트가 너비를 차지하는 방식.

## 리치 텍스트 편집

`ui/richeditor.View`는 `base/richeditor.Session`을 테마의 외관으로 표시하며, `base/richeditor.View`는 앱이 제공한 스타일과 함께 동작을 제공합니다. iOS와 Android에서는 길게 누르거나 두 번 탭하면 단어가 선택되며, 선택 핸들을 드래그해 블록을 가로질러 범위를 확장할 수 있습니다. 네이티브 편집 메뉴는 선택 영역과 클립보드 상태에 따라 복사 및 전체 선택을, 세션이 편집 가능할 때는 잘라내기 및 붙여넣기를 추가로 제공합니다. `Session.SetEditable`은 열려 있는 메뉴도 함께 갱신합니다. 베이스 뷰에서는 `Styles.SelectionHandle` 또는 `Classes.SelectionHandle`로 핸들 노브의 스타일을 지정할 수 있습니다.

커스텀 텍스트 컨트롤에서는 `core.Input`에 포커스를 준 뒤 서피스 포인트 단위의 범위와 `core.EditCut`, `EditCopy`, `EditPaste`, `EditSelectAll`의 비트마스크를 전달해 `Ref.OpenEditMenu(bounds, actions)`를 호출하세요. iOS와 Android에서는 `Input.OnEditAction`이 잘라내기, 복사, 전체 선택을 받으며 붙여넣기는 `Input.OnPaste`를 사용합니다. 메뉴를 닫으려면 `Ref.CloseEditMenu`를 호출하세요.

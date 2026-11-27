---
title: 文本
description: 用 span、字体、装饰、选择和文档展示文本。
group: 核心概念
order: 53
---

## 一段文本

`core.Text` 是一个段落，由平台自己的文本引擎在引擎侧排版，再由主线程绘制。它的尺寸来自字体和宽度，并在盒子内换行。

```go
core.Text{Content: "Hello from NSGo", Style: core.Style{
	FontSize:   15,
	LineHeight: 22,
	FontWeight: core.FontWeightSemibold,
}}
```

`Content` 是固定内容，`ContentFunc` 跟随 signal，因此会变化的文本在函数里读取它。`MaxLines` 限制显示的行数，`EllipsizeMode` 在开头、中间或结尾截断，`WhiteSpaceNoWrap` 让段落保持一行。`AdjustsFontSizeToFit` 会缩小文本，直到它放进盒子和行数限制，最小缩放为 `MinimumFontScale`（至少 0.1）。公式等随字号变化的附件会一起缩小，空间足够时恢复原来的大小；明确指定的点数尺寸和内边距保持原值。

文本字段像 CSS 一样继承：位于设置了 `FontSize`、`Color` 或 `LineHeight` 的视图中的文本会采用这些值，除非自己设置。`TextAlign`、`TextTransform`、`LetterSpacing`、`TextDecoration`、`TextDecorationStyle`、`TextDecorationColor`，以及 `TextShadowColor`、`TextShadowOffset` 和 `TextShadowRadius` 设置行与字形的样式。

## Span 与链接

`Spans` 把文本拆成若干段，每段有自己的样式。带 `OnPress` 的 span 是链接；按在其他 span 上会触发文本自己的 `OnPress`。

```go
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "继续即表示你同意"},
	{Content: "服务条款", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openTerms},
	{Content: "和"},
	{Content: "隐私政策", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openPrivacy},
	{Content: "。"},
}}
```

`SpansFunc` 在读取的值变化时重新给出 span，只有不同的部分会重新排版，因此代码编辑器的行会在高亮到达时重新着色。span 的 `Background` 和 `CornerRadius` 会在每一行字符后面画一个盒子，它的 `Cursor` 决定指针在这些字符上的样子，优先于整段文字的。`Visibility: core.VisibilityHidden` 原地隐藏一个 span：它的文字仍占着位置，显示时各行不会移动，复制和屏幕阅读器都会略过它，适合逐词淡入的文字。

给带有 `Background` 的 span 设置 `BackgroundInsetTop: core.Pct(62)`，会高亮每行背景盒子下方的 38%。`BackgroundInsetBottom` 从底部内缩。两个值都可以用点数或盒子高度的百分比表示，盒子的高度取自字体的上升部到下降部。内缩不会影响排版。在 TSX 中，可以在嵌套的 `<Text>` 样式中设置 `backgroundInsetTop: "62%"` 和 `backgroundInsetBottom`。 鸿蒙和四种小程序目标也会绘制圆角背景及垂直内缩。

`Attachment` 把一个视图放进文本，替代 span 的内容。它按自身尺寸布局，随周围的文字换行，并接收自己的按压，因此图标、图片或小按钮可以出现在一行里。复制选区时用 span 的 `Content` 替代这个视图，为空时略过它。默认以视图内第一行文字的基线对齐周围文字，基线采用该行实际使用的字体度量。`Em` 画布提供自己的基线，其余没有基线的视图以底边对齐。可以用 `Style.VerticalAlign` 选择其他对齐方式。

```go
core.Text{Style: core.Style{FontSize: 17, LineHeight: 24}, Spans: []core.Span{
	{Content: "用 "},
	{Attachment: core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 17}}},
	{Content: " 搜索 NSGo"},
}}
```

## 字体

`FontFamily` 指定一个字体系列或 CSS 列表（`Inter, "Noto Sans SC", sans-serif`），前面的字体缺少的字符由后面的字体补上。列表中的每个字体都采用指定的字重和样式。在 macOS 和 iOS 上，一行的行高和基线取自该行字符实际使用的字体，因此整行都由后面的字体绘制时采用该字体的度量，而浏览器会以第一个字体的度量作为下限。`FontWeight`、`FontStyle`、`FontVariant`、`LineHeight`、`LineHeightMultiple`、`LetterSpacing` 和 `LetterSpacingEm` 设置字形和间距，`core.FontMonospace` 是平台等宽字体。

`Language` 是文本的 BCP 47 语言标签，和上面的字段一样会在文本、`Span` 与输入框上继承。汉字按它的字形绘制，所以设为 `ja` 的 `直` 即使在偏好中文的系统上也显示日文字形；韩语（`ko`）按词边界换行，与 CSS 的 `word-break: keep-all` 一致。没有设置的文本和输入框采用应用显示的语言，并在语言切换时跟着变化。在视图上设置作用于整个子树，在 `Span` 上设置只作用于这一段：

```go
core.Text{Spans: []core.Span{
	{Content: "日本語: "},
	{Content: "直 骨 角", Style: core.Style{Language: "ja"}},
}}
```

把应用的字体放在 `resources/` 下并在第一个视图前注册，或在运行时注册字体字节：

```go
func main() { nsgo.Run(Root, nsgo.Fonts("fonts/Inter-Regular.ttf", "fonts/Inter-Bold.ttf")) }

cx.RegisterFont(data) // 一个 TrueType 或 OpenType 字体
```

字体到达前排版的文本会重新排版，画布也会再次运行 `Draw`，让字形和文本测量使用新加载的字体。`cx.Env().FontScale()` 是用户的文本大小，文本默认跟随它，`NoFontScaling` 可以关闭，`MaxFontSizeMultiplier` 限制最大倍数。

## 选择与测量

`SelectableOn` 让用户选择和复制文本，带 `Selectable` 的 `View` 让其中的文本按文档顺序一起选择，像网页一样。`SelectionColor` 设置高亮颜色。`OnTextSelection` 在用户移动选区时收到 `core.TextSelection`：选中的纯文本 `Text`、`Markdown` 和 `HTML`（保留区域内 `TextView` 解析出的块与行内结构以及带样式文本的格式，包括部分跨度与链接）、各文本的 `Ranges`，以及选区涉及的文本在该视图中的边界框 `Bounds`，可用于按 Markdown/HTML 复制、生词表或翻译选中的词；`desktop.LookUp` 在系统词典中查它们（[服务](services.md#打开链接与文件)）。`cx.MeasureText` 测量一个字符串在某个字体下的尺寸，适合按文本宽度排列的列；响应式函数调用它后会随可用字体变化而更新；`OnTextLayout` 在每次布局后报告行。

## 文档

`base/textview` 用相同的原语显示 Markdown 或 HTML：标题、段落、列表、表格、代码、脚注、提示块、图片、公式和链接。

```go
textview.TextView{
	Markdown: "## NSGo\n\n用 **Go** 写的原生视图。",
	Styles:   ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) {
		if strings.HasPrefix(e.URL, "#") {
			return
		}
		cx.OpenURL(e.URL, nil)
	},
}
```

公式（`$…$`、`$$…$$`）默认显示 TeX 源码；应用传入 `base/formula` 的 `RenderMath: formula.RenderMath` 后才会排版，字号和颜色与周围文本一致，读屏时使用应用的语言，复制时除纯文本 TeX 外，还提供 Presentation MathML（`application/mathml+xml`）和源码格式（`text/x-tex`，Typst 为 `text/x-typst`）；`formula.MathML`、`formula.ClipboardItem` 和 `formula.Copy` 可直接导出或复制相同格式，在不支持多格式剪贴板的平台上自动回退为纯文本。链接 `base/formula` 会让桌面应用增大约 6.5 MB，网页应用增大约 3 MB（gzip 压缩后），其中包括字体。在其他地方显示公式用 `formula.View`，支持 TeX 和 Typst；它的 `OnHit` 回调会把 Untie 的源码部分、最近插入符和重叠诊断传给公式编辑器。watchOS 也使用同一画布路径；链接公式会让可执行文件增大约 6.1 MB，其中包括 1.4 MB 字体。在手表模拟器中测得，加载并解析字体会增加约 3.9 MB 的常驻 Go 堆，包括字体字节的独立副本。模拟器的测量不能确定真实手表的应用内存上限或剩余空间，这些需要在运行应用的手表上测量。配置的字体无法覆盖的文本（`\text{}` 或 Typst 字符串）使用系统字体，包括中日韩文字及其他文字系统；显式设置的 `formula.Fonts.Fallbacks` 优先。

源码分段到达时，为 `formula.View` 设置 `SourceFunc`，并设 `Streaming: true`，或让 `StreamingFunc` 读取应用的接收状态。这会在有效内容继续排版时隐藏可恢复错误和占位符的图形；接收完成后清除标记，显示仍然存在的错误。在 `TextView` 中，通过 `RenderMath` 返回这样的 `formula.View`，传入 `Math.TeX` 和 `Math.Display`，并让 `StreamingFunc` 读取同一接收状态。

块级公式按扣除内边距和边框后的宽度，在安全的关系符和运算符处换行，保持文本字号；调整宽度或字号会重新分行。无法安全拆分的部分仍可横向滚动，行内公式保持自然尺寸。

`mermaid` 代码块默认显示源码；应用传入 `base/diagram` 的 `RenderDiagram: diagram.RenderDiagram` 后，会用 limu 把它画成原生图表，支持流程图、时序图、类图、状态图、实体关系图、甘特图、饼图、思维导图、时间线等，使用平台字体，并随应用的浅色或深色外观切换。带链接的图表项目按下时通过应用的 URL 服务打开，指针停在项目上时显示 Mermaid 提示。图表按文本的可用宽度布局，过宽的流程图会改为向下排列，标签字号保持不变；无法适配的图表仍可横向滚动。复制时得到源码；仍在流式输出中的代码块会先显示为代码，写完后再画成图。在其他地方显示图表用 `diagram.View`，它会随容器宽度的变化重新布局。它的 `Icons` 把 `"fa:fa-car"` 等 Mermaid 名称映射到 `*core.SVG`，也可使用 `icons/lucide` 中的图标；缺少的名称不会留下行内空隙。图片节点通过共用的画布图片加载器加载 HTTP(S)、文件或 `asset:` 来源；设置 `constraint: "on"` 时保持比例并居中，否则拉伸填满。Mermaid 会先筛选图片地址（包括拒绝 `data:` URL）。要给文本中的图表提供图标，把 `RenderDiagram` 设为返回 `diagram.View{Source: code.Code, Icons: icons}` 的回调。

在 `RenderDiagram` 回调中返回带有 `ThemeFunc` 的 `diagram.View`，可以让图表采用应用的 `ui` 调色板。这个函数在画布绘制时读取颜色，因此调色板或子树外观变化时会给现有视图重新着色。保持为零的颜色继续使用图表的默认值：

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

`textview.Parse` 读取 Markdown，`textview.ParseHTML` 读取 HTML，得到应用也可以自己构建的块。`Options.Streaming` 修复仍在到达的文档：补全末尾未闭合的语法，并暂缓显示含义还要由后续字符决定的最后一行，例如单独的列表标记，让回复不会把标记闪成文本；`StreamingCaret` 在末尾绘制光标，适合逐字生成的回复。`FadeIn` 让文档新增的字符依次淡入，且不会落后到达太久，让成批到达的回复也读起来流畅。

`textview.Search` 在加入它的各个视图里查找文本，视图通过 `Search` 字段或上层的 `textview.ProvideSearch` 加入。查找栏用 `SetQuery`、`Open` 和 `Close` 驱动它，`Count` 和 `Selected` 给出位置，`Next` 和 `Previous` 按阅读顺序逐个切换匹配。视图给每个匹配加上 `PartSearch`，给选中的匹配再加 `PartSearchSelected`，颜色由应用的样式决定；视图还会把选中的匹配滚动到视野内，并且不改动用户的选择。

```go
search := textview.NewSearch(cx)
textview.ProvideSearch(cx, search)
search.SetQuery("readme")
search.Open()
```

## 下一步

- [图片与图标](images.md)：图片和 SVG，也可以放进文本。
- [样式与主题](styles.md)：可继承的颜色和文本字段。
- [布局](layout.md)：文本如何取得宽度。

## 富文本编辑

`ui/richeditor.View` 用主题外观显示 `base/richeditor.Session`；`base/richeditor.View` 提供行为，并采用应用传入的样式。在 iOS 和 Android 上，长按或双击选择一个词，拖动选区手柄可跨文本块扩展范围。原生编辑菜单根据选区和剪贴板提供适用的复制和全选操作，会话可编辑时还提供剪切和粘贴。`Session.SetEditable` 也会刷新已经打开的菜单。在 base 视图上，通过 `Styles.SelectionHandle` 或 `Classes.SelectionHandle` 设置手柄圆点的样式。

自定义文本控件可先聚焦 `core.Input`，再调用 `Ref.OpenEditMenu(bounds, actions)`；边界以页面坐标中的点为单位，操作使用 `core.EditCut`、`EditCopy`、`EditPaste` 和 `EditSelectAll` 的位掩码。在 iOS 和 Android 上，剪切、复制和全选由 `Input.OnEditAction` 接收，粘贴通过 `Input.OnPaste`。调用 `Ref.CloseEditMenu` 关闭菜单。

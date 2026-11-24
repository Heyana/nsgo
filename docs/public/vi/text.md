---
title: Văn bản
description: Hiển thị văn bản với span, phông chữ, trang trí, vùng chọn và tài liệu.
group: Khái niệm cốt lõi
order: 53
---

## Một đoạn văn bản

`core.Text` là một đoạn văn, được dàn trang bằng chính bộ máy văn bản của nền tảng ở phía engine và được vẽ bởi luồng chính. Kích thước của nó đến từ phông chữ và chiều rộng, và nó tự xuống dòng bên trong khung của mình.

```go
core.Text{Content: "Hello from NSGo", Style: core.Style{
	FontSize:   15,
	LineHeight: 22,
	FontWeight: core.FontWeightSemibold,
}}
```

`Content` là cố định còn `ContentFunc` bám theo một signal, nên một văn bản thay đổi sẽ đọc signal đó bên trong hàm. `MaxLines` giới hạn số dòng hiển thị, `EllipsizeMode` cắt phần còn lại ở đầu, giữa hoặc cuối, và `WhiteSpaceNoWrap` giữ một đoạn văn trên một dòng. `AdjustsFontSizeToFit` thu nhỏ văn bản cho đến khi vừa khung và giới hạn số dòng, xuống tới `MinimumFontScale` (tối thiểu 0.1). Công thức và các phần đính kèm khác tính theo phông chữ sẽ thu nhỏ theo và trở về kích thước tự nhiên khi đủ chỗ; kích thước điểm và khoảng đệm được chỉ định rõ giữ nguyên giá trị đã viết.

Các trường văn bản được kế thừa như trong CSS: một văn bản nằm trong view có `FontSize`, `Color` hoặc `LineHeight` sẽ nhận các giá trị đó trừ khi tự đặt giá trị riêng. `TextAlign`, `TextTransform`, `LetterSpacing`, `TextDecoration`, `TextDecorationStyle`, `TextDecorationColor` cùng `TextShadowColor`, `TextShadowOffset` và `TextShadowRadius` định kiểu cho các dòng và glyph.

## Run và liên kết

`Spans` tạo một văn bản gồm nhiều run, mỗi run có kiểu dáng riêng. Một span có `OnPress` là một liên kết, còn một lần nhấn vào span khác sẽ là `OnPress` của chính văn bản.

```go
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "By continuing you agree to the "},
	{Content: "terms", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openTerms},
	{Content: " and the "},
	{Content: "privacy policy", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openPrivacy},
	{Content: "."},
}}
```

`SpansFunc` cung cấp lại các span khi những gì nó đọc thay đổi, và chỉ những phần khác biệt mới được chuẩn bị lại, nhờ vậy các dòng của trình soạn thảo mã được tô màu lại khi kết quả tô sáng cú pháp đến. `Background` và `CornerRadius` của một span vẽ một khung phía sau các ký tự của nó trên mọi dòng, và `Cursor` của nó chỉ định hình dạng con trỏ khi di qua chúng, ưu tiên hơn của văn bản. `Visibility: core.VisibilityHidden` ẩn một span tại chỗ: các từ của nó vẫn giữ chỗ, nên các dòng không xê dịch khi nó hiện ra, và thao tác sao chép cũng như trình đọc màn hình sẽ bỏ qua nó, dùng cho các từ hiện dần từng từ một.

Đặt `BackgroundInsetTop: core.Pct(62)` trên một span có `Background` để tô sáng 38% phía dưới khung của nó trên mỗi dòng. `BackgroundInsetBottom` cắt bớt cạnh dưới. Cả hai khoảng thụt nhận giá trị điểm hoặc phần trăm chiều cao của khung, tính từ ascent đến descent của phông chữ. Chúng không làm thay đổi bố cục văn bản. Trong TSX, đặt `backgroundInsetTop: "62%"` và `backgroundInsetBottom` trong style của một `<Text>` lồng bên trong. Nền bo góc và khoảng thụt theo chiều dọc cũng được hiển thị trên HarmonyOS và bốn mục tiêu mini-program.

`Attachment` đặt một view vào bên trong văn bản, thay cho nội dung của span. Nó được dàn trang theo kích thước riêng, xuống dòng cùng các từ xung quanh và tự nhận các lần nhấn của mình, nên một biểu tượng, một hình ảnh hay một nút nhỏ có thể nằm trên một dòng. Một vùng chọn sẽ sao chép `Content` của span vào vị trí của nó, và bỏ qua khi `Content` rỗng. Theo mặc định, dòng văn bản đầu tiên của nó căn theo đường cơ sở xung quanh, dùng số đo của phông chữ thực tế. Một canvas `Em` cung cấp đường cơ sở riêng, còn một view không có đường cơ sở sẽ đặt trên cạnh dưới của nó. Đặt `Style.VerticalAlign` để chọn cách căn khác.

```go
core.Text{Style: core.Style{FontSize: 17, LineHeight: 24}, Spans: []core.Span{
	{Content: "Search with "},
	{Attachment: core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 17}}},
	{Content: " NSGo"},
}}
```

## Phông chữ

`FontFamily` chỉ định một họ phông hoặc một danh sách theo kiểu CSS (`Inter, "Noto Sans SC", sans-serif`), mỗi họ được dùng cho những ký tự mà các họ đứng trước nó không có. Mỗi họ trong danh sách đều nhận độ đậm và kiểu chữ. Trên macOS và iOS, một dòng lấy chiều cao và đường cơ sở từ các mặt chữ mà các ký tự của nó được đặt vào, nên một dòng hoàn toàn dùng một họ phía sau sẽ lấy theo họ đó, trong khi trình duyệt giữ giá trị của họ đầu tiên làm mức tối thiểu. `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LineHeightMultiple`, `LetterSpacing` và `LetterSpacingEm` đặt mặt chữ và khoảng cách, và `core.FontMonospace` là họ phông độ rộng cố định của nền tảng.

`Language` là thẻ BCP 47 của văn bản, được kế thừa như các trường ở trên trên văn bản, span và ô nhập. Chữ Hán được vẽ theo tự dạng của ngôn ngữ đó, nên `直` đặt trong `ja` sẽ mang tự dạng tiếng Nhật ngay cả khi hệ thống ưu tiên tiếng Trung, và tiếng Hàn (`ko`) xuống dòng giữa các từ như `word-break: keep-all` của CSS. Văn bản và ô nhập không đặt giá trị nào sẽ dùng ngôn ngữ mà ứng dụng đang hiển thị và thay đổi theo khi ngôn ngữ đó đổi. Đặt nó trên một view cho cả cây con, hoặc trên một `Span` cho một run:

```go
core.Text{Spans: []core.Span{
	{Content: "日本語: "},
	{Content: "直 骨 角", Style: core.Style{Language: "ja"}},
}}
```

Đóng gói phông chữ của ứng dụng trong `resources/` và đăng ký chúng trước view đầu tiên, hoặc đăng ký dữ liệu byte của một phông chữ trong khi ứng dụng đang chạy:

```go
func main() { nsgo.Run(Root, nsgo.Fonts("fonts/Inter-Regular.ttf", "fonts/Inter-Bold.ttf")) }

cx.RegisterFont(data) // a TrueType or OpenType face
```

Một văn bản được dàn trang trước khi phông chữ có sẵn sẽ được chuẩn bị lại, và các canvas chạy lại `Draw` để glyph và số đo văn bản của chúng dùng phông chữ đã tải. `cx.Env().FontScale()` là cỡ chữ người dùng chọn, mà các văn bản tuân theo trừ khi đặt `NoFontScaling`; `MaxFontSizeMultiplier` giới hạn mức này.

## Vùng chọn và đo đạc

`SelectableOn` cho phép người dùng chọn và sao chép một văn bản, và một `View` có `Selectable` làm cho các văn bản bên trong được chọn cùng nhau, theo thứ tự tài liệu, như trên một trang web. `SelectionColor` tô màu vùng tô sáng. `OnTextSelection` nhận một `core.TextSelection` khi người dùng di chuyển vùng chọn: `Text` thuần, `Markdown` và `HTML` của nó (giữ nguyên cấu trúc inline và block đã phân tích của mọi `TextView` và văn bản có kiểu dáng trong vùng, bao gồm cả span và liên kết được chọn một phần), các `Ranges` theo từng văn bản và `Bounds` của các văn bản mà nó chạm tới, trong view, để sao chép dưới dạng Markdown hoặc HTML, làm danh sách từ hoặc dịch các từ được chọn; `desktop.LookUp` hiển thị chúng trong từ điển của hệ thống ([dịch vụ](services.md#opening-links-and-files)). `cx.MeasureText` đo một chuỗi theo một phông chữ, dùng cho một cột vừa khít với văn bản của nó; các hàm phản ứng gọi nó sẽ cập nhật khi các phông chữ có sẵn thay đổi, và `OnTextLayout` báo cáo các dòng sau mỗi lần dàn trang.

## Tài liệu

`base/textview` hiển thị Markdown hoặc HTML bằng chính các phần tử cơ bản đó: tiêu đề, đoạn văn, danh sách, bảng, mã, chú thích cuối trang, cảnh báo, hình ảnh, công thức và liên kết.

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

Công thức (`$…$`, `$$…$$`) hiển thị mã TeX của chúng cho đến khi ứng dụng truyền `RenderMath: formula.RenderMath` từ `base/formula`, thứ sẽ dàn trang chúng theo kích thước và màu của văn bản xung quanh, đọc to bằng ngôn ngữ của ứng dụng và sao chép dưới dạng TeX; nó làm ứng dụng desktop tăng khoảng 6.5 MB và ứng dụng web tăng 3 MB sau khi nén gzip, đã bao gồm phông chữ. `formula.View` hiển thị một công thức ở bất kỳ nơi nào khác, bằng TeX hoặc Typst; callback `OnHit` của nó báo cáo phần nguồn Untie, vị trí con trỏ chèn gần nhất và các chẩn đoán chồng lấn, dành cho trình soạn thảo công thức. Trên watchOS, công thức dùng cùng đường vẽ canvas; liên kết chúng vào làm tệp thực thi tăng khoảng 6.1 MB, bao gồm phông chữ 1.4 MB. Một lần chạy trên trình mô phỏng đồng hồ đo được khoảng 3.9 MB heap Go được giữ lại cho việc tải và phân tích phông chữ đó, bao gồm bản sao byte riêng của nó. Số đo trên trình mô phỏng không xác lập được giới hạn bộ nhớ hay dung lượng còn trống của đồng hồ thật; hãy đo những điều đó trên chính chiếc đồng hồ chạy ứng dụng. Các run văn bản (`\text{}` hoặc chuỗi Typst) mà các phông chữ đã cấu hình không bao phủ được sẽ dùng phông chữ hệ thống, bao gồm CJK và các hệ chữ viết khác; `formula.Fonts.Fallbacks` được chỉ định rõ sẽ được ưu tiên.

Với nguồn đến theo từng phần, truyền `SourceFunc` cho `formula.View` và đặt `Streaming: true`, hoặc liên kết `StreamingFunc` với trạng thái đang nhận của ứng dụng. Việc này ẩn phần mực của lỗi có thể khắc phục và của placeholder trong khi nội dung hợp lệ tiếp tục được dàn trang. Xóa cờ này khi việc nhận kết thúc để hiển thị các lỗi còn lại. Trong một `TextView`, cung cấp một hàm `RenderMath` trả về `formula.View` đó với `Math.TeX` và `Math.Display` được cung cấp; `StreamingFunc` của nó có thể đọc cùng trạng thái đang nhận.

Công thức dạng hiển thị (display) xuống dòng tại các quan hệ và toán tử an toàn theo chiều rộng của khối bên trong khoảng đệm và viền, giữ nguyên cỡ chữ của văn bản. Thay đổi kích thước hoặc thay đổi cỡ chữ sẽ dàn lại các dòng. Những phần không thể ngắt vẫn cuộn ngang; công thức inline giữ kích thước tự nhiên của chúng.

Một khối mã `mermaid` hiển thị mã nguồn của nó cho đến khi ứng dụng truyền `RenderDiagram: diagram.RenderDiagram` từ `base/diagram`, thứ sẽ vẽ nó thành sơ đồ gốc, bằng limu: lưu đồ, sơ đồ tuần tự, sơ đồ lớp, sơ đồ trạng thái và sơ đồ quan hệ thực thể, biểu đồ Gantt, biểu đồ tròn, sơ đồ tư duy, dòng thời gian và nhiều loại khác, bằng phông chữ của nền tảng, sáng hoặc tối theo giao diện ứng dụng đang hiển thị. Các mục trong sơ đồ có liên kết sẽ mở liên kết đó qua dịch vụ URL của ứng dụng khi được nhấn, và tooltip Mermaid của một mục xuất hiện khi con trỏ dừng trên nó. Nó được dàn trang theo chiều rộng khả dụng của văn bản, chuyển một lưu đồ rộng sang chạy theo chiều dọc trong khi giữ nguyên cỡ nhãn; những sơ đồ không thể vừa vẫn cuộn ngang. Một bản sao sẽ lấy mã nguồn của nó, và một khối vẫn đang được truyền đến sẽ hiển thị dưới dạng mã cho đến khi hoàn tất. `diagram.View` hiển thị một sơ đồ ở bất kỳ nơi nào khác và tuân theo chiều rộng của vùng chứa khi nó thay đổi. `Icons` của nó ánh xạ các tên Mermaid như `"fa:fa-car"` sang tài liệu `*core.SVG`, bao gồm cả các tài liệu từ `icons/lucide`; những tên không có sẽ không để lại khoảng trống inline. Các nút hình ảnh tải nguồn HTTP(S), tệp hoặc `asset:` qua bộ tải hình ảnh canvas dùng chung, vừa khít và căn giữa khi đặt `constraint: "on"` và kéo giãn trong trường hợp khác. Mermaid lọc địa chỉ hình ảnh trước khi tải (bao gồm việc từ chối URL `data:`). Để cung cấp biểu tượng trong một text view, đặt `RenderDiagram` thành một callback trả về `diagram.View{Source: code.Code, Icons: icons}`.

Để áp bảng màu `ui` của bạn cho sơ đồ, trả về một `diagram.View` có `ThemeFunc` từ callback `RenderDiagram`. Hàm này đọc màu khi canvas vẽ, nên khi bảng màu hoặc giao diện của cây con thay đổi, view hiện có sẽ được tô màu lại. Những màu để giá trị không sẽ giữ màu mặc định của sơ đồ.

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

`textview.Parse` đọc Markdown và `textview.ParseHTML` đọc HTML thành các khối mà ứng dụng cũng có thể tự dựng. `Options.Streaming` sửa một tài liệu đến theo từng phần, đóng những gì phần cuối còn để mở và giữ lại dòng cuối cùng mà ý nghĩa của nó do các ký tự tiếp theo quyết định, chẳng hạn một ký hiệu danh sách đứng một mình, nhờ vậy một câu trả lời không bao giờ chớp hiện các ký hiệu dưới dạng văn bản, và `StreamingCaret` vẽ một con trỏ chèn ở cuối tài liệu, dành cho câu trả lời được viết ra trong lúc truyền đến.

Một `textview.Search` tìm văn bản trên các view tham gia vào nó, thông qua trường `Search` của chúng hoặc `textview.ProvideSearch` ở phía trên. `SetQuery`, `Open` và `Close` điều khiển nó từ một thanh tìm kiếm, `Count` và `Selected` cho biết vị trí hiện tại, và `Next` cùng `Previous` lần lượt đi qua các kết quả khớp theo thứ tự đọc. Các view đánh dấu mỗi kết quả khớp bằng `PartSearch` và kết quả đang chọn bằng `PartSearchSelected` để style của ứng dụng tô màu, cuộn kết quả đó vào tầm nhìn, và không động đến vùng chọn của người dùng.

```go
search := textview.NewSearch(cx)
textview.ProvideSearch(cx, search)
search.SetQuery("readme")
search.Open()
```

## Tiếp theo

- [Hình ảnh và biểu tượng](images.md): hình ảnh và SVG, kể cả trong văn bản.
- [Kiểu dáng và chủ đề](styles.md): màu sắc và các trường văn bản được kế thừa.
- [Bố cục](layout.md): cách một văn bản nhận chiều rộng của nó.

## Chỉnh sửa văn bản định dạng

`ui/richeditor.View` hiển thị một `base/richeditor.Session` với giao diện của chủ đề; `base/richeditor.View` cung cấp hành vi với các style do ứng dụng cung cấp. Trên iOS và Android, nhấn giữ sẽ chọn một từ và chạm hai lần cũng vậy; kéo các tay cầm vùng chọn để mở rộng phạm vi qua nhiều khối. Menu chỉnh sửa gốc cung cấp Copy và Select All, cùng Cut và Paste khi phiên đang cho phép chỉnh sửa, tùy theo vùng chọn và bộ nhớ tạm. `Session.SetEditable` cũng làm mới một menu đang mở. Định kiểu các núm kéo bằng `Styles.SelectionHandle` hoặc `Classes.SelectionHandle` trên base view.

Với một điều khiển văn bản tùy chỉnh, đưa tiêu điểm vào một `core.Input` và gọi `Ref.OpenEditMenu(bounds, actions)` với bounds tính theo điểm trên bề mặt và một bitmask gồm `core.EditCut`, `EditCopy`, `EditPaste` và `EditSelectAll`. Trên iOS và Android, `Input.OnEditAction` nhận Cut, Copy và Select All; Paste dùng `Input.OnPaste`. Gọi `Ref.CloseEditMenu` để đóng nó.

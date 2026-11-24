---
title: Ứng dụng TSX
description: Viết ứng dụng bằng TSX với class Tailwind, biên dịch trước mà không cần Node.
group: Khái niệm cốt lõi
order: 63
---

## Một ứng dụng TSX

Một ứng dụng TSX là một package Go nhúng mã nguồn của nó và chạy tệp đầu vào:

```go
//go:embed *.tsx nsgo.yaml
var src embed.FS

var Root = compiler.App(src, "App.tsx")

func main() { nsgo.Run(Root) }
```

```tsx
import { mount, signal, Text, View } from "nsgo";

function App() {
  const count = signal(0);
  return (
    <View class="gap-3 p-6" onPress={() => count.update((n) => n + 1)}>
      <Text class="text-lg font-bold">Pressed {count()} times</Text>
    </View>
  );
}

mount(App);
```

`go tool nsgo init -template tsx` tạo ra một ứng dụng như vậy, và `examples/tsx` là một ví dụ khác. `nsgo dev` và `nsgo build` biên dịch TSX trước, nên các package không kèm trình biên dịch; `go run` và `go test` biên dịch nó khi ứng dụng khởi động. Trong cả hai trường hợp đều không cần Node.

## Thành phần và module

TSX import các phần tử cơ bản từ `nsgo`, các thành phần của `base` từ `nsgo/base`, tay cầm chơi game từ `nsgo/gamepad` (theo capability `gamepad` trong các package đã ký), âm thanh từ `nsgo/audio` (theo `audio`) và các thành phần của `ui` từ `nsgo/ui`, khi ứng dụng đã import `tsx/ui`; `examples/gallery-tsx` trình bày chúng.

Với sơ đồ Mermaid, import `github.com/nsgo-dev/nsgo/tsx/diagram` để lấy hiệu ứng của nó trong host Go, cùng với `tsx/textview` hoặc `tsx/ui`. Việc này thêm `Diagram` vào `nsgo/base` mà không liên kết bộ phân tích sơ đồ vào những ứng dụng không dùng đến nó. Nó nhận `source`, `fontSize`, một `theme` một phần, `accessibilityLabel` và các prop định kiểu thông thường. Dùng trực tiếp hoặc cung cấp một trình hiển thị cho TextView:

```tsx
import { Diagram, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderDiagram={(code) => <Diagram source={code.code} errorClass="font-mono" />} />
```

Prop `renderDiagram` tương tự cũng hoạt động trên TextView của `nsgo/ui`. Một khối mã không phải Mermaid giữ trình hiển thị thông thường của nó; một sơ đồ không hợp lệ sẽ hiển thị mã nguồn.

Với công thức toán được dàn trang, import `github.com/nsgo-dev/nsgo/tsx/formula` để lấy hiệu ứng của nó trong host Go, cùng với `tsx/textview` hoặc `tsx/ui`. Việc này thêm `Formula` vào `nsgo/base`; những ứng dụng bỏ qua package này sẽ không đưa bộ máy toán học và phông chữ vào tệp nhị phân. Nó nhận `source`, `dialect` (mặc định là `tex`, hoặc `typst`), `display`, `accessibilityLabel` và các prop định kiểu. Cỡ chữ và màu sắc tuân theo văn bản xung quanh. Dùng trực tiếp hoặc cung cấp một trình hiển thị cho TextView:

```tsx
import { Formula, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderMath={(math) => <Formula source={math.tex} display={math.display} />} />
```

`renderMath` tương tự cũng hoạt động trên TextView của `nsgo/ui`. Công thức giữ nguyên mã TeX khi được sao chép. `Renderer` của Go vẫn là một cấu hình của host.

`UrlScheme.url()` đọc địa chỉ ứng dụng đang được theo dõi; `UrlScheme.setUrl(url, { history: "push" })` thêm một mục địa chỉ, hoặc thay thế nó khi bỏ qua options. `UrlScheme.on("change", fn)` theo dõi các thao tác quay lại, tiến tới và thay đổi hash của trình duyệt, và trả về một hàm hủy đăng ký. Xem [Địa chỉ ứng dụng](addresses.md) để biết về định tuyến, kiểm tra cùng nguồn gốc (same-origin) và lưu trữ deep link.

## Go từ TSX

Đăng ký Go để TSX import:

```go
tsx.Register("app/native", map[string]core.Node{"Badge": Badge{}})
tsx.RegisterFunctions("app/native", map[string]any{
	"greet": func(name string) string { return "Hello, " + name },
})
```

Khi đó `import { Badge, greet } from "app/native"` nhận các trường của `Badge` làm prop và gọi `greet`. Các hàm nhận `context.Context` trở thành promise, và `tsx.RegisterClass` biến các kiểu Go thành class mà TSX tạo bằng `new`.

Một prop tùy chọn được chuyển tiếp dưới dạng `undefined` sẽ dùng giá trị mặc định của nó. Khi đăng ký một thành phần Go có getter với giá trị mặc định khác giá trị zero của kiểu, hãy thêm một tùy chọn như `tsx.Default("Max", float64(100))`; giá trị mặc định đó cũng áp dụng khi một prop phản ứng trả về undefined, mà không thay thế thành phần.

Các trường dữ liệu có giá trị mặc định Go được ghi trong tài liệu cũng có thể bỏ qua: `width` và `height` của một nút `Flow` sẽ đo theo nội dung JSX khi vắng mặt, `width` bị bỏ qua của một cạnh dùng nét vẽ mặc định, và `pace` bị bỏ qua của một hàng hạn mức sẽ ẩn dấu tốc độ của nó.

Việc đăng ký các thành phần và class Go tìm các trường và phương thức được export của chúng bằng reflection khi ứng dụng chạy. Việc tra cứu đó khiến trình liên kết Go không loại bỏ được các phương thức được export mà không ai gọi, trong toàn bộ ứng dụng chứ không chỉ trong những gì TSX đăng ký, nên việc liên kết `tsx` làm tăng khoảng 20 MB cho một tệp nhị phân desktop đã strip, và càng nhiều hơn khi ứng dụng liên kết càng nhiều package.

## Package đã ký

Một ứng dụng có thể chạy TSX mà nó tải xuống, chẳng hạn các màn hình của chính nó được phân phối lại hoặc các plugin, sau khi xác minh package. Khai báo những gì mã của package được phép truy cập trong `nsgo.yaml` của nó; mọi thứ khác đều bị từ chối:

```yaml
capabilities:
  network:
    hosts: [api.example.com, "*.cdn.example.com"]
  open_url: true # or { schemes: [https] }
  host: [search, "player.*"] # the app's handlers it may call
  # webview: { hosts: [...], input: true, media: [microphone, camera] }, clipboard, picker, speech, text_to_speech, status_bar, notifications, biometrics, sign_in_with_apple, purchases, secrets, audio, gamepad, auth_session
```

Giữ một khóa gốc (root key) ngoại tuyến và dùng nó để chứng nhận các khóa phát hành, sau đó ký ứng dụng trong thư mục đó. `go tool nsgo keygen` in ra một cặp khóa và không ghi tệp nào; chạy nó một lần cho khóa gốc và một lần cho mỗi khóa phát hành. Khóa công khai được đưa vào cờ dòng lệnh và mã của ứng dụng; khóa bí mật nằm ngoài kho mã và chỉ đến được công cụ qua `NSGO_ROOT_KEY` và `NSGO_RELEASE_KEY`:

```sh
go tool nsgo keygen # once for the root, once for the release key
NSGO_ROOT_KEY=<root secret> go tool nsgo certify -key <release public> -apps 'com.example.*'
NSGO_RELEASE_KEY=<release secret> go tool nsgo publish -cert release.cert plugin
go tool nsgo verify -root <root public> plugin/dist/Plugin-1.0.0.nsgopkg
```

Ứng dụng tin cậy khóa công khai của khóa gốc, được viết trong mã của nó, xác minh package bên ngoài goroutine của engine và hiển thị nó trong `tsx.View`, được cách ly (sandbox) bên cạnh các view của chính ứng dụng. Package chỉ tiếp cận được ứng dụng thông qua kênh host mà view cung cấp cho nó, và giữ phần riêng của mình trong bộ lưu trữ và kho bí mật của ứng dụng:

```go
trust, err := pkg.OpenTrust(filepath.Join(cx.DataDir(), "trust"))
trust.AddRoot(root)
p, err := trust.Verify(data) // refuses tampered, revoked and older builds
channel := core.NewHost()
channel.Handle("search", func(c *core.HostCall) { c.Resolve(results(c.Data)) })
view := tsx.View{Package: p, Sandbox: tsx.Sandbox{Host: channel}}
core.Catch{Try: view.Render, Fallback: failed}
trust.Accept(p) // once it ran, older builds are refused
```

`Sandbox.Grant` cấp cho nó ít hơn những gì nó khai báo, `Sandbox.Limits` đặt các ngân sách thời gian và hạn mức khác, và `Sandbox.Modules` cung cấp các hàm và thành phần Go ngoài những gì NSGo có sẵn. Các phím tắt bàn phím, trình xử lý quay lại và ngăn xếp điều hướng của nó chỉ có hiệu lực khi tiêu điểm bàn phím nằm trong view của nó, và nó chỉ tự di chuyển tiêu điểm (`focus()`, `autoFocus`, `Keyboard.dismiss`, `setAccessibilityFocus`) khi tiêu điểm đã ở đó hoặc để đáp lại một lần nhấn của người dùng trong view đó. Nó hiển thị trong một view riêng lấp đầy không gian mà view cha cấp cho, không hơn, và bị cắt theo view đó, kể cả menu, popover và toast; trong một scroll view, hãy đặt nó vào một view để nó nhận chiều cao theo nội dung. Hộp thoại, cảnh báo, menu ngữ cảnh hay thông báo của nó cũng chỉ hiển thị theo cách tương tự, và thao tác quay lại sẽ đóng hộp thoại của nó ngay cả khi nó không gỡ hộp thoại đó. `View.Export` chỉ định một thành phần mà module đầu vào của package export, được hiển thị thay cho gốc mà tệp đầu vào mount, khi đó `mount` của nó không hiển thị cũng không chạy thành phần của nó, và được truyền `View.Props`, một map có thể mã hóa JSON được sao chép cho mỗi view, nên một package cung cấp được nhiều view, mỗi view có trạng thái riêng, dưới cùng các capability và giới hạn; một export bị thiếu sẽ panic với `tsx.ErrNoViewExport` và một export không phải hàm sẽ panic với `tsx.ErrViewExportNotComponent`. Một package có tệp đầu vào export các hàm là một plugin dạng hàm: `tsx.Load(cx, p, tsx.Sandbox{})` chạy nó một cách ẩn và đồng bộ trên goroutine của bên gọi, dưới một `cx` thuộc về một cây view (không có cây view thì nó trả về `tsx.ErrNoTree`), hoặc trả về những gì module đầu vào ném ra khi tải, và `Plugin.Call` gọi các hàm đó. `go tool nsgo dev -limits` chạy một ứng dụng như một view như vậy sẽ chạy, và `go tool nsgo revoke` ký một danh sách các khóa phát hành và package bị thu hồi cho `Trust.UpdateRevocations`. `examples/plugins` yêu cầu người dùng cấp các capability của một package, cài đặt và chạy nó. `Limits.Memory`, mặc định 64 MiB, đặt ngân sách cho bộ lưu trữ gốc được đo lường dành cho mảng, chuỗi, buffer và typed array. Điều này bao gồm các bản sao do `concat`, `flat`, `slice`, spread mảng, nối chuỗi, template, `replace`, `replaceAll`, `TextEncoder` và `Blob` tạo ra. `memory=<bytes>` trong `-limits` thay đổi ngân sách, và `memory=0` tắt nó. Việc kiểm tra diễn ra trước khi cấp phát. Một package vượt ngân sách sẽ dừng với `ERR_NSH_QUOTA_EXCEEDED` và tài nguyên `memory` sau khi Go đã kiểm tra bộ nhớ có thể thu hồi. Các đối tượng thông thường, closure, stack, các lần cấp phát nhỏ, và các built-in cùng API host không được đo lường cũng có thể cấp phát. Trình duyệt giữ heap riêng của nó. Một ứng dụng tự ký package bằng `pkg.Build`, hoặc bằng `go tool nsgo publish -extra <file>` với dữ liệu byte của tệp đó, có thể đặt siêu dữ liệu riêng vào `pkg.Contents.Extra`, chẳng hạn những gì một plugin thêm vào menu của nó: siêu dữ liệu được ký cùng package, và `Verify` trả về nó nguyên vẹn dưới dạng `p.Extra` để ứng dụng phân tích, nhờ vậy ứng dụng đọc được nó mà không phải chạy mã của package.

Một ứng dụng có thể ký các tài liệu JSON của riêng mình, chẳng hạn một danh mục plugin, bằng `pkg.SignDocument` và xác minh chúng bằng `Signed.Open`, với các khóa mà `nsgo keygen` in ra. Hãy cho mỗi loại tài liệu một ngữ cảnh `app:` riêng có kèm phiên bản, như `app:catalog-v1`: một tài liệu không bao giờ được chấp nhận như một loại khác, cũng không như một package, chứng chỉ hay danh sách thu hồi. `Open` tin cậy đúng những khóa công khai được truyền vào, không có chứng chỉ hay danh sách thu hồi, và từ chối các trường trong phần thân mà kiểu được giải mã vào không có. Phong bì không chứa thời gian hay số thứ tự, nên một tài liệu cũ hơn vẫn được xác minh giống như một tài liệu mới hơn: hãy đặt phiên bản hoặc thời hạn vào phần thân và kiểm tra nó.
Trong một sandbox gốc, `JSON.stringify`, `JSON.parse` có reviver, `Array.prototype.join`, `toString`, `toLocaleString` và `flat`, và do đó cả `String()` của một mảng, sẽ ném ra một `RangeError` có thể bắt được khi chúng đi sâu quá 1.000 cấp, tính cả các lời gọi lồng nhau, như `toString` của một mảng và `join` mà nó gọi đều được tính; dữ liệu sâu hơn sẽ làm treo lời gọi hoặc kết thúc ứng dụng. Trên web dùng các built-in của trình duyệt.

Trên các mục tiêu gốc, `DefaultLimits.Stack` là 1.000 frame. Đặt `Limits.Stack` hoặc `-limits stack=<frames>` để chọn độ sâu khác, tối đa 10.000 frame. Giá trị Go bằng hoặc nhỏ hơn không và đầu vào vượt trần sẽ dùng 10.000. Trên web dùng giới hạn stack của trình duyệt.

Các hàm gốc gọi lẫn nhau được tính vào `Limits.Stack` như các lời gọi JS: một getter, `toString` hoặc `valueOf` là built-in, một biểu thức chính quy có `exec` là một phương thức khác của chính nó, `Reflect.apply` và `Reflect.construct` lồng trong nhau, cùng các chuỗi hàm bound hoặc proxy sẽ ném ra một `RangeError` có thể bắt được khi vượt giới hạn thay vì kết thúc ứng dụng, và một giá trị của plugin lồng sâu hơn giới hạn đó sẽ làm lời gọi chuyển nó sang Go thất bại với một `RangeError`. Điều này đến từ bản fork của goja mà NSGo yêu cầu, `github.com/nsgo-dev/goja`, mà ứng dụng của bạn nhận được cùng NSGo mà không cần thay đổi gì trong `go.mod`.

Trên các mục tiêu gốc, `Limits.Call` giới hạn mỗi lát (slice) thực thi JavaScript, bao gồm các callback của timer và các phần tiếp nối promise sau `await`. Khi một lát bị cắt, một `Plugin.Call` đang chờ sẽ báo `tsx.ErrBudget` một lần, dừng các timer còn lại và bỏ qua các câu trả lời đến muộn từ host. Các lời gọi độc lập vẫn tiếp tục chạy, bao gồm các phản ứng promise được xếp hàng trong lát bị ngắt. Một lời gọi giải quyết promise của lời gọi khác vẫn báo kết quả hoặc lỗi của chính nó. Biểu thức chính quy có quay lui dùng ngân sách tải hoặc ngân sách gọi, cái nào ngắn hơn, làm thời gian chờ khớp; một lần hết thời gian chờ không được xử lý cũng trở thành `tsx.ErrBudget`.

`Limits.CallTotal` thêm một hạn chót theo thời gian thực cho mỗi lời gọi plugin và sự kiện view được cách ly. Nó bao gồm thời gian chờ và giữ các timer, interval, frame, phần tiếp nối promise và câu trả lời từ host của lời gọi ngay cả sau khi lời gọi đã hoàn tất. Đến hạn chót, công việc đang chờ bị bỏ và một lời gọi vẫn đang chờ sẽ thất bại với `tsx.ErrBudget`. Công việc của module đầu vào bị giới hạn bởi `Limits.Load`, dùng `CallTotal` nếu `Load` bằng không. Các sự kiện host độc lập dùng `CallTotal` tính từ lúc được chuyển đến. Dùng `Plugin.CallWithBudget` để lấy một handle tới hạn chót của một lời gọi đang chờ. Trong `Scope.Post` của host, gọi `budget.Extend(duration)` trước hạn chót đó để chỉ cho lời gọi này thêm thời gian.

`Plugin.Interrupt` dừng runtime từ bất kỳ goroutine nào, bỏ các timer đang chờ và làm thất bại các lời gọi hiện tại cũng như sau này. `Plugin.Stopped()` và `Plugin.Err()` cũng có thể được đọc từ bất kỳ goroutine nào. `Err()` là nil khi runtime đang chạy và trả về lý do dừng đầu tiên sau khi đã dừng. Đóng plugin hoặc hủy chủ sở hữu của nó theo cách thông thường sẽ chạy các hàm dọn dẹp và dừng nó với `tsx.ErrPluginClosed`. Đặt `Sandbox.OnStop` khi gọi `tsx.Load` để nhận lý do đó một lần trên goroutine của engine, sau khi các lời gọi đang chờ thất bại.

Đặt `Sandbox.OnError` khi tải một plugin dạng hàm để nhận các lỗi không được bắt từ timer, animation frame, `queueMicrotask` và `reportError` bên ngoài một `Plugin.Call` đang chờ. Trình xử lý của mỗi plugin chạy trên goroutine của engine, nên host có thể quy lỗi cho đúng plugin và gọi `Plugin.Interrupt` khi bộ ngắt mạch của nó mở. Các lần cắt theo ngân sách lát trên nền tảng gốc cũng báo `tsx.ErrBudget`, kể cả một công việc promise mà lời gọi của nó đã hoàn tất. Các lỗi callback thông thường không dừng plugin. Lỗi khi tải và lỗi kết quả lời gọi vẫn được trả về cho bên gọi, và view vẫn dùng `core.Catch`; khi không có trình xử lý, hành vi hiện có của ranh giới lỗi và báo cáo của ứng dụng vẫn giữ nguyên.

Đặt `Sandbox.OnViewRuntime` để nhận `*tsx.ViewRuntime` của mỗi view trên goroutine của engine trước khi module đầu vào của nó chạy. Gọi `Interrupt` từ bất kỳ goroutine nào để dừng công việc của nó và giữ nguyên view được hiển thị cuối cùng. `Stopped` và `Err` báo cáo trạng thái kết thúc của nó, với `tsx.ErrViewClosed` khi bị hủy theo cách thông thường. `Sandbox.OnStop` cũng báo cáo lý do kết thúc của một view một lần.

`Plugin.JSTime` và `ViewRuntime.JSTime` báo cáo thời gian đã trôi qua trong các lần đi vào JavaScript của runtime gốc, bao gồm cả lần hiện tại. Thời gian chờ giữa các lần đi vào và thời gian bên trong các hàm Go đồng bộ đã đăng ký không được tính. `ViewRuntime.EngineTime` còn bao gồm các lời gọi Go đồng bộ và công việc phản ứng, mount, commit và dàn trang thuộc về view, kể cả công việc vẫn đang chạy. Nó không tính thời gian chờ giữa các lần đi vào engine, công việc thuộc về host hoặc view khác, phần quản lý commit dùng chung và việc vẽ gốc. Cả hai bộ đếm đều an toàn để đọc từ bất kỳ goroutine nào. Các lượt JavaScript của trình duyệt bên ngoài NSGo không thể đo lường hay chiếm quyền. Ở đó cả hai bộ đếm đều báo không khả dụng và `Interrupt` dừng các callback NSGo trong tương lai khi host giành lại quyền điều khiển.

Web hiện chưa thể cô lập một package khỏi trang: mã của nó sẽ tiếp cận mạng, bộ nhớ tạm và bộ lưu trữ của chính trang, kể cả kho tin cậy, vượt qua các capability của nó. Ở đó `tsx.View` và `tsx.Load` từ chối package với `tsx.ErrUnisolated`, trừ khi bạn đặt `Sandbox.WebUnisolated` để chạy nó với quyền hạn của trang, điều chỉ phù hợp với những package mà bạn tin cậy ngang với chính ứng dụng.

## Trình soạn thảo và kiểm thử

Một `tsconfig.json` kế thừa `.nsgo/tsconfig.json` cung cấp cho trình soạn thảo các kiểu của TSX, được `go tool nsgo dev` và `go tool nsgo check` ghi ra. Các tệp `*.test.tsx` đặt cạnh ứng dụng, viết bằng `nsgo/test`, chạy cùng các bài kiểm thử Go của ứng dụng ([Kiểm thử](testing.md)).

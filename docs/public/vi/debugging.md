---
title: Chạy và gỡ lỗi
description: Chạy ứng dụng trên mọi nền tảng đích với hot reload, đồng thời theo dõi lỗi, cập nhật, bố cục, tốc độ và các ngôn ngữ khác.
group: Bộ công cụ
order: 20
---

## Chạy ứng dụng

```sh
go tool nsgo dev
```

`dev` build ứng dụng cho máy hiện tại và mở nó, hoặc cho nền tảng đích được chỉ định trong `dev.target` của `nsgo.yaml`. Mỗi lần lưu một tệp Go, lệnh sẽ build lại và khởi động lại ứng dụng, tự động chạy `go mod tidy` trước nếu ứng dụng nhập một gói mà `go.mod` hoặc `go.sum` chưa có; nếu chỉ sửa TSX, ứng dụng đang chạy sẽ tự cập nhật tại chỗ và giữ nguyên trạng thái. Quá trình chạy kết thúc khi ứng dụng đóng.

Trên macOS và Windows, một lượt build lại trong chế độ dev gốc sẽ mở lại cửa sổ của view chính ở đúng vị trí và kích thước thông thường trước đó trong phiên hiện tại. Windows còn khôi phục cả trạng thái toàn màn hình. Trên Linux, GTK khôi phục kích thước thông thường và yêu cầu chế độ toàn màn hình gần nhất; vị trí cửa sổ do trình quản lý cửa sổ quyết định.

`-target` chạy ứng dụng trên nền tảng khác, còn `-device` chọn iPhone, Apple TV, Apple Watch, trình giả lập hoặc thiết bị Android theo tên, UDID hoặc số sê-ri:

```sh
go tool nsgo dev -target tui                # terminal cells; diagnostics in .nsgo/tui.log
go tool nsgo dev -target web                # in the browser, at the address it prints, on -port (8080 by default)
go tool nsgo dev -target ios_simulator      # in the iOS simulator
go tool nsgo dev -target ios                # on the connected iPhone, signed by Xcode
go tool nsgo dev -target tvos_simulator     # in the Apple TV simulator
go tool nsgo dev -target watchos_simulator  # in the Apple Watch simulator
go tool nsgo dev -target android            # on a connected device or emulator, over adb
go tool nsgo dev -target miniprogram_wechat # in WeChat DevTools (also _qq, _alipay, _douyin)
```

Theo mặc định, máy chủ dev lắng nghe trên mọi giao diện mạng. Dùng `-host 127.0.0.1` để giữ kết nối trên loopback, kể cả khi Android kết nối qua `adb reverse`, hoặc chỉ định địa chỉ IP của một giao diện mạng cục bộ để chỉ lắng nghe trên đó. Dùng một địa chỉ có thể truy cập qua mạng LAN để dùng các tính năng dev trên thiết bị Apple thật.

`-h` liệt kê mọi cờ dòng lệnh.

## Trình gỡ lỗi Go

Đặt một phiên bản [Delve](https://github.com/go-delve/delve/blob/master/Documentation/installation/README.md) tương thích với phiên bản Go của bạn vào `PATH` với tên `dlv`. Khởi động ứng dụng máy tính để bàn gốc kèm thông tin gỡ lỗi và tắt tối ưu hóa của Go:

```sh
go tool nsgo dev -target native -debug
```

Sao chép số cổng từ dòng `API server listening at` của Delve. Trong VS Code có cài tiện ích mở rộng Go, hãy thêm cấu hình gắn kết (attach) và thay `40000` bằng số cổng đó:

```json
{
  "name": "Attach to NSGo",
  "type": "go",
  "request": "attach",
  "mode": "remote",
  "debugAdapter": "dlv-dap",
  "host": "127.0.0.1",
  "port": 40000
}
```

Trong GoLand, hãy thêm cấu hình chạy **Go Remote** với host `127.0.0.1` và cùng số cổng đó. Đặt breakpoint trong mã Go của ứng dụng, gắn trình gỡ lỗi vào và thực hiện thao tác đi tới đoạn mã đó. Sau khi Go build lại thành công, hãy kết nối lại trình gỡ lỗi vào cùng địa chỉ đó để gửi các breakpoint sang tiến trình mới. Các thay đổi TSX vẫn dùng hot reload bình thường.

`-debug` chạy trên macOS, Windows và Linux với `-target native`. Hãy dùng bản build dev để gỡ lỗi và chỉ dùng `-release` khi đo hiệu năng bản phát hành.

## Xem trước thành phần

Đặt một hoặc nhiều hàm `PreviewXxx` bên cạnh các thành phần trong một gói Go có thể nhập được. Lưu chúng trong một tệp Go được bảo vệ bằng thẻ build xem trước (`nsgopreview`).

```go
//go:build nsgopreview

package components

import "github.com/nsgo-dev/nsgo/core"

func PreviewEmpty(cx *core.Scope) core.Node {
    return core.Text{Content: "No items"}
}
```

```sh
go tool nsgo dev -preview ./components
go tool nsgo dev -target web -preview ./components
```

Đánh dấu các mẫu thử (fixture) bạn muốn hiển thị cùng nhau rồi chọn ngôn ngữ (locale), chủ đề (theme), giao diện sáng hoặc tối, kích thước khung nhìn và hướng bố cục. Trình xem trước lấy tệp `nsgo.yaml` của ứng dụng gần nhất cùng các danh mục bản dịch, `theme` đã cấu hình và tài nguyên của nó. Truyền đường dẫn thư mục ứng dụng sau các cờ để chọn một cấu hình khác trong cùng module; khi đó đường dẫn xem trước được tính tương đối theo thư mục ứng dụng.

Mỗi lần lưu tệp sẽ build lại bản xem trước mà vẫn giữ nguyên lựa chọn và thiết lập hiển thị của bạn. Thẻ `nsgopreview` loại bỏ các mẫu thử khỏi các bản build phát hành thông thường. Dùng chính mẫu thử đó trong một tệp `_test.go` có gắn thẻ với `nsgotest.Start(t, PreviewEmpty)` và chạy bằng `GOFLAGS=-tags=nsgopreview go tool nsgo test -target macos ./components` để chụp ảnh màn hình gốc.

Sau lần cài đặt đầu tiên của ứng dụng chính với `-target ios_simulator`, một lần build lại chỉ có mã Go sẽ thay thế tệp thực thi đã cài và mở lại ứng dụng. Việc chỉnh sửa `nsgo.yaml`, tài nguyên đóng gói hoặc biểu tượng ứng dụng sẽ kích hoạt cài đặt lại.

Dùng `go tool nsgo dev -target android -android-host` khi làm giao diện Android để có vòng lặp build lại Go ngắn hơn. Công cụ cài đặt runtime gốc một lần và chạy mã Go trên máy phát triển. Mỗi lần build lại Go sẽ kết nối lại tới cùng Activity qua adb. Thay đổi tài nguyên đóng gói, các lớp biên dịch sẵn, biểu tượng trình khởi chạy hoặc `nsgo.yaml` sẽ cài lại APK runtime. Tiến trình Go sử dụng ràng buộc build, tệp và mạng của máy phát triển. `Scope.DiskSpace` báo cáo dung lượng ổ đĩa chứa `DataDir` của máy chủ. Hãy dùng lệnh dev Android thông thường khi cần kiểm tra các tệp `_android.go` và hành vi Go trực tiếp trên thiết bị.

## Lỗi

Lỗi build sẽ hiển thị đè lên trên ứng dụng đang chạy, kèm nút sao chép lỗi và nút build lại; hãy sửa tệp và lần lưu tiếp theo sẽ thay thế thông báo lỗi đó. Một thành phần bị panic trong lúc kết xuất cũng hiển thị theo cách tương tự. Tổ hợp `Command+R` trong ứng dụng (hoặc `Control+R` ngoài các nền tảng Apple) cho phép build và khởi động lại bất kỳ lúc nào, kể cả khi đang hiện lỗi, trừ phi ứng dụng tự gán phím đó bằng `cx.OnKey` hoặc dùng làm một mục đang bật trong trình đơn ứng dụng; trên trình duyệt, phím này tải lại trang như thông thường. Đầu ra chuẩn của ứng dụng, như `log.Print`, xuất hiện trong terminal bên dưới các dòng của công cụ CLI.

Trước khi chạy, `go tool nsgo check` kiểm tra (`vet`) ứng dụng và kiểm tra kiểu cho TSX; `-target all` kiểm tra cho mọi nền tảng đích, nên mã nằm sau build tag của nền tảng khác cũng được kiểm tra ngay từ máy Mac.

### Nhật ký của ứng dụng Apple đóng gói

Trên macOS, iOS, tvOS và watchOS, bộ ghi nhật ký mặc định của Go ghi vào hệ thống nhật ký hợp nhất (unified log) của Apple, bao gồm `log.Print`, chẩn đoán của NSGo và đầu ra console của TSX. Trong ứng dụng Console, hãy lọc theo bundle identifier của ứng dụng đang chạy làm `subsystem` và `nsgo` làm `category`. Trên macOS bạn cũng có thể truy vấn kho nhật ký:

```sh
log show --last 10m --predicate 'subsystem == "dev.example.hello"'
```

Các bản ghi sử dụng loại log mặc định và giữ nguyên tiền tố cảnh báo hoặc lỗi. Văn bản trong đó là công khai, vì vậy không ghi thông tin đăng nhập và bí mật vào nhật ký. Thông điệp dài được tách thành nhiều mục. Đầu ra trên terminal vẫn khả dụng trong quá trình phát triển; một log writer hoặc `slog` handler tùy chỉnh sẽ tự kiểm soát đầu ra của chính nó.

### Web view trên Linux

Ubuntu 24.04 trở lên có thể hạn chế user namespace không đặc quyền thông qua AppArmor. Khi bubblewrap không thể khởi động sandbox của WebKitGTK, NSGo sẽ ghi lỗi vào nhật ký và hiển thị một khung giữ chỗ. Một số phiên bản WebKitGTK khác lại thoát với tín hiệu `SIGTRAP` khi mở web view đầu tiên. [Tài liệu AppArmor của Ubuntu](https://documentation.ubuntu.com/security/security-features/privilege-restriction/apparmor/)

Hãy sử dụng profile AppArmor cho bubblewrap của bản phân phối nếu có, chẳng hạn `bwrap-userns-restrict`. Nếu không, quản trị viên có thể nạp một profile cấp quyền `userns` cho `/usr/bin/bwrap`, lấy [profile từ CI Linux của NSGo](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/test.yml) làm điểm khởi đầu. Quản trị viên quản lý profile này vì nó áp dụng cho mọi ứng dụng sử dụng bubblewrap của hệ thống.

`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1` tắt sandbox tiến trình con của WebKit. Chỉ dùng biến này cho quá trình phát triển cục bộ với nội dung đáng tin cậy.

## DevTools

```sh
go tool nsgo dev -devtools
```

`-devtools` mở DevTools trên trình duyệt, được phục vụ bởi `nsgo dev` cho mọi nền tảng đích, kể cả ứng dụng trên điện thoại. Công cụ hiển thị ứng dụng đang chạy theo thời gian thực:

- **Elements**: cây các view. Mỗi mục hiển thị thành phần đã tạo ra nó, liên kết tới dòng mã trong trình soạn thảo, bộ chọn (selector) mà bài kiểm thử dùng để tìm nó, các lớp (class), khung bao (bounds), lề (margin), viền (border) và phần đệm (padding) sau khi bố cục, cùng kiểu dáng được thiết lập. Di chuột qua một hàng sẽ vẽ lề, phần đệm và nội dung của nó ngay trong ứng dụng tương tự DevTools của trình duyệt, và **Pick** cho phép bạn nhấn vào một view trong bất kỳ cửa sổ, panel, sheet hay view nhúng nào để định vị nó trên cây.
- **State**: các signal do mã của bạn tạo ra, nằm bên dưới các thành phần và computation sở hữu chúng, kèm giá trị của chúng, giá trị của các memo, và số lượng computation đang đọc mỗi giá trị. Một giá trị không có computation nào đọc sẽ không làm thay đổi gì trên màn hình. Chuỗi, boolean và số có thể chỉnh sửa trực tiếp để bạn thử nghiệm trạng thái mà không cần sửa mã. Các giá trị do chính NSGo tạo ra nằm sau mục **NSGo's own**.
- **Profiler**: thời gian của từng commit, tách thành thời gian chạy mã của bạn trước đó, thời gian commit kèm tính toán bố cục, và thời gian áp dụng lên luồng chính của nền tảng. Công cụ cũng hiển thị các dòng mã có lệnh ghi gây ra commit và mở sẵn danh sách các computation đã chạy, xếp từ lâu nhất trở xuống. Danh sách thứ hai nêu tên các computation chạy nhiều nhất trong giây vừa qua theo dòng mã đã tạo ra chúng. Runtime Android áp dụng các lô thay đổi trong mã Java riêng nên commit trên Android không ghi nhận thời gian phía nền tảng.
- **Recorder**: ghi lại thao tác nhấn, gõ, xóa và cuộn của bạn trong ứng dụng, xuất thành một tệp kiểm thử `nsgotest` để sao chép vào tệp `_test.go`. Trong mục Elements, **Assert shown** và **Assert text** thêm bước kiểm tra view đang chọn. Kiểm tra văn bản lưu lại chữ theo đúng ngôn ngữ ứng dụng đang hiển thị, vì vậy hãy ghi thao tác bằng đúng ngôn ngữ mà bài kiểm thử của bạn chạy.
- **Highlight updates**: vẽ viền quanh từng view bị thay đổi bởi một commit, tương tự cờ `-highlight`.
- **Layout**: bật và tắt [kiểm tra bố cục](#bố-cục) trên thiết bị, tương tự cách cờ `-layout` khởi chạy.

Trạng thái, computation và lệnh ghi của một ứng dụng TSX được đặt tên theo dòng mã TSX của chúng thông qua source map của bundle, và liên kết thẳng tới các tệp trong ứng dụng của bạn.

DevTools đọc dữ liệu từ ứng dụng thông qua máy chủ của `nsgo dev`, vốn lắng nghe trên mạng để điện thoại có thể kết nối tới. Mỗi lần chạy tạo ra một token mà các địa chỉ `nsgo dev` in ra và mở cho DevTools cũng như nền tảng đích web đều mang theo sau dấu `#`; máy chủ chỉ trả lời các yêu cầu có mang token đó, vì vậy hãy mở DevTools và trang web từ đúng các địa chỉ này. Bất kỳ ai được bạn chia sẻ địa chỉ đều có thể thay đổi trạng thái của ứng dụng khi đang chạy và thêm bước vào bài kiểm thử mà Recorder ghi lại, nên hãy đọc kỹ bài kiểm thử đã ghi trước khi chạy. Người khác trên cùng mạng và các trang web khác bạn truy cập đều không thể tiếp cận được bất kỳ dữ liệu nào trong đó.

## Cập nhật giao diện

```sh
go tool nsgo dev -highlight
```

`-highlight` vẽ viền trong nửa giây quanh từng view vừa được một commit thay đổi, nhờ đó một bản cập nhật vẽ lại nhiều hơn mức cần thiết sẽ lộ rõ ngay trên màn hình khi bạn sử dụng ứng dụng. Đường viền vẽ đè lên mọi cửa sổ, panel, hộp thoại modal, sheet iOS và view nhúng (`nsgo.NewView`). DevTools có thể bật và tắt tính năng này trong khi ứng dụng đang chạy.

```sh
go tool nsgo dev -trace
```

`-trace` in ra từng commit của ứng dụng đang chạy: số thứ tự, thời gian xử lý và số lượng mount item đã gửi, giúp bạn thấy một lần cập nhật đã tạo mới, thay đổi hay gỡ bỏ những view nào. Phía trên các mục đó là nguồn gốc dẫn đến commit: các dòng mã đã ghi vào signal kể từ commit trước (xếp từ dòng ghi nhiều lần nhất), cùng số lượng binding, effect và memo đã chạy.

```text
── commit 12: 40 items, commit 1.65 ms
   after app/market.go:473 ×4, app/market.go:487 ×4, app/market.go:384
   ran 118 bindings, 18 effects, 5 memos
```

Một dòng ghi quá nhiều lần, hoặc quá nhiều binding chạy chỉ vì một lần ghi, chính là chỗ cần gộp các lệnh ghi (`cx.Batch`) hoặc thu hẹp phạm vi mà binding đọc tới. `go tool nsgo test -trace` cũng in ra thông tin tương tự bên dưới từng bước của bài kiểm thử.

## Hồ sơ hiệu năng runtime

Thu thập hoạt động CPU của Go, bộ nhớ heap hoặc execution trace của ứng dụng đang chạy bằng `nsgo dev`. Ứng dụng gửi bản thu về máy chủ dev để lưu lại trên máy đang chạy lệnh.

```sh
go tool nsgo dev -target android -profile cpu -profile-duration 10s
go tool pprof -top .nsgo/profiles/cpu.pprof
```

Ví dụ đo CPU cũng hoạt động trên máy tính để bàn, iPhone hoặc trình giả lập iOS. Trên iPhone, hãy cấp quyền Local Network nếu hệ thống hỏi. Bản chụp heap là ảnh chụp nhanh tại một thời điểm và chạy được cả trên web. Để thu các sự kiện của scheduler và goroutine, chạy `go tool nsgo dev -profile trace` rồi mở `.nsgo/profiles/execution.trace` bằng `go tool trace`. Thời lượng mặc định của CPU và execution trace là 10 giây. `-profile-output` đặt đường dẫn tệp lưu cục bộ. Đối với xem trước thành phần, đường dẫn mặc định và đường dẫn tương đối tính theo thư mục ứng dụng. Một phiên chạy dev sẽ lưu bản thu hoàn chỉnh đầu tiên rồi tiếp tục cho ứng dụng chạy.

```sh
go tool nsgo dev -target web -profile heap
go tool pprof -top .nsgo/profiles/heap.pprof
```

Runtime trình duyệt của Go hỗ trợ hồ sơ heap và execution trace. Để lấy mẫu CPU trên trình duyệt, hãy dùng công cụ Performance của chính trình duyệt. Chỉ chọn một trong hai cờ `-profile` hoặc `-release`. Khi đóng gói phát hành bằng `nsgo build`, client thu thập dữ liệu dev sẽ bị loại bỏ. [Chẩn đoán runtime Go](https://go.dev/doc/diagnostics)

## Bố cục

```sh
go tool nsgo dev -layout
go tool nsgo dev -layout=bounds,grid=4
```

`-layout` khởi động ứng dụng kèm công cụ kiểm tra bố cục của DevTools trên mọi cửa sổ, panel, sheet iOS và view nhúng, vẽ đường viền quanh từng view: khung viền (border box) màu xanh lam, mép trong của phần đệm (padding) màu xanh lục và mép ngoài của lề (margin) màu cam, mỗi đường đều được cắt gọn tại những nơi bị scroll view hoặc view cắt nội dung (clip) che khuất. `grid` vẽ lưới 8 điểm, còn `grid=N` vẽ lưới N điểm (tối thiểu 4). Truyền giá trị sau dấu `=`: đứng một mình, cờ mặc định mang giá trị `bounds`.

Một bảng chỉ số ở góc dưới hiển thị kích thước cửa sổ theo đơn vị điểm (pt), kèm các nút bật/tắt khung bao, lưới và thước đo. Trên Apple TV và các thiết bị chỉ dùng điều khiển từ xa, tiêu điểm có thể di chuyển tới các nút trên bảng chỉ số để bạn bật tắt bằng remote mà không rời ứng dụng. Khi thước đo đang bật, thao tác nhấn sẽ chọn hai điểm thay vì truyền tới ứng dụng, và bảng chỉ số hiển thị khoảng cách ngang dọc giữa hai điểm đó; lần nhấn thứ ba sẽ bắt đầu đo lại từ đầu. Trên Apple TV hoặc khi dùng bàn phím, các phím mũi tên điều hướng (hoặc vuốt bàn di chuột Siri Remote) di chuyển tâm ngắm ngang qua màn hình, còn Select, Return hoặc Space dùng để chọn điểm — cập nhật khoảng cách trực tiếp cho đến khi điểm thứ hai khóa phép đo.

| Phím | Tác dụng |
| --- | --- |
| `Cmd+Option+Shift+L`, `Ctrl+Alt+Shift+L` | Hiện hoặc ẩn công cụ kiểm tra bố cục, trong bản build có `-layout` |
| Phím mũi tên, D-pad trên remote / vuốt | Di chuyển tâm ngắm của thước đo theo bước lưới (mặc định 8 pt) |
| `Shift` + Phím mũi tên | Di chuyển tâm ngắm của thước đo từng 1 pt |
| Select, `Return`, `Space` | Chọn điểm tại tâm ngắm khi đang đo |
| `Esc`, quay lại, Menu | Dừng đo |

Nếu ứng dụng của bạn gán cùng tổ hợp phím, gán phím của ứng dụng sẽ được ưu tiên. Khi không có phím bấm, như trên điện thoại, hãy chạy kèm `-devtools` rồi bật/tắt công cụ kiểm tra bố cục bằng nút **Layout** trong DevTools; ẩn nó đi khi cần chạm tới điều khiển nằm dưới bảng chỉ số. Các backend gốc và web đọc ma trận trình diễn (presentation matrix) để khung bao, Pick và công cụ kiểm tra bố cục bám sát theo phép biến đổi đang hiển thị, hoạt ảnh và thao tác cuộn; watchOS bám theo phép vẽ affine của nó. HarmonyOS và mini program giữ trình theo dõi phía Go cho đến khi tầng kết xuất của chúng báo cáo hình học. Việc vẽ lại các lớp này làm tốn thời gian ở mỗi lần cập nhật, nên hãy đo tốc độ khi tắt chúng đi.

## Tốc độ

```sh
go tool nsgo dev -fps
go tool nsgo dev -release -fps
```

`-fps` hiển thị bộ theo dõi hiệu năng phủ lên ứng dụng trên macOS, iOS, Windows, Linux và Android. Nó đo số khung hình và số commit, riêng Android còn hiển thị thời gian khứ hồi (round-trip) cho văn bản gốc. Bản build dev có mang theo dev client; hãy đo với `-release`, cờ build giống hệt như `nsgo build`.

## Ngôn ngữ và giao diện sáng tối

```sh
go tool nsgo dev -lang zh-Hans
go tool nsgo dev -lang en-XA   # accented and longer, to find untranslated and cut text
go tool nsgo dev -lang ar-XB   # right to left
```

`-lang` khởi động ứng dụng với một ngôn ngữ (locale), bao gồm cả ngôn ngữ giả (pseudo-locale) ([Bản địa hóa](localization.md)).

## Agent tự động hóa

```sh
go tool nsgo dev -automation
go tool nsgo dev -target android -automation
go tool nsgo dev -target web -automation
```

`-automation` cho phép một AI agent hoặc bộ chạy kiểm thử đầu-cuối (end-to-end) thao tác với ứng dụng đang chạy giống như người dùng, trên macOS, Windows, Linux (`-target native`), Android (`-target android`), web (`-target web`), các mini program trong trình giả lập của công cụ phát triển tương ứng (`-target miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` hoặc `miniprogram_douyin`) và thiết bị cùng trình giả lập Apple (`-target ios`, `tvos`, `watchos`, `ios_simulator`, `tvos_simulator` hoặc `watchos_simulator`): liệt kê cửa sổ, phác họa những gì cửa sổ đang hiển thị, nhấn, nhập chữ, gửi phím, cuộn, kéo, di chuột, chọn mục trình đơn, trả lời cảnh báo, chờ phần tử xuất hiện và chụp ảnh màn hình. Các agent kết nối qua MCP thông qua `go tool nsgo mcp` — lệnh này tự dùng phiên `dev -automation` đang chạy cho thư mục đó, hoặc khởi chạy một phiên mới với cùng các cờ (chẳng hạn `-target android`) rồi dừng phiên đó khi client ngắt kết nối:

```sh
claude mcp add nsgo -- go tool nsgo mcp
```

Các client khác nhận cùng lệnh đó — `go` với các đối số `tool nsgo mcp` — chạy trong thư mục ứng dụng. Các view được gọi tên bằng bộ chọn của [bài kiểm thử](testing.md#một-bài-kiểm-thử), hoặc bằng các tham chiếu `view:` mà ảnh chụp cây giao diện (snapshot) in ra (các tham chiếu này sẽ hết hiệu lực mỗi khi lưu tệp khiến ứng dụng khởi động lại).

Agent thao tác trực tiếp trên dữ liệu thật của ứng dụng, đúng như khi chạy `dev`. Chỉ các bản build `dev -automation` mới tích hợp tính năng này, và chỉ các công cụ trên cùng máy này nắm giữ token trong `.nsgo/automation.yaml` (tệp mà chỉ bạn có quyền đọc) mới kết nối được.

### Công cụ MCP do ứng dụng định nghĩa

Gọi `automation.Register` từ ứng dụng hoặc thư viện Go trên goroutine của engine. Ví dụ, hàm dưới đây cung cấp cho agent ID của màn hình (scene) đang hiển thị:

```go
import (
    "context"
    "encoding/json"

    "github.com/nsgo-dev/nsgo/automation"
    "github.com/nsgo-dev/nsgo/core"
)

func registerSceneTool(cx *core.Scope, sceneID *core.Signal[string]) error {
    return automation.Register(cx, automation.Tool{
        Name:        "current_scene",
        Description: "Read the ID of the active scene",
        InputSchema: json.RawMessage(`{"type":"object"}`),
        Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
            return map[string]string{"id": sceneID.Get()}, nil
        },
    })
}
```

Tên công cụ phải là duy nhất trên đồ thị của ứng dụng và khác với các tên tích hợp sẵn. `nsgo mcp` liệt kê công cụ này cho tới khi scope bị dọn dẹp. Hàm `Handler` chạy trên goroutine của engine nên có thể đọc hoặc gán giá trị cho signal. Nó nhận vào một đối tượng JSON, kiểm tra các đối số cần thiết và trả về một giá trị có thể mã hóa JSON hoặc một lỗi.

Trong bài kiểm thử, hãy giữ lại scope khi ứng dụng khởi động, gọi `automation.Call(cx, ctx, name, args)` bên trong `app.Post`, rồi gọi `app.Settle()` trước khi kiểm tra kết quả hoặc view. Việc đăng ký chỉ có hiệu lực trong `dev -automation` và các bài kiểm thử, và sẽ không làm gì (no-op) trong bản build phát hành.

## Các cờ dùng chung cho mọi bản build

Mọi bản build của ứng dụng, kể cả các gói phát hành, đều nhận các cờ riêng của NSGo khi khởi động; các cờ này ghi đè `nsgo.yaml` cũng như mã của ứng dụng và không bao giờ lọt xuống các cờ riêng của ứng dụng:

| Cờ | Tác dụng |
| --- | --- |
| `--nsgo-fps` | Hiện bộ theo dõi hiệu năng trên macOS, iOS, Windows, Linux và Android |
| `--nsgo-lang=zh-Hans` | Khởi động ứng dụng với một ngôn ngữ (locale), bao gồm cả ngôn ngữ giả |
| `--nsgo-appearance=dark` | Khởi động ứng dụng ở chế độ `light`, `dark` hoặc theo `system` |
| `--nsgo-os=ios` | Chạy bản build web như `macos`, `windows`, `linux`, `ios` hoặc `android` (riêng `web` là chính nó) |

```sh
./Hello --nsgo-fps                                   # Windows and Linux
open Hello.app --args --nsgo-fps                     # macOS
xcrun simctl launch booted dev.example.hello --nsgo-fps
adb shell am start -n dev.example.hello/dev.oneo.nsgo.NsgoActivity --ez nsgo-fps true --es nsgo-lang de
```

Trên web, các cờ được đặt trong query string của trang: `?nsgo-fps&nsgo-lang=de`.

`--nsgo-os` xem trước bố cục của một nền tảng khác ngay trên trình duyệt: ứng dụng đọc hệ điều hành và kiểu thiết bị (idiom) tương ứng — điện thoại đối với iOS và Android, máy tính để bàn đối với các nền tảng còn lại — và `Env.Simulated` trả về `true`. Trình duyệt không có các thành phần gốc của nền tảng đó: `NavHeader` và thanh tab tự vẽ sẽ thay thế cho thành phần hệ thống, không có trình đơn ứng dụng, không có trình đơn bật lên gốc (`desktop.PopupMenu`) và các hàm cửa sổ, đồng thời `mobile.IsAvailable` và `desktop.IsAvailable` trả về `false`. Các trình đơn tự vẽ như `ui.ContextMenu` và `ui.Menu` vẫn hoạt động. Phím tắt tuân theo bàn phím của người truy cập. Các bản build không phải web sẽ bỏ qua cờ này và ghi một cảnh báo vào nhật ký.

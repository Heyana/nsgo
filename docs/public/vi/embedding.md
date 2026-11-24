---
title: Nhúng ứng dụng
description: Đặt một ứng dụng NSGo vào bên trong ứng dụng iOS, Android, máy tính để bàn hoặc web của riêng bạn.
group: Bộ công cụ
order: 23
---

## Build một ứng dụng có thể nhúng

Một ứng dụng có thể nhúng là một gói ứng dụng thông thường có `Root` và `main`, được build với cờ `-embed` cho nền tảng đích của ứng dụng chủ. Mỗi view chạy ứng dụng một lần và là một engine độc lập, nên ứng dụng chủ có thể hiển thị bao nhiêu view tùy thích, mỗi view giữ trạng thái riêng.

```sh
go tool nsgo build -embed -target ios examples/embedded
go tool nsgo build -embed -target android_arm64 examples/embedded
go tool nsgo build -target web examples/embedded
```

Bản build iOS xuất ra `<Name>.xcframework` và `<Name>Resources`; bản build Android xuất ra `<Name>.aar`; bản build web xuất ra một thư mục chứa `nsgo.json` và trình nạp (loader).

## iOS

Khi ứng dụng nhập `base/terminal`, bản build cũng xuất ra `GhosttyVT.xcframework` nằm cạnh framework của ứng dụng. Hãy thêm framework đi kèm này vào target chủ với chế độ **Embed & Sign**, áp dụng cho cả framework ứng dụng tĩnh lẫn `-dynamic`. Nó chứa thư viện cho thiết bị thật, trình giả lập Intel và Apple silicon, cùng Mac Catalyst; giữ nguyên tên `GhosttyVT.framework` của nó.

Thêm framework của ứng dụng vào target ứng dụng dưới dạng thư viện tĩnh (không nhúng — do not embed) và thêm thư mục tài nguyên dưới dạng tham chiếu thư mục (folder reference), sau đó `import <Name>`. Gọi `NSGoStart()` một lần khi ứng dụng khởi chạy, trước khi tạo view đầu tiên. Mã nguồn của thư viện tĩnh trở thành một phần của ứng dụng chủ, nên privacy manifest của ứng dụng chủ phải khai báo các API có lý do bắt buộc (required-reason APIs) mà NSGo sử dụng: user defaults (`CA92.1`), dấu thời gian tệp (`C617.1`) và thời gian khởi động hệ thống (`35F9.1`); một framework `-dynamic` đã mang sẵn chúng trong tệp `PrivacyInfo.xcprivacy` riêng của nó.

```swift
import Embedded
import UIKit

@main
final class AppDelegate: UIResponder, UIApplicationDelegate {
	func application(_ app: UIApplication, didFinishLaunchingWithOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
		NSGoStart()
		return true
	}
}

let view = NSGoView(frame: .zero)
view.params = ["user": "Ada"]              // cx.HostParams()
view.dataDirectory = documents.path        // where storage lives
view.localeIdentifier = "de-CH"            // in place of the system's
view.setFontScale(1.3)
view.overrideUserInterfaceStyle = .dark
view.tokens = ["accent": "#cf222e"]        // over the app's theme
view.handle("time") { _, reply in reply(Date().description, nil) }
view.send("message", ["text": "Hello"])    // cx.OnHostEvent("message", …)
view.onOpenURL = { url in router.open(url) }
view.onError = { message in log(message) }
view.register(component: "host.badge") { context in Badge(context) }
```

`NSGoView` khởi động ứng dụng ngay khi được gắn vào một cửa sổ và dừng lại khi bị giải phóng bộ nhớ hoặc nhận lệnh `stop()`. Hãy thiết lập các giá trị khởi chạy trước khi view vào cửa sổ; các thuộc tính còn lại có thể thay đổi bất cứ lúc nào. Các handler, thành phần và callback đều chạy trên luồng chính, còn câu trả lời (`reply`) có thể gửi lại sau từ bất kỳ luồng nào. Một view controller có thể chuyển tiếp `preferredStatusBarStyle`, `prefersStatusBarHidden` và `prefersHomeIndicatorAutoHidden` của view để tuân theo các thiết lập thanh hệ thống của nó.
Ứng dụng chủ cũng có thể trả về `preferredScreenEdgesDeferringSystemGestures` của view từ controller của mình để chuyển tiếp `mobile.DeferSystemGestures`.

## Android

Đối với ứng dụng arm64 nhập `base/terminal`, tệp AAR có sẵn `jni/arm64-v8a/libghostty-vt.so` bên cạnh `libnsgo.so`; ứng dụng chủ cần giữ chế độ giải nén thư viện gốc được bật. Bản prebuild Android và trình nạp terminal yêu cầu [#1452](https://github.com/nsgo-dev/nsgo/issues/1452); các thư viện terminal cho x86_64 hiện chưa khả dụng.

Thêm tệp AAR vào module ứng dụng và cấu hình đóng gói giải nén thư viện gốc để runtime tìm thấy chúng:

```kotlin
implementation(files("libs/Embedded.aar"))

android { packaging { jniLibs { useLegacyPackaging = true } } }
```

Tạo các `dev.oneo.nsgo.NsgoView` bằng mã hoặc trong tệp bố cục XML. Mỗi view khởi chạy ứng dụng trong một tiến trình riêng ngay khi nhận được kích thước lần đầu, và `stop()` kết thúc nó. Chuyển tiếp kết quả trả về của activity tới mọi view, và tự xử lý thay đổi cấu hình (configuration change) trong activity để các ứng dụng không bị khởi động lại.

```java
NsgoView app = new NsgoView(context);
app.setParams(Map.of("user", "Ada"));
app.setStorageDirectory(dir);
app.setLocale(Locale.forLanguageTag("de-CH"));
app.setFontScale(1.3f);
app.setAppearance(NsgoView.APPEARANCE_DARK);
app.setTokens(Map.of("accent", "#cf222e"), null);
app.handle("time", (data, reply) -> reply.resolve(now()));
app.handle("tapped", (data, reply) -> status.setText("tapped " + ((Map<?, ?>) data).get("count")));
app.setOpenUrlHandler(url -> router.open(url));
app.setErrorListener(message -> log(message));
app.registerComponent("host.badge", Badge::new);
```

```java
@Override
protected void onActivityResult(int request, int result, Intent data) {
	super.onActivityResult(request, result, data);
	for (NsgoView app : apps) app.activityResult(request, result, data);
}
```

Để tách cây giao diện gốc ra trong khi vẫn giữ ứng dụng Go sống, hãy giữ một `NsgoEngine` phía ứng dụng chủ và gắn view vào đó một cách tường minh:

```java
NsgoEngine engine = new NsgoEngine(context.getApplicationContext());
app.attachToEngine(engine);
// Before taking the view off screen:
app.detachFromEngine();
// Reuse the same view later:
app.attachToEngine(engine);
// When the host ends the app:
engine.destroy();
```

Thao tác tách (detach) giải phóng các view con gốc; gắn lại sẽ khôi phục các signal hiện tại, dữ liệu nhập, tiêu điểm, vị trí cuộn và bản vẽ mà không chạy lại các thành phần. Các trang web và trình phát media được tạo lại từ cấu hình ứng dụng của chúng. `stop()` chỉ tách một engine được truyền vào tường minh. Mỗi engine vẫn gắn liền với view đầu tiên và Activity của nó và từ chối view khác, vì vậy hãy tự xử lý thay đổi cấu hình trong Activity đó; việc thay thế Activity hoặc hiển thị nhiều view độc lập trong cùng một tiến trình hiện chưa được hỗ trợ.

Để kiểm tra cập nhật qua Google Play, hãy thêm `com.google.android.play:app-update:2.1.0` vào các thư viện phụ thuộc Gradle của ứng dụng chủ Android. Giữ nguyên các API `AppUpdateManager`, `AppUpdateInfo`, `AppUpdateOptions` và `Tasks` mà NSGo tải theo tên bằng [các quy tắc shrink runtime](https://github.com/nsgo-dev/nsgo/blob/main/internal/platform/android/runtime/play-review.pro). Việc chuyển tiếp `onActivityResult` ở trên cũng chuyển tiếp cả sự kiện hủy cập nhật.

Đối với các phiên xác thực trên Android trong ứng dụng chủ nhúng, hãy khai báo callback scheme riêng trên Activity chủ và chuyển tiếp các liên kết mà Activity đó nhận được sang `view.openLink(uri.toString())` trong `onNewIntent` (và trong `onCreate` mới). Phiên đang hoạt động khớp với liên kết sẽ tiêu thụ callback đó; các liên kết khác đi tới `OnOpenLinks`. `NsgoActivity` độc lập tự động chuyển tiếp các liên kết này.

## Máy tính để bàn

Một ứng dụng chủ viết bằng Go chạy vòng lặp sự kiện của nền tảng và đặt `Handle` của mỗi view vào cửa sổ của nó. `nsgo.NewView` khởi chạy ứng dụng ngay khi handle nằm trong một cửa sổ, vì vậy các thiết lập thực hiện trước đó sẽ có mặt ngay từ khung hình đầu tiên.

```go
channel := core.NewHost()
channel.SetParams(map[string]any{"user": "Ada"})
channel.Handle("time", func(c *core.HostCall) { c.Resolve(time.Now().Format("15:04:05")) })
channel.HandleOpenURL(func(url string, answer func(bool)) {
	answer(strings.HasPrefix(url, "https://example.com/orders/"))
})

view := nsgo.NewView(app.Root, nsgo.HostChannel(channel), nsgo.DataDir(dataDir))
defer view.Close()

view.SetTokens(core.Tokens{Values: map[string]string{"accent": "#cf222e"}})
view.SetLocale("de-CH")
view.SetFontScale(1.3)
view.SetAppearance(core.AppearanceDark)
handle := view.Handle() // an NSView, GtkWidget or child window
```

`view.Host()` gửi sự kiện tới ứng dụng và nhận các lời gọi từ nó, còn `nsgo.Component` đăng ký một thành phần gốc cho nền tảng của ứng dụng chủ.

## Web

Bản build web vốn là một trang riêng, nhưng một trang khác có thể gắn (mount) nó vào một phần tử HTML và đóng vai trò ứng dụng chủ của nó. `mount(element, url, options)` trả về một `player`, và `nsgo.json` chỉ định đường dẫn trình nạp, tệp wasm và tài nguyên tính tương đối theo chính nó.

```html
<div id="app"></div>
<script type="module">
const url = new URL("./Embedded-0.1.0-web/nsgo.json", location.href);
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url));

const player = await mount(document.getElementById("app"), url.href, {
	config,
	params: { user: "Ada" },
	host: {
		call(name) {
			if (name === "time") return new Date().toLocaleTimeString();
			throw new Error(`no host function ${name}`);
		},
		post(name, data) { console.log(name, data); },
		openURL(url) { return url.startsWith("https://example.com/orders/"); },
	},
});
player.hostEvent("message", { text: "Hello from the page" });
</script>
```

Nếu không truyền tùy chọn `host`, trình nạp sẽ nhập module được chỉ định bởi `web.host` trong `nsgo.yaml` và `await` hàm `createHost(player)` nếu module đó có xuất hàm này. Một ứng dụng nằm trong một phần tử hoặc một frame sẽ nhường quyền quản lý lịch sử và tiêu đề trình duyệt lại cho trang chủ.

`appearance` (`"light"`, `"dark"` hoặc `"system"`), `locale` và `fontScale` thay thế cho các thiết lập của trình duyệt giống như thiết lập của ứng dụng chủ gốc, và các phương thức `setAppearance`, `setLocale` và `setFontScale` của `player` cho phép đổi lại chúng về sau. `module`, một `WebAssembly.Module` đã biên dịch hoặc một promise trả về nó, cho phép một trang gắn ứng dụng nhiều lần chỉ cần tải về và biên dịch đúng một lần.

Trình nạp hiển thị một phần tử `progress.nsgo-load-progress` trước khi ứng dụng khởi động. Bạn có thể tạo kiểu cho nó bằng CSS, truyền `onProgress` vào các tùy chọn của `mount`, hoặc xuất `onProgress` từ `web.host` cho trang riêng của ứng dụng. Callback này thay thế thanh tiến trình mặc định và nhận `{ loaded, total }` tính bằng số byte đã giải mã, với `total` là `null` khi chưa biết kích thước. Trình nạp gọi callback với `null` khi quá trình khởi động kết thúc để có thể dọn giao diện tải. Truyền một `module` đã biên dịch sẵn sẽ bỏ qua tiến trình tải xuống.

```yaml
web:
  host: host.js
```

## Kênh giao tiếp với ứng dụng chủ

`core.Host` là giao ước duy nhất mà mọi ứng dụng chủ đều triển khai: tham số khởi chạy, sự kiện ứng dụng chủ gửi vào, và các hàm của ứng dụng chủ mà ứng dụng gọi ra. Giá trị truyền qua theo quy tắc của JSON, nên các map trở thành `map[string]any`, số trở thành `float64`, và cùng một đoạn mã hoạt động đồng nhất trên Swift, Java, Go và JavaScript.

- `HostParams` đọc các tham số khởi chạy một lần, lúc bắt đầu.
- `cx.OnHostEvent` lắng nghe `Host.Send` và `hostEvent` của trang web.
- `cx.CallHost` gọi một handler theo tên và hoàn thành với câu trả lời của nó; `cx.PostHost` gọi một handler mà không cần câu trả lời.
- `Host.HandleOpenURL` nhận `cx.OpenURL` trước nền tảng, và câu trả lời của nó quyết định nền tảng có mở tiếp URL đó hay không.
- `Host.SetParams`, `Send`, `Handle` và `HandleDefault` an toàn khi gọi từ bất kỳ goroutine nào.

Trên iOS, ứng dụng chủ gán `params`, cài handler bằng `handle`, gửi sự kiện bằng `send` và trả lời `onOpenURL`. Trên Android, nó làm tương tự với `setParams`, `handle`, `send` và `setOpenUrlHandler`. Trên máy tính để bàn, nó dùng các phương thức của `core.Host`, còn trên web thì đối tượng `host` của trang sẽ trả lời.

## Thành phần gốc

`core.Native` hiển thị một view do ứng dụng chủ tạo ra: `Props` của nó đi tới `SetProp` của thành phần chủ, những gì thành phần phát ra đi tới `OnEvent`, và một prop có giá trị là nút hoạt ảnh (animated node) sẽ chuyển động theo đồng hồ khung hình mà không tốn công sức cho engine.

```go
core.Native{
	Component: "host.badge",
	PropsFunc: func() map[string]any { return map[string]any{"count": count.Get()} },
	OnEvent:   func(name string, data any) { if name == "tap" { count.Set(0) } },
	Style:     core.Style{MinWidth: core.Pt(120), MinHeight: core.Pt(28)},
}
```

Đăng ký thành phần trong ứng dụng chủ: `app.register(component:)` và một `NSGoComponent` trên iOS, `registerComponent` và một `NsgoComponent` trên Android, `nsgo.Component` trên máy tính để bàn. Web không có thành phần gốc, nên một tên không xác định sẽ hiển thị một hộp trống ở đó.

## Thiết lập

`tokens` ghi đè chủ đề của ứng dụng theo tên vai trò (`accent`, `inset`, `raised`), ở cả hai giao diện sáng/tối hoặc chỉ một giao diện; `cx.Palette()` sẽ đọc được chúng. Ứng dụng chủ cũng đặt ngôn ngữ, cỡ chữ, giao diện sáng/tối và thư mục dữ liệu, đồng thời nhận thông báo lỗi của ứng dụng. `nsgo build -embed` không tạo cửa sổ, trình đơn hay biểu tượng khay hệ thống: những thứ đó thuộc quyền quản lý của ứng dụng chủ.

Một lỗi panic nội bộ trên goroutine của engine của một view sẽ dừng engine đó, hủy các bề mặt đang gắn vào nó và báo cáo panic kèm stack trace cho trình lắng nghe lỗi của ứng dụng chủ (`nsgo.OnError` trong Go). Các engine khác vẫn tiếp tục chạy; hãy tạo một view mới để khởi động lại ứng dụng bị lỗi. Các panic thông thường trong callback của ứng dụng vẫn đi tới `core.Catch` gần nhất trước tiên.

`examples/embed-ios`, `examples/embed-android`, `examples/embed-desktop` và `examples/embed-web` là cùng một ứng dụng đặt trong từng loại ứng dụng chủ.

## Tiếp theo

- [Dịch vụ nền tảng](services.md): những gì ứng dụng yêu cầu từ nền tảng.
- [Đóng gói và phát hành](building.md): các gói mà `-embed` tạo ra, và cách ký chúng.
- [Ví dụ: Ứng dụng việc cần làm](tutorials/todo.md): ứng dụng mà các ứng dụng chủ ở trên nhúng vào.

---
title: Xây dựng và đóng gói phát hành
description: Đóng gói ứng dụng cho mọi nền tảng, tạo trình cài đặt, ký số và công chứng, phát hành trên GitHub và nhúng vào các ứng dụng khác.
group: Bộ công cụ
order: 22
---

## Xây dựng

```sh
go tool nsgo build
```

`build` tạo gói phát hành đã lược bỏ thông tin gỡ lỗi (stripped) cho từng nền tảng đích được liệt kê trong `nsgo.yaml`, đặt vào `dist/<name>-<version>-<target>`: tệp `.app`, `.ipa`, `.apk`, thư mục cho Windows và Linux, hoặc một trang web mà trang HTML của nó sẽ tải từng module TSX được `import()` chỉ định khi lệnh import chạy. `-target` thay thế danh sách đích, và `all` có nghĩa là mọi nền tảng chỉ cần Go và Xcode; hãy thêm `android_x64` khi đã cài Android NDK, và `miniprogram`, hoặc riêng `miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` hay `miniprogram_douyin`, cho [mini program](#mini-program), cũng như `harmonyos` cho bản xem trước [HarmonyOS](#harmonyos). `-run` khởi chạy gói đã xây dựng cho máy hiện tại, hoặc cài đặt lên iPhone, trình giả lập hay thiết bị Android được chỉ định bởi `-device`.

Mọi tệp dùng để phân phối đều nằm trong `dist/releases`, thư mục duy nhất bạn cần quan tâm:

| Trong `dist/releases` | Nội dung |
| --- | --- |
| `*.dmg`, `*-setup.exe`, `*.deb`, `*.pkg.tar.gz`, `*.AppImage` | [Trình cài đặt](#trình-cài-đặt) cho macOS, Windows và Linux |
| `*.AppImage.zsync` | Tệp điều khiển cập nhật sai phân cho AppImageUpdate và các trình khởi chạy tương thích |
| `*-ios.ipa`, `*-tvos.ipa`, `*.apk` | Gói iOS, tvOS và Android, dùng cho kho ứng dụng hoặc cài trực tiếp lên thiết bị |
| `*.pkg`, `*.msix`, `*.aab`, `*.flatpak`, `*.tar.gz` và `flatpak/` | Các [gói kho ứng dụng](#gói-kho-ứng-dụng) của bản build `store` |
| `install.sh`, `install.ps1` | Tập lệnh cài đặt ứng dụng từ [bản phát hành GitHub](#phát-hành-trên-github) |
| `appcast-<target>.xml` | Nguồn cấp dữ liệu [cập nhật](#cập-nhật) mà ứng dụng đọc |
| `homebrew/<name>.rb`, `winget/` | Các tệp kê khai [Homebrew cask và winget](#kênh-phân-phối) cho kênh phát hành trực tiếp |
| `*-web.zip` | Trang web, chỉ cần giải nén và phục vụ nguyên trạng |
| `miniprogram/<platform>/` | Dự án cho từng mini program, để tải lên từ công cụ phát triển của nền tảng đó |

Mỗi tệp được đặt tên theo mẫu `<name>-<version>-<system>-<arch>`, chẳng hạn `hello-1.2.0-windows-x64-setup.exe`, để người dùng dễ tìm bản phù hợp trên trang phát hành, và có tệp `.sha256` đi kèm bên cạnh. Khi xây dựng một phiên bản khác, thư mục này sẽ được dọn sạch trước; các lần xây dựng cùng một phiên bản, kể cả trên các máy khác nhau, sẽ bổ sung vào thư mục đó. Gói của trình giả lập và watchOS vẫn nằm lại trong các thư mục `dist/<name>-<version>-<target>` của chúng.

Trang web bao gồm `.wasm.br` và `.wasm.gz` khi chúng nhỏ hơn module gốc. Hãy bật phục vụ tệp nén sẵn trên các máy chủ hỗ trợ tính năng này. Phục vụ chúng tại URL `.wasm` gốc với `Content-Type: application/wasm`, `Content-Encoding` tương ứng và `Vary: Accept-Encoding`. Module gốc hoạt động trên mọi máy chủ tĩnh. [Xuất bản trên web](web.md) đề cập đến tiêu đề ngữ nghĩa, liên kết trình duyệt, siêu dữ liệu theo trang và tải qua CDN.

### Gói web ngoại tuyến

Đặt `web.offline: true` trong `nsgo.yaml` để lưu đệm bản phát hành web ngay trong lần khởi chạy trực tuyến đầu tiên.

```yaml
web:
  offline: true
```

Hãy phục vụ thư mục đã xuất bản qua HTTPS, hoặc dùng `go tool nsgo build -target web -run` trên localhost. Service worker sẽ lưu đệm trang web, WebAssembly, tập lệnh và các tài nguyên đóng gói kèm. Khi truy cập trực tuyến, trình duyệt tải phiên bản đã xuất bản. Sau khi cài đặt thành công và đóng các tab của bản phát hành trước đó, bản phát hành mới nhất có thể khởi chạy từ bộ đệm. Các yêu cầu tới dịch vụ trực tuyến của ứng dụng vẫn cần có kết nối mạng.

## Thiết bị đầu cuối

Chạy ứng dụng trong một thiết bị đầu cuối tương tác bằng `go tool nsgo dev -target tui`. `go tool nsgo test -target tui` chạy các bài kiểm thử tại đó thông qua `nsgotest.Main`, mỗi tiến trình kiểm thử chạy một ứng dụng. Thiết bị đầu cuối quyết định kích thước: một ô ký tự đại diện cho 8 nhân 16 điểm bố cục. Hãy chọn kiểu dáng cho ứng dụng một cách tường minh; các thành phần vẫn giữ nguyên hình thức và kích thước thông thường của chúng. `examples/tui` minh họa các trường nhập liệu, nút bấm, phím tắt và thay đổi kích thước.

Xây dựng tệp thực thi console với `go tool nsgo build -target tui_macos_arm64`, hoặc các đích tương ứng `tui_macos_x64`, `tui_windows_arm64`, `tui_windows_x64`, `tui_linux_arm64` hay `tui_linux_x64`. Nhóm xây dựng/kiểm tra `tui` bao gồm cả sáu đích này, và `all` cũng bao gồm chúng. Mỗi thư mục gói chứa tệp thực thi và tài nguyên; không có trình cài đặt GUI. Cả sáu đều xây dựng chỉ bằng Go.

Backend hỗ trợ văn bản theo ô, màu nền, viền nét liền, cắt vùng (clipping), cuộn, tiêu điểm và chỉnh sửa văn bản. Nó dùng phông chữ và màu sắc của thiết bị đầu cuối; phím Control đóng vai trò `mod`, kể cả trên máy Mac. Ctrl+C sao chép khi đang trong trình soạn thảo và thoát ứng dụng khi không có phím tắt hoặc trình soạn thảo nào đón nhận. Thao tác dán có ngoặc (bracketed paste) chèn văn bản mà không kích hoạt phím tắt, và khay nhớ tạm dùng cục bộ trong tiến trình. Canvas, hình ảnh, phương tiện truyền thông, thành phần nhúng gốc, menu/cảnh báo hệ thống, hoạt ảnh, thu phóng, định hình văn bản hai chiều, rút gọn dấu ba chấm đầu/giữa và cầu nối với khay nhớ tạm hệ thống hiện chưa được triển khai. `base/terminal` vẫn là điều khiển thiết bị đầu cuối được nhúng bên trong một ứng dụng GUI.

Trong khi `nsgo dev` chiếm quyền điều khiển thiết bị đầu cuối, nhật ký chẩn đoán của công cụ và ứng dụng được ghi vào `.nsgo/tui.log`. Khi thoát, công cụ khôi phục lại màn hình, con trỏ và các chế độ thiết bị đầu cuối trước khi trả về shell. Ảnh chụp màn hình trong kiểm thử thiết bị đầu cuối kết xuất các ô bằng một phông chữ cố định mang tính di động thay vì chụp trình giả lập; hãy dùng các khẳng định bằng văn bản đối với những ký tự Unicode không có trong phông chữ đó.

## nsgo.yaml

```yaml
name: Hello
id: dev.example.hello   # the bundle ID; without it, dev.nsgo. and the name's letters and digits
version: 0.1.0
build_number: 1         # CFBundleVersion and Android's versionCode; else from version
# copyright: © 2026 Example  # the Windows executable's details and installer, and the macOS About panel
# appearance: dark      # light or dark whatever the system's
# theme: theme.json     # a TSX app's theme: NSGo's or a shadcn/ui one
# i18n: { dir: locales, source: en }  # the app's catalogs and the language they are written in
icon:
  apple: assets/AppIcon.icon  # macOS, iOS and watchOS, or an .icns for macOS
  windows: assets/IconWindows.png  # optional Windows artwork
  linux: assets/IconLinux.png      # optional Linux artwork
  png: assets/Icon.png        # every other target
  tvos: assets/AppIconTV.brandassets  # the Apple TV's layered icon and top shelf art, or a wide PNG
  watchos: assets/AppIconWatch.icon   # the Apple Watch's own, drawn in circles, or a square PNG
  android:
    png: assets/IconAndroid.png  # optional bitmap; valid without adaptive layers
    foreground: assets/IconForeground.png  # Android's adaptive icon: 432 px, the logo within the center 264 px
    background: "#1F1F1F"                 # a color or a square PNG
    monochrome: assets/IconMonochrome.png  # optional: the shape Android 13's themed icons tint
dev:
  target: native        # native, web, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android, harmonyos or miniprogram_<platform>
  hot_reload: true      # false builds once, watching no files
  # icon: { png: assets/Icon-dev.png }  # replaces icon's files in dev's builds; defaults to assets/*-dev.*
build:
  output: dist
  tags: []              # Go build tags every build, test and check of the app takes, such as timetzdata
  ldflags: []           # the app's own linker flags, such as -X main.releaseDate=2026-11-17
  open: false           # true opens the output directory after building
  installers: true      # false leaves out the disk image, the Windows installer and the Linux packages
  jobs: 0               # how many targets build at once; 0 is half the CPUs, 1 one after another
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android_arm64, web]
# apple: { sign_in_with_apple: true }  # the Sign in with Apple entitlement, for iOS, tvOS and Mac App Store builds
# apple: { purchases: true }  # StoreKit's library, which in-app purchases go through, in macOS, iOS and tvOS packages
# apple: { privacy: { accessed_apis: { NSPrivacyAccessedAPICategoryDiskSpace: [E174.1] }, tracking: false } }  # add required-reason APIs and tracking declarations to PrivacyInfo.xcprivacy
# apple: { requires_full_screen: false }  # let an iPad run the app in Split View, Slide Over and Stage Manager, in all four orientations
# apple: { orientations: [portrait, landscape_left] }  # the orientations a full-screen app takes on an iPad
# apple: { multiple_scenes: true }  # let an iPad open more than one window of the app, each showing the main view with its own state
# apple: { info: { UIBackgroundModes: [audio] } }  # background playback for iOS Picture in Picture
# android: { permissions: [REQUEST_INSTALL_PACKAGES] }  # manifest permissions beyond the runtime's
# android: { recording: true }  # the microphone service microphone.RecordWAV records in, with RECORD_AUDIO, the foreground-service permissions and POST_NOTIFICATIONS, which RecordWAV asks for so its notification shows from Android 13; a refusal still records
# android: { uses_cleartext_traffic: true }  # allow plain HTTP to any host in the APK and App Bundle
# android: { background_downloads: true }  # dataSync foreground service for cx.Download
# android: { background_motion: true }  # explicit finite motion shortService; streams also need a notification title
# android: { loopback_cleartext: true }  # allow plain HTTP to loopback in the APK and App Bundle
# commands: [{ name: hello-cli, package: ./cmd/hello-cli }]  # command-line programs the desktop packages ship beside the app: see Commands
# windows: { command_line: true }  # add <name>.com, a console companion that forwards arguments and streams to <name>.exe; not in store builds
# windows: { path: true }  # the installer puts the app's folder on the user's Path, so consoles find its commands
# windows: { publisher: Example Ltd }  # the company the Windows executable's details, its installer and the uninstall entry name; else windows.store.publisher_name
# linux: { maintainer: "Jane Doe <jane@example.com>" }  # the Linux packages' maintainer, else the app's name
# linux: { categories: [Office, Dictionary] }  # the freedesktop menu categories of the app's desktop entry, at least one a main category such as Office, Utility or Game, else menus file it under Other; your own start with X-
# linux: { share: packaging/share }  # files the .deb and Arch package install under /usr/share and the AppImage in its usr/share, such as metainfo/<id>.metainfo.xml or bash-completion/completions/<command>
# linux: { depends: { deb: [gstreamer1.0-plugins-good], arch: [gst-plugins-good] }, recommends: { deb: [gstreamer1.0-libav], arch: ["gst-libav: H.264 and AAC video"] } }  # packages the .deb and Arch package also depend on and recommend, after GTK 4 and WebKitGTK 6, each as dpkg or pacman writes it
# release: { github: me/hello }  # the GitHub repository the install scripts and updates install from, else the Git remote origin's; tag_prefix: v by default
# updates: { key: <public key> }  # how the app updates, by platform and channel: see Updates
# store: { dir: store }           # store listings directory (YAML and Markdown per language)
# miniprogram: { appid: { wechat: wx0123456789abcdef, alipay: "2021000000000000" }, custom_navigation: false }  # the mini programs'
```

Một tên hệ điều hành trần trong `targets` — hoặc trong `nsgo build -target` và `nsgo check -target` — đại diện cho các kiến trúc của nó (`windows` là `windows_arm64` và `windows_x64`; `android` chỉ gồm `android_arm64`, vì bản dựng x86_64 cần có NDK). `macos_universal` (vốn không nằm trong `macos` và `all`) xây dựng cả hai kiến trúc macOS và gộp vào một ứng dụng duy nhất, bao gồm cả các thư viện và tiện ích mở rộng của nó. Kênh Steam sử dụng gói ứng dụng hợp nhất (universal) này. Các bản dựng trình cài đặt gói nó vào một ảnh đĩa duy nhất, kèm một nguồn cấp Sparkle duy nhất trên kênh phát hành trực tiếp. `nsgo test -target` cũng nhận tên kiến trúc mà bài kiểm thử chạy: với hệ điều hành máy tính để bàn thì theo kiến trúc của máy hiện tại, còn với Android thì trên `arm64`, chẳng hạn `windows_x64` chạy đích `windows` trên máy Windows x64, trong khi một kiến trúc không phù hợp sẽ bị từ chối kèm gợi ý tên đúng cần dùng.

Các cờ dòng lệnh sẽ thay thế các giá trị cùng tên, chẳng hạn `-target`, `-version` hoặc `-output`; `-h` liệt kê tất cả.

`build.ldflags` lưu giữ các cờ trình liên kết (linker) riêng của ứng dụng, viết đúng theo cú pháp `-ldflags` của go build, ví dụ `-X main.releaseDate=2026-11-17` hoặc `-X 'main.channel=beta builds'`. Mọi lệnh build có liên kết ứng dụng đều nhận các cờ này: `dev` và các lần tải lại, `build` kể cả với `-embed`, và `test` kể cả với `-target`, trên mọi nền tảng đích. Cờ `-ldflags` truyền vào `build` và `test` sẽ nối thêm vào danh sách này thay vì thay thế, có thể lặp lại để truyền thêm. Chúng được đặt trước các cờ `-X` riêng của công cụ, và trình liên kết giữ lại giá trị cuối cùng của một biến, nhờ vậy tên, ID và phiên bản trong `core.App` luôn giữ nguyên theo `nsgo.yaml`. Công cụ truyền cho Go cờ `-ldflags` riêng của mình, ghi đè lên giá trị trong `GOFLAGS`, nên các cờ trình liên kết cần được khai báo tại đây; cờ `-ldflags` đứng sau thư mục trong lệnh `test` không giao diện là cờ của `go test` và sẽ thay thế toàn bộ.

Mọi lệnh Go mà công cụ chạy cho ứng dụng đều nhận `build.tags`, trong `dev` và các lần tải lại, `build` kể cả với `-embed`, `test` kể cả với `-target`, và `check`, trên mọi nền tảng đích. Các thẻ riêng của NSGo như `nsgodev` trong `dev` hay `nsgotest` trong kiểm thử theo nền tảng đích sẽ được thêm vào, cùng với các thẻ của `-tags` trong `GOFLAGS` (vốn sẽ bị ghi đè nếu không giữ lại); phần còn lại của `GOFLAGS` vẫn giữ nguyên hiệu lực.

`apple.info.LSMinimumSystemVersion` nâng phiên bản hệ điều hành tối thiểu của gói macOS, còn `apple.info.MinimumOSVersion` nâng phiên bản tối thiểu của gói iOS, tvOS hoặc watchOS được chọn. Hãy viết chuỗi phiên bản trong dấu ngoặc kép, như `"14.2"` hoặc `"18.0"`, gồm một đến ba phần số. Mức sàn của runtime là macOS 13.0 (Go 1.27), iOS 15.0, tvOS 17.0 và watchOS 26.0; giá trị thấp hơn hoặc sai định dạng sẽ gây lỗi. Khóa dành cho di động phải đáp ứng mức sàn của mọi nền tảng di động được chọn. App Clip giữ riêng `app_clip.minimum`. Các giá trị này đi vào siêu dữ liệu gói, quá trình biên dịch tài nguyên, dự án ký số và trình liên kết của ứng dụng; các framework iOS nhúng cũng dùng chúng, với Mac Catalyst tối thiểu 16.0 cho runtime macOS 13 của Go.

`icon.windows` và `icon.linux` chọn các tệp PNG hình vuông cho gói ứng dụng, hình ảnh trình cài đặt và biểu trưng trên kho ứng dụng. Mặc định chúng trỏ tới `assets/IconWindows.png` và `assets/IconLinux.png`, sau đó lùi về `icon.png`; `dev.icon` nhận các thiết lập tương tự cùng các tệp mặc định `-dev`. `icon.android.png` chọn riêng ảnh bitmap cho Android, kể cả khi không cấu hình các lớp thích ứng (adaptive layers). Các nền tảng đích khác vẫn giữ cơ chế lùi về `icon.png` dùng chung.

Biểu tượng của Apple TV là một thẻ phân lớp chứ không phải hình vuông: `tvos` nhận tệp `.brandassets` của Xcode với các lớp `App Icon` và ảnh top shelf, hoặc một ảnh ngang như 1280×768 mà công cụ tự căn vừa lên thẻ trên màu viền của nó và đặt chính giữa cho ảnh top shelf. Nếu thiếu cả hai, ảnh PNG sẽ nằm ở độ cao ba phần tư của thẻ. Apple Watch cắt biểu tượng theo hình tròn, nên `watchos` nhận một tệp `.icon` được vẽ cho vòng tròn của watchOS hoặc một ảnh PNG vuông; nếu thiếu, ứng dụng dùng `apple`, rồi tới `png`. Cả hai đều tự động tìm `assets/AppIconTV.*` và `assets/AppIconWatch.*`, giống như cách `apple` và `png` hoạt động.

Trên Android, `icon.android.foreground` và `background` tạo ra biểu tượng trình khởi chạy thích ứng (adaptive icon), cho phép các trình khởi chạy cắt theo hình dạng riêng và tạo hiệu ứng di chuyển thay vì thu nhỏ `png` đặt lên nền sẵn. Mỗi lớp là một hình vuông 108 dp, tương ứng PNG cỡ 432 px; trình khởi chạy hiển thị vùng trung tâm 72 dp (288 px) và cắt theo hình dạng có thể lẹm vào mọi thứ ngoài vòng 66 dp (264 px) ở giữa, nên hãy giữ biểu trưng gọn trong vòng tròn đó và để màu nền phủ kín toàn bộ hình vuông. `background` là một tệp PNG hoặc mã màu như `"#1F1F1F"`. `monochrome` là hình khối chỉ xét kênh alpha để Android 13 trở lên nhuộm màu khi người dùng bật biểu tượng theo chủ đề; nếu thiếu, biểu tượng sẽ giữ nguyên màu gốc tại đó. Gói ứng dụng giữ `icon.android.png` (hoặc `icon.png`) làm biểu tượng bitmap thông thường, vốn sẽ không được trình khởi chạy nào từ Android 9 trở lên (phiên bản cũ nhất NSGo hỗ trợ) ưu tiên hơn biểu tượng thích ứng; nếu thiếu `png`, công cụ tự vẽ nó bằng cách chồng foreground lên background. Khi không có các lớp thích ứng, trình khởi chạy hiển thị ảnh bitmap Android hoặc `png` dùng chung.

`android.uses_cleartext_traffic: true` cho phép APK hoặc App Bundle dùng HTTP thuần tới bất kỳ máy chủ nào, thiết lập cả cờ trong manifest lẫn chính sách bảo mật mạng. Nếu không bật tùy chọn đó, `android.loopback_cleartext: true` chỉ cho phép HTTP thuần tới `localhost`, `127.0.0.1` và `::1`, chẳng hạn khi ứng dụng tự phục vụ video cục bộ. Chính sách chỉ cho phép loopback sẽ chặn kết nối không mã hóa tới các máy chủ khác; tùy chọn cho phép mọi máy chủ được ưu tiên nếu cả hai cùng bật. Đối với AAR nhúng, hãy cấu hình chính sách bảo mật mạng Android ở ứng dụng chủ.

Mọi bản build cũng nhận các cờ riêng của NSGo lúc khởi chạy, chẳng hạn `--nsgo-fps` ([Chạy và gỡ lỗi](debugging.md#các-cờ-dùng-chung-cho-mọi-bản-build)). Trong `nsgo dev`, phím Command/Control+R tải lại từ mọi cửa sổ đang mở và vẫn khả dụng sau khi view chính đã đóng nếu còn cửa sổ khác đang giữ một hàm ứng dụng chạy.

### Màn hình khởi chạy

Thiết lập `launch_screen` để hiển thị màu nền đặc, một ảnh PNG căn giữa hoặc cả hai trên iOS và Android 12 trở lên. Đường dẫn ảnh là tương đối so với `nsgo.yaml`. Ảnh tự co giãn theo tỉ lệ để nằm gọn trong 128 × 128 point hoặc dp. Một ảnh PNG nền trong suốt có cạnh dài nhất từ 512 pixel trở lên sẽ giữ được độ sắc nét trên các màn hình mật độ điểm ảnh cao.

```yaml
launch_screen:
  background: "#F5F1EA"
  image: assets/Launch.png
  dark:
    background: "#201C26"
    image: assets/LaunchDark.png
```

Hệ thống tự chọn giao diện sáng/tối trừ khi `appearance` ghim cố định một chế độ. Bỏ qua bất kỳ trường nào trong `dark` sẽ dùng lại giá trị tương ứng ở ngoài. Bỏ qua màu nền hoặc ảnh sẽ giữ nguyên mặc định của nền tảng. Các ứng dụng nhúng lấy màn hình khởi chạy từ ứng dụng chủ.

## Lớp gốc Android

Liệt kê các lớp Android đã biên dịch sẵn trong `nsgo.yaml` của ứng dụng để dùng một View tùy chỉnh từ `core.Native`.

```yaml
android:
  classes: [java/BadgeView.jar, java/Helpers.dex]
```

Đường dẫn là tương đối so với `nsgo.yaml` trừ khi là đường dẫn tuyệt đối. Mỗi đường dẫn trỏ tới một tệp `.dex` hoặc một tệp JAR chứa `classes.dex`, `classes2.dex` và các phân đoạn tiếp theo ở thư mục gốc do D8 tạo ra. Hãy chuẩn bị các tệp đó bằng D8 với `--min-api 28`. Trên arm64, NSGo đóng gói APK hoặc App Bundle từ các tệp đã biên dịch sẵn này chỉ bằng Go. Phiên phát triển Android tự động build lại khi một tệp trong danh sách thay đổi. Đối với AAR nhúng, quy trình build Android của ứng dụng chủ cung cấp các phần phụ thuộc Java. [Tài liệu D8](https://developer.android.com/tools/d8)

[Ví dụ Android gốc](https://github.com/nsgo-dev/nsgo/tree/main/examples/android-native) đăng ký một lớp có hàm khởi tạo `Context` công khai và cấu hình nó bằng một prop từ Go.

## Trò chơi

Đặt `game: true` trong `nsgo.yaml` cho ứng dụng trò chơi. Android phân loại ứng dụng là trò chơi và áp dụng các tối ưu Game Mode của hệ thống. macOS và iOS kích hoạt Game Mode, macOS mặc định xếp vào danh mục Games, và Linux thêm `Game` vào mục desktop entry. Một giá trị `macos.category` tường minh hoặc khóa Game Mode trong `apple.info` sẽ được ưu tiên.

Đặt `gamepad: true` khi ứng dụng xử lý tay cầm chơi game phần cứng. Android khai báo cả tay cầm và màn hình cảm ứng là tùy chọn. macOS, iOS và tvOS khai báo hồ sơ Extended Gamepad, và tvOS khai báo thêm Micro Gamepad của Siri Remote, với các khóa bộ điều khiển trong `apple.info` được ưu tiên nếu có.

## Thông báo giấy phép

`nsgo build` đặt tệp `THIRD_PARTY_NOTICES.txt` vào thư mục tài nguyên của gói độc lập. Tệp này chứa nội dung giấy phép của các Go module mà nền tảng đích đó sử dụng, Go runtime và Java hoặc JavaScript runtime được NSGo đóng gói kèm. Bản build web đặt tên tệp theo nội dung của nó giống như các tài nguyên khác. Hãy gọi `core.ThirdPartyNotices()` để hiển thị chính văn bản này trong màn hình ghi công trong ứng dụng. Hàm trả về chuỗi rỗng trong quá trình phát triển, kiểm thử và các bản build thư viện nhúng. [Khai báo thư viện gốc](native-libraries.md) tự động nối văn bản giấy phép gốc của chúng vào cùng phần thông báo này.

Theo mặc định, một giấy phép bên thứ ba không xác định hoặc bị thiếu sẽ làm lỗi quá trình build, còn giấy phép copyleft phát hiện được sẽ đưa ra cảnh báo. Các module được liệt kê trong Go workspace (`go.work`) của ứng dụng được coi là mã nguồn bên thứ nhất cùng với module của ứng dụng: mọi tệp giấy phép chúng chứa đều được đưa vào, và chúng không kích hoạt kiểm tra bên thứ ba. Có thể đặt từng quy tắc thành `error` hoặc `warn`. Danh sách `allow` miễn trừ các đường dẫn module khớp chính xác khỏi cả hai quy tắc và vẫn giữ nguyên mọi văn bản giấy phép tìm được.

```yaml
licenses:
  unknown: error
  copyleft: warn
  allow: [example.org/reviewed-module]
```

## Thư viện từ các gói Go

Các ứng dụng import `base/terminal` cũng đóng gói kèm `libghostty-vt`: gói Windows và Linux đặt thư viện bên cạnh tệp thực thi; gói macOS và iOS ký nó vào trong các framework của mình. [Nhúng](embedding.md) giải thích XCFramework đi kèm cho ứng dụng chủ iOS.

Một gói phụ thuộc có thể khai báo các thư viện chia sẻ máy tính để bàn đã biên dịch sẵn trong `nsgo-natives.yaml`. NSGo kiểm tra mã băm SHA-256 của chúng, sao chép vào ứng dụng trước khi ký số và đưa vào các trình cài đặt; tác giả ứng dụng không cần thực hiện thêm bước nào. [Thư viện gốc](native-libraries.md) mô tả định dạng dành cho tác giả gói và các đường dẫn nạp thư viện.

## Lệnh dòng lệnh

Một ứng dụng có thể đóng gói kèm các chương trình dòng lệnh bên cạnh tệp thực thi máy tính để bàn, chẳng hạn một CLI thao tác trên dữ liệu của ứng dụng. Liệt kê chúng dưới mục `commands`, mỗi lệnh được xây dựng từ một gói Go `main`:

```yaml
commands:
  - name: hello-cli             # the file's name, hello-cli.exe on Windows
    package: ./cmd/hello-cli    # a main package, built for each desktop target with CGO_ENABLED=0 and build.tags
    ldflags: [-X main.channel=beta]  # its own linker flags, after build.ldflags
windows:
  path: true                    # the installer puts the app's folder on the user's Path
```

Tên lệnh chỉ gồm chữ cái, chữ số, `-` và `_`, và không được trùng (kể cả không phân biệt hoa thường) với tệp thực thi của ứng dụng, lệnh Linux của nó, `Uninstall`, `AppRun` hoặc một lệnh khác. `macos_universal` gộp hai bản build macOS của từng lệnh, tương tự như gộp bản build của ứng dụng.

Mỗi gói được liên kết với `build.ldflags` và `-ldflags`, sau đó là `ldflags` riêng của nó (giá trị `-X` của nó được ưu tiên), rồi đến danh tính của ứng dụng, nhờ đó `core.App()` cung cấp cho lệnh tên, ID, phiên bản và số bản dựng từ `nsgo.yaml` giống như cho ứng dụng. Một lệnh không import `core` sẽ nhận phiên bản qua `-X`, chẳng hạn `-ldflags "-X main.version=1.2.0"` truyền cho `build`, vốn được áp dụng cho cả ứng dụng lẫn các lệnh của nó.

| Gói | Lệnh |
| --- | --- |
| macOS | Nằm trong `Contents/MacOS`, được ký trước ứng dụng với cùng chế độ như ứng dụng, dưới tên `<id>.<command>`: ký ad hoc, hoặc với `sign.identity`, hardened runtime và dấu thời gian để ứng dụng vượt qua [công chứng](#công-chứng), nhưng không mang các entitlement của ứng dụng. Ảnh đĩa không thêm gì vào `PATH`; [Homebrew cask](#kênh-phân-phối) tự động liên kết từng lệnh, và người dùng có thể tự tạo liên kết tới `/Applications/<name>.app/Contents/MacOS/<command>` |
| Windows | `<command>.exe` nằm cạnh `<name>.exe`, được [ký số](#ký-các-gói-windows) giống như ứng dụng, được trình cài đặt cài vào và trình gỡ cài đặt gỡ bỏ; với `windows.path`, trình cài đặt còn đưa thư mục của ứng dụng vào `Path` của người dùng, giống như `windows.command_line`, để các cửa sổ dòng lệnh mở sau khi cài tìm thấy các lệnh này. Một lệnh bị bản cập nhật đổi tên hoặc bỏ đi vẫn ở lại trong thư mục — giống như tệp ứng dụng không còn phát hành nữa — vì trình gỡ cài đặt chỉ xóa những gì phiên bản của nó đã cài |
| Linux | Nằm trong `/opt/<slug>` kèm một liên kết `/usr/bin/<command>` thuộc quyền sở hữu của gói `.deb` và gói Arch, nên khi gỡ gói liên kết cũng biến mất, và dpkg cùng pacman sẽ từ chối cài đè lên tệp của gói khác; AppImage giữ chúng bên cạnh ứng dụng và không thêm gì vào `PATH` |

Các trình cài đặt, tệp kiểm tra tổng của chúng và các nguồn cấp appcast [cập nhật](#cập-nhật) đều mang theo các lệnh giống như mang theo các tệp khác của ứng dụng, nên bản cập nhật sẽ thay thế chúng cùng lúc với ứng dụng. [Gói kho ứng dụng](#gói-kho-ứng-dụng) loại bỏ các lệnh này, vì sandbox của Mac App Store, MSIX và Flatpak chỉ chạy duy nhất tệp thực thi của chính ứng dụng.

## Trình cài đặt

Đối với các nền tảng đích máy tính để bàn, `build` xuất tệp cài đặt cho người dùng vào `dist/releases`, mỗi tệp kèm theo một tệp kiểm tra tổng `.sha256`:

| Nền tảng đích | Trình cài đặt |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.dmg`, một ảnh đĩa hiển thị biểu tượng ứng dụng, mũi tên và liên kết tới Applications để kéo thả vào, được ký bằng `sign.identity` và [công chứng](#công-chứng) |
| Windows | `<name>-<version>-windows-<arch>-setup.exe`, một trình cài đặt Windows gốc (native) cài cho người dùng hiện tại, tự dùng lại thư mục người dùng đã chọn trước đó, tự đóng ứng dụng nếu đang chạy, có lối tắt Start menu, tùy chọn lối tắt màn hình nền và trình gỡ cài đặt chỉ xóa những gì nó đã cài, được [ký bằng `sign.certificate`](#ký-các-gói-windows) khi ứng dụng có ký gói Windows; `windows.command_line` cài thêm tệp đồng hành `.com` và đưa thư mục ứng dụng vào `Path` của người dùng để cửa sổ dòng lệnh mở sau khi cài có thể chạy lệnh ứng dụng, trong khi Win+R (cũng tìm thấy tệp đồng hành này) khởi chạy ứng dụng và đóng cửa sổ console vừa mở; trình gỡ cài đặt chỉ gỡ thư mục khỏi `Path` nếu trình cài đặt từng thêm nó vào, không bao giờ xóa mục bạn tự thêm |
| Linux | `<name>-<version>-linux-<arch>.deb` cho Debian và Ubuntu, và `<name>-<version>-linux-<arch>.pkg.tar.gz` cho Arch Linux, cài ứng dụng vào `/opt/<slug>` kèm một lệnh, các liên kết tới các [lệnh](#lệnh-dòng-lệnh) của nó, desktop entry và biểu tượng (gói Arch bao gồm `.MTREE` cho `pacman -Qkk`), và `<name>-<version>-linux-<arch>.AppImage` chạy trực tiếp trên mọi bản phân phối mà không cần cài đặt |

Trình cài đặt (setup) và trình gỡ cài đặt Windows được xây dựng chỉ bằng Go, trên bất kỳ hệ thống nào, và dùng các hộp thoại gốc của Windows. Chúng hiển thị các ngôn ngữ trong danh mục của ứng dụng mà chúng có sẵn bản dịch — bao gồm cả tiếng Trung giản thể và phồn thể — chọn theo ngôn ngữ giao diện của hệ thống; `/LANG=<LCID>` chọn một ngôn ngữ cho một lần chạy, chẳng hạn `/LANG=2052` cho tiếng Trung giản thể. `/S` cài đặt hoặc gỡ cài đặt ở chế độ im lặng, `/D=<folder>` đặt ở cuối chọn thư mục cài đặt (được phép chứa dấu cách), và `/RELAUNCH` mở ứng dụng sau khi cập nhật. Khi không có `/D`, bản nâng cấp sẽ cài vào thư mục của bản cài đặt mà nó thay thế. `-installers=false` hoặc `build.installers: false` chỉ xây dựng các gói ứng dụng mà không tạo trình cài đặt.

Các trình cài đặt có thể nhận hình ảnh và biểu tượng riêng của ứng dụng; nếu không thiết lập, mỗi mục sẽ dùng biểu tượng của ứng dụng hoặc hình ảnh dựng sẵn:

```yaml
macos:
  dmg_background: assets/DMGBackground.png  # the disk image's window, which takes its size in points
  dmg_icon: assets/DMGIcon.png              # the disk image's icon: an .icns or a square PNG
windows:
  installer_icon: assets/Setup.png          # the setup program's icon: an .ico or a square PNG
  uninstaller_icon: assets/Uninstall.png    # the uninstaller's
```

Một ảnh nền PNG có bản sao kích thước gấp đôi, như `DMGBackground@2x.png`, sẽ giữ được độ sắc nét trên màn hình Retina. Biểu tượng ứng dụng nằm cách cạnh trái một phần tư chiều rộng, Applications nằm cách cạnh phải một phần tư chiều rộng, mỗi biểu tượng kèm tên được căn giữa theo chiều dọc ngay tại vị trí ảnh dựng sẵn vẽ mũi tên; hãy chừa khoảng trống cho chúng nếu dùng ảnh nền tự thiết kế. Các gói Linux không có hình ảnh riêng; chúng chỉ cài đặt biểu tượng của ứng dụng.

Các gói Linux cũng cài đặt siêu dữ liệu AppStream của ứng dụng tại `/usr/share/metainfo/<id>.metainfo.xml` để các trung tâm phần mềm hiển thị thông tin, được tạo từ `linux.summary`, `linux.license` (nếu có) và tên ứng dụng theo các ngôn ngữ, với danh mục `Utility` nếu `linux.categories` không chỉ định gì; AppImage giữ tệp này trong thư mục `usr/share` của AppDir bên cạnh liên kết tới desktop entry tại `usr/share/applications` mà siêu dữ liệu trỏ tới. Các tệp hệ thống đọc từ `/usr/share` — như siêu dữ liệu chi tiết hơn, các desktop entry bổ sung, tệp tự động hoàn thành lệnh shell hoặc trang hướng dẫn man — được đặt trong thư mục khai báo tại `linux.share` (tương đối so với `nsgo.yaml`): gói `.deb` và gói Arch cài đặt cây thư mục đó dưới `/usr/share`, còn AppImage đặt dưới `usr/share` của AppDir. Tệp `metainfo/<id>.metainfo.xml` bên trong đó sẽ thay thế tệp do NSGo tự tạo, và mọi tệp trùng đường dẫn với tệp mà gói tự tạo (như desktop entry hoặc biểu tượng của ứng dụng) sẽ làm lỗi quá trình build. Các tệp trong `resources/`, bất kể tên gì, vẫn nằm cạnh tệp thực thi trong `/opt/<slug>`, và Flatpak của bản build kho ứng dụng chỉ lấy siêu dữ liệu của NSGo.

AppImage yêu cầu FUSE (vốn có sẵn trên hầu hết bản phân phối) và dùng GTK 4 từ hệ thống giống như các gói khác; runtime của nó được tải về một lần vào bộ nhớ đệm của người dùng.

AppImage nhúng sẵn bộ định vị `gh-releases-zsync` dùng `release.github` hoặc remote GitHub `origin`, trỏ tới bản phát hành ổn định mới nhất và đúng kiến trúc của ứng dụng này. Hãy tải lên từng `.AppImage` cùng với tệp `.AppImage.zsync` được sinh ra. Với máy chủ lưu trữ khác, hãy đặt `linux.appimage_update` thành `zsync|https://example.com/latest.AppImage.zsync`, một URL ổn định có hỗ trợ HTTP range; nó cũng chấp nhận `gh-releases-zsync|owner|repo|latest-pre|name-*-linux-x64.AppImage.zsync` cho kênh phát hành thử nghiệm (prerelease). Truyền một chuỗi rỗng tường minh sẽ tắt tính năng tự động phát hiện nhúng sẵn; nếu thiếu kho GitHub và chưa cấu hình bộ định vị thì giá trị này cũng để trống. Tệp `.zsync` vẫn được sinh ra tách biệt với appcast đã ký.

## Gói kho ứng dụng

`-channel store`, hoặc `updates.channel: store`, xây dựng các nền tảng đích máy tính để bàn và Android dành cho kho ứng dụng tương ứng (nơi đảm nhiệm việc cập nhật ứng dụng), thay thế cho các trình cài đặt và tập lệnh cài đặt:

| Nền tảng đích | Gói kho ứng dụng |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.pkg` cho Mac App Store: ứng dụng chạy trong App Sandbox, được ký bằng danh tính của hồ sơ cấp phép (provisioning profile) và đóng gói bằng danh tính trình cài đặt của nhóm bạn |
| Windows | `<name>-<version>-windows-<arch>.msix` cho Microsoft Store, vốn sẽ tự ký gói này khi xuất bản |
| Linux | Tệp cài đặt `<name>-<version>-linux-<arch>.flatpak`, kèm một tệp `.tar.gz` chứa AppStream và `flatpak/<id>.yml` cho Flathub hoặc `flatpak-builder` trên runtime của GNOME |
| Android | `<name>-<version>-android-<arch>.aab`, một App Bundle cho Google Play được ký bằng `sign.keystore` (khóa tải lên - upload key) |

MSIX đọc tên hiển thị của từng ngôn ngữ từ khóa `appName` trong các danh mục, lùi về ngôn ngữ nguồn nếu thiếu. Các biểu trưng ở Start, thanh tác vụ và Store bao gồm đầy đủ các biến thể 100%, 125%, 150%, 200% và 400%. Công cụ tự tạo chỉ mục tài nguyên ngay trên macOS; không cần Windows SDK.

`-target macos` chọn cả hai kiến trúc macOS trên kênh kho ứng dụng và xuất ra một tệp duy nhất `<name>-<version>-macos-universal.pkg`, tương tự `-target macos_universal`. Chọn `-target android_arm64,android_x64` để tạo một tệp `<name>-<version>-android-universal.aab` duy nhất chứa cả `arm64-v8a` và `x86_64`. Bản cắt `x86_64` yêu cầu phải có Android NDK.

```yaml
macos:
  category: public.app-category.productivity  # LSApplicationCategoryType, which the Mac App Store requires
  # entitlements: { com.apple.security.files.downloads.read-write: true }  # beyond the network, the files the user picks and what apple.info explains
windows:
  store:                               # as Partner Center's Product identity lists them
    identity: 12345Example.Hello
    publisher: CN=00000000-0000-0000-0000-000000000000
    publisher_name: Example
  # device_capabilities: [microphone, webcam]  # the devices the app uses, which Windows asks the user for
linux:
  summary: Says hello                  # one line for software centers
  license: MIT                         # an SPDX expression; LicenseRef-proprietary by default
  # flatpak_runtime: "51"              # GNOME's runtime version
android:
  target_sdk: 36                       # the targetSdkVersion Google Play requires
sign:
  macos_profile: assets/MacAppStore.provisionprofile  # the Mac App Store's profile
  # installer: "3rd Party Mac Developer Installer: Example (ABCDE12345)"  # by default the keychain's of the profile's team
```

Runtime nhắm mục tiêu API 36 khi không thiết lập `android.target_sdk`. Lệnh build Android App Bundle sẽ báo lỗi nếu dưới API 36 — mức mà [Google Play yêu cầu](https://support.google.com/googleplay/android-developer/answer/11926878) đối với ứng dụng di động mới và bản cập nhật từ ngày 31 tháng 8 năm 2026. Bản build APK độc lập chỉ cảnh báo và tiếp tục.

Thiết lập `sign.macos_profile` để ký gói Mac chuẩn bị tải lên. Khi không có tiện ích mở rộng widget, gói chưa ký vẫn chạy được trong sandbox, giúp bạn kiểm tra quyền truy cập vào container của ứng dụng, mạng và các tệp do người dùng chọn. Bản build kho ứng dụng có widget đòi hỏi cả `sign.macos_profile` cho `<id>` lẫn `sign.widgets_profile` bản macOS cho `<id>.widgets`, thuộc cùng một nhóm phát triển và cùng cấp quyền `apple.app_group` (mặc định là `group.<id>`). Cả hai gói đều sử dụng chung container đó; không thêm ngoại lệ truy cập thư mục cá nhân (home-directory exception). `-installers=false` giữ lại chữ ký sandbox và hồ sơ cấp phép trong khi bỏ qua bước tạo `.pkg` và danh tính trình cài đặt. Tải tệp `.msix` lên Partner Center và `.aab` lên Play Console. Gửi `flatpak/<id>.yml` lên Flathub sau khi bản phát hành đã chứa các tệp lưu trữ được nêu trong đó. `flatpak-builder` cũng xây dựng được gói này ở máy cục bộ. Cài đặt gói bằng `flatpak install --user ./<name>-<version>-linux-<arch>.flatpak`; runtime tương ứng của GNOME phải được cài sẵn hoặc sẵn sàng từ một remote đã cấu hình. Quá trình build tự ghi gói hoàn toàn bằng Go mà không cần bộ công cụ Linux. Các gói kho ứng dụng dùng biểu tượng trong suốt khi thiếu hình ảnh và dùng danh mục Utility khi chưa đặt `linux.categories`. Để cài đặt bên ngoài Store, hãy thiết lập [`sign.certificate`](#ký-các-gói-windows) nhằm ký tệp MSIX; chủ thể (subject) của chứng chỉ phải khớp với `windows.store.publisher`, và máy đích phải tin cậy chứng chỉ đó. Khi không cấu hình ký số, Store sẽ tự ký gói được tải lên.

Trên macOS và iOS, hiệu ứng cạnh của khung cuộn (`Scroll.EdgeEffects`), hiệu ứng làm mờ nền trên macOS không dùng `IOS.BackdropMaterial`, và bộ lọc màu nền (dùng riêng hoặc chồng lên lớp mờ không có `IOS.BackdropMaterial`) được vẽ thông qua `CABackdropLayer` và `CAFilter` nội bộ của Core Animation — đúng như cách AppKit và UIKit tạo chất liệu và hiệu ứng cạnh của riêng chúng — vì không có API công khai nào cho phép làm mờ theo bán kính, thay đổi độ mờ qua một view hay áp dụng ma trận màu lên phần nền phía sau. Quy trình xét duyệt App Store có thể từ chối các ứng dụng sử dụng API riêng tư: NSGo chỉ gọi chúng cho các khung cuộn có thanh hiệu ứng cạnh và các bộ lọc nền nói trên, nhưng mọi bản build macOS và iOS đều có chứa tên của các API đó.

Mọi gói Apple mà `build` tạo ra — ứng dụng, tiện ích mở rộng widget, App Clip, ứng dụng Apple Watch và framework `-dynamic` `-embed` — đều mang theo một tệp kê khai quyền riêng tư `PrivacyInfo.xcprivacy`, khai báo các API bắt buộc nêu lý do mà NSGo và Go runtime sử dụng: tùy chọn người dùng (`CA92.1`), dấu thời gian tệp (`C617.1`) và thời gian khởi động hệ thống (`35F9.1`). `apple.privacy` bổ sung phần khai báo riêng của ứng dụng: `accessed_apis`, lý do cho từng danh mục, `tracking`, `tracking_domains` và `collected_data` (các mục thuộc `NSPrivacyCollectedDataTypes` theo đúng tên Apple quy định); một tệp `PrivacyInfo.xcprivacy` đặt trong `resources/` sẽ được trộn hợp cùng thay vì bị ghi đè.

Tệp Info.plist của mỗi gói iOS, tvOS và watchOS mà `build` tạo cho thiết bị thật — gồm ứng dụng, tiện ích mở rộng widget và App Clip — đều mang theo các khóa `DT` về SDK và Xcode mà App Store Connect kiểm tra (được đọc từ bản Xcode thực hiện build), cùng với `UIRequiredDeviceCapabilities: [arm64]`; các gói dành cho trình giả lập bỏ qua các khóa này, và bất kỳ khóa nào được đặt trong `apple.info` đều giữ nguyên giá trị của nó. `apple.multiple_scenes: true` đặt `UIApplicationSupportsMultipleScenes` trong scene manifest của ứng dụng iOS (mặc định là false): trên iPad, người dùng khi đó có thể mở nhiều cửa sổ cùng lúc của ứng dụng, mỗi cửa sổ hiển thị mới view chính với trạng thái riêng và dùng chung các signal cũng như bộ nhớ lưu trữ của ứng dụng cho đến khi người dùng đóng cửa sổ đó.

### Xuất bản lên Google Play

Bật Google Play Developer API và cấp quyền cho một tài khoản dịch vụ (service account) để phát hành ứng dụng lên kênh (track) đã chọn. Đặt `NSGO_PLAY_SERVICE_ACCOUNT` thành chuỗi JSON khóa của tài khoản hoặc đường dẫn tới tệp khóa đó. [Thiết lập Google Play API](https://developers.google.com/android-publisher/getting_started)

```sh
go tool nsgo build -target android -channel store
go tool nsgo publish -target android -track internal
```

Lệnh sẽ tìm App Bundle tương ứng với phiên bản của dự án trong `build.output/releases`. Truyền thêm đường dẫn `.aab` sau các cờ để chọn tệp cụ thể nếu có nhiều bản build. Dùng `internal`, `production`, hoặc ID của một kênh thử nghiệm kín. Lệnh đọc mã gói và version code từ bundle, yêu cầu một mã cao hơn mọi bản phát hành hiện có trên kênh đó, sau đó tải bundle lên và xác nhận phát hành.

## Thông tin trên kho ứng dụng

Siêu dữ liệu trên kho ứng dụng (tiêu đề, phụ đề, mô tả, từ khóa, ghi chú phát hành, URL, ảnh chụp màn hình và bản xem trước) được lưu trong kho mã nguồn dưới dạng YAML và Markdown thuần văn bản tại `store/` (hoặc `store.dir` trong `nsgo.yaml`):

```text
store/
  listing.yaml              # version: 1, default_locale, app_store and google_play settings
  en-US/
    listing.yaml            # version: 1, title, subtitle, short_description, keywords, URLs, media
    description.md          # full description (optional file in place of YAML description)
    release_notes.md        # what's new (optional file in place of YAML release_notes)
    promotional_text.md     # App Store promotional text (optional file)
    screenshots/
  zh-Hans/
    listing.yaml
    description.md
    release_notes.md
    screenshots/
```

Cả hai tệp `listing.yaml` đều bắt buộc có `version: 1` và từ chối các khóa lạ không xác định. Các thư mục ngôn ngữ chuẩn như `zh-Hans` tự động ánh xạ sang mã tương ứng của từng kho (`zh-Hans` trên App Store Connect, `zh-CN` trên Google Play) trừ khi được ghi đè riêng cho từng kho qua `app_store.locale` hoặc `google_play.locale`.

```sh
go tool nsgo publish -store all -dry-run                  # validate lengths, locales and media without uploading
go tool nsgo publish -store appstore,play                 # validate and upload to App Store Connect and Google Play
go tool nsgo publish -store play -track production        # also update release notes on the named Play track
```

Bước kiểm tra hợp lệ chạy trước khi đọc thông tin xác thực hoặc thực hiện bất kỳ yêu cầu HTTP nào, kiểm tra các trường bắt buộc, mã ngôn ngữ, lược đồ URL, tệp phương tiện và số lượng (tối đa 10 ảnh chụp màn hình và 3 video xem trước cho mỗi loại màn hình App Store; tối đa 8 ảnh chụp màn hình cho mỗi loại ảnh Google Play), cùng giới hạn ký tự (App Store: tiêu đề 30, phụ đề 30, từ khóa 100, văn bản quảng bá 170, mô tả 4000, ghi chú phát hành 4000; Google Play: tiêu đề 30, mô tả ngắn 80, mô tả 4000, ghi chú phát hành 500).

Thông tin xác thực tải lên chỉ lấy từ biến môi trường: `NSGO_APP_STORE_KEY`, `NSGO_APP_STORE_KEY_ID` và `NSGO_APP_STORE_ISSUER` (lùi về khóa công chứng `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` và `NSGO_NOTARY_ISSUER` nếu thiếu) cho App Store Connect, và `NSGO_PLAY_SERVICE_ACCOUNT` (đường dẫn tệp JSON tài khoản dịch vụ hoặc chính văn bản JSON) cho Google Play.

### Tuân thủ quy định kho ứng dụng

Sử dụng `nsgo.yaml` để theo dõi trạng thái của từng bảng câu hỏi phát hành.

```yaml
compliance:
  app_store: { age_rating: false }
  google_play: { data_safety: false }
  steam: { content_survey: false }
```

Đổi giá trị một mục thành `true` sau khi đã hoàn thành bảng câu hỏi tương ứng. Bản build `-channel store` hoặc `-channel steam` tương ứng sẽ nhắc nhở bạn về những mục vẫn đang để `false`. Bạn có thể bỏ qua bất kỳ mục nào không muốn theo dõi trong YAML.

- Hoàn thành [bảng câu hỏi xếp hạng độ tuổi](https://developer.apple.com/help/app-store-connect/manage-app-information/set-an-app-age-rating) và [chi tiết Quyền riêng tư Ứng dụng](https://developer.apple.com/help/app-store-connect/manage-app-information/manage-app-privacy) trên App Store Connect.
- Đối với [bảng câu hỏi mã hóa](https://developer.apple.com/help/app-store-connect/manage-app-information/determine-and-upload-app-encryption-documentation) của Apple, đặt `apple.info.ITSAppUsesNonExemptEncryption` thành câu trả lời của ứng dụng dưới dạng giá trị boolean YAML. Nếu Apple cấp mã phê duyệt, hãy đặt nó vào `apple.info.ITSEncryptionExportComplianceCode` dưới dạng chuỗi. Bạn có thể bỏ trống các khóa này và trả lời khi tải lên. Các bản build cho kho Apple sẽ kiểm tra kiểu dữ liệu của các khóa này và cấu trúc tệp kê khai quyền riêng tư của gói.
- Hoàn thành mục [An toàn dữ liệu](https://support.google.com/googleplay/android-developer/answer/10787469) trong Play Console. Các ứng dụng chỉ phân phối trên kênh thử nghiệm nội bộ được miễn bước này.
- Hoàn thành [Khảo sát nội dung](https://partner.steamgames.com/doc/gettingstarted/contentsurvey) trong Steamworks trước khi gửi trò chơi đi xét duyệt.

## App Store Connect

Tạo một ứng dụng trên App Store Connect và ký tệp IPA dành cho thiết bị iOS bằng hồ sơ phân phối App Store hoặc tệp PKG kho ứng dụng macOS bằng danh tính trình cài đặt của nhóm bạn. Trên máy Mac có Xcode, đặt `NSGO_APP_STORE_KEY` thành tệp `.p8` hoặc nội dung khóa của khóa nhóm App Store Connect, `NSGO_APP_STORE_KEY_ID` thành ID của khóa và `NSGO_APP_STORE_ISSUER` thành issuer ID của nó; nếu thiếu, khóa công chứng (`NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID`, `NSGO_NOTARY_ISSUER`) sẽ được dùng thay thế, giống như khi xuất bản thông tin kho ứng dụng. Hãy cấp cho khóa đó quyền truy cập ứng dụng và quyền tải lên bản build.

```sh
go tool nsgo publish ios path/to/Hello.ipa
go tool nsgo publish macos path/to/Hello.pkg
```

Công cụ kiểm tra số bản dựng (build number) của gói trước khi tải lên, sau đó báo cáo kết quả xử lý của Apple cho TestFlight. Hãy tăng `build_number` cho lần tải lên tiếp theo. Số bản dựng macOS phải tăng liên tục qua mọi phiên bản ứng dụng, trong khi iOS có thể bắt đầu lại dãy số cho một phiên bản mới. Gửi bản build đã xử lý để App Review xét duyệt riêng trên App Store Connect. [Hướng dẫn phân phối của Apple](https://developer.apple.com/documentation/xcode/preparing-your-app-for-distribution)

## Gói tài nguyên kho ứng dụng

Khai báo các thư mục tài nguyên lớn trong `mobile.asset_packs`. Các tệp bên trong những thư mục đó được phân phối tách biệt khỏi Android App Bundle và ứng dụng iOS nền. Tên gói bắt đầu bằng một chữ cái thường và có thể chứa chữ cái thường, chữ số và dấu gạch dưới.

```yaml
mobile:
  asset_packs:
    - {name: levels, dir: resources/levels, delivery: install-time}
    - {name: voices, dir: large-assets/voices, delivery: fast-follow}
```

Xây dựng các gói cho Android bằng `go tool nsgo build -channel store -target android_arm64`. Dùng `bundletool build-apks --local-testing` và `bundletool install-apks` để kiểm thử gói trên thiết bị. NSGo mở được các gói `install-time` đã cài đặt. Các module `fast-follow` đã được đóng gói để Play phân phối, còn việc đọc các gói đó từ Play Asset Delivery SDK hiện vẫn đang chờ hoàn thiện.

Trên iOS, `go tool nsgo build -target ios` ánh xạ `install-time` sang chính sách thiết yếu (essential) của Apple và `fast-follow` sang tải trước (prefetch), áp dụng cho cả lần cài đặt đầu tiên và các bản cập nhật. Xcode 26 trở lên biên dịch trình tải xuống Background Assets. Hãy dùng chung một `apple.app_group` hoặc giá trị mặc định `group.<id>`. Cả `sign.profile` của ứng dụng lẫn `sign.asset_packs_profile` của tiện ích mở rộng `<id>.assetdownloader` đều cần được cấp quyền nhóm này. Tải các tệp `.aar` từ `dist/releases` lên cùng với tệp IPA bằng quy trình tải lên asset-pack của App Store Connect. [Tạo các gói tài nguyên được quản lý](https://developer.apple.com/documentation/backgroundassets/creating-managed-asset-packs)

Từ một goroutine, gọi `mobile.AssetPackReady(ctx, "levels")` để kiểm tra trạng thái sẵn sàng và `mobile.OpenAssetPackFile("levels", "intro.map")` để mở một tệp tính tương đối từ thư mục đã khai báo. Nhớ đóng reader trả về sau khi dùng xong. [Kiểm thử cục bộ gói tài nguyên Apple](https://developer.apple.com/documentation/backgroundassets/testing-asset-packs-locally)

## Kho lưu trữ Steam

```sh
go tool nsgo build -channel steam -target macos_universal,windows_x64,linux_x64
```

Đặt `updates.channel: steam` để dùng kênh này theo mặc định. Chọn các nền tảng đích macOS, Windows và Linux. Tải từng thư mục `dist/<name>-<version>-<target>` lên Steam depot tương ứng của nó, cấu hình tùy chọn khởi chạy là gói ứng dụng trên macOS, tệp thực thi trên Windows và Linux arm64, và `steam-launch.sh` trên Linux x64. Steam tự đảm nhiệm việc cập nhật. Các thư mục này không chứa trình cài đặt, bộ tự cập nhật hay tệp `steam_appid.txt`.

Các depot Linux x64 đóng gói sẵn GTK 4 cùng các phần phụ thuộc cho Steam Linux Runtime 3.0 và 4.0. Trình khởi chạy chỉ dùng chúng khi hệ thống không có sẵn GTK 4.14 trở lên. Hãy giữ thư mục `nsgo-gtk/` nằm cạnh trình khởi chạy: `licenses/` bên trong chứa giấy phép của các thư viện, mã nguồn dùng để biên dịch chúng và tập lệnh build — điều mà giấy phép LGPL bắt buộc đối với GLib, Pango và GTK. Linux arm64 yêu cầu có sẵn GTK 4 trong runtime của nó.

Để kiểm thử cục bộ bên ngoài Steam client, đặt `steam: {app_id: 480}` trong `nsgo.yaml`, thay 480 bằng App ID Steamworks của bạn. `go tool nsgo dev` ghi ID đó vào tệp `steam_appid.txt` trong thư mục build gốc của nó và khởi chạy ứng dụng với thư mục làm việc đặt tại chính thư mục đó. [Steamworks API](https://partner.steamgames.com/doc/api/steam_api), [SteamPipe](https://partner.steamgames.com/doc/sdk/uploading).

## Phát hành trên GitHub

Khi bản build có nền tảng đích macOS, Windows hoặc Linux, `dist/releases` cũng chứa `install.sh` và `install.ps1` dùng để cài đặt ứng dụng từ các bản phát hành của kho GitHub do `release.github` chỉ định (hoặc kho của Git remote `origin` nếu không khai báo); nếu không có cả hai, `build` sẽ không tạo các tập lệnh này và thông báo rõ. Hãy tải các tệp ở tầng cao nhất của thư mục đó (trừ `miniprogram/`) lên bản phát hành gắn thẻ `v<version>` (`release.tag_prefix` thay thế cho `v`; các tập lệnh tự mã hóa các ký tự URL dành riêng trong tiền tố đó):

```sh
gh release create v1.2.0 dist/releases/*.*
```

Khi đó người dùng có thể cài đặt hoặc cập nhật ứng dụng chỉ bằng một dòng lệnh do quá trình build in ra:

```sh
curl -fsSL https://github.com/me/hello/releases/latest/download/install.sh | sh    # macOS and Linux
```

```powershell
irm https://github.com/me/hello/releases/latest/download/install.ps1 | iex         # Windows
```

Trên macOS, tập lệnh sao chép ứng dụng từ ảnh đĩa vào `/Applications`, hoặc `~/Applications` nếu không có quyền ghi vào đó; trên Linux, nó cài gói Arch bằng `pacman`, hoặc gói Debian bằng `apt-get` hay `dpkg` thông qua `sudo`; trên Windows, nó chạy trình cài đặt ở chế độ im lặng và tự cài đè vào nơi ứng dụng đã nằm sẵn. Mỗi bước đều đối chiếu tệp với mã kiểm tra tổng trước tiên. Biến môi trường `<NAME>_VERSION` (với tên ứng dụng viết hoa, như `HELLO_VERSION=1.1.0`) cài một phiên bản phát hành khác, và `<NAME>_DOWNLOAD_URL` tải xuống từ một máy chủ gương (mirror).

## Xuất bản lên itch.io

Đặt `itch.project` thành slug `owner/game` của trò chơi itch.io của bạn trong `nsgo.yaml`. Để ghi đè tên kênh, hãy thiết lập `itch.channels` theo từng nền tảng đích build. Mặc định là `html5` cho web, `osx-<arch>` cho macOS, `windows-<arch>` cho Windows và `linux-<arch>` cho Linux.

```yaml
itch:
  project: example/my-game
  channels:
    macos_arm64: osx-arm64-beta
```

Sau khi thiết lập bí mật `BUTLER_API_KEY` trong CI, hãy build và tải lên các nền tảng đích.

```sh
go tool nsgo build -channel none -installers=false -target web,macos_arm64
go tool nsgo publish itch -target web,macos_arm64
```

Hãy dùng cùng giá trị `-output` và `-version` trên cả hai lệnh nếu bạn ghi đè chúng. Quá trình tải lên lấy thư mục gói của từng nền tảng đích và đặt phiên bản trên itch.io bằng phiên bản của bản build. NSGo dùng `butler` có sẵn trên `PATH` hoặc tự tải xuống một bản cố định từ itch.io. Sau lần tải bản web đầu tiên, hãy đặt loại trò chơi trên itch.io thành HTML và đánh dấu bản tải lên có thể chơi trực tiếp trên trình duyệt. [Tài liệu Butler](https://itch.io/docs/butler/pushing.html)

## Cập nhật

`updates` giúp ứng dụng tự động cập nhật theo đúng cách của nền tảng và kênh phân phối mà người dùng cài đặt nó:

```yaml
updates:
  key: zE0hx2FZwwHy6X7LbEP3GjTTE3KWfCAp5gsaBysC+vg=  # the public key nsgo keygen prints
  # channel: store                    # direct, store, steam or none; direct by default with a key, none without
  # feed: https://example.com/hello/  # where the appcasts and installers are; by default the latest GitHub release
  # automatic: false                  # check only when asked; true by default
  # automatic_downloads: true          # Sparkle downloads automatically; false by default, macOS only
```

`nsgo keygen` in ra một cặp khóa mà không ghi tệp nào: hãy đưa khóa công khai vào `updates.key` và commit nó, còn khóa bí mật giữ bên ngoài kho mã nguồn — trong trình quản lý mật khẩu và trong biến `NSGO_UPDATES_KEY` tại nơi build bản phát hành, chẳng hạn một secret của GitHub Actions. Mỗi bản build trực tiếp sẽ ký các trình cài đặt bằng khóa bí mật và ghi `appcast-<target>.xml` bên cạnh chúng, theo định dạng nguồn cấp dữ liệu của [Sparkle](https://sparkle-project.org) mà ứng dụng đọc tại `feed`: hãy tải lên các tệp appcast cùng với các trình cài đặt. Nếu thiếu khóa bí mật, `build` sẽ không ghi appcast cho macOS hay Windows vì hai nền tảng này chỉ cài đặt các trình cài đặt đã ký, còn ứng dụng Linux và Android khi đọc một appcast chưa ký sẽ mở trang phát hành thay vì tự cài gói.

`-release-notes notes.md` đưa một tệp Markdown chứa ghi chú phát hành (như mục tương ứng trong changelog) vào mỗi appcast, được kết xuất thành HTML nhưng loại bỏ HTML thô có sẵn trong tệp. Sparkle và WinSparkle hiển thị nội dung này trong hộp thoại cập nhật của chúng, còn thông báo trên Linux và Android hiển thị nó dưới dạng văn bản bên dưới số phiên bản, tối đa 1.500 ký tự; ứng dụng đọc nội dung này qua `core.UpdateCheck.Notes`.

| Nền tảng đích | Cách bản build trực tiếp cập nhật |
| --- | --- |
| macOS | Sparkle (do `build` nhúng vào ứng dụng) kiểm tra hằng ngày, đề xuất phiên bản mới, tải về ảnh đĩa, kiểm tra chữ ký, cài đặt và khởi động lại ứng dụng; menu ứng dụng mặc định có sẵn mục Check for Updates…, và ứng dụng tự định nghĩa menu có thể thêm mục này qua `updater.Check` |
| Windows | WinSparkle (nằm cạnh tệp thực thi) thực hiện quy trình tương tự bằng ngôn ngữ ứng dụng đang hiển thị lúc khởi chạy nếu WinSparkle hỗ trợ, rồi chạy trình cài đặt ở chế độ im lặng và khởi động phiên bản mới |
| Linux, Android | Ứng dụng đọc appcast của mình, và khi người dùng chọn Update, ứng dụng tải trực tiếp và xác minh gói đã ký xuống đĩa kèm báo cáo tiến trình (`updater.Status` và thanh tiến trình tác vụ của nền tảng), rồi cài đặt qua `pkexec dpkg`, `pkexec pacman` (`.pkg.tar.gz`/`.pkg.tar.zst`), thay thế nguyên tử AppImage (tự khởi động lại trên Linux), hoặc trình cài đặt gói của Android; các định dạng nằm ngoài danh sách đó hoặc appcast chưa ký sẽ mở trang phát hành thay thế |
| iOS | Ứng dụng truy vấn App Store (nơi đảm nhiệm cập nhật) và mở trang ứng dụng tại đó |

Với ứng dụng web độc lập đã xuất bản, `updater.Check(cx)` so sánh bản phát hành đang chạy với tệp `nsgo.json` mới và đề xuất tải lại trang sau khi người dùng xác nhận. Nó cũng nhận biết được các tệp build đã thay đổi dưới cùng một nhãn phiên bản. Tính năng này không cần khóa appcast; các kênh `none`/`steam` tường minh vẫn vô hiệu hóa việc kiểm tra, còn các máy chủ nhúng tự quản lý việc tải lại của chúng. Các mini program dùng trình quản lý cập nhật của chính nền tảng và chỉ khởi động lại sau khi bản cập nhật đã tải xong. Các nền tảng đích này kiểm tra khi ứng dụng gọi bộ cập nhật; thời điểm tải/kiểm tra của mini program thuộc quyền của ứng dụng chủ. Chúng không cung cấp các hàm thiết lập tự động kiểm tra. Một trình xử lý `cx.CheckForUpdates` tùy chỉnh sử dụng `UpdateCheck.Restart` cho bản cập nhật đã sẵn sàng mà nhãn phiên bản có thể bị khuyết.

### Kênh phân phối

Kênh phân phối quyết định bên nào chịu trách nhiệm cập nhật ứng dụng. `-channel` thay thế `updates.channel` cho một lần build, nhờ đó một dự án có thể build cho từng kênh:

| Kênh | Dành cho | Các gói |
| --- | --- | --- |
| `direct` | GitHub, trang web, Homebrew cask và winget — những nơi cài trực tiếp các tệp của bản phát hành | Tự cập nhật như mô tả ở trên |
| `store` | App Store, Mac App Store, Microsoft Store, Google Play, Flathub và các kho ứng dụng khác | Là các [gói kho ứng dụng](#gói-kho-ứng-dụng) và không mang theo bộ tự cập nhật; iOS và macOS hỏi App Store của chúng mà không cần khóa, còn bản cài đặt từ Google Play có thể dùng luồng cập nhật gốc của Play |
| `steam` | Steam | Tạo ra các [thư mục depot](#kho-lưu-trữ-steam) để Steam cập nhật |
| `none` | Các bản phân phối Linux và bất kỳ ai đóng gói lại ứng dụng | Không kiểm tra gì cả |

Các kho ứng dụng từ chối những ứng dụng tự cập nhật, vì vậy hãy tải lên bản build `store`, tuyệt đối không dùng bản `direct`. Mỗi bản phát hành trực tiếp có trình cài đặt macOS hoặc Windows cũng tạo ra một Homebrew cask (`dist/releases/homebrew/<name>.rb`, khai báo `auto_updates true`, mã kiểm tra tổng và URL tải xuống cho các ảnh đĩa đã build, cùng một liên kết `binary` cho từng [lệnh](#lệnh-dòng-lệnh) của ứng dụng) và một tệp kê khai nhiều tệp cho winget (`dist/releases/winget/<id>.yaml`, `<id>.installer.yaml` và `<id>.locale.en-US.yaml` cho các trình cài đặt NSIS đã build). Bất cứ khi nào hệ thống cho biết nguồn đã cài đặt ứng dụng, ứng dụng đều kiểm tra thông tin đó: ứng dụng Android cài từ một kho khác, gói Flatpak hay Snap, và bản build TestFlight đều nhường quyền cập nhật lại cho kho đó. Lệnh `updater.Check` gọi thủ công trong bản cài đặt Google Play sẽ yêu cầu Play In-App Updates mở luồng cập nhật tức thì khi được phép; Play lo phần xin chấp thuận, tải xuống, cài đặt và khởi động lại. Các bản build phát triển không bao giờ đi vào luồng đó. Gói Android độc lập đóng gói sẵn SDK; [ứng dụng chủ nhúng](embedding.md#android) cung cấp SDK qua Gradle.

### Kiểm tra từ trong ứng dụng

`updater.Check(cx)` kiểm tra giống như mục menu *Check for Updates…* và hiển thị kết quả tìm được; `updater.CheckInBackground(cx)` chỉ hiển thị khi có phiên bản mới hơn, và `updater.CheckDaily(cx)` thực hiện việc kiểm tra đó tối đa mỗi ngày một lần. `updater.Status(cx)` đọc tiến trình tải xuống và cài đặt trực tiếp trên Linux và Android. Khi `automatic` đang bật, ứng dụng tự động kiểm tra theo cách đó trên Linux, bản build Android trực tiếp và bản build Apple App Store mỗi khi khởi chạy và bất cứ khi nào người dùng quay trở lại ứng dụng; nếu một lần kiểm tra thất bại (ví dụ khi ngoại tuyến), nó sẽ thử lại sau một giờ. Bản build phát triển không bao giờ cập nhật. Trong bản build Steam, ba hàm này không làm gì cả.

`updater.CapabilityOf(cx)` báo cáo cách bản sao hiện tại cập nhật mà không cần gửi yêu cầu mạng: `ManagedSelf`, `ManagedPage`, `ManagedDevelopment`, `ManagedChannel`, `ManagedStore` (kèm tên `Store` của nó), hoặc `ManagedUnavailable`. Bản build phát triển, kênh bị vô hiệu hóa hoặc bản cài đặt từ kho ứng dụng sẽ trả về giải thích theo đúng nguyên nhân khi kiểm tra; `core.UpdateCheck.Err` chỉ dành riêng cho các lỗi thực sự như nguồn cấp hỏng hoặc bộ cập nhật đi kèm không thể nạp.

Đọc `updater.Status(cx)` bên trong một binding để theo dõi `Capability`, `AutomaticChecks`, `AutomaticDownloads`, `Available`, `Notes` và `LastCheck`, và trên Linux cùng Android là `Phase`, `Loaded` và `Total` của bản cập nhật (`Fraction()` dùng cho thanh tiến trình). `updater.SetAutomaticChecks(cx, on)` lưu lựa chọn của người dùng và thay đổi bộ lập lịch hằng ngày ngay khi đang chạy. Sparkle và WinSparkle tự nắm giữ tùy chọn của chúng; Linux, Android và iOS lưu lựa chọn trong một phân vùng lưu trữ riêng tư. `updater.SetAutomaticDownloads(cx, on)` chỉ được hỗ trợ trên macOS, mặc định lấy theo `updates.automatic_downloads`; các nền tảng khác trả về false và bỏ qua hàm thiết lập này. Các bản build không hỗ trợ và được quản lý từ bên ngoài đều báo cả hai thiết lập là false. WinSparkle báo siêu dữ liệu bản cập nhật tìm thấy bằng cách đọc appcast sau callback tìm thấy bản mới; nó vẫn kiểm soát quá trình cài đặt. Linux xác định quyền sở hữu gói Debian hoặc Arch và danh tính AppImage lúc khởi chạy; những bản sao đó báo `ManagedSelf`, trong khi các bản sao chưa đóng gói khác báo `ManagedPage` trước khi có bất kỳ lần kiểm tra nào.

## Khởi chạy khi đăng nhập

`desktop.SetLaunchAtLogin(cx, on)` đăng ký ứng dụng với cơ chế đăng nhập của nền tảng, và `desktop.LaunchAtLogin(cx, done)` đọc trạng thái đó. Một gói MSIX do công cụ hiện tại xây dựng tự động kèm theo một tác vụ khởi động đang tắt; việc bật nó tuân theo công tắc Ứng dụng khởi động của người dùng và chính sách quản trị viên. Bản cài đặt Windows và Linux thông thường lưu giữ khóa Run và mục XDG autostart của chúng. macOS sử dụng dịch vụ đăng nhập gốc của gói ứng dụng.

Flatpak gửi yêu cầu tới Background portal (có thể hỏi người dùng chấp thuận) mà không cần cấp quyền truy cập vào các tệp cấu hình của máy chủ. Lệnh đọc tiếp theo sẽ đợi phản hồi gốc. Portal này không có truy vấn khởi động chỉ đọc: các lệnh đọc trên Flatpak chỉ trả về phản hồi đã xác nhận gần nhất trong lượt chạy này (false nếu chưa có), và không thể phát hiện thay đổi từ bên ngoài ứng dụng hay khôi phục trạng thái ở lượt chạy kế tiếp.

Một gói Snap được đóng gói từ bên ngoài cần khai báo tên tệp tự khởi động trên ứng dụng của nó:

```yaml
apps:
  app:
    command: bin/app
    autostart: app.desktop
```

NSGo ghi tệp có tên đó vào `$SNAP_USER_DATA/.config/autostart`, để snapd khởi chạy thông qua trình bọc của ứng dụng. `desktop.LaunchedAtLogin(cx)` phân biệt một lần khởi chạy khi đăng nhập ngay trước khi ứng dụng dựng giao diện, bao gồm cả lượt kích hoạt MSIX StartupTask.

## Loại tệp

`file_types` đăng ký các tài liệu mà ứng dụng có thể mở: từ Finder, Dock và Open With trên macOS; từ Open With của Explorer thông qua trình cài đặt Windows hoặc MSIX trên Windows; thông qua desktop entry của các gói Debian, Arch, AppImage và Flatpak trên Linux; và thông qua Open In trên iOS:

```yaml
file_types:
  - name: Flow File
    extensions: [flow]
    # id: dev.example.hello.flow   # <id>.<first extension> by default; another's ID imports its type
    # icon: assets/Flow.icns       # macOS document icon; or a square PNG
    # role: viewer                 # editor by default
    # conforms: [public.json]      # public.data by default
    # mime: [application/json]     # existing MIME types; otherwise inferred on Linux
```

Trình cài đặt Windows thêm ứng dụng vào Open With mà không ghi đè lựa chọn mặc định của người dùng, và gỡ bỏ các đăng ký của nó khi nâng cấp và gỡ cài đặt. Linux đặt tên kiểu MIME cho các phần mở rộng phổ biến từ một bảng cố định, và gán cho phần mở rộng lạ thuộc loại riêng của ứng dụng kiểu `application/x-<type id>` kèm một định nghĩa shared-mime-info. `mime` chỉ định các kiểu MIME có sẵn để thay thế; một kiểu được nhập vào mà phần mở rộng không có trong bảng thì bắt buộc phải khai báo trường này. Chạy trực tiếp một AppImage không đăng ký gì: một công cụ tích hợp desktop sẽ cài đặt desktop entry và các định nghĩa của nó.

Ứng dụng nhận danh sách tệp qua `desktop.OnOpenFiles`, bao gồm cả trường hợp khởi chạy Windows hoặc Linux lần sau trong lúc ứng dụng đang chạy và thao tác mở tài liệu trên scene iOS. Các tệp đã khởi chạy ứng dụng sẽ chờ trình xử lý đầu tiên được đăng ký:

```go
desktop.OnOpenFiles(cx, func(paths []string) { open(paths[0]) })
```

Trên iOS, các tài liệu thông thường được nhập vào các tệp tạm thuộc quyền sở hữu của ứng dụng với giữ nguyên tên gốc. Các thao tác đọc từ trình cung cấp tệp được điều phối ngoài luồng UI, và phạm vi bảo mật của chúng kết thúc trước khi trình xử lý chạy. Hãy di chuyển bản nhập vào `cx.DataDir()` nếu muốn giữ lại lâu dài; mọi chỉnh sửa sẽ tác động lên bản sao này. iOS dùng biểu tượng tài liệu của ứng dụng; `icon` chỉ áp dụng trên macOS. App Clip và gói tvOS không đăng ký loại tài liệu nào. Thao tác mở tệp tách biệt hoàn toàn với `OnOpenLinks`.

`desktop.AddRecentFile(cx, path)` ghi lại một tài liệu người dùng vừa mở: trong menu Dock và File › Open Recent trên macOS, mục Recent của Jump List trên Windows, và danh sách tệp dùng gần đây của desktop trên Linux. Khi người dùng chọn lại, sự kiện đi tới cùng trình xử lý trên. `desktop.ClearRecentFiles` xóa lịch sử đã đăng ký của ứng dụng, vẫn giữ nguyên lịch sử của các ứng dụng khác đối với tệp dùng chung trên Linux; Linux cũng tuân thủ cài đặt lịch sử của GTK.

`desktop.SetDockMenu(cx, items)` thiết lập các hành động trên biểu tượng ứng dụng: menu Dock trên macOS, các tác vụ Jump List trên Windows và quicklist trên dock của Ubuntu, Plasma cùng Dash to Dock ở những nơi hỗ trợ quicklist. Linux yêu cầu có desktop entry đã cài đặt khớp với `id` của ứng dụng, và giữ nguyên được menu con, đường phân cách, mục đã đánh dấu và mục bị vô hiệu hóa. Menu này gắn với scope đã thiết lập nó, và lần thiết lập cuối cùng trên toàn bộ các cửa sổ của ứng dụng sẽ thay thế các lần trước.

## Liên kết

`url_schemes` cấp cho ứng dụng các liên kết riêng như `myapp://settings`, giúp mở ứng dụng từ trình duyệt, ứng dụng khác, `open`, `start` hoặc `xdg-open`. Các ứng dụng macOS, iOS và Android tự đăng ký chúng, tương tự với trình cài đặt Windows, gói MSIX và desktop entry trên Linux (được mang theo bởi các gói Debian, Arch, AppImage và Flatpak). Việc gỡ cài đặt sẽ xóa các đăng ký này.

```yaml
url_schemes: [myapp]
# app_links: [links.example.com]  # HTTPS hosts for the full app
```

Ứng dụng nhận các liên kết này qua `desktop.OnOpenLinks`, kể cả khi đang chạy: trên Windows và Linux, một lần khởi chạy thứ hai sẽ chuyển liên kết của nó cho ứng dụng đang chạy rồi thoát. Một liên kết đã khởi chạy ứng dụng sẽ chờ trình xử lý đầu tiên. Bất kỳ ai cũng có thể tạo ra một liên kết như vậy, và trên Windows cùng Linux trình xử lý nhận mọi URL được truyền lúc khởi động ứng dụng, vì vậy hãy kiểm tra scheme và hỏi ý kiến người dùng trước khi để một liên kết làm thay đổi điều gì:

```go
desktop.OnOpenLinks(cx, func(links []string) { show(links[0]) })
```

Trên iOS và Android, `mobile.OnOpenLinks` nhận các liên kết lúc khởi chạy và trong khi ứng dụng đang chạy. Hãy mở thử bằng `xcrun simctl openurl booted myapp://settings` trên trình giả lập iOS hoặc `adb shell am start -a android.intent.action.VIEW -d myapp://settings` trên Android. `app.OpenLinks("myapp://settings")` gửi một liên kết trong bài kiểm thử.

Đối với universal link trên iOS, hãy khai báo tường minh các máy chủ HTTPS bằng `app_links: [links.example.com]`. Mỗi máy chủ là một tên miền DNS không có scheme, đường dẫn, cổng, địa chỉ IP hay ký tự đại diện; tên miền được chuyển về chữ thường và các mục trùng lặp bị từ chối. Công cụ ghi `applinks:<host>` vào entitlement Associated Domains của ứng dụng đầy đủ, kể cả khi dùng ký tự động của Xcode; một hồ sơ cấp phép cho IPA phải cho phép quyền này. App Clip vẫn giữ riêng các khai báo `appclips:` từ `app_clip.domains`.

Mỗi máy chủ phải phục vụ tệp `apple-app-site-association` có ghi tên ứng dụng đầy đủ tại `https://<host>/.well-known/apple-app-site-association`; chỉ riêng siêu dữ liệu gói không thể cấp quyền cho universal link. [Yêu cầu liên kết của Apple](https://developer.apple.com/documentation/xcode/supporting-associated-domains) mô tả cách thiết lập máy chủ. Chữ ký ad hoc trên trình giả lập bỏ qua các entitlement dành cho nhà phát triển bị hạn chế, nên một bài kiểm thử delegate không chứng minh được sự liên kết từ phía máy chủ.

Trên iOS, `mobile.OpenedURL(cx)` theo dõi lượt gọi App Clip, universal link hoặc URL scheme tùy chỉnh gần nhất, bao gồm cả giá trị ban đầu trước khi thành phần gốc được gắn lên cây. Lượt chuyển tiếp custom scheme vẫn đi tới `mobile.OnOpenLinks`; giá trị lượt gọi luôn là giá trị mới nhất khi trình xử lý đó chạy. Thao tác mở tệp vẫn nằm ở `OnOpenFiles` và không ghi đè lên URL lượt gọi. Hỗ trợ lượt gọi trên Android/Web được theo dõi riêng tại #1475.

## Nội dung chia sẻ đến trên Android

Chọn các kiểu MIME mà ứng dụng xử lý từ bảng chia sẻ của Android:

```yaml
android:
  share_types: [text/plain, 'image/*', application/pdf]
```

Các bản build APK và App Bundle đăng ký các kiểu đó cho cả trường hợp chia sẻ một hoặc nhiều mục. Hãy đăng ký [`mobile.OnShared`](services.md#mở-liên-kết-và-tệp) để nhận từng đợt chia sẻ.

Đối với ứng dụng web dùng địa chỉ theo đường dẫn, hãy đặt `web.base` thành thư mục triển khai của nó (`/` hoặc `/my-app/`) và cấu hình máy chủ sản xuất trả về `index.html` cho các tuyến đường ứng dụng. Máy chủ phát triển đã cung cấp sẵn cơ chế dự phòng đó cho các yêu cầu điều hướng HTML. Tài nguyên vẫn giữ đường dẫn tương đối theo thẻ base HTML cố định đó khi tải lại một liên kết sâu. Xem [Địa chỉ ứng dụng](addresses.md).

## Nội dung chia sẻ đến trên iOS

Khai báo các Uniform Type Identifier mà tiện ích mở rộng chia sẻ chấp nhận, đặt các kiểu URL và văn bản trước các kiểu tệp tổng quát hơn:

```yaml
apple:
  share_types: [public.url, public.plain-text, public.image, com.adobe.pdf]
  # app_group: group.com.example.app  # defaults to group.<id>
sign:
  # share_profile: assets/Share.mobileprovision
```

`build` và `dev` đóng gói một tiện ích mở rộng chia sẻ viết bằng Go có tên `<id>.share` cùng với ứng dụng. Xcode ký nó và đăng ký App Group của nó khi chạy trên thiết bị thật. Với một IPA, hãy bật cùng App Group cho cả `<id>` và `<id>.share`; `sign.profile` và `sign.share_profile` đều phải cấp quyền cho nhóm đó. App Clip, tvOS và watchOS không có tiện ích mở rộng chia sẻ.

Tiện ích mở rộng sao chép tối đa 128 mục và 512 MiB tệp vào App Group, rồi hoàn tất bảng chia sẻ. `mobile.OnShared` nhận các lô dữ liệu đó trong khi ứng dụng đang hoạt động hoặc vào lần kế tiếp người dùng mở ứng dụng; dữ liệu vẫn được bảo toàn ngay cả khi tiện ích mở rộng và ứng dụng đã thoát trước lúc giao nhận. iOS không cung cấp API được hỗ trợ nào cho phép Share extension đưa ứng dụng chứa nó lên tiền cảnh hay tự khởi chạy ứng dụng đó. Vì vậy bước bàn giao này không tự động mở ứng dụng.

## Mini program

```sh
go tool nsgo build -target miniprogram_wechat -run
go tool nsgo dev -target miniprogram_alipay
```

Mỗi nền tảng đích xuất ra một dự án cho công cụ phát triển của nền tảng đó, đồng thời đặt trong `dist/releases/miniprogram/<platform>`, và `-run` cùng `dev` sẽ mở dự án đó lên; `dev` tự động build lại khi bạn lưu tệp, và công cụ sẽ tải lại:

| Đích | Nền tảng | Công cụ |
| --- | --- | --- |
| `miniprogram_wechat` | WeChat | [WeChat DevTools](https://developers.weixin.qq.com/miniprogram/dev/devtools/download.html), với cổng dịch vụ đang bật (Settings, Security) |
| `miniprogram_qq` | QQ | [Công cụ phát triển của QQ](https://q.qq.com/wiki/tools/devtool/), trên macOS |
| `miniprogram_alipay` | Alipay | [Mini Program Studio của Alipay](https://opendocs.alipay.com/mini/ide/download), trên macOS |
| `miniprogram_douyin` | Douyin và các ứng dụng khác của ByteDance | [Công cụ phát triển của Douyin](https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/developer-instrument/download/developer-instrument-update-and-download), trên macOS |

`miniprogram.appid` cung cấp AppID của mini program trên từng nền tảng (`wechat`, `qq`, `alipay`, `douyin`); nếu không khai báo, WeChat DevTools xem trước bằng AppID khách (tourist AppID) còn các công cụ khác sẽ hỏi nhập AppID. Trên WeChat, `-run` và mỗi lần build lại của `dev` sẽ đóng và tự động mở lại dự án qua CLI, nhờ đó DevTools biên dịch các tệp mới nhất. Hành vi này đã được kiểm tra với DevTools 2.01.2510290. Thanh điều hướng của nền tảng hiển thị tiêu đề ứng dụng; khi bật `custom_navigation`, ứng dụng phủ kín trang và vùng an toàn (safe area) giúp giữ nội dung tránh khỏi nút menu. Ứng dụng đọc hệ điều hành của điện thoại với `Env.Simulated` bằng true, giống như trên web với `--nsgo-os`: các trang trong ngăn xếp tự hiển thị `Header` riêng vì không có tiêu đề hệ thống nào thay thế. QQ, Alipay và Douyin chưa được thử nghiệm trong các công cụ của chúng, và Alipay hiện chưa chuyển nút quay lại qua ứng dụng, chưa hỗ trợ keyframes hoặc đọc chiều cao bàn phím.

Mini program đang ở giai đoạn thử nghiệm. Mini program chỉ chạy các ứng dụng Go thuần túy chứ không chạy TSX, và các nền tảng giới hạn dung lượng module nén của ứng dụng ở mức 2 MB: một ứng dụng vài màn hình dùng `base` vẫn lọt ngưỡng này (như `examples/gallery-miniprogram`), còn ứng dụng dùng các điều khiển của `ui` hiện chưa vừa, và `build` sẽ báo rõ khi vượt ngưỡng. Shader không vẽ gì trên đó, còn web view, video, thành phần gốc và các tệp phông chữ của ứng dụng hiện chưa được hiển thị.

## HarmonyOS

```sh
go tool nsgo build -target harmonyos -run
```

`harmonyos` ghi `<name>-<version>-harmonyos.hap` vào `dist/releases`. Engine chạy dưới dạng WebAssembly bên trong ArkWeb, và một ứng dụng chủ ArkTS vẽ những gì nó gắn lên bằng các view ArkUI, nhờ đó văn bản, trường nhập liệu, khung cuộn và hộp thoại đều là thành phần gốc của hệ thống. Khi có `hdc` trên `PATH`, `-run` cài đặt HAP lên thiết bị hoặc trình giả lập đang kết nối và khởi chạy nó, còn `nsgo test -target harmonyos` chạy các bài kiểm thử ứng dụng từ một HAP chưa ký trên trình giả lập dưới một danh tính gói `.nsgotest` riêng biệt, rồi gỡ bỏ HAP kiểm thử đó sau khi chạy xong để ứng dụng đã cài đặt vẫn còn nguyên.

Để biên dịch ứng dụng chủ ArkTS từ các mẫu và dùng đúng `id` của ứng dụng làm tên gói, hãy đưa `hvigorw` vào `PATH` và đặt `OHOS_BASE_SDK_HOME` trỏ tới OpenHarmony SDK có API 26 (`26.0.0`), phiên bản SDK biên dịch và mục tiêu của các mẫu. `build.harmonyos_bundles` chọn giữa `auto` (mặc định), `exact` (đòi hỏi phải có SDK), hoặc `padded` (đệm id cho khớp độ dài cố định của ứng dụng chủ đã commit sẵn và không cần SDK). Các bài kiểm thử theo đích dùng chung thiết lập này và in ra ứng dụng chủ nào đã được đóng gói. `harmonyos.permissions` liệt kê các tên quyền `ohos.permission` mà ứng dụng yêu cầu, như `MICROPHONE` và `CAMERA`, còn `widget.title` là tiêu đề hiển thị trên thẻ màn hình chính.

HarmonyOS đang ở bản xem trước (preview). Trình giả lập có thể cài các tệp HAP chưa ký. [Ghi chú kiến trúc](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/harmonyos.md) liệt kê các tính năng ArkUI mà bản xem trước hỗ trợ vẽ.

### HAP đã ký

Thiết lập `harmonyos.signing` để ký một HAP bằng tài liệu từ AppGallery Connect của bạn. Cài đặt trực tiếp lên điện thoại Huawei cần có chứng chỉ gỡ lỗi và hồ sơ cho thiết bị đó. Hãy dùng tài liệu phát hành khi phân phối. [Hướng dẫn ký của Huawei](https://developer.huawei.com/consumer/en/doc/harmonyos-guides-V2/command-line-building-app-hap-hvigor-0000001553061909-V2/)

```yaml
harmonyos:
  signing:
    keystore: signing/app.p12
    key_alias: app
    certificate: signing/app.cer
    profile: signing/app.p7b
```

Các đường dẫn là tương đối so với `nsgo.yaml`. Đặt `NSGO_HARMONY_KEYSTORE_PASSWORD` và `NSGO_HARMONY_KEY_PASSWORD` trong biến môi trường. Việc ký số sử dụng `hap-sign-tool.jar` của SDK cùng Java từ `JAVA_HOME` hoặc `PATH`. Bản build giữ chính xác `id` của ứng dụng để khớp với hồ sơ, nên việc ký yêu cầu phải có SDK và `hvigorw` ngay cả khi đang ở chế độ `auto`. Chạy `go tool nsgo doctor` trong thư mục ứng dụng để kiểm tra cấu hình ký và tên gói trong hồ sơ mà không cần mật khẩu. `build -run` chuyển tệp HAP đã ký đó cho `hdc`.

## Ký số

```yaml
sign:
  identity: "Apple Development: you@example.com (ABCDE12345)"  # signs the macOS app, else ad hoc
  profile: assets/AppStore.mobileprovision  # signs the IPA
  keystore: assets/release.p12              # signs APKs, with key_alias
  certificate: assets/codesign.pfx          # signs the Windows app and its installer
  # widgets_profile: assets/Widgets.mobileprovision  # the iOS extension's profile; use a macOS profile for a Mac App Store build
  # share_profile: assets/Share.mobileprovision      # signs the IPA's share extension
  # clip_profile: assets/Clip.mobileprovision        # signs the IPA's App Clip
```

Mật khẩu của keystore được lấy từ `NSGO_KEYSTORE_PASSWORD` và `NSGO_KEY_PASSWORD`. `-team` ký gói dành cho iPhone. `-profile`, `-widgets-profile`, `-share-profile`, `-clip-profile` và `-macos-profile` thay thế lần lượt hồ sơ cấp phép của ứng dụng, tiện ích mở rộng widget, tiện ích mở rộng chia sẻ, App Clip và Mac App Store ngay từ dòng lệnh. `-installer` thay thế danh tính Mac Installer Distribution.

### Ký các gói Windows

Authenticode xác định nhà xuất bản của một bản tải về trên Windows; [SmartScreen kiểm tra uy tín của nó riêng biệt](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation). `sign.certificate` (một tệp `.pfx`) ký tệp thực thi, tệp đồng hành dòng lệnh, các chương trình và thư viện đi kèm trong tài nguyên ứng dụng, trình cài đặt cùng trình gỡ cài đặt của nó, và tệp MSIX được build cho kênh Store; mật khẩu của chứng chỉ lấy từ `NSGO_CERTIFICATE_PASSWORD`. Chữ ký được tạo hoàn toàn bằng Go, giống hệt cách `signtool` tạo, nên máy Mac có thể ký chúng mà không cần Windows SDK:

```yaml
sign:
  certificate: assets/codesign.pfx
  timestamp_url: http://timestamp.digicert.com  # the RFC 3161 server that timestamps them
```

`sign.timestamp_url` mặc định trỏ tới máy chủ của DigiCert, và giá trị `none` sẽ bỏ qua việc đóng dấu thời gian (khiến chữ ký mất hiệu lực khi chứng chỉ hết hạn). Chứng chỉ lưu trên token phần cứng hoặc dịch vụ đám mây có thể ký thông qua một lệnh riêng, trong đó `%1` là tệp cần ký và lệnh sẽ được chạy cho mọi tệp liệt kê ở trên:

```yaml
sign:
  sign_command: "signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a %1"
```

`build` sẽ dừng lại nếu lệnh ký thành công nhưng lại để tệp thực thi ứng dụng, tệp đồng hành dòng lệnh, một tài nguyên PE hoặc tệp thực thi setup không có chữ ký Authenticode nhúng bên trong.

Trình gỡ cài đặt nằm bên trong trình cài đặt được ký trước khi nhúng vào, bằng cùng `sign.certificate` hoặc `sign.sign_command` như ứng dụng và trình cài đặt (setup). `-certificate`, `-sign-command` và `-timestamp-url` cho phép thiết lập các thông số này từ dòng lệnh.

### Công chứng

Các ứng dụng tải về trên máy Mac có thể mở lên mà không bị cảnh báo của Gatekeeper sau khi đã được Apple công chứng (notarize). Hãy ký bằng chứng chỉ Developer ID và chỉ định thông tin xác thực mà `notarytool` lưu trong keychain của bạn:

```sh
xcrun notarytool store-credentials hello --apple-id you@example.com --team-id ABCDE12345
```

```yaml
sign:
  identity: "Developer ID Application: Example Inc (ABCDE12345)"
  notarize: hello   # the keychain profile above; or -notarize hello
```

Khi đó `build` sẽ ký kèm hardened runtime, gửi ảnh đĩa lên, chờ phán quyết của Apple (vài phút), rồi ghim ticket công chứng vào cả ảnh đĩa và ứng dụng; nếu bị từ chối, công cụ in ra nhật ký của Apple. Trong CI, `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` và `NSGO_NOTARY_ISSUER` (tệp `.p8`, ID và issuer của khóa App Store Connect API) sẽ thay thế cho hồ sơ keychain. Hardened runtime mặc định chặn quyền truy cập micro, camera, lịch và vị trí trừ khi ứng dụng khai báo sử dụng chúng — việc mà `build` tự động làm khi `apple.info` có `NSMicrophoneUsageDescription`, `NSCameraUsageDescription`, mô tả sử dụng lịch hoặc lời nhắc, hoặc mô tả sử dụng vị trí.

## Widget và Live Activities

`build` và `dev` tự động biên dịch tiện ích mở rộng vẽ widget và Live Activities của ứng dụng vào các gói macOS và iOS bằng trình biên dịch Swift của Xcode mà không cần bước riêng nào. Trên iOS, ứng dụng và các widget của nó dùng chung một app group: `-run` trên iPhone để Xcode tự ký cả hai và đăng ký nhóm; với tệp IPA, hãy bật nhóm đó cho cả `<id>` và `<id>.widgets`, đồng thời ký tiện ích mở rộng bằng `sign.widgets_profile`. Các tệp APK và AAB độc lập trên Android đã bao gồm các trình cung cấp App Widget và tài nguyên từ `widgets`; Java runtime dựng sẵn tự vẽ dòng thời gian của chúng mà không cần đến SDK hay trình biên dịch Java trong quá trình build ứng dụng. Android 16 hiển thị Live Activities dưới dạng Live Updates và không cần tiện ích mở rộng. [Widget](widgets.md) và [Live Activities](live-activities.md) trình bày chi tiết cấu hình và API của chúng.

## Android App Links

`app_links` liệt kê tường minh các máy chủ HTTPS mà liên kết của chúng sẽ mở trực tiếp ứng dụng Android đầy đủ:

```yaml
app_links: [example.com, shop.example.com]
```

APK và App Bundle khai báo một bộ lọc `ACTION_VIEW` có thể duyệt với `autoVerify` cho từng máy chủ. Hãy dùng tên miền DNS không có scheme, đường dẫn, cổng, ký tự đại diện hoặc địa chỉ IP. Phục vụ tệp `https://<host>/.well-known/assetlinks.json` chứa mã gói của ứng dụng và dấu vân tay chứng chỉ ký; Android sẽ xác minh liên kết đó trước khi định tuyến các đường dẫn về ứng dụng. Công cụ không tự xuất bản các tệp xác minh liên kết này; [hướng dẫn xác minh của Android](https://developer.android.com/training/app-links/verify-applinks) mô tả cách thiết lập chúng. Một ứng dụng chủ Android nhúng tự khai báo các intent filter riêng và chuyển tiếp các liên kết được chấp nhận tới `NsgoView` của nó.

`mobile.OpenedURL(cx)` đọc lượt gọi được chấp nhận trước lần kết xuất gốc đầu tiên và cập nhật khi có liên kết khác mở lại ứng dụng đang chạy. `mobile.OnOpenLinks` vẫn nhận từng liên kết được chấp nhận. Một ứng dụng web toàn trang đọc địa chỉ trang hiện tại qua `mobile.OpenedURL`, bao gồm cả lịch sử trình duyệt và thay đổi phần hash; một ứng dụng gắn bên trong trang khác không đọc được địa chỉ của trang chứa. Hãy sử dụng [`cx.AppURL`](addresses.md) khi ứng dụng cũng cần tự ghi lại địa chỉ của nó.

## App Clip

App Clip là một phần gọn nhẹ của ứng dụng iOS có thể chạy từ một liên kết, mã quét hoặc thẻ NFC mà không cần cài đặt toàn bộ ứng dụng. `app_clip` đóng gói một App Clip vào trong gói iOS, lấy mã nguồn từ một gói `main` riêng biệt để chỉ liên kết đúng những gì nó hiển thị:

```yaml
app_clip:
  main: ./clip              # the clip's main package, the app's own by default
  domains: [example.com]    # whose links open it
  # name: Hello Clip        # "<name> Clip" by default
  # id: dev.example.hello.Clip  # <id>.Clip by default
  # minimum: "17.0"         # 16.0 by default; 17.0 allows 100 MB when only links open it
  # notifications: true     # ask for notifications for a day
  # location: true          # let the clip confirm where it was opened
```

Clip đọc liên kết đã mở nó bằng `mobile.OpenedURL` (giá trị này thay đổi khi một liên kết khác mở lại nó), và `mobile.IsAppClip` cho biết mã có đang chạy dưới dạng App Clip hay không. Hệ thống tải về clip có dung lượng tối đa 15 MB, hoặc 100 MB từ iOS 17 khi chỉ được mở qua liên kết, và `build` sẽ từ chối bản build vượt ngưỡng; một clip liên kết `ui` chiếm khoảng 11 MB. `dev -target ios_simulator -clip https://example.com/order?table=12` chạy riêng clip đó trong trình giả lập và mở từ liên kết kia; `-target ios` chạy nó trên iPhone và in ra mã QR của liên kết: hãy thêm liên kết vào Settings > Developer > Local Experiences, rồi quét mã bằng camera. Dùng `-release` nếu các ký hiệu gỡ lỗi phát triển đẩy dung lượng clip vượt quá giới hạn kích thước QR. Apple chỉ cấp quyền ký App Clip cho các tài khoản nhóm thuộc Apple Developer Program, không áp dụng cho tài khoản cá nhân.

`dev -target ios` và `build -run -target ios` cũng giữ clip nằm bên trong ứng dụng đầy đủ: Xcode ký nó như một target riêng biệt và nhúng vào trước khi ký ứng dụng chính. Chế độ ký tự động đòi hỏi một tài khoản nhóm trả phí đã xác thực trong Xcode, có cấu hình cấp phép cho cả ứng dụng và clip. Để đưa lên App Store, hãy bật App Clips cho `<id>`, ký clip bằng `sign.clip_profile`, và phục vụ tệp `apple-app-site-association` trên các tên miền đã khai báo. Thư mục `examples/appclip` trong kho mã nguồn là ví dụ một quán cà phê cho phép dùng clip để đặt món tại bàn.

Để chuyển giao đơn hàng hoặc dữ liệu cục bộ khác sang cho ứng dụng đầy đủ, hãy thiết lập tường minh một nhóm dùng chung bên cạnh `app_clip`:

```yaml
apple:
  app_group: group.dev.example.hello
```

Khi đó clip và ứng dụng đầy đủ sẽ dùng chung `cx.DataDir()` và `cx.Storage()` trong container của nhóm. Các tệp dưới thư mục dữ liệu và các mục khóa-giá trị mà clip ghi vào đều sẵn sàng khi ứng dụng đầy đủ khởi chạy; `cx.LogsDir()` đi theo thư mục đó, trong khi mỗi gói vẫn giữ riêng bộ nhớ đệm (cache) của mình. Quy trình ký của Xcode yêu cầu quyền nhóm cho cả hai target; khi dùng hồ sơ cấp phép thủ công, hãy bật cùng nhóm cho cả hai App ID và cấp quyền trong `sign.profile` lẫn `sign.clip_profile`. Thiếu quyền cấp sẽ khiến việc ký bị từ chối, và nhóm không truy cập được sẽ làm dừng quá trình khởi động. Thư mục dữ liệu tường minh của ứng dụng chủ vẫn được ưu tiên nếu có.

Nếu không có `apple.app_group`, mỗi gói giữ riêng vùng lưu trữ sandbox của mình. Việc bật thiết lập này không tự động di chuyển dữ liệu đã lưu trước đó trong các sandbox cũ, và các ứng dụng chỉ có widget vẫn giữ nguyên vị trí lưu trữ hiện tại của chúng. Hãy lưu các bí mật dùng chung trong keychain thay vì bộ nhớ thuần. Ví dụ quán cà phê lưu bền vững đơn hàng từ clip và hiển thị lại trong ứng dụng đầy đủ.

Từ một hành động tường minh của người dùng bên trong clip, hãy gọi `mobile.ShowAppClipOverlay(cx, mobile.AppClipOverlayBottom)` để đề nghị StoreKit gợi ý cài đặt ứng dụng đầy đủ tương ứng. `AppClipOverlayBottomRaised` chừa khoảng trống cho thanh dưới cùng. Hàm trả về dùng để ẩn yêu cầu đó; việc dọn dẹp scope cũng tự động ẩn nó, và một yêu cầu cũ không thể ẩn một yêu cầu mới hơn. Gọi hàm này bên ngoài App Clip gốc trên iOS sẽ không làm gì cả. App Store tự quyết định có thể tải và hiển thị đề xuất hay không, nên lệnh gọi không báo cáo liệu đề xuất có hiện ra hay người dùng có cài đặt hay không. Hãy luôn giữ tác vụ của clip dùng được trọn vẹn mà không bắt buộc phải cài ứng dụng đầy đủ. [Hướng dẫn overlay của Apple](https://developer.apple.com/documentation/appclip/recommending-your-app-to-app-clip-users)

## Nhúng vào ứng dụng khác

`-embed` đóng gói ứng dụng cho ứng dụng iOS hoặc Android riêng của bạn: một XCFramework (hoặc gồm các framework động khi kèm `-dynamic`), hoặc một tệp AAR với `NSGoView` hay `NsgoView` để hiển thị ứng dụng. Các chương trình Go trên máy tính để bàn hiển thị ứng dụng bằng `nsgo.NewView`, còn các trang web dùng tệp `nsgo.js` của trang với máy chủ là trang chứa hoặc địa chỉ được chỉ định qua `web.host` trong `nsgo.yaml`. Các ví dụ `examples/embed-*` trong kho mã nguồn minh họa từng trường hợp; [Nhúng](embedding.md) trình bày chi tiết phía ứng dụng chủ.

---
title: Thiết lập môi trường phát triển
description: Build NSGo từ mã nguồn, chạy gallery và kiểm thử thay đổi.
group: Đóng góp
order: 80
---

## Yêu cầu

macOS có Xcode và Go 1.27.3 trở lên có thể build mọi nền tảng đích cho arm64, bao gồm cả APK của Android. Để chạy trên thiết bị thật, cần các thành phần được liệt kê ở mục [Cài đặt](getting-started.md#yêu-cầu); riêng trình giả lập Android x86_64 (`android_x64`) cần có Android NDK, vì Go chỉ liên kết `android/amd64` khi có cgo. Các chương trình cài đặt và gỡ cài đặt Windows được build chỉ bằng Go.

```sh
git clone https://github.com/nsgo-dev/nsgo.git
cd nsgo
```

Hãy đọc [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) và [AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) trước khi sửa mã nguồn.

## Ứng dụng gallery

Gallery có bản minh họa cho từng phần tử, từng thành phần trong `base` và `ui`, cũng như các dịch vụ. Ở cửa sổ rộng, các bản minh họa nằm dưới các thẻ ở thanh bên; ở cửa sổ hẹp, các thẻ và phần cài đặt nằm trên thanh dưới cùng và màn hình minh họa được đẩy lên trên. Chạy gallery từ thư mục gốc của kho lưu trữ:

```sh
go tool nsgo dev examples/gallery                   # nsgo.yaml's dev target, exiting with the app
go tool nsgo dev examples/gallery-tsx               # the gallery's components and apps, written in TSX
go tool nsgo dev -target web examples/gallery       # in the browser, serving until interrupted
go tool nsgo dev -target ios_simulator examples/gallery
go tool nsgo dev -target ios examples/gallery       # on the connected iPhone, signed by Xcode
go tool nsgo dev -target android examples/gallery   # on the connected device or emulator, over adb
go tool nsgo build -target all examples/gallery     # every platform's packages in examples/gallery/dist, what to hand out in dist/releases
```

## Kiểm tra và kiểm thử

```sh
go tool nsgo check                 # vet every package
go tool nsgo check -target all     # vet every available Go/Xcode target instead
go tool nsgo test                  # every package's tests
go tool nsgo test -bench . ./core  # core's benchmarks
(cd benchmark && go run ./runner)  # measure against other frameworks
# capacity curve for NSGo's 600-cell grid, without rebuilding it
(cd benchmark && go run ./runner -frameworks nsgo -cases grid -grid-cols 20 -grid-rows 15 -out /tmp/grid.json)
```

`-target` kiểm tra (`vet`) các nền tảng đích được chỉ định thay vì máy chủ hiện tại và, trong kho mã nguồn của NSGo, kiểm tra cả dev client mà lệnh `check` thông thường kiểm tra. Để kiểm tra các nền tảng mà thay đổi của bạn chạm tới, hãy liệt kê tất cả trong một lệnh `check`, chẳng hạn `check -target web,android -jobs 2`, thay vì chạy nhiều lệnh riêng lẻ: `-jobs n` kiểm tra tối đa `n` nền tảng đích cùng lúc, mỗi `go vet` dùng `n` worker biên dịch (mặc định là một nền tảng một lần theo cấu hình mặc định của Go), và một lệnh `check` được khởi chạy khi lệnh khác đang chạy trong cùng module sẽ phải chờ và thông báo điều đó. Workflow `check.yml` trên CI kiểm tra mọi nền tảng đích.

Khi chạy ở thư mục gốc của kho mã nguồn hoặc trong `internal/dev`, lệnh chạy kiểm thử headless sẽ kiểm thử `internal/dev` lần thứ hai với thẻ build `nsgodev` mà các tệp của dev client cần, nhờ đó các bài kiểm thử của gói này chạy cả ở máy cục bộ lẫn trên CI. Cả hai lượt chạy đều nhận cùng bộ lọc và cờ, dùng chung một bảng tổng hợp kết quả và báo cáo JUnit, trong đó tên gói của lượt chạy thứ hai là `internal/dev [nsgodev]`.

`check -target all` kiểm tra các nền tảng đích macOS, Windows, Linux, Android arm64 và web trên bất kỳ máy chủ nào. Lệnh này cũng kiểm tra iOS, tvOS và watchOS khi có sẵn các Xcode SDK tương ứng, và liệt kê những nền tảng bị bỏ qua. Nếu một nền tảng đích được nêu tên tường minh, chẳng hạn `check -target ios`, mà thiếu công cụ thì lệnh sẽ báo lỗi, kể cả khi dùng chung với `all`. Một nền tảng bị lỗi không làm gián đoạn việc kiểm tra các nền tảng còn lại, và lệnh sẽ thất bại nếu bất kỳ bước kiểm tra bắt buộc nào thất bại. `build -target all` vẫn yêu cầu macOS có Xcode. Trong chính kho mã nguồn của NSGo, mỗi nền tảng đích được chỉ định cũng kiểm tra các máy chủ kiểm thử nền tảng của nó với `nsgotest`; CI chỉ định tường minh nền tảng thử nghiệm `harmonyos`, vì nó nằm ngoài `all`.

Tác vụ CI trên Linux chạy các bài kiểm thử `js/wasm` cho backend Go của HarmonyOS bằng Node.js và trình chạy Wasm của Go. Bạn có thể chạy bài kiểm thử gallery cục bộ với trình giả lập HarmonyOS kết nối qua `hdc`. Việc biên dịch các [mẫu máy chủ HarmonyOS](building.md#harmonyos) yêu cầu OpenHarmony SDK API 26.

```sh
go tool nsgo test -env GOOS=js -env GOARCH=wasm ./internal/platform/harmonyos \
  -- "-exec=env -i \"PATH=$PATH\" \"$(go env GOROOT)/lib/wasm/go_js_wasm_exec\"" -count=1
go tool nsgo test -target harmonyos -run TestHarmonyOSPages -timeout 10m ./examples/gallery
```

Trong phiên Wayland trên máy tính để bàn Linux, chạy bài kiểm thử nghiệm thu cổng phím tắt toàn cục thực tế với `NSGO_TEST_GLOBAL_SHORTCUT_PORTAL=<installed-desktop-id> go tool nsgo test -run TestGlobalShortcutPortalWithPermission ./internal/platform/linux`. ID phải có một mục `.desktop`. Hãy cấp quyền `Ctrl+Alt+K` trong hộp thoại của compositor, sau đó nhấn tổ hợp phím này khi đang chọn một ứng dụng khác; bài kiểm thử sẽ chờ cả quá trình đăng ký lẫn kích hoạt. Bài kiểm thử bị bỏ qua trừ khi biến môi trường này được đặt.

Khi có sẵn Docker trên máy chủ Linux, lệnh `go tool nsgo test -run '^TestLinuxPackagesNative$' -env NSGO_TEST_LINUX_PACKAGES=1 ./cmd/nsgo -- -timeout=20m` sẽ kiểm tra việc cài đặt thực tế trong các container Debian, Ubuntu và Arch dùng một lần. Lệnh tải về các image chính thức của chúng và cài đặt các thư viện phụ thuộc GTK bên trong, xác minh cơ sở dữ liệu gói cùng các tệp đã cài, rồi gỡ bỏ gói. Cơ sở dữ liệu gói của máy chủ không bao giờ bị đụng tới.

## Phát hành

NSGo phát hành theo nhu cầu khi một dự án phụ thuộc cần một phiên bản gắn thẻ (tag) hoặc khi các maintainer có những thay đổi sẵn sàng phát hành. Khi yêu cầu phát hành cho một thư viện, hãy dẫn liên kết tới công việc phụ thuộc đó và nêu rõ commit chứa thay đổi NSGo cuối cùng mà nó cần. Thẻ phát hành đầu tiên chứa commit đó chính là phiên bản mà thư viện có thể yêu cầu.

Để kiểm thử một thay đổi đã gộp trước khi nó được gắn thẻ, hãy chạy `go get github.com/nsgo-dev/nsgo@main` từ module phụ thuộc. Bạn có thể dùng mã commit thay cho `main` để chọn đúng bản sửa đổi đã kiểm chứng. Go sẽ ghi lại phiên bản giả (pseudo-version) tương ứng vào `go.mod`.

Một maintainer sẽ gán phiên bản và ngày phát hành cho các thay đổi trong `CHANGELOG.md`, để lại một mục Unreleased mới, rồi gắn thẻ lên commit tương ứng trên `main`. [Workflow phát hành](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/release.yml) sẽ kiểm tra thẻ và xuất bản sau khi workflow kiểm thử chạy thành công; ghi chú phát hành khi đó sẽ rời khỏi `CHANGELOG.md` — nơi chỉ giữ các thay đổi chưa phát hành — để chuyển sang mục [Releases](https://github.com/nsgo-dev/nsgo/releases).

## NSH

NSGo đọc [Native Shell](https://github.com/longbridge/nsh) như một tài liệu tham khảo về thiết kế; không có bài kiểm thử nào cần đến nó. Để đọc, hãy clone nó bên cạnh kho lưu trữ này và checkout thẻ phát hành mới nhất của nó, tuyệt đối không dùng `main`:

```sh
git clone https://github.com/longbridge/nsh.git ../nsh
git -C ../nsh checkout "$(git -C ../nsh describe --tags --abbrev=0)"
```

## Tiếp theo

- [Cấu trúc mã nguồn](contributing/codebase.md): nơi đặt từng thành phần của NSGo.
- [Thực hiện thay đổi](contributing/changes.md): những gì một pull request cần có.

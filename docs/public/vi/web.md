---
title: Xuất bản trên web
description: Xuất bản một ứng dụng NSGo với ngữ nghĩa tài liệu, siêu dữ liệu trang và tài nguyên qua CDN.
group: Bộ công cụ
order: 24
---

## Build ứng dụng

```sh
go tool nsgo build -target web
```

Phục vụ thư mục được tạo ra qua HTTP. Các tài nguyên bản phát hành có mã băm nội dung trong tên, nên hãy lưu cache chúng như dữ liệu bất biến; xác thực lại `index.html`, `nsgo.json` và `manifest.webmanifest`. Ứng dụng web giữ nguyên hành vi bố cục, cuộn, tiêu điểm và con trỏ của NSGo.

## Tiêu đề và liên kết

Một `core.View` hoặc `core.Text` có `Role: core.RoleHeader` trở thành `h1` đến `h6`, được chọn theo `AccessibilityLevel`; nếu không có cấp, nó là `h2`. Các tiêu đề Markdown từ `base/textview` mang theo cấp của chúng. Lề và phông chữ tiêu đề của trình duyệt không làm thay đổi kiểu dáng của ứng dụng.

`base.Link` và `ui.Link` xuất bản `Href` của chúng dưới dạng một thẻ neo. Một cú nhấp thông thường chạy `OnOpen` và `OnPress` của ứng dụng; nhấp chuột giữa hoặc nhấp kèm phím bổ trợ sẽ dùng hành động liên kết của trình duyệt. Trình duyệt cung cấp menu ngữ cảnh liên kết của nó trừ khi ứng dụng xử lý `OnContextMenu`. Một liên kết bị vô hiệu hóa không có địa chỉ hoạt động. Với một view tùy chỉnh có vai trò liên kết, đặt `core.View.Web` thành `core.WebViewProps{Href: "/notes"}` hoặc gắn `HrefFunc`. TSX dùng `web={{href: "/notes"}}`; tiêu đề dùng `accessibilityLevel={1}`. Các lược đồ URL có thể thực thi không an toàn bị từ chối. Nội dung bị giới hạn giữ điều hướng trong các trình xử lý nhấn trong phạm vi của nó và không công bố địa chỉ trình duyệt nào.

## Siêu dữ liệu trang

Import `github.com/nsgo-dev/nsgo/web` và đặt một `web.Head` bên cạnh nội dung của trang:

```go
core.Group{
	web.Head{Title: "Notes", Description: "Your saved notes"},
	pageContent,
}
```

`TitleFunc` và `DescriptionFunc` bám theo signal. Các head hiển thị sau sẽ ghi đè các trường chúng đặt và kế thừa các trường còn lại. Ẩn hoặc gỡ bỏ một trang sẽ khôi phục các head trước đó; gỡ bỏ mọi head sẽ khôi phục tài liệu ban đầu. Một trường tĩnh rỗng sẽ kế thừa; một hàm trả về chuỗi rỗng sẽ xóa giá trị đó. Một head không thêm nút bố cục nào. Các nền tảng gốc, ứng dụng nhúng và nội dung bị giới hạn không đụng tới tài liệu của trình chủ.

Trong TSX, import `Head` từ `nsgo/web` và viết `<Head title={title()} description="Your saved notes" />`. Để có bản xem trước HTML tĩnh, hãy prerender từng URL như một phần của quá trình triển khai trang web; `Head` cập nhật tài liệu đang chạy.

## Phục vụ tài nguyên từ CDN

Trình tải phân giải mọi tệp của ứng dụng tương đối so với `nsgo.json`. Một trang có thể tải ứng dụng từ một origin khác:

```js
const url = new URL("https://cdn.example.com/notes/nsgo.json");
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url).href);
await mount(null, url.href, { config });
```

Cho phép origin của trang tải các tệp của CDN bằng CORS, và phục vụ WebAssembly với kiểu `application/wasm`. Phục vụ HTML điểm vào của ứng dụng cho mọi route sâu mà ứng dụng của bạn dùng. [Nhúng ứng dụng](embedding.md#web) cho thấy cách gắn vào một phần tử thay vì vậy.

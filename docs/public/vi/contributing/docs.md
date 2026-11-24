---
title: Tài liệu và trang web
description: Viết một trang tài liệu, dịch nó, chạy trang web và giữ cho skill dành cho agent luôn cập nhật.
group: Đóng góp
order: 83
---

## Một trang tài liệu

Các tài liệu này là những tệp Markdown nằm trong `docs/public/`, có thể đọc trực tiếp trên GitHub. `en/` và `zh/` đối xứng nhau theo từng đường dẫn; một trang chưa có bản tiếng Trung sẽ hiển thị bản tiếng Anh kèm theo thông báo. Mỗi trang bắt đầu bằng phần front matter:

```yaml
---
title: Testing
description: Test an app the way a user uses it, headless or in a native window.
group: Toolchain
order: 21
---
```

- `group` là tên nhóm của trang trên thanh bên, viết bằng ngôn ngữ của trang đó, còn `order` xác định thứ tự sắp xếp. Các nhóm đi theo hành trình của người đọc: hướng dẫn thực hành, bộ công cụ, thành phần, khái niệm cốt lõi, sau đó đến viết thành phần và đóng góp. Một trang không có `group`, chẳng hạn như trang giới thiệu, sẽ đứng trước các nhóm đó.
- Bố cục trang đã hiển thị sẵn `title` và `description`, nên phần nội dung chính bắt đầu từ `##`.

Hãy liên kết giữa các trang bằng đường dẫn `.md` tương đối (`[Testing](testing.md#run-on-a-platform)`); trang web sẽ tự chuyển chúng thành URL tương ứng. Mỗi trang trả lời một câu hỏi cụ thể; phần giải thích cách NSGo hoạt động bên trong được đặt ở `docs/architecture/`.

## Trang web

Trang web trong `website/` được build bằng [nsgo-website](https://github.com/nsgo-dev/nsgo-website), một integration của Astro, từ các thiết lập trong `website/site.config.ts`. Nó yêu cầu [Bun](https://bun.com) 1.4 trở lên:

```sh
cd website
bun install
bun run dev    # at http://localhost:4321/
bun run build  # type-check and build, as the deployment does
```

`website/README.md` giải thích cách trang web được build và xuất bản.

## Dành cho agent

Trang web cũng xuất bản các trang tiếng Anh dành cho mô hình ngôn ngữ: `/llms.txt` liệt kê chúng, `/llms-full.txt` chứa toàn bộ nội dung và `/docs/<page>.md` chứa từng trang riêng lẻ với liên kết tuyệt đối và bỏ qua các đánh dấu của khối mã (`src/lib/llms.ts`). Chúng tự động cập nhật theo các trang tài liệu mà không cần bước xử lý riêng.

`skills/nsgo/` là skill mà các agent cài đặt qua `bunx skills add nsgo-dev/nsgo`: gồm `SKILL.md` chứa các quy tắc mà ứng dụng tuân theo, và thư mục `references/` để đọc khi tác vụ cần đến. Hãy cập nhật skill này trong cùng một thay đổi với tài liệu khi có thay đổi về cách viết ứng dụng, các lệnh của công cụ CLI, kiểm thử hoặc đóng gói; giữ nội dung chỉ gồm những gì agent bắt buộc phải tuân theo và gắn liên kết tới tài liệu cho phần còn lại. `go tool nsgo test -run TestSkills .` kiểm tra phần front matter, các liên kết và mọi tên của NSGo được nhắc tới trong đó.

---
title: Giao diện và chủ đề
description: Lấy màu sắc từ chủ đề đang áp dụng, và bổ sung các biến thể cùng kích thước cho thành phần.
group: Viết thành phần
order: 72
---

Một thành phần có kiểu dáng chỉ chọn kiểu dáng chứ không làm gì khác: nó bọc lấy một thành phần không kiểu dáng trong một gói riêng như `chipsui`, và giữ phần hành vi nằm tại gói gốc đó. Màu sắc của nó lấy từ chủ đề, nhờ vậy thành phần tự thích ứng theo chủ đề của ứng dụng, chủ đề riêng của một bảng điều khiển và giao diện sáng hoặc tối mà không cần xây dựng lại.

## Màu sắc từ chủ đề

Hãy đọc màu sắc bên trong `StyleFunc`: hàm này tự động chạy lại khi bất kỳ giá trị nào nó đọc thay đổi, nên việc đổi chủ đề hoặc giao diện sáng/tối sẽ tạo kiểu lại cho thành phần ngay tại chỗ. `cx.Palette()` trả về bảng màu đang áp dụng tại vị trí của thành phần, còn `ui.ColorsOf(cx)` định danh các vai trò màu mà `ui` sử dụng.

```go
// Toggle is chips.Toggle in the theme's colors.
type Toggle struct {
	On       func() bool
	OnChange func(bool)
	Label    string
}

func (t Toggle) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	on := func() bool { return t.On != nil && t.On() }
	return chips.Toggle{On: t.On, OnChange: t.OnChange, Label: t.Label,
		Class: "flex-row items-center gap-2 rounded-full px-3 py-1",
		StyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.Accent()}
			}
			return core.Style{Background: colors.Inset()}
		},
		DotClass: "size-2 rounded-full",
		DotStyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.OnAccent()}
			}
			return core.Style{Background: colors.Faint()}
		},
		Children: []core.Node{ui.Text(t.Label, 13, ui.Medium, colors.Text)},
	}
}
```

Mã màu cứng sẽ bị hỏng ở chế độ tối và dưới chủ đề tùy biến của ứng dụng; hãy lấy mọi màu sắc từ một vai trò màu.

## Vai trò màu của riêng bạn

Một màu mà bảng màu chưa có vai trò tương ứng sẽ được đặt vào `Palette.Colors` dưới một tên do bạn chọn, thiết lập cho cả hai chế độ sáng và tối. Ứng dụng có thể ghi đè màu đó bằng mã nguồn hoặc trong tệp chủ đề của họ, dưới mục `light` và `dark`:

```go
theme := core.DefaultTheme()
theme.Light.SetColor("chip", core.RGB(0xeef5ff))
theme.Dark.SetColor("chip", core.RGB(0x1b2a3d))
cx.SetTheme(theme)
```

Thành phần đọc màu đó bằng `cx.Palette().Color("chip")`; phương thức này cho biết chủ đề có định nghĩa màu đó hay không, để thành phần có thể lùi về một vai trò màu mặc định khi chưa có. Dải chuyển sắc (gradient), đổ bóng và các hiệu ứng hoàn thiện khác không phải là màu sắc: hãy giữ chúng trong mã nguồn và chọn theo `cx.Dark()`.

## Biến thể

Một thành phần có nhiều diện mạo khác nhau sẽ nhận `Variant`, một kiểu riêng có giá trị rỗng (zero value) là diện mạo mặc định. Kiểu này chỉ đặt tên cho những diện mạo mà thành phần tự vẽ — bao gồm cả màu sắc của nó — thay vì dùng các trường boolean rời rạc hoặc một kiểu màu dùng chung:

```go
// ToggleVariant is a toggle's look.
type ToggleVariant uint8

const (
	// ToggleSolid fills the pill with the accent when on.
	ToggleSolid ToggleVariant = iota
	// ToggleOutline draws its edge in the accent instead.
	ToggleOutline
)
```

## Kích thước

Hầu hết các thành phần chỉ cần một kích thước duy nhất, giúp các điều khiển thẳng hàng với nhau mà ứng dụng không phải lựa chọn. Thành phần nào cần nhiều kích thước hơn sẽ nhận `Size`, một giá trị `ui.Size` gồm mặc định, nhỏ hoặc lớn, khớp với chiều cao 28, 24 và 32 point của `ui.Button`; `Size.Height()` trả về chiều cao để căn chỉnh các điều khiển khác cho thẳng hàng. Một thành phần giữ nguyên một hình dạng ở mọi tỉ lệ, chẳng hạn như ảnh đại diện (avatar), thì nhận `Size` tính bằng point.

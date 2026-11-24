---
title: Tampilan dan tema
description: "Mengambil warna dari tema yang berlaku, serta memberi komponen varian dan ukuran."
group: Menulis komponen
order: 72
---

Komponen bergaya memilih gaya dan tidak melakukan hal lain: ia membungkus komponen tanpa gaya, di dalam paket tersendiri seperti `chipsui`, dan mempertahankan perilakunya di sana. Warnanya berasal dari tema, sehingga ia mengikuti tema aplikasi, tema panel itu sendiri, dan tampilan terang maupun gelap tanpa perlu dibuat ulang.

## Warna dari tema

Baca warna di dalam `StyleFunc`: fungsi ini dijalankan ulang ketika apa yang dibacanya berubah, sehingga perubahan tema atau tampilan menata ulang komponen di tempat. `cx.Palette()` memberikan palet yang ditampilkan di posisi komponen berada, dan `ui.ColorsOf(cx)` menamai peran-peran warna yang digunakan `ui`.

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

Warna yang di-hardcode akan rusak di mode gelap dan di bawah tema milik aplikasi; ambil setiap warna dari sebuah peran warna.

## Peran warna milik Anda sendiri

Warna yang tidak memiliki peran di palet dimasukkan ke `Palette.Colors` dengan nama Anda sendiri, yang diatur untuk kedua tampilan (terang dan gelap). Aplikasi menimpanya di dalam kode atau di file temanya, di bawah `light` dan `dark`:

```go
theme := core.DefaultTheme()
theme.Light.SetColor("chip", core.RGB(0xeef5ff))
theme.Dark.SetColor("chip", core.RGB(0x1b2a3d))
cx.SetTheme(theme)
```

Komponen membacanya dengan `cx.Palette().Color("chip")`, yang melaporkan apakah tema memilikinya, sehingga komponen dapat kembali ke peran warna standar jika tidak ada. Gradien, bayangan, dan sentuhan akhir lainnya bukanlah warna: simpan di dalam kode dan pilih berdasarkan `cx.Dark()`.

## Varian

Komponen dengan beberapa tampilan menerima `Variant`, yaitu tipe tersendiri yang nilai nolnya adalah tampilan bawaan. Tipe ini hanya menamai tampilan-tampilan yang digambar komponen—termasuk warnanya—alih-alih memakai field boolean atau tipe warna bersama:

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

## Ukuran

Sebagian besar komponen hanya memerlukan satu ukuran, yang menjaga kontrol tetap sejajar tanpa mengharuskan aplikasi memilih. Komponen yang membutuhkan lebih dari satu menerima `Size`, berupa `ui.Size` bernilai bawaan (*default*), kecil (*small*), atau besar (*large*), agar cocok dengan ukuran 28, 24, dan 32 poin milik `ui.Button`; `Size.Height()` memberikan tinggi yang digunakan untuk menyelaraskan kontrol lain. Komponen yang selalu memiliki satu bentuk pada skala apa pun, seperti avatar, menerima `Size` dalam satuan poin.

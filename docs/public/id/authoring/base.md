---
title: Komponen tanpa gaya
description: "Kontrol milik NSGo sendiri tanpa tampilan, untuk diberi gaya bagian demi bagian atau dijadikan dasar pustaka bergaya."
group: Menulis komponen
order: 73
---

`base` menyimpan semua kontrol yang dimiliki `ui`, lengkap dengan perilaku, status, dan aksesibilitasnya, tetapi tanpa tampilan. Setiap bagian menerima `Class`, `Style`, dan `StyleFunc` ([Bagian dan status](parts.md)), sehingga pustaka atau aplikasi dengan desainnya sendiri dapat menggunakannya alih-alih `ui`, atau membungkusnya seperti yang dilakukan `ui` ([Tampilan dan tema](looks.md)). Halaman ini membahas kontrol-kontrol dalam [referensi komponen](../ui.md); galeri menampilkan masing-masing berdampingan dengan bentuk `ui`-nya.

Kontrol seperti `Button`, `Checkbox`, `Select`, `Slider`, dan `NumberInput` menerima `Invalid: rejected.Get` untuk mengikuti sinyal validasi. Beri gaya pada status tersebut dengan `Class: "aria-invalid:border-red-500"` atau `Style.AriaInvalid`. Properti ini memperbarui aksesibilitas dan gaya tanpa menonaktifkan kontrol. Kolom komposit meneruskannya ke input asli dan bagian dekoratifnya; `ColorPicker` membagikannya ke kontrol-kontrolnya.

Untuk nilai aplikasi yang diseret antar-tampilan, gunakan [`base/dragdrop`](../drag-drop.md): suatu area menghubungkan sumber dan target sementara aplikasi menyediakan konten, pratinjau, dan tampilannya.

## Button

`base.Button` mengambil tampilannya dari `Style` dan kontennya dari `Children`. `OnPress` berjalan saat penekanan selesai; `OnPressDown` dan `OnPressUp` mengikuti awal dan akhir penekanan untuk umpan balik Anda sendiri.

```go
base.Button{
	Label:   "Save",
	OnPress: func(core.PressEvent) { save() },
	Style: core.Style{Background: core.RGB(0x007aff), Padding: core.Symmetric(8, 14), CornerRadius: 8, AlignSelf: core.AlignStart,
		Active: &core.Style{Opacity: core.Alpha(0.85)}},
	Children: []core.Node{core.Text{Content: "Save", Style: core.Style{Color: core.RGB(0xffffff)}}},
}
```

`Disabled`, `Busy`, `Selected`, dan `Expanded` melaporkan status ke aplikasi, gaya, dan teknologi asistif: aksi yang masih berjalan menggunakan `Busy` (diberi gaya dengan `aria-busy:`), tombol toggle atau item aktif dalam daftar menggunakan `Selected`, dan tombol penyingkap (*disclosure*) menggunakan `Expanded`. `NoFocus` mengeluarkan tombol dari urutan hentian Tab apabila baris atau daftar sudah menjangkaunya dengan cara lain.

## Checkbox

`base.Checkbox` menerima gaya untuk kotak, gaya untuk indikator, dan tanda Anda di dalamnya, yang ditampilkan berdasarkan nilai milik aplikasi.

```go
base.Checkbox{
	Checked:        done.Get,
	OnChange:       done.Set,
	Label:          "Remember me",
	Style:          core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	IndicatorStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), BorderWidth: 1, CornerRadius: 5},
	Indicator:      core.Icon{SVG: lucide.Check(), Style: core.Style{FontSize: 14}},
	Children:       []core.Node{core.Text{Content: "Remember me"}},
}
```

Indikator mengambil statusnya dari kotak melalui `PartStates`, sehingga kelas `checked:` memberi gaya padanya tanpa menjadikannya hentian fokus kedua.

Untuk tombol pengubah (*modifiers*), atur `OnChangeEvent func(checked bool, press core.PressEvent)`. Ini menggantikan `OnChange` dan menyerahkan nilainya kepada aplikasi; `press.Modifiers.Shift` memungkinkan daftar memilih suatu rentang. Aktivasi dengan Space/Return di desktop, web, Android, dan iOS/tvOS tetap mempertahankan tombol pengubah kecuali pintasan aplikasi menangani tombol tersebut lebih dulu; di Windows, Alt bersama Space atau Return akan membuka menu jendela. Di TSX, `onChangeEvent` menerima `{ checked, press }`.

## Switch

`base.Switch` terdiri atas jalur (*track*), kenop (*thumb*) di dalamnya, lalu anak-anaknya sebagai label. Penekanan melaporkan nilai yang diminta, yaitu kebalikan dari `Checked`.

```go
base.Switch{
	Checked:    dark.Get,
	OnChange:   dark.Set,
	Label:      "Dark appearance",
	Style:      core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	TrackStyle: core.Style{Width: core.Pt(38), Height: core.Pt(22), CornerRadius: 11},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

Jalur dan kenop menerima `StyleFunc` masing-masing, dan gaya statusnya mengikuti `Checked`.

## Slider

`base.Slider` menerima `base.SliderValue`: `base.SingleValue(v)` atau `base.RangeValue(low, high)`. Komponen ini memberi gaya pada jalur, indikator, rentang, dan kenop secara terpisah.

```go
level := core.NewSignal(cx, 3.0)
base.Slider{
	Value:      func() base.SliderValue { return base.SingleValue(level.Get()) },
	OnChange:   func(v base.SliderValue) { level.Set(v.End) },
	Min:        func() float64 { return 0 },
	Max:        func() float64 { return 10 },
	Step:       func() float64 { return 1 },
	Label:      "Level",
	Style:      core.Style{Height: core.Pt(28)},
	TrackStyle: core.Style{Height: core.Pt(4), CornerRadius: 2},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

`Min`, `Max`, dan `Step` adalah getter reaktif: mengubah sinyalnya memperbarui slider yang terpasang beserta rentang aksesibelnya, termasuk saat sedang diseret. `Axis` menjadikannya vertikal dan `Inverted` meletakkan nilai minimumnya di ujung berlawanan, seperti halnya `base.Progress`; `Marks` menempatkan tanda skala Anda sendiri pada indikator, dan `OnChangeEnd` melaporkan nilai setelah seretan berhenti.

## Select

`base.Select` menerima `base.SelectItem` dan nilai `*string`, sehingga "tidak ada" adalah `nil`. Anda memberi gaya pada pemicu, popup, baris, dan grupnya.

```go
choice := core.NewSignal[*string](cx, nil)
base.Select{
	Items: func() []base.SelectItem {
		return []base.SelectItem{{Value: "s", Title: "Small"}, {Value: "m", Title: "Medium"}}
	},
	Value:      choice.Get,
	OnChange:   choice.Set,
	Label:      "Size",
	Style:      core.Style{Height: core.Pt(28), Padding: core.Symmetric(4, 10), BorderWidth: 1, CornerRadius: 6},
	PopupStyle: core.Style{Padding: core.All(4), CornerRadius: 8},
	ItemStyle:  core.Style{Padding: core.Symmetric(4, 8), CornerRadius: 4},
}
```

Item yang berdekatan dengan `Group` yang sama berbagi satu tajuk, pengetikan cepat (*typeahead*) mencocokkan judul item, `SearchField` menggambar kolom pencarian Anda sendiri, dan `Cleanable` menampilkan tombol bersihkan. `OnCancel` mendengar Escape, tombol kembali, dan penekanan di luar area.

## Tabs

`base.Tabs` menampung `base.Tab` dan menggambar indikator dari `IndicatorStyle`; aplikasi menyimpan tab mana yang dipilih. `base.Tab.OnPress` berjalan pada setiap penekanan, termasuk pada tab yang sedang dipilih.

```go
base.Tabs{Label: "Sections", IndicatorStyle: core.Style{Height: core.Pt(2), Background: core.RGB(0x007aff)}, Children: []core.Node{
	base.Tab{Label: "Overview", Selected: func() bool { return section.Get() == "overview" },
		OnPress: func(core.PressEvent) { section.Set("overview") }, Children: []core.Node{core.Text{Content: "Overview"}}},
	base.Tab{Label: "Activity", Selected: func() bool { return section.Get() == "activity" },
		OnPress: func(core.PressEvent) { section.Set("activity") }, Children: []core.Node{core.Text{Content: "Activity"}}},
}}
```

## Dialog

`base.Dialog` menerima gaya latar belakang (*backdrop*), lapisan, dan popup, serta `Animation` atau `TransitionShader`, dan anak-anak Anda. `Open`, `DefaultOpen`, dan `OnOpenChange` mengendalikannya; `OnOpen`, `OnClose`, dan `OnCancel` melaporkan apa yang terjadi. `OnConfirm` adalah aksi bawaan: Return menjalankannya—kecuali fokus papan ketik berada pada tombol atau kolom yang menangani Return sendiri atau ada modal lain yang menutupi dialog—lalu dialog ditutup dengan `ReasonSelect`. `Role` dan `AccessibilityDescription` adalah yang diumumkan pembaca layar, dengan `RoleAlertDialog` untuk konfirmasi. `NoCloseOnSwipe` mencegah gestur usap iOS menutupnya, `Presentation` memilih gaya presentasi iOS, dan `StatusBar` mengubah bilah status selama dialog terbuka.

`base.Sheet` adalah panel di sisi mana pun, dengan latar belakang (*backdrop*) dan konten Anda; dengan `CloseOnSwipe`, bottom sheet mengikuti seretan ke bawah dan menutup jika melewati ambang batas. Komponen ini menerima `OnConfirm`, `Role`, dan `AccessibilityDescription` seperti `base.Dialog`. `base.ShowAlertDialog` menampilkan peringatan atau action sheet sistem dengan tombol sesuai perannya:

```go
base.ShowAlertDialog(cx, base.AlertDialog{
	Title: "Delete the note?",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(label string) {
	if label == "Delete" {
		remove()
	}
})
```

## Menu

`base.Menu` membuka `base.MenuItem` dari pemicu Anda sendiri, dan `base.ContextMenu` di posisi pointer. Sebuah item melaporkan `OnSelect` dan menutup menu kecuali jika `NoCloseOnSelect` disetel; `Checked` menjadikannya item yang dapat dicentang, `Indicator` menggambar tanda Anda sebelum kontennya, dan `Role` memilih apa yang didengar teknologi asistif: item menu, kotak centang, atau item radio. `base.Submenu` menampung item tambahan.

```go
base.Menu{Trigger: core.Text{Content: "Actions"}, Children: []core.Node{
	base.MenuItem{Label: "Show sidebar", Checked: sidebar.Get, OnSelect: func() { sidebar.Set(!sidebar.Get()) }},
	base.Submenu{Label: "Share", Children: []core.Node{
		base.MenuItem{Label: "Copy link", OnSelect: copyLink},
		base.MenuItem{Label: "Email", OnSelect: emailLink},
	}},
}}
```

`Placement`, `Align`, `Offset`, dan `Margin` memosisikan popup di desktop, sedangkan `PopupStyle` dan `BackdropStyle` memberi gaya padanya.

## EmojiPicker

`base.EmojiPicker` mencari setiap emoji Unicode berdasarkan nama CLDR yang terlokalisasi, kata kunci, dan shortcode dalam bahasa aplikasi (atau `Languages` / `LanguagesFunc` eksplisit), lengkap dengan tab kategori, entri `Recent` opsional, pemilihan warna kulit, navigasi papan ketik, dan grid tervirtualisasi. Semua label bawaan dan nama kategori berasal dari `base/locales`, dan dataset emoji hanya ditautkan ke aplikasi yang menggunakan `EmojiPicker` atau pembantunya (`base.SearchEmojis`, `base.EmojisInCategory`, `base.EmojiByString`).

```go
base.EmojiPicker{
	Recent:   recent.Get,
	OnSelect: func(e base.Emoji) { insert(e.Emoji) },
	Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
}
```

`Category` dan `DefaultCategory` memilih grup penjelajahan. Tab kategori dan pilihan warna kulit masing-masing merupakan satu hentian Tab dengan navigasi tombol panah. `EmojiPickerRef` menyediakan `Focus`, `Query`, `SetQuery`, `SetCategory`, dan `MatchedCount`; pemilih bergaya menggunakan status kueri yang sama. `LabelFunc`, `SearchLabelFunc`, dan `PlaceholderFunc` memperbarui teks di tempat. `Emoji.WithSkinTone` menerapkan warna kulit yang dipilih ke kedua orang dalam pasangan, dan warna bawaan memulihkan bentuk netralnya; pencarian dengan sebuah emoji mempertahankan urutan persisnya.

Gunakan `ui.EmojiPicker` untuk pemilih bertema dengan ikon kategori Lucide, kolom pencarian yang dapat dibersihkan, pratinjau emoji yang disorot, dan sampel warna kulit.

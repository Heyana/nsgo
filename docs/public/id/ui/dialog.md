---
title: Dialog
description: Permukaan modal, sheet, dan peringatan.
group: Komponen
order: 41
---

`ui.Dialog` adalah permukaan modal di atas aplikasi yang diredupkan, berupa sheet dari bawah di layar sentuh. `ui.Sheet` adalah panel dari bawah, dan `ui.AlertDialog` meminta pengguna untuk mengonfirmasi.

```go preview=dialog/basic height=360
open := core.NewSignal(cx, false)
ui.Dialog{
	Open:         open.Get,
	OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
	Title:        "Rename the file",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { open.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

## Membuka dan menutup

Aplikasi menyimpan apakah dialog terbuka. `OnOpenChange` menerima setiap perubahan yang diminta dialog, dengan `base.Reason` yang membedakan tekanan di luar dari Escape atau kembali; `OnClose` berjalan setelah dialog tertutup. `NoCloseOnEscape` dan `NoCloseOnBackdropPress` mencegah Escape dan tekanan di luar menutupnya.

`Title` dan `Description` mengepalai permukaan, `TitleFunc` dan `DescriptionFunc` ketika keduanya berubah saat dialog terbuka, dan `Footer` menampung tombol-tombolnya. `Bare` menyerahkan seluruh permukaan kepada `Children`, untuk dialog dengan tata letak milik aplikasi sendiri, yang di desktop dipotong di dalam tepi permukaan mengikuti sudut yang diberikan `Style` dan `StyleFunc`, dan `Inset` mengatur jarak terkecilnya dari tepi jendela di desktop.

Pada sheet sentuh, `Bare` kini menghapus grabber dan padding konten bawaan sehingga `Children` mengisi panel. `NoGrabber` hanya menyembunyikan grabber, `ContentStyle` dan `ContentStyleFunc` menyesuaikan pembungkus konten, dan `TitleStyle` dan `TitleStyleFunc` menyesuaikan judul. `Leading` dan `Trailing` menempatkan node, seperti tombol Cancel dan Done, di ujung-ujung baris judul, dengan judul berada di tengah di antara keduanya. `StartingStyle` dan `EndingStyle` menyesuaikan transisi sheet; sertakan `Transition` di dalamnya ketika Anda membutuhkan pengaturan waktu khusus.

`Style` digabungkan di atas tampilan permukaan. `StyleFunc` digabungkan setelahnya dan mengikuti signal yang dibacanya, termasuk tampilan (appearance), selama dialog terbuka dan ketika dibuka kembali. Misalnya, `StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} }` mengikuti tema, dengan `colors := ui.ColorsOf(cx)`. Kedua field ini juga berlaku pada `ui.AlertDialog`. Warna nol membiarkan warna yang ada tetap di tempatnya; gunakan warna transparan yang bukan nol seperti `core.RGBA(0xFFFFFF, 0)` untuk menghapusnya.

## Sheet

`ui.Sheet` adalah panel di bagian bawah di atas latar belakang yang diredupkan; tekanan pada latar belakang, Escape, atau kembali memanggil `OnClose`.

Atur `Label` atau `LabelFunc` untuk nama dialog yang aksesibel, dan `TestID` untuk menemukan panel dalam pengujian. `LabelFunc` mengikuti signal-nya tanpa mengganti konten yang terbuka. `CloseOnSwipe: true` juga memanggil `OnClose` setelah usapan ke bawah melewati ambang penutupan; perbarui `Open` di callback tersebut untuk menutupnya.

Atur `Style` atau `StyleFunc` untuk menyesuaikan latar belakang, radius sudut, dan padding permukaan sambil tetap mempertahankan grabber dan penanganan area aman. `StyleFunc` mengikuti signal yang dibacanya. `CornerRadius` tanpa `CornerRadii` membulatkan setiap sudut sebagai ganti bagian atas membulat bawaan.

```go preview=dialog/sheet height=320
ui.Sheet{Open: filters.Get, OnClose: func() { filters.Set(false) }, Children: filterControls}
```

## Peringatan

`ui.AlertDialog` meminta pengguna untuk mengonfirmasi atau mengakui sesuatu. Setiap aksi menutupnya, sedangkan tekanan di luar tidak.

```go preview=dialog/alert height=320
ui.AlertDialog{
	Open:         confirm.Get,
	OnOpenChange: func(v bool, _ base.Reason) { confirm.Set(v) },
	Title:        "Delete the note?",
	Description:  "This cannot be undone.",
	Actions: []ui.AlertAction{
		{Text: "Cancel", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
		{Text: "Delete", Color: colors.Danger, Role: core.AlertDestructive, Default: true, OnPress: remove},
	},
}
```

Aksi dengan `Role: core.AlertCancel` berjalan saat Escape atau kembali, diikuti oleh `OnOpenChange(false, base.ReasonCancel)`. `Default: true` memberikan fokus papan ketik pada sebuah aksi ketika peringatan terbuka di desktop, web, dan Apple TV, dan membuat Return memilihnya; tekanan pada aksi menjalankan `OnPress`, lalu `OnOpenChange(false, base.ReasonSelect)`. Return menjalankan aksi bawaan selama fokus papan ketik tidak berada pada aksi atau kolom lain: dengan fokus dipindahkan ke aksi lain, Return menekan aksi tersebut, dan dialog di atas peringatan mempertahankan Return untuk dirinya. Begitu sebuah aksi berjalan, peringatan tertutup, apa pun yang dilakukan `OnOpenChange`, dan tidak menjalankan aksi lain sampai `Open` menjadi false lalu true kembali. Jika beberapa aksi membawa penanda yang sama, yang pertama menang. Tanpa penanda, Escape menutup tanpa menjalankan aksi dan Return tidak memiliki aksi bawaan. `Role: core.AlertDestructive` mendeskripsikan sebuah aksi; `Variant` atau `Color` memilih tampilannya.

Untuk peringatan atau action sheet milik sistem, gunakan `cx.ShowAlert` ([Layanan platform](../services.md#alerts-and-the-clipboard)).

## Perilaku

Modal memblokir aplikasi di belakangnya, mengambil fokus papan ketik, dan tertutup saat Escape dan kembali kecuali diminta untuk tidak. Di layar sentuh, dialog adalah sheet yang mengikuti seretan ke bawah dan tertutup setelah melewati sebuah ambang.

## Aksesibilitas

`ui.Dialog` melaporkan `RoleDialog`, atau `Role` eksplisitnya, dan `ui.AlertDialog` melaporkan `RoleAlertDialog` di desktop dan layar sentuh. Keduanya membacakan judulnya, atau `Label` ketika judul tidak cukup, diikuti oleh deskripsinya. Fokus berpindah ke dalam dialog saat terbuka dan kembali saat tertutup; latar belakang bukan perhentian fokus. Tombol-tombol di footer mempertahankan peran dan labelnya.

## Platform

Di Android dan layar sentuh, dialog adalah sheet dari bawah; di desktop, dialog adalah permukaan di tengah dengan latar belakang. Kontennya sama; hanya penyajiannya yang berubah.

## Terkait

- [Navigasi](../navigation.md): halaman, tab, dan modal sekaligus.
- [Menu](menu.md): perintah yang ditambatkan dan menu konteks.
- [Aksesibilitas](../accessibility.md): fokus dan peran dialog.
- Halaman Dialog, Sheet, dan AlertDialog di galeri.
- [Komponen tanpa gaya](../authoring/base.md#dialog): `base.Dialog` dan `base.Sheet`, dengan penyajian iOS dan gerakan Anda sendiri.

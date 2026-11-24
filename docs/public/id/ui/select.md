---
title: Select
description: Satu nilai dari sebuah daftar, sebagai popup atau sheet.
group: Komponen
order: 36
---

`ui.Select[T]` memilih satu dari daftar nilai dengan tipe comparable apa pun. Daftar terbuka di bawah pemicu di setiap perangkat; `Presentation: base.PresentSheet` justru membukanya sebagai sheet dari bawah, sebagaimana mungkin diinginkan aplikasi ponsel.

```go preview=select/basic height=280
size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options: []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:   size.Get,
	OnChange: size.Set,
	Label:   "Size",
}
```

## Opsi dan pencarian

`Options` adalah `ui.Option[T]`, masing-masing dengan `Value`, `Label`, dan `Group`, `Icon`, fungsi `Disabled`, atau `Content` milik aplikasi sendiri yang opsional. Opsi-opsi bersebelahan dari satu `Group` yang tidak kosong ditampilkan di bawah header grup tersebut, di `Select` maupun `Combobox`, yang dilewati oleh tombol panah dan pengetikan serta dibacakan pembaca layar sebagai judul. Gunakan `OptionsFunc` ketika opsi mengikuti sebuah signal, sehingga daftar yang terbuka diperbarui di tempat dan mempertahankan nilai serta fokus kolom. `Placeholder` ditampilkan selama belum ada opsi yang dipilih. `Searchable` menyaring daftar dengan pengetikan, `OnClear` menampilkan tombol hapus, dan `Disabled` menonaktifkannya.

```go preview=select/searchable height=400
ui.Select[string]{
	Options:  languages,
	Value:    language.Get,
	OnChange: language.Set,
	Searchable: true,
	SearchPlaceholder: "Search languages",
	OnClear:  func() { language.Set("") },
	Placeholder: "Choose a language",
}
```

`Style` mengatur tata letak select, selebar 200 poin kecuali mengatur lebarnya; tinggi, padding, dan tipografinya diteruskan ke kolom, sebagaimana pada [Input](input.md), sehingga select sejajar dengan tombol `SizeSmall`:

```go
ui.Select[string]{Options: sizes, Value: size.Get, OnChange: size.Set, Label: "Size",
	Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
```

## Perilaku

Up, Down, Return, dan Space membuka daftar dari pemicu yang difokuskan; tombol panah memindahkan sorotan dan typeahead melompat berdasarkan judul item. Memilih sebuah item menutup daftar dan melaporkan nilainya. Escape, kembali, dan tekanan di luar menutupnya tanpa perubahan. `Empty` adalah teks yang ditampilkan selama pencarian tidak menemukan apa pun.

## Aksesibilitas

`Label` menamai kolom untuk teknologi asistif. Gunakan `LabelFunc` untuk memperbarui nama tersebut di tempat saat signal-nya berubah. Label kosong menggunakan `Placeholder`. Pemicu dan daftar membawa peran select, dan item yang dipilih dilaporkan. Papan ketik mencapai pemicu dengan Tab dan membukanya dengan Return atau Space.

## Platform

Di macOS, Windows, dan Linux, daftar adalah popup di samping pemicu. Di iOS dan Android, daftar adalah sheet, dan baris yang dipilih tetap ditampilkan pada kolom. Opsi dan nilai yang sama berfungsi di setiap target.

## Terkait

- [Formulir](../forms.md): select dalam formulir.
- [Menu](menu.md): perintah alih-alih nilai.
- [Tabs](tabs.md): beberapa opsi yang ditampilkan sekaligus.
- Halaman Select dan Combobox di galeri.
- [Komponen tanpa gaya](../authoring/base.md#select): `base.Select`, dengan grup dan pemicu Anda sendiri.

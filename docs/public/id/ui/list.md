---
title: List
description: Baris dalam jumlah berapa pun, dibangun hanya saat terlihat, dengan pemilihan dan aktivasi.
group: Komponen
order: 38
---

`ui.List` merender baris di atas tampilan gulir native dan hanya membangun baris yang dekat dengan layar, dengan pemilihan, aktivasi, dan papan ketik.

```go preview=list/basic height=280
ui.List{
	Count:         func() int { return len(items) },
	Key:           func(i int) any { return items[i].ID },
	EstimatedSize: 44,
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		item := items[row.Index()]
		return ui.Text(item.Title, 13, ui.Regular, colors.Text)
	},
}
```

## Baris dan identitas

`Count` menyatakan berapa banyak baris yang ada dan `Row` membangun satu baris. `Key` memberi identitas pada sebuah baris: baris yang tetap ada mempertahankan sel dan statusnya saat baris-baris di atasnya datang dan pergi. Gunakan ID alih-alih indeks ketika baris dapat ditambahkan, dihapus, atau diurutkan ulang; indeks adalah signal yang mengikuti sel saat sel itu menampilkan baris lain.

```go
ui.List{Count: count, Key: func(i int) any { return items[i].ID }, Row: func(cx *core.Scope, row core.ListRow) core.Node {
	return ui.Text(items[row.Index()].Title, 13, ui.Regular, colors.Text)
}}
```

`EstimatedSize` adalah tinggi baris yang diperkirakan, 50 poin jika 0. Baris dapat memiliki tinggi yang berbeda; daftar mengukur baris yang dibangunnya dan memperkirakan sisanya.

## Tata letak

`Header`, `Footer`, dan `Empty` menampung konten di sekitar baris dan selama tidak ada baris. `Gap` memisahkan baris, `Columns` menyusunnya berdampingan, dan padding daftar adalah milik konten, sehingga bilah gulir tetap berada di tepi. Berikan tinggi pada daftar atau biarkan daftar mengisi induknya.

## Pemilihan dan aktivasi

`ui.List` menerima `Selection`, yang dibuat dengan `ui.NewSelection(cx, multiple)`. Kunci, label, aktivasi, dan papan ketik mengikutinya: Up, Down, Home, dan End memindahkan kursor, Shift memperluas, Mod+A memilih semua, Space mengubah status dalam pemilihan ganda, dan Enter atau klik ganda mengaktifkan sebuah baris.

```go preview=list/selection height=320
selected := ui.NewSelection(cx, true)
ui.List{
	Count: count, Key: func(i int) any { return items[i].ID },
	Selection: selected,
	OnActivate: func(i int) { open(items[i]) },
	RowLabel:   func(i int) string { return items[i].Title },
	Row: func(cx *core.Scope, row core.ListRow) core.Node { return rowContent(cx, items[row.Index()]) },
}
```

`OnSelectionChange` berjalan setelah tekanan, tombol, atau typeahead mengubah baris mana yang dipilih, dan tidak ketika aplikasi mengatur `Selection` sendiri, sehingga panel detail dapat mengikuti pilihan pengguna sementara aplikasi memindahkan pilihan ke item yang dibuka dengan cara lain.

`TapSelects` membuat ketukan di ponsel memilih alih-alih mengaktifkan. `RowLabel` adalah apa yang dibacakan teknologi asistif dan apa yang dicocokkan typeahead.

## Memuat dan menggulir

`OnLoadMore` berjalan saat ujung daftar mendekat, sekali untuk setiap jumlah baris. `Ref` menggulir dengan `ScrollToIndexAt`, `ScrollToEnd`, dan `ScrollToIndex`. `core.List`, daftar polos di bawahnya, juga memiliki `OnLoadMoreAtStart`, `Inverted` untuk obrolan, `StickyHeaderIndices`, `MaintainVisibleContentPosition`, `Viewport`, dan `ViewabilityConfig` untuk kasus tingkat rendah ([Daftar dan pengguliran](../lists.md)).

## Aksesibilitas

Daftar adalah satu list atau listbox, dinamai oleh `Label`, dengan teks baris atau `RowLabel` sebagai nama barisnya. Daftar dengan pemilihan melaporkan baris tempat kursor berada dan mengumumkannya saat papan ketik bergerak. Pembaca layar yang bergerak keluar dari area tampilan meminta daftar untuk menggulir ke sana, yang membangun baris yang dicapainya.

## Platform

Tampilan gulir native dan virtualisasinya sama di setiap target. Baris didaur ulang, sehingga status baris harus mengikuti `Key`-nya; menyimpan status pembangunan di aplikasi alih-alih di dalam baris adalah aturan yang umum.

## Terkait

- [Daftar dan pengguliran](../lists.md): panduan lengkap, termasuk header lengket dan memuat lebih banyak.
- [Scroll](scroll.md): tampilan gulir polos.
- [DataTable](../lists.md#tables): tabel dengan banyak baris dan kolom.
- Halaman List, Table, dan DataTable di galeri.

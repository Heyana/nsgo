---
title: Daftar dan pengguliran
description: Tampilan gulir dengan gestur native, dan daftar yang mendaur ulang baris untuk berapa pun jumlah item.
group: Konsep inti
order: 55
---

## Tampilan gulir

`core.Scroll` adalah tampilan gulir milik platform itu sendiri, sehingga fling, pantulan, bilah gulir, snapping, dan pengguliran sentuh bersifat native. Turunannya ditata dalam sebuah kolom, atau sebuah baris dengan `Horizontal`, sepanjang yang mereka butuhkan.

```go
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

Tampilan gulir mengisi kontainernya dan padding diterapkan pada kontennya, sehingga bilah gulir tetap menempel di tepi. Tampilan ini menyusut agar muat di induknya kecuali gayanya menentukan lain. Atur `NoShrink` jika tampilan harus mempertahankan ukuran yang dimintanya saat elemen saudaranya menyusut. `HideIndicator` menyembunyikan indikator milik platform, dan `Bar` menggambar indikator milik aplikasi sendiri.

`Behavior` dan `BehaviorFunc` memilih cara tampilan ini bergulir: `Paging`, `SnapToInterval` dan `SnapToOffsets` dengan `SnapToAlignment`, sebuah `DecelerationRate`, `NoBounces`, `ContentInset`, dan aturan papan ketik. `MinZoomScale` dan `MaxZoomScale` memungkinkan cubitan untuk memperbesar, `OnRefresh` dan `Refreshing` membuat tarikan menyegarkannya, dan `OffsetValues` mengikuti offset-nya di thread utama untuk efek yang bergerak bersamanya.

```go
page := core.Scroll{
	OnScroll: func(e core.ScrollEvent) { y.Set(e.Offset.Y) },
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: []core.Node{...},
}
```

`OnScroll` melaporkan offset paling banyak sekali per giliran, atau sekali per `ScrollEventThrottle`. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd`, dan `OnScrollEnd` mengapit gestur pengguna. `Ref` menggulir dari kode dengan `ScrollTo`, `ScrollToEnd`, `ZoomTo`, dan `ScrollToIndex` untuk sebuah daftar.

## Daftar

`core.List` merender data panjang pada tampilan gulir native sambil hanya membuat baris yang berada di dekat layar. `Count` menyatakan berapa banyak baris yang ada dan `Row` membangun satu baris; `Key` memberi setiap baris sebuah identitas, sehingga baris yang tetap ada mempertahankan selnya saat baris-baris di atasnya datang dan pergi.

```go
core.List{
	Count:         func() int { return len(items.Get()) },
	Key:           func(i int) any { return items.Get()[i].ID },
	EstimatedSize: 56,
	Row: func(cx *core.Scope, r core.ListRow) core.Node {
		return core.View{Style: core.Style{Padding: core.All(12)}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return items.Get()[r.Index()].Title }},
		}}
	},
}
```

`Row` berjalan sekali per sel yang didaur ulang dan memperbarui `r.Index()` ketika sel tersebut dipakai ulang untuk baris lain, jadi bacalah `r.Index()` dan signal item di dalam fungsi reaktif (`ContentFunc`, `StyleFunc`, `NewMemo`) alih-alih menangkap nilai sekali pakai saat membangun baris. Sel yang menganggur diparkir sehingga komputasinya menunggu sampai sel tersebut terikat lagi ke baris yang aktif.

`Header`, `Footer`, `Empty`, dan `Separator` menampung konten di sekitar dan di antara baris. `NumColumns` menempatkan baris berdampingan, `Gap` dan `ColumnGap` memisahkannya, dan `Horizontal` menggulir ke samping. `StickyHeaderIndices` menyematkan baris-baris tersebut di awal viewport sampai baris berikutnya mendorongnya keluar, dan `StickyHeaderOffset` menjaga agar header di atasnya tidak tertutup.

`OnViewableItemsChange` melaporkan baris yang terlihat berdasarkan ambang batas `ViewabilityConfig`, dan `OnLoadMore` berjalan saat viewport mendekati akhir, sekali per jumlah baris. `OnLoadMoreAtStart` melakukan hal yang sama di awal, dan `InitialScrollIndex` membuka daftar pada sebuah baris. `Alignment: core.ListAlignEnd` menempatkan daftar di ujungnya, seperti yang dilakukan obrolan dengan baris-barisnya dalam urutan baca: daftar terbuka pada baris terbarunya, mengikuti baris baru selama pengguna berada di ujung, dan menahan percakapan pendek di bagian bawah. `Inverted` justru menaruh baris 0 di ujung, dan `MaintainVisibleContentPosition` mengikuti ujung daftar yang bertambah selama pengguna berada di dekatnya.

```go
core.List{Ref: list, Count: count, EstimatedSize: 64, Row: row, OnLoadMore: loadMore,
	StickyHeaderIndices: headers(cx), Footer: core.View{Style: core.Style{Height: core.Pt(24)}}}
```

`list.ScrollToIndexAt(index, 0.5, 0, true)` menggulir sebuah baris ke tengah viewport, dan `list.ScrollToEnd(true)` ke ujung. `list.ListPosition()` membaca di mana viewport dimulai, yaitu sebuah baris dan seberapa jauh ke dalamnya, untuk mempertahankan posisi baca: `list.ScrollToIndexAt(p.Index, 0, -p.Offset, false)` mengembalikan daftar yang dibuat ulang ke posisi itu. Sebuah daftar juga bisa berada dalam satu halaman bersama konten lain: atur `Viewport: core.ListViewportParent`, dan tampilan gulir di sekitarnya menggulir daftar bersama sisa kontennya.

## Tabel

`base/datatable` menampilkan tabel berisi sejuta baris dengan baris dan kolom tervirtualisasi, header lengket, kolom yang tetap di kiri (`Fixed`) atau di kanan (`FixedRight`) sementara kolom lain bergulir di bawahnya, pengurutan, pemilihan satu atau banyak baris, pengubahan ukuran dan pemindahan kolom, serta sel milik aplikasi sendiri. `MultiSelect` mendukung pengalihan dengan Command atau Control, rentang dengan Shift, dan perluasan papan ketik dengan Shift, sedangkan `SelectedRows` dan `OnSelectedRowsChange` memungkinkan aplikasi mengendalikan baris-barisnya. `CellTooltip` memberi sebuah sel tooltip, yang juga mendeskripsikan sel tersebut kepada pembaca layar, dengan gaya dari `Styles.CellTooltip`; apa yang digambar `RenderCell` tidak menerima pointer, sehingga `ui.Tooltip` di dalam sel tidak pernah tampil. `base.Table` adalah tabel biasa berisi baris-baris milik aplikasi untuk data yang lebih kecil, dan `base/plot` menggambar grafik.

Gunakan `DataTable.LabelFunc` untuk nama grid yang mengikuti locale atau status aplikasi. `ColumnGroup.LabelFunc` memperbarui judul kolom berkelompok selama kelompok tersebut tetap terlihat.

## Berikutnya

- [Animasi](animation.md): efek yang mengikuti tampilan gulir.
- [Teks](text.md): baris-baris teks yang membungkus.
- [Navigasi](navigation.md): layar yang kontennya bergulir.

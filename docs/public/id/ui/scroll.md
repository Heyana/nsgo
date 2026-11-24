---
title: Scroll
description: Tampilan gulir milik platform, dengan paging, snapping, zoom, dan penyegaran.
group: Komponen
order: 39
---

`core.Scroll` adalah tampilan gulir milik platform sendiri. Anak-anaknya disusun dalam kolom, atau baris dengan `Horizontal`, dan tampilan ini mengisi wadahnya sementara kontennya digulir.

```go preview=scroll/basic height=240
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

Padding diletakkan pada konten, bukan pada tampilan gulir, sehingga bilah gulir tetap menempel di tepi. Tampilan gulir menyusut agar muat di dalam induknya kecuali gayanya menyatakan lain. Atur `NoShrink` ketika tampilan gulir harus mempertahankan ukuran yang dimintanya sementara elemen saudaranya menyusut.

## Perilaku

`Behavior` memilih cara menggulirnya: `Paging`, `SnapToInterval` atau `SnapToOffsets` dengan `SnapToAlignment`, `DecelerationRate`, `NoBounces`, `ContentInset`, dan aturan papan ketik.

```go preview=scroll/paging height=240
core.Scroll{
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: pages,
}
```

`HideIndicator` menyembunyikan bilah gulir milik platform dan `Bar` menggambar milik aplikasi sendiri. `MinZoomScale` dan `MaxZoomScale` memungkinkan cubitan untuk melakukan zoom; `NoPinchZoom` mengabaikannya. `CenterContent` menempatkan konten yang lebih kecil dari viewport di tengah.

## Penyegaran dan event

`OnRefresh` membuat tampilan dapat disegarkan; atur `Refreshing` selama pekerjaan berjalan. `PullValue` mengikuti seberapa jauh konten ditarik, untuk indikator milik aplikasi sendiri, dan `Behavior.RefreshIndicator` mengatur tingginya.

```go
refreshing := core.NewSignal(cx, false)
core.Scroll{
	Refreshing: refreshing.Get,
	OnRefresh:  func() { refreshing.Set(true); reload(cx) },
	Children:   content,
}
```

`OnScroll` melaporkan offset paling banyak sekali per giliran, atau sekali per `ScrollEventThrottle`; `Stale` menandai laporan dari sebelum platform menerapkan guliran dari kode. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd`, dan `OnScrollEnd` mengapit gestur pengguna.

`OffsetValues` mengikuti offset di thread utama, untuk header yang menciut tanpa commit di setiap frame. `Ref` menggulir dari kode dengan `ScrollTo`, `ScrollToEnd`, dan `ZoomTo`, dan ref dari tampilan mana pun di dalamnya membawanya ke area tampilan dengan `Reveal`, menggulir setiap tampilan gulir di sekitarnya:

```go
row.Reveal(core.RevealOptions{Vertical: core.ScrollAlignCenter, Animated: true})
```

`RevealBox` justru membawa sebuah kotak di dalam tampilan ke area tampilan, seperti kotak sebuah kata dari `TextRangeBoxes` milik teks, dan `Margin` menjaga ruang tetap kosong di tepi tampilan gulir terluar untuk bilah yang berada di atasnya.

Tampilan yang menerima fokus yang dipindahkan oleh papan ketik akan menggulir ke area tampilan dengan sendirinya.

## Tata letak

`Horizontal` menggulir secara mendatar. `Behavior.ContentInset` menambahkan ruang di sekitar konten yang ikut menggulir bersamanya. Untuk halaman yang mengisi area aman, gunakan `PaddingEnv` pada konten, bukan pada tampilan gulir.

## Aksesibilitas

`Label` menamai area gulir dan `Role` menyatakan jenisnya, seperti kotak daftar. Gestur gulir platform, pengguliran dengan papan ketik, dan pengguliran oleh pembaca layar dipertahankan; `Busy` melaporkan tampilan yang masih memuat.

## Platform

Setiap backend menggunakan tampilan gulir milik platform, sehingga lemparan (fling), pantulan, bilah gulir, dan pengguliran sentuh bersifat native. Efek tepi berbeda-beda: macOS dan iOS memantul seperti karet, Android meregang, Windows dan Linux berhenti, dan web mengikuti browser. `EdgeEffects` dan `EdgeBar` menambahkan bilah di atas konten di tempat yang diinginkan aplikasi. Di ponsel dan tablet iOS dan Android, tampilan gulir vertikal dan horizontal yang meluap menampilkan pegangan gulir yang dapat diseret dengan sentuhan di sepanjang sumbu aktif, yang menyusuri seluruh rentang konten; tampilan gulir pendek yang muat tanpa meluap, tampilan yang dapat di-zoom, `Behavior.NoScroll`, `HideIndicator`, dan tampilan yang menggunakan `Bar` kustom atau `base.SetScrollbarTheme` menyembunyikannya. Dalam pengujian, `nsgotest.App.ScrollHandle` melaporkan batas viewport-nya dan `DragAt` atau `Touch` menyeretnya secara langsung. Di Windows, indikator overlay muncul untuk roda, sentuhan, seretan bilah, dan momentumnya, bukan untuk `ScrollTo`, `ScrollBy`, reveal, inset, atau animasi milik aplikasi.

## Terkait

- [Daftar dan pengguliran](../lists.md): panduan lengkap tentang pengguliran dan daftar.
- [List](list.md): baris yang didaur ulang.
- [Tata letak](../layout.md): padding dan area aman.
- Halaman Scroll dan Lists di galeri.

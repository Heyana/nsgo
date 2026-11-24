---
title: Gambar dan ikon
description: Tampilkan gambar dari URL, resource aplikasi, byte atau piksel yang digambar aplikasi, serta gambar ikon Lucide dan SVG.
group: Konsep inti
order: 54
---

## Sebuah gambar

`core.Image` menampilkan bitmap atau SVG: sebuah URL, file, path `asset:` di dalam resource aplikasi, atau byte yang disimpan aplikasi. Platform memuat, mendekode, dan menyimpannya di cache di luar thread utama, lalu thread utama menggambar hasilnya.

```go
core.Image{
	Source: core.Src("https://example.com/cover.jpg"),
	ResizeMode: core.Cover,
	Style: core.Style{Width: core.Pt(160), Height: core.Pt(100), CornerRadius: 12},
}
```

`core.Cover` mengisi kotak dan memotong bagian yang meluap; `core.Contain` memuat seluruh gambar agar pas, `core.Center` mempertahankan ukuran alaminya, dan `core.ScaleDown` mempertahankannya kecuali jika lebih besar daripada kotak. `BlurRadius` memburamkan gambar dalam satuan poin, `TintColor` mewarnai ulang pikselnya yang tidak transparan, dan `ObjectPosition` menempatkannya di dalam kotak.

Bitmap yang diskalakan menggunakan sampling halus secara default. Atur `Sampling: core.ImageSamplingNearest` di Go atau `sampling="nearest"` di TSX untuk mempertahankan tepi tajam dari pixel art dan buffer piksel. Platform Apple, Windows, Linux, Android, dan web mendukung pengaturan ini; HarmonyOS dan mini program juga menerapkannya, meskipun hasilnya belum diuji di perangkat fisik. Untuk tepi piksel yang tepat di browser menggunakan CSS `pixelated`, skalakan ke bilangan bulat piksel perangkat per piksel sumber.

`Source` mendeklarasikan ukuran yang digunakan tata letak: satu dimensi menurunkan dimensi lainnya, dan gambar tanpa ukuran bernilai 0 kali 0 hingga gaya memberinya ukuran. `core.ImageSource{Data: data, Size: core.Size{Width: 24, Height: 24}}` menampilkan byte yang disematkan dengan `go:embed`, dan `Size` memberi SVG ukuran intrinsiknya.

Dengan [`desktop.WindowPointerPainted`](services.md), `HitTestAlpha: 0.5` membiarkan pointer menembus piksel yang tingkat opasitasnya kurang dari setengah pada macOS dan Windows. TSX mengatur ambang batas yang sama dengan `hitTestAlpha`.

## Frame yang digambar aplikasi

`Pixels` menampilkan `*image.RGBA` yang digambar oleh aplikasi, misalnya frame pemutar video atau emulator, tanpa ada yang dienkode, dimuat, atau didekode. Tulis setiap frame ke dalam buffer yang sama dan naikkan `Version`: view menampilkannya tanpa event pemuatan dan tanpa efek pudar, dengan alpha tetap terjaga, serta di macOS, iOS, tvOS, watchOS, Android, Windows, dan Linux menukarnya langsung di tempat tanpa frame kosong di antaranya. Tanpa `Size`, ukurannya dihitung berdasarkan piksel dalam satuan poin.

```go
frame := image.NewRGBA(image.Rect(0, 0, 960, 540))
version := core.NewSignal(cx, uint64(0))
var tick func(time.Time)
tick = func(at time.Time) {
	if decoder.WriteDue(at, frame) { // writes the next frame into frame.Pix
		version.Set(version.Get() + 1)
	}
	cx.RequestAnimationFrame(tick)
}
cx.RequestAnimationFrame(tick)

core.Image{ResizeMode: core.Contain, SourceFunc: func() core.ImageSource {
	return core.ImageSource{Pixels: frame, Version: version.Get()}
}}
```

NSGo menyalin piksel saat commit yang membawa `Version` baru dijalankan, sehingga aplikasi dapat menulis frame berikutnya dari callback `RequestAnimationFrame` berikutnya; dekoder pada goroutine tersendiri menulis ke buffer kedua dan mengirimkan pertukaran tersebut. `Version` yang sama tidak digambar ulang. `cx.ImageSize` langsung menjawab sumber piksel dengan ukuran buffer-nya. `core.Canvas` menggambar piksel dengan `DrawImage`, disalin dengan cara yang sama, di setiap platform; mini program memerlukan kanvas 2D WeChat. `Images` milik `core.Shader` menerima piksel dengan cara yang sama, sehingga shader dapat memfilter frame yang ditulis aplikasi, seperti layar emulator, di setiap platform yang menggambar shader.

Platform Apple, Android, Windows, Linux, dan web memperbarui gambar piksel secara langsung, sedangkan mini program WeChat menggunakan kanvas 2D. HarmonyOS membuat sebuah `PixelMap` untuk setiap versi, sudah diuji di perangkat, dan penyalinannya melintasi bridge string, sehingga buffer besar yang ditulis ulang setiap frame berjalan jauh di bawah 60 fps di sana ([buffer piksel](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/images.md#pixel-buffers)).

## Mengambil tangkapan tampilan

Panggil `cx.Capture(&ref, done)` dengan `core.Ref` milik view yang terpasang, atau `cx.Capture(nil, done)` untuk permukaan (*surface*) saat ini. Callback menerima byte PNG pada skala piksel permukaan tersebut atau sebuah error pada goroutine mesin. Gunakan `core.ImageSource{Data: png}` untuk menampilkan hasilnya. Untuk membagikannya, tulis byte tersebut ke file `.png` di direktori cache aplikasi dan berikan path-nya di `core.ShareOptions.Files`. [Membagikan file](services.md#opening-links-and-files)

Pengambilan tangkapan mengambil piksel permukaan yang terlihat dalam batas view. Fitur ini tersedia di macOS, iOS, tvOS, Windows, Linux, Android, dan web. Android menangkap jendela activity. Tangkapan web melukis DOM aplikasi ke dalam kanvas, termasuk keluaran shader NSGo, serta mengabaikan frame tertanam, gambar yang tidak dapat dibaca, dan sebagian efek CSS.

## Resource dan skala

File di bawah `resources/` ikut dikemas bersama aplikasi. `core.Src("asset:images/cover.png")` memuat salah satunya, dan `Scales: core.ScalesOf(1, 2, 3)` memilih `cover.png`, `cover@2x.png`, atau `cover@3x.png` sesuai rasio piksel layar.

```go
core.Image{Source: core.ImageSource{
	URI:    "asset:images/cover.png",
	Scales: core.ScalesOf(1, 2, 3),
}}
```

`Placeholder` menampilkan sebuah sumber selagi gambar dimuat, dan `FadeDuration` memunculkan gambar yang sudah dimuat secara perlahan di atasnya; nilai 0 membiarkan platform memilih, dan satu nanodetik langsung menampilkannya. `OnLoadStart`, `OnProgress`, `OnLoad`, `OnError`, dan `OnLoadEnd` mengikuti proses pemuatan. `cx.ImageSize` membaca ukuran sumber dalam piksel tanpa menampilkannya, `cx.PrefetchImage` mengunduh satu gambar ke dalam cache, `cx.SetImageCacheLimit` menetapkan anggaran byte gambar terenkode di memori (default `core.DefaultImageCacheLimit`, 32 MiB; nilai negatif dibatasi ke 0), dan `cx.KeepImage` menahan atlas atau tekstur agar tidak dikeluarkan dari cache hingga `release` yang dikembalikannya dijalankan atau scope-nya dibuang. Batas tersebut berlaku untuk setiap cache, tidak termasuk gambar yang ditahan (*pin*), pemuatan yang sedang berlangsung, dan pass penggambaran saat ini; batas ini tidak membatasi piksel yang sudah didekode, memori proses, atau caching browser. Nilai nol menonaktifkan retensi yang tidak ditahan. Backend native menyimpan gambar jarak jauh dalam cache disk miliknya sendiri.

## Gambar animasi dan vektor

Animasi GIF dan WebP diputar di dalam view, setiap frame mengikuti jeda waktunya masing-masing, dan berulang sesuai ketentuan file. Dokumen SVG dimuat sebagai gambar dan tetap tajam di ukuran berapa pun; SVG satu warna dapat diwarnai dengan `TintColor`.

## Ikon

`core.Icon` menggambar SVG dengan warna teks yang diwarisinya, setinggi ukuran font kecuali jika gayanya menentukan ukuran lain. Kotaknya mempertahankan rasio aspek SVG, sehingga kotak persegi tidak akan meregangkannya.

```go
core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 20}, Label: "Search"}
```

`icons/lucide` memiliki aksesor untuk setiap ikon Lucide, dinamai dalam PascalCase (`lucide.ArrowUpRight`). Sebuah ikon dianggap sebagai dekorasi bagi teknologi asistif kecuali jika `Label` memberinya nama. `ui.Icon` sama namun dengan nilai default `ui`, sedangkan `core.SVG` dan `core.SVGView` menggambar path yang dibangun aplikasi sendiri.

## Gambar bergaya

`ui.Image` adalah gambar di dalam wadah bersudut membulat, dengan placeholder selagi dimuat dan ikon gambar redup saat gagal. Gambar ini mempertahankan rasio aspek sumbernya, atau 3:2 jika tidak ada, kecuali jika gayanya menentukan ukuran kedua sisi.

```go
ui.Image{
	Source:      core.Src("asset:images/cover.png"),
	Label:       "Cover",
	ResizeMode:  core.Cover,
	AspectRatio: 16.0 / 9.0,
	Placeholder: ui.Skeleton{Style: core.Style{Height: core.Pt(180)}},
}
```

## Selanjutnya

- [Teks](text.md): gambar di dalam baris teks, sebagai span.
- [Gaya dan tema](styles.md): sudut, bayangan, dan pewarnaan (*tint*).
- [Daftar dan pengguliran](lists.md): baris gambar yang didaur ulang.

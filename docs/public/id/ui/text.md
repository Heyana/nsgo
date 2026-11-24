---
title: Teks
description: Paragraf, span, tautan, font, dan dokumen.
group: Komponen
order: 32
---

`core.Text` adalah satu paragraf yang disusun oleh mesin teks milik platform. `ui.Text` dan `ui.Textf` adalah helper untuk satu baris dengan tipografi `ui`; elemennya tetap sebuah `core.Text`, sehingga ia juga menerima `Style` dan `Spans`.

```go preview=text/basic
ui.Text("Settings", 20, ui.Bold, colors.Text)
ui.Textf(func() string { return fmt.Sprintf("%d files", n.Get()) }, 13, ui.Regular, colors.Muted)
```

## Konten

`Content` bersifat tetap dan `ContentFunc` mengikuti sebuah signal. `MaxLines` membatasi jumlah baris dan `EllipsizeMode` memotong sisanya di awal, tengah, atau akhir; `Style.WhiteSpaceNoWrap` menjaga paragraf tetap dalam satu baris. `TextAlign`, `TextTransform`, `LineHeight`, dan `LetterSpacing` memberi gaya pada paragraf, dan `OnTextLayout` melaporkan baris-barisnya setelah tata letak.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) },
	MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 15}}
```

## Span dan tautan

`Spans` membentuk teks dari beberapa run, masing-masing dengan gayanya sendiri. Span dengan `OnPress` adalah tautan, dan `Background` serta `CornerRadius`-nya digambar di belakang karakternya di setiap baris. `SpansFunc` memberikan run-nya lagi ketika apa yang dibacanya berubah, untuk kode yang disorot atau kalimat yang berubah.

```go preview=text/spans
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "Read the "},
	{Content: "guide", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openGuide},
	{Content: " before you start."},
}}
```

`Attachment` menempatkan sebuah view di dalam baris, menggantikan konten span: ikon, gambar, atau tombol kecil. Ia ditata dengan ukurannya sendiri, berpindah baris bersama kata-kata, dan menerima tekanannya sendiri.

## Font dan gaya

`FontFamily` menamai sebuah keluarga font atau daftar CSS. `FontSize`, `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LetterSpacing`, `TextDecoration`, serta `TextShadowColor` beserta offset dan radiusnya memberi gaya pada teks. `core.FontMonospace` adalah keluarga font lebar tetap milik platform.

```go
core.Text{Content: "fn main() {}", Style: core.Style{FontFamily: core.FontMonospace, FontSize: 13}}
```

Daftarkan font yang dikemas dengan `nsgo.Fonts` sebelum view pertama, atau daftarkan sebuah face saat aplikasi berjalan dengan `cx.RegisterFont`. Teks yang disiapkan sebelum sebuah font tiba akan disiapkan ulang.

## Dokumen

`base/textview.TextView` menampilkan Markdown atau HTML dengan primitif yang sama: judul, paragraf, daftar, tabel, kode, catatan kaki, peringatan, gambar, rumus, dan tautan. `textview.Parse` membaca Markdown dan `textview.ParseHTML` membaca HTML, dan `Options.Streaming` memperbaiki dokumen yang masih sedang tiba.

```go preview=text/document height=240
textview.TextView{Markdown: "## Notes\n\nA **native** document.", Styles: ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) { open(e.URL) }}
```

`textview.NewSearch` mencari di teks yang ditampilkan `TextView` dan menyorot kecocokannya. Berikan pencarian yang sama ke beberapa view dengan `textview.ProvideSearch`, lalu panggil `Open`, `SetQuery`, `Next`, dan `Previous`; `Count` dan `Selected` melaporkan hasilnya. Bagian-bagian pencarian diberi gaya dengan `textview.PartSearch` dan `textview.PartSearchSelected`.

## Seleksi dan aksesibilitas

`SelectableOn` memungkinkan pengguna memilih dan menyalin teks. `core.View` dengan `Selectable` membuat teks-teks di dalamnya terseleksi bersama sesuai urutan dokumen. `Role` seperti `RoleHeader` memberi tahu teknologi bantu apa teks tersebut, dan `Label` menggantikannya ketika kata-katanya bukan labelnya.

## Platform

Mesin teks milik platform menata teks, sehingga pemenggalan baris, font, dan metode input mengikutinya. Pemenggalan baris CJK, emoji, dan teks kanan-ke-kiri mengikuti sistem. Aplikasi dapat membaca ukuran teks dengan `cx.Env().FontScale()`, dan teks mengikutinya kecuali `NoFontScaling` diatur.

## Terkait

- [Teks](../text.md): panduan lengkap tentang teks, span, font, dan dokumen.
- [Gambar dan ikon](../images.md): gambar dan ikon di dalam baris.
- [Aksesibilitas](../accessibility.md): judul, label, dan seleksi.
- Halaman Text dan TextView di galeri.

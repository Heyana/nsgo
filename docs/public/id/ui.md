---
title: Referensi komponen
description: Komponen yang paling sering digunakan, beserta varian, state, dan aksesibilitasnya.
group: Komponen
order: 30
---

Inilah komponen-komponen `ui`, dengan tampilan NSGo dan mengikuti tema. Referensi ini dimulai dengan komponen yang pertama kali dibutuhkan aplikasi; setiap halaman dimulai dengan contoh terpendek, lalu membahas bagian-bagian, state, dan aksesibilitasnya. Galeri mencakup setiap komponen, termasuk yang belum memiliki halaman di sini.

[Galeri](https://nsgo.dev/gallery/) menjalankan demo setiap komponen di browser.

## Aksi

- [Button](ui/button.md): tombol tekan, polos atau dalam warna aksen, serta tombol ikon.

## Teks dan input

- [Text](ui/text.md): paragraf, span, tautan, font, dan teks dokumen.
- [Input](ui/input.md): field teks milik platform, dikendalikan atau tidak.
- [Checkbox](ui/checkbox.md): kotak centang, tercentang, tidak tercentang, atau tak tentu.
- [Switch](ui/switch.md): pengaturan yang aktif atau nonaktif.
- [Select](ui/select.md): satu nilai dari sebuah daftar, sebagai popup atau sheet.
- [Slider](ui/slider.md): satu nilai atau sebuah rentang di sepanjang track.

## Koleksi

- [List](ui/list.md): baris dengan jumlah berapa pun, dibangun hanya saat terlihat, dengan pemilihan dan aktivasi.
- [Scroll](ui/scroll.md): scroll view milik platform, dengan paging, snapping, zoom, dan refresh.

## Navigasi

- [Tabs](ui/tabs.md): daftar tab, segmented control, dan tab bar sistem.

## Overlay

- [Dialog](ui/dialog.md): permukaan modal, sheet, dan alert sistem.
- [Menu](ui/menu.md): menu dari sebuah pemicu, menu konteks, dan item-itemnya.

## Lainnya

Galeri mencakup sisanya: tabel, grafik, editor kode, teks kaya, terminal, dock, command palette, pemilih warna, tanggal, dan emoji (`ui.EmojiPicker`), toast, dan lainnya. Masing-masing memiliki demo di sana.

`base/terminal.View` membuka path lokal yang sudah ada dengan Command+klik (Control+klik di Windows dan Linux), sebagaimana ia membuka URL. Path relatif menggunakan direktori kerja OSC 7 milik shell; shell lokal yang belum melaporkannya menggunakan `terminal.Options.Dir`, atau direktori home jika tidak diatur. Beri tanda kutip pada nama file yang mengandung spasi. Otoritas direktori jarak jauh dan koneksi tanpa laporan direktori lokal tidak di-resolve terhadap file lokal. File dan folder menggunakan `Scope.OpenFile`; tautan URL tetap melalui `Scope.OpenURL` dan handler URL milik host penyemat.

`base/terminal.View` secara bawaan menolak pembacaan dan penulisan clipboard oleh program. Atur `AllowClipboardRead` dan `AllowClipboardWrite` hanya jika aplikasi mengizinkannya; `AccessClipboard` milik view yang meminta tetap berlaku. Prop yang sama tersedia di `Terminal` milik TSX dan dapat diubah tanpa memulai ulang sesinya. Aplikasi menyediakan UI persetujuan. Tempel eksplisit oleh pengguna secara terpisah menawarkan snapshot teks/PNG kepada program yang menggunakan protokol clipboard Kitty (mode 5522); kata sandi sekali pakainya hanya dapat membaca snapshot tersebut. Program tanpa mode itu menerima teks bracketed biasa. Ketersediaan gambar di clipboard mengikuti layanan clipboard milik platform; primary selection X11 tidak diekspos.

## Selanjutnya

- [Komponen dan state](components.md): bagaimana sebuah komponen mengembalikan elemen dan bagaimana signal memperbaruinya.
- [Gaya dan tema](styles.md): tema tempat komponen mengambil warnanya.
- [Komponen tanpa gaya](authoring/base.md): kontrol yang sama tanpa tampilan, untuk tampilan Anda sendiri.
- [Aksesibilitas](accessibility.md): label, peran, dan nilai untuk setiap kontrol.

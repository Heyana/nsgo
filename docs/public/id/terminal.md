---
title: Terminal
description: Jalankan program atau tampilkan sesi jarak jauh dalam grid terminal.
group: Konsep inti
order: 63
---

`base/terminal` menyediakan sebuah sesi (`Terminal`) dan `View`-nya. Desktop menjalankan program lokal; iOS dan browser menggunakan `Options.Conn`, sebuah `io.ReadWriteCloser` yang terhubung ke program jarak jauh. Aplikasi memiliki sesi tersebut dan menutupnya setelah selesai.

```go
t, err := terminal.New(terminal.Options{CursorBlink: true})
if err != nil {
	return core.Text{Content: err.Error()}
}
cx.Cleanup(func() { t.Close() })
return terminal.View{
	Terminal: t, AutoFocus: true,
	MinimumContrast: 4.5, PaddingColor: terminal.PaddingExtend,
	Style: core.Style{FlexGrow: 1},
}
```

`Options.Cursor` memilih bentuk bawaan block, bar, atau underline. `CursorBlink` mengaktifkan kedipan bawaannya; nilai nol tetap diam. Program menimpa bentuk dan kedipan melalui DECSCUSR atau mode DEC 12. Kursor yang tidak terfokus berbentuk berongga dan diam; komposisi input dan gerakan yang dikurangi (*reduced motion*) juga membuatnya tetap diam. Jam animasi milik platform menggerakkan kedipan tanpa engine menggambar ulang teks terminal.

`View.Font`, `Theme`, dan `DarkTheme` mengatur font dan palet. Grid menggambar sendiri karakter box-drawing dan block Unicode serta pemisah Powerline U+E0B0–U+E0B7, dengan menyelaraskan sel yang bersebelahan ke piksel perangkat. Teks lainnya menggunakan font platform dan cadangannya.

`MinimumContrast` meningkatkan kontras teks biasa terhadap latar belakang sel yang sebenarnya, termasuk warna terbalik (*inverse*), teks samar (*faint*), dan seleksi. Nilai 1–21 menggunakan rasio kontras WCAG; nol mempertahankan warna. Grafik dan emoji mempertahankan warnanya. Dengan `Transparent`, kontras tetap menggunakan latar belakang terminal yang dikonfigurasi.

`Padding` menyisakan ruang di luar grid, dalam satuan poin. `PaddingColor` memilih latar belakangnya:

| Nilai | Perilaku |
| --- | --- |
| `PaddingBackground` | Latar belakang terminal, termasuk perubahan OSC 11 |
| `PaddingExtend` | Latar belakang sel terdekat; pada layar utama, perluasan atas/bawah ditahan di samping baris prompt, baris Powerline, atau baris yang berisi sel berlatar belakang bawaan |
| `PaddingExtendAlways` | Memperluas setiap tepi tanpa pengecualian tersebut |

Aplikasi TSX mengimpor `tsx/terminal` demi efek sampingnya untuk mendaftarkan `Terminal` di `nsgo/base`. Prop-nya adalah `cursorBlink`, `minimumContrast`, dan `paddingColor`; yang terakhir menerima `background`, `extend`, dan `extend-always`. Prop tampilan dapat berubah selama sesi berlanjut.

## Di browser

Aplikasi menyematkan modul WebAssembly libghostty-vt yang versinya dipatok dan menjalankan view terminal serta jalur input yang sama. Membangun aplikasi tidak memerlukan Zig, instalasi pustaka native, atau unduhan saat runtime.

Adaptasikan WebSocket milik browser ke `io.ReadWriteCloser`: kirim byte biner dan gabungkan atau pisahkan pesan masuk sebagai sebuah stream. Aplikasi dan servernya bertanggung jawab atas autentikasi, penyambungan ulang, dan pesan resize/kontrol apa pun; NSGo mengonsumsi byte terminal. [Fixture penerimaan browser](https://github.com/nsgo-dev/nsgo/blob/main/internal/testhost/testdata/terminalweb/socket_js_test.go) menunjukkan adaptor byte-stream lengkap beserta penanganan penutupan.

## Aksi dan zoom

`View.Keybinds` menimpa bawaan dengan `Keybind{Key, Action, Amount}`. Tombol menggunakan nama yang sama seperti `Scope.OnKey`, seperti `mod+=` atau `ctrl+shift+c`. Entri yang belakangan menang. Pengikatan berjalan selama input terminal ini memiliki fokus dan tidak ada metode input yang sedang menyusun teks.

Aksinya adalah `CopyToClipboard`, `PasteFromClipboard`, `SelectAll`, `ClearScreen`, `IncreaseFontSize`, `DecreaseFontSize`, `ResetFontSize`, `SetFontSize`, dan `Ignore`. Aksi jendela dan tab termasuk dalam keymap aplikasi. `Ignore` mengonsumsi tombol tanpa mengirimkannya ke program.

Secara bawaan, `mod+=` dan `mod+shift+=` memperbesar font satu poin, `mod+-` memperkecilnya, dan `mod+0` mengatur ulang ke `View.Font.Size` (13 jika tidak diatur). `Amount` mengatur langkah untuk memperbesar/memperkecil, satu jika nol, atau ukuran untuk `SetFontSize`. Ukuran dibatasi pada 1–255 poin; permintaan tak hingga atau NaN diabaikan. Zoom mempertahankan sesi dan view luar, memperbarui baris dan kolom grid, serta mengubah ukuran program.

```go
ref := &terminal.Ref{}
view := terminal.View{
	Terminal: session, Ref: ref,
	Keybinds: []terminal.Keybind{
		{Key: "ctrl+shift+k", Action: terminal.ClearScreen},
		{Key: "mod+=", Action: terminal.IncreaseFontSize, Amount: 2},
	},
}
```

`Ref.FontSize()` dilacak untuk kontrol yang menampilkannya; `Ref.SetFontSize(size)` dan `Ref.Perform(action, amount)` berjalan di engine aplikasi, seperti halnya handler event. `Perform` melaporkan apakah sebuah aksi tersedia. Ref yang tidak terpasang mengembalikan ukuran nol dan menolak aksi.

## Menu konteks

Klik sekunder membuka `base.ContextMenu` tanpa gaya dengan item Copy, Paste, Select All, dan Clear yang dilokalkan. Copy memerlukan seleksi. Clear menghapus konten di atas prompt dan dinonaktifkan pada layar alternatif. Layar sentuh menggunakan presentasi tekan lama milik menu konteks. Beri gaya pada permukaannya dengan `MenuClass`/`MenuStyle` dan pada itemnya dengan `MenuItemClass`/`MenuItemStyle`.

Atur `RightClickAction` ke `RightClickPaste`, `RightClickCopy`, `RightClickCopyOrPaste` (salin seleksi, jika tidak ada maka tempel), atau `RightClickIgnore`. Ignore membiarkan event untuk menu aplikasi di sekelilingnya; gunakan saat menyediakan `ui.ContextMenu` bergaya yang memanggil aksi milik ref.

Impor `tsx/terminal` untuk mendaftarkan `<Terminal>` di `nsgo/base`. Prop reaktif `keybinds` dan `rightClickAction`-nya memperbarui view sambil mempertahankan sesi. Nilai aksi menggunakan nama Ghostty seperti `increase_font_size` dan `copy_to_clipboard`; nilai klik kanan adalah `context-menu`, `paste`, `copy`, `copy-or-paste`, dan `ignore`. Prop gaya menu menggunakan nama Go dalam lower camel case.

## Laporan mouse

Program yang mengaktifkan pelacakan mouse menerima tekanan, pelepasan, seretan, dan hover yang diminta, dikodekan oleh libghostty-vt dalam protokol yang dipilihnya. Shift melewati penangkapan klik dan seret untuk seleksi. `View.MouseReporting` adalah `func() bool` reaktif opsional, bernilai true jika nil; kembalikan false untuk mempertahankan seleksi mouse dan pengguliran scrollback native bahkan saat program meminta penangkapan. Klik kanan milik program diprioritaskan di atas menu konteks di sekelilingnya selama pelaporan diaktifkan.

Pada layar alternatif, dengan mode gulir alternatif diaktifkan dan tanpa permintaan pelacakan mouse, input roda vertikal menjadi tombol kursor, mengikuti mode tombol kursor normal atau aplikasi. `MouseScrollMultiplier{Precision: 1, Discrete: 3}` mengatur jumlah laporan bawaan untuk touchpad dan roda; nol menggunakan nilai bawaan tersebut, dan nilai positif dibatasi pada 0,01–10.000. Input pecahan diakumulasi sampai satu langkah lengkap tersedia. Scrollback normal mempertahankan pengguliran dan inersia native milik platform.

Impor `tsx/terminal` untuk mendaftarkan `<Terminal>` di `nsgo/base`. Prop `mouseReporting`-nya bernilai bawaan true; `mouseScrollPrecision` dan `mouseScrollDiscrete` mengatur pengalinya.


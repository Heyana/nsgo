---
title: Penyiapan
description: Pasang apa yang dibutuhkan NSGo, buat aplikasi, dan jalankan di setiap target.
group: Mulai
order: 10
---

## Persyaratan

- [Go](https://go.dev/dl/) 1.27.3 atau lebih baru.
- Untuk membangun aplikasi iOS, tvOS, dan watchOS, dibutuhkan Mac dengan Xcode. Semua target lainnya dapat dibangun dari macOS dengan Go saja; emulator Android x86_64 juga membutuhkan Android NDK.
- Untuk menjalankan di iPhone, dibutuhkan Apple ID di Accounts Xcode dan iPhone tersebut harus dipercaya (*trusted*), dengan Developer Mode aktif.
- Untuk menjalankan di Android, dibutuhkan `adb` dari platform-tools Android SDK.
- Untuk menjalankan di Windows, cukup Go saja, yang juga membangun paket Windows dan web.
- Di Linux, dibutuhkan GTK 4.14 atau lebih baru, Pango, dan Cairo. Gambar jaringan menggunakan libsoup 3, web view menggunakan WebKitGTK 6.0, dan audio serta video menggunakan plug-in GStreamer. Pada sistem dengan pembatasan user namespace AppArmor, [WebKitGTK membutuhkan profil bubblewrap](debugging.md#linux-web-views).

## Membuat aplikasi

Buat sebuah modul, tambahkan NSGo beserta tool-nya, lalu biarkan tool tersebut membuatkan aplikasi:

```sh
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
```

`go get -tool` menambahkan `nsgo` ke modul satu kali saja, sehingga `go tool nsgo` dapat menjalankannya tanpa memasang apa pun. `init` menghasilkan:

| File | Isinya |
| --- | --- |
| `app.go` | Aplikasi penghitung: komponen `Root` dan `main`, yang memanggil `nsgo.Run(Root)` |
| `app_test.go` | Pengujian yang menekan tombol |
| `nsgo.yaml` | Nama, id, versi, dan target aplikasi |

`go tool nsgo init -template tsx` membuat aplikasi TSX, dan `-template mixed` membuat aplikasi Go yang entry TSX-nya menggunakan komponen Go miliknya sendiri.

### Templat dari modul Go

Berikan path modul templat beserta versi opsionalnya. Versi yang dikosongkan akan merujuk ke versi terbaru yang dapat ditemukan oleh proxy modul Go.

```sh
go tool nsgo init -template example.com/my-template@v1.2.0 -module example.com/hello hello
```

Publikasikan proyek awal di dalam direktori `template/` milik modul. Sertakan `nsgo.yaml` dan `main.go`. Tool akan merender keduanya, ditambah file apa pun yang berakhiran `.tmpl`, menggunakan `text/template` milik Go, lalu menghapus `.tmpl` dari nama file keluaran. File lainnya, termasuk aset biner, disalin apa adanya.

Templat menerima `.Name` (nama tampilan aplikasi), `.ID` (ID aplikasi yang dihasilkan), `.Slug` (nama huruf kecil yang dipisahkan tanda hubung), dan `.Module` (nilai dari `-module`, atau slug). Sertakan `go.mod.tmpl` untuk mendeklarasikan modul dan dependensi aplikasi yang dihasilkan, karena Go mengecualikan file `go.mod` bersarang dari pengunduhan modul. Tanpa file tersebut, `-module` akan menulis `go.mod` minimal yang sama seperti untuk templat bawaan.

## Menjalankannya

```sh
go tool nsgo dev
```

`dev` membangun aplikasi untuk mesin ini dan membukanya. Setiap kali Anda menyimpan perubahan, ia membangun ulang dan memulai ulang aplikasi, dan perubahan TSX saja akan memperbarui aplikasi yang sedang berjalan secara langsung di tempat. Kegagalan build ditampilkan di atas aplikasi, lengkap dengan tombol untuk menyalin pesan error dan membangun ulang.

Jalankan di tempat lain dengan `-target`:

```sh
go tool nsgo dev -target web                # in the browser
go tool nsgo dev -target ios_simulator      # in the iOS simulator
go tool nsgo dev -target tvos_simulator     # in the Apple TV simulator
go tool nsgo dev -target watchos_simulator  # in the Apple Watch simulator
go tool nsgo dev -target android            # on a connected device or emulator
```

`go tool nsgo doctor` mencantumkan setiap target beserta informasi apakah mesin ini dapat membangun dan menjalankannya, serta apa yang kurang untuk target yang belum bisa, seperti Xcode, `adb`, perangkat yang terhubung, atau GTK.

[Menjalankan dan men-debug](debugging.md) membahas perangkat, pelacakan pembaruan, monitor performa, dan bahasa lainnya.

## Dengan agen pemrograman

Skill NSGo mengajarkan agen pemrograman, seperti Claude Code, Codex, dan Cursor, cara menulis, menguji, dan mendistribusikan aplikasi NSGo. Tambahkan ke direktori aplikasi, atau gunakan `-g` untuk setiap proyek:

```sh
bunx skills add nsgo-dev/nsgo
```

Agen yang membaca dokumentasi sendiri dapat memulai dari <https://nsgo.dev/llms.txt>, yang mencantumkan setiap halaman dalam format Markdown di `https://nsgo.dev/docs/<page>.md`; <https://nsgo.dev/llms-full.txt> berisi semuanya. Agen juga dapat menggunakan aplikasi yang sedang berjalan ([Menjalankan dan men-debug](debugging.md#agents)).

## Selanjutnya

- [Contoh: Aplikasi todo](tutorials/todo.md): state, daftar, dan pengujian, dalam sekitar sepuluh menit.
- [Contoh: Aplikasi catatan](tutorials/notes.md): tumpukan navigasi dan catatan yang disimpan di perangkat.
- [Contoh: Repositori GitHub populer](tutorials/repos.md): memuat dari jaringan beserta penanganan error dan coba ulang.

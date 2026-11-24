---
title: Lokalisasi
description: Terjemahkan pesan aplikasi dan ikuti bahasa yang ditampilkannya.
group: Konsep inti
order: 62
---

## Pesan

`locales/` di samping `nsgo.yaml` menyimpan pesan-pesan aplikasi, satu file YAML per bahasa (`en.yaml`, `de.yaml`) dalam ICU MessageFormat:

```yaml
greeting: Hello, {name}!
appName:
  message: Hello
  description: The app's name, for translators.
```

Sebuah pesan berupa string, atau `message` dengan `description` dan `placeholders` untuk penerjemah.

Paket mengimpor `locales/`. `dev`, `build`, dan `test` memeriksa katalog dan menulis satu fungsi Go per pesan di sampingnya, seperti `locales.Greeting(cx, "Ada")`, yang mengucapkannya dalam bahasa yang ditampilkan aplikasi dan mengikutinya saat bahasa berubah; `go tool nsgo i18n` hanya menulis fungsi-fungsinya saja. Pesan memformat angka dan tanggal (`{d, date, medium}`), dan `appName` menamai aplikasi dalam setiap bahasa di paketnya, sebagaimana pesan dengan kunci `apple.info`, seperti `NSCameraUsageDescription`, menerjemahkan entri Info.plist tersebut ([izin](permissions.md#deskripsi-penggunaan)).

## Bahasa

`cx.SetLocale("de")` memilih bahasa, dan tata letak berjalan sesuai arah penulisan bahasa tersebut. Pada aplikasi mandiri, apa yang ditampilkan oleh sistem sendiri—seperti menu pemilihan teks, dialog file, dan widget GTK—juga mengikuti bahasa yang dipaksakan (langsung di Android 13+, Windows, dan Linux, serta mulai peluncuran berikutnya di macOS dan iOS); `cx.SetLocale("")` kembali mengikuti bahasa pengguna.

```sh
go tool nsgo dev -lang en-XA
```

menjalankan aplikasi dalam pseudo locale yang menampilkan teks yang belum diterjemahkan dan teks yang terpotong. Build apa pun menerimanya saat dimulai: `--nsgo-lang=en-XA`.

## Minggu, waktu, dan satuan ukur

`i18n.Regional(cx)` menyatakan bagaimana pengguna membaca minggu, waktu, dan satuan ukur, terpisah dari bahasa: hari dimulainya minggu, format waktu 12 atau 24 jam, sistem pengukuran, dan satuan suhu, sesuai pengaturan mereka di platform, atau jika tidak, sesuai wilayah mereka. Kalender dan pemilih tanggal memulai minggunya di hari tersebut dan kolom waktu menampilkan format jam tersebut kecuali aplikasi mengatur `FirstDayOfWeek` atau `HourCycle`, dan waktu yang ditulis oleh pesan-pesan katalog mengikuti format jam pengguna.

## Kata-kata milik NSGo

Komponen NSGo mengucapkan kata-katanya dalam bahasa-bahasa yang sudah diterjemahkan untuk NSGo. Untuk bahasa lain, kunci yang menamai sebuah paket, seperti `github.com/nsgo-dev/nsgo/base/locales`, memberikan pesan-pesan paket tersebut dalam bahasa file itu. Menu platform, serta peran dan status yang diumumkan pembaca layar Android dan Windows ketika sistem tidak memiliki kata sendiri untuknya, seperti tab atau kotak pencarian, mengucapkan kata-kata dari `github.com/nsgo-dev/nsgo/desktop/locales`.

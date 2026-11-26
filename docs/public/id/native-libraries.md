---
title: Pustaka native
description: Kirimkan pustaka bersama (shared library) yang sudah dibangun dan framework Apple bersama paket Go melalui langkah pengemasan, lisensi, dan penandatanganan normal NSGo.
group: Perangkat bantu
order: 23
---

Sebuah paket Go dapat menyertakan pustaka bersama yang dimuatnya melalui purego dengan menempatkan `nsgo-natives.yaml` di direktori paketnya, dan sejak versi 2 juga framework-nya untuk iOS dan tvOS. Ketika sebuah aplikasi mengimpor paket tersebut, NSGo memilih deklarasinya untuk target, memverifikasi file-filenya, dan menyertakannya sebelum penandatanganan dan pembuatan installer. Aplikasi tidak memerlukan konfigurasi tambahan atau perintah pengemasan.

Build aplikasi desktop dan pengujian target menonaktifkan cgo untuk kode aplikasi. Kode native harus menggunakan purego dan pustaka yang dideklarasikan di `nsgo-natives.yaml`. NSGo menolak paket aplikasi yang berisi file cgo sebelum build dimulai.

## Mendeklarasikan pustaka

```yaml
version: 2
libraries:
  macos_arm64:
    - name: libexample.dylib
      path: lib/macos-arm64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  macos_x64:
    - name: libexample.dylib
      url: https://downloads.example.org/v1.0/macos-x64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  windows_arm64: []
  windows_x64: []
  linux_arm64: []
  linux_x64: []
  ios:
    - name: Example.framework
      url: https://downloads.example.org/v1.0/ios/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  ios_simulator:
    - name: Example.framework
      path: lib/ios-simulator/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  tvos: []
  tvos_simulator: []
```

Versi 2 menerima kunci `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64`, `linux_x64`, `ios`, `ios_simulator`, `tvos`, dan `tvos_simulator`. Versi 1 hanya menerima enam kunci desktop; sebuah file hanya dibaca untuk kunci versinya, sehingga build iOS dan tvOS tidak mengemas apa pun dari file versi 1. Deklarasikan setiap target tempat paket Go dipilih; `[]` secara eksplisit menyatakan bahwa target tersebut tidak memerlukan pustaka yang dibundel. Sebagai alternatif, build tag Go dapat menjauhkan paket dari target yang tidak didukungnya. `macos_universal` mengambil kedua entri macOS, dan target terminal mengambil entri desktop-nya: `tui_macos_arm64` mengambil `macos_arm64`.

Setiap pustaka memiliki:

- `name`: nama file terpasangnya, berakhiran `.dylib` di macOS, `.dll` di Windows, atau `.so` maupun `.so.*` berversi di Linux. Di iOS dan tvOS, nama ini adalah nama framework, `<Name>.framework`, dengan `Name` terdiri dari huruf ASCII, angka, `.`, dan `-`, serta diawali dan diakhiri huruf atau angka.
- Tepat satu dari `path`, sebuah file relatif terhadap direktori paket ini, atau `url`, sebuah URL HTTP(S) untuk pustaka itu sendiri. Versi 2 hanya menerima URL https, dan unduhan gagal bila sebuah pengalihan meninggalkan https. Terbitkan pustaka yang sudah diekstrak, bukan URL arsip, dan untuk framework hanya biner-nya. URL tidak boleh menyematkan nama pengguna dan kata sandi.
- `sha256`: SHA-256 dari file asli, wajib untuk kedua sumber. Misalnya, `shasum -a 256 lib/macos-arm64/libexample.dylib` mencetaknya di macOS.
- `licenses`: path yang tidak kosong ke file lisensi dan notice asli, relatif terhadap direktori paket ini. Teks-teks ini dikirim dalam `THIRD_PARTY_NOTICES.txt` dan mencapai `core.ThirdPartyNotices()` pada build rilis. [Kebijakan lisensi](building.md#pemberitahuan-lisensi) aplikasi yang sudah ada, termasuk pengecualian modul yang persis, berlaku secara terpisah untuk lisensi native.

Path lokal dan tautan simbolik tetap berada di dalam paket yang mendeklarasikannya. NSGo menolak field yang tidak dikenal, ketidakcocokan checksum, file untuk sistem operasi atau arsitektur lain, dan nama terpasang yang bertentangan. Dua paket dapat berbagi file bernama identik dengan checksum yang sama. Sebuah file dibatasi hingga 1 GiB.

## Mengemas dan memuat

| Target | Direktori pustaka |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows, Linux, dan target terminal | Di samping file executable |
| iOS dan tvOS | `<app>.app/Frameworks/<Name>.framework/<Name>` |

Paket memuat pustakanya dari direktori tersebut, menggunakan lokasi file executable alih-alih direktori kerja. Bangun dependensi terlebih dahulu agar pustaka pendampingnya dimuat dari direktori tempat dependensi itu dikirim. Deklarasikan setiap file pendamping yang diperlukan dan setiap nama file yang dibutuhkan loader.

NSGo memeriksa ulang unduhan yang di-cache sebelum digunakan kembali dan mengoordinasikan build yang berjalan bersamaan melalui kunci file. Cache konten berada di bawah direktori user-cache milik sistem operasi, di `nsgo/natives`. File lokal diverifikasi saat disalin.

Paket macOS universal memerlukan nama pustaka yang sama untuk kedua arsitektur. NSGo menggabungkan slice-slice tersebut dengan `lipo`; pustaka dari penerbit yang sudah universal dapat melayani entri mana pun. Pustaka ditandatangani sebelum aplikasi, lalu melewati langkah disk-image, penandatanganan, dan notarisasi normal aplikasi. Penandatanganan Windows menyertakan DLL yang dideklarasikan bahkan dengan `build.installers: false`. Installer Linux menyertakan file dan notice yang sama seperti direktori file executable. Di Mac, target terminal menandatangani pustaka macOS secara ad hoc; di sistem lain pustaka dikirim apa adanya, pustaka universal secara utuh, dan pustaka arm64 yang belum ditandatangani ditolak.

Pengembangan, `-run`, dan pengujian target menggunakan langkah pemilihan paket dan penyalinan yang sama, baik di desktop maupun di iOS dan tvOS. Deklarasi dipilih berdasarkan target, tag, dan overlay Go yang dihasilkan dari build, termasuk penggantian lokal dan paket yang di-vendor. Android, HarmonyOS, watchOS, web, dan program mini tidak mengemas apa pun dari file ini.

## Framework iOS dan tvOS

NSGo memeriksa biner setiap framework dan menghentikan build dengan menyebutkan alasannya, kecuali biner tersebut:

- adalah pustaka dinamis Mach-O arm64 yang dibangun untuk platform target, perangkat atau simulator, sebagaimana dinyatakan `LC_BUILD_VERSION`-nya; file universal menyumbangkan slice arm64-nya; arm64e ditolak;
- membutuhkan OS minimum yang tidak lebih baru daripada milik aplikasi, yaitu `apple.info.MinimumOSVersion` atau jika tidak ada, iOS 15.0 dan tvOS 17.0 ([Membangun dan mendistribusikan](building.md#nsgoyaml));
- memiliki nama instalasi `@rpath/<Name>.framework/<Name>`;
- hanya memuat melalui `@rpath` framework yang dideklarasikan paket-paket aplikasi untuk target yang sama.

NSGo menulis `Info.plist` framework tersebut, dengan `CFBundleIdentifier` berupa ID aplikasi, sebuah titik, dan `Name` dalam huruf kecil, beserta platform dan OS minimum aplikasi. Build simulator menandatangani framework secara ad hoc. Build perangkat menyerahkannya kepada penandatanganan aplikasi: `sign.profile` menandatanganinya dengan identitas aplikasi sebelum aplikasi itu sendiri, seperti yang dilakukan Xcode untuk `nsgo build -run` dan `nsgo dev` di perangkat. Framework yang memuat framework pendampingnya memerlukan run path `@loader_path/..` atau `@executable_path/Frameworks` untuk menemukannya di `Frameworks`, karena executable aplikasi tidak memilikinya, dan tidak memuat hal lain melalui `@rpath` kecuali memiliki run path sistem seperti `/usr/lib/swift`. Framework yang bundle ID-nya sudah dimiliki bundle lain di aplikasi, tanpa membedakan huruf besar-kecil, seperti `StoreKit.framework` di samping `NSGoStoreKit.framework` milik NSGo, menghentikan build.

`nsgo build -embed` berhenti dan menyebutkan nama paketnya ketika aplikasi menautkan paket yang mendeklarasikan framework iOS: XCFramework untuk aplikasi host belum dapat membawanya.

## Mempertahankan file saat vendoring

Operasi vendor Go memerlukan data tertanam agar file di bawah direktori paket tetap dipertahankan. Sebuah paket dapat menandai manifest dan lisensinya untuk disertakan tanpa menggunakan byte-nya saat runtime:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

Sertakan juga path pustaka lokal dalam direktif embed tersebut saat menggunakan `path`, misalnya `lib/*`. Unduhan URL hanya memerlukan file manifest dan lisensi di dalam paket Go.

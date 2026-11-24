---
title: Pengaturan pengembangan
description: Bangun NSGo dari sumber, jalankan galeri, dan uji perubahan.
group: Berkontribusi
order: 80
---

## Persyaratan

macOS dengan Xcode dan Go 1.27.3 atau yang lebih baru membangun setiap target untuk arm64, termasuk APK Android. Menjalankan di perangkat membutuhkan apa yang tercantum di [Pengaturan](getting-started.md#requirements), dan target emulator Android x86_64, `android_x64`, membutuhkan Android NDK, karena Go hanya menautkan android/amd64 dengan cgo. Installer Windows membutuhkan NSIS (`brew install makensis`); tanpanya `build` hanya menyisakan skripnya.

```sh
git clone https://github.com/nsgo-dev/nsgo.git
cd nsgo
```

Baca [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) dan [AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) sebelum mengubah kode.

## Galeri

Galeri berisi demo untuk setiap elemen, komponen `base` dan `ui`, serta layanan. Jendela lebar menampilkan demo di bawah tab di sidebar; jendela sempit menampilkan tab dan pengaturan di bilah bawah, dengan demo didorong ke atasnya. Jalankan dari root repositori:

```sh
go tool nsgo dev examples/gallery                   # nsgo.yaml's dev target, exiting with the app
go tool nsgo dev examples/gallery-tsx               # the gallery's components and apps, written in TSX
go tool nsgo dev -target web examples/gallery       # in the browser, serving until interrupted
go tool nsgo dev -target ios_simulator examples/gallery
go tool nsgo dev -target ios examples/gallery       # on the connected iPhone, signed by Xcode
go tool nsgo dev -target android examples/gallery   # on the connected device or emulator, over adb
go tool nsgo build -target all examples/gallery     # every platform's packages in examples/gallery/dist, what to hand out in dist/releases
```

## Periksa dan uji

```sh
go tool nsgo check                 # vet every package
go tool nsgo check -target all     # vet every available Go/Xcode target instead
go tool nsgo test                  # every package's tests
go tool nsgo test -bench . ./core  # core's benchmarks
(cd benchmark && go run ./runner)  # measure against other frameworks
# capacity curve for NSGo's 600-cell grid, without rebuilding it
(cd benchmark && go run ./runner -frameworks nsgo -cases grid -grid-cols 20 -grid-rows 15 -out /tmp/grid.json)
```

`-target` memeriksa target yang disebutkannya alih-alih host dan, di repositori NSGo, klien dev, yang diperiksa oleh `check` biasa. Untuk memeriksa platform yang disentuh perubahan, sebutkan semuanya dalam satu pemeriksaan, seperti `check -target web,android -jobs 2`, alih-alih memulai beberapa sekaligus: `-jobs n` memeriksa hingga n target secara bersamaan, masing-masing `go vet` dengan n worker kompilasi, dengan bawaannya adalah satu target sekaligus memakai bawaan Go, dan pemeriksaan yang dimulai saat pemeriksaan lain berjalan di modul yang sama akan menunggunya dan memberitahukannya. `check.yml` di CI memeriksa setiap target.

Dijalankan dari root repositori, atau di `internal/dev`, perintah headless menguji `internal/dev` untuk kedua kalinya dengan build tag `nsgodev`, yang dibutuhkan oleh file klien dev, sehingga pengujiannya berjalan secara lokal dan di CI. Kedua eksekusi menerima filter dan flag yang sama serta berbagi satu penghitungan dan laporan JUnit, dengan paket eksekusi kedua bernama `internal/dev [nsgodev]`.

`check -target all` memeriksa target macOS, Windows, Linux, Android arm64, dan web di host mana saja. Perintah ini juga memeriksa iOS, tvOS, dan watchOS ketika SDK Xcode-nya tersedia, dan mencantumkan yang dilewati. Target yang disebutkan secara eksplisit, seperti `check -target ios`, akan gagal jika alatnya tidak tersedia, termasuk ketika digabung dengan `all`. Target yang gagal tidak menghentikan pemeriksaan target tersisa, dan perintah gagal jika ada pemeriksaan wajib yang gagal. `build -target all` tetap membutuhkan macOS dengan Xcode. Di repositori NSGo sendiri, setiap target yang disebutkan juga memeriksa host pengujian platformnya dengan `nsgotest`; CI menyebutkan target pratinjau `harmonyos` secara eksplisit, karena berada di luar `all`.

Pekerjaan CI Linux menjalankan pengujian `js/wasm` backend Go HarmonyOS dengan Node.js dan peluncur Wasm Go. Jalankan pengujian galeri secara lokal dengan emulator HarmonyOS yang terhubung melalui `hdc`. Mengompilasi [templat host HarmonyOS](building.md#harmonyos) membutuhkan OpenHarmony SDK API 26.

```sh
go tool nsgo test -env GOOS=js -env GOARCH=wasm ./internal/platform/harmonyos \
  -- "-exec=env -i \"PATH=$PATH\" \"$(go env GOROOT)/lib/wasm/go_js_wasm_exec\"" -count=1
go tool nsgo test -target harmonyos -run TestHarmonyOSPages -timeout 10m ./examples/gallery
```

Pada sesi Wayland di desktop Linux, jalankan pemeriksaan penerimaan portal pintasan global nyata dengan `NSGO_TEST_GLOBAL_SHORTCUT_PORTAL=<installed-desktop-id> go tool nsgo test -run TestGlobalShortcutPortalWithPermission ./internal/platform/linux`. ID tersebut harus memiliki entri `.desktop`. Berikan izin Ctrl+Alt+K pada dialog compositor, lalu tekan kombinasi tombol itu saat aplikasi lain sedang fokus; pemeriksaan ini menunggu pendaftaran maupun aktivasi. Pemeriksaan dilewati kecuali variabel lingkungan tersebut diatur.

Dengan Docker tersedia di host Linux, `go tool nsgo test -run '^TestLinuxPackagesNative$' -env NSGO_TEST_LINUX_PACKAGES=1 ./cmd/nsgo -- -timeout=20m` memeriksa pemasangan nyata di dalam kontainer Debian, Ubuntu, dan Arch sekali pakai. Pengujian ini mengunduh image resminya dan memasang dependensi GTK di dalamnya, memverifikasi setiap basis data paket serta file yang terpasang, lalu menghapus paketnya. Basis data paket host tidak pernah digunakan.

## Rilis

NSGo merilis sesuai kebutuhan ketika proyek hilir membutuhkan versi bertag atau para pengelola memiliki perubahan yang siap dikirim. Saat meminta rilis untuk sebuah pustaka, tautkan pekerjaan yang bergantung padanya dan sebutkan commit yang berisi perubahan NSGo terakhir yang dibutuhkannya. Tag rilis pertama yang mengandung commit tersebut adalah versi yang dapat diminta oleh pustaka itu.

Untuk menguji perubahan yang sudah di-merge sebelum diberi tag, jalankan `go get github.com/nsgo-dev/nsgo@main` dari modul yang bergantung padanya. Anda dapat menggunakan ID commit sebagai pengganti `main` untuk memilih revisi yang telah Anda verifikasi. Go mencatat pseudo-version hasilnya di `go.mod`.

Seorang pengelola memberikan versi dan tanggal pada perubahan-perubahan tersebut di `CHANGELOG.md`, menyisakan bagian Unreleased yang baru, dan memberi tag pada commit hasilnya di `main`. [Alur kerja rilis](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/release.yml) memeriksa tag tersebut dan menerbitkannya setelah alur kerja pengujian lolos; catatan yang diterbitkan kemudian berpindah dari `CHANGELOG.md`—yang hanya menyimpan perubahan yang belum dirilis—ke [Rilis](https://github.com/nsgo-dev/nsgo/releases).

## NSH

NSGo membaca [Native Shell](https://github.com/longbridge/nsh) sebagai referensi desain; tidak ada pengujiannya yang membutuhkannya. Untuk membacanya, klon di samping repositori dan checkout tag rilis terbarunya, jangan pernah `main`:

```sh
git clone https://github.com/longbridge/nsh.git ../nsh
git -C ../nsh checkout "$(git -C ../nsh describe --tags --abbrev=0)"
```

## Berikutnya

- [Basis kode](contributing/codebase.md): di mana setiap bagian NSGo berada.
- [Membuat perubahan](contributing/changes.md): apa yang dibutuhkan sebuah pull request.

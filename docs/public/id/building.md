---
title: Membangun dan mendistribusikan
description: "Memaketkan aplikasi untuk setiap platform, membuat penginstal, menandatangani dan melakukan notarisasi, menerbitkannya di GitHub, serta menyematkannya di aplikasi lain."
group: Perangkat bantu
order: 22
---

## Membangun

```sh
go tool nsgo build
```

`build` menulis paket rilis yang telah di-strip untuk setiap target yang tercantum di `nsgo.yaml`, ke dalam `dist/<name>-<version>-<target>`: berupa `.app`, `.ipa`, `.apk`, direktori Windows dan Linux, atau situs web yang halamannya mengambil setiap modul TSX yang disebutkan `import()` saat impor berjalan. `-target` menggantikan daftar tersebut, dan `all` berarti semua target yang hanya memerlukan Go dan Xcode; tambahkan `android_x64` apabila Android NDK terpasang, dan `miniprogram`, atau `miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay`, maupun `miniprogram_douyin` saja untuk [program mini](#program-mini), serta `harmonyos` untuk pratinjau [HarmonyOS](#harmonyos). `-run` menjalankan paket yang dibangun untuk mesin ini, atau memasangnya di iPhone, simulator, atau perangkat Android yang dipilih `-device`.

Semua hasil yang siap didistribusikan berakhir di `dist/releases`, satu-satunya direktori yang perlu Anda periksa:

| Di `dist/releases` | Keterangan |
| --- | --- |
| `*.dmg`, `*-setup.exe`, `*.deb`, `*.pkg.tar.gz`, `*.AppImage` | [Penginstal](#penginstal) untuk macOS, Windows, dan Linux |
| `*.AppImage.zsync` | Kontrol pembaruan delta untuk AppImageUpdate dan peluncur yang kompatibel |
| `*-ios.ipa`, `*-tvos.ipa`, `*.apk` | Paket iOS, tvOS, dan Android, untuk toko aplikasi atau perangkat |
| `*.pkg`, `*.msix`, `*.aab`, `*.flatpak`, `*.tar.gz` dan `flatpak/` | [Paket toko aplikasi](#paket-toko-aplikasi) dari build `store` |
| `install.sh`, `install.ps1` | Skrip yang memasang aplikasi dari [rilis GitHub](#publikasikan-di-github) |
| `appcast-<target>.xml` | Umpan yang dibaca aplikasi untuk [memperbarui diri](#pembaruan) |
| `homebrew/<name>.rb`, `winget/` | [Homebrew cask dan manifes winget](#saluran) untuk rilis langsung |
| `*-web.zip` | Situs web, siap diekstrak dan disajikan apa adanya |
| `miniprogram/<platform>/` | Proyek masing-masing program mini, untuk diunggah dari alat pengembangnya |

Setiap file dinamai `<name>-<version>-<system>-<arch>`, seperti `hello-1.2.0-windows-x64-setup.exe`, sehingga pengguna mudah menemukan versi untuk sistem mereka di halaman rilis, dan memiliki `.sha256` di sebelahnya. Build versi lain akan mengosongkan direktori tersebut terlebih dahulu; build untuk versi yang sama, termasuk di mesin lain, akan ditambahkan ke dalamnya. Paket simulator dan watchOS tetap berada di direktori `dist/<name>-<version>-<target>` masing-masing.

Situs web menyertakan `.wasm.br` dan `.wasm.gz` apabila ukurannya lebih kecil daripada modul aslinya. Aktifkan file terkompresi awal (*precompressed files*) di host yang mendukungnya. Sajikan pada URL `.wasm` asli dengan `Content-Type: application/wasm`, `Content-Encoding` yang sesuai, dan `Vary: Accept-Encoding`. Modul aslinya dapat berjalan di host statis mana pun. [Publikasi di web](web.md) membahas tajuk semantik, tautan peramban, metadata per halaman, dan pemuatan CDN.

### Paket web luring

Tetapkan `web.offline: true` di `nsgo.yaml` untuk menyimpan cache rilis web pada peluncuran daring pertamanya.

```yaml
web:
  offline: true
```

Sajikan direktori yang dipublikasikan melalui HTTPS, atau gunakan `go tool nsgo build -target web -run` di localhost. Service worker menyimpan cache halaman, WebAssembly, skrip, dan sumber daya yang dipaketkan. Kunjungan daring akan memuat versi yang dipublikasikan. Setelah instalasi berhasil dan tab dari rilis sebelumnya ditutup, rilis terbaru dapat diluncurkan dari cache-nya. Permintaan ke layanan daring aplikasi tetap memerlukan koneksi.

## Terminal

Jalankan aplikasi di terminal interaktif dengan `go tool nsgo dev -target tui`. `go tool nsgo test -target tui` menjalankan pengujiannya di sana melalui `nsgotest.Main`, satu aplikasi per proses pengujian. Terminal mengendalikan ukurannya: satu sel mewakili 8 × 16 poin tata letak. Pilih gaya aplikasi Anda secara eksplisit; komponen mempertahankan bentuk dan ukurannya yang biasa. `examples/tui` mendemonstrasikan kolom input, tombol, pintasan, dan pengubahan ukuran.

Bangun berkas eksekusi konsol dengan `go tool nsgo build -target tui_macos_arm64`, atau `tui_macos_x64`, `tui_windows_arm64`, `tui_windows_x64`, `tui_linux_arm64`, maupun `tui_linux_x64` yang sesuai. Grup build/check `tui` mencakup keenamnya, dan `all` juga menyertakannya. Setiap direktori paket berisi berkas eksekusi dan sumber daya; tidak ada penginstal GUI. Keenamnya dibangun dengan Go saja.

Backend mendukung teks sel, latar belakang, batas padat (*solid borders*), pemotongan (*clipping*), pengguliran, fokus, dan penyuntingan teks. Backend ini menggunakan font dan warna milik terminal; Control menjadi `mod`, termasuk di Mac. Ctrl+C menyalin di dalam editor dan keluar apabila tidak ada ikatan (*binding*) atau editor yang menanganinya. Tempel berurung (*bracketed paste*) menyisipkan teks tanpa menjalankan pintasan, dan papan klip bersifat lokal per proses. Kanvas, gambar, media, komponen yang dihosting secara native, menu/peringatan sistem, animasi, zoom, pembentukan dua arah (*bidirectional shaping*), elipsis awal/tengah, dan jembatan papan klip sistem belum diimplementasikan. `base/terminal` tetap menjadi kontrol terminal yang disematkan dalam aplikasi GUI.

Selama `nsgo dev` menguasai terminal, diagnostik alat dan aplikasi masuk ke `.nsgo/tui.log`. Saat keluar, layar, kursor, dan mode terminal dipulihkan sebelum kembali ke shell. Tangkapan layar pengujian terminal merasterisasi sel menggunakan font portabel tetap alih-alih menangkap emulator; gunakan asersi teks untuk glif Unicode yang tidak ada di font tersebut.

## nsgo.yaml

```yaml
name: Hello
id: dev.example.hello   # the bundle ID; without it, dev.nsgo. and the name's letters and digits
version: 0.1.0
build_number: 1         # CFBundleVersion and Android's versionCode; else from version
# copyright: © 2026 Example  # the Windows executable's details and installer, and the macOS About panel
# appearance: dark      # light or dark whatever the system's
# theme: theme.json     # a TSX app's theme: NSGo's or a shadcn/ui one
# i18n: { dir: locales, source: en }  # the app's catalogs and the language they are written in
icon:
  apple: assets/AppIcon.icon  # macOS, iOS and watchOS, or an .icns for macOS
  windows: assets/IconWindows.png  # optional Windows artwork
  linux: assets/IconLinux.png      # optional Linux artwork
  png: assets/Icon.png        # every other target
  tvos: assets/AppIconTV.brandassets  # the Apple TV's layered icon and top shelf art, or a wide PNG
  watchos: assets/AppIconWatch.icon   # the Apple Watch's own, drawn in circles, or a square PNG
  android:
    png: assets/IconAndroid.png  # optional bitmap; valid without adaptive layers
    foreground: assets/IconForeground.png  # Android's adaptive icon: 432 px, the logo within the center 264 px
    background: "#1F1F1F"                 # a color or a square PNG
    monochrome: assets/IconMonochrome.png  # optional: the shape Android 13's themed icons tint
dev:
  target: native        # native, web, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android, harmonyos or miniprogram_<platform>
  hot_reload: true      # false builds once, watching no files
  # icon: { png: assets/Icon-dev.png }  # replaces icon's files in dev's builds; defaults to assets/*-dev.*
build:
  output: dist
  tags: []              # Go build tags every build, test and check of the app takes, such as timetzdata
  ldflags: []           # the app's own linker flags, such as -X main.releaseDate=2026-11-17
  open: false           # true opens the output directory after building
  installers: true      # false leaves out the disk image, the Windows installer and the Linux packages
  jobs: 0               # how many targets build at once; 0 is half the CPUs, 1 one after another
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android_arm64, web]
# apple: { sign_in_with_apple: true }  # the Sign in with Apple entitlement, for iOS, tvOS and Mac App Store builds
# apple: { purchases: true }  # StoreKit's library, which in-app purchases go through, in macOS, iOS and tvOS packages
# apple: { privacy: { accessed_apis: { NSPrivacyAccessedAPICategoryDiskSpace: [E174.1] }, tracking: false } }  # add required-reason APIs and tracking declarations to PrivacyInfo.xcprivacy
# apple: { requires_full_screen: false }  # let an iPad run the app in Split View, Slide Over and Stage Manager, in all four orientations
# apple: { orientations: [portrait, landscape_left] }  # the orientations a full-screen app takes on an iPad
# apple: { multiple_scenes: true }  # let an iPad open more than one window of the app, each showing the main view with its own state
# apple: { info: { UIBackgroundModes: [audio] } }  # background playback for iOS Picture in Picture
# android: { permissions: [REQUEST_INSTALL_PACKAGES] }  # manifest permissions beyond the runtime's
# android: { recording: true }  # the microphone service microphone.RecordWAV records in, with RECORD_AUDIO, the foreground-service permissions and POST_NOTIFICATIONS, which RecordWAV asks for so its notification shows from Android 13; a refusal still records
# android: { uses_cleartext_traffic: true }  # allow plain HTTP to any host in the APK and App Bundle
# android: { background_downloads: true }  # dataSync foreground service for cx.Download
# android: { background_motion: true }  # explicit finite motion shortService; streams also need a notification title
# android: { loopback_cleartext: true }  # allow plain HTTP to loopback in the APK and App Bundle
# commands: [{ name: hello-cli, package: ./cmd/hello-cli }]  # command-line programs the desktop packages ship beside the app: see Commands
# windows: { command_line: true }  # add <name>.com, a console companion that forwards arguments and streams to <name>.exe; not in store builds
# windows: { path: true }  # the installer puts the app's folder on the user's Path, so consoles find its commands
# windows: { publisher: Example Ltd }  # the company the Windows executable's details, its installer and the uninstall entry name; else windows.store.publisher_name
# linux: { maintainer: "Jane Doe <jane@example.com>" }  # the Linux packages' maintainer, else the app's name
# linux: { categories: [Office, Dictionary] }  # the freedesktop menu categories of the app's desktop entry, at least one a main category such as Office, Utility or Game, else menus file it under Other; your own start with X-
# linux: { share: packaging/share }  # files the .deb and Arch package install under /usr/share and the AppImage in its usr/share, such as metainfo/<id>.metainfo.xml or bash-completion/completions/<command>
# linux: { depends: { deb: [gstreamer1.0-plugins-good], arch: [gst-plugins-good] }, recommends: { deb: [gstreamer1.0-libav], arch: ["gst-libav: H.264 and AAC video"] } }  # packages the .deb and Arch package also depend on and recommend, after GTK 4 and WebKitGTK 6, each as dpkg or pacman writes it
# release: { github: me/hello }  # the GitHub repository the install scripts and updates install from, else the Git remote origin's; tag_prefix: v by default
# updates: { key: <public key> }  # how the app updates, by platform and channel: see Updates
# store: { dir: store }           # store listings directory (YAML and Markdown per language)
# miniprogram: { appid: { wechat: wx0123456789abcdef, alipay: "2021000000000000" }, custom_navigation: false }  # the mini programs'
```

Nama OS tanpa arsitektur pada `targets` — atau di `nsgo build -target` dan `nsgo check -target` — berarti mencakup semua arsitekturnya (`windows` berarti `windows_arm64` dan `windows_x64`; `android` hanya `android_arm64`, karena build x86_64 membutuhkan NDK). `macos_universal`, yang tidak disertakan oleh `macos` dan `all`, membangun kedua arsitektur macOS lalu menggabungkannya menjadi satu aplikasi, termasuk pustaka dan widget-nya. Saluran Steam menggunakan bundel aplikasi universal tersebut. Build penginstal membungkusnya dalam satu disk image, dengan satu umpan Sparkle pada saluran langsung (*direct*). `nsgo test -target` juga menerima nama arsitektur tempat pengujiannya berjalan: untuk OS desktop pada arsitektur mesin ini dan Android pada `arm64`, sehingga `windows_x64` menjalankan target `windows` di mesin Windows x64, sedangkan arsitektur yang tidak didukung target tersebut akan ditolak dengan pesan berisi nama yang harus digunakan.

Flag menggantikan nilai yang dinamainya, seperti `-target`, `-version`, atau `-output`; `-h` mencantumkan semuanya.

`build.ldflags` menampung flag linker milik aplikasi itu sendiri, masing-masing ditulis seperti yang diterima `-ldflags` milik `go build`, misalnya `-X main.releaseDate=2026-11-17` atau `-X 'main.channel=beta builds'`. Setiap build yang menautkan aplikasi akan menggunakannya: `dev` beserta muat ulangnya, `build` termasuk dengan `-embed`, dan `test` termasuk dengan `-target`, pada semua target. `-ldflags` pada `build` dan `test` menambahkannya alih-alih menggantikannya, dan dapat diulang untuk menambah lebih banyak. Flag ini ditempatkan sebelum flag `-X` milik alat, dan linker menyimpan nilai terakhir suatu variabel, sehingga nama, ID, dan versi di `core.App` tetap mengikuti `nsgo.yaml`. Alat ini meneruskan `-ldflags`-nya sendiri ke `go`, yang menggantikan apa yang ada di `GOFLAGS`, sehingga flag linker milik aplikasi sebaiknya ditaruh di sini; `-ldflags` milik `test` headless setelah direktori merupakan milik `go test` dan menggantikan semuanya.

Setiap perintah `go` yang dijalankan alat untuk aplikasi menggunakan `build.tags`, baik di `dev` dan muat ulangnya, `build` termasuk dengan `-embed`, `test` termasuk dengan `-target`, maupun `check`, pada semua target. Tag milik NSGo sendiri, seperti `nsgodev` di `dev` atau `nsgotest` di pengujian target, ditambahkan ke dalamnya, begitu pula tag dari `-tags` di `GOFLAGS` yang jika tidak demikian akan tergantikan; bagian lain dari `GOFLAGS` berlaku sebagaimana adanya.

`apple.info.LSMinimumSystemVersion` menaikkan versi minimum paket macOS, dan `apple.info.MinimumOSVersion` menaikkan versi minimum paket iOS, tvOS, atau watchOS yang dipilih. Tulis string versi dalam tanda kutip, seperti `"14.2"` atau `"18.0"`, dengan satu hingga tiga bagian angka. Batas bawah runtime adalah macOS 13.0 (Go 1.27), iOS 15.0, tvOS 17.0, dan watchOS 26.0; nilai yang lebih rendah atau salah format akan gagal. Kunci mobile harus memenuhi batas bawah setiap target mobile yang dipilih. App Clip tetap memakai `app_clip.minimum` masing-masing. Nilai-nilai ini diteruskan ke metadata paket, kompilasi aset, proyek penandatanganan, dan linker aplikasi; kerangka iOS tersemat (*embedded frameworks*) juga menggunakannya, dengan Mac Catalyst minimal 16.0 untuk runtime macOS 13 milik Go.

`icon.windows` dan `icon.linux` memilih PNG persegi untuk paket aplikasi, gambar penginstal, dan logo toko aplikasi masing-masing. Secara bawaan nilainya adalah `assets/IconWindows.png` dan `assets/IconLinux.png`, lalu mundur ke `icon.png`; `dev.icon` menerima pengaturan yang sama dan file bawaan `-dev`. `icon.android.png` memilih bitmap Android secara independen, termasuk saat tidak ada lapisan adaptif yang dikonfigurasi. Target lain tetap menggunakan fallback bersama `icon.png`.

Ikon Apple TV berbentuk kartu berlapis, bukan persegi: `tvos` menerima `.brandassets` milik Xcode—tumpukan `App Icon` dan gambar *top shelf*-nya—atau gambar lebar seperti 1280×768, yang akan digambar oleh alat agar pas pada kartu di atas warna tepinya dan dipusatkan pada gambar *top shelf*. Tanpa keduanya, PNG ditempatkan pada tiga perempat tinggi kartu. Apple Watch memotong ikonnya menjadi lingkaran, sehingga `watchos` menerima `.icon` yang digambar untuk lingkaran watchOS atau PNG persegi; tanpanya aplikasi menggunakan `apple`, lalu `png`. Keduanya mundur ke `assets/AppIconTV.*` dan `assets/AppIconWatch.*`, seperti halnya `apple` dan `png`.

Di Android, `icon.android.foreground` dan `background` membuat ikon peluncur menjadi adaptif, sehingga peluncur memotongnya sesuai bentuknya dan menggerakkannya sesuai keinginan alih-alih mengecilkan `png` ke atas pelat tersendiri. Setiap lapisan berukuran 108 dp persegi, berupa PNG berskala 432 px; peluncur menampilkan bagian tengah 72 dp (288 px), dan memotongnya ke bentuk yang dapat memotong apa pun di luar bagian tengah 66 dp (264 px), jadi jaga logo tetap berada di dalam lingkaran tersebut dan biarkan latar belakang mengisi seluruh persegi. `background` berupa PNG atau warna seperti `"#1F1F1F"`. `monochrome`, yaitu bentuk yang hanya mengandalkan saluran alfa, adalah yang diwarnai oleh Android 13 dan versi lebih baru ketika pengguna mengaktifkan ikon bertema; tanpanya, ikon mempertahankan warnanya di sana. Paket tetap menyimpan `icon.android.png`, atau `icon.png`, sebagai ikon bitmap biasa, yang tidak akan dipilih peluncur Android 9 atau lebih baru (versi tertua tempat NSGo berjalan) ketimbang ikon adaptif; tanpa `png`, alat menggambarnya dari foreground di atas background. Tanpa lapisan adaptif, peluncur menampilkan bitmap Android atau `png` bersama.

`android.uses_cleartext_traffic: true` mengizinkan APK atau App Bundle menggunakan HTTP biasa ke host mana pun, dengan mengatur flag manifes sekaligus kebijakan keamanan jaringannya. Tanpa pengaturan tersebut, `android.loopback_cleartext: true` mengizinkan HTTP biasa hanya dari `localhost`, `127.0.0.1`, dan `::1`, misalnya untuk video yang disajikan oleh aplikasi itu sendiri. Kebijakan khusus loopback menolak cleartext ke host lain; izin untuk semua host didahulukan apabila kedua pengaturan bernilai true. Untuk AAR yang disematkan, konfigurasikan kebijakan keamanan jaringan Android milik aplikasi host.

Setiap build juga menerima flag milik NSGo sendiri saat dijalankan, seperti `--nsgo-fps` ([Menjalankan dan debugging](debugging.md#flag-pada-setiap-build)). Di `nsgo dev`, Command/Control+R memuat ulang dari setiap jendela yang terbuka dan tetap tersedia setelah tampilan utama ditutup apabila ada jendela lain yang mempertahankan fungsi aplikasi tetap berjalan.

### Layar peluncuran

Atur `launch_screen` untuk menampilkan latar belakang buram (*opaque*), PNG di tengah, atau keduanya pada iOS dan Android 12 ke atas. Jalur gambar bersifat relatif terhadap `nsgo.yaml`. Gambar disesuaikan secara proporsional di dalam 128 × 128 poin atau dp. PNG transparan dengan sisi terpanjang minimal 512 piksel menjaganya tetap tajam pada layar berdensitas tinggi.

```yaml
launch_screen:
  background: "#F5F1EA"
  image: assets/Launch.png
  dark:
    background: "#201C26"
    image: assets/LaunchDark.png
```

Sistem memilih tampilan (terang/gelap) kecuali jika `appearance` menguncinya. Kosongkan salah satu field `dark` untuk memakai ulang nilai yang sesuai. Latar belakang atau gambar yang dihilangkan mempertahankan bawaan platform. Aplikasi tersemat memperoleh layar peluncurannya dari host.

## Kelas native Android

Cantumkan kelas Android yang telah dikompilasi sebelumnya di `nsgo.yaml` aplikasi untuk menggunakan View kustom dari `core.Native`.

```yaml
android:
  classes: [java/BadgeView.jar, java/Helpers.dex]
```

Jalur bersifat relatif terhadap `nsgo.yaml` kecuali jika absolut. Setiap jalur menunjuk file `.dex` atau JAR yang berisi `classes.dex`, `classes2.dex`, dan shard berikutnya di root hasil D8. Siapkan file tersebut dengan D8 menggunakan `--min-api 28`. Pada arm64, NSGo membangun APK atau App Bundle dari file yang telah dikompilasi ini dengan Go saja. Sesi pengembangan Android membangun ulang apabila file yang tercantum berubah. Untuk AAR yang disematkan, build Android milik host menyediakan dependensi Java-nya. [Dokumentasi D8](https://developer.android.com/tools/d8)

[Contoh native Android](https://github.com/nsgo-dev/nsgo/tree/main/examples/android-native) mendaftarkan sebuah kelas dengan konstruktor `Context` publik dan mengonfigurasinya dengan prop Go.

## Gim

Tetapkan `game: true` di `nsgo.yaml` untuk sebuah gim. Android mengklasifikasikan aplikasi sebagai gim dan memakai pengoptimalan Game Mode sistem. macOS dan iOS ikut serta dalam Game Mode, macOS secara bawaan masuk kategori Games, dan Linux menambahkan `Game` ke entri desktop. `macos.category` eksplisit atau kunci Game Mode di `apple.info` diutamakan.

Tetapkan `gamepad: true` apabila aplikasi Anda menangani gamepad perangkat keras. Android mendeklarasikan baik gamepad maupun layar sentuh sebagai opsional. macOS, iOS, dan tvOS mendeklarasikan profil Extended Gamepad, dan tvOS juga mendeklarasikan Micro Gamepad milik Siri Remote, dengan kunci kontroler di `apple.info` lebih diutamakan.

## Pemberitahuan lisensi

`nsgo build` menempatkan `THIRD_PARTY_NOTICES.txt` di dalam sumber daya paket mandiri. File ini memuat teks lisensi dari modul Go yang digunakan oleh target tersebut, runtime Go, serta runtime Java atau JavaScript yang dibundel NSGo. Build web menamai file tersebut berdasarkan kontennya, sama seperti sumber daya lainnya. Gunakan `core.ThirdPartyNotices()` untuk menampilkan teks yang sama di layar kredit dalam aplikasi. Fungsi ini mengembalikan string kosong saat pengembangan, pengujian, dan build pustaka tersemat. [Deklarasi pustaka native](native-libraries.md) menambahkan teks lisensi aslinya ke pemberitahuan yang sama.

Secara default, lisensi pihak ketiga yang tidak dikenal atau hilang menggagalkan build, dan lisensi copyleft yang terdeteksi memunculkan peringatan. Modul yang tercantum dalam workspace Go aplikasi (`go.work`) diperlakukan sebagai kode pihak pertama bersama modul aplikasi itu sendiri: file lisensi apa pun di dalamnya disertakan, dan tidak memicu pemeriksaan pihak ketiga. Atur masing-masing aturan ke `error` atau `warn`. Daftar `allow` mengecualikan jalur modul persis dari kedua aturan dan tetap menyimpan teks lisensi yang ditemukan saat build.

```yaml
licenses:
  unknown: error
  copyleft: warn
  allow: [example.org/reviewed-module]
```

## Menambahkan pustaka

Tambahkan pustaka, yaitu sebuah modul Go, dengan `nsgo add`, yakni `go get` yang menampilkan rencananya terlebih dahulu:

```sh
nsgo add github.com/acme/chart           # rilis terbarunya, kecuali go.mod sudah memilikinya
nsgo add github.com/acme/chart@v1.3.0    # versi ini, meskipun lebih rendah dari versi di go.mod
nsgo add -dry-run github.com/acme/chart  # cetak rencananya dan tidak menulis apa pun
```

```text
github.com/acme/chart
  module     github.com/acme/chart v1.3.0, published 2026-11-20
  owner      github.com/acme, the first module from it in go.mod
  next       import "github.com/acme/chart"

example.com/hello
  go.mod     + require github.com/acme/chart v1.3.0 // indirect
             + require example.com/units v1.0.0 // indirect
             ~ require github.com/nsgo-dev/nsgo v0.4.0 => v0.4.2, a release
  go.sum     + 6 lines
  tools      go tool nsgo then runs v0.4.2, and so does an installed nsgo here
Apply these changes? [y/N]
```

Untuk setiap modul, rencana menyebutkan versi dan tanggal terbitnya, serta pemiliknya: host-nya, dan pada layanan hosting kode seperti github.com juga akunnya, ditandai bila `go.mod` belum memiliki modul dari pemilik itu. Pemberitahuan penarikan (retract) dan penghentian (deprecation), versi tanpa tag rilis, `replace` atas modul tersebut, dan baris import paket akarnya menyusul bila berlaku. Lalu muncul baris-baris `go.mod` yang berubah, termasuk `go` dan `toolchain`, perintah `go tool` yang akan menjalankan versi lain, modul yang diambil dari host-nya sendiri, dan modul yang hash-nya diterima `go.sum` tanpa diperiksa oleh basis data checksum. `go get` menandai modul yang belum diimpor paket mana pun dengan `// indirect`, dan `go mod tidy` menghapus tanda itu begitu ada paket yang mengimpornya.

Di terminal, `add` bertanya sebelum menulis, dan `-y` menerapkan rencana tanpa bertanya; tanpa terminal dan tanpa `-y`, ia tidak menulis apa pun dan keluar dengan status 1. Interupsi, seperti Ctrl+C, membiarkan `go.mod` dan `go.sum` seperti semula dan keluar dengan status 130. Modul yang sudah dibutuhkan `go.mod` dibiarkan apa adanya dan versinya dicetak, kecuali sebuah versi diketikkan.

Perencanaan tidak menjalankan apa pun selain perintah go, pada salinan `go.mod` dan `go.sum`:

- dengan toolchain yang sudah dipakai modul dan `GOTOOLCHAIN=local`, sehingga tidak ada toolchain lain yang diunduh atau dijalankan;
- melalui proxy modul yang dicantumkan `GOPROXY` sebelum `direct` pertamanya, yang diubah menjadi `off`, sehingga hanya modul yang ditandai `GOPRIVATE` atau `GONOPROXY` yang diambil dari host-nya sendiri, dengan `GONOSUMDB` dan `GOAUTH` tetap apa adanya;
- dengan `GOWORK=off`, sehingga di dalam workspace ia merencanakan `go.mod` hanya untuk modul itu sendiri, sebagaimana `GOWORK=off go get` menuliskannya, dan menyebutkannya.

Dengan atau tanpa `-y`, `add` menolak:

- memindahkan kebutuhan NSGo ke pseudo-version atau pra-rilis, seperti yang dilakukan pustaka yang membutuhkan sebuah commit NSGo; `go get github.com/nsgo-dev/nsgo@<versi>` memindahkannya ke sana dengan sengaja terlebih dahulu;
- modul yang membutuhkan Go yang lebih baru daripada toolchain modul; `go get go@<versi>` menaikkan milik modul terlebih dahulu;
- menurunkan versi modul melalui kueri seperti `@latest`; mengetikkan versinya akan menurunkannya, dan rencana menyebutnya downgrade.

Ia menulis `go.mod` dan `go.sum` hanya jika keduanya tidak berubah sejak perencanaan, masing-masing melalui berkas sementara yang diganti namanya untuk menimpanya, dan mengembalikan keduanya bila terjadi galat atau interupsi. Pada modul yang memiliki direktori `vendor`, rencana menyebut `go mod vendor` sebagai langkah berikutnya.

`add` baru ada setelah NSGo 0.3.4. Di modul yang menetapkan 0.3.4 atau rilis yang lebih lama, `nsgo` yang terpasang tidak menjalankan `nsgo` milik modul untuknya, melainkan mencetak `go get github.com/nsgo-dev/nsgo@<versi>` yang membawa `add`; di sana, `go tool nsgo add` menjalankan `nsgo` yang ditetapkan, yang tidak mengenal `add`.

## Pustaka dari paket Go

Aplikasi yang mengimpor `base/terminal` juga menyertakan `libghostty-vt`: paket Windows dan Linux menaruh pustaka ini di sebelah berkas eksekusi; bundel macOS dan iOS menandatanganinya ke dalam framework masing-masing. [Menyematkan](embedding.md) menjelaskan XCFramework pendamping untuk host iOS.

Sebuah dependensi dapat mendeklarasikan pustaka bersama (*shared libraries*) desktop prakompilasi di `nsgo-natives.yaml`, dan sejak versi 2-nya juga framework untuk iOS dan tvOS. NSGo memverifikasi SHA-256-nya, menyalinnya ke dalam aplikasi sebelum penandatanganan, dan menyertakannya dalam penginstal; pembuat aplikasi tidak perlu menjalankan langkah tambahan. [Pustaka native](native-libraries.md) menjelaskan format bagi pembuat paket dan jalur pemuatannya.

## Perintah

Aplikasi dapat menyertakan program baris perintah (*command-line programs*) di sebelah berkas eksekusi desktopnya, seperti CLI yang bekerja pada data aplikasi. Cantumkan di bawah `commands`, masing-masing dibangun dari paket `main` Go:

```yaml
commands:
  - name: hello-cli             # the file's name, hello-cli.exe on Windows
    package: ./cmd/hello-cli    # a main package, built for each desktop target with CGO_ENABLED=0 and build.tags
    ldflags: [-X main.channel=beta]  # its own linker flags, after build.ldflags
windows:
  path: true                    # the installer puts the app's folder on the user's Path
```

Nama hanya terdiri atas huruf, angka, `-`, dan `_`, dan tidak boleh sama (tanpa membedakan huruf besar/kecil) dengan berkas eksekusi aplikasi, perintah Linux-nya, `Uninstall`, `AppRun`, atau perintah lain. `macos_universal` menggabungkan dua build macOS dari setiap perintah, seperti halnya aplikasi utama.

Paket ditautkan dengan `build.ldflags` dan `-ldflags`, lalu `ldflags`-nya sendiri—yang nilai `-X`-nya menang—kemudian identitas aplikasi, sehingga `core.App()` memberi perintah tersebut nama, ID, versi, dan nomor build dari `nsgo.yaml`, seperti yang diberikannya kepada aplikasi. Perintah yang tidak mengimpor `core` menerima versi melalui `-X`, seperti `-ldflags "-X main.version=1.2.0"` pada `build`, yang diterima oleh aplikasi maupun perintah-perintahnya.

| Paket | Perintah |
| --- | --- |
| macOS | Di dalam `Contents/MacOS`, ditandatangani sebelum aplikasi sebagaimana adanya, sebagai `<id>.<command>`: ad hoc, atau dengan `sign.identity`, hardened runtime, dan cap waktu (*timestamp*), sehingga aplikasi lolos [notarisasi](#notarisasi), tetapi tanpa entitlement aplikasi. Disk image tidak menaruh apa pun ke `PATH`; [Homebrew cask](#saluran) menautkan setiap perintah, dan pengguna dapat menautkan `/Applications/<name>.app/Contents/MacOS/<command>` sendiri |
| Windows | `<command>.exe` di sebelah `<name>.exe`, [ditandatangani](#menandatangani-paket-windows) sebagaimana adanya, yang dipasang oleh penginstal dan dihapus oleh uninstaller-nya; dengan `windows.path`, penginstal juga menambahkan folder tersebut ke `Path` pengguna, sebagaimana `windows.command_line` melakukannya, sehingga konsol yang dibuka setelah pemasangan menemukan perintah-perintah tersebut. Perintah yang diubah namanya atau dihapus pada pembaruan tetap berada di folder itu, seperti file yang tidak lagi dikirimkan aplikasi, karena uninstaller hanya menghapus apa yang dipasang oleh versinya |
| Linux | Di dalam `/opt/<slug>` dengan tautan `/usr/bin/<command>` yang dimiliki paket `.deb` dan Arch, sehingga menghapus paket akan menghapusnya, dan dpkg serta pacman menolak memasang di atas milik paket lain; AppImage menyimpannya di sebelah aplikasi dan tidak menaruh apa pun di `PATH` |

Penginstal, checksum-nya, dan appcast [pembaruan](#pembaruan) membawa perintah-perintah tersebut seperti halnya file aplikasi lainnya, sehingga pembaruan menggantikannya bersama aplikasi. [Paket toko aplikasi](#paket-toko-aplikasi) tidak menyertakannya, karena sandbox Mac App Store, MSIX, dan Flatpak hanya menjalankan berkas eksekusi aplikasi itu sendiri.

## Penginstal

Untuk target desktop, `build` menulis berkas yang digunakan pengguna untuk memasang aplikasi ke dalam `dist/releases`, masing-masing dengan checksum `.sha256`:

| Target | Penginstal |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.dmg`, sebuah disk image yang menampilkan aplikasi, panah, dan tautan ke Applications untuk menyeretnya ke sana, ditandatangani dengan `sign.identity` dan [dinotarisasi](#notarisasi) |
| Windows | `<name>-<version>-windows-<arch>-setup.exe`, penginstal Windows native yang memasang untuk pengguna, lagi di folder yang dipilih pengguna sebelumnya, menutup aplikasi jika sedang berjalan, dengan pintasan Start menu, pintasan desktop opsional, dan uninstaller yang hanya menghapus apa yang dipasangnya, [ditandatangani dengan `sign.certificate`](#menandatangani-paket-windows) tempat aplikasi menandatangani paket Windows; `windows.command_line` juga memasang pendamping `.com` dan menambahkan folder aplikasi ke `Path` pengguna, sehingga konsol yang dibuka setelah pemasangan menjalankan perintah aplikasi, sementara Win+R—yang juga menemukan pendamping tersebut—menjalankan aplikasi dan menutup konsol yang dibukanya; uninstaller menghapus folder dari `Path` hanya jika penginstal yang menambahkannya, tidak pernah menghapus entri yang sudah Anda miliki |
| Linux | `<name>-<version>-linux-<arch>.deb` untuk Debian dan Ubuntu, serta `<name>-<version>-linux-<arch>.pkg.tar.gz` untuk Arch Linux, yang memasang aplikasi di `/opt/<slug>` beserta perintah, tautan ke [perintah-perintahnya](#perintah), entri desktop, dan ikon (paket Arch menyertakan `.MTREE` untuk `pacman -Qkk`), serta `<name>-<version>-linux-<arch>.AppImage`, yang berjalan di distribusi mana pun tanpa perlu instalasi |

Setup dan uninstaller Windows dibangun hanya dengan Go, di sistem apa pun, dan menggunakan dialog Windows native. Keduanya berbicara dalam bahasa-bahasa katalog aplikasi yang kosakatanya mereka miliki, termasuk Bahasa Mandarin Sederhana dan Tradisional, dengan memilih bahasa UI sistem; `/LANG=<LCID>` memilih satu bahasa untuk satu kali jalan, misalnya `/LANG=2052` untuk Bahasa Mandarin Sederhana. `/S` memasang atau mencopot pemasangan secara senyap, `/D=<folder>` di posisi terakhir memilih folder, termasuk yang berisi spasi, dan `/RELAUNCH` membuka aplikasi setelah pembaruan. Tanpa `/D`, peningkatan versi memasang di folder instalasi yang digantikannya. `-installers=false` atau `build.installers: false` hanya membangun paketnya saja.

Penginstal menerima gambar dan ikon milik aplikasi sendiri; jika tidak ada, masing-masing memakai ikon aplikasi atau gambar bawaan:

```yaml
macos:
  dmg_background: assets/DMGBackground.png  # the disk image's window, which takes its size in points
  dmg_icon: assets/DMGIcon.png              # the disk image's icon: an .icns or a square PNG
windows:
  installer_icon: assets/Setup.png          # the setup program's icon: an .ico or a square PNG
  uninstaller_icon: assets/Uninstall.png    # the uninstaller's
```

PNG latar belakang yang memiliki pasangan berukuran dua kali lipat, seperti `DMGBackground@2x.png`, tetap tajam pada layar Retina. Ikon aplikasi berada seperempat dari kiri, Applications seperempat dari kanan, masing-masing dengan namanya tepat di tengah secara vertikal, tempat gambar bawaan menggambar panahnya; sisakan ruang untuk keduanya pada latar belakang Anda sendiri. Paket Linux tidak memiliki gambar tersendiri; paket-paket itu memasang ikon aplikasi.

Paket Linux juga memasang metadata AppStream aplikasi, `/usr/share/metainfo/<id>.metainfo.xml`, yang dipakai pusat perangkat lunak untuk mendaftarkannya, ditulis dari `linux.summary`, `linux.license` jika diatur, dan nama-nama aplikasi, dengan `Utility` sebagai kategorinya jika `linux.categories` tidak menyebutkan apa pun; AppImage menyimpannya di `usr/share` milik AppDir-nya, di sebelah tautan ke entri desktopnya di `usr/share/applications`, yang disebut oleh metadata tersebut. File yang dibaca sistem dari `/usr/share`, seperti metadata yang lebih lengkap, entri desktop tambahan, pelengkapan shell (*shell completions*), atau halaman manual, diletakkan di direktori yang disebutkan `linux.share`, relatif terhadap `nsgo.yaml`: paket `.deb` dan Arch memasang pohon direktori itu di bawah `/usr/share`, dan AppImage di bawah `usr/share` milik AppDir-nya. `metainfo/<id>.metainfo.xml`-nya menggantikan yang ditulis NSGo, dan file di jalur yang ditulis sendiri oleh paket—seperti entri desktop atau ikon aplikasi—menggagalkan build. File di `resources/`, apa pun namanya, tetap berada di sebelah berkas eksekusi di `/opt/<slug>`, dan Flatpak pada build toko aplikasi hanya menggunakan metadata milik NSGo saja.

AppImage memerlukan FUSE, yang dimiliki sebagian besar distribusi, dan mengambil GTK 4 dari sistem, sebagaimana paket lainnya; runtime-nya diunduh satu kali ke dalam cache pengguna.

AppImage menyematkan pelokasi `gh-releases-zsync` menggunakan `release.github` atau remote GitHub `origin`, yang memilih rilis stabil terbaru dan arsitektur aplikasi ini. Unggah setiap `.AppImage` bersama file `.AppImage.zsync` yang dihasilkan. Untuk host lain, atur `linux.appimage_update` ke `zsync|https://example.com/latest.AppImage.zsync`, yaitu URL stabil yang disajikan dengan dukungan HTTP range; pengaturan ini juga menerima `gh-releases-zsync|owner|repo|latest-pre|name-*-linux-x64.AppImage.zsync` untuk saluran prarilis. String kosong eksplisit menonaktifkan penemuan tersemat; tanpa repositori GitHub atau pelokasi yang dikonfigurasi, nilainya juga kosong. File `.zsync` tetap dihasilkan secara terpisah dari appcast yang ditandatangani.

## Paket toko aplikasi

`-channel store`, atau `updates.channel: store`, membangun target desktop dan Android untuk toko aplikasi masing-masing—yang akan menanganai pembaruannya—alih-alih membuat penginstal dan skrip instalasi:

| Target | Paket toko aplikasi |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.pkg` untuk Mac App Store: aplikasi di dalam App Sandbox, ditandatangani dengan identitas profil provisi dan dipaketkan dengan identitas penginstal tim Anda |
| Windows | `<name>-<version>-windows-<arch>.msix` untuk Microsoft Store, yang menandatanganinya saat publikasi |
| Linux | `<name>-<version>-linux-<arch>.flatpak` yang dapat dipasang, ditambah `.tar.gz` berisi AppStream dan `flatpak/<id>.yml` untuk Flathub atau `flatpak-builder` di atas runtime GNOME |
| Android | `<name>-<version>-android-<arch>.aab`, sebuah App Bundle untuk Google Play yang ditandatangani dengan `sign.keystore`, yaitu kunci unggah |

MSIX membaca nama tampilan setiap bahasa dari `appName` di katalog, dan mundur ke bahasa sumber jika tidak ada. Logo Start, bilah tugas, dan Store-nya mencakup varian 100%, 125%, 150%, 200%, dan 400%. Alat ini menulis indeks sumber dayanya di macOS; Windows SDK tidak diperlukan.

`-target macos` memilih kedua arsitektur macOS pada saluran `store` dan menulis satu `<name>-<version>-macos-universal.pkg`, sebagaimana `-target macos_universal`. Pilih `-target android_arm64,android_x64` untuk satu `<name>-<version>-android-universal.aab` yang berisi `arm64-v8a` dan `x86_64`. Potongan `x86_64` memerlukan Android NDK.

```yaml
macos:
  category: public.app-category.productivity  # LSApplicationCategoryType, which the Mac App Store requires
  # entitlements: { com.apple.security.files.downloads.read-write: true }  # beyond the network, the files the user picks and what apple.info explains
windows:
  store:                               # as Partner Center's Product identity lists them
    identity: 12345Example.Hello
    publisher: CN=00000000-0000-0000-0000-000000000000
    publisher_name: Example
  # device_capabilities: [microphone, webcam]  # the devices the app uses, which Windows asks the user for
linux:
  summary: Says hello                  # one line for software centers
  license: MIT                         # an SPDX expression; LicenseRef-proprietary by default
  # flatpak_runtime: "51"              # GNOME's runtime version
android:
  target_sdk: 36                       # the targetSdkVersion Google Play requires
sign:
  macos_profile: assets/MacAppStore.provisionprofile  # the Mac App Store's profile
  # installer: "3rd Party Mac Developer Installer: Example (ABCDE12345)"  # by default the keychain's of the profile's team
```

Runtime menargetkan API 36 ketika `android.target_sdk` tidak diatur. Build Android App Bundle gagal di bawah API 36, yang diwajibkan oleh [Google Play](https://support.google.com/googleplay/android-developer/answer/11926878) untuk aplikasi seluler baru dan pembaruan mulai 31 Agustus 2026. Build APK mandiri memberi peringatan lalu tetap berjalan.

Atur `sign.macos_profile` untuk menandatangani paket Mac untuk pengunggahan. Tanpa widget, paket yang tidak ditandatangani tetap dapat berjalan di dalam sandbox, sehingga Anda dapat menguji akses ke kontainer aplikasi, jaringan, dan file yang dipilih pengguna. Build `store` dengan widget memerlukan baik `sign.macos_profile` untuk `<id>` maupun `sign.widgets_profile` macOS untuk `<id>.widgets`, dari tim yang sama dan memberikan `apple.app_group` (bawaan `group.<id>`). Kedua bundel menggunakan kontainer bersama tersebut; tidak ada pengecualian direktori home yang ditambahkan. `-installers=false` mempertahankan sandbox dan tanda tangan profil sambil melewatkan `.pkg` dan identitas penginstalnya. Unggah `.msix` di Partner Center dan `.aab` di Play Console. Kirimkan `flatpak/<id>.yml` ke Flathub setelah rilis memuat arsip yang disebutkannya. `flatpak-builder` juga dapat membangunnya secara lokal. Pasang bundel dengan `flatpak install --user ./<name>-<version>-linux-<arch>.flatpak`; runtime GNOME yang sesuai harus terpasang atau tersedia dari remote yang terkonfigurasi. Build menulis bundel dalam Go tanpa toolchain Linux. Paket toko aplikasi menggunakan ikon transparan jika artwork tidak ada dan kategori `Utility` jika `linux.categories` tidak diatur. Untuk pemasangan di luar Store, atur [`sign.certificate`](#menandatangani-paket-windows) untuk menandatangani MSIX; subjek sertifikatnya harus cocok dengan `windows.store.publisher`, dan mesin penerima harus memercayai sertifikat tersebut. Tanpa pengaturan penandatanganan, Store akan menandatangani paket yang diunggah.

Di macOS dan iOS, efek tepi tampilan gulir (`Scroll.EdgeEffects`), keburaman latar (*backdrop blurs*) macOS tanpa `IOS.BackdropMaterial`, dan filter warna latar (sendiri, atau di atas keburaman tanpa `IOS.BackdropMaterial`) digambar melalui `CABackdropLayer` dan `CAFilter` privat milik Core Animation—seperti halnya material dan efek tepi milik AppKit dan UIKit sendiri—karena tidak ada API publik yang memburamkan berdasarkan radius, memvariasikan keburaman di seluruh tampilan, atau menerapkan matriks warna latar. Peninjauan App Store dapat menolak aplikasi yang memakai API privat: NSGo hanya memanggilnya untuk tampilan gulir dengan bilah tepi dan filter latar tersebut, tetapi setiap build macOS dan iOS memuat namanya.

Setiap bundel Apple yang ditulis `build`—aplikasi, ekstensi widget, App Clip, aplikasi Apple Watch, dan framework `-dynamic` `-embed`—membawa manifes privasi, `PrivacyInfo.xcprivacy`, yang mendeklarasikan API dengan alasan wajib (*required-reason APIs*) yang digunakan NSGo dan runtime Go: user defaults (`CA92.1`), cap waktu file (`C617.1`), dan waktu boot sistem (`35F9.1`). `apple.privacy` menambahkan milik aplikasi sendiri: `accessed_apis`, alasan setiap kategori, `tracking`, `tracking_domains`, dan `collected_data`, yaitu entri `NSPrivacyCollectedDataTypes` sesuai penamaan Apple; `PrivacyInfo.xcprivacy` di dalam `resources/` akan digabungkan dengan entri tersebut alih-alih diganti.

`Info.plist` pada setiap bundel iOS, tvOS, dan watchOS yang ditulis `build` untuk perangkat—milik aplikasi, ekstensi widget, dan App Clip—membawa kunci SDK dan Xcode `DT` yang diperiksa App Store Connect, dibaca dari Xcode yang membangunnya, serta `UIRequiredDeviceCapabilities: [arm64]`; bundel simulator tidak menyertakannya, dan kunci yang diatur `apple.info` tetap mempertahankan nilainya. `apple.multiple_scenes: true` mengatur `UIApplicationSupportsMultipleScenes` dalam manifes scene aplikasi iOS, yang secara bawaan bernilai false: di iPad, pengguna kemudian dapat membuka lebih dari satu jendela aplikasi, dan masing-masing menampilkan ulang tampilan utama aplikasi, dengan statusnya sendiri serta sinyal dan penyimpanan bersama aplikasi, hingga pengguna menutupnya.

### Publikasikan ke Google Play

Aktifkan Google Play Developer API dan beri akun layanan (*service account*) izin untuk merilis aplikasi ke jalur (*track*) yang dipilih. Tetapkan `NSGO_PLAY_SERVICE_ACCOUNT` ke kunci JSON akun tersebut atau ke jalur file kuncinya. [Penyiapan Google Play API](https://developers.google.com/android-publisher/getting_started)

```sh
go tool nsgo build -target android -channel store
go tool nsgo publish -target android -track internal
```

Perintah ini menemukan App Bundle untuk versi proyek di `build.output/releases`. Berikan jalur `.aab` setelah flag untuk memilih file ketika ada beberapa build. Gunakan `internal`, `production`, atau ID jalur dari pengujian tertutup. Perintah ini membaca ID paket dan kode versi dari bundel, mensyaratkan kode yang lebih tinggi daripada semua rilis di jalur tersebut, lalu mengunggah bundel dan mengesahkan (*commit*) rilisnya.

## Daftar toko aplikasi

Metadata toko aplikasi (judul, subjudul, deskripsi, kata kunci, catatan rilis, URL, tangkapan layar, dan pratinjau) berada di repositori sebagai YAML dan Markdown teks biasa di bawah `store/` (atau `store.dir` di `nsgo.yaml`):

```text
store/
  listing.yaml              # version: 1, default_locale, app_store and google_play settings
  en-US/
    listing.yaml            # version: 1, title, subtitle, short_description, keywords, URLs, media
    description.md          # full description (optional file in place of YAML description)
    release_notes.md        # what's new (optional file in place of YAML release_notes)
    promotional_text.md     # App Store promotional text (optional file)
    screenshots/
  zh-Hans/
    listing.yaml
    description.md
    release_notes.md
    screenshots/
```

Kedua file `listing.yaml` mewajibkan `version: 1` dan menolak kunci yang tidak dikenal. Direktori lokal kanonik seperti `zh-Hans` dipetakan secara otomatis ke kode masing-masing toko (`zh-Hans` di App Store Connect, `zh-CN` di Google Play) kecuali ditimpa per toko dengan `app_store.locale` atau `google_play.locale`.

```sh
go tool nsgo publish -store all -dry-run                  # validate lengths, locales and media without uploading
go tool nsgo publish -store appstore,play                 # validate and upload to App Store Connect and Google Play
go tool nsgo publish -store play -track production        # also update release notes on the named Play track
```

Validasi berjalan sebelum membaca kredensial atau membuat permintaan HTTP apa pun, memeriksa field wajib, kode lokal, skema URL, file dan jumlah media (hingga 10 tangkapan layar dan 3 pratinjau per jenis tampilan App Store; hingga 8 tangkapan layar per jenis gambar Google Play), serta batas karakter (App Store: judul 30, subjudul 30, kata kunci 100, teks promosi 170, deskripsi 4000, catatan rilis 4000; Google Play: judul 30, deskripsi singkat 80, deskripsi 4000, catatan rilis 500).

Kredensial unggah hanya berasal dari variabel lingkungan: `NSGO_APP_STORE_KEY`, `NSGO_APP_STORE_KEY_ID`, dan `NSGO_APP_STORE_ISSUER` (mundur ke kunci notarisasi `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID`, dan `NSGO_NOTARY_ISSUER`) untuk App Store Connect, serta `NSGO_PLAY_SERVICE_ACCOUNT` (jalur file JSON akun layanan atau teks JSON) untuk Google Play.

### Kepatuhan toko aplikasi

Gunakan `nsgo.yaml` untuk melacak status setiap kuesioner rilis.

```yaml
compliance:
  app_store: { age_rating: false }
  google_play: { data_safety: false }
  steam: { content_survey: false }
```

Ubah entri menjadi `true` setelah menyelesaikan kuesionernya. Build `-channel store` atau `-channel steam` yang cocok akan mengingatkan Anda tentang entri yang bernilai `false`. Hilangkan entri apa pun yang tidak Anda lacak di YAML.

- Lengkapi [kuesioner peringkat usia](https://developer.apple.com/help/app-store-connect/manage-app-information/set-an-app-age-rating) dan [detail App Privacy](https://developer.apple.com/help/app-store-connect/manage-app-information/manage-app-privacy) di App Store Connect.
- Untuk [kuesioner enkripsi](https://developer.apple.com/help/app-store-connect/manage-app-information/determine-and-upload-app-encryption-documentation) milik Apple, tetapkan `apple.info.ITSAppUsesNonExemptEncryption` ke jawaban aplikasi Anda sebagai boolean YAML. Jika Apple mengirim kode persetujuan, masukkan ke `apple.info.ITSEncryptionExportComplianceCode` sebagai string. Anda dapat mengabaikan kunci-kunci tersebut dan menjawab saat mengunggah. Build toko aplikasi Apple memeriksa tipe kunci-kunci ini dan struktur manifes privasi bundel.
- Lengkapi [Keamanan data](https://support.google.com/googleplay/android-developer/answer/10787469) di Play Console. Aplikasi yang didistribusikan secara eksklusif di jalur pengujian internal dikecualikan.
- Lengkapi [Survei Konten](https://partner.steamgames.com/doc/gettingstarted/contentsurvey) di Steamworks sebelum mengirimkan gim untuk ditinjau.

## App Store Connect

Buat aplikasi di App Store Connect dan tandatangani IPA perangkat iOS dengan profil distribusi App Store atau PKG toko macOS dengan identitas penginstal tim Anda. Di Mac dengan Xcode, tetapkan `NSGO_APP_STORE_KEY` ke file `.p8` kunci tim App Store Connect Anda atau teksnya, `NSGO_APP_STORE_KEY_ID` ke ID kuncinya, dan `NSGO_APP_STORE_ISSUER` ke ID penerbitnya; tanpanya, kunci notarisasi (`NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID`, `NSGO_NOTARY_ISSUER`) akan digunakan, seperti untuk daftar toko aplikasi. Beri kunci tersebut akses ke aplikasi dan izin untuk mengunggah build.

```sh
go tool nsgo publish ios path/to/Hello.ipa
go tool nsgo publish macos path/to/Hello.pkg
```

Alat ini memeriksa nomor build bundel sebelum mengunggah, lalu melaporkan hasil pemrosesan Apple untuk TestFlight. Naikkan `build_number` untuk pengunggahan berikutnya. Nomor build macOS terus meningkat di semua versi aplikasi, sedangkan iOS dapat memulai ulang urutannya untuk versi baru. Kirimkan build yang telah diproses untuk App Review secara terpisah di App Store Connect. [Panduan distribusi Apple](https://developer.apple.com/documentation/xcode/preparing-your-app-for-distribution)

## Paket aset toko aplikasi

Deklarasikan direktori aset berukuran besar di `mobile.asset_packs`. File di dalam direktori tersebut dikirimkan secara terpisah dari Android App Bundle dan aplikasi iOS dasar. Nama paket diawali huruf kecil dan dapat berisi huruf kecil, angka, serta garis bawah.

```yaml
mobile:
  asset_packs:
    - {name: levels, dir: resources/levels, delivery: install-time}
    - {name: voices, dir: large-assets/voices, delivery: fast-follow}
```

Bangun paket Android dengan `go tool nsgo build -channel store -target android_arm64`. Gunakan `bundletool build-apks --local-testing` dan `bundletool install-apks` untuk menguji bundel di perangkat. NSGo membuka paket `install-time` yang terpasang. Modul `fast-follow` disertakan untuk pengiriman Play, dan pembacaan paket tersebut dari Play Asset Delivery SDK masih menunggu dukungan.

Di iOS, `go tool nsgo build -target ios` memetakan `install-time` ke kebijakan esensial Apple dan `fast-follow` ke prefetch, baik saat pemasangan pertama maupun pembaruan. Xcode 26 atau lebih baru membangun pengunduh Background Assets. Gunakan `apple.app_group` bersama atau bawaan `group.<id>`. `sign.profile` milik aplikasi dan `sign.asset_packs_profile` milik ekstensi `<id>.assetdownloader` sama-sama memerlukan grup tersebut. Unggah file `.aar` dari `dist/releases` bersama IPA menggunakan alur unggah paket aset App Store Connect. [Membuat paket aset terkelola](https://developer.apple.com/documentation/backgroundassets/creating-managed-asset-packs)

Dari sebuah goroutine, panggil `mobile.AssetPackReady(ctx, "levels")` untuk memeriksa kesiapan dan `mobile.OpenAssetPackFile("levels", "intro.map")` untuk membuka file secara relatif terhadap direktori yang dideklarasikan. Tutup reader yang dikembalikan setelah digunakan. [Menguji paket aset Apple secara lokal](https://developer.apple.com/documentation/backgroundassets/testing-asset-packs-locally)

## Depot Steam

```sh
go tool nsgo build -channel steam -target macos_universal,windows_x64,linux_x64
```

Tetapkan `updates.channel: steam` untuk menggunakan saluran tersebut secara bawaan. Pilih target macOS, Windows, dan Linux. Unggah setiap direktori `dist/<name>-<version>-<target>` ke depot Steam masing-masing, dengan menggunakan bundel aplikasi di macOS, berkas eksekusi di Windows dan Linux arm64, serta `steam-launch.sh` di Linux x64 sebagai opsi peluncuran. Steam menangani pembaruannya. Direktori tersebut tidak berisi penginstal, pembaru mandiri (*self-updater*), maupun `steam_appid.txt`.

Depot Linux x64 menyertakan GTK 4 beserta dependensinya untuk Steam Linux Runtime 3.0 dan 4.0. Peluncur hanya menggunakannya ketika sistem tidak memiliki GTK 4.14 atau lebih baru. Simpan `nsgo-gtk/` di sebelah peluncur: `licenses/` di dalamnya menyimpan lisensi pustaka, kode sumber tempatnya dibangun, dan skrip build, yang diwajibkan oleh LGPL untuk GLib, Pango, dan GTK. Linux arm64 memerlukan GTK 4 yang tersedia di runtime-nya.

Untuk pengujian lokal di luar klien Steam, tetapkan `steam: {app_id: 480}` di `nsgo.yaml`, dengan mengganti 480 menjadi App ID Steamworks Anda. `go tool nsgo dev` menulis ID tersebut ke `steam_appid.txt` di direktori build native-nya dan meluncurkan aplikasi dengan direktori tersebut sebagai direktori kerjanya. [Steamworks API](https://partner.steamgames.com/doc/api/steam_api), [SteamPipe](https://partner.steamgames.com/doc/sdk/uploading).

## Publikasikan di GitHub

Ketika build memiliki target macOS, Windows, atau Linux, `dist/releases` juga menyimpan `install.sh` dan `install.ps1`, yang memasang aplikasi dari rilis repositori GitHub yang disebutkan `release.github`, atau repositori tempat remote Git `origin` aplikasi berada; tanpa keduanya, `build` tidak menulis skrip apa pun dan memberi tahu hal itu. Unggah file-filenya—yang berada di tingkat teratas tanpa `miniprogram/`—ke rilis bertag `v<version>` (`release.tag_prefix` menggantikan `v`; skrip menyandikan karakter URL tercadang di dalam awalan tersebut):

```sh
gh release create v1.2.0 dist/releases/*.*
```

Pengguna kemudian memasang atau memperbarui aplikasi dengan satu baris perintah, yang dicetak oleh build:

```sh
curl -fsSL https://github.com/me/hello/releases/latest/download/install.sh | sh    # macOS and Linux
```

```powershell
irm https://github.com/me/hello/releases/latest/download/install.ps1 | iex         # Windows
```

Di macOS, skrip menyalin aplikasi dari disk image ke `/Applications`, atau `~/Applications` jika tidak dapat menulis di sana; di Linux, skrip memasang paket Arch dengan `pacman`, atau paket Debian dengan `apt-get` atau `dpkg`, melalui `sudo`; di Windows, skrip menjalankan penginstal secara senyap (*silent*), yang memasang di lokasi tempat aplikasi sudah berada. Masing-masing memeriksa file terhadap checksum-nya terlebih dahulu. `<NAME>_VERSION`, nama aplikasi dalam huruf kapital seperti `HELLO_VERSION=1.1.0`, memasang rilis lain, dan `<NAME>_DOWNLOAD_URL` mengunduh dari mirror.

## Publikasikan di itch.io

Tetapkan `itch.project` ke slug `owner/game` gim itch.io Anda di `nsgo.yaml`. Untuk menimpa nama saluran, tetapkan `itch.channels` berdasarkan target build. Nilai bawaannya adalah `html5` untuk web, `osx-<arch>` untuk macOS, `windows-<arch>` untuk Windows, dan `linux-<arch>` untuk Linux.

```yaml
itch:
  project: example/my-game
  channels:
    macos_arm64: osx-arm64-beta
```

Dengan `BUTLER_API_KEY` yang diatur sebagai secret di CI, bangun dan unggah target-targetnya.

```sh
go tool nsgo build -channel none -installers=false -target web,macos_arm64
go tool nsgo publish itch -target web,macos_arm64
```

Gunakan `-output` dan `-version` yang sama pada kedua perintah jika Anda menimpanya. Pengunggahan mengambil direktori paket masing-masing target dan mengatur versi itch.io-nya ke versi build. NSGo menggunakan `butler` di `PATH` atau mengunduh salinan yang dipatok dari itch.io. Setelah unggahan web pertama, atur jenis gim itch.io ke HTML dan tandai unggahan tersebut agar dapat dimainkan di peramban. [Dokumentasi Butler](https://itch.io/docs/butler/pushing.html)

## Pembaruan

`updates` membuat aplikasi memperbarui dirinya sendiri sesuai cara platformnya dan saluran tempat pengguna mendapatkannya:

```yaml
updates:
  key: zE0hx2FZwwHy6X7LbEP3GjTTE3KWfCAp5gsaBysC+vg=  # the public key nsgo keygen prints
  # channel: store                    # direct, store, steam or none; direct by default with a key, none without
  # feed: https://example.com/hello/  # where the appcasts and installers are; by default the latest GitHub release
  # automatic: false                  # check only when asked; true by default
  # automatic_downloads: true          # Sparkle downloads automatically; false by default, macOS only
```

`nsgo keygen` mencetak pasangan kunci tanpa menulis file apa pun: masukkan kunci publik ke `updates.key` dan komit, lalu simpan kunci rahasianya di luar repositori, di pengelola kata sandi dan di `NSGO_UPDATES_KEY` tempat rilis dibangun, seperti GitHub Actions secret. Setiap build langsung (*direct*) menandatangani penginstal dengan kunci rahasia dan menulis `appcast-<target>.xml` di sebelahnya, berupa umpan berformat [Sparkle](https://sparkle-project.org) yang dibaca aplikasi di bawah `feed`: unggah appcast bersama penginstal. Tanpa kunci rahasia, `build` tidak menulis appcast macOS maupun Windows, karena keduanya hanya memasang penginstal bertanda tangan, dan aplikasi Linux serta Android yang membaca appcast tanpa tanda tangan akan membuka halaman rilis alih-alih memasang paketnya.

`-release-notes notes.md` memasukkan file Markdown catatan rilis, seperti bagian changelog-nya, ke setiap appcast, dirender sebagai HTML tanpa HTML mentah milik file itu sendiri. Sparkle dan WinSparkle menampilkannya di dialog pembaruan mereka, dan perintah konfirmasi di Linux serta Android menampilkannya sebagai teks di bawah versi, hingga 1.500 karakter; aplikasi membacanya sebagai `core.UpdateCheck.Notes`.

| Target | Cara build langsung melakukan pembaruan |
| --- | --- |
| macOS | Sparkle, yang dimasukkan `build` ke dalam aplikasi, memeriksa setiap hari, menawarkan versi baru, mengunduh disk image, memeriksa tanda tangannya, memasangnya, dan memulai ulang aplikasi; menu aplikasi bawaan memiliki Check for Updates…, yang ditambahkan aplikasi dengan menu sendiri melalui `updater.Check` |
| Windows | WinSparkle, di sebelah berkas eksekusi, melakukan hal yang sama dalam bahasa yang ditampilkan aplikasi saat dimulai apabila WinSparkle memilikinya, dan menjalankan penginstal secara senyap, yang lalu memulai versi baru |
| Linux, Android | Aplikasi membaca appcast-nya, dan setelah pengguna memilih Perbarui, aplikasi mengalirkan (*stream*) dan memverifikasi paket bertanda tangan langsung ke disk dengan progres yang dilaporkan (`updater.Status` dan indikator progres tugas platform), lalu memasangnya melalui `pkexec dpkg`, `pkexec pacman` (`.pkg.tar.gz`/`.pkg.tar.zst`), penggantian AppImage atomik (memulai ulang di Linux), atau penginstal paket Android; pemasangan di luar format tersebut atau appcast tanpa tanda tangan menawarkan halaman rilis sebagai gantinya |
| iOS | Aplikasi menanyakan App Store yang akan memperbaruinya, dan menawarkan halamannya di sana |

Untuk aplikasi web mandiri yang dipublikasikan, `updater.Check(cx)` membandingkan rilis yang sedang berjalan dengan `nsgo.json` baru dan menawarkan untuk memuat ulang setelah konfirmasi. Fungsi ini juga mendeteksi file build yang berubah di bawah label versi yang sama. Tidak diperlukan kunci appcast; saluran `none`/`steam` eksplisit tetap menonaktifkan pemeriksaan, dan host tersemat mengelola muat ulangnya sendiri. Program mini menggunakan manajer pembaruan platformnya dan memulai ulang hanya setelah pembaruan yang diunduh siap. Target-target ini memeriksa saat aplikasi memanggil pembaru; waktu unduh/periksa program mini dikendalikan oleh host. Target-target ini tidak mengekspos penyetel preferensi pemeriksaan otomatis. Penangan `cx.CheckForUpdates` kustom menggunakan `UpdateCheck.Restart` untuk pembaruan yang sudah siap yang label versinya mungkin tidak ada.

### Saluran

Saluran (*channel*) menentukan siapa yang memperbarui aplikasi. `-channel` menggantikan `updates.channel` untuk satu kali build, sehingga satu proyek dapat membangun untuk setiap saluran:

| Saluran | Untuk | Paket |
| --- | --- | --- |
| `direct` | GitHub, situs web, Homebrew cask, dan winget, yang memasang file rilis itu sendiri | Memperbarui dirinya sendiri seperti di atas |
| `store` | App Store, Mac App Store, Microsoft Store, Google Play, Flathub, dan toko lainnya | Merupakan [paket toko aplikasi](#paket-toko-aplikasi) dan tidak membawa pembaru mandiri; iOS dan macOS menanyakan App Store masing-masing tanpa kunci, dan instalasi Google Play dapat menggunakan alur pembaruan native milik Play |
| `steam` | Steam | Menghasilkan [direktori depot](#depot-steam) yang diperbarui oleh Steam |
| `none` | Distribusi Linux dan siapa pun yang memaketkan ulang aplikasi | Tidak memeriksa apa pun |

Toko aplikasi menolak aplikasi yang memperbarui dirinya sendiri, jadi unggah build `store`, jangan pernah mengunggah build `direct`. Setiap rilis langsung dengan penginstal macOS atau Windows juga menulis Homebrew cask (`dist/releases/homebrew/<name>.rb`, yang mendeklarasikan `auto_updates true`, checksum, dan URL unduhan untuk disk image yang dibangun, serta tautan `binary` ke setiap [perintah](#perintah) aplikasi) dan manifes multi-file winget (`dist/releases/winget/<id>.yaml`, `<id>.installer.yaml`, dan `<id>.locale.en-US.yaml` untuk penginstal NSIS yang dibangun). Di mana pun sistem memberi tahu siapa yang memasang aplikasi, aplikasi juga memeriksanya: aplikasi Android dari toko lain, Flatpak atau Snap, serta build TestFlight menyerahkan pembaruan ke toko tersebut. `updater.Check` manual dalam instalasi Google Play meminta Play In-App Updates menawarkan alur langsungnya (*immediate flow*) jika diizinkan; Play menangani persetujuan, pengunduhan, pemasangan, dan mulai ulang. Build pengembangan tidak pernah memasuki alur tersebut. Paket Android mandiri membundel SDK-nya; [host penyematan](embedding.md#android) menyediakannya melalui Gradle.

### Memeriksa dari dalam aplikasi

`updater.Check(cx)` memeriksa sebagaimana item menu *Check for Updates…* dan menampilkan apa yang ditemukannya; `updater.CheckInBackground(cx)` hanya menampilkan jika ada versi yang lebih baru, dan `updater.CheckDaily(cx)` melakukannya maksimal sekali sehari. `updater.Status(cx)` membaca progres pengunduhan dan pemasangan langsung di Linux dan Android. Selama `automatic` aktif, aplikasi memeriksa dengan cara itu di Linux, build Android langsung, dan build Apple App Store saat dimulai serta setiap kali pengguna kembali ke aplikasi; pemeriksaan yang gagal, seperti saat luring, dicoba lagi satu jam kemudian. Build pengembangan tidak pernah memperbarui. Pada build Steam, ketiga fungsi ini tidak melakukan apa pun.

`updater.CapabilityOf(cx)` melaporkan cara salinan ini diperbarui tanpa permintaan jaringan: `ManagedSelf`, `ManagedPage`, `ManagedDevelopment`, `ManagedChannel`, `ManagedStore` (dengan nama `Store`-nya), atau `ManagedUnavailable`. Build pengembangan, saluran yang dinonaktifkan, atau instalasi toko menghasilkan penjelasan khusus alasan saat diperiksa; `core.UpdateCheck.Err` dicadangkan untuk kegagalan seperti umpan yang rusak atau pembaru terbundel yang tidak dapat dimuat.

Baca `updater.Status(cx)` di dalam ikatan untuk mengikuti `Capability`, `AutomaticChecks`, `AutomaticDownloads`, `Available`, `Notes`, dan `LastCheck`, serta di Linux dan Android untuk `Phase`, `Loaded`, dan `Total` milik pembaruan (`Fraction()` untuk bilah progres). `updater.SetAutomaticChecks(cx, on)` menyimpan pilihan pengguna dan mengubah penjadwal harian saat runtime. Sparkle dan WinSparkle memiliki preferensinya masing-masing; Linux, Android, dan iOS menyimpan pilihan tersebut di partisi penyimpanan privat. `updater.SetAutomaticDownloads(cx, on)` hanya didukung di macOS, dengan bawaan `updates.automatic_downloads`; platform lain melaporkan false dan mengabaikan penyetelnya. Build yang tidak didukung dan yang dikelola secara eksternal melaporkan kedua pengaturan sebagai false. WinSparkle melaporkan metadata pembaruan yang ditemukan dengan membaca appcast setelah callback temuannya; WinSparkle tetap mengendalikan pemasangannya. Linux menyelesaikan kepemilikan paket Debian atau Arch serta identitas AppImage saat peluncuran; salinan tersebut melaporkan `ManagedSelf`, sementara salinan tak terpaket lainnya melaporkan `ManagedPage` sebelum pemeriksaan apa pun.

## Jalankan saat masuk

`desktop.SetLaunchAtLogin(cx, on)` mendaftarkan aplikasi ke mekanisme login platform, dan `desktop.LaunchAtLogin(cx, done)` membacanya. MSIX yang dibangun oleh alat saat ini menyertakan tugas startup yang dinonaktifkan secara otomatis; mengaktifkannya menghormati sakelar Startup apps pengguna dan kebijakan administrator. Instalasi Windows dan Linux biasa menyimpan nilai Run dan entri autostart XDG masing-masing. macOS menggunakan layanan login native aplikasi yang dibundel.

Flatpak meminta portal Background, yang dapat meminta persetujuan, tanpa memberikan akses ke file konfigurasi host. Pembacaan berikutnya menunggu balasan native. Portal tidak memiliki kueri startup hanya-baca: pembacaan Flatpak hanya mengembalikan balasan terkonfirmasi terakhir dalam sesi ini, bernilai false sebelum ada balasan, dan tidak dapat mendeteksi perubahan di luar aplikasi atau memulihkan pengaturan pada sesi berikutnya.

Snap yang dipaketkan secara eksternal mendeklarasikan nama file autostart pada aplikasinya:

```yaml
apps:
  app:
    command: bin/app
    autostart: app.desktop
```

NSGo menulis nama file tersebut di bawah `$SNAP_USER_DATA/.config/autostart`, yang diluncurkan snapd melalui pembungkus (*wrapper*) aplikasi. `desktop.LaunchedAtLogin(cx)` membedakan peluncuran saat masuk sebelum aplikasi dibangun, termasuk aktivasi StartupTask MSIX.

## Jenis file

`file_types` mendaftarkan dokumen yang dapat dibuka aplikasi: dari Finder, Dock, dan Open With di macOS; dari Open With Explorer melalui penginstal Windows atau MSIX di Windows; melalui entri desktop paket Debian, Arch, AppImage, dan Flatpak di Linux; serta melalui Open In di iOS:

```yaml
file_types:
  - name: Flow File
    extensions: [flow]
    # id: dev.example.hello.flow   # <id>.<first extension> by default; another's ID imports its type
    # icon: assets/Flow.icns       # macOS document icon; or a square PNG
    # role: viewer                 # editor by default
    # conforms: [public.json]      # public.data by default
    # mime: [application/json]     # existing MIME types; otherwise inferred on Linux
```

Penginstal Windows menambahkan aplikasi ke Open With tanpa menggantikan bawaan pengguna, dan menghapus pendaftarannya saat pemutakhiran dan pencopotan. Linux menamai tipe MIME untuk ekstensi umum dari tabel tetap, dan memberi ekstensi yang tidak dikenal milik aplikasi sendiri tipe `application/x-<type id>` dengan definisi shared-MIME-info. `mime` menyebutkan tipe yang sudah ada sebagai gantinya; tipe yang diimpor yang ekstensinya tidak ada di tabel memerlukannya. Menjalankan AppImage tidak mendaftarkan apa pun: alat integrasi desktop memasang entri dan definisinya.

Aplikasi mendengar file-file tersebut dengan `desktop.OnOpenFiles`, termasuk peluncuran Windows atau Linux berikutnya saat aplikasi sedang berjalan serta pembukaan dokumen scene iOS. File yang meluncurkan aplikasi menunggu penangan pertama:

```go
desktop.OnOpenFiles(cx, func(paths []string) { open(paths[0]) })
```

Di iOS, dokumen reguler diimpor ke dalam file sementara milik aplikasi dengan nama aslinya. Pembacaan penyedia file (*file-provider*) dikoordinasikan di luar utas UI, dan cakupan keamanannya berakhir sebelum penangan berjalan. Pindahkan hasil impor ke `cx.DataDir()` untuk menyimpannya; suntingan memengaruhi salinan tersebut. iOS menggunakan ikon dokumen aplikasi; `icon` berlaku di macOS. App Clip dan paket tvOS tidak mendaftarkan jenis dokumen apa pun. Pembukaan file tetap terpisah dari `OnOpenLinks`.

`desktop.AddRecentFile(cx, path)` mencatat dokumen yang dibuka pengguna: di menu Dock dan File › Open Recent di macOS, kategori Recent di Jump List di Windows, dan file yang baru digunakan desktop di Linux. Pilihan dari sana mencapai penangan yang sama. `desktop.ClearRecentFiles` melupakan pendaftaran aplikasi, sembari mempertahankan riwayat file bersama milik aplikasi lain di Linux; Linux juga menghormati pengaturan riwayat GTK.

`desktop.SetDockMenu(cx, items)` mengatur aksi pada ikon aplikasi: menu Dock di macOS, tugas Jump List di Windows, dan quicklist di dock Ubuntu, Plasma, serta Dash to Dock di Linux apabila quicklist didukung. Linux memerlukan entri desktop terpasang yang cocok dengan `id` aplikasi, dan mempertahankan submenu, pemisah, serta item yang dicentang dan dinonaktifkan. Menu tersebut dimiliki oleh cakupan yang mengaturnya, dan penyetel terakhir di seluruh jendela aplikasi menggantikannya.

## Tautan

`url_schemes` memberi aplikasi tautan tersendiri, seperti `myapp://settings`, yang membukanya dari peramban, aplikasi lain, `open`, `start`, atau `xdg-open`. Aplikasi macOS, iOS, dan Android mendaftarkannya, begitu pula penginstal Windows, paket MSIX, dan entri desktop Linux yang dibawa paket Debian, Arch, AppImage, serta Flatpak. Pencopotan akan menghapusnya.

```yaml
url_schemes: [myapp]
# app_links: [links.example.com]  # HTTPS hosts for the full app
```

Aplikasi mendengarnya melalui `desktop.OnOpenLinks`, termasuk saat sedang berjalan: di Windows dan Linux, peluncuran kedua menyerahkan tautannya ke aplikasi yang sedang berjalan lalu keluar. Tautan yang meluncurkan aplikasi menunggu penangan pertama. Siapa pun dapat membuat tautan semacam itu, dan di Windows serta Linux penangan menerima URL apa pun yang dipakai untuk memulai aplikasi, jadi periksa skemanya dan tanyakan pengguna sebelum tautan mengubah apa pun:

```go
desktop.OnOpenLinks(cx, func(links []string) { show(links[0]) })
```

Di iOS dan Android, `mobile.OnOpenLinks` mendengar tautan saat peluncuran dan saat aplikasi berjalan. Buka satu tautan dengan `xcrun simctl openurl booted myapp://settings` di simulator iOS atau `adb shell am start -a android.intent.action.VIEW -d myapp://settings` di Android. `app.OpenLinks("myapp://settings")` mengirimkannya dalam pengujian.

Untuk universal link iOS, deklarasikan host HTTPS eksplisit dengan `app_links: [links.example.com]`. Setiap host adalah nama DNS tanpa skema, jalur, port, alamat IP, atau wildcard; nama diubah ke huruf kecil dan duplikat ditolak. Alat ini menulis `applinks:<host>` ke entitlement Associated Domains aplikasi penuh, termasuk penandatanganan yang dikelola Xcode; profil IPA harus mengizinkannya. App Clip mempertahankan klaim `appclips:` terpisah dari `app_clip.domains`.

Setiap host harus menyajikan file `apple-app-site-association` yang menamai aplikasi penuh di `https://<host>/.well-known/apple-app-site-association`; metadata paket saja tidak dapat mengotorisasi universal link. [Persyaratan asosiasi Apple](https://developer.apple.com/documentation/xcode/supporting-associated-domains) menjelaskan penyiapan servernya. Tanda tangan simulator ad hoc menghilangkan entitlement pengembang terbatas, sehingga pengujian delegate tidak membuktikan asosiasi server.

Di iOS, `mobile.OpenedURL(cx)` melacak pemanggilan App Clip, universal link, atau URL skema kustom terbaru, termasuk nilai awal sebelum root dipasang. Pengiriman skema kustom tetap mencapai `mobile.OnOpenLinks`; nilai pemanggilan sudah mutakhir saat penangan tersebut berjalan. Pembukaan file tetap berada di `OnOpenFiles` dan tidak menggantikan URL pemanggilan. Dukungan pemanggilan Android/Web dilacak secara terpisah di #1475.

## Berbagi masuk di Android

Pilih tipe MIME yang ditangani aplikasi dari lembar berbagi (*share sheet*) Android:

```yaml
android:
  share_types: [text/plain, 'image/*', application/pdf]
```

Build APK dan App Bundle mendaftarkan tipe-tipe tersebut baik untuk item tunggal maupun ganda. Daftarkan [`mobile.OnShared`](services.md#membuka-tautan-dan-file) untuk menerima setiap kiriman berbagi.

Untuk aplikasi web dengan alamat berbasis jalur, atur `web.base` ke direktori deployment-nya (`/` atau `/my-app/`) dan konfigurasikan server produksi Anda agar menyajikan `index.html` untuk rute-rute aplikasi. Server pengembangan sudah menyediakan fallback tersebut untuk permintaan navigasi HTML. Aset tetap relatif terhadap HTML base tetap itu ketika deep link dimuat ulang. Lihat [Alamat aplikasi](addresses.md).

## Berbagi masuk di iOS

Deklarasikan Uniform Type Identifiers yang diterima ekstensi berbagi (*share extension*), dengan tipe URL dan teks sebelum tipe file yang lebih luas:

```yaml
apple:
  share_types: [public.url, public.plain-text, public.image, com.adobe.pdf]
  # app_group: group.com.example.app  # defaults to group.<id>
sign:
  # share_profile: assets/Share.mobileprovision
```

`build` dan `dev` memaketkan ekstensi berbagi Go, `<id>.share`, bersama aplikasi. Xcode menandatanganinya dan mendaftarkan App Group-nya untuk dijalankan di perangkat. Untuk IPA, aktifkan App Group yang sama untuk `<id>` dan `<id>.share`; `sign.profile` dan `sign.share_profile` harus memberikannya. App Clip, tvOS, dan watchOS tidak memiliki ekstensi berbagi.

Ekstensi ini menyalin hingga 128 item dan 512 MiB file ke dalam App Group, lalu menyelesaikan lembar berbagi. `mobile.OnShared` menerima batch tersebut saat aplikasi aktif atau ketika pengguna membukanya kembali; data tetap bertahan meskipun ekstensi dan aplikasi keluar sebelum pengiriman. iOS tidak menyediakan API yang didukung bagi ekstensi Share untuk membawa aplikasi induknya ke latar depan atau meluncurkannya. Oleh karena itu, penyerahan ini tidak membuka aplikasi secara otomatis.

## Program mini

```sh
go tool nsgo build -target miniprogram_wechat -run
go tool nsgo dev -target miniprogram_alipay
```

Setiap target menulis proyek untuk alat pengembang platformnya, juga di `dist/releases/miniprogram/<platform>`, yang dibuka oleh `-run` dan `dev`; `dev` membangun ulang saat Anda menyimpan, dan alat tersebut memuat ulang:

| Target | Platform | Alat |
| --- | --- | --- |
| `miniprogram_wechat` | WeChat | [WeChat DevTools](https://developers.weixin.qq.com/miniprogram/dev/devtools/download.html), dengan port layanannya aktif (Settings, Security) |
| `miniprogram_qq` | QQ | [Alat pengembang QQ](https://q.qq.com/wiki/tools/devtool/), di macOS |
| `miniprogram_alipay` | Alipay | [Mini Program Studio Alipay](https://opendocs.alipay.com/mini/ide/download), di macOS |
| `miniprogram_douyin` | Douyin dan aplikasi ByteDance lainnya | [Alat pengembang Douyin](https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/developer-instrument/download/developer-instrument-update-and-download), di macOS |

`miniprogram.appid` memberikan AppID program mini di setiap platform (`wechat`, `qq`, `alipay`, `douyin`); tanpanya, WeChat DevTools mempratinjaunya dengan tourist AppID dan alat lain akan memintanya. Di WeChat, `-run` dan setiap build ulang `dev` menutup lalu membuka ulang proyek secara otomatis melalui CLI, sehingga DevTools mengompilasi file-file baru. Ini telah diuji dengan DevTools 2.01.2510290. Bilah navigasi platform menampilkan judul aplikasi; dengan `custom_navigation`, aplikasi mengisi seluruh halaman dan safe area-nya menjaganya tetap bersih dari tombol menu. Aplikasi membaca OS ponsel dengan `Env.Simulated` bernilai true, seperti di web dengan `--nsgo-os`: halaman-halaman tumpukan (*stack*) menampilkan `Header`-nya sendiri, karena tidak ada tajuk sistem yang menggantikannya. QQ, Alipay, dan Douyin belum dicoba di alat masing-masing, dan Alipay belum mendukung tombol kembali melalui aplikasi, keyframe, atau tinggi papan ketik.

Program mini masih bersifat eksperimental. Program mini hanya menjalankan aplikasi Go, bukan TSX, dan platform membatasi modul terkompresi aplikasi hingga 2 MB: aplikasi beberapa layar di atas `base` muat, seperti `examples/gallery-miniprogram`, sedangkan kontrol `ui` belum muat, dan `build` akan memberi tahu jika melebihi batas. Shader tidak menggambar apa pun di sana, dan tampilan web, video, komponen native, serta file font aplikasi belum ditampilkan.

## HarmonyOS

```sh
go tool nsgo build -target harmonyos -run
```

`harmonyos` menulis `<name>-<version>-harmonyos.hap` ke `dist/releases`. Mesin berjalan sebagai WebAssembly di dalam ArkWeb, dan host ArkTS menggambar apa yang dipasangnya menggunakan tampilan ArkUI, sehingga teks, kolom input, pengguliran, dan dialog adalah milik sistem itu sendiri. Dengan `hdc` di `PATH`, `-run` memasang HAP di perangkat atau emulator yang terhubung dan memulainya, serta `nsgo test -target harmonyos` menjalankan pengujian aplikasi dari HAP tanpa tanda tangan di emulator di bawah identitas bundel `.nsgotest` terpisah, lalu menghapus HAP pengujian tersebut setelah selesai agar aplikasi yang terpasang tetap tersedia.

Untuk mengompilasi host ArkTS dari templat dan menggunakan `id` aplikasi sebagai nama bundel, taruh `hvigorw` di `PATH` dan tetapkan `OHOS_BASE_SDK_HOME` ke OpenHarmony SDK dengan API 26 (`26.0.0`), yaitu versi SDK kompilasi dan target templat. `build.harmonyos_bundles` memilih `auto` (bawaan), `exact` (yang memerlukan SDK), atau `padded` (yang mengisi `id` hingga panjang tetap host yang dikomit dan tidak memerlukan SDK). Pengujian target menggunakan pengaturan yang sama dan mencetak host mana yang dipaketkan. `harmonyos.permissions` mencantumkan nama `ohos.permission` yang diminta aplikasi, seperti `MICROPHONE` dan `CAMERA`, dan `widget.title` adalah yang ditampilkan kartu layar berandanya.

HarmonyOS masih berupa pratinjau. Emulator dapat memasang HAP tanpa tanda tangan. [Catatan arsitektur](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/harmonyos.md) mencantumkan fitur ArkUI yang digambar oleh pratinjau ini.

### HAP bertanda tangan

Tetapkan `harmonyos.signing` untuk menandatangani HAP dengan materi AppGallery Connect Anda. Pemasangan langsung di ponsel Huawei memerlukan sertifikat debug dan profil untuk perangkat tersebut. Gunakan materi rilis untuk distribusi. [Panduan penandatanganan Huawei](https://developer.huawei.com/consumer/en/doc/harmonyos-guides-V2/command-line-building-app-hap-hvigor-0000001553061909-V2/)

```yaml
harmonyos:
  signing:
    keystore: signing/app.p12
    key_alias: app
    certificate: signing/app.cer
    profile: signing/app.p7b
```

Jalur bersifat relatif terhadap `nsgo.yaml`. Tetapkan `NSGO_HARMONY_KEYSTORE_PASSWORD` dan `NSGO_HARMONY_KEY_PASSWORD` di variabel lingkungan. Penandatanganan menggunakan `hap-sign-tool.jar` milik SDK serta Java dari `JAVA_HOME` atau `PATH`. Build mempertahankan `id` persis aplikasi agar cocok dengan profil, sehingga penandatanganan memerlukan SDK dan `hvigorw` bahkan dalam mode `auto`. `go tool nsgo doctor`, yang dijalankan di direktori aplikasi, memeriksa konfigurasi penandatanganan dan nama bundel profil tanpa kata sandi. `build -run` memberikan HAP yang telah ditandatangani ke `hdc`.

## Penandatanganan

```yaml
sign:
  identity: "Apple Development: you@example.com (ABCDE12345)"  # signs the macOS app, else ad hoc
  profile: assets/AppStore.mobileprovision  # signs the IPA
  keystore: assets/release.p12              # signs APKs, with key_alias
  certificate: assets/codesign.pfx          # signs the Windows app and its installer
  # widgets_profile: assets/Widgets.mobileprovision  # the iOS extension's profile; use a macOS profile for a Mac App Store build
  # share_profile: assets/Share.mobileprovision      # signs the IPA's share extension
  # clip_profile: assets/Clip.mobileprovision        # signs the IPA's App Clip
```

Kata sandi keystore berasal dari `NSGO_KEYSTORE_PASSWORD` dan `NSGO_KEY_PASSWORD`. `-team` menandatangani paket iPhone. `-profile`, `-widgets-profile`, `-share-profile`, `-clip-profile`, dan `-macos-profile` mengganti profil provisi aplikasi, ekstensi widget, ekstensi berbagi, App Clip, dan Mac App Store dari baris perintah. `-installer` mengganti identitas Mac Installer Distribution.

### Menandatangani paket Windows

Authenticode mengidentifikasi penerbit unduhan Windows; [SmartScreen memeriksa reputasinya secara terpisah](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation). `sign.certificate`, sebuah `.pfx`, menandatangani berkas eksekusi, pendamping perintah, program serta pustaka yang dibawa sumber daya aplikasi, penginstal beserta uninstaller-nya, dan MSIX yang dibangun untuk saluran Store; kata sandinya berasal dari `NSGO_CERTIFICATE_PASSWORD`. Tanda tangan dibuat dalam Go, sebagaimana `signtool` membuatnya, sehingga Mac menandatanganinya tanpa Windows SDK:

```yaml
sign:
  certificate: assets/codesign.pfx
  timestamp_url: http://timestamp.digicert.com  # the RFC 3161 server that timestamps them
```

`sign.timestamp_url` secara bawaan menunjuk ke server DigiCert, dan `none` membiarkan tanda tangan tanpa cap waktu, sehingga tanda tangan kedaluwarsa saat sertifikat berakhir. Sertifikat pada token perangkat keras atau di layanan cloud menandatangani melalui perintahnya sendiri sebagai gantinya, di mana `%1` adalah file yang akan ditandatangani dan yang dijalankan build untuk setiap file di atas:

```yaml
sign:
  sign_command: "signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a %1"
```

`build` berhenti jika perintah berhasil tetapi meninggalkan berkas eksekusi aplikasi, pendamping perintah, sumber daya PE, atau berkas eksekusi setup tanpa tanda tangan Authenticode yang tersemat.

Uninstaller di dalam penginstal ditandatangani sebelum disematkan, dengan `sign.certificate` atau `sign.sign_command` yang sama seperti aplikasi dan setup. `-certificate`, `-sign-command`, dan `-timestamp-url` mengaturnya dari baris perintah.

### Notarisasi

Aplikasi yang diunduh di Mac terbuka tanpa peringatan Gatekeeper setelah Apple melakukan notarisasi padanya. Tandatangani dengan Developer ID dan sebutkan kredensial yang disimpan `notarytool` di keychain Anda:

```sh
xcrun notarytool store-credentials hello --apple-id you@example.com --team-id ABCDE12345
```

```yaml
sign:
  identity: "Developer ID Application: Example Inc (ABCDE12345)"
  notarize: hello   # the keychain profile above; or -notarize hello
```

`build` kemudian menandatangani dengan hardened runtime, mengirimkan disk image, menunggu keputusan Apple selama beberapa menit, dan menempelkan (*staple*) tiket ke image serta aplikasi; penolakan akan mencetak log Apple. Di CI, `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID`, dan `NSGO_NOTARY_ISSUER`—file `.p8`, ID, dan penerbit kunci API App Store Connect—menggantikan profil tersebut. Hardened runtime menolak mikrofon, kamera, kalender, dan lokasi kecuali aplikasi mengklaimnya, yang dilakukan `build` ketika `apple.info` memiliki `NSMicrophoneUsageDescription`, `NSCameraUsageDescription`, deskripsi penggunaan kalender atau pengingat, atau deskripsi penggunaan lokasi.

## Widget dan Live Activities

`build` dan `dev` mengompilasi ekstensi yang menggambar widget dan Live Activities aplikasi ke dalam paket macOS dan iOS menggunakan kompilator Swift milik Xcode, tanpa langkah terpisah. Di iOS, aplikasi dan widget-nya berbagi grup aplikasi: `-run` di iPhone membuat Xcode menandatangani keduanya dan mendaftarkan grupnya; untuk IPA, aktifkan untuk `<id>` maupun `<id>.widgets`, dan tandatangani ekstensi dengan `sign.widgets_profile`. APK dan AAB mandiri Android menyertakan penyedia App Widget dan sumber daya dari `widgets`; runtime Java prabangun menggambar linimasanya tanpa perlu SDK atau kompilator Java oleh build aplikasi. Android 16 menampilkan Live Activities sebagai Live Updates, yang tidak memerlukan ekstensi. [Widget](widgets.md) dan [Live Activities](live-activities.md) membahas konfigurasi dan API-nya.

## Android App Links

`app_links` menyebutkan host HTTPS eksplisit yang tautannya membuka aplikasi Android penuh:

```yaml
app_links: [example.com, shop.example.com]
```

APK dan App Bundle mendeklarasikan filter `ACTION_VIEW` yang dapat dijelajahi dengan `autoVerify` untuk setiap host. Gunakan host DNS tanpa skema, jalur, port, wildcard, atau alamat IP. Sajikan `https://<host>/.well-known/assetlinks.json` berisi ID paket aplikasi dan sidik jari sertifikat penandatanganan; Android memverifikasi asosiasi tersebut sebelum mengarahkan tautan ke aplikasi. Alat ini tidak menerbitkan file asosiasi; [panduan verifikasi Android](https://developer.android.com/training/app-links/verify-applinks) menjelaskan penyiapannya. Host Android yang disematkan mendeklarasikan filter intent-nya sendiri dan meneruskan tautan yang diterima ke `NsgoView`-nya.

`mobile.OpenedURL(cx)` membaca pemanggilan yang diterima sebelum render root pertama dan berubah ketika tautan lain membuka aplikasi yang sedang berjalan. `mobile.OnOpenLinks` tetap mendengar setiap tautan yang diterima. Aplikasi web satu halaman penuh membaca alamat halamannya saat ini melalui `mobile.OpenedURL`, termasuk riwayat peramban dan perubahan hash; aplikasi yang dipasang di dalam halaman lain tidak membaca alamat halaman induknya. Gunakan [`cx.AppURL`](addresses.md) ketika aplikasi juga menulis alamatnya sendiri.

## App Clips

App Clip adalah bagian kecil dari aplikasi iOS yang berjalan dari tautan, kode, atau tag NFC tanpa memasang aplikasinya. `app_clip` membangun satu App Clip ke dalam paket iOS, dari paket `main` tersendiri sehingga hanya menautkan apa yang ditampilkannya:

```yaml
app_clip:
  main: ./clip              # the clip's main package, the app's own by default
  domains: [example.com]    # whose links open it
  # name: Hello Clip        # "<name> Clip" by default
  # id: dev.example.hello.Clip  # <id>.Clip by default
  # minimum: "17.0"         # 16.0 by default; 17.0 allows 100 MB when only links open it
  # notifications: true     # ask for notifications for a day
  # location: true          # let the clip confirm where it was opened
```

Clip membaca tautan yang membukanya dengan `mobile.OpenedURL`, yang berubah saat tautan lain membukanya kembali, dan `mobile.IsAppClip` menyatakan apakah kode berjalan sebagai App Clip. Sistem mengunduh clip hingga 15 MB, atau 100 MB mulai iOS 17 jika hanya tautan yang membukanya, dan `build` menolak yang lebih besar; clip yang menautkan `ui` memakan sekitar 11 MB. `dev -target ios_simulator -clip https://example.com/order?table=12` menjalankan clip saja di simulator, dibuka dari tautan tersebut; `-target ios` menjalankannya di iPhone dan mencetak kode QR tautannya: tambahkan tautan di Settings > Developer > Local Experiences, lalu pindai dengan kamera. Gunakan `-release` jika simbol pengembangan mendorong ukuran clip melewati batas ukuran QR. Apple hanya menandatangani App Clip untuk tim Apple Developer Program, bukan tim personal.

`dev -target ios` dan `build -run -target ios` juga menyimpan clip di dalam aplikasi penuh: Xcode menandatanganinya sebagai target terpisah dan menyematkannya sebelum menandatangani aplikasi. Penandatanganan otomatis memerlukan akun tim berbayar yang terautentikasi di Xcode, dengan provisi untuk aplikasi maupun clip. Untuk App Store, aktifkan App Clips untuk `<id>`, tandatangani clip dengan `sign.clip_profile`, dan sajikan file `apple-app-site-association` milik domain tersebut. `examples/appclip` di repositori adalah contoh kafe yang clip-nya memesan di meja.

Untuk menyerahkan pesanan atau data lokal lainnya ke aplikasi penuh, tetapkan grup eksplisit bersama `app_clip`:

```yaml
apple:
  app_group: group.dev.example.hello
```

Clip dan aplikasi penuh kemudian berbagi `cx.DataDir()` dan `cx.Storage()` di dalam kontainer grup tersebut. File di bawah direktori data dan entri kunci-nilai yang ditulis clip tersedia saat aplikasi penuh dimulai; `cx.LogsDir()` mengikuti direktori tersebut, sedangkan setiap bundel tetap menyimpan cache masing-masing. Penandatanganan Xcode mengklaim grup tersebut untuk kedua target; saat menggunakan profil, aktifkan grup yang sama untuk kedua App ID dan berikan izinnya di `sign.profile` serta `sign.clip_profile`. Izin yang tidak ada menolak penandatanganan, dan grup yang tidak dapat diakses menghentikan startup. Direktori data eksplisit milik host tetap diutamakan.

Tanpa `apple.app_group`, setiap bundel menyimpan penyimpanan sandbox masing-masing. Mengaktifkan pengaturan ini tidak memigrasikan data yang sebelumnya tersimpan di sandbox tersebut, dan aplikasi khusus widget tetap menggunakan lokasi penyimpanannya yang sudah ada. Simpan rahasia bersama di keychain alih-alih penyimpanan biasa. Contoh kafe menyimpan pesanan clip-nya dan menampilkannya di aplikasi penuh.

Dari aksi pengguna eksplisit di dalam clip, panggil `mobile.ShowAppClipOverlay(cx, mobile.AppClipOverlayBottom)` untuk meminta StoreKit merekomendasikan aplikasi penuh yang sesuai. `AppClipOverlayBottomRaised` menyisakan ruang untuk bilah bawah. Fungsi yang dikembalikan menutup permintaan tersebut; pembersihan cakupan juga menutupnya, dan permintaan yang lebih lama tidak dapat menutup yang lebih baru. Pemanggilan di luar App Clip iOS native tidak melakukan apa pun. App Store menentukan apakah ia dapat memuat dan menampilkan rekomendasi, sehingga pemanggilan tersebut tidak melaporkan apakah penawaran ditampilkan maupun apakah pemasangan terjadi. Jaga tugas clip tetap dapat digunakan tanpa memasang aplikasi penuh. [Panduan overlay Apple](https://developer.apple.com/documentation/appclip/recommending-your-app-to-app-clip-users)

## Menyematkan di aplikasi lain

`-embed` memaketkan aplikasi untuk aplikasi iOS atau Android milik Anda sendiri: berupa XCFramework, dari framework dinamis dengan `-dynamic`, atau AAR yang `NSGoView` atau `NsgoView`-nya menampilkan aplikasi tersebut. Program desktop Go menampilkan aplikasi dengan `nsgo.NewView`, dan halaman web dengan `nsgo.js` milik situs, yang host-nya adalah milik halaman tersebut atau yang disebutkan `web.host` di `nsgo.yaml`. `examples/embed-*` di repositori memperlihatkan masing-masing; [Menyematkan](embedding.md) membahas sisi host-nya.

---
title: Dibandingkan dengan framework lain
description: Posisi NSGo di samping NSH, MyGo, dan framework UI lainnya, serta apa yang dikorbankannya.
order: 2
---

Posisi NSGo di antara framework UI berasal dari tiga pilihan: aplikasi ditulis dalam Go (atau TSX di atasnya), setiap platform menggambar dengan view-nya sendiri, dan satu basis kode mencakup ponsel, desktop, serta web. Framework lain membuat pilihan berbeda, dan setiap pilihan memiliki konsekuensi.

## Sekilas

| Framework | Bahasa | Menggambar dengan | Target |
| --- | --- | --- | --- |
| NSGo | Go, TSX | View milik masing-masing platform | iOS, App Clips, tvOS, watchOS, Android, macOS, Windows, Linux, web; mini program, secara eksperimental |
| [NSH](https://nsh.dev) | Rust, TSX | View milik masing-masing platform | iOS, Android, macOS, Windows, Linux, web, aplikasi GPUI, terminal; HarmonyOS dan mini program WeChat melalui platform komunitas |
| [MyGo](https://github.com/egoist/mygo) | Go, halaman web | Renderer GPU sendiri, atau webview sistem | macOS, Windows, Linux, web |
| [Fyne](https://fyne.io), [Gio](https://gioui.org) | Go | Renderer sendiri | Desktop, ponsel, web |
| [Wails](https://wails.io) | Go, halaman web | Webview sistem | Desktop |
| [Flutter](https://flutter.dev) | Dart | Renderer sendiri | Ponsel, desktop, web |
| [Compose Multiplatform](https://www.jetbrains.com/compose-multiplatform/) | Kotlin | Renderer sendiri; Compose milik Android | Ponsel, desktop, web |
| [React Native](https://reactnative.dev) | JavaScript, TSX | View milik masing-masing platform | iOS, Android; lainnya melalui platform komunitas |
| [.NET MAUI](https://dotnet.microsoft.com/apps/maui) | C# | Kontrol milik masing-masing platform | iOS, Android, macOS, Windows |
| [Avalonia](https://avaloniaui.net) | C# | Renderer sendiri | Desktop, ponsel, web |
| [Qt](https://www.qt.io) | C++, QML | Renderer sendiri | Desktop, ponsel, perangkat tertanam |
| [Electron](https://www.electronjs.org), [Tauri](https://tauri.app) | Halaman web; Rust untuk Tauri | Chromium bawaan; webview sistem untuk Tauri | Desktop; ponsel untuk Tauri |

## Keunggulan

- **View native.** Teks, metode input, seleksi, pengguliran, aksesibilitas, dan menu adalah milik platform itu sendiri, sehingga aplikasi berperilaku sebagaimana yang diharapkan pengguna di sana, sementara framework yang menggambar sendiri harus membangun ulang masing-masing fitur tersebut. Apa yang hanya dimiliki satu platform, seperti Liquid Glass di iOS, ditiru dengan cara yang sama oleh platform lain alih-alih dihilangkan.
- **Pembaruan terperinci (fine-grained).** Komponen berjalan sekali dan signal hanya memperbarui bagian yang membacanya, tanpa virtual DOM atau fungsi view per frame untuk di-diff; animasi berjalan pada jam frame platform.
- **Paket terkecil dan start tercepat.** Dalam [tolok ukur](https://nsgo.dev/benchmarks/), aplikasi penghitung yang sama dikemas menjadi sekitar 8 MB di macOS, Windows, dan Linux—lebih kecil dari MyGo, GPUI, dan NSH (33 hingga 45 MB untuk NSH); aplikasi ini juga mulai paling cepat, dan memakai lebih sedikit memori dibandingkan framework yang menggambar di GPU, seperti MyGo dan GPUI.
- **Hanya Go.** Engine, tata letak, dan backend semuanya ditulis dalam Go, tanpa C, tanpa cgo, dan tanpa engine JavaScript untuk aplikasi Go.
- **Semua platform dari satu Mac.** Sebuah Mac dengan Go dan Xcode dapat melakukan cross-compile ke setiap target: tanpa NDK, Android SDK, JDK, GTK, atau toolchain Windows.
- **Semua file rilis dari satu perintah.** `nsgo build` menghasilkan image disk yang ditandatangani dan dinotarisasi untuk macOS, installer Windows yang ditandatangani, paket deb dan Arch serta AppImage, paket untuk Mac App Store, Microsoft Store, Google Play, dan Flathub, serta skrip yang memasang dari rilis GitHub ([pembangunan](building.md)).
- **Pembaruan bawaan masing-masing platform.** Di macOS Sparkle, dan di Windows WinSparkle, mengunduh, memverifikasi, dan memasang versi berikutnya; di Linux dan Android aplikasi menemukannya dan membuka halaman rilisnya; di iOS App Store yang menanganinya. Aplikasi yang dipasang melalui toko aplikasi diserahkan kepada toko tersebut ([pembaruan](building.md#pembaruan)).
- **Setiap platform.** Ponsel, tablet, TV, jam tangan pintar, desktop, dan web berbagi satu engine dan satu API, bersama dengan iOS App Clips, widget iPhone dan Mac, serta—secara eksperimental—mini program WeChat, QQ, Alipay, dan Douyin.
- **Penyematan.** Aplikasi NSGo dapat ditempatkan sebagai sebuah view di dalam aplikasi iOS, Android, desktop Go, atau web yang sudah ada, sehingga Anda bisa bermigrasi layar demi layar ([penyematan](embedding.md)).
- **Siklus pengembangan cepat.** Menyimpan file Go akan membangun ulang dan memulai ulang aplikasi; perubahan TSX memperbarui aplikasi yang sedang berjalan di tempat dan mempertahankan state-nya; error build dan panic saat merender ditampilkan di atas aplikasi ([menjalankan dan debugging](debugging.md)).
- **Satu set pengujian di mana saja.** Pengujian menemukan dan menggunakan kontrol berdasarkan teks atau peran seperti yang dilakukan pengguna, secara cepat tanpa jendela, atau tanpa perubahan apa pun di dalam jendela nyata pada setiap target dengan input milik platform tersebut, sambil menyimpan tangkapan layar ([pengujian](testing.md)).
- **Pelokalan bawaan.** Kata-kata aplikasi dan komponennya tersimpan di dalam katalog, dan pseudo-locale menemukan teks yang belum diterjemahkan, terpotong, atau belum siap untuk arah kanan-ke-kiri ([pelokalan](localization.md)).
- **Shader dalam Go.** `core.Shader` menjalankan shader GPU yang ditulis dalam sintaks Go atau WGSL, di setiap platform ([kanvas](canvas.md#shader)).
- **TSX bila diinginkan.** Layar dapat ditulis dalam TSX dengan kelas Tailwind, di atas engine yang sama dengan komponen Go.

## Kelemahan

- **Masih awal.** 0.1.0 adalah rilis pertama, dan API-nya belum stabil; [peta jalan](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md) mencantumkan hal-hal yang belum ada, seperti batas heap untuk TSX yang dikirim saat runtime.
- **Ekosistem kecil.** Masih sedikit komponen pihak ketiga, contoh, atau jawaban daring dibandingkan Flutter, React Native, atau Qt.
- **Platform tetap berbeda.** View native berarti terdapat perbedaan kecil antar platform, sehingga aplikasi tetap perlu diperiksa di masing-masing platform. Framework yang menggambar sendiri tampak sama di mana saja.
- **Bridge tersendiri.** Di Android program Go berjalan berdampingan dengan runtime Java dan berkomunikasi melaluinya lewat pipe, dan di Linux pengguna membutuhkan GTK 4.14 atau yang lebih baru.
- **TSX diinterpretasikan.** TSX berjalan di goja, sebuah interpreter JavaScript yang ditulis dalam Go, yang lebih lambat daripada engine yang dipakai React Native atau Electron.

## NSH

NSGo mem-porting desain NSH: arsitektur, backend, penamaan API, serta komponen tanpa gaya di `base` adalah milik NSH ([status](index.md#status)).

- **Pilih NSGo** untuk menulis aplikasi dalam Go alih-alih Rust atau TSX, untuk membangun setiap target dari satu Mac dengan Go dan Xcode, atau untuk fitur yang ditambahkan NSGo: shader GPU yang ditulis dalam Go, ikon vektor, terminal, aplikasi yang membuka jendelanya sendiri, panel dan ikon baki, serta komponen bergaya di `ui`.
- **Pilih NSH** untuk proyek yang lebih matang, TSX yang dijalankan oleh QuickJS dengan bytecode dan batas heap, backend GPUI, atau Android 32-bit.

## MyGo

Keduanya adalah UI yang ditulis dalam Go tanpa cgo, tetapi MyGo menggambar sendiri UI native-nya di GPU dan membangunnya ulang dari fungsi view di setiap frame, sedangkan NSGo hanya mengirimkan bagian yang berubah ke view native.

- **Pilih MyGo** untuk aplikasi desktop yang tampak sama di setiap desktop, jendela halaman web dengan pemanggilan bertipe ke Go, serta API desktop dan pengirimannya: ikon baki, asosiasi file, installer, notarisasi, dan pembaruan delta yang ditandatangani.
- **Pilih NSGo** untuk menjangkau ponsel juga, memanfaatkan teks, input, dan aksesibilitas bawaan masing-masing platform, atau untuk state yang disimpan di dalam signal alih-alih digambar ulang dari data aplikasi.

## Toolkit Go lainnya

- **Fyne dan Gio** menggambar semuanya sendiri, seperti halnya MyGo, dan keduanya menjangkau ponsel serta web. Fyne menyediakan set widget lengkap dengan tampilannya sendiri; Gio bersifat immediate mode, tingkat rendah, dan fleksibel. Keduanya memiliki versi rilis dan pengguna; tidak satu pun tampak atau terasa native.
- **Wails** menempatkan frontend web di dalam webview sistem dengan Go di belakangnya: pustaka web apa pun dapat digunakan, tetapi antarmukanya adalah halaman web, dan hanya untuk desktop.

## Framework dalam bahasa lain

- **Flutter dan Compose Multiplatform** menggambar setiap pikselnya sendiri: identik di setiap platform dan matang, dengan ekosistem besar, tetapi teks, pengguliran, dan input dibangun ulang alih-alih native, dan Anda perlu mempelajari Dart atau Kotlin.
- **React Native** memiliki model yang sama dengan NSGo berupa inti reaktif yang menggerakkan view native, dan memiliki ekosistem terbesar di kelasnya. Framework ini berfokus pada iOS dan Android, serta menjalankan logika aplikasi di JavaScript; NSGo menjalankannya di Go dan memperlakukan desktop serta web sebagai warga kelas satu.
- **.NET MAUI** membungkus kontrol setiap platform dalam C#, tanpa Linux atau web. **Avalonia** menggambar sendiri, mirip dengan WPF, di setiap desktop, ponsel, dan web.
- **Qt** matang, cepat, dan menjangkau perangkat tertanam, tetapi menggambar widget-nya sendiri, membutuhkan C++ atau QML, dan dilisensikan di bawah LGPL atau lisensi komersial.
- **Electron dan Tauri** menampilkan halaman web: alat dan pustaka web secara penuh, dengan biaya memori sebuah browser—terutama pada Electron—serta antarmuka yang terasa seperti halaman web.

## Memilih

Pilih NSGo untuk aplikasi yang harus terasa native di ponsel, desktop, dan web dari satu basis kode Go, bila proyek tahap awal dapat diterima. Untuk aplikasi yang harus segera dirilis di atas API yang stabil, pilih framework matang dari tabel di atas: Flutter atau React Native untuk ponsel, MyGo, Wails, atau Tauri untuk desktop saja.

---
title: Picture in Picture
description: Putar video dalam Picture in Picture di iOS, tvOS, macOS, Android, dan browser, atau tampilkan pohon kustom di iOS.
group: Fitur platform
order: 45
---

Di iOS, deklarasikan pemutaran latar belakang sebelum memakai Picture in Picture; macOS tidak memerlukan mode latar belakang:

```yaml
apple:
  info:
    UIBackgroundModes: [audio]
```

Untuk video, aktifkan `PictureInPicture`, simpan sebuah `core.Ref`, lalu mulai atau hentikan dari kontrol aplikasi. Ikuti callback status untuk mengaktifkan kontrol tersebut saat `Possible` menjadi true:

```go
ref := &core.Ref{}
state := core.NewSignal(cx, core.PictureInPictureState{})
video := core.Video{
    Source: "asset:clip.mp4", Ref: ref,
    PictureInPicture: true, AutoPictureInPicture: true,
    OnPictureInPicture: state.Set,
}
start := ui.Button{
    Text: "Picture in Picture", OnPress: ref.StartPictureInPicture,
    Disabled: func() bool { return !state.Get().Possible },
}
```

`AutoPictureInPicture` mengaktifkan masuk otomatis saat aplikasi iOS berpindah ke latar belakang; tvOS dan macOS mengabaikan opsi ini. `Controls` tetap menampilkan kontrol pemutaran native; tombol PiP milik aplikasi memakai sesi terkelola yang sama untuk video yang dikontrol. `OnPictureInPictureRestore` memungkinkan aplikasi menampilkan halaman pemutarnya saat pengguna kembali. Biarkan video tetap terpasang selama PiP aktif: menghapus scope-nya akan menghentikan pemutaran dan PiP.

Di Android, activity yang dipaketkan sudah mendeklarasikan dukungan PiP. Video masuk ke jendela PiP milik activity, dengan antarmuka aplikasi lainnya disembunyikan, dan mempertahankan pemutar serta posisinya saat kembali. `StopPictureInPicture` memperluas activity yang sama. Kontrol putar/jeda jendela melaporkan `OnPlay` dan `OnPause`, dengan `VideoPauseControls`; `Ref.Seek` memakai pemutar yang sama. Masuk otomatis memerlukan pengaturan eksplisit serta video yang sudah disiapkan dan sedang diputar. Memulai tetap bisa gagal karena kondisi sistem saat itu; tangani `Error` meskipun `Possible` bernilai true.

Host Android yang disematkan harus mendeklarasikan `android:supportsPictureInPicture="true"` dan menangani perubahan konfigurasi `orientation|screenSize|screenLayout|smallestScreenSize`. Teruskan `onPictureInPictureModeChanged` dan `onUserLeaveHint` milik activity ke metode yang sama pada `NsgoView`; di Android 15+, teruskan juga `PictureInPictureUiState.isTransitioningToPip()` ke `NsgoView.onPictureInPictureTransition`. Host atau perangkat tanpa dukungan melaporkan `Supported: false`.

Di web, mulailah dari ketukan pengguna di browser yang menawarkan API PiP video standar. `Supported` juga mencerminkan Permissions Policy halaman, dan `Possible` memerlukan metadata video yang sudah dimuat. Kontrol PiP milik browser sendiri melaporkan status aktif yang sama. Menghentikan, mengganti, atau mengosongkan sumber, atau menghapus video, mengakhiri sesinya; permulaan yang tertunda tidak dapat membuka kembali sumber sebelumnya. PiP web hanya menampilkan video, tanpa konten NSGo kustom, masuk otomatis di latar belakang, atau `OnPictureInPictureRestore`. Browser yang tidak mendukung melaporkan `Supported: false`.

Di iOS, `mobile.PictureInPicture` menerima `Children` dengan tata letaknya sendiri, dengan ukuran default 320 × 180 poin. Komponen ini tidak menempati ruang di aplikasi. `Ref`-nya memiliki metode mulai dan berhenti yang sama, sedangkan `OnState`, `OnRestore`, `OnPlay`, `OnPause`, dan `OnSkip` menyampaikan aksi sistem. Ikat `PausedFunc` dan `PositionFunc` ke status pemutaran yang diperbarui oleh callback tersebut; `Duration: 0` menggambarkan konten langsung. Aplikasi yang ditangguhkan mempertahankan frame terakhir yang dirender.

Taruh `core.Video` di pohon kustom untuk menggabungkan frame hasil dekodenya dengan teks keterangan yang digambar aplikasi atau overlay lainnya. Mulai sesi menggunakan `Ref` luar. Bagikan signal jeda dengan video, isi `PositionFunc` sesi kustom dari `Video.OnProgress`, dan teruskan `OnSkip` ke `Ref.Seek` milik video. Audio video diputar melalui pemutarnya sendiri.

Pohon kustom juga merender teks, tampilan, gambar, dan Canvas. WebView, shader Metal, dan efek backdrop tidak tersedia dalam frame yang ditangkap. Gambar animasi dan transisi layer native menampilkan status modelnya. Gunakan nilai animasi eksplisit untuk gerakan. Peninjauan App Store dapat menolak penggunaan yang tidak terkait dengan video.

`Supported: false` membedakan platform yang tidak tersedia dari sesi yang didukung tetapi belum siap atau yang `Error`-nya menjelaskan suatu kegagalan. Video mendukung iOS dan tvOS bila AVKit melaporkan bahwa perangkat mendukung PiP, macOS, Android, dan browser web yang menawarkan API standar; pohon kustom hanya mendukung iOS. Di iOS dan tvOS, sesi audio bersama tetap berada di kategori pemutaran selama PiP diaktifkan, termasuk saat dijeda, dan bercampur dengan audio aplikasi lain bila `AudioSession` video bernilai `AudioSessionMixWithOthers`.

Dalam pengujian headless, `nsgotest.PictureInPictureSupport(supported, possible)` mengatur apa yang ditawarkan sistem di bawah `nsgotest.Environment` iOS, macOS, atau Android, `PiPState`, `PiPStarts`, dan `PiPStops` milik tampilan mencatat sesinya, `app.ReportPictureInPicture` melaporkan status seperti yang akan dilaporkan sistem, dan `app.PictureInPictureControl` mengirim `play`, `pause`, `skip`, dan `restore` milik jendela. Jendela PiP video memutar dan menjeda pemutar milik video itu sendiri, yang dilaporkan oleh `Video.OnPlay` dan `Video.OnPause`, dengan `VideoPauseControls` untuk jeda; `app.PlayVideo` dan `app.PauseVideo` melaporkannya dalam pengujian.

Jalankan `go tool nsgo dev -target ios examples/pip` untuk kedua jalur, atau `-target macos` atau `-target android` untuk PiP video. Pengujian Mac native memverifikasi masuk, keluar, dan perintah berulang dengan dan tanpa kontrol native, serta penggantian sumber terkontrol yang sedang aktif. Uji penerimaan di iPhone, termasuk layar penuh video terkontrol dan masuk otomatis, masih tertunda; implementasinya saat ini memiliki pengujian delegate/frame native dan cakupan aplikasi headless.

Untuk video tvOS, jalankan `go tool nsgo dev -target tvos_simulator examples/pip`; NSGo mengaitkan pemutar dengan sesi Now Playing native. Simulator tvOS 27 memuat video dengan dan tanpa kontrol native, tetapi AVKit melaporkan PiP tidak didukung di sana. Masuk, berhenti, dan pemulihan PiP yang sebenarnya masih memerlukan verifikasi di Apple TV fisik.

Regresi target web memakai klip lokal yang direkam dan ketukan tombol aplikasi tepercaya untuk memverifikasi sesi PiP browser yang sebenarnya melalui masuk, keluar, penggantian sumber, dan penghapusan di Chromium. Mesin browser lainnya masih memerlukan uji penerimaan.

---
title: Izin
description: Periksa dan minta izin sistem, buka pengaturannya, dan pandu pengguna ke System Settings di macOS.
group: Fitur platform
order: 49
---

## Memeriksa dan meminta

Sistem menyerahkan sebagian hal yang boleh dilakukan aplikasi kepada pengguna untuk diberikan: mikrofon, kamera, kontak, kalender, pengingat, foto, Bluetooth, pustaka media, lokasi, dan notifikasi di ponsel dan Mac, gerakan dan lokasi di latar belakang di ponsel, dan di macOS juga Accessibility, Screen Recording, Input Monitoring, Full Disk Access, App Management, Developer Tools, dan Automation aplikasi lain ([lokasi dan gerakan](services.md#lokasi-dan-gerakan), [notifikasi](notifications.md)). `core.Permissions()` mencantumkannya sebagai kunci `core.Permission`. `cx.PermissionStatus` menjawab apakah aplikasi memiliki suatu izin tanpa bertanya, dan `cx.RequestPermission` meminta dengan prompt sistem jika ada dan belum pernah ditanyakan. Mintalah saat pengguna melakukan sesuatu yang membutuhkan izin tersebut. Face ID dan Touch ID tidak termasuk di dalamnya: `cx.BiometricAvailability` memberitahukannya dan `cx.AuthenticateBiometric` meminta dengan prompt miliknya sendiri ([biometrik](services.md#biometrik)).

```go
cx.RequestPermission(core.PermissionCamera, func(s core.PermissionStatus) {
	if s.Allowed() {
		startScanner()
	}
})
```

Di macOS, Automation diberikan per aplikasi target: berikan `core.PermissionAutomationOf("com.apple.finder")` untuk memeriksa atau meminta izin bagi aplikasi target yang sedang berjalan berdasarkan bundle ID-nya, atau `core.PermissionAutomation` untuk membuka panel Automation di System Settings. Aplikasi target yang tidak sedang berjalan menjawab `PermissionUnknown`.

| Status | `String()` | Arti |
| --- | --- | --- |
| `PermissionNotAsked` | `prompt` | Belum ditanyakan: permintaan menampilkan prompt sistem |
| `PermissionGranted` | `granted` | Aplikasi memilikinya |
| `PermissionLimited` | `limited` | Akses sebagian, seperti foto atau kontak yang dipilih |
| `PermissionDenied` | `denied` | Ditolak; hanya pengaturan sistem yang dapat mengubahnya |
| `PermissionUnknown` | `unknown` | Dibatasi, tetapi sistem atau platform tidak memberitahukannya |
| `PermissionNotGated` | `not-gated` | Tidak ada yang membatasinya dari aplikasi, seperti di Linux |
| `PermissionUnavailable` | `unavailable` | Platform tidak memiliki hal semacam itu |

`Allowed` bernilai true untuk granted, limited, dan not gated. macOS menjawab Accessibility, Screen Recording, dan Input Monitoring sebagai denied sampai pengguna mengaktifkannya, karena meminta hanya mencantumkan aplikasi di panelnya, dan memberitahukan pemberian Screen Recording setelah aplikasi dijalankan ulang.

## Deskripsi penggunaan

Di macOS dan iOS, meminta izin membutuhkan alasan izin tersebut di Info.plist aplikasi, yang disimpan di `apple.info` milik `nsgo.yaml`; tanpanya permintaan menjawab unavailable dan mencatat kuncinya ke log, karena sistem akan menghentikan aplikasi. Pesan di [katalog](localization.md) aplikasi dengan kunci yang sama, seperti `NSCameraUsageDescription: Scans the codes you point it at.` di `locales/en.yaml`, menerjemahkan alasannya, yang kemudian ditanyakan sistem dalam bahasa pengguna; bahasa yang tidak menerjemahkannya memakai teks sumber.

```yaml
apple:
  info:
    NSMicrophoneUsageDescription: Records your voice notes.
    NSCameraUsageDescription: Scans the codes you point it at.
    NSContactsUsageDescription: Finds friends who already use the app.
    NSCalendarsFullAccessUsageDescription: Adds your bookings to your calendar.  # macOS 14 and iOS 17; NSCalendarsUsageDescription before
    NSRemindersFullAccessUsageDescription: Turns tasks into reminders.            # NSRemindersUsageDescription before
    NSPhotoLibraryUsageDescription: Imports photos from your library.
    NSBluetoothAlwaysUsageDescription: Finds your headphones.
    NSAppleMusicUsageDescription: Plays your library.                            # iOS
    NSAppleEventsUsageDescription: Reveals files in Finder.                      # macOS Automation
android:
  permissions:
    - RECORD_AUDIO
    - CAMERA
    - READ_CALENDAR
    - WRITE_CALENDAR
    - BLUETOOTH_CONNECT      # Android 12+; BLUETOOTH on Android 9–11
    - READ_MEDIA_AUDIO       # Android 13+; READ_EXTERNAL_STORAGE on Android 9–12
windows:
  device_capabilities: [microphone, webcam]  # what a store build's MSIX declares
```

Di Android, setiap izin harus dicantumkan di `android.permissions` milik `nsgo.yaml`; izin yang tidak ada di manifest menjawab `PermissionUnavailable`. Di bawah `nsgo dev`, macOS membebankan apa yang dilakukan aplikasi kepada terminal yang memulainya, sehingga terminal itulah aplikasi yang ditanya dan dicantumkan; build aplikasi untuk menguji izinnya sendiri.

## Membuka pengaturan

`cx.OpenPermissionSettings` membuka halaman yang memberikan suatu izin dan menjawab apakah halaman tersebut terbuka: Privacy & Security pada panel izin itu (termasuk Automation, Contacts, Photos, dan Location Services) atau Notifications di macOS, halaman aplikasi di Settings (atau halaman Notifications-nya sejak iOS 16) di iOS, halaman aplikasi di Settings di Android, dan halaman mikrofon, kamera, atau notifikasi di Windows.

Untuk `PermissionLocation` dan `PermissionBackgroundLocation`, macOS membuka Location Services; `PermissionMotion` membuka Motion & Fitness, meskipun macOS tidak menyediakan sampel gerakan. iOS dan Android membuka halaman aplikasi untuk ketiganya. Membuka pengaturan tidak meminta maupun memberikan izin.

## Memandu pengguna

`desktop.GuidePermission` membuka halaman tersebut dan menunggu pengguna memberikan izin. Di macOS, sebuah panel mengikuti jendela System Settings tanpa mengambil fokus dan, untuk panel yang daftarnya menerima aplikasi yang diseret ke dalamnya, menyodorkan aplikasi agar pengguna menyeretnya ke sana. Panduan berakhir, dan fungsinya menerima status, begitu aplikasi memperoleh izin, pengguna menutup panel atau System Settings, `stop` dipanggil, atau komponennya hilang; panduan baru mengakhiri panduan sebelumnya.

```go
stop := desktop.GuidePermission(cx, core.PermissionAccessibility, desktop.GuideOptions{}, func(s core.PermissionStatus) {
	trusted.Set(s == core.PermissionGranted)
})
```

Aplikasi yang disodorkan adalah bundle-nya, atau jika tidak ada, executable-nya; `GuideOptions.App` menentukan aplikasi lain dan `Name` menentukan nama yang dipakai panel untuknya. Windows membuka halaman dan menunggu Settings ditutup. Di tempat lain, fungsi langsung menerima status.

## Platform

| Platform | Apa yang diberitahukan dan diminta |
| --- | --- |
| macOS | Setiap izin (Automation per aplikasi target yang berjalan melalui `PermissionAutomationOf`, ditambah notifikasi untuk aplikasi dalam bundle); Automation tanpa target, App Management, Developer Tools, dan Media & Apple Music bernilai unknown; gerakan bernilai unavailable |
| iOS | Mikrofon, kamera, kontak, kalender, pengingat, foto (dengan `PermissionLimited`), Bluetooth, pustaka media, lokasi, gerakan, dan notifikasi |
| Windows | Mikrofon, kamera, dan notifikasi, yang tidak pernah dimintakan lewat prompt; sisanya tidak dibatasi |
| Linux | Tidak dibatasi di luar sandbox; notifikasi mencerminkan apakah server notifikasi menjawab; di Flatpak dan Snap, kamera melalui portal desktop dan mikrofon melalui portal dan PipeWire |
| Web | Mikrofon dan kamera (unknown di Firefox), notifikasi, lokasi (bukan lokasi latar belakang), dan gerakan di browser seluler |
| Android | Mikrofon, kamera, kalender, Bluetooth, dan pustaka media, jika dideklarasikan di `android.permissions` |
| Apple Watch, mini program | Notifikasi, lokasi, dan gerakan melalui layanannya; sisanya yang dibatasi oleh sistem untuk saat ini bernilai unknown |

Izin lokasi dan gerakan mengikuti [sensor masing-masing platform](services.md#lokasi-dan-gerakan), dan `PermissionNotifications` mengikuti [notifikasi](notifications.md). Kebugaran memakai `PermissionActivityRecognition` (`ACTIVITY_RECOGNITION` di Android, `NSMotionUsageDescription` di Apple), termasuk tekanan udara Apple; gerakan mentah Android tetap terpisah. Di Safari seluler, mintalah `PermissionMotion` dari handler ketukan.

## Pengujian

`nsgotest` menjawab dari modelnya: `app.Permissions` menyimpan setiap status, awalnya belum ditanyakan, dan permintaan atas izin yang belum ditanyakan menjadikannya nilai `app.PermissionAnswers` untuk izin itu, atau granted jika tidak ada. `PermissionNotifications` tetap selaras dengan `app.NotificationPermission` dan `app.NotificationAnswer`. `app.PermissionSettings` mencantumkan pengaturan yang dibuka dan `app.Guide` panduan yang sedang tampil, yang diakhiri oleh `app.GrantPermission` dan `app.CloseGuide`.

```go
app.PermissionAnswers[core.PermissionMicrophone] = core.PermissionDenied
app.Press("id:record")
app.Find("text:The microphone is off")
```

## Berikutnya

- [Notifikasi](notifications.md): izinnya sendiri, badge, dan ketukan.
- [Membangun dan mendistribusikan](building.md): entitlement dan kapabilitas yang dideklarasikan sebuah paket.

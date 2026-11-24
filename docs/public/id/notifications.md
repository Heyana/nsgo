---
title: Notifikasi
description: Minta izin menampilkan notifikasi, tampilkan, jadwalkan, dan tutup notifikasi, beri angka dan tampilkan progres tugas pada ikon aplikasi, serta tangkap ketukan.
group: Fitur platform
order: 45
---

## Izin

`cx.RequestNotificationPermission` meminta pengguna mengizinkan aplikasi menampilkan notifikasi, dengan prompt sistem pada kali pertama, dan menjawab dengan `core.NotificationPermission`: `NotificationsGranted`, `NotificationsDenied`, atau `NotificationsUnavailable` bila platform atau build tidak menampilkan notifikasi sama sekali. Mintalah saat pengguna melakukan sesuatu yang membutuhkan notifikasi: iOS dan Android hanya bertanya sekali, dan browser menolak prompt yang tidak dipicu oleh pengguna. `cx.NotificationPermission` membaca jawabannya tanpa bertanya.

```go
cx.RequestNotificationPermission(func(p core.NotificationPermission) {
	if p == core.NotificationsGranted {
		cx.ShowNotification(core.Notification{ID: "chat:42", Title: "Ada", Body: "Lunch?"}, nil)
	}
})
```

## Menampilkan notifikasi

`cx.ShowNotification` menampilkan notifikasi, juga saat aplikasi berada di depan, dan menggantikan notifikasi dengan `ID` yang sama; `Silent` menghilangkan suara dan getaran sistem. Fungsi ini menjawab `done` dengan apakah sistem menerima notifikasi tersebut, false tanpa izin, dan mengembalikan `core.ErrNotification`, tanpa memanggil `done`, untuk notifikasi tanpa ID atau judul. `cx.CloseNotification` menghapus notifikasi yang dinamai oleh suatu ID.

```go
err := cx.ShowNotification(core.Notification{ID: "sync", Title: "Synced", Body: "12 notes"}, func(shown bool) {})
cx.CloseNotification("sync")
```

`cx.CloseNotifications` menghapus setiap notifikasi yang ditampilkan aplikasi dan membatalkan yang dijadwalkannya. Di macOS, iOS, watchOS, Windows, dan Android, ini mencakup notifikasi dari run sebelumnya yang tertinggal di Notification Center, Action Center, atau panel notifikasi; di Linux dan web, ini hanya menjangkau notifikasi yang ditampilkan atau dijadwalkan oleh run ini. Panggil saat aplikasi kembali ke depan ketika jendelanya menampilkan apa yang diberitahukan notifikasi tersebut:

```go
cx.Effect(func() {
	if cx.Env().Lifecycle() == core.LifecycleActive {
		cx.CloseNotifications()
		cx.SetBadge(0)
	}
})
```

## Menjadwalkan notifikasi

Notifikasi yang `At`-nya lebih lambat dari sekarang akan dijadwalkan, dan memanggil `cx.ShowNotification` lagi dengan `ID`-nya akan memindahkannya, sedangkan `cx.CloseNotification` membatalkannya sebelum ditampilkan. `Repeat` menampilkannya setiap hari pada jam `At`, atau juga setiap minggu pada hari yang sama, mulai dari waktu berikutnya yang sesuai, dalam zona waktu perangkat. `cx.Env().CanScheduleNotifications()` memberi tahu apakah platform menjadwalkannya: aplikasi macOS, iOS, dan watchOS dalam bundle, Android, dan Windows menjadwalkannya secara native baik aplikasi berjalan maupun tidak (dengan Windows merangkai pengulangan harian dan mingguan selama aplikasi berjalan dan memasangnya kembali saat diluncurkan), sedangkan Linux dan web (tempat `Notification` tersedia) menyimpannya selama aplikasi atau halaman berjalan. Di tempat lain (tvOS, yang pusat notifikasinya hanya menampilkan badge, dan mini program) `done` menjawab false.

```go
cx.ShowNotification(core.Notification{ID: "review", Title: "Cards to review", At: due}, nil)
cx.ShowNotification(core.Notification{ID: "habit", Title: "Practice",
	At: time.Date(2026, 1, 1, 9, 0, 0, 0, time.Local), Repeat: core.NotificationDaily}, nil)
```

Android dapat menampilkan notifikasi terjadwal beberapa menit setelah waktunya saat ponsel dalam mode Doze; aplikasi yang membutuhkannya tepat pada menitnya, seperti jam alarm, mencantumkan `android.permission.SCHEDULE_EXACT_ALARM` di `android.permissions`, yang diberikan pengguna di pengaturan sistem sejak Android 12, atau `USE_EXACT_ALARM`, yang hanya diizinkan Google Play untuk aplikasi alarm dan kalender. Notifikasi aplikasi Android yang dihentikan paksa menunggu hingga aplikasi itu dijalankan lagi. Untuk menguji pengiriman setelah proses mati, tunggu `ShowNotification` menjawab true, lalu matikan prosesnya tanpa menghentikan paket secara paksa.

## Ketukan

`cx.OnNotificationPress` menjalankan fungsinya dengan `ID` notifikasi yang diketuk pengguna, selama komponen yang memasangnya masih hidup, dan membawa aplikasi ke depan. Ketukan yang memulai aplikasi sampai ke handler pertama yang terdaftar. Simpan di `ID` apa yang dibuka oleh ketukan (`"chat:42"`), karena ketukan bisa memulai aplikasi dari awal. Di Windows, hal ini berlaku untuk aplikasi yang dimasukkan installer-nya ke menu Start; selain itu, satu run hanya menangkap ketukan selama berjalan.

```go
cx.OnNotificationPress(func(id string) { openChat(strings.TrimPrefix(id, "chat:")) })
```

Aplikasi Android yang meminta izin sejak Android 13 mencantumkan `android.permission.POST_NOTIFICATIONS` di `android.permissions` milik `nsgo.yaml`.

## Badge

`cx.SetBadge` menaruh angka pada ikon aplikasi: ikon di Dock pada macOS, di Layar Utama pada iOS, yang membutuhkan izin notifikasi, tombol taskbar pada Windows, launcher Linux yang kompatibel dengan Unity, dan ikon aplikasi web yang terpasang. Angka 0 atau kurang akan menghapusnya. Aplikasi macOS tidak memerlukan izin untuk badge Dock-nya sampai ia memeriksa atau meminta izin notifikasi atau menampilkan notifikasi; sejak saat itu, pada run tersebut dan run berikutnya, Dock hanya menampilkan badge selama pengguna mengizinkan badge aplikasi di System Settings, yang diminta oleh `cx.RequestNotificationPermission` bersama dengan peringatan. Launcher tanpa antarmuka badge Linux dan Android tidak memiliki angka untuk diatur; launcher Android menandai aplikasi yang notifikasinya sedang tampil.

```go
cx.SetBadge(len(unread.Get()))
```

## Progres tugas

`cx.SetTaskProgress` menampilkan sejauh mana tugas yang panjang telah berjalan pada ikon aplikasi: bilah di atas ubin Dock pada macOS, tombol taskbar pada Windows, dan launcher Linux yang kompatibel dengan Unity. Fungsi ini tidak memerlukan izin dan tampil di samping badge. `TaskProgressNormal` menampilkan `Value`, pecahan dari 0 sampai 1; `TaskProgressPaused` dan `TaskProgressError` mewarnainya di Windows dan macOS; `TaskProgressIndeterminate` bergerak di Windows dan berupa bilah diam yang terisi setengah di macOS dan Linux; `TaskProgress` bernilai nol menghapusnya. Di Windows, progres tampil pada tombol jendela milik scope, dan progres yang diatur di tempat yang tidak memiliki tombol jendela sendiri, seperti scope aplikasi itu sendiri atau panel tray, tampil pada setiap jendela aplikasi yang tidak mengatur progresnya sendiri. iOS, Android, dan web tidak menampilkan apa pun.

```go
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressIndeterminate}) // no fraction yet
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressNormal, Value: .43})
cx.SetTaskProgress(core.TaskProgress{}) // done
```

## Berikutnya

- [Izin](permissions.md): izin sistem lainnya, seperti mikrofon dan Accessibility.
- [Haptik](haptics.md): umpan balik yang diputar perangkat di bawah jari pengguna.
- [Layanan platform](services.md): peringatan, clipboard, file, penyimpanan, dan jaringan.
- [Live Activities](live-activities.md): apa yang diikuti aplikasi, di Layar Terkunci dan di Dynamic Island.
- [Membangun dan mendistribusikan](building.md): izin yang dibutuhkan sebuah paket.

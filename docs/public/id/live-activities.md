---
title: Live Activities
description: Ikuti sesuatu saat sedang berlangsung di Lock Screen dan di Dynamic Island, serta sebagai Live Updates di Android.
group: Fitur platform
order: 48
---

## Mengaktifkannya

Live Activity mengikuti sesuatu saat sedang berlangsung, seperti pengiriman atau timer: di Lock Screen dan di Dynamic Island pada iPhone mulai iOS 16.1, dan sebagai Live Update di Android 16, di bagian atas panel notifikasi dan dalam sebuah chip di bilah status; Android versi sebelumnya menampilkan notifikasi yang sedang berlangsung. Aktifkan di `nsgo.yaml`:

```yaml
live_activities:
  enabled: true
  # frequent_updates: true   # iOS: more updates than the system's budget, as the user allows
```

## Memulai, memperbarui, dan mengakhiri

`widget.StartActivity` memulai sebuah activity dari suatu jenis, yang menamai apa yang diikutinya dan ditulis seperti jenis widget, dengan menampilkan sebuah `widget.ActivityContent`. `Activity.Update` mengganti apa yang ditampilkannya, `Activity.Alert` juga melakukannya sekaligus memberi peringatan kepada pengguna, dan `Activity.End` mengakhirinya, dengan konten akhir jika tidak nil, lalu meninggalkan Lock Screen sesuai yang dinyatakan `Dismissal`. `widget.Activities` mencantumkan activity aplikasi yang sedang berjalan, termasuk dari aplikasi yang dijalankan sebelumnya, dan `widget.ActivitiesEnabled` menyatakan apakah pengguna dan platform mengizinkannya.

```go
a, err := widget.StartActivity("delivery", widget.ActivityContent{
	Title: "Your order", Body: "On its way", Symbol: "box.truck.fill",
	Timer: arrival, ShowProgress: true, Progress: 0.4,
	Actions: []widget.ActivityAction{{ID: "pause", Title: "Pause", Symbol: "pause.fill"}},
})
if err != nil {
	return
}
a.Update(widget.ActivityContent{Title: "Your order", Body: "Arrived", Status: "Now"})
a.End(nil, widget.DismissAt(time.Now().Add(2*time.Hour)))
```

`StartActivity` melaporkan `widget.ErrNoActivities` pada aplikasi yang dibangun tanpa fitur ini, `widget.ErrActivitiesUnsupported` di luar iOS 16.1 dan Android, dan `widget.ErrActivitiesDisabled` ketika pengguna menonaktifkannya; `Update` dan `End` melaporkan `widget.ErrActivityEnded` setelah activity berakhir. Konten sebuah activity paling besar 4 KB, sesuai batas yang diterima ActivityKit dan APNs.

## Apa yang ditampilkannya

Kedua platform menampilkan judul dan isi, status atau timer yang dijalankan sistem, bilah progres, hingga tiga aksi, dan warna aksen. iOS menggambarnya dengan ekstensi widget, dan menerima tampilan dari `widget` sebagai gantinya untuk Lock Screen dan setiap bagian Dynamic Island: `LockScreen`, `Leading`, `Trailing`, `Center`, dan `Bottom` dalam keadaan diperluas, `CompactLeading` dan `CompactTrailing` di island ringkas, dan `Minimal` ketika beberapa activity berbagi island tersebut. Android hanya menampilkan field-field tersebut, dengan ikon aplikasi, karena Live Updates tidak menerima tampilan dari aplikasi.

```go
widget.ActivityContent{
	Title: "Order 412", Body: "On its way", Status: "8 min", Symbol: "box.truck.fill",
	Tint: widget.RGB(0x30d158), ShowProgress: true, Progress: 0.4,
	Trailing: widget.Text{Content: "8 min", Style: widget.Style{FontSize: 34, FontWeight: core.FontWeightBold}},
}
```

`URL` dibuka bersama aplikasi saat activity diketuk, `Stale` adalah waktu ketika iOS menampilkan konten sebagai usang, dan `Background` mengisi activity di Lock Screen.

## Penekanan

`widget.OnAction` menjalankan handler untuk penekanan sebuah aksi atau sebuah `widget.Button`, dengan activity dan ID aksinya (`Activity` bernilai nil untuk [tombol widget](widgets.md#tombol)). Atur di `main`, sebelum `nsgo.Run`: di iOS sistem memulai aplikasi di latar belakang untuk sebuah penekanan, tanpa tampilannya, dan menahan activity sampai handler kembali, sehingga handler memperbaruinya di sana; di Android penekanan mencapai proses Go yang sedang berjalan melalui receiver runtime tanpa membuka Activity-nya. Aksi memerlukan engine yang mempostingnya agar tetap berjalan; aksi yang usang tidak melakukan apa pun setelah engine itu berakhir. Notifikasinya sendiri tetap membuka aplikasi saat diketuk. Handler berjalan di goroutine milik sistem, bukan milik tampilan aplikasi, yang dijangkaunya melalui `Post` milik sebuah `core.Scope`.

```go
func main() {
	widget.OnAction(func(p widget.ActivityPress) {
		if p.Action == "pause" {
			p.Activity.Update(paused)
		}
	})
	nsgo.Run(Root)
}
```

Tampilan milik activity juga dapat melakukannya, di iOS 17 dan yang lebih baru: `widget.Button{Action: "pause"}` menjalankan handler yang sama tanpa membuka aplikasi, dan hanya menampilkan turunannya di tempat penekanan hanya membuka aplikasi.

## Pembaruan push

Aplikasi di latar belakang hanya memperbarui activity-nya selama aplikasi berjalan, jadi aplikasi yang mengikuti apa yang terjadi di tempat lain memulainya dengan `widget.ActivityOptions{Push: true}`, mengirim token yang diterima `widget.OnPushToken` ke servernya, dan meminta server mengirim pembaruan APNs `liveactivity` yang `content-state`-nya adalah jawaban `widget.ActivityState` untuk konten tersebut: tidak ada gambar yang masuk ke dalam push. Mulai iOS 17.2 aplikasi juga mendapatkan token untuk memulai activity melalui push, dengan tipe atribut `widget.ActivityAttributesType` dan atribut berupa jenis dan id. Push memerlukan entitlement push notifications di profil aplikasi.

```go
state, err := widget.ActivityState(content)  // the push's content-state
```

## Batasan

Sistem mengakhiri sebuah activity setelah delapan jam. `widget.DismissDefault` membiarkan activity yang telah berakhir tetap di Lock Screen hingga empat jam di iOS dan langsung menghapusnya di Android, `widget.DismissNow` langsung menghapusnya, dan `widget.DismissAt` menghapusnya pada waktu tertentu dalam empat jam. Android meminta `POST_NOTIFICATIONS` dan `POST_PROMOTED_NOTIFICATIONS` sendiri: mintalah izin pengguna dengan `cx.RequestNotificationPermission` terlebih dahulu ([Notifikasi](notifications.md)). Demo Widgets di galeri dan `examples/widgets` menjalankan salah satunya.

## Berikutnya

- [Widget](widgets.md): timeline Home Screen dan Lock Screen, digambar oleh ekstensi yang sama.
- [Notifikasi](notifications.md): izin yang diminta Android terlebih dahulu.
- [Membangun dan merilis](building.md): ekstensi dan entitlement yang dibutuhkannya.

---
title: Widget
description: Tampilkan view aplikasi di Home Screen dan Lock Screen iPhone dan iPad, serta di desktop Mac.
group: Fitur platform
order: 47
---

## Sebuah jenis widget

Widget menampilkan sekilas isi aplikasi, baik aplikasi sedang berjalan maupun tidak: di Home Screen dan Lock Screen iPhone dan iPad, di desktop Mac dan di Notification Center, serta di launcher Android. Sebuah jenis (kind) adalah satu widget yang ditawarkan galeri sistem, beserta keluarga ukuran yang tersedia:

```yaml
widgets:
  - kind: counter
    name: Counter                 # shown in the system's gallery of widgets
    description: Shows the count.
    families: [small, medium, circular]  # also large, extra_large, rectangular and inline
# apple: { app_group: group.dev.example.hello }  # the shared App Group, group.<id> by default
```

Di platform Apple, sebuah ekstensi widget menggambarnya, yang dikompilasi oleh `build` dan `dev` dengan compiler Swift milik Xcode ([Membangun dan mendistribusikan](building.md#widgets-and-live-activities)). Di Android, runtime Java yang sudah dibangun sebelumnya menggambar timeline yang sama sebagai App Widgets. Keduanya tetap menampilkan konten yang telah diterbitkan ketika aplikasi berhenti.

Untuk build Mac App Store yang di-sandbox, atur `apple.app_group` dan sediakan provisioning profile macOS melalui `sign.macos_profile` dan `sign.widgets_profile`. Aplikasi dan `<id>.widgets` harus berada di tim yang sama dan memiliki entitlement untuk grup tersebut. Timeline dan gambar disimpan di kontainer bersamanya; build macOS langsung dan build pengembangan tetap menggunakan lokasi di direktori home yang sudah ada. [Penandatanganan untuk store](building.md#store-packages) menjelaskan opsi build-nya.

`name` dan `description` dapat menamai pesan di katalog aplikasi. Jika demikian, galeri widget dan placeholder menggunakan bahasa pengguna, dengan kembali ke teks katalog sumber ketika terjemahan tidak tersedia. Nilai yang bukan kunci katalog ditampilkan apa adanya.

## Sebuah timeline

`widget.Update` menulis apa yang ditampilkan widget dari suatu jenis mulai sekarang, sebagai timeline berisi entri: sistem menampilkan entri yang tanggalnya paling akhir terlewati, lalu setiap entri berikutnya pada tanggalnya, tanpa aplikasi. `Date` bernilai nol berarti sekarang. Tanpa entri, widget menampilkan nama dan deskripsi jenisnya. Di platform Apple, aplikasi di latar depan dapat memperbarui widget-nya sesering yang diinginkan, sedangkan aplikasi di latar belakang beberapa puluh kali sehari.

```go
err := widget.Update("counter", widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Count", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Text{Content: strconv.Itoa(n), Style: widget.Style{FontSize: 44, FontWeight: core.FontWeightBold}},
	}},
})
```

`Entry.Families` menampilkan view tersendiri untuk sebuah keluarga sebagai pengganti `Entry.View`, `Entry.Background` mengisi widget di belakangnya hingga melewati margin yang dijaga sistem, dan `Entry.URL` dibuka bersama aplikasi ketika widget diketuk. `widget.Update` melaporkan `widget.ErrNoWidgets` pada aplikasi yang dibangun tanpa widget, `widget.ErrUnsupported` di luar macOS, iOS, dan Android, dan `widget.Available` memberi tahu mana yang berlaku sebelum menulis. Fungsi ini dapat berjalan di goroutine mana pun, termasuk sebelum `nsgo.Run`.

## View

View milik widget bukan elemen `core`: view tersebut ditata seperti SwiftUI, yaitu sebuah view sebesar kontennya kecuali `Style` mengatur ukurannya atau membuatnya mengisi ruang, dan sebuah kontainer memusatkan isinya kecuali `AlignItems` dan `Justify` menentukan lain. `Text`, `Symbol`, dan `Image`, dari byte atau dari file yang dibaca `Update`, menampilkan konten; `Column`, `Row`, `Stack`, `Spacer`, dan `Divider` mengaturnya; `Rectangle`, `Progress`, dan `Gauge` menggambar; dan `Date` menampilkan waktu dengan format milik sistem, yang terus dihitung sendiri oleh `Relative`, `Offset`, dan `Timer`, tanpa perlu entri untuk setiap detik.

```go
widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Next hour in", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Date{Time: hour, Format: widget.Timer, Style: widget.Style{TextStyle: widget.Title2, FontVariant: core.FontTabularNums}},
		widget.Progress{Value: 0.4, Style: widget.Style{Color: widget.Accent}},
	}},
}
```

`Style` menerima gap, padding, ukuran atau pengisian, perataan, latar belakang dengan sudut membulat, gaya teks milik sistem atau ukuran font, ketebalan, desain, dan varian, batas jumlah baris, `Link` yang dibuka bersama aplikasi, dan `Accented`, yang menempatkan sebuah view di antara view yang diwarnai sistem. `widget.Primary`, `Secondary`, dan `Accent` mengikuti tampilan (appearance), sedangkan `RGB` dan `RGBA` menerima warna lain untuk mode gelap dengan `Dark`.

## Tombol

`widget.Button` menjalankan `widget.OnAction` mulai iOS 17 dan macOS 14, tanpa membuka aplikasi. Sistem yang lebih lama hanya menampilkan anak-anaknya. Daftarkan handler di `main`, sebelum `nsgo.Run`: sistem dapat memulai aplikasi di latar belakang tanpa view-nya. Tekanan pada widget memiliki `Activity == nil`; `Action`-nya adalah nilai tombol tersebut.

```go
widget.Button{Action: "refresh", Children: []widget.View{
    widget.Text{Content: "Refresh"},
}}
```

Tangani aksi tersebut dan panggil `widget.Update` sebelum handler kembali, agar pemuatan ulang timeline otomatis oleh sistem melihat hasilnya. Handler berjalan di luar goroutine view; gunakan `core.Scope.Post` untuk mengubah view. Ini menggunakan handler dan intent yang sama dengan [Live Activities](live-activities.md), dan hanya membutuhkan `widgets` di `nsgo.yaml`, tanpa `live_activities`.

## Android

Tambahkan widget dari pemilih widget di launcher dan ubah ukurannya di sana. Renderer memilih keluarga yang diizinkan berdasarkan batasnya: small di bawah 240 poin pada kedua dimensi, medium mulai 240 secara melintang, large mulai 240 secara menurun, dan extra-large mulai 500 secara melintang; jika keluarga itu tidak tercantum, ia menggunakan keluarga pertama yang tercantum. `Entry.URL` dan `Style.Link` membuka aplikasi melalui skema yang dideklarasikannya di `url_schemes`. Tautan pada view bersarang didahulukan daripada tautan kontainernya.

Entri timeline mendatang menggunakan alarm tidak eksak milik Android, yang dapat ditunda sistem saat aplikasi berada di latar belakang. View timer menggunakan kronometer native dan berdetak tanpa aplikasi; saat ini view tersebut mendukung ukuran dan warna, sementara ketebalan font kustom, desain, dan gaya miring masih menjadi target. Tanggal relatif disegarkan sesuai jadwal sistem. Renderer generik mendukung teks, gambar, tata letak, bentuk, bilah, cincin, dan sekumpulan glyph simbol umum; gunakan `Image` untuk karya seni yang harus sama di kedua platform. Nama SF Symbol sembarang masih menjadi target terbuka di Android. Provider widget Android dihasilkan untuk APK dan AAB mandiri; host tertanam tidak menerimanya.

## Live Activities

Ekstensi yang sama menggambar Live Activities milik aplikasi di Lock Screen dan di Dynamic Island ([Live Activities](live-activities.md)).

## Selanjutnya

- [Live Activities](live-activities.md): apa yang diikuti aplikasi saat terjadi.
- [Notifikasi](notifications.md): notifikasi dan badge di samping widget.
- [Membangun dan mendistribusikan](building.md): grup aplikasi yang dibagikan ekstensi.

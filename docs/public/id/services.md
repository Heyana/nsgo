---
title: Layanan platform
description: Peringatan, papan klip, file, foto, berbagi, penyimpanan, rahasia, ucapan, pembacaan teks, jaringan, masuk akun, pembelian dalam aplikasi, dan siklus hidup aplikasi.
group: Fitur platform
order: 44
---

## Peringatan dan papan klip

`cx.ShowAlert` menampilkan peringatan (*alert*) atau lembar aksi (*action sheet*) milik platform. `Actions`-nya dikembalikan berdasarkan indeks, dan sebuah peran menggambar tombol batal dan destruktif sesuai cara platform. Di macOS, peringatan dan pemilih file menggunakan *sheet* selama jendelanya terlihat. Jendela yang tersembunyi, diminimalkan, atau berada di luar layar, atau aplikasi baki (*tray*) tanpa jendela, membuka dialog secara terpisah. Menutup jendela yang meminta akan membatalkan dialog-dialognya.

```go
cx.ShowAlert(core.AlertConfig{
	Title:   "Delete the file?",
	Message: "This cannot be undone.",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(index int) {
	if index == 1 {
		remove()
	}
})
```

`SetClipboard` menulis teks dan `SetClipboardImage` menulis gambar. `ReadClipboard`, `ReadClipboardImage`, dan `ReadClipboardFiles` menjawab secara asinkron, sedangkan `HasClipboardText` dan `HasClipboardImage` membaca isi papan klip tanpa pemberitahuan tempel. `core.FileFromURL` membuka URL yang diberikan oleh `ReadClipboardFiles` dan `View.OnFileDrop` sebagai `core.File`: URL `file:`, dan di web URL `blob:`.

tvOS tidak memiliki papan tempel (*pasteboard*) publik. Penulisan ke papan klip diabaikan, pembacaan tidak mengembalikan konten, dan pemeriksaan keberadaan mengembalikan false.

```go
cx.ReadClipboard(func(text string) { pasted.Set(text) })
cx.SetClipboard("Hello")
```

Di macOS dan Windows, `ReadClipboardItems` memberikan cuplikan (*snapshot*) seluruh representasi byte yang tersedia hingga 48 MiB. Setiap item membawa nilai `ClipboardRepresentation` yang berurutan dengan `Type` native dan `Data` mentahnya, termasuk format yang hanya menandakan keberadaan dengan data kosong. `ClipboardFormats` mencantumkan nama format native tanpa menyalin muatannya. Untuk memulihkan cuplikan, berikan ke `WriteClipboardItems` bersama callback opsional yang menerima galat penulisan. Slice kosong akan mengosongkan papan klip. Bandingkan kesamaan nilai dari `ClipboardChangeCount` untuk mendeteksi salinan baru, termasuk saat teks yang disalin identik.

macOS mempertahankan batas item dan nama tipe pasteboard. Windows menggunakan satu item dan mempertahankan nama Win32 berbasis byte seperti `CF_HDROP` serta format terdaftar seperti `Rich Text Format`. Windows menggabungkan item dengan format berbeda saat menulis dan menggunakan satu byte nol untuk muatan kosong. Handle GDI, format *owner-display*, dan ID format numerik privat dihilangkan. Di platform lain, callback baca dan tulis menerima `CodeUnavailable` dan callback hitungan menerima `-1`.

## Membuka tautan dan file

`cx.OpenURL` membuka URL di peramban atau aplikasi sistem, dan `core.CheckURL` menyebutkan apa yang ditolaknya: tanpa skema, lebih dari 16 KiB, berisi spasi, alamat web tanpa host, serta `javascript:`, `data:`, `blob:`, `file:`, `about:`, dan `asset:`.

```go
if err := core.CheckURL(url); err == nil {
	cx.OpenURL(url, func(opened bool) {
		if !opened {
			status.Set("No app opens that link.")
		}
	})
}
```

`cx.OpenFile` membuka file milik aplikasi di aplikasi yang terkait dengan tipenya, dan `cx.Share` menampilkan lembar berbagi (*share sheet*) sistem untuk file, teks, atau keduanya di macOS, iOS, Android, Windows, Linux (melalui portal desktop atau pengelola file), dan web (`navigator.share`). Sebuah `core.SharedFile` adalah path di disk — atau, untuk `Share` di web dan di mana pun file disimpan di memori, byte `Data`-nya — dengan nama dan tipe yang dilihat aplikasi lain. Di desktop, `desktop.RevealFile` menampilkan file atau folder dalam keadaan terpilih di Finder, Explorer, atau pengelola file, dan `desktop.TrashFile` memindahkannya ke Trash atau Recycle Bin; fungsi ini tidak pernah menghapus secara permanen, melainkan menjawab `core.ErrNoTrash` dan membiarkan item di tempatnya jika volume tidak memiliki tempat sampah atau tempat sampah tidak mau menyimpannya, seperti file yang lebih besar daripada Recycle Bin.

```go
cx.Share(core.ShareOptions{
	Files: []core.SharedFile{{Path: path, Name: "Notes.txt"}},
	Text:  "A note from NSGo",
}, func(shown bool, err error) {})
```

`mobile.OnShared(cx, func(items []mobile.SharedItem) { ... })` menerima teks dan file untuk [`android.share_types`](building.md#incoming-shares-on-android) atau [`apple.share_types`](building.md#incoming-shares-on-ios). Kiriman berbagi di Android dapat memulai aplikasi; iOS mengantrekannya di App Group hingga aplikasi aktif atau pengguna membukanya berikutnya. Kiriman yang tertunda menunggu handler pertama. iOS menahan batch kotak masuk hingga callback kembali; pengiriman terjadi setidaknya sekali meskipun proses berhenti. Teks tiba di `Text`, dan file tiba sebagai salinan cache yang dapat dibaca dengan `Path`, `Name`, dan `MIME`. Pindahkan atau salin file ke penyimpanan persisten jika file itu perlu bertahan lebih lama daripada cache.

`desktop.LookUp` menampilkan panel kamus sistem untuk sebuah teks, seperti ketukan tiga jari di macOS, digambar di atas sebuah kotak dalam tampilan: teks dan kotak suatu seleksi beserta `Ref` milik tampilan `Selectable`-nya. `desktop.CanLookUp` menyatakan apakah hal itu bisa dilakukan, di dalam jendela di macOS; Windows, Linux, dan web tidak memiliki panel seperti itu. Dalam pengujian, `nsgotest.App.LookUps` mencantumkan panel yang ditampilkan ketika `nsgotest.Environment` mengatur `CanLookUp`.

```go
var area core.Ref
var selected core.TextSelection
core.View{Ref: &area, Selectable: true, OnTextSelection: func(s core.TextSelection) { selected = s }, Children: subtitles}
// From a menu item or a shortcut:
if desktop.CanLookUp(cx) {
	desktop.LookUp(cx, selected.Text, &area, selected.Bounds)
}
```

`desktop.QuickLook` meminta pratinjau file, dimulai dari sebuah indeks, dan memperbarui daftar file jika dipanggil lagi selama panel terbuka. `desktop.CanQuickLook` melaporkan ketersediaannya di jendela milik aplikasi di macOS dan Windows, serta di Linux ketika sesi menyediakan layanan D-Bus GNOME Sushi yang sedang berjalan atau dapat diaktifkan. Windows menggunakan handler pratinjau yang terdaftar untuk tipe file tersebut; format yang didukung bergantung pada handler atau penampil pratinjau yang terpasang. Hasil true berarti platform telah diminta, bukan berarti platform telah merender file tersebut.

Spasi atau Escape menutup panel. Tombol panah memilih file dalam permintaan di Windows dan di versi Sushi yang melaporkan seleksi dan kepemilikan di X11. Di Wayland, Linux saat ini membuka pratinjau tanpa induk; penutupan otomatis oleh pemilik dan pemilihan dengan tombol panah tidak tersedia. Penampil pratinjau khusus KDE dan web tidak didukung.

```go
if desktop.CanQuickLook(cx) {
	desktop.QuickLook(cx, []string{reportPath, sourcePath}, 0)
}
```

## Ikon menu native

`desktop.MenuItem.Icon` menampilkan ikon di samping item dalam menu aplikasi, Dock, baki, dan popup di macOS, Windows, dan Linux. Sebuah `desktop.MenuIcon` menerima ikon vektor yang telah diurai (`SVG`, seperti `lucide.Folder()`), byte gambar terenkode (`Data`, PNG, JPEG, GIF, WebP, atau SVG), atau `core.ImageSource` lokal/aset/piksel (`Source`). Mengatur `Template` mewarnai gambar monokrom dengan warna teks menu, mengikuti tampilan terang dan gelap (serta status sorotan di macOS); ikon `SVG` monokrom diperlakukan sebagai template secara default.

```go
desktop.SetApplicationMenu(cx, []desktop.MenuItem{{
	Label: "File",
	Items: []desktop.MenuItem{
		{Label: "Open Folder…", Icon: desktop.MenuIcon{SVG: lucide.Folder()}, OnSelect: openFolder},
		{Label: "Export", Icon: desktop.MenuIcon{Data: exportPNG, Template: true}, OnSelect: exportFile},
	},
}})
```

Ikon dimuat secara sinkron dari sumber lokal; URL jarak jauh dan gambar yang tidak valid membuat item tampil tanpa ikon. Label menu, pintasan, dan indikator centang/radio mempertahankan perilaku native-nya.

Dalam pengujian, `nsgotest.MenuItem.Icon` mencatat ikon setiap item di `app.Menu`, `app.DockMenu`, dan `app.PopupMenu`.

## Foto dan file

`cx.PickImages` menampilkan pemilih foto sistem dan menjawab dengan `core.PickedFile`, yang byte, nama, tipe, dan ukuran pikselnya dibaca oleh aplikasi. `cx.TakePhoto` menampilkan kamera jika platform memilikinya; `cx.Env().CanTakePhoto` menyatakan apakah platform memilikinya. Di macOS, Windows, dan Linux, fungsi ini menampilkan pratinjau kamera pemindai dengan tombol Take Photo. `cx.ScanCode` membuka pratinjau kamera dan menjawab dengan kode pertama yang dibacanya, yaitu kode QR kecuali `ScanOptions.Kinds` meminta jenis lain, atau nil saat pengguna menutupnya; `cx.Env().CanScanCode` menyatakan apakah platform memiliki pemindai, yaitu di perangkat macOS, iOS, dan Android yang memiliki kamera, mesin Windows yang memilikinya, desktop Linux dengan portal Camera dan GStreamer `pipewiresrc`, serta peramban dengan `getUserMedia` dan `BarcodeDetector`. macOS menampilkan *sheet* saat jendela yang meminta terlihat, Windows dan Linux menampilkan modal di atas jendela, dan web menampilkan lapisan di atas halaman. Letakkan `NSCameraUsageDescription` di `apple.info` untuk macOS dan iOS, dan `CAMERA` di `android.permissions` untuk Android. Android, Windows, dan Linux membaca semua jenis kecuali PDF417. Perlakukan teks kode sebagai input yang tidak tepercaya.

Di tvOS, `PickImages` menjawab tanpa gambar dan `TakePhoto` mengembalikan `core.ErrNoCamera`; NSGo tidak menyediakan pemilih Continuity Camera.

Dalam pengujian, `nsgotest.App.PickImages`, `nsgotest.App.TakePhoto`, dan `nsgotest.App.ScanCode` menerima catatan `nsgotest.ImagePick` untuk pemilihan dan pengambilan gambar serta catatan `nsgotest.CodeScan` untuk pemindaian, dan menjawab sebagai pengganti UI sistem. Hook ini bekerja secara headless dan di bawah host `nsgo test -target` yang didukung. `nsgotest.Environment(core.Environment{Camera: true, ScanCode: true})` memberikan kamera kepada target yang tidak memilikinya.

```go
cx.PickImages(core.PickOptions{Limit: 1, MaxDimension: 2048}, func(files []core.PickedFile) {
	if len(files) == 1 {
		avatar.Set(files[0].Data)
	}
})
```

`cx.PickFiles` memilih file dengan tipe apa pun, `cx.PickDirectories` memilih folder sebagai `fs.FS`, dan `cx.SaveFile` menyimpan file atau byte melalui dialog simpan sistem. Kedua pemilih menerima `Title`, label `Button`, dan folder `Start`, dan `PickFilesOptions.TypesName` menamai menu tipe, sehingga aplikasi dapat menampilkannya dalam bahasanya sendiri; semua ini adalah petunjuk, yang diikuti oleh macOS, Windows, dan Linux, oleh iOS hanya untuk `Start`, serta sama sekali tidak oleh Android dan web. `cx.PickScreenColor` memilih warna di mana saja pada layar jika `cx.Env().CanPickScreenColor()` bernilai true: macOS, Windows, Linux dengan portal Screenshot desktop, dan peramban dengan `EyeDropper`.

Atur `core.SaveOptions.Start` ke folder absolut yang sudah ada untuk membuka dialog simpan di sana di macOS, Windows, Linux, dan iOS. Dengan petunjuk yang kosong atau tidak dapat digunakan, atau di target lain, sistem yang memilih foldernya.

Atur `core.SaveOptions.Accept` untuk memvalidasi tujuan yang dipilih sebelum menulis. Fungsi ini berjalan di goroutine engine. Mengembalikan galat akan membiarkan tujuan tidak tersentuh. Di macOS, Windows, dan di pemilih GTK dalam proses di Linux, dialog menampilkan alasannya dan memungkinkan pengguna memilih path lain. Membatalkan setelah penolakan melaporkan `saved == false`, path yang ditolak, dan galatnya. Di Android dan di portal desktop Linux, penolakan menyelesaikan penyimpanan dengan hasil tersebut. Android memungkinkan pengguna memilih folder dan menyediakan URI pohon dengan nama file yang diminta sebagai parameter kueri `name`. Android membuat dokumen setelah disetujui dan melaporkan URI dokumen baru saat berhasil. Platform membatalkan penulisan jika pemeriksaan tidak menjawab dalam lima detik. iOS dan HarmonyOS melaporkan galat untuk penyimpanan yang memerlukan pemeriksaan. Web tidak memiliki path tujuan dan menggunakan alur simpan biasanya.

Dalam pengujian headless, `App.PickFiles`, `App.PickDirectories`, dan `App.SaveFile` menerima catatan `nsgotest.FilePick`, `nsgotest.DirectoryPick`, dan `nsgotest.FileSave`. Catatan yang sama ada di `app.FilePicks`, `app.DirectoryPicks`, dan `app.Saves`.

### Aset bawaan

Letakkan file di bawah direktori `resources/` milik aplikasi dan gunakan `cx.ReadAsset("asset:maps/level.bin", done)` untuk membacanya dari Go. Callback menerima `([]byte, error)` di goroutine engine setelah pembacaan dan dibuang jika pemiliknya hilang atau berjalan lagi. Gunakan `errors.Is(err, core.ErrAssetNotFound)` untuk mengenali file yang tidak ada. Pembacaan memuat seluruh file ke memori.

## Menarik audio PCM

Gunakan `audio.NewOutput(cx, audio.OutputConfig{SampleRate: 48000, Channels: 2, Render: render})` untuk memutar PCM Float32 berselang-seling (*interleaved*) yang didekode atau disintesis oleh aplikasi Anda. `Start` mulai menarik sampel, `Pause` menghentikan penarikan, dan cakupan pemilik menutup output saat cakupan itu berakhir. macOS, iOS, Android, Windows, Linux, dan web mendukung stream ini. Target lain mengembalikan `audio.ErrUnsupported`.

`Render` mengembalikan jumlah frame yang diisinya. Fungsi ini berjalan di thread audio dan harus menghindari pemblokiran, alokasi, dan akses graf. Pengisian yang kurang memutar keheningan untuk sisanya dan menambah `Underruns`. `Position` mengembalikan hitungan frame monoton dan waktu host pada frame tersebut. Gunakan `Flush` untuk membuang frame yang tertunda dan `Latency` untuk membaca latensi buffer. Perubahan rute dan interupsi menjeda output dan memanggil `OnInterrupt` di engine. Aplikasi dapat melanjutkannya dengan `Start`.

## Penyimpanan

`cx.Storage()` adalah penyimpanan kunci-nilai aplikasi, disimpan di perangkat dan dibagikan oleh setiap graf aplikasi. Pembacaan bersifat sinkron, dan pembacaan di dalam komputasi membuat komputasi itu berlangganan perubahan; `SetItem` mengembalikan `core.StorageFullError` jika melewati kuota (`core.StorageQuota`, 5 MiB secara default). Penyimpanan ini adalah file biasa yang dapat dibaca oleh program mana pun milik pengguna, jadi simpan token dan kata sandi di [rahasia](#secrets) sebagai gantinya.

`cx.Store()` mengembalikan penyimpanan yang mendasarinya, untuk host yang membuat root lain dengan `core.RootOptions.Storage`, seperti host yang menjalankan plugin di goroutine-nya sendiri: kedua root kemudian berbagi entri aplikasi dan apa yang disimpan di perangkat. Ini adalah seluruh penyimpanan, bukan partisi yang diberikan `cx.ProvideStorage` kepada kode yang tidak ditulis oleh aplikasi.

Host yang menyediakan persistensinya sendiri memanggil `core.NewStore(backend)` dengan `core.StorageBackend`: `Load() (map[string]string, error)` memuat sekali, dan `Write(core.StorageChange)` menerima perubahan secara berurutan. Lakukan switch pada `change.Op` (`core.StorageSet`, `core.StorageRemove`, `core.StorageClear`) dan gunakan `change.Key` serta `change.Value`. Berikan `nil` untuk penyimpanan yang hanya disimpan di memori.

```go
store := cx.Storage()
if tab, ok := store.GetItem("tab"); ok {
	selected.Set(tab)
}
store.SetItem("tab", "library")
```

`store.Keys()` mencantumkan kunci, `RemoveItem` menghapus satu kunci, dan `Clear` mengosongkan penyimpanan. `cx.DataDir()` dan `cx.CacheDir()` memberikan folder aplikasi untuk file yang terlalu besar bagi penyimpanan kunci-nilai; `cx.LogsDir()` memberikan direktori platform untuk file log, dan `Scope.ProvideStorage` memberikan partisi tersendiri kepada sebuah subpohon.

iOS App Clip dan aplikasi lengkapnya berbagi lokasi data dan penyimpanan ini ketika `app_clip` dan `apple.app_group` eksplisit dikonfigurasi; cache keduanya tetap terpisah. Mengaktifkan grup tidak memigrasikan data sandbox yang sudah ada. [Penyimpanan App Clip](building.md#app-clips) membahas konfigurasi dan penandatanganan.

Di HarmonyOS, operasi file Go seperti `os.ReadFile`, `os.WriteFile`, dan `os.Rename` bekerja di bawah direktori-direktori ini dan `os.TempDir()`. Path relatif dimulai di direktori files. Kunci file advisori dan tautan simbolik tidak tersedia.

### Folder pengguna

`desktop.UserDir(cx, desktop.DownloadsDir)` mengembalikan path salah satu folder pengguna: `DocumentsDir`, `DownloadsDir`, `DesktopDir`, `PicturesDir`, `MusicDir`, atau `VideosDir`. Fungsi ini bertanya ke sistem setiap kali dipanggil, sehingga folder yang dipindahkan pengguna tetap diikuti: folder di direktori home di macOS (`Movies` untuk video), known folders di Windows, termasuk milik OneDrive, dan direktori pengguna XDG di Linux. Folder tersebut mungkin belum ada. Ponsel, web, dan HarmonyOS tidak memiliki folder semacam itu yang dapat dijangkau aplikasi melalui path, begitu pula Linux untuk folder yang tidak dicantumkan `user-dirs.dirs`: `UserDir` mengembalikan `core.ErrUserDirUnavailable`, dan aplikasi meminta pengguna memilih dengan `cx.PickDirectories` atau `cx.SaveFile` sebagai gantinya.

```go
dir, err := desktop.UserDir(cx, desktop.DownloadsDir)
if err != nil {
	dir = cx.DataDir()
}
```

Pengujian tidak pernah mendapatkan folder pengguna: masing-masing adalah direktori kosong di dalam direktori sementara pengujian, dan `nsgotest.UserDirs(map[core.StandardDir]string{…})` menetapkannya, dengan folder yang tidak dicantumkan menjadi tidak tersedia.

### Laporan crash

Build rilis native menyimpan jejak crash Go di bawah `cx.LogsDir()/crashes`. Apple mencatat exception Objective-C yang tidak tertangkap, Android mencatat exception Java yang tidak tertangkap, dan Windows mencatat exception SEH yang mencapai filter tingkat atasnya. Cantumkan laporan yang tertunda pada peluncuran berikutnya dengan `cx.CrashReports()`. Setiap laporan mengidentifikasi versi, build, dan platform yang menghasilkannya. Panggil `Open()` untuk membaca teks diagnostik, tutup pembacanya, lalu panggil `Acknowledge()` untuk menghapus laporan setelah aplikasi memprosesnya. Jalankan operasi file ini di sebuah goroutine.

## Ruang disk

`cx.DiskSpace(func(space core.DiskSpace, err error) { … })` menanyakan volume yang berisi `cx.DataDir()` secara asinkron dan memanggil kembali di goroutine engine. `Total` adalah kapasitas yang dilaporkan sistem, `Available` adalah ruang yang dapat digunakan aplikasi untuk operasi yang diminta pengguna, dan `Opportunistic` adalah ruang yang dapat digunakannya untuk cache dan prefetch. Ketiganya dalam satuan byte. `Available` mencakup ruang yang diambil kembali sistem sesuai kebutuhan jika sistem melaporkan angka tersebut. Platform yang hanya memiliki satu angka ruang kosong mengembalikannya di kedua kolom. Di web, `Total` adalah perkiraan kuota penyimpanan peramban untuk origin tersebut dan `Available` adalah bagian yang belum terpakai. Saat layanan tidak tersedia, fungsi ini mengembalikan `core.ErrDiskSpaceUnavailable`.

Build Apple mengharuskan `apple.privacy.accessed_apis` di `nsgo.yaml` mendeklarasikan `NSPrivacyAccessedAPICategoryDiskSpace`. Deklarasikan `E174.1` untuk memeriksa ruang sebelum menulis atau untuk mengosongkan file saat ruang hampir habis, dengan perubahan yang dapat diamati pengguna. Deklarasikan `85F4.1` untuk menampilkan ruang yang tersedia kepada pengguna. Aplikasi harus mengikuti batasan Apple dalam menggunakan dan membagikan hasil pembacaan tersebut. Build tanpa kategori ini mengembalikan `core.ErrNotDeclared`. [Alasan yang disetujui Apple](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)

Atur jawaban pengujian dengan `nsgotest.DiskSpace(space, err)`.

## Penyimpanan cloud

Paket `cloud` menyinkronkan entri kunci-nilai kecil (`cloud.Get`, `cloud.Set`, `cloud.Delete`, `cloud.Keys`) serta file atau cuplikan simpanan yang lebih besar (`cloud.ReadFile`, `cloud.WriteFile`, `cloud.DeleteFile`, `cloud.ListFiles`) di berbagai perangkat: iCloud Key-Value Store dan kontainer dokumen ubiquity di macOS dan iOS, serta Google Play Games Saved Games di Android.

Setiap operasi mengirimkan callback-nya di goroutine engine dan melewatinya jika `cx` telah dibuang. `cx.Env().CanUseCloudStorage()` melaporkan apakah penyimpanan cloud tersedia; saat pengguna belum masuk, entitlement tidak ada, atau platform tidak memiliki backend cloud native (Windows, Linux, dan web), setiap panggilan mengembalikan `cloud.ErrUnavailable` dan tidak pernah membuang penulisan secara diam-diam.

```go
if cx.Env().CanUseCloudStorage() {
	cloud.WriteFile(cx, "saves/slot1.sav", saveBytes, func(err error) {
		if err != nil {
			status.Set(err.Error())
		}
	})
}

cloud.WatchChanges(cx, func(ev cloud.ChangeEvent) {
	reload(ev.Keys, ev.Files)
})

cloud.OnConflict(cx, func(c cloud.Conflict) {
	_ = c.Resolve(mergeSaves(c.Local, c.Remote))
})
```

`cloud.WatchChanges` dan `cloud.OnConflict` mengirimkan perubahan eksternal multi-perangkat dan konflik versi di goroutine engine serta membersihkan dirinya secara otomatis saat `cx` dibuang. Membaca file yang berkonflik mengembalikan `*cloud.ConflictError` (cocok dengan `errors.Is(err, cloud.ErrConflict)`).

Untuk mengaktifkan entitlement Apple iCloud di `nsgo build`, tambahkan bagian `cloud:` (atau `icloud:`) ke `nsgo.yaml`:

```yaml
cloud:
  kv_store: true
  containers:
    - iCloud.com.example.myapp
```

Profil penandatanganan harus memberikan kapabilitas iCloud yang Anda minta. Di macOS, hanya build Mac App Store (`sign.macos_profile`) yang membawa iCloud, karena macOS hanya memberikannya dengan profil provisioning; build Developer ID menghilangkannya disertai peringatan. Di Android, penyimpanan cloud memerlukan SDK Play Games Services v2 di dalam aplikasi dan pemain yang sudah masuk.

Di toko desktop seperti Steam, Steam Auto-Cloud menyinkronkan `cx.DataDir()` secara langsung tanpa kode tambahan sama sekali — tetap tulis file simpanan lokal di sana dan biarkan Steam menyinkronkan direktori tersebut.

Dalam pengujian, `nsgotest` memberi setiap aplikasi penyimpanan cloud terisolasi di memori: alihkan ketersediaannya dengan `nsgotest.CloudAvailable` atau `app.SetCloudAvailable`, simulasikan pembaruan eksternal multi-perangkat dengan `app.SimulateCloudChange`, dan uji penyelesaian konflik dengan `app.SimulateCloudConflict`.

## Rahasia

`cx.Secrets()` adalah penyimpanan aman aplikasi, untuk refresh token, kunci API, dan kata sandi: `Get`, `Set`, dan `Delete` sebuah string berdasarkan akun, yang dilindungi oleh platform. Akun yang tidak ada terbaca sebagai `("", false, nil)`, dan jika platform tidak memiliki penyimpanan aman, seperti di web, setiap panggilan mengembalikan `core.ErrSecretsUnavailable`; NSGo tidak pernah beralih ke penyimpanan biasa. Sistem dapat membuat panggilan menunggu, seperti keychain yang bertanya kepada pengguna, jadi panggil fungsi-fungsi ini di luar goroutine engine dan serahkan hasilnya kembali dengan `cx.Post`.

```go
secrets := cx.Secrets()
go func() {
	token, ok, err := secrets.Get("refresh-token")
	cx.Post(func() {
		if err == nil && ok {
			login(token)
		}
	})
}()
```

| Platform | Tempat rahasia disimpan |
| --- | --- |
| macOS | Kata sandi generik di keychain login: `kSecAttrService` adalah layanannya, `kSecAttrAccount` adalah akunnya |
| iOS, tvOS, watchOS | Kata sandi generik di keychain, dengan cara yang sama |
| Windows | Kredensial generik di Credential Manager yang targetnya `service/account`, paling banyak 2560 byte |
| Linux | Item di Secret Service (GNOME Keyring, KWallet) dengan atribut `service` dan `account`; tidak tersedia jika sesi tidak menjalankan layanan tersebut |
| Android | Dienkripsi dengan kunci AES dari Android Keystore, di file privat aplikasi, di luar cadangan; pembacaan juga beralih ke nilai yang dienkripsi `expo-secure-store` ke dalam `SharedPreferences` `SecureStore` miliknya |
| Mini program WeChat | Penyimpanan terenkripsi platform (`wx.setStorage` / `wx.getStorage` dengan `encrypt: true`, pustaka dasar 2.21.3+); tidak tersedia di pustaka dasar yang lebih lama |
| Web, mini program lain | Tidak tersedia (`core.ErrSecretsUnavailable`): peramban tidak memiliki penyimpanan rahasia klien yang dilindungi OS (simpan sesi web di cookie `HttpOnly` yang diatur server), dan mini program QQ, Alipay, dan Douyin tidak memiliki penyimpanan terenkripsi |

Layanannya adalah ID aplikasi (`core.App().ID`) secara default. Aplikasi yang berpindah dari framework lain mempertahankan penggunanya tetap masuk dengan menamai item yang ditulis oleh framework tersebut: `Service`, serta di platform Apple `AccessGroup` dan `Accessible`. Di Android, atur `Service` ke `keychainService` milik `expo-secure-store` (`"key_v1"` secara default) untuk membaca nilai yang dienkripsinya ke dalam `SharedPreferences`; menulis atau menghapus akan membersihkan entri yang dimigrasikan. Di iOS, `core.SecretAfterFirstUnlock` memungkinkan pekerjaan latar belakang membaca item selama ponsel terkunci, dan aksesibilitas `ThisDeviceOnly` menjauhkannya dari cadangan. Di macOS, `AccessGroup` memindahkan item ke keychain perlindungan data, yang memerlukan build yang ditandatangani oleh tim yang entitlement `keychain-access-groups`-nya mencantumkan grup tersebut, dikonfigurasi di bawah `macos.entitlements` di `nsgo.yaml` untuk build Mac App Store maupun Developer ID.

```go
s := cx.Secrets()
s.Service = "app:no-auth" // the service an earlier build used
s.Accessible = core.SecretAfterFirstUnlockThisDeviceOnly
err := s.Set("refresh-token", token)
```

`nsgotest` menyimpan rahasia aplikasi di memori, di `App.Secrets`, dan `nsgotest.Secrets` memulai pengujian dengan beberapa rahasia, sehingga pengujian tidak pernah menyentuh keychain mesin. Paket TSX menjangkau penyimpanan ini dengan kapabilitas `secrets`, di bawah layanannya sendiri.

## Jaringan

`cx.Env().Network()` melaporkan apakah sistem memiliki rute yang dapat digunakan (`Online`) dan apakah sistem menganggap rute itu berkuota atau mahal (`Expensive`). Binding mengikutinya seperti nilai lingkungan lainnya. `cx.OnNetworkChange(func(core.Network))` memanggil handler di goroutine engine untuk setiap perubahan selama cakupannya hidup. Rute sistem tidak menyatakan apa pun tentang apakah server tertentu menjawab. Target yang tidak dapat melaporkan konektivitas dianggap online secara default, dan biaya yang tidak diketahui dianggap false secara default.

Atur `core.Environment{Network: &core.Network{Online: false}}` melalui `nsgotest.Environment` atau `App.SetEnvironment` untuk menguji perilaku offline.

`net/http` milik Go sendiri membuat permintaan di setiap target, dengan pengaturan TLS dan proxy sistem. Signal adalah milik goroutine engine, sehingga respons dari goroutine lain mencapainya melalui `cx.Post`.

```go
go func() {
	resp, err := http.Get("https://api.example.com/items")
	if err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	defer resp.Body.Close()
	var items []Item
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	cx.Post(func() { list.Set(items) })
}()
```

`cx.Post` menjalankan fungsinya di goroutine graf, tempat signal boleh diatur. Di Android, manifes meminta izin internet, dan unduhan gambar di-cache sendiri di disk.

`core.NewResource` melakukan hal yang sama untuk nilai yang dimuat dari sebuah sumber. Fetcher-nya berjalan di goroutine-nya sendiri dan memuat ulang saat sumber berubah:

```go
items := core.NewResource(cx, func() (string, bool) { return query.Get(), query.Get() != "" },
	func(ctx context.Context, q string, _ core.ResourceInfo) ([]Item, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/items?q="+url.QueryEscape(q), nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var list []Item
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return nil, err
		}
		return list, nil
	})
```

`items.Get()`, `items.Err()`, dan `items.Loading()` adalah signal yang diikuti oleh binding. Kueri baru membatalkan permintaan yang masih berjalan melalui `ctx`, sehingga jawaban yang terlambat tidak pernah menggantikan jawaban yang lebih baru. `items.Refetch()` mencoba lagi, dengan mempertahankan daftar terakhir selama memuat, dan `items.Mutate` langsung mengatur nilai untuk pembaruan optimistis.

`core.Suspense` menampilkan cadangan (*fallback*) selama resource yang dibaca kontennya memuat nilai pertamanya, sehingga layar tidak memerlukan penanda pemuatan sendiri:

```go
core.Suspense{
	Content:  func(cx *core.Scope) core.Node { return itemList(cx, items) },
	Fallback: func(*core.Scope) core.Node { return ui.Spinner{Label: "Loading"} },
}
```

Konten tetap terpasang tetapi tersembunyi selama menunggu, sehingga langsung tampil saat nilai tiba. Refetch tetap menampilkannya dengan nilai terakhir.

### Menemukan layanan lokal

Di macOS, iOS, Android, Windows, dan Linux, `cx.AdvertiseService` memublikasikan port yang sedang mendengarkan dan atribut TXT. `cx.BrowseServices` melaporkan layanan yang telah di-resolve beserta host, port, dan atribut TXT-nya. `Host` dapat berupa nama host DNS atau alamat IP. Saat sebuah layanan pergi, callback menerimanya lagi dengan `Removed` diatur. Periksa `cx.Env().CanDiscoverServices()` untuk dukungan platform. Kedua panggilan mengembalikan fungsi penghenti dan berakhir saat cakupannya dibuang. Galat platform dilaporkan secara asinkron di goroutine engine. Linux menggunakan Avahi dan melaporkan galat jika daemon tidak tersedia.

Untuk Bonjour di platform Apple, deklarasikan tipe layanan dan jelaskan akses jaringan lokal di `nsgo.yaml`. Sistem mengelola permintaan izin dan mengingat pilihan pengguna.

```yaml
apple:
  info:
    NSLocalNetworkUsageDescription: Finds nearby devices to sync with.
    NSBonjourServices:
      - _example._tcp
```

Android menggunakan `NsdManager`. Android 12 dan sebelumnya serta Android 13 sebelum T Extensions 7 memerlukan `CHANGE_WIFI_MULTICAST_STATE` untuk penemuan Wi-Fi. Build mandiri mendeklarasikan izin tersebut. Jika Anda menyematkan NSGo di host Android, tambahkan izin itu ke manifes host untuk perangkat-perangkat tersebut. Nilai TXT yang diiklankan harus berupa UTF-8 yang valid di perangkat tanpa API TXT mentah Android (API 37 atau T Extensions 22). Penjelajahan mempertahankan nilai TXT mentah. Jika host Android menargetkan API 37 atau lebih tinggi, host tersebut harus mendeklarasikan dan meminta `ACCESS_LOCAL_NETWORK` di Android 17 atau lebih baru.

Gunakan `nsgotest.LocalServices` untuk menyediakan hasil penjelajahan dalam pengujian headless.

## Now Playing

Gunakan `cx.SetNowPlaying(core.NowPlaying{...})` untuk menampilkan media aplikasi saat ini di Control Center dan bilah menu macOS. Sediakan `Title`, `Artist`, `Album`, dan `Artwork` opsional sebagai `*core.ImageSource`. `Duration` nol berarti siaran langsung. Berikan `Position` saat ini dan `Rate` pemutaran (nol saat dijeda, satu pada kecepatan normal). Publikasikan lagi saat trek, posisi, atau laju berubah. Sistem memajukan posisi di antara panggilan.

Cantumkan kontrol transport yang didukung di `Commands`, seperti `core.MediaPlay`, `core.MediaPause`, `core.MediaTogglePlayPause`, dan `core.MediaSeek`. Tangani permintaan dengan `cx.OnMediaCommand`. Callback menerima `Kind` perintah dan, untuk seek, `Position` yang diminta. Handler hidup yang terbaru menerima perintah. Panggil `cx.ClearNowPlaying()` saat aplikasi selesai memutar. Selama aplikasi memublikasikan secara manual, video terkontrol di macOS menangguhkan publikasinya sendiri. Periksa `cx.Env().CanShowNowPlaying()` untuk dukungan platform.

## Unduhan latar belakang

Gunakan `cx.Download` dan `cx.OnDownloadUpdate` di aplikasi iOS dan macOS mandiri, serta di Android. Tampilan Apple yang disematkan tidak memiliki host transfer. Aplikasi Android mengaktifkan `android.background_downloads: true` di `nsgo.yaml`. Mulai unduhan dari aksi pengguna selama aplikasi terlihat. Gunakan tujuan absolut dengan direktori induk yang sudah ada dan dapat ditulis oleh aplikasi.

```go
cx.OnDownloadUpdate(func(p core.DownloadProgress) {
    progress.Set(p)
})
task, err := cx.Download(core.DownloadRequest{URL: url}, path, core.DownloadOptions{
    CertificatePEM: serverCertificate,
})
```

Transfer menggunakan GET dan menerima header permintaan opsional. Transfer menggunakan trust store HTTPS normal kecuali `CertificatePEM` menyematkan (*pin*) sertifikat leaf server. Pin tersebut juga memerlukan nama host yang cocok dan sertifikat yang valid. Gunakan URL akhir karena layanan memerlukan respons sukses dari URL tersebut.

Di Apple, sertifikat self-signed pada nama host publik memerlukan pengecualian App Transport Security untuk host tersebut di `apple.info`. Transfer tetap memeriksa pin sertifikat yang tepat, nama host, dan validitasnya. Koneksi NAS lokal juga memerlukan `NSLocalNetworkUsageDescription` yang menjelaskan akses, dan host tanpa kualifikasi atau `.local` menggunakan `NSAllowsLocalNetworking`. [Pengaturan ATS Apple](https://developer.apple.com/documentation/security/preventing-insecure-network-connections)

```yaml
apple:
  info:
    NSAppTransportSecurity:
      NSExceptionDomains:
        files.example.com:
          NSExceptionAllowsInsecureHTTPLoads: true
```

Layanan foreground Android terus berjalan setelah activity meninggalkan layar. Layanan ini memasang notifikasi progres, bergantung pada izin notifikasi Android. Progres menggunakan hitungan byte dan mengatur `Total` ke -1 hingga diketahui. Keberhasilan menggantikan tujuan secara atomik. `task.Pause()` mempertahankan file parsial, `task.Resume()` menggunakan range HTTP ketika server menyediakan validator, dan `task.Cancel()` menghapus file parsial.

iOS menggunakan URLSession latar belakang sistem yang melanjutkan transfer selama aplikasi ditangguhkan. Aplikasi menyambungkan kembali sesinya saat diluncurkan dan memublikasikan file sebelum mengakui penyelesaian latar belakang. Transfer macOS berjalan di dalam proses. Platform Apple menggunakan data resume native untuk unduhan yang dijeda dan menyimpan metadata permintaan di penyimpanan privat aplikasi hingga hasil akhir disimpan. Server yang tidak dapat melanjutkan akan memulai transfer dari awal.

Daftarkan `OnDownloadUpdate` lagi saat aplikasi dimulai untuk melihat tugas aktif dan hasil yang tersimpan. Gunakan `p.Task.Forget()` setelah menangani hasil akhir. Android dapat menghentikan layanan, misalnya saat waktu transfer foreground-nya habis. Aplikasi kemudian menerima `DownloadInterrupted`. Di Windows, Linux, tvOS, watchOS, dan web, `Download` mengembalikan `ErrDownloadUnavailable`.

## Ucapan dan siklus hidup

`cx.StartSpeech` mengenali ucapan dengan pengenal (*recognizer*) sistem jika `cx.Env().CanRecognizeSpeech()` menyatakan pengenal itu ada (macOS, iOS, Android, Windows, dan peramban yang menyediakan `SpeechRecognition` atau `webkitSpeechRecognition`), dan menjawab dengan transkrip saat transkrip itu berubah. Di platform Apple, deklarasikan `NSMicrophoneUsageDescription` dan `NSSpeechRecognitionUsageDescription` di `apple.info`; di Android tambahkan `RECORD_AUDIO` ke `android.permissions`. Dalam pengujian headless, `app.Speech` menyimpan `nsgotest.SpeechSession` selama pengenalan aktif. Gunakan `app.HearSpeech` dan `app.EndSpeech` untuk mengirimkan hasil.

```go
session, err := cx.StartSpeech(core.SpeechOptions{
	Lang:    "en-US",
	Interim: true,
	OnResult: func(text string, final bool) {
		transcript.Set(text)
	},
})
if err == nil {
	defer session.Stop()
}
```

`cx.Env().Lifecycle()` menyatakan apakah aplikasi aktif, tidak aktif, atau di latar belakang: di desktop untuk jendela tempat `cx` berada, sehingga konten jendela yang diminimalkan dapat dijeda selama jendela lain tampil, dan dalam cakupan milik aplikasi untuk semua jendelanya sekaligus. `desktop.Minimized(cx)` menyatakan apakah jendela itu diminimalkan, dan `desktop.FrontmostApp(cx)` menyatakan aplikasi mana yang berada di depan pada desktop, termasuk aplikasi itu sendiri, serta apakah jendelanya dalam layar penuh, seperti yang diikuti oleh hewan peliharaan desktop atau pelacak waktu; `desktop.CanReadFrontmostApp(cx)` menyatakan apakah hal itu dapat diikuti, di macOS, Windows, X11, dan compositor Wayland yang didukung. `cx.OnQuit` membereskan saat aplikasi keluar, bagaimanapun caranya, dengan waktu 200 ms untuk pekerjaannya. `desktop.OnQuitRequest` dapat membatalkan atau menunda item menu Quit desktop atau `desktop.RequestQuit(cx)`. Di Windows dan Linux, handler ini juga berjalan setelah `OnClosing` ketika menutup jendela akan mengakhiri aplikasi. `desktop.RequestRestart(cx)` menanyakan handler yang sama sebelum memulai ulang; `desktop.Restart(cx)` tetap langsung. Peluncuran ulang oleh updater di Linux dan Windows juga bertanya, sehingga penyimpanan dapat membatalkan atau menundanya. Saat sistem melaporkan logout, restart, atau shutdown, `q.Forced()` bernilai true dan aplikasi berlanjut melalui `OnQuit`. Periksa `Forced()` sebelum meminta keputusan pengguna. `cx.SetAppearance` memaksakan tema terang atau gelap, `cx.SetLocale` memilih bahasa, `cx.OnKey` mengikat pintasan papan ketik, yang beristirahat selama apa yang ditampilkan pemiliknya tersembunyi atau berada di bawah dialog kecuali `core.KeyOptions{OverModals: true}` mempertahankannya, yang mengambil tombolnya sebelum menu aplikasi kecuali `YieldsToMenu` menyerahkannya ke item menu aktif yang memakai pintasan yang sama di tempat fokus berada (selama kolom teks atau web view memegang fokus, item `MenuNotEditing` menyerahkan tombolnya kepada binding dan menahan tombol itu dari item-item berikutnya yang memakai pintasan yang sama, dan menu Linux tidak menerima tombol yang ditekan tanpa Control, Alt, atau tombol logo, sehingga binding mempertahankan tombol seperti itu di sana), yang bekerja di jendela tempat `cx` berada kecuali `AllSurfaces` mengikatnya di setiap jendela dan panel aplikasi, termasuk yang dibuka kemudian, untuk pintasan seluruh aplikasi, yang beristirahat di semuanya selama apa yang ditampilkan pemiliknya tersembunyi di jendela tempat ia diikat, dan yang menyerahkan kepada kontrol yang terfokus tombol-tombol yang dipakainya, Return dan Space untuk tombol serta tombol panah untuk slider, bilah tab, atau grup radio, kecuali diikat dengan `core.KeyAlways` atau `core.KeyNotEditing`; `cx.OnOpenFiles` menerima dokumen yang dibuka di macOS, Windows, Linux, dan iOS. iOS mengimpor file biasa ke penyimpanan sementara milik aplikasi; pindahkan ke `cx.DataDir()` untuk menyimpannya. Di macOS, menutup jendela yang menyediakan menu aplikasi akan memulihkan menu milik jendela tersisa yang paling baru, atau menu default jika tidak ada yang tersisa.

Di Wayland, pelacakan aplikasi terdepan memerlukan protokol wlr foreign-toplevel milik compositor versi 2 atau lebih baru; compositor lain melaporkan `CanReadFrontmostApp` false. ID-nya adalah ID aplikasi Wayland, namanya berasal dari entri desktop jika tersedia, dan PID-nya 0. Aplikasi Windows yang dikemas menggunakan AUMID-nya alih-alih path frame-host. macOS mengikuti status presentasi layar penuh AppKit alih-alih menyimpulkannya dari ukuran jendela.

Peran penyuntingan di menu aplikasi mengambil editor yang terfokus sebelum cadangan `OnSelect`-nya. Di macOS, Undo dan Redo mengikuti riwayat editor tersebut, dan Cut mengikuti seleksinya, termasuk saat aplikasi tidak aktif; tanpa editor, cadangan tetap aktif.

Di iOS, berikan fungsi `core.KeyOptions.Title` kepada `cx.OnKeyWith` untuk menawarkan pintasan di menu papan ketik sistem selama binding dapat terpicu. `Group` menamai menunya di iPadOS 26 dan yang lebih baru. Fungsi-fungsi ini dapat membaca katalog aplikasi sehingga judul pintasan mengikuti bahasa. Gunakan `app.KeyCommands()` dalam pengujian untuk memeriksa daftar saat ini.

```go
cx.OnKey("mod+k", func() { palette.Set(true) })
cx.SetLocale("de")
cx.OnQuit(func(ctx context.Context) { saveDraft(ctx) })
```

`cx.Env().DeviceName()` adalah nama yang diberikan pengguna untuk perangkat, dan `cx.Env().DeviceModel()` adalah model perangkat kerasnya, seperti `Mac15,7`, `iPhone15,2`, atau `Google Pixel 8`, sehingga aplikasi dapat membedakan dua perangkat yang dipasangkan. macOS, Android, Windows, dan Linux memberikan keduanya; iOS memberikan modelnya, dan namanya hanya kepada aplikasi yang memiliki entitlement nama perangkat dari pengguna milik Apple. watchOS memberikan pengenal perangkat kerasnya, HarmonyOS nama pasar atau model produknya, dan mini program merek serta model platform; target-target tersebut membiarkan nama dari pengguna kosong. Web tidak memberikan keduanya. Pengujian mengaturnya dengan `nsgotest.Environment`.

`cx.Env().ThermalState()` melaporkan `core.ThermalNominal`, `ThermalFair`, `ThermalSerious`, atau `ThermalCritical` di platform Apple dan Android API 29 atau lebih baru. `cx.Env().LowPowerMode()` melaporkan `core.LowPowerOn` atau `LowPowerOff` di platform Apple, Android, Windows, dan sesi Linux dengan power-profiles-daemon. Jika sistem tidak memberikan nilai, pembacaan mengembalikan `ThermalUnknown` dan `LowPowerUnknown`. Setiap pembacaan mengikuti perubahan secara independen, sehingga aplikasi dapat menyesuaikan beban kerjanya selama berjalan. Dalam pengujian, atur `ThermalState` dan `LowPowerMode` pada `core.Environment` melalui `nsgotest.Environment` atau `app.SetEnvironment`.

`desktop.CursorPoint(cx, func(p core.Point, available bool) { ... })` membaca penunjuk bahkan di luar jendela aplikasi di macOS, Windows, dan X11. Fungsi ini mengambil satu sampel per panggilan. Titiknya berada dalam koordinat layar yang sama dengan `desktop.Monitors(cx)` dan `Window.ShowAt`. Di Wayland dan target lain yang tidak didukung, atau jika cakupan tidak memiliki `core.AccessCursorPoint`, `available` bernilai false.

`desktop.SetWindowConstraints` membatasi ukuran konten dan mengontrol pengubahan ukuran, minimalisasi, dan maksimalisasi; atur `WindowOptions.Constraints` saat membuka jendela atau gunakan `nsgo.WindowConstraints` untuk jendela utama. Linux menerapkan batas maksimum yang tidak tetap di X11 dan menghapus tombol minimalkan/maksimalkan yang dilarang dari bilah judul native GTK di X11 dan Wayland. Wayland saat ini hanya mendukung ukuran tetap untuk batas atas.

`desktop.SetWindowLevel` membuat jendela mengambang di atas jendela lain, dan `desktop.SetWindowWorkspaces(cx, desktop.WindowWorkspacesAll)` atau `WindowOptions.Workspaces` mempertahankannya di setiap ruang kerja, seperti hewan peliharaan desktop, HUD, atau penghitung waktu: di macOS di setiap Space dan di atas ruang layar penuh aplikasi lain, di luar Cmd-`, dan di Linux di setiap desktop virtual pengelola jendela X11. Windows dan Wayland membiarkannya di ruang kerja tempat jendela itu dibuka, dan `desktop.CanSetWindowWorkspaces` menyatakan apakah pengaturan itu berlaku: true di macOS dan di bawah pengelola jendela X11 yang mendukung `_NET_WM_DESKTOP`. Jendela utama ditempatkan di setiap ruang kerja dengan memanggil `desktop.SetWindowWorkspaces` dari kontennya. Dalam pengujian, `nsgotest.App.Window` mencantumkan permintaan, dan `nsgotest.Environment` mengatur `CanSetWindowWorkspaces`.

`desktop.RegisterHotKey` mengikat pintasan global, yang bekerja selama aplikasi apa pun berada di depan, di macOS, di Linux, dan di aplikasi Windows yang dijalankan dengan `nsgo.Run(App)`. `State` milik handle-nya menyatakan apakah sistem menerimanya: `HotKeyPending` hingga sistem menjawab, lalu `HotKeyRegistered`, `HotKeyTaken` saat registrasi lain memegang kombinasi tersebut, atau `HotKeyUnsupported` untuk tombol atau platform tanpa pintasan global, sehingga perekam pintasan dapat meminta kombinasi lain. Di macOS, sebuah kombinasi dianggap sudah dipakai hanya jika aplikasi sudah memegangnya atau aplikasi lain mendaftarkannya secara eksklusif. Di Wayland, compositor meminta pengguna untuk mengikat pintasan: pintasan terdaftar setelah pengguna melakukannya, dan dianggap sudah dipakai saat pengguna atau compositor tidak menyertakannya. Dalam pengujian, `nsgotest.HotKeyTaken` dan `nsgotest.HotKeyUnsupported` membuat kombinasi menjawab demikian.

Dengan `desktop.TitleBarHidden`, Windows dan Linux menawarkan menu aplikasi sebagai popup saat Alt ditekan dan dilepas sendirian atau F10. Alt dengan huruf atau digit pertama dari label tingkat atas membuka submenu tersebut atau memilih perintah tersebut, dalam aksara label itu. Binding aplikasi mempertahankan tombolnya; Shift+F10 tetap menjadi tombol menu konteks, dan kombinasi Alt lainnya tetap milik sistem. Escape menutup popup dan memulihkan fokus papan ketik.

```go
shortcut := desktop.RegisterHotKey(cx, "alt+space", launcher.Toggle)
warning := core.Text{ContentFunc: func() string {
	if shortcut.State() == desktop.HotKeyTaken {
		return "Another app uses Alt+Space"
	}
	return ""
}}
```

Pengubahan ukuran desktop di macOS, Windows, dan Linux menunggu hingga 16 ms untuk tata letak baru sebelum menggambar. Jaga agar komputasi reaktif dan handler event tetap cepat sehingga tata letak dapat memenuhi tenggat itu; lakukan I/O yang lambat di goroutine dan kirim hasilnya melalui `cx.Post`. Aplikasi yang sibuk tetap melanjutkan pengubahan ukuran dan menampilkan tata letaknya saat sudah menyusul.

`desktop.SetWindowFrame` memindahkan jendela saat `WindowFrame.Move` bernilai true, menggunakan `X` dan `Y` dalam satuan point dari sudut kiri atas layar utama. Dimensi yang tidak diatur mempertahankan ukurannya saat ini. Periksa `desktop.CanPlaceWindows`: macOS, Windows, dan Linux di X11 mendukung penempatan posisi; toplevel Wayland biasa mempertahankan posisi dari compositor. `desktop.Frame` mengikuti posisi jendela native di desktop-desktop ini.

Di macOS dan Windows, `WindowFrame{Animated: true}` menggerakkan panel yang terlihat secara halus ke frame barunya, termasuk ukuran panel yang di-dock. Panel tersembunyi dan mode gerakan dikurangi menerapkan frame baru secara langsung. Jendela macOS biasa menggunakan animasi AppKit.

Atur `desktop.WindowOptions.Parent` ke jendela aplikasi yang sedang terbuka untuk menutup keduanya saat jendela induk ditutup. Di desktop, jendela anak tetap berada di atas induknya dan mengikutinya saat induk disembunyikan atau diminimalkan; jendela anak yang disembunyikan secara eksplisit tetap tersembunyi saat induk kembali. Di macOS, jendela anak juga diposisikan di tengah induknya saat `Frame` tidak diatur dan mengikutinya ke layar penuh.

`WindowOptions.NoShadow` dan `desktop.SetWindowShadow(cx, false)` juga menghilangkan bayangan dari jendela Windows yang bilah judulnya disembunyikan, yang menggambar bingkainya sendiri; jendela dengan bilah judul sistem mempertahankan bingkai dan bayangannya, begitu pula jendela Linux yang didekorasi oleh pengelola jendelanya, di mana `NoShadow` hanya menghapus bayangan dari dekorasi milik GTK sendiri.

`desktop.SetWindowPointerEvents` meneruskan penunjuk melalui sebuah jendela ke jendela di bawahnya, milik aplikasi apa pun: `WindowPointerNone` di mana saja, seperti yang dilakukan *heads-up display*, dan `WindowPointerPainted` di mana pun tidak ada tampilan yang menggambar sesuatu atau mendengarkan penunjuk, seperti di sekitar hewan peliharaan desktop yang tetap menerima tekanan, seretan, dan klik kanan pada dirinya sendiri; bilah judul yang digambar sistem dihitung sebagai tidak ada yang digambar. `WindowOptions.PointerEvents` dan `PanelOptions.PointerEvents` membuka jendela dengan cara tersebut. Wilayah `Desktop.WindowDrag` tetap menyeret di dalam leluhur yang dapat ditekan; kontrol yang dapat ditekan atau kontrol penyuntingan yang lebih dekat mempertahankan inputnya sendiri alih-alih menyeret jendela. Jendela yang tidak menerima penunjuk tidak mendengar tekanan, sehingga aplikasi mengembalikannya dari pintasan, ikon baki, atau jendela lain. Kode yang dipegang oleh sebuah `Access` memerlukan `core.AccessOverlay`.

```go
desktop.OpenPanel(cx, desktop.PanelOptions{Width: 600, Height: 400, PointerEvents: desktop.WindowPointerPainted}, Pet)
```

Di macOS dan Windows, gambar di jendela yang dicat dapat mengatur `Image.HitTestAlpha` (TSX `hitTestAlpha`) ke nilai di atas 0 dan paling banyak 1. Misalnya, 0.5 menerima penunjuk pada piksel gambar yang setidaknya setengah buram. Ambang ini juga berlaku untuk `OnPress` milik gambar. Latar belakang atau border tampilan tetap dihitung sebagai dicat. Nol menggunakan seluruh tampilan gambar, begitu pula gambar di Linux.

Di jendela macOS yang dicat, `Video.HitTestAlpha` (TSX `hitTestAlpha`) menerapkan ambang yang sama pada frame video yang telah didekode. Ini bekerja dengan `Controls` dimatikan dan trek video yang tidak ditransformasi, seperti HEVC dengan alfa untuk hewan peliharaan desktop. Nol, frame yang tidak tersedia, dan host desktop lainnya menggunakan batas tampilan video.

`desktop.NewTray` dapat menggambar ikon langsung dengan `TrayOptions.View` di macOS dan Windows. Windows menyesuaikan rendering transparan tampilan dengan ukuran ikon area notifikasi dan memperbaruinya saat tampilan berubah atau gambar selesai dimuat; gunakan `TrayOptions.OnClick`, `Panel`, dan `Menu` untuk aksinya. Linux saat ini menampilkan ikon PNG; menggambar tampilan bakinya masih menjadi tujuan.

`desktop.OpenPanel` membuka panel seperti yang ditampilkan oleh pintasan atau ikon baki semacam itu: jendela tanpa bingkai di atas jendela lainnya. Seperti `desktop.OpenWindow`, `desktop.NewTray`, `mobile.OpenOverlay`, `audio.NewPlayer`, `audio.LoadSound`, dan `core.NewHolder`, fungsi ini menjadi milik langsung `cx` yang Anda berikan, bukan milik pemilik handler yang sedang berjalan, sehingga memberikan cakupan aplikasi dari `OnPress` sebuah tombol akan menjaga jendela, panel, atau pemutar tetap hidup setelah cakupan tombol itu hilang; primitif reaktif (`Effect`, `Cleanup`, `AfterFunc`, `OnKey`) sebaliknya menjadi milik pemilik yang sedang berjalan, yang dialihkan oleh `cx.Run`. Kartu popup menggambar dirinya pada panel dengan `Transparent`, yang tidak memiliki latar belakang, bingkai, atau bayangan sendiri, sehingga sudut membulat dan `BoxShadow` milik kartu adalah satu-satunya yang tampil, dengan ruang untuk bayangan di dalam panel; `FitContent` mengatur ukuran panel sesuai kontennya, hingga `Width` dan `Height`, sebelum pertama kali tampil dan saat konten berubah. `desktop.Transparent` menyatakan apakah panel saat ini tidak menggambar apa pun miliknya sendiri, seperti pada jendela transparan, sedangkan `desktop.CanMakeWindowsTransparent` hanya menyatakan apakah sebuah jendela bisa transparan: jika bernilai false, seperti di Linux tanpa compositor, berikan latar belakang pada konten.

Di macOS, panel tanpa `Transparent` menggunakan sudut persegi saat salah satu dimensinya di bawah 24 point dan memulihkan sudut membulatnya pada ukuran normal.

```go
lookup := desktop.OpenPanel(cx, desktop.PanelOptions{Width: 380, Height: 520, Transparent: true, FitContent: true, HideOnBlur: true}, Card)
desktop.RegisterHotKey(cx, "alt+space", lookup.Toggle)
```

`desktop.NewTray` menampilkan `TrayOptions.Title` di samping gambar atau tampilan yang digambarnya di macOS. Untuk popup kustom, baca `tray.ClickBounds()` di dalam `OnClick` atau `Menu` dan berikan persegi panjang yang tersedia ke `panel.ShowAt(bounds, core.PlaceBottom)`. Fungsi ini menggunakan point layar yang sama dengan `Env.Displays` dan mencakup judul. Batas tidak tersedia sebelum klik pertama, setelah penghapusan, di Linux, dan saat shell Windows tidak dapat menyediakannya. Setiap klik menggantikan batas sebelumnya, sehingga hasil yang tidak tersedia tidak pernah memakai ulang persegi panjang lama. Mengatur `TrayOptions.Panel` justru mempertahankan perilaku klik-untuk-mengalihkan bawaan.

`PanelOptions.Dock` menjaga panel tetap di tengah sepanjang tepi yang dapat digunakan pada sebuah layar melalui `Show`, `Focus`, `Toggle`, dan pengubahan ukuran konten. Atur `Edge` ke `core.PlaceTop`, `PlaceBottom`, `PlaceLeft`, atau `PlaceRight`, `Display` ke ID `Env.Displays` (kosong untuk layar utama), dan `Margin` ke jarak dari tepi tersebut dalam point. Panel yang di-dock tidak dapat menggunakan `ShowAt` atau `MoveTo`; `SetWindowFrame` hanya mengubah ukurannya. Ikon baki menampilkannya di posisi dock-nya alih-alih di bawah ikon.

```go
panel := desktop.OpenPanel(cx, desktop.PanelOptions{
	Width: 320, Height: 80, NoActivate: true,
	Dock: desktop.PanelDock{Edge: core.PlaceBottom, Margin: 12},
}, Candidates)
panel.OnPlacementError(cx, func(err error) { /* offer another presentation */ })
panel.Show()
```

`desktop.CanDockPanels(cx)` melaporkan dukungan docking secara terpisah dari penempatan jendela sembarang. macOS, Windows, dan X11 mendukungnya; Wayland memerlukan pustaka runtime opsional `libgtk4-layer-shell.so.0` dan compositor yang mengimplementasikan `zwlr_layer_shell_v1`. NSGo memuat pustaka tersebut sebelum GTK; host penyemat yang sudah memuat Wayland harus memuat gtk4-layer-shell terlebih dahulu sebelum memulai host. Pustaka, protokol, atau layar terpilih yang tidak ada membuat panel tetap tersembunyi dan memanggil `OnPlacementError`. Dock Wayland menggunakan lapisan atas tanpa mencadangkan ruang kerja: `NoActivate` membiarkan papan ketik tetap pada jendela saat ini, sedangkan panel tanpanya mengambil fokus papan ketik eksklusif hingga disembunyikan. Jendela Wayland biasa mempertahankan penempatan compositor seperti biasanya.

Perekam pintasan memungkinkan pengguna memilih tombol seperti itu. `OnKeyDown` milik tampilan `Focusable` mendengar setiap tombol yang ditekan selama tampilan memegang fokus, sebelum binding aplikasi, beserta modifier yang ditahan, termasuk modifier yang ditekan sendirian; mengembalikan `true` menahan tombol dari binding, sehingga merekam Command+S tidak menyimpan. `KeyEvent.Shortcut` menuliskan tekanan itu dalam bentuk yang diterima `OnKey` dan `RegisterHotKey`, setiap modifier dengan namanya ("ctrl+shift+k", "alt+space", "shift+meta+k" untuk Command+Shift+K), dan "" untuk modifier sendirian. `OnKeyUp`-nya mendengar setiap tombol tersebut dilepas, bahkan setelah `OnKeyDown` mengonsumsinya atau fokus berpindah, beserta modifier yang masih ditahan. Keduanya bekerja di macOS, Windows, Linux, web (di mana peramban mempertahankan pintasan yang tidak diberikannya kepada halaman), Apple TV, dan di `nsgotest`; Android mengirimkan event papan ketik perangkat keras ke tampilan yang terfokus secara asinkron, sehingga nilai kembalian `OnKeyDown` di sana tidak dapat mengonsumsi tombol. iPhone dan iPad mendukung polling tombol perangkat keras. Pintasan yang didaftarkan aplikasi secara global mengambil tombolnya sebelum tampilan, sehingga perekam membatalkan pendaftarannya selama merekam.

```go
core.View{
	Focusable: func() bool { return true },
	OnKeyDown: func(e core.KeyEvent) bool {
		if s := e.Shortcut(); s != "" {
			shortcut.Set(s)
		}
		return true
	},
	OnKeyUp: func(core.KeyEvent) { recording.Set(false) },
	Children: []core.Node{core.Text{ContentFunc: shortcut.Get}},
}
```

Game atau canvas justru melakukan polling tombol. `core.KeysDown(cx)` mencantumkan tombol yang ditahan di jendela dan `core.KeyHeld(cx, "left")` menyatakan apakah suatu tombol ditahan: pembacaan terlacak, yang juga dibaca oleh loop `RequestAnimationFrame`. Pembacaan pertama membuat platform melaporkan setiap tombol sejak saat itu, sehingga tombol yang sudah ditahan tetap tidak diketahui hingga ditekan lagi; tombol yang diketik ke dalam kolom atau web view, atau selama metode input sedang menyusun, tidak pernah dianggap ditahan, dan tombol yang masih ditahan saat jendela kehilangan fokus dianggap dilepas saat itu. Pembacaan ini bekerja di tempat `OnKeyUp` bekerja dan di iPhone serta iPad dengan papan ketik perangkat keras.

Untuk tombol mouse, `View.OnMouseDown` dan `View.OnMouseUp` mengirimkan `core.MouseButtonEvent` untuk setiap tombol (`core.MouseButtonLeft`, `core.MouseButtonRight`, `core.MouseButtonMiddle`, `core.MouseButtonBack`, `core.MouseButtonForward`) di macOS, Windows, Linux, web, dan `nsgotest`, tanpa menjadikan tampilan sebagai hentian fokus papan ketik. `core.MouseButtonsDown(cx)` dan `core.MouseButtonHeld(cx, core.MouseButtonRight)` adalah pembacaan terlacak untuk tombol yang ditahan di jendela, yang dibersihkan otomatis saat jendela kehilangan fokus.

Untuk kontrol yang terikat pada posisi fisik, baca `KeyEvent.Code`, `core.KeyCodesDown(cx)`, atau `core.KeyCodeHeld(cx, "KeyW")`. Kode menggunakan nama `KeyboardEvent.code`: pada AZERTY, posisi W QWERTY memiliki `Code: "KeyW"` dan `Key: "z"`. `core.KeyCodeLabel(cx, "KeyW")` memberikan label tampilan tata letak aktif untuk sebuah petunjuk. Di iOS, Android, HarmonyOS, dan peramban tanpa peta tata letak, fungsi ini menggunakan nama tombol terakhir yang dilaporkan pada posisi tersebut dan mengembalikan string kosong hingga sebuah tombol terlihat. HarmonyOS menamai posisi dan tombol dari `KeyCode` InputKit, karena ArkUI tidak melaporkan scan code, sehingga di sana nama tombol belum mengikuti tata letak non-QWERTY.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if core.KeyHeld(cx, "left") {
		x.Update(func(x float32) float32 { return x - 4 })
	}
	if core.KeyHeld(cx, "right") {
		x.Update(func(x float32) float32 { return x + 4 })
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

## Menjaga layar tetap menyala

`cx.KeepAwake("Playing video", core.KeepAwakeOptions{Display: true})` menahan layar tetap menyala hingga fungsi pelepas yang dikembalikan dijalankan atau cakupan saat ini dibuang. Di Android, penahanan berlaku selama aplikasi terlihat. Di web, fitur ini bekerja dalam konteks aman dengan dukungan Screen Wake Lock, dan halaman meminta kunci lagi saat kembali terlihat. Safari dapat mensyaratkan tekanan atau tombol sebelum mengabulkan permintaan pertama. Penahanan khusus komputer tidak berpengaruh di kedua platform ini.

## Kunci penunjuk

Dalam kamera game atau seretan yang perlu berlanjut di tepi layar, panggil `ref.LockPointer()` dari tekanan pada tampilan yang memegang ref tersebut. Penunjuk tetap di tempatnya dan disembunyikan selama tampilan menerima gerakan relatif di `OnPointerDelta(dx, dy)`. Nilai positif bergerak ke kanan dan ke bawah.

Panggil fungsi yang dikembalikan untuk melepas kunci. Escape, jendela yang kehilangan fokus, atau menyembunyikan maupun menghapus tampilan juga melepasnya. Periksa `cx.Env().CanLockPointer()` untuk melihat apakah permukaan mendukungnya. Web memerlukan gestur pengguna seperti tekanan. Linux di Wayland memerlukan compositor yang mendukung batasan penunjuk dan gerakan relatif.

## Batas suara

Gunakan `audio.SetSoundLimits(cx, audio.SoundLimits{MaxSounds: 200, MaxDuration: 20 * time.Second})` sebelum memuat suara untuk mengubah batas engine. `MaxEncodedBytes`, `MaxDecodedBytes`, dan `MaxPlaybacks` juga dapat diatur. Kolom bernilai nol menggunakan default 8 MiB terenkode dan 5 detik per suara, 64 sumber, 16 MiB terdekode untuk semua suara, dan 32 pemutaran yang tumpang tindih per output. Suara yang sudah ada tetap dimuat dan dihitung terhadap batas baru. Cakupan yang dibatasi mempertahankan batas default dalam anggaran engine: suara `nsgo/audio` dari paket TSX yang ditandatangani terkena batas tersebut apa pun yang diatur aplikasi Go, sedangkan TSX milik aplikasi sendiri memakai batas aplikasi Go. SoundPool Android memutar paling banyak 32 suara sekaligus dan mendekode paling banyak 1 MiB dari masing-masing, apa pun batasnya.

## Layar penuh

`desktop.SetFullScreen(cx, true, displayID)` mengisi layar yang dinamai oleh `cx.Env().Displays()` di macOS, Windows, dan Linux. Hilangkan ID untuk menggunakan layar jendela saat ini. ID yang tidak dikenal membiarkan jendela di tempatnya. Mengulang permintaan yang sama tidak berpengaruh. `desktop.SetFullScreen(cx, false)` keluar dari layar penuh dan memulihkan frame jendela sebelumnya. Di web, masuklah dari handler tekan. Peramban memilih layarnya, dan `cx.Env().FullScreen()` mengikuti saat masuk dan keluar, termasuk Esc.

## Layar

`cx.Env().Display()` mengembalikan `core.DisplayInfo` jendela saat ini dan memperbarui binding saat jendela berpindah ke layar lain. `cx.Env().Displays()` mencantumkan layar yang dilaporkan platform. `RefreshRate` dan `MaxRefreshRate` dalam Hz, dengan nol untuk laju yang tidak tersedia. iOS melaporkan nilai maksimum panel. HarmonyOS melaporkan layar jendela, laju saat ini, dan laju tertinggi yang didukungnya. Web melaporkan layar saat ini dan menandai pengukuran callback frame-nya dengan `RefreshRateEstimated`. Dalam pengujian, atur `Displays` dan `DisplayID` melalui `nsgotest.Environment`.

## Membacakan teks

`cx.Speak` membacakan teks dengan suara sistem untuk sebuah bahasa jika `cx.Env().CanSpeak()` menyatakan platform dapat berbicara: macOS, iOS, tvOS, Android, Windows, web, dan Linux dengan speech-dispatcher. Hanya satu ujaran yang diucapkan pada satu waktu: `Speak` lain memotong ujaran yang sedang diucapkan, begitu pula `cx.StopSpeaking` dan berakhirnya cakupan yang mengucapkannya. `done` mendengar sekali bagaimana ujaran itu berakhir: selesai, dihentikan, atau gagal dengan `core.SpeakError`, seperti `core.SpeakLanguageUnavailable` saat tidak ada suara terpasang yang mengucapkan bahasa tersebut.

```go
cx.Speak("Guten Morgen", core.Voice{Lang: "de-DE", Rate: 0.8}, func(stopped bool, err error) {
	speaking.Set(false)
})
```

`Rate` dan `Pitch` berkisar dari 0.5 hingga 2, dengan 1 sebagai nilai asli suara. `cx.Voices("ja", ...)` mencantumkan suara yang terpasang untuk sebuah bahasa, sehingga aplikasi dapat meminta pengguna memasang suara yang belum ada atau membiarkan mereka memilih salah satunya, yang `ID`-nya dimasukkan ke `core.Voice`. Peramban hanya berbicara setelah pengguna menekan sesuatu di halaman, dan gagal dengan `core.SpeakNotAllowed` sebelum itu. Komponen Go dari paket TSX hanya dapat berbicara dengan kapabilitas `text_to_speech`.

## Audio berulang

Atur `audio.Voice.Loop` untuk mengulang pemutaran suara hingga `Stop` atau `Close`. Untuk musik yang di-stream, atur `audio.PlayerOptions.LoopStart` dan panggil `SetLoop(true)`. Putaran pertama mencakup intro. Putaran berikutnya dimulai dari offset. Offset nol mengulang seluruh trek.

Di macOS 14 dan iOS 17 atau lebih baru, trek terbatas menggunakan antrean native untuk menyiapkan setiap pengulangan sebelum pengulangan saat ini berakhir. Versi Apple sebelumnya melakukan seek ke awal loop saat putaran saat ini berakhir, dengan jeda singkat. Di Linux dan web, pemutar yang mengulang mendekode trek hingga 32 MiB dan 5 menit ke memori, sekitar 23 MB per menit, dan mengulangnya tanpa jeda. Di Linux, trek yang lebih besar atau lebih panjang di-stream melalui dekodernya sendiri dan juga diulang tanpa jeda; di web pemutar melakukan seek kembali di akhir, dengan jeda singkat. Di Windows dan Android, loop dengan titik awal melakukan seek kembali ke titik tersebut di akhir, dengan jeda singkat. Di web, pemutar yang mengulang pada laju selain 1 juga melakukan seek kembali di akhir, karena pemutaran tanpa jeda milik peramban akan menggeser nada mengikuti laju. Di web, pemutar yang mengulang, seperti suara, hanya diputar setelah pengguna menekan sesuatu di halaman, dan gagal dengan `NotAllowed` sebelum itu.

## Biometrik

`cx.BiometricAvailability` melaporkan apakah Face ID, Touch ID, atau sidik jari perangkat telah didaftarkan; Android tidak menyebutkan jenis mana yang didaftarkan, sehingga di sana fungsi ini melaporkan sidik jari jika ada yang didaftarkan dan selain itu wajah jika perangkat memiliki perangkat keras wajah. `cx.AuthenticateBiometric` meminta sistem untuk mengonfirmasi pengguna dengan sebuah alasan, yang tidak boleh kosong, dan melaporkan keberhasilan, kegagalan, pembatalan, atau ketidaktersediaan. Fungsi ini tidak memilihkan cadangan kode sandi untuk aplikasi.

```go
cx.BiometricAvailability(func(kind core.BiometricKind) {
	if kind != core.BiometricNone {
		cx.AuthenticateBiometric("Confirm deleting this account", func(result core.BiometricResult) {
			if result == core.BiometricSuccess {
				deleteAccount()
			}
		})
	}
})
```

Aplikasi Face ID mendeklarasikan `NSFaceIDUsageDescription` di bawah `apple.info`; aplikasi Android mencantumkan `USE_BIOMETRIC` di bawah `android.permissions`.

## Masuk akun

`cx.AuthSession` memasukkan pengguna dengan penyedia OAuth di sesi autentikasi sistem alih-alih web view: `ASWebAuthenticationSession` di macOS, iOS, dan watchOS, Custom Tab atau peramban di Android, peramban default di Windows, Linux, dan Mac tanpa skema, serta popup di web. Di web, halaman pengalihan juga mengirim callback-nya melalui `BroadcastChannel` dengan origin yang sama, sehingga proses masuk selesai ketika `Cross-Origin-Opener-Policy` milik penyedia memutus popup dari halaman. Popup seperti itu terbaca sebagai tertutup oleh halaman, sehingga menutup popup setelah menampilkan halaman penyedia mengakhiri sesi sebagai dibatalkan hanya setelah 5 menit; berikan tombol batal pada sesi. Aplikasi memegang protokolnya: aplikasi membuat verifier PKCE dan state, membangun URL otorisasi untuk pengalihan yang diberikan kepadanya, dan memeriksa callback sebelum menukarkan kodenya. Membatalkan bukanlah gagal: `errors.Is(err, core.AuthCancelled)` mengenali pengguna yang menutup sesi, atau `cancel` yang dikembalikan yang mengakhirinya.

```go
verifier, challenge, state := newPKCE() // the app's own
cx.AuthSession(func(redirect string) string {
	q := url.Values{"client_id": {clientID(redirect)}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid email"},
		"state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	return "https://accounts.example.com/authorize?" + q.Encode()
}, core.AuthSessionOptions{Scheme: "com.example.app", Path: "/oauth2redirect", Timeout: 5 * time.Minute}, func(callback string, err error) {
	if errors.Is(err, core.AuthCancelled) {
		return
	}
	if err != nil {
		showError(err)
		return
	}
	u, _ := url.Parse(callback)
	if u.Query().Get("state") == state {
		exchangeCode(u.Query().Get("code"), verifier)
	}
})
```

Pengalihannya adalah `com.example.app:/oauth2redirect` jika skema diatur (Windows, Linux, dan Android memerlukannya di `url_schemes` milik `nsgo.yaml` agar OS menyerahkan callback ke aplikasi yang sedang berjalan; sesi sistem macOS, iOS, dan watchOS menangkapnya tanpa deklarasi itu), dan `http://127.0.0.1:<port>/oauth2redirect` pada loopback desktop, jadi daftarkan keduanya pada penyedia, sering kali sebagai klien tersendiri. Biarkan `Scheme` kosong untuk loopback saja, atau atur ke `https` dengan `Host` untuk domain terkait aplikasi, yang ditangkap oleh iOS 17.4, macOS 14.4, dan watchOS 10.4. watchOS memerlukan skema atau domain HTTPS terkait; watchOS tidak memiliki cadangan loopback, dan batas minimum watchOS 26 yang sudah ada di NSGo tetap berlaku. Tab peramban yang ditutup tidak memberi tahu sesi loopback apa pun, jadi berikan `Timeout` atau tombol batal. `Ephemeral` meminta macOS, iOS, dan watchOS untuk tidak berbagi cookie peramban, dan meminta Custom Tab privat di Android jika peramban mendukungnya; jangan mengasumsikan isolasi cookie di Android. tvOS dan mini program untuk saat ini menjawab `core.AuthUnavailable`. Di TSX, `AuthSession.start(url, options)` memerlukan kapabilitas `auth_session` dan memberikan sesi yang `result`-nya di-resolve dengan callback, atau null setelah dibatalkan.

Android mengonsumsi callback yang cocok sebelum `OnOpenLinks` biasa, dan kembali tanpa callback mengakhiri sesi sebagai dibatalkan. Callback-nya harus menggunakan skema privat milik aplikasi; skema kosong, callback HTTPS, atau skema yang tidak dideklarasikan mengembalikan `core.AuthUnavailable`, begitu pula runtime Android yang dibangun sebelum adanya sesi masuk.

Sesi peramban hanya mengonsumsi tautan callback yang cocok; tautan yang tidak terkait dan yang terlambat tetap mencapai `OnOpenLinks`. Memulai sesi peramban lain untuk URI callback yang sama membatalkan sesi sebelumnya; URI yang berbeda dapat menunggu di jendela yang berbeda.

Gunakan pengalihan persis yang diberikan ke fungsi URL otorisasi. Pengalihan loopback web dan desktop mengenkode path Unicode dan me-resolve segmen titik sebelum fungsi menerimanya; `Path` loopback adalah teks terdekode, sehingga `%` literal juga di-escape.

Mini program menggunakan `cx.SignInMiniProgram(core.MiniProgramSignInOptions{}, done)` sebagai gantinya. Callback menerima `core.MiniProgramCredential` dengan `Platform`-nya (`MiniProgramWeChat`, `MiniProgramQQ`, `MiniProgramDouyin`, atau `MiniProgramAlipay`) dan `Code` berumur pendek yang tidak kosong. Kirim ke server Anda untuk penukaran kode milik host; simpan rahasia aplikasi di sana. Ini adalah `wx.login`/`qq.login`/`tt.login`, atau otorisasi dasar `auth_base` milik Alipay, tanpa meminta informasi profil. Fungsi ini tidak membuka URL atau menjalankan pembangun URL OAuth.

`cx.Env().CanSignInMiniProgram()` melaporkan ketersediaan API host. Target lain menjawab `core.MiniProgramSignInUnavailable`; kode tanpa `core.AccessMiniProgramSignIn` mendapat `AccessDenied`. Douyin bertanya secara diam-diam secara default: host yang belum masuk mengembalikan `MiniProgramSignInNotSignedIn`, dan `MiniProgramSignInOptions{Prompt: true}` mengizinkan jendela masuk akun host-nya. Menutup jendela itu mengembalikan `MiniProgramSignInCancelled`. Kode perangkat anonim tidak pernah dihitung sebagai pengguna yang sudah masuk. Pengujian mengatur `nsgotest.Environment(core.Environment{MiniProgramSignIn: true})` dan `app.MiniProgramSignIn`; `app.MiniProgramSignIns` mencatat permintaan. TSX tidak berjalan di mini program.

Aplikasi yang web view-nya harus masuk sebagai pengguna yang sudah masuk di tempat lain, seperti di peramban mereka sendiri, menulis cookie dari sesi masuk itu dengan `cx.SetWebCookies`, termasuk yang HttpOnly, yang tidak dapat diatur oleh skrip halaman, lalu memuat halaman: done berjalan setelah penyimpanan menampungnya. Penyimpanan tersebut adalah penyimpanan persisten yang dibagikan oleh setiap web view aplikasi, sehingga cookie dengan `Expires` bertahan lebih lama daripada aplikasi; tampilan Incognito tidak melihatnya, kecuali di Android, yang menyimpan satu penyimpanan per aplikasi. HarmonyOS, web, tvOS, dan watchOS menjawab `Unavailable`.

```go
cx.SetWebCookies([]core.WebCookie{
	{Name: "SID", Value: sid, Domain: ".example.com", Expires: expires, Secure: true, HTTPOnly: true},
}, func(err error) {
	if err == nil {
		page.Set(core.WebSrc("https://www.example.com/"))
	}
})
```

Untuk menghapus satu cookie, tulis lagi dengan nama, domain, dan path yang sama serta `Expires` di masa lalu; kedaluwarsa nol membuat cookie sesi. Tunggu callback sebelum memuat halaman yang seharusnya melihat penghapusan tersebut.

Sebaliknya, `cx.WebCookies` mencantumkan cookie di penyimpanan, termasuk yang HttpOnly beserta domain, path, kedaluwarsa, dan flag-nya, yang tidak dapat dibaca oleh `document.cookie`, terlepas dari apakah halaman situs tersebut sedang ditampilkan atau tidak: `core.WebCookieFilter` memilih cookie yang akan dikirim oleh permintaan ke sebuah `URL`, atau cookie milik sebuah `Domain` beserta setiap host di bawahnya, dan filter nol memilih semuanya. Aplikasi yang memanggil situs dari Go dengan sesi masuk pengguna, atau yang menampilkan di mana pengguna sudah masuk, membacanya dengan cara ini. `cx.ClearWebsiteData` mengeluarkan pengguna dari setiap situs: fungsi ini menghapus cookie penyimpanan, penyimpanan lokal dan sesi, IndexedDB, service worker, dan cache, lalu done berjalan setelah semuanya hilang. `CookieManager` Android hanya menjawab nama dan nilai yang akan dikirim oleh sebuah URL, sehingga di sana hanya filter `URL` yang mencantumkan, dengan mengisi `Domain` dengan host-nya dan membiarkan kolom lain kosong, sedangkan filter `Domain` atau filter nol menjawab `Unavailable`; pembersihan di sana menyisakan cache HTTP. HarmonyOS, web, tvOS, dan watchOS menjawab `Unavailable` untuk keduanya.

```go
cx.WebCookies(core.WebCookieFilter{Domain: "example.com"}, func(cookies []core.WebCookie, err error) {
	signedIn.Set(err == nil && slices.ContainsFunc(cookies, func(c core.WebCookie) bool { return c.Name == "SID" }))
})
```

## Sign in with Apple

`cx.SignInWithApple` menampilkan sheet Sign in with Apple milik sistem dari tombol milik aplikasi sendiri di macOS, iOS, dan tvOS, di mana `cx.Env().CanSignInWithApple()` bernilai true. Fungsi ini menjawab sekali dengan kredensial, atau dengan galat: `core.AppleSignInCancelled` saat pengguna menutup sheet, yang bukan kegagalan, selain itu `AppleSignInFailed` atau, di platform lain, `AppleSignInUnavailable`. Apple memberikan email dan nama hanya pada saat pengguna pertama kali masuk ke aplikasi, jadi simpanlah saat itu. `Nonce` dimasukkan ke dalam permintaan sebagai SHA-256-nya dalam hex huruf kecil, yang dimuat oleh klaim `nonce` pada token identitas: server melakukan hash pada nonce yang diberikannya kepada aplikasi lalu membandingkannya. `cx.AppleCredentialState` menyatakan apakah pengguna telah mencabutnya sejak itu. Dalam pengujian headless, `app.AppleSignIn` menerima `nsgotest.AppleSignInRequest` dan menyediakan kredensial atau galat.

```go
nonce := rand.Text() // also sent to the server, which checks the token's claim against its SHA-256
cx.SignInWithApple(core.AppleSignInOptions{FullName: true, Email: true, Nonce: nonce}, func(c core.AppleCredential, err error) {
	switch {
	case errors.Is(err, core.AppleSignInCancelled):
	case err != nil:
		showError(err)
	default:
		signInOnServer(c.IdentityToken, c.AuthorizationCode, nonce, c.GivenName, c.FamilyName, c.Email)
	}
})
cx.AppleCredentialState(savedUser, func(s core.AppleCredentialState, err error) {
	if err == nil && s == core.AppleCredentialRevoked {
		signOut()
	}
})
```

Aplikasi harus ditandatangani dengan entitlement Sign in with Apple, yang ditambahkan oleh `build` saat `nsgo.yaml` memintanya; tanpanya sheet akan gagal:

```yaml
apple:
  sign_in_with_apple: true  # com.apple.developer.applesignin, in iOS and tvOS packages and Mac App Store builds
```

Xcode mendaftarkan kapabilitas ini untuk build perangkat dari `nsgo dev`; profil provisioning di `sign.profile` atau `sign.macos_profile` harus berasal dari App ID dengan Sign in with Apple diaktifkan. macOS hanya memberikannya dengan profil, sehingga disk image atau `nsgo dev` yang dijalankan di Mac tidak dapat melakukan proses masuk, begitu pula simulator, yang build-nya tidak mengklaim entitlement pengembang apa pun.

## Pembelian dalam aplikasi

`cx.Products`, `cx.Purchase`, dan panggilan-panggilan di sampingnya menjual melalui App Store di macOS, iOS, dan tvOS, di mana `cx.Env().CanPurchase()` bernilai true, dalam build yang `nsgo.yaml`-nya meminta StoreKit:

```yaml
apple:
  purchases: true  # StoreKit's library, in macOS, iOS and tvOS packages; no entitlement is needed
```

Tampilkan harga yang diberikan toko, `DisplayPrice`, jangan pernah harga milik aplikasi sendiri: toko menetapkan harga untuk setiap storefront sendiri. Pembelian menjawab sekali, dengan transaksi atau galat: `core.PurchaseCancelled` saat pengguna menutup sheet, `core.PurchasePending` selama menunggu persetujuan, seperti pada Ask to Buy, dan selain itu `core.PurchaseFailed`, dengan pesan dari toko, atau `PurchasesUnavailable` di tempat lain. Kirim `Signed` milik transaksi, yaitu JWS dari toko, ke server aplikasi untuk diverifikasi, serahkan apa yang dibelinya, lalu selesaikan transaksinya; toko menyerahkan kembali transaksi yang belum selesai ke aplikasi pada setiap peluncuran.

`cx.ManageSubscriptions` membuka UI pengelolaan langganan Apple (sheet sistem di iOS, halaman langganan App Store di macOS dan tvOS). `cx.RequestRefund` memulai permintaan pengembalian dana untuk sebuah transaksi dan menjawab `core.RefundSuccess` atau `core.RefundCancelled`.

```go
cx.OnPurchaseUpdate(func(t core.Transaction) { // renewals, approved purchases, other devices', refunds, and unfinished ones at launch
	verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
})
cx.Products([]string{"com.example.pro.monthly"}, func(products []core.Product, err error) {
	if err == nil && len(products) > 0 {
		price.Set(products[0].DisplayPrice)
	}
})
cx.Purchase("com.example.pro.monthly", core.PurchaseOptions{AccountToken: userUUID}, func(t core.Transaction, err error) {
	switch {
	case errors.Is(err, core.PurchaseCancelled):
	case errors.Is(err, core.PurchasePending):
		showWaiting() // its transaction comes through OnPurchaseUpdate
	case err != nil:
		showError(err)
	default:
		verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
	}
})
```

`FinishTransaction` mengembalikan `PurchaseNotFound` untuk transaksi yang sudah selesai, termasuk transaksi yang dikembalikan oleh `CurrentEntitlements`; item non-consumable yang sudah selesai tetap dimiliki.

Daftarkan `OnPurchaseUpdate` saat aplikasi dimulai: pembaruan yang datang sebelum ada handler menunggu handler pertama. `cx.CurrentEntitlements` menjawab apa yang dimiliki pengguna saat ini, untuk membukanya saat peluncuran, dan tombol Restore Purchases memanggil `cx.SyncPurchases`, yang dapat meminta pengguna untuk masuk. `AccountToken` adalah UUID milik aplikasi sendiri yang dibawa oleh transaksi dan notifikasi server App Store. macOS hanya menjual dalam build Mac App Store, dan App Clip tidak menjual apa pun; Android, watchOS, Windows, Linux, web, dan mini program untuk saat ini menjawab `PurchasesUnavailable`. Uji pembelian dengan `nsgotest`, yang `Products`, `PurchaseAnswer`, `UpdatePurchase`, dan `UnfinishedTransactions`-nya menggantikan toko. `app.PurchaseRequests` mencatat operasi toko sebagai nilai `nsgotest.PurchaseRequest`. Di TSX, modul `Purchases` memerlukan kapabilitas `purchases`: `purchase` di-resolve dengan `{ status: "purchased", transaction }`, `"pending"`, atau `"cancelled"`, dan `on("update", listener)` mendengar pembaruan.

## Orientasi antarmuka

Panggil `cx.RequestOrientation(core.OrientationLandscape)` dalam cakupan pemutar layar penuh untuk meminta lanskap kepada iOS, Android, HarmonyOS, atau peramban. Permintaan terbaru yang cakupannya masih hidup akan berlaku. Saat cakupan itu berakhir, permintaan sebelumnya dilanjutkan atau aplikasi kembali ke kebijakan orientasi biasanya.

Di iOS 16 dan yang lebih baru, permintaan dapat langsung memutar antarmuka. Di versi iOS yang lebih lama, permintaan membatasi rotasi sistem berikutnya. `IOS.SupportedOrientations` milik modal yang ditampilkan lebih diutamakan. Di Android dan HarmonyOS, campuran arah potret dan lanskap yang tidak dapat direpresentasikan oleh sistem dikurangi menjadi satu arah yang diizinkan, dengan mengutamakan arah yang sudah ditampilkan. Android dapat mengabaikan permintaan orientasi dalam mode layar besar atau multi-jendela.

Di aplikasi web, panggil `desktop.ToggleFullScreen(cx)` dari handler tekan yang membuka cakupan pemutar. Peramban masuk ke layar penuh dari tekanan tersebut, lalu NSGo menerapkan permintaan orientasi pemutar. Keluar dari layar penuh melepas kunci. Konsol peramban melaporkan operasi yang tidak didukung dan penolakan.

`nsgotest.App.Orientation` mencatat mask terbaru yang diminta, dengan nol untuk kebijakan orientasi biasa.

## Ulasan toko

`cx.RequestReview()` meminta prompt penilaian dalam aplikasi dari App Store di macOS dan iOS, atau Google Play di Android. `cx.Env().CanRequestReview()` melaporkan apakah layanan tersedia. Toko memutuskan apakah dan kapan prompt ditampilkan dan tidak melaporkan hasil apa pun. Gunakan build pengembangan untuk menguji prompt Apple. Build TestFlight menekannya.

Paket Android mandiri menyertakan SDK Play Review, activity dialognya, dan kueri visibilitas Play Store; membangunnya tidak memerlukan Android SDK atau JDK. Perangkat tanpa Play Store yang aktif melaporkan layanan sebagai tidak tersedia. Uji paket yang dirilis menggunakan [Internal App Sharing](https://developer.android.com/guide/playcore/in-app-review/test).

Saat menyematkan NSGo, tambahkan `com.google.android.play:review:2.0.2` ke host Gradle. Di manifes host, tambahkan `<package android:name="com.android.vending" />` di bawah `<queries>` agar aplikasi dapat melihat Play Store. Jika host menyusutkan kodenya, pertahankan API Play Review dan Tasks dengan aturan di bawah ini, karena NSGo memuatnya berdasarkan nama.

```proguard
-keep class com.google.android.play.core.review.ReviewManagerFactory { *; }
-keep interface com.google.android.play.core.review.ReviewManager { *; }
-keep class com.google.android.play.core.review.ReviewInfo { *; }
-keep class com.google.android.gms.tasks.Task { *; }
-keep interface com.google.android.gms.tasks.OnCompleteListener { *; }
```

Dalam pengujian, atur `CanRequestReview` di `nsgotest.Environment` dan periksa `nsgotest.App.ReviewRequests`.

## Gestur tepi sistem

Game dengan kontrol sentuh di tepi layar dapat memanggil `mobile.DeferSystemGestures` selama kontrol aktif. Gunakan fungsi yang dikembalikan untuk melepas permintaan lebih awal. Permintaan juga berakhir saat cakupannya dibersihkan. Permintaan dari beberapa cakupan di jendela yang sama digabungkan.

```go
release := mobile.DeferSystemGestures(cx, mobile.SystemGestureOptions{
	IOSEdges: mobile.GestureEdgeBottom,
	AndroidRects: []core.Bounds{{
		Origin: core.Point{Y: 120},
		Size:   core.Size{Width: 24, Height: 96},
	}},
})
```

iOS menggunakan tepi layar fisik. Android 10 dan yang lebih baru menggunakan persegi panjang dalam point permukaan yang diukur dari kiri atas. Jaga agar persegi panjang tetap di sekitar kontrol yang membutuhkannya. Android mencadangkan gestur home-nya dan membatasi area yang dapat dikecualikan aplikasi dari Back. `nsgotest.App.SystemGestures` memberi pengujian permintaan gabungan saat ini pada permukaan utamanya. Gunakan `Surface.SystemGestures()` untuk jendela lain.

## Overlay di Android

`mobile.OpenOverlay` menampilkan tampilan milik aplikasi di atas aplikasi lain di Android, sebuah pohon tersendiri yang berbagi signal aplikasi, dan layanan foreground menjaga aplikasi tetap berjalan selama overlay tampil. Aplikasi mencantumkan `SYSTEM_ALERT_WINDOW` di bawah `android.permissions`, dan pengguna memberikannya di pengaturan sistem, yang dibuka oleh `mobile.RequestOverlayPermission`; tanpanya overlay langsung berakhir dan `OnEnd` mendengar alasannya. Overlay menerima sentuhan tetapi menyerahkan tombol dan sentuhan di sekitarnya kepada aplikasi di depan, kecuali overlay bersifat `Focusable` atau `Passive`, dan berpindah dengan `desktop.SetWindowFrame` dari kontennya. Platform lain tidak membuka overlay apa pun.

```go
mobile.RequestOverlayPermission(cx, func(p core.OverlayPermission) {
	if p == mobile.OverlayGranted {
		mobile.OpenOverlay(cx, mobile.OverlayOptions{Width: 200, Height: 80}, Score)
	}
})
```

## Lokasi dan gerakan

`cx.Locate` mendapatkan satu titik lokasi (*fix*), `cx.WatchLocation` mengalirkan titik lokasi, dan `cx.WatchMotion` mengalirkan sampel akselerometer, giroskop, magnetometer, serta gerakan perangkat gabungan. Masing-masing mengembalikan `*core.SensorStream` yang `Stop`-nya membatalkannya, sebagaimana berakhirnya cakupan yang memulainya; callback berjalan di goroutine engine. Izinnya adalah `core.PermissionLocation`, `PermissionBackgroundLocation`, dan `PermissionMotion` ([izin](permissions.md)): memulai stream tidak pernah meminta izin, dan stream tanpa izinnya gagal dengan `core.SensorNotAllowed`.

```go
cx.RequestPermission(core.PermissionLocation, func(s core.PermissionStatus) {
	if !s.Allowed() {
		return
	}
	cx.Locate(core.LocationOptions{}, func(f core.LocationFix, err error) {
		if err == nil {
			showPosition(f.Latitude, f.Longitude, f.Approximate)
		}
	})
})
```

`Locate` menunggu titik lokasi baru, 30 detik secara default (`Timeout`); hanya `MaxAge` yang memungkinkannya mengambil titik lokasi yang di-cache, yang menyatakannya dengan `Cached` dan `Age`. `Approximate` menandai titik lokasi yang hanya diizinkan pengguna secara perkiraan; Android menyegarkan titik lokasi seperti itu paling sering setiap sepuluh menit, jadi berikan `MaxAge` kepada `Locate` untuknya di sana. Koordinat menggunakan WGS 84; pengukuran yang tidak dimiliki penyedia memiliki `Valid` false, dan `Altitude` bersifat elipsoidal sedangkan `MSLAltitude` diukur di atas permukaan laut rata-rata. Gerakan menggunakan sumbu alami perangkat, akselerasi dalam m/s² (+9.81 pada z saat berbaring menghadap ke atas), rotasi dalam rad/s, dan medan magnet dalam µT, setiap 20 ms secara default, dibatasi pada 10 ms–1 s. Di bawah beban, stream menyimpan sampel terbaru per sensor dan menghitung sampel yang digantikannya di `Dropped`.

Stream terus berjalan selama aplikasi tidak aktif dan dijeda di latar belakang, dengan menandai sampel pertama setelahnya dengan `Gap`; `Locate` yang tertunda mempertahankan sisa batas waktunya. `LocationOptions.Background` menjaga stream lokasi tetap berjalan di latar belakang di bawah `PermissionBackgroundLocation`: aplikasi Apple menambahkan mode latar belakang `location`, dan di Android layanan foreground menampilkan notifikasi dengan `NotificationTitle` dan `NotificationBody` serta menjaga aplikasi tetap berjalan setelah activity-nya berakhir. Tidak ada yang memulai proses yang telah diakhiri oleh sistem.

```yaml
apple:
  info:
    NSLocationWhenInUseUsageDescription: Shows where you are on the map.
    NSLocationUsageDescription: Shows where you are on the map.  # macOS
    NSMotionUsageDescription: Counts your reps.
    UIBackgroundModes: [location]                                # background streams
android:
  permissions: [ACCESS_COARSE_LOCATION, ACCESS_FINE_LOCATION, FOREGROUND_SERVICE, FOREGROUND_SERVICE_LOCATION]
```

iOS dan watchOS memberikan lokasi dan gerakan, macOS memberikan lokasi, tvOS satu titik lokasi pada satu waktu, dan Android keduanya melalui penyedia milik framework sendiri. Linux mendapatkan titik lokasi dan stream dari GeoClue 2 di bus sistem, yang agennya bertanya kepada pengguna saat permintaan pertama aplikasi dimulai; karena GeoClue tidak dapat ditanya tanpa memunculkan prompt, izin terbaca sebagai belum ditanyakan hingga aplikasi bertanya dalam proses berjalan tersebut, dan lokasi latar belakang tidak tersedia. Windows mendapatkannya dari `Geolocator` milik WinRT; izin terbaca sebagai ditolak selama lokasi dimatikan untuk aplikasi, sebagai diberikan setelah aplikasi bertanya dalam proses berjalan tersebut dan diizinkan, dan selain itu sebagai belum ditanyakan, dan lokasi latar belakang juga tidak tersedia di sana. Peramban yang aman menyediakan titik lokasi geolokasi dan, di perangkat seluler, pembacaan akselerometer, giroskop, dan gerakan gabungan. Panggil `cx.RequestPermission(core.PermissionMotion, done)` dari handler tekan di Safari. Peramban melaporkan lokasi tidak tersedia tanpa `navigator.geolocation`, dan gerakan tidak tersedia di desktop serta untuk stream magnetometer. Di TSX, `Sensors` memiliki `permission`, `requestPermission`, `locate`, `watchLocation`, dan `watchMotion`, di bawah kapabilitas paket `location`, `backgroundLocation`, dan `motion`, dalam milidetik.

## Audio

Paket `audio` memutar suara pendek yang didekode sekali ke memori (`audio.LoadSound`) dan audio panjang yang di-stream (`audio.NewPlayer`) jika `cx.Env().CanPlayAudio()` bernilai true; di tempat lain panggilan gagal dengan `audio.Unavailable`.

```go
audio.LoadSound(cx, audio.Source{URI: "asset:sounds/click.wav"}, func(click *audio.Sound, err error) {
	if err != nil {
		return
	}
	click.Play(audio.Voice{})                                  // center, full volume, normal rate
	click.Play(audio.Voice{Volume: 0.5, Rate: 1.2, Pan: -0.5}) // overlaps, panned left
})
```

`audio.Voice` mengatur `Volume` per pemutaran (0 hingga 1, 0 dianggap 1), `Rate` (0.5 hingga 2, 0 dianggap 1), dan `Pan` (-1 penuh ke kiri hingga 1 penuh ke kanan, 0 di tengah); nilai di luar rentang tersebut dijepit dan NaN memakai nilai default. `Sound.Stop` menghentikan semua pemutaran suara tersebut dan `Close` membebaskannya, sebagaimana dilakukan cakupan pemiliknya saat dibuang. `Player` mendukung `Play`, `Preload`, `Pause`, `Seek`, `SetVolume`, `SetRate`, `SetLoop`, dan `Close`. macOS, iOS, Windows, Linux, Android, dan web sudah memutar audio saat ini. Backend Apple, Windows, dan peramban mendukung WAV, AAC, dan MP3. Linux menggunakan codec GStreamer, dan pemutar stream-nya memutar pada 1×. Android menjeda pemutar saat tampilannya meninggalkan layar dan membatasi setiap suara terdekode hingga 1 MiB. tvOS, watchOS, HarmonyOS, dan mini program masih melaporkan `CanPlayAudio` false.

Peramban memerlukan gestur pengguna untuk pemutaran yang terdengar. Muat suara pendek lebih awal, lalu panggil `Sound.Play` setelah klik, ketukan, atau penekanan tombol. Suara yang diputar sebelum gestur akan senyap. Callback `Play` milik pemutar menerima `audio.NotAllowed` jika peramban memblokir pemutaran. Coba lagi dari gestur pengguna.

`Sound.Play` mengembalikan `audio.Playback`, yang di macOS, iOS, Windows, Linux, dan web dapat mengatur volume dan lajunya sendiri atau berhenti secara independen. `Ended()` bersifat reaktif dan menjadi true saat pemutaran itu selesai, berhenti, tergeser oleh batas suara, atau tidak dapat dimulai. Backend tanpa kontrol individual mengembalikan handle yang sudah berakhir.

Di iOS, engine audio milik pustaka sendiri yang memutar di luar `audio` dan `Video`, seperti `AudioQueue`, memegang satu-satunya sesi audio aplikasi dengan `audio.HoldSession(core.AudioSessionPlayback)` selama memutar dan memanggil pelepas yang dikembalikan saat berhenti. Fungsi ini menerima tingkat yang sama dengan `AudioSession` milik pemutar, di samping video, pemutar, dan rekaman aplikasi, sehingga `AudioSessionPlayback` dan `AudioSessionMixWithOthers` tetap memutar dengan sakelar Ring/Silent dalam posisi senyap. Di tempat lain, fungsi ini tidak memegang apa pun.

Gunakan `audio.Voice{Muted: true}` untuk memulai efek tanpa suara dan `playback.SetVolume(0)` untuk membisukan efek yang sedang aktif. Untuk audio panjang, `audio.NewPlayer` men-stream sebuah sumber dan `player.SetMuted(true)` membisukan output selama pemutaran berlanjut. Membatalkan bisu memulihkan volume yang dikonfigurasi pada pemutar. `Voice.Volume` dan `Player.SetVolume` menggunakan nol untuk volume penuh default.

Di TSX, `nsgo/audio` menyediakan `supported()` dan `loadSound(uri)`, yang dibatasi oleh kapabilitas paket `audio`. `loadSound` di-resolve dengan suara yang `play({ volume, rate, pan, loop, muted })`-nya mengembalikan pemutaran dengan `setVolume`, `setRate`, `stop`, dan `ended` yang reaktif; di sana, volume 0 berarti senyap. Pemuatan yang ditolak di-reject dengan `Error` yang `reason`-nya adalah nama galat audio, seperti `quota-exceeded`, dan yang `code`-nya adalah `ERR_ACCESS_DENIED` tanpa kapabilitas, `ERR_NSH_QUOTA_EXCEEDED` jika melewati batas, dan `ERR_FEATURE_UNAVAILABLE_ON_PLATFORM` jika tidak ada yang dapat memutar audio. Suara hanya dilepas oleh `close()` atau saat tampilannya hilang; di dalam sandbox, setiap pemuatan dihitung terhadap `Limits.Sounds` (256 secara default) hingga pemuatan gagal atau suaranya ditutup.

## Gamepad

Paket `gamepad` membaca pengontrol game yang terhubung dalam tata letak W3C Standard Gamepad (`ButtonSouth`/`ButtonA` hingga `ButtonHome`, `AxisLeftX`/`AxisLeftY`/`AxisRightX`/`AxisRightY`). `cx.Env().SupportsGamepads()` melaporkan apakah target mendukung pengontrol (macOS, iOS, Windows, Linux, Android, dan peramban dengan Gamepad API).

`gamepad.Pads(cx)` adalah pembacaan reaktif terlacak yang merender ulang cakupan saat pengontrol terhubung atau terputus tanpa merender ulang pada perubahan tombol atau stik. Setiap `gamepad.Pad` menyediakan `Family`-nya (`FamilyXbox`, `FamilyPlayStation`, `FamilyNintendo`, atau `FamilyGeneric`, juga diklasifikasikan melalui `gamepad.DetectFamily`), `VendorID` dan `ProductID` USB jika platform melaporkannya, serta `CanRumble` untuk dukungan motor haptik. Di dalam loop `RequestAnimationFrame`, `gamepad.State(cx, id)` mengembalikan tombol pengontrol (`[0, 1]`, dengan `Pad.Pressed` bernilai true pada `>= 0.5`) dan sumbu (`[-1, 1]`, negatif ke kiri/atas dan positif ke kanan/bawah) yang di-snapshot sekali per frame layar.

`gamepad.Rumble(cx, id, lowFreq, highFreq, duration)` menggerakkan motor getar frekuensi rendah (berat/kiri) dan frekuensi tinggi (ringan/kanan) dengan intensitas yang dijepit ke `[0, 1]`, menggantikan getaran yang sedang berjalan pada pengontrol tersebut dan berhenti otomatis setelah `duration`, saat aplikasi masuk ke latar belakang atau kehilangan fokus (`cx.Env().Lifecycle() != core.LifecycleActive`), saat `cx` dibuang, atau saat pengontrol terputus. `gamepad.StopRumble(cx, id)` menghentikan getaran lebih awal; pengontrol dengan `CanRumble == false` mengabaikan panggilan getar dengan rapi.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if pad, ok := gamepad.State(cx, 0); ok {
		move(pad.Axes[gamepad.AxisLeftX], pad.Axes[gamepad.AxisLeftY], pad.Pressed(gamepad.ButtonA))
		if hit {
			gamepad.Rumble(cx, pad.ID, 0.8, 0.3, 180*time.Millisecond)
		}
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

Panggil `gamepad.EnableFocusNavigation(cx)` untuk ikut serta dalam navigasi hentian fokus native dengan pengontrol (D-pad dan stik kiri memindahkan fokus, `ButtonSouth` mengaktifkan kontrol yang terfokus, dan `ButtonEast` memicu kembali). Tampilan gameplay aktif memanggil `gamepad.CaptureInput(cx)` untuk menjeda navigasi fokus selama tampilan tersebut terpasang. Di TSX, `nsgo/gamepad` menyediakan `supported()`, `pads()`, `state(id)`, `Button`, dan `Axis`, yang dibatasi oleh kapabilitas paket `gamepad`.

## Pengaturan waktu audio

Gunakan `audio.Now(cx)` sebagai jam untuk pemutaran berwaktu. Atur `audio.Voice.StartAt` ke waktu di masa depan saat `audio.CanSchedule(cx)` bernilai true. Panggil `audio.OutputLatency(cx, func(latency time.Duration, known bool) { ... })` untuk rute output yang aktif. macOS, iOS, Linux, dan web mendukung awal berwaktu; di Windows dan Android `CanSchedule` bernilai false dan `StartAt` langsung diputar. Nilai `known` pada callback menyatakan apakah rute melaporkan latensi, yang tidak dilakukan oleh Windows dan Android.

## Layanan game

Paket `gameservices` menyediakan API netral penyedia untuk masuk pemain, pencapaian, papan peringkat, dan overlay native Game Center / Play Games jika `cx.Env().CanUseGameServices()` bernilai true. Setiap callback berjalan di goroutine engine sehingga dapat mengatur signal secara langsung tanpa memblokir frame.

```go
if cx.Env().CanUseGameServices() {
	gameservices.Authenticate(cx, func(p gameservices.Player, err error) {
		if err == nil && p.Authenticated {
			playerName.Set(p.DisplayName)
			gameservices.ReportAchievement(cx, "first-run", 100, nil)
			gameservices.SubmitScore(cx, "high-scores", 1250, nil)
		}
	})
}
```

`gameservices.ReportAchievement` menjepit progres ke `0..100` (`100` membuka pencapaian). `gameservices.LoadScores` membaca baris `gameservices.ScoreEntry` yang berperingkat untuk `Global` atau `Friends` selama `AllTime`, `Week`, atau `Today`, dan `gameservices.ShowAchievements` serta `gameservices.ShowLeaderboard` membuka overlay native platform. macOS, iOS, dan tvOS menjangkau GameKit melalui purego; Android akan menjangkau Play Games Services melalui hook runtime Java yang belum ada di runtime, sehingga untuk saat ini melaporkan `CanUseGameServices() == false`; Windows, Linux, web, watchOS, dan mini program melaporkan `CanUseGameServices() == false` dan menjawab `gameservices.ErrUnsupported`.

Dalam pengujian, aktifkan layanan game dengan `nsgotest.Environment(core.Environment{CanUseGameServices: true})` dan kendalikan tiruan terisolasi setiap aplikasi dengan `App.SetGameServicesPlayer`, `App.SetGameServicesFriends`, `App.SeedLeaderboardScores`, `App.SetGameServicesError`, `App.GameServicesAchievements`, `App.GameServicesScores`, `App.GameServicesShownAchievements`, dan `App.GameServicesShownLeaderboards`.

## Selanjutnya

- [Notifikasi](notifications.md): izin, lencana, dan tekanan.
- [Formulir](forms.md): kolom teks dan papan ketik.
- [Penyematan](embedding.md): layanan yang diberikan aplikasi host kepada aplikasi yang disematkannya.
- [Membangun dan merilis](building.md): izin dan file yang dibutuhkan sebuah paket.

### Kebugaran, wilayah, dan gerakan terbatas

`cx.WatchSteps`, `cx.WatchActivity`, dan `cx.WatchPressure` mengembalikan stream yang tercakup. Mintalah `core.PermissionActivityRecognition` sebelum langkah/aktivitas, dan sebelum tekanan udara di Apple. Apple menyediakan ketiganya melalui Core Motion; Android menyediakan penghitung langkah dan tekanan udara, serta melaporkan klasifikasi aktivitas tidak tersedia. Hitungan merupakan milik jendela `Since` pada sampel; jendela yang dilanjutkan membawa `Gap`. Tekanan udara dalam pascal, dengan ketinggian relatif opsional dalam meter. Jarak, jumlah lantai, dan ketinggian yang tidak ada memiliki `Valid` false.

`cx.WatchGeofences` menerima `core.GeofenceOptions` dengan satu hingga dua puluh lingkaran `GeofenceRegion` yang berbeda (koordinat WGS 84 dan radius dalam meter). Fungsi ini melaporkan `GeofenceInside`, `GeofenceOutside`, atau `GeofenceUnknown` per wilayah. `Background` memerlukan akses/deklarasi lokasi latar belakang dan otorisasi Always dari Apple; Android juga memerlukan judul notifikasi. Stop atau pembuangan menghapus pendaftarannya; tidak ada langganan yang bertahan setelah proses mati.

`cx.RequestPreciseLocation("route", done)` secara eksplisit meminta otorisasi presisi dan menjawab dengan boolean, tidak pernah dengan posisi. Di `apple.info`, deklarasikan `NSLocationTemporaryUsageDescriptionDictionary: {route: Explains why this feature needs precision.}`. Android memerlukan izin kasar (*coarse*) dan halus (*fine*). Jawaban false masih dapat membiarkan lokasi perkiraan tetap diizinkan.

`cx.WatchBackgroundMotion(core.BackgroundMotionOptions{Motion: core.MotionOptions{}, Duration: 20 * time.Second, NotificationTitle: "Recording motion"}, sample, failed)` meminta stream langsung yang terbatas waktunya. Fungsi ini memerlukan `AccessMotion` dan `AccessBackgroundMotion`; paket TSX mendeklarasikan `motion` dan `backgroundMotion`. iOS dapat mengakhiri sewa eksekusinya lebih cepat. Aplikasi Android mengatur `android.background_motion: true` dan mendeklarasikan `FOREGROUND_SERVICE`; layanan non-sticky berhenti dalam tiga menit meskipun durasi yang diminta lebih lama. Berakhirnya durasi atau kedaluwarsa oleh OS melaporkan `SensorTimedOut`. Stop/pembuangan mengakhiri penyedia dan sewanya; tidak ada yang direkam setelah penghentian. Target lain melaporkan tidak tersedia.

`Sensors` TSX mencerminkan keenam metode ini dan menggunakan milidetik. Pengujian headless memasok `FeedSteps`, `FeedActivity`, `FeedPressure`, dan `FeedGeofence`, serta mengatur `PreciseLocationAnswer` untuk permintaan akurasi. Perilaku kebugaran fisik, pemicu bangun oleh wilayah (*region wake-up*), dan latar belakang yang ditangguhkan masih memerlukan uji penerimaan di perangkat.

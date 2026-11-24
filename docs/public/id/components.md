---
title: Komponen dan state
description: Elemen, komponen, signal, dan gaya.
group: Konsep inti
order: 50
---

## Elemen

Layar adalah sebuah pohon elemen, yang merupakan nilai struct Go. `core` berisi primitifnya: `View`, `Text`, `Image`, `Scroll`, `Input`, dan lainnya. `ui` berisi kontrol yang digunakan aplikasi, seperti `Button`, `Select`, `DatePicker`, dan `EmojiPicker`, dengan tampilan yang mengikuti tema. (`base` menyimpan kontrol yang sama tanpa tampilan, untuk [pembuat komponen](authoring/index.md).)

```go
core.View{
	Style: core.Style{Padding: core.All(24), Gap: 12},
	Children: []core.Node{
		core.Text{Content: "Hello"},
	},
}
```

`core.WebView` menampilkan konten web; `WebSource` kosong memuat `about:blank` dan melaporkan event pemuatan serta navigasi seperti biasa. Atur `SourceFunc` ke sumber kosong atau panggil `Ref.Load(core.WebSource{})` untuk membersihkan halaman sebelumnya. Atur `InjectedJavaScriptBeforeContentFunc` ketika skrip sebelum skrip milik halaman mengikuti sebuah signal: saat berubah, view memuat ulang sumbernya dengan skrip baru, menggantikan halaman yang ditampilkan, bahkan halaman yang dinavigasi pengguna atau `Ref.Load`. Atur `UserAgentSuffix` (TSX `userAgentSuffix`) untuk menambahkan token, seperti `Version/26.0 Safari/605.1.15`, setelah user agent milik platform, atau setelah `UserAgent` jika diatur; di macOS dan iOS akhiran pada user agent milik platform hanya berlaku saat view pertama kali dibuat, sedangkan HarmonyOS dan web tetap memakai user agent-nya sendiri. Pasang `core.Ref` untuk memanggil `Reload`, `GoBack`, dan `GoForward`. Pada target native, perintah ini menjaga HTML dan sumber data milik aplikasi tetap dikecualikan dari `core.AccessWebSchemes`; halaman yang membuat navigasi baru tetap mengikuti kebijakan akses.

Di tvOS, `cx.Env().HasWebView()` bernilai false. Memasang `WebView` melaporkan `*core.WebError` melalui `OnError` tanpa memuat WebKit; sediakan konten aplikasi untuk target tersebut.

Di macOS, iOS, Windows, dan Linux, `OnNavigationStateChange` juga melaporkan perubahan URL, riwayat, dan judul dalam dokumen yang sama.

Atur `WebView.Media` ke `core.WebMicrophone`, `core.WebCamera`, atau keduanya untuk mengizinkan permintaan `getUserMedia` dari halaman. Deklarasikan deskripsi penggunaan di `apple.info` dan `RECORD_AUDIO` atau `CAMERA` di `android.permissions`; sistem tetap akan meminta izin pengguna. Di Android, permintaan yang tertunda akan ditolak jika aplikasi menghapus salah satu perangkat yang diminta, halaman berpindah, atau view ditutup saat dialog sistem terbuka. Halaman harus meminta akses lagi setelah perubahan tersebut.

`WebView.Muted` dan `MutedFunc` (TSX `muted`) membisukan seluruh audio halaman di Windows dan Linux, termasuk Web Audio dan frame tertanam. macOS 12 dan iOS 15 atau lebih baru menjeda pemutaran media saat dibisukan. Host lain tetap mempertahankan pemutaran seperti biasa. Pembaruan berlaku pada halaman saat ini dan terbawa melintasi navigasi.

Di TSX, perubahan `injectedJavaScriptBeforeContentLoaded` memuat ulang sumber dengan skrip baru di web view yang sama, sebagaimana `InjectedJavaScriptBeforeContentFunc`; perubahan `injectedJavaScript` membuat web view baru, yang memuat sumber dari awal, sebagaimana perubahan `userAgent`, `userAgentSuffix`, atau `mobile`. Di HarmonyOS, perubahan salah satu skrip yang disuntikkan membuat web view baru dan memulai riwayat native baru. Di Windows dan HarmonyOS, setiap skrip yang disuntikkan berjalan di frame utama di dalam sebuah blok, sehingga deklarasi tingkat atas `let`, `const`, dan `class` tetap lokal di dalamnya; tetapkan ke `window` untuk global yang harus dilihat oleh halaman.

Atur `Mobile` (TSX `mobile`) untuk menampilkan halaman seperti yang dilihat ponsel di Windows, macOS, dan Linux, untuk situs seperti YouTube Shorts yang hanya mengganti video dengan usapan vertikal di ponsel: halaman mendapat user agent ponsel kecuali `UserAgent` diatur serta layar sentuh tanpa hover, dan seretan dengan mouse, atau guliran roda maupun trackpad di atas area yang tidak menggulir, diterima sebagai usapan jari. Windows mengemulasikan ponsel seperti mode perangkat Chrome. macOS dan Linux melakukannya dengan skrip, sehingga stylesheet halamannya masih melihat mouse, dan event sentuhnya tidak tepercaya (untrusted), yang mungkin diabaikan oleh halaman. iOS, Android, HarmonyOS, web, dan mini program menampilkan halaman apa adanya.

Di macOS, iOS, Windows, dan Linux, `OnOpenWindow` menerima permintaan jendela baru dari halaman web. View tidak membuat popup. Gunakan `URL` dan `UserInitiated` dari permintaan untuk memutuskan apakah akan menavigasi view yang sama dengan `Ref.Load`, membuka URL di tempat lain, atau mengabaikannya. View yang dibatasi mengirim permintaan yang ditolak ke `OnError`.

Atur `OnDownload` untuk menyimpan file yang diunduh oleh halaman web view di macOS, iOS, Linux, Windows, dan Android. Unduhan disimpan ke `downloads/<id>/<file name>` di dalam direktori data aplikasi dan melaporkan awal, penyelesaian dengan jumlah byte, atau kegagalannya; view tanpa `OnDownload`, atau view yang dibatasi, membatalkan unduhan.

`core.Video` memutar file atau URL di pemutar milik platform. Di macOS, iOS, dan tvOS, atur `Video.AudioSource` ke file atau URL terpisah yang berisi suara untuk klip video berdurasi terbatas. Audio pemutar berasal dari sumber tersebut. Gambar dan suara dimulai bersamaan, dan `Ref.Seek` menggerakkan keduanya pada garis waktu yang sama. Video menentukan durasi pemutaran, dan audio yang lebih pendek akan menjadi hening saat berakhir.

Klip transparan menampilkan apa yang ada di belakangnya sebagai HEVC dengan alpha dalam `.mov` di macOS (iOS belum diperiksa), dan sebagai VP9 dengan alpha dalam WebM di Linux dengan plug-in `codecalpha` GStreamer (`gstreamer1.0-plugins-bad`, yang dapat direkomendasikan paket Linux dalam `nsgo.yaml`). Di macOS, iOS, Windows, dan Linux, klip lokal yang alpha-nya tidak dapat dipertahankan (WebM di platform Apple, HEVC di Linux, dan format apa pun di Windows hingga output alpha Media Foundation diperiksa di mesin Windows) membuat `OnError` melaporkannya alih-alih menampilkan video dalam kotak buram: error tersebut berupa `*core.MediaError` dengan `Reason` bernilai `core.MediaErrorAlphaUnsupported`, sehingga aplikasi dapat mendekode klip tersebut sendiri. Android dan web, serta URL jarak jauh di setiap platform, memutar klip tersebut sesuai hasil dekodernya, yang mungkin buram.

`OnPause` dan `OnPlay` mengikuti kontrol bawaan di Apple, Android, Windows, Linux, dan web. `OnPause` memberikan `VideoPauseControls` untuk jeda dari kontrol. `Paused` berlaku saat sumber dibuka atau nilainya berubah. Perubahan volume, laju, dan tata letak mempertahankan status jeda atau putar native, termasuk pemutaran dari kontrol sementara `Paused` tetap true. Ikat `PausedFunc` ke state Anda dan perbarui di callback tersebut bila aplikasi Anda menampilkan tombol putarnya sendiri.

`Rate` mengatur kecepatan pemutaran dan `RateFunc` dapat mengubahnya selama pemutaran. Gunakan `VideoInfo.RateChangeable` dari `OnLoad` untuk menampilkan kontrol kecepatan. Linux dapat mengubah laju video yang dapat di-seek bila GstPlay dan filter `scaletempo` GStreamer terpasang. Pemutar cadangan menggunakan 1×.

Atur `Video.Headers` untuk mengirim header permintaan HTTP seperti `Referer`, `User-Agent`, atau `Cookie` di Android dan untuk file HTTP langsung seperti MP4 atau MOV di Linux, macOS, iOS, dan tvOS.

```go
core.Video{
    Source:  "https://video.example/clip.mp4",
    Headers: map[string]string{"Referer": "https://video.example/player"},
}
```

Video menyalin map tersebut saat dipasang dan menggunakan header yang sama jika `SourceFunc` menyediakan URL baru. Di platform Apple, gunakan sumber dengan panjang byte yang diketahui dan dukungan range untuk seeking. Pemutar mengirim header yang dikonfigurasi melalui pengalihan asal yang sama (same-origin redirects). Header yang sama juga berlaku untuk file `AudioSource` HTTP langsung di platform Apple. URL daftar putar yang berakhiran `.m3u` atau `.m3u8` menggunakan pemuat native.

Di Linux, pasang GstPlay serta plug-in `scaletempo` dan `souphttpsrc` GStreamer. Gunakan URL media akhir jika Anda mengatur `User-Agent` atau `souphttpsrc` Anda tidak memiliki properti `location-trusted`. Dengan properti tersebut dan user agent bawaan, GStreamer mempertahankan header aplikasi pada pengalihan asal yang sama. Header aplikasi berlaku untuk file sumber, dan segmen media adaptif menggunakan permintaannya sendiri.

## Komponen

Komponen adalah sebuah fungsi yang menerima scope dan mengembalikan elemen. Fungsi ini berjalan sekali; apa yang berubah kemudian digerakkan oleh signal.

```go
func Greeting(cx *core.Scope) core.Node {
	return core.Text{Content: "Hello from NSGo"}
}
```

Komponen dengan props adalah sebuah tipe yang memiliki metode `Render`:

```go
type Badge struct{ Text string }

func (b Badge) Render(*core.Scope) core.Node {
	return core.Text{Content: b.Text}
}
```

`ui.ToastContent` dapat menghilangkan tombol tutupnya dengan `NoClose`, menambahkan node `Trailing`, dan menutup toast melalui `OnPress`. `Toasts.NoClose` menyembunyikan tombol tutup di seluruh tumpukan.

`ui.Toasts.Style` dan `StyleFunc` menyesuaikan setiap permukaan toast, termasuk latar belakang, batas, sudut, dan padding-nya. Gaya tumpukan digabung di atas tema, diikuti oleh setiap `ToastContent.Style` dan `StyleFunc`. Setiap `StyleFunc` berjalan setelah `Style` pasangannya dan diperbarui ketika signal-nya berubah. `Color` dalam gaya ini mewarnai judul, serta deskripsi dengan opasitas yang dikurangi, sehingga teks tetap terbaca di permukaan gelap atau berwarna. Fase masuk, keluar, dan usapan tetap menerapkan gaya geraknya. Di TSX, atur prop `style` pada `Toasts` dan berikan `style` atau `styleFunc` ke `show` untuk satu toast.

## Signal

Signal menyimpan sebuah nilai. Membacanya di dalam fungsi yang dipanggil oleh elemen, seperti `ContentFunc`, melanggankan bagian layar tersebut, dan hanya bagian itu yang diperbarui ketika nilainya berubah.

```go
count := core.NewSignal(cx, 0)

core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }}

ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }}
```

Signal dari keseluruhan slice memperbarui setiap baris yang membacanya. `core.NewCollection` menyimpan item berdasarkan kunci, masing-masing di dalam signal-nya sendiri, sehingga satu baris mengikuti item-nya sendiri dan mengubah satu item hanya menjalankan satu baris:

```go
todos := core.NewCollection(cx, func(t Todo) int { return t.ID }, nil)

core.List{Count: todos.Len, Key: func(i int) any { return todos.KeyAt(i) },
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		return core.Text{ContentFunc: func() string { t, _ := todos.At(row.Index()); return t.Title }}
	}}

todos.Update(id, func(t Todo) Todo { t.Done = true; return t })
```

State lain dibangun di atas signal dengan cara yang sama:

- `core.NewSelector` memberi tahu apakah suatu kunci merupakan yang terpilih, dan hanya menjalankan baris yang ditinggalkan dan dicapai oleh pilihan.
- `core.NewHistory` menyimpan nilai dengan undo dan redo, beserta signal `CanUndo` dan `CanRedo` untuk menu Edit.
- `core.NewPersisted` menyimpan nilai bertipe di penyimpanan aplikasi sebagai JSON berversi, sambil memigrasikan apa yang ditulis versi lama.
- `core.From` mengikuti sumber di luar graf, seperti langganan yang mengirim dari goroutine lain.

## Gaya

`core.Style` mengikuti CSS: flexbox dan grid, padding dan margin, sudut, batas, bayangan, gradien, transformasi, dan state seperti `Active`. Elemen juga menerima kelas Tailwind melalui `Class`:

```go
core.Text{Content: "Done", Class: "self-start rounded-full bg-zinc-100 px-3 py-1 text-sm"}
```

Tema memberikan tampilan komponen `ui` dalam mode terang dan gelap.

## Aplikasi desktop

`nsgo.Run(Root)` menampilkan satu view root. Sebagai gantinya, aplikasi desktop dapat membuka sendiri apa yang ditampilkannya: `nsgo.Run(App)` menjalankan `func App(app *core.Scope)` sekali, dan fungsi tersebut membuka jendela, panel, ikon baki, dan pintasan global dengan `desktop`, yang berbagi signal-nya. Ini bekerja di macOS, Windows, dan Linux; di macOS `examples/tray` tidak muncul di Dock dan tinggal di menu bar. `nsgo.Run(App, nsgo.StartHidden())` membuka jendela utama tanpa menampilkannya, sehingga aplikasi baki, atau aplikasi yang dijalankan sistem saat login (`desktop.LaunchedAtLogin`), tidak berkedip menampilkannya; aplikasi menampilkannya dengan `app.MainWindow().Show()`. Ponsel dan web menampilkan aplikasi saat diluncurkan.

`desktop.SetWindowLevel` menjaga jendela tetap berada di atas jendela lain—seperti pengatur waktu atau desktop pet—atau di bawahnya, dan `desktop.SetWindowFrame` memindahkan serta mengubah ukurannya. `desktop.CanSetWindowLevel` dan `desktop.CanPlaceWindows` menyatakan apakah keduanya berlaku di scope jendela: keduanya berlaku di macOS, Windows, dan Linux di bawah X11, dan tidak berlaku untuk toplevel Wayland biasa, yang penumpukan dan penempatannya diatur oleh compositor.

Jendela yang dibuka dengan `desktop.WindowOptions{Transparent: true}`, atau jendela utama dengan `nsgo.WindowTransparent()`, tidak memiliki latar belakang atau bingkai, sehingga desktop terlihat di mana pun aplikasi tidak menggambar apa-apa, seperti untuk desktop pet atau widget; view dengan `Desktop.WindowDrag` dapat menggesernya. `NoShadow` menghilangkan bayangan sistem dan `SkipTaskbar` menjaga jendela keluar dari taskbar Windows serta Alt+Tab dan daftar tugas X11, sementara `desktop.SetWindowTransparent`, `SetWindowShadow`, dan `SetSkipTaskbar` mengubahnya kemudian. Windows hanya mengubah transparansi pada jendela yang dibuka transparan, yang tidak memiliki bayangan, dan Wayland tidak memiliki petunjuk untuk menyembunyikan jendela dari daftar tugasnya. `desktop.CanMakeWindowsTransparent` dan `desktop.CanSkipTaskbar` menyatakan di dalam scope jendela apakah hal ini berlaku: transparansi membutuhkan DirectComposition di Windows dan compositor di Linux—karena X11 tanpa compositor tidak memilikinya—dan di macOS `desktop.SetDockVisible` mengeluarkan aplikasi dari Dock.

## Terminal

Impor `base/terminal` dan buat sesi dengan `terminal.New(terminal.Options{Conn: conn})` untuk aliran jarak jauh, atau atur `Command` untuk program lokal di macOS, Linux, atau Windows. Render `terminal.View{Terminal: session}` dan panggil `Close` saat sesi berakhir. `Options.ImageLimit` membatasi penyimpanan grafis Kitty dalam byte: nol menggunakan 64 MiB, nilai negatif menonaktifkan gambar. Program dapat mengirim gambar RGB, RGBA, atau PNG serta penempatan placeholder Unicode.

Panggil `Search(text)` untuk menyorot kecocokan di layar dan scrollback, `SearchNext` dan `SearchPrevious` untuk memilihnya, serta `EndSearch` untuk membersihkannya. `Options.OnSearch` melaporkan `SearchStatus`; `Theme.Search`, `SearchText`, `SearchSelected`, dan `SearchSelectedText` mengatur warna kecocokan. Shell yang memancarkan penanda OSC 133 mendukung Alt/Option-klik di dalam input perintah aktifnya untuk memindahkan kursor. NSGo tidak memasang skrip integrasi shell.

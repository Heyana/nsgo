---
title: Menjalankan dan debugging
description: Jalankan aplikasi di target mana saja dengan hot reload, lalu lihat error, pembaruan, tata letak, kecepatan, dan bahasa lain.
group: Perangkat bantu
order: 20
---

## Menjalankan

```sh
go tool nsgo dev
```

`dev` membangun aplikasi untuk mesin ini dan membukanya, atau untuk target yang disebutkan `dev.target` pada `nsgo.yaml`. Setiap kali file Go disimpan, perintah ini membangun ulang dan memulai ulang aplikasi, menjalankan `go mod tidy` terlebih dahulu jika aplikasi mengimpor paket yang modulnya belum ada di `go.mod` atau `go.sum`; perubahan TSX saja memperbarui aplikasi yang sedang berjalan di tempat, sambil mempertahankan state-nya. Eksekusi berakhir bersama aplikasi.

Di macOS dan Windows, build ulang dev native membuka kembali jendela view utama pada posisi dan ukuran reguler terakhirnya dalam sesi saat ini. Windows juga memulihkan mode layar penuh. Di Linux, GTK memulihkan ukuran regulernya dan meminta mode layar penuh terakhirnya. Window manager menentukan posisinya.

`-target` menjalankannya di tempat lain, dan `-device` memilih iPhone, Apple TV, Apple Watch, simulator, atau perangkat Android berdasarkan nama, UDID, atau nomor seri:

```sh
go tool nsgo dev -target tui                # terminal cells; diagnostics in .nsgo/tui.log
go tool nsgo dev -target web                # in the browser, at the address it prints, on -port (8080 by default)
go tool nsgo dev -target ios_simulator      # in the iOS simulator
go tool nsgo dev -target ios                # on the connected iPhone, signed by Xcode
go tool nsgo dev -target tvos_simulator     # in the Apple TV simulator
go tool nsgo dev -target watchos_simulator  # in the Apple Watch simulator
go tool nsgo dev -target android            # on a connected device or emulator, over adb
go tool nsgo dev -target miniprogram_wechat # in WeChat DevTools (also _qq, _alipay, _douyin)
```

Server dev mendengarkan di semua antarmuka secara bawaan. Gunakan `-host 127.0.0.1` untuk menahannya pada loopback, termasuk ketika Android terhubung melalui `adb reverse`, atau sebutkan alamat IP antarmuka lokal agar hanya mendengarkan di sana. Gunakan alamat yang dapat dijangkau melalui LAN untuk fitur dev pada perangkat Apple fisik.

`-h` mencantumkan setiap flag.

## Debugger Go

Letakkan versi [Delve](https://github.com/go-delve/delve/blob/master/Documentation/installation/README.md) yang kompatibel dengan versi Go Anda di `PATH` sebagai `dlv`. Jalankan aplikasi desktop native dengan informasi debugging dan optimisasi Go dinonaktifkan.

```sh
go tool nsgo dev -target native -debug
```

Salin port dari baris `API server listening at` milik Delve. Di VS Code dengan ekstensi Go, tambahkan konfigurasi attach dan ganti `40000` dengan port tersebut.

```json
{
  "name": "Attach to NSGo",
  "type": "go",
  "request": "attach",
  "mode": "remote",
  "debugAdapter": "dlv-dap",
  "host": "127.0.0.1",
  "port": 40000
}
```

Di GoLand, tambahkan konfigurasi run **Go Remote** dengan host `127.0.0.1` dan port yang sama. Pasang breakpoint pada kode Go aplikasi Anda, attach, lalu jalankan aksi yang mencapainya. Setelah build ulang Go berhasil, hubungkan kembali debugger ke alamat yang sama agar breakpoint dikirim ke proses baru. Perubahan TSX tetap menggunakan hot reload.

`-debug` berjalan di macOS, Windows, dan Linux dengan `-target native`. Gunakan build dev untuk debugging dan cadangkan `-release` untuk mengukur build rilis.

## Pratinjau komponen

Letakkan satu atau beberapa fungsi `PreviewXxx` di samping komponen dalam paket Go yang dapat diimpor. Simpan fungsi tersebut di file Go yang dijaga oleh build tag pratinjau.

```go
//go:build nsgopreview

package components

import "github.com/nsgo-dev/nsgo/core"

func PreviewEmpty(cx *core.Scope) core.Node {
    return core.Text{Content: "No items"}
}
```

```sh
go tool nsgo dev -preview ./components
go tool nsgo dev -target web -preview ./components
```

Centang fixture yang ingin ditampilkan bersama dan pilih locale, tema, tampilan terang atau gelap, ukuran viewport, dan arah tata letak. Pratinjau mengambil `nsgo.yaml` aplikasi terdekat, katalog terjemahannya, `theme` yang dikonfigurasi, serta sumber dayanya. Berikan direktori aplikasi setelah flag untuk memilih konfigurasi berbeda dalam modul yang sama. Dalam hal tersebut, jalur pratinjau bersifat relatif terhadap direktori aplikasi.

Menyimpan file akan membangun ulang pratinjau dan mempertahankan pilihan serta pengaturan tampilan Anda. Tag `nsgopreview` mengecualikan fixture dari build rilis biasa. Gunakan fixture yang sama di file `_test.go` bertag dengan `nsgotest.Start(t, PreviewEmpty)` dan jalankan dengan `GOFLAGS=-tags=nsgopreview go tool nsgo test -target macos ./components` untuk mengambil tangkapan layar native.

Setelah pemasangan pertama aplikasi utama dengan `-target ios_simulator`, build ulang khusus Go mengganti executable yang terpasang dan meluncurkan ulang aplikasi. Mengedit `nsgo.yaml`, sumber daya yang dikemas, atau ikon aplikasi akan memicu pemasangan ulang.

Gunakan `go tool nsgo dev -target android -android-host` untuk pekerjaan UI Android dengan siklus build ulang Go yang lebih singkat. Alat ini memasang runtime native sekali dan menjalankan Go di mesin pengembangan. Setiap build ulang Go terhubung kembali ke Activity yang sama melalui adb. Perubahan pada sumber daya yang dikemas, kelas yang dikompilasi sebelumnya, ikon peluncur, atau `nsgo.yaml` akan memasang ulang APK runtime. Proses Go menggunakan batasan build, file, dan jaringan milik host. `Scope.DiskSpace` melaporkan volume yang memuat `DataDir` host. Gunakan perintah dev Android biasa untuk menguji file `_android.go` dan perilaku Go di perangkat.

## Error

Build yang gagal ditampilkan di atas aplikasi yang sedang berjalan, dengan tombol untuk menyalin error dan membangun lagi; perbaiki filenya dan penyimpanan berikutnya akan menggantikannya. Komponen yang mengalami panic saat merender juga ditampilkan dengan cara yang sama. Command+R di aplikasi, Control+R di luar platform Apple, membangun dan memulainya lagi kapan saja, termasuk di atas tampilan error, kecuali aplikasi mengikat tombol itu sendiri dengan `cx.OnKey` atau sebagai item menu aplikasi yang aktif; di browser tombol itu memuat ulang halaman seperti biasa. Output aplikasi sendiri, seperti `log.Print`, muncul di terminal di bawah baris output alat.

Sebelum menjalankan, `go tool nsgo check` memeriksa aplikasi dan melakukan type-check pada TSX-nya; `-target all` memeriksanya untuk setiap target, sehingga kode di balik build tag platform lain juga diperiksa dari Mac.

### Log aplikasi Apple yang dikemas

Di macOS, iOS, tvOS, dan watchOS, logger Go bawaan menulis ke unified log Apple, termasuk `log.Print`, diagnostik NSGo, dan output konsol TSX. Di Console, filter berdasarkan bundle identifier aplikasi yang berjalan sebagai subsystem dan `nsgo` sebagai category. Di macOS Anda juga dapat melakukan kueri ke store:

```sh
log show --last 10m --predicate 'subsystem == "dev.example.hello"'
```

Catatan menggunakan tipe log bawaan serta mempertahankan awalan warning atau error-nya. Teksnya bersifat publik, jadi jauhkan kredensial dan rahasia lainnya dari log. Pesan panjang menjadi beberapa entri. Output terminal tetap tersedia selama pengembangan; writer log kustom atau handler `slog` mengontrol outputnya sendiri.

### Web view Linux

Ubuntu 24.04 dan yang lebih baru dapat membatasi user namespace tanpa hak istimewa melalui AppArmor. Ketika bubblewrap tidak dapat memulai sandbox WebKitGTK, NSGo mencatat error dan menampilkan placeholder. Sebagian versi WebKitGTK akan keluar dengan `SIGTRAP` saat membuka web view pertama. [Dokumentasi AppArmor Ubuntu](https://documentation.ubuntu.com/security/security-features/privilege-restriction/apparmor/)

Gunakan profil AppArmor bubblewrap distribusi Anda jika tersedia, seperti `bwrap-userns-restrict`. Jika tidak, administrator dapat memuat profil yang memberikan `userns` ke `/usr/bin/bwrap` menggunakan [profil dari CI Linux NSGo](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/test.yml) sebagai titik awal. Administrator mengelola profil tersebut karena berlaku untuk setiap aplikasi yang menggunakan bubblewrap sistem.

`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1` menonaktifkan sandbox subproses WebKit. Gunakan hanya untuk pengembangan lokal dengan konten tepercaya.

## DevTools

```sh
go tool nsgo dev -devtools
```

`-devtools` membuka DevTools di browser, dilayani oleh `nsgo dev` untuk setiap target, termasuk aplikasi ponsel. Alat ini menampilkan aplikasi yang sedang berjalan secara langsung:

- **Elements**: view dalam bentuk pohon. Masing-masing menampilkan komponen yang membuatnya, tautan ke barisnya di editor Anda, selector yang digunakan pengujian untuk menemukannya, kelasnya, batasnya, margin, border, serta padding sesuai tata letak, dan gaya yang ditetapkannya. Mengarahkan kursor ke sebuah baris akan menggambar margin, padding, dan kontennya di aplikasi, seperti DevTools browser, dan **Pick** memungkinkan Anda menekan view di jendela, panel, sheet, atau embedded view mana pun untuk menemukannya di pohon.
- **State**: signal yang dibuat kode Anda, di bawah komponen dan komputasi yang memilikinya, beserta nilainya, nilai memo, dan berapa banyak komputasi yang membaca masing-masingnya. Nilai yang tidak dibaca oleh komputasi apa pun tidak mengubah apa pun di layar. String, boolean, dan angka dapat diedit, sehingga Anda dapat mencoba suatu state tanpa mengubah kode. Nilai yang dibuat NSGo sendiri berada di balik **NSGo's own**.
- **Profiler**: waktu setiap commit, dipecah menjadi kode Anda sebelumnya, commit beserta tata letaknya, dan penerapannya di thread utama platform. Profiler juga menampilkan baris kode yang penulisannya menyebabkan commit tersebut, dan terbuka pada komputasi yang dijalankannya, dari yang paling lama terlebih dahulu. Daftar kedua menyebutkan komputasi yang paling sering berjalan dalam satu detik terakhir, berdasarkan baris yang membuatnya. Runtime Android menerapkan batch di kode Java-nya sendiri, sehingga commit Android tidak memiliki waktu platform.
- **Recorder**: apa yang Anda tekan, ketik, hapus, dan gulir di aplikasi, ditulis sebagai file pengujian `nsgotest` untuk disalin ke file `_test.go`. Di Elements, **Assert shown** dan **Assert text** menambahkan pemeriksaan untuk view yang dipilih. Pemeriksaan teks menyimpan teks dalam bahasa yang ditampilkan aplikasi, jadi rekam dengan bahasa tempat pengujian Anda berjalan.
- **Highlight updates** memberi garis luar pada setiap view yang diubah oleh commit, seperti `-highlight`.
- **Layout** menampilkan dan menyembunyikan [audit tata letak](#tata-letak) di perangkat, seperti yang dimulai oleh `-layout`.

State, komputasi, dan penulisan pada aplikasi TSX dinamai berdasarkan baris TSX-nya, melalui source map bundel, dan tertaut ke file di aplikasi Anda.

DevTools membaca aplikasi melalui server `nsgo dev`, yang mendengarkan di jaringan Anda agar ponsel dapat menjangkaunya. Setiap eksekusi membuat sebuah token, yang dibawa oleh alamat yang dicetak dan dibuka `nsgo dev` untuk DevTools serta target web setelah `#`, dan server hanya menjawab permintaan yang membawanya, jadi bukalah DevTools dan target web dari alamat tersebut. Siapa pun yang Anda beri alamat itu dapat mengubah state aplikasi saat berjalan dan menambahkan langkah ke pengujian yang ditulis Recorder, jadi bacalah pengujian yang direkam sebelum menjalankannya. Pihak lain di jaringan Anda dan halaman web dari situs lain yang Anda kunjungi tidak dapat menjangkaunya.

## Pembaruan

```sh
go tool nsgo dev -highlight
```

`-highlight` memberi garis luar selama setengah detik pada setiap view yang diubah oleh sebuah commit, sehingga pembaruan yang menggambar ulang lebih dari yang seharusnya terlihat di layar saat Anda menggunakan aplikasi. Garis luar digambar di atas setiap jendela, panel, modal, sheet iOS, dan embedded view (`nsgo.NewView`). DevTools menyalakan dan mematikannya saat aplikasi berjalan.

```sh
go tool nsgo dev -trace
```

`-trace` mencetak setiap commit dari aplikasi yang berjalan: nomornya, waktunya, dan item mount yang dikirimnya, sehingga Anda melihat view mana yang dibuat, diubah, atau dihapus oleh suatu pembaruan. Di atas item tersebut tercetak asal muasal commit: baris kode Anda yang menulis signal sejak commit terakhir, dari yang paling sering menulis terlebih dahulu, serta berapa banyak binding, effect, dan memo yang berjalan.

```text
── commit 12: 40 items, commit 1.65 ms
   after app/market.go:473 ×4, app/market.go:487 ×4, app/market.go:384
   ran 118 bindings, 18 effects, 5 memos
```

Baris yang menulis berkali-kali, atau banyak binding yang berjalan untuk satu penulisan, adalah tempat untuk menggabungkan penulisan (`cx.Batch`) atau mempersempit apa yang dibaca suatu binding. `go tool nsgo test -trace` mencetak hal yang sama di bawah setiap langkah pengujian.

## Profil runtime

Tangkap aktivitas CPU Go, heap, atau jejak eksekusi aplikasi yang sedang berjalan dengan `nsgo dev`. Aplikasi mengirim tangkapan tersebut ke server dev, yang menyimpannya di mesin yang menjalankan perintah.

```sh
go tool nsgo dev -target android -profile cpu -profile-duration 10s
go tool pprof -top .nsgo/profiles/cpu.pprof
```

Contoh CPU tersebut juga berfungsi di desktop, iPhone, atau simulator iOS. Di iPhone, izinkan akses Local Network jika sistem memintanya. Heap berupa snapshot dan berfungsi di web juga. Untuk event scheduler dan goroutine, jalankan `go tool nsgo dev -profile trace` dan buka `.nsgo/profiles/execution.trace` dengan `go tool trace`. Jejak CPU dan eksekusi secara bawaan berdurasi 10 detik. `-profile-output` mengatur jalur file lokal. Untuk pratinjau komponen, jalur bawaan dan jalur relatif menggunakan direktori aplikasi. Eksekusi dev menyimpan tangkapan lengkap pertamanya, lalu terus menjalankan aplikasi.

```sh
go tool nsgo dev -target web -profile heap
go tool pprof -top .nsgo/profiles/heap.pprof
```

Runtime browser Go mendukung profil heap dan jejak eksekusi. Untuk sampel CPU browser, gunakan alat Performance milik browser. Pilih salah satu antara `-profile` atau `-release`. Pengemasan dengan `nsgo build` tidak menyertakan klien tangkapan dev. [Diagnostik runtime Go](https://go.dev/doc/diagnostics)

## Tata letak

```sh
go tool nsgo dev -layout
go tool nsgo dev -layout=bounds,grid=4
```

`-layout` memulai aplikasi dengan audit tata letak DevTools di atas setiap jendela, panel, sheet iOS, dan embedded view, yang memberi garis luar pada setiap view: kotak border-nya berwarna biru, tepi dalam padding-nya berwarna hijau, dan tepi luar margin-nya berwarna oranye, masing-masing terpotong di tempat scroll view atau view yang melakukan clipping menyembunyikannya. `grid` menggambar kisi 8 poin, dan `grid=N` kisi N poin, minimal 4. Berikan nilainya setelah `=`: flag itu sendiri saja berarti `bounds`.

Tampilan pembacaan di sudut bawah menunjukkan ukuran jendela dalam poin, dengan tombol untuk menyalakan dan mematikan kotak, kisi, serta penggaris. Di Apple TV dan perangkat khusus remote lainnya, fokus dapat menjangkau tombol pada pembacaan tersebut sehingga Anda dapat menyalakan atau mematikannya dengan remote tanpa meninggalkan aplikasi. Saat penggaris aktif, penekanan memilih dua titik alih-alih mencapai aplikasi, dan pembacaan menampilkan seberapa jauh jarak keduanya secara mendatar dan menurun; penekanan ketiga memulai ulang pengukuran. Di Apple TV atau dengan keyboard, panah arah (atau mengusap touchpad Siri Remote) menggerakkan bidik silang di layar dan Select, Return, atau Space memilih titik, memperbarui jarak secara langsung hingga titik kedua menguncinya.

| Tombol | Efek |
| --- | --- |
| `Cmd+Option+Shift+L`, `Ctrl+Alt+Shift+L` | Menampilkan atau menyembunyikan audit, pada build dengan `-layout` |
| Panah, D-pad / usap remote | Menggerakkan bidik silang penggaris sebesar langkah kisi (8 pt secara bawaan) |
| `Shift` + Panah | Menggerakkan bidik silang penggaris sebesar 1 pt |
| Select, `Return`, `Space` | Memilih titik bidik silang saat mengukur |
| `Esc`, back, Menu | Menghentikan pengukuran |

Jika aplikasi Anda mengikat tombol yang sama, ikatan milik aplikasi yang menang. Tanpa tombol tersebut, seperti di ponsel, jalankan dengan `-devtools` juga dan tampilkan serta sembunyikan audit lewat **Layout** di DevTools; sembunyikan untuk menjangkau kontrol di bawah pembacaan. Backend native dan web membaca matriks presentasi sehingga batas, Pick, dan audit mengikuti transformasi, animasi, serta pengguliran yang ditampilkan; watchOS mengikuti penggambaran afinnya. HarmonyOS dan mini program mempertahankan pelacak sisi Go hingga lapisan rendernya melaporkan geometri. Menggambar ulang semuanya memakan waktu di setiap pembaruan, jadi ukurlah kecepatan tanpa opsi ini.

## Kecepatan

```sh
go tool nsgo dev -fps
go tool nsgo dev -release -fps
```

`-fps` menampilkan pemantau performa di atas aplikasi pada macOS, iOS, Windows, Linux, Android, dan web. Pemantau ini mengukur frame dan commit, dan Android juga menampilkan waktu pulang-pergi (round-trip) teks native. Build dev membawa klien dev; lakukan pengukuran dengan `-release`, yang membangun sebagaimana `nsgo build`.

## Bahasa dan tampilan

```sh
go tool nsgo dev -lang zh-Hans
go tool nsgo dev -lang en-XA   # accented and longer, to find untranslated and cut text
go tool nsgo dev -lang ar-XB   # right to left
```

`-lang` memulai aplikasi dalam suatu locale, termasuk pseudo-locale ([Pelokalan](localization.md)).

## Agen

```sh
go tool nsgo dev -automation
go tool nsgo dev -target android -automation
go tool nsgo dev -target web -automation
```

`-automation` memungkinkan agen AI atau runner end-to-end menggunakan aplikasi yang sedang berjalan seperti halnya manusia, di macOS, Windows, Linux (`-target native`), Android (`-target android`), web (`-target web`), mini program di simulator alat pengembangnya (`-target miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay`, atau `miniprogram_douyin`), serta perangkat dan simulator Apple (`-target ios`, `tvos`, `watchos`, `ios_simulator`, `tvos_simulator`, atau `watchos_simulator`) sejauh ini: mencantumkan jendelanya, merinci apa yang ditampilkan suatu jendela, menekan, mengetik, mengirim tombol, menggulir, menyeret, mengarahkan kursor, memilih item menu, menjawab peringatan, menunggu apa yang muncul, dan mengambil tangkapan layar. Agen menjangkaunya melalui MCP lewat `go tool nsgo mcp`, yang menggunakan `dev -automation` yang sedang berjalan untuk direktori tersebut, atau memulai satu proses dengan flag yang sama (seperti `-target android`) dan menghentikannya saat klien keluar:

```sh
claude mcp add nsgo -- go tool nsgo mcp
```

Klien lain menggunakan perintah yang sama, `go` dengan argumen `tool nsgo mcp`, yang dijalankan di direktori aplikasi. View dinamai dengan selector [pengujian](testing.md#sebuah-pengujian), atau dengan referensi `view:` yang dicetak snapshot, yang menjadi usang saat penyimpanan file memulai ulang aplikasi.

Agen bertindak pada data nyata aplikasi, sebagaimana `dev` menjalankannya. Hanya build `dev -automation` yang menyertakannya, dan hanya alat di mesin ini yang memegang token di `.nsgo/automation.yaml`, yang hanya dapat Anda baca, yang dapat menjangkaunya.

### Alat MCP yang didefinisikan aplikasi

Panggil `automation.Register` dari aplikasi atau pustaka Go pada goroutine engine. Misalnya, fungsi ini memberi agen ID scene yang sedang ditampilkan.

```go
import (
    "context"
    "encoding/json"

    "github.com/nsgo-dev/nsgo/automation"
    "github.com/nsgo-dev/nsgo/core"
)

func registerSceneTool(cx *core.Scope, sceneID *core.Signal[string]) error {
    return automation.Register(cx, automation.Tool{
        Name:        "current_scene",
        Description: "Read the ID of the active scene",
        InputSchema: json.RawMessage(`{"type":"object"}`),
        Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
            return map[string]string{"id": sceneID.Get()}, nil
        },
    })
}
```

Namanya harus unik pada graf aplikasi dan berbeda dari nama bawaan. `nsgo mcp` mencantumkan alat tersebut hingga scope dibersihkan. Handler berjalan pada goroutine engine, sehingga dapat membaca atau mengatur signal. Handler menerima objek JSON, memeriksa argumen yang dibutuhkannya, dan mengembalikan nilai yang dapat dienkode JSON atau error.

Dalam pengujian, tangkap scope saat aplikasi dimulai, panggil `automation.Call(cx, ctx, name, args)` di dalam `app.Post`, lalu gunakan `app.Settle()` sebelum memeriksa hasil atau view. Pendaftaran aktif di `dev -automation` dan pengujian, dan tidak melakukan apa pun (no-op) di build rilis.

## Flag pada setiap build

Setiap build aplikasi, termasuk paket yang dikemas, menerima flag milik NSGo sendiri saat dimulai, yang menimpa `nsgo.yaml` dan kode aplikasi serta tidak pernah diteruskan ke flag milik aplikasi itu sendiri:

| Flag | Efek |
| --- | --- |
| `--nsgo-fps` | Menampilkan pemantau performa di macOS, iOS, Windows, Linux, Android, dan web |
| `--nsgo-lang=zh-Hans` | Memulai aplikasi dalam suatu locale, termasuk pseudo-locale |
| `--nsgo-appearance=dark` | Memulai aplikasi dalam mode `light`, `dark`, atau mengikuti `system` |
| `--nsgo-os=ios` | Menjalankan build web sebagai `macos`, `windows`, `linux`, `ios`, atau `android`, sedangkan `web` adalah dirinya sendiri |

```sh
./Hello --nsgo-fps                                   # Windows and Linux
open Hello.app --args --nsgo-fps                     # macOS
xcrun simctl launch booted dev.example.hello --nsgo-fps
adb shell am start -n dev.example.hello/dev.oneo.nsgo.NsgoActivity --ez nsgo-fps true --es nsgo-lang de
```

Di web, flag tersebut masuk ke query halaman: `?nsgo-fps&nsgo-lang=de`.

`--nsgo-os` mempratinjau tata letak platform lain di browser: aplikasi membaca OS tersebut beserta idiomnya—ponsel untuk iOS dan Android dan desktop untuk lainnya—dan `Env.Simulated` bernilai true. Browser tidak memiliki bagian native platform tersebut: `NavHeader` dan bilah tab yang digambar menggantikan milik sistem, tidak ada menu aplikasi, tidak ada menu popup native (`desktop.PopupMenu`), dan tidak ada fungsi jendela, serta `mobile.IsAvailable` dan `desktop.IsAvailable` bernilai false. Menu yang digambar seperti `ui.ContextMenu` dan `ui.Menu` tetap berfungsi. Pintasan mengikuti keyboard pengunjung. Build lain mengabaikan flag ini dan mencatat peringatan.

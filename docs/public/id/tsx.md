---
title: Aplikasi TSX
description: Tulis aplikasi dalam TSX dengan kelas Tailwind, dikompilasi lebih dulu tanpa Node.
group: Konsep inti
order: 63
---

## Sebuah aplikasi TSX

Aplikasi TSX adalah paket Go yang menyematkan sumbernya dan menjalankan entry-nya:

```go
//go:embed *.tsx nsgo.yaml
var src embed.FS

var Root = compiler.App(src, "App.tsx")

func main() { nsgo.Run(Root) }
```

```tsx
import { mount, signal, Text, View } from "nsgo";

function App() {
  const count = signal(0);
  return (
    <View class="gap-3 p-6" onPress={() => count.update((n) => n + 1)}>
      <Text class="text-lg font-bold">Pressed {count()} times</Text>
    </View>
  );
}

mount(App);
```

`go tool nsgo init -template tsx` menulis satu aplikasi seperti ini, dan `examples/tsx` adalah contoh lainnya. `nsgo dev` dan `nsgo build` mengompilasi TSX lebih dulu (*ahead of time*), sehingga paket tidak menyertakan compiler; `go run` dan `go test` mengompilasinya saat aplikasi dimulai. Node tidak diperlukan dalam kedua cara tersebut.

## Komponen dan modul

TSX mengimpor primitif dari `nsgo`, komponen `base` dari `nsgo/base`, pengontrol game dari `nsgo/gamepad` (di bawah kapabilitas `gamepad` dalam paket bertanda tangan), suara dari `nsgo/audio` (di bawah `audio`), dan komponen `ui` dari `nsgo/ui`, setelah aplikasi mengimpor `tsx/ui`; `examples/gallery-tsx` menampilkannya.

Untuk diagram Mermaid, impor `github.com/nsgo-dev/nsgo/tsx/diagram` demi efeknya di host Go, bersama dengan `tsx/textview` atau `tsx/ui`. Ini menambahkan `Diagram` ke `nsgo/base` tanpa menautkan parser diagram ke aplikasi yang tidak menggunakannya. Komponen ini menerima `source`, `fontSize`, `theme` parsial, `accessibilityLabel`, dan prop gaya yang biasa. Gunakan secara langsung atau sediakan renderer TextView:

```tsx
import { Diagram, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderDiagram={(code) => <Diagram source={code.code} errorClass="font-mono" />} />
```

Prop `renderDiagram` yang sama berfungsi pada TextView milik `nsgo/ui`. Blok kode non-Mermaid tetap memakai renderer biasanya; diagram yang tidak valid menampilkan sumbernya.

Untuk matematika yang ditata, impor `github.com/nsgo-dev/nsgo/tsx/formula` demi efeknya di host Go, bersama dengan `tsx/textview` atau `tsx/ui`. Ini menambahkan `Formula` ke `nsgo/base`; aplikasi yang tidak menyertakan paket ini menjaga mesin matematika dan fontnya tetap di luar binernya. Komponen ini menerima `source`, `dialect` (`tex` secara default, atau `typst`), `display`, `accessibilityLabel`, dan prop gaya. Ukuran dan warna font mengikuti teks di sekitarnya. Gunakan secara langsung atau sediakan renderer TextView:

```tsx
import { Formula, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderMath={(math) => <Formula source={math.tex} display={math.display} />} />
```

`renderMath` yang sama berfungsi pada TextView milik `nsgo/ui`. Formula mempertahankan TeX-nya saat disalin. `Renderer` Go tetap menjadi konfigurasi host.

`UrlScheme.url()` membaca alamat aplikasi yang dilacak; `UrlScheme.setUrl(url, { history: "push" })` menambahkan entri alamat, atau menggantinya jika opsi dihilangkan. `UrlScheme.on("change", fn)` mengamati navigasi mundur, maju, dan perubahan hash di browser serta mengembalikan fungsi untuk berhenti berlangganan. Lihat [Alamat aplikasi](addresses.md) untuk routing, validasi same-origin, dan hosting deep link.

## Go dari TSX

Daftarkan Go agar dapat diimpor oleh TSX:

```go
tsx.Register("app/native", map[string]core.Node{"Badge": Badge{}})
tsx.RegisterFunctions("app/native", map[string]any{
	"greet": func(name string) string { return "Hello, " + name },
})
```

`import { Badge, greet } from "app/native"` kemudian menerima field `Badge` sebagai prop dan memanggil `greet`. Fungsi yang menerima `context.Context` menjadi promise, dan `tsx.RegisterClass` menjadikan tipe Go sebagai kelas yang dibuat TSX dengan `new`.

Prop opsional yang diteruskan sebagai `undefined` menggunakan nilai default-nya. Saat mendaftarkan komponen Go dengan getter yang default-nya berbeda dari nilai nol tipenya, tambahkan opsi seperti `tsx.Default("Max", float64(100))`; default yang sama berlaku selama prop reaktif mengembalikan undefined, tanpa mengganti komponen.

Field data dengan default Go yang terdokumentasi juga dapat dihilangkan: `width` dan `height` pada node `Flow` mengukur konten JSX-nya jika tidak ada, `width` sebuah edge yang dihilangkan menggunakan goresan default, dan `pace` yang dihilangkan pada baris kuota membuat tanda pace-nya tersembunyi.

Mendaftarkan komponen dan kelas Go menemukan field dan method yang diekspor melalui refleksi saat aplikasi berjalan. Pencarian itu mencegah linker Go membuang method terekspor yang tidak dipanggil oleh apa pun, di seluruh aplikasi dan bukan hanya pada apa yang didaftarkan TSX, sehingga menautkan `tsx` menambah sekitar 20 MB pada biner desktop yang di-strip, dan lebih besar lagi semakin banyak paket yang ditautkan aplikasi.

## Paket bertanda tangan

Aplikasi dapat menjalankan TSX yang diunduhnya, seperti layar miliknya sendiri yang dikirim ulang atau plugin, setelah ia memverifikasi paketnya. Deklarasikan apa yang boleh dijangkau oleh kode paket di `nsgo.yaml`-nya; segala hal lain akan ditolak:

```yaml
capabilities:
  network:
    hosts: [api.example.com, "*.cdn.example.com"]
  open_url: true # or { schemes: [https] }
  host: [search, "player.*"] # the app's handlers it may call
  # webview: { hosts: [...], input: true, media: [microphone, camera] }, clipboard, picker, speech, text_to_speech, status_bar, notifications, biometrics, sign_in_with_apple, purchases, secrets, audio, gamepad, auth_session
```

Simpan root key secara offline dan sertifikasi release key dengannya, lalu tandatangani aplikasi di direktori tersebut. `go tool nsgo keygen` mencetak pasangan kunci dan tidak menulis file apa pun; jalankan sekali untuk root dan sekali untuk setiap release key. Kunci publik dimasukkan ke flag dan kode aplikasi; kunci rahasia tetap di luar repositori dan hanya mencapai tool melalui `NSGO_ROOT_KEY` dan `NSGO_RELEASE_KEY`:

```sh
go tool nsgo keygen # once for the root, once for the release key
NSGO_ROOT_KEY=<root secret> go tool nsgo certify -key <release public> -apps 'com.example.*'
NSGO_RELEASE_KEY=<release secret> go tool nsgo publish -cert release.cert plugin
go tool nsgo verify -root <root public> plugin/dist/Plugin-1.0.0.nsgopkg
```

Aplikasi memercayai kunci publik root yang ditulis dalam kodenya, memverifikasi paket di luar goroutine engine, dan menampilkannya di `tsx.View`, dalam sandbox di samping view miliknya sendiri. Paket hanya dapat menjangkau aplikasi melalui kanal host yang diberikan view kepadanya, dan menyimpan bagiannya sendiri dari penyimpanan dan rahasia aplikasi:

```go
trust, err := pkg.OpenTrust(filepath.Join(cx.DataDir(), "trust"))
trust.AddRoot(root)
p, err := trust.Verify(data) // refuses tampered, revoked and older builds
channel := core.NewHost()
channel.Handle("search", func(c *core.HostCall) { c.Resolve(results(c.Data)) })
view := tsx.View{Package: p, Sandbox: tsx.Sandbox{Host: channel}}
core.Catch{Try: view.Render, Fallback: failed}
trust.Accept(p) // once it ran, older builds are refused
```

`Sandbox.Grant` memberinya lebih sedikit dari yang dideklarasikannya, `Sandbox.Limits` memberi anggaran waktu dan kuota lain, dan `Sandbox.Modules` memberi fungsi dan komponen Go di luar milik NSGo sendiri. Pintasan keyboard, handler kembali (*back*), dan tumpukan navigasinya hanya berlaku selama fokus keyboard berada di view-nya, dan ia hanya memindahkan fokus sendiri (`focus()`, `autoFocus`, `Keyboard.dismiss`, `setAccessibilityFocus`) selama fokus sudah berada di sana atau sebagai jawaban atas tekanan pengguna di dalamnya. Paket ditampilkan dalam view miliknya sendiri yang mengisi ruang yang diberikan induknya, tidak lebih, dan memotongnya, termasuk menu, popover, dan toast; di dalam scroll view, letakkan di dalam sebuah view agar ia mengambil tinggi kontennya. Dialog, alert, menu konteks, atau pengumuman miliknya hanya ditampilkan dengan cara yang sama, dan tombol kembali menutup dialog miliknya meskipun paket tidak menghapusnya. `View.Export` menyebutkan komponen yang diekspor oleh modul entry paket, yang ditampilkan menggantikan root yang di-mount oleh entry, sehingga `mount`-nya tidak merender maupun menjalankan komponennya, dan diberi `View.Props`, sebuah map yang dapat dienkode JSON dan disalin untuk setiap view, sehingga satu paket menyediakan beberapa view, masing-masing dengan state-nya sendiri, di bawah kapabilitas dan batas yang sama; ekspor yang tidak ada menyebabkan panic dengan `tsx.ErrNoViewExport` dan ekspor yang bukan fungsi dengan `tsx.ErrViewExportNotComponent`. Paket yang entry-nya mengekspor fungsi adalah plugin fungsi: `tsx.Load(cx, p, tsx.Sandbox{})` menjalankannya tanpa terlihat dan secara sinkron di goroutine pemanggil, di bawah `cx` yang termasuk dalam sebuah pohon view (tanpa itu ia mengembalikan `tsx.ErrNoTree`), atau mengembalikan apa yang dilemparkan modul entry-nya saat dimuat, dan `Plugin.Call` memanggil fungsi-fungsi tersebut. `go tool nsgo dev -limits` menjalankan aplikasi seperti view semacam itu, dan `go tool nsgo revoke` menandatangani daftar release key dan paket yang dicabut untuk `Trust.UpdateRevocations`. `examples/plugins` meminta pengguna memberikan kapabilitas sebuah paket, memasangnya, dan menjalankannya. `Limits.Memory`, 64 MiB secara default, menganggarkan penyimpanan native terukur untuk array, string, buffer, dan typed array. Ini mencakup salinan `concat`, `flat`, `slice`, spread array, penggabungan string, template, `replace`, `replaceAll`, `TextEncoder`, dan `Blob`. `memory=<bytes>` di `-limits` mengubah anggaran, dan `memory=0` mematikannya. Pemeriksaan dilakukan sebelum alokasi. Paket yang melampaui anggaran berhenti dengan `ERR_NSH_QUOTA_EXCEEDED` dan resource `memory` setelah Go memeriksa penyimpanan yang dapat dikumpulkan. Objek biasa, closure, stack, alokasi kecil, serta built-in dan API host yang tidak terukur juga dapat melakukan alokasi. Browser mempertahankan heap-nya sendiri. Aplikasi yang menandatangani paket sendiri dengan `pkg.Build`, atau `go tool nsgo publish -extra <file>` dengan byte file tersebut, dapat menaruh metadata miliknya sendiri di `pkg.Contents.Extra`, seperti apa yang ditambahkan plugin ke menunya: metadata ini ditandatangani bersama paket, dan `Verify` mengembalikannya tanpa perubahan sebagai `p.Extra` untuk di-parse aplikasi, sehingga aplikasi membacanya tanpa menjalankan kode paket.

Aplikasi dapat menandatangani dokumen JSON miliknya sendiri, seperti katalog plugin-nya, dengan `pkg.SignDocument` dan memverifikasinya dengan `Signed.Open`, menggunakan kunci yang dicetak oleh `nsgo keygen`. Berikan setiap jenis dokumen konteks `app:` tersendiri beserta versinya, seperti `app:catalog-v1`: sebuah dokumen tidak pernah lolos sebagai jenis lain, maupun sebagai paket, sertifikat, atau daftar pencabutan. `Open` memercayai persis kunci publik yang diberikan kepadanya, tanpa sertifikat atau daftar pencabutan, dan menolak field body yang tidak dimiliki tipe tujuan dekodenya. Amplopnya tidak menyimpan waktu atau urutan, sehingga dokumen yang lebih lama lolos verifikasi sama baiknya dengan yang lebih baru: taruh versi atau masa berlaku di body dan periksa.
Di sandbox native, `JSON.stringify`, `JSON.parse` dengan reviver, `Array.prototype.join`, `toString`, `toLocaleString`, dan `flat`, dan dengan demikian `String()` dari sebuah array, melempar `RangeError` yang dapat ditangkap begitu kedalamannya melebihi 1.000 tingkat, dengan menghitung yang satu di dalam yang lain, sebagaimana `toString` sebuah array dan `join` yang dipanggilnya masing-masing dihitung; data yang lebih dalam akan membuat panggilan macet atau mengakhiri aplikasi. Web menggunakan built-in milik browser.

Pada target native, `DefaultLimits.Stack` adalah 1.000 frame. Atur `Limits.Stack` atau `-limits stack=<frames>` untuk memilih kedalaman lain hingga 10.000 frame. Nilai Go di bawah atau sama dengan nol dan input di atas batas atas menggunakan 10.000. Web menggunakan batas stack milik browser.

Fungsi native yang saling memanggil dihitung terhadap `Limits.Stack` seperti panggilan JS: getter, `toString`, atau `valueOf` yang merupakan built-in, ekspresi reguler yang `exec`-nya adalah method lain miliknya, `Reflect.apply` dan `Reflect.construct` di dalam satu sama lain, serta rantai fungsi terikat (*bound*) atau proxy melempar `RangeError` yang dapat ditangkap setelah melewati batas itu alih-alih mengakhiri aplikasi, dan nilai plugin yang bersarang lebih dalam dari batas itu menggagalkan panggilan yang mengonversinya ke Go dengan `RangeError`. Ini berasal dari fork goja yang dibutuhkan NSGo, `github.com/nsgo-dev/goja`, yang didapat aplikasi Anda bersama NSGo tanpa perubahan apa pun pada `go.mod`-nya.

Pada target native, `Limits.Call` membatasi setiap potongan (*slice*) JavaScript, termasuk callback timer dan kelanjutan promise setelah `await`. Saat sebuah potongan dipotong, `Plugin.Call` yang tertunda melaporkan `tsx.ErrBudget` sekali, menghentikan timer yang tersisa, dan mengabaikan jawaban host yang terlambat. Panggilan independen tetap berjalan, termasuk reaksi promise yang diantrekan dalam potongan yang terinterupsi. Panggilan yang menyelesaikan promise milik panggilan lain tetap melaporkan hasil atau galatnya sendiri. Ekspresi reguler dengan backtracking menggunakan anggaran load atau call yang lebih pendek sebagai batas waktu pencocokannya; batas waktu yang tidak ditangani juga menjadi `tsx.ErrBudget`.

`Limits.CallTotal` menambahkan tenggat waktu nyata (*wall-clock*) untuk setiap panggilan plugin dan event view yang di-sandbox. Tenggat ini mencakup waktu menunggu dan menahan timer, interval, frame, kelanjutan promise, dan jawaban host miliknya bahkan setelah panggilan selesai. Pada tenggat waktu, pekerjaan yang tertunda dibuang dan panggilan yang masih menunggu gagal dengan `tsx.ErrBudget`. Pekerjaan modul entry dibatasi oleh `Limits.Load`, menggunakan `CallTotal` jika `Load` bernilai nol. Event host mandiri menggunakan `CallTotal` sejak pengiriman. Gunakan `Plugin.CallWithBudget` untuk mendapatkan handle ke tenggat waktu panggilan yang sedang menunggu. Di `Scope.Post` milik host, panggil `budget.Extend(duration)` sebelum tenggat tersebut untuk memberi waktu lebih hanya pada panggilan ini.

`Plugin.Interrupt` menghentikan runtime dari goroutine mana pun, membuang timer yang tertunda, dan menggagalkan panggilan saat ini maupun berikutnya. `Plugin.Stopped()` dan `Plugin.Err()` juga dapat dibaca dari goroutine mana pun. `Err()` bernilai nil selama runtime berjalan dan mengembalikan alasan berhenti pertamanya setelah berhenti. Menutup plugin atau membuang pemiliknya biasanya menjalankan pembersihannya dan menghentikannya dengan `tsx.ErrPluginClosed`. Atur `Sandbox.OnStop` saat memanggil `tsx.Load` untuk menerima alasan tersebut sekali di goroutine engine, setelah panggilan yang menunggu gagal.

Atur `Sandbox.OnError` saat memuat plugin fungsi untuk menerima kegagalan timer, animation frame, `queueMicrotask`, dan `reportError` yang tidak tertangkap di luar `Plugin.Call` yang sedang menunggu. Handler setiap plugin berjalan di goroutine engine, sehingga host dapat mengatribusikan kegagalan dan memanggil `Plugin.Interrupt` saat pemutus (*breaker*)-nya terbuka. Pemotongan anggaran potongan native juga melaporkan `tsx.ErrBudget`, termasuk job promise yang panggilannya sudah selesai. Galat callback biasa membiarkan plugin tetap berjalan. Galat saat memuat dan hasil panggilan tetap dikembalikan ke pemanggilnya, dan view tetap menggunakan `core.Catch`; tanpa handler, perilaku boundary dan pelaporan aplikasi yang ada tetap berlaku.

Atur `Sandbox.OnViewRuntime` untuk menerima `*tsx.ViewRuntime` setiap view di goroutine engine sebelum modul entry-nya berjalan. Panggil `Interrupt` dari goroutine mana pun untuk menghentikan pekerjaannya dan membiarkan view yang terakhir dirender tetap di tempatnya. `Stopped` dan `Err` melaporkan state akhirnya, dengan `tsx.ErrViewClosed` pada pembuangan normal. `Sandbox.OnStop` juga melaporkan alasan akhir sebuah view sekali.

`Plugin.JSTime` dan `ViewRuntime.JSTime` melaporkan waktu yang dihabiskan di entri JavaScript runtime native, termasuk entri saat ini. Waktu menunggu di antara entri dan waktu di dalam fungsi Go sinkron yang terdaftar tidak dihitung. `ViewRuntime.EngineTime` juga mencakup panggilan Go sinkron serta pekerjaan reaktif, mounting, commit, dan layout miliknya, termasuk pekerjaan yang masih berjalan. Ini tidak mencakup waktu menunggu di antara entri engine, pekerjaan milik host atau view lain, pencatatan commit bersama, dan penggambaran native. Kedua penghitung aman dibaca dari goroutine mana pun. Giliran JavaScript browser di luar NSGo tidak dapat diukur atau didahului (*preempt*). Di sana kedua penghitung melaporkan tidak tersedia dan `Interrupt` menghentikan callback NSGo berikutnya saat host mendapatkan kembali kendali.

Web belum dapat mengisolasi paket dari halaman: kodenya akan menjangkau jaringan, clipboard, dan penyimpanan milik halaman, termasuk milik trust store, melampaui kapabilitasnya. Di sana `tsx.View` dan `tsx.Load` menolak paket dengan `tsx.ErrUnisolated`, kecuali Anda mengatur `Sandbox.WebUnisolated` untuk menjalankannya dengan otoritas halaman, yang hanya cocok untuk paket yang Anda percayai setara dengan aplikasi itu sendiri.

## Editor dan pengujian

`tsconfig.json` yang memperluas `.nsgo/tsconfig.json` memberi editor tipe-tipe TSX, yang ditulis oleh `go tool nsgo dev` dan `go tool nsgo check`. File `*.test.tsx` di samping aplikasi, yang ditulis dengan `nsgo/test`, berjalan bersama pengujian Go-nya ([Pengujian](testing.md)).

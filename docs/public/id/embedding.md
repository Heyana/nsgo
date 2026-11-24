---
title: Penyematan
description: Tempatkan aplikasi NSGo di dalam aplikasi iOS, Android, desktop, atau web milik Anda sendiri.
group: Perangkat bantu
order: 23
---

## Membangun aplikasi yang dapat disematkan

Aplikasi yang dapat disematkan adalah paket aplikasi biasa dengan `Root` dan `main`, yang dibangun dengan `-embed` untuk target host-nya. Setiap view menjalankan aplikasi sekali dan merupakan engine tersendiri, sehingga host dapat menampilkan view sebanyak yang diinginkan, masing-masing dengan state-nya sendiri.

```sh
go tool nsgo build -embed -target ios examples/embedded
go tool nsgo build -embed -target android_arm64 examples/embedded
go tool nsgo build -target web examples/embedded
```

Build iOS menghasilkan `<Name>.xcframework` dan `<Name>Resources`; build Android menghasilkan `<Name>.aar`; build web menghasilkan direktori dengan `nsgo.json` dan loader.

## iOS

Saat aplikasi mengimpor `base/terminal`, build juga menulis `GhosttyVT.xcframework` di samping framework aplikasi. Tambahkan pendamping ini ke target host dengan **Embed & Sign**, baik untuk framework aplikasi statis maupun `-dynamic`. Framework ini berisi pustaka perangkat, simulator Intel dan Apple silicon, serta Mac Catalyst; pertahankan nama `GhosttyVT.framework`-nya.

Tambahkan framework ke target aplikasi sebagai pustaka statis (jangan disematkan) dan folder sumber daya sebagai referensi folder, lalu `import <Name>`. Panggil `NSGoStart()` sekali saat aplikasi diluncurkan, sebelum view pertama. Kode pustaka statis menjadi bagian dari aplikasi host, sehingga manifes privasi host mendeklarasikan API required-reason yang digunakan NSGo: user defaults (`CA92.1`), stempel waktu file (`C617.1`), dan waktu boot sistem (`35F9.1`); framework `-dynamic` membawanya di `PrivacyInfo.xcprivacy`-nya sendiri.

```swift
import Embedded
import UIKit

@main
final class AppDelegate: UIResponder, UIApplicationDelegate {
	func application(_ app: UIApplication, didFinishLaunchingWithOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
		NSGoStart()
		return true
	}
}

let view = NSGoView(frame: .zero)
view.params = ["user": "Ada"]              // cx.HostParams()
view.dataDirectory = documents.path        // where storage lives
view.localeIdentifier = "de-CH"            // in place of the system's
view.setFontScale(1.3)
view.overrideUserInterfaceStyle = .dark
view.tokens = ["accent": "#cf222e"]        // over the app's theme
view.handle("time") { _, reply in reply(Date().description, nil) }
view.send("message", ["text": "Hello"])    // cx.OnHostEvent("message", …)
view.onOpenURL = { url in router.open(url) }
view.onError = { message in log(message) }
view.register(component: "host.badge") { context in Badge(context) }
```

`NSGoView` memulai aplikasi setelah berada di dalam jendela dan menghentikannya ketika didealisasi (deallocated) atau dikirimi `stop()`. Atur nilai peluncuran sebelum berada di jendela; sisanya dapat berubah kapan saja. Handler, komponen, dan callback berjalan di thread utama, dan balasan dapat datang kemudian dari thread mana saja. View controller dapat meneruskan `preferredStatusBarStyle`, `prefersStatusBarHidden`, dan `prefersHomeIndicatorAutoHidden` milik view untuk mengikuti pengaturan bilah sistemnya.
Host juga dapat mengembalikan `preferredScreenEdgesDeferringSystemGestures` milik view dari controller-nya untuk meneruskan `mobile.DeferSystemGestures`.

## Android

Untuk aplikasi arm64 yang mengimpor `base/terminal`, AAR menyertakan `jni/arm64-v8a/libghostty-vt.so` di samping `libnsgo.so`; host tetap mengaktifkan ekstraksi pustaka native. Prebuild Android dan loader terminal memerlukan [#1452](https://github.com/nsgo-dev/nsgo/issues/1452); pustaka terminal x86_64 belum tersedia.

Tambahkan AAR ke modul aplikasi dan kemas pustaka native secara terekstrak, agar runtime dapat menemukannya:

```kotlin
implementation(files("libs/Embedded.aar"))

android { packaging { jniLibs { useLegacyPackaging = true } } }
```

Buat `dev.oneo.nsgo.NsgoView` di kode atau di tata letak. Masing-masing memulai aplikasi di prosesnya sendiri saat pertama kali mendapatkan ukuran, dan `stop()` mengakhirinya. Teruskan hasil activity ke setiap view, dan tangani perubahan konfigurasi di activity agar aplikasi tidak dimulai ulang.

```java
NsgoView app = new NsgoView(context);
app.setParams(Map.of("user", "Ada"));
app.setStorageDirectory(dir);
app.setLocale(Locale.forLanguageTag("de-CH"));
app.setFontScale(1.3f);
app.setAppearance(NsgoView.APPEARANCE_DARK);
app.setTokens(Map.of("accent", "#cf222e"), null);
app.handle("time", (data, reply) -> reply.resolve(now()));
app.handle("tapped", (data, reply) -> status.setText("tapped " + ((Map<?, ?>) data).get("count")));
app.setOpenUrlHandler(url -> router.open(url));
app.setErrorListener(message -> log(message));
app.registerComponent("host.badge", Badge::new);
```

```java
@Override
protected void onActivityResult(int request, int result, Intent data) {
	super.onActivityResult(request, result, data);
	for (NsgoView app : apps) app.activityResult(request, result, data);
}
```

Untuk melepas pohon native sambil menjaga aplikasi Go tetap hidup, simpan `NsgoEngine` di host dan pasang view-nya secara eksplisit:

```java
NsgoEngine engine = new NsgoEngine(context.getApplicationContext());
app.attachToEngine(engine);
// Before taking the view off screen:
app.detachFromEngine();
// Reuse the same view later:
app.attachToEngine(engine);
// When the host ends the app:
engine.destroy();
```

Pelepasan membebaskan anak native; pemasangan kembali memulihkan signal, input, fokus, posisi gulir, dan gambar saat ini tanpa menjalankan ulang komponen. Halaman web dan pemutar dibuat ulang dari konfigurasi aplikasinya. `stop()` hanya melepas engine yang diberikan secara eksplisit. Setiap engine tetap mempertahankan view pertamanya dan Activity-nya serta menolak view lain, jadi tangani perubahan konfigurasi di Activity tersebut; menggantinya atau menampilkan view independen dalam satu proses belum didukung.

Untuk pemeriksaan pembaruan Google Play, tambahkan `com.google.android.play:app-update:2.1.0` ke dependensi Gradle host Android. Pertahankan API AppUpdateManager, AppUpdateInfo, AppUpdateOptions, dan Tasks yang dimuat NSGo berdasarkan nama menggunakan [aturan shrink runtime](https://github.com/nsgo-dev/nsgo/blob/main/internal/platform/android/runtime/play-review.pro). Penerusan hasil activity di atas juga mengirimkan pembatalan pembaruan.

Untuk sesi autentikasi Android di host yang disematkan, deklarasikan skema callback privat pada Activity hosting dan teruskan tautan yang diterima Activity tersebut ke `view.openLink(uri.toString())` di `onNewIntent` (dan `onCreate` baru). Sesi aktif yang cocok mengonsumsi callback-nya; tautan lain diteruskan ke `OnOpenLinks`. `NsgoActivity` mandiri meneruskannya sendiri.

## Desktop

Host Go menjalankan event loop platform dan menempatkan `Handle` setiap view di jendelanya. `nsgo.NewView` memulai aplikasi setelah handle berada di jendela, sehingga pengaturan yang dibuat sebelumnya sudah ada pada frame pertamanya.

```go
channel := core.NewHost()
channel.SetParams(map[string]any{"user": "Ada"})
channel.Handle("time", func(c *core.HostCall) { c.Resolve(time.Now().Format("15:04:05")) })
channel.HandleOpenURL(func(url string, answer func(bool)) {
	answer(strings.HasPrefix(url, "https://example.com/orders/"))
})

view := nsgo.NewView(app.Root, nsgo.HostChannel(channel), nsgo.DataDir(dataDir))
defer view.Close()

view.SetTokens(core.Tokens{Values: map[string]string{"accent": "#cf222e"}})
view.SetLocale("de-CH")
view.SetFontScale(1.3)
view.SetAppearance(core.AppearanceDark)
handle := view.Handle() // an NSView, GtkWidget or child window
```

`view.Host()` mengirim event ke aplikasi dan menerima panggilannya, dan `nsgo.Component` mendaftarkan komponen native untuk platform host.

## Web

Build web adalah halaman tersendiri, tetapi sebuah halaman dapat memasangnya di dalam sebuah elemen dan menjadi host-nya. `mount(element, url, options)` mengembalikan sebuah player, dan `nsgo.json` menyebutkan loader, wasm, dan sumber daya yang relatif terhadap dirinya sendiri.

```html
<div id="app"></div>
<script type="module">
const url = new URL("./Embedded-0.1.0-web/nsgo.json", location.href);
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url));

const player = await mount(document.getElementById("app"), url.href, {
	config,
	params: { user: "Ada" },
	host: {
		call(name) {
			if (name === "time") return new Date().toLocaleTimeString();
			throw new Error(`no host function ${name}`);
		},
		post(name, data) { console.log(name, data); },
		openURL(url) { return url.startsWith("https://example.com/orders/"); },
	},
});
player.hostEvent("message", { text: "Hello from the page" });
</script>
```

Tanpa opsi `host`, loader mengimpor modul yang disebutkan oleh `web.host` pada `nsgo.yaml` dan menunggu `createHost(player)` jika modul tersebut mengekspornya. Aplikasi di dalam elemen atau frame menyerahkan riwayat dan judul browser kepada halaman.

`appearance` (`"light"`, `"dark"`, atau `"system"`), `locale`, dan `fontScale` menggantikan pengaturan milik browser, sebagaimana pengaturan host native, dan `setAppearance`, `setLocale`, serta `setFontScale` milik player mengubahnya kemudian. `module`, sebuah `WebAssembly.Module` terkompilasi atau promise-nya, memungkinkan halaman yang memasang aplikasi berkali-kali mengunduh dan mengompilasinya sekali saja.

Loader menampilkan elemen `progress.nsgo-load-progress` sebelum aplikasi dimulai. Beri gaya dengan CSS, berikan `onProgress` di opsi mount, atau ekspor `onProgress` dari `web.host` untuk halaman milik aplikasi sendiri. Callback tersebut menggantikan bilah kemajuan dan menerima `{ loaded, total }` dalam byte terdekode, dengan `total` bernilai `null` jika ukurannya tidak diketahui. Loader memanggilnya dengan `null` saat startup berakhir agar UI-nya dapat dibersihkan. `module` terkompilasi melewati progres unduhan.

```yaml
web:
  host: host.js
```

## Saluran host

`core.Host` adalah satu kontrak yang diimplementasikan setiap host: parameter peluncuran, event yang dikirim host, dan fungsi host yang dipanggil aplikasi. Nilai melintas seperti JSON, sehingga map menjadi `map[string]any`, angka menjadi `float64`, dan kode yang sama bekerja di Swift, Java, Go, dan JavaScript.

- `HostParams` membaca parameter peluncuran sekali, saat dimulai.
- `cx.OnHostEvent` mendengar `Host.Send` dan `hostEvent` milik halaman.
- `cx.CallHost` memanggil handler bernama dan selesai dengan jawabannya; `cx.PostHost` memanggilnya tanpa jawaban.
- `Host.HandleOpenURL` melihat `cx.OpenURL` sebelum platform, dan jawabannya menentukan apakah platform membukanya juga.
- `Host.SetParams`, `Send`, `Handle`, dan `HandleDefault` aman dipanggil dari goroutine mana saja.

Di iOS host mengatur `params`, memasang handler dengan `handle`, mengirim event dengan `send`, dan menjawab `onOpenURL`. Di Android host melakukan hal yang sama dengan `setParams`, `handle`, `send`, dan `setOpenUrlHandler`. Di desktop host menggunakan metode milik `core.Host`, dan di web objek `host` milik halaman yang menjawab.

## Komponen native

`core.Native` menampilkan view yang dibuat oleh host: `Props`-nya mencapai `SetProp` komponen host, apa yang dipancarkan komponen mencapai `OnEvent`, dan prop yang nilainya berupa node animasi bergerak pada jam frame tanpa membebani engine.

```go
core.Native{
	Component: "host.badge",
	PropsFunc: func() map[string]any { return map[string]any{"count": count.Get()} },
	OnEvent:   func(name string, data any) { if name == "tap" { count.Set(0) } },
	Style:     core.Style{MinWidth: core.Pt(120), MinHeight: core.Pt(28)},
}
```

Daftarkan komponen di host: `app.register(component:)` dan sebuah `NSGoComponent` di iOS, `registerComponent` dan sebuah `NsgoComponent` di Android, `nsgo.Component` di desktop. Web tidak memiliki komponen native, sehingga nama yang tidak dikenal menampilkan kotak kosong di sana.

## Pengaturan

`tokens` menimpa tema aplikasi berdasarkan nama peran (`accent`, `inset`, `raised`), di kedua tampilan atau di salah satunya; `cx.Palette()` melihatnya. Host juga mengatur locale, ukuran teks, tampilan, dan direktori data, serta mendengar kegagalan aplikasi. `nsgo build -embed` tidak membuat jendela, menu, atau ikon baki: semuanya adalah milik host.

Panic internal pada goroutine engine suatu view menghentikan engine tersebut, menghancurkan permukaan yang terpasang, dan melaporkan panic serta stack-nya ke error listener host (`nsgo.OnError` di Go). Engine lain tetap berjalan; buat view baru untuk memulai ulang aplikasi yang gagal. Panic callback aplikasi biasa tetap masuk ke `core.Catch` terdekat terlebih dahulu.

`examples/embed-ios`, `examples/embed-android`, `examples/embed-desktop`, dan `examples/embed-web` adalah aplikasi yang sama dengan masing-masing jenis host.

## Berikutnya

- [Layanan platform](services.md): apa yang diminta aplikasi dari platform.
- [Membangun dan mendistribusikan](building.md): paket yang dibuat `-embed`, dan penandatanganannya.
- [Contoh: Aplikasi todo](tutorials/todo.md): aplikasi yang disematkan oleh host.

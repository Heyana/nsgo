---
title: Pengenalan
description: Apa itu NSGo, di mana ia berjalan, dan bagaimana cara kerjanya.
order: 1
---

NSGo adalah framework UI native lintas platform yang ditulis dalam bahasa Go. Anda menulis aplikasi satu kali, dalam Go atau TSX, dan aplikasi berjalan di iOS beserta App Clips-nya, tvOS, watchOS, Android, macOS, Windows, Linux, dan web, digambar menggunakan view asli dari masing-masing platform. Aplikasi Go juga dapat berjalan di mini program WeChat, QQ, Alipay, dan Douyin secara eksperimental: platform-platform tersebut membatasi ukuran aplikasi hingga 2 MB, yang sering terlampaui oleh aplikasi yang lebih besar. HarmonyOS menjalankan aplikasi Go sebagai pratinjau (*preview*), digambar menggunakan ArkUI.

## Apa yang Anda tulis

Sebuah aplikasi adalah paket `main` Go. Komponen-komponennya mengembalikan elemen: view, teks, gambar, input, dan kontrol yang ada di `ui`. State berada di dalam signal, dan hanya bagian yang membaca suatu signal yang diperbarui ketika nilainya berubah.

```go
func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }},
	}}
}
```

Aplikasi yang sama dapat ditulis dalam TSX dan diberi gaya dengan kelas Tailwind ([Aplikasi TSX](tsx.md)).

## Cara kerjanya

Sebuah mesin (*engine*) Go memegang kendali aplikasi: graf reaktif, gaya, tata letak flexbox dan grid, serta persiapan teks. Setiap pembaruan menghasilkan satu batch perubahan, dan backend tipis menerapkannya ke view native milik platform. Animasi berjalan pada frame clock platform, sehingga tidak ada yang di-commit lebih dari sekali per frame.

| Platform | Menggambar dengan |
| --- | --- |
| iOS | UIKit dan CoreText |
| Android | Android Views |
| macOS | AppKit dan CoreText |
| Windows | Direct2D dan DirectWrite |
| Linux | GTK 4, Pango, dan Cairo |
| Web | WebAssembly dan elemen DOM |

Setiap target dibangun di Mac menggunakan Go; iOS juga membutuhkan Xcode. Tool `nsgo` membuat, menjalankan, menguji, dan mengemas aplikasi.

## Status

NSGo masih pada tahap awal: 0.1.0 adalah rilis pertamanya, API-nya belum stabil, dan hal-hal yang belum tersedia tercantum dalam [peta jalan](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md). Proyek ini memporting desain [Native Shell](https://github.com/longbridge/nsh) ke Go.

## Cara membaca dokumentasi

Setiap halaman menjawab satu pertanyaan. Contoh kode merupakan potongan komponen kecuali halaman menyatakan lain: `cx` adalah `*core.Scope` yang diterima oleh sebuah komponen. Aplikasi menggunakan komponen milik `ui`, yang sudah memiliki tampilan; `base` berisi kontrol yang sama tanpa tampilan, bagi pembuat komponen.

- **Mulai**: [penyiapan](getting-started.md), lalu contoh aplikasi yang dibangun langkah demi langkah, [aplikasi todo](tutorials/todo.md), [aplikasi catatan](tutorials/notes.md), dan [repositori GitHub populer](tutorials/repos.md).
- **Perangkat bantu**: [menjalankan dan men-debug](debugging.md), [pengujian](testing.md), [membangun dan mendistribusikan](building.md), dan [penyematan](embedding.md).
- **Komponen**: [komponen yang paling sering digunakan](ui.md), varian, status, dan aksesibilitasnya.
- **Konsep inti**: [komponen dan state](components.md), [tata letak](layout.md), [gaya](styles.md), [teks](text.md), [daftar](lists.md), [navigasi](navigation.md), [formulir](forms.md), [pelokalan](localization.md), [TSX](tsx.md), dan lainnya.
- **Fitur platform**: [layanan platform](services.md), [notifikasi](notifications.md), [haptik](haptics.md), [widget](widgets.md), [Live Activities](live-activities.md), dan [izin](permissions.md).
- **Menulis komponen**: [komponen untuk aplikasi orang lain](authoring/index.md), tanpa gaya dan bergaya.
- **Berkontribusi**: [bangun NSGo dari kode sumber](contributing.md), pahami strukturnya, dan kirimkan perubahan.
- [Demo langsung](https://nsgo.dev/gallery/): setiap komponen, di dalam browser Anda.

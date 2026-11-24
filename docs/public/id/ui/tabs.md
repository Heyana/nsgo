---
title: Tabs
description: Daftar tab, kontrol tersegmentasi, dan bilah tab sistem.
group: Komponen
order: 40
---

`ui.Tabs[T]` memilih satu dari beberapa nilai, sebagai kontrol tersegmentasi atau daftar tab; aplikasi menampilkan konten nilai yang dipilih di sampingnya. Daftar tab adalah satu perhentian Tab: tombol panah berpindah di antara tab-tabnya.

```go preview=tabs/basic
section := core.NewSignal(cx, "overview")
ui.Tabs[string]{
	Options: []ui.Option[string]{
		{Value: "overview", Label: "Overview"},
		{Value: "activity", Label: "Activity"},
		{Value: "settings", Label: "Settings"},
	},
	Value:    section.Get,
	OnChange: section.Set,
	Label:    "Sections",
}
```

## Varian dan ukuran

`Variant` bernilai `TabsSegmented` (bawaan, sebuah thumb yang terangkat di dalam trek yang menjorok) atau `TabsUnderline` (garis aksen di bawah tab yang dipilih). Garis bawah dapat mengatur `IndicatorColor` dan menggunakan `IndicatorFit: TabsIndicatorContent` agar mengikuti label dan ikonnya, alih-alih seluruh tab; di TSX keduanya adalah `indicatorColor` dan `indicatorFit="content"`. `Size` bernilai `SizeDefault`, `SizeSmall`, atau `SizeLarge`, dan `Equal` memberi setiap tab lebar yang sama alih-alih menyesuaikan dengan labelnya.

```go preview=tabs/underline
ui.Tabs[string]{Variant: ui.TabsUnderline, Equal: true, Size: ui.SizeSmall, Options: tabs, Value: section.Get, OnChange: section.Set}
```

Sebuah opsi dapat membawa `Icon`, fungsi `Disabled`, node `Content`, dan `TestID`. `Content` menggantikan label dan mewarisi gaya teks tab.

Gunakan `OptionsFunc` ketika tab mengikuti sebuah signal. NSGo memperbarui baris di tempat berdasarkan nilainya, dengan tetap mempertahankan tab yang dipilih dan fokus keyboard selama nilai itu masih ditawarkan; nilai yang menghilang dibiarkan tidak terpilih hingga aplikasi mengubahnya.

## Bilah tab

`ui.TabShell` menempatkan bagian-bagian tingkat atas di samping bilah tab pada setiap lebar jendela — di bawah secara bawaan, atau secara vertikal di sepanjang tepi awal atau akhir ketika `Edge` (atau `EdgeFunc`) bernilai `mobile.TabBarLeading`, `mobile.TabBarTrailing`, atau `mobile.TabBarAutomatic` (yang mengikuti `cx.Env().VerticalBarEdge()`). Ia menggunakan bilah milik sistem ketika setiap tab menyediakan `SystemIcon` atau `Icon` SVG, di tepi mana pun di iOS dan di tepi bawah di Android; `SystemIcon` yang menamai SF Symbol di iOS atau drawable di Android didahulukan, dan SVG ditampilkan jika tidak. Selain itu, ia menggambar bilahnya sendiri dengan ikon SVG yang disediakan. Bilah yang mengambil ruangnya sendiri menjaga jarak dari safe area di tepinya sendiri, sehingga `core.EnvSafeArea` di tepi tersebut pada bagian di sampingnya, termasuk halaman `base.Nav`, bernilai 0; di bawah bilah sistem bawah iOS yang mengambang, jaga konten tetap bebas dengan `ui.TabBarInset`.

Atur `ShellTab.TextFunc` untuk judul tab yang mengikuti locale atau state aplikasi dan `TabShell.LabelFunc` untuk nama aksesibel bilah. Mengubah judul tetap mempertahankan bagian yang dipilih dan tab yang ada selama ikonnya tetap sama.

`mobile.TabBar` adalah bilah tab milik sistem di iOS dan Android, dengan `Items`, `Selected`, `OnChange`, serta `Edge` opsional (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing`, atau `mobile.TabBarAutomatic`) / `EdgeFunc`. Setiap `mobile.TabBarItem` menerima `Title` dan nama ikon platform di `Icon`; `SVG: lucide.House()` ditampilkan di mana pun `Icon` tidak menamai SF Symbol di iOS atau drawable di Android, disesuaikan ke dalam 24 poin dan diwarnai sesuai pilihan. Di TSX, `svg` menerima dokumen SVG dan `edge` menerima `"bottom"`, `"leading"`, `"trailing"`, atau `"automatic"`. Di tempat lain `mobile.TabBar` tidak mengambil ruang, jadi tampilkan `ui.Tabs` di sana.

`ui.SidebarShell` menjaga isi sidebar, footer, header, konten, dan panel kanannya tetap bebas dari safe area kiri dan kanan sendiri, dengan latar belakangnya terbentang hingga tepi jendela, sehingga `core.EnvSafeArea` di sisi-sisi tersebut bernilai 0 di dalamnya, dan `ui.NavHeader` menambahkan safe area samping ke padding-nya. Konten `ui.NavPage` atau bagian `TabShell` menjaga jarak dari safe area sendiri, dengan `PaddingEnv`, sehingga apa yang seharusnya terbentang hingga tepi, seperti gambar full-bleed, dapat melakukannya.

## Aksesibilitas

Daftar melaporkan `RoleTabList`, setiap tab `RoleTab`, dan tab yang dipilih melaporkan state-nya. Daftar adalah satu perhentian Tab; Left dan Up, Right dan Down, Home dan End berpindah di antara tab yang aktif, dan tab yang dicapai melaporkan sebuah tekanan. `Label` menamai daftar.

## Platform

iOS dan Android menampilkan bilah tab milik sistem dengan tampilan dan gesturnya sendiri; macOS, Windows, Linux, dan web menampilkan `ui.Tabs`. Konten aplikasi sama di setiap target.

## Terkait

- [Navigasi](../navigation.md): halaman dan bilah tab di dalam aplikasi.
- [Menu](menu.md): perintah, bukan bagian.
- [Select](select.md): banyak opsi di dalam daftar.
- Halaman Tabs dan NavStack di galeri.
- [Komponen tanpa gaya](../authoring/base.md#tabs): `base.Tabs`, untuk tab dengan tampilan Anda sendiri.

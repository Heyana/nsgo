---
title: Gaya dan tema
description: Gaya bertipe, kelas Tailwind, gaya status, serta tema terang dan gelap.
group: Konsep inti
order: 52
---

## Sebuah gaya

`core.Style` adalah satu nilai Go untuk tata letak, tampilan, dan teks sebuah kotak. Nilai nol setiap field berarti tidak diatur, sehingga gaya dapat digabungkan: `Merge` hanya mengganti apa yang diatur oleh gaya yang belakangan, dan anak mewarisi field teks yang diatur oleh induknya. Untuk mengosongkan panjang di atas gaya yang memberinya nilai, gunakan `core.Zero` di `Padding`, `Margin`, `Borders`, `BorderWidth`, `CornerRadius`, atau gap, sebagaimana `core.Transparent` berlaku untuk warna: `Padding: core.All(core.Zero)`.

```go
card := core.Style{
	Background: core.RGB(0xffffff),
	CornerRadius: 12,
	Padding: core.All(16),
	Gap: 12,
	BorderWidth: 1,
	BorderColor: core.RGBA(0x000000, 0.1),
}
```

Gaya menerima warna, sudut, border, outline, bayangan, gradien, filter, blur latar belakang (*backdrop blur*), mask, mode blend, transformasi 3D, dan transisi dari CSS. `core.RGB`, `core.RGBA`, dan `core.ParseColor` membuat warna, `core.LinearGradient` dan sejenisnya membuat gradien, `core.Ring` membuat ring ala Tailwind, dan `core.AllCorners` memberikan radius untuk setiap sudut. `BorderCurveContinuous` membulatkan sudut hingga menyatu dengan tepinya seperti di iOS. Bayangan dan outline sebuah kotak tetap berada di luar klip `Overflow`-nya kecuali di tempat yang disebutkan oleh [Tata letak](layout.md#posisi). Filter latar belakang mengubah apa yang ada di belakang sebuah tampilan seperti `backdrop-filter` di CSS: `Blur` mengaburkannya sesuai radiusnya dan filter warna mengubah warnanya. iOS menggambar material sistem yang dinamai oleh `View.IOS.BackdropMaterial` untuk blur tanpa filter warna; blur yang menamai material selain `core.BackdropUltraThin` menggambar material tersebut di iOS dan material AppKit yang paling mendekati di macOS, tanpa menyertakan filter warna. Linux dan Android tidak menggambar filter latar belakang. `Style.GlassEffect` (`core.GlassRegular` atau `core.GlassClear`, `glassEffect` di TSX) menggambar Liquid Glass sebagai pengganti blur latar belakang di macOS 26 dan iOS 26 (serta shader kaca milik NSGo di Windows; berikan blur pada tampilan kaca sebagai cadangan di tempat lain). Mengatur `Style.GlassInteractive` (`glassInteractive` di TSX) mengaktifkan respons tekan interaktif milik sistem untuk Liquid Glass di macOS 27 (`NSGlassEffectView.effectIsInteractive`) dan iOS 26 (`UIGlassEffect.isInteractive`) sambil tetap menjaga `OnPress` dan anak yang dapat ditekan berfungsi.

`Transform` memindahkan target pointer bersama tampilan: tekanan, hover, dan gestur dimulai di tempat tampilan digambar, dan posisi pointer yang dilaporkan membatalkan transformasi tersebut. Anak yang ditransformasi di luar induknya tetap dapat dijangkau kecuali klip atau viewport gulir menyembunyikannya. Mini program belum membatalkan keyframe yang dijalankan oleh lapisan rendernya ([peta jalan](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md#mini-programs)).

```go
core.View{Style: core.Style{
	CornerRadii: core.Corners{TopLeft: 28, BottomRight: 28},
	BorderCurve: core.BorderCurveContinuous,
	Shadows:     []core.BoxShadow{core.Ring(1, core.RGBA(0x000000, 0.08)), ui.Shadow(0, 8, 24, -6, ui.Black(0.18))},
}}
```

## Status

Tampilan sebuah view dapat berubah sesuai statusnya. `Hover`, `Focus`, `FocusWithin`, `Active`, `Selected`, `Checked`, `Disabled`, dan status `aria-*` adalah nilai `*Style` yang digabungkan di atas gaya lainnya selama status tersebut berlaku. Aplikasi mengatur `Selected`, `Checked`, `Disabled`, dan `Expanded` melalui fungsi, dan platform melaporkan tekanan serta fokus.

`Active` dan `group-active:` mengikuti view selama view itu ditekan, yang memerlukan handler tekan atau `Responder` yang memegang sentuhan. Memberi gaya pada view biasa dengan `Active`, `active:`, atau `group` tidak membuatnya dapat ditekan atau menjadi hentian Tab papan ketik; atur `Focusable` jika view itu sendiri perlu menerima fokus.

```go
core.View{
	OnPress: func(core.PressEvent) { open() },
	Style: core.Style{
		Background: core.RGB(0x007aff),
		Active:     &core.Style{Opacity: core.Alpha(0.85)},
		Hover:      &core.Style{Background: core.RGB(0x006be0)},
	},
	Disabled: func() bool { return saving },
}
```

Bagian dari sebuah komponen dapat mengambil status kontrol di sekitarnya tanpa melaporkannya sendiri: `PartStates` memberikan `Checked`, `Selected`, `Disabled`, dan lainnya, sehingga tanda centang pada kotak centang juga mengikuti kotak centang tersebut bagi pembaca layar.

## Kelas

`Class` pada `View`, `Text`, `Span`, `Input`, `Image`, `Scroll`, dan `Canvas` menerima kelas Tailwind CSS 4, dan setiap bagian dari komponen `base` memiliki kelas di samping gayanya. Utilitas, varian, dan nilai arbitrer berarti sama seperti di Tailwind, dengan tema dan palet bawaannya.

```go
core.View{Class: "flex items-center gap-3 rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"}
```

Kelas digabungkan di bawah gaya, sehingga field gaya yang menang. Kelas dapat mengikuti apa yang berubah dengan `ClassFunc`, dan varian status (`hover:bg-zinc-100`, `checked:opacity-100`) menjadi status gaya. `group` dan `group-hover:` memberi gaya pada anak dari status induknya, `first:` dan `last:` mengikuti posisi sebuah anak, dan `aria-checked:`, `data-open:`, serta `focus-within:` mengikuti status yang ditampilkan kontrol. `pointer-fine:` dan `pointer-coarse:` memberi gaya pada subpohon untuk mouse atau layar sentuh, sesuai dengan fitur media `pointer` di CSS melalui `cx.Env().Pointer()` (kueri `pointer` milik browser di web, fine di desktop dan Mac, coarse di ponsel, tablet, dan jam tangan, serta tidak keduanya di TV), dan dapat dipadukan dengan varian lain seperti `dark:` dan `hover:` sehingga aplikasi memilih ukuran untuk sentuhan atau pointer secara eksplisit alih-alih `core` mengubahnya secara implisit.

Tandai kontrol dengan `peer` untuk memberi gaya pada saudara (*sibling*) berikutnya dari statusnya, misalnya kotak centang dengan `Class: "peer"` diikuti label dengan `Class: "peer-checked:text-accent peer-disabled:opacity-50"`. Peer hanya memberi gaya pada saudara setelahnya di bawah induk yang sama, termasuk `Show` dan `Each`, tidak pernah saudara sebelumnya atau view di dalamnya. Gunakan `peer/field` dengan `peer-checked/field:` untuk menamai satu set, dan `peer-data-invalid:` untuk status data boolean `invalid` milik kontrol. Penanda dan status diperbarui secara reaktif. Menandai view tidak menjadikannya hentian Tab atau membuatnya menerima tekanan, sehingga `peer-active:` hanya mengikuti peer yang menerima tekanan sendiri, seperti tombol.

`has-[>svg]:px-3` mengubah padding induk ketika memiliki anak ikon langsung; `[&_svg]:size-4 [&_svg]:shrink-0` mengatur ukuran ikon turunannya. `svg` memilih `core.Icon` dan `SVGView`, termasuk melalui wilayah kondisional. Selektor-selektor ini diperbarui saat anak, kelas, atau status berubah. `hover:[&_svg]` mengikuti hover induk; `[&_svg]:hover` mengikuti hover masing-masing ikon. `active` pada selektor hanya mengikuti view yang menerima tekanan, seperti tombol, sebagaimana `peer-active:`. Selektor yang cocok bersaing berdasarkan spesifisitas sebelum urutan varian; gaya inline menang kecuali kelasnya bersifat important.

Selektor mencocokkan elemen NSGo di setiap target. Selektor mendukung daftar, kombinator anak (`>`), turunan (`_`), saudara bersebelahan (`+`), dan saudara berikutnya (`~`), kelas, keberadaan/kesamaan `data-*`, peran, status aksesibilitas boolean yang didukung, serta `:is`, `:where`, `:not`, `:has`, dan posisi anak. Tipenya adalah `view`/`div`, `text`/`span` (node teks utuh), `scroll`, `input`, `textarea`, `image`/`img`, `canvas`, `svg`, `video`, `webview`, dan `native`. Gunakan satu `&` tingkat atas di setiap alternatif selektor arbitrer; tanpanya, selektor menguji elemen itu sendiri. Selektor khusus browser, pseudo-elemen, at-rule, escape, dan `&` eksplisit di dalam fungsi tidak dikenali; selektor-selektor ini tidak menjangkau view implementasi backend atau span teks individual.

Varian `peer-*` di `Span.Class` atau `Screen.HeaderClass` tidak memiliki saudara untuk diikuti. Varian tersebut diabaikan dan dicatat sekali per kelas dan konteks; letakkan varian peer milik span pada `Text` yang memuatnya.

Field `Tailwind` milik tema memperluas tema bawaan dengan warna, breakpoint, dan utilitas, dan warna yang tidak ada di palet menamai peran `Palette` (`bg-accent`, `text-text-muted/70`) atau warna tema, yang dibaca dari tema node dalam mode tampilan yang ditunjukkannya.

## Tema

`core.Theme` adalah satu tampilan dalam kedua mode tampilan: `Palette` untuk terang dan satu lagi untuk gelap, radius sudut, dan keluarga font. Peran palet adalah `Background`, `Surface`, `Text`, `TextMuted`, `Border`, `Accent`, `AccentText`, `Danger`, `Warning`, `Success`, `Selection`, `Focus`, dan `Scrollbar`; `Colors` menyimpan peran yang ditambahkan pustaka, seperti `inset` dan `raised`. `core.DefaultTheme()`, yang ditampilkan aplikasi sampai aplikasi mengatur tema lain, adalah tema netral dengan aksen biru dan peran-peran milik `ui`, dan `ui.WarmDuskTheme()` adalah palet hangat.

```go
theme := core.DefaultTheme()
theme.Light.Accent, theme.Dark.Accent = core.RGB(0xbd481b), core.RGB(0xed986e)
theme.Radius = 10
cx.SetTheme(theme)
```

`cx.SetTheme` memberikan tema kepada scope dan semua yang dibangun di bawahnya, sehingga sidebar atau panel dapat memiliki temanya sendiri; tema nil mengambil tema di atasnya. `core.ParseTheme` membaca file tema YAML atau JSON, atau variabel CSS dari file tema shadcn/ui, dengan warna peran, radius, dan keluarga font. `core.DefaultTheme`, tema yang ditampilkan aplikasi sampai aplikasi mengatur satu, dan `ui.WarmDuskTheme` mengembalikan tema yang siap diubah.

`cx.SetAppearance` memaksa seluruh aplikasi menjadi terang atau gelap, `cx.SetSubtreeAppearance` melakukannya untuk sebuah subpohon, dan `core.AppearanceSystem` kembali mengikuti sistem. `cx.Dark()` menunjukkan mode tampilan yang ditampilkan sebuah subpohon, dan `cx.Palette()` memberikan warnanya dengan penimpaan dari host penyemat diterapkan.

Jika tidak ada leluhur yang mengatur `Style.Color`, teks dan ikon `currentColor` menggunakan `Palette.Text` milik subpohon, mengikuti perubahan tema, mode tampilan, dan token host secara langsung. Warna eksplisit, termasuk `core.Transparent`, tetap diwariskan.

## Warna dalam komponen

Komponen membaca warnanya di tempat ia dibangun dan di dalam `StyleFunc`-nya, sehingga perubahan tema atau mode tampilan diikuti secara langsung, tanpa membangun ulang komponen. `ui.ColorsOf(cx)` menamai peran yang digunakan `ui`, `ui.TintsOf(cx)` memberikan isian tembus pandangnya, dan `ui.FinOf(cx)` memberikan lapisan finishing milik `ui`.

```go
colors := ui.ColorsOf(cx)
core.Text{Content: "Hello", StyleFunc: func() core.Style {
	return core.Style{Color: colors.Text(), FontSize: 15}
}}
```

`StyleFunc` berjalan lagi saat apa yang dibacanya berubah, sehingga `cx.Dark()` dan `cx.Palette()` di dalamnya mengikuti tema. Hal yang sama berlaku untuk `ClassFunc`, `StyleFunc` milik span, dan field `StyleFunc` milik komponen.

## Kursor

`Style.Cursor` mengatur kursor pointer bernama (`core.CursorText`, `core.CursorColResize`, dan seterusnya); biarkan elemen yang dapat ditekan pada `core.CursorAuto` agar tetap memakai panah milik platform. Untuk alat gambar, game, atau pointer kanvas kustom di desktop dan web, `Style.CursorImage` menerima `*core.CustomCursor` dengan `Image` 1x, `Image2x` HiDPI opsional (dipilih otomatis saat `PixelRatio >= 1.5`), `Hotspot` dalam titik logis yang dibatasi ke batas gambar, dan kursor bernama `Fallback`. Periksa `cx.Env().SupportsCustomCursor()` untuk mengetahui apakah target saat ini menampilkan gambar kursor kustom; platform sentuh mengabaikannya.

## Selanjutnya

- [Tata letak](layout.md): di mana kotak ditempatkan.
- [Teks](text.md): font, span, dan dekorasi.
- [Animasi](animation.md): transisi antargaya.

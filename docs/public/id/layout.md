---
title: Tata letak
description: Flexbox, grid, ukuran, dan penentuan posisi, dengan safe area dan papan ketik di dalam tata letak.
group: Konsep inti
order: 51
---

## Kotak

Setiap elemen adalah sebuah kotak yang ditata oleh mesin tata letak. Nilai default-nya mengikuti React Native: flexbox kolom, anak mempertahankan ukurannya kecuali jika tumbuh atau menyusut, item diregangkan melintasi sumbu silang (*cross axis*), dan ukuran sudah mencakup padding serta border.

```go
core.View{
	Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.All(16), AlignItems: core.AlignCenter},
	Children: []core.Node{
		core.Text{Content: "One"},
		core.Text{Content: "Two"},
	},
}
```

`Column`, `Row`, `ColumnReverse`, dan `RowReverse` mengatur sumbu utama; `Wrap` dan `WrapReverse` membiarkan anak mengalir ke baris tambahan. `JustifyContent` menempatkannya di sepanjang sumbu utama, sementara `AlignItems`, atau `AlignSelf` pada satu anak, menempatkannya melintasi sumbu tersebut. `Gap`, `RowGap`, dan `ColumnGap` memisahkannya, sedangkan padding dan margin pada `style` menggunakan `core.All`, `core.Symmetric`, atau `core.Edges`.

## Ukuran

`Width`, `Height`, `MinWidth`, `MaxWidth`, dan `FlexBasis` menerima `core.Pt` untuk satuan poin atau `core.Pct` untuk persentase dari induknya. `FlexGrow` membagi ruang kosong dan `FlexShrink` membiarkan kotak menyusut saat ruang sempit; kotak tanpa keduanya mempertahankan ukurannya. `AspectRatio` adalah lebar dibagi tinggi. `AutoMargin` mengambil ruang kosong pada sisi yang disebutkannya, sehingga satu sisi dapat mendorong kotak ke sisi lain.

```go
core.View{Style: core.Style{Direction: core.Row, Gap: 16}, Children: []core.Node{
	core.View{Style: core.Style{Width: core.Pt(220), FlexShrink: 0}},
	core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}},
}}
```

Konten kotak menentukan ukurannya sampai lebar atau tinggi menyatakan sebaliknya. `core.MinContent`, `core.MaxContent`, dan `core.FitContent` mengambil ukuran sebagaimana definisi CSS: lebar min-content sebuah teks adalah kata terlebarnya. Teks di dalam baris hanya menyusut dan membungkus baris (*wrap*) jika memiliki `FlexShrink: 1`; tambahkan `FlexGrow: 1` bila teks harus mengambil sisa ruang yang ada. Nilai minimum yang tidak diatur sudah bernilai 0, seperti di React Native, sehingga `MinWidth: core.Pt(0)` tidak mengubah apa pun dan teks yang menyusut akan memutus kata ketika baris lebih sempit daripada kata terlebarnya. `MinWidth: core.AutoMinimum` menahannya pada kata terlebarnya, sebagaimana nilai minimum otomatis pada CSS.

Layar yang memenuhi jendela menggunakan `FlexGrow: 1` dan `FlexShrink: 1`, begitu pula setiap kotak di antara layar tersebut dan daftar atau scroll view yang harus menggulir. Dengan `FlexGrow` saja, sebuah kotak akan membesar mengikuti kontennya, sehingga daftar yang panjang akan meregangkannya melewati jendela alih-alih menggulir.

## Grid

`DisplayGrid` menata anak dalam CSS grid. `GridTemplateColumns` dan `GridTemplateRows` menerima sintaks track CSS, sementara `GridRow` dan `GridColumn` menempatkan item berdasarkan garis atau rentang (*span*).

```go
core.View{Style: core.Style{
	Display: core.DisplayGrid,
	GridTemplateColumns: "repeat(3, 1fr)",
	GridAutoRows: "44px",
	Gap: 12,
}, Children: []core.Node{
	core.View{Style: core.Style{GridColumn: core.GridLines{Start: core.GridSpan(2)}}},
}}
```

`repeat(auto-fill, minmax(160px, 1fr))` membuat kolom sebanyak yang muat. `GridFlow` memilih urutan pengisian grid, dan `JustifyItems`, `JustifySelf`, serta `AlignItems` menempatkan item di dalam selnya.

## Posisi

`Relative` adalah default dan menjaga kotak tetap berada di dalam alur; `Top`, `Right`, `Bottom`, dan `Left` menggesernya. `Absolute` mengeluarkannya dari alur dan menempatkannya relatif terhadap leluhur (*ancestor*) terdekat yang diposisikan. `Fixed` menempatkannya di root view atau modalnya, tidak peduli apa yang digulir, dan `Sticky` menahannya di dalam induknya pada scroll view terdekat: `Top` dari atas, atau `Bottom` dari bawah saat `Top` bernilai auto (`Left` dan `Right` secara horizontal). Footer dengan `Position: core.Sticky, Bottom: core.Pt(0)` tetap berada di bawah saat konten panjang digulir dan mengalir setelah konten pendek. `ZIndex` menumpuk kotak di antara saudara-saudaranya.

```go
core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Right: core.Pt(8)}}
```

`Static` menjaga kotak di dalam alur dan mengabaikan inset-nya, sehingga anak yang diposisikan secara absolut mencari leluhur di atasnya. `Overflow` memotong anak sesuai batas kotak, dan `DisplayNone` mengeluarkan kotak dari tata letak.

Bayangan luar dan garis tepi (*outline*) milik kotak sendiri tetap berada di luar area pemotongan (*clip*), sebagaimana di CSS, pada Android, Windows, Linux, web, dan Apple, berlaku untuk setiap jenis view, termasuk sudut dengan radius berbeda dan konten gulir yang diperbesar. Sebelum hal itu diperiksa di iPhone fisik, `Bare` milik `ui.Dialog` tetap menempatkan pemotongan pada view dalam dengan sudut yang sama, diberi inset oleh border, yang menjaga tampilannya di setiap target:

```go
core.View{Style: core.Style{CornerRadius: 16, Shadows: []core.BoxShadow{ui.Shadow(0, 8, 24, -6, ui.Black(0.18))}},
	Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, CornerRadius: 16, Overflow: core.Hidden}, Children: content}}}
```

## Arah dan lingkungan

`LayoutDirection` menata subpohon dari kiri ke kanan atau kanan ke kiri, menimpa arah aplikasi: `DirectionRTL` dan `DirectionLTR` mengaturnya, dan nilai default mewarisinya. Kiri dan kanan pada margin, padding, border, dan inset bertukar mengikuti arah, sehingga berfungsi sebagai awal (*start*) dan akhir (*end*) seperti di React Native. `PhysicalEdges` dan `PhysicalBorders` mempertahankan sisi yang disebutkan agar tidak bertukar. `WritingDirection` mengurutkan teks dengan arah campuran, dan `WritingAuto` menentukannya dari huruf kuat pertama.

`PaddingEnv` dan `MarginEnv` menambahkan nilai lingkungan pada suatu sisi. `core.EnvSafeArea` adalah inset safe area pada tepi tersebut dan `core.EnvKeyboard` adalah tinggi papan ketik perangkat lunak, sehingga konten tetap berada di atas papan ketik sementara pergerakannya beranimasi mengikutinya.

```go
core.View{Style: core.Style{
	Padding: core.All(16),
	PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea},
}}
```

`core.EnvAll(core.EnvSafeArea)` menyebutkan setiap tepi, dan `cx.Env().SafeArea()` serta `cx.Env().KeyboardHeight()` membaca nilainya di dalam kode.

Pada ponsel lipat atau layar ganda, `cx.Env().Geometry()` mencantumkan wilayah-wilayah tempat lipatan atau engsel membagi jendela, masing-masing dengan safe area yang jatuh di dalamnya, serta lipatan di antara keduanya; `State` dan `Angle`-nya menyatakan seberapa jauh perangkat terbuka (`core.HingeClosed`, `HingeHalfOpen`, atau `HingeOpen`) bahkan ketika tidak ada lipatan yang melintasi jendela, seperti pada layar sampul atau layar yang dibuka rata. Perangkat lain melaporkan satu wilayah dan `HingeUnknown`. Di iOS 27.1, `cx.Env().VerticalBarEdge()` melaporkan tepi yang dipilih sistem untuk bilah vertikal (`core.VerticalBarLeading`, `core.VerticalBarTrailing`, atau `core.VerticalBarUnspecified`), yang diikuti oleh `mobile.TabBar` dan `ui.TabShell` ketika `Edge` bernilai `mobile.TabBarAutomatic`. Tidak ada yang ditata secara implisit berdasarkan geometri: aplikasi memilih panelnya sendiri.

## Kelas responsif

`Class` menerima kelas Tailwind CSS: `flex`, `flex-col`, `flex-wrap`, `items-center`, `justify-between`, `gap-3`, `p-4`, `w-full`, `grid`, `grid-cols-3`, `col-span-2`, dan seterusnya. Breakpoint mengikuti jendela, sementara tampilan (*appearance*), arah, dan pengurangan gerakan (*reduced motion*) mengikuti lingkungan:

```go
core.View{Class: "flex flex-col gap-3 p-4 md:flex-row md:gap-6 dark:bg-zinc-900 rtl:flex-row-reverse"}
```

`sm:`, `md:`, `lg:`, `xl:`, dan `2xl:` berlaku mulai lebar 40, 48, 64, 80, dan 96 rem; `max-md:` berlaku di bawahnya. `portrait:`, `dark:`, `rtl:`, `motion-reduce:`, `contrast-more:`, `pointer-fine:`, `pointer-coarse:`, `ios:`, `android:`, dan `web:` mengikuti apa yang dilaporkan oleh lingkungan. Kelas digabungkan di bawah `Style`, dan kelas untuk status (`hover:`) menjadi gaya status, sehingga keduanya dapat dipadukan.

## Selanjutnya

- [Gaya dan tema](styles.md): warna, sudut, bayangan, dan tampilan subpohon.
- [Teks](text.md): bagaimana teks mengambil ukuran dan membungkus baris.
- [Daftar dan pengguliran](lists.md): konten yang menggulir dan baris yang didaur ulang.

---
title: Teks
description: Menampilkan teks dengan span, font, dekorasi, seleksi, dan dokumen.
group: Konsep inti
order: 53
---

## Sebuah teks

`core.Text` adalah satu paragraf, yang ditata oleh mesin teks milik platform di sisi engine dan digambar oleh thread utama. Ukurannya berasal dari font dan lebarnya, dan teks membungkus (*wrap*) di dalam kotaknya.

```go
core.Text{Content: "Hello from NSGo", Style: core.Style{
	FontSize:   15,
	LineHeight: 22,
	FontWeight: core.FontWeightSemibold,
}}
```

`Content` bersifat tetap dan `ContentFunc` mengikuti sebuah signal, sehingga teks yang berubah membacanya di dalam fungsi tersebut. `MaxLines` membatasi jumlah baris yang ditampilkan, `EllipsizeMode` memotong sisanya di bagian awal, tengah, atau akhir, dan `WhiteSpaceNoWrap` menjaga paragraf tetap dalam satu baris. `AdjustsFontSizeToFit` mengecilkan teks hingga muat di kotaknya dan batas barisnya, sampai `MinimumFontScale` (minimal 0.1). Formula dan lampiran lain yang relatif terhadap font ikut mengecil dan kembali ke ukuran alaminya saat ruang memungkinkan; ukuran titik dan padding yang ditetapkan secara eksplisit mempertahankan nilai yang ditulis.

Field teks diwariskan seperti pada CSS: teks di dalam view yang memiliki `FontSize`, `Color`, atau `LineHeight` mengambil nilai tersebut kecuali teks itu menetapkan nilainya sendiri. `TextAlign`, `TextTransform`, `LetterSpacing`, `TextDecoration`, `TextDecorationStyle`, `TextDecorationColor` serta `TextShadowColor`, `TextShadowOffset`, dan `TextShadowRadius` menata gaya baris dan glyph.

## Run dan tautan

`Spans` membentuk teks dari beberapa run, masing-masing dengan gayanya sendiri. Span dengan `OnPress` adalah tautan, dan tekanan pada span lain menjadi `OnPress` milik teks itu sendiri.

```go
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "By continuing you agree to the "},
	{Content: "terms", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openTerms},
	{Content: " and the "},
	{Content: "privacy policy", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openPrivacy},
	{Content: "."},
}}
```

`SpansFunc` memberikan span lagi ketika apa yang dibacanya berubah, dan hanya bagian yang berbeda yang disiapkan ulang, sehingga baris editor kode berganti warna saat penyorotan sintaks tiba. `Background` dan `CornerRadius` sebuah span menggambar kotak di belakang karakternya di setiap baris, dan `Cursor`-nya menentukan tampilan penunjuk di atasnya, mendahului milik teks. `Visibility: core.VisibilityHidden` menyembunyikan span di tempatnya: kata-katanya tetap mempertahankan ruangnya, sehingga baris tidak bergeser saat span muncul, dan salinan serta pembaca layar mengabaikannya, untuk kata-kata yang muncul perlahan satu per satu.

Atur `BackgroundInsetTop: core.Pct(62)` pada span yang memiliki `Background` untuk menyorot 38% bagian bawah kotaknya di setiap baris. `BackgroundInsetBottom` memangkas tepi bawah. Kedua inset menerima titik atau persentase tinggi kotak, dari ascent font hingga descent-nya. Keduanya tidak mengubah tata letak teks. Di TSX, atur `backgroundInsetTop: "62%"` dan `backgroundInsetBottom` pada gaya `<Text>` bersarang. Latar belakang membulat dan inset vertikal juga dirender di HarmonyOS dan keempat target mini-program.

`Attachment` menempatkan sebuah view di dalam teks, menggantikan konten span. View tersebut ditata dengan ukurannya sendiri, membungkus bersama kata-kata di sekitarnya, dan menerima tekanannya sendiri, sehingga ikon, gambar, atau tombol kecil dapat berada di dalam sebuah baris. Seleksi menyalin `Content` span sebagai gantinya, dan mengabaikannya jika kosong. Secara default, baris teks pertamanya disejajarkan dengan baseline di sekitarnya, menggunakan metrik font yang sebenarnya. Canvas `Em` menyediakan baseline-nya sendiri, dan view tanpa baseline berada di tepi bawahnya. Atur `Style.VerticalAlign` untuk memilih penyejajaran lain.

```go
core.Text{Style: core.Style{FontSize: 17, LineHeight: 24}, Spans: []core.Span{
	{Content: "Search with "},
	{Attachment: core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 17}}},
	{Content: " NSGo"},
}}
```

## Font

`FontFamily` menyebutkan sebuah family atau daftar CSS (`Inter, "Noto Sans SC", sans-serif`), masing-masing digunakan untuk karakter yang tidak dimiliki family sebelumnya. Setiap family dalam daftar mengambil weight dan style. Di macOS dan iOS, sebuah baris mengambil tinggi dan baseline-nya dari face yang dipakai karakternya, sehingga baris yang seluruhnya memakai family yang lebih belakang mengambil milik family tersebut, sedangkan browser mempertahankan milik family pertama sebagai batas bawah. `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LineHeightMultiple`, `LetterSpacing`, dan `LetterSpacingEm` mengatur face dan spasi, dan `core.FontMonospace` adalah family lebar tetap milik platform.

`Language` adalah tag BCP 47 teks, diwariskan seperti field di atas pada teks, span, dan input. Karakter Han digambar dalam bentuk bahasa tersebut, sehingga `直` yang diatur dalam `ja` mengambil bentuk Jepangnya meskipun sistem lebih memilih bahasa Tionghoa, dan bahasa Korea (`ko`) memecah baris di antara kata seperti `word-break: keep-all` milik CSS. Teks dan input yang tidak menetapkannya memakai bahasa yang ditampilkan aplikasi dan mengikutinya saat berubah. Atur pada view untuk sebuah subpohon, atau pada `Span` untuk satu run:

```go
core.Text{Spans: []core.Span{
	{Content: "日本語: "},
	{Content: "直 骨 角", Style: core.Style{Language: "ja"}},
}}
```

Kemas font aplikasi di bawah `resources/` dan daftarkan sebelum view pertama, atau daftarkan byte sebuah font saat aplikasi berjalan:

```go
func main() { nsgo.Run(Root, nsgo.Fonts("fonts/Inter-Regular.ttf", "fonts/Inter-Bold.ttf")) }

cx.RegisterFont(data) // a TrueType or OpenType face
```

Teks yang ditata sebelum sebuah font tiba akan disiapkan ulang, dan canvas menjalankan `Draw` lagi sehingga glyph dan pengukuran teksnya menggunakan font yang telah dimuat. `cx.Env().FontScale()` adalah ukuran teks pengguna, yang diikuti teks kecuali `NoFontScaling` diatur; `MaxFontSizeMultiplier` membatasinya.

## Seleksi dan pengukuran

`SelectableOn` memungkinkan pengguna memilih dan menyalin teks, dan `View` dengan `Selectable` membuat teks-teksnya terseleksi bersama, sesuai urutan dokumen, seperti pada halaman web. `SelectionColor` mewarnai sorotan. `OnTextSelection` menerima `core.TextSelection` saat pengguna menggeser seleksi: `Text` polos, `Markdown`, dan `HTML`-nya (mempertahankan struktur inline dan blok hasil parsing dari setiap `TextView` dan teks bergaya di area tersebut, termasuk span parsial dan tautan), `Ranges` per teks, dan `Bounds` dari teks yang disentuhnya, di dalam view, untuk menyalin sebagai Markdown atau HTML, daftar kata, atau terjemahan kata-kata yang dipilih; `desktop.LookUp` menampilkannya di kamus sistem ([layanan](services.md#membuka-tautan-dan-file)). `cx.MeasureText` mengukur string dalam sebuah font, untuk kolom yang disesuaikan dengan teksnya; fungsi reaktif yang memanggilnya akan diperbarui saat font yang tersedia berubah, dan `OnTextLayout` melaporkan baris-baris setelah setiap penataan.

## Dokumen

`base/textview` menampilkan Markdown atau HTML dengan primitif yang sama: judul, paragraf, daftar, tabel, kode, catatan kaki, peringatan, gambar, formula, dan tautan.

```go
textview.TextView{
	Markdown: "## NSGo\n\nNative views from **Go**.",
	Styles:   ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) {
		if strings.HasPrefix(e.URL, "#") {
			return
		}
		cx.OpenURL(e.URL, nil)
	},
}
```

Formula (`$…$`, `$$…$$`) menampilkan TeX-nya hingga aplikasi memberikan `RenderMath: formula.RenderMath` dari `base/formula`, yang menatanya dengan ukuran dan warna teks di sekitarnya, dibacakan dalam bahasa aplikasi, dan disalin sebagai TeX; ini menambah sekitar 6.5 MB pada aplikasi desktop dan 3 MB (gzip) pada aplikasi web, termasuk fontnya. `formula.View` menampilkan formula di tempat lain, dalam TeX atau Typst; callback `OnHit`-nya melaporkan bagian sumber Untie, posisi caret terdekat, dan diagnostik yang tumpang tindih untuk editor formula. Di watchOS, formula menggunakan jalur canvas yang sama; menautkannya menambah sekitar 6.1 MB executable, termasuk font 1.4 MB. Satu kali run di simulator jam mengukur sekitar 3.9 MB heap Go yang tertahan untuk memuat dan mem-parsing font tersebut, termasuk salinan byte miliknya. Pengukuran di simulator tidak menetapkan batas memori jam fisik maupun ruang yang tersedia; ukur keduanya pada jam yang menjalankan aplikasi. Run teks (`\text{}` atau string Typst) yang tidak dapat dicakup oleh font yang dikonfigurasi menggunakan font sistem, termasuk CJK dan aksara lain; `formula.Fonts.Fallbacks` yang eksplisit didahulukan.

Untuk sumber yang tiba sepotong-sepotong, berikan `SourceFunc` ke `formula.View` dan atur `Streaming: true`, atau ikat `StreamingFunc` ke state penerimaan aplikasi. Ini menyembunyikan tinta galat yang dapat dipulihkan dan tinta placeholder sementara konten yang valid tetap ditata. Hapus flag tersebut saat penerimaan selesai untuk menampilkan galat yang tersisa. Di `TextView`, sediakan fungsi `RenderMath` yang mengembalikan `formula.View` tersebut dengan `Math.TeX` dan `Math.Display` yang diberikan; `StreamingFunc`-nya dapat membaca state penerimaan yang sama.

Formula tampilan (*display*) membungkus pada relasi dan operator yang aman sesuai lebar blok di dalam padding dan border-nya, dengan mempertahankan ukuran font teks. Mengubah ukuran atau mengubah ukuran teks akan menata ulang baris. Bagian yang tidak dapat dipecah tetap menggulir ke samping; formula inline mempertahankan ukuran alaminya.

Blok kode `mermaid` menampilkan sumbernya hingga aplikasi memberikan `RenderDiagram: diagram.RenderDiagram` dari `base/diagram`, yang menggambarnya sebagai diagram native, dengan limu: flowchart, diagram sekuens, kelas, state, dan relasi entitas, diagram Gantt, pie, mind map, timeline, dan lainnya, dalam font platform, terang atau gelap sesuai tampilan aplikasi. Item diagram yang memiliki tautan membukanya melalui layanan URL aplikasi saat ditekan, dan tooltip Mermaid sebuah item muncul selama penunjuk berada di atasnya. Diagram ditata sesuai lebar teks yang tersedia, mengubah flowchart lebar menjadi mengalir ke bawah sambil mempertahankan ukuran labelnya; diagram yang tidak muat tetap menggulir ke samping. Salinan mengambil sumbernya, dan blok yang masih sedang di-streaming ditampilkan sebagai kode hingga selesai. `diagram.View` menampilkan diagram di tempat lain dan mengikuti lebar kontainernya saat berubah. `Icons`-nya memetakan nama Mermaid seperti `"fa:fa-car"` ke dokumen `*core.SVG`, termasuk dari `icons/lucide`; nama yang tidak ada tidak meninggalkan celah inline. Node gambar memuat sumber HTTP(S), file, atau `asset:` melalui pemuat gambar canvas bersama, menyesuaikan dan memusatkan ketika `constraint: "on"` diatur dan meregangkannya jika tidak. Mermaid menyaring alamat gambar sebelum memuat (termasuk menolak URL `data:`). Untuk menyediakan ikon di text view, atur `RenderDiagram` ke callback yang mengembalikan `diagram.View{Source: code.Code, Icons: icons}`.

Untuk memberi diagram palet `ui` Anda, kembalikan `diagram.View` dengan `ThemeFunc` dari callback `RenderDiagram` Anda. Fungsi tersebut membaca warna saat canvas menggambar, sehingga perubahan palet atau tampilan subpohon mewarnai ulang view yang ada. Warna yang dibiarkan nol mempertahankan default diagram.

```go
func drawDiagram(cx *core.Scope, code textview.CodeBlock) core.Node {
	colors := ui.ColorsOf(cx)
	return diagram.View{Source: code.Code, ThemeFunc: func() *diagram.Theme {
		return &diagram.Theme{
			Text: colors.Text(), Surface: colors.Surface(), Border: colors.Border(),
			Line: colors.Muted(), Accent: colors.Accent(), Critical: colors.Danger(),
		}
	}}
}
// ui.TextView{Markdown: source, RenderDiagram: drawDiagram}
```

`textview.Parse` membaca Markdown dan `textview.ParseHTML` membaca HTML menjadi blok yang juga dapat dibangun oleh aplikasi. `Options.Streaming` memperbaiki dokumen yang tiba sepotong-sepotong, menutup apa yang dibiarkan terbuka di akhirnya dan menahan baris terakhir yang maknanya ditentukan oleh karakter berikutnya, seperti penanda daftar yang berdiri sendiri, sehingga balasan tidak pernah memunculkan penanda sebagai teks sesaat, dan `StreamingCaret` menggambar caret di ujungnya, untuk balasan yang ditulis sambil di-streaming.

`textview.Search` menemukan teks di seluruh view yang bergabung dengannya, melalui field `Search` mereka atau `textview.ProvideSearch` di atasnya. `SetQuery`, `Open`, dan `Close` menjalankannya dari bilah pencarian, `Count` dan `Selected` menunjukkan posisinya, dan `Next` serta `Previous` berpindah antar kecocokan dalam urutan baca. View menandai setiap kecocokan dengan `PartSearch` dan yang terpilih dengan `PartSearchSelected`, agar gaya aplikasi dapat mewarnainya, menggulirnya ke dalam tampilan, dan membiarkan seleksi pengguna apa adanya.

```go
search := textview.NewSearch(cx)
textview.ProvideSearch(cx, search)
search.SetQuery("readme")
search.Open()
```

## Selanjutnya

- [Gambar dan ikon](images.md): gambar dan SVG, termasuk di dalam teks.
- [Gaya dan tema](styles.md): warna dan field teks yang diwariskan.
- [Tata letak](layout.md): bagaimana sebuah teks mengambil lebarnya.

## Penyuntingan teks kaya

`ui/richeditor.View` menampilkan `base/richeditor.Session` dengan tampilan tema; `base/richeditor.View` menyediakan perilakunya dengan gaya yang disediakan aplikasi. Di iOS dan Android, tekan lama memilih sebuah kata dan ketuk ganda melakukan hal yang sama; seret pegangan seleksi untuk memperluas rentang melintasi blok. Menu edit native menawarkan Copy dan Select All, serta Cut dan Paste selama sesi dapat disunting, sesuai dengan seleksi dan clipboard. `Session.SetEditable` juga menyegarkan menu yang sedang terbuka. Beri gaya pada kenop dengan `Styles.SelectionHandle` atau `Classes.SelectionHandle` pada base view.

Untuk kontrol teks kustom, fokuskan `core.Input` dan panggil `Ref.OpenEditMenu(bounds, actions)` dengan bounds dalam titik surface dan bitmask dari `core.EditCut`, `EditCopy`, `EditPaste`, dan `EditSelectAll`. Di iOS dan Android, `Input.OnEditAction` menerima Cut, Copy, dan Select All; Paste menggunakan `Input.OnPaste`. Panggil `Ref.CloseEditMenu` untuk menutupnya.

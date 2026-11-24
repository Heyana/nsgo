---
title: Kanvas dan shader
description: "Menggambar grafik 2D di mesin, dan mengisi tampilan dengan fragment shader GPU."
group: Konsep inti
order: 59
---

## Sebuah kanvas

`core.Canvas` merekam gambar 2D di mesin dan meminta platform memutarnya, seperti `<canvas>` di halaman web. `Draw` berjalan lagi ketika sinyal yang dibacanya, ukuran kanvas, atau font yang tersedia berubah, dan platform menggambar rekaman tersebut di utas utama. Font yang dimuat melalui `cx.RegisterFont` menjalankan ulang `Draw` setelah tersedia, memperbarui teks maupun geometri berdasarkan `TextMetrics`.

Gunakan `Em` atau `EmFunc` untuk kanvas yang ukurannya dinyatakan dalam satuan em dari font yang diwarisinya. Untuk konten yang tingginya bergantung pada lebar, `MeasureEm(width, fontSize)` mengembalikan `EmBox` pada lebar yang tersedia dalam poin (`core.Auto` jika tak terbatas) dan ukuran font yang telah diselesaikan. Pembacaannya dilacak, dan fungsi ini juga berjalan selama tata letak, jadi pastikan bebas dari penulisan sinyal. `Ref.ContentBox()` melacak kotak tata letak tampilan yang terpasang di dalam padding dan batas (*borders*), dalam koordinatnya sendiri.

```go
core.Canvas{
	Style: core.Style{Height: core.Pt(220)},
	Draw: func(d *core.Drawing, size core.Size) {
		d.SetFillColor(core.RGB(0xf1f5f9)).FillRect(0, 0, float32(size.Width), float32(size.Height))
		d.SetStrokeColor(core.RGB(0x007aff)).SetLineWidth(2).SetLineCap(core.CapRound)
		d.BeginPath().MoveTo(24, 160).LineTo(90, 80).LineTo(160, 120).LineTo(230, 40).Stroke()
	},
}
```

Rekaman ini mengikuti `CanvasRenderingContext2D` di web: `Save` dan `Restore`, transformasi, jalur garis, kurva, persegi panjang, dan busur, pengisian (*fills*) serta goresan (*strokes*) dengan warna dan gradien, pemotongan (*clipping*), garis putus-putus, bayangan, teks, dan gambar. `core.ParsePath` membuat `core.Path` dari data jalur SVG, dan `FillFromView` serta `StrokeFromView` mengambil gaya `Fill` dan `Stroke` yang diwarisi tampilan kanvas, sehingga bagan mengikuti tema aplikasi.

Di web, kanvas di dalam `core.Scroll` yang diperbesar atau `Style.Transform` / `AnimatedStyle` yang diskalakan diputar ulang pada skala kumulatifnya setelah gerakan berhenti, hingga empat kali rasio piksel layar. Kanvas yang bitmap hasil pembesarannya akan melebihi 4096 × 4096 piksel hanya menggambar bagian yang terlihat dan memperbaruinya saat digulir atau digeser (*pan*), menjaga garis dan glif tetap tajam tanpa menjalankan ulang `Draw`. Apple, Android, Windows, dan Linux juga menjaga kanvas dan teks yang diperbesar tetap tajam baik di bawah pembesaran gulir maupun transformasi gaya.

```go
d.SetFillGradient(core.CanvasLinear(0, 0, float32(size.Width), 0,
	core.GradientStop{Offset: 0, Color: core.RGB(0x4da3ff)},
	core.GradientStop{Offset: 1, Color: core.RGB(0x007aff)},
)).FillRect(0, 0, float32(size.Width), 40)
```

## Teks, gambar, dan bagian

`MeasureText`, `TextMetrics`, dan `SetTextAlign` menata teks pada mesin teks platform, sehingga label menggunakan font yang sama dengan bagian aplikasi lainnya. `DrawImage`, `DrawImageIn`, dan `DrawImageFrom` menggambar gambar; `DrawSVG` dan `DrawSVGIn` menggambar dokumen `*core.SVG` yang telah diurai (`core.ParseSVG`) pada ukuran intrinsiknya atau disesuaikan ke dalam persegi panjang — dengan elemen `<text>` dan `<tspan>` diukur serta dirender oleh mesin teks milik platform sendiri dan `currentColor` diambil dari warna isi gambar — serta `OnPress` dapat menguji gambar terakhir dengan `IsPointInPath` dan `IsPointInStroke`. Backend native men-cache 64 sumber gambar kanvas terakhir yang digambar tanpa dipertahankan; panggil `release := cx.KeepImage(src)` untuk menyematkan atlas sprite atau tekstur berumur panjang agar terhindar dari batas pengusiran tersebut sampai `release()` dijalankan atau `cx` dibuang.

```go
badge := core.MustParseSVG([]byte(`<svg viewBox="0 0 120 40">
	<rect width="120" height="40" rx="8" fill="#007aff"/>
	<text x="60" y="25" text-anchor="middle" font-size="14" fill="#fff">Live</text>
</svg>`))

d.DrawSVGIn(badge, 12, 12, 180, 60)
```

Bitmap yang diskalakan menggunakan pengambilan sampel halus secara default. Untuk seni piksel (*pixel art*), panggil `d.SetImageSmoothing(false)` sebelum `DrawImage`, `DrawImageFrom`, atau `DrawAtlas`, atau atur `ctx.imageSmoothingEnabled = false` di TSX. `Save` dan `Restore` menyimpan pengaturan tersebut bersama status menggambar lainnya. Pengambilan sampel nearest-neighbor tersedia di platform Apple, Windows, Linux, Android, dan web; HarmonyOS dan program mini juga menerapkannya, meskipun hasilnya belum diperiksa di perangkat.

`d.DrawVertices(image, vertices)` menggambar jaring bertekstur (*textured mesh*). Setiap tiga nilai `core.CanvasVertex` berturut-turut membentuk satu segitiga. Atur `Position` dalam poin kanvas dan `TexCoord` dalam poin gambar, seperti di `DrawImageFrom`. `Color` pada setiap verteks mengalikan tekstur dan menginterpolasi di seluruh segitiga. Nilai nolnya membiarkan tekstur tidak berubah. Perbarui titik-titiknya lalu rekam gambarnya lagi untuk mendeformasi jaring. Gambarnya dapat berupa `ImageSource` apa pun, termasuk buffer piksel atau `Source()` milik `OffscreenImage`; kanvas Android dan program mini belum menggambar gambar luar layar (*offscreen images*). `SetImageSmoothing(false)` juga berlaku untuk jaring. Web memerlukan gambar inline atau same-origin untuk jaring arbitrer. Program mini menggambar jaring menggunakan pemutar web, yang belum diperiksa di WeChat DevTools maupun di perangkat. HarmonyOS memotong setiap segitiga dan menggambar gambarnya melalui transformasi affine-nya; seperti halnya `DrawImage`, HarmonyOS menggambar gambar terkodekan dan buffer piksel tetapi bukan gambar luar layar, dan memutar alfa verteks rata-rata setiap segitiga tanpa pewarnaan (*color tint*). Cara itu hanya menyembunyikan sambungan antar-segitiga yang buram, dan belum diperiksa di perangkat.

`DrawTintedImage`, `DrawTintedImageIn`, dan `DrawTintedImageFrom` mengalikan saluran sRGB dan alfa gambar dengan suatu warna sebelum pengomposisian. Dengan mode *source-over* bawaan, piksel transparan membiarkan latar belakang tetap terlihat. Gunakan `drawTintedImage(image, tint, ...args)` di TSX dengan bentuk koordinat yang sama seperti `drawImage`. Sumber lintas-origin (*cross-origin*) di web harus mengizinkan permintaan CORS anonim agar pikselnya dapat diwarnai. HarmonyOS mewarnai gambar dari byte, sumber daya aplikasi, dan buffer piksel, meskipun belum diperiksa di perangkat; gambar luar layar belum digambar di sana.

Gunakan `d.DrawAtlas(source, rects, xforms, alphas)` untuk menggambar banyak sprite dari satu gambar secara berurutan. Setiap `core.Bounds` memilih satu ubin dalam poin gambar itu sendiri. `core.RSXform` yang sesuai memetakan sudut kiri atas ubin ke `TX, TY`, dengan `SCos` dan `SSin` diatur ke skala dikalikan kosinus dan sinus dari rotasi. Sebagai contoh, `core.RSXform{SCos: 0.5, TX: 40, TY: 20}` menggambar ubin 32-poin dengan ukuran setengah di (40, 20). Berikan `nil` untuk opasitas sprite penuh, atau berikan faktor pengali untuk alfa gambar saat ini.

`Drawing.Node` menamai bagian dari suatu gambar, seperti batang pada bagan atau wilayah pada peta, dengan batas (*bounds*), peran, label, dan status terpilih. Teknologi asistif membacakan bagian-bagian tersebut sebagai anak dari kanvas, dan pengujian menemukannya berdasarkan label, teks, atau ID pengujian; penekanan pada salah satunya adalah penekanan di titik tengahnya.

```go
d.Node(core.CanvasNode{
	Bounds: core.Bounds{Origin: core.Point{X: x, Y: y}, Size: core.Size{Width: w, Height: h}},
	Role:   core.RoleButton,
	Label:  "Open the report",
	TestID: "report",
})
```

## Gambar luar layar

`cx.NewOffscreenImage(size, scale, draw)` merender `core.Drawing` satu kali ke dalam gambar luar layar (*offscreen image*) yang dipertahankan, yang digambar oleh frame-frame berikutnya dengan `DrawImage`, `DrawImageIn`, `DrawImageFrom`, `DrawAtlas`, atau `core.Image`. Memberikan `scale <= 0` menggunakan rasio piksel tampilan saat ini. Gambar hanya dirender ulang saat `Invalidate()` atau `Resize(size, scale)` dipanggil (atau saat font baru didaftarkan), dan dilepaskan secara otomatis ketika `cx` dibuang atau `Release()` dipanggil.

```go
tiles := cx.NewOffscreenImage(core.Size{Width: 512, Height: 512}, 0, func(d *core.Drawing) {
	// Drawn once into the offscreen image until tiles.Invalidate() is called.
	for y := range 16 {
		for x := range 16 {
			d.SetFillColor(tileColor(x, y)).FillRect(float32(x*32), float32(y*32), 32, 32)
		}
	}
})

core.Canvas{
	Style: core.Style{Width: core.Pt(320), Height: core.Pt(240)},
	Draw: func(d *core.Drawing, _ core.Size) {
		// Reuses the cached surface in a single DrawImage operation per frame.
		d.DrawImage(tiles.Source(), -cameraX.Get(), -cameraY.Get())
	},
}
```

## Argumen teranimasi

`Drawing.Bind` mengikat argumen dari operasi terakhir ke sebuah node teranimasi, dan platform menggambar ulang hanya bagian tersebut pada jam frame-nya, tanpa komit. `core.ArgX`, `ArgY`, `ArgR`, `ArgA`, dan `ArgColor` menamai argumen-argumennya; macOS, iOS, dan Linux hanya menggambar ulang area yang berubah, sedangkan Windows merekamnya ulang dan Android serta web menggambar ulang seluruh kanvas.

```go
needle := core.NewAnimatedValue(cx, 0)
d.Save().Translate(cx, cy).Rotate(0).Bind(core.ArgA, needle).
	BeginPath().MoveTo(-14, -3).LineTo(80, 0).LineTo(-14, 3).ClosePath().Fill().Restore()
```

## Shader

`core.Shader` mengisi kotaknya dengan fragment shader yang dijalankan GPU untuk setiap piksel. `core.CompileShader` menerima program yang ditulis dalam sintaksis Go dengan fungsi `Fragment`, dan `core.CompileWGSL` menerima WGSL. `Uniforms` mengatur variabel program yang diekspor, dan `Images` memberinya hingga empat tekstur.

```go
var program = core.MustCompileShader(`
package main

var Tint vec4

func Fragment(dst vec4, pos vec2) vec4 {
	v := 0.5 + 0.5*sin(pos.x/40)
	return Tint * vec4(v, v, v, 1)
}
`)

core.Shader{
	Program:  program,
	Uniforms: core.Uniforms{"Tint": core.RGB(0x007aff)},
	Style:    core.Style{Height: core.Pt(220), CornerRadius: 16},
}
```

Shader Go menerima pusat piksel di `dst` dan titik yang sama dalam poin di `pos`, serta memiliki fungsi bawaan GLSL; `rectSize`, `pixelScale`, `image0At` hingga `image3At`, dan ukurannya adalah tambahan milik NSGo. Program WGSL menulis `fn shade(uv: vec2f) -> vec4f` dan membaca `nsh.time` serta `nsh.size`; `Paused` menghentikan jamnya. Platform tanpa jalur GPU untuk shader tidak menggambar apa pun di sana.

`core.ShaderEffect` menggambar anak-anaknya melalui WGSL dan `core.ScreenEffect` menutupi seluruh permukaan. `core.ShaderTransition` dan `core.ScreenTransition` mengambil sampel gambar sebelum dan sesudah `Value` berubah. `core.TransitionShader` melakukannya untuk navigasi dan dialog. Di Android, menangkap tampilan untuk efek memerlukan Android 10 atau lebih baru dan OpenGL ES 3.0 dengan dukungan tekstur eksternal. `core.Canvas` dan `core.Shader` sama-sama menerima `Label` dan anak-anaknya digambar di atas keduanya.

## Berikutnya

- [Gambar dan ikon](images.md): gambar yang digambar oleh kanvas.
- [Animasi](animation.md): nilai yang diikat oleh kanvas.
- [Aksesibilitas](accessibility.md): bagian yang diekspos oleh kanvas.

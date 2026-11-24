---
title: Tuval ve gölgelendiriciler
description: Motor üzerinde 2B grafikler çizin ve bir görünümü GPU parça gölgelendiricisiyle doldurun.
group: Temel kavramlar
order: 59
---

## Bir tuval (Canvas)

`core.Canvas`, tıpkı bir web sayfasının `<canvas>` öğesi gibi 2B çizimi motor üzerinde kaydeder ve platformun bunu oynatmasını sağlar. `Draw`, okuduğu bir sinyal, tuvalin boyutu veya kullanılabilir yazı tipleri değiştiğinde yeniden çalışır ve platform kaydı ana iş parçacığında çizer. `cx.RegisterFont` ile yüklenen bir yazı tipi, kullanılabilir hale geldiğinde `Draw`'ı yeniden çalıştırarak hem metni hem de `TextMetrics`'e dayalı geometriyi günceller.

Kalıtılan yazı tipinin `em` birimleriyle boyutlandırılmış bir tuval için `Em` veya `EmFunc` kullanın. Yüksekliği genişliğe bağlı olan içerikler için `MeasureEm(width, fontSize)`, punto cinsinden kullanılabilir genişlikte (sınırsız olduğunda `core.Auto`) ve çözümlenmiş yazı tipi boyutunda bir `EmBox` döndürür. Okumaları izlenir ve yerleşim sırasında da çalışır; bu nedenle sinyal yazmalarından uzak tutun. `Ref.ContentBox()` bağlı bir görünümün dolgu ve kenarlıklar içindeki yerleşim kutusunu kendi koordinatlarında izler.

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

Kayıt web'in `CanvasRenderingContext2D` arabirimini izler: `Save` ve `Restore`, dönüşümler, çizgilerden, eğrilerden, dikdörtgenlerden ve yaylardan oluşan yollar, renkler ve gradyanlarla dolgular ve konturlar, kırpma, kesik çizgiler, gölgeler, metin ve görüntüler. `core.ParsePath` SVG yol verilerinden bir `core.Path` oluşturur; `FillFromView` ve `StrokeFromView` ise tuval görünümünün kalıtılan `Fill` ve `Stroke` stillerini alır, böylece bir grafik uygulamanın temasını izler.

Web'de yakınlaştırılmış bir `core.Scroll` veya ölçeklendirilmiş bir `Style.Transform` / `AnimatedStyle` içindeki tuvaller, hareket durduğunda ekranın piksel oranının dört katına kadar kümülatif ölçekte yeniden oynatılır. Yakınlaştırılmış bit eşlemi 4096 × 4096 pikseli aşacak bir tuval yalnızca görünür kısmını çizer ve kaydırıldıkça veya gezdirildikçe güncellenir; böylece `Draw`'ı yeniden çalıştırmadan çizgileri ve glifleri keskin tutar. Apple, Android, Windows ve Linux da hem kaydırma yakınlaştırması hem de stil dönüşümleri altında yakınlaştırılmış tuvaller ile metinleri keskin tutar.

```go
d.SetFillGradient(core.CanvasLinear(0, 0, float32(size.Width), 0,
	core.GradientStop{Offset: 0, Color: core.RGB(0x4da3ff)},
	core.GradientStop{Offset: 1, Color: core.RGB(0x007aff)},
)).FillRect(0, 0, float32(size.Width), 40)
```

## Metin, görüntüler ve parçalar

`MeasureText`, `TextMetrics` ve `SetTextAlign` metni platformun metin motorunda düzenler; böylece etiketler uygulamanın geri kalanıyla aynı yazı tiplerini kullanır. `DrawImage`, `DrawImageIn` ve `DrawImageFrom` resimler çizer; `DrawSVG` ve `DrawSVGIn` ayrıştırılmış `*core.SVG` belgelerini (`core.ParseSVG`) kendi doğal boyutlarında veya bir dikdörtgene sığdırarak çizer — `<text>` ve `<tspan>` öğeleri platformun kendi metin motoru tarafından ölçülüp işlenir ve `currentColor` çizimin dolgu renginden alınır — ve `OnPress`, son çizimi `IsPointInPath` ve `IsPointInStroke` ile sınayabilir. Yerel arka uçlar en son çizilen ve tutulmayan 64 tuval görüntü kaynağını önbelleğe alır; `release()` çalışana veya `cx` sonlandırılana kadar hareketli grafik atlaslarını veya uzun ömürlü dokuları bu çıkarma sınırının dışında sabitlemek için `release := cx.KeepImage(src)` çağırın.

```go
badge := core.MustParseSVG([]byte(`<svg viewBox="0 0 120 40">
	<rect width="120" height="40" rx="8" fill="#007aff"/>
	<text x="60" y="25" text-anchor="middle" font-size="14" fill="#fff">Live</text>
</svg>`))

d.DrawSVGIn(badge, 12, 12, 180, 60)
```

Ölçeklendirilmiş bit eşlemler varsayılan olarak yumuşak örneklemeyi kullanır. Piksel sanatı için `DrawImage`, `DrawImageFrom` veya `DrawAtlas` çağrısından önce `d.SetImageSmoothing(false)` çağırın ya da TSX'te `ctx.imageSmoothingEnabled = false` ayarlayın. `Save` ve `Restore` bu ayarı çizim durumunun geri kalanıyla birlikte saklar. En yakın komşu (nearest-neighbor) örneklemesi Apple platformlarında, Windows, Linux, Android ve web'de kullanılabilir; HarmonyOS ve mini programlar da bunu uygular ancak çıktıları henüz cihazlarda doğrulanmamıştır.

`d.DrawVertices(image, vertices)` dokulu bir ağ (mesh) çizer. Ardışık her üç `core.CanvasVertex` değeri bir üçgen oluşturur. Tıpkı `DrawImageFrom`'da olduğu gibi `Position` değerini tuval puntosu cinsinden ve `TexCoord` değerini görüntü puntosu cinsinden ayarlayın. Her köşenin `Color` değeri dokuyu çarpar ve üçgen üzerinde enterpolasyon yapar. Sıfır değeri dokuyu değiştirmeden bırakır. Ağı deforme etmek için noktaları güncelleyin ve çizimi yeniden kaydedin. Görüntü, piksel arabelleği veya bir `OffscreenImage`'ın `Source()` sonucu dahil herhangi bir `ImageSource` olabilir; Android tuvalleri ve mini programlar henüz ekran dışı görüntüler çizmez. `SetImageSmoothing(false)` ağlar için de geçerlidir. Web, keyfi ağlar için satır içi (inline) veya aynı kökenli görüntüler gerektirir. Mini programlar ağları web'in oynatıcısıyla çizer; bu henüz WeChat DevTools'da veya bir cihazda doğrulanmamıştır. HarmonyOS her üçgeni kırpar ve görüntüyü kendi afin dönüşümü üzerinden çizer; `DrawImage`'da olduğu gibi kodlanmış görüntüleri ve piksel arabelleklerini çizer ancak ekran dışı görüntüleri çizmez ve her üçgenin ortalama köşe alfasını oynatır fakat renk tonlaması yapmaz. Yalnızca opak üçgenler arasındaki dikiş yerlerini gizler ve henüz bir cihazda doğrulanmamıştır.

`DrawTintedImage`, `DrawTintedImageIn` ve `DrawTintedImageFrom` birleştirme öncesinde bir görüntünün sRGB ve alfa kanallarını bir renkle çarpar. Varsayılan source-over kipinde saydam pikseller arka planı görünür bırakır. TSX'te `drawImage` ile aynı koordinat biçimleriyle `drawTintedImage(image, tint, ...args)` kullanın. Web'de çapraz kökenli bir kaynağın piksellerinin tonlanabilmesi için anonim CORS isteklerine izin vermesi gerekir. HarmonyOS baytlardan, uygulamanın kaynaklarından ve piksel arabelleklerinden gelen görüntüleri tonlar, ancak henüz bir cihazda doğrulanmamıştır; ekran dışı görüntüler orada henüz çizilmez.

Bir görüntüden birçok hareketli grafiği (sprite) sırayla çizmek için `d.DrawAtlas(source, rects, xforms, alphas)` kullanın. Her `core.Bounds` görüntünün kendi puntoları içinde bir döşeme seçer. Eşleşen `core.RSXform` döşemenin sol üst köşesini `TX, TY` konumuna eşler; `SCos` ve `SSin` ise döndürmenin kosinüsü ve sinüsü çarpı ölçek olarak ayarlanır. Örneğin `core.RSXform{SCos: 0.5, TX: 40, TY: 20}` 32 puntoluk bir döşemeyi (40, 20) konumunda yarı boyutta çizer. Tam hareketli grafik opaklığı için `nil` geçirin veya geçerli çizim alfasını çarpan katsayılar sağlayın.

`Drawing.Node` sınırları, rolü, etiketi ve seçili durumuyla bir grafiğin çubuğu veya bir haritanın bölgesi gibi bir çizim parçasını adlandırır. Yardımcı teknolojiler bu parçaları tuvalin alt öğeleri olarak okur ve testler bunları etiket, metin veya test kimliğine göre bulur; bunlardan birine basmak merkezine basmak anlamına gelir.

```go
d.Node(core.CanvasNode{
	Bounds: core.Bounds{Origin: core.Point{X: x, Y: y}, Size: core.Size{Width: w, Height: h}},
	Role:   core.RoleButton,
	Label:  "Open the report",
	TestID: "report",
})
```

## Ekran dışı görüntüler

`cx.NewOffscreenImage(size, scale, draw)` bir `core.Drawing` çizimini sonraki karelerin `DrawImage`, `DrawImageIn`, `DrawImageFrom`, `DrawAtlas` veya `core.Image` ile çizdiği kalıcı bir ekran dışı görüntüye bir kez işler. `scale <= 0` geçirmek geçerli ekran piksel oranını kullanır. Çizim yalnızca `Invalidate()` veya `Resize(size, scale)` çağrıldığında (ya da yeni yazı tipleri kaydedildiğinde) yeniden işlenir ve `cx` sonlandırıldığında veya `Release()` çağrıldığında otomatik olarak serbest bırakılır.

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

## Animasyonlu bağımsız değişkenler

`Drawing.Bind` son işlemin bir bağımsız değişkenini animasyonlu bir düğüme bağlar ve platform bir işleme (commit) olmadan yalnızca o parçayı kendi kare saatinde yeniden çizer. `core.ArgX`, `ArgY`, `ArgR`, `ArgA` ve `ArgColor` bağımsız değişkenleri adlandırır; macOS, iOS ve Linux yalnızca değişen alanı yeniden çizerken Windows bunu yeniden kaydeder, Android ve web ise tüm tuvali yeniden çizer.

```go
needle := core.NewAnimatedValue(cx, 0)
d.Save().Translate(cx, cy).Rotate(0).Bind(core.ArgA, needle).
	BeginPath().MoveTo(-14, -3).LineTo(80, 0).LineTo(-14, 3).ClosePath().Fill().Restore()
```

## Gölgelendiriciler (Shaders)

`core.Shader` kutusunu GPU'nun her piksel için çalıştırdığı bir parça gölgelendiricisiyle (fragment shader) doldurur. `core.CompileShader` bir `Fragment` işleviyle Go sözdiziminde yazılmış bir program alır, `core.CompileWGSL` ise WGSL alır. `Uniforms` programın dışa aktarılan değişkenlerini ayarlar ve `Images` ona en fazla dört doku verir.

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

Bir Go gölgelendiricisi pikselin merkezini `dst` içinde ve aynı noktayı punto cinsinden `pos` içinde alır ve GLSL'in yerleşik işlevlerine sahiptir; `rectSize`, `pixelScale`, `image0At` ile `image3At` arası ve bunların boyutları NSGo'nun eklemeleridir. Bir WGSL programı `fn shade(uv: vec2f) -> vec4f` yazar ve `nsh.time` ile `nsh.size` değerlerini okur; `Paused` saatini durdurur. Gölgelendiriciler için GPU yolu olmayan bir platform orada hiçbir şey çizmez.

`core.ShaderEffect` alt öğelerini WGSL üzerinden çizer ve `core.ScreenEffect` yüzeyi kaplar. `core.ShaderTransition` ve `core.ScreenTransition` `Value` değişmeden önceki ve sonraki resimleri örnekler. `core.TransitionShader` bunu gezinme ve iletişim kutuları için yapar. Android'de efektler için görünümleri yakalamak Android 10 veya üzerini ve harici doku desteğine sahip OpenGL ES 3.0 gerektirir. `core.Canvas` ve `core.Shader` her ikisi de `Label` alır ve alt öğeleri bunların üzerine çizilir.

## Sonraki adımlar

- [Görüntüler ve simgeler](images.md): bir tuvalin çizdiği resimler.
- [Animasyon](animation.md): bir tuvalin bağladığı değerler.
- [Erişilebilirlik](accessibility.md): bir tuvalin sunduğu parçalar.

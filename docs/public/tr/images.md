---
title: Görüntüler ve simgeler
description: Bir URL'den, uygulamanın kaynaklarından, baytlardan veya uygulamanın çizdiği piksellerden bir resim gösterin; Lucide simgeleri ve SVG'ler çizin.
group: Temel kavramlar
order: 54
---

## Bir resim

`core.Image` bir bitmap veya SVG gösterir: bir URL, bir dosya, uygulamanın kaynaklarındaki bir `asset:` yolu veya uygulamanın tuttuğu baytlar. Platform onu ana iş parçacığının dışında yükler, çözer ve önbelleğe alır; ana iş parçacığı ise sonucu çizer.

```go
core.Image{
	Source: core.Src("https://example.com/cover.jpg"),
	ResizeMode: core.Cover,
	Style: core.Style{Width: core.Pt(160), Height: core.Pt(100), CornerRadius: 12},
}
```

`core.Cover` kutuyu doldurur ve taşan kısmı kırpar; `core.Contain` resmin tamamını sığdırır, `core.Center` doğal boyutunu korur, `core.ScaleDown` ise kutudan büyük olmadıkça boyutunu korur. `BlurRadius` resmi nokta cinsinden bulanıklaştırır, `TintColor` opak piksellerini yeniden renklendirir ve `ObjectPosition` onu kutu içinde konumlandırır.

Ölçeklenen bitmap'ler varsayılan olarak yumuşak örnekleme kullanır. Piksel sanatının ve piksel arabelleklerinin keskin kenarlarını korumak için Go'da `Sampling: core.ImageSamplingNearest` veya TSX'te `sampling="nearest"` ayarlayın. Apple platformları, Windows, Linux, Android ve web bu ayarı destekler; HarmonyOS ve mini programlar da uygular, ancak çıktıları henüz cihazlarda doğrulanmamıştır. CSS `pixelated` kullanan bir tarayıcıda tam piksel kenarları için, kaynak pikseli başına tam sayıda cihaz pikseline ölçekleyin.

`Source`, yerleşimin kullandığı boyutu bildirir: bir boyut diğerini türetir ve boyutu olmayan bir görüntü, stili onu boyutlandırana kadar 0'a 0'dır. `core.ImageSource{Data: data, Size: core.Size{Width: 24, Height: 24}}`, `go:embed` ile gömülmüş baytları gösterir ve `Size` bir SVG'ye içsel boyutunu verir.

[`desktop.WindowPointerPainted`](services.md) ile `HitTestAlpha: 0.5`, macOS ve Windows'ta yarıdan az opak olan piksellerde işaretçinin geçmesine izin verir. TSX aynı eşiği `hitTestAlpha` ile ayarlar.

## Uygulamanın çizdiği kareler

`Pixels`, uygulamanın çizdiği bir `*image.RGBA`'yı, yani bir video oynatıcının veya bir emülatörün karelerini, hiçbir şey kodlanmadan, yüklenmeden veya çözülmeden gösterir. Her kareyi aynı arabelleğe yazın ve `Version` değerini artırın: görünüm onu yükleme olayları ve solma olmadan, alfa kanalı korunarak gösterir ve macOS, iOS, tvOS, watchOS, Android, Windows ve Linux'ta arada boş bir kare olmadan yerinde değiştirir. `Size` olmadan piksellerini nokta cinsinden ölçer.

```go
frame := image.NewRGBA(image.Rect(0, 0, 960, 540))
version := core.NewSignal(cx, uint64(0))
var tick func(time.Time)
tick = func(at time.Time) {
	if decoder.WriteDue(at, frame) { // writes the next frame into frame.Pix
		version.Set(version.Get() + 1)
	}
	cx.RequestAnimationFrame(tick)
}
cx.RequestAnimationFrame(tick)

core.Image{ResizeMode: core.Contain, SourceFunc: func() core.ImageSource {
	return core.ImageSource{Pixels: frame, Version: version.Get()}
}}
```

NSGo, yeni bir `Version` taşıyan işleme çalıştığında pikselleri kopyalar; böylece uygulama bir sonraki kareyi bir sonraki `RequestAnimationFrame` geri çağrısından yazabilir; kendi goroutine'inde çalışan bir kod çözücü ikinci bir arabelleğe yazar ve değiş tokuşu gönderir. Aynı `Version` yeniden çizilmez. `cx.ImageSize`, bir piksel kaynağını arabelleğinin boyutuyla hemen yanıtlar. Bir `core.Canvas`, pikselleri her platformda aynı şekilde kopyalanan `DrawImage` ile çizer; mini programlar bir WeChat 2D tuvali gerektirir. Bir `core.Shader`'ın `Images` alanı da pikselleri aynı şekilde alır; böylece bir gölgelendirici, gölgelendirici çizen her platformda, bir emülatörün ekranı gibi uygulamanın yazdığı bir kareyi filtreleyebilir.

Apple platformları, Android, Windows, Linux ve web piksel görüntülerini doğrudan günceller; WeChat mini programları ise bir 2D tuval kullanır. HarmonyOS her sürümden bir `PixelMap` oluşturur (bir cihazda doğrulanmıştır) ve kopyaları bir dize köprüsünden geçer; bu nedenle her karede yeniden yazılan büyük arabellekler orada 60 fps'nin oldukça altında çalışır ([piksel arabellekleri](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/images.md#pixel-buffers)).

## Bir görünümü yakalamak

Bağlanmış bir görünümün `core.Ref` değeriyle `cx.Capture(&ref, done)` veya geçerli yüzey için `cx.Capture(nil, done)` çağırın. Geri çağrı, motor goroutine'inde yüzeyin piksel ölçeğinde PNG baytlarını veya bir hatayı alır. Sonucu görüntülemek için `core.ImageSource{Data: png}` kullanın. Paylaşmak için baytları uygulamanın önbellek dizininde bir `.png` dosyasına yazın ve yolunu `core.ShareOptions.Files` içinde verin. [Dosya paylaşma](services.md#bağlantıları-ve-dosyaları-açma)

Yakalama, görünümün sınırları içindeki görünür yüzey piksellerini alır. macOS, iOS, tvOS, Windows, Linux, Android ve web üzerinde kullanılabilir. Android, etkinliğin penceresini yakalar. Web yakalaması, NSGo gölgelendirici çıktısı dahil uygulamanın DOM'unu bir tuvale boyar; gömülü çerçeveleri, okuyamadığı görüntüleri ve bazı CSS efektlerini atlar.

## Kaynaklar ve ölçekler

`resources/` altındaki dosyalar uygulamayla birlikte gönderilir. `core.Src("asset:images/cover.png")` bunlardan birini yükler ve `Scales: core.ScalesOf(1, 2, 3)` ekranın piksel oranına göre `cover.png`, `cover@2x.png` veya `cover@3x.png` dosyasını seçer.

```go
core.Image{Source: core.ImageSource{
	URI:    "asset:images/cover.png",
	Scales: core.ScalesOf(1, 2, 3),
}}
```

`Placeholder`, resim yüklenirken bir kaynak gösterir ve `FadeDuration` yüklenen resmi onun üzerinde belirerek getirir; 0 seçimi platforma bırakır, bir nanosaniye ise resmi hemen gösterir. `OnLoadStart`, `OnProgress`, `OnLoad`, `OnError` ve `OnLoadEnd` yüklemeyi izler. `cx.ImageSize` bir kaynağın boyutunu göstermeden piksel cinsinden okur, `cx.PrefetchImage` birini önbelleğe indirir, `cx.SetImageCacheLimit` bellek içi kodlanmış görüntü bayt bütçesini ayarlar (varsayılan `core.DefaultImageCacheLimit`, 32 MiB; negatif değerler 0'a sabitlenir) ve `cx.KeepImage` bir atlası veya dokuyu, döndürdüğü `release` çalışana ya da kapsamı yok edilene kadar önbellekten çıkarılmaya karşı sabitler. Sınır, sabitlenenler, sürmekte olan yüklemeler ve geçerli çizim geçişi hariç her önbelleğe uygulanır; çözülmüş pikselleri, işlem belleğini veya tarayıcı önbelleğini sınırlamaz. Sıfır, sabitlenmemiş görüntülerin tutulmasını devre dışı bırakır. Yerel arka uçlar uzak görüntüleri kendilerine ait bir disk önbelleğinde tutar.

## Animasyonlu ve vektör görüntüler

GIF ve WebP animasyonları görünümde oynar; her kare kendi gecikmesi kadar gösterilir ve dosyanın belirttiği şekilde döngüye girer. SVG belgeleri görüntü olarak yüklenir ve her boyutta keskin kalır; tek renkli bir SVG `TintColor` ile renklendirilebilir.

## Simgeler

`core.Icon`, bir SVG'yi devraldığı metin renginde, stili onu boyutlandırmadıkça yazı tipi boyutu yüksekliğinde çizer. Kutusu SVG'nin en boy oranını korur, bu yüzden kare bir kutu onu germez.

```go
core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 20}, Label: "Search"}
```

`icons/lucide`, her Lucide simgesi için PascalCase ile adlandırılmış bir erişimci içerir (`lucide.ArrowUpRight`). Bir simge, `Label` onu adlandırmadıkça yardımcı teknolojiler için süslemedir. `ui.Icon`, `ui`'ın varsayılanlarıyla aynı şeydir; `core.SVG` ve `core.SVGView` ise uygulamanın kendisinin oluşturduğu yolları çizer.

## Stilli görüntü

`ui.Image`, yüklenirken bir yer tutucu ve başarısız olduğunda soluk bir resim simgesi gösteren, köşeleri yuvarlatılmış bir çukur içindeki bir resimdir. Stili her iki kenarı da boyutlandırmadıkça kaynağın en boy oranını, kaynak yoksa 3:2 oranını korur.

```go
ui.Image{
	Source:      core.Src("asset:images/cover.png"),
	Label:       "Cover",
	ResizeMode:  core.Cover,
	AspectRatio: 16.0 / 9.0,
	Placeholder: ui.Skeleton{Style: core.Style{Height: core.Pt(180)}},
}
```

## Sonraki adımlar

- [Metin](text.md): bir satırın içindeki görüntüler, span olarak.
- [Stiller ve temalar](styles.md): köşeler, gölgeler ve renk tonları.
- [Listeler ve kaydırma](lists.md): yeniden kullanılan resim satırları.

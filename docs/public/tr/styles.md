---
title: Stiller ve temalar
description: Tipli stiller, Tailwind sınıfları, durum stilleri ve açık ile koyu temalar.
group: Temel kavramlar
order: 52
---

## Bir stil

`core.Style`, bir kutunun yerleşimi, görünümü ve metni için tek bir Go değeridir. Her alanın sıfır değeri ayarlanmamış anlamına gelir, bu yüzden stiller birleştirilebilir: `Merge` yalnızca sonraki stilin ayarladıklarını değiştirir ve bir alt öğe, üst öğelerinin ayarladığı metin alanlarını devralır. Bir uzunluk veren bir stilin üzerine o uzunluğu hiçbir şeye ayarlamak için, bir renk için `core.Transparent`'ın yaptığı gibi `Padding`, `Margin`, `Borders`, `BorderWidth`, `CornerRadius` veya bir boşlukta `core.Zero` kullanın: `Padding: core.All(core.Zero)`.

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

Stiller CSS'in renklerini, köşelerini, kenarlıklarını, dış çizgilerini, gölgelerini, gradyanlarını, filtrelerini, arka plan bulanıklaştırmalarını, maskelerini, karışım modlarını, 3B dönüşümlerini ve geçişlerini alır. `core.RGB`, `core.RGBA` ve `core.ParseColor` renk oluşturur, `core.LinearGradient` ve benzerleri gradyan oluşturur, `core.Ring` Tailwind'in halkasını (ring) oluşturur ve `core.AllCorners` her köşeye kendi yarıçapını verir. `BorderCurveContinuous`, köşeleri iOS'un yaptığı gibi kenarlarına doğru yuvarlar. Bir kutunun gölgeleri ve dış çizgisi, [Yerleşim](layout.md#konum) sayfasının belirttiği durumlar dışında `Overflow` kırpmasının dışında kalır. Arka plan filtreleri (backdrop filter), CSS'in `backdrop-filter`'ının yaptığı gibi bir görünümün arkasındakini değiştirir: bir `Blur` onu yarıçapı kadar bulanıklaştırır, renk filtreleri ise renklerini değiştirir. iOS, renk filtresi olmayan bir bulanıklaştırma için `View.IOS.BackdropMaterial`'ın adlandırdığı sistem malzemesini çizer; `core.BackdropUltraThin` dışında bir malzeme adlandıran bir bulanıklaştırma, iOS'ta o malzemeyi, macOS'ta ise AppKit'in en yakın malzemesini çizer ve renk filtrelerini dışarıda bırakır. Linux ve Android arka plan filtresi çizmez. `Style.GlassEffect` (`core.GlassRegular` veya `core.GlassClear`, TSX'te `glassEffect`), macOS 26 ve iOS 26'da bir arka plan bulanıklaştırması yerine Liquid Glass çizer (Windows'ta NSGo'nun cam gölgelendiricisini çizer; başka yerlerde geri dönülecek bir bulanıklaştırmayı cam görünüme verin). `Style.GlassInteractive` (TSX'te `glassInteractive`) ayarlamak, `OnPress`'i ve basılabilir alt öğeleri çalışır durumda tutarken macOS 27'de (`NSGlassEffectView.effectIsInteractive`) ve iOS 26'da (`UIGlassEffect.isInteractive`) Liquid Glass için sistemin etkileşimli basma tepkisini etkinleştirir.

`Transform`, işaretçi hedefini görünümle birlikte taşır: basmalar, üzerine gelme (hover) ve hareketler görünümün çizildiği yerde başlar ve bildirilen işaretçi konumları dönüşümü geri alır. Üst öğesinin dışına çıkan dönüştürülmüş bir alt öğe, bir kırpma veya kaydırma görüntü alanı onu gizlemedikçe erişilebilir kalır. Mini programlar, oluşturma katmanlarının çalıştırdığı anahtar kareleri henüz geri almaz ([yol haritası](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md#mini-programs)).

```go
core.View{Style: core.Style{
	CornerRadii: core.Corners{TopLeft: 28, BottomRight: 28},
	BorderCurve: core.BorderCurveContinuous,
	Shadows:     []core.BoxShadow{core.Ring(1, core.RGBA(0x000000, 0.08)), ui.Shadow(0, 8, 24, -6, ui.Black(0.18))},
}}
```

## Durumlar

Bir görünümün görünüşü durumuyla birlikte değişebilir. `Hover`, `Focus`, `FocusWithin`, `Active`, `Selected`, `Checked`, `Disabled` ve `aria-*` durumları, durum sürdüğü sürece geri kalanın üzerine birleşen `*Style` değerleridir. Bir uygulama `Selected`, `Checked`, `Disabled` ve `Expanded` durumlarını fonksiyonlar aracılığıyla ayarlar; basmaları ve odağı ise platform bildirir.

`Active` ve `group-active:`, bir görünüme basılı tutulduğu sürece onu izler; bunun için bir basma işleyicisi veya dokunuşu tutan bir `Responder` gerekir. Düz bir görünümü `Active`, `active:` veya `group` ile stillendirmek onu basılabilir ya da bir klavye Tab durağı yapmaz; görünümün kendisi odak alması gerektiğinde `Focusable` ayarlayın.

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

Bir bileşenin bir parçası, durumları kendisi bildirmeden çevresindeki denetimin durumlarını alabilir: `PartStates` `Checked`, `Selected`, `Disabled` ve diğerlerini verir; böylece bir onay kutusunun işareti ekran okuyucular için de onay kutusunu izler.

## Sınıflar

`View`, `Text`, `Span`, `Input`, `Image`, `Scroll` ve `Canvas` üzerindeki `Class` Tailwind CSS 4 sınıflarını alır ve bir `base` bileşeninin her parçasının stilinin yanında bir sınıfı vardır. Yardımcı sınıflar (utility), varyantlar ve keyfi değerler, Tailwind'in varsayılan teması ve paleti üzerinde Tailwind'de ne anlama geliyorsa o anlama gelir.

```go
core.View{Class: "flex items-center gap-3 rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"}
```

Bir sınıf stilin altında birleşir, bu yüzden stilin alanları kazanır. Sınıflar `ClassFunc` ile değişenleri izleyebilir ve bir durum varyantı (`hover:bg-zinc-100`, `checked:opacity-100`) stilin durumu haline gelir. `group` ve `group-hover:` alt öğeleri bir üst öğenin durumundan stillendirir, `first:` ve `last:` bir alt öğenin yerini izler, `aria-checked:`, `data-open:` ve `focus-within:` ise bir denetimin gösterdiği durumları izler. `pointer-fine:` ve `pointer-coarse:`, CSS'in `pointer` medya özelliğine `cx.Env().Pointer()` aracılığıyla uyarak bir alt ağacı fare veya dokunmatik ekran için stillendirir (web'de tarayıcının `pointer` sorgusu; masaüstlerinde ve Mac'lerde fine, telefonlarda, tabletlerde ve saatlerde coarse, TV'lerde ise hiçbiri) ve `dark:` ve `hover:` gibi diğer varyantlarla birleştirilebilir; böylece dokunma veya işaretçi boyutlandırmasını `core`'un örtük olarak değiştirmesi yerine uygulama açıkça seçer.

Sonraki kardeş öğeleri bir denetimin durumundan stillendirmek için denetimi `peer` ile işaretleyin; örneğin `Class: "peer"` olan bir onay kutusu ve ardından `Class: "peer-checked:text-accent peer-disabled:opacity-50"` olan bir etiket. Bir peer, `Show` ve `Each` dahil olmak üzere yalnızca aynı üst öğe altında kendisinden sonra gelen kardeşleri stillendirir; asla önceki kardeşleri veya kendi içindeki görünümleri stillendirmez. Tek bir kümeyi adlandırmak için `peer-checked/field:` ile birlikte `peer/field`, bir denetimin boolean `invalid` veri durumu için ise `peer-data-invalid:` kullanın. İşaretleyiciler ve durumlar reaktif olarak güncellenir. Bir görünümü işaretlemek onu bir Tab durağı yapmaz veya basmaları almasını sağlamaz; bu yüzden `peer-active:` yalnızca bir düğme gibi basmaları kendisi alan bir peer'i izler.

`has-[>svg]:px-3`, bir üst öğenin doğrudan bir simge alt öğesi olduğunda dolgusunu değiştirir; `[&_svg]:size-4 [&_svg]:shrink-0` alt soyundaki simgeleri boyutlandırır. `svg`, koşullu bölgeler üzerinden de olmak üzere `core.Icon` ve `SVGView` öğelerini seçer. Bu seçiciler alt öğeler, sınıflar veya durumlar değiştiğinde güncellenir. `hover:[&_svg]` üst öğenin hover durumunu izler; `[&_svg]:hover` ise her simgenin kendi durumunu izler. Bir seçicinin `active`'i, `peer-active:`'in yaptığı gibi yalnızca bir düğme gibi basmaları alan görünümleri izler. Eşleşen seçiciler, varyant sırasından önce özgüllüğe (specificity) göre yarışır; sınıf important olmadıkça satır içi stiller kazanır.

Seçiciler her hedefte NSGo öğeleriyle eşleşir. Listeleri, alt öğe (`>`), alt soy (`_`), bitişik (`+`) ve sonraki kardeş (`~`) birleştiricilerini, sınıfları, `data-*` varlığını/eşitliğini, rolleri, desteklenen boolean erişilebilirlik durumlarını ve `:is`, `:where`, `:not`, `:has` ile alt öğe konumlarını desteklerler. Türler `view`/`div`, `text`/`span` (tüm metin düğümleri), `scroll`, `input`, `textarea`, `image`/`img`, `canvas`, `svg`, `video`, `webview` ve `native`'dir. Her keyfi seçici alternatifinde üst düzeyde bir `&` kullanın; o olmadan seçici öğenin kendisini test eder. Yalnızca tarayıcıya özgü seçiciler, sözde öğeler (pseudo-element), at-kuralları, kaçış dizileri ve bir fonksiyon içindeki açık `&` bilinmez; bu seçiciler arka uç uygulama görünümlerine veya tek tek metin span'lerine ulaşmaz.

`Span.Class` veya `Screen.HeaderClass` içindeki `peer-*` varyantlarının izleyecek kardeşi yoktur. Bunlar yok sayılır ve her sınıf ve bağlam için bir kez günlüğe kaydedilir; bir span'in peer varyantını bunun yerine onu içeren `Text` üzerine koyun.

Temanın `Tailwind` alanı varsayılan temayı renkler, kesme noktaları ve yardımcı sınıflarla genişletir; paletin içermediği bir renk ise, düğümün temasından gösterdiği görünümde okunan bir `Palette` rolünü (`bg-accent`, `text-text-muted/70`) veya bir tema rengini adlandırır.

## Temalar

Bir `core.Theme`, her iki görünümde tek bir görünüştür: açık için bir `Palette` ve koyu için bir tane, bir köşe yarıçapı ve yazı tipi aileleri. Paletin rolleri `Background`, `Surface`, `Text`, `TextMuted`, `Border`, `Accent`, `AccentText`, `Danger`, `Warning`, `Success`, `Selection`, `Focus` ve `Scrollbar`'dır; `Colors` ise `inset` ve `raised` gibi bir kütüphanenin eklediklerini tutar. Bir uygulamanın başka bir tema ayarlayana kadar gösterdiği `core.DefaultTheme()`, mavi vurgu rengine ve `ui`'ın rollerine sahip nötr temadır; `ui.WarmDuskTheme()` ise sıcak bir palettir.

```go
theme := core.DefaultTheme()
theme.Light.Accent, theme.Dark.Accent = core.RGB(0xbd481b), core.RGB(0xed986e)
theme.Radius = 10
cx.SetTheme(theme)
```

`cx.SetTheme` bir temayı kapsama ve onun altında oluşturulan her şeye verir; böylece bir kenar çubuğu veya bir panel kendine ait bir tema alabilir; nil bir tema yukarıdakini alır. `core.ParseTheme` rol renkleri, bir yarıçap ve yazı tipi aileleriyle bir YAML veya JSON tema dosyasını ya da bir shadcn/ui tema dosyasının CSS değişkenlerini okur. `core.DefaultTheme` (bir uygulamanın bir tema ayarlayana kadar gösterdiği tema) ve `ui.WarmDuskTheme` değiştirilmeye hazır temalar döndürür.

`cx.SetAppearance` tüm uygulamayı açık veya koyu olmaya zorlar, `cx.SetSubtreeAppearance` bunu bir alt ağaç için yapar ve `core.AppearanceSystem` yeniden sistemi izler. `cx.Dark()` bir alt ağacın hangi görünümü gösterdiğini söyler, `cx.Palette()` ise gömülen ana bilgisayarın (host) geçersiz kılmaları uygulanmış olarak renklerini verir.

Hiçbir üst öğe `Style.Color` ayarlamadığında, metin ve `currentColor` simgeleri alt ağacın `Palette.Text`'ini kullanır ve tema, görünüm ve host token değişikliklerini yerinde izler. `core.Transparent` dahil açık bir renk devralınmaya devam eder.

## Bileşenlerde renkler

Bir bileşen renklerini oluşturulduğu yerde ve `StyleFunc`'larında okur; böylece bir tema veya görünüm değişikliği, bileşen yeniden oluşturulmadan yerinde izlenir. `ui.ColorsOf(cx)` `ui`'ın kullandığı rolleri adlandırır, `ui.TintsOf(cx)` bunların yarı saydam dolgularını verir ve `ui.FinOf(cx)` `ui`'ın katmanlı yüzey işlemlerini (finish) verir.

```go
colors := ui.ColorsOf(cx)
core.Text{Content: "Hello", StyleFunc: func() core.Style {
	return core.Style{Color: colors.Text(), FontSize: 15}
}}
```

`StyleFunc`, okuduğu şey değiştiğinde yeniden çalışır; bu yüzden içindeki `cx.Dark()` ve `cx.Palette()` temayı izler. Aynısı `ClassFunc`, bir span'in `StyleFunc`'ı ve bir bileşenin `StyleFunc` alanları için de geçerlidir.

## İmleçler

`Style.Cursor` adlandırılmış bir işaretçi imleci ayarlar (`core.CursorText`, `core.CursorColResize` vb.); basılabilir öğeleri platformun okunu korumaları için `core.CursorAuto` üzerinde bırakın. Masaüstlerinde ve web'de çizim araçları, oyunlar veya özel tuval işaretçileri için `Style.CursorImage`; 1x bir `Image`, isteğe bağlı bir HiDPI `Image2x` (`PixelRatio >= 1.5` olduğunda otomatik olarak seçilir), görüntü sınırlarına sıkıştırılan mantıksal nokta cinsinden bir `Hotspot` ve adlandırılmış bir `Fallback` imleci içeren bir `*core.CustomCursor` alır. Geçerli hedefin özel imleç görüntülerini gösterip göstermediğini görmek için `cx.Env().SupportsCustomCursor()`'u kontrol edin; dokunmatik platformlar bunları yok sayar.

## Sonraki

- [Yerleşim](layout.md): kutuların nereye yerleştiği.
- [Metin](text.md): yazı tipleri, span'ler ve süslemeler.
- [Animasyon](animation.md): stiller arasındaki geçişler.

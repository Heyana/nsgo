---
title: Metin
description: Metni parçalar, yazı tipleri, süslemeler, seçim ve belgelerle gösterin.
group: Temel kavramlar
order: 53
---

## Bir metin

`core.Text` tek bir paragraftır; motor tarafında platformun kendi metin motoruyla dizilir ve ana iş parçacığı tarafından çizilir. Boyutu yazı tipinden ve genişliğinden gelir ve kutusunun içinde satır kaydırır.

```go
core.Text{Content: "Hello from NSGo", Style: core.Style{
	FontSize:   15,
	LineHeight: 22,
	FontWeight: core.FontWeightSemibold,
}}
```

`Content` sabittir, `ContentFunc` ise bir sinyali izler; bu nedenle değişen bir metin sinyali fonksiyonun içinde okur. `MaxLines` gösterilen satırları sınırlar, `EllipsizeMode` kalanı başından, ortasından veya sonundan keser ve `WhiteSpaceNoWrap` bir paragrafı tek satırda tutar. `AdjustsFontSizeToFit` metni kutusuna ve satır sınırına sığana kadar `MinimumFontScale` değerine (en az 0.1) kadar küçültür. Formüller ve yazı tipine göre boyutlanan diğer ekler onunla birlikte küçülür ve yer elverdiğinde doğal boyutlarına döner; açıkça verilmiş nokta boyutları ve dolgular yazıldıkları değerleri korur.

Metin alanları CSS'te olduğu gibi kalıtılır: `FontSize`, `Color` veya `LineHeight` içeren bir görünümün içindeki metin, kendi değerlerini ayarlamadıkça bunları alır. `TextAlign`, `TextTransform`, `LetterSpacing`, `TextDecoration`, `TextDecorationStyle`, `TextDecorationColor` ile `TextShadowColor`, `TextShadowOffset` ve `TextShadowRadius` satırlara ve glif'lere stil verir.

## Parçalar ve bağlantılar

`Spans`, her biri kendi stiline sahip parçalardan (run) oluşan bir metin oluşturur. `OnPress` içeren bir parça bir bağlantıdır; başka bir parçaya basmak ise metnin kendi `OnPress`'ini tetikler.

```go
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "By continuing you agree to the "},
	{Content: "terms", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openTerms},
	{Content: " and the "},
	{Content: "privacy policy", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openPrivacy},
	{Content: "."},
}}
```

`SpansFunc`, okuduğu şey değiştiğinde parçaları yeniden verir ve yalnızca farklı olan bölümler yeniden hazırlanır; böylece bir kod düzenleyicinin satırları vurgulama geldikçe yeniden renklenir. Bir parçanın `Background` ve `CornerRadius` alanları her satırda karakterlerinin arkasına bir kutu çizer; `Cursor` alanı ise üzerlerindeki işaretçinin görünümünü metninkinden önce belirler. `Visibility: core.VisibilityHidden` bir parçayı yerinde gizler: sözcükleri yerlerini korur, böylece parça göründüğünde satırlar yerinde kalır; kopyalar ve ekran okuyucular onu dışarıda bırakır. Bu, sözcükleri birer birer beliren metinler içindir.

Her satırda kutusunun alt %38'ini vurgulamak için `Background` içeren bir parçaya `BackgroundInsetTop: core.Pct(62)` ayarlayın. `BackgroundInsetBottom` alt kenarı kırpar. Her iki iç boşluk da nokta ya da kutunun yazı tipinin üst uzantısından (ascent) alt uzantısına (descent) kadar olan yüksekliğinin yüzdesini alır. Metin yerleşimini değiştirmezler. TSX'te iç içe bir `<Text>`'in stilinde `backgroundInsetTop: "62%"` ve `backgroundInsetBottom` ayarlayın. Yuvarlatılmış arka planlar ve dikey iç boşluklar HarmonyOS'ta ve dört mini program hedefinde de çizilir.

`Attachment`, parçanın içeriği yerine metnin içine bir görünüm yerleştirir. Kendi boyutunda yerleştirilir, etrafındaki sözcüklerle birlikte satır kaydırır ve kendi basışlarını alır; böylece bir simge, bir resim veya küçük bir düğme bir satırın içinde durabilir. Bir seçim, onun yerine parçanın `Content` değerini kopyalar; boşsa dışarıda bırakır. Varsayılan olarak ilk metin satırı, gerçek yazı tipinin ölçülerini kullanarak çevresindeki taban çizgisine hizalanır. `Em` tuvali kendi taban çizgisini sağlar; taban çizgisi olmayan bir görünüm alt kenarı üzerinde durur. Başka bir hizalama seçmek için `Style.VerticalAlign` ayarlayın.

```go
core.Text{Style: core.Style{FontSize: 17, LineHeight: 24}, Spans: []core.Span{
	{Content: "Search with "},
	{Attachment: core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 17}}},
	{Content: " NSGo"},
}}
```

## Yazı tipleri

`FontFamily` bir aileyi veya bir CSS listesini (`Inter, "Noto Sans SC", sans-serif`) adlandırır; her biri kendinden öncekilerde bulunmayan karakterler için kullanılır. Listedeki her aile kalınlığı ve stili alır. macOS ve iOS'ta bir satır yüksekliğini ve taban çizgisini karakterlerinin dizildiği yüzlerden alır; bu nedenle tamamen sonraki bir aileyle dizilen bir satır o ailenin değerlerini alır, oysa bir tarayıcı ilk ailenin değerlerini alt sınır olarak korur. `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LineHeightMultiple`, `LetterSpacing` ve `LetterSpacingEm` yüzü ve aralığı belirler; `core.FontMonospace` ise platformun sabit aralıklı ailesidir.

`Language`, metnin BCP 47 etiketidir; metinlerde, parçalarda ve girişlerde yukarıdaki alanlar gibi kalıtılır. Han karakterleri onun biçimleriyle çizilir; böylece `ja` ile dizilen `直`, sistem Çinceyi tercih ettiğinde bile Japonca biçimini alır ve Korece (`ko`), CSS'in `word-break: keep-all` özelliğinin yaptığı gibi satırları sözcükler arasında böler. Hiçbir dil ayarlamayan metinler ve girişler uygulamanın gösterdiği dildedir ve o değiştiğinde onu izler. Bir alt ağaç için bir görünüme veya tek bir parça için bir `Span`'e ayarlayın:

```go
core.Text{Spans: []core.Span{
	{Content: "日本語: "},
	{Content: "直 骨 角", Style: core.Style{Language: "ja"}},
}}
```

Uygulamanın yazı tiplerini `resources/` altında paketleyin ve ilk görünümden önce kaydedin ya da uygulama çalışırken bir yazı tipinin baytlarını kaydedin:

```go
func main() { nsgo.Run(Root, nsgo.Fonts("fonts/Inter-Regular.ttf", "fonts/Inter-Bold.ttf")) }

cx.RegisterFont(data) // a TrueType or OpenType face
```

Bir yazı tipi gelmeden önce yerleştirilmiş bir metin yeniden hazırlanır ve tuvaller `Draw`'ı yeniden çalıştırır; böylece glif'leri ve metin ölçümleri yüklenen yazı tipini kullanır. `cx.Env().FontScale()` kullanıcının metin boyutudur; `NoFontScaling` ayarlanmadıkça metinler onu izler; `MaxFontSizeMultiplier` ona bir üst sınır koyar.

## Seçim ve ölçüm

`SelectableOn` kullanıcının bir metni seçip kopyalamasına izin verir; `Selectable` içeren bir `View` ise metinlerinin bir web sayfasındaki gibi belge sırasıyla birlikte seçilmesini sağlar. `SelectionColor` vurguyu renklendirir. `OnTextSelection`, kullanıcı seçimi hareket ettirdikçe bir `core.TextSelection` alır: düz `Text`, `Markdown` ve `HTML` (alandaki `TextView`'lerin ve stilli metinlerin ayrıştırılmış satır içi ve blok yapısını, kısmi parçalar ve bağlantılar dahil, koruyarak), metin başına `Ranges` ve dokunduğu metinlerin görünüm içindeki `Bounds` değerleri; bunlar Markdown veya HTML olarak kopyalamak, bir sözcük listesi ya da seçilen sözcüklerin çevirisi içindir. `desktop.LookUp` bunları sistemin sözlüğünde gösterir ([hizmetler](services.md#bağlantıları-ve-dosyaları-açma)). `cx.MeasureText` bir dizeyi bir yazı tipinde ölçer; metnine göre boyutlanan bir sütun içindir. Onu çağıran reaktif fonksiyonlar kullanılabilir yazı tipleri değiştiğinde güncellenir ve `OnTextLayout` her yerleşimden sonra satırları bildirir.

## Belgeler

`base/textview`, Markdown veya HTML'i aynı temel yapı taşlarıyla gösterir: başlıklar, paragraflar, listeler, tablolar, kod, dipnotlar, uyarılar, resimler, formüller ve bağlantılar.

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

Formüller (`$…$`, `$$…$$`), uygulama `base/formula`'dan `RenderMath: formula.RenderMath` geçirene kadar TeX'lerini gösterir; bu, onları çevrelerindeki metnin boyutu ve rengiyle dizer, uygulamanın dilinde sesli okur ve TeX olarak kopyalar; yazı tipi dahil, bir masaüstü uygulamasına yaklaşık 6.5 MB, bir web uygulamasına ise gzip ile sıkıştırılmış olarak 3 MB ekler. `formula.View` bir formülü başka herhangi bir yerde TeX veya Typst ile gösterir; `OnHit` geri çağrısı formül düzenleyiciler için Untie kaynak bölümünü, en yakın imleç konumunu ve örtüşen tanılamaları bildirir. watchOS'ta formüller aynı tuval yolunu kullanır; bunları bağlamak 1.4 MB'lık yazı tipi dahil yaklaşık 6.1 MB yürütülebilir dosya boyutu ekler. Bir saat simülatörü çalıştırmasında, o yazı tipini yüklemek ve ayrıştırmak için, sahip olunan bayt kopyası dahil yaklaşık 3.9 MB tutulan Go yığını ölçüldü. Simülatör ölçümleri ne fiziksel bir saatin bellek sınırını ne de kullanılabilir payını belirler; bunları uygulamayı çalıştıran saatte ölçün. Yapılandırılmış yazı tiplerinin karşılayamadığı metin parçaları (`\text{}` veya Typst dizeleri), CJK ve diğer yazı sistemleri dahil sistem yazı tiplerini kullanır; açıkça verilen `formula.Fonts.Fallbacks` önceliklidir.

Parça parça gelen kaynak için `formula.View`'e `SourceFunc` geçirin ve `Streaming: true` ayarlayın ya da `StreamingFunc`'ı uygulamanın alma durumuna bağlayın. Bu, geçerli içerik yerleşmeye devam ederken kurtarılabilir hata ve yer tutucu çizimlerini gizler. Kalan hataları göstermek için alma bittiğinde bayrağı temizleyin. Bir `TextView`'de, sağlanan `Math.TeX` ve `Math.Display` ile o `formula.View`'i döndüren bir `RenderMath` fonksiyonu sağlayın; onun `StreamingFunc`'ı aynı alma durumunu okuyabilir.

Blok formüller, metnin yazı tipi boyutunu koruyarak, bloğun dolgu ve kenarlıkları içindeki genişliğine göre güvenli ilişki ve işleç noktalarında satır kaydırır. Yeniden boyutlandırma veya metin boyutunu değiştirme satırları yeniden akıtır. Bölünemeyen kısımlar yine yana doğru kayar; satır içi formüller doğal boyutlarını korur.

Bir `mermaid` kod bloğu, uygulama `base/diagram`'dan `RenderDiagram: diagram.RenderDiagram` geçirene kadar kaynağını gösterir; bu, onu limu ile yerel bir diyagram olarak çizer: akış şemaları, sıra, sınıf, durum ve varlık ilişki diyagramları, Gantt şemaları, pastalar, zihin haritaları, zaman çizelgeleri ve daha fazlası; platformun yazı tipiyle, uygulamanın gösterdiği gibi açık veya koyu. Bağlantılı diyagram öğeleri basıldığında bağlantıları uygulamanın URL hizmeti üzerinden açar ve bir öğenin Mermaid ipucu, işaretçi üzerinde durduğu sürece görünür. Metnin kullanılabilir genişliğine göre yerleşir; geniş bir akış şemasını etiket boyutunu koruyarak aşağı doğru akacak şekilde çevirir; sığamayan diyagramlar yine yana doğru kayar. Bir kopya kaynağını alır ve hâlâ akış hâlinde gelen bir blok tamamlanana kadar kod olarak görünür. `diagram.View` bir diyagramı başka herhangi bir yerde gösterir ve kapsayıcısının genişliği değiştikçe onu izler. `Icons` alanı `"fa:fa-car"` gibi Mermaid adlarını, `icons/lucide`'dakiler dahil `*core.SVG` belgelerine eşler; bulunmayan adlar satır içinde boşluk bırakmaz. Resim düğümleri HTTP(S), dosya veya `asset:` kaynaklarını paylaşılan tuval resim yükleyicisi üzerinden yükler; `constraint: "on"` ayarlıysa sığdırıp ortalar, aksi halde gerer. Mermaid, resim adreslerini yüklemeden önce süzer (`data:` URL'lerini reddetmek dahil). Bir metin görünümünde simge sağlamak için `RenderDiagram`'ı `diagram.View{Source: code.Code, Icons: icons}` döndüren bir geri çağrıya ayarlayın.

Diyagramlara `ui` paletinizi vermek için `RenderDiagram` geri çağrınızdan `ThemeFunc` içeren bir `diagram.View` döndürün. Fonksiyon renkleri tuval çizerken okur; böylece bir palet veya alt ağaç görünümü değişikliği mevcut görünümü yeniden renklendirir. Sıfır bırakılan renkler diyagramın varsayılanlarını korur.

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

`textview.Parse` Markdown'ı, `textview.ParseHTML` ise HTML'i uygulamanın kendisinin de oluşturabileceği bloklara okur. `Options.Streaming` parça parça gelen bir belgeyi onarır: sonunun açık bıraktığı şeyleri kapatır ve tek başına bir liste işareti gibi anlamını sonraki karakterlerin belirleyeceği son satırı geri tutar; böylece bir yanıt işaretleri asla metin olarak yanıp söndürmez. `StreamingCaret` ise sonuna bir imleç çizer; bu, akış sırasında yazılan bir yanıt içindir. `FadeIn`, belgeye eklenen her karakteri birbiri ardına ve gelişinin çok gerisinde kalmadan yavaşça belirginleştirir; böylece parça parça gelen bir yanıt akıcı okunur.

Bir `textview.Search`, `Search` alanları veya üstlerindeki `textview.ProvideSearch` aracılığıyla ona katılan görünümler genelinde metin bulur. `SetQuery`, `Open` ve `Close` onu bir arama çubuğundan yönetir, `Count` ve `Selected` nerede olduğunu söyler, `Next` ve `Previous` ise eşleşmeleri arasında okuma sırasıyla ilerler. Görünümler her eşleşmeyi `PartSearch` ile, seçili olanı ise `PartSearchSelected` ile işaretler; uygulamanın stilleri bunları renklendirir. Görünümler seçili eşleşmeyi görünür alana kaydırır ve kullanıcının seçimine dokunmaz.

```go
search := textview.NewSearch(cx)
textview.ProvideSearch(cx, search)
search.SetQuery("readme")
search.Open()
```

## Sonraki adımlar

- [Görüntüler ve simgeler](images.md): resimler ve SVG'ler, bir metnin içinde de.
- [Stiller ve temalar](styles.md): renkler ve kalıtılan metin alanları.
- [Yerleşim](layout.md): bir metnin genişliğini nasıl aldığı.

## Zengin metin düzenleme

`ui/richeditor.View`, bir `base/richeditor.Session`'ı temanın görünümüyle gösterir; `base/richeditor.View` ise davranışı uygulamanın sağladığı stillerle sunar. iOS ve Android'de uzun basma bir sözcüğü seçer, çift dokunma da aynısını yapar; aralığı bloklar boyunca genişletmek için seçim tutamaçlarını sürükleyin. Yerel düzenleme menüsü, seçime ve panoya uygun olduğu ölçüde Kopyala ve Tümünü Seç'i, oturum düzenlenebilir olduğunda ise Kes ve Yapıştır'ı sunar. `Session.SetEditable` açık bir menüyü de yeniler. Tutamaçlara temel görünümde `Styles.SelectionHandle` veya `Classes.SelectionHandle` ile stil verin.

Özel bir metin denetimi için bir `core.Input`'a odaklanın ve yüzey noktası cinsinden sınırlar ile `core.EditCut`, `EditCopy`, `EditPaste` ve `EditSelectAll` bit maskesiyle `Ref.OpenEditMenu(bounds, actions)` çağırın. iOS ve Android'de `Input.OnEditAction` Kes, Kopyala ve Tümünü Seç'i alır; Yapıştır `Input.OnPaste`'i kullanır. Kapatmak için `Ref.CloseEditMenu` çağırın.
